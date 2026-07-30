package customerfiles

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
)

type readerStub struct {
	matches []customer.Match
	file    customer.File
	err     error
	queries []string
	ids     []string
}

func (s *readerStub) Search(_ context.Context, query string) ([]customer.Match, error) {
	s.queries = append(s.queries, query)
	return s.matches, s.err
}

func (s *readerStub) File(_ context.Context, id string) (customer.File, error) {
	s.ids = append(s.ids, id)
	return s.file, s.err
}

func serve(t *testing.T, reader customer.FileReader, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(reader).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func TestSearchListsMatchesAndKeepsTheQuery(t *testing.T) {
	stub := &readerStub{matches: []customer.Match{
		{Customer: customer.Customer{ID: "c1", FirstName: "Marie", LastName: "Lubin", Phone: "+596696000001"}, Plates: []string{"AB-123-CD"}},
		{Customer: customer.Customer{ID: "c2", Phone: "+596696000002"}},
	}}

	body := serve(t, stub, "/app/customers?q=lubin").Body.String()
	for _, want := range []string{"Marie Lubin", "AB-123-CD", `value="lubin"`, `href="/app/customers/c1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("search page is missing %q", want)
		}
	}
	// A customer with no name is titled by their number, never invented.
	if !strings.Contains(body, "+596696000002") {
		t.Error("a nameless customer lost their number")
	}
	if !strings.Contains(body, "Aucun véhicule enregistré") {
		t.Error("a customer without a vehicle should say so")
	}
	if len(stub.queries) != 1 || stub.queries[0] != "lubin" {
		t.Errorf("reader received %v", stub.queries)
	}
}

func TestFileShowsVehiclesAndVisits(t *testing.T) {
	start := time.Date(2026, 7, 30, 9, 0, 0, 0, time.FixedZone("AST", -4*60*60))
	stub := &readerStub{file: customer.File{
		Customer: customer.Customer{ID: "c1", FirstName: "Marie", LastName: "Lubin", Phone: "+596696000001", CreatedAt: start},
		Vehicles: []customer.Vehicle{{ID: "v1", Plate: "AB-123-CD", Make: "Renault", Model: "Clio IV"}},
		Visits:   []customer.Visit{{ID: "a1", Start: start, ServiceLabel: "Vidange", Status: "done", Plate: "AB-123-CD"}},
	}}

	body := serve(t, stub, "/app/customers/c1").Body.String()
	for _, want := range []string{"Marie Lubin", "Renault Clio IV", "AB-123-CD", "Vidange", "Terminé", "30/07", "09:00"} {
		if !strings.Contains(body, want) {
			t.Errorf("file page is missing %q", want)
		}
	}
	if len(stub.ids) != 1 || stub.ids[0] != "c1" {
		t.Errorf("reader received %v", stub.ids)
	}
}

// An unknown id and another workshop's customer must be indistinguishable, and an
// outage must not be reported as "this file does not exist".
func TestFileSeparatesMissingFromUnavailable(t *testing.T) {
	missing := serve(t, &readerStub{err: &domain.NotFoundError{Entity: "customer"}}, "/app/customers/nope")
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "introuvable") {
		t.Errorf("missing file: status=%d", missing.Code)
	}

	broken := serve(t, &readerStub{err: errors.New("database is down")}, "/app/customers/c1")
	body := broken.Body.String()
	if strings.Contains(body, "introuvable") {
		t.Error("an outage was reported as a missing file")
	}
	if !strings.Contains(body, "indisponible") || strings.Contains(body, "database is down") {
		t.Errorf("degraded file page is wrong: %.120s", body)
	}
}

func TestSearchDegradesWithoutLeakingTheError(t *testing.T) {
	response := serve(t, &readerStub{err: errors.New("database is down")}, "/app/customers")
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(body, "momentanément indisponible") || strings.Contains(body, "database is down") {
		t.Errorf("degraded search is wrong: %.120s", body)
	}

	unauthorized := serve(t, &readerStub{err: &domain.UnauthorizedError{}}, "/app/customers")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", unauthorized.Code)
	}
}
