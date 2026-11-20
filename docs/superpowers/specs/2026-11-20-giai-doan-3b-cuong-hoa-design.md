# Giai đoạn 3b — Cường hóa và hồi theo mốc

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng). Server đã triển khai (kế hoạch 3b, Task 1–9); client chờ triển khai.

Phần thứ hai của Giai đoạn 3 (xem bảng chia ở [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.7 (hồi theo mốc cường hóa) và 3.3 (cường hóa, tỷ lệ ẩn, vũ khí phát sáng); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 3a. 3b chạy sau khi 3a đã triển khai.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Đứng trong khu an toàn, người chơi chọn một trang bị (đang mặc hoặc trong hành trang) và cường hóa từ +0 lên tối đa +15, tốn đá cường hóa và Xu; từ +8 có thể dùng bùa bảo vệ chống tụt cấp. Chỉ số món tăng theo bảng đã duyệt. Món đạt +7/+10/+12/+15 cho hồi HP (món giáp) hoặc MP (vũ khí, trang sức) mỗi giây. Vũ khí đang mặc ở +7 trở lên phát sáng, người xung quanh cũng thấy.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Nơi cường hóa | Bất cứ đâu trong khu an toàn. Giai đoạn 4 đổi thành đứng gần NPC thợ rèn | Chưa có NPC; khu an toàn là chỗ gần đúng nhất với Làng Thợ Rèn/Tổng hành dinh (game-design 5.4) |
| Món được cường hóa | Mọi trang bị, đang mặc hoặc trong hành trang; vật phẩm chồng thì không | Không bắt tháo đồ ra |
| Tỷ lệ và tụt cấp | Đúng bảng game-design 3.3 | Đã chốt |
| Đá và Xu mỗi lần thử | Bảng mục 2.1: đá 1/2/3/4/5 theo nhóm cấp đích; Xu = hệ số cấp đích × bậc món | Game-design chỉ ghi "tiêu hao đá và Xu"; bộ số ban đầu, cân chỉnh sau chơi thử |
| Bùa bảo vệ | Vật phẩm chồng `charm_protect`, một lá mỗi lần thử, chỉ dùng được khi cấp đích từ +8 (nơi thất bại có thể tụt cấp); dưới +8 server không lấy bùa | Bùa chỉ chống tụt cấp, không tăng tỷ lệ (game-design 3.3) |
| Nguồn bùa | Tạp hóa bán ở Giai đoạn 4; tới lúc đó lấy bằng `devitem` | Game-design 4.1: tạp hóa bán "bùa hỗ trợ" |
| Tỷ lệ ẩn | +1 điểm % mỗi lần thất bại, riêng từng món và từng cấp đích; giữ khi tụt cấp; về 0 khi lên thành công cấp đích đó; không gửi cho client | Game-design 3.3 |
| Cường hóa liên tục | `EnhanceReq.times` (1–100): server thử liên tiếp tới khi thành công, tụt cấp, hết đá/bùa/Xu hoặc đủ số lần | Lên +15 không bùa trung bình khoảng 6.500 lần thử; bấm từng lần là quá nặng tay. Mỗi lần thử vẫn độc lập và tốn đủ |
| Lưu | Lưu ngay sau mỗi `EnhanceReq` | Kết quả quý; khớp luật lưu của 3a |
| Hồi theo mốc | Đúng bảng game-design 2.7; mỗi giây một lần cho cả zone; HP dùng bộ cộng dồn phần lẻ nên món +7 ở cấp thấp vẫn có hồi | Nếu làm tròn xuống mỗi giây thì 0,05% của 1.640 HP là 0 |
| Món không đủ điều kiện mặc | Không cho hồi, không phát sáng | Giống luật chỉ số của 3a |
| Phát sáng | Server gửi mức cường hóa của vũ khí đang mặc trong `EntityEnter` và khi đổi; client tự đổi ra màu Xanh/Vàng/Tím/Đỏ theo +7/+10/+12/+15 | Người xung quanh thấy; một số nguyên đủ cho mọi hiệu ứng sau này |
| Hiển thị tỷ lệ | Client hiện tỷ lệ gốc theo bảng | Game-design 3.3 |
| Chống spam | Chỉ giới hạn gói chung | Một lệnh đã gồm tới 100 lần thử |

### 1.3. Không làm ở 3b

NPC thợ rèn, bán bùa và đá (Giai đoạn 4); tương sinh, khảm ngọc, tẩy luyện (3c); hình vũ khí thật trên nhân vật (chỉ có vầng sáng vẽ tạm); hồi theo mốc của thú cưỡi, thời trang.

## 2. Dữ liệu

### 2.1. Thêm vào `data/items.json`

`item_stats.py` sinh thêm (không sửa tay):

```json
{
  "enhance": {
    "pity_pct": 1,
    "max_times": 100,
    "levels": [
      {"level": 1, "success_pct": 100, "drop": false, "stones": 1, "xu_per_tier": 10},
      {"level": 8, "success_pct": 60, "drop": true, "stones": 3, "xu_per_tier": 80}
    ]
  },
  "regen": [
    {"enhance": 7, "hp_bp": 5, "mp": 1},
    {"enhance": 10, "hp_bp": 10, "mp": 2},
    {"enhance": 12, "hp_bp": 20, "mp": 3},
    {"enhance": 15, "hp_bp": 30, "mp": 5}
  ]
}
```

- Mỗi mục `slots` thêm `"regen": "hp"` (Giày, Hộ uyển, Quần, Mũ, Áo) hoặc `"mp"` (Vũ khí, Dây chuyền, Ngọc bội, Nhẫn trái, Nhẫn phải), theo game-design 2.7.
- `stackables` thêm `{"code": "charm_protect", "name": "Bùa bảo vệ"}` (sau hai loại đá).
- `hp_bp` là phần vạn của HP tối đa (5 = 0,05%); `mp` là MP mỗi giây.

**Bảng cường hóa** (`enhance.levels`; tỷ lệ và tụt cấp của game-design 3.3, đá và Xu do spec này chốt):

| Cấp đích | `success_pct` | Thất bại không bùa | `stones` | `xu_per_tier` | Xu mỗi lần với Lữ Hành (bậc 10) | với Xích Cương (bậc 50) |
| --- | --- | --- | --- | --- | --- | --- |
| +1 | 100 | — | 1 | 10 | 100 | 500 |
| +2 | 100 | — | 1 | 15 | 150 | 750 |
| +3 | 100 | — | 1 | 20 | 200 | 1.000 |
| +4 | 100 | — | 1 | 25 | 250 | 1.250 |
| +5 | 90 | giữ cấp | 2 | 40 | 400 | 2.000 |
| +6 | 80 | giữ cấp | 2 | 50 | 500 | 2.500 |
| +7 | 70 | giữ cấp | 2 | 60 | 600 | 3.000 |
| +8 | 60 | giảm 1 cấp | 3 | 80 | 800 | 4.000 |
| +9 | 50 | giảm 1 cấp | 3 | 100 | 1.000 | 5.000 |
| +10 | 40 | giảm 1 cấp | 3 | 120 | 1.200 | 6.000 |
| +11 | 30 | giảm 1 cấp | 4 | 150 | 1.500 | 7.500 |
| +12 | 25 | giảm 1 cấp | 4 | 180 | 1.800 | 9.000 |
| +13 | 20 | giảm 1 cấp | 5 | 220 | 2.200 | 11.000 |
| +14 | 15 | giảm 1 cấp | 5 | 260 | 2.600 | 13.000 |
| +15 | 10 | giảm 1 cấp | 5 | 300 | 3.000 | 15.000 |

**Chi phí trung bình** (script mô phỏng với hạt giống cố định và in ra; một món từ +0):

| Đích | Không bùa: lượt / đá / Xu bậc 50 | Có bùa từ +8: lượt / đá / Xu bậc 50 / bùa |
| --- | --- | --- |
| +7 | 8 / 12 / 13.000 | như không bùa |
| +10 | 24 / 56 / 83.000 | 14 / 29 / 43.000 / 6 |
| +12 | 105 / 305 / 515.000 | 21 / 57 / 100.000 / 13 |
| +15 | khoảng 6.500 / 20.000 / 36 triệu | 37 / 140 / 320.000 / 30 |

Ở bậc 10 Xu bằng 1/5 cột bậc 50. Tham chiếu thu nhập (game-design 2.7): quái thường cấp 50 cho khoảng 200 Xu và 0,2 viên đá mỗi con; khoảng 720 con mỗi giờ. Lên +7 đủ bộ Lữ Hành tốn khoảng 120 đá và 26.000 Xu, chừng một giờ cày ở cấp 10–19. Số lượt không bùa lên +15 khớp mức "khoảng 6.500 lượt" của game-design 3.3.

**Server kiểm tra khi nạp:** `levels` đúng 15 mục cấp 1–15; `success_pct` trong 1–100 và không tăng theo cấp; cấp 1–4 có `success_pct` 100 và `drop` false; `drop` chỉ true khi `success_pct` < 100; `stones` ≥ 1; `xu_per_tier` ≥ 1 và không giảm; `pity_pct` ≥ 1; `max_times` trong 1–1000; `regen` có `enhance` tăng dần trong 1–15, `hp_bp` và `mp` dương và tăng dần; mỗi ô có `regen` là `hp` hoặc `mp`; `charm_protect` có trong `stackables`.

## 3. Lưu trữ

Migration `00005_enhance.sql`: `character_items.pity smallint[] NOT NULL DEFAULT '{}'`. Mảng rỗng nghĩa là mọi cấp đích bằng 0; nếu không, đúng 15 số, số thứ k (0-based) là phần cộng (điểm %) của cấp đích k + 1. Store nạp và lưu cùng các cột khác. Khi vào game, món có `pity` sai độ dài hoặc có số ngoài 0–99 thì đặt lại về rỗng và ghi log cảnh báo (không bỏ món).

## 4. Server

### 4.1. Cường hóa

`EnhanceReq{item, protect, times}`, `item` là `ItemRef{equipped, slot}`; `times` = 0 hiểu là 1. Kiểm tra theo thứ tự, lỗi đầu tiên được trả, mọi lỗi không tốn gì:

1. Nhân vật chết: `DEAD`.
2. Nhân vật không đứng trong khu an toàn: `NOT_IN_SAFE_ZONE`.
3. Ô ngoài phạm vi, hoặc `times` < 0 hay > `max_times`: `BAD_REQUEST`.
4. Ô rỗng: `ITEM_NOT_FOUND`.
5. Món không phải trang bị: `BAD_REQUEST`.
6. Món đã +15: `ENHANCE_MAXED`.
7. Thiếu đá cho cấp đích, hoặc `protect` với cấp đích có `drop` mà không có bùa: `NOT_ENOUGH_MATERIALS`.
8. Thiếu Xu (`xu_per_tier` × bậc): `NOT_ENOUGH_XU`.

Mỗi lần thử, với cấp hiện tại n và cấp đích k = n + 1:

1. Trừ đá của cấp đích, Xu, và một bùa nếu `protect` và cấp đích có `drop`. Lấy từ các chồng theo thứ tự ô tăng dần.
2. Tỷ lệ thật = min(100, `success_pct` + phần cộng của cấp đích k). Thành công nếu số ngẫu nhiên nguyên đều trong [0, 100) của `z.rng` nhỏ hơn tỷ lệ thật.
3. Thành công: mức cường hóa thành k, phần cộng của k về 0.
4. Thất bại: phần cộng của k tăng `pity_pct` (không vượt quá 100 − `success_pct`); nếu cấp đích có `drop` và không dùng bùa thì mức cường hóa thành n − 1; nếu không, giữ n.

Lặp lại khi cả ba điều kiện đúng: chưa đủ `times` lần, lần vừa rồi thất bại mà giữ cấp, và còn đủ đá, bùa (nếu `protect` và cấp đích có `drop`) và Xu cho lần kế. Thành công hay tụt cấp đều dừng.

Sau cùng: nếu món đang mặc thì tính lại chỉ số và áp luật HP của 3a mục 4.7; nếu là vũ khí đang mặc và mức phát sáng đổi thì phát `LookChange` (mục 4.3); lưu ngay; trả `EnhanceResp{attempts, success, level, update, stats}` với `success` là kết quả lần thử cuối, `level` là mức sau cùng, `update` gồm ô của món và các ô vật phẩm đã trừ, `stats` có Xu mới.

### 4.2. Hồi theo mốc

- Mốc của một món = mốc cao nhất trong `regen` có `enhance` ≤ mức cường hóa của món; dưới +7 thì không có. Chỉ tính món đang mặc **đủ điều kiện mặc**.
- Mỗi người chơi có tổng `hp_bp` (từ món `regen: hp`) và tổng `mp` (từ món `regen: mp`), tính lại cùng `recompute`. Đủ 10 món +15: 150 bp (1,5% HP) và 25 MP mỗi giây.
- Mỗi `z.ticks(1)` tick (khi `tickNo` chia hết cho số đó), bước `runPlayerTimers` cho mọi người chơi còn sống:
  - HP: `carry += maxHP × hp_bp`; HP tăng `carry / 10000` (không vượt HP tối đa); `carry %= 10000`. HP đầy thì `carry` về 0. HP đổi thì `hpChanged` và `statsDirty`.
  - MP: tăng `mp`, không vượt MP tối đa; đổi thì `statsDirty`.
- Hồi cả khi đang giao chiến; người chết không hồi.

### 4.3. Vũ khí phát sáng

- `EntityEnter.weapon_enhance` của người chơi là mức cường hóa của vũ khí đang mặc đủ điều kiện, 0 nếu không có.
- Khi số đó đổi (mặc, tháo vũ khí; cường hóa vũ khí đang mặc; lên cấp làm vũ khí đủ điều kiện), zone thêm `LookChange{id, weapon_enhance}` vào sự kiện tick; người thấy người chơi đó và chính người đó nhận trong `WorldTick.looks`.
- Client đổi ra màu: +7 đến +9 Xanh, +10 đến +11 Vàng, +12 đến +14 Tím, +15 Đỏ (game-design 3.3).

## 5. Giao thức

`ProtocolVersion` lên **7**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 71 | `EnhanceReq` | `ItemRef item = 1`, `bool protect = 2`, `int32 times = 3` |
| 72 | `EnhanceResp` | `int32 attempts = 1`, `bool success = 2`, `int32 level = 3`, `InventoryUpdate update = 4`, `SelfStats stats = 5` |

```proto
// An item in the bag (equipped = false) or worn.
message ItemRef {
  bool equipped = 1;
  int32 slot = 2;
}

message LookChange {
  uint64 id = 1;
  int32 weapon_enhance = 2;
}
```

- `EntityEnter.weapon_enhance = 14`; `WorldTick.looks = 9` (`repeated LookChange`). Thứ tự client áp một `WorldTick`: `moves`, `casts`, `hits`, `deaths`, `hp_changes`, `level_ups`, `looks`; ghi chú của `WorldTick` cập nhật theo.
- `SelfStats.hp_regen_bp = 22`, `SelfStats.mp_regen = 23`: tổng hồi mỗi giây hiện có, để hiển thị.
- `ErrorCode`: `NOT_ENOUGH_XU = 30`, `NOT_ENOUGH_MATERIALS = 31`, `ENHANCE_MAXED = 32`, `NOT_IN_SAFE_ZONE = 33`.
- Fixture mới: `enhance_req.hex`, `enhance_resp.hex`, `world_tick_look.hex` (một `LookChange`), `entity_enter_glow.hex` (người chơi có `weapon_enhance` 12). Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`)

- **`core/forge_rules.gd`** (không dùng Node): đọc bảng `enhance` và `regen` từ `item_book`; `enhance_cost(item)` trả `{stones, xu, success_pct, drop, charm_usable}` của cấp đích kế tiếp; `enhance_block(item, inventory, xu, in_safe, protect)` trả `""`, `"safe"`, `"maxed"`, `"stones"`, `"charm"` hoặc `"xu"`, cùng thứ tự với server; `glow_tier(enhance)` trả 0–4; `regen_of(item)` trả mốc của món.
- `core/entity_table.gd`: lưu `weapon_enhance` từ `EntityEnter`; áp `looks` và trả hiệu ứng `{"type": "look", id, weapon_enhance}`.
- `core/errors.gd`: NOT_ENOUGH_XU "Không đủ Xu", NOT_ENOUGH_MATERIALS "Không đủ nguyên liệu", ENHANCE_MAXED "Đã cường hóa tối đa", NOT_IN_SAFE_ZONE "Cần đứng trong khu an toàn".

### 6.2. Giao diện

- **Cửa sổ Rèn** (`ui/forge_window.gd`): mở từ nút **Cường hóa** trong ô thông tin món. 3b có một tab **Cường hóa** (3c thêm Khảm ngọc và Tẩy luyện):
  - tên món và +n → +n+1; năm chỉ số hiện tại và sau khi lên (chỉ số khác 0);
  - tỷ lệ thành công gốc, "Thất bại: giữ cấp" hoặc "Thất bại: giảm 1 cấp";
  - chi phí và số đang có: đá, Xu, bùa (khi dùng được);
  - ô chọn **Dùng bùa bảo vệ**: chỉ bật được khi cấp đích từ +8 và có bùa;
  - nút **Cường hóa** (`times` 1) và **Liên tục** (`times` 100);
  - dòng kết quả: "Thành công! Lên +8", "Thất bại, giữ +6", "Thất bại, tụt về +7"; với Liên tục thêm "sau N lần".
  - Nút khóa thì ghi lý do theo `enhance_block`; ngoài khu an toàn ghi "Vào khu an toàn để cường hóa".
- **Ô thông tin món**: tên kèm "+n"; với món +7 trở lên thêm dòng "Mốc +n: hồi x% HP mỗi giây" hoặc "hồi y MP mỗi giây"; nút Cường hóa.
- **Tab Trang bị và Hành trang**: ô món hiện "+n" khi n > 0.
- **Tab Thông tin**: thêm "Hồi HP mỗi giây" (phần trăm, từ `hp_regen_bp`) và "Hồi MP mỗi giây".
- **Vầng sáng vũ khí** (`ui/entity_view.gd`): người chơi có `glow_tier` > 0 có vòng sáng nhấp nháy quanh tay cầm vũ khí, màu Xanh/Vàng/Tím/Đỏ, kèm vài hạt lấp lánh vẽ bằng `_draw`.

### 6.3. Smoke

`tools/smoke.sh` sau bước 3a chạy `devitem` cho tài khoản smoke: một vũ khí Lữ Hành đúng class, 10 đá cường hóa, 1.000 Xu. Smoke vào lại (điểm vào nằm trong khu an toàn), cường hóa vũ khí lên +1, in "SMOKE OK" kèm mức mới và Xu đã tiêu.

## 7. Lệnh dev

`devitem` thêm `-enhance <0-15>` cho trang bị (mặc định 0) và nhận `-code charm_protect`.

## 8. Kiểm thử

**Script:** `item_stats.py` kiểm tra bảng mục 2.1; in bảng chi phí trung bình với hạt giống cố định và kiểm tra số lượt không bùa lên +15 trong khoảng 5.500–7.500 (game-design: khoảng 6.500).

**Server** (`go test -race`):

- `gamedata`: dữ liệu mới nạp được; từ chối thiếu cấp, `success_pct` tăng, cấp 1–4 có `drop`, `drop` ở tỷ lệ 100, `xu_per_tier` giảm, `regen` không tăng dần, ô thiếu `regen`, thiếu `charm_protect`.
- `items`: một lần thử với tỷ lệ 100% luôn lên; với hạt giống cố định, thất bại cộng phần ẩn đúng cấp đích, giữ khi tụt, về 0 khi lên; phần ẩn của +15 không vượt 90; 90 lần thất bại liền ở +15 thì lần sau chắc chắn lên; bùa chỉ bị lấy ở cấp đích có `drop`; thất bại có bùa không tụt; Liên tục dừng đúng lúc thành công, tụt cấp, hết vật liệu hoặc đủ lần; đá lấy theo thứ tự ô; mốc hồi của món +6, +7, +11, +15; tổng hồi đủ bộ +15 là 150 bp và 25 MP; món không đủ điều kiện không tính.
- `store`: lưu và nạp `pity`; `pity` sai thì về rỗng và ghi log.
- `zone`: từng mã lỗi theo thứ tự (chết trước ngoài khu an toàn; ngoài khu an toàn trước ô sai); lỗi không tốn gì; cường hóa món đang mặc làm chỉ số tăng và áp luật HP; lưu ngay; Xu trong `stats` giảm đúng; hồi mỗi giây đúng lượng (1.640 HP với 5 bp: giây đầu 0 HP, giây thứ hai 1 HP, tổng 8 HP sau 10 giây), không hồi khi chết, không vượt tối đa; `LookChange` khi mặc vũ khí +10 và khi cường hóa vũ khí đang mặc từ +6 lên +7, không phát khi cường hóa món khác; người ngoài tầm nhìn không nhận.
- `gateway`: `ProtocolVersion` 7; fixture mới.
- e2e `TestEnhanceOverWebSocket`: `devitem` cho vũ khí, 20 đá, 10.000 Xu; vào game ở điểm vào (khu an toàn), cường hóa lên +4 bằng bốn lệnh (100%), chỉ số tăng, Xu giảm 700; đi ra khỏi khu an toàn thì bị `NOT_IN_SAFE_ZONE`; thoát rồi vào lại, món vẫn +4.

**Client** (`tests/run.sh`): `forge_rules` (chi phí, lý do khóa, mức sáng, mốc hồi); `entity_table` (`looks`, `weapon_enhance`); giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** `test1` và `test2` được `devitem` cho đá, bùa và Xu; cường hóa đồ đang mặc lên +7, +10, +15 trong khu an toàn; thấy hồi HP/MP mỗi giây và vũ khí của nhau phát sáng đúng màu.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 3.3: bảng đá và Xu mỗi lần thử, bùa chỉ dùng từ cấp đích +8, cường hóa liên tục, cường hóa trong khu an toàn cho tới khi có NPC thợ rèn; mục 2.7: hồi mỗi giây cộng dồn phần lẻ.
- `CLAUDE.md`: cường hóa và tỷ lệ ẩn nằm trong `internal/items`; bảng cường hóa sinh bằng `item_stats.py`.
- README hai repo: cửa sổ Rèn, `devitem -enhance`.
