package metrics

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// dec parses a decimal from string — test helper only.
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// mustDay parses "YYYY-MM-DD" into a time.Time at midnight UTC — test helper.
func mustDay(s string) time.Time {
	t, err := time.Parse(dayFormat, s)
	if err != nil {
		panic("mustDay: " + err.Error())
	}
	return t
}

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

// ---------------------------------------------------------------------------
// windowLargestPayments (item 6.2)
// ---------------------------------------------------------------------------

func TestWindowLargestPayments_AllWindowTakesMax(t *testing.T) {
	// Three cells; max is the whale on Jan 10. "all" must see it; "7d" (from May 30) must not.
	asOf := mustDay("2026-06-05")
	slices := []cubeSlice{
		{day: "2026-01-10", band: "whale", txns: 1, volume: dec("1000"), maxAmt: dec("1000")},
		{day: "2026-06-05", band: "small", txns: 1, volume: dec("50"), maxAmt: dec("50")},
		{day: "2026-06-05", band: "micro", txns: 1, volume: dec("1"), maxAmt: dec("0.50")},
	}

	got := windowLargestPayments(slices, asOf)

	require.NotNil(t, got["all"], "all-window max must be non-nil when verified rows exist")
	require.Equal(t, "1000.000000", *got["all"])

	require.NotNil(t, got["7d"])
	require.Equal(t, "50.000000", *got["7d"], "7d must only see the Jun 5 slice")
}

func TestWindowLargestPayments_NilWhenNoSlices(t *testing.T) {
	got := windowLargestPayments(nil, mustDay("2026-06-05"))
	require.Nil(t, got["all"], "nil slices → nil largest for all window")
	require.Nil(t, got["7d"], "nil slices → nil largest for 7d window")
}

func TestWindowLargestPayments_MultipleBandsSameDay(t *testing.T) {
	// Two bands on the same day: largest must come from the bigger max_amount_usdc.
	asOf := mustDay("2026-06-05")
	slices := []cubeSlice{
		{day: "2026-06-05", band: "dust", txns: 5, volume: dec("1"), maxAmt: dec("0.001")},
		{day: "2026-06-05", band: "whale", txns: 1, volume: dec("5000"), maxAmt: dec("5000")},
	}
	got := windowLargestPayments(slices, asOf)
	require.Equal(t, "5000.000000", *got["all"])
}

// ---------------------------------------------------------------------------
// buildGasCostDailySeries (item 6.4)
// ---------------------------------------------------------------------------

func TestBuildGasCostDailySeries_TwoDaysCorrectRatio(t *testing.T) {
	// Day 1: $1 cost / $10 vol = 10¢/$
	// Day 2: $4 cost / $100 vol = 4¢/$
	slices := []gasSlice{
		{day: "2026-06-01", band: "small", txns: 1, usd: dec("1.00"), volume: dec("10.00")},
		{day: "2026-06-02", band: "small", txns: 1, usd: dec("4.00"), volume: dec("100.00")},
	}
	got := buildGasCostDailySeries(slices)
	require.Len(t, got, 2)

	require.Equal(t, "2026-06-01", got[0].Day)
	require.True(t, got[0].Complete, "first day must be complete")
	require.Equal(t, "10.0000", got[0].CentsPerDollar)

	require.Equal(t, "2026-06-02", got[1].Day)
	require.False(t, got[1].Complete, "last day must be incomplete (edge convention)")
	require.Equal(t, "4.0000", got[1].CentsPerDollar)
}

func TestBuildGasCostDailySeries_ZeroVolumeDaySkipped(t *testing.T) {
	// A day with zero volume is undefined (division by zero) — skip it.
	slices := []gasSlice{
		{day: "2026-06-01", band: "small", txns: 0, usd: dec("0"), volume: dec("0")},
		{day: "2026-06-02", band: "small", txns: 1, usd: dec("2.00"), volume: dec("50.00")},
	}
	got := buildGasCostDailySeries(slices)
	require.Len(t, got, 1, "zero-volume day must be skipped, not produce a null or panic")
	require.Equal(t, "2026-06-02", got[0].Day)
	require.False(t, got[0].Complete, "only day in series is also the edge day")
}

func TestBuildGasCostDailySeries_MultipleBandsAggregated(t *testing.T) {
	// Two bands on the same day must be summed before computing the ratio.
	// cost=1+3=4, vol=10+40=50 → 4/50*100 = 8.0000¢/$
	slices := []gasSlice{
		{day: "2026-06-01", band: "dust", txns: 10, usd: dec("1.00"), volume: dec("10.00")},
		{day: "2026-06-01", band: "small", txns: 5, usd: dec("3.00"), volume: dec("40.00")},
	}
	got := buildGasCostDailySeries(slices)
	require.Len(t, got, 1)
	require.Equal(t, "8.0000", got[0].CentsPerDollar, "bands must be summed before the ratio")
	require.False(t, got[0].Complete)
}

func TestBuildGasCostDailySeries_EmptyInput(t *testing.T) {
	require.Empty(t, buildGasCostDailySeries(nil))
}
