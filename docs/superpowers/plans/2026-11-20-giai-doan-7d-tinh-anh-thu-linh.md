# Phase 7d Elites and Bosses Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One elite or boss per training map — Quỷ lang thang đầu đàn (6, Kumotori), Quỷ Đền (10, Sagiri 1), Quỷ săn đêm (15, Sagiri 2), Quỷ Rừng tinh anh (20, Fujikasane) — with rank stats from game-design 5.9, aggressive AI, data-declared skills and traits, telegraphed area skills that can be dodged, 5/10 minute respawns per zone, rank loot, and a special-spawn API for the event phases. The client draws bigger bodies, rank labels, the warning areas and a boss HP bar.

**Architecture:**
- Data: `monster_stats.py` writes `elite`/`boss` stat blocks into every `levels` row and four kinds with `skills`/`traits`; `mapgen.py` writes a `bosses` list per map. `gamedata` validates skills, traits and the map cross-refs; `mapdata` validates boss areas.
- Pure rules in `internal/combat`: `CountedDamage`, `Enraged`, `MonsterSkillDamage`.
- Zone: boss spots become internal one-monster regions; rank-aware stats via `Level.ForRank`; an aggressive AI branch with skill choice; an "area" impact kind whose victims are chosen at the impact tick; traits after each hit; the counted-damage cap; `SpawnSpecial`/`DespawnTag`.
- Protocol 19: `MonsterRank`, `AreaShape`, `CastArea`, `EntityEnter.monster_rank = 18`, `Cast.area = 10`. No new envelopes or errors.
- Client: `core/monster_book.gd`, `core/boss_bar.gd`, warning effects from `Cast.area`, rank visuals.

**Tech stack:** Go, protobuf (buf), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md](../specs/2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 7c plan](2026-11-20-giai-doan-7c-bang-hoi.md) is fully implemented (protocol 18, migration 00011). 7d touches none of 7a–7c's code, so it can be rebased onto an earlier HEAD if needed; bump the protocol to one above whatever HEAD has.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–7c are designed, not yet implemented)

Later phases move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Data**
- `docs/balance/monster_stats.py`: `KINDS` (`:43`) lists kind dicts with `rank`; `monster(level)` (`:106`) builds a `levels` row `{level, hp, attack, defense, exp}`; `to_json` (`:127`) writes `{"kinds", "levels"}`. Current values match game-design 5.9 for the multipliers: level 6 normal 316 HP / 67 atk / 48 def / 47 EXP → elite 1,896 / 101 / 60 / 282; level 10 → elite 4,500, boss 15,000 / 190 / 120 / 3,575; level 20 → elite 16,224, boss 54,080.
- `internal/gamedata/gamedata.go`: `MonsterKind` already has `Rank` (`:57`), validated against `rankList` (`:121`, constants in `items.go:44`); `Level` (`:62`) holds `HP, Attack, Defense, Exp`. Add the `Elite`/`Boss` blocks and `ForRank` there; `Parse` uses `DisallowUnknownFields`, so the new JSON fields must exist in the structs before the data is regenerated.

**Zone**
- `internal/zone/monster.go`: `monster` (`:50`) keeps `region *regionState`, `dmgBy`, `expPaid`; `fight` (`:285`) does chase → `canHit` → melee with `combat.Damage(z.area.Levels[m.level].Attack, …)` and `monsterAttack` (`:333`) schedules an impact and appends a `Cast`. The new skill choice goes in front of the melee step; the aggressive acquire goes where `nearestShootable` (`:130`) is used for normal monsters.
- `internal/zone/spawn.go`: `regionState` (`:15`), `newRegionState` (`:22`), `spawnAll` (`:68`), `spawn` (`:107`), `runSpawnTimers` (`:145`); `killMonster` (`internal/zone/combat.go:216`) appends `m.region.Kind.RespawnSec` to `region.pending` — boss spots reuse this with count 1 and the nest = spawn point; special spawns must skip it.
- `internal/zone/impact.go`: `impact` (`:15`), `schedule`, `runImpacts` (`:37`), `applyToMonster` (`:79`, calls `payExpShare` with the counted damage), `applyToPlayer` (`:105`). Add an area-impact variant resolved in `runImpacts`; traits and `CountedDamage` hook into `applyToMonster`.
- `internal/zone/skill.go`: `dashLanding` (`:186`, `dashGap = 1`) and the shape tests used by `victims` — reuse them for monster skills with the monster's position and body.
- `internal/zone/progression.go`: `payExpShare` (`:73`) reads `z.area.Levels[m.level].Exp`; switch to `ForRank`.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/monster_stats.py, data/monsters.json (gen), internal/gamedata/*        rank stats, kinds, skills (Task 1)
internal/combat/monster.go (new or existing file), combat_test.go                   pure rules (Task 2)
tools/mapgen.py, maps/*.json (gen), internal/mapdata/*                              boss spots (Task 3)
proto/ds/v1/protocol.proto, internal/pb/, internal/gateway/*, testdata/client/*.hex protocol 19 (Task 4)
internal/zone/boss.go (new), monster.go, spawn.go, impact.go, combat.go,
progression.go, zone_test.go, combat_test.go                                         zone (Tasks 5–6)
e2e/e2e_test.go, docs/game-design.md, CLAUDE.md, README.md, earlier specs           e2e, docs (Task 7)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, data/maps/, tests/fixtures/, tools/sync_server.sh   sync (Task 8)
core/monster_book.gd (new), core/boss_bar.gd (new), core/entity_table.gd,
core/attack_timeline.gd, core/wire.gd                                                   model (Task 8)
ui/entity_view.gd, ui/warning_effect.gd (new), ui/boss_bar.gd (new),
screens/world_entities.gd, screens/world.gd                                            UI (Task 9)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md                tests, smoke (Tasks 8–10)
```

---

### Task 1: Server — rank stats, the four kinds and their skills

**Files:** `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `internal/gamedata/gamedata.go`, `internal/gamedata/monsters.go` (new if it helps), `internal/gamedata/gamedata_test.go`.

**Interface:**
- `gamedata.Stats{HP, Attack, Defense int; Exp int64}`; `Level` gains `Elite, Boss Stats`; `func (l Level) ForRank(rank string) Stats`.
- `MonsterKind.Skills []MonsterSkill` (`ID, Name, Shape, Damage string; Pct int; CooldownSec float64; Range, Radius, ConeDeg, Width float64; Dash bool; WindupMS int; Speed float64; MaxTargets int`), `MonsterKind.Traits []Trait` (`Kind string; BelowPct, HealPct, AttackPct int`); `func (s MonsterSkill) Area() bool`.

**Behaviour:** §2.1–2.2 and the load checks of §2.3. The script asserts the 5.9 table values (level 10/20/30 elite and boss HP) and the EXP examples.

**TODO / tests** (`// ____TEST MONSTERS____`):
- [ ] Script `--write-json`, `--check` clean; the printed table matches spec §8 "Script".
- [ ] Real data loads: four new kinds with the right rank, level 10 `ForRank("boss").HP == 15000`.
- [ ] Every rejection of spec §8 "gamedata" (skills and traits part).
- [ ] Commit: `Generate elite and boss stats and the first four elites and bosses`.

### Task 2: Server — pure combat rules

**Files:** `internal/combat/monster.go` (new) or the existing combat file, `internal/combat/combat_test.go`.

**Interface:**
- `func CountedDamage(dmg, hpBefore, maxHP, counted int) int` — `max(0, min(dmg, hpBefore, maxHP − counted))`.
- `func Enraged(attack, pct int) int` — `⌈attack × (100 + pct) / 100⌉`.
- `func MonsterSkillDamage(attack, pct, defense, level int) int` — `Damage(attack × pct / 100, defense, level)` (floor of the scaled attack).

**TODO / tests** (`// ____TEST MONSTER SKILLS____`):
- [ ] The Quỷ Đền walk-through of §4.6 sums to 15,000; a normal monster's counted damage is unchanged; `Enraged(190, 20) == 228`; `MonsterSkillDamage` at 150 % equals `Damage(⌊1.5 × atk⌋, …)`.
- [ ] Commit: `Add counted damage, enrage and monster skill damage rules`.

### Task 3: Server — boss spots in the maps

**Files:** `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`, `internal/gamedata/gamedata.go` (`Validate(maps)`).

**Interface:** `mapdata.BossSpot{ID, Monster string; Level int; Rects []Rect; Spawn Point}`; `Map.Bosses []BossSpot`; `func (b BossSpot) Tiles(m *Map) []Tile` (same tile rule as regions).

**Behaviour:** §2.3 (layout rule in the script; parse checks; cross-check of ranks).

**TODO / tests** (`// ____TEST BOSS SPOTS____`):
- [ ] `mapgen.py --check` clean; each training map has exactly one spot inside its highest-level region; Ubuyashiki none.
- [ ] Rejections: outside the map, touching a safe zone, 1.5 tiles from a portal, spawn outside the area, two spots sharing a tile, unreachable spawn; `Validate`: normal kind in a spot, elite kind in a region.
- [ ] Commit: `Place one elite or boss spot on each training map`.

### Task 4: Server — protocol 19

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** `MonsterRank`, `AreaShape`, `CastArea`; `EntityEnter.monster_rank = 18`; `Cast.area = 10` with comments; `ProtocolVersion = 19`. The zone fills `monster_rank` for every monster in this task (cheap, keeps fixtures honest).

**TODO / tests:**
- [ ] Fixtures of §5; regenerate all with `-update`; version test → 19.
- [ ] Commit: `Add monster ranks and telegraphed cast areas and bump the protocol to 19`.

### Task 5: Server — spawning, rank stats and the aggressive AI

**Files:** `internal/zone/boss.go` (new), `internal/zone/spawn.go`, `internal/zone/monster.go`, `internal/zone/combat.go`, `internal/zone/progression.go`, `internal/zone/impact.go`, `internal/zone/zone_test.go`.

**Interface:**
- `monster.rank`, `monster.special` (no respawn), `monster.tag`, `monster.onDeath`, `monster.counted`, `monster.healUsed`, `monster.enraged`, `monster.skillReady []uint32`, `monster.castUntil uint32`.
- `func (z *Zone) statsOf(m *monster) gamedata.Stats` (the one place that reads `Levels` for a monster).
- `func (z *Zone) SpawnSpecial(s SpecialSpawn) (uint64, error)`, `func (z *Zone) DespawnTag(tag string) int` with the contract of §4.8 (`SpecialSpawn{Kind; Level; Pos; Home; HPMul; ExpMul; Aggressive; Tag; OnDeath}`; `Aggressive` chases the nearest living player inside `Home` at once and never shoots).

**Behaviour:** §4.1–4.3, §4.5–4.8 (everything except skills): boss spots as one-monster regions spawned in `spawnAll` and respawned via `region.pending` after the rank's `respawn_sec`; aggressive acquire within `detect_range` with sight, no shooting; melee every 1.2 s with rank attack; traits after each hit in `applyToMonster`; `CountedDamage` feeding `payExpShare`; EXP base from `statsOf`.

**TODO / tests** (`// ____TEST ELITES____`):
- [ ] Spawn and respawn timing (299 s / 300 s; boss 600 s); new id; two zones keep separate monsters and timers.
- [ ] Acquire at 4.9 tiles with sight; not at 5.1 tiles or behind a wall; no shot `Cast`; leash back without regen; melee every 12 ticks at tick rate 10.
- [ ] Quỷ Đền heals once below 30 % with an `HpChange`; solo kill pays exactly 3,575 EXP; fake-data enrage raises the next hit by 20 %.
- [ ] Boss kill drops two equipment pieces and 10 stones; `kill_any` (6d) credits it.
- [ ] `SpawnSpecial`/`DespawnTag` per spec §8 (`HPMul` 3 → HP ×3, `ExpMul` 2 → base EXP ×2, `Aggressive` chase and re-pick, no respawn, `OnDeath` once, `EntityLeave` and no loot on despawn, wall position → error).
- [ ] Commit: `Spawn elites and bosses with rank stats, aggressive AI and traits`.

### Task 6: Server — monster skills and dodgeable area impacts

**Files:** `internal/zone/boss.go`, `internal/zone/impact.go`, `internal/zone/monster.go`, `internal/zone/skill.go` (shared shape helpers), `internal/zone/zone_test.go`.

**Interface:**
- `func (z *Zone) chooseSkill(m *monster, t *player) int` — index of the first ready skill whose cast range (§4.4) reaches `t`, or −1.
- `func (z *Zone) castMonsterSkill(m *monster, t *player, i int)` — `single` (with `dash`) as a targeted impact; area skills as an area impact.
- `impact` gains an area variant (`area *areaHit{shape, origin, dir, reach, radius, coneDeg, width, dash bool, attack, pct, damageKind, maxTargets, skillID}`); `func (z *Zone) resolveArea(im impact)` picks victims at the impact tick.

**Behaviour:** §4.3 step 4–5, §4.4, §4.9. The monster stands still until `castUntil`; the dash of `line` moves it after victims are picked; area impacts still fire if the caster died.

**TODO / tests** (`// ____TEST MONSTER SKILLS____`):
- [ ] Vồ: landing 1 tile short, `Hit` at t0 + ticks(0.3 + 3/25), 150 % damage, 60-tick cooldown.
- [ ] Quét móng: `Cast.area` (cone, 120°, reach 3, windup 800, empty impacts); still target hit at t0 + 8 ticks; a target that leaves before the impact tick is not hit; a player who walks in after the press is hit; five in the cone → the three nearest the origin; the monster does not move for 8 ticks; killed during windup → still fires.
- [ ] Lao tới: victims in the line at impact; the monster ends at the line end, or before the first wall.
- [ ] Safe-zone players at the impact tick are not hit; `Hit.skill_id` carries the `m_` id.
- [ ] Commit: `Let elites and bosses cast data-driven skills with dodgeable warnings`.

### Task 7: Server — e2e and documentation

**Files:** `e2e/e2e_test.go`, `internal/client/client.go` (if a helper is needed), `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-20-moc-don-danh-design.md`, `docs/superpowers/specs/2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md`.

**TODO / tests:**
- [ ] e2e `TestEliteOverWebSocket` per spec §8 (spawn position from the generated `maps/kumotori_1.json`).
- [ ] Docs per §9.
- [ ] Commit: `Add an end-to-end elite test and document the phase 7d elites and bosses`.

### Task 8: Client — sync, protocol 19 and the model

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/monsters.json`, `data/maps/`, `tests/fixtures/`, `core/wire.gd`, `core/monster_book.gd` (new), `core/boss_bar.gd` (new), `core/entity_table.gd`, `core/attack_timeline.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`monster_book.skill_name`, `rank_label`; `boss_bar.pick(entities, target_id, self_pos) -> int`; `entity_table` `warning` and `skill_name` effects).

**TODO / tests** (`# ____TEST ELITES____`):
- [ ] `PROTOCOL_VERSION := 19`; decode the three new fixtures; `sync_server.sh --check` covers `monsters.json`.
- [ ] The `entity_table`, `attack_timeline`, `boss_bar`, `monster_book` cases of spec §8 "Client".
- [ ] Commit: `Sync the phase 7d protocol and read monster ranks and warnings`.

### Task 9: Client — rank visuals, warnings and the boss bar

**Files:** `ui/entity_view.gd`, `ui/warning_effect.gd` (new), `ui/boss_bar.gd` (new), `screens/world_entities.gd`, `screens/world.gd`, `ui/hud.gd`.

**Behaviour:** §6.2. `world_entities.gd` only applies the effects `entity_table` returns; `world.gd` only wires.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server: the Kumotori elite, a Fujikasane cone dodged and not dodged, the Quỷ Đền heal. Screenshots in the report.
- [ ] Commit: `Draw elite and boss bodies, warning areas and the boss HP bar`.

### Task 10: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3).** Print `SMOKE OK (… elite down)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: elites and bosses, warnings.
- [ ] Commit: `Extend the smoke test with an elite kill`.

---

## Acceptance (controller, after Task 10)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID).
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
