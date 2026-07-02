//go:build integration

package metrics_test

import (
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestBuildEconomy_WindowVerifiedOnly(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00"}, // known small, in 7d
		{"0xb", 0, "2026-06-08T11:00:00Z", "0xfac1", "0xp2", "0xs1", "3.00"}, // known small, in 7d
		{"0xc", 0, "2026-05-01T09:00:00Z", "0xfac2", "0xp3", "0xs2", "5.00"}, // unknown: filtered out
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// asOf pins "now" so the 7d/30d windows are deterministic in tests.
	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)

	require.Equal(t, int64(2), econ.Windows["7d"].TxnCount)
	require.Equal(t, "5.000000", econ.Windows["7d"].VolumeUSDC) // 2 + 3 (known only)
	require.Equal(t, int64(2), econ.Windows["all"].TxnCount)    // unknown payment filtered
	require.Len(t, econ.DailySeries, 1)                         // only Jun 8 (May 1 unknown filtered)
}

func TestBuildEconomy_WindowIsSevenDaysInclusive(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac2")
	seedPayments(t, ctx, db, []seedRow{
		{"in", 0, "2026-06-03T12:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00"},  // exactly 7th day back from 06-09 → included
		{"out", 0, "2026-06-02T12:00:00Z", "0xfac2", "0xp2", "0xs1", "1.00"}, // 8th day back → excluded
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))
	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), econ.Windows["7d"].TxnCount, "7d must include 06-03 but exclude 06-02")
}

func TestBuildFacilitators_TopN(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "10.00"},
		{"0xb", 0, "2026-06-08T11:00:00Z", "0xfac2", "0xp2", "0xs1", "1.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	// Only fac1 (allowlisted/known) appears; fac2 is excluded by the HAVING filter.
	require.Len(t, page.Rows, 1)
	require.Equal(t, "0xfac1", page.Rows[0].Facilitator)
	require.Equal(t, "10.000000", page.Rows[0].VolumeUSDC)
	require.True(t, page.Rows[0].FacilitatorKnown)
}

func TestBuildEconomy_AsOfBoundsAbove(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac2")
	seedPayments(t, ctx, db, []seedRow{
		{"in", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00"},
		{"future", 0, "2026-06-15T10:00:00Z", "0xfac2", "0xp2", "0xs1", "1.00"}, // after asOf → excluded everywhere
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), econ.Windows["7d"].TxnCount, "7d must not include days after asOf")
	require.Equal(t, int64(1), econ.Windows["all"].TxnCount, "'all' is as-of asOf, not beyond it")
	require.Len(t, econ.DailySeries, 1, "daily series shares the asOf upper bound")
}

func TestBuildFacilitators_VerifiedOnly(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	// Only facilitators with at least one 'known' cell appear. Volume and count
	// reflect only the known cells so the artifact is purely verified activity.
	// 0xfac2 has no 'known' cell and must be excluded entirely.
	_, err := db.ExecContext(ctx, `
		INSERT INTO metrics_daily_v2
			(day, chain, facilitator, membership, amount_band, methodology_version, txn_count, volume_usdc, max_amount_usdc)
		VALUES
			('2026-06-08','base','0xfac1','known',  'small',1,1,5.000000,5.000000),
			('2026-06-08','base','0xfac1','unknown','small',1,1,5.000000,5.000000),
			('2026-06-08','base','0xfac2','unknown','small',1,1,3.000000,3.000000)`)
	require.NoError(t, err)

	page, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	require.Len(t, page.Rows, 1)
	require.Equal(t, "0xfac1", page.Rows[0].Facilitator)
	require.True(t, page.Rows[0].FacilitatorKnown)
}

// TestBuildFacilitators_TotalsConservation verifies that the top-level totals
// block equals both the sum of rows and the economy all-window measure (same
// verified-only data, same DB snapshot).
func TestBuildFacilitators_TotalsConservation(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	allowlist(t, ctx, db, "0xfac2")
	allowlist(t, ctx, db, "0xfac3")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "10.00"},
		{"0xb", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp2", "0xs2", "5.00"},
		{"0xc", 0, "2026-06-08T10:00:00Z", "0xfac3", "0xp3", "0xs3", "3.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	fac, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)

	// rows must sum to totals
	var sumTxns int64
	sumVol := decimal.Zero
	for _, r := range fac.Rows {
		sumTxns += r.TxnCount
		v, err2 := decimal.NewFromString(r.VolumeUSDC)
		require.NoError(t, err2)
		sumVol = sumVol.Add(v)
	}
	require.Equal(t, sumTxns, fac.Totals.TxnCount, "totals.txn_count must equal sum of rows")
	require.Equal(t, sumVol.StringFixed(6), fac.Totals.VolumeUSDC, "totals.volume_usdc must equal sum of rows")

	// totals must conserve with economy all-window (verified-only)
	require.Equal(t, econ.Windows["all"].TxnCount, fac.Totals.TxnCount,
		"facilitator totals must conserve with economy all-window txn_count")
	require.Equal(t, econ.Windows["all"].VolumeUSDC, fac.Totals.VolumeUSDC,
		"facilitator totals must conserve with economy all-window volume_usdc")
}

// TestBuildFacilitators_NoRowCap seeds 101 known facilitators and asserts all
// 101 appear in the result — verifying that LIMIT 100 has been removed.
func TestBuildFacilitators_NoRowCap(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	var rows []seedRow
	for i := 0; i < 101; i++ {
		addr := fmt.Sprintf("0xfac%03d", i)
		allowlist(t, ctx, db, addr)
		rows = append(rows, seedRow{
			txHash:      fmt.Sprintf("0x%04x", i),
			logIndex:    0,
			ts:          "2026-06-08T10:00:00Z",
			facilitator: addr,
			payer:       "0xpayer",
			payee:       "0xpayee",
			amountUSDC:  "1.00",
		})
	}
	seedPayments(t, ctx, db, rows)
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	require.Len(t, page.Rows, 101, "all 101 facilitators must appear; LIMIT 100 must be removed")
	require.Equal(t, int64(101), page.Totals.TxnCount, "totals must reflect all 101 rows")
}
