# Giai đoạn 7g — Chuyến Tàu Vô Tận (phó bản tổ đội)

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ bảy của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md)). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 5.1 (phó bản), 5.5 (Chuyến Tàu Vô Tận cấp 66–75, Enmu cấp 75), 5.7 (luật hồi sinh riêng của hoạt động), 5.8 (EXP và đồ rơi), 5.10 (Enmu: Sóng mộng), 5.13 (bản sự kiện) và 6.2 (1–2 khung/ngày, tổ đội tối đa 5 người, 15 phút, không PK; thưởng EXP, trang bị, đá cường hóa); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) (chuyển zone, bản trao tay), [spec 5b](2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md) (tổ đội), [spec 7d](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) (hạng quái, kỹ năng quái, sinh quái đặc biệt), [spec 7e](2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md) (lịch, loại sự kiện, ảnh chụp sự kiện, `EVENT_DEV_START`) và [spec 7f](2026-11-20-giai-doan-7f-thao-phat-thuong-huyen-design.md) (hòm thưởng `reward.Service.Grant`, trường map `max_zones`, `capacity`, `min_level`, `EVENT_DEV_MINUTES`). 7g là phần đầu tiên cần NPC sự kiện nên định nghĩa vai NPC `event` (mục 2.1); 7h dùng lại. 7g chạy sau khi 7f đã triển khai (giao thức 21, migration 00012).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Hai khung mỗi ngày, ở Tổng hành dinh có **Trưởng tàu**. Trưởng nhóm (hoặc một người chơi đi một mình) từ cấp 66 chạm Trưởng tàu và bấm **Mở chuyến tàu**: server mở một bản riêng của map Chuyến Tàu Vô Tận chỉ cho tổ đội đó và đưa người mở vào toa 1. Các đồng đội trong 2 phút chạm Trưởng tàu, bấm **Lên tàu**. Trên tàu có bốn toa: bước vào toa nào thì các đợt quái của toa đó lần lượt xuất hiện; dọn sạch toa trước mới mở toa sau; toa cuối là Enmu với lượng máu gấp bốn bản thường, chiêu Sóng mộng làm ngủ 1 giây. Hạ Enmu trong 15 phút thì mọi người trên tàu nhận thưởng hoàn thành; hết giờ thì chuyến thất bại và mọi người về Tổng hành dinh.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Khung giờ | Hai khung mỗi ngày **15:00–16:00** và **21:00–22:00** UTC+7, sự kiện `chuyen_tau` loại mới `train` trong `data/events.json` (khung của 7e, cấu hình riêng ở khóa `train`); 7e báo trước 5 phút, lúc mở và lúc đóng trên kênh Hệ thống | Game-design 6.2 "1–2 khung/ngày"; một giờ đủ cho nhiều tổ đội mở lượt; tránh 20:00–21:00 thứ Bảy của Vô Hạn Thành (7h) |
| NPC sự kiện | Vai NPC mới `event` có trường `event` (id sự kiện); Trưởng tàu là `event: "chuyen_tau"`. Cửa sổ NPC chọn nội dung theo id sự kiện | 7e, 7f không cần NPC (sự kiện ở map luyện cấp và qua cổng); 7g, 7h cần; một vai cho mọi NPC sự kiện |
| Khung dùng cho gì | Khung chỉ quyết định lúc **mở** lượt; lượt mở lúc 15:59 vẫn chạy đủ 15 phút | Không cắt ngang tổ đội đang đánh |
| Ai mở | Trưởng nhóm của tổ đội, hoặc người **không ở trong tổ đội** (đi một mình); thành viên không phải trưởng nhóm bị `NOT_PARTY_LEADER` | Game-design "tối đa 5 người", không có tối thiểu; một luật cho người đi một mình là tổ một người |
| Ai vào | Người mở, và thành viên **hiện tại** của đúng tổ đội đó (ô id tổ đội bằng id tổ đội lúc mở), trong **2 phút** sau khi mở; người đi một mình thì không ai khác vào được | Không cưỡng ép chuyển zone người khác: `World.Transfer` chạy trên goroutine kết nối của chính người đó (4a), nên mỗi người tự bấm Lên tàu |
| Đứng ở đâu khi mở và lên tàu | Trong tầm NPC Trưởng tàu (3 ô + 0,5, luật 4b) | Như mọi dịch vụ NPC; tổ đội tập hợp ở thành trước khi đi |
| Cấp | Từ cấp **66** (cấp quái thấp nhất của tàu): trường `min_level: 66` của map tàu (7f), kiểm tra ở bước cuối của mọi lần chuyển zone vào map đó (`LEVEL_TOO_LOW`) | Game-design 5.5: tàu cấp 66–75; cửa sổ EXP ±10 vẫn áp như thường |
| Số lượt | Mỗi nhân vật **một lượt mỗi khung**: lần đầu vào một lượt là ghi nhận cho khung đó; vào lại đúng lượt của mình (sau khi rời hay mất kết nối) thì được; lượt khác cùng khung thì `TRAIN_ALREADY_RAN`. Ghi nhận chỉ trong bộ nhớ | Game-design "1–2 khung/ngày" là nhịp tham gia; khởi động lại server giữa khung chỉ làm người chơi được thêm một lượt |
| Một tổ, một lượt | Một tổ đội (hay một người đi một mình) chỉ có một lượt đang chạy; mở lần hai bị `TRAIN_RUN_EXISTS` | Không mở nhiều bản để chia người |
| Bản riêng | Zone riêng (instance, tổng quan mục 3.4) của map mới `tau_vo_tan` loại `instance`: không có trong danh sách khu, `reserve` không bao giờ chọn, sức chứa 5, chỉ nhận người của lượt | Game-design 5.13: "instance riêng, không dùng bộ đếm hồi sinh thủ lĩnh thường" |
| Toa | Bốn toa nối liền trong một map, mỗi toa là một **vùng có tên** (`areas`, mục 2.1) với một khu an toàn nhỏ ở đầu toa; không có cửa đóng mở hay va chạm động | Không phải sửa va chạm của server và dự đoán của client; luật mở toa đủ để giữ thứ tự |
| Mở toa | Toa 1 bắt đầu khi có thành viên còn sống đứng trong toa 1 ngoài khu an toàn đầu toa. Toa k (k ≥ 2) bắt đầu khi toa k − 1 đã sạch **và** có thành viên còn sống đứng trong toa k ngoài khu an toàn. Đi trước sang toa chưa mở chỉ thấy toa trống | Game-design "qua các đợt quái rồi đánh Enmu"; chạy vượt không bỏ qua được đợt nào |
| Đợt quái | Toa 1–3 mỗi toa 3 đợt; đợt sau xuất hiện **3 giây** sau khi đợt trước chết hết; quái đợt lao vào người gần nhất trong toa ngay khi xuất hiện, chỉ đi trong toa, không hồi sinh | Nhịp đánh liên tục; quái phó bản không lang thang hay bắn người đi ngang |
| Quái của tàu | Ba loại quái thường mới cấp 66–75 (`train_tentacle` Xúc tu, `train_flesh` Mô thịt, `train_nightmare` Quái ác mộng) sinh bằng `monster_stats.py`, dùng chung với map Chuyến Tàu thường sau này | Game-design 5.5; một nguồn số liệu |
| Enmu | Loại `enmu` hạng thủ lĩnh cấp 75 (dùng cho map thường sau này) với kỹ năng Sóng mộng; bản sự kiện sinh bằng hàm sinh quái đặc biệt của 7d với **HP × 4**, công và thủ giữ nguyên, không hồi sinh | Game-design 5.13 |
| Sóng mộng | Quạt 90°, vươn 5 ô, cảnh báo 0,8 giây, trúng tối đa 5 người, 150% công phép, gây **ngủ 1 giây**, hồi 10 giây; chọn người trúng lúc đòn tới (luật cảnh báo vùng của 7d) | Game-design 5.10 cho sát thương, hiệu ứng, hồi chiêu; 5.10 "thủ lĩnh tối đa 5 người, cảnh báo 0,8 giây" |
| Ngủ | Hiệu ứng trạng thái đầu tiên của game (`StatusKind.SLEEP`): trong thời gian ngủ nhân vật không di chuyển, không chém, không dùng chiêu, không dùng vật phẩm, không nhặt; lệnh bị từ chối `ASLEEP`. Bị đánh không làm tỉnh. Ngủ mới khi đang ngủ thì kéo dài tới mốc muộn hơn | Hiệu ứng ngắn 1 giây, luật đơn giản nhất mà vẫn đúng nghĩa; khung trạng thái dùng lại được cho chậm, giữ chân sau v1 |
| Báo ngủ | `Hit.status` (loại hiệu ứng đòn này gây) và `WorldTick.statuses` (bắt đầu, kết thúc, thời lượng còn lại) | Client vẽ "Zz" đúng lúc đòn trúng và khóa điều khiển của chính mình theo thời lượng server |
| Thời hạn | **15 phút** từ lúc mở | Game-design 5.13 |
| PK | Không (map `instance`, luật 5a) | Game-design 5.13 "bản tổ đội không PK" |
| Chết trên tàu | Hồi sinh **miễn phí** ở điểm đầu toa đang mở xa nhất (`car_k`); hồi sinh tại chỗ bằng 1 Lượng vẫn được; tự hồi sinh sau 60 giây cũng về đầu toa | Game-design 5.7 cho phép hoạt động có luật riêng; về làng sẽ làm người chơi văng khỏi phó bản |
| Phạt EXP | Không (map không phải `training`) | Game-design 5.13 |
| EXP, đồ rơi | Như map thường: EXP theo sát thương, đồng đội cùng zone thêm 5% (5b); đồ rơi và Xu của quái thuộc người kết liễu theo bảng rơi của hạng quái; Enmu rơi theo bảng thủ lĩnh | Game-design 5.8, 6.2 |
| Thưởng hoàn thành | Mọi thành viên **đang ở trong bản** lúc Enmu chết (sống hay chết) nhận: **30.000 Xu**, **20 đá cường hóa**, **2 đá tẩy luyện** và **một trang bị bậc 50** ô ngẫu nhiên (vũ khí đúng class), phẩm chất Vàng 75% / Cam 25%, dòng sinh như đồ rơi (3a) | Game-design 6.2 "EXP, trang bị, đá cường hóa"; EXP đã đến từ quái; 30.000 Xu ≈ Xu của khoảng 100 quái cấp 75; số tạm, cân chỉnh sau chơi thử |
| Hành trang đầy | Cả phần thưởng chứa được thì cộng thẳng, lưu ngay; nếu không thì **cả phần thưởng** (kể cả Xu) vào hòm thưởng của 7f bằng `reward.Service.Grant` với `source = "chuyen_tau:<id lượt>"` | Không mất thưởng; một luật tất cả hoặc không như 3a; `source` riêng mỗi lượt nên không phát trùng (khóa `(character_id, source)` của 7f) |
| Kết thúc | Thành công hay thất bại, server đẩy `TrainResult`; client tự gửi `TrainLeaveReq` sau 10 giây (thành công) hoặc ngay (thất bại); sau 60 giây ai còn ở lại bị ngắt kết nối với `Kicked{ZONE_RESTART}` | Chuyển zone chỉ chạy được trên goroutine kết nối của người đó; hạn 60 giây giữ bản không sống mãi |
| Rời tàu | `TrainLeaveReq` bất cứ lúc nào: về điểm `default` của Tổng hành dinh; giấy về thành cũng dùng được | Cho thoát giữa chừng |
| Vào lại | Người đã vào một lượt còn chạy thì vào lại được qua Trưởng tàu (không cần trong 2 phút); vào game khi đã lưu ở map tàu thì đặt ở `default` của thành (`town` của map) | Mất kết nối không mất lượt; không bao giờ đặt người vào zone riêng lúc vào game |
| Đóng bản | Bản đóng khi kết thúc và trống, hoặc khi đang chạy mà trống liên tục 60 giây (lượt coi như thất bại) | Không để zone chạy không |
| Đổi khu, xa phu, cổng | Không có trên tàu (`ChangeZoneReq` trả `ZONE_NOT_FOUND`) | Chỉ một bản cho lượt này |

### 1.3. Bổ sung cho game-design

Game-design 5.13 và 6.2 không nói cách vào, số lượt, luật hồi sinh trên tàu, số đợt và thưởng cụ thể. Spec này chốt ở mục 1.2 và ghi vào game-design mục 5.13 (mục 9).

### 1.4. Không làm ở 7g

Map Chuyến Tàu thường (thuộc phần Thế giới mở rộng, tổng quan mục 3.6); bảng xếp hạng thời gian vượt tàu; độ khó nhiều mức; ghép tổ ngẫu nhiên; thưởng theo thời gian còn lại; các hiệu ứng khác ngoài ngủ; vào thay người giữa chừng (người mới vào tổ sau khi mở vẫn vào được nếu còn trong 2 phút, nhưng không thay ai); Akaza (game-design 5.13: không có).

## 2. Dữ liệu

### 2.1. Vùng có tên trong file map

`maps/<id>.json` thêm trường không bắt buộc `areas`:

```json
"areas": [{"id": "car_1", "x": 1, "y": 2, "w": 28, "h": 10}]
```

**Kiểm tra** (`mapdata.Parse`): `id` khác rỗng, không trùng; `w`, `h` dương; nằm trong map; có ít nhất một ô đi được ngoài khu an toàn. 7h dùng lại trường này cho đấu trường Boss.

Trường mới cho map sự kiện: `event_only: true` (không bắt buộc, mặc định `false`): map chỉ vào được qua sự kiện; vào game với vị trí đã lưu ở map này thì đặt ở `default` của map `town` của nó. Script và test đồ thị cổng (4a) bỏ qua map `event_only` khi kiểm tra "mọi map đi tới được từ `kumotori_1`". `event_only` là cờ của **map**, khác cờ `event_only` của **loại quái** (7e: loại chỉ sinh bằng sự kiện, không đặt trong vùng quái).

**Vai NPC `event`** (mới, `mapdata.Parse`): NPC vai `event` phải có trường `event` (id sự kiện, khác rỗng), không có `shop`, `routes`; vai khác không được có `event`. `gamedata.Validate(maps)` kiểm tra `event` là id có thật trong `events.json`. Client mở cửa sổ NPC theo vai, rồi chọn nội dung theo `event` (7g: `chuyen_tau`; 7h: `vo_han_thanh`). `tools/mapgen.py` thêm `truong_tau` "Trưởng tàu", vai `event`, `event: "chuyen_tau"` ở Tổng hành dinh theo luật vị trí NPC của 4b.

### 2.2. Map `tau_vo_tan`

`tools/mapgen.py` sinh `maps/tau_vo_tan.json`:

| Trường | Giá trị |
| --- | --- |
| `name` | Chuyến Tàu Vô Tận |
| `type` | `instance`, `event_only: true`, `town: "ubuyashiki"`, `min_zones: 1`, `max_zones: 1`, `capacity: 5`, `min_level: 66` (trường của 7f), không có `regions`, `portals`, `npcs` |
| Kích thước | 124 × 14: một hành lang cao 10 ô (hàng 2–11), tường bao |
| Toa | `car_1` … `car_4` mỗi toa 28 × 10, nối liền theo chiều ngang (x = 1, 32, 63, 94), giữa hai toa là khe nối rộng 3 ô cao 4 ô (giữa chiều cao) |
| Khu an toàn | `car_k_head` 4 × 10 ở đầu tây mỗi toa |
| Điểm xuất hiện | `default` (= `car_1`) và `car_1` … `car_4` ở giữa khu an toàn đầu toa, đều `respawn: true` |

Kiểm tra liên thông 4a (mọi điểm tới được từ `default`) đúng nhờ các khe nối.

### 2.3. Quái mới

`monster_stats.py` thêm (sửa script, không sửa JSON):

| id | Tên | Hạng | Hành vi |
| --- | --- | --- | --- |
| `train_tentacle` | Xúc tu | `normal` | như `wanderer` |
| `train_flesh` | Mô thịt | `normal` | như `wanderer`, `speed` 0,6 |
| `train_nightmare` | Quái ác mộng | `normal` | như `wanderer` |
| `enmu` | Enmu | `boss` | kỹ năng `dream_wave` (dưới đây) |

Cả bốn loại **không** có `event_only` (7e) vì sẽ dùng ở map Chuyến Tàu thường; ở tàu sự kiện chúng chỉ sinh bằng `SpawnSpecial` (map tàu không có vùng quái).

Chỉ số theo cấp lấy từ bảng `levels` và hệ số hạng của 7d. `enmu` có:

```json
{"id": "dream_wave", "shape": "cone", "pct": 150, "damage": "magic", "range": 5, "cone_deg": 90,
 "cooldown_sec": 10, "telegraph_ms": 800, "max_targets": 5, "status": {"kind": "sleep", "ms": 1000}}
```

Trường `status` (không bắt buộc) của kỹ năng quái là mới: `kind` thuộc `sleep`; `ms` trong 100–5.000. `gamedata` từ chối `status` lạ.

### 2.4. Sự kiện `chuyen_tau` trong `data/events.json`

`docs/balance/event_data.py` (7e) thêm sự kiện loại mới `train` (không sửa tay JSON); cấu hình riêng nằm ở khóa `train` (`gamedata.Event.Train`):

```json
{
  "id": "chuyen_tau", "name": "Chuyến Tàu Vô Tận", "kind": "train", "min_level": 66,
  "summary": "Tổ đội tối đa 5 người vượt bốn toa và hạ Enmu trong 15 phút",
  "schedule": [
    {"days": [1, 2, 3, 4, 5, 6, 7], "start": "15:00", "minutes": 60},
    {"days": [1, 2, 3, 4, 5, 6, 7], "start": "21:00", "minutes": 60}
  ],
  "train": {
  "map": "tau_vo_tan", "max_members": 5,
  "join_sec": 120, "duration_sec": 900, "wave_delay_sec": 3, "end_grace_sec": 60, "empty_close_sec": 60,
  "cars": [
    {"area": "car_1", "head": "car_1_head", "spawn": "car_1", "waves": [
      [{"monster": "train_tentacle", "min_level": 66, "max_level": 67, "count": 8}],
      [{"monster": "train_tentacle", "min_level": 67, "max_level": 68, "count": 6},
       {"monster": "train_flesh", "min_level": 68, "max_level": 68, "count": 2}],
      [{"monster": "train_flesh", "min_level": 68, "max_level": 69, "count": 6}]]},
    "…",
    {"area": "car_4", "head": "car_4_head", "spawn": "car_4",
     "boss": {"monster": "enmu", "level": 75, "hp_mul": 4}}
  ],
  "reward": {"xu": 30000, "stacks": [{"code": "stone_enhance", "count": 20}, {"code": "stone_refine", "count": 2}],
             "equipment": {"tier": 50, "quality_bp": [0, 0, 7500, 2500, 0]}}
  }
}
```

`min_level` của sự kiện chỉ để hiện ở Lịch sự kiện (7e); luật cấp nằm ở `min_level` của map tàu.

| Toa | Đợt 1 | Đợt 2 | Đợt 3 |
| --- | --- | --- | --- |
| 1 | 8 Xúc tu 66–67 | 6 Xúc tu 67–68, 2 Mô thịt 68 | 6 Mô thịt 68–69 |
| 2 | 8 Mô thịt 69–70 | 6 Mô thịt 70–71, 2 Quái ác mộng 71 | 6 Xúc tu 70–71, 3 Quái ác mộng 71 |
| 3 | 8 Quái ác mộng 72–73 | 6 Quái ác mộng 73–74, 3 Mô thịt 73 | 8 Quái ác mộng 74 |
| 4 | Enmu cấp 75, HP × 4 | | |

**Server kiểm tra khi nạp** (phần `train` của 7e: loại `train` phải có khóa `train` và không khóa của loại khác): có đúng một sự kiện loại `train`; `map` có thật, loại `instance`, `max_zones` 1, `capacity` bằng `max_members`; mỗi toa trỏ tới `areas` và điểm xuất hiện có thật, `head` là một khu an toàn nằm trong toa; toa 1–3 có 1–5 đợt, toa cuối có `boss` và không có đợt; mỗi nhóm có loại quái có thật, cấp trong bảng, `count` 1–20; `hp_mul` 1–100; `max_members` 1–5; các thời lượng dương; mã thưởng là vật phẩm chồng có thật; `quality_bp` 5 số, tổng 10.000.

## 3. Lưu trữ

Không có migration. Lượt, ghi nhận lượt theo khung và tiến độ toa chỉ trong bộ nhớ. Zone tàu lưu nhân vật như mọi zone (map `tau_vo_tan`); `world.Enter` đặt lại vị trí theo luật `event_only` (mục 2.1).

## 4. Server

### 4.1. Dịch vụ tàu

Gói mới `internal/train` (thuần, một khóa, không I/O) giữ:

- **Lượt** (`Run`): id lượt (bộ đếm), id lượt sự kiện của khung (7e), id tổ đội lúc mở (0 khi đi một mình), id nhân vật người mở, lúc mở, hạn vào (`mở + join_sec`), hạn chạy (`mở + duration_sec`), tập id nhân vật đã vào, zone của lượt (con trỏ world giữ), trạng thái `open`/`ended`.
- **Ghi nhận theo khung:** `id nhân vật → id lượt` cho id khung hiện tại; đổi khung thì xóa hết.
- Hàm: `Open(window, party, char, now) (*Run, error)`, `CanJoin(window, party, char, now) (*Run, error)`, `Entered(run, char)`, `Ended(run)`, `RunOfParty(party)`, `Info(char, party, now)`.

World nối dịch vụ với zone (mục 4.2–4.3); dịch vụ không gọi world hay zone.

### 4.2. Mở chuyến tàu

`TrainOpenReq{npc_id}` chạy trên goroutine kết nối qua `World.Transfer` với loại chuyển mới. Lỗi đầu tiên được trả:

1. Không có khung `chuyen_tau` đang mở trong ảnh chụp sự kiện: `TRAIN_WINDOW_CLOSED` (world).
2. Người gửi ở trong tổ đội mà không phải trưởng nhóm: `NOT_PARTY_LEADER` (world, đọc dịch vụ tổ đội).
3. Tổ đội (hoặc người đi một mình) đã có lượt chưa kết thúc: `TRAIN_RUN_EXISTS` (world).
4. Nhân vật đã vào một lượt khác trong khung này: `TRAIN_ALREADY_RAN` (world).
5. Chết: `DEAD` (zone, hàm kiểm tra của loại chuyển).
6. NPC không có, không phải vai `event` của `chuyen_tau`: `BAD_REQUEST` (zone).
7. Ngoài tầm NPC: `NPC_TOO_FAR` (zone).
8. Cấp < `min_level` của map tàu: `LEVEL_TOO_LOW` (bước cuối chung của mọi loại chuyển zone, 7f).

Bước 1–4 do world kiểm tra trước khi giữ chỗ; khi chúng qua, world mở zone riêng (mục 4.4) **sau** khi zone cũ chấp nhận ở bước 5–8; nếu zone cũ từ chối thì không mở gì. Hợp lệ: `train.Open` ghi lượt, `Entered`, người mở tới điểm `car_1`; trả `EnterWorldResp` (có `train`, mục 5) mang `seq` của yêu cầu. Kênh Hệ thống gửi riêng cho các thành viên khác của tổ (`chat.SystemTo`): "<tên> đã mở Chuyến Tàu Vô Tận, lên tàu ở Trưởng tàu trong 2 phút".

### 4.3. Lên tàu

`TrainJoinReq{npc_id}`. Lỗi đầu tiên được trả:

1. Không có khung đang mở: `TRAIN_WINDOW_CLOSED`.
2. Nhân vật đã vào lượt R còn chạy: dùng R, bỏ qua bước 3–4.
3. Nhân vật đã vào một lượt khác trong khung (lượt đó đã kết thúc): `TRAIN_ALREADY_RAN`.
4. Không có lượt chưa kết thúc của tổ đội hiện tại của nhân vật, hoặc đã quá hạn vào, hoặc nhân vật không có tổ đội: `TRAIN_JOIN_CLOSED`.
5. Zone của lượt đủ `max_members` người (đếm cả chỗ đang giữ): `ZONE_FULL`.
6. `DEAD`, `BAD_REQUEST` (NPC), `NPC_TOO_FAR`, `LEVEL_TOO_LOW` như mục 4.2 bước 5–8.

Hợp lệ: `Entered`, nhân vật tới điểm đầu toa đang mở xa nhất của lượt (toa 1 nếu chưa mở toa nào); trả `EnterWorldResp`.

### 4.4. Zone riêng

- World giữ zone riêng trong bảng riêng `instances` (id lượt → slot), không nằm trong `zones`, nên `reserve`, `ZoneListReq` và đổi khu không thấy. Giữ chỗ khi chuyển vào bằng `slot.reserved` như 4a.
- `zone.Config.Train *TrainRun` (id lượt, cấu hình `Event.Train`, hạn chạy, hàm `OnEnd(success)`) bật luật phó bản trong zone; sức chứa `max_members`; số khu hiển thị là 1.
- `closeIdle` đóng zone riêng khi: (a) zone báo đã kết thúc và không còn ai, không còn chỗ giữ; hoặc (b) đang chạy mà trống liên tục `empty_close_sec` (khi đó zone tự kết thúc thất bại trước). Đóng zone thì `train.Ended`.

### 4.5. Toa và đợt trong zone

Zone tàu giữ: toa đang mở `car` (0 khi chưa mở), đợt `wave`, thời điểm đợt kế (`nextWaveAt`), tập id quái của đợt hiện tại, trạng thái `running`/`success`/`failed`.

Mỗi tick sau `runSpawnTimers`, bước mới `runTrain`:

1. `running` và tick ≥ hạn chạy: kết thúc **thất bại** (mục 4.8).
2. Chưa mở toa nào, hoặc toa `car` đã sạch: nếu có thành viên còn sống đứng trong vùng toa `car + 1` mà không trong khu an toàn đầu toa đó thì mở toa `car + 1`: toa 1–3 đặt `wave = 1` và sinh đợt 1 ngay; toa cuối sinh Enmu.
3. Đợt hiện tại chết hết: nếu còn đợt thì đặt `nextWaveAt` = tick + `wave_delay_sec` (một lần); tới `nextWaveAt` thì sinh đợt kế. Hết đợt thì toa sạch; đẩy `TrainState`.

**Sinh quái** dùng hàm sinh quái đặc biệt của 7d: mỗi con ở một ô đi được ngẫu nhiên của vùng toa (ngoài khu an toàn, `z.rng`), cấp đều trong khoảng, vùng nhà là vùng toa, không hồi sinh, gắn nhãn lượt; cụ thể `SpawnSpecial(SpecialSpawn{Kind, Level, Pos, Home: [vùng toa], Aggressive: true, Tag: "chuyen_tau:<id lượt>", OnDeath})` (luật `Aggressive` của 7d: đuổi ngay người còn sống gần nhất trong `Home`, chọn lại khi mục tiêu chết hay rời, không bắn, không bỏ đuổi khi mục tiêu còn trong `Home`). Enmu sinh ở tâm vùng toa 4 với `HPMul` = `hp_mul`, `Aggressive: true`, cùng nhãn. Thất bại thì `DespawnTag` bỏ mọi quái còn lại của lượt. Mỗi lần mở toa, đổi đợt, toa sạch hay đổi số quái còn lại thì zone đẩy `TrainState` cho mọi người trong zone.

### 4.6. Ngủ

- `player.sleepUntil` (tick). Đòn quái có `status` khi **trúng** người chơi đặt `sleepUntil = max(sleepUntil, tick trúng + ticks(ms))` và ghi `Hit.status = SLEEP`; nếu người đó vừa chuyển từ thức sang ngủ thì `WorldTick.statuses` có `StatusChange{id, SLEEP, remaining_ms}`; lúc hết ngủ có `StatusChange{id, SLEEP, 0}`. Gửi cho người thấy thực thể đó và chính họ.
- Trong lúc ngủ: `movePlayers` không dời nhân vật (hướng nhập vẫn ghi lại, `ack_input_seq` vẫn tăng); `AttackReq`, `CastReq`, `UseItemReq`, `PickupReq` trả `ASLEEP`, xét **ngay sau** bước `DEAD` của từng lệnh. Chuyển zone, `RespawnReq`, chat, giao dịch không bị chặn.
- Ngủ không đi qua bản trao tay. Chết thì hết ngủ.

### 4.7. Hồi sinh trên tàu

`Area.Train` bật luật: hồi sinh không tại chỗ (bấm hay tự sau 60 giây) đưa tới điểm `car_k` của toa đang mở xa nhất (toa 1 nếu chưa mở toa nào), đầy HP/MP, không mất gì. Hồi sinh tại chỗ như 4c.

### 4.8. Kết thúc

- **Thành công:** Enmu chết (đòn kết liễu). Với mỗi người chơi đang ở trong zone: dựng phần thưởng (Xu, chồng, một trang bị: ô đều trong 10 ô, vũ khí đúng class, phẩm chất theo `quality_bp`, dòng theo 3a mục 4.4, cùng `z.rng`); thử thêm vào bản sao hành trang: chứa hết thì áp thật, cộng Xu, lưu ngay; không thì gọi `Config.Rewards.Grant(reward.Grant{CharID, Source: "chuyen_tau:<id lượt>", Title: "Chuyến Tàu Vô Tận — hoàn thành", Xu, Items})` (không chặn; dịch vụ hòm thưởng của 7f ghi DB trên goroutine của nó). Đẩy `TrainResult{success: true, …}` riêng cho từng người.
- **Thất bại:** hết giờ hoặc bản trống 60 giây. `DespawnTag` bỏ mọi quái còn sống của lượt (không rơi đồ, không EXP); đẩy `TrainResult{success: false}`.
- Sau khi kết thúc: không sinh quái nữa; `OnEnd`; sau `end_grace_sec` ai còn trong zone nhận `Kicked{ZONE_RESTART}` và bị ngắt như mất kết nối (lưu như thường; vào lại thì về thành theo `event_only`).

### 4.9. Rời tàu

`TrainLeaveReq{}`: nhân vật không ở zone tàu: `BAD_REQUEST`. Chết không bị chặn: người đang chết rời tàu thì sống lại đầy HP/MP ở thành, như hồi sinh về làng miễn phí (để người chết lúc thất bại không phải hồi sinh trên tàu rồi mới rời). Hợp lệ: chuyển tới `default` của `ubuyashiki` như giấy về thành (4b); trả `EnterWorldResp`.

### 4.10. Thông tin tàu

`TrainInfoReq{}` (gateway trả thẳng từ world và dịch vụ, không qua zone): khung có đang mở không và còn bao lâu, khung kế tiếp bắt đầu sau bao lâu, nhân vật đã vào lượt nào trong khung chưa, tổ đội có lượt đang nhận người không và còn bao lâu.

### 4.11. Thứ tự tick

```
drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runTrain → runGroundTimers → runPlayerTimers → syncView
```

`runTrain` chỉ chạy ở zone có `Config.Train`. Hết ngủ xét trong `runPlayerTimers`.

## 5. Giao thức

`ProtocolVersion` lên **22**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 215 | `TrainOpenReq` | `string npc_id = 1` (trả lời: `EnterWorldResp`) |
| 216 | `TrainJoinReq` | `string npc_id = 1` (trả lời: `EnterWorldResp`) |
| 217 | `TrainLeaveReq` | rỗng (trả lời: `EnterWorldResp`) |
| 218 | `TrainState` | xem dưới |
| 219 | `TrainResult` | `bool success = 1`, `int64 xu = 2`, `InventoryUpdate update = 3`, `SelfStats stats = 4`, `bool mailed = 5` |
| 220 | `TrainInfoReq` | rỗng |
| 221 | `TrainInfoResp` | `bool window_open = 1`, `int64 window_left_ms = 2`, `int64 next_window_ms = 3`, `bool ran = 4`, `bool party_run_open = 5`, `int64 join_left_ms = 6` |

```proto
enum TrainPhase {
  TRAIN_PHASE_UNSPECIFIED = 0;
  TRAIN_PHASE_RUNNING = 1;
  TRAIN_PHASE_SUCCESS = 2;
  TRAIN_PHASE_FAILED = 3;
}

// The run as everyone aboard sees it.
message TrainState {
  TrainPhase phase = 1;
  int32 car = 2;            // 0: no car opened yet; 1..4
  int32 car_count = 3;
  int32 wave = 4;           // 0 on the boss car
  int32 wave_count = 5;
  int32 monsters_left = 6;  // of the current wave or the boss
  bool car_cleared = 7;
  int64 remaining_ms = 8;   // until the run's time limit
}

enum StatusKind {
  STATUS_KIND_UNSPECIFIED = 0;
  STATUS_KIND_SLEEP = 1; // cannot move, attack, cast, use or pick up items
}

// A status starting (remaining_ms > 0) or ending (0) on an entity.
message StatusChange {
  uint64 id = 1;
  StatusKind kind = 2;
  int32 remaining_ms = 3;
}
```

- `EnterWorldResp.train = 19` (`TrainState`): có khi vào zone tàu, rỗng ở nơi khác.
- `WorldTick.statuses = 10` (`repeated StatusChange`); thứ tự client áp: sau `hits`, trước `deaths`.
- `Hit.status = 7` (`StatusKind`): hiệu ứng đòn này gây cho mục tiêu.
- `TrainState`, `TrainResult` đẩy với `seq = 0`. `TrainResult.mailed`: phần thưởng vào hòm thưởng; khi đó `xu = 0`, `update` rỗng.
- `ErrorCode`: `TRAIN_WINDOW_CLOSED = 99` (không có khung Chuyến Tàu đang mở), `TRAIN_ALREADY_RAN = 100` (đã đi một lượt khác trong khung này), `TRAIN_RUN_EXISTS = 101` (tổ đội đã có lượt đang chạy), `TRAIN_JOIN_CLOSED = 102` (không có lượt của tổ đội để lên, hoặc đã quá 2 phút), `ASLEEP = 103` (nhân vật đang ngủ). Số 104 để trống.
- Tên an toàn với GDScript. Fixture mới: `train_state.hex` (toa 2, đợt 2/3, 5 quái, còn 600.000 ms), `train_result.hex` (thành công, có trang bị trong `update`), `world_tick_sleep.hex` (một `Hit` có `status` và một `StatusChange`), `enter_world_resp_train.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/train_run.gd`** (mới): `apply_state(TrainState, now)`; `phase`, `car`, `wave`, `monsters_left`, `left(now) -> float`; `banner_text(now) -> String` ("Toa 2/4 · Đợt 2/3 · còn 5 quái · 09:58"; toa cuối "Toa 4/4 · Enmu"; toa sạch "Toa 2 đã sạch — tiến sang toa 3"); `result_text(TrainResult) -> String`.
- **`core/status_table.gd`** (mới): `apply(StatusChange, now)`, `apply_hit(Hit, now)` (đặt ngủ tạm 1 giây cho tới khi `StatusChange` tới), `asleep(id, now) -> bool`.
- **`core/predictor.gd`**: khi chính mình đang ngủ thì coi hướng là (0, 0) (vẫn gửi `MoveInput` như cũ khi hướng tay đổi), để dự đoán khớp server.
- **`core/auto_play.gd`** (6c): `step` trả không làm gì khi mình đang ngủ.
- **`core/wire.gd`**: payload `train_*`, `StatusChange`; **`core/errors.gd`**: TRAIN_WINDOW_CLOSED "Chuyến Tàu chưa mở khung giờ này", TRAIN_ALREADY_RAN "Bạn đã đi Chuyến Tàu trong khung giờ này", TRAIN_RUN_EXISTS "Tổ đội đã có chuyến tàu đang chạy", TRAIN_JOIN_CLOSED "Không có chuyến tàu nào của tổ đội để lên", ASLEEP "Đang ngủ".
- `Router.train` giữ `TrainState` cuối để còn khi màn World nạp lại; xóa khi rời tàu.

### 6.2. Giao diện

- **Cửa sổ NPC** (`ui/npc_window.gd`): thêm vai `event`, chọn nội dung theo `event`; nội dung của `chuyen_tau`: dòng giờ khung (từ `TrainInfoResp`), "Đã đi khung này" nếu có; nút **Mở chuyến tàu** (trưởng nhóm hoặc không có tổ), nút **Lên tàu** (khi `party_run_open`, đếm ngược `join_left_ms`), dòng "Cần cấp 66".
- **Thanh tàu** (`ui/train_banner.gd`, mới): trên cùng giữa màn hình khi ở map tàu, theo `banner_text`; nút **Rời tàu** (hỏi lại).
- **Kết quả** (`ui/train_result.gd`, mới): "Chuyến tàu hoàn thành" kèm Xu, các món (hoặc "Phần thưởng đã gửi vào hòm thưởng"), hoặc "Chuyến tàu thất bại"; thành công thì tự gửi `TrainLeaveReq` sau 10 giây, thất bại thì ngay khi bấm Đóng hoặc sau 3 giây.
- **Ngủ**: chữ "Zz" trên đầu thực thể đang ngủ; khi chính mình ngủ thì joystick, nút Đánh và thanh chiêu mờ đi.
- **Cảnh báo vùng của Sóng mộng**: dùng cách vẽ của 7d.

### 6.3. Smoke

`tools/smoke.sh`: `devchar -level 70 -map ubuyashiki` đặt cạnh `truong_tau`; server smoke chạy với `EVENT_DEV_START=chuyen_tau` (7e: lượt dev bắt đầu 60 giây sau khi khởi động) và `EVENT_DEV_MINUTES=5` (7f); smoke chờ tới khi `EventList` báo `chuyen_tau` đang chạy. `tests/smoke/smoke.gd`: gửi `TrainOpenReq`, kiểm tra `EnterWorldResp.map_id == "tau_vo_tan"` và `train.phase == RUNNING`; đi vào toa 1, chờ `TrainState.wave == 1` và `monsters_left > 0`; gửi `TrainLeaveReq`, kiểm tra về `ubuyashiki`; mở lại bị `TRAIN_RUN_EXISTS` hoặc vào lại được (lượt của mình). In "SMOKE OK" kèm "train car 1 wave 1".

## 7. Lệnh dev

- Không có lệnh dev mới. Mở khung khi thử bằng `EVENT_DEV_START=chuyen_tau` (7e) và `EVENT_DEV_MINUTES` (7f).
- Thử nhanh toa cuối: test zone dựng một `Event.Train` nhỏ (một toa một đợt rồi Enmu).

## 8. Kiểm thử

**Script:** `event_data.py --check` cho sự kiện `chuyen_tau`; `monster_stats.py` in HP Enmu thường và bản sự kiện (× 4).

**Server** (`go test -race`):

- `mapdata`: vai `event` có `event`, thiếu `event` hay kèm `shop` bị từ chối; `areas` hợp lệ và các trường hợp từ chối (trùng id, ngoài map, không có ô đi được); `event_only` không bị đòi đi tới được từ `kumotori_1`; map tàu thật nạp được, bốn toa, bốn điểm hồi sinh trong khu an toàn đầu toa.
- `gamedata`: `events.json` thật có sự kiện `chuyen_tau` loại `train`; vai NPC `event` trỏ tới id lạ, NPC `event` thiếu trường `event`, vai khác có `event` bị từ chối; từ chối toa cuối thiếu `boss`, toa thường có `boss`, `head` không nằm trong toa, quái lạ, `hp_mul` 0; `status` kỹ năng lạ; `enmu` là `boss` có `dream_wave`.
- `train` (đồng hồ giả): `Open` hai lần cùng tổ bị `TRAIN_RUN_EXISTS`; người đi một mình mở được, người khác không vào được; `CanJoin` đúng tổ trong 2 phút được, quá 2 phút hoặc khác tổ bị `TRAIN_JOIN_CLOSED`; vào lượt A rồi lượt B cùng khung bị `TRAIN_ALREADY_RAN`; vào lại lượt A được kể cả sau 2 phút; đổi khung xóa ghi nhận.
- `zone` (zone tàu với dữ liệu nhỏ): chưa ai bước vào toa 1 thì không có quái; bước ra khỏi khu an toàn đầu toa thì đợt 1 xuất hiện đủ số, cấp trong khoảng, đều trong toa; đợt chết hết thì đợt 2 xuất hiện đúng 3 giây sau; đứng ở toa 3 khi toa 2 chưa sạch thì toa 3 không mở; Enmu có HP đúng 4 × HP thủ lĩnh cấp 75, công và thủ như bản thường; Sóng mộng trúng người trong quạt lúc đòn tới, người đã chạy ra không trúng; người trúng ngủ: `MoveInput` không dời nhân vật trong 1 giây, `AttackReq`, `CastReq`, `UseItemReq` bị `ASLEEP`, hết 1 giây đi lại được; `StatusChange` bắt đầu và kết thúc; Enmu chết thì mọi người trong zone nhận `TrainResult` thành công, Xu và vật phẩm vào hành trang, lưu ngay; hành trang đầy thì `mailed` và `Rewards.Grant` (giả) được gọi đúng một lần với cả phần thưởng và `source` `chuyen_tau:<id lượt>`; quá 15 phút thì `TrainResult` thất bại và quái bị bỏ; 60 giây sau kết thúc người còn lại bị ngắt với `ZONE_RESTART`; chết trên tàu hồi sinh ở `car_k` của toa xa nhất; không đánh được người cùng tàu (map `instance`).
- `world`: mở lượt tạo zone riêng không có trong `Stats`/danh sách khu, `reserve` không chọn; người thứ sáu bị `ZONE_FULL`; bước lỗi zone (NPC xa) không mở zone; zone riêng trống 60 giây thì đóng; vào game với vị trí lưu ở `tau_vo_tan` thì ở `default` của `ubuyashiki`; `TrainLeaveReq` về `ubuyashiki`.
- `gateway`: `ProtocolVersion` 22; fixture mới; ba lệnh tàu khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestTrainOverWebSocket`: A, B cấp 70 (store), cùng tổ đội, đặt cạnh `truong_tau`; server thử với lượt dev `chuyen_tau` (`Config.DevDelay` 1 giây của 7e); B mở bị `NOT_PARTY_LEADER`; A mở, `EnterWorldResp.map_id` là `tau_vo_tan`; B lên tàu, thấy A; A đi vào toa 1, cả hai nhận `TrainState` đợt 1; A gửi `TrainLeaveReq`, về `ubuyashiki`; A lên tàu lại được.

**Client** (`tests/run.sh`): `train_run` (`banner_text` các pha, đếm ngược); `status_table` (ngủ từ `Hit`, kết thúc bằng `StatusChange` 0); `predictor` đứng yên khi ngủ; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`, `test1`, `test2` cấp 75 cùng tổ, server khởi động với `EVENT_DEV_START=chuyen_tau` và `EVENT_DEV_MINUTES=20`. Kaito mở tàu, Hana lên tàu; qua ba toa, đợt sau tới sau đợt trước; Enmu có cảnh báo quạt, ai trúng thì ngủ 1 giây; hạ Enmu, cả hai nhận thưởng; về thành. Thử lại mở bị từ chối vì đã đi khung này.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 5.13 (cách vào, một lượt mỗi khung, khung 15:00 và 21:00, bốn toa ba đợt, hồi sinh đầu toa, ngủ, thưởng hoàn thành), mục 6.2 (dòng Chuyến Tàu: giờ khung), mục 5.10 (Sóng mộng: quạt 90°, 5 ô).
- Tổng quan Giai đoạn 7: trạng thái 7g.
- `CLAUDE.md`: zone riêng không nằm trong `zones` của world; map `event_only` không bao giờ là nơi vào game; ngủ chặn lệnh ngay sau bước `DEAD`.
- README hai repo: Chuyến Tàu Vô Tận, Trưởng tàu, vai NPC `event`, `EVENT_DEV_START=chuyen_tau`.
