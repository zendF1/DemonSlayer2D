-- +goose Up
-- Whether the character has PK mode on; kept across logout, gates and zones.
ALTER TABLE characters ADD COLUMN pk_on boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE characters DROP COLUMN pk_on;
