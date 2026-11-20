-- +goose Up
-- The punched holes of an item: one entry per hole, a gem code or '' for an
-- empty hole, so the length is the hole count. The world empties unknown or
-- too-high gems and cuts arrays longer than the hole maximum.
ALTER TABLE character_items
    ADD COLUMN gems text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE character_items DROP COLUMN gems;
