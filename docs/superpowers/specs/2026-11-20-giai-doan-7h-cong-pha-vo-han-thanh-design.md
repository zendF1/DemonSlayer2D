# Giai đoạn 7h — Công Phá Vô Hạn Thành và thành chủ

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần cuối của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md)). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 5.1 (map chiến sự: không phân khu, chết do PK không mất EXP), 5.6 (Hạ Huyền/Thượng Huyền một lần mỗi kỳ, mở map theo tiến trình), 5.7, 6.2 (Công Phá Vô Hạn Thành là sự kiện bang cuối tuần), 6.3 (tiến trình, điểm và quyền chiếm thành, phần thưởng, quyền lợi thành chủ), 7.2 (thuế vào quỹ bang) và 10.3 (các điểm còn mở); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 7b](2026-11-20-giai-doan-7b-giao-dich-design.md) (sạp, phép ghi kèm), [spec 7c](2026-11-20-giai-doan-7c-bang-hoi-design.md) (bang, quỹ), [spec 7d](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) (kỹ năng quái, sinh quái đặc biệt), [spec 7e](2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md) (lịch, loại sự kiện, ảnh chụp sự kiện, `EVENT_DEV_START`), [spec 7f](2026-11-20-giai-doan-7f-thao-phat-thuong-huyen-design.md) (trường map `max_zones`, `capacity`, `min_level`; `Scoreboard`; hòm thưởng `reward.Service.Grant`; `EVENT_DEV_MINUTES`) và [spec 7g](2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md) (vùng có tên `areas`, map `event_only`, vai NPC `event`). 7h chạy sau khi 7g đã triển khai (giao thức 22, migration 00012).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Tối thứ Bảy 20:00–21:00, thành viên các bang từ cấp 50 chạm **Sứ giả Vô Hạn Thành** ở Tổng hành dinh để vào map đang mở xa nhất của Vô Hạn Thành. Năm map mở lần lượt: Ngoại thành có Quỷ Cấp Thấp, Quỷ Dị Hình và ba Hạ Huyền lần lượt xuất hiện; ba map sau mỗi map một Thượng Huyền; map cuối là Quỷ Vương. Hạ Boss của map thì cổng sang map kế mở ngay; hết giờ của map thì Boss biến mất và cổng vẫn mở. Người kết liễu quái và người chơi nhận điểm cá nhân, đồng thời cộng điểm cho bang. Khi sự kiện kết thúc, bang Top 1 chiếm thành nếu Quỷ Vương đã bị hạ. Mọi người đủ điểm tham gia nhận quà ở hòm thưởng, thêm quà mốc điểm bang và quà thứ hạng. Bang chủ của bang thành chủ đặt thuế 0–5% trên mọi lần bán ở sạp; tiền thuế vào quỹ bang.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Lịch | Thứ Bảy **20:00–21:00** UTC+7, sự kiện `vo_han_thanh` trong `data/events.json` (7e báo trước 5 phút, lúc mở, lúc kết thúc) | Game-design 6.2 "sự kiện bang hội cuối tuần"; một giờ đủ cho năm map với thời hạn dưới đây |
| Ai vào | Cấp ≥ **50** (trường `min_level: 50` của năm map, 7f) và là thành viên của một bang **từ trước lúc sự kiện bắt đầu** (`joinedAt` < giờ bắt đầu) | Thượng Huyền là nội dung cuối game (tổng quan mục 3.6); chặn kéo người vào bang giữa chừng để tranh điểm (game-design 10.3 "điều kiện thành viên hợp lệ") |
| Cách vào | Chỉ qua NPC **Sứ giả Vô Hạn Thành** (vai `event`) ở Tổng hành dinh, trong lúc sự kiện chạy; tới điểm vào của **map đang mở xa nhất**. Map Vô Hạn Thành không có cổng từ thế giới ngoài | Người vào muộn tới thẳng mặt trận; map `event_only` (7g) không ai lạc vào ngoài giờ |
| Map | Năm map `vht_1` … `vht_5`, loại `battle`, `event_only`, `max_zones: 1`, `capacity: 150` (7f); mỗi map có khu an toàn ở điểm vào (điểm hồi sinh duy nhất) và đấu trường Boss là vùng `boss_arena` | Game-design 6.3 "không phân khu"; 150 người mỗi map là giới hạn tạm của một zone, cân chỉnh khi thử tải |
| Đối tượng theo map | `vht_1` Ngoại thành: Quỷ Cấp Thấp (cấp 70) gần điểm vào, Quỷ Dị Hình (cấp 75) sâu hơn, rồi ba Hạ Huyền lần lượt ở đấu trường. `vht_2`–`vht_4`: Quỷ Dị Hình và một Thượng Huyền mỗi map. `vht_5`: Quỷ Dị Hình và Quỷ Vương | Đúng thứ tự bảng game-design 6.3 (cấp thấp → dị hình → Hạ Huyền → Thượng Huyền → Quỷ Vương); thứ tự thể hiện bằng vị trí trong map và lúc Boss xuất hiện |
| Bộ Boss v1 | Hạ Huyền Lục Kamanue, Hạ Huyền Tứ Mukago, Hạ Huyền Nhị Rokuro (cấp 80); Thượng Huyền Tam Akaza (85), Thượng Huyền Nhị Doma (88), Thượng Huyền Nhất Kokushibo (90); Quỷ Vương Kibutsuji Muzan (90) | Không lấy Rui, Enmu (đã là thủ lĩnh map thường, game-design 5.5) để tránh lẫn; Thượng Huyền chỉ ở sự kiện (game-design 5.5). 7f dùng ba Thượng Huyền khác (`tp_daki`, `tp_gyokko`, `tp_hantengu`) nên không trùng; loại của 7h có tiền tố `vht_` như tiền tố `tp_` của 7f |
| Quái thường trong map | Quỷ Cấp Thấp hồi sinh 8 giây như quái thường; Quỷ Dị Hình hạng tinh anh, hồi sinh **30 giây** (trường mới `respawn_sec` của vùng quái) | Game-design 6.3: cả hai "chết hồi sinh"; 5 phút của tinh anh map luyện cấp không hợp một trận một giờ (game-design 5.3: không dùng quy tắc đó thay luật sự kiện) |
| Hạ Huyền xuất hiện | Hạ Huyền đầu tiên **3 phút** sau khi sự kiện bắt đầu; mỗi Hạ Huyền sau xuất hiện **30 giây** sau khi con trước chết | Ba phút đầu để các bang vào và đánh quái lấy điểm; game-design 10.3 để mở "khoảng chờ Boss kế tiếp" |
| Thượng Huyền, Quỷ Vương xuất hiện | **30 giây** sau khi map của nó mở | Đủ thời gian đi qua cổng |
| Chết không hồi sinh | Hạ Huyền, Thượng Huyền, Quỷ Vương chỉ sinh một lần mỗi kỳ, chết là xong, không có bộ đếm hồi sinh | Game-design 5.6, 6.3 |
| Thời hạn từng map | `vht_1` 15 phút, `vht_2`–`vht_4` 10 phút, `vht_5` 15 phút, tính từ lúc map mở; tổng tối đa 60 phút | Game-design 6.3 "10 phút là ví dụ"; tổng vừa khung một giờ |
| Mở map kế | Hạ Boss cuối của map mở map kế **ngay**. Hết thời hạn mà Boss còn sống (hoặc còn Hạ Huyền chưa xuất hiện): Boss biến mất, không ai nhận điểm Boss đó, map kế vẫn mở | Game-design 10.3 để mở "hết giờ khi Boss còn sống"; giữ sự kiện chạy tới Quỷ Vương để mọi bang còn tranh điểm, nhưng điểm Boss chỉ đến từ việc hạ |
| Map đã qua | Vẫn chạy tới hết sự kiện: quái thường còn hồi sinh và vẫn cho điểm | Người yếu hơn vẫn có việc làm, không bị đẩy ra |
| Cổng | Mỗi map có cổng sang map kế (khóa tới khi map kế mở, `PORTAL_LOCKED`) và cổng về map trước (luôn mở) | Game-design 6.3 "tiến qua các map" |
| Kết thúc | Khi Quỷ Vương chết, khi hết thời hạn `vht_5`, hoặc khi hết khung giờ, cái nào tới trước | Một điểm kết thúc rõ ràng |
| Điểm quái | Người kết liễu nhận: Quỷ Cấp Thấp **1**, Quỷ Dị Hình **5**, Hạ Huyền **100**, Thượng Huyền **300**, Quỷ Vương **1.000**; cùng số đó cộng vào bang của người đó | Game-design 6.3 "người kết liễu quái nhận điểm cá nhân; đồng thời cộng điểm bang"; Boss đáng giá hơn nhiều để các bang tranh kết liễu |
| Điểm hạ người chơi | **3** điểm cho người kết liễu một người chơi khác bang ở map Vô Hạn Thành; mỗi cặp (người hạ → người bị hạ) chỉ được tính một lần mỗi **60 giây** | Game-design 10.3 để mở; có điểm thì PK có ý nghĩa, giới hạn cặp chặn hai người cho nhau điểm liên tục |
| Ai được điểm | Chỉ người đủ điều kiện vào (cấp, bang từ trước giờ bắt đầu) và **đang** ở đúng bang đó; người rời bang giữa sự kiện không nhận điểm mới | Một luật với điều kiện vào |
| Rời, đổi bang | Điểm đã ghi giữ nguyên cho cá nhân và cho bang lúc ghi; người rời bang mất quà bang (mốc, hạng bang), vẫn giữ quà tham gia và quà hạng cá nhân | Game-design 10.3 để mở; bang không mất điểm vì người đi, người đi không mang quà bang theo |
| Đồng điểm | Xếp theo điểm giảm dần, rồi **ai đạt tổng đó sớm hơn** đứng trên, rồi id nhỏ hơn | Game-design 10.3 để mở; thưởng bên tới trước, không cần chia đôi |
| Chiếm thành | Kết thúc mà Quỷ Vương đã bị hạ: bang Top 1 tổng điểm chiếm thành (có thể không phải bang kết liễu Quỷ Vương). Không hạ được: không ai chiếm, Top 1 chỉ nhận quà bang | Game-design 6.3 |
| Thành chủ cũ khi không ai chiếm | **Giữ thành** tới kỳ sau có bang chiếm được | Game-design 10.3 để mở; thành không bị bỏ trống chỉ vì một tuần thất bại, và thành chủ vẫn có lý do bảo vệ vị trí ở kỳ sau |
| Thời hạn sở hữu | Tới lúc kết thúc kỳ kế tiếp có bang chiếm được | Như trên |
| Thuế | Bang chủ của bang thành chủ đặt **0–5%** (0–500 điểm cơ bản, bước 50); thuế tính trên mỗi lần bán ở **sạp** (7b) ở mọi nơi, **khấu trừ từ tiền người bán**; tiền thuế vào quỹ bang thành chủ bằng phép ghi kèm `GuildFundTax` trong cùng giao dịch DB với lần bán | Game-design 6.3, 7.2; v1 không có NPC đấu giá (tổng quan mục 4) nên phạm vi chịu thuế là sạp; người mua thấy đúng giá niêm yết |
| Giao dịch trực tiếp | Không chịu thuế | Số Xu trong giao dịch 1–1 không phải "giá giao dịch" (có thể là quà, trả nợ) |
| Thuế khi bang mới chiếm thành | **0%** tới khi bang chủ đặt; bang cũ chiếm lại thì giữ mức cũ | Đổi giá người bán nhận phải do bang chủ chủ động đặt và các người bán thấy trước |
| Đổi thuế | Ở cửa sổ Thành chủ, bất cứ đâu; mỗi **10 phút** một lần; kênh Hệ thống báo mức mới cho mọi người | Việc quản trị, không cần đứng ở NPC; giới hạn để không đổi liên tục bẫy người bán |
| Dấu thành chủ | Thành viên bang thành chủ có dấu thành chủ cạnh tên bang (`castle_owner`) | Quyền lợi danh dự dễ thấy; không cần hệ thống danh hiệu (để sau v1) |
| Phần thưởng | Qua hòm thưởng (7f), phát ngay khi kết thúc; bảng ở mục 2.3 | Người chơi có thể ở bất cứ đâu hay ngoại tuyến lúc kết thúc |
| Quỹ bang theo hạng | Top 1/2/3 bang cộng thêm 1.000.000 / 500.000 / 250.000 Xu vào quỹ (`guild.AddFund(ctx, bang, xu)` của 7c, ghi DB) | Thưởng chung cho bang ngoài thưởng từng thành viên |
| PK | Luật map `battle` của 5a (bật PK để đánh, không mất EXP); cùng bang hay cùng tổ không đánh nhau (tổng quan mục 3.5) | Game-design 5.1, 6.3 "có PK tranh mục tiêu" |
| Chết | Hồi sinh miễn phí ở điểm vào của map đó (điểm hồi sinh duy nhất của map); hồi sinh tại chỗ bằng Lượng vẫn được | Luật 4a sẵn có cho map có một điểm hồi sinh |
| Rời | `SiegeLeaveReq` hoặc giấy về thành đưa về Tổng hành dinh; vào lại qua Sứ giả | Tự do ra vào trong giờ |
| Sập server giữa sự kiện | Điểm chỉ trong bộ nhớ nên kỳ đó mất kết quả, không phát thưởng, thành chủ giữ nguyên; ghi log lỗi | Khởi động lại đúng giờ sự kiện là hiếm; lưu điểm liên tục tốn công mà ít lợi |

### 1.3. Bổ sung cho game-design

Game-design 6.3 và 10.3 để mở: khoảng chờ Boss kế tiếp, thứ tự Boss, hết giờ khi Boss còn sống, điểm từng mục tiêu, điểm hạ người chơi, đồng điểm, rời/đổi bang, điều kiện thành viên hợp lệ, cách chia thưởng bang, quyền thành chủ cũ khi Quỷ Vương không bị hạ, thời hạn sở hữu, phạm vi chịu thuế, thời điểm thu và bên trả thuế. Spec này chốt tất cả ở mục 1.2 và 2.3, ghi vào game-design mục 6.3 và bỏ mục 10.3 (mục 9). NPC đấu giá/treo bán của thành để sau v1 (tổng quan mục 4).

### 1.4. Không làm ở 7h

NPC đấu giá/treo bán; danh hiệu; thưởng mốc điểm cá nhân (game-design để mở, chỉ làm mốc bang); giữ phòng tích điểm; nhiều thành; liên minh; lưu điểm qua khởi động lại; xem lại kết quả các kỳ trước trong game (bảng `siege_runs` chỉ để vận hành tra cứu).

## 2. Dữ liệu

### 2.1. Map Vô Hạn Thành

`tools/mapgen.py` sinh `vht_1` … `vht_5` (64 × 48, `type: battle`, `event_only: true`, `min_zones: 1`, `max_zones: 1`, `capacity: 150`, `min_level: 50`, `town: ubuyashiki`):

| Map | Tên | Vùng quái (loại, cấp, số con, hồi sinh) |
| --- | --- | --- |
| `vht_1` | Vô Hạn Thành – Ngoại thành | `vht_lesser` 70 × 15 (hai vùng gần điểm vào), `vht_aberrant` 75 × 6 (hai vùng sâu, 30 giây) |
| `vht_2` | Vô Hạn Thành – Hành lang gỗ | `vht_lesser` 70 × 10, `vht_aberrant` 75 × 6 (30 giây) |
| `vht_3` | Vô Hạn Thành – Sảnh băng | như `vht_2` |
| `vht_4` | Vô Hạn Thành – Tháp trăng | như `vht_2` |
| `vht_5` | Vô Hạn Thành – Điện Quỷ Vương | `vht_aberrant` 75 × 8 (30 giây) |

- Khu an toàn `entry` 10 × 10 sát tường tây, điểm `default` (`respawn: true`) giữa khu; vùng `boss_arena` 16 × 16 ở phía đông, không trùng vùng quái.
- Cổng: `vht_k` có `to_vht_{k+1}` ở tường đông (trừ `vht_5`) và `to_vht_{k-1}` ở tường tây (trừ `vht_1`); điểm đến `from_vht_k` theo luật bố cục 4a. Không có cổng nối với map ngoài; `vht_1` không có cổng tây.
- **Trường mới của vùng quái:** `respawn_sec` (không bắt buộc, 1–3.600): thay thời gian hồi sinh theo hạng của loại quái cho riêng vùng đó.
- `ubuyashiki` thêm NPC `su_gia_vht` "Sứ giả Vô Hạn Thành", vai `event` (7g), `event: "vo_han_thanh"`, theo luật vị trí NPC của 4b.

### 2.2. Quái mới

`monster_stats.py` thêm (chỉ số theo bảng `levels` và hệ số hạng của 7d; kỹ năng theo khung 7d, chiêu vùng có cảnh báo 0,8 giây, thủ lĩnh trúng tối đa 5 người, tinh anh 3):

| id | Tên | Hạng | Kỹ năng hoặc nội tại |
| --- | --- | --- | --- |
| `vht_lesser` | Quỷ Cấp Thấp | `normal` | như `wanderer` |
| `vht_aberrant` | Quỷ Dị Hình | `elite` | Quét tay hình quạt 120°, 4 ô: 150% công vật lý, hồi 6 giây |
| `vht_kamanue` | Hạ Huyền Lục Kamanue | `boss` | Vồ (đơn mục tiêu, lao tới): 160% vật lý, hồi 6 giây; vòng quanh người bán kính 3: 150% vật lý, hồi 8 giây |
| `vht_mukago` | Hạ Huyền Tứ Mukago | `boss` | Dải thẳng dài 6, rộng 1,5: 170% vật lý, hồi 6 giây; nổi giận dưới 30% HP: công +20% |
| `vht_rokuro` | Hạ Huyền Nhị Rokuro | `boss` | Vòng tại mục tiêu bán kính 2,5, tầm 6: 180% phép, hồi 8 giây; quạt 90°, 5 ô: 150% vật lý, hồi 6 giây |
| `vht_akaza` | Thượng Huyền Tam Akaza | `boss` | Vòng quanh người bán kính 3,5: 200% vật lý, hồi 8 giây; lao tới đơn mục tiêu: 180% vật lý, hồi 6 giây; nổi giận dưới 30% HP: công +20% |
| `vht_doma` | Thượng Huyền Nhị Doma | `boss` | Quạt 120°, 6 ô: 180% phép, hồi 8 giây; vòng tại mục tiêu bán kính 3, tầm 7: 200% phép, hồi 10 giây |
| `vht_kokushibo` | Thượng Huyền Nhất Kokushibo | `boss` | Dải thẳng dài 8, rộng 2: 220% vật lý, hồi 8 giây; vòng quanh người bán kính 4: 180% vật lý, hồi 10 giây; nổi giận dưới 30% HP: công +20% |
| `vht_muzan` | Quỷ Vương Kibutsuji Muzan | `boss` | Vòng quanh người bán kính 4,5: 220% vật lý, hồi 8 giây; quạt 150°, 6 ô: 200% phép, hồi 8 giây; dải thẳng dài 9, rộng 2: 240% vật lý, hồi 12 giây; hồi 10% HP một lần dưới 30% HP |

Kỹ năng quái là chuyển thể gameplay (game-design 5.10). Bảy Boss có `event_only: true` (cờ loại quái của 7e: chỉ sinh bằng sự kiện); `vht_lesser`, `vht_aberrant` **không** có cờ này vì nằm trong vùng quái của map Vô Hạn Thành (luật 7e cấm loại `event_only` trong vùng quái).

### 2.3. Sự kiện `vo_han_thanh` trong `data/events.json`

`docs/balance/event_data.py` (7e) thêm sự kiện loại mới `siege` (không sửa tay JSON); cấu hình riêng nằm ở khóa `siege` (`gamedata.Event.Siege`):

```json
{
  "id": "vo_han_thanh", "name": "Công Phá Vô Hạn Thành", "kind": "siege", "min_level": 50,
  "summary": "Các bang công phá năm map, tranh điểm và quyền chiếm thành",
  "schedule": [{"days": [6], "start": "20:00", "minutes": 60}],
  "siege": {
  "first_boss_sec": 180, "boss_gap_sec": 30, "map_boss_sec": 30,
  "maps": [
    {"map": "vht_1", "limit_sec": 900, "bosses": [
      {"monster": "vht_kamanue", "level": 80, "hp_mul": 25, "points": 100},
      {"monster": "vht_mukago", "level": 80, "hp_mul": 25, "points": 100},
      {"monster": "vht_rokuro", "level": 80, "hp_mul": 25, "points": 100}]},
    {"map": "vht_2", "limit_sec": 600, "bosses": [{"monster": "vht_akaza", "level": 85, "hp_mul": 50, "points": 300}]},
    "…",
    {"map": "vht_5", "limit_sec": 900, "bosses": [{"monster": "vht_muzan", "level": 90, "hp_mul": 100, "points": 1000, "final": true}]}
  ],
  "monster_points": {"vht_lesser": 1, "vht_aberrant": 5},
  "player_kill": {"points": 3, "pair_sec": 60},
  "rewards": {"…": "bảng dưới"}
  }
}
```

`min_level` của sự kiện chỉ để hiện ở Lịch sự kiện; luật cấp nằm ở `min_level` của năm map.

`hp_mul` nhân lên HP hạng thủ lĩnh ở cấp đó (Thượng Huyền cấp 85 ≈ 12.966 × 20 × 50 ≈ 13 triệu HP); công và thủ theo hạng thủ lĩnh. Số tạm, cân chỉnh sau thử với nhiều người.

**Phần thưởng** (hòm thưởng; "đá" là `stone_enhance`, "đá tẩy" là `stone_refine`, "ngọc cấp g" là một viên ngẫu nhiên đều trong `gem_attack_g`, `gem_hp_g`, `gem_defense_g` của 3c):

| Loại | Điều kiện | Quà |
| --- | --- | --- |
| Tham gia | ≥ 10 điểm cá nhân | 100.000 Xu, 10 đá, EXP = 5% EXP lên cấp kế tiếp ở cấp lúc kết thúc (`quest.Round10`) |
| Mốc bang 500 | bang ≥ 500 điểm; người nhận đủ điều kiện quà bang | 10 đá |
| Mốc bang 2.000 | bang ≥ 2.000 | 2 đá tẩy |
| Mốc bang 5.000 | bang ≥ 5.000 | 1 ngọc cấp 3 |
| Hạng cá nhân 1 | ≥ 10 điểm | 1.000.000 Xu, 50 đá, 10 đá tẩy, 2 ngọc cấp 5 |
| Hạng cá nhân 2–3 | ≥ 10 điểm | 500.000 Xu, 30 đá, 5 đá tẩy, 1 ngọc cấp 5 |
| Hạng cá nhân 4–10 | ≥ 10 điểm | 250.000 Xu, 20 đá, 3 đá tẩy, 1 ngọc cấp 4 |
| Hạng cá nhân 11–50 | ≥ 10 điểm | 100.000 Xu, 10 đá, 1 đá tẩy |
| Hạng bang 1 | người nhận đủ điều kiện quà bang | 300.000 Xu, 20 đá, 3 đá tẩy |
| Hạng bang 2 | như trên | 200.000 Xu, 15 đá, 2 đá tẩy |
| Hạng bang 3 | như trên | 150.000 Xu, 10 đá, 1 đá tẩy |
| Hạng bang 4–10 | như trên | 50.000 Xu, 5 đá |

- **Đủ điều kiện quà bang:** ≥ 10 điểm cá nhân trong kỳ, và lúc kết thúc vẫn là thành viên của đúng bang mà điểm của người đó đã cộng vào, với `joinedAt` trước giờ bắt đầu.
- Mốc bang cộng dồn (bang 5.000 điểm cho cả ba mốc). Mỗi loại quà là một dòng hòm thưởng riêng với `source` riêng (khóa `(character_id, source)` của 7f): `vo_han_thanh:<id kỳ>:join`, `vo_han_thanh:<id kỳ>:milestone:<500|2000|5000>`, `vo_han_thanh:<id kỳ>:rank`, `vo_han_thanh:<id kỳ>:guild_rank`; tiêu đề như "Vô Hạn Thành – Hạng cá nhân 2". Id kỳ là `RunID` của 7e (giây Unix lúc bắt đầu).
- Quỹ bang theo hạng: mục 1.2.

**Server kiểm tra khi nạp** (phần `siege` của 7e: loại `siege` có khóa `siege` và không khóa của loại khác): có đúng một sự kiện loại `siege`; đúng năm map có thật, loại `battle`, `event_only`, `max_zones` 1, có `boss_arena`; map đầu có 1–5 Boss, các map khác đúng 1; Boss cuối có `final`; loại quái có thật, hạng `boss`, `event_only`, cấp trong bảng; `hp_mul` 1–1.000; `points` ≥ 1; `limit_sec` 60–3.600; `monster_points` trỏ tới loại có thật; mã vật phẩm thưởng có thật; khoảng hạng không chồng nhau.

## 3. Lưu trữ

Migration `00013_castle.sql`:

- Bảng `castle_owner` (một dòng): `id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1)`, `guild_id bigint REFERENCES guilds (id) ON DELETE SET NULL`, `tax_bp smallint NOT NULL DEFAULT 0 CHECK (tax_bp BETWEEN 0 AND 500 AND tax_bp % 50 = 0)`, `since timestamptz`, `tax_changed_at timestamptz`. Up chèn dòng `id = 1` với `guild_id` NULL.
- Bảng `siege_runs`: `id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `started_at timestamptz NOT NULL`, `ended_at timestamptz NOT NULL`, `demon_king_killed boolean NOT NULL`, `captured boolean NOT NULL`, `winner_guild_id bigint REFERENCES guilds (id) ON DELETE SET NULL`, `guilds jsonb NOT NULL` (50 bang đầu: id, tên, điểm), `persons jsonb NOT NULL` (100 người đầu: id nhân vật, tên, id bang, điểm).
- Down xóa hai bảng.

**Store:** `GetCastle(ctx) (Castle, error)`; `SaveSiegeResult(ctx, SiegeResult) error` (một giao dịch: chèn `siege_runs`; nếu chiếm thành: cập nhật `castle_owner` với bang mới, `since = ended_at`, `tax_bp` về 0 nếu bang đổi, giữ nếu cùng bang); `SetCastleTax(ctx, guildID, bp, now) (bool, error)` (cập nhật khi `guild_id` khớp; trả false khi không khớp); `ClearCastleOwner(ctx, guildID)`. Phép ghi kèm `GuildFundTax{GuildID, Xu}` (tổng quan mục 3.2) thêm vào `store`: `UPDATE guilds SET fund = fund + $xu WHERE id = $guild`.

## 4. Server

### 4.1. Dịch vụ Vô Hạn Thành

Gói mới `internal/siege` (dịch vụ mức world, tổng quan mục 3.1) có một goroutine riêng chạy mỗi giây theo `Config.Now` và giữ:

- **Thành:** bang thành chủ (id, tên), thuế, lúc đổi thuế; nạp từ DB lúc khởi động.
- **Kỳ đang chạy:** id kỳ sự kiện (7e), giờ bắt đầu; mỗi map: đã mở chưa, lúc mở, hạn; Boss hiện tại của map (chỉ số trong danh sách), lúc xuất hiện của Boss đó, Boss đó đã sinh, đã chết hay đã bị hủy; điểm cá nhân (id nhân vật → tên, cấp gần nhất, id bang lúc ghi, điểm, lúc đạt tổng hiện tại); điểm bang (id → tên, điểm, lúc đạt); lần cuối từng cặp hạ người chơi.
- **Ảnh chụp** `atomic.Pointer[siege.Snapshot]` (không đổi sau khi đăng) cho zone đọc: kỳ có chạy không, id kỳ; với mỗi map: mở chưa, hạn còn lại, Boss cần có mặt (loại, cấp, `hp_mul`, nhãn "map k, Boss i") hoặc không có; bảng 10 người và 10 bang đầu; thành chủ (id bang, thuế); kết quả kỳ vừa xong (để zone gửi `SiegeResult`).

**Bước mỗi giây:**

1. Ảnh chụp 7e có kỳ `vo_han_thanh` mới mà dịch vụ chưa chạy: bắt đầu kỳ: `vht_1` mở, hạn = bắt đầu + `limit_sec`; Boss 1 xuất hiện lúc bắt đầu + `first_boss_sec`.
2. Map đang mở xa nhất quá hạn: hủy Boss hiện tại và các Boss chưa xuất hiện của map; nếu là map cuối thì kết thúc kỳ (mục 4.7), nếu không thì mở map kế (Boss xuất hiện sau `map_boss_sec`).
3. Kỳ 7e đã đóng mà kỳ còn chạy: kết thúc kỳ.
4. Bang thành chủ không còn (`guild.Exists` sai): xóa thành chủ trong bộ nhớ và `ClearCastleOwner`.
5. Đăng ảnh chụp mới nếu có gì đổi.

**Hàm zone gọi** (giữ khóa ngắn, không I/O): `BossDied(run, mapIndex, bossIndex)`, `MonsterKill(run, killer Scorer, kind string)`, `PlayerKill(run, killer, victim Scorer, now)`. `Scorer{CharID int64, Name string, Level int}`; dịch vụ tra bang bằng `guild.MemberInfo(charID) (guildID uint64, rank, joinedAt, ok)` (7c; `ok` sai là không ở bang), tên bang bằng ô bang hay dịch vụ bang.

- `BossDied`: Boss đó còn hiệu lực (chưa bị hủy, đúng chỉ số) thì đánh dấu chết; còn Boss trong danh sách của map thì Boss kế xuất hiện sau `boss_gap_sec`; nếu không: Boss `final` thì kết thúc kỳ với Quỷ Vương đã bị hạ, còn lại mở map kế ngay.
- Điểm Boss được ghi trong `MonsterKill` với loại Boss (mục 4.4).

### 4.2. Vào Vô Hạn Thành

`SiegeEnterReq{npc_id}` qua `World.Transfer`. Lỗi đầu tiên được trả:

1. Không có kỳ đang chạy: `SIEGE_NOT_ACTIVE` (world).
2. Nhân vật không ở trong bang, hoặc vào bang từ lúc kỳ bắt đầu trở đi: `SIEGE_NOT_ELIGIBLE` (world).
3. Map đang mở xa nhất đã đủ `capacity` người (gồm chỗ đang giữ): `ZONE_FULL` (world).
4. Chết: `DEAD` (zone).
5. NPC không có hoặc không phải vai `event` của `vo_han_thanh`: `BAD_REQUEST` (zone).
6. Ngoài tầm NPC: `NPC_TOO_FAR` (zone).
7. Cấp < `min_level` của map đích: `LEVEL_TOO_LOW` (bước cuối chung của mọi loại chuyển zone, 7f).

Hợp lệ: tới điểm `default` của map đang mở xa nhất; trả `EnterWorldResp` có `siege` (mục 5).

### 4.3. Cổng, rời, vào game

- `UsePortalReq` ở map Vô Hạn Thành: sau bước `BAD_REQUEST` của 4a, cổng `to_vht_{k+1}` khi map đó chưa mở (hoặc không có kỳ đang chạy): `PORTAL_LOCKED` (world, đọc ảnh chụp). Cổng về map trước luôn mở.
- `SiegeLeaveReq{}`: không ở map Vô Hạn Thành: `BAD_REQUEST`. Hợp lệ: tới `default` của `ubuyashiki`; người đang chết sống lại đầy HP/MP như hồi sinh về làng (như `TrainLeaveReq` của 7g).
- Vào game với vị trí lưu ở map Vô Hạn Thành: luật `event_only` (7g) đặt ở `default` của `ubuyashiki`.

### 4.4. Zone của map Vô Hạn Thành

`zone.Config.Siege *SiegeMap{Index int; Service}` bật luật dưới đây. Mỗi giây zone đọc ảnh chụp:

- Ảnh chụp đòi Boss "map k, Boss i" mà zone chưa sinh: `SpawnSpecial(SpecialSpawn{Kind, Level, Pos: tâm boss_arena, Home: [boss_arena], HPMul: hp_mul, Aggressive: true, Tag: "vo_han_thanh:<id kỳ>:<map>:<Boss i>", OnDeath})` của 7d; `OnDeath` gọi `BossDied`. Kênh Hệ thống (do dịch vụ gửi khi Boss xuất hiện): "<tên Boss> đã xuất hiện ở <tên map>".
- Boss đang có mà ảnh chụp không còn đòi (bị hủy, kỳ kết thúc): `DespawnTag` với nhãn của nó (không rơi đồ, không EXP, không điểm).
- Mỗi **2 giây** đẩy `SiegeState` cho mọi người trong zone (mục 5); kỳ vừa kết thúc thì đẩy `SiegeResult` riêng cho từng người trong zone một lần.

**Điểm:**

- Quái chết bởi đòn kết liễu của người chơi P còn trong zone: `MonsterKill(run, P, loại quái)`; dịch vụ cộng `monster_points` của loại, hoặc `points` của Boss nếu đó là Boss hiện hành chưa chết. Quái không có điểm (loại khác) thì không cộng.
- Người chơi V chết bởi đòn kết liễu của người chơi K: `PlayerKill(run, K, V, now)`.
- Dịch vụ chỉ cộng khi người nhận điểm đủ điều kiện (mục 1.2 "Ai được điểm"); với `PlayerKill` thêm: K và V khác bang, cặp K → V chưa được tính trong 60 giây.
- Mỗi lần cộng: điểm cá nhân và điểm bang tăng, "lúc đạt" của mỗi bên là lúc cộng.

EXP, Xu và đồ rơi của quái theo luật thường (Boss rơi theo bảng thủ lĩnh cho người kết liễu).

### 4.5. Bảng xếp hạng

Xếp theo điểm giảm dần, rồi "lúc đạt" tăng dần, rồi id tăng dần. Ảnh chụp mang 10 người và 10 bang đầu; `SiegeState` của từng người điền `board.self` (dòng của chính người đó) và `my_guild` (dòng bang của người đó) (dịch vụ tính trong `Rank(charID, guildID)`, gọi từ zone mỗi 2 giây, giữ khóa ngắn).

`SiegeInfoReq{}` (gateway, từ bất cứ đâu): trả `SiegeInfoResp{state}` dựng từ ảnh chụp (bảng xếp hạng hiện tại hoặc của kỳ vừa xong, thời gian kỳ kế tiếp từ lịch 7e).

### 4.6. Thành chủ và thuế

- `zone.Config.StallTax func() (guildID uint64, bp int)` (7b) do 7h điền từ ảnh chụp: trả id bang thành chủ (khác 0 khi có thành chủ, kể cả khi thuế 0) và thuế; `(0, 0)` khi không có thành chủ. Zone dùng cùng hàm này cho dấu thành chủ (dưới đây), không có hàm thứ hai. 7b tính `thuế = ⌊tiền bán × bp / 10.000⌋`, người bán nhận phần còn lại, lần lưu nhóm của lần bán mang phép `GuildFundTax{bang, thuế}` khi thuế > 0, và zone gọi `guild.AddFundCached(bang, thuế)` (7c, chỉ cộng bộ nhớ) để quỹ hiển thị đúng.
- `EntityEnter.castle_owner` và `LookChange.castle_owner`: người chơi có id bang (ô bang của 7c) bằng id bang thành chủ lấy từ `StallTax`. Zone so id đó mỗi giây; đổi thì ghi `LookChange` cho người chơi có cờ thay đổi (như `pk_on` của 5a: `LookChange` mang toàn bộ ngoại hình).
- `CastleInfoReq{}` trả `CastleInfoResp{castle}` từ bất cứ đâu.
- `CastleTaxReq{tax_bp}` (gateway gọi dịch vụ trên goroutine kết nối). Lỗi đầu tiên được trả:
  1. Nhân vật không phải bang chủ của bang thành chủ: `NOT_CASTLE_OWNER`.
  2. `tax_bp` ngoài 0–500 hoặc không chia hết cho 50: `BAD_REQUEST`.
  3. Chưa đủ 10 phút kể từ lần đổi trước: `CASTLE_TAX_COOLDOWN`.
  Hợp lệ: `SetCastleTax` (false thì `NOT_CASTLE_OWNER`); cập nhật bộ nhớ và ảnh chụp; `chat.System("Thuế sạp Vô Hạn Thành nay là n%")`; trả `CastleTaxResp{castle}`. Đặt bằng mức hiện tại vẫn tính là một lần đổi.

### 4.7. Kết thúc kỳ

Trên goroutine của dịch vụ:

1. Chốt bảng xếp hạng; chiếm thành nếu Quỷ Vương đã bị hạ và có bang Top 1.
2. `SaveSiegeResult` (thử lại như saver nếu DB lỗi, tối đa 30 giây giữa các lần); thành công mới đổi thành chủ trong bộ nhớ.
3. Phát thưởng theo mục 2.3: mỗi dòng một `reward.Service.Grant(reward.Grant{CharID, Source, Title, Xu, Exp, Items})` (7f, không chặn; ngọc và đá là `items.Item`); quỹ theo hạng bằng `guild.AddFund(ctx, bang, xu)`. `source` cố định theo kỳ nên thử lại sau lỗi không phát trùng.
4. `chat.System`: "Bang <tên> đã chiếm Vô Hạn Thành" hoặc "Không bang nào chiếm được Vô Hạn Thành; <tên> vẫn là thành chủ" / "… thành vẫn bỏ trống".
5. Đăng ảnh chụp có kết quả; zone gửi `SiegeResult` (mục 4.4) và bỏ Boss còn lại. Người chơi ở lại map được, quái thường vẫn còn nhưng không còn điểm.

### 4.8. Thứ tự tick

Không đổi. Việc đọc ảnh chụp chạy cùng bước xét mỗi giây của 6d/7e (trong `runPlayerTimers` hoặc bước sự kiện của 7e).

## 5. Giao thức

`ProtocolVersion` lên **23**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 225 | `SiegeEnterReq` | `string npc_id = 1` (trả lời: `EnterWorldResp`) |
| 226 | `SiegeLeaveReq` | rỗng (trả lời: `EnterWorldResp`) |
| 227 | `SiegeState` | xem dưới; đẩy với `seq = 0` |
| 228 | `SiegeInfoReq` | rỗng |
| 229 | `SiegeInfoResp` | `SiegeState state = 1` |
| 230 | `SiegeResult` | xem dưới; đẩy với `seq = 0` |
| 231 | `CastleInfoReq` | rỗng |
| 232 | `CastleInfoResp` | `CastleInfo castle = 1` |
| 233 | `CastleTaxReq` | `int32 tax_bp = 1` |
| 234 | `CastleTaxResp` | `CastleInfo castle = 1` |

```proto
// One siege map as players see it.
message SiegeMap {
  string map_id = 1;
  bool open = 2;
  int64 left_ms = 3;      // until this map's limit; 0 when not open or over
  string boss_name = 4;   // empty: no boss present now
  int64 boss_in_ms = 5;   // > 0: the next boss appears in this many ms
  int32 bosses_left = 6;  // bosses of this map not yet killed or cancelled
}

message SiegeState {
  bool active = 1;
  repeated SiegeMap maps = 2;
  // 7f's Scoreboard: event_id "vo_han_thanh", run_id, top 10 players, self
  // (the receiver's row), top 10 guilds, ends_in_ms of the whole run, final.
  Scoreboard board = 3;
  ScoreRow my_guild = 4;      // the receiver's guild row; rank 0: no points
  int64 next_start_ms = 5;    // when not active: until the next siege
}

message SiegeResult {
  bool demon_king_killed = 1;
  bool captured = 2;
  string owner_guild = 3;     // the castle owner after this siege; empty: none
  int64 my_points = 4;
  int32 my_rank = 5;
  int32 my_guild_rank = 6;
  int32 rewards = 7;          // mailbox entries granted to this character
}

message CastleInfo {
  uint64 guild_id = 1;        // 0: no owner
  string guild_name = 2;
  int32 tax_bp = 3;
  int64 since_ms = 4;         // Unix ms of the capture; 0 when no owner
  int64 next_siege_ms = 5;    // until the next siege starts
  int64 tax_change_in_ms = 6; // until the tax can be changed again; 0: now
}
```

- `Scoreboard` và `ScoreRow` là message của 7f (`ScoreRow{rank, name, guild_name, points}`; `Scoreboard{event_id, run_id, players, self, guilds, ends_in_ms, final, success, killer_name}`); 7h dùng đúng định nghĩa đó, nhúng trong `SiegeState` thay vì đẩy `Scoreboard` riêng (envelope 203), để một gói mang cả map và bảng. Bảng cuối có `final`, `success` = Quỷ Vương đã bị hạ, `killer_name` = người kết liễu Quỷ Vương.
- `EnterWorldResp.siege = 20` (`SiegeState`): có khi vào map Vô Hạn Thành. `EnterWorldResp.castle = 21` (`CastleInfo`): luôn có.
- `EntityEnter.castle_owner = 19` (`bool`), `LookChange.castle_owner = 6` (`bool`).
- `ErrorCode`: `SIEGE_NOT_ACTIVE = 105` (Vô Hạn Thành chưa mở), `SIEGE_NOT_ELIGIBLE = 106` (không ở bang hoặc vào bang sau giờ bắt đầu), `PORTAL_LOCKED = 107` (cổng chưa mở), `NOT_CASTLE_OWNER = 108` (chỉ bang chủ của bang thành chủ), `CASTLE_TAX_COOLDOWN = 109` (vừa đổi thuế). Số 110–112 để trống.
- Tên an toàn với GDScript. Fixture mới: `siege_state.hex` (hai map mở, một Boss, bảng ba người và hai bang), `siege_result.hex` (chiếm thành), `castle_info.hex` (thuế 300), `entity_enter_castle.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/siege_board.gd`** (mới): `apply_state(SiegeState, now)`; `frontier()` (map mở xa nhất), `map_line(i, now) -> String` ("Hành lang gỗ · Akaza · còn 08:12", "Ngoại thành · Hạ Huyền kế tiếp sau 0:25", "Khóa"); `my_line() -> String` ("Điểm 120 · hạng 7 · bang 1.540 điểm, hạng 2", từ `board.self` và `my_guild`); phần bảng dùng `core/scoreboard.gd` của 7f (`apply(board, now)`); `result_text(SiegeResult) -> String`.
- **`core/castle.gd`** (mới): `apply(CastleInfo, now)`; `owner_name`, `tax_pct() -> float`, `can_set(self_guild_id, self_rank)`; `net_of(price) -> int` (tiền người bán nhận sau thuế, cùng công thức server) cho cửa sổ sạp.
- **`core/wire.gd`**: payload mới; **`core/errors.gd`**: SIEGE_NOT_ACTIVE "Vô Hạn Thành chưa mở", SIEGE_NOT_ELIGIBLE "Cần ở trong bang từ trước giờ bắt đầu", PORTAL_LOCKED "Cổng chưa mở", NOT_CASTLE_OWNER "Chỉ bang chủ của bang thành chủ", CASTLE_TAX_COOLDOWN "Vừa đổi thuế, thử lại sau".
- `Router.castle` giữ `CastleInfo` (từ `EnterWorldResp.castle` và các phản hồi); `Router.siege` giữ `SiegeState` cuối.

### 6.2. Giao diện

- **Cửa sổ NPC Sứ giả** (vai `event` của 7g, nội dung theo `event: "vo_han_thanh"`): giờ kỳ kế tiếp hoặc "Đang diễn ra"; thành chủ và thuế; nút **Vào Vô Hạn Thành** (khi `active`), dòng "Cần cấp 50 và ở trong bang từ trước giờ bắt đầu"; nút **Bảng xếp hạng** (gửi `SiegeInfoReq`).
- **Khung Vô Hạn Thành** (`ui/siege_panel.gd`, mới): bên phải dưới khung theo dõi khi ở map Vô Hạn Thành: năm dòng `map_line`, dòng `my_line`, nút **Xếp hạng** mở cửa sổ hai tab Cá nhân / Bang (10 dòng đầu, dòng của mình), nút **Rời** (hỏi lại).
- **Kết quả** (`ui/siege_result.gd`, mới): chiếm thành hay không, thành chủ, điểm và hạng, "Đã gửi n phần quà vào hòm thưởng".
- **Cửa sổ Thành chủ** (`ui/castle_window.gd`, mới; mở từ cửa sổ bang của 7c và từ NPC): bang thành chủ, từ ngày, thuế; bang chủ của bang thành chủ có thanh chọn 0–5% bước 0,5% và nút **Đặt thuế** (mờ khi `tax_change_in_ms` > 0).
- **Dấu thành chủ** (`ui/entity_view.gd`): biểu tượng thành nhỏ trước tên bang khi `castle_owner`.
- **Sạp** (7b, `ui/stall_setup.gd`): cửa sổ bày sạp ghi "Thuế thành n% · nhận x Xu" theo `castle.net_of`; client gửi `CastleInfoReq` khi mở cửa sổ bày sạp.
- **Cổng khóa**: chạm cổng khóa thì báo "Cổng chưa mở"; `portal_watch` không gửi lại liên tục (chờ `SiegeState` báo map mở).

### 6.3. Smoke

`tools/smoke.sh`: nhân vật smoke cấp 60, vào một bang bằng `devguild -create <tên bang> -leader <nhân vật>` (7c, chạy khi server tắt, nên `joined_at` luôn trước kỳ dev), đặt cạnh `su_gia_vht`; server smoke chạy với `EVENT_DEV_START=vo_han_thanh` (7e: kỳ dev bắt đầu 60 giây sau khi khởi động) và `EVENT_DEV_MINUTES=5` (7f); smoke chờ `EventList` báo đang chạy. `tests/smoke/smoke.gd`: `SiegeEnterReq` → `map_id == "vht_1"`, `siege.active`; đánh một Quỷ Cấp Thấp tới chết, chờ `SiegeState.board.self.points ≥ 1`; dùng cổng `to_vht_2` bị `PORTAL_LOCKED`; `SiegeLeaveReq` về `ubuyashiki`. In "SMOKE OK" kèm "siege points 1".

## 7. Lệnh dev

- `EVENT_DEV_START=vo_han_thanh` (7e) và `EVENT_DEV_MINUTES` (7f) chạy một kỳ dev; `devguild` (7c) dựng bang cho thử.
- `cmd/devcastle` (mới, chạy khi server tắt): `-guild <tên>` đặt thành chủ, `-tax <bp>` đặt thuế, `-clear` bỏ thành chủ. Tên bang lạ thì báo lỗi, không ghi gì.

## 8. Kiểm thử

**Script:** `event_data.py --check` cho sự kiện `vo_han_thanh`; `monster_stats.py` in HP các Boss sự kiện sau `hp_mul`.

**Server** (`go test -race`):

- `mapdata`: `respawn_sec` của vùng (1–3.600, ngoài khoảng bị từ chối); năm map thật nạp được, mỗi map một điểm hồi sinh trong `entry`, có `boss_arena`; đồ thị cổng giữa `vht_k` hai chiều.
- `gamedata`: `events.json` thật có sự kiện `vo_han_thanh` loại `siege`; từ chối Boss thiếu `event_only`, `vht_lesser` có `event_only`, map không phải `battle`, map đầu không có Boss, map khác có hai Boss, thiếu `final`, Boss không phải hạng `boss`, khoảng hạng chồng nhau, mã thưởng lạ.
- `siege` (đồng hồ giả, dịch vụ bang giả, hòm thưởng giả): kỳ bắt đầu khi ảnh chụp 7e có kỳ mới; Boss 1 đòi có mặt đúng 180 giây sau; `BossDied` thì Boss 2 sau 30 giây; Boss 3 chết thì `vht_2` mở ngay, Boss của nó sau 30 giây; `vht_1` quá 15 phút khi còn Boss: Boss bị hủy, `vht_2` mở; hủy rồi mới báo `BossDied` (đòn tới muộn) không cộng điểm Boss; `MonsterKill` cộng đúng điểm cho cá nhân và bang; người vào bang sau giờ bắt đầu không được điểm; người rời bang giữa kỳ không được điểm mới; `PlayerKill` cùng cặp trong 59 giây chỉ tính một lần, 61 giây tính lại; cùng bang không tính; đồng điểm thì ai đạt trước đứng trên; Quỷ Vương chết: kỳ kết thúc, bang Top 1 chiếm thành dù bang khác kết liễu, `SaveSiegeResult` có `captured`, thuế về 0 khi bang đổi và giữ khi cùng bang; hết `vht_5` không hạ Quỷ Vương: không chiếm, thành chủ cũ giữ; quà: đúng các dòng theo mục 2.3 cho người 10 điểm hạng 1, người 9 điểm không có gì, người đã rời bang không có quà bang nhưng có quà cá nhân; quỹ Top 1–3 đúng; bang thành chủ bị giải tán thì thành chủ bị xóa.
- `siege` thuế: người không phải bang chủ bang thành chủ `NOT_CASTLE_OWNER`; 501 và 120 `BAD_REQUEST`; đổi lần hai trong 10 phút `CASTLE_TAX_COOLDOWN`; ảnh chụp mang thuế mới.
- `zone` (map Vô Hạn Thành với dữ liệu nhỏ): Boss sinh ở `boss_arena` khi ảnh chụp đòi, chỉ một lần; ảnh chụp hủy Boss thì Boss biến mất không rơi đồ; người chơi kết liễu quái gọi `MonsterKill` với đúng loại; người chơi hạ người chơi gọi `PlayerKill`; `SiegeState` mỗi 2 giây; `SiegeResult` một lần khi kỳ kết thúc; vùng có `respawn_sec` 30 hồi sinh sau 30 giây; `castle_owner` trong `EntityEnter` và `LookChange` khi thành chủ đổi.
- `zone` thuế sạp: bán 100.000 Xu với thuế 500: người bán nhận 95.000, phép `GuildFundTax` 5.000 nằm trong cùng lần lưu nhóm; thuế 0 hay không có thành chủ thì không có phép.
- `store` (DB): migration lên rồi xuống; dòng `castle_owner` mặc định; `SaveSiegeResult` chiếm thành rồi đọc lại; `SetCastleTax` sai bang trả false; xóa bang làm `guild_id` NULL; phép `GuildFundTax` cộng quỹ trong cùng giao dịch với nhân vật (lỗi phần nhân vật thì quỹ không đổi).
- `world`: `SiegeEnterReq` theo đúng thứ tự lỗi mục 4.2; tới map mở xa nhất; cổng khóa `PORTAL_LOCKED`; `SiegeLeaveReq` về thành; vào game với vị trí lưu ở `vht_3` thì ở `ubuyashiki`.
- `gateway`: `ProtocolVersion` 23; fixture mới; `SiegeEnterReq`, `SiegeLeaveReq`, `CastleTaxReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- `cmd/devcastle`: đặt và xóa thành chủ; bang lạ không ghi gì.
- e2e `TestSiegeOverWebSocket`: A (bang X), B (bang Y) cấp 60, vào bang trước khi server thử khởi động (store của 7c); server thử với kỳ dev `vo_han_thanh` (`Config.DevDelay` 1 giây, `DevMinutes` 1 của 7e); A vào, ở `vht_1`; C (không bang) bị `SIEGE_NOT_ELIGIBLE`; A hạ một Quỷ Cấp Thấp, `SiegeState.board.self.points` = 1; B vào, cả hai bật PK, A hạ B, A có 4 điểm; A hạ B lần hai trong 60 giây vẫn 4; hết 1 phút của kỳ dev: không chiếm thành; `CastleInfoResp` không có thành chủ; đặt thành chủ X bằng store rồi khởi động lại world trong test: A đặt thuế 300 được, B bị `NOT_CASTLE_OWNER`.

**Client** (`tests/run.sh`): `siege_board` (`map_line` cho map khóa, đang mở có Boss, chờ Boss kế; `my_line`; `result_text`); `castle` (`net_of` khớp server: 100.000 với 500 là 95.000, 99 với 300 là 97; `can_set`); giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`, `test1` (Kaito, bang A) và `test2` (Hana, bang B) cấp 90, bang dựng bằng `devguild`, server khởi động với `EVENT_DEV_START=vo_han_thanh` và `EVENT_DEV_MINUTES=60`. Cả hai vào, đánh quái lấy điểm, PK nhau; Hạ Huyền xuất hiện sau 3 phút; hạ đủ Boss để mở các map; khung bên phải cập nhật điểm và hạng; kết thúc, bang Top 1 chiếm thành nếu hạ được Quỷ Vương (dùng `devchar` cấp và đồ cao, hoặc chờ hết giờ để thấy nhánh không chiếm); mở hòm thưởng nhận quà; bang chủ thành chủ đặt thuế 5%; Hana bày sạp và Kaito mua: Hana nhận 95%, quỹ bang thành chủ tăng 5%.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 6.3 (lịch thứ Bảy 20:00, điều kiện vào, năm map và bộ Boss v1, thời hạn, hết giờ, điểm, đồng điểm, rời bang, chiếm thành, thành chủ cũ giữ thành, thuế 0–5% trên sạp khấu trừ người bán, quà), mục 7.2 (phạm vi chịu thuế là sạp; NPC đấu giá để sau v1), mục 10.3 bỏ (đã chốt), mục 11 thêm NPC đấu giá.
- Spec Giai đoạn 0 mục 1.1 và 3.3 theo tổng quan Giai đoạn 7 mục 5 (bảng `castle_owner`, `siege_runs`).
- Tổng quan Giai đoạn 7: trạng thái 7h; Giai đoạn 7 xong.
- `CLAUDE.md`: dịch vụ Vô Hạn Thành quyết Boss nào có mặt, zone chỉ sinh/bỏ theo ảnh chụp; thuế sạp chỉ vào quỹ qua phép `GuildFundTax` trong cùng lần lưu nhóm.
- README hai repo: Vô Hạn Thành, thành chủ, thuế, `devcastle`.
