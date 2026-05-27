package base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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
	baseURL string
	token   string
	client  *http.Client
}

// NewHTTPFetcher constructs an HTTPFetcher. token may be empty.
func NewHTTPFetcher(baseURL, token string) *HTTPFetcher {
	return &HTTPFetcher{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
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

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		s.fetcher.baseURL+"/query",
		bytes.NewReader(bs),
	)
	if err != nil {
		return HyperSyncBatch{}, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.fetcher.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.fetcher.token)
	}

	resp, err := s.fetcher.client.Do(req)
	if err != nil {
		return HyperSyncBatch{}, false, fmt.Errorf("post hypersync query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return HyperSyncBatch{}, false, fmt.Errorf("hypersync status %d: %s", resp.StatusCode, string(body))
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
