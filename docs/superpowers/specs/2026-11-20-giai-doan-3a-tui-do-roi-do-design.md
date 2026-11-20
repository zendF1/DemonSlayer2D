# Giai đoạn 3a — Hành trang, rơi đồ, mặc đồ và Xu

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế (người dùng đã ủy quyền chọn phương án khuyên dùng). Đã triển khai đủ server và client (client 24e9c99, sửa sau review 644a46a); smoke OK.

Spec này thiết kế phần đầu của Giai đoạn 3 (Trang bị) cho server (repo này) và client Godot (`../DemonSlayer2DGame`). Luật chơi lấy từ [game-design.md](../../game-design.md) mục 3.1, 5.9, 5.12 và 7.1; khi hai tài liệu lệch nhau về luật chơi thì game-design là chuẩn, trừ các điểm ghi ở mục 1.4. Những gì không nhắc tới ở đây giữ nguyên như [spec Giai đoạn 2](2026-11-20-giai-doan-2-ky-nang-design.md) và [spec ba mốc của đòn đánh](2026-11-20-moc-don-danh-design.md).

## 1. Mục tiêu và phạm vi

### 1.1. Chia Giai đoạn 3

Lộ trình (spec Giai đoạn 0 mục 1.1) ghi Giai đoạn 3 gồm túi đồ, rơi đồ, mặc đồ, cường hóa, tương sinh, khảm ngọc, tẩy luyện. Phần này quá lớn cho một kế hoạch nên chia ba phần, làm theo thứ tự, mỗi phần xong là chơi được:

| Phần | Nội dung | Kết quả chơi được |
| --- | --- | --- |
| **3a** (spec này) | Hành trang, Xu, rơi đồ từ quái, nhặt, mặc/tháo, hủy, sắp xếp; món đồ sinh đủ phẩm chất, dòng phụ, hệ, dòng ẩn; quái tính lại theo trang bị tham chiếu | Đánh quái nhặt đồ, mặc vào thấy chỉ số tăng |
| 3b ([spec](2026-11-20-giai-doan-3b-cuong-hoa-design.md)) | Cường hóa +1 đến +15, bùa bảo vệ, tỷ lệ ẩn, hồi HP/MP theo mốc cường hóa, vũ khí phát sáng | Đập đồ, thấy hồi máu theo giây và vũ khí sáng |
| 3c ([spec](2026-11-20-giai-doan-3c-tuong-sinh-kham-ngoc-tay-luyen-design.md)) | Tương sinh mở dòng ẩn, đục lỗ và khảm ngọc, tẩy luyện dòng phụ | Phối hệ đồ, khảm ngọc, tẩy dòng |

3a chạy sau khi [spec ba mốc](2026-11-20-moc-don-danh-design.md) đã triển khai: người kết liễu là người có đòn trúng làm quái về 0 HP.

### 1.2. Mục tiêu 3a

Quái chết thì người kết liễu nhận Xu ngay, và có thể có trang bị hoặc đá rơi cạnh xác. Chỉ người đó thấy và nhặt được đồ của mình. Người chơi mở cửa sổ nhân vật, tab **Hành trang** thấy đồ đã nhặt và số Xu, tab **Trang bị** thấy 10 ô đang mặc; bấm vào món để xem chỉ số, mặc, tháo hoặc hủy. Mặc đồ làm chỉ số tăng đúng theo bảng chỉ số đã duyệt. Đồ và Xu còn nguyên sau khi thoát game.

### 1.3. Quyết định đã chốt

Người dùng đã ủy quyền chọn phương án khuyên dùng cho mọi quyết định dưới đây.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Chia giai đoạn | 3a, 3b, 3c như mục 1.1 | Mỗi phần một kế hoạch vừa sức và kết thúc bằng thứ chơi được |
| Món rơi sinh đủ thuộc tính | Lúc rơi đã có phẩm chất, dòng phụ, hệ và hai dòng ẩn (Vàng trở lên). Dòng ẩn lưu nhưng chưa có tác dụng cho tới 3c | Đồ nhặt ở 3a không phải sửa dữ liệu khi 3c bật tương sinh |
| Loại dòng phụ | Năm loại: HP, công vật lý, công phép, thủ vật lý, thủ phép. Một món không có hai dòng phụ cùng loại | Trang bị không cộng MP (game-design 2.7); chính xác, né, chí mạng chưa có tác dụng (spec Giai đoạn 2 mục 1.2), không cho ra dòng vô dụng |
| Giá trị dòng | Giá trị chuẩn S của mỗi loại ở mỗi bậc = max(1, làm tròn nửa lên 4% tổng chỉ số +0 của cả bộ ở bậc đó). Dòng phụ ngẫu nhiên nguyên trong [⌈S/2⌉, S]. Dòng ẩn cố định S × 100/125/150% cho Vàng/Cam/Đỏ | Game-design chỉ chốt tỷ lệ dòng ẩn so với dòng phụ chuẩn; 4% giữ một bộ Vàng đầy dòng ở khoảng +20% so với đồ Trắng |
| Số dòng theo phẩm chất | Trắng 0; Xanh 1–2; Vàng 3–4; Cam 4; Đỏ 4. Vàng, Cam, Đỏ có một hệ ngẫu nhiên trong năm hệ và đúng hai dòng ẩn | Game-design 3.1 và 3.2 ("mỗi món thuộc cơ chế này có hai dòng ẩn") |
| Vũ khí rơi | Đúng class của người kết liễu | Chưa có giao dịch tới Giai đoạn 7; vũ khí class khác chỉ là rác |
| Ô của món rơi | Đều nhau trong 10 ô | Game-design không nói khác |
| Xu | Cộng thẳng cho người kết liễu khi quái chết, không rơi xuống đất; client hiện "+N Xu" | Xu rơi mỗi con, nhặt từng đống rất phiền trên điện thoại |
| Trang bị và đá | Rơi xuống đất cạnh xác. Chỉ người kết liễu thấy và nhặt; 60 giây sau biến mất | Game-design 5.8: đồ rơi thuộc người kết liễu |
| Người kết liễu đã rời zone khi đòn trúng | Không có Xu, không rơi đồ | Không có chủ để giao |
| Nhặt | `PickupReq` từng món; tầm 1,5 ô (+0,5 dung sai như chém); không đủ chỗ cho cả chồng thì từ chối | Đơn giản, không phải tách chồng dưới đất |
| Hành trang | 60 ô; vật phẩm chồng tối đa 999; trang bị không chồng | Đủ cho một buổi cày khi chưa có NPC bán đồ |
| Sắp xếp | Có nút Sắp xếp (gộp chồng, xếp theo loại); không có kéo-thả từng ô | Ít thao tác trên điện thoại, ít lệnh mạng |
| Bán đồ | Chưa có; thay bằng **Hủy** | Bán cho NPC thuộc Giai đoạn 4 |
| Mặc đồ | Đổi chỗ với món đang mặc ở ô đó; cần đủ cấp và đúng class (vũ khí); không cần khu an toàn; chết thì không được | Game-design 3.1 |
| HP khi đổi đồ | Giữ tỷ lệ HP hiện tại trên HP tối đa; đang đầy thì vẫn đầy | Không thể tháo rồi mặc lại để hồi máu |
| Món đang mặc không còn đủ điều kiện | Vẫn ở ô đó nhưng không cộng chỉ số; giao diện ghi đỏ | Chỉ xảy ra khi `devchar` hạ cấp; không mất đồ |
| Trang bị trong công thức chỉ số | Cộng phẳng vào chỉ số nền và tiềm năng, rồi mới áp bị động | Giống `equipment_balance.py` đã duyệt |
| Quái tính lại | `monster_stats.py` cho nhân vật tham chiếu mặc trang bị thường +0 tốt nhất theo cấp; HP, công, EXP gốc của quái từ cấp 10 sinh lại; bảng EXP lên cấp sinh lại, thời gian lên cấp giữ nguyên | Game-design 5.9; spec Giai đoạn 2 mục 2.4 đã hẹn làm ở Giai đoạn 3 |
| Hạng quái | `monsters.json` thêm `rank` (`normal`, `elite`, `boss`); hai loại hiện có là `normal`. Bảng rơi có đủ ba hạng | Tinh anh/thủ lĩnh thêm sau chỉ cần khai báo hạng |
| Cửa sổ cấp cho rơi đồ | Không có | Quái thấp cấp rơi đồ thấp bậc và ít Xu, không cần chặn |
| Lưu | Mọi thay đổi đồ (nhặt, mặc, tháo, hủy, sắp xếp) lưu ngay. Xu từ quái lưu theo nhịp định kỳ và các lần lưu khác | Đồ là tài sản; Xu đổi quá thường xuyên |
| Định danh món | Theo ô hành trang (0–59) hoặc ô trang bị (0–9); không có id món toàn cục | Đủ khi chưa có giao dịch; Giai đoạn 7 thêm nếu cần |
| Đồ dưới đất | Không lưu DB; zone dừng thì mất | Chỉ sống 60 giây |
| Sách kỹ năng | Không có ở Giai đoạn 3. Đến cùng nhiệm vụ ở Giai đoạn 6 (vật phẩm mới trong `items.json` và điều kiện học) | Chưa có nguồn; thêm bây giờ chỉ chặn việc học |
| Bình thuốc, bán đồ, kho, Lượng | Giai đoạn 4 | Lộ trình |
| Xu | Có từ 3a (lộ trình ghi Giai đoạn 4): ví, kiếm từ quái, tiêu ở 3b/3c. Giai đoạn 4 thêm cửa hàng và Lượng | Cường hóa cần Xu |
| Thử đồ bậc cao | Lệnh dev `devitem` thêm món hoặc Xu vào nhân vật đang thoát game | Map hiện chỉ có quái cấp 1–5, rơi đồ bậc 10 |

### 1.4. Khác với game-design

- Game-design 5.9 ghi HP quái thường cấp 30 là 4.338 (tinh anh 26.028, thủ lĩnh 86.760). Script tính theo đúng luật mục đó ra **4.336** (26.016, 86.720); các cấp còn lại khớp bảng. Script là chuẩn; game-design sửa theo.
- Game-design 5.12 không nói Xu rơi xuống đất hay cộng thẳng; spec này chốt cộng thẳng (mục 1.3) và game-design ghi thêm.

### 1.5. Không làm ở 3a

Cường hóa, hồi theo mốc, vũ khí phát sáng (3b); tương sinh, đục lỗ, khảm ngọc, tẩy luyện (3c); bán đồ, bình thuốc, kho, Lượng, NPC (Giai đoạn 4); tổ đội và quyền nhặt trong tổ đội (Giai đoạn 5); tự nhặt theo bộ lọc (Giai đoạn 6); giao dịch, khóa đồ (Giai đoạn 7+); hình ảnh trang bị trên nhân vật; tinh anh và thủ lĩnh trên map.

## 2. Dữ liệu

### 2.1. `data/items.json`

Sinh bằng `python3 docs/balance/item_stats.py --write-json` (`--print-json` in cùng nội dung); không sửa tay. Script import `equipment_balance.generate()` để lấy chỉ số từng món và tự kiểm tra kết quả trùng từng dòng với [CSV chỉ số trang bị](../../balance/equipment-stats-proposal.csv). Server nạp qua `internal/gamedata`; client đọc bản chép ở `data/items.json` của repo client.

```json
{
  "bag_size": 60,
  "stack_max": 999,
  "ground_sec": 60,
  "slots": [
    {"id": "weapon", "name": "Vũ khí", "index": 0},
    {"id": "boots", "name": "Giày", "index": 1}
  ],
  "qualities": [
    {"name": "Trắng", "rank": "Thường", "lines": [0, 0], "hidden_pct": 0},
    {"name": "Vàng", "rank": "Hiếm", "lines": [3, 4], "hidden_pct": 100}
  ],
  "line_stats": ["hp", "phys_attack", "magic_attack", "phys_defense", "magic_defense"],
  "line_roll_min_pct": 50,
  "tiers": [
    {"tier": 10, "name": "Lữ Hành",
     "line_standard": {"hp": 86, "phys_attack": 4, "magic_attack": 4, "phys_defense": 1, "magic_defense": 1}}
  ],
  "equipment": [
    {"code": "eq10_weapon_viem", "tier": 10, "slot": "weapon", "class": "viem",
     "name": "Viêm Nha Lữ Hành Kiếm", "level": 10,
     "hp": [0, 0, 0, 0, 0, 0, 2, 22, 48, 88, 140, 196, 262, 337, 424, 553],
     "phys_attack": [25, 26, 27, 28, 29, 31, 34, 37, 41, 46, 53, 60, 69, 79, 91, 108],
     "magic_attack": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
     "phys_defense": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
     "magic_defense": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]},
    {"code": "eq10_armor", "tier": 10, "slot": "armor", "class": "", "name": "Áo Lữ Hành", "level": 15}
  ],
  "stackables": [
    {"code": "stone_enhance", "name": "Đá cường hóa"},
    {"code": "stone_refine", "name": "Đá tẩy luyện"}
  ],
  "drops": {
    "normal": {
      "xu_per_level": [3, 5],
      "equipment": {"chance_bp": 500, "count": 1, "quality_bp": [8000, 1700, 300, 0, 0]},
      "stacks": [{"code": "stone_enhance", "chance_bp": 2000, "count": 1}]
    }
  }
}
```

(Ví dụ rút gọn; mảng của vũ khí lấy đúng từ CSV, ô `armor` cũng có đủ năm mảng.)

**Ô trang bị** (`slots`, đúng thứ tự game-design 3.1, `index` là vị trí 0–9 và cũng là độ lệch cấp mặc so với bậc):

| index | id | Tên |
| --- | --- | --- |
| 0 | `weapon` | Vũ khí |
| 1 | `boots` | Giày |
| 2 | `bracers` | Hộ uyển |
| 3 | `pants` | Quần |
| 4 | `helmet` | Mũ |
| 5 | `armor` | Áo |
| 6 | `necklace` | Dây chuyền |
| 7 | `pendant` | Ngọc bội |
| 8 | `ring_left` | Nhẫn trái |
| 9 | `ring_right` | Nhẫn phải |

**Phẩm chất** (`qualities`, chỉ số 0–4 là số phẩm chất trong giao thức và DB):

| Số | Tên | Hạng | `lines` (min, max) | `hidden_pct` |
| --- | --- | --- | --- | --- |
| 0 | Trắng | Thường | 0, 0 | 0 |
| 1 | Xanh Dương | Ma thuật | 1, 2 | 0 |
| 2 | Vàng | Hiếm | 3, 4 | 100 |
| 3 | Cam | Thần thoại | 4, 4 | 125 |
| 4 | Đỏ | Hoàng Kim | 4, 4 | 150 |

`hidden_pct > 0` nghĩa là món có hệ và đúng hai dòng ẩn.

**Trang bị** (`equipment`): 5 bậc × (9 món chung + 5 vũ khí) = 70 món. `code` là `eq<bậc>_<ô>` cho món chung và `eq<bậc>_weapon_<class>` cho vũ khí; `class` rỗng với món chung; `name` và chỉ số lấy từ CSV; `level` = bậc + `index` của ô. Năm mảng có 16 phần tử, phần tử n là chỉ số ở +n (3a chỉ dùng +0; mảng đủ để 3b không phải đổi dữ liệu).

**Giá trị chuẩn của dòng** (`tiers[].line_standard`), S = max(1, làm tròn nửa lên 4% tổng +0 của cả bộ, game-design 3.3 "Mốc chỉ số +0 của đủ 10 món"); công vật lý và công phép cùng lấy tổng Công, hai loại thủ cùng lấy "Mỗi loại thủ":

| Bậc | Dòng | HP | Công vật lý, Công phép | Thủ vật lý, Thủ phép |
| --- | --- | --- | --- | --- |
| 10 | Lữ Hành | 86 | 4 | 1 |
| 20 | Thanh Thiết | 156 | 8 | 1 |
| 30 | Tinh Cương | 220 | 14 | 2 |
| 40 | Huyền Cương | 320 | 22 | 3 |
| 50 | Xích Cương | 469 | 32 | 5 |

**Bảng rơi** (`drops`, theo game-design 5.12; `chance_bp` trên 10.000; `quality_bp` theo thứ tự Trắng, Xanh, Vàng, Cam, Đỏ):

| Hạng | `xu_per_level` | Trang bị | `quality_bp` | Đá cường hóa | Đá tẩy luyện |
| --- | --- | --- | --- | --- | --- |
| `normal` | 3, 5 | 500 bp, 1 món | 8000, 1700, 300, 0, 0 | 2000 bp, 1 | không có |
| `elite` | 18, 30 | 10000 bp, 1 món | 0, 7500, 2500, 0, 0 | 10000 bp, 3 | 1000 bp, 1 |
| `boss` | 75, 125 | 10000 bp, 2 món | 0, 2000, 7500, 500, 0 | 10000 bp, 10 | 10000 bp, 1 |

Xu tinh anh và thủ lĩnh là 6 và 25 lần khoảng của quái thường (game-design 5.12).

**Server kiểm tra khi nạp** (sai thì dừng và báo lỗi, như `skills.json`):

- Không có trường lạ; `bag_size` trong 1–200; `stack_max` ≥ 1; `ground_sec` > 0; `line_roll_min_pct` trong 1–100.
- `slots` đúng 10 mục, đúng thứ tự id ở bảng trên, `index` bằng vị trí.
- `qualities` đúng 5 mục; 0 ≤ min ≤ max ≤ số loại dòng; `hidden_pct` bằng 0 ở Trắng, Xanh và dương ở Vàng, Cam, Đỏ.
- `line_stats` khác nhau, mỗi mục thuộc năm khóa ở trên.
- Mỗi bậc có `line_standard` cho mọi loại dòng, giá trị ≥ 1; bậc không trùng.
- `equipment`: `code` không trùng; bậc có trong `tiers`; ô hợp lệ; vũ khí có `class` hợp lệ, món khác `class` rỗng; `level` = bậc + `index`; năm mảng đúng 16 số không âm và không giảm; vũ khí class vật lý có `magic_attack` toàn 0, class phép có `phys_attack` toàn 0; mỗi bậc đủ 9 món chung và 5 vũ khí.
- `stackables`: `code` không trùng và không trùng mã trang bị; tên khác rỗng.
- `drops`: đúng ba khóa `normal`, `elite`, `boss`; 0 ≤ min ≤ max của `xu_per_level`; `chance_bp` trong 0–10000; `count` ≥ 1; `quality_bp` 5 số không âm, tổng 10000; mỗi mục `stacks` trỏ tới một `stackables` có thật.

### 2.2. `monsters.json` và `exp.json` sinh lại

- `monster_stats.py` lấy trang bị tham chiếu từ `equipment_balance.py`: ở cấp L, mỗi ô mặc món thường +0 có bậc cao nhất mà bậc + `index` ≤ L (hàm `available_tier`), không mặc gì nếu chưa đủ cấp. Công, HP và thủ của nhân vật tham chiếu cộng tổng chỉ số các món đó. Các luật khác giữ nguyên (spec Giai đoạn 2 mục 2.4).
- Cấp 1–9 không đổi (chưa mặc được món nào). Script chốt cứng các giá trị của game-design 5.9: cấp 10 là 750/95/80, 20 là 2.704/319/160, 40 là 6.240/694/320, 50 là 8.408/934/400, 60 là 10.452/1.211/480, 90 là 13.478/1.412/720 (HP/công/thủ), và cấp 30 là 4.336/508/240 (mục 1.4).
- Mỗi loại quái thêm `"rank": "normal"`; `gamedata.Kind.Rank` phải là `normal`, `elite` hoặc `boss`.
- `exp_table.py` chạy lại: EXP gốc quái từ cấp 10 tăng nên EXP lên cấp kế tiếp tăng theo, số quái cần hạ và giờ ước tính giữ nguyên. Ví dụ EXP gốc cấp 10 đổi từ 125 thành 143.
- Hệ quả cần biết: quái từ cấp 10 đánh mạnh hơn trước (cấp 20: công 169 thành 319) vì mốc 4% HP nay tính trên HP có trang bị. Nhân vật không mặc đồ ở cấp 20 mất khoảng 7–8% HP mỗi đòn.

## 3. Lưu trữ

Migration `00004_items.sql`:

- `characters.xu bigint NOT NULL DEFAULT 0 CHECK (xu >= 0)`.
- Bảng `character_items`:

| Cột | Kiểu | Ghi chú |
| --- | --- | --- |
| `character_id` | `bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE` | |
| `equipped` | `boolean NOT NULL` | đang mặc hay trong hành trang |
| `slot` | `smallint NOT NULL CHECK (slot >= 0)` | ô hành trang hoặc ô trang bị |
| `code` | `text NOT NULL` | |
| `count` | `int NOT NULL CHECK (count >= 1)` | 1 với trang bị |
| `quality` | `smallint NOT NULL DEFAULT 0 CHECK (quality BETWEEN 0 AND 4)` | |
| `element` | `text NOT NULL DEFAULT '' CHECK (element IN ('', 'viem', 'thuy', 'minh', 'nham', 'phong'))` | |
| `enhance` | `smallint NOT NULL DEFAULT 0 CHECK (enhance BETWEEN 0 AND 15)` | luôn 0 ở 3a |
| `lines` | `jsonb NOT NULL DEFAULT '[]'` | `[{"stat": "hp", "value": 43}]` |
| `hidden` | `jsonb NOT NULL DEFAULT '[]'` | cùng dạng |

Khóa chính `(character_id, equipped, slot)`. Down xóa bảng và cột.

- Store nạp đồ và Xu cùng nhân vật (một truy vấn thêm như `loadSkills`); `SaveCharacterState` lưu Xu trong `UPDATE` và thay toàn bộ dòng `character_items` của nhân vật trong cùng transaction (xóa rồi chèn), như chiêu ở Giai đoạn 2.
- Khi vào game (`world.Enter`), dòng nào sai thì bỏ và ghi một dòng log cảnh báo: mã không có trong `items.json`; ô hành trang ≥ `bag_size`; ô trang bị ngoài 0–9 hoặc khác ô của món; trùng ô; `count` vượt `stack_max` hoặc khác 1 với trang bị; vật phẩm chồng có `quality`, `element`, `enhance`, dòng; trang bị có dòng với loại lạ.
- Zone giao cho saver bản sao sâu của đồ (saver chạy ở goroutine khác).

## 4. Server: chỉ số và hành trang

### 4.1. Món đồ trong zone

Gói mới `internal/items` (thuần, không I/O) giữ mô hình món đồ và các luật: tạo món rơi, cộng chồng, tính chỉ số trang bị. Zone chỉ gọi gói này và chuyển kết quả thành tin nhắn.

- Một món: mã, số lượng, phẩm chất, hệ, mức cường hóa, dòng phụ (danh sách loại + giá trị, theo thứ tự sinh), dòng ẩn (hai mục hoặc rỗng).
- Hành trang: `bag_size` ô, mỗi ô rỗng hoặc một món; 10 ô trang bị.
- Thêm một món vào hành trang (nhặt): vật phẩm chồng trước hết dồn vào các chồng cùng mã còn chỗ theo thứ tự ô tăng dần, phần còn lại vào ô trống đầu tiên; trang bị vào ô trống đầu tiên. Nếu không chứa hết thì không đổi gì và báo đầy (tất cả hoặc không).

### 4.2. Chỉ số trang bị

`combat.Compute(magic, level, points, gear, passive)` nhận thêm `gear` là tổng chỉ số trang bị (HP, công vật lý, công phép, thủ vật lý, thủ phép). Tính chỉ số nền và tiềm năng như cũ, cộng `gear`, rồi mới áp bị động lên tổng đó (spec Giai đoạn 2 mục 3.2 nay hiểu "chỉ số trước bị động" là đã gồm trang bị).

Tổng `gear` cộng từng món đang mặc **đủ điều kiện mặc** (đủ cấp; vũ khí đúng class):

- năm chỉ số của món ở mức `enhance` (3a luôn +0);
- các dòng phụ;
- dòng ẩn chưa tính ở 3a (3c bật).

Món đang mặc nhưng không đủ điều kiện (chỉ xảy ra sau `devchar` hạ cấp) vẫn ở ô đó, không cộng gì. Lên cấp thì tính lại nên món có thể có tác dụng trở lại.

Ví dụ Viêm cấp 10, 60 STR/30 VIT, mặc Viêm Nha Lữ Hành Kiếm Trắng +0: công vật lý 185 + 25 = 210, HP 1.640.

### 4.3. Rơi đồ khi quái chết

Khi một đòn trúng đưa quái về 0 HP (`applyToMonster`), người ra đòn là người kết liễu. Nếu người đó không còn trong zone: không Xu, không rơi gì. Nếu còn (kể cả đã chết), với L là cấp quái và bảng `drops` theo `rank` của loại quái:

1. **Xu:** ngẫu nhiên nguyên đều trong [min × L, max × L]; cộng vào ví, `statsDirty` bật.
2. **Trang bị:** với `chance_bp`, rơi `count` món. Mỗi món độc lập: bậc = min(50, max(10, 10 × ⌊L / 10⌋)); ô đều trong 10 ô; vũ khí lấy đúng class người kết liễu; phẩm chất theo `quality_bp`; rồi sinh dòng (mục 4.4).
3. **Vật phẩm chồng:** mỗi mục `stacks` độc lập, với `chance_bp` rơi một chồng `count` viên.

Mọi lượt dùng chung bộ số ngẫu nhiên của zone (`z.rng`), theo đúng thứ tự trên để test tái lập được.

### 4.4. Sinh dòng của món

Với phẩm chất q, bậc t, S = `line_standard` của bậc t:

- Số dòng phụ ngẫu nhiên đều trong [min, max] của q. Chọn loại không lặp, đều trong năm loại. Giá trị mỗi dòng ngẫu nhiên nguyên đều trong [⌈S × `line_roll_min_pct` / 100⌉, S] của loại đó.
- Nếu `hidden_pct > 0`: hệ đều trong năm hệ (`viem`, `thuy`, `minh`, `nham`, `phong`); hai dòng ẩn, mỗi dòng một loại đều trong năm loại (hai dòng ẩn được trùng loại với nhau và với dòng phụ), giá trị = làm tròn nửa lên S × `hidden_pct` / 100.
- Trắng, Xanh: không hệ, không dòng ẩn.

Ví dụ: Áo Lữ Hành Cam có 4 dòng phụ khác loại, mỗi dòng HP nằm trong 43–86; dòng ẩn HP luôn là 108.

### 4.5. Đồ dưới đất

- Mỗi món hoặc chồng rơi là một thực thể mới loại `ITEM`, id lấy từ `Config.NewID`, chủ là **id nhân vật** (`charID`) của người kết liễu, nằm trong lưới AOI như quái.
- Vị trí: n món rơi cùng lúc thì món thứ i (0-based) đặt cách xác 0,6 ô theo góc 2π·i/n; n = 1 thì đặt đúng chỗ xác. Ô ở vị trí đó không đi được thì đặt ở chỗ xác.
- Chỉ chủ thấy: `syncView` bỏ qua thực thể `ITEM` của chủ khác (như cách bỏ xác quái). Chủ thoát rồi vào lại cùng zone trong 60 giây vẫn thấy, vì chủ tính theo nhân vật.
- Sau `ground_sec` (60 giây) kể từ lúc rơi thì mất. Bước `runGroundTimers` chạy ngay sau `runSpawnTimers`. Mất hoặc được nhặt thì thực thể rời lưới AOI, chủ nhận `EntityLeave` ở `syncView`.

### 4.6. Nhặt

`PickupReq{item_id}`. Kiểm tra theo thứ tự, lỗi đầu tiên được trả:

1. Nhân vật chết: `DEAD`.
2. Không có thực thể `ITEM` với id đó, hoặc chủ khác: `ITEM_NOT_FOUND`.
3. Khoảng cách nhân vật tới món > 1,5 + 0,5 ô: `OUT_OF_RANGE`.
4. Hành trang không chứa hết (mục 4.1): `BAG_FULL`.

Hợp lệ: thêm vào hành trang, xóa thực thể, lưu ngay, trả `PickupResp{update}` với các ô hành trang đã đổi.

### 4.7. Mặc, tháo, hủy, sắp xếp

**`EquipReq{bag_slot}`:**

1. Chết: `DEAD`.
2. `bag_slot` ngoài [0, `bag_size`): `BAD_REQUEST`.
3. Ô rỗng: `ITEM_NOT_FOUND`.
4. Món không phải trang bị: `BAD_REQUEST`.
5. Vũ khí khác class: `WRONG_CLASS`.
6. Cấp nhân vật < `level` của món: `LEVEL_TOO_LOW`.

Hợp lệ: đổi chỗ món ở ô hành trang với ô trang bị tương ứng (ô trang bị rỗng thì ô hành trang thành rỗng); tính lại chỉ số; áp luật HP (dưới đây); lưu ngay; trả `EquipResp{update, stats}`.

**`UnequipReq{equip_slot}`:** chết `DEAD` → ô ngoài 0–9 `BAD_REQUEST` → ô rỗng `ITEM_NOT_FOUND` → hành trang không còn ô trống `BAG_FULL`. Hợp lệ: chuyển vào ô trống đầu tiên, tính lại, áp luật HP, lưu, trả `UnequipResp{update, stats}`.

**`DiscardReq{bag_slot, count}`:** chết `DEAD` → ô ngoài phạm vi `BAD_REQUEST` → ô rỗng `ITEM_NOT_FOUND` → `count` < 0 hoặc lớn hơn số lượng `BAD_REQUEST`. `count` = 0 nghĩa là hủy cả ô. Hợp lệ: bớt số lượng (về 0 thì ô rỗng), lưu, trả `DiscardResp{update}`. Không có xác nhận ở server; client hỏi lại người chơi.

**`SortBagReq{}`:** chết `DEAD`. Gộp các chồng cùng mã (mỗi chồng tối đa `stack_max`), rồi xếp từ ô 0: trang bị trước, theo bậc giảm dần, rồi `index` ô tăng dần, phẩm chất giảm dần, mức cường hóa giảm dần, mã; sau đó vật phẩm chồng theo thứ tự trong `stackables`, chồng lớn trước. Lưu, trả `SortBagResp{update}` với **mọi** ô hành trang (ô rỗng không kèm món).

**Luật HP khi chỉ số trang bị đổi** (mặc, tháo; về sau cả cường hóa, khảm, tẩy luyện món đang mặc): gọi M cũ và M mới là HP tối đa trước và sau. HP = M cũ thì HP mới = M mới; nếu không, HP mới = max(1, ⌊HP × M mới / M cũ⌋). MP không đổi (trang bị không cộng MP). HP tối đa đổi thì `hpChanged` để người xung quanh thấy thanh máu mới.

### 4.8. Chống spam

Các lệnh đồ chỉ chịu giới hạn gói chung (30 gói mỗi giây), như `LearnSkillReq`.

### 4.9. Thứ tự tick

```
drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runGroundTimers → runPlayerTimers → syncView
```

Lệnh đồ được xử lý ngay trong `drain`, như cộng điểm tiềm năng. Quái chết và rơi đồ trong `runImpacts`.

## 5. Giao thức

`ProtocolVersion` lên **6**.

| Envelope | Message | Trường |
| --- | --- | --- |
| 61 | `PickupReq` | `uint64 item_id = 1` |
| 62 | `PickupResp` | `InventoryUpdate update = 1` |
| 63 | `EquipReq` | `int32 bag_slot = 1` |
| 64 | `EquipResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 65 | `UnequipReq` | `int32 equip_slot = 1` |
| 66 | `UnequipResp` | `InventoryUpdate update = 1`, `SelfStats stats = 2` |
| 67 | `DiscardReq` | `int32 bag_slot = 1`, `int32 count = 2` |
| 68 | `DiscardResp` | `InventoryUpdate update = 1` |
| 69 | `SortBagReq` | rỗng |
| 70 | `SortBagResp` | `InventoryUpdate update = 1` |

Message mới:

```proto
// An item in the bag, worn or on the ground.
message ItemInfo {
  string code = 1;
  int32 count = 2;      // 1 for equipment
  int32 quality = 3;    // 0 Trắng … 4 Đỏ
  CharClass element = 4; // UNSPECIFIED: no element
  int32 enhance = 5;
  repeated ItemLine lines = 6;
  repeated ItemLine hidden = 7;
}

message ItemLine {
  string stat = 1; // hp, phys_attack, magic_attack, phys_defense, magic_defense
  int32 value = 2;
}

// A bag slot (0..bag_size-1) or an equipment slot (0..9). No item: the slot is empty.
message ItemSlot {
  int32 slot = 1;
  ItemInfo item = 2;
}

message Inventory {
  int32 bag_size = 1;
  repeated ItemSlot bag = 2;      // occupied slots only
  repeated ItemSlot equipped = 3; // occupied slots only
}

// Slots that changed; an ItemSlot without an item is now empty.
message InventoryUpdate {
  repeated ItemSlot bag = 1;
  repeated ItemSlot equipped = 2;
}
```

- `EnterWorldResp.inventory = 11` (`Inventory`).
- `SelfStats.xu = 21` (`int64`).
- `EntityKind`: `ENTITY_KIND_ITEM = 3`. `EntityEnter.item = 13` (`ItemInfo`): có với thực thể `ITEM`; `name` là tên món, HP và cấp bằng 0.
- `ErrorCode`: `BAG_FULL = 27`, `ITEM_NOT_FOUND = 28`, `WRONG_CLASS = 29`, mỗi mã một dòng chú thích.
- Tên theo ràng buộc godobuf (CLAUDE.md). Fixture mới: `enter_world_resp_inventory.hex` (hai món trong hành trang, một món đang mặc), `entity_enter_item.hex` (một món Vàng dưới đất, có dòng và dòng ẩn), `equip_resp.hex`. Fixture cũ sinh lại.

## 6. Client Godot

### 6.1. Lõi (`core/`, không dùng Node)

- **`core/item_book.gd`**: nạp `items.json`; tra tên, ô, bậc, cấp mặc, class, chỉ số ở +n; tên và màu phẩm chất (Trắng xám nhạt, Xanh Dương xanh, Vàng vàng, Cam cam, Đỏ đỏ); tên loại dòng ("HP", "Công vật lý", "Công phép", "Thủ vật lý", "Thủ phép"); tên hệ. `equip_block(item, char_level, cls)` trả `""`, `"class"` hoặc `"level"`, cùng thứ tự với server.
- **`core/inventory.gd`**: giữ `bag_size`, các ô hành trang, 10 ô trang bị và Xu; nạp từ `Inventory` (`EnterWorldResp`), áp `InventoryUpdate`, lấy Xu từ `SelfStats`; `can_take(item)` mô phỏng mục 4.1 để client biết trước "Hành trang đã đầy"; `xu_gained(new_xu)` trả phần tăng để hiện "+N Xu".
- **`core/pickup_loop.gd`**: như `attack_loop`: nhận một id đồ dưới đất, đi tới cách món 1,2 ô rồi gửi `PickupReq` một lần; tự di chuyển, chọn món khác hoặc món biến mất thì hủy; `OUT_OF_RANGE` thì đi thẳng tới và thử lại; `BAG_FULL` thì dừng và báo. Chế độ **Nhặt hết**: sau mỗi lần nhặt xong tự chuyển sang món của mình gần nhất trong 6 ô, cho tới khi hết.
- **`core/entity_table.gd`**: `EntityEnter` loại `ITEM` tạo mục đồ dưới đất (id, vị trí, `ItemInfo`); không có HP, không bị chọn làm mục tiêu; `ground_items()` cho `pickup_loop` và nút Nhặt.
- **`core/errors.gd`**: BAG_FULL "Hành trang đã đầy", ITEM_NOT_FOUND "Không tìm thấy vật phẩm", WRONG_CLASS "Không đúng class". **`core/wire.gd`**: thêm payload mới vào `PAYLOADS`.

### 6.2. Giao diện (`ui/`)

- **Đồ dưới đất** (`ui/ground_item_view.gd`): hình thoi nhỏ màu phẩm chất (vật phẩm chồng màu trắng), tên ngắn bên dưới, có số lượng nếu là chồng. Chạm hoặc bấm vào món (trong 0,6 ô) thì chạy `pickup_loop` thay vì chọn mục tiêu.
- **Nút Nhặt** trên HUD: nút tròn nhỏ ở cụm góc phải dưới, phía trên nút Đổi mục tiêu. Bấm thì chạy chế độ Nhặt hết; không có món nào trong 6 ô thì báo "Không có vật phẩm gần đây".
- **"+N Xu"**: chữ vàng bay lên trên nhân vật mình khi Xu tăng.
- **Tab Hành trang** (`ui/bag_tab.gd`): đầu tab ghi "Xu: 12.345" và nút **Sắp xếp**; dưới là lưới 6 cột × 10 hàng cuộn được. Mỗi ô: viền màu phẩm chất, tên viết tắt, số lượng ở góc với vật phẩm chồng. Chạm một ô mở ô thông tin món, có nút **Mặc** (trang bị) và **Hủy**; Hủy hỏi lại "Hủy <tên>? Không lấy lại được." rồi gửi `DiscardReq{count: 0}`.
- **Tab Trang bị** (`ui/equip_tab.gd`): 10 ô quanh khung chân dung tạm. Cột trái: Vũ khí, Mũ, Áo, Quần, Giày; cột phải: Dây chuyền, Ngọc bội, Hộ uyển, Nhẫn trái, Nhẫn phải. Ô rỗng ghi tên ô mờ. Giữa là tổng chỉ số trang bị (HP, công vật lý, công phép, mỗi loại thủ). Chạm ô có đồ mở ô thông tin món, có nút **Tháo**. Món không đủ điều kiện mặc có viền đỏ.
- **Ô thông tin món** (`ui/item_tooltip.gd`, dùng chung hai tab): tên màu phẩm chất; "Phẩm chất · Ô · Bậc"; "Cấp yêu cầu N" (đỏ khi chưa đủ); class với vũ khí (đỏ khi khác class); năm chỉ số khác 0 ở mức hiện tại; danh sách dòng phụ; với Vàng trở lên: "Hệ: <tên>" và hai dòng ẩn màu xám kèm chú thích "Kích hoạt bằng tương sinh (có ở bản sau)". Với món trong hành trang cùng ô với món đang mặc, thêm dòng "Đang mặc: <tên>".
- Thay "Có ở bản sau" của tab Trang bị và Hành trang ở `ui/char_window.gd` bằng hai tab mới.
- `screens/world.gd` nối: `Inventory` từ `EnterWorldResp`, các yêu cầu và phản hồi, lỗi ra dòng thông báo.

### 6.3. Smoke

`tests/smoke` sau bước kỹ năng: đánh tối đa 20 quái, kiểm tra Xu tăng sau con đầu tiên, nhặt món đầu tiên rơi ra (nếu có trong 20 con) và thấy nó trong hành trang. In "SMOKE OK" kèm Xu đã nhận và món đã nhặt (hoặc "không rơi đồ").

### 6.4. Đồng bộ

`tools/sync_server.sh` chép thêm `server/data/items.json` sang `data/items.json`; `--check` so cả file này.

## 7. Lệnh dev `cmd/devitem`

`go run ./cmd/devitem -u <username> [-code <mã>] [-count <n>] [-quality <0-4>] [-xu <n>]` (đọc `DATABASE_URL` và `DATA_DIR`, mặc định `./data`):

- Nhân vật đầu tiên (slot nhỏ nhất) của tài khoản; phải đã thoát game (như `devchar`, lệnh in nhắc trước khi ghi).
- `-code` là trang bị: thêm một món vào ô hành trang trống đầu tiên, phẩm chất `-quality` (mặc định 0), dòng và hệ sinh theo mục 4.4 với hạt giống ngẫu nhiên. `-code` là vật phẩm chồng: thêm `-count` (mặc định 1) theo luật mục 4.1. Không đủ chỗ thì báo lỗi, không ghi gì.
- `-xu` cộng thêm Xu (có thể dùng một mình).
- In tóm tắt: tên nhân vật, món đã thêm và ô, Xu hiện có.

## 8. Kiểm thử

**Script cân bằng:** `item_stats.py` có `--print-json`; tự kiểm tra trang bị khớp CSV, bảng S đúng mục 2.1, tổng `quality_bp` là 10000. `monster_stats.py` chốt các giá trị mục 2.2. `exp_table.py` giữ các kiểm tra giờ của Giai đoạn 2. Test Go so ba file JSON với script.

**Server** (`go test -race`, mỗi package một hoặc hai file test, chia section):

- `gamedata`: `items.json` hợp lệ nạp được (70 trang bị, 2 vật phẩm chồng, 3 hạng rơi); mỗi vi phạm ở mục 2.1 bị từ chối; `rank` lạ bị từ chối.
- `combat`: `Compute` với `gear` rỗng giữ nguyên kết quả cũ; `gear` cộng trước bị động (Nham có bị động 20% HP mặc món +200 HP thì HP tăng 240).
- `items`: sinh dòng đúng số, không trùng loại, trong khoảng; dòng ẩn đúng giá trị theo phẩm chất; Trắng/Xanh không hệ; bậc rơi ở cấp quái 1, 19, 20, 55, 90; vũ khí đúng class; tỷ lệ phẩm chất xấp xỉ bảng qua 100.000 lượt với hạt giống cố định; cộng chồng dồn vào chồng cũ, sang ô mới, tất cả hoặc không; tổng `gear` bỏ món không đủ điều kiện; Viêm cấp 10 mặc kiếm Lữ Hành ra công 210.
- `store`: lưu và nạp lại đủ đồ, dòng, Xu; lưu danh sách rỗng xóa đồ cũ; xóa nhân vật xóa đồ.
- `world`: dòng sai (mã lạ, ô ngoài phạm vi, sai ô trang bị, trùng ô) bị bỏ và ghi log, dòng còn lại giữ.
- `zone` rơi đồ: quái chết cho người kết liễu Xu trong khoảng; với dữ liệu rơi 100% thì có thực thể `ITEM` cạnh xác; người khác không nhận `EntityEnter` của món; người kết liễu rời zone trước khi đòn trúng thì không rơi; món mất sau 60 giây và chủ nhận `EntityLeave`.
- `zone` nhặt: từng mã lỗi theo thứ tự; nhặt chồng dồn vào chồng cũ; hành trang đầy thì món còn dưới đất; nhặt xong lưu ngay.
- `zone` mặc/tháo/hủy/sắp xếp: từng mã lỗi theo thứ tự; đổi chỗ khi ô đã có món; chỉ số tăng đúng; luật HP (đầy vẫn đầy; 500/1.000 lên HP tối đa 1.500 thành 750; tháo rồi mặc lại không tăng HP); `hpChanged` khi HP tối đa đổi; sắp xếp gộp chồng và đúng thứ tự.
- `gateway`: `ProtocolVersion` là 6; giải mã fixture mới.
- `cmd/devitem`: thêm trang bị đúng ô; thêm chồng dồn vào chồng cũ; hành trang đầy thì lỗi; `-xu`; tài khoản không có nhân vật thì lỗi.
- e2e `TestItemsOverWebSocket`: dùng bản sao `data/` có bảng rơi 100%; nhân vật được `DevSetLevel` lên 19, vào game, hạ một quái, thấy Xu tăng và món dưới đất, nhặt, mặc (luôn mặc được: quái cấp 1–5 rơi đồ Lữ Hành, cấp 19 mặc được cả bộ và vũ khí đúng class), chỉ số tăng; thoát rồi vào lại, hành trang và đồ đang mặc còn nguyên.

**Client** (`tests/run.sh`): `item_book` (tên, chỉ số +0, `equip_block`); `inventory` (nạp, áp cập nhật, ô rỗng, `can_take`, Xu tăng); `pickup_loop` (đi tới rồi gửi một lần, hủy khi tự đi, Nhặt hết chuyển món kế tiếp); `entity_table` (thực thể `ITEM`); giải mã fixture mới; `sync --check`. Smoke theo mục 6.3.

**Nghiệm thu:** server ở `:8199`. `test1` (Kaito, Viêm) và `test2` (Hana, Thủy) cấp 90 hạ quái ở Kumotori, nhặt đồ Lữ Hành, mặc vào và thấy chỉ số đổi. `devitem` cho mỗi người vài món Xích Cương Vàng để thử mặc đủ bộ bậc 50.

## 9. Tài liệu cập nhật

- `docs/game-design.md`: mục 3.1 (loại dòng phụ, bảng S, cách sinh giá trị, số dòng và hệ theo phẩm chất; hành trang 60 ô, chồng 999); mục 5.9 (HP quái cấp 30 là 4.336, tinh anh 26.016, thủ lĩnh 86.720); mục 5.12 (Xu cộng thẳng cho người kết liễu; đồ rơi dưới đất chỉ người kết liễu thấy, 60 giây; vũ khí rơi đúng class người kết liễu); mục 2.2 (bảng EXP lên cấp với số mới từ CSV, giờ không đổi).
- Spec Giai đoạn 0 mục 1.1: ghi chú Xu có từ Giai đoạn 3; Giai đoạn 4 còn cửa hàng và Lượng.
- Spec Giai đoạn 2: ghi chú "đã làm ở spec 3a" tại câu "Trang bị tham chiếu vào công thức ở Giai đoạn 3".
- `CLAUDE.md`: bất biến `items.json` sinh bằng `item_stats.py`; `monster_stats.py` đọc trang bị từ `equipment_balance.py`, đổi chỉ số trang bị thì chạy lại `item_stats.py`, `monster_stats.py`, `exp_table.py`; đồ rơi thuộc người kết liễu; thứ tự tick mới.
- README hai repo: `devitem`, nút Nhặt, tab Trang bị và Hành trang.
