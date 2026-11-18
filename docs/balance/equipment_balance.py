"""Generate the proposed static equipment table and verify the PvP reference.

Run from any directory: python3 docs/balance/equipment_balance.py
Add --write-csv to export the per-item proposal beside this script.
Uses the Python standard library only. This is an offline design calculator,
not a runtime damage/HP adjustment or a complete combat simulator.
"""

import argparse
import csv
from math import ceil, floor
from pathlib import Path


UPGRADES = [100, 103, 107, 112, 118, 126, 136, 149,
            165, 185, 210, 240, 275, 315, 365, 430]
# Source, tier, collection, full-set primary attack and each defense at +0.
TIERS = [
    ("Thường", 10, "Lữ Hành", 100, 15),
    ("Thường", 20, "Thanh Thiết", 200, 30),
    ("Thường", 30, "Tinh Cương", 350, 53),
    ("Thường", 40, "Huyền Cương", 550, 83),
    ("Thường", 50, "Xích Cương", 800, 120),
    ("Chế", 30, "Tử Đằng", 403, 61),
    ("Chế", 50, "Phá Dạ", 920, 138),
    ("Chế", 70, "Nhật Luân", 1610, 242),
]
SLOTS = ["Vũ khí", "Giày", "Hộ uyển", "Quần", "Mũ", "Áo",
         "Dây chuyền", "Ngọc bội", "Nhẫn trái", "Nhẫn phải"]
# Crafted lines (indices 5-7) are kept for a later update but are out of
# scope for v1.0.0.0: they are excluded from verification and CSV export.
# Normal lines never depend on crafted lines, so their stats are unaffected.
INCLUDE_CRAFTED = False
CLASSES = ["Viêm", "Thủy", "Minh", "Nham", "Phong"]  # Fire, Water, Lightning, Stone, Wind.
WEAPONS = ["Viêm Nha", "Thủy Nguyệt", "Lôi Ảnh", "Nham Trấn", "Phong Vũ"]
ATTACK_WEIGHTS = [25, 5, 10, 10, 10, 15, 5, 10, 5, 5]
DEFENSE_WEIGHTS = [0, 10, 0, 25, 15, 40, 0, 10, 0, 0]


def allocate(total, weights):
    result = [total * weight // 100 for weight in weights]
    order = sorted(range(10), key=lambda i: (-(total * weights[i] % 100), i))
    for index in order[:total - sum(result)]:
        result[index] += 1
    return result


MAGIC_CLASSES = (1, 4)  # Water and Wind.


def base_stats(level, class_id=0):
    """Base attack, HP and each defense; magic classes grow less HP.

    Equipment HP is tuned on the physical-class HP (class_id 0), so magic
    classes are allowed to fall one hit lower in verify().
    """
    points = 10 * (level - 1)
    primary = points * 2 // 3
    vitality = points - primary
    hp_growth, hp_per_vit = (50, 16) if class_id in MAGIC_CLASSES else (60, 20)
    return (20 + 5 * (level - 1) + 2 * primary,
            500 + hp_growth * (level - 1) + hp_per_vit * vitality,
            10 + 2 * (level - 1) + 2 * vitality)


def damage_pair(level, attacker, defender, gear_attack, gear_defense):
    attack, _, defense = base_stats(level)
    attack += gear_attack
    defense += gear_defense
    passive = max(0, min(10, level - 29))
    rank = min(10, level - 9)
    if attacker == 4:  # Wind: magical attack, also selected by basic max().
        attack *= 1 + 0.01 * passive
    physical = defense * (1 + 0.02 * passive if defender in (0, 3) else 1)
    magical = defense * (1 + 0.02 * passive if defender == 0 else 1)
    coefficient = 1.1 + 0.1 * (rank - 1) if attacker == 4 else 1.5 + 0.15 * (rank - 1)
    skill_defense = magical if attacker in (1, 4) else physical
    skill = floor(attack * coefficient * 20 * level / (20 * level + skill_defense))
    basic = floor(attack * 20 * level / (20 * level + physical))
    hp_multiplier = 1 + 0.02 * passive if defender == 3 else 1
    return skill, basic, hp_multiplier


def hp_interval(level, gear_attack, gear_defense):
    """Base HP interval producing 8..12 hits for all 25 class pairings."""
    lower, upper = 0, float("inf")
    for attacker in range(5):
        for defender in range(5):
            skill, basic, hp_multiplier = damage_pair(
                level, attacker, defender, gear_attack, gear_defense)
            # Seven hits must not kill; twelve hits must kill.
            lower = max(lower, (4 * skill + 3 * basic + 1) / hp_multiplier)
            upper = min(upper, 6 * (skill + basic) / hp_multiplier)
    assert lower <= upper, (level, gear_attack, gear_defense)
    return lower, upper


def available_tier(indices, level, slot):
    options = [i for i in indices if TIERS[i][1] + slot <= level]
    return max(options, key=lambda i: (TIERS[i][1], i >= 5)) if options else None


def generate():
    """Compute fixed item stats offline, including separate HP upgrades."""
    items = {}
    for tier_index, (_, tier, _, attack_budget, defense_budget) in enumerate(TIERS):
        for upgrade, factor in enumerate(UPGRADES):
            attacks = allocate(attack_budget * factor // 100, ATTACK_WEIGHTS)
            defenses = allocate(defense_budget * factor // 100, DEFENSE_WEIGHTS)
            # Rounding must never lower an item's stat on upgrade.
            if upgrade:
                for slot in range(10):
                    old = items[tier_index, upgrade - 1, slot]
                    attacks[slot] = max(attacks[slot], old[0])
                    defenses[slot] = max(defenses[slot], old[2])
            for slot in range(10):
                level = tier + slot
                selected = []
                for position in range(10):
                    if position <= slot:
                        selected.append((tier_index, upgrade, position))
                    else:
                        previous = [i for i in range(tier_index)
                                    if TIERS[i][1] < tier and (i < 5 or tier_index >= 5)]
                        old_tier = available_tier(previous, level, position)
                        if old_tier is not None:
                            selected.append((old_tier, upgrade, position))
                gear_attack = sum(attacks[k[2]] if k[0] == tier_index else items[k][0]
                                  for k in selected)
                gear_defense = sum(defenses[k[2]] if k[0] == tier_index else items[k][2]
                                   for k in selected)
                previous_hp = sum(items[k][1] for k in selected
                                  if k != (tier_index, upgrade, slot))
                lower, upper = hp_interval(level, gear_attack, gear_defense)
                hp = round((lower + upper) / 2 - base_stats(level)[1] - previous_hp)
                hp = max(0, hp, items[tier_index, upgrade - 1, slot][1] if upgrade else 0)
                items[tier_index, upgrade, slot] = (attacks[slot], hp, defenses[slot])
            # End-of-line equipment remains usable after unlocking its last slot.
            end = {4: 90, 6: 69, 7: 90}.get(tier_index, tier + 9)
            if end > tier + 9:
                gear_attack, gear_defense = sum(attacks), sum(defenses)
                other_hp = sum(items[tier_index, upgrade, i][1] for i in range(9))
                lower, upper = 0, float("inf")
                for level in range(tier + 9, end + 1):
                    lo, hi = hp_interval(level, gear_attack, gear_defense)
                    lower = max(lower, lo - base_stats(level)[1] - other_hp)
                    upper = min(upper, hi - base_stats(level)[1] - other_hp)
                old = items[tier_index, upgrade, 9]
                hp = max(old[1], ceil(lower) + 2)
                assert hp <= upper, (tier_index, upgrade, lower, upper)
                items[tier_index, upgrade, 9] = (old[0], hp, old[2])
    for tier_index in range(8):
        for slot in range(10):
            for upgrade in range(1, 16):
                previous = items[tier_index, upgrade - 1, slot]
                current = items[tier_index, upgrade, slot]
                assert all(a <= b for a, b in zip(previous, current))
    return items


def verify(items):
    counts = []
    for crafted_route in (False, True) if INCLUDE_CRAFTED else (False,):
        for level in range(10, 91):
            for upgrade in range(16):
                gear = []
                for slot in range(10):
                    tier = available_tier(range(8 if crafted_route else 5), level, slot)
                    if tier is not None:
                        gear.append(items[tier, upgrade, slot])
                gear_attack, gear_hp, gear_defense = [sum(g[i] for g in gear) for i in range(3)]
                for attacker in range(5):
                    for defender in range(5):
                        skill, basic, multiplier = damage_pair(
                            level, attacker, defender, gear_attack, gear_defense)
                        hp = (base_stats(level, defender)[1] + gear_hp) * multiplier
                        hits, damage = 0, 0
                        while damage < hp:
                            damage += skill if hits % 2 == 0 else basic
                            hits += 1
                        minimum = 7 if defender in MAGIC_CLASSES else 8
                        assert minimum <= hits <= 12, (crafted_route, level, upgrade, attacker, defender, hits)
                        counts.append(hits)
    return len(counts), min(counts), max(counts)


def write_csv(items, path):
    with path.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.writer(stream)
        writer.writerow(["source", "tier", "collection", "slot", "required_level", "class",
                         "name", "upgrade", "hp", "physical_attack", "magical_attack",
                         "physical_defense", "magical_defense"])
        for tier_index, (source, tier, collection, _, _) in enumerate(TIERS):
            if tier_index >= 5 and not INCLUDE_CRAFTED:
                continue
            for upgrade in range(16):
                for slot, slot_name in enumerate(SLOTS):
                    attack, hp, defense = items[tier_index, upgrade, slot]
                    for class_id in range(5) if slot == 0 else (None,):
                        if class_id is not None:
                            kind = "Phủ–Chùy" if class_id == 3 else "Kiếm"
                            name = f"{WEAPONS[class_id]} {collection} {kind}"
                            physical = attack if class_id in (0, 2, 3) else 0
                            magical = attack if class_id in (1, 4) else 0
                            restriction = CLASSES[class_id]
                        else:
                            name = f"{slot_name} {collection}"
                            physical = magical = attack
                            restriction = "Tất cả"
                        writer.writerow([source, tier, collection, slot_name, tier + slot,
                                         restriction, name, upgrade, hp, physical, magical,
                                         defense, defense])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-csv", action="store_true")
    args = parser.parse_args()
    items = generate()
    count, low, high = verify(items)
    print(f"PASS: {count} reference cases, {low}–{high} hits; item stats never decrease on upgrade.")
    for index, (source, tier, collection, _, _) in enumerate(TIERS):
        if index >= 5 and not INCLUDE_CRAFTED:
            continue
        totals = [[sum(items[index, upgrade, slot][stat] for slot in range(10))
                   for stat in range(3)] for upgrade in (0, 15)]
        print(f"{source} {tier} {collection}: +0 {totals[0]}, +15 {totals[1]} (attack, HP, each defense)")
    if args.write_csv:
        path = Path(__file__).with_name("equipment-stats-proposal.csv")
        write_csv(items, path)
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
