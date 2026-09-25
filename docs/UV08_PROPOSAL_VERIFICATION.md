# UV-08 — Proposal backend và apply có kiểm soát

Ngày kiểm tra: 23/09/2026. Tiếp nối [chặng nền](UV08_GENERATION_FOUNDATION_VERIFICATION.md).
**Chưa hoàn thành UV-08**: đây là backend opt-in, chưa có UI scope/compare/apply.

Đây là mốc kiểm chứng backend trước khi nối giao diện. Chặng UI tiếp nối đã chuyển
hai trang testcase sang proposal-first; trạng thái mới nhất nằm trong
[UV08_COMPLETION_VERIFICATION.md](UV08_COMPLETION_VERIFICATION.md). Các giới hạn
UI ghi bên dưới phản ánh thời điểm chặng backend, không phải trạng thái hiện tại.

## Phạm vi đã triển khai

- Migration 29: proposal lưu content/hash, generation context, classification,
  reason, source revision và candidates `{family_id,revision_id,head_token}`.
  Input bất biến; quyết định đã ghi không được sửa. Receipt review theo key/set
  nằm cùng transaction với revision/archive và quyết định proposal.
- Enqueue `GENERATE_TESTCASES` với `review_proposals:true` pin heads trước AI;
  optional selected requirements giữ contract chặng nền. Worker dùng pinned heads,
  không lấy human head mới làm baseline so sánh. Tối đa 1000 active identities/set.
- Persist cả batch dưới job row lock, kiểm tra đúng set/operation/input hash/
  attempt, RUNNING, lease còn hạn và chưa cancel; source revision phải còn hiện
  hành. Mọi output phải grounded vào approved/unblocked requirements. Batch lỗi
  rollback hết. Generation ở mode này không đổi testcase đang dùng.
- `NEW_CASE`: không có liên kết nguồn với identity đã pin. `UNCHANGED`: exact full
  content/citations và known generation context. `NEW_REVISION`: một exact-content
  target nhưng context khác/unknown. Content bao gồm steps/data/preconditions,
  không chỉ title/type/expected. Context gồm provider/model/prompt version/hash và
  requirement review hash; bỏ bookkeeping job/input/source counter và raw response
  formatting. AI-call log vẫn lưu prompt/schema/response thật.
- `AMBIGUOUS_MATCH`: nhiều exact targets hoặc chỉ liên quan requirement/ancestry/
  source comparison; ngay cả một related candidate cũng không đủ tự nối lineage.
  Reviewer chọn rõ `REVISE` vào candidate hoặc `CREATE_NEW` độc lập.
- `RETIRE_CANDIDATE`: chỉ all-baseline và tất cả requirement của pinned case đều
  `REMOVED`; bỏ khỏi selected scope không phải removed. `ARCHIVE` giữ nguyên lịch
  sử, release, run. Không tự chuyển binding hoặc sửa manifest đã publish.
- GET inventory phân trang; POST review chỉ reviewer/admin, trusted actor từ server.
  Apply yêu cầu job thành công, source/set hợp lệ, target nằm trong candidates và
  current head ID/token trùng bản pin. Không tự rebase khi QA đã sửa.
- Apply create/revise sinh DRAFT/MANUAL bằng primitive insert revision hiện có;
  không kế thừa review/automation/PASS. KEEP không tăng revision. Cùng key/body
  replay đúng response; key khác trên proposal đã quyết định bị conflict. Duplicate
  CREATE_NEW từ proposal khác bị conflict, không tạo identity trùng exact content.
  DISMISS cho phép đóng proposal stale/failed mà không đổi testcase.

Contract và ví dụ: [API](api.md#generation-proposal-review-uv-08-backend-opt-in).

## Bằng chứng tự động

- `proposal_test.go`: exact/related/unknown context/model change và bỏ bookkeeping
  khỏi context fingerprint; không dùng related candidate làm bằng chứng lineage.
- `uv08_proposal_integration_test.go`: generation chỉ tạo proposal; job chưa thành
  công không được apply; hai retry đồng thời tạo đúng một draft; khác body/key bị
  chặn; DB chặn sửa input; KEEP không thêm version; reviewer xác nhận ambiguity
  tạo v2 draft; v1/R1 và v2/R2 vẫn nguyên sau human edit/apply conflict. Source đổi
  chặn apply nhưng vẫn dismiss được. Retire giữ đủ revisions, release manifests và
  PASS của v1. Selected scope không retire; hai proposal khác job tạo cùng output
  đồng thời chỉ một thành công. Output invalid rollback batch; cancel/expired lease
  chặn persist.
- `workflow/uv08_generation_integration_test.go`: enqueue/claim/process/complete
  với testcase service và PostgreSQL thật; đổi human head sau enqueue không làm
  snapshot đổi hoặc rebase; so với target cũ, apply stale bị conflict. Process lại
  dùng proposal checkpoint, không thêm revision/proposal. Đổi mode với cùng key
  bị từ chối. Fixture deterministic, không gọi AI trả phí.
- `httpapi/testcase_proposals_test.go`: viewer đọc được, bounds pagination sai bị
  từ chối; viewer/editor không review được; reviewer được; actor/key được truyền
  từ context/header; GET vào mutation route không mutate.
- `make test lint`: Go tests/vet và frontend TypeScript check.
- PostgreSQL integration toàn bộ `./internal/...`, tuần tự package với `-p 1`.
- Fresh migration 1–29 trên database test riêng; down 29/up 29 trên database riêng
  **trống**. Đây không phải bằng chứng rollback production có dữ liệu.

Chạy lại từ repo root (database test riêng đã migrate tới 29, không có worker nền):

```sh
make test lint
cd backend
TEST_DATABASE_URL='<test PostgreSQL URL>' \
  go test -count=1 -p 1 -tags=integration ./internal/...
```

Database kiểm thử lần này: `uv08_proposals_20260923`; fixture proof được giữ lại,
không để active jobs rơi sang lần claim sau. Database down/up trống:
`uv08_migration29_empty_20260923`. Không nâng schema database ứng dụng chính.

## Rollout và phần còn thiếu

1. Backup và chạy migration 29 trước khi bật API mode mới; deploy API và worker
   cùng bản code. Không dùng rollback migration để restart. Down mất proposals/
   receipts nhưng không xóa testcase revisions đã apply; xem [Database](database.md).
2. Omit/false `review_proposals` giữ direct-draft legacy. UI hiện chưa gửi mode mới;
   do đó backend này chưa làm luồng người dùng mặc định thành proposal-first.
3. Còn UI scope/coverage/budget summary, diff/source, explicit apply và publish R2;
   default affected cases chưa có. Mapping không chắc vẫn phải được báo ambiguity.
4. Còn retry từng failed requirement unit. Hiện chỉ một operation unit: toàn batch
   persist xong có checkpoint; crash trước commit có thể gọi lại AI và phát sinh
   phí. Không tuyên bố exactly-once provider hay resume incremental. Source validation
   vẫn chạy trước checkpoint reuse; source stale cần job mới, proposal cũ vẫn đọc được.
5. All-removed-only baseline chưa enqueue được vì còn guard ít nhất một approved
   current requirement. Retire cho mixed removed/current source chưa tự suy diễn.
6. Còn browser journey đầy đủ R1/run/export → cập nhật nguồn → review/apply → R2,
   real provider/SCM/sandbox E2E và populated rollback/purge drill. Không đóng các
   gate usability/screen reader UV-05/07 từ kết quả backend này.
