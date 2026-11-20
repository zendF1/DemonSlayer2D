# Giai đoạn 3c — Tương sinh, khảm ngọc và tẩy luyện

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng). Server đã triển khai (kế hoạch 3c, Task 1–10); client chờ triển khai.

Phần cuối của Giai đoạn 3 (xem bảng chia ở [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 3.2 (tương sinh) và 3.3 (khảm ngọc, tẩy luyện); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 3a và [spec 3b](2026-11-20-giai-doan-3b-cuong-hoa-design.md). 3c chạy sau khi 3b đã triển khai.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

- **Tương sinh:** dòng ẩn của đồ Vàng/Cam/Đỏ đã có từ 3a nay có tác dụng khi ô kích hoạt đang mặc món đúng hệ. Người chơi thấy dòng nào sáng, dòng nào còn tắt và cần gì để mở.
- **Khảm ngọc:** trong khu an toàn, đục tối đa ba lỗ trên một trang bị, khảm ngọc công, sinh lực hoặc hộ thể vào lỗ, tháo ngọc ra khi cần.
- **Tẩy luyện:** trong khu an toàn, làm mới loại và giá trị dòng phụ của đồ Vàng/Cam/Đỏ, khóa được dòng muốn giữ, xem kết quả rồi chọn giữ bộ cũ hoặc nhận bộ mới.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Điều kiện mở dòng ẩn | Món nhận và món ở ô kích hoạt đều đang mặc, đủ điều kiện mặc và có hệ; hệ của món ở ô kích hoạt là hệ sinh ra hệ của món nhận | Game-design 3.2; món không đủ điều kiện mặc thì không có tác dụng gì (3a) |
| Ai tính dòng sáng | Server tính chỉ số. Client tự tính dòng nào sáng từ bảng trong `items.json` để hiển thị | Bảng nhỏ, cố định; không phải gửi lại cả bộ đồ mỗi khi đổi một món |
| Loại ngọc | Ba loại: Ngọc Công (công vật lý và công phép), Ngọc Sinh Lực (HP), Ngọc Hộ Thể (thủ vật lý và thủ phép); mỗi loại 5 cấp | Xuyên giáp, kháng choáng chưa có công thức (game-design mục 10); thêm khi có |
| Giá trị ngọc | Ngọc cấp g bằng giá trị chuẩn S của bậc 10g (3a mục 2.1) | Một nguồn số liệu; ngọc cấp cao đáng giá ở đồ bậc cao |
| Ngọc vào món nào | Cấp ngọc ≤ bậc món / 10 | Không cho ngọc cấp 5 vào đồ bậc 10 ở cấp thấp |
| Đục lỗ | Mọi phẩm chất, tối đa 3 lỗ, đục lần lượt; tốn Xu 100/200/400 × bậc cho lỗ 1/2/3 | Game-design chỉ chốt "tối đa ba lỗ"; Xu là khoản tiêu ổn định |
| Khảm | Không tốn Xu, tốn một viên ngọc; lỗ phải trống | Đục lỗ đã là chi phí chính |
| Tháo ngọc | Trả ngọc về hành trang, tốn 500 × cấp ngọc Xu | Cho phép chuyển ngọc khi đổi đồ bậc cao hơn mà vẫn có khoản tiêu |
| Nguồn ngọc | Phần thưởng sự kiện, nhiệm vụ (Giai đoạn 6, 7+); tới lúc đó lấy bằng `devitem` | Game-design 6.3 đưa ngọc vào thưởng sự kiện; bảng rơi 5.12 không có ngọc |
| Tẩy luyện dùng cho | Vàng, Cam, Đỏ | Game-design 3.3 |
| Tẩy luyện đổi gì | Loại và giá trị các dòng phụ không khóa; số dòng giữ nguyên; không trùng loại với dòng khác của món | Game-design 3.3; giữ luật "không trùng loại" của 3a |
| Chi phí tẩy luyện | 1 đá tẩy luyện + 20 × bậc Xu; mỗi dòng khóa thêm 1 đá và 20 × bậc Xu; phải còn ít nhất một dòng không khóa | "Càng khóa nhiều càng tốn đá tẩy và Xu" (game-design 3.3) |
| Nguồn đá tẩy luyện | Tinh anh và thủ lĩnh (game-design 5.12); map hiện chưa có, tới lúc đó lấy bằng `devitem` | Không đổi bảng rơi đã chốt |
| Xem rồi chọn | Kết quả chờ giữ trong bộ nhớ zone, một kết quả mỗi người chơi; thoát game hoặc dùng lệnh đồ khác (trừ nhặt) thì mất, giữ dòng cũ | Đơn giản, không cần lưu; đá và Xu đã tiêu theo game-design |
| Nơi khảm và tẩy | Khu an toàn, như cường hóa (3b) | Cùng NPC thợ rèn ở Giai đoạn 4 |

### 1.3. Không làm ở 3c

Ngọc xuyên giáp, kháng choáng và các chỉ số chưa có công thức; ghép/nâng cấp ngọc; nguồn ngọc và đá tẩy thật (sự kiện, nhiệm vụ, tinh anh); NPC thợ rèn; kích hoạt theo bộ (game-design 3.2: bản đầu không có).

## 2. Dữ liệu

### 2.1. Thêm vào `data/items.json`

`item_stats.py` sinh thêm:

```json
{
  "resonance": {
    "activator": {"viem": "phong", "nham": "viem", "minh": "nham", "thuy": "minh", "phong": "thuy"},
    "slots": {
      "weapon": ["helmet", "armor"], "helmet": ["necklace", "ring_left"],
      "armor": ["necklace", "ring_right"], "pants": ["armor", "pendant"],
      "bracers": ["weapon", "ring_left"], "boots": ["pants", "pendant"],
      "necklace": ["weapon", "bracers"], "pendant": ["helmet", "boots"],
      "ring_left": ["pants", "ring_right"], "ring_right": ["boots", "bracers"]
    }
  },
  "sockets": {"max": 3, "punch_xu_per_tier": [100, 200, 400], "unsocket_xu_per_level": 500},
  "refine": {"min_quality": 2, "stones": 1, "stones_per_lock": 1, "xu_per_tier": 20, "xu_per_tier_per_lock": 20}
}
```

- `activator[e]` là hệ của món kích hoạt dòng ẩn của món hệ e, theo chu kỳ **Thủy → Phong → Viêm → Nham → Minh → Thủy** (game-design 3.2).
- `slots[S]` là hai ô kích hoạt dòng 1 và dòng 2 của món ở ô S (bảng game-design 3.2).
- `stackables` thêm 15 viên ngọc, sau bùa bảo vệ, mỗi viên có khối `gem`:

```json
{"code": "gem_attack_1", "name": "Ngọc Công cấp 1",
 "gem": {"kind": "attack", "level": 1, "stats": {"phys_attack": 4, "magic_attack": 4}}}
```

| Cấp ngọc | Đồ từ bậc | Ngọc Công (`gem_attack_g`): công vật lý và công phép | Ngọc Sinh Lực (`gem_hp_g`): HP | Ngọc Hộ Thể (`gem_defense_g`): thủ vật lý và thủ phép |
| --- | --- | --- | --- | --- |
| 1 | 10 | 4 | 86 | 1 |
| 2 | 20 | 8 | 156 | 1 |
| 3 | 30 | 14 | 220 | 2 |
| 4 | 40 | 22 | 320 | 3 |
| 5 | 50 | 32 | 469 | 5 |

Giá trị lấy đúng `line_standard` của bậc 10 × cấp ngọc; không nhân theo cường hóa (game-design 3.3).

**Server kiểm tra khi nạp:** `activator` đủ năm hệ, là một hoán vị tạo đúng một chu trình dài 5, không hệ nào tự kích hoạt mình; `slots` đủ 10 ô, mỗi ô hai ô khác nhau và khác chính nó; `sockets.max` = độ dài `punch_xu_per_tier`, các số dương; `unsocket_xu_per_level` > 0; `refine.min_quality` trong 1–4, các số còn lại ≥ 0, `stones` ≥ 1; mỗi `gem` có `kind` trong ba loại, `level` 1–5, `stats` chỉ gồm khóa dòng hợp lệ với giá trị dương, mỗi (loại, cấp) đúng một viên.

## 3. Lưu trữ

Migration `00006_sockets.sql`: `character_items.gems text[] NOT NULL DEFAULT '{}'`. Độ dài mảng là số lỗ đã đục; mỗi phần tử là mã ngọc hoặc `''` (lỗ trống). Khi vào game: mảng dài hơn `sockets.max` thì cắt bớt, mã ngọc lạ hoặc cấp quá bậc món thì thành lỗ trống; vật phẩm chồng có lỗ thì xóa lỗ; mỗi lần sửa ghi một dòng log cảnh báo.

## 4. Server

### 4.1. Tương sinh

Với món X đang mặc ở ô S, có hệ e và hai dòng ẩn: dòng j (j = 1, 2) sáng khi món Y đang mặc ở ô `slots[S][j]` tồn tại, cả X và Y đủ điều kiện mặc, và hệ của Y là `activator[e]`. Dòng sáng cộng vào tổng chỉ số trang bị (3a mục 4.2); dòng tắt không cộng gì. Tính lại mỗi lần `recompute`.

Ví dụ: kiếm hệ Viêm ở ô Vũ khí có ô kích hoạt Mũ và Áo; Mũ hệ Phong mở dòng 1, Áo hệ Phong mở dòng 2. Bộ hệ sau mở 13/20 dòng, mức tối đa có thể (game-design 3.2): Vũ khí Minh, Giày Thủy, Hộ uyển Phong, Quần Minh, Mũ Nham, Áo Nham, Dây chuyền Viêm, Ngọc bội Minh, Nhẫn trái Thủy, Nhẫn phải Viêm.

### 4.2. Đục lỗ

`PunchReq{item}`:

1. Chết: `DEAD`.
2. Ngoài khu an toàn: `NOT_IN_SAFE_ZONE`.
3. Ô ngoài phạm vi: `BAD_REQUEST`.
4. Ô rỗng: `ITEM_NOT_FOUND`.
5. Không phải trang bị: `BAD_REQUEST`.
6. Đã đủ `sockets.max` lỗ: `HOLES_MAXED`.
7. Thiếu Xu (`punch_xu_per_tier[số lỗ hiện có]` × bậc): `NOT_ENOUGH_XU`.

Hợp lệ: trừ Xu, thêm một lỗ trống, lưu ngay, trả `PunchResp{update, stats}`.

### 4.3. Khảm ngọc

`SocketReq{item, hole, gem_slot}` (`hole` 0-based, `gem_slot` là ô hành trang của ngọc):

1. `DEAD`. 2. `NOT_IN_SAFE_ZONE`.
3. Ô món hoặc `gem_slot` ngoài phạm vi: `BAD_REQUEST`.
4. Ô món rỗng hoặc ô ngọc rỗng: `ITEM_NOT_FOUND`.
5. Món không phải trang bị; vật ở `gem_slot` không phải ngọc; `hole` ngoài [0, số lỗ); lỗ đã có ngọc: `BAD_REQUEST`.
6. Cấp ngọc > bậc món / 10: `GEM_TOO_HIGH`.

Hợp lệ: bớt một viên ở `gem_slot`, đặt mã ngọc vào lỗ; nếu món đang mặc thì tính lại chỉ số và áp luật HP (3a mục 4.7); lưu; trả `SocketResp{update, stats}`.

### 4.4. Tháo ngọc

`UnsocketReq{item, hole}`:

1. `DEAD`. 2. `NOT_IN_SAFE_ZONE`.
3. Ô ngoài phạm vi: `BAD_REQUEST`.
4. Ô rỗng: `ITEM_NOT_FOUND`.
5. Món không phải trang bị, `hole` ngoài [0, số lỗ), hoặc lỗ trống: `BAD_REQUEST`.
6. Hành trang không nhận được viên ngọc (luật thêm của 3a mục 4.1): `BAG_FULL`.
7. Thiếu Xu (`unsocket_xu_per_level` × cấp ngọc): `NOT_ENOUGH_XU`.

Hợp lệ: trừ Xu, lỗ thành trống (vẫn giữ lỗ), ngọc vào hành trang; tính lại nếu đang mặc; lưu; trả `UnsocketResp{update, stats}`.

### 4.5. Tẩy luyện

`RefineReq{item, locked}` (`locked` là chỉ số 0-based của dòng phụ muốn giữ):

1. `DEAD`. 2. `NOT_IN_SAFE_ZONE`.
3. Ô ngoài phạm vi: `BAD_REQUEST`. 4. Ô rỗng: `ITEM_NOT_FOUND`. 5. Không phải trang bị: `BAD_REQUEST`.
6. Phẩm chất < `min_quality` (Trắng, Xanh): `NOT_REFINABLE`.
7. `locked` có chỉ số ngoài [0, số dòng), trùng nhau, hoặc khóa hết các dòng: `BAD_REQUEST`.
8. Thiếu đá tẩy (`stones` + `stones_per_lock` × số khóa): `NOT_ENOUGH_MATERIALS`.
9. Thiếu Xu ((`xu_per_tier` + `xu_per_tier_per_lock` × số khóa) × bậc): `NOT_ENOUGH_XU`.

Hợp lệ: trừ đá và Xu; tạo bộ dòng mới: dòng khóa giữ nguyên loại, giá trị và vị trí; mỗi dòng không khóa lấy loại ngẫu nhiên đều trong các loại chưa có trong bộ (không trùng dòng khóa và các dòng mới khác), giá trị như 3a mục 4.4. Lưu bộ mới làm **kết quả chờ** của người chơi (thay kết quả chờ cũ nếu có); dòng của món chưa đổi. Lưu ngay (vì đá và Xu đã trừ). Trả `RefineResp{lines, update, stats}` với `lines` là bộ mới đầy đủ theo thứ tự.

`RefineChooseReq{accept}`:

1. Không có kết quả chờ: `NO_PENDING_REFINE`.

Hợp lệ: `accept` thì dòng phụ của món thành bộ mới; nếu không thì giữ. Xóa kết quả chờ. Nếu món đang mặc và dòng đổi thì tính lại và áp luật HP. Lưu, trả `RefineChooseResp{update, stats}`. Lệnh này không cần khu an toàn và chạy được cả khi chết (chỉ là chọn kết quả đã trả tiền).

**Kết quả chờ mất** (dòng cũ giữ nguyên) khi người chơi rời zone, hoặc khi xử lý bất kỳ lệnh đồ nào khác ngoài `PickupReq` và `RefineChooseReq` (một `RefineReq` mới thì thay kết quả cũ). Vì vậy ô của món không thể đổi trong lúc chờ.

Hệ, phẩm chất, chỉ số cơ bản, mức cường hóa, ngọc và dòng ẩn không đổi (game-design 3.3).

## 5. Giao thức

`ProtocolVersion` lên **8**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 73 | `PunchReq` | `ItemRef item = 1` |
| 74 | `PunchResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 75 | `SocketReq` | `ItemRef item = 1`, `int32 hole = 2`, `int32 gem_slot = 3` |
| 76 | `SocketResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 77 | `UnsocketReq` | `ItemRef item = 1`, `int32 hole = 2` |
| 78 | `UnsocketResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 79 | `RefineReq` | `ItemRef item = 1`, `repeated int32 locked = 2` |
| 80 | `RefineResp` | `repeated ItemLine lines = 1`, `InventoryUpdate update = 2`, `SelfStats stats = 3` |
| 81 | `RefineChooseReq` | `bool accept = 1` |
| 82 | `RefineChooseResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |

- `ItemInfo.holes = 8` (`int32`), `ItemInfo.gems = 9` (`repeated string`, dài bằng `holes`, chuỗi rỗng là lỗ trống).
- `ErrorCode`: `HOLES_MAXED = 34`, `GEM_TOO_HIGH = 35`, `NOT_REFINABLE = 36`, `NO_PENDING_REFINE = 37`.
- Fixture mới: `item_with_gems.hex` (`EquipResp` có món 3 lỗ, 2 ngọc), `refine_resp.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`)

- **`core/resonance.gd`** (không dùng Node): `active(equipped, book, char_level, cls)` trả, cho mỗi ô có món, hai giá trị sáng/tắt theo mục 4.1; `need(slot, line, element)` trả ô và hệ cần để mở dòng `line` (1 hoặc 2) của món hệ `element` ở ô `slot`; `count(equipped, book, char_level, cls)` trả số dòng đang sáng.
- **`core/forge_rules.gd`** thêm: `punch_cost(item)`, `punch_block(...)`; `gems_for(item, inventory)` (ngọc trong hành trang cấp vừa món); `socket_block`, `unsocket_cost`, `unsocket_block`; `refine_cost(item, locks)`, `refine_block(...)` — cùng thứ tự kiểm tra với server.
- `core/item_book.gd`: tên và chỉ số ngọc. `core/inventory.gd`: `holes`, `gems` trong item dict.
- `core/errors.gd`: HOLES_MAXED "Đã đủ số lỗ", GEM_TOO_HIGH "Ngọc cấp cao hơn món đồ", NOT_REFINABLE "Chỉ tẩy luyện được đồ Vàng trở lên", NO_PENDING_REFINE "Không có kết quả tẩy luyện".

### 6.2. Giao diện

- **Ô thông tin món**: dòng ẩn đang sáng hiện màu của phẩm chất; dòng tắt hiện xám kèm "Cần <ô> hệ <hệ>"; dưới cùng là các lỗ: "◇ Trống" hoặc tên ngọc và chỉ số.
- **Tab Trang bị**: dòng "Dòng ẩn đang mở: n/20"; ô món hiện số lỗ đã khảm (chấm nhỏ).
- **Cửa sổ Rèn** thêm hai tab:
  - **Khảm ngọc**: danh sách lỗ của món; nút **Đục lỗ** kèm giá; lỗ trống có nút **Khảm** mở danh sách ngọc vừa cấp trong hành trang; lỗ có ngọc có nút **Tháo** kèm giá.
  - **Tẩy luyện**: các dòng phụ, mỗi dòng có ô khóa; chi phí theo số dòng khóa; nút **Tẩy luyện**. Có kết quả thì hiện hai cột "Hiện tại" và "Mới" với nút **Giữ cũ** và **Nhận mới**; đóng cửa sổ khi đang chờ thì gửi giữ cũ.
  - Ngoài khu an toàn: các nút khóa, ghi "Vào khu an toàn để rèn".
- **Hành trang**: ngọc hiện như vật phẩm chồng (màu theo loại).

### 6.3. Smoke

`tools/smoke.sh` thêm `devitem` cho tài khoản smoke: một Áo Lữ Hành Vàng, 2 đá tẩy luyện, một Ngọc Sinh Lực cấp 1, Xu đủ dùng. Smoke đục một lỗ, khảm ngọc, tẩy luyện một lần rồi nhận bộ mới; in "SMOKE OK" kèm ngọc trong lỗ và hai bộ dòng (cũ, mới). Không cần mặc áo.

## 7. Lệnh dev

`devitem` nhận mã ngọc (ví dụ `gem_hp_3`) và đá tẩy luyện như mọi vật phẩm chồng, thêm `-holes <0-3>` cho trang bị (lỗ trống).

## 8. Kiểm thử

**Server** (`go test -race`):

- `gamedata`: dữ liệu mới nạp được; từ chối `activator` có hệ tự kích hoạt hoặc hai chu trình, ô kích hoạt trùng chính nó, thiếu ô, `sockets.max` khác độ dài bảng giá, ngọc trùng (loại, cấp), ngọc có khóa chỉ số lạ.
- `items`: ví dụ kiếm Viêm với Mũ Phong mở dòng 1 và Áo Phong mở dòng 2; Mũ Phong không đủ cấp thì tắt; món Xanh không có dòng ẩn; duyệt toàn bộ 5^10 cách gán hệ cho 10 ô (mọi món có hệ): tối đa đúng 13 dòng, và bộ hệ ở mục 4.1 đạt 13 (chạy dưới 5 giây với `-race`; dùng mảng số nguyên, không dùng map); ngọc cộng chỉ số đúng bảng; cấp ngọc so với bậc; đục lỗ tới 3; tháo ngọc trả về hành trang và dồn chồng; tẩy luyện giữ dòng khóa đúng vị trí, dòng mới không trùng loại, số dòng không đổi, hệ/dòng ẩn/ngọc/cường hóa không đổi.
- `store`: lưu và nạp `gems`; mã ngọc lạ thành lỗ trống, mảng dài quá thì cắt, có ghi log.
- `zone`: từng mã lỗi theo thứ tự cho bốn lệnh; lỗi không tốn gì; khảm vào món đang mặc làm chỉ số tăng; mặc Mũ Phong làm dòng ẩn của kiếm Viêm có tác dụng và tháo ra thì mất; kết quả chờ: chọn nhận thì đổi dòng, chọn giữ thì không; `NO_PENDING_REFINE` khi chưa tẩy; một lệnh mặc đồ xóa kết quả chờ; nhặt đồ không xóa; rời zone xóa.
- `gateway`: `ProtocolVersion` 8; fixture mới.
- e2e `TestForgeOverWebSocket`: nhân vật được `DevSetLevel` lên 25; `devitem` cho Áo Thanh Thiết Vàng (cấp mặc 25), Ngọc Sinh Lực cấp 2, 2 đá tẩy, Xu; mặc áo, ghi HP tối đa, rồi trong khu an toàn đục lỗ và khảm vào áo đang mặc: HP tối đa tăng đúng 156; tẩy luyện khóa một dòng, nhận bộ mới, dòng khóa còn nguyên; thoát rồi vào lại, lỗ, ngọc và dòng còn nguyên.

**Client** (`tests/run.sh`): `resonance` (ví dụ mục 4.1, bộ 13 dòng, món không đủ cấp); `forge_rules` mới (chi phí, lý do khóa); giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** `test1` và `test2` được `devitem` cho đồ Xích Cương Vàng/Cam đủ năm hệ, ngọc và đá tẩy; thử phối hệ để mở dòng ẩn, khảm ngọc, tẩy luyện trong khu an toàn, thấy chỉ số đổi đúng.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 3.3: ba loại ngọc và bảng giá trị, cấp ngọc theo bậc, giá đục lỗ và tháo ngọc, giá tẩy luyện và khóa dòng, kết quả chờ mất khi rời game; mục 3.2: ví dụ bộ hệ đạt 13 dòng.
- `CLAUDE.md`: tương sinh, khảm, tẩy luyện nằm trong `internal/items`; client chỉ tính dòng sáng để hiển thị.
- README hai repo: các tab Khảm ngọc, Tẩy luyện; `devitem -holes`.
