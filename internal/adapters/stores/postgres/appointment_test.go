package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/esrid/garage/internal/core/vehicle"
)

func TestAppointmentSchedulingTenantIsolationIdempotencyAndCapacity(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tenantService := tenant.NewService(store)
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Planning A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Planning B"})
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	t.Cleanup(func() {
		for _, tenantID := range []string{tenantA.ID, tenantB.ID} {
			if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantID); cleanupErr != nil {
				t.Errorf("cleanup tenant %s: %v", tenantID, cleanupErr)
			}
		}
	})

	ctxA := tenant.WithID(ctx, tenantA.ID)
	ctxB := tenant.WithID(ctx, tenantB.ID)
	customerService := customer.NewService(store)
	customerA1, err := customerService.Create(ctxA, customer.CreateInput{FirstName: "Ana", Phone: "+596696100001"})
	if err != nil {
		t.Fatalf("create customer A1: %v", err)
	}
	customerA2, err := customerService.Create(ctxA, customer.CreateInput{FirstName: "Luc", Phone: "+596696100002"})
	if err != nil {
		t.Fatalf("create customer A2: %v", err)
	}
	customerB, err := customerService.Create(ctxB, customer.CreateInput{FirstName: "Mia", Phone: "+596696100003"})
	if err != nil {
		t.Fatalf("create customer B: %v", err)
	}
	vehicleA, err := vehicle.NewService(store).Create(ctxA, vehicle.CreateInput{CustomerID: customerA1.ID, Plate: "AA-111-AA", Make: "Renault", Model: "Clio"})
	if err != nil {
		t.Fatalf("create vehicle A: %v", err)
	}

	scheduling := appointment.NewService(store, store, store)
	start := mustParseTime(t, "2030-01-02T08:00:00-04:00")
	for _, tenantCtx := range []context.Context{ctxA, ctxB} {
		if _, err := scheduling.ConfigureOpening(tenantCtx, appointment.ConfigureOpeningInput{Start: start, End: start.Add(2 * time.Hour), Capacity: 1}); err != nil {
			t.Fatalf("configure opening: %v", err)
		}
	}
	if _, err := scheduling.ConfigureOpening(ctxA, appointment.ConfigureOpeningInput{Start: start.Add(2 * time.Hour), End: start.Add(4 * time.Hour), Capacity: 1}); err != nil {
		t.Fatalf("configure second opening: %v", err)
	}

	slots, err := scheduling.AvailableSlots(ctxA, appointment.AvailabilityQuery{Day: start, Duration: time.Hour})
	if err != nil || len(slots) != 10 {
		t.Fatalf("initial slots = %#v, %v", slots, err)
	}

	bookInput := appointment.BookInput{
		CustomerID: customerA1.ID, VehicleID: vehicleA.ID, ServiceLabel: "Révision",
		Start: start, Duration: time.Hour, IdempotencyKey: "book-a-1",
	}
	booked, err := scheduling.Book(ctxA, bookInput)
	if err != nil || booked.Status != appointment.StatusConfirmed {
		t.Fatalf("Book() = %#v, %v", booked, err)
	}
	retried, err := scheduling.Book(ctxA, bookInput)
	if err != nil || retried.ID != booked.ID || retried.Start != booked.Start {
		t.Fatalf("idempotent Book() = %#v, %v", retried, err)
	}
	changed := bookInput
	changed.Start = start.Add(time.Hour)
	if _, err := scheduling.Book(ctxA, changed); !errors.Is(err, appointment.ErrIdempotencyConflict) {
		t.Fatalf("changed idempotency request error = %v", err)
	}

	_, err = scheduling.Book(ctxA, appointment.BookInput{
		CustomerID: customerA2.ID, VehicleID: vehicleA.ID, ServiceLabel: "Freins",
		Start: start.Add(2 * time.Hour), Duration: time.Hour, IdempotencyKey: "wrong-customer-vehicle",
	})
	var notFoundErr *domain.NotFoundError
	if !errors.As(err, &notFoundErr) || notFoundErr.Entity != "vehicle" {
		t.Fatalf("wrong customer vehicle error = %v, want vehicle NotFoundError", err)
	}

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, customerID := range []string{customerA1.ID, customerA2.ID} {
		wait.Add(1)
		go func(index int, customerID string) {
			defer wait.Done()
			_, bookErr := scheduling.Book(ctxA, appointment.BookInput{
				CustomerID: customerID, ServiceLabel: "Créneau concurrent",
				Start: start.Add(time.Hour), Duration: time.Hour,
				IdempotencyKey: "concurrent-" + string(rune('a'+index)),
			})
			results <- bookErr
		}(index, customerID)
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for resultErr := range results {
		switch {
		case resultErr == nil:
			successes++
		case errors.Is(resultErr, appointment.ErrSlotUnavailable):
			conflicts++
		default:
			t.Fatalf("concurrent booking error = %v", resultErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent booking successes=%d conflicts=%d", successes, conflicts)
	}

	bookedB, err := scheduling.Book(ctxB, appointment.BookInput{
		CustomerID: customerB.ID, ServiceLabel: "Même heure autre tenant",
		Start: start, Duration: time.Hour, IdempotencyKey: "book-b-1",
	})
	if err != nil || bookedB.TenantID != tenantB.ID {
		t.Fatalf("tenant B Book() = %#v, %v", bookedB, err)
	}
	if _, err := scheduling.Cancel(ctxB, appointment.CancelInput{AppointmentID: booked.ID, IdempotencyKey: "cross-cancel"}); !errors.As(err, &notFoundErr) {
		t.Fatalf("cross-tenant cancel error = %v, want NotFoundError", err)
	}

	rescheduleInput := appointment.RescheduleInput{
		AppointmentID: booked.ID, Start: start.Add(2 * time.Hour), Duration: time.Hour,
		IdempotencyKey: "reschedule-a-1",
	}
	rescheduled, err := scheduling.Reschedule(ctxA, rescheduleInput)
	if err != nil || !rescheduled.Start.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("Reschedule() = %#v, %v", rescheduled, err)
	}
	retriedReschedule, err := scheduling.Reschedule(ctxA, rescheduleInput)
	if err != nil || retriedReschedule.ID != booked.ID || !retriedReschedule.Start.Equal(rescheduled.Start) {
		t.Fatalf("idempotent Reschedule() = %#v, %v", retriedReschedule, err)
	}
	retriedOriginalBook, err := scheduling.Book(ctxA, bookInput)
	if err != nil || retriedOriginalBook.OpeningID != booked.OpeningID ||
		retriedOriginalBook.ServiceLabel != booked.ServiceLabel || retriedOriginalBook.Note != booked.Note ||
		!retriedOriginalBook.Start.Equal(booked.Start) || !retriedOriginalBook.End.Equal(booked.End) ||
		!retriedOriginalBook.CreatedAt.Equal(booked.CreatedAt) || !retriedOriginalBook.UpdatedAt.Equal(booked.UpdatedAt) {
		t.Fatalf("original Book() result changed after reschedule: got %#v, want %#v, err=%v", retriedOriginalBook, booked, err)
	}

	cancelInput := appointment.CancelInput{AppointmentID: booked.ID, IdempotencyKey: "cancel-a-1"}
	cancelled, err := scheduling.Cancel(ctxA, cancelInput)
	if err != nil || cancelled.Status != appointment.StatusCancelled {
		t.Fatalf("Cancel() = %#v, %v", cancelled, err)
	}
	retriedCancel, err := scheduling.Cancel(ctxA, cancelInput)
	if err != nil || retriedCancel.ID != cancelled.ID || retriedCancel.Status != appointment.StatusCancelled {
		t.Fatalf("idempotent Cancel() = %#v, %v", retriedCancel, err)
	}

	day, err := scheduling.Day(ctxA, start)
	if err != nil || day.Timezone != tenant.DefaultTimezone || len(day.Appointments) < 2 {
		t.Fatalf("Day() = %#v, %v", day, err)
	}
}

func TestCapacityUsesPeakConcurrencyInsteadOfTotalOverlapCount(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Planning capacité deux"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})
	tenantCtx := tenant.WithID(ctx, tenantValue.ID)
	customerValue, err := customer.NewService(store).Create(tenantCtx, customer.CreateInput{FirstName: "Capucine", Phone: "+596696100005"})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}

	scheduling := appointment.NewService(store, store, store)
	start := mustParseTime(t, "2030-02-04T08:00:00-04:00")
	if _, err := scheduling.ConfigureOpening(tenantCtx, appointment.ConfigureOpeningInput{
		Start: start, End: start.Add(4 * time.Hour), Capacity: 2,
	}); err != nil {
		t.Fatalf("configure opening: %v", err)
	}
	for index, offset := range []time.Duration{0, 2 * time.Hour} {
		if _, err := scheduling.Book(tenantCtx, appointment.BookInput{
			CustomerID: customerValue.ID, ServiceLabel: "Occupation séparée",
			Start: start.Add(offset), Duration: time.Hour,
			IdempotencyKey: "separate-" + string(rune('a'+index)),
		}); err != nil {
			t.Fatalf("book separated appointment %d: %v", index, err)
		}
	}

	candidateStart := start.Add(30 * time.Minute)
	slots, err := scheduling.AvailableSlots(tenantCtx, appointment.AvailabilityQuery{Day: start, Duration: 2 * time.Hour})
	if err != nil {
		t.Fatalf("AvailableSlots() error = %v", err)
	}
	foundCandidate := false
	for _, slot := range slots {
		if slot.Start.Equal(candidateStart) {
			foundCandidate = true
			break
		}
	}
	if !foundCandidate {
		t.Fatalf("candidate %v missing from slots %#v", candidateStart, slots)
	}
	if _, err := scheduling.Book(tenantCtx, appointment.BookInput{
		CustomerID: customerValue.ID, ServiceLabel: "Créneau traversant",
		Start: candidateStart, Duration: 2 * time.Hour, IdempotencyKey: "crossing-capacity-2",
	}); err != nil {
		t.Fatalf("Book() with peak usage one and capacity two error = %v", err)
	}
	if _, err := scheduling.Book(tenantCtx, appointment.BookInput{
		CustomerID: customerValue.ID, ServiceLabel: "Créneau en trop",
		Start: candidateStart, Duration: 2 * time.Hour, IdempotencyKey: "crossing-capacity-3",
	}); !errors.Is(err, appointment.ErrSlotUnavailable) {
		t.Fatalf("third concurrent Book() error = %v, want ErrSlotUnavailable", err)
	}
}

func TestAvailableSlotsClipsCrossMidnightOpeningsAndCountsOverlappingAppointments(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Planning nuit"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})
	tenantCtx := tenant.WithID(ctx, tenantValue.ID)
	customerValue, err := customer.NewService(store).Create(tenantCtx, customer.CreateInput{FirstName: "Nora", Phone: "+596696100004"})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}

	scheduling := appointment.NewService(store, store, store)
	openingStart := mustParseTime(t, "2030-01-01T22:00:00-04:00")
	if _, err := scheduling.ConfigureOpening(tenantCtx, appointment.ConfigureOpeningInput{
		Start: openingStart, End: openingStart.Add(6 * time.Hour), Capacity: 1,
	}); err != nil {
		t.Fatalf("configure opening: %v", err)
	}
	if _, err := scheduling.Book(tenantCtx, appointment.BookInput{
		CustomerID: customerValue.ID, ServiceLabel: "Intervention de nuit",
		Start: openingStart.Add(time.Hour), Duration: 4 * time.Hour, IdempotencyKey: "night-booking",
	}); err != nil {
		t.Fatalf("book crossing appointment: %v", err)
	}

	selectedDay := mustParseTime(t, "2030-01-02T12:00:00-04:00")
	slots, err := scheduling.AvailableSlots(tenantCtx, appointment.AvailabilityQuery{Day: selectedDay, Duration: time.Hour})
	if err != nil {
		t.Fatalf("AvailableSlots() error = %v", err)
	}
	if len(slots) != 1 || !slots[0].Start.Equal(mustParseTime(t, "2030-01-02T03:00:00-04:00")) {
		t.Fatalf("slots = %#v, want only 03:00 after the overlapping appointment", slots)
	}
	for _, slot := range slots {
		if slot.Start.Day() != 2 {
			t.Fatalf("slot %v escaped the selected calendar day", slot.Start)
		}
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
