# DemonSlayer2D server

Go game server for DemonSlayer2D. The Godot client lives in a separate repository and generates its protocol code from [`proto/`](proto/) and reads maps from [`maps/`](maps/).

Design documents: [game design](docs/game-design.md), [Phase 0 server spec](docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md).

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
| `DATA_DIR` | `./data` | Game data (`monsters.json`) |
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

## Game data

Maps live in [`maps/`](maps/) (collision, spawn points, safe zones, rest points and
monster regions). Monster kinds and per-level stats live in
[`data/monsters.json`](data/monsters.json), which is generated: edit
[`docs/balance/monster_stats.py`](docs/balance/monster_stats.py), then run

```bash
python3 docs/balance/monster_stats.py --write-json
```

A Go test checks that the committed JSON matches the script's output.

## Develop

```bash
make proto    # regenerate internal/pb after editing proto/
make vet
make test     # unit tests; database tests are skipped
make test-db  # all tests including Postgres and end-to-end (needs make db-up)
```
