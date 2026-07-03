package anatomy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ManualLabel is one curated identity label from data/entity-labels.json.
// The file is a git-reviewed input (same pattern as data/claims.json); the
// rollup replaces all source='manual' entity_signal rows with its content.
type ManualLabel struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label   string `json:"label"`
	URL     string `json:"url"`
	Note    string `json:"note"`
}

var evmAddrRe = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

// LoadManualLabels reads and validates the curated label file. EVM addresses
// are normalized to lowercase; duplicates (chain, address) are rejected so a
// bad merge cannot silently drop a label.
func LoadManualLabels(path string) ([]ManualLabel, error) {
	//nolint:gosec // file path is from configuration, not user input
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity labels: %w", err)
	}
	var doc struct {
		Labels []ManualLabel `json:"labels"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse entity labels %s: %w", path, err)
	}
	seen := make(map[string]struct{}, len(doc.Labels))
	for i := range doc.Labels {
		l := &doc.Labels[i]
		if l.Chain != "base" {
			return nil, fmt.Errorf("entity label %d: unknown chain %q", i, l.Chain)
		}
		l.Address = strings.ToLower(l.Address)
		if !evmAddrRe.MatchString(l.Address) {
			return nil, fmt.Errorf("entity label %d: malformed address %q", i, l.Address)
		}
		if l.Label == "" {
			return nil, fmt.Errorf("entity label %d (%s): empty label", i, l.Address)
		}
		key := l.Chain + ":" + l.Address
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("entity label %d: duplicate address %s", i, l.Address)
		}
		seen[key] = struct{}{}
	}
	return doc.Labels, nil
}
