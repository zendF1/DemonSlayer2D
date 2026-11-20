# Normal Monster Regions and Ranged Shots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `kumotori_1`'s four crowded camps with four level regions. Normal monsters spread across their region, wander it, and respawn at the region's nest. A player who only passes by gets shot (always hits, 50% damage, every 2 s). A player who hits a monster gets chased; a monster that gives up keeps its HP.

**Architecture:**
- Regions are lists of tile rectangles in the map JSON. `mapdata` validates them and computes their tiles, and `world` turns them into `zone.Region` values.
- The zone goroutine still owns all monster state. `spawn.go` places monsters by best-candidate sampling and respawns them at the nest. `monster.go` gets four states (wander, shoot, chase, back).
- Ranged hits are ordinary `Hit` events with a new `ranged` flag. The client draws a projectile for them instead of a slash.

**Tech stack:** Go, protobuf (buf), Postgres for e2e; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-quai-thuong-vung-va-chuong-design.md](../specs/2026-11-20-quai-thuong-vung-va-chuong-design.md). Section references below (§n) point to it. It replaces §2.1 camps, §2.2 kind fields and §4.2 of the [Phase 1 spec](../specs/2026-11-19-giai-doan-1-chien-dau-pve-design.md).

**Plan format (from the user):** this plan gives interfaces, behaviour, test cases and TODOs. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):**
- Commit and push after every task, with a clear commit message.
- **No** `Co-Authored-By` or other signature trailer.
- Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:**
- Run Go tests with `-race`.
- Keep 1–2 test files per package, with sections split by `// ______________________TEST X_____________________` (Go) or `# ______________________TEST X_____________________` (GDScript).
- A fix comes with a test that fails on the old code.
- Before committing Go code, run `staticcheck ./...`, `go vet ./...` and `graphify update .`.

**Environment:**
- The user's dev server runs on :8099. Don't stop it.
- For your own live runs, use `LISTEN_ADDR=:8199`, and stop only the PID you started.
- Port 8080 is taken.
- DB tests use `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable'`.

---

## Verified findings (2026-11-20)

**Server code today**
- **Camp code to replace.**
  - `mapdata.Camp` and `CampTiles` live in `internal/mapdata/mapdata.go:56,253`, and `parseTriggers` validates camps (`:150-213`).
  - `gamedata.Validate` (`internal/gamedata/gamedata.go:99`) walks `m.Camps`.
  - `world.areaOf` (`internal/world/world.go:98-115`) builds `zone.Camp`.
  - `zone.Camp` and `zone.Area.Camps` are in `internal/zone/area.go`, and `campState` is in `internal/zone/spawn.go`.
- **The zone has its own `Rect`** (`zone.Rect{X, Y, W, H}` with `containsTile`) and never imports `mapdata`. Keep it that way: `world` converts rectangles.
- **Monster code that changes.**
  - `monster.home` drives both wandering (2 tiles around home) and the leash (`LeashRange` from home).
  - `monster.attackers` is a set, `map[uint64]struct{}`. It needs last-hit ticks for the 5 s fallback (§4.4).
- **Two places in `internal/zone/combat.go` change.**
  - `checkAttack` (`:161`) answers `INVALID_TARGET` for `m.state == returning`. §4.5 drops that.
  - `slash` (`:176-191`) turns only an `idle` monster on its attacker. §4.4 makes any hit start a chase unless the monster is already chasing.
- **Monster hits are created in one place,** `damagePlayer` (`combat.go:35-45`). Its `Hit` literal is the one place `Ranged` gets set. Add a parameter, or a sibling `shootPlayer`.
- **gamedata rejects unknown JSON fields** (`DisallowUnknownFields`). Renaming `aggro_range`/`leash_range` therefore has to change the script, the JSON and the Go struct in the same commit.
- **The world checks AOI cell size ≥ `AggroRange`** at `world.go:81`. This becomes `DetectRange`.

**Proto**
- `Hit` uses fields 1–4, so `ranged` takes field 5.
- `gateway.ProtocolVersion` is 2.
- `ranged` is a safe name in GDScript.

**e2e and botclient**
- They walk to `campC1 = (20, 10)` (`e2e/e2e_test.go:365`). The botclient's `-seek` default is the same point (`cmd/botclient/main.go:37`). Both move to r1's nest (22.5, 10.5).

**Client**
- `tests/smoke/smoke.gd:212-220` reads `camps` straight from the map JSON. `MapGrid` doesn't parse camps.
- `tools/sync_server.sh` copies `server/maps` to `data/maps`, and `--check` compares them.

**Existing zone tests that encode the old AI** and must be rewritten or removed in Tasks 6–7:
- `TestMonsterAggroWithinRange`
- `TestMonsterLeashesAndHeals`
- `TestReturningMonsterStuckTeleportsHome`
- `TestLeashHealResetsExpTracking`
- `TestMonsterKeepsTargetWhenOthersPass`
- `TestMonsterSwitchesToAttackerWhenTargetDies`
- the `INVALID_TARGET`-for-returning case in `TestAttackValidation`
- `TestAttackHitsAndAggro`

Tests about melee damage, line of sight, death, respawn, EXP and rest stay. Only their fixtures change.

## File structure

Server (`DemonSlayer2D`):

```
internal/combat/combat.go, combat_test.go          RangedFactor, MonsterRangedDamage (Task 1)
docs/balance/monster_stats.py, data/monsters.json   new kind fields (Task 2)
internal/gamedata/gamedata.go, gamedata_test.go     Kind fields, Validate over regions (Tasks 2, 5)
internal/mapdata/mapdata.go, mapdata_test.go        Region, RegionTiles, validation (Task 3); Camp removed (Task 5)
proto/ds/v1/protocol.proto, internal/pb/            Hit.ranged (Task 4)
internal/gateway/server.go, testdata/client/        ProtocolVersion 3, new fixture (Task 4)
maps/kumotori_1.json                                regions (Task 5)
internal/world/world.go, world_test.go              areaOf builds zone.Region; AOI check on DetectRange (Tasks 2, 5)
internal/zone/area.go                               Region replaces Camp (Task 5)
internal/zone/spawn.go                              regionState, best-candidate placement, nest respawn (Task 5)
internal/zone/monster.go                            four-state AI (Tasks 6, 7)
internal/zone/combat.go                             ranged hits, hit starts chase, no INVALID_TARGET for back (Tasks 6, 7)
internal/zone/zone_test.go, combat_test.go          tests
internal/client/fight.go, cmd/botclient/main.go     seek r1 nest (Task 5)
e2e/e2e_test.go                                     seek r1 nest (Task 5)
docs/...                                            spec notes (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), data/maps/                    sync (Task 9)
core/protocol_version (wherever PROTOCOL_VERSION lives)   3 (Task 9)
core/entity_table.gd                               projectile effect (Task 9)
ui/projectile_effect.gd                            new (Task 10)
screens/world_entities.gd                          draw projectile, delayed damage number (Task 10)
tests/unit/test_core.gd, tests/unit/<protocol test>    tests (Task 9)
tests/smoke/smoke.gd                               regions (Task 10)
README.md                                          monster behaviour line (Task 10)
```

---

### Task 1: Server — ranged damage formula

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`.

**Interface:**
- `const RangedFactor = 0.5`
- `func MonsterRangedDamage(base, defense, attackerLevel int) int`

**Behaviour (§3.1):** returns `max(1, floor(Damage(base, defense, attackerLevel) × RangedFactor))`.

**TODO / tests** (new section `// ____TEST RANGED____` in `combat_test.go`):
- [ ] A table of cases. Level-1 monster attack 30 against defense 0, 8 and 16 gives `floor(Damage/2)`.
- [ ] A case where `Damage` is 1 still gives 1.
- [ ] Commit: `Add the monster ranged damage formula`.

### Task 2: Server — new monster kind fields

**Files:**
- `docs/balance/monster_stats.py`
- `data/monsters.json`, regenerated with `python3 docs/balance/monster_stats.py --write-json`
- `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`
- `internal/world/world.go`, `internal/world/world_test.go`
- `internal/zone/monster.go`, only to follow the renames

**Interface** (`gamedata.Kind`, §2.3):
- `DetectRange float64` (json `detect_range`) replaces `AggroRange`.
- `ChaseRange float64` (json `chase_range`) replaces `LeashRange`.
- New: `ShootSec`, `ShootWindupSec` and `RegionMargin` (json `shoot_sec`, `shoot_windup_sec`, `region_margin`).
- Kept: `AttackRange`, `AttackSec`, `Speed`, `RespawnSec`.

**Behaviour:**
- Both kinds get the values in §2.3: detect 5, chase 10, shoot 2, windup 0.5, margin 3.
- Validation:
  - every time and distance field must be `> 0` (NaN rejected);
  - `region_margin` must be `>= 0`.
- The world's AOI check uses `DetectRange` and its error message says "detect range".
- The zone temporarily reads `DetectRange` where it read `AggroRange`, and `ChaseRange` where it read `LeashRange`. Behaviour doesn't change until Task 6. Zone test fixtures set their own kind values, so rename the fields there and keep the values.

**TODO / tests:**
- [ ] gamedata: parse a kind with every field.
- [ ] gamedata: each field set to 0 or a negative value is rejected with its name in the error.
- [ ] gamedata: `region_margin: 0` is accepted.
- [ ] gamedata: the old field `aggro_range` is rejected as unknown.
- [ ] world: the AOI check fails when the cell size is below `detect_range`.
- [ ] Run the script with `--print-json` and check that it matches the committed JSON.
- [ ] Commit: `Rename monster ranges and add shooting fields`.

### Task 3: Server — map regions in mapdata

**Files:** `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`.

Camps stay for now, so the build and the current map keep working. Task 5 removes them.

**Interface:**
- `type Region struct { ID, Monster string; MinLevel, MaxLevel, Count int; Rects []Rect; Nest Point }`, with JSON fields as in §2.1. `Rect` is reused, and its `id` is optional inside a region.
- `Map.Regions []Region`
- `func (m *Map) RegionTiles(r Region) []Point` returns the centres of the region's tiles, sorted by (y, x).

**Behaviour (§2.1):**
- A **region tile** is a tile that is walkable, inside at least one of the rects, and not in the safe zones expanded by 1 tile on every side.
- Validation in `parseTriggers`, after the safe zones are known. Each error names the map and the region:
  - duplicate region id;
  - a rect with `w` or `h` ≤ 0;
  - a rect outside the map;
  - `min_level` > `max_level`, or `min_level` < 1;
  - `count` < 1;
  - `count` > the number of region tiles;
  - the nest is not on a region tile (also catches a nest in a safe zone or the margin);
  - two regions share a tile.
- `gamedata.Validate` also checks each region's monster kind and its levels, the same way it checks camps.

**TODO / tests** (new section `// ____TEST REGIONS____`):
- [ ] Parse a small map with one region of two overlapping rects. `RegionTiles` must:
  - drop blocked tiles;
  - drop safe-zone tiles and the 1-tile margin around them;
  - be sorted;
  - contain no duplicates where the rects overlap.
- [ ] A table test with one case per validation error, each checking its message.
- [ ] gamedata: `Validate` rejects a region with an unknown monster, and one with a level that has no stats.
- [ ] Commit: `Parse and validate monster regions in map data`.

### Task 4: Server — `Hit.ranged` and protocol version 3

**Files:**
- `proto/ds/v1/protocol.proto`, then `make proto`
- `internal/gateway/server.go`
- `testdata/client/`: a new fixture, plus the generator or test that writes fixtures. Follow how the existing fixtures are produced.
- the gateway fixture test

**Behaviour (§5):**
- `message Hit` gains `bool ranged = 5;`, with a comment saying it marks a monster's ranged shot.
- `ProtocolVersion = 3`.
- New fixture: a `WorldTick` with one `Hit` that has `ranged = true`, named like the existing combat fixtures.
- Nothing sets `ranged` yet.

**TODO / tests:**
- [ ] The fixture test round-trips the new fixture.
- [ ] The version test (or the hello-handshake e2e) expects 3.
- [ ] `make test` passes.
- [ ] Commit: `Add Hit.ranged and bump the protocol to version 3`.

### Task 5: Server — spawn by region, respawn at the nest

**Files:**
- `maps/kumotori_1.json`
- `internal/mapdata/mapdata.go` and its test (remove `Camp`)
- `internal/gamedata/gamedata.go` (remove the camp part of `Validate`)
- `internal/world/world.go` and its test
- `internal/zone/area.go`, `internal/zone/spawn.go`, `internal/zone/monster.go`
- zone tests
- `internal/client/fight.go`, `cmd/botclient/main.go`, `e2e/e2e_test.go`

**Interface:**
- `zone.Region { ID string; Tiles []Vec2; Rects []Rect; Nest Vec2; Count, MinLevel, MaxLevel int; Kind gamedata.Kind }`, plus an unexported tile set or method `hasTile(tx, ty int) bool` built once.
- `zone.Area.Regions` replaces `Camps`.
- `regionState` replaces `campState`, with the same `present` and `pending` fields.
- `monster.region *regionState` replaces `camp`.
- `world.areaOf` builds a `Region` from `mapdata.Region`, using `RegionTiles` and converting the rects.

**Behaviour (§2.2, §4.1):**
- `kumotori_1.json`: delete `camps` and add the four regions exactly as in the §2.2 table.
- **Initial placement** (`spawnAll`, best candidate). For each of `Count` monsters:
  - take 8 random tiles from `Tiles` with `z.rng`;
  - keep the one whose distance to the nearest already-placed monster of this region is largest (the first monster takes the first sample);
  - ties go to the earlier sample.
- **Respawn** (`runSpawnTimers`): due monsters spawn at `Region.Nest`, with full HP and a random level in range.
- Old AI, until Task 6: set `home` = spawn position so wandering and the leash still work. Task 6 removes `home`.
- **Fight helpers:**
  - `internal/client/fight.go`, botclient `-seek` default and e2e: seek r1's nest (22.5, 10.5).
  - Rename `campC1` to `nestR1`.
  - Update the doc comments.

**TODO / tests:**
- [ ] mapdata: the real `kumotori_1.json` parses with 4 regions and counts 8/7/7/8. Every nest is on a region tile.
- [ ] mapdata: remove the camp tests.
- [ ] world: `areaOf` for `kumotori_1` gives 4 regions with non-empty tiles and the right nests.
- [ ] zone, new section `// ____TEST REGIONS____`:
  - `TestSpawnFillsRegions`: every monster stands on a tile of its region, with counts per region.
  - `TestSpawnSpreadsOut`: on a 20×20 open fixture region with count 8 and a fixed seed, the minimum pairwise distance is ≥ 2 tiles. Also run the real map through `world` (world test) and check ≥ 2 per region.
  - `TestRespawnAtNest`: kill a monster, then after `respawn_sec` a new one appears exactly at the nest with full HP.
- [ ] Update the existing zone fixtures from `Camps` to `Regions`.
- [ ] e2e: `TestKillMonsterOverWebSocket`, `TestCombatProgressSurvivesLogout` and the rate-limit test still pass.
- [ ] Run `make test-db`.
- [ ] Commit: `Spawn monsters across map regions and respawn them at the nest`.

### Task 6: Server — wander the region and shoot passers-by

**Files:** `internal/zone/monster.go`, `internal/zone/combat.go`, `internal/zone/combat_test.go`.

**States after this task:** `wander`, `shoot`, `chase`, `attack`, `back`. `chase` and `attack` together are the spec's "Đuổi". The old `idle` becomes `wander`. `returning` stays for now and Task 7 replaces it.

**Behaviour (§4.2, §4.3, §3.1–3.2):**
- **Wander.**
  - The monster rests a random 2–5 s.
  - Then it picks a random region tile within 6 tiles of its position, excluding the tile it stands on, and walks there at half speed.
  - On arrival or when stuck, it rests again.
  - Delete `wanderRadius` and the old home-based target code.
- **Detect** (every tick while wandering): `nearestShootable(m)` finds the nearest valid player within `DetectRange` with `!lineBlocked(grid, m.pos, p.pos)`, scanning the 3×3 AOI cells. Ties go to the lower id.
  - If one is found, the state becomes `shoot`. The monster stops, and `nextShot = tickNo + ticks(ShootWindupSec)`.
  - Wandering no longer starts a chase.
- **Shoot.**
  - The monster stays still.
  - When `tickNo ≥ nextShot`, it re-picks with `nearestShootable`.
  - If nobody is there, it goes back to `wander`, starting with a rest.
  - Otherwise it shoots: `dmg = combat.MonsterRangedDamage(level.Attack, p.stats.PhysDefense, m.level)`, a `Hit` with `Ranged: true` goes through the same path as melee (death at 0 HP included), and `nextShot += ticks(ShootSec)`.
  - Shooting never sets `target` or starts a chase.
- **Being hit** (`slash`): a monster in `wander` or `shoot` switches to chase on the attacker. The rest of chase is unchanged until Task 7.
- **Melee hits** keep `Ranged: false`.

**TODO / tests** (section MONSTER AI; rewrite the old aggro tests):
- [ ] `TestWanderStaysInRegion`: 2000 ticks with no players; every tick, every monster's tile is a region tile.
- [ ] `TestWanderReachesAcrossRegion`: over a long run, a single monster visits tiles more than 6 tiles from where it spawned.
- [ ] `TestShootPasserBy`:
  - a player steps to 4.9 tiles with a clear line;
  - the first `Hit{Ranged: true}` arrives exactly `ticks(0.5)` later, then one every `ticks(2)`;
  - damage equals `MonsterRangedDamage`;
  - the monster's position doesn't change while it shoots;
  - it never chases.
- [ ] `TestShootIgnoresFarSafeAndHidden`: nobody is shot at 5.1 tiles, inside a safe zone, or behind a wall.
- [ ] `TestShootStopsWhenPlayerLeaves`: the player walks out of range and the monster is wandering again by the next shot time.
- [ ] `TestShootPicksNearest`: with two players in range, the nearer one is shot.
- [ ] `TestShotKillsPlayer`: a player with 1 HP dies from a shot, and the normal death and respawn flow applies.
- [ ] `TestHitStartsChaseFromShoot`: hitting a shooting monster makes it chase and melee, with no further ranged hits.
- [ ] Commit: `Make normal monsters wander their region and shoot passers-by`.

### Task 7: Server — chase, give up without healing, back to the region

**Files:** `internal/zone/monster.go`, `internal/zone/combat.go`, `internal/zone/spawn.go`, `internal/zone/combat_test.go`.

**Interface changes:**
- `monster.attackers map[uint64]uint32`: the last tick each player hit it.
- `monster.chaseFrom Vec2`: where the current chase started.
- Remove `home`, `arriveHome`, `LeashRange` use and `returning`. The state `back` replaces `returning`.
- `regionState` gets `nearestTile(p Vec2) Vec2` and `nearMargin(p Vec2, margin float32) bool`.
  - `nearMargin` means p is inside some rect expanded by `margin` tiles on every side.
  - The tile search can be linear over `Tiles`; counts are small.

**Behaviour (§4.4–4.5):**
- **A hit on a monster in any state other than chase/attack** (`slash`): it starts chasing the attacker and sets `chaseFrom = m.pos`.
  - A monster already chasing keeps its target.
  - Every hit records `attackers[p.id] = tickNo`.
- **Give-up check,** at the start of each chase think. The target is dropped when any of these holds:
  - it is invalid (dead, gone, or in a safe zone);
  - `!region.nearMargin(target.pos, RegionMargin)`;
  - `distance(m.pos, chaseFrom) > ChaseRange`.
- **Fallback.** After dropping, pick the nearest valid attacker with `tickNo − attackers[id] ≤ ticks(5)` who also passes the margin check.
  - The chase continues on that player, with `chaseFrom = m.pos`.
  - With none, the state becomes `back`.
- **Back.**
  - If the monster is on a region tile, it goes straight to `wander`, starting with a rest.
  - Otherwise it walks at full speed to `nearestTile(m.pos)`.
  - HP is **not** restored, and `dmgBy`/`expPaid` are **not** cleared.
  - Stuck for `returnStuckSec` → snap to `Nest` (`snap = true`), then wander.
  - Arriving → wander.
- **Attacks on a `back` monster** (`checkAttack`) are accepted. `INVALID_TARGET` stays for missing or dead monsters only.
- **`killPlayer`** keeps clearing monster targets. The next think runs the fallback/back logic.
- **Melee and line of sight** are as today (`canHit`, `chaseStep`).

**TODO / tests** (rewrite the leash and return tests):
- [ ] `TestChaseGivesUpOutsideRegionMargin`:
  - hit a monster, then walk the player out until it is more than 3 tiles beyond the region rects;
  - the monster stops chasing within one tick;
  - its HP is unchanged and no `HpChange` is sent;
  - it walks back onto a region tile and then wanders.
- [ ] `TestChaseGivesUpBeyondChaseRange`: with the region margin large enough not to trigger, a chase stops once the monster is more than 10 tiles from where it started.
- [ ] `TestChaseGivesUpWhenTargetEntersSafeZone`.
- [ ] `TestChaseKeepsTargetWhenOthersHit`: a second player's hit does not switch the target.
- [ ] `TestChaseFallsBackToRecentAttacker`: A and B both hit. A leaves the margin, and the monster turns on B. A variant where B's hit is older than 5 s leads to `back`.
- [ ] `TestBackMonsterIsAttackable`: a slash on a `back` monster gets `AttackResp`, not `INVALID_TARGET`, and restarts the chase. Update `TestAttackValidation`.
- [ ] `TestBackStuckSnapsToNest`: a fixture where the way back is blocked; after `ticks(2)` the monster appears at the nest with an `EntityMove` that has `snap`.
- [ ] `TestGiveUpKeepsExpTracking`: replaces `TestLeashHealResetsExpTracking`.
  - Damage, give-up, then the same player kills the monster.
  - The EXP paid in total equals the monster's EXP, with no double pay and no loss.
- [ ] `TestMonsterDropsDeadTarget` and `TestRespawnedPlayerIsNotChased` still pass, adapted to the new states.
- [ ] Run `make test-db` and the botclient by hand: `go run ./cmd/botclient -fight 3` against your own :8199 server kills 3 monsters.
- [ ] Commit: `Chase attackers until they leave the region and return without healing`.

### Task 7b: Server — anti-kite (added after the Task 7 review)

The Task 7 review reproduced two zero-risk kills:
- **Safe-zone poke:** step out, slash, step back into the safe zone in the same tick.
- **Open-field kite:** the player is faster (4 vs 3.2 tiles/s) and reaches further (2.0 vs 1.5 tiles), and a monster that gives up does not heal.

The user chose to fix both (spec §1.3, §2.3, §4.4).

**Files:**
- `docs/balance/monster_stats.py` and `data/monsters.json`
- `internal/gamedata/gamedata.go`
- `internal/zone/monster.go`
- tests

**Interface:** `gamedata.Kind` gains two fields, both validated `> 0`:
- `ChaseSpeed` (json `chase_speed`), 1.2: a factor of the player move speed, used while chasing.
- `SafeWaitSec` (json `safe_wait_sec`), 5.

**Behaviour:**
- **Chase speed:** in chase and attack, a monster moves at `ChaseSpeed × cfg.MoveSpeed / TickRate` per tick. Wander and back keep using `Speed` (wander at half of it).
- **Safe-zone wait:**
  - A target that is in a safe zone, and otherwise valid (alive, in the zone), is kept while it has been in a safe zone for at most `SafeWaitSec`.
  - The monster stores the tick the target entered the safe zone, and resets it when the target is outside.
  - While waiting:
    - the margin give-up check is skipped, but `ChaseRange` still applies;
    - the monster walks towards the target with `chaseStep`; the monster grid stops it at the safe-zone edge;
    - it attacks as soon as the target is outside the safe zone, within `canHit`, and its cooldown is ready.
  - After `SafeWaitSec` continuously in the safe zone, the target is dropped, then the usual fallback or `back` applies.
  - The fallback still requires a candidate that is not in a safe zone.
- `validTarget` keeps its meaning for detection and shooting. The wait logic is separate, for example `chaseTarget(m)`, which returns the target and whether it is hittable.

**TODO / tests:**
- [ ] `TestChaseOutrunsRetreatingPlayer`:
  - hit a monster, then hold the player's direction straight away from it at full speed, in an open region with a large margin;
  - the monster lands a melee hit within 2 s;
  - the old speed must fail this test.
- [ ] `TestSafeZonePokeGetsHit`:
  - put the player one step outside the r1-like safe edge;
  - alternate step out and slash with step back in, as the review did;
  - within a few cycles the monster lands a hit;
  - the old code (immediate give-up) fails.
- [ ] `TestSafeZoneWaitExpires`:
  - the target stays in the safe zone;
  - the monster keeps the target until `SafeWaitSec`, then gives up and goes `back`/wander;
  - its HP is unchanged.
- [ ] `TestSafeZoneWaitResets`: stepping out, then back in, restarts the 5 s.
- [ ] gamedata: `chase_speed` and `safe_wait_sec` must be positive.
- [ ] Regenerate the data. Run `make test-db` and the botclient live check.
- [ ] Commit: `Make chasing monsters faster than players and wait at the safe zone edge`.

### Task 8: Server — docs

**Files:**
- `docs/superpowers/specs/2026-11-19-giai-doan-1-chien-dau-pve-design.md`
- `docs/game-design.md`, the line about normal monsters respawning, around line 616
- `docs/superpowers/specs/2026-11-20-quai-thuong-vung-va-chuong-design.md`
- `README.md`, if it mentions camps or aggro
- `CLAUDE.md`, if an invariant changed

**TODO:**
- [ ] In the Phase 1 spec, add a one-line note under §2.1 (camps), §2.2 (kind fields) and §4.2: "Đã thay bằng [spec 2026-11-20](…)".
- [ ] game-design: normal monsters respawn at their region's nest.
- [ ] New spec: set the status to "Đã triển khai phần server (date)", and record any refinements found while implementing, in the same style as Phase 1.
- [ ] Commit: `Point the phase 1 spec at the monster region design`.

### Task 9: Client — sync, protocol 3 and the projectile effect in core

**Files:**
- the `server/` submodule, moved to the server HEAD after Task 8
- `data/maps/` via `tools/sync_server.sh`
- the protocol-version constant (`PROTOCOL_VERSION`)
- `proto/protocol.gd`, regenerated the way the repo does it (see the client README)
- `core/entity_table.gd`
- `tests/unit/test_core.gd`, and the protocol fixture test

**Behaviour (§7):**
- `PROTOCOL_VERSION = 3`.
- `entity_table.apply_tick` emits, for a `Hit` with `ranged`, an effect `{type: "projectile", from: attacker_id, to: target_id, damage, self_hit}` instead of `slash` and `damage`.
  - Filtering stays as it is: unknown attackers are dropped, except when the hit is on the own character.
  - For a ranged hit from an unknown attacker on self, emit a plain `damage` effect so the number still shows.
- HP effects are unchanged and applied immediately.

**TODO / tests:**
- [ ] `tools/sync_server.sh --check` passes after the sync.
- [ ] Protocol test: the new fixture decodes with `ranged == true`.
- [ ] `test_entity_table_ranged_hit_projectile`: a ranged hit gives `projectile` and no `slash`; a melee hit still gives `slash` and `damage`.
- [ ] `test_entity_table_ranged_hit_unknown_attacker_on_self`: gives `damage` only.
- [ ] `tests/run.sh` passes.
- [ ] Commit: `Sync protocol 3 and turn ranged hits into projectile effects`.

### Task 10: Client — draw projectiles and update the smoke test

**Files:**
- new `ui/projectile_effect.gd`
- `screens/world_entities.gd`
- `tests/smoke/smoke.gd`, `tools/smoke.sh` if needed
- `README.md`

**Behaviour (§7):**
- **`projectile_effect.gd`:** a Node2D that flies a small glowing orb from a start point to a moving end point in 0.3 s, then calls a callback and frees itself.
  - It is drawn in code like the other effects, and has a reddish-purple tint.
  - The end point follows the target view's position each frame.
  - If the target view is freed mid-flight, the effect frees itself without calling back.
- **`world_entities.gd`:** a `projectile` effect spawns the orb from the attacker's view to the target's view. When it arrives, it shows the damage number, red if `self_hit`. A missing attacker view means no orb, only the number.
- **Smoke:**
  - read `regions` instead of `camps`;
  - seek the nearest region nest;
  - keep the kill-and-EXP assertion;
  - expect to be shot on the way and survive (a level-1 character).
- **README:** one line on monster behaviour: they shoot passers-by and chase whoever hits them.

**TODO / tests:**
- [ ] A scratch harness, not committed, drives `world.tscn` with a synthetic ranged hit and checks: the orb is created and freed; the number shows on arrival; freeing the target mid-flight raises no error.
- [ ] The headless load is clean.
- [ ] `tests/run.sh` passes.
- [ ] Live smoke against your own `make run LISTEN_ADDR=:8199 REGISTER_PER_HOUR=1000` prints `SMOKE OK`.
- [ ] Commit: `Draw monster projectiles and seek region nests in the smoke test`.

### Task 11: Play test with the user

Not for a subagent. The controller asks the user to:
- restart their :8099 server on the new code;
- run `tools/run2.sh --ws-url=ws://localhost:8099/ws` as `test1` and `test2`;
- check §9: monsters spread over four regions and wander; passing by gets you shot with a visible orb; hitting starts a chase; running away makes the monster give up with its HP unchanged; a dead monster comes back at the nest.

Each issue found gets its own fix commit. After that, set the new spec's status to fully implemented.
