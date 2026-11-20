# Giai đoạn 5a — Chế độ PK, đánh người và phạt EXP

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Spec này thiết kế phần đầu của Giai đoạn 5 (PK và tổ đội) cho server (repo này) và client Godot (`../DemonSlayer2DGame`). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.2 (công thức PvP), 2.5 (chọn mục tiêu), 5.1 (loại bản đồ), 5.7 (chế độ PK, phạt EXP, hồi sinh) và [nguyên tắc cân bằng chiến đấu](../../combat-balance.md); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như các spec trước, nhất là [spec Giai đoạn 2](2026-11-20-giai-doan-2-ky-nang-design.md), [spec ba mốc của đòn đánh](2026-11-20-moc-don-danh-design.md), [spec 4a](2026-11-20-giai-doan-4a-nhieu-map-cong-khu-design.md) và [spec 4c](2026-11-20-giai-doan-4c-kho-luong-hoi-sinh-design.md). Giai đoạn 3 và 4 được triển khai trước, nên spec này viết trên giao thức 11 và migration 00007.

## 1. Mục tiêu và phạm vi

### 1.1. Chia Giai đoạn 5

Lộ trình (spec Giai đoạn 0 mục 1.1) ghi Giai đoạn 5 gồm bật/tắt PK, phạt EXP, tổ đội 5 người, EXP chia sẻ. Chia hai phần, làm theo thứ tự, mỗi phần xong là chơi được:

| Phần | Nội dung | Kết quả chơi được |
| --- | --- | --- |
| **5a** (spec này) | Nút PK, đánh người bằng chém và chiêu, bộ lọc PK cho chiêu diện rộng, phạt EXP khi chết do PK, thông báo ai hạ mình | Bật PK đánh người ở map luyện cấp, chết mất EXP |
| 5b ([spec](2026-11-20-giai-doan-5b-to-doi-exp-chia-se-design.md)) | Tổ đội 5 người: mời, nhận, rời, mời ra; EXP chia sẻ theo đòn; đồng đội không đánh nhau; khung tổ đội | Lập nhóm đi đánh quái cùng nhau |

### 1.2. Mục tiêu 5a

Trên HUD có nút **PK**. Bật PK thì tên nhân vật hiện màu đỏ với mọi người. Ở map luyện cấp, ngoài khu an toàn, người bật PK chém và dùng chiêu lên được mọi người chơi; người tắt PK đánh được người đang bật PK (phản công ngay, không cần bật). Chiêu diện rộng trúng cả quái lẫn người chơi đánh được. Người đang bật PK bị người chơi khác hạ ở map luyện cấp mất 5% EXP cần để lên cấp kế tiếp; bảng chết ghi ai đã hạ mình và mất bao nhiêu EXP. Trạng thái PK còn nguyên khi thoát game và khi qua cổng.

### 1.3. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chia giai đoạn | 5a, 5b như mục 1.1 | PK dùng được một mình; tổ đội cần PK sẵn có để chặn đồng đội đánh nhau |
| Map được PK | Map loại `training` và `battle`, ngoài khu an toàn. Map `town` và `instance` không cho đánh người | Game-design 5.1: làng/thành cấm PK tuyệt đối; phó bản "luật PK tùy hoạt động", v1 chưa có hoạt động nào nên mặc định cấm |
| Ai đánh được ai | Đúng bảng game-design 5.7: được khi người đánh **hoặc** mục tiêu đang bật PK. Một hàm thuần `combat.PvPAllowed` dùng cho chém, chiêu đơn, từng mục tiêu của chiêu diện rộng và client | Một luật cho mọi nơi (game-design 5.7 "dùng chung bảng trên") |
| Giới hạn cấp | Không có; người cấp 1 cũng đánh được và bị đánh | Game-design 5.7 "người bật PK đánh được tất cả người chơi"; người tắt PK không bao giờ mất EXP do PK |
| Đồng đội | 5b thêm: cùng tổ đội không đánh nhau, kể cả khi bật PK | Luật thường gặp của thể loại; game-design không nói khác |
| Bật PK | Được bất cứ lúc nào khi còn sống, cả trong khu an toàn; không có hồi chiêu | Bật PK chỉ làm mình dễ bị đánh hơn |
| Tắt PK | Bị chặn (`IN_PVP_COMBAT`) khi chưa đủ **10 giây** kể từ đòn PvP cuối cùng trúng mình hoặc do mình gây | Không để người bật PK sắp chết tắt PK để né phạt EXP; 10 giây khớp nhịp giao tranh |
| Lưu trạng thái PK | Cột `characters.pk_on`; giữ khi thoát game, qua cổng và đổi khu; đổi PK không lưu ngay mà theo nhịp lưu định kỳ và các lần lưu khác | Lộ trình Giai đoạn 0 đã dự kiến cột này; sập server chỉ làm mất một lần bật/tắt |
| Hệ số PvP | 100% (game-design 2.2), là hằng số `combat.PvPFactor`; sát thương vật lý xét thủ vật lý, sát thương phép xét thủ phép của người bị đánh; chém mặc định là vật lý | Game-design 2.2; giữ hằng số ở `internal/combat` để cân bằng sau chỉ sửa một chỗ |
| Thời điểm kiểm tra luật PK | Lúc bấm, cùng lúc chốt mục tiêu và sát thương. Đòn đã chốt vẫn trúng dù sau đó ai đổi PK, mục tiêu chạy vào khu an toàn hay hai người vào chung tổ đội | Giữ luật ba mốc "đòn đã ra thì trúng" và luật khu an toàn của đòn quái (spec ba mốc mục 3.3) |
| Chiêu diện rộng | Từng người chơi trong vùng đều qua bộ lọc PK; quái và người chơi chung giới hạn số mục tiêu, chung thứ tự (mục tiêu đang khóa trước, rồi gần tâm vùng nhất, bằng nhau thì id nhỏ hơn) | Game-design 2.5 và 2.6 |
| Mục tiêu được nêu trong chiêu là người không đánh được | `single`, `circle_target` (cần mục tiêu): báo lỗi. `cone`, `line`, `circle_self`: coi như không nêu mục tiêu | Client có thể chưa biết mục tiêu vừa tắt PK; chiêu không cần mục tiêu vẫn ra theo hướng |
| Phạt EXP | Chỉ ở map `training`; nạn nhân đang bật PK lúc chết; đòn kết liễu là của người chơi (kể cả khi người đó đã chết hay rời zone trước khi đòn trúng). Mất `floor(5% × EXP cần lên cấp kế tiếp)`, EXP được âm; cấp 90 không mất (không có cấp kế tiếp); lưu ngay | Game-design 5.7; "người kết liễu là người có đòn trúng làm HP về 0" như quái (game-design 5.8) |
| Người hạ người khác | Không nhận gì: không EXP, không Xu, không đồ | Game-design không có thưởng PK; thưởng sẽ làm nảy sinh nuôi nhân vật phụ để giết lấy thưởng |
| Thông báo chết | Gói riêng `DeathNotice` cho nạn nhân ở mọi lần chết: ai hạ (tên, là người chơi hay quái), EXP đã mất | Bảng chết hiện "Bị … hạ gục" và "Mất N EXP"; không đoán bằng hiệu EXP |
| Ứng viên chọn mục tiêu (client) | Quái còn sống và người chơi đánh được theo bảng PK (không ở khu an toàn, không cùng tổ đội ở 5b). Chọn gần nhất, đổi mục tiêu, tự chuyển khi mục tiêu chết đều dùng chung danh sách này | Game-design 2.5 "ưu tiên mục tiêu hợp lệ gần nhất"; mục tiêu hợp lệ theo bảng 5.7 |
| Mục tiêu người chơi thôi đánh được | Client bỏ khóa (như mục tiêu ra ngoài 8 ô) | Không gửi lệnh chắc chắn bị từ chối |
| Chạm vào người chơi | Người đánh được thì khóa làm mục tiêu như quái; người không đánh được thì không khóa (5b mở thẻ người chơi để mời tổ đội) | Không lẫn thao tác tấn công với thao tác xã hội |
| Phản công | Bị người chơi đánh trúng khi đang không khóa mục tiêu thì client tự khóa người đó (không tự đánh) | Game-design 5.7 "phản công ngay"; một lần bấm Đánh là đánh trả |
| Hiển thị PK | Tên người đang bật PK màu đỏ; nút PK trên HUD tô đỏ khi bật | Người khác biết ai đánh được mình |
| EXP âm | Thanh EXP rỗng, chữ ghi số âm ("−1.400 / 64.000") | Game-design 5.7 cho EXP âm |
| PvP và quái | Đòn PvP không đổi thù, không đổi bảng sát thương tính EXP của quái | Hai hệ thống độc lập |
| Hồi chiêu giao tranh khi chuyển zone | Thời gian còn lại của khóa tắt PK đi qua bản trao tay (khóa `pvp_combat`) | Không đổi khu để tắt PK ngay |

### 1.4. Bổ sung cho game-design

Game-design không trái với spec này. Các điểm game-design chưa nói, spec này chốt và sẽ ghi vào game-design (mục 9): tắt PK bị khóa 10 giây sau đòn PvP cuối; bật PK không cần điều kiện và làm được trong khu an toàn; luật PK kiểm tra lúc bấm; phó bản v1 cấm PK; cấp 90 không mất EXP; người hạ không nhận thưởng; client tự khóa người vừa đánh mình.

### 1.5. Không làm ở 5a

Tổ đội và luật đồng đội (5b); ác danh, điểm PK, lính canh, thưởng hạ người; map chiến sự, lôi đài, phó bản có luật PK riêng; né, trượt, chí mạng, kháng; auto-play tự phản công (Giai đoạn 6 dùng chung bộ lọc này); chat và bảng tin hạ gục; chế độ PK nhiều mức (chỉ hai mức bật/tắt).

## 2. Dữ liệu

Không có file dữ liệu mới. Luật PK theo loại map khai báo trong `internal/mapdata`:

| Loại map (`type`) | Đánh người | Phạt EXP do PK |
| --- | --- | --- |
| `town` | Không | Không |
| `training` | Có, ngoài khu an toàn | Có |
| `battle` | Có, ngoài khu an toàn | Không |
| `instance` | Không (v1) | Không |

Năm map của 4a là một `town` và bốn `training`. Client đọc trường `type` sẵn có trong file map đã đồng bộ.

## 3. Lưu trữ

Migration `00008_pk_mode.sql`: `characters.pk_on boolean NOT NULL DEFAULT false`. Down xóa cột.

- `store.Character.PkOn` nạp cùng các cột nhân vật (`ListCharacters` và `GetCharacter` dùng chung câu chọn cột).
- `store.CharState.PkOn` được ghi ở mọi lần lưu.
- Cột `exp` không có ràng buộc dấu từ 00001, nên EXP âm lưu được; spec này không đổi gì ở đó.

## 4. Server

### 4.1. Trạng thái

- `PlayerInfo.PkOn` (từ `Character.PkOn`); người chơi trong zone giữ `pkOn` và `pvpUntil` (tick đầu tiên được tắt PK lại; mỗi đòn PvP trúng mình hoặc do mình gây đặt nó bằng tick trúng + 10 giây).
- Bản trao tay (4a mục 3.2.1) mang `PkOn` và mục hồi chiêu `pvp_combat` = thời gian còn lại của khóa 10 giây (bỏ khi đã hết).
- `zone.Area` thêm `PvP bool` và `PkPenalty bool`, lấy từ loại map lúc dựng area (`mapdata.Map.PvP()`, `mapdata.Map.PkPenalty()`).

### 4.2. Kiểm tra mục tiêu là người chơi

Hàm dùng chung cho chém và chiêu, với người đánh A và mục tiêu B (người chơi), chạy sau khi A đã qua các bước `DEAD` và `IN_SAFE_ZONE` của chính A. Lỗi đầu tiên được trả:

1. B là A, không có trong zone, hoặc đã chết: `INVALID_TARGET`.
2. Map không cho đánh người (`Area.PvP` sai): `PK_FORBIDDEN`.
3. B đứng trong khu an toàn: `IN_SAFE_ZONE`.
4. `combat.PvPAllowed(A.pkOn, B.pkOn, cùng tổ đội)` sai: `PK_FORBIDDEN`. Ở 5a "cùng tổ đội" luôn sai.

`combat.PvPAllowed(attackerPK, targetPK, sameParty bool) bool` = `!sameParty && (attackerPK || targetPK)`.

Id mục tiêu tra trong quái trước, rồi người chơi; id của world là duy nhất nên không trùng.

### 4.3. Chém mặc định

`AttackReq.target_id` nay có thể là người chơi. Thứ tự kiểm tra:

1. A chết: `DEAD`.
2. A trong khu an toàn: `IN_SAFE_ZONE`.
3. Mục tiêu: quái còn sống thì tiếp như cũ; người chơi thì các bước của mục 4.2; không phải cả hai: `INVALID_TARGET`.
4. Ngoài tầm chém + dung sai hoặc bị che: `OUT_OF_RANGE`.
5. Chưa hết hồi chém: `ON_COOLDOWN`.

Hợp lệ: như với quái, sát thương chốt lúc bấm (mục 4.5), trúng sau `SlashWindup`.

### 4.4. Chiêu

Thứ tự kiểm tra của Giai đoạn 2 giữ nguyên; phần chọn mục tiêu (`aim`) đổi như sau, với mục tiêu được nêu `target_id`:

- **Mục tiêu được nêu** là quái còn sống, hoặc người chơi qua được mục 4.2, thì là mục tiêu chính hợp lệ.
- **`single`, `circle_target`**: cần mục tiêu chính. Không có: nếu id là người chơi thì trả đúng lỗi của mục 4.2, còn lại `INVALID_TARGET`. Rồi tầm và tầm nhìn như cũ (`OUT_OF_RANGE`). Lao tới (`dash`) lên người chơi dùng cùng điểm đáp như với quái.
- **`cone`, `line`**: mục tiêu chính hợp lệ cho hướng; người chơi không qua mục 4.2 bị coi như không nêu, dùng `dir`; không có cả hai thì `BAD_REQUEST` như cũ.
- **`circle_self`**: mục tiêu chính hợp lệ chỉ được xếp trước; người chơi không hợp lệ bị bỏ qua.

**Nạn nhân** (`victims`): ngoài quái như cũ, mọi người chơi C khác A, còn sống, nằm trong vùng, không bị che từ gốc vùng, và qua các bước 2–4 của mục 4.2 (map cho PvP, C ngoài khu an toàn, `PvPAllowed`). Quái và người chơi xếp chung: mục tiêu chính trước nếu nằm trong vùng, rồi theo khoảng cách tới tâm vùng, bằng nhau thì id nhỏ hơn; lấy tối đa `max_targets` của cấp chiêu. `Cast.impacts` liệt kê cả người chơi.

### 4.5. Sát thương PvP

Với cơ sở `base` (chém: `combat.SlashBase`; chiêu: `combat.SkillBase` như cũ), cấp người đánh L và thủ D của mục tiêu (thủ vật lý nếu đòn vật lý, thủ phép nếu đòn phép):

`combat.PvPDamage(base, D, L) = max(1, floor(base × (1 − D / (D + 20 × L)) × PvPFactor))`, `PvPFactor = 1,0`.

Sát thương chốt lúc bấm bằng chỉ số và cấp của A, thủ của B tại tick bấm. Kiểm tra mốc tham chiếu (game-design 2.2): hai Viêm cấp 10 không đồ, 60 STR/30 VIT: chém 128, Bất Tri Hỏa cấp 1 gây 192, 9 Bất Tri Hỏa hạ 1.640 HP.

### 4.6. Áp đòn lên người chơi

Hàng đợi đòn (spec ba mốc mục 3.1) thêm loại đòn người chơi → người chơi, mang tên người đánh. Tại tick trúng:

- Mục tiêu đã rời zone hoặc đã chết: bỏ đòn, không có `Hit`.
- Ngược lại: trừ máu, phát `Hit{attacker, target, damage, hp_after, skill_id}` (đòn của người chơi nay cũng mang `skill_id`). `pvpUntil` của mục tiêu và của người đánh (nếu còn trong zone) đặt bằng tick này + 10 giây.
- HP về 0: mục tiêu chết (mục 4.7), đòn này là đòn kết liễu; các đòn khác trúng sau đó bị bỏ.
- Không đụng tới quái: không thù, không bảng sát thương, không EXP.

### 4.7. Chết, phạt EXP và thông báo

Khi người chơi B chết (do quái hay người), ngoài những gì `killPlayer` đã làm:

1. Ghi người kết liễu: id, tên (tên quái hoặc tên người chơi lấy từ đòn), và là người chơi hay không.
2. **Phạt EXP** khi cả ba đúng: đòn kết liễu là của người chơi; `Area.PkPenalty`; B đang bật PK tại tick chết. Khi đó `mất = combat.PkExpLoss(Exp.ToNext(cấp B))` = `floor(ToNext × 5 / 100)`; `exp −= mất` (được âm, không hạ cấp); xếp một lần lưu ngay; `SelfStats` mới gửi trong tick đó.
3. Gửi riêng B `DeathNotice{killer_id, killer_name, by_player, exp_lost}` ngay sau `WorldTick` của tick chết, trước `SelfStats`.

Ví dụ: cấp 10 cần 64.000 EXP thì mất 3.200; cấp 1 cần 130 thì mất 6; cấp 90 mất 0. Hồi sinh tại chỗ hay về làng (4c) không hoàn EXP. Người kết liễu không nhận gì.

### 4.8. Bật, tắt PK

`SetPkReq{on}`, xử lý trong `drain`:

1. Nhân vật chết: `DEAD`.
2. `on` bằng trạng thái hiện tại: trả `SetPkResp{on}`, không đổi gì.
3. Tắt PK (`on` sai) khi tick hiện tại < `pvpUntil`: `IN_PVP_COMBAT`.

Hợp lệ: đổi `pkOn`, trả `SetPkResp{on}`, `SelfStats` mới (có `pk_on`) và một `LookChange` cho người thấy nhân vật và chính họ. Không lưu ngay.

### 4.9. Hiển thị cho người khác

- `EntityEnter.pk_on` của người chơi là trạng thái hiện tại.
- Từ 5a, `LookChange` mang **toàn bộ** ngoại hình của thực thể ở cuối tick: `weapon_enhance` (3b) và `pk_on`. Zone chỉ ghi id thực thể đổi ngoại hình trong tick (như `hpChanges`), rồi điền giá trị lúc gửi. Client áp cả hai trường.

### 4.10. Thứ tự tick

Không đổi. `SetPkReq` xử lý trong `drain` như cộng điểm tiềm năng; đòn PvP tạo trong `resolveAttacks` và trúng trong `runImpacts`.

## 5. Giao thức

`ProtocolVersion` lên **12**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 105 | `SetPkReq` | `bool on = 1` |
| 106 | `SetPkResp` | `bool on = 1` |
| 107 | `DeathNotice` | `uint64 killer_id = 1`, `string killer_name = 2`, `bool by_player = 3`, `int64 exp_lost = 4` |

- `DeathNotice` đẩy với `seq = 0`, chỉ cho người vừa chết.
- `EntityEnter.pk_on = 15` (`bool`); `LookChange.pk_on = 3` (`bool`) và ghi chú "the entity's whole look at the end of the tick"; `SelfStats.pk_on = 25` (`bool`).
- Ghi chú của `AttackReq.target_id` và `CastReq.target_id`: có thể là người chơi. Ghi chú của `INVALID_TARGET`: không có, đã chết, là chính mình hoặc không phải quái hay người chơi. Ghi chú của `Cooldown.key` thêm `"pvp_combat"`.
- `ErrorCode`: `PK_FORBIDDEN = 45` (mục tiêu là người chơi mà luật PK, loại map hoặc tổ đội không cho đánh), `IN_PVP_COMBAT = 46` (chưa thể tắt PK vì vừa giao tranh với người chơi).
- Fixture mới: `set_pk_req.hex`, `death_notice.hex` (hạ bởi người chơi, mất 3.200 EXP), `entity_enter_pk.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/pk_rules.gd`** (mới): `can_attack(my_pk, their_pk, map_pvp, their_in_safe, same_party := false) -> bool` theo mục 4.2; `map_pvp(map_type) -> bool` (`training`, `battle`).
- **`core/map_grid.gd`**: đọc `type` thành `map_type`.
- **`core/entity_table.gd`**: mục người chơi lưu `pk_on` từ `EntityEnter`; áp `looks` với cả `weapon_enhance` và `pk_on`, hiệu ứng `{"type": "look", id, weapon_enhance, pk_on}`; `self_pk` lấy từ `SelfStats`; `live_players() -> Array[int]`.
- **`core/targeting.gd`**: ứng viên là `{id, pos, kind}`; mọi hàm chọn giữ nguyên trên danh sách ứng viên đã lọc. Thêm `on_hurt_by(attacker_id, candidates)`: chưa khóa mục tiêu và người đánh có trong ứng viên thì khóa người đó.
- **`core/errors.gd`**: PK_FORBIDDEN "Không thể tấn công người này", IN_PVP_COMBAT "Đang giao tranh, chưa thể tắt PK". **`core/wire.gd`**: `SetPkResp`, `DeathNotice` vào `PAYLOADS`.

### 6.2. Màn World

- **Ứng viên** (`screens/world_entities.gd`): quái còn sống và người chơi còn sống có `pk_rules.can_attack(self_pk, pk_on, map_pvp, ở khu an toàn)` đúng. `screens/world.gd` chỉ truyền ngữ cảnh (PK của mình, loại map, hàm kiểm tra khu an toàn).
- **Chạm**: `pick_at` trên danh sách ứng viên, nên người không đánh được không bị khóa.
- **Phản công**: hiệu ứng `hit` có `target` là mình và `attacker` là người chơi thì gọi `targeting.on_hurt_by`.

### 6.3. Giao diện

- **Nút PK** (`ui/hud.gd`): nút tròn nhỏ chữ "PK" cạnh nút Đổi mục tiêu; viền xám khi tắt, nền đỏ khi bật. Bấm gửi `SetPkReq{on: !hiện tại}`; trả lời thì hiện "Đã bật PK" hoặc "Đã tắt PK"; lỗi thì hiện chữ lỗi.
- **Tên** (`ui/entity_view.gd`): người chơi bật PK có tên màu đỏ, kể cả chính mình; tắt thì như cũ.
- **Bảng chết** (`ui/death_panel.gd`): dưới "Bạn đã gục ngã" thêm "Bị <tên> hạ gục" (tên rỗng thì "Bị người chơi hạ gục") và, khi `exp_lost` > 0, "Mất N EXP" màu đỏ. Hai nút hồi sinh của 4c giữ nguyên.
- **Thanh EXP**: EXP âm thì thanh rỗng, chữ ghi số âm.

### 6.4. Smoke

`tests/smoke/smoke.gd` sau các bước cũ: gửi `SetPkReq{on: true}`, kiểm tra `SetPkResp.on` và `SelfStats.pk_on`; gửi `SetPkReq{on: false}` (chưa giao tranh nên được). In "SMOKE OK" kèm "PK on/off".

## 7. Lệnh dev

Không có lệnh mới. Thử dùng `devchar -level` và `devchar -map` sẵn có.

## 8. Kiểm thử

**Server** (`go test -race`, mỗi package một hoặc hai file test, chia section):

- `combat`: bảng `PvPAllowed` đủ tám trường hợp (hai PK × hai PK × cùng tổ đội); `PkExpLoss` với 64.000 → 3.200, 130 → 6, 0 → 0; `PvPDamage` cho mốc tham chiếu: chém 128, Bất Tri Hỏa cấp 1 192, Viêm đánh Thủy cùng cấu hình 8 Bất Tri Hỏa hạ 1.430 HP; `PvPDamage` không dưới 1.
- `mapdata`: `PvP()` và `PkPenalty()` theo bảng mục 2 cho bốn loại.
- `store` (DB): migration lên rồi xuống; nhân vật cũ có `pk_on` sai; lưu rồi nạp lại `pk_on` đúng; EXP âm lưu và nạp được.
- `world`: `Enter` đưa `PkOn` vào zone; qua cổng giữ `PkOn`.
- `zone` chém người: từng mã lỗi theo thứ tự mục 4.3 (chính mình `INVALID_TARGET`; map `town` thử bằng area không PvP `PK_FORBIDDEN`; mục tiêu trong khu an toàn `IN_SAFE_ZONE`; hai người tắt PK `PK_FORBIDDEN`); A bật PK chém B tắt PK được, B tắt PK chém A được; sát thương dùng thủ vật lý của B; `Hit` tới cả hai.
- `zone` chiêu người: `single` lên người không hợp lệ trả lỗi của mục 4.2; `cone` nêu người không hợp lệ vẫn ra theo `dir`; chiêu phép dùng thủ phép; vòng quanh mình trúng hai quái và một người bật PK nhưng bỏ người tắt PK khi người dùng chiêu tắt PK; giới hạn 2 mục tiêu ở cấp chiêu 1 lấy người gần tâm hơn dù là quái hay người; người trong khu an toàn không bị trúng; lao tới người chơi đáp cách 1 ô.
- `zone` áp đòn: mục tiêu chạy vào khu an toàn sau khi bấm vẫn dính; mục tiêu tắt PK sau khi bấm vẫn dính; mục tiêu rời zone thì bỏ đòn; hai đòn cùng tick, đòn đầu hạ thì đòn sau bị bỏ; đòn PvP không làm quái đổi mục tiêu.
- `zone` phạt EXP: B bật PK bị người hạ ở area `PkPenalty` mất đúng 5% và có một lần lưu; B tắt PK không mất; B bật PK bị quái hạ không mất; area `PvP` nhưng không `PkPenalty` không mất; EXP 1.000 ở cấp 10 thành −2.200; cấp 90 không mất; người đánh đã rời zone khi đòn trúng vẫn tính là người chơi; `DeathNotice` đúng tên, `by_player`, `exp_lost` và chỉ nạn nhân nhận.
- `zone` bật tắt: `DEAD`; cùng trạng thái trả `SetPkResp` không đổi; tắt trong 10 giây sau đòn PvP (gây hoặc nhận) bị `IN_PVP_COMBAT`, sau 10 giây được; `LookChange` tới người xung quanh với `pk_on` và `weapon_enhance` đúng; bản trao tay mang `pk_on` và `pvp_combat`; tắt PK ở zone mới trước khi hết khóa vẫn bị chặn.
- `gateway`: `ProtocolVersion` 12; fixture mới; `SetPkReq` khi chưa vào thế giới bị `NOT_IN_WORLD`.
- e2e `TestPvPOverWebSocket`: A cấp 30 và B cấp 30 (`DevSetLevel`), đặt cạnh nhau ngay ngoài trại Kumotori (`DevSetPlace`); A chém B khi cả hai tắt PK bị `PK_FORBIDDEN`; A bật PK; B nhận `LookChange` với `pk_on`; A chém B, B nhận `Hit`; B (tắt PK) chém lại A được; A tắt PK ngay bị `IN_PVP_COMBAT`; B bật PK, A đánh tới khi B chết: B nhận `DeathNotice` có tên A, `by_player` và `exp_lost` đúng 5% `ToNext(30)`, `SelfStats.exp` giảm đúng số đó; B thoát rồi vào lại, `pk_on` vẫn bật, EXP vẫn là số sau phạt.

**Client** (`tests/run.sh`): `pk_rules` (bảng, loại map); `entity_table` (`pk_on` từ `EntityEnter` và `looks`); `targeting` (ứng viên có người chơi; `on_hurt_by` khóa khi chưa có mục tiêu, không đổi khi đã có); giải mã fixture mới. Smoke theo mục 6.4.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito, Viêm) và `test2` (Hana, Thủy) ở Sagiri: hai người tắt PK không đánh được nhau; Kaito bật PK, tên đỏ, đánh Hana; Hana phản công không cần bật PK; chiêu vòng của Kaito trúng cả quái và Hana; Kaito không tắt được PK ngay sau trận; Hana bật PK rồi bị hạ thì bảng chết ghi "Bị Kaito hạ gục" và "Mất N EXP"; vào thành thì không đánh được nhau.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 2.5 (danh sách ứng viên gồm người chơi đánh được; chạm vào người không đánh được thì không khóa; tự khóa người vừa đánh mình), mục 5.7 (map được PK theo loại; bật PK không điều kiện, cả trong khu an toàn; tắt PK bị khóa 10 giây sau đòn PvP cuối; luật kiểm tra lúc bấm; người kết liễu theo đòn trúng; cấp 90 không mất EXP; người hạ không nhận thưởng; phó bản v1 cấm PK).
- Spec Giai đoạn 0 mục 1.1: Giai đoạn 5 chia 5a/5b.
- `CLAUDE.md`: bất biến "luật đánh người chỉ qua `combat.PvPAllowed` và kiểm tra lúc bấm; phạt EXP chỉ trong `killPlayer` theo đòn kết liễu".
- README hai repo: nút PK, luật PK, phạt EXP.
