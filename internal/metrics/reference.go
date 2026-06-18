package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ETHPrices is the curated monthly ETH/USD reference used to convert
// gas_cost_wei into dollars at rollup time. One price per "YYYY-MM" month;
// rollup fails if any month present in payments lacks a price.
type ETHPrices struct {
	Source string                     // citation, required
	Unit   string                     // documentation only, e.g. "USD per ETH"
	Prices map[string]decimal.Decimal // "YYYY-MM" → USD per ETH, all > 0
}

// ethPricesFile is the on-disk JSON shape (prices as decimal strings).
// Note: encoding/json silently keeps the LAST value for any duplicate month key
// in the JSON object; deduplication is a curatorial responsibility, not enforced here.
type ethPricesFile struct {
	Source string            `json:"source"`
	Unit   string            `json:"unit"`
	Prices map[string]string `json:"prices"`
}

// LoadETHPrices reads and validates the monthly price file. All validation
// happens here so a bad file aborts before the rollup transaction starts.
func LoadETHPrices(path string) (ETHPrices, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is a CLI-supplied config file, not user input
	if err != nil {
		return ETHPrices{}, fmt.Errorf("read eth prices: %w", err)
	}
	var f ethPricesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ETHPrices{}, fmt.Errorf("parse eth prices %s: %w", path, err)
	}
	if f.Source == "" {
		return ETHPrices{}, fmt.Errorf("eth prices %s: source citation is required", path)
	}
	if len(f.Prices) == 0 {
		return ETHPrices{}, fmt.Errorf("eth prices %s: no prices", path)
	}
	out := ETHPrices{Source: f.Source, Unit: f.Unit, Prices: map[string]decimal.Decimal{}}
	for month, raw := range f.Prices {
		if _, err := time.Parse("2006-01", month); err != nil {
			return ETHPrices{}, fmt.Errorf("eth prices %s: bad month key %q (want YYYY-MM)", path, month)
		}
		d, err := decimal.NewFromString(raw)
		if err != nil {
			return ETHPrices{}, fmt.Errorf("eth prices %s: month %s: bad decimal %q", path, month, raw)
		}
		if !d.IsPositive() {
			return ETHPrices{}, fmt.Errorf("eth prices %s: month %s: price must be positive", path, month)
		}
		out.Prices[month] = d
	}
	return out, nil
}

// Claim is one entry of the curated claim ledger (E6): a public figure to be
// compared against a measured number. measured_metric names which measured
// number, as "<subject>_<kind>_<window>" (e.g. known_txns_all): subject ∈
// {total, known, unknown}, kind ∈ {txns, volume},
// window ∈ {7d, 30d, all}.
type Claim struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	SourceURL      string `json:"source_url"`
	ClaimDate      string `json:"claim_date"` // free-form ("2026 (Q2 report)")
	ClaimText      string `json:"claim_text"`
	ClaimedValue   string `json:"claimed_value"`
	ClaimedUnit    string `json:"claimed_unit"`
	MeasuredMetric string `json:"measured_metric"`
	Note           string `json:"note"`
}

// LoadClaims reads and validates the claim ledger. Unknown metric keys fail
// here (and again defensively at resolution). An empty ledger ([]) is valid —
// zero claims means the economy page renders no claims section, unlike prices
// where an empty file is always a configuration mistake.
func LoadClaims(path string) ([]Claim, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is a CLI-supplied config file, not user input
	if err != nil {
		return nil, fmt.Errorf("read claims: %w", err)
	}
	var claims []Claim
	if err := json.Unmarshal(b, &claims); err != nil {
		return nil, fmt.Errorf("parse claims %s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, c := range claims {
		if c.ID == "" {
			return nil, fmt.Errorf("claims %s: entry %d: id is required", path, i)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("claims %s: duplicate id %q", path, c.ID)
		}
		seen[c.ID] = true
		if c.Source == "" || c.ClaimText == "" || c.ClaimedValue == "" {
			return nil, fmt.Errorf("claims %s: %s: source, claim_text and claimed_value are required", path, c.ID)
		}
		if _, _, _, err := parseMetric(c.MeasuredMetric); err != nil {
			return nil, fmt.Errorf("claims %s: %s: %w", path, c.ID, err)
		}
	}
	return claims, nil
}

// parseMetric splits "<subject>_<kind>_<window>" and validates each part.
func parseMetric(s string) (subject, kind, window string, err error) {
	parts := strings.Split(s, "_")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("bad measured_metric %q (want subject_kind_window)", s)
	}
	subject, kind, window = parts[0], parts[1], parts[2]
	switch subject {
	case "total", "known", "unknown":
	default:
		return "", "", "", fmt.Errorf("measured_metric %q: unknown subject %q", s, subject)
	}
	switch kind {
	case "txns", "volume":
	default:
		return "", "", "", fmt.Errorf("measured_metric %q: unknown kind %q", s, kind)
	}
	if _, ok := windowDays[window]; !ok {
		return "", "", "", fmt.Errorf("measured_metric %q: unknown window %q", s, window)
	}
	return subject, kind, window, nil
}
