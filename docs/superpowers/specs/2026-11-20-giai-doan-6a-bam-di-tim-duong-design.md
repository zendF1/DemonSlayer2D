# Giai đoạn 6a — Bấm để đi, tìm đường A* và đi xuyên map

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Spec này thiết kế phần đầu của Giai đoạn 6 (Nhiệm vụ và auto) cho client Godot (`../DemonSlayer2DGame`) và tài liệu của server (repo này). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.5 (tự chạy tới mục tiêu), 4.3 (tự tìm đường) và 5.2 (lớp va chạm phục vụ A*); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như các spec trước, nhất là [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) (cổng, `portal_watch`, nạp lại màn World khi chuyển zone), [spec 4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) (NPC, `npc_loop`) và [spec client Giai đoạn 0](2026-11-19-giai-doan-0-client-design.md) (dự đoán di chuyển). Giai đoạn 3–5 được triển khai trước, nên spec này viết trên giao thức 13 và migration 00008.

## 1. Mục tiêu và phạm vi

### 1.1. Chia Giai đoạn 6

Lộ trình (spec Giai đoạn 0 mục 1.1) ghi Giai đoạn 6 gồm nhiệm vụ, tự tìm đường (A*), bấm để đi tới điểm, auto đánh. Chia bốn phần, làm theo thứ tự, mỗi phần xong là chơi được:

| Phần | Nội dung | Kết quả chơi được | Giao thức |
| --- | --- | --- | --- |
| **6a** (spec này) | Bấm đất để đi, tìm đường A* trong map, lộ trình qua cổng tới map khác, cửa sổ Bản đồ, đuổi mục tiêu vòng qua tường | Bấm một điểm hay một NPC ở map khác, nhân vật tự đi tới | 13 (không đổi) |
| 6b ([spec](2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md)) | NPC nhiệm vụ, chuỗi nhiệm vụ chính tuyến cấp 1–20 và ba mốc 30/40/50, sách kỹ năng, theo dõi nhiệm vụ bấm để đi | Làm chính tuyến, nhận sách rồi học chiêu | 14 |
| 6c ([spec](2026-11-20-giai-doan-6c-tu-dong-danh-design.md)) | Auto đánh quái phía client: chọn quái, dùng chiêu, uống bình, nhặt đồ, phản công, giữ bãi | Bật auto ở bãi, treo máy đánh quái | 14 (không đổi) |
| 6d ([spec](2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md)) | Ba nhiệm vụ hàng ngày, làm mới 00:00 giờ Việt Nam | Vòng chơi hằng ngày | 15 |

### 1.2. Mục tiêu 6a

Chạm vào mặt đất thì nhân vật tự đi tới điểm đó, vòng qua tường theo đường ngắn. Chạm vào NPC ở xa thì nhân vật đi vòng qua tường tới NPC rồi mở cửa sổ NPC. Nút **Bản đồ** trên HUD mở cửa sổ có bản đồ thu nhỏ của map hiện tại (chạm vào đâu thì đi tới đó) và danh sách các map cùng NPC của chúng; bấm **Đi tới** cạnh một NPC ở map khác thì nhân vật tự đi qua các cổng tới map đó và tới NPC. Khi bấm Đánh mà mục tiêu ở sau tường, nhân vật chạy vòng qua tường thay vì đứng ép vào tường. Server không đổi gì.

### 1.3. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chia giai đoạn | 6a, 6b, 6c, 6d như mục 1.1 | Mỗi phần một kế hoạch vừa sức và kết thúc bằng thứ chơi được; 6b và 6c đều dùng đường đi của 6a |
| Tìm đường ở đâu | Client. Server vẫn chỉ nhận `MoveInput` (hướng) như cũ; không có lệnh "đi tới điểm" | Server đã kiểm tra mọi bước đi; dự đoán của client vẫn khớp từng bước; không tốn CPU server cho A*; không thêm lệnh mới để kiểm tra |
| Lưới tìm đường | Ô của lớp va chạm, 8 hướng; đi chéo chỉ khi hai ô kề cạnh cũng đi được (không cắt góc tường); chi phí 1 cho ô ngang/dọc, √2 cho ô chéo; ước lượng octile | Hộp va chạm 0,6 × 0,6 đi chéo qua góc tường sẽ bị chặn; với luật này mọi đoạn giữa hai tâm ô liền nhau đều đi được |
| Làm mượt | Kéo dây: từ mỗi điểm, nối thẳng tới điểm xa nhất trên đường mà hộp **0,35** (lớn hơn hộp va chạm 0,3) quét dọc đoạn thẳng không chạm ô tường | Đường thẳng, ít lần đổi hướng nên ít `MoveInput`; khoảng hở 0,05 ô tránh cọ vào góc tường |
| Tránh cổng | Khi tìm đường, ô thuộc hình chữ nhật của mọi cổng coi như tường, trừ cổng là đích của chặng | Cổng nằm sát tường biên; đường đi dọc mép map không được vô tình bước vào cổng và chuyển map |
| Giới hạn tìm | Tối đa 4.096 ô được mở rộng mỗi lần (map 64 × 48 có 3.072 ô); hết giới hạn hoặc không có đường thì báo "Không tìm được đường" | Không bao giờ treo khung hình |
| Đi theo đường | Mỗi tick dự đoán, hướng là từ vị trí dự đoán tới điểm kế tiếp; chỉ gửi hướng mới khi lệch hướng đã gửi quá **3°** hoặc chuyển điểm; cách điểm kế tiếp không quá một bước tick thì chuyển điểm; bước cuối gửi hướng có độ dài nhỏ hơn 1 để dừng đúng đích | Ít `MoveInput` (tối đa một gói mỗi tick, dưới giới hạn 20/giây); dừng đúng chỗ, không đi quá đích |
| Kẹt | Không gần điểm kế tiếp thêm 0,05 ô trong 1 giây thì tìm lại đường từ vị trí hiện tại; tìm lại 3 lần liên tiếp vẫn kẹt thì dừng và báo "Không tìm được đường" | Quái và người chơi không chặn đường, nên kẹt chỉ do sai số trượt theo tường; tìm lại là đủ |
| Chạm đất | Chạm chỗ không có quái, đồ, NPC trong tầm chọn thì đi tới đó. Chạm ô tường thì đi tới ô đi được gần nhất trong 3 ô (khoảng cách tâm ô); không có thì không đi | Thao tác quen thuộc trên điện thoại; chạm nhầm sát tường vẫn đi được |
| Thứ tự chạm | NPC (0,6 ô) → đồ dưới đất của mình → quái hay người chơi chọn được → mặt đất | Giữ thứ tự của 4b và 3a; mặt đất là lựa chọn cuối |
| Hủy đường đi | Tự di chuyển (joystick, phím), bấm Đánh, chọn chiêu để dùng, chết, chạm điểm khác (thay đích), rời màn World. Bị quái đánh không hủy | Người chơi luôn giành lại quyền điều khiển ngay; game-design 4.3 không tự đánh trên đường |
| Đi xuyên map | Tìm chuỗi cổng ít nhất bằng BFS trên đồ thị cổng đọc từ các file map đã đồng bộ; bằng nhau thì lấy cổng đứng trước trong file map. Chỉ đi bộ qua cổng; **không tự dùng Xa phu hay Giấy về thành** | Game-design 4.3 "tìm chuỗi cổng phù hợp"; Xa phu và giấy tốn Xu hay vật phẩm, chỉ người chơi tự quyết |
| Qua cổng khi đang đi | Đích của chặng là tâm hình chữ nhật cổng; `portal_watch` (4a) tự gửi `UsePortalReq` khi bước vào. Lộ trình lưu ở `Router.travel`, sống qua lần nạp lại màn World và tiếp tục ở map mới | Một đường chuyển zone (4a); không có code chuyển map riêng cho tìm đường |
| Chuyển zone lỗi | Lỗi của `UsePortalReq` thì hủy lộ trình và báo lỗi như 4a | Tránh vòng lặp đi vào cổng hỏng |
| Khu | Lộ trình không đổi khu; qua cổng chọn khu theo luật 4a | Đổi khu là việc người chơi tự quyết |
| Tới NPC | Đích là vị trí NPC với bán kính dừng 2,5 ô (tầm `npc_loop` của 4b); tới nơi thì mở cửa sổ NPC như khi chạm | Cùng luật tầm NPC; một cú chạm là đủ |
| Cửa sổ Bản đồ | Nút **Bản đồ** cạnh tên map trên HUD. Phần trên: bản đồ thu nhỏ của map hiện tại (ô tường, khu an toàn, NPC, cổng, vị trí mình), chạm để đi. Phần dưới: danh sách map đi tới được, mỗi map ghi tên, khoảng cấp quái, các NPC (tên, vai trò) và nút **Đi tới** (tới NPC hoặc tới điểm `default` của map) | Đi tới điểm xa ngoài màn hình; chọn đích ở map khác mà không cần nhiệm vụ |
| Đuổi mục tiêu vòng qua tường | Khi vòng đánh, vòng chiêu, vòng nhặt hay `npc_loop` muốn đi tới mục tiêu mà đường thẳng bị tường chắn (kiểm tra hộp 0,35), hướng đi lấy từ đường A* tới vị trí mục tiêu; tìm lại khi mục tiêu đã dời quá 1 ô so với đích đường cũ, tối đa 2 lần mỗi giây | Game-design 2.5 "tự chạy đến đủ tầm"; trước đây nhân vật trượt dọc tường và có thể kẹt ở góc |
| Hiển thị | Vòng tròn nhỏ ở đích và chấm mờ mỗi 0,5 ô dọc phần đường còn lại; khi đi xuyên map, dòng thông báo "Đang đi tới <tên map>" | Người chơi biết nhân vật đang đi đâu |
| Tốc độ | Không đổi, như đi tay | Không có lợi thế khi bấm để đi |

### 1.4. Không làm ở 6a

Thay đổi server hoặc giao thức; tự dùng Xa phu hay Giấy về thành khi đi xa; bản đồ thế giới vẽ hình; bản đồ nhỏ thường trực ở góc màn hình; đi tới đồng đội; tránh quái trên đường; nhiệm vụ (6b); auto đánh (6c).

## 2. Dữ liệu

Không có file dữ liệu mới. Client đọc các trường sẵn có của `data/maps/*.json` (đồng bộ từ server bằng `tools/sync_server.sh`): `collision`, `safe_zones`, `spawn_points`, `portals`, `npcs`, `regions`. Map khác map hiện tại được đọc khi cần và giữ trong bộ nhớ đệm (như `map_grid.name_of` của 4a).

## 3. Server

Không đổi code, không đổi giao thức (`ProtocolVersion` vẫn là 13), không migration. Hai ràng buộc đã có bảo đảm mọi đích tìm đường đều tới được:

- `mapdata` kiểm tra mọi điểm xuất hiện, ô của mỗi cổng, ổ quỷ, lửa trại và NPC tới được từ `default` theo bốn hướng (4a mục 2.1, 4b mục 2.1). Liên thông bốn hướng kéo theo liên thông tám hướng không cắt góc.
- Script `mapgen.py` kiểm tra mỗi cổng A→B có cổng B→A và mọi map tới được từ `kumotori_1` (4a mục 2.2), nên đồ thị cổng liên thông hai chiều.

## 4. Client Godot

### 4.1. Lõi (`core/`, không dùng Node)

**`core/pathfinder.gd`** (mới, hàm tĩnh, `grid` là đối tượng có `blocked(x, y)`, `width`, `height`):

- `find(grid, from: Vector2, to: Vector2, avoid := []) -> PackedVector2Array`: đường đã làm mượt, không gồm `from`, kết thúc đúng tại `to` khi ô của `to` đi được; luôn có ít nhất một điểm (đích) khi tới được, kể cả khi `from` và `to` cùng ô. Rỗng chỉ khi không có đường. `avoid` là các `Rect2i` (cổng không phải đích) mà ô bên trong coi như tường, cả trong A* lẫn trong `clear`.
  - Ô bắt đầu là ô của `from`. Ô đích là ô của `to`; nếu ô đó là tường thì dùng `nearest_walkable(to, 3)` và đường kết thúc ở tâm ô đó.
  - A* trên ô theo mục 1.3, hàng đợi ưu tiên là đống nhị phân; bằng `f` thì lấy `h` nhỏ hơn, rồi chỉ số ô (y × width + x) nhỏ hơn, nên kết quả xác định.
  - Đường thô là chuỗi tâm ô; kéo dây theo mục 1.3, với điểm đầu là `from` thật và điểm cuối là `to` thật (hoặc tâm ô đích thay thế).
- `clear(grid, a: Vector2, b: Vector2, half := 0.35, avoid := []) -> bool`: lấy mẫu dọc đoạn ab mỗi 0,1 ô (gồm hai đầu); với mỗi mẫu, hình vuông [x − half, x + half] × [y − half, y + half] không chạm ô tường nào (ô ngoài map là tường).
- `nearest_walkable(grid, pos: Vector2, max_r: int) -> Variant`: tâm ô đi được gần `pos` nhất trong bán kính `max_r` ô (khoảng cách từ `pos` tới tâm ô), bằng nhau thì chỉ số ô nhỏ hơn; `null` nếu không có.
- `length(from, path) -> float`: tổng độ dài để hiển thị và kiểm thử.

**`core/walker.gd`** (mới): đi theo một đường, gọi mỗi tick dự đoán.

- `start(grid, from: Vector2, goal: Vector2, radius: float, avoid := []) -> bool`: tìm đường (`pathfinder.find`, cùng `avoid` cho mọi lần tìm lại); false và trạng thái `"failed"` khi không có đường. `radius` là bán kính dừng (0 = tới đúng điểm).
- `step(pos: Vector2, step_dist: float, now: float) -> Vector2`: hướng cho tick này theo mục 1.3. Trả `Vector2.ZERO` và trạng thái `"arrived"` khi `pos` cách đích không quá `radius` (hoặc không quá 0,01 ô khi `radius` là 0). Khi còn một bước và `radius` là 0: trả `(đích − pos) / step_dist` (độ dài < 1).
- Giữ hướng: nếu hướng mới lệch hướng trả ở tick trước không quá 3° và chưa chuyển điểm thì trả lại đúng hướng cũ (cùng giá trị, nên `predictor` không gửi gói mới).
- Kẹt: theo mục 1.3, tính trên khoảng cách tới điểm kế tiếp và thời gian `now`; tìm lại thì đường mới bắt đầu từ `pos`; lần thứ ba thì trạng thái `"failed"`.
- `chase(grid, pos: Vector2, target: Vector2, now: float, avoid := []) -> Vector2`: dùng cho các vòng đánh, chiêu, nhặt và NPC (`avoid` là mọi cổng của map). `clear(pos, target)` thì trả hướng thẳng tới `target` (như hiện nay). Nếu không, đi theo đường A* tới `target`, tìm lại khi `target` cách đích đường cũ quá 1 ô và đã qua 0,5 giây từ lần tìm trước; không có đường thì trả hướng thẳng.
- `status` (`"idle"`, `"walking"`, `"arrived"`, `"failed"`), `remaining() -> PackedVector2Array` (để vẽ), `cancel()`.

**`core/world_graph.gd`** (mới): đồ thị cổng giữa các map.

- `load(map_id) -> bool` đọc file map (bộ nhớ đệm theo id); `portals(map_id) -> Array` (`{id, rect: Rect2i, to_map, to_spawn}` theo thứ tự trong file), `spawn(map_id, spawn_id) -> Variant` (`Vector2`), `npcs(map_id) -> Array` (`{id, name, role, pos}`), `name_of(map_id)`, `level_range(map_id) -> Vector2i` (cấp thấp nhất và cao nhất của các vùng quái; `(0, 0)` khi không có).
- `route(from_map, to_map) -> Variant`: mảng cổng phải đi qua theo thứ tự (`[]` khi cùng map), `null` khi không tới được. BFS theo map, duyệt cổng của mỗi map theo thứ tự trong file; map được đọc dần qua `to_map`.
- `reachable(from_map) -> Array[String]`: mọi map tới được, theo thứ tự BFS (cho cửa sổ Bản đồ).

**`core/travel.gd`** (mới): một lộ trình có thể qua nhiều map.

- Đích (`goal`) là `Dictionary`: `map` (id map), `pos` (`Vector2`), `radius`, `kind` (`"point"`, `"npc"`; 6b thêm `"region"`), `ref` (id NPC khi `kind` là `"npc"`), `label` (tên hiển thị). Đích có thể chuyển qua lại với `Router.travel` (chỉ chứa kiểu dữ liệu đơn giản).
- `begin(goal, map_id, pos, graph, grid) -> String`: `""` khi bắt đầu được; `"no_route"` khi `route` là `null`; `"no_path"` khi không có đường trong map hiện tại.
- Chặng hiện tại: nếu `map_id` là map đích thì chặng tới `goal.pos` với `goal.radius`, tránh mọi cổng; nếu không thì chặng tới tâm hình chữ nhật của cổng đầu tiên trong `route(map_id, goal.map)`, bán kính 0, tránh mọi cổng khác.
- `step(pos, step_dist, now) -> Vector2` gọi `walker.step`; `arrived() -> bool` chỉ đúng khi đang ở map đích và chặng cuối đã tới.
- `resume(map_id, pos, graph, grid) -> String`: gọi sau khi nạp lại màn World ở map mới; tính lại chặng như `begin`.
- `cancel()`.

**`core/npc_loop.gd`** (4b): hướng đi lấy từ `walker.chase` thay cho đường thẳng. Mọi luật khác giữ nguyên.

**`core/attack_loop.gd`, `core/cast_loop.gd`, `core/pickup_loop.gd`**: không đổi; `screens/world.gd` thay hướng đi khác 0 mà các vòng trả ra bằng `walker.chase(grid, pos, vị trí mục tiêu, now)` (mục 4.3).

### 4.2. Router

`autoload/router.gd` thêm `travel` (`Dictionary` đích của mục 4.1, hoặc `null`). Đặt khi bắt đầu lộ trình xuyên map; xóa khi tới nơi, hủy, lỗi chuyển zone, hay về màn chọn nhân vật.

### 4.3. Màn World

`screens/world.gd` chỉ nối dây; mọi luật ở `core/`.

- **Hướng đi mỗi tick dự đoán.** Thứ tự ưu tiên: hướng tay (joystick, phím) → hướng các vòng đánh/chiêu/nhặt/NPC (đã qua `walker.chase`) → `travel.step` → đứng yên. Vì đường đi cần vị trí dự đoán của đúng tick đó, `travel.step` được gọi trong vòng tick của `predictor` (không phải một lần mỗi khung hình).
- **Chạm.** `_pick` theo thứ tự ở mục 1.3. Chạm đất: `travel.begin({map: map hiện tại, pos, radius: 0, kind: "point"})`; lỗi `"no_path"` thì báo "Không tìm được đường".
- **Hủy.** Theo mục 1.3: hướng tay khác 0, nút Đánh vừa được giữ, chọn ô chiêu rồi giữ Đánh, chết, rời màn World.
- **Qua cổng.** Khi chặng là cổng, `portal_watch` tự gửi `UsePortalReq` như 4a; trong lúc chờ trả lời nhân vật đứng yên như 4a. Thành công: `Router.travel` còn nguyên, màn World nạp lại và gọi `travel.resume` ở `_ready`. Lỗi: hủy lộ trình, xóa `Router.travel`, báo lỗi như 4a.
- **Tới NPC.** `travel.arrived()` với `kind` `"npc"` thì mở cửa sổ NPC đó như khi `npc_loop` báo "mở"; với `"point"` thì chỉ dừng.
- **Cổng không thuộc lộ trình.** Đi tay hay chạm đất vào một cổng thì vẫn qua cổng như 4a; khi đó không có lộ trình nào được tiếp tục.

### 4.4. Giao diện

- **Đường đi** (`ui/path_view.gd`, mới, `Node2D` vẽ bằng `_draw` trong không gian thế giới): vòng tròn bán kính 0,3 ô ở đích của chặng hiện tại, chấm bán kính 0,06 ô mỗi 0,5 ô dọc `walker.remaining()`, màu trắng mờ. Ẩn khi không có lộ trình.
- **Nút Bản đồ** (`ui/hud.gd`): nút tròn nhỏ cạnh tên map và khu (4a); mở cửa sổ Bản đồ.
- **Cửa sổ Bản đồ** (`ui/map_window.gd`, mới):
  - Bản đồ thu nhỏ của map hiện tại vẽ bằng `_draw` từ `map_grid` (ô tường tối, ô đi được sáng, khu an toàn xanh nhạt, cổng tím, NPC chấm màu theo vai trò như `map_view`, mình chấm vàng), vừa khung theo chiều rộng màn hình dọc. Chạm vào bản đồ thì bắt đầu lộ trình tới điểm đó như chạm đất (ô tường thì ô đi được gần nhất) và đóng cửa sổ.
  - Danh sách map theo `world_graph.reachable`: map hiện tại ghi "(Đang ở)"; mỗi map một dòng tiêu đề "<tên map> · cấp a–b" (map thành ghi "Thành") và dưới đó mỗi NPC một dòng "<tên NPC> · <vai trò>" với nút **Đi tới**; dòng tiêu đề của map khác có nút **Đi tới** tới điểm `default`. Bấm thì bắt đầu lộ trình (`kind` `"npc"` với bán kính 2,5, hoặc `"point"` với bán kính 0) và đóng cửa sổ.
  - Tên vai trò: `shop` "Cửa hàng", `blacksmith` "Thợ rèn", `carriage` "Xa phu", `storage` "Thủ kho" (6b thêm `quest` "Nhiệm vụ").
- **Thông báo**: bắt đầu lộ trình xuyên map: "Đang đi tới <tên map>"; không có đường: "Không tìm được đường".

### 4.5. Smoke

`tests/smoke/smoke.gd` sau các bước cũ: về Kumotori điểm `default` (dùng `devchar -map kumotori_1 -x 5.5 -y 5.5` như 4c), bắt đầu lộ trình tới NPC `tap_hoa` của `ubuyashiki` bằng `core/travel.gd`, chạy `predictor` và gửi `MoveInput` như màn World; kiểm tra client tự gửi `UsePortalReq`, nhận `EnterWorldResp` map `ubuyashiki`, `travel.resume` rồi tới cách Tạp hóa không quá 2,5 ô trong 60 giây. In "SMOKE OK" kèm "travel kumotori_1 → ubuyashiki".

## 5. Kiểm thử

**Server:** không có test mới (không đổi code).

**Client** (`tests/run.sh`, mục mới `# ____TEST PATHFINDING____`, `# ____TEST TRAVEL____`):

- `pathfinder` trên lưới nhỏ dựng trong test: đường thẳng trên nền trống là một điểm (đích), kể cả khi cùng ô; ô trong `avoid` bị đi vòng; vòng qua một bức tường; ra khỏi chữ U; không cắt góc (hai ô chéo nhau bị tường hai bên kề thì phải đi vòng); không có đường trả rỗng; đích là ô tường dùng ô đi được gần nhất, không có ô nào trong 3 ô thì rỗng; mọi đoạn của kết quả qua `clear`; độ dài không quá 1,01 lần đường octile ngắn nhất trên lưới trống có một vật cản; cùng đầu vào cho cùng kết quả.
- `pathfinder` trên **map thật** đã đồng bộ: từ `default` của mỗi map tới mọi NPC, mọi cổng (tránh các cổng khác) và mọi điểm xuất hiện (tránh mọi cổng) đều có đường, và không đoạn nào chạm ô cổng bị tránh; mỗi lần tìm mở rộng không quá 4.096 ô.
- `walker` mô phỏng bằng `movement.gd` với bước tick 0,4 ô trên map thật: từ `default` của mỗi map tới tâm mỗi cổng và tới từng NPC (bán kính 2,5) đều `"arrived"` trong số tick không quá 1,5 lần độ dài đường chia bước tick cộng 20; số lần đổi hướng không quá số điểm của đường cộng 2; dừng đúng đích với sai số 0,01 khi bán kính 0.
- `walker`: lệch dưới 3° trả lại đúng hướng cũ; kẹt (lưới bị đổi thành tường ngay trước nhân vật sau khi bắt đầu) thì tìm lại, lần thứ ba thì `"failed"`; `chase` đi thẳng khi không có tường chắn và đi theo đường khi có; mục tiêu dời 2 ô thì tìm lại sau 0,5 giây.
- `world_graph` trên map thật: `route("kumotori_1", "fujikasane_1")` là bốn cổng `to_ubuyashiki`, `to_sagiri_1`, `to_sagiri_2`, `to_fujikasane_1`; cùng map là `[]`; map không có trả `null`; `reachable("kumotori_1")` đủ năm map; `level_range("sagiri_1")` là (6, 10), của `ubuyashiki` là (0, 0).
- `travel`: ở Kumotori với đích NPC ở Sagiri 1, chặng đầu là tâm cổng `to_ubuyashiki`; `resume` ở Ubuyashiki cho chặng tới tâm cổng `to_sagiri_1`; `resume` ở Sagiri 1 cho chặng tới NPC với bán kính 2,5; `arrived` chỉ đúng ở map đích; đích chuyển qua `Dictionary` của `Router` vẫn dùng được.
- Smoke theo mục 4.5.

**Nghiệm thu:** server ở `:8199`. `test1` ở trại Kumotori chạm một điểm sau dãy tường thì nhân vật đi vòng tới đó; mở Bản đồ, bấm Đi tới Thợ rèn ở Tổng hành dinh: nhân vật tự qua cổng, tới Thợ rèn và mở cửa sổ NPC; khóa một quái sau tường rồi giữ Đánh: nhân vật chạy vòng tới đánh; đẩy joystick giữa đường thì dừng lộ trình.

## 6. Tài liệu cập nhật

- `docs/game-design.md` mục 4.3 (A* tám hướng phía client trên lớp va chạm; chuỗi cổng ít nhất; không tự dùng Xa phu hay giấy; cửa sổ Bản đồ), mục 2.5 (tự chạy tới mục tiêu vòng qua tường).
- Spec Giai đoạn 0 mục 1.1: ghi chú Giai đoạn 6 chia 6a/6b/6c/6d.
- `CLAUDE.md` (server): bất biến "tìm đường và bấm để đi chạy ở client; server chỉ nhận hướng của `MoveInput`, không có lệnh đi tới điểm".
- `CLAUDE.md` (client): tìm đường ở `core/pathfinder.gd`, đi theo đường ở `core/walker.gd`, lộ trình nhiều map ở `core/travel.gd` và `core/world_graph.gd`; `screens/world.gd` chỉ nối dây.
- README client: chạm đất để đi, cửa sổ Bản đồ, Đi tới.
