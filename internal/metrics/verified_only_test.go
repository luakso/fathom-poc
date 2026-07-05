//go:build integration

package metrics_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

// TestEmit_NoMembershipLeaks asserts every emitted artifact is verified-only:
// it carries scope=verified-x402 and contains no by_membership / unknown keys.
//
// Seeding BOTH a known (allowlisted) and an unknown (non-allowlisted) facilitator
// makes the guard non-trivial: if any builder leaks the unknown slice, the
// `"unknown":` assertion fires.
func TestEmit_NoMembershipLeaks(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1") // 0xfac1 is known; 0xfac2 is deliberately NOT allowlisted
	seedPayments(t, ctx, db, []seedRow{
		{"0xa", 0, "2026-06-05T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00"}, // known
		{"0xb", 0, "2026-06-05T11:00:00Z", "0xfac2", "0xp2", "0xs2", "2.00"}, // unknown
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	artifacts := []string{
		"economy.json",
		"payees.json",
		"payers.json",
		"reliability.json",
		"mechanics.json",
		"facilitators.json",
	}
	for _, name := range artifacts {
		b, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err, "%s must be emitted", name)
		s := string(b)

		require.Contains(t, s, `"scope": "verified-x402"`, "%s must stamp scope", name)
		// Match object-key form (trailing colon) so a legitimate field value of
		// "unknown" (e.g. a settlement_kind string) does not trip the guard.
		require.NotContains(t, s, `"by_membership":`, "%s must not ship by_membership", name)
		require.NotContains(t, s, `"unknown":`, "%s must not ship an unknown-membership key", name)
	}
}
