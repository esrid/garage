package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/esrid/garage/assets"
	"github.com/esrid/garage/internal/features/calls"
	"github.com/esrid/garage/internal/features/dashboard"
	"github.com/esrid/garage/internal/features/identity"
	"github.com/esrid/garage/internal/features/planning"
	"github.com/esrid/garage/internal/features/postcall"
	"github.com/esrid/garage/internal/features/site"
	"github.com/esrid/garage/internal/features/voicetools"
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
	Authentication *identity.Authentication

	// Behind the staff session.
	Dashboard    *dashboard.Dashboard
	Calls        *calls.Calls
	Planning     *planning.Handler
	Appointments *planning.AppointmentMutations
	Openings     *planning.OpeningMutations

	// Authenticated by their own tenant-scoped bearer token or signature.
	CustomerLookup   *voicetools.CustomerLookup
	CustomerRecord   *voicetools.CustomerRecord
	AppointmentTools *voicetools.AppointmentTools
	FollowUpTool     *voicetools.FollowUpTool
	PostCallWebhook  *postcall.PostCallWebhook
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
	site.NewSite().Register(mux)
	identity.NewLogin().Register(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets.Static())))
}

// mountApplication is the staff area, and the sub-router earns its place: every
// route inside goes through requireStaffSession, which is what puts the staff
// identity and the tenant into the context these handlers read. Registering them
// on the root mux would mean repeating that wrapper per route, and losing it the
// day someone adds a page in a hurry.
//
// Each feature registers its own patterns; this function only says which ones are
// behind the session.
func mountApplication(mux *http.ServeMux, deps Deps) {
	appMux := http.NewServeMux()
	deps.Dashboard.Register(appMux)
	deps.Planning.Register(appMux)
	deps.Calls.Register(appMux)
	deps.Appointments.Register(appMux)
	deps.Openings.Register(appMux)

	protected := requireStaffSession(deps.Sessions, appMux)
	// Both patterns: mounting only "/app/" makes the mux redirect "/app" to it,
	// which the documentation describes and which would turn the dashboard into a
	// round trip. Registering both is the documented way to override that.
	mux.Handle("/app", protected)
	mux.Handle("/app/", protected)
}

// mountVoiceTools is what the voice agent calls during a conversation. Every
// route here authenticates a bearer token issued per tenant, resolves the tenant
// from it on the server, and never accepts a tenant identifier from the payload
// the model produced (PRD 7.1).
//
// The sub-router is also where a concern shared by tool traffic belongs the day
// one appears — a rate limit, an audit trail. Nothing wraps it yet because each
// handler still enforces its own bounds.
func mountVoiceTools(mux *http.ServeMux, deps Deps) {
	tools := http.NewServeMux()
	deps.CustomerLookup.Register(tools)
	deps.CustomerRecord.Register(tools)
	deps.AppointmentTools.Register(tools)
	deps.FollowUpTool.Register(tools)

	// Full paths stay inside the sub-router, which the mux documentation
	// guarantees: mounting a handler on a subtree does not strip the prefix. A
	// pattern therefore reads the same here, in the access log, and in the
	// ElevenLabs tool configuration.
	mux.Handle("/voice/", tools)
}

// mountWebhooks is what the provider calls after a conversation. Separate from
// the tools on purpose: this boundary is proved by an HMAC signature over the raw
// body, not by a bearer token.
func mountWebhooks(mux *http.ServeMux, deps Deps) {
	webhooks := http.NewServeMux()
	deps.PostCallWebhook.Register(webhooks)

	mux.Handle("/webhooks/", webhooks)
}

// mountAuthentication is the boundary crossing itself: the only two routes that
// turn a password into a session and back.
func mountAuthentication(mux *http.ServeMux, deps Deps) {
	deps.Authentication.Register(mux)
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
