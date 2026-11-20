# Giai đoạn 6d — Nhiệm vụ hàng ngày

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần cuối của Giai đoạn 6 (xem bảng chia ở [spec 6a](2026-11-20-giai-doan-6a-bam-di-tim-duong-design.md) mục 1.1). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.2 (nhiệm vụ hàng ngày là nguồn EXP bổ sung), 4.2 (hàng ngày làm mới 00:00; diệt quái, nâng cấp), 5.8 (cửa sổ cấp ±10) và 3.3 (cường hóa); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec 6b](2026-11-20-giai-doan-6b-nhiem-vu-sach-ky-nang-design.md) (nhận, trả, tiến độ, khung theo dõi, lưu) và [spec 3b](2026-11-20-giai-doan-3b-cuong-hoa-design.md) (lệnh cường hóa). 6d chạy sau khi 6c đã triển khai (giao thức 14, migration 00009).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Ở Tổng hành dinh có **Quạ truyền tin** giao ba nhiệm vụ hàng ngày cho nhân vật từ cấp 10: hạ 40 quái hợp cấp, thu 20 "Huyết quỷ" từ quái hợp cấp, và cường hóa 3 lần. Mỗi nhiệm vụ làm được một lần mỗi ngày; 00:00 giờ Việt Nam cả ba làm mới, kể cả với người đang chơi. Thưởng EXP theo phần trăm một cấp của chính nhân vật, Xu theo cấp, và đá cường hóa. Khung theo dõi ghi nhiệm vụ hàng ngày và thời gian còn lại tới lần làm mới; chạm dòng thì đi tới bãi hợp cấp gần nhất hoặc tới Thợ rèn.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Giờ làm mới | **00:00 giờ Việt Nam (UTC+7)** mỗi ngày | Game-design 4.2 chốt 00:00, để mở múi giờ; người chơi mục tiêu ở Việt Nam; Việt Nam không đổi giờ theo mùa nên một độ lệch +7 giờ cố định là đủ, không cần dữ liệu múi giờ |
| Cách tính ngày | `quest.Day(t) = ⌊(giây Unix của t + 25.200) / 86.400⌋`; mục hàng ngày lưu `day` lúc nhận | Một số nguyên so sánh được, dễ thử với đồng hồ giả |
| Bộ nhiệm vụ | Ba nhiệm vụ: `d_kill` "Lệnh diệt quỷ" (hạ 40 quái), `d_collect` "Huyết quỷ" (thu 20, tỷ lệ 40%), `d_enhance` "Rèn luyện" (cường hóa 3 lần) | Game-design 4.2 "diệt quái, phó bản, nâng cấp"; v1 chưa có phó bản; một nhiệm vụ đánh, một thu thập, một rèn |
| Cấp nhận | Từ cấp 10 | Dưới 10 đã có chính tuyến dày; cửa sổ ±10 cần có bãi hợp cấp |
| NPC | Một NPC vai `quest` mới: `qua_truyen_tin` "Quạ truyền tin" ở Tổng hành dinh; giao và trả cùng NPC | Thành là nơi mọi người đi qua; Xa phu và đường đi 6a đưa về nhanh |
| Quái hợp cấp | Mọi quái có cấp chênh cấp nhân vật không quá 10 (cùng luật cửa sổ EXP `combat.ExpEligible`); người kết liễu được tính như 6b | Không cho cày quái cấp 1 để xong nhanh; một luật với EXP |
| Cường hóa | Mỗi **lần thử** trong `EnhanceReq` (thành công, thất bại hay tụt cấp) tính 1; lệnh "Liên tục" thử nhiều lần thì tính đủ số lần đã thử | Đúng chữ "nâng cấp"; không bắt phải thành công |
| Thưởng | `d_kill`, `d_collect`: EXP = 5% EXP lên cấp kế tiếp ở cấp nhân vật **lúc trả** (làm tròn 10 như 6b), Xu = 80 × cấp; `d_enhance`: 5 đá cường hóa và Xu = 40 × cấp | Ba nhiệm vụ mất khoảng 10 phút và cho khoảng 0,1 cấp mỗi ngày; 80 × cấp ≈ Xu của 20 quái; 5 đá ≈ đá rơi từ 25 quái thường; số tạm, cân chỉnh sau chơi thử |
| Cấp tối đa | Ở cấp 90 (không có cấp kế tiếp), phần EXP là 0, Xu và đá giữ nguyên | Như phạt EXP ở 5a |
| Chưa xong khi làm mới | Mục đang làm của ngày cũ bị xóa, tiến độ mất; nhận lại được ngay | Luật thường gặp; không cộng dồn qua ngày |
| Người đang chơi lúc 00:00 | Mỗi zone xét ngày mỗi giây; ngày đổi thì xóa mục hàng ngày cũ của mọi người chơi trong zone và đẩy `QuestLog` mới cho từng người | Khung theo dõi và biểu tượng NPC đúng ngay, không phải vào lại |
| Đồng hồ | `zone.Config.Now` (mặc định `time.Now`), chỉ dùng cho ngày; tick vẫn dùng bộ đếm tick | Thử được bằng đồng hồ giả |
| Đếm ngược | `QuestLog.daily_reset_ms`: số mili giây tới lần làm mới tính lúc gửi; client tự đếm và chờ `QuestLog` mới | Không phải đồng bộ giờ máy client |
| Điểm năng động, rương hàng ngày | Không làm ở v1 | Game-design 4.2 có nhắc nhưng chưa có thiết kế thưởng; thêm sau như một lớp trên ba nhiệm vụ |
| Tổ đội | Không chia tiến độ (6b) | Game-design 4.2, 5.8 |
| Đích khi chạm | `d_kill`, `d_collect`: vùng quái hợp cấp gần nhất (mục 5.3); `d_enhance`: Thợ rèn gần nhất theo lộ trình | Game-design 4.3: đi tới mục tiêu rồi dừng |

### 1.3. Bổ sung cho game-design

Game-design 4.2 để mở múi giờ làm mới (cả ở mục 10.6); spec này chốt UTC+7. Ba nhiệm vụ, thưởng và luật làm mới được ghi vào mục 4.2 (mục 8).

### 1.4. Không làm ở 6d

Điểm năng động, rương mốc; nhiệm vụ hàng ngày theo phó bản, PK hay tổ đội; làm lại nhiều vòng trong ngày; tự nhận khi đăng nhập; làm mới bằng Lượng; nhiệm vụ tuần.

## 2. Dữ liệu

`quest_data.py` thêm vào `data/quests.json` ba nhiệm vụ `line: "daily"` (sau các nhiệm vụ chính tuyến):

```json
{
  "id": "d_kill", "line": "daily", "name": "Lệnh diệt quỷ", "min_level": 10, "requires": "",
  "giver": {"map": "ubuyashiki", "npc": "qua_truyen_tin"},
  "turn_in": {"map": "ubuyashiki", "npc": "qua_truyen_tin"},
  "objective": {"type": "kill_any", "count": 40},
  "text": {"offer": "…", "progress": "…", "done": "…"},
  "reward": {"exp": 0, "xu": 0, "exp_pct": 5, "xu_per_level": 80, "items": [], "equipment": [], "skill_book": 0}
}
```

| id | Tên | Mục tiêu | `exp_pct` | `xu_per_level` | Vật phẩm |
| --- | --- | --- | --- | --- | --- |
| `d_kill` | Lệnh diệt quỷ | `kill_any`, 40 | 5 | 80 | |
| `d_collect` | Huyết quỷ | `collect_any`, 20, `chance_bp` 4.000, `item_name` "Huyết quỷ" | 5 | 80 | |
| `d_enhance` | Rèn luyện | `enhance`, 3 | 0 | 40 | 5 `stone_enhance` |

- Loại mục tiêu mới: `kill_any` (`count`), `collect_any` (`count`, `chance_bp`, `item_name`), `enhance` (`count`). Không có `monster`, `map`, `region`.
- `reward.exp_pct` (0–100) và `reward.xu_per_level` (≥ 0) mới; với nhiệm vụ `main` cả hai bằng 0; với `daily` thì `exp`, `xu`, `skill_book` bằng 0 và `equipment` rỗng.
- `tools/mapgen.py` thêm `qua_truyen_tin` "Quạ truyền tin" vai `quest` ở Tổng hành dinh theo luật vị trí của 4b.

**Server kiểm tra khi nạp** (thêm vào mục 2.6 của 6b): `line` thuộc `main`, `daily`; `daily` có `requires` rỗng, loại mục tiêu thuộc `kill_any`, `collect_any`, `enhance`, các luật thưởng ở trên; `main` không dùng ba loại mục tiêu này; `collect_any` có `chance_bp` 1–10.000 và `item_name`; `count` 1–999.

## 3. Lưu trữ

Không có migration: cột `character_quests.day` có từ 00009. Mục hàng ngày lưu `day` = ngày lúc nhận (giữ nguyên khi trả). `world.Enter` không lọc theo ngày; zone lọc khi người chơi vào (mục 4.2).

## 4. Server

### 4.1. Ngày

- `quest.Day(t time.Time) int32` theo mục 1.2; `quest.ResetIn(t time.Time) time.Duration` = thời gian tới đầu ngày kế tiếp (luôn trong (0, 24 giờ]).
- Zone giữ `day` hiện tại, tính lúc khởi động và **mỗi giây** (mỗi `tickRate` tick) từ `Config.Now()`.

### 4.2. Làm mới

- **Khi vào zone** (vào game và nhận bản trao tay): bỏ mọi mục `daily` có `day` khác ngày của zone. `EnterWorldResp.quests` là nhật ký đã lọc, có `daily_reset_ms`.
- **Khi ngày của zone đổi:** với mỗi người chơi trong zone, bỏ mục `daily` có `day` khác ngày mới, rồi đẩy `QuestLog{quests, daily_reset_ms}` (gửi cả khi người đó không có mục hàng ngày nào, để client đặt lại đếm ngược). Không lưu ngay.

### 4.3. Nhận, tiến độ, trả

Như 6b mục 4.2–4.4, với các điểm khác:

- **Nhận** (bước 5 của 6b): có mục của nhiệm vụ đó (đang làm hoặc đã xong **hôm nay**) thì `QUEST_NOT_AVAILABLE`. Hợp lệ thì mục có `day` = ngày của zone.
- **Tiến độ khi hạ quái** (6b mục 4.3): `kill_any` +1 khi `combat.ExpEligible(cấp người kết liễu, cấp quái)`; `collect_any` cùng điều kiện cộng tung `chance_bp`.
- **Tiến độ khi cường hóa:** trong lệnh `EnhanceReq` (3b), sau khi lặp xong, mỗi mục `enhance` đang làm cộng số lần thử thật sự đã chạy (kẹp ở số lượng); lệnh bị lỗi không cộng. Mục đổi đi vào danh sách đổi của tick như 6b (`QuestProgress`).
- **Trả:** thưởng EXP = `quest.Round10(ExpTable.ToNext(cấp), exp_pct)` (0 ở cấp tối đa); Xu = `reward.xu + xu_per_level × cấp`; vật phẩm như 6b. `QuestTurnInResp.exp`, `.xu` là số thực cộng.

## 5. Giao thức

`ProtocolVersion` lên **15**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 124 | `QuestLog` | đẩy với `seq = 0` khi ngày đổi (mục 4.2) |

- `QuestLog.daily_reset_ms = 2` (`int64`): số mili giây tới 00:00 UTC+7 kế tiếp, tính lúc gửi. Có trong `EnterWorldResp.quests` và trong `QuestLog` đẩy.
- Không có mã lỗi mới.
- Fixture mới: `quest_log.hex` (một mục hàng ngày đang làm, `daily_reset_ms` 3.600.000). Fixture cũ sinh lại.

### 5.1. Đích của nhiệm vụ hàng ngày (client)

- `kill_any`, `collect_any`: trong các vùng quái của các map tới được (`world_graph.reachable`), chọn vùng có mọi cấp nằm trong [L − 10, L + 10] (L là cấp nhân vật), ưu tiên tâm khoảng cấp gần L nhất, rồi ít cổng nhất, rồi thứ tự map trong `reachable`, rồi thứ tự vùng trong file. Không có vùng nào thì dòng không chạm được và ghi "Chưa có bãi hợp cấp".
- `enhance`: NPC vai `blacksmith` của map gần nhất theo số cổng.

## 6. Client Godot

### 6.1. Lõi

- **`core/quest_book.gd`**: đọc ba loại mục tiêu mới và thưởng `exp_pct`, `xu_per_level`; `apply_log` lưu `reset_at` = giờ máy lúc nhận + `daily_reset_ms`; `reset_in(now) -> float`; `reward_of` tính EXP và Xu theo cấp hiện tại (cùng công thức server, `exp_to_next` từ `data/exp.json` đã đồng bộ); `goal_of` cho ba loại mới theo mục 5.1; `tracker` thêm phần hàng ngày (mục 6.2).
- **`core/world_graph.gd`**: `regions_of(map_id)`; `npcs_with_role(role)`.
- **`core/wire.gd`**: payload `quest_log`. `tools/sync_server.sh` chép thêm `data/exp.json` nếu chưa chép.

### 6.2. Giao diện

- **Khung theo dõi** (6b): tối đa 6 dòng; sau phần chính tuyến là phần **Hàng ngày** với tiêu đề "Hàng ngày · làm mới sau h:mm" (đếm theo `reset_in`); mỗi nhiệm vụ hàng ngày đang làm một dòng có tiến độ ("Lệnh diệt quỷ 12/40", "Đã xong — về gặp Quạ truyền tin"); nếu còn nhiệm vụ hàng ngày nhận được thì thêm một dòng "Gặp Quạ truyền tin nhận nhiệm vụ hàng ngày (n)". Dưới cấp 10 không có phần này.
- **Thẻ nhiệm vụ** (6b): mục tiêu "Hạ 12/40 quái chênh không quá 10 cấp", "Thu thập Huyết quỷ 5/20 từ quái chênh không quá 10 cấp", "Cường hóa 1/3 lần"; thưởng ghi số EXP và Xu tính theo cấp hiện tại.
- **Biểu tượng Quạ truyền tin**: theo luật 6b.
- **Thông báo** lúc làm mới: "Nhiệm vụ hàng ngày đã làm mới".

### 6.3. Smoke

`tools/smoke.sh`: tài khoản smoke đã ở cấp 10 (bước 6b), `devchar -map ubuyashiki -x … -y …` cạnh `qua_truyen_tin`, `devitem` đủ đá và Xu, một kiếm Lữ Hành. `tests/smoke/smoke.gd`: `EnterWorldResp.quests.daily_reset_ms` trong (0, 86.400.000]; nhận `d_enhance`; nhận lại bị `QUEST_NOT_AVAILABLE`. In "SMOKE OK" kèm "daily accepted, reset in h:mm".

## 7. Lệnh dev

`devquest` với nhiệm vụ `daily`: `-active` và `-done` đặt `day` là ngày hiện tại theo `quest.Day(time.Now())`. Thêm cờ `-day-offset <n>` (mặc định 0) cộng n ngày vào `day` ghi xuống, để thử mục của ngày cũ.

## 8. Kiểm thử

**Script:** `quest_data.py` kiểm tra ba nhiệm vụ hàng ngày theo mục 2 và in EXP thưởng ở cấp 10, 30, 50 (hiện 4.580; 545.080; 5.179.280 mỗi nhiệm vụ có `exp_pct` 5).

**Server** (`go test -race`):

- `gamedata`: dữ liệu thật nạp được (28 nhiệm vụ); từ chối `daily` có `requires`, `daily` dùng `kill`, `main` dùng `kill_any`, `daily` có `exp` khác 0, `main` có `exp_pct`, `collect_any` thiếu `item_name`, `line` lạ; `qua_truyen_tin` có ở `ubuyashiki`.
- `quest`: `Day` ở 16:59:59 UTC và 17:00:00 UTC khác nhau một ngày; `ResetIn` ở 17:00:00 UTC là 24 giờ, ở 16:00:00 UTC là 1 giờ; `Available` của nhiệm vụ hàng ngày theo cấp 10 và theo mục cùng ngày; `kill_any` chỉ tăng khi chênh cấp ≤ 10; `collect_any` với bộ ngẫu nhiên giả; thưởng EXP ở cấp 30 là 545.080, ở cấp 90 là 0.
- `zone` (đồng hồ giả): nhận `d_kill` ghi ngày của zone; nhận lại cùng ngày bị `QUEST_NOT_AVAILABLE`; đồng hồ qua 00:00 UTC+7 thì trong một giây mục đang làm bị xóa và người chơi nhận `QuestLog` với `daily_reset_ms` khoảng 86.400.000 (sai số một giây); mục đã xong hôm qua cũng bị xóa và nhận lại được; người vào zone với mục hôm qua thấy nhật ký đã lọc; `EnhanceReq` với `times` 5 dừng ở lần thành công thứ nhất cộng 1, lệnh lỗi không cộng; quái cấp 1 do nhân vật cấp 20 hạ không tính `kill_any`; trả `d_enhance` ở cấp 12 cho 5 đá và 480 Xu; mục hàng ngày đi qua bản trao tay và bị lọc nếu zone mới đã sang ngày khác.
- `gateway`: `ProtocolVersion` 15; fixture mới.
- `cmd/devquest`: `-active d_kill` ghi `day` hôm nay; `-day-offset -1` ghi hôm qua.
- e2e `TestDailyQuestsOverWebSocket`: nhân vật cấp 10 (store), có kiếm Lữ Hành, đủ đá và Xu, đặt cạnh `qua_truyen_tin`; `daily_reset_ms` trong (0, 86.400.000]; nhận `d_enhance`; thoát, đặt cạnh `tho_ren`, vào lại; ba `EnhanceReq` (`times` 1) đưa tiến độ lên 3 qua `QuestProgress`; thoát, đặt cạnh `qua_truyen_tin`, vào lại; trả được (5 đá, 400 Xu); nhận lại bị `QUEST_NOT_AVAILABLE`.

**Client** (`tests/run.sh`): `quest_book` (ba loại mới; `reward_of` theo cấp khớp số của server; `reset_in` đếm ngược; `goal_of` chọn vùng hợp cấp đúng luật mục 5.1 ở cấp 12 và cấp 25, không có vùng ở cấp 60; `enhance` tới Thợ rèn Ubuyashiki; `tracker` có phần hàng ngày từ cấp 10); giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` nhận ba nhiệm vụ ở Quạ truyền tin; chạm "Lệnh diệt quỷ" thì tự đi tới bãi hợp cấp; cường hóa ba lần ở Thợ rèn; trả nhiệm vụ và thấy EXP, Xu, đá; khung theo dõi đếm ngược tới 00:00. (Thử làm mới bằng server dev với đồng hồ thật là không thực tế; lần làm mới được kiểm tra ở test zone với đồng hồ giả.)

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 4.2 (ba nhiệm vụ hàng ngày v1, thưởng, làm mới 00:00 UTC+7, mục chưa xong bị xóa), mục 2.2 (ghi chú nguồn EXP hàng ngày), mục 10.6 (bỏ "múi giờ reset nhiệm vụ").
- Spec Giai đoạn 0 mục 1.1: Giai đoạn 6 xong khi 6d triển khai.
- `CLAUDE.md`: ngày nhiệm vụ đổi lúc 00:00 UTC+7 (`quest.Day`); zone chỉ dùng `Config.Now` để tính ngày.
- README hai repo: nhiệm vụ hàng ngày, Quạ truyền tin, `devquest -day-offset`.
