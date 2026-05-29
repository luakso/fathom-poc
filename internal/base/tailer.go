package base

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// TailerOptions configures the live polling loop. Zero values get sensible
// defaults via NewTailer.
type TailerOptions struct {
	PollInterval      time.Duration // sleep between iterations when caught up. Default 1s.
	BlockBatchSize    uint64        // max blocks per eth_getLogs window. Default 100.
	Concurrency       int64         // parallel block + receipt fetches. Default 10.
	ConfirmationDepth uint64        // tip - depth = safe tip. Default 6.
}

// Tailer owns the live-tail polling loop. Constructed once per process; Run
// blocks until ctx is cancelled.
type Tailer struct {
	client Client
	store  *Store
	opts   TailerOptions
}

// NewTailer constructs a Tailer. Zero-value option fields are replaced with
// the spec §8 defaults.
func NewTailer(c Client, s *Store, opts TailerOptions) *Tailer {
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.BlockBatchSize == 0 {
		opts.BlockBatchSize = 100
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 10
	}
	if opts.ConfirmationDepth == 0 {
		opts.ConfirmationDepth = 6
	}
	return &Tailer{client: c, store: s, opts: opts}
}

// nextWindow computes the next block range to query from the current cursor
// and chain tip. It is the pure core of the polling loop, factored out so the
// confirmation-depth and batch-clamp boundaries can be unit-tested without a
// live RPC or database.
//
// safeTip = tip - confirmationDepth (the deepest block we trust not to reorg).
// When the chain is younger than the confirmation depth, or the cursor has
// already reached safeTip, hasWork is false and from/to are meaningless.
// Otherwise the closed range [from, to] is [cursor+1, min(cursor+batchSize,
// safeTip)].
func nextWindow(cursor, tip, batchSize, confirmationDepth uint64) (from, to uint64, hasWork bool) {
	if tip < confirmationDepth {
		return 0, 0, false // chain too young
	}
	safeTip := tip - confirmationDepth
	if cursor >= safeTip {
		return 0, 0, false // caught up
	}
	from = cursor + 1
	to = min(from+batchSize-1, safeTip)
	return from, to, true
}

// Run drives the polling loop until ctx is cancelled. On cancel Run returns
// nil (graceful shutdown). On any in-flight error (RPC dropout, decode
// failure, Postgres unavailable) Run returns the error — the caller is
// expected to exit non-zero so compose's unless-stopped restarts the process.
func (t *Tailer) Run(ctx context.Context) error {
	slog.Info(
		"tailer: starting",
		"poll_interval", t.opts.PollInterval.String(),
		"block_batch_size", t.opts.BlockBatchSize,
		"concurrency", t.opts.Concurrency,
		"confirmation_depth", t.opts.ConfirmationDepth,
	)
	for {
		select {
		case <-ctx.Done():
			slog.Info("tailer: shutdown")
			return nil
		default:
		}

		advanced, err := t.iterate(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if !advanced {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(t.opts.PollInterval):
			}
		}
	}
}

// iterate does one polling iteration: read cursor, fetch a range, insert.
// Returns (advanced, err): advanced=true if we made any forward progress, so
// the loop should NOT sleep and should try the next window immediately.
func (t *Tailer) iterate(ctx context.Context) (bool, error) {
	cursor, err := t.store.GetCursor(ctx)
	if err != nil {
		return false, fmt.Errorf("read cursor: %w", err)
	}

	tip, err := t.client.BlockNumber(ctx)
	if err != nil {
		return false, fmt.Errorf("read tip: %w", err)
	}

	fromBlock, rangeEnd, hasWork := nextWindow(cursor, tip, t.opts.BlockBatchSize, t.opts.ConfirmationDepth)
	if !hasWork {
		return false, nil // chain too young or caught up; sleep
	}

	started := time.Now()
	payments, maxBlock, err := FetchRange(ctx, t.client, fromBlock, rangeEnd, t.opts.Concurrency)
	if err != nil {
		return false, fmt.Errorf("fetch range %d-%d: %w", fromBlock, rangeEnd, err)
	}

	// Even with zero payments, advance the cursor to rangeEnd: we DID query
	// the range and confirmed it's empty. FetchRange returns rangeEnd as
	// maxBlock on every success path, so this max is belt-and-suspenders
	// against a future FetchRange that reports a partial high-water mark.
	advanceTo := max(maxBlock, rangeEnd)

	if err := t.store.InsertBatch(ctx, payments, advanceTo); err != nil {
		return false, fmt.Errorf("insert %d rows (range %d-%d): %w",
			len(payments), fromBlock, rangeEnd, err)
	}

	slog.Info(
		"tailer: range committed",
		"from", fromBlock, "to", rangeEnd,
		"rows", len(payments),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return true, nil
}
