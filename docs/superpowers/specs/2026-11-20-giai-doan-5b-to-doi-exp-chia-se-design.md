# Giai đoạn 5b — Tổ đội và EXP chia sẻ

Ngày: 2026-11-20. Trạng thái: Server đã triển khai, client chờ (nhiệm vụ 7–10).

Phần cuối của Giai đoạn 5 (xem bảng chia ở [spec 5a](2026-11-20-giai-doan-5a-pk-phat-exp-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.2 (thời gian lên cấp khi đi tổ đội), 5.7 (bộ lọc PK) và 5.8 (EXP tổ đội, quyền nhặt đồ); game-design là chuẩn, trừ luật 5% mà game-design 5.8 đã ghi là "sẽ được định lại ở Giai đoạn 5" — spec này định lại nó (mục 1.4). Những gì không nhắc tới ở đây giữ nguyên như spec 5a, các spec Giai đoạn 4 và [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md). 5b chạy sau khi 5a đã triển khai (giao thức 12, migration 00008).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Chạm vào một người chơi để mở thẻ người chơi, bấm **Mời tổ đội**. Người được mời thấy lời mời có đếm ngược và chọn Đồng ý hoặc Từ chối. Tổ đội tối đa 5 người, có một trưởng nhóm. Bên trái màn hình là khung tổ đội: tên, cấp, máu của đồng đội đang ở gần, hoặc map và khu của đồng đội ở xa, kèm nút sang khu của họ. Mỗi đòn của một thành viên trả EXP thì các đồng đội cùng map, cùng khu được thêm 5% số EXP đó. Đồng đội không đánh được nhau. Rời tổ đội, trưởng nhóm mời người ra, thoát game là rời tổ đội.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Tổ đội sống ở đâu | Dịch vụ `internal/party` ở mức world, một khóa, chỉ trong bộ nhớ; không lưu DB; tắt server thì mất | Tổ đội trải qua nhiều map và khu, không thuộc một zone; lập lại tổ đội rất nhanh |
| Zone biết tổ đội thế nào | Mỗi phiên vào game có một **ô id tổ đội** nguyên tử (`*atomic.Uint64`) do dịch vụ tổ đội ghi, zone chỉ đọc; ô đi theo bản trao tay khi chuyển zone | Zone không phải nhận lệnh khi tổ đội đổi, không có lệnh nào bị rơi khi hộp thư zone đầy; zone vẫn không ghi trạng thái ngoài |
| Thoát game | Thoát game, mất kết nối, bị đá đều là rời tổ đội | Tổ đội chỉ trong bộ nhớ, không có trạng thái "ngoại tuyến" để hiển thị |
| Kích thước | 2–5 người; còn 1 người thì tổ đội giải tán | Game-design 5.8 |
| Trưởng nhóm | Người mời đầu tiên. Chỉ trưởng nhóm mời và mời người ra. Trưởng nhóm rời thì người vào sớm nhất còn lại làm trưởng | Luật thường gặp; không cần bầu |
| Chuyển trưởng nhóm, giải tán chủ động | Không có; rời tổ đội là đủ | Giữ ít lệnh |
| Mời ai | Theo id thực thể (chạm vào nhân vật đang thấy); người được mời ở bất cứ map, khu nào cũng được; không mời theo tên | Không cần chat hay ô nhập tên; id thực thể giữ nguyên khi chuyển zone (4a) |
| Lời mời | Hết hạn sau 30 giây; mỗi người chỉ có một lời mời đang chờ; lời mời thứ hai bị từ chối `INVITE_PENDING` | Không phải xếp hàng nhiều lời mời trên điện thoại |
| Hết hạn | Xét khi trả lời (không có bộ hẹn giờ); client tự ẩn lời mời khi hết giờ | Không cần goroutine hẹn giờ |
| EXP tổ đội (định lại luật 5%) | Mỗi lần một thành viên A nhận g EXP từ một đòn (luật EXP theo sát thương), mỗi đồng đội T **cùng zone** (cùng map, cùng khu) nhận thêm **5% × g**, không khấu trừ của A; phần lẻ cộng dồn theo từng T nên không mất | Trả theo đòn như EXP gốc; tổng một con quái do cả tổ đánh là EXP gốc × (1 + 5% × số đồng đội), đúng ×1,2 của bảng game-design 2.2 với tổ năm người |
| Điều kiện nhận | T cùng tổ đội và cùng zone với A; **cấp T chênh cấp quái không quá 10**; không xét khoảng cách, còn sống hay đã đánh con quái | Game-design 5.8 (cùng map, cùng khu, không giới hạn khoảng cách, không cần gây sát thương); cửa sổ ±10 chặn kéo cấp nhân vật cấp thấp ở map cấp cao, như cửa sổ EXP gốc |
| Đồ rơi, Xu | Không đổi: thuộc người kết liễu | Game-design 5.8 |
| Tiến độ nhiệm vụ | Không chia (Giai đoạn 6) | Game-design 5.8 |
| PK với đồng đội | Không đánh được nhau, kể cả khi bật PK; chiêu diện rộng bỏ qua đồng đội | Chặn đánh nhầm trong nhóm; dùng `PvPAllowed(…, sameParty)` của 5a |
| Khung tổ đội | Bên trái, dưới thẻ nhân vật; mỗi đồng đội một dòng: chấm màu class, tên, cấp, dấu ★ nếu là trưởng nhóm; máu khi nhân vật đó đang trong tầm nhìn, nếu không thì tên map và khu; cùng map khác khu thì có nút "Sang khu N" | Không thêm luồng gửi máu đồng đội qua các zone; thông tin cần nhất khi đánh cùng nhau đã có trong tầm nhìn |
| Cập nhật khung | Server đẩy `PartyState` đủ thành viên mỗi khi có người vào, ra, đổi trưởng nhóm, qua map hoặc khu, lên cấp | Ít sự kiện, gói nhỏ; client không phải ghép |
| Thẻ người chơi | Chạm vào người chơi không khóa được làm mục tiêu (5a), hoặc chạm thẻ mục tiêu khi mục tiêu là người chơi, thì mở thẻ: tên, cấp, class, PK, nút **Mời tổ đội** | Mời được cả trong thành, nơi không ai đánh được ai |
| Chat | Không làm; sự kiện tổ đội hiện thành dòng thông báo trên HUD | Chat thuộc Giai đoạn 7+; tổ đội không cần chat để chạy |
| Tên đồng đội | Màu xanh lá (đè màu đỏ PK) | Nhận ra đồng đội giữa đám đông |

### 1.3. Ví dụ EXP

Quái cấp 1 có 56 HP và 6 EXP gốc. A và T cùng tổ, cùng khu; A tự đánh 4 nhát 14 sát thương, nhận 1, 2, 1, 2 EXP (game-design 5.8). T nhận 5% của từng lần: cộng dồn 5, 15, 20, 30 phần trăm EXP, tức 0 EXP cho tới khi tích đủ 100; sau con quái này T còn dư 30 và chưa nhận EXP nào. Con tiếp theo có 750 HP và 143 EXP gốc (750 × r(10) = 142,5, làm tròn lên): A hạ một mình thì nhận 143, T nhận floor((30 + 143 × 5) / 100) = 7 và giữ phần dư 45 cho lần sau. Năm người cùng đánh với tốc độ như nhau thì mỗi người nhận thêm khoảng 20% (4 đồng đội × 5%).

### 1.4. Khác với game-design

Game-design 5.8 (bản cũ) cho mỗi đồng đội 5% EXP gốc khi **người hạ** hạ quái. Với EXP trả theo đòn, không còn "người hạ nhận EXP gốc"; spec này đổi thành 5% của **từng lần trả EXP**, cùng tổng như cũ khi một người hạ cả con, và thêm cửa sổ ±10 cấp cho đồng đội. Mục 5.8 được viết lại theo spec này (mục 8).

### 1.5. Không làm ở 5b

Chat (tổ đội, thế giới, riêng); mời theo tên; chuyển trưởng nhóm; tổ đội ngoại tuyến hoặc lưu qua khởi động lại; tìm tổ đội; chia đồ hay Xu theo tổ đội; chia tiến độ nhiệm vụ; máu và MP của đồng đội ở ngoài tầm nhìn; dịch chuyển tới đồng đội; phó bản tổ đội (Chuyến Tàu sự kiện thuộc Giai đoạn 7+); auto theo trưởng nhóm.

## 2. Dữ liệu

Không có file dữ liệu mới. Hằng số trong code: `party.MaxMembers = 5`, `party.InviteTTL = 30 * time.Second`, `combat.PartyBonusPct = 5`.

## 3. Lưu trữ

Không có migration. Tổ đội không lưu; phần dư EXP tổ đội của mỗi nhân vật không lưu và không đi qua bản trao tay (mất tối đa dưới 1 EXP mỗi lần chuyển zone).

## 4. Server

### 4.1. Dịch vụ tổ đội

Gói `internal/party` giữ mọi tổ đội dưới một khóa và đẩy tin nhắn thẳng vào hộp gửi của từng thành viên (hộp gửi của gateway an toàn khi nhiều goroutine cùng gửi, không chặn). Dịch vụ không bao giờ gọi vào world hay zone.

- **Đăng ký:** `World.Enter` đăng ký nhân vật (id thực thể, tên, class, cấp, map, khu, hộp gửi) và nhận về ô id tổ đội, đặt vào `PlayerInfo.Party`.
- **Hủy đăng ký:** `Membership.Leave` hủy đăng ký trước khi rời zone: người đó rời tổ đội như lệnh rời (mục 4.4) nhưng không nhận gì; mọi lời mời gửi tới hoặc do người đó gửi bị bỏ.
- **Chuyển zone:** `World.Transfer` thành công thì báo map và khu mới (`Moved`).
- **Lên cấp:** zone gọi `Leveled(id, cấp)` (một hàm trong `zone.Config`, có thể rỗng) khi cấp đổi.
- Tổ đội có id riêng (bộ đếm trong dịch vụ, khác 0), danh sách thành viên theo thứ tự vào và trưởng nhóm. Vào tổ đội thì ô id tổ đội của người đó được ghi id tổ đội; rời thì ghi 0.

**`PartyState`** của một tổ đội: `leader_id` và các thành viên theo thứ tự vào, mỗi người có id thực thể, tên, class, cấp, map, khu. Gửi cho **mọi** thành viên mỗi khi tổ đội đổi (vào, ra, mời ra, đổi trưởng nhóm, `Moved`, `Leveled`). Người vừa ra khỏi tổ đội (rời, bị mời ra, tổ giải tán) nhận `PartyState` rỗng.

### 4.2. Mời

`PartyInviteReq{target_id}` do gateway gọi dịch vụ trên goroutine của kết nối. Người mời là I, người được mời là T. Lỗi đầu tiên được trả:

1. T là I hoặc không có người chơi nào đang trong thế giới với id đó: `INVALID_TARGET`.
2. I ở trong tổ đội nhưng không phải trưởng nhóm: `NOT_PARTY_LEADER`.
3. Tổ đội của I đã đủ 5 người: `PARTY_FULL`.
4. T đang ở trong một tổ đội (kể cả tổ của I): `ALREADY_IN_PARTY`.
5. T đang có lời mời chưa hết hạn: `INVITE_PENDING`.

Hợp lệ: ghi lời mời (I, T, hết hạn sau 30 giây), đẩy cho T `PartyInvite{from_id, from_name, from_level, from_class, expires_ms = 30000}`, trả `PartyInviteResp{}`. I chưa có tổ đội thì vẫn chưa có tổ đội cho tới khi T đồng ý. I có thể có nhiều lời mời đang chờ tới nhiều người.

### 4.3. Trả lời lời mời

`PartyAnswerReq{from_id, accept}` của T:

1. Không có lời mời chưa hết hạn từ `from_id` tới T: `INVITE_NOT_FOUND`.

Từ đây lời mời bị xóa dù kết quả thế nào.

- **Từ chối:** đẩy cho I (nếu còn trong thế giới) `PartyNotice{DECLINED, tên T}`; trả `PartyAnswerResp{}`.
- **Đồng ý**, lỗi đầu tiên được trả:
  2. T đã ở trong một tổ đội: `ALREADY_IN_PARTY`.
  3. I không còn trong thế giới, hoặc I ở trong tổ đội mà không phải trưởng nhóm: `INVITE_NOT_FOUND`.
  4. Tổ đội của I đã đủ 5 người: `PARTY_FULL`.

  Hợp lệ: I chưa có tổ đội thì lập tổ đội mới với I là trưởng nhóm và thành viên đầu; thêm T vào cuối. Ghi ô id tổ đội của người mới. Đẩy `PartyNotice{JOINED, tên T}` cho các thành viên cũ, `PartyState` cho mọi thành viên; trả `PartyAnswerResp{}`.

### 4.4. Rời tổ đội

`PartyLeaveReq{}` của R:

1. R không ở trong tổ đội: `NOT_IN_PARTY`.

Hợp lệ: bỏ R khỏi tổ đội, ghi ô của R về 0, gửi R `PartyState` rỗng, trả `PartyLeaveResp{}`. Với những người còn lại:

- Còn 1 người: tổ giải tán; người đó nhận ô về 0, `PartyNotice{DISBANDED}` và `PartyState` rỗng.
- Còn từ 2 người: `PartyNotice{LEFT, tên R}`; nếu R là trưởng nhóm thì người vào sớm nhất còn lại làm trưởng nhóm và mọi người nhận thêm `PartyNotice{NEW_LEADER, tên người đó}`; rồi `PartyState`.

Hủy đăng ký (thoát game) chạy đúng các bước trên, trừ phần gửi cho R.

### 4.5. Mời người ra

`PartyKickReq{member_id}` của L:

1. L không ở trong tổ đội: `NOT_IN_PARTY`.
2. L không phải trưởng nhóm: `NOT_PARTY_LEADER`.
3. `member_id` là L hoặc không phải thành viên của tổ: `BAD_REQUEST`.

Hợp lệ: bỏ người đó khỏi tổ, ô về 0; người đó nhận `PartyNotice{YOU_WERE_KICKED}` và `PartyState` rỗng; trả `PartyKickResp{}`. Những người còn lại: còn 1 người thì giải tán như mục 4.4, nếu không thì `PartyNotice{KICKED, tên người đó}` và `PartyState`.

### 4.6. EXP tổ đội trong zone

Trong `payExpShare` (EXP theo sát thương), sau khi A nhận `g > 0` EXP từ một đòn lên quái m:

1. `pid` = ô id tổ đội của A; 0 thì dừng.
2. Với mỗi người chơi T khác A **trong zone này** có ô id tổ đội bằng `pid` và `combat.ExpEligible(cấp T, cấp m)`: `(thưởng, T.partyCarry) = combat.PartyBonus(T.partyCarry, g)`; `gainExp(T, thưởng)`.

`combat.PartyBonus(carry, gain) (bonus, newCarry int64)`: `tổng = carry + gain × PartyBonusPct`; `bonus = tổng / 100`; `newCarry = tổng % 100`.

- Thưởng không ghi vào bảng sát thương hay EXP đã trả của quái, nên không làm giảm EXP gốc của ai.
- T chết vẫn nhận (như `gainExp` hiện nay với người chết); lên cấp khi chết không hồi máu.
- Duyệt mọi người chơi của zone (tối đa sức chứa khu, 50) mỗi lần trả EXP là đủ nhanh cho v1.

### 4.7. PK với đồng đội

Bước "cùng tổ đội" của spec 5a mục 4.2 nay là: ô id tổ đội của A khác 0 và bằng ô của B. Áp cho chém, chiêu đơn, mục tiêu được nêu và từng nạn nhân của chiêu diện rộng. Kiểm tra lúc bấm như 5a.

### 4.8. Gateway

Bốn lệnh `PartyInviteReq`, `PartyAnswerReq`, `PartyLeaveReq`, `PartyKickReq` cần đã vào thế giới (`NOT_IN_WORLD`), chỉ chịu giới hạn gói chung, gọi dịch vụ đồng bộ trên goroutine của kết nối, trả lời bằng phản hồi tương ứng hoặc `ErrorResp`. Các tin đẩy của dịch vụ cho chính người gửi (`PartyState`) có thể tới trước trả lời.

## 5. Giao thức

`ProtocolVersion` lên **13**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 108 | `PartyInviteReq` | `uint64 target_id = 1` |
| 109 | `PartyInviteResp` | rỗng |
| 110 | `PartyInvite` | `uint64 from_id = 1`, `string from_name = 2`, `int32 from_level = 3`, `CharClass from_class = 4`, `int32 expires_ms = 5` |
| 111 | `PartyAnswerReq` | `uint64 from_id = 1`, `bool accept = 2` |
| 112 | `PartyAnswerResp` | rỗng |
| 113 | `PartyLeaveReq` | rỗng |
| 114 | `PartyLeaveResp` | rỗng |
| 115 | `PartyKickReq` | `uint64 member_id = 1` |
| 116 | `PartyKickResp` | rỗng |
| 117 | `PartyState` | `uint64 leader_id = 1`, `repeated PartyMember members = 2` |
| 118 | `PartyNotice` | `PartyNoticeKind kind = 1`, `string name = 2` |

```proto
// One party member as every member sees it. id is the entity id, kept
// across zone changes.
message PartyMember {
  uint64 id = 1;
  string name = 2;
  CharClass char_class = 3;
  int32 level = 4;
  string map_id = 5;
  int32 zone = 6;
}

enum PartyNoticeKind {
  PARTY_NOTICE_KIND_UNSPECIFIED = 0;
  PARTY_NOTICE_KIND_JOINED = 1;          // name joined the party
  PARTY_NOTICE_KIND_LEFT = 2;            // name left the party
  PARTY_NOTICE_KIND_KICKED = 3;          // the leader removed name
  PARTY_NOTICE_KIND_YOU_WERE_KICKED = 4; // the leader removed you
  PARTY_NOTICE_KIND_DECLINED = 5;        // name declined your invite
  PARTY_NOTICE_KIND_DISBANDED = 6;       // you were the last member
  PARTY_NOTICE_KIND_NEW_LEADER = 7;      // name now leads the party
}
```

- `PartyInvite`, `PartyState`, `PartyNotice` đẩy với `seq = 0`. `PartyState` không có thành viên nghĩa là không ở trong tổ đội.
- `ErrorCode`: `PARTY_FULL = 47`, `ALREADY_IN_PARTY = 48`, `NOT_PARTY_LEADER = 49`, `NOT_IN_PARTY = 50`, `INVITE_NOT_FOUND = 51`, `INVITE_PENDING = 52`. Ghi chú của `INVALID_TARGET` thêm "or no player with that id is in the world" và của `PK_FORBIDDEN` thêm "or a party member".
- Fixture mới: `party_invite.hex`, `party_state.hex` (ba thành viên, hai map), `party_notice.hex`, `party_answer_req.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/party.gd`** (mới): `apply_state(PartyState)`; `members` (theo thứ tự), `leader_id`, `in_party()`, `is_member(id)`, `is_leader(id)`, `others(self_id)`; `can_invite(self_id, target_id)` (mình chưa có tổ đội, hoặc là trưởng nhóm của tổ chưa đủ 5, và người đó không phải đồng đội); `invite` đang chờ `{from_id, name, level, class, until}` với `set_invite(msg, now)`, `invite_left(now)`, `clear_invite()`; `notice_text(kind, name)` trả câu tiếng Việt của mục 6.3.
- **`core/pk_rules.gd`**: `same_party` lấy từ `party.is_member`.
- **`core/errors.gd`**: PARTY_FULL "Tổ đội đã đủ 5 người", ALREADY_IN_PARTY "Đã ở trong một tổ đội", NOT_PARTY_LEADER "Chỉ trưởng nhóm làm được việc này", NOT_IN_PARTY "Bạn chưa ở trong tổ đội", INVITE_NOT_FOUND "Lời mời không còn hiệu lực", INVITE_PENDING "Người này đang có lời mời khác". **`core/wire.gd`**: các payload mới.
- Mô hình tổ đội nằm ở `autoload/router.gd` (`Router.party`) để còn nguyên khi màn World nạp lại sau chuyển zone (4a); xóa khi về màn chọn nhân vật.

### 6.2. Giao diện

- **Khung tổ đội** (`ui/party_frame.gd`): dưới thẻ nhân vật, ẩn khi không có tổ đội. Dòng đầu "Tổ đội n/5" và nút **Rời**. Mỗi đồng đội một dòng như mục 1.2; máu lấy từ bảng thực thể khi đồng đội đang trong tầm nhìn, nếu không ghi "<tên map> · Khu n" (`map_grid.name_of`). Đồng đội cùng map khác khu: nút **Sang khu n** gửi `ChangeZoneReq` qua đường chuyển zone của 4a. Trưởng nhóm chạm dòng một đồng đội thì hiện nút **Mời ra** (gửi `PartyKickReq`).
- **Thẻ người chơi** (`ui/player_card.gd`): mở như mục 1.2; tên, "Lv n", tên class, dòng "Đang bật PK" nếu có; nút **Mời tổ đội** chỉ hiện khi `party.can_invite`; trả lời thì hiện "Đã gửi lời mời", lỗi thì hiện chữ lỗi. Tự đóng khi người đó ra khỏi tầm nhìn.
- **Lời mời** (`ui/invite_popup.gd`): "<tên> (Lv n, class) mời bạn vào tổ đội", đếm ngược, nút **Đồng ý** và **Từ chối** (gửi `PartyAnswerReq`); hết giờ thì tự ẩn; lời mời mới thay lời mời đang hiện.
- **Tên đồng đội** (`ui/entity_view.gd`): màu xanh lá.
- **Thông báo** (`PartyNotice`) hiện trên dòng thông báo của HUD:

| Loại | Câu |
| --- | --- |
| `JOINED` | "<tên> đã vào tổ đội" |
| `LEFT` | "<tên> đã rời tổ đội" |
| `KICKED` | "<tên> bị mời ra khỏi tổ đội" |
| `YOU_WERE_KICKED` | "Bạn bị mời ra khỏi tổ đội" |
| `DECLINED` | "<tên> đã từ chối lời mời" |
| `DISBANDED` | "Tổ đội đã giải tán" |
| `NEW_LEADER` | "<tên> là trưởng nhóm mới" |

### 6.3. Smoke

`tools/smoke.sh` chạy thêm `botclient partner` (mục 7) với tài khoản mới, đọc tên nhân vật của nó qua thư mục trao tay. `tests/smoke/smoke.gd` sau bước 5a: chờ thấy thực thể người chơi có tên đó (cả hai vào ở điểm `default` của Kumotori), gửi `PartyInviteReq`, chờ `PartyState` hai thành viên với mình là trưởng nhóm, gửi `PartyLeaveReq`, chờ `PartyState` rỗng. In "SMOKE OK" kèm "party 2/5".

## 7. Lệnh dev

`cmd/botclient` thêm lệnh `partner [-for 60s]`: đăng ký tài khoản mới, tạo nhân vật, vào thế giới, in `PARTNER <tên>` rồi đồng ý lời mời tổ đội đầu tiên nhận được, in `PARTY OK` khi `PartyState` có hai thành viên, đứng yên tới hết `-for`.

## 8. Kiểm thử

**Server** (`go test -race`):

- `combat`: `PartyBonus` với ví dụ mục 1.3 (bốn lần 1, 2, 1, 2 từ dư 0 cho 0 EXP và dư 30; 143 với dư 30 cho 7 và dư 45); tổng thưởng qua nhiều lần đúng `floor(5% × tổng g)`.
- `party` (đồng hồ giả, hộp gửi giả): từng mã lỗi theo thứ tự cho bốn lệnh; mời lập tổ đội khi đồng ý, trưởng nhóm là người mời; thứ tự thành viên theo lúc vào; người thứ sáu bị `PARTY_FULL` cả lúc mời lẫn lúc đồng ý; lời mời hết hạn sau 30 giây thì đồng ý bị `INVITE_NOT_FOUND` và mời lại được; từ chối gửi `DECLINED` cho người mời; trưởng nhóm rời thì người vào sớm nhất làm trưởng và có `NEW_LEADER`; còn một người thì giải tán; mời ra gửi `YOU_WERE_KICKED` và `PartyState` rỗng cho người bị mời ra; hủy đăng ký như rời và bỏ mọi lời mời liên quan; ô id tổ đội đúng sau mỗi bước; `Moved` và `Leveled` đẩy `PartyState` mới cho mọi thành viên; người mời đã vào tổ khác làm thành viên thì lời mời cũ bị `INVITE_NOT_FOUND`.
- `zone` EXP tổ đội: A hạ quái, T cùng tổ cùng zone nhận đúng `floor(5% × EXP gốc)`, A nhận đủ EXP gốc; T chênh cấp quái 11 không nhận; người khác tổ, người không có tổ không nhận; T đã chết vẫn nhận; A ngoài cửa sổ cấp (g = 0) thì T không nhận; hai thành viên cùng đánh thì mỗi người nhận phần mình cộng 5% phần người kia.
- `zone` PK đồng đội: hai đồng đội cùng bật PK chém nhau bị `PK_FORBIDDEN`; chiêu `single` lên đồng đội bị `PK_FORBIDDEN`; chiêu vòng bỏ qua đồng đội nhưng trúng người bật PK khác tổ.
- `world`: `Enter` đăng ký và ô đi vào zone; `Leave` hủy đăng ký (đồng đội nhận `LEFT` hoặc `DISBANDED`); qua cổng thì `PartyState` mang map và khu mới; ô id tổ đội đi qua bản trao tay (đồng đội vẫn không đánh được nhau ở zone mới); lên cấp đẩy `PartyState`.
- `gateway`: `ProtocolVersion` 13; fixture mới; bốn lệnh khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestPartyOverWebSocket`: A, B, C cấp 1; trước khi vào game, A và B được `DevSetPlace` đặt cạnh nhau ngay ngoài trại Kumotori, C ở điểm `default`; A mời B, B nhận `PartyInvite` và đồng ý, A và B nhận `PartyState` hai người với trưởng nhóm A; A mời C, C từ chối, A nhận `DECLINED`; A mời lại C, C đồng ý, ba người; B mời ra C bị `NOT_PARTY_LEADER`; A mời ra C, C nhận `YOU_WERE_KICKED` và `PartyState` rỗng; A và B bật PK, A chém B bị `PK_FORBIDDEN`; A đi tới và hạ một quái thường trong khi B đứng yên và không đánh gì, EXP của B tăng đúng `floor(5 × EXP gốc của quái / 100)`; A thoát game, B nhận `DISBANDED` và `PartyState` rỗng.

**Client** (`tests/run.sh`): `party` (áp `PartyState`, `can_invite` các trường hợp, lời mời hết giờ, `notice_text` đủ bảy loại); `pk_rules` với `same_party`; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito) và `test2` (Hana) gặp nhau ở Tổng hành dinh: Kaito chạm Hana, mời; Hana đồng ý; cả hai thấy khung tổ đội; đi qua cổng tới Sagiri, khung cập nhật map; Hana đổi sang khu 2, Kaito thấy "Sang khu 2" và bấm theo; cùng đánh quái, EXP của mỗi người tăng nhanh hơn khi đi một mình; hai người bật PK vẫn không đánh được nhau; Hana rời tổ, tổ giải tán.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 5.8 (viết lại luật EXP tổ đội theo mục 1.2–1.3: 5% của từng lần trả EXP cho đồng đội cùng map, cùng khu, chênh cấp quái không quá 10; bỏ các dòng ví dụ bản cũ; giữ quyền nhặt đồ), mục 2.2 (ghi chú ×1,2 nay đến từ luật mới), mục 5.7 (đồng đội không đánh nhau).
- Spec Giai đoạn 0 mục 1.1: Giai đoạn 5 xong khi 5b triển khai.
- `CLAUDE.md`: bất biến "tổ đội nằm trong `internal/party`, chỉ trong bộ nhớ; zone chỉ đọc ô id tổ đội nguyên tử, không bao giờ ghi; dịch vụ tổ đội không gọi vào world hay zone"; dòng EXP thêm "đồng đội cùng zone nhận thêm 5% mỗi lần trả EXP".
- README hai repo: tổ đội, mời và rời, EXP tổ đội, `botclient partner`.
