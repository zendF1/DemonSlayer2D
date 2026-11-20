# Phase 3a Bag, Drops and Equipment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A monster's killer gets Xu at once and may get equipment or stones on the ground beside the corpse, visible and pickable only by them. Players see their bag and their ten worn slots in the character window, wear, take off, discard and sort items, and worn gear raises their stats by the approved tables. Monsters from level 10 are regenerated against the reference +0 gear, as game-design 5.9 says.

**Architecture:**
- `docs/balance/item_stats.py` generates `data/items.json` (slots, qualities, line rules, 70 equipment pieces from `equipment_balance.py`, stackables, drop tables). `monster_stats.py` adds the reference gear and a `rank` per kind; `exp_table.py` is rerun. `gamedata` loads and validates `items.json`; the client reads a synced copy.
- A new pure package `internal/items` holds the item model and rules: rolling drops and lines, adding to the bag, the gear stat sum. `combat.Compute` takes the gear sum before passives.
- The zone owns each player's bag, worn items and Xu, ground items (a new `ITEM` entity kind, shown only to the owner, gone after 60 s) and the item commands. The saver persists everything through `SaveCharacterState` (new `characters.xu`, new `character_items` table).
- Protocol 6: five request/response pairs (61–70), `ItemInfo`/`ItemLine`/`ItemSlot`/`Inventory`/`InventoryUpdate`, `EnterWorldResp.inventory`, `SelfStats.xu`, `ENTITY_KIND_ITEM`, `EntityEnter.item`, three error codes.

**Tech stack:** Go, protobuf (buf), Postgres (goose migrations), Python 3 balance scripts; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md](../specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md). Section references below (§n) point to it.

**Prerequisite:** the [attack phases plan](2026-11-20-moc-don-danh.md) is fully implemented (impact queue, `applyToMonster` decides the killer, protocol 5). Start this plan from a server HEAD that contains it.

**Plan format (from the user):** interfaces, behaviour, test cases and TODOs only. Write the code at implementation time; nothing here is meant to be copied.

**Commit rules (from the user):** commit and push after every task with a clear message; **no** `Co-Authored-By` or other trailer; `git add` specific paths only (never `-a`), because other agents may edit the same tree; `git pull --rebase` before `git push`, never force push. Server tasks commit in this repo, client tasks in `../DemonSlayer2DGame`.

**Test rules:** Go tests with `-race` (DB/e2e: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test -race ./...`); 1–2 test files per package with `// ______________________TEST X_____________________` sections (GDScript `# ____...`); a fix comes with a test that fails on the old code; before committing Go: `staticcheck ./...`, `go vet ./...`, `graphify update .` (skip if graphify is missing). Every commit leaves `go build ./...` and `make test` green. Client: `tests/run.sh`.

**Environment:** never stop anything on :8099; the controller's acceptance server runs on :8199 (PID in the scratchpad file `gameserver-8199.pid`) — do not kill it; for your own live runs use another free port such as :8299 and kill only your PID. Never print DB passwords.

---

## Verified findings (2026-11-20, server HEAD 864be67 with the attack-phases zone work in the tree, client HEAD 4e668b2)

Line numbers drift while the attack phases land; find the symbols by name (`graphify explain "<symbol>"`) before editing.

**Data and balance**
- `docs/balance/equipment_balance.py`: `generate()` returns `items[(tier_index, upgrade, slot)] = (attack, hp, defense)` for 8 lines (indices 5–7 crafted, excluded by `INCLUDE_CRAFTED = False`); `available_tier(indices, level, slot)` picks the highest tier with `tier + slot <= level`; `write_csv` derives names (`WEAPONS`, `SLOTS`, "Phủ–Chùy" for Nham) and splits weapon attack into physical/magical by class (`physical = attack if class_id in (0, 2, 3)`). The CSV has 1,120 data rows: 5 tiers × 16 upgrades × 14 rows (9 shared + 5 weapons).
- `docs/balance/monster_stats.py`: `reference(level)` returns `(hp, attack, defense)` without gear; `monster_hp(level, atk_ref, defense)`; `monster(level)`; pinned assertions at the end of `main()` include `monster(10) == (658 hp, 125 exp)` — these change (level 10 becomes 750 HP, 143 EXP). Levels 1–5 stay pinned. `KINDS` carries `melee_windup_ms`/`shot_speed` since the attack phases.
- A scratch run with the reference gear added reproduces game-design 5.9 exactly at levels 10, 20, 40, 50, 60, 70, 80, 90; level 30 gives 4,336 HP (the doc says 4,338) with attack 508 and defense 240.
- `gamedata.Parse` decodes `monsters.json` strictly (`internal/gamedata/gamedata.go:85`); `Kind` (`:21`) has no rank. `Load(dir)` reads monsters, skills, exp (`:130`). `gamedata_test.go` compares each script's `--print-json` with the committed file (skips without python3) — extend that table.

**Combat**
- `combat.Compute(magic bool, level int, p Points, pas Passive) Stats` (`internal/combat/combat.go:38`) applies passives with `st.X += st.X * pct / 100` after the base block. `SlashBase` is `max(PhysAttack, MagicAttack)` (`:124`).

**Zone**
- `PlayerInfo` (`internal/zone/zone.go:52`) and `player` (`:80`) carry points, skills, skill points; `join` copies them and sends `EnterWorldResp` (`:437`, `:472`). `state(p)` (`:532`) builds `store.CharState` and clones the skill map for the saver goroutine.
- `syncView` (`:542`) hides monster corpses from viewers who never saw them alive (`:561`); reuse that branch for owner-only items.
- `recompute(p)` (`internal/zone/progression.go:17`) is the only stat path; `allocate` (`:82`) shows the "save at once + reply with SelfStats" pattern; `gainExp` (`:57`).
- Attack phases: `applyToMonster(im)` (`internal/zone/impact.go:79`) subtracts HP, pays EXP to `z.players[im.attacker]` when present, and calls `z.killMonster(m)` at 0 HP (`:98`). `killMonster` (`internal/zone/combat.go:215`) has no killer argument today.
- Monster ids come from `z.cfg.NewID()` (`internal/zone/spawn.go:120`); use it for ground items too. `m.region.Kind` is the monster's `gamedata.Kind`.
- `hpChanged(e)` (`combat.go:73`), `entityByID` (`:80`), `addEvents` (`:95`), `replyError`.
- Tick order today: `drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView` (`zone.go:355`).
- Test helpers: `newTestZone(capacity, grid)` returns the zone and a `fakeSaver` (`internal/zone/zone_test.go:102`).

**Store, world, gateway**
- `store.Character`/`CharState` (`internal/store/characters.go:15`, `:47`), `charColumns` + `scanCharacter`, `loadSkills` (one query for many characters, `:75`), `saveState` (UPDATE then delete+insert skills via `unnest`, `:172`). Latest migration: `migrations/00003_skills.sql`.
- `store.DevSetLevel` (`internal/store/dev.go`) and `cmd/devchar/main.go` are the model for `devitem` (flags, `DATABASE_URL`, offline warning).
- `world.Enter` builds `PlayerInfo` (`internal/world/world.go:239`); `knownSkills` (`:280`) drops bad rows with one `log.Warn` each — copy for items. `areaOf` (`:103`) fills `zone.Area` from `gamedata.Data`. `Membership` wraps zone calls (`:203–235`).
- `internal/gateway/handlers.go`: `LearnSkillReq` (`:100`) is the model for a request with only the packet limit: `inWorld`, then `zoneBusyIfDropped(seq, c.member.X(...))`. `ProtocolVersion = 5` (`internal/gateway/server.go:23`).
- Proto: Envelope payloads end at `cast_resp = 60`; `ErrorCode` ends at `LEVEL_TOO_LOW = 26`; `EnterWorldResp` uses 1–10; `SelfStats` 1–20; `EntityEnter` 1–12; `EntityKind` 0–2.
- e2e: `startServer` builds `config.Config{... DataDir: "../data" ...}` (`e2e/e2e_test.go:37`); `newPlayer`, `waitFor`; `TestSkillsOverWebSocket` (`:527`) uses `DevSetLevel` then re-enters — the model for the items e2e. `internal/client` request helpers (`client.go:213–256`).

**Client**
- `ui/char_window.gd`: `enum { EQUIP, INFO, POTENTIAL, SKILLS, BAG }`, pages built as `[_later(), info, potential, skills, _later()]` (`:117`) — replace the two `_later()` pages.
- `ui/hud.gd` `_build_buttons` (`:297`) places round buttons in the bottom-right `CLUSTER` box with `_place_round`; potion buttons show `LATER`.
- `core/entity_table.gd` keeps `entries` by id with `kind`; `screens/world_entities.gd` builds views per entry; `screens/world.gd` `_pick` (`:256`) maps a tap to `_targeting.pick_at` for monsters.
- `core/attack_loop.gd` is the model for `pickup_loop.gd` (approach, OUT_OF_RANGE walk-straight, reply handling).
- `tools/sync_server.sh` copies maps, fixtures, `skills.json` and regenerates `proto/protocol.gd`; `--check` diffs them.
- `autoload/net.gd` `PROTOCOL_VERSION` is 5 after the attack phases; `core/wire.gd` `PAYLOADS`.

## File structure

Server (`DemonSlayer2D`):

```
docs/balance/item_stats.py (new), data/items.json (new)          item data (Task 1)
docs/balance/monster_stats.py, data/monsters.json                 reference gear, rank (Task 1)
docs/balance/exp_table.py, data/exp.json, exp-table.csv           rerun (Task 1)
internal/gamedata/items.go (new), gamedata.go, gamedata_test.go   parse + validate (Task 2)
internal/combat/combat.go, combat_test.go                         Bonus in Compute (Task 3)
internal/items/items.go, roll.go, bag.go (new), items_test.go     item rules (Task 4)
proto/ds/v1/protocol.proto, internal/pb/                          protocol 6 (Task 5)
internal/gateway/server.go, handlers.go, gateway_test.go, testdata/client/  version, fixtures, handlers (Tasks 5, 8, 9)
migrations/00004_items.sql (new)                                  xu, character_items (Task 6)
internal/store/characters.go, store_test.go                       items load/save (Task 6)
internal/world/world.go, world_test.go                            item validation, PlayerInfo (Task 6)
internal/zone/zone.go, progression.go, area.go                    inventory state, recompute (Task 6)
internal/zone/loot.go (new)                                       Xu, drops, ground items (Task 7)
internal/zone/inventory.go (new)                                  pickup, equip, unequip, discard, sort (Tasks 8, 9)
internal/zone/zone_test.go, combat_test.go                        tests
internal/client/*.go                                              Pickup, Equip, Unequip, Discard, SortBag (Tasks 8, 9)
internal/store/dev.go, cmd/devitem/main.go (new)                  devitem (Task 10)
e2e/e2e_test.go                                                   items e2e (Task 10)
docs/game-design.md, CLAUDE.md, README.md, phase 0 and 2 specs    docs (Task 11)
```

Client (`DemonSlayer2DGame`):

```
server/ (submodule), data/items.json, proto/protocol.gd, tests/fixtures/  sync (Task 12)
tools/sync_server.sh, autoload/net.gd, core/wire.gd, core/errors.gd       protocol 6 (Task 12)
core/item_book.gd, core/inventory.gd (new)                        item data, bag state (Task 13)
core/pickup_loop.gd (new), core/entity_table.gd                   ground items, pickup (Task 14)
ui/bag_tab.gd, ui/equip_tab.gd, ui/item_tooltip.gd, ui/ground_item_view.gd (new)  UI (Task 15)
ui/char_window.gd, ui/hud.gd, screens/world.gd, screens/world_entities.gd         wiring (Task 15)
tests/unit/test_core.gd, tests/unit/test_net.gd                   tests (Tasks 12-14)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                   smoke (Task 16)
```

---

### Task 1: Server — item data, monsters with reference gear, EXP rerun

**Files:** `docs/balance/item_stats.py` (new), `data/items.json` (new), `docs/balance/monster_stats.py`, `data/monsters.json`, `docs/balance/exp_table.py` (only if it needs the import path), `data/exp.json`, `docs/balance/exp-table.csv`, `internal/gamedata/gamedata_test.go`.

**Behaviour:**
- `item_stats.py` (§2.1) imports `equipment_balance` (same directory, as `monster_stats` imports `skill_stats`), calls `generate()`, and builds the JSON in §2.1's key order: `bag_size 60`, `stack_max 999`, `ground_sec 60`, `slots` (the §2.1 table), `qualities` (the five rows), `line_stats`, `line_roll_min_pct 50`, `tiers` with `line_standard` (S = max(1, round_half_up(4% × full-set +0 budget)), computed with `Fraction` from `TIERS` of `equipment_balance`), the 70 `equipment` entries (names and per-class attack split exactly as `write_csv` does), `stackables` (`stone_enhance`, `stone_refine`), `drops` (§2.1 table). `--write-json` / `--print-json` as the other scripts (2-space indent, `ensure_ascii=False`, trailing newline).
- Self-checks in `item_stats.py`: every equipment array element equals the matching CSV row (read `equipment-stats-proposal.csv`); the S table equals §2.1; each `quality_bp` sums to 10000; codes unique.
- `monster_stats.py` (§2.2): `reference(level)` adds the summed +0 gear of `available_tier(range(5), level, slot)` per slot from `equipment_balance.generate()` (compute once, module level or cached). Add `"rank": "normal"` to both kinds. Update the pinned assertions: keep levels 1–5; replace `monster(10) == (658, 125)` with `(750, 143)`; add the §2.2 pins (10, 20, 30, 40, 50, 60, 90 HP/attack/defense). Update the module docstring rules.
- Rerun `exp_table.py --write-json --write-csv`; its hour assertions must still pass unchanged.

**TODO / tests:**
- [ ] Write `item_stats.py`; regenerate `items.json`, `monsters.json`, `exp.json`, `exp-table.csv`.
- [ ] `// ____TEST GENERATED DATA____`: add `item_stats.py → items.json` to the compare-with-script table.
- [ ] Existing zone tests that hard-code level ≥ 10 monster numbers, if any, derive them from the data instead.
- [ ] Add `Rank string \`json:"rank"\`` to `gamedata.Kind` in this task, because strict decoding rejects the new field; Task 2 validates it. `make test` green.
- [ ] Commit: `Generate item data and rebuild monster stats against the reference gear`.

### Task 2: Server — load and validate items.json and monster ranks

**Files:** `internal/gamedata/items.go` (new), `internal/gamedata/gamedata.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `type Items struct { BagSize, StackMax int; GroundSec float64; Slots []Slot; Qualities []Quality; LineStats []string; LineRollMinPct int; Tiers map[int]Tier; Equipment map[string]Equipment; Stackables map[string]Stackable; StackOrder []string; Drops map[string]Drop }`.
- `Slot{ID, Name string; Index int}`, `Quality{Name, Rank string; Lines [2]int; HiddenPct int}`, `Tier{Tier int; Name string; LineStandard map[string]int}`, `Equipment{Code string; Tier int; Slot, Class, Name string; Level int; HP, PhysAttack, MagicAttack, PhysDefense, MagicDefense []int}` (+ `SlotIndex int` filled after parse), `Stackable{Code, Name string}`, `Drop{XuPerLevel [2]int; Equipment DropEquipment; Stacks []DropStack}`, `DropEquipment{ChanceBP, Count int; QualityBP []int}`, `DropStack{Code string; ChanceBP, Count int}`.
- Constants: slot ids (`SlotWeapon = "weapon"`, …, in index order as `SlotIDs [10]string`), line stat keys (`StatHP = "hp"`, …), ranks (`RankNormal`, `RankElite`, `RankBoss`), `EnhanceLevels = 16`.
- `func ParseItems(b []byte) (*Items, error)`; `(*Items).IsEquipment(code) bool`; `(*Items).Weapon(tier int, class string) string` (code lookup); `(*Items).Piece(tier, slotIndex int) string`.
- `Data.Items *Items`; `Load` reads `items.json`. `Kind.Rank` validated in `Parse`.

**Behaviour:** every check of §2.1 "Server kiểm tra khi nạp" and the rank rule of §2.2, with errors naming the code/field. Equipment arrays are indexed by enhance level.

**TODO / tests** (`// ____TEST ITEMS DATA____`):
- [ ] The committed `items.json` parses: 70 equipment pieces, 14 per tier, codes `eq10_weapon_viem`, `eq50_ring_right` exist; `Weapon(50, "thuy")` is the Thủy Nguyệt Xích Cương Kiếm code; 2 stackables; 3 ranks.
- [ ] Reject table built by mutating a valid minimal JSON: unknown field; 9 slots; slots out of order; 4 qualities; Vàng with `hidden_pct` 0; Xanh with `hidden_pct` 10; `lines` max above 5; duplicate line stat; unknown line stat; tier missing a line standard; standard 0; duplicate code; weapon without class; armour with class; wrong `level`; array of 15; decreasing array; negative value; physical-class weapon with magic attack; tier missing a piece; stackable code equal to an equipment code; missing `boss` drop; `quality_bp` sum 9999; stack drop of an unknown code; `chance_bp` 10001.
- [ ] `monsters.json` kind with `rank: "champion"` or without rank fails.
- [ ] Commit: `Load and validate item data and monster ranks`.

### Task 3: Server — gear in derived stats

**Files:** `internal/combat/combat.go`, `internal/combat/combat_test.go`, every `Compute` call site (`internal/zone/progression.go`, tests).

**Interface:**
- `type Bonus struct { HP, PhysAttack, MagicAttack, PhysDefense, MagicDefense int }`; `func (b Bonus) Add(c Bonus) Bonus`.
- `func Compute(magic bool, level int, p Points, gear Bonus, pas Passive) Stats` — base block, then `+gear`, then passives on the sum (§4.2).

**TODO / tests** (`// ____TEST GEAR BONUS____`):
- [ ] Zero `Bonus` gives exactly today's stats (table over levels and points, with and without passives).
- [ ] Viêm level 10, 60 STR/30 VIT, `Bonus{PhysAttack: 25}`: physical attack 210, HP 1,640.
- [ ] Nham passive `MaxHPPct 20` with `Bonus{HP: 200}`: max HP is `floor((base + 200) × 1.2)`, exactly 240 more than with `Bonus{}` (the passive applies to the gear too).
- [ ] Commit: `Add worn gear to derived stats before passives`.

### Task 4: Server — the items package

**Files:** `internal/items/items.go`, `roll.go`, `bag.go` (new), `internal/items/items_test.go`.

**Interface:**
- `type Line struct { Stat string; Value int }`.
- `type Item struct { Code string; Count, Quality int; Element string; Enhance int; Lines, Hidden []Line }`; `func (it Item) Clone() Item` (deep copy of slices).
- `type Inventory struct { Bag []*Item; Equip [10]*Item }`; `func NewInventory(bagSize int) *Inventory`; `func (inv *Inventory) Clone() *Inventory`.
- Errors: `ErrBagFull`, `ErrNotFound`, `ErrBadSlot`, `ErrNotEquipment`, `ErrWrongClass`, `ErrLevelTooLow`, `ErrBadCount` (the zone maps them to `pb.ErrorCode`).
- `func DropTier(monsterLevel int) int` — `min(50, max(10, 10 × floor(L/10)))`.
- `func RollDrops(rng *rand.Rand, d *gamedata.Items, rank string, monsterLevel int, killerClass string) (xu int64, drops []Item)` — §4.3 order: Xu, equipment, stacks.
- `func NewEquipment(rng *rand.Rand, d *gamedata.Items, code string, quality int) Item` — §4.4.
- `func (inv *Inventory) Add(d *gamedata.Items, it Item) (changed []int, err error)` — §4.1, all or nothing.
- `func (inv *Inventory) FirstFree() int` (−1 when full).
- `func (inv *Inventory) Equip(d *gamedata.Items, bagSlot, level int, class string) (equipSlot int, err error)`; `Unequip(equipSlot int) (bagSlot int, err error)`; `Discard(bagSlot, count int) error`; `Sort(d *gamedata.Items)` — §4.7 rules and check order (after the zone's DEAD check).
- `func Wearable(d *gamedata.Items, it *Item, level int, class string) bool`.
- `func GearBonus(d *gamedata.Items, inv *Inventory, level int, class string) combat.Bonus` — base stats at `Enhance` plus lines of wearable worn items (hidden lines ignored in 3a; leave a named hook for 3c).

**Behaviour:** classes are the lower-case ids (`viem`…); use `classes` helpers in the zone to convert from `pb.CharClass`. All random choices go through the passed `rng`, in the §4.3/§4.4 order, so seeded tests are stable.

**TODO / tests** (`// ____TEST ROLL____`, `// ____TEST BAG____`, `// ____TEST GEAR____`):
- [ ] `DropTier` at 1, 9, 19, 20, 55, 90 → 10, 10, 10, 20, 50, 50.
- [ ] `NewEquipment` over 10,000 seeded rolls per quality: line counts within the quality's range; no duplicate stat in `Lines`; every value in `[ceil(S/2), S]` of its tier; Vàng/Cam/Đỏ have an element and exactly two hidden lines valued `round_half_up(S × pct / 100)` (tier-10 HP: 86/108/129); Trắng/Xanh have neither.
- [ ] `RollDrops` with the real data, 100,000 seeded normal kills at level 15: equipment rate within 5% ± 0.3 points; quality shares within 1 point of 80/17/3; stone rate within 20% ± 0.5; Xu always in [45, 75]; weapons always of the killer's class; tier 10.
- [ ] `Add`: a stack fills an existing stack first (998 + 3 → 999 and a new slot with 2); equipment goes to the first free slot; a full bag returns `ErrBagFull` and changes nothing.
- [ ] `Equip` swaps with the worn item; order of errors per §4.7; `Unequip` into the first free slot; `ErrBagFull` when none.
- [ ] `Discard` count 0 removes the slot, count 2 of 5 leaves 3, count 6 of 5 → `ErrBadCount`.
- [ ] `Sort` merges two stacks of 600 into 999 + 201 and orders equipment by tier desc, slot index, quality desc.
- [ ] `GearBonus`: Viêm level 10 wearing the Lữ Hành sword → `PhysAttack 25`; the same sword on a Thủy adds nothing (wrong class); a tier-50 ring at level 30 adds nothing; lines add their values.
- [ ] Commit: `Add the item model, drop rolls and bag rules`.

### Task 5: Server — protocol 6

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/` (`make proto`), `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 61–70; `ItemInfo`, `ItemLine`, `ItemSlot`, `Inventory`, `InventoryUpdate`; `EnterWorldResp.inventory = 11`; `SelfStats.xu = 21`; `ENTITY_KIND_ITEM = 3`; `EntityEnter.item = 13`; error codes 27–29 with one-line comments. `ProtocolVersion = 6`. The gateway does not handle the new requests yet (default branch answers `BAD_REQUEST`).

**TODO / tests:**
- [ ] `make proto`; no field named `message`/`class`, no message `Error`.
- [ ] Fixtures in `TestClientFixtures`: `enter_world_resp_inventory`, `entity_enter_item`, `equip_resp` (§5). Regenerate all with `-update`.
- [ ] Round-trip test: `entity_enter_item` decodes to kind ITEM with its quality, element, lines and hidden lines.
- [ ] Version test → 6; e2e still logs in.
- [ ] Commit: `Add the phase 3a item messages and bump the protocol to 6`.

### Task 6: Server — store, world and zone plumbing for items and Xu

**Files:** `migrations/00004_items.sql` (new), `internal/store/characters.go`, `internal/store/store_test.go`, `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/zone.go`, `internal/zone/area.go`, `internal/zone/progression.go`, tests.

**Interface:**
- `store.ItemRow { Equipped bool; Slot int; Code string; Count, Quality int; Element string; Enhance int; Lines, Hidden []store.ItemLine }`, `store.ItemLine { Stat string \`json:"stat"\`; Value int \`json:"value"\` }`.
- `store.Character` and `CharState` gain `Xu int64`, `Items []ItemRow` (never nil on a loaded character).
- `store` loads items with one query for many characters (as `loadSkills`); `saveState` writes `xu` in the UPDATE, then deletes and inserts the item rows (one `INSERT … SELECT FROM unnest(...)` with `jsonb[]` for lines, or `CopyFrom` inside the tx — pick one, document it).
- `zone.PlayerInfo` gains `Xu int64`, `Inventory *items.Inventory` (the world builds it; the zone clones it on join). `zone.Area` gains `Items *gamedata.Items`.
- `player` gains `xu int64`, `inv *items.Inventory`.
- `world.knownItems(c store.Character) *items.Inventory` — validates per §3 and logs one warning per dropped row.

**Behaviour:**
- Migration per §3 with a goose Down.
- `z.recompute(p)` passes `items.GearBonus(z.area.Items, p.inv, p.level, class)`.
- `z.state(p)` saves `xu` and item rows from a deep copy.
- `join` sends `EnterWorldResp.inventory` (occupied slots only, bag size) and `SelfStats.xu`. Add a zone helper `itemInfo(it *items.Item) *pb.ItemInfo` and `inventoryUpdate(bag, equip []int)`.

**TODO / tests:**
- [ ] `// ____TEST ITEMS STORE____` (DB): save then load round-trips Xu and items (bag and worn, lines, hidden, element); saving an empty list deletes old rows; deleting the character cascades; `xu` defaults to 0 after migrating an existing character.
- [ ] World: rows with an unknown code, bag slot 60, worn slot 10, a ring in the weapon slot, two rows in one slot, a stack of 1,000, equipment with count 2 are dropped and logged; good rows kept.
- [ ] Zone: joining with a worn Lữ Hành sword gives the higher attack; `EnterWorldResp` carries the inventory; `SelfStats.xu` carries Xu; `state` round-trips the inventory without sharing slices (mutating the saved copy does not change the player).
- [ ] Commit: `Store, load and carry the bag, worn items and Xu`.

### Task 7: Server — Xu, drops and ground items

**Files:** `internal/zone/loot.go` (new), `internal/zone/impact.go`, `internal/zone/combat.go`, `internal/zone/zone.go`, `internal/zone/entity.go`, tests.

**Interface (internal):**
- `type groundItem struct { entity; owner int64; item items.Item; expires uint32 }`; zone field `ground map[uint64]*groundItem`.
- `killMonster(m *monster, killer *player)` — `killer` nil when the attacker left; `applyToMonster` passes `z.players[im.attacker]`.
- `func (z *Zone) loot(m *monster, killer *player)` — §4.3; `func (z *Zone) dropAt(pos Vec2, i, n int) Vec2` — §4.5 placement.
- `func (z *Zone) runGroundTimers()` — removes expired items.
- `enterEnv` fills `EntityEnter.item` for `ITEM` entities.

**Behaviour (§4.3, §4.5, §4.9):**
- Xu: `killer.xu += xu`, `killer.statsDirty = true`.
- Each drop becomes a `groundItem` with a `NewID` id, kind `ITEM`, `name` from the data, `owner = killer.charID`, `expires = tickNo + z.ticks(ground_sec)`, added to the AOI cells.
- `syncView` skips `ITEM` entities whose owner is not `p.charID`; a viewer never gets their `EntityEnter`.
- `entityByID` does not need items (no HP events).
- Tick order: `runGroundTimers` after `runSpawnTimers`.

**TODO / tests** (`// ____TEST LOOT____`, use an area whose `Items` has 100% drop chances, and the real data for the Xu range):
- [ ] A kill by a slash pays Xu within `[3L, 5L]` and sets `statsDirty`; the next `SelfStats` carries it.
- [ ] With 100% chances: one equipment and one stone stack appear as `ITEM` entities 0.6 tiles from the corpse; the killer's next `WorldTick` cycle gets their `EntityEnter` with `item` filled; a second player standing next to the corpse gets none.
- [ ] The killer leaves before the killing impact lands: no Xu, no drop.
- [ ] A dead killer still gets the drop and the Xu.
- [ ] After `z.ticks(60)` ticks the item is gone and the owner gets `EntityLeave`.
- [ ] The owner re-joins as a new entity (same `charID`) within 60 s and sees the item again.
- [ ] A drop position on a wall tile falls back to the corpse position.
- [ ] Commit: `Pay Xu to the killer and drop owner-only items on the ground`.

### Task 8: Server — pickup

**Files:** `internal/zone/inventory.go` (new), `internal/zone/zone.go`, `internal/world/world.go` (Membership), `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:**
- `zone.(*Zone).Pickup(id uint64, seq uint32, itemID uint64) bool` + `pickupCmd`; `Membership.Pickup(seq, itemID) bool`.
- Gateway `PickupReq`: `inWorld`, then `zoneBusyIfDropped`; packet limit only.
- `client.Pickup(ctx, itemID) (*pb.PickupResp, error)`.

**Behaviour (§4.6):** order DEAD → ITEM_NOT_FOUND (missing or other owner) → OUT_OF_RANGE (`distance > 1.5 + attackRangeSlack`) → BAG_FULL. Accept: `inv.Add`, remove the ground entity from its cell and `z.ground`, save at once, reply `PickupResp{update}` with the changed bag slots.

**TODO / tests** (`// ____TEST PICKUP____`):
- [ ] One case per error, in order (a dead owner next to their item gets DEAD; another player's item id gets ITEM_NOT_FOUND; 2.1 tiles away OUT_OF_RANGE, 1.9 accepted; a full bag BAG_FULL and the item stays).
- [ ] A stone stack merges into an existing stack and the update lists that slot.
- [ ] A save is queued after a pickup; the saved state holds the item.
- [ ] Gateway: `PickupReq` before entering the world gets NOT_IN_WORLD.
- [ ] Commit: `Let players pick up their drops`.

### Task 9: Server — equip, unequip, discard, sort

**Files:** `internal/zone/inventory.go`, `internal/zone/progression.go`, `internal/zone/zone.go`, `internal/world/world.go`, `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:**
- Zone methods + commands: `Equip(id, seq, bagSlot)`, `Unequip(id, seq, equipSlot)`, `Discard(id, seq, bagSlot, count)`, `SortBag(id, seq)`; matching `Membership` wrappers, gateway cases and client helpers (`Equip`, `Unequip`, `Discard`, `SortBag`).
- `func (z *Zone) gearChanged(p *player)` — recompute, apply the §4.7 HP rule, `hpChanged` when max HP changed, `statsDirty`.

**Behaviour (§4.7):** DEAD first for every command; then the item package's checks mapped to codes (`ErrBadSlot`/`ErrNotEquipment`/`ErrBadCount` → BAD_REQUEST, `ErrNotFound` → ITEM_NOT_FOUND, `ErrWrongClass` → WRONG_CLASS, `ErrLevelTooLow` → LEVEL_TOO_LOW, `ErrBagFull` → BAG_FULL). Accept: save at once, reply with the response of §5 (`stats` from `selfStats` after `gearChanged` for equip and unequip). Sort replies with every bag slot.

**TODO / tests** (`// ____TEST EQUIP____`):
- [ ] Each command's errors in order (DEAD beats a bad slot; WRONG_CLASS beats LEVEL_TOO_LOW).
- [ ] Equip into an occupied slot swaps; the update lists the bag slot and the worn slot.
- [ ] HP rule: full stays full; 500/1,000 → max 1,500 gives 750; unequip then re-equip never raises HP above where it started; max HP change records an `HpChange` for viewers.
- [ ] Discard and sort as in Task 4, through the zone, each queues a save.
- [ ] Commit: `Let players wear, take off, discard and sort items`.

### Task 10: Server — devitem and the items e2e

**Files:** `internal/store/dev.go`, `internal/store/store_test.go`, `cmd/devitem/main.go` (new), `e2e/e2e_test.go`, `README.md`.

**Interface:**
- `func (s *Store) DevGiveItem(ctx context.Context, username string, place func(rows []ItemRow) (ItemRow, error), xu int64) (DevItemResult, error)` — loads the first character's rows in a tx (`FOR UPDATE`), calls `place` to choose the new row (or merged rows; let `place` return the full replacement list if simpler — document the choice), writes, adds `xu`. `DevItemResult{Name string; Slot int; Xu int64}`.
- `cmd/devitem`: flags `-u`, `-code`, `-count`, `-quality`, `-xu`; reads `DATABASE_URL` and `DATA_DIR` (default `./data`); uses `items.NewEquipment` with a time-seeded `rand.Rand` and `Inventory.Add` for placement.

**Behaviour (§7):** offline warning first; unknown code → error listing nothing else; full bag → error, nothing written; `-xu` alone only adds Xu.

**TODO / tests:**
- [ ] `// ____TEST DEV ITEM____` (DB): gives a sword into slot 0 of an empty bag; merges a stack; full bag errors; `-xu 500` adds 500; missing account errors.
- [ ] e2e `TestItemsOverWebSocket` (§8): copy `../data` into `t.TempDir()` with the normal drop chances set to 10000 (decode `items.json` into `map[string]any`, patch, re-encode), start the server on it, create a player, leave, `DevSetLevel(…, 19)`, enter, kill a monster (reuse the fight helper), wait for the `SelfStats` with Xu > 0 and an `EntityEnter` of kind ITEM, walk to it, pick it up, equip it if it is equipment (else equip a second drop), see the higher stat, leave, re-enter, and find the bag and worn item in `EnterWorldResp.inventory`.
- [ ] README: a "Dev items" paragraph with the `devitem` command and examples (`-code eq50_armor -quality 2`, `-code stone_enhance -count 100`, `-xu 100000`).
- [ ] Commit: `Add the devitem command and an end-to-end items test`.

### Task 11: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`, `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`, `docs/superpowers/specs/2026-11-20-giai-doan-2-ky-nang-design.md`.

**TODO:**
- [ ] game-design per §9 (3.1, 5.9, 5.12, 2.2 with numbers from the regenerated CSV).
- [ ] Phase 0 roadmap note and phase 2 spec note per §9.
- [ ] CLAUDE.md invariants per §9 and the tick order with `runGroundTimers`.
- [ ] Vietnamese prose in docs, matching their style.
- [ ] Commit: `Document the phase 3a items, drops and regenerated monsters`.

### Task 12: Client — sync and protocol 6

**Files:** `server` submodule, `tools/sync_server.sh`, `data/items.json` (new), `proto/protocol.gd`, `tests/fixtures/`, `autoload/net.gd`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_net.gd`.

**Behaviour:** bump the submodule to the server HEAD after Task 11; sync copies and checks `items.json`; `PROTOCOL_VERSION := 6`; `PAYLOADS` adds the ten new names in proto order; error texts per §6.1.

**TODO / tests:**
- [ ] Decode `enter_world_resp_inventory.hex`, `entity_enter_item.hex`, `equip_resp.hex`.
- [ ] Every new code has a non-empty text; `tests/run.sh` green including `--check`.
- [ ] Commit: `Sync phase 3a protocol and item data from the server`.

### Task 13: Client — item book and inventory

**Files:** `core/item_book.gd`, `core/inventory.gd` (new), `tests/unit/test_core.gd`.

**Interface:**
- `item_book.gd`: `func load_json(text) -> bool`; `def(code) -> Dictionary`; `is_equipment(code)`; `slot_index(code)`; `stats_at(code, enhance) -> Dictionary` (non-zero stats only); `quality_name(q)`, `quality_color(q)`; `stat_name(key)`; `element_name(e)`; `equip_block(item, char_level, cls) -> String`.
- `inventory.gd`: `func load(inv)`, `func apply(update)`, `func set_xu(xu) -> int` (returns the gain, 0 if none), `bag_size`, `bag` (slot → item dict), `equipped` (slot → item dict), `func can_take(item_dict) -> bool`, `func first_free() -> int`. Item dicts: `{code, count, quality, element, enhance, lines: [{stat, value}], hidden: [...]}`.

**TODO / tests** (`# ____TEST ITEM BOOK____`, `# ____TEST INVENTORY____`):
- [ ] The synced `items.json` loads: 70 equipment pieces; `stats_at("eq10_weapon_viem", 0)` is `{phys_attack: 25}`.
- [ ] `equip_block`: wrong class → "class"; level 14 with a helmet (needs 14) → ""; level 13 → "level".
- [ ] `apply` sets and clears slots; `can_take` mirrors the server (merge, full bag); `set_xu` returns gains only.
- [ ] Commit: `Add the client item book and inventory`.

### Task 14: Client — ground items and the pickup loop

**Files:** `core/entity_table.gd`, `core/pickup_loop.gd` (new), `tests/unit/test_core.gd`.

**Interface:**
- `entity_table.gd`: `enter` stores ITEM entries with `item`; `ground_items() -> Array` of `{id, pos, item}`; ITEM entries never appear in `live_monsters()`.
- `pickup_loop.gd`: `func start(item_id: int, all := false)`, `func step(now, self_pos, items: Array, manual: bool) -> Dictionary` (`{"dir": Vector2, "send": int or 0}`), `func on_reply(code) -> String` (notice key or ""), `func cancel()`. Constants `APPROACH := 1.2`, `ALL_RADIUS := 6.0`.

**Behaviour (§6.1):** walk until within `APPROACH`, send one `PickupReq`, wait for the reply; in "all" mode pick the next nearest own item within `ALL_RADIUS` after each success; a manual move cancels; the target leaving the table cancels (or moves on in "all" mode); OUT_OF_RANGE walks straight and retries; BAG_FULL stops with the notice.

**TODO / tests** (`# ____TEST GROUND ITEMS____`, `# ____TEST PICKUP LOOP____`):
- [ ] `entity_enter_item` fixture → a ground item entry, not a monster candidate.
- [ ] `step` walks toward an item 3 tiles away and sends once within 1.2; no second send while waiting.
- [ ] "All" mode moves to the next item after a success; stops when none is within 6 tiles.
- [ ] Manual move cancels; BAG_FULL stops and returns its notice.
- [ ] Commit: `Track ground items and add the pickup loop`.

### Task 15: Client — bag and equipment tabs, tooltip, pick button, Xu text

**Files:** `ui/bag_tab.gd`, `ui/equip_tab.gd`, `ui/item_tooltip.gd`, `ui/ground_item_view.gd` (new), `ui/char_window.gd`, `ui/hud.gd`, `screens/world.gd`, `screens/world_entities.gd`.

**Behaviour (§6.2):**
- `char_window.gd`: the Trang bị and Hành trang pages become `equip_tab` and `bag_tab`; public like the other tabs; signals `equip(bag_slot)`, `unequip(equip_slot)`, `discard(bag_slot)`, `sort()`; the caller answers with `applied(update)` / `failed(code)`.
- `item_tooltip.gd`: the §6.2 layout; buttons passed in by the tab (Mặc/Hủy or Tháo); the discard confirmation dialog.
- `ground_item_view.gd`: diamond in the quality colour, short name, count.
- `hud.gd`: a small round "Nhặt" button above Đổi mục tiêu (signal `pick_pressed`); `show_xu_gain(n)` floating "+N Xu" over the own character.
- `world.gd`: inventory from `EnterWorldResp`; `_pick` checks ground items (0.6 tiles) before monsters and starts `pickup_loop`; the Nhặt button starts "all" mode; send the requests, apply responses, show error notices; feed `SelfStats.xu` to the inventory and the Xu text.
- `world_entities.gd`: ITEM entries get a `ground_item_view`.

**TODO / tests:**
- [ ] Unit tests stay green; no Node code in `core/`.
- [ ] Manual check against a :8299 server with a `devitem`-stocked level-90 character: tabs show the bag and worn slots; wear/take off/discard/sort work; stats change in the Thông tin tab; killing monsters shows "+N Xu" and drops; tapping a drop and the Nhặt button pick it up. Screenshots in the report.
- [ ] Commit: `Add the bag and equipment tabs, item tooltips and ground item pickup`.

### Task 16: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `tools/smoke.sh`, `README.md`.

**Behaviour (§6.3):** after the skill step, fight up to 20 monsters; assert Xu rose after the first kill; pick up the first drop seen and find it in the inventory; print `SMOKE OK (… xu +N, picked <name> | no drop)`. Keep the watchdog.

**TODO:**
- [ ] Run against a :8299 server built from server HEAD; paste the OK line in the task report.
- [ ] README: the bag and equipment tabs, the Nhặt button, tapping drops, `devitem`.
- [ ] Commit: `Extend the smoke test with Xu and picking up a drop`.

---

## Acceptance (controller, after Task 16)

- [ ] Server `go test -race -count=1 ./...` with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart the :8199 server from HEAD (kill only its recorded PID). With `test1` and `test2` offline, give each a few Xích Cương pieces with `devitem` (one Vàng, one Cam) and some Xu.
- [ ] Tell the user how to open two windows on :8199 and what to try: kill monsters for Xu and Lữ Hành drops, pick up, open Trang bị/Hành trang, wear the bậc 50 set and watch the stats.
