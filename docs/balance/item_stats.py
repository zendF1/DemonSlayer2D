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
  the normal range;
- enhancement (phase 3b spec 2.1): success rates and level drops follow
  game-design 3.3, stones and Xu per attempt are the spec's table; regen
  milestones and each slot's regen kind follow game-design 2.7;
- resonance, sockets and refining (phase 3c spec 2.1): the activator cycle
  and activating slots follow game-design 3.2; gem g of each kind is worth
  the line standard of tier 10 g, placed after the charm by kind then level.
The script checks every equipment value against equipment-stats-proposal.csv.
Without --print-json it also prints the mean enhancement costs from a seeded
simulation and checks the +15 mean attempts without charms (game-design 3.3:
about 6,500).
"""

import argparse
import csv
import json
import random
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
SLOT_REGEN = {"weapon": "mp", "boots": "hp", "bracers": "hp", "pants": "hp", "helmet": "hp",
              "armor": "hp", "necklace": "mp", "pendant": "mp", "ring_left": "mp", "ring_right": "mp"}
STACKABLES = [
    {"code": "stone_enhance", "name": "Đá cường hóa"},
    {"code": "stone_refine", "name": "Đá tẩy luyện"},
    {"code": "charm_protect", "name": "Bùa bảo vệ"},
]
STONE = "stone_enhance"
CHARM = "charm_protect"
STONE_REFINE = "stone_refine"
# Phase 3c spec 2.1. activator[e] lights the hidden lines of a piece of
# element e: the cycle Thuy -> Phong -> Viem -> Nham -> Minh -> Thuy.
ACTIVATOR = {"viem": "phong", "nham": "viem", "minh": "nham", "thuy": "minh", "phong": "thuy"}
# Slot -> the slots that light its hidden lines 1 and 2 (game-design 3.2).
RESONANCE_SLOTS = {
    "weapon": ["helmet", "armor"], "helmet": ["necklace", "ring_left"],
    "armor": ["necklace", "ring_right"], "pants": ["armor", "pendant"],
    "bracers": ["weapon", "ring_left"], "boots": ["pants", "pendant"],
    "necklace": ["weapon", "bracers"], "pendant": ["helmet", "boots"],
    "ring_left": ["pants", "ring_right"], "ring_right": ["boots", "bracers"],
}
SOCKETS = {"max": 3, "punch_xu_per_tier": [100, 200, 400], "unsocket_xu_per_level": 500}
REFINE = {"min_quality": 2, "stones": 1, "stones_per_lock": 1, "xu_per_tier": 20, "xu_per_tier_per_lock": 20}
# Gem kind, name, the line stats it raises.
GEM_KINDS = [
    ("attack", "Ngọc Công", ["phys_attack", "magic_attack"]),
    ("hp", "Ngọc Sinh Lực", ["hp"]),
    ("defense", "Ngọc Hộ Thể", ["phys_defense", "magic_defense"]),
]
GEM_LEVELS = range(1, 6)
PITY_PCT = 1
MAX_TIMES = 100
# Phase 3b spec 2.1: target level, success %, drops a level on an unprotected
# failure, stones per attempt, Xu per attempt per item tier.
ENHANCE_TABLE = [
    (1, 100, False, 1, 10), (2, 100, False, 1, 15), (3, 100, False, 1, 20), (4, 100, False, 1, 25),
    (5, 90, False, 2, 40), (6, 80, False, 2, 50), (7, 70, False, 2, 60),
    (8, 60, True, 3, 80), (9, 50, True, 3, 100), (10, 40, True, 3, 120),
    (11, 30, True, 4, 150), (12, 25, True, 4, 180), (13, 20, True, 5, 220),
    (14, 15, True, 5, 260), (15, 10, True, 5, 300),
]
# game-design 2.7: enhance milestone, HP per second in 1/10000 of max HP
# (armour), MP per second (weapon and jewellery).
REGEN = [(7, 5, 1), (10, 10, 2), (12, 20, 3), (15, 30, 5)]
SIM_SEED = 3
SIM_RUNS = {7: 20000, 10: 20000, 12: 20000, 15: 2000}
SIM_TIER = 50
MEAN_15_RANGE = (5500, 7500)
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


def gems(tiers):
    """The 15 gems: level g is worth tier 10 g's line standard."""
    standard = {t["tier"]: t["line_standard"] for t in tiers}
    return [{"code": f"gem_{kind}_{level}", "name": f"{name} cấp {level}",
             "gem": {"kind": kind, "level": level,
                     "stats": {stat: standard[10 * level][stat] for stat in stats}}}
            for kind, name, stats in GEM_KINDS for level in GEM_LEVELS]


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
        "slots": [{"id": sid, "name": eb.SLOTS[i], "index": i, "regen": SLOT_REGEN[sid]}
                  for i, sid in enumerate(SLOT_IDS)],
        "qualities": QUALITIES,
        "line_stats": LINE_STATS,
        "line_roll_min_pct": LINE_ROLL_MIN_PCT,
        "tiers": tiers,
        "equipment": equipment,
        "stackables": STACKABLES + gems(tiers),
        "drops": DROPS,
        "enhance": {
            "pity_pct": PITY_PCT,
            "max_times": MAX_TIMES,
            "levels": [{"level": lv, "success_pct": pct, "drop": drop, "stones": stones, "xu_per_tier": xu}
                       for lv, pct, drop, stones, xu in ENHANCE_TABLE],
        },
        "regen": [{"enhance": n, "hp_bp": hp, "mp": mp} for n, hp, mp in REGEN],
        "resonance": {"activator": ACTIVATOR, "slots": RESONANCE_SLOTS},
        "sockets": SOCKETS,
        "refine": REFINE,
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
    check_enhance(data)
    check_forge(data)


def check_forge(data):
    """The load checks of phase 3c spec 2.1, as the Go server makes them, and
    the spec's gem table."""
    act = data["resonance"]["activator"]
    assert sorted(act) == sorted(CLASS_IDS) and sorted(act.values()) == sorted(CLASS_IDS), act
    e, steps = CLASS_IDS[0], 0
    while True:
        assert act[e] != e, e
        e, steps = act[e], steps + 1
        if e == CLASS_IDS[0]:
            break
    assert steps == 5, act
    slots = data["resonance"]["slots"]
    assert sorted(slots) == sorted(SLOT_IDS), slots
    for sid, on in slots.items():
        assert len(on) == 2 and on[0] != on[1] and sid not in on and all(o in SLOT_IDS for o in on), (sid, on)
    sock = data["sockets"]
    assert sock["max"] == len(sock["punch_xu_per_tier"]) >= 1, sock
    assert all(v > 0 for v in sock["punch_xu_per_tier"]) and sock["unsocket_xu_per_level"] > 0, sock
    ref = data["refine"]
    assert 1 <= ref["min_quality"] <= 4 and ref["stones"] >= 1, ref
    assert min(ref["stones_per_lock"], ref["xu_per_tier"], ref["xu_per_tier_per_lock"]) >= 0, ref
    codes = [s["code"] for s in data["stackables"]]
    assert STONE_REFINE in codes, codes
    seen = set()
    for s in data["stackables"]:
        g = s.get("gem")
        if g is None:
            continue
        key = (g["kind"], g["level"])
        assert key not in seen and 1 <= g["level"] <= 5 and g["stats"], s
        assert all(k in LINE_STATS and v > 0 for k, v in g["stats"].items()), s
        seen.add(key)
        hp, attack, defense = WANT_STANDARD[10 * g["level"]]
        want = {"attack": {"phys_attack": attack, "magic_attack": attack}, "hp": {"hp": hp},
                "defense": {"phys_defense": defense, "magic_defense": defense}}[g["kind"]]
        assert g["stats"] == want, s
    assert seen == {(k, lv) for k, _, _ in GEM_KINDS for lv in GEM_LEVELS}, seen


def check_enhance(data):
    """The load checks of phase 3b spec 2.1, as the Go server makes them."""
    enh = data["enhance"]
    assert enh["pity_pct"] >= 1 and 1 <= enh["max_times"] <= 1000, enh
    levels = enh["levels"]
    assert [lv["level"] for lv in levels] == list(range(1, 16)), levels
    for i, lv in enumerate(levels):
        assert 1 <= lv["success_pct"] <= 100, lv
        assert lv["stones"] >= 1 and lv["xu_per_tier"] >= 1, lv
        assert not lv["drop"] or lv["success_pct"] < 100, lv
        if lv["level"] <= 4:
            assert lv["success_pct"] == 100 and not lv["drop"], lv
        if i:
            assert lv["success_pct"] <= levels[i - 1]["success_pct"], lv
            assert lv["xu_per_tier"] >= levels[i - 1]["xu_per_tier"], lv
    regen = data["regen"]
    assert regen, regen
    for i, step in enumerate(regen):
        assert 1 <= step["enhance"] <= 15 and step["hp_bp"] > 0 and step["mp"] > 0, step
        if i:
            prev = regen[i - 1]
            assert step["enhance"] > prev["enhance"] and step["hp_bp"] > prev["hp_bp"] and step["mp"] > prev["mp"], step
    for slot in data["slots"]:
        assert slot["regen"] in ("hp", "mp"), slot
    codes = [s["code"] for s in data["stackables"]]
    assert STONE in codes and CHARM in codes, codes


def simulate(levels, pity_pct, target, charm, rng):
    """One item from +0 to target (phase 3b spec 4.1), attempts one at a
    time; charm protects every attempt whose target level can drop. Returns
    (attempts, stones, Xu per tier, charms)."""
    level, pity = 0, [0] * len(levels)
    attempts = stones = xu = charms = 0
    while level < target:
        k = levels[level]
        protect = charm and k["drop"]
        attempts += 1
        stones += k["stones"]
        xu += k["xu_per_tier"]
        charms += protect
        rate = min(100, k["success_pct"] + pity[level])
        if rng.randrange(100) < rate:
            pity[level] = 0
            level += 1
            continue
        pity[level] = min(pity[level] + pity_pct, 100 - k["success_pct"])
        if k["drop"] and not protect:
            level -= 1
    return attempts, stones, xu, charms


def mean_costs(data):
    """{(target, charm): (attempts, stones, Xu at SIM_TIER, charms)}, means
    over SIM_RUNS[target] seeded runs."""
    enh = data["enhance"]
    out = {}
    for target, runs in SIM_RUNS.items():
        for charm in (False, True):
            rng = random.Random(f"{SIM_SEED}-{target}-{charm}")
            sums = [0, 0, 0, 0]
            for _ in range(runs):
                for i, v in enumerate(simulate(enh["levels"], enh["pity_pct"], target, charm, rng)):
                    sums[i] += v
            a, s, x, c = (v / runs for v in sums)
            out[target, charm] = (a, s, x * SIM_TIER, c)
    return out


def print_costs(costs):
    print(f"mean enhancement cost from +0 (seed {SIM_SEED}, Xu at tier {SIM_TIER}):")
    print("target | no charm: attempts / stones / Xu | charm from +8: attempts / stones / Xu / charms")
    for target in SIM_RUNS:
        a, s, x, _ = costs[target, False]
        ca, cs, cx, cc = costs[target, True]
        print(f"+{target:<5} | {a:,.0f} / {s:,.0f} / {x:,.0f} | {ca:,.0f} / {cs:,.0f} / {cx:,.0f} / {cc:,.0f}")
    lo, hi = MEAN_15_RANGE
    mean15 = costs[15, False][0]
    assert lo <= mean15 <= hi, f"+15 mean attempts without charm {mean15:.0f} outside {lo}-{hi}"


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
    print_costs(mean_costs(data))
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "items.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(text.encode("utf-8"))
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
