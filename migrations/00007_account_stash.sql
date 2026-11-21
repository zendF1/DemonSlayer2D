-- +goose Up
-- Lượng, the premium money, and the size of the stash belong to the account.
ALTER TABLE accounts
    ADD COLUMN luong      bigint   NOT NULL DEFAULT 0 CHECK (luong >= 0),
    ADD COLUMN stash_size smallint NOT NULL DEFAULT 30 CHECK (stash_size BETWEEN 1 AND 200);

-- The account stash: the item columns of character_items, keyed by account
-- and slot (0..stash_size-1).
CREATE TABLE account_stash (
    account_id bigint   NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    slot       smallint NOT NULL CHECK (slot >= 0),
    code       text     NOT NULL,
    count      int      NOT NULL CHECK (count >= 1),
    quality    smallint NOT NULL DEFAULT 0 CHECK (quality BETWEEN 0 AND 4),
    element    text     NOT NULL DEFAULT '' CHECK (element IN ('', 'viem', 'thuy', 'minh', 'nham', 'phong')),
    enhance    smallint NOT NULL DEFAULT 0 CHECK (enhance BETWEEN 0 AND 15),
    lines      jsonb    NOT NULL DEFAULT '[]',
    hidden     jsonb    NOT NULL DEFAULT '[]',
    pity       smallint[] NOT NULL DEFAULT '{}',
    gems       text[]   NOT NULL DEFAULT '{}',
    PRIMARY KEY (account_id, slot)
);

-- +goose Down
DROP TABLE account_stash;
ALTER TABLE accounts DROP COLUMN stash_size, DROP COLUMN luong;
