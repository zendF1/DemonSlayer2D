# Phase 2 Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Characters learn and level their class's five skills, cast the four actives on monsters (MP, cooldown, server-side areas, a real dash), and passives feed the stats. EXP per damage point rises with monster level, and only monsters within 10 levels pay EXP.

**Architecture:**
- `docs/balance/skill_stats.py` generates `data/skills.json`. `monster_stats.py` now covers levels 1–90 with an `exp` field, and `exp_table.py` generates `data/exp.json`. `gamedata` loads and validates all three. The client reads a synced copy of `skills.json`.
- The zone goroutine owns skill state: learned levels, skill points, a per-skill ready tick and a pending cast. Casts resolve in `resolveAttacks`, after the slash. Five shapes are implemented in Go; skills differ only by data.
- The protocol adds `LearnSkillReq/Resp`, `CastReq/Resp`, a `Cast` tick event, `Hit.skill_id` and four error codes. `ProtocolVersion` becomes 4.

**Tech stack:** Go, protobuf (buf), Postgres (goose migrations), Python 3 balance scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-2-ky-nang-design.md](../specs/2026-11-20-giai-doan-2-ky-nang-design.md). Section references below (§n) point to it.

**Plan format (from the user):** this plan gives interfaces, behaviour, test cases and TODOs. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):**
- Commit and push after every task, with a clear commit message.
- **No** `Co-Authored-By` or other signature trailer.
- Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:**
- Run Go tests with `-race`. DB and e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`.
- Keep 1–2 test files per package, sections split by `// ______________________TEST X_____________________` (Go) or `# ______________________TEST X_____________________` (GDScript).
- A fix comes with a test that fails on the old code.
- Before committing Go code: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip graphify if missing). Every commit leaves `go build ./...` and `make test` green.
- Client: `tests/run.sh` (headless Godot with a watchdog; it also runs `tools/sync_server.sh --check`).

**Environment:**
- The user's dev server runs on :8099. Never stop it; never use pkill or broad kill patterns.
- For your own live runs, use `LISTEN_ADDR=:8199`, start it in the background, keep its PID and kill only that PID. Port 8080 is taken.
- Never print database passwords. Don't touch `battlesquad_postgres`.

---

## Verified findings (2026-11-20)

**Combat and EXP**
- `combat.Compute(magic bool, level int, p Points) Stats` (`internal/combat/combat.go:36`). `Stats` has `MaxHP, MaxMP, PhysAttack, MagicAttack, PhysDefense, MagicDefense, Accuracy, Evasion, CritRateBP`.
- `combat.ExpToNext(level)` computes the curve in Go (`:90`), `GainExp` loops over it (`:108`), `MonsterExp(level) = 10*level` (`:126`), `ExpShare` (`:133`).
- Call sites: `internal/zone/progression.go:23` (`MonsterExp`), `:36` (`GainExp`), `:47,:84` (`Compute`), `:100` (`ExpToNext` in `selfStats`); `internal/zone/zone.go:415` (`Compute` in `join`). Tests touching them: `internal/combat/combat_test.go`, `internal/zone/combat_test.go`, `e2e/e2e_test.go`.
- `MaxLevel = 90`, `PointsPerLevel = 10` (`combat.go:14-15`).

**Data**
- `gamedata.Parse` decodes `monsters.json` with `DisallowUnknownFields` (`internal/gamedata/gamedata.go:70`). `Level{Level, HP, Attack, Defense}` (`:51`). `Data{Kinds, Levels}`. `Load(dir)` reads only `monsters.json` (`:115`); `internal/app/app.go:67` calls it.
- `gamedata_test.go:156-169` runs `python3 docs/balance/monster_stats.py --print-json` and compares it with `data/monsters.json`; it skips when python3 is missing. Copy that pattern for the new scripts.
- `monster_stats.py` has `LEVELS = range(1, 6)` and computes HP as four slashes. It asserts `monster(10)["attack"] == 95`.
- `exp_table.py` computes `exp_to_next` itself and writes `docs/balance/exp-table.csv` with `--write-csv`.

**Zone**
- `player` (`internal/zone/zone.go:77`) holds `stats, points, freePoints, exp, mp, lastAttack, pendingAttack *attackCmd, statsDirty`.
- Commands: `attackCmd`, `allocateCmd`, `respawnCmd`; public methods `Attack/Allocate/Respawn` push to `z.inbox` and return false when it is full (`zone.go:255-287`); `handle` dispatches (`:383`).
- Tick order: `drain → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView` (`zone.go:323`).
- `resolveAttacks` / `checkAttack` / `slash` live in `internal/zone/combat.go`. `attackRangeSlack = 0.5`. `slash` pays EXP with `payExpShare`, sets `m.attackers[p.id] = z.tickNo`, and calls `z.startChase(m, p.id)` unless the monster is chasing or attacking.
- `tickEvents{hits, deaths, hpChanges, levelUps}`; `addEvents(p, wt)` filters by `p.visible` or own id.
- `entity.snap` makes viewers jump; `syncView` sends every changed entity in `near` as `EntityMove`, **including the player itself** (a player sees its own `EntityEnter` once and then its own moves). Respawn uses `p.changed, p.snap = true, true` (`lifecycle.go:36`). The flags reset after every tick.
- `updateCell(&p.entity)` moves an entity between AOI cells. `nearby(cell)` returns the 3×3 cells. Default `AOI_CELL_SIZE` is 10.
- `lineBlocked(grid, a, b)` (`move.go:110`), `distance(a, b)`, `ClampDir`, `bad(v)` (NaN/Inf check).
- `z.ticks(sec)` rounds up, at least 1.
- `z.state(p)` builds `store.CharState` for saves (`zone.go:490`). `join` sends `EnterWorldResp` with `selfStats(p)` (`zone.go:400-443`).

**Store and world**
- `store.Character` and `store.CharState` (`internal/store/characters.go`). `charColumns` + `scanCharacter`. `SaveCharacterState` is one `UPDATE` (`:120`). `GetCharacter(ctx, accountID, charID)`; `GetAccountByUsername`.
- Migrations in `migrations/` (goose, embedded). Latest is `00002_combat.sql`.
- `world.Enter` builds `zone.PlayerInfo` from `store.Character` (`internal/world/world.go:226`). `world.areaOf(m, d)` builds `zone.Area{..., Levels: d.Levels}` (`:101`). `world.New(cfg, maps, monsters *gamedata.Data, ...)`.
- `world.Membership` wraps zone calls (`Attack`, `Allocate`, `Respawn`, `Leave`).

**Gateway and proto**
- `ProtocolVersion uint32 = 3` (`internal/gateway/server.go:23`). Limits: `packetsPerSec 30`, `movesPerSec 20`, `attacksPerSec 5` (`server.go:32-34`), each a `rate.Limiter` on `conn` (`conn.go:47-63`).
- `handlers.go:64-90`: `AttackReq` checks `inWorld`, then `c.attacks.Allow()` (else `RATE_LIMITED`), then `c.zoneBusyIfDropped(seq, c.member.Attack(...))`. `AllocatePointsReq` has no own limiter.
- Fixtures: `gateway_test.go:200` has `-update`; `TestClientFixtures` (`:288`) writes `testdata/client/*.hex`; `TestRangedHitFixtureRoundTrip` (`:267`) is the model for decode tests.
- Envelope payload numbers end at `self_stats = 56`. `ErrorCode` ends at `NOT_ENOUGH_POINTS = 22`. `WorldTick` uses fields 1–7. `Hit` uses 1–5. `SelfStats` uses 1–19. `EnterWorldResp` uses 1–9.

**e2e and client library**
- `e2e/e2e_test.go`: `startServer`, `newPlayer(t, s, username, charName, class)`, `waitFor`, `TestKillMonsterOverWebSocket` (`:369`), `TestCombatProgressSurvivesLogout` (`:410`, uses `s.store.SaveCharacterState`), `TestAttackRateLimit` (`:519`).
- `internal/client` has request helpers such as `c.Attack(ctx, target)`; add `Learn` and `Cast` the same way.

**Client**
- `autoload/net.gd:27` `PROTOCOL_VERSION := 3`. `autoload/config.gd:9` `ACTION_KEYS` (`stats` is `KEY_C`).
- `core/wire.gd` `PAYLOADS` lists every Envelope oneof name in proto order.
- `screens/world.gd:236-261` handles pushes. For own `EntityMove`, `snap and _dead` means respawn; otherwise `_predictor.reconcile(pos, ack)`.
- `core/predictor.gd`: `reset(pos)`, `reconcile(server_pos, ack)`, `SNAP_TILES 0.5`.
- `ui/stats_panel.gd` is the Phase 1 stats/allocation panel (signals `confirmed(points)`, `closed`; `open/refresh/allocated/failed/set_dead/close`). `ui/hud.gd:215` builds the "Chỉ số" button with a red dot and the "Đánh" hold button.
- `tools/sync_server.sh` copies `server/maps → data/maps`, `server/testdata/client → tests/fixtures` and regenerates `proto/protocol.gd`; `--check` diffs them.
- Tests: `tests/unit/test_core.gd`, `tests/unit/test_net.gd`. Smoke: `tests/smoke/smoke.gd` via `tools/smoke.sh`.
- Dev accounts to raise at acceptance: `test1` (Kaito, viem) and `test2` (Hana, thuy).

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/skill_stats.py (new), data/skills.json (new)       skill definitions (Task 1)
docs/balance/monster_stats.py, data/monsters.json               levels 1-90, exp field (Task 1)
docs/balance/exp_table.py, data/exp.json (new), exp-table.csv   EXP curve from monster exp (Task 1)
internal/gamedata/gamedata.go, skills.go (new), gamedata_test.go  parse + validate (Tasks 1, 2)
internal/combat/combat.go, combat_test.go                       ExpTable, ExpEligible (Task 3); Passive, SkillBase (Task 4)
internal/zone/progression.go                                    EXP rule, skill points, learn (Tasks 3, 6, 7)
proto/ds/v1/protocol.proto, internal/pb/                        Phase 2 messages (Task 5)
internal/gateway/server.go, handlers.go, conn.go, gateway_test.go, testdata/client/  version 4, fixtures, handlers (Tasks 5, 7, 8)
migrations/00003_skills.sql (new)                               skill_points, character_skills (Task 6)
internal/store/characters.go, dev.go (new), store_test.go       skills load/save, DevSetLevel (Tasks 6, 11)
internal/world/world.go                                         skills into PlayerInfo and Area (Tasks 3, 6, 7, 8)
internal/zone/zone.go, area.go                                  player skill state, commands (Tasks 6-8)
internal/zone/skill.go (new)                                    cast validation, shapes, dash (Tasks 8-10)
internal/zone/zone_test.go, combat_test.go                      tests
internal/client/*.go                                            Learn, Cast helpers (Tasks 7, 8)
cmd/devchar/main.go (new)                                       dev level command (Task 11)
e2e/e2e_test.go                                                 skill e2e (Task 11)
docs/game-design.md, CLAUDE.md, README.md, phase 1 spec          docs (Task 12)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), data/skills.json, proto/protocol.gd, tests/fixtures/  sync (Task 13)
tools/sync_server.sh                                            copy + check skills.json (Task 13)
autoload/net.gd, autoload/config.gd                             version 4 (Task 13); keys 1-5, K (Task 16)
core/wire.gd, core/errors.gd                                    payloads, error texts (Task 13)
core/skill_book.gd (new)                                        skill data + learned state (Task 14)
core/cast_loop.gd (new), core/predictor.gd, core/entity_table.gd  casting, dash snap, cast events (Task 15)
ui/char_window.gd, info_tab.gd, potential_tab.gd, skill_tab.gd (new); ui/stats_panel.gd (removed)  tabbed window (Task 16)
ui/skill_bar.gd, ui/skill_effect.gd (new), ui/hud.gd            bar, effects, Thông tin button (Task 16)
screens/world.gd, screens/world_entities.gd                     wiring (Task 16)
tests/unit/test_core.gd, tests/unit/test_net.gd                 tests (Tasks 13-15)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                 skill smoke step (Task 17)
```

---

### Task 1: Server — balance scripts and generated data

**Files:** `docs/balance/skill_stats.py` (new), `docs/balance/monster_stats.py`, `docs/balance/exp_table.py`, `data/skills.json` (new), `data/monsters.json`, `data/exp.json` (new), `docs/balance/exp-table.csv`, `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Behaviour:**
- `skill_stats.py` (§2.1): holds the 25-skill table and writes `data/skills.json` with `--write-json` or prints the same bytes with `--print-json`. It computes every 10-element array: `coef_pct`, `mp`, `cooldown_ms = round(base × 0.9^(n−1) × 1000)`, `max_targets` (2,2,2,3,3,3,4,4,4,5) for every shape except `single`, and passive arrays. It exposes a function the monster script imports for the Bất Tri Hỏa coefficient at skill level n. JSON key order and formatting stay stable (2-space indent, `ensure_ascii=False`, trailing newline), as in `monster_stats.py`.
- `monster_stats.py` (§2.4): `LEVELS = range(1, 91)`. HP for levels 1–9 is unchanged (four slashes). From level 10 it is two slashes plus two Bất Tri Hỏa at skill level `min(10, L − 9)` on the same-level monster. Each level gets `exp = max(1, round_half_up(HP × r(L)))` with `r(L) = 0.1 × (1 + 0.1 × (L − 1))`, computed with `Fraction`. Keep the 4% hit assertion and the `monster(10)["attack"] == 95` assertion. Levels 1–5 must keep today's `hp`, `attack` and `defense`.
- `exp_table.py` (§2.4): imports `exp(L)` from `monster_stats.py`. `kills(L)` is unchanged. `exp_to_next(L) = round10(kills(L) × exp(L))`. `--write-json` writes `data/exp.json` as `{"exp_to_next": [89 ints]}`; `--print-json` prints it; `--write-csv` keeps writing the CSV with the new numbers. Assertions: strictly increasing; hours to level 10 within 2.8–3.2; hours to level 50 within ±5% of 456.3.
- `gamedata.Level` gains `Exp int64 \`json:"exp"\``, which must be positive (same error as the other numbers).

**TODO / tests:**
- [ ] Write the three scripts; regenerate the three JSON files and the CSV.
- [ ] `// ____TEST GENERATED DATA____` in `gamedata_test.go`: extend the existing compare-with-script test into a table over `monster_stats.py → monsters.json`, `skill_stats.py → skills.json`, `exp_table.py → exp.json` (byte equality, skip without python3).
- [ ] A level with `exp: 0` fails `Parse`; the committed `monsters.json` parses and has levels 1–90.
- [ ] `make test` and the existing zone tests still pass (levels 1–5 unchanged).
- [ ] Commit: `Generate skill data, monster levels 1-90 with EXP and the EXP table`.

### Task 2: Server — load and validate skills.json and exp.json

**Files:** `internal/gamedata/skills.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`, `internal/app/app.go` if `Load`'s use changes.

**Interface:**
- `type Skill struct { ID, Class, Name, Form string; LearnLevel, Slot int; Damage, Shape string; CastRange, Radius, ConeDeg, Reach, Length, Width float64; Dash bool; CoefPct, MP, CooldownMS, MaxTargets []int; Passive map[string][]int }` with JSON tags matching §2.1 (`omitempty` not needed; absent fields decode to zero).
- `func (s Skill) Active() bool` — `Slot > 0`.
- Constants for shapes (`ShapeSingle = "single"`, …) and passive keys (`PassivePhysDefPct = "phys_def_pct"`, …).
- `Data` gains `Skills map[string]Skill`, `ClassSkills map[string][]Skill` (each class's five, ordered by slot with the passive last or by learn level — pick one and document it), `Exp []int64` (index 0 = level 1).
- `func ParseSkills(b []byte) (map[string]Skill, error)`, `func ParseExp(b []byte) ([]int64, error)`.
- `Load(dir)` reads `monsters.json`, `skills.json`, `exp.json`.

**Behaviour:** every check of §2.1 "Server kiểm tra khi nạp" and §2.4 (`exp.json`: exactly `MaxLevel − 1` = 89 positive, strictly increasing numbers; unknown fields rejected). Passive keys outside the seven known ones are rejected. Errors name the skill id and the field.

**TODO / tests** (`// ____TEST SKILLS DATA____`):
- [ ] The committed `skills.json` parses: 25 skills, five per class, slots 1–4 at learn levels 10/20/40/50, passive at 30.
- [ ] Table of broken inputs, each built by mutating a valid minimal JSON, each must fail: unknown field; duplicate id; bad class; missing `radius` on `circle_self`; `reach` on a `single`; array of 9; zero in `mp`; `dash` on `cone`; `max_targets` on `single`; missing `max_targets` on `line`; passive at learn level 20; passive with an active field (`shape`); unknown passive key; class with two slot-2 skills; class missing slot 4; `cone_deg` 0 and 400.
- [ ] `exp.json`: valid parses to 89 values; 88 values fails; a non-increasing pair fails; a zero fails.
- [ ] Commit: `Load and validate skill definitions and the EXP table`.

### Task 3: Server — EXP table and the level-window rule

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, `internal/zone/area.go`, `internal/zone/progression.go`, `internal/world/world.go`, zone and e2e tests that assumed `10 × level` or the old curve.

**Interface:**
- `type ExpTable []int64` (index 0 = level 1). `func (t ExpTable) ToNext(level int) int64` (0 at `MaxLevel` or out of range). `func (t ExpTable) Gain(level int, exp, gain int64) (int, int64)` with today's `GainExp` semantics (carry over, several levels at once, stop at `MaxLevel` with exp 0; at `MaxLevel` nothing).
- `const ExpLevelWindow = 10`; `func ExpEligible(playerLevel, monsterLevel int) bool` — `|p − m| ≤ 10`.
- Remove `ExpToNext`, `GainExp`, `MonsterExp`.
- `zone.Area` gains `Exp combat.ExpTable`; `world.areaOf` fills it from `gamedata.Data.Exp`.

**Behaviour (§2.4):**
- `payExpShare` uses `z.area.Levels[m.level].Exp` as the total.
- A hit outside the window pays nothing and does not add to `m.dmgBy[p.id]`. Check it in `payExpShare` so slash and skills share it.
- `gainExp` and `selfStats.ExpNext` use `z.area.Exp`.

**TODO / tests:**
- [ ] `// ____TEST EXP TABLE____` in `combat_test.go`: `ToNext` at 1, 89, 90 and 0; `Gain` carries over, gains two levels at once, stops at 90; replace the old formula tests.
- [ ] `ExpEligible(20, 10)`, `(10, 20)` true; `(21, 10)`, `(10, 21)` false.
- [ ] Using the real `gamedata`: EXP per damage point (`exp / hp`) at level 50 is greater than at level 10, and at 10 greater than at 1.
- [ ] Zone: a level-12 player slashing a level-1 monster gets no EXP and no `dmgBy` entry; a level-11 player does get EXP. Use the test helpers that build an area with chosen levels.
- [ ] Fix existing zone and e2e tests that hard-code EXP amounts; derive expected values from the level table instead of literals.
- [ ] Commit: `Pay EXP by the generated table and only within ten levels`.

### Task 4: Server — passives in stats and the skill damage base

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, every `Compute` call site (pass a zero passive for now).

**Interface:**
- `type Passive struct { PhysDefPct, MagicDefPct, MaxHPPct, EvasionPct, AccuracyPct, MagicAttackPct, CritBP int }`; `func (p Passive) Add(q Passive) Passive`.
- `func Compute(magic bool, level int, pts Points, pas Passive) Stats`.
- `func SkillBase(s Stats, magic bool, coefPct int) int` — `floor(attack × coefPct / 100)`, attack = `MagicAttack` when magic, else `PhysAttack`.

**Behaviour (§3.2):** compute the Phase 1 stats, then add `floor(before × pct / 100)` per percent field to its stat, and `CritBP` to `CritRateBP`. Integer maths only.

**TODO / tests** (`// ____TEST PASSIVE____`, `// ____TEST SKILL BASE____`):
- [ ] Zero passive gives exactly the Phase 1 stats (table over a few levels and point spreads).
- [ ] Nham level 10 passive (`MaxHPPct 20, PhysDefPct 20`) on a known stat block: max HP and physical defense rise by the floored 20%, magic defense unchanged.
- [ ] Flooring: a stat of 57 with 3% adds 1.
- [ ] `CritBP 1000` adds 1000 basis points.
- [ ] `SkillBase` with phys 285 and magic 120: phys at 150% gives 427, magic at 150% gives 180.
- [ ] Commit: `Add passive skill effects to derived stats and the skill damage base`.

### Task 5: Server — Phase 2 protocol

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):**
- Messages and fields exactly as §5: Envelope 57–60, `SkillLevel`, `EnterWorldResp.skills = 10`, `SelfStats.skill_points = 20`, `Cast`, `WorldTick.casts = 8`, `Hit.skill_id = 6`, error codes 23–26 with one-line comments.
- Update the `WorldTick` comment to the client apply order `moves, casts, hits, deaths, hp_changes, level_ups`.
- `ProtocolVersion = 4`.
- The gateway does not handle the new requests yet (its default branch answers `BAD_REQUEST`); Tasks 7 and 8 add them.

**TODO / tests:**
- [ ] `make proto`; no field named `message`/`class`, no message `Error`.
- [ ] New fixtures in `TestClientFixtures`: `learn_skill_resp` (stats with `skill_points`, two `SkillLevel`), `cast_req`, `world_tick_cast` (one `Cast`, two `Hit` with `skill_id`, one `EntityMove` with `snap`). Regenerate all with `-update`.
- [ ] A round-trip test in the style of `TestRangedHitFixtureRoundTrip`: `world_tick_cast` decodes to the `Cast` and the `skill_id`s.
- [ ] Bump the `ProtocolVersion` test to 4; e2e login still works (the client library sends the constant).
- [ ] Commit: `Add the phase 2 skill messages and bump the protocol to 4`.

### Task 6: Server — store, world and zone plumbing for skills

**Files:** `migrations/00003_skills.sql` (new), `internal/store/characters.go`, `internal/store/store_test.go`, `internal/world/world.go`, `internal/zone/zone.go`, `internal/zone/area.go`, `internal/zone/progression.go`, tests.

**Interface:**
- `store.Character` and `store.CharState` gain `SkillPoints int` and `Skills map[string]int`.
- `GetCharacter` and `ListCharacters` load skills (one extra query for `character_skills`, or a join; keep `scanCharacter` simple). `SaveCharacterState` runs in one transaction: the `UPDATE` (now with `skill_points`), then replaces the character's `character_skills` rows (delete + insert).
- `zone.PlayerInfo` gains `SkillPoints int`, `Skills map[string]int`. `zone.Area` gains `Skills map[string]gamedata.Skill` (shared, read-only).
- `player` gains `skillPoints int`, `skills map[string]int`.
- A zone helper `z.recompute(p)` that calls `combat.Compute` with the passive sum from `p.skills` (replace every direct `Compute` call in the zone).

**Behaviour:**
- Migration per §2.2 with a goose Down that drops both.
- `world.Enter` drops skill ids missing from `gamedata` (or of another class) with one `log.Warn` per id, and fills `PlayerInfo`.
- Each level gained adds 1 skill point next to the 10 potential points.
- `z.state(p)` saves skill points and skills.
- `selfStats(p)` sets `SkillPoints`; `EnterWorldResp.Skills` lists learned skills sorted by id.

**TODO / tests:**
- [ ] `// ____TEST SKILLS STORE____` (DB tests): migration backfills `skill_points = level − 1` (migrate a fresh schema to version 2 with goose, insert a level-5 character, migrate up, read 4). Save and load round-trips skills and points; saving an empty map deletes old rows; deleting a character cascades.
- [ ] World: a stored unknown skill id is dropped and logged, the rest kept.
- [ ] Zone: join with a learned Nham passive gives the higher max HP; level-up adds 1 skill point; `EnterWorldResp` carries skills; `SelfStats` carries skill points.
- [ ] Commit: `Store, load and carry learned skills and skill points`.

### Task 7: Server — learning skills

**Files:** `internal/zone/progression.go`, `internal/zone/zone.go`, `internal/world/world.go` (Membership), `internal/gateway/handlers.go`, `internal/client/*.go`, tests.

**Interface:**
- `zone.(*Zone).Learn(id uint64, seq uint32, skillID string) bool` + `learnCmd`; `Membership.Learn(seq, skillID) bool`.
- Gateway `LearnSkillReq`: `inWorld`, then forward with `zoneBusyIfDropped`. No own limiter (§4.7).
- `client.Learn(ctx, skillID) (*pb.LearnSkillResp, error)`.

**Behaviour (§3.1):** check order DEAD → BAD_REQUEST (unknown or other class) → SKILL_MAXED → LEVEL_TOO_LOW (`level < learn_level + n`) → NOT_ENOUGH_POINTS. On success: spend a point, raise the level, `z.recompute(p)`, raise current HP and MP by the rise in their maxima, save at once (`z.saver.Save(z.state(p), nil)`), reply `LearnSkillResp{stats, skills}`, and call `z.hpChanged(&p.entity)` when max HP changed so viewers see the new bar.

**TODO / tests** (`// ____TEST LEARN____` in `zone_test.go`):
- [ ] One case per error code, in order: a dead level-90 player gets DEAD even for a bad id; another class's skill gets BAD_REQUEST; level 10 at skill 10 gets SKILL_MAXED (set state directly); level 10 learning skill 1 twice gets LEVEL_TOO_LOW on the second (needs 11); level 10 with 0 points gets NOT_ENOUGH_POINTS.
- [ ] Success: points −1, level +1, reply carries skills; a save is queued.
- [ ] Learning Nham passive raises max HP, current HP by the same amount, and records an HP change.
- [ ] Gateway: `LearnSkillReq` before entering the world gets NOT_IN_WORLD.
- [ ] Commit: `Let players learn and level skills`.

### Task 8: Server — casting: queue, checks, single-target skills

**Files:** `internal/zone/skill.go` (new), `internal/zone/combat.go`, `internal/zone/zone.go`, `internal/world/world.go`, `internal/gateway/handlers.go`, `conn.go`, `server.go`, `internal/client/*.go`, tests.

**Interface:**
- `zone.(*Zone).Cast(id uint64, seq uint32, skillID string, target uint64, dir Vec2) bool` + `castCmd`; `Membership.Cast(...)`.
- `player` gains `pendingCast *castCmd`, `skillReady map[string]uint32` (tick when usable again).
- `tickEvents` gains `casts []*pb.Cast`; `addEvents` adds a `Cast` when `sees(caster)`.
- Gateway: `castsPerSec = 5`, a `casts` limiter, `CastReq` handled like `AttackReq`; `dir_x/dir_y` passed through `ClampDir`-style sanitising (NaN/Inf → zero).
- `client.Cast(ctx, skillID, target, dirX, dirY) error`.

**Behaviour (§4.1–4.3, §4.6) for `single` and `circle_self` only in this task** (other shapes answer BAD_REQUEST until Task 9; dash skills cast as plain `single` until Task 10):
- `queueCast` mirrors `queueAttack` (newer replaces older; the older gets ON_COOLDOWN).
- `resolveAttacks` resolves the slash, then the cast, per player in id order.
- Check order DEAD → IN_SAFE_ZONE → SKILL_NOT_LEARNED → ON_COOLDOWN → NOT_ENOUGH_MP → shape checks (INVALID_TARGET / OUT_OF_RANGE with `cast_range + attackRangeSlack` and `lineBlocked`).
- Accept: `mp -= cost`, `skillReady = tick + z.ticks(cooldown_ms/1000)`, `statsDirty = true`, send `CastResp`, then hits: `combat.SkillBase` → `combat.Damage` → `Hit{..., SkillId}` → `payExpShare` → attackers/startChase as in `slash` → `killMonster` at 0 HP. Factor the shared part of `slash` into one helper used by both.
- Emit one `Cast` per accepted cast (§4.6 `dir` rules).

**TODO / tests** (`// ____TEST CAST____` in `combat_test.go`):
- [ ] One case per error code in order; after every error MP and `skillReady` are unchanged.
- [ ] Slash and a skill both land in the same tick (independent cooldowns).
- [ ] A second cast of the same skill before its cooldown gets ON_COOLDOWN; after `z.ticks(...)` ticks it lands.
- [ ] Damage equals `combat.Damage(combat.SkillBase(...), def, level)` and the hit carries `skill_id`; the monster starts chasing.
- [ ] `circle_self` (Thủy Liệt Oa) hits a monster at radius 2.4, not one at 2.6; hits nobody when alone but still spends MP and emits `Cast`.
- [ ] A viewer who sees the caster gets the `Cast`; one out of view does not.
- [ ] Gateway: six `CastReq` in one second → the sixth is RATE_LIMITED (model on `TestAttackRateLimit` if it is an e2e test, or a gateway unit test).
- [ ] Commit: `Cast single-target and self-centred skills`.

### Task 8b: Server — unified skill ranges (user change, spec §1.2 "Tầm đánh", §2.1, §4.3–4.4)

**Files:** `docs/balance/skill_stats.py`, `data/skills.json`, `internal/gamedata/skills.go`, `internal/gamedata/gamedata_test.go`, `internal/zone/skill.go`, `internal/zone/combat_test.go`.

**Behaviour:**
- Data: every active except `circle_self` gets `range[10]`: `single` (incl. dash) is 5 at every level; `cone`, `line`, `circle_target` are 4,4,4,5,5,5,6,6,6,6. Remove the scalar `cast_range`, `reach` and `length`. Keep `radius` (circle shapes), `cone_deg` (cone) and `width` (line). Regenerate `skills.json`; monster HP is unaffected (it uses only Bất Tri Hỏa's coefficient) — verify `monsters.json` and `exp.json` stay byte-identical.
- `gamedata.Skill`: replace `CastRange`, `Reach`, `Length` with `Range []int`/`[]float64` (pick one, document it). Validation per §2.1: `range` required on every shape except `circle_self` and forbidden there; positive; non-decreasing over levels; 10 entries.
- Zone: R = `Range[n−1]` of the learned level. Single/circle_target target checks use `R + attackRangeSlack`. Cone reach and line length (Task 9) use R without slack.

**TODO / tests:**
- [ ] Update the gamedata fixtures and reject cases (missing `range` on single/cone/line/circle_target; `range` on circle_self; a decreasing `range`; scalar `cast_range` now an unknown field).
- [ ] Zone: a level-1 `single` reaches at 5.49 and fails at 5.51 (5 + 0.5 slack); range comes from the learned level.
- [ ] Commit: `Use one range table for every class's skills`.

### Task 9: Server — cone, line and target-circle areas

**Files:** `internal/zone/skill.go`, `internal/zone/combat_test.go`.

**Interface (internal):**
- `func (z *Zone) aim(p *player, c castCmd, s gamedata.Skill) (target *monster, dir Vec2, code pb.ErrorCode)` per §4.3.
- `func (z *Zone) victims(p *player, s gamedata.Skill, n int, primary *monster, dir Vec2) []*monster` per §4.4.
- `func (z *Zone) monstersInBox(min, max Vec2) []*monster` scanning every AOI cell that overlaps the box (not only 3×3).

**Behaviour (§4.3–4.4):**
- `cone`/`line`: aim at a living `target_id` regardless of range; otherwise at the normalised `dir`; neither → BAD_REQUEST.
- `circle_target`: needs a target in `R + 0.5` with line of sight (R = `range[n−1]`); the area centre is the target. Cone reach and line length are R (no slack).
- Area membership by the §4.4 table; line of sight from the shape's origin; living monsters only; a monster at the origin counts as inside.
- Order: `target_id` first if inside, then by distance to the centre, ties by lower id; truncate to `max_targets[n−1]`.

**TODO / tests** (`// ____TEST AREAS____`):
- [ ] Cone 120° at skill level 1 (reach 4): hits at 3.9 tiles straight ahead and at 59° off axis; misses at 61°, behind, and at 4.1 tiles. At skill level 10 (reach 6) it hits at 5.9.
- [ ] Line at skill level 10 (length 6), width 1.5: hits at (5.9, 0.7); misses at (6.1, 0), (3, 0.8), and behind the caster. At level 1 it misses at (4.1, 0).
- [ ] Circle around a target, radius 1.5: hits a second monster 1.4 from the target, misses one at 1.6.
- [ ] `max_targets`: six monsters in a level-1 Liệt Oa get exactly 2 hits; the locked target is first even when it is the farthest; the rest are the nearest to the centre.
- [ ] A monster inside the area but behind a wall from the origin is not hit.
- [ ] Cone with no target and zero `dir` → BAD_REQUEST, no MP spent; with only `dir` it hits along `dir`.
- [ ] Commit: `Resolve cone, line and target-circle skill areas`.

### Task 10: Server — dash

**Files:** `internal/zone/skill.go`, `internal/zone/combat_test.go`.

**Behaviour (§4.5):** for `dash` skills, after the target checks and before spending MP, compute the landing point (1 tile short of the target on the caster→target segment; no move when closer than 1 tile). Reject with OUT_OF_RANGE when the landing tile is not walkable, is in a safe zone, or the segment caster→landing is blocked. On accept, set `p.pos`, `p.changed, p.snap = true, true`, `z.updateCell(&p.entity)`, then apply damage. The player's `dir` is kept, so `movePlayers` continues from the landing point.

**TODO / tests** (`// ____TEST DASH____`):
- [ ] Viêm Bất Tri Hỏa from 3.5 tiles lands exactly 1 tile from the monster on the line; the caster's own next `WorldTick` and a viewer's carry its `EntityMove` with `snap`.
- [ ] Target 0.8 tiles away: no move, no snap, damage lands.
- [ ] Landing in a safe zone or on a wall tile → OUT_OF_RANGE, position, MP and cooldown unchanged.
- [ ] The AOI cell updates when the dash crosses a cell border (a viewer only near the new cell gets the caster's EntityEnter).
- [ ] Commit: `Move the caster for dash skills`.

### Task 11: Server — devchar and the skill e2e

**Files:** `internal/store/dev.go` (new), `internal/store/store_test.go`, `cmd/devchar/main.go` (new), `e2e/e2e_test.go`, `README.md`.

**Interface:**
- `func (s *Store) DevSetLevel(ctx context.Context, username string, level int) (DevResult, error)` with `DevResult{Name string; Level, FreePoints, SkillPoints int}`.
- `cmd/devchar`: flags `-u` and `-level`, config from `DATABASE_URL` only (reuse `internal/config` or read the env var; never print it).

**Behaviour (§2.3):** first character by slot; error when the account or the character is missing; `1 ≤ level ≤ 90`; one transaction: `level`, `exp = 0`, all four points to 0, `free_points = 10 × (L − 1)`, `skill_points = (L − 1) − sum(skill levels)`, and when that is negative delete all skills and set `skill_points = L − 1`; `hp`, `mp` NULL. The command prints a warning that the character must be offline, then a summary line.

**TODO / tests:**
- [ ] `// ____TEST DEV LEVEL____` (DB): level 90 on a fresh character → 890 free points, 89 skill points; with spent points and two learned skills (levels 3 and 2) at level 20 → 190 free, 14 skill points, skills kept; level 5 with those skills → skills deleted, 4 skill points; missing account → error; level 0 and 91 → error.
- [ ] e2e `TestSkillsOverWebSocket`: create a player, leave, `DevSetLevel(…, 20)`, enter, learn slot 1 and slot 2, walk to a monster (reuse the existing fight helper), cast slot 1 on it: `CastResp`, `SelfStats` with lower MP, a `Hit` with that `skill_id`; cast again at once → ON_COOLDOWN.
- [ ] README: a "Dev characters" section with the devchar command.
- [ ] Commit: `Add the devchar command and an end-to-end skill test`.

### Task 12: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-19-giai-doan-1-chien-dau-pve-design.md`.

**TODO:**
- [ ] game-design §2.2: replace the EXP formula text and table with the new curve (`kills(L) × exp(L)`), using numbers from the regenerated CSV; hours unchanged.
- [ ] §5.8: EXP per hit = damage × EXP gốc / HP, EXP gốc = HP × r(L); add the ±10 level window. Update the 56 HP / 10 EXP example with the new numbers.
- [ ] §2.6: the "Tầm đánh / vùng ảnh hưởng" column of the five class tables follows the unified ranges (single 5 ô; multi-target 4/5/6 ô by skill level, circle-self radii unchanged); replace the rule "Cộng điểm tăng hệ số và số mục tiêu theo bảng, không tăng tầm đánh hoặc kích thước vùng" with one saying points also raise the multi-target range by the 1–3/4–6/7–10 brackets; note the anti-kite reason.
- [ ] §5.12: EXP row becomes `HP × r(L)` for normal monsters (elite and boss multipliers stay).
- [ ] CLAUDE.md invariants: `skills.json` from `skill_stats.py`, `exp.json` from `exp_table.py`, `monster_stats.py` and `exp_table.py` run together when monster HP changes.
- [ ] Phase 1 spec: a "Đã thay bằng spec Giai đoạn 2" note where it defines EXP gốc `10 × cấp` and `ExpToNext`.
- [ ] Vietnamese prose in docs, matching their style.
- [ ] Commit: `Document the phase 2 EXP rules and generated skill data`.

### Task 13: Client — sync and protocol 4

**Files:** `server` submodule, `tools/sync_server.sh`, `data/skills.json` (new), `proto/protocol.gd`, `tests/fixtures/`, `autoload/net.gd`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_net.gd`, `tests/unit/test_core.gd`.

**Behaviour:**
- Bump the submodule to the server HEAD after Task 12.
- `sync_server.sh` copies `server/data/skills.json` to `data/skills.json`; `--check` reports it stale when different.
- `PROTOCOL_VERSION := 4`; `Wire.PAYLOADS` adds `learn_skill_req, learn_skill_resp, cast_req, cast_resp` in proto order.
- `errors.gd`: Vietnamese texts — SKILL_NOT_LEARNED "Chưa học kỹ năng này", NOT_ENOUGH_MP "Không đủ MP", SKILL_MAXED "Kỹ năng đã đạt cấp tối đa", LEVEL_TOO_LOW "Chưa đủ cấp".

**TODO / tests:**
- [ ] Decode `learn_skill_resp.hex`, `cast_req.hex`, `world_tick_cast.hex` (casts, `skill_id`, snap).
- [ ] Every new code has a non-empty text.
- [ ] `tests/run.sh` green including `--check`.
- [ ] Commit: `Sync phase 2 protocol and skill data from the server`.

### Task 14: Client — skill book

**Files:** `core/skill_book.gd` (new), `tests/unit/test_core.gd`.

**Interface:**
- `func load_json(text: String) -> bool`; `func for_class(cls: String) -> Array` (five skill dicts, ordered by slot 1–4 then the passive); `func by_slot(cls, slot) -> Dictionary`.
- `func set_learned(levels: Dictionary)` (id → level), `func level_of(id) -> int`, `var skill_points := 0`.
- `func mp(id, n)`, `cooldown_sec(id, n)`, `coef(id, n)`, `max_targets(id, n)`, `passive(id, n) -> Dictionary`.
- `func learn_block(id, char_level) -> String` — `""`, `"maxed"`, `"level"`, `"points"`; same order as the server after DEAD/BAD_REQUEST.
- `func start_cooldown(id, now)`, `func ready_in(id, now) -> float` (0 when ready).
- A helper mapping `Proto.CharClass` to the class string.

**TODO / tests** (`# ____TEST SKILL BOOK____`):
- [ ] Loads the synced `res://data/skills.json`: five skills per class, slot order.
- [ ] `learn_block`: level 9 → "level"; level 10, 0 points → "points"; level 10, 1 point → ""; skill at 10 → "maxed"; skill at 1 with level 10 → "level" (needs 11).
- [ ] `cooldown_sec("viem_1", 3)` = 1.215; `ready_in` counts down to 0.
- [ ] Commit: `Add the client skill book`.

### Task 15: Client — cast loop, dash snap and cast events

**Files:** `core/cast_loop.gd` (new), `core/predictor.gd`, `core/entity_table.gd`, `tests/unit/test_core.gd`.

**Interface:**
- `cast_loop.gd`: `func press(skill: Dictionary, level: int, target_id: int) -> String` (called when the attack button goes down with a skill selected; returns `""` or a notice key: `"no_target"`, `"cooldown"`, `"mp"`; `"cooldown"`/`"mp"` mean "slash instead"); `func step(now, self_pos, target_pos, manual: bool, mp: int, facing: Vector2) -> Dictionary` returning `{"dir": Vector2, "send": Dictionary or null}` where `send` holds `skill_id, target_id, dir`; `func on_reply(code)`; `func cancel()`.
- `predictor.gd`: `func teleport(server_pos: Vector2, ack: int)` — position becomes `server_pos`, history keeps only entries with seq > ack, rebased so later reconciles match.
- `entity_table.gd`: `apply_tick` returns a `cast` effect per `Cast` (`{"type": "cast", "caster", "skill_id", "target", "dir"}`); a `Hit` with `skill_id` returns the damage-number effect without the slash.

**Behaviour (§6.1):** the loop runs while the attack button (or Space) is held with an active skill selected in the bar; slot 1 (default slash) stays with `attack_loop`. Each time the selected skill is ready it is cast once; while it is cooling down or MP is short and the button is still held, the default slash runs instead (through `attack_loop`). Every skill has a reach (`range[n−1]` for single/circle_target/cone/line, `radius` for circle_self). With a locked target beyond the reach, walk (like `attack_loop`'s approach) until within reach − 0.3, then send once — for every shape. Targeted shapes need a locked target. Untargeted shapes without a locked target send at once with `facing`; a manual move, a new target or another press cancels; OUT_OF_RANGE switches to walking straight at the target and retrying, as `attack_loop` does; local cooldown and MP checks block sending.

**TODO / tests** (`# ____TEST CAST LOOP____`, `# ____TEST DASH SNAP____`, `# ____TEST CAST EVENTS____`):
- [ ] Single skill, target 6 tiles away, range 4: `step` walks towards the target and sends only when within range.
- [ ] Cone with no target sends at once with `facing`.
- [ ] Circle-self with a locked target 5 tiles away (radius 2.5) walks first and sends once the target is within the radius − 0.3.
- [ ] Targeted skill without a target → `"no_target"`, nothing sent; on cooldown → `"cooldown"`; low MP → `"mp"`.
- [ ] A manual move, releasing the button or selecting another slot cancels a pending approach.
- [ ] Held button with the selected skill on cooldown reports "slash instead"; once ready it casts again.
- [ ] `teleport` jumps 3 tiles; a later reconcile at the next ack causes no further correction.
- [ ] `world_tick_cast` fixture through `apply_tick`: one cast effect, two damage numbers, no slash effect.
- [ ] Commit: `Add the cast loop, dash snapping and cast events`.

### Task 16: Client — character window, skill bar and effects

**Files:** `ui/char_window.gd`, `ui/info_tab.gd`, `ui/potential_tab.gd`, `ui/skill_tab.gd`, `ui/skill_bar.gd`, `ui/skill_effect.gd` (all new), `ui/stats_panel.gd` (removed), `ui/hud.gd`, `ui/joystick.gd`, `autoload/config.gd`, `screens/world.gd`, `screens/world_entities.gd`.

**Behaviour (§6.2):**
- `config.gd`: actions `slot_1..slot_5` (`KEY_1..KEY_5`, select a bar slot) and `skills` (`KEY_K`); `attack` (Space) uses the selected slot; `stats` (C) stays and opens the window on Thông tin.
- `char_window.gd`: five tabs in order Trang bị, Thông tin, Tiềm năng, Kỹ năng, Hành trang; `open(tab)`, `close()`; the same key again closes; remembers the last tab for the session; dots on Tiềm năng/Kỹ năng when points are free. Trang bị and Hành trang show "Có ở bản sau".
- `info_tab.gd` and `potential_tab.gd` split `stats_panel.gd` without behaviour change (the allocation flow `confirmed → AllocatePointsReq → allocated/failed`, disabled while dead or waiting).
- `skill_tab.gd`: rows per §6.2; "Học"/"Nâng" emits `learn(skill_id)`; the caller sends `LearnSkillReq` and calls `learned(resp)` or `failed(code)`; a locked button shows the reason (`learn_block`).
- `hud.gd`: "Chỉ số" becomes "Thông tin"; its dot shows when free potential or skill points exist.
- `skill_bar.gd`: five slots in one row at the bottom centre per §6.2 (slot 1 default slash, slots 2–5 the four actives); selecting highlights the slot until another is chosen (default slot 1); unlearned slots are grey and not selectable; cooldown sweep and seconds; dark when MP is short; emits `selected(slot)`.
- `hud.gd` bottom-right: the Đánh button in the centre, the HP and MP potion buttons above it, greyed, each showing "Có ở bản sau" when pressed; "Đổi mục tiêu" and "Thông tin" stay beside Đánh.
- Look (spec §6.2, from the user's reference screenshots; layout only, no copied art): square dark slots with a gold border and the key number above each; the selected slot's border brighter; round large Đánh button; small round potion buttons with a count label; "Đổi mục tiêu" and "Thông tin" as small round buttons left of Đánh.
- `ui/joystick.gd`: redraw as a ring with four direction arrows (up/down/left/right); input stays analog as in Phase 1.
- `skill_effect.gd`: `_draw` shapes sized from the skill data, class colours from §6.2, freed after about 0.35 s; dash draws a streak from the old to the new position.
- `world.gd`: wire slot keys and the bar to the selected slot, the Đánh button/Space to `attack_loop` (slot 1) or `cast_loop` (slots 2–5), send `CastReq`/`LearnSkillReq`, handle `CastResp` (start cooldown) and errors (notice text), own `EntityMove` with `snap` while alive → `_predictor.teleport`, `EnterWorldResp.skills` and `SelfStats.skill_points` into the skill book.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code moves into `core/`.
- [ ] Manual check on a level-90 dev character against a :8199 server: every tab opens, learning works, selecting each slot and holding Đánh casts it and draws its effect (slash fills cooldown gaps), potion buttons show the notice, dash moves the character without rubber-banding.
- [ ] Commit: `Add the tabbed character window, skill bar and skill effects`.

### Task 17: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `tools/smoke.sh`, `README.md`.

**Behaviour (§6.3):** after today's kill, the smoke leaves the world; `tools/smoke.sh` runs `go run ./cmd/devchar -u <user> -level 20` in `server/` with the smoke's username (pass it through a file or an argument the smoke prints), and the smoke logs in again, learns slot 1 and slot 2, casts slot 1 on a monster and prints `SMOKE OK (… skill <name> hit <dmg>, MP −<cost>)`. Keep the watchdog.

**TODO:**
- [ ] Run against a :8199 server built from server HEAD; paste the OK line in the task report.
- [ ] README: keys 1–5 (select), Space/Đánh, K, C and the Thông tin window; how to raise a dev character.
- [ ] Commit: `Extend the smoke test with learning and casting a skill`.

---

## Acceptance (controller, after Task 17)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Start a :8199 server from HEAD (own PID only). Make sure `test1` and `test2` are not logged in on any server, then `go run ./cmd/devchar -u test1 -level 90` and `-u test2 -level 90`.
- [ ] Tell the user how to open two windows on :8199 and what to try.
