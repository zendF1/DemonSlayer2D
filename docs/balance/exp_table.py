"""Generate the level-up EXP table.

Run from any directory: python3 docs/balance/exp_table.py
Add --write-csv to export exp-table.csv beside this script.

Base curve: going from level L to L + 1 needs the EXP of
10 + 2 * (L - 1) ** 1.5 same-level normal monsters (10 * L EXP each).
The base is multiplied by exp_multiplier(L) and rounded to the nearest 10:
- levels 1-9 ramp up as 4 * L / 3, about 3 hours of monster grinding from
  level 1 to level 10;
- from level 10 the multiplier grows by 0.5 per level: 10 at level 10,
  15 at level 20, ... 45 at level 80, reaching 50 at level 90.
Hours assume 5 seconds per kill, solo, monster EXP only (no daily/main
quests, elites, bosses, events or party EXP).
"""

import argparse
import csv
from pathlib import Path

MAX_LEVEL = 90
SECONDS_PER_KILL = 5
RAMP_END = 10


def exp_multiplier(level):
    if level < RAMP_END:
        return 4 * level / 3
    return 10 + 0.5 * (level - RAMP_END)


def exp_to_next(level):
    base = 10 * level * (10 + 2 * (level - 1) ** 1.5)
    return round(base * exp_multiplier(level) / 10) * 10


def rows():
    total, hours = 0, 0.0
    for level in range(1, MAX_LEVEL):
        need = exp_to_next(level)
        kills = need / (10 * level)
        total += need
        hours += kills * SECONDS_PER_KILL / 3600
        yield level, need, round(kills), total, round(hours, 2)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-csv", action="store_true")
    args = parser.parse_args()
    table = list(rows())
    assert all(a[1] < b[1] for a, b in zip(table, table[1:]))
    hours_to_10 = table[RAMP_END - 2][4]
    assert 2.8 <= hours_to_10 <= 3.2, hours_to_10
    for row in table:
        if row[0] % 10 == 0 or row[0] in (1, 5, 9, 89):
            print(row)
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
