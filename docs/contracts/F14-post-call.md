# F14 — ElevenLabs post-call contract

Frozen on 2026-07-30 before implementation. Any incompatible change requires a
mini-task in `WORKBOARD.md`.

## HTTP boundary

`POST /webhooks/elevenlabs/post-call`

The endpoint accepts only the ElevenLabs `post_call_transcription` event. It
does not accept a tenant identifier in the path, query string or payload. The
server resolves `data.agent_id` through the deployment-owned
`ELEVENLABS_AGENT_TENANTS` mapping after authenticating the event.

The raw request body is limited to 2 MiB. The handler verifies
`ElevenLabs-Signature` before parsing JSON. Its current official format is
`t=<unix-seconds>,v0=<lowercase-hex>` and the signed message is
`<timestamp>.<raw-body>`. Verification uses HMAC-SHA-256 and rejects a timestamp
older than 30 minutes, matching official Python SDK source commit
`061e28b4878caf7aeaa28baaceeeae8cf02c8e4d` checked on 2026-07-30. The exact
raw bytes are signed; decoded or re-encoded JSON is never used for
authentication.

Required provider fields:

- top-level `type` and numeric `event_timestamp`;
- `data.agent_id`, `data.conversation_id`, `data.status`;
- `data.transcript`, `data.metadata`, and `data.analysis` JSON values;
- `metadata.start_time_unix_secs` and `metadata.call_duration_secs`.

Known additive ElevenLabs fields are ignored at the typed boundary and remain
available in the stored raw payload. The normalized cost is only
`metadata.cost_fiat`, stored as integer micro-USD and rounded to the nearest
micro-USD with exact decimal half-up rounding. The legacy `metadata.cost` has
no documented unit and is retained only inside metadata/raw JSON; it is never
interpreted as dollars or euros.

Responses are JSON with `Cache-Control: no-store`:

- `200 {"status":"received"}` only after the event and normalized conversation
  are durable; an exact retry also returns 200;
- `400 {"error":"invalid event"}` for malformed/unsupported events or an
  unknown `agent_id` mapping;
- `401 {"error":"invalid signature"}` for a missing, stale or invalid
  signature;
- `409 {"error":"event conflict"}` if the same tenant, conversation and event
  timestamp are reused with different raw content;
- `413 {"error":"event too large"}` above the body limit;
- `415 {"error":"unsupported media type"}` outside `application/json`;
- `503 {"error":"service unavailable"}` for unavailable configuration or
  persistence.

Only 5xx/429/408 deliveries are retryable according to ElevenLabs. A signed
event for an unknown agent mapping is rejected as invalid with 400; a deployment
where F14 is not configured yet returns 503, as does a transient persistence
failure. No provider detail, tenant ID, payload or secret appears in an error
response.

## Persistence and idempotency

The service obtains `tenant_id` from `context.Context`; it is not a free method
argument. A PostgreSQL transaction first records the immutable provider event,
then upserts the normalized conversation.

The event key is `(tenant_id, provider, event_type,
provider_conversation_id, provider_event_at)`. The row stores a SHA-256 hash of
the exact raw payload. An exact duplicate returns the existing conversation;
different content for the same key is a conflict and never overwrites history.
An older distinct event may be retained but cannot overwrite a newer normalized
conversation snapshot.

Normalized fields are tenant ID, provider/agent/conversation identifiers,
provider status/event time, start time, duration seconds, optional fiat cost in
micro-USD, transcript JSON, optional provider-generated summary and outcome,
analysis JSON and metadata JSON. Provider analysis is historical information,
not verified garage business truth.

## Deployment configuration

- `ELEVENLABS_WEBHOOK_SECRET`: shared HMAC secret returned when the workspace
  webhook is created;
- `ELEVENLABS_AGENT_TENANTS`:
  `<agent-id>:<tenant-uuid>,<agent-id>:<tenant-uuid>`.

Both variables may be empty before post-call onboarding, in which case the
endpoint is present but returns 503. Supplying only one, a duplicate agent or an
invalid tenant UUID fails application startup. Multiple agents may map to one
tenant.

Configure the ElevenLabs workspace webhook for transcript JSON only, with audio
disabled. Enable retries unless a compliance mode forbids them. The backend
does not call an ElevenLabs management endpoint and adds no SDK dependency.
