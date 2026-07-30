package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	_ appointment.SchedulingProvider = (*Store)(nil)
	_ appointment.DayReader          = (*Store)(nil)
	_ appointment.OpeningConfigurer  = (*Store)(nil)
)

const appointmentColumns = `
	id::text, tenant_id::text, customer_id::text, COALESCE(vehicle_id::text, ''),
	opening_id::text, service_label, note, starts_at, ends_at, status,
	created_at, updated_at`

func (s *Store) ConfigureOpening(ctx context.Context, input appointment.ConfigureOpeningInput) (appointment.Opening, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return appointment.Opening{}, err
	}
	const query = `
		INSERT INTO workshop_openings (tenant_id, starts_at, ends_at, capacity)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, starts_at, ends_at, capacity`
	var value appointment.Opening
	err = s.pool.QueryRow(ctx, query, tenantID, input.Start, input.End, input.Capacity).Scan(
		&value.ID, &value.Start, &value.End, &value.Capacity,
	)
	if err != nil {
		// 23P01 is the exclusion constraint from migration 00006: this window
		// overlaps one the workshop already has, and two overlapping openings make
		// capacity ambiguous. A conflict, not a server fault.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" && pgErr.ConstraintName == "workshop_openings_no_overlap" {
			return appointment.Opening{}, &domain.AlreadyExistsError{Entity: "opening", Field: "range", Value: input.Start.Format(time.RFC3339)}
		}
		return appointment.Opening{}, fmt.Errorf("postgres: configure opening: %w", err)
	}
	return value, nil
}

func (s *Store) AvailableSlots(ctx context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error) {
	day, err := s.Day(ctx, query.Day)
	if err != nil {
		return nil, err
	}
	slots := make([]appointment.Slot, 0)
	dayStart := day.Date
	dayEnd := dayStart.AddDate(0, 0, 1)
	for _, opening := range day.Openings {
		openingStart := opening.Start
		if openingStart.Before(dayStart) {
			openingStart = dayStart
		}
		openingEnd := opening.End
		if openingEnd.After(dayEnd) {
			openingEnd = dayEnd
		}
		for start := openingStart; !start.Add(query.Duration).After(openingEnd); start = start.Add(15 * time.Minute) {
			end := start.Add(query.Duration)
			if hasCapacity(day.Appointments, opening.ID, start, end, opening.Capacity, "") {
				slots = append(slots, appointment.Slot{Start: start, End: end})
			}
		}
	}
	return slots, nil
}

func (s *Store) Book(ctx context.Context, input appointment.BookInput) (appointment.Appointment, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return appointment.Appointment{}, err
	}
	requestHash := hashParts(input.CustomerID, input.VehicleID, input.ServiceLabel, input.Start.UTC().Format(time.RFC3339Nano), input.Duration.String(), input.Note)
	var result appointment.Appointment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		duplicate, existing, claimErr := claimAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, "book", requestHash)
		if claimErr != nil {
			return claimErr
		}
		if duplicate {
			result = existing
			return nil
		}

		end := input.Start.Add(input.Duration)
		openingID, capacity, lockErr := lockOpening(ctx, tx, tenantID, input.Start, end)
		if lockErr != nil {
			return lockErr
		}
		if err := ensureCapacity(ctx, tx, tenantID, openingID, input.Start, end, capacity, ""); err != nil {
			return err
		}

		const query = `
			INSERT INTO appointments (
				tenant_id, customer_id, vehicle_id, opening_id, service_label,
				note, starts_at, ends_at, status
			)
			VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4::uuid, $5, $6, $7, $8, 'confirmed')
			RETURNING ` + appointmentColumns
		result, err = scanAppointment(tx.QueryRow(ctx, query,
			tenantID, input.CustomerID, input.VehicleID, openingID,
			input.ServiceLabel, input.Note, input.Start, end,
		))
		if err != nil {
			return mapAppointmentWriteError(err, input.CustomerID, input.VehicleID)
		}
		return finishAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, result)
	})
	if err != nil {
		return appointment.Appointment{}, fmt.Errorf("postgres: book appointment: %w", err)
	}
	return result, nil
}

func (s *Store) Reschedule(ctx context.Context, input appointment.RescheduleInput) (appointment.Appointment, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return appointment.Appointment{}, err
	}
	requestHash := hashParts(input.AppointmentID, input.Start.UTC().Format(time.RFC3339Nano), input.Duration.String())
	var result appointment.Appointment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		duplicate, existing, claimErr := claimAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, "reschedule", requestHash)
		if claimErr != nil {
			return claimErr
		}
		if duplicate {
			result = existing
			return nil
		}

		current, loadErr := loadAppointment(ctx, tx, tenantID, input.AppointmentID, true)
		if loadErr != nil {
			return loadErr
		}
		if current.Status != appointment.StatusPending && current.Status != appointment.StatusConfirmed {
			return appointment.ErrInvalidTransition
		}
		end := input.Start.Add(input.Duration)
		openingID, capacity, lockErr := lockOpening(ctx, tx, tenantID, input.Start, end)
		if lockErr != nil {
			return lockErr
		}
		if err := ensureCapacity(ctx, tx, tenantID, openingID, input.Start, end, capacity, current.ID); err != nil {
			return err
		}

		query := `UPDATE appointments
			SET opening_id = $3::uuid, starts_at = $4, ends_at = $5, status = 'confirmed', updated_at = now()
			WHERE tenant_id = $1::uuid AND id = $2::uuid
			RETURNING ` + appointmentColumns
		result, err = scanAppointment(tx.QueryRow(ctx, query, tenantID, current.ID, openingID, input.Start, end))
		if err != nil {
			return fmt.Errorf("update appointment: %w", err)
		}
		return finishAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, result)
	})
	if err != nil {
		return appointment.Appointment{}, fmt.Errorf("postgres: reschedule appointment: %w", err)
	}
	return result, nil
}

func (s *Store) Cancel(ctx context.Context, input appointment.CancelInput) (appointment.Appointment, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return appointment.Appointment{}, err
	}
	requestHash := hashParts(input.AppointmentID)
	var result appointment.Appointment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		duplicate, existing, claimErr := claimAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, "cancel", requestHash)
		if claimErr != nil {
			return claimErr
		}
		if duplicate {
			result = existing
			return nil
		}
		current, loadErr := loadAppointment(ctx, tx, tenantID, input.AppointmentID, true)
		if loadErr != nil {
			return loadErr
		}
		if current.Status != appointment.StatusPending && current.Status != appointment.StatusConfirmed {
			return appointment.ErrInvalidTransition
		}
		query := `UPDATE appointments
			SET status = 'cancelled', updated_at = now()
			WHERE tenant_id = $1::uuid AND id = $2::uuid
			RETURNING ` + appointmentColumns
		result, err = scanAppointment(tx.QueryRow(ctx, query, tenantID, current.ID))
		if err != nil {
			return fmt.Errorf("cancel appointment: %w", err)
		}
		return finishAppointmentCommand(ctx, tx, tenantID, input.IdempotencyKey, result)
	})
	if err != nil {
		return appointment.Appointment{}, fmt.Errorf("postgres: cancel appointment: %w", err)
	}
	return result, nil
}

func (s *Store) Day(ctx context.Context, day time.Time) (appointment.Day, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return appointment.Day{}, err
	}
	dayStart, dayEnd, timezone, err := s.tenantDayBounds(ctx, tenantID, day)
	if err != nil {
		return appointment.Day{}, err
	}
	result := appointment.Day{Date: dayStart, Timezone: timezone, Openings: make([]appointment.Opening, 0), Appointments: make([]appointment.DayEntry, 0)}

	openingRows, err := s.pool.Query(ctx, `SELECT id::text, starts_at, ends_at, capacity
		FROM workshop_openings
		WHERE tenant_id = $1::uuid AND starts_at < $3 AND ends_at > $2
		ORDER BY starts_at, id`, tenantID, dayStart, dayEnd)
	if err != nil {
		return appointment.Day{}, fmt.Errorf("postgres: list openings: %w", err)
	}
	for openingRows.Next() {
		var value appointment.Opening
		if err := openingRows.Scan(&value.ID, &value.Start, &value.End, &value.Capacity); err != nil {
			openingRows.Close()
			return appointment.Day{}, fmt.Errorf("postgres: scan opening: %w", err)
		}
		result.Openings = append(result.Openings, value)
	}
	if err := openingRows.Err(); err != nil {
		openingRows.Close()
		return appointment.Day{}, fmt.Errorf("postgres: opening rows: %w", err)
	}
	openingRows.Close()

	rows, err := s.pool.Query(ctx, `SELECT
		a.id::text, a.tenant_id::text, a.customer_id::text, COALESCE(a.vehicle_id::text, ''),
		a.opening_id::text, a.service_label, a.note, a.starts_at, a.ends_at, a.status,
		a.created_at, a.updated_at,
		btrim(concat_ws(' ', c.first_name, c.last_name)),
		btrim(concat_ws(' ', v.make, v.model)), COALESCE(v.plate, '')
		FROM appointments a
		JOIN customers c ON c.tenant_id = a.tenant_id AND c.id = a.customer_id
		LEFT JOIN vehicles v ON v.tenant_id = a.tenant_id AND v.id = a.vehicle_id
		WHERE a.tenant_id = $1::uuid AND a.ends_at > $2 AND a.starts_at < $3
		ORDER BY a.starts_at, a.id`, tenantID, dayStart, dayEnd)
	if err != nil {
		return appointment.Day{}, fmt.Errorf("postgres: list day appointments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry appointment.DayEntry
		if err := rows.Scan(
			&entry.ID, &entry.TenantID, &entry.CustomerID, &entry.VehicleID,
			&entry.OpeningID, &entry.ServiceLabel, &entry.Note, &entry.Start,
			&entry.End, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt,
			&entry.CustomerName, &entry.VehicleLabel, &entry.Plate,
		); err != nil {
			return appointment.Day{}, fmt.Errorf("postgres: scan day appointment: %w", err)
		}
		result.Appointments = append(result.Appointments, entry)
	}
	if err := rows.Err(); err != nil {
		return appointment.Day{}, fmt.Errorf("postgres: day appointment rows: %w", err)
	}
	return result, nil
}

func (s *Store) tenantDayBounds(ctx context.Context, tenantID string, day time.Time) (time.Time, time.Time, string, error) {
	var timezone string
	if err := s.pool.QueryRow(ctx, `SELECT timezone FROM tenants WHERE id = $1::uuid`, tenantID).Scan(&timezone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, time.Time{}, "", &domain.NotFoundError{Entity: "tenant"}
		}
		return time.Time{}, time.Time{}, "", fmt.Errorf("postgres: tenant timezone: %w", err)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("postgres: load tenant timezone: %w", err)
	}
	local := day.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1), timezone, nil
}

func lockOpening(ctx context.Context, tx pgx.Tx, tenantID string, start, end time.Time) (string, int, error) {
	const query = `SELECT id::text, capacity FROM workshop_openings
		WHERE tenant_id = $1::uuid AND starts_at <= $2 AND ends_at >= $3
		ORDER BY starts_at, id LIMIT 1 FOR UPDATE`
	var openingID string
	var capacity int
	if err := tx.QueryRow(ctx, query, tenantID, start, end).Scan(&openingID, &capacity); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, appointment.ErrSlotUnavailable
		}
		return "", 0, fmt.Errorf("lock opening: %w", err)
	}
	return openingID, capacity, nil
}

func ensureCapacity(ctx context.Context, tx pgx.Tx, tenantID, openingID string, start, end time.Time, capacity int, excludeID string) error {
	const query = `WITH grouped_events AS (
		SELECT event_at, sum(delta) AS delta
		FROM (
			SELECT greatest(starts_at, $3::timestamptz) AS event_at, 1 AS delta
			FROM appointments
			WHERE tenant_id = $1::uuid AND opening_id = $2::uuid
			AND status IN ('pending', 'confirmed', 'in_progress')
			AND starts_at < $4 AND ends_at > $3
			AND ($5::text = '' OR id::text <> $5)
			UNION ALL
			SELECT least(ends_at, $4::timestamptz) AS event_at, -1 AS delta
			FROM appointments
			WHERE tenant_id = $1::uuid AND opening_id = $2::uuid
			AND status IN ('pending', 'confirmed', 'in_progress')
			AND starts_at < $4 AND ends_at > $3
			AND ($5::text = '' OR id::text <> $5)
		) AS events
		GROUP BY event_at
	), concurrent_usage AS (
		SELECT sum(delta) OVER (ORDER BY event_at ROWS UNBOUNDED PRECEDING) AS used
		FROM grouped_events
	)
	SELECT COALESCE(max(used), 0) FROM concurrent_usage`
	var peak int64
	if err := tx.QueryRow(ctx, query, tenantID, openingID, start, end, excludeID).Scan(&peak); err != nil {
		return fmt.Errorf("calculate overlapping appointment capacity: %w", err)
	}
	if peak >= int64(capacity) {
		return appointment.ErrSlotUnavailable
	}
	return nil
}

func claimAppointmentCommand(ctx context.Context, tx pgx.Tx, tenantID, key, operation, requestHash string) (bool, appointment.Appointment, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO appointment_commands
		(tenant_id, idempotency_key, operation, request_hash)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT DO NOTHING`, tenantID, key, operation, requestHash)
	if err != nil {
		return false, appointment.Appointment{}, fmt.Errorf("claim idempotency key: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return false, appointment.Appointment{}, nil
	}

	var storedOperation, storedHash, appointmentID string
	var encodedResult []byte
	err = tx.QueryRow(ctx, `SELECT operation, request_hash, appointment_id::text, result
		FROM appointment_commands
		WHERE tenant_id = $1::uuid AND idempotency_key = $2`, tenantID, key).Scan(
		&storedOperation, &storedHash, &appointmentID, &encodedResult,
	)
	if err != nil {
		return false, appointment.Appointment{}, fmt.Errorf("read idempotency result: %w", err)
	}
	if storedOperation != operation || storedHash != requestHash {
		return false, appointment.Appointment{}, appointment.ErrIdempotencyConflict
	}
	var value appointment.Appointment
	if err := json.Unmarshal(encodedResult, &value); err != nil {
		return false, appointment.Appointment{}, fmt.Errorf("decode idempotency result: %w", err)
	}
	if value.ID != appointmentID || value.TenantID != tenantID {
		return false, appointment.Appointment{}, fmt.Errorf("idempotency result does not match command")
	}
	return true, value, nil
}

func finishAppointmentCommand(ctx context.Context, tx pgx.Tx, tenantID, key string, value appointment.Appointment) error {
	encodedResult, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode idempotency result: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE appointment_commands
		SET appointment_id = $3::uuid, result = $4::jsonb
		WHERE tenant_id = $1::uuid AND idempotency_key = $2`,
		tenantID, key, value.ID, encodedResult,
	)
	if err != nil {
		return fmt.Errorf("store idempotency result: %w", err)
	}
	return nil
}

func loadAppointment(ctx context.Context, tx pgx.Tx, tenantID, appointmentID string, forUpdate bool) (appointment.Appointment, error) {
	query := `SELECT ` + appointmentColumns + ` FROM appointments
		WHERE tenant_id = $1::uuid AND id = $2::uuid`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	value, err := scanAppointment(tx.QueryRow(ctx, query, tenantID, appointmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return appointment.Appointment{}, &domain.NotFoundError{Entity: "appointment", ID: appointmentID}
	}
	if err != nil {
		return appointment.Appointment{}, fmt.Errorf("load appointment: %w", err)
	}
	return value, nil
}

func scanAppointment(row pgx.Row) (appointment.Appointment, error) {
	var value appointment.Appointment
	err := row.Scan(
		&value.ID, &value.TenantID, &value.CustomerID, &value.VehicleID,
		&value.OpeningID, &value.ServiceLabel, &value.Note, &value.Start,
		&value.End, &value.Status, &value.CreatedAt, &value.UpdatedAt,
	)
	return value, err
}

func mapAppointmentWriteError(err error, customerID, vehicleID string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		switch pgErr.ConstraintName {
		case "appointments_tenant_customer_fkey":
			return &domain.NotFoundError{Entity: "customer", ID: customerID}
		case "appointments_tenant_customer_vehicle_fkey":
			return &domain.NotFoundError{Entity: "vehicle", ID: vehicleID}
		}
	}
	return fmt.Errorf("insert appointment: %w", err)
}

func hashParts(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:])
}

func blocksCapacity(status appointment.Status) bool {
	return status == appointment.StatusPending || status == appointment.StatusConfirmed || status == appointment.StatusInProgress
}

func hasCapacity(entries []appointment.DayEntry, openingID string, start, end time.Time, capacity int, excludeID string) bool {
	events := make(map[int64]int)
	for _, entry := range entries {
		if entry.OpeningID != openingID || entry.ID == excludeID || !blocksCapacity(entry.Status) || !entry.Start.Before(end) || !entry.End.After(start) {
			continue
		}
		overlapStart := entry.Start
		if overlapStart.Before(start) {
			overlapStart = start
		}
		overlapEnd := entry.End
		if overlapEnd.After(end) {
			overlapEnd = end
		}
		events[overlapStart.UnixNano()]++
		events[overlapEnd.UnixNano()]--
	}
	timestamps := make([]int64, 0, len(events))
	for timestamp := range events {
		timestamps = append(timestamps, timestamp)
	}
	slices.Sort(timestamps)
	concurrent := 0
	for _, timestamp := range timestamps {
		concurrent += events[timestamp]
		if concurrent >= capacity {
			return false
		}
	}
	return true
}
