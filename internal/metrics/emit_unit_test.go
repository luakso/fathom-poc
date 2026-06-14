package metrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteSite_PrunesStaleAssets(t *testing.T) {
	dir := t.TempDir()

	// Artifacts at the root are not the site's to manage.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "economy.json"), []byte("{}"), 0o644))

	// Leftovers from a previous site version: a renamed-away module and an
	// orphaned temp file from an interrupted write.
	stale := filepath.Join(dir, "assets", "js", "renamed-away.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte("// stale"), 0o644))
	orphan := filepath.Join(dir, "assets", "workbench.css.tmp")
	require.NoError(t, os.WriteFile(orphan, []byte("partial"), 0o644))

	require.NoError(t, writeSite(dir))

	require.NoFileExists(t, stale, "renamed-away module must be pruned from the served tree")
	require.NoFileExists(t, orphan, "orphaned temp file must be pruned from the served tree")
	require.FileExists(t, filepath.Join(dir, "economy.json"), "root artifacts must survive the prune")
	require.FileExists(t, filepath.Join(dir, "index.html"))
	require.FileExists(t, filepath.Join(dir, "assets", "js", "app.js"))
}

func TestWriteSite_FirstEmitHasNothingToPrune(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeSite(dir))
	require.FileExists(t, filepath.Join(dir, "index.html"))
}
