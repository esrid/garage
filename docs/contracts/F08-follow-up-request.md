# Contract — F08 Voice Follow-up Request

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumer:** ElevenLabs webhook tool `create_follow_up_request`.

Changing the route, headers, JSON fields, response fields, statuses or tenant
boundary below requires an explicit mini-task in `WORKBOARD.md` before
implementation.

## Purpose

Record a concrete task for the garage when the voice agent must not conclude
alone, notably after a scheduling/provider failure or when the caller asks for
a quote. F08 does not invent a price, diagnosis, callback time or customer
identity and does not create a workflow engine.

## Tenant and authentication boundary

- route: `POST /voice/tools/follow-up-request`;
- `Content-Type: application/json`;
- `Authorization: Bearer <tenant-specific-secret>` uses the frozen F03
  credential map;
- the secret establishes tenant context before any read or write;
- `tenant_id`, `customer_id`, `status` and an idempotency key are forbidden in
  the body, query string and route;
- body is capped at 16 KiB, with unknown fields and multiple JSON values
  rejected;
- every response uses `Cache-Control: no-store`.

## Request

```json
{
  "conversation_id": "{{system__conversation_id}}",
  "kind": "callback",
  "phone": "+596696123456",
  "details": "Le client souhaite être rappelé pour un devis de révision."
}
```

Rules:

- `conversation_id` is required, opaque, 1–512 trimmed UTF-8 characters and is
  injected from ElevenLabs `system__conversation_id`, never authored by the
  LLM;
- `kind` is exactly `callback` or `quote`;
- `phone` is the caller-confirmed international number and follows the frozen
  F01 normalization/validation rule;
- `details` contains 1–1,000 trimmed UTF-8 characters describing only the
  caller-confirmed request; it must contain no invented price or certain
  mechanical diagnosis.

The backend looks up the normalized phone inside the authenticated tenant. A
match is linked as `customer_id`; no match remains an anonymous request with
the phone preserved. A match in another tenant is indistinguishable from no
match. The voice caller cannot choose the customer link.

## Idempotence

At most one request of each `kind` may be recorded per tenant and conversation.
The database key is `(tenant_id, conversation_id, kind)`. The backend also
stores a SHA-256 hash of the normalized phone and details:

- an exact repeated call returns the first committed request;
- the same tenant, conversation and kind with different normalized content
  returns `409`;
- `callback` and `quote` may each exist once in the same conversation.

This protects against repeated tool invocation and ambiguous timeouts without
accepting an LLM-controlled idempotency key.

## Success

Only a committed row returns `200`:

```json
{
  "recorded": true,
  "request": {
    "id": "019c09ea-bca7-7a5d-98b6-3f3b3ed79eaf",
    "kind": "callback",
    "status": "pending"
  }
}
```

The response deliberately omits phone, details, customer ID, tenant ID and
conversation ID. The assistant may say the request was recorded only after
`recorded=true`.

## Errors

All failures return `{"recorded":false,"error":"..."}`.

| Condition | Status | Stable error value |
|---|---:|---|
| missing/invalid bearer token | `401` | `authentication required` |
| wrong content type, malformed/extra/oversized JSON or invalid field | `422` | `invalid request` |
| same conversation/kind reused with different content | `409` | `request conflict` |
| database unavailable or invalid committed result | `503` | `service unavailable` |

Errors never contain SQL, credentials, tenant IDs, phone numbers, customer
existence or another tenant's data. On any failure the assistant explains that
the request could not be recorded and offers the configured human fallback; it
must not claim success.

## Persistence

`follow_up_requests` stores:

- server-generated UUIDv7 `id` and server-derived `tenant_id`;
- nullable tenant-scoped `customer_id` resolved by normalized phone;
- `kind`, normalized `phone_e164`, trimmed `details` and `status=pending`;
- `conversation_id`, normalized request hash, `created_at`, `updated_at`.

Canonical statuses are `pending`, `completed`, `cancelled`. F08 only creates
`pending`; completing/cancelling and dashboard rendering require separate
contracts. There is no external provider interface for this local PostgreSQL
operation.

## ElevenLabs configuration

- type: Webhook;
- name: `create_follow_up_request`;
- method: `POST`;
- secret header: `Authorization: Bearer <tenant token>`;
- dynamic body value: `conversation_id={{system__conversation_id}}`;
- LLM body parameters: `kind`, `phone`, `details` under the rules above.

The application does not call the ElevenLabs tool-management API. Configuration
remains an explicit dashboard/deployment step.
