# Giai đoạn 4e — Tinh anh và thủ lĩnh cấp 25–90

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ hai của Thế giới mở rộng (xem [spec 4d](2026-11-20-giai-doan-4d-the-gioi-mo-rong-design.md) mục 1.1). Spec này đặt 17 tinh anh/thủ lĩnh cấp 25–90 của [game-design.md](../../game-design.md) mục 5.5 và 5.10 lên các map của 4d, bằng khung của [spec 7d](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) (hạng, kỹ năng có cảnh báo vùng, nội tại, khu thủ lĩnh trong file map, hồi sinh 5/10 phút). Enmu dùng loại `enmu` và hiệu ứng ngủ của [spec 7g](2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md). Game-design là chuẩn; những gì không nhắc tới ở đây giữ nguyên như 7d.

4e chạy **sau 4d, 7d và 7g**; trước hay sau 7h đều được. 4e chỉ thêm dữ liệu: **không đổi giao thức, không có migration**.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Mỗi map 21–90 có các tinh anh và thủ lĩnh mà game-design 5.5 liệt kê cho vùng đó, mỗi con đứng ở khu riêng trong một vùng quái và hồi sinh sau 5 hoặc 10 phút ở từng khu. Chúng dùng kỹ năng đặc trưng của game-design 5.10 với cảnh báo vùng né được. Quỷ Rắn ở Hầm Ngầm săn tự do khắp bãi.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Nội dung | 16 loại mới (bảng mục 2.1) cộng `enmu` của 7g, đặt trên 12 trong 14 map của 4d; `mugen_1` và `yoshiwara_1` không có | Đúng danh sách 5.5 và 5.10; hai vùng 10 cấp đã chia đôi ở 4d, con của vùng đặt ở nửa có cấp của nó |
| Số con mỗi map | Đúng các con 5.5 liệt kê cho vùng, **tối đa một con mỗi vùng quái**, mỗi con trong vùng có cấp gần cấp của nó nhất. Luật "đúng một con mỗi map" của 7d chỉ còn áp cho bốn map 1–20 | 7d chọn một con mỗi map vì Kumotori là map tân thủ; game-design 5.5 liệt kê hai hoặc ba con cho Asakusa – Khu chiến đấu, Dinh thự – Hành lang, Natagumo – Rừng tơ và Vùng lõi. Game-design 5.11: "1 con/vùng" |
| Hạng, chỉ số, thưởng | Theo bảng game-design 5.10 và hệ số của 7d (`ForRank`), bảng rơi `elite`/`boss` của 3a | Không có luật mới |
| Kỹ năng | Dùng năm hình của 7d. Kích thước vùng game-design không nêu; chọn ở mục 2.1, theo cỡ chiêu người chơi (quạt 90–120°, dải rộng 1–1,5, vòng bán kính 2–3) | Người chơi đọc được vùng; một bộ số kiểm thử được |
| Hiệu ứng chưa có khung | **Giữ sát thương, bỏ hiệu ứng** cho tới sau v1: đẩy lùi (Yahaba), chậm (Quỷ Nhện Anh, Quỷ Rắn), giữ chân (Quỷ Nhện Mẹ) | Tổng quan Giai đoạn 7 mục 4 đã để sau v1 các hiệu ứng này. Chậm và giữ chân đổi tốc độ người chơi, kéo theo dự đoán di chuyển của client; đẩy lùi là dời vị trí cưỡng bức. Mỗi cái là một tính năng riêng |
| Ngủ của Enmu | Dùng nguyên kỹ năng Sóng mộng và trạng thái ngủ của 7g; Enmu thường không nhân HP | Game-design 5.13: bản thường hồi sinh 10 phút, bản sự kiện HP × 4 |
| Quỷ Đầm Lầy | **Một thân**, không có ba phân thân chung thanh HP; giữ vùng đầm 150% công phép | Phân thân chung máu cần thực thể liên kết, đã để sau v1 (tổng quan Giai đoạn 7 mục 4); một thân tự đúng luật "trao EXP và đồ một lần" |
| Nội tại | Quỷ Asakusa: `enrage` dưới 30% HP, công +20%. Quỷ Nhện Bố "Lột xác": `enrage` dưới 50% HP, công +20%, không hồi HP | Đúng khuôn `enrage` của 7d; chỉ dữ liệu |
| Quỷ Rắn "săn tự do" | Khu thủ lĩnh của Quỷ Rắn phủ cả năm vùng của Hầm Ngầm (một hình chữ nhật, ngoài trại và dải 1 ô quanh trại), điểm xuất hiện ở giữa | Game-design 5.5, 5.11: "săn tự do trong cùng vùng, không chặn đường qua map"; Hầm Ngầm không có cổng đông |
| Quái thường dưới khu thủ lĩnh | Giữ nguyên vùng quái thường; khu thủ lĩnh chồng lên như 7d | Luật 7d |
| Giao thức, migration | Không | 7d đã có `MonsterRank`, `CastArea`; 7g đã có trạng thái ngủ |

### 1.3. Bổ sung cho game-design

Game-design 5.10 chưa nêu kích thước vùng của từng kỹ năng và chưa nói hiệu ứng nào làm ở v1. Spec này chốt kích thước ở mục 2.1; đẩy lùi, chậm, giữ chân và phân thân để sau v1 (sát thương giữ nguyên). Game-design 5.11 "1 con/vùng" giữ nghĩa "tối đa một con mỗi vùng"; map có nhiều con khi 5.5 liệt kê nhiều con. Ghi vào game-design ở mục 6.

### 1.4. Không làm ở 4e

Hiệu ứng đẩy lùi, chậm, giữ chân; phân thân chung máu; Boss chặn đường; thông báo toàn server khi Boss xuất hiện hay bị hạ; tinh anh cho `mugen_1`, `yoshiwara_1` (5.5 không có).

## 2. Dữ liệu

### 2.1. Mười bảy con

`monster_stats.py` thêm 16 loại (sửa script, không sửa JSON); thông số hành vi như bốn con của 7d (`attack_sec` 1,2, `respawn_sec` 300 hay 600, `detect_range` 5, …). Kỹ năng vùng có `windup_ms` 800 (luật 7d); kỹ năng `single` có `windup_ms` 300.

| id | Tên | Hạng | Cấp | Map, vùng | Kỹ năng / nội tại |
| --- | --- | --- | --- | --- | --- |
| `hand_demon` | Quỷ Tay | `boss` | 25 | `fujikasane_2` `r4` | `m_hand_slam` "Đập tay": `circle_self`, `phys`, 180%, hồi 8 giây, bán kính 3, tối đa 5 |
| `swamp_demon` | Quỷ Đầm Lầy | `boss` | 30 | `tay_bac_1` `r4` | `m_swamp_pool` "Vùng đầm": `circle_target`, `magic`, 150%, hồi 8 giây, tầm 6, bán kính 2,5, tối đa 5 |
| `asakusa_demon` | Quỷ Asakusa | `boss` | 35 | `asakusa_1` `r4` | `enrage`: dưới 30% HP, công +20% |
| `yahaba` | Yahaba | `boss` | 39 | `asakusa_2` `r3` | `m_arrow` "Mũi tên": `single`, `magic`, 150%, hồi 8 giây, tầm 6, tốc độ đạn 15 (không đẩy lùi) |
| `susamaru` | Susamaru | `boss` | 40 | `asakusa_2` `r4` | `m_temari` "Bóng": `line`, `phys`, 180%, hồi 6 giây, dài 6, rộng 1,5, tối đa 5 |
| `horn_demon` | Quỷ Sừng | `elite` | 45 | `tsuzumi_1` `r4` | `m_horn_charge` "Húc": `single`, `dash`, `phys`, 150%, hồi 6 giây, tầm 5, tốc độ lao 25 |
| `tongue_demon` | Quỷ Lưỡi | `elite` | 48 | `tsuzumi_2` `r3` | `m_tongue_lash` "Quất lưỡi": `line`, `phys`, 150%, hồi 6 giây, dài 4, rộng 1, tối đa 3 |
| `kyogai` | Kyogai | `boss` | 50 | `tsuzumi_2` `r4` | `m_drum_wave` "Sóng chém": `cone`, `magic`, 180%, hồi 8 giây, vươn 4, góc 120°, tối đa 5 |
| `headless_demon` | Quỷ Không Đầu | `elite` | 55 | `natagumo_1` `r4` | `m_sweep` "Quét quanh người": `circle_self`, `phys`, 150%, hồi 6 giây, bán kính 2,5, tối đa 3 |
| `spider_brother` | Quỷ Nhện Anh | `boss` | 58 | `natagumo_2` `r3` | `m_venom_spray` "Phun độc": `cone`, `magic`, 150%, hồi 8 giây, vươn 4, góc 90°, tối đa 5 (không chậm) |
| `spider_sister` | Quỷ Nhện Chị/Em Gái | `boss` | 60 | `natagumo_2` `r4` | `m_acid_cocoon` "Kén axit": `circle_target`, `magic`, 180%, hồi 8 giây, tầm 6, bán kính 2, tối đa 5 |
| `spider_mother` | Quỷ Nhện Mẹ | `boss` | 62 | `natagumo_3` `r2` | `m_bind_thread` "Tơ trói": `single`, `magic`, 120%, hồi 10 giây, tầm 6, tốc độ đạn 15 (không giữ chân) |
| `spider_father` | Quỷ Nhện Bố | `boss` | 64 | `natagumo_3` `r3` | `enrage` "Lột xác": dưới 50% HP, công +20% |
| `rui` | Rui | `boss` | 65 | `natagumo_3` `r4` | `m_thread_net` "Lưới tơ": `circle_target`, `magic`, 200%, hồi 10 giây, tầm 6, bán kính 3, tối đa 5 |
| `enmu` (7g) | Enmu | `boss` | 75 | `mugen_2` `r4` | Sóng mộng của 7g (quạt 90°, vươn 5, 150% công phép, ngủ 1 giây, hồi 10 giây, tối đa 5) |
| `sash_elite` | Dải lụa tinh anh | `elite` | 85 | `yoshiwara_2` `r4` | `m_sash_lash` "Quất lụa": `line`, `magic`, 150%, hồi 6 giây, dài 5, rộng 1,5, tối đa 3 |
| `serpent_demon` | Quỷ Rắn | `boss` | 90 | `yoshiwara_3`, khu phủ năm vùng | `m_serpent_venom` "Phun độc": `cone`, `magic`, 180%, hồi 8 giây, vươn 4, góc 90°, tối đa 5 (không chậm) |

Vùng được chọn theo khoảng cấp của 4d: vùng `r3` của map 5 cấp `a … a + 4` là a+2 – a+3, `r4` là a+3 – a+4, `r2` là a+1 – a+2. Ví dụ Yahaba 39 ở `r3` (38–39), Susamaru 40 ở `r4` (39–40).

### 2.2. Khu thủ lĩnh trong file map

`tools/mapgen.py` ghi `bosses` của mỗi map theo bảng mục 2.1 (id mục là id loại bỏ hậu tố, ví dụ `hand`, `kyogai`):

- **Luật bố cục** (thay luật "đúng một mục" của 7d cho map 21–90): mỗi mục một khu 12 × 10 nằm trọn trong vùng ghi ở bảng, điểm xuất hiện ở tâm khu trên ô đi được; khu cách mọi cổng ít nhất 2 ô, không chạm khu an toàn hay dải 1 ô quanh nó; hai khu không chung ô; mỗi vùng tối đa một khu. Bốn map 1–20 giữ đúng một mục như 7d.
- **Quỷ Rắn:** một hình chữ nhật phủ phần phía đông trại của `yoshiwara_3` (cột 12 tới 62, hàng 1 tới 46), ô của khu theo luật 7d (bỏ khu an toàn và dải 1 ô); điểm xuất hiện ở ô đi được gần tâm khu nhất.
- `mapdata` không đổi: 7d đã kiểm tra khu trong map, không chạm khu an toàn, cách cổng, hai khu không chung ô, điểm xuất hiện tới được.

## 3. Server

Không có code mới. 7d đã sinh, hồi sinh, chạy AI, kỹ năng và nội tại theo dữ liệu; 7g đã có trạng thái ngủ. Cần kiểm chứng (mục 5) rằng:

- trạng thái ngủ của 7g áp ở zone map luyện cấp như ở zone phó bản (không bị gắn với loại map `instance`);
- hai, ba khu thủ lĩnh trong một map chạy độc lập (mỗi con một vùng nội bộ, một bộ đếm).

Nếu một trong hai không đúng thì sửa trong zone, kèm test hỏng trên code cũ.

## 4. Giao thức và client

Không đổi giao thức. Client: đồng bộ `maps/` và `data/monsters.json` (7d đã đưa `monsters.json` vào `tools/sync_server.sh`); tên kỹ năng mới hiện qua `monster_book.skill_name`. Không có màn hay lệnh mới.

## 5. Kiểm thử

**Script:** `monster_stats.py --check`, `mapgen.py --check` sạch; script kiểm tra mỗi loại ở mục 2.1 có đúng hạng và cấp, mỗi khu nằm trong vùng ghi ở bảng, mỗi vùng tối đa một khu, `mugen_1`, `yoshiwara_1` không có khu.

**Server** (`go test -race`):

- `gamedata`: dữ liệu thật nạp được; 16 loại mới có hạng đúng, kỹ năng hợp lệ theo luật 7d (vùng có `windup_ms` 800, `max_targets` ≤ 3 với tinh anh, ≤ 5 với thủ lĩnh); `Validate` với map thật: mọi khu trỏ tới loại `elite`/`boss`.
- `mapdata` (map thật): số khu mỗi map đúng bảng mục 2.1 (`natagumo_3` ba khu, `asakusa_2`, `tsuzumi_2`, `natagumo_2` hai khu, `mugen_1`, `yoshiwara_1` không có, còn lại một); khu Quỷ Rắn phủ cả năm vùng của `yoshiwara_3`.
- `zone` (tick giả, map nhỏ dựng trong test với dữ liệu thật của loại):
  - Hai khu thủ lĩnh trong một map: hạ con thứ nhất thì con thứ hai không bị ảnh hưởng; con thứ nhất hồi sinh sau đúng 600 giây.
  - Quỷ Tay: người đứng trong vòng bán kính 3 lúc trúng nhận 180% công, người ra ngoài trước tick trúng không nhận gì.
  - Quỷ Nhện Bố: xuống dưới 50% HP thì đòn sau mạnh hơn 20%; không hồi HP.
  - Enmu ở zone map luyện cấp: người dính Sóng mộng ngủ 1 giây (`ASLEEP` khi di chuyển hay chém trong lúc ngủ).
  - Yahaba, Quỷ Nhện Mẹ: đòn `single` có đạn, không đổi vị trí hay tốc độ người chơi.

**Nghiệm thu:** server ở `:8199`. `test1` (`devchar -level 65`) tới `natagumo_3`, thấy ba thủ lĩnh ở ba vùng; né Lưới tơ của Rui; hạ Quỷ Nhện Bố và thấy nó mạnh lên dưới 50%. `devchar -level 75` tới `mugen_2`, bị Enmu ru ngủ 1 giây. `devchar -level 90` ở Hầm Ngầm thấy Quỷ Rắn đi khắp bãi.

## 6. Tài liệu cập nhật

- `docs/game-design.md` mục 5.10: kích thước vùng từng kỹ năng (mục 2.1); ghi chú v1 bỏ hiệu ứng đẩy lùi, chậm, giữ chân, phân thân (sát thương giữ); Quỷ Đầm Lầy một thân. Mục 5.11: "1 con/vùng" nghĩa là tối đa một con mỗi vùng; map có nhiều con khi 5.5 liệt kê nhiều con; khu Quỷ Rắn phủ cả Hầm Ngầm. Mục 11 (Để sau v1): bốn hiệu ứng trên.
- Tổng quan Giai đoạn 7 mục 4: dòng "Tinh anh/thủ lĩnh cấp 25–90 …" chỉ còn các hiệu ứng (tinh anh làm ở 4e); đã sửa cùng commit với spec này.
- README hai repo: tinh anh và thủ lĩnh trên mọi map.
