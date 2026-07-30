package customer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

// maxSearchResults bounds one search. A desk looking for a customer types enough
// to narrow it down; a list of five hundred is not an answer.
const maxSearchResults = 50

// Match is a customer as a search result, with what makes them recognisable at
// the desk: their vehicles.
type Match struct {
	Customer
	Plates []string
}

// Vehicle of a customer, as the file page shows it. Declared here rather than
// imported from the vehicle domain so this read has one shape, not two.
type Vehicle struct {
	ID    string
	Plate string
	Make  string
	Model string
}

// Visit is one appointment on a customer's file.
type Visit struct {
	ID           string
	Start        time.Time
	ServiceLabel string
	Status       string
	Plate        string
}

// File is everything the desk needs about one customer.
type File struct {
	Customer Customer
	Vehicles []Vehicle
	Visits   []Visit
	Timezone string
}

// ReadStore is the persistence capability the customer files need.
type ReadStore interface {
	SearchCustomers(ctx context.Context, tenantID, query string, limit int) ([]Match, error)
	CustomerFile(ctx context.Context, tenantID, customerID string) (File, error)
}

// FileReader is what the HTTP adapter consumes.
type FileReader interface {
	Search(ctx context.Context, query string) ([]Match, error)
	File(ctx context.Context, customerID string) (File, error)
}

type ReadService struct {
	store ReadStore
}

func NewReadService(store ReadStore) *ReadService {
	return &ReadService{store: store}
}

// Search matches on a name, a phone number or a plate, because those are the
// three things a caller gives. An empty query lists the most recent customers:
// the desk opening the page usually wants whoever just called.
func (s *ReadService) Search(ctx context.Context, query string) ([]Match, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	matches, err := s.store.SearchCustomers(ctx, tenantID, strings.TrimSpace(query), maxSearchResults)
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		if match.TenantID != tenantID {
			// A store answering with another workshop's customer is a bug we refuse
			// to render, not one we pass to a page.
			return nil, fmt.Errorf("customer search returned a foreign customer")
		}
	}
	return matches, nil
}

func (s *ReadService) File(ctx context.Context, customerID string) (File, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return File{}, err
	}
	if strings.TrimSpace(customerID) == "" {
		return File{}, &domain.NotFoundError{Entity: "customer"}
	}
	file, err := s.store.CustomerFile(ctx, tenantID, customerID)
	if err != nil {
		return File{}, err
	}
	if file.Customer.TenantID != tenantID {
		return File{}, fmt.Errorf("customer file returned a foreign customer")
	}
	return file, nil
}
