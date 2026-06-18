//go:build integration

package metrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

func TestEmit_EntityArtifactsAndConcentration(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xS1", "100.00"},
		{"0xb", 0, "2026-06-05T10:01:00Z", "0xfac1", "0xp2", "0xS2", "5.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	var doc struct {
		MethodologyVersion int `json:"methodology_version"`
		Data               struct {
			Role    string `json:"role"`
			Windows map[string]struct {
				Leaderboard []struct {
					Address    string `json:"address"`
					VolumeUSDC string `json:"volume_usdc"`
				} `json:"leaderboard"`
				Concentration struct {
					TotalEntities int64  `json:"total_entities"`
					TotalVolume   string `json:"total_volume"`
				} `json:"concentration"`
			} `json:"windows"`
		} `json:"data"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "payees.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "payee", doc.Data.Role)
	require.Equal(t, 1, doc.MethodologyVersion)
	all := doc.Data.Windows["all"]
	require.Equal(t, "0xS1", all.Leaderboard[0].Address)
	require.Equal(t, int64(2), all.Concentration.TotalEntities)
	require.Equal(t, "105.000000", all.Concentration.TotalVolume)

	_, err = os.Stat(filepath.Join(dir, "payers.json"))
	require.NoError(t, err)

	var econ struct {
		Data struct {
			Concentration struct {
				Windows map[string]map[string]struct {
					TotalEntities int64 `json:"total_entities"`
				} `json:"windows"`
			} `json:"concentration"`
		} `json:"data"`
	}
	eb, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(eb, &econ))
	require.Equal(t, int64(2), econ.Data.Concentration.Windows["all"]["payee"].TotalEntities)
	require.Contains(t, econ.Data.Concentration.Windows["all"], "payer")
}

func TestRebuildEntities_Top10CutoffStrictSubset(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	rows := make([]seedRow, 0, 12)
	// S01=12.00 .. S12=1.00, one payment each, distinct payers.
	names := []string{"S01", "S02", "S03", "S04", "S05", "S06", "S07", "S08", "S09", "S10", "S11", "S12"}
	for i, n := range names {
		amt := 12 - i // 12,11,...,1
		rows = append(rows, seedRow{
			txHash:      "0x" + n,
			logIndex:    0,
			ts:          "2026-06-05T10:00:00Z",
			facilitator: "0xfac1",
			payer:       "0xp" + n,
			payee:       "0x" + n,
			amountUSDC:  fmtAmt2(amt),
		})
	}
	seedPayments(t, ctx, db, rows)
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var totalEntities, totalTxns int64
	var totalVol, top10Vol, top100Vol string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT total_entities, total_txns, total_volume::text, top10_volume::text, top100_volume::text
		FROM entity_concentration_v1 WHERE window_name='all' AND role='payee'`).
		Scan(&totalEntities, &totalTxns, &totalVol, &top10Vol, &top100Vol))
	require.Equal(t, int64(12), totalEntities)
	require.Equal(t, int64(12), totalTxns)
	require.Equal(t, "78.000000", totalVol)
	require.Equal(t, "75.000000", top10Vol)  // sum of the 10 largest (12..3), strict subset of total
	require.Equal(t, "78.000000", top100Vol) // only 12 entities → top100 == total
}

// fmtAmt2 renders an int dollar amount as a 2dp decimal string for seedRow.
func fmtAmt2(n int) string {
	return strconv.Itoa(n) + ".00"
}
