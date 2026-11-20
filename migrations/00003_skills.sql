-- +goose Up
ALTER TABLE characters
    ADD COLUMN skill_points int NOT NULL DEFAULT 0 CHECK (skill_points >= 0);

-- Each level above 1 has earned one skill point.
UPDATE characters SET skill_points = level - 1;

CREATE TABLE character_skills (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    skill_id     text   NOT NULL,
    level        int    NOT NULL CHECK (level BETWEEN 1 AND 10),
    PRIMARY KEY (character_id, skill_id)
);

-- +goose Down
DROP TABLE character_skills;
ALTER TABLE characters DROP COLUMN skill_points;
