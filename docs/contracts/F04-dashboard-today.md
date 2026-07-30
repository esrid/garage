# Contract — F04 Dashboard Today (`GET /app`)

**Frozen on** 2026-07-30 by Agent B (frontend). **Consumers:** `internal/adapters/handlers`.
**Producer:** Agent A, when F02 (mini-planning) lands.

Per AGENTS.md §15.3 rule 8, changing anything below requires an explicit mini-task
first — not a silent edit.

## Route

| Method / path | Response | Notes |
|---|---|---|
| `GET /app` | full HTML page | the dashboard of the day |
| `GET /app/today` | HTML fragment only (the three panels) | used by `hx-get` to refresh; identical markup to the page's panels |

`GET /app/today` exists so a refresh does not re-send the layout. It returns the
same fragment the full page embeds, so the page works with JavaScript disabled.

## Who owns what

Agent B owns the route registration, the handler, the templ views and the CSS.
Agent B does **not** read the database and does **not** decide business rules.

## The Go seam

Defined consumer-side in `internal/adapters/handlers/dashboard.go`:

```go
type TodayProvider interface {
	Today(ctx context.Context, day time.Time) (Today, error)
}
```

- **`tenant_id` is NOT a parameter.** It travels in `ctx`, put there by the tenant
  middleware Agent A owns (PRD §7.1). A frontend argument for it would break the
  invariant, so the seam makes it impossible to pass one.
- `day` is the calendar day to render, already resolved to the tenant's location
  timezone by the caller.
- An error is rendered as a degraded panel, never as a blank page.

## Amendment 2026-07-30 (Agent B, implementation)

Two changes made while implementing, both recorded before Agent A wrote anything
against this contract:

1. **The DTOs live in `internal/web/views/today.go`, not in the handlers package.**
   They are what the templ components take as input; putting them in `handlers`
   forced either an import cycle or a second identical struct plus mapping code.
   The seam is unchanged: `TodayProvider` still lives in `handlers` and returns
   `views.Today`.
2. **A row falls back to the phone number** when the customer name is unknown,
   instead of rendering "—". A row titled "—" while we hold the caller's number
   is useless at the desk. Nothing is invented: the number is shown only if the
   provider sent one, otherwise "Numéro inconnu".

Everything else below stands as frozen.

## Data shape

Declared in `internal/web/views/today.go`. Presentation DTOs: no domain types, so
the domain never depends on the UI.

```go
type Today struct {
	Day          time.Time
	Calls        []Call
	Appointments []Appointment
	Tasks        []Task
}

type Call struct {
	ID           string
	At           time.Time
	Duration     time.Duration
	CustomerName string // empty when the caller is unknown
	Phone        string
	Subject      string // qualified reason, one line
	Outcome      string // see allowed values
	Transferred  bool
}

type Appointment struct {
	ID           string
	Start, End   time.Time
	CustomerName string
	Vehicle      string // "Clio IV" or "" — display label, not a domain type
	Plate        string // may be empty: never invent one (PRD §7.1)
	Service      string
	Status       string // see allowed values
}

type Task struct {
	ID           string
	CreatedAt    time.Time
	Kind         string // see allowed values
	CustomerName string
	Phone        string
	Note         string
}
```

### Allowed values

Strings, not typed enums: the canonical enum belongs to Agent A's domain, and
duplicating it in the view layer would let the two drift apart.

| Field | Values |
|---|---|
| `Call.Outcome` | `booked`, `rescheduled`, `cancelled`, `callback`, `quote`, `info`, `transferred`, `dropped` |
| `Appointment.Status` | `pending`, `confirmed`, `in_progress`, `done`, `cancelled`, `no_show` |
| `Task.Kind` | `callback`, `quote` |

The view maps an unrecognised value to a neutral badge showing the raw string.
It never hides the row and never guesses a nicer label — an unknown status is a
visible integration bug, not something to paper over.

## Rules the view honours

- Empty slice → an explicit empty state ("Aucun appel aujourd'hui"), not a blank panel.
- Empty `Plate`, `Vehicle` or `CustomerName` → "—". Never fabricated (PRD §7.1).
- Nothing is computed from a price, a stock level or a status the provider did not send.

## Out of scope

Usage/quota metering (PRD §5) is deliberately absent. It gets its own panel when
the metering feature exists; adding a field for it now would be a guess at its shape.

## Until F02 lands

`internal/adapters/handlers/dashboard_fixture.go` holds a static `TodayProvider`
used to develop and screenshot the view. It is presentation fixture data, not a
service: no rules, no persistence. DI wires it today and Agent A replaces that
one line when the real provider exists. It is marked `TODO(F02)`.
