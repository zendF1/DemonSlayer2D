# Giai đoạn 4d — Thế giới mở rộng: map và quái thường cấp 21–90

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Spec này làm nốt phần Thế giới (Giai đoạn 4) mà [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) mục 1.3 hẹn "làm sau bằng cùng script": các map luyện cấp và quái thường cấp 21–90 của [game-design.md](../../game-design.md) mục 5.5. [Tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md) mục 3.6 đề xuất phần này vì không có nó thì người chơi thật không tới được Thảo Phạt Thượng Huyền (7f, cấp 50), Chuyến Tàu Vô Tận (7g, cấp 66) và Vô Hạn Thành (7h, cấp 50). Luật chơi lấy từ game-design mục 2.2 (EXP, giờ luyện cấp), 5.1–5.6 (loại map, sinh quái, dịch vụ, phân bố cấp), 5.9 (chỉ số quái), 5.11 (mật độ, quái Hầm Ngầm), 5.12 (thưởng) và 5.13 (Chuyến Tàu thường); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 4a (map, cổng, khu, `tools/mapgen.py`), [4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) (Dược sư, Xa phu), [4c](2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md) (Thủ kho), [6a](2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md) (đi xuyên map), [6b](2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md) (chính tuyến) và [6d](2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md) (hàng ngày).

4d chạy **sau 6d và trước 7f**. 7a–7e làm trước hay sau 4d đều được, vì 4d chỉ thêm dữ liệu và không dùng gì của chúng. Spec viết trên giao thức 15, migration 00009 (sau 6d). 4d **không đổi giao thức và không có migration**, nên nếu 7a–7e đã chạy trước thì số hiệu của chúng giữ nguyên. Mục 9 ghi những chỗ phải để ý khi 7d hoặc 7e đã có.

## 1. Mục tiêu và phạm vi

### 1.1. Chia phần

| Phần | Nội dung | Kết quả chơi được | Cần trước |
| --- | --- | --- | --- |
| **4d** (spec này) | 14 map luyện cấp 21–90 nối bằng cổng theo thứ tự cấp, 26 loại quái thường, trại có Dược sư, Xa phu, Thủ kho, ba đặc điểm quái Hầm Ngầm | Đi bộ hoặc đi xe từ Fujikasane tới Yoshiwara – Hầm Ngầm, luyện cấp tới 90 | 4a–4c, 6a, 6b, 6d |
| **4e** ([spec](2026-11-20-giai-doan-4e-tinh-anh-thu-linh-21-90-design.md)) | 17 tinh anh/thủ lĩnh cấp 25–90 của game-design 5.10 trên các map của 4d, theo khung của 7d | Săn Quỷ Tay, Kyogai, Rui, Enmu, Quỷ Rắn… | 4d, 7d, 7g |

4e tách riêng vì tinh anh/thủ lĩnh cần khung kỹ năng quái của 7d, và Enmu cần loại `enmu` cùng hiệu ứng ngủ của 7g. Nếu gộp vào 4d thì 4d phải chờ tới sau 7g, mà 7f đã cần người chơi lên được cấp 50.

### 1.2. Mục tiêu 4d

Từ Fujikasane – Rừng tuyển chọn (cấp 16–20), người chơi đi qua cổng phía đông vào Fujikasane – Vùng sâu (21–25), rồi qua từng cổng tới Thị trấn Tây Bắc, Asakusa, Dinh thự Trống, Natagumo, Chuyến Tàu Vô Tận và Yoshiwara, tới Hầm Ngầm cấp 86–90. Mỗi map có một trại an toàn sát tường tây, gồm điểm hồi sinh, lửa trại, Dược sư, Xa phu và Thủ kho. Xa phu ở mọi trại và ở Tổng hành dinh đưa thẳng tới trại của mọi map. Quái mới mang tên theo game-design 5.5. Ở Hầm Ngầm, mỗi loại quái có đặc điểm riêng theo game-design 5.11. Server không có lệnh mới, client chỉ đồng bộ dữ liệu.

### 1.3. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chia phần | 4d gồm map và quái thường; 4e gồm tinh anh và thủ lĩnh (mục 1.1) | 4d không phụ thuộc 7d nên chạy được ngay sau 6d. Tinh anh cần 7d, Enmu cần 7g |
| Thứ tự | 4d sau 6d, trước 7f; 7a–7e làm trước hay sau 4d đều được. 4e sau 7g | 7f, 7g, 7h chỉ cần người chơi lên được cấp, không cần tinh anh trên map thường |
| Số map | **14 map luyện cấp, mỗi map 5 cấp**: mỗi vùng 5 cấp của game-design 5.5 là một map, hai vùng 10 cấp (Chuyến Tàu 66–75; Yoshiwara – Ngõ đêm, đường ngầm 76–85) mỗi vùng chia hai map | Game-design 5.5 cho phép "mỗi địa danh lớn chia thành nhiều map nhỏ". Game-design 5.13 nói Chuyến Tàu thường gồm "các toa nối bằng cổng cố định". Tên vùng Yoshiwara đã gồm hai nơi. Mọi map cùng khổ 5 cấp nên dùng được nguyên luật bố cục và chia vùng của 4a |
| Cỡ map | Mọi map mới là **map luyện cấp nhỏ** của game-design 5.11: 64 × 48, 4 vùng quái, 30 quái thường. Riêng Hầm Ngầm có 5 vùng × 6 con, mỗi loại quái một vùng | Một hàm bố cục cho mọi map. Game-design 5.11 liệt kê 5 loại quái Hầm Ngầm, mỗi loại một cấp |
| Map luyện cấp lớn (6 vùng × 15) | Không dùng ở v1 | Map 5 cấp đã đủ chỗ; người chơi cấp cao ít, chia thêm vùng chỉ làm map trống |
| Đồ thị cổng | Chuỗi thẳng theo cấp, nối tiếp `fujikasane_1` (mục 2.3) | Đi theo cấp là đi theo độ khó; 6a tìm chuỗi cổng ít nhất trên đồ thị này |
| Thành và điểm dịch vụ mới | Không có. Mỗi map có trại an toàn 10 × 10 (luật 4a) đóng vai Nhà Hoa Tử Đằng, với Dược sư, Xa phu, Thủ kho. Không có Thợ rèn ngoài Tổng hành dinh | Game-design 5.4: Nhà Hoa Tử Đằng có điểm hồi sinh, bình, kho, xa phu. Giấy về thành (50 Xu) và Xa phu đưa tới Thợ rèn nhanh. Điệp Phủ, Làng Thợ Rèn để sau như 4b |
| Trạm giữa đường | Không có | Map 64 ô ngang; bãi xa nhất cách trại khoảng 50 ô, đi bộ khoảng 12 giây |
| Tuyến Xa phu | Xa phu ở mọi trại có tuyến tới Tổng hành dinh và **mọi** map luyện cấp khác; Xa phu ở Tổng hành dinh có tuyến tới mọi map luyện cấp. Giá theo công thức 4b (mục 2.4) | Luật 4b giữ nguyên, chỉ thêm dữ liệu. Đi bộ từ thành tới Hầm Ngầm qua 18 cổng mất khoảng 5 phút |
| `min_zones` | 2, như mọi map luyện cấp của 4a | Một luật; luôn có khu để đổi. 18 map × 2 khu và thành là 37 zone lúc khởi động; quái của zone không có người tốn ít, sẽ đo khi nghiệm thu (mục 7) |
| Cấp vào map (`min_level` của 7f) | Không đặt cho map luyện cấp | Game-design 4.2: "Cấp quái thể hiện độ khó và phân bố bãi, không phải điều kiện vượt map" |
| Quái thường | 26 loại mới (mục 2.5), tên theo game-design 5.5, hành vi như `wanderer` trừ các đặc điểm ở mục 2.6. Vùng chỉ có một tên quái trong 5.5 thì dùng một loại cho cả bốn vùng | Mọi loại dùng chung bảng `levels`, nên loại chỉ khác tên và hành vi. Không bịa tên ngoài game-design |
| Tên dễ nhầm | "Quỷ rừng" ở Dinh thự Trống đặt là **Quỷ rừng ngoài dinh** (Fujikasane đã có "Quỷ Rừng"); "Con rối tinh anh" là quái thường nên đặt là **Con rối tinh nhuệ** (tránh nhầm với nhãn hạng "Tinh anh" của 7d); "quỷ nhiều tay cấp thấp" đặt là **Quỷ nhiều tay** ("cấp thấp" chỉ là mô tả) | Tên quái hiện trên đầu và trong nhiệm vụ; hai tên gần trùng hay một chữ trùng nhãn hạng làm người chơi hiểu sai |
| Quái Chuyến Tàu | Ba loại `train_tentacle` Xúc tu, `train_flesh` Mô thịt (`speed` 0,6), `train_nightmare` Quái ác mộng, đúng id và thông số của [spec 7g](2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md) mục 2.3. 7g chỉ còn thêm `enmu` | 7g đã định các loại này "dùng chung với map Chuyến Tàu thường"; 4d chạy trước 7g nên tạo trước |
| Đặc điểm Hầm Ngầm | Ba trường mới, không bắt buộc, của loại quái: `hp_pct`, `attack_pct` (mặc định 100), `damage_type` (`phys` mặc định, hoặc `magic`). Quỷ Huyết Nha: HP 80, công 120. Quỷ Lân Xà: `damage_type: magic`, `detect_range` 4. Quỷ Tàng Ảnh: `speed` 0,96, `chase_speed` 1,44 (+20%). Quỷ Trảo Dài: tầm đánh 2 ô, bằng chuẩn | Game-design 5.11 đã chốt các đặc điểm này. Tốc độ và tầm chỉ cần dữ liệu đã có; HP, công, loại sát thương cần ba trường nhỏ |
| EXP của quái có `hp_pct` | EXP gốc nhân cùng `hp_pct` với HP | Game-design 5.8: EXP gốc = HP × r(L), nên EXP trên mỗi điểm sát thương giữ đúng r(L) |
| Chính tuyến 21–50 | **Không thêm**; giữ ba nhiệm vụ mốc m23, m24, m25 (cấp 30, 40, 50) ở Chỉ huy. Không có NPC nhiệm vụ ở map mới | Sách 3–5 đã có nguồn. Cửa sổ Bản đồ của 6a ghi khoảng cấp từng map, đủ để người chơi biết đi đâu. Viết thêm chuỗi là nội dung lớn, để sau v1 |
| Hàng ngày (6d) | Không đổi | `kill_any`, `collect_any` tính quái hợp cấp ở mọi map; đích "vùng hợp cấp gần nhất" tự tìm thấy map mới |
| Tinh anh, thủ lĩnh | Không có ở 4d; làm ở 4e. Map mới để `bosses` rỗng nếu 7d đã có (mục 9) | Mục 1.1 |
| Giao thức | **Không đổi** `ProtocolVersion` | Không có message mới. Số hiệu 16–23 đã cấp cho 7a–7h trong khi vị trí của 4d giữa 6d và 7f không cố định. Server và client đổi cùng nhau qua submodule và `tools/sync_server.sh` |
| Migration | Không | Không có bảng hay cột mới. Vị trí đã lưu ở map cũ vẫn đúng |
| `fujikasane_1` | Thêm cổng đông `to_fujikasane_2`, điểm đến `from_fujikasane_2` và khu an toàn `gate_east`; tường, trại, vùng và NPC giữ nguyên | Như cách 4a sửa `kumotori_1`; khu an toàn mới không đổi ô đi được nên vị trí đã lưu vẫn hợp lệ |
| Hình ảnh | Vẫn vẽ lưới ô như 4a; tường trang trí do người viết script chọn theo chủ đề (dãy nhà ở Asakusa, vách toa ở Chuyến Tàu…) | Chưa có hình map thật |

### 1.4. Bổ sung cho game-design

- Game-design 5.5 chưa có id map và cách chia hai vùng 10 cấp; 5.11 chưa nói Hầm Ngầm có mấy vùng. Spec này chốt ở mục 2 và ghi vào game-design (mục 8).
- Ba tên ở mục 1.3 ("Quỷ rừng ngoài dinh", "Con rối tinh nhuệ", "Quỷ nhiều tay") được sửa trong bảng 5.5.
- Quỷ Trảo Dài "tầm 2 ô" bằng tầm cận chiến chuẩn (spec quái thường); ghi rõ ở 5.11 rằng nó không khác quái chuẩn về tầm.

### 1.5. Không làm ở 4d

Tinh anh, thủ lĩnh (4e); chính tuyến và phụ tuyến cấp 21+; NPC nhiệm vụ ở map mới; thành mới, Điệp Phủ, Làng Thợ Rèn, Thợ rèn ngoài Tổng hành dinh; map luyện cấp lớn; cấp vào map luyện cấp; trang bị và kỹ năng mới cho cấp 60–90 (đã chấp nhận để sau); hình ảnh map thật; bản đồ thế giới vẽ hình.

## 2. Dữ liệu

Mọi file dữ liệu sinh bằng script, không sửa tay: `maps/*.json` bằng `tools/mapgen.py --write`, `data/monsters.json` bằng `docs/balance/monster_stats.py --write-json`.

### 2.1. Mười bốn map mới

| id | Tên | Cấp quái | Vùng 5.5 |
| --- | --- | --- | --- |
| `fujikasane_2` | Fujikasane – Vùng sâu | 21–25 | Fujikasane – Vùng sâu |
| `tay_bac_1` | Thị trấn Tây Bắc – Ngoại ô, đầm lầy | 26–30 | Thị trấn Tây Bắc – Ngoại ô, đầm lầy |
| `asakusa_1` | Asakusa – Ngõ tối, khu ngoại vi | 31–35 | Asakusa – Ngõ tối, khu ngoại vi |
| `asakusa_2` | Asakusa – Khu chiến đấu | 36–40 | Asakusa – Khu chiến đấu |
| `tsuzumi_1` | Dinh thự Trống – Rừng ngoài dinh | 41–45 | Dinh thự Trống – Rừng ngoài dinh |
| `tsuzumi_2` | Dinh thự Trống – Hành lang, phòng trống | 46–50 | Dinh thự Trống – Hành lang, phòng trống |
| `natagumo_1` | Natagumo – Bìa rừng | 51–55 | Natagumo – Bìa rừng |
| `natagumo_2` | Natagumo – Rừng tơ nhện | 56–60 | Natagumo – Rừng tơ nhện |
| `natagumo_3` | Natagumo – Vùng lõi | 61–65 | Natagumo – Vùng lõi |
| `mugen_1` | Chuyến Tàu Vô Tận – Các toa đầu | 66–70 | Chuyến Tàu Vô Tận (nửa đầu) |
| `mugen_2` | Chuyến Tàu Vô Tận – Các toa cuối | 71–75 | Chuyến Tàu Vô Tận (nửa sau) |
| `yoshiwara_1` | Yoshiwara – Ngõ đêm | 76–80 | Yoshiwara – Ngõ đêm, đường ngầm (nửa đầu) |
| `yoshiwara_2` | Yoshiwara – Đường ngầm | 81–85 | Yoshiwara – Ngõ đêm, đường ngầm (nửa sau) |
| `yoshiwara_3` | Yoshiwara – Hầm Ngầm | 86–90 | Yoshiwara – Hầm Ngầm |

Mọi map: `type: training`, 64 × 48, `min_zones: 2`, `town: "ubuyashiki"`, không có `min_level`. Id map Chuyến Tàu là `mugen_*` để khác map phó bản `tau_vo_tan` của 7g. Ước tính chỉ đánh quái một mình (game-design 2.2): từ cấp 21 lên 51 khoảng 430 giờ, từ 51 lên 90 khoảng 2.600 giờ; nhiệm vụ, tổ đội và sự kiện rút ngắn.

### 2.2. Vùng quái

Map 5 cấp `a … a + 4` có bốn vùng như 4a: `r1` cấp a – a+1 × 8 con, `r2` a+1 – a+2 × 7, `r3` a+2 – a+3 × 7, `r4` a+3 – a+4 × 8 (tổng 30). Ví dụ `fujikasane_2`: 21–22, 22–23, 23–24, 24–25.

| Map | `r1`, `r2` | `r3`, `r4` |
| --- | --- | --- |
| `fujikasane_2` | `big_demon` | `many_arms` |
| `tay_bac_1` | `marsh_demon` | `mud_ambusher` |
| `asakusa_1` | `new_demon` | `city_wanderer` |
| `asakusa_2` | `henchman` | `henchman` |
| `tsuzumi_1` | `manor_woods` | `long_nail` |
| `tsuzumi_2` | `manor_dweller` | `manor_dweller` |
| `natagumo_1` | `small_spider` | `thread_puppet` |
| `natagumo_2` | `spider_demon` | `silk_cocoon` |
| `natagumo_3` | `silk_fiend` | `veteran_puppet` |
| `mugen_1` | `train_tentacle` | `train_flesh` |
| `mugen_2` | `train_flesh` | `train_nightmare` |
| `yoshiwara_1` | `night_lurker` | `sash_fragment` |
| `yoshiwara_2` | `night_lurker` | `sash_fragment` |

`yoshiwara_3` có năm vùng, mỗi vùng một loại và một cấp, 6 con mỗi vùng (tổng 30):

| Vùng | Loại | Cấp |
| --- | --- | --- |
| `r1` | `mask_demon` | 86 |
| `r2` | `long_talon` | 87 |
| `r3` | `blood_fang` | 88 |
| `r4` | `shadow_demon` | 89 |
| `r5` | `scale_serpent` | 90 |

### 2.3. Cổng

Chuỗi hai chiều, theo luật tên và vị trí của 4a (cổng `to_<map đích>` 1 × 4 ô ở cột 1 phía tây hoặc cột 62 phía đông, giữa chiều cao; điểm đến `from_<map nguồn>` cách cột cổng 3 ô vào trong):

`fujikasane_1` ↔ `fujikasane_2` ↔ `tay_bac_1` ↔ `asakusa_1` ↔ `asakusa_2` ↔ `tsuzumi_1` ↔ `tsuzumi_2` ↔ `natagumo_1` ↔ `natagumo_2` ↔ `natagumo_3` ↔ `mugen_1` ↔ `mugen_2` ↔ `yoshiwara_1` ↔ `yoshiwara_2` ↔ `yoshiwara_3`

Mỗi map mới có cổng tây về map trước (trong trại) và cổng đông sang map sau (có khu an toàn `gate_east`), trừ `yoshiwara_3` chỉ có cổng tây. Đường ngắn nhất từ `kumotori_1` tới `yoshiwara_3` qua 18 cổng.

### 2.4. Bố cục, trại và NPC

- **Map 5 cấp:** dùng chung hàm bố cục map luyện cấp mới của 4a mục 2.3 (trại `camp` 10 × 10 sát tường tây chứa cổng tây, điểm đến và `default` có `respawn: true`; lửa trại bán kính 1,5; `gate_east` 6 × 8 quanh điểm đến đông; bốn vùng bắc-giữa, bắc-đông, nam-giữa, nam-đông, vùng cấp thấp gần trại hơn). Mỗi map có vài khối tường trang trí riêng theo chủ đề; script kiểm tra liên thông (BFS của 4a).
- **`yoshiwara_3`:** trại như trên, không có cổng đông. Phần phía đông trại chia thành **năm dải dọc** nối tiếp từ tây sang đông, mỗi dải một vùng `r1` … `r5`, cấp tăng dần về phía đông. Ổ quỷ ở tâm dải.
- **`mugen_1`, `mugen_2`:** cùng luật map 5 cấp; tường trang trí là các vách ngang chia map thành dãy toa, mỗi vách chừa lối đi rộng ít nhất 3 ô.
- **NPC trại** (luật vị trí của 4b, 4c: trong trại, cách `default` 2–4 ô, cách nhau ít nhất 1,5 ô; Thủ kho cách `default` không quá 3 ô): `duoc_su` "Dược sư" (`shop` → `apothecary`), `xa_phu` "Xa phu" (`carriage`), `thu_kho` "Thủ kho" (`storage`). Không có NPC `quest`.
- **Tuyến Xa phu:** mỗi Xa phu có tuyến tới mọi map khác trong danh sách {`ubuyashiki`, các map luyện cấp}, theo thứ tự: `ubuyashiki` trước, rồi các map luyện cấp theo cấp thấp nhất tăng dần. Giá: 50 Xu tới `ubuyashiki`, 20 × cấp quái thấp nhất của map đích (tối thiểu 50) tới map luyện cấp. Xa phu của bốn map luyện cấp 4a và của Tổng hành dinh cũng có thêm 14 tuyến mới. Map `battle`, `instance` và `event_only` (7f–7h) không có trong tuyến.

| Đích | Giá | Đích | Giá |
| --- | --- | --- | --- |
| `fujikasane_2` | 420 | `natagumo_2` | 1.120 |
| `tay_bac_1` | 520 | `natagumo_3` | 1.220 |
| `asakusa_1` | 620 | `mugen_1` | 1.320 |
| `asakusa_2` | 720 | `mugen_2` | 1.420 |
| `tsuzumi_1` | 820 | `yoshiwara_1` | 1.520 |
| `tsuzumi_2` | 920 | `yoshiwara_2` | 1.620 |
| `natagumo_1` | 1.020 | `yoshiwara_3` | 1.720 |

Giá cao nhất 1.720 Xu bằng Xu của khoảng 5 con cấp 86 (game-design 5.12: 3–5 × cấp mỗi con).

### 2.5. Hai mươi sáu loại quái thường mới

`monster_stats.py` thêm vào `KINDS`, mọi thông số hành vi như `wanderer`, `rank: "normal"`, trừ các thay đổi ở cột cuối:

| id | Tên | Map | Khác `wanderer` |
| --- | --- | --- | --- |
| `big_demon` | Quỷ thân lớn | `fujikasane_2` | |
| `many_arms` | Quỷ nhiều tay | `fujikasane_2` | |
| `marsh_demon` | Quỷ ven đầm | `tay_bac_1` | |
| `mud_ambusher` | Quỷ phục kích dưới bùn | `tay_bac_1` | |
| `new_demon` | Quỷ mới biến đổi | `asakusa_1` | |
| `city_wanderer` | Quỷ lang thang đô thị | `asakusa_1` | |
| `henchman` | Quỷ tay sai | `asakusa_2` | |
| `manor_woods` | Quỷ rừng ngoài dinh | `tsuzumi_1` | |
| `long_nail` | Quỷ móng dài | `tsuzumi_1` | |
| `manor_dweller` | Quỷ trú trong dinh | `tsuzumi_2` | |
| `small_spider` | Nhện quỷ nhỏ | `natagumo_1` | |
| `thread_puppet` | Con rối bị tơ điều khiển | `natagumo_1` | |
| `spider_demon` | Nhện quỷ | `natagumo_2` | |
| `silk_cocoon` | Quái kén tơ | `natagumo_2` | |
| `silk_fiend` | Quái tơ nhện | `natagumo_3` | |
| `veteran_puppet` | Con rối tinh nhuệ | `natagumo_3` | |
| `train_tentacle` | Xúc tu | `mugen_1` | |
| `train_flesh` | Mô thịt | `mugen_1`, `mugen_2` | `speed` 0,6 (spec 7g) |
| `train_nightmare` | Quái ác mộng | `mugen_2` | |
| `night_lurker` | Quỷ ẩn phố đêm | `yoshiwara_1`, `yoshiwara_2` | |
| `sash_fragment` | Phân thể dải lụa | `yoshiwara_1`, `yoshiwara_2` | |
| `mask_demon` | Quỷ Mặt Nạ | `yoshiwara_3` | |
| `long_talon` | Quỷ Trảo Dài | `yoshiwara_3` | `attack_range` 2,0 (bằng chuẩn) |
| `blood_fang` | Quỷ Huyết Nha | `yoshiwara_3` | `hp_pct` 80, `attack_pct` 120 |
| `shadow_demon` | Quỷ Tàng Ảnh | `yoshiwara_3` | `speed` 0,96, `chase_speed` 1,44 |
| `scale_serpent` | Quỷ Lân Xà | `yoshiwara_3` | `damage_type` `magic`, `detect_range` 4 |

Bảng `levels` không đổi. `gamedata.Validate(maps)` đã kiểm tra mỗi vùng trỏ tới loại có thật và đủ cấp.

### 2.6. Ba trường mới của loại quái

Script ghi ba trường cho **mọi** loại (giá trị mặc định với các loại cũ), giống cách mọi loại đang ghi đủ mọi thông số:

| Trường | Kiểu | Mặc định | Ý nghĩa | Kiểm tra khi nạp |
| --- | --- | --- | --- | --- |
| `hp_pct` | số nguyên | 100 | HP tối đa và EXP gốc của con quái = ⌈giá trị của bảng × `hp_pct` / 100⌉ | 50–200 |
| `attack_pct` | số nguyên | 100 | Công dùng cho cận chiến, chưởng (và kỹ năng quái của 7d) = ⌈công của bảng × `attack_pct` / 100⌉ | 50–200 |
| `damage_type` | chuỗi | `phys` | `phys`: đòn của quái xét thủ vật lý của người chơi (như hiện tại); `magic`: xét thủ phép. Áp cho cả cận chiến và chưởng | `phys` hoặc `magic` |

- "Giá trị của bảng" là dòng `levels` của cấp con quái; khi 7d đã có thì là khối theo hạng (`ForRank`). Thủ của quái không đổi.
- Ví dụ Quỷ Huyết Nha cấp 88: HP, EXP gốc bằng ⌈80% của dòng cấp 88⌉, công bằng ⌈120%⌉. Script in các số này.
- Hàm làm tròn nằm trong `internal/combat` (`combat.ScalePct`), như mọi công thức chiến đấu.

## 3. Server

### 3.1. Dữ liệu

- `gamedata.Kind` thêm `HPPct`, `AttackPct int`, `DamageType string`. `Load` từ chối giá trị ngoài khoảng ở mục 2.6. Map file không có trường mới.
- `mapdata`, `world`: không đổi code. `World.Start` mở `min_zones` khu cho mỗi map như 4a.

### 3.2. Zone

- **Sinh quái** (`spawn`): HP tối đa = `combat.ScalePct(HP của bảng, kind.HPPct)`. Mọi chỗ đọc công hay EXP gốc của con quái từ bảng cấp (`z.area.Levels[m.level]`) đọc qua **một hàm** của zone trả chỉ số đã nhân của con đó (gợi ý: `z.statsOf(m)`), để không chỗ nào quên hệ số.
- **Đòn của quái lên người chơi** (cận chiến và chưởng; với 7d cả kỹ năng quái): thủ = thủ phép khi `damage_type` là `magic`, nếu không thì thủ vật lý.
- **EXP:** `payExpShare` dùng EXP gốc đã nhân; HP tối đa đã nhân nên tỷ lệ EXP theo sát thương giữ đúng r(L).
- Không có gói mới: `EntityEnter` đã mang `max_hp` thật của con quái.

### 3.3. Thứ tự tick

Không đổi.

## 4. Giao thức

Không đổi; `ProtocolVersion` giữ nguyên. Không có fixture mới.

## 5. Client Godot

- **Đồng bộ:** bump submodule `server`, chạy `tools/sync_server.sh` (chép 15 file map mới và đã đổi; `monsters.json` nếu 7d đã thêm vào danh sách đồng bộ).
- **Danh sách dài:** cửa sổ NPC Xa phu (4b) nay có tới 18 tuyến, cửa sổ Bản đồ (6a) liệt kê 19 map cùng NPC. Cả hai phải cuộn được trong khung dọc điện thoại; nếu chưa có thì bọc danh sách trong `ScrollContainer` (như `ui/bag_tab.gd`).
- Không có màn, lệnh hay lỗi mới.

## 6. Lệnh dev

Không có lệnh mới. Thử map cao dùng `devchar -level N -map <id> -x … -y …` như cũ.

## 7. Kiểm thử

**Script:** `mapgen.py --check` sạch, tự kiểm tra như 4a mục 2.2 (cổng hai chiều, mọi map tới được từ `kumotori_1`, mỗi vùng đủ ô) và 4b (NPC trong trại, giá Xa phu theo công thức); thêm: mỗi map luyện cấp có đúng `duoc_su`, `xa_phu`, `thu_kho`; tuyến của mỗi Xa phu là mọi map khác trong {thành, map luyện cấp}; khoảng cấp các vùng của map mới khớp bảng mục 2.1. `monster_stats.py --check` sạch; in bảng HP, công, EXP của Quỷ Huyết Nha cấp 88 so với chuẩn. `quest_data.py --check` vẫn sạch (không đổi vùng của map 1–20).

**Server** (`go test -race`):

- `combat`: `ScalePct(1000, 80) = 800`, `ScalePct(1001, 80) = 801` (làm tròn lên), `ScalePct(x, 100) = x`.
- `gamedata`: dữ liệu thật nạp được, có đủ 26 id mới với tên ở mục 2.5; `blood_fang` có 80/120, `scale_serpent` có `magic`; từ chối `hp_pct` 49 và 201, `attack_pct` 0, `damage_type` lạ; `Validate` với map thật không lỗi.
- `mapdata` (map thật): `LoadDir` nạp 19 map của 4a và 4d (cộng các map của phần đã triển khai trước); mọi map tới được từ `kumotori_1`; mỗi cổng có cổng ngược; đường cổng ngắn nhất từ `kumotori_1` tới `yoshiwara_3` dài 18; `yoshiwara_3` có năm vùng, cấp 86 … 90.
- Các test map thật đang đếm năm map hay NPC của năm map (4b, 4c, 6b) đổi sang đếm theo danh sách map thật.
- `zone`:
  - Quái có `hp_pct` 80 sinh với HP tối đa và `EntityEnter.max_hp` bằng 80% (làm tròn lên); người chơi hạ một mình nhận đúng EXP gốc đã nhân.
  - Quái có `attack_pct` 120: một đòn cận chiến lên người chơi bằng `Damage(⌈1,2 × công⌉, thủ vật lý, cấp)`; chưởng cũng dùng công đã nhân.
  - Quái `magic`: với người chơi có thủ vật lý 100, thủ phép 500, sát thương cận chiến và chưởng theo thủ phép; quái `phys` cùng cấp theo thủ vật lý.
  - Quái mặc định (100, 100, `phys`) cho đúng số như trước (test cũ giữ nguyên).
- `world`: `Start` mở 2 khu cho mỗi map luyện cấp thật và 1 cho thành.
- e2e `TestWorldExpansionOverWebSocket`: nhân vật cấp 25 (`DevSetLevel`) đặt trong `gate_east` của `fujikasane_1`; `UsePortalReq` `to_fujikasane_2` → `EnterWorldResp` map `fujikasane_2` tại `from_fujikasane_1`; nhận `EntityEnter` của một quái cấp 21–25; thoát, cho 5.000 Xu bằng helper store mà `devitem` dùng, đặt cạnh `xa_phu` của `fujikasane_2`, vào lại; `TravelReq` tới `yoshiwara_3` → ở `default` của `yoshiwara_3`, Xu giảm đúng 1.720; nhận `EntityEnter` của quái cấp 86–90.

**Client** (`tests/run.sh`): `world_graph` trên map thật: `reachable("kumotori_1")` có đủ 19 map; `route("kumotori_1", "yoshiwara_3")` dài 18 cổng, cổng đầu `to_ubuyashiki`, cổng cuối `to_yoshiwara_3`; `level_range("yoshiwara_3")` là (86, 90). Test A* và walker trên map thật của 6a tự chạy cho map mới (mọi NPC, cổng, điểm xuất hiện có đường). `sync --check`.

**Nghiệm thu:** server ở `:8199`. `test1` được `devchar -level 22 -map fujikasane_1 …` cạnh cổng đông, đi qua sang Vùng sâu, đánh Quỷ thân lớn; mở cửa sổ Xa phu, cuộn hết danh sách, đi xe tới Hầm Ngầm (đặt cấp 86 trước) và thấy năm loại quái; Quỷ Tàng Ảnh đuổi nhanh hơn; Quỷ Lân Xà gây ít sát thương hơn với nhân vật có thủ phép cao. Đo CPU của server khi không có ai online với 37 zone: dưới 5% một nhân; nếu cao hơn thì ghi lại để xử lý riêng, không chặn 4d.

## 8. Tài liệu cập nhật

- `docs/game-design.md` mục 5.5: cột id map, cách chia Chuyến Tàu và Yoshiwara – Ngõ đêm, đường ngầm thành hai map; ba tên đã sửa (mục 1.3). Mục 5.11: Hầm Ngầm năm vùng × 6 con, ba trường `hp_pct`, `attack_pct`, `damage_type`; Trảo Dài tầm bằng chuẩn. Mục 4.1: tuyến Xa phu tới mọi map luyện cấp. Mục 4.2: chính tuyến v1 dừng ở cấp 20, sau đó chỉ có nhiệm vụ mốc 30/40/50.
- Spec 4a mục 1.3 (dòng "Lượng nội dung"): các vùng 21–90 làm ở 4d.
- Tổng quan Giai đoạn 7 mục 3.6 và mục 4: phụ thuộc vào 4d/4e (sửa cùng commit với spec này).
- README hai repo: danh sách map, Xa phu tới mọi map.
- `CLAUDE.md`: không đổi (luật "dữ liệu quái sinh bằng script" và "công thức chiến đấu chỉ ở `internal/combat`" đã bao phần mới).

## 9. Khi 7d, 7e hoặc 7g đã triển khai trước 4d

- **7d đã có:** `mapgen.py` của 7d kiểm tra "mỗi map luyện cấp có đúng một khu thủ lĩnh". 4d đổi thành "map 1–20 có đúng một, map 21–90 không có" cho tới khi 4e thay bằng bảng của mình. Map mới ghi `bosses: []`. Mọi chỗ đọc chỉ số theo hạng (`ForRank`) nhân `hp_pct`, `attack_pct` sau khi chọn khối hạng. Kỹ năng quái dùng `damage` riêng của kỹ năng; `damage_type` của loại chỉ áp cho cận chiến và chưởng.
- **7e đã có:** Lệnh Diệt Quỷ tự sinh nhóm truy nã ở mọi khu của 14 map mới (luật "mọi map `training`"). `gamedata.Validate` của 7e kiểm tra tổng `count` của nhóm vừa vùng nhỏ nhất, nên kiểm tra lại với vùng Hầm Ngầm; mỗi dải có hàng trăm ô nên dư.
- **7g:** nếu 4d đã có thì task dữ liệu của 7g không thêm lại ba loại quái tàu (mục 1.3), chỉ thêm `enmu`.
- **7f:** Hoang Sơn có `min_level` 50 và không có tuyến Xa phu; luật tuyến ở mục 2.4 đã bỏ qua map `battle`.
