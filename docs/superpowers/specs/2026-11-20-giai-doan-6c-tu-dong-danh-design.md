# Giai đoạn 6c — Auto đánh quái

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ ba của Giai đoạn 6 (xem bảng chia ở [spec 6a](2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 8 (auto-play), 2.5 (chọn mục tiêu), 2.7 (bình thuốc), 5.7 (bảng PK, phản công) và 4.3 (chạm nhiệm vụ không bật auto); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như spec 6a, [spec 6b](2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md), [spec 5a](2026-11-20-giai-doan-5a-pk-phat-exp-design.md) (ứng viên mục tiêu, tự khóa người vừa đánh mình), [spec 4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) (bình thuốc, `potion_rules`) và [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) (`pickup_loop`). 6c chạy sau khi 6b đã triển khai (giao thức 14, migration 00009).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Trên HUD có nút **Auto**. Bấm khi đang đứng ở bãi quái thì nhân vật tự đánh quái quanh chỗ đó: chọn quái gần nhất trong bãi, dùng chiêu theo thứ tự ưu tiên và chém khi chiêu đang hồi, tự uống bình khi HP hoặc MP thấp, tự nhặt đồ của mình theo bộ lọc, tự quay về bãi khi đi quá xa. Bị người chơi khác đánh thì quay sang phản công người đó. Giữ nút Auto mở cửa sổ cài đặt. Chết thì auto tắt; sau khi hồi sinh có nút **Tiếp tục auto** đưa nhân vật về bãi cũ và bật lại. Server không đổi gì.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Auto chạy ở đâu | Client (`core/auto_play.gd`), lái đúng các vòng đã có (`targeting`, `attack_loop`, `cast_loop`, `pickup_loop`, `potion_rules`, đường đi 6a). Server không có lệnh auto và không biết ai đang auto | Game-design 8: "cùng quy tắc kỹ năng và kiểm tra server như thao tác tay"; mọi lệnh vẫn qua kiểm tra và giới hạn tần suất của server; không thêm tải server |
| Treo khi tắt máy hay thoát game | Không có; auto chỉ chạy khi client đang chạy màn World | Auto phía server là một hệ thống khác hẳn (mô phỏng nhân vật vắng mặt); ngoài v1 |
| Điều kiện bật | Còn sống, map có vùng quái (không phải `town`), đứng ngoài khu an toàn | Không có gì để đánh ở thành hay trong khu an toàn |
| Bãi gốc | Vị trí, map và khu lúc bật; bán kính **10 ô** | Game-design 8 "giữ phạm vi bãi gốc"; 10 ô phủ một vùng quái của map nhỏ (4a) |
| Chọn quái | Chỉ quái còn sống, nằm trong bán kính bãi (tính từ tâm bãi), gần mình nhất; bằng nhau thì id nhỏ hơn; bỏ qua id đang bị `targeting` tạm bỏ (5 giây sau `INVALID_TARGET`). Có quái ngoài 6 ô vẫn chọn, các vòng tự chạy tới (đường đi 6a vòng tường) | Game-design 2.5 "ưu tiên gần nhất"; trong bãi, không kéo nhân vật đi khắp map |
| Giữ mục tiêu quái | Đánh tới chết; bỏ khi quái ra ngoài bán kính bãi + 2 ô hoặc ngoài tầm giữ khóa 8 ô (luật `targeting`) | Không đuổi quái chạy về vùng khác mãi |
| Không có quái | Ngoài bán kính bãi thì đi về tâm bãi (đường đi 6a); trong bán kính thì đứng chờ quái hồi sinh | Quái thường hồi sinh 8 giây tại ổ quỷ trong vùng |
| Đánh người | Auto **không bao giờ tự chọn người chơi** làm mục tiêu, kể cả khi bật PK; chỉ phản công | Không biến auto thành công cụ đi giết người; game-design 8 chỉ nêu phản công |
| Phản công | Bị một người chơi đánh trúng (`Hit` lên mình, người đánh là người chơi) mà người đó đang là ứng viên đánh được (bảng PK 5a, không cùng tổ đội) thì chuyển mục tiêu sang người đó, kể cả khi đang đánh quái | Game-design 8 "tự chuyển mục tiêu sang người tấn công để phản công" |
| Ngừng truy đuổi | Bỏ người đó khi: không còn là ứng viên (vào khu an toàn, tắt PK mà mình cũng tắt, chết, rời tầm nhìn), cách mình quá 8 ô, ra ngoài bán kính bãi + 5 ô, hoặc **10 giây** không đánh trúng mình | 10 giây khớp khóa giao tranh PvP của 5a; game-design 10.5 để mở |
| Nhiều người đánh | Giữ người đang phản công cho tới khi bỏ theo luật trên; khi bỏ thì chuyển sang người đã đánh trúng mình **gần đây nhất** trong 10 giây còn là ứng viên; không còn ai thì quay lại đánh quái | Không đổi mục tiêu liên tục; game-design 10.5 để mở |
| Chiêu | Thứ tự ưu tiên cố định ô thanh chiêu 5 → 4 → 3 → 2 (chiêu học sau mạnh hơn trước), rồi chém. Mỗi ô bật hay tắt trong cài đặt (mặc định bật hết). Chọn ô đầu tiên: đã học, hết hồi, đủ MP, và có mục tiêu nếu chiêu cần mục tiêu. Không có ô nào thì chém | Game-design 8 "dùng kỹ năng theo thứ tự ưu tiên"; một thứ tự cố định dễ hiểu, cài đặt chỉ là bật/tắt |
| Bình | Uống bình HP khi HP < ngưỡng HP, bình MP khi MP < ngưỡng MP; mặc định 50% và 30%, chỉnh 10–90% bước 10; chọn bình như nút HUD (4b, `potion_rules.best_slot`) | Game-design 8 (50% là giá trị tham khảo) |
| Nhặt | Đồ của mình dưới đất trong bán kính bãi, khi auto **không có mục tiêu**; bộ lọc trang bị: Không nhặt / Tất cả / Từ Xanh / Từ Vàng (mặc định Từ Xanh); đá: nhặt / không (mặc định nhặt). Hành trang không chứa được món đó thì bỏ qua món đó; lần đầu gặp hành trang đầy thì báo "Hành trang đầy, auto không nhặt nữa" | Game-design 8 "có thể bỏ đồ Trắng"; nhặt giữa hai con, không bỏ dở trận |
| Tự hồi sinh, về thành mua bình | Không làm ở v1. Chết thì auto tắt. Sau khi hồi sinh (bất kỳ cách nào) HUD hiện nút **Tiếp tục auto**: đi về bãi gốc bằng lộ trình 6a (qua cổng nếu cần), tới nơi thì bật lại auto với bãi và cài đặt cũ | Game-design 10.5 để mở chi phí hồi sinh và tiếp tế; hồi sinh tại chỗ tốn Lượng nên phải do người chơi chọn; tiếp tục bằng một chạm là đủ tiện |
| Hết bình | Tiếp tục đánh; không tự đi mua | Như trên |
| Tắt auto | Bấm nút Auto; tự di chuyển (joystick, phím); chạm đất, NPC hay đồ dưới đất; chạm dòng nhiệm vụ hay Đi tới; chết; chuyển zone (cổng, đổi khu, giấy về thành, xa phu) trừ lộ trình Tiếp tục auto | Người chơi luôn giành lại quyền điều khiển ngay; game-design 4.3: chạm nhiệm vụ chỉ tìm đường, không bật auto |
| Chạm quái hay người chơi khi đang auto | Đổi mục tiêu sang con đó, auto vẫn chạy | Cho phép chỉ định con muốn đánh trước |
| Nút Đánh, chọn ô khi đang auto | Bỏ qua; ô đang chọn trên thanh chiêu không đổi | Auto tự chọn chiêu riêng, không làm rối ô của người chơi |
| Cài đặt | Lưu trên máy theo tên nhân vật (`user://auto.cfg`) | Không cần server; mỗi nhân vật một cách chơi |
| Tổ đội | Không có luật riêng (không đi theo trưởng nhóm); đồng đội không bao giờ là ứng viên (5b) | Theo trưởng nhóm là tính năng khác; ngoài v1 |
| Hiển thị | Nút Auto sáng khi bật, dòng "Auto" nhấp nháy dưới thẻ nhân vật, vòng tròn mờ bán kính bãi trên map | Biết auto đang chạy và giữ ở đâu |

### 1.3. Bổ sung cho game-design

Game-design 8 và 10.5 để mở các điểm: chọn đối thủ khi nhiều người tấn công, giới hạn truy đuổi, thời điểm quay lại đánh quái, điều kiện và chi phí tự hồi sinh và tiếp tế. Spec này chốt chúng ở mục 1.2 và ghi vào game-design (mục 6).

### 1.4. Không làm ở 6c

Auto phía server hay khi thoát game; tự hồi sinh; tự về thành mua bình hay bán đồ; tự đổi khu khi bãi đông; theo trưởng nhóm; chọn quái theo loại hay cấp; thứ tự chiêu tự sắp; tự cộng điểm; giới hạn thời gian auto; thay đổi server hay giao thức.

## 2. Dữ liệu và server

Không có dữ liệu mới. Server không đổi code, giao thức vẫn là **14**, không migration.

## 3. Client Godot

### 3.1. Lõi (`core/`, không dùng Node)

**`core/auto_settings.gd`** (mới): `slots: Array[bool]` (4 ô chiêu, ô thanh 2–5), `hp_pct := 50`, `mp_pct := 30`, `pick_equipment := "blue"` (`"none"`, `"all"`, `"blue"`, `"gold"`), `pick_stones := true`; `to_dict()`, `from_dict(d)` (giá trị sai thì về mặc định, phần trăm kẹp 10–90 và làm tròn về bội 10); `wants(item, book) -> bool` theo bộ lọc (trang bị theo phẩm chất: `"blue"` là Xanh trở lên, `"gold"` là Vàng trở lên; vật phẩm chồng là đá cường hóa, đá tẩy luyện theo `pick_stones`; vật phẩm chồng khác luôn nhặt).

**`core/auto_play.gd`** (mới): quyết định mỗi khung hình, không gửi gì.

- Hằng số: `RADIUS := 10.0`, `KEEP_MONSTER := 2.0` (thêm ngoài bán kính bãi), `KEEP_PLAYER := 5.0`, `PVP_FORGET_SEC := 10.0`.
- `start(map_id: String, zone: int, pos: Vector2, map_type: String, in_safe: bool, alive: bool) -> String`: `""` khi bật được; `"dead"`, `"town"`, `"safe"` khi không. Ghi bãi gốc `{map, zone, pos}`.
- `stop(reason: String)`; `active: bool`; `anchor: Dictionary`; `last_stop: String`.
- `on_hit_by_player(attacker_id: int, now: float)`: ghi thời điểm người đó đánh trúng mình.
- `step(ctx: Dictionary) -> Dictionary`. `ctx` gồm: `now`, `pos`, `hp`, `max_hp`, `mp`, `max_mp`, `level`, `target_id`, `monsters` (`[{id, pos}]` còn sống), `players` (ứng viên người chơi đánh được theo 5a, `[{id, pos}]`), `items` (đồ của mình dưới đất `[{id, pos, item}]`), `can_take: Callable(item) -> bool`, `skipped: Callable(id) -> bool`, `slot_ready: Callable(slot) -> bool` (đã học, hết hồi, đủ MP), `slot_needs_target: Callable(slot) -> bool`, `potion_block: Callable(group) -> String` (luật `potion_rules.block`). Trả:
  - `target`: id mục tiêu mong muốn (0 là không có);
  - `slot`: ô thanh chiêu để dùng (1–5) và `hold: true` khi có mục tiêu;
  - `potion`: `"hp"`, `"mp"` hoặc `""`;
  - `pickup`: id đồ cần nhặt hoặc 0;
  - `home`: `Vector2` tâm bãi khi cần đi về, hoặc `null`;
  - `notice`: câu thông báo một lần, hoặc `""`.
- Thứ tự quyết định trong `step`: (1) bình: HP dưới ngưỡng và `potion_block("hp") == ""` thì `"hp"`, nếu không thì xét MP; (2) phản công theo mục 1.2; (3) giữ hay bỏ mục tiêu quái hiện tại; (4) không có mục tiêu: đồ cần nhặt gần nhất trong bán kính bãi mà `wants` và `can_take` (món không chứa được bị bỏ qua và báo một lần khi lần đầu gặp); (5) chọn quái theo mục 1.2; (6) không có gì để làm: cách tâm bãi lớn hơn `RADIUS` thì trả `home`, ngược lại đứng chờ. Chọn ô chiêu theo mục 1.2 khi có mục tiêu.

**`core/targeting.gd`**: không đổi; auto đặt mục tiêu bằng `select` sẵn có.

### 3.2. Router

`autoload/router.gd` thêm `auto_anchor` (`Dictionary` bãi gốc gần nhất hoặc `null`) và `auto_resume: bool` (đang trên lộ trình Tiếp tục auto). Xóa khi về màn chọn nhân vật.

### 3.3. Màn World

`screens/world.gd` chỉ nối dây:

- Mỗi khung hình khi `auto.active`: dựng `ctx` từ `predictor`, `entity_table`, `inventory`, `skill_book`, `potion_rules`, `world_entities.candidates` (5a); gọi `auto.step`; `targeting.select(target)` khi khác mục tiêu hiện tại; dùng `slot`/`hold` thay cho ô thanh chiêu và nút Đánh khi chạy `cast_loop`/`attack_loop`; `potion` gửi `UseItemReq` qua đúng đường của nút bình HUD; `pickup` chạy `pickup_loop.start(id)` (khi `pickup_loop` chưa bận); `home` dùng `walker.chase(grid, pos, home, now)`.
- `Hit` lên chính mình với người đánh là người chơi: `auto.on_hit_by_player`.
- Tắt auto theo mục 1.2 (hướng tay khác 0, chạm đất/NPC/đồ, bắt đầu một lộ trình khác, chết, gửi lệnh chuyển zone không thuộc Tiếp tục auto). Chạm quái hay người chơi chỉ đổi mục tiêu.
- Bật auto lưu `Router.auto_anchor`. Sau hồi sinh (`RespawnResp` hoặc màn World nạp lại khi còn `auto_anchor`), HUD hiện **Tiếp tục auto**; bấm thì `Router.auto_resume = true` và bắt đầu lộ trình `{map, pos, radius: 1, kind: "point"}` tới bãi gốc; tới nơi (kể cả sau khi qua cổng và nạp lại màn World) thì `auto.start` với bãi cũ, `auto_resume` về false. Khu khác khu bãi gốc thì vẫn bật ở khu hiện tại.

### 3.4. Giao diện

- **Nút Auto** (`ui/hud.gd`): nút tròn nhỏ bên trái hai nút bình; chạm để bật/tắt; giữ 0,5 giây mở cửa sổ cài đặt. Không bật được thì báo: "Không thể auto khi đã chết", "Không có quái ở thành", "Hãy ra khỏi khu an toàn".
- **Dòng trạng thái** dưới thẻ nhân vật: "Auto" nhấp nháy khi bật.
- **Vòng bãi** (`ui/path_view.gd` của 6a): vòng tròn mờ bán kính 10 ô quanh tâm bãi khi auto bật.
- **Cửa sổ cài đặt** (`ui/auto_window.gd`, mới): bốn dòng chiêu theo thanh chiêu (tên chiêu, công tắc; ô chưa học thì xám), hai thanh trượt "Uống bình HP khi dưới n%", "Uống bình MP khi dưới n%" (10–90, bước 10), lựa chọn "Nhặt trang bị: Không / Tất cả / Từ Xanh / Từ Vàng", công tắc "Nhặt đá". Đổi là lưu ngay.
- **Nút Tiếp tục auto**: nút chữ ở giữa cạnh dưới, trên thanh chiêu; ẩn khi auto bật, khi đang trên lộ trình Tiếp tục auto, hoặc khi người chơi tắt auto bằng tay.
- **Thông báo**: "Đã bật auto", "Đã tắt auto" (khi người chơi tắt), "Auto dừng: nhân vật đã chết", "Auto dừng: đã chuyển khu vực", "Đang về bãi auto".

### 3.5. Lưu cài đặt

`autoload/config.gd` thêm `load_auto(char_name) -> Dictionary` và `save_auto(char_name, d)` qua `ConfigFile` ở `user://auto.cfg`, mỗi nhân vật một mục. Lỗi đọc thì dùng mặc định.

### 3.6. Smoke

`tests/smoke/smoke.gd` sau các bước cũ: về Kumotori ngoài trại (`devchar -map kumotori_1 -x 14.5 -y 6.5`, vùng `wanderer` cấp 1–2; kiểm tra ô đi được và ngoài khu an toàn trong file map, nếu không thì lấy ô gần nhất thỏa), vào lại, `auto.start` rồi chạy `auto_play` với `attack_loop` (chỉ chém) và `predictor` như màn World cho tới khi có 2 `Death` của quái mình đánh trong 90 giây. In "SMOKE OK" kèm "auto kills 2".

## 4. Kiểm thử

**Server:** không có test mới (không đổi code).

**Client** (`tests/run.sh`, mục `# ____TEST AUTO____`):

- `auto_settings`: mặc định; `from_dict` kẹp 5 → 10, 95 → 90, 47 → 50; giá trị lạ về mặc định; `wants` với bốn mức lọc trang bị và hai mức đá.
- `auto_play.start`: chết, thành, khu an toàn bị từ chối với đúng lý do; hợp lệ ghi bãi.
- Chọn quái: gần mình nhất trong bãi; quái gần hơn nhưng ngoài bán kính bãi không được chọn; id bị tạm bỏ không được chọn; bằng khoảng cách thì id nhỏ hơn.
- Giữ mục tiêu: quái ra ngoài 12 ô từ tâm bãi bị bỏ.
- Không có quái: ngoài 10 ô trả `home`; trong 10 ô không trả gì.
- Không bao giờ chọn người chơi khi chưa bị đánh, kể cả có ứng viên người chơi gần hơn quái.
- Phản công: bị người chơi A đánh khi đang đánh quái thì mục tiêu thành A; A không còn là ứng viên thì bỏ; 10 giây không đánh thì bỏ, 9,9 giây thì giữ; A và B cùng đánh: giữ A, A bị bỏ thì sang B nếu B đánh trong 10 giây, nếu không thì về quái; A ra ngoài bán kính bãi + 5 bị bỏ.
- Chiêu: ô 5 sẵn sàng thì chọn 5; ô 5 tắt trong cài đặt thì xét 4; không ô nào sẵn sàng thì ô 1; chiêu cần mục tiêu không được chọn khi không có mục tiêu.
- Bình: HP 49% với ngưỡng 50 trả `"hp"`; HP 50% không; bình đang hồi (`potion_block` khác rỗng) không; HP và MP cùng thấp thì HP trước.
- Nhặt: chỉ khi không có mục tiêu; món ngoài bãi bỏ qua; món không `wants` bỏ qua; hành trang không chứa được thì bỏ qua và báo một lần.
- Smoke theo mục 3.6.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito) bật auto ở bãi Sagiri: tự đánh, dùng chiêu, uống bình khi máu thấp, nhặt đồ Xanh và đá; đẩy joystick thì auto tắt; `test2` (Hana) bật PK đánh Kaito đang auto: Kaito quay sang đánh trả, Hana chạy vào trại thì Kaito quay lại đánh quái; để Kaito chết, về làng, bấm Tiếp tục auto: nhân vật tự về bãi và auto lại.

## 5. Không đổi

Giới hạn tần suất của server (5 lệnh đánh, 5 lệnh chiêu, 20 lệnh di chuyển, 30 gói mỗi giây) không đổi; auto gửi trong giới hạn đó như người chơi giữ nút.

## 6. Tài liệu cập nhật

- `docs/game-design.md` mục 8 (auto phía client; bãi 10 ô; chỉ phản công người chơi; luật ngừng truy đuổi và chọn người đánh; không tự hồi sinh hay tiếp tế ở v1, có Tiếp tục auto; thứ tự chiêu 5 → 2), mục 10.5 (bỏ phần auto-play đã chốt, giữ phần thú cưỡi/trợ thủ).
- `CLAUDE.md` (server): auto chạy ở client và chỉ gửi các lệnh thường; server không có trạng thái auto.
- `CLAUDE.md` (client): quyết định auto ở `core/auto_play.gd`, cài đặt ở `core/auto_settings.gd`; `screens/world.gd` chỉ nối dây.
- README client: nút Auto, cài đặt, Tiếp tục auto.
