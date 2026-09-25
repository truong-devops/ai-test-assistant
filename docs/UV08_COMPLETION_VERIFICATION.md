# UV-08 — Nghiệm thu regenerate có phạm vi

Ngày kiểm tra: 24/09/2026. **Hoàn thành kỹ thuật UV-08**, schema 30.
Tiếp nối các mốc [nền](UV08_GENERATION_FOUNDATION_VERIFICATION.md),
[proposal backend](UV08_PROPOSAL_VERIFICATION.md) và
[UI ban đầu](UV08_PROPOSAL_UI_VERIFICATION.md). Các giới hạn ở ba mốc này là lịch sử;
trang này ghi trạng thái hiện tại. UV-05/07 human acceptance và UV-09 vẫn mở.

## Contract hoàn tất

- UI hai trang testcase luôn sinh proposal trước; reviewer quyết định riêng.
  ALL cho bộ mới; AFFECTED mặc định khi đã có identity active; SELECTED chọn tối đa
  100 requirement. Không tự gọi AI khi mở trang; vẫn phải xác nhận phạm vi.
- AFFECTED lấy requirement current/approved/unblocked chưa được head active liên
  kết, hoặc head liên kết cả nguồn không còn current. Summary có comparison nguồn,
  số yêu cầu/case ảnh hưởng, removed và blocked, cùng usage/budget theo quyền.
  Đây là ảnh hưởng theo quan hệ đã lưu, không phải suy đoán semantic đầy đủ.
- Exact content, ordered steps, test data, citations và generation context mới
  đủ phân loại UNCHANGED. Liên quan requirement/ancestry/comparison chỉ tạo candidate
  AMBIGUOUS_MATCH; reviewer xác nhận REVISE hay CREATE_NEW, không tự nối lineage.
- ALL/AFFECTED có checkpoint retire không gọi provider. Tất cả nguồn của identity
  phải được xác nhận REMOVED; việc bỏ khỏi SELECTED không có nghĩa removed.
  Baseline không còn requirement approved vẫn chạy retire, kể cả hết ngân sách AI.
  Archive không xóa testcase/release/run/export và không đổi project binding.
- Mỗi requirement có checkpoint bền vững. Proposal batch của unit và trạng thái
  SUCCEEDED commit trong cùng transaction. Unit `operation` chỉ điều phối queue;
  tổng tiến độ không tính coordinator. Unit retire có thể thành công với 0 proposal.
- Lỗi một phần giữ proposal thành công và cho phép reviewer xử lý unit đó ngay
  trong PARTIAL_FAILED. Retry chỉ chạy unit chưa thành công, giữ quyết định human,
  input snapshot và head pin. UI hiển thị checkpoint, attempt và lỗi từng unit.
- Claim revision/attempt/lease/cancel chặn worker cũ ghi hoặc heartbeat vào lần
  retry mới. Checkpoint không hứa provider exactly-once: trước commit, request có
  thể lặp và tốn phí; reservation/usage vẫn gắn job/unit/attempt.
- Apply kiểm tra nguồn/current approval và CAS head. Lost response dùng lại receipt;
  conflict giữ lựa chọn/lý do, không rebase. Draft mới không kế thừa approval,
  automation hoặc PASS. Publish R2 vẫn qua review/preview riêng.
- Đổi nguồn làm snapshot cũ stale: phải xác nhận job mới, không nới phạm vi retry.
  API legacy không gửi `review_proposals` vẫn giữ direct-draft compatibility.

## Bằng chứng đã chạy

| Kiểm tra | Kết quả và phạm vi |
| --- | --- |
| `make test lint` | PASS: TypeScript, backend/sample unit tests, Go vet |
| `npm --prefix frontend run build` | PASS: production Next.js build |
| PostgreSQL `go test -count=1 -p 1 -tags=integration ./internal/...` | PASS trên DB riêng `uv08_proposals_20260923`, schema 30 |
| Migration | PASS fresh 1–30 trên `uv08_migration30_fresh_20260923`; 29→30→29→30 trên DB trống `uv08_migration29_empty_20260923` |
| Browser | PASS script `scripts/uv08-proposal-ui-e2e.cjs`, API/worker thật + production UI reviewer/viewer, `LLM_PROVIDER=disabled`, schema 30 |

`uv08_completion_integration_test.go` kiểm tra hai requirement/case độc lập:
unit thứ hai lỗi có kiểm soát, unit đầu được KEEP rồi human edit; retry chỉ gọi lại
unit lỗi, không ghi đè head/decision. Worker của claim cũ không heartbeat/fail/process
được claim mới. Thay một requirement chỉ đưa đúng requirement/family vào affected
scope, không đưa case không liên quan vào candidate. Review/publish R2 rồi removed
toàn bộ vẫn giữ manifest R1, bytes/hash XLSX và run PASSED cũ; revision mới không
có execution. Run PASSED này là fixture SQL để kiểm tra retention, không phải một
lượt chạy sandbox thật. Budget-exhausted dùng usage ledger fixture, không bỏ constraint.

Các test proposal trước đó vẫn kiểm tra immutable input/decision, exact/ambiguous
classification, CAS, stale source/cancel guards, idempotency, cùng test type khác
steps/data và comparison đọc frozen revision sau human edit.

Browser đi qua scope all/selected, mất response sau commit và retry cùng key,
KEEP không tăng revision, head conflict giữ reason, DISMISS, explicit REVISE;
sau đó upload source v2 với stable requirement ID, extract/approve, kiểm tra
AFFECTED mặc định, diff/apply draft v4, duyệt exact v4 và preview/publish R2 bằng UI.
R1 manifest/items và bytes XLSX không đổi. Source v3 chỉ có heading (không còn yêu
cầu) được extract thật; UI sinh RETIRE_CANDIDATE và ARCHIVE, giữ cả R1/R2/history.
Kiểm tra thêm viewport 390 px với diff mở, không pageerror, viewer chỉ đọc và POST 403.

Fixture browser đầy đủ đầu tiên: set 11, jobs 75–86, R1 id 12; database riêng
`uv08_browser_20260923`. Screenshots local nằm trong
`/tmp/ai-test-uv08-ui.BVmtg0/completion-evidence/`: `head-conflict.png`,
`proposal-applied.png`, `source-update-r2.png`, `mobile.png`, `viewer-readonly.png`,
`all-removed-archive.png`. Artifacts chưa được lưu Git/CI. Fixture thất bại ban đầu
được giữ để audit: paragraph không có stable ID được nhận là ADDED/REMOVED, không
tự suy ra lineage chỉ vì heading giống nhau. Fixture cuối dùng bảng mã YC rõ ràng.

Lượt chạy lặp lại cũng PASS: set 12, jobs 90–100, R1 id 14; screenshots riêng tại
`/tmp/ai-test-uv08-ui.BVmtg0/completion-evidence-repeat/`.

## Chạy lại

Chỉ chạy trên database/storage kiểm thử riêng, không dùng DB có worker khác đang
claim test jobs. Migrate đến 30, chạy API/worker cùng cấu hình `LLM_PROVIDER=disabled`;
UI reviewer tại 3188, tùy chọn UI viewer 3189, cùng backend URL/token test.

```sh
make test lint
npm --prefix frontend run build
cd backend
TEST_DATABASE_URL='<isolated schema-30 PostgreSQL URL>' \
  go test -count=1 -p 1 -tags=integration ./internal/...
cd ..
PLAYWRIGHT_MODULE='<installed playwright package path>' \
UV08_BASE_URL=http://127.0.0.1:3188 \
UV08_VIEWER_URL=http://127.0.0.1:3189 \
UV08_ARTIFACT_DIR=/tmp/uv08-completion-evidence \
node scripts/uv08-proposal-ui-e2e.cjs
```

Script giữ fixture và trả JSON PASS/IDs để truy vấn lại. Chuẩn hóa Playwright
dependency/CI và lưu artifacts lâu dài thuộc UV-09.

## Triển khai và giới hạn

1. Backup DB/storage; drain worker cũ, chạy migrations 29 rồi 30 trước code mới.
2. Deploy API/worker/frontend đồng bộ. Job snapshot cũ vẫn chạy mode cũ; chỉ job
   proposal mới có per-requirement checkpoints. UI mới cần comparison và scope API.
3. Kiểm tra readiness, read-only scope, generation/partial retry trên môi trường
   staging. Không đổi release/project binding ngầm khi cutover.
4. Rollback cần dừng writer/worker và code tương thích. Down 30 làm mất unit link;
   down 29 xóa proposal/receipts, không xóa revision đã apply. Không dùng down
   migrations để rollback production đã có dữ liệu mới nếu chưa có kế hoạch khôi phục.

Chưa migrate database ứng dụng chính (vẫn schema 28), chưa deploy production,
không gọi provider trả phí hoặc tự commit. Chứng cứ browser dùng deterministic
provider, không thay cho UV-09 real SCM/LLM/sandbox demo, populated migration/rollback
drill, usability người mới hoặc VoiceOver/NVDA acceptance.
