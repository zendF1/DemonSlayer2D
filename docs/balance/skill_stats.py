"""Generate the player skill definitions (data/skills.json).

Run from any directory: python3 docs/balance/skill_stats.py
Add --write-json to write data/skills.json, or --print-json to print the
same bytes to stdout (the Go test compares them with the committed file).

Rules (phase 2 spec 2.1): five skills per class, ids <class>_1.._5 in learn
order (levels 10/20/30/40/50). Slots 1-4 are the actives in learn order, the
passive (always the third skill) has slot 0. Per skill level n (1-10):
- coef_pct = coef0 + step * (n - 1), mp = mp0 + mp_step * (n - 1);
- cooldown_ms = round_half_up(cooldown0 * 0.9 ** (n - 1) * 1000), exact
  fractions so the output never depends on float rounding;
- max_targets = 2,2,2,3,3,3,4,4,4,5 for every shape except single;
- range (tiles) is the same for every class: 5 at every level for single
  (dash included), 4,4,4,5,5,5,6,6,6,6 for cone, line and circle_target. It
  is the targeting range of single/circle_target, the cone's reach and the
  line's length. circle_self has no range, only its radius.
- windup_ms and projectile_speed (tiles/s) come from timing() by shape, slot and
  dash (phases spec 2.3); passives have neither. For a dash the speed is the
  dash speed; circle_self has 0.
monster_stats.py imports coef_pct() for the Bat Tri Hoa (viem_1) coefficient.
"""

import argparse
import json
import sys
from fractions import Fraction
from pathlib import Path

LEVELS = range(1, 11)
MAX_TARGETS = [2, 2, 2, 3, 3, 3, 4, 4, 4, 5]
SINGLE_RANGE = [5] * 10
AREA_RANGE = [4, 4, 4, 5, 5, 5, 6, 6, 6, 6]
ROMAN = ["I", "II", "III", "IV", "V"]

# Tiers of the four actives: (coef0, coef step, mp0, mp step, cooldown0 seconds).
TIER_1 = (150, 15, 10, 2, Fraction(3, 2))
TIER_2 = (110, 10, 15, 3, Fraction(5, 2))
TIER_3 = (160, 15, 25, 4, Fraction(4))
TIER_4 = (250, 25, 40, 5, Fraction(15, 2))
TIER_5 = (200, 20, 45, 6, Fraction(15, 2))


def active(cls, n, name, damage, shape, tier, **area):
    return {"id": f"{cls}_{n}", "class": cls, "name": name, "form": ROMAN[n - 1],
            "learn_level": 10 * n, "damage": damage, "shape": shape, "tier": tier, "area": area}


def passive(cls, n, name, **effects):
    return {"id": f"{cls}_{n}", "class": cls, "name": name, "form": ROMAN[n - 1],
            "learn_level": 10 * n, "passive": effects}


# Order inside a class is the learn order; the passive is the third skill.
SKILLS = [
    active("viem", 1, "Bất Tri Hỏa", "phys", "single", TIER_1, dash=True),
    active("viem", 2, "Thăng Viêm Thiên", "phys", "cone", TIER_2, cone_deg=120),
    passive("viem", 3, "Thịnh Viêm Lãng Triều", phys_def_pct=2, magic_def_pct=2),
    active("viem", 4, "Viêm Hổ", "phys", "circle_target", TIER_3, radius=1.5),
    active("viem", 5, "Luyện Ngục", "phys", "single", TIER_4),
    active("thuy", 1, "Thủy Diện Trảm", "magic", "single", TIER_1),
    active("thuy", 2, "Đả Triều", "magic", "cone", TIER_2, cone_deg=120),
    passive("thuy", 3, "Lặng", evasion_pct=3),
    active("thuy", 4, "Liệt Oa", "magic", "circle_self", TIER_3, radius=2.5),
    active("thuy", 5, "Sinh Sinh Lưu Chuyển", "magic", "single", TIER_4),
    active("minh", 1, "Phích Lịch Nhất Thiểm", "phys", "single", TIER_1, dash=True),
    active("minh", 2, "Tụ Văn Thành Lôi", "phys", "circle_self", TIER_2, radius=2),
    passive("minh", 3, "Nhiệt Giới Lôi", crit_bp=100),
    active("minh", 4, "Viễn Lôi", "phys", "line", TIER_3, width=1.5),
    active("minh", 5, "Hỏa Lôi Thần", "phys", "single", TIER_4),
    active("nham", 1, "Xà Văn Nham · Song Cực", "phys", "single", TIER_1),
    active("nham", 2, "Thiên Diện Toái", "phys", "circle_target", TIER_2, radius=1.5),
    passive("nham", 3, "Nham Phu Khu", max_hp_pct=2, phys_def_pct=2),
    active("nham", 4, "Lưu Văn Nham · Tốc Chinh", "phys", "circle_self", TIER_3, radius=3),
    active("nham", 5, "Ngõa Hình Luân Bộ", "phys", "single", TIER_4),
    active("phong", 1, "Trần Toàn Phong · Tước", "magic", "line", TIER_2, width=1.5),
    active("phong", 2, "Trảo · Khoa Hộ Phong", "magic", "single", TIER_1),
    passive("phong", 3, "Tình Lam Phong Thụ", accuracy_pct=2, magic_attack_pct=1),
    active("phong", 4, "Mộc Khô Lạp Phong", "magic", "circle_target", TIER_3, radius=2),
    active("phong", 5, "Vi Đà Thiên Đài Phong", "magic", "circle_target", TIER_5, radius=3),
]


def round_half_up(value):
    return int(value + Fraction(1, 2)) if value >= 0 else -int(-value + Fraction(1, 2))


def cooldown_ms(base_sec, n):
    return round_half_up(base_sec * Fraction(9, 10) ** (n - 1) * 1000)


def coef_pct(skill_id, n):
    """Damage coefficient (percent) of an active skill at skill level n."""
    for skill in SKILLS:
        if skill["id"] == skill_id:
            return skill["tier"][0] + skill["tier"][1] * (n - 1)
    raise KeyError(skill_id)


def timing(shape, slot, dash):
    """(windup_ms, projectile_speed) of an active skill (phases spec 2.3)."""
    if dash:
        return 100, 25
    if shape == "circle_self":
        return 300, 0
    if shape == "circle_target":
        return 300, 20
    if shape == "single" and slot == 4:
        return 400, 20
    return 250, 20  # single slots 1-3, cone, line


def build():
    out, slot = [], {}
    for skill in SKILLS:
        entry = {"id": skill["id"], "class": skill["class"], "name": skill["name"],
                 "form": skill["form"], "learn_level": skill["learn_level"]}
        if "passive" in skill:
            entry["slot"] = 0
            entry["passive"] = {key: [value * n for n in LEVELS]
                                for key, value in skill["passive"].items()}
        else:
            slot[skill["class"]] = slot.get(skill["class"], 0) + 1
            coef0, coef_step, mp0, mp_step, cd0 = skill["tier"]
            entry["slot"] = slot[skill["class"]]
            entry["damage"] = skill["damage"]
            entry["shape"] = skill["shape"]
            entry.update(skill["area"])
            entry["windup_ms"], entry["projectile_speed"] = timing(
                skill["shape"], entry["slot"], bool(skill["area"].get("dash")))
            if skill["shape"] == "single":
                entry["range"] = list(SINGLE_RANGE)
            elif skill["shape"] != "circle_self":
                entry["range"] = list(AREA_RANGE)
            entry["coef_pct"] = [coef0 + coef_step * (n - 1) for n in LEVELS]
            entry["mp"] = [mp0 + mp_step * (n - 1) for n in LEVELS]
            entry["cooldown_ms"] = [cooldown_ms(cd0, n) for n in LEVELS]
            if skill["shape"] != "single":
                entry["max_targets"] = list(MAX_TARGETS)
        out.append(entry)
    return out


def to_json(skills):
    return json.dumps({"skills": skills}, indent=2, ensure_ascii=False) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-json", action="store_true")
    parser.add_argument("--print-json", action="store_true")
    args = parser.parse_args()
    skills = build()
    assert len(skills) == 25 and len({s["id"] for s in skills}) == 25
    assert skills[0]["cooldown_ms"] == [1500, 1350, 1215, 1094, 984, 886, 797, 717, 646, 581]
    assert all(("range" in s) == (s.get("shape", "circle_self") != "circle_self") for s in skills)
    if args.print_json:
        sys.stdout.write(to_json(skills))
        return
    for s in skills:
        print(s["id"], s["name"], "passive" if "passive" in s else s["shape"])
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "skills.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(to_json(skills).encode("utf-8"))
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
