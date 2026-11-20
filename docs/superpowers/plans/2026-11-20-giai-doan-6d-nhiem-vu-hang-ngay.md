# Phase 6d Daily Quests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Quạ truyền tin in Ubuyashiki gives three daily quests from level 10: kill 40 monsters within ±10 levels, collect 20 "Huyết quỷ" from them, enhance 3 times. Each can be done once per day and resets at 00:00 Vietnam time (UTC+7), also for players online at midnight. Rewards scale with the character's level (5% of a level in EXP, Xu per level, enhance stones). The tracker shows them with a countdown and walks to the nearest level-appropriate ground or to the blacksmith.

**Architecture:**
- Data: three `line: "daily"` quests in `quest_data.py` with the objective types `kill_any`, `collect_any`, `enhance` and the reward fields `exp_pct`, `xu_per_level`; `mapgen.py` adds the `qua_truyen_tin` quest NPC. `gamedata` validates the new rules.
- `internal/quest`: `Day`, `ResetIn`, daily availability and credit, level-scaled rewards, `Prune`.
- Zone: `Config.Now` (default `time.Now`), a per-second day check that prunes and pushes `QuestLog`; pruning on join; enhance attempts credit `enhance` quests; daily rewards at turn-in.
- Protocol 15: `QuestLog.daily_reset_ms = 2`, envelope 124 `QuestLog` push. No migration (`character_quests.day` exists since 00009).
- Dev: `devquest` writes today's day for daily quests and takes `-day-offset`.
- Client: daily parsing, rewards and goals in `core/quest_book.gd`, the daily section and countdown in the tracker.

**Tech stack:** Go, protobuf (buf), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md](../specs/2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 6c plan](2026-11-20-giai-doan-6c-tu-dong-danh.md) is fully implemented (and therefore 6b: protocol 14, migration 00009, `internal/quest`, `quests.json`, the quest UI). Start from HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 56e276b plus the phase 3–6c specs; client HEAD 5d275f7)

Phases 3–6c land before this plan. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

- `zone.Config` (`internal/zone/zone.go:38`) has no clock today; zones count ticks. Add `Now func() time.Time` there (nil → `time.Now`), set by the world from its own config so tests can inject a fake clock; never use wall time for anything but the quest day.
- The world builds each zone's `Config`; 5b already added a `Leveled` hook there — add `Now` beside it and pass it through `world.Config` (tests construct worlds directly).
- 3b's enhance loop (`EnhanceReq{item, protect, times}`) tries repeatedly until success, a drop, a lack of materials or `times`; credit the number of attempts it actually ran, after the loop, only on a successful command (no error code).
- 6b's `quest.Available(q, log, level)` gains the day; `quest.Credit` gains the killer and monster levels (`combat.ExpEligible`); keep the 6b call sites compiling in the same task.
- `syncView` order after 6b: `WorldTick`, `DeathNotice`, `QuestProgress`, `SelfStats`; send the reset `QuestLog` before `QuestProgress` of the same tick so a credit right after midnight lands on the fresh log.
- `ExpTable.ToNext(level)` is 0 at `MaxLevel` (5a finding), which gives the level-90 rule for free.
- Vietnam has no daylight saving time: a fixed +25,200 s offset is exact; do not load tzdata.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/quest_data.py, data/quests.json (gen)                daily quests (Task 1)
tools/mapgen.py, maps/ubuyashiki.json (gen)                       Quạ truyền tin (Task 1)
internal/gamedata/quests.go, gamedata_test.go                     validation (Task 1)
internal/quest/quest.go, quest_test.go                            day and daily rules (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures        protocol 15 (Task 3)
internal/zone/zone.go, quest.go, enhance code, transfer.go        clock, reset, credit, rewards (Task 4)
internal/world/world.go                                           Now in the zone config (Task 4)
cmd/devquest, e2e/e2e_test.go                                     dev, e2e (Task 5)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec           docs (Task 6)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/, core/wire.gd   sync (Task 7)
core/quest_book.gd, core/world_graph.gd, tests/unit/test_core.gd    model (Task 8)
ui/quest_tracker.gd, ui/quest_card.gd, screens/world.gd             UI (Task 9)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                     smoke (Task 10)
```

---

### Task 1: Server — daily quest data and Quạ truyền tin

**Files:** `docs/balance/quest_data.py`, `data/quests.json` (generated), `tools/mapgen.py`, `maps/*.json` (generated), `internal/gamedata/quests.go`, `internal/gamedata/gamedata_test.go`, `internal/mapdata/mapdata_test.go`.

**Behaviour (§2):** three daily quests after the main line; `Objective` types `kill_any`, `collect_any`, `enhance`; `Reward.ExpPct`, `Reward.XuPerLevel`; the line/objective/reward rules of §2; the script prints daily EXP at levels 10/30/50.

**TODO / tests:**
- [ ] Regenerate; `--check` clean.
- [ ] `// ____TEST QUESTS____`: 28 quests load; every rejection of spec §8 "gamedata"; `qua_truyen_tin` on `ubuyashiki` (update 6b's "one quest NPC per map" test: Ubuyashiki now has two).
- [ ] Commit: `Generate the daily quests and the Quạ truyền tin NPC`.

### Task 2: Server — day and daily rules

**Files:** `internal/quest/quest.go`, `internal/quest/quest_test.go`, the 6b call sites in `internal/zone/quest.go`.

**Interface:**
- `const DayOffsetSec = 7 * 3600`; `func Day(t time.Time) int32`; `func ResetIn(t time.Time) time.Duration`.
- `func Available(q *gamedata.Quest, log Log, level int, today int32) bool` — daily: no entry with `Day == today`; main as 6b.
- `func Credit(d *gamedata.Data, log Log, kind string, killerLevel, monsterLevel int, roll func(bp int) bool) []string` — adds `kill_any`/`collect_any` (§4.3).
- `func CreditEnhance(d *gamedata.Data, log Log, attempts int) []string`.
- `func Prune(d *gamedata.Data, log Log, today int32) bool` — drops daily entries of other days; true when anything was dropped.
- `func DailyReward(q *gamedata.Quest, level int, toNext int64) (exp, xu int64)` — `Round10(toNext, ExpPct)` and `Xu + XuPerLevel × level`.

**TODO / tests** (`// ____TEST DAILY____`):
- [ ] `Day` across 16:59:59/17:00:00 UTC; `ResetIn` at 17:00:00 UTC = 24 h, at 16:00:00 UTC = 1 h.
- [ ] `Available` daily: level 9 false, level 10 true, an entry of today false, of yesterday true.
- [ ] `kill_any` only within ±10 levels; `collect_any` with a roll stub; `CreditEnhance(…, 5)` caps at 3.
- [ ] `Prune` keeps main entries and today's dailies.
- [ ] `DailyReward(d_kill, 30, ToNext(30))` → 545,080 EXP and 2,400 Xu; level 90 (`toNext` 0) → 0 EXP.
- [ ] Commit: `Add the quest day and the daily quest rules`.

### Task 3: Server — protocol 15

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** `QuestLog.daily_reset_ms = 2`; Envelope 124 `QuestLog` (comment: pushed with seq 0 when the quest day changes); `ProtocolVersion = 15`.

**TODO / tests:**
- [ ] `make proto`; fixture `quest_log` (one active daily, 3,600,000 ms); regenerate all with `-update`; version test → 15.
- [ ] Commit: `Add the daily reset push and bump the protocol to 15`.

### Task 4: Server — clock, reset, credit and rewards in the zone

**Files:** `internal/zone/zone.go`, `internal/zone/quest.go`, the 3b enhance handler file, `internal/zone/transfer.go`, `internal/world/world.go`, `internal/zone/zone_test.go`, `internal/world/world_test.go`.

**Interface:** `zone.Config.Now func() time.Time`; `Zone.day int32`; `world.Config.Now` passed to every zone.

**Behaviour (§4):**
- Compute `day` at start and every `tickRate` ticks; on change, `Prune` each player and queue a `QuestLog{quests, daily_reset_ms}` for every player of the zone (sent in `syncView` before `QuestProgress`).
- Join (enter and handoff): `Prune` with the zone's day before building `EnterWorldResp`; fill `daily_reset_ms` from `ResetIn(Now())`.
- Accept a daily: §4.3 (entry `Day` = zone day). Credit via the new `Credit` signature at the loot site; `CreditEnhance` after a successful enhance command. Turn-in of a daily uses `DailyReward`.

**TODO / tests** (fake clock, `// ____TEST DAILY____`):
- [ ] Every bullet of spec §8 "zone (đồng hồ giả)".
- [ ] World: the zone config carries the injected clock.
- [ ] Commit: `Reset daily quests at midnight Vietnam time and credit kills and enhances`.

### Task 5: Server — devquest and e2e

**Files:** `cmd/devquest/main.go`, its test, `e2e/e2e_test.go`.

**Behaviour:** §7 (`day` for daily quests from `quest.Day(time.Now())` plus `-day-offset`); e2e `TestDailyQuestsOverWebSocket` per spec §8 (use `devitem`-style store helpers for the sword, stones and Xu; `DevSetPlace` beside `qua_truyen_tin` and `tho_ren` from the generated `maps/ubuyashiki.json`).

**TODO / tests:**
- [ ] `devquest -active d_kill` writes today; `-day-offset -1` writes yesterday.
- [ ] e2e green with the DB.
- [ ] Commit: `Support daily quests in devquest and add an end-to-end daily test`.

### Task 6: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 4.2, 2.2, 10.6 per §9; phase 0 roadmap: phase 6 done when 6d lands.
- [ ] CLAUDE.md: the quest day changes at 00:00 UTC+7 (`quest.Day`); zones use `Config.Now` only for the quest day. README: daily quests, `devquest -day-offset`.
- [ ] Commit: `Document the phase 6d daily quests`.

### Task 7: Client — sync and protocol 15

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 6; sync (also `data/exp.json` if not yet copied); `PROTOCOL_VERSION := 15`; `PAYLOADS` adds `quest_log`.

**TODO / tests:**
- [ ] Decode `quest_log.hex` (`daily_reset_ms` 3,600,000).
- [ ] Commit: `Sync the phase 6d protocol and daily quests from the server`.

### Task 8: Client — daily model and goals

**Files:** `core/quest_book.gd`, `core/world_graph.gd`, `tests/unit/test_core.gd`.

**Interface (§6.1):** `reset_in(now) -> float`; `reward_of` level-scaled for dailies; `goal_of` for `kill_any`/`collect_any`/`enhance` per §5.1; `tracker` daily section; `world_graph.regions_of(map_id)`, `npcs_with_role(role)`.

**TODO / tests** (`# ____TEST QUESTS____`):
- [ ] Every client bullet of spec §8 (rewards equal the server examples; goals at levels 12, 25, 60; enhance → `tho_ren` on `ubuyashiki`; tracker shows the daily section from level 10 only; countdown).
- [ ] Commit: `Add daily quests, their goals and the reset countdown to the quest model`.

### Task 9: Client — daily UI

**Files:** `ui/quest_tracker.gd`, `ui/quest_card.gd`, `screens/world.gd`.

**Behaviour (§6.2):** up to 6 tracker lines with the "Hàng ngày · làm mới sau h:mm" header; daily objective texts and level-scaled rewards on the card; a pushed `QuestLog` replaces `Router.quests`, refreshes markers and the tracker and notices "Nhiệm vụ hàng ngày đã làm mới".

**TODO:**
- [ ] Manual check against a :8299 server: accept the three dailies, tap Lệnh diệt quỷ → walks to a level-fitting ground, enhance three times, turn in, and watch the countdown. The midnight reset itself is covered by the fake-clock zone tests; no special server build is needed.
- [ ] Commit: `Show daily quests and the reset countdown in the tracker`.

### Task 10: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3):** place beside `qua_truyen_tin`; `daily_reset_ms` in (0, 86,400,000]; accept `d_enhance`; accepting again → `QUEST_NOT_AVAILABLE`. Print `SMOKE OK (… daily accepted, reset in h:mm)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: daily quests and the reset time.
- [ ] Commit: `Extend the smoke test with a daily quest`.

---

## Acceptance (controller, after Task 10)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID).
- [ ] Tell the user what to try (spec §8 "Nghiệm thu") and that phase 6 is complete.
