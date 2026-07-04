package anatomy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMedianInt64(t *testing.T) {
	require.Equal(t, int64(0), medianInt64(nil))
	require.Equal(t, int64(5), medianInt64([]int64{5}))
	require.Equal(t, int64(3), medianInt64([]int64{1, 3, 9}))
	require.Equal(t, int64(2), medianInt64([]int64{1, 2, 3, 9})) // lower median
}

func TestSpanDays(t *testing.T) {
	n, err := spanDays("2026-06-01", "2026-06-03")
	require.NoError(t, err)
	require.Equal(t, int64(3), n) // inclusive span
	n, err = spanDays("2026-06-01", "2026-06-01")
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}
