# Nguyên tắc cân bằng chiến đấu

Tài liệu này là căn cứ khi thiết kế hoặc sửa công thức thuộc tính, sát thương, phòng thủ, chí mạng, kỹ năng, MP và trang bị. Đọc cùng [thiết kế trò chơi](game-design.md). Mục tiêu trải nghiệm dưới đây được ưu tiên hơn các hệ số thử nghiệm chưa đạt mục tiêu.

## 1. Mục tiêu PvP khi sức mạnh tương đương

**Hai nhân vật cùng cấp, cách cộng điểm và sức mạnh tương đương đánh nhau thì khoảng 10 đòn trúng là hạ được đối phương, kể cả khi mặc trang bị.** Cùng bậc, phẩm chất và mức cường hóa phải tiếp tục đạt mục tiêu này: hai người cùng bộ +0 hay cùng bộ +15 đều quanh 10 đòn. Không giới hạn tiêu chí vào cấp 10 hoặc cấu hình không đồ.

- Mốc kiểm tra đầu tiên: hai nhân vật cấp 10, không trang bị, không thú cưỡi/trợ thủ hoặc nguồn cộng chỉ số khác, bắt đầu đầy HP/MP.
- Ở cấp nhân vật 10, kỹ năng riêng mới là cấp kỹ năng 1. Không dùng kỹ năng cấp 10 để đại diện cho trận đấu này; kỹ năng đó đạt cấp 10 sớm nhất ở cấp nhân vật 19.
- Phép đo cơ sở bỏ hồi máu, hồi MP, chí mạng và trượt để tách ảnh hưởng của công, HP và phòng thủ. Sau đó phải kiểm tra lại với đầy đủ cơ chế thực tế.
- Một đòn trúng là một lần server áp dụng sát thương từ chém mặc định hoặc một kỹ năng độc lập lên mục tiêu; không đếm số vệt chém trong animation, không có thưởng combo.
- Mục tiêu trung tâm là 10 đòn; dùng khoảng **8–12 đòn** làm khoảng kiểm tra ban đầu cho cấu hình sức mạnh tương đương. Khi bên bị đánh là class phép (Thủy, Phong), khoảng kiểm tra là **7–12 đòn**, vì class phép có HP thấp hơn theo chủ đích. Đây không phải quy tắc ép mọi cách cộng điểm hoặc mọi cặp class cùng chết sau đúng 10 đòn.
- Trang bị so sánh phải tương đương cả chỉ số phụ, dòng ẩn được kích hoạt và ngọc; nguồn cộng khác như thú cưỡi/trợ thủ phải được loại bỏ hoặc giữ tương đương giữa hai bên. Chênh lệch trang bị/cường hóa được phép tạo khác biệt rõ về số đòn chịu được.
- Với phép đo chỉ dùng một loại đòn, sát thương thực nhận trung bình nên xấp xỉ **10% HP tối đa** của mục tiêu. Với nhịp dùng nhiều loại đòn, xét tổng sát thương thực nhận của cả chuỗi khoảng 10 đòn.
- Ghi lại cả số đòn và thời gian hạ mục tiêu. Hồi chiêu ngắn làm cùng số đòn diễn ra nhanh hơn; animation không phải thời gian khóa đánh.

## 2. Cấu hình kiểm tra và cách cộng điểm

- Mỗi lần lên cấp nhận 10 điểm tiềm năng. Khi không có điểm tặng khởi tạo, nhân vật từ cấp 1 lên cấp 10 có 90 điểm nhận qua lên cấp.
- Cấu hình tham chiếu đầu tiên: 60 điểm thuộc tính sát thương chính và 30 VIT. Viêm/Minh/Nham dùng STR; Thủy/Phong dùng INT.
- Kiểm tra thêm cấu hình tập trung sát thương, nhiều VIT và có AGI. Đổi điểm tấn công lấy sinh tồn phải có hiệu quả hợp lý; không để một nhóm thuộc tính lấn át tất cả cách cộng khác.
- Không bắt mọi cấu hình cùng đạt đúng 10 đòn. Dùng cấu hình tham chiếu để hiệu chỉnh, rồi so sánh độ lệch của những cấu hình còn lại.
- Kiểm tra cả cùng class lẫn khác class; cùng hệ số kỹ năng không tự đảm bảo cân bằng vì khác tầm đánh, diện rộng, bị động và chỉ số dùng cho chém mặc định.
- Chém mặc định dùng **100% × max(Công vật lý, Công phép)** làm sát thương cơ sở, nhưng vẫn gây sát thương vật lý và chịu thủ vật lý. Kiểm tra tổng hiệu quả chém thường và kỹ năng của Thủy/Phong, không chỉ so hệ số chiêu riêng.

## 3. Hiệu chỉnh công thức theo kỹ năng thực tế

1. Tính HP, MP, công/thủ và chỉ số phụ từ nền, tiềm năng, trang bị và bị động; ghi rõ mọi giả định.
2. Tính sát thương từng đòn sau phòng thủ, kháng và hệ số PvP; không dùng sát thương cơ sở làm sát thương thực nhận.
3. Đo số đòn và thời gian hạ mục tiêu ở cấp 10 trước, hiệu chỉnh về khoảng 10 đòn trong cấu hình tham chiếu.
4. Kiểm tra tiếp cấp 19, 30, 39, 50, 59 và 90 để bao phủ các mốc kỹ năng, bị động và kỹ năng đạt cấp tối đa. Một hệ số phù hợp ở cấp cao không được mặc định là phù hợp ở đầu game.
5. Kiểm tra nhịp dùng kỹ năng hợp lệ với hồi chiêu độc lập, MP và chém mặc định. Không giả định animation khóa kỹ năng, hoặc luôn có vô hạn MP.
6. Sau phép đo cơ sở, bật lại chính xác/né, chí mạng, bình hồi phục và các bị động để đánh giá kết quả thực tế.
7. Bắt buộc kiểm tra các cặp trang bị tương đương theo bảng dưới đây; không lấy kết quả không đồ làm bằng chứng cả hệ thống trang bị đã cân bằng. Kiểm tra riêng cặp trang bị chênh lệch, PvE và chiêu đánh 2–5 mục tiêu. Mục tiêu 10 đòn PvP không mặc định áp cho quái thường hoặc Boss.

### Kiểm tra chỉ số trang bị và cường hóa

| Cấu hình hai bên | Phạm vi kiểm tra | Tiêu chí |
| --- | --- | --- |
| Không trang bị | Các mốc cấp nhân vật và kỹ năng | Khoảng 10 đòn trúng với cấu hình tham chiếu |
| Cùng đồ thường bậc 10/20/30/40/50 | Cùng phẩm chất, đủ cấp mặc các ô đang kiểm tra | Khoảng 10 đòn trúng ở từng bậc |
| Cùng đồ chế bậc 30/50/70 (để sau v1.0.0.0) | Cùng phẩm chất, đủ cấp mặc các ô đang kiểm tra | Khoảng 10 đòn trúng ở từng bậc |
| Cùng bậc và phẩm chất, cùng mức cường hóa | Từng mức +0 đến +15 | Khoảng 10 đòn trúng ở từng mức, gồm cả +15 |
| Cùng vũ khí hoặc cùng giáp nhưng khác các món còn lại | Kiểm tra tổng bộ, không chỉ một món | So sánh theo tổng sức mạnh thực tế |
| Khác bậc/phẩm chất/cường hóa | Kiểm tra lợi ích nâng cấp | Cho phép số đòn khác nhau, không ép về 10 |

- Xây công trên vũ khí cùng HP và phòng thủ của toàn bộ trang bị. Không tăng công theo bậc hoặc cường hóa rồi giữ nguyên khả năng chịu đòn của bộ tương đương.
- Tính sát thương sau phòng thủ trước khi suy ra số đòn; cùng nhân công, HP và thủ với một hệ số không tự bảo toàn số đòn vì công thức giảm sát thương không tuyến tính.
- Dùng chuỗi chém thường xen kỹ năng hợp lệ theo cấp, có ghi rõ thứ tự đòn, hồi chiêu và MP. Không dùng sát thương của một chiêu đơn lẻ thay cho nhịp đánh thực tế để tuyên bố đã cân bằng cả bộ.
- Các hệ số tăng theo bậc trang bị, chênh lệch đồ chế và thưởng cường hóa chỉ được coi là đạt cân bằng sau khi kiểm tra các cấu hình tương đương trên; bảng chỉ số thử chưa đạt phải được hiệu chỉnh trước khi chốt.

Các nhóm số được phép hiệu chỉnh để đạt mục tiêu: chỉ số nền/theo cấp, mức quy đổi tiềm năng, phòng thủ/kháng, hệ số PvP và các hệ số sát thương. Không tự thay đổi quy tắc đã chốt như điểm nhận khi lên cấp, mốc học, tối đa 10 cấp kỹ năng, tối đa 5 mục tiêu, sách kỹ năng và cơ chế animation.

## 4. Bộ số hiện hành và kết quả tham chiếu

Bộ số đã duyệt để làm căn cứ ban đầu; xem bảng đầy đủ tại mục 2.2 của [thiết kế trò chơi](game-design.md):

| Thành phần | Giá trị |
| --- | --- |
| 1 STR | +2 công vật lý |
| 1 AGI | +3 chính xác, +2 né tránh, +0,02 điểm phần trăm chí mạng |
| 1 VIT | +20 HP với class vật lý, +16 HP với class phép; +2 thủ vật lý, +2 thủ phép |
| 1 INT | +2 công phép, +5 MP |
| HP nền | 500 ở cấp 1; class vật lý +60, class phép +50 mỗi lần lên cấp |
| Công vật lý/phép nền | Mỗi loại 20 ở cấp 1, +5 mỗi lần lên cấp |
| Thủ vật lý/phép nền | Mỗi loại 10 ở cấp 1, +2 mỗi lần lên cấp |
| Giảm do thủ | Thủ / (Thủ + 20 × cấp người tấn công) |
| Hệ số PvP | 100% |
| Sát thương cơ sở chém mặc định | 100% × max(Công vật lý, Công phép); loại sát thương vật lý |

Hai nhân vật Viêm cấp 10, mỗi người cộng 60 STR/30 VIT, không đồ và không nguồn cộng khác:

| Chỉ số | Giá trị |
| --- | --- |
| Công vật lý | 185 |
| HP tối đa | 1.640 |
| Thủ vật lý | 88 |
| MP tối đa | 280 |

Bất Tri Hỏa cấp kỹ năng 1:

`floor(185 × 1,5 × (1 − 88 / 288) × 1,0) = 192 sát thương`.

Chém mặc định:

`floor(185 × (1 − 88 / 288) × 1,0) = 128 sát thương`.

- Chỉ dùng Bất Tri Hỏa: **9 đòn trúng** để hạ mục tiêu.
- Xen một kỹ năng rồi một chém mặc định: **11 đòn trúng**; 10 đòn gây 1.600 sát thương, chưa đủ hạ 1.640 HP; đòn kỹ năng thứ 11 kết liễu.
- Các kết quả trên bỏ trượt, chí mạng và hồi phục. Đây mới là kiểm tra cấp 10 không đồ; chưa có kết quả xác nhận bộ chỉ số trang bị. Phải kiểm tra tiếp các cặp trang bị tương đương và cường hóa theo mục 3, vẫn dùng mục tiêu khoảng 10 đòn.
- Bộ **+80 HP/VIT, HP nền +100/cấp và PvP 35%** cũ đã được thay thế. Kết quả cũ 67 sát thương/3.800 HP, cần 57 đòn, chỉ là ví dụ cấu hình không đạt mục tiêu; không dùng làm bộ số hiện hành.
