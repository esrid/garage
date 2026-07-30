package voicetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/adapters/stores/postgres"
	"github.com/esrid/garage/internal/adapters/voice"
	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/esrid/garage/internal/core/vehicle"
)

// This tool writes, so it is exercised against a real PostgreSQL: the rules that
// matter - a known phone is not renamed, a plate is not moved between owners -
// live in the unique keys, and a double would only assert what the double does.
func recordTool(t *testing.T) (*CustomerRecord, string, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workshop, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Garage record"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	// Each test gets its own workshop, so nothing has to be deleted afterwards:
	// the rows are scoped to a tenant no other test uses.

	token := "tokenRECORDAAAAAAAAAAAAAAAAAAAAAAAA"
	authenticator, err := voice.NewTokenAuthenticator(workshop.ID + ":" + token)
	if err != nil {
		t.Fatalf("NewTokenAuthenticator() error = %v", err)
	}
	return NewCustomerRecord(customer.NewService(store), vehicle.NewService(store), authenticator), token, workshop.ID
}

func record(t *testing.T, tool *CustomerRecord, token, body string) (int, recordResponse) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/voice/tools/customer-record", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	tool.ServeHTTP(response, request)

	var decoded recordResponse
	_ = json.Unmarshal(response.Body.Bytes(), &decoded)
	return response.Code, decoded
}

func TestCustomerRecordCreatesThenReturnsTheSameCaller(t *testing.T) {
	tool, token, _ := recordTool(t)

	status, first := record(t, tool, token, `{"phone":"+596696500001","first_name":"Marie","last_name":"Lubin","plate":"AB-123-CD","make":"Renault","model":"Clio"}`)
	if status != http.StatusOK || !first.Created || first.CustomerID == "" {
		t.Fatalf("first call: status=%d body=%+v", status, first)
	}
	if !first.VehicleCreated || first.VehicleID == "" {
		t.Fatalf("the plate was not recorded: %+v", first)
	}

	// The same call again - a dropped line, a retry - must not duplicate anyone.
	status, second := record(t, tool, token, `{"phone":"+596696500001","first_name":"Marie","last_name":"Lubin","plate":"AB-123-CD"}`)
	if status != http.StatusOK {
		t.Fatalf("second call status = %d", status)
	}
	if second.Created || second.CustomerID != first.CustomerID {
		t.Errorf("the caller was created twice: %+v then %+v", first, second)
	}
	if second.VehicleCreated || second.VehicleID != first.VehicleID {
		t.Errorf("the vehicle was created twice: %+v then %+v", first, second)
	}
}

// A model that mishears a name must not rename a customer the garage validated.
func TestCustomerRecordNeverRenamesAKnownCaller(t *testing.T) {
	tool, token, workshopID := recordTool(t)

	_, created := record(t, tool, token, `{"phone":"+596696500002","first_name":"Ana","last_name":"Bertrand"}`)
	status, again := record(t, tool, token, `{"phone":"+596696500002","first_name":"Anna","last_name":"Berthrand"}`)
	if status != http.StatusOK || again.Created {
		t.Fatalf("status=%d body=%+v", status, again)
	}

	stored, err := tool.customers.FindByPhone(
		tenant.WithID(context.Background(), workshopID), "+596696500002")
	if err != nil {
		t.Fatalf("FindByPhone() error = %v", err)
	}
	if stored.FirstName != "Ana" || stored.LastName != "Bertrand" {
		t.Errorf("the stored name changed to %q %q", stored.FirstName, stored.LastName)
	}
	if stored.ID != created.CustomerID {
		t.Errorf("a second customer was created for the same number")
	}
}

// Moving a vehicle between owners is a desk decision, not something a phone call
// does silently.
func TestCustomerRecordRefusesAPlateHeldByAnotherCustomer(t *testing.T) {
	tool, token, _ := recordTool(t)

	if status, _ := record(t, tool, token, `{"phone":"+596696500003","first_name":"Marie","plate":"XY-987-ZZ"}`); status != http.StatusOK {
		t.Fatalf("first owner status = %d", status)
	}
	status, body := record(t, tool, token, `{"phone":"+596696500004","first_name":"Jean","plate":"XY-987-ZZ"}`)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if body.VehicleID != "" {
		t.Errorf("the conflict response carries a vehicle: %+v", body)
	}
}

func TestCustomerRecordRefusesUnusableInput(t *testing.T) {
	tool, token, _ := recordTool(t)

	for name, body := range map[string]string{
		"no phone":      `{"first_name":"Marie"}`,
		"unparsable":    `{"phone":"allo","first_name":"Marie"}`,
		"unknown field": `{"phone":"+596696500005","first_name":"Marie","tenant_id":"019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"}`,
		"bad plate":     `{"phone":"+596696500005","first_name":"Marie","plate":"?"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if status, _ := record(t, tool, token, body); status != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422", status)
			}
		})
	}
}

// A caller who will not give a name is still bookable: the number is the
// identity, and every screen already titles such a row by its phone.
func TestCustomerRecordAcceptsACallerWithoutAName(t *testing.T) {
	tool, token, _ := recordTool(t)

	status, body := record(t, tool, token, `{"phone":"+596696500007"}`)
	if status != http.StatusOK || !body.Created || body.CustomerID == "" {
		t.Fatalf("status=%d body=%+v", status, body)
	}
}

func TestCustomerRecordRequiresItsOwnToken(t *testing.T) {
	tool, _, _ := recordTool(t)

	status, _ := record(t, tool, "wrong-token-aaaaaaaaaaaaaaaaaaaaaaaa", `{"phone":"+596696500006","first_name":"Marie"}`)
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}
