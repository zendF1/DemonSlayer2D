# Phase 7f Thảo Phạt Thượng Huyền and Reward Mailbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single-zone battle map, Hoang Sơn Chiến Địa (level ≥ 50, 150 players), north of Ubuyashiki. At 13:00 and 18:00 for 30 minutes a level-80 Thượng Huyền (three rotating by day, 31.2 M HP) spawns in its arena; guilds fight it while PvP stays on (no EXP loss; same party/guild protected). A live damage scoreboard (players and guilds) refreshes every 2 s. On death or timeout, rank rewards, a killer reward and a guild fund bonus go out through a new reward mailbox (`character_rewards`) that works for offline players and is claimed atomically with the character save.

**Architecture:**
- Data: `event_data.py` adds the `world_boss` event; `monster_stats.py` adds `tp_daki`, `tp_gyokko`, `tp_hantengu` (boss, `event_only`); `mapgen.py` adds `hoang_son`, the Ubuyashiki north portal and the map fields `max_zones`, `capacity`, `min_level`.
- World: `reserve` honours `max_zones`/`capacity`; every transfer check ends with the destination `min_level`; `Enter` falls back to the town for blocked maps and registers the reward inbox before loading rewards.
- Zone: a `world_boss` event handler (7e's zone event framework) spawns the boss with 7d's special spawn (`HPMul`, `ExpMul`), records counted damage per character and guild in `applyToMonster`, pushes `Scoreboard`, and hands one `event.Result` to the event service via `zone.Config.EventResult`. Reward pending list per player, inbox drain each second, `RewardClaimReq`.
- Event service: turns a result into rewards (bands, timeout halving, killer, guild `AddFund`), ends the run early, announces.
- New `internal/reward`: in-memory queue + own writer goroutine (`store.InsertReward`, retry/backoff, flush on stop), per-session `Inbox`.
- Store: migration 00012 (`character_rewards`), `InsertReward`, `PendingRewards`, `PruneRewards`, the `ClaimRewards` save op (7b's `store.Op`), `DevGrantReward`.
- Protocol 21: envelopes 200–203, `RewardInfo`, `RewardBox`, `ScoreRow`, `Scoreboard`, `EnterWorldResp.rewards = 18`, `SelfStats.rewards = 26`, `REWARD_NOT_FOUND = 94`.
- Dev: `EVENT_DEV_MINUTES`, `cmd/devreward`.
- Client: `core/reward_box.gd`, `core/scoreboard.gd`, min-level aware `world_graph`/`portal_watch`, reward window, scoreboard panel, HUD badge.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7f-thao-phat-thuong-huyen-design.md](../specs/2026-11-20-giai-doan-7f-thao-phat-thuong-huyen-design.md). Section references below (§n) point to it. Overview: [Phase 7](../specs/2026-11-20-giai-doan-7-tong-quan.md).

**Prerequisite:** the [phase 7e plan](2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy.md) is fully implemented (protocol 20, migration 00011; `internal/event`, the zone event handlers, `EVENT_DEV_START`), on top of 7a–7d (chat SYSTEM channel, 7b's `store.Op`/`SaveLinked` and trade locks, 7c guild service with `AddFund` and the guild Tag cell, 7d boss rank, monster skills and special spawn). Start from HEADs that contain them.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–6d and 7a–7e are designed but not implemented)

Find each symbol by name (`graphify explain "<symbol>"`) before editing; later phases move most lines.

- `data/monsters.json` level 80 row (current script output): HP 12,466, attack 1,344, defense 640, EXP 11,095. With 7d's boss factors (HP ×20, attack ×2, defense ×1.5, EXP ×25) the boss is 249,320 HP; `hp_mul` 125 gives 31,165,000 (§1.3). Re-check the numbers after 7d regenerates `monsters.json` and update the spec table if 7d changed a factor.
- Monster damage lands in `(*Zone).applyToMonster(im impact)` (`internal/zone/impact.go:79`): the counted damage (clamped to HP left, and by 7d to max HP minus counted total) is computed there before EXP; record the scoreboard right after that value is known, only for player attackers still in the zone.
- `(*Zone).killMonster` (`internal/zone/combat.go:216`) → `loot` (`internal/zone/loot.go:30`) → 7d death hook: the boss's own rank loot keeps going to the killer; the world-boss handler runs in the hook after `loot`.
- `World.reserve` (`internal/world/world.go:408`) picks the first zone with room or opens a new one via `openZoneLocked` (`:166`); 4a adds `min_zones`; add the `max_zones` stop and the per-map `capacity` there. `World.Transfer` (4a §3.2) owns the check order; the zone-side check function is per transfer kind.
- `saver.Saver` (`internal/saver/saver.go`) batches a map of entries; after 7b, entries carry `Ops` that are appended on coalesce and kept on requeue. `ClaimRewards` is one more `store.Op` kind executed in `SaveCharacterStates`' transaction after the character rows.
- The reward writer must mirror the saver's retry/backoff (`MinBackoff` 1 s, `MaxBackoff` 30 s) and its "flush under its own timeout on shutdown" invariant (CLAUDE.md).
- `items.NewEquipment(rng, d, code, quality)` (`internal/items/roll.go`) builds a piece; 3a's line roll (`internal/items`) adds lines for quality ≥ 1 — reuse the same functions for reward equipment; equipment codes are `eq<tier>_<slot>` and `eq<tier>_weapon_<class>`.
- 3a's item row validation in `world.badItem` (`internal/world/world.go:355`) is the check to reuse for reward items (without the slot rules).
- `tools/mapgen.py` (4a) owns portal layout rules (west/east only today); extend them for north/south portals and keep its two-way portal and connectivity checks.

## File structure

Server (`DemonSlayer2D`):

```
tools/mapgen.py, maps/*.json (gen), internal/mapdata/*                 hoang_son, map fields (Task 1)
docs/balance/event_data.py, data/events.json (gen),
docs/balance/monster_stats.py, data/monsters.json (gen),
internal/gamedata/events.go, gamedata_test.go                          world_boss data, bosses (Task 1)
internal/world/world.go, world_test.go                                 single zone, min level (Task 2)
migrations/00012_rewards.sql (new), internal/store/rewards.go (new),
internal/store/characters.go, internal/store/store_test.go             reward storage, ClaimRewards op (Task 3)
internal/reward/reward.go (new), reward_test.go (new)                  reward service (Task 3)
cmd/devreward (new), internal/config/config.go                         dev tools (Task 3)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures             protocol 21 (Task 4)
internal/zone/reward.go (new), zone.go, transfer.go, zone_test.go      mailbox in the zone (Task 5)
internal/zone/event_boss.go (new), impact.go, zone_test.go             world boss run, scoreboard (Task 6)
internal/event/service.go, results.go (new), event_test.go             rewards from results (Task 6)
internal/gateway/handlers.go, internal/client/client.go, e2e/          wiring, e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, 4a spec                     docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/                    sync (Task 9)
core/wire.gd, core/errors.gd, core/reward_box.gd (new),
core/scoreboard.gd (new), core/world_graph.gd, core/portal_watch.gd,
autoload/router.gd                                                      model (Task 9)
ui/reward_window.gd (new), ui/scoreboard_panel.gd (new), ui/hud.gd,
ui/map_window.gd, screens/world.gd                                     UI (Task 10)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md   tests, smoke (Tasks 9–11)
```

---

### Task 1: Server — Hoang Sơn, map fields and world-boss data

**Files:** `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`, `docs/balance/event_data.py`, `data/events.json`, `docs/balance/monster_stats.py`, `data/monsters.json`, `internal/gamedata/events.go`, `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `mapdata.Map.MaxZones int` (0 = no limit), `Capacity int` (0 = config default), `MinLevel int` (0/1 = none).
- `gamedata.WorldBossConfig{Map string; Spawn Point; Arena Rect; Bosses []string; Level, HPMul, ExpMul, BoardPlayers, BoardGuilds, BoardSec, ParticipationBP int; Bands []RewardBand; Killer RewardBand; GuildFundXu int64; TimeoutPct int}`; `RewardBand{From, To int; Xu int64; Stacks []StackReward; Equipment *EquipReward{Tier, Quality int}}`; `Event.WorldBoss *WorldBossConfig`.
- `func (c *WorldBossConfig) BossFor(start time.Time) string` (`quest.Day` mod len).

**Behaviour:** §2.1–2.3. Map checks of §8 "mapdata" (per map in `Parse`, the graph rules in the script and the real-maps test). `Validate(maps)` checks the `world_boss` cross rules (battle map, spawn/arena, boss rank and `event_only`).

**TODO / tests** (`// ____TEST MAPS____` in mapdata, `// ____TEST EVENTS____` in gamedata):
- [ ] Scripts: `hoang_son` + Ubuyashiki north portal; three bosses; the event; all `--check` clean; `event_data.py` prints the boss numbers of §1.3.
- [ ] Every rejection of spec §8 "mapdata" and "gamedata"; real data loads.
- [ ] Commit: `Add Hoang Sơn, single-zone map fields and the Thảo Phạt data`.

### Task 2: Server — single-zone maps and entry level in the world

**Files:** `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/transfer.go` (the per-kind check functions).

**Behaviour:** §4.1. `reserve` stops at `max_zones` and uses the map capacity; `Transfer` returns `ZONE_FULL` before zone checks; every kind's zone check ends with `LEVEL_TOO_LOW` against the destination `min_level`; `Enter` places blocked characters at the `default` of their map's `town`.

**TODO / tests** (`// ____TEST ZONES AND TRANSFER____`):
- [ ] Full single-zone map → `ZONE_FULL`, no zone 2; level 49 portal → `LEVEL_TOO_LOW`, 50 passes; return scroll and carriage into a min-level map obey the same check; saved position in Hoang Sơn at level 40 → Ubuyashiki `default`.
- [ ] Commit: `Cap single-zone maps and check the destination level on every transfer`.

### Task 3: Server — reward storage, service and dev tool

**Files:** `migrations/00012_rewards.sql` (new), `internal/store/rewards.go` (new), `internal/store/characters.go`, `internal/store/dev.go`, `internal/store/store_test.go`, `internal/reward/reward.go` (new), `internal/reward/reward_test.go` (new), `cmd/devreward/main.go` (new), its test, `internal/config/config.go`.

**Interface:**
- `store.RewardRow{ID, CharID int64; Source, Title string; Xu, Exp int64; Items []ItemRow /* character_items columns, slot unused */; CreatedAt time.Time}`.
- `func (s *Store) InsertReward(ctx, r RewardRow) (id int64, inserted bool, err error)`; `PendingRewards(ctx, charID int64, since time.Time) ([]RewardRow, error)`; `PruneRewards(ctx, before time.Time) (int64, error)`; `DevGrantReward(ctx, charID int64, r RewardRow) error`.
- `store.ClaimRewards{CharID int64; IDs []int64}` implementing 7b's `store.Op`.
- `reward.Grant{CharID int64; Source, Title string; Xu, Exp int64; Items []items.Item}`; `reward.Reward` (id + the same fields + `CreatedAt`).
- `type Inbox struct` with `Push(Reward)`, `Drain() []Reward` (mutex-guarded).
- `func New(st Store, log *slog.Logger) *Service`; `Run(ctx)`; `Stop()`, `Done()`; `Register(charID int64) *Inbox`; `Unregister(charID int64)`; `Grant(g Grant)` (never blocks).
- Config `EVENT_DEV_MINUTES` (1–720, 0 = event default) passed to 7e's `event.Config.DevMinutes`; server start calls `PruneRewards(now − 30 days)`.

**Behaviour:** §3, §4.4. The writer converts items to rows, retries with backoff, skips duplicates without pushing, pushes to registered inboxes after insert, flushes on stop within 10 s.

**TODO / tests** (`// ____TEST REWARDS____` in store with DB; `// ____TEST SERVICE____` in reward):
- [ ] Every store case of spec §8 "store" (migration up/down, duplicate source, pending filter, prune, `ClaimRewards` in the same transaction, forced failure leaves rows unclaimed, cascade on delete).
- [ ] Every reward case of spec §8 "reward" with a fake store and fake clock.
- [ ] `devreward`: one row; unknown code writes nothing.
- [ ] Commit: `Store rewards and add the reward mailbox service`.

### Task 4: Server — protocol 21

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 200–203; `RewardInfo`, `RewardBox`, `ScoreRow`, `Scoreboard`; `EnterWorldResp.rewards = 18`; `SelfStats.rewards = 26`; `REWARD_NOT_FOUND = 94`; the `LEVEL_TOO_LOW` comment. `ProtocolVersion = 21`. `RewardClaimReq` not handled yet (default `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe (`reward_id`, not `id` clashes are fine; no `message`/`class` fields).
- [ ] The five fixtures of §5; regenerate all with `-update`.
- [ ] Version test → 21.
- [ ] Commit: `Add the phase 7f reward and scoreboard messages and bump the protocol to 21`.

### Task 5: Server — the reward mailbox in the zone

**Files:** `internal/zone/reward.go` (new), `internal/zone/zone.go`, `internal/zone/transfer.go`, `internal/zone/zone_test.go`, `internal/world/world.go`, `internal/world/world_test.go`.

**Interface:**
- `PlayerInfo.Rewards []reward.Reward`, `PlayerInfo.RewardInbox *reward.Inbox`; `player.rewards`, `player.inbox`.
- `func (z *Zone) RewardClaim(id uint64, seq uint32, rewardID int64) bool` (inbox command; false on a full inbox).
- `func (z *Zone) drainRewards()` once per second.
- `world.Enter`: `reward.Register` → `store.PendingRewards(now − 30 days)` → validate items (3a rules without slots) → `PlayerInfo`; leave unregisters.

**Behaviour:** §4.5. Error order DEAD → 7b trade/stall lock → `REWARD_NOT_FOUND` → `BAG_FULL` (clone first). Success applies items, Xu, `gainExp`, removes the entry and saves now with `store.ClaimRewards`. `SelfStats.rewards` counts pending; `EnterWorldResp.rewards` lists them; handoff copies the list and keeps the inbox pointer.

**TODO / tests** (`// ____TEST REWARDS____` in zone, `// ____TEST ZONES AND TRANSFER____` in world):
- [ ] Every zone case of spec §8 "zone hòm thưởng" (fake saver records the op).
- [ ] World: register-before-load ordering (a grant pushed between register and load appears once); bad item in a reward row dropped with the rest kept.
- [ ] Commit: `Claim mailbox rewards in the zone with an atomic save`.

### Task 6: Server — the world-boss run and its rewards

**Files:** `internal/zone/event_boss.go` (new), `internal/zone/impact.go`, `internal/zone/zone.go`, `internal/zone/zone_test.go`, `internal/event/service.go`, `internal/event/results.go` (new), `internal/event/event_test.go`.

**Interface:**
- `event.Result`, `event.ResultRow` (§4.2); `zone.Config.EventResult func(event.Result)`; `(*Service).Report(r Result)` (queue, brief lock).
- `type bossBoard struct` with `add(charID int64, name string, class pb.CharClass, guildID uint64, guildName string, points int64, tick uint32)`, `ranked() []event.ResultRow`, `guilds() []event.ResultRow`, `rows(n int, self int64) (top []*pb.ScoreRow, me *pb.ScoreRow)`.
- `type bossRun struct{ run event.Run; boss uint64; board bossBoard; done bool }`; handler `start`/`end`/`joined`; `onBossHit(m *monster, attacker *player, counted int64)`; `onBossDeath(m, killer)`.
- `event`: `func rewardsFor(cfg *gamedata.WorldBossConfig, d *gamedata.Data, res Result, rng *rand.Rand) (grants []reward.Grant, fund *FundBonus)`; the service's per-tick `processResults(now)`; `Config.Rewards interface{ Grant(reward.Grant) }`, `Config.Guilds interface{ AddFund(guildID uint64, xu int64) error }`; early end bookkeeping (`ended map[eventID+runID]bool`).

**Behaviour:** §4.2–4.3. Spawn with `HPMul`/`ExpMul`; record counted damage only for players still in the zone, with their guild at hit time; board every `board_sec` seconds; exactly one result per run (death and end on the same tick: first wins); timeout removes the boss without loot; the service converts results into grants, halves on timeout, adds the killer grant and the guild fund bonus, ends the run early and announces.

**TODO / tests** (`// ____TEST WORLD BOSS____` in zone, `// ____TEST RESULTS____` in event):
- [ ] Every zone case of spec §8 "zone Thảo Phạt" (test data with `hp_mul` 1 so a few hits kill the boss).
- [ ] Every event case of spec §8 "event phát thưởng" with fake reward and guild services and a seeded rng.
- [ ] Commit: `Run Thảo Phạt Thượng Huyền with a live scoreboard and rank rewards`.

### Task 7: Server — gateway, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** `Membership.RewardClaim(seq, rewardID) bool`; gateway case (needs `inWorld`, packet limit only, full inbox → `RATE_LIMITED`); `client.RewardClaim(ctx, id)`; the helper records `RewardAdded` and `Scoreboard`.

**TODO / tests:**
- [ ] Gateway: `RewardClaimReq` before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestRewardMailboxOverWebSocket` and `TestWorldBossOverWebSocket` per spec §8 (dev run of 1 minute starting after 1 s; `DevSetLevel`, `DevSetPlace`, guilds created through the 7c store helpers).
- [ ] Commit: `Handle reward claims and add end-to-end mailbox and world boss tests`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md`, `docs/superpowers/specs/2026-11-20-giai-doan-7-tong-quan.md`.

**TODO:**
- [ ] game-design 6.2, 5.1, 10.2; 4a map-field and portal-layout notes; CLAUDE.md invariants (rewards via `internal/reward`, claims saved with `ClaimRewards`, reward writer flushes on shutdown); README; overview row marked implemented.
- [ ] Commit: `Document Thảo Phạt Thượng Huyền and the reward mailbox`.

### Task 9: Client — sync, protocol 21 and the models

**Files:** `server` submodule, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `core/reward_box.gd` (new), `core/scoreboard.gd` (new), `core/world_graph.gd`, `core/portal_watch.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1; `PROTOCOL_VERSION := 21`; `Router.rewards`, `Router.scoreboard`; `world_graph.reachable(map, level)` takes the character level.

**TODO / tests** (`# ____TEST REWARDS____`, `# ____TEST SCOREBOARD____`):
- [ ] Decode the five fixtures; client cases of spec §8.
- [ ] Commit: `Sync the phase 7f protocol and add the reward and scoreboard models`.

### Task 10: Client — mailbox window, scoreboard panel and HUD badge

**Files:** `ui/reward_window.gd` (new), `ui/scoreboard_panel.gd` (new), `ui/hud.gd`, `ui/map_window.gd`, `screens/world.gd`.

**Behaviour (§6.2):** gift button with the `SelfStats.rewards` badge; reward cards with Nhận (locked when it does not fit) and Nhận tất cả (sequential, stops at the first error); scoreboard panel with player/guild tabs, own row, countdown and the final result; Hoang Sơn labels on the map; the min-level portal notice.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with `EVENT_DEV_START=thao_phat EVENT_DEV_MINUTES=3` and two level-80 characters; screenshots of the board and the mailbox in the report.
- [ ] Commit: `Show the reward mailbox and the event scoreboard`.

### Task 11: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3):** `devreward` before re-entering; claim it; re-claim → `REWARD_NOT_FOUND`. Print `SMOKE OK (… reward claimed)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: Hoang Sơn, Thảo Phạt, Hòm thưởng.
- [ ] Commit: `Extend the smoke test with a mailbox reward`.

---

## Acceptance (controller, after Task 11)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD with `EVENT_DEV_START=thao_phat EVENT_DEV_MINUTES=5` (kill only its recorded PID); migration 00012 applied.
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
