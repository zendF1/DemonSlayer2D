# Giai đoạn 7b — Giao dịch trực tiếp và sạp

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ hai của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md) mục 1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 5.1 (sạp giao dịch ở làng/thành), 5.4 (Tổng hành dinh có sạp giao dịch), 7.1 (Xu, Lượng) và 7.2 (giao dịch 1–1 có khóa/xác nhận, sạp tự động ở khu an toàn trong thành); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) (hành trang, luật thêm món tất cả hoặc không), [spec 4c](2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md) (phần tài khoản trong `CharState`), [spec 5b](2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md) (lời mời, thẻ người chơi) và [spec 7a](2026-11-20-giai-doan-7a-chat-design.md). 7b chạy sau khi 7a đã triển khai (giao thức 16, migration 00009).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

- **Giao dịch trực tiếp:** chạm một người chơi đứng gần trong khu an toàn, bấm **Giao dịch**. Người kia đồng ý thì cả hai thấy cửa sổ giao dịch: mỗi bên đặt tối đa 8 món từ hành trang và một số Xu. Mỗi bên bấm **Khóa**; khi cả hai đã khóa thì bấm **Xác nhận**; cả hai xác nhận thì đồ và Xu đổi chủ cùng lúc. Bất kỳ ai đổi món hay số Xu thì cả hai bên mất khóa và phải khóa lại.
- **Sạp:** ở Tổng hành dinh, người chơi chọn tối đa 10 món trong hành trang, đặt giá Xu cho từng món, đặt tên sạp và **Bày sạp**. Nhân vật ngồi yên, trên đầu hiện tên sạp. Người khác chạm vào xem sạp và mua; người bán nhận Xu ngay. Sạp chỉ mở khi người bán còn online.
- Không có cách nào nhân bản hay làm mất đồ, kể cả khi server sập giữa chừng: thay đổi của hai nhân vật luôn được ghi trong một giao dịch DB.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Ai xử lý giao dịch | Zone, khi cả hai nhân vật ở **cùng zone** (cùng map, cùng khu). Không có giao dịch qua khu khác | Một goroutine sở hữu cả hai hành trang nên đổi đồ là một bước, không cần khóa hay ký gửi; người giao dịch trực tiếp vốn đứng cạnh nhau |
| Nơi được giao dịch | Cả hai đứng trong **khu an toàn** (thành, trại, khu quanh cổng), cách nhau không quá **5 ô** khi mời và khi đồng ý | Cửa sổ giao dịch che màn hình điện thoại; không để bị đánh lúc đang giao dịch; 5 ô là khoảng "đứng cạnh nhau" |
| Tự hủy theo khoảng cách | Cách nhau quá **8 ô**, hoặc một bên ra khỏi khu an toàn, thì giao dịch tự hủy | Có khoảng đệm để đi lại chút ít không làm hủy |
| Cấp tối thiểu | Cấp **10** cho cả giao dịch, bày sạp và mua ở sạp | Giảm nhân vật rác chuyển đồ; như kênh Thế giới (7a) |
| Lời mời | Theo id thực thể; hết hạn 30 giây, xét lúc trả lời; mỗi người chỉ có một lời mời giao dịch đến đang chờ (`TRADE_BUSY` cho lời mời thứ hai) và một lời mời đi (mời người khác thì lời mời cũ bị bỏ) | Như lời mời tổ đội (5b) |
| Đặt món | Tối đa **8 ô hành trang**, mỗi ô nguyên ô (cả chồng); không đặt đồ đang mặc; mỗi lệnh thay toàn bộ phần của mình | Ít thao tác; tách chồng làm trước bằng lệnh khác thì chưa có, nên đặt nguyên ô là đủ cho v1 |
| Tiền | Xu; **Lượng không giao dịch** | Tổng quan mục 3.7; Lượng gắn với nạp tiền |
| Vật phẩm cấm giao dịch | Cờ `no_trade` trong `items.json`; v1 chỉ sách kỹ năng | Sách đến từ chính tuyến (6b); các món khác giao dịch được (tổng quan mục 3.7) |
| Hai bước | **Khóa** từng bên; chỉ khi cả hai đã khóa mới **Xác nhận**; cả hai xác nhận thì thực hiện. Đổi phần đặt (món hay Xu) thì cả hai bên mất khóa và mất xác nhận | Đúng game-design 7.2: "khóa nội dung rồi xác nhận; thay đổi món hoặc số tiền phải hủy xác nhận cũ" |
| Đồ đang đặt | Trong lúc giao dịch, mọi lệnh làm đổi hành trang hay Xu của người đó bị từ chối `IN_TRADE` (mục 4.9) | Món đã đặt không thể đổi hay biến mất trước khi thực hiện; không cần so lại từng ô |
| Thiếu chỗ khi thực hiện | Dựng hành trang mới của cả hai trên bản sao trước; một bên không chứa hết thì không đổi gì, cả hai mất khóa, người bấm nhận `BAG_FULL` hoặc `PARTNER_BAG_FULL` | Luật tất cả hoặc không của 3a |
| Lưu | Lưu ngay, **hai nhân vật trong một giao dịch DB** (`SaveLinked`), kèm một dòng nhật ký (`TradeLog`) | Chống nhân bản: không có lúc nào DB chỉ có một nửa giao dịch |
| Cách saver lưu nhóm | `CharState.Ops` (phép ghi kèm) và `Saver.SaveLinked` (mục 3.2): phép ghi kèm nối thêm khi gộp, giữ khi lưu lỗi; nhóm liên kết giữ tới khi ghi xong, nhóm chung nhân vật thì gộp | Saver hiện gộp theo nhân vật và ghi theo thứ tự bất kỳ; tổng quan mục 3.2; các phần 7c, 7f, 7h dùng lại |
| Nhật ký | Bảng `trade_logs` (mục 3.1) cho mọi giao dịch và mọi lần mua ở sạp | Tra cứu khi có khiếu nại; `item_logs` đầy đủ vẫn để sau |
| Sạp ở đâu | Chỉ map `town` (Tổng hành dinh), trong khu an toàn; cách NPC, điểm xuất hiện và cổng ít nhất **2 ô**, cách sạp khác ít nhất **1,5 ô** | Game-design 7.2 "khu an toàn trong thành"; không chắn NPC và đường đi của người mới tới |
| Sạp ngoại tuyến | Không; sạp đóng khi người bán rời zone (thoát, qua cổng, đổi khu) | Tổng quan mục 4 |
| Kích thước sạp | Tối đa **10 món**; tên sạp 1–20 ký tự (luật `textutil.Clean` của 7a) | Vừa một cửa sổ trên điện thoại |
| Giá | Giá Xu cho **một đơn vị**, 1–1.000.000.000; vật phẩm chồng bán lẻ được (mua 1 tới số còn lại); có thể bày một phần chồng | Chồng đá bán theo viên là cách dùng chính |
| Đổi giá | Đóng sạp rồi bày lại | Ít lệnh; `price` trong lệnh mua chặn mua nhầm giá |
| Người bán di chuyển | `MoveInput` bị bỏ qua khi sạp đang mở (vẫn ack `input_seq`); muốn đi thì đóng sạp | Sạp đứng yên mới tìm được; không đóng nhầm khi chạm cần điều khiển |
| Đồ đang bày | Như giao dịch: mọi lệnh làm đổi hành trang hay Xu của người bán bị từ chối `IN_STALL` | Như trên |
| Thuế | Người bán nhận giá × số lượng − thuế. Thuế lấy từ hàm `zone.Config.StallTax` (bang thành chủ và mức thuế); ở 7b luôn là 0. 7h đặt thuế tối đa 5% và cộng vào quỹ bang bằng phép ghi kèm `GuildFundTax` | Game-design 6.3: thuế trên giá giao dịch, tối đa 5%, khấu trừ từ tiền người bán, vào quỹ bang; tổng quan mục 4: thuế đặt lên sạp thay cho NPC đấu giá |
| Giao dịch trực tiếp có thuế không | Không | Xu trong giao dịch trực tiếp không phải "giá" (có thể là quà); game-design để mở phạm vi chịu thuế |
| Phí sạp | Không có | Game-design 7.2: không thêm phí chợ hệ thống |
| Kết quả tẩy luyện đang chờ (3c) | Giao dịch thành công và bán ở sạp xóa kết quả chờ của người có đồ đi ra | Luật 3c mục 4.5: món có thể dời ô |
| Báo kết quả | Gói đẩy `TradeClosed`, `StallSold`; client ghi thêm một dòng Hệ thống vào chat (7a, `add_local`) | Không cần tin Hệ thống từ server |

### 1.3. Bổ sung cho game-design

Game-design 7.2 để mở giao diện và chi tiết; spec này chốt nơi giao dịch, cấp 10, tối đa 8 món, hai bước khóa/xác nhận, sạp ở Tổng hành dinh chỉ khi online, 10 món, giá theo đơn vị, không thuế cho giao dịch trực tiếp, Lượng và sách không giao dịch. Ghi vào game-design mục 7.2 (mục 9).

### 1.4. Không làm ở 7b

Giao dịch giữa hai khu hay hai map; sạp ngoại tuyến; NPC đấu giá/treo bán (tổng quan mục 4); tách chồng trong hành trang; tìm kiếm món trên mọi sạp; giao dịch Lượng; đồ khóa; thuế (7h); nhật ký đồ và tiền đầy đủ (`item_logs`, `currency_logs`).

## 2. Dữ liệu

`docs/balance/item_stats.py` thêm trường không bắt buộc `no_trade` (mặc định `false`) cho vật phẩm chồng và đặt `"no_trade": true` cho 25 sách `book_<id chiêu>` (6b). `data/items.json` sinh lại. `gamedata.Stackable.NoTrade bool`; trang bị không có cờ này (luôn giao dịch được).

Hằng số trong code (`internal/zone`):

| Hằng số | Giá trị |
| --- | --- |
| `TradeMinLevel` | 10 |
| `TradeInviteTTL` | 30 giây |
| `TradeRange` | 5 ô (mời, đồng ý) |
| `TradeKeepRange` | 8 ô (tự hủy khi xa hơn) |
| `TradeMaxSlots` | 8 |
| `StallMaxEntries` | 10 |
| `StallTitleRunes` | 20 |
| `StallMaxPrice` | 1.000.000.000 Xu |
| `StallGapNPC` | 2 ô (tới NPC, điểm xuất hiện, hình chữ nhật cổng) |
| `StallGapStall` | 1,5 ô |
| `StallViewRange` | 6 ô + 0,5 ô dung sai |

## 3. Lưu trữ

### 3.1. Migration `00010_trades.sql`

Bảng `trade_logs`:

| Cột | Kiểu | Ghi chú |
| --- | --- | --- |
| `id` | `bigint generated always as identity` | khóa chính |
| `kind` | `text NOT NULL CHECK (kind IN ('trade', 'stall'))` | |
| `a_char` | `bigint NOT NULL` | giao dịch: người mời; sạp: người bán. Không có khóa ngoại để nhật ký còn khi nhân vật bị xóa |
| `b_char` | `bigint NOT NULL` | giao dịch: người được mời; sạp: người mua |
| `a_items`, `b_items` | `jsonb NOT NULL DEFAULT '[]'` | các món mỗi bên trao đi, mỗi món đủ thuộc tính như một dòng `character_items` (mã, số lượng, phẩm chất, hệ, cường hóa, dòng, dòng ẩn, `pity`, `gems`) kèm ô gốc |
| `a_xu`, `b_xu` | `bigint NOT NULL DEFAULT 0` | Xu mỗi bên trao đi; sạp: `b_xu` là tổng người mua trả |
| `tax` | `bigint NOT NULL DEFAULT 0` | thuế khấu trừ từ người bán (7h) |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | |

Chỉ mục `(a_char, created_at)` và `(b_char, created_at)`. Down xóa bảng.

### 3.2. Phép ghi kèm và lưu nhóm

**Store:**

- `store.Op` là interface với một phương thức không xuất khẩu `exec(ctx, tx pgx.Tx) error`, nên chỉ gói `store` định nghĩa được các loại phép. 7b định nghĩa `store.TradeLog{Kind string; A, B int64; AItems, BItems []ItemRow; AXu, BXu, Tax int64}` (chèn một dòng `trade_logs`). 7c, 7f, 7h thêm `GuildDonate`, `ClaimRewards`, `GuildFundTax` (tổng quan mục 3.2).
- `CharState.Ops []Op`: chạy trong cùng giao dịch DB với trạng thái nhân vật, ngay sau các dòng của nhân vật đó, theo thứ tự trong slice. Phép ghi kèm vẫn chạy khi dòng nhân vật không còn (nhân vật đã bị xóa).
- `Store.SaveCharacterStates(ctx, states []CharState) error`: **một** giao dịch DB; xếp các trạng thái theo id nhân vật tăng dần (tránh khóa chéo giữa hai giao dịch); với mỗi trạng thái ghi phần nhân vật (và phần tài khoản của 4c) rồi các phép ghi kèm của nó. Lỗi ở bất kỳ bước nào thì cả giao dịch hủy. `SaveCharacterState(st)` thành `SaveCharacterStates(ctx, []CharState{st})`.
- Hai trạng thái trong một nhóm luôn thuộc hai tài khoản khác nhau (mỗi tài khoản chỉ có một nhân vật trong thế giới), nên phần tài khoản không ghi chồng nhau.

**Saver** (`internal/saver`):

- `Writer` đổi thành `SaveCharacterStates(ctx, []store.CharState) error`.
- Mỗi mục đang chờ giữ: trạng thái mới nhất, **danh sách phép ghi kèm**, người chờ, và nhóm (có thể không có).
- `Save(st, done)`: trạng thái của mục thay bằng `st`; `st.Ops` được **nối vào cuối** danh sách phép của mục (không thay); `done` vào người chờ của mục.
- `SaveLinked(states, done)`: gọi `Save` cho từng trạng thái (không có `done`), rồi gộp nhóm của mọi mục đó (và mọi nhóm mà các mục đó đang thuộc) làm **một** nhóm; `done` vào người chờ của nhóm. Nhận đúng một giá trị khi cả nhóm đã ghi.
- Ghi: mỗi nhóm là một lần gọi `SaveCharacterStates` với trạng thái của mọi thành viên (mỗi trạng thái mang toàn bộ phép của mục); mục không thuộc nhóm nào ghi một mình. Thành công thì báo người chờ của mục và của nhóm, xóa mục, nhóm và phép.
- Lỗi: đưa cả đơn vị trở lại hàng chờ, giữ nhóm và phép. Nếu trong lúc đó đã có mục mới hơn cho một nhân vật thì trạng thái mới hơn thắng, phép = phép cũ rồi tới phép mới, người chờ gộp, nhóm cũ gộp với nhóm của mục mới.
- Lần lưu đơn lẻ mới hơn của một thành viên khi nhóm còn chờ không tách nhóm (mục vẫn là mục đó).
- Một mục đang được ghi (đã lấy khỏi hàng chờ) không còn là thành viên nhóm nào trong hàng chờ; lần lưu tới sau đó là mục mới, độc lập. Giao dịch đang ghi đã chứa thay đổi của nhóm nên điều này đúng.
- `Stop` ghi hết mọi mục và nhóm; ngữ cảnh bị hủy thì mọi người chờ của mục và nhóm nhận lỗi đúng một lần.

**Zone:** `zone.Saver` thêm `SaveLinked(states []store.CharState, done chan<- error)`.

## 4. Server

### 4.1. Trạng thái trong zone

Mỗi người chơi thêm:

- `tradeInvite`: lời mời đến đang chờ (id người mời, tick hết hạn) hoặc không có; `tradeOut`: id người mình đang mời hoặc 0.
- `trade *trade`: phiên giao dịch chung của hai người. Mỗi bên có: các ô đã đặt (theo thứ tự lệnh), bản sao sâu của món ở các ô đó lúc đặt, Xu đặt, `locked`, `confirmed`. Người mời là bên A.
- `stall *stall`: tên sạp và các mục `{index, ô hành trang, số lượng còn, giá đơn vị}`; `index` là thứ tự trong lệnh bày (0…n−1), không đổi khi mục khác bán hết.

`busy(p)` khi `trade` hoặc `stall` khác rỗng. `itemLock(p)` trả `IN_TRADE` khi có `trade`, `IN_STALL` khi có `stall`, rỗng khi không.

Không gì trong số này đi qua bản trao tay: rời zone thì giao dịch hủy, sạp đóng, lời mời bị bỏ (mục 4.8, 4.11).

Mọi lệnh của mục 4 xử lý trong `drain`. Gói đẩy (`TradeInvite`, `TradeState`, `TradeClosed`, `StallSold`) gửi ngay trong lệnh, trước phản hồi của lệnh.

### 4.2. Mời giao dịch

`TradeInviteReq{target_id}` của A. Lỗi đầu tiên được trả:

1. A chết: `DEAD`.
2. `target_id` là A, không phải người chơi trong zone này, hoặc người đó đã chết: `INVALID_TARGET`.
3. Cấp A hoặc cấp B < 10: `LEVEL_TOO_LOW`.
4. A hoặc B không đứng trong khu an toàn: `NOT_IN_SAFE_ZONE`.
5. Khoảng cách A–B > 5 ô: `OUT_OF_RANGE`.
6. A bận, B bận, hoặc B đang có lời mời giao dịch chưa hết hạn: `TRADE_BUSY`.

Hợp lệ: bỏ lời mời đi cũ của A (nếu có); ghi lời mời đến của B (A, hết hạn sau 30 giây) và `tradeOut` của A; đẩy cho B `TradeInvite{from_id, from_name, from_level, expires_ms = 30000}`; trả `TradeInviteResp{}`.

### 4.3. Trả lời lời mời

`TradeAnswerReq{from_id, accept}` của B:

1. B không có lời mời đến chưa hết hạn từ `from_id`: `TRADE_NOT_FOUND`.

Từ đây lời mời bị xóa (cả `tradeOut` của A nếu trỏ tới B) dù kết quả thế nào.

- **Từ chối:** đẩy cho A (nếu còn trong zone) `TradeClosed{DECLINED, tên B}`; trả `TradeAnswerResp{}`.
- **Đồng ý**, lỗi đầu tiên được trả:
  2. B chết: `DEAD`.
  3. A không còn trong zone hoặc đã chết: `TRADE_NOT_FOUND`.
  4. Các bước 3–6 của mục 4.2 (cấp, khu an toàn, khoảng cách 5 ô, bận).

  Hợp lệ: mở giao dịch với hai phần rỗng; đẩy `TradeState` cho cả hai; trả `TradeAnswerResp{}`.

### 4.4. Đặt món và Xu

`TradeOfferReq{bag_slots, xu}` của P:

1. P không ở trong giao dịch: `TRADE_NOT_FOUND`.
2. Quá 8 ô, có ô trùng, ô ngoài [0, kích thước hành trang), hoặc `xu` < 0: `BAD_REQUEST`.
3. Một ô rỗng: `ITEM_NOT_FOUND`.
4. Một món có `no_trade`: `NOT_TRADABLE`.
5. `xu` lớn hơn Xu của P: `NOT_ENOUGH_XU`.

Hợp lệ: thay toàn bộ phần của P (ô, bản sao món, Xu); **cả hai bên** `locked` và `confirmed` về sai; đẩy `TradeState` cho cả hai; trả `TradeOfferResp{}`. Lệnh với đúng phần đang có vẫn làm mất khóa (client không gửi khi không đổi).

### 4.5. Khóa

`TradeLockReq{}` của P: không ở trong giao dịch thì `TRADE_NOT_FOUND`. Hợp lệ: `locked` của P đúng (đã đúng thì giữ); đẩy `TradeState` cho cả hai; trả `TradeLockResp{}`. Không có lệnh mở khóa riêng: đặt lại phần của mình là mở khóa.

### 4.6. Xác nhận

`TradeConfirmReq{}` của P:

1. P không ở trong giao dịch: `TRADE_NOT_FOUND`.
2. Một trong hai bên chưa khóa: `TRADE_NOT_LOCKED`.

Hợp lệ: `confirmed` của P đúng. Bên kia chưa xác nhận: đẩy `TradeState` cho cả hai, trả `TradeConfirmResp{}`. Bên kia đã xác nhận: thực hiện (mục 4.7).

### 4.7. Thực hiện

P là người xác nhận sau, Q là bên kia.

1. **Kiểm lại** (bất biến, không xảy ra khi luật mục 4.9 đúng): mỗi ô đã đặt vẫn chứa món bằng bản sao lúc đặt và Xu mỗi bên ≥ Xu đã đặt. Sai thì ghi log lỗi, hủy giao dịch (`TradeClosed{CANCELLED}` cho cả hai) và trả `INTERNAL`.
2. **Dựng trên bản sao:** hành trang mới của P = bản sao hành trang P, làm rỗng các ô P đặt, rồi thêm các món Q đặt theo thứ tự (luật thêm món của 3a). Không chứa hết thì `BAG_FULL`. Làm như vậy cho Q; không chứa hết thì `PARTNER_BAG_FULL`. Khi lỗi: không đổi gì, cả hai bên `locked` và `confirmed` về sai, đẩy `TradeState` cho cả hai, trả lỗi cho P.
3. **Áp:** thay hai hành trang bằng bản mới; Xu của P += Xu Q đặt − Xu P đặt, Xu của Q ngược lại; xóa kết quả tẩy luyện chờ của cả hai; đóng giao dịch; `statsDirty` cho cả hai.
4. **Lưu:** `SaveLinked([state(A) kèm TradeLog, state(B)], nil)` với `TradeLog{kind: "trade", A, B, AItems/BItems: các món mỗi bên trao (ô gốc), AXu, BXu, Tax: 0}`.
5. **Báo:** đẩy cho mỗi người `TradeClosed{DONE, tên bên kia, update, stats}` (`update`: mọi ô hành trang của người đó đã đổi; `stats` có Xu mới); trả `TradeConfirmResp{}` cho P.

### 4.8. Hủy

- `TradeCancelReq{}` của P: không ở trong giao dịch thì `TRADE_NOT_FOUND`. Hợp lệ: đóng giao dịch, đẩy cho bên kia `TradeClosed{CANCELLED, tên P}`, trả `TradeCancelResp{}`.
- **Mỗi tick**, bước `runTrades` (ngay sau `movePlayers`): giao dịch có hai người cách nhau quá 8 ô hoặc một người ngoài khu an toàn thì đóng, đẩy cho cả hai `TradeClosed{TOO_FAR}`; có một người đã chết thì đóng, người còn lại nhận `TradeClosed{LEFT, tên người kia}`.
- **Rời zone** (thoát, mất kết nối, qua cổng, đổi khu, giấy về thành, xa phu): trước khi dựng bản trao tay hoặc lưu lần cuối, zone đóng giao dịch của người đó (người kia nhận `TradeClosed{LEFT, tên}`), xóa lời mời đến và đi liên quan tới người đó, đóng sạp (mục 4.11).

### 4.9. Khóa đồ khi giao dịch hoặc bày sạp

Các lệnh sau, ngay sau bước `DEAD` của chúng (hoặc là bước đầu khi lệnh không có bước `DEAD`), trả `itemLock(p)` nếu khác rỗng: `PickupReq`, `EquipReq`, `UnequipReq`, `DiscardReq`, `SortBagReq`, `UseItemReq`, `ReturnTownReq`, `BuyReq`, `SellReq`, `TravelReq`, `StashDepositReq`, `StashWithdrawReq`, `StashExpandReq`, `StashSortReq`, `EnhanceReq`, `PunchReq`, `SocketReq`, `UnsocketReq`, `RefineReq`, `RefineChooseReq`, `QuestTurnInReq`, `LearnSkillReq`, `StallBuyReq` (người mua). `StashOpenReq`, `QuestAcceptReq`, qua cổng, đổi khu vẫn được (hai lệnh sau làm hủy giao dịch và đóng sạp). Mở sạp và mời giao dịch khi đang bận trả `TRADE_BUSY` (mục 4.2, 4.10).

### 4.10. Bày sạp

`StallOpenReq{title, entries}` của P:

1. P chết: `DEAD`.
2. Map không phải `town`: `STALL_FORBIDDEN`.
3. P không đứng trong khu an toàn: `NOT_IN_SAFE_ZONE`.
4. Cấp P < 10: `LEVEL_TOO_LOW`.
5. P bận: `TRADE_BUSY`.
6. P cách một NPC, một điểm xuất hiện hoặc một hình chữ nhật cổng dưới 2 ô, hoặc cách một sạp đang mở khác dưới 1,5 ô: `STALL_FORBIDDEN`.
7. `textutil.Clean(title, 20)` sai: `BAD_REQUEST`.
8. Không có mục hoặc quá 10 mục; một ô ngoài phạm vi hoặc trùng; giá ngoài 1–1.000.000.000: `BAD_REQUEST`.
9. Một ô rỗng: `ITEM_NOT_FOUND`.
10. Một món có `no_trade`: `NOT_TRADABLE`.
11. `count`: 0 nghĩa là cả ô; trang bị chỉ được 0 hoặc 1; vật phẩm chồng 0 hoặc 1 tới số lượng của ô; sai thì `BAD_REQUEST`.

Hợp lệ: mở sạp với tên đã làm sạch và các mục theo thứ tự lệnh (`count` 0 đổi thành số lượng ô); xóa lời mời giao dịch đến và đi của P; P dừng (hướng 0); ghi P vào danh sách đổi ngoại hình của tick (`LookChange` có `stall_title`); trả `StallOpenResp{}`.

Khi sạp mở, `MoveInput` của P không đổi hướng đi (P đứng yên) nhưng vẫn cập nhật `ack_input_seq` để dự đoán của client khớp lại.

### 4.11. Đóng sạp

- `StallCloseReq{}` của P: không có sạp thì `STALL_NOT_FOUND`. Hợp lệ: đóng, ghi đổi ngoại hình, trả `StallCloseResp{}`.
- Sạp cũng đóng khi P rời zone và khi mục cuối cùng bán hết (mục 4.13).

### 4.12. Xem sạp

`StallViewReq{owner_id}` của V:

1. V chết: `DEAD`.
2. `owner_id` không phải người chơi trong zone có sạp đang mở: `STALL_NOT_FOUND`.
3. Khoảng cách V–chủ sạp > 6,5 ô: `OUT_OF_RANGE`.

Hợp lệ: `StallViewResp{owner_id, owner_name, title, entries}`, mỗi mục có `index`, món (bản sao, `count` là số còn bày) và `price`. Người bán xem sạp của chính mình cũng được.

### 4.13. Mua ở sạp

`StallBuyReq{owner_id, index, count, price}` của V:

1. V chết: `DEAD`.
2. `itemLock(V)` khác rỗng: `IN_TRADE` hoặc `IN_STALL`.
3. Cấp V < 10: `LEVEL_TOO_LOW`.
4. `owner_id` không phải người chơi trong zone có sạp đang mở: `STALL_NOT_FOUND`.
5. Khoảng cách > 6,5 ô: `OUT_OF_RANGE`.
6. `count` < 1: `BAD_REQUEST`.
7. Không còn mục `index`, giá mục khác `price`, hoặc `count` lớn hơn số còn bày: `STALL_CHANGED`.
8. Xu của V < `price × count`: `NOT_ENOUGH_XU`.
9. Hành trang V không chứa hết `count` đơn vị của món: `BAG_FULL`.

Hợp lệ, với `total = price × count` (≤ 10¹² nên vừa `int64`) và `(guild, bp) = Config.StallTax()` (hàm rỗng là `(0, 0)`):

- `tax = ⌊total × bp / 10.000⌋` khi `guild ≠ 0`, ngược lại 0.
- Bớt `count` khỏi ô của người bán (hết thì ô rỗng), thêm vào hành trang V; Xu V −= `total`; Xu người bán += `total − tax`; xóa kết quả tẩy luyện chờ của người bán; `statsDirty` cho cả hai.
- Số còn của mục giảm; về 0 thì bỏ mục (các `index` khác giữ nguyên); không còn mục thì sạp đóng (ghi đổi ngoại hình).
- `SaveLinked([state(người bán) kèm TradeLog{kind: "stall", A: người bán, B: V, AItems: món đã bán, BXu: total, Tax: tax}, state(V)], nil)`. 7h thêm phép `GuildFundTax` vào trạng thái người bán khi `tax > 0`.
- Đẩy cho người bán `StallSold{index, count, buyer_name, xu = total − tax, tax, closed, update, stats}`; trả V `StallBuyResp{update, stats}`.

### 4.14. Hiển thị

- `EntityEnter.stall_title` của người chơi là tên sạp đang mở, rỗng khi không bày sạp.
- `LookChange` (toàn bộ ngoại hình, 5a) thêm `stall_title`.

### 4.15. Thứ tự tick

`drain → runImpacts → resolveAttacks → movePlayers → runTrades → thinkMonsters → …` (các bước sau giữ nguyên). `runTrades` chỉ xét các giao dịch đang mở.

### 4.16. Gateway

Mười lệnh `TradeInviteReq`, `TradeAnswerReq`, `TradeOfferReq`, `TradeLockReq`, `TradeConfirmReq`, `TradeCancelReq`, `StallOpenReq`, `StallCloseReq`, `StallViewReq`, `StallBuyReq` cần đã vào thế giới (`NOT_IN_WORLD`), chỉ chịu giới hạn gói chung, đi vào zone qua `Membership` (hộp thư zone đầy thì `RATE_LIMITED`), như lệnh nhặt đồ.

## 5. Giao thức

`ProtocolVersion` lên **17**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 135 | `TradeInviteReq` | `uint64 target_id = 1` |
| 136 | `TradeInviteResp` | rỗng |
| 137 | `TradeInvite` | `uint64 from_id = 1`, `string from_name = 2`, `int32 from_level = 3`, `int32 expires_ms = 4` |
| 138 | `TradeAnswerReq` | `uint64 from_id = 1`, `bool accept = 2` |
| 139 | `TradeAnswerResp` | rỗng |
| 140 | `TradeOfferReq` | `repeated int32 bag_slots = 1`, `int64 xu = 2` |
| 141 | `TradeOfferResp` | rỗng |
| 142 | `TradeLockReq` | rỗng |
| 143 | `TradeLockResp` | rỗng |
| 144 | `TradeConfirmReq` | rỗng |
| 145 | `TradeConfirmResp` | rỗng |
| 146 | `TradeCancelReq` | rỗng |
| 147 | `TradeCancelResp` | rỗng |
| 148 | `TradeState` | `uint64 partner_id = 1`, `string partner_name = 2`, `TradeSide mine = 3`, `TradeSide theirs = 4` |
| 149 | `TradeClosed` | `TradeCloseReason reason = 1`, `string name = 2`, `InventoryUpdate update = 3`, `SelfStats stats = 4` |
| 150 | `StallOpenReq` | `string title = 1`, `repeated StallOffer entries = 2` |
| 151 | `StallOpenResp` | rỗng |
| 152 | `StallCloseReq` | rỗng |
| 153 | `StallCloseResp` | rỗng |
| 154 | `StallViewReq` | `uint64 owner_id = 1` |
| 155 | `StallViewResp` | `uint64 owner_id = 1`, `string owner_name = 2`, `string title = 3`, `repeated StallEntry entries = 4` |
| 156 | `StallBuyReq` | `uint64 owner_id = 1`, `int32 index = 2`, `int32 count = 3`, `int64 price = 4` |
| 157 | `StallBuyResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 158 | `StallSold` | `int32 index = 1`, `int32 count = 2`, `string buyer_name = 3`, `int64 xu = 4`, `int64 tax = 5`, `bool closed = 6`, `InventoryUpdate update = 7`, `SelfStats stats = 8` |

Số 159 để trống.

```proto
// One offered bag slot. bag_slot is the owner's slot.
message TradeItem {
  int32 bag_slot = 1;
  ItemInfo item = 2;
}

// One side of an open trade.
message TradeSide {
  repeated TradeItem items = 1;
  int64 xu = 2;
  bool locked = 3;
  bool confirmed = 4;
}

enum TradeCloseReason {
  TRADE_CLOSE_REASON_UNSPECIFIED = 0;
  TRADE_CLOSE_REASON_DONE = 1;      // items and Xu changed hands
  TRADE_CLOSE_REASON_CANCELLED = 2; // name cancelled
  TRADE_CLOSE_REASON_DECLINED = 3;  // name declined your invite
  TRADE_CLOSE_REASON_TOO_FAR = 4;   // more than 8 tiles apart or out of the safe zone
  TRADE_CLOSE_REASON_LEFT = 5;      // name left the zone or died
}

// One stall line when opening. count 0 offers the whole slot.
message StallOffer {
  int32 bag_slot = 1;
  int32 count = 2;
  int64 price = 3; // Xu per unit
}

// One stall line as buyers see it; item.count is the count still offered.
message StallEntry {
  int32 index = 1;
  ItemInfo item = 2;
  int64 price = 3;
}
```

- `TradeInvite`, `TradeState`, `TradeClosed`, `StallSold` đẩy với `seq = 0`.
- `EntityEnter.stall_title = 16` (`string`), `LookChange.stall_title = 4` (`string`); ghi chú: rỗng khi không bày sạp.
- `ErrorCode`: `TRADE_BUSY = 61` (already trading, has a stall open, or the target has a pending trade invite), `TRADE_NOT_FOUND = 62` (no open trade, or no pending invite from that player), `TRADE_NOT_LOCKED = 63`, `NOT_TRADABLE = 64` (Lượng, a `no_trade` item), `IN_TRADE = 65`, `IN_STALL = 66`, `STALL_FORBIDDEN = 67` (not a town, or too close to an NPC, a spawn point, a portal or another stall), `STALL_NOT_FOUND = 68`, `STALL_CHANGED = 69` (the line was sold, its price differs, or fewer units are left), `PARTNER_BAG_FULL = 70`. Số 71–72 để trống. Ghi chú của `NOT_IN_SAFE_ZONE` đổi thành "the trade or stall needs a safe zone" (không còn "unused").
- Fixture mới: `trade_state.hex` (hai món bên mình, Xu bên kia, bên mình đã khóa), `trade_closed_done.hex` (có `update`, `stats`), `stall_open_req.hex`, `stall_view_resp.hex` (một chồng đá, một kiếm Vàng), `stall_sold.hex`, `entity_enter_stall.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/trade.gd`** (mới): lời mời đang chờ (`set_invite(msg, now)`, `invite_left(now)`, `clear_invite()` như `party.gd`); `apply_state(TradeState)`; `close(reason, name) -> String` (câu mục 6.3) và xóa phiên; `active()`, `partner_id`, `mine`, `theirs`; `draft_slots` (ô đang chọn trước khi gửi) và `draft_xu`; `can_add(slot, inventory, item_book) -> String` (`""`, `"no_trade"`, `"full"` khi đã 8 ô); `action() -> String` (`"lock"` khi mình chưa khóa, `"confirm"` khi cả hai đã khóa và mình chưa xác nhận, `"wait"` khi còn lại); `theirs_changed(now) -> bool` (đúng trong 3 giây sau khi phần bên kia đổi so với `TradeState` trước) để giao diện tô đỏ.
- **`core/stall.gd`** (mới): bản nháp sạp (`add(slot, count, price)`, `remove(slot)`, `entries`); `validate(title, inventory, item_book) -> String` (`""`, `"title"`, `"empty"`, `"full"`, `"price"`, `"no_trade"`); `place_block(map_type, in_safe, pos, npcs, spawns, portals, stalls) -> String` (luật bước 2, 3, 6 của mục 4.10; server vẫn là chuẩn); mô hình xem sạp `apply_view(StallViewResp)`, `total(index, count)`.
- **`core/item_book.gd`**: `no_trade(code) -> bool`.
- **`core/inventory.gd`**: `locked_slots` (ô đang đặt trong giao dịch hoặc đang bày) để giao diện tắt nút thao tác.
- **`core/errors.gd`**: TRADE_BUSY "Người này đang bận giao dịch", TRADE_NOT_FOUND "Giao dịch không còn hiệu lực", TRADE_NOT_LOCKED "Cả hai bên phải khóa trước khi xác nhận", NOT_TRADABLE "Vật phẩm này không giao dịch được", IN_TRADE "Đang giao dịch", IN_STALL "Đang bày sạp", STALL_FORBIDDEN "Không thể bày sạp ở đây", STALL_NOT_FOUND "Sạp đã đóng", STALL_CHANGED "Sạp đã thay đổi, hãy xem lại", PARTNER_BAG_FULL "Hành trang bên kia không đủ chỗ"; NOT_IN_SAFE_ZONE "Cần đứng trong khu an toàn".
- **`core/wire.gd`**: 24 payload mới. Mô hình giao dịch ở `Router.trade` (xóa khi màn World nạp lại vì chuyển zone làm hủy giao dịch).

### 6.2. Giao diện

- **Thẻ người chơi** (5b): nút **Giao dịch** khi cả hai trong khu an toàn, cùng cấp ≥ 10 và mình không bận; nút **Xem sạp** khi người đó đang bày sạp.
- **Lời mời** (`ui/invite_popup.gd` của 5b, thêm loại "giao dịch"): "<tên> (Lv n) muốn giao dịch với bạn", đếm ngược, Đồng ý / Từ chối.
- **Cửa sổ giao dịch** (`ui/trade_window.gd`, mới): hai cột "Bạn" và "<tên>", mỗi cột 8 ô món và dòng Xu, nhãn "Đã khóa" / "Đã xác nhận"; cột của mình chạm ô trống mở hành trang để chọn món (món `no_trade` và đồ đang mặc xám); ô nhập Xu; nút **Khóa** / **Xác nhận** theo `action()`, nút **Hủy**. Phần bên kia vừa đổi thì viền đỏ nhấp nháy 3 giây và dòng "Bên kia vừa thay đổi". Chạm món mở thẻ món (3a) để xem đủ dòng. Bấm Xác nhận hỏi lại với tóm tắt "Bạn đưa … · Bạn nhận …".
- **Bày sạp** (`ui/stall_setup.gd`, mới): mở từ nút **Bày sạp** của tab Hành trang (chỉ hiện ở map `town`); chọn món, số lượng (chồng), giá đơn vị; ô tên sạp; lỗi theo `validate` và `place_block`. Khi sạp mở, HUD hiện "Đang bày sạp" và nút **Đóng sạp**, cần điều khiển mờ.
- **Xem sạp** (`ui/stall_window.gd`, mới): tên sạp, người bán, các mục (món, số còn, giá đơn vị); chọn số lượng, dòng tổng, nút **Mua** (hỏi lại khi tổng ≥ 100.000 Xu); `STALL_CHANGED` thì tự gửi lại `StallViewReq`.
- **Tên sạp trên đầu** (`ui/entity_view.gd`): khung chữ nhỏ trên tên nhân vật; chạm vào mở xem sạp.
- **Hành trang**: ô đang đặt hoặc đang bày có biểu tượng khóa; nút Mặc, Hủy, Bán… tắt kèm chữ "Đang giao dịch" / "Đang bày sạp".
- **Chat** (7a): `add_local` "Giao dịch với <tên> thành công", "Đã bán <n> <món> cho <tên>, nhận <x> Xu".

### 6.3. Câu thông báo

| `TradeCloseReason` | Câu |
| --- | --- |
| `DONE` | "Giao dịch với <tên> thành công" |
| `CANCELLED` | "<tên> đã hủy giao dịch" |
| `DECLINED` | "<tên> từ chối giao dịch" |
| `TOO_FAR` | "Giao dịch bị hủy: hai bên ở quá xa hoặc rời khu an toàn" |
| `LEFT` | "Giao dịch bị hủy: <tên> đã rời đi" |

### 6.4. Smoke

`cmd/botclient partner` (5b) thêm cờ `-user`, `-pass` để dùng tài khoản và nhân vật có sẵn; khi nhận `TradeInvite` thì đồng ý, khóa khi bên kia đã khóa, xác nhận khi cả hai đã khóa, in `TRADE OK` khi nhận `TradeClosed{DONE}`. `tools/smoke.sh` tạo tài khoản partner, dùng `devchar` đặt partner và nhân vật smoke cấp 10, `devitem -xu 1000` cho nhân vật smoke, rồi chạy partner. `tests/smoke/smoke.gd` (cả hai ở trại Kumotori): mời partner, chờ `TradeState`, đặt 100 Xu, khóa, chờ bên kia khóa, xác nhận, chờ `TradeClosed{DONE}` và Xu giảm đúng 100. In "SMOKE OK" kèm "trade 100 Xu".

## 7. Lệnh dev

Không có lệnh mới ngoài cờ `-user`/`-pass` của `botclient partner`. Tra nhật ký bằng SQL trên `trade_logs` (README ghi câu truy vấn mẫu theo tên nhân vật).

## 8. Kiểm thử

**Server** (`go test -race`):

- `gamedata`: dữ liệu thật có `no_trade` ở đúng 25 sách và không ở món nào khác; thiếu trường thì `false`.
- `saver` (writer giả ghi lại từng lần gọi):
  - `SaveLinked(A, B)` ghi A và B trong **một** lần gọi; người chờ nhận `nil` đúng một lần.
  - Hai lần `Save` của A mang hai phép: một lần gọi, trạng thái mới nhất, đủ hai phép theo thứ tự.
  - Writer lỗi lần đầu: lần thử lại vẫn có đủ phép và cả nhóm; không gọi nào chỉ có một nửa nhóm.
  - `Save` đơn lẻ mới hơn của A khi nhóm (A, B) còn chờ: vẫn một lần gọi chứa A mới và B.
  - `SaveLinked(A, B)` rồi `SaveLinked(B, C)`: một lần gọi chứa A, B, C.
  - Nhóm lỗi trong lúc có `Save` mới của A: lần thử lại gộp, trạng thái mới, phép cũ rồi mới.
  - `Stop` ghi hết nhóm; ngữ cảnh hủy thì người chờ của mục và nhóm nhận lỗi một lần.
- `store` (DB): `SaveCharacterStates` hai nhân vật một giao dịch; một phép giả trả lỗi làm cả hai nhân vật giữ nguyên trong DB (mô phỏng sập giữa chừng: không có nửa giao dịch); `TradeLog` chèn đúng một dòng với `jsonb` đủ thuộc tính món; nhân vật đã xóa vẫn chạy phép; migration lên rồi xuống.
- `zone` (saver giả ghi lại `SaveLinked`):
  - Mời, trả lời: thứ tự lỗi mục 4.2, 4.3; lời mời hết hạn sau 30 giây là `TRADE_NOT_FOUND`; mời người thứ hai thì lời mời đầu bị bỏ; từ chối đẩy `DECLINED`.
  - Đặt: thứ tự lỗi mục 4.4; sách bị `NOT_TRADABLE`; đặt lại làm mất khóa và xác nhận của cả hai.
  - Xác nhận khi bên kia chưa khóa là `TRADE_NOT_LOCKED`; khóa, đổi phần, khóa lại, xác nhận cả hai thì đồ và Xu đổi đúng, chồng đá dồn vào chồng có sẵn, một lần `SaveLinked` chứa hai trạng thái và đúng một `TradeLog`.
  - Hành trang bên nhận đầy: `BAG_FULL` cho người bấm khi đó là hành trang của mình, `PARTNER_BAG_FULL` khi là của bên kia; không đổi gì; cả hai mất khóa.
  - Trong giao dịch: nhặt, mặc, hủy, sắp xếp, dùng bình, bán, gửi kho, cường hóa, trả nhiệm vụ đều `IN_TRADE`; sau khi đóng thì được.
  - Đi xa quá 8 ô thì cả hai nhận `TOO_FAR` trong tick đó; qua cổng thì bên kia nhận `LEFT` và bản trao tay không mang giao dịch; thoát game cũng vậy.
  - Sạp: thứ tự lỗi mục 4.10; map luyện cấp bị `STALL_FORBIDDEN`; sát NPC, sát sạp khác bị `STALL_FORBIDDEN`; `MoveInput` không làm người bán đi nhưng `ack_input_seq` tăng; `LookChange` có `stall_title` khi mở và rỗng khi đóng; người khác vào tầm nhìn thấy `EntityEnter.stall_title`.
  - Mua: thứ tự lỗi mục 4.13; mua 4 trong chồng 10 giá 20: người mua −80 Xu, +4 đá, người bán +80 Xu, chồng còn 6, mục còn 6; giá sai là `STALL_CHANGED`; mua phần còn lại thì mục bỏ, sạp đóng khi hết mục, `StallSold.closed` đúng; một `SaveLinked` với `TradeLog` kind `stall`; `StallTax` giả trả (7, 500) thì người bán nhận 95% và `tax` ghi đúng; người bán bị `IN_STALL` khi hủy đồ.
- `gateway`: `ProtocolVersion` 17; fixture mới; mười lệnh khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestTradeOverWebSocket`: A, B cấp 10 cạnh nhau ở Tổng hành dinh (`DevSetLevel`, `DevSetPlace`), A có kiếm Vàng và 1.000 Xu, B có 10 đá và một sách; A mời, B đồng ý; B đặt sách bị `NOT_TRADABLE`; A đặt kiếm và 500 Xu, B đặt 10 đá; A khóa, B khóa; B đặt lại 5 đá (cả hai mất khóa); A xác nhận bị `TRADE_NOT_LOCKED`; khóa lại cả hai, xác nhận cả hai; cả hai nhận `TradeClosed{DONE}`; A nhặt đồ trong lúc giao dịch thứ hai mở bị `IN_TRADE`; thoát cả hai, `GetCharacter` cho đúng đồ và Xu mới; có một dòng `trade_logs`.
- e2e `TestStallOverWebSocket`: A bày sạp "Đá rẻ" với 10 đá giá 20 và kiếm giá 5.000; B xem, mua 4 đá, mua với giá 19 bị `STALL_CHANGED`; A nhận `StallSold` và +80 Xu; A gửi `MoveInput` mà vị trí không đổi; A đóng sạp; B mua tiếp bị `STALL_NOT_FOUND`; DB đúng sau khi thoát.

**Client** (`tests/run.sh`, mục `# ____TEST TRADE____`): `trade` (áp `TradeState`; `action()` qua các bước khóa/xác nhận; đặt lại làm `theirs_changed`; `can_add` với sách và với 8 ô; lời mời hết giờ; câu đóng đủ năm lý do); `stall` (`validate` các lỗi; `place_block` ở map luyện cấp, sát NPC, sát sạp khác; `total`); `item_book.no_trade`; giải mã fixture mới. Smoke theo mục 6.4.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito) và `test2` (Hana) ở Tổng hành dinh: Kaito giao dịch một món và 1.000 Xu lấy đá của Hana; Hana đổi phần sau khi Kaito khóa thì Kaito thấy viền đỏ và phải khóa lại; xong thì đồ và Xu đúng sau khi thoát vào lại. Kaito bày sạp vài viên đá; Hana chạm tên sạp, mua 3 viên; Kaito thấy "Đã bán…" trong chat; Kaito không đi được cho tới khi đóng sạp.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 7.2: luật giao dịch trực tiếp (cùng khu, khu an toàn, cấp 10, 8 món, khóa/xác nhận, Lượng và sách không giao dịch) và sạp (Tổng hành dinh, online, 10 món, giá đơn vị, thuế sạp do 7h); mục 5.4 giữ "sạp giao dịch" ở Tổng hành dinh.
- Spec Giai đoạn 0 mục 3.3: bảng `trade_logs`.
- `CLAUDE.md` (server): mọi thay đổi chạm hai nhân vật, hoặc một nhân vật và một bảng khác, phải nằm trong một giao dịch DB qua `Saver.SaveLinked` và `CharState.Ops`; saver nối thêm phép ghi kèm khi gộp, không bao giờ bỏ; người đang giao dịch hay bày sạp không đổi được hành trang (`itemLock`).
- `CLAUDE.md` (client): mô hình giao dịch ở `core/trade.gd`, sạp ở `core/stall.gd`.
- README hai repo: giao dịch, sạp, `botclient partner -user/-pass`, câu truy vấn `trade_logs`.
