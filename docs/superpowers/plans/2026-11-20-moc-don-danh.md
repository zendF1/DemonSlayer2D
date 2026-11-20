# Attack Phases (Windup, Flight, Impact) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every attack (25 skills, the default slash, monster melee and monster shots) is decided when it is pressed but deals its damage only when it lands: windup, then flight, then impact. The server applies damage and sends `Hit` at the impact tick; the client animates the three phases from a richer `Cast` event.

**Architecture:**
- Timing data lives in generated data (`skills.json`: `windup_ms`, `projectile_speed`; `monsters.json`: `melee_windup_ms`, `shot_speed`) plus `combat.SlashWindup`.
- The zone gets an impact queue. `resolveAttacks` and `thinkMonsters` validate, spend, snapshot targets and damage, schedule impacts and emit `Cast`; a new `runImpacts` step (right after `drain`) applies due impacts: damage, `Hit`, EXP, aggro, death.
- Protocol 5: `Cast` gains `windup_ms`, `impacts` (target + impact_ms) and `ranged`; slash and monster attacks emit `Cast` too. The client's new `core/attack_timeline.gd` turns `Cast` + `Hit` into the three visual phases.

**Tech stack:** Go, protobuf (buf), Postgres (e2e), Python 3 balance scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-moc-don-danh-design.md](../specs/2026-11-20-moc-don-danh-design.md). Section references (§n) point to it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents or the controller may edit docs in the same tree. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .`. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 6121c83, client HEAD 4e668b2)

**Server**
- Tick order today (`internal/zone/zone.go` `advance`): `drain → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView`.
- Player attacks: `resolveAttacks` → `resolveAttack` (slash) and `resolveCast` (`internal/zone/skill.go:53`). Both end in `strike(p, m, base, level, skillID)` (`internal/zone/combat.go:202`), which computes `combat.Damage`, subtracts HP, appends `Hit`, records `m.attackers`, pays EXP (`payExpShare`, which applies the ±10 window), starts a chase, and kills at 0 HP. The slash emits no `Cast` today.
- `resolveCast` builds the `Cast` event (`skill.go:~73-83`) with caster, skill, target, dir, level; victims come from `aim`/`victims`; dash moves the caster in `dashLanding` (`skill.go:175`) before striking.
- Monster attacks: `shoot` (`internal/zone/monster.go:159`) calls `z.damagePlayer(p, m.id, combat.MonsterRangedDamage(...), true)`; melee in the attack state (`monster.go:~318-322`) calls `z.damagePlayer(t, m.id, combat.Damage(...), false)`. `damagePlayer` (`combat.go:37`) appends the `Hit` (with `Ranged`) and kills the player at 0.
- **Monsters respawn with a new id** (`spawn.go` `spawn` uses `z.cfg.NewID()`), and corpses are removed `corpseSec` after death. So an impact only needs to check that its target id still maps to a living monster.
- `killMonster` (`combat.go:220`) clears `m.attackers`; `killPlayer` clears monster targets and attackers.
- `z.ticks(sec)` rounds up, minimum 1. `combat.SlashRange 1.5`, `SlashCooldown 800ms`.
- `gateway.ProtocolVersion` is 4. Fixtures via `go test ./internal/gateway -run TestClientFixtures -update`.
- Data scripts: `docs/balance/skill_stats.py` → `data/skills.json`; `docs/balance/monster_stats.py` → `data/monsters.json` (kinds list + levels); `gamedata` validates with strict decoding and presence checks (`internal/gamedata/skills.go`, `gamedata.go`).

**Client**
- `core/entity_table.gd` `apply_tick`: casts first (`{"type": "cast", ...}` effects incl. `level`), then hits; a `Hit` with `ranged` yields `{"type": "projectile", from, to, damage, self_hit}`; a `Hit` with `skill_id` yields only a damage number; a plain `Hit` yields the slash effect + number.
- `screens/world_entities.gd` draws effects: `ui/projectile_effect.gd` for monster shots (fixed 0.3 s, number shown when it lands), `ui/skill_effect.gd` for casts, `ui/slash_effect.gd`, `ui/damage_text.gd`.
- `core/interpolator.gd` `add(..., snap)` drops samples on snap; `core/predictor.gd` `teleport`.
- `autoload/net.gd` `PROTOCOL_VERSION := 4`; `tools/sync_server.sh` syncs proto, fixtures, maps, skills.json.

## File structure

Server:
```
docs/balance/skill_stats.py, data/skills.json          windup_ms, projectile_speed (Task 1)
docs/balance/monster_stats.py, data/monsters.json      melee_windup_ms, shot_speed (Task 1)
internal/gamedata/*.go, gamedata_test.go               fields + validation (Task 1)
internal/combat/combat.go                              SlashWindup (Task 1)
proto/ds/v1/protocol.proto, internal/pb, gateway, testdata/client   protocol 5 (Task 2)
internal/zone/impact.go (new)                          impact queue, runImpacts, timing helpers (Task 3)
internal/zone/combat.go, skill.go, zone.go             player attacks via impacts, slash Cast (Task 3)
internal/zone/monster.go                               monster attacks via impacts + Cast (Task 4)
internal/zone/*_test.go, e2e/e2e_test.go               tests (Tasks 3, 4)
docs/game-design.md, CLAUDE.md, region spec            docs (Task 5)
```
Client:
```
server/, proto/protocol.gd, tests/fixtures, data/skills.json, autoload/net.gd   sync, protocol 5 (Task 6)
core/attack_timeline.gd (new), core/entity_table.gd   timeline + events (Task 6)
ui/projectile_effect.gd, ui/skill_effect.gd, ui/slash_effect.gd, screens/world_entities.gd, screens/world.gd   visuals (Task 7)
tests/unit/test_core.gd, tests/unit/test_net.gd, tests/smoke/smoke.gd   tests (Tasks 6, 7)
```

---

### Task 1: Server — timing data

**Files:** `docs/balance/skill_stats.py`, `data/skills.json`, `docs/balance/monster_stats.py`, `data/monsters.json`, `internal/gamedata/skills.go`, `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`, `internal/combat/combat.go`.

**Behaviour (§2.3–2.4):**
- `skills.json` actives gain `windup_ms` (int > 0) and `projectile_speed` (float ≥ 0, tiles/s): single non-dash slots 1–2 → 250/20; dash → 100/25; cone, line → 250/20; circle_target → 300/20; circle_self → 300/0; single slot 4 → 400/20. Passives have neither.
- `monsters.json` kinds gain `melee_windup_ms` (200) and `shot_speed` (15).
- Validation: windup > 0; speed ≥ 0; dash speed > 0; circle_self speed == 0; passives must not carry them (presence check like the other active fields); monster kind fields positive. `monsters.json` levels and `exp.json` must stay byte-identical (verify).
- `combat.SlashWindup = 150 * time.Millisecond`.

**TODO / tests:**
- [ ] Regenerate both JSON files; the generated-data compare test stays green.
- [ ] Reject cases: windup 0 / missing; negative speed; dash speed 0; circle_self speed 0.5; passive with `windup_ms`; monster kind missing `shot_speed`.
- [ ] Repo-data test: every active's windup/speed matches the §2.3 table (by shape/slot/dash).
- [ ] Commit: `Add windup and projectile speed data for every attack`.

### Task 2: Server — protocol 5

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§4):** `Cast` gains `int32 windup_ms = 7`, `repeated CastImpact impacts = 8`, `bool ranged = 9`; new `CastImpact { uint64 target_id = 1; int32 impact_ms = 2; }`. Update comments (`skill_id` empty = basic attack; `Hit` is sent at the impact tick). `ProtocolVersion = 5`. Fixtures: update `world_tick_cast` (windup + two impacts), add `world_tick_monster_attack` (monster `Cast` with `ranged`, one impact), `world_tick_impact` (only a `Hit`). Round-trip tests for the new fields. The zone does not fill the new fields yet.

**TODO:** [ ] `make proto`; godobuf-safe names. [ ] Fixture round-trip tests. [ ] Version test → 5; e2e still logs in. [ ] Commit: `Add attack timing to Cast and bump the protocol to 5`.

### Task 3: Server — impact queue and player attacks

**Files:** `internal/zone/impact.go` (new), `internal/zone/combat.go`, `internal/zone/skill.go`, `internal/zone/zone.go`, `internal/zone/combat_test.go` (and `zone_test.go` if helpers live there), `e2e/e2e_test.go` if waits need adjusting.

**Interface (internal):**
- `type impact struct { due, seq uint32; attacker uint64; byMonster bool; target uint64; damage int; skillID string; ranged bool }` and a zone field holding pending impacts ordered by `(due, seq)` (slice + sort on insert, or a heap — keep it simple).
- `func (z *Zone) schedule(im impact)`; `func (z *Zone) runImpacts()`; `func (z *Zone) impactTicks(windupMS int, flight float64) uint32` = `z.ticks((windup_ms/1000) + flight)`.
- Split `strike` into: damage at press (`combat.Damage(base, monsterDefense, level)`) and `applyToMonster(im)` at impact.

**Behaviour (§2, §3):**
- `advance`: `drain → runImpacts → resolveAttacks → …`.
- Slash: after the existing checks and cooldown, compute damage, schedule one impact at `t0 + ticks(SlashWindup)`, emit `Cast{caster, skill_id "", target, dir to target, level 0, windup_ms 150, impacts [{target, impact_ms}]}`. `AttackResp` unchanged.
- Skills: after the existing checks/spend/victims (and the dash move), compute each victim's damage now, its flight per §2.2 (single: caster→target distance / speed; cone/line: caster→that victim; circle_target: caster→primary for all; circle_self: 0; dash: start→landing / speed), schedule impacts, and fill `Cast.windup_ms` and `impacts` with `impact_ms = (due − t0) × 1000 / tickRate`.
- `runImpacts` on a monster target: skip if missing or dead; else subtract `min(damage, hp)`, append `Hit` (skill_id), and if the attacker is a player still in the zone: `m.attackers[p]`, `payExpShare`, start chase if not chasing/attacking; kill at 0 HP (later impacts in the same tick then skip).
- No HP change, `Hit`, EXP or aggro in the press tick.

**TODO / tests** (`// ____TEST IMPACTS____`):
- [ ] Slash: no `Hit` and full HP in the press tick; `Hit` exactly at `t0 + ticks(0.15)`; `Cast` with empty skill_id, windup 150, one impact.
- [ ] Single skill at 5 tiles: impact at `t0 + ticks(0.25 + 5/20)`; `impact_ms` matches.
- [ ] Cone with monsters at 2 and 4 tiles: two different impact ticks, nearer first.
- [ ] circle_target: all victims share one tick. circle_self: all at `ticks(0.3)`.
- [ ] Dash: caster moved at press (snap), hit at `ticks(0.1 + travel/25)`.
- [ ] Monster killed by another impact first: later impact skipped, no `Hit`, no EXP; the first impact is the killer (two players).
- [ ] Attacker leaves before impact: damage lands, no EXP, no chase on the departed player.
- [ ] Monster aggro happens at impact, not at press.
- [ ] Update existing combat/cast/EXP tests that expected same-tick hits (advance ticks until the impact); keep their intent.
- [ ] Commit: `Land player attacks at their impact tick`.

### Task 4: Server — monster attacks through impacts

**Files:** `internal/zone/monster.go`, `internal/zone/impact.go`, `internal/zone/combat.go`, tests, `e2e/e2e_test.go`.

**Behaviour (§2.2–2.3, §3.3–3.4):**
- Melee (attack state): when `nextAttack` is due and `canHit`, compute damage now, schedule an impact at `t0 + ticks(melee_windup_ms)`, emit `Cast{caster monster, skill_id "", target, dir, windup_ms, impacts}`; `nextAttack` cadence unchanged.
- Shot: compute ranged damage now, flight = distance / `shot_speed`, schedule at `t0 + ticks(flight)` (min 1 tick), emit `Cast{..., ranged true, windup_ms 0, impacts}`.
- `runImpacts` on a player target: skip if the player left or is dead; else `damagePlayer(p, attacker, damage, ranged)` (it appends the `Hit`). A monster that died meanwhile still lands its shot. A player who stepped into a safe zone after the attack started is still hit.
- Monster `Cast` events are sent to viewers who see the monster (existing `addEvents` rule by caster visibility).

**TODO / tests** (`// ____TEST MONSTER IMPACTS____`):
- [ ] Melee: no damage in the decision tick; `Hit` at `t0 + ticks(0.2)`; `Cast` emitted.
- [ ] Shot at 5 tiles: `Hit` with `ranged` at `t0 + ticks(5/15)`; `Cast.ranged`.
- [ ] Player dies or leaves before impact: impact skipped. Monster killed before its shot lands: shot still lands. Player enters safe zone after the shot: still hit.
- [ ] Existing monster AI tests (shoot cadence, melee, anti-kite, give-ups) updated to the delayed hits; keep intent.
- [ ] e2e: tests waiting for monster hits or kills still pass (they wait on events; adjust timeouts only if needed).
- [ ] Commit: `Land monster attacks at their impact tick`.

### Task 5: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `docs/superpowers/specs/2026-11-20-quai-thuong-vung-va-chuong-design.md`.

**TODO:** [ ] game-design §2.4 → the three-phase model (decided at press, damage at impact, animations never block the next command); §5.8 → the killer is the hit that lands first. [ ] Region spec: "Đã thay bằng [spec ba mốc](2026-11-20-moc-don-danh-design.md)" note where shots deal damage at once and the client draws a 0.3 s orb. [ ] CLAUDE.md: tick order `drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView`. [ ] Vietnamese prose for docs, matching style. [ ] Commit: `Document the attack phases`.

### Task 6: Client — sync, protocol 5 and the attack timeline

**Files:** `server` submodule, `proto/protocol.gd`, `tests/fixtures/`, `data/skills.json`, `autoload/net.gd`, `core/attack_timeline.gd` (new), `core/entity_table.gd`, `tests/unit/test_core.gd`, `tests/unit/test_net.gd`.

**Interface:**
- `attack_timeline.gd` (RefCounted): `func on_cast(cast_dict, now) -> Array` (effects to start: windup on the caster; one flight per impact with `land_at`), `func on_hit(attacker, target, damage, now) -> Array` (effects: finish the matching flight early or explode a waiting one; or an immediate impact when nothing matches), `func update(now) -> Array` (flights that reached `land_at` switch to waiting; waiting ones older than `land_at + 0.4` expire → fade, no number). Constants: `RUSH_SEC = 0.1`, `WAIT_SEC = 0.4`. Flights keyed by (attacker, target); a second flight on the same pair queues FIFO.
- `entity_table.gd`: casts now carry `windup_ms`, `impacts`, `ranged`; monster casts produce effects too; a `Hit` produces only `{"type": "hit", attacker, target, damage, ...}` — no slash/projectile from `Hit` any more. HP bars still change at the `Hit`/`hp_changes`.
- `PROTOCOL_VERSION := 5`.

**TODO / tests** (`# ____TEST ATTACK TIMELINE____`, `# ____TEST PHASE EVENTS____`):
- [ ] Fixture `world_tick_cast`: windup end and both `land_at` values from receipt time.
- [ ] `Hit` before `land_at`: rush; after `land_at`: explode at once; none by `land_at + 0.4`: fade with no number.
- [ ] Unmatched `Hit`: immediate impact effect + number.
- [ ] `world_tick_monster_attack`: a ranged flight from the monster; `world_tick_impact`: only a hit.
- [ ] Commit: `Sync protocol 5 and add the client attack timeline`.

### Task 7: Client — three-phase visuals

**Files:** `screens/world_entities.gd`, `screens/world.gd`, `ui/skill_effect.gd`, `ui/projectile_effect.gd`, `ui/slash_effect.gd`, `ui/damage_text.gd` (if needed), `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§5.2):**
- Windup: class-coloured charge ring on the caster for `windup_ms`; slash: a short swing arc; monsters: a brief flash/lunge.
- Flight: projectiles home on the target's current drawn position and land at `land_at`; cone/line: a wave expanding to the skill reach over the longest impact time (or reach/speed when there are no impacts); circle_target: projectile to the primary, then the ring burst; circle_self: ring burst at windup end; monster shot: `projectile_effect` driven by the timeline (no more fixed 0.3 s).
- Impact: flash at the target and the damage number when the timeline says so.
- Dash: own character's sprite slides from the old to the new position over the dash flight (predictor already teleported; offset the drawing, camera follows); other players: a dash `snap` slides over the flight time from the `Cast` instead of jumping.
- Smoke: assert the skill's `Hit` arrives no sooner than `windup_ms` after its `Cast` (live :8299 run; kill only your PID).

**TODO:**
- [ ] Unit tests stay green; `core/` stays Node-free.
- [ ] Scratch harness screenshots of each phase (windup, flight, impact) for a single, a cone, a circle_target and a monster shot.
- [ ] Live smoke OK line in the report.
- [ ] README: one paragraph on the three phases.
- [ ] Commit: `Animate attack windup, flight and impact`.

---

## Acceptance (controller)

- [ ] Server full `-race` suite incl. DB/e2e, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID), keep `test1`/`test2` at level 90, and ask the user to check each skill's three phases and monster shots in two windows.
