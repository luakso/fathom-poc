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
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
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
