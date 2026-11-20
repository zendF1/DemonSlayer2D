# Ba mốc của một đòn đánh: chuẩn bị, bay, trúng

Ngày: 2026-11-20. Trạng thái: Đã duyệt thiết kế, chờ triển khai.

Spec này sửa cách server và client (`../DemonSlayer2DGame`) xử lý thời gian của mọi đòn đánh. Nó xây trên [spec Giai đoạn 2](2026-11-20-giai-doan-2-ky-nang-design.md), [spec quái thường theo vùng](2026-11-20-quai-thuong-vung-va-chuong-design.md) và [spec Giai đoạn 1](2026-11-19-giai-doan-1-chien-dau-pve-design.md); phần nào không nhắc tới thì giữ nguyên như các spec đó.

## 1. Vấn đề và quyết định

**Vấn đề:** server gây sát thương ngay tick nhận lệnh. Chiêu đánh xa vừa bấm đã trừ máu và hiện số, hiệu ứng không kịp chạy. Chưởng của quái cũng vậy: sát thương tới trước, quả cầu 0,3 giây vẽ sau.

**Mục tiêu:** mỗi đòn có ba mốc, và server trừ máu đúng lúc đòn trúng để biết ai kết liễu (dùng cho nhặt đồ ở Giai đoạn 3):

1. **Chuẩn bị** — bấm chiêu: hoạt ảnh ra chiêu.
2. **Bay** — đạn hoặc sóng bay tới mục tiêu (nếu đòn có đạn).
3. **Trúng** — hiệu ứng trúng và số sát thương.

| Vấn đề | Quyết định | Lý do |
| --- | --- | --- |
| Cách đồng bộ | Server chốt đòn lúc bấm, trừ máu và gửi `Hit` đúng tick trúng (phương án A) | Thanh máu, số sát thương và người kết liễu luôn khớp server; `Cast` và `Hit` cùng đi qua mạng nên trễ như nhau |
| Phạm vi | Mọi đòn: 25 chiêu, chém mặc định, đòn cận chiến và chưởng của quái | Một cơ chế chung, không đòn nào trúng trước khi hiệu ứng tới |
| Giữ chân khi chuẩn bị | Không. Bấm là chiêu chắc chắn ra; nhân vật vẫn đi lại được | Giữ game-design mục 2.4: hoạt ảnh không khóa lệnh tiếp theo |
| Né đòn | Không. Đòn đã ra thì trúng, trừ khi mục tiêu đã chết hoặc rời zone | Mốc chuẩn bị và bay ngắn (0,15–0,65 giây); né cần luật riêng |

**Không làm:** né bằng cách chạy khỏi tầm, thời gian niệm phép, hủy chiêu, trượt và chí mạng, hình ảnh chiêu thật, rơi đồ.

## 2. Mô hình thời gian

### 2.1. Luật chung

- **Mốc 1, tick t0 server nhận lệnh:** kiểm tra như hiện tại, trừ MP, bắt đầu hồi chiêu, chốt danh sách mục tiêu theo vị trí lúc đó, tính sát thương từng mục tiêu bằng chỉ số người ra đòn lúc đó, tính thời điểm trúng từng mục tiêu, phát `Cast`.
- **Thời điểm trúng** của một mục tiêu = t0 + `z.ticks(chuẩn bị + bay)`, với `z.ticks` làm tròn lên và ít nhất 1 tick. Vì chuẩn bị luôn > 0, không đòn nào trúng trong tick bấm.
- **Thời gian bay** = khoảng cách lúc bấm / tốc độ đạn; tốc độ 0 nghĩa là không có đạn (bay = 0).
- **Mốc 3, tick trúng:** mục tiêu đã chết, đã rời zone, hoặc (với quái) đã chết rồi hồi sinh thì đòn bị bỏ, không có `Hit`. Nếu không, trừ máu và phát `Hit`.
- **Người ra đòn** chết hoặc rời zone trước khi đòn trúng: đòn vẫn trúng; EXP chỉ trả khi người chơi đó còn trong zone. Quái chết trước khi chưởng của nó tới: chưởng vẫn trúng.
- **Hồi chiêu và MP** tính từ lúc bấm như hiện tại.

### 2.2. Thời gian bay theo kiểu đòn

| Kiểu | Bay tới mục tiêu |
| --- | --- |
| Chém mặc định, quái cận chiến, `circle_self` | 0 (cả vòng trúng cùng lúc khi hết chuẩn bị) |
| `single` | khoảng cách người ra đòn → mục tiêu / tốc độ |
| `cone`, `line` | khoảng cách người ra đòn → từng quái / tốc độ (sóng lan ra, quái xa trúng muộn hơn) |
| `circle_target` | khoảng cách người ra đòn → mục tiêu chính / tốc độ; mọi quái trong vòng trúng cùng lúc |
| Lao tới (`dash`) | quãng lao (vị trí cũ → điểm đáp) / tốc độ lao; server vẫn dời nhân vật ngay lúc bấm |
| Chưởng của quái | khoảng cách quái → người chơi / `shot_speed` |

### 2.3. Bộ số ban đầu

| Đòn | Chuẩn bị (`windup_ms`) | Tốc độ (ô/giây) |
| --- | --- | --- |
| Chém mặc định | 150 | 0 |
| Chiêu `single` không lao, trừ ô 4 (Thủy I, Nham I, Phong II) | 250 | 20 |
| Lao tới (Viêm I, Minh I) | 100 | 25 (tốc độ lao) |
| `cone`, `line` | 250 | 20 |
| `circle_target` | 300 | 20 |
| `circle_self` | 300 | 0 |
| Chiêu `single` ô 4 (cấp 50: Viêm, Thủy, Minh, Nham) | 400 | 20 |
| Quái cận chiến | 200 | 0 |
| Chưởng của quái | 0 (AI đã ngắm 0,5 giây trước phát đầu) | 15 |

Ví dụ: chiêu đơn mục tiêu cách 5 ô trúng sau 0,25 + 5/20 = 0,5 giây.

Chưởng của quái có chuẩn bị 0 nhưng bay luôn > 0 khi người chơi không đứng trùng chỗ; nếu tổng là 0 thì `z.ticks` vẫn làm tròn lên 1 tick.

### 2.4. Dữ liệu

- `skills.json` (sinh bằng `skill_stats.py`): mỗi chiêu chủ động thêm `windup_ms` (> 0) và `projectile_speed` (≥ 0; với `dash` là tốc độ lao, phải > 0; với `circle_self` phải là 0). Bị động không có hai trường này.
- `monsters.json` (sinh bằng `monster_stats.py`): mỗi loại quái thêm `melee_windup_ms` (> 0) và `shot_speed` (> 0).
- Chém mặc định: hằng số `SlashWindup` trong `internal/combat`.
- `gamedata` kiểm tra các ràng buộc trên khi nạp.

## 3. Server

### 3.1. Hàng đợi đòn đang bay

Zone có một hàng đợi impact. Mỗi impact gồm: tick trúng, thứ tự tạo, người ra đòn (id và là người chơi hay quái), mục tiêu (id; với quái thêm **số lần hồi sinh** lúc bấm), sát thương đã chốt, `skill_id` (rỗng cho đòn cơ bản), cờ `ranged` (chưởng quái), và cấp người ra đòn (cho cửa sổ EXP, lấy lúc trúng từ người chơi nếu còn trong zone).

Quái có thêm bộ đếm số lần hồi sinh, tăng mỗi lần hồi sinh.

### 3.2. Thứ tự tick

```
drain → runImpacts → resolveAttacks → movePlayers → thinkMonsters → runSpawnTimers → runPlayerTimers → syncView
```

- `runImpacts` áp mọi impact có tick trúng ≤ tick hiện tại, theo tick trúng rồi thứ tự tạo.
- `resolveAttacks` (chém và chiêu của người chơi) và `thinkMonsters` (đòn của quái) chỉ tạo impact và phát `Cast`; không trừ máu.

### 3.3. Áp impact

**Mục tiêu là quái:** bỏ nếu quái không còn, đã chết, hoặc số lần hồi sinh khác lúc bấm. Nếu không:

- sát thương tính EXP = min(sát thương, HP còn lại); trừ máu; phát `Hit{attacker, target, damage, hp_after, skill_id}`;
- nếu người ra đòn còn trong zone: trả EXP theo phần sát thương (cửa sổ ±10 cấp như hiện tại), ghi vào danh sách người đánh, quái chưa đuổi thì đuổi người đó;
- quái về 0 HP thì chết; đòn này là đòn kết liễu.

Nhiều impact cùng trúng một quái trong một tick được áp lần lượt; sau đòn kết liễu các đòn còn lại bị bỏ.

**Mục tiêu là người chơi:** bỏ nếu người chơi không còn trong zone hoặc đã chết. Nếu không, gây sát thương như `damagePlayer` hiện tại (cờ `ranged` với chưởng). Người chơi đã chạy vào khu an toàn sau khi quái ra đòn vẫn dính đòn đó; kiểm tra khu an toàn của quái vẫn ở lúc quái quyết định ra đòn.

### 3.4. AI quái

Nhịp đánh (`attack_sec`) và nhịp bắn (`shoot_sec`) giữ nguyên, tính từ lúc quái ra đòn. Mọi luật đuổi, bỏ cuộc, thù và "quay sang người vừa đánh" phản ứng với lúc **trúng** (impact áp lên quái), không phải lúc người chơi bấm.

### 3.5. Không lưu

Impact không ghi xuống DB. Zone dừng thì các đòn đang bay mất theo.

## 4. Giao thức

`ProtocolVersion` lên **5**.

```proto
message Cast {
  uint64 caster_id = 1;   // người chơi hoặc quái
  string skill_id = 2;    // rỗng: đòn cơ bản (chém mặc định, quái cận chiến, chưởng quái)
  uint64 target_id = 3;
  float dir_x = 4;
  float dir_y = 5;
  int32 level = 6;
  int32 windup_ms = 7;
  repeated CastImpact impacts = 8;
  bool ranged = 9;        // chưởng của quái
}

message CastImpact {
  uint64 target_id = 1;
  int32 impact_ms = 2;    // từ tick của Cast tới lúc trúng: (tick trúng − t0) × 1000 / tick rate
}
```

- Chém mặc định và đòn của quái cũng phát `Cast` ở mốc 1. Người thấy người ra đòn (hoặc là người ra đòn) nhận `Cast`.
- `impacts` không chứa sát thương; số sát thương chỉ đến cùng `Hit` ở mốc 3. Chiêu không trúng ai có `impacts` rỗng.
- `Hit` giữ nguyên các trường, phát ở tick trúng, gửi cho người thấy người ra đòn hoặc mục tiêu như hiện tại.
- Thứ tự áp một `WorldTick` giữ nguyên: `moves`, `casts`, `hits`, `deaths`, `hp_changes`, `level_ups`.
- Fixture: cập nhật `world_tick_cast` (có `windup_ms`, `impacts`); thêm `world_tick_monster_attack` (một `Cast` của quái có `ranged`) và `world_tick_impact` (chỉ có `Hit` của một đòn ra từ trước).

## 5. Client

### 5.1. `core/attack_timeline.gd` (không dùng Node)

- Nhận `Cast` cùng thời điểm client nhận nó (giờ client). Lúc hết chuẩn bị = nhận + `windup_ms`; lúc trúng mục tiêu i = nhận + `impact_ms` của i.
- Giữ danh sách đạn đang bay, khóa theo (người ra đòn, mục tiêu).
- Khi `Hit` về:
  - đạn chưa tới nơi: đạn bay nhanh nốt trong khoảng 0,1 giây rồi nổ và hiện số;
  - đạn đã tới mà `Hit` chưa về: đạn lơ lửng ở mục tiêu, chờ tối đa 0,4 giây; `Hit` về thì nổ và hiện số;
  - quá 0,4 giây sau lúc trúng dự tính mà không có `Hit`: đạn mờ đi, không hiện số (server đã bỏ đòn).
- `Hit` không khớp `Cast` nào (người ra đòn ngoài tầm nhìn): hiện hiệu ứng trúng và số ngay.

### 5.2. Hiển thị

| Mốc | Chiêu và chém | Quái |
| --- | --- | --- |
| 1. Chuẩn bị | Vòng tụ lực màu class quanh người ra đòn; chém mặc định là vệt vung kiếm ngắn | Quái nhún hoặc lóe sáng |
| 2. Bay | Đạn đuổi theo vị trí hiện tại của mục tiêu; `cone`/`line` là sóng lan ra theo tầm; `circle_target` là đạn bay tới rồi nổ vòng tròn; `circle_self` là vòng bùng ra khi hết chuẩn bị; lao tới là nhân vật trượt từ chỗ cũ tới chỗ mới | Chưởng: quả cầu bay (`ui/projectile_effect.gd`, nay do `Cast` điều khiển) |
| 3. Trúng (`Hit`) | Lóe sáng tại mục tiêu, số sát thương, thanh máu tụt | Như cột trái |

**Lao tới:** nhân vật của mình đã nhảy vị trí dự đoán khi nhận `snap` (`predictor.teleport`), riêng hình vẽ trượt từ chỗ cũ tới chỗ mới trong thời gian lao và camera trượt theo. Người khác: `snap` của chiêu lao không nhảy cóc mà trượt theo thời gian lao lấy từ `Cast`.

**Không đổi:** `cast_loop`, `attack_loop` (hồi chiêu bắt đầu ở `CastResp`/`AttackResp`), thanh 5 ô, các nút.

## 6. Kiểm thử

**Server (`go test -race`):**

- Dữ liệu: các script sinh trường mới đúng bảng mục 2.3; `gamedata` từ chối chuẩn bị ≤ 0, tốc độ âm, `dash` tốc độ 0, `circle_self` tốc độ khác 0, quái thiếu trường.
- Thời gian: chém trúng ở t0 + ticks(0,15); chiêu đơn cách 5 ô trúng ở t0 + ticks(0,5); quạt có quái ở 2 ô và 4 ô trúng ở hai tick khác nhau; `circle_target` trúng cả vòng cùng tick; lao tới trúng ở t0 + ticks(0,1 + quãng lao/25).
- Trong tick bấm: máu quái chưa đổi, chưa có `Hit`, chưa trả EXP, quái chưa đuổi.
- Bỏ đòn: quái chết trước khi đạn tới (không `Hit`, không EXP); quái chết rồi hồi sinh trước khi đạn tới (không trúng con mới); người chơi bị quái đánh chết hoặc rời zone trước khi đòn quái tới.
- Kết liễu: hai người chơi, đòn trúng trước là đòn kết liễu; đòn sau bị bỏ.
- Người ra đòn rời zone trước khi trúng: đòn vẫn trúng, không EXP.
- Quái chết trước khi chưởng tới: chưởng vẫn trúng. Người chơi vào khu an toàn sau khi quái ra đòn: vẫn dính.
- `Cast` có `windup_ms`, `impacts` đúng; chém mặc định và quái phát `Cast`; tầm nhìn của `Cast` và `Hit`.
- Gateway: `ProtocolVersion` 5, fixture mới giải mã đúng. e2e: các test chờ `Hit` sau khi đánh/dùng chiêu vẫn qua với độ trễ mới.

**Client (`tests/run.sh`):** `attack_timeline` (mốc từ fixture; `Hit` sớm; `Hit` muộn; không `Hit` thì mờ, không số; `Hit` không khớp hiện ngay); `entity_table` tạo hiệu ứng từ `Cast` của quái, `Hit` chỉ còn số và hiệu ứng trúng; smoke với server thật kiểm tra `Hit` của chiêu về sau `Cast` ít nhất bằng `windup_ms`.

**Nghiệm thu:** hai cửa sổ trên server `:8199`, `test1`/`test2` cấp 90: mỗi chiêu thấy rõ ba mốc; chưởng của quái trúng đúng lúc quả cầu chạm người.

## 7. Tài liệu cập nhật

- `docs/game-design.md` mục 2.4: mô hình ba mốc (server chốt đòn lúc bấm, trừ máu lúc trúng; hoạt ảnh không khóa lệnh tiếp theo); mục 5.8: người kết liễu là đòn trúng trước.
- Spec quái thường theo vùng: ghi chú "đã thay bằng spec 2026-11-20 ba mốc" ở chỗ nói chưởng gây sát thương ngay và client vẽ quả cầu 0,3 giây.
- `CLAUDE.md`: thứ tự tick mới.
