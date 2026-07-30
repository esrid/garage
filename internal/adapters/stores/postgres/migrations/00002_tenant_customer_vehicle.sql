-- +goose Up
CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL,
    timezone text NOT NULL DEFAULT 'America/Martinique',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tenants_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT tenants_timezone_not_blank CHECK (btrim(timezone) <> '')
);

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    first_name text NOT NULL DEFAULT '',
    last_name text NOT NULL DEFAULT '',
    phone_e164 text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT customers_phone_e164_format CHECK (phone_e164 ~ '^\+[0-9]{8,15}$'),
    CONSTRAINT customers_tenant_phone_key UNIQUE (tenant_id, phone_e164),
    CONSTRAINT customers_tenant_id_id_key UNIQUE (tenant_id, id)
);

CREATE TABLE vehicles (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    plate text NOT NULL DEFAULT '',
    plate_normalized text,
    make text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vehicles_plate_normalized_format CHECK (
        plate_normalized IS NULL OR plate_normalized ~ '^[A-Z0-9]{2,15}$'
    ),
    CONSTRAINT vehicles_tenant_plate_key UNIQUE (tenant_id, plate_normalized),
    CONSTRAINT vehicles_tenant_customer_fkey FOREIGN KEY (tenant_id, customer_id)
        REFERENCES customers(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX vehicles_tenant_customer_idx ON vehicles (tenant_id, customer_id);

-- +goose Down
DROP TABLE vehicles;
DROP TABLE customers;
DROP TABLE tenants;
