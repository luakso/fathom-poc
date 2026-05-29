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

	if tip < t.opts.ConfirmationDepth {
		return false, nil // chain too young; wait
	}
	safeTip := tip - t.opts.ConfirmationDepth

	if cursor >= safeTip {
		return false, nil // caught up; sleep
	}

	fromBlock := cursor + 1
	rangeEnd := fromBlock + t.opts.BlockBatchSize - 1
	if rangeEnd > safeTip {
		rangeEnd = safeTip
	}

	started := time.Now()
	payments, maxBlock, err := FetchRange(ctx, t.client, fromBlock, rangeEnd, t.opts.Concurrency)
	if err != nil {
		return false, fmt.Errorf("fetch range %d-%d: %w", fromBlock, rangeEnd, err)
	}

	// Even with zero payments, advance the cursor to rangeEnd: we DID query
	// the range and confirmed it's empty. maxBlock from FetchRange is the
	// queried range end (or toBlock if no logs in this batch).
	advanceTo := maxBlock
	if advanceTo < rangeEnd {
		advanceTo = rangeEnd
	}

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
