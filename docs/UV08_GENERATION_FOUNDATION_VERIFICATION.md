# UV-08 — Chặng nền generation có scope và dedupe an toàn

Ngày kiểm tra: 22/09/2026. Đây là một phần UV-08, **chưa hoàn thành phase**.
Không thêm migration; schema vẫn là 28. Thay đổi chưa commit.

## Phần đã triển khai

- Workflow generation truyền exact requirement IDs trong `input_snapshot` xuống
  testcase service, không gọi lại `List(APPROVED)` sau khi validate snapshot.
  Tất cả requirement được tải và kiểm tra ownership/status/blockers/evidence
  trước khi gọi provider hoặc ghi testcase. Nguồn duyệt thêm lúc provider chạy
  không bị đưa âm thầm vào job đang chạy.
- API workflow nhận optional `requirement_ids`: 1–100 unique positive IDs, phải
  thuộc đúng set, current/approved/unblocked. Selection lưu vào snapshot và tham
  gia idempotency; đổi scope với cùng key bị từ chối. Omit/empty giữ hành vi
  all-approved-current cũ; job cũ không có selection vẫn đọc/replay được.
- Bỏ semantic auto-suppress và tự merge requirement sources trong generation.
  Exact dedupe so title/type/risk/actor/precondition/data/expected/postcondition,
  ordered steps, assumptions, exact requirement IDs và generation metadata.
  Hai negative/boundary scenario có cùng title/expected nhưng data/steps khác
  nhau không bị gộp. Khác requirement revision không tự quyết lineage.
- Revision mới có `provenance.generation` do server xây dựng: provider/model,
  prompt version/hash, response hash khi có AI, requirement review hash,
  workflow job/input hash và source revision. Trường model cũng được giữ ở cấp
  provenance ngoài để timeline UV-07 đọc được. AI call log hiện có tiếp tục lưu
  prompt/schema/response thật. Không sửa provenance của revision lịch sử/replayed.

## Bằng chứng

- Unit: so đầy đủ field/data/case-sensitive input/ordered steps/source/model/
  prompt; exact duplicate bị suppress, semantic candidate được giữ; bookkeeping
  job mới không tự thành scenario mới. Baseline invalid/foreign/draft/blocked/
  thiếu evidence bị từ chối trước ghi dữ liệu.
- Testcase PostgreSQL integration: provider fixture trả hai scenario khác data/
  steps và một bản trùng; tạo đúng hai draft. Thêm approved requirement trong lúc
  provider chạy không mở rộng baseline. Retry không thêm testcase hoặc duplicate
  suppression receipt, giữ head do QA sửa. Chọn thêm requirement khác với cùng
  output tạo case độc lập; provenance và 4 AI-call records được kiểm tra.
- Workflow unit/integration: scope reordered replay được; scope khác với cùng
  key bị chặn; foreign/nonexistent/revoked approval không được nhận; thêm approved
  requirement ngoài selected scope không đổi snapshot đó; all-baseline cũ vẫn
  phát hiện input stale. Worker stub xác nhận nhận exact IDs và job/hash pin.
- HTTP: JSON `requirement_ids` được chuyển nguyên vẹn tới workflow service,
  endpoint vẫn trả HTTP 202/status URL. Authorization hiện hữu không thay đổi.
- Sửa cleanup workflow fixture: chạy trước khi đóng pool và xóa theo dependency
  order của source snapshots; không bỏ qua lỗi để queued job rơi sang test sau.
  Workflow integration chạy đạt `-count=2` sau khi sửa. Những fixture từ lượt
  kiểm tra thất bại được xóa theo exact IDs, không xóa dữ liệu người dùng.
- `make test`, `make lint` và PostgreSQL integration toàn bộ `./internal/...`:
  đạt trên database local schema 28, provider fixture, không gọi AI trả phí.

Chạy lại từ repo root:

```sh
make test lint
cd backend
TEST_DATABASE_URL='<test PostgreSQL URL>' \
  go test -count=1 -p 1 -tags=integration ./internal/...
```

API và worker phải deploy cùng bản code để truyền/đọc scope mới. Không thay UI
trong chặng này; không ghi nhận một browser journey UV-08 đã đạt.

## Những việc chưa hoàn thành

1. Persist proposal/generation units và exact target head, phân loại NEW_CASE /
   NEW_REVISION / UNCHANGED / RETIRE_CANDIDATE / AMBIGUOUS_MATCH.
2. So content **và** provenance với baseline đã lưu trước khi kết luận UNCHANGED.
   `SaveGenerated` legacy vẫn reuse theo canonical content hash; chặng này không
   biến cơ chế reuse đó thành classifier UNCHANGED. Model/prompt khác chưa có
   màn đối chiếu hay chính sách lineage mới.
3. Apply proposal có CAS expected head, idempotency/approval guards; không dùng
   similarity để tự ghép identity. Test hiện chỉ chứng minh generation replay
   không ghi đè head do người sửa, chưa chứng minh concurrent proposal apply.
4. Scope affected cases mặc định sau cập nhật nguồn, đề xuất retire cho removed
   source, và mapping ambiguity do reviewer xử lý.
5. UI scope/coverage/budget summary, bảng diff proposals và chọn apply. UI hiện
   tại chưa gửi selected requirements qua control generation.
6. Retry từng failed unit và publication cùng lease/cancel guard: workflow hiện
   vẫn một operation unit, retry có thể gọi lại provider cho requirements đã
   xử lý; không tuyên bố tiết kiệm chi phí hoặc checkpoint từng requirement.
7. Integration/browser trọn hành trình R1/run/export → đổi nguồn → proposals →
   review/apply → R2, và bảo toàn các proof cũ.

Generation hiện vẫn tạo draft trực tiếp; không tự approve, bind release hoặc
chạy sandbox. Nếu nguồn thay đổi sau preflight, draft vẫn gắn exact nguồn cũ;
review/publish guards quyết định eligibility. Chưa có transaction nguyên job
để chặn mọi output khi cancel/source-change xảy ra giữa lúc generation đang chạy.
Không đóng các checkbox gộp của UV-08 hoặc những gate thủ công UV-05/07 vì chặng này.
