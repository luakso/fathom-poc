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
		"source": "CoinGecko monthly average",
		"unit": "USD per ETH",
		"prices": {"2026-01": "3085.20", "2026-02": "2039.93"}
	}`)
	prices, err := metrics.LoadETHPrices(p)
	require.NoError(t, err)
	require.Equal(t, "CoinGecko monthly average", prices.Source)
	require.Equal(t, "3085.2", prices.Prices["2026-01"].String())
	require.Len(t, prices.Prices, 2)
}

func TestLoadETHPrices_Rejects(t *testing.T) {
	cases := []struct{ name, body, wantErr string }{
		{"bad month", `{"source":"s","unit":"u","prices":{"2026-13":"1"}}`, "2026-13"},
		{"bad decimal", `{"source":"s","unit":"u","prices":{"2026-01":"abc"}}`, "abc"},
		{"non-positive", `{"source":"s","unit":"u","prices":{"2026-01":"0"}}`, "positive"},
		{"empty prices", `{"source":"s","unit":"u","prices":{}}`, "no prices"},
		{"missing source", `{"source":"","unit":"u","prices":{"2026-01":"1"}}`, "source"},
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
		"id": "c1", "source": "Report", "source_url": "",
		"claim_date": "2026 (Q2 report)", "claim_text": "169M+ payments",
		"claimed_value": "169000000", "claimed_unit": "transactions",
		"measured_metric": "agentic_txns_all", "note": ""
	}]`)
	claims, err := metrics.LoadClaims(p)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, "agentic_txns_all", claims[0].MeasuredMetric)
}

func TestLoadClaims_Rejects(t *testing.T) {
	cases := []struct{ name, body, wantErr string }{
		{"unknown subject", `[{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"bogus_txns_all"}]`, "bogus"},
		{"unknown kind", `[{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_count_all"}]`, "count"},
		{"unknown window", `[{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_90d"}]`, "90d"},
		{"missing id", `[{"id":"","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_all"}]`, "id"},
		{"duplicate id", `[{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_all"},{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_all"}]`, "duplicate"},
		{"short metric", `[{"id":"c","source":"s","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns"}]`, "subject_kind_window"},
		{"missing source", `[{"id":"c","source":"","claim_text":"t","claimed_value":"1","measured_metric":"agentic_txns_all"}]`, "required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := metrics.LoadClaims(writeTemp(t, "claims.json", c.body))
			require.ErrorContains(t, err, c.wantErr)
		})
	}
}

func TestResolveClaims(t *testing.T) {
	page := metrics.EconomyPage{Windows: map[string]metrics.WindowEconomy{
		"30d": {
			Measure: metrics.Measure{TxnCount: 100, VolumeUSDC: "500.000000"},
			ByMembership: map[string]metrics.Measure{
				"agentic": {TxnCount: 90, VolumeUSDC: "50.000000"},
			},
		},
	}}
	claims := []metrics.Claim{
		{ID: "a", Source: "s", ClaimText: "t", ClaimedValue: "3700000", MeasuredMetric: "total_txns_30d"},
		{ID: "b", Source: "s", ClaimText: "t", ClaimedValue: "1000000", MeasuredMetric: "agentic_volume_30d"},
		{ID: "c", Source: "s", ClaimText: "t", ClaimedValue: "5", MeasuredMetric: "contested_volume_30d"},
	}
	got, err := metrics.ResolveClaims(page, claims)
	require.NoError(t, err)
	require.Equal(t, "100", got[0].MeasuredValue)
	require.Equal(t, "transactions", got[0].MeasuredUnit)
	require.Equal(t, "50.000000", got[1].MeasuredValue)
	require.Equal(t, "USDC", got[1].MeasuredUnit)
	// Absent attribution resolves to zeros, not empty strings.
	require.Equal(t, "0.000000", got[2].MeasuredValue)
}
