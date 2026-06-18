//go:build integration

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestBuildEconomy_MonthlySeriesCompleteness(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-01-15T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // Jan: starts mid-month
		{"0xb", 0, "2026-02-03T10:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"}, // Feb: complete
		{"0xc", 0, "2026-03-01T10:00:00Z", "0xfac2", "0xp3", "0xs2", "4.00"}, // Mar: cut by data edge
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
	require.Equal(t, "2.000000", feb.ByMembership["known"].VolumeUSDC)

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

	g := page.Gas.Windows["all"].ByMembership["known"]
	require.Equal(t, int64(1), g.TxnCount)
	require.Equal(t, "0.001000", g.GasETHL2) // 1e15 wei = 0.001 ETH
	require.Equal(t, "0.001000", g.GasETHL1)
	require.Equal(t, "0.002000", g.GasETH) // total l1+l2
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

	// typical_payment: avg = 12/3 = 4, median = 2 (known and overall equal here).
	tp := econ.TypicalPayment["7d"]["known"]
	require.Equal(t, "4.000000", tp.AvgUSDC)
	require.Equal(t, "2.000000", tp.MedianUSDC)
	require.Equal(t, int64(3), tp.TxnCount)
	require.Equal(t, "2.000000", econ.TypicalPayment["all"]["all"].MedianUSDC)

	// price_points: three distinct amounts, each once; rank 1 is the smallest
	// (tie broken by amount asc). Share = 1/3.
	pp := econ.PricePoints["all"]
	require.Len(t, pp, 3)
	require.Equal(t, "1.000000", pp[0].AmountUSDC)
	require.Equal(t, int64(1), pp[0].PayeeCount)
	require.Equal(t, "33.33", pp[0].TxnSharePct)

	// gas: 3 payments × $2 = $6 over $12 moved → 50 cents per dollar.
	g := econ.Gas.Windows["all"].ByMembership["known"]
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
	require.Equal(t, int64(2), econ.Velocity.Windows["7d"]["known"].MaxPerMin)
	require.Len(t, econ.Velocity.DailySeries, 1)
	require.Equal(t, int64(2), econ.Velocity.DailySeries[0].MaxPerMin)
}
