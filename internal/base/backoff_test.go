package base

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithRateLimitBackoff_SuccessFirstTry(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		return nil
	}, instantSleeps())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestWithRateLimitBackoff_RetriesOnRateLimit(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("HTTP 429 Too Many Requests")
		}
		return nil
	}, instantSleeps())
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestWithRateLimitBackoff_GivesUpAfterMaxRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		return errors.New("429 rate limit")
	}, instantSleeps())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "rate limit"))
	require.Equal(t, 6, calls, "1 initial + 5 retries = 6 total")
}

func TestWithRateLimitBackoff_FailsFastOnNonRateLimit(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		return errors.New("schema validation failed")
	}, instantSleeps())
	require.Error(t, err)
	require.Equal(t, 1, calls, "non-rate-limit errors must not retry")
}

func TestWithRateLimitBackoff_CtxCancelDuringSleep(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Deliberately a non-zero sleep: the cancelled ctx.Done() is the only
	// ready select case, so cancellation is chosen deterministically. Using a
	// zero-duration sleep here would make time.After(0) ready too and let the
	// select pick randomly between the two — a ~50% flake. Do not "simplify"
	// this to instantSleeps().
	err := WithRateLimitBackoff(ctx, func() error {
		return errors.New("429")
	}, []time.Duration{time.Second})
	require.Error(t, err)
}

func TestWithRateLimitBackoff_StopsRetryingWhenErrorShapeChanges(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		if calls == 1 {
			return errors.New("429 rate limit")
		}
		return errors.New("schema validation failed")
	}, instantSleeps())
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema validation failed")
	require.Equal(t, 2, calls, "retry once for the 429, then fail fast on the non-rate-limit error")
}

func TestWithRateLimitBackoff_EmptySleepsMeansZeroRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	err := WithRateLimitBackoff(context.Background(), func() error {
		calls++
		return errors.New("429 rate limit")
	}, nil)
	require.Error(t, err)
	require.Equal(t, 1, calls, "no sleeps means no retries — one call then exhausted")
}

func instantSleeps() []time.Duration {
	return []time.Duration{0, 0, 0, 0, 0}
}
