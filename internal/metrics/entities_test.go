//go:build integration

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestRebuildEntities_TopNUnionAndFingerprint(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xS1", "100.00"},
		{"0xb", 0, "2026-06-05T10:01:00Z", "0xfac1", "0xp2", "0xS1", "100.00"},
		{"0xc", 0, "2026-06-05T10:02:00Z", "0xfac1", "0xp3", "0xS1", "50.00"},
		{"0xd", 0, "2026-06-05T10:03:00Z", "0xfac1", "0xq1", "0xSINK", "0.001"},
		{"0xe", 0, "2026-06-05T10:04:00Z", "0xfac1", "0xq1", "0xSINK", "0.001"},
		{"0xf", 0, "2026-06-05T10:05:00Z", "0xfac1", "0xq1", "0xSINK", "0.001"},
		{"0xg", 0, "2026-06-05T10:06:00Z", "0xfac1", "0xq1", "0xSINK", "0.001"},
		{"0xh", 0, "2026-06-05T10:07:00Z", "0xfac1", "0xq1", "0xSINK", "0.001"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var dp, da, txns int64
	var vol string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT distinct_counterparties, distinct_amounts, txn_count, volume_usdc::text
		FROM entity_rank_v1 WHERE window_name='all' AND role='payee' AND address='0xS1'`).
		Scan(&dp, &da, &txns, &vol))
	require.Equal(t, int64(3), dp)
	require.Equal(t, int64(2), da)
	require.Equal(t, int64(3), txns)
	require.Equal(t, "250.000000", vol)

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT txn_count FROM entity_rank_v1
		WHERE window_name='all' AND role='payee' AND address='0xSINK'`).Scan(&txns))
	require.Equal(t, int64(5), txns)
}

func TestRebuildEntities_KnownVolumeSplit(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xS1", "100.00"},
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac2", "0xp2", "0xS1", "40.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var vol, known string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT volume_usdc::text, known_volume_usdc::text
		FROM entity_rank_v1 WHERE window_name='all' AND role='payee' AND address='0xS1'`).
		Scan(&vol, &known))
	require.Equal(t, "140.000000", vol)
	require.Equal(t, "100.000000", known)
}

func TestRebuildEntities_BucketsAndConcentrationConserve(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xS1", "100.00"},
		{"0xb", 0, "2026-06-05T10:01:00Z", "0xfac1", "0xp1", "0xS2", "5.00"},
		{"0xc", 0, "2026-06-05T10:02:00Z", "0xfac1", "0xp2", "0xS2", "5.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var bucketEntities int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT sum(entity_count) FROM entity_buckets_v1
		WHERE window_name='all' AND role='payee'`).Scan(&bucketEntities))
	require.Equal(t, int64(2), bucketEntities)

	var te, tt int64
	var tv string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT total_entities, total_volume::text, total_txns FROM entity_concentration_v1
		WHERE window_name='all' AND role='payee'`).Scan(&te, &tv, &tt))
	require.Equal(t, int64(2), te)
	require.Equal(t, "110.000000", tv)
	require.Equal(t, int64(3), tt)

	var cubeVol string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT sum(volume_usdc)::text FROM metrics_daily_v2`).Scan(&cubeVol))
	require.Equal(t, cubeVol, tv)
}

func TestRebuildEntities_BucketBoundaries(t *testing.T) {
	ctx, db, _ := setupMetrics(t)
	cases := []struct {
		n    int64
		want string
	}{
		{1, "1"},
		{2, "2-10"},
		{10, "2-10"},
		{11, "11-100"},
		{100, "11-100"},
		{101, "101-1k"},
		{1000, "101-1k"},
		{1001, "1k-100k"},
		{100000, "1k-100k"},
		{100001, "100k+"},
	}
	for _, c := range cases {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT entity_txn_bucket($1::bigint)`, c.n).Scan(&got))
		require.Equal(t, c.want, got, "entity_txn_bucket(%d)", c.n)
	}
}

func TestRebuildEntities_WindowAnchoring(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xRECENT", "1.00"},
		{"0xb", 0, "2026-04-26T10:00:00Z", "0xfac1", "0xp2", "0xOLD", "1.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var inAll, in30 int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM entity_rank_v1 WHERE role='payee' AND address='0xOLD' AND window_name='all'`).Scan(&inAll))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM entity_rank_v1 WHERE role='payee' AND address='0xOLD' AND window_name='30d'`).Scan(&in30))
	require.Equal(t, int64(1), inAll)
	require.Equal(t, int64(0), in30)
}
