# Phase 0 Godot Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Godot 4.7 client where a player registers, logs in, creates a character, enters `kumotori_1` and walks with the keyboard or a virtual joystick, and two client windows see each other move smoothly.

**Architecture:** UI is built in code, as in `../battle-squad-game`: each `.tscn` holds only a root node with a script, `_ready()` builds the children, and shapes are drawn with `_draw()`. Logic that needs no Node lives in `core/` and is unit-tested headless. The `Net` autoload speaks binary protobuf Envelopes over `WebSocketPeer`. The client predicts its own character by running fixed ticks at the server's `tick_rate` with a port of `move.go`, and it interpolates other players 2 ticks in the past.

**Tech stack:** Godot 4.7.2 and GDScript; godobuf (oniksan/godobuf commit `523a66e2388c4235ded0aa5f2fecf91cf7e7a861`, "0.7.0 for Godot 4.6") for protobuf; a git submodule of the server repo for `proto/` and `maps/`.

**Spec:** [docs/superpowers/specs/2026-11-19-giai-doan-0-client-design.md](../specs/2026-11-19-giai-doan-0-client-design.md)

**Plan format (from the user):** this plan gives interfaces, behaviour, test cases and TODOs. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task; clear commit messages; **no** `Co-Authored-By` or other signature trailer.

---

## Verified findings (2026-11-19, from a scratch run of godobuf against `protocol.proto`)

- **godobuf rejects three names in today's proto**, so Task 1 renames them. Field numbers stay the same, so the bytes on the wire do not change:
  - `Error.message` breaks the parser, because `message` is a keyword. It becomes `ErrorResp.detail`.
  - The message name `Error` shadows GDScript's global `Error` enum, which turns `get_error()` into "Cannot call function on enum value". The message becomes `ErrorResp`. The Envelope field stays `error`.
  - A field called `class` generates `get_class()`, which overrides `Object.get_class()`; Godot treats that warning as an error. It becomes `char_class` in `CharSummary`, `CreateCharReq` and `EntityEnter`.
- **Encoding matches Go byte for byte.** This was checked for `LoginReq` and `MoveInput`. Decoding a Go-encoded `WorldTick` with two `EntityMove`s, an `Error` and a `Pong` also works.
- **The generated API looks like this:**
  - Construct and encode: `Proto.Envelope.new()`, `env.set_seq(n)`, `env.new_login_req()` (returns the sub-message and selects the oneof), `env.has_world_tick()`, `env.get_world_tick()`, `msg.to_bytes()`.
  - Decode: `msg.from_bytes(bytes)` returns `Proto.PB_ERR.NO_ERRORS` (0) on success.
  - Repeated fields: `get_moves()` returns an Array.
  - Enums lose their prefix, e.g. `Proto.ErrorCode.AUTH_FAILED`, `Proto.KickReason.DUPLICATE_LOGIN`, `Proto.CharClass.VIEM`.
  - Setting one oneof field clears the others.
- **`from_bytes` is lenient.** Empty or garbage bytes can return `NO_ERRORS`, and `has_seq()` is always true. A received Envelope is valid only when one known payload `has_*()` is true (`core/wire.gd`).
- **Generated output is deterministic.** It has no timestamp header, and absolute `--input`/`--output` paths work. That makes it possible to compare against the committed `proto/protocol.gd`.
- **CLI:** `godot --headless --path . -s res://addons/godobuf/godobuf_cmdln.gd --input=<abs.proto> --output=<abs.gd>`. Success prints `Output file was created successfully`.
- **A `-s` script with a script error never quits**, so it hangs. Every shell wrapper that runs Godot needs a watchdog. macOS has no `timeout`; use a background `sleep N; kill`.
- **GDScript constructs verified on 4.7.2:**
  - `await` on a two-argument signal returns `[a, b]`.
  - A `const` Dictionary can use preloaded enum values as keys.
  - `Proto.ErrorCode.values()` works.
  - Multi-line lambdas work and capture Arrays by reference.
  - `Vector2` components are float32, which matches the server's `float32`.
- **Server behaviour the client relies on:**
  - A `Ping` with seq 0 gets a `Pong` with seq 0 in any state.
  - The zone sends `EntityEnter` for the player's own entity too.
  - `WorldTick.moves` only lists entities that changed this tick.
  - `Kicked` is sent just before the server closes the socket.
  - An unauthenticated connection is kicked after 30 s (`IDLE_TIMEOUT`).
  - `REGISTER_PER_HOUR` defaults to 5 per IP, so smoke runs need `make run REGISTER_PER_HOUR=1000`.

## File structure

Server repo (`DemonSlayer2D`), changed in Tasks 1–2:

```
proto/ds/v1/protocol.proto        renames + EnterWorldResp.tick_rate = 8; header comment on GDScript-safe names
internal/pb/protocol.pb.go        regenerated (make proto)
internal/zone/zone.go             EnterWorldResp.TickRate; EntityEnter.CharClass
internal/gateway/{conn,handlers}.go, internal/client/client.go, cmd/botclient/main.go   follow the renames
internal/zone/zone_test.go, e2e/e2e_test.go   assert tick_rate
internal/gateway/gateway_test.go  new section CLIENT FIXTURES
testdata/client/*.hex             Go-encoded Envelopes for the client tests
.gdignore                         Godot skips this repo when it is the client's submodule
docs/.../2026-11-18-...-server-design.md, CLAUDE.md   document the renames and tick_rate
```

Client repo (`../DemonSlayer2DGame`, remote `git@github.com:zendF1/DemonSlayer2DGame.git`):

```
project.godot, .gitignore, README.md, CLAUDE.md
server/                     submodule → DemonSlayer2D
addons/godobuf/             vendored godobuf (no test/ dir) + its LICENSE
proto/protocol.gd           generated, committed
data/maps/*.json            copied from server/maps, committed
tests/fixtures/*.hex        copied from server/testdata/client, committed
autoload/config.gd          ws_url, --touch, move input actions
autoload/net.gd             WebSocket link, request/response by seq, pushes, ping, hold/release
autoload/router.gd          scene changes, notice, data handed to the world screen
core/wire.gd                decode + payload name of an Envelope
core/request_book.gd        pending seqs and deadlines
core/errors.gd              ErrorCode / KickReason / link failures → Vietnamese text
core/map_grid.gd            map file → size + blocked tiles
core/movement.gd            port of internal/zone/move.go
core/predictor.gd           own-character prediction and reconciliation
core/interpolator.gd        other entities drawn in the past
ui/ui.gd                    UI builders, colours, class colours and names
ui/slot_card.gd             character slot (Button drawn in code)
ui/map_view.gd              collision grid drawing, TILE_PX
ui/entity_view.gd           entity square, direction line, name
ui/joystick.gd              virtual joystick
ui/hud.gd                   CanvasLayer: place, ping, position, leave button
screens/login.{tscn,gd}     root Control
screens/char_select.{tscn,gd}  root Control
screens/world.{tscn,gd}     root Node2D (camera and world space; spec §5 is amended in Task 13)
tests/run.sh, tests/run_tests.gd, tests/test_case.gd
tests/unit/test_core.gd     sections MOVEMENT, MAP, ERRORS, PREDICTION, INTERPOLATION
tests/unit/test_net.gd      sections PROTOCOL, REQUESTS
tests/smoke/smoke.{tscn,gd} live-server smoke scene
tools/gen_proto.sh, tools/sync_server.sh, tools/smoke.sh, tools/run2.sh
```

Conventions for the client, taken from battle-squad-game:
- Use `preload` constants, never `class_name`, so headless `-s` runs need no global class cache.
- Start every script with a `##` doc comment that says what it is and points to the spec section.
- Section dividers in test files are `# ______________________TEST X_____________________` (user rule: one or two test files per folder).
- The default `GODOT` is `/Applications/Godot.app/Contents/MacOS/Godot`; scripts let you override it with `$GODOT`.

---

### Task 1: Server — GDScript-safe proto names and `tick_rate`

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/protocol.pb.go` (generated), `internal/zone/zone.go`, `internal/zone/zone_test.go`, `e2e/e2e_test.go`, `internal/gateway/conn.go`, `internal/gateway/handlers.go`, `internal/client/client.go`, `cmd/botclient/main.go`, `.gdignore`, server spec, `CLAUDE.md`.

Proto changes:
- `message Error` → `message ErrorResp { ErrorCode code = 1; string detail = 2; }`; in Envelope, `ErrorResp error = 2;`.
- `CharClass class` → `CharClass char_class` (same numbers) in `CharSummary` (4), `CreateCharReq` (3), `EntityEnter` (4).
- `EnterWorldResp`: `uint32 tick_rate = 8;` with the comment `// Zone ticks per second; each tick moves move_speed / tick_rate tiles.`
- Header comment: names must also be valid in GDScript (godobuf), so no field called `message` or `class` and no message called `Error`; renames keep field numbers.

- [ ] **Step 1: Failing tests.**
  - `TestJoinSendsEnterWorldRespFirst` (zone_test.go): also require `r.TickRate == 10`.
  - e2e helper after `EnterWorld`: fail unless `ew.TickRate == 10`.
  - Run `go test ./internal/zone/`. Expect a compile failure: no field `TickRate`.
- [ ] **Step 2: Implement.**
  - Edit the proto and run `make proto`.
  - Fix the compile errors. `go build ./... && go vet ./...` lists them: `pb.Error` → `pb.ErrorResp`, `Message` → `Detail`, `Class:` / `.Class` on pb types → `CharClass`. Internal structs such as `zone.PlayerInfo.Class` and `store.Character.Class` keep their names.
  - In `zone.join`, set `TickRate: uint32(z.cfg.TickRate)`.
- [ ] **Step 3: `.gdignore`.** Add it at the repo root with a one-line comment: Godot skips this repo when it is the client's `server/` submodule (it would otherwise import `docs/balance/*.csv` as translations).
- [ ] **Step 4: Docs.**
  - Server spec §4.1: `ErrorResp { ErrorCode code; string detail; }`.
  - §4.2: `CharSummary { …, char_class, … }`, `CreateCharReq { slot, name, char_class }`, and `EnterWorldResp { …, move_speed, tick_rate }`.
  - §4.3: `EntityEnter { …, char_class, … }`.
  - `CLAUDE.md` Invariants: one line saying proto names must stay valid in GDScript (no `message`/`class` field, no `Error` message), because the client generates code with godobuf.
- [ ] **Step 5: Verify.** `go vet ./... && staticcheck ./... && make test-db`. All pass.
- [ ] **Step 6: Commit and push.** `Make proto names GDScript-safe and send tick_rate in EnterWorldResp`, then `graphify update .` if the hook did not run.

### Task 2: Server — client fixtures

**Files:** `internal/gateway/gateway_test.go` (new section `// ______________________TEST CLIENT FIXTURES_____________________`), `testdata/client/*.hex`.

Interface:
- `var updateFixtures = flag.Bool("update", false, "rewrite testdata/client")`
- `func clientFixtures() map[string]*pb.Envelope` covers:
  - `login_req`: seq 3, username/password, `ProtocolVersion`.
  - `create_char_req`: seq 5, slot 2, a Vietnamese name with diacritics such as `Hoàng`, class THUY.
  - `move_input`: input_seq 9, dir (-0.6, 0.8).
  - `char_list_resp`: two chars.
  - `enter_world_resp`: seq 6, entity 77, zone 2, (5.5, 6.25), server_tick 1234, move_speed 4, tick_rate 10.
  - `entity_enter`: entity 78, PLAYER, MINH, level 2, dir (0, -1).
  - `world_tick`: tick 42, ack 7, two moves, one at x 1.3.
  - `error_resp`: seq 4, AUTH_FAILED, a detail string.
  - `kicked`: DUPLICATE_LOGIN.
  - `pong`: client 1700000000123, server 1700000000456.
  - Use neutral character names.
- `func TestClientFixtures(t *testing.T)`: marshal each fixture with `proto.MarshalOptions{Deterministic: true}` and hex-encode it with a trailing newline. With `-update`, write `../../testdata/client/<name>.hex`. Without it, compare with that file. On a mismatch, `t.Errorf` names the file and the command `go test ./internal/gateway -run TestClientFixtures -update`.

- [ ] **Step 1:** Write the test. Run it without `-update` and expect FAIL with missing files.
- [ ] **Step 2:** Run it with `-update`, then without. It passes. Check that `login_req.hex` is `080362180a0774616e6a69726f120b70617373776f72643132331801` if you used username `tanjiro`, password `password123` and version 1; otherwise just eyeball it.
- [ ] **Step 3:** `go vet ./... && make test-db`.
- [ ] **Step 4:** Commit and push: `Add Go-encoded protocol fixtures for the Godot client tests`.

### Task 3: Client — repository, project and test runner

**Files:** `project.godot`, `.gitignore`, `server/` (submodule), `addons/godobuf/`, `tests/run.sh`, `tests/run_tests.gd`, `tests/test_case.gd`, `tests/unit/test_core.gd` (one placeholder test, replaced in Task 5).

- [ ] **Step 1: Git.**
  - `git init -b main` in `../DemonSlayer2DGame`.
  - `git remote add origin git@github.com:zendF1/DemonSlayer2DGame.git`.
  - `git submodule add git@github.com:zendF1/DemonSlayer2D.git server`, pinned to the Task 2 commit.
- [ ] **Step 2: godobuf.**
  - Clone it into the session scratchpad and check out `523a66e2388c4235ded0aa5f2fecf91cf7e7a861`.
  - Copy `addons/godobuf` without `test/` and add its `LICENSE`.
  - Do not enable the editor plugin; only the CLI is used.
- [ ] **Step 3: `project.godot`.**
  - `config_version=5`, name `DemonSlayer2DGame`, features `4.7`, `Mobile`.
  - The `[display]` block matches battle-squad-game: 1600×900 viewport, 1280×720 window override, `canvas_items` / `expand`, landscape orientation 4.
  - `[rendering]` uses the mobile renderer with `import_etc2_astc=true`.
  - `[demonslayer] ws_url="ws://localhost:8080/ws"`.
  - No autoloads or main scene yet; later tasks add them as their files appear, because a missing autoload is a script error in every run.
- [ ] **Step 4: `.gitignore`:** `.godot/`, `/build/`, `.DS_Store`. Commit the `*.uid` files.
- [ ] **Step 5: Test runner.**
  - `tests/test_case.gd` extends `RefCounted` and declares `var failures: Array[String]`. It records failures instead of aborting, with these assertions:
    - `check(cond, msg)`
    - `eq(actual, expected, msg)`
    - `near(a: float, b: float, eps, msg)`
    - `near_vec(a: Vector2, b: Vector2, eps, msg)`
  - `tests/run_tests.gd` extends `SceneTree`. In `_initialize()` it loads every `res://tests/unit/test_*.gd` in sorted order and calls every `test_*` method on a fresh instance. It prints `FAIL file::method` and each failure, then `N passed, M failed`, and calls `quit(1 if M > 0 else 0)`.
  - `tests/run.sh` does four things:
    - Runs `$GODOT --headless --path . -s res://tests/run_tests.gd` with a 120 s watchdog.
    - Prints the log.
    - Fails if the log matches `SCRIPT ERROR|Parse Error|Failed to load script|Invalid call|timed out`.
    - Then, if `tools/sync_server.sh` is executable, runs it with `--check` and fails if that fails.
- [ ] **Step 6:** Run `$GODOT --headless --path . --import` once. Then `tests/run.sh` prints `1 passed, 0 failed`. Temporarily break the placeholder to see FAIL and exit 1, then restore it.
- [ ] **Step 7:** Commit and push: `Bootstrap the Godot 4.7 client: project, server submodule, godobuf and test runner`.

### Task 4: Client — generated protocol, data sync, protocol tests

**Files:** `tools/gen_proto.sh`, `tools/sync_server.sh`, `proto/protocol.gd`, `data/maps/`, `tests/fixtures/`, `core/wire.gd`, `tests/unit/test_net.gd` (section PROTOCOL).

Interfaces:
- `tools/gen_proto.sh [out.gd]`: default `proto/protocol.gd`. It makes paths absolute and runs the godobuf CLI on `$PWD/server/proto/ds/v1/protocol.proto`. It fails, printing the log, unless the log contains `Output file was created successfully`.
- `tools/sync_server.sh [--check]`:
  - Fails with `git submodule update --init` advice when `server/proto` is missing.
  - Without flags: replaces `data/maps` with `server/maps` and `tests/fixtures` with `server/testdata/client`, runs `gen_proto.sh`, and prints the server short SHA.
  - With `--check`: runs `diff -r` on both directories and compares a fresh `gen_proto.sh` output in a temp dir with `cmp`. Each stale part prints which one and `run tools/sync_server.sh`, and the script exits 1.
- `core/wire.gd` (RefCounted, static):
  - `const PAYLOADS: Array[String]` lists every Envelope oneof field name in proto order.
  - `static func payload(env) -> String`: the name of the payload whose `has_<name>()` is true, or `""`.
  - `static func decode(bytes: PackedByteArray)`: returns an Envelope, or null when `from_bytes` fails or `payload()` is `""`.

TODO / tests (`test_net.gd`, PROTOCOL):
- A helper `_fixture(name) -> PackedByteArray` reads `res://tests/fixtures/<name>.hex`, strips it and runs `hex_decode()`.
- `test_encode_login_req`, `test_encode_create_char_req` (UTF-8 name), `test_encode_move_input`: build each Envelope with the same values as Task 2; `to_bytes()` must equal the fixture.
- `test_decode_char_list_resp`, `test_decode_enter_world_resp` (including `get_tick_rate() == 10`), `test_decode_entity_enter`, `test_decode_world_tick` (two moves; x 1.3 within 1e-6), `test_decode_error_resp`, `test_decode_kicked`, `test_decode_pong`: `Wire.decode` gives the expected payload name and fields.
- `test_decode_rejects_payloadless`: `Wire.decode(PackedByteArray())` and `Wire.decode("0801".hex_decode())` are null.

- [ ] **Step 1:** Write the scripts and run `tools/sync_server.sh`. `proto/protocol.gd` appears with about 3,800 lines.
- [ ] **Step 2:** Write `test_net.gd` PROTOCOL and run `tests/run.sh`. Expect FAIL: `core/wire.gd` is missing.
- [ ] **Step 3:** Implement `core/wire.gd`. Tests pass and `sync_server.sh --check` passes.
- [ ] **Step 4:** Edit a fixture byte, check that `tests/run.sh` fails on `--check`, then restore it.
- [ ] **Step 5:** Commit and push: `Generate protocol code with godobuf and sync maps and fixtures from the server`.

### Task 5: Client — map grid and movement

**Files:** `core/map_grid.gd`, `core/movement.gd`, `tests/unit/test_core.gd` (sections MOVEMENT, MAP; delete the placeholder).

Interfaces:
- `core/map_grid.gd` (RefCounted, instance methods because there is no `class_name` to construct itself):
  - `var id: String`, `var name: String`, `var width: int`, `var height: int`
  - `const DIR := "res://data/maps"`
  - `static func path_of(map_id) -> String`, `static func has_map(map_id) -> bool` (`FileAccess.file_exists`)
  - `func parse(text: String) -> bool`: validates width and height > 0, `collision` holding `height` strings of `width` characters, and only `.` or `#`. On failure the grid is left unchanged.
  - `func load_map(map_id) -> bool`
  - `func blocked(tx: int, ty: int) -> bool`: outside the map counts as blocked.
- `core/movement.gd` (RefCounted, static), a line-for-line port of `internal/zone/move.go`:
  - `const HALF_SIZE := 0.3`, `MAX_SUBSTEP := 0.25`, `EPS := 1e-4`
  - `static func clamp_dir(d: Vector2) -> Vector2`: non-finite input becomes zero, length above 1 is normalised.
  - `static func step(grid, pos: Vector2, dir: Vector2, dist: float) -> Vector2`: `n = ceil(dist / MAX_SUBSTEP)` sub-steps, X then Y, the same `floor(± HALF_SIZE ∓ EPS)` row and column tests, stopping at the tile edge.
  - `grid` is any object with `blocked(int, int)`.

TODO / tests (port of `move_test.go`, same numbers, epsilon 1e-3):
- MOVEMENT:
  - `test_step_open_area`: (1.5, 1.5), right, 0.4 → (1.9, 1.5).
  - `test_step_stops_at_wall`: room `#####/#...#/#...#/#####`, left 1 → x 1.3.
  - `test_step_slides_along_wall`: clamp(-1, 1), 1 → x 1.3, y ≈ 2.207.
  - `test_step_no_tunnelling`: `.#....`, right 10 → x 0.7.
  - `test_step_outside_map_is_blocked`: `...`, left 1 → x 0.3.
  - `test_step_never_exceeds_distance`: 0.4 at clamp(0.6, 0.8) moves ≤ 0.4 + 1e-4.
  - `test_clamp_dir`: (3, 4) → (0.6, 0.8); (0.5, 0) is kept; NaN and INF → zero.
  - A helper `_grid(rows: Array)` builds a MapGrid through `parse(JSON.stringify(...))`.
- MAP:
  - `test_map_parse_kumotori`: `load_map("kumotori_1")` is true, 64×48, the border is blocked, the default spawn tile (5, 5) is open.
  - `test_map_outside_is_blocked`.
  - `test_map_rejects_bad_files`: a wrong row length, a bad character, a missing collision.

- [ ] Write the tests, run them and expect FAIL. Implement until they pass. Commit and push: `Port the server movement and map grid to GDScript`.

### Task 6: Client — error texts and request book

**Files:** `core/errors.gd`, `core/request_book.gd`, `tests/unit/test_core.gd` (section ERRORS), `tests/unit/test_net.gd` (section REQUESTS).

Interfaces:
- `core/errors.gd` (RefCounted, static):
  - Constants `LINK_LOST` ("Mất kết nối tới máy chủ"), `CANNOT_CONNECT` ("Không kết nối được máy chủ"), `MISSING_MAP` ("Client thiếu dữ liệu map, hãy cập nhật"), `UNKNOWN`, `CONFIRM_MISMATCH` ("Tên gõ lại không khớp").
  - `const ERROR_TEXT` maps every `Proto.ErrorCode` except UNSPECIFIED to short Vietnamese text. Examples: AUTH_FAILED "Sai tài khoản hoặc mật khẩu"; USERNAME_INVALID states the 4–20 `a-z 0-9 _` rule; PASSWORD_INVALID states 8–64; NAME_INVALID states 2–16 letters or digits with no spaces; VERSION_MISMATCH asks to update the client.
  - `const KICK_TEXT` maps every `Proto.KickReason` except UNSPECIFIED, e.g. DUPLICATE_LOGIN "Tài khoản đã đăng nhập ở nơi khác", SERVER_SHUTDOWN "Máy chủ đang bảo trì".
  - `static func error_text(code: int) -> String` (unknown → `UNKNOWN`)
  - `static func kick_text(reason: int) -> String` (unknown → `LINK_LOST`)
  - `static func of_reply(resp) -> String`: null → `LINK_LOST`; an `error` payload → `error_text(code)`; otherwise `""`.
- `core/request_book.gd` (RefCounted):
  - `func _init(timeout_ms := 10000)`
  - `func open(now_ms: int) -> int`: the next seq, starting at 1 and never reused.
  - `func settle(seq: int) -> bool`: false when seq is not pending.
  - `func expired(now_ms: int) -> Array[int]`: removed and sorted.
  - `func drain() -> Array[int]`: every pending seq, removed and sorted.
  - `func pending() -> int`

TODO / tests:
- ERRORS:
  - `test_every_error_code_has_text`: iterate `Proto.ErrorCode.values()`, skipping 0.
  - `test_every_kick_reason_has_text`.
  - `test_of_reply`: null, an error reply decoded from the `error_resp` fixture, and a result reply.
- REQUESTS:
  - `test_seq_counts_from_one`.
  - `test_settle_only_pending`: a second settle of the same seq and an unknown seq are false.
  - `test_expired_after_timeout`: just before the deadline nothing expires; at the deadline the seq expires once.
  - `test_drain_returns_all`.

- [ ] Write the tests, run them and expect FAIL. Implement. Commit and push: `Add Vietnamese error texts and the pending-request book`.

### Task 7: Client — predictor

**Files:** `core/predictor.gd`, `tests/unit/test_core.gd` (section PREDICTION).

Algorithm (spec §6.3, refined here; Task 13 writes it back into the spec):
- **History entries store the position *before* the tick's step:** `{seq, dir, from}`. A server position always lies at the start of some tick of the input it is currently running. This gives the server k ≥ 0 steps of that input.
- **Candidates for a WorldTick with ack `a`** are the entries with `seq == a`, plus the current `pos` when `input_seq == a`. The server has finished every earlier input, and it keeps running input `a` until the next one arrives, so the later `seq == a` ticks will happen on the server too.
  - Matching against *older* seqs is wrong. Example: the client steps right 10 times and the server only 9 before the stop arrives. The 9th right entry would match exactly, and the client would stay one step ahead for ever.
- **On the nearest candidate `i`:**
  - If its distance is ≤ `NOISE_TILES` (0.001): drop the entries before `i` and stop.
  - Otherwise: `p = server_pos`, replay `Movement.step(grid, p, e.dir, step_dist)` for each entry from `i` on (each replayed entry's `from` is updated), set `pos = p`, and drop the entries before `i`.
  - If the current-pos candidate matched, the history is cleared.
- **No candidate** (ack older than the history, or none at that seq): ignore the update.
- **Drawing:**
  - `prev_pos` shifts by the same jump as `pos`, so drawing between ticks stays continuous.
  - A jump below `SNAP_TILES` (0.5) adds `offset -= jump` (the drawn position does not move yet) and decays the offset to zero linearly over `SMOOTH_SEC` (0.1 s). A larger jump sets the offset to zero.
- **Sending:**
  - The predictor moves with `sent_dir`, never with the raw input. Its direction is therefore always what the server was told.
  - A new direction is sent only when it differs from `sent_dir` and at least `SEND_GAP_SEC` (0.05) has passed since the last send. Otherwise it waits for a later tick.

Interface (`core/predictor.gd`, RefCounted):
- `func _init(grid, start: Vector2, move_speed: float, tick_rate: int)`
- Fields:
  - `tick_sec`, `step_dist` (`move_speed / tick_rate`)
  - `pos`, `prev_pos`
  - `sent_dir`, `input_seq` (0 until the first send)
  - `offset`
  - The history is kept for at most `2 * tick_rate` entries.
- `func tick(want: Vector2)`: clamps `want`, may send (advancing `input_seq`), records `{seq, dir, from}`, steps. Returns the direction to send, or null.
- `func reconcile(server_pos: Vector2, ack: int) -> bool`: true when it corrected.
- `func decay(delta: float) -> void`: shrinks `offset`.
- `func draw_pos(alpha: float) -> Vector2`: `prev_pos.lerp(pos, alpha) + offset`.
- `func history_size() -> int`: for tests.

TODO / tests (open 40×40 grid, start (10, 10), speed 4, rate 10, so step 0.4):
- `test_prediction_moves_with_sent_dir`
- `test_prediction_sends_only_changes`: right → sent (seq 1); right → null; zero → sent (seq 2).
- `test_prediction_send_gap`: rate 100, so two changes 1 tick apart: the second waits until 0.05 s has passed.
- `test_prediction_server_on_path_keeps_prediction`: 5 right ticks, then reconcile((11.2, 10), 1) → false, pos (12, 10), history trimmed.
- `test_prediction_server_stopped_earlier`: 10 right (seq 1), 3 zero (seq 2), then reconcile((13.6, 10), 2) → true, pos (13.6, 10).
- `test_prediction_replays_unacked`: 5 right (seq 1), 5 down (seq 2), then reconcile((11.1, 10), 1) → true, pos (11.9, 12).
- `test_prediction_small_correction_is_smoothed`: `draw_pos(1)` is unchanged right after the correction and reaches the new pos after `decay(0.1)`.
- `test_prediction_big_correction_snaps`: offset is zero.
- `test_prediction_ignores_ack_older_than_history`
- `test_prediction_wall`: the server pos is clamped at the wall and the replay keeps sliding.

- [ ] Write the tests, run them and expect FAIL. Implement. Commit and push: `Predict the own character with fixed ticks and reconcile from WorldTick`.

### Task 8: Client — interpolator

**Files:** `core/interpolator.gd`, `tests/unit/test_core.gd` (section INTERPOLATION).

Interface (RefCounted):
- `const DELAY_TICKS := 2.0`, `MAX_SAMPLES := 32`
- `func _init(tick_rate: int)`, `var latest_tick := -1`
- `func on_tick(tick: int)`: a newer tick resets the time since the latest one.
- `func advance(delta: float)`
- `func render_tick() -> float`: `latest - DELAY + min(since / tick_sec, DELAY)`, so it never passes `latest`.
- `func add(id: int, tick: int, pos: Vector2, dir: Vector2)`:
  - An older tick is ignored; an equal tick replaces the last sample.
  - After a gap of more than one tick, first insert a copy of the last sample at `tick - 1`, so an entity idle for a while does not glide from its old sample.
  - Trim to `MAX_SAMPLES`.
- `func position(id) -> Vector2`: before the first sample it is the first sample; between samples, a lerp; after the last, the last.
- `func direction(id) -> Vector2`: the dir of the newest sample at or before the render tick.
- `func remove(id)`, `func has(id) -> bool`

TODO / tests:
- `test_interp_between_samples`: samples at ticks 10, 11, 12 at x 0, 0.4, 0.8 with latest 12 → x 0; after `advance(0.05)` → x 0.2.
- `test_interp_stops_at_last_sample`: `advance(1)` → x 0.8 and it stays there.
- `test_interp_before_first_sample`
- `test_interp_hold_after_gap`: a sample at 5, then one at 20; render at 18 → the tick-5 position.
- `test_interp_ignores_older_tick`
- `test_interp_remove`

- [ ] Write the tests, run them and expect FAIL. Implement. Commit and push: `Interpolate other entities two ticks in the past`.

### Task 9: Client — autoloads, UI builders and the live smoke test

**Files:** `autoload/config.gd`, `autoload/net.gd`, `autoload/router.gd`, `ui/ui.gd`, `project.godot` (autoloads in this order: Config, Net, Router), `tests/smoke/smoke.{tscn,gd}`, `tools/smoke.sh`.

Interfaces:
- `Config`:
  - `var ws_url` from `demonslayer/ws_url`, `--ws-url=`.
  - `var force_touch` (`--touch`).
  - `_init()` adds the actions `move_left`/`right`/`up`/`down` with physical keys A/D/W/S and the arrows when they are missing.
- `Net`:
  - Signals: `connected`, `disconnected(reason: String)`, `pushed(env)`.
  - Constants: `PROTOCOL_VERSION := 1` (must equal `gateway.ProtocolVersion`), `REQUEST_TIMEOUT_MS := 10000`, `PING_EVERY_SEC := 5.0`. `var ping_ms := -1`.
  - `func open_link() -> bool` (awaitable): connects to `Config.ws_url` if the link is not already open. Returns false when the socket closes before opening.
  - `func is_open() -> bool`
  - `func request(env)` (awaitable): sets seq from the RequestBook, sends binary, and waits on an internal `_settled(seq, env)` signal for its seq. Returns the reply or null (timeout, closed, not open).
  - `func send(env)`: seq 0, no reply.
  - `func close()`: on purpose. Settles all pending requests with null, clears any hold, and emits no `disconnected`.
  - `func hold()` / `func release(deliver := true)`: buffers pushes across the change to the world scene.
  - `_process` behaviour:
    - Poll the socket. On the first OPEN, emit `connected` and resolve `open_link`.
    - Drain packets through `Wire.decode`. Undecodable packets get a `push_warning`.
    - A seq other than 0 settles a pending request; a late reply is dropped.
    - A `Pong` sets `ping_ms`. `Kicked` stores `Errors.kick_text`. Anything else goes to `pushed`, or into the hold buffer.
    - Ping every 5 s with `Time.get_ticks_msec()`. Expire requests.
    - On CLOSED: read any packets still left first (the `Kicked`), then settle every pending request with null, clear the hold, and emit `disconnected(kick text or LINK_LOST)` if the link had been open.
- `Router`:
  - `const LOGIN, CHAR_SELECT, WORLD` (scene paths)
  - `var enter_world` (EnterWorldResp or null), `var character_name`
  - `func goto(path)`: `change_scene_to_file` deferred.
  - `func notice(text)` / `func take_notice() -> String`
  - `_ready` connects `Net.disconnected` to: reset `enter_world`, store the reason as the notice, `goto(LOGIN)`.
- `ui/ui.gd` (static):
  - Colours `BG`, `CARD_BG`, `CARD_SELECTED`, `TEXT_DIM`, `ERROR`, `ACCENT`.
  - `const CLASSES` (VIEM, THUY, MINH, NHAM, PHONG), `class_color(cls) -> Color`, `class_label(cls) -> String` (Viêm, Thủy, Minh, Nham, Phong).
  - Builders: `fill_background(parent)`, `center_column(parent) -> VBoxContainer`, `label(text, size, color)` (centred), `text(...)` (left-aligned), `button(text, size)`, `line_edit(placeholder, secret := false)`, `vbox(sep)`, `hbox(sep)`, `card(bg, padding)`.
- Smoke scene (`tests/smoke/smoke.gd`, root Node):
  - Connects `Net.disconnected` so a kick fails fast with the reason.
  - Steps: register `smoke_<random>`; log in; create `Smoke<digits>` in slot 1; enter; assert `tick_rate > 0`; send `MoveInput(1, (1, 0))`; wait 1 s; assert a self `EntityMove` with x > start x; leave; `Net.close()`.
  - Prints `SMOKE OK` and quits 0, or `SMOKE FAIL: <step>: <reason>` and quits 1.
- `tools/smoke.sh`: skips with exit 0 when `DS_SERVER_URL` is unset. Otherwise it runs `$GODOT --headless --path . res://tests/smoke/smoke.tscn -- --ws-url=$DS_SERVER_URL` with a 30 s watchdog, and requires `SMOKE OK` and no script errors.

- [ ] **Step 1:** Implement Config, Net, Router and ui.gd; register the autoloads. `tests/run.sh` still passes, which proves the autoloads parse.
- [ ] **Step 2:** Write the smoke scene and script.
  - In the server repo, run `make db-up && make run REGISTER_PER_HOUR=1000`.
  - Then run `DS_SERVER_URL=ws://localhost:8080/ws tools/smoke.sh` and expect `SMOKE OK`.
  - Run it with the server stopped and expect a fast `SMOKE FAIL: connect`.
- [ ] **Step 3:** Commit and push: `Add the Net link, Router and Config autoloads and a live-server smoke test`.

### Task 10: Client — login screen

**Files:** `screens/login.{tscn,gd}`, `project.godot` (`run/main_scene="res://screens/login.tscn"`).

Behaviour (spec §5.1):
- Layout: title, username, password (`secret`; Enter submits), the main button, a flat switch button, and a red message label that starts from `Router.take_notice()`.
- Submitting:
  - Lock both buttons while busy.
  - `await Net.open_link()`; if it fails, show `CANNOT_CONNECT`.
  - In register mode, send `RegisterReq` first.
  - Then send `LoginReq` with the lowercased trimmed username and `PROTOCOL_VERSION`.
  - On success, `Router.goto(CHAR_SELECT)`.
  - On failure, call `Net.close()` (an idle open link would be kicked after 30 s) and show `Errors.of_reply`.

TODO:
- `_ready()` builds the form.
- `_refresh()` updates texts and disabled state for the mode.
- `_on_switch()`.
- `_on_submit()`.
- `_attempt(username, password) -> String` returns `""` or the text to show.

- [ ] **Step 1:** Implement. Run `$GODOT --path .` against a running server and check by hand:
  - A wrong password shows the AUTH_FAILED text.
  - A short username in register mode shows USERNAME_INVALID.
  - Registering logs straight in; since char select does not exist yet, an error about the missing scene is expected here.
- [ ] **Step 2:** `tests/run.sh` passes. Commit and push: `Add the login and registration screen`.

### Task 11: Client — character select

**Files:** `ui/slot_card.gd`, `screens/char_select.{tscn,gd}`.

Interfaces:
- `ui/slot_card.gd` extends Button: flat, no focus, `SIZE := Vector2(260, 300)`.
  - `func _init(slot: int)`, `var slot`, `var selected` (the setter redraws), `func show_char(summary_or_null)`.
  - `_draw()`: the card background (selected colour and an `ACCENT` outline when selected). An empty slot shows a plus sign and "Ô N trống". A filled slot shows a 96 px square in the class colour, the name, and "<class> · Cấp N" drawn with `draw_string` centred.
- `screens/char_select.gd`: TODO
  - `_ready()` builds the title, the 3 cards, the detail area, the message label from `Router.take_notice()` and "Đăng xuất", then calls `_reload()`.
  - `_reload()`: `CharListReq` → slot map → cards → `_select(current)`.
  - `_select(slot)` rebuilds the detail area:
    - A filled slot gets "Vào game" and "Xóa".
    - An empty slot gets the name field, 5 toggle buttons (`ButtonGroup`, class-coloured text) and "Tạo nhân vật".
  - `_create(slot, name, cls)` → `CreateCharReq`, then `_reload()`.
  - `_ask_delete(c)`: a `ConfirmationDialog` containing the text and a name field. It checks the typed name client-side (`CONFIRM_MISMATCH`), then calls `_delete(c, confirm)` → `DeleteCharReq` → `_reload()`.
  - `_enter(c)`:
    - Call `Net.hold()`, then `EnterWorldReq`.
    - On an error, `Net.release(false)` and show it.
    - If `MapGrid.has_map(map_id)` is false, send `LeaveWorldReq`, `Net.release(false)` and show `MISSING_MAP`.
    - Otherwise set `Router.enter_world` and `character_name`, then `goto(WORLD)`.
  - `_logout()`: `Net.close()` and `goto(LOGIN)`.
  - A `_busy` guard on every action.

- [ ] **Step 1:** Implement. Check by hand against the server:
  - Creating in slots 1 and 3 works.
  - A duplicate name shows NAME_TAKEN.
  - Deleting with a wrong typed name is refused client-side; with the right name it works.
  - Logging out returns to login.
- [ ] **Step 2:** `tests/run.sh` passes. Commit and push: `Add the character select screen`.

### Task 12: Client — world screen, views, joystick, HUD

**Files:** `ui/map_view.gd`, `ui/entity_view.gd`, `ui/joystick.gd`, `ui/hud.gd`, `screens/world.{tscn,gd}` (root Node2D).

Interfaces:
- `ui/map_view.gd` (Node2D):
  - `const TILE_PX := 32`, `func _init(grid)`, `z_index = -1`.
  - `_draw()` paints the whole map light, then the blocked tiles dark, then faint grid lines. It is drawn once.
- `ui/entity_view.gd` (Node2D):
  - `func _init(name: String, char_class: int, is_self: bool)`, `var dir` (the setter redraws).
  - `_draw()`: a 0.6-tile square in the class colour; a white outline for the player's own character; a direction line when `dir` is not zero; the name above with an outline (`draw_string_outline`, then `draw_string`, using `ThemeDB.fallback_font`).
  - The player's own entity has `z_index = 1`.
- `ui/joystick.gd` (Control, bottom left):
  - `RADIUS := 90`, `KNOB := 38`, `DEAD_ZONE := 0.15`, `MARGIN := 48`. `var active`, `var value: Vector2`.
  - Handles left-mouse press, motion and release only. Touch arrives as emulated mouse, which is enough for one finger.
  - `value` is the knob offset over the radius, at most 1, and zero inside the dead zone.
  - `_draw()` draws the base circle and the knob.
- `ui/hud.gd` (CanvasLayer):
  - `signal leave_pressed`.
  - A top-left translucent card shows the place ("<map name> · Khu N") and the status ("Ping N ms · (x, y)", "…" while unknown).
  - A top-right "Về chọn nhân vật" button.
  - `func set_place(map_id, map_name, zone)`, `func set_status(ping_ms, pos: Vector2)`.
- `screens/world.gd`: TODO
  - `_ready()`:
    - Take `Router.enter_world`; if it is null, go to char select.
    - Load the MapGrid.
    - Build the Predictor (`tick_rate`, at least 1) and the Interpolator (`on_tick(server_tick)`).
    - Add the MapView, a Camera2D limited to the map and made current, the HUD, and the joystick inside the HUD (visible when `DisplayServer.is_touchscreen_available()` or `Config.force_touch`).
    - Connect `Net.pushed`, then call `Net.release()`.
  - `_process(delta)`:
    - Accumulate time and run at most 5 predictor ticks per frame. When the cap is hit, drop the rest of the time.
    - Send each direction the predictor returns as a `MoveInput` with `predictor.input_seq`.
    - Decay the offset and advance the interpolator.
    - Place the own view and the camera at `draw_pos(accum / tick_sec) * TILE_PX`, and the other views at `interp.position(id) * TILE_PX` with `direction(id)`.
    - Update the HUD status.
  - `_wanted_dir()`:
    - While leaving, the direction is zero.
    - An active joystick wins over the keyboard.
    - Otherwise `Input.get_vector("move_left", "move_right", "move_up", "move_down")`.
  - `_on_pushed(env)`:
    - `entity_enter`: create or replace the view. The own entity becomes `_self_view`; others are added to the interpolator at `latest_tick`.
    - `entity_leave`: free the view and `remove` it.
    - `world_tick`: call `on_tick`. Each move for the own entity goes to `reconcile(pos, ack)`; the others go to `add(id, tick, pos, dir)`.
  - `_leave()`:
    - Set the leaving flag and send `LeaveWorldReq`.
    - A null reply means the link dropped, and Router handles it.
    - Otherwise clear `Router.enter_world`, set the notice to `of_reply` (`""` on success), and `goto(CHAR_SELECT)`.

- [ ] **Step 1:** Implement. Check by hand with one client:
  - The map shows and walking with WASD feels immediate.
  - Walking into walls slides with no snapping back.
  - "Về chọn nhân vật" returns to char select, and entering again stands on the same tile.
- [ ] **Step 2:** `$GODOT --path . -- --touch`: the joystick shows and drags with the mouse, and the character follows.
- [ ] **Step 3:** `tests/run.sh` passes. Commit and push: `Add the world screen with prediction, interpolation, joystick and HUD`.

### Task 13: Client — two-window play test, docs, spec status

**Files:** `tools/run2.sh`, `README.md`, `CLAUDE.md` (client), and in the server repo the client spec (status, §5 world root Node2D, §6.3 history stores the pre-step position and matches only `seq == ack`).

- `tools/run2.sh [user args]`: starts two `$GODOT --path .` windows at `--position 40,80` and `1020,80` with `--resolution 960x540`, passes on the extra args after `--`, and waits.
- `README.md`:
  - Clone with `--recurse-submodules` (or run `git submodule update --init`).
  - Requirements: Godot 4.7 path, `$GODOT`.
  - `--import` once.
  - `tests/run.sh`; the smoke test with `REGISTER_PER_HOUR`.
  - Run with `--ws-url` and `--touch`; `tools/run2.sh`.
  - Update the protocol: bump the submodule, run `tools/sync_server.sh`, commit.
- `CLAUDE.md` (short, like the server's):
  - Commands.
  - UI built in code (battle-squad-game pattern).
  - `preload`, never `class_name`.
  - `core/` stays Node-free and unit-tested.
  - Never edit `proto/`, `data/maps/` or `tests/fixtures/` by hand; run `tools/sync_server.sh`.
  - Godot `-s` hangs on script errors (use the wrappers).
  - 1–2 test files per folder with section dividers.
  - Commit and push after each piece of work, with no signature trailer.
  - Specs live in the server repo.

- [ ] **Step 1: Play test against the done criteria.** Run `make run` (server), then `tools/run2.sh`, and check each item from spec §9:
  - Both windows see each other move smoothly.
  - Walls neither snap nor let anyone through.
  - The joystick works with `-- --touch`.
  - Logging in with the same account in the second window sends the first back to login with "Tài khoản đã đăng nhập ở nơi khác".
  - Leaving and re-entering keeps the position.
  - Note anything that fails and fix it before continuing.
- [ ] **Step 2:** Run `tests/run.sh`, `DS_SERVER_URL=… tools/smoke.sh`, and in the server repo `go test -race ./...`. All pass.
- [ ] **Step 3:** Write the README, the CLAUDE.md and run2.sh. Commit and push the client: `Add the two-window play script, README and CLAUDE.md`.
- [ ] **Step 4:** Server repo: mark the client spec as implemented and write the refinements back into it. Commit and push: `Mark the phase 0 client spec implemented and record the prediction refinements`.
