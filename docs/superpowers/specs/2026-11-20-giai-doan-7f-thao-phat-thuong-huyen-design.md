# Giai đoạn 7f — Thảo Phạt Thượng Huyền và hòm thưởng

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ sáu của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md) mục 1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 6.2 (Thảo Phạt Thượng Huyền: 2–3 lần/ngày, Boss thế giới ở vùng tranh chấp, các bang vừa đánh Boss vừa PK, không phân khu; thưởng trang bị hiếm, đá cường hóa, vật phẩm bang), 5.1 (map chiến sự: không phân khu, chết do PK không mất EXP), 5.5 và 5.6 (Thượng Huyền chỉ xuất hiện trong sự kiện, cấu hình riêng), 5.7 (PK), 5.8 (EXP theo sát thương, đồ thuộc người kết liễu), 5.9 và 5.10 (chỉ số và chiêu thủ lĩnh) và 5.12 (rơi đồ); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 7e](2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md) (lịch, ảnh chụp sự kiện, `events.json`), [spec 7d](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) (thủ lĩnh, kỹ năng quái có cảnh báo, hàm sinh quái đặc biệt), [spec 7c](2026-11-20-giai-doan-7c-bang-hoi-design.md) (bang, quỹ), [spec 7b](2026-11-20-giai-doan-7b-giao-dich-design.md) (phép ghi kèm của saver), [spec 5a](2026-11-20-giai-doan-5a-pk-phat-exp-design.md) và [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) (khu, chuyển zone). 7f chạy sau khi 7e đã triển khai (giao thức 20, migration 00011).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Tổng hành dinh có cổng phía bắc tới **Hoang Sơn Chiến Địa**, một map chiến sự **một khu**, tối đa 150 người, chỉ nhân vật từ cấp 50 vào được. Mỗi ngày lúc 13:00 và 18:00, trong 30 phút, một **Thượng Huyền** (cấp 80, ba Boss thay phiên theo ngày) xuất hiện giữa đấu trường. Người chơi các bang cùng đánh Boss trong khi vẫn PK được nhau (không mất EXP; cùng tổ đội, cùng bang không đánh nhau). Bên phải màn hình là **bảng sát thương** cá nhân và bang, cập nhật mỗi 2 giây.

Boss chết hoặc hết giờ thì sự kiện kết thúc: người chơi nhận thưởng theo hạng sát thương, người kết liễu có thêm thưởng, bang gây nhiều sát thương nhất được cộng quỹ. Thưởng gửi vào **Hòm thưởng** mới: nút trên HUD có số thưởng chưa nhận, mở ra là danh sách thưởng, bấm Nhận để lấy Xu và đồ vào hành trang. Thưởng tới cả khi nhân vật đang ngoại tuyến hay ở map khác.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Map | Map mới `hoang_son` "Hoang Sơn Chiến Địa", loại `battle`, 64 × 48, sinh bằng `tools/mapgen.py` | Game-design 6.2 "vùng tranh chấp"; map chiến sự đầu tiên (5.1) |
| Đường vào | Cổng phía bắc của Tổng hành dinh ↔ cổng phía nam của Hoang Sơn; không có tuyến Xa phu | Một đường chuyển zone (4a), tự tìm đường 6a dùng được; thành ngay cạnh để mua bình |
| Một khu | Trường map mới `max_zones` (1 với Hoang Sơn) và `capacity` (150); không đổi khu, không mở khu thêm; đầy thì `ZONE_FULL` | Game-design 5.1, 6.2 "không phân khu"; 150 đủ cho nhiều bang mà AOI 10 ô vẫn chia tải (tổng quan mục 3.4) |
| Cấp vào | Trường map mới `min_level` (50 với Hoang Sơn): qua cổng, giấy về thành hay mọi đường chuyển zone tới map đó khi cấp thấp hơn bị `LEVEL_TOO_LOW`; vào game với vị trí đã lưu ở map đó mà cấp thấp hơn thì về `default` của `town` | Tổng quan mục 3.6; luật nằm trên map nên đúng cả ngoài giờ sự kiện |
| Quái thường ở Hoang Sơn | Không có; ngoài giờ sự kiện map trống | Map tranh chấp chỉ có nội dung theo hoạt động; không phải cân bãi cấp 50+ khi chưa có map 21–90 |
| Trại | Khu an toàn `camp` ở mép nam, có cổng về thành, điểm hồi sinh, lửa trại, Dược sư | Chết thì hồi sinh tại trại như mọi map (4a); mua bình không phải về thành |
| PK | Luật map `battle` của 5a: đánh người được ngoài khu an toàn, không phạt EXP; cùng tổ đội hoặc cùng bang không đánh nhau (tổng quan mục 3.5) | Game-design 6.2 "các bang vừa đánh Boss vừa PK"; 5.7 không mất EXP ở map chiến sự |
| Giờ | 13:00 và 18:00 mỗi ngày, 30 phút | Game-design 6.2 "2–3 lần/ngày"; tránh 12:00/19:00/21:30 (7e) và 15:00/21:00 (7g), và Vô Hạn Thành tối thứ Bảy 20:00 (7h) |
| Boss | Ba Thượng Huyền v1, mỗi ngày một con theo `quest.Day(start) mod 3`: Thượng Huyền Lục – Daki, Thượng Huyền Ngũ – Gyokko, Thượng Huyền Tứ – Hantengu. Hai khung trong ngày cùng một Boss | Game-design 5.5 "Thượng Huyền chỉ xuất hiện trong sự kiện"; ba Boss đủ đổi mới mỗi ngày; Thượng Huyền mạnh hơn (Tam, Nhị, Nhất) để dành cho Vô Hạn Thành (7h) |
| Cấp và hạng Boss | Cấp 80, hạng thủ lĩnh (`boss`) của 7d (công ×2, thủ ×1,5, HP ×20 so với quái thường cùng cấp), rồi **HP ×125** và **EXP gốc ×125** của sự kiện | Tổng quan mục 3.6 (Boss cấp 80); mục 1.3 tính số |
| Kỹ năng Boss | Mỗi Boss 2–3 chiêu theo dạng của 7d (`line`, `cone`, `circle_self`, `circle_target`), cảnh báo 0,8 giây, tối đa 5 người mỗi chiêu (mục 2.2) | Game-design 5.10: chiêu vùng thủ lĩnh tối đa 5 người, cảnh báo 0,8 giây |
| Đấu trường | Boss sinh ở tâm đấu trường, vùng nhà là hình chữ nhật đấu trường; không bao giờ đuổi ra ngoài; không hồi máu khi bỏ đuổi (7d) | Không kéo Boss vào trại; không reset Boss để ăn gian |
| Sát thương tính hạng | Sát thương **tính EXP** lên Boss (đã kẹp theo HP còn lại, luật 5.8 và 7d) của từng nhân vật; điểm bang là tổng của thành viên theo bang **lúc ra đòn** | Một số liệu với EXP, không đếm sát thương thừa; đổi bang giữa trận không mang điểm theo |
| Ngưỡng tham gia | Tổng sát thương ≥ 0,1% HP tối đa của Boss (31.165) | Đứng ở trại không có quà; khoảng 15–20 giây đánh của nhân vật cấp 50 |
| Thứ hạng | Theo sát thương giảm dần; bằng nhau thì người đạt tổng đó sớm hơn (tick của lần cộng cuối nhỏ hơn) xếp trên | Không hòa; thưởng người tới trước (game-design 6.3 để mở đồng điểm) |
| Thưởng theo hạng (Boss bị hạ) | Mục 2.1: hạng 1, 2–3, 4–10, 11–30, tham gia; Xu, đá cường hóa, đá tẩy luyện, trang bị bậc 50 Cam/Vàng ở các hạng cao | Game-design 6.2 "trang bị hiếm, đá cường hóa"; trang bị bậc 50 là trần rơi đồ thường (5.12); Cam chỉ ở hạng 1 như tỷ lệ Cam 5% của thủ lĩnh |
| Người kết liễu | Thêm một thưởng: 50.000 Xu và 3 đá tẩy luyện; đồ rơi thủ lĩnh (5.12) vẫn rơi cho người kết liễu như mọi quái | Game-design 5.8 tách quyền nhặt khỏi điểm sự kiện; kết liễu là phần thưởng nhỏ, không lấn thưởng hạng |
| Thưởng bang | Bang có tổng sát thương cao nhất được cộng **300.000 Xu vào quỹ bang** (7c `AddFund`) | Game-design 6.2 "vật phẩm bang"; v1 chưa có vật phẩm bang (tổng quan mục 4), quỹ là tài sản chung đã có |
| Hết giờ khi Boss còn sống | Boss biến mất; vẫn xếp hạng và thưởng, nhưng Xu và đá **giảm một nửa** (làm tròn xuống), không trang bị, không thưởng kết liễu, không thưởng bang | Người chơi đã bỏ công vẫn có quà (game-design 6.3 "cứ tham gia là có quà" cho sự kiện bang); hạ được Boss vẫn hơn hẳn |
| Kết thúc sớm | Boss chết thì lượt kết thúc ngay (báo "đã kết thúc", dải sự kiện tắt) | Không treo dải sự kiện khi đã xong |
| Ai tính bảng | Zone của Hoang Sơn (map một khu) giữ bảng sát thương, đẩy bảng cho người trong map và đưa kết quả cuối cho dịch vụ sự kiện; dịch vụ phát thưởng | Sát thương xảy ra trong zone; zone không ghi DB (CLAUDE.md) |
| Bảng hiển thị | Mỗi 2 giây cho mọi người trong map khi lượt chạy: top 10 cá nhân, dòng của chính mình, top 5 bang, thời gian còn lại; bảng cuối có kết quả. Message `Scoreboard` dùng chung, 7h dùng lại | Đủ thông tin mà gói nhỏ; một message cho mọi sự kiện có điểm |
| Hòm thưởng | Bảng `character_rewards`; dịch vụ mức world `internal/reward` ghi DB trên goroutine riêng và báo cho nhân vật đang online; zone giữ danh sách thưởng chưa nhận của người chơi; nhận từng thưởng | Thưởng tới được người ngoại tuyến hay ở zone khác mà không ghi đè trạng thái zone đang giữ (tổng quan mục 3.1) |
| Nhận thưởng | Tất cả hoặc không: hành trang không chứa hết đồ của thưởng thì `BAG_FULL`; nhận thì cộng đồ, Xu, EXP trong zone và lưu ngay kèm phép `ClaimRewards`, nên đánh dấu đã nhận và trạng thái nhân vật cùng một giao dịch DB | Không mất, không nhân đôi thưởng khi sập server (tổng quan mục 3.2) |
| Phát hai lần | Mỗi thưởng có `source` riêng (`thao_phat:<run_id>:rank`, `…:killer`); cặp (nhân vật, `source`) là duy nhất, chèn trùng bị bỏ qua | Thử lại khi DB lỗi hay khởi động lại không phát trùng |
| Hạn | Thưởng chưa nhận giữ **30 ngày** từ lúc phát; quá hạn thì không nạp và bị xóa khi server khởi động | Không để bảng phình mãi; 30 ngày đủ cho người nghỉ chơi vài tuần |
| Giao dịch | Đồ và Xu của thưởng giao dịch được như mọi thứ khác (tổng quan mục 3.7) | v1 không có đồ khóa |
| Nhận khi đang giao dịch hay bày sạp | Bị chặn bằng lỗi của 7b như mọi lệnh đổi hành trang | Một luật khóa hành trang của 7b |
| Lệnh dev | `EVENT_DEV_START` (7e) nhận `thao_phat`; thêm `EVENT_DEV_MINUTES` (thời lượng lượt dev); lệnh `cmd/devreward` phát một thưởng cho nhân vật | Thử bảng và hết giờ trong một phút; thử hòm thưởng không cần đánh Boss 31 triệu HP |

### 1.3. Tính HP Boss

Số liệu từ `data/monsters.json` (game-design 5.9): quái thường cấp 80 có 12.466 HP, công 1.344, mỗi loại thủ 640, EXP gốc 11.095. Thủ lĩnh cấp 80 (7d): 249.320 HP, công 2.688, thủ 960, EXP gốc 25 × 11.095 = 277.375.

- HP quái thường = 4 đòn tham chiếu của nhân vật cùng cấp (5.9), nên một đòn tham chiếu cấp 80 gây khoảng 12.466 / 4 = 3.117 lên thủ 640. Với công thức thủ của 2.2, phần giữ lại là 1 − 640 / (640 + 20 × 80) = 0,714; lên thủ 960 là 1 − 960 / 2.560 = 0,625; một đòn tham chiếu lên Boss còn 3.117 × 0,625 / 0,714 ≈ 2.727.
- Nhân vật cấp 50 (đòn tham chiếu 8.408 / 4 = 2.102 lên thủ 400, giữ 0,714): lên thủ 960 giữ 1 − 960 / (960 + 1.000) = 0,510, còn 2.102 × 0,510 / 0,714 ≈ 1.501.
- Giả định mỗi người ra khoảng một đòn tham chiếu mỗi giây (chém xen chiêu hết hồi), người tham gia trải từ cấp 50 tới 80: trung bình khoảng 2.100 sát thương/giây; trừ khoảng 30% thời gian chạy né cảnh báo, chết, đánh nhau với người khác: khoảng 1.470/giây.
- 30 người × 1.470 × 720 giây (12 phút) ≈ 31,8 triệu. Chọn **HP ×125**: 249.320 × 125 = **31.165.000 HP**. 15 người cần khoảng 24 phút, vẫn kịp trong 30 phút; 60 người khoảng 6 phút.
- EXP gốc ×125 (34.671.875) giữ đúng tỷ lệ EXP trên mỗi điểm sát thương của thủ lĩnh cấp 80; người chênh Boss quá 10 cấp (dưới 70) không nhận EXP (cửa sổ ±10) nhưng vẫn nhận thưởng theo hạng.

Số tạm, cân chỉnh sau chơi thử bằng `hp_mul` trong `events.json`.

### 1.4. Bổ sung cho game-design

Game-design 6.2 và 10.2 để mở lịch, điều kiện tham gia, mức thưởng, tính khóa. Spec này chốt: Hoang Sơn một khu, cấp ≥ 50; 13:00 và 18:00, 30 phút; ba Thượng Huyền thay phiên; thưởng theo hạng sát thương, kết liễu, quỹ bang; hết giờ giảm nửa; thưởng giao dịch được; hòm thưởng giữ 30 ngày. "Vật phẩm bang" của 6.2 đổi thành Xu quỹ bang ở v1. Ghi vào game-design mục 6.2 và 5.1 (mục 9).

### 1.5. Không làm ở 7f

Vật phẩm bang; danh hiệu; Thảo Phạt khung thứ ba; Boss ra ngoài đấu trường; tài nguyên quý hay áp tiêu ở map chiến sự; quái thường ở Hoang Sơn; điểm PK; thưởng EXP trong hòm thưởng của 7f (cột có sẵn cho 7h); nhận tất cả bằng một lệnh; thư giữa người chơi; xóa thưởng bằng tay.

## 2. Dữ liệu

### 2.1. Sự kiện trong `data/events.json`

`event_data.py` thêm sau Lệnh Diệt Quỷ (loại mới `world_boss`):

```json
{
  "id": "thao_phat",
  "name": "Thảo Phạt Thượng Huyền",
  "kind": "world_boss",
  "min_level": 50,
  "summary": "Boss thế giới ở Hoang Sơn Chiến Địa, vừa đánh Boss vừa PK",
  "schedule": [
    {"days": [1, 2, 3, 4, 5, 6, 7], "start": "13:00", "minutes": 30},
    {"days": [1, 2, 3, 4, 5, 6, 7], "start": "18:00", "minutes": 30}
  ],
  "world_boss": {
    "map": "hoang_son",
    "spawn": {"x": 32.5, "y": 20.5},
    "arena": {"x": 12, "y": 6, "w": 40, "h": 28},
    "bosses": ["tp_daki", "tp_gyokko", "tp_hantengu"],
    "level": 80,
    "hp_mul": 125,
    "exp_mul": 125,
    "board_players": 10,
    "board_guilds": 5,
    "board_sec": 2,
    "participation_bp": 10,
    "bands": [
      {"from": 1, "to": 1, "xu": 100000, "stacks": [{"code": "stone_enhance", "count": 30}, {"code": "stone_refine", "count": 5}], "equipment": {"tier": 50, "quality": 3}},
      {"from": 2, "to": 3, "xu": 60000, "stacks": [{"code": "stone_enhance", "count": 20}, {"code": "stone_refine", "count": 3}], "equipment": {"tier": 50, "quality": 2}},
      {"from": 4, "to": 10, "xu": 40000, "stacks": [{"code": "stone_enhance", "count": 15}, {"code": "stone_refine", "count": 2}], "equipment": {"tier": 50, "quality": 2}},
      {"from": 11, "to": 30, "xu": 20000, "stacks": [{"code": "stone_enhance", "count": 10}, {"code": "stone_refine", "count": 1}]},
      {"from": 31, "to": 0, "xu": 10000, "stacks": [{"code": "stone_enhance", "count": 5}]}
    ],
    "killer": {"xu": 50000, "stacks": [{"code": "stone_refine", "count": 3}]},
    "guild_fund_xu": 300000,
    "timeout_pct": 50
  }
}
```

- `map` là map `battle` có thật; `spawn` nằm trên ô đi được trong `arena`; `arena` nằm trong map và không chạm khu an toàn.
- `bosses`: 1–7 loại quái có thật, `rank: "boss"`, `event_only: true` (7e mục 2.2).
- `level` 1–90; `hp_mul`, `exp_mul` 1–10.000; `board_players` 1–20; `board_guilds` 1–10; `board_sec` 1–10; `participation_bp` 0–10.000 (phần vạn HP tối đa của Boss).
- `bands`: hạng liên tục từ 1, không chồng; `to` = 0 nghĩa là "tới hết" và chỉ ở dải cuối; `xu` ≥ 0; `stacks` như 7e; `equipment` (không bắt buộc): `tier` là bậc có thật, `quality` 0–4.
- `killer`: như một dải không có hạng. `guild_fund_xu` ≥ 0. `timeout_pct` 0–100.

**Thưởng theo hạng khi Boss bị hạ:**

| Hạng | Xu | Đá cường hóa | Đá tẩy luyện | Trang bị bậc 50 |
| --- | --- | --- | --- | --- |
| 1 | 100.000 | 30 | 5 | 1 món Cam |
| 2–3 | 60.000 | 20 | 3 | 1 món Vàng |
| 4–10 | 40.000 | 15 | 2 | 1 món Vàng |
| 11–30 | 20.000 | 10 | 1 | — |
| Từ 31 (đạt ngưỡng) | 10.000 | 5 | — | — |

Trang bị thưởng: ô ngẫu nhiên đều trong 10 ô; vũ khí theo class người nhận; sinh đủ dòng như đồ rơi (3a mục 4.4) bằng bộ ngẫu nhiên của dịch vụ sự kiện. Hết giờ: Xu và số đá mỗi mục × `timeout_pct` / 100 làm tròn xuống (mục có số lượng về 0 thì bỏ), không trang bị.

### 2.2. Ba Thượng Huyền trong `monsters.json`

`monster_stats.py` thêm (hạng `boss`, `event_only: true`, khai báo chiêu theo định dạng của 7d; cảnh báo 0,8 giây, tối đa 5 người):

| id | Tên | Đòn thường | Chiêu |
| --- | --- | --- | --- |
| `tp_daki` | Thượng Huyền Lục – Daki | vật lý | Quất đai lụa: `line` dài 7, rộng 1,5, 170% công vật lý, hồi 6 giây; Lốc đai lụa: `circle_self` bán kính 4, 150% công vật lý, hồi 10 giây |
| `tp_gyokko` | Thượng Huyền Ngũ – Gyokko | phép | Bình vỡ: `circle_target` tầm 8, bán kính 2,5, 180% công phép, hồi 8 giây; Sóng cá: `cone` 90°, tầm 6, 150% công phép, hồi 10 giây |
| `tp_hantengu` | Thượng Huyền Tứ – Hantengu | phép | Sét giáng: `circle_target` tầm 8, bán kính 3, 170% công phép, hồi 8 giây; Quạt gió: `cone` 120°, tầm 5, 150% công phép, hồi 6 giây; Tiếng gầm: `circle_self` bán kính 5, 130% công phép, hồi 12 giây |

Chỉ số lấy từ bảng `levels` × hệ số thủ lĩnh như mọi thủ lĩnh; `hp_mul`/`exp_mul` của sự kiện áp lúc sinh, không ghi vào `monsters.json`.

### 2.3. Map `hoang_son` và các trường map mới

`maps/<id>.json` thêm ba trường không bắt buộc:

- `max_zones`: số khu tối đa, 1–10, ≥ `min_zones`; không có nghĩa là không giới hạn (như hiện tại).
- `capacity`: sức chứa mỗi khu, 1–500; không có thì dùng `ZONE_CAPACITY`.
- `min_level`: cấp tối thiểu để vào, 1–90; không có là 1. Map `town` và map có cổng tới từ `START_MAP` không được có `min_level` > 1 (kiểm tra cả đồ thị trong script và test).

`tools/mapgen.py`:

| Thành phần | Giá trị |
| --- | --- |
| `hoang_son` | `type: battle`, 64 × 48, `min_zones: 1`, `max_zones: 1`, `capacity: 150`, `min_level: 50`, `town: "ubuyashiki"`, không có vùng quái |
| Trại `camp` | khu an toàn (24, 37, 16, 10) ở mép nam |
| Cổng | `to_ubuyashiki` (30, 46, 4, 1) ở hàng `height − 2`; điểm đến `from_ubuyashiki` (32,0; 43,5) |
| Điểm hồi sinh | `default` (32,0; 40,5) `respawn: true` |
| Lửa trại, NPC | lửa trại (28,5; 40,5) bán kính 1,5; `duoc_su` "Dược sư" vai `shop` → `apothecary` ở (36,5; 40,5) |
| Đấu trường | trống từ (12, 6) tới (52, 34), vài khối đá trang trí ngoài đấu trường |
| `ubuyashiki` | thêm cổng bắc `to_hoang_son` (18, 1, 4, 1) ở hàng 1 và điểm đến `from_hoang_son` (20,0; 4,5) |

Luật bố cục 4a nay cho phép cổng ở hàng 1 (phía bắc) hoặc hàng `height − 2` (phía nam), rộng 4 ô, cao 1 ô, ở 4 cột giữa map; điểm đến cách hàng cổng 3 ô vào trong. Script kiểm tra cổng hai chiều và liên thông như cũ.

## 3. Lưu trữ

Migration `00012_rewards.sql`:

| Cột | Kiểu | Ghi chú |
| --- | --- | --- |
| `id` | `bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY` | |
| `character_id` | `bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE` | |
| `source` | `text NOT NULL` | khóa chống phát trùng |
| `title` | `text NOT NULL` | dòng hiện trong hòm thưởng |
| `xu` | `bigint NOT NULL DEFAULT 0 CHECK (xu >= 0)` | |
| `exp` | `bigint NOT NULL DEFAULT 0 CHECK (exp >= 0)` | 7f luôn 0 |
| `items` | `jsonb NOT NULL DEFAULT '[]'` | mảng món đồ, mỗi món đúng các trường của một dòng `character_items` (`code`, `count`, `quality`, `element`, `enhance`, `lines`, `hidden`, `pity`, `gems`) |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | |
| `claimed_at` | `timestamptz` | NULL: chưa nhận |

`UNIQUE (character_id, source)`; chỉ mục `(character_id) WHERE claimed_at IS NULL`. Down xóa bảng.

**Store:**

- `InsertReward(ctx, RewardRow) (id int64, inserted bool, err error)`: `INSERT … ON CONFLICT (character_id, source) DO NOTHING RETURNING id`; trùng thì `inserted` sai.
- `PendingRewards(ctx, charID, since time.Time) ([]RewardRow, error)`: các dòng chưa nhận có `created_at ≥ since`, theo `id` tăng dần.
- `PruneRewards(ctx, before time.Time) (int64, error)`: xóa dòng đã nhận trước `before` và dòng chưa nhận tạo trước `before`; server gọi một lần lúc khởi động với `now − 30 ngày`.
- Phép ghi kèm mới `store.ClaimRewards{CharID int64; IDs []int64}` (tổng quan mục 3.2): `UPDATE character_rewards SET claimed_at = now() WHERE character_id = $1 AND id = ANY($2) AND claimed_at IS NULL`. Số dòng đổi ít hơn số id thì ghi log cảnh báo, **không** hủy giao dịch (chỉ xảy ra nếu có hai phiên cùng nhân vật, điều mà `Takeover` đã chặn).
- Lệnh dev: `DevGrantReward(ctx, charID, RewardRow)`.

**Khi vào game** (`world.Enter`): món sai trong `items` (luật kiểm tra món của 3a mục 3, `pity` của 3b, `gems` của 3c, không xét ô) bị bỏ khỏi thưởng đó và ghi log; thưởng không còn Xu, EXP, đồ nào thì vẫn giữ (nhận được, không cho gì) để đánh dấu xong.

## 4. Server

### 4.1. Map một khu và cấp vào

- `World.reserve(mapID)`: khu đầu tiên còn chỗ theo `capacity` của map; nếu không còn và số khu đang mở đã bằng `max_zones` thì báo đầy (không mở khu mới). `closeIdle` không đổi.
- `World.Transfer` bước 2: map đích đầy hết → `ZONE_FULL` (lỗi của world, trước mọi kiểm tra của zone, như thứ tự 4a).
- Hàm kiểm tra của mọi loại chuyển zone (cổng, giấy về thành, xa phu, đổi khu; 7g thêm loại của mình) thêm bước **cuối cùng**: cấp nhân vật < `min_level` của map đích → `LEVEL_TOO_LOW`. Cổng: `BAD_REQUEST` → `ZONE_FULL` → `DEAD` → `OUT_OF_RANGE` → `LEVEL_TOO_LOW`.
- `World.Enter`: vị trí đã lưu ở map có `min_level` lớn hơn cấp, hoặc map đó đầy hết, thì nhân vật vào ở điểm `default` của map `town` của map đó.
- `ZoneListResp` của Hoang Sơn có đúng một khu; `ChangeZoneReq` sang khu khác bị `ZONE_NOT_FOUND` như cũ.

### 4.2. Lượt Thảo Phạt trong zone

Phần xử lý `world_boss` (khung zone của 7e mục 4.3) chỉ chạy ở zone của `world_boss.map`.

**Bắt đầu** (run r): Boss = `bosses[quest.Day(r.Start) mod len(bosses)]`. Sinh bằng hàm sinh quái đặc biệt của 7d: loại đó, `level`, vị trí `spawn`, vùng nhà `arena`, không hồi sinh, hệ số HP `hp_mul` và hệ số EXP gốc `exp_mul` (`HPMul`, `ExpMul` của 7d), thẻ lượt, móc khi chết. Ghi `bossRun{run, bossID, board, done: false}`. Người trong zone nhận `Scoreboard` rỗng ngay.

**Ghi sát thương:** trong `applyToMonster`, sau khi tính sát thương tính EXP `c` (đã kẹp) của một đòn người chơi lên Boss của lượt, nếu người ra đòn còn trong zone: `board.add(charID, name, class, guild, c, tick)`; `guild` là id và tên bang của người đó lúc này (ô bang của 7c; 0 nếu không có bang). Mỗi dòng giữ tổng sát thương và tick của lần cộng cuối; mỗi bang giữ tổng và tick lần cộng cuối.

**Xếp hạng:** `board.ranked()`: sát thương giảm dần, rồi tick lần cộng cuối tăng dần, rồi id nhân vật tăng dần. Bang xếp cùng luật (id bang thay id nhân vật).

**Đẩy bảng:** mỗi `board_sec` giây khi lượt chạy và chưa xong, mỗi người chơi trong zone nhận `Scoreboard` (mục 5.2): `board_players` dòng đầu, dòng của chính họ (hạng 0 nếu chưa gây sát thương), `board_guilds` bang đầu, `ends_in_ms` = r.End − now.

**Boss chết** (móc của 7d, sau `loot` của thủ lĩnh cho người kết liễu): `done = true`; `killer` = id nhân vật người kết liễu nếu còn trong zone, nếu không 0; gửi `Scoreboard` cuối (`final: true`, `success: true`, `killer_name`); gọi `zone.Config.EventResult(Result{…, Killed: true})`.

**Hết giờ** (`end` của 7e khi chưa `done`): Boss bị xóa khỏi zone như hết thời gian xác (`EntityLeave`, không rơi gì); `done = true`; gửi `Scoreboard` cuối (`final: true`, `success: false`); gọi `EventResult(Result{…, Killed: false})`.

Mỗi lượt gọi `EventResult` đúng một lần. Lượt bị bỏ khi zone dừng (tắt server) thì không có kết quả, không thưởng.

```go
type ResultRow struct {
    CharID   int64
    Name     string
    Class    pb.CharClass
    GuildID  uint64
    GuildName string
    Points   int64 // counted damage
}
type Result struct {
    EventID  string
    RunID    int64
    BossName string
    Killed   bool
    KillerID int64          // 0: none
    KillerName string
    MaxHP    int64
    Players  []ResultRow    // ranked
    Guilds   []ResultRow    // ranked; CharID 0, Name empty
}
```

`zone.Config.EventResult func(event.Result)` chỉ nối kết quả vào hàng đợi của dịch vụ (khóa ngắn, không I/O).

### 4.3. Phát thưởng (dịch vụ sự kiện)

Goroutine của dịch vụ sự kiện, ở lần xét kế tiếp sau khi nhận kết quả:

1. Đánh dấu lượt đã xong: lượt đó không còn trong ảnh chụp, trạng thái sự kiện không còn `RUNNING` (kết thúc sớm, mục 1.2), báo Hệ thống: Boss bị hạ "<Boss> đã bị hạ! Người kết liễu: <tên>. Bang gây sát thương nhiều nhất: <bang>"; hết giờ "<Boss> đã rời Hoang Sơn". Khung lịch đó không được kích hoạt lại.
2. Ngưỡng = ⌈`MaxHP` × `participation_bp` / 10.000⌉. Duyệt `Players` theo hạng; người có `Points` < ngưỡng không nhận gì. Người đạt ngưỡng ở hạng h nhận dải chứa h. Nếu không `Killed`: áp `timeout_pct`, bỏ trang bị.
3. Mỗi người nhận một thưởng `source = "thao_phat:<run_id>:rank"`, `title = "Thảo Phạt <Boss> — hạng h"`; trang bị sinh bằng `items.NewEquipment` và sinh dòng của 3a với bộ ngẫu nhiên của dịch vụ.
4. `Killed` và `KillerID` ≠ 0: thưởng `source = "thao_phat:<run_id>:killer"`, `title = "Thảo Phạt <Boss> — kết liễu"`.
5. `Killed`, có ít nhất một bang trong `Guilds` và `guild_fund_xu` > 0: `guild.AddFund(Guilds[0].GuildID, guild_fund_xu)` (7c; chạy trên goroutine sự kiện, không phải zone).
6. Mỗi thưởng gọi `reward.Service.Grant` (không chặn).

### 4.4. Dịch vụ hòm thưởng

Gói `internal/reward`, mức world (tổng quan mục 3.1):

- `Register(charID) *Inbox`, `Unregister(charID)`. `Inbox` là hàng đợi có khóa (`Push`, `Drain`), một phiên một hộp, đi theo `PlayerInfo` qua bản trao tay.
- `Grant(Grant{CharID, Source, Title, Xu, Exp, Items})` nối vào hàng đợi trong bộ nhớ, không chặn.
- Goroutine ghi riêng: lấy từng mục, `store.InsertReward`; lỗi DB thì thử lại sau 1, 2, 4… tối đa 30 giây (như saver); chèn trùng thì bỏ; chèn được thì nếu nhân vật đang đăng ký, `Inbox.Push(Reward{ID, …})`.
- Tắt server: ghi hết hàng đợi dưới hạn giờ riêng (10 giây), như saver (CLAUDE.md).
- **Thứ tự khi vào game:** `World.Enter` gọi `Register` **trước** `store.PendingRewards`; thưởng chèn sau lúc đó vừa được nạp vừa vào hộp, zone bỏ id đã có. Không thưởng nào bị sót.

### 4.5. Hòm thưởng trong zone

- `PlayerInfo.Rewards []Reward` (đã nạp, đã lọc) và `PlayerInfo.RewardInbox *reward.Inbox`; bản trao tay sao chép danh sách và giữ con trỏ hộp.
- Mỗi giây zone `Drain` hộp của từng người chơi; id chưa có thì thêm vào cuối danh sách, đẩy `RewardAdded`, `statsDirty`.
- `SelfStats.rewards` = số thưởng chưa nhận. `EnterWorldResp.rewards` = cả danh sách.

**`RewardClaimReq{reward_id}`**, xử lý trong `drain`, lỗi đầu tiên được trả:

1. Nhân vật chết: `DEAD`.
2. Đang giao dịch hoặc đang bày sạp: lỗi tương ứng của 7b.
3. Không có thưởng chưa nhận với id đó: `REWARD_NOT_FOUND`.
4. Hành trang (bản sao) không chứa hết mọi món của thưởng (luật 3a mục 4.1): `BAG_FULL`.

Hợp lệ: thêm các món vào hành trang, cộng Xu, `gainExp(exp)` (lên cấp như mọi nguồn EXP), bỏ thưởng khỏi danh sách, lưu ngay với phép `ClaimRewards{CharID, [reward_id]}`; trả `RewardClaimResp{reward_id, update, stats, xu, exp}`. Thưởng hết hạn trong lúc đang chơi vẫn nhận được tới khi thoát (hạn chỉ xét lúc nạp).

### 4.6. Gateway

`RewardClaimReq` cần đã vào thế giới (`NOT_IN_WORLD`), chỉ chịu giới hạn gói chung, đi vào zone như các lệnh đồ (hộp thư đầy → `RATE_LIMITED`).

## 5. Giao thức

`ProtocolVersion` lên **21**.

### 5.1. Envelope

| Envelope | Message | Trường |
| --- | --- | --- |
| 200 | `RewardClaimReq` | `int64 reward_id = 1` |
| 201 | `RewardClaimResp` | `int64 reward_id = 1`, `InventoryUpdate update = 2`, `SelfStats stats = 3`, `int64 xu = 4`, `int64 exp = 5` |
| 202 | `RewardAdded` | `RewardInfo reward = 1`; đẩy với `seq = 0` |
| 203 | `Scoreboard` | đẩy với `seq = 0` (mục 5.2) |

### 5.2. Message

```proto
// One unclaimed reward in the mailbox.
message RewardInfo {
  int64 reward_id = 1;
  string title = 2;
  int64 xu = 3;
  int64 exp = 4;
  repeated ItemInfo items = 5;
  int64 expires_in_ms = 6; // from when the message was sent
}

message RewardBox {
  repeated RewardInfo rewards = 1;
}

// One row of an event scoreboard: a player (name set) or a guild (name empty).
message ScoreRow {
  int32 rank = 1;          // 0: not ranked
  string name = 2;
  string guild_name = 3;
  int64 points = 4;
}

// A live or final event scoreboard. Shared by every event with points (7f, 7h).
message Scoreboard {
  string event_id = 1;
  int64 run_id = 2;
  repeated ScoreRow players = 3;
  ScoreRow self = 4;
  repeated ScoreRow guilds = 5;
  int64 ends_in_ms = 6;
  bool final = 7;          // the run is over; no more boards for it
  bool success = 8;        // final only: the goal was reached (boss killed)
  string killer_name = 9;  // final only, may be empty
}
```

- `EnterWorldResp.rewards = 18` (`RewardBox`). `SelfStats.rewards = 26` (`int32`, số thưởng chưa nhận).
- `ErrorCode`: `REWARD_NOT_FOUND = 94` (no unclaimed reward with that id). Dải 95–98 để trống. Ghi chú của `LEVEL_TOO_LOW` thêm "or the destination map's min_level".
- Fixture mới: `scoreboard.hex` (ba người, hai bang, dòng của mình hạng 7), `scoreboard_final.hex` (`final`, `success`, `killer_name`), `reward_added.hex` (Xu, 30 đá, một món Cam có dòng), `reward_claim_resp.hex`, `enter_world_resp_rewards.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/reward_box.gd`** (mới): `apply_box(RewardBox, now)`, `add(RewardInfo, now)`, `remove(id)`, `count()`, `rows()` (theo thứ tự nhận), `expires_text(id, now)` ("Còn 29 ngày", "Còn 5 giờ"), `fits(id, inventory) -> bool` (dùng luật thêm món của `core/inventory.gd` trên bản sao).
- **`core/scoreboard.gd`** (mới): `apply(Scoreboard, now)`; `visible` khi có bảng chưa `final` hoặc bảng cuối trong 30 giây; `rows`, `self_row`, `guild_rows`, `ends_text(now)`; `result_text()` ("Daki đã bị hạ — người kết liễu Kaito" / "Hết giờ — Boss đã rời đi").
- **`core/world_graph.gd`**: map có `min_level` lớn hơn cấp nhân vật bị bỏ khỏi `reachable` và đường đi; **`core/portal_watch.gd`** (4a) không gửi `UsePortalReq` vào cổng tới map đó mà báo một lần "Cần cấp 50 để vào Hoang Sơn Chiến Địa".
- **`core/wire.gd`**: payload `reward_claim_resp`, `reward_added`, `scoreboard`; **`core/errors.gd`**: `REWARD_NOT_FOUND` "Thưởng không còn". `Router.rewards`, `Router.scoreboard`.

### 6.2. Giao diện

- **Nút Hòm thưởng** (`ui/hud.gd`): biểu tượng hộp quà ở cụm nút trên, huy hiệu đỏ là `SelfStats.rewards`; ẩn huy hiệu khi 0.
- **Cửa sổ Hòm thưởng** (`ui/reward_window.gd`, mới): mỗi thưởng một thẻ: tiêu đề, Xu, các món (chạm xem chi tiết như hành trang), hạn; nút **Nhận** (gửi `RewardClaimReq`; khóa và ghi "Hành trang không đủ chỗ" khi `fits` sai); nút **Nhận tất cả** gửi lần lượt từng thưởng, dừng ở lỗi đầu tiên. Nhận xong hiện "+N Xu" và tên món.
- **Bảng sát thương** (`ui/scoreboard_panel.gd`, mới): bên phải, dưới khung theo dõi nhiệm vụ, khi `scoreboard.visible`; tab **Cá nhân** (hạng, tên, bang, sát thương rút gọn "1,2 tr") có dòng của mình in đậm ở cuối nếu ngoài top; tab **Bang**; đếm ngược; thu gọn được. Bảng cuối hiện `result_text` và "Thưởng đã gửi vào Hòm thưởng".
- **Bản đồ** (4a/6a): Hoang Sơn ghi "Cấp ≥ 50" và "Một khu"; dải sự kiện (7e) hiện Thảo Phạt.

### 6.3. Smoke

`tools/smoke.sh`: trước khi vào lại, `devreward -char <tên> -xu 500 -stack stone_enhance:3 -title "Smoke"`. `tests/smoke/smoke.gd`: `EnterWorldResp.rewards` có thưởng đó, `SelfStats.rewards` ≥ 1; gửi `RewardClaimReq`; `RewardClaimResp.xu` 500 và hành trang thêm 3 đá; gửi lại cùng id bị `REWARD_NOT_FOUND`. In "SMOKE OK" kèm "reward claimed".

## 7. Lệnh dev và cấu hình

- `EVENT_DEV_START=thao_phat` chạy một lượt dev (7e mục 7). `EVENT_DEV_MINUTES` (mặc định 0 = thời lượng của sự kiện): thời lượng mọi lượt dev, 1–720.
- `cmd/devreward` (nhân vật đang thoát game, như `devitem`): `-char <tên>`, `-title <chữ>` (mặc định "Thưởng thử"), `-xu n`, `-exp n`, `-stack code:n` (lặp được), `-equip code:quality` (lặp được, sinh dòng như đồ rơi); `source` là `dev:<giây Unix>:<số ngẫu nhiên>`. Mã lạ thì báo lỗi, không ghi gì.

## 8. Kiểm thử

**Script:** `event_data.py --check` và `mapgen.py --check` sạch; `event_data.py` in HP, công, thủ, EXP gốc của ba Boss sau hệ số (31.165.000 HP) và bảng thưởng.

**Server** (`go test -race`):

- `mapdata`: `max_zones` < `min_zones`, `capacity` 0 hay 501, `min_level` 0 hay 91 bị từ chối; map thật `hoang_son` hợp lệ, có đúng một khu an toàn chứa điểm hồi sinh; đồ thị thật: map `town` và map nối với `START_MAP` có `min_level` 1; cổng bắc/nam đúng luật bố cục.
- `gamedata`: dữ liệu thật nạp được (hai sự kiện, ba Boss `event_only`); từ chối `world_boss` với map không phải `battle`, `spawn` ngoài `arena`, `arena` chạm khu an toàn, Boss không phải hạng `boss`, Boss thiếu `event_only`, dải hạng hở hay chồng, `to` = 0 không ở dải cuối, `tier` lạ, `quality` 5.
- `world`: map một khu đầy thì `Transfer` bị `ZONE_FULL` và không mở khu 2; cổng tới Hoang Sơn ở cấp 49 bị `LEVEL_TOO_LOW`, cấp 50 qua được; vào game ở Hoang Sơn với cấp 40 thì về `default` của Ubuyashiki; `Enter` gọi `reward.Register` trước khi nạp thưởng (thưởng chèn giữa hai bước có trong danh sách đúng một lần).
- `zone` Thảo Phạt (ảnh chụp giả, đồng hồ giả, HP Boss nhỏ bằng `hp_mul` 1 trong dữ liệu test): lượt bắt đầu sinh đúng Boss theo ngày ở `spawn` với HP = HP thủ lĩnh × `hp_mul`; zone khác map không sinh; đòn lên Boss cộng đúng sát thương đã kẹp vào bảng, đòn thừa lúc Boss còn 10 HP chỉ cộng 10; người không bang không có trong bảng bang; hai người cùng sát thương thì người cộng trước xếp trên; `Scoreboard` mỗi 2 giây với top 10, dòng của mình và top 5 bang; Boss chết: bảng cuối `success`, `killer_name`, `EventResult` một lần với `Killed`; hết giờ: Boss rời zone, bảng cuối không `success`, `EventResult` một lần; Boss bị chết đúng tick kết thúc chỉ có một kết quả; Boss không ra khỏi đấu trường khi mục tiêu chạy vào trại; PvP hai người khác bang được, cùng bang bị `PK_FORBIDDEN`, chết ở Hoang Sơn không mất EXP.
- `event` phát thưởng (reward giả, guild giả): bảng 35 người có 3 người dưới ngưỡng thì 32 thưởng hạng đúng dải; hạng 1 có trang bị Cam bậc 50 đúng class, hạng 2–3 và 4–10 Vàng; kết liễu có thưởng riêng; bang đầu nhận `AddFund(…, 300000)`; hết giờ: Xu, đá giảm nửa, không trang bị, không kết liễu, không quỹ; kết quả làm lượt kết thúc sớm (ảnh chụp không còn lượt, tin Hệ thống), khung đó không chạy lại trong cùng giờ; bảng rỗng không phát gì.
- `reward` (store giả): `Grant` không chặn; chèn trùng `source` không đẩy vào hộp; nhân vật đang đăng ký nhận `Push`; DB lỗi hai lần rồi thành công thì vẫn chèn một lần; dừng thì ghi hết hàng đợi.
- `store` (DB): migration lên rồi xuống; chèn, chèn trùng (`inserted` sai), `PendingRewards` lọc đã nhận và quá hạn; `PruneRewards`; lưu nhân vật kèm `ClaimRewards` đánh dấu đúng dòng trong cùng giao dịch; giao dịch lỗi (ép lỗi ở phần nhân vật) không đánh dấu; xóa nhân vật xóa thưởng.
- `zone` hòm thưởng: `Drain` thêm thưởng mới và bỏ id trùng, đẩy `RewardAdded`, `SelfStats.rewards` tăng; nhận: chết `DEAD`, đang giao dịch bị lỗi của 7b, id lạ `REWARD_NOT_FOUND`, túi thiếu chỗ `BAG_FULL` không đổi gì; nhận được: đồ, Xu, EXP (lên cấp có `LevelUp`), lần lưu mang phép `ClaimRewards` đúng id; nhận lại `REWARD_NOT_FOUND`; danh sách đi qua bản trao tay.
- `gateway`: `ProtocolVersion` 21; fixture mới; `RewardClaimReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- `cmd/devreward`: ghi đúng một dòng; mã lạ không ghi gì.
- e2e `TestRewardMailboxOverWebSocket`: `DevGrantReward` cho nhân vật đang thoát (300 Xu, 2 đá); vào game thấy thưởng trong `EnterWorldResp.rewards`; trong lúc online, store chèn thêm một thưởng qua `reward.Service.Grant` (test gọi thẳng dịch vụ của server thử) và client nhận `RewardAdded`; nhận cả hai; thoát, vào lại, hòm rỗng.
- e2e `TestWorldBossOverWebSocket`: server thử với lượt dev `thao_phat` 1 phút, bắt đầu sau 1 giây; hai nhân vật cấp 80 (`DevSetLevel`) ở hai bang khác nhau đặt ở trại Hoang Sơn; nhận `Scoreboard` trong 3 giây; một người chém Boss và thấy mình hạng 1 với đúng sát thương trong `Hit`; hết giờ: `Scoreboard` cuối không `success`; không ai đạt ngưỡng nên hòm thưởng không thêm gì; nhân vật cấp 49 qua cổng bị `LEVEL_TOO_LOW`.

**Client** (`tests/run.sh`, mục `# ____TEST REWARDS____`, `# ____TEST SCOREBOARD____`): `reward_box` (áp hòm, thêm, bỏ, đếm, `fits` với túi đầy, `expires_text`); `scoreboard` (bảng sống, dòng của mình ngoài top, bảng cuối, ẩn sau 30 giây); `world_graph` bỏ Hoang Sơn khỏi `reachable` ở cấp 49; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199` khởi động với `EVENT_DEV_START=thao_phat` và `EVENT_DEV_MINUTES=5`. `test1` (Kaito) và `test2` (Hana) được `devchar -level 80` và ở hai bang khác nhau (7c). Hai người đi qua cổng bắc Tổng hành dinh tới Hoang Sơn; Boss xuất hiện giữa đấu trường; né vùng cảnh báo; bảng sát thương cập nhật; Hana bật PK đánh Kaito (không mất EXP khi chết); hết 5 phút Boss rời đi; cả hai thấy bảng cuối và thưởng hết giờ (nếu đạt ngưỡng) trong Hòm thưởng, nhận được. Thử thêm `devreward` cho `test1` và nhận ở Hòm thưởng.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 6.2 (Thảo Phạt v1: Hoang Sơn, cấp, giờ, Boss, bảng, thưởng, hết giờ, quỹ bang thay vật phẩm bang), mục 5.1 (map chiến sự đầu tiên một khu, cấp vào), mục 10.2 (bỏ phần đã chốt cho Thảo Phạt).
- Spec 4a mục 2.1–2.3: trường `max_zones`, `capacity`, `min_level`; luật cổng bắc/nam.
- `CLAUDE.md`: thưởng ngoài zone đi qua `internal/reward` và hòm thưởng; nhận thưởng lưu kèm phép `ClaimRewards`; dịch vụ thưởng ghi hết hàng đợi khi tắt server dưới hạn giờ riêng.
- README hai repo: Hoang Sơn, Thảo Phạt, Hòm thưởng, `EVENT_DEV_MINUTES`, `devreward`.
- Tổng quan Giai đoạn 7: đánh dấu 7f đã triển khai khi xong.
