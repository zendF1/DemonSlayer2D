# Giai đoạn 7e — Khung sự kiện và Lệnh Diệt Quỷ

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ năm của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md) mục 1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 6.1 (sự kiện định kỳ hàng ngày), 6.2 (Quạ Truyền Tin – Lệnh Diệt Quỷ: 2–3 khung/ngày, diệt nhóm quỷ và quỷ đầu đàn ở map luyện cấp, thưởng EXP, Xu, đá cường hóa), 5.10 (tinh anh), 5.12 (rơi đồ) và 4.2 (giờ làm mới 00:00); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 7a](2026-11-20-giai-doan-7a-chat-design.md) (kênh Hệ thống), [spec 7d](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) (hạng quái, kỹ năng quái, hàm sinh quái đặc biệt) và [spec 6d](2026-11-20-giai-doan-6d-nhiem-vu-hang-ngay-design.md) (ngày theo UTC+7, `zone.Config.Now`). 7e chạy sau khi 7d đã triển khai (giao thức 19, migration 00011).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Server có **lịch sự kiện** theo giờ Việt Nam. Năm phút trước giờ, lúc bắt đầu và lúc kết thúc, kênh Hệ thống báo cho mọi người đang online. Trên HUD có dải thông báo sự kiện đang diễn ra kèm đếm ngược; chạm vào mở cửa sổ **Lịch sự kiện** liệt kê mọi sự kiện, trạng thái và giờ diễn ra kế tiếp.

Sự kiện đầu tiên là **Quạ Truyền Tin – Lệnh Diệt Quỷ**, ba khung mỗi ngày (12:00, 19:00, 21:30), mỗi khung 20 phút. Lúc bắt đầu, ở mọi khu của mọi map luyện cấp, một nhóm sáu **Quỷ bị truy nã** cùng một **Quỷ đầu đàn bị truy nã** (tinh anh, có chiêu Vồ) xuất hiện ở một vùng quái. Người chơi trong khu thấy dòng Hệ thống và vùng đó được đánh dấu trên bản đồ. Quỷ truy nã không hồi sinh, rơi đồ như quái cùng hạng cộng thêm phần thưởng sự kiện (Xu và đá), cho EXP theo sát thương như mọi quái. Hết giờ thì nhóm còn sống biến mất.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Dữ liệu sự kiện | `data/events.json` sinh bằng `python3 docs/balance/event_data.py --write-json`; không sửa tay; có `--check`; test Go so file đã commit với script | Như `quests.json`, `items.json`: một nguồn số liệu; client đồng bộ cùng file |
| Loại sự kiện | Mỗi sự kiện có `kind`; 7e có `wanted` (Lệnh Diệt Quỷ). 7f thêm `world_boss`, 7g `train`, 7h `siege`. Phần cấu hình riêng của mỗi loại nằm trong một khóa trùng tên loại; `kind` lạ thì server không khởi động | Khung chung cho lịch, báo tin, trạng thái; mỗi loại chỉ viết phần xử lý của mình |
| Lịch | Mỗi sự kiện một danh sách khung: các thứ trong tuần (1 = thứ Hai … 7 = Chủ nhật) theo UTC+7, giờ bắt đầu `HH:MM` UTC+7, số phút. Hai khung của cùng một sự kiện không được chồng nhau (kể cả qua nửa đêm) | Game-design 6.1 "theo giờ"; UTC+7 như 6d; Việt Nam không đổi giờ theo mùa |
| Giờ Lệnh Diệt Quỷ | 12:00, 19:00, 21:30 mỗi ngày, 20 phút | Game-design 6.2 "2–3 khung/ngày"; trưa, tối, khuya; tránh các khung của 7f (13:00, 18:00) và 7g (15:00, 21:00) |
| Ai chạy lịch | Dịch vụ mức world `internal/event` có goroutine riêng, xét lịch **mỗi giây** theo `Config.Now`; không ở trong zone | Một nơi quyết định giờ cho mọi zone; thử được bằng đồng hồ giả (tổng quan mục 3.3) |
| Zone biết sự kiện thế nào | Dịch vụ đăng **ảnh chụp** không đổi (`atomic.Pointer[event.Snapshot]`: các lượt đang chạy); zone đọc mỗi giây (cùng nhịp xét ngày của 6d) và tự làm phần của mình | Dịch vụ không gửi lệnh vào hộp thư zone, nên không có lệnh bị rơi khi hộp thư đầy (tổng quan mục 3.1) |
| Mã lượt | `run_id` = giây Unix lúc **bắt đầu theo lịch** của lượt đó; cặp (`event_id`, `run_id`) là duy nhất | Không cần bộ đếm lưu DB; khởi động lại server giữa lượt vẫn ra cùng mã |
| Khởi động lại giữa giờ | Server khởi động khi đang trong một khung thì lượt chạy ngay với thời gian còn lại; không báo "sắp bắt đầu", báo "đang diễn ra" một lần | Không mất một lượt vì bảo trì; Lệnh Diệt Quỷ không có gì cần khôi phục |
| Báo tin | Kênh Hệ thống (7a) cho mọi người online: trước 5 phút, lúc bắt đầu, lúc kết thúc | Game-design 6.2 "nhận thông báo"; không thêm loại tin riêng |
| Trạng thái cho client | `EventList` gồm mọi sự kiện với trạng thái (chờ, sắp bắt đầu, đang diễn ra), số mili giây tới lần bắt đầu kế tiếp, tới lúc kết thúc nếu đang chạy; có trong `EnterWorldResp`, đẩy cho mọi người online mỗi khi trạng thái đổi, và trả khi hỏi (`EventStateReq`) | Client tự đếm ngược như `daily_reset_ms` của 6d, không cần đồng bộ giờ máy |
| Nơi xuất hiện Lệnh Diệt Quỷ | **Mọi khu đang mở** của **mọi map `training`** lúc bắt đầu. Khu mở trong lúc lượt chạy cũng sinh nhóm nếu lượt còn ít nhất 5 phút | Game-design 6.2 "ở map luyện cấp"; mỗi khu một nhóm nên người chơi không phải dồn về một khu; khu mới mở vì đông người cũng có quỷ |
| Vùng | Mỗi khu chọn ngẫu nhiên đều một vùng quái của map bằng `z.rng`; các khu chọn độc lập | Không cần điều phối giữa các zone; khu khác có thể ở vùng khác |
| Thành phần nhóm | 6 Quỷ bị truy nã (`wanted_demon`, hạng thường, cấp ngẫu nhiên trong khoảng cấp của vùng) + 1 Quỷ đầu đàn bị truy nã (`wanted_leader`, tinh anh, cấp = cấp tối đa của vùng + 1) | Game-design 6.2 "nhóm quỷ và quỷ đầu đàn"; đầu đàn mạnh hơn một bậc như Quỷ lang thang đầu đàn cấp 6 ở map 1–5 |
| Hành vi | Cả hai dùng AI có sẵn: Quỷ bị truy nã như quái thường (bắn người đi qua, bị đánh thì đuổi, giữ trong vùng); đầu đàn như tinh anh của 7d (chủ động đuổi, chiêu Vồ 150% công vật lý, hồi 6 giây) | Không thêm AI mới; tinh anh đã có ở 7d |
| Vị trí sinh | Đầu đàn ở ổ quỷ của vùng; sáu con còn lại rải đều trên ô của vùng như luật rải quái thường | Nhóm nằm gọn trong một vùng, dễ tìm |
| Hồi sinh | Không; chết là hết. Hết giờ thì nhóm còn sống rời map (`EntityLeave`), không rơi gì | Đúng nghĩa "lệnh truy nã" một lần; game-design 5.3 "không dùng quy tắc này thay luật Boss của sự kiện" |
| Phần thưởng | EXP theo sát thương (game-design 5.8, cửa sổ ±10); rơi đồ theo bảng của hạng (5.12) cho người kết liễu; **cộng thêm** phần thưởng sự kiện cho người kết liễu: Quỷ bị truy nã ×2 Xu và 1 đá cường hóa chắc chắn; đầu đàn ×2 Xu, 5 đá cường hóa và 1 đá tẩy luyện chắc chắn | Game-design 6.2 "EXP, Xu, đá cường hóa"; đồ thuộc người kết liễu (5.8); số tạm, cân chỉnh sau chơi thử |
| Đồ thưởng thêm | Xu cộng thẳng vào ví như Xu quái; đá rơi xuống đất cạnh xác, chỉ người kết liễu thấy, 60 giây (luật 3a) | Cùng đường rơi đồ của 3a; không cần hòm thưởng (hòm thưởng ở 7f) |
| Cấp tham gia | Không giới hạn; quái truy nã theo cấp vùng nên mỗi map hợp một khoảng cấp | Tổng quan mục 3.6: chơi được ngay ở năm map |
| Báo trong khu | Lúc nhóm xuất hiện: dòng Hệ thống cho người trong khu "Quỷ bị truy nã xuất hiện ở <vùng> (Lv a–b)". Nhóm bị hạ hết: "Khu n đã sạch quỷ truy nã". Gửi kèm `WantedGroupState` để bản đồ đánh dấu vùng | Biết đi đâu ngay; tin "sạch quỷ" báo người khác khỏi tìm |
| Tên vùng trong tin | Tên quái thường của vùng: "bãi Quỷ lang thang"; client dùng `region_id` để vẽ khung vùng | Vùng không có tên riêng trong file map; tên loại quái đủ để nhận ra |
| Nhiệm vụ | Hạ quỷ truy nã tính cho nhiệm vụ như quái thường: `kill_any`, `collect_any` của 6d (theo cửa sổ cấp); `kill`/`collect` theo loại quái không tính (loại khác) | Một luật tiến độ; không bơm nhiệm vụ chính tuyến |
| Tổ đội | EXP tổ đội 5% như 5b | Không có luật riêng |
| Lệnh dev | Biến môi trường `EVENT_DEV_START` (danh sách id): mỗi sự kiện trong danh sách chạy một lượt dev bắt đầu 60 giây sau khi server khởi động, đúng thời lượng của sự kiện | Nghiệm thu và e2e không phải chờ tới giờ thật |

### 1.3. Bổ sung cho game-design

Game-design 6.2 ghi lịch "là định hướng, chưa phải cấu hình cuối" và mục 10.2 để mở lịch, thời lượng, điều kiện tham gia. Spec này chốt cho Lệnh Diệt Quỷ: 12:00, 19:00, 21:30 UTC+7, 20 phút, mọi khu mọi map luyện cấp, nhóm 6 + 1 không hồi sinh, phần thưởng thêm như mục 1.2; ghi vào game-design mục 6.2 (mục 9).

### 1.4. Không làm ở 7e

Hòm thưởng và thưởng cuối lượt (7f); các sự kiện khác của game-design 6.2 (7f, 7g, 7h hoặc để sau theo tổng quan mục 4); đổi lịch khi server đang chạy; lịch theo ngày tháng cụ thể (lễ hội); đăng ký tham gia; điểm sự kiện; xếp hạng Lệnh Diệt Quỷ; nhiệm vụ sự kiện; thông báo đẩy ra ngoài game.

## 2. Dữ liệu

### 2.1. `data/events.json`

```json
{
  "announce_min": 5,
  "events": [
    {
      "id": "lenh_diet_quy",
      "name": "Quạ Truyền Tin – Lệnh Diệt Quỷ",
      "kind": "wanted",
      "min_level": 1,
      "summary": "Diệt nhóm quỷ truy nã và quỷ đầu đàn ở mọi map luyện cấp",
      "schedule": [
        {"days": [1, 2, 3, 4, 5, 6, 7], "start": "12:00", "minutes": 20},
        {"days": [1, 2, 3, 4, 5, 6, 7], "start": "19:00", "minutes": 20},
        {"days": [1, 2, 3, 4, 5, 6, 7], "start": "21:30", "minutes": 20}
      ],
      "wanted": {
        "late_min_left": 5,
        "members": [
          {"monster": "wanted_demon", "count": 6, "level": "region"},
          {"monster": "wanted_leader", "count": 1, "level": "region_max", "level_plus": 1, "at_nest": true}
        ],
        "bonus": {
          "wanted_demon": {"xu_mul": 2, "stacks": [{"code": "stone_enhance", "count": 1}]},
          "wanted_leader": {"xu_mul": 2, "stacks": [{"code": "stone_enhance", "count": 5}, {"code": "stone_refine", "count": 1}]}
        }
      }
    }
  ]
}
```

- `announce_min`: số phút báo trước (1–60).
- `events[]`: thứ tự trong file là thứ tự hiện ở Lịch sự kiện. `id` không trùng; `name`, `summary` khác rỗng; `min_level` 1–90 (chỉ để hiện; mỗi loại tự áp nếu cần); `schedule` không rỗng.
- `schedule[]`: `days` không rỗng, mỗi số 1–7, không trùng; `start` dạng `HH:MM` 00:00–23:59; `minutes` 1–720. Khung của cùng sự kiện không chồng nhau trên cả tuần (tính theo phút trong tuần, quay vòng qua Chủ nhật → thứ Hai).
- `wanted.members[]`: `monster` là loại có thật trong `monsters.json`; `count` 1–20; `level` là `region` (ngẫu nhiên đều trong khoảng cấp của vùng) hoặc `region_max` (cấp tối đa của vùng cộng `level_plus` 0–10, kẹp ở 90); `at_nest` (mặc định sai): sinh ở ổ quỷ. Tổng `count` không lớn hơn số ô nhỏ nhất của mọi vùng quái trên map luyện cấp.
- `wanted.bonus`: khóa là loại quái có trong `members`; `xu_mul` 1–10 (Xu của lượt rơi nhân lên); `stacks[]` là vật phẩm chồng có thật, `count` 1–999, rơi chắc chắn.
- `wanted.late_min_left`: 0–`minutes` nhỏ nhất của lịch.

### 2.2. Quái truy nã trong `monsters.json`

`monster_stats.py` thêm hai loại (không sửa tay JSON):

| id | Tên | Hạng | Hành vi và kỹ năng |
| --- | --- | --- | --- |
| `wanted_demon` | Quỷ bị truy nã | `normal` | Như `wanderer` (mọi thông số hành vi giống nhau) |
| `wanted_leader` | Quỷ đầu đàn bị truy nã | `elite` | Như `wanderer_leader` của 7d: thông số tinh anh, kỹ năng Vồ (`pounce`, mục tiêu đơn, 150% công vật lý, hồi 6 giây) |

Chỉ số theo bảng `levels` và hệ số hạng của 7d. Loại quái thêm trường không bắt buộc `event_only` (mặc định `false`); hai loại này có `event_only: true`: không được đặt trong vùng quái hay điểm thủ lĩnh của file map (`gamedata.Validate` từ chối), chỉ sinh qua sự kiện. Boss sự kiện của 7f–7h dùng cùng trường.

### 2.3. Kiểm tra khi nạp

`gamedata.Load` đọc `events.json` (`DisallowUnknownFields`) và kiểm tra mục 2.1; `(*Data).Validate(maps)` kiểm tra thêm: mọi map `training` có ít nhất một vùng quái; tổng `count` của `wanted.members` vừa mọi vùng. Lỗi ghi rõ id sự kiện. Script kiểm tra cùng các luật trước khi ghi.

## 3. Lưu trữ

Không có migration. Lượt sự kiện không lưu: `run_id` tính từ lịch (mục 1.2), nhóm quỷ truy nã sống trong zone và mất khi zone dừng như quái thường.

## 4. Server

### 4.1. Lịch

Gói `internal/event` (thuần phần lịch, không I/O):

- `Window{EventID string; Start, End time.Time}`; `RunID(w) int64 = w.Start.Unix()`.
- `Active(ev, t)`: khung của `ev` chứa t (Start ≤ t < End), nếu có.
- `Next(ev, t)`: khung kế tiếp có Start > t.
- Mọi phép tính đổi t sang UTC+7 bằng độ lệch cố định +7 giờ (như `quest.Day`); thứ 1–7 là thứ của ngày UTC+7.

### 4.2. Dịch vụ

`event.Service` mức world (tổng quan mục 3.1):

- **Đăng ký**: `World.Enter` đăng ký phiên (id thực thể, hộp gửi); `Membership.Leave` hủy đăng ký.
- **Goroutine lịch** chạy mỗi giây với t = `Config.Now()`. Với mỗi sự kiện, trạng thái là:
  - `RUNNING` nếu có khung chứa t;
  - `SOON` nếu không chạy và lần bắt đầu kế tiếp cách t không quá `announce_min` phút;
  - `IDLE` còn lại.
- Khi trạng thái của một sự kiện đổi so với lần xét trước:
  - `IDLE → SOON`: Hệ thống "<tên> sẽ bắt đầu sau n phút" (n = số phút còn lại làm tròn lên).
  - `* → RUNNING`: Hệ thống "<tên> đã bắt đầu" (lần xét đầu tiên sau khởi động mà đã đang chạy: "<tên> đang diễn ra, còn n phút").
  - `RUNNING → *`: Hệ thống "<tên> đã kết thúc".
  - Đăng ảnh chụp mới (mục 4.3) nếu tập lượt đang chạy đổi; đẩy `EventList` cho mọi phiên đã đăng ký.
- Lượt dev (`EVENT_DEV_START`, mục 7): mỗi id trong danh sách thêm một khung một lần Start = lúc khởi động + 60 giây, End = Start + `minutes` của khung đầu tiên của sự kiện; lượt dev được xét như khung thường (báo trước ngay vì Start − t ≤ `announce_min`). `Config.DevDelay`, `Config.DevMinutes` cho test.
- Gửi tin Hệ thống qua `chat.Service.System(text)` (7a). Dịch vụ không gọi vào world hay zone.

### 4.3. Ảnh chụp cho zone

```go
type Run struct {
    EventID string
    Kind    string
    RunID   int64
    Start, End time.Time
}
type Snapshot struct { Runs []Run } // theo thứ tự sự kiện trong file
```

- `Service.Snapshot() *Snapshot` (đọc nguyên tử, không khóa). Ảnh chụp không bao giờ bị sửa sau khi đăng.
- `zone.Config.Events func() *event.Snapshot` (nil: không có sự kiện, dùng trong test cũ).
- Mỗi giây (cùng chỗ zone xét ngày của 6d) zone đọc ảnh chụp và so với tập lượt nó đã biết:
  - lượt mới mà zone có phần xử lý cho `Kind` → gọi `start`;
  - lượt zone đang giữ mà không còn trong ảnh chụp, hoặc `End` ≤ `Config.Now()` → gọi `end`.
- Mỗi loại sự kiện có phần xử lý trong zone (`zone/event_wanted.go` ở 7e); zone giữ trạng thái của lượt trong bộ nhớ zone, chỉ goroutine zone đụng tới.

### 4.4. Lệnh Diệt Quỷ trong zone

Chỉ chạy ở zone có `Area.Type == training`.

**Bắt đầu** (`start`, run r):

1. Nếu r.End − now < `late_min_left` phút và zone mở sau r.Start: bỏ qua lượt này (ghi nhận để không xét lại).
2. Chọn vùng: chỉ số đều trong [0, số vùng) bằng `z.rng`.
3. Với từng mục `members` theo thứ tự, với từng con: cấp theo `level`; vị trí là ổ quỷ nếu `at_nest`, nếu không thì một ô ngẫu nhiên của vùng (luật rải của spec quái thường mục 4.1); sinh bằng hàm sinh quái đặc biệt của 7d: loại, cấp, vị trí, vùng nhà = vùng đã chọn, **không hồi sinh**, thẻ lượt (`event_id`, `run_id`), móc khi chết.
4. Ghi `wanted{run, region, alive: tổng số con, total}`; gửi cho mọi người chơi trong zone tin Hệ thống "Quỷ bị truy nã xuất hiện ở bãi <tên loại quái thường của vùng> (Lv a–b)" (a, b là cấp nhỏ nhất và lớn nhất của nhóm) và `WantedGroupState{event_id, run_id, region_id, alive, total}`.

**Khi một con của nhóm chết** (móc của 7d, gọi trong `killMonster` sau `loot`):

1. Nếu người kết liễu còn trong zone: phần thưởng thêm theo `bonus[loại]`: Xu thêm = Xu vừa rơi × (`xu_mul` − 1), cộng vào ví như Xu quái; mỗi mục `stacks` là một món đất mới của người kết liễu cạnh xác (luật vị trí và 60 giây của 3a, đánh số tiếp sau các món vừa rơi). Người kết liễu đã rời zone: không có gì (như 3a).
2. `alive −= 1`; gửi `WantedGroupState` mới cho người trong zone. `alive` về 0: Hệ thống "Khu n đã sạch quỷ truy nã".

**Kết thúc** (`end`): mọi con còn sống của nhóm bị xóa khỏi zone như khi hết thời gian xác (`EntityLeave` cho người thấy), không rơi gì, không EXP; `wanted` của zone về rỗng; gửi `WantedGroupState{alive: 0, total: 0}`.

**Người vào zone** (vào game, qua cổng, đổi khu) khi zone đang có nhóm: `EnterWorldResp` có `events` như mọi lúc; ngay sau `EnterWorldResp` zone gửi `WantedGroupState` hiện tại.

**Đóng zone** (trống 5 phút): nhóm mất theo zone; không ảnh hưởng zone khác.

### 4.5. Rơi đồ, EXP, nhiệm vụ

Không đổi đường code: quỷ truy nã là quái của zone, đi qua `applyToMonster`, EXP theo sát thương, `loot` theo hạng, tiến độ nhiệm vụ 6b/6d như mọi quái. Phần thưởng thêm chạy sau `loot` (mục 4.4).

### 4.6. Gateway

`EventStateReq{}` cần đã vào thế giới (`NOT_IN_WORLD`), chỉ chịu giới hạn gói chung, trả thẳng từ dịch vụ (không qua zone): `EventStateResp{events}`.

## 5. Giao thức

`ProtocolVersion` lên **20**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 190 | `EventStateReq` | rỗng |
| 191 | `EventStateResp` | `EventList events = 1` |
| 192 | `EventList` | đẩy với `seq = 0` mỗi khi trạng thái sự kiện đổi |
| 193 | `WantedGroupState` | đẩy với `seq = 0` (mục 4.4) |

```proto
enum EventStatus {
  EVENT_STATUS_UNSPECIFIED = 0;
  EVENT_STATUS_IDLE = 1;    // not running, not about to start
  EVENT_STATUS_SOON = 2;    // starts within the announce lead time
  EVENT_STATUS_RUNNING = 3; // a run is going on
}

// One event as the calendar shows it. Times are milliseconds from when the
// message was sent, so the client counts down with its own clock.
message EventInfo {
  string event_id = 1;
  EventStatus status = 2;
  int64 run_id = 3;        // set while RUNNING
  int64 starts_in_ms = 4;  // to the next start; 0 while RUNNING
  int64 ends_in_ms = 5;    // to the end of the current run; 0 unless RUNNING
}

// Every event of events.json in file order.
message EventList {
  repeated EventInfo events = 1;
}

// The wanted group of the receiver's zone. alive 0 and total 0: no group.
message WantedGroupState {
  string event_id = 1;
  int64 run_id = 2;
  string region_id = 3;
  int32 alive = 4;
  int32 total = 5;
}
```

- `EnterWorldResp.events = 17` (`EventList`).
- Tên, mô tả và lịch của sự kiện không đi qua mạng: client đọc `data/events.json` đã đồng bộ.
- Không có mã lỗi mới (dải 91–93 để trống).
- Fixture mới: `event_list.hex` (một sự kiện `RUNNING` còn 600.000 ms), `wanted_group_state.hex` (`alive` 4 trên 7), `enter_world_resp_events.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/event_book.gd`** (mới): đọc `data/events.json`; `apply_list(EventList, now)` lưu cho mỗi sự kiện trạng thái, `start_at` = now + `starts_in_ms`, `end_at` = now + `ends_in_ms`; `running(now) -> Array` (các sự kiện đang chạy, theo thứ tự file); `banner_text(now) -> String` ("Lệnh Diệt Quỷ · còn 14:32"; nhiều sự kiện thì sự kiện kết thúc sớm nhất, kèm "+n"); `next_text(id, now)` ("Bắt đầu sau 2:05:10", "Đang diễn ra", "Sắp bắt đầu sau 4:12"); `schedule_text(id)` ("Hằng ngày 12:00, 19:00, 21:30 · 20 phút"; tên thứ khi không phải cả tuần); `wanted` của zone hiện tại `{region_id, alive, total}` từ `apply_wanted`.
- **`core/wire.gd`**: payload `event_state_resp`, `event_list`, `wanted_group_state`. `tools/sync_server.sh` chép thêm `data/events.json` (và `--check` so sánh).
- Mô hình ở `autoload/router.gd` (`Router.events`), còn nguyên khi màn World nạp lại; xóa khi về màn chọn nhân vật.

### 6.2. Giao diện

- **Dải sự kiện** (`ui/event_banner.gd`, mới): giữa cạnh trên, dưới tên map; hiện `banner_text` khi có sự kiện đang chạy; chạm thì mở Lịch sự kiện. Ẩn khi không có.
- **Lịch sự kiện** (`ui/event_window.gd`, mới): mở từ dải sự kiện hoặc nút "Sự kiện" trong menu HUD; mỗi sự kiện một dòng: tên, `summary`, `schedule_text`, `next_text`; sự kiện đang chạy tô sáng. Mở cửa sổ thì gửi `EventStateReq` để làm mới.
- **Bản đồ** (`ui/map_window.gd` của 4a/6a): khi `wanted.alive > 0` ở map hiện tại, vẽ khung đỏ các hình chữ nhật của vùng `region_id` (lấy từ file map) và chữ "Quỷ truy nã n/m"; chạm khung thì đi tới vùng (lộ trình `region` của 6b).
- **Thực thể**: quỷ truy nã có nhãn "Truy nã" trên tên (theo id loại `wanted_*`); đầu đàn dùng nhãn tinh anh của 7d.
- Tin Hệ thống hiện trong khung chat (7a) như mọi tin Hệ thống.

### 6.3. Smoke

`tools/smoke.sh` chạy server smoke với `EVENT_DEV_START=lenh_diet_quy`. `tests/smoke/smoke.gd` sau các bước cũ: `EnterWorldResp.events` có `lenh_diet_quy` với `SOON` hoặc `RUNNING`; ở Kumotori chờ tối đa 90 giây tới `WantedGroupState` có `total` 7; chờ `EntityEnter` của 7 quái có tên "Quỷ bị truy nã" hoặc "Quỷ đầu đàn bị truy nã". In "SMOKE OK" kèm "wanted 7 in <region_id>".

## 7. Lệnh dev và cấu hình

- `EVENT_DEV_START` (mặc định rỗng): danh sách id sự kiện cách nhau bởi dấu phẩy; id lạ thì server dừng và báo lỗi. Mỗi sự kiện chạy một lượt dev như mục 4.2. Ghi vào bảng cấu hình README.
- Không có lệnh `cmd/` mới.

## 8. Kiểm thử

**Script:** `event_data.py --check` sạch; script in giờ các khung trong tuần và tổng số lượt mỗi tuần (21).

**Server** (`go test -race`):

- `gamedata`: dữ liệu thật nạp được (một sự kiện `wanted`); từ chối `kind` lạ, khóa cấu hình thiếu hoặc thừa, `days` có 0 hay 8 hay trùng, `start` "24:00" hay "7:5", `minutes` 0, hai khung chồng nhau (21:30 + 180 phút với 23:00 cùng ngày) và hai khung chồng qua đêm Chủ nhật → thứ Hai, quái lạ, `level` lạ, `bonus` cho loại không có trong `members`, vật phẩm thưởng lạ, loại `event_only` đặt trong vùng quái của map, tổng `count` lớn hơn số ô của vùng nhỏ nhất.
- `event` lịch: 11:59:59 UTC+7 không chạy, 12:00:00 chạy, 12:19:59 chạy, 12:20:00 không; `Next` lúc 21:50 thứ Hai là 12:00 thứ Ba; khung chỉ thứ Bảy: `Next` lúc thứ Bảy 21:00 là thứ Bảy tuần sau; `RunID` bằng giây Unix của Start.
- `event` dịch vụ (đồng hồ giả, chat giả, hộp gửi giả): tin "sẽ bắt đầu sau 5 phút" ở 11:55:00, không lặp ở 11:55:01; "đã bắt đầu" ở 12:00; "đã kết thúc" ở 12:20; ảnh chụp có lượt từ 12:00 tới 12:20 và rỗng sau đó; mỗi lần đổi trạng thái đẩy `EventList` cho mọi phiên; khởi động lúc 12:10 thì có lượt ngay với `End` 12:20, tin "đang diễn ra, còn 10 phút", không có tin "sắp bắt đầu"; lượt dev với `DevDelay` 1 giây; hủy đăng ký thì không nhận nữa.
- `zone` Lệnh Diệt Quỷ (ảnh chụp giả, đồng hồ giả): lượt mới trong một giây sinh đúng 7 con trong một vùng, đầu đàn ở ổ quỷ với cấp tối đa + 1, sáu con trong khoảng cấp vùng; người trong zone nhận tin Hệ thống và `WantedGroupState{alive 7, total 7}`; zone `town` không sinh; hạ một con: người kết liễu nhận Xu gấp đôi Xu rơi và một đá cường hóa dưới đất ngoài lượt rơi thường, `alive` 6; người kết liễu đã rời zone: không thưởng thêm; hạ đầu đàn có 5 đá cường hóa và 1 đá tẩy luyện thêm; hạ hết: tin "Khu n đã sạch quỷ truy nã"; quái không hồi sinh; lượt kết thúc: con còn sống rời zone, không rơi gì, `WantedGroupState{0, 0}`; zone mở lúc lượt còn 4 phút không sinh, còn 6 phút thì sinh; người vào zone giữa lượt nhận `WantedGroupState` ngay sau `EnterWorldResp`; `kill_any` của 6d tăng khi hạ quỷ truy nã hợp cấp.
- `world`: `Enter` đăng ký với dịch vụ sự kiện và `EnterWorldResp.events` có danh sách; `Leave` hủy đăng ký.
- `gateway`: `ProtocolVersion` 20; fixture mới; `EventStateReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestWantedEventOverWebSocket`: server dựng với lượt dev `lenh_diet_quy` bắt đầu sau 1 giây, thời lượng 1 phút; nhân vật cấp 10 (`DevSetLevel`; quái Kumotori cấp 1–6 nằm trong cửa sổ ±10) đặt ngoài trại Kumotori; nhận `EventList` `RUNNING` và `WantedGroupState` 7; đọc `region_id`, đi tới và hạ một Quỷ bị truy nã; thấy đá cường hóa thêm dưới đất và nhặt được; chờ hết lượt: nhận `EventList` không còn `RUNNING`, `WantedGroupState{0, 0}` và `EntityLeave` của các con còn lại.

**Client** (`tests/run.sh`, mục `# ____TEST EVENTS____`): `event_book` đọc file thật; `apply_list` và đếm ngược `banner_text`, `next_text` theo giờ giả; `schedule_text` cho khung cả tuần và khung chỉ thứ Bảy; `apply_wanted`; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199` khởi động với `EVENT_DEV_START=lenh_diet_quy`. `test1` ở Kumotori, `test2` ở Sagiri 1: cả hai thấy tin "sắp bắt đầu", rồi "đã bắt đầu" và dải sự kiện đếm ngược; mỗi người thấy tin quỷ xuất hiện ở bãi nào trong khu mình và khung đỏ trên bản đồ; chạm khung để đi tới; hạ nhóm, nhặt đá thưởng thêm, thấy "Khu n đã sạch quỷ truy nã"; mở Lịch sự kiện thấy giờ 12:00, 19:00, 21:30.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 6.2 (Lệnh Diệt Quỷ v1: giờ, thời lượng, mọi khu mọi map luyện cấp, nhóm 6 + 1, không hồi sinh, phần thưởng thêm), mục 10.2 (bỏ phần lịch/thời lượng của Lệnh Diệt Quỷ).
- `CLAUDE.md`: lịch sự kiện ở `internal/event`, giờ UTC+7; zone chỉ đọc ảnh chụp sự kiện mỗi giây, dịch vụ sự kiện không gửi lệnh vào zone; `data/events.json` sinh bằng `docs/balance/event_data.py`.
- README hai repo: Lịch sự kiện, Lệnh Diệt Quỷ, biến `EVENT_DEV_START`.
- Tổng quan Giai đoạn 7: đánh dấu 7e đã triển khai khi xong.
