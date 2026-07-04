package anatomy

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// validChains restricts the API to chains with real data.
// Solana support returns when its data is real (spec §9).
var validChains = map[string]bool{"base": true}

var (
	evmAddrRe40     = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
	paymentCursorRe = regexp.MustCompile(`^\d+:0x[0-9a-f]{64}:\d+$`)
)

// chainOK reports whether chain is a supported chain identifier.
func chainOK(chain string) bool { return validChains[chain] }

// parseLens returns the membership lens for a request: known (default) | all.
func parseLens(r *http.Request) (string, error) {
	lens := r.URL.Query().Get("lens")
	switch lens {
	case "", "known":
		return "known", nil
	case "all":
		return "all", nil
	default:
		return "", fmt.Errorf("unknown lens %q", lens)
	}
}

// parseAddr normalizes an EVM address to lowercase and validates its shape.
func parseAddr(raw string) (string, error) {
	a := strings.ToLower(raw)
	if !evmAddrRe40.MatchString(a) {
		return "", fmt.Errorf("malformed address")
	}
	return a, nil
}

// parseLimit reads ?limit= with a default and hard cap.
func parseLimit(r *http.Request, def, max int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return 0, fmt.Errorf("limit must be 1..%d", max)
	}
	return n, nil
}
