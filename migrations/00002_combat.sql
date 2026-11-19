-- +goose Up
ALTER TABLE characters
    ADD COLUMN str         int NOT NULL DEFAULT 0 CHECK (str >= 0),
    ADD COLUMN agi         int NOT NULL DEFAULT 0 CHECK (agi >= 0),
    ADD COLUMN vit         int NOT NULL DEFAULT 0 CHECK (vit >= 0),
    ADD COLUMN int         int NOT NULL DEFAULT 0 CHECK (int >= 0),
    ADD COLUMN free_points int NOT NULL DEFAULT 0 CHECK (free_points >= 0),
    -- NULL hp/mp means full.
    ADD COLUMN hp          int CHECK (hp >= 0),
    ADD COLUMN mp          int CHECK (mp >= 0);

-- +goose Down
ALTER TABLE characters
    DROP COLUMN str,
    DROP COLUMN agi,
    DROP COLUMN vit,
    DROP COLUMN int,
    DROP COLUMN free_points,
    DROP COLUMN hp,
    DROP COLUMN mp;
