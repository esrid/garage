package appointment

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusConfirmed  Status = "confirmed"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusCancelled  Status = "cancelled"
	StatusNoShow     Status = "no_show"
)

var (
	ErrSlotUnavailable     = errors.New("appointment slot unavailable")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
	ErrInvalidTransition   = errors.New("invalid appointment status transition")
)

type Appointment struct {
	ID           string
	TenantID     string
	CustomerID   string
	VehicleID    string
	OpeningID    string
	ServiceLabel string
	Note         string
	Start        time.Time
	End          time.Time
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DayEntry struct {
	Appointment
	CustomerName string
	VehicleLabel string
	Plate        string
}

type Opening struct {
	ID       string
	Start    time.Time
	End      time.Time
	Capacity int
}

type Slot struct {
	Start time.Time
	End   time.Time
}

type Day struct {
	Date         time.Time
	Timezone     string
	Openings     []Opening
	Appointments []DayEntry
}

type AvailabilityQuery struct {
	Day      time.Time
	Duration time.Duration
}

type BookInput struct {
	CustomerID     string
	VehicleID      string
	ServiceLabel   string
	Start          time.Time
	Duration       time.Duration
	Note           string
	IdempotencyKey string
}

type RescheduleInput struct {
	AppointmentID  string
	Start          time.Time
	Duration       time.Duration
	IdempotencyKey string
}

type CancelInput struct {
	AppointmentID  string
	IdempotencyKey string
}

type ConfigureOpeningInput struct {
	Start    time.Time
	End      time.Time
	Capacity int
}

type SchedulingProvider interface {
	AvailableSlots(context.Context, AvailabilityQuery) ([]Slot, error)
	Book(context.Context, BookInput) (Appointment, error)
	Reschedule(context.Context, RescheduleInput) (Appointment, error)
	Cancel(context.Context, CancelInput) (Appointment, error)
}

type DayReader interface {
	Day(context.Context, time.Time) (Day, error)
}

type OpeningConfigurer interface {
	ConfigureOpening(context.Context, ConfigureOpeningInput) (Opening, error)
}

// UpdateStatusInput moves one appointment along the workshop's day.
type UpdateStatusInput struct {
	AppointmentID string
	Status        Status
}

// StatusUpdater is the persistence capability the status change needs.
type StatusUpdater interface {
	UpdateAppointmentStatus(context.Context, UpdateStatusInput) (Appointment, error)
}

// allowedTransitions is the table frozen in docs/contracts/F02A-planning.md. It
// lives here rather than in a handler because it is a rule about appointments,
// not about HTTP: the same table has to hold whoever asks, a desk or a tool.
var allowedTransitions = map[Status][]Status{
	StatusPending:    {StatusConfirmed, StatusCancelled},
	StatusConfirmed:  {StatusInProgress, StatusCancelled, StatusNoShow},
	StatusInProgress: {StatusDone},
}

// NextStatuses is what may be done to an appointment in this state. The UI reads
// it so a button never offers a move the service would refuse.
func NextStatuses(from Status) []Status {
	return allowedTransitions[from]
}

// CanTransition reports whether from -> to is allowed. Terminal states have no
// entry in the table, so they answer false for everything.
func CanTransition(from, to Status) bool {
	for _, candidate := range allowedTransitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}
