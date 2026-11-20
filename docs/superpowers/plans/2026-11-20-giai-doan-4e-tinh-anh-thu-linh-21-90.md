# Phase 4e Elites and Bosses for Levels 25–90 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The 17 elites and bosses of game-design 5.10 for levels 25–90 stand in their own areas on the phase 4d maps (16 new kinds plus 7g's `enmu`), using the 7d framework: rank stats, telegraphed and dodgeable area skills, `enrage`, 5/10 minute respawns per zone. Knockback, slow, root and split bodies are left out (damage kept). Data only: no protocol change, no migration.

**Architecture:** `monster_stats.py` adds the kinds with their `skills`/`traits` (7d schema); `tools/mapgen.py` writes their `bosses` entries, replacing 7d's "exactly one spot per training map" rule for the 21–90 maps with the §2.2 table rule and a map-wide area for Quỷ Rắn. The 7d zone code runs them unchanged; two behaviours are verified by tests (7g sleep in a training zone, several boss spots in one zone).

**Tech stack:** Go, Python 3 scripts; Godot 4.7.2 (sync only).

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-4e-tinh-anh-thu-linh-21-90-design.md](../specs/2026-11-20-giai-doan-4e-tinh-anh-thu-linh-21-90-design.md). Section references below (§n) point to it.

**Prerequisite:** phases 4d, 7d and 7g are implemented (`bosses` in map files, `skills`/`traits`/rank blocks in `monsters.json`, `SpawnSpecial`, the `enmu` kind with Sóng mộng and the sleep status). 7h may or may not have landed. Start from a server HEAD that contains 7g.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections; a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; use another free port such as :8299 for your own runs and kill only your PID. Never print DB passwords.

---

## Findings to verify first (none of 4d, 7d, 7g exists at plan time, 2026-11-20)

- 7d plan Task 2 adds the mapgen self-check "each training map has exactly one spot inside its highest-level region"; 4d (§9 of its spec) relaxes it to "1–20 maps one, 21–90 none". Find the check by name in `tools/mapgen.py` before changing it.
- 7d `gamedata` skill validation: area skills need `windup_ms` 800 and `max_targets` within the rank cap; `single` needs `windup_ms` > 0 and no `max_targets`; ids start with `m_`. 7g writes Enmu's skill as `dream_wave` with `telegraph_ms` — reuse whatever the landed code accepted; do not rename it here.
- 7g's sleep: check where the status is applied and whether anything ties it to `instance` maps (map type checks, the train run). If it is tied, Task 3 decouples it with a failing test first.
- 7d boss spots: each spot builds its own internal region with its own respawn timer; confirm two spots in one zone share nothing (timer map keyed by spot id, not by map).

## File structure

```
docs/balance/monster_stats.py, data/monsters.json (gen)     16 kinds with skills and traits (Task 1)
internal/gamedata/gamedata_test.go                          real data checks (Task 1)
tools/mapgen.py, maps/*.json (gen)                          boss spots on 12 maps, layout rule, Quỷ Rắn area (Task 2)
internal/mapdata/mapdata_test.go                            real-map spot counts (Task 2)
internal/zone/*_test.go (+ zone code only if a check fails)  several spots, sleep in a training zone, skills (Task 3)
docs/game-design.md, README.md, phase 7 index               docs (Task 4)
client: server/ submodule, data/maps/, data/monsters.json   sync (Task 5)
```

---

### Task 1: Server — sixteen elite and boss kinds

**Files:** `docs/balance/monster_stats.py`, `data/monsters.json` (generated), `internal/gamedata/gamedata_test.go`.

**Behaviour:** add the 16 kinds of §2.1 (all but `enmu`) with the 7d elite/boss behaviour fields (`attack_sec` 1.2, `respawn_sec` 300/600, `detect_range` 5, `chase_range` 10, `region_margin` 3, `chase_speed` 1.2, `speed` 0.8, `attack_range` 2.0, …), the 4d kind fields at their defaults, and the skills/traits of the table: shapes, `damage`, `pct`, `cooldown_sec`, sizes, `windup_ms` (800 for area skills, 300 for `single`), `speed` for projectiles and the dash, `max_targets` (elite 3, boss 5). `asakusa_demon` and `spider_father` have no skills and one `enrage` trait (`below_pct` 30 / 50, `attack_pct` 20). No effect fields (no knockback, slow, root).

**TODO / tests** (`// ____TEST GENERATED DATA____`):
- [ ] `--write-json`; `--print-json` matches.
- [ ] Every §2.1 id has the listed rank and a skill or trait matching the table (spot-check `hand_demon` circle_self r 3 180 % phys, `kyogai` cone 120° reach 4 magic 180 %, `horn_demon` single dash speed 25, `spider_father` enrage 50/20); all load under 7d's skill validation.
- [ ] Commit: `Add the sixteen elites and bosses of levels 25-90`.

### Task 2: Server — boss spots on the 4d maps

**Files:** `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata_test.go`.

**Interface:** a `BOSS_SPOTS` table in the script: map id → list of `(spot id, kind, level, region id)`; one helper places a 12 × 10 area inside the region's rectangles (first position, scanning from the region's far corner from the camp, whose tiles are all region tiles and that stays ≥ 2 tiles from every portal), spawn at the walkable tile nearest the area centre. A second helper for `serpent_demon`: one rectangle columns 12–62, rows 1–46 of `yoshiwara_3`.

**Behaviour (§2.2):** the 1–20 maps keep their 7d spot; the 21–90 maps get exactly the table's spots; self-checks: at most one spot per region, spots pairwise disjoint, every spot inside its stated region, `mugen_1` and `yoshiwara_1` empty, the 7d distance and safe-zone rules.

**TODO / tests** (`// ____TEST REAL MAPS____`):
- [ ] `mapgen.py --write`; `--check` clean.
- [ ] Real maps: spot count per map as §5 (`natagumo_3` 3; `asakusa_2`, `tsuzumi_2`, `natagumo_2` 2; `mugen_1`, `yoshiwara_1` 0; the other 21–90 maps 1); `yoshiwara_3`'s spot tiles include tiles of all five regions; `gamedata.Validate` with the real maps passes.
- [ ] Commit: `Place the level 25-90 elites and bosses on the training maps`.

### Task 3: Server — behaviour checks

**Files:** zone tests (`internal/zone/*_test.go`, the 7d elite section); zone code only if a check fails.

**TODO / tests** (`// ____TEST ELITES____`, fake ticks, small test maps using the real kinds from `monsters.json`):
- [ ] Two spots (`spider_brother`, `spider_sister`) in one zone: killing the first leaves the second at full HP and in place; the first respawns at exactly 600 s with a new id; the second's timer is untouched.
- [ ] `hand_demon`: a player standing within radius 3 at the impact tick takes 180 % (`MonsterSkillDamage`); one who leaves before the impact tick takes nothing.
- [ ] `spider_father`: after a hit takes it below 50 % HP, its next melee hit is `Enraged(attack, 20)`; its HP did not rise.
- [ ] `enmu` in a `training` area: a player hit by Sóng mộng is asleep for 1 s (`ASLEEP` on move and slash), awake after.
- [ ] `yahaba`, `spider_mother`: the `single` projectile hits after windup + distance / 15; the player's position and speed are unchanged.
- [ ] If sleep was tied to instance maps or spot timers were shared, fix with the failing test first and say so in the commit.
- [ ] Commit: `Test several boss spots per map, telegraphs and sleep on the level 25-90 bosses`.

### Task 4: Server — documentation

**Files:** `docs/game-design.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-7-tong-quan.md` (only if the 4d commit's edit needs a follow-up).

**TODO:**
- [ ] game-design per spec §6: 5.10 skill sizes, effects left for later, Quỷ Đầm Lầy one body; 5.11 at most one per region, Quỷ Rắn roams the whole Hầm Ngầm; section 11 lists knockback, slow, root, split bodies.
- [ ] README: elites and bosses on every map.
- [ ] Commit: `Document the phase 4e elites and bosses`.

### Task 5: Client — sync

**Files:** `server` submodule, `data/maps/`, `data/monsters.json`, `README.md`.

**TODO:**
- [ ] Bump the submodule, `tools/sync_server.sh`, `sync --check` clean; `tests/run.sh` green (`monster_book.skill_name("m_hand_slam") == "Đập tay"` added to the elites section).
- [ ] Manual check on :8299: `natagumo_3` shows three bosses with warnings; Rui's net is dodgeable. Screenshot in the report.
- [ ] Commit: `Sync the level 25-90 elites and bosses`.

---

## Acceptance (controller, after Task 5)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet, both script checks; client `tests/run.sh`.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID); tell the user what to try (spec §5 acceptance).
