-- +goose Up
ALTER TABLE characters
    ADD COLUMN xu bigint NOT NULL DEFAULT 0 CHECK (xu >= 0);

-- A character's bag (equipped = false, slot 0..bag_size-1) and worn items
-- (equipped = true, slot 0..9). Lines and hidden lines are
-- [{"stat": "hp", "value": 43}, ...].
CREATE TABLE character_items (
    character_id bigint   NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    equipped     boolean  NOT NULL,
    slot         smallint NOT NULL CHECK (slot >= 0),
    code         text     NOT NULL,
    count        int      NOT NULL CHECK (count >= 1),
    quality      smallint NOT NULL DEFAULT 0 CHECK (quality BETWEEN 0 AND 4),
    element      text     NOT NULL DEFAULT '' CHECK (element IN ('', 'viem', 'thuy', 'minh', 'nham', 'phong')),
    enhance      smallint NOT NULL DEFAULT 0 CHECK (enhance BETWEEN 0 AND 15),
    lines        jsonb    NOT NULL DEFAULT '[]',
    hidden       jsonb    NOT NULL DEFAULT '[]',
    PRIMARY KEY (character_id, equipped, slot)
);

-- +goose Down
DROP TABLE character_items;
ALTER TABLE characters DROP COLUMN xu;
