package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/esrid/garage/assets"
	"github.com/esrid/garage/internal/adapters/handlers"
	"github.com/esrid/garage/internal/adapters/voice"
)

type readinessChecker interface {
	Check(context.Context) error
}

type handler struct {
	readiness readinessChecker
}

func New(readiness readinessChecker, dashboard *handlers.Dashboard, appointments *handlers.AppointmentMutations, customerLookup *voice.CustomerLookup) http.Handler {
	h := &handler{readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)

	mux.HandleFunc("GET /app", dashboard.Page)
	mux.HandleFunc("GET /app/today", dashboard.Fragment)
	mux.HandleFunc("POST /app/appointments", appointments.Book)
	mux.HandleFunc("POST /app/appointments/{id}/reschedule", appointments.Reschedule)
	mux.HandleFunc("POST /app/appointments/{id}/cancel", appointments.Cancel)
	mux.Handle("POST /voice/tools/customer-lookup", customerLookup)

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
