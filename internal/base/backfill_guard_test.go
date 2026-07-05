package base_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

// candidateNoCompanionBatch builds a batch with one genuine x402 candidate
// (AuthorizationUsed on USDC, parent tx carries a kept selector) but NO
// companion Transfer log — exactly the shape a JoinAll/pairing regression would
// produce: candidates present, zero rows assembled.
func candidateNoCompanionBatch() base.HyperSyncBatch {
	b := fixtureBatch()
	// Drop the companion Transfer (index 1), keep the AuthorizationUsed (index 0).
	b.Data.Logs = b.Data.Logs[:1]
	return b
}

// An interrupted run must exit non-zero: Run covers [from, to] and a ctx
// cancellation means the range is unfinished. Returning nil here made a
// SIGTERM'd partial backfill indistinguishable from success for shell callers
// (`just backfill && rollup`).
func TestBackfill_Run_CanceledContextReturnsError(t *testing.T) {
	// Nil-pool store: cancellation is detected before any store access.
	store := base.NewStore(nil)
	f := &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}}
	bf := base.NewBackfiller(f, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bf.Run(ctx, 100, 100)
	require.ErrorIs(t, err, context.Canceled, "interrupted run must surface cancellation, not report success")
}

func TestBackfill_Run_HaltsWhenAllCandidatesDrop(t *testing.T) {
	// Store with a nil pool: the guard must fire BEFORE InsertBatch, so the
	// store is never touched. If the guard regresses, this nil-deref panics —
	// a loud, useful failure.
	store := base.NewStore(nil)
	f := &fakeFetcher{batches: []base.HyperSyncBatch{candidateNoCompanionBatch()}}
	bf := base.NewBackfiller(f, store)

	err := bf.Run(context.Background(), 100, 100)
	require.Error(t, err, "a batch with candidates but zero kept rows must halt, not silently advance")
	require.Contains(t, strings.ToLower(err.Error()), "0 rows")
}
