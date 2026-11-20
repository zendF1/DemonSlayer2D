# Phase 6a Click-to-Move, A* Pathfinding and Cross-Map Travel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tap the ground and the character walks there around walls. Tap a far NPC and it walks around walls to it and opens its window. A Bản đồ window shows a minimap of the current map (tap to walk) and every reachable map with its NPCs and an Đi tới button; picking an NPC on another map walks through the portals to it. Holding Đánh on a target behind a wall runs around the wall. The server does not change.

**Architecture:**
- Client only. `core/pathfinder.gd`: 8-way A* on the collision tiles without corner cutting, string pulling with a 0.35 box sweep, `clear` and `nearest_walkable`. `core/walker.gd`: follows a path one predictor tick at a time, keeps the sent direction within 3°, a partial last step, stuck detection and re-planning, and `chase` for the combat/pickup/NPC loops.
- `core/world_graph.gd` reads the synced map files lazily and answers BFS portal routes; `core/travel.gd` turns a goal (map, pos, radius, kind) into legs: a portal centre on the way, the goal on the last map. `Router.travel` keeps the goal across the World reload of a portal transfer; `portal_watch` (4a) still sends `UsePortalReq`.
- `screens/world.gd` asks `travel.step` inside the predictor tick loop, after manual input and the loops; taps on the ground start a point goal; the loops' walk direction goes through `walker.chase`.
- UI: `ui/path_view.gd` (goal ring and dots), a Bản đồ button and `ui/map_window.gd`.
- Server: documentation only; protocol stays 13.

**Tech stack:** Godot 4.7.2, GDScript; server docs in Markdown.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md](../specs/2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 5b plan](2026-11-20-giai-doan-5b-to-doi-exp-chia-se.md) is fully implemented on both repos (protocol 13; client `core/portal_watch.gd`, `core/npc_loop.gd`, `core/pickup_loop.gd`, `Router.enter_world` reloads, `map_grid` portals and NPCs). Start from client and server HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (none are added here; run `make test` once before the docs commit to confirm nothing broke); client `tests/run.sh` (headless, with watchdog); 1–2 test files per folder with `# ______________________TEST X_____________________` sections; a fix comes with a test that fails on the old code; `core/` stays Node-free.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 57cc02e (phase 3a in progress) plus the phase 3–5b specs; client HEAD 5d275f7)

Phases 3–5b land before this plan and move or add most client symbols below. Find each by name before editing.

**Movement and prediction**
- `core/movement.gd` mirrors the server's `Step` (`internal/zone/move.go:43`): `HALF_SIZE 0.3`, sub-steps ≤ 0.25, X then Y resolution, so a box slides along walls. `clamp_dir` only shortens vectors longer than 1, and the server's `ClampDir` (`internal/zone/zone.go:274`) does the same, so a `MoveInput` shorter than 1 moves a partial step on both sides — the walker's last step relies on this.
- `core/predictor.gd` `tick(want)` sends only when `want != sent_dir` (and at most every `SEND_GAP_SEC` 0.05 s) or to resend an unacked input; returning the exact previous `Vector2` therefore sends nothing. Defaults `TICK_RATE=10`, `MOVE_SPEED_TILES=4` → 0.4 tiles per tick. Gateway `movesPerSec = 20`, `packetsPerSec = 30` (`internal/gateway/server.go:32`): one move per tick while walking is within limits even with attacks and casts.
- `screens/world.gd` `_process` builds one `want` per frame and runs up to `MAX_TICKS_PER_FRAME` predictor ticks with it; travel must be queried per tick (inside that loop) with `_predictor.pos`.

**Map data on the client**
- `core/map_grid.gd` exposes `blocked(x, y)` (outside the map is blocked), `width`, `height`, `in_safe_zone`; 4a adds `portals` (`{id, rect: Rect2i, to_map, to_spawn}`), `portal_at`, static `name_of(map_id)` with a cache; 4b adds `npcs` (`{id, name, role, shop, routes, pos}`), `npc`, `npc_at`. `parse` ignores unknown keys; add `spawn_points` and `regions` parsing if 4a–4b did not (`regions[].rects`, `nest`, `monster`, `min_level`, `max_level`; see `maps/kumotori_1.json`).
- Map JSON is generated by the server's `tools/mapgen.py`; the client copy under `data/maps/` comes only from `tools/sync_server.sh`.

**Input**
- `screens/world.gd` `_pick(screen_pos)` turns a tap into tiles (`canvas_transform.affine_inverse() * screen_pos / MapView.TILE_PX`) and today only selects monsters ("the ground does nothing"); 3a/4b put ground items and NPCs ahead of monsters. A touch also arrives as an emulated mouse click: keep the `DEVICE_ID_EMULATION` filter.
- `ui/hud.gd` has the place label (`set_place`, tappable since 4a for the zone window) and `show_notice`.

## File structure

Client (`DemonSlayer2DGame`):

```
core/pathfinder.gd (new)                                   A*, string pulling, clear (Task 1)
core/walker.gd (new)                                       path following, chase (Task 2)
core/world_graph.gd (new), core/travel.gd (new)            portal routes, legs (Task 3)
core/map_grid.gd                                           spawn_points, regions if missing (Task 3)
autoload/router.gd                                         travel goal (Task 3)
core/npc_loop.gd, screens/world.gd                         wiring (Task 4)
ui/path_view.gd (new), ui/map_window.gd (new), ui/hud.gd   UI (Task 5)
tests/unit/test_core.gd                                    tests (Tasks 1–3)
tests/smoke/smoke.gd, README.md, CLAUDE.md                 smoke, docs (Task 6)
```

Server (`DemonSlayer2D`):

```
docs/game-design.md, CLAUDE.md, phase 0 spec               docs (Task 7)
```

---

### Task 1: Client — A* pathfinder

**Files:** `core/pathfinder.gd` (new), `tests/unit/test_core.gd`.

**Interface (static):**
- `func find(grid, from: Vector2, to: Vector2) -> PackedVector2Array` — §4.1; empty when no path or when already a straight `clear` line inside the same tile.
- `func clear(grid, a: Vector2, b: Vector2, half := 0.35) -> bool` — samples every 0.1 tile, both ends included; each sample's square must touch no blocked tile.
- `func nearest_walkable(grid, pos: Vector2, max_r: int) -> Variant` — tile centre or `null`.
- `func length(from: Vector2, path: PackedVector2Array) -> float`.
- `const MAX_EXPANDED := 4096`, `const SWEEP_HALF := 0.35`.

**Behaviour:**
- Neighbours: 8 directions; a diagonal step needs both orthogonal neighbours walkable; costs 1 and `sqrt(2)`; octile heuristic; binary-heap open set; ties by lower `h`, then lower tile index; closed set as a `PackedByteArray`.
- String pulling: from the current anchor, take the farthest later point `p_j` with `clear(anchor, p_j)`; the first anchor is `from`, the last point is `to` (or the replacement tile centre). Any raw step between adjacent tile centres passes `clear` (both tiles and, for diagonals, both orthogonals are free), so pulling never fails.
- Gotcha: a tile-sized square at half 0.35 around a tile centre stays inside that tile; do not "optimise" `clear` into a centre-line test, which lets the 0.6 box clip corners.

**TODO / tests** (`# ____TEST PATHFINDING____`):
- [ ] Small hand-built grids (a test grid object with `blocked`, `width`, `height`): open floor → single point; wall between → path around it; U-shape exit; no corner cut through two diagonal walls; unreachable → empty; wall goal → nearest walkable within 3, none → empty; every consecutive pair of `[from] + path` passes `clear`; length ≤ 1.01 × the octile shortest path on an open grid with one obstacle; same input → identical output.
- [ ] Real synced maps: from each map's `default` to every NPC, portal centre and spawn point, `find` returns a non-empty path (or a straight clear line) and expands ≤ `MAX_EXPANDED` tiles (expose a counter for tests).
- [ ] Commit: `Add an A* pathfinder over the collision tiles`.

### Task 2: Client — walker

**Files:** `core/walker.gd` (new), `tests/unit/test_core.gd`.

**Interface:**
- `func start(grid, from: Vector2, goal: Vector2, radius: float) -> bool`.
- `func step(pos: Vector2, step_dist: float, now: float) -> Vector2` — direction for one predictor tick (§1.3, §4.1).
- `func chase(grid, pos: Vector2, target: Vector2, now: float) -> Vector2`.
- `var status: String` (`"idle"`, `"walking"`, `"arrived"`, `"failed"`); `func remaining() -> PackedVector2Array`; `func cancel() -> void`.
- Constants: `KEEP_DEG := 3.0`, `STUCK_SEC := 1.0`, `STUCK_GAIN := 0.05`, `MAX_REPLANS := 3`, `CHASE_MOVE := 1.0`, `CHASE_REPLAN_SEC := 0.5`, `ARRIVE_EPS := 0.01`.

**Behaviour:**
- Advance to the next point when within `step_dist` of it; when the remaining one is the goal and `radius` is 0 and it is within `step_dist`, return `(goal − pos) / step_dist`.
- Return the previous direction object unchanged while the new one is within `KEEP_DEG` and the point did not change (so `predictor` sends nothing).
- Stuck: track the best distance to the current point; if it has not improved by `STUCK_GAIN` for `STUCK_SEC`, re-plan from `pos`; a successful arrival at a point resets the count; the third consecutive re-plan → `"failed"`, `Vector2.ZERO`.
- `chase` keeps its own small state (last goal, last plan time) separate from `start`'s path.

**TODO / tests** (`# ____TEST PATHFINDING____`):
- [ ] Simulation helper in the test: loop `pos = movement.step(grid, pos, walker.step(pos, 0.4, t), 0.4)`, `t += 0.1`.
- [ ] Real maps: from `default` to every portal centre (radius 0) and every NPC (radius 2.5) → `"arrived"` within `ceil(1.5 × length / 0.4) + 20` ticks; direction changes ≤ path points + 2; radius 0 ends within 0.01 of the goal.
- [ ] A turn within 2° returns the identical previous vector; 4° returns a new one.
- [ ] Stuck: after `start`, block the tile in front of the character in the test grid; the walker re-plans; with the goal sealed off it reports `"failed"` after the third re-plan.
- [ ] `chase`: no wall → straight direction to the target; wall between → direction along the A* path; target moved 2 tiles → re-planned only after 0.5 s.
- [ ] Commit: `Follow paths one predictor tick at a time and chase around walls`.

### Task 3: Client — world graph, travel and Router

**Files:** `core/world_graph.gd` (new), `core/travel.gd` (new), `core/map_grid.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:**
- `world_graph.gd`: `func load(map_id) -> bool`, `portals(map_id) -> Array`, `spawn(map_id, spawn_id) -> Variant`, `npcs(map_id) -> Array`, `name_of(map_id) -> String`, `level_range(map_id) -> Vector2i`, `route(from_map, to_map) -> Variant`, `reachable(from_map) -> Array[String]`. Map files read through `map_grid` parsing (one `map_grid` instance per id in a cache).
- `travel.gd`: `func begin(goal: Dictionary, map_id: String, pos: Vector2, graph, grid) -> String` (`""`, `"no_route"`, `"no_path"`); `func resume(map_id, pos, graph, grid) -> String`; `func step(pos, step_dist, now) -> Vector2`; `func arrived() -> bool`; `func leg() -> Dictionary` (`{pos, radius, portal_id}` for drawing); `func cancel()`; `var goal: Dictionary`.
- `router.gd`: `var travel = null` — cleared in the same place as the party model when going back to character select.

**Behaviour:** §4.1–4.2. A portal leg targets the rect centre (`rect.position + rect.size / 2.0`) with radius 0; a goal on the current map targets `goal.pos` with `goal.radius`. `begin` on an unknown goal map or a `null` route → `"no_route"`.

**TODO / tests** (`# ____TEST TRAVEL____`):
- [ ] `route("kumotori_1", "fujikasane_1")` → `to_ubuyashiki`, `to_sagiri_1`, `to_sagiri_2`, `to_fujikasane_1`; same map → `[]`; unknown map → `null`; `reachable` lists all five maps in BFS order; `level_range("sagiri_1") == Vector2i(6, 10)`, `ubuyashiki` → `(0, 0)`.
- [ ] Travel to an NPC on `sagiri_1` from Kumotori: first leg = centre of `to_ubuyashiki`; `resume` on `ubuyashiki` at `from_kumotori_1` → centre of `to_sagiri_1`; `resume` on `sagiri_1` at `from_ubuyashiki` → NPC with radius 2.5; `arrived()` false until the last leg arrives.
- [ ] A goal copied through a plain `Dictionary` (as `Router.travel` stores it) resumes identically.
- [ ] Commit: `Route travel goals across maps through the portal graph`.

### Task 4: Client — world wiring

**Files:** `screens/world.gd`, `core/npc_loop.gd`.

**Behaviour (§4.3):**
- Build one `walker` (for `chase`), one `travel`, one `world_graph` per World screen; on `_ready`, if `Router.travel` is set, `travel.resume` (failure → notice, clear it).
- Inside the predictor tick loop: `want` = manual direction, else the loops' direction (replaced by `walker.chase(grid, pos, target_pos, now)` whenever non-zero), else `travel.step(_predictor.pos, step_dist, now)`, else zero; dead or leaving → zero.
- `npc_loop.step` takes its walk direction from `walker.chase` (pass the walker in or return the target and let `world.gd` chase; keep `core/` Node-free).
- `_pick`: NPC → ground item → monster/player → ground (`travel.begin` point goal; `"no_path"` → notice "Không tìm được đường"). Cancel travel on manual input, the Đánh press edge, death, `_leave`.
- `travel.arrived()`: `kind == "npc"` → open the NPC window exactly as `npc_loop`'s "open"; clear `Router.travel`. Start of a cross-map goal sets `Router.travel` and shows "Đang đi tới <tên map>".
- A failed `UsePortalReq` (4a path) also cancels travel and clears `Router.travel`.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server: tap behind the Kumotori camp wall → walks around; tap an NPC far away → walks and opens it; hold Đánh on a monster behind a wall → runs around; joystick mid-route → stops.
- [ ] Commit: `Walk to tapped points and around walls to targets and NPCs`.

### Task 5: Client — path view and map window

**Files:** `ui/path_view.gd` (new), `ui/map_window.gd` (new), `ui/hud.gd`, `screens/world.gd`.

**Behaviour (§4.4):**
- `path_view.gd`: `func show_path(leg_goal: Vector2, points: PackedVector2Array)`, `func clear()`; draws in world pixels (`MapView.TILE_PX`); added above the map, below entities.
- HUD: a small round "Bản đồ" button beside the place label; signal `map_pressed`.
- `map_window.gd`: `func open(grid, graph, map_id, self_pos)`; signal `go(goal: Dictionary)`; the minimap is drawn in `_draw` from `grid.blocked`, `safe_zones`, `portals`, `npcs`; a tap converts to tiles and emits a point goal (wall → `nearest_walkable`, none → notice); the list per §4.4 with role names; closes on `go`.
- `world.gd` connects `go` to `travel.begin`.

**TODO:**
- [ ] Manual check at portrait size: the minimap fits the width; Đi tới Thợ rèn from Kumotori reaches Ubuyashiki and opens the NPC window; screenshots in the report.
- [ ] Commit: `Add the map window, the path markers and Đi tới`.

### Task 6: Client — smoke and docs

**Files:** `tests/smoke/smoke.gd`, `README.md`, `CLAUDE.md`.

**Behaviour (§4.5):** after the existing steps, `devchar -map kumotori_1 -x 5.5 -y 5.5` and re-enter (reuse the smoke's existing re-enter helper), travel to `tap_hoa` on `ubuyashiki` with `core/travel.gd` + `predictor`, send `MoveInput`s, expect an automatic `UsePortalReq`, `EnterWorldResp` on `ubuyashiki`, `resume`, arrival within 2.5 tiles in 60 s. Print `SMOKE OK (… travel kumotori_1 → ubuyashiki)`. Keep the watchdog.

**TODO:**
- [ ] Run against a :8299 server; paste the OK line in the task report.
- [ ] README: tap the ground, the map window, Đi tới. CLAUDE.md: the pathfinding/walker/travel layout rule (§6).
- [ ] Commit: `Extend the smoke test with cross-map travel and document click-to-move`.

### Task 7: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 4.3 and 2.5 per §6; phase 0 roadmap note (phase 6 split into 6a–6d with links).
- [ ] CLAUDE.md invariant: pathfinding and click-to-move are client-side; the server accepts only `MoveInput` directions.
- [ ] `make test` green (no code change); commit: `Document the phase 6a click-to-move and pathfinding`.

---

## Acceptance (controller, after Task 7)

- [ ] Client `tests/run.sh` green; smoke OK; server `make test` green.
- [ ] Restart the :8199 server from HEAD only if its binary is older than phase 5b (kill only its recorded PID).
- [ ] Tell the user what to try (spec §5 "Nghiệm thu").
