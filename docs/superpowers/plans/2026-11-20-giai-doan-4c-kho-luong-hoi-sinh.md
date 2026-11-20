# Phase 4c Account Stash, Lượng and Respawn in Place Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A storage keeper NPC in the town and every camp opens the account-wide stash: deposit, withdraw, sort, and expand it for Xu (30 → 120 slots). Each account has a Lượng wallet, granted for now by `devitem -luong`; Lượng pays for respawning where you died (1 Lượng) and buys from the Lượng shop opened from the HUD.

**Architecture:**
- Migration 00007 adds `accounts.luong`, `accounts.stash_size` and the `account_stash` table. `GetCharacter` loads the account part; `SaveCharacterState` writes it in the same transaction whenever `CharState` carries it. Only one character of an account is ever in the world, so the zone holding that character owns the stash and the Lượng.
- `items.json` gains the `stash` block and the `luong` shop (`anywhere`); maps gain `storage` NPCs.
- The zone handles five stash commands, Lượng purchases through `BuyReq.shop_id`, and `RespawnReq.in_place`. The 4a handoff carries the account part across zones.
- Protocol 11: five stash request/response pairs (95–104), `Stash`, `BuyReq.shop_id`, `RespawnReq.in_place`, `SelfStats.luong`, three error codes.

**Tech stack:** Go, protobuf (buf), Postgres (goose migrations), Python 3 scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md](../specs/2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 4b plan](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc.md) is fully implemented (NPCs and `nearNPC`, shops and `BuyReq`, protocol 10). Start from a server HEAD that contains it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD ac605a4 plus the phase 3–4b specs)

Phases 3, 4a and 4b land before this plan and rename or add most of the symbols below. Find each by name (`graphify explain "<symbol>"`) before editing.

- **Store today:** `store.Character` has `AccountID` (`internal/store/characters.go:15`); `GetCharacter` (`:116`) and `ListCharacters` (`:101`) share `charColumns`/`scanCharacter`; `SaveCharacterState` (`:165`) runs `saveState` (`:172`) in one `pgx` transaction. 3a adds `Items []ItemRow` with delete-then-insert of `character_items`; 3b/3c add `pity` and `gems`. `store/accounts.go` has `Account`, `CreateAccount`, `GetAccountByUsername`, `TouchLogin` (`:25-53`). Latest migration after phase 3: `00006_sockets.sql`.
- **One character per account in the world:** `World.Takeover` (`internal/world/world.go:361`) kicks a previous session and waits for its `Done`, which `conn.cleanup` (`internal/gateway/conn.go:161`) closes only after the leave save; `handleLeaveWorld` (`handlers.go:263`) waits for the save before the client can enter another character. This is what makes zone-owned account data safe.
- **Saver:** coalesces per character id and keeps the newest state (`internal/saver/saver.go:56`), so the account part must be present in **every** `CharState` the zone saves, never only in "dirty" ones.
- **Item rules (3a):** `items.Inventory.Add` (all or nothing, merge then first free), `Sort`; reuse them on a plain slot slice for the stash — if they are methods on `Inventory` bound to `Bag`, factor the slice logic into functions (`AddTo(d, slots []*Item, it)`, `SortSlots(d, slots)`) and keep the methods as wrappers.
- **World item validation (3a–3c):** `world.knownItems` drops bad rows with one warning each; extend it with a slot limit parameter for the stash.
- **Respawn (4a):** `respawnPlayer` moves to `nearestRespawn`; the auto respawn runs in `runPlayerTimers` after `autoRespawnSec`. `RespawnReq` is handled by `z.respawn` (`internal/zone/lifecycle.go:17`).
- **Shops (4b):** `BuyReq{npc_id, code, count}` and `gamedata.Shop{Currency, Goods}`; NPC roles `shop`, `blacksmith`, `carriage`.
- **devitem (3a):** `cmd/devitem` + `store.DevGiveItem` with `-xu`; add `-luong` there.
- **Client:** `ui/death_panel.gd` has one button "Về khu an toàn" and `RESPAWN_SEC = 60` (`:11`); `ui/shop_window.gd`, `ui/npc_window.gd`, `core/shop_rules.gd`, `core/inventory.gd` come from 4b/3a.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/item_stats.py, data/items.json                       stash block, Lượng shop (Task 1)
internal/gamedata/items.go, gamedata_test.go                      parse + validate (Task 1)
tools/mapgen.py, maps/*.json, internal/mapdata/                    storage NPCs (Task 1)
migrations/00007_account_stash.sql (new)                          schema (Task 2)
internal/store/characters.go, accounts.go, store_test.go          account part load/save (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, internal/gateway/       protocol 11 (Task 3)
internal/items/bag.go, items_test.go                              slot-slice add/sort (Task 4)
internal/world/world.go, world_test.go                            stash validation, PlayerInfo (Task 4)
internal/zone/zone.go, transfer.go, progression.go                account state, handoff, SelfStats (Task 4)
internal/zone/stash.go (new)                                      stash commands (Task 5)
internal/zone/npc.go, lifecycle.go                                Lượng buy, respawn in place (Task 6)
internal/gateway/handlers.go, internal/client/client.go           new requests (Tasks 5, 6)
internal/store/dev.go, cmd/devitem/main.go                        -luong (Task 7)
e2e/e2e_test.go                                                   stash and Lượng e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec           docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/, data/, proto/, tests/fixtures/, autoload/net.gd, core/wire.gd, core/errors.gd   sync, protocol 11 (Task 9)
core/stash.gd (new), core/inventory.gd, core/shop_rules.gd, core/item_book.gd            rules (Task 10)
ui/stash_window.gd (new), ui/npc_window.gd, ui/shop_window.gd, ui/hud.gd                 UI (Task 11)
ui/bag_tab.gd, ui/death_panel.gd, ui/map_view.gd, screens/world.gd                       wiring (Task 11)
tests/unit/test_core.gd, tests/unit/test_net.gd                                          tests (Tasks 9, 10)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                                          smoke (Task 12)
```

---

### Task 1: Server — stash and Lượng shop data, storage NPCs

**Files:** `docs/balance/item_stats.py`, `data/items.json`, `internal/gamedata/items.go`, `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`, `tools/mapgen.py`, `maps/*.json`, `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`.

**Interface:**
- `Items.Stash StashRules{Base, Step, Max int; ExpandXuPerStep int64}`; `(StashRules).ExpandCost(size int) (int64, bool)` — false at `Max`.
- `Shop.Anywhere bool`; currency constants `CurrencyXu`, `CurrencyLuong`.
- `mapdata.RoleStorage`.

**Behaviour:** §2.1 data and checks; §2.2 storage NPCs (within 3 tiles of each `default`; Kumotori at (3.5, 7.5)); `Validate(maps)` refuses NPCs pointing at `anywhere` shops.

**TODO / tests** (`// ____TEST STASH DATA____`):
- [ ] Data loads; `ExpandCost(30) = 20,000`, `ExpandCost(110) = 180,000`, `ExpandCost(120)` is false; the `luong` shop sells `charm_protect` for 2.
- [ ] Reject table per §8 `gamedata` and `mapdata` bullets.
- [ ] Real maps: five storage NPCs, each within 3 tiles of its map's `default`; `--check` clean.
- [ ] Commit: `Add the stash rules, the Lượng shop and storage keeper NPCs`.

### Task 2: Server — migration and the account part in the store

**Files:** `migrations/00007_account_stash.sql` (new), `internal/store/characters.go`, `internal/store/accounts.go`, `internal/store/store_test.go`.

**Interface:**
- `type AccountState struct { ID int64; Luong int64; StashSize int; Stash []ItemRow }` (stash rows use `Equipped: false`).
- `Character.Account AccountState` — filled by `GetCharacter` only (two extra queries: account columns, stash rows); `ListCharacters` leaves it zero.
- `CharState.Account *AccountState` — nil writes nothing to the account.
- `saveState`: when `Account != nil`, `UPDATE accounts SET luong, stash_size WHERE id`, then delete and re-insert `account_stash` rows (same insert method as `character_items`).

**TODO / tests** (`// ____TEST ACCOUNT STASH____`, DB):
- [ ] Migrate up on a DB with an existing account: Lượng 0, stash size 30; goose down then up again works.
- [ ] Save then `GetCharacter`: Lượng, size and stash rows (with lines, hidden, gems, pity) round-trip; an empty stash list deletes old rows; `Account == nil` leaves the account untouched.
- [ ] Two characters of one account load the same stash; deleting the account cascades the stash.
- [ ] Commit: `Store the account stash and Lượng with the character save`.

### Task 3: Server — protocol 11

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/`, `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 95–104; `Stash`; `BuyReq.shop_id = 4`; `RespawnReq.in_place = 1`; `SelfStats.luong = 24`; error codes 42–44; `ProtocolVersion = 11`.

**TODO / tests:**
- [ ] `make proto`; fixtures `stash_open_resp`, `stash_deposit_resp`, `respawn_req_in_place`; regenerate all; version test → 11.
- [ ] Commit: `Add the phase 4c stash and Lượng messages and bump the protocol to 11`.

### Task 4: Server — account state through world, zone and handoff

**Files:** `internal/items/bag.go`, `internal/items/items_test.go`, `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/zone.go`, `internal/zone/transfer.go`, `internal/zone/progression.go`, tests.

**Interface:**
- `items.AddTo(d, slots []*Item, it Item) ([]int, error)`, `items.SortSlots(d, slots []*Item)`; `Inventory.Add`/`Sort` call them.
- `zone.PlayerInfo` gains `AccountID int64`, `Luong int64`, `Stash []*items.Item` (length = stash size).
- `player` gains the same three; `state(p)` sets `CharState.Account` from a deep copy **on every save**; `handoff` copies them; `selfStats` sets `Luong`.
- `world.knownItems` gains a slot limit; `Enter` clamps the stash size into `[Base, Max]` with a warning and builds the stash slice.

**TODO / tests:**
- [ ] `// ____TEST SLOT SLICES____`: `AddTo` on a 30-slot slice merges and fills like `Inventory.Add`; `SortSlots` matches `Sort`.
- [ ] World: a stash row at slot 35 with size 30 is dropped; size 10 → 30, 150 → 120, each logged.
- [ ] Zone: every queued save (periodic, leave, departure) carries the account part; mutating the saved stash does not touch the player's; a portal transfer keeps Lượng and stash; `SelfStats.luong` is set.
- [ ] Commit: `Carry the account stash and Lượng through the world and zones`.

### Task 5: Server — stash commands

**Files:** `internal/zone/stash.go` (new), `internal/zone/zone.go`, `internal/world/world.go` (`Membership`), `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:** zone commands + wrappers + gateway cases + client helpers: `StashOpen(npcID)`, `StashDeposit(npcID, bagSlot)`, `StashWithdraw(npcID, stashSlot)`, `StashExpand(npcID)`, `StashSort(npcID)`; a zone helper `stashSlots(idx []int) []*pb.ItemSlot`.

**Behaviour (§4.2):** shared steps (`DEAD`, `nearNPC(..., RoleStorage)`), then each command's steps; deposit/withdraw/sort clear the 3c pending refine; open and expand do not; every changing command saves at once.

**TODO / tests** (`// ____TEST STASH____`):
- [ ] Every §8 `zone` stash bullet: errors in order for the five commands, the 600 + 500 stones case, full stash and full bag leaving everything unchanged, expansion costs 20,000 then 40,000, `STASH_MAXED` at 120, sort listing every slot, pending-refine behaviour, one save per changing command.
- [ ] Gateway: the five requests before entering the world get `NOT_IN_WORLD`.
- [ ] Commit: `Let players use the account stash at the storage keeper`.

### Task 6: Server — Lượng purchases and respawn in place

**Files:** `internal/zone/npc.go`, `internal/zone/lifecycle.go`, `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:** `Buy` gains `shopID`; `Respawn(id, seq, inPlace bool)`; `Membership.Respawn(seq, inPlace)`; `client.Respawn(ctx, inPlace)`, `client.Buy(ctx, npcID, shopID, code, count)`.

**Behaviour:** §4.3 order (the `npc_id`/`shop_id` branch at step 2; balance by currency at step 5); §4.4: in place keeps the position, full HP/MP, `snap` not needed (same place) but `hpChanged`, Lượng −1, save at once; the auto respawn stays "về làng".

**TODO / tests** (`// ____TEST LUONG____`):
- [ ] Every §8 `zone` Lượng bullet: Lượng shop with an empty `npc_id`, `general` without an NPC refused, `NOT_ENOUGH_LUONG`, respawn in place (position, HP/MP, Lượng, save, viewers), respawn in place with 0 Lượng refused, về làng free, auto respawn về làng.
- [ ] Commit: `Spend Lượng in the Lượng shop and to respawn in place`.

### Task 7: Server — devitem -luong and the stash e2e

**Files:** `internal/store/dev.go`, `internal/store/store_test.go`, `cmd/devitem/main.go`, `e2e/e2e_test.go`, `README.md`.

**Interface:** `store.DevAddLuong(ctx, username string, n int64) (int64, error)` (or a `luong` parameter on `DevGiveItem` — pick one and document it); `devitem -luong`.

**TODO / tests:**
- [ ] `// ____TEST DEV LUONG____` (DB): `-luong 5` adds to the account; alone works; missing account errors.
- [ ] e2e `TestStashOverWebSocket` exactly as §8.
- [ ] README: stash, Lượng, `devitem -luong`.
- [ ] Commit: `Add devitem -luong and an end-to-end stash and Lượng test`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 4.1, 5.7, 7.1 per §9; phase 0 roadmap note; CLAUDE.md account-data invariant.
- [ ] Commit: `Document the phase 4c stash, Lượng and respawn in place`.

### Task 9: Client — sync and protocol 11

**Files:** `server` submodule, `data/`, `proto/protocol.gd`, `tests/fixtures/`, `autoload/net.gd`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_net.gd`.

**TODO / tests:**
- [ ] Bump the submodule; sync; `PROTOCOL_VERSION := 11`; ten new `PAYLOADS`; three error texts.
- [ ] Decode the three new fixtures; `tests/run.sh` green including `--check`.
- [ ] Commit: `Sync phase 4c protocol, maps and item data from the server`.

### Task 10: Client — stash state and Lượng rules

**Files:** `core/stash.gd` (new), `core/inventory.gd`, `core/shop_rules.gd`, `core/item_book.gd`, `tests/unit/test_core.gd`.

**Interface:** as §6.1; move the add simulation of `inventory.gd` (`can_take`) into a shared static helper over `(slots: Dictionary, size: int)` that both `inventory.gd` and `stash.gd` call.

**TODO / tests** (`# ____TEST STASH____`):
- [ ] Load a `Stash` fixture (40 slots, two items); apply a changed-slot list with an emptied slot; `can_take` merges and refuses when full.
- [ ] `expand_cost` 30 → 20,000, 110 → 180,000, 120 → −1; `expand_block` "max" and "xu".
- [ ] `buy_block` in a Lượng shop reports "luong" when short; `inventory.luong` follows `SelfStats`.
- [ ] Commit: `Add the client stash state and Lượng rules`.

### Task 11: Client — stash window, Lượng shop, death panel

**Files:** `ui/stash_window.gd` (new), `ui/npc_window.gd`, `ui/shop_window.gd`, `ui/hud.gd`, `ui/bag_tab.gd`, `ui/death_panel.gd`, `ui/map_view.gd`, `screens/world.gd`.

**Behaviour (§6.2):** storage NPC drawing and the Kho button; the stash window (Kho / Hành trang tabs, Gửi, Rút, Sắp xếp, Mở rộng, auto-close beyond 4 tiles); "Xu · Lượng" in the bag tab; the HUD Lượng button opening the shop window in Lượng mode (`BuyReq{shop_id: "luong"}`, buy tab only); the death panel's two buttons and the "Tự về làng" countdown.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with two characters on one account and some Lượng: deposit with one, withdraw with the other; expand; buy a charm with Lượng; die and respawn in place, then die with 0 Lượng and see the button locked. Screenshots in the report.
- [ ] Commit: `Add the stash window, the Lượng shop and respawn choices`.

### Task 12: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `tools/smoke.sh`, `README.md`.

**Behaviour (§6.3):** `devitem -luong 3` for the smoke account; buy a charm with Lượng, deposit it at the Kumotori storage keeper, see it in the stash, withdraw it. Print `SMOKE OK (… luong 3 → 1, stash n items)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: stash, Lượng shop, respawn choices.
- [ ] Commit: `Extend the smoke test with the stash and the Lượng shop`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet, `mapgen.py --check`; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID). `devitem -luong 5` for `test1` and `test2` while they are offline; make sure `test1` has a second character.
- [ ] Tell the user what to try: share items between `test1`'s two characters through the stash, expand it, buy a charm in the Lượng shop, die and respawn in place.
