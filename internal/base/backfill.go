package base

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/lukostrobl/fathom/internal/x402"
)

// Backfiller drives the HyperSync stream → decode → assemble → store loop.
//
// One Run pass covers the [fromBlock, toBlock] window inclusive. Run is safe
// to invoke repeatedly with overlapping ranges: idempotency comes from the
// payments PK and the cursor's monotonic advance.
//
// Per spec §11: on any fetch/decode/insert error, Run returns the error and
// the caller exits non-zero. Re-invoking from the last committed cursor
// resumes cleanly — the cursor only advances on a successfully committed
// batch.
type Backfiller struct {
	fetcher Fetcher
	store   *Store

	// allowCandidateLoss downgrades the all-candidates-lost halt to a loud
	// warning. Escape hatch for a single poisoned batch (e.g. one anomalous
	// tx that legitimately fails pairing) that would otherwise wedge the
	// cursor forever; never set it for routine runs.
	allowCandidateLoss bool
}

// BackfillerOption configures optional Backfiller behavior.
type BackfillerOption func(*Backfiller)

// AllowCandidateLoss lets a batch whose candidates all fail pairing commit and
// advance the cursor (with a loud warning) instead of halting the run.
func AllowCandidateLoss() BackfillerOption {
	return func(b *Backfiller) { b.allowCandidateLoss = true }
}

// NewBackfiller constructs a Backfiller. Fetcher and store are required.
func NewBackfiller(fetcher Fetcher, store *Store, opts ...BackfillerOption) *Backfiller {
	b := &Backfiller{fetcher: fetcher, store: store}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Run streams batches from fromBlock to toBlock (inclusive) and writes them
// to Store. Returns the first error encountered; ctx cancellation triggers
// a graceful shutdown between batches (never mid-batch).
func (b *Backfiller) Run(ctx context.Context, fromBlock, toBlock uint64) error {
	q := BuildBackfillQuery(fromBlock, toBlock)
	stream, err := b.fetcher.Stream(q)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	for {
		select {
		case <-ctx.Done():
			slog.Info("backfill: shutdown requested between batches", "err", ctx.Err())
			return nil
		default:
		}

		started := time.Now()
		batch, ok, err := stream.Next()
		if err != nil {
			return fmt.Errorf("stream next: %w", err)
		}
		if !ok {
			slog.Info("backfill: stream complete")
			return nil
		}

		payments, cancellations, stats, decodeErr := decodeBatch(batch)
		if decodeErr != nil {
			return fmt.Errorf("decode batch: %w", decodeErr)
		}

		maxBlock := batch.MaxBlock() // returns 0 for empty batches → cursor skip in Store

		// Silent-loss guard. The whole pipeline assumes JoinAll returns the
		// companion Transfer logs alongside each AuthorizationUsed. If that
		// assumption ever regresses, every candidate fails pairing, Assemble
		// returns zero rows, and — without this guard — the cursor would still
		// advance over the block range, turning a systemic break into permanent
		// data gaps. Halt before InsertBatch so the cursor stays put and the run
		// is resumable once the cause is fixed. Denied-only batches
		// (receiveWithAuthorization) legitimately produce zero rows and are
		// excluded by allCandidatesLost.
		if allCandidatesLost(stats) {
			if !b.allowCandidateLoss {
				return fmt.Errorf(
					"assemble produced 0 rows from %d candidate AuthorizationUsed logs "+
						"(denied=%d dropped=%d max_block=%d): companion pairing likely broke — "+
						"refusing to advance cursor (re-run with --allow-candidate-loss to "+
						"accept the loss for a single poisoned batch)",
					stats.AuthLogs, stats.Denied, stats.Dropped, maxBlock,
				)
			}
			slog.Warn(
				"backfill: ALL candidates lost in batch — continuing because allow-candidate-loss is set",
				"auth_logs", stats.AuthLogs,
				"denied", stats.Denied,
				"dropped", stats.Dropped,
				"max_block", maxBlock,
			)
		}

		if err := b.store.InsertBatch(ctx, payments, cancellations, maxBlock); err != nil {
			return fmt.Errorf("insert batch (rows=%d cancels=%d max_block=%d): %w", len(payments), len(cancellations), maxBlock, err)
		}

		// Anomalous drops below the all-lost threshold don't halt the run, but
		// they must be visible — a rising drop count is the early warning that
		// something upstream is degrading.
		if stats.Dropped > 0 {
			slog.Warn(
				"backfill: candidates dropped during assemble",
				"dropped", stats.Dropped,
				"kept", stats.Kept,
				"auth_logs", stats.AuthLogs,
				"max_block", maxBlock,
			)
		}

		slog.Info(
			"backfill: batch committed",
			"rows", len(payments),
			"cancels", len(cancellations),
			"kept", stats.Kept,
			"denied", stats.Denied,
			"dropped", stats.Dropped,
			"auth_logs", stats.AuthLogs,
			"max_block", maxBlock,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}
}

// allCandidatesLost reports whether a batch carried genuine x402 candidates
// (AuthorizationUsed logs beyond the expected receiveWithAuthorization denials)
// yet produced no rows at all — the signature of a companion-pairing/JoinAll
// regression. Denied-only and genuinely-empty batches return false.
func allCandidatesLost(stats x402.AssembleStats) bool {
	expected := stats.AuthLogs - stats.Denied
	return expected > 0 && stats.Kept == 0
}

// decodeBatch converts the HyperSync wire batch into ([]Payment) ready for
// Store.InsertBatch. Per-row decode failures (bad hex, missing companion)
// log a warn inside Assemble and are skipped — only structural failures
// (whole-row convert errors) abort.
func decodeBatch(batch HyperSyncBatch) ([]x402.Payment, []x402.Cancellation, x402.AssembleStats, error) {
	logs := make([]x402.Log, 0, len(batch.Data.Logs))
	receiptByHash := map[common.Hash][]x402.Log{}
	for i, hl := range batch.Data.Logs {
		lg, err := ConvertLog(hl)
		if err != nil {
			return nil, nil, x402.AssembleStats{}, fmt.Errorf("log[%d]: %w", i, err)
		}
		logs = append(logs, lg)
		receiptByHash[lg.TxHash] = append(receiptByHash[lg.TxHash], lg)
	}

	txByHash := map[common.Hash]x402.Transaction{}
	for i, ht := range batch.Data.Transactions {
		tx, err := ConvertTransaction(ht)
		if err != nil {
			return nil, nil, x402.AssembleStats{}, fmt.Errorf("tx[%d]: %w", i, err)
		}
		txByHash[tx.Hash] = tx
	}

	blockByNumber := map[uint64]x402.Block{}
	for i, hb := range batch.Data.Blocks {
		blk, err := ConvertBlock(hb)
		if err != nil {
			return nil, nil, x402.AssembleStats{}, fmt.Errorf("block[%d]: %w", i, err)
		}
		blockByNumber[blk.Number] = blk
	}

	payments, stats := x402.Assemble(logs, txByHash, receiptByHash, blockByNumber)
	cancellations := x402.ExtractCancellations(logs, txByHash, blockByNumber)
	return payments, cancellations, stats, nil
}
