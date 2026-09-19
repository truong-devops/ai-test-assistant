# Kịch bản demo Document-Driven Testing

Kịch bản này dùng PTYC/URD của chức năng đặt hàng. Tài liệu là nguồn nghiệp vụ;
repository chỉ được dùng sau khi test case đã duyệt để tạo và chạy automation.

## Chuẩn bị

- Stack đã chạy và migration mới nhất đã được áp dụng.
- Gemini/OpenAI đã cấu hình nếu muốn dùng AI thật; local deterministic provider
  vẫn dùng được để kiểm tra workflow không phụ thuộc mạng.
- Có một project GitHub/GitLab và sandbox image đã build.
- Production phải có `secrets/api_auth_token`; frontend dùng token đó để gọi API.

## Luồng trình diễn

1. Vào **Documents**, tạo set `Đặt hàng thành công`, product và scope `UC-B08`.
2. Upload DOCX/Markdown, mở version, kiểm tra structured blocks/source locator,
   nhập reviewer rồi chọn **Approve source**.
3. Mở **Semantic index**, chọn **Re-index** và chờ `Ready`.
4. Mở **Requirements**, chọn **Extract requirements**. API trả job `202`; UI
   hiện số chunk đã xử lý và tự tải lại khi job `COMPLETED`, nên request trình
   duyệt không còn phải chờ Gemini hàng phút và không bị proxy trả 502.
5. Mở từng requirement để kiểm tra evidence, conflict/TBD và duyệt baseline.
6. Mở **Test cases**, sinh draft, kiểm tra ma trận main/alternate/exception flow,
   rồi duyệt test case. Xuất XLSX trước khi chạy để minh họa trạng thái `NY`.
7. Vào **Projects**, chọn document suite làm baseline và đặt pipeline mode là
   `DOCUMENT_DRIVEN` với actor/reason audit.
8. Gửi Pull Request/Merge Request webhook. Mở **Change runs**, kiểm tra source
   SHA, technical scope và các test case được chọn từ baseline đã duyệt.
9. Sinh và duyệt Go automation artifact, sau đó chạy test run trong sandbox.
10. So sánh `PASSED`, `PRODUCT_FAILED`, `AUTOMATION_ERROR` và `INFRA_ERROR`.
    Chỉ item `AUTOMATION_ERROR` có nút repair; expected hash/assertion luôn khóa.
11. Duyệt artifact repair để hệ thống tự append attempt và chạy lại sandbox,
    hoặc từ chối để xử lý thủ công; sau đó xuất XLSX/Markdown có Expected,
    Actual, taxonomy gốc, source SHA và evidence.
12. Ghi lại `DOCUMENT_SET_ID`, `PROJECT_ID`, `ANALYSIS_ID`, `TEST_RUN_ID` rồi chạy
    `make prod-document-e2e-verify DOCUMENT_SET_ID=... PROJECT_ID=... ANALYSIS_ID=... TEST_RUN_ID=...`.
    Chỉ kết quả JSON `"passed": true` mới là bằng chứng DoD E2E.

## Bằng chứng nên mở cho giảng viên

- Document version hash và source locator.
- Semantic chunk count và requirement evidence.
- Requirement extraction job/progress thay vì một HTTP request dài.
- Coverage matrix xây từ liên kết database.
- Project baseline hash và source SHA của run.
- Sandbox image digest/environment fingerprint.
- Repair policy, before/after source hash và immutable expected-result hash.
- File XLSX cuối cùng và audit review/classification.

## Giới hạn cần nói rõ

Golden dataset trong `evaluation/datasets/document-controlled-v1.json` chỉ là
fixture kiểm thử công cụ đo, chưa phải kết quả thực nghiệm luận văn. Demo E2E
với webhook/LLM thật chỉ được xem là đạt sau khi credential, repository và
sandbox của môi trường demo đều được kiểm tra thành công.
