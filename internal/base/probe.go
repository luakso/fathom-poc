package base

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/lukostrobl/fathom/internal/x402"
)

// ProbeDeps bundles the arguments for one probe run.
type ProbeDeps struct {
	Fetcher   Fetcher
	FromBlock uint64
	ToBlock   uint64
}

// ProbeReport summarizes what the probe observed without touching Postgres.
//
// MatchedEvents is the count of AuthorizationUsed logs whose parent tx passed
// KeepAuthorizationUsed (i.e., would have become payments rows).
//
// OuterSighashCounts maps outer-tx sighash hex → count of MATCHING
// AuthorizationUsed events on the USDC proxy seen with that sighash. Includes
// sighashes outside ALLOW that emitted the event — these are coverage-gap
// signals (probe found x402 traffic our filter would have missed).
type ProbeReport struct {
	FromBlock          uint64
	ToBlock            uint64
	BatchCount         int
	TotalLogs          int
	MatchedEvents      int
	OuterSighashCounts map[string]int
}

// RunProbe streams the same query backfill uses but writes nothing. Useful
// before a real backfill to (a) verify HyperSync auth, (b) see the actual
// outer-tx sighash distribution, (c) catch coverage gaps (sighashes outside
// AllowSighashes that still emit AuthorizationUsed).
func RunProbe(ctx context.Context, d ProbeDeps) (ProbeReport, error) {
	if d.FromBlock == 0 {
		return ProbeReport{}, fmt.Errorf("probe: from_block must be > 0")
	}
	if d.ToBlock <= d.FromBlock {
		return ProbeReport{}, fmt.Errorf("probe: to_block (%d) must exceed from_block (%d)", d.ToBlock, d.FromBlock)
	}
	if d.Fetcher == nil {
		return ProbeReport{}, fmt.Errorf("probe: fetcher is required")
	}

	stream, err := d.Fetcher.Stream(BuildBackfillQuery(d.FromBlock, d.ToBlock))
	if err != nil {
		return ProbeReport{}, fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	report := ProbeReport{
		FromBlock:          d.FromBlock,
		ToBlock:            d.ToBlock,
		OuterSighashCounts: map[string]int{},
	}

	for {
		select {
		case <-ctx.Done():
			return report, nil
		default:
		}
		batch, ok, err := stream.Next()
		if err != nil {
			return report, fmt.Errorf("stream next: %w", err)
		}
		if !ok {
			return report, nil
		}
		report.BatchCount++
		report.TotalLogs += len(batch.Data.Logs)

		// Index transactions by hash for fast lookup of outer sighash.
		txByHash := map[string]HyperSyncTransaction{}
		for _, tx := range batch.Data.Transactions {
			txByHash[tx.Hash] = tx
		}

		for _, lg := range batch.Data.Logs {
			// Only count USDC AuthorizationUsed (we asked for them, but the
			// HyperSync response can include other rows depending on field
			// selection — guard anyway).
			if len(lg.Topics) == 0 || lg.Topics[0] != x402.AuthorizationUsedTopic.Hex() {
				continue
			}
			tx, ok := txByHash[lg.TxHash]
			if !ok {
				continue
			}
			// Convert hex input → []byte → uint32 sighash for matching.
			xtx, err := ConvertTransaction(tx)
			if err != nil {
				continue
			}
			sigBytes := SighashFromTransactionInput(xtx.Input)
			report.OuterSighashCounts[sigBytes]++

			// Run the actual filter check.
			xlog, err := ConvertLog(lg)
			if err != nil {
				continue
			}
			if x402.KeepAuthorizationUsed(xlog, xtx.Input) {
				report.MatchedEvents++
			}
		}
	}
}

// SighashFromTransactionInput formats the calldata prefix as a lowercase
// 0x-prefixed 4-byte hex string. Returns "" for too-short calldata.
func SighashFromTransactionInput(input []byte) string {
	v, ok := x402.SighashFromBytes(input)
	if !ok {
		return ""
	}
	return SighashHex(v)
}

// Print writes the report in a human-readable form. Sorted by count descending.
func (r ProbeReport) Print(w io.Writer) {
	_, _ = fmt.Fprintf(w, "probe: blocks %d..%d (batches=%d, total_logs=%d)\n",
		r.FromBlock, r.ToBlock, r.BatchCount, r.TotalLogs)
	_, _ = fmt.Fprintf(w, "matched events: %d\n\n", r.MatchedEvents)
	_, _ = fmt.Fprintln(w, "outer-tx sighash distribution (across all AuthorizationUsed-on-USDC events):")

	type kv struct {
		sig   string
		count int
	}
	rows := make([]kv, 0, len(r.OuterSighashCounts))
	for s, c := range r.OuterSighashCounts {
		rows = append(rows, kv{s, c})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "  %s  %d\n", row.sig, row.count)
	}
}
