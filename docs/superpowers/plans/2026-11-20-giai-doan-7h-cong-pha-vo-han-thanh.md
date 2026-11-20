# Phase 7h Infinity Castle Siege and Castle Owner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Saturday 20:00–21:00 UTC+7 guild event. Guild members of level 50+ (in the guild since before the start) enter the current frontier of five single-zone battle maps through the Sứ giả Vô Hạn Thành NPC. Map 1 has lesser and aberrant demons and three Hạ Huyền in turn; maps 2–4 one Thượng Huyền each; map 5 Quỷ Vương. A boss kill opens the next map at once; a map's time limit removes its boss and opens the next anyway. Killers earn personal and guild points (monsters and players). At the end the Top 1 guild captures the castle if Quỷ Vương died; participation, guild milestone and rank rewards go to the reward mailbox. The castle owner's leader sets a 0–5% tax on stall sales that flows into the owner guild's fund in the same DB transaction as the sale.

**Architecture:**
- Data: `tools/mapgen.py` adds `vht_1`..`vht_5` (battle, `event_only`, `max_zones: 1`, `capacity: 150`, `min_level: 50`, `boss_arena` area, locked forward portals), the region field `respawn_sec` and the `su_gia_vht` NPC (7g's role `event`); `monster_stats.py` adds the two siege demons and seven `event_only` bosses (`vht_` prefix); `event_data.py` adds the `vo_han_thanh` event of the new kind `siege` to `data/events.json` with its config under the `siege` key (`gamedata.Event.Siege`).
- Store: migration 00013 (`castle_owner`, `siege_runs`), castle/siege functions, the `GuildFundTax` op.
- New world-level service `internal/siege` with its own 1 s goroutine: run state, boss schedule, scores, ranking, end-of-run results, castle owner and tax; publishes an atomic snapshot that zones read every second.
- Zone: `Config.Siege` (spawn/remove bosses by snapshot, score callbacks, `SiegeState` every 2 s, `SiegeResult`); 7b's `Config.StallTax` filled from the castle snapshot (owner id also drives `castle_owner` in the look).
- World: `SiegeEnter`/`SiegeLeave` transfers, `PORTAL_LOCKED` on forward portals.
- Protocol 23: envelopes 225–234, `SiegeMap`/`SiegeState`/`SiegeResult`/`CastleInfo`, `EnterWorldResp.siege = 20`, `.castle = 21`, `EntityEnter.castle_owner = 19`, `LookChange.castle_owner = 6`, errors 105–109.
- Dev: `cmd/devcastle`.
- Client: `core/siege_board.gd`, `core/castle.gd`, NPC window, siege panel and scoreboard, result, castle window, owner mark, stall tax line.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7h-cong-pha-vo-han-thanh-design.md](../specs/2026-11-20-giai-doan-7h-cong-pha-vo-han-thanh-design.md). Section references below (§n) point to it. Number ranges: [phase 7 overview](../specs/2026-11-20-giai-doan-7-tong-quan.md).

**Prerequisite:** phases 7a–7g are fully implemented (server protocol 22, migration 00012; 7b stalls with `Config.StallTax`, `store.Op`, `Saver.SaveLinked`; 7c guild service with `MemberInfo(charID) (guildID uint64, rank Rank, joinedAt time.Time, ok bool)`, `Exists`, `AddFundCached`, `AddFund(ctx, guildID, xu)`, the guild Tag cell and `cmd/devguild`; 7d `SpawnSpecial`/`DespawnTag` and boss skills; 7e event service, per-kind config keys in `events.json`, `EVENT_DEV_START`; 7f map fields `max_zones`/`capacity`/`min_level` (`LEVEL_TOO_LOW` as the last transfer step), `Scoreboard`/`ScoreRow`, `reward.Service.Grant`, `EVENT_DEV_MINUTES`; 7g `areas`, `event_only`, NPC role `event`). Start from HEADs that contain them.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–7g are designed, not yet implemented)

Phases 3b–7g land before this plan and move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Services and the world**
- `World.Start` (`internal/world/world.go:135`) runs the periodic `closeIdle` loop; start the siege goroutine beside the other services from `internal/app` (where 5b's party service and 7e's event service are built), not from the world, so the world only reads its snapshot.
- `World.Enter` (`:272`) → `placement` (`:400`) already handles 7g's `event_only` rule, which covers the siege maps.
- Single-zone maps: 7f makes `reserve` (`:408`) honour `max_zones`/`capacity`; `SiegeEnter` uses the same path for the frontier map, so `ZONE_FULL` comes from it.
- 4a's portal check is "step 1 `BAD_REQUEST` in the world"; add `PORTAL_LOCKED` right after it for forward portals of siege maps (portal id `to_vht_<n>` with n greater than the current map's index).

**Zone**
- Tick order (`internal/zone/zone.go:370`) is unchanged; the once-per-second snapshot read goes in the same place as 6d's day check / 7e's event check.
- Monster deaths with a killer go through `applyToMonster` (`internal/zone/impact.go:79`) → `loot` (`internal/zone/loot.go:30`); call `MonsterKill` beside 6b's quest credit. Player deaths go through `killPlayer` (`internal/zone/combat.go:51`), where 5a records the killer; call `PlayerKill` there.
- 5a's `LookChange` carries the whole look filled at send time from a per-tick changed-id list; add `castle_owner` to that fill and push the ids whose flag flips when the snapshot's owner id changes.
- 7b's stall purchase builds the linked save; the tax op and `AddFundCached` call go into that same code path (Task 4).

**Data**
- `monster_stats.py` (`KINDS` around line 44) generates `monsters.json`; 7d added rank multipliers and the skill schema. `event_data.py` (7e) writes every event into `events.json`; the siege config is the `siege` key of the `vo_han_thanh` event.
- `cmd/devguild -create <guild> -leader <char> [-members a,b]` (7c) runs with the server stopped, so the `joined_at` it writes is always before a dev siege run (`EVENT_DEV_START` starts it 60 s after boot); 7h needs no new devguild flag.
- Gem codes come from 3c: `gem_attack_g`, `gem_hp_g`, `gem_defense_g`.

## File structure

Server (`DemonSlayer2D`):

```
tools/mapgen.py, maps/vht_*.json, maps/ubuyashiki.json (gen), internal/mapdata/*        siege maps, respawn_sec, NPC (Task 1)
docs/balance/monster_stats.py, data/monsters.json (gen)                                  siege monsters and bosses (Task 1)
docs/balance/event_data.py, data/events.json (gen), internal/gamedata/*                 siege event kind (Task 1)
migrations/00013_castle.sql (new), internal/store/*                                      castle, results, tax op (Task 2)
cmd/devcastle (new)                                                                      dev tool (Task 2)
internal/siege/siege.go, rank.go, rewards.go (new), siege_test.go (new)                 the service (Task 3)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures                               protocol 23 (Task 4)
internal/zone/siege.go (new), zone.go, combat.go, loot.go, stall code of 7b               bosses, scores, tax, look (Task 5)
internal/world/world.go, internal/app/*                                                  transfers, portal lock, wiring (Task 6)
internal/gateway/handlers.go, internal/client/client.go, e2e/e2e_test.go                  wiring, e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec, phase 7 overview                docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/, tools/sync_server.sh       sync (Task 9)
core/wire.gd, core/errors.gd, core/siege_board.gd (new), core/castle.gd (new),
autoload/router.gd                                                               model (Task 10)
ui/npc_window.gd, ui/siege_panel.gd (new), ui/siege_result.gd (new),
ui/castle_window.gd (new), ui/guild_window.gd, ui/stall_setup.gd,
ui/entity_view.gd, ui/hud.gd, screens/world.gd                                   UI (Task 11)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md         tests, smoke (Tasks 9–12)
```

---

### Task 1: Server — siege maps, monsters and data

**Files:** `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`, `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `docs/balance/event_data.py`, `data/events.json` (generated), `internal/gamedata/siege.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `mapdata.Region.RespawnSec int` (0 = rank default); zone spawn timers use it when set.
- `gamedata.SiegeConfig{FirstBossSec, BossGapSec, MapBossSec int; Maps []SiegeMap; MonsterPoints map[string]int; PlayerKill{Points, PairSec int}; Rewards SiegeRewards}`, `SiegeMap{Map string; LimitSec int; Bosses []SiegeBoss}`, `SiegeBoss{Monster string; Level, HPMul, Points int; Final bool}`, `SiegeRewards{Participation, Milestones []Milestone, PersonRanks, GuildRanks []RankBand, GuildFund []int64}` (each reward a `RewardPack{Xu int64; ExpPct int; Stacks []StackReward; Gems []GemReward{Grade, Count}}`); `Event.Siege *SiegeConfig` (set for kind `siege`).

**Behaviour:** §2.1–2.3. mapgen: five maps by the spec table, forward/back portals between consecutive maps only, `entry` safe zone with the only respawn point, `boss_arena` disjoint from regions; `su_gia_vht` in Ubuyashiki. Bosses' skills exactly as §2.2 within 7d's schema (reuse a kind id if 7f already declared it).

**TODO / tests:**
- [ ] Scripts: write and `--check` clean (mapgen, monster_stats, event_data); monster_stats prints event boss HP after `hp_mul`.
- [ ] `// ____TEST SIEGE MAPS____` (mapdata): `respawn_sec` bounds; the five real maps per spec §8 "mapdata".
- [ ] `// ____TEST SIEGE____` (gamedata): real data loads; every rejection of spec §8 "gamedata".
- [ ] Commit: `Generate the Infinity Castle maps, bosses and siege data`.

### Task 2: Server — castle storage, the tax op and devcastle

**Files:** `migrations/00013_castle.sql` (new), `internal/store/castle.go` (new), `internal/store/characters.go` (op dispatch), `internal/store/store_test.go`, `cmd/devcastle/main.go` (new), its test.

**Interface:**
- `store.Castle{GuildID int64 (0 = none); TaxBP int; Since, TaxChangedAt time.Time}`; `GetCastle(ctx) (Castle, error)`; `SetCastleTax(ctx, guildID int64, bp int, now time.Time) (bool, error)`; `ClearCastleOwner(ctx, guildID int64) error`.
- `store.SiegeResult{StartedAt, EndedAt time.Time; DemonKingKilled, Captured bool; WinnerGuildID int64; Guilds, Persons json.RawMessage}`; `SaveSiegeResult(ctx, r SiegeResult) error`.
- `store.GuildFundTax{GuildID int64; Xu int64}` implementing 7b's `store.Op`.
- `devcastle -guild <name> | -tax <bp> | -clear`.

**Behaviour:** §3. `SaveSiegeResult` resets the tax to 0 only when the owner guild changes. The op skips silently when the guild row is gone.

**TODO / tests** (`// ____TEST CASTLE____`, DB):
- [ ] Every case of spec §8 "store"; devcastle sets/clears; unknown guild writes nothing.
- [ ] Commit: `Store the castle owner, siege results and the guild tax op`.

### Task 3: Server — the siege service

**Files:** `internal/siege/siege.go`, `internal/siege/rank.go`, `internal/siege/rewards.go` (new), `internal/siege/siege_test.go` (new).

**Interface:**
- `type Deps struct { Now func() time.Time; Events func() *event.Snapshot; Guilds GuildView (MemberInfo, Exists, AddFund(ctx, guildID uint64, xu int64) error); Rewards interface{ Grant(reward.Grant) }; Chat Announcer (System); Store CastleStore; Exp *gamedata.ExpTable; Log *slog.Logger }`.
- `func New(d *gamedata.Siege, deps Deps, castle store.Castle) *Service`; `func (s *Service) Run(ctx)` (1 s loop); `func (s *Service) Step(now time.Time)` (one loop body, for tests); `func (s *Service) Snapshot() *Snapshot`.
- Zone-facing (short lock, no I/O): `BossDied(run uint64, mapIdx, bossIdx int)`, `MonsterKill(run uint64, killer Scorer, kind string)`, `PlayerKill(run uint64, killer, victim Scorer, now time.Time)`, `Rank(charID int64, guildID uint64) (myPts int64, myRank int, gPts int64, gRank int)`.
- Gateway-facing: `Info(now) *pb.SiegeState`, `Castle(now) *pb.CastleInfo`, `SetTax(charID int64, bp int, now time.Time) (*pb.CastleInfo, error)` (does the DB write on the caller's goroutine; errors `ErrNotOwner`, `ErrBadTax`, `ErrTaxCooldown`).
- `Snapshot{Active bool; Run uint64; Maps []MapView{Open bool; LeftMS int64; Boss *BossSpec{Map, Index int; Kind string; Level, HPMul int}; BossInMS int64; BossesLeft int}; Persons, Guilds []Row; Owner uint64; TaxBP int; Result *Result}`.

**Behaviour:** §4.1, §4.5–4.7. Ranking order: points desc, reached-at asc, id asc. End: results → `SaveSiegeResult` with retry and backoff (cap 30 s), only then swap the owner; rewards per §2.3 (one `reward.Grant{CharID, Source, Title, Xu, Exp, Items}` per reward line with the sources of spec §2.3 — `vo_han_thanh:<run>:join`, `:milestone:<n>`, `:rank`, `:guild_rank` — gems and stones as `items.Item`, gems rolled with the service's own seeded rng); fund bonuses; chat lines. The snapshot is replaced, never mutated.

**TODO / tests** (`// ____TEST SCHEDULE____`, `// ____TEST SCORES____`, `// ____TEST END AND REWARDS____`, `// ____TEST TAX____`):
- [ ] Every case of spec §8 "siege" and "siege thuế" with a fake clock, fake event snapshot, fake guild view, fake granter and fake store.
- [ ] Commit: `Add the Infinity Castle siege service`.

### Task 4: Server — protocol 23

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 225–234; `SiegeMap`, `SiegeState` (embeds 7f's `Scoreboard` as `board = 3` and a `ScoreRow my_guild = 4`), `SiegeResult`, `CastleInfo`; `EnterWorldResp.siege = 20`, `.castle = 21`; `EntityEnter.castle_owner = 19`; `LookChange.castle_owner = 6`; errors 105–109 with one-line comments; `ProtocolVersion = 23`.

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `siege_state`, `siege_result`, `castle_info`, `entity_enter_castle`; regenerate all with `-update`; version test → 23.
- [ ] Commit: `Add the phase 7h siege and castle messages and bump the protocol to 23`.

### Task 5: Server — siege maps, scores, castle look and stall tax in the zone

**Files:** `internal/zone/siege.go` (new), `internal/zone/zone.go`, `internal/zone/combat.go`, `internal/zone/loot.go`, the 7b stall file in `internal/zone`, `internal/zone/zone_test.go`.

**Interface:**
- `zone.SiegeMap{Index int; Service SiegeHooks}` with `SiegeHooks` = the zone-facing methods of Task 3 plus `Snapshot()`; `Config.Siege *SiegeMap`; 7b's `Config.StallTax func() (guildID uint64, bp int)` is filled from the castle snapshot (owner id non-zero whenever there is an owner, even at 0 tax) and is the only castle hook the zone reads.
- Zone state: `siegeBoss *monster`, `siegeBossKey (map, index)`, `lastOwner uint64`, `siegeResultSent uint64`.
- `func (z *Zone) runSiege()` (once per second), `func (z *Zone) siegeState(p *player) *pb.SiegeState`.

**Behaviour:** §4.4, §4.6. Spawn with `SpawnSpecial(SpecialSpawn{Kind, Level, Pos: centre of boss_arena, Home: [boss_arena], HPMul, Aggressive: true, Tag: "vo_han_thanh:<run>:<map>:<i>", OnDeath → BossDied})`; remove a boss the snapshot no longer wants with `DespawnTag` (no loot, no EXP, no points). `MonsterKill` beside quest credit; `PlayerKill` in `killPlayer` when the killer is a player. `SiegeState` every 2 s, `SiegeResult` once per run. Stall sale: tax from `Config.StallTax`, `GuildFundTax` op in the linked save when tax > 0, `AddFundCached`. Owner change → `LookChange` for players whose `castle_owner` flips; `EntityEnter.castle_owner` from the guild cell and the owner id.

**TODO / tests** (`// ____TEST SIEGE____`, `// ____TEST STALL TAX____`):
- [ ] Every case of spec §8 "zone" and "zone thuế sạp" with a fake `SiegeHooks`.
- [ ] Commit: `Run siege bosses, scores, the castle mark and stall tax in the zone`.

### Task 6: Server — siege transfers, portal lock and wiring

**Files:** `internal/world/world.go`, `internal/world/world_test.go`, `internal/app/app.go` (or wherever services are built).

**Interface:** `func (w *World) SiegeEnter(ctx, m *Membership, seq uint32, npc string) (*Membership, error)`, `func (w *World) SiegeLeave(ctx, m, seq)`; world gets a `Siege` view (snapshot, guild `MemberInfo`); zones of `vht_*` maps get `Config.Siege`; every zone gets `Config.StallTax` from the siege snapshot; app starts `siege.Service.Run` and stops it before the saver flush on shutdown.

**Behaviour:** §4.2, §4.3. World steps 1–3 before touching a zone; zone check function for `DEAD`, NPC, range; `LEVEL_TOO_LOW` from 7f's last transfer step (`min_level: 50` of the siege maps); destination `default` of the frontier map. `PORTAL_LOCKED` for forward siege portals when the target map is not open. `SiegeLeave` to Ubuyashiki `default`, alive at full HP/MP when dead.

**TODO / tests** (`// ____TEST SIEGE TRANSFERS____`):
- [ ] Every case of spec §8 "world".
- [ ] Commit: `Move players into and out of the Infinity Castle`.

### Task 7: Server — gateway, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/errors.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** gateway cases for 225, 226 (transfers; replace `c.member`), 228, 231 (answered from the service), 233 (`SetTax` on the connection goroutine); siege errors → codes 105–109; `client.SiegeEnter`, `SiegeLeave`, `SiegeInfo`, `CastleInfo`, `CastleTax`; the helper records `SiegeState` and `SiegeResult`.

**TODO / tests:**
- [ ] Gateway: 225, 226, 233 before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestSiegeOverWebSocket` per spec §8 (dev run `vo_han_thanh` with 7e's `Config.DevDelay` 1 s and `DevMinutes` 1; guilds created through 7c's store before the test server starts).
- [ ] Commit: `Handle siege and castle requests and add an end-to-end siege test`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`, `docs/superpowers/specs/2026-11-20-giai-doan-7-tong-quan.md`.

**TODO:**
- [ ] game-design 6.3, 7.2, 10.3, 11 per §9; phase 0 spec 1.1/3.3 and the overview per §9.
- [ ] CLAUDE.md invariants per §9; README: the siege, castle tax, `devcastle`.
- [ ] Commit: `Document the phase 7h Infinity Castle siege and castle owner`.

### Task 9: Client — sync and protocol 23

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 8; `data/events.json` already syncs since 7e (it now carries `vo_han_thanh`); `PROTOCOL_VERSION := 23`; `PAYLOADS` adds the new payloads; the five error texts of §6.1.

**TODO / tests:**
- [ ] Decode the four new fixtures.
- [ ] Commit: `Sync the phase 7h protocol and siege data from the server`.

### Task 10: Client — siege board and castle model

**Files:** `core/siege_board.gd` (new), `core/castle.gd` (new), `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (functions as listed; `Router.castle`, `Router.siege`).

**TODO / tests** (`# ____TEST SIEGE____`):
- [ ] Every client case of spec §8 (`map_line`, `my_line`, `result_text`, `net_of`, `can_set`).
- [ ] Commit: `Add the siege board and castle models`.

### Task 11: Client — siege and castle UI

**Files:** `ui/npc_window.gd`, `ui/siege_panel.gd` (new), `ui/siege_result.gd` (new), `ui/castle_window.gd` (new), `ui/guild_window.gd`, `ui/stall_setup.gd`, `ui/entity_view.gd`, `ui/hud.gd`, `screens/world.gd`.

**Behaviour (§6.2):** NPC window for `vo_han_thanh`; siege panel with the scoreboard window and Rời; result panel; castle window (tax slider for the owner's leader); owner mark; stall tax line (`CastleInfoReq` on opening the stall setup); locked portals report once and wait for `SiegeState`. `world.gd` only wires.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server started with `EVENT_DEV_START=vo_han_thanh EVENT_DEV_MINUTES=60`, two guilds made with `devguild`, level-90 characters. Screenshots in the report.
- [ ] Commit: `Show the Infinity Castle panel, scoreboard, result and castle window`.

### Task 12: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3).** Print `SMOKE OK (… siege points 1)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: Vô Hạn Thành, thành chủ, thuế sạp.
- [ ] Commit: `Extend the smoke test with an Infinity Castle entry`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID); migration 00013 adds an empty castle row.
- [ ] Tell the user what to try (spec §8 "Nghiệm thu"); phase 7 is complete.
