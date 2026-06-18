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
		`SELECT coalesce(sum(txn_count),0), coalesce(sum(volume_usdc),0)::text FROM metrics_daily_v2`).
		Scan(&cubeTxns, &cubeVol))
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*), coalesce(sum(amount_usdc),0)::text FROM payment_x402_v1`).
		Scan(&viewTxns, &viewVol))
	require.Equal(t, viewTxns, cubeTxns, "cube txn_count must equal view row count")
	require.Equal(t, viewVol, cubeVol, "cube volume must equal view volume")

	// Grain check: one agentic dust row on day 1.
	var n int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT txn_count FROM metrics_daily_v2
		  WHERE day='2026-06-01' AND membership='known' AND amount_band='dust'`).Scan(&n))
	require.Equal(t, int64(1), n)

	// The cube carries the view's methodology version, single-valued.
	var version int16
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT DISTINCT methodology_version FROM metrics_daily_v2`).Scan(&version))
	require.Equal(t, int16(1), version)
}

func TestRebuild_MembershipConservation(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1") // known
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // known
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"}, // known
		{"0xc", 0, "2026-06-05T12:00:00Z", "0xfac2", "0xp3", "0xs2", "9.00"}, // unknown
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var known, unknown, all int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT txn_count FROM metrics_window_stats_v2 WHERE window_name='all' AND membership='known'`).Scan(&known))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT txn_count FROM metrics_window_stats_v2 WHERE window_name='all' AND membership='unknown'`).Scan(&unknown))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT txn_count FROM metrics_window_stats_v2 WHERE window_name='all' AND membership='all'`).Scan(&all))
	require.Equal(t, all, known+unknown, "membership partition must reconcile to the independently-computed 'all' row")
	require.Equal(t, int64(2), known)
	require.Equal(t, int64(1), unknown)
}

func TestRebuild_Idempotent(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac2", "0xp1", "0xs1", "5.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t))) // second run must not double-count
	var total int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT sum(txn_count) FROM metrics_daily_v2`).Scan(&total))
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
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM metrics_daily_v2`).Scan(&n))
	require.NotZero(t, n, "failed rebuild must leave the previous cube intact")
}

func TestRebuild_WindowStatsMedians(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		// Three agentic amounts on the anchor day: median = 2.00.
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"},
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"},
		{"0xc", 0, "2026-06-05T12:00:00Z", "0xfac1", "0xp3", "0xs1", "9.00"},
		// Contested whale 40 days earlier: inside 'all', outside '30d'/'7d'.
		{"0xd", 0, "2026-04-26T09:00:00Z", "0xfac2", "0xp4", "0xs2", "5000.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var median string
	var txns int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT median_amount_usdc::text, txn_count FROM metrics_window_stats_v2
		WHERE window_name='7d' AND membership='known'`).Scan(&median, &txns))
	require.Equal(t, "2.000000", median)
	require.Equal(t, int64(3), txns)

	// 'all' attribution row aggregates across attributions; in the all-window
	// it covers all four payments (even count: percentile_disc picks the lower
	// middle = 2.00).
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT median_amount_usdc::text, txn_count FROM metrics_window_stats_v2
		WHERE window_name='all' AND membership='all'`).Scan(&median, &txns))
	require.Equal(t, "2.000000", median)
	require.Equal(t, int64(4), txns)

	// The contested whale exists in 'all' but not '30d'.
	var contested30d int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM metrics_window_stats_v2
		WHERE window_name='30d' AND membership='unknown'`).Scan(&contested30d))
	require.Zero(t, contested30d)
}

func TestRebuild_PricePointsAgenticTopN(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		// 0.10 three times across two distinct payees → rank 1.
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.10"},
		{"0xb", 0, "2026-06-05T10:01:00Z", "0xfac1", "0xp2", "0xs2", "0.10"},
		{"0xc", 0, "2026-06-05T10:02:00Z", "0xfac1", "0xp3", "0xs1", "0.10"},
		// 5.00 once → rank 2.
		{"0xd", 0, "2026-06-05T10:03:00Z", "0xfac1", "0xp1", "0xs1", "5.00"},
		// Contested 0.10 must NOT count (agentic only).
		{"0xe", 0, "2026-06-05T10:04:00Z", "0xfac2", "0xp4", "0xs3", "0.10"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var amount string
	var txns, payees int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT amount_usdc::text, txn_count, payee_count FROM metrics_price_points_v2
		WHERE window_name='all' AND rank=1`).Scan(&amount, &txns, &payees))
	require.Equal(t, "0.100000", amount)
	require.Equal(t, int64(3), txns)
	require.Equal(t, int64(2), payees)

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT amount_usdc::text FROM metrics_price_points_v2
		WHERE window_name='all' AND rank=2`).Scan(&amount))
	require.Equal(t, "5.000000", amount)
}

func TestRebuild_PricePointsTieBreakByAmount(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// Equal txn counts: the lower amount must take the lower rank.
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "5.00"},
		{"0xb", 0, "2026-06-05T10:01:00Z", "0xfac1", "0xp2", "0xs1", "0.10"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var amount string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT amount_usdc::text FROM metrics_price_points_v2
		WHERE window_name='all' AND rank=1`).Scan(&amount))
	require.Equal(t, "0.100000", amount)
}

func TestRebuild_WindowBoundaryInclusive(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// Anchor day = 2026-06-05. 30d window = anchor-29 .. anchor, inclusive:
	// 2026-05-07 is the edge (in); 2026-05-06 is one day out.
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T12:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"},
		{"0xb", 0, "2026-05-07T00:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"}, // edge: in
		{"0xc", 0, "2026-05-06T23:59:59Z", "0xfac1", "0xp3", "0xs1", "4.00"}, // out
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var txns int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count FROM metrics_window_stats_v2
		WHERE window_name='30d' AND membership='known'`).Scan(&txns))
	require.Equal(t, int64(2), txns, "30d must include the anchor-29 edge day and exclude anchor-30")

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count FROM metrics_window_stats_v2
		WHERE window_name='all' AND membership='known'`).Scan(&txns))
	require.Equal(t, int64(3), txns)
}

func TestRebuild_PricePointsWindowed(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.10"},
		{"0xb", 0, "2026-04-01T10:00:00Z", "0xfac1", "0xp2", "0xs1", "0.10"}, // outside 30d
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var txns int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count FROM metrics_price_points_v2
		WHERE window_name='30d' AND rank=1`).Scan(&txns))
	require.Equal(t, int64(1), txns)

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count FROM metrics_price_points_v2
		WHERE window_name='all' AND rank=1`).Scan(&txns))
	require.Equal(t, int64(2), txns)
}

func TestRebuild_GasDedupesAndConserves(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedGasPayments(t, ctx, db, []seedGasRow{
		// One batch tx: 3 payments, tx gas 300 wei carried on EVERY row.
		// Naive row-sum would count 900; correct total is 300 (100 per payment).
		{"0xbatch", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.005", "300"},
		{"0xbatch", 1, "2026-06-05T10:00:00Z", "0xfac1", "0xp2", "0xs1", "0.50", "300"},
		{"0xbatch", 2, "2026-06-05T10:00:00Z", "0xfac1", "0xp3", "0xs2", "50.00", "300"},
		// One single-payment tx, gas 100 wei.
		{"0xsingle", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00", "100"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// Conservation: apportioned wei across all rows == 300 + 100. Compare in
	// SQL — numeric division (300/3) carries trailing decimal zeros, so a
	// ::text comparison against "400" would fail on formatting, not math.
	var conserved bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sum(l2_gas_cost_wei + l1_fee_wei) = 400 FROM metrics_gas_daily_v2`).Scan(&conserved))
	require.True(t, conserved, "apportioned wei must equal the per-tx sum (300 + 100)")

	// Band split: the batch spreads 100 wei each into dust (0.005), micro
	// (0.50), small (50.00); the single tx's 2.00 is also 'small', so
	// small = 100 (batch share) + 100 (single tx) = 200.
	var smallOK bool
	var smallTxns int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT sum(l2_gas_cost_wei + l1_fee_wei) = 200, sum(txn_count) FROM metrics_gas_daily_v2
		WHERE amount_band='small'`).Scan(&smallOK, &smallTxns))
	require.True(t, smallOK)
	require.Equal(t, int64(2), smallTxns)

	// Payment counts conserve against the cube.
	var gasTxns, cubeTxns int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sum(txn_count) FROM metrics_gas_daily_v2`).Scan(&gasTxns))
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sum(txn_count) FROM metrics_daily_v2`).Scan(&cubeTxns))
	require.Equal(t, cubeTxns, gasTxns)
}

func TestRebuild_GasBreakeven(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// 1e15 wei at the $2000 test price = $2 of gas.
	seedGasPayments(t, ctx, db, []seedGasRow{
		// $2 gas > $0.005 moved → breakeven breach.
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.005", "1000000000000000"},
		// $2 gas < $5 moved → fine.
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp1", "0xs1", "5.00", "1000000000000000"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var breakeven int64
	var usd string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT sum(breakeven_txn_count), sum(cost_usd)::text
		FROM metrics_gas_daily_v2`).Scan(&breakeven, &usd))
	require.Equal(t, int64(1), breakeven)
	require.Equal(t, "4.00000000", usd) // 2 × ($2 per payment)
}

func TestRebuild_GasApportionNonTerminating(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// 100 wei over 3 payments does not divide evenly: conservation holds to
	// numeric precision (sub-wei drift), not exactly.
	seedGasPayments(t, ctx, db, []seedGasRow{
		{"0xb3", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.005", "100"},
		{"0xb3", 1, "2026-06-05T10:00:00Z", "0xfac1", "0xp2", "0xs1", "0.50", "100"},
		{"0xb3", 2, "2026-06-05T10:00:00Z", "0xfac1", "0xp3", "0xs2", "50.00", "100"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var withinDrift bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT abs(sum(l2_gas_cost_wei + l1_fee_wei) - 100) < 1e-6 FROM metrics_gas_daily_v2`).Scan(&withinDrift))
	require.True(t, withinDrift, "apportioned sum must conserve tx gas to sub-wei precision")
}

func TestRebuild_VelocityPerMinute(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		// Two payments inside the same minute, one an hour later: max_per_min=2.
		{"0xa", 0, "2026-06-05T10:00:10Z", "0xfac2", "0xp1", "0xs1", "1.00"},
		{"0xb", 0, "2026-06-05T10:00:50Z", "0xfac2", "0xp2", "0xs1", "1.00"},
		{"0xc", 0, "2026-06-05T11:00:00Z", "0xfac2", "0xp3", "0xs1", "1.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var txns int64
	var maxPM, p99PM int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count, max_per_min, p99_per_min FROM metrics_velocity_daily_v2
		WHERE day='2026-06-05' AND membership='unknown'`).Scan(&txns, &maxPM, &p99PM))
	require.Equal(t, int64(3), txns)
	require.Equal(t, 2, maxPM)
	require.Equal(t, 2, p99PM) // p99 over active minutes [2,1] picks 2
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

func TestRebuild_GasFoldsL1Fee(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// Single-payment tx: L2 = 100 wei, L1 = 300 wei. Total cost = 400 wei.
	seedL1GasPayments(t, ctx, db, []seedL1GasRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "5.00", "100", "300"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// Compare numerically, not via ::text: numeric division (tx_l2 / n) carries
	// trailing decimal zeros even for n=1, so "100.0000000000000000" != "100"
	// would fail on formatting, not math — the same reason the conservation
	// tests above compare with `= N` rather than ::text.
	var l2OK, l1OK, total bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT l2_gas_cost_wei = 100, l1_fee_wei = 300, (l2_gas_cost_wei + l1_fee_wei) = 400
		FROM metrics_gas_daily_v2 WHERE membership='known'`).Scan(&l2OK, &l1OK, &total))
	require.True(t, l2OK, "L2 execution gas component must be 100 wei")
	require.True(t, l1OK, "L1 data fee component must be 300 wei")
	require.True(t, total, "total settlement cost must be l2 + l1 = 400 wei")
}

func TestRebuild_GasFoldsL1FeeAcrossBatch(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// One batch tx, 3 payments across 3 bands. tx L2=300, tx L1=300, each
	// carried identically on every row (production shape). Apportioned 100/100
	// per payment — exercises the n>1 dedup (max) + apportioning (sum(tx_l1/n)).
	seedL1GasPayments(t, ctx, db, []seedL1GasRow{
		{"0xbatch", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.005", "300", "300"},
		{"0xbatch", 1, "2026-06-05T10:00:00Z", "0xfac1", "0xp2", "0xs1", "0.50", "300", "300"},
		{"0xbatch", 2, "2026-06-05T10:00:00Z", "0xfac1", "0xp3", "0xs2", "50.00", "300", "300"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// L1 and L2 each conserve their per-tx total (300 each), independent of band.
	var l2ok, l1ok, totalok bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT sum(l2_gas_cost_wei) = 300,
		       sum(l1_fee_wei) = 300,
		       sum(l2_gas_cost_wei + l1_fee_wei) = 600
		FROM metrics_gas_daily_v2`).Scan(&l2ok, &l1ok, &totalok))
	require.True(t, l2ok, "L2 component must conserve the per-tx L2 sum (300)")
	require.True(t, l1ok, "L1 component must conserve the per-tx L1 sum (300)")
	require.True(t, totalok, "total settlement cost must be L2+L1 = 600")

	// Per-band L1 split: the 'small' band (50.00) gets exactly one payment's share.
	var smallL1 bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT l1_fee_wei = 100 FROM metrics_gas_daily_v2
		WHERE amount_band='small' AND membership='known'`).Scan(&smallL1))
	require.True(t, smallL1, "small band gets one payment's L1 share (100)")
}
