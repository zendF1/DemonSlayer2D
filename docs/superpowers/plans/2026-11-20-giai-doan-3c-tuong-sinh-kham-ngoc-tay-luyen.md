# Phase 3c Resonance, Gems and Refining Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The hidden lines every Vàng/Cam/Đỏ item has carried since 3a start working when the right activating slots wear items of the generating element. Inside a safe zone, players punch up to three holes, socket and unsocket attack/HP/defense gems, and refine the sub-lines of Vàng+ items with locks, previewing the result before keeping or taking it.

**Architecture:**
- `item_stats.py` adds the resonance table (activator element per element, two activating slots per slot), socket and refine costs, and 15 gems as stackables with a `gem` block to `data/items.json`; `gamedata` validates them.
- `internal/items` gains hidden-line activation (in `GearBonus`), gem stats, punch/socket/unsocket and the refine roll. Gems are stored per item as a `gems text[]` column whose length is the hole count.
- The zone handles four new forge commands in safe zones and keeps one pending refine result per player in memory, cleared by any other item command except pickup and by leaving.
- Protocol 8: Envelope 73–82, `ItemInfo.holes`/`gems`, four error codes. The client computes which hidden lines are lit for display from the same table.

**Tech stack:** Go, protobuf (buf), Postgres (goose), Python 3; Godot 4.7.2, GDScript, godobuf.

**Spec:** [docs/superpowers/specs/2026-11-20-giai-doan-3c-tuong-sinh-kham-ngoc-tay-luyen-design.md](../specs/2026-11-20-giai-doan-3c-tuong-sinh-kham-ngoc-tay-luyen-design.md) (§n). Builds on the [3a](../specs/2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) and [3b](../specs/2026-11-20-giai-doan-3b-cuong-hoa-design.md) specs.

**Prerequisite:** the [3b plan](2026-11-20-giai-doan-3b-cuong-hoa.md) is fully implemented and accepted. Before Task 1, re-verify the findings below against HEAD and note renames in the task report.

**Plan format, commit rules, test rules, environment:** as in the 3a plan.

---

## Verified findings (2026-11-20; "3a"/"3b" mark symbols those plans create)

- 3a: items carry `Element` and two `Hidden` lines from the drop roll (`items.NewEquipment`); `GearBonus` skips hidden lines through a named hook — replace it here. 3a's line roll helper (distinct stats, `[ceil(S/2), S]`) is reused for refining.
- 3a/3b: `Inventory.At(Ref)`, `Count`, `Take`, `Add` (all-or-nothing, merges stacks), sentinel errors mapped to codes in `internal/zone/inventory.go`; `zone.gearChanged(p)`; the safe-zone check in `internal/zone/forge.go` (3b) — reuse its order DEAD → NOT_IN_SAFE_ZONE.
- 3a: stackables are ordered by `items.json` order for sorting (`StackOrder`); gems appended there keep sorting stable.
- Resonance maximum verified by a scratch brute force over all 5^10 element assignments with the §2.1 tables: the maximum is exactly 13 lit lines, reached by 10 assignments (the five rotations of the cycle × 2), one of them the §4.1 set.
- Proto after 3b: Envelope ends at `enhance_resp = 72`; `ErrorCode` ends at `NOT_IN_SAFE_ZONE = 33`; `ItemInfo` uses 1–7; `ProtocolVersion = 7`.
- Client after 3b: `core/forge_rules.gd`, `ui/forge_window.gd` (one tab, built for more), `ui/item_tooltip.gd` shows hidden lines grey with the 3a "có ở bản sau" note — replace it.

## File structure

Server:

```
docs/balance/item_stats.py, data/items.json                      resonance, sockets, refine, gems (Task 1)
internal/gamedata/items.go, gamedata_test.go                      parse + validate (Task 1)
internal/items/resonance.go, sockets.go, refine.go (new), items.go, items_test.go   rules (Tasks 2-4)
proto/ds/v1/protocol.proto, internal/pb, gateway, testdata/client protocol 8 (Task 5)
migrations/00006_sockets.sql (new), internal/store/characters.go, store_test.go, internal/world/world.go   gems column (Task 6)
internal/zone/forge.go, inventory.go, zone.go                     punch, socket, unsocket, refine, pending (Tasks 7, 8)
internal/client, internal/gateway/handlers.go, cmd/devitem, internal/store/dev.go, e2e   wiring, devitem -holes, e2e (Task 9)
docs/game-design.md, CLAUDE.md, README.md                         docs (Task 10)
```

Client:

```
server/, data/items.json, proto/protocol.gd, tests/fixtures, autoload/net.gd, core/wire.gd, core/errors.gd   sync, protocol 8 (Task 11)
core/resonance.gd (new), core/forge_rules.gd, core/item_book.gd, core/inventory.gd   rules (Task 12)
ui/forge_window.gd, ui/item_tooltip.gd, ui/equip_tab.gd, ui/bag_tab.gd, screens/world.gd   UI (Task 13)
tests/smoke/smoke.gd, tools/smoke.sh, README.md                   smoke (Task 14)
```

---

### Task 1: Server — resonance, socket, refine and gem data

**Files:** `docs/balance/item_stats.py`, `data/items.json`, `internal/gamedata/items.go`, `internal/gamedata/gamedata_test.go`.

**Interface:**
- `gamedata.Items` gains `Resonance{Activator map[string]string; Slots [10][2]int}` (slot indices after parse), `Sockets{Max int; PunchXuPerTier []int; UnsocketXuPerLevel int}`, `Refine{MinQuality, Stones, StonesPerLock, XuPerTier, XuPerTierPerLock int}`; `Stackable.Gem *Gem` with `Gem{Kind string; Level int; Stats map[string]int}`; `(*Items).IsGem(code) bool`.

**Behaviour (§2.1):** the script emits the tables and the 15 gems with values from the 3a `line_standard` of tier `10 × level`; validation per §2.1 "Server kiểm tra khi nạp" (the activator must be a single 5-cycle without fixed points).

**TODO / tests** (`// ____TEST FORGE DATA____`):
- [ ] Committed data parses; `gem_hp_5` gives 469 HP; `Resonance.Slots[weapon]` = (helmet, armor).
- [ ] Reject: `activator` with `viem → viem`; two cycles (a 2-cycle plus a 3-cycle); a slot activating itself; a missing slot; `max` 3 with two punch prices; duplicate (kind, level); a gem stat `mp`.
- [ ] Commit: `Add resonance, socket, refine and gem data`.

### Task 2: Server — hidden-line activation and gem stats

**Files:** `internal/items/resonance.go` (new), `internal/items/items.go`, `internal/items/items_test.go`.

**Interface:**
- `Item` gains `Gems []string` (length = holes; `""` empty); `Clone` copies it.
- `func HiddenActive(d *gamedata.Items, inv *Inventory, level int, class string) [10][2]bool` — §4.1.
- `GearBonus` adds lit hidden lines and every socketed gem of wearable worn items.

**TODO / tests** (`// ____TEST RESONANCE____`):
- [ ] Viêm sword in the weapon slot with a Phong helmet lights line 1, a Phong armour line 2; a Phong helmet below its level lights nothing; a Xanh helmet (no element) lights nothing.
- [ ] Brute force over all 5^10 assignments (every slot a Vàng item of a wearable level): the maximum lit count is 13 and the §4.1 set reaches it. Use int arrays and a precomputed pair list; it must finish under 5 s with `-race` (measure; if slower, enumerate with an incremental odometer that recounts only the pairs touching the changed slot).
- [ ] `GearBonus` with two lit HP hidden lines of a tier-10 Cam item adds 216 HP; with gems `gem_attack_1` and `gem_defense_1` adds 4 to both attacks and 1 to both defenses.
- [ ] Commit: `Activate hidden lines by resonance and count socketed gems`.

### Task 3: Server — punch, socket, unsocket

**Files:** `internal/items/sockets.go` (new), `internal/items/items_test.go`.

**Interface:**
- `func (inv *Inventory) Punch(d *gamedata.Items, r Ref, xu *int64) (changed []int, err error)`.
- `func (inv *Inventory) Socket(d *gamedata.Items, r Ref, hole, gemSlot int) (changed []int, err error)`.
- `func (inv *Inventory) Unsocket(d *gamedata.Items, r Ref, hole int, xu *int64) (changed []int, err error)`.
- New errors `ErrHolesMaxed`, `ErrGemTooHigh` (mapped to HOLES_MAXED, GEM_TOO_HIGH).

**Behaviour (§4.2–4.4):** check orders from step 3 on (the zone does DEAD and the safe zone); an error changes nothing; unsocket checks bag room with the 3a add rule before charging Xu.

**TODO / tests** (`// ____TEST SOCKETS____`):
- [ ] Punch costs 100/200/400 × tier for holes 1–3; the fourth gives `ErrHolesMaxed`; no Xu → `ErrNoXu` and no hole.
- [ ] Socket consumes one gem from a stack of 3 (leaves 2); a filled hole, a hole index equal to the count, a stone in `gemSlot` → bad request; `gem_attack_3` into a tier-20 item → `ErrGemTooHigh`.
- [ ] Unsocket returns the gem merging into an existing stack; a full bag with no matching stack → `ErrBagFull` and nothing changes; costs 500 × gem level.
- [ ] Commit: `Punch holes and socket and unsocket gems`.

### Task 4: Server — refine roll

**Files:** `internal/items/refine.go` (new), `internal/items/items_test.go`.

**Interface:**
- `func (inv *Inventory) RefineRoll(rng *rand.Rand, d *gamedata.Items, r Ref, locked []int, xu *int64) (lines []Line, changed []int, err error)` — checks §4.5 steps 3–9 (`ErrNotRefinable` for quality below `min_quality`), pays, returns the candidate set without touching the item.
- `func (inv *Inventory) RefineApply(r Ref, lines []Line) error`.

**Behaviour (§4.5):** locked lines keep stat, value and index; each unlocked index draws a stat uniformly from those not in the set (locked lines and already-drawn new lines), value as the 3a roll; line count unchanged.

**TODO / tests** (`// ____TEST REFINE____`):
- [ ] Over 10,000 seeded rolls of a 4-line Cam item with lines 0 and 2 locked: those lines never change; the set never repeats a stat; values within range.
- [ ] Locking every line, an out-of-range or duplicate index → bad request; a Xanh item → `ErrNotRefinable`.
- [ ] Cost with 2 locks on tier 30: 3 stones, `(20 + 40) × 30 = 1,800` Xu.
- [ ] Element, hidden lines, gems, enhance and quality are unchanged by roll and apply.
- [ ] Commit: `Roll refined sub-lines with locks`.

### Task 5: Server — protocol 8

**Files:** `proto/ds/v1/protocol.proto`, `internal/pb/`, `internal/gateway/server.go`, `internal/gateway/gateway_test.go`, `testdata/client/*.hex`.

**Behaviour (§5):** Envelope 73–82; `ItemInfo.holes = 8`, `gems = 9`; error codes 34–37; `ProtocolVersion = 8`.

**TODO / tests:**
- [ ] `make proto`; godobuf-safe names (`locked`, `accept`, `hole`, `gem_slot` are fine).
- [ ] Fixtures `item_with_gems`, `refine_resp`; regenerate all; round-trip `item_with_gems`.
- [ ] Version test → 8.
- [ ] Commit: `Add the socket and refine messages and bump the protocol to 8`.

### Task 6: Server — persist gems

**Files:** `migrations/00006_sockets.sql` (new), `internal/store/characters.go`, `internal/store/store_test.go`, `internal/world/world.go`, `internal/world/world_test.go`, `internal/zone/zone.go`.

**Behaviour (§3):** `store.ItemRow.Gems []string` ↔ `text[]`; `world.knownItems` truncates long arrays, empties unknown or too-high gems, strips holes from stackables, one warning per fix; the zone sends `holes`/`gems` in `ItemInfo`.

**TODO / tests:**
- [ ] DB round-trip of gems with an empty hole in the middle; a 3b-era row loads with no holes.
- [ ] World: four holes → three; `gem_hp_9` → empty hole; logged.
- [ ] Commit: `Persist holes and socketed gems`.

### Task 7: Server — forge commands in the zone

**Files:** `internal/zone/forge.go`, `internal/zone/inventory.go`, `internal/zone/zone.go`, `internal/world/world.go` (Membership), `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:** zone methods + commands `Punch(id, seq, ref)`, `Socket(id, seq, ref, hole, gemSlot)`, `Unsocket(id, seq, ref, hole)`; `Membership` wrappers; gateway cases (packet limit only); client helpers.

**Behaviour (§4.2–4.4):** DEAD → NOT_IN_SAFE_ZONE → the item package's checks; on success `gearChanged(p)` when the item is worn (resonance can change other items' lit lines too — `recompute` covers it), save at once, reply with update and stats.

**TODO / tests** (`// ____TEST FORGE ZONE____`):
- [ ] Error order per command, with DEAD and NOT_IN_SAFE_ZONE first.
- [ ] Socketing an HP gem into the worn armour raises max HP by the gem value and records an `HpChange`.
- [ ] Wearing a Phong helmet lights the worn Viêm sword's line 1 (stats rise by its value); taking the helmet off removes it.
- [ ] Commit: `Let players punch, socket and unsocket in safe zones`.

### Task 8: Server — refine with a pending result

**Files:** `internal/zone/forge.go`, `internal/zone/inventory.go`, `internal/zone/zone.go`, `internal/world/world.go`, `internal/gateway/handlers.go`, `internal/client/client.go`, tests.

**Interface:** `player.refine *pendingRefine{ref items.Ref; lines []items.Line}`; zone commands `Refine(id, seq, ref, locked)`, `RefineChoose(id, seq, accept)`; wrappers, gateway cases, client helpers.

**Behaviour (§4.5):** `Refine` checks DEAD → NOT_IN_SAFE_ZONE → `RefineRoll` (`ErrNotRefinable` → NOT_REFINABLE, `ErrNoMaterials` → NOT_ENOUGH_MATERIALS, `ErrNoXu` → NOT_ENOUGH_XU, the rest as in 3a); on success store the pending result (replacing any older one), save at once, reply `RefineResp{lines, update, stats}`. `RefineChoose`: NO_PENDING_REFINE when none; accept → `RefineApply`, `gearChanged` if worn; clear; save; reply. Every other item command except Pickup and RefineChoose clears the pending result before running (put the clear in one place, e.g. the item-command dispatcher). Leaving drops it with the player.

**TODO / tests** (`// ____TEST REFINE ZONE____`):
- [ ] Accept replaces the lines; decline keeps them; both clear the pending result (a second choose → NO_PENDING_REFINE).
- [ ] A refine followed by an equip, sort or enhance clears the pending result; a pickup in between does not.
- [ ] A second refine replaces the first pending result (choose applies the second).
- [ ] Stones and Xu are spent at the roll even when declined; a save is queued at the roll.
- [ ] Commit: `Refine sub-lines with a preview and a keep-or-take choice`.

### Task 9: Server — devitem -holes and the forge e2e

**Files:** `internal/store/dev.go`, `cmd/devitem/main.go`, `internal/store/store_test.go`, `e2e/e2e_test.go`, `README.md`.

**TODO / tests:**
- [ ] `devitem -holes N` (0–3, equipment only) adds empty holes; gem codes work as stackables.
- [ ] e2e `TestForgeOverWebSocket` per §8 (level 25 via `DevSetLevel`, Thanh Thiết Vàng armour, `gem_hp_2`).
- [ ] README: gem codes and `-holes`.
- [ ] Commit: `Add devitem holes and an end-to-end forge test`.

### Task 10: Server — documentation

**Files:** `docs/game-design.md`, `CLAUDE.md`, `README.md`.

**TODO:**
- [ ] game-design 3.3 and 3.2 per §9; CLAUDE.md per §9; Vietnamese prose.
- [ ] Commit: `Document gems, sockets, refining and the 13-line resonance example`.

### Task 11: Client — sync and protocol 8

**Files:** `server`, `data/items.json`, `proto/protocol.gd`, `tests/fixtures/`, `autoload/net.gd`, `core/wire.gd`, `core/errors.gd`, `tests/unit/test_net.gd`.

**TODO / tests:**
- [ ] Submodule to server HEAD after Task 10; `PROTOCOL_VERSION := 8`; the ten new payload names; error texts per §6.1.
- [ ] Decode `item_with_gems.hex`, `refine_resp.hex`; `tests/run.sh` green with `--check`.
- [ ] Commit: `Sync the socket and refine protocol and data from the server`.

### Task 12: Client — resonance and forge rules

**Files:** `core/resonance.gd` (new), `core/forge_rules.gd`, `core/item_book.gd`, `core/inventory.gd`, `tests/unit/test_core.gd`.

**Interface:** per §6.1 (`active`, `need`, `count`; `punch_cost/block`, `gems_for`, `socket_block`, `unsocket_cost/block`, `refine_cost/block`); item dicts gain `holes`, `gems`.

**TODO / tests** (`# ____TEST RESONANCE____`, `# ____TEST FORGE RULES 3C____`):
- [ ] The §4.1 example (Viêm sword, Phong helmet and armour) and the 13-line set; an under-level activator lights nothing; `need("weapon", 1, "viem")` names the helmet and Phong.
- [ ] Costs and block reasons match the server's order for each command.
- [ ] Commit: `Add client resonance and socket and refine rules`.

### Task 13: Client — tooltips, equipment count, forge tabs

**Files:** `ui/forge_window.gd`, `ui/item_tooltip.gd`, `ui/equip_tab.gd`, `ui/bag_tab.gd`, `screens/world.gd`.

**Behaviour (§6.2):** lit/grey hidden lines with "Cần <ô> hệ <hệ>"; holes in the tooltip; "Dòng ẩn đang mở: n/20" in the equipment tab; the Khảm ngọc and Tẩy luyện tabs with their flows (closing while a result is pending sends a decline); requests and responses wired in `world.gd`.

**TODO / tests:**
- [ ] Unit tests green; `core/` Node-free.
- [ ] Manual check on :8299 with a `devitem`-stocked character: lines light as helmets/armour of the right element are worn; punch/socket/unsocket and refine with locks in the camp; refused outside it. Screenshots in the report.
- [ ] Commit: `Show resonance and add the socket and refine tabs`.

### Task 14: Client — smoke and README

**Files:** `tests/smoke/smoke.gd`, `tools/smoke.sh`, `README.md`.

**Behaviour (§6.3):** `smoke.sh` stocks the smoke account with `devitem` (a Vàng Lữ Hành armour, 2 refine stones, `gem_hp_1`, Xu); the smoke punches, sockets, refines once and takes the new lines; prints `SMOKE OK (… gem gem_hp_1, lines <old> -> <new>)`.

**TODO:**
- [ ] Live run on :8299; paste the OK line.
- [ ] README: the Khảm ngọc and Tẩy luyện tabs.
- [ ] Commit: `Extend the smoke test with sockets and refining`.

---

## Acceptance (controller, after Task 14)

- [ ] Server full `-race` suite with the DB, staticcheck, vet; client `tests/run.sh`; smoke OK.
- [ ] Restart :8199 from HEAD (own recorded PID only). With `test1`/`test2` offline: `devitem` Xích Cương Vàng/Cam pieces covering the five elements, gems of each kind at level 5, 50 refine stones, Xu.
- [ ] Ask the user to mix elements to light hidden lines (aim for 13/20), socket gems and refine with locks, and watch the stats change.
