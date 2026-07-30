# Contract — F03 Voice Customer Lookup

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumer:** ElevenLabs webhook tool `lookup_customer_by_phone`.

Changing the route, headers, JSON fields or tenant boundary below requires an
explicit mini-task in `WORKBOARD.md` before implementation.

## Tenant and authentication boundary

- route: `POST /voice/tools/customer-lookup`;
- `Content-Type: application/json`;
- `Authorization: Bearer <tenant-specific-secret>` is configured as an
  ElevenLabs secret custom header, never as an LLM-generated parameter;
- the backend maps the secret to one tenant and puts that tenant ID in the
  request context before calling the F01 customer service;
- `tenant_id` is forbidden in the JSON body, query string and route;
- tokens are deployment secrets, never logged or returned.

The MVP credential map is supplied by deployment configuration, one distinct
high-entropy ASCII token per tenant. Rotation requires replacing the environment
value and restarting the application. Moving credentials to PostgreSQL later is
a separate authentication mini-task and must preserve this HTTP contract.

## Request

Body, capped at 8 KiB with unknown fields rejected:

```json
{"phone":"+596696123456"}
```

`phone` is the only allowed field. F01 applies its frozen E.164-like
normalization and validation. The assistant may ask the caller for a number but
must never invent one.

## Success responses

Known customer (`200`):

```json
{
  "found": true,
  "customer": {
    "id": "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1",
    "first_name": "Ana"
  }
}
```

Unknown phone (`200`):

```json
{"found":false}
```

The response deliberately omits last name, stored phone, address and vehicles.
The voice agent may confirm the first name, but it must not reveal extra account
data to an unverified caller. `customer.id` is an opaque reference; every later
operation must still enforce the current tenant from server context.

## Errors

| Condition | Status | Body |
|---|---:|---|
| missing/invalid bearer token | `401` | `{"error":"authentication required"}` |
| wrong content type, malformed/extra/oversized JSON, invalid phone | `422` | `{"error":"invalid request"}` |
| customer store unavailable | `503` | `{"error":"service unavailable"}` |

Responses never expose SQL, tokens, tenant IDs or whether the same phone exists
in another tenant. They use `Cache-Control: no-store`.

## ElevenLabs tool configuration

- type: Webhook;
- name: `lookup_customer_by_phone`;
- method: `POST`;
- body parameter: string `phone`, required, described as the caller's confirmed
  phone number;
- secret header: `Authorization` with value `Bearer <tenant token>`;
- orchestration: use at most after obtaining/confirming the caller number; on
  `found=false`, continue qualification without claiming an identity; on error,
  do not guess customer data.

The application does not call the ElevenLabs tool-creation API in F03. Tool
configuration is an explicit dashboard/deployment step, avoiding an API surface
and credential scope not needed for the MVP.
