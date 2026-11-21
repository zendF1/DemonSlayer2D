# Giai đoạn 2 — Kỹ năng

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế, chờ triển khai.

Spec này thiết kế Giai đoạn 2 cho cả server (repo này) và client Godot (`../DemonSlayer2DGame`). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 2.3–2.6; khi hai tài liệu lệch nhau về luật chơi thì game-design là chuẩn, trừ các điểm ghi rõ ở mục 1.3. Những gì không nhắc tới ở đây giữ nguyên như [spec Giai đoạn 1](2026-11-19-giai-doan-1-chien-dau-pve-design.md) và [spec quái thường theo vùng](2026-11-20-quai-thuong-vung-va-chuong-design.md).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Nhân vật đủ cấp thì học và nâng kỹ năng của class mình. Người chơi dùng bốn chiêu chủ động lên quái từ thanh kỹ năng; mỗi chiêu tốn MP, có hồi chiêu, tính vùng trúng trên server, và chiêu lao tới dời nhân vật thật. Bị động cộng vào chỉ số. Hai cửa sổ cùng map thấy chiêu và số sát thương của nhau. Hai tài khoản test `test1` (Kaito, Viêm) và `test2` (Hana, Thủy) ở cấp 90, đủ điểm để thử mọi chiêu.

Giai đoạn này cũng đổi luật EXP: quái cấp càng cao cho càng nhiều EXP trên mỗi điểm sát thương, và chỉ nhận EXP từ quái chênh không quá 10 cấp.

### 1.2. Quyết định đã chốt

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Lên cấp cao để thử chiêu | Không thêm map hay quái. Lệnh dev `devchar` đưa `test1`, `test2` lên cấp 90 | Map mới thuộc Giai đoạn 4; người dùng chỉ cần thử kỹ năng |
| Sách kỹ năng | Tạm bỏ. Học chỉ cần đủ cấp nhân vật và 1 điểm kỹ năng | Sách đến từ nhiệm vụ (Giai đoạn 6) và là vật phẩm (Giai đoạn 3); thêm điều kiện khi có sách |
| Trượt và chí mạng | Vẫn luôn trúng, không chí mạng. Lặng, Nhiệt Giới Lôi và phần chính xác của Tình Lam Phong Thụ chỉ tăng chỉ số hiển thị | Công thức trượt/chí mạng chưa chốt trong game-design; bật lên phải cân bằng lại quái và PvP |
| Chiêu lao tới (Bất Tri Hỏa, Phích Lịch Nhất Thiểm) | Server dời nhân vật thật tới cạnh mục tiêu; đường lao bị chắn thì không dùng được | Đúng chữ "lao/lướt" và yêu cầu kiểm tra vị trí, vật cản ở game-design mục 2.6 |
| Thanh kỹ năng | Năm ô cố định ở giữa cạnh dưới: chém mặc định và bốn chiêu chủ động. Chọn một ô rồi bấm/giữ nút Đánh để dùng chiêu đó | Người dùng yêu cầu; không cần lưu cách xếp |
| Tầm đánh | Chiêu đơn mục tiêu (kể cả lao tới) 5 ô cho mọi class. Chiêu nhiều mục tiêu đánh xa, tầm theo cấp kỹ năng chung cho mọi class: cấp 1–3 là 4 ô, 4–6 là 5 ô, 7–10 là 6 ô. Ba chiêu vòng quanh mình giữ bán kính | Người dùng yêu cầu: các class có tầm xa bằng nhau để không thả diều được nhau |
| Học và nâng chiêu | Tab Kỹ năng trong cửa sổ nhân vật (nút "Thông tin", tab Trang bị, Thông tin, Tiềm năng, Kỹ năng, Hành trang) | Người dùng yêu cầu; tab Trang bị và Hành trang để trống tới Giai đoạn 3 |
| Nguồn định nghĩa chiêu | `docs/balance/skill_stats.py` sinh `data/skills.json`; server và client cùng đọc | Một nguồn số liệu, như `monsters.json` |
| Mục tiêu của chiêu | Chỉ quái | PK thuộc Giai đoạn 5 |
| Hồi MP | Như Giai đoạn 1: điểm nghỉ và lên cấp | Bình thuốc thuộc Giai đoạn 4 |
| Điểm kỹ năng | 1 điểm mỗi lần lên cấp; nhân vật đang có được bù `cấp − 1` điểm | Game-design mục 2.6 |
| EXP theo cấp quái | Mỗi điểm sát thương cho r(L) EXP, r tăng theo cấp quái; bảng EXP lên cấp tính lại để giữ thời gian lên cấp | Người dùng yêu cầu: quái cấp cao đánh đau hơn nên cho nhiều EXP hơn |
| Cửa sổ cấp | Nhân vật và quái chênh quá 10 cấp thì không nhận EXP | Người dùng yêu cầu |

### 1.3. Khác với game-design

- Học lần đầu **không cần sách** cho tới khi có vật phẩm sách. (Ghi chú: sách kỹ năng đã có từ spec 6b; học lần đầu nay cần sách, trừ chiêu có cấp học không quá mức miễn của nhân vật.)
- **Tầm đánh** của chiêu đổi theo mục 2.1 (chiêu đơn 5 ô; chiêu nhiều mục tiêu 4/5/6 ô theo cấp kỹ năng) thay cho tầm riêng từng chiêu, và cộng điểm **có** tăng tầm của chiêu nhiều mục tiêu. Game-design mục 2.6 được cập nhật theo.
- **EXP gốc của quái** đổi từ `10 × cấp` thành `HP × r(L)` (mục 2.4). Game-design mục 2.2, 5.8, 5.12 được cập nhật theo.

### 1.4. Không làm ở Giai đoạn 2

Sách kỹ năng, tinh anh Quỷ lang thang đầu đàn và chiêu Vồ, kỹ năng của quái, trượt và chí mạng, chiêu hỗ trợ/buff, PK, bình thuốc, tẩy điểm kỹ năng, lưu hồi chiêu qua lần đăng nhập, hình ảnh chiêu thật (chỉ có hiệu ứng vẽ tạm).

## 2. Dữ liệu

### 2.1. `data/skills.json`

Sinh bằng `python3 docs/balance/skill_stats.py --write-json`; không sửa tay. Server nạp qua `internal/gamedata`; client đọc bản chép ở `data/skills.json` của repo client (mục 6.4).

```json
{
  "skills": [
    {
      "id": "viem_1", "class": "viem", "name": "Bất Tri Hỏa", "form": "I",
      "learn_level": 10, "slot": 1, "damage": "phys",
      "shape": "single", "dash": true,
      "range":       [5, 5, 5, 5, 5, 5, 5, 5, 5, 5],
      "coef_pct":    [150, 165, 180, 195, 210, 225, 240, 255, 270, 285],
      "mp":          [10, 12, 14, 16, 18, 20, 22, 24, 26, 28],
      "cooldown_ms": [1500, 1350, 1215, 1094, 984, 886, 797, 717, 646, 581]
    },
    {
      "id": "viem_3", "class": "viem", "name": "Thịnh Viêm Lãng Triều", "form": "IV",
      "learn_level": 30, "slot": 0,
      "passive": { "phys_def_pct": [2, 4, 6, 8, 10, 12, 14, 16, 18, 20],
                   "magic_def_pct": [2, 4, 6, 8, 10, 12, 14, 16, 18, 20] }
    }
  ]
}
```

**Trường chung:** `id` (`<class>_<1..5>` theo thứ tự cấp học), `class` (`viem`, `thuy`, `minh`, `nham`, `phong`), `name`, `form` (số thức La Mã), `learn_level` (10/20/30/40/50), `slot` (1–4 cho chủ động theo thứ tự cấp học, 0 cho bị động).

**Chủ động** có thêm:

| Trường | Ý nghĩa |
| --- | --- |
| `damage` | `phys` (dùng công vật lý) hoặc `magic` (công phép) |
| `shape` | `single`, `cone`, `circle_self`, `circle_target`, `line` |
| `range[10]` | Tầm theo cấp kỹ năng, ô. Có ở mọi shape trừ `circle_self`. Là tầm chọn mục tiêu của `single`/`circle_target`, độ vươn của `cone`, độ dài của `line`. `single`: 5 ở mọi cấp; các shape khác: 4,4,4,5,5,5,6,6,6,6 |
| `radius` | Bán kính vùng, ô. Có ở `circle_self` và `circle_target` |
| `cone_deg` | Góc mở của quạt. Có ở `cone` |
| `width` | Bề rộng của dải. Có ở `line` |
| `dash` | `true` với chiêu lao tới; chỉ hợp lệ với `single` |
| `coef_pct[10]` | Hệ số theo cấp kỹ năng 1–10, phần trăm nguyên |
| `mp[10]` | MP theo cấp kỹ năng |
| `cooldown_ms[10]` | `round(hồi chiêu cấp 1 × 0,9^(n−1) × 1000)` |
| `max_targets[10]` | Chỉ có ở chiêu nhiều mục tiêu (mọi shape trừ `single`): 2,2,2,3,3,3,4,4,4,5 |

**Bị động** có `passive`: một hoặc nhiều khóa trong `phys_def_pct`, `magic_def_pct`, `max_hp_pct`, `evasion_pct`, `accuracy_pct`, `magic_attack_pct`, `crit_bp`, mỗi khóa là mảng 10 số nguyên không âm (phần trăm, hoặc điểm cơ bản với `crit_bp`, 100 = 1%). Bị động không có các trường của chủ động.

**Bảng 25 chiêu** (theo game-design mục 2.6, MP và hồi chiêu theo mục "MP và hồi chiêu"):

| id | Tên | Ô | Kiểu | Thông số vùng | Hệ số | MP | Hồi cấp 1 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| viem_1 | Bất Tri Hỏa | 1 | single, dash | | 150 + 15(n−1) | 10 + 2(n−1) | 1,5 s |
| viem_2 | Thăng Viêm Thiên | 2 | cone | 120° | 110 + 10(n−1) | 15 + 3(n−1) | 2,5 s |
| viem_3 | Thịnh Viêm Lãng Triều | 0 | bị động | phys_def_pct, magic_def_pct = 2n | | | |
| viem_4 | Viêm Hổ | 3 | circle_target | radius 1,5 | 160 + 15(n−1) | 25 + 4(n−1) | 4 s |
| viem_5 | Luyện Ngục | 4 | single | | 250 + 25(n−1) | 40 + 5(n−1) | 7,5 s |
| thuy_1 | Thủy Diện Trảm | 1 | single | | 150 + 15(n−1) | 10 + 2(n−1) | 1,5 s |
| thuy_2 | Đả Triều | 2 | cone | 120° | 110 + 10(n−1) | 15 + 3(n−1) | 2,5 s |
| thuy_3 | Lặng | 0 | bị động | evasion_pct = 3n | | | |
| thuy_4 | Liệt Oa | 3 | circle_self | radius 2,5 | 160 + 15(n−1) | 25 + 4(n−1) | 4 s |
| thuy_5 | Sinh Sinh Lưu Chuyển | 4 | single | | 250 + 25(n−1) | 40 + 5(n−1) | 7,5 s |
| minh_1 | Phích Lịch Nhất Thiểm | 1 | single, dash | | 150 + 15(n−1) | 10 + 2(n−1) | 1,5 s |
| minh_2 | Tụ Văn Thành Lôi | 2 | circle_self | radius 2 | 110 + 10(n−1) | 15 + 3(n−1) | 2,5 s |
| minh_3 | Nhiệt Giới Lôi | 0 | bị động | crit_bp = 100n | | | |
| minh_4 | Viễn Lôi | 3 | line | width 1,5 | 160 + 15(n−1) | 25 + 4(n−1) | 4 s |
| minh_5 | Hỏa Lôi Thần | 4 | single | | 250 + 25(n−1) | 40 + 5(n−1) | 7,5 s |
| nham_1 | Xà Văn Nham · Song Cực | 1 | single | | 150 + 15(n−1) | 10 + 2(n−1) | 1,5 s |
| nham_2 | Thiên Diện Toái | 2 | circle_target | radius 1,5 | 110 + 10(n−1) | 15 + 3(n−1) | 2,5 s |
| nham_3 | Nham Phu Khu | 0 | bị động | max_hp_pct, phys_def_pct = 2n | | | |
| nham_4 | Lưu Văn Nham · Tốc Chinh | 3 | circle_self | radius 3 | 160 + 15(n−1) | 25 + 4(n−1) | 4 s |
| nham_5 | Ngõa Hình Luân Bộ | 4 | single | | 250 + 25(n−1) | 40 + 5(n−1) | 7,5 s |
| phong_1 | Trần Toàn Phong · Tước | 1 | line | width 1,5 | 110 + 10(n−1) | 15 + 3(n−1) | 2,5 s |
| phong_2 | Trảo · Khoa Hộ Phong | 2 | single | | 150 + 15(n−1) | 10 + 2(n−1) | 1,5 s |
| phong_3 | Tình Lam Phong Thụ | 0 | bị động | accuracy_pct = 2n, magic_attack_pct = n | | | |
| phong_4 | Mộc Khô Lạp Phong | 3 | circle_target | radius 2 | 160 + 15(n−1) | 25 + 4(n−1) | 4 s |
| phong_5 | Vi Đà Thiên Đài Phong | 4 | circle_target | radius 3 | 200 + 20(n−1) | 45 + 6(n−1) | 7,5 s |

Viêm, Minh, Nham dùng `damage: phys`; Thủy, Phong dùng `magic`. Tầm (`range`) của mọi chiêu theo dòng `range[10]` ở bảng trường; `circle_self` không có tầm, chỉ có bán kính.

**Server kiểm tra khi nạp** (sai thì dừng và báo lỗi, như `monsters.json`):

- JSON không có trường lạ (`DisallowUnknownFields`); `id` không trùng; `class` hợp lệ.
- Mỗi class có đúng năm chiêu: bốn chủ động ở ô 1, 2, 3, 4 với `learn_level` lần lượt 10, 20, 40, 50, và một bị động ô 0 với `learn_level` 30.
- Chủ động có `damage` và `shape` hợp lệ, có đủ và chỉ có thông số vùng của shape đó (`range` với mọi shape trừ `circle_self`; `radius` với `circle_self`/`circle_target`; `cone_deg` với `cone`; `width` với `line`), mọi số đo dương, `cone_deg` trong (0, 360]; `range` không giảm theo cấp; `dash` chỉ đi với `single`; `max_targets` có khi và chỉ khi shape khác `single`.
- Mọi mảng có đúng 10 phần tử; `coef_pct`, `mp`, `cooldown_ms`, `max_targets`, `range` dương; mảng bị động không âm.
- Bị động có ít nhất một khóa `passive` và không có trường của chủ động.

### 2.2. Lưu trữ

Migration `00003_skills.sql`:

- `characters.skill_points int NOT NULL DEFAULT 0 CHECK (skill_points >= 0)`; khi migrate, `UPDATE characters SET skill_points = level - 1`.
- Bảng `character_skills (character_id bigint REFERENCES characters(id) ON DELETE CASCADE, skill_id text NOT NULL, level int NOT NULL CHECK (level BETWEEN 1 AND 10), PRIMARY KEY (character_id, skill_id))`.
- Store nạp chiêu cùng nhân vật và lưu chiêu cùng lần lưu nhân vật (cùng transaction). `skill_id` không có trong `skills.json`, thuộc class khác, hoặc có `learn_level` cao hơn cấp nhân vật (ví dụ sau khi `devchar` hạ cấp) thì bị bỏ khi vào game, ghi một dòng log cảnh báo.
- Học hoặc nâng chiêu được lưu ngay (như cộng điểm tiềm năng). Lên cấp đã lưu ngay từ Giai đoạn 1; nay lưu thêm `skill_points`.
- Hồi chiêu không lưu. Vào lại game thì mọi chiêu sẵn sàng; chấp nhận được vì đăng xuất rồi vào lại lâu hơn hồi chiêu dài nhất 7,5 giây.

### 2.3. Lệnh dev `cmd/devchar`

`go run ./cmd/devchar -u <username> -level <L>` (đọc `DATABASE_URL`, không cờ nào khác):

- Tìm nhân vật đầu tiên (slot nhỏ nhất) của tài khoản; không có thì báo lỗi.
- Chỉ chạy khi nhân vật đã thoát game. Server không ghi trạng thái online xuống DB nên lệnh không tự phát hiện được; nếu nhân vật đang online, lần lưu kế tiếp của server sẽ ghi đè thay đổi. Lệnh in nhắc điều này trước khi ghi. Chỉ dùng cho dev.
- Đặt `level = L` (1 ≤ L ≤ 90), `exp = 0`, trả hết điểm tiềm năng đã cộng về tự do: `str = agi = vit = int = 0`, `free_points = 10 × (L − 1)`; `skill_points = (L − 1) − tổng cấp các chiêu đã học`, nếu số đó âm thì xóa hết chiêu và đặt `skill_points = L − 1`; `hp`, `mp` về NULL (đầy).
- In tóm tắt: tên nhân vật, cấp, điểm tiềm năng, điểm kỹ năng. Không in chuỗi kết nối.

### 2.4. EXP theo cấp quái

**Tỷ lệ EXP trên sát thương:** `r(L) = 0,1 × (1 + 0,1 × (L − 1))` với L là cấp quái: cấp 1 là 0,1 (100 sát thương được 10 EXP), cấp 10 là 0,19, cấp 50 là 0,59, cấp 90 là 0,99.

**EXP gốc của quái** `exp(L) = max(1, round(HP(L) × r(L)))`, tính bằng phân số trong script rồi làm tròn nửa lên. Cách trả EXP theo phần sát thương (game-design mục 5.8) giữ nguyên, chỉ thay `10 × L` bằng `exp(L)`. Hệ quả: EXP trên mỗi điểm sát thương xấp xỉ r(L), tăng theo cấp quái.

**Cửa sổ cấp:** một đòn của người chơi lên quái chỉ trả EXP khi `|cấp nhân vật − cấp quái| ≤ 10`. Đòn ngoài cửa sổ vẫn gây sát thương nhưng không trả EXP và không được ghi vào bảng sát thương tính EXP của con quái. Áp dụng cho cả chém mặc định và kỹ năng.

**Chỉ số quái cấp 1–90:** `monster_stats.py` sinh đủ 90 cấp (hiện chỉ 1–5) và thêm trường `exp` cho mỗi cấp. Quy tắc giữ như Giai đoạn 1 (chưa có trang bị): nhân vật tham chiếu Viêm, 2/3 điểm vào STR, thủ quái `8 × L`, công quái theo mốc 4% HP. HP quái:

- cấp 1–9: bốn chém mặc định của nhân vật tham chiếu (như cũ);
- từ cấp 10: hai chém mặc định cộng hai Bất Tri Hỏa ở cấp kỹ năng `min(10, L − 9)` (game-design mục 5.9), hệ số lấy từ `skill_stats.py`.

Trang bị tham chiếu vào công thức ở Giai đoạn 3; khi đó HP và EXP gốc sinh lại cùng nhau. (Đã làm ở [spec Giai đoạn 3a](2026-11-20-giai-doan-3a-tui-do-roi-do-design.md) mục 2.2: từ cấp 10, quái và bảng EXP lên cấp sinh lại theo trang bị thường +0.)

**Bảng EXP lên cấp:** `exp_table.py` giữ số quái cần hạ cho mỗi cấp như cũ, `kills(L) = (10 + 2 × (L − 1)^1,5) × hệ số cấp(L)`, và tính `EXP lên cấp kế tiếp = round10(kills(L) × exp(L))`. Vì số quái và giả định 5 giây mỗi quái không đổi, thời gian ước tính giữ nguyên (khoảng 3 giờ lên cấp 10, khoảng 456 giờ lên cấp 50). Script lấy `exp(L)` từ `monster_stats.py`, ghi `data/exp.json` (`{"exp_to_next": [89 số cho cấp 1 → 89]}`) và `docs/balance/exp-table.csv`. Đổi HP quái thì chạy lại cả hai script.

Server nạp `exp.json` qua `gamedata` (đúng 89 số dương, tăng dần); `combat.ExpToNext` đọc bảng này thay cho công thức trong Go. `combat.MonsterExp` bỏ; EXP gốc lấy từ `Levels[L].Exp`.

Nhân vật dev có EXP vượt mức của bảng mới sẽ lên cấp ở lần nhận EXP kế tiếp (`GainExp` lặp tới khi EXP nhỏ hơn mức cần).

## 3. Server: học chiêu và chỉ số

### 3.1. Học hoặc nâng chiêu

`LearnSkillReq{skill_id}`. Kiểm tra theo thứ tự, lỗi đầu tiên được trả:

1. Nhân vật chết: `DEAD`.
2. `skill_id` không có hoặc thuộc class khác: `BAD_REQUEST`.
3. Chiêu đã cấp 10: `SKILL_MAXED`.
4. Gọi n là cấp hiện tại (0 nếu chưa học). Cấp nhân vật nhỏ hơn `learn_level + n`: `LEVEL_TOO_LOW`.
5. `skill_points < 1`: `NOT_ENOUGH_POINTS`.

Hợp lệ: trừ 1 điểm, cấp chiêu thành n + 1, lưu ngay, trả `LearnSkillResp{stats, skills}`. Học hoặc nâng bị động thì chỉ số tính lại; HP và MP hiện tại tăng đúng bằng phần tăng của HP và MP tối đa (như cộng điểm tiềm năng, spec Giai đoạn 1 mục 3.5).

Mỗi lần lên cấp nhân vật cộng 1 điểm kỹ năng, cùng lúc với 10 điểm tiềm năng.

### 3.2. Bị động trong chỉ số

`combat.Compute(magic, level, points, passive)` nhận tổng hiệu ứng bị động ở cấp đã học. Tính chỉ số như cũ, rồi với mỗi khóa phần trăm: `chỉ số += floor(chỉ số trước bị động × pct / 100)`; `crit_bp` cộng thẳng vào tỷ lệ chí mạng. `max_hp_pct` áp lên HP tối đa, `magic_attack_pct` lên công phép, `accuracy_pct` lên chính xác, `evasion_pct` lên né tránh, `phys_def_pct`/`magic_def_pct` lên thủ tương ứng.

Theo mục 1.2, né tránh, chính xác và tỷ lệ chí mạng chỉ hiển thị.

## 4. Server: dùng chiêu

### 4.1. Lệnh và hàng chờ

`CastReq{skill_id, target_id, dir_x, dir_y}`. Mỗi người chơi có một chỗ chờ chiêu, tách riêng chỗ chờ chém. Lệnh mới thay lệnh cũ; lệnh cũ nhận `ON_COOLDOWN`. `resolveAttacks` xử lý từng người theo thứ tự id: chém trước, chiêu sau, trước khi ai di chuyển. Hồi chiêu của mỗi chiêu độc lập với nhau và với chém.

### 4.2. Kiểm tra

Theo thứ tự, lỗi đầu tiên được trả; mọi lỗi đều không trừ MP và không bắt đầu hồi chiêu:

1. Nhân vật chết: `DEAD`.
2. Nhân vật đứng trong khu an toàn: `IN_SAFE_ZONE`.
3. Chiêu không có, thuộc class khác, chưa học hoặc là bị động: `SKILL_NOT_LEARNED`.
4. Chưa hết hồi chiêu: `ON_COOLDOWN`. Hồi chiêu tính theo tick: `z.ticks(cooldown_ms / 1000)` của cấp đã học.
5. `mp < mp[n]`: `NOT_ENOUGH_MP`.
6. Mục tiêu theo shape (mục 4.3).

Hợp lệ: trừ MP, ghi tick dùng chiêu, gửi `CastResp`, rồi áp kết quả (mục 4.4–4.5). MP đổi nên `statsDirty` bật và người chơi nhận `SelfStats` cuối tick.

### 4.3. Mục tiêu và hướng

Gọi R = `range[n−1]` của cấp đã học. Tầm chọn mục tiêu R cộng `attackRangeSlack` (0,5 ô) như chém mặc định, vì client thấy quái trễ hai tick. Kích thước vùng (`radius`, `width`, và R khi là độ vươn của quạt hay độ dài của dải) dùng đúng số, không cộng.

- **`single`, `circle_target`:** cần `target_id` là quái còn sống (`INVALID_TARGET`), trong `R + 0,5` và không bị tường chắn từ nhân vật (`OUT_OF_RANGE`).
- **`cone`, `line`:** nếu `target_id` là quái còn sống thì hướng chiêu là từ nhân vật tới quái đó, không xét tầm. Nếu không, hướng là `(dir_x, dir_y)` chuẩn hóa. Không có quái hợp lệ và `dir` bằng 0 (hoặc không hữu hạn): `BAD_REQUEST`.
- **`circle_self`:** không cần mục tiêu; `target_id`, `dir` bị bỏ qua (chỉ dùng `target_id` để ưu tiên, mục 4.4).

### 4.4. Vùng trúng

Mỗi shape có **gốc** (điểm xét tường chắn) và **tâm** (điểm xếp thứ tự ưu tiên):

| Shape | Gốc và tâm | Quái trúng khi |
| --- | --- | --- |
| `single` | | chỉ mục tiêu chính |
| `circle_self` | nhân vật | khoảng cách tới nhân vật ≤ `radius` |
| `circle_target` | mục tiêu chính | khoảng cách tới mục tiêu chính ≤ `radius` |
| `cone` | nhân vật | khoảng cách ≤ R và góc giữa hướng chiêu và hướng tới quái ≤ `cone_deg / 2` |
| `line` | nhân vật | hình chiếu lên hướng chiêu trong [0, R] và khoảng cách vuông góc ≤ `width / 2` |

- Chỉ xét quái còn sống, và từ gốc tới quái không bị tường chắn (`lineBlocked`). Quái trùng vị trí gốc luôn được xét là trong vùng.
- Quét quái bằng lưới ô AOI quanh tâm, đủ phủ vùng lớn nhất (dải 6 ô); không duyệt toàn zone.
- Thứ tự: `target_id` nếu là quái trong vùng, rồi các quái còn lại theo khoảng cách tới tâm tăng dần, hòa thì id nhỏ trước. Cắt ở `max_targets[n]`.
- Chiêu nhiều mục tiêu không trúng ai vẫn hợp lệ: trừ MP, hồi chiêu, phát `Cast`.

### 4.5. Lao tới

Áp cho chiêu `dash`, sau khi qua kiểm tra mục 4.2–4.3 và trước khi tính sát thương:

- Gọi d là khoảng cách nhân vật tới mục tiêu. Nếu d ≤ 1 thì không dời. Nếu không, điểm đáp là điểm trên đoạn nhân vật → mục tiêu, cách mục tiêu 1 ô.
- Điểm đáp phải nằm trên ô đi được, không thuộc khu an toàn, và đoạn nhân vật → điểm đáp không bị tường chắn; sai thì `OUT_OF_RANGE` (kiểm tra trước khi trừ MP).
- Server đặt vị trí mới, cập nhật ô AOI của nhân vật, và đánh dấu để `syncView` phát `EntityMove{snap: true}` cho mọi người thấy, kể cả chính người dùng chiêu. Input di chuyển của tick đó áp tiếp từ điểm đáp.

### 4.6. Sát thương và hệ quả

Với mỗi quái trúng, theo thứ tự ở mục 4.4:

- `base = floor(công × coef_pct[n] / 100)`, công là công vật lý nếu `damage = phys`, công phép nếu `magic`.
- `dmg = combat.Damage(base, thủ của quái, cấp nhân vật)`; sát thương tính EXP tối đa bằng HP còn lại.
- Thêm `Hit{attacker, target, damage, hp_after, skill_id}`.
- Trả EXP theo phần sát thương nếu trong cửa sổ cấp (mục 2.4).
- Ghi người dùng chiêu vào danh sách người đánh của quái; quái chưa đuổi thì đuổi người dùng chiêu (như chém mặc định).
- Quái về 0 HP thì chết.

Phát một `Cast{caster_id, skill_id, target_id, dir_x, dir_y}` mỗi lần dùng chiêu hợp lệ: `target_id` là mục tiêu chính (0 nếu không có), `dir` là hướng đã chốt cho `cone`/`line`, hướng tới mục tiêu cho `single`/`circle_target`, và 0 cho `circle_self`.

### 4.7. Chống spam

Gateway giới hạn `CastReq` bằng một `rate.Limiter` riêng, cùng cách và cùng mức với `AttackReq` (5 lệnh mỗi giây, vượt thì `RATE_LIMITED`). `LearnSkillReq` chỉ chịu giới hạn gói chung, như `AllocatePointsReq`.

## 5. Giao thức

`ProtocolVersion` lên **4**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 57 | `LearnSkillReq` | `string skill_id = 1` |
| 58 | `LearnSkillResp` | `SelfStats stats = 1`, `repeated SkillLevel skills = 2` |
| 59 | `CastReq` | `string skill_id = 1`, `uint64 target_id = 2`, `float dir_x = 3`, `float dir_y = 4` |
| 60 | `CastResp` | rỗng |

- `SkillLevel { string skill_id = 1; int32 level = 2; }`.
- `EnterWorldResp.skills = 10` (`repeated SkillLevel`): toàn bộ chiêu đã học.
- `SelfStats.skill_points = 20`.
- `Cast { uint64 caster_id = 1; string skill_id = 2; uint64 target_id = 3; float dir_x = 4; float dir_y = 5; int32 level = 6; }` (`level` là cấp chiêu đã dùng, để client vẽ đúng tầm; thêm sau đánh giá cuối, không đổi `ProtocolVersion`); `WorldTick.casts = 8`. Gửi cho người thấy người dùng chiêu và chính người đó.
- `Hit.skill_id = 6`: rỗng là chém mặc định hoặc đòn của quái.
- Thứ tự client áp một `WorldTick`: `moves`, `casts`, `hits`, `deaths`, `hp_changes`, `level_ups`. Ghi chú của `WorldTick` trong proto cập nhật theo.
- `ErrorCode`: `SKILL_NOT_LEARNED = 23`, `NOT_ENOUGH_MP = 24`, `SKILL_MAXED = 25`, `LEVEL_TOO_LOW = 26`.
- Tên tuân theo ràng buộc godobuf (CLAUDE.md). Fixture mới: `learn_skill_resp.hex`, `cast_req.hex`, `world_tick_cast.hex` (một `Cast`, hai `Hit` có `skill_id`, một `EntityMove` có `snap`). Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/skill_book.gd`**: nạp `skills.json`; giữ cấp chiêu đã học (từ `EnterWorldResp`, `LearnSkillResp`) và điểm kỹ năng (từ `SelfStats`). Trả về chiêu theo class và ô; MP, hồi chiêu, hệ số, số mục tiêu, hiệu ứng bị động ở cấp n; `learn_block(skill, char_level)` trả lý do không học được (`""`, `"maxed"`, `"level"`, `"points"`). Giữ thời điểm hết hồi chiêu của từng chiêu, đặt khi nhận `CastResp`.
- **`core/cast_loop.gd`**: chạy khi giữ nút Đánh (hoặc Space) với một chiêu chủ động đang được chọn; chém mặc định vẫn do `attack_loop` lo. Mỗi lần chiêu sẵn sàng thì dùng một lần; chiêu đang hồi hoặc thiếu MP mà vẫn giữ Đánh thì chém mặc định thay (qua `attack_loop`) cho tới khi chiêu sẵn sàng.
  - Mỗi chiêu có một **tầm**: `range[n−1]` với `single`/`circle_target`/`cone`/`line`, `radius` với `circle_self`. Không chiêu nào tự lướt tới từng con quái; chỉ chiêu `dash` dời nhân vật, và chỉ tới một mục tiêu (mục 4.5).
  - Chiêu `single`/`circle_target`/`dash` cần mục tiêu đang khóa; không có thì báo "Chưa chọn mục tiêu" và không gửi.
  - Có mục tiêu đang khóa mà nó ở xa hơn tầm của chiêu thì tự chạy lại gần (như `attack_loop`), vào tầm thì gửi; áp dụng cho mọi shape.
  - `cone`/`line`/`circle_self` không khóa mục tiêu thì gửi ngay tại chỗ, theo hướng nhân vật đang nhìn.
  - Trước khi gửi, tự kiểm tra hồi chiêu và MP; thiếu thì không gửi chiêu (chém thay như trên); lần bấm đầu tiên báo "Chưa hồi chiêu" hoặc "Không đủ MP".
  - Đang chạy lại gần mà người chơi tự di chuyển, chọn ô khác hoặc thả nút Đánh thì hủy; đổi mục tiêu thì chạy sang mục tiêu mới (như `attack_loop`).
  - Giữ nút Đánh trong khu an toàn thì không gửi chiêu, nhưng vẫn giữ trạng thái giữ nút: ra khỏi khu an toàn là tiếp tục.
  - `OUT_OF_RANGE` thì chạy thẳng tới mục tiêu và thử lại, như `attack_loop`.
- **`core/entity_table.gd`**: `casts` phát tín hiệu vẽ hiệu ứng; `Hit` có `skill_id` chỉ hiện số sát thương.
- **`core/predictor.gd`**: `EntityMove` của chính mình có `snap` thì đặt vị trí dự đoán bằng vị trí server và giữ lại các input chưa được xác nhận để phát lại từ điểm mới.
- **`core/errors.gd`**: câu tiếng Việt cho bốn mã mới. **`core/wire.gd`**: thêm payload mới vào `PAYLOADS`.

### 6.2. Giao diện (`ui/`)

- **Bố cục HUD** (yêu cầu của người dùng):
  - **Giữa cạnh dưới: `ui/skill_bar.gd`**, năm ô cố định xếp thành một hàng: ô 1 là chém mặc định, ô 2–5 là bốn chiêu chủ động theo cấp học (ô kỹ năng 1–4 của dữ liệu). Bị động chỉ hiện trong tab Kỹ năng. Bấm một ô (hoặc phím 1–5 trên PC) để **chọn** chiêu; ô đang chọn có viền sáng và giữ nguyên tới khi chọn ô khác; vào game mặc định chọn ô 1. Ô chưa học thì xám và không chọn được; đang hồi thì có vòng quét và số giây; thiếu MP thì tối màu.
  - **Góc phải dưới:** nút **Đánh** ở giữa; phía trên nút Đánh là hai nút **bình HP** và **bình MP**. Bình thuốc thuộc Giai đoạn 4 nên hai nút hiện xám, bấm thì báo "Có ở bản sau". Bấm/giữ Đánh (hoặc Space) dùng chiêu đang chọn theo `cast_loop`; ô 1 thì chém như Giai đoạn 1.
  - Nút "Đổi mục tiêu" là nút tròn nhỏ bên trái nút Đánh. **Không có nút "Thông tin" riêng:** bấm vào khung chân dung ở thẻ nhân vật góc trái trên thì mở cửa sổ nhân vật; chấm báo còn điểm tiềm năng/kỹ năng nằm ở góc khung chân dung.
  - **Hồi chiêu trên mọi ô**, kể cả chém mặc định: vừa dùng thành công thì ô phủ xám và hiện số giây còn lại đếm về 0, hết hồi thì sáng lại. Chém mặc định hồi 0,8 giây; chiêu theo `cooldown_ms` của cấp đã học.
  - **Hình dáng** (theo ảnh tham khảo người dùng gửi; chỉ lấy bố cục, không chép icon hay art): ô kỹ năng vuông nền tối viền vàng, số phím 1–5 ghi phía trên mỗi ô, ô đang chọn viền sáng hơn; icon tạm vẽ bằng màu class kèm tên ngắn và cấp chiêu; đang hồi thì phủ tối kèm số giây. Nút Đánh tròn lớn; nút bình HP/MP tròn nhỏ có chỗ hiện số lượng.
  - **Thẻ nhân vật góc trái trên:** khung tròn chân dung tạm, cấp, thanh HP đỏ và thanh MP xanh (theo ảnh tham khảo). Bản đồ nhỏ góc phải trên để Giai đoạn 4.
  - **Nút di chuyển:** joystick đổi hình thành vòng tròn có bốn mũi tên lên/xuống/trái/phải; vẫn kéo đi mọi hướng như Giai đoạn 1 (chỉ đổi hình).
- **Cửa sổ nhân vật `ui/char_window.gd`** (yêu cầu của người dùng): nút "Chỉ số" trên HUD bỏ; bấm khung chân dung mở một cửa sổ có năm tab theo thứ tự **Trang bị, Thông tin, Tiềm năng, Kỹ năng, Hành trang**. Phím `stats` hiện có mở cửa sổ ở tab Thông tin, phím K mở ở tab Kỹ năng; bấm lại phím của tab đang mở hoặc nút đóng thì đóng cửa sổ. Cửa sổ nhớ tab mở gần nhất trong phiên.
  - **Thông tin** (`ui/info_tab.gd`): tên, class, cấp, EXP, các chỉ số dẫn xuất và ghi chú chính xác/né tránh/chí mạng chưa có tác dụng (phần này chuyển từ bảng chỉ số cũ).
  - **Tiềm năng** (`ui/potential_tab.gd`): bốn thuộc tính với − và +, điểm còn lại, Xác nhận và Hủy, giữ nguyên hành vi của bảng chỉ số Giai đoạn 1.
  - **Kỹ năng** (`ui/skill_tab.gd`): năm chiêu của class: thức, tên, cấp n/10, cấp nhân vật cần, mô tả vùng; so sánh cấp hiện tại với cấp kế (hệ số, MP, hồi chiêu, số mục tiêu, hoặc hiệu ứng bị động). Nút "Học"/"Nâng" khóa thì ghi lý do. Đầu tab hiện điểm kỹ năng còn lại.
  - **Trang bị**, **Hành trang**: có tab nhưng nội dung chỉ ghi "Có ở bản sau" (Giai đoạn 3).
  - `ui/stats_panel.gd` được tách thành hai tab Thông tin và Tiềm năng rồi bỏ.
- **`ui/skill_effect.gd`**: hiệu ứng tạm bằng `_draw`, khoảng 0,35 giây, đúng kích thước vùng server: quạt, vòng tròn, dải thẳng, chớp sáng tại mục tiêu (`single`), vệt lướt (`dash`). Màu theo class: Viêm đỏ cam, Thủy xanh nước, Minh vàng, Nham nâu xám, Phong xanh lá.
- **HUD**: chấm báo ở góc khung chân dung khi còn điểm tiềm năng hoặc điểm kỹ năng, và chấm trên tên tab Tiềm năng/Kỹ năng tương ứng; thanh MP giảm khi dùng chiêu.

### 6.3. Smoke

`tests/smoke` thêm bước kỹ năng: tạo tài khoản mới, vào game rồi thoát, chạy `devchar -level 20` cho tài khoản đó, vào lại, học chiêu ô 1 và ô 2, dùng chiêu lên quái. Kết quả in "SMOKE OK" kèm sát thương chiêu và MP đã tiêu.

### 6.4. Đồng bộ

`tools/sync_server.sh` chép thêm `server/data/skills.json` sang `data/skills.json`; `--check` so cả file này.

## 7. Kiểm thử

**Script cân bằng:** `skill_stats.py`, `monster_stats.py`, `exp_table.py` đều có `--print-json`; test Go so với file đã commit. `exp_table.py` tự kiểm tra lên cấp 10 mất 2,8–3,2 giờ, lên cấp 50 trong ±5% của 456 giờ, bảng tăng dần.

**Server** (`go test -race`, mỗi package một hoặc hai file test, chia section):

- `gamedata`: `skills.json` hợp lệ nạp được; lỗi khi thiếu thông số shape, thừa thông số, mảng không đủ 10, class thiếu hoặc trùng ô, `dash` với shape khác `single`, `max_targets` sai chỗ, trường lạ, bị động sai cấp. `exp.json` sai độ dài hoặc không tăng dần thì lỗi. `monsters.json` đủ 90 cấp, có `exp`.
- `combat`: `ExpToNext` theo bảng; `Compute` với bị động (làm tròn xuống, tính trên chỉ số trước bị động, Nham tăng HP tối đa); cửa sổ cấp ở chênh 10 và 11; EXP trên mỗi điểm sát thương của quái cấp cao lớn hơn cấp thấp.
- `store`: migration bù `skill_points`; lưu và nạp chiêu; `skill_id` lạ bị bỏ.
- `zone` học chiêu: từng mã lỗi theo thứ tự; bị động tăng chỉ số, HP, MP; lên cấp cộng điểm kỹ năng.
- `zone` dùng chiêu: từng mã lỗi theo thứ tự; lỗi không trừ MP và không hồi chiêu; hồi chiêu độc lập với chém; quạt không trúng phía sau; biên bán kính; bề rộng dải; `max_targets` và thứ tự ưu tiên; quái sau tường không trúng; chiêu không trúng ai vẫn tốn MP; lao tới đáp cách mục tiêu 1 ô và phát `snap`; đường lao bị chắn hoặc điểm đáp trong khu an toàn bị từ chối; nhân vật cấp 90 đánh quái cấp 5 không nhận EXP; `Cast` chỉ gửi cho người thấy.
- `gateway`: giới hạn tần suất `CastReq`; `ProtocolVersion` là 4; giải mã fixture mới.
- `cmd/devchar`: đặt đúng cấp và điểm; trả điểm tiềm năng; bớt chiêu khi điểm kỹ năng không đủ; tài khoản không có nhân vật thì báo lỗi.
- e2e: nhân vật được đưa lên cấp 20 bằng helper store, vào game, học chiêu ô 1, ô 2, dùng lên quái; MP giảm, có `Hit` mang `skill_id`, dùng lại ngay bị `ON_COOLDOWN`.

**Client** (`tests/run.sh`): `skill_book` (cấp, lý do, hồi chiêu); `cast_loop` (chạy lại gần, gửi ngay, tự kiểm tra); `entity_table` (`Cast`, `Hit` có `skill_id`); `predictor` (`snap` của mình); giải mã fixture mới; `sync --check`. Smoke theo mục 6.3.

**Nghiệm thu:** server chạy ở `:8199`. `devchar` đưa `test1`, `test2` lên cấp 90. Người dùng mở hai cửa sổ: Kaito (Viêm) và Hana (Thủy) học đủ chiêu, dùng lên quái và thấy chiêu của nhau.

## 8. Tài liệu cập nhật

- `docs/game-design.md`: mục 2.2 (công thức và bảng EXP lên cấp mới), 5.8 (EXP theo r(L), cửa sổ ±10 cấp), 5.12 (cột EXP là HP × r(L)).
- `CLAUDE.md`: thêm bất biến `skills.json` sinh bằng `skill_stats.py`, `exp.json` bằng `exp_table.py`; không sửa tay.
- README hai repo: `devchar`, phím 1–5 chọn chiêu, Space/Đánh, K và nút Thông tin.
- Spec Giai đoạn 1: ghi chú "đã thay bằng spec Giai đoạn 2" ở chỗ nói EXP gốc `10 × cấp` và `ExpToNext`.
