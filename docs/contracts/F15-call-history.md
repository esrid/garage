# Contract — F15 Call history and summaries (UI)

**Frozen on:** 2026-07-30 by Agent B (frontend), before implementation.
**Depends on:** F14 (post-call persistence), F09 (session), F04 (shared DTOs).
**Needs from Agent A:** one adapter over the `conversations` read model (MT-09).

Changing a route, a DTO field, a Go signature or the tenant boundary below
requires a mini-task in `WORKBOARD.md` first. This is the same arrangement as
F04: the frontend freezes the seam, the backend writes the adapter.

## Tenant boundary

`tenant_id` appears in no route, query string or seam argument. It travels in
`context.Context`, put there by the F09 session middleware, and the adapter
resolves it with `tenant.IDFromContext`. Both routes live under `/app`, so they
are already behind the session and the F12 browser matrix.

## HTTP routes

| Method / route | Owner | Success |
|---|---|---|
| `GET /app/calls?day=YYYY-MM-DD` | Agent B | full HTML page: the day's calls |
| `GET /app/calls/{id}` | Agent B | full HTML page: one call, summary and transcript |

`day` is a civil date resolved in the tenant timezone, exactly as F02B does it:
the handler asks the reader for the current day first, then parses the parameter
inside the location it reports. An unreadable date falls back to the current day
with a visible notice. An unknown call id renders a "call not found" page with
`404`, and never another tenant's call.

No mutation, no form, no htmx requirement: this is a read-only history.

## Go seam

Both live in `internal/adapters/handlers`. The DTOs are presentation types in
`internal/web/views`, so the conversation domain never imports a view:

```go
type CallHistoryReader interface {
    // Calls lists the calls of one day, most recent first.
    Calls(ctx context.Context, day time.Time) (views.CallHistory, error)
    // Call returns one call with its transcript. It reports a NotFoundError when
    // the id does not belong to the tenant in context.
    Call(ctx context.Context, id string) (views.CallDetail, error)
}
```

```go
type CallHistory struct {
    Day      time.Time // midnight in the tenant timezone
    Timezone string    // IANA name
    Calls    []CallSummary
}

type CallSummary struct {
    ID           string        // opaque, used in /app/calls/{id}
    At           time.Time     // in the tenant location
    Duration     time.Duration // from metadata.call_duration_secs
    CustomerName string        // empty when the caller is unknown
    Phone        string        // empty when the provider gave none
    Outcome      string        // provider outcome, free string, may be empty
    Status       string        // provider status, free string
    Summary      string        // provider summary, may be empty
}

type CallDetail struct {
    CallSummary
    Turns []CallTurn
}

type CallTurn struct {
    Role string        // "agent" or "user"; anything else renders raw
    Text string
    At   time.Duration // offset from the start of the call; zero when unknown
}
```

The adapter maps the provider transcript JSON into `[]CallTurn`. Provider shape
stops at the adapter: no view ever parses provider JSON.

## Rules the views honour

- Every instant is converted into the tenant location before rendering.
- Empty `Summary`, `Outcome`, `Phone` or `CustomerName` render as an explicit
  gap, never as an invented value (PRD §7.1).
- `Status` and `Outcome` are provider strings, not garage truth: an unknown value
  keeps its raw text under a neutral badge, like F04 already does.
- A provider summary is labelled as coming from the assistant. It is historical
  provider information, not verified business truth (F14 says so explicitly).
- No cost is shown. `metadata.cost_fiat` is our supplier cost, not the garage's
  price; putting it on a garage screen would invite reading it as a charge.
  Usage and cost reporting gets its own feature (PRD §5).
- Transcripts can be long: the page renders them in document order with no
  pagination, and the browser scrolls.

## Errors

| Condition | Response |
|---|---|
| no session | handled upstream by F09/F12, never by these handlers |
| unreadable `day` | `200` with the current day and a notice |
| unknown or foreign call id | `404` with a human-readable page |
| reader unavailable | `200` with a degraded page and a notice, failure logged |

The degraded behaviour matches F02B: an operational read failure explains itself
instead of blanking the screen, and the reason goes to the structured log.

## Delivery order

The views, handlers and tests ship first, unwired, against a test stub. The
route registration and the DI line land with Agent A's adapter (MT-09), so no
production route ever serves fixture data.
