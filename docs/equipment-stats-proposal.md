# Chỉ số trang bị — bộ số bản đầu

Bộ số này đã được người dùng duyệt cho bản đầu; giữ để triển khai, cân chỉnh tiếp khi có dữ liệu chơi thử. Quy tắc trang bị được ghi tại mục 3 của game-design. Dữ liệu từng món và từng mức +0 đến +15 nằm trong [bảng CSV](balance/equipment-stats-proposal.csv); phép tính có thể chạy lại bằng [script cân bằng](balance/equipment_balance.py).

## 1. Kết quả và phạm vi kiểm tra

Bản v1.0.0.0 chỉ có **năm dòng đồ thường**; đồ chế được để sau (xem mục 7). Đã kiểm tra **32.400 cấu hình tham chiếu**, tất cả cho kết quả **8–12 đòn trúng khi bên bị đánh là class vật lý** và **7–12 đòn khi bên bị đánh là class phép** (HP nền +50/cấp và +16 HP/VIT thay vì +60 và +20). HP trang bị vẫn tính theo mốc HP class vật lý và dùng chung cho mọi class, nên CSV không đổi:

- Cấp nhân vật 10 đến 90, từng cấp một; 25 cặp tấn công/phòng thủ giữa năm class.
- Dùng đồ thường tốt nhất mặc được ở từng ô. Từ cấp 59 trở lên, cả bộ là Xích Cương bậc 50.
- Hai bên dùng cùng bậc, cùng mức cường hóa, từ +0 đến +15; có kiểm tra lúc mới thay từng món, không chỉ khi đủ bộ.
- Cộng 2/3 điểm vào thuộc tính sát thương chính, phần còn lại vào VIT; làm tròn xuống số điểm thuộc tính chính. Ví dụ cấp 10 là 60/30; cấp 19 là 120/60.
- Cấp kỹ năng đầu bằng min(10, cấp nhân vật − 9); bị động có từ cấp 30 và tăng tối đa ở cấp 39, theo luật nâng kỹ năng đã chốt.
- Nhịp đo là một kỹ năng đầu rồi một chém mặc định, lặp lại; đủ tầm đánh. Tính riêng thủ vật lý/thủ phép; tính bị động Viêm, Nham và Phong. Không tính chí mạng, né, hồi máu, bình, dòng phụ, dòng ẩn, ngọc, thú cưỡi hoặc trợ thủ.

Đây là kết quả của nhịp đánh tham chiếu, không phải khẳng định mọi chuỗi kỹ năng hoặc mọi cấu hình trang bị đã cân bằng. Cần kiểm tra tiếp toàn bộ kỹ năng và các nguồn cộng chỉ số trên cùng dữ liệu này trước khi dùng làm cấu hình phát hành. Chưa có bảng chỉ số quái để xác nhận PvE.

## 2. Chỉ số của toàn bộ 10 món

Các số sau chỉ là phần **trang bị cộng thêm**, chưa gồm nền nhân vật, tiềm năng và bị động. Cột Thủ là lượng cộng vào **mỗi loại** thủ vật lý và thủ phép, không phải tổng hai loại.

| Dòng | Bậc | Công +0 | HP +0 | Mỗi loại thủ +0 | Công +15 | HP +15 | Mỗi loại thủ +15 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Lữ Hành — thường | 10 | 100 | 2.152 | 15 | 430 | 5.084 | 64 |
| Thanh Thiết — thường | 20 | 200 | 3.896 | 30 | 860 | 9.518 | 129 |
| Tinh Cương — thường | 30 | 350 | 5.505 | 53 | 1.505 | 14.272 | 227 |
| Huyền Cương — thường | 40 | 550 | 7.992 | 83 | 2.365 | 21.065 | 356 |
| Xích Cương — thường | 50 | 800 | 11.731 | 120 | 3.440 | 30.814 | 516 |

Công trên vũ khí chỉ tăng loại sát thương của class; vũ khí khóa đúng class. Công trên chín món còn lại tăng cùng lượng vào công vật lý và công phép để các class dùng chung trang bị này. Do đó tổng Công trong bảng là lượng tăng cho chỉ số tấn công chính của nhân vật.

## 3. Ví dụ đầy đủ từng ô: Lữ Hành +0

| Ô | Cấp mặc | Công | HP | Thủ vật lý | Thủ phép |
| --- | --- | --- | --- | --- | --- |
| Vũ khí | 10 | 25 theo class | 0 | 0 | 0 |
| Giày | 11 | 5 mỗi loại | 35 | 2 | 2 |
| Hộ uyển | 12 | 10 mỗi loại | 226 | 0 | 0 |
| Quần | 13 | 10 mỗi loại | 214 | 4 | 4 |
| Mũ | 14 | 10 mỗi loại | 206 | 2 | 2 |
| Áo | 15 | 15 mỗi loại | 294 | 6 | 6 |
| Dây chuyền | 16 | 5 mỗi loại | 268 | 0 | 0 |
| Ngọc bội | 17 | 10 mỗi loại | 274 | 1 | 1 |
| Nhẫn trái | 18 | 5 mỗi loại | 305 | 0 | 0 |
| Nhẫn phải | 19 | 5 mỗi loại | 330 | 0 | 0 |

Các dòng khác giữ cùng thứ tự ô, cấp mặc bằng bậc + vị trí 0–9. CSV có đầy đủ năm dòng đồ thường, từng ô và toàn bộ 16 mức cường hóa, gồm năm phiên bản vũ khí riêng. Không tự nhân các số của Lữ Hành để suy ra HP của dòng khác.

## 4. Cường hóa trong bộ số này

Công và thủ dùng thang tổng so với +0: **1,00 / 1,03 / 1,07 / 1,12 / 1,18 / 1,26 / 1,36 / 1,49 / 1,65 / 1,85 / 2,10 / 2,40 / 2,75 / 3,15 / 3,65 / 4,30**. Tổng được chia theo ô, làm tròn và bảo đảm không có chỉ số nào giảm khi nâng thành công.

**HP dùng bảng riêng theo món và mức cường hóa**, không nhân đồng loạt với công/thủ. Công thức thủ phi tuyến khiến nhân tất cả cùng 4,3 không giữ được số đòn. Đề xuất này có thể cho HP bổ sung khi cường hóa cả món ban đầu không có HP: ví dụ vũ khí Lữ Hành từ +0 có 25 công/0 HP lên +15 có 108 công/553 HP. Đây là chỉ số cường hóa đã được duyệt cho bản đầu, không phải dòng phụ ngẫu nhiên.

Bảng HP được tính **ngoại tuyến** theo ngân sách sát thương/sinh tồn rồi lưu thành chỉ số cố định của món. Khi đánh nhau không tự đổi HP, không giảm sát thương theo đồ đối thủ và không ép mỗi đòn mất 10% máu.

## 5. Ví dụ sát thương thực nhận

Hai nhân vật Viêm cùng cấp, cùng cách cộng điểm, trang bị tương đương; sát thương đã tính phòng thủ và PvP 100%, chưa chí mạng/né/hồi máu. Kỹ năng là Bất Tri Hỏa ở cấp kỹ năng hợp lệ cao nhất.

| Cấp nhân vật | Trang bị | HP tổng | Sát thương kỹ năng | Sát thương chém thường | Số đòn xen kẽ để hạ |
| --- | --- | --- | --- | --- | --- |
| 10 | Chỉ vũ khí Lữ Hành +0 | 1.640 | 218 | 145 | 9 |
| 10 | Chỉ vũ khí Lữ Hành +15 | 2.193 | 305 | 203 | 9 |
| 19 | Đủ Lữ Hành +0 | 4.932 | 868 | 304 | 9 |
| 19 | Đủ Lữ Hành +15 | 7.864 | 1.384 | 485 | 9 |
| 59 | Đủ Xích Cương +0 | 19.591 | 3.261 | 1.144 | 9 |
| 59 | Đủ Xích Cương +15 | 38.674 | 6.294 | 2.208 | 9 |

Cùng các cấu hình trên nhưng Viêm đánh Thủy: HP Thủy lần lượt 1.430 / 1.983 / 4.512 / 7.444 / 18.235 / 37.318, cần 8 / 8 / 8 / 8 / 8 / 9 đòn. Ở cấp 59, sát thương lên Thủy cao hơn vì Thủy không có bị động tăng thủ như Viêm.

Số đòn không phải số giây. Khi đo thời gian cần xếp lịch theo hồi chiêu, xác nhận đủ MP và tính hành vi thực tế; calculator này chỉ đo số đòn của chuỗi tham chiếu.

## 6. Chạy lại phép tính

`python3 docs/balance/equipment_balance.py`

Xuất lại CSV: `python3 docs/balance/equipment_balance.py --write-csv`.

Script kiểm tra đủ 32.400 cấu hình và bảo đảm các chỉ số của từng món không giảm khi nâng cường hóa. CSV là dữ liệu tĩnh đã duyệt cho bản đầu; game chưa được triển khai. Tên tệp giữ nguyên để không làm hỏng các liên kết.

## 7. Đồ chế — để sau v1.0.0.0

Ba dòng đồ chế dưới đây đã được thiết kế nhưng **không có trong v1.0.0.0**. Chúng sẽ quay lại cùng tính năng chế tạo và độ bền. Script vẫn giữ dữ liệu (`INCLUDE_CRAFTED = False` trong [script cân bằng](balance/equipment_balance.py)); bật lại sẽ kiểm tra cả đường đồ chế. Chỉ số từng món của đồ chế được lưu tại [CSV đồ chế](balance/later/crafted-equipment-stats.csv).

| Dòng | Bậc | Công +0 | HP +0 | Mỗi loại thủ +0 | Công +15 | HP +15 | Mỗi loại thủ +15 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Tử Đằng — chế | 30 | 403 | 5.966 | 61 | 1.732 | 15.719 | 262 |
| Phá Dạ — chế | 50 | 920 | 11.824 | 138 | 3.956 | 31.665 | 593 |
| Nhật Luân — chế | 70 | 1.610 | 19.037 | 242 | 6.923 | 50.400 | 1.040 |

Đồ chế cùng bậc tăng khoảng 15% ngân sách công/thủ so với đồ thường. HP được cân riêng, không ép tăng cùng 15%, vì còn phải xét giảm sát thương do thủ. Tên Nhật Luân chỉ thuộc dòng chế bậc 70. Đồ thường không phụ thuộc vào đồ chế, nên bỏ đồ chế không làm thay đổi chỉ số đồ thường.
