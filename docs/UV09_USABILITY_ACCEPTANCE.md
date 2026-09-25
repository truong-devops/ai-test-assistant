# UV-09 — Phiếu nghiệm thu người mới

Trạng thái: **chưa thực hiện**. Không điền kết quả thay cho người dùng.
Ba người chưa quen hệ thống, dùng dữ liệu tổng hợp và tài khoản staging phù hợp.
Không thu tên thật hoặc dữ liệu cá nhân khi mã P1/P2/P3 đã đủ.

## Chuẩn bị

- Ghi build commit, schema, URL staging, browser/OS và screen reader/version.
- Người điều phối chỉ đọc đề bài; ghi mọi lần giải thích, không bấm hộ.
- Đo thời gian thao tác riêng với thời gian chờ AI. Ghi job/request ID khi lỗi.
- Không cho người thử SQL/log hoặc Re-index để vượt lỗi trong luồng thành công.

## Nhiệm vụ

1. Tạo bộ, upload, duyệt nguồn/yêu cầu, sinh và duyệt testcase, chốt R1 và xuất XLSX.
2. Tìm testcase đang dùng, tạo draft mới, so sánh, duyệt và chốt R2; chứng minh R1
   vẫn pin revision cũ, không tự chuyển binding project.
3. Upload nguồn v2, xem affected scope, cập nhật một case; mở run/export của revision
   trước và giải thích vì sao expected/actual không chuyển sang revision mới.

| Người | Nhiệm vụ | Xong không? | Thao tác / chờ AI | Số lần hỏi bấm đâu | Refresh/Re-index | Hiểu latest/pinned? | Vướng / job ID |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P1 | 1–3 (ghi từng nhiệm vụ) | Chưa thử | — | — | — | — | — |
| P2 | 1–3 (ghi từng nhiệm vụ) | Chưa thử | — | — | — | — | — |
| P3 | 1–3 (ghi từng nhiệm vụ) | Chưa thử | — | — | — | — | — |

## Điều kiện đạt

- Cả ba hoàn thành nhiệm vụ 1, không cần DB/log; tối đa 1 lần hỏi/người/nhiệm vụ.
- 0 refresh/Re-index thủ công ở luồng bình thường; tìm diff tối đa 3 thao tác.
- Phân biệt được testcase revision / automation version / release / execution.
- Keyboard/screen reader đọc được evidence, diff, selection, error và review;
  không mắc kẹt focus. Ghi case NVDA/VoiceOver đã chạy riêng, không suy từ screenshot.
- Ghi và sửa blocker trước release; chạy lại nhiệm vụ bị ảnh hưởng.

Người điều phối, người duyệt release, ngày ký và link evidence: **chưa có**.
