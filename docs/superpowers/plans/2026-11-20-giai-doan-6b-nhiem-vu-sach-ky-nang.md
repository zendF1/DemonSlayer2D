# Phase 6b Main-Line Quests and Skill Books Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Quest NPCs in every camp and in Ubuyashiki with !/? markers. A 25-quest main line (22 chained quests for levels 1–20 across the five maps, plus milestone quests at 30/40/50) with kill, collect and talk objectives, EXP/Xu/item rewards, and the five skill books per class. Learning a skill for the first time now needs its book unless the character's level at migration already covered it. A tracker on the right lists the quests; tapping one walks there with the 6a travel.

**Architecture:**
- Data: `docs/balance/quest_data.py` writes `data/quests.json` (reads `data/exp.json` and `maps/*.json`); `item_stats.py` adds 25 `book_<skill id>` stackables; `mapgen.py` adds five NPCs with the new role `quest`. `gamedata` loads and cross-validates quests against maps, monsters, items and skills.
- Pure rules in a new `internal/quest` package (availability, kill/collect credit, rewards, book need, `Round10`).
- Store: migration 00009 (`character_quests`, `characters.skill_book_waiver` set to the current level). `Character`/`CharState`/`PlayerInfo`/handoff carry quests and the waiver.
- Zone: `QuestAccept`/`QuestTurnIn` commands, credit at the killer's loot site, a per-tick changed list flushed as `QuestProgress` in `syncView`; `learn` checks and consumes books.
- Protocol 14: envelopes 119–123, `QuestEntry`/`QuestLog`/`QuestState`, `EnterWorldResp.quests = 13`, `.skill_book_waiver = 14`, `LearnSkillResp.update = 3`, errors 53–56.
- Dev: `cmd/devquest`, `devchar -waive-books`.
- Client: `core/quest_book.gd` (in `Router.quests`), the `region` travel kind, book reason in `skill_book`, NPC markers, quest card, tracker.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3 data scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md](../specs/2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md). Section references below (§n) point to it.

**Prerequisite:** the [phase 6a plan](2026-11-20-giai-doan-6a-bam-di-tim-duong.md) is fully implemented (client `core/travel.gd`, `core/world_graph.gd`, `Router.travel`; server protocol 13, migration 00008). Start from HEADs that contain it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 83fee78 (phase 3a in progress) plus the phase 3–6a specs; client HEAD 5d275f7)

Phases 3–6a land before this plan and move most lines below. Find each symbol by name (`graphify explain "<symbol>"`) before editing.

**Data and scripts**
- `gamedata.Load(dir)` (`internal/gamedata/gamedata.go:139`) reads every data file of `data/`; `Parse` (`:91`) uses `DisallowUnknownFields`; `(*Data).Validate(maps)` (`:179`) is the cross-check hook with the maps (4b added NPC/shop checks there). Add `quests.json` to both.
- Data scripts live in `docs/balance/` (`skill_stats.py`, `item_stats.py`, `exp_table.py`, `monster_stats.py`) with `--write-json`/`--print-json`; Go tests compare the committed JSON with the script output. Maps come from `tools/mapgen.py` (4a). Follow the same pattern for `quest_data.py`, including a `--check`.
- `data/exp.json` `exp_to_next[L-1]` is the EXP from L to L+1; `quest_data.py` must read it (not recompute) so a later regeneration only needs a re-run. Current values give m01 = 20 EXP, m11 = 9,150, m22 = 206,550 (§2.2).
- Region ids per map come from `mapgen.py`; the script resolves `(map, monster, min_level)` to the region id by reading `maps/*.json` (`regions[].monster`, `min_level`).

**Items**
- `items.NewEquipment(rng, d, code, quality)` (`internal/items/roll.go:59`) with quality 0 builds a Trắng +0 piece with no lines and no element — use it for weapon rewards (`eq<tier>_weapon_<class>`; other slots `eq<tier>_<slot>`).
- `(*Inventory).Add` (`internal/items/bag.go:20`) adds one item and reports changed slots or an error; for an all-or-nothing reward batch, add every reward to `inv.Clone()` first and only then to the real bag (4b's buy check works the same way). Removing one book: find the lowest bag slot whose `code` matches and use `Discard(slot, 1)` (`:123`).

**Zone**
- The killer's loot runs at `z.loot(m, killer)` (`internal/zone/combat.go:223`); credit quests right beside it (`killer` may be dead; nil when it left the zone). The monster kind id is `m.region.Kind.ID`.
- `gainExp` (`internal/zone/progression.go:88`) handles level-ups (`LevelUp` in the tick, 5b's `Leveled` hook); `learn` (`:159`) is the learn command to extend with the book step.
- `syncView` sends `WorldTick`, then 5a's `DeathNotice`, then `SelfStats`; put `QuestProgress` between the notice and `SelfStats`.
- Zone commands: follow `learnCmd`/4b's `buyCmd` (inbox command + `Membership` method returning false on a full inbox → gateway `RATE_LIMITED`).

**Store**
- `charColumns` (`internal/store/characters.go:91`) is shared by `ListCharacters` and `GetCharacter`; `CharState` (`:72`) holds `Skills`/`Items` that replace every row on save — mirror that for quests. The next migration file is `00009_quests.sql`.

**Client**
- 4b's `ui/npc_window.gd` switches content by role; add the `quest` role there. 5b keeps per-session models in `autoload/router.gd` (`Router.party`); put `Router.quests` beside it. `core/skill_book.gd` `learn_block` returns `""`, `"maxed"`, `"level"`, `"points"` today.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/quest_data.py (new), data/quests.json (gen)          quest data (Task 1)
docs/balance/item_stats.py, data/items.json (gen)                 25 books (Task 1)
tools/mapgen.py, maps/*.json (gen), internal/mapdata/*            quest NPCs (Task 1)
internal/gamedata/quests.go (new), gamedata.go, gamedata_test.go  load, validate (Task 1)
internal/quest/quest.go (new), quest_test.go (new)                pure rules (Task 2)
migrations/00009_quests.sql (new), internal/store/*               storage (Task 3)
cmd/devquest (new), cmd/devchar                                   dev tools (Task 3)
proto/ds/v1/protocol.proto, internal/pb/, gateway fixtures        protocol 14 (Task 4)
internal/zone/quest.go (new), zone.go, combat.go, transfer.go     quest state, accept, turn-in, credit (Task 5)
internal/world/world.go                                           load checks (Task 5)
internal/zone/progression.go                                      skill books (Task 6)
internal/gateway/handlers.go, internal/client/client.go, e2e/     wiring, e2e (Task 7)
docs/game-design.md, CLAUDE.md, README.md, phase 2 spec           docs (Task 8)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), proto/, data/, tests/fixtures/, tools/sync_server.sh   sync (Task 9)
core/wire.gd, core/errors.gd                                                protocol 14 (Task 9)
core/quest_book.gd (new), core/travel.gd, core/map_grid.gd,
core/world_graph.gd, core/skill_book.gd, core/inventory.gd,
autoload/router.gd                                                          model (Task 10)
ui/map_view.gd, ui/npc_window.gd, ui/quest_card.gd (new),
ui/quest_tracker.gd (new), ui/skill_tab.gd, ui/hud.gd, screens/world.gd     UI (Task 11)
tests/unit/test_core.gd, tests/smoke/smoke.gd, tools/smoke.sh, README.md    tests, smoke (Tasks 9–12)
```

---

### Task 1: Server — quest data, books and quest NPCs

**Files:** `docs/balance/quest_data.py` (new), `data/quests.json` (generated), `docs/balance/item_stats.py`, `data/items.json` (generated), `tools/mapgen.py`, `maps/*.json` (generated), `internal/mapdata/mapdata.go`, `internal/mapdata/mapdata_test.go`, `internal/gamedata/quests.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `quest_data.py --write-json | --print-json | --check` (§2.1–2.3); prints the EXP share of m01–m22 and fails outside 15–30%.
- `mapdata`: role `quest` (no `shop`, no `routes`).
- `gamedata.Quest` (`ID`, `Line`, `Name`, `MinLevel`, `Requires`, `Giver`, `TurnIn` `NPCRef{Map, NPC}`, `Objective{Type, Monster, Count, ChanceBP, ItemName, Map, Region}`, `Text`, `Reward{Exp, Xu int64; Items []StackReward; Equipment []EquipReward; SkillBook int}`); `Data.Quests []Quest` in file order plus `Data.QuestByID map[string]*Quest`; `func (o Objective) Need() int` (1 for `talk`).

**Behaviour:** §2.4–2.6. Books are appended after the existing stackables (sort order of 3a follows file order). Validation errors name the quest id.

**TODO / tests:**
- [ ] Scripts: write `quests.json`, regenerate `items.json` and maps; `--check` clean for all three.
- [ ] `// ____TEST QUESTS____` (gamedata): real data loads (25 quests, 25 books); every rejection case of spec §8 "gamedata"; each skill has exactly one book.
- [ ] `// ____TEST NPCS____` (mapdata): `quest` role accepted, `quest` with `shop` rejected; the five real maps have their quest NPC.
- [ ] Commit: `Generate the main-line quests, skill books and quest NPCs`.

### Task 2: Server — pure quest rules

**Files:** `internal/quest/quest.go` (new), `internal/quest/quest_test.go` (new).

**Interface:**
- `type State int8` (`Active = 1`, `Done = 2`); `type Entry struct { State State; Progress int; Day int32 }`; `type Log map[string]Entry` with `Clone()`.
- `func Available(q *gamedata.Quest, log Log, level int) bool` — §4.1 (6d adds a day argument).
- `func Complete(q *gamedata.Quest, e Entry) bool`.
- `func Credit(d *gamedata.Data, log Log, monsterKind string, roll func(bp int) bool) []string` — §4.3; mutates `log`, returns changed ids in id order.
- `func BookSkill(class string, n int) string` (`"<class>_<n>"`), `func NeedsBook(learnLevel, current, waiver int) bool` (`current == 0 && learnLevel > waiver`).
- `func Grant(d *gamedata.Data, q *gamedata.Quest, class string, learned map[string]int, waiver int) (stacks []items.Item, equipment []string, book string)` — reward items to build (equipment as codes; `book` empty when not given).
- `func Round10(toNext int64, pct int) int64` — `(toNext × pct + 500) / 1000 × 10` (used by the script's Go twin in tests and by 6d).

**TODO / tests** (`// ____TEST RULES____`, `// ____TEST CREDIT____`, `// ____TEST REWARDS____`):
- [ ] `Available`: m01 at level 1; m02 not before m01 is done; m02 not at level 1 after m01; m23 at level 30 with an empty log; never when an entry exists.
- [ ] `Credit`: `kill` +1 for the matching kind only, capped at the count; `collect` with a roll stub true/false; a `talk` entry never changes; two active quests on one kind both change; returns ids sorted.
- [ ] `Grant` for m11/Viêm: `eq10_weapon_viem`, `book_viem_1`, two potion stacks; with `viem_1` learned or waiver 10 → no book; Nham gets `eq10_weapon_nham`.
- [ ] `Round10(80, 20) = 20`, `(91520, 10) = 9150`, `(2065500, 10) = 206550`.
- [ ] Commit: `Add the pure quest availability, credit and reward rules`.

### Task 3: Server — storage and dev tools

**Files:** `migrations/00009_quests.sql` (new), `internal/store/characters.go`, `internal/store/dev.go`, `internal/store/store_test.go`, `cmd/devquest/main.go` (new), `cmd/devchar/main.go`, their tests.

**Interface:**
- `store.QuestRow{QuestID string; State int16; Progress int; Day int32}`; `Character.Quests []QuestRow`, `Character.SkillBookWaiver int`; `CharState.Quests []QuestRow` (replaces all rows), `CharState.SkillBookWaiver int`.
- Dev helpers for e2e and `devquest`: `DevSetQuests(ctx, charID, rows []QuestRow)` (replace all), `DevSetWaiver(ctx, charID, w int)`.
- `devquest` flags per §7; `devchar -waive-books`.

**TODO / tests** (`// ____TEST QUESTS____`, DB):
- [ ] Migration up/down; an existing level-25 row gets waiver 25, a new character 0.
- [ ] Save active (progress 3) and done entries, reload; a second save with one entry leaves one row.
- [ ] `devquest`: `-done m05` → m01–m05 done; `-active m07 -progress 3`; `-active m05` (talk) → progress 1; `-reset`; unknown id errors and writes nothing. `devchar -level 10 -waive-books` → waiver 10; without the flag the waiver is unchanged.
- [ ] Commit: `Store quest progress and the skill book waiver`.

### Task 4: Server — protocol 14

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** envelopes 119–123; `QuestState`, `QuestEntry`, `QuestLog`; `EnterWorldResp.quests = 13`, `skill_book_waiver = 14` (confirm 12 is the last used number after 4a–5b); `LearnSkillResp.update = 3`; `QUEST_NOT_AVAILABLE = 53` … `NO_SKILL_BOOK = 56` with one-line comments. `ProtocolVersion = 14`. Quest requests are not handled yet (default `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; names GDScript-safe.
- [ ] Fixtures `quest_accept_req`, `quest_turn_in_resp` (book + weapon in `update`), `quest_progress`, `enter_world_resp_quests`; regenerate all with `-update`.
- [ ] Version test → 14.
- [ ] Commit: `Add the phase 6b quest messages and bump the protocol to 14`.

### Task 5: Server — quests in the zone

**Files:** `internal/zone/quest.go` (new), `internal/zone/zone.go`, `internal/zone/combat.go`, `internal/zone/transfer.go`, `internal/zone/area.go`, `internal/world/world.go`, `internal/zone/zone_test.go`, `internal/world/world_test.go`.

**Interface:**
- `PlayerInfo.Quests quest.Log`, `PlayerInfo.SkillBookWaiver int`; `player.quests quest.Log`, `player.questChanged []string`, `player.bookWaiver int`; `Area.Quests` (from `gamedata.Data`).
- `func (z *Zone) QuestAccept(id uint64, seq uint32, npc, quest string) bool`, `func (z *Zone) QuestTurnIn(id uint64, seq uint32, npc, quest string) bool`.
- `func (z *Zone) creditQuests(m *monster, killer *player)` — called beside `loot`.

**Behaviour:** §4.1–4.4, §4.6. Accept/turn-in error orders exactly as §4.2/§4.4; turn-in grants on a cloned bag first (BAG_FULL leaves everything untouched), then the real bag, Xu, `gainExp`, `statsDirty`, `z.saver.Save(z.state(p), nil)`. `syncView` flushes `questChanged` as one `QuestProgress` with the current entries. `state` writes `Quests` and the waiver; handoff deep-copies both. `world.Enter` copies them from `Character` after the §3 load checks (unknown ids dropped with a log line, progress clamped, main `day` reset to 0).

**TODO / tests** (`// ____TEST QUESTS____` in zone, `// ____TEST ZONES AND TRANSFER____` in world):
- [ ] Error orders of both commands (spec §8 "zone nhận/trả").
- [ ] Accept talk → progress 1; accept queues one save; done quest → `QUEST_NOT_AVAILABLE`.
- [ ] Turn in m01: +20 EXP, +40 Xu, 5 potions; full bag → `BAG_FULL`, nothing changed; m11 for Viêm → book and white weapon; waiver 10 → no book; a turn-in that levels up emits `LevelUp`.
- [ ] Credit: killer only; damage-only player and same-zone teammate get nothing; dead killer credited; killer gone → nothing; `QuestProgress` after that tick's `WorldTick` with only changed entries.
- [ ] Handoff keeps quests and the waiver; `EnterWorldResp.quests` lists every entry.
- [ ] World: unknown quest id dropped; progress 99 of a 5-kill quest clamped to 5.
- [ ] Commit: `Accept, credit and turn in quests in the zone`.

### Task 6: Server — skill books gate the first learn

**Files:** `internal/zone/progression.go`, `internal/zone/zone_test.go`.

**Behaviour (§4.5):** step 6 after `NOT_ENOUGH_POINTS`; on success consume one book from the lowest matching bag slot and put that slot in `LearnSkillResp.update`; save as today.

**TODO / tests** (`// ____TEST SKILL BOOKS____`):
- [ ] No book, waiver 0 → `NO_SKILL_BOOK`, points unchanged; with a book → learned, one book fewer, `update` lists the slot; level 1 → 2 needs no book; waiver 10 learns `viem_1` without a book; no points and no book → `NOT_ENOUGH_POINTS`.
- [ ] Commit: `Require a skill book for the first learn`.

### Task 7: Server — gateway, client helper and e2e

**Files:** `internal/gateway/handlers.go`, `internal/gateway/gateway_test.go`, `internal/client/client.go`, `e2e/e2e_test.go`.

**Interface:** `Membership.QuestAccept(seq, npc, quest) bool`, `Membership.QuestTurnIn(seq, npc, quest) bool`; gateway cases (need `inWorld`, packet limit only, full inbox → `RATE_LIMITED`); `client.QuestAccept(ctx, npc, quest)`, `client.QuestTurnIn(ctx, npc, quest)`; the client helper records `QuestProgress` pushes.

**TODO / tests:**
- [ ] Gateway: both requests before entering → `NOT_IN_WORLD`.
- [ ] e2e `TestQuestsOverWebSocket` per spec §8 (two characters; use `DevSetQuests`, `DevSetWaiver`, `DevSetLevel`, `DevSetPlace`; the `chi_huy` position comes from the generated `maps/ubuyashiki.json`).
- [ ] Commit: `Handle quest requests and add an end-to-end quest test`.

### Task 8: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-20-giai-doan-2-ky-nang-design.md`.

**TODO:**
- [ ] game-design 2.6, 4.1, 4.2, 10.7 per §9; phase 2 spec note.
- [ ] CLAUDE.md invariants per §9; README: quests, books, `devquest`, `devchar -waive-books`.
- [ ] Commit: `Document the phase 6b quests and skill books`.

### Task 9: Client — sync and protocol 14

**Files:** `server` submodule, `tools/sync_server.sh`, `proto/protocol.gd`, `data/`, `tests/fixtures/`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_core.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 8; `sync_server.sh` also copies `data/quests.json` (and `--check` compares it); `PROTOCOL_VERSION := 14`; `PAYLOADS` adds the five new payloads; the four error texts of §6.1.

**TODO / tests:**
- [ ] Decode the four new fixtures (book and weapon in the turn-in `update`; `EnterWorldResp.quests`, `skill_book_waiver`).
- [ ] Commit: `Sync the phase 6b protocol and quest data from the server`.

### Task 10: Client — quest model, region travel and the book rule

**Files:** `core/quest_book.gd` (new), `core/travel.gd`, `core/map_grid.gd`, `core/world_graph.gd`, `core/skill_book.gd`, `core/inventory.gd`, `autoload/router.gd`, `tests/unit/test_core.gd`.

**Interface:** §6.1 (`quest_book.gd` functions as listed; `travel` `kind == "region"` with `rects`; `world_graph.region(map_id, region_id)`; `skill_book.learn_block(skill, char_level, has_book, waiver)`; `inventory.count_of(code)`; `Router.quests`).

**TODO / tests** (`# ____TEST QUESTS____`):
- [ ] Real `quests.json`: `available` along the chain and by level; `npc_icon` "ready" beats "offer" beats "active"; `tracker` before/after m10 is done (next line "Gặp Chỉ huy…"/locked "Đạt cấp N…"); `goal_of` for a ready quest (turn-in NPC), an active kill (region of `objective`), an available quest (giver).
- [ ] `reward_of` drops the book when learned or waived; `book_source("viem_1", "viem") == "m11"`.
- [ ] `travel` region goal arrives as soon as the simulated walk enters a region tile.
- [ ] `learn_block` returns `"book"` only for a first learn above the waiver without a book; `count_of` counts bag stacks only.
- [ ] Commit: `Add the quest model, region travel and the skill book rule`.

### Task 11: Client — quest UI

**Files:** `ui/map_view.gd`, `ui/npc_window.gd`, `ui/quest_card.gd` (new), `ui/quest_tracker.gd` (new), `ui/skill_tab.gd`, `ui/hud.gd`, `ui/map_window.gd`, `screens/world.gd`.

**Behaviour (§6.2–6.3):** quest NPC circle "N" with the !/? marker; the NPC window's quest list and the quest card (Nhận, Trả nhiệm vụ with the bag-full lock, Đi tới mục tiêu); the tracker (collapse, tap → `travel.begin(goal_of(...))`); arrival opens the card of that quest (NPC) or notices "Đã tới bãi <tên quái>" (region); notices for progress, completion, accept and rewards; skill tab "Cần Sách …" and the source quest; map window role name "Nhiệm vụ". `world.gd` applies `EnterWorldResp.quests`, `QuestProgress`, accept/turn-in replies and `LearnSkillResp.update`.

**TODO:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server: m01–m02 on a new character via the tracker; m05 walks to Ubuyashiki; markers change; book learn after `devquest`. Screenshots in the report.
- [ ] Commit: `Show quest markers, the quest card and the tracker`.

### Task 12: Client — smoke and README

**Files:** `tools/smoke.sh`, `tests/smoke/smoke.gd`, `README.md`.

**Behaviour (§6.4):** the dev commands before re-entering; `NO_SKILL_BOOK` → turn in m11 → learn → accept m12. Print `SMOKE OK (… quest m11, book used)`.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: quest NPCs, markers, tracker, books.
- [ ] Commit: `Extend the smoke test with a quest turn-in and a skill book`.

---

## Acceptance (controller, after Task 12)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID); migration 00009 gives `test1`/`test2` waiver 90.
- [ ] Tell the user what to try (spec §8 "Nghiệm thu").
