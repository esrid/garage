package appointment

import (
	"context"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type Service struct {
	provider   SchedulingProvider
	reader     DayReader
	configurer OpeningConfigurer
}

func NewService(provider SchedulingProvider, reader DayReader, configurer OpeningConfigurer) *Service {
	return &Service{provider: provider, reader: reader, configurer: configurer}
}

func (s *Service) AvailableSlots(ctx context.Context, query AvailabilityQuery) ([]Slot, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return nil, err
	}
	if err := validateDuration(query.Duration); err != nil {
		return nil, err
	}
	if query.Day.IsZero() {
		return nil, validationError("day", "is required")
	}
	return s.provider.AvailableSlots(ctx, query)
}

func (s *Service) Book(ctx context.Context, input BookInput) (Appointment, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return Appointment{}, err
	}
	input.CustomerID = strings.TrimSpace(input.CustomerID)
	input.VehicleID = strings.TrimSpace(input.VehicleID)
	input.ServiceLabel = strings.TrimSpace(input.ServiceLabel)
	input.Note = strings.TrimSpace(input.Note)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !validUUID(input.CustomerID) {
		return Appointment{}, validationError("customer_id", "must be a UUID")
	}
	if input.VehicleID != "" && !validUUID(input.VehicleID) {
		return Appointment{}, validationError("vehicle_id", "must be a UUID")
	}
	if err := validateWrite(input.CustomerID, input.ServiceLabel, input.Start, input.Duration, input.Note, input.IdempotencyKey); err != nil {
		return Appointment{}, err
	}
	return s.provider.Book(ctx, input)
}

func (s *Service) Reschedule(ctx context.Context, input RescheduleInput) (Appointment, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return Appointment{}, err
	}
	input.AppointmentID = strings.TrimSpace(input.AppointmentID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.AppointmentID == "" {
		return Appointment{}, validationError("appointment_id", "is required")
	}
	if !validUUID(input.AppointmentID) {
		return Appointment{}, validationError("appointment_id", "must be a UUID")
	}
	if input.Start.IsZero() {
		return Appointment{}, validationError("start_at", "is required")
	}
	if err := validateDuration(input.Duration); err != nil {
		return Appointment{}, err
	}
	if err := validateKey(input.IdempotencyKey); err != nil {
		return Appointment{}, err
	}
	return s.provider.Reschedule(ctx, input)
}

func (s *Service) Cancel(ctx context.Context, input CancelInput) (Appointment, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return Appointment{}, err
	}
	input.AppointmentID = strings.TrimSpace(input.AppointmentID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.AppointmentID == "" {
		return Appointment{}, validationError("appointment_id", "is required")
	}
	if !validUUID(input.AppointmentID) {
		return Appointment{}, validationError("appointment_id", "must be a UUID")
	}
	if err := validateKey(input.IdempotencyKey); err != nil {
		return Appointment{}, err
	}
	return s.provider.Cancel(ctx, input)
}

func (s *Service) Day(ctx context.Context, day time.Time) (Day, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return Day{}, err
	}
	if day.IsZero() {
		return Day{}, validationError("day", "is required")
	}
	return s.reader.Day(ctx, day)
}

func (s *Service) ConfigureOpening(ctx context.Context, input ConfigureOpeningInput) (Opening, error) {
	if _, err := tenant.IDFromContext(ctx); err != nil {
		return Opening{}, err
	}
	if input.Start.IsZero() || input.End.IsZero() || !input.End.After(input.Start) {
		return Opening{}, validationError("opening", "end must be after start")
	}
	if input.Capacity < 1 || input.Capacity > 50 {
		return Opening{}, validationError("capacity", "must be between 1 and 50")
	}
	return s.configurer.ConfigureOpening(ctx, input)
}

func validateWrite(customerID, serviceLabel string, start time.Time, duration time.Duration, note, key string) error {
	if customerID == "" {
		return validationError("customer_id", "is required")
	}
	if serviceLabel == "" || !utf8.ValidString(serviceLabel) || utf8.RuneCountInString(serviceLabel) > 200 {
		return validationError("service_label", "must contain 1 to 200 characters")
	}
	if start.IsZero() {
		return validationError("start_at", "is required")
	}
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > 2000 {
		return validationError("note", "must not exceed 2000 characters")
	}
	if err := validateDuration(duration); err != nil {
		return err
	}
	return validateKey(key)
}

func validateDuration(duration time.Duration) error {
	if duration < 15*time.Minute || duration > 8*time.Hour || duration%(15*time.Minute) != 0 {
		return validationError("duration_minutes", "must be 15 to 480 in 15-minute increments")
	}
	return nil
}

func validateKey(key string) error {
	if key == "" || !utf8.ValidString(key) || utf8.RuneCountInString(key) > 200 {
		return validationError("idempotency_key", "must contain 1 to 200 characters")
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}

func validationError(field, message string) error {
	return &domain.ValidationError{Entity: "appointment", Errors: map[string]string{field: message}}
}
