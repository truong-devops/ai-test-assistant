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
2. Upload DOCX/Markdown trong bước **Tài liệu**; worker tự parse/chuẩn bị nguồn.
   Kiểm tra preview/locator và chọn rõ các phiên bản nguồn cần duyệt.
3. Chọn **Duyệt & trích xuất**, nhập reviewer và xác nhận phạm vi. Đây là intent
   bền vững; không cần vào Semantic index để Re-index thủ công ở luồng bình thường.
4. Chờ tiến độ nguồn/trích xuất trên workspace. API trả `202`; worker xử lý nền,
   UI polling trạng thái nên không giữ HTTP request chờ LLM nhiều phút.
5. Mở **Yêu cầu**, xem evidence và giải quyết conflict/TBD; chọn exact revisions
   để duyệt theo nhóm rồi sinh testcase cho phạm vi đã approved.
6. Mở **Test cases**, chọn phạm vi sinh và xem proposals/diff. Apply các đề xuất
   được chọn để tạo draft, kiểm tra main/alternate/exception flow rồi duyệt riêng
   từng revision. Apply không tự approve hoặc publish. Xuất XLSX trước khi chạy
   để minh họa trạng thái `NY`.
7. Sang **Sử dụng & xuất**, chọn exact approved revisions, xem preview coverage
   rồi chốt release R1. Vào **Projects**, bind release đó làm baseline và đặt
   pipeline mode `DOCUMENT_DRIVEN` với actor/reason audit.
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
    Cần JSON `"passed": true` cùng artifacts của webhook/provider/sandbox thật.
    Verifier kiểm tra dữ liệu persisted, không tự chứng thực rằng các hệ thống
    ngoài đã được gọi; dữ liệu seed hoặc browser deterministic không thay demo thật.

## Quản lý version testcase (UV-07)

1. Ở bước testcase, mở v1 đã approved. Thay title/actor hoặc bước thao tác, nhập
   lý do rồi **Lưu bản nháp mới**. UI mở v2 draft; quyết định approve là nút riêng.
2. Mở **Lịch sử & so sánh**, đọc field/steps/citation thay đổi và nguồn. Nếu đổi
   expected bị chặn vì thiếu nguồn, làm rõ/duyệt requirement trước; không sửa
   expected theo kết quả code để biến FAIL thành PASS.
3. Duyệt v2, sang bước sử dụng/xuất, chọn đúng v2, xem coverage preview rồi chốt
   R2. R1 và project còn bind R1 không tự thay đổi.
4. Trong history v2 chọn **Đối chiếu / phục hồi v1**, xem diff, nhập lý do và
   **Phục hồi thành bản nháp mới**. Kết quả là v3 draft, không quay ngược ID/version
   và không đổi manifest R2.
5. Mở **Run & automation** của v3: chưa chạy không được hiển thị PASS từ v1.
   Chỉ chọn phạm vi cả identity khi muốn xem các run của revision khác. Xuất v3
   hoặc release R2 là hai lựa chọn có provenance khác nhau.
6. Thử hai tab cùng revision: lưu một tab trước; tab còn lại gặp 409 sẽ giữ form,
   cho so sánh head mới với nội dung đang nhập. Chỉ chọn dùng head mới sau khi
   đối chiếu; thao tác này không tự merge các field.

Xem [bằng chứng và giới hạn UV-07](UV07_TESTCASE_VERSIONING_VERIFICATION.md).
Kiểm thử local này không thay thế demo provider/SCM/sandbox thật bên trên.

## Cập nhật nguồn và giữ proof lịch sử (UV-08/09)

1. Lưu IDs của analysis/run R1 và tải export R1 trước khi thay nguồn.
2. Upload và duyệt nguồn v2; giải quyết requirement changes, chọn **affected**,
   **selected** hoặc **all** rõ ràng khi sinh proposal. Theo dõi job/unit progress;
   retry unit lỗi không được nhân proposal hoặc xóa quyết định đã hoàn tất.
3. Đọc diff, xử lý từng đề xuất apply/dismiss/keep/retire theo action UI cho phép.
   Matching mơ hồ cần người review; không tự nối hai scenario bằng title giống nhau.
4. Duyệt revision mới, publish R2 và bind project sang R2. Mở lại run/export R1:
   expected, revision manifest và bytes export đã lưu phải không đổi.
5. Chạy lại verifier với **IDs R1 ban đầu**, không thay bằng analysis/run mới để
   che mất proof lịch sử. Ghi riêng lỗi provider/sandbox nếu có.

Xem [ma trận UV-09](UV09_VERIFICATION.md) và
[mẫu nghiệm thu người mới](UV09_USABILITY_ACCEPTANCE.md). UV-09 còn mở; synthetic
migration/restore và browser local không phải sign-off rollout.

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
