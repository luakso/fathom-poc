package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEnumWireCompat asserts that the named enum types marshal to JSON
// identically to the plain strings they replaced — both as struct field values
// and as map keys. This is the load-bearing guarantee that H1's typing changes
// leave the emitted artifacts byte-for-byte unchanged.
func TestEnumWireCompat(t *testing.T) {
	// Struct field of a named-string type marshals to the same string.
	typed := EntityPage{Role: RolePayee, Windows: map[Window]EntityWindow{}}
	b, err := json.Marshal(typed)
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"payee","windows":{}}`, string(b))

	// A named-string map key marshals to the same key a string would.
	m := map[Window]int{Window7d: 1, Window30d: 2, WindowAll: 3}
	bm, err := json.Marshal(m)
	require.NoError(t, err)
	require.JSONEq(t, `{"7d":1,"30d":2,"all":3}`, string(bm))
}

// TestTxTypeCountsWireCompat asserts the H2 struct reproduces the exact JSON of
// the map[string]int64{"0":..,"1":..,"2":..} it replaced.
func TestTxTypeCountsWireCompat(t *testing.T) {
	got, err := json.Marshal(TxTypeCounts{Type0: 5, Type1: 0, Type2: 42})
	require.NoError(t, err)
	want, err := json.Marshal(map[string]int64{"0": 5, "1": 0, "2": 42})
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

// TestLatencyBucketsWireCompat asserts the H2 struct reproduces the exact JSON
// of the map[string]int64 latency histogram it replaced.
func TestLatencyBucketsWireCompat(t *testing.T) {
	got, err := json.Marshal(LatencyBuckets{Sub1s: 1, B1To10s: 2, B10To60: 3, B1To10m: 4, GT10m: 5})
	require.NoError(t, err)
	want, err := json.Marshal(map[string]int64{
		"sub1s": 1, "1_10s": 2, "10_60s": 3, "1_10m": 4, "gt10m": 5,
	})
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

// TestWriteArtifactFixedClock verifies L2: the injected clock (not wall time)
// stamps GeneratedAt, so emit is reproducible under a fixed clock.
func TestWriteArtifactFixedClock(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixed }

	require.NoError(t, writeArtifact(dir, "sample.json", 7, "2026-06-06",
		map[string]int{"x": 1}, clock))

	b, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	require.NoError(t, err)
	var doc struct {
		GeneratedAt string `json:"generated_at"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "2026-07-06T12:00:00Z", doc.GeneratedAt)
}
