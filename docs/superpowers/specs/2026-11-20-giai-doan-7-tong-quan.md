# Giai đoạn 7 — Xã hội: tổng quan và chia phần

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Lộ trình ([spec Giai đoạn 0](2026-11-18-giai-doan-0-nen-mong-server-design.md) mục 1.1) ghi Giai đoạn 7+ là "Giao dịch, bang hội, sự kiện, Vô Hạn Thành". Luật chơi lấy từ [game-design.md](../../game-design.md) mục 4.1, 4.2, 5.1, 5.6, 5.10, 5.13, 6, 7 và 10; game-design là chuẩn, chỗ nào game-design để mở hoặc "chưa chốt" thì spec từng phần chốt. Giai đoạn 7 chạy sau khi Giai đoạn 6 đã triển khai (giao thức **15**, migration **00009**).

Tài liệu này chia Giai đoạn 7 thành tám phần, cấp số hiệu giao thức cho từng phần và mô tả các hạ tầng dùng chung. Thiết kế chi tiết nằm trong spec của từng phần.

## 1. Các phần, thứ tự và phụ thuộc

Làm theo thứ tự; mỗi phần xong là chơi được.

| Phần | Nội dung | Kết quả chơi được | Cần trước |
| --- | --- | --- | --- |
| **7a** [Chat](2026-11-20-giai-doan-7a-chat-design.md) | Kênh Thế giới, Khu, Tổ đội, Riêng (theo tên), Hệ thống; giới hạn tần suất; khung chat trên HUD; gói làm sạch chữ dùng chung `internal/textutil` | Nói chuyện với mọi người, với tổ đội, nhắn riêng | 5b |
| **7b** [Giao dịch](2026-11-20-giai-doan-7b-giao-dich-design.md) | Giao dịch trực tiếp 1–1 hai bước (khóa, xác nhận); sạp bán ở thành; lưu nhóm nguyên tử và phép ghi kèm của saver; nhật ký giao dịch | Đổi đồ và Xu với người khác, bày sạp bán đồ ở Tổng hành dinh | 7a (dòng Hệ thống ở client, `textutil`), 4c |
| **7c** [Bang hội](2026-11-20-giai-doan-7c-bang-hoi-design.md) | Lập bang, mời, rời, mời ra, ba chức vụ, nhường bang chủ, giải tán, thông báo bang, quỹ bang (góp Xu, cống hiến), cấp bang; kênh chat Bang; tên bang trên đầu; cùng bang không đánh nhau | Lập và quản lý bang, chat bang, góp quỹ nâng cấp bang | 7a, 7b (phép ghi kèm) |
| **7d** [Tinh anh và thủ lĩnh](2026-11-20-giai-doan-7d-tinh-anh-thu-linh-design.md) | Hạng quái tinh anh/thủ lĩnh, kỹ năng quái có cảnh báo vùng, nội tại (hồi máu một lần, nổi giận), điểm thủ lĩnh trong map, hồi sinh 5/10 phút; bốn con của map 1–20; hàm sinh quái đặc biệt cho sự kiện | Săn Quỷ lang thang đầu đàn, Quỷ Đền, Quỷ săn đêm, Quỷ Rừng tinh anh | 6 (không phụ thuộc 7a–7c) |
| **7e** [Khung sự kiện và Lệnh Diệt Quỷ](2026-11-20-giai-doan-7e-khung-su-kien-lenh-diet-quy-design.md) | Lịch sự kiện theo giờ Việt Nam, báo trước trên kênh Hệ thống, ảnh chụp trạng thái sự kiện cho zone, cửa sổ Lịch sự kiện; sự kiện Quạ Truyền Tin – Lệnh Diệt Quỷ ở mọi map luyện cấp | Ba khung mỗi ngày săn nhóm quỷ truy nã và quỷ đầu đàn | 7a, 7d |
| **7f** [Thảo Phạt Thượng Huyền](2026-11-20-giai-doan-7f-thao-phat-thuong-huyen-design.md) | Map chiến sự một khu; Boss thế giới Thượng Huyền theo lịch; bảng xếp hạng sát thương cá nhân và bang; hòm thưởng (`character_rewards`) | Hai khung mỗi ngày đánh Boss thế giới vừa PK, nhận thưởng theo hạng ở hòm thưởng | 7c, 7d, 7e |
| **7g** [Chuyến Tàu Vô Tận](2026-11-20-giai-doan-7g-chuyen-tau-vo-tan-design.md) | Phó bản riêng cho một tổ đội (instance zone); các toa với đợt quái, Enmu sự kiện; 15 phút; hiệu ứng ngủ; thưởng hoàn thành | Tổ đội tối đa 5 người vượt tàu và hạ Enmu | 5b, 7d, 7e, 7f (hòm thưởng khi đầy túi) |
| **7h** [Công Phá Vô Hạn Thành](2026-11-20-giai-doan-7h-cong-pha-vo-han-thanh-design.md) | Chuỗi map mở dần, Hạ Huyền/Thượng Huyền một lần mỗi kỳ, Quỷ Vương; điểm cá nhân và bang; chiếm thành; thưởng tham gia, mốc, hạng; thành chủ đặt thuế sạp vào quỹ bang | Sự kiện bang cuối tuần, bang Top 1 chiếm thành và thu thuế | 7b, 7c, 7d, 7e, 7f |

Lý do thứ tự: chat là kênh mọi phần sau dùng để báo tin; giao dịch đặt ra cách lưu nguyên tử nhiều nhân vật mà bang (góp quỹ), hòm thưởng (nhận thưởng) và thuế thành dùng lại; bang có trước mọi sự kiện có điểm bang; tinh anh/thủ lĩnh dựng AI và kỹ năng quái mà mọi Boss sự kiện dùng; khung sự kiện có trước các sự kiện cụ thể; Vô Hạn Thành để cuối vì dùng gần như mọi thứ trước nó.

So với gợi ý ban đầu (7f Vô Hạn Thành, 7g Chuyến Tàu): Thảo Phạt Thượng Huyền được tách thành 7f vì nó dựng map chiến sự, bảng xếp hạng và hòm thưởng mà Vô Hạn Thành cần; Chuyến Tàu thành 7g; Vô Hạn Thành thành 7h.

## 2. Số hiệu giao thức, mã lỗi và migration

Mỗi phần tăng `ProtocolVersion` một bậc và dùng **dải số riêng** để các spec viết được song song; số trong dải không dùng thì để trống (protobuf cho phép).

| Phần | `ProtocolVersion` | Envelope | `ErrorCode` | Migration |
| --- | --- | --- | --- | --- |
| 7a | 16 | 125–134 | 57–60 | không |
| 7b | 17 | 135–159 | 61–72 | `00010_trades.sql` |
| 7c | 18 | 160–184 | 73–88 | `00011_guilds.sql` |
| 7d | 19 | 185–189 | 89–90 | không |
| 7e | 20 | 190–199 | 91–93 | không |
| 7f | 21 | 200–214 | 94–98 | `00012_rewards.sql` |
| 7g | 22 | 215–224 | 99–104 | không |
| 7h | 23 | 225–244 | 105–112 | `00013_castle.sql` |

Trường mới trong các message đã có (số cuối hiện tại: `EntityEnter` 15, `LookChange` 3, `EnterWorldResp` 14, `SelfStats` 25, `WorldTick` 9, `Cast` 9, `Hit` 6):

| Message | 7b | 7c | 7d | 7e | 7f | 7g | 7h |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `EntityEnter` | 16 `stall_title` | 17 `guild_name` | 18 `monster_rank` | | | | 19 `castle_owner` |
| `LookChange` | 4 `stall_title` | 5 `guild_name` | | | | | 6 `castle_owner` |
| `EnterWorldResp` | | 15–16 | | 17 | 18 | 19 | 20–21 |
| `SelfStats` | | | | | 26 | | |
| `WorldTick` | | | | | | 10 | |
| `Cast` | | | 10 `area` | | | | |
| `Hit` | | | | | | 7 | |

Số thực dùng (spec từng phần là chuẩn): 7a envelope 125–127, lỗi 57–59; 7b 135–158, 61–70; 7c 160–184, 73–85; 7d không có envelope, lỗi; 7e 190–193, không lỗi mới; 7f 200–203, lỗi 94; 7g 215–221, 99–103; 7h 225–234, 105–109.

Enum mới: 7a tạo `ChatChannel` (`WORLD = 1`, `ZONE = 2`, `PARTY = 3`, `PRIVATE = 4`, `SYSTEM = 5`); 7c thêm `CHAT_CHANNEL_GUILD = 6`. 7d tạo `MonsterRank` (`NORMAL = 1`, `ELITE = 2`, `BOSS = 3`); Boss sự kiện (7f–7h) dùng `BOSS`. 7d thêm `AreaShape` (hình vùng cảnh báo trong `Cast.area`); 7g thêm `StatusKind` (hiệu ứng ngủ). Tên theo ràng buộc godobuf (không trường `message`, `class`; không message `Error`).

## 3. Hạ tầng dùng chung

### 3.1. Dịch vụ mức world

Chat (7a), bang (7c), sự kiện (7e), hòm thưởng (7f), Vô Hạn Thành (7h) là dịch vụ mức world theo khuôn của tổ đội (spec 5b mục 4.1):

- Trạng thái trong bộ nhớ dưới một khóa; đẩy tin thẳng vào hộp gửi của phiên (an toàn khi nhiều goroutine cùng gửi, không chặn).
- Dịch vụ **không bao giờ gọi vào world hay zone** và không gửi lệnh vào hộp thư zone.
- Zone chỉ đọc **ô nguyên tử** của phiên (như ô id tổ đội) hoặc **ảnh chụp nguyên tử** của dịch vụ (`atomic.Pointer` tới dữ liệu không đổi), và được gọi những hàm của dịch vụ chỉ giữ khóa trong thời gian ngắn, **không I/O**.
- Thao tác cần ghi DB của dịch vụ (lập bang, vào bang, phát thưởng) chạy trên goroutine của kết nối hoặc goroutine riêng của dịch vụ, không bao giờ trên goroutine zone.
- `World.Enter` đăng ký phiên với từng dịch vụ cần biết người chơi online (tổ đội, chat, bang, hòm thưởng); `Membership.Leave` hủy đăng ký; `World.Transfer` thành công báo map và khu mới (`Moved`) như 5b.

### 3.2. Lưu nhóm và phép ghi kèm (7b dựng, các phần sau dùng)

Zone sở hữu trạng thái người chơi; khi một việc đổi tài sản của **hai nhân vật** (giao dịch, mua ở sạp) hoặc đổi tài sản của nhân vật **cùng với** một bảng khác (quỹ bang, hòm thưởng, nhật ký), các thay đổi phải nằm trong **một giao dịch DB**. Saver hiện ghi mỗi nhân vật một giao dịch, theo thứ tự bất kỳ, nên 7b mở rộng:

- **Phép ghi kèm:** `store.CharState.Ops []store.Op`. Saver chạy các phép này trong cùng giao dịch DB với trạng thái nhân vật, sau phần nhân vật. Khi gộp lần lưu mới hơn của cùng nhân vật, saver **nối thêm** phép ghi kèm (trạng thái thì lấy bản mới nhất); lưu lỗi thì giữ nguyên các phép để thử lại. Phép ghi kèm chỉ chạy một lần vì chỉ bị xóa sau khi giao dịch DB thành công.
- **Lưu nhóm:** `Saver.SaveLinked(states []store.CharState, done)` ghi các nhân vật đó trong một giao dịch DB. Liên kết giữ qua các lần gộp cho tới khi ghi xong; hai nhóm có chung nhân vật thì gộp làm một.
- Các loại phép (mỗi phần thêm loại của mình vào `store`):

| Phép | Phần | Việc làm trong giao dịch |
| --- | --- | --- |
| `TradeLog` | 7b | chèn một dòng `trade_logs` |
| `GuildDonate` | 7c | `guilds.fund += xu`, `guild_members.contribution += c` của người góp; bang không còn thì bỏ qua |
| `ClaimRewards` | 7f | đánh dấu đã nhận các dòng `character_rewards` của nhân vật chưa nhận |
| `GuildFundTax` | 7h | `guilds.fund += xu` của bang thành chủ; bang không còn thì bỏ qua |

Quỹ bang chỉ đổi bằng phép cộng/trừ tương đối (`fund = fund + Δ`), không bao giờ ghi đè, nên saver và dịch vụ bang ghi quỹ theo thứ tự nào cũng đúng. Khoản chi từ quỹ dùng câu điều kiện `… WHERE fund >= chi` (7c).

### 3.3. Đồng hồ và lịch

- Mọi giờ trong game là **giờ Việt Nam (UTC+7)**, cùng luật ngày của 6d (`quest.Day`). Lịch sự kiện (7e) khai báo theo thứ trong tuần và giờ:phút UTC+7.
- Dịch vụ sự kiện có goroutine riêng xét lịch mỗi giây theo `Config.Now`; zone đọc ảnh chụp sự kiện mỗi giây như cách 6d xét ngày.

### 3.4. Loại zone mới

- **Map một khu** (7f): map `battle` có `max_zones: 1` và `capacity` riêng (mặc định `ZONE_CAPACITY`); không đổi khu, không mở khu thêm; đầy thì `ZONE_FULL`.
- **Cấp vào map** (7f): trường `min_level` của map, xét ở bước cuối của mọi lần chuyển zone (`LEVEL_TOO_LOW`); map thành và map nối với `START_MAP` có `min_level` 1.
- **Map chỉ dành cho sự kiện** (7g): trường `event_only` của map (đăng nhập lại khi đang ở map đó thì về thành của map); trường `areas` (hình chữ nhật có tên, như các toa tàu). Quái chỉ của sự kiện có cờ `event_only` trong `monsters.json` (7e) và không được đặt trong vùng quái của map.
- **Hồi sinh riêng của vùng** (7h): trường `respawn_sec` không bắt buộc của vùng quái, đè `respawn_sec` của loại quái.
- **Zone riêng (instance)** (7g): world mở một zone của map `instance` cho một tổ đội, không có trong danh sách khu, không ai khác vào được, đóng khi hết lượt hoặc trống.

### 3.5. Luật PK theo nhóm

Từ 7c, "cùng nhóm" trong `combat.PvPAllowed(attackerPK, targetPK, sameGroup)` là **cùng tổ đội hoặc cùng bang**: người cùng bang không đánh được nhau ở mọi map, kể cả map chiến sự. Áp cho chém, chiêu, chiêu diện rộng, auto (6c) và bộ lọc mục tiêu của client.

### 3.6. Cấp của nội dung sự kiện

Thế giới hiện có năm map (cấp 1–20, spec 4a). Game-design 5.5 chốt khung 1–90 cho bản đầu nhưng chưa có giai đoạn nào trong lộ trình làm các map 21–90 (spec 4a mục 1.3 hẹn "làm sau bằng cùng script"). Giai đoạn 7 giữ đúng thiết kế game-design và **không hạ cấp nội dung cuối game** cho vừa năm map:

| Sự kiện | Cấp tham gia | Cấp quái |
| --- | --- | --- |
| Lệnh Diệt Quỷ (7e) | Mọi cấp | Theo vùng quái của từng map luyện cấp: chơi được ngay |
| Thảo Phạt Thượng Huyền (7f) | ≥ 50 | Boss cấp 80 |
| Chuyến Tàu Vô Tận (7g) | ≥ 66 | 66–75, Enmu 75 (game-design 5.13) |
| Vô Hạn Thành (7h) | ≥ 50 | 70–90 (Boss 80–90) |

Cho tới khi có map 21–90, người chơi thật chưa tới được ba sự kiện cuối; kiểm thử và nghiệm thu dùng `devchar` để đặt cấp. Cấp quái sự kiện nằm trong dữ liệu sinh bằng script nên chỉnh được khi cân bằng.

**Phụ thuộc ngoài Giai đoạn 7 (đề xuất cho người dùng):** một phần "Thế giới mở rộng" làm các map và quái thường cấp 21–90 của game-design 5.5 cùng tinh anh/thủ lĩnh của chúng (khung của 7d), trước khi phát hành v1.

### 3.7. Tiền và vật phẩm trong giao dịch

- Xu giao dịch được; **Lượng không giao dịch được** giữa người chơi (Lượng thuộc tài khoản và gắn với nạp tiền, game-design 7.1).
- Vật phẩm có cờ `no_trade` trong `items.json` không giao dịch, không bày sạp: sách kỹ năng (6b). Mọi vật phẩm khác, kể cả thưởng sự kiện, giao dịch được; v1 không có đồ khóa.

## 4. Để sau v1

| Nội dung | Lý do |
| --- | --- |
| NPC đấu giá/treo bán ngoại tuyến ở Vô Hạn Thành (game-design 6.3, 7.2) | Cần ký gửi đồ ngoài nhân vật, trả tiền cho người bán đang ngoại tuyến hay đang ở zone khác, hết hạn và hoàn đồ: một hệ thống lớn. v1 thay bằng thuế trên **sạp** (7h), nên thành chủ vẫn có quyền lợi kinh tế |
| Sạp ngoại tuyến (treo sạp khi thoát game) | Cần giữ nhân vật hoặc ký gửi ngoài zone như trên |
| Kho bang, cửa hàng bang, đổi cống hiến lấy thưởng | Cần luật quyền rút đồ, nhật ký và xử lý tranh chấp; quỹ và cống hiến của 7c đủ cho v1 |
| Nhiệm vụ bang/tuần hoàn (áp tiêu, thu thập, 10–20 vòng/ngày, game-design 4.2) | Cần hộ tống và vật phẩm thu thập chưa có |
| Liên minh, bang chiến ngoài Vô Hạn Thành, chiến tranh bang | Không có trong game-design |
| Huấn Luyện Trụ Cột | Cần cơ chế mới: chướng ngại phá được, bảo vệ mục tiêu, chấm điểm bài tập |
| Tiếp Vận Làng Thợ Rèn | Cần xe hàng là thực thể di chuyển có máu, cướp hàng và map tranh chấp riêng |
| Phòng Thủ Làng Thợ Rèn | Cần NPC có máu bị quái tấn công và map sự kiện Làng Thợ Rèn |
| Diễn Võ Sát Quỷ Đoàn, danh hiệu Trụ Cột, danh hiệu nói chung | Cần ghép trận, lôi đài, xếp hạng tuần; game-design 10.4 còn mở thể thức |
| Lễ hội theo mùa, đua top máy chủ mới | Game-design 6.1 chưa chốt công thức và luật xếp hạng |
| Bạn bè, thư giữa người chơi, lịch sử chat lưu server, lọc từ ngữ, báo cáo người chơi | Không cần cho vòng chơi lõi; chặn người chơi làm ở client (7a) |
| Đồ khóa (không giao dịch) cho thưởng sự kiện | Chưa có nhu cầu khi thưởng v1 là Xu, EXP, đá, trang bị thường |
| Tinh anh/thủ lĩnh cấp 25–90 và các hiệu ứng chậm, giữ chân, đẩy lùi, phân thân chung máu | Đến cùng map của chúng (mục 3.6); 7d dựng khung, 7g thêm hiệu ứng ngủ cho Enmu |
| Hạ Huyền/Thượng Huyền ngoài bộ v1 của 7f, 7h | Thêm bằng dữ liệu khi cần |
| Điểm năng động, rương mốc hàng ngày | Đã để sau ở 6d |

## 5. Tài liệu cập nhật khi xong Giai đoạn 7

- Spec Giai đoạn 0 mục 1.1: Giai đoạn 7 chia 7a–7h theo tài liệu này; bảng tham chiếu mục 3.3 ghi các bảng thật đã tạo (`trade_logs`; `guilds` không có cột `leader_id`, bang chủ là `guild_members.rank = 1`; `guild_members`; cột `characters.guild_left_at`; `character_rewards`; `castle_owner`; `siege_runs`) thay cho `market_listings`, `event_runs`, `event_scores`, `character_titles` chưa làm.
- `docs/game-design.md` mục 10: bỏ các điểm đã chốt trong spec 7a–7h; mục 11 thêm danh sách "Để sau v1" ở trên.
