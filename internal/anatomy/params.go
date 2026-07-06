package anatomy

import (
	"errors"
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

// Lens is the membership lens applied to a request: known (default) | all.
type Lens string

const (
	// LensKnown counts only payments settled by an allowlisted facilitator.
	LensKnown Lens = "known"
	// LensAll counts every payment, regardless of facilitator provenance.
	LensAll Lens = "all"
)

// Window is a leaderboard time window: 7d | 30d | all (default).
type Window string

const (
	// Window7d restricts to the trailing 7 data days.
	Window7d Window = "7d"
	// Window30d restricts to the trailing 30 data days.
	Window30d Window = "30d"
	// WindowAll spans the full dataset.
	WindowAll Window = "all"
)

// Sort is an ordering key. The valid subset depends on the endpoint:
// counterparty lists accept {volume, txns, last_seen}; leaderboards accept
// {volume, txns, counterparties}. The per-endpoint parsers enforce the subset
// and the provider whitelist maps translate each value to safe SQL. SortVolume
// is the shared default.
type Sort string

const (
	// SortVolume orders by summed volume (default).
	SortVolume Sort = "volume"
	// SortTxns orders by transaction count.
	SortTxns Sort = "txns"
	// SortLastSeen orders by most recent activity (counterparty lists only).
	SortLastSeen Sort = "last_seen"
	// SortCounterparties orders by distinct counterparties (leaderboards only).
	SortCounterparties Sort = "counterparties"
)

// chainOK reports whether chain is a supported chain identifier.
func chainOK(chain string) bool { return validChains[chain] }

// parseLens returns the membership lens for a request: known (default) | all.
func parseLens(r *http.Request) (Lens, error) {
	raw := r.URL.Query().Get("lens")
	switch Lens(raw) {
	case "", LensKnown:
		return LensKnown, nil
	case LensAll:
		return LensAll, nil
	default:
		return "", fmt.Errorf("unknown lens %q", raw)
	}
}

// parseRole validates a counterparty/payment subject role. Empty is an error:
// these endpoints require an explicit role.
func parseRole(raw string) (Role, error) {
	switch Role(raw) {
	case RolePayer, RolePayee, RoleFacilitator:
		return Role(raw), nil
	case "":
		return "", errors.New("role is required")
	default:
		return "", errors.New("role must be payer, payee, or facilitator")
	}
}

// parseLeaderboardRole validates a leaderboard role. Facilitators are not
// ranked, so the closed set is narrower than parseRole.
func parseLeaderboardRole(raw string) (Role, error) {
	switch Role(raw) {
	case RolePayer, RolePayee:
		return Role(raw), nil
	case "":
		return "", errors.New("role is required")
	default:
		return "", errors.New("role must be payer or payee")
	}
}

// parseWindow validates a leaderboard window; empty defaults to all.
func parseWindow(raw string) (Window, error) {
	switch Window(raw) {
	case "", WindowAll:
		return WindowAll, nil
	case Window7d, Window30d:
		return Window(raw), nil
	default:
		return "", errors.New("window must be 7d, 30d, or all")
	}
}

// parseCounterpartySort validates a counterparty-list sort; empty defaults to volume.
func parseCounterpartySort(raw string) (Sort, error) {
	switch Sort(raw) {
	case "", SortVolume:
		return SortVolume, nil
	case SortTxns, SortLastSeen:
		return Sort(raw), nil
	default:
		return "", errors.New("sort must be volume, txns, or last_seen")
	}
}

// parseLeaderboardSort validates a leaderboard sort; empty defaults to volume.
func parseLeaderboardSort(raw string) (Sort, error) {
	switch Sort(raw) {
	case "", SortVolume:
		return SortVolume, nil
	case SortTxns, SortCounterparties:
		return Sort(raw), nil
	default:
		return "", errors.New("sort must be volume, txns, or counterparties")
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
