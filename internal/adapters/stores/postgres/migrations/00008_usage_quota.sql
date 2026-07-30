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

-- Usage is read per tenant and per month, always over the same window.
CREATE INDEX conversations_tenant_started_idx
    ON conversations (tenant_id, started_at);

-- +goose Down
DROP INDEX IF EXISTS conversations_tenant_started_idx;
ALTER TABLE tenants DROP COLUMN monthly_minutes_quota;
