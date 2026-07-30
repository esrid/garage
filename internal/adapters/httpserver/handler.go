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

// Deps is everything the router mounts. A struct rather than a parameter list:
// this signature grew to ten positional arguments in a day, every feature had to
// touch it, and two of them are only distinguishable by their type. Named fields
// make a call site readable and a new route a one-line change.
type Deps struct {
	Readiness      readinessChecker
	Sessions       sessionVerifier
	Authentication *handlers.Authentication

	// Behind the staff session.
	Dashboard    *handlers.Dashboard
	Calls        *handlers.Calls
	Planning     *handlers.Planning
	Appointments *handlers.AppointmentMutations

	// Authenticated by their own tenant-scoped bearer token or signature.
	CustomerLookup   *voice.CustomerLookup
	AppointmentTools *voice.AppointmentTools
	FollowUpTool     *voice.FollowUpTool
	PostCallWebhook  *voice.PostCallWebhook
}

func New(deps Deps) http.Handler {
	h := &handler{readiness: deps.Readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)

	appMux := http.NewServeMux()
	appMux.HandleFunc("GET /app", deps.Dashboard.Page)
	appMux.HandleFunc("GET /app/today", deps.Dashboard.Fragment)
	deps.Calls.Register(appMux)
	appMux.HandleFunc("GET /app/planning", deps.Planning.Page)
	appMux.HandleFunc("GET /app/planning/day", deps.Planning.Fragment)
	appMux.HandleFunc("POST /app/appointments", deps.Appointments.Book)
	appMux.HandleFunc("POST /app/appointments/{id}/reschedule", deps.Appointments.Reschedule)
	appMux.HandleFunc("POST /app/appointments/{id}/cancel", deps.Appointments.Cancel)
	protectedApp := requireStaffSession(deps.Sessions, appMux)
	mux.Handle("/app", protectedApp)
	mux.Handle("/app/", protectedApp)

	mux.HandleFunc("POST /auth/login", deps.Authentication.Login)
	mux.HandleFunc("POST /auth/logout", deps.Authentication.Logout)
	mux.Handle("POST /voice/tools/customer-lookup", deps.CustomerLookup)
	mux.HandleFunc("POST /voice/tools/appointment-availability", deps.AppointmentTools.Availability)
	mux.HandleFunc("POST /voice/tools/appointment-book", deps.AppointmentTools.Book)
	mux.Handle("POST /voice/tools/follow-up-request", deps.FollowUpTool)
	mux.Handle("POST /webhooks/elevenlabs/post-call", deps.PostCallWebhook)

	// The public site (F07) owns "/" and the legal pages, and the login page (F13)
	// is where the F09 middleware sends a browser without a session. Neither has a
	// dependency, so both are built here rather than in the DI root.
	handlers.NewSite().Register(mux)
	handlers.NewLogin().Register(mux)

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
