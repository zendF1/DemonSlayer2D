# Quái thường — vùng, lang thang và chưởng

Ngày: 2026-11-20. Trạng thái: Đã triển khai (2026-11-20); còn chờ người dùng chơi thử hai cửa sổ. Các điểm làm rõ thêm lúc triển khai đã ghi thẳng vào mục 4.2–4.5.

Tài liệu này sửa phần quái của [spec Giai đoạn 1](2026-11-19-giai-doan-1-chien-dau-pve-design.md) cho cả server (repo này) lẫn client Godot (`../DemonSlayer2DGame`). Nó thay mục 2.1 (bãi quái), 2.2 (trường của loại quái) và 4.2 (AI quái) của spec đó. Mọi phần khác của Giai đoạn 1 giữ nguyên.

## 1. Mục tiêu và phạm vi

### 1.1. Vấn đề

Quái hiện đứng thành 4 bãi, mỗi bãi 12 con trong bán kính 5 ô, nên trông đông và túm tụm. Quái cũng chỉ có một kiểu phản ứng: thấy người là lao vào cắn.

### 1.2. Mục tiêu

- Quái rải đều theo vùng và lang thang khắp vùng của nó.
- Quái chết thì hồi sinh tại một điểm cố định của vùng, gọi là ổ quỷ.
- Quái có hai kiểu phản ứng:
  - người chơi chỉ đi qua vùng phát hiện thì quái đứng lại bắn chưởng;
  - người chơi đánh quái thì quái đuổi theo và đánh cận chiến.

### 1.3. Quyết định đã chốt

| Câu hỏi | Chốt |
|---|---|
| Rải quái | Chia map thành vùng, mỗi vùng một loại quái và một khoảng cấp; quái rải đều trong vùng |
| Phạm vi đi lại, hồi sinh | Quái chỉ đi trong vùng; chết thì hồi sinh tại ổ quỷ cố định của vùng rồi lang thang ra |
| Chưởng | Luôn trúng, server tính ngay lúc bắn; client vẽ viên chưởng bay 0,3 giây |
| Đuổi | Bị đánh thì đuổi và đánh cận chiến, không bắn lúc đuổi; bỏ đuổi thì **không hồi máu** |
| Chống lách (bổ sung sau review) | Lúc đuổi quái chạy 1,2 lần tốc độ người chơi; mục tiêu trốn vào khu an toàn thì quái chờ ở mép 5 giây rồi mới bỏ |
| Chống lách, vòng 2 (sau review lần hai) | Tầm đánh cận chiến của quái bằng tầm chém thực của người chơi (2,0 ô); ra quá biên vùng phải 2 giây mới được bỏ; đang chờ ở mép mà có người khác vừa đánh thì quay sang người đó; quái nhớ người đã đánh nó trong 10 giây và lao vào đuổi ngay khi thấy, không bắn |
| Sức chưởng | 50% sát thương cận chiến, mỗi 2 giây, tầm 5 ô, đứng yên khi bắn |
| Khai báo vùng | Danh sách hình chữ nhật trong JSON của map |

### 1.4. Không làm

- Boss (Hạ Huyền, Thượng Huyền, Quỷ Vương), tinh anh, thủ lĩnh. Chúng sẽ có AI riêng, không dùng thiết kế này.
- Chưởng né được (vật thể bay thật).
- Vẽ ranh giới vùng trên map.
- Map khác ngoài `kumotori_1`.

## 2. Dữ liệu

### 2.1. Vùng quái trong JSON của map

Trường `camps` bị bỏ, thay bằng `regions`. Mỗi vùng có:

| Trường | Ý nghĩa |
|---|---|
| `id` | tên vùng, duy nhất trong map |
| `monster` | id loại quái trong `data/monsters.json` |
| `min_level`, `max_level` | khoảng cấp; mỗi lần sinh, cấp được chọn ngẫu nhiên đều trong khoảng |
| `count` | số quái của vùng |
| `rects` | danh sách hình chữ nhật `{x, y, w, h}` tính theo ô, cùng quy ước với `safe_zones` (phủ các ô từ `x` đến `x + w − 1`) |
| `nest` | điểm `{x, y}` theo toạ độ ô (thường là tâm ô, ví dụ 22,5) |

**Ô của vùng** là các ô đi được nằm trong ít nhất một hình chữ nhật của vùng, trừ ô thuộc khu an toàn và ô kề khu an toàn (khu an toàn nới thêm 1 ô mỗi phía). Các hình chữ nhật trong cùng một vùng được phép chồng nhau.

Kiểm tra khi nạp map (lỗi thì server không khởi động):

- mỗi hình chữ nhật nằm trong map và có `w`, `h` dương;
- `monster` có trong dữ liệu quái, `1 ≤ min_level ≤ max_level`, và cấp có trong bảng cấp;
- `count ≥ 1` và không lớn hơn số ô của vùng;
- ổ quỷ nằm trên một ô của vùng (đi được, không thuộc khu an toàn);
- hai vùng không chung ô nào;
- `id` không trùng.

### 2.2. Bố trí `kumotori_1`

Map rộng 64 × 48. Bức tường ngang ở hàng 20 có khe ở x 1–9, 25–27 và 41–62. Khu an toàn ở x 1–10, y 1–10. Khối đá 4 × 4 ở x 45–48, y 30–33.

| Vùng | `rects` | Quái | Cấp | Số con | Ổ quỷ |
|---|---|---|---|---|---|
| `r1` Chân núi | `{12,1,20,19}`, `{1,12,11,8}` | `wanderer` | 1–2 | 8 | (22,5; 10,5) |
| `r2` Sườn đông | `{32,1,31,19}` | `wanderer` | 2–3 | 7 | (47,5; 9,5) |
| `r3` Thung tuyết | `{1,21,31,26}` | `snow_ghoul` | 3–4 | 7 | (15,5; 34,5) |
| `r4` Bãi đá | `{32,21,31,26}` | `snow_ghoul` | 4–5 | 8 | (52,5; 40,5) |

Tổng 30 con. Cấp tăng dần khi càng xa trại.

### 2.3. `data/monsters.json`

File vẫn sinh từ `docs/balance/monster_stats.py` (sửa script, không sửa JSON). Trường của mỗi loại quái:

| Trường | Giá trị | Ghi chú |
|---|---|---|
| `attack_range` | 2,0 | tăng từ 1,5: bằng tầm chém thực của người chơi (1,5 + độ trễ 0,5), để người chơi không đứng ngoài tầm quái mà vẫn chém được |
| `attack_sec` | 1,5 | nhịp đánh cận chiến, giữ nguyên |
| `speed` | 0,8 | hệ số so với tốc độ người chơi, giữ nguyên |
| `respawn_sec` | 8 | giữ nguyên |
| `detect_range` | 5 | thay `aggro_range` (4) |
| `chase_range` | 10 | thay `leash_range`; tính từ chỗ bắt đầu đuổi |
| `shoot_sec` | 2 | mới: nhịp bắn chưởng |
| `shoot_windup_sec` | 0,5 | mới: độ trễ của viên đầu |
| `region_margin` | 3 | mới: mục tiêu ra khỏi vùng quá số ô này thì quái bỏ đuổi |
| `chase_speed` | 1,2 | mới: hệ số so với tốc độ người chơi khi đang đuổi |
| `safe_wait_sec` | 5 | mới: thời gian quái chờ ở mép khi mục tiêu ở trong khu an toàn |
| `margin_grace_sec` | 2 | mới: mục tiêu phải ở ngoài biên vùng liên tục chừng này mới bị bỏ |
| `grudge_sec` | 10 | mới: quái nhớ người đã đánh nó trong chừng này giây |

Kiểm tra dữ liệu: các trường thời gian và khoảng cách đều dương; `region_margin ≥ 0`. Bảng HP, công, thủ theo cấp giữ nguyên.

Ô AOI phải lớn hơn hoặc bằng `detect_range` lớn nhất (thay cho `aggro_range` trong kiểm tra hiện có).

## 3. Luật

### 3.1. Sát thương chưởng

`internal/combat` thêm `RangedFactor = 0,5` và `MonsterRangedDamage(base, defense, attackerLevel)` = max(1, ⌊`Damage(base, defense, attackerLevel)` × 0,5⌋). Đầu vào giống đòn cận chiến của quái.

- Chưởng luôn trúng, không chí mạng. Server trừ HP ngay trong tick quái bắn.
- Chưởng có thể giết người chơi. Luồng chết và hồi sinh giữ như Giai đoạn 1.
- Người chơi đứng trong khu an toàn không bị bắn.

### 3.2. Người chơi hợp lệ cho quái

Giữ như Giai đoạn 1: còn sống, không đứng trong khu an toàn, cùng zone. Thêm điều kiện cho **bắn**: cách quái không quá `detect_range` và đoạn thẳng giữa hai bên không qua ô bị chặn (`lineBlocked`).

## 4. AI quái thường

Bốn trạng thái: **Lang thang**, **Bắn**, **Đuổi**, **Về vùng**. Quái vẫn dùng chung hàm di chuyển của người chơi (hộp 0,6 ô, trượt dọc tường), và vẫn coi ô khu an toàn là bị chặn.

### 4.1. Sinh và hồi sinh

- **Lúc zone khởi động:** đặt `count` con cho mỗi vùng bằng cách "ứng viên tốt nhất". Với mỗi con, lấy ngẫu nhiên 8 ô của vùng, rồi chọn ô xa nhất so với các con đã đặt (theo khoảng cách nhỏ nhất). Quái đứng ở tâm ô. Ngẫu nhiên lấy từ `Config.Rand` của zone, nên test lặp lại được.
- **Quái chết:** xác nằm 1 giây (như cũ). Sau `respawn_sec` kể từ lúc chết, một con mới sinh tại ổ quỷ của vùng với đầy HP, cấp chọn lại trong khoảng, và dữ liệu chia EXP theo sát thương được đặt lại.

### 4.2. Lang thang

- Quái đứng nghỉ ngẫu nhiên 2–5 giây, rồi chọn một ô ngẫu nhiên của vùng cách vị trí hiện tại không quá 6 ô và đi tới đó với nửa tốc độ.
- Chỉ chọn ô mà đoạn thẳng từ quái tới tâm ô chỉ đi qua ô của vùng (dùng `lineBlocked` trên lưới coi ô ngoài vùng là bị chặn). Nhờ vậy quái không băng qua dải 1 ô quanh khu an toàn ở góc trong của vùng hình chữ L như `r1`, và không lao vào tường giữa vùng.
- Tới nơi thì nghỉ tiếp. Nếu bị kẹt (hàm đi trả về "stuck") thì nghỉ rồi chọn ô khác.
- **Nhớ thù:** mỗi tick (ở **Lang thang** và **Bắn**), nếu có người đã đánh quái trong `grudge_sec` (10 giây), hợp lệ, đứng trong vùng đã nới `region_margin` ô, trong `detect_range` và không bị tường che, thì quái vào **Đuổi** người gần nhất trong số đó ngay, không bắn và không chờ độ trễ. Điều này áp dụng cả sau khi quái bỏ cuộc vì mục tiêu trốn trong khu an toàn (mọi lối ra khỏi khu an toàn đều nằm trong biên vùng).
  - Điều kiện biên vùng giữ cho quái không bị kéo ra ngoài: người đứng ngoài biên thì quái chỉ bắn, không đuổi lại. Thiếu điều kiện này, quái bỏ đuổi vì biên rồi tick sau lại đuổi đúng người ấy (vẫn trong 5 ô vì quái chạy nhanh hơn), và cứ thế bị dắt sang vùng khác.
  - Quái **tha** cho người mà nó vừa bỏ đuổi vì quá `chase_range`: người đó bị xoá khỏi danh sách người đã đánh, nên nhớ thù và mục tiêu dự phòng không chọn lại cho tới khi người đó đánh nó lần nữa.
- Không có ai như vậy thì quái tìm người chơi hợp lệ để bắn (mục 3.2). Có người thì chuyển sang **Bắn**.

### 4.3. Bắn

- **Khi vào trạng thái:** quái đứng yên. Viên đầu bắn sau `shoot_windup_sec`.
- **Mỗi lần bắn:** quái chọn người chơi hợp lệ gần nhất rồi bắn, tạo một `Hit` với `ranged = true` và sát thương ở mục 3.1. Viên kế tiếp sau `shoot_sec`.
- **Không còn mục tiêu:** đến lượt bắn mà không còn ai hợp lệ thì quái về **Lang thang**, bắt đầu bằng một lần nghỉ.
- Bắn không làm quái đuổi theo ai.

### 4.4. Đuổi

- **Khi vào:** quái bị người chơi đánh trúng thì vào **Đuổi** ngay, dù đang ở trạng thái nào. Mục tiêu là người vừa đánh nó. Quái ghi lại chỗ bắt đầu đuổi.
- **Khi đã đang đuổi:** người khác đánh thì quái vẫn đuổi mục tiêu cũ.
- **Danh sách người đã đánh:** quái nhớ thời điểm gần nhất từng người đánh nó. Danh sách này dùng để chọn mục tiêu dự phòng.
- **Cách đuổi:** quái chạy với `chase_speed` (1,2 lần tốc độ người chơi) tới khoảng dừng như Giai đoạn 1 (min(1,3; reach − 2·eps)). Quái phải nhanh hơn người chơi, vì người chơi với xa hơn (2,0 so với 1,5 ô): bằng tốc độ thì người chơi vừa lùi vừa chém mà quái không bao giờ tới gần.
  - Đánh cận chiến mỗi `attack_sec` khi trong `attack_range` và không bị tường che.
  - Bị tường che thì đi thẳng về phía mục tiêu, trượt quanh góc.
  - Lúc đuổi quái không bắn chưởng.
- **Bỏ mục tiêu** khi xảy ra bất kỳ điều nào:
  - mục tiêu chết hoặc rời zone;
  - mục tiêu ở trong khu an toàn liên tục quá `safe_wait_sec` (5 giây). Trong lúc chờ, quái giữ mục tiêu, đi sát tới mép (lưới của quái vẫn chặn ô khu an toàn) và đánh ngay khi mục tiêu bước ra trong tầm, khi đã hết hồi chiêu. Bộ đếm đặt lại mỗi khi mục tiêu ra khỏi khu an toàn. Trong lúc mục tiêu ở khu an toàn thì bỏ qua điều kiện biên vùng bên dưới. Đang chờ mà có người khác đủ điều kiện dự phòng (đánh nó trong 5 giây, hợp lệ, trong biên) thì quái quay sang người đó ngay;
  - vị trí mục tiêu nằm ngoài mọi hình chữ nhật của vùng sau khi nới mỗi cạnh thêm `region_margin` ô, liên tục quá `margin_grace_sec` (2 giây); quay lại trong biên thì bộ đếm đặt lại. Trong 2 giây đó quái vẫn đuổi và đánh bình thường;
  - quái đã cách chỗ bắt đầu đuổi hơn `chase_range` (quái tha cho người này, mục 4.2);
  - mục tiêu không với tới được: đường thẳng tới mục tiêu bị tường che và quái kẹt (hàm đi trả về "stuck") liên tục 2 giây (`returnStuckSec`). Bộ đếm đặt lại khi quái đi được hoặc khi bắt đầu một lần đuổi mới. Quái tha cho người này (không nhớ thù), rồi chuyển sang người dự phòng nếu có, không thì vào **Về vùng**. Nhờ vậy người thứ hai không chém được quái mãi trong lúc nó đâm vào tường đuổi người đứng sau tường.
- **Sau khi bỏ:** nếu có người khác trong danh sách đánh nó trong 5 giây gần nhất và vẫn hợp lệ, quái chuyển sang người gần nhất trong số đó. Chỗ bắt đầu đuổi đặt lại ở vị trí hiện tại. Không có ai thì quái vào **Về vùng**.
  - "Người khác" không gồm mục tiêu vừa bỏ, kể cả khi người đó vừa đánh nó; nếu không, quái bỏ vì quá `chase_range` sẽ lại chọn đúng người ấy và đuổi mãi.
  - Người dự phòng phải hợp lệ (còn sống, không ở khu an toàn) và đứng trong vùng đã nới `region_margin` ô.
- **Bộ đếm chờ ở khu an toàn** tính từ tick đầu tiên quái thấy mục tiêu trong khu an toàn; đủ `safe_wait_sec` thì bỏ.
- **Khi một người chơi chết:** mọi quái đang nhắm người đó bỏ mục tiêu (như Giai đoạn 1), và xoá người đó khỏi danh sách người đã đánh. Người chơi hồi sinh trong vòng 5 giây vì vậy không bị chọn làm mục tiêu dự phòng chỉ vì một đòn trước khi chết.

### 4.5. Về vùng

- **Đang ở trên ô của vùng:** quái chuyển ngay sang **Lang thang**, ngay trong tick bỏ đuổi và tại chỗ đang đứng (không đi về tâm ô).
- **Ở ngoài vùng:** quái đi với `speed` tới ô gần nhất của vùng.
- **Máu:** quái **không hồi máu**. Dữ liệu chia EXP giữ nguyên.
- **Không bắn:** quái đang về không bắn chưởng, kể cả khi có người trong tầm; về tới ô của vùng và vào **Lang thang** thì mới tìm người để bắn.
- **Nhớ thù:** mỗi tick, trước khi đi tiếp, quái đang về cũng xét nhớ thù như ở **Lang thang** và **Bắn** (mục 4.2): có người đã đánh nó trong `grudge_sec`, hợp lệ, trong biên vùng, trong `detect_range` và không bị tường che thì quái vào **Đuổi** người gần nhất ngay, không chờ độ trễ, và đánh luôn trong tick đó nếu đã trong tầm và hết hồi chiêu. Nhờ vậy người chơi không chọc được quái lúc nó vừa bỏ cuộc chờ ở mép khu an toàn và đang đi về.
- **Bị đánh:** quái đang về vẫn đánh được, bị đánh thì vào lại **Đuổi**. Mã lỗi `INVALID_TARGET` không còn dùng cho quái đang về; mã này vẫn dùng cho mục tiêu không tồn tại hoặc đã chết.
- **Bị kẹt:** kẹt quá 2 giây thì quái dịch chuyển tới ổ quỷ, gửi `EntityMove` với `snap = true`.

### 4.6. Thứ tự trong tick

Giữ như Giai đoạn 1. Đòn của người chơi được giải quyết trước `thinkMonsters`, nên quái bị đánh chuyển sang **Đuổi** ngay trong tick đó. Sau đó `thinkMonsters` chạy theo trạng thái mới.

## 5. Giao thức

- `Hit` thêm `bool ranged = 5`: đòn chưởng của quái. Các đòn khác để `false`.
- `gateway.ProtocolVersion` tăng từ 2 lên 3, vì JSON map đổi `camps` thành `regions` và hành vi quái thay đổi. Client cũ sẽ nhận lỗi phiên bản rõ ràng.
- Thêm fixture `testdata/client` cho một `WorldTick` có `Hit` với `ranged = true`.
- Không thêm message hay mã lỗi mới.

## 6. Server

| Package | Thay đổi |
|---|---|
| `internal/combat` | `RangedFactor`, `MonsterRangedDamage` |
| `internal/gamedata` | trường mới và đổi tên (mục 2.3), kiểm tra giá trị |
| `internal/mapdata` | `Region`, `RegionTiles`, kiểm tra (mục 2.1); bỏ `Camp`, `CampTiles` |
| `internal/world` | dựng vùng cho `zone.Area`; kiểm tra ô AOI theo `detect_range` |
| `internal/zone` | sinh theo vùng (`spawn.go`), bốn trạng thái AI (`monster.go`), cờ `ranged` khi tạo `Hit` |
| `internal/client`, `cmd/botclient` | chế độ `-fight` đi tới ổ quỷ của vùng thay cho tâm bãi |
| `docs/balance/monster_stats.py` | sinh trường mới |
| `maps/kumotori_1.json` | `regions` như mục 2.2 |

## 7. Client

- `core/entity_table.gd`: `Hit` có `ranged` sinh hiệu ứng `projectile` (từ người bắn tới người trúng, kèm sát thương) thay cho hiệu ứng chém.
- `ui/`: thêm hiệu ứng quả cầu chưởng bay 0,3 giây từ quái tới mục tiêu. Tới nơi mới hiện số sát thương. Mục tiêu đã mất view thì bỏ hiệu ứng. Thanh HP vẫn cập nhật ngay theo `hp_changes`.
- `PROTOCOL_VERSION = 3`; cập nhật submodule `server/` và chạy đồng bộ.
- Smoke test: đọc `regions` thay cho `camps`, đi tới ổ quỷ gần nhất rồi đánh.
- Phần bỏ qua quái trả `INVALID_TARGET` vẫn giữ. Nó gần như không còn chạy tới nhưng vô hại.

## 8. Kiểm thử

### 8.1. Server

- `combat`: `MonsterRangedDamage` bằng ⌊50%⌋ sát thương cận chiến và không nhỏ hơn 1.
- `gamedata`: đọc trường mới; báo lỗi khi giá trị không dương.
- `mapdata`:
  - đọc `regions`;
  - ô của vùng không gồm khu an toàn và dải 1 ô quanh nó, cũng không gồm ô bị chặn;
  - mỗi lỗi kiểm tra ở mục 2.1 báo đúng thông điệp.
- `zone`, sinh quái:
  - quái ban đầu đều nằm trên ô của vùng;
  - với `kumotori_1` và một seed cố định, khoảng cách nhỏ nhất giữa hai con cùng vùng ít nhất 2 ô;
  - quái chết thì sau `respawn_sec` sinh lại ở ổ quỷ với đầy HP.
- `zone`, lang thang: chạy nhiều tick không có người chơi, quái luôn ở trên ô của vùng.
- `zone`, bắn:
  - người chơi vào tầm 5 ô thì sau 0,5 giây có `Hit` `ranged`, rồi cứ 2 giây một viên;
  - lúc bắn quái không di chuyển;
  - người trong khu an toàn hoặc bị tường che không bị bắn;
  - người rời tầm thì quái về **Lang thang**.
- `zone`, đuổi:
  - bị đánh thì quái thôi bắn, đuổi, và đánh cận chiến (không có `ranged`);
  - người thứ hai đánh thì quái không đổi mục tiêu.
- `zone`, bỏ đuổi:
  - mục tiêu ra khỏi vùng quá 3 ô thì quái bỏ đuổi; HP giữ nguyên; quái đi về ô của vùng rồi lang thang;
  - quái đã đuổi quá 10 ô thì bỏ;
  - mục tiêu vào khu an toàn thì quái chờ ở mép; bước ra trong 5 giây thì bị đánh; ở trong quá 5 giây thì quái bỏ;
  - lúc đuổi quái chạy 1,2 lần tốc độ người chơi: người chơi chạy lùi không giữ được khoảng cách;
  - sau khi bỏ, quái chuyển sang người đánh nó trong 5 giây gần nhất.
- `zone`, về vùng:
  - quái đang về vẫn nhận đòn, không trả `INVALID_TARGET`, và vào lại **Đuổi**;
  - kẹt quá 2 giây thì `snap` về ổ quỷ.
- `zone`: người chơi bị chưởng giết đi đúng luồng chết và hồi sinh.
- `gateway`: fixture mới.
- e2e: các test hạ quái, giới hạn tốc độ và lưu tiến độ qua đăng xuất vẫn qua.

### 8.2. Client

- `entity_table`: `Hit` có `ranged` sinh `projectile`, không sinh `slash`.
- Test protocol cho fixture mới.
- Smoke test giết được một con quái với server mới.

## 9. Tiêu chí hoàn thành

- Mọi test ở mục 8 qua (server với `-race`).
- Chơi thử hai cửa sổ:
  - quái rải khắp bốn vùng và lang thang;
  - đi ngang qua quái thì bị bắn chưởng, thấy viên chưởng bay và số sát thương;
  - đánh quái thì quái đuổi;
  - chạy ra xa thì quái bỏ đuổi mà không hồi máu;
  - quái chết thì hồi sinh ở ổ quỷ.
- Spec Giai đoạn 1 ghi chú ở mục 2.1, 2.2 và 4.2 rằng phần đó đã được thay bằng tài liệu này.
- `docs/game-design.md` (mục về quái thường hồi sinh) ghi: hồi sinh tại ổ quỷ của vùng.
