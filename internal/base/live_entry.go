package base

import (
	"context"
	"fmt"
	"time"
)

// LiveDeps bundles the runtime arguments for one live-tail invocation.
type LiveDeps struct {
	Client            Client
	Store             *Store
	PollInterval      time.Duration
	BlockBatchSize    uint64
	Concurrency       int64
	ConfirmationDepth uint64
}

// RunLive validates dependencies and blocks driving the Tailer until ctx is
// cancelled (returns nil) or a fail-fast error occurs (returns the error).
func RunLive(ctx context.Context, d LiveDeps) error {
	if d.Client == nil {
		return fmt.Errorf("live: client is required")
	}
	if d.Store == nil {
		return fmt.Errorf("live: store is required")
	}
	tailer := NewTailer(d.Client, d.Store, TailerOptions{
		PollInterval:      d.PollInterval,
		BlockBatchSize:    d.BlockBatchSize,
		Concurrency:       d.Concurrency,
		ConfirmationDepth: d.ConfirmationDepth,
	})
	return tailer.Run(ctx)
}
