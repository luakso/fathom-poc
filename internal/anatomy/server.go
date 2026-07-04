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
	// legacy v1 route for stats — removed in Plan C when the new frontend ships
	mux.HandleFunc("GET /api/address/{chain}/{addr}/stats", h.stats)
	mux.Handle("/", spaFileServer(assets))
	// legacyTxRewrite translates the old /api/tx/{chain}/{hash} pattern to the
	// v2 /api/{chain}/tx/{hash} pattern before the mux sees the request. This
	// avoids a panic: both patterns overlap on paths like /api/tx/tx/HASH, which
	// Go's ServeMux rejects at registration time.
	return withTimeout(5*time.Second, legacyTxRewrite(mux))
}

// legacyTxRewrite transparently rewrites GET /api/tx/{chain}/{hash} to the v2
// path /api/{chain}/tx/{hash} so the mux can route it normally. The old URL
// shape is preserved for the frontend until Plan C ships.
func legacyTxRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// /api/tx/{chain}/{hash} splits as ["", "api", "tx", chain, hash]
			parts := strings.SplitN(r.URL.Path, "/", 6)
			if len(parts) == 5 && parts[1] == "api" && parts[2] == "tx" {
				r = r.Clone(r.Context())
				r.URL.Path = "/api/" + parts[3] + "/tx/" + parts[4]
			}
		}
		next.ServeHTTP(w, r)
	})
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
func (h *handler) entityParams(w http.ResponseWriter, r *http.Request) (addr, lens string, ok bool) {
	if !chainOK(r.PathValue("chain")) {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return "", "", false
	}
	var err error
	addr, err = parseAddr(r.PathValue("addr"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	lens, err = parseLens(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return addr, lens, true
}

func (h *handler) meta(w http.ResponseWriter, r *http.Request) {
	if h.p.Meta == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	m, err := h.p.Meta.Meta(r.Context())
	h.respond(w, m, err)
}

func (h *handler) tx(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	hash := r.PathValue("hash")
	if !chainOK(chain) {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	if chain == "base" && !evmHashRe.MatchString(hash) {
		writeErr(w, http.StatusBadRequest, "malformed tx hash")
		return
	}
	if h.p.Dossier == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	g, err := h.p.Dossier.Dossier(r.Context(), chain, strings.ToLower(hash))
	h.respond(w, g, err)
}

func (h *handler) stats(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	addr := r.PathValue("addr")
	if !chainOK(chain) {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	if addr == "" {
		writeErr(w, http.StatusBadRequest, "empty address")
		return
	}
	if h.p.Stats == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s, err := h.p.Stats.Stats(r.Context(), chain, strings.ToLower(addr))
	h.respond(w, s, err)
}

func (h *handler) entity(w http.ResponseWriter, r *http.Request) {
	addr, _, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	if h.p.Entity == nil {
		writeErr(w, http.StatusNotFound, "not found")
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
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.p.Neighbors == nil {
		writeErr(w, http.StatusNotFound, "not found")
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
		writeErr(w, http.StatusNotFound, "not found")
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
		writeErr(w, http.StatusNotFound, "not found")
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

	role := q.Get("role")
	switch role {
	case "payer", "payee", "facilitator":
		// valid
	case "":
		writeErr(w, http.StatusBadRequest, "role is required")
		return
	default:
		writeErr(w, http.StatusBadRequest, "role must be payer, payee, or facilitator")
		return
	}

	sort := q.Get("sort")
	switch sort {
	case "", "volume":
		sort = "volume"
	case "txns", "last_seen":
		// valid
	default:
		writeErr(w, http.StatusBadRequest, "sort must be volume, txns, or last_seen")
		return
	}

	limit, err := parseLimit(r, 50, 200)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var offset int
	if raw := q.Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeErr(w, http.StatusBadRequest, "offset must be >= 0")
			return
		}
	}

	if h.p.Lists == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cq := CounterpartyQuery{Role: role, Lens: lens, Sort: sort, Limit: limit, Offset: offset}
	cp, err := h.p.Lists.Counterparties(r.Context(), r.PathValue("chain"), addr, cq)
	h.respond(w, cp, err)
}

func (h *handler) payments(w http.ResponseWriter, r *http.Request) {
	addr, lens, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()

	role := q.Get("role")
	switch role {
	case "payer", "payee", "facilitator":
		// valid
	case "":
		writeErr(w, http.StatusBadRequest, "role is required")
		return
	default:
		writeErr(w, http.StatusBadRequest, "role must be payer, payee, or facilitator")
		return
	}

	limit, err := parseLimit(r, 25, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	before := q.Get("before")
	if before != "" && !paymentCursorRe.MatchString(before) {
		writeErr(w, http.StatusBadRequest, "malformed before cursor")
		return
	}

	if h.p.Lists == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	pq := PaymentQuery{Role: role, Lens: lens, Limit: limit, Before: before}
	pp, err := h.p.Lists.Payments(r.Context(), r.PathValue("chain"), addr, pq)
	h.respond(w, pp, err)
}

func (h *handler) leaderboard(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	if !chainOK(chain) {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	q := r.URL.Query()

	role := q.Get("role")
	switch role {
	case "payer", "payee":
		// valid
	case "":
		writeErr(w, http.StatusBadRequest, "role is required")
		return
	default:
		writeErr(w, http.StatusBadRequest, "role must be payer or payee")
		return
	}

	window := q.Get("window")
	switch window {
	case "", "all":
		window = "all"
	case "7d", "30d":
		// valid
	default:
		writeErr(w, http.StatusBadRequest, "window must be 7d, 30d, or all")
		return
	}

	sort := q.Get("sort")
	switch sort {
	case "", "volume":
		sort = "volume"
	case "txns", "counterparties":
		// valid
	default:
		writeErr(w, http.StatusBadRequest, "sort must be volume, txns, or counterparties")
		return
	}

	lens, err := parseLens(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, err := parseLimit(r, 100, 500)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = limit // Task 8 trims the Leaderboard rows to this limit

	if h.p.Leaderboard == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	lb, err := h.p.Leaderboard.Leaderboard(r.Context(), chain, role, window, lens, sort)
	h.respond(w, lb, err)
}

func (h *handler) respond(w http.ResponseWriter, payload any, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case err != nil:
		h.log.Error("anatomy request failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, payload)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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
