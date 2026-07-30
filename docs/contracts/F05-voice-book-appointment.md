# Contract — F05 Voice Appointment Tools

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumers:** ElevenLabs webhook tools `find_appointment_slots` and
`book_appointment`.

Changing a route, header, JSON field, response field or tenant boundary below
requires an explicit mini-task in `WORKBOARD.md` before implementation.

## Shared tenant and authentication boundary

- routes: `POST /voice/tools/appointment-availability` and
  `POST /voice/tools/appointment-book`;
- `Content-Type: application/json`;
- `Authorization: Bearer <tenant-specific-secret>` uses the same server-side
  F03 credential map;
- the secret establishes tenant context before any scheduling call;
- `tenant_id` is forbidden in bodies, query strings and routes;
- bodies are capped at 16 KiB, unknown fields and multiple JSON values are
  rejected;
- every response uses `Cache-Control: no-store`.

## Find available slots

Request:

```json
{
  "day": "2030-01-02T12:00:00-04:00",
  "duration_minutes": 60
}
```

`day` is an RFC3339 instant with an explicit offset. The backend derives the
civil day in the authenticated tenant's persisted timezone; it never treats a
bare date as UTC. Duration is 15–480 minutes in 15-minute increments.

Success (`200`):

```json
{
  "slots": [
    {
      "start_at": "2030-01-02T08:00:00-04:00",
      "end_at": "2030-01-02T09:00:00-04:00"
    }
  ]
}
```

An empty `slots` array is a normal result. Slots come only from persisted,
tenant-scoped openings and appointments. Slot timestamps are serialized in the
authenticated tenant's persisted timezone, independently of the server's local
timezone.

## Book an appointment

Request:

```json
{
  "conversation_id": "{{system__conversation_id}}",
  "customer_id": "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1",
  "vehicle_id": "",
  "service_label": "Révision",
  "start_at": "2030-01-02T08:00:00-04:00",
  "duration_minutes": 60,
  "note": "Voyant signalé par le client"
}
```

- `conversation_id` is required, opaque, 1–512 trimmed UTF-8 characters, and
  must be populated by ElevenLabs from `system__conversation_id`, not generated
  by the LLM;
- `customer_id` comes from the authenticated F03 lookup result;
- `vehicle_id` is optional, but when present must belong to that customer and
  tenant;
- `service_label` is required, 1–200 trimmed characters and must describe the
  caller-confirmed request, not an invented mechanical diagnosis;
- `start_at` is RFC3339 with an explicit offset;
- duration is 15–480 minutes in 15-minute increments;
- `note` is optional and capped at 2,000 trimmed characters.

The HTTP client cannot supply the F02A idempotency key. The backend derives a
tenant-scoped deterministic key from `conversation_id` and the normalized
booking fields. An exact repeated tool call therefore returns the committed
first result, while a materially different booking in the same conversation
gets a different key. No automatic Webhook Tool retry policy is assumed.

Success is returned only after `SchedulingProvider.Book` succeeds and returns a
confirmed appointment (`200`):

```json
{
  "confirmed": true,
  "appointment": {
    "id": "019c09ea-bca7-7a5d-98b6-3f3b3ed79eaf",
    "start_at": "2030-01-02T08:00:00-04:00",
    "end_at": "2030-01-02T09:00:00-04:00",
    "status": "confirmed"
  }
}
```

The assistant must not say the appointment is confirmed before receiving this
exact successful result. Returned appointment timestamps are also serialized in
the persisted tenant timezone.

## Errors

Availability errors use `{"error":"..."}`. Booking errors additionally carry
`"confirmed":false`.

| Condition | Status | Stable error value |
|---|---:|---|
| missing/invalid bearer token | `401` | `authentication required` |
| wrong content type, malformed/extra/oversized JSON or invalid field | `422` | `invalid request` |
| customer/vehicle absent from authenticated tenant | `404` | `resource not found` |
| slot lost/capacity full, duplicate, idempotency mismatch | `409` | `appointment conflict` |
| database/provider failure or non-confirmed provider result | `503` | `service unavailable` |

Errors never contain SQL, credentials, tenant IDs, cross-tenant existence or a
guessed alternative slot. A `409` requires calling availability again before a
new booking attempt.

## ElevenLabs orchestration

1. Call `find_appointment_slots` after collecting the desired day and duration.
2. Offer only returned slots and have the caller explicitly choose one.
3. Call `book_appointment`, injecting `system__conversation_id` into
   `conversation_id` as a dynamic value and sending the tenant bearer token as a
   secret header.
4. Announce confirmation only when `confirmed=true`; otherwise explain that the
   booking could not be confirmed and follow the error rule above.

The application does not call the ElevenLabs tool-creation API in F05. Tool
configuration remains an explicit dashboard/deployment step.
