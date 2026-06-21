//go:build integration

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

// A settlement carrying only valid_before (no valid_after), settled after that
// bound, must NOT count toward expired — expired/not_yet_valid are gated on the
// windowed (both-bounds) subset so the numerator can never exceed windowed_count,
// the rate denominator. Guards against a published rate > 1.
func TestRebuildReliability_PartialWindowNotCountedExpired(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedWindowedPayments(t, ctx, db, []seedWindowedRow{
		// only valid_before set, settled AFTER it — "expired" only if NOT gated on both bounds
		{"0xa", 0, "2026-06-10T12:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", "", "2026-06-10T11:00:00Z"},
		// a proper fully-windowed, in-window row so windowed_count > 0
		{"0xb", 0, "2026-06-10T10:30:00Z", "0xfac1", "0xp2", "0xs1", "2.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var settle, windowed, expired int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT settlement_count, windowed_count, expired_count
		FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='all'`).
		Scan(&settle, &windowed, &expired))
	require.Equal(t, int64(2), settle)
	require.Equal(t, int64(1), windowed, "only 0xb carries both bounds")
	require.Equal(t, int64(0), expired, "partial-window 0xa must NOT count as expired")
	require.LessOrEqual(t, expired, windowed, "numerator must stay within the windowed denominator")
}

// Windowed-subset latency, expired/not-yet-valid counts, and the all==known+unknown
// reconciliation, all computed on a tiny hand-checkable fixture anchored to 2026-06-10.
func TestRebuildReliability_WindowStats(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1") // known
	seedWindowedPayments(t, ctx, db, []seedWindowedRow{
		// known: fast 5s latency, valid window, settled inside it
		{"0xa", 0, "2026-06-10T10:00:05Z", "0xfac1", "0xp1", "0xs1", "1.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
		// known: slow 2h latency (7200s → >10m bucket), settled AFTER valid_before (expired)
		{"0xb", 0, "2026-06-10T12:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
		// known: window-less (NULL window) — counts as a settlement, excluded from windowed/latency
		{"0xc", 0, "2026-06-10T13:00:00Z", "0xfac1", "0xp3", "0xs1", "3.00", "", ""},
		// unknown: not-yet-valid (settled BEFORE valid_after)
		{"0xd", 0, "2026-06-10T09:00:00Z", "0xfac2", "0xp4", "0xs2", "4.00", "2026-06-10T09:30:00Z", "2026-06-10T11:00:00Z"},
		// known: 30s latency → 10_60s bucket
		{"0xe", 0, "2026-06-10T10:00:30Z", "0xfac1", "0xp5", "0xs1", "1.50", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
		// known: 120s latency → 1_10m bucket
		{"0xf", 0, "2026-06-10T10:02:00Z", "0xfac1", "0xp6", "0xs1", "1.75", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
	})
	seedCancellations(t, ctx, db, []seedCancelRow{
		{"0xcx", 0, "0xp9", "2026-06-10T10:30:00Z", "0xfac1"}, // submitter allowlisted → known
	})

	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var settle, windowed, expired, notYet int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT settlement_count, windowed_count, expired_count, not_yet_valid_count
		FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='all'`).
		Scan(&settle, &windowed, &expired, &notYet))
	require.Equal(t, int64(6), settle, "all six payments are settlements")
	require.Equal(t, int64(5), windowed, "five carry a full auth window (0xc is window-less)")
	require.Equal(t, int64(1), expired, "0xb settled after valid_before")
	require.Equal(t, int64(1), notYet, "0xd settled before valid_after")

	var knownS, unknownS int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='known'`).Scan(&knownS))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='unknown'`).Scan(&unknownS))
	require.Equal(t, settle, knownS+unknownS, "membership must reconcile to 'all'")
	require.Equal(t, int64(5), knownS)
	require.Equal(t, int64(1), unknownS)

	var sub1, b110s, b1060s, b110m, gt10m int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT lat_bucket_sub1s, lat_bucket_1_10s, lat_bucket_10_60s, lat_bucket_1_10m, lat_bucket_gt10m
		FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='known'`).
		Scan(&sub1, &b110s, &b1060s, &b110m, &gt10m))
	require.Equal(t, int64(0), sub1)
	require.Equal(t, int64(1), b110s, "0xa: 5s → 1_10s")
	require.Equal(t, int64(1), b1060s, "0xe: 30s → 10_60s")
	require.Equal(t, int64(1), b110m, "0xf: 120s → 1_10m")
	require.Equal(t, int64(1), gt10m, "0xb: 7200s → gt10m")

	// latency p50 for known: percentile_cont(0.5) over {5, 30, 120, 7200} sorted =
	// interpolate between indices 1 and 2 → 30 + 0.5*(120-30) = 75.0
	var p50 float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT latency_p50_s FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='known'`).Scan(&p50))
	require.InDelta(t, 75.0, p50, 1e-6, "p50 of {5s, 30s, 120s, 7200s} via percentile_cont")

	// cancellation_count reconciles across the GROUPING SETS membership rows.
	var cancKnown, cancAll int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cancellation_count FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='known'`).Scan(&cancKnown))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cancellation_count FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='all'`).Scan(&cancAll))
	require.Equal(t, int64(1), cancKnown, "the 0xfac1-submitted cancellation is known")
	require.Equal(t, int64(1), cancAll, "and shows in the 'all' row")
}

func TestBuildReliability_Shape(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedWindowedPayments(t, ctx, db, []seedWindowedRow{
		{"0xa", 0, "2026-06-10T10:00:05Z", "0xfac1", "0xp1", "0xs1", "1.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
		{"0xb", 0, "2026-06-10T12:00:00Z", "0xfac2", "0xp2", "0xs1", "2.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"}, // expired, unknown
	})
	seedCancellations(t, ctx, db, []seedCancelRow{
		{"0xc1", 0, "0xp2", "2026-06-10T12:00:00Z", "0xrelayer"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildReliability(ctx, pool)
	require.NoError(t, err)

	all := page.Windows["all"]
	require.Equal(t, int64(2), all.SettlementCount)
	require.Equal(t, int64(2), all.WindowedCount)
	require.Equal(t, int64(1), all.ExpiredCount)
	require.InDelta(t, 1.0, all.WindowedShare, 1e-9, "2 of 2 carry windows")
	require.Contains(t, all.ByMembership, "known")
	require.Contains(t, all.ByMembership, "unknown")

	require.NotEmpty(t, page.Daily)
	require.Len(t, page.CancellationAttribution.ByPayer, 1)
	require.Equal(t, "0xp2", page.CancellationAttribution.ByPayer[0].Address)
	require.Equal(t, int64(1), page.CancellationAttribution.ByPayer[0].Count)
}

func TestRebuildReliability_Daily(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedWindowedPayments(t, ctx, db, []seedWindowedRow{
		{"0xa", 0, "2026-06-09T10:00:00Z", "0xfac2", "0xp1", "0xs1", "1.00", "2026-06-09T09:00:00Z", "2026-06-09T11:00:00Z"},
		{"0xb", 0, "2026-06-10T10:00:00Z", "0xfac2", "0xp2", "0xs1", "2.00", "2026-06-10T09:00:00Z", "2026-06-10T11:00:00Z"},
	})
	seedCancellations(t, ctx, db, []seedCancelRow{
		{"0xc1", 0, "0xp2", "2026-06-10T12:00:00Z", "0xrelayer"},
	})

	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var day1 int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_reliability_daily_v2 WHERE day='2026-06-09' AND membership='unknown'`).Scan(&day1))
	require.Equal(t, int64(1), day1)

	var cancel int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cancellation_count FROM metrics_reliability_daily_v2 WHERE day='2026-06-10' AND membership='unknown'`).Scan(&cancel))
	require.Equal(t, int64(1), cancel, "the 2026-06-10 cancellation joins to that day's unknown row")

	var dailySum, windowAll int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT coalesce(sum(settlement_count),0) FROM metrics_reliability_daily_v2`).Scan(&dailySum))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_reliability_window_v2 WHERE window_name='all' AND membership='all'`).Scan(&windowAll))
	require.Equal(t, windowAll, dailySum, "daily settlements must sum to the all-window total")
}
