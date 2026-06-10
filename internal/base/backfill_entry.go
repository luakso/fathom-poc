package base

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// BackfillDeps bundles the runtime arguments for one backfill invocation.
// Constructed by the CLI layer (Plan 4) and passed to RunBackfill.
type BackfillDeps struct {
	Fetcher   Fetcher
	Store     *Store
	FromBlock uint64
	ToBlock   uint64 // required (> 0); inclusive last block. Operator subtracts their own reorg margin.

	// AllowCandidateLoss downgrades the all-candidates-lost halt to a warning
	// so a single poisoned batch can be stepped past. See AllowCandidateLoss().
	AllowCandidateLoss bool
}

// RunBackfill validates dependencies and drives one backfill pass. Returns
// an error on validation failure or on any in-flight failure from Backfiller.Run.
// Logs a summary line at start and finish.
func RunBackfill(ctx context.Context, d BackfillDeps) error {
	if d.Fetcher == nil {
		return fmt.Errorf("backfill: fetcher is required")
	}
	if d.FromBlock == 0 {
		return fmt.Errorf("backfill: from_block must be > 0")
	}
	if d.ToBlock == 0 {
		return fmt.Errorf("backfill: to_block is required (> 0)")
	}
	if d.ToBlock < d.FromBlock {
		return fmt.Errorf("backfill: to_block (%d) < from_block (%d)", d.ToBlock, d.FromBlock)
	}
	if d.Store == nil {
		return fmt.Errorf("backfill: store is required")
	}

	started := time.Now()
	slog.Info(
		"backfill: starting",
		"from_block", d.FromBlock,
		"to_block", d.ToBlock,
	)

	var opts []BackfillerOption
	if d.AllowCandidateLoss {
		opts = append(opts, AllowCandidateLoss())
	}
	bf := NewBackfiller(d.Fetcher, d.Store, opts...)
	if err := bf.Run(ctx, d.FromBlock, d.ToBlock); err != nil {
		slog.Error(
			"backfill: failed",
			"err", err,
			"duration_s", int64(time.Since(started).Seconds()),
		)
		return err
	}

	cur, _ := d.Store.GetCursor(ctx)
	slog.Info(
		"backfill: complete",
		"duration_s", int64(time.Since(started).Seconds()),
		"cursor_now", cur,
	)
	return nil
}
