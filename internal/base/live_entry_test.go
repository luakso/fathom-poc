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

// This proves RunLive wires LiveDeps -> TailerOptions and propagates the
// nil-on-cancel return. The loop behaviour itself (cursor advance, confirmation
// depth) is covered by the Tailer tests; here we only care about the wiring.
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
