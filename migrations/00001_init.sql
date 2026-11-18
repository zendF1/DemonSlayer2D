-- +goose Up
CREATE TABLE accounts (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      text        NOT NULL,
    password_hash text        NOT NULL,
    status        text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'banned')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    CONSTRAINT accounts_username_key UNIQUE (username)
);

CREATE TABLE characters (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    slot       smallint    NOT NULL CHECK (slot BETWEEN 1 AND 3),
    name       text        NOT NULL,
    name_key   text        NOT NULL,
    class      text        NOT NULL CHECK (class IN ('viem', 'thuy', 'minh', 'nham', 'phong')),
    level      int         NOT NULL DEFAULT 1 CHECK (level >= 1),
    exp        bigint      NOT NULL DEFAULT 0,
    map_id     text        NOT NULL,
    pos_x      real        NOT NULL,
    pos_y      real        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT characters_account_slot_key UNIQUE (account_id, slot),
    CONSTRAINT characters_name_key_key UNIQUE (name_key)
);

-- +goose Down
DROP TABLE characters;
DROP TABLE accounts;
