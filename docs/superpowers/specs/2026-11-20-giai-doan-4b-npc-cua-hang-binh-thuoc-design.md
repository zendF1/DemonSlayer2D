# Giai đoạn 4b — NPC, cửa hàng, bình thuốc, giấy về thành và xa phu

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế; server đã triển khai (protocol 10), client chờ (nhiệm vụ 11–14).

Phần thứ hai của Giai đoạn 4 (xem bảng chia ở [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.7 (bình thuốc), 4.1 (NPC), 5.4 (địa điểm dịch vụ) và 7.1 (Xu); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 4a và các spec Giai đoạn 3. 4b chạy sau khi 4a đã triển khai.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Trong thành và trại mỗi map luyện cấp có NPC đứng sẵn, có tên và biểu tượng chức năng. Chạm vào NPC thì nhân vật đi tới và mở cửa sổ của NPC:

- **Tạp hóa / Dược sư**: mua bình HP/MP, giấy về thành, bùa bảo vệ bằng Xu; bán đồ lấy Xu.
- **Thợ rèn** (Tổng hành dinh): mở cửa sổ Rèn của Giai đoạn 3; cường hóa, khảm ngọc, tẩy luyện nay phải đứng gần Thợ rèn.
- **Xa phu**: trả Xu để tới trại của map khác ngay.

Hai nút bình trên HUD dùng được: bấm là uống bình tốt nhất đang có, hồi chiêu 10 giây. Giấy về thành đưa nhân vật về Tổng hành dinh.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| NPC là gì | Dữ liệu tĩnh trong file map (`npcs`); client vẽ từ file map đã đồng bộ; không phải thực thể, không có `EntityEnter` | NPC đứng yên, không có trạng thái; không tốn băng thông |
| Vai trò | Mỗi NPC một vai trò: `shop`, `blacksmith`, `carriage` (4c thêm `storage`) | Cửa sổ NPC đơn giản; một nơi đủ dịch vụ thì đặt nhiều NPC |
| Tầm NPC | 3 ô từ nhân vật tới NPC, server cộng 0,5 ô dung sai; client đi tới cách 2,5 ô | Ngang tầm chọn mục tiêu, đủ rộng cho điện thoại |
| Bố trí | Tổng hành dinh: Tạp hóa, Thợ rèn, Xa phu. Trại mỗi map luyện cấp (vai Nhà Hoa Tử Đằng): Dược sư, Xa phu | Game-design 5.4; Làng Thợ Rèn và Điệp Phủ làm sau |
| Nơi rèn đồ | Đứng gần NPC Thợ rèn, thay luật "trong khu an toàn" của 3b và 3c | Spec 3b đã hẹn; game-design 4.1 |
| Cửa hàng | Hai cửa hàng Xu khai báo trong `items.json`: `general` (Tạp hóa) và `apothecary` (Dược sư); chỉ bán vật phẩm chồng | Một nguồn dữ liệu với vật phẩm, kiểm tra chéo khi nạp |
| Hàng và giá | Bình theo đúng bảng game-design 2.7; giấy về thành 50 Xu; bùa bảo vệ 5.000 Xu chỉ ở Tạp hóa | 3b hẹn bán bùa ở tạp hóa; với bùa, lên +15 bậc 50 tốn thêm khoảng 150.000 Xu, vẫn thấp hơn nhiều so với 36 triệu Xu không bùa |
| Đá cường hóa, đá tẩy luyện | Không bán bằng Xu | Giữ giá trị của rơi đồ (game-design 5.12) |
| Số lượng mua | 1 tới `stack_max` mỗi lệnh; đủ chỗ và đủ Xu cho cả lô, không thì không mua gì | Như luật nhặt của 3a |
| Bán đồ | Mọi NPC `shop` đều mua. Trang bị: bậc × hệ số phẩm chất 5/10/20/40/80 (Trắng … Đỏ). Vật phẩm chồng: giá `sell` mỗi viên; 0 là không bán được. Mức cường hóa, ngọc, dòng không cộng giá; client hỏi lại với món quý | Đơn giản; đồ rơi tạo một khoản Xu nhỏ, không thay Xu từ quái (bậc 50 Vàng = 1.000 Xu, khoảng 5 con cấp 50) |
| Bán đồ đang mặc | Không; tháo ra trước | Tránh bán nhầm |
| Dùng bình | `UseItemReq` theo ô hành trang; cấp nhân vật ≥ cấp bình; hồi chiêu 10 giây chung cho mọi bình HP, riêng cho bình MP; HP (hoặc MP) đã đầy thì từ chối | Game-design 2.7; từ chối khi đầy để không phí bình |
| Nút bình trên HUD | Client chọn bình cùng loại có cấp cao nhất mà nhân vật dùng được, cùng cấp thì ô nhỏ nhất; huy hiệu là tổng số bình loại đó dùng được | Bấm một lần trên điện thoại |
| Lưu khi dùng bình | Không lưu ngay; theo nhịp lưu định kỳ và các lần lưu khác | Bình dùng mỗi 10 giây; sập server chỉ làm người chơi được lại bình |
| Giấy về thành | Vật phẩm chồng; dùng ở map không phải thành; tới điểm `default` của map `town` khai báo trong map (Tổng hành dinh); dùng được khi đang giao chiến; không hồi chiêu | Game-design 4.1; đổi map khi giao chiến được phép (5.1) |
| Xa phu | Tới điểm `default` (trại) của map đích; giá 50 Xu tới thành, 20 × cấp quái thấp nhất của map đích (tối thiểu 50) tới map luyện cấp | "Di chuyển giữa các bản đồ với phí" (game-design 4.1); giá nhỏ so với Xu cùng cấp |
| Khu khi về thành hay đi xe | Như qua cổng (4a): cùng số khu nếu mở và còn chỗ | Một luật |
| Hồi chiêu bình khi chuyển zone | Giữ như các hồi chiêu khác (4a mục 3.2.1) | Không đổi khu để uống liền |
| Kết quả tẩy luyện đang chờ (3c) | Bán đồ xóa kết quả chờ (lệnh đồ có thể bỏ món đó đi); mua và dùng bình không xóa (chỉ đụng vật phẩm chồng, không đụng ô trang bị) | Giữ mục đích của luật 3c mục 4.5: ô của món không thể đổi |
| Hồi phục tại NPC | Giữ lửa trại hiện có làm "NPC nghỉ ngơi" của game-design 5.4 | Đã có từ Giai đoạn 1 |
| Hội thoại NPC | Không có; cửa sổ NPC chỉ có các nút dịch vụ | NPC nhiệm vụ thuộc Giai đoạn 6 |

### 1.3. Không làm ở 4b

Kho, Lượng, Cửa hàng Lượng, hồi sinh tại chỗ (4c); NPC nhiệm vụ, hướng dẫn, bang hội, sự kiện, thương nhân theo giờ (Giai đoạn 6 và 7+); tự mua bình và tự dùng bình khi auto (Giai đoạn 6); sạp và giao dịch (Giai đoạn 7); các loại bùa khác; bán Lượng; Làng Thợ Rèn, Điệp Phủ; hình NPC thật.

## 2. Dữ liệu

### 2.1. NPC và thành trong file map

`maps/<id>.json` thêm hai trường bắt buộc, do `tools/mapgen.py` sinh:

```json
{
  "town": "ubuyashiki",
  "npcs": [
    {"id": "duoc_su", "name": "Dược sư", "role": "shop", "shop": "apothecary", "x": 8.5, "y": 3.5},
    {"id": "xa_phu", "name": "Xa phu", "role": "carriage", "x": 8.5, "y": 7.5,
     "routes": [{"map": "ubuyashiki", "xu": 50}, {"map": "sagiri_1", "xu": 120}]}
  ]
}
```

- `town`: map mà giấy về thành đưa tới; với map thành là chính nó.
- `npcs[]`: `id`, `name`, `role`, vị trí; `shop` chỉ có ở vai `shop`; `routes` chỉ có ở vai `carriage` (mỗi tuyến: map đích và giá Xu; điểm tới luôn là `default` của map đích).

**Kiểm tra mỗi map** (`mapdata.Parse`): `npcs` có id khác rỗng, không trùng; `name` khác rỗng; `role` thuộc `shop`, `blacksmith`, `carriage`; `shop` khác rỗng khi và chỉ khi vai `shop`; `routes` không rỗng khi và chỉ khi vai `carriage`, mỗi tuyến có `xu` ≥ 1, không trùng map, không trỏ chính map này; vị trí đi được, nằm trong khu an toàn, tới được từ `default` (BFS của 4a); hai NPC cách nhau ít nhất 1,5 ô.

**Kiểm tra giữa các map** (`mapdata.Link`): `town` là một map có thật loại `town`; map thành có `town` là chính nó; mỗi tuyến xa phu trỏ tới map có thật.

**Kiểm tra chéo với vật phẩm** (`gamedata.Validate(maps)`): `shop` của mỗi NPC là một cửa hàng có thật trong `items.json`.

### 2.2. NPC của năm map

| Map | NPC | Vai trò | Ghi chú |
| --- | --- | --- | --- |
| `ubuyashiki` | `tap_hoa` "Tạp hóa" | `shop` → `general` | gần `default` |
| `ubuyashiki` | `tho_ren` "Thợ rèn" | `blacksmith` | |
| `ubuyashiki` | `xa_phu` "Xa phu" | `carriage` | bốn map luyện cấp |
| mỗi map luyện cấp | `duoc_su` "Dược sư" | `shop` → `apothecary` | trong trại |
| mỗi map luyện cấp | `xa_phu` "Xa phu" | `carriage` | Tổng hành dinh và ba map luyện cấp còn lại |

Giá xa phu (script tính): tới `ubuyashiki` 50 Xu; tới `kumotori_1` 50, `sagiri_1` 120, `sagiri_2` 220, `fujikasane_1` 320 (20 × cấp quái thấp nhất của map đích, tối thiểu 50). Mọi map có `town: "ubuyashiki"`.

Trong Kumotori, trại là (1, 1, 10, 10), lửa trại ở (3,5; 3,5), `default` ở (5,5; 5,5): Dược sư ở (8,5; 3,5), Xa phu ở (8,5; 7,5). Ở các map khác script đặt NPC trong trại hoặc quanh `default` của thành, cách `default` 2–4 ô.

### 2.3. Thêm vào `data/items.json`

`item_stats.py` sinh thêm (không sửa tay):

```json
{
  "use_cooldown_sec": {"hp": 10, "mp": 10},
  "sell_equipment_xu_per_tier": [5, 10, 20, 40, 80],
  "stackables": [
    {"code": "stone_enhance", "name": "Đá cường hóa", "sell": 5},
    {"code": "potion_hp_1", "name": "Bình HP cấp 1", "sell": 1,
     "use": {"effect": "hp_pct", "value": 10, "level": 1, "group": "hp"}},
    {"code": "potion_mp_10", "name": "Bình MP cấp 10", "sell": 5,
     "use": {"effect": "mp", "value": 100, "level": 10, "group": "mp"}},
    {"code": "scroll_return", "name": "Giấy về thành", "sell": 10,
     "use": {"effect": "return_town", "value": 0, "level": 1, "group": ""}}
  ],
  "shops": [
    {"id": "general", "name": "Tạp hóa", "currency": "xu",
     "goods": [{"code": "potion_hp_1", "price": 5}, {"code": "charm_protect", "price": 5000}]}
  ]
}
```

(Ví dụ rút gọn.)

**Vật phẩm chồng mới**, thêm vào cuối `stackables` theo thứ tự (thứ tự này cũng là thứ tự sắp xếp của 3a):

| Mã | Tên | Cấp dùng | Hiệu ứng | Nhóm hồi | Giá mua | Giá bán |
| --- | --- | --- | --- | --- | --- | --- |
| `potion_hp_1` | Bình HP cấp 1 | 1 | 10% HP tối đa | hp | 5 | 1 |
| `potion_hp_10` | Bình HP cấp 10 | 10 | 15% HP tối đa | hp | 30 | 6 |
| `potion_hp_30` | Bình HP cấp 30 | 30 | 20% HP tối đa | hp | 100 | 20 |
| `potion_hp_50` | Bình HP cấp 50 | 50 | 25% HP tối đa | hp | 200 | 40 |
| `potion_mp_10` | Bình MP cấp 10 | 10 | 100 MP | mp | 25 | 5 |
| `potion_mp_30` | Bình MP cấp 30 | 30 | 250 MP | mp | 80 | 16 |
| `potion_mp_50` | Bình MP cấp 50 | 50 | 400 MP | mp | 160 | 32 |
| `scroll_return` | Giấy về thành | 1 | về thành | — | 50 | 10 |

Giá bán của bình và giấy = ⌊giá mua / 5⌋. Không có bình MP cấp 1 (game-design 2.7).

**Giá bán của vật phẩm chồng đã có** (trường `sell` mới, bắt buộc với mọi vật phẩm chồng): `stone_enhance` 5, `stone_refine` 50, `charm_protect` 1.000, ngọc cấp g 200 × g.

**Cửa hàng:**

| id | Tên | Hàng |
| --- | --- | --- |
| `general` | Tạp hóa | bảy loại bình, `scroll_return`, `charm_protect` |
| `apothecary` | Dược sư | bảy loại bình, `scroll_return` |

Giá mua lấy từ bảng trên; `charm_protect` 5.000.

**Server kiểm tra khi nạp:** mọi vật phẩm chồng có `sell` ≥ 0; `use` (nếu có) có `effect` thuộc `hp_pct`, `mp`, `return_town`; `level` trong 1–90; `hp_pct` có `value` trong 1–100, `mp` có `value` ≥ 1, `return_town` có `value` 0; `group` là một khóa của `use_cooldown_sec` với `hp_pct` và `mp`, rỗng với `return_town`; `use_cooldown_sec` có giá trị dương; `sell_equipment_xu_per_tier` đúng 5 số không âm; `shops` có id không trùng, tên khác rỗng, `currency` là `xu` (4c thêm `luong`), `goods` không rỗng, mỗi mã là vật phẩm chồng có thật, không trùng trong một cửa hàng, `price` ≥ 1.

## 3. Lưu trữ

Không có migration. Bình và giấy là vật phẩm chồng trong `character_items` như đá. Hồi chiêu bình không lưu.

## 4. Server

### 4.1. NPC trong zone

`zone.Area` nhận danh sách NPC của map (id, vai trò, vị trí, cửa hàng, tuyến). Một nhân vật **gần** NPC khi khoảng cách tới NPC ≤ 3 + 0,5 ô. Lỗi xa NPC là `NPC_TOO_FAR`.

### 4.2. Mua

`BuyReq{npc_id, code, count}`. Kiểm tra theo thứ tự, lỗi đầu tiên được trả, mọi lỗi không tốn gì:

1. Nhân vật chết: `DEAD`.
2. Map không có NPC đó, hoặc NPC không phải vai `shop`: `BAD_REQUEST`.
3. Không gần NPC: `NPC_TOO_FAR`.
4. Cửa hàng của NPC không bán `code`: `BAD_REQUEST`.
5. `count` < 1 hoặc > `stack_max`: `BAD_REQUEST`.
6. Xu < giá × `count`: `NOT_ENOUGH_XU`.
7. Hành trang không chứa hết (3a mục 4.1): `BAG_FULL`.

Hợp lệ: trừ Xu, thêm vào hành trang, lưu ngay, trả `BuyResp{update, stats}`.

### 4.3. Bán

`SellReq{npc_id, bag_slot, count}` (`count` 0 là cả ô):

1. `DEAD`.
2. NPC không có hoặc không phải vai `shop`: `BAD_REQUEST`.
3. `NPC_TOO_FAR`.
4. `bag_slot` ngoài phạm vi: `BAD_REQUEST`.
5. Ô rỗng: `ITEM_NOT_FOUND`.
6. `count` < 0 hoặc lớn hơn số lượng: `BAD_REQUEST`.
7. Giá bán bằng 0: `NOT_SELLABLE`.

Giá bán một đơn vị: trang bị = bậc × `sell_equipment_xu_per_tier[phẩm chất]`; vật phẩm chồng = `sell`. Hợp lệ: bớt số lượng (về 0 thì ô rỗng), cộng Xu = đơn giá × số lượng, xóa kết quả tẩy luyện đang chờ (3c), lưu ngay, trả `SellResp{update, stats}`.

### 4.4. Dùng vật phẩm

`UseItemReq{bag_slot}`:

1. `DEAD`.
2. `bag_slot` ngoài phạm vi: `BAD_REQUEST`.
3. Ô rỗng: `ITEM_NOT_FOUND`.
4. Món không có `use`, hoặc là giấy về thành (dùng `ReturnTownReq`): `BAD_REQUEST`.
5. Cấp nhân vật < `use.level`: `LEVEL_TOO_LOW`.
6. Nhóm hồi của món chưa hết hồi: `ON_COOLDOWN`.
7. `hp_pct` mà HP đã đầy, hoặc `mp` mà MP đã đầy: `ALREADY_FULL`.

Hợp lệ:

- `hp_pct`: HP tăng max(1, ⌊HP tối đa × `value` / 100⌋), không vượt HP tối đa; `hpChanged` để người xung quanh thấy.
- `mp`: MP tăng `value`, không vượt MP tối đa.
- Bớt một món ở ô đó; nhóm hồi sẵn sàng lại sau `z.ticks(use_cooldown_sec[group])`; `statsDirty`.
- Không lưu ngay (mục 1.2). Trả `UseItemResp{update, stats, cooldown_ms}` với `cooldown_ms` = số giây hồi × 1000.

Hồi chiêu bình đi theo bản trao tay của 4a với khóa `potion_hp` và `potion_mp`.

### 4.5. Giấy về thành

`ReturnTownReq{bag_slot}`, qua `World.Transfer` (4a mục 3.2):

1. Map hiện tại là `town`: `BAD_REQUEST` (world).
2. `DEAD` (zone, các bước sau cũng vậy).
3. `bag_slot` ngoài phạm vi: `BAD_REQUEST`.
4. Ô rỗng: `ITEM_NOT_FOUND`.
5. Món không có `use.effect` là `return_town`: `BAD_REQUEST`.
6. Cấp nhân vật < `use.level`: `LEVEL_TOO_LOW`.

Hợp lệ: bớt một giấy (trong bản trao tay và trong lần lưu khi rời), chuyển tới điểm `default` của map `town`. Trả lời là `EnterWorldResp`.

### 4.6. Xa phu

`TravelReq{npc_id, route}` (`route` là chỉ số 0-based trong `routes`), qua `World.Transfer`:

1. NPC không có hoặc không phải vai `carriage`; `route` ngoài phạm vi: `BAD_REQUEST` (world).
2. `DEAD` (zone).
3. `NPC_TOO_FAR`.
4. Xu < giá tuyến: `NOT_ENOUGH_XU`.

Hợp lệ: trừ Xu, chuyển tới điểm `default` của map đích. Trả lời là `EnterWorldResp`.

### 4.7. Rèn đồ tại Thợ rèn

Trong `EnhanceReq` (3b), `PunchReq`, `SocketReq`, `UnsocketReq`, `RefineReq` (3c), bước 2 "không đứng trong khu an toàn: `NOT_IN_SAFE_ZONE`" đổi thành "không gần NPC `blacksmith` nào của map: `NPC_TOO_FAR`". Các lệnh không mang id NPC; server xét mọi Thợ rèn của map. `RefineChooseReq` không đổi. Mã `NOT_IN_SAFE_ZONE` giữ trong enum nhưng không còn dùng.

## 5. Giao thức

`ProtocolVersion` lên **10**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 87 | `BuyReq` | `string npc_id = 1`, `string code = 2`, `int32 count = 3` |
| 88 | `BuyResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 89 | `SellReq` | `string npc_id = 1`, `int32 bag_slot = 2`, `int32 count = 3` |
| 90 | `SellResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 91 | `UseItemReq` | `int32 bag_slot = 1` |
| 92 | `UseItemResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2`, `int32 cooldown_ms = 3` |
| 93 | `ReturnTownReq` | `int32 bag_slot = 1` |
| 94 | `TravelReq` | `string npc_id = 1`, `int32 route = 2` |

- Trả lời thành công của `ReturnTownReq` và `TravelReq` là `EnterWorldResp`.
- `Cooldown.key` thêm `potion_hp`, `potion_mp`; ghi chú cập nhật.
- `ErrorCode`: `NPC_TOO_FAR = 39`, `NOT_SELLABLE = 40`, `ALREADY_FULL = 41`; ghi chú của `NOT_IN_SAFE_ZONE` thành "unused since protocol 10".
- Fixture mới: `buy_req.hex`, `use_item_resp.hex`, `travel_req.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/map_grid.gd`**: đọc `npcs` (`{id, name, role, shop, routes, pos}`) và `town`; `npc(id)`, `npc_at(pos, radius)`.
- **`core/item_book.gd`**: `use_of(code) -> Dictionary`; `use_cooldown(group) -> float`; `sell_value(item, count) -> int` (cùng công thức với server); `shop(id) -> Dictionary`.
- **`core/shop_rules.gd`**: `buy_block(shop, code, count, xu, inventory, book) -> String` trả `""`, `"count"`, `"xu"` hoặc `"bag"`; `sell_block(item, book) -> String` trả `""` hoặc `"unsellable"`; `sell_warns(item) -> bool` (Vàng trở lên, có cường hóa hoặc có ngọc).
- **`core/potion_rules.gd`**: `best_slot(inventory, book, group, level) -> int` (−1 khi không có); `count(inventory, book, group, level) -> int`; `start(group, now)`, `ready_in(group, now)`, `set_ready_in(group, sec, now)`; `block(group, level, hp, max_hp, mp, max_mp, inventory, book, now) -> String` trả `""`, `"none"`, `"cooldown"` hoặc `"full"`.
- **`core/npc_loop.gd`**: như `pickup_loop`: nhận id NPC, đi thẳng tới khi cách NPC ≤ 2,5 ô thì báo "mở"; tự di chuyển hoặc chọn mục tiêu khác thì hủy; `NPC_TOO_FAR` từ server thì đi thêm rồi mở lại.
- **`core/forge_rules.gd`**: tham số `in_safe` đổi thành `near_smith`, lý do khóa `"safe"` đổi thành `"smith"`.
- **`core/errors.gd`**: NPC_TOO_FAR "Hãy đến gần NPC hơn", NOT_SELLABLE "Không bán được vật phẩm này", ALREADY_FULL "Đã đầy, không cần dùng". **`core/wire.gd`**: payload mới.

### 6.2. Giao diện

- **NPC trên map** (`ui/map_view.gd`): vòng tròn màu theo vai trò (Tạp hóa/Dược sư xanh lá, Thợ rèn cam, Xa phu nâu) có chữ biểu tượng (B, R, X) và tên phía trên. Chạm trong 0,6 ô quanh NPC thì chạy `npc_loop` (ưu tiên trước đồ dưới đất và quái).
- **Cửa sổ NPC** (`ui/npc_window.gd`): tên NPC; theo vai trò: nút **Mua bán** (mở cửa hàng), **Rèn đồ** (mở cửa sổ Rèn của 3b/3c), hoặc danh sách tuyến xa phu "Sagiri – Chân núi · 120 Xu" với nút **Đi** (khóa khi thiếu Xu). Tự đóng khi nhân vật cách NPC quá 4 ô.
- **Cửa hàng** (`ui/shop_window.gd`): đầu cửa sổ ghi Xu đang có. Tab **Mua**: danh sách hàng (tên, cấp dùng, giá, số đang có), chọn số lượng 1/10/100 hoặc nhập, nút **Mua** ghi tổng giá, khóa và ghi lý do theo `buy_block`. Tab **Bán**: lưới hành trang; chạm món mở ô thông tin món có giá bán và chọn số lượng; món `sell_warns` hỏi lại "Bán <tên> lấy N Xu? Cường hóa, ngọc và dòng sẽ mất."
- **Nút bình trên HUD**: bỏ "Có ở bản sau"; huy hiệu là `count`; bấm thì gửi `UseItemReq` cho `best_slot`, khóa thì báo ("Không có bình", "Bình đang hồi", "Đã đầy"); vòng quét hồi 10 giây sau khi dùng.
- **Ô thông tin món**: vật phẩm có `use` có nút **Dùng**; giấy về thành gửi `ReturnTownReq` theo luồng chuyển zone của 4a (khóa và ghi "Đang ở trong thành" ở map thành). Nút **Cường hóa** chỉ mở cửa sổ Rèn khi gần Thợ rèn; nếu không thì báo "Đến gặp Thợ rèn ở Tổng hành dinh".
- **Cửa sổ Rèn**: nút khóa ngoài tầm Thợ rèn ghi "Đến gần Thợ rèn để rèn"; tự đóng khi rời tầm.
- `screens/world.gd` nối các yêu cầu, phản hồi và lỗi; nạp `potion_hp`, `potion_mp` từ `EnterWorldResp.cooldowns`.

### 6.3. Smoke

`tools/smoke.sh` sau bước 4a: `devchar -map kumotori_1 -x 7.5 -y 3.5` (cạnh Dược sư) và `devitem -xu 1000`; vào lại; mua 3 Bình HP cấp 1 và 1 Giấy về thành; bán 1 bình; dùng 1 bình nếu HP chưa đầy (nếu đầy thì kiểm tra bị `ALREADY_FULL`); dùng giấy về thành và kiểm tra map mới là `ubuyashiki`. In "SMOKE OK" kèm Xu trước và sau.

## 7. Lệnh dev

Không có lệnh mới; `devitem` nhận mã bình và giấy như mọi vật phẩm chồng.

## 8. Kiểm thử

**Script:** `item_stats.py` kiểm tra bảng mục 2.3 (giá bình đúng game-design 2.7, giá bán = ⌊giá / 5⌋, mọi mã trong cửa hàng là vật phẩm chồng); `mapgen.py` kiểm tra NPC trong khu an toàn, cách nhau ≥ 1,5 ô, giá xa phu theo công thức.

**Server** (`go test -race`):

- `gamedata`: dữ liệu mới nạp được (26 vật phẩm chồng: 2 đá, bùa, 15 ngọc, 7 bình, giấy; 2 cửa hàng); từ chối: thiếu `sell`, `sell` âm, `effect` lạ, `hp_pct` 0 hoặc 101, `group` không có trong `use_cooldown_sec`, `return_town` có nhóm, cửa hàng bán trang bị, mã lạ, trùng mã trong cửa hàng, giá 0, `currency` lạ; `Validate` từ chối NPC trỏ cửa hàng không có.
- `mapdata`: từ chối NPC ngoài khu an toàn, trên ô tường, trùng id, vai trò lạ, `shop` ở NPC không phải `shop`, xa phu không có tuyến, tuyến trỏ chính map hay map không có, giá 0, hai NPC cách 1 ô, `town` trỏ map luyện cấp; năm map thật có đúng NPC ở mục 2.2.
- `zone` mua/bán: từng mã lỗi theo thứ tự; mua 10 Bình HP cấp 1 trừ 50 Xu và dồn vào chồng cũ; hành trang đầy thì không trừ Xu; bán Áo Xích Cương Vàng được 1.000 Xu; bán `stone_enhance` 7 viên được 35 Xu; bán đồ xóa kết quả tẩy luyện chờ, mua thì không; mỗi lệnh thành công xếp một lần lưu.
- `zone` dùng bình: từng mã lỗi theo thứ tự; HP 500/1.640 dùng Bình HP cấp 1 lên 664; HP 1.600/1.640 lên 1.640; MP cộng 100 không vượt tối đa; hồi 10 giây chung cho mọi bình HP (dùng cấp 1 xong thì cấp 10 cũng `ON_COOLDOWN`), bình MP độc lập; `hpChanged` cho người xung quanh; không xếp lần lưu; hồi bình đi qua bản trao tay.
- `zone` rèn đồ: `EnhanceReq` cách Thợ rèn 3,4 ô được, 3,6 ô bị `NPC_TOO_FAR`; ở map không có Thợ rèn luôn bị `NPC_TOO_FAR`; thứ tự `DEAD` trước `NPC_TOO_FAR` trước lỗi ô.
- `world`: giấy về thành ở thành bị `BAD_REQUEST`; ở Sagiri thì tới `default` của Ubuyashiki và mất một giấy; xa phu tuyến ngoài phạm vi `BAD_REQUEST`, thiếu Xu `NOT_ENOUGH_XU` (không đổi zone, không mất Xu), đủ Xu thì tới trại map đích và mất đúng giá.
- `gateway`: `ProtocolVersion` 10; fixture mới.
- e2e `TestShopsOverWebSocket`: `devitem -xu 10000`, `DevSetPlace` cạnh Dược sư Kumotori; mua 3 Bình HP cấp 1 (Xu −15); dùng một bình khi đầy HP bị `ALREADY_FULL`; bán một bình (Xu +1); mua giấy về thành, dùng, tới `ubuyashiki`; thoát, `DevSetPlace` cạnh Xa phu Ubuyashiki, vào lại, đi xe tới `sagiri_1` (Xu −120) và ở điểm `default` của Sagiri.
- e2e của 3b và 3c (`TestEnhanceOverWebSocket`, `TestForgeOverWebSocket`) đặt nhân vật cạnh Thợ rèn Ubuyashiki bằng `DevSetPlace`; bước "ra khỏi khu an toàn bị `NOT_IN_SAFE_ZONE`" thành "đi xa Thợ rèn bị `NPC_TOO_FAR`".

**Client** (`tests/run.sh`): `map_grid` (NPC, `town`); `item_book` (`use_of`, `sell_value` khớp ví dụ server); `shop_rules`; `potion_rules` (chọn bình cấp cao nhất dùng được, bỏ bình quá cấp, đếm, hồi chiêu, đầy); `npc_loop` (đi tới rồi mở một lần, hủy khi tự đi); `forge_rules` với `near_smith`; giải mã fixture mới. Smoke theo mục 6.3; smoke 3b/3c đặt nhân vật cạnh Thợ rèn bằng `devchar -map ubuyashiki -x … -y …` (lấy tọa độ từ file map).

**Nghiệm thu:** `test1` và `test2` mua bình ở trại Kumotori, đánh quái và uống bình bằng nút HUD; bán đồ Lữ Hành nhặt được; đi xe tới Sagiri; dùng giấy về thành; cường hóa ở Thợ rèn Ubuyashiki và thấy nút Rèn khóa khi đi xa.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 2.7 (bình chọn tốt nhất trên HUD, từ chối khi đầy), mục 4.1 (bố trí NPC ở mục 2.2, giá xa phu, giấy về thành), mục 3.3 (rèn đồ tại Thợ rèn; bùa bảo vệ 5.000 Xu ở Tạp hóa), mục 7.1–7.2 (giá bán đồ cho NPC).
- Spec 3b mục 1.2 và spec 3c mục 1.2: ghi chú "nơi rèn đổi thành gần Thợ rèn ở spec 4b".
- `CLAUDE.md`: NPC là dữ liệu map, không phải thực thể; cửa hàng và giá trong `items.json` (sinh bằng `item_stats.py`); rèn đồ cần đứng gần Thợ rèn.
- README hai repo: NPC, cửa hàng, nút bình, giấy về thành, xa phu.
