package base_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

func TestRunProbe_CountsByOuterSighash(t *testing.T) {
	t.Parallel()

	// Build a batch where two txs hit different outer sighashes:
	// - 0xdead: classic transferWithAuthorization (0xe3ee160e)
	// - 0xbeef: aggregate3 (0x82ad56cb)
	b := fixtureBatch()
	// Add a second tx + log with aggregate3 sighash:
	b.Data.Transactions = append(b.Data.Transactions, base.HyperSyncTransaction{
		Hash:              "0xbeef",
		BlockNumber:       100,
		From:              "0xfac1000000000000000000000000000000000002",
		To:                strings.ToLower("0xcA11bde05977b3631167028862bE2a173976CA11"),
		Input:             "0x82ad56cb",
		Type:              2,
		Nonce:             "0x1",    // 1
		GasUsed:           "0xc350", // 50_000
		EffectiveGasPrice: "0x3b9aca00",
	})
	// Mirror the first batch's existing Transfer+AuthorizationUsed pattern, but
	// attached to the second tx so the second sighash actually gets counted.
	first := b.Data.Logs[0]
	first.TxHash = "0xbeef"
	first.LogIndex = 2
	second := b.Data.Logs[1]
	second.TxHash = "0xbeef"
	second.LogIndex = 3
	b.Data.Logs = append(b.Data.Logs, first, second)

	f := &fakeFetcher{batches: []base.HyperSyncBatch{b}}
	report, err := base.RunProbe(context.Background(), base.ProbeDeps{
		Fetcher:   f,
		FromBlock: 100,
		ToBlock:   101, // range must satisfy to > from (probe contract); fakeFetcher ignores range
	})
	require.NoError(t, err)
	require.Equal(t, 2, report.MatchedEvents)
	require.Equal(t, 1, report.OuterSighashCounts["0xe3ee160e"])
	require.Equal(t, 1, report.OuterSighashCounts["0x82ad56cb"])
}

func TestRunProbe_StampsElapsedFromClock(t *testing.T) {
	t.Parallel()

	// Injected clock advances exactly 2s between the two reads RunProbe makes
	// (start, then the deferred stamp).
	base0 := time.Unix(1_700_000_000, 0)
	ticks := []time.Time{base0, base0.Add(2 * time.Second)}
	i := 0
	clk := func() time.Time {
		t := ticks[i]
		if i < len(ticks)-1 {
			i++
		}
		return t
	}

	f := &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}}
	report, err := base.RunProbe(context.Background(), base.ProbeDeps{
		Fetcher:   f,
		FromBlock: 100,
		ToBlock:   101,
		Now:       clk,
	})
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, report.Elapsed)
}

func TestProbeReport_Print_ShowsThroughputWhenTimed(t *testing.T) {
	t.Parallel()
	r := base.ProbeReport{
		FromBlock:          100,
		ToBlock:            1100, // 1000-block span
		TotalLogs:          5000,
		MatchedEvents:      42,
		OuterSighashCounts: map[string]int{"0xe3ee160e": 10},
		Elapsed:            2 * time.Second,
	}
	var buf bytes.Buffer
	r.Print(&buf)
	out := buf.String()
	require.Contains(t, out, "elapsed: 2s")
	require.Contains(t, out, "2500 logs/s")  // 5000 logs / 2s
	require.Contains(t, out, "500 blocks/s") // 1000 blocks / 2s
}

func TestProbeReport_Print_PartialRunUsesActualBlocksCovered(t *testing.T) {
	t.Parallel()
	// Requested 100..1100 (1000-block span) but interrupted at block 600 — only
	// 500 blocks actually covered. blocks/s must reflect 500, not 1000, and the
	// report must flag the run as partial.
	r := base.ProbeReport{
		FromBlock:          100,
		ToBlock:            1100,
		LastBlock:          600,
		TotalLogs:          5000,
		MatchedEvents:      42,
		OuterSighashCounts: map[string]int{"0xe3ee160e": 10},
		Elapsed:            2 * time.Second,
	}
	var buf bytes.Buffer
	r.Print(&buf)
	out := buf.String()
	require.Contains(t, out, "PARTIAL")
	require.Contains(t, out, "reached block 600 of 1100")
	require.Contains(t, out, "250 blocks/s") // 500 blocks / 2s, NOT 1000/2s=500
	require.Contains(t, out, "2500 logs/s")  // logs are accurate regardless
}

func TestProbeReport_Print_OmitsThroughputWhenUntimed(t *testing.T) {
	t.Parallel()
	r := base.ProbeReport{MatchedEvents: 1, OuterSighashCounts: map[string]int{}}
	var buf bytes.Buffer
	r.Print(&buf)
	require.NotContains(t, buf.String(), "elapsed:")
}

func TestRunProbe_RejectsZeroFromBlock(t *testing.T) {
	t.Parallel()
	_, err := base.RunProbe(context.Background(), base.ProbeDeps{
		Fetcher:   &fakeFetcher{},
		FromBlock: 0,
		ToBlock:   100,
	})
	require.Error(t, err)
}

func TestProbeReport_Print_HumanReadable(t *testing.T) {
	t.Parallel()
	r := base.ProbeReport{
		MatchedEvents: 42,
		OuterSighashCounts: map[string]int{
			"0xe3ee160e": 10,
			"0xcf092995": 30,
			"0x82ad56cb": 2,
		},
	}
	var buf bytes.Buffer
	r.Print(&buf)
	out := buf.String()
	require.Contains(t, out, "matched events: 42")
	require.Contains(t, out, "0xcf092995")
	require.Contains(t, out, "30")
}
