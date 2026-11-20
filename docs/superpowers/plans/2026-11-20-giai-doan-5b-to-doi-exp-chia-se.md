# Phase 5b Parties and Shared EXP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parties of up to five. Tap a player to open a player card and invite them; the invitee accepts or declines a 30 s invite; the leader can remove members; anyone can leave; logging out leaves. A party frame lists the members with HP when they are in view, otherwise their map and khu, and a "Sang khu N" button. Every time a member is paid EXP by a hit, each teammate in the same zone within ±10 levels of the monster gets 5% of it on top. Teammates cannot hit each other, even with PK on.

**Architecture:**
- New package `internal/party`: one `Service` behind one mutex, in memory only. It registers every character that enters the world (entity id, name, class, level, map, khu, outbox) and hands back a per-session party-id cell (`*atomic.Uint64`). It answers invite/answer/leave/kick and pushes `PartyInvite`, `PartyState`, `PartyNotice` straight into members' outboxes. It never calls the world or a zone.
- The world registers at `Enter`, unregisters in `Membership.Leave`, reports `Moved` after a successful `Transfer`, and wires a `Leveled` hook into each zone's config.
- The zone only reads the cell: the shared-EXP bonus in `payExpShare` and the same-party flag of the 5a PK check. The cell rides the handoff.
- Pure bonus maths in `internal/combat` (`PartyBonus`, carry kept per player).
- Protocol 13: messages 108–118, `PartyMember`, `PartyNoticeKind`, six error codes.
- The client keeps the party model in `Router` across zone reloads and adds the party frame, the player card, the invite popup, notices and green names. The smoke invites a `botclient partner`.

**Tech stack:** Go, protobuf (buf); Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md](../specs/2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 5a plan](2026-11-20-giai-doan-5a-pk-phat-exp.md) is fully implemented (protocol 12, `combat.PvPAllowed` with its `sameParty` argument, `checkPlayerTarget`, mixed `victims`). Start from a server HEAD that contains it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 886c2a9 (phase 3a in progress) plus the phase 3–5a specs; client HEAD 5d275f7)

Phases 3–5a land before this plan and move most of the lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Concurrency**
- The gateway `outbox.Send` (`internal/gateway/outbox.go:24`) takes its own mutex and never blocks (false when full or closed), so the party service may push into any member's outbox from any goroutine while holding its lock. Lock order is party → outbox; the outbox never calls back. Zone outboxes in tests (`zone_test.go` fakes) must be made goroutine-safe if a test pushes from the party service and the zone at once.
- `zone.Outbox` is the interface the world already hands to zones; reuse it as `party.Outbox` (or a one-method interface with the same `Send`) so the gateway passes one value.
- Zone state is touched only by the zone goroutine (CLAUDE.md). The party-id cell is party-service state shared through `sync/atomic`; the zone only `Load`s it. Never add a zone inbox command for party changes.

**World and gateway**
- `World.Enter` (`internal/world/world.go:245`) builds `PlayerInfo` and joins with retries; `Membership` (`:202`) holds `z`, `EntityID`, `MapID`, `ZoneNo`; `Membership.Leave` (`:241`) is what `conn.cleanup` (`internal/gateway/conn.go:161`) and `handleLeaveWorld` (`handlers.go:263`) call, so unregistering there covers logout, disconnect, kicks and shutdown. 4a's `World.Transfer` returns a new `*Membership`; the gateway swaps `c.member`. The party registration must follow the session, not the membership: carry the registration (or just the entity id, which 4a keeps across transfers) into the new `Membership`.
- `World.Takeover` (`:442`) waits for the old session's `Done`, which closes after `cleanup`, so a re-login always finds the old registration gone.
- `zone.Config` (`internal/zone/zone.go:38`) is per zone and built by the world; add the `Leveled func(entityID uint64, level int)` hook there (nil-safe).

**Zone EXP**
- `payExpShare` (`internal/zone/progression.go:48`) pays `gain` to the attacker and calls `gainExp` (`:63`), which handles level-ups (and a dead player). The party bonus goes right after the attacker's `gainExp`, only when `gain > 0`.
- 5a's `checkPlayerTarget` and `victims` call `combat.PvPAllowed(…, false)`; pass the real same-party flag here.

**Bot client**
- `cmd/botclient/main.go` dispatches subcommands from a map (`cmds`, around `:48`); `cmdPlay` (`:192`) and `cmdFight` (`:273`) show the register → create → enter pattern for a fresh account.

**Client**
- 4a's `Router` keeps state across World reloads (selected bar slot); keep the party model there. `ui/hud.gd` top-left self card (`_build_self_card`) is where the party frame hangs; the top-centre target card (`_build_target`) gets a tap for the player card; notices use `show_notice`.

## File structure

Server (`DemonSlayer2D`):

```
internal/combat/combat.go, combat_test.go                   PartyBonus (Task 1)
internal/party/party.go (new), party_test.go (new)          party service (Task 1)
proto/ds/v1/protocol.proto, internal/pb/                    protocol 13 (Task 2)
internal/gateway/server.go, gateway_test.go, testdata/client/ version, fixtures (Task 2)
internal/world/world.go, world_test.go                      register, unregister, Moved, Leveled hook (Task 3)
internal/zone/zone.go, transfer.go, progression.go          cell in PlayerInfo and handoff, Leveled call, bonus (Tasks 3, 4)
internal/zone/combat.go, skill.go, combat_test.go           same-party PK filter (Task 4)
internal/gateway/handlers.go, internal/client/client.go     party requests (Task 5)
cmd/botclient/main.go                                       partner command (Task 5)
e2e/e2e_test.go                                             party e2e (Task 5)
docs/game-design.md, CLAUDE.md, README.md, phase 0 spec     docs (Task 6)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/protocol.gd, tests/fixtures/     sync (Task 7)
core/wire.gd, core/errors.gd                                protocol 13 (Task 7)
core/party.gd (new), core/pk_rules.gd, autoload/router.gd   party model, same-party filter (Task 8)
ui/party_frame.gd (new), ui/player_card.gd (new),
ui/invite_popup.gd (new), ui/hud.gd, ui/entity_view.gd       party UI (Task 9)
screens/world.gd, screens/world_entities.gd                 wiring (Task 9)
tests/unit/test_core.gd                                     tests (Tasks 7, 8)
tests/smoke/smoke.gd, tools/smoke.sh, README.md             smoke (Task 10)
```

---

### Task 1: Server — PartyBonus and the party service

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, `internal/party/party.go` (new), `internal/party/party_test.go` (new).

**Interface:**
- `combat.PartyBonusPct = 5`; `func PartyBonus(carry, gain int64) (bonus, newCarry int64)` — `total = carry + gain × PartyBonusPct`, `bonus = total / 100`, `newCarry = total % 100` (§4.6).
- `party.MaxMembers = 5`, `party.InviteTTL = 30 * time.Second`.
- `type Outbox interface { Send(*pb.Envelope) bool }`.
- `type Info struct { ID uint64; Name string; Class pb.CharClass; Level int; MapID string; Zone int }`.
- `func New(now func() time.Time) *Service`.
- `func (s *Service) Register(info Info, out Outbox) *atomic.Uint64` — a fresh cell at 0; registering an id twice replaces the old entry (it cannot happen with `Takeover`, but must not leak).
- `func (s *Service) Unregister(id uint64)` — leave as §4.4 without sending to the leaver; drop invites from and to it.
- `func (s *Service) Invite(from, to uint64) pb.ErrorCode`, `Answer(to, from uint64, accept bool) pb.ErrorCode`, `Leave(id uint64) pb.ErrorCode`, `Kick(leader, member uint64) pb.ErrorCode` — `UNSPECIFIED` on success; pushes happen inside.
- `func (s *Service) Moved(id uint64, mapID string, zone int)`, `func (s *Service) Leveled(id uint64, level int)` — update the entry; push `PartyState` to the party when in one.
- Internal: `type party struct { id uint64; leader uint64; members []uint64 /* join order */ }`; `type invite struct { from uint64; until time.Time }` keyed by invitee; `func (s *Service) stateOf(p *party) *pb.PartyState`; `func (s *Service) push(id uint64, env *pb.Envelope)`.

**Behaviour:** exactly §4.1–§4.5, error checks in the listed order. Expiry is checked lazily against `now()` (invite and answer). Every membership change writes the members' cells (party id or 0) before pushing. `PartyState` lists members in join order with their current `Info`; a leaver, kicked member or last member of a disbanded party gets an empty `PartyState`. `PartyInvite.expires_ms` is `InviteTTL` in ms.

**TODO / tests:**
- [ ] `// ____TEST PARTY BONUS____` (combat): §1.3 example (1, 2, 1, 2 from 0 → 0 EXP and carry 30; 143 from 30 → 7 and 45); over random gains the summed bonus equals `floor(5 × Σgain / 100)`.
- [ ] `// ____TEST INVITE____`: errors in order (self, unknown id → `INVALID_TARGET`; member not leader → `NOT_PARTY_LEADER`; full → `PARTY_FULL`; target in a party → `ALREADY_IN_PARTY`; pending → `INVITE_PENDING`); success pushes `PartyInvite` with name, level, class and 30000 ms; an expired invite (clock + 31 s) no longer blocks a new one.
- [ ] `// ____TEST ANSWER____`: no invite → `INVITE_NOT_FOUND`; decline pushes `DECLINED` to the inviter and removes the invite; accept creates a party led by the inviter, both cells set, both get a two-member `PartyState`; third member: old members get `JOINED`; accept after expiry → `INVITE_NOT_FOUND`; accept when the invitee has since joined a party → `ALREADY_IN_PARTY`; inviter now a non-leader member → `INVITE_NOT_FOUND`; inviter offline → `INVITE_NOT_FOUND`; party filled meanwhile → `PARTY_FULL`; the invite is gone after any answer.
- [ ] `// ____TEST LEAVE AND KICK____`: leave when not in a party → `NOT_IN_PARTY`; leader leaves a party of three → the earliest remaining member leads, `LEFT` and `NEW_LEADER` to the two, the leaver gets an empty state and cell 0; leaving a party of two disbands it (`DISBANDED` and empty state to the last one); kick errors in order (`NOT_IN_PARTY`, `NOT_PARTY_LEADER`, self or non-member → `BAD_REQUEST`); a kick sends `YOU_WERE_KICKED` and an empty state to the kicked one and `KICKED` to the rest.
- [ ] `// ____TEST REGISTRY____`: `Unregister` of a leader hands leadership on and sends nothing to the leaver; it drops invites from and to it; `Moved` and `Leveled` push a fresh `PartyState` to every member and nothing when not in a party.
- [ ] Run the package tests with `-race` while several goroutines invite, answer and leave at once (no data race, cells consistent at the end).
- [ ] Commit: `Add the party service and the shared EXP bonus formula`.

### Task 2: Server — protocol 13

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 108–118; `PartyMember`; `PartyNoticeKind`; error codes 47–52 with one comment line each; comments on `INVALID_TARGET` and `PK_FORBIDDEN`. `ProtocolVersion = 13`. Requests are not handled yet (default `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe (no `message`/`class` fields; enum values lose their prefix in godobuf, check `YOU_WERE_KICKED` and `NEW_LEADER` read well).
- [ ] Fixtures `party_invite`, `party_state` (three members on two maps), `party_notice`, `party_answer_req`; regenerate all with `-update`.
- [ ] Version test → 13.
- [ ] Commit: `Add the phase 5b party messages and bump the protocol to 13`.

### Task 3: Server — world and zone wiring

**Files:** `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/zone.go`, `internal/zone/transfer.go`, `internal/zone/progression.go`.

**Interface:**
- `World` owns a `*party.Service` (`party.New(time.Now)`); `func (w *World) Party() *party.Service`.
- `PlayerInfo.Party *atomic.Uint64` — nil in old tests means "no party"; `player.party` keeps it; `func (p *player) partyID() uint64` (0 when nil).
- `zone.Config.Leveled func(entityID uint64, level int)` — called by `gainExp` when the level changes; nil-safe.

**Behaviour:**
- `Enter`: once the entity id and the reserved zone are known and before the join, `Register(Info{…}, req.Out)` and put the cell into `PlayerInfo`. If a retry joins another zone, `Moved` with the final zone; if the join fails for good, `Unregister`.
- `Membership.Leave`: `Unregister(EntityID)` first, then the zone leave as today.
- `Transfer` success: `Moved(EntityID, newMap, newZone)`. The handoff copies the cell pointer (not its value); the new `Membership` keeps whatever `Leave` needs to unregister.
- Each zone's config gets `Leveled: w.party.Leveled`.

**TODO / tests** (`// ____TEST PARTY WIRING____` in world):
- [ ] `Enter` registers: inviting the entered entity id succeeds; `Leave` unregisters: a teammate gets `LEFT` or `DISBANDED`.
- [ ] A portal transfer pushes `PartyState` with the new map and zone to a teammate; the cell in the new zone is the same pointer (setting it via the service is seen by the new zone).
- [ ] A level-up in the zone pushes `PartyState` with the new level.
- [ ] A failed join (full world in the test config) leaves nothing registered.
- [ ] Commit: `Register players with the party service and report moves and levels`.

### Task 4: Server — shared EXP and the same-party filter

**Files:** `internal/zone/progression.go`, `internal/zone/combat.go`, `internal/zone/skill.go`, `internal/zone/combat_test.go`.

**Interface:** `player.partyCarry int64`; `func (z *Zone) payPartyBonus(a *player, m *monster, gain int64)`; `func sameParty(a, b *player) bool` (`a.partyID() != 0 && a.partyID() == b.partyID()`).

**Behaviour (§4.6, §4.7):** after `payExpShare` pays `a` a positive `gain`, `payPartyBonus` loops over `z.players` (sorted ids, for deterministic tests) and pays each teammate `t ≠ a` with `combat.ExpEligible(t.level, m.level)` the `PartyBonus` of its carry; `gainExp(t, bonus)`. The bonus never touches `m.dmgBy` or `m.expPaid`. `checkPlayerTarget` and `victims` pass `sameParty(a, b)` to `combat.PvPAllowed`.

**TODO / tests** (`// ____TEST PARTY EXP____`, `// ____TEST PARTY PK____`; set cells by hand in zone tests):
- [ ] A kills a monster alone; teammate T in the zone, never attacking, gets exactly `floor(5 × Exp / 100)`; A gets the full `Exp`.
- [ ] T 11 levels away from the monster gets nothing; a player in another party and a player with no party get nothing.
- [ ] Dead T still gets the bonus and stays at 0 HP.
- [ ] A outside the ±10 window (gain 0): T gets nothing.
- [ ] A and T both hit: each gets its own share plus 5% of the other's.
- [ ] Two teammates, both PK on: slash → `PK_FORBIDDEN`; `single` skill → `PK_FORBIDDEN`; `circle_self` skips the teammate and hits a PK-on player of another party.
- [ ] Commit: `Pay teammates 5% of each hit's EXP and stop teammates hitting each other`.

### Task 5: Server — gateway, client helpers, botclient partner and the party e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `cmd/botclient/main.go`, `e2e/e2e_test.go`.

**Interface:**
- Gateway cases `PartyInviteReq`, `PartyAnswerReq`, `PartyLeaveReq`, `PartyKickReq`: need `inWorld`, packet limit only; call `World.Party()` with `c.member.EntityID`; `UNSPECIFIED` → the matching empty response, else `fail(seq, code)`.
- `client.PartyInvite(ctx, target)`, `client.PartyAnswer(ctx, from, accept)`, `client.PartyLeave(ctx)`, `client.PartyKick(ctx, member)`.
- `botclient partner [-for 60s] [-url …]` per spec §7.

**TODO / tests:**
- [ ] Gateway: the four requests before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestPartyOverWebSocket` as spec §8. For the EXP step reuse the e2e fight helper (walk to the nearest monster and slash until it dies); read the monster's EXP from `gamedata` (`Levels[level].Exp`, level from its `EntityEnter`) and assert B's `SelfStats.exp` rose by exactly `5 × Exp / 100`. Before entering, `DevSetPlace` puts A and B side by side just east of the camp (the 5a e2e spot; the PK step needs both outside safe zones) and C at the `default` spawn. B stands still and never attacks; a monster hitting or even killing B does not matter (dead teammates still get the bonus). A and B start at level 1 so both are inside the ±10 window of levels 1–5.
- [ ] `botclient partner` against a local server: prints `PARTNER <name>`, accepts an invite sent with `client.PartyInvite` from a test account, prints `PARTY OK`.
- [ ] Commit: `Handle party requests, add botclient partner and an end-to-end party test`.

### Task 6: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO:**
- [ ] game-design 5.8 rewritten per spec §9 (the new 5% rule and its ±10 window; drop the old example lines; keep loot rights), 2.2 note, 5.7 teammates.
- [ ] Phase 0 roadmap: phase 5 done with 5b.
- [ ] CLAUDE.md invariants per §9.
- [ ] README: parties, `botclient partner`.
- [ ] Commit: `Document parties and shared EXP`.

### Task 7: Client — sync and protocol 13

**Files:** `server` submodule, `proto/protocol.gd`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 6; `tools/sync_server.sh`; `PROTOCOL_VERSION := 13`; `PAYLOADS` adds the eleven names; the six error texts of spec §6.1.

**TODO / tests:**
- [ ] Decode the four new fixtures (`PartyState` members and leader, `PartyNotice` kind).
- [ ] Commit: `Sync the phase 5b protocol from the server`.

### Task 8: Client — party model and the same-party filter

**Files:** `core/party.gd` (new), `core/pk_rules.gd`, `autoload/router.gd`, `screens/world_entities.gd`, `tests/unit/test_core.gd`.

**Interface (`core/party.gd`):** `func apply_state(s) -> void`; `var members: Array` (`{id, name, char_class, level, map_id, zone}` in order); `var leader_id: int`; `func in_party() -> bool`; `func is_member(id: int) -> bool`; `func is_leader(id: int) -> bool`; `func others(self_id: int) -> Array`; `func can_invite(self_id: int, target_id: int) -> bool`; `func set_invite(msg, now: float) -> void`; `func invite_left(now: float) -> float` (0 when none or expired); `func clear_invite() -> void`; `static func notice_text(kind: int, name: String) -> String`; `func clear() -> void`.

**Behaviour:** `Router.party` is created once and cleared when going back to character select; `world_entities.candidates(ctx)` passes `same_party = ctx.party.is_member(id)` to `pk_rules.can_attack`.

**TODO / tests** (`# ____TEST PARTY____`):
- [ ] `apply_state` with three members then an empty state; `is_member`, `is_leader`, `others` order.
- [ ] `can_invite`: alone → true; leader of 4 → true; leader of 5 → false; member not leader → false; target already a member → false; self → false.
- [ ] Invite countdown: `invite_left` falls to 0 at 30 s; a new invite replaces the old.
- [ ] `notice_text` for all seven kinds matches spec §6.2.
- [ ] `can_attack` with `same_party` true is false even when both PK on.
- [ ] Commit: `Add the party model and keep teammates out of the target list`.

### Task 9: Client — party UI

**Files:** `ui/party_frame.gd` (new), `ui/player_card.gd` (new), `ui/invite_popup.gd` (new), `ui/hud.gd`, `ui/entity_view.gd`, `screens/world.gd`.

**Behaviour (spec §6.2):**
- `party_frame.gd`: under the self card; hidden when not in a party; header "Tổ đội n/5" with **Rời** (`PartyLeaveReq`); one row per other member (class-colour dot, name, "Lv n", ★ for the leader); HP bar from the entity table when the member is in view, else "<map name> · Khu n" (`MapGrid.name_of`); "Sang khu n" when on the same map in another zone (goes through 4a's `_transfer` with `ChangeZoneReq`); the leader tapping a row shows **Mời ra** (`PartyKickReq`). Refreshes on `PartyState` and on entity HP/enter/leave effects.
- `player_card.gd`: opened by `world.gd` when a tap lands on a player that `pick_at` did not lock (find it with the same radius over all live players), or when the target card is tapped while the target is a player; shows name, "Lv n", class name, "Đang bật PK" when on; **Mời tổ đội** only when `can_invite`; closes when the player leaves view.
- `invite_popup.gd`: shown on `PartyInvite`; text, countdown from `invite_left`, **Đồng ý**/**Từ chối** (`PartyAnswerReq{from_id, accept}`); hides at 0 or on answer.
- `world.gd`: routes `PartyState` (model + frame + name colours), `PartyNotice` (`show_notice(notice_text…)`), `PartyInvite` (popup); after a transfer the frame is rebuilt from `Router.party`.
- `entity_view.gd`: teammates' names green, over the PK red.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with two windows (`tools/run2.sh`): invite in Ubuyashiki by tapping, accept, both frames show; walk through a portal and see the map update in the other window; switch khu and use "Sang khu n"; kill monsters and see the teammate's EXP rise; teammates cannot attack each other with PK on; leave → disbanded notice. Screenshots in the report.
- [ ] Commit: `Add the party frame, player card and invite popup`.

### Task 10: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (spec §6.3):** `smoke.sh` builds `server/cmd/botclient` next to `devchar`, starts `botclient partner -url "$DS_SERVER_URL" -for 120s` in the background, and writes the name from its `PARTNER <name>` line to `$dir/partner` (moved into place whole); kill it at the end. `smoke.gd` after the 5a step: wait for `$dir/partner`, wait for a player `EntityEnter` with that name, `PartyInviteReq`, wait for a two-member `PartyState` led by itself, `PartyLeaveReq`, wait for an empty `PartyState`. Print `SMOKE OK (… party 2/5)`. Raise the watchdog if the extra steps need it.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: inviting, the party frame, leaving, shared EXP.
- [ ] Commit: `Extend the smoke test with a party invite`.

---

## Acceptance (controller, after Task 10)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID).
- [ ] Tell the user how to open two windows on :8199 and what to try (spec §8 "Nghiệm thu").
