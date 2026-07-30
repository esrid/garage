# Contract — F02A Workshop Planning Backend

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumers:** F02B planning UI, F04 dashboard, F05 voice booking.

Changing a route, form field, status, Go signature or tenant boundary below
requires an explicit mini-task in `WORKBOARD.md` before implementation.

## Tenant and authentication boundary

- `tenant_id` never appears in a route, query string, form or service input.
- authenticated middleware puts the tenant ID in `context.Context`;
- every service resolves it with `tenant.IDFromContext`;
- every PostgreSQL read/write includes the resolved tenant ID;
- appointment IDs alone never authorize access.

Until authentication and CSRF middleware exist, mutation routes are implemented
and tested but must not be exposed as a public unauthenticated deployment.

### Amendment 2026-07-30 — F09 authentication

F09 now supplies that middleware. Every `/app` route requires a valid server
session, and unsafe browser requests pass through cross-origin protection before
the mutation handler. The tenant still comes only from `context.Context`; no
route or form field changed. See `docs/contracts/F09-authentication.md` and
mini-task MT-04.

## Canonical statuses

```text
pending | confirmed | in_progress | done | cancelled | no_show
```

Allowed transitions:

| From | To |
|---|---|
| `pending` | `confirmed`, `cancelled` |
| `confirmed` | `in_progress`, `cancelled`, `no_show` |
| `in_progress` | `done` |

`done`, `cancelled` and `no_show` are terminal. Booking and rescheduling forms do
not accept a status field. A successful booking is returned as `confirmed` only
after the database transaction commits.

## Domain/provider seam

The external-risk boundary is the need-oriented provider required by the PRD:

```go
type SchedulingProvider interface {
    AvailableSlots(ctx context.Context, query AvailabilityQuery) ([]Slot, error)
    Book(ctx context.Context, input BookInput) (Appointment, error)
    Reschedule(ctx context.Context, input RescheduleInput) (Appointment, error)
    Cancel(ctx context.Context, input CancelInput) (Appointment, error)
}
```

Inputs contain no tenant ID. Each write contains a caller-generated opaque
`IdempotencyKey`. Reusing a key with the same request returns the first result;
reusing it with different data returns a conflict.

The internal PostgreSQL provider also supplies:

```go
type DayReader interface {
    Day(ctx context.Context, day time.Time) (Day, error)
}
```

`Day` contains concrete opening windows, appointments and display fields needed
by the planning UI. F04 receives a small adapter that maps appointments to its
already-frozen presentation DTO; calls and tasks remain empty until their own
features exist.

## Availability rules

- availability comes only from persisted, tenant-scoped opening windows;
- no stored opening means no available slots — never invent 08:00–17:00;
- requested duration is 15–480 minutes, in 15-minute increments;
- requested range must fit completely inside one opening;
- active overlapping appointments are `pending`, `confirmed`, `in_progress`;
- `done`, `cancelled`, `no_show` do not consume future capacity;
- booking and rescheduling lock the opening row and recheck capacity inside the
  same PostgreSQL transaction before confirming;
- provider/database outage returns an explicit unavailable error, never a fake
  slot.

Opening-window configuration belongs to onboarding/admin and has no F02B HTTP
route. F02A exposes a backend `ConfigureOpening` service operation for tests and
future onboarding; it is not callable by the voice provider.

## HTTP routes

All request bodies are `application/x-www-form-urlencoded`, so native HTML forms
work without JavaScript. F02B owns HTML rendering; F02A owns the business service
and mutation handler behavior.

| Method / route | Owner | Success |
|---|---|---|
| `GET /app/planning?day=YYYY-MM-DD` | Agent B | full HTML planning page |
| `GET /app/planning/day?day=YYYY-MM-DD&duration_minutes=60` | Agent B | HTML day fragment |
| `POST /app/appointments` | Agent A | `303` to `/app/planning?day=YYYY-MM-DD` |
| `POST /app/appointments/{id}/reschedule` | Agent A | `303` to the resulting day |
| `POST /app/appointments/{id}/cancel` | Agent A | `303` to the appointment day |

No mutation requires HTMX. F02B may progressively enhance it later without
changing the server contract. Redirect targets are constructed server-side;
there is no caller-controlled `return_to` open redirect.

### Create appointment form

| Field | Required | Rule |
|---|---:|---|
| `customer_id` | yes | UUID returned by F01 |
| `vehicle_id` | no | when present, vehicle must belong to customer and tenant |
| `service_label` | yes | 1–200 trimmed characters; validated garage data only |
| `start_at` | yes | RFC3339 timestamp with offset |
| `duration_minutes` | yes | 15–480, multiple of 15 |
| `note` | no | max 2000 trimmed characters |
| `idempotency_key` | yes | opaque 1–200 characters |

### Reschedule form

| Field | Required | Rule |
|---|---:|---|
| `start_at` | yes | RFC3339 timestamp with offset |
| `duration_minutes` | yes | 15–480, multiple of 15 |
| `idempotency_key` | yes | opaque 1–200 characters |

### Cancel form

| Field | Required | Rule |
|---|---:|---|
| `idempotency_key` | yes | opaque 1–200 characters |

## HTTP errors

| Condition | Status |
|---|---:|
| unauthenticated / tenant missing | F09/F12 session response (`401` or browser login redirect) |
| invalid form or transition | `422` |
| appointment/customer/vehicle not found in tenant | `404` |
| slot lost, capacity full, duplicate resource or idempotency mismatch | `409` |
| provider/database unavailable | `503` |

Error responses never expose SQL, DSN, another tenant's data or a guessed
alternative slot. F02B renders human-readable HTML from these outcomes.

### Amendment 2026-07-30 — F11 mutation error redirects

After F09 authenticates the request, a mutation handler no longer leaves the
garage on a raw `text/plain` error page. It redirects with `303 See Other` to:

```text
/app/planning?error=invalid
/app/planning?error=not_found
/app/planning?error=conflict
/app/planning?error=unavailable
```

The values are a closed server-generated set. F02B may map them to human-readable
HTML and must treat unknown values as `unavailable`. The error redirect omits
`day`: a failed cancel has no date in its form, and the backend must not invent
one when the requested resource/provider cannot be read. The planning page's
existing default-day behavior applies.

Authentication and cross-origin protection run before the mutation handler.
Cross-origin rejection remains `403`; missing/invalid sessions follow the F12
browser/API response matrix in `F09-authentication.md`. Success redirects,
routes and form fields are unchanged. This amendment resolves mini-task MT-01.

## F04 dashboard adapter

The adapter must satisfy the frozen seam exactly:

```go
Today(ctx context.Context, day time.Time) (views.Today, error)
```

It resolves tenant timezone from persisted tenant data, maps real appointments,
and leaves calls/tasks empty. It never imports view types into the appointment
domain. The fixture is deleted only when DI injects this real adapter.

## Acceptance

- two tenants may book the same wall-clock range independently;
- a tenant cannot read, reschedule or cancel another tenant's appointment;
- two concurrent requests for the last capacity slot yield one confirmation and
  one conflict;
- duplicate write delivery is idempotent;
- availability is rechecked in the write transaction;
- provider failure is explicit;
- dashboard renders real appointments after DI wiring.
