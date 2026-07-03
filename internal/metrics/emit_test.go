//go:build integration

package metrics_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
		Scope              string `json:"scope"`
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
	require.Equal(t, "verified-x402", doc.Scope)
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
		ID: "c1", Source: "Report", SourceURL: "https://example.com/report",
		ClaimText: "169M+ payments", ClaimedValue: "169000000", ClaimedUnit: "transactions",
		MeasuredMetric: "total_txns_all", Lens: "verified x402 payments",
	}}
	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, claims))

	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	var doc struct {
		Data struct {
			MonthlySeries []struct {
				Month    string `json:"month"`
				Complete bool   `json:"complete"`
			} `json:"monthly_series"`
			TypicalPayment map[string]struct {
				MedianUSDC string `json:"median_usdc"`
			} `json:"typical_payment"`
			PricePoints map[string][]struct {
				AmountUSDC  string `json:"amount_usdc"`
				TxnSharePct string `json:"txn_share_pct"`
			} `json:"price_points"`
			Gas struct {
				Method  map[string]string `json:"method"`
				Windows map[string]struct {
					GasUSD            string  `json:"gas_usd"`
					GasCentsPerDollar *string `json:"gas_cents_per_dollar"`
				} `json:"windows"`
			} `json:"gas"`
			Velocity struct {
				Windows map[string]struct {
					MaxPerMin int64 `json:"max_per_min"`
				} `json:"windows"`
				DailySeries []struct {
					Day string `json:"day"`
				} `json:"daily_series"`
			} `json:"velocity"`
			Claims []struct {
				ID            string `json:"id"`
				MeasuredValue string `json:"measured_value"`
				MeasuredUnit  string `json:"measured_unit"`
				Lens          string `json:"lens"`
			} `json:"claims"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "2026-06", doc.Data.MonthlySeries[0].Month)
	require.Equal(t, "2.000000", doc.Data.TypicalPayment["all"].MedianUSDC)
	require.Equal(t, "2.000000", doc.Data.PricePoints["all"][0].AmountUSDC)
	require.NotEmpty(t, doc.Data.Gas.Method)
	require.Equal(t, int64(1), doc.Data.Velocity.Windows["all"].MaxPerMin)
	require.Len(t, doc.Data.Claims, 1)
	require.Equal(t, "1", doc.Data.Claims[0].MeasuredValue)

	gw := doc.Data.Gas.Windows["all"]
	require.Equal(t, "0.00", gw.GasUSD)
	require.NotNil(t, gw.GasCentsPerDollar)
	require.Equal(t, "0.0000", *gw.GasCentsPerDollar)
	require.Len(t, doc.Data.Velocity.DailySeries, 1)
	require.Equal(t, "2026-06-08", doc.Data.Velocity.DailySeries[0].Day)
	require.False(t, doc.Data.MonthlySeries[0].Complete, "single mid-month data day cannot be a complete month")
	require.Equal(t, "100.00", doc.Data.PricePoints["all"][0].TxnSharePct)
	require.Equal(t, "transactions", doc.Data.Claims[0].MeasuredUnit)
	require.Equal(t, "verified x402 payments", doc.Data.Claims[0].Lens, "lens must flow through to emitted claim")
}

func TestEmit_WritesSiteFiles(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac2") // must be known so cubeStamp finds a verified day
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp1", "0xs1", "2.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	// The page ships with the data.
	idx, err := os.ReadFile(filepath.Join(dir, "index.html"))
	require.NoError(t, err)
	require.Contains(t, string(idx), `src="assets/js/app.js"`)

	// Every assets/ path referenced by index.html resolves to an emitted file.
	refs := regexp.MustCompile(`(?:src|href)="(assets/[^"]+)"`).FindAllStringSubmatch(string(idx), -1)
	require.NotEmpty(t, refs)
	for _, m := range refs {
		st, err := os.Stat(filepath.Join(dir, m[1]))
		require.NoError(t, err, "referenced asset %s must be emitted", m[1])
		require.NotZero(t, st.Size(), "asset %s must be non-empty", m[1])
	}

	// EVERY file in the embedded site ships, byte-for-byte present and non-empty
	// (catches a deleted font or JS module that index.html doesn't reference
	// directly — ES-module imports and @font-face urls are invisible to the
	// index.html regex above).
	err = fs.WalkDir(os.DirFS("../../web/sonar/app"), ".", func(path string, d fs.DirEntry, werr error) error {
		require.NoError(t, werr)
		if d.IsDir() {
			return nil
		}
		st, serr := os.Stat(filepath.Join(dir, path))
		require.NoError(t, serr, "site file %s must be emitted", path)
		require.NotZero(t, st.Size(), "site file %s must be non-empty", path)
		return nil
	})
	require.NoError(t, err)

	// Second emit overwrites cleanly (idempotent).
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))
}

func TestEmit_WritesEntityPages(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac2") // must be known so cubeStamp finds a verified day
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp1", "0xs1", "2.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	for _, page := range []struct{ html, script string }{
		{"payees.html", "assets/js/payees/app.js"},
		{"payers.html", "assets/js/payers/app.js"},
		{"reliability.html", "assets/js/reliability/app.js"},
		{"mechanics.html", "assets/js/mechanics/app.js"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, page.html))
		require.NoError(t, err, "%s must be emitted", page.html)
		require.Contains(t, string(b), `src="`+page.script+`"`)
		st, err := os.Stat(filepath.Join(dir, page.script))
		require.NoError(t, err, "%s must be emitted", page.script)
		require.NotZero(t, st.Size())
		require.Contains(t, string(b), `href="index.html"`)
	}
	idx, err := os.ReadFile(filepath.Join(dir, "index.html"))
	require.NoError(t, err)
	require.Contains(t, string(idx), `href="payees.html"`)
	require.Contains(t, string(idx), `href="reliability.html"`)
	require.Contains(t, string(idx), `href="mechanics.html"`)
}

// TestEmit_StampsKnownDayWhenNewestDayUnknownOnly verifies that data_through_day
// reflects the newest KNOWN (verified) payment day, not the newest day overall.
// If the dataset's newest day contains only unknown-membership rows, the stamp
// must not advance past the last known day - otherwise the dashboard would claim
// to cover a day its own verified series never reaches.
func TestEmit_StampsKnownDayWhenNewestDayUnknownOnly(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // known, Jun 1
		{"0xb", 0, "2026-06-08T10:00:00Z", "0xfac2", "0xp2", "0xs2", "2.00"}, // unknown, Jun 8 (newer)
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	var doc struct {
		DataThroughDay string `json:"data_through_day"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "2026-06-01", doc.DataThroughDay,
		"data_through_day must be the newest KNOWN day, not the newest unknown day")
}

func TestEmit_WritesReliability(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedWindowedPayments(t, ctx, db, []seedWindowedRow{
		{"0xa", 0, "2026-06-10T10:00:05Z", "0xfac1", "0xp1", "0xs1", "1.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
		{"0xb", 0, "2026-06-10T12:00:00Z", "0xfac2", "0xp2", "0xs1", "2.00", "2026-06-10T10:00:00Z", "2026-06-10T11:00:00Z"},
	})
	seedCancellations(t, ctx, db, []seedCancelRow{
		{"0xc1", 0, "0xp2", "2026-06-10T12:00:00Z", "0xrelayer"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "reliability.json"))
	require.NoError(t, err)

	var doc struct {
		MethodologyVersion int    `json:"methodology_version"`
		DataThroughDay     string `json:"data_through_day"`
		Data               struct {
			Windows map[string]struct {
				SettlementCount int64   `json:"settlement_count"`
				WindowedShare   float64 `json:"windowed_share"`
			} `json:"windows"`
			Daily                   []map[string]any `json:"daily"`
			CancellationAttribution struct {
				ByPayer []map[string]any `json:"by_payer"`
			} `json:"cancellation_attribution"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))

	require.Equal(t, 1, doc.MethodologyVersion)
	require.Equal(t, "2026-06-10", doc.DataThroughDay)

	// Headline is the verified (known) slice: only 0xa (0xfac1) is known.
	const allKnownSettlement = int64(1)
	all := doc.Data.Windows["all"]
	require.Equal(t, allKnownSettlement, all.SettlementCount) // headline is the verified slice
	require.NotEmpty(t, doc.Data.Daily)
	// Cancellation submitter 0xrelayer is not allowlisted → filtered by facilitator_known.
	require.Empty(t, doc.Data.CancellationAttribution.ByPayer)
}

// TestEmit_TypicalPaymentPercentiles verifies that economy.json carries
// p10_usdc/p90_usdc/p99_usdc alongside the existing median in each
// typical_payment window entry.
func TestEmit_TypicalPaymentPercentiles(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// Three amounts on the anchor day so percentiles are non-trivial.
	// p10 disc = 1st of 3 = 0.01, median = 2nd = 0.10, p90/p99 disc = 3rd = 5.00.
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "0.01"},
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac1", "0xp2", "0xs1", "0.10"},
		{"0xc", 0, "2026-06-05T12:00:00Z", "0xfac1", "0xp3", "0xs1", "5.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)

	var doc struct {
		Data struct {
			TypicalPayment map[string]struct {
				MedianUSDC string  `json:"median_usdc"`
				P10USDC    *string `json:"p10_usdc"`
				P90USDC    *string `json:"p90_usdc"`
				P99USDC    *string `json:"p99_usdc"`
			} `json:"typical_payment"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))

	tp := doc.Data.TypicalPayment["all"]
	require.Equal(t, "0.100000", tp.MedianUSDC, "median unchanged by percentile addition")
	require.NotNil(t, tp.P10USDC, "p10_usdc must be present")
	require.NotNil(t, tp.P90USDC, "p90_usdc must be present")
	require.NotNil(t, tp.P99USDC, "p99_usdc must be present")
	require.Equal(t, "0.010000", *tp.P10USDC)
	require.Equal(t, "5.000000", *tp.P90USDC)
	require.Equal(t, "5.000000", *tp.P99USDC)
}

// TestEmit_ActiveEntitiesSeries verifies that economy.json carries an
// active_entities daily series: day, payer_count, payee_count, and complete.
// The newest (edge) day must be marked complete=false.
func TestEmit_ActiveEntitiesSeries(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		// Day 1: 2 payers, 1 payee.
		{"0xa", 0, "2026-06-01T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"},
		{"0xb", 0, "2026-06-01T11:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00"},
		// Day 2 (edge/newest): 1 payer, 1 payee.
		{"0xc", 0, "2026-06-02T09:00:00Z", "0xfac1", "0xp1", "0xs2", "3.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)

	var doc struct {
		Data struct {
			ActiveEntities []struct {
				Day        string `json:"day"`
				Complete   bool   `json:"complete"`
				PayerCount int64  `json:"payer_count"`
				PayeeCount int64  `json:"payee_count"`
			} `json:"active_entities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Len(t, doc.Data.ActiveEntities, 2, "one point per verified day")

	pt1 := doc.Data.ActiveEntities[0]
	require.Equal(t, "2026-06-01", pt1.Day)
	require.True(t, pt1.Complete, "non-edge day must be complete=true")
	require.Equal(t, int64(2), pt1.PayerCount)
	require.Equal(t, int64(1), pt1.PayeeCount)

	pt2 := doc.Data.ActiveEntities[1]
	require.Equal(t, "2026-06-02", pt2.Day)
	require.False(t, pt2.Complete, "edge (newest) day must be complete=false")
	require.Equal(t, int64(1), pt2.PayerCount)
	require.Equal(t, int64(1), pt2.PayeeCount)
}

// TestEmit_FacilitatorsWindows verifies that facilitators.json carries window
// measures (7d and 30d) on each row, matching the same anchoring as economy.json.
func TestEmit_FacilitatorsWindows(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-08T10:00:00Z", "0xfac1", "0xp1", "0xs1", "2.00"},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "facilitators.json"))
	require.NoError(t, err)
	var doc struct {
		Data struct {
			Rows []struct {
				Facilitator string `json:"facilitator"`
				TxnCount    int64  `json:"txn_count"`
				VolumeUSDC  string `json:"volume_usdc"`
				Windows     map[string]struct {
					TxnCount   int64  `json:"txn_count"`
					VolumeUSDC string `json:"volume_usdc"`
				} `json:"windows"`
			} `json:"rows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Len(t, doc.Data.Rows, 1)
	r := doc.Data.Rows[0]
	require.Equal(t, "0xfac1", r.Facilitator)
	require.NotNil(t, r.Windows)
	// The payment is on Jun 8 = the data edge, which is within the 7d window.
	require.Equal(t, int64(1), r.Windows["7d"].TxnCount, "7d window must include the Jun 8 payment")
	require.Equal(t, int64(1), r.Windows["30d"].TxnCount, "30d window must include the Jun 8 payment")
}
