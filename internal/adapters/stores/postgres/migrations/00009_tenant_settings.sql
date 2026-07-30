-- +goose Up
-- Where the assistant hands a call to a human. ElevenLabs initiates the transfer
-- itself, from its agent configuration; what belongs here is the number the
-- workshop wants it sent to, so the founder configures one place and the app can
-- state what it is instead of the marketing page promising it alone.
ALTER TABLE tenants
    ADD COLUMN transfer_phone_e164 text
        CONSTRAINT tenants_transfer_phone_format
        CHECK (transfer_phone_e164 IS NULL OR transfer_phone_e164 ~ '^\+[0-9]{8,15}$');

-- +goose Down
ALTER TABLE tenants DROP COLUMN transfer_phone_e164;
