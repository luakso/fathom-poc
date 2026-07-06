package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ETHPrices is the curated weekly ETH/USD reference used to convert
// gas_cost_wei into dollars at rollup time. One price per ISO Monday
// week-start date "YYYY-MM-DD"; rollup fails if any week present in
// payments lacks a price.
type ETHPrices struct {
	Source string                     // citation, required
	Unit   string                     // documentation only, e.g. "USD per ETH"
	Prices map[string]decimal.Decimal // "YYYY-MM-DD" (Monday) → USD per ETH, all > 0
}

// ethPricesFile is the on-disk JSON shape (prices as decimal strings).
type ethPricesFile struct {
	Source string          `json:"source"`
	Unit   string          `json:"unit"`
	Prices json.RawMessage `json:"prices"`
}

// decodePricesStrict decodes the prices JSON object, returning an error on
// any duplicate key. encoding/json.Unmarshal silently keeps the LAST value
// for duplicate keys; this decoder makes duplicates a hard error.
func decodePricesStrict(raw json.RawMessage) (map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("prices: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("prices must be a JSON object")
	}
	seen := map[string]struct{}{}
	result := map[string]string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("prices key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("prices: unexpected non-string key")
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("prices: duplicate key %q", key)
		}
		seen[key] = struct{}{}
		var val string
		if err := dec.Decode(&val); err != nil {
			return nil, fmt.Errorf("prices: value for key %q: %w", key, err)
		}
		result[key] = val
	}
	return result, nil
}

// LoadETHPrices reads and validates the weekly price file. All validation
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
	rawMap, err := decodePricesStrict(f.Prices)
	if err != nil {
		return ETHPrices{}, fmt.Errorf("eth prices %s: %w", path, err)
	}
	if len(rawMap) == 0 {
		return ETHPrices{}, fmt.Errorf("eth prices %s: no prices", path)
	}
	out := ETHPrices{Source: f.Source, Unit: f.Unit, Prices: map[string]decimal.Decimal{}}
	for week, raw := range rawMap {
		t, err := time.Parse("2006-01-02", week)
		if err != nil {
			return ETHPrices{}, fmt.Errorf("eth prices %s: bad week key %q (want YYYY-MM-DD Monday)", path, week)
		}
		if t.Weekday() != time.Monday {
			return ETHPrices{}, fmt.Errorf("eth prices %s: week key %q is not a Monday", path, week)
		}
		d, err := decimal.NewFromString(raw)
		if err != nil {
			return ETHPrices{}, fmt.Errorf("eth prices %s: week %s: bad decimal %q", path, week, raw)
		}
		if !d.IsPositive() {
			return ETHPrices{}, fmt.Errorf("eth prices %s: week %s: price must be positive", path, week)
		}
		out.Prices[week] = d
	}
	return out, nil
}

// Claim is one entry of the curated claim ledger (E6): a public figure to be
// compared against a measured number. measured_metric names which measured
// number, as "total_<kind>_<window>" (e.g. total_txns_all): kind ∈
// {txns, volume}, window ∈ {7d, 30d, all}. The subject is always "total"
// (the verified/known window total).
//
// Lens is a required free-text note naming what the claim actually measures
// (e.g. "all EIP-3009 USDC transfers" vs "verified x402 payments") so readers
// can judge comparability without reading the source.  It is emitted verbatim
// into the artifact alongside the verdict.
type Claim struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	SourceURL      string `json:"source_url"` // required; must start with http/https
	ClaimDate      string `json:"claim_date"` // free-form ("2026 (Q2 report)")
	ClaimText      string `json:"claim_text"`
	ClaimedValue   string `json:"claimed_value"`
	ClaimedUnit    string `json:"claimed_unit"`
	MeasuredMetric string `json:"measured_metric"`
	Lens           string `json:"lens"` // required; what the claim measures
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
		if c.SourceURL == "" {
			return nil, fmt.Errorf("claims %s: %s: source_url is required", path, c.ID)
		}
		if !strings.HasPrefix(c.SourceURL, "http://") && !strings.HasPrefix(c.SourceURL, "https://") {
			return nil, fmt.Errorf("claims %s: %s: source_url must start with http:// or https://", path, c.ID)
		}
		if c.Lens == "" {
			return nil, fmt.Errorf("claims %s: %s: lens is required", path, c.ID)
		}
		if _, _, _, err := parseMetric(c.MeasuredMetric); err != nil {
			return nil, fmt.Errorf("claims %s: %s: %w", path, c.ID, err)
		}
	}
	return claims, nil
}

// parseMetric splits "total_<kind>_<window>" and validates each part.
func parseMetric(s string) (subject string, kind ClaimKind, window Window, err error) {
	parts := strings.Split(s, "_")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("bad measured_metric %q (want subject_kind_window)", s)
	}
	subject, kind, window = parts[0], ClaimKind(parts[1]), Window(parts[2])
	switch subject {
	case "total":
	default:
		return "", "", "", fmt.Errorf("measured_metric %q: unknown subject %q", s, subject)
	}
	switch kind {
	case ClaimKindTxns, ClaimKindVolume:
	default:
		return "", "", "", fmt.Errorf("measured_metric %q: unknown kind %q", s, kind)
	}
	if _, ok := windowDays[window]; !ok {
		return "", "", "", fmt.Errorf("measured_metric %q: unknown window %q", s, window)
	}
	return subject, kind, window, nil
}
