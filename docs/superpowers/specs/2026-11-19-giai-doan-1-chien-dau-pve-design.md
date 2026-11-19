# Giai đoạn 1 — Chiến đấu PvE cơ bản

Ngày: 2026-11-19. Trạng thái: Đã triển khai (2026-11-20); còn chờ người dùng chơi thử hai cửa sổ (mục 9).

Tài liệu này thiết kế Giai đoạn 1 cho cả server (repo này) lẫn client Godot (`../DemonSlayer2DGame`). Luật chơi lấy từ [game-design.md](../../game-design.md); khi hai tài liệu khác nhau về luật chơi, game-design là chuẩn. Các phần không nhắc tới ở đây giữ nguyên như [spec server Giai đoạn 0](2026-11-18-giai-doan-0-nen-mong-server-design.md) và [spec client Giai đoạn 0](2026-11-19-giai-doan-0-client-design.md).

## 1. Mục tiêu và phạm vi

### 1.1. Mục tiêu

Người chơi vào `kumotori_1`, thấy quái đi lang thang trong bãi, chọn mục tiêu, giữ nút để chém, hạ quái nhận EXP, lên cấp, cộng điểm tiềm năng, chết thì hồi sinh ở khu an toàn, và hồi đầy HP/MP ở điểm nghỉ. Hai client cùng map thấy đòn đánh và số sát thương của nhau.

### 1.2. Cách chia giai đoạn từ đây

Từ Giai đoạn 1, mỗi giai đoạn có một spec chung cho server và client, để cuối giai đoạn mở game lên là chơi được. Kế hoạch triển khai vẫn chia task theo từng repo.

### 1.3. Quyết định đã chốt

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Hồi HP khi chưa có bình, NPC, cường hóa (game-design mục 2.7) | Thêm khu an toàn có điểm nghỉ trên `kumotori_1` | Đúng luật mục 5.4; lớp trigger dùng lại được ở Giai đoạn 4 |
| Trượt và chí mạng (mục 2.2 ghi "chưa chốt") | Luôn trúng, không chí mạng. Chính xác, né tránh, tỷ lệ chí mạng chỉ hiển thị | Mọi số cân bằng đã chốt đều đo khi không trượt, không chí mạng |
| Quái | Chỉ quái thường. Tinh anh Quỷ lang thang đầu đàn và chiêu Vồ để Giai đoạn 2 cùng khung kỹ năng | Đủ cho mục tiêu "đánh quái lên cấp" |
| Tự chạy tới mục tiêu và đánh lặp | Client lái bằng `MoveInput`; giữ nút thì đánh lặp | Dự đoán vị trí của mình vẫn đúng; server chỉ kiểm tra |
| Gửi diễn biến chiến đấu | Gói theo tick trong `WorldTick`, thêm push `SelfStats` cho chỉ số của mình | Một khung WebSocket mỗi tick; sự kiện gắn đúng tick |
| Quái khi rảnh | Đi lang thang quanh điểm sinh | Bãi quái có chuyển động |
| Lên cấp | Hồi đầy HP/MP | Giai đoạn 1 không có cách hồi nào khác ngoài điểm nghỉ. Game-design chưa ghi luật này |
| EXP theo sát thương | Mỗi đòn trúng trả ngay EXP theo phần sát thương trên HP tối đa của quái, thay vì người kết liễu nhận hết (mục 3.4) | Người chơi cùng đánh một con đều nhận EXP theo công sức |

### 1.4. Không làm ở Giai đoạn 1

Kỹ năng riêng của class, MP tiêu hao, tinh anh và thủ lĩnh, kỹ năng của quái, trượt và chí mạng, Xu, đồ rơi, bình thuốc, NPC, hồi sinh tại chỗ (cần Lượng), PK, tổ đội, nhiều map, cổng, đổi khu, tẩy điểm tiềm năng, click để đi, tự tìm đường, auto đánh.

Đổi map và đổi khu thuộc Giai đoạn 4.

## 2. Dữ liệu

### 2.1. Map `kumotori_1`

`maps/kumotori_1.json` thêm ba trường. Cả client lẫn server đọc file này; client bỏ qua `camps`.

```json
"safe_zones":  [{ "id": "camp", "x": 1, "y": 1, "w": 10, "h": 10 }],
"rest_points": [{ "id": "bonfire", "x": 3.5, "y": 3.5, "radius": 1.5 }],
"camps": [
  { "id": "c1", "monster": "wanderer",   "x": 20, "y": 10, "radius": 5, "count": 12, "min_level": 1, "max_level": 2 },
  { "id": "c2", "monster": "snow_ghoul", "x": 15, "y": 35, "radius": 5, "count": 12, "min_level": 2, "max_level": 3 },
  { "id": "c3", "monster": "wanderer",   "x": 45, "y": 10, "radius": 5, "count": 12, "min_level": 3, "max_level": 4 },
  { "id": "c4", "monster": "snow_ghoul", "x": 52, "y": 40, "radius": 5, "count": 12, "min_level": 4, "max_level": 5 }
]
```

- Khu an toàn là hình chữ nhật theo ô: ô (i, j) thuộc khu khi `x ≤ i < x + w` và `y ≤ j < y + h`. Một vị trí thuộc khu khi ô chứa nó thuộc khu.
- Khu `camp` chứa spawn `default` (5.5, 5.5). Đây là điểm vào game của nhân vật mới và là điểm hồi sinh.
- Bãi xa khu an toàn hơn thì cấp cao hơn (mật độ map nhỏ theo game-design mục 5.11).
- Server kiểm tra khi nạp map; sai thì dừng và báo lỗi:
  - khu an toàn nằm trong map, `w` và `h` dương;
  - điểm nghỉ có bán kính dương và cả hình tròn của nó nằm hẳn trong các khu an toàn (mọi ô mà hình tròn phủ tới đều thuộc khu an toàn), để quái không với tới chỗ người chơi nghỉ;
  - spawn `default` nằm trong một khu an toàn nếu map có khu an toàn;
  - mỗi bãi có `min_level ≤ max_level`, `count > 0`, có ít nhất `count` ô đi được trong bán kính và không ô nào trong bán kính thuộc khu an toàn;
  - `id` không trùng trong từng danh sách.

### 2.2. `data/monsters.json`

Chỉ server đọc. Thư mục cấu hình bằng biến `DATA_DIR`, mặc định `./data`.

```json
{
  "kinds": [
    { "id": "wanderer",   "name": "Quỷ lang thang",      "attack_range": 1.5, "attack_sec": 1.5,
      "speed": 0.8, "aggro_range": 4, "leash_range": 10, "respawn_sec": 8 },
    { "id": "snow_ghoul", "name": "Quỷ đói trong tuyết", "attack_range": 1.5, "attack_sec": 1.5,
      "speed": 0.8, "aggro_range": 4, "leash_range": 10, "respawn_sec": 8 }
  ],
  "levels": [
    { "level": 1, "hp": 56,  "attack": 30, "defense": 8 },
    { "level": 2, "hp": 104, "attack": 39, "defense": 16 },
    { "level": 3, "hp": 160, "attack": 45, "defense": 24 },
    { "level": 4, "hp": 212, "attack": 52, "defense": 32 },
    { "level": 5, "hp": 260, "attack": 60, "defense": 40 }
  ]
}
```

- `speed` là hệ số tốc độ, nhân với `MOVE_SPEED_TILES` như người chơi: 0,8 × 4 = 3,2 ô/giây. Game-design không ghi tốc độ quái; chậm hơn người chơi để người chơi chạy thoát được.
- `defense` dùng cho cả thủ vật lý và thủ phép.
- Server kiểm tra khi khởi động: `id` loại quái không trùng; mọi bãi của mọi map trỏ tới loại có thật; bảng `levels` có mọi cấp từ `min_level` đến `max_level` của mọi bãi; các số dương.

### 2.3. Bảng chỉ số quái

Bảng `levels` được sinh bằng script `docs/balance/monster_stats.py` (ghi JSON và in bảng), đúng ý "dữ liệu quái tính ngoại tuyến thành số cố định" (game-design mục 5.9). Chỉnh cân bằng thì sửa script rồi chạy lại.

Nhân vật tham chiếu cấp L: Viêm không trang bị (Giai đoạn 1 chưa có đồ), có 10 × (L − 1) điểm, floor(2/3) số điểm vào STR, còn lại vào VIT.

- Thủ quái = 8 × L.
- HP quái = 4 × sát thương chém mặc định của nhân vật tham chiếu lên quái cùng cấp (công thức mục 5.9 cho cấp dưới 10).
- Công quái = ceil(4% HP tham chiếu / (1 − giảm do thủ của nhân vật tham chiếu)), để một đòn 100% công lấy khoảng 4% HP.

| Cấp | HP | Công | Thủ | Chém mặc định cùng cấp gây | Đòn quái lấy |
| --- | --- | --- | --- | --- | --- |
| 1 | 56 | 30 | 8 | 14 | 20 / 500 (4,0%) |
| 2 | 104 | 39 | 16 | 26 | 26 / 640 (4,1%) |
| 3 | 160 | 45 | 24 | 40 | 30 / 760 (3,9%) |
| 4 | 212 | 52 | 32 | 53 | 35 / 880 (4,0%) |
| 5 | 260 | 60 | 40 | 65 | 41 / 1.020 (4,0%) |

Kiểm tra chéo: cùng công thức ở cấp 10 cho công 95, khớp bảng mục 5.9. HP cấp 10 trở lên tính cả Bất Tri Hỏa nên không so ở đây.

## 3. Luật chiến đấu

### 3.1. Chỉ số nhân vật

Theo game-design mục 2.2. L là cấp; STR, AGI, VIT, INT là điểm tiềm năng đã cộng.

| Chỉ số | Công thức |
| --- | --- |
| HP tối đa | 500 + a × (L − 1) + b × VIT; class vật lý (Viêm, Minh, Nham) a = 60, b = 20; class phép (Thủy, Phong) a = 50, b = 16 |
| MP tối đa | 100 + 20 × (L − 1) + 5 × INT |
| Công vật lý | 20 + 5 × (L − 1) + 2 × STR |
| Công phép | 20 + 5 × (L − 1) + 2 × INT |
| Thủ vật lý, thủ phép | 10 + 2 × (L − 1) + 2 × VIT |
| Chính xác | 100 + 5 × (L − 1) + 3 × AGI |
| Né tránh | 50 + 5 × (L − 1) + 2 × AGI |
| Tỷ lệ chí mạng | 500 + 2 × AGI, đơn vị phần vạn (5% + 0,02 điểm phần trăm mỗi AGI) |

Chính xác, né tránh và tỷ lệ chí mạng chỉ hiển thị ở Giai đoạn 1. MP chưa được dùng.

### 3.2. Sát thương

- **Công thức chung:** sát thương = max(1, floor(cơ sở × (1 − thủ / (thủ + 20 × cấp người đánh)))).
- **Người đánh quái:** chém mặc định, đòn vật lý, cơ sở = max(công vật lý, công phép), dùng thủ của quái.
- **Quái đánh người:** đòn vật lý, cơ sở = công quái, dùng thủ vật lý của người.
- Luôn trúng, không chí mạng.

### 3.3. Chém mặc định

- Tầm 1,5 ô, hồi chiêu 0,8 giây, không tốn MP.
- Khoảng cách đo từ vị trí tới vị trí (tâm tới tâm), đơn vị ô.

### 3.4. EXP và lên cấp

- EXP gốc của quái là 10 × cấp quái (mục 5.12), chia theo sát thương (game-design mục 5.8):
  1. Mỗi đòn trúng quái cho EXP ngay: EXP = sát thương × EXP gốc / HP tối đa của quái. Ví dụ quái 1.000 HP, 100 EXP: đòn 100 sát thương cho 10 EXP.
  2. Sát thương thừa không tính: sát thương tính EXP tối đa bằng HP còn lại của quái trước đòn, nên một con không trả quá EXP gốc. `Hit.damage` vẫn gửi đủ sát thương.
  3. Không mất phần lẻ: mỗi con quái ghi tổng sát thương đã tính và EXP đã trả cho từng người chơi. Mỗi đòn trả floor(EXP gốc × tổng sát thương / HP tối đa) − EXP đã trả. Một người tự hạ quái cấp 1 (56 HP, 10 EXP, mỗi nhát 14) nhận đúng 10 EXP, lần lượt 2, 3, 2, 3.
  4. Quái về chỗ và hồi đầy HP (kéo quá xa rồi về, hoặc kẹt rồi được đặt lại chỗ cũ): EXP đã trả giữ nguyên, bảng sát thương và EXP đã trả của con quái về 0.
  5. Đồ rơi (Giai đoạn 3) vẫn thuộc người kết liễu (game-design mục 5.8). Thưởng 5% tổ đội định lại ở Giai đoạn 5; Giai đoạn 1 chưa có tổ đội.
- EXP cần để lên từ cấp L: 10 × L × (10 + 2 × (L − 1)^1,5) × hệ số cấp, làm tròn đến 10. Hệ số cấp: 4 × L / 3 khi L ≤ 9; 10 + 0,5 × (L − 10) khi L ≥ 10 (mục 2.2).
- EXP lưu theo tổng tích lũy trong cấp hiện tại (`exp` là EXP đã có của cấp hiện tại). Đủ EXP thì lên cấp, trừ phần đã dùng, phần dư giữ lại; đủ thì lên nhiều cấp liền.
- Cấp tối đa 90. Ở cấp 90 không nhận thêm EXP.
- Mỗi lần lên cấp: +10 điểm tiềm năng, hồi đầy HP/MP.

### 3.5. Cộng điểm tiềm năng

- `AllocatePointsReq { strength, agility, vitality, intellect }`: mỗi số không âm, tổng lớn hơn 0 và không vượt `free_points`. Zone kiểm tra theo thứ tự: đang chết (`DEAD`); có số âm (`BAD_REQUEST`); vượt `free_points` (`NOT_ENOUGH_POINTS`); cả bốn số bằng 0 (`BAD_REQUEST`). Gateway chặn trước: tổng lớn hơn 1.000.000 trả `BAD_REQUEST`, để phép cộng trong zone không tràn số.
- Cộng được mọi lúc, kể cả đang đánh. Không cộng được khi đang chết.
- HP và MP hiện tại tăng đúng bằng phần HP và MP tối đa vừa tăng.
- Không có tẩy điểm.

### 3.6. Chết và hồi sinh

- HP về 0 thì nhân vật chết:
  - server bỏ qua `MoveInput`, từ chối lệnh đánh và cộng điểm;
  - mọi quái đang nhắm nhân vật này bỏ mục tiêu ngay lúc chết (`killPlayer`) và chọn lại theo mục 4.2.
- `RespawnReq` đưa nhân vật về spawn `default` của map đang đứng, đầy HP/MP. Sau 60 giây không gửi thì server tự làm.
- Chết do quái không mất EXP (mục 5.7).
- Thoát game khi đang chết: lần vào sau nhân vật ở spawn `default`, đầy HP/MP.

### 3.7. Khu an toàn và điểm nghỉ

- Trong khu an toàn không đánh được và không bị quái đánh. Quái không bước vào ô thuộc khu an toàn.
- Còn sống, đứng trong bán kính điểm nghỉ và không di chuyển đủ 5 giây liên tục thì hồi đầy HP/MP. Có hướng di chuyển khác 0 thì bộ đếm về 0. Đã đầy HP/MP thì vẫn đếm nhưng không gửi gì.

## 4. Mục tiêu và hành vi quái

### 4.1. Người chơi chọn mục tiêu (client)

Theo game-design mục 2.5. Client chọn và đổi mục tiêu; server chỉ kiểm tra lệnh đánh.

- Mục tiêu hợp lệ: quái còn sống, không đang về chỗ. Người chơi khác chưa đánh được vì chưa có PK.
- Chọn: bấm chuột hoặc chạm vào quái.
- Đổi: `Tab` hoặc nút "Đổi mục tiêu" chuyển sang quái hợp lệ gần kế tiếp trong 6 ô, xoay vòng theo khoảng cách.
- Bấm đánh khi chưa có mục tiêu: chọn quái hợp lệ gần nhất trong 6 ô; không có thì hiện "Không có mục tiêu".
- Bỏ khóa khi mục tiêu cách quá 8 ô, ra khỏi tầm nhìn, hoặc mình vào khu an toàn.
- Mục tiêu chết: chuyển sang quái hợp lệ gần nhất trong 6 ô, không có thì bỏ. Đang giữ nút đánh thì đánh tiếp con mới.

### 4.2. Quái

Mỗi quái có điểm sinh (vị trí lúc sinh ra), danh sách người đã đánh nó, và một trong bốn trạng thái:

| Trạng thái | Hành vi | Chuyển sang |
| --- | --- | --- |
| Rảnh | Mỗi 4–8 giây (ngẫu nhiên) chọn một ô đi được ngoài khu an toàn, cách điểm sinh tối đa 2 ô, đi tới tâm ô với nửa tốc độ | Đuổi: có người chơi hợp lệ trong `aggro_range`, hoặc bị đánh |
| Đuổi | Đi thẳng tới mục tiêu, dừng khi cách min(1,3; tầm đánh − 2·eps) ô, với eps là sai số tới nơi (1e-3 ô), để sai số số thực không để quái đứng ngay ngoài tầm | Đánh: trong `attack_range`. Về: cách điểm sinh quá `leash_range` |
| Đánh | Đánh mỗi `attack_sec`; mục tiêu ra ngoài `attack_range` thì đuổi tiếp | Về: cách điểm sinh quá `leash_range` |
| Về | Đi thẳng về điểm sinh với tốc độ đầy đủ; không nhận đòn, không nhận mục tiêu. Kẹt liền 2 giây thì dịch chuyển thẳng về điểm sinh, `EntityMove` của tick đó có `snap = true` | Rảnh: tới nơi, hồi đầy HP, xóa danh sách người đã đánh |

- Người chơi hợp lệ cho quái: còn sống, không đứng trong khu an toàn, cùng zone.
- Nhận mục tiêu khi rảnh: người hợp lệ gần nhất trong `aggro_range`. Bị đánh khi rảnh: người vừa đánh.
- Đã có mục tiêu thì không đổi chỉ vì có người khác đi ngang hoặc đánh thêm.
- Mục tiêu mất (chết, rời zone, vào khu an toàn) thì chọn theo thứ tự: người hợp lệ gần nhất trong danh sách đã đánh nó; nếu không có, người hợp lệ gần nhất trong `aggro_range`; nếu vẫn không có, chuyển sang Về.
- Đòn đầu tiên khi vào tầm: đánh ngay nếu đã qua `attack_sec` kể từ đòn trước.
- Quái dùng chung hàm di chuyển và va chạm của người chơi (hộp 0,6 ô, trượt dọc tường), thêm luật ô thuộc khu an toàn tính là bị chặn với quái. Quái bị kẹt thì vẫn giữ trạng thái; khoảng cách tới điểm sinh vượt `leash_range` vẫn đưa nó về.
- Quái và người chơi không chặn nhau.
- Quái chết: `WorldTick` của tick đó có `Death`; xác nằm lại 1 giây rồi bị xóa (`EntityLeave`). Sau `respawn_sec` tính từ lúc chết, bãi sinh một quái mới: id mới, cấp bốc lại trong khoảng của bãi, vị trí là ô đi được ngẫu nhiên trong bán kính bãi, đầy HP.
- Khi zone mở, mọi bãi sinh đủ `count` quái.

## 5. Giao thức

Proto giữ luật đặt tên an toàn cho GDScript. Thuộc tính tiềm năng đặt tên `strength`, `agility`, `vitality`, `intellect`; không dùng `str` hay `int`.

### 5.1. Client gửi (có reply)

| Tin | Reply | Lỗi |
| --- | --- | --- |
| `AttackReq { uint64 target_id }` | `AttackResp {}` | `INVALID_TARGET`, `OUT_OF_RANGE`, `ON_COOLDOWN`, `IN_SAFE_ZONE`, `DEAD`, `NOT_IN_WORLD` |
| `AllocatePointsReq { uint32 strength, agility, vitality, intellect }` | `AllocatePointsResp { SelfStats stats }` | `BAD_REQUEST`, `NOT_ENOUGH_POINTS`, `DEAD`, `NOT_IN_WORLD` |
| `RespawnReq {}` | `RespawnResp { float x, y }` | `NOT_DEAD`, `NOT_IN_WORLD` |

- Mã lỗi mới thêm vào `ErrorCode` từ 16: `INVALID_TARGET`, `OUT_OF_RANGE`, `ON_COOLDOWN`, `IN_SAFE_ZONE`, `DEAD`, `NOT_DEAD`, `NOT_ENOUGH_POINTS`.
- Zone xử lý ba lệnh này ở tick kế tiếp; reply được gửi trong tick đó, trước `WorldTick`.
- Gateway giới hạn `AttackReq` 5 lần mỗi giây, burst 5, mỗi kết nối; vượt thì trả `RATE_LIMITED` (không vào zone).

**Kiểm tra `AttackReq`**, theo thứ tự:

1. Người đánh còn sống (`DEAD`).
2. Người đánh không ở khu an toàn (`IN_SAFE_ZONE`).
3. Mục tiêu tồn tại trong zone, là quái, còn sống và không đang về chỗ (`INVALID_TARGET`).
4. Khoảng cách ≤ 1,5 + `ATTACK_RANGE_SLACK` (0,5 ô) và đoạn thẳng nối hai vị trí không đi qua ô bị chặn (`OUT_OF_RANGE`).
5. Đã qua 0,8 giây kể từ đòn trước được chấp nhận (`ON_COOLDOWN`).

Dung sai 0,5 ô vì client thấy quái chậm 2 tick: quái đang đuổi đi được khoảng 0,64 ô trong 0,2 giây. Client chỉ gửi khi đã vào trong 1,3 ô.

### 5.2. Server gửi

- `EntityKind` thêm `ENTITY_KIND_MONSTER = 2`.
- `EntityEnter` thêm `int32 hp`, `int32 max_hp`, `bool dead`. Với quái, `name` là tên loại quái, `char_class` để trống. Không gửi `EntityEnter` cho xác quái.
- `WorldTick` thêm các danh sách, chỉ chứa thực thể trong tầm nhìn của người nhận:

| Trường | Nội dung | Khi nào |
| --- | --- | --- |
| `repeated Hit hits` | `attacker_id`, `target_id`, `int32 damage`, `int32 hp_after` | Mỗi đòn trúng |
| `repeated Death deaths` | `id` | HP về 0 |
| `repeated HpChange hp_changes` | `id`, `int32 hp`, `int32 max_hp` | HP đổi không do đòn: điểm nghỉ, hồi sinh, lên cấp, cộng VIT, quái về chỗ. Mang HP ở cuối tick |
| `repeated LevelUp level_ups` | `id`, `int32 level` | Người chơi lên cấp |

- Tin di chuyển trong `WorldTick` là `EntityMove` (không phải `Move`), thêm `bool snap`: client đặt thẳng vị trí, không nội suy. `snap` áp dụng cho mọi thực thể: bật khi người chơi hồi sinh và khi quái bị dịch chuyển về điểm sinh.
- Thứ tự áp dụng ở client: các `Hit` trước, rồi `hp_changes`; `hp_changes` mang HP cuối tick nên thắng `Hit.hp_after` của cùng tick.
- Client bỏ qua id trong `Hit` hay `Death` mà mình chưa có `EntityEnter` (ngoài tầm nhìn), trừ nhân vật của chính mình.
- `SelfStats`: `level`, `int64 exp`, `int64 exp_next`, `free_points`, `strength`, `agility`, `vitality`, `intellect`, `hp`, `max_hp`, `mp`, `max_mp`, `phys_attack`, `magic_attack`, `phys_defense`, `magic_defense`, `accuracy`, `evasion`, `crit_rate_bp`.
  - Là trường `SelfStats stats = 9` của `EnterWorldResp`.
  - Là push riêng (seq 0) sau `WorldTick` của tick mà EXP, cấp, điểm, HP tối đa hoặc MP đổi không qua `Hit`: mỗi đòn trúng quái làm EXP đổi, lên cấp, hồi sinh, hồi ở điểm nghỉ.
  - Nằm trong `AllocatePointsResp`.
  - HP của mình khi bị đánh đã có trong `Hit.hp_after`, không gửi `SelfStats` cho mỗi đòn.

### 5.3. Phiên bản và fixture

- `PROTOCOL_VERSION` lên 2 ở cả server (`gateway.ProtocolVersion`) và client. Client cũ bị chặn bằng `VERSION_MISMATCH`.
- `testdata/client` thêm fixture: `attack_req`, `attack_resp`, `allocate_points_req`, `respawn_resp`, `self_stats`, `entity_enter_monster`, `world_tick_combat` (có đủ bốn danh sách mới và một `EntityMove` có `snap`).

## 6. Server

### 6.1. Package

| Package | Thay đổi |
| --- | --- |
| `internal/mapdata` | Đọc `safe_zones`, `rest_points`, `camps`; kiểm tra mục 2.1; `InSafeZone(x, y)`, `RestPointAt(x, y)` |
| `internal/gamedata` (mới) | Nạp `data/monsters.json`, kiểm tra mục 2.2 với các map đã nạp |
| `internal/combat` (mới, thuần) | `Compute(class, level, points) Stats`, `Damage(base, def, attackerLevel)`, `ExpToNext(level)`, `GainExp(level, exp, gain) (level, exp, levelsGained)`, hằng số chém mặc định. Không phụ thuộc zone hay `pb` |
| `internal/zone` | `monster.go` (trạng thái, hành vi), `spawn.go` (bãi, hẹn giờ sinh lại), `combat.go` (đánh, chết, EXP, cộng điểm, hồi sinh, điểm nghỉ). `zone.go` giữ vòng tick |
| `internal/store`, `internal/saver`, `internal/world` | Đọc và ghi cột mới; `SaveCharacter` mang thêm điểm tiềm năng, `free_points`, `hp`, `mp` |
| `internal/gateway` | Chuyển ba lệnh mới vào zone kèm kênh reply; rate limit `AttackReq` |
| `internal/config`, `internal/app` | Biến `DATA_DIR`; nạp `gamedata` sau map |

Ràng buộc ở `internal/world`: kích thước ô lưới tầm nhìn (AOI) phải không nhỏ hơn `aggro_range` lớn nhất của các loại quái, vì quái chỉ tìm người trong các ô lưới quanh nó; vi phạm thì `world.New` báo lỗi khi khởi động.
| `cmd/botclient` | Chế độ `-fight N`: đi tới quái gần nhất, đánh cho tới khi hạ N con, in EXP và cấp |

### 6.2. Tick

1. Lấy hết lệnh trong hộp thư: `Join`, `Leave`, `MoveInput`, `Attack`, `Allocate`, `Respawn`, `SnapshotRequest`.
2. Xử lý lệnh đánh của người chơi, trước khi ai di chuyển: vị trí lúc này gần với cái client vừa thấy nhất.
3. Người chơi di chuyển (bỏ qua người chết); cập nhật bộ đếm điểm nghỉ.
4. Quái chạy hành vi (mục 4.2).
5. Hẹn giờ: xóa xác quái, sinh lại quái, tự hồi sinh sau 60 giây, hồi đầy ở điểm nghỉ.
6. Cập nhật tầm nhìn. Quái nằm trong cùng lưới tầm nhìn với người chơi.
7. Gửi mỗi người: reply của lệnh trong tick, `EntityLeave`, `EntityEnter`, `WorldTick`, rồi `SelfStats` nếu có.

- Quái dùng chung dãy id thực thể của zone.
- Zone nhận nguồn số ngẫu nhiên tiêm vào (như đồng hồ giả) để test lặp lại được.
- Quái tìm người chỉ trong các ô lưới tầm nhìn quanh nó.

### 6.3. Lưu dữ liệu

- Migration `00002` thêm vào `characters`: `str`, `agi`, `vit`, `int` (`int`, `NOT NULL DEFAULT 0`, `CHECK >= 0`), `free_points` (như trên), `hp`, `mp` (`int`, cho phép NULL; NULL nghĩa là đầy). Nhân vật cũ vì vậy vẫn vào được.
- Khi vào game: HP/MP lớn hơn tối đa thì kẹp về tối đa; `hp = 0` thì đặt ở spawn `default`, đầy HP/MP (thoát khi đang chết).
- Lưu ngay: lên cấp, cộng điểm.
- Lưu định kỳ (`SAVE_INTERVAL`) và khi rời zone: EXP, HP/MP, vị trí.
- Saver đã gộp yêu cầu cùng nhân vật, nên lên nhiều cấp liền vẫn ít lần ghi. Server sập thì mất tối đa `SAVE_INTERVAL` EXP, không mất cấp hay điểm đã cộng.

## 7. Client

### 7.1. Logic thuần (`core/`)

| File | Việc |
| --- | --- |
| `targeting.gd` (mới) | Mục 4.1: chọn gần nhất trong 6 ô, xoay vòng, bỏ khóa quá 8 ô hoặc mất khỏi tầm nhìn, tự chuyển khi mục tiêu chết |
| `attack_loop.gd` (mới) | Vào: vị trí mình, vị trí mục tiêu, nút đánh đang giữ, thời gian. Ra: hướng chạy tới (khi cách hơn 1,3 ô) hoặc "gửi `AttackReq`". Mỗi lúc một lệnh chờ reply; hồi chiêu 0,8 giây tính từ lúc gửi. Đi bằng phím hoặc joystick thì hủy tự chạy |
| `rest_timer.gd` (mới) | Tiến độ 0–1 khi đứng yên trong bán kính điểm nghỉ; chỉ để vẽ |
| `map_grid.gd` | Đọc khu an toàn và điểm nghỉ; `in_safe_zone(pos)` |
| `predictor.gd` | `reset(pos)` xóa lịch sử khi hồi sinh; khi chết không đi |
| `interpolator.gd` | Mẫu có `snap` thì nhảy thẳng, không nội suy từ mẫu trước |

Xử lý lỗi `AttackReq`: `OUT_OF_RANGE` thì chạy lại gần; `ON_COOLDOWN` thì chờ; `INVALID_TARGET` thì bỏ mục tiêu; `IN_SAFE_ZONE` thì hiện "Không thể đánh trong khu an toàn"; `DEAD` thì bỏ qua.

### 7.2. Giao diện (`ui/`)

- `map_view`: tô nhạt khu an toàn, vẽ điểm nghỉ thành vòng lửa trại.
- `entity_view`: quái vẽ khác người chơi; nhãn tên và cấp; thanh HP trên đầu; vòng chọn dưới chân mục tiêu; xác quái mờ dần 1 giây; người chết vẽ xám.
- `damage_text.gd` (mới): số bay lên và mờ trong 0,8 giây; trắng cho đòn lên quái, đỏ cho đòn mình nhận.
- Vệt chém ngắn từ người đánh hướng về mục tiêu cho mỗi `Hit`.
- `hud`: HP, MP, cấp ở góc trái trên; tên, cấp, HP mục tiêu ở giữa trên; thanh EXP ở đáy; nút Đánh (giữ để đánh lặp), Đổi mục tiêu, Chỉ số (có chấm khi còn điểm) ở góc phải dưới; chữ "Lên cấp N!" ở giữa khi mình lên cấp, chữ nhỏ trên đầu khi người khác lên cấp; vòng tiến độ điểm nghỉ quanh nhân vật.
- `stats_panel.gd` (mới): bốn thuộc tính với nút + và − để cộng thử, số điểm còn lại, Xác nhận và Hủy; danh sách chỉ số chi tiết; dòng "Chính xác, né tránh và chí mạng chưa có tác dụng ở bản này".
- `death_panel.gd` (mới): "Bạn đã gục ngã", nút "Về khu an toàn", đếm ngược 60 giây.

### 7.3. Điều khiển

- Giữ `Space` hoặc nút Đánh: đánh lặp.
- `Tab` hoặc nút Đổi mục tiêu: đổi mục tiêu.
- `C` hoặc nút Chỉ số: bảng chỉ số.
- Bấm chuột hoặc chạm vào quái: chọn mục tiêu. Bấm vào nền không làm gì. Kéo joystick không chọn mục tiêu.

### 7.4. Khác

- `PROTOCOL_VERSION` lên 2.
- Tên payload mới phải được thêm vào `core/wire.gd` `PAYLOADS` (godobuf không có phản chiếu oneof). Enum mất tiền tố khi sinh mã, ví dụ `Proto.ErrorCode.INVALID_TARGET`.
- Gen lại `proto/protocol.gd` và đồng bộ map bằng `tools/sync_server.sh` sau khi cập nhật submodule `server/`.
- `screens/world.gd` chỉ nối: nhận push, chuyển cho `core/` và `ui/`. Logic mới không viết thẳng vào đó.

## 8. Kiểm thử

Trạng thái: phần server đã có test và qua `go test -race ./...`; `TestKillMonsterOverWebSocket` (e2e) là bài botclient hạ quái. Các mục client do repo client theo dõi.

Mỗi package hoặc thư mục có 1–2 file test, chia đoạn bằng dòng `// ______________________TEST X_____________________` (Go) và `# ______________________TEST X_____________________` (GDScript).

**Server**

- `combat`:
  - Viêm cấp 10 với 60 STR / 30 VIT: công vật lý 185, HP 1.640, thủ vật lý 88, MP 280; chém mặc định lên Viêm cùng cấu hình gây 128.
  - Thủy cấp 10 với 60 INT / 30 VIT: công phép 185, HP 1.430, MP 580.
  - `ExpToNext` khớp mọi dòng `docs/balance/exp-table.csv`.
  - `GainExp`: lên nhiều cấp liền, giữ EXP dư, dừng ở cấp 90.
  - Sát thương tối thiểu 1.
- `mapdata`, `gamedata`: mỗi lỗi dữ liệu ở mục 2.1 và 2.2 bị bắt; `data/monsters.json` khớp đầu ra của `monster_stats.py`.
- `zone` (đồng hồ giả, số ngẫu nhiên cố định):
  - mở zone sinh đủ quái mỗi bãi, đúng khoảng cấp, đúng ô hợp lệ;
  - quái rảnh đi lang thang trong 2 ô quanh điểm sinh;
  - quái nhận người trong 4 ô, không nhận người trong khu an toàn;
  - kéo quá 10 ô thì quái về chỗ, hồi đầy HP, không nhận đòn trên đường về;
  - mất mục tiêu thì chuyển sang người đã đánh nó, rồi người gần trong 4 ô;
  - quái không bước vào khu an toàn;
  - mỗi lỗi `AttackReq` trả đúng mã; đòn hợp lệ trừ đúng HP;
  - mỗi đòn trúng trả EXP theo sát thương (2, 3, 2, 3 khi tự hạ quái cấp 1), sát thương thừa không tính, hai người cùng đánh đều nhận phần mình, quái về chỗ thì bảng sát thương về 0;
  - EXP từ một đòn đủ lên cấp thì lên cấp, +10 điểm, đầy HP/MP và có yêu cầu lưu ngay;
  - cộng điểm đúng, sai thì trả lỗi;
  - người chết không đi được, được tự hồi sinh sau 60 giây ở spawn `default` với `snap`;
  - đứng ở điểm nghỉ 5 giây thì hồi đầy, di chuyển thì đếm lại;
  - quái sinh lại sau 8 giây với id mới; xác bị xóa sau 1 giây.
- `gateway`: fixture mới (`TestClientFixtures`); rate limit `AttackReq` ở `handlers.go` (đã làm, chưa có test riêng cho giới hạn này).
- e2e: botclient hạ một quái qua WebSocket thật và nhận `SelfStats` với EXP tăng (đã làm: `e2e/e2e_test.go`).

**Client**

- `test_core.gd` thêm đoạn TARGETING, ATTACK LOOP, REST, MAP (khu an toàn, điểm nghỉ), PREDICTION (`reset`), INTERPOLATION (`snap`).
- `test_net.gd`: giải mã mọi fixture mới.
- Smoke với server thật: vào game, đánh quái gần nhất cho tới khi hạ một con, kiểm `SelfStats` có EXP tăng.

## 9. Tiêu chí hoàn thành

Đã xong: test server (`go test -race ./...`) qua. Còn chờ người dùng kiểm: bài chơi thử hai cửa sổ bên dưới.

- Mọi test server (`make test-db`) và client (`tests/run.sh`) qua; smoke in `SMOKE OK`.
- Hai client mở bằng `tools/run2.sh`, đăng nhập `test1` và `test2`:
  - thấy quái đi lang thang trong bãi;
  - thấy đòn đánh và số sát thương của nhau;
  - hạ quái, lên cấp, cộng điểm;
  - chết và hồi sinh ở khu an toàn;
  - hồi đầy ở điểm nghỉ.
