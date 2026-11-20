"""Generate the level-up EXP table (data/exp.json and exp-table.csv).

Run from any directory: python3 docs/balance/exp_table.py
Add --write-json to write data/exp.json, --print-json to print the same
bytes to stdout (the Go test compares them with the committed file) and
--write-csv to export exp-table.csv beside this script. The Go server only
reads data/exp.json; it never recomputes the curve.

Going from level L to L + 1 needs kills(L) same-level normal monsters:
kills(L) = (10 + 2 * (L - 1) ** 1.5) * exp_multiplier(L), and each pays
exp(L) EXP (monster_stats.py, phase 2 spec 2.4), so
exp_to_next(L) = round10(kills(L) * exp(L)).
- levels 1-9 ramp up as 4 * L / 3, about 3 hours of monster grinding from
  level 1 to level 10;
- from level 10 the multiplier grows by 0.5 per level: 10 at level 10,
  15 at level 20, ... 45 at level 80, reaching 50 at level 90.
round10 is Python's round(x / 10) * 10: nearest ten, exact .5 ties to the
even multiple (this matches the previous table). Changing monster HP changes
exp(L), so rerun both scripts.
Hours assume 5 seconds per kill, solo, monster EXP only (no daily/main
quests, elites, bosses, events or party EXP).
"""

import argparse
import csv
import json
import sys
from pathlib import Path

import monster_stats

MAX_LEVEL = 90
SECONDS_PER_KILL = 5
RAMP_END = 10


def exp_multiplier(level):
    if level < RAMP_END:
        return 4 * level / 3
    return 10 + 0.5 * (level - RAMP_END)


def kills(level):
    return (10 + 2 * (level - 1) ** 1.5) * exp_multiplier(level)


def round10(value):
    return round(value / 10) * 10


def exp_to_next(level):
    return round10(kills(level) * monster_stats.exp(level))


def rows():
    total, hours = 0, 0.0
    for level in range(1, MAX_LEVEL):
        need = exp_to_next(level)
        count = kills(level)
        total += need
        hours += count * SECONDS_PER_KILL / 3600
        yield level, need, round(count), total, round(hours, 2)


def to_json(table):
    payload = {"exp_to_next": [row[1] for row in table]}
    return json.dumps(payload, indent=2) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-json", action="store_true")
    parser.add_argument("--print-json", action="store_true")
    parser.add_argument("--write-csv", action="store_true")
    args = parser.parse_args()
    table = list(rows())
    assert exp_to_next(1) == 80 and exp_to_next(10) == 80000, (exp_to_next(1), exp_to_next(10))
    assert all(a[1] < b[1] for a, b in zip(table, table[1:]))
    hours_to_10 = table[RAMP_END - 2][4]
    assert 2.8 <= hours_to_10 <= 3.2, hours_to_10
    hours_to_50 = table[49][4]  # the level 50 row of game-design 5.8
    assert abs(hours_to_50 - 456.3) <= 0.05 * 456.3, hours_to_50
    text = to_json(table)
    if args.print_json:
        sys.stdout.write(text)
        return
    print(f"hours to level 10: {hours_to_10}, hours to level 50: {hours_to_50}")
    for row in table:
        if row[0] % 10 == 0 or row[0] in (1, 5, 9, 89):
            print(row)
    if args.write_json:
        path = Path(__file__).resolve().parents[2] / "data" / "exp.json"
        path.parent.mkdir(exist_ok=True)
        path.write_bytes(text.encode("utf-8"))
        print(f"Wrote {path}")
    if args.write_csv:
        path = Path(__file__).with_name("exp-table.csv")
        with path.open("w", encoding="utf-8", newline="") as stream:
            writer = csv.writer(stream)
            writer.writerow(["level", "exp_to_next", "same_level_normal_kills",
                             "cumulative_exp_to_reach_next", "cumulative_solo_hours"])
            writer.writerows(table)
        print(f"Wrote {path}")


if __name__ == "__main__":
    main()
