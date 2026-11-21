"""Generate the monster kinds and per-level stats (data/monsters.json).

Run from any directory: python3 docs/balance/monster_stats.py
Add --write-json to write data/monsters.json, or --print-json to print the
same bytes to stdout (the Go test compares them with the committed file).

Rules (phase 1 spec 2.3, phase 3a spec 2.2, game-design 5.9):
- the reference character is Viem at level L with 10 * (L - 1) potential
  points, floor(2/3) of them in STR and the rest in VIT, wearing in each
  slot the highest-tier normal +0 piece with tier + slot index <= L
  (equipment_balance.available_tier), nothing before level 10; its HP,
  attack and defense add the summed stats of those pieces;
- monster defense = 8 * L;
- monster HP, levels 1-9: 4 * the default slash of the reference character on
  a same-level monster (slash base = physical attack); from level 10: two
  slashes plus two Bat Tri Hoa (viem_1) at skill level min(10, L - 9) on a
  same-level monster (phase 2 spec 2.4);
- monster exp = max(1, round_half_up(HP * r(L))), r(L) = 0.1 * (1 + 0.1 * (L - 1)),
  in exact fractions;
- monster attack = ceil(0.04 * reference HP / (1 - def_ref / (def_ref + 20 * L))),
  so one monster hit takes about 4% of the reference character's HP.
- kinds (phase 4d spec 2.5, 2.6): every kind sets every field; hp_pct and
  attack_pct (default 100) scale a monster's HP and base EXP, and its attack,
  to ceil(table value * pct / 100); damage_type (default phys, or magic) picks
  the player defense its melee and shots meet. The levels table is shared.
All maths uses integers and fractions, so the output never depends on float
rounding. The formulas mirror internal/combat.
"""

import argparse
import json
import math
import sys
from fractions import Fraction
from pathlib import Path

import equipment_balance
import skill_stats

LEVELS = range(1, 91)
SKILL_FROM = 10  # first level whose HP includes Bat Tri Hoa casts

# Attack timing (phases spec 2.3): the melee windup in ms, the shot speed in tiles/s.
MELEE_WINDUP_MS = 200
SHOT_SPEED = 15

KINDS = [
    {"id": "wanderer", "name": "Quỷ lang thang", "rank": "normal", "attack_range": 2.0, "attack_sec": 1.5,
     "speed": 0.8, "detect_range": 5, "chase_range": 10, "shoot_sec": 2, "shoot_windup_sec": 0.5,
     "region_margin": 3, "chase_speed": 1.2, "safe_wait_sec": 5,
     "margin_grace_sec": 2, "grudge_sec": 10, "respawn_sec": 8,
     "melee_windup_ms": MELEE_WINDUP_MS, "shot_speed": SHOT_SPEED,
     "hp_pct": 100, "attack_pct": 100, "damage_type": "phys"},
    {"id": "snow_ghoul", "name": "Quỷ đói trong tuyết", "rank": "normal", "attack_range": 2.0, "attack_sec": 1.5,
     "speed": 0.8, "detect_range": 5, "chase_range": 10, "shoot_sec": 2, "shoot_windup_sec": 0.5,
     "region_margin": 3, "chase_speed": 1.2, "safe_wait_sec": 5,
     "margin_grace_sec": 2, "grudge_sec": 10, "respawn_sec": 8,
     "melee_windup_ms": MELEE_WINDUP_MS, "shot_speed": SHOT_SPEED,
     "hp_pct": 100, "attack_pct": 100, "damage_type": "phys"},
]

# Phase 4a spec 2.4: two normal kinds per new training map, behaving exactly
# like wanderer (the levels table gives their stats).
NEW_KINDS = [
    ("mountain_demon", "Quỷ rừng núi"),  # sagiri_1
    ("ambusher", "Quỷ phục kích"),  # sagiri_1
    ("mist_lurker", "Quỷ ẩn sương"),  # sagiri_2
    ("long_claw", "Quỷ vuốt dài"),  # sagiri_2
    ("forest_demon", "Quỷ Rừng"),  # fujikasane_1
    ("captive_demon", "Quỷ bị giam trên núi"),  # fujikasane_1
]
KINDS += [{**KINDS[0], "id": kind_id, "name": name} for kind_id, name in NEW_KINDS]

# Phase 4d spec 2.5: the 26 normal kinds of levels 21-90 in map order, like
# wanderer except for the overrides. The three train kinds keep the ids, names
# and speed of the 7g spec 2.3. Spec 2.6: hp_pct scales HP and base EXP,
# attack_pct the attack, damage_type picks the player defense the hits meet.
NEW_KINDS_4D = [
    ("big_demon", "Quỷ thân lớn", {}),  # fujikasane_2
    ("many_arms", "Quỷ nhiều tay", {}),  # fujikasane_2
    ("marsh_demon", "Quỷ ven đầm", {}),  # tay_bac_1
    ("mud_ambusher", "Quỷ phục kích dưới bùn", {}),  # tay_bac_1
    ("new_demon", "Quỷ mới biến đổi", {}),  # asakusa_1
    ("city_wanderer", "Quỷ lang thang đô thị", {}),  # asakusa_1
    ("henchman", "Quỷ tay sai", {}),  # asakusa_2
    ("manor_woods", "Quỷ rừng ngoài dinh", {}),  # tsuzumi_1
    ("long_nail", "Quỷ móng dài", {}),  # tsuzumi_1
    ("manor_dweller", "Quỷ trú trong dinh", {}),  # tsuzumi_2
    ("small_spider", "Nhện quỷ nhỏ", {}),  # natagumo_1
    ("thread_puppet", "Con rối bị tơ điều khiển", {}),  # natagumo_1
    ("spider_demon", "Nhện quỷ", {}),  # natagumo_2
    ("silk_cocoon", "Quái kén tơ", {}),  # natagumo_2
    ("silk_fiend", "Quái tơ nhện", {}),  # natagumo_3
    ("veteran_puppet", "Con rối tinh nhuệ", {}),  # natagumo_3
    ("train_tentacle", "Xúc tu", {}),  # mugen_1
    ("train_flesh", "Mô thịt", {"speed": 0.6}),  # mugen_1, mugen_2
    ("train_nightmare", "Quái ác mộng", {}),  # mugen_2
    ("night_lurker", "Quỷ ẩn phố đêm", {}),  # yoshiwara_1, yoshiwara_2
    ("sash_fragment", "Phân thể dải lụa", {}),  # yoshiwara_1, yoshiwara_2
    ("mask_demon", "Quỷ Mặt Nạ", {}),  # yoshiwara_3 r1, level 86
    ("long_talon", "Quỷ Trảo Dài", {"attack_range": 2.0}),  # r2, 87: the standard melee range
    ("blood_fang", "Quỷ Huyết Nha", {"hp_pct": 80, "attack_pct": 120}),  # r3, 88
    ("shadow_demon", "Quỷ Tàng Ảnh", {"speed": 0.96, "chase_speed": 1.44}),  # r4, 89: +20%
    ("scale_serpent", "Quỷ Lân Xà", {"damage_type": "magic", "detect_range": 4}),  # r5, 90
]
KINDS += [{**KINDS[0], "id": kind_id, "name": name, **over} for kind_id, name, over in NEW_KINDS_4D]


def damage(base, defense, attacker_level):
    """max(1, floor(base * (1 - def / (def + 20 * level))))."""
    k = 20 * attacker_level
    return max(1, base * k // (defense + k))


GEAR_ITEMS = equipment_balance.generate()
NORMAL_TIERS = range(5)  # TIERS indices of the normal lines; no crafted gear in v1.0.0.0


def gear(level):
    """Summed +0 stats of the reference gear at this level: (HP, attack, defense)."""
    hp = attack = defense = 0
    for slot in range(10):
        tier = equipment_balance.available_tier(NORMAL_TIERS, level, slot)
        if tier is not None:
            piece_attack, piece_hp, piece_defense = GEAR_ITEMS[tier, 0, slot]
            hp, attack, defense = hp + piece_hp, attack + piece_attack, defense + piece_defense
    return hp, attack, defense


def reference(level):
    """Stats of the reference character with its gear: (HP, attack, defense)."""
    points = 10 * (level - 1)
    strength = points * 2 // 3
    vit = points - strength
    step = level - 1
    gear_hp, gear_attack, gear_defense = gear(level)
    return (500 + 60 * step + 20 * vit + gear_hp, 20 + 5 * step + 2 * strength + gear_attack,
            10 + 2 * step + 2 * vit + gear_defense)


def exp_ratio(level):
    return Fraction(1, 10) * (1 + Fraction(level - 1, 10))


def exp(level):
    """Base EXP a monster of this level pays in total (exact, half up)."""
    return monster(level)[0]["exp"]


def monster_hp(level, atk_ref, defense):
    slash = damage(atk_ref, defense, level)
    if level < SKILL_FROM:
        return 4 * slash
    coef = skill_stats.coef_pct("viem_1", min(10, level - 9))
    return 2 * slash + 2 * damage(atk_ref * coef // 100, defense, level)


def monster(level):
    hp_ref, atk_ref, def_ref = reference(level)
    defense = 8 * level
    hp = monster_hp(level, atk_ref, defense)
    k = 20 * level
    attack = math.ceil(Fraction(4, 100) * hp_ref * (def_ref + k) / k)
    hit = damage(attack, def_ref, level)
    entry = {"level": level, "hp": hp, "attack": attack, "defense": defense}
    entry["exp"] = max(1, int(hp * exp_ratio(level) + Fraction(1, 2)))
    return entry, hp_ref, hit


def build():
    levels, rows = [], []
    for level in LEVELS:
        entry, hp_ref, hit = monster(level)
        levels.append(entry)
        rows.append((level, entry, hp_ref, hit))
    return levels, rows


def scale_pct(value, pct):
    """ceil(value * pct / 100) in integers, like combat.ScalePct."""
    return -(-value * pct // 100)


def to_json(levels):
    text = json.dumps({"kinds": KINDS, "levels": levels}, indent=2, ensure_ascii=False)
    return text + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-json", action="store_true")
    parser.add_argument("--print-json", action="store_true")
    args = parser.parse_args()
    levels, rows = build()
    for level, entry, hp_ref, hit in rows:
        assert 0.039 <= hit / hp_ref <= 0.041, (level, hit, hp_ref)
    assert monster(10)[0]["attack"] == 95, monster(10)
    # Pinned values: a wrong rule that gets regenerated must not slip through.
    pinned = {1: (56, 30, 8), 2: (104, 39, 16), 3: (160, 45, 24), 4: (212, 52, 32), 5: (260, 60, 40)}
    for level, want in pinned.items():
        entry = levels[level - 1]
        assert (entry["hp"], entry["attack"], entry["defense"]) == want, (level, entry)
    # Game-design 5.9 (HP, attack, defense) with the reference gear; level 30
    # is 4336, not the doc's 4338 (phase 3a spec 1.4).
    pinned = {10: (750, 95, 80), 20: (2704, 319, 160), 30: (4336, 508, 240), 40: (6240, 694, 320),
              50: (8408, 934, 400), 60: (10452, 1211, 480), 90: (13478, 1412, 720)}
    for level, want in pinned.items():
        entry = levels[level - 1]
        assert (entry["hp"], entry["attack"], entry["defense"]) == want, (level, entry)
    assert (monster(10)[0]["hp"], monster(10)[0]["exp"]) == (750, 143), monster(10)
    assert all(gear(level) == (0, 0, 0) for level in range(1, 10)), "no gear before level 10"
    assert len({k["id"] for k in KINDS}) == len(KINDS) == 8 + 26, [k["id"] for k in KINDS]
    assert len({k["name"] for k in KINDS}) == len(KINDS), "kind names must differ"
    overrides = {kind_id: over for kind_id, _, over in NEW_KINDS_4D}
    for kind in KINDS:
        assert {**kind, "id": "wanderer", "name": KINDS[0]["name"],
                **{f: KINDS[0][f] for f in overrides.get(kind["id"], {})}} == KINDS[0], kind
        assert 50 <= kind["hp_pct"] <= 200 and 50 <= kind["attack_pct"] <= 200, kind
        assert kind["damage_type"] in ("phys", "magic"), kind
    assert KINDS[0]["hp_pct"] == KINDS[0]["attack_pct"] == 100 and KINDS[0]["damage_type"] == "phys"
    assert scale_pct(1000, 80) == 800 and scale_pct(1001, 80) == 801 and scale_pct(1001, 100) == 1001
    # Phase 4d: adding kinds leaves the levels table as committed.
    committed = Path(__file__).resolve().parents[2] / "data" / "monsters.json"
    if committed.exists():
        assert json.loads(committed.read_text(encoding="utf-8"))["levels"] == levels, "the levels table changed"
    if args.print_json:
        sys.stdout.write(to_json(levels))
        return
    for level, entry, hp_ref, hit in rows:
        print(f"L{level}: hp {entry['hp']} exp {entry['exp']} atk {entry['attack']} def {entry['defense']} "
              f"hit {hit}/{hp_ref} ({100 * hit / hp_ref:.1f}%)")
    print("kinds: " + ", ".join(f"{k['id']} ({k['name']})" for k in KINDS))
    blood = next(k for k in KINDS if k["id"] == "blood_fang")
    row = levels[88 - 1]
    print(f"blood_fang L88 ({blood['name']}, hp {blood['hp_pct']}%, attack {blood['attack_pct']}%): "
          f"hp {row['hp']} -> {scale_pct(row['hp'], blood['hp_pct'])}, "
          f"exp {row['exp']} -> {scale_pct(row['exp'], blood['hp_pct'])}, "
          f"attack {row['attack']} -> {scale_pct(row['attack'], blood['attack_pct'])}")
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "monsters.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(to_json(levels).encode("utf-8"))
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
