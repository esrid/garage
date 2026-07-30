package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/web/views"
)

// CallHistoryReader is the read model the call history needs, frozen in
// docs/contracts/F15-call-history.md. Agent A writes the adapter over the F14
// conversations table (MT-09).
//
// No tenant ID in the signatures: it travels in ctx, put there by the F09 session
// middleware, so no frontend caller can pass one (PRD 7.1).
type CallHistoryReader interface {
	Calls(ctx context.Context, day time.Time) (views.CallHistory, error)
	Call(ctx context.Context, id string) (views.CallDetail, error)
}

// Calls renders the call history: one day as a list, one call in full.
type Calls struct {
	reader CallHistoryReader
	// now is injected so a rendered day is a function of input, not of the wall
	// clock, which keeps these handlers testable.
	now func() time.Time
}

func NewCalls(reader CallHistoryReader) *Calls {
	return &Calls{reader: reader, now: time.Now}
}

func (h *Calls) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/calls", h.Day)
	mux.HandleFunc("GET /app/calls/{id}", h.One)
}

// Day serves GET /app/calls?day=YYYY-MM-DD.
func (h *Calls) Day(w http.ResponseWriter, r *http.Request) {
	history, notices := h.day(r)
	history.Notices = append(history.Notices, notices...)
	renderPage(w, r, http.StatusOK, views.CallsPage(history))
}

// One serves GET /app/calls/{id}.
func (h *Calls) One(w http.ResponseWriter, r *http.Request) {
	call, err := h.reader.Call(r.Context(), r.PathValue("id"))
	if err != nil {
		var notFound *domain.NotFoundError
		if errors.As(err, &notFound) {
			// One page for an unknown id and for another tenant's call: telling the two
			// apart is how an id becomes an existence oracle.
			renderPage(w, r, http.StatusNotFound, views.CallProblemPage(
				"Appel introuvable",
				"Cet appel n'existe pas, ou n'appartient pas à cet atelier.",
			))
			return
		}
		// Not "introuvable": we could not look. Saying the call does not exist would
		// be inventing a fact out of an outage.
		slog.ErrorContext(r.Context(), "calls: call unavailable", "err", err)
		renderPage(w, r, http.StatusOK, views.CallProblemPage(
			"Appel indisponible",
			"Cet appel n'a pas pu être lu. Réessayez dans un instant.",
		))
		return
	}
	renderPage(w, r, http.StatusOK, views.CallPage(call))
}

// day resolves the requested civil date and reads it.
//
// The timezone comes from the tenant, in the database, so the reader is asked for
// the current day first and the parameter is parsed inside the location it
// reports. Parsing "2026-07-30" as UTC and using it directly would ask for the
// previous day in Martinique — the same trap as the planning page.
func (h *Calls) day(r *http.Request) (views.CallHistory, []string) {
	ctx := r.Context()
	history, err := h.reader.Calls(ctx, h.now())
	if err != nil {
		return h.unavailable(ctx, err)
	}
	requested, dayErr := requestedDay(r.URL.Query().Get("day"), history.Day.Location())
	switch {
	case errors.Is(dayErr, errNoDayParameter):
		return history, nil
	case dayErr != nil:
		return history, []string{dayUnreadableNotice}
	}
	requestedHistory, err := h.reader.Calls(ctx, requested)
	if err != nil {
		return h.unavailable(ctx, err)
	}
	return requestedHistory, nil
}

// unavailable degrades the page instead of blanking it: an empty list would read
// as "no calls today", which is a different fact from "we could not look".
func (h *Calls) unavailable(ctx context.Context, err error) (views.CallHistory, []string) {
	slog.ErrorContext(ctx, "calls: history unavailable", "err", err)
	return views.CallHistory{
			Day:      h.now(),
			Degraded: true,
		}, []string{
			"L'historique des appels est momentanément indisponible. Réessayez dans un instant.",
		}
}
