package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAvgUSDC_ErrorOnMalformedVolume proves that a non-parseable VolumeUSDC
// produces a named error instead of a silent zero.  If avgUSDC used the old
// swallow-behavior (return "0.000000", nil on parse failure) this test would
// fail because require.Error would see a nil error.
func TestAvgUSDC_ErrorOnMalformedVolume(t *testing.T) {
	_, err := avgUSDC(Measure{TxnCount: 1, VolumeUSDC: "not-a-decimal"})
	require.Error(t, err, "malformed VolumeUSDC must produce an error, not a silent zero")
	require.ErrorContains(t, err, "not-a-decimal", "error must quote the offending value for debuggability")
}

func TestAvgUSDC_ZeroCountReturnsZero(t *testing.T) {
	got, err := avgUSDC(Measure{TxnCount: 0, VolumeUSDC: "anything"})
	require.NoError(t, err)
	require.Equal(t, "0.000000", got, "empty measure must return zero without parsing VolumeUSDC")
}

func TestAvgUSDC_ValidDivision(t *testing.T) {
	got, err := avgUSDC(Measure{TxnCount: 4, VolumeUSDC: "12.000000"})
	require.NoError(t, err)
	require.Equal(t, "3.000000", got)
}
