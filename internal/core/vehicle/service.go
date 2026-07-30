package vehicle

import (
	"context"
	"strings"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type Store interface {
	CreateVehicle(ctx context.Context, tenantID string, input CreateInput, normalizedPlate string) (Vehicle, error)
	FindVehicleByPlate(ctx context.Context, tenantID, normalizedPlate string) (Vehicle, error)
	ListVehiclesByCustomer(ctx context.Context, tenantID, customerID string) ([]Vehicle, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Vehicle, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Vehicle{}, err
	}
	input.CustomerID = strings.TrimSpace(input.CustomerID)
	if !validUUID(input.CustomerID) {
		return Vehicle{}, &domain.ValidationError{
			Entity: "vehicle",
			Errors: map[string]string{"customer_id": "must be a valid ID"},
		}
	}

	input.Plate = strings.TrimSpace(input.Plate)
	normalizedPlate := ""
	if input.Plate != "" {
		normalizedPlate, err = NormalizePlate(input.Plate)
		if err != nil {
			return Vehicle{}, err
		}
	}
	input.Make = strings.TrimSpace(input.Make)
	input.Model = strings.TrimSpace(input.Model)
	return s.store.CreateVehicle(ctx, tenantID, input, normalizedPlate)
}

func (s *Service) FindByPlate(ctx context.Context, plate string) (Vehicle, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Vehicle{}, err
	}
	normalizedPlate, err := NormalizePlate(plate)
	if err != nil {
		return Vehicle{}, err
	}
	return s.store.FindVehicleByPlate(ctx, tenantID, normalizedPlate)
}

func (s *Service) ListByCustomer(ctx context.Context, customerID string) ([]Vehicle, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	customerID = strings.TrimSpace(customerID)
	if !validUUID(customerID) {
		return nil, &domain.ValidationError{
			Entity: "vehicle",
			Errors: map[string]string{"customer_id": "must be a valid ID"},
		}
	}
	return s.store.ListVehiclesByCustomer(ctx, tenantID, customerID)
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}
