package metrics

import (
	"testing"

	"github.com/shopspring/decimal"
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

// ---------------------------------------------------------------------------
// dailySeries partial-day flag (item 5.1)
// ---------------------------------------------------------------------------

func makeSlices(days []string) []cubeSlice {
	s := make([]cubeSlice, 0, len(days))
	for _, d := range days {
		s = append(s, cubeSlice{day: d, band: "small", txns: 100, volume: decimal.NewFromInt(10)})
	}
	return s
}

func TestDailySeries_EmptyReturnsEmpty(t *testing.T) {
	got := dailySeries(nil)
	require.Empty(t, got)
}

func TestDailySeries_SingleDayIsIncomplete(t *testing.T) {
	got := dailySeries(makeSlices([]string{"2026-06-01"}))
	require.Len(t, got, 1)
	require.False(t, got[0].Complete, "single (= max) day must be marked incomplete")
}

func TestDailySeries_MultiDayLastIsIncomplete(t *testing.T) {
	got := dailySeries(makeSlices([]string{"2026-06-01", "2026-06-02", "2026-06-03"}))
	require.Len(t, got, 3)
	require.True(t, got[0].Complete, "first day must be complete")
	require.True(t, got[1].Complete, "middle day must be complete")
	require.False(t, got[2].Complete, "last (max) day must be incomplete")
}

func TestDailySeries_MultipleBandsPerDayOnlyLastDayIncomplete(t *testing.T) {
	// Two bands on the same day: they collapse to one DailyPoint, which is either
	// complete or not depending on whether that day is the max.
	slices := []cubeSlice{
		{day: "2026-06-01", band: "dust", txns: 5, volume: decimal.NewFromInt(1)},
		{day: "2026-06-01", band: "small", txns: 10, volume: decimal.NewFromInt(5)},
		{day: "2026-06-02", band: "dust", txns: 3, volume: decimal.NewFromInt(1)},
		{day: "2026-06-02", band: "small", txns: 7, volume: decimal.NewFromInt(3)},
	}
	got := dailySeries(slices)
	require.Len(t, got, 2)
	require.True(t, got[0].Complete)
	require.False(t, got[1].Complete)
	require.Equal(t, int64(15), got[0].TxnCount, "bands must be summed within a day")
	require.Equal(t, int64(10), got[1].TxnCount)
}
