//go:build integration

package metrics_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/metrics"
)

// setupMetrics starts an ephemeral Postgres, applies all migrations and the
// views/ directory (mirroring database/init/init-db.sh), and returns a raw
// *sql.DB (for goose + ad-hoc queries) alongside a *pgxpool.Pool (what the
// metrics functions take). Mirrors internal/base/classification_test.go:setupClassified.
func setupMetrics(t *testing.T) (context.Context, *sql.DB, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	pg, err := postgres.Run(
		ctx, "postgres:16-alpine",
		postgres.WithDatabase("fathom_test"),
		postgres.WithUsername("fathom"),
		postgres.WithPassword("fathom"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "../../database/migrations"))

	views, err := filepath.Glob("../../database/views/*.sql")
	require.NoError(t, err)
	sort.Strings(views)
	for _, v := range views {
		b, err := os.ReadFile(v)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, string(b))
		require.NoError(t, err, "applying view %s", filepath.Base(v))
	}

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return ctx, sqlDB, pool
}

func TestRebuild_Conservation(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.005"}, // agentic, dust
		{"0xb", 0, "2026-06-01T11:00:00Z", "0xfac1", "0xp2", "0xs1", "2.50"},  // agentic, small
		{"0xc", 0, "2026-06-02T09:00:00Z", "0xfac2", "0xp3", "0xs2", "5.00"},  // contested, small
	})

	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// Cube totals must equal the same aggregate taken directly from the view.
	var cubeTxns, viewTxns int64
	var cubeVol, viewVol string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT coalesce(sum(txn_count),0), coalesce(sum(volume_usdc),0)::text FROM metrics_daily_v1`).
		Scan(&cubeTxns, &cubeVol))
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*), coalesce(sum(amount_usdc),0)::text FROM payment_classified_v1`).
		Scan(&viewTxns, &viewVol))
	require.Equal(t, viewTxns, cubeTxns, "cube txn_count must equal view row count")
	require.Equal(t, viewVol, cubeVol, "cube volume must equal view volume")

	// Grain check: one agentic dust row on day 1.
	var n int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT txn_count FROM metrics_daily_v1
		  WHERE day='2026-06-01' AND attribution='agentic' AND amount_band='dust'`).Scan(&n))
	require.Equal(t, int64(1), n)

	// The cube carries the view's methodology version, single-valued.
	var version int16
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT DISTINCT methodology_version FROM metrics_daily_v1`).Scan(&version))
	require.Equal(t, int16(1), version)
}

func TestRebuild_Idempotent(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac2", "0xp1", "0xs1", "5.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t))) // second run must not double-count
	var total int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT sum(txn_count) FROM metrics_daily_v1`).Scan(&total))
	require.Equal(t, int64(1), total)
}

func TestRebuild_MissingPriceMonthFailsAndPreservesTables(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac2", "0xp1", "0xs1", "5.00"},
	})
	// First rebuild succeeds and populates the cube.
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// Second rebuild with a price file that lacks 2026-06 must fail BEFORE
	// truncating anything: the previous generation stays queryable.
	bad := metrics.ETHPrices{
		Source: "test", Unit: "USD per ETH",
		Prices: map[string]decimal.Decimal{"2026-01": decimal.NewFromInt(2000)},
	}
	err := metrics.Rebuild(ctx, pool, bad)
	require.ErrorContains(t, err, "2026-06")

	var n int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM metrics_daily_v1`).Scan(&n))
	require.NotZero(t, n, "failed rebuild must leave the previous cube intact")
}

func TestAmountBand_Boundaries(t *testing.T) {
	ctx, db, _ := setupMetrics(t)
	cases := []struct {
		usd  string
		want string
	}{
		{"0.009", "dust"},
		{"0.01", "micro"},
		{"0.999999", "micro"},
		{"1", "small"},
		{"99.999999", "small"},
		{"100", "mid"},
		{"999.999999", "mid"},
		{"1000", "whale"},
	}
	for _, c := range cases {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT amount_band($1::numeric)`, c.usd).Scan(&got))
		require.Equal(t, c.want, got, "amount_band(%s)", c.usd)
	}
}
