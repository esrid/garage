package voicetools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/esrid/garage/internal/adapters/voice"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
)

const maxAppointmentToolBodyBytes = 16 << 10

type AppointmentTools struct {
	scheduling    *appointment.Service
	authenticator *voice.TokenAuthenticator
}

type availabilityRequest struct {
	Day             string `json:"day"`
	DurationMinutes int    `json:"duration_minutes"`
}

type slotResponse struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type availabilityResponse struct {
	Slots []slotResponse `json:"slots"`
}

type bookingRequest struct {
	ConversationID  string `json:"conversation_id"`
	CustomerID      string `json:"customer_id"`
	VehicleID       string `json:"vehicle_id"`
	ServiceLabel    string `json:"service_label"`
	StartAt         string `json:"start_at"`
	DurationMinutes int    `json:"duration_minutes"`
	Note            string `json:"note"`
}

type bookingResponse struct {
	Confirmed   bool               `json:"confirmed"`
	Appointment *bookedAppointment `json:"appointment,omitempty"`
	Error       string             `json:"error,omitempty"`
}

type bookedAppointment struct {
	ID      string             `json:"id"`
	StartAt string             `json:"start_at"`
	EndAt   string             `json:"end_at"`
	Status  appointment.Status `json:"status"`
}

func NewAppointmentTools(scheduling *appointment.Service, authenticator *voice.TokenAuthenticator) *AppointmentTools {
	return &AppointmentTools{scheduling: scheduling, authenticator: authenticator}
}

// Register mounts the two scheduling tools the agent calls during a call.
func (h *AppointmentTools) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /voice/tools/appointment-availability", h.Availability)
	mux.HandleFunc("POST /voice/tools/appointment-book", h.Book)
}

func (h *AppointmentTools) Availability(w http.ResponseWriter, r *http.Request) {
	var input availabilityRequest
	ctx, _, err := voice.DecodeToolRequest(w, r, h.authenticator, maxAppointmentToolBodyBytes, &input)
	if err != nil {
		writeAppointmentToolError(w, appointmentToolRequestError(err), false)
		return
	}
	day, err := parseAppointmentToolTime(input.Day)
	if err != nil {
		writeAppointmentToolError(w, err, false)
		return
	}
	duration, err := parseAppointmentToolDuration(input.DurationMinutes)
	if err != nil {
		writeAppointmentToolError(w, err, false)
		return
	}
	slots, err := h.scheduling.AvailableSlots(ctx, appointment.AvailabilityQuery{Day: day, Duration: duration})
	if err != nil {
		writeAppointmentToolError(w, err, false)
		return
	}
	location, err := h.tenantLocation(ctx, day)
	if err != nil {
		writeAppointmentToolError(w, err, false)
		return
	}
	responseSlots := make([]slotResponse, 0, len(slots))
	for _, slot := range slots {
		responseSlots = append(responseSlots, slotResponse{
			StartAt: slot.Start.In(location).Format(time.RFC3339),
			EndAt:   slot.End.In(location).Format(time.RFC3339),
		})
	}
	writeAppointmentToolJSON(w, http.StatusOK, availabilityResponse{Slots: responseSlots})
}

func (h *AppointmentTools) Book(w http.ResponseWriter, r *http.Request) {
	var input bookingRequest
	ctx, tenantID, err := voice.DecodeToolRequest(w, r, h.authenticator, maxAppointmentToolBodyBytes, &input)
	if err != nil {
		writeAppointmentToolError(w, appointmentToolRequestError(err), true)
		return
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	if input.ConversationID == "" || !utf8.ValidString(input.ConversationID) || utf8.RuneCountInString(input.ConversationID) > 512 {
		writeAppointmentToolError(w, appointmentToolValidation("conversation_id"), true)
		return
	}
	start, err := parseAppointmentToolTime(input.StartAt)
	if err != nil {
		writeAppointmentToolError(w, err, true)
		return
	}
	duration, err := parseAppointmentToolDuration(input.DurationMinutes)
	if err != nil {
		writeAppointmentToolError(w, err, true)
		return
	}
	bookInput := appointment.BookInput{
		CustomerID:     strings.TrimSpace(input.CustomerID),
		VehicleID:      strings.TrimSpace(input.VehicleID),
		ServiceLabel:   strings.TrimSpace(input.ServiceLabel),
		Start:          start,
		Duration:       duration,
		Note:           strings.TrimSpace(input.Note),
		IdempotencyKey: appointmentToolIdempotencyKey(input, start, duration),
	}
	location, err := h.tenantLocation(ctx, start)
	if err != nil {
		writeAppointmentToolError(w, err, true)
		return
	}
	booked, err := h.scheduling.Book(ctx, bookInput)
	if err != nil {
		writeAppointmentToolError(w, err, true)
		return
	}
	if booked.TenantID != tenantID || booked.ID == "" || booked.Status != appointment.StatusConfirmed || booked.Start.IsZero() || !booked.End.After(booked.Start) {
		writeAppointmentToolError(w, errors.New("scheduling provider returned an invalid booking result"), true)
		return
	}
	writeAppointmentToolJSON(w, http.StatusOK, bookingResponse{
		Confirmed: true,
		Appointment: &bookedAppointment{
			ID:      booked.ID,
			StartAt: booked.Start.In(location).Format(time.RFC3339),
			EndAt:   booked.End.In(location).Format(time.RFC3339),
			Status:  booked.Status,
		},
	})
}

func (h *AppointmentTools) tenantLocation(ctx context.Context, instant time.Time) (*time.Location, error) {
	day, err := h.scheduling.Day(ctx, instant)
	if err != nil {
		return nil, err
	}
	if day.Timezone == "" {
		return nil, errors.New("scheduling provider returned no tenant timezone")
	}
	location, err := time.LoadLocation(day.Timezone)
	if err != nil {
		return nil, errors.New("scheduling provider returned an invalid tenant timezone")
	}
	return location, nil
}

func parseAppointmentToolTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, appointmentToolValidation("timestamp")
	}
	return parsed, nil
}

func parseAppointmentToolDuration(minutes int) (time.Duration, error) {
	if minutes < 15 || minutes > 480 || minutes%15 != 0 {
		return 0, appointmentToolValidation("duration_minutes")
	}
	return time.Duration(minutes) * time.Minute, nil
}

func appointmentToolIdempotencyKey(input bookingRequest, start time.Time, duration time.Duration) string {
	parts := []string{
		strings.TrimSpace(input.ConversationID),
		strings.TrimSpace(input.CustomerID),
		strings.TrimSpace(input.VehicleID),
		strings.TrimSpace(input.ServiceLabel),
		start.UTC().Format(time.RFC3339Nano),
		strconv.FormatInt(int64(duration/time.Minute), 10),
		strings.TrimSpace(input.Note),
	}
	var canonical strings.Builder
	for _, part := range parts {
		canonical.WriteString(strconv.Itoa(len(part)))
		canonical.WriteByte(':')
		canonical.WriteString(part)
	}
	hash := sha256.Sum256([]byte(canonical.String()))
	return "voice-book-" + hex.EncodeToString(hash[:])
}

// appointmentToolRequestError turns the shared preamble's reason into the error
// this tool already knows how to answer with.
func appointmentToolRequestError(err error) error {
	if errors.Is(err, voice.ErrToolUnauthorized) {
		return &domain.UnauthorizedError{Message: "voice tool authentication required"}
	}
	return appointmentToolValidation("body")
}

func appointmentToolValidation(field string) error {
	return &domain.ValidationError{Entity: "voice appointment", Errors: map[string]string{field: "invalid"}}
}

func writeAppointmentToolError(w http.ResponseWriter, err error, booking bool) {
	status := http.StatusServiceUnavailable
	message := "service unavailable"
	var unauthorized *domain.UnauthorizedError
	var validation *domain.ValidationError
	var notFound *domain.NotFoundError
	var alreadyExists *domain.AlreadyExistsError
	switch {
	case errors.As(err, &unauthorized):
		status, message = http.StatusUnauthorized, "authentication required"
	case errors.As(err, &validation), errors.Is(err, appointment.ErrInvalidTransition):
		status, message = http.StatusUnprocessableEntity, "invalid request"
	case errors.As(err, &notFound):
		status, message = http.StatusNotFound, "resource not found"
	case errors.As(err, &alreadyExists), errors.Is(err, appointment.ErrSlotUnavailable), errors.Is(err, appointment.ErrIdempotencyConflict):
		status, message = http.StatusConflict, "appointment conflict"
	}
	if booking {
		writeAppointmentToolJSON(w, status, bookingResponse{Confirmed: false, Error: message})
		return
	}
	writeAppointmentToolJSON(w, status, map[string]string{"error": message})
}

func writeAppointmentToolJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
