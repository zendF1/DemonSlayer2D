"""Generate the item data (data/items.json).

Run from any directory: python3 docs/balance/item_stats.py
Add --write-json to write data/items.json, or --print-json to print the
same bytes to stdout (the Go test compares them with the committed file).

Rules (phase 3a spec 2.1):
- 10 equipment slots in game-design 3.1 order; a slot's index is also its
  wear-level offset from the tier (level = tier + index);
- five qualities; Vang, Cam and Do have an element and two hidden lines
  worth hidden_pct % of the line standard;
- line standard S of each line stat at each tier = max(1, round_half_up(4 %
  of the full-set +0 total)), in exact fractions: HP from the generated +0
  pieces, both attacks from the tier's attack budget, both defenses from
  its defense budget;
- the 70 normal pieces (5 tiers x (9 shared + 5 class weapons)) come from
  equipment_balance.generate(); names and the per-class attack split follow
  its write_csv(), and each array element n is the stat at +n;
- drop tables follow game-design 5.12; elite and boss Xu are 6 and 25 times
  the normal range.
The script checks every equipment value against equipment-stats-proposal.csv.
"""

import argparse
import csv
import json
import sys
from fractions import Fraction
from pathlib import Path

import equipment_balance as eb

BAG_SIZE = 60
STACK_MAX = 999
GROUND_SEC = 60
LINE_ROLL_MIN_PCT = 50
LINE_PCT = Fraction(4, 100)
NORMAL_TIERS = range(5)  # TIERS indices 0-4; crafted lines are out of scope.

SLOT_IDS = ["weapon", "boots", "bracers", "pants", "helmet", "armor",
            "necklace", "pendant", "ring_left", "ring_right"]
CLASS_IDS = ["viem", "thuy", "minh", "nham", "phong"]  # equipment_balance.CLASSES order
PHYSICAL_CLASSES = (0, 2, 3)  # as write_csv: Viem, Minh, Nham
QUALITIES = [
    {"name": "Trắng", "rank": "Thường", "lines": [0, 0], "hidden_pct": 0},
    {"name": "Xanh Dương", "rank": "Ma thuật", "lines": [1, 2], "hidden_pct": 0},
    {"name": "Vàng", "rank": "Hiếm", "lines": [3, 4], "hidden_pct": 100},
    {"name": "Cam", "rank": "Thần thoại", "lines": [4, 4], "hidden_pct": 125},
    {"name": "Đỏ", "rank": "Hoàng Kim", "lines": [4, 4], "hidden_pct": 150},
]
LINE_STATS = ["hp", "phys_attack", "magic_attack", "phys_defense", "magic_defense"]
STAT_KEYS = LINE_STATS  # the five per-piece arrays, in JSON order
STACKABLES = [
    {"code": "stone_enhance", "name": "Đá cường hóa"},
    {"code": "stone_refine", "name": "Đá tẩy luyện"},
]
XU_NORMAL = [3, 5]
DROPS = {
    "normal": {
        "xu_per_level": XU_NORMAL,
        "equipment": {"chance_bp": 500, "count": 1, "quality_bp": [8000, 1700, 300, 0, 0]},
        "stacks": [{"code": "stone_enhance", "chance_bp": 2000, "count": 1}],
    },
    "elite": {
        "xu_per_level": [6 * x for x in XU_NORMAL],
        "equipment": {"chance_bp": 10000, "count": 1, "quality_bp": [0, 7500, 2500, 0, 0]},
        "stacks": [{"code": "stone_enhance", "chance_bp": 10000, "count": 3},
                   {"code": "stone_refine", "chance_bp": 1000, "count": 1}],
    },
    "boss": {
        "xu_per_level": [25 * x for x in XU_NORMAL],
        "equipment": {"chance_bp": 10000, "count": 2, "quality_bp": [0, 2000, 7500, 500, 0]},
        "stacks": [{"code": "stone_enhance", "chance_bp": 10000, "count": 10},
                   {"code": "stone_refine", "chance_bp": 10000, "count": 1}],
    },
}
# Phase 3a spec 2.1 table: tier -> (HP, each attack, each defense).
WANT_STANDARD = {10: (86, 4, 1), 20: (156, 8, 1), 30: (220, 14, 2), 40: (320, 22, 3), 50: (469, 32, 5)}


def round_half_up(value):
    return int(value + Fraction(1, 2))


def line_standard(items, tier_index):
    """S per line stat at a tier, from the full-set +0 totals."""
    _, _, _, attack_budget, defense_budget = eb.TIERS[tier_index]
    hp_total = sum(items[tier_index, 0, slot][1] for slot in range(10))
    assert sum(items[tier_index, 0, slot][0] for slot in range(10)) == attack_budget
    assert sum(items[tier_index, 0, slot][2] for slot in range(10)) == defense_budget
    hp, attack, defense = (max(1, round_half_up(LINE_PCT * total))
                           for total in (hp_total, attack_budget, defense_budget))
    return {"hp": hp, "phys_attack": attack, "magic_attack": attack,
            "phys_defense": defense, "magic_defense": defense}


def piece(items, tier_index, slot, class_id):
    """One equipment entry; class_id is None for a shared piece."""
    _, tier, collection, _, _ = eb.TIERS[tier_index]
    stats = {key: [] for key in STAT_KEYS}
    for upgrade in range(16):
        attack, hp, defense = items[tier_index, upgrade, slot]
        if class_id is None:
            physical = magical = attack
        else:
            physical = attack if class_id in PHYSICAL_CLASSES else 0
            magical = 0 if class_id in PHYSICAL_CLASSES else attack
        for key, value in zip(STAT_KEYS, (hp, physical, magical, defense, defense)):
            stats[key].append(value)
    if class_id is None:
        code = f"eq{tier}_{SLOT_IDS[slot]}"
        name = f"{eb.SLOTS[slot]} {collection}"
        cls = ""
    else:
        code = f"eq{tier}_weapon_{CLASS_IDS[class_id]}"
        kind = "Phủ–Chùy" if class_id == 3 else "Kiếm"
        name = f"{eb.WEAPONS[class_id]} {collection} {kind}"
        cls = CLASS_IDS[class_id]
    entry = {"code": code, "tier": tier, "slot": SLOT_IDS[slot], "class": cls,
             "name": name, "level": tier + slot}
    entry.update(stats)
    return entry


def build():
    items = eb.generate()
    tiers, equipment = [], []
    for tier_index in NORMAL_TIERS:
        _, tier, collection, _, _ = eb.TIERS[tier_index]
        tiers.append({"tier": tier, "name": collection,
                      "line_standard": line_standard(items, tier_index)})
        for slot in range(10):
            for class_id in range(5) if slot == 0 else (None,):
                equipment.append(piece(items, tier_index, slot, class_id))
    return {
        "bag_size": BAG_SIZE,
        "stack_max": STACK_MAX,
        "ground_sec": GROUND_SEC,
        "slots": [{"id": sid, "name": eb.SLOTS[i], "index": i} for i, sid in enumerate(SLOT_IDS)],
        "qualities": QUALITIES,
        "line_stats": LINE_STATS,
        "line_roll_min_pct": LINE_ROLL_MIN_PCT,
        "tiers": tiers,
        "equipment": equipment,
        "stackables": STACKABLES,
        "drops": DROPS,
    }


def check(data):
    """Self-checks: CSV match, the spec's S table, quality shares, unique codes."""
    path = Path(__file__).with_name("equipment-stats-proposal.csv")
    with path.open(encoding="utf-8", newline="") as stream:
        rows = list(csv.DictReader(stream))
    assert len(rows) == 5 * 16 * 14, len(rows)
    by_name = {entry["name"]: entry for entry in data["equipment"]}
    seen = set()
    for row in rows:
        entry = by_name[row["name"]]
        upgrade = int(row["upgrade"])
        seen.add((row["name"], upgrade))
        assert int(row["tier"]) == entry["tier"], row
        assert int(row["required_level"]) == entry["level"], row
        want_class = "Tất cả" if not entry["class"] else eb.CLASSES[CLASS_IDS.index(entry["class"])]
        assert row["class"] == want_class, row
        for key, column in zip(STAT_KEYS, ("hp", "physical_attack", "magical_attack",
                                           "physical_defense", "magical_defense")):
            assert entry[key][upgrade] == int(row[column]), (row, key)
    assert len(seen) == len(rows) == 16 * len(data["equipment"]), (len(seen), len(rows))
    assert len(data["equipment"]) == 70
    got = {t["tier"]: (t["line_standard"]["hp"], t["line_standard"]["phys_attack"],
                       t["line_standard"]["phys_defense"]) for t in data["tiers"]}
    assert got == WANT_STANDARD, got
    for t in data["tiers"]:
        s = t["line_standard"]
        assert s["magic_attack"] == s["phys_attack"] and s["magic_defense"] == s["phys_defense"], t
    for rank, drop in data["drops"].items():
        assert sum(drop["equipment"]["quality_bp"]) == 10000, rank
    codes = [e["code"] for e in data["equipment"]] + [s["code"] for s in data["stackables"]]
    assert len(codes) == len(set(codes)), codes


def to_json(data):
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-json", action="store_true")
    parser.add_argument("--print-json", action="store_true")
    args = parser.parse_args()
    data = build()
    check(data)
    text = to_json(data)
    if args.print_json:
        sys.stdout.write(text)
        return
    for t in data["tiers"]:
        print(f"tier {t['tier']} {t['name']}: line standard {t['line_standard']}")
    print(f"{len(data['equipment'])} equipment pieces, {len(data['stackables'])} stackables")
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "items.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(text.encode("utf-8"))
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
