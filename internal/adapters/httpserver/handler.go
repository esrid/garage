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
	Openings     *handlers.OpeningMutations

	// Authenticated by their own tenant-scoped bearer token or signature.
	CustomerLookup   *voice.CustomerLookup
	AppointmentTools *voice.AppointmentTools
	FollowUpTool     *voice.FollowUpTool
	PostCallWebhook  *voice.PostCallWebhook
}

// New builds the router as a list of trust boundaries, because that is what the
// grouping is actually about: who may call what, and what proves it.
//
// Each sub-router below exists for a boundary it enforces or names, not for
// tidiness. A nested mux sharing no behaviour with its group would only be a
// second map to look things up in.
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mountOperations(mux, &handler{readiness: deps.Readiness})
	mountPublic(mux)
	mountAuthentication(mux, deps)
	mountApplication(mux, deps)
	mountVoiceTools(mux, deps)
	mountWebhooks(mux, deps)

	return requestID(recoverPanic(accessLog(securityHeaders(crossOriginProtection(mux)))))
}

// mountOperations exposes the two probes the platform polls. Deliberately
// unauthenticated: a health check that needs a credential is a health check that
// fails for the wrong reason.
func mountOperations(mux *http.ServeMux, h *handler) {
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
}

// mountPublic serves everyone: the marketing site and its legal pages (F07), the
// sign-in page the session middleware sends browsers to (F13), and the embedded
// assets. None of them has a dependency, so they are built here instead of
// travelling through the DI root.
func mountPublic(mux *http.ServeMux) {
	handlers.NewSite().Register(mux)
	handlers.NewLogin().Register(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets.Static())))
}

// mountAuthentication is the boundary crossing itself: the only two routes that
// turn a password into a session and back. Public by necessity, and protected by
// the cross-origin check plus their own derivation budget.
func mountAuthentication(mux *http.ServeMux, deps Deps) {
	mux.HandleFunc("POST /auth/login", deps.Authentication.Login)
	mux.HandleFunc("POST /auth/logout", deps.Authentication.Logout)
}

// mountApplication is the staff area, and the sub-router earns its place: every
// route inside goes through requireStaffSession, which is what puts the staff
// identity and the tenant into the context these handlers read. Registering them
// on the root mux would mean repeating that wrapper per route, and losing it the
// day someone adds a page in a hurry.
func mountApplication(mux *http.ServeMux, deps Deps) {
	appMux := http.NewServeMux()
	appMux.HandleFunc("GET /app", deps.Dashboard.Page)
	appMux.HandleFunc("GET /app/today", deps.Dashboard.Fragment)
	appMux.HandleFunc("GET /app/planning", deps.Planning.Page)
	appMux.HandleFunc("GET /app/planning/day", deps.Planning.Fragment)
	appMux.HandleFunc("POST /app/openings", deps.Openings.Configure)
	appMux.HandleFunc("POST /app/appointments", deps.Appointments.Book)
	appMux.HandleFunc("POST /app/appointments/{id}/reschedule", deps.Appointments.Reschedule)
	appMux.HandleFunc("POST /app/appointments/{id}/cancel", deps.Appointments.Cancel)
	deps.Calls.Register(appMux)

	protected := requireStaffSession(deps.Sessions, appMux)
	// Both patterns: "/app" alone does not match "/app/planning", and "/app/" does
	// not match "/app". Dropping either would leave a page outside the session.
	mux.Handle("/app", protected)
	mux.Handle("/app/", protected)
}

// mountVoiceTools is what the voice agent calls during a conversation. The
// sub-router names one credential: every route here authenticates a bearer token
// issued per tenant, resolves the tenant from it on the server, and never accepts
// a tenant identifier from the payload the model produced (PRD 7.1).
//
// It is also where a concern shared by tool traffic belongs the day one appears —
// a rate limit, an audit trail. Nothing wraps it yet because each handler still
// enforces its own bounds, and moving that would be a behaviour change disguised
// as a refactor.
func mountVoiceTools(mux *http.ServeMux, deps Deps) {
	tools := http.NewServeMux()
	tools.Handle("POST /voice/tools/customer-lookup", deps.CustomerLookup)
	tools.HandleFunc("POST /voice/tools/appointment-availability", deps.AppointmentTools.Availability)
	tools.HandleFunc("POST /voice/tools/appointment-book", deps.AppointmentTools.Book)
	tools.Handle("POST /voice/tools/follow-up-request", deps.FollowUpTool)

	// Full paths stay inside the sub-router, so a pattern reads the same here, in
	// the access log, and in the ElevenLabs tool configuration.
	mux.Handle("/voice/", tools)
}

// mountWebhooks is what the provider calls after a conversation. Separate from
// the tools on purpose: this boundary is proved by an HMAC signature over the raw
// body, not by a bearer token, and it is the only place where a replay window and
// a payload hash decide whether a delivery is accepted.
func mountWebhooks(mux *http.ServeMux, deps Deps) {
	webhooks := http.NewServeMux()
	webhooks.Handle("POST /webhooks/elevenlabs/post-call", deps.PostCallWebhook)

	mux.Handle("/webhooks/", webhooks)
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
