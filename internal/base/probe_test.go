package base_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

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
		Nonce:             1,
		GasUsed:           50_000,
		EffectiveGasPrice: "0x3b9aca00",
		BaseFeePerGas:     "0x1dcd6500",
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
