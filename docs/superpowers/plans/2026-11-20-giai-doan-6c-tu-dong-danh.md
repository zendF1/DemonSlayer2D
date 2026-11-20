# Phase 6c Auto-Combat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Auto button on the HUD. Turned on at a hunting ground, the character fights by itself within 10 tiles of where it started: nearest monster in the ground, skills by fixed priority (bar slots 5 → 2, then the slash), HP/MP potions under configurable thresholds, own drops picked up by a filter, back to the centre when it strays. A player who hits it is counter-attacked until they stop being a valid target, run away or stop hitting for 10 s. Death turns auto off; after a respawn a "Tiếp tục auto" button walks back (6a travel) and restarts it. Holding the button opens the settings, saved per character on the device. The server does not change.

**Architecture:**
- Client only. `core/auto_settings.gd` (model, clamping, the pickup filter) and `core/auto_play.gd` (one decision per frame from a plain `ctx` dictionary; never sends anything) are Node-free and unit-tested.
- `screens/world.gd` builds `ctx` from the existing models and feeds the decision into the existing loops: `targeting.select`, `cast_loop`/`attack_loop` with a virtual slot and hold, the HUD potion path, `pickup_loop.start`, `walker.chase` towards home. Stop rules live in `world.gd` wiring only as calls to `auto.stop(reason)`.
- `Router.auto_anchor` and `Router.auto_resume` survive the World reloads of a respawn or a portal; `autoload/config.gd` persists settings in `user://auto.cfg`.
- UI: the Auto button (tap/hold), a status line, the ground ring on `path_view`, `ui/auto_window.gd`, the "Tiếp tục auto" button.
- Server: documentation only; protocol stays 14.

**Tech stack:** Godot 4.7.2, GDScript; server docs in Markdown.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-6c-tu-dong-danh-design.md](../specs/2026-11-20-giai-doan-6c-tu-dong-danh-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 6b plan](2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang.md) is fully implemented (protocol 14; client `core/travel.gd`, `core/walker.gd`, `core/potion_rules.gd`, `core/pickup_loop.gd`, `core/pk_rules.gd`, `world_entities.candidates(ctx)` with players). Start from HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** client `tests/run.sh` (headless, with watchdog); 1–2 test files per folder with `# ______________________TEST X_____________________` sections; a fix comes with a test that fails on the old code; `core/` stays Node-free. Run the server's `make test` once before the docs commit.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 8da4fd2 plus the phase 3–6b specs; client HEAD 5d275f7)

Phases 3–6b land before this plan and move most client symbols below. Find each by name before editing.

- `screens/world.gd` `_process` already separates "what to do" from "how to walk": `targeting.press/update`, then `cast_loop.step(now, me, target_pos, holding, manual, target_id, mp, facing, in_safe)` for bar slots 2–5 (it hands the slash back while the skill cools down or MP is short), then `attack_loop.step(...)`. Auto only needs to supply `holding`, the slot and the target; keep using these loops so all server-reply handling (`OUT_OF_RANGE` chase, `ON_COOLDOWN` retry, `INVALID_TARGET` skip) stays in one place.
- `core/targeting.gd`: `select(id)`, `skipped(id)`, `on_invalid`, `on_death` (switches to the nearest candidate — auto then overrides with its own choice next frame, which is fine), `update` drops targets beyond `KEEP` 8 tiles; 5a adds `on_hurt_by` (selects the attacker only when there is no target) — auto's counter-attack replaces it while auto is on.
- 5a `world_entities.candidates(ctx)` returns monsters and attackable players as `{id, pos, kind}`; split by `kind` for `ctx.monsters`/`ctx.players`. 5b removes teammates via `pk_rules.can_attack(..., same_party)`.
- 4b `core/potion_rules.gd`: `best_slot`, `block(group, level, hp, max_hp, mp, max_mp, inventory, book, now)` → `""`, `"none"`, `"cooldown"`, `"full"`; the HUD potion buttons already send `UseItemReq` for `best_slot` — route auto through the same function so cooldown bookkeeping is shared.
- 3a `core/pickup_loop.gd`: `start(id)`, walks to 1.2 tiles, one `PickupReq`, `BAG_FULL` stops; `entity_table.ground_items()` lists own items; `inventory.can_take(item)`.
- 6a `core/walker.gd` `chase(grid, pos, target, now)` gives a direction around walls; `core/travel.gd` + `Router.travel` handle the cross-map resume walk.
- `skill_book` knows learned levels, MP cost and cooldown per skill; a "slot ready" callable is `learned && ready_in(now) == 0 && mp >= cost`; "needs target" is `shape in ["single", "circle_target"]`.
- `autoload/config.gd` is the place for device settings (it already holds `force_touch`).

## File structure

Client (`DemonSlayer2DGame`):

```
core/auto_settings.gd (new)                       settings model (Task 1)
core/auto_play.gd (new)                           decisions (Task 2)
tests/unit/test_core.gd                           tests (Tasks 1–2)
autoload/router.gd, autoload/config.gd            anchor, resume, persistence (Task 3)
screens/world.gd                                  wiring (Task 3)
ui/hud.gd, ui/auto_window.gd (new), ui/path_view.gd   UI (Task 4)
tests/smoke/smoke.gd, README.md, CLAUDE.md        smoke, docs (Task 5)
```

Server (`DemonSlayer2D`):

```
docs/game-design.md, CLAUDE.md                    docs (Task 6)
```

---

### Task 1: Client — auto settings

**Files:** `core/auto_settings.gd` (new), `tests/unit/test_core.gd`.

**Interface (§3.1):** `var slots: Array[bool]`, `hp_pct`, `mp_pct`, `pick_equipment`, `pick_stones`; `func to_dict() -> Dictionary`; `func from_dict(d: Dictionary) -> void`; `func wants(item: Dictionary, book) -> bool` (equipment by quality index 0–4; `stone_enhance`/`stone_refine` by `pick_stones`; other stackables always).

**TODO / tests** (`# ____TEST AUTO____`):
- [ ] Defaults; clamping 5 → 10, 95 → 90, 47 → 50; unknown `pick_equipment` → `"blue"`; a non-array `slots` → all true.
- [ ] `wants`: White piece under `"blue"` false, Blue true; `"gold"` needs quality ≥ 2; `"none"` false for any piece, `"all"` true; stones follow `pick_stones`; a potion stack is always wanted.
- [ ] Commit: `Add the auto-combat settings model`.

### Task 2: Client — auto decisions

**Files:** `core/auto_play.gd` (new), `tests/unit/test_core.gd`.

**Interface (§3.1):** constants; `func start(map_id, zone, pos, map_type, in_safe, alive) -> String`; `func stop(reason: String)`; `var active`, `var anchor`, `var last_stop`; `func on_hit_by_player(attacker_id: int, now: float)`; `func step(ctx: Dictionary) -> Dictionary` returning `{target, slot, hold, potion, pickup, home, notice}`.

**Behaviour:** the decision order of §3.1 and every rule of §1.2. Keep per-attacker last-hit times in a dictionary pruned after `PVP_FORGET_SEC`. The "bag full" notice fires once per `start`. `slot` is 1 when no enabled slot is ready.

**TODO / tests** (`# ____TEST AUTO____`): every bullet of spec §4 for `start`, monster choice, keeping, home, never targeting players unprovoked, counter-attack (single, two attackers, 9.9 s vs 10 s, out of ground + 5), slot priority, potions, pickup.
- [ ] Commit: `Decide auto-combat targets, skills, potions and pickups`.

### Task 3: Client — router, persistence and world wiring

**Files:** `autoload/router.gd`, `autoload/config.gd`, `screens/world.gd`.

**Interface:** `Router.auto_anchor`, `Router.auto_resume`; `Config.load_auto(char_name) -> Dictionary`, `Config.save_auto(char_name, d: Dictionary)` (`ConfigFile`, `user://auto.cfg`, one section per character; read errors → `{}`).

**Behaviour (§3.3):**
- Build `ctx` once per frame while `auto.active`; apply `target` (`targeting.select` when different), use `slot`/`hold` in place of the bar slot and the Đánh button for `cast_loop`/`attack_loop` (ignore the real Đánh button and bar selection meanwhile), send `potion` through the HUD potion path, start `pickup_loop` for `pickup` when idle, walk `home` through `walker.chase`, show `notice`.
- A `Hit` on self by a player → `auto.on_hit_by_player`.
- Stops: manual direction, taps on ground/NPC/item, starting any travel other than the resume walk, death, any zone-change request outside the resume walk; taps on a monster/player only select it.
- Resume: show "Tiếp tục auto" after a respawn or a World reload while `Router.auto_anchor` is set and auto is off; pressing it sets `auto_resume` and starts the travel goal; arrival (also after a portal reload) → `auto.start` with the stored anchor (current khu), clear `auto_resume`.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server: auto kills, uses skills by priority, drinks potions, picks Blue items and stones, returns to the centre, stops on joystick.
- [ ] Commit: `Wire auto-combat into the world screen`.

### Task 4: Client — auto UI

**Files:** `ui/hud.gd`, `ui/auto_window.gd` (new), `ui/path_view.gd`, `screens/world.gd`.

**Behaviour (§3.4):** the Auto button (tap toggles, 0.5 s hold opens settings; refusal notices), the blinking "Auto" status line, the 10-tile ground ring, the settings window (four skill switches with names from `skill_book`, two sliders, the equipment choice, the stones switch; saves on change through `Config.save_auto`), the "Tiếp tục auto" button and the notices.

**TODO:**
- [ ] Manual check at portrait size; screenshots of the settings window and the ring in the report.
- [ ] Commit: `Add the auto button, its settings window and the resume button`.

### Task 5: Client — smoke and docs

**Files:** `tests/smoke/smoke.gd`, `README.md`, `CLAUDE.md`.

**Behaviour (§3.6):** `devchar -map kumotori_1 -x 14.5 -y 6.5` (re-check the tile in the synced map: walkable, outside every safe zone, inside the level 1–2 `wanderer` region; otherwise the nearest such tile), re-enter, run `auto_play` + `attack_loop` (slash only) + `predictor` until two monsters it attacked die, within 90 s. Print `SMOKE OK (… auto kills 2)`.

**TODO:**
- [ ] Run against a :8299 server; paste the OK line in the task report.
- [ ] README: the Auto button, settings, Tiếp tục auto. CLAUDE.md: auto decisions in `core/auto_play.gd`, settings in `core/auto_settings.gd`.
- [ ] Commit: `Extend the smoke test with auto-combat and document it`.

### Task 6: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`.

**TODO:**
- [ ] game-design 8 and 10.5 per §6; CLAUDE.md invariant: auto runs on the client and sends only ordinary requests; the server has no auto state.
- [ ] `make test` green; commit: `Document the phase 6c auto-combat`.

---

## Acceptance (controller, after Task 6)

- [ ] Client `tests/run.sh` green; smoke OK; server `make test` green.
- [ ] Tell the user what to try (spec §4 "Nghiệm thu"), with two windows (`tools/run2.sh`) for the counter-attack check.
