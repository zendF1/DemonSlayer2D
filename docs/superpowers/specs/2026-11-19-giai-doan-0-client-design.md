# Giai đoạn 0 — Client Godot

Ngày: 2026-11-19. Trạng thái: đã triển khai (2026-11-19).

Tài liệu này là thiết kế client cho Giai đoạn 0. Giao thức, map và luật di chuyển nằm ở [spec server Giai đoạn 0](2026-11-18-giai-doan-0-nen-mong-server-design.md); khi hai tài liệu khác nhau về giao thức, spec server là chuẩn.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Một người mở client, đăng ký, đăng nhập, tạo nhân vật, vào `kumotori_1` và đi lại bằng bàn phím hoặc joystick ảo. Hai cửa sổ client trên cùng máy thấy nhau di chuyển mượt.

### 1.2. Ràng buộc

- Repo riêng `git@github.com:zendF1/DemonSlayer2DGame.git`, thư mục `../DemonSlayer2DGame` cạnh repo server.
- Godot 4.7, GDScript. Renderer `mobile`, màn ngang 1600×900, `window/stretch/mode="canvas_items"`, `aspect="expand"` (giống `battle-squad-game`).
- Giai đoạn 0 chạy thử trên PC (macOS). Điều khiển có cả bàn phím và joystick ảo, để khi xuất Android/iOS không phải làm lại.
- Chưa có hình ảnh: mọi thứ vẽ bằng code (`_draw()` và Control dựng trong `_ready()`).
- Tài liệu tiếng Việt; code, comment và commit message tiếng Anh. Chữ hiển thị trong game tiếng Việt.

### 1.3. Không làm ở Giai đoạn 0

Hình ảnh và âm thanh, tự kết nối lại, xuất Android/iOS, màn cài đặt, chat, TLS (`wss://`).

## 2. Thay đổi phía server

`EnterWorldResp` thêm trường:

```proto
// Zone ticks per second; each tick moves move_speed / tick_rate tiles.
uint32 tick_rate = 8;
```

Chỉ thêm trường nên client cũ không hỏng, không tăng `PROTOCOL_VERSION`. Zone điền giá trị `TICK_RATE`. Test e2e kiểm `tick_rate` bằng cấu hình. Spec server mục 4.2 ghi thêm trường này.

Lý do: server đi mỗi tick một bước cố định `move_speed / tick_rate`. Biết `tick_rate`, client chạy tick cố định giống hệt server, nên dự đoán khớp và không phải kéo vị trí liên tục.

## 3. Cấu trúc repo client

```
DemonSlayer2DGame/
├── project.godot
├── server/                 # git submodule → DemonSlayer2D (nguồn proto/ và maps/)
├── addons/protobuf/        # godobuf, vendored
├── proto/protocol.gd       # sinh từ server/proto bằng tools/gen_proto.sh, có commit
├── data/maps/              # copy từ server/maps bằng tools/sync_server.sh, có commit
├── autoload/
│   ├── config.gd           # ws_url
│   ├── net.gd              # kết nối WebSocket, request/response theo seq
│   └── router.gd           # chuyển màn, dữ liệu trao giữa các màn
├── core/                   # logic không phụ thuộc Node, test được headless
│   ├── movement.gd         # port internal/zone/move.go
│   ├── map_grid.gd         # đọc file map, ô bị chặn
│   ├── predictor.gd        # dự đoán và đối chiếu vị trí của mình
│   ├── interpolator.gd     # nội suy vị trí người khác
│   └── errors.gd           # ErrorCode / KickReason → câu tiếng Việt
├── screens/                # .tscn chỉ có node gốc gắn script (màn world: Node2D, còn lại: Control)
│   ├── login.gd/.tscn
│   ├── char_select.gd/.tscn
│   └── world.gd/.tscn
├── ui/                     # vẽ bằng code
│   ├── ui.gd               # builder: label, button, vbox, card, input
│   ├── map_view.gd
│   ├── entity_view.gd
│   ├── joystick.gd
│   └── hud.gd
├── tests/
│   ├── run.sh              # chạy headless, fail khi có lỗi script
│   ├── run_tests.gd
│   ├── test_case.gd        # check / eq / near
│   ├── fixtures/           # byte Envelope do Go sinh
│   └── unit/
│       ├── test_core.gd
│       └── test_net.gd
└── tools/
    ├── gen_proto.sh
    ├── sync_server.sh
    ├── smoke.sh
    └── run2.sh
```

- **Submodule `server/`.** Ghim repo server ở một commit cố định, nên biết client khớp với server nào. Muốn lấy giao thức mới: cập nhật submodule, chạy `tools/sync_server.sh` (copy map và fixture, sinh lại proto), commit.
- **Export bỏ `server/`, `tests/`, `tools/`.** Runtime chỉ đọc `res://proto/` và `res://data/maps/`.
- **Godobuf.** Plugin thuần GDScript, hỗ trợ proto3 và `oneof`. `tools/gen_proto.sh` chạy trình biên dịch của godobuf ở chế độ headless, sinh `proto/protocol.gd` từ `server/proto/ds/v1/protocol.proto`.

## 4. Mạng

### 4.1. `Net` (autoload)

- `WebSocketPeer`, tin nhắn nhị phân. Poll trong `_process`.
- `connect_to(url)` mở kết nối. Signal `connected` và `disconnected(reason: String)`.
- `await Net.request(env)` gán `seq` tăng dần từ 1, gửi, rồi trả về Envelope phản hồi cùng `seq`: gói kết quả hoặc `Error`. Không có phản hồi sau 10 giây, hoặc mất kết nối khi đang chờ, thì trả lỗi mạng.
- Request quá 10 giây, hoặc 15 giây không nhận được gói nào (trong game server gửi `WorldTick` mỗi tick, ngoài game có `Pong` cho mỗi `Ping`), thì coi như đường truyền chết: đóng kết nối và phát `disconnected`. Nhờ vậy người chơi không bao giờ kẹt ở một màn khi kết nối treo mà không đóng (ví dụ đổi mạng trên điện thoại).
- Gói server tự đẩy (`seq = 0`) phát ra qua signal `pushed(env)`.
- Gửi không cần phản hồi (`MoveInput`) bằng `Net.send(env)`.
- Mỗi 5 giây gửi `Ping`, tính `ping_ms` từ `Pong`. Việc này cũng giữ kết nối không bị `IDLE_TIMEOUT`.
- `Kicked` hoặc mất kết nối: `Router` đưa về màn đăng nhập và hiện lý do (`core/errors.gd`). Không tự kết nối lại.

### 4.2. Cấu hình

`Config.ws_url` đọc project setting `demonslayer/ws_url` (mặc định `ws://localhost:8080/ws`), ghi đè bằng tham số chạy `-- --ws-url=ws://host:port/ws`. `-- --touch` bật joystick ảo trên PC.

## 5. Màn hình

Mọi màn dựng bằng code theo cách của `battle-squad-game`: file `.tscn` chỉ có node gốc gắn script (màn trong game có gốc là Node2D để chứa camera và không gian thế giới; các màn còn lại có gốc là Control full-rect); `_ready()` dựng con bằng builder ở `ui/ui.gd`; hình khối vẽ bằng `_draw()`.

### 5.1. Đăng nhập / đăng ký

- Ô tài khoản, ô mật khẩu (ẩn ký tự), nút chính và nút chuyển giữa đăng nhập và đăng ký.
- Nút chính tạm khóa khi đang chờ phản hồi.
- Đăng ký thành công thì tự đăng nhập luôn.
- `LoginReq` gửi `PROTOCOL_VERSION` (hằng trong `net.gd`, bằng `gateway.ProtocolVersion` của server). Đăng nhập xong thì sang màn chọn nhân vật.
- Lỗi hiện một dòng chữ đỏ dưới nút, lấy từ `core/errors.gd`. Thông báo do Router chuyển tới (bị đá, mất kết nối) hiện ở cùng chỗ.

### 5.2. Chọn nhân vật

- Ba ô theo slot 1–3. Ô có nhân vật vẽ hình vuông màu theo class, tên, cấp; ô trống vẽ dấu cộng.
- Chọn ô có nhân vật: nút "Vào game" (`EnterWorldReq`) và "Xóa".
- Xóa mở hộp xác nhận, phải gõ lại đúng tên (`DeleteCharReq.confirm_name`). Client chỉ chặn ô trống; server so tên sau chuẩn hóa NFC, trả `BAD_REQUEST` thì client báo "Tên gõ lại không khớp". (Godot không có chuẩn hóa NFC, nên so ở client sẽ chặn nhầm chữ tiếng Việt gõ dạng tổ hợp.)
- Chưa tải xong danh sách thì chưa hiện nút nào; tải lỗi thì hiện lỗi và nút "Thử lại".
- Chọn ô trống mở form: ô tên, 5 nút class (Viêm, Thủy, Minh, Nham, Phong) có màu riêng, nút "Tạo". Tạo xong thì danh sách làm mới.
- Nút "Đăng xuất" đóng kết nối, về màn đăng nhập.

### 5.3. Trong game

- **`map_view`** vẽ lưới va chạm của map từ `data/maps/<map_id>.json`: ô `#` tối, ô `.` sáng, đường lưới mờ. Một ô = 32 px (`TILE_PX`). Vẽ một lần vào một Node2D.
- **`entity_view`** cho mỗi thực thể: hình vuông 0,6 × 0,6 ô màu theo class, vạch ngắn chỉ hướng đang đi, tên ở trên. Nhân vật của mình có viền sáng.
- **`Camera2D`** đi theo nhân vật của mình, giới hạn trong biên map.
- **`hud`** (CanvasLayer, giữ một cỡ trên màn): map và zone, ping, tọa độ ô, nút "Về chọn nhân vật" (`LeaveWorldReq`, xong thì về màn chọn nhân vật).
- **`joystick`** (vẽ bằng `_draw`, góc trái dưới): hiện khi `DisplayServer.is_touchscreen_available()` hoặc có `--touch`; nhận cả chạm và chuột.

## 6. Điều khiển, dự đoán và nội suy

### 6.1. Đọc hướng

- Bàn phím: WASD và phím mũi tên, khai báo trong Input Map (`move_left`, `move_right`, `move_up`, `move_down`). Trục y của map hướng xuống, giống màn hình.
- Joystick: vector từ tâm tới ngón tay chia bán kính. Vùng chết 0,15. Đang kéo joystick thì joystick được ưu tiên hơn bàn phím.
- Hướng được kẹp độ dài tối đa 1 bằng `Movement.clamp_dir`, giống `ClampDir` của server.

### 6.2. `Movement` (port `move.go`)

`clamp_dir`, `step(grid, pos, dir, dist)` với cùng hằng số (`HALF_SIZE = 0.3`, `MAX_SUBSTEP = 0.25`, `EPS = 1e-4`): chia bước con, đi trục X rồi trục Y, đụng ô bị chặn thì dừng sát mép. `Vector2` của Godot là `float32`, khớp với `float32` của server nên hai bên tính ra cùng kết quả; sai lệch còn lại (nếu có) được 6.3 sửa.

### 6.3. Dự đoán vị trí của mình (`Predictor`)

- Bộ đếm thời gian chạy tick cục bộ với chu kỳ `1 / tick_rate` (từ `EnterWorldResp`). Mỗi tick cục bộ:
  1. Đọc hướng hiện tại. Nếu khác hướng gửi lần trước, gửi `MoveInput` với `input_seq` tăng dần. Gửi tối đa 20 gói/giây: hướng đổi quá nhanh thì giữ lại và gửi hướng mới nhất ở tick sau.
  2. `pos = Movement.step(grid, pos, dir, move_speed / tick_rate)`.
  3. Lưu `(seq, dir, from)` vào lịch sử, với `from` là vị trí trước bước của tick này. Lịch sử tối đa 2 giây tick; cũ hơn thì bỏ.
- Khi `WorldTick` có `EntityMove` của mình (vị trí server `P`, `ack = ack_input_seq`):
  1. Ứng viên gồm các mốc lịch sử có `seq == ack`, cộng thêm vị trí server đi xong input `ack`: `from` của mốc đầu tiên có `seq > ack`, hoặc vị trí hiện tại nếu `seq` gửi gần nhất bằng `ack`. Chọn ứng viên gần `P` nhất. Thiếu ứng viên cuối này thì khi mạng trễ từ 1 tick trở lên, mỗi lần rẽ hay dừng client sẽ sửa sai một bước rồi kéo lại.
  2. Bỏ lịch sử tới mốc đó. Gán vị trí tại mốc bằng `P`.
  3. Chạy lại `Movement.step` cho các mốc sau, theo `dir` đã lưu, ra vị trí hiện tại mới.
- Không được khớp với `seq` cũ hơn `ack`: ví dụ client đã đi 10 bước còn server mới đi 9 bước; nếu khớp với mốc cũ thì client coi như server đứng sau và kéo nhân vật lùi sai. Chỉ mốc `seq == ack` mới là trạng thái server đã xử lý.
- Predictor luôn đi theo hướng *đã gửi* gần nhất (`sent_dir`), không theo hướng đọc được lúc đó, để khớp với những gì server thực sự nhận.
- Server có thể bỏ `MoveInput` (vượt giới hạn, hộp thư zone đầy). Input mới nhất chưa được ack sau 0,5 giây (tính từ lần gửi hoặc lần ack tiến gần nhất) thì gửi lại `sent_dir` với `input_seq` mới. Mọi `WorldTick` đều báo ack cho predictor, kể cả khi nhân vật không đổi vị trí.
- Tick cục bộ chạy lệch nửa tick so với lúc nhận `EnterWorldResp`, để tick cục bộ và `WorldTick` không rơi vào cùng khung hình và thứ tự xử lý luôn ổn định.
- Lệch giữa vị trí vẽ cũ và vị trí mới dưới 0,5 ô thì kéo dần về trong 100 ms. Lệch từ 0,5 ô trở lên thì nhảy thẳng tới vị trí mới.
- Nhân vật của mình được vẽ nội suy giữa hai tick cục bộ, để mượt ở tốc độ khung hình cao hơn `tick_rate`.

### 6.4. Người chơi khác (`Interpolator`)

- Mỗi thực thể giữ một bộ đệm các mốc `(server_tick, pos, dir)` lấy từ `EntityEnter` và `WorldTick`. `EntityEnter` không có số tick; nó đến trước `WorldTick` của cùng tick, nên được gán tick của `WorldTick` sắp tới (lúc mới vào game là `server_tick` của `EnterWorldResp`).
- Thực thể đứng yên một lúc không có mốc mới; khi có mốc mới cách xa, chèn bản sao mốc cũ ở tick ngay trước để thực thể đứng yên rồi mới đi, không trượt dần từ mốc cũ.
- Thời điểm vẽ = tick server mới nhất nhận được − 2 tick, tiến theo thời gian thực giữa các lần nhận. Vị trí là nội suy tuyến tính giữa hai mốc kẹp thời điểm đó. Hết mốc thì đứng ở mốc cuối, không ngoại suy.
- `EntityEnter` tạo `entity_view`, `EntityLeave` xóa nó.

## 7. Xử lý lỗi

- `core/errors.gd` đổi mọi `ErrorCode` và `KickReason` thành câu tiếng Việt ngắn, ví dụ `AUTH_FAILED` → "Sai tài khoản hoặc mật khẩu", `DUPLICATE_LOGIN` → "Tài khoản đã đăng nhập ở nơi khác".
- Lỗi mạng của `request` (quá giờ, mất kết nối) → "Mất kết nối tới máy chủ".
- Gói không giải mã được thì bỏ và ghi `push_warning`. Server tự đóng kết nối nếu client gửi gói sai.
- Không có file map cho `map_id` server trả về: về màn chọn nhân vật, báo "Client thiếu dữ liệu map, hãy cập nhật".

## 8. Kiểm thử

| Loại | Nội dung | Cách chạy |
| --- | --- | --- |
| Unit core | `tests/unit/test_core.gd`, chia phần bằng comment `# ______________________TEST MOVEMENT_____________________`: MOVEMENT (các ca của `move_test.go`: không xuyên tường, trượt dọc tường, bước con, kẹp hướng, NaN), MAP (đọc file, ô ngoài biên bị chặn), PREDICTION (không lệch thì không sửa; lệch thì sửa đúng từ mốc và chạy lại; input chưa ack được giữ), INTERPOLATION (nội suy giữa hai mốc, hết mốc thì đứng yên), ERRORS (mọi mã đều có câu) | `tests/run.sh` (Godot headless) |
| Unit net | `tests/unit/test_net.gd`: mã hóa `Envelope` có `oneof` ra đúng byte do Go sinh trong `tests/fixtures/`, giải mã ngược lại; ghép phản hồi theo `seq`; quá giờ trả lỗi | `tests/run.sh` |
| Đồng bộ dữ liệu | Map trong `data/maps/` trùng với `server/maps/` | `tests/run.sh` |
| Smoke | Client headless chạy với server thật: đăng ký tài khoản ngẫu nhiên, tạo nhân vật, vào map, đi sang phải 1 giây, thấy `WorldTick` đổi vị trí, rời game | `DS_SERVER_URL=ws://localhost:8080/ws tools/smoke.sh`; không có biến thì bỏ qua |
| Chơi thử | `tools/run2.sh` mở hai cửa sổ client | Bằng tay, với `make run` ở repo server |

Fixture byte do một test của server sinh ra vào `testdata/client/` (phần `CLIENT FIXTURES` trong `internal/gateway/gateway_test.go`; ghi lại bằng `go test ./internal/gateway -run TestClientFixtures -update`, chạy thường thì so khớp và fail khi file cũ). `tools/sync_server.sh` copy chúng vào `tests/fixtures/`, nên hai bên dùng cùng một bộ mã hóa chuẩn.

## 9. Tiêu chí hoàn thành

1. `make run` ở repo server và `tools/run2.sh` ở repo client: hai cửa sổ đăng ký, tạo nhân vật, vào `kumotori_1`, thấy nhau di chuyển mượt.
2. Đi dọc và đâm vào tường không giật, không xuyên tường; khi không mất gói thì nhân vật của mình không bị kéo lại.
3. Joystick ảo chạy được với `-- --touch` bằng chuột.
4. Đăng nhập trùng: cửa sổ cũ về màn đăng nhập với "Tài khoản đã đăng nhập ở nơi khác".
5. Về chọn nhân vật rồi vào lại: đứng đúng chỗ đã rời.
6. `tests/run.sh` đạt, smoke test đạt với server đang chạy, `go test -race ./...` ở server đạt.
7. Repo client có `README.md` (cách lấy submodule, sinh proto, chạy test, chạy client) và `CLAUDE.md`.
