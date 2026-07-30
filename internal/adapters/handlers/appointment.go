package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
)

const maxAppointmentFormBytes = 64 << 10

// AppointmentMutations owns the POST side of the frozen F02A HTTP contract.
// Planning pages and fragments remain owned by F02B.
type AppointmentMutations struct {
	service *appointment.Service
}

func NewAppointmentMutations(service *appointment.Service) *AppointmentMutations {
	return &AppointmentMutations{service: service}
}

func (h *AppointmentMutations) Book(w http.ResponseWriter, r *http.Request) {
	if err := parseAppointmentForm(w, r); err != nil {
		writeAppointmentError(w, err)
		return
	}
	start, err := parseStart(r.PostForm.Get("start_at"))
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	duration, err := parseDuration(r.PostForm.Get("duration_minutes"))
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	created, err := h.service.Book(r.Context(), appointment.BookInput{
		CustomerID:     r.PostForm.Get("customer_id"),
		VehicleID:      r.PostForm.Get("vehicle_id"),
		ServiceLabel:   r.PostForm.Get("service_label"),
		Start:          start,
		Duration:       duration,
		Note:           r.PostForm.Get("note"),
		IdempotencyKey: r.PostForm.Get("idempotency_key"),
	})
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	h.redirectToDay(w, r, created.Start)
}

func (h *AppointmentMutations) Reschedule(w http.ResponseWriter, r *http.Request) {
	if err := parseAppointmentForm(w, r); err != nil {
		writeAppointmentError(w, err)
		return
	}
	start, err := parseStart(r.PostForm.Get("start_at"))
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	duration, err := parseDuration(r.PostForm.Get("duration_minutes"))
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	updated, err := h.service.Reschedule(r.Context(), appointment.RescheduleInput{
		AppointmentID:  r.PathValue("id"),
		Start:          start,
		Duration:       duration,
		IdempotencyKey: r.PostForm.Get("idempotency_key"),
	})
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	h.redirectToDay(w, r, updated.Start)
}

func (h *AppointmentMutations) Cancel(w http.ResponseWriter, r *http.Request) {
	if err := parseAppointmentForm(w, r); err != nil {
		writeAppointmentError(w, err)
		return
	}
	cancelled, err := h.service.Cancel(r.Context(), appointment.CancelInput{
		AppointmentID:  r.PathValue("id"),
		IdempotencyKey: r.PostForm.Get("idempotency_key"),
	})
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	h.redirectToDay(w, r, cancelled.Start)
}

func (h *AppointmentMutations) redirectToDay(w http.ResponseWriter, r *http.Request, instant time.Time) {
	day, err := h.service.Day(r.Context(), instant)
	if err != nil {
		writeAppointmentError(w, err)
		return
	}
	http.Redirect(w, r, "/app/planning?day="+day.Date.Format(time.DateOnly), http.StatusSeeOther)
}

func parseAppointmentForm(w http.ResponseWriter, r *http.Request) error {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/x-www-form-urlencoded" {
		return appointmentValidation("form", "content type must be application/x-www-form-urlencoded")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAppointmentFormBytes)
	if err := r.ParseForm(); err != nil {
		return appointmentValidation("form", "could not be parsed")
	}
	return nil
}

func parseStart(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, appointmentValidation("start_at", "must be an RFC3339 timestamp with offset")
	}
	return parsed, nil
}

func parseDuration(value string) (time.Duration, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || minutes < 15 || minutes > 480 || minutes%15 != 0 {
		return 0, appointmentValidation("duration_minutes", "must be 15 to 480 in 15-minute increments")
	}
	return time.Duration(minutes) * time.Minute, nil
}

func appointmentValidation(field, message string) error {
	return &domain.ValidationError{Entity: "appointment", Errors: map[string]string{field: message}}
}

func writeAppointmentError(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
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
		status, message = http.StatusUnprocessableEntity, "invalid appointment request"
	case errors.As(err, &notFound):
		status, message = http.StatusNotFound, "resource not found"
	case errors.As(err, &alreadyExists), errors.Is(err, appointment.ErrSlotUnavailable), errors.Is(err, appointment.ErrIdempotencyConflict):
		status, message = http.StatusConflict, "appointment conflict"
	}
	http.Error(w, message, status)
}
