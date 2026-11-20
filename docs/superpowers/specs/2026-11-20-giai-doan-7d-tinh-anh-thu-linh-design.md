# Giai đoạn 7d — Tinh anh và thủ lĩnh

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng), chờ triển khai.

Phần thứ tư của Giai đoạn 7 (xem [tổng quan Giai đoạn 7](2026-11-20-giai-doan-7-tong-quan.md)). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 5.3 (hồi sinh tinh anh 5 phút, thủ lĩnh 10 phút, mỗi khu bộ đếm riêng), 5.5 (tinh anh/thủ lĩnh theo map), 5.8 (EXP theo sát thương, không trả quá EXP gốc), 5.9 (hệ số chỉ số theo hạng), 5.10 (kỹ năng đặc trưng, giới hạn mục tiêu, cảnh báo vùng 0,8 giây), 5.11 (điểm xuất hiện riêng, không chặn cổng) và 5.12 (thưởng theo hạng); game-design là chuẩn. Những gì không nhắc tới ở đây giữ nguyên như [spec quái thường](2026-11-20-quai-thuong-vung-va-chuong-design.md) (đuổi, bỏ đuổi, thù, về vùng), [spec ba mốc](2026-11-20-moc-don-danh-design.md) (hàng đợi đòn), [spec Giai đoạn 2](2026-11-20-giai-doan-2-ky-nang-design.md) (hình vùng của chiêu, lao tới) và [spec 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) (bảng rơi theo hạng). 7d chạy sau khi 7c đã triển khai (giao thức 18, migration 00011); 7d không phụ thuộc gì của 7a–7c.

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Mỗi map luyện cấp có một con tinh anh hoặc thủ lĩnh đứng ở khu riêng, xa trại: **Quỷ lang thang đầu đàn** (cấp 6, Kumotori), **Quỷ Đền** (cấp 10, Sagiri – Chân núi), **Quỷ săn đêm** (cấp 15, Sagiri – Rừng sương mù), **Quỷ Rừng tinh anh** (cấp 20, Fujikasane). Chúng to hơn quái thường, tên có nhãn "Tinh anh" hay "Thủ lĩnh", máu và công cao hơn theo game-design 5.9, chủ động lao vào người chơi tới gần và dùng kỹ năng đặc trưng. Kỹ năng vùng có cảnh báo trên đất 0,8 giây và **né được** bằng cách chạy ra khỏi vùng. Hạ được thì người kết liễu nhận Xu và đồ theo bảng thưởng hạng cao; mọi người đánh nhận EXP theo phần sát thương. Con bị hạ hồi sinh sau 5 phút (tinh anh) hoặc 10 phút (thủ lĩnh), mỗi khu một con và một bộ đếm.

Khung này (hạng, kỹ năng và nội tại khai báo bằng dữ liệu, khu thủ lĩnh trong map, hàm sinh quái đặc biệt) là thứ các Boss sự kiện 7e–7h dùng lại.

### 1.2. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Nội dung v1 | Bốn con của bốn map luyện cấp hiện có (bảng mục 2.2). Các con cấp 25–90 của game-design 5.10 thêm cùng map của chúng | Chỉ có map 1–20 (tổng quan mục 3.6); khung đủ để thêm bằng dữ liệu |
| Số con mỗi map | Đúng **một** con mỗi map luyện cấp, ở khu riêng trong vùng quái cấp cao nhất, xa trại | Game-design 5.5 liệt kê một con mỗi map; đọc "1 con/vùng" của 5.11 là "tối đa một con mỗi vùng"; bốn tinh anh cấp 6 trong map cấp 1–5 quá nặng cho tân thủ |
| Chỉ số theo hạng | Script nhân từ bảng cấp thường: tinh anh HP ×6, công ×1,5, mỗi thủ ×1,25; thủ lĩnh ×20, ×2, ×1,5; làm tròn lên. EXP gốc tinh anh ×6, thủ lĩnh ×25 EXP thường cùng cấp | Game-design 5.9 và 5.12 |
| Nơi tính hệ số | `monster_stats.py` ghi thẳng chỉ số tinh anh và thủ lĩnh vào từng dòng `levels` (`elite`, `boss`); Go chỉ đọc | CLAUDE.md: dữ liệu quái sinh bằng script; không nhân hệ số ở hai nơi |
| Nhịp đánh, hồi sinh | Tinh anh và thủ lĩnh đánh thường mỗi 1,2 giây; hồi sinh 300 và 600 giây từ lúc chết, tại điểm xuất hiện riêng | Game-design 5.3, 5.9 |
| Kỹ năng và nội tại | Khai báo trong `monsters.json`: `skills` (hình vùng, phần trăm công, hồi chiêu, kích thước, chuẩn bị, số mục tiêu) và `traits` (`heal_once`, `enrage`). Boss sau này chỉ thêm dữ liệu | Game-design 5.10 có 21 con; không viết code riêng từng con |
| Hình vùng của kỹ năng quái | Dùng lại năm hình của chiêu người chơi: `single` (có thể `dash`), `cone`, `line` (có thể `dash`), `circle_self`, `circle_target` | Một hình học đã kiểm thử; client đã vẽ được |
| Cảnh báo vùng | Kỹ năng vùng (`cone`, `line`, `circle_self`, `circle_target`) chuẩn bị **800 ms**; `Cast` mang `area` để client vẽ vùng cảnh báo suốt thời gian chuẩn bị | Game-design 5.10 "cảnh báo vùng trước 0,8 giây" |
| Né kỹ năng vùng | Vùng chốt lúc bấm (gốc, hướng); **nạn nhân chọn lúc trúng**: người chơi đứng trong vùng ở tick trúng mới dính. Đây là ngoại lệ có chủ đích của luật "đòn đã ra thì trúng" (spec ba mốc mục 1) | Cảnh báo 0,8 giây chỉ có nghĩa khi né được; luật cũ giữ cho mọi đòn không có cảnh báo |
| Kỹ năng đơn mục tiêu | `single` (như Vồ) chốt mục tiêu lúc bấm, không né được, chuẩn bị ngắn, không có `area` | Giữ luật ba mốc cho đòn không cảnh báo |
| Số mục tiêu | Kỹ năng vùng của tinh anh tối đa 3, thủ lĩnh tối đa 5 người; chọn gần gốc vùng nhất, bằng nhau thì id nhỏ hơn | Game-design 5.10 |
| Sát thương kỹ năng | Công của quái lúc bấm × phần trăm, rồi công thức thủ của người bị trúng lúc trúng (vật lý xét thủ vật lý, phép xét thủ phép) | Vùng chọn nạn nhân lúc trúng nên thủ cũng lấy lúc trúng; công chốt lúc bấm như mọi đòn |
| Lúc chuẩn bị | Quái đứng yên, không đánh thường, không đổi hướng cho tới tick trúng | Vùng cảnh báo không trôi theo quái |
| Quái chết trong lúc chuẩn bị | Đòn vẫn trúng | Như chưởng của quái chết trước khi tới (spec ba mốc mục 2.1) |
| Chủ động tấn công | Tinh anh và thủ lĩnh không bắn chưởng; thấy người chơi hợp lệ trong `detect_range` (5 ô, có tầm nhìn) là đuổi và đánh | Game-design: tinh anh/thủ lĩnh là mục tiêu săn; không cần hành vi "đứng bắn người đi ngang" |
| Đuổi, bỏ đuổi | Như quái thường, với khu thủ lĩnh thay cho vùng: tầm đuổi 10 ô, ra quá khu 3 ô liên tục 2 giây thì bỏ, chờ ở mép khu an toàn 5 giây, thù 10 giây | Một luật đuổi; người chơi đã quen |
| Hồi máu khi bỏ đuổi | **Không** | Như quái thường (game-design 5.3); giữ EXP đã trả đúng luật 5.8 |
| Nội tại `heal_once` (Quỷ Đền) | Lần đầu HP xuống dưới 30% sau một đòn trúng: hồi 10% HP tối đa một lần trong đời con quái | Game-design 5.10 |
| Nội tại `enrage` | Lần đầu HP xuống dưới ngưỡng: công tăng phần trăm cho tới khi chết; không con v1 nào dùng, có test bằng dữ liệu giả | Quỷ Asakusa, Quỷ Nhện Bố sau này chỉ thêm dữ liệu |
| EXP khi quái hồi máu | Tổng sát thương tính EXP của một con (mọi người cộng lại) không vượt HP tối đa; phần sát thương vào máu vừa hồi không tính khi đã chạm trần | Game-design 5.8 "một con quái không trả quá EXP gốc" |
| Thưởng | Bảng rơi `elite`, `boss` đã có từ 3a; người kết liễu nhận như quái thường; EXP theo sát thương; đồng đội nhận 5% (5b) | Game-design 5.12 |
| Khu thủ lĩnh trong map | Trường `bosses` của file map (do `mapgen.py` sinh): hình chữ nhật khu, điểm xuất hiện, loại, cấp. Khu được chồng lên vùng quái thường, không được chạm khu an toàn và cách mọi cổng ít nhất 2 ô | Game-design 5.11 "điểm xuất hiện riêng", "không chặn cổng qua map" |
| Auto (6c) | Auto vẫn chọn tinh anh/thủ lĩnh như quái khác trong bán kính bãi | Không thêm cài đặt; người chơi tự tắt auto khi không muốn đánh |
| Sinh quái cho sự kiện | Hàm `SpawnSpecial` trong zone (mục 4.8): loại, cấp, vị trí, khu nhà, không hồi sinh, hệ số HP, nhãn lượt sự kiện, hàm gọi khi chết | 7e–7h sinh Boss và nhóm quái theo lịch mà không sửa file map |
| Hiển thị | Thân to hơn (tinh anh ×1,3, thủ lĩnh ×1,6), nhãn "Tinh anh" vàng hoặc "Thủ lĩnh" đỏ trên tên, thanh máu lớn ở đỉnh màn hình khi khóa mục tiêu hoặc khi con đó ở trong 8 ô; tên kỹ năng hiện trên đầu lúc ra chiêu | Nhận ra mục tiêu nguy hiểm; thấy máu Boss trong lúc đánh đông người |

### 1.3. Bổ sung cho game-design

Game-design 5.10 nêu cảnh báo vùng nhưng không nói né được hay không, và không nêu góc quạt, độ dài dải của từng chiêu. Spec này chốt: kỹ năng vùng có cảnh báo thì né được; kích thước ở mục 2.2. Game-design 5.11 "1 con/vùng" được ghi rõ là tối đa một con mỗi vùng và v1 đặt một con mỗi map. Ghi vào game-design ở mục 9.

### 1.4. Không làm ở 7d

Tinh anh/thủ lĩnh cấp 25–90 và map của chúng; hiệu ứng chậm, giữ chân, ngủ, đẩy lùi (7g thêm ngủ cho Enmu); Quỷ Đầm Lầy ba phân thân chung máu; Boss chặn đường; thông báo toàn server khi Boss xuất hiện hay bị hạ; bảng xếp hạng sát thương (7f); chưởng của tinh anh; quái thường dùng kỹ năng.

## 2. Dữ liệu

### 2.1. `data/monsters.json`

`docs/balance/monster_stats.py` (sửa script, không sửa JSON):

- **Mỗi dòng `levels`** thêm hai khối cùng dạng `{hp, attack, defense, exp}`:
  - `elite`: `hp = ⌈6 × hp⌉`, `attack = ⌈1,5 × attack⌉`, `defense = ⌈1,25 × defense⌉`, `exp = 6 × exp`;
  - `boss`: `hp = 20 × hp`, `attack = 2 × attack`, `defense = ⌈1,5 × defense⌉`, `exp = 25 × exp`.
  - Ví dụ cấp 10: tinh anh 4.500 HP, 143 công, 100 thủ, 858 EXP; thủ lĩnh 15.000 HP, 190 công, 120 thủ, 3.575 EXP (khớp bảng game-design 5.9).
- **Mỗi loại** có thêm `skills` (mảng, có thể rỗng) và `traits` (mảng, có thể rỗng). Loại `normal` luôn rỗng cả hai.

Trường của một kỹ năng quái:

| Trường | Ý nghĩa |
| --- | --- |
| `id` | Mã, bắt đầu bằng `m_` (không trùng mã chiêu người chơi) |
| `name` | Tên hiện trên đầu quái lúc ra chiêu |
| `shape` | `single`, `cone`, `line`, `circle_self`, `circle_target` |
| `damage` | `phys` hoặc `magic` |
| `pct` | Phần trăm công, ≥ 1 |
| `cooldown_sec` | > 0 |
| `range` | Tầm ra chiêu (`single`, `circle_target`), độ vươn (`cone`), độ dài (`line`); không có ở `circle_self` |
| `radius` | `circle_self`, `circle_target` |
| `cone_deg` | `cone`, trong (0, 360] |
| `width` | `line` |
| `dash` | `single` hoặc `line`: quái lao tới (mục 4.4) |
| `windup_ms` | `single`: > 0, mặc định 300; vùng: đúng 800 |
| `speed` | Tốc độ lao (ô/giây) với `dash`; tốc độ đạn với `single` không lao (0 là không đạn) |
| `max_targets` | Chỉ với vùng; ≤ 3 với tinh anh, ≤ 5 với thủ lĩnh |

Trường của một nội tại: `kind` (`heal_once` hoặc `enrage`), `below_pct` (1–99), và `heal_pct` (1–100, với `heal_once`) hoặc `attack_pct` (1–200, với `enrage`). Mỗi loại tối đa một nội tại mỗi `kind`.

Trường loại của tinh anh/thủ lĩnh khác quái thường: `rank` là `elite` hoặc `boss`; `attack_sec` 1,2; `respawn_sec` 300 hoặc 600; `shoot_sec`, `shoot_windup_sec`, `shot_speed` giữ giá trị như quái thường để dữ liệu hợp lệ nhưng không dùng (mục 4.3); `detect_range` 5, `chase_range` 10, `region_margin` 3, `chase_speed` 1,2, `safe_wait_sec` 5, `margin_grace_sec` 2, `grudge_sec` 10, `speed` 0,8, `attack_range` 2,0, `melee_windup_ms` 200.

### 2.2. Bốn loại mới

| id | Tên | Hạng | Cấp | Map | Kỹ năng / nội tại |
| --- | --- | --- | --- | --- | --- |
| `wanderer_leader` | Quỷ lang thang đầu đàn | `elite` | 6 | `kumotori_1` | `m_pounce` "Vồ": `single`, `dash`, `phys`, 150%, hồi 6 giây, tầm 5, chuẩn bị 300 ms, tốc độ lao 25 |
| `shrine_demon` | Quỷ Đền | `boss` | 10 | `sagiri_1` | `heal_once`: dưới 30% HP hồi 10% HP tối đa |
| `night_hunter` | Quỷ săn đêm | `elite` | 15 | `sagiri_2` | `m_night_dash` "Lao tới": `line`, `dash`, `phys`, 150%, hồi 6 giây, dài 5, rộng 1,5, chuẩn bị 800 ms, tốc độ lao 25, tối đa 3 |
| `forest_elite` | Quỷ Rừng tinh anh | `elite` | 20 | `fujikasane_1` | `m_claw_sweep` "Quét móng": `cone`, `phys`, 130%, hồi 6 giây, vươn 3, góc 120°, chuẩn bị 800 ms, tối đa 3 |

Kích thước vùng (góc quạt, độ dài và bề rộng dải) game-design không nêu; lấy theo chiêu người chơi cùng hình ở cấp thấp (spec Giai đoạn 2: quạt 120°, dải rộng 1,5) để người chơi đọc được vùng.

### 2.3. Khu thủ lĩnh trong file map

`maps/<id>.json` thêm trường `bosses` (mảng, có thể rỗng; map `town` phải rỗng), do `tools/mapgen.py` sinh:

```json
"bosses": [
  {"id": "leader", "monster": "wanderer_leader", "level": 6,
   "rects": [{"x": 49, "y": 34, "w": 12, "h": 10}], "spawn": {"x": 54.5, "y": 39.5}}
]
```

(Tọa độ ví dụ ở Kumotori, trong vùng `r4` Bãi đá; số chính xác do script quyết.)

**Luật bố cục trong script:** mỗi map luyện cấp có đúng một mục; khu là một hình chữ nhật 12 × 10 nằm trọn trong vùng quái có cấp cao nhất (vùng xa trại nhất); điểm xuất hiện ở tâm khu, trên ô đi được; khu cách mọi cổng ít nhất 2 ô và không chạm khu an toàn hay dải 1 ô quanh nó.

**`mapdata` kiểm tra:** `id` không trùng trong map; `rects` không rỗng, nằm trong map, `w`, `h` dương; **ô của khu** là ô đi được trong ít nhất một hình chữ nhật, trừ khu an toàn và dải 1 ô quanh nó (cùng luật ô của vùng); điểm xuất hiện nằm trên một ô của khu; khoảng cách từ mọi ô của khu tới mọi cổng ≥ 2 ô; điểm xuất hiện tới được từ `default` (BFS của 4a); khu được chồng lên vùng quái thường nhưng hai khu thủ lĩnh không chung ô.

**`gamedata.Validate(maps)` kiểm tra chéo:** `monster` của khu là loại hạng `elite` hoặc `boss`, cấp có trong bảng; vùng quái thường chỉ trỏ tới loại `normal`.

**`gamedata` kiểm tra kỹ năng khi nạp:** đủ và chỉ có trường kích thước của hình (như chiêu người chơi); `dash` chỉ với `single`/`line`, có `speed` > 0; vùng có `windup_ms` = 800 và `max_targets` trong 1 … giới hạn hạng; `single` không có `max_targets`; mã bắt đầu bằng `m_`, không trùng trong loại; quái `normal` không có kỹ năng, nội tại.

## 3. Lưu trữ

Không có migration. Quái không lưu; bộ đếm hồi sinh mất khi zone dừng (khởi động lại server thì mọi con sinh ngay).

## 4. Server

### 4.1. Chỉ số theo hạng

`gamedata.Level.ForRank(rank) Stats` trả khối thường, `elite` hoặc `boss`. Mọi chỗ zone đang đọc `z.area.Levels[m.level]` (HP tối đa, công, thủ, EXP gốc) đọc qua hàm này với hạng của loại quái. Quái thường không đổi gì.

### 4.2. Sinh và hồi sinh

- Khi zone khởi động (`spawnAll`), mỗi mục `bosses` của map sinh một con ở điểm xuất hiện với cấp của mục đó, HP đầy.
- Chết: giờ hồi sinh = tick chết + `respawn_sec` của loại (300 hay 600 giây); tới giờ thì sinh một con mới (id mới từ `NewID`) ở điểm xuất hiện. Mỗi zone (khu) có con và bộ đếm riêng.
- Khu thủ lĩnh dựng thành một vùng nội bộ (cùng kiểu `regionState` của vùng quái thường, số con 1, ổ quỷ là điểm xuất hiện), để đuổi, bỏ đuổi, về vùng và lang thang dùng chung code với quái thường.
- Lang thang: như quái thường, trong ô của khu, tốc độ `speed`.

### 4.3. AI

Thứ tự trong tick không đổi (`thinkMonsters`, theo id). Với con hạng `elite`/`boss`:

1. **Đang chuẩn bị kỹ năng** (tick hiện tại < tick trúng của kỹ năng đang ra): đứng yên, bỏ qua mọi bước dưới.
2. **Không có mục tiêu:** người chơi hợp lệ gần nhất (còn sống, ngoài khu an toàn, cách ≤ `detect_range`, đường thẳng không bị chặn; bằng nhau thì id nhỏ hơn) thì bắt đầu đuổi người đó (`startChase`). Người có thù (đã đánh nó trong `grudge_sec`) được ưu tiên như quái thường. Không bắn chưởng.
3. **Có mục tiêu:** các luật giữ, đổi, bỏ mục tiêu và về vùng của spec quái thường mục 4.4–4.5 (khu thủ lĩnh thay cho vùng).
4. **Ra đòn** khi mục tiêu còn giữ được và đang trong tầm nhìn: xét từng kỹ năng theo thứ tự trong dữ liệu; kỹ năng đầu tiên đã hết hồi và có mục tiêu trong tầm ra chiêu (mục 4.4) thì dùng. Không kỹ năng nào dùng được thì đuổi và đánh thường như quái thường (mỗi `attack_sec` = 1,2 giây, công theo hạng).
5. Dùng kỹ năng thì: hồi kỹ năng đó = tick bấm + `cooldown_sec`; đánh thường kế tiếp không sớm hơn tick trúng của kỹ năng.

### 4.4. Kỹ năng

**Tầm ra chiêu** (xét lúc bấm, từ quái tới mục tiêu đang đuổi, cộng 0,5 ô dung sai như chiêu người chơi): `single`, `circle_target`: ≤ `range`; `cone`: ≤ `range`; `line`: ≤ `range`; `circle_self`: ≤ `radius`.

**Kỹ năng `single`:** chốt mục tiêu và sát thương lúc bấm (`combat.MonsterSkillDamage(công, pct, thủ mục tiêu, cấp quái)`), như chiêu người chơi:

- `dash` (Vồ): quái dời ngay tới điểm đáp cách mục tiêu `dashGap` (1 ô) trên đường thẳng, dừng trước ô bị chặn (cùng hàm điểm đáp của chiêu lao tới, thân quái); trúng sau `windup_ms` + quãng lao / `speed`.
- Không lao: trúng sau `windup_ms` + khoảng cách / `speed` (0 là không có đạn).
- `Cast{caster_id, skill_id, target_id, dir, windup_ms, impacts}` như đòn có mục tiêu; không có `area`.

**Kỹ năng vùng** (`cone`, `line`, `circle_self`, `circle_target`):

- **Lúc bấm (t0):** quái dừng. Chốt **vùng**: gốc là vị trí quái (`circle_target`: vị trí mục tiêu lúc bấm), hướng từ quái tới mục tiêu, kích thước theo dữ liệu. Chốt **công** của quái (gồm `enrage` nếu đang bật). Tick trúng = t0 + `z.ticks(800 ms)`. Phát `Cast{caster_id, skill_id, target_id, dir, windup_ms = 800, impacts = [], area}`.
- **Tick trúng:** nạn nhân là mọi người chơi còn sống, ngoài khu an toàn, nằm trong vùng (cùng phép thử hình của chiêu người chơi: khoảng cách và góc với quạt, hình chữ nhật dọc hướng với dải, khoảng cách tới tâm với vòng), và đường thẳng từ gốc vùng tới họ không bị chặn; xếp theo khoảng cách tới gốc, bằng nhau thì id nhỏ hơn; lấy tối đa `max_targets`. Sát thương từng người = `combat.MonsterSkillDamage(công đã chốt, pct, thủ của họ lúc này, cấp quái)`; áp như đòn quái lên người chơi (spec ba mốc mục 3.3) và phát `Hit` với `skill_id`. Không ai trong vùng thì không có `Hit`.
- **`line` có `dash`** (Lao tới): ở tick trúng, sau khi chọn nạn nhân, quái dời tới cuối dải (hoặc chỗ dừng trước ô bị chặn đầu tiên theo hướng dải, thân quái), người xung quanh thấy qua `EntityMove`.
- Quái chết hoặc mục tiêu chạy mất trước tick trúng: vùng vẫn nổ đúng chỗ đã chốt.

Kỹ năng quái không bị bộ lọc PK (quái đánh mọi người chơi như đòn thường); người ra đòn của `Hit` là quái nên không đổi thù hay EXP.

### 4.5. Nội tại

Xét ngay sau khi một đòn trúng làm HP con quái giảm mà nó vẫn còn sống (`applyToMonster`):

- **`heal_once`:** chưa dùng và `hp × 100 < max_hp × below_pct` thì `hp = min(max_hp, hp + ⌊max_hp × heal_pct / 100⌋)`, đánh dấu đã dùng, ghi `HpChange` để người xung quanh thấy thanh máu tăng.
- **`enrage`:** chưa bật và cùng điều kiện thì bật; từ đó công dùng cho đòn mới = `combat.Enraged(công, attack_pct)` = `⌈công × (100 + attack_pct) / 100⌉`. Không có gói riêng; client không cần biết.

Con hồi sinh là con mới, nội tại chưa dùng.

### 4.6. EXP

Con quái giữ thêm `counted`: tổng sát thương tính EXP của mọi người. Sát thương tính EXP của một đòn = `combat.CountedDamage(sát thương, HP trước đòn, max_hp, counted)` = `min(sát thương, HP trước đòn, max_hp − counted)` (không âm); `counted` cộng phần đó. Còn lại như luật 5.8 hiện có (`payExpShare`, cửa sổ ±10 cấp, EXP tổ đội 5b). Với quái không hồi máu, `max_hp − counted` luôn ≥ HP còn lại nên kết quả không đổi.

Ví dụ Quỷ Đền (15.000 HP, 3.575 EXP): đánh xuống 4.400 HP (đã tính 10.600), hồi thành 5.900; đánh tiếp 5.900 nữa nhưng chỉ 4.400 được tính; tổng tính 15.000, EXP trả đúng 3.575.

### 4.7. Thưởng khi chết

Không đổi so với 3a: người kết liễu nhận Xu, trang bị và đá theo bảng `drops` của hạng; nhiệm vụ 6b/6d tính như quái khác (`kill_any` của 6d tính cả tinh anh, thủ lĩnh trong cửa sổ cấp).

### 4.8. Sinh quái đặc biệt (cho 7e–7h)

Hàm nội bộ của zone, chỉ gọi trên goroutine zone (từ code sự kiện của các phần sau):

```go
// SpecialSpawn describes a monster that is not part of the map's regions or boss spots.
type SpecialSpawn struct {
	Kind      string        // monsters.json kind, any rank
	Level     int
	Pos       Vec2          // must be walkable
	Home      []mapdata.Rect // its region for wander, chase and return; tiles as for regions
	HPMul     int           // max HP = rank HP × HPMul; 0 means 1
	ExpMul    int           // base EXP = rank EXP × ExpMul; 0 means 1
	Aggressive bool         // chase at once, see below
	Tag       string        // event run tag; DespawnTag removes every live monster with it
	OnDeath   func(m MonsterRef, killer *player) // called after loot, killer may be nil
}

func (z *Zone) SpawnSpecial(s SpecialSpawn) (id uint64, err error)
func (z *Zone) DespawnTag(tag string) int // removes them (EntityLeave, no loot, no EXP); returns how many
```

- Con sinh đặc biệt **không hồi sinh**; chết thì loot và EXP như thường, rồi gọi `OnDeath`.
- `HPMul` chỉ nhân HP tối đa, `ExpMul` chỉ nhân EXP gốc (thưởng riêng của sự kiện do phần đó quyết). Trần `counted` (mục 4.6) dùng HP tối đa mới, nên EXP trả theo tỷ lệ sát thương trên HP mới: con có `HPMul` 4 và `ExpMul` 1 trả tổng EXP như con thường cùng hạng, cấp.
- `Aggressive` (cho đợt quái của phó bản, 7g): vừa sinh là đuổi người chơi còn sống gần nhất trong `Home`; mục tiêu chết hay ra khỏi `Home` thì chọn lại người gần nhất trong `Home`; không bắn người đi ngang; không bỏ đuổi khi mục tiêu còn trong `Home`. Không có ai trong `Home` thì về vị trí sinh và đứng chờ.
- Kiểm tra: loại có thật, cấp có trong bảng, vị trí đi được, `Home` có ít nhất một ô: sai thì trả lỗi, không sinh.

### 4.9. Thứ tự tick

Không đổi: `drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView`. Kỹ năng vùng xếp một mục mới vào hàng đợi đòn kiểu "vùng" (gốc, hướng, kích thước, công, kỹ năng); `runImpacts` chọn nạn nhân khi tới tick. Hồi sinh tinh anh/thủ lĩnh trong `runSpawnTimers`.

## 5. Giao thức

`ProtocolVersion` lên **19** (hành vi quái và dữ liệu map đổi; client cũ không vẽ được cảnh báo).

Không có envelope mới (dải 185–189 để trống) và không có mã lỗi mới (89–90 để trống).

```proto
enum MonsterRank {
  MONSTER_RANK_UNSPECIFIED = 0; // not a monster
  MONSTER_RANK_NORMAL = 1;
  MONSTER_RANK_ELITE = 2;
  MONSTER_RANK_BOSS = 3;
}

enum AreaShape {
  AREA_SHAPE_UNSPECIFIED = 0;
  AREA_SHAPE_CONE = 1;
  AREA_SHAPE_LINE = 2;
  AREA_SHAPE_CIRCLE = 3;
}

// The ground a telegraphed monster skill will hit when its windup ends.
message CastArea {
  AreaShape shape = 1;
  float x = 2;        // cone and line origin, circle centre
  float y = 3;
  float dir_x = 4;    // cone and line direction, unit length
  float dir_y = 5;
  float reach = 6;    // cone radius, line length
  float radius = 7;   // circle radius
  float cone_deg = 8;
  float width = 9;    // line width
  bool dash = 10;     // the caster ends at the far end of the line
}
```

- `EntityEnter.monster_rank = 18` (`MonsterRank`): có với thực thể `MONSTER`.
- `Cast.area = 10` (`CastArea`): có với kỹ năng vùng của quái; khi có `area` thì `impacts` rỗng và `Hit` tới ở tick trúng cho từng người dính.
- `Cast.skill_id` của quái là mã `m_…`; `Hit.skill_id` cũng vậy.
- Fixture mới: `entity_enter_boss.hex` (thủ lĩnh cấp 10, máu đầy), `world_tick_area_cast.hex` (`Cast` quạt có `area`), `world_tick_area_hits.hex` (hai `Hit` của cùng kỹ năng). Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`tools/sync_server.sh`** chép thêm `data/monsters.json` (tên kỹ năng quái); `--check` so cả file này.
- **`core/monster_book.gd`** (mới): đọc `monsters.json`; `skill_name(skill_id) -> String`; `rank_label(rank) -> String` ("", "Tinh anh", "Thủ lĩnh").
- **`core/entity_table.gd`**: lưu `monster_rank`; `Cast` có `area` sinh hiệu ứng `"warning"` `{shape, origin, dir, reach, radius, cone_deg, width, until}` (until = giờ nhận + `windup_ms`) và hiệu ứng `"skill_name"` trên đầu quái; `Hit` của kỹ năng vùng hiện số và lóe sáng như đòn quái (không khớp đạn nào).
- **`core/attack_timeline.gd`**: `Cast` có `area` không tạo đạn.
- **`core/targeting.gd`**: không đổi (tinh anh/thủ lĩnh là quái).
- **`core/boss_bar.gd`** (mới): chọn con hiện thanh máu lớn: mục tiêu đang khóa nếu là tinh anh/thủ lĩnh, nếu không thì con tinh anh/thủ lĩnh còn sống gần nhất trong 8 ô; không có thì ẩn.

### 6.2. Giao diện

- **`ui/entity_view.gd`**: thân tròn bán kính ×1,3 (tinh anh) hoặc ×1,6 (thủ lĩnh); nhãn "Tinh anh" màu vàng hoặc "Thủ lĩnh" màu đỏ phía trên tên.
- **`ui/warning_effect.gd`** (mới): vẽ vùng đỏ mờ theo `shape` (quạt, dải, vòng) suốt thời gian chuẩn bị, viền đậm dần, tắt khi hết giờ.
- **`ui/boss_bar.gd`** (mới): thanh máu rộng ở đỉnh màn hình, tên, cấp, nhãn hạng, số HP.
- **Tên kỹ năng**: chữ nổi "Vồ!", "Lao tới!", "Quét móng!" trên đầu quái 1 giây.

### 6.3. Smoke

`tools/smoke.sh`: `devchar -level 20 -map kumotori_1 -x … -y …` cạnh điểm xuất hiện của Quỷ lang thang đầu đàn (đọc từ `maps/kumotori_1.json`), vào lại. `tests/smoke/smoke.gd` sau các bước cũ: chờ `EntityEnter` có `monster_rank` `ELITE`; đánh bằng `attack_loop` (chỉ chém) tới khi có `Death` của con đó trong 120 giây; kiểm tra đã nhận ít nhất một `Cast` có `skill_id` `m_pounce`. In "SMOKE OK" kèm "elite down".

## 7. Lệnh dev

Không có lệnh mới. Thử Boss ở khu khác dùng `devchar -map … -x … -y …` như cũ; thời gian hồi sinh 5/10 phút thử bằng test zone với tick giả.

## 8. Kiểm thử

**Script:** `monster_stats.py --check` so `monsters.json`; in bảng tinh anh/thủ lĩnh cấp 10, 20, …, 90 và khớp bảng game-design 5.9 (cấp 10: 4.500 và 15.000; cấp 20: 16.224 và 54.080; cấp 30: 26.016 và 86.720); EXP tinh anh cấp 10 là 858, thủ lĩnh 3.575.

**Server** (`go test -race`):

- `gamedata`: dữ liệu thật nạp được (bốn loại mới, hạng đúng); `ForRank` trả đúng khối; từ chối: kỹ năng thiếu `radius` ở `circle_self`, có `cone_deg` ở `line`, vùng có `windup_ms` khác 800, `max_targets` 4 ở tinh anh, `dash` ở `cone`, mã không bắt đầu bằng `m_`, quái `normal` có kỹ năng, nội tại `heal_pct` 0, hai nội tại cùng `kind`; vùng quái thường trỏ tới loại `elite`; khu thủ lĩnh trỏ tới loại `normal`.
- `mapdata`: khu thủ lĩnh ngoài map, chạm khu an toàn, cách cổng 1,5 ô, điểm xuất hiện ngoài khu, hai khu chung ô đều bị từ chối; bốn map thật mỗi map đúng một mục, `ubuyashiki` không có; `--check` của `mapgen.py`.
- `combat`: `CountedDamage` (ví dụ Quỷ Đền mục 4.6: tổng tính 15.000); `Enraged(190, 20) = 228`; `MonsterSkillDamage` 150% bằng `Damage` của công × 1,5 làm tròn xuống.
- `zone` (tick giả):
  - Sinh: zone mới có một con ở điểm xuất hiện với HP theo hạng; chết thì 299 giây chưa sinh, 300 giây sinh lại (thủ lĩnh 600), id mới; hai khu có hai con và hai bộ đếm riêng.
  - AI: người chơi cách 4,9 ô có tầm nhìn bị đuổi ngay, không có `Cast` chưởng; người ngoài 5 ô hoặc sau tường thì không; bỏ đuổi về khu không hồi máu; đánh thường mỗi 12 tick (tick rate 10).
  - Vồ: mục tiêu cách 4 ô, quái dời tới cách 1 ô lúc bấm, `Hit` tới ở t0 + ticks(0,3 + 3/25), sát thương 150%; hồi 6 giây (không dùng lại trong 60 tick).
  - Quét móng: `Cast` có `area` quạt 120°, vươn 3, `windup_ms` 800, `impacts` rỗng; người đứng yên trong quạt nhận `Hit` ở t0 + 8 tick; người chạy ra ngoài quạt trước tick trúng không nhận gì; người chạy vào quạt sau lúc bấm thì dính; năm người trong quạt chỉ ba người gần gốc nhất dính; quái đứng yên suốt 8 tick; quái bị hạ trong lúc chuẩn bị thì vùng vẫn nổ.
  - Lao tới: nạn nhân trong dải ở tick trúng; quái tới cuối dải (dừng trước tường nếu có).
  - Quỷ Đền: xuống dưới 30% thì hồi 10% một lần, có `HpChange`; xuống lần hai không hồi; EXP tổng của một người hạ một mình đúng 3.575.
  - Nội tại `enrage` (dữ liệu giả): đòn sau ngưỡng mạnh hơn 20%.
  - Thưởng: người kết liễu thủ lĩnh có đúng hai món trang bị và 10 đá cường hóa (bảng `boss`).
  - `SpawnSpecial`: sinh được loại bất kỳ hạng; `HPMul` 3 cho HP ×3; `ExpMul` 2 cho EXP gốc ×2; `Aggressive` đuổi ngay người gần nhất trong `Home` và chọn lại khi người đó ra khỏi `Home`; không hồi sinh; `OnDeath` gọi đúng một lần với người kết liễu; `DespawnTag` bỏ đúng các con có nhãn đó, người xung quanh nhận `EntityLeave`, không rơi đồ; vị trí trên tường trả lỗi.
- `gateway`: `ProtocolVersion` 19; fixture mới.
- e2e `TestEliteOverWebSocket`: nhân vật cấp 30 (store, có kiếm bậc 20) đặt cạnh điểm xuất hiện của Quỷ lang thang đầu đàn; vào game; nhận `EntityEnter` có `monster_rank` `ELITE` và HP 1.896; chém tới khi có `Death`; nhận ít nhất một `Cast` `m_pounce` trước đó; có trang bị dưới đất của mình (tinh anh rơi chắc chắn một món).

**Client** (`tests/run.sh`, mục `# ____TEST ELITES____`): `entity_table` tạo `warning` đúng hình từ fixture và `skill_name`; `attack_timeline` không tạo đạn cho `Cast` có `area`; `boss_bar` chọn mục tiêu khóa trước, rồi con gần nhất trong 8 ô, ẩn khi không có; `monster_book.skill_name("m_pounce") == "Vồ"`; giải mã fixture mới. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito) tới khu Bãi đá ở Kumotori, thấy Quỷ lang thang đầu đàn to hơn, có nhãn "Tinh anh" và thanh máu trên đỉnh màn hình; bị Vồ; ở Fujikasane đứng trong vùng đỏ của Quét móng thì dính, chạy ra kịp thì không; ở Sagiri 1 thấy Quỷ Đền hồi máu một lần dưới 30%; hạ xong thấy hai món đồ rơi; đổi sang khu 2 vẫn có một Quỷ Đền khác.

## 9. Tài liệu cập nhật

- `docs/game-design.md` mục 5.10: kỹ năng vùng có cảnh báo né được, nạn nhân chọn lúc trúng; kích thước Vồ, Lao tới, Quét móng; Quỷ Đền hồi máu không làm tăng EXP; mục 5.11: "1 con/vùng" là tối đa một con, v1 một con mỗi map ở khu riêng; mục 2.4: ngoại lệ né cho kỹ năng quái có cảnh báo.
- Spec ba mốc mục 1: ghi chú ngoại lệ của 7d.
- Spec 4a mục 1.3 (dòng tinh anh, thủ lĩnh): làm ở 7d.
- `CLAUDE.md` (server): chỉ số tinh anh/thủ lĩnh do script ghi trong `levels`; kỹ năng vùng của quái chọn nạn nhân lúc trúng; `SpawnSpecial` là cách duy nhất để sự kiện sinh quái.
- README hai repo: tinh anh và thủ lĩnh trên bốn map, cảnh báo vùng.
