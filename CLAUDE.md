# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

The Go game server and the game-design docs for DemonSlayer2D, a 2D online RPG for mobile and PC. The Godot (GDScript) client lives in a **separate repository**. It generates its protocol code from `proto/` and reads maps from `maps/`, so changes to either are client-facing.

- Game design: `docs/game-design.md`, `docs/combat-balance.md`, `docs/equipment-stats-proposal.md`, with balance scripts and CSVs in `docs/balance/`.
- Server spec and plan per phase: `docs/superpowers/specs/` and `docs/superpowers/plans/`. Phase 0 ("Giai đoạn 0", the server foundation) is implemented.
- Design docs and specs are written in Vietnamese. Code, comments and commit messages are in English.
- In-game class names are Viêm, Thủy, Minh, Nham, Phong (ids `viem`, `thuy`, …).
- Scope is v1.0.0.0 core only: crafted gear and content for levels 60–90 are deferred.

## Commands

```bash
make db-up      # Postgres 17 in Docker on localhost:5433 (databases ds and ds_test)
make run        # server on :8080 — WebSocket at /ws, health at /healthz
make test       # go test -race ./... ; DB-backed tests skip themselves
make test-db    # every test incl. Postgres and e2e (needs make db-up)
make vet
make proto      # regenerate internal/pb after editing proto/ (buf is a Go tool dep)
staticcheck ./...
```

Run a single test, including DB-backed ones:

```bash
TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' \
  go test -race -run TestTakeoverChainWaitsForOldestSession ./internal/world/
```

### Configuration

- The server reads only environment variables, via `config.Load(os.Getenv)`. Variables, defaults and ranges are listed in `README.md` and in the spec.
- `make run` and `make test-db` also include an optional, git-ignored `.env`; `.env.example` is the template.
- A value in `.env` beats one exported in the shell. Override it for a single run with `make run VAR=value`.
- If `make db-up` fails or DB tests fail with SASL auth errors, check whether another container is already bound to port 5433.

To try the server by hand, use `cmd/botclient` (`register`, `create`, `chars`, `play`, `load`; see `README.md`). For load tests, raise `REGISTER_PER_HOUR` and keep `-ramp` generous: argon2id costs about 76 ms per login.

## Architecture

`internal/app` wires everything together: store → migrations (goose, embedded from `migrations/`) → maps → saver → world → auth → gateway → HTTP mux. `cmd/gameserver` is a thin main around it, and the e2e tests drive the same `app.App` through `httptest`.

**Request path.**
1. Client → WebSocket (`coder/websocket`) → `gateway`.
2. Each message is a protobuf `pb.Envelope`, defined in `proto/ds/v1` and generated into `internal/pb` (never edit `internal/pb` by hand). `Envelope.Seq` correlates requests and responses.
3. Each connection (`gateway/conn.go`) has a read loop and a write loop. The read goroutine owns the connection state machine: `connected → authed → inWorld`.
4. Outgoing messages go through a bounded `outbox`. A full outbox means a slow consumer, which gets kicked. The outbox keeps one extra slot so the final `Kicked` message is always delivered.

**World and zones.**
- `world` owns every map's zones and the `online` map (account → session).
- Each zone (`internal/zone`) is **one goroutine** with a command inbox and a fixed tick (`TICK_RATE`, default 10 Hz). All zone state is touched only from that goroutine; other packages talk to it through methods that enqueue commands (`Join`, `Move`, `Leave`).
- Zones fill up to `ZONE_CAPACITY`. `world.Enter` opens another zone of the same map when they are full, and the janitor closes empty zones other than zone 1.
- The zone itself sends `EnterWorldResp`, not the gateway.
- Visibility uses an AOI grid (3×3 cells of `AOI_CELL_SIZE` tiles).
- Movement is server-authoritative: a box collider (half-size 0.3) moved in sub-steps of at most 0.25 tiles against the map's collision grid (`internal/mapdata`).

**Persistence.**
- `store` uses pgx and holds accounts and characters.
- Zones never write to the DB on the tick. They hand `CharState` to `saver`, which coalesces saves per character and retries with backoff.
- `Leave()` returns a channel that yields the save result. The gateway waits on it before a connection counts as done (`conn.Done()`).

**Invariants that cost bugs to learn. Keep them when changing this code:**
- A new login for an account must never load the character before the previous session's save has landed. `world.Takeover` kicks the old session (`DUPLICATE_LOGIN`), waits on its `Done()`, and serialises takeovers per account so a third login cannot skip past a pending save.
- `zone.Join` honours its ctx only while waiting for inbox space. Once queued, it waits for the zone's answer, so a join is never half-applied.
- After the world's Start context is cancelled, `world.Enter` returns `ErrStopped` and must not open zones; `wg.Add` must not race `Wait`.
- Shutdown order (`app.Shutdown`, idempotent): the gateway kicks everyone (`SERVER_SHUTDOWN`) and waits, bounded by the caller's ctx, then the zones stop, then the saver flushes under its **own** `flushTimeout`. Saves must flush even when the gateway step timed out.
- Rate limits (`auth.Limiter`): use `Take`, which checks and records under one lock, plus `Refund`. Never split them into separate check and record calls. Login failures are keyed on `ip|username`, registrations on IP.
- The client IP comes from `gateway/clientip.go`. `X-Forwarded-For` is believed only when the peer is in `TRUSTED_PROXIES`, and is walked right to left.

## Testing conventions

- Tests run with `-race`, and timing-sensitive behaviour is tested with real goroutines and short timeouts, so run new concurrency tests a few times (`-count=5`).
- DB tests use `storetest.New(t)`. It skips the test when `TEST_DATABASE_URL` is unset; otherwise it creates a fresh migrated schema in `ds_test` and drops it when the test ends.
- `internal/client` is a Go implementation of the protocol, shared by `cmd/botclient` and `e2e/`. Prefer it to hand-rolled WebSocket code in tests.
- Fixes should come with a test that fails on the old code.

## Repo conventions

- Commit and push after each self-contained piece of work, with clear descriptive messages, and **no signature or `Co-Authored-By` trailer**.
- Keep `gofmt`, `go vet` and `staticcheck` clean.
- When changing config, update the variable tables in both `README.md` and the spec.
