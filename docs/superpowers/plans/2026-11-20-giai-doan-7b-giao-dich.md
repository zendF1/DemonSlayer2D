# Phase 7b Direct Trade and Stalls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Two players in the same zone and in a safe zone trade up to 8 bag slots and Xu each with a two-step lock/confirm; any offer change unlocks both sides. In Ubuyashiki a player opens a stall of up to 10 priced lines while online; others view and buy, partial stacks included. Every exchange writes both characters and a `trade_logs` row in one DB transaction, so no crash can duplicate or lose items. Skill books and Lượng are not tradeable.

**Architecture:**
- Data: `item_stats.py` adds `no_trade` (true on the 25 skill books); `gamedata.Stackable.NoTrade`.
- Store: `store.Op` (interface with an unexported `exec`), `store.TradeLog`, `CharState.Ops`, `Store.SaveCharacterStates` (one transaction, ascending ids); migration `00010_trades.sql`.
- Saver: `Writer.SaveCharacterStates`; per-entry op lists appended on coalesce and kept on retry; linked groups (`SaveLinked`) written as one call, merged when they share a character. This is the shared mechanism of overview §3.2 that 7c, 7f and 7h reuse.
- Zone: trade invites, sessions, `runTrades` after `movePlayers`, the `itemLock` guard on every item/Xu command, stalls (open, close, view, buy), `Config.StallTax` hook (zero until 7h), `stall_title` in `EntityEnter`/`LookChange`, `SaveLinked` + `TradeLog` on every exchange.
- Protocol 17: envelopes 135–158, `TradeItem`, `TradeSide`, `TradeCloseReason`, `StallOffer`, `StallEntry`, errors 61–70, `EntityEnter.stall_title = 16`, `LookChange.stall_title = 4`.
- Dev: `botclient partner -user/-pass`, partner accepts and completes trades.
- Client: `core/trade.gd` (`Router.trade`), `core/stall.gd`, `item_book.no_trade`, `inventory.locked_slots`, trade window, stall setup, stall window, stall banner, player card buttons.

**Tech stack:** Go, protobuf (buf), Postgres (goose, pgx), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7b-giao-dich-design.md](../specs/2026-11-20-giai-doan-7b-giao-dich-design.md). Section references below (§n) point to it. Phase overview: [2026-11-20-giai-doan-7-tong-quan.md](../specs/2026-11-20-giai-doan-7-tong-quan.md) (§3.2 shared save mechanism, §3.7 tradeability).

**Prerequisite:** the [phase 7a plan](2026-11-20-giai-doan-7a-chat.md) is fully implemented (protocol 16, migration 00009, `internal/textutil`, `core/chat_log.gd`). Start from HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f with phase 3b in progress; client HEAD 92d588b)

When this plan was written the server code was at **phase 3a (protocol 6)** with 3b landing (`internal/items/enhance.go`, `migrations/00005_enhance.sql`); 3c–7a are designed but not implemented. Line numbers below will move — find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Saver and store**
- `saver.Saver` (`internal/saver/saver.go:23`) keeps `pending map[int64]*entry` (one entry per character: latest `store.CharState` + waiters). `Save` (`:53`) replaces `e.st` and appends `done`; `take` (`:112`) swaps the whole map out, so a batch has **no order**; `write` (`:126`) calls `Writer.SaveCharacterState` once per entry; `requeue` (`:143`) lets a newer pending state win and merges waiters. There is no notion of groups or extra writes today: Task 3 adds both without changing the "newer state wins, waiters merge" rule.
- `Writer` (`:19`) has one method, `SaveCharacterState(ctx, store.CharState) error`; `store.Store.SaveCharacterState` (`internal/store/characters.go:237`) is `pgx.BeginFunc` around `saveState(ctx, tx, st)` (`:254`), which returns early (no skills/items) when the character row is gone. Build `SaveCharacterStates` on the same `saveState` and run ops after it **even when** the row is gone.
- `store.CharState` (`internal/store/characters.go:72`) is copied by value into the saver; `Ops` is a slice field, so the saver must copy the op slice it accumulates rather than alias the caller's.
- 4c adds the account part (Lượng, stash) to `CharState`; a linked group never holds two characters of one account (one character per account in the world, `world.Takeover`), so no extra rule is needed.

**Items and zone**
- `items.Inventory.Clone` (`internal/items/items.go:81`) and `Item.Clone` (`:62`) are deep; `(*Inventory).Add(d, it)` (`internal/items/bag.go:20`) is all-or-nothing and reports changed slots; `Discard(bagSlot, count)` (`:123`) removes part of a stack. Build trade results on clones with these; for equality of an offered slot with its snapshot, compare with a deep-equal helper added beside `Clone` in `items` (tests must cover lines, hidden lines, gems and pity).
- `zone.Saver` (`internal/zone/zone.go:34`) has only `Save`; zone tests use a fake saver (`internal/zone/zone_test.go:71`) — extend it to record `SaveLinked` calls.
- 5a's `LookChange` carries the entity's whole look from a per-tick dirty list (`weapon_enhance`, `pk_on`); add `stall_title` to the same list.
- The item/Xu commands that must call `itemLock` come from 3a–6b (spec §4.9 lists them); grep `case *pb.Envelope_` in `internal/gateway/handlers.go` and the matching zone command handlers to be sure none is missed — add one zone test per command family.

**Client**
- 5b's `ui/invite_popup.gd` and `core/party.gd` hold the invite countdown pattern to copy; `ui/player_card.gd` gets the new buttons; 7a's `Router.chat.add_local` takes the trade lines.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/item_stats.py, data/items.json (gen), internal/gamedata/*   no_trade (Task 1)
migrations/00010_trades.sql (new), internal/store/characters.go,
internal/store/ops.go (new), internal/store/store_test.go                ops, linked save (Task 2)
internal/saver/saver.go, saver_test.go                                    linked groups (Task 3)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures                protocol 17 (Task 4)
internal/zone/trade.go (new), zone.go, lifecycle.go, inventory.go,
internal/items/items.go, internal/zone/zone_test.go                       direct trade, item lock (Task 5)
internal/zone/stall.go (new), move.go, internal/zone/zone_test.go         stalls (Task 6)
internal/world/world.go, internal/gateway/handlers.go,
internal/client/client.go, cmd/botclient/main.go, e2e/                    wiring, botclient, e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec                    docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/                       sync (Task 9)
core/wire.gd, core/errors.gd                                              protocol 17 (Task 9)
core/trade.gd (new), core/stall.gd (new), core/item_book.gd,
core/inventory.gd, autoload/router.gd                                      models (Task 10)
ui/trade_window.gd (new), ui/stall_setup.gd (new), ui/stall_window.gd (new),
ui/invite_popup.gd, ui/player_card.gd, ui/entity_view.gd, ui/char_window.gd,
ui/hud.gd, screens/world.gd                                               UI (Task 11)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md  tests, smoke (Tasks 9–12)
```

---

### Task 1: Server — the `no_trade` flag

**Files:** `docs/balance/item_stats.py`, `data/items.json` (generated), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:** stackable field `no_trade` (optional, default false) → `gamedata.Stackable.NoTrade bool`; `func (d *Items) NoTrade(code string) bool` (false for equipment and unknown codes).

**TODO / tests:**
- [ ] Script: `"no_trade": true` on every `book_*`; `--check` clean; regenerate.
- [ ] `// ____TEST ITEMS____`: real data has exactly the 25 books flagged; a file without the field loads as false.
- [ ] Commit: `Mark skill books as not tradeable`.

### Task 2: Server — store ops, linked saves and the trade log

**Files:** `migrations/00010_trades.sql` (new), `internal/store/ops.go` (new), `internal/store/characters.go`, `internal/store/store_test.go`.

**Interface:**
- `type Op interface { exec(ctx context.Context, tx pgx.Tx) error }` — only `store` defines kinds.
- `type TradeLog struct { Kind string; A, B int64; AItems, BItems []ItemRow; AXu, BXu, Tax int64 }` (`Kind` is `"trade"` or `"stall"`; items marshalled with the same JSON shape as `character_items` columns plus the source slot).
- `CharState.Ops []Op`.
- `func (s *Store) SaveCharacterStates(ctx context.Context, sts []CharState) error` — §3.2; `SaveCharacterState` delegates to it.
- Test-only (in `store_test.go` via an exported-for-test hook or a `storetest` helper): an op that always fails, to prove rollback.

**Behaviour:** §3.1–3.2. One `pgx.BeginFunc`; states sorted by `ID` ascending (copy, do not reorder the caller's slice); each state's character part (`saveState`), then its ops in order; any error aborts all.

**TODO / tests** (`// ____TEST LINKED SAVES____`, DB):
- [ ] Migration up/down.
- [ ] Two characters saved in one call; both rows updated.
- [ ] The failing op leaves both characters exactly as before (Xu, items) — the "crash mid-trade" case.
- [ ] `TradeLog` inserts one row with both item lists (lines, hidden, gems, pity round-trip through jsonb), Xu and tax.
- [ ] A deleted character still runs its ops; `SaveCharacterState` behaves as before (existing tests green).
- [ ] Commit: `Save several characters and attached writes in one transaction`.

### Task 3: Server — saver groups and attached writes

**Files:** `internal/saver/saver.go`, `internal/saver/saver_test.go`, `internal/app/app.go` (writer wiring if needed).

**Interface:**
- `type Writer interface { SaveCharacterStates(ctx context.Context, sts []store.CharState) error }`.
- `func (s *Saver) Save(st store.CharState, done chan<- error)` — replaces the state, **appends** `st.Ops`.
- `func (s *Saver) SaveLinked(sts []store.CharState, done chan<- error)` — §3.2; `done` gets exactly one value when the whole group is written (or the context is cancelled).

**Behaviour:** §3.2 "Saver". Suggested model: `entry{st, ops, waiters, group *group}`, `group{ids map[int64]struct{}, waiters}`; merge = move ids and waiters into the larger group and repoint entries. `take` returns write units (one per group, one per lone entry); `write` calls the writer once per unit with each member's state carrying its accumulated ops; `requeue` restores units, merging with newer pending entries (newer state wins, old ops then new ops, waiters merged, groups merged). Log a failed unit with all its character ids.

**TODO / tests** (`// ____TEST GROUPS____`, `// ____TEST OPS____`, fake writer that records calls and can fail N times):
- [ ] Every saver case of spec §8 (linked pair in one call; ops appended across two saves; retry keeps ops and the whole group; newer single save inside a pending group; overlapping groups merge into one call; failed group merges with a newer save; `Stop` flushes groups; cancelled context fails entry and group waiters once).
- [ ] Existing saver tests stay green with the new writer signature (adapt the fake).
- [ ] Commit: `Let the saver write linked characters and attached writes atomically`.

### Task 4: Server — protocol 17

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 135–158; the five new messages/enums; `EntityEnter.stall_title = 16`, `LookChange.stall_title = 4`; errors 61–70 with one-line comments; `NOT_IN_SAFE_ZONE` comment updated. `ProtocolVersion = 17`. Confirm 7a left 134/60 as the highest numbers it could use and nothing else took 135–159 or 61–72.

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe (`TradeSide.mine`/`theirs`, no `message`/`class`).
- [ ] Fixtures of spec §5; regenerate all with `-update`; version test → 17.
- [ ] Commit: `Add the phase 7b trade and stall messages and bump the protocol to 17`.

### Task 5: Server — direct trade in the zone

**Files:** `internal/zone/trade.go` (new), `internal/zone/zone.go`, `internal/zone/lifecycle.go`, `internal/zone/inventory.go` (and the 3b–6b command files that change bags or Xu), `internal/items/items.go`, `internal/zone/zone_test.go`.

**Interface:**
- `func (z *Zone) TradeInvite(id uint64, seq uint32, target uint64) bool`, `TradeAnswer(id, seq, from uint64, accept bool) bool`, `TradeOffer(id, seq, slots []int, xu int64) bool`, `TradeLock(id, seq) bool`, `TradeConfirm(id, seq) bool`, `TradeCancel(id, seq) bool` (false on a full inbox).
- `func (z *Zone) itemLock(p *player) pb.ErrorCode`; `func (z *Zone) runTrades()`; `func (z *Zone) closeTrade(p *player, reason pb.TradeCloseReason)`.
- `zone.Saver` gains `SaveLinked`.
- `func (it Item) Equal(o Item) bool` in `items` (deep).
- Constants of spec §2.

**Behaviour:** §4.1–4.9, §4.15. Check orders exactly as §4.2–4.6; execution as §4.7 (clones first, `BAG_FULL` vs `PARTNER_BAG_FULL`, apply, clear pending refine, `SaveLinked` with the `TradeLog` on side A's state, pushes before the response). The leave path (logout, disconnect, every `Transfer` kind) closes the trade and drops invites before the handoff is built. `itemLock` is the first check after `DEAD` in every command of §4.9.

**TODO / tests** (`// ____TEST TRADE____`, `// ____TEST ITEM LOCK____`):
- [ ] Every zone trade case of spec §8 (invite/answer orders, expiry, replaced invite, decline; offer orders, book `NOT_TRADABLE`, re-offer unlocks; confirm before lock; full exchange with stack merge and one `SaveLinked` + one `TradeLog`; both bag-full variants leave everything unchanged; `TOO_FAR` at 8.1 tiles but not at 7.9; leaving the safe zone; portal and logout give `LEFT` and no trade in the handoff).
- [ ] `IN_TRADE` for one command of each family (pickup, equip, discard, sort, use, buy, sell, stash, enhance, refine, quest turn-in, learn); all pass again after the trade closes.
- [ ] `items.Equal` covers lines, hidden lines, gems and pity.
- [ ] Commit: `Trade items and Xu between two players in a zone`.

### Task 6: Server — stalls

**Files:** `internal/zone/stall.go` (new), `internal/zone/move.go`, `internal/zone/zone.go`, `internal/zone/zone_test.go`.

**Interface:**
- `func (z *Zone) StallOpen(id uint64, seq uint32, title string, entries []StallOffer) bool`, `StallClose(id, seq) bool`, `StallView(id, seq, owner uint64) bool`, `StallBuy(id, seq, owner uint64, index, count int, price int64) bool`.
- `type StallOffer struct { Slot, Count int; Price int64 }`.
- `zone.Config.StallTax func() (guildID uint64, bp int)` (nil → `(0, 0)`).

**Behaviour:** §4.10–4.14. Placement uses the map's NPCs, spawn points and portal rectangles (point-to-rectangle distance of 4a) and the other open stalls of the zone; `MoveInput` of a stall owner sets no direction but still advances `ack_input_seq`; open/close/sell-out mark the look dirty; buy order exactly as §4.13 with `Discard` for the partial take, the tax hook, `SaveLinked` with `TradeLog{kind: "stall"}` on the seller's state, `StallSold` before `StallBuyResp`. `EntityEnter.stall_title` filled for players.

**TODO / tests** (`// ____TEST STALL____`):
- [ ] Every zone stall case of spec §8 (open orders including training map and NPC/stall gaps; move ignored with ack; look changes; buy orders, the 4-of-10 example, wrong price, sell-out closes, tax hook (7, 500) gives 95%; `IN_STALL` on the seller).
- [ ] Commit: `Let players open stalls in town and buy from them`.

### Task 7: Server — wiring, botclient and e2e

**Files:** `internal/world/world.go`, `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `cmd/botclient/main.go`, `e2e/e2e_test.go`.

**Interface:** ten `Membership` methods mirroring the zone commands; gateway cases per §4.16; client helpers `TradeInvite`, `TradeAnswer`, `TradeOffer`, `TradeLock`, `TradeConfirm`, `TradeCancel`, `StallOpen`, `StallClose`, `StallView`, `StallBuy` and recording of `TradeInvite`/`TradeState`/`TradeClosed`/`StallSold` pushes; botclient `partner -user U -pass P` (existing account) and the trade-accepting behaviour of §6.4.

**TODO / tests:**
- [ ] Gateway: the ten requests before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestTradeOverWebSocket` and `TestStallOverWebSocket` per spec §8 (use `DevSetLevel`, `DevSetPlace`, the dev item helpers; positions from the generated `maps/ubuyashiki.json`, at least 2 tiles from NPCs and spawns).
- [ ] Commit: `Handle trade and stall requests and add end-to-end trade tests`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 7.2 per §9; phase 0 spec §3.3 lists `trade_logs`; CLAUDE.md invariants (linked saves and ops, `itemLock`); README: trade, stalls, partner flags, a sample `trade_logs` query by character name.
- [ ] Commit: `Document the phase 7b trade and stalls`.

### Task 9: Client — sync and protocol 17

**Files:** `server` submodule, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 8; `tools/sync_server.sh` (brings the regenerated `items.json`); `PROTOCOL_VERSION := 17`; `PAYLOADS` adds the 24 payloads; error texts of §6.1.

**TODO / tests:**
- [ ] Decode the six new fixtures (`entity_enter_stall.stall_title`, `trade_state` sides, `stall_view_resp` entries).
- [ ] Commit: `Sync the phase 7b trade protocol and item data from the server`.

### Task 10: Client — trade and stall models

**Files:** `core/trade.gd` (new), `core/stall.gd` (new), `core/item_book.gd`, `core/inventory.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`trade.gd`, `stall.gd` functions as listed; `item_book.no_trade(code)`; `inventory.locked_slots`; `Router.trade`).

**TODO / tests** (`# ____TEST TRADE____`):
- [ ] Every client case of spec §8 (`action()` sequence lock → wait → confirm; re-offer sets `theirs_changed` for 3 s; `can_add` with a book and with 8 slots; invite expiry; the five close texts; stall `validate` and `place_block`; `total`).
- [ ] Commit: `Add the trade and stall models`.

### Task 11: Client — trade and stall UI

**Files:** `ui/trade_window.gd` (new), `ui/stall_setup.gd` (new), `ui/stall_window.gd` (new), `ui/invite_popup.gd`, `ui/player_card.gd`, `ui/entity_view.gd`, `ui/char_window.gd`, `ui/hud.gd`, `screens/world.gd`.

**Behaviour (§6.2–6.3):** player card buttons; trade invite popup; trade window (two columns, picker, Xu input, Khóa/Xác nhận/Hủy, red flash on partner change, confirm summary); stall setup from the bag tab on town maps; "Đang bày sạp" HUD state with Đóng sạp and a dimmed joystick; stall window with count, total, Mua, re-view on `STALL_CHANGED`; stall banner above the owner (tap opens it); lock icons on locked bag slots; chat `add_local` lines. `world.gd` only wires messages to the models and windows.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with `tools/run2.sh`: a full trade with a re-offer, a stall sale. Screenshots in the report.
- [ ] Commit: `Show the trade window, stall setup and stall view`.

### Task 12: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.4):** prepare the partner account and both characters at level 10 (`devchar -u … -level 10`), 1,000 Xu for the smoke character (`devitem`), run `botclient partner -user … -pass …`; trade 100 Xu; check the Xu drop and the partner's `TRADE OK`. Print `SMOKE OK (… trade 100 Xu)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: trading, stalls, what cannot be traded.
- [ ] Commit: `Extend the smoke test with a direct trade`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID); migration 00010 applies.
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
