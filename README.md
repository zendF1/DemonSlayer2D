# DemonSlayer2D server

Go game server for DemonSlayer2D. The Godot client lives in a separate repository and generates its protocol code from [`proto/`](proto/) and reads maps from [`maps/`](maps/).

The protocol version is 7 (`gateway.ProtocolVersion`). A login with any other
version fails with `ERROR_CODE_VERSION_MISMATCH`, so bump it together with the client.

Design documents: [game design](docs/game-design.md), [Phase 0 server spec](docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md), [Phase 2 skills spec](docs/superpowers/specs/2026-11-20-giai-doan-2-ky-nang-design.md), [Phase 3a bag, drops and equipment spec](docs/superpowers/specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md), [Phase 3b enhancement spec](docs/superpowers/specs/2026-11-20-giai-doan-3b-cuong-hoa-design.md).

## Requirements

- Go 1.25+
- Docker (for Postgres)

Code generators (`buf`, `protoc-gen-go`) are Go tool dependencies; nothing else to install.

## Run

```bash
cp .env.example .env   # optional: local settings, git-ignored
make db-up   # Postgres 17 on localhost:5433 (databases ds and ds_test)
make run     # server on :8080 — WebSocket at /ws, health at /healthz
```

Configuration is read from environment variables. `make run` and `make test-db`
also load them from `.env` if it exists (copy [`.env.example`](.env.example)); the
server itself never reads the file, so in production set real environment variables.
A value in `.env` beats one exported in your shell; pass `make run VAR=value` to
override it for one run.

| Variable | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | (required; `make run` sets it) | Postgres connection string |
| `LISTEN_ADDR` | `:8080` | HTTP/WebSocket address |
| `MAPS_DIR` | `./maps` | Map files |
| `DATA_DIR` | `./data` | Game data (`monsters.json`, `skills.json`, `exp.json`, `items.json`) |
| `START_MAP` | `kumotori_1` | Map for new characters |
| `TICK_RATE` | `10` | Zone ticks per second, 1–100 |
| `MOVE_SPEED_TILES` | `4` | Tiles per second at speed factor 1, above 0 and at most 100 |
| `AOI_CELL_SIZE` | `10` | Visibility grid cell size in tiles |
| `ZONE_CAPACITY` | `50` | Players per zone |
| `SAVE_INTERVAL` | `60s` | Periodic position save |
| `REGISTER_PER_HOUR` | `5` | Registrations per IP per hour |
| `TRUSTED_PROXIES` | _(empty)_ | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is believed, e.g. `127.0.0.1,10.0.0.0/8` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Behind nginx or Caddy, set `TRUSTED_PROXIES` to the proxy's address, otherwise every
player shares the proxy's IP and its rate limits. Caddy sends `X-Forwarded-For` by
default; nginx needs `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.
Only list proxies you run: the header is read only when the connection comes from one.

If something else already uses port 8080, pass another address:
`make run LISTEN_ADDR=:8099` (then use `-url ws://localhost:8099/ws` below).

## Try it with the bot client

```bash
go run ./cmd/botclient register -u tanjiro -p password123
go run ./cmd/botclient create   -u tanjiro -p password123 -slot 1 -name Tanjiro -class viem
go run ./cmd/botclient chars    -u tanjiro -p password123
go run ./cmd/botclient play     -u tanjiro -p password123 -id 1 -dir 1,0 -for 10s
```

Fight mode registers a new account (unless `-u`/`-p` are given), creates a
character, walks out of the safe zone and kills N monsters, logging the level and
EXP after each kill. It exits 0 after N kills and 1 after `-timeout` (default 2m):

```bash
go run ./cmd/botclient -fight 3                      # against ws://localhost:8080/ws
go run ./cmd/botclient -fight 3 -url ws://localhost:8099/ws -timeout 1m
```

With no monster in view it walks towards `-seek` (default `22.5,10.5`, the nest
of region r1 on `kumotori_1`). Each run without `-u` counts against `REGISTER_PER_HOUR`.

Load test (lift the registration limit on the server first, e.g. `REGISTER_PER_HOUR=100000`):

```bash
go run ./cmd/botclient load -n 1000 -for 6m -ramp 150ms
```

Keep `-ramp` generous on a first run: every bot registers and logs in, and each
of those hashes a password with argon2id (64 MiB, ~76 ms), so a 1,000-bot burst
started in a few seconds exhausts the 10-second request deadline. `-ramp 150ms`
spreads the sign-ups over 150 s and leaves the rest of the run measuring the
game loop. Measured on an 8-core dev machine with 1,000 bots in world across 20
zones: tick p99 ≈ 9 ms, no dropped clients.

## Dev characters

`devchar` puts an account's first character (lowest slot) at a level, for
development only. It reads `DATABASE_URL` and takes no other settings:

```bash
DATABASE_URL='postgres://ds:ds@localhost:5433/ds?sslmode=disable' go run ./cmd/devchar -u tanjiro -level 90
```

It sets the level (1–90) and EXP to 0, refunds every spent potential point
(free points become 10 × (level − 1)), sets the skill points to (level − 1)
minus the levels of the learned skills, or drops every skill and sets them to
level − 1 when that is negative, and fills HP and MP. It prints the
character's name, level, free potential points and skill points.

The character must be offline. The server does not record who is online, so
`devchar` cannot check; a running server's next save of the character
overwrites the change.

## Dev items

`devitem` gives an account's first character (lowest slot) an item or Xu, for
development only. It reads `DATABASE_URL` and `DATA_DIR` (default `./data`):

```bash
export DATABASE_URL='postgres://ds:ds@localhost:5433/ds?sslmode=disable'
go run ./cmd/devitem -u tanjiro -code eq50_armor -quality 2   # a Vàng Áo Xích Cương
go run ./cmd/devitem -u tanjiro -code eq10_weapon_viem -enhance 9   # a +9 sword
go run ./cmd/devitem -u tanjiro -code stone_enhance -count 100
go run ./cmd/devitem -u tanjiro -code charm_protect -count 20      # protection charms
go run ./cmd/devitem -u tanjiro -xu 100000
```

An equipment code adds one piece into the first empty bag slot, with quality
`-quality` (0 Trắng, 1 Xanh Dương, 2 Vàng, 3 Cam, 4 Đỏ; default 0), enhance
level `-enhance` (0 to 15, default 0; equipment only) and lines and element
rolled at random. Stackable codes are the stones (`stone_enhance`,
`stone_refine`) and the protection charm `charm_protect`, which only the
general store will sell from phase 4. A stackable code adds `-count` (default 1), topping
up existing stacks first (999 per stack). `-xu` adds Xu and can be used alone
or with `-code`. An unknown code or a full bag is an error, and nothing is
written. It prints the character's name, the item added and its bag slots,
and the Xu it now has. Like `devchar`, it needs the character offline.

## Game data

Maps live in [`maps/`](maps/) (collision, spawn points, safe zones, rest points and
monster regions). The files in [`data/`](data/) are generated; edit the script,
never the JSON:

| File | Script | Contents |
| --- | --- | --- |
| [`data/monsters.json`](data/monsters.json) | [`monster_stats.py`](docs/balance/monster_stats.py) | Monster kinds and stats for levels 1–90, including base EXP (`HP × r(L)`) |
| [`data/skills.json`](data/skills.json) | [`skill_stats.py`](docs/balance/skill_stats.py) | The 25 class skills: shape, range, coefficients, MP, cooldowns, passives |
| [`data/exp.json`](data/exp.json) | [`exp_table.py`](docs/balance/exp_table.py) | EXP to the next level for levels 1–89 (also writes `docs/balance/exp-table.csv`) |
| [`data/items.json`](data/items.json) | [`item_stats.py`](docs/balance/item_stats.py) | Bag size, equipment slots, qualities, line rules, the 70 equipment pieces, stackables, drop tables per monster rank, the enhancement table (rates, stones, Xu) and the regen milestones |

```bash
python3 docs/balance/monster_stats.py --write-json
python3 docs/balance/skill_stats.py --write-json
python3 docs/balance/exp_table.py --write-json --write-csv
python3 docs/balance/item_stats.py --write-json
```

The level-up table depends on monster HP, so rerun `exp_table.py` whenever you
rerun `monster_stats.py`. Both `item_stats.py` and `monster_stats.py` (which
builds monsters from level 10 against the best +0 gear) read the gear stats from
`equipment_balance.py`, so after changing those, rerun `item_stats.py`,
`monster_stats.py` and `exp_table.py`. A Go test checks that each committed JSON
matches its script's output. The Godot client keeps copies of `skills.json` and
`items.json`, refreshed by its `tools/sync_server.sh`.

## Develop

```bash
make proto    # regenerate internal/pb after editing proto/
make vet
make test     # unit tests; database tests are skipped
make test-db  # all tests including Postgres and end-to-end (needs make db-up)
```
