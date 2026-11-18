# Giai đoạn 0 — Nền móng server

Ngày: 2026-11-18. Trạng thái: chờ duyệt.

Tài liệu này là thiết kế kỹ thuật cho Giai đoạn 0 của server game. Thiết kế gameplay nằm ở [game-design.md](../../game-design.md); khi hai tài liệu khác nhau về luật chơi, game-design là chuẩn.

## 1. Bối cảnh và phạm vi

### 1.1. Lộ trình các giai đoạn

Server được làm theo giai đoạn. Mỗi giai đoạn có spec, kế hoạch và code riêng, và khi xong thì chạy được.

| Giai đoạn | Nội dung | Kết quả |
| --- | --- | --- |
| **0. Nền móng** | Cấu trúc repo, server Go, Postgres, giao thức mạng, đăng nhập, tạo nhân vật, vào một map, di chuyển đồng bộ | Hai client cùng vào một map và thấy nhau di chuyển |
| 1. Chiến đấu PvE cơ bản | Quái sinh theo bãi, chọn mục tiêu, chém mặc định, công thức sát thương, chết và hồi sinh, EXP, lên cấp, điểm tiềm năng | Đánh quái lên cấp |
| 2. Kỹ năng | 5 class, 4 chủ động và 1 bị động, MP, hồi chiêu, sách kỹ năng | Dùng chiêu theo class |
| 3. Trang bị | Túi đồ, rơi đồ, mặc đồ, cường hóa, tương sinh, khảm ngọc, tẩy luyện | Săn và nâng đồ |
| 4. Thế giới | Nhiều map, cổng, khu/kênh, NPC, cửa hàng bình, kho, Xu, Lượng | Đi lại giữa các vùng |
| 5. PK và tổ đội | Bật/tắt PK, phạt EXP, tổ đội 5 người, EXP chia sẻ | Đánh người, đi theo nhóm |
| 6. Nhiệm vụ và auto | Nhiệm vụ, tự tìm đường (A*), click để đi tới điểm, auto đánh | Vòng chơi hằng ngày |
| 7+. Xã hội | Giao dịch, bang hội, sự kiện, Vô Hạn Thành | Nội dung cộng đồng |

### 1.2. Ràng buộc

- **Repo này chỉ chứa server.** Client Godot 4 (GDScript) ở repo khác và làm sau.
- Server viết bằng **Go**, database **Postgres**.
- Client sẽ chạy trên **di động (Android, iOS) và PC**, nên giao thức không được giả định một kiểu điều khiển.
- Quy mô mục tiêu: **khoảng 1.000 người online cùng lúc** trên một máy chủ, chạy **một tiến trình**.
- Đăng nhập bằng **tài khoản và mật khẩu**. Mỗi tài khoản tối đa **3 nhân vật** trên một máy chủ.
- Chưa có hình ảnh; Giai đoạn 0 không phụ thuộc hình ảnh.

### 1.3. Mục tiêu Giai đoạn 0

Hai client giả lập (`botclient`) cùng đăng ký, đăng nhập, tạo nhân vật, vào một map và nhận được vị trí di chuyển của nhau. Có test tự động chứng minh điều đó.

### 1.4. Không làm ở Giai đoạn 0

Chiến đấu, quái, nhiều map và cổng chuyển map, click để đi tới điểm, chat, client Godot, TLS ở môi trường dev.

## 2. Kiến trúc

### 2.1. Cấu trúc repo

```
DemonSlayer2D/              # repo server
├── docs/                   # tài liệu thiết kế
├── proto/                  # file .proto: định nghĩa gốc của giao thức
├── maps/                   # dữ liệu map dạng JSON
├── cmd/
│   ├── gameserver/         # main: đọc config, nối DB, mở cổng WebSocket
│   └── botclient/          # client giả lập trong terminal: lệnh thủ công và chế độ thử tải
├── internal/
│   ├── gateway/            # kết nối WebSocket, đọc/ghi gói tin, phiên kết nối, giới hạn tốc độ
│   ├── auth/               # đăng ký, đăng nhập, băm mật khẩu, kiểm tra tên
│   ├── store/              # truy cập Postgres: tài khoản, nhân vật
│   ├── world/              # quản lý map và zone, đưa người chơi vào/ra, phiên trực tuyến
│   ├── zone/               # vòng lặp một map/khu: thực thể, di chuyển, tầm nhìn
│   ├── mapdata/            # đọc và kiểm tra file map
│   ├── saver/              # goroutine ghi dữ liệu nhân vật xuống DB
│   └── pb/                 # code Go sinh từ proto
├── e2e/                    # test end-to-end
├── migrations/             # file SQL tạo bảng (goose)
├── docker-compose.yml      # Postgres cho dev và test
└── Makefile                # proto, run, db-up, db-down, test, lint
```

- Code Go đặt ở thư mục gốc (`cmd/`, `internal/`), không lồng trong `server/`, vì repo chỉ có server.
- `proto/` và `maps/` là **nguồn chuẩn** mà repo client sẽ lấy về (git submodule hoặc copy khi build) để sinh code GDScript và dự đoán va chạm.

### 2.2. Thư viện

| Việc | Thư viện |
| --- | --- |
| WebSocket | `github.com/coder/websocket` |
| Postgres | `github.com/jackc/pgx/v5` (pgxpool) |
| Mã hóa gói tin | `google.golang.org/protobuf`, sinh code bằng `protoc-gen-go` |
| Băm mật khẩu | `golang.org/x/crypto/argon2` (argon2id) |
| Chuẩn hóa Unicode | `golang.org/x/text/unicode/norm` |
| Migration | `github.com/pressly/goose/v3`, nhúng file SQL vào binary |
| Log | `log/slog` (thư viện chuẩn) |

Go 1.25. Postgres 17 chạy trong Docker.

### 2.3. Luồng chạy

1. **Gateway.** Mỗi kết nối WebSocket có hai goroutine: một đọc, một ghi. Gói gửi đi qua một hàng đợi có giới hạn (256 gói). Hàng đợi đầy nghĩa là client đọc quá chậm; kết nối đó bị đóng để không kéo chậm server.
2. **Trước khi vào game.** Đăng ký, đăng nhập, danh sách nhân vật, tạo/xóa nhân vật được xử lý trong goroutine đọc của kết nối, gọi `store` trực tiếp.
3. **Vào game.** Phiên kết nối được `world` giao cho một **zone**. Mỗi zone là **một goroutine duy nhất sở hữu toàn bộ trạng thái** của map/khu đó, nên không cần khóa. Lệnh từ kết nối vào zone qua channel; zone gửi gói vào hàng đợi gửi của từng kết nối.
4. **Lưu dữ liệu.** Zone không gọi DB. Zone gửi yêu cầu lưu sang goroutine `saver`.

Ranh giới gateway ↔ zone chỉ là channel và gói tin, nên sau này có thể tách thành nhiều tiến trình nếu cần.

## 3. Database

### 3.1. Nguyên tắc

- **Dữ liệu tĩnh của game** (định nghĩa vật phẩm, kỹ năng, quái, map, nhiệm vụ, bảng EXP) nằm trong file dữ liệu của repo, nạp vào bộ nhớ khi khởi động. Database chỉ lưu dữ liệu người chơi.
- **Dữ liệu tạm thời** chỉ giữ trong bộ nhớ: phiên đăng nhập, zone đang đứng, tổ đội, quái, mục tiêu đang khóa, hồi chiêu.
- Toàn bộ bảng v1.0.0.0 được thiết kế ở đây để làm chuẩn, nhưng **mỗi giai đoạn chỉ tạo bảng và cột nó dùng**, mỗi lần bằng một file migration.
- Tiền và EXP dùng `bigint`.

### 3.2. Bảng tạo ở Giai đoạn 0

#### `accounts`

| Cột | Kiểu | Ràng buộc |
| --- | --- | --- |
| `id` | `bigint generated always as identity` | Khóa chính |
| `username` | `text not null` | 4–20 ký tự `a-z 0-9 _`, lưu dạng chữ thường; chỉ mục duy nhất |
| `password_hash` | `text not null` | Chuỗi argon2id theo định dạng PHC (gồm tham số và salt) |
| `status` | `text not null default 'active'` | `check (status in ('active','banned'))` |
| `created_at` | `timestamptz not null default now()` | |
| `last_login_at` | `timestamptz` | |

#### `characters`

| Cột | Kiểu | Ràng buộc |
| --- | --- | --- |
| `id` | `bigint generated always as identity` | Khóa chính |
| `account_id` | `bigint not null` | `references accounts(id) on delete cascade` |
| `slot` | `smallint not null` | `check (slot between 1 and 3)`; `unique (account_id, slot)` |
| `name` | `text not null` | 2–16 ký tự sau chuẩn hóa NFC; chữ cái (kể cả tiếng Việt có dấu) và chữ số, không khoảng trắng hay ký tự đặc biệt; chỉ mục duy nhất trên `lower(name)` |
| `class` | `text not null` | `check (class in ('viem','thuy','minh','nham','phong'))` |
| `level` | `int not null default 1` | `check (level >= 1)` |
| `exp` | `bigint not null default 0` | Được phép âm (phạt PK, game-design mục 5.7) |
| `map_id` | `text not null` | |
| `pos_x`, `pos_y` | `real not null` | Tọa độ theo ô |
| `created_at`, `updated_at` | `timestamptz not null default now()` | |

Xóa nhân vật là xóa hẳn dòng. Phiên đăng nhập không lưu DB.

### 3.3. Bảng v1.0.0.0 cho các giai đoạn sau

Chỉ là thiết kế tham chiếu; chi tiết cột được chốt lại ở spec của từng giai đoạn.

| Bảng / cột | Mục đích | Cột chính | Giai đoạn |
| --- | --- | --- | --- |
| `characters` + cột | Chỉ số | `str`, `agi`, `vit`, `int`, `free_points`, `hp`, `mp` | 1 |
| `characters` + cột | Điểm kỹ năng | `free_skill_points` | 2 |
| `character_skills` | Kỹ năng đã học | `character_id`, `skill_id`, `level` (1–10) | 2 |
| `items` | Mọi vật phẩm: trang bị và vật phẩm xếp chồng | `id`, `def_id`, `quantity`, `location` (`bag`, `equip`, `storage`, `quest_bag`, `market`), `character_id`, `account_id`, `slot_index`, `created_at` | 3 |
| `items` + cột trang bị | Thuộc tính trang bị | `quality`, `element`, `enhance` (0–15), `sockets` (0–3), `sub_lines` (`jsonb`), `hidden_lines` (`jsonb`), `gems` (`jsonb`), `enhance_pity` (`jsonb`, tỷ lệ ẩn theo cấp đích) | 3 |
| `item_logs` | Món đồ sinh ra, mất đi, đổi chủ | `item_id`, `def_id`, `action`, `from_char`, `to_char`, `detail` (`jsonb`), `created_at` | 3 |
| `accounts` + cột | Lượng, kho | `luong` (`bigint`), `storage_slots` | 4 |
| `characters` + cột | Xu | `xu` (`bigint`) | 4 |
| `currency_logs` | Mọi lần cộng/trừ Xu và Lượng | `account_id`, `character_id`, `currency`, `delta`, `balance_after`, `reason`, `ref_id`, `created_at` | 4 |
| `luong_topups` | Giao dịch nạp tiền | `account_id`, `amount`, `provider`, `provider_ref` (duy nhất), `status`, `created_at` | 4 |
| `characters` + cột | Chế độ PK | `pk_on` (`bool`) | 5 |
| `character_settings` | Cài đặt lưu trên server | `character_id`, `data` (`jsonb`: auto-play, phím tắt) | 6 |
| `character_quests` | Nhiệm vụ | `character_id`, `quest_id`, `status`, `progress` (`jsonb`), `accepted_at`, `done_at` | 6 |
| `character_daily` | Bộ đếm reset hằng ngày | `character_id`, `day`, `activity_points`, `counters` (`jsonb`) | 6 |
| `guilds` | Bang hội | `id`, `name`, `leader_id`, `fund`, `created_at` | 7 |
| `guild_members` | Thành viên bang | `guild_id`, `character_id`, `role`, `contribution`, `joined_at` | 7 |
| `castle_owner` | Thành chủ Vô Hạn Thành | `guild_id`, `tax_rate` (0–5), `since` | 7 |
| `market_listings` | Treo bán ở Vô Hạn Thành | `id`, `seller_id`, `item_id`, `price`, `status`, `created_at`, `expires_at` | 7 |
| `event_runs` | Mỗi lượt/kỳ sự kiện | `id`, `event_id`, `started_at`, `ended_at`, `result` (`jsonb`) | 7 |
| `event_scores` | Điểm cá nhân/bang | `event_run_id`, `character_id`, `guild_id`, `points` | 7 |
| `character_titles` | Danh hiệu | `character_id`, `title_id`, `acquired_at` | 7 |

Tổng cộng 18 bảng. Chưa có bảng cho thú cưỡi, trợ thủ, thời trang (thiết kế gameplay còn mở) và đồ chế (ngoài v1.0.0.0).

## 4. Giao thức

### 4.1. Khung chung

- Kết nối WebSocket tại đường dẫn `/ws`, tin nhắn **dạng nhị phân**.
- Mỗi tin nhắn chứa đúng một `Envelope`:

```proto
syntax = "proto3";
package ds.v1;

message Envelope {
  uint32 seq = 1;   // client đánh số mỗi yêu cầu, tăng dần từ 1
                    // phản hồi của server dùng lại đúng seq đó
                    // tin server tự đẩy xuống có seq = 0
  oneof payload {
    // các gói ở mục 4.2 và 4.3
  }
}
```

- Mỗi yêu cầu nhận đúng một phản hồi: gói kết quả tương ứng hoặc `Error { ErrorCode code; string message; }`.
- Gói tin trong game (`MoveInput`) không có phản hồi riêng; kết quả thể hiện qua `WorldTick`.
- Thêm trường mới vào gói tin không làm hỏng client cũ (quy tắc của proto3). Thay đổi không tương thích thì tăng `PROTOCOL_VERSION`.

### 4.2. Trước khi vào game

| Client gửi | Server trả lời | Quy tắc |
| --- | --- | --- |
| `RegisterReq { username, password }` | `RegisterResp {}` | Tên tài khoản 4–20 ký tự `a-z 0-9 _`, không phân biệt hoa thường. Mật khẩu 8–64 ký tự |
| `LoginReq { username, password, protocol_version }` | `LoginResp { account_id }` | Sai `protocol_version` thì `VERSION_MISMATCH`. Tài khoản bị khóa thì `AUTH_FAILED` |
| `CharListReq {}` | `CharListResp { repeated CharSummary chars }` | `CharSummary { id, slot, name, class, level, map_id }` |
| `CreateCharReq { slot, name, class }` | `CreateCharResp { CharSummary char }` | Nhân vật mới ở `default` spawn point của map khởi đầu |
| `DeleteCharReq { char_id, confirm_name }` | `DeleteCharResp {}` | `confirm_name` phải trùng tên nhân vật |
| `EnterWorldReq { char_id }` | `EnterWorldResp { self_entity_id, map_id, zone, x, y, server_tick, move_speed }` | `move_speed` tính bằng ô/giây |
| `LeaveWorldReq {}` | `LeaveWorldResp {}` | Về màn chọn nhân vật, nhân vật được lưu |

Các gói trước khi vào game chỉ hợp lệ khi kết nối ở đúng trạng thái (mục 4.6); sai trạng thái thì `BAD_REQUEST`.

### 4.3. Trong game

| Chiều | Gói tin | Quy tắc |
| --- | --- | --- |
| C → S | `MoveInput { input_seq, dir_x, dir_y }` | Hướng từ joystick ảo hoặc WASD. Độ dài vector được kẹp về tối đa 1; (0, 0) là dừng. Client gửi khi hướng đổi, tối đa 20 lần/giây. `input_seq` tăng dần |
| S → C | `EntityEnter { id, kind, name, class, level, x, y, dir_x, dir_y }` | Thực thể vào tầm nhìn. `kind` ở Giai đoạn 0 chỉ có `PLAYER` |
| S → C | `EntityLeave { id }` | Thực thể ra khỏi tầm nhìn hoặc rời zone |
| S → C | `WorldTick { tick, ack_input_seq, repeated EntityMove moves }` | Mỗi tick (10 lần/giây). `EntityMove { id, x, y, dir_x, dir_y }` chỉ cho thực thể trong tầm nhìn có thay đổi, gồm cả chính mình. `ack_input_seq` là `input_seq` lớn nhất server đã áp dụng cho người nhận |
| C ↔ S | `Ping { client_time }` / `Pong { client_time, server_time }` | Thời gian tính bằng mili-giây |
| S → C | `Kicked { KickReason reason }` | Server gửi rồi đóng kết nối |

`KickReason`: `DUPLICATE_LOGIN`, `SERVER_SHUTDOWN`, `ZONE_RESTART`, `RATE_LIMITED`, `IDLE_TIMEOUT`, `SLOW_CONSUMER`, `PROTOCOL_ERROR`.

Một `WorldTick` vẫn được gửi khi không có thay đổi nào, để client biết server còn chạy; khi đó `moves` rỗng.

### 4.4. Mã lỗi

`BAD_REQUEST`, `VERSION_MISMATCH`, `RATE_LIMITED`, `AUTH_FAILED`, `USERNAME_TAKEN`, `USERNAME_INVALID`, `PASSWORD_INVALID`, `NAME_TAKEN`, `NAME_INVALID`, `SLOT_OCCUPIED`, `CHAR_NOT_FOUND`, `NOT_IN_WORLD`, `ALREADY_IN_WORLD`, `ZONE_FULL`, `INTERNAL`.

Đăng nhập sai tên tài khoản hay sai mật khẩu đều trả `AUTH_FAILED`, không phân biệt, để không lộ tài khoản nào tồn tại.

### 4.5. Giới hạn chống lạm dụng

| Giới hạn | Giá trị | Vượt thì |
| --- | --- | --- |
| Kích thước một tin nhắn | 4 KB | Đóng kết nối, `PROTOCOL_ERROR` |
| Số gói mỗi kết nối | 30 gói/giây | Bỏ gói; vượt liên tục 5 giây thì `RATE_LIMITED` và đóng |
| `MoveInput` | 20 gói/giây | Bỏ gói vượt mức |
| Đăng nhập sai | 5 lần/phút theo cặp IP + tên tài khoản | `RATE_LIMITED` |
| Đăng ký | 5 lần/giờ theo IP | `RATE_LIMITED` |
| Chưa đăng nhập | 30 giây kể từ khi kết nối | `IDLE_TIMEOUT` |
| Không nhận gói nào | 30 giây | `IDLE_TIMEOUT` |
| Gói không giải mã được | 1 lần | `PROTOCOL_ERROR` |

### 4.6. Trạng thái của một kết nối

```
CONNECTED ──Login OK──▶ AUTHED ──EnterWorld OK──▶ IN_WORLD
    ▲                     ▲  │                       │
    │                     │  └──(CharList/Create/    │
    │                     │      Delete hợp lệ ở đây)│
    │                     └───────LeaveWorld─────────┘
    └── Register hợp lệ ở CONNECTED
```

`MoveInput` chỉ hợp lệ ở `IN_WORLD`; ở trạng thái khác thì bỏ qua và ghi log debug.

## 5. Map, zone và vòng lặp game

### 5.1. File map

```json
{
  "id": "kumotori_1",
  "name": "Kumotori – Đường núi tuyết",
  "type": "training",
  "width": 64,
  "height": 48,
  "collision": ["####...", "#....#", "..."],
  "spawn_points": [{ "id": "default", "x": 5.5, "y": 5.5 }]
}
```

- `collision` có `height` chuỗi, mỗi chuỗi `width` ký tự; `.` đi được, `#` bị chặn. Ô ngoài biên tính là bị chặn.
- `type`: `town`, `training`, `battle`, `instance` (game-design mục 5.1). Giai đoạn 0 chỉ có `kumotori_1` loại `training`.
- Khi khởi động, server kiểm tra: kích thước khớp, chỉ có ký tự hợp lệ, `id` không trùng, mọi spawn point nằm trên ô đi được và có ít nhất spawn point `default`. Sai thì server dừng và báo lỗi.
- Map khởi đầu cho nhân vật mới cấu hình bằng `START_MAP` (mặc định `kumotori_1`).

### 5.2. World và zone

- `world` nạp mọi map khi khởi động. Mỗi map luôn có zone 1.
- Mỗi zone tối đa `ZONE_CAPACITY` người (mặc định 50, theo mốc 30–50 người/khu ở game-design mục 5.3). Khi mọi zone của map đều đầy, mở zone mới với số thứ tự kế tiếp. Zone khác zone 1 không có ai trong 5 phút thì đóng.
- Vào game: chọn zone còn chỗ có số thứ tự nhỏ nhất.
- Vị trí đã lưu nằm trên ô bị chặn, hoặc `map_id` không còn tồn tại: đưa về spawn point `default` của map khởi đầu.
- `world` giữ bảng `account_id → phiên đang online`.

### 5.3. Vòng lặp zone

Mỗi zone là một goroutine với hộp thư lệnh (channel đệm 1.024) và đồng hồ `TICK_RATE` (mặc định 10 lần/giây, 100 ms mỗi tick). Mỗi tick:

1. Lấy hết lệnh trong hộp thư: `Join`, `Leave`, `MoveInput`, `SnapshotRequest`.
2. Với mỗi người chơi: bước di chuyển = hướng × `move_speed` × thời gian một tick, rồi xử lý va chạm (mục 5.4).
3. Cập nhật ô tầm nhìn của từng người (mục 5.5), tính danh sách ai mới thấy, ai không còn thấy.
4. Gửi mỗi người: `EntityLeave`, `EntityEnter` rồi `WorldTick`.
5. Gửi không chờ: hàng đợi gửi của ai đầy thì người đó bị đóng kết nối với `SLOW_CONSUMER`, xử lý như mất kết nối.

- Hàm `tick(now)` tách riêng và nhận thời gian làm tham số, để test gọi thẳng với đồng hồ giả.
- Tick chạy quá 50 ms thì ghi log cảnh báo.
- Hộp thư đầy thì kết nối gửi lệnh bị bỏ lệnh và ghi log; zone không bao giờ bị chặn.

### 5.4. Di chuyển và va chạm

- Tọa độ theo ô, kiểu `float32`. Ô (i, j) phủ vùng `[i, i+1) × [j, j+1)`.
- Nhân vật là hình tròn bán kính 0,3 ô.
- `move_speed` = hệ số tốc độ × `MOVE_SPEED_TILES` (mặc định 4 ô/giây ứng với hệ số 1, game-design mục 2.2). Con số 4 là tạm thời, chỉnh sau khi chơi thử.
- Mỗi tick, di chuyển trục X trước, kiểm tra va chạm, rồi trục Y. Trục nào đụng ô bị chặn thì dừng sát mép ô trên trục đó, trục còn lại vẫn đi. Kết quả là trượt dọc theo tường.
- Bước di chuyển mỗi tick nhỏ hơn bán kính, nên không thể xuyên qua tường mỏng một ô.
- Server chỉ dùng hướng; vị trí do server tính. Client gửi bao nhiêu `MoveInput` cũng không đi nhanh hơn `move_speed`.
- Client tự dự đoán vị trí của mình theo cùng thuật toán. Khi nhận `WorldTick`, client lấy vị trí server cho mình kèm `ack_input_seq`, chạy lại các input chưa được ack để sửa sai lệch.

### 5.5. Tầm nhìn

- Map chia thành ô lưới `AOI_CELL_SIZE × AOI_CELL_SIZE` (mặc định 10 × 10 ô).
- Mỗi người thấy mọi thực thể trong 3 × 3 ô lưới quanh ô lưới mình đang đứng, khoảng 15 ô mỗi hướng. Tầm này lớn hơn tầm phát hiện 6 ô và tầm giữ khóa 8 ô (game-design mục 2.5).
- Người chơi luôn thấy chính mình.

### 5.6. Lưu dữ liệu

- Goroutine `saver` nhận yêu cầu `SaveCharacter { char_id, map_id, x, y, level, exp }` qua channel và ghi bằng `UPDATE`.
- Thời điểm lưu:
  - nhân vật rời zone (thoát, về màn chọn nhân vật, mất kết nối, bị đá);
  - định kỳ: mỗi `SAVE_INTERVAL` (mặc định 60 giây), zone gửi một lô vị trí của mọi người đang trong zone.
- Nhiều yêu cầu chưa ghi của cùng một nhân vật được gộp, chỉ giữ bản mới nhất.
- Lỗi DB: thử lại sau 1, 2, 4… tối đa 30 giây; ghi log lỗi mỗi lần thất bại.
- Lưu khi rời zone có kênh báo hoàn thành, để luồng đăng nhập trùng (mục 5.7) chờ được.
- Server sập đột ngột thì mất tối đa `SAVE_INTERVAL` di chuyển. Từ Giai đoạn 1, thay đổi quan trọng như lên cấp, nhận đồ, tiêu tiền được lưu ngay khi xảy ra.

### 5.7. Mất kết nối, đăng nhập trùng, tắt server

- **Mất kết nối:** gateway gửi `Leave` cho zone; nhân vật rời zone ngay và được lưu. Giữ nhân vật lại vài giây để chống né PK bằng cách thoát game để Giai đoạn 5.
- **Đăng nhập trùng:** đăng nhập thành công khi tài khoản đang online thì phiên cũ nhận `Kicked { DUPLICATE_LOGIN }`. Nếu phiên cũ đang trong game, server **chờ lưu xong nhân vật của phiên cũ** rồi mới trả `LoginResp`. Phiên mới vì vậy không đọc phải dữ liệu cũ hơn dữ liệu vừa lưu.
- **Tắt server (SIGINT/SIGTERM):** ngừng nhận kết nối mới; mọi người nhận `Kicked { SERVER_SHUTDOWN }`; các zone gửi lần lưu cuối; `saver` ghi hết rồi thoát; chờ tối đa 30 giây.

### 5.8. Cấu hình

Đọc từ biến môi trường; thiếu biến bắt buộc thì server dừng và báo lỗi.

| Biến | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | Cổng WebSocket (`/ws`) và HTTP (`/healthz`) |
| `DATABASE_URL` | (bắt buộc) | Chuỗi kết nối Postgres |
| `MAPS_DIR` | `./maps` | Thư mục file map |
| `START_MAP` | `kumotori_1` | Map của nhân vật mới |
| `TICK_RATE` | `10` | Số tick mỗi giây |
| `MOVE_SPEED_TILES` | `4` | Số ô/giây ứng với hệ số tốc độ 1 |
| `AOI_CELL_SIZE` | `10` | Cạnh ô lưới tầm nhìn, tính theo ô |
| `ZONE_CAPACITY` | `50` | Số người tối đa mỗi zone |
| `SAVE_INTERVAL` | `60s` | Chu kỳ lưu vị trí |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## 6. Xử lý lỗi và vận hành

- **Lỗi do client** (gói sai, sai mật khẩu, tên trùng…): trả `Error` có mã; không bao giờ làm sập server.
- **Panic trong goroutine của một kết nối:** bắt lại, ghi log kèm stack, đóng riêng kết nối đó.
- **Panic trong zone:** bắt lại, ghi log kèm mã map và zone, lưu vị trí mọi người trong zone, gửi `Kicked { ZONE_RESTART }`, rồi tạo lại zone rỗng. Zone khác không bị ảnh hưởng.
- **DB không truy cập được:** đăng ký, đăng nhập, tạo, xóa nhân vật trả `INTERNAL`; `saver` thử lại như mục 5.6.
- **Log:** `slog` dạng JSON. Mỗi dòng log liên quan tới người chơi kèm `conn_id`, `account_id`, `char_id` khi đã biết. Không bao giờ ghi mật khẩu.
- **Thống kê:** mỗi phút một dòng log `stats` gồm số kết nối, số người trong game, số zone, thời gian tick lớn nhất và p99 trong phút đó.
- **`GET /healthz`:** 200 nếu server chạy và `ping` được DB, ngược lại 503.
- **TLS:** khi triển khai, server đặt sau reverse proxy (Caddy hoặc nginx) xử lý `wss://`. Môi trường dev dùng `ws://`.

## 7. Bảo mật tài khoản

- Băm mật khẩu bằng argon2id: bộ nhớ 64 MB, 3 vòng, 2 luồng, salt 16 byte ngẫu nhiên, khóa 32 byte. Tham số lưu trong chuỗi băm, nên đổi tham số sau này vẫn kiểm được mật khẩu cũ.
- So sánh băm bằng hàm thời gian không đổi.
- Băm mật khẩu tốn CPU, nên số lần băm chạy đồng thời giới hạn bằng số nhân CPU, tránh một đợt đăng nhập ồ ạt làm chậm cả server.
- Tên tài khoản và tên nhân vật được chuẩn hóa NFC trước khi kiểm tra và lưu, để cùng một chữ tiếng Việt gõ theo hai cách vẫn là một tên.

## 8. Kiểm thử

| Loại | Nội dung | Cách chạy |
| --- | --- | --- |
| Unit | Đọc và kiểm tra map; di chuyển (không xuyên tường, trượt theo tường, không vượt tốc độ, kẹp độ dài hướng); tầm nhìn (vào/ra đúng lúc, luôn thấy chính mình); kiểm tra tên tài khoản và tên nhân vật (tiếng Việt, NFC, độ dài, ký tự cấm); băm và kiểm mật khẩu; giới hạn tốc độ | `go test ./...` |
| Zone | Gọi `tick(now)` với đồng hồ giả; kết quả lặp lại được | Trong unit test |
| Database | Tạo/đọc tài khoản và nhân vật; trùng tên không phân biệt hoa thường; tối đa 3 slot; lưu và đọc vị trí; xóa nhân vật | Postgres thật qua `TEST_DATABASE_URL`; mỗi test dùng schema riêng; không có biến thì bỏ qua |
| End-to-end | Server chạy trong test trên cổng ngẫu nhiên. Hai bot đăng ký, đăng nhập, tạo nhân vật, vào map; bot A di chuyển, bot B nhận `EntityEnter` và vị trí mới. Đăng nhập trùng đá phiên cũ. Thoát rồi vào lại đúng vị trí. Gửi gói sai thì bị đóng với `PROTOCOL_ERROR` | `go test ./e2e/...` (cần `TEST_DATABASE_URL`) |
| Race | Truy cập dữ liệu đồng thời | `go test -race ./...` |
| Tải | `botclient load -n 1000`: 1.000 kết nối đăng ký, vào game và đi ngẫu nhiên trên nhiều zone | Chạy tay trên máy dev |

## 9. Tiêu chí hoàn thành

1. `make db-up && make run` khởi động Postgres và server với map `kumotori_1`.
2. `botclient` có các lệnh `register`, `login`, `chars`, `create`, `delete`, `enter`, `move`, `load`.
3. Test end-to-end đạt: hai bot thấy nhau di chuyển, vị trí mới đến bot kia trong vòng 200 ms trên máy dev.
4. Có test chứng minh không xuyên tường và không vượt tốc độ.
5. Đăng nhập trùng đá phiên cũ; thoát rồi vào lại đúng vị trí đã lưu.
6. `go vet ./...` và `go test -race ./...` đạt.
7. Thử tải 1.000 bot trên máy dev: thời gian tick p99 dưới 50 ms, không bot nào bị đóng vì `SLOW_CONSUMER`.
8. `README.md` hướng dẫn cài công cụ, chạy DB, sinh proto, chạy server và bot.
