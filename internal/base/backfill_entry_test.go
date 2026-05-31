//go:build integration

package base_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

func TestRunBackfill_HappyPath(t *testing.T) {
	ctx, store := setupStore(t)

	deps := base.BackfillDeps{
		Fetcher:   &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}},
		Store:     store,
		FromBlock: 100,
		ToBlock:   100,
	}
	require.NoError(t, base.RunBackfill(ctx, deps))

	cur, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cur)
}

func TestRunBackfill_ValidatesFromBlock(t *testing.T) {
	t.Parallel()
	deps := base.BackfillDeps{
		Fetcher:   &fakeFetcher{},
		Store:     nil, // unused on validation failure
		FromBlock: 0,
		ToBlock:   100,
	}
	require.Error(t, base.RunBackfill(context.Background(), deps))
}

func TestRunBackfill_ValidatesToBlock(t *testing.T) {
	t.Parallel()
	deps := base.BackfillDeps{
		Fetcher:   &fakeFetcher{},
		Store:     nil, // unused: to_block check fires before the store check
		FromBlock: 100,
		ToBlock:   0,
	}
	err := base.RunBackfill(context.Background(), deps)
	require.ErrorContains(t, err, "to_block")
}
