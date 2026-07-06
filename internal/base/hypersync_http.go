package base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Default 429 backoff policy. The public Base HyperSync endpoint rate-limits
// callers hard (even with a free token); wide backfills fire thousands of
// sequential batches, so a 429 is transient and worth riding out rather than
// failing the whole run. Delays double from baseDelay (1s, 2s, 4s, …) capped at
// maxRetryDelay, with ±20% jitter — a ~5min total budget per batch, matching the
// official hypersync-client's resilience. Backfill commits a cursor per batch,
// so even an exhausted-retry failure resumes cleanly on re-run.
const (
	defaultMaxRetries = 10
	defaultBaseDelay  = 1 * time.Second
	maxRetryDelay     = 60 * time.Second
)

// HTTPFetcher posts HyperSync queries against {baseURL}/query and parses one
// JSON batch per response. It tracks the cursor across calls via the
// `next_block` field returned in each batch.
//
// Bearer token is optional (HyperSync's public Base endpoint is anonymous;
// tiered access uses Authorization: Bearer <token>).
//
// The HTTP client uses a generous timeout — backfill batches can be large.
type HTTPFetcher struct {
	baseURL    string
	token      string
	client     *http.Client
	maxRetries int           // retries on HTTP 429 before giving up
	baseDelay  time.Duration // first backoff delay; doubles each retry
	// sleep is the injectable (for tests) cancellable wait used between retries.
	// It returns ctx.Err() if ctx is cancelled before the delay elapses, so a
	// SIGTERM during a multi-minute backoff aborts the batch instead of waiting
	// it out. Defaults to ctxSleep.
	sleep func(context.Context, time.Duration) error
}

// ctxSleep waits d, or returns early with ctx.Err() if ctx is cancelled first.
func ctxSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// FetcherOption customizes an HTTPFetcher.
type FetcherOption func(*HTTPFetcher)

// WithRetry overrides the HTTP 429 backoff policy. maxRetries bounds the number
// of retries after the initial attempt; baseDelay is the first wait and doubles
// each retry (capped at maxRetryDelay). A server-sent Retry-After header, when
// present, takes precedence over the computed backoff.
func WithRetry(maxRetries int, baseDelay time.Duration) FetcherOption {
	return func(f *HTTPFetcher) {
		f.maxRetries = maxRetries
		f.baseDelay = baseDelay
	}
}

// NewHTTPFetcher constructs an HTTPFetcher. token may be empty.
func NewHTTPFetcher(baseURL, token string, opts ...FetcherOption) *HTTPFetcher {
	f := &HTTPFetcher{
		baseURL:    baseURL,
		token:      token,
		client:     &http.Client{Timeout: 5 * time.Minute},
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		sleep:      ctxSleep,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Stream issues the initial query and returns a Stream that iterates batches.
// Per-batch HTTP errors are surfaced via Next's error return; callers are
// expected to fail-fast (the run is re-invocable from the last committed
// cursor — see spec §11).
func (f *HTTPFetcher) Stream(_ context.Context, query HyperSyncQuery) (Stream, error) {
	// Stream construction does no IO; each Next carries its own ctx for the fetch.
	return &httpStream{
		fetcher: f,
		query:   query,
		toBlock: query.ToBlock,
		next:    query.FromBlock,
	}, nil
}

type httpStream struct {
	fetcher *HTTPFetcher
	query   HyperSyncQuery
	toBlock uint64
	next    uint64
	done    bool
}

func (s *httpStream) Next(ctx context.Context) (HyperSyncBatch, bool, error) {
	if s.done {
		return HyperSyncBatch{}, false, nil
	}
	q := s.query
	q.FromBlock = s.next
	q.ToBlock = s.toBlock

	bs, err := json.Marshal(q)
	if err != nil {
		return HyperSyncBatch{}, false, fmt.Errorf("marshal query: %w", err)
	}

	body, err := s.fetcher.postWithRetry(ctx, bs)
	if err != nil {
		return HyperSyncBatch{}, false, err
	}

	var batch HyperSyncBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		return HyperSyncBatch{}, false, fmt.Errorf("decode batch: %w", err)
	}

	// Determine the next cursor position. toBlock is the EXCLUSIVE wire bound
	// (see BuildBackfillQuery), so next_block == toBlock means the range is
	// fully covered.
	if batch.NextBlock <= s.next {
		// Server didn't advance while the range is unfinished. Stopping quietly
		// here would report an arbitrarily large uncovered tail as success, so
		// halt the stream AND fail loudly; the run is re-invocable from the last
		// committed cursor once the cause is understood.
		s.done = true
		return HyperSyncBatch{}, false, fmt.Errorf(
			"hypersync did not advance: next_block %d <= cursor %d with to_block %d unreached (archive_height %d)",
			batch.NextBlock, s.next, s.toBlock, batch.ArchiveHeight,
		)
	}
	s.next = batch.NextBlock
	if s.next >= s.toBlock {
		s.done = true
	}
	return batch, true, nil
}

func (s *httpStream) Close() error {
	s.done = true
	return nil
}

// postWithRetry posts the query body to {baseURL}/query, retrying transient
// failures with exponential backoff. Two things are retried:
//   - HTTP 429 (rate limit), honoring a Retry-After header when present, and
//   - transport errors (connection reset, unexpected EOF, timeout, DNS blip) —
//     common over a multi-hour backfill, and fatal to the whole run if not
//     ridden out.
//
// Other non-200 statuses (4xx/5xx) and any failure outlasting maxRetries surface
// to the caller as an error. A successful (200) response returns its body.
func (f *HTTPFetcher) postWithRetry(ctx context.Context, query []byte) ([]byte, error) {
	delay := f.baseDelay
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, status, retryAfter, err := f.post(ctx, query)
		switch {
		case err != nil:
			// Transport-level failure — retryable (unless ctx was cancelled).
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if attempt >= f.maxRetries {
				return nil, err
			}
			if delay, err = f.backoff(ctx, delay, 0, attempt, err.Error()); err != nil {
				return nil, err
			}
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusTooManyRequests && attempt < f.maxRetries:
			if delay, err = f.backoff(ctx, delay, retryAfter, attempt, "http 429"); err != nil {
				return nil, err
			}
		default:
			// Non-retryable status, or retries exhausted.
			return nil, fmt.Errorf("hypersync status %d: %s", status, string(body))
		}
	}
}

// backoff sleeps before the next retry and returns the next delay (doubled).
// The wait is jittered ±20%, capped at maxRetryDelay; a server Retry-After (>0)
// overrides the computed wait precisely (no jitter). reason is logged so a long
// stall is explained (rate limit vs network drop).
// It returns the doubled delay for the next attempt, and ctx.Err() if the wait
// was cut short by cancellation (so the caller aborts instead of retrying).
func (f *HTTPFetcher) backoff(ctx context.Context, delay, retryAfter time.Duration, attempt int, reason string) (time.Duration, error) {
	wait := jitter(delay)
	if retryAfter > 0 {
		wait = retryAfter
	}
	if wait > maxRetryDelay {
		wait = maxRetryDelay
	}
	slog.Warn(
		"hypersync: transient failure, backing off",
		"reason", reason,
		"attempt", attempt+1,
		"max_retries", f.maxRetries,
		"wait", wait.Round(time.Millisecond).String(),
	)
	if err := f.sleep(ctx, wait); err != nil {
		return delay, err
	}
	return delay * 2, nil
}

// post issues one POST and returns the body, HTTP status, and parsed
// Retry-After delay (0 when absent/unparseable). A transport-level failure is
// returned as err; an HTTP error status is reported via status, not err, so the
// caller can decide whether it is retryable.
func (f *HTTPFetcher) post(ctx context.Context, query []byte) (body []byte, status int, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		f.baseURL+"/query",
		bytes.NewReader(query),
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("post hypersync query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read response body: %w", err)
	}
	return body, resp.StatusCode, parseRetryAfter(resp.Header.Get("Retry-After")), nil
}

// jitter applies ±20% randomization to a backoff delay so successive retries
// don't reconverge in lockstep (and concurrent collectors don't synchronize
// their retry storms). Matches the official hypersync-client's approach.
func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + rand.Float64()*0.4)) //nolint:gosec // backoff jitter needs no cryptographic randomness
}

// parseRetryAfter reads the delay-seconds form of the Retry-After header.
// Returns 0 when the header is absent or not a non-negative integer (the
// HTTP-date form is unused by HyperSync), in which case the caller falls back
// to exponential backoff.
func parseRetryAfter(v string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
