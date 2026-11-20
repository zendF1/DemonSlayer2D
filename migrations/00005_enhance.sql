-- +goose Up
-- The hidden enhancement pity per target level: empty means all zero,
-- otherwise exactly 15 entries where entry k (0-based) is the bonus, in
-- percentage points, for target level k + 1. The world resets bad arrays.
ALTER TABLE character_items
    ADD COLUMN pity smallint[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE character_items DROP COLUMN pity;
