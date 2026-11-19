# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Go game server for DemonSlayer2D, a 2D online RPG. The Godot client is in a separate repo and consumes `proto/` and `maps/`, so treat changes to those as client-facing. Design docs (`docs/`) are in Vietnamese; code, comments and commit messages are in English.

## Commands

```bash
make db-up     # Postgres on localhost:5433
make run       # server on :8080 (/ws, /healthz)
make test      # unit tests; DB tests skip
make test-db   # all tests incl. DB and e2e
make proto     # regenerate internal/pb (never edit it by hand)
staticcheck ./...

# single test
TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race -run TestName ./internal/world/
```

Config comes only from environment variables (see `README.md`). The Makefile also loads `.env`. To override for one run: `make run VAR=value`.

## Architecture

`internal/app` wires the pieces: store (pgx, goose migrations) → saver → world → auth → gateway (WebSocket, protobuf `Envelope`).

- **Zones:** each zone is one goroutine with its own tick, and only that goroutine touches its state. Use `Join`, `Move` and `Leave` to talk to it.
- **Saves:** zones never write to the DB; they pass saves to `saver`.

Invariants to keep:
- A new login must wait until the previous session has finished saving (`world.Takeover`, serialised per account).
- On shutdown, the saver flushes under its own timeout, even when the gateway step has timed out.
- Rate limits use `Limiter.Take` and `Refund`; never split checking and recording into separate calls.

## Conventions

- Run tests with `-race`. A fix comes with a test that fails on the old code.
- Commit and push after each piece of work, with no signature or `Co-Authored-By` trailer.
