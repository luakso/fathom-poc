//go:build integration

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestBuildEconomy_WindowAndAttribution(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00"}, // agentic small, in 7d
		{"0xb", 0, "2026-06-08T11:00:00Z", "0xfac1", "0xp2", "0xs1", "3.00"}, // agentic small, in 7d
		{"0xc", 0, "2026-05-01T09:00:00Z", "0xfac2", "0xp3", "0xs2", "5.00"}, // contested, only in 'all'
	})
	require.NoError(t, metrics.RebuildDaily(ctx, pool))

	// asOf pins "now" so the 7d/30d windows are deterministic in tests.
	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)

	require.Equal(t, int64(2), econ.Windows["7d"].TxnCount)
	require.Equal(t, "5.000000", econ.Windows["7d"].VolumeUSDC) // 2 + 3
	require.Equal(t, int64(3), econ.Windows["all"].TxnCount)
	require.Equal(t, int64(2), econ.Windows["7d"].ByAttribution["agentic"].TxnCount)
	require.Len(t, econ.DailySeries, 2) // two distinct days seeded
}

func TestBuildEconomy_WindowIsSevenDaysInclusive(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		{"in", 0, "2026-06-03T12:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00"},  // exactly 7th day back from 06-09 → included
		{"out", 0, "2026-06-02T12:00:00Z", "0xfac2", "0xp2", "0xs1", "1.00"}, // 8th day back → excluded
	})
	require.NoError(t, metrics.RebuildDaily(ctx, pool))
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
	require.NoError(t, metrics.RebuildDaily(ctx, pool))

	page, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	// Ranked by 'all'-window volume, fac1 (10) before fac2 (1).
	require.Equal(t, "0xfac1", page.Rows[0].Facilitator)
	require.Equal(t, "10.000000", page.Rows[0].VolumeUSDC)
	require.Equal(t, "agentic", page.Rows[0].Attribution) // fac1 is allowlisted
}

func TestBuildEconomy_AsOfBoundsAbove(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	seedPayments(t, ctx, db, []seedRow{
		{"in", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00"},
		{"future", 0, "2026-06-15T10:00:00Z", "0xfac2", "0xp2", "0xs1", "1.00"}, // after asOf → excluded everywhere
	})
	require.NoError(t, metrics.RebuildDaily(ctx, pool))

	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	econ, err := metrics.BuildEconomy(ctx, pool, asOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), econ.Windows["7d"].TxnCount, "7d must not include days after asOf")
	require.Equal(t, int64(1), econ.Windows["all"].TxnCount, "'all' is as-of asOf, not beyond it")
	require.Len(t, econ.DailySeries, 1, "daily series shares the asOf upper bound")
}

func TestBuildFacilitators_AttributionTieIsDeterministic(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	// Equal volume under two attributions: without a tie-break the label could
	// flip between emits over unchanged data. Seed the cube directly — the tie
	// is a property of the rollup output, not of the payments pipeline.
	_, err := db.ExecContext(ctx, `
		INSERT INTO metrics_daily_v1
			(day, chain, facilitator, attribution, amount_band, methodology_version, txn_count, volume_usdc, max_amount_usdc)
		VALUES
			('2026-06-08','base','0xfac1','agentic',  'small',1,1,5.000000,5.000000),
			('2026-06-08','base','0xfac1','contested','small',1,1,5.000000,5.000000)`)
	require.NoError(t, err)

	page, err := metrics.BuildFacilitators(ctx, pool)
	require.NoError(t, err)
	require.Len(t, page.Rows, 1)
	require.Equal(t, "agentic", page.Rows[0].Attribution, "volume tie must resolve by attribution order, deterministically")
}
