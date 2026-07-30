-- +goose Up
-- Two openings covering the same instant make capacity ambiguous: booking locks
-- one opening row (ORDER BY starts_at, id LIMIT 1) and rechecks capacity against
-- that one alone, so a second overlapping window silently doubles what the
-- workshop accepts. Nothing prevented it while openings were only created by
-- tests; a form makes a double click enough to produce one.
--
-- The invariant belongs here rather than in a service check, which would race
-- between its SELECT and its INSERT.
--
-- btree_gist is a stock contrib extension: it lets a gist exclusion constraint
-- mix an equality on tenant_id with a range overlap.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE workshop_openings
    ADD CONSTRAINT workshop_openings_no_overlap
    EXCLUDE USING gist (
        tenant_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    );

-- +goose Down
ALTER TABLE workshop_openings DROP CONSTRAINT workshop_openings_no_overlap;
DROP EXTENSION IF EXISTS btree_gist;
