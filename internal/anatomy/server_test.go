package anatomy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

type fakeDossier struct {
	g   anatomy.Graph
	err error
}

func (f fakeDossier) Dossier(_ context.Context, _, _ string) (anatomy.Graph, error) {
	return f.g, f.err
}

type fakeStats struct {
	s   anatomy.Stats
	err error
}

func (f fakeStats) Stats(_ context.Context, _, _ string) (anatomy.Stats, error) {
	return f.s, f.err
}

func testAssets() fs.FS {
	return fstest.MapFS{"index.html": {Data: []byte("<html>anatomy</html>")}}
}

func newTestServer(d anatomy.DossierProvider, s anatomy.StatsProvider) http.Handler {
	return anatomy.NewServer(anatomy.Providers{Dossier: d, Stats: s}, testAssets(), slog.Default())
}

type fakeMeta struct{ m anatomy.Meta }

func (f fakeMeta) Meta(context.Context) (anatomy.Meta, error) { return f.m, nil }

func TestServer_MetaEndpoint(t *testing.T) {
	srv := anatomy.NewServer(anatomy.Providers{
		Meta: fakeMeta{anatomy.Meta{DataMaxDay: "2026-06-06", MethodologyVersion: 1}},
	}, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, slog.Default())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/api/meta", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"dataMaxDay":"2026-06-06"`)
}

func TestServer_BadLensRejected(t *testing.T) {
	srv := anatomy.NewServer(anatomy.Providers{}, fstest.MapFS{}, slog.Default())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/api/base/entity/0x1234567890123456789012345678901234567890?lens=bogus", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestServer_TxOK(t *testing.T) {
	d := fakeDossier{g: anatomy.Graph{Chain: "base", TxHash: "0xabc"}}
	srv := newTestServer(d, fakeStats{})
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/tx/base/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var g anatomy.Graph
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &g))
	require.Equal(t, "0xabc", g.TxHash)
}

func TestServer_TxBadChain(t *testing.T) {
	srv := newTestServer(fakeDossier{}, fakeStats{})
	req := httptest.NewRequest(http.MethodGet, "/api/tx/ethereum/0xabc", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestServer_TxBadHash(t *testing.T) {
	srv := newTestServer(fakeDossier{}, fakeStats{})
	req := httptest.NewRequest(http.MethodGet, "/api/tx/base/notahash", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestServer_TxNotFound(t *testing.T) {
	srv := newTestServer(fakeDossier{err: anatomy.ErrNotFound}, fakeStats{})
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/tx/base/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServer_StatsOK(t *testing.T) {
	s := fakeStats{s: anatomy.Stats{Address: "0xhero", PaymentCount: 3}}
	srv := newTestServer(fakeDossier{}, s)
	req := httptest.NewRequest(http.MethodGet, "/api/address/base/0xHERO/stats", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestServer_ServesIndex(t *testing.T) {
	srv := newTestServer(fakeDossier{}, fakeStats{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "anatomy")
}

func TestServer_Tx500(t *testing.T) {
	d := fakeDossier{err: errors.New("boom")}
	srv := newTestServer(d, fakeStats{})
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/tx/base/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "internal error", body["error"])
}

func TestServer_SolanaRejected(t *testing.T) {
	srv := newTestServer(fakeDossier{}, fakeStats{})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tx/solana/abc123", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

type fakeEntity struct {
	e   anatomy.Entity
	err error
}

func (f fakeEntity) Entity(_ context.Context, _, _ string) (anatomy.Entity, error) {
	return f.e, f.err
}

func TestServer_EntityOK(t *testing.T) {
	addr := "0x1234567890123456789012345678901234567890"
	fe := fakeEntity{e: anatomy.Entity{Chain: "base", Address: addr, Roles: []string{"payer"}}}
	srv := anatomy.NewServer(anatomy.Providers{Entity: fe}, testAssets(), slog.Default())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/base/entity/"+addr, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), addr)
}

func TestServer_EntityNotFound(t *testing.T) {
	fe := fakeEntity{err: anatomy.ErrNotFound}
	srv := anatomy.NewServer(anatomy.Providers{Entity: fe}, testAssets(), slog.Default())
	rec := httptest.NewRecorder()
	addr := "0x1234567890123456789012345678901234567890"
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/base/entity/"+addr, nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
