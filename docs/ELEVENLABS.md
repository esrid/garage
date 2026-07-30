# ElevenLabs MVP integration

Verified on 2026-07-30 against the current official ElevenLabs Agents webhook
tool, authentication and environment-variable documentation.

## Responsibility

ElevenLabs owns speech, turn-taking and the voice session. Go owns tenant
identity, authorization, customer data and every business rule. F03 exposes one
read-only webhook tool for customer lookup. F05 adds tenant-scoped availability
and booking tools. F08 adds a tenant-scoped local callback/quote request tool;
none of these features creates agents, phone numbers or outbound calls through
the ElevenLabs API.

The frozen endpoint and response schema are in
[`contracts/F03-voice-customer-lookup.md`](contracts/F03-voice-customer-lookup.md).
The F05 scheduling contract is in
[`contracts/F05-voice-book-appointment.md`](contracts/F05-voice-book-appointment.md).
The F08 follow-up contract is in
[`contracts/F08-follow-up-request.md`](contracts/F08-follow-up-request.md).
The F14 post-call contract is in
[`contracts/F14-post-call.md`](contracts/F14-post-call.md).

## Authentication

ElevenLabs webhook tools support secret custom headers. Configure
`Authorization: Bearer <token>` as a secret header, never as an LLM prompt
parameter. The backend hashes the presented token, resolves exactly one tenant,
and only then calls the tenant-scoped F01 service.

Deployment supplies credentials through `VOICE_TOOL_TOKENS`:

```text
<tenant-uuid>:<high-entropy-token>,<tenant-uuid>:<different-token>
```

Tokens are 32–200 visible non-space ASCII characters and must not contain comma or
colon. Use a distinct token for every tenant. An empty value is allowed before
voice onboarding; the endpoint then authenticates no caller.

For the MVP onboarding model, each garage's ElevenLabs agent/tool receives only
that garage's token. If a shared agent is introduced later, use an ElevenLabs
secret dynamic variable in the header; never expose the credential or tenant ID
to the LLM prompt.

## Endpoint and failures

`POST /voice/tools/customer-lookup` accepts a bounded JSON body containing only
`phone`. Unknown customers are a normal `200 {"found":false}` result. Invalid
input is `422`, invalid credentials are `401`, and database unavailability is
`503`. No error response contains provider or tenant details.

F05 exposes `POST /voice/tools/appointment-availability` and
`POST /voice/tools/appointment-book`. The availability tool only returns stored
slots. The booking tool injects ElevenLabs' documented
`system__conversation_id` dynamic variable and the backend combines it with the
normalized booking fields to derive the F02A idempotency key. This makes an
exact repeated tool call stable without allowing the LLM to choose a key. A booking
is reported as confirmed only after the PostgreSQL transaction commits.

The official retry schedule applies to event webhooks such as
`post_call_transcription`, not to Webhook Tool calls. F05 therefore does not
assume an automatic tool retry. It is still idempotent because an LLM can repeat
a call and a timeout can leave the caller uncertain about the result. The
conversation ID is conversation-scoped, so F05 combines it with the normalized
operation instead of treating the whole conversation as one write.

F08 exposes `POST /voice/tools/follow-up-request`. It uses the same secret
header and documented `system__conversation_id`, but persists the request
locally instead of invoking an ElevenLabs management endpoint. Exact repeated
calls return the first request; changed content for the same conversation and
request kind conflicts explicitly.

## Post-call history and metering

F14 receives `post_call_transcription` at
`POST /webhooks/elevenlabs/post-call`. Configure the workspace webhook for
`events: ["transcript"]`, JSON transcript format and no audio. Its HMAC secret
is supplied as `ELEVENLABS_WEBHOOK_SECRET`; `ELEVENLABS_AGENT_TENANTS` maps each
server-configured ElevenLabs agent ID to a tenant UUID. Neither tenant IDs nor
credentials come from dynamic variables or the LLM.

Signature verification is performed over the bounded raw body before JSON
parsing. The official Python SDK source and tests at commit
`061e28b4878caf7aeaa28baaceeeae8cf02c8e4d` were inspected on 2026-07-30 because
the prose documentation does not spell out the digest: it uses
HMAC-SHA-256 over `<timestamp>.<raw-body>`, a lowercase hexadecimal `v0`
signature and a 30-minute past-timestamp tolerance. The implementation uses
the Go standard library and does not import either provider SDK.

ElevenLabs retries are disabled by default and, when enabled, use identical
payloads without a retry marker. F14 therefore deduplicates durably by tenant,
conversation and event timestamp plus a hash of the exact body. It returns 200
only after the event and normalized conversation are committed. Provider
`cost_fiat` is stored as micro-USD using exact decimal half-up rounding; the
older `cost` example has no documented unit and is never treated as currency.

## Costs and quotas

F03, F05 and F08 make no ElevenLabs management API request and therefore add no
separate API call cost; their tools run inside a paid voice conversation.
Product metering must still budget the PRD's prudent 0.10 EUR/minute envelope.
Rate limits and voice plan quotas remain those of the configured ElevenLabs
workspace and are not guessed or duplicated in code.

## Official references

## F19 — enregistrer l'appelant et son véhicule

`POST /voice/tools/customer-record`, contrat gelé dans
`docs/contracts/F19-voice-customer-create.md`. Même credential que les autres
outils : un Bearer par garage, tenant résolu côté serveur.

L'agent l'appelle quand `customer-lookup` a répondu `found:false`, après avoir
fait confirmer la plaque à voix haute — le tool enregistre ce qu'on lui donne, la
confirmation est une règle de prompt, pas d'endpoint.

Il renvoie `customer_id` (et `vehicle_id` si une plaque était fournie), ce que
`appointment-book` attend. Rappeler le même numéro ou la même plaque ne crée
jamais de doublon : `created:false`. Un nom différent sur un numéro connu est
ignoré, jamais écrit par-dessus. Une plaque déjà rattachée à un autre client du
garage répond `409` : déplacer un véhicule est une décision du comptoir.

- [Webhook tools](https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools)
- [Tools overview](https://elevenlabs.io/docs/eleven-agents/customization/tools)
- [Dynamic and secret variables](https://elevenlabs.io/docs/eleven-agents/customization/personalization/dynamic-variables)
- [Environment variables for tool URLs and headers](https://elevenlabs.io/docs/eleven-agents/integrate/environment-variables)
- [Webhooks and idempotency](https://elevenlabs.io/docs/eleven-api/resources/webhooks)
- [Post-call webhook payloads](https://elevenlabs.io/docs/eleven-agents/workflows/post-call-webhooks)
- [Official Python HMAC implementation (source inspected)](https://github.com/elevenlabs/elevenlabs-python/blob/061e28b4878caf7aeaa28baaceeeae8cf02c8e4d/src/elevenlabs/webhooks_custom.py)
- [Conversation response and `cost_fiat`](https://elevenlabs.io/docs/api-reference/conversations/get)
- [July 2026 `cost_fiat` changelog](https://elevenlabs.io/docs/changelog/2026/7/6)
