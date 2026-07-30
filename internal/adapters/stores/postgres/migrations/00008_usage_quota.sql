-- +goose Up
-- The economic rule of the PRD (§5) is "no unlimited plan": a workshop buys a
-- number of voice minutes per month, and is warned at 70, 85 and 100 % of them.
-- Nothing could tell a workshop where it stood, because the quota existed only
-- on the pricing page.
--
-- 750 is the entry plan of the PRD, and the safe default: a tenant provisioned
-- before billing exists is warned early rather than never.
ALTER TABLE tenants
    ADD COLUMN monthly_minutes_quota integer NOT NULL DEFAULT 750
        CONSTRAINT tenants_quota_positive CHECK (monthly_minutes_quota > 0);

-- Usage is read per tenant and per month, always over the same window, and
-- 00006 already indexes exactly that: conversations_tenant_started_idx on
-- (tenant_id, started_at DESC, id DESC). A btree serves a range scan in either
-- direction, so that index covers this read and creating a second one here made
-- every virgin database fail to migrate.

-- +goose Down
ALTER TABLE tenants DROP COLUMN monthly_minutes_quota;
