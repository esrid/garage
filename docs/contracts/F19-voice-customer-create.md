# Contract — F19 Voice tool: record a caller and their vehicle

**Frozen on:** 2026-07-30, before implementation.
**Consumers:** the ElevenLabs agent, during a call.
**Depends on:** F01 (customer/vehicle domain), F03 (tenant-scoped tool credential).

Changing a route, a field, a status or the tenant boundary below requires a
mini-task in `WORKBOARD.md` first.

## Why it exists

`customer-lookup` reads. `appointment-book` requires a `customer_id`. Nothing
wrote one, so an unknown caller could never be given an appointment by the
assistant, and the plate the PRD lists as P0 had no path into the database. This
is that path.

## HTTP route

`POST /voice/tools/customer-record`

Authenticated by the same per-tenant bearer token as the other tools (F03). The
tenant is resolved from the token on the server. No route, field or payload
accepts a tenant identifier.

Request, `application/json`, unknown fields refused, body bounded:

| Field | Required | Rule |
|---|---|---|
| `phone` | yes | the caller's number, normalised server-side to E.164 |
| `first_name` | no | up to 100 characters; the phone is the identity |
| `last_name` | no | up to 100 characters |
| `plate` | no | normalised server-side; when present a vehicle is recorded |
| `make` | no | up to 100 characters, ignored without a plate |
| `model` | no | up to 100 characters, ignored without a plate |

Response, `200`:

```json
{"customer_id":"<opaque>","created":true,"vehicle_id":"<opaque>","vehicle_created":true}
```

`vehicle_id` and `vehicle_created` are absent when no plate was sent.

### Amendment 2026-07-30, before implementation

`first_name` was specified as required and is not. A caller who will not give a
name still has to be bookable: the phone number is the identity here, F01 already
models a customer without a name, and every screen already titles such a row by
its number. Requiring a name would mean an assistant that cannot record the
caller cannot book them either, which is worse than a record with a number and no
name.

## Rules

- **Never overwrite validated garage data.** A phone already known returns the
  existing customer with `created:false`; the name in the payload is ignored. The
  same for a plate already recorded against that customer. A model that
  mishears a name must not rename a real customer.
- **A plate belonging to another customer of the tenant is a conflict**, not a
  transfer: `409`. Moving a vehicle between customers is a desk decision.
- **The assistant confirms before recording.** The tool records what it is given;
  the PRD requires the plate to be repeated and confirmed out loud first, which
  belongs to the agent prompt, not to this endpoint.
- Every write is tenant-scoped through the context, and a stored row that comes
  back with another tenant is answered as `503` with no data.

## Errors

| Condition | Status | Body |
|---|---:|---|
| missing/invalid token | `401` | `{"error":"authentication required"}` |
| unusable field, unparsable phone or plate | `422` | `{"error":"invalid request"}` |
| plate held by another customer | `409` | `{"error":"vehicle conflict"}` |
| database unavailable | `503` | `{"error":"service unavailable"}` |

Responses are `Cache-Control: no-store`. No error exposes SQL, a tenant, another
customer or the provider payload.

## Idempotency

Recording the same caller twice is the normal case: a call drops, the agent
retries, the same number comes back. Both the customer and the vehicle resolve by
their business key — phone, then plate — so a repeat returns the same identifiers
with `created:false`. No idempotency key is needed, and none is accepted.
