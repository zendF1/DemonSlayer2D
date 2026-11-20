# Phase 7e Event Framework and Lệnh Diệt Quỷ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A server-wide event schedule in Vietnam time (UTC+7) with System-chat announcements 5 minutes before, at the start and at the end, a HUD banner with a countdown and an event calendar window. The first event, Quạ Truyền Tin – Lệnh Diệt Quỷ, runs at 12:00, 19:00 and 21:30 for 20 minutes: every open zone of every training map spawns six "Quỷ bị truy nã" and one elite "Quỷ đầu đàn bị truy nã" in one monster region; they never respawn, give damage-share EXP and rank loot plus an event bonus, and vanish at the end.

**Architecture:**
- Data: `docs/balance/event_data.py` writes `data/events.json`; `monster_stats.py` adds `wanted_demon` and `wanted_leader`. `gamedata` loads and cross-validates events.
- New `internal/event`: pure schedule math (`Active`, `Next`, `RunID`) plus a world-level `Service` with its own one-second goroutine, System-chat announcements through `chat.Service.System`, `EventList` pushes to registered sessions and an immutable `atomic.Pointer[Snapshot]`.
- Zone: reads the snapshot once per second (same hook as 6d's day check) through `zone.Config.Events`, starts/ends runs locally; the `wanted` handler spawns through 7d's special-spawn API with a death hook that adds the bonus loot and updates `WantedGroupState`.
- Protocol 20: envelopes 190–193, `EventStatus`, `EventInfo`, `EventList`, `WantedGroupState`, `EnterWorldResp.events = 17`. No new error codes, no migration.
- Config: `EVENT_DEV_START` for dev runs 60 s after boot.
- Client: `core/event_book.gd` (in `Router.events`), `ui/event_banner.gd`, `ui/event_window.gd`, map window marks.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md](../specs/2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md). Section references below (§n) point to it. Overview: [Phase 7](../specs/2026-11-20-giai-doan-7-tong-quan.md).

**Prerequisite:** the [phase 7d plan](2026-11-20-giai-doan-7d-tinh-anh-thu-linh.md) is fully implemented (server protocol 19, migration 00011; monster ranks, monster skills, the special-spawn API) together with 7a (chat service, SYSTEM channel). Start from HEADs that contain them.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–6d and 7a–7d are designed but not implemented)

Phases 3b–7d land before this plan and move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

- The zone tick is `(*Zone).advance` (`internal/zone/zone.go`): `drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runGroundTimers → runPlayerTimers → syncView`. 6d adds a once-per-second day check from `Config.Now`; hang the event-snapshot check on the same once-per-second branch, before `runSpawnTimers`, so a spawned group is in that tick's `syncView`.
- `zone.Config` (`internal/zone/zone.go:38`) holds `NewID`, `Rand`, `TickRate`; 5b adds `Leveled`, 6d `Now`, 7a/7c their hooks. Add `Events func() *event.Snapshot` there; nil means no events (old tests).
- Kills go through `(*Zone).killMonster(m, killer)` (`internal/zone/combat.go:216`) which calls `(*Zone).loot(m, killer)` (`internal/zone/loot.go:30`). `loot` rolls Xu with `items.RollDrops` and adds ground items with `z.dropAt(pos, i, n)`; the bonus needs the Xu just rolled and the index after the rolled items — have `loot` return `(xu int64, n int)` or pass them to the 7d death hook. 7d's death hook is the extension point; do not add a second kill path.
- `world.New`/`World.Enter` (`internal/world/world.go:76`, `:272`) build `zone.Config` per zone (`openZoneLocked`, `:166`) and register sessions with party (5b) and chat (7a); follow that for `event.Service.Register`.
- Data scripts live in `docs/balance/` (`monster_stats.py`, `item_stats.py`, `exp_table.py`, `skill_stats.py`; 6b adds `quest_data.py`) with `--write-json`/`--print-json`/`--check`; Go tests compare the committed JSON with the script output. `gamedata.Parse` uses `DisallowUnknownFields`; `(*Data).Validate(maps)` is the cross-check hook.
- Time math must use the fixed +7 h offset like 6d's `quest.Day` (`internal/quest`); never `time.LoadLocation`.
- Client: `autoload/router.gd` keeps per-session models (`Router.party`, `Router.quests`); `core/wire.gd` `PAYLOADS` must list every new payload or `Wire.decode` rejects it.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/event_data.py (new), data/events.json (gen)            event data (Task 1)
docs/balance/monster_stats.py, data/monsters.json (gen)             wanted kinds (Task 1)
internal/gamedata/events.go (new), gamedata.go, gamedata_test.go    load, validate (Task 1)
internal/event/schedule.go (new), service.go (new), event_test.go   schedule, service (Task 2)
internal/config/config.go, cmd/gameserver/main.go, internal/app     EVENT_DEV_START, wiring (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures          protocol 20 (Task 3)
internal/zone/event.go (new), event_wanted.go (new), zone.go,
internal/zone/combat.go, loot.go, zone_test.go                      zone runs, Lệnh Diệt Quỷ (Task 4)
internal/world/world.go, world_test.go                              registration, EnterWorldResp (Task 4)
internal/gateway/handlers.go, internal/client/client.go, e2e/       wiring, e2e (Task 5)
docs/game-design.md, CLAUDE.md, README.md                           docs (Task 6)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/, tools/sync_server.sh   sync (Task 7)
core/wire.gd, core/event_book.gd (new), autoload/router.gd                  model (Task 7)
ui/event_banner.gd (new), ui/event_window.gd (new), ui/map_window.gd,
ui/entity_view.gd, ui/hud.gd, screens/world.gd                              UI (Task 8)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md    tests, smoke (Tasks 7–9)
```

---

### Task 1: Server — event data and wanted monsters

**Files:** `docs/balance/event_data.py` (new), `data/events.json` (generated), `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `internal/gamedata/events.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `event_data.py --write-json | --print-json | --check` (§2.1); prints each event's weekly windows and the run count per week.
- `gamedata.Event{ID, Name, Kind, Summary string; MinLevel int; Schedule []Slot; Wanted *WantedConfig}`; `Slot{Days []int; StartMin int /* minutes after 00:00 UTC+7 */; Minutes int}`; `WantedConfig{LateMinLeft int; Members []WantedMember; Bonus map[string]WantedBonus}`; `WantedMember{Monster string; Count int; Level string; LevelPlus int; AtNest bool}`; `WantedBonus{XuMul int; Stacks []StackReward}`.
- `Data.Events []Event` (file order), `Data.EventByID map[string]*Event`, `Data.AnnounceMin int`.
- Monster kinds `wanted_demon` (normal, `wanderer` behaviour) and `wanted_leader` (elite, same data as 7d's `wanderer_leader` including the `pounce` skill), both with the new optional kind field `event_only: true` (`gamedata.MonsterKind.EventOnly`).

**Behaviour:** §2.1–2.3. The weekly overlap check works on minutes-of-week (0–10079) with wrap-around. Kinds other than `wanted` are rejected until 7f/7g/7h add theirs; each kind's config key must be present exactly for that kind. `Validate(maps)` rejects `event_only` kinds in map regions or 7d boss spots and member counts above the smallest region of any training map.

**TODO / tests** (`// ____TEST EVENTS____`):
- [ ] Scripts: write `events.json`, regenerate `monsters.json`; `--check` clean for both.
- [ ] Real data loads (one `wanted` event, two wanted kinds).
- [ ] Every rejection case of spec §8 "gamedata" (one table-driven test).
- [ ] Commit: `Generate the event schedule and the wanted monsters`.

### Task 2: Server — schedule and event service

**Files:** `internal/event/schedule.go` (new), `internal/event/service.go` (new), `internal/event/event_test.go` (new), `internal/config/config.go`, `internal/config/config_test.go`, the server wiring (`internal/app` or `cmd/gameserver`).

**Interface:**
- `type Window struct{ EventID string; Start, End time.Time }`; `func RunID(w Window) int64`.
- `func Active(ev *gamedata.Event, t time.Time) (Window, bool)`, `func Next(ev *gamedata.Event, t time.Time) Window` (UTC+7 via a fixed offset).
- `type Run struct{ EventID, Kind string; RunID int64; Start, End time.Time }`; `type Snapshot struct{ Runs []Run }`.
- `type Config struct{ Now func() time.Time; Chat interface{ System(text string) }; DevStart []string; DevDelay time.Duration; DevMinutes int /* 0: the event's own */ }`.
- `func NewService(d *gamedata.Data, cfg Config, log *slog.Logger) *Service`; `Run(ctx)` (one-second loop; returns on ctx done); `Register(id uint64, out Outbox)`, `Unregister(id)`; `Snapshot() *Snapshot` (atomic load); `List(now) *pb.EventList`; `Tick(now)` (one evaluation; used by `Run` and tests).
- `config.Config.EventDevStart []string` from `EVENT_DEV_START` (comma list, trimmed; ids checked against `events.json` at startup).

**Behaviour:** §4.1–4.3. `Tick` computes each event's status, sends the System texts of §4.2 on transitions (first evaluation after start: "đang diễn ra, còn n phút", never "sắp bắt đầu" for a run already going), republishes the snapshot only when the run set changes, and pushes `EventList` to every registered session on any status change. Dev runs are extra one-shot windows. The service never calls world or zone; pushes are non-blocking.

**TODO / tests** (`// ____TEST SCHEDULE____`, `// ____TEST SERVICE____`):
- [ ] Schedule edges of spec §8 "event lịch" (12:00:00 in, 12:20:00 out; Monday 21:50 → Tuesday 12:00; Saturday-only slot).
- [ ] Service with a fake clock, fake chat and fake outboxes: the announcement sequence and no repeats; snapshot content; push on every status change; boot at 12:10; dev run with `DevDelay` 1 s; unregister stops pushes.
- [ ] Config: `EVENT_DEV_START=lenh_diet_quy` parsed; unknown id rejected at startup.
- [ ] Commit: `Add the event schedule service with announcements`.

### Task 3: Server — protocol 20

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 190–193; `EventStatus`, `EventInfo`, `EventList`, `WantedGroupState`; `EnterWorldResp.events = 17`. `ProtocolVersion = 20`. No new error codes.

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `event_list`, `wanted_group_state`, `enter_world_resp_events`; regenerate all with `-update`.
- [ ] Version test → 20.
- [ ] Commit: `Add the phase 7e event messages and bump the protocol to 20`.

### Task 4: Server — event runs in the zone and Lệnh Diệt Quỷ

**Files:** `internal/zone/event.go` (new), `internal/zone/event_wanted.go` (new), `internal/zone/zone.go`, `internal/zone/combat.go`, `internal/zone/loot.go`, `internal/zone/zone_test.go`, `internal/world/world.go`, `internal/world/world_test.go`.

**Interface:**
- `zone.Config.Events func() *event.Snapshot`.
- `type eventHandler interface{ start(r event.Run); end(r event.Run); joined(p *player) }`; `z.handlers map[string]eventHandler` keyed by kind (7e registers `wanted` only for `training` areas); `z.runs map[string]event.Run` (key `eventID/runID`), `z.skippedRuns` set.
- `func (z *Zone) checkEvents(now time.Time)` — once per second; diff the snapshot against `z.runs`.
- `type wantedRun struct{ run event.Run; region *region; alive, total int; members []uint64 }` and the handler methods; the 7d death hook calls `onWantedDeath(m, killer, xuRolled, dropsRolled)`.
- `world`: `event.Service.Register` in `Enter`, `Unregister` in leave; `EnterWorldResp.events` from `List(now)`; `zone.Config.Events` wired to `Service.Snapshot`.

**Behaviour:** §4.3–4.5. Start: late-zone rule, region choice with `z.rng`, spawn order members-then-count, levels, positions, special spawn with no respawn and the run tag, System text and `WantedGroupState` to zone players. Death: bonus Xu and stacks for a killer still in the zone (ground items owned by the killer, indices after the rolled drops), `alive` update, wiped text. End: remove the live members like expired corpses, no loot, `WantedGroupState{0,0}`. Joiners get `WantedGroupState` right after `EnterWorldResp`. The System line to zone players is a `ChatMessage{channel: SYSTEM}` sent by the zone to its own players (no call into the chat service).

**TODO / tests** (`// ____TEST EVENTS____` in zone, `// ____TEST ZONES AND TRANSFER____` in world):
- [ ] Every zone case of spec §8 "zone Lệnh Diệt Quỷ" with a fake snapshot and fake clock (seeded `Rand` so the region is fixed).
- [ ] `town` area ignores `wanted` runs; a snapshot with an unknown kind is ignored.
- [ ] World: enter registers and `EnterWorldResp.events` is set; leave unregisters.
- [ ] Commit: `Spawn the wanted demons of Lệnh Diệt Quỷ in every training zone`.

### Task 5: Server — gateway, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** gateway case `EventStateReq` (needs `inWorld`, packet limit only, answered from the event service); `client.EventState(ctx)`; the client helper records `EventList` and `WantedGroupState` pushes. The e2e harness builds the world with a dev run (delay 1 s, 1 minute).

**TODO / tests:**
- [ ] Gateway: `EventStateReq` before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestWantedEventOverWebSocket` per spec §8 (level 10 via `DevSetLevel`, `DevSetPlace` outside the Kumotori camp; walk to the reported region using the map JSON; pick up the bonus stone).
- [ ] Commit: `Handle event state requests and add an end-to-end wanted event test`.

### Task 6: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-7-tong-quan.md`.

**TODO:**
- [ ] game-design 6.2 and 10.2 per §9; CLAUDE.md invariant (event service publishes snapshots, zones read them; never commands into zones); README `EVENT_DEV_START` and the event; overview row marked implemented.
- [ ] Commit: `Document the phase 7e event framework and Lệnh Diệt Quỷ`.

### Task 7: Client — sync, protocol 20 and the event model

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/event_book.gd` (new), `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`event_book.gd`: `apply_list`, `apply_wanted`, `running`, `banner_text`, `next_text`, `schedule_text`, `wanted`); `Router.events`; `PROTOCOL_VERSION := 20`.

**TODO / tests** (`# ____TEST EVENTS____`):
- [ ] `sync_server.sh` copies `data/events.json`; decode the three fixtures.
- [ ] `event_book` cases of spec §8 "Client".
- [ ] Commit: `Sync the phase 7e protocol and add the event model`.

### Task 8: Client — banner, calendar and map marks

**Files:** `ui/event_banner.gd` (new), `ui/event_window.gd` (new), `ui/map_window.gd`, `ui/entity_view.gd`, `ui/hud.gd`, `screens/world.gd`.

**Behaviour (§6.2):** banner under the map name while something runs (tap opens the calendar); calendar rows with name, summary, schedule and countdown, refreshed with `EventStateReq` on open; red region frames and "Quỷ truy nã n/m" on the map window, tap travels to the region; "Truy nã" tag on wanted monsters. `world.gd` applies `EnterWorldResp.events`, `EventList`, `WantedGroupState`.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with `EVENT_DEV_START=lenh_diet_quy`: announcements, banner countdown, map frame, bonus stone. Screenshots in the report.
- [ ] Commit: `Show the event banner, calendar and wanted group on the map`.

### Task 9: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3):** start the smoke server with `EVENT_DEV_START=lenh_diet_quy`; check `EnterWorldResp.events`, wait for `WantedGroupState` total 7 and the seven entities. Print `SMOKE OK (… wanted 7 in <region>)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: event calendar, banner, Lệnh Diệt Quỷ.
- [ ] Commit: `Extend the smoke test with the wanted event`.

---

## Acceptance (controller, after Task 9)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD with `EVENT_DEV_START=lenh_diet_quy` (kill only its recorded PID).
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
