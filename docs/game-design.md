# Thiết kế trò chơi

Cập nhật: 2026-11-18.

Tài liệu tổng hợp buổi thảo luận thiết kế game nhập vai trực tuyến trên di động, lấy cảm hứng từ Khí Phách Anh Hùng Online và Demon Slayer. Đây là thiết kế sản phẩm, chưa phải đặc tả triển khai kỹ thuật. Những thông số ghi là dự kiến và những mục cuối tài liệu chưa được chốt.

## 1. Định hướng chung và hình ảnh

- Game nhập vai online cày cuốc: đánh quái, lên cấp, phát triển kỹ năng, săn trang bị và tham gia hoạt động cộng đồng/PK. Chỉ lấy cảm hứng hình ảnh, nhân vật và bối cảnh từ truyện, không thiết kế theo tiến trình cốt truyện nguyên tác.
- Người chơi tự do qua lại các map để luyện cấp, săn đồ, làm nhiệm vụ và PK. Không yêu cầu hạ Boss hay hoàn thành tuyến truyện để mở map; không có cơ chế Boss kết vùng.
- Pixel art với nhân vật, quái, hiệu ứng và cảnh vật bằng sprite 2D; bản đồ ghép từ tileset.
- Góc nhìn top-down/2.5D, camera cố định nhìn chéo từ trên xuống khoảng 45 độ. Nhân vật di chuyển trên mặt bản đồ, không phải đi cảnh ngang. Chưa chốt dùng lưới isometric.
- Nhân vật chibi nhẹ, đầu hơi lớn so với thân, phong cách cổ trang lấy cảm hứng từ Demon Slayer.
- Ngoại hình thể hiện thay đổi vũ khí, giáp, thời trang và thú cưỡi.

## 2. Class, thuộc tính và kỹ năng

### 2.1. Năm class hơi thở

| Class | Loại sát thương chính |
| --- | --- |
| Viêm | Vật lý |
| Minh | Vật lý |
| Nham | Vật lý |
| Thủy | Phép |
| Phong | Phép |

Viêm, Thủy, Minh, Nham, Phong là tên class trong game; năm hệ nguyên tố của trang bị (mục 3.2) dùng cùng các tên này.

Cả năm class đều sử dụng kiếm thuật và kỹ thuật hơi thở. Không mặc định gắn các điểm đánh đổi, chuỗi combo hay cơ chế thưởng sát thương combo cho class. Điểm mạnh cụ thể của từng class chưa chốt.

Mỗi class có một đòn chém mặc định và năm kỹ năng riêng, gồm **bốn chủ động và một bị động**. Từ cấp 1 đến 9, mọi class chỉ có chém mặc định giống nhau. Các kỹ năng riêng lần lượt học ở cấp **10, 20, 30, 40, 50**; bị động học ở cấp 30. Chém mặc định vẫn sử dụng được sau khi học kỹ năng riêng.

### 2.2. Điểm tiềm năng

Mỗi lần lên cấp nhân vật nhận **10 điểm tiềm năng** để tự phân bổ; điểm chưa dùng được tích lại. Bảng quy đổi sau là **bộ số khởi đầu đã duyệt sau kiểm tra PvP cấp 10**. Tiếp tục kiểm chứng ở các cấp cao và với trang bị theo [nguyên tắc cân bằng chiến đấu](combat-balance.md).

| Thuộc tính | Mức tăng mỗi 1 điểm | Vai trò |
| --- | --- | --- |
| STR — Sức mạnh | +2 công vật lý | Sát thương chính của Viêm, Minh, Nham |
| AGI — Thân pháp | +3 chính xác, +2 né tránh, +0,02 điểm phần trăm tỷ lệ chí mạng | Đánh trúng, né đòn và chí mạng |
| VIT — Thể lực | Class vật lý (Viêm, Minh, Nham): +20 HP tối đa; class phép (Thủy, Phong): +16 HP tối đa. Mọi class: +2 thủ vật lý, +2 thủ phép | Khả năng chịu đòn cho cả năm class |
| INT — Trí tuệ/Linh lực | +2 công phép, +5 MP tối đa | Sát thương chính của Thủy, Phong và tài nguyên thi triển |

Phần tăng thủ phép được chuyển sang VIT; INT không tăng thủ phép. Chính xác và né tránh là chỉ số, không phải phần trăm xác suất trực tiếp. Ví dụ 100 AGI tăng 300 chính xác, 200 né tránh và **2 điểm phần trăm** tỷ lệ chí mạng, trước khi áp dụng giới hạn chí mạng chung.

Ví dụ 100 STR tăng 200 công vật lý: Bất Tri Hỏa cấp kỹ năng 10 (285%) tăng 570 sát thương cơ sở; Luyện Ngục cấp kỹ năng 10 (475%) tăng 950. Với 100 INT, mức tăng tương ứng áp dụng cho kỹ năng phép cùng hệ số. 100 VIT tăng 2.000 HP với class vật lý hoặc 1.600 HP với class phép, cùng 200 mỗi loại phòng thủ. Đây là mức tăng từ tiềm năng, trước khi xét bị động và các hiệu ứng tăng chỉ số khác.

Chỉ số chiến đấu tổng hợp từ chỉ số nền theo cấp, tiềm năng, trang bị (gồm dòng phụ, dòng ẩn, cường hóa và ngọc khảm), kỹ năng bị động, thú cưỡi và linh thú/trợ thủ: HP, MP, công/thủ vật lý, công/thủ phép, chính xác, né tránh, tỷ lệ và sát thương chí mạng, tốc độ di chuyển.

Các thuộc tính bổ sung gồm kháng sát thương theo hệ, kháng hiệu ứng hoặc giảm thời gian khống chế; danh vọng/công trạng phục vụ tiến trình nhân vật. PK dùng chế độ bật/tắt, không yêu cầu hệ thống ác danh. Người bật PK đánh được mọi người chơi, và mọi người chơi đều đánh được người bật PK; hai người cùng tắt PK không đánh được nhau (mục 5.7). Khi đang bật PK mà bị người chơi khác giết sẽ mất EXP theo mục 5.7; không có lính canh trừng phạt người bật PK.

#### Chỉ số nền và tăng theo cấp

Các class dùng chung chỉ số nền, trừ HP: class phép (Thủy, Phong) tăng ít HP hơn class vật lý (Viêm, Minh, Nham), để đánh đổi việc INT đồng thời tăng công phép và MP. Với cấp nhân vật **L**, chỉ số nền = giá trị cấp 1 + mức tăng × (L − 1).

| Chỉ số | Cấp nhân vật 1 | Tăng mỗi lần lên cấp |
| --- | --- | --- |
| HP tối đa | 500 | Class vật lý +60; class phép +50 |
| MP tối đa | 100 | +20 |
| Công vật lý | 20 | +5 |
| Công phép | 20 | +5 |
| Thủ vật lý | 10 | +2 |
| Thủ phép | 10 | +2 |
| Chính xác | 100 | +5 |
| Né tránh | 50 | +5 |
| Tỷ lệ chí mạng | 5% | Không tự tăng |
| Tốc độ di chuyển | 1 (hệ số chuẩn) | Không tự tăng |

Tốc độ di chuyển 1 là mức chuẩn cho mọi class. Các nguồn khác cộng theo phần trăm của mức này, ví dụ thú cưỡi hoặc quái "+20% tốc độ chạy". Số ô mỗi giây ứng với hệ số 1 sẽ chốt sau khi chơi thử.

#### EXP lên cấp

Càng lên cao càng cần nhiều EXP. EXP cần để lên từ cấp **L** lên **L + 1** bằng EXP của **(10 + 2 × (L − 1)^1,5) × hệ số cấp** quái thường cùng cấp, mỗi con cho 10 × L EXP (mục 5.12):

**EXP lên cấp kế tiếp = 10 × L × (10 + 2 × (L − 1)^1,5) × hệ số cấp**, làm tròn đến 10.

| Cấp hiện tại | Hệ số cấp | Mục đích |
| --- | --- | --- |
| 1–9 | 4 × L / 3 (từ 1,33 đến 12) | Từ cấp 1 lên cấp 10 mất khoảng **3 giờ** nếu chỉ đánh quái |
| Từ 10 trở lên | 10 + 0,5 × (L − 10): cấp 10 là 10, cấp 20 là 15, cấp 30 là 20 … cấp 80 là 45, cấp 90 là 50 | Luyện cấp dài hạn, càng lên cao càng chậm |

| Cấp hiện tại | EXP lên cấp kế tiếp | Số quái thường cùng cấp | Tổng EXP từ cấp 1 | Giờ đánh quái ước tính |
| --- | --- | --- | --- | --- |
| 1 | 130 | 13 | 130 | ~0 |
| 5 | 8.670 | 173 | 15.670 | 0,5 |
| 9 | 59.680 | 663 | 156.760 | 3,0 |
| 10 | 64.000 | 640 | 220.760 | 3,9 |
| 20 | 526.910 | 2.635 | 2.876.490 | 26,2 |
| 30 | 1.934.040 | 6.447 | 14.842.000 | 89,6 |
| 40 | 4.971.100 | 12.428 | 49.226.980 | 222,2 |
| 50 | 10.440.000 | 20.880 | 126.652.560 | 456,3 |
| 60 | 19.243.880 | 32.073 | 276.325.610 | 828,6 |
| 70 | 32.376.790 | 46.253 | 536.991.700 | 1.378,8 |
| 80 | 50.916.050 | 63.645 | 957.800.360 | 2.150,2 |
| 89 | 73.176.520 | 82.221 | 1.523.087.240 | 3.071,9 |

- Hệ số tăng 0,5 mỗi cấp nên không có bước nhảy: cấp 9→10 và cấp 10→11 đều mất khoảng 0,9–1,1 giờ đánh quái. Cấp 1–10 là giai đoạn làm quen; từ cấp 10 bắt đầu luyện cấp dài hạn.
- Nhiệm vụ hàng ngày, chính tuyến, phụ tuyến và bang hội là nguồn EXP chính bổ sung ngoài đánh quái, nên thời gian lên cấp thực tế ngắn hơn bảng.
- EXP tổ đội (mục 5.8) cũng rút ngắn thời gian. Ước tính chỉ tính đánh quái, chưa gồm nhiệm vụ:

| Cách chơi | Hệ số EXP từ quái | Lên cấp 10 | Lên cấp 50 | Lên cấp 90 |
| --- | --- | --- | --- | --- |
| Đánh một mình | ×1 | ~3 giờ | ~456 giờ | ~3.072 giờ |
| Tổ đội 5 người cùng đánh | ×1,2 | ~2,5 giờ | ~380 giờ | ~2.560 giờ |

Tổ đội ×1,2 giả định cả năm người hạ quái nhanh như nhau. Trên thực tế, năm người chung một bãi 12 quái hồi sinh 8 giây có thể không đủ quái cho cả nhóm.
- Tổng EXP lên cấp 90 khoảng 1,5 tỷ, gần giới hạn số nguyên 32-bit có dấu (khoảng 2,1 tỷ). Server và database lưu EXP bằng số nguyên 64-bit.

- Cột "Tổng EXP từ cấp 1" là tổng EXP đến khi lên được cấp kế tiếp. Ví dụ ở dòng cấp 89, đó là tổng EXP từ cấp 1 lên cấp 90.
- Cột giờ ước tính giả định đánh một mình, 5 giây mỗi quái thường cùng cấp, chưa tính nhiệm vụ, tinh anh, thủ lĩnh, sự kiện và EXP tổ đội. Thời gian thực tế sẽ ngắn hơn khi có các nguồn này.
- Bảng đầy đủ từng cấp nằm trong [CSV EXP lên cấp](balance/exp-table.csv), xuất bằng [script EXP](balance/exp_table.py).

#### Công thức phòng thủ và sát thương PvP ban đầu

- Dùng thủ vật lý cho sát thương vật lý, thủ phép cho sát thương phép.
- **Tỷ lệ giảm sát thương do thủ = Thủ / (Thủ + 20 × cấp người tấn công)**, với chỉ số thủ không âm. Quy tắc xuyên giáp, giới hạn giảm sát thương và kháng sẽ được thiết kế riêng.
- **Hệ số sát thương PvP = 100% (1,0)**. Không dùng hệ số 35% cũ; phòng thủ và kháng vẫn được xét, không phải bỏ phòng thủ khi đánh người.
- Khi chưa xét kháng, chí mạng và hiệu ứng bổ sung: **sát thương thực nhận = floor(Công dùng cho đòn × Hệ số đòn × (1 − Giảm do thủ) × Hệ số PvP)**.
- Xác suất đánh trúng/né, công thức kháng, hệ số và giới hạn chí mạng chưa chốt. Mức sát thương chí mạng 1,5–2 lần vẫn là định hướng, chưa phải công thức cuối.

#### Mục tiêu và kết quả cân bằng

- **Mục tiêu đã chốt:** PvP giữa hai nhân vật cùng cấp, cách cộng điểm và sức mạnh tương đương cần khoảng **10 đòn trúng để hạ đối phương**, cả khi không đồ lẫn mặc trang bị. Cùng bậc, phẩm chất và mức cường hóa, từ +0 đến +15, vẫn phải quanh 10 đòn; không giới hạn mục tiêu ở đầu game. Khoảng kiểm tra là **8–12 đòn khi bên bị đánh là class vật lý, 7–12 đòn khi bên bị đánh là class phép**, vì class phép có HP thấp hơn theo chủ đích. Khi thiết kế công thức phải dùng [nguyên tắc cân bằng chiến đấu](combat-balance.md).
- Hai nhân vật Viêm cấp 10 không đồ, mỗi người 60 STR/30 VIT: công vật lý **185**, HP **1.640**, thủ vật lý **88**, MP **280**. Bất Tri Hỏa cấp kỹ năng 1 gây **192** sát thương thực nhận; chém mặc định gây **128**.
- Không trượt, không chí mạng, không hồi máu: chỉ dùng Bất Tri Hỏa cần **9 đòn trúng**; xen một Bất Tri Hỏa rồi một chém mặc định cần **11 đòn trúng**. Đây là cấu hình tham chiếu đã kiểm tra, không ép mọi cách cộng điểm đều có cùng kết quả.
- Nhân vật Thủy cấp 10 không đồ, 60 INT/30 VIT: công phép **185**, HP **1.430**, mỗi loại thủ **88**, MP **580**. Viêm đánh Thủy cùng cấu hình: chỉ dùng Bất Tri Hỏa cần **8 đòn trúng**, xen kỹ năng với chém mặc định cần **9 đòn trúng**.
- Chém mặc định dùng chỉ số công cao hơn để nhân vật cộng INT vẫn đánh thường hiệu quả. Nó vẫn gây sát thương vật lý, nên đối phương dùng thủ vật lý để giảm đòn chém.
- Với bốn kỹ năng chủ động cấp 10 của Viêm/Thủy/Minh/Nham, tổng sát thương cơ sở lý thuyết khoảng **10,51 lần công/giây** và tiêu hao khoảng **160 MP/giây**, nếu luôn có mục tiêu và dùng ngay khi từng chiêu hồi xong. Đây không phải sát thương thực nhận hoặc bằng chứng cân bằng cấp cao.
- Cần kiểm tra tiếp ở các mốc cấp cao, cách cộng khác, ghép cặp class, trang bị, quái và bình hồi phục. Bộ +80 HP/VIT và PvP 35% trước đây đã được thay thế; không dùng lại làm cấu hình hiện hành.

### 2.3. Phân loại kỹ năng

- Đánh thường: không tốn MP; hồi chiêu cố định 0,8 giây.
- Chủ động đơn mục tiêu: người chơi kích hoạt, tác động lên một mục tiêu hợp lệ.
- Chủ động nhiều mục tiêu: tác động lên nhiều mục tiêu hợp lệ, tính sát thương riêng cho từng mục tiêu.
- Chủ động dùng MP và có hồi chiêu; có thể có kỹ năng hỗ trợ/buff.
- Bị động: tự có hiệu lực hoặc kích hoạt khi đủ điều kiện, không cần bấm nút.

Loại sát thương vật lý/phép và phạm vi đơn/nhiều mục tiêu là hai thuộc tính độc lập. Chém mặc định là kỹ năng chung, không cộng điểm, gây sát thương vật lý với sát thương cơ sở bằng **100% × max(Công vật lý, Công phép)** cho mọi class.

### 2.4. Server xử lý sát thương, client hiển thị animation

- Mỗi lệnh đánh hoặc dùng kỹ năng được server kiểm tra điều kiện, tính và áp dụng kết quả; không chờ animation kết thúc.
- Mỗi lần xử lý sử dụng một kỹ năng; nhiều lệnh bấm không gộp thành một đòn combo.
- Nếu đang phát animation kỹ năng 1 mà người chơi dùng kỹ năng 2 hợp lệ, client ngắt animation 1 để chuyển sang animation 2.
- Ngắt animation không hủy hay hoàn lại sát thương đã được server xử lý.
- Mỗi kỹ năng tính sát thương độc lập. Không thưởng sát thương do bấm đồng thời hoặc dùng liên tiếp.
- Animation không khóa việc nhận lệnh tiếp theo. Server vẫn kiểm tra MP, hồi chiêu và điều kiện thi triển.

### 2.5. Chọn mục tiêu

Nhân vật có vùng quét tròn hoặc elip để phát hiện thực thể xung quanh. Hệ thống dùng trạng thái PK để xác định mục tiêu được phép tấn công.

- **Tầm phát hiện: 6 ô** quanh nhân vật để tìm mục tiêu mới.
- **Tầm giữ khóa: 8 ô**. Mục tiêu đã khóa vẫn được giữ khi ra ngoài 6 ô nhưng chưa vượt 8 ô; tầm giữ khóa không thay thế tầm đánh riêng của từng kỹ năng.
- Khi đã khóa mục tiêu nhưng ngoài tầm đánh, nhấn đánh/dùng kỹ năng sẽ tự chạy đến đủ tầm rồi thực hiện. Server vẫn kiểm tra mục tiêu, khoảng cách, vật cản, MP và hồi chiêu tại lúc xử lý đòn.
- Bỏ khóa khi khoảng cách vượt 8 ô hoặc một trong hai bên đổi map/khu.
- Khi mục tiêu chết, chuyển sang mục tiêu hợp lệ khác trong tầm phát hiện; nếu không có thì bỏ mục tiêu đã chết.
- NPC có thể là mục tiêu tương tác nhưng không mặc định là mục tiêu tấn công. Bộ lọc PK cần kiểm tra từng mục tiêu của kỹ năng nhiều mục tiêu.
- Chọn mục tiêu mới: ưu tiên mục tiêu hợp lệ gần nhân vật nhất trong tầm phát hiện 6 ô.
- Khi chiêu diện rộng vượt giới hạn số mục tiêu: ưu tiên mục tiêu đang khóa nếu hợp lệ và nằm trong vùng chiêu, sau đó lấy các mục tiêu hợp lệ gần tâm vùng chiêu nhất đến khi đủ giới hạn.

### 2.6. Bảng kỹ năng và cấp học

Tên thức và hình ảnh lấy cảm hứng từ nguyên tác; công dụng được chuyển thể cho game, đặc biệt với các thức chuyển thành bị động. Số thứ tự thức không phải thứ tự học kỹ năng.

Tầm đánh dùng **ô bản đồ** làm đơn vị, đo trên mặt đất từ chân nhân vật tới chân mục tiêu, không theo khoảng cách pixel khi nhìn chéo. Kích thước ô quy đổi sang tọa độ thế giới chưa chốt. Với kỹ năng chọn mục tiêu rồi gây sát thương vùng, tầm chọn và bán kính vùng là hai thông số riêng. Các khoảng cách dưới đây là thông số thiết kế ban đầu để cân bằng tiếp.

#### Cấp kỹ năng, điểm cộng và sát thương

- Mỗi kỹ năng riêng tối đa **10 cấp**, mỗi lần cộng tốn **1 điểm kỹ năng**. Học lần đầu là cấp 1 và cũng tốn một điểm; nâng một kỹ năng từ chưa học lên tối đa tốn 10 điểm. Năm kỹ năng cần tổng cộng **50 điểm** để tối đa.
- Mỗi lần lên cấp nhân vật nhận **1 điểm kỹ năng**; điểm chưa dùng được tích lại.
- Gọi **n** là cấp kỹ năng, từ 1 đến 10. Cấp nhân vật để học lần đầu là 10/20/30/40/50. Mỗi lần nâng tiếp yêu cầu thêm **1 cấp nhân vật**: **cấp nhân vật yêu cầu = cấp học ban đầu + (n − 1)**. Ví dụ kỹ năng học từ cấp nhân vật 10: cấp kỹ năng 2 cần cấp nhân vật 11, cấp kỹ năng 10 cần cấp nhân vật 19. Các kỹ năng học từ cấp 20/30/40/50 lần lượt có thể đạt cấp kỹ năng 10 ở cấp nhân vật 29/39/49/59, nếu đủ điểm. Có đủ điểm cũng không được bỏ qua điều kiện cấp.
- Học lần đầu cần đủ cấp nhân vật, **1 điểm kỹ năng và sách của kỹ năng tương ứng**. Sách nhận từ nhiệm vụ chính tuyến để người chơi tham gia nhiệm vụ, không chỉ treo máy luyện cấp. Sách dùng khi học lần đầu; nâng tiếp không cần thêm sách.
- Sát thương cơ sở = **Công tương ứng × hệ số kỹ năng**. Viêm, Minh, Nham dùng công vật lý; Thủy, Phong dùng công phép.
- Các hệ số dưới đây tính trước phòng thủ, chí mạng và hiệu ứng khác; là bộ thông số ban đầu để cân bằng cùng MP/hồi chiêu.
- Kỹ năng nhiều mục tiêu gây đủ sát thương riêng cho từng mục tiêu, không chia tổng sát thương theo số mục tiêu. Mỗi lần dùng tính một lần sát thương cho mỗi mục tiêu hợp lệ; nhiều vệt chém trong animation không tạo thêm lần tính sát thương.
- Bị động chỉ hỗ trợ bản thân, có hiệu lực khi học, không tốn MP, không có thời hạn và không chiếm nút bấm. Phần trăm tăng chỉ số tính trên chỉ số trước khi áp dụng chính bị động đó, tránh cộng lặp.

#### Giới hạn mục tiêu của kỹ năng diện rộng

| Cấp kỹ năng | Số mục tiêu tối đa |
| --- | --- |
| 1–3 | 2 |
| 4–6 | 3 |
| 7–9 | 4 |
| 10 | 5 |

Áp dụng cho tất cả kỹ năng nhiều mục tiêu. Chỉ đánh các mục tiêu hợp lệ nằm trong vùng ảnh hưởng; không mặc định đánh đủ số lượng nếu vùng có ít mục tiêu hơn. Cộng điểm tăng hệ số và số mục tiêu theo bảng, không tăng tầm đánh hoặc kích thước vùng. Khi vượt giới hạn, chọn theo thứ tự tại mục 2.5: mục tiêu đang khóa nếu hợp lệ trong vùng, rồi các mục tiêu gần tâm vùng chiêu nhất.

#### Chém mặc định — chung cho năm class

| Kỹ năng | Cấp có sẵn | Phân loại | Tầm đánh | Công dụng |
| --- | --- | --- | --- | --- |
| Chém mặc định | 1 | Chủ động · Đơn mục tiêu | 1,5 ô | Chém mục tiêu đang chọn, sát thương vật lý, sát thương cơ sở 100% × max(Công vật lý, Công phép); không cộng điểm, không tốn MP; hồi chiêu cố định 0,8 giây |

**Cấp 1–9 chỉ dùng chém mặc định.** Các mốc dưới đây là cấp nhân vật cần đạt để học, không phải cấp nâng kỹ năng hay điều kiện vào map.

#### Hơi thở Viêm — Sát thương vật lý

| Kỹ năng | Cấp học | Phân loại | Tầm đánh / vùng ảnh hưởng | Công dụng | Hệ số / hiệu quả theo cấp n | Cấp kỹ năng 1 | Cấp kỹ năng 10 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I — Bất Tri Hỏa | 10 | Chủ động · Đơn mục tiêu | Chọn mục tiêu trong 4 ô | Lao tới mục tiêu và chém lửa | 150% + 15% × (n − 1) công vật lý | 150% | 285% |
| II — Thăng Viêm Thiên | 20 | Chủ động · Nhiều mục tiêu | Hình quạt 120°, vươn 2 ô phía trước | Chém vòng cung, gây sát thương các mục tiêu trong vùng | 110% + 10% × (n − 1) công vật lý / mục tiêu | 110% | 200% |
| IV — Thịnh Viêm Lãng Triều | 30 | Bị động | Chỉ bản thân | Tăng thủ vật lý và thủ phép, hình tượng vòng lửa bảo vệ | +2% × n thủ vật lý và thủ phép | +2% mỗi loại | +20% mỗi loại |
| V — Viêm Hổ | 40 | Chủ động · Nhiều mục tiêu | Chọn mục tiêu trong 5 ô; vùng tròn bán kính 1,5 ô quanh mục tiêu | Phóng hình hổ lửa, gây sát thương trong vùng quanh mục tiêu chính | 160% + 15% × (n − 1) công vật lý / mục tiêu | 160% | 295% |
| IX — Luyện Ngục | 50 | Chủ động · Đơn mục tiêu | Chọn mục tiêu trong 5 ô | Đòn sát thương lớn hình rồng lửa lên mục tiêu đang chọn | 250% + 25% × (n − 1) công vật lý | 250% | 475% |

#### Hơi thở Thủy — Sát thương phép

| Kỹ năng | Cấp học | Phân loại | Tầm đánh / vùng ảnh hưởng | Công dụng | Hệ số / hiệu quả theo cấp n | Cấp kỹ năng 1 | Cấp kỹ năng 10 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I — Thủy Diện Trảm | 10 | Chủ động · Đơn mục tiêu | 3 ô | Chém ngang bằng lưỡi nước vào mục tiêu đang chọn | 150% + 15% × (n − 1) công phép | 150% | 285% |
| IV — Đả Triều | 20 | Chủ động · Nhiều mục tiêu | Hình quạt 120°, vươn 4 ô phía trước | Tạo sóng nước gây sát thương các mục tiêu trong vùng | 110% + 10% × (n − 1) công phép / mục tiêu | 110% | 200% |
| XI — Lặng | 30 | Bị động | Chỉ bản thân | Tăng né tránh, thể hiện khả năng hóa giải đòn đánh | +3% × n chỉ số né tránh | +3% | +30% |
| VI — Liệt Oa | 40 | Chủ động · Nhiều mục tiêu | Vòng tròn bán kính 2,5 ô quanh bản thân | Tạo xoáy nước gây sát thương xung quanh | 160% + 15% × (n − 1) công phép / mục tiêu | 160% | 295% |
| X — Sinh Sinh Lưu Chuyển | 50 | Chủ động · Đơn mục tiêu | 5 ô | Đòn sát thương lớn hình rồng nước lên mục tiêu đang chọn; không tăng sát thương nhờ dùng chiêu trước đó | 250% + 25% × (n − 1) công phép | 250% | 475% |

#### Hơi thở Minh — Sát thương vật lý

| Kỹ năng | Cấp học | Phân loại | Tầm đánh / vùng ảnh hưởng | Công dụng | Hệ số / hiệu quả theo cấp n | Cấp kỹ năng 1 | Cấp kỹ năng 10 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I — Phích Lịch Nhất Thiểm | 10 | Chủ động · Đơn mục tiêu | Chọn mục tiêu trong 5 ô | Lướt tới mục tiêu, gây sát thương bằng nhát chém chớp nhoáng | 150% + 15% × (n − 1) công vật lý | 150% | 285% |
| III — Tụ Văn Thành Lôi | 20 | Chủ động · Nhiều mục tiêu | Vòng tròn bán kính 2 ô quanh bản thân | Phát vòng sét gây sát thương xung quanh | 110% + 10% × (n − 1) công vật lý / mục tiêu | 110% | 200% |
| V — Nhiệt Giới Lôi | 30 | Bị động | Chỉ bản thân | Tăng tỷ lệ chí mạng; công dụng chuyển thể cho game | +n điểm phần trăm tỷ lệ chí mạng | +1 điểm % | +10 điểm % |
| IV — Viễn Lôi | 40 | Chủ động · Nhiều mục tiêu | Dải thẳng dài 6 ô, rộng 1,5 ô, hướng về mục tiêu | Phóng luồng sét gây sát thương các mục tiêu trên đường | 160% + 15% × (n − 1) công vật lý / mục tiêu | 160% | 295% |
| VII — Hỏa Lôi Thần | 50 | Chủ động · Đơn mục tiêu | 6 ô | Đòn sát thương lớn hình rồng sét lên mục tiêu đang chọn | 250% + 25% × (n − 1) công vật lý | 250% | 475% |

#### Hơi thở Nham — Sát thương vật lý

Giữ đủ năm thức. Vũ khí riêng của class Nham là rìu và chùy nối xích, tính một món ở ô vũ khí, theo mục 3.1.

| Kỹ năng | Cấp học | Phân loại | Tầm đánh / vùng ảnh hưởng | Công dụng | Hệ số / hiệu quả theo cấp n | Cấp kỹ năng 1 | Cấp kỹ năng 10 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I — Xà Văn Nham · Song Cực | 10 | Chủ động · Đơn mục tiêu | 3 ô | Đánh kẹp bằng rìu và chùy, gây sát thương tập trung | 150% + 15% × (n − 1) công vật lý | 150% | 285% |
| II — Thiên Diện Toái | 20 | Chủ động · Nhiều mục tiêu | Chọn mục tiêu trong 4 ô; vùng tròn bán kính 1,5 ô quanh mục tiêu | Đập chùy xuống vị trí mục tiêu chính, gây sát thương vùng | 110% + 10% × (n − 1) công vật lý / mục tiêu | 110% | 200% |
| III — Nham Phu Khu | 30 | Bị động | Chỉ bản thân | Tăng HP tối đa và thủ vật lý | +2% × n HP tối đa và thủ vật lý | +2% mỗi loại | +20% mỗi loại |
| IV — Lưu Văn Nham · Tốc Chinh | 40 | Chủ động · Nhiều mục tiêu | Vòng tròn bán kính 3 ô quanh bản thân | Quét rìu và chùy gây sát thương xung quanh | 160% + 15% × (n − 1) công vật lý / mục tiêu | 160% | 295% |
| V — Ngõa Hình Luân Bộ | 50 | Chủ động · Đơn mục tiêu | 4 ô | Đòn sát thương lớn khi rìu và chùy cùng đánh mục tiêu | 250% + 25% × (n − 1) công vật lý | 250% | 475% |

#### Hơi thở Phong — Sát thương phép

| Kỹ năng | Cấp học | Phân loại | Tầm đánh / vùng ảnh hưởng | Công dụng | Hệ số / hiệu quả theo cấp n | Cấp kỹ năng 1 | Cấp kỹ năng 10 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I — Trần Toàn Phong · Tước | 10 | Chủ động · Nhiều mục tiêu | Dải thẳng dài 5 ô, rộng 1,5 ô, hướng về mục tiêu | Phóng gió xoáy gây sát thương các mục tiêu trên đường | 110% + 10% × (n − 1) công phép / mục tiêu | 110% | 200% |
| II — Trảo · Khoa Hộ Phong | 20 | Chủ động · Đơn mục tiêu | 5 ô | Tập trung các vệt chém hình móng vuốt lên mục tiêu đang chọn | 150% + 15% × (n − 1) công phép | 150% | 285% |
| III — Tình Lam Phong Thụ | 30 | Bị động | Chỉ bản thân | Tăng chính xác và công phép | +2% × n chính xác; +1% × n công phép | +2% / +1% | +20% / +10% |
| V — Mộc Khô Lạp Phong | 40 | Chủ động · Nhiều mục tiêu | Chọn mục tiêu trong 5 ô; vùng tròn bán kính 2 ô quanh mục tiêu | Tạo bão kiếm gây sát thương vùng | 160% + 15% × (n − 1) công phép / mục tiêu | 160% | 295% |
| IX — Vi Đà Thiên Đài Phong | 50 | Chủ động · Nhiều mục tiêu | Chọn mục tiêu trong 6 ô; vùng tròn bán kính 3 ô quanh mục tiêu | Tạo cuồng phong diện rộng | 200% + 20% × (n − 1) công phép / mục tiêu | 200% | 380% |

#### MP và hồi chiêu

Gọi **n** là cấp kỹ năng. MP tăng tuyến tính theo bảng. Hồi chiêu cấp 1 lấy theo bảng; mỗi cấp tăng thêm giảm **10% hồi chiêu của cấp liền trước**, không trừ 10% mốc cấp 1 cố định.

**Hồi chiêu cấp n = Hồi chiêu cấp 1 × 0,9^(n − 1).**

Ví dụ chiêu có mốc 1,5 giây: cấp 2 còn 1,35 giây; cấp 3 còn 1,215 giây. Bảng làm tròn ba chữ số thập phân để hiển thị; tính toán dùng công thức gốc, không làm tròn sau mỗi cấp.

| Class | Kỹ năng | MP theo cấp n | MP cấp 1 → 10 | Hồi chiêu cấp 1 | Hồi chiêu cấp 10 |
| --- | --- | --- | --- | --- | --- |
| Viêm | I — Bất Tri Hỏa | 10 + 2 × (n − 1) | 10 → 28 | 1,5s | 0,581s |
| Viêm | II — Thăng Viêm Thiên | 15 + 3 × (n − 1) | 15 → 42 | 2,5s | 0,969s |
| Viêm | IV — Thịnh Viêm Lãng Triều | 0 | 0 | Không áp dụng | Không áp dụng |
| Viêm | V — Viêm Hổ | 25 + 4 × (n − 1) | 25 → 61 | 4s | 1,550s |
| Viêm | IX — Luyện Ngục | 40 + 5 × (n − 1) | 40 → 85 | 7,5s | 2,906s |
| Thủy | I — Thủy Diện Trảm | 10 + 2 × (n − 1) | 10 → 28 | 1,5s | 0,581s |
| Thủy | IV — Đả Triều | 15 + 3 × (n − 1) | 15 → 42 | 2,5s | 0,969s |
| Thủy | XI — Lặng | 0 | 0 | Không áp dụng | Không áp dụng |
| Thủy | VI — Liệt Oa | 25 + 4 × (n − 1) | 25 → 61 | 4s | 1,550s |
| Thủy | X — Sinh Sinh Lưu Chuyển | 40 + 5 × (n − 1) | 40 → 85 | 7,5s | 2,906s |
| Minh | I — Phích Lịch Nhất Thiểm | 10 + 2 × (n − 1) | 10 → 28 | 1,5s | 0,581s |
| Minh | III — Tụ Văn Thành Lôi | 15 + 3 × (n − 1) | 15 → 42 | 2,5s | 0,969s |
| Minh | V — Nhiệt Giới Lôi | 0 | 0 | Không áp dụng | Không áp dụng |
| Minh | IV — Viễn Lôi | 25 + 4 × (n − 1) | 25 → 61 | 4s | 1,550s |
| Minh | VII — Hỏa Lôi Thần | 40 + 5 × (n − 1) | 40 → 85 | 7,5s | 2,906s |
| Nham | I — Xà Văn Nham · Song Cực | 10 + 2 × (n − 1) | 10 → 28 | 1,5s | 0,581s |
| Nham | II — Thiên Diện Toái | 15 + 3 × (n − 1) | 15 → 42 | 2,5s | 0,969s |
| Nham | III — Nham Phu Khu | 0 | 0 | Không áp dụng | Không áp dụng |
| Nham | IV — Lưu Văn Nham · Tốc Chinh | 25 + 4 × (n − 1) | 25 → 61 | 4s | 1,550s |
| Nham | V — Ngõa Hình Luân Bộ | 40 + 5 × (n − 1) | 40 → 85 | 7,5s | 2,906s |
| Phong | I — Trần Toàn Phong · Tước | 15 + 3 × (n − 1) | 15 → 42 | 2,5s | 0,969s |
| Phong | II — Trảo · Khoa Hộ Phong | 10 + 2 × (n − 1) | 10 → 28 | 1,5s | 0,581s |
| Phong | III — Tình Lam Phong Thụ | 0 | 0 | Không áp dụng | Không áp dụng |
| Phong | V — Mộc Khô Lạp Phong | 25 + 4 × (n − 1) | 25 → 61 | 4s | 1,550s |
| Phong | IX — Vi Đà Thiên Đài Phong | 45 + 6 × (n − 1) | 45 → 99 | 7,5s | 2,906s |

- Hồi chiêu của từng chiêu độc lập, bắt đầu khi server chấp nhận và xử lý kỹ năng; MP được tiêu hao khi xử lý thành công.
- Có thể dùng chiêu khác ngay nếu đủ điều kiện, không cần chờ animation kết thúc. Ngắt animation không hoàn MP hoặc xóa hồi chiêu.
- Bị động không tiêu hao MP và không có hồi chiêu. Chém mặc định không tốn MP, hồi chiêu cố định 0,8 giây, không cộng điểm.
- Lượng MP và hồi chiêu là bộ số thiết kế ban đầu; cần cân bằng cùng MP nhân vật và bình hồi phục.

#### Quy tắc áp dụng và thông số còn mở

- Lặng tăng chỉ số né tránh, không cộng trực tiếp vào xác suất né; tỷ lệ né thực tế còn phụ thuộc chính xác đối phương. Nhiệt Giới Lôi cộng trực tiếp điểm phần trăm chí mạng: ví dụ tỷ lệ nền 5% thành 15% ở cấp kỹ năng 10, trước khi áp dụng giới hạn chí mạng chung; giới hạn này chưa chốt.
- Các kỹ năng chủ động tuân theo kiểm tra server, MP, hồi chiêu và bộ lọc PK; không có sát thương combo.
- Mỗi lần dùng kỹ năng gây sát thương tính một lần cho mỗi mục tiêu hợp lệ, độc lập với số vệt chém hiển thị.
- Server phải kiểm tra vị trí và vật cản với chiêu lao/lướt; animation nhanh không tự tăng số lần gây sát thương.
- Server xác định mục tiêu, vị trí và vùng trúng ngay khi xử lý lệnh kỹ năng hợp lệ, không đợi animation.

### 2.7. Hồi HP/MP

Bản v1.0.0.0 **không có hồi HP/MP tự nhiên**. Có ba nguồn hồi:
- nghỉ tại NPC trong khu an toàn (mục 5.4);
- hồi theo giây từ các mốc cường hóa trang bị;
- bình thuốc.

#### Hồi theo mốc cường hóa

Mỗi món đạt mốc cường hóa cố định sẽ kích hoạt hồi theo giây. Các mốc trùng với mốc vũ khí phát sáng: +7, +10, +12, +15. Mỗi món chỉ tính mốc cao nhất đã đạt, không cộng dồn các mốc thấp hơn của chính món đó. Các món khác nhau thì cộng dồn với nhau.

| Mốc của món | Món giáp: hồi HP mỗi giây | Vũ khí và trang sức: hồi MP mỗi giây |
| --- | --- | --- |
| +7 | 0,05% HP tối đa | 1 MP |
| +10 | 0,1% HP tối đa | 2 MP |
| +12 | 0,2% HP tối đa | 3 MP |
| +15 | 0,3% HP tối đa | 5 MP |

- **Món giáp hồi HP:** Giày, Hộ uyển, Quần, Mũ, Áo.
- **Món hồi MP:** Vũ khí, Dây chuyền, Ngọc bội, Nhẫn trái, Nhẫn phải.
- Đủ 10 món +15: hồi **1,5% HP tối đa và 25 MP mỗi giây**.
- Hồi cả khi đang giao chiến, mỗi giây một lần. Không hồi khi đã chết.
- HP hồi theo phần trăm vì HP tăng mạnh theo trang bị. MP hồi theo số cố định vì trang bị không cộng MP và chi phí MP của kỹ năng giống nhau cho mọi class. Nếu MP cũng hồi theo phần trăm thì class phép, vốn có nhiều MP hơn nhờ INT, sẽ được lợi thêm.
- Tham chiếu: quái thường cùng cấp đánh mỗi 1,5 giây, mỗi đòn khoảng 4% HP, tức khoảng 2,7% HP mỗi giây. Đủ bộ +15 bù được khoảng một nửa sát thương của một con.

#### Bình thuốc

Bán tại tạp hóa/dược sư, Điệp Phủ và Nhà Hoa Tử Đằng. Mỗi bình hồi ngay khi dùng. Người chơi dùng được mọi bình có cấp yêu cầu bằng hoặc thấp hơn cấp của mình.

| Bình | Cấp dùng | Hồi | Giá |
| --- | --- | --- | --- |
| Bình HP cấp 1 | 1 | 10% HP tối đa | 5 Xu |
| Bình HP cấp 10 | 10 | 15% HP tối đa | 30 Xu |
| Bình HP cấp 30 | 30 | 20% HP tối đa | 100 Xu |
| Bình HP cấp 50 | 50 | 25% HP tối đa | 200 Xu |
| Bình MP cấp 10 | 10 | 100 MP | 25 Xu |
| Bình MP cấp 30 | 30 | 250 MP | 80 Xu |
| Bình MP cấp 50 | 50 | 400 MP | 160 Xu |

- **Không có bình MP cấp 1**, vì từ cấp 1 đến 9 chỉ dùng chém mặc định, không tốn MP.
- **Hồi chiêu 10 giây**, HP và MP tính riêng. Mọi cấp bình HP dùng chung một hồi chiêu; bình MP cũng vậy.
- Với cấu hình tham chiếu, bình MP cấp 50 hồi khoảng 37% MP của class vật lý ở cấp 50 (1.080 MP). Ở cấp 10, bình MP cấp 10 hồi khoảng 36% (280 MP).
- Giá tính theo Xu rơi từ quái thường (trung bình 4 × cấp quái mỗi con, mục 5.12). Ở cấp 50, mỗi con cho khoảng 200 Xu. Dùng cả hai loại bình liên tục mỗi 10 giây tốn 360 Xu, xấp xỉ thu nhập khi hạ khoảng hai con trong 10 giây. Thực tế người chơi không dùng liên tục, nên bình là khoản tiêu Xu đáng kể nhưng vẫn kham được.
- PvP: một bình HP hồi tương đương khoảng 1,5–2,5 đòn tham chiếu. Phép đo 10 đòn hiện tại chưa tính bình và hồi theo cường hóa; phải kiểm tra lại theo [nguyên tắc cân bằng chiến đấu](combat-balance.md).

## 3. Trang bị và nâng cấp

### 3.1. Nhóm trang bị và phẩm chất

**Nguyên tắc xây chỉ số:** công của vũ khí, HP và phòng thủ của cả bộ phải được tính cùng nhau để hai nhân vật mặc trang bị tương đương vẫn hạ nhau trong khoảng 10 đòn trúng với nhịp chém thường xen kỹ năng hợp lệ. Kiểm tra từng bậc đồ thường 10/20/30/40/50 và từng mức cường hóa +0 đến +15; phải đủ cấp mặc các món đang xét. Trang bị chênh lệch được phép tạo khác biệt về số đòn chịu được. Bộ số bản đầu đã được duyệt; các lần điều chỉnh sau tiếp tục tuân theo [nguyên tắc cân bằng chiến đấu](combat-balance.md).

Bảng chỉ số cụ thể được chốt cho bản đầu tại [bảng chỉ số trang bị](equipment-stats-proposal.md), kèm CSV từng món và script kiểm tra. Kết quả 32.400 cấu hình tham chiếu cho 8–12 đòn trúng khi bên bị đánh là class vật lý, 7–12 đòn khi bên bị đánh là class phép; phạm vi hiện tại dùng kỹ năng đầu xen chém thường, chưa gồm các kỹ năng còn lại, dòng phụ/ẩn, ngọc, chí mạng, né và hồi phục. Giữ bộ số này để triển khai bản đầu và cân chỉnh tiếp sau chơi thử; các giới hạn của phép kiểm tra không phải quyết định thiết kế còn chờ duyệt.

**Chốt 10 ô chiến đấu**, theo thứ tự cấp mặc trong mỗi bậc:

| Ô | Cấp yêu cầu so với bậc | Ví dụ bậc 10 |
| --- | --- | --- |
| Vũ khí | +0 | 10 |
| Giày | +1 | 11 |
| Hộ uyển | +2 | 12 |
| Quần | +3 | 13 |
| Mũ | +4 | 14 |
| Áo | +5 | 15 |
| Dây chuyền | +6 | 16 |
| Ngọc bội | +7 | 17 |
| Nhẫn trái | +8 | 18 |
| Nhẫn phải | +9 | 19 |

Thời trang, cánh, thú cưỡi và trợ thủ có vị trí riêng, không tham gia liên kết của 10 ô chiến đấu.

| Nguồn | Bậc | Tên dòng |
| --- | --- | --- |
| Thường | 10 | Lữ Hành |
| Thường | 20 | Thanh Thiết |
| Thường | 30 | Tinh Cương |
| Thường | 40 | Huyền Cương |
| Thường | 50 | Xích Cương |

Bản v1.0.0.0 chỉ có năm dòng đồ thường trên; **đồ chế, chế tạo và độ bền để sau** (mục 11). Bậc, cấp yêu cầu, phẩm chất, hệ nguyên tố và mức cường hóa là các thuộc tính riêng. Đồ thường không có độ bền, không cần sửa.

Vũ khí **khóa đúng class**: Viêm — Viêm Nha Kiếm; Thủy — Thủy Nguyệt Kiếm; Minh — Lôi Ảnh Kiếm; Phong — Phong Vũ Kiếm; Nham — Nham Trấn Phủ–Chùy. Tên ghép **tên riêng + tên dòng + loại vũ khí**, ví dụ Viêm Nha Lữ Hành Kiếm. Chín món khác dùng loại món + tên dòng, ví dụ Áo Lữ Hành, Ngọc Bội Thanh Thiết.

| Phẩm chất | Thuộc tính và nguồn định hướng |
| --- | --- |
| Trắng — Thường | Chỉ số cơ bản |
| Xanh Dương — Ma thuật | Thêm 1–2 dòng phụ |
| Vàng — Hiếm | Thêm 3–4 dòng phụ, bắt đầu có hệ nguyên tố và dòng ẩn |
| Cam — Thần thoại | 4 dòng phụ, 2 dòng ẩn; trang bị cao cấp từ thủ lĩnh/Boss và sự kiện |
| Đỏ — Hoàng Kim | 4 dòng phụ, 2 dòng ẩn; ngân sách thuộc tính cao hơn Cam, từ hoạt động lớn/sự kiện |

So sức mạnh theo cả bậc và phẩm chất; đồ Đỏ bậc thấp không mặc định hơn đồ Cam bậc cao. Các chỉ số cơ bản và cường hóa dùng bảng dữ liệu bản đầu; phần phụ/ẩn bổ sung lên bảng đó.

### 3.2. Tương sinh trang bị — đã chốt

Dùng năm hệ của game (Viêm, Thủy, Minh, Nham, Phong), không theo ngũ hành Kim–Mộc–Thủy–Hỏa–Thổ:

**Thủy → Phong → Viêm → Nham → Minh → Thủy.**

Mũi tên A → B nghĩa là món hệ A kích hoạt dòng ẩn của món hệ B.

| Hệ món cần kích hoạt | Hệ món kích hoạt |
| --- | --- |
| Viêm | Phong |
| Nham | Viêm |
| Minh | Nham |
| Thủy | Minh |
| Phong | Thủy |

- Dòng hiện có hiệu lực khi mặc trang bị.
- Mỗi món thuộc cơ chế này có hai dòng ẩn, tương ứng hai ô kích hoạt cố định. Mỗi ô đúng hệ mở một dòng; đủ hai ô mở cả hai.
- Hệ trang bị độc lập với class và loại sát thương. Ví dụ vũ khí hệ Thủy có thể mang công vật lý và được class Viêm sử dụng.
- Tương sinh chỉ mở dòng ẩn, không tự tạo quan hệ khắc chế sát thương giữa các class.

Ví dụ kiếm hệ Viêm có hai ô kích hoạt là Mũ và Áo: Mũ Phong mở dòng 1, Áo Phong mở dòng 2. Chỉ có một ô đúng hệ vẫn mở được dòng tương ứng.

**Không thể mở đủ 20 dòng ẩn cùng lúc, đây là chủ đích.** Bảng ô kích hoạt có vòng ba món, ví dụ Vũ khí ← Mũ ← Dây chuyền ← Vũ khí, trong khi chu kỳ tương sinh dài năm hệ. Cách gán hệ tốt nhất mở được tối đa **13/20 dòng**. Người chơi tự chọn dòng nào cần kích hoạt theo chiến thuật và build đồ.

| Món nhận dòng ẩn | Ô kích hoạt dòng 1 | Ô kích hoạt dòng 2 |
| --- | --- | --- |
| Vũ khí | Mũ | Áo |
| Mũ | Dây chuyền | Nhẫn trái |
| Áo | Dây chuyền | Nhẫn phải |
| Quần | Áo | Ngọc bội |
| Hộ uyển | Vũ khí | Nhẫn trái |
| Giày | Quần | Ngọc bội |
| Dây chuyền | Vũ khí | Hộ uyển |
| Ngọc bội | Mũ | Giày |
| Nhẫn trái | Quần | Nhẫn phải |
| Nhẫn phải | Giày | Hộ uyển |

Giá trị mỗi dòng ẩn so với một dòng phụ cùng loại/cùng bậc: **Vàng 100%, Cam 125%, Đỏ 150%**. Ví dụ dòng phụ chuẩn 100 HP thì dòng ẩn tương ứng là 100/125/150 HP. **Bản đầu không có kích hoạt theo bộ riêng**, chỉ dùng liên kết tương sinh.

### 3.3. Ba tính năng rèn đúc

| Tính năng | Thiết kế hiện tại |
| --- | --- |
| Cường hóa | +1 đến +15, tiêu hao đá và Xu; +1 đến +4 thành công 100%; từ +5 tỷ lệ giảm, thất bại có thể tụt cấp; bùa bảo vệ chống tụt cấp; mỗi lần thất bại cộng ẩn +1% tỷ lệ cho lần sau; mốc +7/+10/+12/+15 kích hoạt hồi HP/MP theo mục 2.7 |
| Khảm ngọc | Đục tối đa ba lỗ để khảm ngọc công, HP, xuyên giáp, kháng choáng… |
| Tẩy luyện | Dùng đá tẩy và Xu để làm mới dòng phụ trên đồ Vàng/Cam/Đỏ |

Vũ khí phát sáng ở +7/+10/+12/+15, lần lượt Xanh/Vàng/Tím/Đỏ, có hiệu ứng hạt/tia lửa. Chỉ số công/thủ và HP cường hóa dùng [bảng dữ liệu đã duyệt](equipment-stats-proposal.md): công/thủ theo thang tăng dần đến khoảng 4,3 lần ở +15; HP theo bảng riêng, có thể bổ sung HP cho món +0 không có HP. Không nhân dòng phụ, dòng ẩn hoặc ngọc theo thang này.

#### Chỉ số cộng thêm sau cường hóa

Các bảng dưới đây là bộ số bản đầu đã duyệt. **Cộng thêm** là phần tăng so với chính món/bộ đó ở +0; **tổng** là chỉ số trang bị sau cường hóa, chưa cộng nền nhân vật, tiềm năng hoặc bị động. Không cộng lặp phần +0 vào phần tổng.

**Công và thủ:** thang phần trăm tính trên ngân sách chỉ số cơ bản +0, không nhân lũy tiến từ cấp trước. Phân bổ theo ô, làm tròn và bảo đảm không có chỉ số từng món giảm khi nâng thành công. Không dùng thang này nhân dòng phụ, dòng ẩn hoặc ngọc.

| Mức | Hệ số công/thủ so với +0 | Tổng cộng thêm so với gốc | Bước tăng so với mức trước, tính trên gốc |
| --- | --- | --- | --- |
| +0 | 1,00 lần | +0% | — |
| +1 | 1,03 lần | +3% | +3% |
| +2 | 1,07 lần | +7% | +4% |
| +3 | 1,12 lần | +12% | +5% |
| +4 | 1,18 lần | +18% | +6% |
| +5 | 1,26 lần | +26% | +8% |
| +6 | 1,36 lần | +36% | +10% |
| +7 | 1,49 lần | +49% | +13% |
| +8 | 1,65 lần | +65% | +16% |
| +9 | 1,85 lần | +85% | +20% |
| +10 | 2,10 lần | +110% | +25% |
| +11 | 2,40 lần | +140% | +30% |
| +12 | 2,75 lần | +175% | +35% |
| +13 | 3,15 lần | +215% | +40% |
| +14 | 3,65 lần | +265% | +50% |
| +15 | 4,30 lần | +330% | +65% |

Ví dụ mức +14 có ngân sách 365% gốc, +15 có 430%: nâng thành công tăng thêm **65% gốc**, tương đương khoảng **17,8% so với mức +14**. Vì làm tròn theo từng món, giá trị nguyên thực tế lấy theo bảng chỉ số, không làm tròn lại nhiều lần.

**Mốc chỉ số +0 của đủ 10 món:** Công là lượng tăng chỉ số tấn công chính; mỗi loại thủ tăng cùng giá trị trong cột cuối.

| Dòng | Bậc | Công +0 | HP +0 | Mỗi loại thủ +0 |
| --- | --- | --- | --- | --- |
| Lữ Hành (thường) | 10 | 100 | 2.152 | 15 |
| Thanh Thiết (thường) | 20 | 200 | 3.896 | 30 |
| Tinh Cương (thường) | 30 | 350 | 5.505 | 53 |
| Huyền Cương (thường) | 40 | 550 | 7.992 | 83 |
| Xích Cương (thường) | 50 | 800 | 11.731 | 120 |

**HP cộng thêm của đủ 10 món so với cùng bộ +0:** mỗi món dùng bảng HP riêng, không nhân hệ số công/thủ. Các số sau là tổng phần HP tăng do cường hóa của cả bộ; cộng với HP +0 ở bảng trên để ra tổng HP trang bị.

| Mức | Lữ Hành 10 | Thanh Thiết 20 | Tinh Cương 30 | Huyền Cương 40 | Xích Cương 50 |
| --- | --- | --- | --- | --- | --- |
| +0 | +0 | +0 | +0 | +0 | +0 |
| +1 | +33 | +65 | +84 | +140 | +202 |
| +2 | +73 | +133 | +216 | +332 | +467 |
| +3 | +130 | +232 | +371 | +569 | +793 |
| +4 | +179 | +344 | +554 | +845 | +1.189 |
| +5 | +260 | +499 | +794 | +1.210 | +1.709 |
| +6 | +350 | +684 | +1.084 | +1.667 | +2.352 |
| +7 | +471 | +923 | +1.470 | +2.242 | +3.190 |
| +8 | +626 | +1.214 | +1.930 | +2.952 | +4.189 |
| +9 | +811 | +1.572 | +2.495 | +3.814 | +5.431 |
| +10 | +1.037 | +2.013 | +3.204 | +4.871 | +6.953 |
| +11 | +1.305 | +2.533 | +4.024 | +6.099 | +8.741 |
| +12 | +1.619 | +3.141 | +4.955 | +7.479 | +10.768 |
| +13 | +1.969 | +3.805 | +5.985 | +9.007 | +13.012 |
| +14 | +2.397 | +4.617 | +7.218 | +10.833 | +15.720 |
| +15 | +2.932 | +5.622 | +8.767 | +13.073 | +19.083 |

**Ví dụ từng món Lữ Hành:** vũ khí dưới đây là Viêm Nha Lữ Hành Kiếm của Viêm; áo dùng chung class. Các cột đều là phần cộng thêm so với +0, không phải tổng sau cường hóa.

| Mức | Công vật lý vũ khí tăng | HP vũ khí tăng | Công mỗi loại của áo tăng | HP áo tăng | Mỗi loại thủ áo tăng |
| --- | --- | --- | --- | --- | --- |
| +0 | +0 | +0 | +0 | +0 | +0 |
| +1 | +1 | +0 | +1 | +10 | +0 |
| +2 | +2 | +0 | +1 | +14 | +0 |
| +3 | +3 | +0 | +2 | +20 | +0 |
| +4 | +4 | +0 | +2 | +20 | +1 |
| +5 | +6 | +0 | +4 | +34 | +1 |
| +6 | +9 | +2 | +5 | +42 | +2 |
| +7 | +12 | +22 | +7 | +49 | +3 |
| +8 | +16 | +48 | +10 | +66 | +4 |
| +9 | +21 | +88 | +13 | +91 | +5 |
| +10 | +28 | +140 | +17 | +117 | +6 |
| +11 | +35 | +196 | +21 | +138 | +8 |
| +12 | +44 | +262 | +26 | +162 | +11 |
| +13 | +54 | +337 | +32 | +200 | +13 |
| +14 | +66 | +424 | +40 | +240 | +16 |
| +15 | +83 | +553 | +50 | +280 | +20 |

Ví dụ vũ khí +0 có **25 công/0 HP**, +15 cộng **83 công/553 HP**, thành **108 công/553 HP**. HP bổ sung khi cường hóa là chỉ số cố định của món, không phải một dòng phụ và không thay đổi theo đối thủ.

Chỉ số nguyên từng món của cả năm dòng đồ thường ở mọi mức +0 đến +15 được lưu trong [CSV chỉ số trang bị](balance/equipment-stats-proposal.csv). Các bảng cộng thêm trên được tổng hợp từ chính CSV này để tránh dùng hai bộ số khác nhau.

| Cấp đích | Thành công | Thất bại không có bùa |
| --- | --- | --- |
| +1 đến +4 | 100% | Không có |
| +5 | 90% | Giữ cấp |
| +6 | 80% | Giữ cấp |
| +7 | 70% | Giữ cấp |
| +8 | 60% | Giảm 1 cấp |
| +9 | 50% | Giảm 1 cấp |
| +10 | 40% | Giảm 1 cấp |
| +11 | 30% | Giảm 1 cấp |
| +12 | 25% | Giảm 1 cấp |
| +13 | 20% | Giảm 1 cấp |
| +14 | 15% | Giảm 1 cấp |
| +15 | 10% | Giảm 1 cấp |

Bùa bảo vệ chống tụt cấp, không tăng tỷ lệ thành công. Thất bại vẫn tiêu Xu, đá và bùa đã sử dụng; không phá hủy trang bị.

**Tỷ lệ ẩn tích lũy khi thất bại:** kết quả vẫn ngẫu nhiên theo từng lần.
- Mỗi lần thất bại cộng **+1 điểm phần trăm** vào tỷ lệ thành công thật của lần thử sau, tích lũy đến khi tỷ lệ thật đạt **100%**. Thất bại càng nhiều thì tỷ lệ thành công càng cao. Ví dụ lên +15 có tỷ lệ gốc 10%: sau 90 lần thất bại ở cấp đích này, lần thử tiếp theo chắc chắn thành công.
- Mô phỏng từ +0 lên +15 không dùng bùa: trung bình khoảng **6.500 lượt thử**, so với khoảng 18.800 lượt nếu không có tỷ lệ ẩn. Có dùng bùa bảo vệ thì cần ít lượt hơn nhiều.
- Phần cộng lưu **riêng cho từng món và từng cấp đích**. Ví dụ phần cộng của lần lên +15 không dùng cho lần lên +14.
- **Giữ phần cộng khi tụt cấp**; chỉ về 0 khi lên thành công cấp đích đó. Nếu xóa khi tụt cấp thì từ +8 trở lên cơ chế gần như không có tác dụng, vì mỗi lần thất bại đều tụt cấp.
- Có hoặc không dùng bùa bảo vệ, phần cộng vẫn tích lũy như nhau.
- Số đá, Xu và bùa tiêu hao mỗi lần không đổi.
- Giao diện vẫn hiển thị **tỷ lệ gốc** theo bảng trên; phần cộng ẩn không hiển thị.

Tẩy luyện làm mới loại và giá trị dòng phụ. Có thể khóa từng dòng muốn giữ; càng khóa nhiều càng tốn đá tẩy và Xu. Xem kết quả rồi chọn giữ bộ cũ hoặc nhận bộ mới, đá tẩy và Xu vẫn tiêu hao. Không đổi hệ, phẩm chất, chỉ số cơ bản, cường hóa, ngọc hoặc dòng ẩn.

## 4. NPC và nhiệm vụ

### 4.1. NPC

NPC chủ yếu tập trung ở làng, thành và doanh trại an toàn; có biểu tượng chức năng:

- Thợ rèn: cường hóa, khảm ngọc, tẩy luyện.
- Tạp hóa/dược sư: bình HP/MP, giấy về thành, bùa hỗ trợ.
- Thủ kho: kho chung tài khoản và mở rộng kho.
- Xa phu: di chuyển giữa các bản đồ với phí.
- NPC giao nhiệm vụ/hướng dẫn: trưởng làng, chỉ huy, sứ giả môn phái…; không bắt người chơi diễn lại cốt truyện nguyên tác.
- NPC hoạt động: bang hội, lôi đài, sự kiện, thương nhân theo giờ.

Biểu tượng nhiệm vụ: dấu ! vàng khi có nhiệm vụ mới; ? xám khi đang làm; ? vàng khi đủ điều kiện trả. Thứ tự ưu tiên biểu tượng khi có nhiều nhiệm vụ chưa chốt.

### 4.2. Tuyến và mục tiêu nhiệm vụ

| Tuyến | Vai trò |
| --- | --- |
| Chính tuyến/hướng dẫn phát triển nhân vật | Hướng dẫn tính năng, hỗ trợ luyện cấp, thưởng EXP, trang bị tân thủ và sách học kỹ năng tương ứng các mốc cấp; không dùng để khóa map hay bắt đi theo cốt truyện nguyên tác |
| Phụ tuyến | Khám phá map, nhận Xu, đá cường hóa, danh vọng |
| Hàng ngày | Làm mới 00:00; hoạt động diệt quái, phó bản, nâng cấp… và điểm năng động |
| Bang hội/tuần hoàn | Chuỗi hoạt động như áp tiêu, thu thập; thưởng cống hiến; dự kiến 10–20 vòng/ngày |

Mẫu mục tiêu: diệt quái, thu thập vật phẩm, hái/đào tài nguyên, đưa thư/trò chuyện, săn Boss/vượt ải và hộ tống/áp tiêu. Vật phẩm nhiệm vụ có thể dùng kho riêng, không chiếm hành trang chính. **Không chia sẻ tiến độ nhiệm vụ qua tổ đội**; nhận EXP chia sẻ không tự cộng tiến độ nhiệm vụ. Múi giờ reset chưa chốt.

Nhiệm vụ phục vụ hướng dẫn, luyện cấp và kiếm tài nguyên; không yêu cầu người chơi trải qua hành trình của Tanjiro. Cấp quái thể hiện độ khó và phân bố bãi, không phải điều kiện vượt map.

### 4.3. Tự tìm đường

- Chạm nhiệm vụ để đi đến mục tiêu của bước hiện tại.
- Trong map dùng dữ liệu va chạm và tìm đường A*; giữa các map tìm chuỗi cổng phù hợp để đi đến map đích.
- Đích có thông tin map và vị trí/vùng mục tiêu.
- **Nhiệm vụ diệt quái chỉ đưa nhân vật tới bãi rồi dừng, không tự chọn quái hay bật auto đánh.**

## 5. Bản đồ, sinh quái và PK

### 5.1. Loại bản đồ

| Loại | Chức năng và luật |
| --- | --- |
| Làng/thành | Cấm PK tuyệt đối; NPC dịch vụ, nhiệm vụ và sạp giao dịch |
| Luyện cấp | Chia theo cấp, nhiều bãi quái, PK theo trạng thái; có nhiều khu/kênh |
| Chiến sự/tranh chấp | Boss, tài nguyên quý, áp tiêu; PK theo luật hoạt động; không phân khu; chết do PK không mất EXP |
| Phó bản/lôi đài | Bản đồ riêng theo nhóm hoặc trận; luật PK tùy hoạt động |

**Đang giao chiến ở map luyện cấp vẫn được đổi khu hoặc đi qua cổng sang map khác.** Không yêu cầu ngừng gây/nhận sát thương trước khi đổi khu.

**Map chiến sự không có khu để đổi; người chơi rời map hoặc chết để thoát giao tranh.**

### 5.2. Các lớp bản đồ

1. Ground: nền cỏ, đất, gạch, nước…
2. Collision: ô đi được/bị chặn cho tường, vách núi, sông sâu, gốc cây…; phục vụ di chuyển và A*.
3. Foreground/depth: mái nhà, tán cây, cổng; Y-sorting để tạo trước/sau.
4. Trigger/spawn: vị trí NPC, điểm hồi sinh, cổng chuyển map và bãi sinh quái.

Hình ảnh nền không tự quyết định va chạm. Xếp lớp cần dựa vào chân nhân vật và điểm gốc vật thể.

### 5.3. Sinh quái và phân kênh

- Bãi quái có tâm và bán kính, số lượng quái và thời gian hồi sinh.
- Quái thường hồi sinh sau **8 giây từ lúc bị tiêu diệt**, tại vị trí đi được trong bãi.
- **Tinh anh hồi sinh 5 phút, thủ lĩnh hồi sinh 10 phút tính từ lúc bị tiêu diệt**. Mỗi khu có quái và bộ đếm riêng; không dùng quy tắc này thay luật Boss của sự kiện.
- Quái phát hiện người trong **4 ô**, truy đuổi tối đa **10 ô từ điểm sinh**; vượt giới hạn thì bỏ mục tiêu, về gốc và hồi đầy HP. Không đổi mục tiêu chỉ vì có người đi ngang; mục tiêu chết/rời map thì ưu tiên người gần nhất đang tấn công mình.
- Map luyện cấp có nhiều khu như Khu 1/2/3; mốc dự kiến 30–50 người/khu, chưa kiểm chứng năng lực kỹ thuật.
- Kênh công cộng khác với phó bản dành riêng cho một nhóm/trận.

### 5.4. Các địa điểm dịch vụ và khu an toàn

| Địa điểm | Dịch vụ |
| --- | --- |
| Tổng hành dinh gia tộc Ubuyashiki | Đầy đủ NPC nhiệm vụ, tạp hóa, kho, thợ rèn, xa phu, bang hội, sự kiện; sạp giao dịch |
| Điệp Phủ | Bán bình, hồi phục, kho, nhiệm vụ, huấn luyện |
| Làng Thợ Rèn | Cường hóa, khảm ngọc, tẩy luyện, kho |
| Nhà Hoa Tử Đằng | Điểm hồi sinh, bán bình, kho, xa phu |

Cả bốn loại là **khu an toàn, không quái và cấm PK tuyệt đối**. Tại NPC nghỉ ngơi, đứng yên **5 giây hồi đầy HP/MP miễn phí**; mua hàng và dịch chuyển dùng Xu. Phòng Thủ Làng Thợ Rèn là bản đồ sự kiện riêng, không biến bản đồ sinh hoạt thành chiến trường.

### 5.5. Phân bố quái và cấp độ theo map

Khung cấp **1–90** được giữ làm cơ sở thiết kế ban đầu, chưa phải giới hạn cấp cuối game. Bảng sắp theo độ khó để dễ phân bố nội dung, **không phải tuyến bắt buộc đi tuần tự**. Mỗi địa danh lớn có thể chia thành nhiều map nhỏ/bãi luyện.

Cấp và danh sách dưới đây đã chốt cho bản đầu, cân chỉnh tiếp qua chơi thử. Quái có dấu (*) là tên/biến thể sáng tạo cho game, không khẳng định là nhân vật có trong nguyên tác. Các tên có sẵn được dùng làm hình ảnh và chủ đề gameplay, không bắt buộc tái hiện hoàn cảnh xuất hiện trong truyện.

| Vùng / map nhỏ | Cấp quái thường | Quái thường | Tinh anh / thủ lĩnh |
| --- | --- | --- | --- |
| Kumotori – Đường núi tuyết | 1–5 | Quỷ lang thang, quỷ đói trong tuyết (*) | Quỷ lang thang đầu đàn (*) — cấp 6 |
| Sagiri – Chân núi, đường đến ngôi đền | 6–10 | Quỷ rừng núi, quỷ phục kích (*) | Quỷ Đền — cấp 10 |
| Sagiri – Rừng sương mù | 11–15 | Quỷ ẩn sương, quỷ vuốt dài (*) | Quỷ săn đêm (*) — cấp 15 |
| Fujikasane – Rừng tuyển chọn | 16–20 | Quỷ Rừng, các quỷ bị giam trên núi (*) | Quỷ Rừng tinh anh (*) — cấp 20 |
| Fujikasane – Vùng sâu | 21–25 | Quỷ thân lớn, quỷ nhiều tay cấp thấp (*) | Quỷ Tay — cấp 25 |
| Thị trấn Tây Bắc – Ngoại ô, đầm lầy | 26–30 | Quỷ ven đầm, quỷ phục kích dưới bùn (*) | Quỷ Đầm Lầy — cấp 30 |
| Asakusa – Ngõ tối, khu ngoại vi | 31–35 | Quỷ mới biến đổi, quỷ lang thang đô thị (*) | Quỷ Asakusa — cấp 35 |
| Asakusa – Khu chiến đấu | 36–40 | Quỷ tay sai (*) | Yahaba — cấp 39; Susamaru — cấp 40 |
| Dinh thự Trống – Rừng ngoài dinh | 41–45 | Quỷ rừng, quỷ móng dài (*) | Quỷ Sừng — cấp 45 |
| Dinh thự Trống – Hành lang, phòng trống | 46–50 | Quỷ trú trong dinh (*) | Quỷ Lưỡi — cấp 48; Kyogai — cấp 50 |
| Natagumo – Bìa rừng | 51–55 | Nhện quỷ nhỏ, con rối bị tơ điều khiển (*) | Quỷ Không Đầu — cấp 55 |
| Natagumo – Rừng tơ nhện | 56–60 | Nhện quỷ, quái kén tơ (*) | Quỷ Nhện Anh — cấp 58; Quỷ Nhện Chị/Em Gái — cấp 60 |
| Natagumo – Vùng lõi | 61–65 | Quái tơ nhện, con rối tinh anh (*) | Quỷ Nhện Mẹ — cấp 62; Quỷ Nhện Bố — cấp 64; Rui — cấp 65 |
| Chuyến Tàu Vô Tận | 66–75 | Xúc tu, mô thịt, quái ác mộng (*) | Enmu — cấp 75 |
| Yoshiwara – Ngõ đêm, đường ngầm | 76–85 | Quỷ ẩn phố đêm, phân thể dải lụa (*) | Dải lụa tinh anh (*) — cấp 85 |
| Yoshiwara – Hầm Ngầm | 86–90 | Quỷ Mặt Nạ 86, Quỷ Trảo Dài 87, Quỷ Huyết Nha 88, Quỷ Tàng Ảnh 89, Quỷ Lân Xà 90 (*) | Quỷ Rắn — cấp 90; săn tự do, không chặn đường qua map |

- Tinh anh/thủ lĩnh trong map luyện cấp dùng quy tắc hồi sinh 5 hoặc 10 phút từ lúc bị hạ, không phải mục tiêu vượt ải một lần để mở vùng tiếp theo.
- **Tất cả Boss Thượng Huyền chỉ xuất hiện trong sự kiện.** Akaza không có ở bản đồ Chuyến Tàu Vô Tận thường; Daki và Gyutaro không có ở bản đồ luyện cấp Yoshiwara. Không thay bằng Boss kết vùng bắt buộc.
- Enmu là thủ lĩnh cấp 75 ở bản đồ thường, hồi sinh 10 phút; bản sự kiện có cấu hình riêng tại mục 5.13. Không yêu cầu hạ Enmu để đi map khác.
- Quỷ Rắn đặt ở Yoshiwara — Hầm Ngầm, cấp 90, thủ lĩnh hồi sinh 10 phút. Dùng hình ảnh nhân vật làm gameplay, không tạo phó bản hồi ức bắt buộc.
- Chỉ số, kỹ năng, mật độ và rơi đồ dùng bộ số bản đầu tại các mục 5.9–5.13; không mặc định tái hiện mọi khả năng nguyên tác.

### 5.6. Phân biệt map thường và bản đồ sự kiện

Map thường cho phép đi lại tự do và quái hồi sinh theo loại. Cùng một nhân vật quỷ có thể được cấu hình riêng cho map thường và sự kiện; cấp, HP và phần thưởng không mặc định giống nhau.

Quy tắc **mở map theo tiến trình** và **Hạ Huyền/Thượng Huyền chỉ xuất hiện một lần, chết không hồi sinh** áp dụng riêng cho mỗi kỳ Công Phá Vô Hạn Thành, không áp dụng cho toàn bộ thế giới luyện cấp.

### 5.7. Chế độ PK và phạt EXP khi chết

- Bật chế độ PK để tấn công người chơi khác tại map cho phép PK. Không cần điều kiện ác danh; vùng an toàn vẫn cấm PK tuyệt đối.
- **Người bật PK đánh được tất cả người chơi**, kể cả người đang tắt PK. **Người đang bật PK thì ai cũng đánh được**, kể cả người đang tắt PK. Hai người cùng tắt PK không thể tấn công nhau.

| Người tấn công | Mục tiêu bật PK | Mục tiêu tắt PK |
| --- | --- | --- |
| Bật PK | Được | Được |
| Tắt PK | Được | Không |

- Bộ lọc mục tiêu tại mục 2.5, kỹ năng nhiều mục tiêu và auto-play dùng chung bảng trên. Người tắt PK bị người bật PK tấn công có thể phản công ngay mà không cần bật PK.
- Chỉ tại **map luyện cấp**, nhân vật **đang bật PK bị người chơi khác giết** mất **5% EXP cần để lên cấp kế tiếp ở cấp hiện tại**, không phải 5% số EXP đang tích lũy.
- Công thức: **EXP mất = floor(0,05 × EXP yêu cầu của cấp hiện tại)**, lấy theo bảng EXP lên cấp tại mục 2.2; **EXP sau chết = EXP đang có − EXP mất**. Làm tròn xuống số EXP bị mất, không làm tròn số EXP còn lại. Mỗi lần chết tính lại cùng quy tắc; EXP được phép âm, không chặn về 0.
- Ví dụ cấp 10 lên 11 cần 64.000 EXP: mỗi lần chết theo điều kiện trên mất floor(64.000 × 5%) = 3.200 EXP; đang có 5.000 về 1.800, chết tiếp về −1.400. Ví dụ cấp 1 lên 2 cần 130 EXP thì mức phạt là floor(130 × 5%) = 6 EXP.
- Không có lính canh trừng phạt người bật PK. Vào làng/thành vẫn được, chỉ không thể tấn công vì khu an toàn cấm PK tuyệt đối.
- **Map chiến sự/tranh chấp, phó bản/lôi đài và sự kiện không mất EXP do PK**, kể cả khi map chiến sự không có sự kiện nào đang diễn ra. Không áp mức phạt trên cho chết do quái hoặc người đang tắt PK.
- Đổi map/khu khi giao chiến vẫn theo luật mục 5.1; bật PK không khóa quyền đổi khu ở map luyện cấp.

#### Hồi sinh khi chết

Khi chết, người chơi chọn một trong hai cách:

| Cách | Chi phí | Vị trí | Trạng thái sau hồi sinh |
| --- | --- | --- | --- |
| Hồi sinh tại chỗ | **1 Lượng** | Ngay nơi chết | Đầy HP/MP |
| Về làng | Miễn phí | Điểm hồi sinh ở khu an toàn gần nhất của map đó (làng, thành hoặc Nhà Hoa Tử Đằng) | Đầy HP/MP |

- Nếu không chọn trong **60 giây**, nhân vật tự về làng.
- Mất EXP do PK (nếu có) tính lúc chết, không phụ thuộc cách hồi sinh. Hồi sinh tại chỗ không hoàn lại EXP đã mất.
- Không đủ 1 Lượng thì chỉ còn lựa chọn về làng.
- Phó bản, lôi đài và sự kiện có thể có luật hồi sinh riêng của hoạt động. Mặc định áp dụng bảng trên nếu hoạt động không quy định khác.

### 5.8. EXP tổ đội và quyền nhặt đồ

- **Tổ đội tối đa 5 người.**
- Người hạ quái nhận EXP gốc của quái. Mỗi đồng đội trong tổ đội đủ điều kiện bên dưới nhận **thêm 5% EXP gốc**, không khấu trừ từ EXP người hạ quái.
- Ví dụ quái cho 100 EXP, tổ đội năm người: người hạ nhận **100 EXP**, bốn đồng đội mỗi người nhận **5 EXP**, tổng phát ra 120 EXP.
- Đây là chủ đích: ai trong tổ đội hạ quái thì các thành viên còn lại đều nhận 5%. Nếu cả năm người cùng tự đánh quái với tốc độ như nhau, mỗi người nhận thêm khoảng **20%** so với EXP tự đánh (4 đồng đội × 5%).
- Điều kiện nhận EXP chia sẻ: **cùng map, cùng khu** với người hạ quái; không giới hạn khoảng cách và không yêu cầu đồng đội phải trực tiếp gây sát thương.
- **Không chia sẻ tiến độ nhiệm vụ** cho đồng đội. Nhận 5% EXP không đồng nghĩa được cộng số quái hoặc mục tiêu nhiệm vụ.
- **Đồ rơi thuộc người kết liễu**, kể cả khi đang ở tổ đội. Không chia quyền nhặt theo tổng sát thương hoặc theo người nhận EXP chia sẻ.
- Quyền nhặt đồ tách biệt với điểm cá nhân/bang trong sự kiện và phần thưởng trao qua bảng tổng kết.
### 5.9. Chỉ số quái — bộ số bản đầu

Mốc tính là nhân vật cùng cấp, 2/3 điểm vào thuộc tính sát thương chính (làm tròn xuống), còn lại VIT, mặc trang bị thường +0 tốt nhất theo cấp (v1.0.0.0 không có đồ chế). Bỏ dòng phụ/ẩn, ngọc, thú cưỡi, bị động, chí mạng, né và hồi phục khi dựng mốc. Dữ liệu quái được tính ngoại tuyến thành số cố định, **không tự tăng theo người đang đánh**.

| Loại | HP so với thường cùng cấp | Công so với thường | Mỗi loại thủ so với thường | Đánh thường | Hồi sinh sau chết |
| --- | --- | --- | --- | --- | --- |
| Thường | 1 lần, khoảng 4 đòn tham chiếu | 1 lần, một đòn trúng khoảng 4% HP nhân vật tham chiếu | 1 lần | Mỗi 1,5 giây | 8 giây |
| Tinh anh | 6 lần | 1,5 lần | 1,25 lần | Mỗi 1,2 giây | 5 phút |
| Thủ lĩnh | 20 lần | 2 lần | 1,5 lần | Mỗi 1,2 giây | 10 phút |

Quái thường có mỗi loại thủ bằng **8 × cấp quái**. HP thường bằng tổng sát thương thực nhận của hai chém mặc định và hai lần dùng kỹ năng I (Bất Tri Hỏa) ở cấp kỹ năng hợp lệ cao nhất của nhân vật Viêm tham chiếu; trước cấp nhân vật 10 dùng bốn chém mặc định. Công thường làm tròn lên sao cho đòn 100% công sau thủ nhân vật tham chiếu gây xấp xỉ 4% HP. Các phép tính dùng công thức phòng thủ tại mục 2.2; không dùng hệ số giảm PvP riêng cho PvE. HP/công/thủ loại cao hơn nhân theo bảng, làm tròn lên khi có phần lẻ. Class phép có HP thấp hơn mốc Viêm nên mỗi đòn quái lấy tỷ lệ HP cao hơn; ví dụ ở cấp 10, đòn 66 sát thương bằng khoảng 4,6% của 1.430 HP.

| Cấp | HP thường | Công thường | Mỗi loại thủ thường | HP tinh anh | HP thủ lĩnh |
| --- | --- | --- | --- | --- | --- |
| 10 | 750 | 95 | 80 | 4.500 | 15.000 |
| 20 | 2.704 | 319 | 160 | 16.224 | 54.080 |
| 30 | 4.338 | 508 | 240 | 26.028 | 86.760 |
| 40 | 6.240 | 694 | 320 | 37.440 | 124.800 |
| 50 | 8.408 | 934 | 400 | 50.448 | 168.160 |
| 60 | 10.452 | 1.211 | 480 | 62.712 | 209.040 |
| 70 | 11.466 | 1.276 | 560 | 68.796 | 229.320 |
| 80 | 12.466 | 1.344 | 640 | 74.796 | 249.320 |
| 90 | 13.478 | 1.412 | 720 | 80.868 | 269.560 |

Đánh thường của quái dùng 100% công tương ứng với vật lý/phép. Hiệu ứng và nhịp đánh riêng theo bảng kỹ năng dưới đây. Mục tiêu khoảng 10 đòn PvP không áp lên quái hoặc thủ lĩnh.

### 5.10. Tinh anh/thủ lĩnh và kỹ năng đặc trưng

| Quái | Cấp | Loại / hồi sinh | Kỹ năng hoặc nội tại |
| --- | --- | --- | --- |
| Quỷ lang thang đầu đàn | 6 | Tinh anh / 5 phút | Vồ: 150% công vật lý, hồi 6 giây |
| Quỷ Đền | 10 | Thủ lĩnh / 10 phút | Hồi 10% HP tối đa một lần khi dưới 30% HP |
| Quỷ săn đêm | 15 | Tinh anh / 5 phút | Lao tới: 150% công vật lý, hồi 6 giây |
| Quỷ Rừng tinh anh | 20 | Tinh anh / 5 phút | Quét móng hình quạt: 130% công vật lý, hồi 6 giây |
| Quỷ Tay | 25 | Thủ lĩnh / 10 phút | Đập tay vùng tròn: 180% công vật lý, hồi 8 giây |
| Quỷ Đầm Lầy | 30 | Thủ lĩnh / 10 phút | Ba phân thân chung thanh HP; vùng đầm 150% công phép, hồi 8 giây |
| Quỷ Asakusa | 35 | Thủ lĩnh / 10 phút | Dưới 30% HP tăng 20% công |
| Yahaba | 39 | Thủ lĩnh / 10 phút | Mũi tên đẩy lùi 2 ô: 150% công phép, hồi 8 giây |
| Susamaru | 40 | Thủ lĩnh / 10 phút | Bóng theo đường thẳng: 180% công vật lý, hồi 6 giây |
| Quỷ Sừng | 45 | Tinh anh / 5 phút | Húc: 150% công vật lý, hồi 6 giây |
| Quỷ Lưỡi | 48 | Tinh anh / 5 phút | Quất lưỡi dài 4 ô: 150% công vật lý, hồi 6 giây |
| Kyogai | 50 | Thủ lĩnh / 10 phút | Sóng chém hình quạt: 180% công phép, hồi 8 giây |
| Quỷ Không Đầu | 55 | Tinh anh / 5 phút | Quét quanh người: 150% công vật lý, hồi 6 giây |
| Quỷ Nhện Anh | 58 | Thủ lĩnh / 10 phút | Phun độc hình quạt: 150% công phép, chậm 20% trong 3 giây, hồi 8 giây |
| Quỷ Nhện Chị/Em Gái | 60 | Thủ lĩnh / 10 phút | Kén axit tại mục tiêu: 180% công phép, hồi 8 giây |
| Quỷ Nhện Mẹ | 62 | Thủ lĩnh / 10 phút | Tơ trói: 120% công phép, giữ chân 1 giây, hồi 10 giây |
| Quỷ Nhện Bố | 64 | Thủ lĩnh / 10 phút | Lột xác một lần dưới 50% HP: tăng 20% công, không hồi đầy HP |
| Rui | 65 | Thủ lĩnh / 10 phút | Lưới tơ vùng tròn: 200% công phép, hồi 10 giây |
| Enmu | 75 | Thủ lĩnh / 10 phút | Sóng mộng: 150% công phép, ngủ 1 giây, hồi 10 giây |
| Dải lụa tinh anh | 85 | Tinh anh / 5 phút | Quất lụa đường thẳng: 150% công phép, hồi 6 giây |
| Quỷ Rắn | 90 | Thủ lĩnh / 10 phút | Phun độc hình quạt: 180% công phép, chậm 20% trong 3 giây, hồi 8 giây |

Chiêu vùng của tinh anh đánh tối đa **3 người**, thủ lĩnh tối đa **5 người**, cảnh báo vùng trước **0,8 giây**. Quỷ Đầm Lầy chỉ trao EXP và đồ một lần khi thanh HP chung về 0. Kỹ năng quái là chuyển thể gameplay, không khẳng định có nguyên trạng trong truyện.

### 5.11. Mật độ bãi và quái Yoshiwara

| Thành phần | Cấu hình |
| --- | --- |
| Map luyện cấp nhỏ | 4 bãi, mỗi bãi 12 quái thường |
| Map luyện cấp lớn | 6 bãi, mỗi bãi 15 quái thường |
| Bãi quái | Bán kính 5 ô, chỉ sinh ở vị trí đi được |
| Tinh anh | 1 con/bãi, điểm xuất hiện riêng |
| Thủ lĩnh | 1 con/khu vực thủ lĩnh, không chặn cổng qua map |

| Quái thường Yoshiwara — Hầm Ngầm | Cấp | Đặc điểm |
| --- | --- | --- |
| Quỷ Mặt Nạ | 86 | Cận chiến |
| Quỷ Trảo Dài | 87 | Cận chiến, tầm 2 ô |
| Quỷ Huyết Nha | 88 | Công +20%, HP −20% so với chuẩn cùng cấp |
| Quỷ Tàng Ảnh | 89 | Tốc độ chạy +20% so với quái thường |
| Quỷ Lân Xà | 90 | Phun độc tầm 4 ô, sát thương phép |

Đây là quái sáng tạo cho game. Quỷ Rắn cấp 90 là thủ lĩnh săn tự do trong cùng vùng; Daki/Gyutaro chỉ xuất hiện ở sự kiện.

### 5.12. EXP, Xu và rơi đồ

| Phần thưởng | Thường | Tinh anh | Thủ lĩnh |
| --- | --- | --- | --- |
| EXP | 10 × cấp quái | 6 lần thường | 25 lần thường |
| Xu | Ngẫu nhiên 3–5 × cấp quái | 6 lần thường | 25 lần thường |
| Trang bị | 5% rơi 1 món | Chắc chắn 1 món | Chắc chắn 2 món |
| Phẩm chất khi rơi trang bị | Trắng 80%, Xanh 17%, Vàng 3% | Xanh 75%, Vàng 25% | Xanh 20%, Vàng 75%, Cam 5% |
| Đá cường hóa | 20% rơi 1 | Chắc chắn 3 | Chắc chắn 10 |
| Đá tẩy luyện | Không | 10% rơi 1 | Chắc chắn 1 |

- Bậc đồ thường rơi theo chục cấp quái: **min(50, max(10, 10 × floor(cấp quái / 10)))**. Không thay cấp mặc theo từng ô; món rơi có thể dành để mặc ở cấp cao hơn.
- Đồ Đỏ dành cho hoạt động lớn/sự kiện, không nằm trong lượt rơi trang bị thường của bảng này.
- v1.0.0.0 không có nguyên liệu chế tạo (da, tơ, gỗ, quặng…); hai dòng nguyên liệu cũ được đổi thành đá cường hóa và đá tẩy luyện, giữ nguyên tỷ lệ rơi.
- Các lượt rơi trang bị/đá độc lập; tỷ lệ phẩm chất chỉ xét sau khi có trang bị rơi. Thủ lĩnh xét phẩm chất riêng cho từng món trong hai món rơi.
- Người kết liễu giữ quyền nhặt; đồng đội cùng map/khu nhận thêm 5% EXP như mục 5.8. Phần thưởng nhân vật/người chơi khác không làm nhân bản lượt rơi đồ của một con quái.

### 5.13. Chuyến Tàu Vô Tận thường và sự kiện

| Nội dung | Map thường | Bản sự kiện |
| --- | --- | --- |
| Cấu trúc | Map cấp 66–75, các toa nối bằng cổng cố định, tự do đi lại | Phó bản tổ đội tối đa 5 người, qua các đợt quái rồi đánh Enmu |
| Quái | Hồi sinh 8 giây | Theo đợt trong lượt sự kiện |
| Enmu | Cấp 75, thủ lĩnh hồi sinh 10 phút sau chết | Xuất hiện một lần/lượt; HP gấp 4 lần bản thường, công/thủ giữ nguyên |
| Thời hạn | Không có thời hạn lượt | 15 phút |
| Chuyển map | Không cần hạ Enmu để đi map khác | Theo tiến trình riêng của hoạt động |
| Phạt PK | Theo luật map luyện cấp | Không mất EXP do PK; bản tổ đội không PK |

Cả hai phiên bản đều **không có Akaza**. Bản thường dùng luật khu của map luyện cấp; bản sự kiện là instance riêng, không dùng bộ đếm hồi sinh thủ lĩnh thường.

## 6. Sự kiện

### 6.1. Các nhóm sự kiện tổng thể

- Định kỳ hàng ngày/tuần: hoạt động theo giờ, săn Boss, tổ đội, PK và bang hội.
- Lễ hội/theo mùa: dự kiến 1–2 tuần; đánh quái chênh không quá năm cấp để lấy nguyên liệu, ghép quà bằng nguyên liệu và Xu/vật phẩm đặc biệt. Mở quà nhận EXP, cơ hội vật phẩm hiếm và điểm tích lũy đổi thưởng. Công thức và danh sách thưởng chưa chốt.
- Máy chủ mới/đua top: cấp độ, lực chiến/tài phú, bang hội; mốc 7–14 ngày là dự kiến, chưa có quy tắc xếp hạng cuối cùng.

### 6.2. Danh sách sự kiện định kỳ được giữ lại

Đây là các hoạt động chuyển thể cho gameplay, không phải khẳng định mọi cơ chế đều có trong nguyên tác. Lịch và phần thưởng dưới đây là định hướng đã được giữ lại, chưa phải cấu hình cuối.

| Hoạt động | Nhịp dự kiến | Nội dung | Thưởng định hướng |
| --- | --- | --- | --- |
| Quạ Truyền Tin – Lệnh Diệt Quỷ | 2–3 khung/ngày | Nhận thông báo, diệt nhóm quỷ và quỷ đầu đàn ở map luyện cấp | EXP, Xu, đá cường hóa |
| Huấn Luyện Trụ Cột | Hàng ngày | Bài tập chiến đấu, né vùng nguy hiểm, phá chướng ngại, bảo vệ mục tiêu; phó bản cá nhân/tổ đội | Điểm huấn luyện, tài nguyên nhân vật |
| Chuyến Tàu Vô Tận | 1–2 khung/ngày | Tổ đội tối đa 5 người, 15 phút, vượt các đợt quái và hạ Enmu; không PK; theo mục 5.13 | EXP, trang bị, đá cường hóa |
| Thảo Phạt Thượng Huyền | 2–3 lần/ngày | Boss thế giới ở vùng tranh chấp, các bang vừa đánh Boss vừa PK; không phân khu | Trang bị hiếm, đá cường hóa, vật phẩm bang |
| Tiếp Vận Làng Thợ Rèn | Giờ vàng hàng ngày | Hộ tống vật tư/quặng qua vùng tranh chấp; có thể cướp hàng | Xu, đá cường hóa, cống hiến |
| Phòng Thủ Làng Thợ Rèn | Một số buổi/tuần | Bảo vệ thợ rèn và điểm rèn qua các đợt quỷ, kết thúc bằng Boss; không PK | Đá cường hóa, đá tẩy luyện, điểm đóng góp |
| Diễn Võ Sát Quỷ Đoàn | Thi đấu trong tuần | Lôi đài 1vs1 hoặc 3vs3; tổng kết, trao thưởng cuối tuần | Điểm/vật phẩm diễn võ và danh hiệu |
| Công Phá Vô Hạn Thành | Sự kiện bang hội cuối tuần | Tiến qua các map, tranh điểm và quyền chiếm thành | Xem mục riêng |

Top 1 Diễn Võ tại kỳ tổng kết tuần nhận danh hiệu **Trụ Cột vĩnh viễn**. Không cần giữ Top 1 ở tuần sau để giữ danh hiệu; nhiều người có thể cùng sở hữu. Cách áp dụng Top 1 cho thể thức cá nhân/tổ đội và trường hợp thắng lặp lại chưa chốt.

### 6.3. Công Phá Vô Hạn Thành

#### Tiến trình

Các bang cùng công phá chuỗi map mở lần lượt, có PK tranh mục tiêu. Không phân khu. Mỗi map có thời gian riêng; **10 phút là ví dụ**, chưa chốt tổng thời lượng.

| Thứ tự | Đối tượng | Xuất hiện/hồi sinh |
| --- | --- | --- |
| Đầu tiên | Quỷ Cấp Thấp | Nhiều, rải rác; chết hồi sinh như quái thường |
| Tiếp theo | Quỷ Dị Hình | Sát thương lớn hơn quỷ cấp thấp; chết hồi sinh |
| Tiếp theo | Hạ Huyền | Từng Boss chỉ xuất hiện một lần trong kỳ sự kiện; chết chuyển Boss hoặc mở map kế |
| Tiếp theo | Thượng Huyền | Mỗi Thượng Huyền một map; từng Boss chỉ xuất hiện một lần trong kỳ; chết mở map kế |
| Cuối cùng | Quỷ Vương | Hạ Boss là điều kiện để có bang chiếm thành |

**Hạ Huyền và Thượng Huyền đã chết không hồi sinh trong kỳ đó.** Không dùng cơ chế đếm giờ để sinh lại chính Boss đã bị hạ. Khoảng chờ Boss tiếp theo, thứ tự cụ thể và xử lý hết giờ khi Boss còn sống chưa chốt.

#### Điểm và quyền chiếm thành

- Người kết liễu quái nhận điểm cá nhân; đồng thời cộng điểm bang của người đó.
- Có bảng điểm cá nhân và bảng điểm bang hội.
- Mức điểm từng quái/Boss chưa chốt.
- Kết thúc sự kiện, bang Top 1 tổng điểm chỉ chiếm Vô Hạn Thành khi Quỷ Vương đã bị hạ.
- Nếu không hạ được Quỷ Vương, Top 1 chỉ nhận thưởng bang, không giành được thành.
- Quyền chiếm thành xét Top 1 tổng điểm bang, không mặc định giao cho bang kết liễu Quỷ Vương.
- Chưa chốt điểm khi hạ người chơi, đồng điểm, rời/đổi bang và điều kiện thành viên hợp lệ.

Không mặc định có cơ chế đứng giữ phòng để tích điểm: phần chiếm thành hiện được xác định bằng kết quả cuối sự kiện.

#### Phần thưởng

Nguyên tắc đã chốt: **cứ tham gia là có quà**, có **thưởng mốc điểm bang** và **thưởng thứ hạng**.

- Quà tham gia cá nhân: phần cơ bản, không phụ thuộc thứ hạng.
- Mốc điểm bang: đạt từng mốc mở thêm thưởng cho thành viên tham gia.
- Thứ hạng cá nhân và bang: thưởng tăng theo thứ hạng.
- Quyền chiếm thành là quyền lợi bổ sung khi đủ điều kiện.

Danh mục được đề xuất, chưa chốt số lượng: Xu, EXP, đá cường hóa cho quà cơ bản; đá tẩy luyện, ngọc và danh hiệu cho các mức cao hơn. Chưa chốt thưởng mốc cá nhân, điều kiện ghi nhận tham gia, cách chia thưởng bang và tính khóa/giao dịch của vật phẩm.

#### Quyền lợi thành chủ

- Thành có NPC đấu giá và treo bán đồ.
- Bang chủ đặt thuế trên giá giao dịch, tối đa **5%**.
- Thuế thu được tích vào **quỹ bang**, không vào tiền cá nhân bang chủ.
- Đề xuất thu khi giao dịch thành công, khấu trừ từ tiền người bán. Ví dụ giá 100.000 Xu, thuế 5%: người bán nhận 95.000, bang nhận 5.000. Thời điểm và bên chịu thuế chưa được xác nhận riêng.
- Chưa chốt thời hạn sở hữu, phạm vi sàn giao dịch chịu thuế và cách xử lý nếu kỳ sau không ai hạ được Quỷ Vương.

## 7. Kinh tế và giao dịch

### 7.1. Tiền tệ

| Loại | Nguồn và sử dụng |
| --- | --- |
| Xu | Kiếm qua đánh quái, nhiệm vụ, sự kiện và hoạt động trong game; dùng giao dịch và các chi phí trong game |
| Lượng | Nhận từ nạp tiền; dùng mua vật phẩm và tiện ích trong shop |

**Chỉ có hai loại tiền: Xu và Lượng. Xu không phân thành tiền khóa/không khóa**, không có thứ tự ưu tiên tiêu hao giữa hai dạng Xu. Giá và sản lượng cụ thể được cấu hình, cân chỉnh theo vận hành; không thêm loại tiền hoặc cơ chế phân tầng khác.

### 7.2. Giao dịch

- Giao dịch trực tiếp 1–1 có khóa/xác nhận.
- Mở sạp tự động ở khu an toàn trong thành.
- NPC đấu giá/treo đồ ở Vô Hạn Thành theo thiết kế thành chủ.
- Đề xuất an toàn giao dịch: khóa nội dung rồi xác nhận; thay đổi món hoặc số tiền phải hủy xác nhận cũ. Chi tiết giao diện chưa chốt.

Các khoản tiêu hao Xu như bình thuốc, mở kho, cường hóa và tẩy luyện giúp loại tiền khỏi lưu thông. **Thuế giao dịch chuyển vào quỹ bang**, theo cơ chế thành chủ đã có; khoản này vẫn nằm trong nền kinh tế. Không bổ sung phí chợ hệ thống riêng trong thiết kế bản đầu.

## 8. Auto-play

- Tự chọn quái và dùng kỹ năng theo thứ tự ưu tiên.
- Tự nhặt theo bộ lọc phẩm chất/loại vật phẩm; có thể bỏ đồ Trắng.
- Giữ phạm vi bãi gốc, quay lại nếu đi quá xa.
- Dùng bình HP/MP theo phần trăm người chơi cài; ví dụ 50% là giá trị tham khảo.
- Có định hướng tự hồi sinh, về thành mua thêm bình và trở lại bãi cũ; điều kiện, chi phí và tuyến đi chưa chốt.
- **Khi auto đánh quái bị người chơi khác tấn công, tự chuyển mục tiêu sang người tấn công để phản công.**
- Auto sử dụng cùng quy tắc kỹ năng và kiểm tra server như thao tác tay; không có thưởng combo.
- Chạm nhiệm vụ chỉ tìm đường, không tự bật auto.

Chưa chốt khi nào ngừng truy đuổi người tấn công, cách chọn giữa nhiều người tấn công và thời điểm quay lại đánh quái.

## 9. Thú cưỡi, trợ thủ và thời trang

- Thú cưỡi tăng tốc độ di chuyển, bổ sung chỉ số và có tiến hóa ngoại hình.
- Linh thú/trợ thủ đi theo, có thể nhặt đồ hoặc dùng kỹ năng hỗ trợ như hồi máu, choáng, buff sát thương.
- Thời trang/cánh, mũ, áo choàng và ngoại hình vũ khí thay đổi hình ảnh mà không làm mất chỉ số trang bị thực tế.
- Chưa chốt cách bố trí ô thú cưỡi/trợ thủ, số lượng hoạt động đồng thời, quy tắc kỹ năng trợ thủ và cách sử dụng thú cưỡi trong chiến đấu.

## 10. Các quyết định còn mở

1. Chiến đấu: xác suất chính xác/né, công thức kháng và xuyên giáp, giới hạn giảm sát thương, hệ số và giới hạn chí mạng. Kiểm chứng cân bằng cấp cao/trang bị theo [nguyên tắc cân bằng chiến đấu](combat-balance.md); bộ số nền, quy đổi tiềm năng, công thức thủ ban đầu và hệ số PvP đã được ghi tại mục 2.2.
2. Sự kiện: lịch và thời lượng chính thức, mức điểm từng mục tiêu, các mốc thưởng, số lượng và tính khóa của phần thưởng, điều kiện ghi nhận tham gia; công thức ghép quà lễ hội và quy tắc xếp hạng đua top.
3. Vô Hạn Thành: khoảng chờ Boss kế tiếp, xử lý hết giờ khi Boss còn sống, đồng điểm, điểm PK, rời/đổi bang, cách chia thưởng bang, quyền thành chủ cũ khi Quỷ Vương không bị hạ, thời hạn sở hữu, phạm vi chịu thuế, thời điểm thu và bên trả thuế.
4. Diễn Võ: chọn thể thức 1vs1/3vs3, công thức xếp hạng, cách trao danh hiệu cho đội 3vs3 và phần thưởng khi đạt Top 1 nhiều tuần.
5. Auto-play: điều kiện/chi phí hồi sinh và tiếp tế, chọn đối thủ khi nhiều người tấn công, giới hạn truy đuổi và thời điểm quay lại đánh quái. Thú cưỡi/trợ thủ: cách bố trí ô, số lượng hoạt động đồng thời, kỹ năng trợ thủ và sử dụng thú cưỡi khi chiến đấu.
6. Kỹ thuật: công cụ và nền tảng triển khai, ngân sách hiệu năng, kích thước ô tọa độ thế giới, số ô mỗi giây của tốc độ di chuyển 1 (chốt sau chơi thử), sức chứa kênh thực tế và múi giờ reset nhiệm vụ.
7. Nhiệm vụ: thứ tự ưu tiên biểu tượng khi NPC có nhiều nhiệm vụ.

## 11. Để sau v1.0.0.0

Các phần dưới đây đã có thiết kế nháp hoặc ý tưởng nhưng **không nằm trong v1.0.0.0**:

- **Chế tạo và đồ chế:** ba dòng Tử Đằng (bậc 30), Phá Dạ (bậc 50), Nhật Luân (bậc 70), đủ 10 ô. Chỉ số đã tính sẵn tại mục 7 của [bảng chỉ số trang bị](equipment-stats-proposal.md). Khi bật lại phải tính lại chỉ số quái từ cấp 30, vì quái v1 dựng theo đồ thường.
- **Nguyên liệu chế tạo:** vải, da, tơ, sắt, gỗ, quặng; quái thú/nhện cho da/tơ, quái rừng cho gỗ, quái thân cứng cho quặng; lưu tên người chế tạo trên món.
- **Độ bền và sửa đồ:** chỉ áp dụng cho đồ chế; đồ thường không có độ bền.
- **Hậu kỳ cấp 60–90:** kỹ năng hoặc bậc trang bị mới.
