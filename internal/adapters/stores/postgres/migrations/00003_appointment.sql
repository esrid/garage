-- +goose Up
ALTER TABLE vehicles
    ADD CONSTRAINT vehicles_tenant_customer_id_key UNIQUE (tenant_id, customer_id, id);

CREATE TABLE workshop_openings (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    capacity integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workshop_openings_range CHECK (ends_at > starts_at),
    CONSTRAINT workshop_openings_capacity CHECK (capacity BETWEEN 1 AND 50),
    CONSTRAINT workshop_openings_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT workshop_openings_unique_window UNIQUE (tenant_id, starts_at, ends_at)
);

CREATE TABLE appointments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id uuid NOT NULL,
    vehicle_id uuid,
    opening_id uuid NOT NULL,
    service_label text NOT NULL,
    note text NOT NULL DEFAULT '',
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT appointments_range CHECK (ends_at > starts_at),
    CONSTRAINT appointments_service_not_blank CHECK (btrim(service_label) <> ''),
    CONSTRAINT appointments_status CHECK (
        status IN ('pending', 'confirmed', 'in_progress', 'done', 'cancelled', 'no_show')
    ),
    CONSTRAINT appointments_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT appointments_tenant_customer_fkey FOREIGN KEY (tenant_id, customer_id)
        REFERENCES customers(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT appointments_tenant_customer_vehicle_fkey FOREIGN KEY (tenant_id, customer_id, vehicle_id)
        REFERENCES vehicles(tenant_id, customer_id, id) ON DELETE RESTRICT,
    CONSTRAINT appointments_tenant_opening_fkey FOREIGN KEY (tenant_id, opening_id)
        REFERENCES workshop_openings(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX appointments_tenant_day_idx ON appointments (tenant_id, starts_at);
CREATE INDEX appointments_opening_overlap_idx ON appointments (opening_id, starts_at, ends_at);

CREATE TABLE appointment_commands (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL,
    operation text NOT NULL,
    request_hash text NOT NULL,
    appointment_id uuid,
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, idempotency_key),
    CONSTRAINT appointment_commands_operation CHECK (operation IN ('book', 'reschedule', 'cancel')),
    CONSTRAINT appointment_commands_appointment_fkey FOREIGN KEY (tenant_id, appointment_id)
        REFERENCES appointments(tenant_id, id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE appointment_commands;
DROP TABLE appointments;
DROP TABLE workshop_openings;
ALTER TABLE vehicles DROP CONSTRAINT vehicles_tenant_customer_id_key;
