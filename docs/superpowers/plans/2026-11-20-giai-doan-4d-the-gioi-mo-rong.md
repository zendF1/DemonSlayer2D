# Phase 4d World Expansion (Levels 21–90) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fourteen generated training maps for levels 21–90, chained by portals east of `fujikasane_1` in level order, with a camp on each (respawn, bonfire, Dược sư, Xa phu, Thủ kho) and 26 new normal monster kinds named per game-design 5.5. Xa phu everywhere reaches every map. Three new monster kind fields (`hp_pct`, `attack_pct`, `damage_type`) carry the Hầm Ngầm traits. No protocol change, no migration.

**Architecture:**
- Data only for the world: `tools/mapgen.py` declares the 14 maps with the 4a training-map layout helper (plus a five-strip layout for `yoshiwara_3`), adds the east portal to `fujikasane_1`, and regenerates every Xa phu route list. `monster_stats.py` adds 26 kinds and writes `hp_pct`, `attack_pct`, `damage_type` on every kind.
- Small server change: `gamedata.Kind` reads the three fields; the zone applies them through one stats helper per monster (`combat.ScalePct` for HP, EXP and attack; magic defense for `damage_type: magic`).
- Client: sync the data; make the Xa phu route list and the Bản đồ map list scroll.

**Tech stack:** Go, Python 3 scripts, Postgres (tests only); Godot 4.7.2, GDScript.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-4d-the-gioi-mo-rong-design.md](../specs/2026-11-20-giai-doan-4d-the-gioi-mo-rong-design.md). Section references below (§n) point to it.

**Prerequisite:** phases 4a, 4b, 4c, 6a, 6b and 6d are implemented (protocol 15, migrations up to 00009, `tools/mapgen.py`, NPC roles `shop`/`carriage`/`storage`/`quest`, `quest_data.py`). Phases 7a–7e may or may not have landed; §9 says what changes when 7d or 7e is already there. Start from a server HEAD that contains 6d.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 1b79ccf — before phases 4a–6d landed)

Phases 4a–6d add `tools/mapgen.py`, the NPC data and the quest scripts, and move code around, so line numbers below drift. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Monster data**
- `docs/balance/monster_stats.py` `KINDS` (line ~42) lists every behaviour field on every kind (no defaults in Go); `to_json` writes `{"kinds", "levels"}`. 3a adds `rank`, 7d adds `skills`/`traits` and the `elite`/`boss` blocks in `levels`.
- `gamedata.Kind` (`internal/gamedata/gamedata.go:22`); monsters are decoded with `DisallowUnknownFields` (`gamedata.go:94`), so the script and the struct must gain the three fields in the same commit. `Load` at `:139`, `Validate(maps)` at `:179`.

**Where the zone reads a monster's level stats** (all must go through the new helper):
- `internal/zone/spawn.go:113` — `stats := z.area.Levels[level]`; HP and max HP set from `stats.HP` (`:124-125`).
- `internal/zone/monster.go:169` — ranged shot: `combat.MonsterRangedDamage(z.area.Levels[m.level].Attack, p.stats.PhysDefense, m.level)`.
- `internal/zone/monster.go:322` — melee: `combat.Damage(z.area.Levels[m.level].Attack, t.stats.PhysDefense, m.level)`.
- `internal/zone/progression.go:106` — EXP share: `combat.ExpShare(z.area.Levels[m.level].Exp, m.maxHP, m.dmgBy[p.id])`.
- `internal/zone/combat.go:215` — a player's hit on a monster uses `z.area.Levels[m.level].Defense` (defense is **not** scaled; leave it, but route it through the helper for one read path).
- The player's magic defense field next to `PhysDefense` in the zone's player stats (find by name; `combat.Damage` is used for both kinds in player-vs-player code).
- 7d (if landed) replaces these reads with `Level.ForRank(rank)`; apply the percentages after picking the rank block.

**Maps (after 4a–6b, per their specs)**
- 4a §2.3: training layout helper — camp `camp` 10 × 10 at the west wall, mid height, holding the west portal, its arrival and `default` (respawn); bonfire r 1.5; `gate_east` 6 × 8 round the east arrival; regions north-mid, north-east, south-mid, south-east, lower levels nearer the camp; portals 1 × 4 at column 1 / `width − 2`, rows `height/2 − 2 … height/2 + 1`; arrival 3 tiles inside.
- 4b §2.2: Xa phu route price = 50 to `ubuyashiki`, else `max(50, 20 × lowest region level of the target)`; NPCs in the camp 2–4 tiles from `default`, ≥ 1.5 apart. 4c: `thu_kho` ≤ 3 tiles from `default`. 6b: quest NPCs only on the five 4a maps.
- 6a: client `world_graph` reads every synced map; the real-map A* and walker tests iterate over all maps automatically.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/monster_stats.py, data/monsters.json (gen)            26 kinds, three new kind fields (Task 1)
internal/gamedata/gamedata.go, gamedata_test.go                    Kind fields and checks (Task 1)
internal/combat/combat.go, combat_test.go                          ScalePct (Task 2)
internal/zone/spawn.go, monster.go, progression.go, combat.go      per-monster stats, damage type (Task 2)
internal/zone/*_test.go                                            stats and damage type tests (Task 2)
tools/mapgen.py, maps/*.json (gen)                                 14 maps, fujikasane_1 east portal, routes (Task 3)
internal/mapdata/mapdata_test.go, internal/world/world_test.go     real-map tests (Task 3)
other real-map tests that count maps or NPCs (4b, 4c, 6b)          generalise (Task 3)
e2e/e2e_test.go                                                    world expansion e2e (Task 4)
docs/game-design.md, README.md, 4a spec                            docs (Task 5)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), data/maps/ (+ data/monsters.json if synced)   sync (Task 6)
ui/npc_window.gd, ui/map_window.gd (names per 4b/6a)               scrollable lists (Task 6)
tests/unit/test_core.gd                                            world_graph on real maps (Task 6)
README.md                                                          maps (Task 6)
```

---

### Task 1: Server — 26 monster kinds and three kind fields

**Files:** `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `gamedata.Kind` gains `HPPct int \`json:"hp_pct"\``, `AttackPct int \`json:"attack_pct"\``, `DamageType string \`json:"damage_type"\``; constants `DamagePhys = "phys"`, `DamageMagic = "magic"`.
- `func (k Kind) Magic() bool` — `DamageType == DamageMagic`.

**Behaviour:**
- Script: every kind gets `hp_pct: 100`, `attack_pct: 100`, `damage_type: "phys"` unless set; add the 26 kinds of §2.5 in map order (copy `wanderer`, then the overrides in the last column). The three train kinds use the exact ids, names and `speed` of the 7g spec §2.3. `levels` unchanged — assert by comparing with the committed file's `levels` before writing.
- Script prints Quỷ Huyết Nha at level 88: base HP/attack/EXP and the scaled ones (`ceil(v × pct / 100)`).
- `Load` rejects `hp_pct` or `attack_pct` outside 50–200 and any `damage_type` other than the two constants, naming the kind.

**TODO / tests** (`// ____TEST GENERATED DATA____`, `// ____TEST KIND TRAITS____`):
- [ ] `--write-json`; `--print-json` matches the committed file (existing test).
- [ ] All 26 ids present with the §2.5 names; `blood_fang` 80/120; `scale_serpent` magic, `detect_range` 4; `shadow_demon` speeds 0.96/1.44; `train_flesh` speed 0.6; old kinds 100/100/phys.
- [ ] Reject table (mutate a valid JSON): `hp_pct` 49, `hp_pct` 201, `attack_pct` 0, `damage_type` "fire".
- [ ] Commit: `Add the 26 normal monster kinds of levels 21-90 and the hp, attack and damage type kind fields`.

### Task 2: Server — apply the kind fields in the zone

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, `internal/zone/spawn.go`, `internal/zone/monster.go`, `internal/zone/progression.go`, `internal/zone/combat.go`, zone tests.

**Interface:**
- `func ScalePct(v, pct int) int` (combat) — `ceil(v × pct / 100)` in integers; `pct == 100` returns `v`.
- `type monsterStats struct { HP, Attack, Defense int; Exp int64 }` (or the existing level struct's type) and `func (z *Zone) statsOf(kind *gamedata.Kind, level int) (monsterStats, bool)` — the level row (rank block when 7d has landed) with `HP` and `Exp` scaled by `HPPct` and `Attack` by `AttackPct`; `Defense` unchanged. Every read listed in the verified findings goes through it.
- `func (z *Zone) monsterHitDefense(m *monster, p *player) int` — magic defense when `m`'s kind is magic, else physical; used by melee and ranged (and by 7d skill code only where a skill has no own `damage`).

**Behaviour (§3.2):** spawn sets `hp`/`maxHP` from `statsOf`; melee and ranged damage use `statsOf(...).Attack` and `monsterHitDefense`; `payExpShare` uses the scaled EXP with the scaled `maxHP` (ratio stays r(L)). Kinds with 100/100/phys behave exactly as before.

**TODO / tests** (`// ____TEST KIND TRAITS____` in the zone test file; combat in its file):
- [ ] `ScalePct(1000, 80) == 800`, `ScalePct(1001, 80) == 801`, `ScalePct(7, 100) == 7`, `ScalePct(10, 120) == 12`.
- [ ] A region of a test kind with `hp_pct` 80: the spawned monster's `maxHP` and `EntityEnter.max_hp` are `ScalePct(level HP, 80)`; one player killing it alone gets exactly `ScalePct(level EXP, 80)`.
- [ ] `attack_pct` 120: a melee hit on a player equals `combat.Damage(ScalePct(attack, 120), PhysDefense, level)`; a ranged shot uses the same scaled attack in `MonsterRangedDamage`.
- [ ] `damage_type` magic vs a player with physical defense 100 and magic defense 500: melee and ranged damage use 500; a phys kind of the same level uses 100.
- [ ] Existing zone tests stay unchanged and green (defaults are neutral).
- [ ] Commit: `Scale monster HP, EXP and attack by kind and let magic monsters hit magic defense`.

### Task 3: Server — the fourteen maps

**Files:** `tools/mapgen.py`, `maps/*.json` (generated: 14 new, `fujikasane_1` and every map with a Xa phu changed), `internal/mapdata/mapdata_test.go`, `internal/world/world_test.go`, the real-map tests of 4b/4c/6b that count maps or NPCs (find by name, e.g. `grep -rn "fujikasane_1" internal/*/…_test.go`).

**Behaviour:**
- Declare the maps of §2.1 with the regions of §2.2 through the existing training-map helper; each map gets a few themed wall rects of its own (keep every region at ≥ its monster count of region tiles; the script already checks).
- `yoshiwara_3`: a second layout helper (`strip_regions`) splitting the area east of the camp (columns 12–62) into five equal vertical strips, full inner height, nests at strip centres, levels 86…90 west to east, 6 monsters each; no east portal, no `gate_east`.
- `mugen_1`, `mugen_2`: horizontal "car" partition walls with gaps ≥ 3 tiles.
- `fujikasane_1`: add the east portal `to_fujikasane_2`, arrival `from_fujikasane_2`, `gate_east`; nothing else moves. If 7d landed, re-check that its boss area is still ≥ 2 tiles from the new portal and clear of `gate_east` + 1 (move the area inside its region if not, and say so in the commit).
- Camp NPCs `duoc_su`, `xa_phu`, `thu_kho` by the 4b/4c placement rules; no `quest` NPC.
- Xa phu routes generated, not listed: for each `carriage` NPC, every other map in {`ubuyashiki`} ∪ training maps, `ubuyashiki` first then by lowest region level; price per §2.4. Maps of type `battle`/`instance` or `event_only` are skipped.
- New self-checks in the script: every training map has exactly `duoc_su`, `xa_phu`, `thu_kho`; each Xa phu's route set is as above; the region level ranges of the 14 maps match §2.1/§2.2; `min_level` is absent on training maps.

**TODO / tests** (`// ____TEST REAL MAPS____`):
- [ ] `python3 tools/mapgen.py --write`; `--check` clean; `quest_data.py --check` still clean (event_data too if 7e landed).
- [ ] `mapdata`: `LoadDir("../../maps")` loads the 19 world maps (plus any event maps already there); BFS over portals from `kumotori_1` reaches every non-`event_only` map; every portal has a reverse; the shortest portal path `kumotori_1` → `yoshiwara_3` has 18 hops; `yoshiwara_3` has regions `r1`…`r5` with levels 86…90 and 6 monsters each; no training map has `min_level` > 1.
- [ ] `gamedata.Validate` with the real maps passes.
- [ ] `world`: `Start` with the real maps opens 2 zones per training map and 1 for `ubuyashiki`.
- [ ] Generalise the 4b/4c/6b real-map tests that count "five maps" or list NPCs per map: iterate over the loaded training maps instead of fixed names where the assertion is "every training map has …"; keep the fixed-name assertions that test 1–20 content (quest NPCs).
- [ ] Commit: `Generate the fourteen training maps for levels 21-90 and route every Xa phu to every map`.

### Task 4: Server — end-to-end test

**Files:** `e2e/e2e_test.go`.

**Behaviour (§7):** `TestWorldExpansionOverWebSocket`: store-level setup (`DevSetLevel` 25, `DevSetPlace` inside `gate_east` of `fujikasane_1`, 0.5 tiles from the east portal); enter; `UsePortalReq{to_fujikasane_2}` → `EnterWorldResp` on `fujikasane_2` at `from_fujikasane_1`; an `EntityEnter` of a monster with level 21–25 arrives within 3 s. Leave; give 5,000 Xu (the store helper `devitem` uses) and place next to `xa_phu` of `fujikasane_2`; enter; `TravelReq` with the route index of `yoshiwara_3` (read from the map file) → `EnterWorldResp` on `yoshiwara_3` at its `default`, Xu 3,280; an `EntityEnter` of a level 86–90 monster arrives.

**TODO:**
- [ ] Test passes with `-race` against the test DB.
- [ ] Commit: `Add an end-to-end test that walks into Fujikasane's deep forest and rides to the Yoshiwara tunnels`.

### Task 5: Server — documentation

**Files:** `docs/game-design.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md`.

**TODO:**
- [ ] game-design per §8: 5.5 (map ids column, the two split rows, the three renamed kinds), 5.11 (Hầm Ngầm five regions × 6, the three kind fields, Trảo Dài range equals the standard), 4.1 (Xa phu reaches every map), 4.2 (main line stops at 20, milestones 30/40/50).
- [ ] 4a spec §1.3 "Lượng nội dung" row: note that 21–90 is done in 4d.
- [ ] README: map list, Xa phu routes.
- [ ] Commit: `Document the phase 4d maps and monsters`.

### Task 6: Client — sync, scrolling lists, real-map tests

**Files:** `server` submodule, `data/maps/` (+ `data/monsters.json` if the sync script copies it), `ui/` NPC window and Bản đồ window (names from 4b/6a), `tests/unit/test_core.gd`, `README.md`.

**Behaviour:**
- Bump the submodule to the server HEAD after Task 5; `tools/sync_server.sh`; `sync --check` clean.
- The Xa phu route list (up to 18 rows) and the Bản đồ map list (19 maps with their NPCs) sit in a `ScrollContainer` with horizontal scroll disabled (as `ui/bag_tab.gd`); skip if they already scroll.

**TODO / tests** (`# ____TEST WORLD GRAPH____`):
- [ ] `world_graph.reachable("kumotori_1")` has all 19 world maps; `route("kumotori_1", "yoshiwara_3")` has 18 portals, first `to_ubuyashiki`, last `to_yoshiwara_3`; `level_range("yoshiwara_3") == Vector2i(86, 90)`, `level_range("fujikasane_2") == Vector2i(21, 25)`. Update the 6a test that expected five maps.
- [ ] The existing real-map A* / walker tests pass over all maps (they iterate the synced maps); check the run time stays reasonable and note it in the report.
- [ ] Manual check against a :8299 server: open the Xa phu window in a camp and scroll to `yoshiwara_3`; open Bản đồ and scroll to the last map. Screenshots in the report.
- [ ] README: the new maps.
- [ ] Commit: `Sync the phase 4d maps and let long Xa phu and map lists scroll`.

---

## Acceptance (controller, after Task 6)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet, `mapgen.py --check`, `monster_stats.py --check`, `quest_data.py --check`; client `tests/run.sh`; client smoke still OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID). With nobody online, watch its CPU for a minute (`ps -o %cpu -p <pid>`); record the figure; above 5 % of one core is a follow-up, not a blocker.
- [ ] Tell the user how to try it (§7 acceptance): `devchar -level 22 -map fujikasane_1 -x 59.5 -y 24.0` for `test1`, walk east into Fujikasane – Vùng sâu; open Xa phu, ride to Hầm Ngầm after `devchar -level 86`; see the five kinds, the fast Quỷ Tàng Ảnh and the magic Quỷ Lân Xà.

## §9 notes when 7d / 7e / 7g landed first

- **7d:** in Task 3 relax 7d's mapgen rule "each training map has exactly one boss spot" to "each 1–20 map has exactly one, 21–90 maps none" and write `bosses: []` on the new maps; in Task 2 the helper picks `ForRank(kind.Rank)` first, then scales; 7d skills keep their own `damage` (the kind's `damage_type` applies only to melee and shots).
- **7e:** `event_data.py --check` and `gamedata.Validate` must still pass (wanted group size vs the smallest region of every training map); Lệnh Diệt Quỷ then runs on the new maps automatically.
- **7g (later):** its Task 1 must not add `train_tentacle`, `train_flesh`, `train_nightmare` again; it only adds `enmu`.
