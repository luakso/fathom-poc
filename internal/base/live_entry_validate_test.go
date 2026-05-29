package base_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

// validateStubClient is a do-nothing base.Client used only to exercise the
// dependency-validation branches of RunLive, which return before any of these
// methods are called. (The integration tests use the richer stubRPC.)
type validateStubClient struct{}

func (validateStubClient) BlockNumber(context.Context) (uint64, error) { return 0, nil }
func (validateStubClient) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (validateStubClient) BlockByNumber(context.Context, uint64) (*types.Block, error) {
	return nil, nil
}

func (validateStubClient) BlockReceipts(context.Context, uint64) ([]*types.Receipt, error) {
	return nil, nil
}
func (validateStubClient) Close() {}

// RunLive's dependency validation returns before any RPC or DB access, so
// these cases run under plain `go test` (no testcontainers / Docker needed).
func TestRunLive_ValidatesDeps(t *testing.T) {
	t.Parallel()

	t.Run("nil client", func(t *testing.T) {
		t.Parallel()
		err := base.RunLive(context.Background(), base.LiveDeps{Client: nil})
		require.Error(t, err)
	})

	t.Run("nil store", func(t *testing.T) {
		t.Parallel()
		// A non-nil client but nil store must still be rejected before the
		// Tailer is constructed.
		err := base.RunLive(context.Background(), base.LiveDeps{
			Client: &validateStubClient{},
			Store:  nil,
		})
		require.Error(t, err)
	})
}
