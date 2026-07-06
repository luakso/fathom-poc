package anatomy

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var evmHashRe = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// NewServer wires the anatomy API routes plus the embedded frontend (assets).
func NewServer(p Providers, assets fs.FS, log *slog.Logger) http.Handler {
	h := &handler{p: p, log: log}
	mux := http.NewServeMux()
	// v2 routes (spec §5)
	mux.HandleFunc("GET /api/meta", h.meta)
	mux.HandleFunc("GET /api/{chain}/tx/{hash}", h.tx)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}", h.entity)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}/neighbors", h.neighbors)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}/timeline", h.timeline)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}/fingerprint", h.fingerprint)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}/counterparties", h.counterparties)
	mux.HandleFunc("GET /api/{chain}/entity/{addr}/payments", h.payments)
	mux.HandleFunc("GET /api/{chain}/leaderboard", h.leaderboard)
	mux.Handle("/", spaFileServer(assets))
	return withTimeout(5*time.Second, mux)
}

// withTimeout bounds every request; providers read precomputed tables, so
// hitting this is a signal (slow query, rollup contention), not a UX issue.
func withTimeout(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type handler struct {
	p   Providers
	log *slog.Logger
}

// entityParams centralizes chain+addr+lens validation for entity routes.
func (h *handler) entityParams(w http.ResponseWriter, r *http.Request) (addr string, lens Lens, ok bool) {
	if !chainOK(r.PathValue("chain")) {
		h.writeErr(w, http.StatusBadRequest, "unknown chain")
		return "", "", false
	}
	var err error
	addr, err = parseAddr(r.PathValue("addr"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	lens, err = parseLens(r)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return addr, lens, true
}

func (h *handler) meta(w http.ResponseWriter, r *http.Request) {
	if h.p.Meta == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	m, err := h.p.Meta.Meta(r.Context())
	h.respond(w, m, err)
}

func (h *handler) tx(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	hash := r.PathValue("hash")
	if !chainOK(chain) {
		h.writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	// validChains only admits "base", so any accepted chain is EVM here; the
	// hash must be a 32-byte EVM tx hash. (A future non-EVM chain needs its own
	// hash-shape branch.)
	if !evmHashRe.MatchString(hash) {
		h.writeErr(w, http.StatusBadRequest, "malformed tx hash")
		return
	}
	if h.p.Dossier == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	g, err := h.p.Dossier.Dossier(r.Context(), chain, strings.ToLower(hash))
	h.respond(w, g, err)
}

func (h *handler) entity(w http.ResponseWriter, r *http.Request) {
	addr, _, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	if h.p.Entity == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	e, err := h.p.Entity.Entity(r.Context(), r.PathValue("chain"), addr)
	h.respond(w, e, err)
}

func (h *handler) neighbors(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 8, 50)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.p.Neighbors == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	n, err := h.p.Neighbors.Neighbors(r.Context(), r.PathValue("chain"), addr, lens, limit)
	h.respond(w, n, err)
}

func (h *handler) timeline(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	if h.p.Activity == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	t, err := h.p.Activity.Timeline(r.Context(), r.PathValue("chain"), addr, lens)
	h.respond(w, t, err)
}

func (h *handler) fingerprint(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	if h.p.Activity == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	f, err := h.p.Activity.Fingerprint(r.Context(), r.PathValue("chain"), addr, lens)
	h.respond(w, f, err)
}

func (h *handler) counterparties(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()

	role, err := parseRole(q.Get("role"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sortKey, err := parseCounterpartySort(q.Get("sort"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, err := parseLimit(r, 50, 200)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var offset int
	if raw := q.Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			h.writeErr(w, http.StatusBadRequest, "offset must be >= 0")
			return
		}
	}

	if h.p.Lists == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cq := CounterpartyQuery{Role: role, Lens: lens, Sort: sortKey, Limit: limit, Offset: offset}
	cp, err := h.p.Lists.Counterparties(r.Context(), r.PathValue("chain"), addr, cq)
	h.respond(w, cp, err)
}

func (h *handler) payments(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()

	role, err := parseRole(q.Get("role"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, err := parseLimit(r, 25, 100)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	before := q.Get("before")
	if before != "" && !paymentCursorRe.MatchString(before) {
		h.writeErr(w, http.StatusBadRequest, "malformed before cursor")
		return
	}
	// Parse the cursor in the handler to catch int64 overflow before dispatch.
	// The provider parses again as a second line of defence.
	if _, _, _, _, cursorErr := parseCursor(before); cursorErr != nil {
		h.writeErr(w, http.StatusBadRequest, "malformed cursor")
		return
	}

	if h.p.Lists == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	pq := PaymentQuery{Role: role, Lens: lens, Limit: limit, Before: before}
	pp, err := h.p.Lists.Payments(r.Context(), r.PathValue("chain"), addr, pq)
	h.respond(w, pp, err)
}

func (h *handler) leaderboard(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	if !chainOK(chain) {
		h.writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	q := r.URL.Query()

	role, err := parseLeaderboardRole(q.Get("role"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	window, err := parseWindow(q.Get("window"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sortKey, err := parseLeaderboardSort(q.Get("sort"))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	lens, err := parseLens(r)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, err := parseLimit(r, 100, 500)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.p.Leaderboard == nil {
		h.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	lb, err := h.p.Leaderboard.Leaderboard(r.Context(), chain, role, window, lens, sortKey)
	if err == nil && len(lb.Rows) > limit {
		lb.Rows = lb.Rows[:limit]
	}
	h.respond(w, lb, err)
}

func (h *handler) respond(w http.ResponseWriter, payload any, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.writeErr(w, http.StatusNotFound, "not found")
	case err != nil:
		h.log.Error("anatomy request failed", "err", err)
		h.writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		h.writeJSON(w, http.StatusOK, payload)
	}
}

// errorResponse is the JSON error envelope: {"error":"..."}.
type errorResponse struct {
	Error string `json:"error"`
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// Status and headers are already sent, so a failed encode cannot change the
	// response code; log it so a truncated body is diagnosable.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Error("anatomy: encode response failed", "err", err)
	}
}

func (h *handler) writeErr(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, errorResponse{Error: msg})
}

// spaFileServer serves assets, falling back to index.html for unknown paths so
// the single-page app can client-route. Real assets are served directly;
// everything else rewrites to index.html so the SPA can client-route.
func spaFileServer(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(assets, strings.TrimPrefix(r.URL.Path, "/")); err != nil && r.URL.Path != "/" {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
