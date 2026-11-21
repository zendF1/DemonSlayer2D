"""Generate the main-line quests (data/quests.json), phase 6b spec 2.

Run from any directory: python3 docs/balance/quest_data.py
Add --write-json to write data/quests.json, --print-json to print the same
bytes to stdout (the Go test compares them with the committed file) and
--check to run the self-checks only (also run before writing or printing).
The Go server and the client only read data/quests.json.

Rewards (spec 2.3), with L = min_level, T = EXP to leave level L (data/exp.json)
and n = count:
- exp = round10(T * p / 100), p = 20 for kill/collect and 10 for talk, in
  integers: (T * p + 500) // 1000 * 10;
- xu = 8 * L * n for kill/collect, 40 * L for talk.
The region of a kill/collect objective is looked up in maps/*.json by the
monster and the level range. Rerun after changing exp.json or the maps.
"""

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SHARE_MIN_PCT, SHARE_MAX_PCT = 15, 30
LAST_CHAIN = 22  # m01..m22 are the chain; m23..m25 are level milestones.

TH = ("kumotori_1", "tho_san")
CH = ("ubuyashiki", "chi_huy")
TL = ("sagiri_1", "truong_lang")
TS = ("sagiri_2", "trinh_sat")
GK = ("fujikasane_1", "giam_khao")

POTION_HP_1 = {"code": "potion_hp_1", "count": 5}
SCROLLS = {"code": "scroll_return", "count": 2}
POTIONS_10 = [{"code": "potion_hp_10", "count": 10}, {"code": "potion_mp_10", "count": 10}]

# (id, name, level, giver, turn_in, objective, text, extra reward)
# objective: ("kill", monster, count, (lo, hi)), ("collect", monster, count,
# item_name, (lo, hi)) or ("talk",); (lo, hi) is the region level range.
QUESTS = [
    ("m01", "Quỷ lang thang trên đường tuyết", 1, TH, TH, ("kill", "wanderer", 5, (1, 2)),
     ("Quỷ lang thang đang làm loạn đường tuyết. Cậu hạ giúp ta năm con nhé?",
      "Quỷ lang thang còn lảng vảng ngoài kia, cố lên.",
      "Giỏi lắm! Đường tuyết yên ổn hơn rồi. Cầm lấy ít thuốc này."),
     {"items": [POTION_HP_1]}),
    ("m02", "Nanh quỷ", 2, TH, TH, ("collect", "wanderer", 6, "Nanh quỷ", (2, 3)),
     ("Ta cần sáu chiếc nanh của quỷ lang thang để làm bùa hộ thân.",
      "Chưa đủ nanh đâu, quỷ lang thang vẫn còn đầy ngoài kia.",
      "Đủ sáu chiếc nanh rồi. Cảm ơn cậu."),
     {}),
    ("m03", "Quỷ đói trong tuyết", 3, TH, TH, ("kill", "snow_ghoul", 8, (3, 4)),
     ("Quỷ đói đã xuất hiện ở phía nam. Hãy hạ tám con để dân làng bớt sợ.",
      "Quỷ đói vẫn đang lảng vảng ở phía nam.",
      "Tám con quỷ đói đã ngã xuống. Cậu khá hơn ta tưởng."),
     {}),
    ("m04", "Băng tinh", 4, TH, TH, ("collect", "snow_ghoul", 6, "Băng tinh", (4, 5)),
     ("Quỷ đói mang trong người băng tinh. Mang về cho ta sáu viên.",
      "Còn thiếu băng tinh, tiếp tục tìm quỷ đói nhé.",
      "Băng tinh đẹp lắm. Thuốc này là quà của ta."),
     {"items": [POTION_HP_1]}),
    ("m05", "Lên đường tới Tổng hành dinh", 5, TH, CH, ("talk",),
     ("Cậu đã đủ sức rồi. Hãy tới Tổng hành dinh gặp Chỉ huy, nói rằng ta cử cậu đến.",
      "Chỉ huy đang đợi ở Tổng hành dinh, qua cổng phía đông.",
      "Thợ săn già cử cậu tới ư? Tốt, Sát Quỷ Đoàn cần người như cậu."),
     {"items": [SCROLLS]}),
    ("m06", "Ngôi làng dưới chân núi", 6, CH, TL, ("talk",),
     ("Ngôi làng Sagiri đang cầu cứu. Hãy tới gặp Trưởng làng ở đó.",
      "Trưởng làng Sagiri ở phía đông Tổng hành dinh.",
      "Sát Quỷ Đoàn đã cử người tới! Làng ta mừng lắm."),
     {}),
    ("m07", "Quỷ rừng núi", 6, TL, TL, ("kill", "mountain_demon", 10, (6, 7)),
     ("Quỷ rừng núi hay xuống làng. Hạ mười con giúp làng ta.",
      "Quỷ rừng núi còn nhiều lắm, đừng dừng lại.",
      "Làng ta biết ơn cậu. Nhưng chuyện chưa xong đâu."),
     {}),
    ("m08", "Sừng quỷ", 7, TL, TL, ("collect", "mountain_demon", 8, "Sừng quỷ núi", (7, 8)),
     ("Thầy lang cần sừng quỷ núi để bào chế thuốc. Cậu thu giúp tám chiếc.",
      "Chưa đủ sừng quỷ núi, cậu tìm thêm nhé.",
      "Đúng thứ ta cần. Cảm ơn cậu."),
     {}),
    ("m09", "Kẻ phục kích", 8, TL, TL, ("kill", "ambusher", 10, (8, 9)),
     ("Kẻ phục kích núp trong bụi rậm hại người đi đường. Hạ mười tên.",
      "Kẻ phục kích vẫn núp ngoài kia, cẩn thận.",
      "Đường đã an toàn hơn nhờ cậu."),
     {}),
    ("m10", "Mảnh mặt nạ", 9, TL, TL, ("collect", "ambusher", 8, "Mảnh mặt nạ", (9, 10)),
     ("Chúng đeo mặt nạ lạ. Thu về tám mảnh để Sát Quỷ Đoàn tra xét.",
      "Còn thiếu mảnh mặt nạ, tìm tiếp đi.",
      "Mảnh mặt nạ này có dấu hiệu lạ. Hãy mang tin về Tổng hành dinh."),
     {}),
    ("m11", "Báo cáo Chỉ huy", 10, TL, CH, ("talk",),
     ("Cậu hãy về báo cáo Chỉ huy những gì đã thấy ở Sagiri.",
      "Chỉ huy đang đợi báo cáo của cậu ở Tổng hành dinh.",
      "Báo cáo rất hữu ích. Đây là sách chiêu thức và vũ khí cho cậu."),
     {"items": POTIONS_10, "equipment": [{"tier": 10, "slot": "weapon"}], "skill_book": 1}),
    ("m12", "Rừng sương mù", 11, CH, TS, ("talk",),
     ("Phía sau Sagiri là rừng sương mù. Hãy gặp Kiếm sĩ trinh sát ở đó.",
      "Kiếm sĩ trinh sát ở trại rừng sương mù, phía đông Sagiri.",
      "Chỉ huy cử cậu tới à? Rừng này nguy hiểm, theo ta."),
     {}),
    ("m13", "Quỷ ẩn sương", 11, TS, TS, ("kill", "mist_lurker", 12, (11, 12)),
     ("Quỷ ẩn trong sương rất khó thấy. Hạ mười hai con.",
      "Quỷ ẩn sương còn đó, giữ bình tĩnh.",
      "Cậu có mắt tinh đấy."),
     {}),
    ("m14", "Sương độc", 12, TS, TS, ("collect", "mist_lurker", 10, "Túi sương độc", (12, 13)),
     ("Túi sương độc của chúng dùng được làm thuốc giải. Thu mười túi.",
      "Chưa đủ túi sương độc.",
      "Mười túi sương độc, đủ cho cả đội."),
     {}),
    ("m15", "Quỷ vuốt dài", 13, TS, TS, ("kill", "long_claw", 12, (13, 14)),
     ("Quỷ vuốt dài đã làm bị thương nhiều kiếm sĩ. Hạ mười hai con.",
      "Quỷ vuốt dài vẫn còn trong rừng.",
      "Tốt lắm, các kiếm sĩ bớt nguy hiểm rồi."),
     {}),
    ("m16", "Vuốt gãy", 14, TS, TS, ("collect", "long_claw", 10, "Vuốt quỷ", (14, 15)),
     ("Vuốt quỷ rèn được vũ khí tốt. Thu về mười chiếc.",
      "Còn thiếu vuốt quỷ.",
      "Vuốt sắc thật. Cậu sẵn sàng cho kỳ tuyển chọn rồi."),
     {}),
    ("m17", "Kỳ tuyển chọn", 15, TS, GK, ("talk",),
     ("Kỳ tuyển chọn Sát Quỷ Đoàn sắp bắt đầu ở Fujikasane. Hãy gặp Giám khảo.",
      "Giám khảo ở trại Fujikasane, phía đông rừng sương mù.",
      "Cậu tới đúng lúc. Kỳ tuyển chọn sắp bắt đầu."),
     {"items": [SCROLLS]}),
    ("m18", "Quỷ Rừng", 16, GK, GK, ("kill", "forest_demon", 15, (16, 17)),
     ("Thử thách đầu tiên: hạ mười lăm Quỷ Rừng.",
      "Thử thách chưa xong, Quỷ Rừng vẫn còn.",
      "Thử thách đầu tiên đã qua."),
     {}),
    ("m19", "Rễ cây quỷ", 17, GK, GK, ("collect", "forest_demon", 12, "Rễ cây quỷ", (17, 18)),
     ("Thử thách thứ hai: thu mười hai rễ cây quỷ.",
      "Chưa đủ rễ cây quỷ.",
      "Đủ số rễ rồi. Cậu kiên trì đấy."),
     {}),
    ("m20", "Quỷ bị giam", 18, GK, GK, ("kill", "captive_demon", 15, (18, 19)),
     ("Thử thách thứ ba: hạ mười lăm quỷ bị giam.",
      "Quỷ bị giam vẫn còn, tiếp tục.",
      "Cậu đã vượt qua thử thách này."),
     {}),
    ("m21", "Mảnh xiềng", 19, GK, GK, ("collect", "captive_demon", 12, "Mảnh xiềng", (19, 20)),
     ("Thử thách cuối: thu mười hai mảnh xiềng của quỷ bị giam.",
      "Chưa đủ mảnh xiềng.",
      "Cậu đã hoàn thành mọi thử thách. Hãy về báo Chỉ huy."),
     {}),
    ("m22", "Vượt qua tuyển chọn", 20, GK, CH, ("talk",),
     ("Cậu đã vượt qua tuyển chọn. Hãy về Tổng hành dinh gặp Chỉ huy.",
      "Chỉ huy đang đợi ở Tổng hành dinh.",
      "Chúc mừng, cậu là thành viên Sát Quỷ Đoàn. Nhận thức thứ hai và vũ khí mới."),
     {"items": POTIONS_10, "equipment": [{"tier": 20, "slot": "weapon"}], "skill_book": 2}),
    ("m23", "Thức thứ ba", 30, CH, CH, ("talk",),
     ("Cậu đã đủ sức học thức thứ ba. Đây là sách của nó.",
      "Hãy đọc kỹ sách thức thứ ba.",
      "Thức thứ ba nay thuộc về cậu."),
     {"skill_book": 3}),
    ("m24", "Thức thứ tư", 40, CH, CH, ("talk",),
     ("Cậu đã đủ sức học thức thứ tư. Đây là sách của nó.",
      "Hãy đọc kỹ sách thức thứ tư.",
      "Thức thứ tư nay thuộc về cậu."),
     {"skill_book": 4}),
    ("m25", "Thức thứ năm", 50, CH, CH, ("talk",),
     ("Cậu đã đủ sức học thức thứ năm. Đây là sách của nó.",
      "Hãy đọc kỹ sách thức thứ năm.",
      "Thức thứ năm nay thuộc về cậu."),
     {"skill_book": 5}),
]


def load_exp():
    return json.loads((ROOT / "data" / "exp.json").read_text(encoding="utf-8"))["exp_to_next"]


def load_maps():
    return {p.stem: json.loads(p.read_text(encoding="utf-8")) for p in sorted((ROOT / "maps").glob("*.json"))}


def find_region(maps, map_id, monster, lo, hi):
    ids = [r["id"] for r in maps[map_id]["regions"]
           if r["monster"] == monster and (r["min_level"], r["max_level"]) == (lo, hi)]
    assert len(ids) == 1, (map_id, monster, lo, hi, ids)
    return ids[0]


def npc_ref(pair):
    return {"map": pair[0], "npc": pair[1]}


def build():
    exp, maps = load_exp(), load_maps()
    quests, prev = [], ""
    for n, (qid, name, level, giver, turn_in, obj, text, extra) in enumerate(QUESTS, start=1):
        kind = obj[0]
        if kind == "talk":
            objective, count, p = {"type": "talk"}, 1, 10
            xu = 40 * level
        else:
            monster, count = obj[1], obj[2]
            lo, hi = obj[-1]
            where = None
            for map_id, m in maps.items():
                if any(r["monster"] == monster and (r["min_level"], r["max_level"]) == (lo, hi) for r in m["regions"]):
                    where = map_id
            assert where, (qid, monster, lo, hi)
            objective = {"type": kind, "monster": monster, "count": count}
            if kind == "collect":
                objective["chance_bp"] = 5000
                objective["item_name"] = obj[3]
            objective["map"] = where
            objective["region"] = find_region(maps, where, monster, lo, hi)
            p = 20
            xu = 8 * level * count
        quests.append({
            "id": qid, "line": "main", "name": name, "min_level": level,
            "requires": prev if n <= LAST_CHAIN and n > 1 else "",
            "giver": npc_ref(giver), "turn_in": npc_ref(turn_in),
            "objective": objective,
            "text": {"offer": text[0], "progress": text[1], "done": text[2]},
            "reward": {"exp": (exp[level - 1] * p + 500) // 1000 * 10, "xu": xu,
                       "items": extra.get("items", []), "equipment": extra.get("equipment", []),
                       "skill_book": extra.get("skill_book", 0)},
        })
        prev = qid
    return {"quests": quests}


def exp_share(data):
    total = sum(load_exp()[:19])  # EXP from level 1 to level 20
    got = sum(q["reward"]["exp"] for q in data["quests"][:LAST_CHAIN])
    return got, total


def check(data):
    quests = data["quests"]
    assert len(quests) == 25, len(quests)
    ids = [q["id"] for q in quests]
    assert ids == [f"m{i:02d}" for i in range(1, 26)], ids
    for i, q in enumerate(quests):
        want = ids[i - 1] if 0 < i < LAST_CHAIN else ""
        assert q["requires"] == want, q["id"]
        assert all(q["text"].values()), q["id"]
        assert q["reward"]["exp"] > 0 and q["reward"]["xu"] > 0, q["id"]
    books = [q["reward"]["skill_book"] for q in quests if q["reward"]["skill_book"]]
    assert books == [1, 2, 3, 4, 5], books
    got, total = exp_share(data)
    pct = 100 * got / total
    assert SHARE_MIN_PCT <= pct <= SHARE_MAX_PCT, f"quest EXP share {pct:.1f}% outside {SHARE_MIN_PCT}-{SHARE_MAX_PCT}%"
    return pct


def to_json(data):
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-json", action="store_true")
    parser.add_argument("--print-json", action="store_true")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    data = build()
    pct = check(data)
    text = to_json(data)
    if args.print_json:
        sys.stdout.write(text)
        return
    got, total = exp_share(data)
    print(f"m01-m{LAST_CHAIN}: {got:,} of {total:,} EXP from level 1 to 20 ({pct:.1f}%)")
    if args.write_json:
        path = ROOT / "data" / "quests.json"
        path.write_bytes(text.encode("utf-8"))
        print(f"Wrote {path}")
    elif not args.check:
        print(f"{len(data['quests'])} quests (use --write-json to write)")


if __name__ == "__main__":
    main()
