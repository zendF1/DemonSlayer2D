-- +goose Up
-- Main-line quest progress: state 1 is in progress, 2 is done. day is 0 for
-- the main line (the daily line of phase 6d uses it).
CREATE TABLE character_quests (
    character_id bigint   NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    quest_id     text     NOT NULL,
    state        smallint NOT NULL CHECK (state IN (1, 2)),
    progress     int      NOT NULL DEFAULT 0 CHECK (progress >= 0),
    day          int      NOT NULL DEFAULT 0,
    PRIMARY KEY (character_id, quest_id)
);

-- Skills a character could already learn before skill books existed are
-- learned without a book: those with a learn level up to this value.
ALTER TABLE characters
    ADD COLUMN skill_book_waiver smallint NOT NULL DEFAULT 0 CHECK (skill_book_waiver BETWEEN 0 AND 90);
UPDATE characters SET skill_book_waiver = level;

-- +goose Down
ALTER TABLE characters DROP COLUMN skill_book_waiver;
DROP TABLE character_quests;
