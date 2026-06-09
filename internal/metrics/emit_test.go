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
	require.NoError(t, metrics.RebuildDaily(ctx, pool))

	dir := t.TempDir()
	asOf := mustTime(t, "2026-06-09T00:00:00Z")
	require.NoError(t, metrics.Emit(ctx, pool, dir, asOf))

	// economy.json exists and carries stamps.
	b, err := os.ReadFile(filepath.Join(dir, "economy.json"))
	require.NoError(t, err)
	var doc struct {
		MethodologyVersion int    `json:"methodology_version"`
		GeneratedAt        string `json:"generated_at"`
		Data               struct {
			Windows map[string]struct {
				TxnCount int64 `json:"txn_count"`
			} `json:"windows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, 1, doc.MethodologyVersion)
	require.NotEmpty(t, doc.GeneratedAt)
	require.Equal(t, int64(1), doc.Data.Windows["all"].TxnCount)

	// facilitators.json exists too.
	_, err = os.Stat(filepath.Join(dir, "facilitators.json"))
	require.NoError(t, err)
}
