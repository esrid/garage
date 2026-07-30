# ElevenLabs MVP integration

Verified on 2026-07-30 against the current official ElevenLabs Agents webhook
tool, authentication and environment-variable documentation.

## Responsibility

ElevenLabs owns speech, turn-taking and the voice session. Go owns tenant
identity, authorization, customer data and every business rule. F03 exposes one
read-only webhook tool for customer lookup; it does not create agents, phone
numbers or outbound calls through the ElevenLabs API.

The frozen endpoint and response schema are in
[`contracts/F03-voice-customer-lookup.md`](contracts/F03-voice-customer-lookup.md).

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

## Costs and quotas

F03 makes no ElevenLabs API request and therefore adds no direct API call cost;
the webhook runs inside a paid voice conversation. Product metering must still
budget the PRD's prudent 0.10 EUR/minute envelope. Rate limits and voice plan
quotas remain those of the configured ElevenLabs workspace and are not guessed
or duplicated in code.

## Official references

- [Webhook tools](https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools)
- [Tools overview](https://elevenlabs.io/docs/eleven-agents/customization/tools)
- [Dynamic and secret variables](https://elevenlabs.io/docs/eleven-agents/customization/personalization/dynamic-variables)
- [Environment variables for tool URLs and headers](https://elevenlabs.io/docs/eleven-agents/integrate/environment-variables)
