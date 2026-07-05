//go:build integration

package metrics_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestBuildEconomy_ExcludedBlock(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // known
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac2", "0xp2", "0xs2", "2.00"}, // unknown
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	// Verified window includes only the known payment.
	require.Equal(t, int64(1), page.Windows["all"].TxnCount)
	require.Equal(t, "1.000000", page.Windows["all"].VolumeUSDC)

	// Excluded block captures the non-verified (unknown) payment.
	require.Equal(t, int64(1), page.Excluded.TxnCount)
	require.Equal(t, "2.000000", page.Excluded.VolumeUSDC)
}

func TestBuildEconomy_ExcludedBlockZeroWhenNoneUnknown(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // known only
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	// No unknown rows: excluded should be zero, not nil/error.
	require.Equal(t, int64(0), page.Excluded.TxnCount)
	require.Equal(t, "0.000000", page.Excluded.VolumeUSDC)
}

func TestBuildEconomy_MonthlySeriesCompleteness(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-01-15T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // Jan: starts mid-month
		{"0xb", 0, "2026-02-03T10:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"}, // Feb: complete
		{"0xc", 0, "2026-03-01T10:00:00Z", "0xfac1", "0xp3", "0xs2", "4.00"}, // Mar: cut by data edge
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	econ, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-03-01T00:00:00Z"))
	require.NoError(t, err)

	require.Len(t, econ.MonthlySeries, 3)
	jan, feb, mar := econ.MonthlySeries[0], econ.MonthlySeries[1], econ.MonthlySeries[2]

	require.Equal(t, "2026-01", jan.Month)
	require.False(t, jan.Complete, "data starts Jan 15 — month is cut at the left edge")
	require.Equal(t, int64(1), jan.TxnCount)

	require.Equal(t, "2026-02", feb.Month)
	require.True(t, feb.Complete)
	require.Equal(t, "2.000000", feb.VolumeUSDC)

	require.Equal(t, "2026-03", mar.Month)
	require.False(t, mar.Complete, "data_through_day is Mar 1 — month is cut at the right edge")
}

func TestBuildEconomy_GasL1L2Split(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// L2=1e15 wei ($2), L1=1e15 wei ($2). Single payment, known facilitator.
	seedL1GasPayments(t, ctx, db, []seedL1GasRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "5.00", "1000000000000000", "1000000000000000"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	g := page.Gas.Windows["all"]
	require.Equal(t, int64(1), g.TxnCount)
	require.Equal(t, "0.001000", g.GasETHL2) // 1e15 wei = 0.001 ETH
	require.Equal(t, "0.001000", g.GasETHL1)
	require.Equal(t, "0.002000", g.GasETH) // total l1+l2
}

// TestBuildEconomy_TypicalPaymentAllWindowsPresent verifies that all three
// windows are always present in the typical_payment map even when only a
// subset of windows have rows in metrics_window_stats_v2.  Without
// pre-initialisation a missing window silently disappears from the artifact.
func TestBuildEconomy_TypicalPaymentAllWindowsPresent(t *testing.T) {
	ctx, db, pool := setupMetrics(t)

	// Insert one verified cube row so the asOf assertion in BuildEconomy passes.
	_, err := db.ExecContext(ctx, `
		INSERT INTO metrics_daily_v2
			(day, chain, facilitator, membership, amount_band, methodology_version,
			 txn_count, volume_usdc, max_amount_usdc)
		VALUES ('2026-06-01','base','0xfac1','known','small',1,10,20.000000,5.000000)`)
	require.NoError(t, err)

	// Only seed a '7d' row in window_stats — simulating the case where one
	// window has no verified rows (e.g. data edge or rollup anomaly).
	_, err = db.ExecContext(ctx, `
		INSERT INTO metrics_window_stats_v2
			(window_name, membership, methodology_version, txn_count, median_amount_usdc)
		VALUES ('7d','known',1,5,2.000000)`)
	require.NoError(t, err)

	asOf := mustTime(t, "2026-06-01T00:00:00Z")
	page, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)

	// All three windows must exist in the map regardless of DB row coverage.
	for _, w := range []string{"7d", "30d", "all"} {
		tp, ok := page.TypicalPayment[w]
		require.True(t, ok, "window %q must always be present in typical_payment", w)
		if w != "7d" {
			require.Equal(t, int64(0), tp.TxnCount, "unset window %q must default to zero", w)
		}
	}
}

// TestBuildEconomy_RejectsAsOfMismatch verifies that BuildEconomy returns a
// descriptive error when the caller passes an asOf that differs from the
// cube's verified data edge.  typical_payment and price_points are anchored
// at rollup time; a different asOf would make those windows inconsistent
// with the economy series, gas, and velocity sections.
func TestBuildEconomy_RejectsAsOfMismatch(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac2")
	seedPayments(t, ctx, db, []seedRow{
		{"in", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))
	// cubeMaxDay = "2026-06-08"

	// asOf one day before the cube's verified edge — must error.
	_, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-07T00:00:00Z"))
	require.Error(t, err, "asOf before cubeMaxDay must be rejected")
	require.ErrorContains(t, err, "2026-06-07")
	require.ErrorContains(t, err, "2026-06-08")

	// asOf one day after the cube's verified edge — also must error (window
	// semantics shift: economy 7d anchors at Jun 9 while typical_payment
	// anchors at Jun 8, yielding different lower bounds).
	_, err = metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-09T00:00:00Z"))
	require.Error(t, err, "asOf after cubeMaxDay must also be rejected")
}

func TestBuildEconomy_TypicalPricePointsGasVelocity(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// 1e15 wei @ $2000 test price = $2 gas per payment.
	seedGasPayments(t, ctx, db, []seedGasRow{
		{"0xa", 0, "2026-06-05T10:00:10Z", "0xfac1", "0xp1", "0xs1", "1.00", "1000000000000000"},
		{"0xb", 0, "2026-06-05T10:00:50Z", "0xfac1", "0xp2", "0xs1", "2.00", "1000000000000000"},
		{"0xc", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp3", "0xs2", "9.00", "1000000000000000"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	econ, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	// typical_payment: avg = 12/3 = 4, median = 2 (all payments are verified here).
	tp := econ.TypicalPayment["7d"]
	require.Equal(t, "4.000000", tp.AvgUSDC)
	require.Equal(t, "2.000000", tp.MedianUSDC)
	require.Equal(t, int64(3), tp.TxnCount)
	require.Equal(t, "2.000000", econ.TypicalPayment["all"].MedianUSDC)

	// price_points: three distinct amounts, each once; rank 1 is the smallest
	// (tie broken by amount asc). Share = 1/3.
	pp := econ.PricePoints["all"]
	require.Len(t, pp, 3)
	require.Equal(t, "1.000000", pp[0].AmountUSDC)
	require.Equal(t, int64(1), pp[0].PayeeCount)
	require.Equal(t, "33.33", pp[0].TxnSharePct)

	// gas: 3 payments × $2 = $6 over $12 moved → 50 cents per dollar.
	g := econ.Gas.Windows["all"]
	require.Equal(t, int64(3), g.TxnCount)
	require.Equal(t, "6.00", g.GasUSD)
	require.Equal(t, "0.003000", g.GasETH)
	require.Equal(t, "0.003000", g.GasETHL2, "no L1 fee seeded → total is all L2")
	require.Equal(t, "0.000000", g.GasETHL1)
	require.NotNil(t, g.GasCentsPerDollar)
	require.Equal(t, "50.0000", *g.GasCentsPerDollar)
	require.Equal(t, int64(1), g.BreakevenTxnCount) // the $1.00 payment lost to $2 gas
	require.NotEmpty(t, econ.Gas.Method)

	// velocity: two payments share minute 10:00.
	require.Equal(t, int64(2), econ.Velocity.Windows["7d"].MaxPerMin)
	require.Len(t, econ.Velocity.DailySeries, 1)
	require.Equal(t, int64(2), econ.Velocity.DailySeries[0].MaxPerMin)
}

// TestBuildEconomy_LargestPaymentPerWindow verifies item 6.2:
//   - The per-window largest_payment_usdc reflects only known-membership rows.
//   - The "all" window sees all known rows; a tighter window sees only its range.
//   - An unknown-membership payment with a larger amount must NOT win.
func TestBuildEconomy_LargestPaymentPerWindow(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		// Old known payment: large amount but outside the 7d window.
		{"0xa", 0, "2026-01-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1000.00"},
		// Recent known payment: smaller amount, inside the 7d window.
		{"0xb", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp2", "0xs1", "50.00"},
		// Unknown payment with an even larger amount: must NOT affect the largest stat.
		{"0xc", 0, "2026-06-05T11:00:00Z", "0xfac2", "0xp3", "0xs3", "9999.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// cubeMaxDay = 2026-06-05 (max of known rows).
	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	// "all" window: $1000 (Jan) > $50 (Jun), unknown $9999 excluded.
	tp := page.TypicalPayment["all"]
	require.NotNil(t, tp.LargestPaymentUSDC, "all-window largest must be non-nil when verified rows exist")
	require.Equal(t, "1000.000000", *tp.LargestPaymentUSDC,
		"all-window must find the Jan $1000 known payment; unknown $9999 must not win")

	// "7d" window (2026-05-30 to 2026-06-05): only the Jun $50 is in range.
	tp7 := page.TypicalPayment["7d"]
	require.NotNil(t, tp7.LargestPaymentUSDC, "7d-window largest must be non-nil")
	require.Equal(t, "50.000000", *tp7.LargestPaymentUSDC,
		"7d-window must only see the Jun 5 $50 payment (Jan is out of range)")
}

// TestBuildEconomy_CostDailySeries verifies item 6.4:
//   - The daily cost-per-dollar series is emitted inside gas.
//   - Two days with different cost/volume ratios produce correct per-day values.
//   - Unknown-membership payments (no entry in the gas table's 'known' rows) are excluded.
//   - The last day is marked Complete=false (edge convention).
func TestBuildEconomy_CostDailySeries(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")

	// Day 1 (known): $10 vol, 1e15 wei = $2 cost -> 2/10*100 = 20.0000 cents per dollar
	// Day 2 (known): $100 vol, 1e15 wei = $2 cost -> 2/100*100 = 2.0000 cents per dollar
	// Day 2 (unknown): large amount, excluded from gas table's known rows.
	seedGasPayments(t, ctx, db, []seedGasRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac1", "0xp1", "0xs1", "10.00", "1000000000000000"},
		{"0xb", 0, "2026-06-02T10:00:00Z", "0xfac1", "0xp2", "0xs1", "100.00", "1000000000000000"},
		{"0xc", 0, "2026-06-02T11:00:00Z", "0xfac2", "0xp3", "0xs3", "50.00", "1000000000000000"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-02T00:00:00Z"))
	require.NoError(t, err)

	cd := page.Gas.CostDaily
	require.Len(t, cd, 2, "two known-verified days must appear in cost_daily")

	require.Equal(t, "2026-06-01", cd[0].Day)
	require.True(t, cd[0].Complete, "first day is complete")
	require.Equal(t, "20.0000", cd[0].CentsPerDollar,
		"day 1: $2 gas / $10 vol * 100 = 20 cents per dollar")

	require.Equal(t, "2026-06-02", cd[1].Day)
	require.False(t, cd[1].Complete, "last (edge) day must be marked incomplete")
	require.Equal(t, "2.0000", cd[1].CentsPerDollar,
		"day 2: $2 gas / $100 vol * 100 = 2 cents per dollar (unknown $50 payment excluded)")
}

// TestRebuild_PayerCohorts_Classification verifies item 6.5:
//   - Payers whose first verified payment falls within the window are "new".
//   - Payers whose first verified payment predates the window are "returning".
//   - Unverified payments do not count toward a payer's first-day.
//   - Conservation holds: new_vol + ret_vol == verified window volume.
//   - No "all" key is stored (the window is degenerate for cohort analysis).
func TestRebuild_PayerCohorts_Classification(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// 0xold: verified 2026-01-01 AND 2026-06-05 -> first_verified = 2026-01-01 -> RETURNING for both windows.
	// 0xnew: verified 2026-06-04 -> first_verified = 2026-06-04 -> NEW for both windows.
	// 0xpartial: unverified 2026-01-01 (via 0xfac2, not allowlisted) + verified 2026-06-03 -> first_verified = 2026-06-03 -> NEW for both windows.
	seedPayments(t, ctx, db, []seedRow{
		{"0xtx-old-jan", 0, "2026-01-01T10:00:00Z", "0xfac1", "0xold", "0xs1", "10.00"},
		{"0xtx-old-jun", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xold", "0xs1", "5.00"},
		{"0xtx-new-jun", 0, "2026-06-04T10:00:00Z", "0xfac1", "0xnew", "0xs2", "3.00"},
		{"0xtx-prt-jan", 0, "2026-01-01T12:00:00Z", "0xfac2", "0xpartial", "0xs3", "2.00"}, // unverified
		{"0xtx-prt-jun", 0, "2026-06-03T10:00:00Z", "0xfac1", "0xpartial", "0xs3", "4.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// max verified day = 2026-06-05.
	// 7d lb = 2026-06-05 - 6 = 2026-05-30.
	// 30d lb = 2026-06-05 - 29 = 2026-04-07.

	page, err := metrics.BuildEconomy(ctx, pool, mustTime(t, "2026-06-05T00:00:00Z"))
	require.NoError(t, err)

	require.NotNil(t, page.PayerCohorts, "PayerCohorts must be populated after rollup")
	_, hasAll := page.PayerCohorts["all"]
	require.False(t, hasAll, "PayerCohorts must not contain the 'all' window")
	require.Contains(t, page.PayerCohorts, "7d")
	require.Contains(t, page.PayerCohorts, "30d")

	// 7d window: 0xold is returning ($5), 0xnew and 0xpartial are new ($3+$4=$7).
	c7 := page.PayerCohorts["7d"]
	require.Equal(t, int64(2), c7.NewPayers, "7d: 0xnew + 0xpartial are new")
	require.Equal(t, int64(1), c7.ReturningPayers, "7d: only 0xold is returning")
	require.Equal(t, "7.000000", c7.NewPayerVolumeUSDC, "7d: new vol = $3 + $4")
	require.Equal(t, "5.000000", c7.ReturningPayerVolumeUSDC, "7d: returning vol = $5")

	// 30d window: same classification because all first-verified days are unchanged.
	c30 := page.PayerCohorts["30d"]
	require.Equal(t, int64(2), c30.NewPayers, "30d: 0xnew + 0xpartial are new")
	require.Equal(t, int64(1), c30.ReturningPayers, "30d: only 0xold is returning")

	// Conservation: new_vol + ret_vol must equal the verified window volume.
	for _, wn := range []string{"7d", "30d"} {
		c := page.PayerCohorts[wn]
		windowVol := page.Windows[wn].VolumeUSDC
		newVol, err2 := decimal.NewFromString(c.NewPayerVolumeUSDC)
		require.NoError(t, err2)
		retVol, err3 := decimal.NewFromString(c.ReturningPayerVolumeUSDC)
		require.NoError(t, err3)
		sum := newVol.Add(retVol).StringFixed(6)
		require.Equal(t, windowVol, sum,
			"window %s: new_vol + ret_vol must equal verified window volume", wn)
	}
}
