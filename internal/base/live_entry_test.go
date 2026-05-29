//go:build integration

package base_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

func TestRunLive_ValidatesClient(t *testing.T) {
	t.Parallel()
	err := base.RunLive(context.Background(), base.LiveDeps{Client: nil})
	require.Error(t, err)
}

func TestRunLive_ReturnsCleanlyOnCtxCancel(t *testing.T) {
	ctx, store := setupStore(t)

	deps := base.LiveDeps{
		Client:            &stubRPC{tip: 300},
		Store:             store,
		PollInterval:      5 * time.Millisecond,
		BlockBatchSize:    50,
		Concurrency:       2,
		ConfirmationDepth: 6,
	}
	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	require.NoError(t, base.RunLive(runCtx, deps))
}
