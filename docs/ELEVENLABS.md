# ElevenLabs MVP integration

Verified on 2026-07-30 against the current official ElevenLabs Agents webhook
tool, authentication and environment-variable documentation.

## Responsibility

ElevenLabs owns speech, turn-taking and the voice session. Go owns tenant
identity, authorization, customer data and every business rule. F03 exposes one
read-only webhook tool for customer lookup. F05 adds tenant-scoped availability
and booking tools; neither feature creates agents, phone numbers or outbound
calls through the ElevenLabs API.

The frozen endpoint and response schema are in
[`contracts/F03-voice-customer-lookup.md`](contracts/F03-voice-customer-lookup.md).
The F05 scheduling contract is in
[`contracts/F05-voice-book-appointment.md`](contracts/F05-voice-book-appointment.md).

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

## Costs and quotas

F03 and F05 make no ElevenLabs management API request and therefore add no
separate API call cost; their tools run inside a paid voice conversation.
Product metering must still budget the PRD's prudent 0.10 EUR/minute envelope.
Rate limits and voice plan quotas remain those of the configured ElevenLabs
workspace and are not guessed or duplicated in code.

## Official references

- [Webhook tools](https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools)
- [Tools overview](https://elevenlabs.io/docs/eleven-agents/customization/tools)
- [Dynamic and secret variables](https://elevenlabs.io/docs/eleven-agents/customization/personalization/dynamic-variables)
- [Environment variables for tool URLs and headers](https://elevenlabs.io/docs/eleven-agents/integrate/environment-variables)
- [Webhooks and idempotency](https://elevenlabs.io/docs/eleven-api/resources/webhooks)
