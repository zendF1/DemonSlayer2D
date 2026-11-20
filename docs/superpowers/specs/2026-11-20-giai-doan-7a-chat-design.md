# Giai đoạn 7a — Chat

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần đầu của Giai đoạn 7 (xem bảng chia ở [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md) mục 1). Game-design không có mục riêng cho chat; nó là nền cho mọi nội dung cộng đồng của mục 6 và 7 (bang hội, sự kiện, giao dịch) và cho tổ đội (mục 5.8). Những gì không nhắc tới ở đây giữ nguyên như [spec 5b](2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md) (dịch vụ tổ đội, ô id tổ đội, thẻ người chơi) và [spec 6d](2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md). 7a chạy sau khi Giai đoạn 6 đã triển khai (giao thức 15, migration 00009).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Trên HUD có một dòng chat hiện hai tin gần nhất; chạm vào thì mở cửa sổ chat với các thẻ **Chung**, **Thế giới**, **Khu**, **Tổ đội**, **Riêng**. Người chơi gõ tin vào kênh Thế giới (mọi người đang online, từ cấp 10), kênh Khu (mọi người cùng map và cùng khu), kênh Tổ đội, hoặc nhắn riêng theo tên nhân vật. Server phát thông báo hệ thống (sự kiện, tin vận hành) trên kênh Hệ thống. Chạm tên người gửi để nhắn riêng, xem thẻ người chơi hoặc chặn người đó. Mỗi kênh có giới hạn tần suất; tin không lưu lại.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chat sống ở đâu | Dịch vụ `internal/chat` ở mức world, một khóa, chỉ trong bộ nhớ; đẩy tin thẳng vào hộp gửi của phiên; không bao giờ gọi vào world hay zone | Khuôn của tổ đội (5b) và tổng quan mục 3.1; tin chat đi qua nhiều map và khu, không thuộc zone nào; zone không phải nhận thêm lệnh |
| Kênh | `WORLD` (Thế giới), `ZONE` (Khu), `PARTY` (Tổ đội), `PRIVATE` (Riêng), `SYSTEM` (Hệ thống, chỉ server gửi). Số `6` để dành cho `GUILD` của 7c | Đủ cho v1; kênh bang đến cùng bang hội |
| Kênh Khu | Mọi người cùng map **và** cùng khu với người gửi | Khu là nhóm người thật sự thấy nhau (game-design 5.3); kênh theo cả map gộp các khu không có ích bằng |
| Kênh Thế giới | Từ cấp **10**; mỗi nhân vật 1 tin mỗi **10 giây**; toàn server tối đa **5 tin mỗi giây** | Chặn nhân vật rác vừa tạo; kênh chung đọc được khi ít tin; 5 tin/giây cho 1.000 người là 5.000 gói/giây, server chịu được |
| Kênh khác | Khu: 1 tin mỗi **2 giây**; Tổ đội và Riêng: 1 tin mỗi **giây** | Đủ để nói chuyện, chặn spam |
| Cách đếm tần suất | `auth.Limiter` (cửa sổ trượt) với `Take`, hoàn lại bằng `Refund` khi phần đếm sau (giới hạn toàn server) từ chối; đếm theo id nhân vật | Bất biến của CLAUDE.md: kiểm và ghi trong một lần gọi; theo nhân vật nên thoát vào lại không làm mới |
| Thứ tự kiểm | Mọi kiểm tra nội dung, cấp, tổ đội, người nhận chạy **trước** khi đếm tần suất | Tin bị từ chối không làm mất lượt |
| Nhắn riêng | Theo **tên nhân vật**, không phân biệt hoa thường và cách gõ dấu (khóa giống `name_key`); người nhận phải đang online; không nhắn cho chính mình | Tên là thứ người chơi biết; không có thư ngoại tuyến ở v1 |
| Người gửi thấy tin của mình | Server gửi lại tin (echo) cho người gửi, cùng dạng với người nhận, trước phản hồi | Client chỉ hiện tin server đã nhận; thứ tự tin trong kênh giống mọi người |
| Nội dung | Chuẩn hóa NFC, bỏ ký tự điều khiển và ký tự định dạng (Unicode Cc, Cf), gộp mọi khoảng trắng liền nhau thành một dấu cách, cắt hai đầu; còn 1–**100** ký tự; chuỗi gốc dài quá 1.024 byte bị từ chối ngay | Một dòng trên điện thoại; không để chèn xuống dòng hay ký tự ẩn đảo chiều chữ |
| Lọc từ ngữ | Không làm ở v1 | Cần danh sách và quy trình vận hành; tổng quan mục 4 |
| Lịch sử | Server không lưu; client giữ 100 dòng gần nhất mỗi thẻ trong bộ nhớ, còn qua lần nạp lại màn World, mất khi về màn chọn nhân vật | Không cần bảng DB; chat là tin tức thời |
| Chặn người chơi | Ở client: danh sách tên bị chặn lưu `user://chat.cfg` theo nhân vật; tin của người bị chặn (trừ Hệ thống) bị bỏ khi nhận | Không cần server, không cần giao thức; đủ cho v1 |
| Tin hệ thống | Hàm `System(text)` gửi mọi người online, `SystemTo(ids, text)` gửi một nhóm id thực thể; dùng cho sự kiện (7e) và các phần sau | Một đường cho mọi thông báo của server |
| Đăng ký phiên | `World.Enter` đăng ký (id thực thể, id nhân vật, tên, cấp, map, khu, ô id tổ đội, hộp gửi); `Membership.Leave` hủy; `World.Transfer` báo `Moved`; zone báo `Leveled` | Như dịch vụ tổ đội; không trạng thái nào đi qua bản trao tay |
| Gateway | `ChatSendReq` xử lý đồng bộ trên goroutine của kết nối, cần đã vào thế giới, chỉ chịu giới hạn gói chung ngoài giới hạn riêng của chat | Không qua zone; như các lệnh tổ đội |
| Thông báo tổ đội | Vẫn là `PartyNotice` (5b); client ghi thêm một dòng Hệ thống vào chat | Không đổi server; người chơi xem lại được |

### 1.3. Bổ sung cho game-design

Game-design không nói về chat. Spec này chốt các kênh, giới hạn và luật nội dung ở mục 1.2 và ghi một mục ngắn vào game-design (mục 9).

### 1.4. Không làm ở 7a

Kênh bang (7c); lọc từ ngữ; lịch sử chat lưu server; thư ngoại tuyến; bạn bè; báo cáo người chơi; cấm chat từ phía vận hành; biểu tượng cảm xúc riêng; kênh theo cả map; gửi vật phẩm vào chat.

## 2. Dữ liệu

Không có file dữ liệu mới. Hằng số trong code (`internal/chat`):

| Hằng số | Giá trị |
| --- | --- |
| `MaxRunes` | 100 |
| `MaxRawBytes` | 1.024 |
| `WorldMinLevel` | 10 |
| `WorldEvery` | 10 giây (1 tin) |
| `WorldGlobalPerSec` | 5 |
| `ZoneEvery` | 2 giây (1 tin) |
| `PartyEvery`, `PrivateEvery` | 1 giây (1 tin) |

Gói mới `internal/textutil` giữ hàm làm sạch chữ dùng chung (chat ở 7a, tên sạp ở 7b, thông báo bang ở 7c):

`textutil.Clean(s string, maxRunes int) (string, bool)`: chuỗi gốc dài quá `MaxRawBytes` (1.024 byte) thì trả sai; chuẩn hóa NFC; bỏ mọi rune thuộc Cc hoặc Cf; thay mỗi đoạn rune `unicode.IsSpace` liền nhau bằng một dấu cách; cắt hai đầu; trả đúng khi còn 1–`maxRunes` rune. Không sửa rune lạ khác (chữ, số, dấu câu, biểu tượng giữ nguyên).

## 3. Lưu trữ

Không có migration. Không lưu tin, không lưu danh sách chặn ở server.

## 4. Server

### 4.1. Dịch vụ chat

`chat.Service` giữ dưới một khóa:

- `members map[uint64]*member` theo id thực thể; `byKey map[string]uint64` theo khóa tên (khóa của `auth.NormalizeCharName`).
- Mỗi `member`: id thực thể, id nhân vật, tên hiển thị, khóa tên, cấp, map, khu, ô id tổ đội (`*atomic.Uint64` chung với zone, chỉ đọc), hộp gửi (`zone.Outbox`).
- Năm `auth.Limiter`: Thế giới theo nhân vật (1 / 10 giây), Thế giới toàn server (khóa cố định `"*"`, 5 / 1 giây), Khu (1 / 2 giây), Tổ đội (1 / 1 giây), Riêng (1 / 1 giây). Khóa của bộ đếm theo nhân vật là id nhân vật dạng chuỗi.

Hàm:

- `Register(m Member)`, `Unregister(id)`, `Moved(id, mapID, zoneNo)`, `Leveled(id, level)`.
- `Send(from uint64, ch pb.ChatChannel, text, toName string) pb.ErrorCode` (mục 4.2), trả `UNSPECIFIED` khi gửi được.
- `System(text string)`, `SystemTo(ids []uint64, text string)`.

Dịch vụ lấy danh sách người nhận dưới khóa rồi gửi sau khi nhả khóa. Hộp gửi không chặn (gateway tự đóng kết nối chậm như mọi tin khác).

### 4.2. Gửi tin

`ChatSendReq{channel, text, to_name}` của người gửi S. Lỗi đầu tiên được trả:

1. S chưa vào thế giới: `NOT_IN_WORLD` (gateway).
2. `channel` không thuộc `WORLD`, `ZONE`, `PARTY`, `PRIVATE`: `BAD_REQUEST`.
3. `textutil.Clean(text, 100)` sai: `CHAT_TEXT_INVALID`.
4. `WORLD` mà cấp S < 10: `LEVEL_TOO_LOW`.
5. `PARTY` mà ô id tổ đội của S bằng 0: `NOT_IN_PARTY`.
6. `PRIVATE`: `to_name` không qua `auth.NormalizeCharName` hoặc không có ai online với khóa đó: `CHAT_TARGET_OFFLINE`; người đó là S: `BAD_REQUEST`.
7. Tần suất: `Take` của bộ đếm kênh theo nhân vật sai: `CHAT_COOLDOWN`. Với `WORLD`, sau đó `Take` của bộ đếm toàn server; sai thì `Refund` bộ đếm theo nhân vật vừa lấy và trả `CHAT_COOLDOWN`.

Hợp lệ: dựng `ChatMessage{channel, from_id = id thực thể S, from_name = tên S, text = chữ đã làm sạch, to_name}` (`to_name` là tên hiển thị của người nhận, chỉ có ở `PRIVATE`) và gửi với `seq = 0` tới:

| Kênh | Người nhận (luôn gồm S) |
| --- | --- |
| `WORLD` | mọi thành viên |
| `ZONE` | thành viên cùng map và cùng khu với S |
| `PARTY` | thành viên có ô id tổ đội bằng ô của S |
| `PRIVATE` | S và người nhận |

Rồi trả `ChatSendResp{}` cho S (tin echo đi trước phản hồi trong hộp gửi).

### 4.3. Tin hệ thống

`System(text)` gửi `ChatMessage{channel: SYSTEM, text}` (không người gửi) cho mọi thành viên; `SystemTo(ids, text)` cho các id có trong danh sách và đang đăng ký. Không qua `Clean`, không giới hạn tần suất (người gọi là code server).

### 4.4. Đăng ký trong world

- `World.Enter` đăng ký sau khi zone nhận người chơi, cùng chỗ đăng ký tổ đội (5b); ô id tổ đội là ô của 5b.
- `Membership.Leave` hủy đăng ký trước khi rời zone, cùng chỗ hủy đăng ký tổ đội.
- `World.Transfer` thành công gọi `Moved` cùng chỗ với `party.Moved`.
- `zone.Config.Leveled` (5b) nay là một hàm của world gọi cả `party.Leveled` và `chat.Leveled`.
- Đăng nhập trùng: phiên cũ hủy đăng ký khi rời thế giới, phiên mới đăng ký khi vào; `Takeover` giữ thứ tự nên khóa tên không bao giờ trỏ tới hai phiên.

### 4.5. Gateway

`ChatSendReq` cần đã vào thế giới, chỉ chịu giới hạn gói chung (30 gói/giây), gọi `chat.Send` trên goroutine của kết nối với id thực thể của `c.member`, trả `ChatSendResp` hoặc `ErrorResp`.

## 5. Giao thức

`ProtocolVersion` lên **16**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 125 | `ChatSendReq` | `ChatChannel channel = 1`, `string text = 2`, `string to_name = 3` |
| 126 | `ChatSendResp` | rỗng |
| 127 | `ChatMessage` | `ChatChannel channel = 1`, `uint64 from_id = 2`, `string from_name = 3`, `string text = 4`, `string to_name = 5` |

```proto
enum ChatChannel {
  CHAT_CHANNEL_UNSPECIFIED = 0;
  CHAT_CHANNEL_WORLD = 1;   // everyone online, level 10+
  CHAT_CHANNEL_ZONE = 2;    // same map and zone as the sender
  CHAT_CHANNEL_PARTY = 3;   // the sender's party
  CHAT_CHANNEL_PRIVATE = 4; // one character by name
  CHAT_CHANNEL_SYSTEM = 5;  // server notices; clients cannot send
  // 6 is reserved for CHAT_CHANNEL_GUILD (phase 7c).
}

// A chat line. The sender also receives its own line. from_id is 0 and
// from_name empty for SYSTEM; to_name is set only for PRIVATE.
message ChatMessage {
  ChatChannel channel = 1;
  uint64 from_id = 2;
  string from_name = 3;
  string text = 4;
  string to_name = 5;
}
```

- `ChatMessage` đẩy với `seq = 0`.
- `ErrorCode`: `CHAT_TEXT_INVALID = 57` (empty after cleaning, longer than 100 characters, or raw text over 1024 bytes), `CHAT_TARGET_OFFLINE = 58` (no online character with that name), `CHAT_COOLDOWN = 59` (the channel's send limit is reached). Số 60 để trống.
- Mỗi mã có một dòng chú thích. Tên an toàn với GDScript (`text` không phải từ khóa; không có trường `message`).
- Fixture mới: `chat_send_req.hex` (riêng, có `to_name`), `chat_message.hex` (kênh Khu), `chat_message_system.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/chat_log.gd`** (mới):
  - Thẻ: `"all"`, `"world"`, `"zone"`, `"party"`, `"private"`. Mỗi thẻ giữ tối đa 100 dòng; `"all"` nhận mọi kênh kể cả Hệ thống; Hệ thống chỉ hiện ở `"all"`.
  - `add(msg: Dictionary, self_id: int) -> Dictionary`: bỏ (trả `{}`) nếu người gửi bị chặn và kênh khác Hệ thống; nếu không thì thêm vào thẻ của kênh và `"all"`, tăng số chưa đọc của các thẻ không đang mở, trả dòng `{channel, from_id, from_name, to_name, text, outgoing}` (`outgoing` khi `from_id == self_id`).
  - `add_local(text)`: dòng Hệ thống do client tự tạo (thông báo tổ đội, sau này giao dịch).
  - `lines(tab)`, `unread(tab)`, `open(tab)` (xóa chưa đọc), `last(n)`.
  - `block(name)`, `unblock(name)`, `is_blocked(name)`, so sánh theo khóa thường hóa giống server (`to_lower` sau chuẩn hóa NFC của Godot `String`).
  - `clean(text) -> String` theo luật `textutil.Clean` (để khóa nút Gửi khi rỗng hoặc quá 100 ký tự).
  - `cooldown_left(channel, now) -> float` và `mark_sent(channel, now)`: chặn bấm Gửi trong khung giới hạn của mục 2 (server vẫn là chuẩn).
  - `prefix(line) -> String`: "[Thế giới]", "[Khu]", "[Tổ đội]", "[Riêng] Tới <tên>:" hoặc "[Riêng] <tên>:", "[Hệ thống]".
- **`core/errors.gd`**: `CHAT_TEXT_INVALID` "Tin nhắn rỗng hoặc dài quá 100 ký tự", `CHAT_TARGET_OFFLINE` "Người này không online", `CHAT_COOLDOWN` "Gửi quá nhanh, hãy chờ một chút"; `LEVEL_TOO_LOW` khi gửi kênh Thế giới hiện "Cần cấp 10 để chat kênh Thế giới".
- **`core/wire.gd`**: payload `chat_send_req`, `chat_send_resp`, `chat_message`.
- Mô hình ở `autoload/router.gd` (`Router.chat`) để còn qua lần nạp lại màn World; xóa khi về màn chọn nhân vật.
- **`autoload/config.gd`**: `load_blocked(char_name) -> Array`, `save_blocked(char_name, names)` qua `ConfigFile` ở `user://chat.cfg`; lỗi đọc thì danh sách rỗng.

### 6.2. Giao diện

- **Dòng chat** (`ui/chat_line.gd`, mới): góc dưới trái, trên cần điều khiển; hai dòng gần nhất của thẻ `"all"`, mờ dần sau 8 giây không có tin mới; chấm đỏ khi thẻ Riêng có tin chưa đọc. Chạm mở cửa sổ chat.
- **Cửa sổ chat** (`ui/chat_window.gd`, mới): năm thẻ có số chưa đọc; danh sách dòng cuộn, mỗi kênh một màu (Thế giới vàng, Khu trắng, Tổ đội xanh lá, Riêng tím, Hệ thống cam); ô nhập một dòng và nút **Gửi**. Thẻ Thế giới, Khu, Tổ đội gửi vào kênh đó; thẻ Chung gửi vào kênh chọn ở nút kênh cạnh ô nhập (mặc định Khu); thẻ Riêng có thêm ô "Tới" (tên). Nút Gửi xám khi chữ rỗng hoặc đang trong khung chờ; lỗi hiện dưới ô nhập. Thẻ Tổ đội báo "Bạn chưa ở trong tổ đội" khi không có tổ.
- **Chạm tên** trong một dòng: menu **Nhắn riêng** (mở thẻ Riêng, điền tên), **Xem** (thẻ người chơi 5b, chỉ khi người đó đang trong tầm nhìn), **Chặn** / **Bỏ chặn**.
- **Thẻ người chơi** (5b): thêm nút **Nhắn riêng**.
- Thông báo tổ đội (5b) ghi thêm bằng `add_local`.
- Bàn phím ảo: dùng `LineEdit` của Godot; Enter là Gửi; đóng cửa sổ thì ẩn bàn phím. Khi ô nhập đang có focus, phím WASD không điều khiển nhân vật.

### 6.3. Smoke

`cmd/botclient partner` (5b) in thêm `CHAT <kênh> <tên người gửi> <chữ>` cho mỗi `ChatMessage` nhận được. `tests/smoke/smoke.gd` sau các bước cũ (cả hai ở cùng khu Kumotori): gửi `ChatSendReq{ZONE, "smoke <số ngẫu nhiên>"}`, chờ `ChatMessage` echo có `from_id` là mình rồi `ChatSendResp`; `tools/smoke.sh` kiểm tra đầu ra của partner có dòng `CHAT` với đúng chữ. In "SMOKE OK" kèm "chat echo".

## 7. Lệnh dev

`cmd/botclient` thêm lệnh tương tác `say <world|zone|party> <chữ>` và `tell <tên> <chữ>`, in `CHAT …` cho tin nhận được như chế độ partner.

## 8. Kiểm thử

**Server** (`go test -race`):

- `textutil`: "  a \t b  " → "a b"; xuống dòng thành dấu cách; ký tự điều khiển và U+202E bị bỏ; chuỗi chỉ có khoảng trắng → sai; 100 ký tự có dấu → đúng, 101 → sai; NFC: "e" + dấu kết hợp và "é" cho cùng kết quả; 1.025 byte → sai.
- `auth`: `NewLimiterAt(max, window, now)` với đồng hồ giả (thêm constructor cho test): `Take` sau khi hết cửa sổ lại được.
- `chat` (đồng hồ giả, hộp gửi giả):
  - Thứ tự lỗi đúng mục 4.2 cho từng bước; kênh `SYSTEM` và `UNSPECIFIED` từ client là `BAD_REQUEST`.
  - Người nhận: Thế giới tới mọi thành viên; Khu chỉ cùng map và khu (người cùng map khác khu không nhận); Tổ đội theo ô id tổ đội (đổi giá trị ô thì người nhận đổi theo); Riêng tới đúng hai người; người gửi luôn nhận echo trước phản hồi.
  - Riêng theo tên viết hoa khác và dạng dấu tổ hợp vẫn tới; tên không hợp lệ hoặc người đã `Unregister` là `CHAT_TARGET_OFFLINE`; gửi cho chính mình là `BAD_REQUEST`.
  - Cấp 9 gửi Thế giới bị `LEVEL_TOO_LOW`; sau `Leveled(id, 10)` thì được.
  - Tần suất: hai tin Thế giới trong 10 giây thì tin thứ hai bị `CHAT_COOLDOWN`, sau 10 giây được; tin bị lỗi nội dung hay người nhận không làm mất lượt; sáu người gửi Thế giới trong một giây thì người thứ sáu bị `CHAT_COOLDOWN` và bộ đếm của người đó được hoàn (gửi lại được ngay khi bộ đếm toàn server có chỗ); Khu 1 tin / 2 giây; Tổ đội và Riêng 1 tin / giây; bộ đếm theo id nhân vật nên đăng ký lại cùng nhân vật không làm mới.
  - `Moved` đổi người nhận kênh Khu; `System` tới mọi thành viên, `SystemTo` chỉ các id có đăng ký.
- `world`: `Enter` đăng ký chat; `Leave` hủy; qua cổng thì tin Khu đi theo khu mới; lên cấp gọi `chat.Leveled`.
- `gateway`: `ProtocolVersion` 16; giải mã fixture mới; `ChatSendReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestChatOverWebSocket`: A (cấp 10), B (cấp 1) cùng khu Kumotori, C ở Tổng hành dinh; A gửi Thế giới, A, B, C cùng nhận; B gửi Thế giới bị `LEVEL_TOO_LOW`; B gửi Khu, A nhận, C không; A nhắn riêng tên của C viết hoa khác, chỉ A và C nhận, `to_name` đúng; nhắn tên không có bị `CHAT_TARGET_OFFLINE`; A gửi Khu hai lần liền, lần hai bị `CHAT_COOLDOWN`; A mời B vào tổ đội, B gửi Tổ đội, A nhận, C không; C gửi Tổ đội bị `NOT_IN_PARTY`.

**Client** (`tests/run.sh`, mục `# ____TEST CHAT____`): `chat_log` (dòng vào đúng thẻ và thẻ Chung; Hệ thống chỉ ở Chung; giới hạn 100 dòng; chưa đọc tăng và `open` xóa; chặn bỏ tin của người đó nhưng không bỏ Hệ thống; `outgoing` theo `self_id`; `clean` khớp các ví dụ của `textutil`; `cooldown_left`; `prefix` các kênh); giải mã ba fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito, cấp 90) và `test2` (Hana) cùng khu: Kaito gửi Khu, Hana thấy; Hana nhắn riêng "kaito", Kaito thấy chấm đỏ ở dòng chat; Kaito gửi Thế giới hai lần liền thấy lỗi chờ; lập tổ đội và chat Tổ đội; Kaito chặn Hana thì không còn thấy tin của Hana.

## 9. Tài liệu cập nhật

- `docs/game-design.md`: mục mới ngắn trong mục 7 hoặc 6.1 "Chat": năm kênh, cấp 10 cho Thế giới, giới hạn tần suất, 100 ký tự, chặn ở client, không lưu lịch sử.
- `CLAUDE.md` (server): dịch vụ chat ở mức world, không gọi vào zone; giới hạn chat dùng `Limiter.Take`/`Refund`; hàm làm sạch chữ dùng chung ở `internal/textutil`.
- `CLAUDE.md` (client): mô hình chat ở `core/chat_log.gd` (`Router.chat`).
- README hai repo: kênh chat, giới hạn, `botclient say`/`tell`.
