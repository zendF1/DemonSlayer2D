# Phase 5a PK Mode, Player Combat and the EXP Penalty Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A PK button on the HUD. With PK on, a player's name turns red and, on training maps outside safe zones, the default slash and every skill can hit other players; a PK-off player can hit anyone with PK on. Area skills hit monsters and attackable players alike under one target cap. A PK-on player killed by a player on a training map loses 5% of the EXP to the next level, and the death panel says who did it and how much was lost. PK state survives logout and zone changes.

**Architecture:**
- Pure rules in `internal/combat`: `PvPAllowed`, `PvPDamage` (factor 1.0), `PkExpLoss`. Map-type rules in `mapdata` (`PvP()`, `PkPenalty()`), copied into `zone.Area`.
- Migration 00008 adds `characters.pk_on`; `Character`/`CharState`/`PlayerInfo`/handoff carry it, and the 10 s PvP combat lock travels as the `pvp_combat` cooldown.
- The zone resolves attack and cast targets to "a monster or a player"; one helper checks a player target (`INVALID_TARGET`, `PK_FORBIDDEN`, `IN_SAFE_ZONE`); `victims` mixes monsters and players. Impacts gain a player→player kind; `killPlayer` records the killer, applies the penalty and queues a `DeathNotice`.
- `LookChange` becomes "the entity's whole look at the end of the tick" (weapon glow and PK), recorded by id like `hpChanges`.
- Protocol 12: `SetPkReq/Resp`, `DeathNotice`, `EntityEnter.pk_on`, `LookChange.pk_on`, `SelfStats.pk_on`, `PK_FORBIDDEN`, `IN_PVP_COMBAT`.
- The client filters target candidates with `core/pk_rules.gd`, auto-locks a player who hits it while it has no target, and adds the PK button, red names, the death panel lines and negative EXP.

**Tech stack:** Go, protobuf (buf), Postgres (goose migrations); Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-5a-pk-phat-exp-design.md](../specs/2026-11-20-giai-doan-5a-pk-phat-exp-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 4c plan](2026-11-20-giai-doan-4c-kho-luong-hoi-sinh.md) is fully implemented (protocol 11, migration 00007, `World.Transfer` with the handoff and `Cooldown`s, `LookChange` from 3b, respawn in place). Start from a server HEAD that contains it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 79e0468 (phase 3a in progress) plus the phase 3–4c specs; client HEAD 5d275f7)

Phases 3 and 4 land before this plan and rename or add many symbols below. Find each by name (`graphify explain "<symbol>"`) before editing.

**Combat and data**
- `combat.Damage(base, defense, attackerLevel)` (`internal/combat/combat.go:90`) is `max(1, floor(base × (1 − d/(d + 20L))))`; `SlashBase` (`:146`) is `max(PhysAttack, MagicAttack)`; `SkillBase(s, magic, coefPct)` (`:136`). Players have separate `PhysDefense`/`MagicDefense` in `combat.Stats`; monsters have one `Levels[level].Defense`. `ExpTable.ToNext` (`:155`) is 0 at `MaxLevel`; `Gain` (`:166`) already works from a negative `exp`.
- `gamedata.Skill.Damage == gamedata.DamageMagic` picks magic (`skill.go:86`). `mapdata.Map.Type` exists (`internal/mapdata/mapdata.go:23`); `validTypes` (`:69`) is `town`, `training`, `battle`, `instance`.
- Migrations live in `migrations/` (embedded by `migrations/embed.go`); `characters.exp` is a plain `bigint` with no sign check (`00001_init.sql:20`), so negative EXP needs no schema change. The next file after 4c is `00008_pk_mode.sql`.

**Zone combat (line numbers at 79e0468)**
- Tick order `drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView` (`internal/zone/zone.go:364`, `advance`).
- `checkAttack` (`combat.go:173`) returns a `*monster` only; `slash` (`:196`) schedules `impact{attacker, target, damage}` with `damageTo(m, …)` (`:208`). `resolveCast` (`skill.go:54`), `aim` (`:150`) and `victims` (`:231`) all take `*monster`; `monstersInBox` (`:294`) filters cells by `ENTITY_KIND_MONSTER` — add a players-in-box twin or a kind-agnostic box query. `flightDist` (`:105`) takes `primary, m *monster`.
- `impact` (`impact.go:15`) has `byMonster bool` meaning "monster → player"; `runImpacts` (`:37`) dispatches to `applyToPlayer` (`:105`) or `applyToMonster` (`:79`). A player→player kind is new; `applyToMonster` is where a player's hit pays EXP (`payExpShare`).
- `damagePlayer` (`combat.go:37`) builds the `Hit` without `SkillId`; `killPlayer` (`:51`) clears monster targets and appends a `Death`. `syncView` (`zone.go:653`) sends `WorldTick` then `SelfStats` when `statsDirty`; send `DeathNotice` between them.
- `hpChanges` (`combat.go:31`) is the "record the id, fill at send" pattern to copy for looks. 3b adds `looks []*pb.LookChange` snapshots and `LookChange{id = 1, weapon_enhance = 2}` — confirm the field numbers in the proto before adding `pk_on = 3`.
- `selfStats` (`progression.go:173`) builds `SelfStats`; `state` (`zone.go:548`) builds `CharState`.

**Client**
- `screens/world_entities.gd:190` `candidates()` returns live monsters only, from `entity_table.live_monsters()` (`core/entity_table.gd:64`). `core/targeting.gd` works on any `{id, pos}` list (`nearest`, `press`, `pick_at`, `cycle`, `update`, `on_death`); `update` drops a target missing from the list, which gives "stop targeting a player who is no longer attackable" for free.
- `screens/world.gd:145` `_process` builds `candidates` once per frame and passes them to targeting, the attack loop and the cast loop. `ui/hud.gd` has the round small buttons (`_small_button`, `:333`) next to Đánh and the notice line (`show_notice`, `:398`). `ui/death_panel.gd` builds the card in `_ready`. `core/map_grid.gd:26` `parse` ignores unknown keys.

## File structure

Server (`DemonSlayer2D`):

```
internal/combat/combat.go, combat_test.go                     PvP rules and formulas (Task 1)
internal/mapdata/mapdata.go, mapdata_test.go                  PvP()/PkPenalty() (Task 1)
migrations/00008_pk_mode.sql (new), internal/store/*          pk_on column (Task 2)
proto/ds/v1/protocol.proto, internal/pb/                      protocol 12 (Task 3)
internal/gateway/server.go, gateway_test.go, testdata/client/ version, fixtures (Task 3)
internal/zone/zone.go, stats.go, progression.go, combat.go    PK state, SetPk, looks (Task 4)
internal/zone/transfer.go, internal/world/world.go            handoff, areaOf flags (Task 4)
internal/zone/combat.go, skill.go, impact.go                  player targets and PvP impacts (Task 5)
internal/zone/combat.go, lifecycle.go                         killer, penalty, DeathNotice (Task 6)
internal/gateway/handlers.go, internal/client/client.go       SetPkReq (Task 7)
e2e/e2e_test.go                                               PvP e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec       docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/protocol.gd, tests/fixtures/       sync (Task 9)
core/wire.gd, core/errors.gd                                  protocol 12 (Task 9)
core/pk_rules.gd (new), core/map_grid.gd                      PK table, map type (Task 10)
core/entity_table.gd, core/targeting.gd                       pk_on, players as candidates, on_hurt_by (Task 10)
screens/world_entities.gd, screens/world.gd                   candidate filter, SetPk, DeathNotice (Task 11)
ui/hud.gd, ui/entity_view.gd, ui/death_panel.gd               PK button, red names, death lines, EXP (Task 11)
tests/unit/test_core.gd                                       tests (Tasks 9, 10)
tests/smoke/smoke.gd, README.md                               smoke (Task 12)
```

---

### Task 1: Server — PvP rules in combat and map types

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`.

**Interface:**
- `const PvPFactor = 1.0` — the PvP damage factor of game-design 2.2.
- `func PvPDamage(base, defense, attackerLevel int) int` — `max(1, floor(base × (1 − d/(d+20L)) × PvPFactor))`; `defense` is the target's physical or magic defense by damage type.
- `func PvPAllowed(attackerPK, targetPK, sameParty bool) bool` — `!sameParty && (attackerPK || targetPK)` (§4.2).
- `func PkExpLoss(toNext int64) int64` — `toNext × 5 / 100` for `toNext ≥ 0`, else 0.
- `func (m *Map) PvP() bool` (`training`, `battle`) and `func (m *Map) PkPenalty() bool` (`training`).

**TODO / tests:**
- [ ] `// ____TEST PVP____` (combat): the eight `PvPAllowed` cases; `PkExpLoss(64000) = 3200`, `(130) = 6`, `(0) = 0`; reference Viêm level 10, 60 STR/30 VIT, no gear (use `Compute`): slash `PvPDamage(SlashBase, PhysDefense, 10) = 128`, Bất Tri Hỏa level 1 `= 192`, nine of those reach 1,640 HP and eight do not; against the Thủy reference (1,430 HP, 88 magic and physical defense) eight Bất Tri Hỏa kill; `PvPDamage` never below 1.
- [ ] `// ____TEST MAP TYPES____` (mapdata): the §2 table for the four types.
- [ ] Commit: `Add the PvP damage, PK table and EXP penalty formulas`.

### Task 2: Server — pk_on in the store

**Files:** `migrations/00008_pk_mode.sql` (new), `internal/store/characters.go`, `internal/store/store_test.go`.

**Behaviour (§3):** `pk_on boolean NOT NULL DEFAULT false`; down drops it. `Character.PkOn` scanned in the shared column list; `CharState.PkOn` written on every save.

**TODO / tests** (`// ____TEST PK MODE____`, DB):
- [ ] Migration up and down.
- [ ] A new character has `PkOn` false; save `true`, reload → `true`; save an `Exp` of −1,400, reload → −1,400.
- [ ] Commit: `Store each character's PK mode`.

### Task 3: Server — protocol 12

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 105–107; `EntityEnter.pk_on = 15`, `LookChange.pk_on = 3` (comment: the whole look at the end of the tick), `SelfStats.pk_on = 25`; comments on `AttackReq.target_id`, `CastReq.target_id`, `INVALID_TARGET`, `Cooldown.key`; `PK_FORBIDDEN = 45`, `IN_PVP_COMBAT = 46`. `ProtocolVersion = 12`. `SetPkReq` is not handled yet (default `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `set_pk_req`, `death_notice` (by a player, 3,200 EXP lost), `entity_enter_pk`; regenerate all with `-update`.
- [ ] Version test → 12.
- [ ] Commit: `Add the phase 5a PK messages and bump the protocol to 12`.

### Task 4: Server — PK state, SetPk, looks and the handoff

**Files:** `internal/zone/zone.go`, `internal/zone/entity.go`, `internal/zone/progression.go`, `internal/zone/combat.go`, `internal/zone/transfer.go`, `internal/zone/area.go`, `internal/world/world.go`, `internal/zone/zone_test.go`, `internal/world/world_test.go`.

**Interface:**
- `Area.PvP`, `Area.PkPenalty bool`; `areaOf` sets them from `mapdata.Map.PvP()`/`PkPenalty()`.
- `PlayerInfo.PkOn bool`; `player.pkOn bool`, `player.pvpUntil uint32` (the first tick at which PK may be turned off again; 0 = never locked).
- `func (z *Zone) SetPk(id uint64, seq uint32, on bool) bool` — queues a `setPkCmd`; false when the inbox is full.
- `func (z *Zone) lookChanged(e *entity)` — records the id once per tick (like `hpChanged`); `addEvents` builds each `LookChange` from the entity's current `weapon_enhance` and `pk_on`. Replace 3b's snapshot appends with calls to it.
- `func (z *Zone) pvpLocked(p *player) bool` — `tickNo < pvpUntil`; a landed PvP hit sets `pvpUntil = tickNo + ticks(10)` on both sides (Task 5).

**Behaviour:**
- `setPk` in `drain` (§4.8): `DEAD`; same state → `SetPkResp{on}` and nothing else; turning off while `pvpLocked` → `IN_PVP_COMBAT`; else flip `pkOn`, `statsDirty`, `lookChanged`, reply. No save.
- `enterEnv` fills `pk_on` for players; `selfStats` fills `pk_on`; `state` writes `PkOn`.
- Handoff (4a §3.2.1): carries `PkOn`, and while `pvpLocked` the cooldown `"pvp_combat"` with the time left (`pvpUntil − tickNo` in ms); the join side sets `pvpUntil = tickNo + ticks(left)` like the other cooldowns.
- `world.Enter` copies `Character.PkOn` into `PlayerInfo`.

**TODO / tests** (`// ____TEST PK MODE____` in zone, `// ____TEST ZONES AND TRANSFER____` in world):
- [ ] `SetPk` while dead → `DEAD`; same state → `SetPkResp`, no `LookChange`; on → `SelfStats.pk_on` and a `LookChange{pk_on: true}` with the current `weapon_enhance` to a viewer and to self; off right after a set `pvpUntil` → `IN_PVP_COMBAT`, accepted 10 s later.
- [ ] A weapon-glow change and a PK change in the same tick give one `LookChange` with both values.
- [ ] `EntityEnter` of a PK-on player has `pk_on`; the saved state has `PkOn`.
- [ ] Handoff carries `PkOn`; a lock with 4 s left still refuses turning PK off in the new zone for about 4 s and lists `pvp_combat` in `EnterWorldResp.cooldowns`.
- [ ] World: `Enter` with `PkOn` true; a portal keeps it.
- [ ] Commit: `Track PK mode in the zone and let players switch it`.

### Task 5: Server — players as attack and cast targets

**Files:** `internal/zone/combat.go`, `internal/zone/skill.go`, `internal/zone/impact.go`, `internal/zone/combat_test.go`.

**Interface:**
- `type foe struct { m *monster; p *player }` (exactly one set) or an equivalent target handle with `pos()`, `id()`, `alive()`; used by `checkAttack`, `aim`, `victims`, `flightDist`, `dashLanding`.
- `func (z *Zone) checkPlayerTarget(a *player, id uint64, needMap bool) (*player, pb.ErrorCode)` — §4.2 steps 1–4 (step 4 calls `combat.PvPAllowed(a.pkOn, b.pkOn, false)`; 5b fills the party flag).
- `func (z *Zone) damageToPlayer(b *player, base, level int, magic bool) int` — `combat.PvPDamage` with `b`'s physical or magic defense.
- `impact` gains a player→player kind (for example `targetIsPlayer bool` beside `byMonster`) and `attackerName string`.
- `func (z *Zone) applyPvP(im impact)` — skip a gone or dead target; else `damagePlayer` with `skill_id`, set `pvpUntil = tickNo + ticks(10)` on the target and on the attacker if still in the zone, and kill at 0 HP with the attacker as killer (Task 6 fills the killer bookkeeping).

**Behaviour:**
- `checkAttack` order §4.3; a monster id keeps today's path; a player id goes through `checkPlayerTarget`; range and sight use the target's position.
- `aim` §4.4: a named target is "valid" when it is a living monster or a player passing `checkPlayerTarget`. `single`/`circle_target` without a valid target answer the player error (when the id is a player) or `INVALID_TARGET`; `cone`/`line`/`circle_self` treat an invalid named player as no target.
- `victims` §4.4: monsters as today plus players (not the caster, alive, in sight of the origin, inside the area, `Area.PvP`, outside safe zones, `PvPAllowed`); one list ordered primary first, then distance to the centre, ties by lower id, cut to `max_targets`.
- Damage is fixed at the press: monsters via `damageTo`, players via `damageToPlayer` (slash physical; skills by `s.Damage`). The `Cast` lists player impacts like monster ones.
- PvP impacts never touch monster aggro, `dmgBy` or EXP.

**TODO / tests** (`// ____TEST PVP ATTACKS____`, `// ____TEST PVP SKILLS____`):
- [ ] Slash error order: self id → `INVALID_TARGET`; a non-PvP area → `PK_FORBIDDEN`; target in a safe zone → `IN_SAFE_ZONE`; both PK off → `PK_FORBIDDEN`; too far → `OUT_OF_RANGE`; cooldown → `ON_COOLDOWN`.
- [ ] A (PK on) slashes B (PK off): B's HP drops by `PvPDamage(SlashBase(A), B.PhysDefense, A.level)` at the impact tick; both see the `Hit`; B (PK off) slashes A back.
- [ ] `single` magic skill on a player uses magic defense; `single` on a forbidden player answers `PK_FORBIDDEN`; `cone` naming a forbidden player still fires along `dir` and does not hit them.
- [ ] `circle_self` by a PK-off caster around two monsters, a PK-on player and a PK-off player hits the two monsters and the PK-on player; at skill level 1 (cap 2) it keeps the two nearest to the centre whatever their kind; a player standing in a safe zone is never a victim.
- [ ] Dash on a player lands 1 tile short.
- [ ] A target that walks into a safe zone or turns PK off after the press is still hit; a target that leaves the zone is not; two impacts due the same tick: the first kills, the second is dropped.
- [ ] A monster chasing A does not switch to B when B hits A.
- [ ] Commit: `Let slashes and skills target players under the PK rules`.

### Task 6: Server — killer, EXP penalty and DeathNotice

**Files:** `internal/zone/combat.go`, `internal/zone/lifecycle.go`, `internal/zone/monster.go` (monster kills pass their name), `internal/zone/zone.go` (`syncView`), `internal/zone/combat_test.go`.

**Interface:**
- `type killer struct { id uint64; name string; byPlayer bool }`; `killPlayer(p *player, k killer)` (monster call sites pass the monster's id and kind name).
- `player.deathNotice *pb.DeathNotice` — set by `killPlayer`, sent and cleared by `syncView` after the `WorldTick` and before `SelfStats`.

**Behaviour (§4.7):** in `killPlayer`, when `k.byPlayer && z.area.PkPenalty && p.pkOn`: `loss = combat.PkExpLoss(z.area.Exp.ToNext(p.level))`; `p.exp -= loss`; `statsDirty`; `z.saver.Save(z.state(p), nil)`. Always set `deathNotice{killer_id, killer_name, by_player, exp_lost}`. The killer gets nothing.

**TODO / tests** (`// ____TEST PK DEATH____`):
- [ ] PK-on B killed by A in a `PkPenalty` area at level 10 with 1,000 EXP ends at −2,200 and one save with that EXP is queued; at level 90 nothing is lost.
- [ ] No loss for: B PK off; B PK on killed by a monster; a `PvP` area without `PkPenalty`.
- [ ] A has left the zone before its impact lands: still a player kill (penalty applies, name from the impact).
- [ ] `DeathNotice` goes to B only, after B's `WorldTick` of that tick, with A's id and name, `by_player`, `exp_lost`; a monster kill gives the monster's name and `by_player` false.
- [ ] Respawn in place (4c) after a PK death does not restore EXP.
- [ ] Commit: `Charge the PK EXP penalty and tell the victim who killed it`.

### Task 7: Server — gateway, client helper and the PvP e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** `Membership.SetPk(seq, on) bool`; gateway case `SetPkReq` (needs `inWorld`, packet limit only; a full inbox answers `RATE_LIMITED` like the other zone commands); `client.SetPk(ctx, on) (*pb.SetPkResp, error)`.

**TODO / tests:**
- [ ] Gateway: `SetPkReq` before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestPvPOverWebSocket` as §8: `DevSetLevel` both to 30 and `DevSetPlace` A at (11.5, 5.5) and B at (12.5, 5.5) on `kumotori_1` (just east of the camp `(1, 1, 10, 10)`; check both tiles are walkable and outside every safe zone in the generated map, else pick the nearest that is). Wait for `Hit`s and `DeathNotice` with a timeout; ignore monster hits (levels 1–5 hit for about 1).
- [ ] Commit: `Handle PK switching and add an end-to-end PvP test`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 2.5 and 5.7 per §9; phase 0 roadmap note (phase 5 split into 5a/5b).
- [ ] CLAUDE.md invariant per §9.
- [ ] README: PK button, rules, penalty.
- [ ] Commit: `Document the phase 5a PK mode and EXP penalty`.

### Task 9: Client — sync and protocol 12

**Files:** `server` submodule, `proto/protocol.gd`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 8; `tools/sync_server.sh`; `PROTOCOL_VERSION := 12`; `PAYLOADS` adds `set_pk_req`, `set_pk_resp`, `death_notice`; the two error texts of §6.1.

**TODO / tests:**
- [ ] Decode the three new fixtures (`DeathNotice` fields, `EntityEnter.pk_on`).
- [ ] Commit: `Sync the phase 5a protocol from the server`.

### Task 10: Client — PK rules, map type, entity table, targeting

**Files:** `core/pk_rules.gd` (new), `core/map_grid.gd`, `core/entity_table.gd`, `core/targeting.gd`, `tests/unit/test_core.gd`.

**Interface:**
- `pk_rules.gd`: `static func map_pvp(map_type: String) -> bool`; `static func can_attack(my_pk: bool, their_pk: bool, map_pvp: bool, their_in_safe: bool, same_party := false) -> bool`.
- `map_grid.gd`: `map_type: String`.
- `entity_table.gd`: entries gain `pk_on`; `self_pk: bool` from `apply_self_stats`; `live_players() -> Array[int]`; `apply_tick` applies each `LookChange` (`weapon_enhance` and `pk_on`) and returns `{"type": "look", id, weapon_enhance, pk_on}`.
- `targeting.gd`: candidates carry `kind`; `func on_hurt_by(attacker_id: int, candidates: Array) -> void` selects the attacker when `target_id == 0` and it is in the list.

**TODO / tests** (`# ____TEST PK____`):
- [ ] `can_attack` table (both off false; either on true; town false; target in safe false; same party false).
- [ ] `map_type` of the synced `ubuyashiki` is `town`, of `sagiri_1` `training`.
- [ ] `EntityEnter` with `pk_on`, then a `LookChange` turning it off, updates the entry and returns the look effect.
- [ ] `on_hurt_by` selects when there is no target, keeps an existing target, ignores an attacker not in the list.
- [ ] `nearest` over mixed monster and player candidates picks the nearest of either kind.
- [ ] Commit: `Add the PK table and players as target candidates`.

### Task 11: Client — world wiring and UI

**Files:** `screens/world_entities.gd`, `screens/world.gd`, `ui/hud.gd`, `ui/entity_view.gd`, `ui/death_panel.gd`.

**Behaviour (§6.2, §6.3):**
- `world_entities.candidates(ctx)` returns live monsters plus live players passing `pk_rules.can_attack(ctx.self_pk, pk_on, ctx.map_pvp, ctx.in_safe.call(pos))`, each `{id, pos, kind}`; `world.gd` builds `ctx` once per frame from `entity_table.self_pk`, `pk_rules.map_pvp(grid.map_type)` and `grid.in_safe_zone`.
- A `hit` effect on the own character from a player calls `targeting.on_hurt_by`.
- HUD: a small round "PK" button beside Đổi mục tiêu (grey ring off, red fill on, from `SelfStats.pk_on`); press → `Net.request(SetPkReq{on: !self_pk})`; reply → notice "Đã bật PK"/"Đã tắt PK"; error → `Errors.error_text`.
- `entity_view.gd`: player names red while `pk_on` (own included); the `look` effect updates it.
- `death_panel.gd`: `set_notice(killer_name, by_player, exp_lost)` adds "Bị <tên> hạ gục" ("Bị người chơi hạ gục" for an empty name) and, when `exp_lost > 0`, "Mất N EXP" in the error colour; the 4c buttons stay. `world.gd` routes `DeathNotice` to it (it may arrive in the same frame as the own `Death`, in either order).
- EXP bar: negative EXP draws an empty bar and labels "−1.400 / 64.000" (Vietnamese thousands separator, as elsewhere in the HUD).

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with two windows (`tools/run2.sh`): both PK off cannot fight (no target lock on tap); PK on turns the name red in both windows; PvP slashes and an area skill land; the victim's death panel shows the killer and the EXP lost; EXP bar negative. Screenshots in the report.
- [ ] Commit: `Fight other players: PK button, red names, PK target filter and death details`.

### Task 12: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.4):** after the existing steps: `SetPkReq{on: true}` → `SetPkResp.on` and `SelfStats.pk_on`; `SetPkReq{on: false}` → off. Print `SMOKE OK (… PK on/off)`. Keep the watchdog.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: PK button, who can attack whom, the EXP penalty.
- [ ] Commit: `Extend the smoke test with switching PK on and off`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID). Raise `test1` and `test2` to level 20 with `devchar` if needed.
- [ ] Tell the user how to open two windows on :8199 and what to try (spec §8 "Nghiệm thu").
