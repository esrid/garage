-- +goose Up
CREATE TABLE follow_up_requests (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id uuid,
    kind text NOT NULL,
    phone_e164 text NOT NULL,
    details text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    conversation_id text NOT NULL,
    request_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT follow_up_requests_kind CHECK (kind IN ('callback', 'quote')),
    CONSTRAINT follow_up_requests_phone_format CHECK (phone_e164 ~ '^\+[0-9]{8,15}$'),
    CONSTRAINT follow_up_requests_details_length CHECK (
        btrim(details) <> '' AND char_length(details) <= 1000
    ),
    CONSTRAINT follow_up_requests_status CHECK (
        status IN ('pending', 'completed', 'cancelled')
    ),
    CONSTRAINT follow_up_requests_conversation_length CHECK (
        btrim(conversation_id) <> '' AND char_length(conversation_id) <= 512
    ),
    CONSTRAINT follow_up_requests_hash_format CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT follow_up_requests_tenant_customer_fkey FOREIGN KEY (tenant_id, customer_id)
        REFERENCES customers(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT follow_up_requests_conversation_kind_key UNIQUE (tenant_id, conversation_id, kind)
);

CREATE INDEX follow_up_requests_pending_idx
    ON follow_up_requests (tenant_id, created_at, id)
    WHERE status = 'pending';

-- +goose Down
DROP TABLE follow_up_requests;
