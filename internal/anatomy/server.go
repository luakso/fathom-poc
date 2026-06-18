package anatomy

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

var (
	validChains = map[string]bool{"base": true, "solana": true}
	evmHashRe   = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
)

// NewServer wires the anatomy API routes plus the embedded frontend (assets).
func NewServer(d DossierProvider, s StatsProvider, assets fs.FS, log *slog.Logger) http.Handler {
	h := &handler{d: d, s: s, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tx/{chain}/{hash}", h.tx)
	mux.HandleFunc("GET /api/address/{chain}/{addr}/stats", h.stats)
	mux.Handle("/", spaFileServer(assets))
	return mux
}

type handler struct {
	d   DossierProvider
	s   StatsProvider
	log *slog.Logger
}

func (h *handler) tx(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	hash := r.PathValue("hash")
	if !validChains[chain] {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	if chain == "base" && !evmHashRe.MatchString(hash) {
		writeErr(w, http.StatusBadRequest, "malformed tx hash")
		return
	}
	g, err := h.d.Dossier(r.Context(), chain, strings.ToLower(hash))
	h.respond(w, g, err)
}

func (h *handler) stats(w http.ResponseWriter, r *http.Request) {
	chain := r.PathValue("chain")
	addr := r.PathValue("addr")
	if !validChains[chain] {
		writeErr(w, http.StatusBadRequest, "unknown chain")
		return
	}
	if addr == "" {
		writeErr(w, http.StatusBadRequest, "empty address")
		return
	}
	s, err := h.s.Stats(r.Context(), chain, strings.ToLower(addr))
	h.respond(w, s, err)
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
