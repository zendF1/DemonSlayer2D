"""Generate the map files (maps/*.json) from rectangle declarations.

Run from any directory:
  python3 tools/mapgen.py            print a summary of every map
  python3 tools/mapgen.py --write    write maps/*.json
  python3 tools/mapgen.py --check    compare with maps/*.json, print the maps
                                     that differ and exit 1 (a Go test runs it)

Phase 4b (spec 2.1-2.2) adds to every map "town" (the map a return scroll leads
to) and "npcs": the town has a general shop, a blacksmith and a carriage, each
training map an apothecary and a carriage. A carriage has a route to every
other map, 50 Xu to the town, otherwise max(50, 20 x the target's lowest region
min_level). Before writing, the script checks the NPCs in safe zones, reachable
and 1.5 tiles apart. Phase 6b (spec 2.5) adds the role "quest": one quest npc
per map, 2-4 tiles from default, no shop and no routes. Phase 6d (spec 2) adds
a second quest npc to the town, qua_truyen_tin, who gives the daily quests.

Phase 4d (spec 2.1-2.4) adds fourteen training maps of levels 21-90 in a
chain after fujikasane_1, each a camp with duoc_su, xa_phu and thu_kho (no
quest npc). Carriage routes are generated: every other map of {town, training
maps}, the town first, then by lowest region level; battle, instance and
event_only maps are left out.

Rules (phase 4a spec 2.1-2.3): each map is declared with its size, wall
rectangles (inside the border wall), safe zones, rest points, spawn points,
portals, monster regions and min_zones. A portal is a 1 x 4 tile rectangle in
column 1 (west) or width - 2 (east), rows height/2 - 2 .. height/2 + 1, named
to_<target map>; its arrival in the target map is the spawn point
from_<source map>, 3 tiles inside from the target map's reverse portal, on its
middle row. Before writing, the script checks every rule of spec 2.1 that it
can see, that every portal has a reverse, that every map is reachable from
kumotori_1 and that every region has enough region tiles (mirroring
mapdata.RegionTiles). Never edit maps/*.json by hand; edit this script.
"""

import argparse
import json
import math
import sys
from collections import deque
from pathlib import Path

MAPS_DIR = Path(__file__).resolve().parents[1] / "maps"
START_MAP = "kumotori_1"
TYPES = ("town", "training", "battle", "instance")
ARRIVAL_MIN_DISTANCE = 2  # tiles from every portal of the arrival's map
ROLES = ("shop", "blacksmith", "carriage", "storage", "quest")
STORAGE_MAX_DEFAULT_DISTANCE = 3.0  # phase 4c spec 2.2
# Phase 6b spec 2.5: each map has one quest npc 2-4 tiles from default; 6d
# adds qua_truyen_tin (daily quests) to the town.
QUEST_NPCS = {"kumotori_1": ["tho_san"], "ubuyashiki": ["chi_huy", "qua_truyen_tin"],
              "sagiri_1": ["truong_lang"], "sagiri_2": ["trinh_sat"], "fujikasane_1": ["giam_khao"]}
QUEST_DEFAULT_DISTANCE = (2.0, 4.0)
CAMP_NPCS = {"duoc_su": ("Dược sư", "shop"), "xa_phu": ("Xa phu", "carriage"), "thu_kho": ("Thủ kho", "storage")}
CAMP_DEFAULT_DISTANCE = (2.0, 4.0)  # phase 4b spec 2.2: apothecary and carriage
ROUTE_SKIP_TYPES = ("battle", "instance")  # and every event_only map (phase 4d spec 2.4)
NPC_MIN_DISTANCE = 1.5  # tiles between two NPCs of a map
MIN_FARE = 50  # Xu; a trip to a map costs 20 x its lowest region level, at least this
FARE_PER_LEVEL = 20


# ______________________DECLARATION HELPERS_____________________

def wall(x, y, w, h):
    """Wall tiles x..x+w-1 by y..y+h-1 (the border wall is always there)."""
    return ("wall", (x, y, w, h))


def safe(zone_id, x, y, w, h):
    return ("safe", {"id": zone_id, "x": x, "y": y, "w": w, "h": h})


def gate_safe(side):
    """The 6 x 8 safe zone gate_<side> around the arrival of a portal (spec 2.3)."""
    return ("gate", side)


def rest(rest_id, x, y, radius=1.5):
    return ("rest", {"id": rest_id, "x": float(x), "y": float(y), "radius": float(radius)})


def spawn(spawn_id, x, y, respawn=False):
    sp = {"id": spawn_id, "x": float(x), "y": float(y)}
    if respawn:
        sp["respawn"] = True
    return ("spawn", sp)


def portal(side, to_map):
    """Portal to_<to_map> on the west or east wall; its rect and to_spawn are computed."""
    return ("portal", {"side": side, "to_map": to_map})


def npc(npc_id, name, role, x, y, shop=None):
    """An NPC (phase 4b spec 2.2); a carriage's routes are computed in build_map."""
    return ("npc", {"id": npc_id, "name": name, "role": role, "shop": shop, "x": float(x), "y": float(y)})


def region(region_id, monster, lo, hi, count, rects, nest):
    return ("region", {
        "id": region_id, "monster": monster, "min_level": lo, "max_level": hi, "count": count,
        "rects": [{"x": x, "y": y, "w": w, "h": h} for x, y, w, h in rects],
        "nest": {"x": float(nest[0]), "y": float(nest[1])},
    })


def quarters(monsters, levels, counts, nests, walls=()):
    """The four regions of a new training map (spec 2.3): north-middle,
    south-middle, north-east, south-east, levels rising in that order (the
    middle is nearer the camp). monsters is (middle kind, east kind)."""
    rects = [(1, 1, 31, 23), (1, 24, 31, 23), (32, 1, 31, 23), (32, 24, 31, 23)]
    out = [wall(*w) for w in walls]
    for i in range(4):
        lo, hi = levels[i]
        out.append(region(f"r{i + 1}", monsters[i // 2], lo, hi, counts[i], [rects[i]], nests[i]))
    return out


def camp_west(quest_id=None, quest_name=None):
    """The camp of a new training map (spec 2.3): a 10 x 10 safe zone against
    the west wall at mid height, holding the west portal and its arrival, the
    default (respawn) spawn point in its middle and a bonfire. The maps of
    phase 4d have no quest npc."""
    parts = [safe("camp", 1, 19, 10, 10), rest("bonfire", 4.5, 21.5), spawn("default", 6.5, 24.5, respawn=True),
             npc("duoc_su", "Dược sư", "shop", 8.5, 21.5, shop="apothecary"),
             npc("xa_phu", "Xa phu", "carriage", 8.5, 27.5),
             npc("thu_kho", "Thủ kho", "storage", 8.5, 24.5)]
    if quest_id:
        parts.append(npc(quest_id, quest_name, "quest", 4.5, 27.5))
    return parts


def strip_regions(kinds, levels, count, walls=(), x0=12, x1=62, height=48):
    """Phase 4d spec 2.4 (yoshiwara_3): the area east of the camp, columns
    x0 .. x1 - 1, split into equal vertical strips of the full inner height,
    one region each, levels rising west to east, the nest at the strip's
    centre."""
    n = len(kinds)
    if (x1 - x0) % n:
        raise SystemExit(f"mapgen: columns {x0}-{x1} do not split into {n} equal strips")
    w = (x1 - x0) // n
    out = [wall(*wl) for wl in walls]
    for i, (kind, level) in enumerate(zip(kinds, levels)):
        x = x0 + i * w
        out.append(region(f"r{i + 1}", kind, level, level, count, [(x, 1, w, height - 2)],
                          (x + w // 2 + 0.5, height // 2 + 0.5)))
    return out


def car_partition(x, gaps, y0=1, y1=47):
    """Phase 4d spec 2.4 (mugen_*): a wall across the train, which runs west
    to east: column x from row y0 to y1 - 1 between two cars, open at each
    gap (y, h), h >= 3, so the player walks east from car to car."""
    out, y = [], y0
    for gy, gh in sorted(gaps):
        if gh < 3 or gy < y:
            raise SystemExit(f"mapgen: car wall column {x}: gap ({gy}, {gh}) narrower than 3 or overlapping")
        if gy > y:
            out.append((x, y, 1, gy - y))
        y = gy + gh
    if y < y1:
        out.append((x, y, 1, y1 - y))
    return out


# Phase 4d spec 2.1-2.2: the five-level maps after fujikasane_1, in chain
# order: (id, name, lowest level, (middle kind, east kind), decorative walls).
# Every map has the default nests of NESTS_4D; the walls keep off them.
NESTS_4D = [(18.5, 9.5), (18.5, 38.5), (46.5, 9.5), (46.5, 38.5)]
CHAIN_4D = [
    ("fujikasane_2", "Fujikasane – Vùng sâu", 21, ("big_demon", "many_arms"),
     # Wisteria thickets.
     [(14, 5, 3, 3), (24, 14, 3, 3), (14, 40, 3, 3), (24, 31, 3, 3), (36, 4, 3, 3), (54, 14, 3, 3),
      (40, 16, 3, 2), (36, 41, 3, 3), (54, 32, 3, 3), (40, 30, 3, 2)]),
    ("tay_bac_1", "Thị trấn Tây Bắc – Ngoại ô, đầm lầy", 26, ("marsh_demon", "mud_ambusher"),
     # Marsh pools.
     [(22, 4, 6, 4), (14, 14, 4, 3), (38, 6, 5, 4), (52, 15, 6, 3),
      (14, 33, 6, 3), (24, 40, 5, 4), (38, 36, 6, 4), (52, 41, 5, 3)]),
    ("asakusa_1", "Asakusa – Ngõ tối, khu ngoại vi", 31, ("new_demon", "city_wanderer"),
     # Rows of houses along dark alleys.
     [(14, 4, 8, 3), (26, 4, 4, 3), (14, 15, 8, 2), (36, 4, 8, 3), (50, 4, 8, 3), (36, 15, 8, 2), (50, 15, 6, 2),
      (14, 32, 8, 2), (14, 41, 8, 3), (36, 32, 8, 2), (36, 41, 8, 3), (50, 32, 6, 2), (50, 41, 8, 3)]),
    ("asakusa_2", "Asakusa – Khu chiến đấu", 36, ("henchman", "henchman"),
     # Barricades and broken walls.
     [(15, 6, 2, 6), (26, 12, 6, 2), (38, 5, 2, 7), (52, 10, 6, 2),
      (15, 34, 2, 6), (26, 34, 6, 2), (38, 36, 2, 7), (52, 36, 6, 2)]),
    ("tsuzumi_1", "Dinh thự Trống – Rừng ngoài dinh", 41, ("manor_woods", "long_nail"),
     # Trees round the manor.
     [(x, y, 2, 2) for x, y in [(14, 4), (22, 7), (28, 3), (13, 14), (25, 16), (36, 6), (43, 3), (51, 8),
                                (57, 4), (40, 15), (53, 15), (14, 33), (24, 31), (13, 42), (27, 41),
                                (37, 33), (44, 42), (52, 34), (58, 41), (39, 40)]]),
    ("tsuzumi_2", "Dinh thự Trống – Hành lang, phòng trống", 46, ("manor_dweller", "manor_dweller"),
     # Corridor walls and the corners of empty rooms.
     [(13, 5, 12, 1), (24, 5, 1, 8), (36, 5, 12, 1), (36, 5, 1, 8), (50, 12, 1, 9),
      (13, 42, 12, 1), (24, 35, 1, 8), (36, 42, 12, 1), (36, 35, 1, 8), (50, 27, 1, 9)]),
    ("natagumo_1", "Natagumo – Bìa rừng", 51, ("small_spider", "thread_puppet"),
     # Trees at the forest edge.
     [(14, 3, 3, 2), (26, 8, 2, 4), (13, 15, 4, 2), (36, 4, 3, 3), (48, 3, 2, 4), (55, 10, 3, 3), (42, 16, 4, 2),
      (14, 44, 3, 2), (26, 35, 2, 4), (13, 30, 4, 2), (36, 41, 3, 3), (48, 41, 2, 4), (55, 34, 3, 3),
      (42, 30, 4, 2)]),
    ("natagumo_2", "Natagumo – Rừng tơ nhện", 56, ("spider_demon", "silk_cocoon"),
     # Spider-silk strands between the trees.
     [(14, 4, 1, 5), (15, 8, 5, 1), (28, 12, 1, 6), (38, 4, 5, 1), (42, 5, 1, 5), (54, 12, 1, 6),
      (14, 39, 1, 5), (15, 39, 5, 1), (28, 30, 1, 6), (38, 43, 5, 1), (42, 38, 1, 5), (54, 30, 1, 6)]),
    ("natagumo_3", "Natagumo – Vùng lõi", 61, ("silk_fiend", "veteran_puppet"),
     # Old trunks and silk cocoons.
     [(20, 4, 4, 4), (13, 13, 3, 3), (27, 16, 3, 3), (40, 4, 4, 4), (52, 8, 4, 4), (44, 16, 3, 3),
      (20, 40, 4, 4), (13, 32, 3, 3), (27, 29, 3, 3), (40, 40, 4, 4), (52, 36, 4, 4), (44, 29, 3, 3)]),
    ("mugen_1", "Chuyến Tàu Vô Tận – Các toa đầu", 66, ("train_tentacle", "train_flesh"),
     # Car walls across the train split the map into four cars.
     car_partition(25, [(6, 4), (22, 4), (38, 4)]) + car_partition(39, [(12, 3), (23, 3), (33, 4)])
     + car_partition(53, [(8, 4), (22, 4), (40, 3)])),
    ("mugen_2", "Chuyến Tàu Vô Tận – Các toa cuối", 71, ("train_flesh", "train_nightmare"),
     car_partition(22, [(4, 3), (22, 4), (41, 3)]) + car_partition(36, [(14, 4), (30, 4)])
     + car_partition(50, [(5, 3), (22, 4), (38, 5)])),
    ("yoshiwara_1", "Yoshiwara – Ngõ đêm", 76, ("night_lurker", "sash_fragment"),
     # Lantern-lit houses of the pleasure district.
     [(14, 3, 6, 4), (23, 3, 6, 4), (14, 13, 6, 4), (36, 3, 6, 4), (45, 3, 6, 4), (36, 13, 6, 4), (53, 11, 5, 4),
      (14, 31, 6, 4), (14, 40, 6, 4), (23, 40, 6, 4), (36, 31, 6, 4), (36, 40, 6, 4), (45, 40, 6, 4),
      (53, 32, 5, 4)]),
    ("yoshiwara_2", "Yoshiwara – Đường ngầm", 81, ("night_lurker", "sash_fragment"),
     # Tunnel walls.
     [(12, 6, 18, 1), (12, 16, 14, 1), (34, 6, 20, 1), (38, 16, 16, 1),
      (12, 32, 14, 1), (12, 42, 18, 1), (38, 32, 16, 1), (34, 42, 20, 1)]),
]
# Yoshiwara - Hầm Ngầm (spec 2.2): five strips of one kind and one level each.
LAST_4D = ("yoshiwara_3", "Yoshiwara – Hầm Ngầm", ["mask_demon", "long_talon", "blood_fang", "shadow_demon",
                                                   "scale_serpent"], [86, 87, 88, 89, 90], 6)


# ______________________MAPS_____________________

MAPS = [
    {
        "id": "ubuyashiki", "name": "Tổng hành dinh Ubuyashiki", "type": "town",
        "width": 40, "height": 32, "min_zones": 1,
        "parts": [
            safe("town", 1, 1, 38, 30),
            # Houses north and south of the road (rows 12-20 stay open).
            wall(5, 3, 6, 5), wall(15, 2, 10, 6), wall(29, 3, 6, 5),
            wall(5, 24, 6, 5), wall(15, 25, 10, 4), wall(29, 24, 6, 5),
            rest("bonfire", 20.5, 12.5),
            spawn("default", 20.5, 16.5, respawn=True),
            npc("tap_hoa", "Tạp hóa", "shop", 23.5, 16.5, shop="general"),
            npc("tho_ren", "Thợ rèn", "blacksmith", 17.5, 16.5),
            npc("xa_phu", "Xa phu", "carriage", 20.5, 19.5),
            npc("thu_kho", "Thủ kho", "storage", 20.5, 14.5),
            npc("chi_huy", "Chỉ huy Sát Quỷ Đoàn", "quest", 18.5, 19.5),
            npc("qua_truyen_tin", "Quạ truyền tin", "quest", 22.5, 19.5),
            portal("west", "kumotori_1"),
            portal("east", "sagiri_1"),
        ],
    },
    {
        # The phase 1 layout, kept tile for tile so saved positions stay valid.
        "id": "kumotori_1", "name": "Kumotori – Đường núi tuyết", "type": "training",
        "width": 64, "height": 48, "min_zones": 2,
        "parts": [
            wall(10, 20, 15, 1), wall(28, 20, 13, 1), wall(45, 30, 4, 4),
            safe("camp", 1, 1, 10, 10),
            rest("bonfire", 3.5, 3.5),
            spawn("default", 5.5, 5.5, respawn=True),
            npc("duoc_su", "Dược sư", "shop", 8.5, 3.5, shop="apothecary"),
            npc("xa_phu", "Xa phu", "carriage", 8.5, 7.5),
            npc("thu_kho", "Thủ kho", "storage", 3.5, 7.5),
            npc("tho_san", "Thợ săn già", "quest", 5.5, 8.5),
            portal("east", "ubuyashiki"),
            gate_safe("east"),
            region("r1", "wanderer", 1, 2, 8, [(12, 1, 20, 19), (1, 12, 11, 8)], (22.5, 10.5)),
            region("r2", "wanderer", 2, 3, 7, [(32, 1, 31, 19)], (47.5, 9.5)),
            region("r3", "snow_ghoul", 3, 4, 7, [(1, 21, 31, 26)], (15.5, 34.5)),
            region("r4", "snow_ghoul", 4, 5, 8, [(32, 21, 31, 26)], (52.5, 40.5)),
        ],
    },
    {
        "id": "sagiri_1", "name": "Sagiri – Chân núi, đường đến ngôi đền", "type": "training",
        "width": 64, "height": 48, "min_zones": 2,
        "parts": camp_west("truong_lang", "Trưởng làng Sagiri") + [
            portal("west", "ubuyashiki"),
            portal("east", "sagiri_2"),
            gate_safe("east"),
        ] + quarters(
            ("mountain_demon", "ambusher"), [(6, 7), (7, 8), (8, 9), (9, 10)], [8, 7, 7, 8],
            [(16.5, 10.5), (16.5, 36.5), (47.5, 10.5), (47.5, 36.5)],
            # Rocks at the foot of the mountain.
            walls=[(20, 6, 5, 3), (26, 15, 3, 4), (38, 5, 4, 6), (52, 12, 5, 3),
                   (22, 30, 4, 5), (36, 38, 6, 3), (50, 30, 3, 5), (12, 40, 5, 2)],
        ),
    },
    {
        "id": "sagiri_2", "name": "Sagiri – Rừng sương mù", "type": "training",
        "width": 64, "height": 48, "min_zones": 2,
        "parts": camp_west("trinh_sat", "Kiếm sĩ trinh sát") + [
            portal("west", "sagiri_1"),
            portal("east", "fujikasane_1"),
            gate_safe("east"),
        ] + quarters(
            ("mist_lurker", "long_claw"), [(11, 12), (12, 13), (13, 14), (14, 15)], [8, 7, 7, 8],
            [(17.5, 8.5), (17.5, 37.5), (46.5, 8.5), (46.5, 37.5)],
            # Trees in rows north and south of the path (rows 20-27 stay open).
            walls=[(x, y, 2, 2) for y in (4, 11, 33, 40) for x in (14, 21, 28, 35, 42, 49)],
        ),
    },
    {
        "id": "fujikasane_1", "name": "Fujikasane – Rừng tuyển chọn", "type": "training",
        "width": 64, "height": 48, "min_zones": 2,
        "parts": camp_west("giam_khao", "Giám khảo tuyển chọn") + [
            portal("west", "sagiri_2"),
            # Phase 4d spec 1.3: the east portal on to fujikasane_2.
            portal("east", "fujikasane_2"),
            gate_safe("east"),
        ] + quarters(
            ("forest_demon", "captive_demon"), [(16, 17), (17, 18), (18, 19), (19, 20)], [8, 7, 7, 8],
            [(18.5, 8.5), (18.5, 39.5), (45.5, 8.5), (45.5, 39.5)],
            # Wisteria hedges.
            walls=[(14, 14, 12, 1), (38, 14, 12, 1), (14, 33, 12, 1), (38, 33, 12, 1),
                   (31, 3, 2, 8), (31, 37, 2, 8)],
        ),
    },
]




def chain_4d():
    """The fourteen maps of phase 4d spec 2.1-2.4, each linked west to the
    map before it and east to the map after it."""
    ids = ["fujikasane_1"] + [c[0] for c in CHAIN_4D] + [LAST_4D[0]]
    out = []
    for i, (mid, name, a, kinds, walls) in enumerate(CHAIN_4D):
        out.append({
            "id": mid, "name": name, "type": "training", "width": 64, "height": 48, "min_zones": 2,
            "parts": camp_west() + [portal("west", ids[i]), portal("east", ids[i + 2]), gate_safe("east")]
            + quarters(kinds, [(a, a + 1), (a + 1, a + 2), (a + 2, a + 3), (a + 3, a + 4)], [8, 7, 7, 8],
                       NESTS_4D, walls=walls),
        })
    mid, name, kinds, levels, count = LAST_4D
    out.append({
        "id": mid, "name": name, "type": "training", "width": 64, "height": 48, "min_zones": 2,
        "parts": camp_west() + [portal("west", ids[-2])] + strip_regions(
            kinds, levels, count,
            # Pillars of the underground hall.
            walls=[(x, y, 2, 2) for x in (15, 25, 35, 45, 55) for y in (8, 38)]),
    })
    return out


MAPS += chain_4d()

# The region levels of the phase 4d maps (spec 2.1-2.2):
# {map: [(region, kind, min_level, max_level, count)]}.
EXPECTED_4D = {mid: [("r1", k[0], a, a + 1, 8), ("r2", k[0], a + 1, a + 2, 7),
                     ("r3", k[1], a + 2, a + 3, 7), ("r4", k[1], a + 3, a + 4, 8)]
               for mid, _, a, k, _ in CHAIN_4D}
EXPECTED_4D[LAST_4D[0]] = [(f"r{i + 1}", k, lv, lv, LAST_4D[4])
                           for i, (k, lv) in enumerate(zip(LAST_4D[2], LAST_4D[3]))]


# ______________________BUILD_____________________

def portal_rect(side, width, height):
    x = 1 if side == "west" else width - 2
    return {"x": x, "y": height // 2 - 2, "w": 1, "h": 4}


def arrival(side, width, height):
    """The arrival point 3 tiles inside from a portal, on its middle row."""
    x = 4.5 if side == "west" else width - 4.5
    return float(x), float(height // 2)


def gate_rect(side, width, height):
    """6 x 8 tiles around the arrival of the portal on side, the portal included."""
    x = 1 if side == "west" else width - 7
    return {"x": x, "y": height // 2 - 4, "w": 6, "h": 8}


def town_id(specs):
    """The map a return scroll leads to: the one map of type town."""
    towns = [mid for mid, s in specs.items() if s["type"] == "town"]
    if len(towns) != 1:
        raise SystemExit(f"mapgen: want exactly one town map, got {towns}")
    return towns[0]


def fare(target):
    """Carriage price to a map (phase 4b spec 2.2): 50 to the town, otherwise
    20 x the lowest region min_level of the target, at least 50."""
    levels = [v["min_level"] for kind, v in target["parts"] if kind == "region"]
    if target["type"] == "town" or not levels:
        return MIN_FARE
    return max(MIN_FARE, FARE_PER_LEVEL * min(levels))


def lowest_level(spec):
    """The lowest region min_level of a map spec, 0 without regions."""
    return min((v["min_level"] for kind, v in spec["parts"] if kind == "region"), default=0)


def route_targets(map_id, specs):
    """The maps a carriage on map_id goes to (phase 4d spec 2.4): every other
    town or training map, the town first, then by lowest region level;
    battle, instance and event_only maps are left out."""
    towns = [mid for mid, s in specs.items() if s["type"] == "town"]
    training = sorted((mid for mid, s in specs.items()
                       if s["type"] not in ROUTE_SKIP_TYPES + ("town",) and not s.get("event_only")),
                      key=lambda mid: lowest_level(specs[mid]))
    return [mid for mid in towns + training if mid != map_id]


def build_map(spec, specs):
    """The JSON object of one map (spec 2.1 key order) and its collision grid."""
    width, height = spec["width"], spec["height"]
    if height % 2 or width < 8 or height < 8:
        raise SystemExit(f"mapgen {spec['id']}: size {width} x {height} must be even in height and at least 8")
    grid = [["#" if x in (0, width - 1) or y in (0, height - 1) else "." for x in range(width)]
            for y in range(height)]
    safe_zones, rest_points, spawns, portals, regions, npcs = [], [], [], [], [], []
    for kind, value in spec["parts"]:
        if kind == "wall":
            x, y, w, h = value
            for ty in range(y, y + h):
                for tx in range(x, x + w):
                    grid[ty][tx] = "#"
        elif kind == "safe":
            safe_zones.append(value)
        elif kind == "gate":
            safe_zones.append({"id": f"gate_{value}", **gate_rect(value, width, height)})
        elif kind == "rest":
            rest_points.append(value)
        elif kind == "spawn":
            spawns.append(value)
        elif kind == "portal":
            portals.append({"id": "to_" + value["to_map"], **portal_rect(value["side"], width, height),
                            "to_map": value["to_map"], "to_spawn": "from_" + spec["id"]})
        elif kind == "region":
            regions.append(value)
        elif kind == "npc":
            n = {"id": value["id"], "name": value["name"], "role": value["role"]}
            if value["shop"]:
                n["shop"] = value["shop"]
            n["x"], n["y"] = value["x"], value["y"]
            if value["role"] == "carriage":
                n["routes"] = [{"map": mid, "xu": fare(specs[mid])} for mid in route_targets(spec["id"], specs)]
            npcs.append(n)
        else:
            raise AssertionError(kind)
    # Arrivals: from_<other> beside this map's portal back to <other>.
    for kind, value in spec["parts"]:
        if kind == "portal":
            if value["to_map"] not in specs:
                raise SystemExit(f"mapgen {spec['id']}: portal to unknown map {value['to_map']}")
            x, y = arrival(value["side"], width, height)
            spawns.append({"id": "from_" + value["to_map"], "x": x, "y": y})
    out = {
        "id": spec["id"], "name": spec["name"], "type": spec["type"],
        "width": width, "height": height, "min_zones": spec["min_zones"],
        "collision": ["".join(row) for row in grid],
        "safe_zones": safe_zones, "rest_points": rest_points, "spawn_points": spawns,
        "portals": portals, "regions": regions,
        "town": town_id(specs), "npcs": npcs,
    }
    return out


def dump(m):
    """Fixed formatting: 2-space indent, one collision row or one object per line."""
    def compact(v):
        return json.dumps(v, ensure_ascii=False, separators=(", ", ": "))
    lines = ["{"]
    items = list(m.items())
    for i, (key, value) in enumerate(items):
        comma = "," if i < len(items) - 1 else ""
        if isinstance(value, list) and value:
            lines.append(f"  {json.dumps(key)}: [")
            for j, entry in enumerate(value):
                lines.append("    " + compact(entry) + ("," if j < len(value) - 1 else ""))
            lines.append("  ]" + comma)
        else:
            lines.append(f"  {json.dumps(key)}: {compact(value)}{comma}")
    lines.append("}")
    return "\n".join(lines) + "\n"


# ______________________CHECKS_____________________

class Grid:
    """The view of one built map that mapdata.Parse has."""

    def __init__(self, m):
        self.m = m
        self.width, self.height = m["width"], m["height"]
        self.rows = m["collision"]

    def blocked(self, tx, ty):
        if tx < 0 or ty < 0 or tx >= self.width or ty >= self.height:
            return True
        return self.rows[ty][tx] == "#"

    def safe_tile(self, tx, ty):
        return any(in_rect(z, tx, ty) for z in self.m["safe_zones"])

    def near_safe(self, tx, ty):
        return any(z["x"] - 1 <= tx < z["x"] + z["w"] + 1 and z["y"] - 1 <= ty < z["y"] + z["h"] + 1
                   for z in self.m["safe_zones"])

    def region_tiles(self, r):
        """mapdata.RegionTiles: walkable tiles in a rect, outside the safe zones grown by 1."""
        return [(tx, ty) for ty in range(self.height) for tx in range(self.width)
                if not self.blocked(tx, ty) and not self.near_safe(tx, ty)
                and any(in_rect(rc, tx, ty) for rc in r["rects"])]

    def rest_in_safe(self, r):
        """mapdata restInSafeZones: every tile the circle reaches is in a safe zone."""
        x, y, rad = r["x"], r["y"], r["radius"]
        for tx in range(math.floor(x - rad), math.floor(x + rad) + 1):
            for ty in range(math.floor(y - rad), math.floor(y + rad) + 1):
                nx = min(max(x, tx), tx + 1)
                ny = min(max(y, ty), ty + 1)
                if math.hypot(nx - x, ny - y) <= rad and not self.safe_tile(tx, ty):
                    return False
        return True

    def reachable(self, start):
        """Tiles reachable from start, four directions over walkable tiles."""
        seen = {start}
        queue = deque([start])
        while queue:
            tx, ty = queue.popleft()
            for nx, ny in ((tx + 1, ty), (tx - 1, ty), (tx, ty + 1), (tx, ty - 1)):
                if (nx, ny) not in seen and not self.blocked(nx, ny):
                    seen.add((nx, ny))
                    queue.append((nx, ny))
        return seen


def in_rect(r, tx, ty):
    return r["x"] <= tx < r["x"] + r["w"] and r["y"] <= ty < r["y"] + r["h"]


def tile(p):
    return math.floor(p["x"]), math.floor(p["y"])


def rect_tiles(r):
    return [(tx, ty) for ty in range(r["y"], r["y"] + r["h"]) for tx in range(r["x"], r["x"] + r["w"])]


def rect_distance(x, y, r):
    """Distance from (x, y) to the nearest point of [X, X+W] x [Y, Y+H]; 0 inside."""
    dx = max(r["x"] - x, 0, x - (r["x"] + r["w"]))
    dy = max(r["y"] - y, 0, y - (r["y"] + r["h"]))
    return math.hypot(dx, dy)


def check_map(m):
    """The per-map rules of spec 2.1 (mapdata.Parse); returns error strings."""
    g, mid, errs = Grid(m), m["id"], []

    def err(msg):
        errs.append(f"{mid}: {msg}")

    if m["type"] not in TYPES:
        err(f"invalid type {m['type']}")
    if not 1 <= m["min_zones"] <= 10:
        err(f"min_zones {m['min_zones']} not in 1-10")
    for z in m["safe_zones"]:
        if z["x"] < 0 or z["y"] < 0 or z["x"] + z["w"] > g.width or z["y"] + z["h"] > g.height:
            err(f"safe zone {z['id']} is outside the map")
    if not m["safe_zones"]:
        err("no safe zone")
    spawns = {}
    for sp in m["spawn_points"]:
        if sp["id"] in spawns:
            err(f"duplicate spawn point {sp['id']}")
        spawns[sp["id"]] = sp
        if g.blocked(*tile(sp)):
            err(f"spawn point {sp['id']} is not walkable")
    if "default" not in spawns:
        err("no default spawn point")
        return errs
    if not g.safe_tile(*tile(spawns["default"])):
        err("default spawn point is outside the safe zones")
    respawns = [sp for sp in m["spawn_points"] if sp.get("respawn")]
    if not respawns:
        err("no respawn point")
    for sp in respawns:
        if not g.safe_tile(*tile(sp)):
            err(f"respawn point {sp['id']} is outside the safe zones")
    for r in m["rest_points"]:
        if not g.rest_in_safe(r):
            err(f"rest point {r['id']} reaches outside the safe zones")
    if m["type"] == "town":
        if m["regions"]:
            err("a town has monster regions")
        for ty in range(g.height):
            for tx in range(g.width):
                if not g.blocked(tx, ty) and not g.safe_tile(tx, ty):
                    err(f"walkable tile ({tx},{ty}) of a town is outside the safe zones")
                    break
    portal_tiles = {}
    for p in m["portals"]:
        if p["to_map"] == mid:
            err(f"portal {p['id']} leads to its own map")
        for t in rect_tiles(p):
            if g.blocked(*t):
                err(f"portal {p['id']} tile {t} is not walkable")
            if t in portal_tiles:
                err(f"portals {portal_tiles[t]} and {p['id']} share tile {t}")
            portal_tiles[t] = p["id"]
    if len({p["id"] for p in m["portals"]}) != len(m["portals"]):
        err("duplicate portal id")
    owner = {}
    for r in m["regions"]:
        tiles = g.region_tiles(r)
        if len(tiles) < r["count"]:
            err(f"region {r['id']} has {len(tiles)} region tiles, want at least {r['count']}")
        for t in tiles:
            if t in owner:
                err(f"regions {owner[t]} and {r['id']} share tile {t}")
                break
            owner[t] = r["id"]
        if tile(r["nest"]) not in tiles:
            err(f"region {r['id']}: nest is not on a region tile")
    # Reachability from the default tile.
    reach = g.reachable(tile(spawns["default"]))
    npc_ids = set()
    for n in m["npcs"]:
        if not n["id"] or n["id"] in npc_ids:
            err(f"npc id {n['id']!r} is empty or duplicate")
        npc_ids.add(n["id"])
        if not n["name"]:
            err(f"npc {n['id']} has no name")
        if n["role"] not in ROLES:
            err(f"npc {n['id']}: invalid role {n['role']}")
        if ("shop" in n) != (n["role"] == "shop") or n.get("shop") == "":
            err(f"npc {n['id']}: shop must be set exactly for the shop role")
        routes = n.get("routes", [])
        if bool(routes) != (n["role"] == "carriage"):
            err(f"npc {n['id']}: routes must be set exactly for the carriage role")
        if len({r["map"] for r in routes}) != len(routes) or any(r["map"] == mid or r["xu"] < 1 for r in routes):
            err(f"npc {n['id']}: routes must be distinct, not to this map, with xu >= 1")
        if g.blocked(*tile(n)):
            err(f"npc {n['id']} is not walkable")
        elif not g.safe_tile(*tile(n)):
            err(f"npc {n['id']} is outside the safe zones")
        elif tile(n) not in reach:
            err(f"npc {n['id']} is unreachable from default")
    keepers = [n for n in m["npcs"] if n["role"] == "storage"]
    if len(keepers) != 1 or keepers[0]["id"] != "thu_kho" or keepers[0]["name"] != "Thủ kho":
        err("the map needs exactly one storage npc thu_kho (Thủ kho)")
    for n in keepers:
        d = math.hypot(n["x"] - spawns["default"]["x"], n["y"] - spawns["default"]["y"])
        if d > STORAGE_MAX_DEFAULT_DISTANCE:
            err(f"npc {n['id']} is {d:.2f} tiles from default, more than {STORAGE_MAX_DEFAULT_DISTANCE}")
    quest_ids = sorted(n["id"] for n in m["npcs"] if n["role"] == "quest")
    if quest_ids != sorted(QUEST_NPCS.get(m["id"], [])):
        err(f"the map needs exactly the quest npcs {QUEST_NPCS.get(m['id'], [])}, has {quest_ids}")
    if m["type"] == "training":
        # Phase 4d spec 2.4: exactly the three camp NPCs besides quest npcs.
        camp = {n["id"]: n for n in m["npcs"] if n["role"] != "quest"}
        if sorted(camp) != sorted(CAMP_NPCS) or any(
                (camp[i]["name"], camp[i]["role"]) != CAMP_NPCS[i] for i in camp):
            err(f"a training map needs exactly the camp npcs {sorted(CAMP_NPCS)}, has {sorted(camp)}")
        elif camp["duoc_su"].get("shop") != "apothecary":
            err("npc duoc_su must sell the apothecary shop")
        for i in ("duoc_su", "xa_phu"):
            if i in camp:
                d = math.hypot(camp[i]["x"] - spawns["default"]["x"], camp[i]["y"] - spawns["default"]["y"])
                if not CAMP_DEFAULT_DISTANCE[0] <= d <= CAMP_DEFAULT_DISTANCE[1]:
                    err(f"npc {i} is {d:.2f} tiles from default, outside {CAMP_DEFAULT_DISTANCE}")
        if "min_level" in m:
            err("a training map has no min_level")
    for n in m["npcs"]:
        if n["role"] == "quest":
            d = math.hypot(n["x"] - spawns["default"]["x"], n["y"] - spawns["default"]["y"])
            if not QUEST_DEFAULT_DISTANCE[0] <= d <= QUEST_DEFAULT_DISTANCE[1]:
                err(f"npc {n['id']} is {d:.2f} tiles from default, outside {QUEST_DEFAULT_DISTANCE}")
    for i, a in enumerate(m["npcs"]):
        for b in m["npcs"][i + 1:]:
            if math.hypot(a["x"] - b["x"], a["y"] - b["y"]) < NPC_MIN_DISTANCE:
                err(f"npcs {a['id']} and {b['id']} are closer than {NPC_MIN_DISTANCE} tiles")
    for sp in m["spawn_points"]:
        if tile(sp) not in reach:
            err(f"spawn point {sp['id']} is unreachable from default")
    for p in m["portals"]:
        if not any(t in reach for t in rect_tiles(p)):
            err(f"portal {p['id']} is unreachable from default")
    for r in m["regions"]:
        if tile(r["nest"]) not in reach:
            err(f"region {r['id']}: nest is unreachable from default")
    for r in m["rest_points"]:
        if tile(r) not in reach:
            err(f"rest point {r['id']} is unreachable from default")
    return errs


def check_world(built):
    """The cross-map rules: targets, arrivals, reverse portals, connectivity."""
    errs = []
    for mid, m in built.items():
        town = built.get(m["town"])
        if town is None or town["type"] != "town" or (m["type"] == "town" and m["town"] != mid):
            errs.append(f"{mid}: town {m['town']} is not a valid town")
        for n in m["npcs"]:
            for r in n.get("routes", []):
                if r["map"] not in built:
                    errs.append(f"{mid}: npc {n['id']}: route to unknown map {r['map']}")
                elif r["xu"] != fare(SPECS[r["map"]]):
                    errs.append(f"{mid}: npc {n['id']}: route to {r['map']} costs {r['xu']}, want {fare(SPECS[r['map']])}")
        for p in m["portals"]:
            target = built.get(p["to_map"])
            if target is None:
                errs.append(f"{mid}: portal {p['id']} leads to unknown map {p['to_map']}")
                continue
            arrival_point = next((sp for sp in target["spawn_points"] if sp["id"] == p["to_spawn"]), None)
            if arrival_point is None:
                errs.append(f"{mid}: portal {p['id']}: {p['to_map']} has no spawn point {p['to_spawn']}")
                continue
            for q in target["portals"]:
                d = rect_distance(arrival_point["x"], arrival_point["y"], q)
                if d < ARRIVAL_MIN_DISTANCE:
                    errs.append(f"{p['to_map']}: arrival {p['to_spawn']} is {d:.2f} tiles from portal {q['id']}")
            if not any(q["to_map"] == mid for q in target["portals"]):
                errs.append(f"{mid}: portal {p['id']} has no reverse portal in {p['to_map']}")
    for mid, m in built.items():
        for n in m["npcs"]:
            if n["role"] == "carriage":
                got = [r["map"] for r in n["routes"]]
                want = route_targets(mid, SPECS)
                if got != want:
                    errs.append(f"{mid}: npc {n['id']}: routes {got}, want {want}")
    for mid, want in EXPECTED_4D.items():
        m = built.get(mid)
        got = m and [(r["id"], r["monster"], r["min_level"], r["max_level"], r["count"]) for r in m["regions"]]
        if got != want:
            errs.append(f"{mid}: regions {got}, want {want} (phase 4d spec 2.1-2.2)")
    seen, queue = {START_MAP}, deque([START_MAP])
    while queue:
        for p in built[queue.popleft()]["portals"]:
            if p["to_map"] in built and p["to_map"] not in seen:
                seen.add(p["to_map"])
                queue.append(p["to_map"])
    for mid in built:
        if mid not in seen:
            errs.append(f"{mid}: unreachable from {START_MAP} through portals")
    return errs


SPECS = {s["id"]: s for s in MAPS}


def build():
    """Every map, built and checked: {id: JSON object}."""
    specs = {s["id"]: s for s in MAPS}
    if len(specs) != len(MAPS) or START_MAP not in specs:
        raise SystemExit(f"mapgen: duplicate map ids or no {START_MAP}")
    built = {mid: build_map(s, specs) for mid, s in specs.items()}
    errs = [e for m in built.values() for e in check_map(m)] + check_world(built)
    if errs:
        raise SystemExit("mapgen: checks failed:\n  " + "\n  ".join(errs))
    return built


def summary(m):
    walkable = sum(row.count(".") for row in m["collision"])
    monsters = sum(r["count"] for r in m["regions"])
    portals = ", ".join(f"{p['id']} ({p['x']},{p['y']})" for p in m["portals"])
    return (f"{m['id']}: {m['width']} x {m['height']} {m['type']}, min_zones {m['min_zones']}, "
            f"{walkable} walkable tiles, {monsters} monsters, portals: {portals}")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--write", action="store_true", help="write maps/*.json")
    mode.add_argument("--check", action="store_true", help="compare with maps/*.json, exit 1 on a difference")
    args = parser.parse_args()
    built = build()
    texts = {mid: dump(m).encode("utf-8") for mid, m in built.items()}
    if args.write:
        MAPS_DIR.mkdir(exist_ok=True)
        for mid, text in texts.items():
            (MAPS_DIR / f"{mid}.json").write_bytes(text)
            print(f"wrote maps/{mid}.json")
        return
    if args.check:
        bad = []
        for mid, text in texts.items():
            path = MAPS_DIR / f"{mid}.json"
            if not path.exists() or path.read_bytes() != text:
                bad.append(mid)
        bad += [p.stem + " (not generated)" for p in sorted(MAPS_DIR.glob("*.json")) if p.stem not in texts]
        if bad:
            print("maps differ from tools/mapgen.py: " + ", ".join(bad) + "; run it with --write")
            sys.exit(1)
        print(f"maps match tools/mapgen.py ({len(texts)} maps)")
        return
    for m in built.values():
        print(summary(m))


if __name__ == "__main__":
    main()
