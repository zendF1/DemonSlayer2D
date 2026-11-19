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

## Code map (graphify)

`graphify-out/graph.json` maps the code's functions, types, calls and modules. It covers code only: `.graphifyignore` excludes `docs/`, `*.md` and the generated `internal/pb/`. The graph is local and git-ignored.

- **Use the graph before grep or reading files.** For questions about the code, run `graphify query "<question>"`, `graphify explain "<symbol>"` or `graphify path "<A>" "<B>"`. Then read only the lines the graph points to.
- **Overview:** `graphify-out/GRAPH_REPORT.md` lists the communities and god nodes. Read it only for a broad overview.
- **Keeping it current:** git hooks rebuild the graph after commit, checkout and merge. After editing code but before committing, run `graphify update .` (AST only, no cost). After deleting code, run `graphify update . --force`.
- **If `graphify-out/` is missing** (fresh clone), run `graphify update .` and then `graphify hook install`.

## Invariants (not visible in the graph)

- A new login must wait until the previous session has finished saving (`world.Takeover`, serialised per account).
- Zone state is touched only by that zone's goroutine. Zones never write to the DB; they pass saves to `saver`.
- On shutdown, the saver flushes under its own timeout, even when the gateway step has timed out.
- Rate limits use `Limiter.Take` and `Refund`; never split checking and recording into separate calls.
- Proto names must stay valid in GDScript (no field named `message` or `class`, no message named `Error`) because the client generates code with godobuf.

## Conventions

- Run tests with `-race`. A fix comes with a test that fails on the old code.
- Keep one or two test files per package (for example `internal/auth/auth_test.go`), not one per source file. Split sections with a comment line such as `// ______________________TEST LIMITER_____________________`.
- Commit and push after each piece of work, with no signature or `Co-Authored-By` trailer.
