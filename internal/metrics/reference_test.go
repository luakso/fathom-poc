package metrics_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadETHPrices_Valid(t *testing.T) {
	p := writeTemp(t, "eth.json", `{
		"source": "DefiLlama weekly average",
		"unit": "USD per ETH",
		"prices": {"2026-01-05": "3157.17", "2026-01-12": "3261.55"}
	}`)
	prices, err := metrics.LoadETHPrices(p)
	require.NoError(t, err)
	require.Equal(t, "DefiLlama weekly average", prices.Source)
	require.Equal(t, "3157.17", prices.Prices["2026-01-05"].String())
	require.Len(t, prices.Prices, 2)
}

func TestLoadETHPrices_Rejects(t *testing.T) {
	cases := []struct{ name, body, wantErr string }{
		{"bad date", `{"source":"s","unit":"u","prices":{"2026-13-01":"1"}}`, "2026-13-01"},
		{"non-monday", `{"source":"s","unit":"u","prices":{"2026-01-06":"1"}}`, "Monday"},
		{"bad decimal", `{"source":"s","unit":"u","prices":{"2026-01-05":"abc"}}`, "abc"},
		{"non-positive", `{"source":"s","unit":"u","prices":{"2026-01-05":"0"}}`, "positive"},
		{"empty prices", `{"source":"s","unit":"u","prices":{}}`, "no prices"},
		{"missing source", `{"source":"","unit":"u","prices":{"2026-01-05":"1"}}`, "source"},
		{"duplicate key", `{"source":"s","unit":"u","prices":{"2026-01-05":"1","2026-01-05":"2"}}`, "duplicate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := metrics.LoadETHPrices(writeTemp(t, "eth.json", c.body))
			require.ErrorContains(t, err, c.wantErr)
		})
	}
}

func TestLoadETHPrices_MissingFile(t *testing.T) {
	_, err := metrics.LoadETHPrices(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
}

func TestLoadClaims_MissingFile(t *testing.T) {
	_, err := metrics.LoadClaims(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
}

func TestLoadETHPrices_MalformedJSON(t *testing.T) {
	_, err := metrics.LoadETHPrices(writeTemp(t, "eth.json", `{not json`))
	require.ErrorContains(t, err, "parse eth prices")
}

func TestLoadClaims_MalformedJSON(t *testing.T) {
	_, err := metrics.LoadClaims(writeTemp(t, "claims.json", `{not json`))
	require.ErrorContains(t, err, "parse claims")
}

func TestLoadClaims_Valid(t *testing.T) {
	p := writeTemp(t, "claims.json", `[{
		"id": "c1", "source": "Report", "source_url": "https://example.com/report",
		"claim_date": "2026 (Q2 report)", "claim_text": "169M+ payments",
		"claimed_value": "169000000", "claimed_unit": "transactions",
		"measured_metric": "total_txns_all", "note": "",
		"lens": "verified x402 payments"
	}]`)
	claims, err := metrics.LoadClaims(p)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, "total_txns_all", claims[0].MeasuredMetric)
	require.Equal(t, "https://example.com/report", claims[0].SourceURL)
	require.Equal(t, "verified x402 payments", claims[0].Lens)
}

func TestLoadClaims_Rejects(t *testing.T) {
	cases := []struct{ name, body, wantErr string }{
		{"unknown subject", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_all","lens":"l"}]`, "agentic"},
		{"known subject rejected", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"known_txns_all","lens":"l"}]`, "known"},
		{"unknown kind", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_count_all","lens":"l"}]`, "count"},
		{"unknown window", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_90d","lens":"l"}]`, "90d"},
		{"missing id", `[{"id":"","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"}]`, "id"},
		{"duplicate id", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"},{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"}]`, "duplicate"},
		{"short metric", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns","lens":"l"}]`, "subject_kind_window"},
		{"missing source", `[{"id":"c","source":"","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"}]`, "required"},
		{"missing source_url", `[{"id":"c","source":"s","source_url":"","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"}]`, "source_url"},
		{"non-http source_url", `[{"id":"c","source":"s","source_url":"ftp://example.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":"l"}]`, "http"},
		{"missing lens", `[{"id":"c","source":"s","source_url":"https://x.com","claim_text":"t","claimed_value":"1","measured_metric":"total_txns_all","lens":""}]`, "lens"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := metrics.LoadClaims(writeTemp(t, "claims.json", c.body))
			require.ErrorContains(t, err, c.wantErr)
		})
	}
}

// TestResolveClaims_MissingWindow verifies that ResolveClaims returns a
// descriptive error when the window named in a claim is absent from the page.
// Without this check a missing key yields a zero Measure and the claim would
// silently publish "0.000000" as a confident measured value.
func TestResolveClaims_MissingWindow(t *testing.T) {
	// Page has NO windows — any claim referencing a window must fail.
	page := metrics.EconomyPage{Windows: map[metrics.Window]metrics.WindowEconomy{}}
	claims := []metrics.Claim{
		{ID: "c1", Source: "s", ClaimText: "t", ClaimedValue: "1", MeasuredMetric: "total_txns_all"},
	}
	_, err := metrics.ResolveClaims(page, claims)
	require.Error(t, err)
	require.ErrorContains(t, err, "all", "error must name the missing window")
}

func TestResolveClaims(t *testing.T) {
	page := metrics.EconomyPage{Windows: map[metrics.Window]metrics.WindowEconomy{
		"30d": {Measure: metrics.Measure{TxnCount: 100, VolumeUSDC: "500.000000"}},
	}}
	claims := []metrics.Claim{
		{
			ID: "a", Source: "s", SourceURL: "https://example.com", Lens: "verified x402",
			ClaimText: "t", ClaimedValue: "3700000", MeasuredMetric: "total_txns_30d",
		},
		{
			ID: "b", Source: "s", SourceURL: "https://example.com", Lens: "verified x402",
			ClaimText: "t", ClaimedValue: "1000000", MeasuredMetric: "total_volume_30d",
		},
	}
	got, err := metrics.ResolveClaims(page, claims)
	require.NoError(t, err)
	require.Equal(t, "100", got[0].MeasuredValue)
	require.Equal(t, "transactions", got[0].MeasuredUnit)
	require.Equal(t, "verified x402", got[0].Lens, "lens must pass through to the resolved claim")
	require.Equal(t, "500.000000", got[1].MeasuredValue)
	require.Equal(t, "USDC", got[1].MeasuredUnit)
}
