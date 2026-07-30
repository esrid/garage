package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/esrid/garage/assets"
	"github.com/esrid/garage/internal/adapters/handlers"
)

type readinessChecker interface {
	Check(context.Context) error
}

type handler struct {
	readiness readinessChecker
}

func New(readiness readinessChecker) http.Handler {
	h := &handler{readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)

	// TODO(F02A): FixtureToday is presentation fixture data. When the real
	// provider exists, inject it from the DI root instead of constructing it
	// here. It is built here on purpose for now: changing New's signature would
	// touch the DI root, which Agent A holds.
	dashboard := handlers.NewDashboard(handlers.FixtureToday{})
	mux.HandleFunc("GET /app", dashboard.Page)
	mux.HandleFunc("GET /app/today", dashboard.Fragment)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets.Static())))

	return requestID(recoverPanic(accessLog(securityHeaders(mux))))
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := h.readiness.Check(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
