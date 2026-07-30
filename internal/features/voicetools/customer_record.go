package voicetools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esrid/garage/internal/adapters/voice"
	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/vehicle"
)

const maxRecordBodyBytes = 8 << 10

// CustomerRecord writes what the assistant learned during a call: who is calling,
// and which vehicle they are calling about.
//
// It is the missing half of the voice flow. Lookup reads, booking needs a
// customer_id, and nothing created one - so an unknown caller could never be
// given an appointment, and the plate the PRD lists as P0 had no way into the
// database. Frozen in docs/contracts/F19-voice-customer-create.md.
type CustomerRecord struct {
	customers     *customer.Service
	vehicles      *vehicle.Service
	authenticator *voice.TokenAuthenticator
}

func NewCustomerRecord(customers *customer.Service, vehicles *vehicle.Service, authenticator *voice.TokenAuthenticator) *CustomerRecord {
	return &CustomerRecord{customers: customers, vehicles: vehicles, authenticator: authenticator}
}

func (h *CustomerRecord) Register(mux *http.ServeMux) {
	mux.Handle("POST /voice/tools/customer-record", h)
}

type recordRequest struct {
	Phone     string `json:"phone"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Plate     string `json:"plate"`
	Make      string `json:"make"`
	Model     string `json:"model"`
}

type recordResponse struct {
	CustomerID     string `json:"customer_id"`
	Created        bool   `json:"created"`
	VehicleID      string `json:"vehicle_id,omitempty"`
	VehicleCreated bool   `json:"vehicle_created,omitempty"`
}

func (h *CustomerRecord) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input recordRequest
	ctx, tenantID, err := voice.DecodeToolRequest(w, r, h.authenticator, maxRecordBodyBytes, &input)
	if err != nil {
		if errors.Is(err, voice.ErrToolUnauthorized) {
			writeRecordJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		writeRecordJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
		return
	}

	// An existing caller is returned as they are. A model that mishears a name
	// must never rename a customer the garage already validated.
	response := recordResponse{}
	existing, err := h.customers.FindByPhone(ctx, input.Phone)
	switch {
	case err == nil:
		response.CustomerID, response.Created = existing.ID, false
	case isNotFound(err):
		created, createErr := h.customers.Create(ctx, customer.CreateInput{
			FirstName: input.FirstName,
			LastName:  input.LastName,
			Phone:     input.Phone,
		})
		if createErr != nil {
			writeRecordError(w, createErr)
			return
		}
		if created.TenantID != tenantID {
			writeRecordJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service unavailable"})
			return
		}
		response.CustomerID, response.Created = created.ID, true
	default:
		writeRecordError(w, err)
		return
	}

	if input.Plate != "" {
		vehicleID, vehicleCreated, vehicleErr := h.recordVehicle(ctx, response.CustomerID, input)
		if vehicleErr != nil {
			writeRecordError(w, vehicleErr)
			return
		}
		response.VehicleID, response.VehicleCreated = vehicleID, vehicleCreated
	}
	writeRecordJSON(w, http.StatusOK, response)
}

// recordVehicle attaches the plate to the caller. A plate already held by another
// customer of the workshop is a conflict rather than a transfer: moving a vehicle
// between owners is a decision for the desk, not for a phone call.
func (h *CustomerRecord) recordVehicle(ctx context.Context, customerID string, input recordRequest) (string, bool, error) {
	existing, err := h.vehicles.FindByPlate(ctx, input.Plate)
	switch {
	case err == nil && existing.CustomerID == customerID:
		return existing.ID, false, nil
	case err == nil:
		return "", false, &domain.AlreadyExistsError{Entity: "vehicle", Field: "plate", Value: input.Plate}
	case !isNotFound(err):
		return "", false, err
	}

	created, err := h.vehicles.Create(ctx, vehicle.CreateInput{
		CustomerID: customerID,
		Plate:      input.Plate,
		Make:       input.Make,
		Model:      input.Model,
	})
	if err != nil {
		return "", false, err
	}
	return created.ID, true, nil
}

func isNotFound(err error) bool {
	var notFound *domain.NotFoundError
	return errors.As(err, &notFound)
}

func writeRecordError(w http.ResponseWriter, err error) {
	var validation *domain.ValidationError
	var alreadyExists *domain.AlreadyExistsError
	switch {
	case errors.As(err, &validation):
		writeRecordJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
	case errors.As(err, &alreadyExists):
		writeRecordJSON(w, http.StatusConflict, map[string]string{"error": "vehicle conflict"})
	default:
		writeRecordJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service unavailable"})
	}
}

func writeRecordJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
