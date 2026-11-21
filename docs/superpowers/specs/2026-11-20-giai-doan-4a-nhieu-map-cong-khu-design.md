# Giai đoạn 4a — Nhiều map, cổng và đổi khu

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng). Đã triển khai đủ server (Task 1–9) và client (Task 10–13, client 5138fc9; sửa sau review tổng: server b298efc, 71afd80, client cc2d217); smoke OK.

Spec này thiết kế phần đầu của Giai đoạn 4 (Thế giới) cho server (repo này) và client Godot (`../DemonSlayer2DGame`). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 5.1–5.5 và 5.7 (hồi sinh); khi hai tài liệu lệch nhau về luật chơi thì game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như các spec trước, nhất là [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md), [3b](2026-11-20-giai-doan-3b-cuong-hoa-design.md) và [3c](2026-11-20-giai-doan-3c-tuong-sinh-kham-ngoc-tay-luyen-design.md). Giai đoạn 3 được triển khai trước Giai đoạn 4, nên spec này viết trên giao thức 8 và migration 00006.

## 1. Mục tiêu và phạm vi

### 1.1. Chia Giai đoạn 4

Lộ trình (spec Giai đoạn 0 mục 1.1) ghi Giai đoạn 4 gồm nhiều map, cổng, khu/kênh, NPC, cửa hàng bình, kho, Xu, Lượng. Xu đã có từ 3a. Phần còn lại chia ba phần, làm theo thứ tự, mỗi phần xong là chơi được:

| Phần | Nội dung | Kết quả chơi được |
| --- | --- | --- |
| **4a** (spec này) | Năm map (một thành, bốn map luyện cấp 1–20), cổng chuyển map, đổi khu, danh sách khu, điểm hồi sinh theo map, sáu loại quái mới, script sinh map | Đi bộ qua cổng từ Kumotori tới Fujikasane, đổi khu, đánh quái tới cấp 20 |
| 4b ([spec](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md)) | NPC, cửa hàng Xu, bán đồ, bình HP/MP, giấy về thành, xa phu, rèn đồ tại Thợ rèn | Mua bình, uống bình, bán đồ, đi xe, về thành |
| 4c ([spec](2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md)) | Kho chung tài khoản, Lượng, Cửa hàng Lượng, hồi sinh tại chỗ | Cất đồ cho nhân vật khác, hồi sinh tại chỗ bằng Lượng |

### 1.2. Mục tiêu 4a

Nhân vật mới vẫn bắt đầu ở Kumotori. Đi về phía đông qua cổng là tới Tổng hành dinh Ubuyashiki, một thành an toàn; đi tiếp qua các cổng là tới Sagiri (cấp 6–10 và 11–15) rồi Fujikasane (cấp 16–20). Qua cổng thì nhân vật giữ nguyên HP, MP, đồ, Xu và hồi chiêu. Bấm tên map trên HUD để xem các khu đang mở và số người, rồi đổi sang khu khác ngay tại chỗ. Chết thì về điểm hồi sinh của chính map đó. Thoát game rồi vào lại thì vẫn ở map đang đứng.

### 1.3. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chia giai đoạn | 4a, 4b, 4c như mục 1.1 | Mỗi phần một kế hoạch vừa sức và kết thúc bằng thứ chơi được; 4c gom hai thứ theo tài khoản (kho, Lượng) |
| Lượng nội dung | Năm map: Tổng hành dinh Ubuyashiki (thành), Kumotori (1–5), Sagiri – Chân núi (6–10), Sagiri – Rừng sương mù (11–15), Fujikasane – Rừng tuyển chọn (16–20). Các vùng 21–90 của game-design 5.5 làm sau bằng cùng script (đã làm ở [Giai đoạn 4d](2026-11-20-giai-doan-4d-the-gioi-mo-rong-design.md): 14 map) | Đủ cho một vòng chơi từ cấp 1 tới khoảng cấp 20 và có một thành dịch vụ; quái từ cấp 10 cần trang bị của Giai đoạn 3 |
| Cách soạn map | Script `tools/mapgen.py` sinh `maps/*.json` từ khai báo hình chữ nhật (tường, khu an toàn, vùng quái, cổng); không sửa tay JSON. Script tự kiểm tra và có `--check`; test Go so file đã commit với script | `kumotori_1.json` hiện được soạn tay; năm map với lưới 64 × 48 sửa tay rất dễ sai, nhất là cổng hai chiều |
| `kumotori_1` | Sinh lại bằng script với cùng tường, trại, lửa trại và bốn vùng quái; thêm cổng phía đông, điểm đến và một khu an toàn ở cổng | Vị trí đã lưu của nhân vật vẫn hợp lệ |
| Đồ thị cổng | Chuỗi Kumotori ↔ Ubuyashiki ↔ Sagiri 1 ↔ Sagiri 2 ↔ Fujikasane | Thành nằm giữa map tân thủ và vùng cấp 6+, người chơi tự đi ngang qua và gặp dịch vụ (4b) |
| Điểm bắt đầu | Giữ `START_MAP=kumotori_1` | Cấp 1 cần đánh quái ngay |
| Cổng | Hình chữ nhật ô sát tường biên. Client tự gửi `UsePortalReq` khi nhân vật bước vào; server nhận nếu nhân vật cách hình chữ nhật không quá 1 ô | Không cần nút; dung sai 1 ô bù trễ mạng (4 ô/giây × khoảng 0,1 giây) |
| Điểm đến | Điểm xuất hiện có tên ở map đích, cách mọi cổng của map đó ít nhất 2 ô | Đến nơi không tự kích hoạt cổng ngược lại |
| Khu an toàn ở cổng | Mỗi cổng không nằm trong trại có một khu an toàn 6 × 8 ô quanh điểm đến | Quái không đứng chờ ngay đầu cổng |
| Khu khi qua cổng | Cùng số khu ở map đích nếu khu đó đang mở và còn chỗ; nếu không thì khu đầu tiên còn chỗ (đầy hết thì mở khu mới) | Tổ đội (Giai đoạn 5) đi cùng nhau qua cổng sẽ vẫn chung khu |
| Số khu tối thiểu | Trường `min_zones` của map: luyện cấp 2, thành 1. Khu có số lớn hơn mở khi các khu đều đầy và đóng khi trống 5 phút như hiện tại | Game-design 5.3: map luyện cấp có nhiều khu; luôn có ít nhất hai khu để đổi |
| Sức chứa khu | Giữ `ZONE_CAPACITY` mặc định 50 | Game-design 5.3: 30–50 người/khu, chưa kiểm chứng |
| Đổi khu | Giữ nguyên vị trí; được đổi khi đang giao chiến (game-design 5.1); không được khi chết; hồi 10 giây giữa hai lần đổi khu | Hồi chiêu chặn bấm liên tục, mỗi lần đổi tốn một lần lưu và một lần vào zone |
| Trạng thái khi chuyển zone | Zone cũ trao trạng thái trong bộ nhớ cho zone mới (HP, MP, cấp, EXP, điểm, chiêu, Xu, đồ, hồi chiêu còn lại); không đọc lại DB. Zone cũ lưu ngay với map và vị trí đích | Không chờ DB; sập server giữa chừng thì nhân vật ở map đích |
| Hồi chiêu khi chuyển zone | Giữ thời gian còn lại của chém, từng chiêu và đổi khu; `EnterWorldResp` gửi lại cho client | Không đổi khu để làm mới chiêu |
| Kết quả tẩy luyện đang chờ (3c) | Mất khi chuyển zone, như spec 3c mục 4.5 ("rời zone") | Giữ một luật |
| Đồ dưới đất (3a) | Ở lại zone cũ; quay lại cùng khu trong 60 giây vẫn thấy | Luật 3a theo nhân vật |
| Id thực thể khi chuyển zone | Giữ nguyên | Id lấy từ bộ đếm chung của world nên không trùng |
| Hồi sinh | Về điểm hồi sinh gần nhất **trong cùng map** (điểm xuất hiện có `respawn: true`, phải nằm trong khu an toàn) | Game-design 5.7 "khu an toàn gần nhất của map đó" |
| Thoát game khi đã chết | Vào lại ở điểm hồi sinh gần vị trí đã lưu nhất | Như cũ, nay theo nhiều điểm |
| Map thành | Toàn bộ ô đi được nằm trong khu an toàn; không có vùng quái | Game-design 5.1: cấm PK tuyệt đối, không quái |
| Kiểm tra liên thông | `mapdata` kiểm tra mọi điểm xuất hiện, cổng, ổ quỷ và lửa trại đi tới được từ điểm `default` (BFS bốn hướng trên ô đi được) | Bắt lỗi tường bịt kín khi sửa script |
| Quái mới | Sáu loại quái thường, hai loại mỗi map mới, tên theo game-design 5.5; hành vi giống `wanderer` | Chỉ số theo cấp dùng chung bảng `levels`; hình ảnh chưa có |
| Tinh anh, thủ lĩnh | Không làm ở Giai đoạn 4 | Cần kỹ năng quái riêng (game-design 5.10); đề xuất một spec riêng sau Giai đoạn 4 |
| Client khi chuyển map | Nạp lại màn World từ `EnterWorldResp` mới, như lúc vào game | Một đường code; map, thực thể, dự đoán đều làm lại sạch |

### 1.4. Không làm ở 4a

NPC, cửa hàng, bán đồ, bình thuốc, giấy về thành, xa phu (4b); kho, Lượng, hồi sinh tại chỗ (4c); các map cấp 21–90, Điệp Phủ, Làng Thợ Rèn, map chiến sự và phó bản; tinh anh và thủ lĩnh; tự tìm đường giữa các map và bấm để đi (Giai đoạn 6); PK (Giai đoạn 5); bản đồ thế giới, bản đồ nhỏ; hình ảnh map thật (vẫn vẽ lưới ô).

## 2. Dữ liệu

### 2.1. Định dạng file map

Các trường mới trong `maps/<id>.json` (server vẫn từ chối trường lạ):

```json
{
  "id": "sagiri_1",
  "name": "Sagiri – Chân núi, đường đến ngôi đền",
  "type": "training",
  "width": 64,
  "height": 48,
  "min_zones": 2,
  "collision": ["…"],
  "safe_zones": [{"id": "camp", "x": 1, "y": 19, "w": 10, "h": 10}],
  "rest_points": [{"id": "bonfire", "x": 4.5, "y": 21.5, "radius": 1.5}],
  "spawn_points": [
    {"id": "default", "x": 6.5, "y": 24.5, "respawn": true},
    {"id": "from_ubuyashiki", "x": 4.5, "y": 24.0},
    {"id": "from_sagiri_2", "x": 59.5, "y": 24.0}
  ],
  "portals": [
    {"id": "to_ubuyashiki", "x": 1, "y": 22, "w": 1, "h": 4, "to_map": "ubuyashiki", "to_spawn": "from_sagiri_1"},
    {"id": "to_sagiri_2", "x": 62, "y": 22, "w": 1, "h": 4, "to_map": "sagiri_2", "to_spawn": "from_sagiri_1"}
  ],
  "regions": ["…"]
}
```

(Ví dụ rút gọn; tọa độ chính xác do script quyết theo mục 2.3.)

- `min_zones`: số khu luôn mở, 1–10, bắt buộc.
- `spawn_points[].respawn`: không bắt buộc, mặc định `false`. Điểm có `respawn: true` là điểm hồi sinh.
- `portals`: hình chữ nhật ô `x, y, w, h` (cùng nghĩa với khu an toàn), map đích `to_map` và điểm xuất hiện ở map đích `to_spawn`.

**Kiểm tra mỗi map** (thêm vào `mapdata.Parse`, sai thì dừng và báo lỗi):

- `min_zones` trong 1–10.
- Có ít nhất một khu an toàn và ít nhất một điểm hồi sinh; mọi điểm hồi sinh nằm trong khu an toàn.
- Map `town`: không có vùng quái; mọi ô đi được nằm trong khu an toàn.
- Cổng: `id` khác rỗng, không trùng; `w`, `h` dương; nằm trong map; mọi ô của cổng đi được; hai cổng không chung ô; `to_map` và `to_spawn` khác rỗng; `to_map` khác chính map này.
- Liên thông: từ ô của điểm `default`, đi bốn hướng trên ô đi được, tới được ô của mọi điểm xuất hiện, ít nhất một ô của mỗi cổng, ô ổ quỷ của mọi vùng và ô tâm của mọi lửa trại.

**Kiểm tra giữa các map** (`mapdata.LoadDir`, sau khi đọc hết):

- `to_map` có trong thư mục; `to_spawn` là một điểm xuất hiện của map đó.
- Điểm đến cách mọi cổng của map đích ít nhất 2 ô (khoảng cách từ điểm tới hình chữ nhật, mục 3.3).

Kiểm tra cả đồ thị (mọi map đi tới được từ `kumotori_1`, mỗi cổng có cổng ngược) nằm trong script và test Go trên thư mục `maps/` thật, không ở lúc server khởi động, để test được dùng map nhỏ đứng riêng.

### 2.2. Script `tools/mapgen.py`

- Mỗi map khai báo trong script bằng: kích thước, các hình chữ nhật tường (`#`, ngoài viền tường bao quanh), khu an toàn, lửa trại, điểm xuất hiện, cổng, vùng quái, `min_zones`.
- `python3 tools/mapgen.py --write` ghi lại mọi `maps/*.json`; `--check` so với file đang có, in tên map lệch và trả mã 1; không tham số thì in tóm tắt (kích thước, số ô đi được, số quái, cổng).
- Định dạng ra cố định: thụt 2 dấu cách, `ensure_ascii=False`, mỗi hàng `collision` một dòng, khóa theo thứ tự ở mục 2.1, xuống dòng ở cuối file.
- Script tự kiểm tra trước khi ghi: các luật mục 2.1; mỗi cổng A→B có cổng B→A; mọi map đi tới được từ `kumotori_1`; mỗi vùng có đủ ô cho số quái (theo luật `RegionTiles`: ô đi được, ngoài khu an toàn và dải 1 ô quanh nó).
- Test Go (`mapdata`) chạy `--check` (bỏ qua nếu không có `python3`), rồi `LoadDir("../../maps")` và kiểm tra đồ thị như trên.

### 2.3. Năm map

| id | Tên | Loại | Kích thước | `min_zones` | Vùng quái (loại, cấp, số con) |
| --- | --- | --- | --- | --- | --- |
| `ubuyashiki` | Tổng hành dinh Ubuyashiki | `town` | 40 × 32 | 1 | — |
| `kumotori_1` | Kumotori – Đường núi tuyết | `training` | 64 × 48 | 2 | như hiện tại: `wanderer` 1–2 × 8, 2–3 × 7; `snow_ghoul` 3–4 × 7, 4–5 × 8 |
| `sagiri_1` | Sagiri – Chân núi, đường đến ngôi đền | `training` | 64 × 48 | 2 | `mountain_demon` 6–7 × 8, 7–8 × 7; `ambusher` 8–9 × 7, 9–10 × 8 |
| `sagiri_2` | Sagiri – Rừng sương mù | `training` | 64 × 48 | 2 | `mist_lurker` 11–12 × 8, 12–13 × 7; `long_claw` 13–14 × 7, 14–15 × 8 |
| `fujikasane_1` | Fujikasane – Rừng tuyển chọn | `training` | 64 × 48 | 2 | `forest_demon` 16–17 × 8, 17–18 × 7; `captive_demon` 18–19 × 7, 19–20 × 8 |

Mỗi map luyện cấp nhỏ có 4 vùng, 30 quái thường (game-design 5.11).

**Cổng:**

| Map | Cổng | Phía | Tới map | Điểm đến |
| --- | --- | --- | --- | --- |
| `kumotori_1` | `to_ubuyashiki` | đông | `ubuyashiki` | `from_kumotori_1` |
| `ubuyashiki` | `to_kumotori_1` | tây | `kumotori_1` | `from_ubuyashiki` |
| `ubuyashiki` | `to_sagiri_1` | đông | `sagiri_1` | `from_ubuyashiki` |
| `sagiri_1` | `to_ubuyashiki` | tây | `ubuyashiki` | `from_sagiri_1` |
| `sagiri_1` | `to_sagiri_2` | đông | `sagiri_2` | `from_sagiri_1` |
| `sagiri_2` | `to_sagiri_1` | tây | `sagiri_1` | `from_sagiri_2` |
| `sagiri_2` | `to_fujikasane_1` | đông | `fujikasane_1` | `from_sagiri_2` |
| `fujikasane_1` | `to_sagiri_2` | tây | `sagiri_2` | `from_fujikasane_1` |

Tên cổng là `to_<map đích>`, tên điểm đến là `from_<map nguồn>`.

**Luật bố cục** (script phải theo; phần trang trí như tường, khối nhà do người viết script chọn):

- Cổng rộng 1 ô, cao 4 ô, ở cột 1 (phía tây) hoặc cột `width − 2` (phía đông), 4 hàng giữa map (`height / 2 − 2` tới `height / 2 + 1`).
- Điểm đến cách cột cổng 3 ô vào trong, ở hàng giữa của cổng (với cổng tây: `x = 4,5`, `y = height / 2`, tức tâm của cổng theo chiều dọc; đối xứng với cổng đông: `x = width − 4,5`). Khoảng cách tới cổng là 2,5 ô.
- Map luyện cấp mới (`sagiri_1`, `sagiri_2`, `fujikasane_1`): **trại** là khu an toàn `camp` 10 × 10 sát tường tây, giữa chiều cao, chứa cổng tây và điểm đến của nó; điểm `default` (hồi sinh) ở giữa trại; lửa trại bán kính 1,5 trong trại. Cổng đông có khu an toàn `gate_east` 6 × 8 quanh điểm đến. Bốn vùng quái chia phần còn lại (bắc-giữa, bắc-đông, nam-giữa, nam-đông), vùng cấp thấp gần trại hơn.
- `kumotori_1`: giữ tường, trại `camp` (1, 1, 10, 10), lửa trại, bốn vùng và điểm `default` (5,5; 5,5) như hiện tại, `default` có `respawn: true`; thêm cổng đông, điểm `from_ubuyashiki` và khu an toàn `gate_east`.
- `ubuyashiki`: khu an toàn `town` phủ toàn bộ phần trong viền; vài khối tường làm nhà; điểm `default` (hồi sinh) giữa map; một lửa trại gần `default`; cổng tây tới Kumotori, cổng đông tới Sagiri 1.

### 2.4. Quái mới

`monster_stats.py` thêm sáu loại vào `KINDS`, cùng mọi thông số hành vi với `wanderer`, `rank: "normal"` (3a), chỉ khác `id` và `name`:

| id | Tên | Map |
| --- | --- | --- |
| `mountain_demon` | Quỷ rừng núi | `sagiri_1` |
| `ambusher` | Quỷ phục kích | `sagiri_1` |
| `mist_lurker` | Quỷ ẩn sương | `sagiri_2` |
| `long_claw` | Quỷ vuốt dài | `sagiri_2` |
| `forest_demon` | Quỷ Rừng | `fujikasane_1` |
| `captive_demon` | Quỷ bị giam trên núi | `fujikasane_1` |

`monsters.json` sinh lại bằng `--write-json`; bảng `levels` không đổi. `gamedata.Validate(maps)` đã kiểm tra mỗi vùng trỏ tới loại có thật và đủ cấp.

## 3. Server

### 3.1. Mở và đóng khu

- `World.Start` mở `min_zones` khu cho mỗi map (khu 1 … `min_zones`).
- `reserve` giữ như cũ: khu đầu tiên còn chỗ, nếu không thì mở khu mới.
- `closeIdle` không bao giờ đóng khu có số ≤ `min_zones` của map đó (thay luật "không đóng khu 1").

### 3.2. Chuyển zone

Mọi lần chuyển (cổng, đổi khu; 4b thêm giấy về thành và xa phu) đi qua một đường `World.Transfer`, chạy trên goroutine của kết nối như `World.Enter`:

1. **Đích.** World tìm map, vị trí và khu mong muốn từ dữ liệu map. Lỗi ở bước này trả ngay, chưa đụng tới zone nào.
2. **Giữ chỗ.** World giữ một chỗ ở zone đích (tăng `reserved` như `reserve`), để không ai chiếm mất giữa lúc kiểm tra và lúc vào.
3. **Rời zone cũ.** World gửi lệnh rời tới zone hiện tại kèm hàm kiểm tra của loại chuyển. Zone chạy hàm đó trên goroutine của mình với trạng thái thật của người chơi:
   - lỗi: zone trả mã lỗi, không đổi gì; world nhả chỗ đã giữ;
   - hợp lệ: zone áp chi phí nếu có (4b), dựng bản trao tay (mục 3.2.1), xóa người chơi khỏi lưới và danh sách như khi thoát, đếm lại số người, rồi xếp một lần lưu với **map và vị trí đích**; trả bản trao tay và kênh kết quả lưu.
4. **Vào zone mới.** World đưa bản trao tay vào zone đã giữ chỗ, với `RespSeq` là `seq` của yêu cầu. Zone mới gửi `EnterWorldResp` như khi vào game. Nếu zone đó vừa đóng hay đầy (không xảy ra khi giữ chỗ đúng), world thử lại như `Enter` (tới 10 lần, chọn khu theo luật thường).
5. **Kết quả.** World trả `Membership` mới; gateway thay `c.member`. Nếu world đã dừng (tắt server) sau bước 3, gateway chờ kênh lưu xong rồi đóng kết nối với `SERVER_SHUTDOWN`.

Gateway không đọc gói khác của kết nối trong lúc chuyển (cùng goroutine), nên không có lệnh nào tới zone cũ sau khi rời. Lệnh chuyển chỉ chịu giới hạn gói chung.

Saver ghi theo thứ tự trên một goroutine và gộp theo nhân vật, nên lần lưu của zone cũ luôn nằm dưới mọi lần lưu sau của zone mới. Khi thoát, `cleanup` chờ lần lưu của zone đang ở, đủ cho luật `Takeover`.

#### 3.2.1. Bản trao tay

Bản trao tay là `PlayerInfo` dựng từ trạng thái sống: cấp, EXP, điểm tiềm năng, điểm chiêu, chiêu đã học, HP và MP thật (không dùng −1), Xu, hành trang và đồ đang mặc (bản sao sâu, gồm `pity` và `gems`), cùng **hồi chiêu còn lại**:

- chém mặc định (từ lần chém cuối và nhịp chém), từng chiêu (`skillReady`), đổi khu (mục 3.4); 4b thêm bình HP và bình MP;
- mỗi mục là thời gian còn lại; zone mới đổi ra tick của nó, làm tròn lên (`ticks`), mục đã hết thì bỏ.

Không trao: hướng đi (đứng yên khi tới), đếm nghỉ ở lửa trại, kết quả tẩy luyện đang chờ, phần lẻ hồi HP theo mốc (3b), mục tiêu và đòn đang bay. Quái ở zone cũ đang nhắm người chơi đó mất mục tiêu như khi người chơi thoát.

### 3.3. Cổng

`UsePortalReq{portal_id}`. Kiểm tra theo thứ tự, lỗi đầu tiên được trả:

1. Map hiện tại không có cổng với id đó: `BAD_REQUEST` (world).
2. Nhân vật chết: `DEAD` (zone).
3. Khoảng cách từ nhân vật tới hình chữ nhật cổng lớn hơn 1 ô: `OUT_OF_RANGE` (zone).

Khoảng cách từ điểm (x, y) tới hình chữ nhật ô (X, Y, W, H) là khoảng cách tới điểm gần nhất của vùng [X, X + W] × [Y, Y + H]; bằng 0 khi điểm nằm trong.

Hợp lệ: nhân vật tới `to_spawn` của `to_map`, khu chọn theo mục 1.3 ("Khu khi qua cổng"). Trả lời là `EnterWorldResp` mang `seq` của yêu cầu.

### 3.4. Đổi khu

`ChangeZoneReq{zone}`:

1. `zone` < 1 hoặc bằng khu hiện tại: `BAD_REQUEST`.
2. Map hiện tại không có khu đang chạy với số đó: `ZONE_NOT_FOUND`.
3. Khu đó đầy (số người + chỗ đang giữ ≥ sức chứa): `ZONE_FULL`.
4. Nhân vật chết: `DEAD`.
5. Chưa đủ 10 giây kể từ lần đổi khu trước: `ON_COOLDOWN`.

Bước 2–3 do world kiểm tra và giữ chỗ trước, bước 4–5 do zone kiểm tra; thứ tự trên vì thế cố định. Hợp lệ: nhân vật vào khu đó ở **đúng vị trí cũ**; bản trao tay có hồi đổi khu 10 giây. Đi qua cổng không đặt và không bị chặn bởi hồi đổi khu. Trả lời là `EnterWorldResp`.

### 3.5. Danh sách khu

`ZoneListReq{}` do gateway trả thẳng từ world (không qua zone): các khu đang chạy của map hiện tại, theo số khu tăng dần, mỗi khu có số người hiện có và sức chứa. Chỉ giới hạn gói chung.

### 3.6. Hồi sinh và vào game

- `Area.Respawn` (một điểm) đổi thành danh sách điểm hồi sinh của map, theo thứ tự id.
- Hồi sinh (bấm hoặc tự sau 60 giây): tới điểm hồi sinh gần vị trí chết nhất; bằng nhau thì lấy điểm đứng trước trong danh sách.
- Vào game với HP 0 (thoát khi đã chết): điểm hồi sinh gần vị trí đã lưu nhất.
- `placement`: map đã lưu không còn hoặc vị trí không đi được thì về điểm `default` của `START_MAP`, như cũ.

### 3.7. Thứ tự tick

Không đổi. Lệnh rời zone được xử lý trong `drain`, như các lệnh khác.

## 4. Giao thức

`ProtocolVersion` lên **9**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 83 | `UsePortalReq` | `string portal_id = 1` |
| 84 | `ChangeZoneReq` | `int32 zone = 1` |
| 85 | `ZoneListReq` | rỗng |
| 86 | `ZoneListResp` | `repeated ZoneInfo zones = 1` |

```proto
message ZoneInfo {
  int32 zone = 1;
  int32 players = 2;
  int32 capacity = 3;
}

// Time left on a cooldown when entering a zone. key: a skill id, "slash",
// "zone_change"; phase 4b adds "potion_hp" and "potion_mp".
message Cooldown {
  string key = 1;
  int32 remaining_ms = 2;
}
```

- Trả lời thành công của `UsePortalReq` và `ChangeZoneReq` là `EnterWorldResp` (cùng `seq`). Ghi chú của `EnterWorldResp` nói rõ nó cũng là trả lời của các lệnh chuyển zone.
- `EnterWorldResp.cooldowns = 12` (`repeated Cooldown`): rỗng khi vào game từ màn chọn nhân vật.
- `ErrorCode`: `ZONE_NOT_FOUND = 38`.
- Fixture mới: `use_portal_req.hex`, `zone_list_resp.hex` (hai khu), `enter_world_resp_cooldowns.hex` (một chiêu và `zone_change`). Fixture cũ sinh lại.

## 5. Client Godot

### 5.1. Lõi (`core/`, không dùng Node)

- **`core/map_grid.gd`**: đọc thêm `portals` (`{id, rect: Rect2i, to_map, to_spawn}`); `static func name_of(map_id) -> String` đọc tên map khác từ `data/maps` (có bộ nhớ đệm), để vẽ tên đích của cổng; `portal_at(pos) -> String`.
- **`core/portal_watch.gd`**: `reset(pos)` khi vào map (đứng sẵn trong cổng thì không kích hoạt cho tới khi ra khỏi cổng); `update(pos) -> String` trả id cổng khi vị trí dự đoán **vừa bước vào** một cổng (từ ngoài vào trong), rỗng nếu không; sau một lần trả id thì im cho tới `reset` hoặc tới khi ra khỏi cổng rồi vào lại.
- **`core/skill_book.gd`**: `set_ready_in(id, sec, now)` để nạp hồi chiêu từ `EnterWorldResp.cooldowns`; hồi chém và đổi khu giữ ở `screens/world.gd`.
- **`core/errors.gd`**: ZONE_NOT_FOUND "Khu không còn mở". **`core/wire.gd`**: thêm payload mới vào `PAYLOADS`.

### 5.2. Mạng

`Net.request(env, hold_on_reply := false)`: khi `hold_on_reply` và trả lời tới, `Net` bật giữ gói đẩy (`hold`) **trước khi** trả trả lời cho người chờ. Nhờ vậy các `EntityEnter`, `WorldTick` của zone mới đến ngay sau `EnterWorldResp` được giữ cho màn World mới, còn các gói của zone cũ đến trước trả lời vẫn tới màn cũ. Trả lời là lỗi thì người gọi gọi `Net.release()`.

### 5.3. Giao diện

- **Cổng** (`ui/map_view.gd`): hình chữ nhật màu tím nhạt có viền sáng nhấp nháy, tên map đích viết phía trên ("→ Tổng hành dinh Ubuyashiki").
- **Đi qua cổng** (`screens/world.gd`): sau mỗi tick dự đoán, `portal_watch.update`; có id thì gửi `UsePortalReq` một lần với `hold_on_reply`, hiện lớp phủ "Đang chuyển map…" và khóa gửi lệnh đánh, chiêu cho tới khi có trả lời. Thành công: `Router.enter_world` = trả lời, nạp lại màn World. Lỗi: bỏ lớp phủ, `Net.release()`, hiện thông báo; `OUT_OF_RANGE` thì không báo (đi vào lại là thử lại).
- **Tên map và khu** trên HUD (`set_place`, đã có) chạm được: mở **cửa sổ khu** (`ui/zone_window.gd`) gửi `ZoneListReq`; mỗi dòng "Khu N · x/50" với nhãn "Đang ở" ở khu hiện tại, nút **Vào** ở khu khác (khóa và ghi "Đầy" khi đầy, ghi "Chờ n giây" khi còn hồi đổi khu). Bấm Vào gửi `ChangeZoneReq` như đi qua cổng.
- **Sau khi nạp lại**: `screens/world.gd` nạp `cooldowns` vào sách chiêu, hồi chém và hồi đổi khu; `Router` nhớ ô đang chọn của thanh chiêu để chọn lại.
- Bảng chết không đổi ("Về khu an toàn"); nhân vật xuất hiện ở điểm hồi sinh mới do server gửi.

### 5.4. Smoke

`tools/smoke.sh` sau các bước cũ: gửi `ChangeZoneReq` sang khu 2 rồi kiểm tra `EnterWorldResp.zone` là 2; dùng `devchar -map kumotori_1 -x 59.5 -y 24.0` (trong khu an toàn ở cổng đông) rồi vào lại, đi về phía đông tới khi client tự gửi `UsePortalReq`, kiểm tra map mới là `ubuyashiki`. In "SMOKE OK" kèm hai lần chuyển.

### 5.5. Đồng bộ

`tools/sync_server.sh` đã chép cả thư mục `maps/`; không đổi gì thêm.

## 6. Lệnh dev

`devchar` thêm `-map <id>` (bắt buộc đi kèm `-x` và `-y`): đặt nhân vật đang thoát game vào map và vị trí đó. Lệnh đọc map từ `MAPS_DIR` (mặc định `./maps`) và từ chối map không có hoặc vị trí không đi được. Dùng được một mình hoặc cùng `-level`.

## 7. Kiểm thử

**Script:** `tools/mapgen.py` tự kiểm tra như mục 2.2; `monster_stats.py` giữ các kiểm tra cũ và in thêm sáu loại.

**Server** (`go test -race`, mỗi package một hoặc hai file test, chia section):

- `mapdata`: map có cổng, điểm hồi sinh, `min_zones` nạp được; mỗi vi phạm mục 2.1 bị từ chối (`min_zones` 0 và 11; không có điểm hồi sinh; điểm hồi sinh ngoài khu an toàn; thành có vùng quái; thành có ô đi được ngoài khu an toàn; cổng ngoài map; cổng trên ô tường; trùng id cổng; hai cổng chung ô; cổng trỏ chính map này; điểm xuất hiện bị tường bịt kín); `LoadDir` từ chối `to_map` không có, `to_spawn` không có, điểm đến cách cổng của map đích 1,5 ô; khoảng cách điểm tới hình chữ nhật (trong, cạnh, góc).
- `mapdata` dữ liệu thật: `--check` khớp; năm map nạp được; mọi map đi tới được từ `kumotori_1`; mỗi cổng có cổng ngược.
- `gamedata`: `monsters.json` có tám loại; `Validate` với năm map thật không lỗi.
- `world`: `Start` mở đúng `min_zones` khu; `closeIdle` không đóng khu 2 của map có `min_zones` 2 nhưng đóng khu 3 trống quá hạn; qua cổng từ khu 2 vào khu 2 của map đích khi còn chỗ, vào khu 1 khi khu 2 đích không mở; zone từ chối thì chỗ đã giữ được nhả (số người và `reserved` về như cũ); đổi khu từng mã lỗi theo thứ tự; danh sách khu đúng số người.
- `zone`: lệnh rời với hàm kiểm tra lỗi không đổi gì; hợp lệ thì người chơi rời lưới, `Count` giảm, lần lưu mang map và vị trí đích; bản trao tay mang HP/MP thật, Xu, đồ (bản sao không chung slice), chiêu; chiêu còn 3 giây hồi ở zone cũ vẫn bị `ON_COOLDOWN` ở zone mới trong khoảng đó và dùng được sau đó; `EnterWorldResp.cooldowns` có mục đó; quái đang đuổi người chơi rời zone thì mất mục tiêu; `UsePortalReq` cách cổng 0,9 ô được, 1,1 ô bị `OUT_OF_RANGE`; hồi đổi khu 10 giây; hồi sinh về điểm gần nhất trong hai điểm; vào game với HP 0 về điểm gần vị trí đã lưu nhất.
- `gateway`: `ProtocolVersion` là 9; giải mã fixture mới; `UsePortalReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- `store`/`cmd/devchar`: `-map` đặt đúng map và vị trí; map lạ và ô tường bị từ chối.
- e2e `TestPortalsOverWebSocket`: hai người chơi; A được `devchar` đặt cạnh cổng đông của Kumotori, B đứng ở Ubuyashiki; A gửi `UsePortalReq` và nhận `EnterWorldResp` map `ubuyashiki` tại `from_kumotori_1`; B nhận `EntityEnter` của A; A đổi khu ở Ubuyashiki thì bị `ZONE_NOT_FOUND` (thành chỉ một khu); A qua cổng về Kumotori rồi đổi sang khu 2 thì thành công, đổi lại ngay thì bị `ON_COOLDOWN`; A thoát rồi vào lại thì ở Kumotori khu được chọn theo luật thường, đúng vị trí.

**Client** (`tests/run.sh`): `map_grid` (đọc cổng, `name_of`, `portal_at`); `portal_watch` (bước vào thì trả id một lần, đứng trong không lặp, ra rồi vào lại thì trả lại, `reset` trong cổng không kích hoạt); `skill_book.set_ready_in`; giải mã fixture mới; `sync --check`. Smoke theo mục 5.4.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito, Viêm) và `test2` (Hana, Thủy) đi từ Kumotori qua Ubuyashiki tới Fujikasane, thấy nhau ở thành; đổi khu ở Sagiri; chết ở Sagiri thì về trại Sagiri; thoát rồi vào lại vẫn ở map cũ.

## 8. Tài liệu cập nhật

- `docs/game-design.md` mục 5.3 (luôn có ít nhất hai khu ở map luyện cấp, hồi đổi khu 10 giây, đổi khu giữ vị trí), mục 5.5 (năm map đã làm và id), mục 5.7 (điểm hồi sinh trong cùng map).
- Spec Giai đoạn 0 mục 1.1: ghi chú Giai đoạn 4 chia 4a/4b/4c.
- `CLAUDE.md`: bất biến "map sinh bằng `tools/mapgen.py`, sửa script chứ không sửa JSON"; mọi lần chuyển zone đi qua `World.Transfer` (giữ chỗ → zone rời → zone mới), trạng thái trao trong bộ nhớ, không đọc lại DB; khu ≤ `min_zones` không bao giờ đóng.
- README hai repo: năm map, cổng, cửa sổ khu, `devchar -map`.
