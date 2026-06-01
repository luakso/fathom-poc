package base

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/lukostrobl/fathom/internal/x402"
)

// probeLogInterval throttles progress logging so a multi-minute run emits a
// steady heartbeat without flooding the log with one line per batch.
const probeLogInterval = 3 * time.Second

// ProbeDeps bundles the arguments for one probe run.
type ProbeDeps struct {
	Fetcher   Fetcher
	FromBlock uint64
	ToBlock   uint64
	// Now is an optional clock for deterministic timing in tests. Defaults to
	// time.Now when nil.
	Now func() time.Time
}

// ProbeReport summarizes what the probe observed without touching Postgres.
//
// MatchedEvents is the count of AuthorizationUsed logs whose parent tx passed
// KeepAuthorizationUsed (i.e., would have become payments rows).
//
// OuterSighashCounts maps outer-tx sighash hex → count of ALL AuthorizationUsed
// events on the USDC proxy seen with that sighash (bucketed before the
// KeepAuthorizationUsed gate, so the total spans matched and unmatched alike).
// Sighashes outside ALLOW that emitted the event are coverage-gap signals
// (x402 traffic our filter would have missed). MatchedEvents is the subset that
// passed the gate.
type ProbeReport struct {
	FromBlock          uint64
	ToBlock            uint64
	BatchCount         int
	TotalLogs          int
	MatchedEvents      int
	OuterSighashCounts map[string]int
	// Elapsed is the wall-clock time RunProbe spent streaming. Used to derive
	// throughput (logs/s, blocks/s) for extrapolating full-backfill duration.
	Elapsed time.Duration
	// LastBlock is the furthest server cursor reached (batch.NextBlock of the
	// last batch processed). On a completed run it is >= ToBlock; on an
	// interrupted run (ctx cancelled / Ctrl-C) it is the partial high-water mark.
	// Throughput is derived from blocks actually covered, not the requested
	// range, so an interrupted run reports honest blocks/s.
	LastBlock uint64
}

// RunProbe streams the same query backfill uses but writes nothing. Useful
// before a real backfill to (a) verify HyperSync auth, (b) see the actual
// outer-tx sighash distribution, (c) sanity-check the keep rate (MatchedEvents
// vs total). Under the topic-only policy MatchedEvents is every
// AuthorizationUsed-on-USDC log except a direct receiveWithAuthorization.
func RunProbe(ctx context.Context, d ProbeDeps) (report ProbeReport, err error) {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	start := now()
	defer func() { report.Elapsed = now().Sub(start) }()

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

	report = ProbeReport{
		FromBlock:          d.FromBlock,
		ToBlock:            d.ToBlock,
		OuterSighashCounts: map[string]int{},
	}

	lastLog := start
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
		if batch.NextBlock > report.LastBlock {
			report.LastBlock = batch.NextBlock
		}

		// Heartbeat: a wide probe runs for minutes and otherwise prints nothing
		// until the end, so a 429 backoff is indistinguishable from a hang.
		// batch.NextBlock is the server cursor; it advances even on empty batches.
		if t := now(); t.Sub(lastLog) >= probeLogInterval {
			lastLog = t
			logProbeProgress(d.FromBlock, d.ToBlock, batch.NextBlock, report)
		}

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

// logProbeProgress emits a single structured heartbeat line for an in-flight
// probe: how far the server cursor has advanced toward to_block, plus running
// counts. cur is batch.NextBlock (the server's resume cursor).
func logProbeProgress(fromBlock, toBlock, cur uint64, report ProbeReport) {
	var pct float64
	if toBlock > fromBlock && cur > fromBlock {
		pct = float64(cur-fromBlock) / float64(toBlock-fromBlock) * 100
	}
	if pct > 100 {
		pct = 100
	}
	slog.Info(
		"probe progress",
		"block", cur,
		"to_block", toBlock,
		"pct", fmt.Sprintf("%.1f", pct),
		"batches", report.BatchCount,
		"total_logs", report.TotalLogs,
		"matched", report.MatchedEvents,
	)
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
	_, _ = fmt.Fprintf(w, "matched events: %d\n", r.MatchedEvents)

	// Blocks actually covered = high-water cursor clamped into [FromBlock,
	// ToBlock]. An interrupted run stops short of ToBlock; deriving throughput
	// from the full requested range would overstate blocks/s by the fraction
	// left unscanned. LastBlock == 0 means it was never tracked (a manually
	// built report) — treat that as a complete full-range run.
	covered := r.LastBlock
	if covered == 0 || covered > r.ToBlock {
		covered = r.ToBlock
	}
	if covered < r.FromBlock {
		covered = r.FromBlock
	}
	blocksCovered := covered - r.FromBlock
	if covered < r.ToBlock {
		rangeSize := r.ToBlock - r.FromBlock
		var pct float64
		if rangeSize > 0 {
			pct = float64(blocksCovered) / float64(rangeSize) * 100
		}
		_, _ = fmt.Fprintf(w, "PARTIAL: reached block %d of %d (%.1f%% of range — run was interrupted)\n",
			covered, r.ToBlock, pct)
	}
	if secs := r.Elapsed.Seconds(); secs > 0 {
		_, _ = fmt.Fprintf(w, "elapsed: %s  (%.0f logs/s, %.0f blocks/s)\n",
			r.Elapsed.Round(time.Millisecond),
			float64(r.TotalLogs)/secs,
			float64(blocksCovered)/secs)
	}
	_, _ = fmt.Fprintln(w)
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
