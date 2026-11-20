# Phase 7a Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Players chat on five channels: Thế giới (everyone online, level 10+, 1 line per 10 s per character and 5 per second server-wide), Khu (same map and zone, 1 per 2 s), Tổ đội (1 per s), Riêng by character name (1 per s) and server-only Hệ thống. Text is cleaned to one line of 1–100 characters. The server echoes every line to its sender. The client has a HUD chat line, a tabbed chat window, whisper and block from a tapped name, and a client-side block list.

**Architecture:**
- `internal/textutil` (new): `Clean` shared by chat (7a), stall titles (7b) and guild notices (7c).
- `internal/chat` (new): world-level service in the 5b party mould — members under one lock, five `auth.Limiter`s, pushes `ChatMessage` straight into session outboxes, never calls the world or a zone. `System`/`SystemTo` for server notices.
- World: register on `Enter`, unregister on `Leave`, `Moved` on `Transfer`; the 5b `Leveled` hook fans out to party and chat.
- Gateway: `ChatSendReq` handled synchronously on the connection goroutine.
- Protocol 16: envelopes 125–127, `ChatChannel` enum (6 reserved for 7c's guild), errors 57–59. No migration.
- Dev: `botclient say`/`tell`, partner prints `CHAT` lines.
- Client: `core/chat_log.gd` (`Router.chat`), block list in `user://chat.cfg`, `ui/chat_line.gd`, `ui/chat_window.gd`, player card "Nhắn riêng".

**Tech stack:** Go, protobuf (buf); Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7a-chat-design.md](../specs/2026-11-20-giai-doan-7a-chat-design.md). Section references below (§n) point to it. Phase overview: [2026-11-20-giai-doan-7-tong-quan.md](../specs/2026-11-20-giai-doan-7-tong-quan.md).

**Prerequisite:** the [phase 6d plan](2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay.md) is fully implemented (protocol 15, migration 00009, the 5b party service with its party-id cell and `Leveled` hook, the 5b player card). Start from HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f; client HEAD 92d588b)

The server code is at **phase 3a (protocol 6)**, with 3b landing (`internal/items/enhance.go`, `migrations/00005_enhance.sql`). Phases 3b–6d are designed (specs in `docs/superpowers/specs/`) but not implemented when this plan is written; they land before it. Line numbers below are from HEAD 618ca8f and will move — find each symbol by name (`graphify explain "<symbol>"`) before editing.

- **Limiter:** `auth.Limiter` (`internal/auth/limiter.go:9`) is a sliding-window counter: `NewLimiter(max, window)` (`:18`), `Take(key) bool` checks and records under one lock (`:25`), `Refund(key)` gives back the latest take (`:41`). Its clock is the unexported field `now`; add `NewLimiterAt(max int, window time.Duration, now func() time.Time) *Limiter` for the chat tests rather than a second limiter type. The gateway's per-connection limits are `golang.org/x/time/rate` limiters (`internal/gateway/conn.go:47-65`, `packets`, `moves`, `attacks`, `casts`); chat does not use those.
- **Outbox:** `zone.Outbox` (`internal/zone/zone.go:26`) is `Send(*pb.Envelope) bool` + `Kick`; the gateway implementation `outbox` (`internal/gateway/outbox.go:11`, `Send` at `:24`) is safe from many goroutines and never blocks. The chat service stores this interface, exactly like the 5b party service.
- **Names:** `auth.NormalizeCharName(s) (display, key string, err error)` (`internal/auth/names.go:43`) is NFC + lower-case and rejects anything but 2–16 letters/digits; use the same key for the private-chat index and to validate `to_name`.
- **World:** `World.Enter` (`internal/world/world.go:272`) returns a `*Membership` holding `EntityID`, `MapID`, `ZoneNo` (`:202`); `Membership.Leave` (`:269`) is the leave path. 4a adds `World.Transfer` and 5b adds the party registration beside `Enter`/`Leave`/`Transfer` — register chat at the same three places. The world's `online` map (`:63`) is per account and holds only `Session` (kick/done), so it is not a name index; chat keeps its own.
- **Gateway dispatch:** the `switch` over `*pb.Envelope_…` payloads is in `internal/gateway/handlers.go:20-135`; in-world commands start with `c.inWorld(seq)`. `ChatSendReq` needs no zone: call the chat service directly, as 5b does for party requests.

## File structure

Server (`DemonSlayer2D`):

```
internal/textutil/textutil.go (new), textutil_test.go (new)     text cleaning (Task 1)
internal/auth/limiter.go, auth_test.go                           NewLimiterAt (Task 1)
internal/chat/chat.go (new), chat_test.go (new)                  chat service (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures       protocol 16 (Task 3)
internal/world/world.go, world_test.go, internal/app/app.go      registration (Task 4)
internal/gateway/handlers.go, server.go, gateway_test.go         ChatSendReq (Task 4)
internal/client/client.go, cmd/botclient/main.go, e2e/           client helper, botclient, e2e (Task 5)
docs/game-design.md, CLAUDE.md, README.md                        docs (Task 6)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, tests/fixtures/                    sync (Task 7)
core/wire.gd, core/errors.gd                                     protocol 16 (Task 7)
core/chat_log.gd (new), autoload/router.gd, autoload/config.gd   model, block list (Task 8)
ui/chat_line.gd (new), ui/chat_window.gd (new), ui/hud.gd,
ui/player_card.gd, screens/world.gd                              UI (Task 9)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md   tests, smoke (Tasks 7–10)
```

---

### Task 1: Server — text cleaning and a test clock for the limiter

**Files:** `internal/textutil/textutil.go` (new), `internal/textutil/textutil_test.go` (new), `internal/auth/limiter.go`, `internal/auth/auth_test.go`.

**Interface:**
- `const MaxRawBytes = 1024`.
- `func Clean(s string, maxRunes int) (string, bool)` — §2: raw bytes > `MaxRawBytes` → false; NFC (`golang.org/x/text/unicode/norm`, already a dependency); drop runes in `unicode.Cc` and `unicode.Cf`; collapse `unicode.IsSpace` runs to one space; trim; true when 1..`maxRunes` runes remain.
- `func NewLimiterAt(max int, window time.Duration, now func() time.Time) *Limiter` — like `NewLimiter` with an injected clock.

**TODO / tests:**
- [ ] `// ____TEST CLEAN____` (textutil): every example of spec §8 "textutil".
- [ ] `// ____TEST LIMITER____` (auth, existing section): `NewLimiterAt` with a fake clock — two takes of a 1-per-10 s limiter fail on the second, pass after advancing 10 s; `Refund` restores the slot.
- [ ] Commit: `Add the shared text cleaner and a limiter with an injectable clock`.

### Task 2: Server — chat service

**Files:** `internal/chat/chat.go` (new), `internal/chat/chat_test.go` (new).

**Interface:**
- `type Member struct { ID uint64; CharID int64; Name string; Level int; MapID string; ZoneNo int; Party *atomic.Uint64; Out zone.Outbox }` (key computed in `Register` with `auth.NormalizeCharName`; a name that fails normalisation is logged and still registered by `strings.ToLower(Name)`).
- `type Config struct { Now func() time.Time }` (nil → `time.Now`); `func New(cfg Config, log *slog.Logger) *Service`.
- `func (s *Service) Register(m Member)`, `Unregister(id uint64)`, `Moved(id uint64, mapID string, zoneNo int)`, `Leveled(id uint64, level int)`.
- `func (s *Service) Send(from uint64, ch pb.ChatChannel, text, toName string) pb.ErrorCode` — §4.2; `ERROR_CODE_UNSPECIFIED` on success.
- `func (s *Service) System(text string)`, `func (s *Service) SystemTo(ids []uint64, text string)` — §4.3.
- Constants of spec §2.

**Behaviour:** §4.1–4.3. Validate in the exact order of §4.2 before any `Take`; the WORLD per-character take comes before the global take and is refunded when the global one fails. Collect recipients under the lock, send after unlocking; the echo to the sender is sent before `Send` returns (the gateway writes `ChatSendResp` after). Party recipients are the members whose `Party.Load()` equals the sender's non-zero value (scan of all members; ≤ 1,000).

**TODO / tests** (`// ____TEST SEND____`, `// ____TEST LIMITS____`, `// ____TEST MEMBERS____`, fake outboxes record envelopes):
- [ ] Error order per step of §4.2; `SYSTEM`/`UNSPECIFIED` from a client → `BAD_REQUEST`.
- [ ] Recipient sets for the four channels; echo first; `to_name` is the receiver's display name.
- [ ] Private by upper-case name and by decomposed accents reaches the target; unknown, invalid, unregistered → `CHAT_TARGET_OFFLINE`; self → `BAD_REQUEST`.
- [ ] Level 9 world → `LEVEL_TOO_LOW`; after `Leveled(id, 10)` it passes.
- [ ] Limits: world 1/10 s; failed sends keep the slot; sixth sender in one second → `CHAT_COOLDOWN` and the refund lets that sender through once the global window frees; zone 1/2 s; party/private 1/s; re-registering the same `CharID` keeps the cooldown.
- [ ] `Moved` changes the zone recipients; `System` reaches everyone; `SystemTo` skips unknown ids.
- [ ] Commit: `Add the world-level chat service with per-channel limits`.

### Task 3: Server — protocol 16

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex` (the fixture directory the gateway tests already write).

**Behaviour (§5):** envelopes 125–127, `ChatChannel` (comment reserving 6 for `CHAT_CHANNEL_GUILD`), `ChatMessage`; `CHAT_TEXT_INVALID = 57`, `CHAT_TARGET_OFFLINE = 58`, `CHAT_COOLDOWN = 59` with one-line comments. `ProtocolVersion = 16`. Confirm the last used envelope is 124 and the last error 56 before adding.

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `chat_send_req`, `chat_message`, `chat_message_system`; regenerate all with `-update`.
- [ ] Version test → 16.
- [ ] Commit: `Add the phase 7a chat messages and bump the protocol to 16`.

### Task 4: Server — world registration and the gateway handler

**Files:** `internal/world/world.go`, `internal/world/world_test.go`, `internal/app/app.go`, `internal/gateway/handlers.go`, `internal/gateway/server.go`, `internal/gateway/gateway_test.go`.

**Interface:** `world.Config.Chat *chat.Service` (nil allowed in old tests: every call guarded); the world's `Leveled` func passed to zones calls `party.Leveled` then `chat.Leveled`; gateway server config gets the same `*chat.Service`; `case *pb.Envelope_ChatSendReq` → `c.inWorld(seq)` → `chat.Send(c.member.EntityID, …)` → `ChatSendResp` or `c.fail(seq, code, "")`.

**Behaviour:** §4.4–4.5. Register after the zone accepted the join (same place as `party.Register`), with the party cell from `PlayerInfo.Party`; unregister in `Membership.Leave` before the zone leave (same place as party); `Moved` after a successful `Transfer`. `app.New` builds one chat service and hands it to world and gateway.

**TODO / tests:**
- [ ] `// ____TEST CHAT____` (world): `Enter` registers (a ZONE `Send` from the member reaches a second member of the same zone); `Leave` unregisters; a portal transfer moves zone recipients; a level-up reaches `chat.Leveled` (world 9 → 10 allows world chat).
- [ ] Gateway: `ChatSendReq` before entering → `NOT_IN_WORLD`; fixtures decode.
- [ ] Commit: `Register players with the chat service and handle chat requests`.

### Task 5: Server — client helper, botclient and e2e

**Files:** `internal/client/client.go`, `cmd/botclient/main.go`, `e2e/e2e_test.go`.

**Interface:** `client.Chat(ctx, ch pb.ChatChannel, text, toName string) error` (waits for `ChatSendResp` or the error); the helper records `ChatMessage` pushes (`client.ChatLines()`); botclient interactive `say <world|zone|party> <text>`, `tell <name> <text>`; `partner` prints `CHAT <channel> <from> <text>` for every `ChatMessage` (§6.3, §7).

**TODO / tests:**
- [ ] e2e `TestChatOverWebSocket` per spec §8 (A level 10 via `DevSetLevel`, B level 1 in the same Kumotori zone via `DevSetPlace`, C in Ubuyashiki).
- [ ] Commit: `Add chat to the test client and botclient and an end-to-end chat test`.

### Task 6: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`.

**TODO:**
- [ ] game-design chat section per §9; CLAUDE.md invariants (chat service never calls zones; chat limits use `Limiter.Take`/`Refund`; `internal/textutil` is the one text cleaner); README: channels, limits, botclient `say`/`tell`.
- [ ] Commit: `Document the phase 7a chat`.

### Task 7: Client — sync and protocol 16

**Files:** `server` submodule, `proto/protocol.gd`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 6; `tools/sync_server.sh`; `PROTOCOL_VERSION := 16`; `PAYLOADS` adds `chat_send_req`, `chat_send_resp`, `chat_message` (godobuf: enum values lose the prefix, `Proto.ChatChannel.ZONE`); error texts of §6.1.

**TODO / tests:**
- [ ] Decode the three new fixtures.
- [ ] Commit: `Sync the phase 7a chat protocol from the server`.

### Task 8: Client — chat model and block list

**Files:** `core/chat_log.gd` (new), `autoload/router.gd`, `autoload/config.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`add`, `add_local`, `lines`, `unread`, `open`, `last`, `block`, `unblock`, `is_blocked`, `clean`, `cooldown_left`, `mark_sent`, `prefix`); `Router.chat` created on entering the world, cleared on returning to character select; `Config.load_blocked`, `Config.save_blocked`.

**TODO / tests** (`# ____TEST CHAT____`):
- [ ] Every client case of spec §8; `clean` uses the same examples as the Go `textutil` tests (copy the table).
- [ ] Commit: `Add the chat log model and the client-side block list`.

### Task 9: Client — chat UI

**Files:** `ui/chat_line.gd` (new), `ui/chat_window.gd` (new), `ui/hud.gd`, `ui/player_card.gd`, `screens/world.gd`.

**Behaviour (§6.2):** HUD line (two lines, fade after 8 s, private unread dot); window with five tabs, colours, input, channel picker on Chung, "Tới" field on Riêng, Gửi disabled while empty/too long/cooling down, error under the input; tap-a-name menu (Nhắn riêng, Xem, Chặn/Bỏ chặn); player card "Nhắn riêng"; party notices also go to `add_local`. `world.gd` only wires: `ChatMessage` → `Router.chat.add`, send → `ChatSendReq` via `Net`, error → window. Keyboard focus in the input stops WASD movement.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with `tools/run2.sh`: zone, private, party, world cooldown, block. Screenshots in the report.
- [ ] Commit: `Show the chat line and the chat window`.

### Task 10: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3):** after the 5b party step, send a ZONE line and wait for the echo and `ChatSendResp`; `smoke.sh` greps the partner output for the `CHAT zone <name> <text>` line. Print `SMOKE OK (… chat echo)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: chat channels, limits, block list.
- [ ] Commit: `Extend the smoke test with a zone chat line`.

---

## Acceptance (controller, after Task 10)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID).
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
