package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/web/views"
)

// callsStub answers like the adapter Agent A will write (MT-09): the day resolved
// to midnight in the tenant timezone, calls already in that location.
type callsStub struct {
	history  views.CallHistory
	call     views.CallDetail
	listErr  error
	callErr  error
	dayCalls []time.Time
	callIDs  []string
}

func (s *callsStub) Calls(_ context.Context, day time.Time) (views.CallHistory, error) {
	s.dayCalls = append(s.dayCalls, day)
	if s.listErr != nil {
		return views.CallHistory{}, s.listErr
	}
	local := day.In(martinique)
	history := s.history
	history.Day = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, martinique)
	history.Timezone = "America/Martinique"
	return history, nil
}

func (s *callsStub) Call(_ context.Context, id string) (views.CallDetail, error) {
	s.callIDs = append(s.callIDs, id)
	if s.callErr != nil {
		return views.CallDetail{}, s.callErr
	}
	return s.call, nil
}

func fullCallsStub() *callsStub {
	summary := views.CallSummary{
		ID: "conv-1", At: at(8, 12), Duration: 4*time.Minute + 20*time.Second,
		CustomerName: "Marie Lubin", Phone: "0596000001",
		Outcome: "booked", Status: "done",
		Summary: "Vidange prise pour jeudi 9h, véhicule confirmé par la plaque.",
	}
	return &callsStub{
		history: views.CallHistory{Calls: []views.CallSummary{
			summary,
			{
				ID: "conv-2", At: at(9, 3), Duration: 2 * time.Minute,
				Phone: "0696000002", Outcome: "quote", Status: "done",
			},
		}},
		call: views.CallDetail{
			CallSummary: summary,
			Turns: []views.CallTurn{
				{Role: "agent", Text: "Garage Atelier IA, bonjour.", At: 0},
				{Role: "user", Text: "Bonjour, je voudrais une vidange.", At: 4 * time.Second},
				{Role: "agent", Text: "Je regarde le planning.", At: 9 * time.Second},
				{Role: "supervisor", Text: "Note interne.", At: 12 * time.Second},
			},
		},
	}
}

func newTestCalls(reader CallHistoryReader) *Calls {
	h := NewCalls(reader)
	h.now = func() time.Time { return planningNow }
	return h
}

func callsMux(reader CallHistoryReader) *http.ServeMux {
	mux := http.NewServeMux()
	newTestCalls(reader).Register(mux)
	return mux
}

func getCalls(t *testing.T, reader CallHistoryReader, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	callsMux(reader).ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func TestCallsPageListsTheDay(t *testing.T) {
	response := getCalls(t, fullCallsStub(), "/app/calls")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()

	for _, want := range []string{
		"jeudi 30 juillet 2026",
		"America/Martinique",
		"08:12",
		"Marie Lubin",
		"0696000002", // unknown caller falls back to the number, like F04
		"4 min 20 s",
		"RDV pris", // outcome label
		`href="/app/calls/conv-1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("call list is missing %q", want)
		}
	}
	// 08:12 in Martinique is 12:12 UTC: the list must not show provider UTC.
	if strings.Contains(body, "12:12") {
		t.Error("the list renders UTC hours")
	}
}

// The same trap as the planning page: a civil date parsed as UTC lands on the
// previous day in Martinique.
func TestCallsDayParameterIsReadInTheTenantTimezone(t *testing.T) {
	stub := fullCallsStub()
	getCalls(t, stub, "/app/calls?day=2026-07-31")

	if len(stub.dayCalls) != 2 {
		t.Fatalf("Calls called %d times, want 2 (current day, then the requested one)", len(stub.dayCalls))
	}
	if got := stub.dayCalls[1].In(martinique).Format(time.DateOnly); got != "2026-07-31" {
		t.Errorf("asked for %s, want 2026-07-31", got)
	}
}

func TestCallsKeepsRenderingOnAnUnreadableDate(t *testing.T) {
	response := getCalls(t, fullCallsStub(), "/app/calls?day=hier")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Date illisible") {
		t.Error("the page does not say the date was unreadable")
	}
	if !strings.Contains(body, "Marie Lubin") {
		t.Error("the page should still show the current day")
	}
}

// An empty list and a failed read are different facts. Showing "0 appels" for an
// outage tells the garage nobody called.
func TestCallsSaysWhenItCouldNotLook(t *testing.T) {
	stub := fullCallsStub()
	stub.listErr = errors.New("database is down")

	body := getCalls(t, stub, "/app/calls").Body.String()
	if !strings.Contains(body, "momentanément indisponible") {
		t.Error("the page does not explain the failure")
	}
	if strings.Contains(body, "Aucun appel") || strings.Contains(body, "panel-call-history") {
		t.Error("a failed read must not render as an empty day")
	}
	if strings.Contains(body, "database is down") {
		t.Error("the page leaks the backend error")
	}
}

func TestCallDetailShowsSummaryAndTranscript(t *testing.T) {
	response := getCalls(t, fullCallsStub(), "/app/calls/conv-1")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()

	for _, want := range []string{
		"Marie Lubin",
		"Vidange prise pour jeudi 9h",
		"non vérifié", // the summary is provider information, and says so
		"Assistant",
		"Client",
		"supervisor", // an unknown role keeps its raw value
		"Garage Atelier IA, bonjour.",
		"00:04", // turn offset
		"0596000001",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("call detail is missing %q", want)
		}
	}
	// A zero offset is not rendered: "00:00" on the first line is noise.
	if strings.Contains(body, "00:00") {
		t.Error("a zero offset was rendered")
	}
}

func TestCallDetailWithoutSummaryOrTranscript(t *testing.T) {
	stub := fullCallsStub()
	stub.call = views.CallDetail{CallSummary: views.CallSummary{ID: "conv-3", At: at(10, 0)}}

	body := getCalls(t, stub, "/app/calls/conv-3").Body.String()
	for _, want := range []string{"Aucun résumé", "Aucune transcription"} {
		if !strings.Contains(body, want) {
			t.Errorf("the empty state is missing %q", want)
		}
	}
	// Nothing invented for a caller we do not know.
	if strings.Contains(body, "inconnu") && !strings.Contains(body, "Numéro inconnu") {
		t.Error("unexpected wording for a missing caller")
	}
}

// An unknown id and another tenant's id must be indistinguishable, and an outage
// must not be reported as "this call does not exist".
func TestCallDetailSeparatesMissingFromUnavailable(t *testing.T) {
	missing := fullCallsStub()
	missing.callErr = &domain.NotFoundError{Entity: "conversation"}
	response := getCalls(t, missing, "/app/calls/nope")
	if response.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown call", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Appel introuvable") {
		t.Error("the page does not say the call was not found")
	}

	broken := fullCallsStub()
	broken.callErr = errors.New("database is down")
	response = getCalls(t, broken, "/app/calls/conv-1")
	body := response.Body.String()
	if strings.Contains(body, "introuvable") {
		t.Error("an outage was reported as a missing call")
	}
	if !strings.Contains(body, "Appel indisponible") {
		t.Error("the page does not say the call could not be read")
	}
	if strings.Contains(body, "database is down") {
		t.Error("the page leaks the backend error")
	}
}

func TestCallDetailPassesTheIdThrough(t *testing.T) {
	stub := fullCallsStub()
	getCalls(t, stub, "/app/calls/conv-42")

	if len(stub.callIDs) != 1 || stub.callIDs[0] != "conv-42" {
		t.Errorf("reader received %v, want [conv-42]", stub.callIDs)
	}
}
