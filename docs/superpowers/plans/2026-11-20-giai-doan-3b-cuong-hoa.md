# Phase 3b Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inside a safe zone, players enhance any equipment piece (worn or in the bag) from +0 to +15 with stones and Xu, optionally protected from level drops by a charm from target +8, with the hidden +1% per failure. Worn items at +7/+10/+12/+15 regenerate HP (armour) or MP (weapon and jewellery) every second, and a worn weapon at +7 or more glows for everyone around.

**Architecture:**
- `item_stats.py` adds the enhancement table, the regen milestones, a `regen` kind per slot and the `charm_protect` stackable to `data/items.json`; `gamedata` validates them.
- `internal/items` gains the pure enhancement attempt loop (materials, hidden pity, drops) and the regen sum. Pity is a 15-entry array per item, persisted in a new `character_items.pity` column and never sent to clients.
- The zone handles `EnhanceReq` (safe-zone check, up to 100 attempts per request), regenerates HP/MP once a second with a carry for fractional HP, and emits `LookChange` when the worn weapon's enhancement changes.
- Protocol 7: `EnhanceReq/Resp` (71–72), `ItemRef`, `LookChange`, `EntityEnter.weapon_enhance`, `WorldTick.looks`, two `SelfStats` regen fields, four error codes.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-3b-cuong-hoa-design.md](../specs/2026-11-20-giai-doan-3b-cuong-hoa-design.md) (§n). It builds on [the 3a spec](../specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) (3a §n).

**Prerequisite:** the [3a plan](2026-11-20-giai-doan-3a-tui-do-roi-do.md) is fully implemented and accepted. Before Task 1, re-verify the findings below against HEAD (`graphify explain` each symbol) and note any renames in the task report.

**Plan format, commit rules, test rules, environment:** as in the 3a plan (interfaces/behaviour/tests/TODOs only; commit and push after every task, no trailer, `git add` specific paths, `git pull --rebase` before push; `-race`, 1–2 test files per package with section dividers, staticcheck, vet, `graphify update .`; never touch :8099 or the controller's :8199; own runs on :8299, kill only your PID; never print DB passwords).

---

## Verified findings (2026-11-20; symbols the 3a plan creates are marked "3a")

- 3a: `data/items.json` from `docs/balance/item_stats.py`; `gamedata.Items` with `Slots`, `Stackables`, `StackOrder`, `Equipment` (arrays indexed by enhance level 0–15, `gamedata.EnhanceLevels = 16`).
- 3a: `items.Item{Code, Count, Quality, Element, Enhance, Lines, Hidden}`, `items.Inventory{Bag, Equip}`, `Inventory.Add`, `Wearable`, `GearBonus` (already reads stats at `it.Enhance`), sentinel errors mapped to codes in the zone.
- 3a: `zone.gearChanged(p)` (recompute + HP ratio rule + `hpChanged` + `statsDirty`), `itemInfo`, `inventoryUpdate`, `internal/zone/inventory.go` holds the item commands; `store.ItemRow` and `saveState` delete+insert of item rows; `world.knownItems` validates rows.
- `z.area.InSafe(p.pos)` (`internal/zone/area.go`, `InSafe`) is the safe-zone test the attacks use; the map's spawn point (5.5, 5.5) is inside the `camp` safe zone of `maps/kumotori_1.json` (x 1–10, y 1–10), so a freshly entered character can enhance at once.
- `runPlayerTimers` (`internal/zone/lifecycle.go`) runs once per tick over all players (respawn, rest heal); add the per-second regen there. `z.ticks(1)` is the ticks per second.
- `enterEnv(e *entity)` (`internal/zone/entity.go:30`) builds `EntityEnter` from the shared entity fields; players need an extra field, so pass the weapon level in (or give `entity` a `weaponEnhance int` set by the zone).
- `tickEvents` (`internal/zone/combat.go`) and `addEvents` filter events by visibility; add `looks` the same way as `levelUps`.
- Proto after 3a: Envelope ends at `sort_bag_resp = 70`; `ErrorCode` ends at `WRONG_CLASS = 29`; `EntityEnter` uses 1–13; `WorldTick` 1–8; `SelfStats` 1–21; `ProtocolVersion = 6`.
- Client after 3a: `core/item_book.gd`, `core/inventory.gd`, `ui/item_tooltip.gd`, `ui/equip_tab.gd`, `ui/bag_tab.gd`; `ui/entity_view.gd` draws players; `ui/info_tab.gd` lists derived stats; the client knows the safe zones from the map data (it already shows the IN_SAFE_ZONE notice in `screens/world.gd`).

## File structure

Server:

```
docs/balance/item_stats.py, data/items.json                      enhance, regen, charm (Task 1)
internal/gamedata/items.go, gamedata_test.go                      parse + validate (Task 1)
internal/items/enhance.go (new), items.go, items_test.go          attempts, pity, regen sum (Task 2)
proto/ds/v1/protocol.proto, internal/pb, gateway, testdata/client protocol 7 (Task 3)
migrations/00005_enhance.sql (new), internal/store/characters.go, store_test.go, internal/world/world.go   pity (Task 4)
internal/zone/forge.go (new), inventory.go, zone.go               EnhanceReq (Task 5)
internal/zone/lifecycle.go, progression.go                        regen (Task 6)
internal/zone/entity.go, combat.go, inventory.go                  weapon glow (Task 7)
internal/client, internal/gateway/handlers.go, cmd/devitem, internal/store/dev.go, e2e   wiring, devitem -enhance, e2e (Task 8)
docs/game-design.md, CLAUDE.md, README.md                         docs (Task 9)
```

Client:

```
server/, data/items.json, proto/protocol.gd, tests/fixtures, autoload/net.gd, core/wire.gd, core/errors.gd   sync, protocol 7 (Task 10)
core/forge_rules.gd (new), core/entity_table.gd                   rules, looks (Task 11)
ui/forge_window.gd (new), ui/item_tooltip.gd, ui/equip_tab.gd, ui/bag_tab.gd, ui/info_tab.gd, ui/entity_view.gd, screens/world.gd   UI (Task 12)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                   smoke (Task 13)
```

---

### Task 1: Server — enhancement and regen data

**Files:** `docs/balance/item_stats.py`, `data/items.json`, `internal/gamedata/items.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `gamedata.Items` gains `Enhance EnhanceTable{PityPct, MaxTimes int; Levels []EnhanceLevel}` (index 0 = target +1), `EnhanceLevel{Level, SuccessPct int; Drop bool; Stones, XuPerTier int}`, `Regen []RegenStep{Enhance, HPBP, MP int}`; `Slot.Regen string` (`"hp"`/`"mp"`); constant `CharmProtect = "charm_protect"`, `StoneEnhance = "stone_enhance"`.

**Behaviour (§2.1):** the script writes the table, regen steps, slot `regen` and the charm, and prints the §2.1 mean-cost table from a seeded simulation (2,000 runs for +15, more for the others); it asserts the +15 no-charm mean attempts in 5,500–7,500. Validation per §2.1 "Server kiểm tra khi nạp".

**TODO / tests** (`// ____TEST ENHANCE DATA____`):
- [ ] Committed data parses; `Levels[14]` is `{15, 10, true, 5, 300}`; weapon slot regen is `mp`, boots `hp`.
- [ ] Reject: 14 levels; rising `success_pct`; `drop` at +3; `drop` with 100%; falling `xu_per_tier`; regen steps out of order; a slot without `regen`; missing charm.
- [ ] The generated-data compare test stays green.
- [ ] Commit: `Add the enhancement table and regen milestones to the item data`.

### Task 2: Server — enhancement attempts and the regen sum

**Files:** `internal/items/enhance.go` (new), `internal/items/items.go`, `internal/items/items_test.go`.

**Interface:**
- `Item` gains `Pity [15]int` (percentage points per target level; never sent to clients). `Clone` copies it (array, so by value).
- `type Ref struct { Equipped bool; Slot int }`; `func (inv *Inventory) At(r Ref) (*Item, error)` (`ErrBadSlot`, `ErrNotFound`).
- `func (inv *Inventory) Count(code string) int`; `func (inv *Inventory) Take(code string, n int) (changed []int)` — lowest slots first; caller checks `Count` first.
- `type EnhanceResult struct { Attempts int; Success bool; Level int; Changed []int }`.
- `func (inv *Inventory) Enhance(rng *rand.Rand, d *gamedata.Items, r Ref, protect bool, times int, xu *int64) (EnhanceResult, error)` — errors in §4.1 order from step 3 on: `ErrBadSlot` (bad ref) or `ErrBadCount` (`times` < 0 or above `max_times`; 0 means 1), `ErrNotFound`, `ErrNotEquipment`, `ErrMaxed`, `ErrNoMaterials`, `ErrNoXu`. Steps 1–2 (DEAD, safe zone) stay in the zone.
- `func Milestone(d *gamedata.Items, enhance int) (RegenStep, bool)`; `func Regen(d *gamedata.Items, inv *Inventory, level int, class string) (hpBP, mp int)` — wearable worn items only.

**Behaviour (§4.1–4.2):** attempt loop exactly as §4.1, one `rng.IntN(100)` per attempt; pity capped at `100 − success_pct`; the charm is taken only when `protect` and the target level has `drop`; the loop continues only after a failure that keeps the level and while the next attempt is affordable.

**TODO / tests** (`// ____TEST ENHANCE____`, `// ____TEST REGEN____`):
- [ ] +0→+1 always succeeds and costs 1 stone and `10 × tier` Xu.
- [ ] Seeded failure at +5: pity[4] becomes 1, level stays 4; at +8 without charm: level drops to 7, pity[7] = 1 and is kept; a later success at +8 resets pity[7] to 0 only.
- [ ] Pity for +15 never exceeds 90; with pity 90 the next +15 attempt succeeds whatever the roll (use a rng that always returns 99).
- [ ] Charm: not taken at target +7 even with `protect`; taken at +8; a protected failure keeps the level.
- [ ] `times 100` stops at the first success, at a drop, when stones/charms/Xu run out (attempts reported), or after 100 attempts.
- [ ] Errors in order; an error changes nothing (bag, Xu, item).
- [ ] Stones are taken from the lowest slots first and `Changed` lists them and the item's slot.
- [ ] `Milestone`: +6 none, +7 → (5 bp, 1 MP), +11 → +10 step, +15 → (30 bp, 5 MP). `Regen` of ten +15 wearable pieces = (150, 25); an unwearable piece adds nothing.
- [ ] Commit: `Add enhancement attempts with hidden pity and the regen sum`.

### Task 3: Server — protocol 7

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/`, `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 71–72; `ItemRef`; `LookChange`; `EntityEnter.weapon_enhance = 14`; `WorldTick.looks = 9` with the updated apply-order comment; `SelfStats.hp_regen_bp = 22`, `mp_regen = 23`; error codes 30–33; `ProtocolVersion = 7`.

**TODO / tests:**
- [ ] `make proto`; godobuf-safe names.
- [ ] Fixtures `enhance_req`, `enhance_resp`, `world_tick_look`, `entity_enter_glow`; regenerate all; round-trip `world_tick_look`.
- [ ] Version test → 7.
- [ ] Commit: `Add the enhancement messages and bump the protocol to 7`.

### Task 4: Server — persist pity

**Files:** `migrations/00005_enhance.sql` (new), `internal/store/characters.go`, `internal/store/store_test.go`, `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/zone.go` (row conversion).

**Behaviour (§3):** `store.ItemRow.Pity []int`; empty means all zero; saved as `smallint[]`. `world.knownItems` resets a bad array (length not 0 or 15, value outside 0–99) to zero with one warning and keeps the item. The zone converts `[15]int` ↔ rows (all-zero saves as empty).

**TODO / tests:**
- [ ] DB: pity round-trips; a 3a-era row (default `'{}'`) loads as zeros.
- [ ] World: a 14-entry pity is reset and logged, the item kept.
- [ ] Commit: `Persist the hidden enhancement pity per item`.

### Task 5: Server — EnhanceReq in the zone

**Files:** `internal/zone/forge.go` (new), `internal/zone/inventory.go`, `internal/zone/zone.go`, `internal/world/world.go` (Membership), tests.

**Interface:** `zone.(*Zone).Enhance(id uint64, seq uint32, ref items.Ref, protect bool, times int) bool` + `enhanceCmd`; `Membership.Enhance(...)`.

**Behaviour (§4.1):** DEAD → NOT_IN_SAFE_ZONE (`!z.area.InSafe(p.pos)`) → the item package's checks mapped to codes (`ErrMaxed` → ENHANCE_MAXED, `ErrNoMaterials` → NOT_ENOUGH_MATERIALS, `ErrNoXu` → NOT_ENOUGH_XU, the rest as in 3a). Accept: if the item is worn, `gearChanged(p)`; save at once; reply `EnhanceResp{attempts, success, level, update, stats}` with `update` from `Changed` and `stats = selfStats(p)`.

**TODO / tests** (`// ____TEST ENHANCE ZONE____`):
- [ ] Error order: dead outside the safe zone → DEAD; alive outside → NOT_IN_SAFE_ZONE even with a bad slot; then BAD_REQUEST, ITEM_NOT_FOUND, BAD_REQUEST (stone), ENHANCE_MAXED, NOT_ENOUGH_MATERIALS (no charm with `protect` at +8), NOT_ENOUGH_XU.
- [ ] A worn armour going +0→+1 raises max HP per the data and applies the 3a HP rule; a bag item does not change stats.
- [ ] Xu in the reply drops by the cost; a save is queued.
- [ ] Commit: `Let players enhance equipment in safe zones`.

### Task 6: Server — regen every second

**Files:** `internal/zone/lifecycle.go`, `internal/zone/progression.go`, `internal/zone/zone.go`, tests.

**Interface:** `player` gains `regenHPBP, regenMP, hpCarry int`; `recompute` sets the two sums from `items.Regen`; `selfStats` fills `hp_regen_bp`, `mp_regen`.

**Behaviour (§4.2):** in `runPlayerTimers`, on ticks where `tickNo % z.ticks(1) == 0`, for living players: the HP carry rule, MP add, caps, `hpChanged`/`statsDirty` only on change; full HP resets the carry; the dead skip.

**TODO / tests** (`// ____TEST REGEN ZONE____`):
- [ ] 1,640 max HP, one worn +7 boots (5 bp), HP at 1,000: after 1 s +0, after 2 s +1, after 10 s +8.
- [ ] Ten worn +15 pieces: +1.5% max HP (floored with carry) and +25 MP per second; never above max.
- [ ] A dead player regenerates nothing; regen keeps running while the player is being hit.
- [ ] `SelfStats` shows the sums.
- [ ] Commit: `Regenerate HP and MP every second from enhancement milestones`.

### Task 7: Server — weapon glow

**Files:** `internal/zone/entity.go`, `internal/zone/combat.go`, `internal/zone/inventory.go`, `internal/zone/forge.go`, `internal/zone/progression.go`, tests.

**Behaviour (§4.3):** a player's `weapon_enhance` is the enhancement of the worn weapon when wearable, else 0; it fills `EntityEnter.weapon_enhance`. Whenever `recompute` changes it (equip/unequip, enhancing the worn weapon, a level-up that makes it wearable), append `LookChange{id, weapon_enhance}` to the tick events; `addEvents` sends it to viewers of the player and to the player.

**TODO / tests** (`// ____TEST GLOW____`):
- [ ] A viewer entering view of a player with a worn +12 weapon gets `weapon_enhance 12`.
- [ ] Equipping a +10 weapon emits one `LookChange` seen by a nearby viewer and the player; a viewer out of view gets none.
- [ ] Enhancing the worn weapon +6→+7 emits it; enhancing a worn helmet emits none.
- [ ] Commit: `Show the worn weapon's enhancement to everyone around`.

### Task 8: Server — gateway, client helper, devitem -enhance, e2e

**Files:** `internal/gateway/handlers.go`, `internal/client/client.go`, `internal/store/dev.go`, `cmd/devitem/main.go`, `internal/store/store_test.go`, `e2e/e2e_test.go`, `README.md`.

**Behaviour:** `EnhanceReq` like the 3a item requests (`inWorld`, `zoneBusyIfDropped`, packet limit only); `client.Enhance(ctx, ref, protect, times) (*pb.EnhanceResp, error)`; `devitem -enhance N` (0–15, equipment only, else an error) and `-code charm_protect`.

**TODO / tests:**
- [ ] DB: `devitem -enhance 9` stores enhance 9.
- [ ] e2e `TestEnhanceOverWebSocket` per §8.
- [ ] README: `-enhance` and the charm code.
- [ ] Commit: `Wire enhancement through the gateway and add an end-to-end test`.

### Task 9: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`.

**TODO:**
- [ ] game-design 3.3 and 2.7 per §9; CLAUDE.md per §9; Vietnamese prose.
- [ ] Commit: `Document enhancement costs, charms and regen`.

### Task 10: Client — sync and protocol 7

**Files:** `server`, `data/items.json`, `proto/protocol.gd`, `tests/fixtures/`, `autoload/net.gd`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_net.gd`.

**TODO / tests:**
- [ ] Submodule to server HEAD after Task 9; `PROTOCOL_VERSION := 7`; `PAYLOADS` adds `enhance_req`, `enhance_resp`; error texts per §6.1.
- [ ] Decode the four new fixtures; `tests/run.sh` green with `--check`.
- [ ] Commit: `Sync the enhancement protocol and data from the server`.

### Task 11: Client — forge rules and looks

**Files:** `core/forge_rules.gd` (new), `core/entity_table.gd`, `tests/unit/test_core.gd`.

**Interface:** per §6.1: `enhance_cost(item) -> Dictionary`, `enhance_block(item, inventory, xu, in_safe, protect) -> String`, `glow_tier(enhance) -> int`, `regen_of(item) -> Dictionary`; `entity_table` keeps `weapon_enhance` and returns `{"type": "look", ...}` effects.

**TODO / tests** (`# ____TEST FORGE RULES____`, `# ____TEST LOOKS____`):
- [ ] Cost of a tier-50 item at +14 → `{stones 5, xu 15000, success_pct 10, drop true, charm_usable true}`; at +4 → `charm_usable false`.
- [ ] Block order: outside the safe zone "safe" before "maxed"; then "stones", "charm" (only with protect at a drop level), "xu".
- [ ] `glow_tier`: 6→0, 7→1, 9→1, 10→2, 12→3, 15→4.
- [ ] `world_tick_look` fixture updates the entry and returns a look effect.
- [ ] Commit: `Add the client forge rules and weapon look updates`.

### Task 12: Client — forge window, item and info displays, glow

**Files:** `ui/forge_window.gd` (new), `ui/item_tooltip.gd`, `ui/equip_tab.gd`, `ui/bag_tab.gd`, `ui/info_tab.gd`, `ui/entity_view.gd`, `screens/world.gd`, `screens/world_entities.gd`.

**Behaviour (§6.2):** the forge window with one tab (built so 3c can add two more); tooltip "+n", milestone line and the Cường hóa button; "+n" on slot cards; regen lines in the info tab; the glow ring in `entity_view` from `glow_tier`; `world.gd` sends `EnhanceReq`, applies the response (inventory update, stats, result line) and look effects.

**TODO / tests:**
- [ ] Unit tests green; `core/` Node-free.
- [ ] Manual check on :8299 with a `devitem`-stocked character: enhance in the camp, refused outside it, Liên tục, charm checkbox only from +8, glow colours at +7/+10/+12/+15 seen from a second window, regen numbers in Thông tin. Screenshots in the report.
- [ ] Commit: `Add the forge window, enhancement displays and weapon glow`.

### Task 13: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `tools/smoke.sh`, `README.md`.

**Behaviour (§6.3):** `smoke.sh` runs `go run ./cmd/devitem` in `server/` for the smoke account after the 3a step (weapon of the smoke class, 10 stones, 1,000 Xu); the smoke logs in again and enhances the weapon to +1; prints `SMOKE OK (… enhance +1, xu −100)`.

**TODO:**
- [ ] Live run on :8299; paste the OK line.
- [ ] README: the forge window and enhancement controls.
- [ ] Commit: `Extend the smoke test with an enhancement`.

---

## Acceptance (controller, after Task 13)

- [ ] Server full `-race` suite with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart :8199 from HEAD (own recorded PID only). With `test1`/`test2` offline: `devitem` 2,000 stones, 100 charms, 5,000,000 Xu each.
- [ ] Ask the user to enhance worn gear in the camp to +7/+10/+15, watch regen in Thông tin, and see each other's weapon glow in two windows.
