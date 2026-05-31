package base_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
)

func TestHTTPFetcher_StreamsBatches(t *testing.T) {
	t.Parallel()

	// Two-batch fixture in HyperSync's real wire shape (`data` is an array).
	// First response covers blocks [100, 109] and sets next_block = 110; second
	// covers [110, 119] and sets next_block = 120 (past to_block), terminating
	// the stream.
	responses := []string{
		`{"data":[{"logs":[],"transactions":[],"blocks":[{"number":100,"timestamp":"0x1","hash":"0xa"},{"number":109,"timestamp":"0x9","hash":"0xb"}]}],"next_block":110}`,
		`{"data":[{"logs":[],"transactions":[],"blocks":[{"number":110,"timestamp":"0xa","hash":"0xc"},{"number":119,"timestamp":"0x13","hash":"0xd"}]}],"next_block":120}`,
	}

	var idx int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/query", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var q base.HyperSyncQuery
		require.NoError(t, json.Unmarshal(body, &q))
		if idx == 0 {
			require.Equal(t, uint64(100), q.FromBlock)
		} else {
			require.Equal(t, uint64(110), q.FromBlock)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responses[idx])
		idx++
	}))
	defer srv.Close()

	f := base.NewHTTPFetcher(srv.URL, "")
	stream, err := f.Stream(base.BuildBackfillQuery(100, 119))
	require.NoError(t, err)
	defer stream.Close()

	var got []base.HyperSyncBatch
	for {
		b, ok, err := stream.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		got = append(got, b)
	}
	require.Len(t, got, 2)
	require.Equal(t, uint64(109), got[0].MaxBlock())
	require.Equal(t, uint64(119), got[1].MaxBlock())
}

func TestHTTPFetcher_SendsBearerToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"logs":[],"transactions":[],"blocks":[]}],"next_block":200}`))
	}))
	defer srv.Close()

	f := base.NewHTTPFetcher(srv.URL, "secret-token")
	stream, err := f.Stream(base.BuildBackfillQuery(100, 199))
	require.NoError(t, err)
	defer stream.Close()
	_, _, err = stream.Next()
	require.NoError(t, err)
}

func TestHTTPFetcher_StreamEndsWhenNextBlockPastToBlock(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"logs":[],"transactions":[],"blocks":[]}],"next_block":200}`))
	}))
	defer srv.Close()

	f := base.NewHTTPFetcher(srv.URL, "")
	stream, err := f.Stream(base.BuildBackfillQuery(100, 199))
	require.NoError(t, err)
	defer stream.Close()
	for {
		_, ok, err := stream.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	require.Equal(t, 1, calls, "stream must stop after next_block > to_block")
}

func TestHTTPFetcher_ServerErrorBubblesUp(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	f := base.NewHTTPFetcher(srv.URL, "")
	stream, err := f.Stream(base.BuildBackfillQuery(100, 199))
	require.NoError(t, err)
	defer stream.Close()
	_, _, err = stream.Next()
	require.Error(t, err)
}

func TestHTTPFetcher_StreamEndsIfServerDoesNotAdvanceCursor(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// next_block == from_block → no advance. Without the guard the loop
		// would re-issue the same query indefinitely.
		_, _ = w.Write([]byte(`{"data":[{"logs":[],"transactions":[],"blocks":[]}],"next_block":100}`))
	}))
	defer srv.Close()

	f := base.NewHTTPFetcher(srv.URL, "")
	stream, err := f.Stream(base.BuildBackfillQuery(100, 199))
	require.NoError(t, err)
	defer stream.Close()
	for {
		_, ok, err := stream.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	require.Equal(t, 1, calls, "non-advancing server must terminate stream after one call")
}
