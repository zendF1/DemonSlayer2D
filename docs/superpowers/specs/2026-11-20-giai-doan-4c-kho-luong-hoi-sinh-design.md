# Giai đoạn 4c — Kho chung tài khoản, Lượng và hồi sinh tại chỗ

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần cuối của Giai đoạn 4 (xem bảng chia ở [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 4.1 (Thủ kho), 5.4, 5.7 (hồi sinh khi chết) và 7.1 (Xu, Lượng); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 4a, [spec 4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) và các spec Giai đoạn 3. 4c chạy sau khi 4b đã triển khai.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

- **Kho:** Thủ kho ở Tổng hành dinh và ở trại mỗi map luyện cấp mở kho chung của tài khoản. Người chơi gửi đồ từ hành trang vào kho, rút ra, sắp xếp và trả Xu để mở rộng kho. Nhân vật khác cùng tài khoản thấy cùng một kho.
- **Lượng:** mỗi tài khoản có ví Lượng, hiện cạnh Xu. Chưa có nạp tiền; người vận hành cộng Lượng bằng lệnh dev.
- **Dùng Lượng:** khi chết chọn **Hồi sinh tại chỗ** (1 Lượng, đầy HP/MP ngay chỗ chết) hoặc **Về làng** (miễn phí, như 4a). Nút **Lượng** trên HUD mở Cửa hàng Lượng ở bất cứ đâu.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Kho thuộc ai | Tài khoản (chung ba nhân vật) | Game-design 4.1 "kho chung tài khoản" |
| Ai giữ kho khi đang chơi | Zone của nhân vật đang ở trong thế giới. Mỗi tài khoản chỉ có một nhân vật trong thế giới (`Takeover`, thoát nhân vật thì chờ lưu xong), nên không có hai zone cùng sửa một kho | Không cần khóa hay dịch vụ riêng |
| Lưu kho | Cùng giao dịch lưu nhân vật; mọi lần lưu ghi lại cả kho (xóa rồi chèn, như hành trang); lưu ngay sau mỗi lệnh kho | Saver gộp theo nhân vật và giữ bản mới nhất, nên phần kho không được bỏ ở lần lưu nào |
| Kích thước | Bắt đầu 30 ô; mỗi lần mở rộng thêm 10 ô, tối đa 120 ô | Đủ cho đồ của ba nhân vật mà vẫn có khoản tiêu Xu |
| Giá mở rộng | Lần thứ k (k = 1 … 9) tốn 20.000 × k Xu; tổng 900.000 Xu | Game-design 7.2: mở kho là khoản tiêu Xu. Lần đầu bằng khoảng nửa giờ đánh quái cấp 15 |
| Gửi và rút | Cả ô một lần; vật phẩm chồng dồn vào chồng cùng mã còn chỗ rồi tới ô trống đầu tiên; không đủ chỗ thì không làm gì (luật 3a mục 4.1); không gửi đồ đang mặc; không gửi Xu hay Lượng | Ít thao tác, cùng luật với hành trang |
| Sắp xếp kho | Như sắp xếp hành trang (3a mục 4.7) | Một luật |
| Đặt Thủ kho | Tổng hành dinh và trại mỗi map luyện cấp | Game-design 5.4: Nhà Hoa Tử Đằng có kho |
| Kết quả tẩy luyện đang chờ (3c) | Gửi, rút, sắp xếp kho xóa kết quả chờ (có thể dời món); mở kho và mở rộng thì không | Giữ mục đích luật 3c mục 4.5 |
| Lượng thuộc ai | Tài khoản | Nạp tiền theo tài khoản (game-design 7.1) |
| Nguồn Lượng ở v1 | Chỉ lệnh dev `devitem -luong`; không có cổng nạp, không đổi Xu ↔ Lượng, không rơi từ quái | Giai đoạn này không làm thanh toán |
| Dùng Lượng ở v1 | Hồi sinh tại chỗ (1 Lượng) và Cửa hàng Lượng | Game-design 5.7 và 7.1 |
| Cửa hàng Lượng | Mở ở bất cứ đâu từ nút HUD; danh mục cấu hình trong `items.json`; bản đầu chỉ bán Bùa bảo vệ, 2 Lượng mỗi lá | Không bán vật phẩm tăng tốc như x2 EXP (v1 chỉ lõi game); giá Lượng là số tạm cho tới khi có tỷ giá nạp |
| Hồi sinh tại chỗ | Ngay vị trí chết, đầy HP/MP, trừ 1 Lượng, lưu ngay; không hoàn EXP (Giai đoạn 5 mới có phạt EXP) | Game-design 5.7 |
| Về làng | Như 4a: điểm hồi sinh gần nhất của map; tự về sau 60 giây | Game-design 5.7 |
| Lưu Lượng | Lưu ngay mỗi khi tiêu | Tài sản trả tiền thật về sau |
| Nạp Lượng khi đang chơi | Chưa có. Khi có cổng nạp, thêm một lệnh vào zone đang giữ tài khoản | Ngoài phạm vi |

### 1.3. Không làm ở 4c

Cổng nạp và tỷ giá; Lượng khóa/không khóa; gửi Xu vào kho; kho riêng cho vật phẩm nhiệm vụ (Giai đoạn 6); kho bang (Giai đoạn 7+); vật phẩm tăng tốc; phạt EXP khi chết do PK (Giai đoạn 5); luật hồi sinh riêng của phó bản và sự kiện.

## 2. Dữ liệu

### 2.1. Thêm vào `data/items.json`

`item_stats.py` sinh thêm:

```json
{
  "stash": {"base": 30, "step": 10, "max": 120, "expand_xu_per_step": 20000},
  "shops": [
    {"id": "luong", "name": "Cửa hàng Lượng", "currency": "luong", "anywhere": true,
     "goods": [{"code": "charm_protect", "price": 2}]}
  ]
}
```

- `shops[].anywhere` (không bắt buộc, mặc định `false`): mở được không cần NPC.
- Giá mở rộng lần thứ k = `expand_xu_per_step` × k, với k = (kích thước hiện tại − `base`) / `step` + 1.

**Server kiểm tra khi nạp:** `stash.base` ≥ 1; `step` ≥ 1; `max` ≥ `base` và (`max` − `base`) chia hết cho `step`; `max` ≤ 200; `expand_xu_per_step` ≥ 1; `currency` là `xu` hoặc `luong`; cửa hàng `anywhere` phải dùng `luong`; NPC (map) chỉ được trỏ tới cửa hàng không `anywhere`.

### 2.2. Thủ kho trong file map

Vai NPC mới `storage`, không có `shop` hay `routes`. `tools/mapgen.py` thêm `thu_kho` "Thủ kho" ở Tổng hành dinh và trại mỗi map luyện cấp, cách `default` không quá 3 ô (nhân vật mới vào là gửi được). Ở Kumotori: (3,5; 7,5), cách `default` 2,8 ô, cách lửa trại 4 ô, cách Xa phu 5 ô.

## 3. Lưu trữ

Migration `00007_account_stash.sql`:

- `accounts.luong bigint NOT NULL DEFAULT 0 CHECK (luong >= 0)`.
- `accounts.stash_size smallint NOT NULL DEFAULT 30 CHECK (stash_size BETWEEN 1 AND 200)`.
- Bảng `account_stash`: `account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE`, `slot smallint NOT NULL CHECK (slot >= 0)`, và đúng các cột món đồ của `character_items` sau 00006 (`code`, `count`, `quality`, `element`, `enhance`, `lines`, `hidden`, `pity`, `gems`) với cùng kiểu, mặc định và ràng buộc. Khóa chính `(account_id, slot)`. Down xóa bảng và hai cột.

**Nạp:** `GetCharacter` (dùng khi vào game) nạp thêm Lượng, kích thước kho và các món kho của tài khoản (một truy vấn tài khoản, một truy vấn kho). `ListCharacters` không nạp.

**Lưu:** `CharState` mang phần tài khoản (id tài khoản, Lượng, kích thước kho, các món kho). Trong cùng giao dịch với phần nhân vật: `UPDATE accounts SET luong, stash_size`, rồi xóa và chèn lại các dòng `account_stash`. Phần tài khoản rỗng (test cũ không đặt) thì không đụng tới tài khoản.

**Khi vào game** (`world.Enter`): kích thước kho ngoài [`base`, `max`] thì kẹp lại và ghi log; món kho sai thì bỏ và ghi log theo đúng luật kiểm tra món của 3a mục 3 (ô ≥ kích thước kho thay cho ô hành trang ≥ `bag_size`), kèm luật `pity` (3b) và `gems` (3c).

## 4. Server

### 4.1. Trạng thái

- `PlayerInfo` và bản trao tay (4a mục 3.2.1) thêm id tài khoản, Lượng và kho (kích thước và các ô, bản sao sâu).
- `SelfStats.luong` mang Lượng hiện có.
- Mọi lệnh kho dùng NPC vai `storage` với luật tầm của 4b mục 4.1.

### 4.2. Lệnh kho

Bước chung cho cả năm lệnh, theo thứ tự: chết `DEAD`; NPC không có hoặc không phải vai `storage` `BAD_REQUEST`; không gần NPC `NPC_TOO_FAR`. Rồi:

**`StashOpenReq{npc_id}`:** trả `StashOpenResp{stash}` với kích thước và các ô có đồ.

**`StashDepositReq{npc_id, bag_slot}`:**

4. `bag_slot` ngoài phạm vi: `BAD_REQUEST`.
5. Ô rỗng: `ITEM_NOT_FOUND`.
6. Kho không chứa hết: `STASH_FULL`.

Hợp lệ: thêm cả ô vào kho, ô hành trang thành rỗng, xóa kết quả tẩy luyện chờ, lưu ngay, trả `StashDepositResp{update, stash}` (`stash` là các ô kho đã đổi).

**`StashWithdrawReq{npc_id, stash_slot}`:**

4. `stash_slot` ngoài [0, kích thước): `BAD_REQUEST`.
5. Ô rỗng: `ITEM_NOT_FOUND`.
6. Hành trang không chứa hết: `BAG_FULL`.

Hợp lệ: thêm vào hành trang, ô kho thành rỗng, xóa kết quả chờ, lưu ngay, trả `StashWithdrawResp{update, stash}`.

**`StashExpandReq{npc_id}`:**

4. Kích thước đã bằng `max`: `STASH_MAXED`.
5. Thiếu Xu: `NOT_ENOUGH_XU`.

Hợp lệ: trừ Xu, kích thước tăng `step`, lưu ngay, trả `StashExpandResp{size, stats}`.

**`StashSortReq{npc_id}`:** gộp và xếp như 3a mục 4.7 trên các ô kho; xóa kết quả chờ; lưu ngay; trả `StashSortResp{stash}` với **mọi** ô kho.

### 4.3. Mua bằng Lượng

`BuyReq` thêm `shop_id`. Thứ tự kiểm tra của 4b mục 4.2 đổi thành:

1. `DEAD`.
2. Nếu `npc_id` khác rỗng: các bước NPC của 4b (`BAD_REQUEST`, `NPC_TOO_FAR`), cửa hàng là của NPC, `shop_id` bị bỏ qua. Nếu `npc_id` rỗng: `shop_id` phải là một cửa hàng `anywhere`, nếu không `BAD_REQUEST`.
3. Cửa hàng không bán `code`: `BAD_REQUEST`.
4. `count` < 1 hoặc > `stack_max`: `BAD_REQUEST`.
5. Không đủ tiền theo `currency`: `NOT_ENOUGH_XU` hoặc `NOT_ENOUGH_LUONG`.
6. `BAG_FULL`.

Hợp lệ: trừ Xu hoặc Lượng, thêm đồ, lưu ngay, trả `BuyResp{update, stats}`.

### 4.4. Hồi sinh

`RespawnReq{in_place}`:

1. Nhân vật chưa chết: `NOT_DEAD`.
2. `in_place` mà Lượng < 1: `NOT_ENOUGH_LUONG`.

- `in_place`: trừ 1 Lượng; nhân vật sống lại **ngay vị trí chết**, đầy HP/MP, người xung quanh thấy (như `respawnPlayer`, không đổi vị trí); lưu ngay; trả `RespawnResp{x, y}` và `SelfStats` mới.
- Không `in_place`: như 4a (điểm hồi sinh gần nhất của map).
- Tự hồi sinh sau 60 giây luôn là về làng.

## 5. Giao thức

`ProtocolVersion` lên **11**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 95 | `StashOpenReq` | `string npc_id = 1` |
| 96 | `StashOpenResp` | `Stash stash = 1` |
| 97 | `StashDepositReq` | `string npc_id = 1`, `int32 bag_slot = 2` |
| 98 | `StashDepositResp` | `InventoryUpdate update = 1`, `repeated ItemSlot stash = 2` |
| 99 | `StashWithdrawReq` | `string npc_id = 1`, `int32 stash_slot = 2` |
| 100 | `StashWithdrawResp` | `InventoryUpdate update = 1`, `repeated ItemSlot stash = 2` |
| 101 | `StashExpandReq` | `string npc_id = 1` |
| 102 | `StashExpandResp` | `int32 size = 1`, `SelfStats stats = 2` |
| 103 | `StashSortReq` | `string npc_id = 1` |
| 104 | `StashSortResp` | `repeated ItemSlot stash = 1` |

```proto
// The account stash: size slots, occupied ones listed.
message Stash {
  int32 size = 1;
  repeated ItemSlot items = 2;
}
```

- `BuyReq.shop_id = 4` (`string`); `RespawnReq.in_place = 1` (`bool`); `SelfStats.luong = 24` (`int64`).
- Trong `repeated ItemSlot stash` của các trả lời, `ItemSlot` không có món là ô kho đã thành rỗng (như `InventoryUpdate`).
- `ErrorCode`: `STASH_FULL = 42`, `STASH_MAXED = 43`, `NOT_ENOUGH_LUONG = 44`.
- Fixture mới: `stash_open_resp.hex` (kho 40 ô, hai món), `stash_deposit_resp.hex`, `respawn_req_in_place.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/stash.gd`**: nạp `Stash`, áp danh sách ô đổi; `size`, `items` (ô → món); `can_take(item)` cùng luật với hành trang (tách phần mô phỏng thêm món của `inventory.gd` thành hàm dùng chung); `expand_cost(size, cfg)` trả −1 khi đã tối đa; `deposit_block`, `withdraw_block`, `expand_block` trả `""`, `"full"`, `"bag"`, `"max"` hoặc `"xu"`.
- **`core/inventory.gd`**: lấy `luong` từ `SelfStats`.
- **`core/shop_rules.gd`**: `buy_block` theo `currency` (thêm lý do `"luong"`).
- **`core/item_book.gd`**: đọc `stash` và `anywhere`.
- **`core/errors.gd`**: STASH_FULL "Kho đã đầy", STASH_MAXED "Kho đã mở rộng tối đa", NOT_ENOUGH_LUONG "Không đủ Lượng". **`core/wire.gd`**: payload mới.

### 6.2. Giao diện

- **NPC Thủ kho** (`ui/map_view.gd`): vòng tròn xanh dương, chữ K. Cửa sổ NPC có nút **Kho**.
- **Cửa sổ kho** (`ui/stash_window.gd`): gửi `StashOpenReq` khi mở. Hai tab **Kho** (lưới 6 cột, "Kho n/size") và **Hành trang** (lưới hành trang). Chạm món trong tab Kho: ô thông tin món với nút **Rút**; trong tab Hành trang: nút **Gửi** (đồ đang mặc không hiện ở đây). Đầu cửa sổ có nút **Sắp xếp** và **Mở rộng (20.000 Xu)** (khóa và ghi lý do theo `expand_block`). Tự đóng khi cách Thủ kho quá 4 ô.
- **Lượng**: đầu tab Hành trang ghi "Xu: … · Lượng: …".
- **Nút Lượng trên HUD**: nút tròn nhỏ ở cụm góc phải trên, mở cửa hàng (`ui/shop_window.gd` của 4b) với cửa hàng `luong`, chỉ có tab Mua, giá ghi "2 Lượng", gửi `BuyReq{shop_id: "luong"}`.
- **Bảng chết** (`ui/death_panel.gd`): hai nút **Hồi sinh tại chỗ (1 Lượng)** (khóa và ghi "Không đủ Lượng" khi Lượng < 1, có dòng "Đang có: n Lượng") và **Về làng**; đếm ngược "Tự về làng sau N giây".

### 6.3. Smoke

`tools/smoke.sh` thêm `devitem -luong 3` cho tài khoản smoke. Sau bước 4b, smoke về Kumotori (hoặc dùng `devchar -map kumotori_1 -x 5.5 -y 5.5`), mua một Bùa bảo vệ ở Cửa hàng Lượng (Lượng 3 → 1), mở kho ở Thủ kho, gửi lá bùa, kiểm tra nó ở trong kho, rút ra lại. In "SMOKE OK" kèm Lượng và số món trong kho.

## 7. Lệnh dev

`devitem` thêm `-luong <n>`: cộng Lượng cho **tài khoản** (dùng một mình được). Vẫn yêu cầu mọi nhân vật của tài khoản đã thoát game.

## 8. Kiểm thử

**Script:** `item_stats.py` kiểm tra khối `stash` và cửa hàng Lượng; `mapgen.py` kiểm tra mỗi trại và thành có một Thủ kho cách `default` không quá 3 ô.

**Server** (`go test -race`):

- `gamedata`: dữ liệu mới nạp được; từ chối `step` 0, `max` < `base`, `max − base` không chia hết cho `step`, `max` 300, cửa hàng `anywhere` dùng Xu, `currency` lạ; `Validate` từ chối NPC trỏ tới cửa hàng `anywhere`.
- `mapdata`: vai `storage` hợp lệ; `storage` có `shop` bị từ chối; năm map thật có Thủ kho.
- `store` (DB): migration lên rồi xuống; tài khoản cũ có Lượng 0 và kho 30 ô; lưu rồi nạp lại Lượng, kích thước và món kho (có dòng, ngọc, `pity`); lưu với phần tài khoản rỗng không đụng tài khoản; xóa tài khoản xóa kho; hai nhân vật cùng tài khoản nạp ra cùng kho.
- `world`: món kho sai bị bỏ và ghi log; kích thước 10 kẹp lên 30, 150 kẹp xuống 120; món ở ô ≥ kích thước bị bỏ.
- `zone` kho: từng mã lỗi theo thứ tự cho năm lệnh; gửi chồng 600 đá vào kho có 500 đá thành 999 + 101; gửi khi kho đầy không đổi gì; rút khi hành trang đầy không đổi gì; mở rộng 30 → 40 tốn 20.000, 40 → 50 tốn 40.000, ở 120 bị `STASH_MAXED`; sắp xếp trả mọi ô; gửi, rút, sắp xếp xóa kết quả tẩy luyện chờ, mở kho thì không; mỗi lệnh đổi kho xếp một lần lưu có kho mới; kho đi qua bản trao tay khi qua cổng.
- `zone` Lượng: mua bùa ở cửa hàng `luong` với `npc_id` rỗng trừ 2 Lượng; `shop_id` là `general` với `npc_id` rỗng bị `BAD_REQUEST`; thiếu Lượng `NOT_ENOUGH_LUONG`; hồi sinh tại chỗ với 1 Lượng: vị trí không đổi, đầy HP/MP, Lượng 0, xếp một lần lưu, người xung quanh thấy HP; với 0 Lượng bị `NOT_ENOUGH_LUONG` và vẫn chết; về làng không tốn Lượng; tự hồi sinh sau 60 giây về làng.
- `gateway`: `ProtocolVersion` 11; fixture mới.
- `cmd/devitem`: `-luong 5` cộng vào tài khoản; dùng một mình được.
- e2e `TestStashOverWebSocket`: một tài khoản hai nhân vật; nhân vật 1 (có một kiếm Lữ Hành nhờ `devitem`) vào ở điểm `default` của Kumotori, trong tầm Thủ kho, gửi kiếm vào kho, mở rộng kho (đủ Xu nhờ `devitem -xu`), thoát; nhân vật 2 vào, mở kho thấy kiếm và kích thước 40, rút ra; `devitem -luong 2` trước khi vào, nhân vật 2 mua một bùa ở Cửa hàng Lượng (Lượng 0), mua lá thứ hai bị `NOT_ENOUGH_LUONG`; thoát rồi vào lại, Lượng vẫn 0, kho vẫn 40 ô.

**Client** (`tests/run.sh`): `stash` (nạp, áp ô đổi, `can_take`, giá mở rộng, lý do khóa); `shop_rules` với Lượng; `inventory` Lượng; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** `test1` và `test2` là hai tài khoản khác nhau, nên kho thử bằng hai nhân vật của `test1` (tạo nhân vật thứ hai nếu chưa có): gửi đồ ở trại Kumotori bằng nhân vật 1, rút bằng nhân vật 2 ở Tổng hành dinh; `devitem -luong 5` cho `test1` và `test2`; mua bùa bằng Lượng; để quái hạ rồi hồi sinh tại chỗ.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 4.1 (kho 30–120 ô, giá mở rộng, gửi/rút cả ô), mục 5.7 (hồi sinh tại chỗ ngay vị trí chết, lưu ngay), mục 7.1 (Lượng theo tài khoản, v1 chưa có nạp, Cửa hàng Lượng bản đầu chỉ có Bùa bảo vệ).
- Spec Giai đoạn 0 mục 1.1: Giai đoạn 4 đã gồm cả kho và Lượng ở 4c.
- `CLAUDE.md`: bất biến "kho và Lượng thuộc tài khoản, do zone của nhân vật đang chơi giữ và lưu cùng nhân vật; mọi lần lưu ghi lại cả kho".
- README hai repo: kho, Lượng, `devitem -luong`, hai nút hồi sinh.
