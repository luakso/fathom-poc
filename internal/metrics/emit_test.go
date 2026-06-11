//go:build integration

package metrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestEmit_WritesStampedFiles(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	// economy.json exists and carries stamps.
	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	var doc struct {
		MethodologyVersion int    `json:"methodology_version"`
		GeneratedAt        string `json:"generated_at"`
		DataThroughDay     string `json:"data_through_day"`
		Data               struct {
			Windows map[string]struct {
				TxnCount int64 `json:"txn_count"`
			} `json:"windows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, 1, doc.MethodologyVersion)
	require.NotEmpty(t, doc.GeneratedAt)
	require.Equal(t, "2026-06-08", doc.DataThroughDay)
	require.Equal(t, int64(1), doc.Data.Windows["all"].TxnCount)
	// Windows anchor to data_through_day, not the wall clock: the 2026-06-08
	// row is inside "7d" no matter when this test runs.
	require.Equal(t, int64(1), doc.Data.Windows["7d"].TxnCount)

	// facilitators.json exists too.
	_, err = os.Stat(filepath.Join(dir, "facilitators.json"))
	require.NoError(t, err)
}

func TestEmit_EmptyCubeErrors(t *testing.T) {
	ctx, _, pool := setupMetrics(t)

	// No rollup has run: emit must refuse rather than publish all-zero artifacts.
	dir := t.TempDir()
	err := metrics.Emit(ctx, pool, dir, nil)
	require.ErrorContains(t, err, "rollup")

	entries, rerr := os.ReadDir(dir)
	require.NoError(t, rerr)
	require.Empty(t, entries, "no artifacts may be written from an empty cube")
}

func TestEmit_EconomySectionsAndClaims(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	claims := []metrics.Claim{{
		ID: "c1", Source: "Report", ClaimText: "169M+ payments",
		ClaimedValue: "169000000", ClaimedUnit: "transactions",
		MeasuredMetric: "agentic_txns_all",
	}}
	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, claims))

	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	var doc struct {
		Data struct {
			MonthlySeries []struct {
				Month string `json:"month"`
			} `json:"monthly_series"`
			TypicalPayment map[string]map[string]struct {
				MedianUSDC string `json:"median_usdc"`
			} `json:"typical_payment"`
			PricePoints map[string][]struct {
				AmountUSDC string `json:"amount_usdc"`
			} `json:"price_points"`
			Gas struct {
				Method map[string]string `json:"method"`
			} `json:"gas"`
			Velocity struct {
				Windows map[string]map[string]struct {
					MaxPerMin int64 `json:"max_per_min"`
				} `json:"windows"`
			} `json:"velocity"`
			Claims []struct {
				ID            string `json:"id"`
				MeasuredValue string `json:"measured_value"`
			} `json:"claims"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "2026-06", doc.Data.MonthlySeries[0].Month)
	require.Equal(t, "2.000000", doc.Data.TypicalPayment["all"]["agentic"].MedianUSDC)
	require.Equal(t, "2.000000", doc.Data.PricePoints["all"][0].AmountUSDC)
	require.NotEmpty(t, doc.Data.Gas.Method)
	require.Equal(t, int64(1), doc.Data.Velocity.Windows["all"]["agentic"].MaxPerMin)
	require.Len(t, doc.Data.Claims, 1)
	require.Equal(t, "1", doc.Data.Claims[0].MeasuredValue)
}
