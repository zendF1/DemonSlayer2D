# Phase 7c Guilds Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Guild Manager NPC in Ubuyashiki. Characters from level 20 found a guild for free; leaders and officers invite online players from the player card (30 s invites); three ranks, notice, leave/kick with a 24 h rejoin cooldown, leadership transfer, disband with name confirmation. Members donate Xu at the NPC (contribution = Xu / 100); the leader spends the fund to raise the guild level (member cap 20 → 40). The guild name shows under the character name, guild mates cannot attack each other anywhere, and chat gets a Guild channel.

**Architecture:**
- Storage: migration 00011 (`guilds`, `guild_members`, `characters.guild_left_at`); store functions for the guild service; a new `store.Op` kind `GuildDonate` run by the saver inside the donor's character transaction (7b infrastructure).
- A world-level `internal/guild` service (5b pattern): loads every guild at start, in-memory truth under a data lock, DB writes on the caller's connection goroutine serialised by an op lock (DB first, then memory); per-session `*atomic.Pointer[guild.Tag]` cell that zones only read.
- Zone: the `GuildDonate` command, a read-only `NearNPC` query, `guild_name` in `EntityEnter`/`LookChange`, `PvPAllowed` same group = same party or same guild.
- Chat (7a): `CHAT_CHANNEL_GUILD` routed through `guild.OnlineMembers`.
- Protocol 18: envelopes 160–184, errors 73–85, `EnterWorldResp.guild = 15`, `.guild_rejoin_ms = 16`, `EntityEnter.guild_name = 17`, `LookChange.guild_name = 5`.
- Dev: `cmd/devguild`. Client: `core/guild.gd` (`Router.guild`), guild window, NPC role `guild`, invite popup, name line, Guild chat tab.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3 map script; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-7c-bang-hoi-design.md](../specs/2026-11-20-giai-doan-7c-bang-hoi-design.md). Section references below (§n) point to it. Shared rules: [phase 7 overview](../specs/2026-11-20-giai-doan-7-tong-quan.md) §3.

**Prerequisite:** the [phase 7b plan](2026-11-20-giai-doan-7b-giao-dich.md) is fully implemented (`store.Op`, `Saver.SaveLinked`, `IN_TRADE`; protocol 17, migration 00010) and so is 7a (chat service, `ChatChannel`). Start from HEADs that contain them.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 618ca8f = phase 3a; phases 3b–7b are designed, not yet implemented)

Phases 3b–7b land before this plan and move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

- `internal/store/characters.go`: `charColumns` (`:91`) is the shared select list of `ListCharacters`/`GetCharacter`; `DeleteCharacter` (`:223`). `guild_left_at` must **not** enter `charColumns` or the `SaveCharacterState` update — only the guild service writes it.
- `internal/gateway/handlers.go`: `handleDeleteChar` (`:252`) checks `confirm_name` with `norm.NFC` then calls `DeleteCharacter`; the `GUILD_LEADER` check goes between the two, and `guild.CharacterDeleted` after.
- Name rules: `internal/auth` already validates and keys character names (NFC, letters and digits, lower-case key); reuse its helpers with a 2–12 length for guild names instead of re-implementing.
- 5b design: `internal/party` is the model for registration (`World.Enter`/`Membership.Leave`/`Moved`), invites with TTL checked on answer, pushes into the session outbox, and the atomic per-session cell carried by the handoff. 5b's `zone.Config.Leveled` is one function; world wraps it to call both party and guild.
- 5a design: `combat.PvPAllowed(attackerPK, targetPK, sameParty bool)`; rename the third parameter `sameGroup` and compute it in the zone from both cells.
- 4b design: NPC role validation lives in `mapdata.Parse`; NPC range `npcRange` + slack in the zone. 7b design: the saver appends `CharState.Ops` across coalesced saves; `store.SaveCharacterState` runs ops after the character rows in the same transaction.

## File structure

Server (`DemonSlayer2D`):

```
migrations/00011_guilds.sql (new), internal/store/guilds.go (new), internal/store/ops.go, store_test.go   storage (Task 1)
internal/guild/guild.go (new), guild_test.go (new)                                                      service (Task 2)
proto/ds/v1/protocol.proto, internal/pb/, internal/gateway/server.go, testdata/client/*.hex             protocol 18 (Task 3)
tools/mapgen.py, maps/ubuyashiki.json (gen), internal/mapdata/*                                         NPC (Task 4)
internal/zone/guild.go (new), zone.go, combat.go, skill.go, transfer.go, internal/combat/pvp.go,
internal/world/world.go, tests                                                                         zone and world (Task 5)
internal/gateway/handlers.go, internal/chat/chat.go, internal/client/client.go, e2e/e2e_test.go         wiring, chat, e2e (Task 6)
cmd/devguild (new), docs/game-design.md, CLAUDE.md, README.md, phase 0 spec                              dev tool, docs (Task 7)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/maps/, tests/fixtures/                       sync (Task 8)
core/wire.gd, core/errors.gd, core/guild.gd (new), core/pk_rules.gd,
core/entity_table.gd, autoload/router.gd                                       model (Task 8)
ui/guild_window.gd (new), ui/npc_window.gd, ui/player_card.gd,
ui/invite_popup.gd, ui/entity_view.gd, ui/chat_window.gd, ui/hud.gd,
screens/world.gd                                                               UI (Task 9)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md       tests, smoke (Tasks 8–10)
```

---

### Task 1: Server — guild storage and the donate op

**Files:** `migrations/00011_guilds.sql` (new), `internal/store/guilds.go` (new), `internal/store/ops.go` (7b), `internal/store/store_test.go`.

**Interface:**
- `store.GuildRow{ID int64; Name, NameKey, Notice string; Level int; Fund int64}`; `store.GuildMemberRow{GuildID, CharID int64; Name, Class string; Level int; MapID string; Rank int16; Contribution int64; JoinedAt time.Time}`.
- `LoadGuilds`, `LoadGuildCooldowns`, `CreateGuild`, `AddGuildMember`, `RemoveGuildMember`, `SetGuildRanks`, `SetGuildNotice`, `DisbandGuild`, `UpgradeGuild`, `AddGuildFund` with the contracts of §3; `ErrGuildNameTaken`.
- `store.GuildDonate{GuildID, CharID, Xu, Contribution int64}` implements the 7b `Op` interface; no matching row is not an error.

**TODO / tests** (`// ____TEST GUILDS____`, DB):
- [ ] Migration up/down; the partial unique index rejects a second leader.
- [ ] Create → load (leader, level 1, fund 0); duplicate key → `ErrGuildNameTaken`; add/remove member (`guild_left_at` set only when given); `SetGuildRanks` swaps leader and officer in one transaction; disband removes members and sets the leader's `guild_left_at`.
- [ ] `SaveCharacterState` with a `GuildDonate` op: Xu, fund and contribution change together; with the guild deleted the save still succeeds; `guild_left_at` survives a character save.
- [ ] `UpgradeGuild` false on low fund and on a stale level; `AddGuildFund` adds.
- [ ] Commit: `Store guilds, members and the guild donate op`.

### Task 2: Server — the guild service

**Files:** `internal/guild/guild.go` (new), `internal/guild/guild_test.go` (new).

**Interface:**
- Constants of §2.2; `type Rank int8` (`Leader = 1`, `Officer`, `Member`); `type Tag struct{ ID uint64; Name string }`.
- `type Store interface` (the store functions of Task 1) and `type Outbox interface{ Send(*pb.Envelope) }`.
- `func New(st Store, now func() time.Time) *Service`; `Load(ctx) error`.
- `Register(Session) *atomic.Pointer[Tag]`, `Unregister(entityID uint64)`, `Moved(entityID uint64, mapID string, zone int)`, `Leveled(entityID uint64, level int)`; `StateFor(charID) *pb.GuildState`, `RejoinIn(charID) time.Duration` (for `EnterWorldResp`).
- Commands (each returns `(resp proto.Message, code pb.ErrorCode)`): `Create(ctx, s SessionRef, level int, name string)`, `Invite`, `Answer`, `Leave`, `Kick`, `SetRank`, `SetNotice`, `Disband`, `Upgrade(ctx, s, …)`, `Info(s)`.
- Lock-only readers: `Donated(guildID uint64, charID, xu, c int64) (fund, contribution int64, ok bool)`, `MemberInfo(charID int64) (uint64, Rank, time.Time, bool)`, `OnlineMembers(guildID uint64) []Outbox`, `IsLeader(charID int64) bool`, `Exists(guildID uint64) bool`, `AddFundCached(guildID uint64, xu int64)` (memory only, for a fund delta another writer saves as an op, e.g. 7h's stall tax); `CharacterDeleted(charID int64)`; `AddFund(ctx, guildID uint64, xu int64) error`.

**Behaviour:** §4.1–4.6, §4.10. Check under the data lock, then take the op lock, re-check, write DB, mutate memory, set cells, push. `Donated` only flags the guild dirty; a service goroutine (started by `Run(ctx)`) sends the pending `GuildState` pushes so the zone never builds them.

**TODO / tests** (`// ____TEST GUILD COMMANDS____`, `// ____TEST GUILD RANKS____`, `// ____TEST GUILD FUND____`, fake store, fake clock, fake outboxes):
- [ ] Every error order of §8 "guild"; the full permission table of §4.2.
- [ ] Cooldown boundary (23 h 59 m rejected, 24 h accepted); disband sets the cooldown only for the leader; transfer with three officers demotes the old leader to member.
- [ ] Cap: the 21st member at level 1 → `GUILD_FULL` on invite and on answer; after an upgrade both pass.
- [ ] Invite expiry at 30 s; a store error leaves memory unchanged; cells correct after join/leave/kick/disband.
- [ ] `Donated`, `MemberInfo`, `AddFund`, `Register`/`Unregister` online pushes, `CharacterDeleted`; upgrade with store false → `GUILD_FUND_LOW`, fund unchanged; memory fund after upgrade = old − cost.
- [ ] Commit: `Add the guild service with ranks, invites and the fund`.

### Task 3: Server — protocol 18

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 160–184; `GuildRank`, `GuildMember`, `GuildState`, `GuildEventKind`; `EnterWorldResp.guild = 15`, `.guild_rejoin_ms = 16`; `EntityEnter.guild_name = 17`; `LookChange.guild_name = 5`; `CHAT_CHANNEL_GUILD = 6`; errors 73–85 with one-line comments, 86–88 unused; `PK_FORBIDDEN` comment. `ProtocolVersion = 18`. Guild requests not handled yet (`BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe (no `message`/`class` fields).
- [ ] Fixtures of §5; regenerate all with `-update`; version test → 18.
- [ ] Commit: `Add the phase 7c guild messages and bump the protocol to 18`.

### Task 4: Server — the Guild Manager NPC

**Files:** `tools/mapgen.py`, `maps/ubuyashiki.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`.

**Behaviour (§2.1):** role `guild`, no `shop`/`routes`, only on `town` maps; `quan_ly_bang` placed by the 4b rules.

**TODO / tests** (`// ____TEST NPCS____`):
- [ ] `--check` clean; role accepted on a town map, rejected on a training map; the real Ubuyashiki has it.
- [ ] Commit: `Add the Guild Manager NPC to Ubuyashiki`.

### Task 5: Server — guilds in the zone and the world

**Files:** `internal/zone/guild.go` (new), `internal/zone/zone.go`, `internal/zone/combat.go`, `internal/zone/skill.go`, `internal/zone/transfer.go`, `internal/combat/pvp.go`, `internal/world/world.go`, `internal/zone/zone_test.go`, `internal/world/world_test.go`, `internal/combat/combat_test.go`.

**Interface:**
- `PlayerInfo.Guild *atomic.Pointer[guild.Tag]`; `player.guild`, `player.lastTag`.
- `func (z *Zone) GuildDonate(id uint64, seq uint32, npc string, xu int64) bool`; `func (z *Zone) NearNPC(id uint64, npc, role string, reply chan<- NPCCheck) bool` with `NPCCheck{Level int; Code pb.ErrorCode}`.
- `zone.Config.Guilds` (the lock-only `Donated` function); `combat.PvPAllowed(attackerPK, targetPK, sameGroup bool) bool`.
- World: `Enter` registers with the guild service and fills `EnterWorldResp.guild`/`.guild_rejoin_ms`; `Leave` unregisters; `Transfer` calls `Moved`; `Leveled` fans out to party and guild.

**Behaviour:** §4.6–4.8. The donate save is `z.saver.Save(z.state(p) + GuildDonate op, nil)`. The tag compare runs once per tick before `syncView`.

**TODO / tests** (`// ____TEST GUILDS____` in zone, `// ____TEST ZONES AND TRANSFER____` in world):
- [ ] Donate error order (DEAD, BAD_REQUEST, NPC_TOO_FAR, IN_TRADE, NOT_IN_GUILD, BAD_REQUEST for 1,500 and 500, NOT_ENOUGH_XU, NOT_IN_GUILD when `Donated` says no); success: Xu, the queued op and the reply.
- [ ] Tag change → one `LookChange` with `guild_name` for viewers; `EntityEnter.guild_name`.
- [ ] Same-guild slash/single skill → `PK_FORBIDDEN`; circle skill skips the guild mate and hits another guild's PK player.
- [ ] `NearNPC` codes and level.
- [ ] World: cell set on enter; after a portal guild mates still cannot hit each other; `Leveled` reaches the guild service.
- [ ] Commit: `Show guild names, block guild PvP and take donations in the zone`.

### Task 6: Server — gateway, guild chat, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/chat/chat.go`, `internal/chat/chat_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** gateway cases for the 13 guild requests (§4.11; create and upgrade call `Membership.NearNPC` first); `Membership.GuildDonate`, `Membership.NearNPC`; chat `GUILD` channel via `guild.OnlineMembers` (§4.9); `client.Guild*` helpers recording `GuildState`/`GuildEvent`/`GuildInvite` pushes.

**TODO / tests:**
- [ ] Gateway: guild requests before entering → `NOT_IN_WORLD`; deleting a guild leader → `GUILD_LEADER`, deleting a member removes them from the service.
- [ ] Chat: `GUILD` without a guild → `NOT_IN_GUILD`; delivery only to online members of that guild.
- [ ] e2e `TestGuildsOverWebSocket` per spec §8 (store helpers for level, Xu and place; the `quan_ly_bang` position from the generated map).
- [ ] Commit: `Handle guild requests, add guild chat and an end-to-end guild test`.

### Task 7: Server — devguild and documentation

**Files:** `cmd/devguild/main.go` (new), `cmd/devguild/main_test.go` (new), `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`.

**TODO / tests:**
- [ ] `devguild` flags of §7; unknown names and characters already in a guild write nothing (DB test).
- [ ] Docs per §9.
- [ ] Commit: `Add devguild and document the phase 7c guilds`.

### Task 8: Client — sync, protocol 18 and the guild model

**Files:** `server` submodule, `proto/protocol.gd`, `data/maps/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `core/guild.gd` (new), `core/pk_rules.gd`, `core/entity_table.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1.

**TODO / tests** (`# ____TEST GUILD____`):
- [ ] `PROTOCOL_VERSION := 18`; `PAYLOADS` adds the new payloads; error texts; decode the new fixtures.
- [ ] `guild.apply_state`, `can` over the permission table, invite countdown, `event_text` for all ten kinds, `valid_name` cases (1 and 13 characters, a space, Vietnamese letters).
- [ ] `pk_rules.same_group` by guild name; `entity_table` keeps `guild_name` from enter and look changes.
- [ ] Commit: `Sync the phase 7c protocol and add the guild model`.

### Task 9: Client — guild UI

**Files:** `ui/guild_window.gd` (new), `ui/npc_window.gd`, `ui/player_card.gd`, `ui/invite_popup.gd`, `ui/entity_view.gd`, `ui/chat_window.gd`, `ui/hud.gd`, `screens/world.gd`.

**Behaviour:** §6.2. `world.gd` only wires replies, pushes and errors into `Router.guild` and the windows.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with two clients (`tools/run2.sh`): create, invite, accept, name line, Guild chat, donate, kick. Screenshots in the report.
- [ ] Commit: `Add the guild window, guild invites and the Guild chat tab`.

### Task 10: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.3).** Print `SMOKE OK (… guild created, donated 1000)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: guilds, the Guild Manager, the Guild chat tab.
- [ ] Commit: `Extend the smoke test with a guild`.

---

## Acceptance (controller, after Task 10)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID); migration 00011 applies.
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
