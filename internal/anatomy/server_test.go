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

func testAssets() fs.FS {
	return fstest.MapFS{"index.html": {Data: []byte("<html>anatomy</html>")}}
}

func newTestServer(d anatomy.DossierProvider) http.Handler {
	return anatomy.NewServer(anatomy.Providers{Dossier: d}, testAssets(), slog.Default())
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
	srv := newTestServer(d)
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/base/tx/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var g anatomy.Graph
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &g))
	require.Equal(t, "0xabc", g.TxHash)
}

func TestServer_TxBadChain(t *testing.T) {
	srv := newTestServer(fakeDossier{})
	req := httptest.NewRequest(http.MethodGet, "/api/ethereum/tx/0xabc", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestServer_TxBadHash(t *testing.T) {
	srv := newTestServer(fakeDossier{})
	req := httptest.NewRequest(http.MethodGet, "/api/base/tx/notahash", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestServer_TxNotFound(t *testing.T) {
	srv := newTestServer(fakeDossier{err: anatomy.ErrNotFound})
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/base/tx/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServer_ServesIndex(t *testing.T) {
	srv := newTestServer(fakeDossier{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "anatomy")
}

func TestServer_Tx500(t *testing.T) {
	d := fakeDossier{err: errors.New("boom")}
	srv := newTestServer(d)
	hash := "0x" + strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/api/base/tx/"+hash, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "internal error", body["error"])
}

func TestServer_SolanaRejected(t *testing.T) {
	srv := newTestServer(fakeDossier{})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/solana/tx/abc123", nil))
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

// fakeLeaderboard is a stub LeaderboardProvider for unit tests.
type fakeLeaderboard struct {
	rows []anatomy.LeaderboardRow
	err  error
}

func (f fakeLeaderboard) Leaderboard(_ context.Context, _, _, _, _, _ string) (anatomy.Leaderboard, error) {
	return anatomy.Leaderboard{Role: "payee", Rows: f.rows}, f.err
}

func TestServer_LeaderboardLimitApplied(t *testing.T) {
	rows := []anatomy.LeaderboardRow{
		{Rank: 1, Address: "0x" + strings.Repeat("1", 40)},
		{Rank: 2, Address: "0x" + strings.Repeat("2", 40)},
		{Rank: 3, Address: "0x" + strings.Repeat("3", 40)},
		{Rank: 4, Address: "0x" + strings.Repeat("4", 40)},
		{Rank: 5, Address: "0x" + strings.Repeat("5", 40)},
	}
	srv := anatomy.NewServer(anatomy.Providers{Leaderboard: fakeLeaderboard{rows: rows}}, testAssets(), slog.Default())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/base/leaderboard?role=payee&limit=2", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var lb anatomy.Leaderboard
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
	require.Len(t, lb.Rows, 2)
}

// fakeLists is a stub ListProvider that never gets called in the overflow test.
type fakeLists struct{}

func (fakeLists) Counterparties(_ context.Context, _, _ string, _ anatomy.CounterpartyQuery) (anatomy.CounterpartyPage, error) {
	return anatomy.CounterpartyPage{}, nil
}

func (fakeLists) Payments(_ context.Context, _, _ string, _ anatomy.PaymentQuery) (anatomy.PaymentPage, error) {
	return anatomy.PaymentPage{}, nil
}

func TestServer_PaymentsOverflowCursor400(t *testing.T) {
	addr := "0x1234567890123456789012345678901234567890"
	overflowCursor := "99999999999999999999:0x" + strings.Repeat("a", 64) + ":0"
	srv := anatomy.NewServer(anatomy.Providers{Lists: fakeLists{}}, testAssets(), slog.Default())
	rec := httptest.NewRecorder()
	url := "/api/base/entity/" + addr + "/payments?role=payer&before=" + overflowCursor
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
