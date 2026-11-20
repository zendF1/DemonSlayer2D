# Giai đoạn 7c — Bang hội

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ ba của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md)). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 4.1 (NPC hoạt động bang hội), 4.2 (cống hiến), 5.4 (Tổng hành dinh có NPC bang hội), 5.7 (bảng PK), 6.3 (quỹ bang, điểm bang) và 7.2 (thuế vào quỹ bang); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 5b](2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md) (khuôn dịch vụ mức world, lời mời, thẻ người chơi), [spec 7a](2026-11-20-giai-doan-7a-chat-design.md) (chat), [spec 7b](2026-11-20-giai-doan-7b-giao-dich-design.md) (phép ghi kèm của saver, `IN_TRADE`) và [spec 4b](2026-11-20-giai-doan-4b-npc-cua-hang-binh-thuoc-design.md) (NPC, tầm NPC). 7c chạy sau khi 7b đã triển khai (giao thức 17, migration 00010).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Ở Tổng hành dinh có NPC **Quản lý bang hội**. Nhân vật từ cấp 20 chưa có bang lập được một bang miễn phí và thành bang chủ. Bang chủ và phó bang mời người chơi đang online vào bang từ thẻ người chơi; người được mời thấy lời mời có đếm ngược. Bang có ba chức vụ, thông báo bang, quỹ bang và cấp bang từ 1 tới 5 quyết định số thành viên tối đa. Thành viên góp Xu vào quỹ ở NPC và nhận điểm cống hiến; bang chủ dùng quỹ để nâng cấp bang. Tên bang hiện dưới tên nhân vật; người cùng bang không đánh được nhau; khung chat có kênh **Bang**. Cửa sổ Bang hội liệt kê mọi thành viên, kể cả người đang ngoại tuyến.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Bang sống ở đâu | Dịch vụ `internal/guild` mức world (tổng quan mục 3.1): nạp mọi bang và thành viên khi khởi động, giữ trong bộ nhớ dưới một khóa; mọi thay đổi ghi DB trước rồi mới đổi bộ nhớ | Bang tồn tại qua nhiều map, khu và cả khi không ai online; chỉ một tiến trình nên bộ nhớ là nguồn đúng cho đăng ký tên và thành viên |
| Ghi DB của dịch vụ | Chạy trên goroutine của kết nối đã gửi lệnh, tuần tự dưới một **khóa thao tác** riêng (giữ trong lúc ghi DB); khóa dữ liệu chỉ giữ khi đọc/đổi bộ nhớ | Thao tác bang hiếm (vài lần mỗi phút), ghi vài mili-giây; tuần tự tránh hai thao tác cùng sửa một bang; zone và người đọc không bao giờ chờ I/O |
| Phí lập bang | **Miễn phí** | Xu thuộc zone, dòng bang thuộc dịch vụ bang: thu phí cần hai bên cùng ghi nguyên tử. Cấp 20 và hồi 24 giờ đủ chặn lập bang tràn lan |
| Điều kiện lập | Cấp ≥ 20, chưa có bang, hết hồi rời bang, đứng gần Quản lý bang hội | Bang là nội dung sau tân thủ; NPC theo game-design 4.1 |
| Điều kiện vào bang | Cấp ≥ 10, chưa có bang, hết hồi rời bang | Không để nhân vật cấp 1 làm đầy chỗ; tân thủ có chính tuyến dẫn đường |
| Tên bang | 2–12 ký tự sau chuẩn hóa NFC, chữ cái (có dấu) và chữ số, không khoảng trắng; duy nhất theo khóa tên chữ thường (như tên nhân vật, `auth`) | Cùng luật đã có; tên ngắn vừa dưới tên nhân vật trên điện thoại |
| Chức vụ | Bang chủ (1), Phó bang (tối đa 3), Thành viên | Ba mức đủ chia việc mời và mời ra; bảng quyền ở mục 4.2 |
| Mời | Theo id thực thể của người đang online ở bất cứ map, khu nào (thẻ người chơi, như 5b); hết hạn 30 giây; mỗi người chỉ có một lời mời bang đang chờ (`GUILD_INVITE_PENDING`) | Cùng cách với tổ đội; không cần ô nhập tên |
| Xin vào bang | Không có ở v1; chỉ có lời mời | Ít lệnh; người muốn vào nhắn riêng (7a) |
| Rời, bị mời ra | Rời bất cứ lúc nào; bị mời ra cũng vậy. Cả hai đặt **hồi 24 giờ** (`characters.guild_left_at`) trước khi vào hay lập bang khác | Chặn nhảy bang để lấy thưởng bang (7f, 7h) |
| Bang chủ rời | Không được (`GUILD_LEADER`); phải nhường chức hoặc giải tán | Bang luôn có bang chủ |
| Nhường chức | Bang chủ đặt chức `LEADER` cho một thành viên; người cũ thành phó bang (nếu đủ 3 phó bang thì thành thành viên) | Một lệnh đổi chức dùng chung |
| Giải tán | Chỉ bang chủ, phải gõ đúng tên bang để xác nhận; quỹ mất; mọi thành viên ra khỏi bang **không** bị hồi 24 giờ, riêng bang chủ bị hồi | Thành viên không có lỗi; bang chủ không lập-giải tán liên tục |
| Thông báo bang | Bang chủ và phó bang sửa; tối đa 100 ký tự sau chuẩn hóa như chat (7a) | Kênh dặn dò cố định cho cả bang |
| Cấp bang và số thành viên | Cấp 1–5: tối đa 20, 25, 30, 35, 40 người | Bang lớn cần đầu tư; 40 người vừa sức chứa một map chiến sự |
| Nâng cấp | Bang chủ ở Quản lý bang hội; tốn quỹ 200.000 / 500.000 / 1.000.000 / 2.000.000 Xu cho cấp 2, 3, 4, 5 | Quỹ có chỗ tiêu; tổng 3,7 triệu Xu là mục tiêu cả bang góp trong vài tuần |
| Góp quỹ | Mọi thành viên, ở Quản lý bang hội, mỗi lần bội số của 1.000 Xu, tối thiểu 1.000; cống hiến cộng `xu / 100` | Bội số giữ cống hiến là số nguyên; game-design 4.2 "thưởng cống hiến" |
| Lưu góp quỹ | Zone trừ Xu và lưu ngay nhân vật kèm phép `GuildDonate` (tổng quan mục 3.2); cùng lúc gọi dịch vụ bang (chỉ khóa, không I/O) để cộng quỹ và cống hiến trong bộ nhớ | Xu và quỹ đổi trong một giao dịch DB; không mất, không nhân đôi |
| Quỹ trong bộ nhớ | Là DB cộng các khoản góp chưa ghi xong; nâng cấp trừ bằng câu điều kiện `fund >= chi` rồi bộ nhớ trừ đúng khoản chi (không đọc lại quỹ từ DB) | Quỹ chỉ đổi tương đối nên thứ tự ghi không quan trọng (tổng quan mục 3.2) |
| Cống hiến dùng để làm gì | Chỉ hiển thị và để các phần sau xét (7f, 7h); không có cửa hàng cống hiến ở v1 | Cửa hàng bang để sau v1 (tổng quan mục 4) |
| Zone biết bang thế nào | Mỗi phiên có **ô bang** `*atomic.Pointer[guild.Tag]` (`Tag{ID, Name}` không đổi, nil là không có bang) do dịch vụ ghi, zone chỉ đọc; ô đi theo bản trao tay | Như ô tổ đội 5b: zone không nhận lệnh khi bang đổi, không lệnh nào rơi khi hộp thư đầy |
| Tên bang trên đầu | `EntityEnter.guild_name`; zone so ô bang của mỗi người chơi mỗi tick, đổi thì phát `LookChange` mang `guild_name` | Người xung quanh thấy ngay khi vào hay rời bang |
| PK | Cùng bang không đánh nhau ở mọi map (tổng quan mục 3.5): `PvPAllowed(…, cùng tổ đội hoặc cùng bang)` | Bang là đồng minh; tránh phá nhau trong sự kiện bang |
| Kênh chat Bang | `CHAT_CHANNEL_GUILD = 6`; người nhận là thành viên đang online; giới hạn 1 tin mỗi giây, không cần cấp | Kênh trao đổi chính của bang |
| Cập nhật cửa sổ bang | Dịch vụ đẩy `GuildState` cho mọi thành viên online khi thành viên, chức vụ, cấp, quỹ, thông báo hay trạng thái online đổi; map và khu của thành viên chỉ chính xác khi mở cửa sổ (`GuildInfoReq`) | Qua cổng rất thường xuyên; không đẩy 40 gói mỗi lần một người qua cổng |
| Thành viên ngoại tuyến | Hiện tên, class, cấp **lần cuối biết**, chức vụ, cống hiến, "Ngoại tuyến" | Cấp nạp từ DB khi khởi động và cập nhật khi người đó online |
| Xóa nhân vật | Bang chủ không xóa được nhân vật (`GUILD_LEADER`); xóa thành viên khác thì dòng thành viên mất theo (khóa ngoại) và dịch vụ bỏ người đó khỏi bộ nhớ | Bang không bao giờ mất bang chủ |
| Kho bang, cửa hàng bang, nhiệm vụ bang | Không làm ở v1 | Tổng quan mục 4 |
| Cho phần sau | `MemberInfo(charID) (guildID, rank, joinedAt, ok)` cho điều kiện Vô Hạn Thành (7h); `AddFund(guildID, xu)` ghi tương đối thẳng xuống DB cho thưởng sự kiện (7f, 7h) | Các phần sau không phải đọc bảng bang |

### 1.3. Bổ sung cho game-design

Game-design có quỹ bang, cống hiến và điểm bang nhưng không mô tả lập bang, chức vụ hay cấp bang. Spec này chốt chúng (mục 1.2) và ghi vào game-design một mục mới "Bang hội" trong mục 7 (mục 9).

### 1.4. Không làm ở 7c

Phí lập bang; xin vào bang; kho bang, cửa hàng bang, đổi cống hiến; nhiệm vụ bang/tuần hoàn; liên minh, bang chiến; biểu tượng bang; đổi tên bang; nhật ký bang; mời theo tên; giới hạn số lần góp quỹ mỗi ngày; xếp hạng bang ngoài sự kiện.

## 2. Dữ liệu

### 2.1. NPC Quản lý bang hội

Vai NPC mới `guild` trong file map (4b), không có `shop` hay `routes`. `tools/mapgen.py` thêm `quan_ly_bang` "Quản lý bang hội" ở Tổng hành dinh theo luật vị trí của 4b (trong khu an toàn, cách NPC khác ít nhất 1,5 ô, cách `default` 2–4 ô). `mapdata` chấp nhận vai `guild`; vai này chỉ được có ở map loại `town`.

### 2.2. Hằng số

Trong `internal/guild` (không phải file dữ liệu, vì chỉ server dùng và client nhận qua giao thức):

| Hằng | Giá trị |
| --- | --- |
| `CreateLevel` | 20 |
| `JoinLevel` | 10 |
| `RejoinCooldown` | 24 giờ |
| `InviteTTL` | 30 giây |
| `MaxOfficers` | 3 |
| `MaxLevel` | 5 |
| `MemberCap[level]` | 20, 25, 30, 35, 40 |
| `UpgradeCost[level → level+1]` | 200.000, 500.000, 1.000.000, 2.000.000 |
| `DonateStep` | 1.000 Xu |
| `ContributionPerXu` | 1 cống hiến mỗi 100 Xu |
| `NoticeMaxRunes` | 100 |

Client không cần bảng này: `GuildState` mang sẵn số thành viên tối đa và giá nâng cấp kế tiếp.

## 3. Lưu trữ

Migration `00011_guilds.sql`:

- Bảng `guilds`: `id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `name text NOT NULL`, `name_key text NOT NULL UNIQUE`, `level smallint NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 5)`, `fund bigint NOT NULL DEFAULT 0 CHECK (fund >= 0)`, `notice text NOT NULL DEFAULT ''`, `created_at timestamptz NOT NULL DEFAULT now()`.
- Bảng `guild_members`: `guild_id bigint NOT NULL REFERENCES guilds (id) ON DELETE CASCADE`, `character_id bigint NOT NULL UNIQUE REFERENCES characters (id) ON DELETE CASCADE`, `rank smallint NOT NULL CHECK (rank BETWEEN 1 AND 3)` (1 bang chủ, 2 phó bang, 3 thành viên), `contribution bigint NOT NULL DEFAULT 0 CHECK (contribution >= 0)`, `joined_at timestamptz NOT NULL DEFAULT now()`; khóa chính `(guild_id, character_id)`; chỉ mục duy nhất một phần `(guild_id) WHERE rank = 1` (mỗi bang đúng một bang chủ).
- `characters.guild_left_at timestamptz` (null là chưa từng rời). Down xóa cột và hai bảng.

**Ai ghi gì:**

- Dịch vụ bang ghi `guilds` (trừ phép góp quỹ), `guild_members` (trừ cột `contribution` khi góp) và `characters.guild_left_at`. `store.SaveCharacterState` **không** đụng `guild_left_at`, nên lần lưu của zone không ghi đè.
- Phép ghi kèm `GuildDonate{GuildID, CharID, Xu, Contribution}` (saver, tổng quan mục 3.2): `UPDATE guilds SET fund = fund + $xu WHERE id = $g`; `UPDATE guild_members SET contribution = contribution + $c WHERE guild_id = $g AND character_id = $ch`. Không có dòng nào khớp (bang đã giải tán, người đó đã rời) thì bỏ qua, không lỗi.

**Store** (gói `store`, hàm mới):

- `LoadGuilds(ctx) ([]GuildRow, []GuildMemberRow, error)`: mọi bang; mọi thành viên kèm tên, class, cấp, map của nhân vật.
- `LoadGuildCooldowns(ctx, since time.Time) (map[int64]time.Time, error)`: nhân vật có `guild_left_at > since`.
- `CreateGuild(ctx, name, nameKey string, leaderID int64) (GuildRow, error)`: chèn bang và dòng bang chủ trong một giao dịch; trùng `name_key` trả `store.ErrGuildNameTaken`.
- `AddGuildMember(ctx, guildID, charID int64)`, `RemoveGuildMember(ctx, guildID, charID int64, leftAt *time.Time)` (đặt `guild_left_at` khi khác nil, cùng giao dịch), `SetGuildRanks(ctx, guildID int64, ranks map[int64]int16)` (một giao dịch; dùng cho đổi chức và nhường chức), `SetGuildNotice(ctx, guildID, text)`, `DisbandGuild(ctx, guildID, leaderID int64, leftAt time.Time)`.
- `UpgradeGuild(ctx, guildID int64, fromLevel int, cost int64) (bool, error)`: `UPDATE guilds SET level = level + 1, fund = fund − cost WHERE id = $1 AND level = $2 AND fund >= $3`; trả false khi không dòng nào đổi.
- `AddGuildFund(ctx, guildID, xu int64) (bool, error)`: `fund = fund + xu` (xu ≥ 1).
- `GuildLeaderOf(ctx, charID)`: không cần; dịch vụ trả lời từ bộ nhớ.

## 4. Server

### 4.1. Dịch vụ bang

`guild.New(store, now func() time.Time)`; `Load(ctx)` khi khởi động (trước khi mở cổng). Trạng thái: các bang (id, tên, cấp, quỹ, thông báo, thành viên theo thứ tự vào), chỉ mục tên, chỉ mục nhân vật → bang, hồi rời bang theo nhân vật, phiên online (id thực thể, id nhân vật, tên, class, cấp, map, khu, hộp gửi, ô bang), lời mời đang chờ.

- **Đăng ký:** `World.Enter` gọi `Register(Session)`, nhận ô bang (đã đặt `Tag` nếu nhân vật trong bang) và đặt vào `PlayerInfo.Guild`; dịch vụ cập nhật cấp lần cuối biết, đẩy `GuildState` mới cho các thành viên online khác (trạng thái online đổi). `EnterWorldResp.guild` và `.guild_rejoin_ms` lấy từ dịch vụ lúc dựng phiên.
- **Hủy đăng ký:** `Membership.Leave` gọi `Unregister(id)`: bỏ mọi lời mời tới hoặc do người đó gửi; đẩy `GuildState` cho thành viên còn online.
- **Chuyển zone:** `World.Transfer` thành công gọi `Moved(id, map, zone)` (không đẩy gì).
- **Lên cấp:** hàm `Leveled` của `zone.Config` (5b) được world nối tới cả tổ đội lẫn bang; bang chỉ cập nhật cấp, không đẩy.
- **Đọc từ zone và dịch vụ khác** (chỉ khóa dữ liệu): `Donated(guildID, charID, xu, c) (fund, contribution int64, ok bool)`; `MemberInfo(charID) (guildID uint64, rank Rank, joinedAt time.Time, ok bool)`; `OnlineMembers(guildID) []Outbox` (chat, mục 4.8); `Exists(guildID) bool`; `AddFundCached(guildID, xu)` chỉ cộng quỹ trong bộ nhớ khi khoản cộng đã được ghi xuống DB bằng một phép ghi kèm ở nơi khác (thuế sạp của 7h).
- **Cho sự kiện:** `AddFund(ctx, guildID, xu) error`: gọi `store.AddGuildFund` dưới khóa thao tác rồi cộng quỹ trong bộ nhớ và đẩy `GuildState`. Gọi từ goroutine của dịch vụ sự kiện, không bao giờ từ zone.

Mỗi lệnh dưới đây chạy trên goroutine của kết nối: kiểm tra dưới khóa dữ liệu, rồi lấy khóa thao tác, kiểm tra lại (trạng thái có thể đã đổi), ghi DB, đổi bộ nhớ, ghi ô bang, đẩy tin. Lỗi DB trả `INTERNAL` và không đổi gì trong bộ nhớ.

### 4.2. Quyền theo chức vụ

| Việc | Bang chủ | Phó bang | Thành viên |
| --- | --- | --- | --- |
| Mời | Có | Có | Không |
| Mời ra thành viên thường | Có | Có | Không |
| Mời ra phó bang | Có | Không | Không |
| Đổi chức, nhường chức | Có | Không | Không |
| Sửa thông báo | Có | Có | Không |
| Nâng cấp bang | Có | Không | Không |
| Giải tán | Có | Không | Không |
| Góp quỹ, rời bang, chat Bang | Có (trừ rời) | Có | Có |

Thiếu quyền: `GUILD_NO_PERMISSION`.

### 4.3. Lập bang

`GuildCreateReq{npc_id, name}`, lỗi đầu tiên được trả:

1. Zone kiểm tra (truy vấn chỉ đọc `Membership.NearNPC`, mục 4.7): chết `DEAD`; NPC không có hoặc không phải vai `guild` `BAD_REQUEST`; xa NPC `NPC_TOO_FAR`; cấp < 20 `LEVEL_TOO_LOW`.
2. Đã có bang: `ALREADY_IN_GUILD`.
3. Chưa hết hồi rời bang: `GUILD_COOLDOWN`.
4. Tên sai luật: `GUILD_NAME_INVALID`.
5. Trùng khóa tên trong bộ nhớ (hoặc DB báo trùng): `GUILD_NAME_TAKEN`.

Hợp lệ: `store.CreateGuild`; bang cấp 1, quỹ 0; người lập là bang chủ; ghi ô bang; trả `GuildCreateResp{guild}` (đủ `GuildState`).

### 4.4. Mời và trả lời

`GuildInviteReq{target_id}` của I tới T:

1. T là I hoặc không có ai online với id đó: `INVALID_TARGET`.
2. I không có bang: `NOT_IN_GUILD`.
3. I là thành viên thường: `GUILD_NO_PERMISSION`.
4. Bang đã đủ số thành viên tối đa của cấp bang: `GUILD_FULL`.
5. T đã có bang (kể cả bang của I): `ALREADY_IN_GUILD`.
6. T cấp < 10: `LEVEL_TOO_LOW`.
7. T chưa hết hồi rời bang: `GUILD_COOLDOWN`.
8. T đang có lời mời bang chưa hết hạn: `GUILD_INVITE_PENDING`.

Hợp lệ: ghi lời mời (bang, I, T, hết hạn sau 30 giây); đẩy cho T `GuildInvite{from_id, from_name, guild_id, guild_name, guild_level, members, expires_ms = 30000}`; trả `GuildInviteResp{}`.

`GuildAnswerReq{guild_id, accept}` của T:

1. Không có lời mời chưa hết hạn của bang đó tới T: `GUILD_INVITE_NOT_FOUND`.

Từ đây lời mời bị xóa dù kết quả thế nào.

- **Từ chối:** đẩy cho I (nếu còn online) `GuildEvent{DECLINED, tên T}`; trả `GuildAnswerResp{}`.
- **Đồng ý**, lỗi đầu tiên được trả: 2. T đã có bang `ALREADY_IN_GUILD`; 3. bang không còn `GUILD_INVITE_NOT_FOUND`; 4. bang đủ người `GUILD_FULL`; 5. T chưa hết hồi `GUILD_COOLDOWN`. Hợp lệ: `store.AddGuildMember` (chức thành viên, cống hiến 0); ghi ô bang của T; đẩy `GuildEvent{JOINED, tên T}` cho thành viên cũ và `GuildState` cho mọi thành viên online; trả `GuildAnswerResp{}`.

Lời mời của người mời không còn quyền (bị hạ chức, rời bang) vẫn đồng ý được: quyền xét lúc mời.

### 4.5. Rời, mời ra, đổi chức, thông báo, giải tán

**`GuildLeaveReq{}`:** 1. không có bang `NOT_IN_GUILD`; 2. là bang chủ `GUILD_LEADER`. Hợp lệ: `store.RemoveGuildMember` với `leftAt = now`; ô bang về nil; người rời nhận `GuildState` rỗng; thành viên còn lại nhận `GuildEvent{LEFT, tên}` và `GuildState`; trả `GuildLeaveResp{}`.

**`GuildKickReq{char_id}`** của K: 1. `NOT_IN_GUILD`; 2. `char_id` là K hoặc không phải thành viên bang của K: `BAD_REQUEST`; 3. quyền theo mục 4.2 (thành viên thường, hoặc phó bang mời ra phó bang/bang chủ): `GUILD_NO_PERMISSION`. Hợp lệ: như rời (có hồi 24 giờ); người bị mời ra (nếu online) nhận `GuildEvent{YOU_WERE_KICKED}` và `GuildState` rỗng; còn lại nhận `GuildEvent{KICKED, tên}` và `GuildState`; trả `GuildKickResp{}`.

**`GuildSetRankReq{char_id, rank}`** của L: 1. `NOT_IN_GUILD`; 2. L không phải bang chủ `GUILD_NO_PERMISSION`; 3. `char_id` là L hoặc không phải thành viên, hoặc `rank` không thuộc `LEADER`, `OFFICER`, `MEMBER`: `BAD_REQUEST`; 4. `rank` bằng chức hiện tại: trả `GuildSetRankResp{}`, không đổi gì; 5. `OFFICER` khi đã có 3 phó bang: `GUILD_OFFICERS_FULL`. Hợp lệ:

- `OFFICER` hoặc `MEMBER`: đổi chức người đó; `GuildEvent{PROMOTED | DEMOTED, tên}`.
- `LEADER` (nhường chức): người đó thành bang chủ; L thành phó bang, hoặc thành viên nếu sau khi người đó rời chức cũ vẫn đủ 3 phó bang; ghi cả hai trong một giao dịch (`SetGuildRanks`); `GuildEvent{NEW_LEADER, tên}`.

Rồi `GuildState` cho mọi thành viên online; trả `GuildSetRankResp{}`.

**`GuildSetNoticeReq{text}`:** 1. `NOT_IN_GUILD`; 2. thành viên thường `GUILD_NO_PERMISSION`; 3. chuẩn hóa như chat 7a (bỏ ký tự điều khiển, gộp khoảng trắng, cắt hai đầu); dài hơn 100 ký tự `BAD_REQUEST` (rỗng được: xóa thông báo). Hợp lệ: lưu, `GuildState` cho mọi thành viên online, trả `GuildSetNoticeResp{}`.

**`GuildDisbandReq{confirm_name}`:** 1. `NOT_IN_GUILD`; 2. không phải bang chủ `GUILD_NO_PERMISSION`; 3. `confirm_name` (NFC) khác tên bang `BAD_REQUEST`. Hợp lệ: `store.DisbandGuild` (xóa bang, thành viên mất theo, đặt `guild_left_at` của bang chủ); mọi ô bang về nil; mọi lời mời của bang bị bỏ; thành viên online nhận `GuildEvent{DISBANDED}` và `GuildState` rỗng; trả `GuildDisbandResp{}`.

### 4.6. Góp quỹ và nâng cấp

**`GuildDonateReq{npc_id, xu}`** là lệnh zone (Xu thuộc zone), xử lý trong `drain`:

1. Chết: `DEAD`.
2. NPC không có hoặc không phải vai `guild`: `BAD_REQUEST`. Xa NPC: `NPC_TOO_FAR`.
3. Đang giao dịch hay bày sạp (7b): `IN_TRADE`.
4. Ô bang nil: `NOT_IN_GUILD`.
5. `xu` < 1.000 hoặc không chia hết cho 1.000: `BAD_REQUEST`.
6. `xu` lớn hơn Xu đang có: `NOT_ENOUGH_XU`.
7. `guild.Donated(guildID, charID, xu, xu / 100)` trả `ok = false` (vừa rời bang hay bang vừa giải tán): `NOT_IN_GUILD`.

Hợp lệ: trừ Xu; lưu ngay kèm phép `GuildDonate{guild, char, xu, xu / 100}`; trả `GuildDonateResp{stats, fund, contribution}` (quỹ và cống hiến mới trong bộ nhớ). Dịch vụ đẩy `GuildState` cho các thành viên online (sau khi trả khóa, không trên goroutine zone: `Donated` chỉ ghi một cờ "cần đẩy" và một goroutine nhỏ của dịch vụ gửi đi, để zone không phải dựng 40 gói).

**`GuildUpgradeReq{npc_id}`:** 1. zone kiểm tra `NearNPC` (`DEAD`, `BAD_REQUEST`, `NPC_TOO_FAR`); 2. `NOT_IN_GUILD`; 3. không phải bang chủ `GUILD_NO_PERMISSION`; 4. cấp bang là 5 `GUILD_MAXED`; 5. quỹ trong bộ nhớ < giá `GUILD_FUND_LOW`. Hợp lệ: `store.UpgradeGuild`; trả false (DB chưa có khoản góp vừa xong) thì `GUILD_FUND_LOW`. Thành công: cấp +1, quỹ trong bộ nhớ trừ giá; `GuildEvent{UPGRADED, ""}` và `GuildState` cho mọi thành viên online; trả `GuildUpgradeResp{level, fund}`.

### 4.7. Truy vấn NPC của zone

`Membership.NearNPC(npcID, role string) (level int, code pb.ErrorCode)`: gửi lệnh chỉ đọc vào zone kèm kênh trả lời (đệm 1) và chờ tối đa 2 giây (hết giờ hoặc hộp thư đầy: `RATE_LIMITED`). Zone kiểm tra chết, vai NPC, tầm NPC (4b mục 4.1) và trả cấp nhân vật. Kết quả có thể cũ ngay khi trả về; không sao vì không có gì đổi theo nó ngoài lệnh của dịch vụ bang.

### 4.8. Trong zone

- `PlayerInfo.Guild *atomic.Pointer[guild.Tag]`; người chơi trong zone giữ con trỏ đó và `lastTag` (Tag đã gửi lần cuối). Bản trao tay mang con trỏ ô (cùng một ô, không sao chép).
- Cuối mỗi tick (trước `syncView`), Tag hiện tại khác `lastTag` thì ghi id thực thể vào danh sách đổi ngoại hình (5a mục 4.9) và cập nhật `lastTag`. `EntityEnter.guild_name` và `LookChange.guild_name` là tên của Tag hiện tại (rỗng khi nil).
- **PK:** "cùng nhóm" của 5a mục 4.2 bước 4 và 5b mục 4.7 nay là cùng tổ đội **hoặc** hai Tag khác nil có cùng `ID`. Áp cho chém, chiêu đơn, mục tiêu được nêu và từng nạn nhân của chiêu diện rộng; kiểm tra lúc bấm.
- Không chia EXP theo bang; tiến độ nhiệm vụ không đổi.

### 4.9. Chat Bang

Kênh `GUILD` của chat (7a): người gửi phải có bang (không thì `NOT_IN_GUILD`); giới hạn 1 tin mỗi giây (`Limiter.Take`/`Refund` như mọi kênh 7a); người nhận là `guild.OnlineMembers(guildID)` gồm cả người gửi. Chat lấy id bang từ ô bang của phiên người gửi (dịch vụ chat giữ ô này khi đăng ký) và gọi `OnlineMembers` sau khi đã trả khóa của chính nó.

### 4.10. Xóa nhân vật

`handleDeleteChar` (trước khi vào game), sau bước so tên và trước khi xóa: `guild.IsLeader(charID)` đúng thì `GUILD_LEADER`. Xóa xong gọi `guild.CharacterDeleted(charID)`: bỏ người đó khỏi bộ nhớ (dòng DB đã mất theo khóa ngoại), đẩy `GuildState` cho thành viên online.

### 4.11. Gateway

Mọi lệnh bang cần đã vào thế giới (`NOT_IN_WORLD`) và chỉ chịu giới hạn gói chung. `GuildCreateReq`, `GuildUpgradeReq` gọi `NearNPC` rồi dịch vụ trên goroutine của kết nối; `GuildDonateReq` là lệnh zone (hộp thư đầy `RATE_LIMITED`); các lệnh còn lại gọi thẳng dịch vụ. Tin đẩy cho chính người gửi (`GuildState`) có thể tới trước trả lời.

## 5. Giao thức

`ProtocolVersion` lên **18**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 160 | `GuildCreateReq` | `string npc_id = 1`, `string name = 2` |
| 161 | `GuildCreateResp` | `GuildState guild = 1` |
| 162 | `GuildInviteReq` | `uint64 target_id = 1` |
| 163 | `GuildInviteResp` | rỗng |
| 164 | `GuildInvite` | `uint64 from_id = 1`, `string from_name = 2`, `uint64 guild_id = 3`, `string guild_name = 4`, `int32 guild_level = 5`, `int32 members = 6`, `int32 expires_ms = 7` |
| 165 | `GuildAnswerReq` | `uint64 guild_id = 1`, `bool accept = 2` |
| 166 | `GuildAnswerResp` | rỗng |
| 167 | `GuildLeaveReq` | rỗng |
| 168 | `GuildLeaveResp` | rỗng |
| 169 | `GuildKickReq` | `int64 char_id = 1` |
| 170 | `GuildKickResp` | rỗng |
| 171 | `GuildSetRankReq` | `int64 char_id = 1`, `GuildRank rank = 2` |
| 172 | `GuildSetRankResp` | rỗng |
| 173 | `GuildSetNoticeReq` | `string text = 1` |
| 174 | `GuildSetNoticeResp` | rỗng |
| 175 | `GuildDisbandReq` | `string confirm_name = 1` |
| 176 | `GuildDisbandResp` | rỗng |
| 177 | `GuildDonateReq` | `string npc_id = 1`, `int64 xu = 2` |
| 178 | `GuildDonateResp` | `SelfStats stats = 1`, `int64 fund = 2`, `int64 contribution = 3` |
| 179 | `GuildUpgradeReq` | `string npc_id = 1` |
| 180 | `GuildUpgradeResp` | `int32 level = 1`, `int64 fund = 2` |
| 181 | `GuildState` | xem dưới |
| 182 | `GuildEvent` | `GuildEventKind kind = 1`, `string name = 2` |
| 183 | `GuildInfoReq` | rỗng |
| 184 | `GuildInfoResp` | `GuildState guild = 1` |

```proto
enum GuildRank {
  GUILD_RANK_UNSPECIFIED = 0;
  GUILD_RANK_LEADER = 1;
  GUILD_RANK_OFFICER = 2;
  GUILD_RANK_MEMBER = 3;
}

// One member as every member sees them. entity_id is 0 while offline;
// level and char_class are the last known values.
message GuildMember {
  int64 char_id = 1;
  uint64 entity_id = 2;
  string name = 3;
  CharClass char_class = 4;
  int32 level = 5;
  GuildRank rank = 6;
  int64 contribution = 7;
  bool online = 8;
  string map_id = 9; // set only in GuildInfoResp, for online members
  int32 zone = 10;
}

// The receiver's guild. No guild_id: the receiver is not in a guild.
message GuildState {
  uint64 guild_id = 1;
  string name = 2;
  int32 level = 3;
  int64 fund = 4;
  string notice = 5;
  int32 max_members = 6;
  int64 upgrade_cost = 7; // 0 at the top level
  repeated GuildMember members = 8; // in join order, leader first
}

enum GuildEventKind {
  GUILD_EVENT_KIND_UNSPECIFIED = 0;
  GUILD_EVENT_KIND_JOINED = 1;          // name joined the guild
  GUILD_EVENT_KIND_LEFT = 2;            // name left the guild
  GUILD_EVENT_KIND_KICKED = 3;          // name was removed
  GUILD_EVENT_KIND_YOU_WERE_KICKED = 4; // you were removed
  GUILD_EVENT_KIND_DECLINED = 5;        // name declined your invite
  GUILD_EVENT_KIND_DISBANDED = 6;       // the leader disbanded the guild
  GUILD_EVENT_KIND_NEW_LEADER = 7;      // name now leads the guild
  GUILD_EVENT_KIND_PROMOTED = 8;        // name is now an officer
  GUILD_EVENT_KIND_DEMOTED = 9;         // name is now a plain member
  GUILD_EVENT_KIND_UPGRADED = 10;       // the guild gained a level
}
```

- `GuildInvite`, `GuildState`, `GuildEvent` đẩy với `seq = 0`. `GuildState` không có `guild_id` nghĩa là không có bang.
- `EnterWorldResp.guild = 15` (`GuildState`), `EnterWorldResp.guild_rejoin_ms = 16` (`int64`, số mili giây còn lại của hồi rời bang, 0 khi đã hết).
- `EntityEnter.guild_name = 17` (`string`), `LookChange.guild_name = 5` (`string`); ghi chú `LookChange` vẫn là "the entity's whole look at the end of the tick".
- `ChatChannel`: `CHAT_CHANNEL_GUILD = 6`.
- `ErrorCode`: `GUILD_NAME_TAKEN = 73`, `GUILD_NAME_INVALID = 74`, `ALREADY_IN_GUILD = 75`, `NOT_IN_GUILD = 76`, `GUILD_NO_PERMISSION = 77`, `GUILD_FULL = 78`, `GUILD_COOLDOWN = 79` (rời bang chưa đủ 24 giờ), `GUILD_INVITE_NOT_FOUND = 80`, `GUILD_INVITE_PENDING = 81`, `GUILD_FUND_LOW = 82`, `GUILD_MAXED = 83`, `GUILD_LEADER = 84` (bang chủ không rời bang, không xóa nhân vật), `GUILD_OFFICERS_FULL = 85`; 86–88 để trống. Ghi chú `PK_FORBIDDEN` thêm "or a guild member".
- Fixture mới: `guild_state.hex` (ba thành viên, một ngoại tuyến), `guild_invite.hex`, `guild_event.hex`, `guild_donate_resp.hex`, `entity_enter_guild.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/guild.gd`** (mới): `apply_state(GuildState)`; `in_guild()`, `guild_id`, `name`, `level`, `fund`, `notice`, `max_members`, `upgrade_cost`, `members` (theo thứ tự), `my_rank(self_char_id)`, `can(action, self_char_id, target_char_id := 0) -> bool` theo bảng mục 4.2 (`"invite"`, `"kick"`, `"set_rank"`, `"notice"`, `"upgrade"`, `"disband"`); `is_member_entity(entity_id)`; lời mời đang chờ `set_invite(msg, now)`, `invite_left(now)`, `clear_invite()`; `rejoin_at` từ `guild_rejoin_ms`; `event_text(kind, name)` câu tiếng Việt của mục 6.3; `valid_name(s) -> String` (rỗng hoặc lý do, cùng luật server).
- **`core/pk_rules.gd`**: `same_party` mở rộng thành `same_group` = cùng tổ đội hoặc tên bang trong `EntityEnter`/`LookChange` trùng với bang của mình (client không có id bang của người khác; tên bang là duy nhất nên đủ). Bộ lọc mục tiêu (5a) và auto (6c) dùng chung.
- **`core/entity_table.gd`**: lưu `guild_name` từ `EntityEnter` và `LookChange`.
- **`core/errors.gd`**: chữ cho 13 mã mới (ví dụ `GUILD_COOLDOWN` "Vừa rời bang, chưa thể vào bang khác", `GUILD_LEADER` "Bang chủ phải nhường chức hoặc giải tán bang trước"). **`core/wire.gd`**: các payload mới.
- Mô hình ở `autoload/router.gd` (`Router.guild`), giữ qua lần nạp lại màn World; xóa khi về màn chọn nhân vật.

### 6.2. Giao diện

- **Nút Bang** trên HUD (cạnh nút nhân vật) mở **cửa sổ Bang hội** (`ui/guild_window.gd`, mới). Không có bang: dòng "Chưa có bang. Gặp Quản lý bang hội ở Tổng hành dinh để lập bang (cấp 20)" và đếm hồi rời bang nếu còn. Có bang: mở cửa sổ thì gửi `GuildInfoReq`; hai thẻ:
  - **Thông tin:** tên, cấp, "Thành viên n/max", quỹ, giá nâng cấp kế tiếp, thông báo (bang chủ/phó bang chạm để sửa), nút **Rời bang** (hỏi lại; bang chủ thấy nút bị khóa kèm lý do), nút **Giải tán** cho bang chủ (ô gõ lại tên bang).
  - **Thành viên:** mỗi người một dòng: chấm màu class, tên, cấp, chức, cống hiến, "<map> · Khu n" hoặc "Ngoại tuyến"; người online xếp trước. Chạm dòng mở các nút theo `guild.can`: **Lên phó bang**, **Xuống thành viên**, **Nhường bang chủ** (hỏi lại), **Mời ra** (hỏi lại), **Nhắn riêng** (7a).
- **Cửa sổ NPC** vai `guild` (`ui/npc_window.gd`): chưa có bang thì ô nhập tên và nút **Lập bang** (kiểm tra `valid_name` trước khi gửi); có bang thì **Góp quỹ** (chọn 1.000 / 10.000 / 100.000 hoặc nhập bội số 1.000; hiện cống hiến sẽ nhận) và **Nâng cấp bang** (chỉ bang chủ; giá và quỹ hiện có).
- **Thẻ người chơi** (5b): nút **Mời vào bang** khi `guild.can("invite")` và người đó không có tên bang.
- **Lời mời bang** (`ui/invite_popup.gd` dùng chung với tổ đội): "<tên> mời bạn vào bang <tên bang> (cấp n, m người)", đếm ngược, **Đồng ý** / **Từ chối**.
- **Tên bang** (`ui/entity_view.gd`): dòng nhỏ "«Tên bang»" dưới tên nhân vật; người cùng bang tên màu xanh lá như đồng đội.
- **Chat** (7a): thêm thẻ **Bang**; chỉ hiện khi có bang.
- **Thông báo** (`GuildEvent`) trên dòng thông báo HUD:

| Loại | Câu |
| --- | --- |
| `JOINED` | "<tên> đã vào bang" |
| `LEFT` | "<tên> đã rời bang" |
| `KICKED` | "<tên> bị mời ra khỏi bang" |
| `YOU_WERE_KICKED` | "Bạn bị mời ra khỏi bang" |
| `DECLINED` | "<tên> đã từ chối lời mời vào bang" |
| `DISBANDED` | "Bang đã giải tán" |
| `NEW_LEADER` | "<tên> là bang chủ mới" |
| `PROMOTED` | "<tên> được phong phó bang" |
| `DEMOTED` | "<tên> thôi làm phó bang" |
| `UPGRADED` | "Bang đã lên cấp" |

### 6.3. Smoke

`tools/smoke.sh`: `devchar -level 20 -map ubuyashiki -x … -y …` cạnh `quan_ly_bang`, `devitem -xu 2000`, `devguild -clear` cho nhân vật smoke (mục 7). `tests/smoke/smoke.gd` sau các bước cũ: `GuildCreateReq` với tên ngẫu nhiên "Smoke" + 4 chữ số; chờ `GuildCreateResp` có mình là bang chủ; `GuildDonateReq` 1.000 Xu, kiểm tra quỹ 1.000 và cống hiến 10; `GuildDisbandReq` với đúng tên; chờ `GuildState` rỗng. In "SMOKE OK" kèm "guild created, donated 1000".

## 7. Lệnh dev

`cmd/devguild` (chạy khi server tắt, vì dịch vụ bang chỉ nạp DB lúc khởi động):

- `-create <tên bang> -leader <tên nhân vật> [-members a,b,c] [-level n] [-fund n]`: tạo bang (thành viên chức thường, cống hiến 0).
- `-clear <tên nhân vật>`: đưa nhân vật ra khỏi bang (bang chủ thì giải tán bang đó) và xóa `guild_left_at`.
- `-cooldown <tên nhân vật> -ago <thời lượng>`: đặt `guild_left_at = now − ago`.

Lỗi (tên lạ, nhân vật đã có bang) thì không ghi gì.

## 8. Kiểm thử

**Server** (`go test -race`):

- `mapdata`: vai `guild` được nhận ở map `town`, bị từ chối ở map `training`; `ubuyashiki` thật có `quan_ly_bang`.
- `store` (DB): migration lên rồi xuống; `CreateGuild` rồi `LoadGuilds` đúng tên, cấp, bang chủ; trùng khóa tên trả `ErrGuildNameTaken`; chỉ mục một bang chủ chặn hai dòng `rank = 1`; `RemoveGuildMember` đặt `guild_left_at`; `SaveCharacterState` không đổi `guild_left_at`; phép `GuildDonate` cộng quỹ và cống hiến trong cùng giao dịch với Xu (một lần lưu, một lần đọc lại); phép `GuildDonate` với bang đã xóa không lỗi và vẫn lưu Xu; `UpgradeGuild` trả false khi quỹ thiếu hoặc cấp lệch; xóa nhân vật xóa dòng thành viên.
- `guild` (store giả, đồng hồ giả, hộp gửi giả): từng mã lỗi theo đúng thứ tự cho lập, mời, trả lời, rời, mời ra, đổi chức, thông báo, giải tán, nâng cấp; bảng quyền mục 4.2 đủ các ô; hồi 24 giờ: rời lúc t thì mời ở t + 23 giờ 59 phút bị `GUILD_COOLDOWN`, ở t + 24 giờ được; giải tán không đặt hồi cho thành viên, có cho bang chủ; nhường chức khi đã đủ 3 phó bang làm bang chủ cũ thành thành viên; đủ 20 người ở cấp 1 thì lời mời và đồng ý đều `GUILD_FULL`, nâng cấp xong thì được; lời mời hết hạn sau 30 giây; ô bang đúng sau mỗi bước (vào, rời, bị mời ra, giải tán); lỗi DB không đổi bộ nhớ; `Donated` cộng quỹ và cống hiến, trả `ok = false` sau khi rời; `UpgradeGuild` của store trả false thì `GUILD_FUND_LOW` và bộ nhớ không đổi; quỹ bộ nhớ sau nâng cấp bằng quỹ cũ trừ giá; `MemberInfo` trả thời điểm vào bang; `AddFund` cộng quỹ và đẩy `GuildState`; `Register`/`Unregister` đổi `online` và đẩy cho thành viên khác; `CharacterDeleted` bỏ thành viên.
- `zone`: `GuildDonateReq` từng mã lỗi theo thứ tự; hợp lệ thì Xu giảm, lần lưu mang phép `GuildDonate` đúng số, trả quỹ mới; đổi Tag trong ô thì tick đó có `LookChange` với `guild_name` cho người thấy; `EntityEnter` mang `guild_name`; hai người cùng bang bật PK chém nhau bị `PK_FORBIDDEN`, chiêu vòng bỏ qua người cùng bang nhưng trúng người khác bang bật PK; `NearNPC` trả `NPC_TOO_FAR` và cấp đúng.
- `world`: `Enter` đặt ô bang từ dịch vụ; ô đi qua bản trao tay (sau cổng vẫn không đánh được người cùng bang); `Leveled` cập nhật cấp của thành viên.
- `chat`: kênh `GUILD` không có bang bị `NOT_IN_GUILD`; tin tới đúng thành viên online của bang, không tới người bang khác.
- `gateway`: `ProtocolVersion` 18; fixture mới; lệnh bang khi chưa vào thế giới `NOT_IN_WORLD`; xóa nhân vật là bang chủ bị `GUILD_LEADER`.
- e2e `TestGuildsOverWebSocket`: A cấp 20, B cấp 15 (store); A đặt cạnh `quan_ly_bang`, có 5.000 Xu; A lập bang "Kiemsi"; A mời B, B đồng ý, cả hai nhận `GuildState` hai người; B thấy `guild_name` của A khi vào tầm nhìn; A góp 2.000 Xu, quỹ 2.000, cống hiến 20, `SelfStats.xu` 3.000; A thoát, vào lại: Xu 3.000; B gửi chat Bang, A nhận; A đổi B thành phó bang; A nhường bang chủ cho B; A rời bang; A mời lại bị `NOT_IN_GUILD`, B mời A bị `GUILD_COOLDOWN`; A xóa nhân vật được (không còn là bang chủ), B xóa nhân vật bị `GUILD_LEADER`.

**Client** (`tests/run.sh`, mục `# ____TEST GUILD____`): `guild` (áp `GuildState`, `can` đủ bảng quyền, lời mời hết giờ, `event_text` đủ mười loại, `valid_name`); `pk_rules.same_group` với tên bang; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito, cấp 90) lập bang ở Tổng hành dinh, mời `test2` (Hana) qua thẻ người chơi; Hana đồng ý; tên bang hiện dưới tên cả hai; hai người bật PK ở Sagiri vẫn không đánh được nhau; chat kênh Bang; Kaito góp quỹ, nâng cấp bang lên cấp 2 (dùng `devguild -fund` trước), thấy "Thành viên 2/25"; Kaito nhường bang chủ cho Hana, rời bang, thử vào lại bang bị báo hồi 24 giờ.

## 9. Tài liệu cập nhật

- `docs/game-design.md`: mục 7 thêm mục "Bang hội" (lập miễn phí từ cấp 20, ba chức vụ, cấp bang và số người, quỹ từ góp Xu và thuế, cống hiến, hồi 24 giờ, cùng bang không đánh nhau); mục 5.7 thêm "người cùng bang không đánh được nhau"; mục 4.1 ghi Quản lý bang hội ở Tổng hành dinh.
- Spec Giai đoạn 0 mục 3.3: dòng `guilds`, `guild_members` theo cột thật (không có `leader_id`, có `guild_left_at`).
- `CLAUDE.md` (server): dịch vụ bang là nguồn đúng trong bộ nhớ; quỹ bang chỉ đổi tương đối (`fund = fund + Δ`), chi dùng câu điều kiện; zone không bao giờ gọi hàm I/O của dịch vụ bang.
- README hai repo: bang hội, Quản lý bang hội, kênh Bang, `devguild`.
