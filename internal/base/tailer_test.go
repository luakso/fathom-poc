//go:build integration

package base_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

type stubRPC struct {
	tip uint64
}

func (s *stubRPC) BlockNumber(_ context.Context) (uint64, error) { return s.tip, nil }
func (s *stubRPC) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (s *stubRPC) BlockByNumber(_ context.Context, _ uint64) (*types.Block, error) { return nil, nil }

func (s *stubRPC) BlockReceipts(_ context.Context, _ uint64) ([]*types.Receipt, error) {
	return nil, nil
}
func (s *stubRPC) Close() {}

func TestTailer_StopsOnContextCancel(t *testing.T) {
	ctx, store := setupStore(t)

	cli := &stubRPC{tip: 200}
	tailer := base.NewTailer(cli, store, base.TailerOptions{
		PollInterval:      10 * time.Millisecond,
		BlockBatchSize:    100,
		Concurrency:       4,
		ConfirmationDepth: 6,
	})

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- tailer.Run(runCtx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "Run must return nil on ctx cancel, not bubble ctx.Err")
	case <-time.After(2 * time.Second):
		t.Fatal("tailer did not return after ctx cancel")
	}
}

func TestTailer_AdvancesCursorOnEmptyRange(t *testing.T) {
	ctx, store := setupStore(t)

	cli := &stubRPC{tip: 300}
	tailer := base.NewTailer(cli, store, base.TailerOptions{
		PollInterval:      5 * time.Millisecond,
		BlockBatchSize:    50,
		Concurrency:       2,
		ConfirmationDepth: 6,
	})
	runCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	require.NoError(t, tailer.Run(runCtx))

	cur, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Greater(t, cur, uint64(0), "cursor must have advanced past 0 even with no matching logs")
	require.LessOrEqual(t, cur, uint64(294), "cursor must respect confirmation depth (tip 300 - depth 6 = safe tip 294)")
}
