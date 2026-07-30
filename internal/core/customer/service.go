package customer

import (
	"context"
	"strings"

	"github.com/esrid/garage/internal/core/tenant"
)

type Store interface {
	CreateCustomer(ctx context.Context, tenantID string, input CreateInput) (Customer, error)
	FindCustomerByPhone(ctx context.Context, tenantID, normalizedPhone string) (Customer, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Customer, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Customer{}, err
	}
	phone, err := NormalizePhone(input.Phone)
	if err != nil {
		return Customer{}, err
	}
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Phone = phone
	return s.store.CreateCustomer(ctx, tenantID, input)
}

func (s *Service) FindByPhone(ctx context.Context, phone string) (Customer, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Customer{}, err
	}
	normalizedPhone, err := NormalizePhone(phone)
	if err != nil {
		return Customer{}, err
	}
	return s.store.FindCustomerByPhone(ctx, tenantID, normalizedPhone)
}
