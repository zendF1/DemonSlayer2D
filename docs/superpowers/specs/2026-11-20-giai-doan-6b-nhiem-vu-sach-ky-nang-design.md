# Giai đoạn 6b — Nhiệm vụ chính tuyến và sách kỹ năng

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ hai của Giai đoạn 6 (xem bảng chia ở [spec 6a](2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.2 (nhiệm vụ là nguồn EXP bổ sung), 2.6 (sách kỹ năng từ chính tuyến), 4.1 (NPC và biểu tượng nhiệm vụ), 4.2 (tuyến và mục tiêu), 4.3 (chạm nhiệm vụ để đi) và 5.8 (không chia tiến độ qua tổ đội); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 6a, [spec 4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) (NPC, tầm NPC), [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) (hành trang, người kết liễu) và [spec Giai đoạn 2](2026-11-20-giai-doan-2-ky-nang-design.md) (học chiêu). 6b chạy sau khi 6a đã triển khai (giao thức 13, migration 00008).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Ở trại mỗi map luyện cấp và ở Tổng hành dinh có NPC nhiệm vụ, trên đầu có dấu **!** vàng khi có nhiệm vụ mới, **?** xám khi đang làm, **?** vàng khi trả được. Nhân vật mới ở Kumotori nhận chuỗi chính tuyến từ Thợ săn già: hạ quái, thu thập, rồi đi qua cổng gặp NPC ở map khác. Chuỗi dẫn người chơi từ Kumotori qua Tổng hành dinh, Sagiri tới Fujikasane, cấp 1 tới 20. Ở cấp 10 và cấp 20, Chỉ huy ở Tổng hành dinh trao **sách kỹ năng** đầu tiên và thứ hai của class cùng một vũ khí; ở cấp 30, 40, 50 có nhiệm vụ mốc trao sách thứ ba, tư, năm. Học chiêu lần đầu nay cần sách. Bên phải màn hình có khung theo dõi nhiệm vụ; chạm một dòng thì nhân vật tự đi tới mục tiêu (bãi quái hoặc NPC) bằng đường đi của 6a, kể cả ở map khác.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Lượng nội dung v1 | Chính tuyến 22 nhiệm vụ nối nhau cho cấp 1–20 đi theo năm map của 4a, cộng 3 nhiệm vụ mốc ở cấp 30, 40, 50; tổng 25 (mục 2.2). Phụ tuyến không làm; hàng ngày ở 6d | Đủ dẫn người chơi qua toàn bộ nội dung map hiện có và trao đủ năm sách; map cấp 21+ làm sau thì chuỗi viết tiếp bằng cùng script |
| Định dạng dữ liệu | `data/quests.json` sinh bằng `python3 docs/balance/quest_data.py --write-json`; không sửa tay. Script đọc `data/exp.json` để tính thưởng và `maps/*.json` để tra id vùng quái; server và client cùng đọc | Như `skills.json`, `items.json`: một nguồn số liệu, kiểm tra được |
| Một nhiệm vụ | Đúng một mục tiêu, một NPC giao và một NPC trả (có thể là một). "Bước" của game-design 4.3 là nhiệm vụ kế tiếp trong chuỗi | Đơn giản cho dữ liệu, giao thức và theo dõi; chuỗi đủ diễn tả nhiều bước |
| Loại mục tiêu (6b) | `kill` (hạ N con một loại quái), `collect` (thu thập N vật phẩm nhiệm vụ rơi từ một loại quái với tỷ lệ), `talk` (đưa tin: đủ điều kiện ngay khi nhận, trả ở NPC khác). 6d thêm loại cho hàng ngày | Game-design 4.2 "diệt quái, thu thập, đưa thư/trò chuyện"; hái/đào, Boss, hộ tống cần hệ thống chưa có |
| Vật phẩm nhiệm vụ | Không là vật phẩm thật: chỉ là bộ đếm của nhiệm vụ ("Nanh quỷ 3/6"), không chiếm hành trang, không rơi xuống đất | Game-design 4.2 "không chiếm hành trang chính"; không cần kho riêng hay luật nhặt |
| Ai được tính | Người kết liễu con quái (như đồ rơi, game-design 5.8), kể cả khi đang chết; người đã rời zone thì không. Không xét cửa sổ cấp | Một luật với đồ rơi; không chia qua tổ đội (game-design 4.2, 5.8); nhân vật cấp cao làm lại chuỗi cấp thấp vẫn xong được |
| Mở nhiệm vụ | Đã xong nhiệm vụ trước trong chuỗi (`requires`) và cấp nhân vật ≥ `min_level`. Ba nhiệm vụ mốc chỉ cần cấp | Chuỗi dẫn đường; nhân vật cấp cao chưa làm chuỗi vẫn lấy được sách 30/40/50 |
| Số nhiệm vụ nhận cùng lúc | Không giới hạn (chính tuyến thực tế có tối đa hai: một trong chuỗi, một mốc) | Không cần luật |
| Hủy nhiệm vụ | Không có | Mọi nhiệm vụ đều làm được; bớt một lệnh |
| Thưởng EXP | `kill`/`collect`: 20% EXP lên cấp kế tiếp ở `min_level`; `talk`: 10%; làm tròn 10 (mục 2.3). Chuỗi cấp 1–20 cho khoảng 22% tổng EXP từ cấp 1 lên 20 | Game-design 2.2: nhiệm vụ là nguồn EXP bổ sung chính; ra thưởng theo % một cấp để giữ đúng tỷ lệ ở mọi cấp; số tạm, cân chỉnh sau chơi thử |
| Thưởng Xu | `kill`/`collect`: 8 × cấp × số lượng (gấp đôi Xu trung bình của số quái đó); `talk`: 40 × cấp | Xu quái trung bình 4 × cấp mỗi con (game-design 5.12) |
| Thưởng vật phẩm | Bình, giấy về thành ở vài nhiệm vụ; vũ khí class Trắng +0 bậc 10 ở cấp 10 và bậc 20 ở cấp 20 | "Trang bị tân thủ" của game-design 4.2; Trắng không lấn đồ rơi |
| Sách kỹ năng | 25 vật phẩm chồng `book_<id chiêu>` ("Sách Bất Tri Hỏa"), không bán được, không dùng bằng `UseItemReq`. Nhiệm vụ thưởng sách theo **ô chiêu 1–5 của class** người trả | Game-design 2.6: sách từ chính tuyến, dùng khi học lần đầu |
| Sách 1, 2 | Cuối chặng cấp 10 (m11) và cấp 20 (m22) của chuỗi | Đúng mốc học 10 và 20 |
| Sách 3, 4, 5 | Nhiệm vụ mốc m23, m24, m25 ở Chỉ huy, cần cấp 30, 40, 50, không cần chuỗi | Chưa có map cấp 21+; người chơi tới cấp 30 vẫn có sách |
| Học chiêu | Học lần đầu (cấp chiêu 0 → 1) cần một sách trong **hành trang** và tiêu sách; nâng tiếp không cần. Sách trong kho không tính | Game-design 2.6; kho chỉ để cất |
| Nhân vật đã có | Cột `characters.skill_book_waiver` (miễn sách) đặt bằng cấp hiện tại khi migrate: chiêu có cấp học ≤ giá trị này học lần đầu không cần sách. Nhân vật mới có 0 | Không khóa chiêu mà nhân vật đã đủ cấp học trước khi có sách; một cột, không cần dữ liệu nhiệm vụ trong SQL |
| Sách thừa | Nhiệm vụ chỉ trao sách khi người trả **cần** sách đó (chưa học chiêu và cấp học lớn hơn mức miễn); nếu không thì bỏ phần sách, các thưởng khác giữ | Không để sách vô dụng trong túi |
| Hành trang đầy khi trả | Không đủ chỗ cho cả lô vật phẩm thưởng (luật 3a mục 4.1) thì `BAG_FULL`, không trả gì | Như mua hàng ở 4b |
| NPC nhiệm vụ | Vai NPC mới `quest` trong file map (4b): Thợ săn già (Kumotori), Chỉ huy Sát Quỷ Đoàn (Tổng hành dinh), Trưởng làng Sagiri (Sagiri 1), Kiếm sĩ trinh sát (Sagiri 2), Giám khảo tuyển chọn (Fujikasane) | Game-design 4.1 "NPC giao nhiệm vụ/hướng dẫn: trưởng làng, chỉ huy…" |
| Biểu tượng | Thứ tự ưu tiên khi NPC có nhiều nhiệm vụ: **? vàng** (trả được) > **! vàng** (nhận được) > **? xám** (đang làm, NPC này trả) | Game-design 10.7 để mở; việc làm được ngay lên trước |
| Tương tác | Chạm NPC → đi tới (6a) → cửa sổ NPC có danh sách nhiệm vụ của NPC; chạm một nhiệm vụ mở thẻ nhiệm vụ: lời thoại, mục tiêu, thưởng, nút **Nhận** hoặc **Trả** | Cửa sổ NPC của 4b đã có; một thẻ cho mọi trạng thái |
| Khung theo dõi | Bên phải màn hình, tối đa 4 dòng: nhiệm vụ đang làm (tiến độ hoặc "Đã xong — về gặp <NPC>"), rồi nhiệm vụ chính tuyến nhận được kế tiếp ("Gặp <NPC> nhận <tên>"), hoặc dòng khóa "Đạt cấp N để nhận <tên>". Chạm dòng thì đi tới đích (mục 6.3) | Game-design 4.3 "chạm nhiệm vụ để đi đến mục tiêu"; không cần cửa sổ nhật ký riêng |
| Tới bãi | Nhiệm vụ `kill`/`collect` đi tới bãi của mục tiêu rồi **dừng**, không chọn quái, không bật auto | Game-design 4.3 |
| Lưu | Nhận và trả lưu ngay; tiến độ lưu theo nhịp định kỳ và các lần lưu khác | Trả có tài sản; tiến độ đổi quá thường xuyên, sập server chỉ mất ít tiến độ |
| Tiến độ qua tổ đội | Không chia (đã chốt ở 5b) | Game-design 4.2, 5.8 |

### 1.3. Khác với game-design

- Game-design 2.6 nói học lần đầu cần sách; spec này thêm **mức miễn sách** cho nhân vật có từ trước 6b (mục 1.2) và chốt nguồn sách 3–5 là nhiệm vụ mốc không cần chuỗi. Mục 2.6 được ghi thêm (mục 9).
- Game-design 4.1 để mở thứ tự biểu tượng (mục 10.7); spec này chốt (mục 1.2), mục 10.7 bỏ.

### 1.4. Không làm ở 6b

Nhiệm vụ hàng ngày và làm mới (6d); phụ tuyến; bang hội/tuần hoàn; hái/đào, săn Boss, hộ tống; hủy nhiệm vụ; cửa sổ nhật ký nhiệm vụ riêng; lời thoại nhiều trang; nhiệm vụ hướng dẫn tính năng (học chiêu, cường hóa, mua bình) — để sau; điểm năng động; danh vọng.

## 2. Dữ liệu

### 2.1. `data/quests.json`

```json
{
  "quests": [
    {
      "id": "m01", "line": "main", "name": "Quỷ lang thang trên đường tuyết",
      "min_level": 1, "requires": "",
      "giver": {"map": "kumotori_1", "npc": "tho_san"},
      "turn_in": {"map": "kumotori_1", "npc": "tho_san"},
      "objective": {"type": "kill", "monster": "wanderer", "count": 5,
                    "map": "kumotori_1", "region": "r1"},
      "text": {"offer": "…", "progress": "…", "done": "…"},
      "reward": {"exp": 20, "xu": 40, "items": [{"code": "potion_hp_1", "count": 5}],
                 "equipment": [], "skill_book": 0}
    },
    {
      "id": "m02", "line": "main", "name": "Nanh quỷ", "min_level": 2, "requires": "m01",
      "objective": {"type": "collect", "monster": "wanderer", "count": 6, "chance_bp": 5000,
                    "item_name": "Nanh quỷ", "map": "kumotori_1", "region": "r2"}
    },
    {
      "id": "m11", "line": "main", "name": "Báo cáo Chỉ huy", "min_level": 10, "requires": "m10",
      "giver": {"map": "sagiri_1", "npc": "truong_lang"},
      "turn_in": {"map": "ubuyashiki", "npc": "chi_huy"},
      "objective": {"type": "talk"},
      "reward": {"exp": 9150, "xu": 400,
                 "items": [{"code": "potion_hp_10", "count": 10}, {"code": "potion_mp_10", "count": 10}],
                 "equipment": [{"tier": 10, "slot": "weapon"}], "skill_book": 1}
    }
  ]
}
```

(Ví dụ rút gọn; mọi nhiệm vụ có đủ các trường.)

- `line`: `main` ở 6b (6d thêm `daily`).
- `requires`: id nhiệm vụ phải xong trước, hoặc rỗng.
- `giver`, `turn_in`: map và id NPC vai `quest` trên map đó.
- `objective`: `type` và theo loại: `kill` có `monster`, `count`, `map`, `region`; `collect` có thêm `chance_bp` (1–10.000) và `item_name`; `talk` không có trường khác (số lượng coi là 1). `map` và `region` là bãi để đi tới (khung theo dõi), không giới hạn nơi được tính.
- `text`: ba câu tiếng Việt cho thẻ nhiệm vụ: lúc mời nhận, lúc đang làm, lúc trả.
- `reward`: `exp`, `xu` (số nguyên ≥ 0, script tính theo mục 2.3); `items` (vật phẩm chồng, `count` 1 tới `stack_max`); `equipment` (mỗi mục `tier` và `slot`; ô `weapon` là vũ khí đúng class người trả; món Trắng +0, không dòng, không hệ); `skill_book` (0 là không có, 1–5 là ô chiêu theo thứ tự cấp học: sách `book_<class>_<n>` của người trả).

### 2.2. Chuỗi chính tuyến

| id | Tên | Cấp | Giao → Trả | Mục tiêu | EXP | Xu | Thưởng thêm |
| --- | --- | --- | --- | --- | --- | --- | --- |
| m01 | Quỷ lang thang trên đường tuyết | 1 | Thợ săn → Thợ săn | hạ 5 `wanderer` (vùng 1–2) | 20 | 40 | 5 Bình HP cấp 1 |
| m02 | Nanh quỷ | 2 | Thợ săn → Thợ săn | thu 6 "Nanh quỷ" từ `wanderer` (50%, vùng 2–3) | 70 | 96 | |
| m03 | Quỷ đói trong tuyết | 3 | Thợ săn → Thợ săn | hạ 8 `snow_ghoul` (vùng 3–4) | 240 | 192 | |
| m04 | Băng tinh | 4 | Thợ săn → Thợ săn | thu 6 "Băng tinh" từ `snow_ghoul` (50%, vùng 4–5) | 610 | 192 | 5 Bình HP cấp 1 |
| m05 | Lên đường tới Tổng hành dinh | 5 | Thợ săn → Chỉ huy | đưa tin | 620 | 200 | 2 Giấy về thành |
| m06 | Ngôi làng dưới chân núi | 6 | Chỉ huy → Trưởng làng | đưa tin | 1.220 | 240 | |
| m07 | Quỷ rừng núi | 6 | Trưởng làng → Trưởng làng | hạ 10 `mountain_demon` (vùng 6–7) | 2.430 | 480 | |
| m08 | Sừng quỷ | 7 | Trưởng làng → Trưởng làng | thu 8 "Sừng quỷ núi" từ `mountain_demon` (50%, vùng 7–8) | 4.340 | 448 | |
| m09 | Kẻ phục kích | 8 | Trưởng làng → Trưởng làng | hạ 10 `ambusher` (vùng 8–9) | 7.130 | 640 | |
| m10 | Mảnh mặt nạ | 9 | Trưởng làng → Trưởng làng | thu 8 "Mảnh mặt nạ" từ `ambusher` (50%, vùng 9–10) | 11.270 | 576 | |
| m11 | Báo cáo Chỉ huy | 10 | Trưởng làng → Chỉ huy | đưa tin | 9.150 | 400 | **Sách 1**, vũ khí bậc 10, 10 Bình HP cấp 10, 10 Bình MP cấp 10 |
| m12 | Rừng sương mù | 11 | Chỉ huy → Trinh sát | đưa tin | 13.460 | 440 | |
| m13 | Quỷ ẩn sương | 11 | Trinh sát → Trinh sát | hạ 12 `mist_lurker` (vùng 11–12) | 26.920 | 1.056 | |
| m14 | Sương độc | 12 | Trinh sát → Trinh sát | thu 10 "Túi sương độc" từ `mist_lurker` (50%, vùng 12–13) | 39.970 | 960 | |
| m15 | Quỷ vuốt dài | 13 | Trinh sát → Trinh sát | hạ 12 `long_claw` (vùng 13–14) | 57.410 | 1.248 | |
| m16 | Vuốt gãy | 14 | Trinh sát → Trinh sát | thu 10 "Vuốt quỷ" từ `long_claw` (50%, vùng 14–15) | 80.170 | 1.120 | |
| m17 | Kỳ tuyển chọn | 15 | Trinh sát → Giám khảo | đưa tin | 55.950 | 600 | 2 Giấy về thành |
| m18 | Quỷ Rừng | 16 | Giám khảo → Giám khảo | hạ 15 `forest_demon` (vùng 16–17) | 149.280 | 1.920 | |
| m19 | Rễ cây quỷ | 17 | Giám khảo → Giám khảo | thu 12 "Rễ cây quỷ" từ `forest_demon` (50%, vùng 17–18) | 197.480 | 1.632 | |
| m20 | Quỷ bị giam | 18 | Giám khảo → Giám khảo | hạ 15 `captive_demon` (vùng 18–19) | 255.680 | 2.160 | |
| m21 | Mảnh xiềng | 19 | Giám khảo → Giám khảo | thu 12 "Mảnh xiềng" từ `captive_demon` (50%, vùng 19–20) | 326.580 | 1.824 | |
| m22 | Vượt qua tuyển chọn | 20 | Giám khảo → Chỉ huy | đưa tin | 206.550 | 800 | **Sách 2**, vũ khí bậc 20, 10 Bình HP cấp 10, 10 Bình MP cấp 10 |
| m23 | Thức thứ ba | 30 | Chỉ huy → Chỉ huy | đưa tin (không cần chuỗi) | 1.090.150 | 1.200 | **Sách 3** |
| m24 | Thức thứ tư | 40 | Chỉ huy → Chỉ huy | đưa tin (không cần chuỗi) | 3.800.410 | 1.600 | **Sách 4** |
| m25 | Thức thứ năm | 50 | Chỉ huy → Chỉ huy | đưa tin (không cần chuỗi) | 10.358.570 | 2.000 | **Sách 5** |

- `requires` của m02–m22 là nhiệm vụ đứng trước; m01, m23, m24, m25 rỗng.
- "Vùng a–b" là vùng quái của map có loại đó và khoảng cấp đó (4a mục 2.3); script tra id vùng từ `maps/*.json`.
- EXP và Xu là giá trị script tính từ `data/exp.json` lúc viết spec; sinh lại `exp.json` thì chạy lại `quest_data.py`.
- Id NPC: `tho_san` (Kumotori), `chi_huy` (Tổng hành dinh), `truong_lang` (Sagiri 1), `trinh_sat` (Sagiri 2), `giam_khao` (Fujikasane).

### 2.3. Công thức thưởng (trong script)

Với L = `min_level`, T = EXP lên cấp kế tiếp ở cấp L (`exp.json`), n = `count`:

- `exp` = `round10(T × p / 100)`, `round10(v) = ⌊(v + 5) / 10⌋ × 10` tính bằng số nguyên: `(T × p + 500) / 1000 × 10`; p = 20 với `kill`/`collect`, 10 với `talk`.
- `xu` = 8 × L × n với `kill`/`collect`; 40 × L với `talk`.
- Script tự kiểm tra: tổng EXP thưởng của m01–m22 nằm trong 15–30% tổng EXP từ cấp 1 lên cấp 20 (hiện 22,4%) và in tỷ lệ này.

### 2.4. Sách trong `data/items.json`

`item_stats.py` đọc `data/skills.json` và thêm vào cuối `stackables`, theo thứ tự class `viem`, `thuy`, `minh`, `nham`, `phong` rồi ô 1–5, 25 mục:

```json
{"code": "book_viem_1", "name": "Sách Bất Tri Hỏa", "sell": 0}
```

Không có `use`. `gamedata.Validate` kiểm tra mỗi chiêu trong `skills.json` có đúng một sách `book_<id>`.

### 2.5. NPC nhiệm vụ trong file map

`tools/mapgen.py` thêm vai `quest` (không có `shop`, `routes`) và năm NPC của mục 1.2. NPC đặt trong trại hoặc quanh `default` của thành, cách `default` 2–4 ô, cách NPC khác ít nhất 1,5 ô (luật 4b). Ở Kumotori: `tho_san` "Thợ săn già" ở (5,5; 8,5), cách `default` 3 ô, Thủ kho 2,2 ô, Xa phu 3,2 ô. `mapdata.Parse` nhận vai `quest`.

### 2.6. Server kiểm tra khi nạp

`gamedata` nạp `quests.json` (không có trường lạ) và kiểm tra, sai thì dừng:

- `id` khác rỗng, không trùng; `line` là `main`; `name` khác rỗng; `min_level` 1–90; `text` đủ ba câu khác rỗng.
- `requires` rỗng hoặc là một nhiệm vụ `main` đứng **trước** trong mảng (không có vòng).
- `objective.type` thuộc `kill`, `collect`, `talk`; `kill`/`collect`: `monster` là loại quái có thật, `count` 1–999, `map` có thật, `region` là vùng của map đó và vùng đó là loại `monster`; `collect`: `chance_bp` 1–10.000, `item_name` khác rỗng; `talk`: không có `monster`, `count`, `map`, `region`.
- `reward`: `exp`, `xu` ≥ 0; mỗi `items[].code` là vật phẩm chồng có thật, không phải sách, `count` 1 tới `stack_max`; mỗi `equipment[]` có `tier` trong `tiers` và `slot` là ô có thật; `skill_book` 0–5.
- `Validate(maps)`: `giver.map`, `turn_in.map` có thật; `giver.npc`, `turn_in.npc` là NPC vai `quest` của map đó.

## 3. Lưu trữ

Migration `00009_quests.sql`:

- Bảng `character_quests`: `character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE`, `quest_id text NOT NULL`, `state smallint NOT NULL CHECK (state IN (1, 2))` (1 đang làm, 2 đã xong), `progress int NOT NULL DEFAULT 0 CHECK (progress >= 0)`, `day int NOT NULL DEFAULT 0` (0 với chính tuyến; 6d dùng cho hàng ngày). Khóa chính `(character_id, quest_id)`.
- `characters.skill_book_waiver smallint NOT NULL DEFAULT 0 CHECK (skill_book_waiver BETWEEN 0 AND 90)`; khi migrate: `UPDATE characters SET skill_book_waiver = level`.
- Down xóa bảng và cột.

**Nạp:** `GetCharacter` nạp mức miễn sách (trong câu chọn cột chung) và các dòng `character_quests`. **Lưu:** `CharState.Quests` thay toàn bộ dòng nhiệm vụ của nhân vật ở mọi lần lưu (xóa rồi chèn, cùng giao dịch, như chiêu); `CharState.SkillBookWaiver` được ghi.

**Khi vào game** (`world.Enter`): nhiệm vụ có id không còn trong `quests.json` bị bỏ và ghi log; `progress` kẹp vào [0, số lượng của mục tiêu]; nhiệm vụ `main` có `day` khác 0 thì đặt `day` 0.

## 4. Server

### 4.1. Trạng thái

- `PlayerInfo` và bản trao tay (4a mục 3.2.1) thêm `Quests` (map id nhiệm vụ → `{state, progress, day}`, bản sao sâu) và `SkillBookWaiver`.
- `zone.Area.Quests` là dữ liệu nhiệm vụ đã nạp; NPC vai `quest` nằm trong danh sách NPC của area như các vai khác.
- Gói `internal/quest` chứa luật thuần (không biết zone): nhận được hay không, ghi tiến độ khi hạ quái, tính thưởng, sách cần hay không.

Gọi q là nhiệm vụ, e là mục của nhân vật (có thể không có). q **nhận được** khi: không có e; `requires` rỗng hoặc mục của nó có `state` đã xong; cấp nhân vật ≥ `min_level`. q **hoàn thành mục tiêu** khi e đang làm và `progress` ≥ số lượng (`talk`: 1).

### 4.2. Nhận nhiệm vụ

`QuestAcceptReq{npc_id, quest_id}`. Kiểm tra theo thứ tự, lỗi đầu tiên được trả, mọi lỗi không đổi gì:

1. Nhân vật chết: `DEAD`.
2. Map không có NPC đó, hoặc NPC không phải vai `quest`: `BAD_REQUEST`.
3. Không gần NPC (4b mục 4.1): `NPC_TOO_FAR`.
4. Không có nhiệm vụ `quest_id`, hoặc `giver` của nó không phải (map này, NPC này): `BAD_REQUEST`.
5. Đã có mục (đang làm hay đã xong), hoặc `requires` chưa xong: `QUEST_NOT_AVAILABLE`.
6. Cấp nhân vật < `min_level`: `LEVEL_TOO_LOW`.

Hợp lệ: thêm mục `{đang làm, progress = 0}` (`talk`: `progress = 1`), lưu ngay, trả `QuestAcceptResp{quest}`.

### 4.3. Tiến độ khi hạ quái

Khi một quái chết và người kết liễu còn trong zone (cùng chỗ trao Xu và đồ rơi của 3a, người đó có thể đang chết), với mỗi mục đang làm của người đó mà `progress` < số lượng, theo thứ tự id nhiệm vụ:

- `kill` có `monster` là loại của quái: `progress` + 1.
- `collect` có `monster` là loại của quái: tung `rng < chance_bp` (trên 10.000) bằng bộ ngẫu nhiên của zone; trúng thì `progress` + 1.

Mỗi mục đổi được ghi vào danh sách đổi của người đó trong tick; `syncView` gửi `QuestProgress{quests}` sau `WorldTick` (và sau `DeathNotice` nếu có), trước `SelfStats`, rồi xóa danh sách. Không lưu ngay. EXP tổ đội và đồng đội không đổi gì ở đây.

### 4.4. Trả nhiệm vụ

`QuestTurnInReq{npc_id, quest_id}`:

1. `DEAD`.
2. NPC không có hoặc không phải vai `quest`: `BAD_REQUEST`.
3. `NPC_TOO_FAR`.
4. Không có nhiệm vụ `quest_id`: `BAD_REQUEST`.
5. Không có mục đang làm của nhiệm vụ đó: `QUEST_NOT_ACTIVE`.
6. `turn_in` của nhiệm vụ không phải (map này, NPC này): `BAD_REQUEST`.
7. Chưa hoàn thành mục tiêu: `QUEST_NOT_COMPLETE`.
8. Hành trang không chứa hết vật phẩm thưởng (vật phẩm chồng, trang bị, sách nếu được trao): `BAG_FULL`.

Hợp lệ, theo thứ tự: mục thành đã xong; thêm vật phẩm vào hành trang; Xu cộng `reward.xu`; EXP qua `gainExp` (lên cấp như khi đánh quái: `LevelUp` trong `WorldTick`, hàm `Leveled` của 5b); `statsDirty`; lưu ngay; trả `QuestTurnInResp{quest_id, exp, xu, update, stats}` với `exp`, `xu` là `reward.exp`, `reward.xu`.

**Sách được trao** khi `skill_book` = n > 0 và với chiêu c = `<class>_<n>`: nhân vật chưa học c và `learn_level(c)` > mức miễn sách.

### 4.5. Học chiêu cần sách

Thứ tự kiểm tra của `LearnSkillReq` (spec Giai đoạn 2 mục 3.1) thêm bước 6:

1. `DEAD`. 2. `BAD_REQUEST`. 3. `SKILL_MAXED`. 4. `LEVEL_TOO_LOW`. 5. `NOT_ENOUGH_POINTS`.
6. Cấp chiêu hiện tại là 0, `learn_level` > mức miễn sách, và hành trang không có `book_<skill_id>`: `NO_SKILL_BOOK`.

Hợp lệ: như cũ; khi bước 6 có áp dụng thì bớt một sách ở ô hành trang có sách nhỏ nhất. Lưu ngay (đã có). `LearnSkillResp` thêm `update` (ô hành trang đổi; rỗng khi không tiêu sách).

### 4.6. Vào game

`EnterWorldResp.quests` mang mọi mục của nhân vật; `EnterWorldResp.skill_book_waiver` mang mức miễn sách. Chuyển zone giữ nguyên qua bản trao tay.

### 4.7. Gateway

`QuestAcceptReq`, `QuestTurnInReq` cần đã vào thế giới (`NOT_IN_WORLD`), chỉ chịu giới hạn gói chung, đi vào zone như lệnh khác (hộp thư zone đầy thì `RATE_LIMITED`).

## 5. Giao thức

`ProtocolVersion` lên **14**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 119 | `QuestAcceptReq` | `string npc_id = 1`, `string quest_id = 2` |
| 120 | `QuestAcceptResp` | `QuestEntry quest = 1` |
| 121 | `QuestTurnInReq` | `string npc_id = 1`, `string quest_id = 2` |
| 122 | `QuestTurnInResp` | `string quest_id = 1`, `int64 exp = 2`, `int64 xu = 3`, `InventoryUpdate update = 4`, `SelfStats stats = 5` |
| 123 | `QuestProgress` | `repeated QuestEntry quests = 1` |

```proto
enum QuestState {
  QUEST_STATE_UNSPECIFIED = 0;
  QUEST_STATE_ACTIVE = 1; // accepted, objective counted in progress
  QUEST_STATE_DONE = 2;   // turned in
}

// One quest of the character. Quests without an entry are not taken.
message QuestEntry {
  string quest_id = 1;
  QuestState state = 2;
  int32 progress = 3;
}

// Every quest entry of the character.
message QuestLog {
  repeated QuestEntry quests = 1;
}
```

- `EnterWorldResp.quests = 13` (`QuestLog`), `EnterWorldResp.skill_book_waiver = 14` (`int32`).
- `LearnSkillResp.update = 3` (`InventoryUpdate`): ô hành trang của sách vừa tiêu; rỗng khi không tiêu sách.
- `QuestProgress` đẩy với `seq = 0`.
- `ErrorCode`: `QUEST_NOT_AVAILABLE = 53` (đã nhận, đã xong hoặc chưa xong nhiệm vụ trước), `QUEST_NOT_ACTIVE = 54`, `QUEST_NOT_COMPLETE = 55`, `NO_SKILL_BOOK = 56`.
- Tên an toàn với GDScript (không có trường `message`, `class`). Fixture mới: `quest_accept_req.hex`, `quest_turn_in_resp.hex` (có sách và vũ khí), `quest_progress.hex`, `enter_world_resp_quests.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/quest_book.gd`** (mới): nạp `data/quests.json`; `apply_log(QuestLog)`, `apply_entries(entries)`, `set_done(id)`; `entry(id)`, `available(id, level) -> bool` và `complete(id) -> bool` cùng luật mục 4.1; `npc_quests(map_id, npc_id, level) -> Array` (`{id, kind}` với `kind` `"ready"`, `"offer"`, `"active"`, sắp theo ưu tiên mục 1.2 rồi theo thứ tự trong file); `npc_icon(map_id, npc_id, level) -> String` (`"ready"`, `"offer"`, `"active"`, `""`); `tracker(level) -> Array` (tối đa 4 dòng theo mục 1.2: `{id, text, goal_kind}`); `goal_of(id, level, graph) -> Dictionary` (đích `travel` của 6a, mục 6.3); `reward_of(id, class, waiver, learned) -> Dictionary` (cùng luật trao sách mục 4.4, để hiện thưởng); `book_source(skill_id, class) -> String` (id nhiệm vụ trao sách đó).
- **`core/travel.gd`** (6a): thêm `kind` `"region"` với `rects` (các hình chữ nhật ô của vùng): chặng cuối đi tới `nest` của vùng và **tới nơi** ngay khi vị trí dự đoán nằm trong một ô của `rects`.
- **`core/map_grid.gd`**, **`core/world_graph.gd`**: đọc `regions` (`id`, `monster`, `min_level`, `max_level`, `rects`, `nest`) nếu 6a chưa đọc; `region(map_id, region_id)`.
- **`core/skill_book.gd`**: `learn_block(skill, char_level, has_book, waiver)` thêm lý do `"book"` (sau `"points"`), cùng luật mục 4.5.
- **`core/inventory.gd`**: `count_of(code) -> int` (chỉ hành trang).
- **`core/errors.gd`**: QUEST_NOT_AVAILABLE "Chưa nhận được nhiệm vụ này", QUEST_NOT_ACTIVE "Chưa nhận nhiệm vụ này", QUEST_NOT_COMPLETE "Chưa hoàn thành mục tiêu", NO_SKILL_BOOK "Cần sách kỹ năng để học lần đầu". **`core/wire.gd`**: payload mới.
- Mô hình nhiệm vụ nằm ở `Router.quests` để còn nguyên khi màn World nạp lại (như tổ đội ở 5b); xóa khi về màn chọn nhân vật.

### 6.2. Giao diện

- **Biểu tượng trên NPC** (`ui/map_view.gd`): NPC vai `quest` vẽ vòng màu tím có chữ biểu tượng "N"; phía trên tên: "!" vàng, "?" vàng hoặc "?" xám theo `npc_icon`; cập nhật khi nhật ký đổi hoặc lên cấp.
- **Cửa sổ NPC** (`ui/npc_window.gd`, 4b): vai `quest` hiện danh sách `npc_quests`, mỗi dòng biểu tượng và tên. Chạm dòng mở **thẻ nhiệm vụ** (`ui/quest_card.gd`, mới): tên, lời thoại theo trạng thái (`offer`, `progress`, `done`), dòng mục tiêu ("Hạ Quỷ lang thang 3/5", "Thu thập Nanh quỷ 2/6", "Gặp Chỉ huy Sát Quỷ Đoàn ở Tổng hành dinh Ubuyashiki"), thưởng (EXP, Xu, từng vật phẩm, sách; sách không được trao thì không hiện), nút **Nhận** (gửi `QuestAcceptReq`), **Trả nhiệm vụ** (gửi `QuestTurnInReq`; khóa và ghi "Hành trang đầy" khi `inventory` mô phỏng không chứa hết) hoặc **Đi tới mục tiêu** khi đang làm (lộ trình của mục 6.3, đóng cửa sổ).
- **Khung theo dõi** (`ui/quest_tracker.gd`, mới, trong HUD, bên phải dưới cụm nút góc phải trên): tiêu đề "Nhiệm vụ" chạm để thu gọn hay mở; mỗi dòng của `tracker`; chạm một dòng có đích thì bắt đầu lộ trình (6a), dòng khóa thì không làm gì.
- **Tab Kỹ năng** (`ui/skill_tab.gd`): lý do `"book"` ghi "Cần Sách <tên chiêu>" và dòng nhỏ "Nhận từ nhiệm vụ: <tên nhiệm vụ>" (`book_source`).
- **Thông báo** (dòng thông báo HUD): tiến độ "<mục tiêu> n/N"; lần đầu đủ số "Hoàn thành mục tiêu: <tên> — về gặp <NPC>"; nhận "Đã nhận: <tên>"; trả "+N EXP, +M Xu" và tên vật phẩm nhận được.
- **Cửa sổ Bản đồ** (6a): tên vai `quest` là "Nhiệm vụ".

### 6.3. Đích khi chạm nhiệm vụ

`goal_of` trả đích `travel`:

- Đang làm và hoàn thành mục tiêu, hoặc mục tiêu `talk`: NPC `turn_in` (`kind` `"npc"`, bán kính 2,5); tới nơi thì cửa sổ NPC mở thẳng thẻ nhiệm vụ đó.
- Đang làm `kill`/`collect`: vùng `objective.map`/`region` (`kind` `"region"`); tới nơi thì dừng và báo "Đã tới bãi <tên quái>". Không chọn mục tiêu, không bật auto.
- Nhận được: NPC `giver` (`kind` `"npc"`); tới nơi thì mở thẻ nhiệm vụ đó.

### 6.4. Smoke

`tools/smoke.sh` thêm, với tài khoản smoke: `devchar -level 10`, `devquest -done m10 -active m11`, `devchar -map ubuyashiki -x … -y …` (cạnh `chi_huy`, lấy tọa độ từ file map). `tests/smoke/smoke.gd` sau các bước cũ: vào lại; `LearnSkillReq` ô 1 bị `NO_SKILL_BOOK`; `QuestTurnInReq{chi_huy, m11}` thì `QuestTurnInResp` có sách ô 1 và vũ khí trong `update`; `LearnSkillReq` ô 1 thành công và sách biến mất khỏi hành trang; `QuestAcceptReq{chi_huy, m12}` trả `QuestAcceptResp`. In "SMOKE OK" kèm "quest m11, book used".

## 7. Lệnh dev

`cmd/devquest -u <username> [-done <id>] [-active <id> [-progress <n>]] [-reset]` (đọc `DATABASE_URL`), cho nhân vật đầu tiên của tài khoản, chỉ khi đã thoát game (in nhắc như `devchar`):

- `-reset`: xóa mọi dòng nhiệm vụ (chạy trước các cờ khác).
- `-done <id>`: đặt nhiệm vụ đó và mọi nhiệm vụ trong chuỗi `requires` phía trước nó thành đã xong.
- `-active <id>`: đặt nhiệm vụ đó đang làm với `progress` = `-progress` (mặc định 0; `talk` luôn 1), kẹp vào [0, số lượng].
- Id lạ thì báo lỗi, không ghi gì. In danh sách nhiệm vụ sau khi ghi.

`devchar` không đổi mức miễn sách, trừ cờ mới `-waive-books`: đặt `skill_book_waiver` = cấp sau khi đổi.

## 8. Kiểm thử

**Script:** `quest_data.py` tự kiểm tra như mục 2.3 và: mỗi `requires` trỏ về trước; mỗi vùng tra được; m11 và m22 có sách 1 và 2, m23–m25 có sách 3–5; `--check` so với file đã commit. `item_stats.py` kiểm tra 25 sách, giá bán 0. `mapgen.py` kiểm tra năm NPC `quest` theo luật vị trí.

**Server** (`go test -race`):

- `gamedata`: `quests.json` thật nạp được (25 nhiệm vụ); `Validate` với năm map thật không lỗi; từ chối: trường lạ, trùng id, `requires` trỏ về sau hoặc không có, `min_level` 0, loại mục tiêu lạ, `kill` thiếu `region`, vùng khác loại quái, `collect` có `chance_bp` 0, `talk` có `count`, thưởng là sách, thưởng trang bị bậc 15, `skill_book` 6, NPC không phải vai `quest`; mỗi chiêu có đúng một sách.
- `mapdata`: vai `quest` hợp lệ; `quest` có `shop` bị từ chối; năm map thật có đúng năm NPC nhiệm vụ.
- `quest`: nhận được theo `requires` và cấp; m23 nhận được ở cấp 30 khi chưa làm chuỗi; ghi tiến độ `kill` đúng loại, không vượt số lượng; `collect` với bộ ngẫu nhiên giả trúng/trượt; thưởng sách bỏ khi đã học chiêu hoặc cấp học ≤ mức miễn; `round10` với 16 → 20, 9.152 → 9.150.
- `store` (DB): migration lên rồi xuống; nhân vật cấp 25 có sẵn được mức miễn 25, nhân vật mới 0; lưu rồi nạp lại nhiệm vụ (đang làm có tiến độ, đã xong); lưu lần sau thay hết dòng cũ.
- `world`: id nhiệm vụ lạ bị bỏ khi vào game; `progress` 99 của nhiệm vụ cần 5 kẹp về 5.
- `zone` nhận/trả: từng mã lỗi theo thứ tự cho hai lệnh; nhận `talk` có `progress` 1; nhận xong xếp một lần lưu; trả m01 cộng 20 EXP, 40 Xu, 5 bình; trả khi hành trang đầy bị `BAG_FULL`, không đổi gì; trả m11 với Viêm cho `book_viem_1` và `eq10_weapon_viem` Trắng +0; với mức miễn 10 thì không có sách; trả làm lên cấp thì có `LevelUp`; nhiệm vụ đã xong không nhận lại được (`QUEST_NOT_AVAILABLE`).
- `zone` tiến độ: người kết liễu được tính, người chỉ gây sát thương thì không; đồng đội cùng zone không được tính; người kết liễu đã chết vẫn được tính; người đã rời zone thì không; `QuestProgress` tới sau `WorldTick` của tick đó và chỉ có mục đổi; hai nhiệm vụ cùng loại quái đều tăng.
- `zone` học chiêu: không có sách, mức miễn 0 → `NO_SKILL_BOOK`, không tốn điểm; có sách → học được, sách giảm một, `LearnSkillResp.update` có ô đó; nâng cấp 1 → 2 không cần sách; mức miễn 10 học chiêu cấp 10 không cần sách; thứ tự `NOT_ENOUGH_POINTS` trước `NO_SKILL_BOOK`.
- `zone` trao tay: nhiệm vụ và mức miễn đi qua cổng; `EnterWorldResp.quests` có đủ mục.
- `gateway`: `ProtocolVersion` 14; fixture mới; hai lệnh khi chưa vào thế giới bị `NOT_IN_WORLD`.
- `cmd/devquest`: `-done m05` đặt m01–m05 đã xong; `-active m07 -progress 3`; `-reset`; id lạ báo lỗi.
- e2e `TestQuestsOverWebSocket`: nhân vật Viêm cấp 1 mới ở `default` Kumotori (trong tầm `tho_san`); nhận m02 bị `QUEST_NOT_AVAILABLE`; nhận m01; hạ một `wanderer` và nhận `QuestProgress` 1/5; trả bị `QUEST_NOT_COMPLETE`; thoát, đặt tiến độ 5 bằng helper store, vào lại, trả được (EXP +20, Xu +40, 5 bình); nhận m02 được. Nhân vật thứ hai (store đặt cấp 10, mức miễn 0, m01–m10 đã xong, m11 đang làm, `DevSetPlace` cạnh `chi_huy`): học `viem_1` bị `NO_SKILL_BOOK`; trả m11; học được và sách đã tiêu; thoát rồi vào lại, m11 đã xong, `viem_1` cấp 1.

**Client** (`tests/run.sh`): `quest_book` (nạp file thật, `available` theo chuỗi và cấp, `npc_icon` với ba trạng thái và thứ tự ưu tiên, `tracker` trước và sau khi trả m10, `goal_of` ba loại, `reward_of` bỏ sách đúng luật, `book_source`); `travel` với `kind` `"region"` tới nơi khi bước vào vùng; `skill_book.learn_block` lý do `"book"`; `inventory.count_of`; giải mã fixture mới. Smoke theo mục 6.4.

**Nghiệm thu:** server ở `:8199`. Một nhân vật mới của `test1` làm m01–m05 ở Kumotori (chạm khung theo dõi để tới bãi), chạm m05 thì tự đi tới Tổng hành dinh gặp Chỉ huy; `devchar -level 10` và `devquest -done m10 -active m11` cho nhân vật thứ hai, trả m11 lấy sách rồi học Bất Tri Hỏa; `test1` (Kaito, cấp 90, mức miễn 90) vẫn nâng và dùng mọi chiêu như cũ.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 2.6 (sách 1, 2 cuối chặng cấp 10, 20 của chính tuyến; sách 3–5 từ nhiệm vụ mốc; mức miễn sách cho nhân vật có trước), mục 4.1 (năm NPC nhiệm vụ; thứ tự biểu tượng), mục 4.2 (chuỗi chính tuyến v1, ba loại mục tiêu, vật phẩm nhiệm vụ là bộ đếm, người kết liễu được tính, thưởng theo % một cấp), mục 10.7 bỏ.
- Spec Giai đoạn 2 mục 1.3: ghi chú "sách kỹ năng đã có từ spec 6b".
- `CLAUDE.md`: `data/quests.json` sinh bằng `docs/balance/quest_data.py` (sửa script, không sửa JSON); tiến độ nhiệm vụ chỉ tính cho người kết liễu, không bao giờ chia qua tổ đội; học chiêu lần đầu cần sách trừ khi cấp học ≤ `skill_book_waiver`.
- README hai repo: NPC nhiệm vụ, khung theo dõi, sách kỹ năng, `devquest`, `devchar -waive-books`.
