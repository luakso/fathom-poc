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
	maxRetries int                 // retries on HTTP 429 before giving up
	baseDelay  time.Duration       // first backoff delay; doubles each retry
	sleep      func(time.Duration) // injectable for tests; defaults to time.Sleep
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
		sleep:      time.Sleep,
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
func (f *HTTPFetcher) Stream(query HyperSyncQuery) (Stream, error) {
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

func (s *httpStream) Next() (HyperSyncBatch, bool, error) {
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

	body, err := s.fetcher.postWithRetry(bs)
	if err != nil {
		return HyperSyncBatch{}, false, err
	}

	var batch HyperSyncBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		return HyperSyncBatch{}, false, fmt.Errorf("decode batch: %w", err)
	}

	// Determine the next cursor position.
	if batch.NextBlock > s.next {
		s.next = batch.NextBlock
	} else {
		// Server didn't advance — treat as terminal to avoid infinite loop.
		s.done = true
	}
	if s.next > s.toBlock {
		s.done = true
	}
	return batch, true, nil
}

func (s *httpStream) Close() error {
	s.done = true
	return nil
}

// postWithRetry posts the query body to {baseURL}/query, retrying on HTTP 429
// with exponential backoff (honoring a Retry-After header when the server sends
// one). Non-429 failures, and 429s that outlast maxRetries, surface to the
// caller as an error. A successful (200) response returns its body.
func (f *HTTPFetcher) postWithRetry(query []byte) ([]byte, error) {
	delay := f.baseDelay
	for attempt := 0; ; attempt++ {
		body, status, retryAfter, err := f.post(query)
		if err != nil {
			return nil, err
		}
		if status == http.StatusOK {
			return body, nil
		}
		if status != http.StatusTooManyRequests || attempt >= f.maxRetries {
			return nil, fmt.Errorf("hypersync status %d: %s", status, string(body))
		}
		wait := jitter(delay)
		if retryAfter > 0 {
			wait = retryAfter // honor the server's explicit value precisely (no jitter)
		}
		if wait > maxRetryDelay {
			wait = maxRetryDelay
		}
		slog.Warn(
			"hypersync rate-limited (429); backing off",
			"attempt", attempt+1,
			"max_retries", f.maxRetries,
			"wait", wait.Round(time.Millisecond).String(),
		)
		f.sleep(wait)
		delay *= 2
	}
}

// post issues one POST and returns the body, HTTP status, and parsed
// Retry-After delay (0 when absent/unparseable). A transport-level failure is
// returned as err; an HTTP error status is reported via status, not err, so the
// caller can decide whether it is retryable.
func (f *HTTPFetcher) post(query []byte) (body []byte, status int, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
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
