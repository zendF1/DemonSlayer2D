# Phase 7g Endless Train (Party Instance) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Two daily windows (15:00–16:00, 21:00–22:00 UTC+7). At the Trưởng tàu NPC in Ubuyashiki a party leader (or a solo player) of level 66+ opens a private copy of the `tau_vo_tan` map; party members board within 2 minutes. Four cars: stepping into a car starts its waves, a car opens only after the previous one is clear, the last car is event Enmu (HP × 4) whose Sóng mộng puts players to sleep for 1 s. Killing Enmu within 15 minutes gives everyone aboard a completion reward (to the bag, or to the reward mailbox when it does not fit); a timeout fails the run. One run per character per window.

**Architecture:**
- Data: `tools/mapgen.py` adds the generic `areas` and `event_only` map fields and `maps/tau_vo_tan.json`; `monster_stats.py` adds three normal kinds and the `enmu` boss with the `dream_wave` skill (new skill field `status`); `event_data.py` (7e) adds the `chuyen_tau` event of the new kind `train` to `data/events.json` with its config under the `train` key (`gamedata.Event.Train`); `mapdata` gets the new NPC role `event` (field `event`), first needed here and reused by 7h; `mapgen.py` adds the `truong_tau` NPC.
- New pure package `internal/train` (runs, per-window entry records, open/join rules).
- World: an `instances` table of private zones outside `zones`; three new transfer kinds (open, join, leave); `event_only` placement; `closeIdle` closes finished/empty instances.
- Zone: `Config.Train` turns on the train rules (`runTrain` step, car respawn, completion and failure); the generic sleep status (`player.sleepUntil`, `ASLEEP`, `StatusChange`).
- Protocol 22: envelopes 215–221, `TrainState`, `StatusKind`, `StatusChange`, `EnterWorldResp.train = 19`, `WorldTick.statuses = 10`, `Hit.status = 7`, errors 99–103.
- Client: `core/train_run.gd`, `core/status_table.gd`, sleep in the predictor and auto, NPC window, train banner, result panel.

**Tech stack:** Go, protobuf (buf), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md](../specs/2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md). Section references below (§n) point to it. Phase split and number ranges: [phase 7 overview](../specs/2026-11-20-giai-doan-7-tong-quan.md).

**Prerequisite:** phases 7a–7f are fully implemented (server protocol 21, migration 00012; 7d special spawn, monster skills and telegraphs; 7e event service, `data/events.json` with per-kind config keys, `EVENT_DEV_START`; 7f reward service (`reward.Service.Grant`), map fields `max_zones`/`capacity`/`min_level` with `LEVEL_TOO_LOW` as the last step of every transfer, `EVENT_DEV_MINUTES`). Start from HEADs that contain them.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–7f are designed, not yet implemented)

Phases 3b–7f land before this plan and move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**World**
- `World.zones map[string][]*slot` (`internal/world/world.go:61`) holds zone i+1 at index i; `slot{z, cancel, no, reserved atomic.Int32}` (`:68`). `openZoneLocked(mapID)` (`:166`) builds `zone.Config` and starts the zone goroutine under `w.wg`; add an `openInstanceLocked(runID, cfg)` beside it that stores the slot in a new `instances map[uint64]*slot` and never in `zones`.
- `reserve(mapID)` (`:408`) only walks `w.zones[mapID]`, so instances are invisible to it by construction; keep it that way and give the join path its own reservation on the run's slot (`reserved.Add(1)`, capacity = `max_members`).
- `closeIdle(now)` (`:434`) skips index 0 and slots with `reserved > 0` or `Count() > 0`; extend it with a second loop over `instances` (finished-and-empty, or the zone reports it ended itself after `empty_close_sec`).
- `placement(c)` (`:400`) is where the `event_only` rule goes (saved map is `event_only` → `default` of that map's `town`).
- 4a's `World.Transfer` (not yet in code) is the single transfer path; add kinds `trainOpen`, `trainJoin`, `trainLeave` with zone-side check functions like the portal kind.

**Zone**
- Tick order today (`internal/zone/zone.go:370`): `drain → runImpacts → resolveAttacks → (movePlayers) → thinkMonsters → runSpawnTimers → runGroundTimers → runPlayerTimers → syncView`. Insert `runTrain` after `runSpawnTimers` (§4.11).
- `killPlayer` (`internal/zone/combat.go:51`), `respawnPlayer` (`internal/zone/lifecycle.go:32`), `loot` (`internal/zone/loot.go:30`), `applyToMonster` (`internal/zone/impact.go:79`): the monster-death hook of 7d's special spawn fires from `applyToMonster`; the car-respawn rule goes into the respawn-point choice (4a turns `Area.Respawn` into a list).
- `Zone.Move` (`internal/zone/zone.go:274`) records the input; the sleep check belongs in the movement step, not in `Move`, so `ack_input_seq` keeps advancing.
- Player→player impacts and monster→player impacts share the impact queue (attack-phases spec); the sleep status is applied where a monster impact lands on a player.

**Data**
- `monsters.json` is generated by `docs/balance/monster_stats.py` (`KINDS` list, line ~44); never edit the JSON. Rank multipliers and the `skills` schema come from 7d.
- `items.NewEquipment(rng, d, code, quality)` (`internal/items/roll.go`) builds an item with rolled lines; use it for the completion equipment with quality from `quality_bp`.

## File structure

Server (`DemonSlayer2D`):

```
tools/mapgen.py, maps/tau_vo_tan.json (gen), maps/ubuyashiki.json (gen), internal/mapdata/*   areas, event_only, train map, NPC (Task 1)
docs/balance/monster_stats.py, data/monsters.json (gen), internal/gamedata/*                  train monsters, Enmu, skill status (Task 1)
docs/balance/event_data.py, data/events.json (gen), internal/gamedata/*                       train event kind (Task 1)
internal/train/train.go (new), train_test.go (new)                                            runs and entry rules (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures                                    protocol 22 (Task 3)
internal/zone/status.go (new), zone.go, move.go, combat.go, skill.go, inventory.go, impact.go  sleep (Task 4)
internal/zone/train.go (new), zone.go, lifecycle.go, loot.go                                  cars, waves, end (Task 5)
internal/world/world.go, world_test.go                                                        instances, transfers, placement (Task 6)
internal/gateway/handlers.go, internal/client/client.go, e2e/e2e_test.go                       wiring, e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 7 overview                                    docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/, tools/sync_server.sh     sync (Task 9)
core/wire.gd, core/errors.gd, core/train_run.gd (new), core/status_table.gd (new),
core/predictor.gd, core/auto_play.gd, autoload/router.gd                       model (Task 10)
ui/npc_window.gd, ui/train_banner.gd (new), ui/train_result.gd (new),
ui/entity_view.gd, ui/hud.gd, screens/world.gd                                 UI (Task 11)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md       tests, smoke (Tasks 9–12)
```

---

### Task 1: Server — map areas, train map, monsters and train data

**Files:** `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`, `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `docs/balance/event_data.py`, `data/events.json` (generated), `internal/gamedata/train.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `mapdata.Area{ID string; Rect}`; `Map.Areas []Area`, `Map.AreaByID(id) (Area, bool)`; `Map.EventOnly bool`.
- `gamedata.MonsterSkill.Status *SkillStatus{Kind string; MS int}`.
- NPC role `event`: `mapdata.NPC.Event string`, required for role `event` and forbidden for other roles; `gamedata.Validate(maps)` checks it names an event in `events.json`.
- `gamedata.TrainConfig{Map string; MaxMembers int; JoinSec, DurationSec, WaveDelaySec, EndGraceSec, EmptyCloseSec int; Cars []TrainCar; Reward TrainReward}`, `TrainCar{Area, Head, Spawn string; Waves [][]WaveGroup; Boss *TrainBoss}`, `WaveGroup{Monster string; MinLevel, MaxLevel, Count int}`, `TrainBoss{Monster string; Level, HPMul int}`, `TrainReward{Xu int64; Stacks []StackReward; Equipment TrainEquip{Tier int; QualityBP [5]int}}`; `Event.Train *TrainConfig` (set for kind `train`; 7e's per-kind key rule rejects a missing or extra key).

**Behaviour:** §2.1–2.4. mapgen: `areas` and `event_only` keys in the fixed key order; graph checks skip `event_only` maps; `tau_vo_tan` carries `max_zones: 1`, `capacity: 5`, `min_level: 66`; `truong_tau` "Trưởng tàu" role `event`, `event: "chuyen_tau"`, placed by 4b's NPC rules near Ubuyashiki `default`. The four new kinds have no `event_only` monster flag. The `enmu` kind carries `dream_wave` exactly as §2.3.

**TODO / tests:**
- [ ] Scripts: `--write` / `--write-json`, then `--check` clean for mapgen, monster_stats and event_data.
- [ ] `// ____TEST AREAS____` (mapdata): role `event` with/without `event`, `event` on another role; valid `areas`; rejected duplicate id, out of map, no walkable tile; `event_only` map unreachable from `kumotori_1` passes the real-dir graph test; real `tau_vo_tan` has four cars and four respawn points inside the car heads.
- [ ] `// ____TEST TRAIN____` (gamedata): real data loads; every rejection of spec §8 "gamedata"; `enmu` is a boss with `dream_wave` and a sleep status of 1000 ms.
- [ ] Commit: `Generate the Endless Train map, monsters and run data`.

### Task 2: Server — pure train runs

**Files:** `internal/train/train.go` (new), `internal/train/train_test.go` (new).

**Interface:**
- `type Run struct { ID, Window uint64; Party uint64; Opener int64; OpenedAt, JoinUntil, EndsAt time.Time; Entered map[int64]bool; Ended bool }`.
- `type Service struct` with `New(d *gamedata.Train)`; `Open(window, party uint64, char int64, now time.Time) (*Run, error)` (`ErrRunExists`, `ErrAlreadyRan`), `CanJoin(window, party uint64, char int64, now time.Time) (*Run, error)` (own run first; `ErrAlreadyRan`, `ErrJoinClosed`), `Entered(r *Run, char int64)`, `Ended(r *Run)`, `Info(window uint64, char int64, party uint64, now time.Time) Info`.
- Errors map to the protocol codes in the gateway (Task 7); solo runs use party key = `-(char id)` internally so a solo player and a party never collide.

**Behaviour:** §1.2 "Ai mở", "Ai vào", "Số lượt", "Một tổ, một lượt"; §4.1. A new `window` id clears the entry records. One lock, no I/O.

**TODO / tests** (`// ____TEST RUNS____`):
- [ ] Every case of spec §8 "train".
- [ ] Commit: `Add the pure Endless Train run rules`.

### Task 3: Server — protocol 22

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 215–221; `TrainPhase`, `TrainState`, `StatusKind`, `StatusChange`; `EnterWorldResp.train = 19`, `WorldTick.statuses = 10`, `Hit.status = 7`; errors 99–103 with one-line comments; `ProtocolVersion = 22`. Train requests are not handled yet (default `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `train_state`, `train_result`, `world_tick_sleep`, `enter_world_resp_train`; regenerate all with `-update`; version test → 22.
- [ ] Commit: `Add the phase 7g train and status messages and bump the protocol to 22`.

### Task 4: Server — the sleep status

**Files:** `internal/zone/status.go` (new), `internal/zone/zone.go`, `internal/zone/move.go`, `internal/zone/combat.go`, `internal/zone/skill.go`, `internal/zone/inventory.go`, `internal/zone/impact.go`, `internal/zone/zone_test.go`.

**Interface:**
- `player.sleepUntil uint32`; `func (z *Zone) applyStatus(p *player, s *gamedata.SkillStatus) pb.StatusKind` (sets the end tick, queues a `StatusChange` start when the player was awake); `func (p *player) asleep(tick uint32) bool`.
- Per-tick `statusChanges []statusEvent` flushed in `syncView` into `WorldTick.statuses` for viewers of the entity and the entity itself.

**Behaviour (§4.6):** apply at the monster impact on a player (also sets `Hit.status`); the movement step leaves a sleeping player in place while still recording input and acks; `AttackReq`, `CastReq`, `UseItemReq`, `PickupReq` return `ASLEEP` right after their `DEAD` step; the end of sleep is detected in `runPlayerTimers` and queues `StatusChange{remaining_ms: 0}`; death clears sleep; the handoff carries nothing.

**TODO / tests** (`// ____TEST STATUS____`):
- [ ] A monster skill with a 1000 ms sleep: the target does not move for 10 ticks, then moves; `ack_input_seq` still advances while asleep.
- [ ] The four commands return `ASLEEP` while asleep and work after; `DEAD` still wins over `ASLEEP`.
- [ ] A second sleep while asleep extends to the later end and sends no second start event; viewers get start and end `StatusChange`.
- [ ] Commit: `Add the sleep status for monster skills`.

### Task 5: Server — cars, waves and the end of a run in the zone

**Files:** `internal/zone/train.go` (new), `internal/zone/zone.go`, `internal/zone/lifecycle.go`, `internal/zone/loot.go`, `internal/zone/zone_test.go`.

**Interface:**
- `zone.TrainRun{ID uint64; Data *gamedata.TrainConfig; EndsAt uint32 (tick); OnEnd func(success bool)}`; `Config.Train *TrainRun`; `Config.Rewards interface{ Grant(reward.Grant) }` (7f's non-blocking `reward.Service`; a fake in tests).
- Zone train state: `car, wave int; nextWaveAt uint32; waveIDs map[uint64]bool; phase pb.TrainPhase; endedAt uint32`.
- `func (z *Zone) runTrain()`, `func (z *Zone) spawnWave(car, wave int)`, `func (z *Zone) finishTrain(success bool)`, `func (z *Zone) trainState() *pb.TrainState`, `func (z *Zone) Finished() bool` (atomic, read by world).

**Behaviour:** §4.5, §4.7, §4.8. Wave monsters and Enmu use 7d's `SpawnSpecial(SpecialSpawn{Kind, Level, Pos, Home: car rect, HPMul, Aggressive: true, Tag: "chuyen_tau:<run>", OnDeath})`; a failed run calls `DespawnTag`. Death hook of a run monster removes it from `waveIDs`; Enmu's death hook calls `finishTrain(true)`. Completion reward: clone bag, all-or-nothing, else `Rewards.Grant(reward.Grant{CharID, Source: "chuyen_tau:<run>", Title, Xu, Items})` with the whole reward. After `end_grace_sec` send `Kicked{ZONE_RESTART}` to remaining players and remove them like a disconnect. Empty for `empty_close_sec` while running → `finishTrain(false)`.

**TODO / tests** (`// ____TEST TRAIN____`, small in-test train data):
- [ ] Every zone case of spec §8 "zone" except the sleep cases (Task 4).
- [ ] `EnterWorldResp.train` carries the current state when a member boards mid-run.
- [ ] Commit: `Run the Endless Train cars, waves, Enmu and rewards in the zone`.

### Task 6: Server — instances and train transfers in the world

**Files:** `internal/world/world.go`, `internal/world/world_test.go`.

**Interface:**
- `World.instances map[uint64]*slot`; `func (w *World) openInstanceLocked(run *train.Run) *slot`.
- `func (w *World) TrainOpen(ctx, m *Membership, seq uint32, npc string) (*Membership, error)`, `TrainJoin(...)`, `TrainLeave(ctx, m, seq)`; `func (w *World) TrainInfo(m *Membership) train.Info`.
- World reads the current `chuyen_tau` window from 7e's event snapshot and the party id from the session's party cell.

**Behaviour:** §4.2–4.4, §4.9, §4.10. World checks (window, leader, run exists, already ran / join closed, capacity) run before any zone is touched; the zone-side check function covers `DEAD`, NPC, range; `LEVEL_TOO_LOW` comes from 7f's last transfer step with the map's `min_level`. Open the instance only after the old zone accepted the leave; on any later failure close it again and drop the run. `placement` handles `event_only`. `closeIdle` handles instances. Moving out of the train goes to `ubuyashiki` `default`; leaving while dead arrives alive at full HP/MP.

**TODO / tests** (`// ____TEST TRAIN INSTANCES____`):
- [ ] Every world case of spec §8 "world".
- [ ] The opener's party members receive the `chat.SystemTo` line once.
- [ ] Commit: `Open private train zones and move players in and out`.

### Task 7: Server — gateway, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/errors.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** gateway cases for 215, 216, 217 (need `inWorld`, packet limit only, replace `c.member` on success like other transfers) and 220 (answered from the world); `train` errors → codes 99–102; `client.TrainOpen`, `TrainJoin`, `TrainLeave`, `TrainInfo`; the helper records `TrainState`, `TrainResult` and `StatusChange`.

**TODO / tests:**
- [ ] Gateway: the three transfer requests before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestTrainOverWebSocket` per spec §8 (dev run `chuyen_tau` with 7e's `Config.DevDelay` of 1 s; `DevSetLevel`, `DevSetPlace` beside `truong_tau` from the generated map).
- [ ] Commit: `Handle train requests and add an end-to-end train test`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-7-tong-quan.md`.

**TODO:**
- [ ] game-design 5.10, 5.13, 6.2 per §9; overview status of 7g.
- [ ] CLAUDE.md invariants per §9; README: the train event and how to try it.
- [ ] Commit: `Document the phase 7g Endless Train`.

### Task 9: Client — sync and protocol 22

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 8; `data/events.json` already syncs since 7e (it now carries `chuyen_tau`); `PROTOCOL_VERSION := 22`; `PAYLOADS` adds the new payloads; the five error texts of §6.1.

**TODO / tests:**
- [ ] Decode the four new fixtures (`Hit.status`, `StatusChange`, `TrainState`, `TrainResult` with equipment).
- [ ] Commit: `Sync the phase 7g protocol and train data from the server`.

### Task 10: Client — train model and sleep

**Files:** `core/train_run.gd` (new), `core/status_table.gd` (new), `core/predictor.gd`, `core/auto_play.gd`, `core/entity_table.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`train_run.gd`, `status_table.gd` functions as listed; `predictor` takes an `asleep: bool` per step; `entity_table` routes `WorldTick.statuses` and `Hit.status` to `status_table`; `Router.train`).

**TODO / tests** (`# ____TEST TRAIN____`):
- [ ] `banner_text` for running, cleared car, boss car, success, failure; countdown from `remaining_ms`.
- [ ] `status_table`: sleep from `Hit.status`, end by `StatusChange` 0, expiry without an end event after the duration plus 0.5 s.
- [ ] Predictor stays put while asleep and matches the server replay; auto returns nothing while asleep.
- [ ] Commit: `Add the train run model and the sleep status`.

### Task 11: Client — train UI

**Files:** `ui/npc_window.gd`, `ui/train_banner.gd` (new), `ui/train_result.gd` (new), `ui/entity_view.gd`, `ui/hud.gd`, `screens/world.gd`.

**Behaviour (§6.2):** NPC window: add the `event` role and switch content by the NPC's `event`; content for `chuyen_tau` (sends `TrainInfoReq` on open; Mở chuyến tàu / Lên tàu buttons; level line); banner and Rời tàu with confirm; result panel with the automatic `TrainLeaveReq`; "Zz" over sleeping entities, dimmed controls while self is asleep. `world.gd` only wires.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server started with `EVENT_DEV_START=chuyen_tau EVENT_DEV_MINUTES=20` and two level-75 characters. Screenshots in the report.
- [ ] Commit: `Show the train NPC, banner, result and sleep`.

### Task 12: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3).** Print `SMOKE OK (… train car 1 wave 1)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: Chuyến Tàu Vô Tận.
- [ ] Commit: `Extend the smoke test with an Endless Train run`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID).
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
