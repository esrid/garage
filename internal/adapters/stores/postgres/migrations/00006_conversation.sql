-- +goose Up
CREATE TABLE conversations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider text NOT NULL,
    agent_id text NOT NULL,
    provider_conversation_id text NOT NULL,
    provider_status text NOT NULL,
    provider_event_at timestamptz NOT NULL,
    started_at timestamptz NOT NULL,
    duration_seconds integer NOT NULL,
    cost_fiat_microusd bigint,
    transcript jsonb NOT NULL,
    summary text,
    provider_outcome text,
    analysis jsonb NOT NULL,
    metadata jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT conversations_provider CHECK (provider = 'elevenlabs'),
    CONSTRAINT conversations_agent_length CHECK (
        btrim(agent_id) <> '' AND char_length(agent_id) <= 512
    ),
    CONSTRAINT conversations_provider_id_length CHECK (
        btrim(provider_conversation_id) <> '' AND char_length(provider_conversation_id) <= 512
    ),
    CONSTRAINT conversations_status_length CHECK (
        btrim(provider_status) <> '' AND char_length(provider_status) <= 64
    ),
    CONSTRAINT conversations_duration CHECK (
        duration_seconds >= 0 AND duration_seconds <= 86400
    ),
    CONSTRAINT conversations_cost CHECK (
        cost_fiat_microusd IS NULL OR
        cost_fiat_microusd BETWEEN 0 AND 1000000000000
    ),
    CONSTRAINT conversations_summary_length CHECK (
        summary IS NULL OR char_length(summary) <= 10000
    ),
    CONSTRAINT conversations_outcome_length CHECK (
        provider_outcome IS NULL OR char_length(provider_outcome) <= 128
    ),
    CONSTRAINT conversations_tenant_provider_id_key UNIQUE (
        tenant_id, provider, provider_conversation_id
    )
);

CREATE INDEX conversations_tenant_started_idx
    ON conversations (tenant_id, started_at DESC, id DESC);

CREATE TABLE conversation_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider text NOT NULL,
    event_type text NOT NULL,
    provider_conversation_id text NOT NULL,
    provider_event_at timestamptz NOT NULL,
    payload_hash bytea NOT NULL,
    raw_payload jsonb NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT conversation_events_provider CHECK (provider = 'elevenlabs'),
    CONSTRAINT conversation_events_type CHECK (event_type = 'post_call_transcription'),
    CONSTRAINT conversation_events_provider_id_length CHECK (
        btrim(provider_conversation_id) <> '' AND char_length(provider_conversation_id) <= 512
    ),
    CONSTRAINT conversation_events_payload_hash_length CHECK (octet_length(payload_hash) = 32),
    CONSTRAINT conversation_events_dedupe_key UNIQUE (
        tenant_id, provider, event_type, provider_conversation_id, provider_event_at
    )
);

-- +goose Down
DROP TABLE conversation_events;
DROP TABLE conversations;
