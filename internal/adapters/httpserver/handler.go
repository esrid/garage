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

func New(readiness readinessChecker, dashboard *handlers.Dashboard, planning *handlers.Planning, appointments *handlers.AppointmentMutations, customerLookup *voice.CustomerLookup, appointmentTools *voice.AppointmentTools, followUpTool *voice.FollowUpTool, authentication *handlers.Authentication, sessions sessionVerifier) http.Handler {
	h := &handler{readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)

	appMux := http.NewServeMux()
	appMux.HandleFunc("GET /app", dashboard.Page)
	appMux.HandleFunc("GET /app/today", dashboard.Fragment)
	appMux.HandleFunc("GET /app/planning", planning.Page)
	appMux.HandleFunc("GET /app/planning/day", planning.Fragment)
	appMux.HandleFunc("POST /app/appointments", appointments.Book)
	appMux.HandleFunc("POST /app/appointments/{id}/reschedule", appointments.Reschedule)
	appMux.HandleFunc("POST /app/appointments/{id}/cancel", appointments.Cancel)
	protectedApp := requireStaffSession(sessions, appMux)
	mux.Handle("/app", protectedApp)
	mux.Handle("/app/", protectedApp)

	mux.HandleFunc("POST /auth/login", authentication.Login)
	mux.HandleFunc("POST /auth/logout", authentication.Logout)
	mux.Handle("POST /voice/tools/customer-lookup", customerLookup)
	mux.HandleFunc("POST /voice/tools/appointment-availability", appointmentTools.Availability)
	mux.HandleFunc("POST /voice/tools/appointment-book", appointmentTools.Book)
	mux.Handle("POST /voice/tools/follow-up-request", followUpTool)

	// The public site (F07) owns "/" and the legal pages. It has no dependencies,
	// so it is built here instead of the DI root, which Agent A holds for F05.
	handlers.NewSite().Register(mux)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets.Static())))

	return requestID(recoverPanic(accessLog(securityHeaders(crossOriginProtection(mux)))))
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
