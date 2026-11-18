# DemonSlayer2D server

Go game server for DemonSlayer2D. The Godot client lives in a separate repository and generates its protocol code from [`proto/`](proto/) and reads maps from [`maps/`](maps/).

Design documents: [game design](docs/game-design.md), [Phase 0 server spec](docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md).

## Requirements

- Go 1.25+
- Docker (for Postgres)

Code generators (`buf`, `protoc-gen-go`) are Go tool dependencies; nothing else to install.

## Run

```bash
make db-up   # Postgres 17 on localhost:5433 (databases ds and ds_test)
make run     # server on :8080 — WebSocket at /ws, health at /healthz
```

Configuration is read from environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | (required; `make run` sets it) | Postgres connection string |
| `LISTEN_ADDR` | `:8080` | HTTP/WebSocket address |
| `MAPS_DIR` | `./maps` | Map files |
| `START_MAP` | `kumotori_1` | Map for new characters |
| `TICK_RATE` | `10` | Zone ticks per second |
| `MOVE_SPEED_TILES` | `4` | Tiles per second at speed factor 1 |
| `AOI_CELL_SIZE` | `10` | Visibility grid cell size in tiles |
| `ZONE_CAPACITY` | `50` | Players per zone |
| `SAVE_INTERVAL` | `60s` | Periodic position save |
| `REGISTER_PER_HOUR` | `5` | Registrations per IP per hour |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

If something else already uses port 8080, pass another address:
`LISTEN_ADDR=:8099 make run` (then use `-url ws://localhost:8099/ws` below).

## Try it with the bot client

```bash
go run ./cmd/botclient register -u tanjiro -p password123
go run ./cmd/botclient create   -u tanjiro -p password123 -slot 1 -name Tanjiro -class viem
go run ./cmd/botclient chars    -u tanjiro -p password123
go run ./cmd/botclient play     -u tanjiro -p password123 -id 1 -dir 1,0 -for 10s
```

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

## Develop

```bash
make proto    # regenerate internal/pb after editing proto/
make vet
make test     # unit tests; database tests are skipped
make test-db  # all tests including Postgres and end-to-end (needs make db-up)
```
