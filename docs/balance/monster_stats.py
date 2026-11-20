"""Generate the monster kinds and per-level stats (data/monsters.json).

Run from any directory: python3 docs/balance/monster_stats.py
Add --write-json to write data/monsters.json, or --print-json to print the
same bytes to stdout (the Go test compares them with the committed file).

Rules (spec 2.3, no gear in Phase 1):
- the reference character is Viem at level L with 10 * (L - 1) potential
  points, floor(2/3) of them in STR and the rest in VIT;
- monster defense = 8 * L;
- monster HP, levels 1-9: 4 * the default slash of the reference character on
  a same-level monster (slash base = physical attack); from level 10: two
  slashes plus two Bat Tri Hoa (viem_1) at skill level min(10, L - 9) on a
  same-level monster (phase 2 spec 2.4);
- monster exp = max(1, round_half_up(HP * r(L))), r(L) = 0.1 * (1 + 0.1 * (L - 1)),
  in exact fractions;
- monster attack = ceil(0.04 * reference HP / (1 - def_ref / (def_ref + 20 * L))),
  so one monster hit takes about 4% of the reference character's HP.
All maths uses integers and fractions, so the output never depends on float
rounding. The formulas mirror internal/combat.
"""

import argparse
import json
import math
import sys
from fractions import Fraction
from pathlib import Path

import skill_stats

LEVELS = range(1, 91)
SKILL_FROM = 10  # first level whose HP includes Bat Tri Hoa casts

KINDS = [
    {"id": "wanderer", "name": "Quỷ lang thang", "attack_range": 2.0, "attack_sec": 1.5,
     "speed": 0.8, "detect_range": 5, "chase_range": 10, "shoot_sec": 2, "shoot_windup_sec": 0.5,
     "region_margin": 3, "chase_speed": 1.2, "safe_wait_sec": 5,
     "margin_grace_sec": 2, "grudge_sec": 10, "respawn_sec": 8},
    {"id": "snow_ghoul", "name": "Quỷ đói trong tuyết", "attack_range": 2.0, "attack_sec": 1.5,
     "speed": 0.8, "detect_range": 5, "chase_range": 10, "shoot_sec": 2, "shoot_windup_sec": 0.5,
     "region_margin": 3, "chase_speed": 1.2, "safe_wait_sec": 5,
     "margin_grace_sec": 2, "grudge_sec": 10, "respawn_sec": 8},
]


def damage(base, defense, attacker_level):
    """max(1, floor(base * (1 - def / (def + 20 * level))))."""
    k = 20 * attacker_level
    return max(1, base * k // (defense + k))


def reference(level):
    """Stats of the reference character: (HP, attack, defense)."""
    points = 10 * (level - 1)
    strength = points * 2 // 3
    vit = points - strength
    step = level - 1
    return 500 + 60 * step + 20 * vit, 20 + 5 * step + 2 * strength, 10 + 2 * step + 2 * vit


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
    if args.print_json:
        sys.stdout.write(to_json(levels))
        return
    for level, entry, hp_ref, hit in rows:
        print(f"L{level}: hp {entry['hp']} exp {entry['exp']} atk {entry['attack']} def {entry['defense']} "
              f"hit {hit}/{hp_ref} ({100 * hit / hp_ref:.1f}%)")
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "monsters.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(to_json(levels).encode("utf-8"))
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
