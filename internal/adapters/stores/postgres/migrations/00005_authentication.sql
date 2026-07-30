-- +goose Up
CREATE TABLE staff_users (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    role text NOT NULL DEFAULT 'staff',
    password_hash text NOT NULL,
    disabled boolean NOT NULL DEFAULT false,
    failed_login_attempts integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT staff_users_email_key UNIQUE (email),
    CONSTRAINT staff_users_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT staff_users_email_normalized CHECK (email = lower(btrim(email)) AND btrim(email) <> ''),
    CONSTRAINT staff_users_role_valid CHECK (role IN ('owner', 'staff')),
    CONSTRAINT staff_users_password_hash_not_blank CHECK (btrim(password_hash) <> ''),
    CONSTRAINT staff_users_failed_login_attempts_valid CHECK (failed_login_attempts >= 0)
);

CREATE TABLE browser_sessions (
    token_hash bytea PRIMARY KEY,
    tenant_id uuid NOT NULL,
    staff_user_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT browser_sessions_token_hash_length CHECK (octet_length(token_hash) = 32),
    CONSTRAINT browser_sessions_staff_fkey FOREIGN KEY (tenant_id, staff_user_id)
        REFERENCES staff_users(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX browser_sessions_expires_at_idx ON browser_sessions (expires_at);
CREATE INDEX browser_sessions_staff_user_idx ON browser_sessions (tenant_id, staff_user_id);

-- +goose Down
DROP TABLE browser_sessions;
DROP TABLE staff_users;
