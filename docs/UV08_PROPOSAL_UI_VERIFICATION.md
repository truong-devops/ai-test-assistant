# UV-08 — Giao diện proposal-first

Ngày triển khai: 23/09/2026. Tiếp nối [backend proposal](UV08_PROPOSAL_VERIFICATION.md).
Đây là mốc kiểm chứng UI ban đầu trên schema 29, khi UV-08 chưa hoàn thành.
**Trạng thái hiện tại:** xem [nghiệm thu UV-08](UV08_COMPLETION_VERIFICATION.md)
(schema 30, affected mặc định, per-unit retry, all-removed và E2E đổi nguồn).
Các mô tả/giới hạn bên dưới được giữ như bằng chứng lịch sử của chặng UI đầu tiên.

## Đã triển khai

- Cả workspace bước Testcase và trang coverage dùng chung giao diện generation,
  luôn gửi `review_proposals:true`. Không còn nút generation chính âm thầm ghi
  draft trực tiếp; API legacy vẫn giữ compatibility cho client cũ.
- Chọn rõ all-approved hoặc selected requirements (tối đa 100); không tự chọn
  scope khi mở trang. Tìm kiếm, 20 requirement/trang và giữ selection ngoài trang.
  Source revision, số yêu cầu đủ điều kiện và revision cần đối chiếu được hiển thị;
  link sang đối chiếu nguồn. Người có quyền xem budget thấy usage/reservation/
  remaining thực, không hiển thị dự đoán token giả chính xác.
- Xác nhận phạm vi trước gửi; source/eligibility thay đổi sẽ yêu cầu xác nhận lại,
  refresh không đổi dữ liệu không tự xóa xác nhận. Job polling giữ nguyên luồng
  retry/cancel; trạng thái terminal nạp lại inventory proposal, không auto-apply.
- Inventory proposal phân trang server 20 mục, lọc job, đọc đủ năm classifications,
  reason/status và mở nội dung, citations, source links, provenance.
- API GET comparison kiểm tra candidate family rồi tải exact revision content/
  frozen citations. UI diff trước/sau không dùng live head hoặc evidence dựng lại
  từ requirement hiện tại. Đây là display, không thay quyền quyết định backend.
- Reviewer chọn decision, target khi cần, reason và xác nhận. Không tự ghép identity
  mơ hồ; head ID gửi luôn là bản đã pin. Create/revise chỉ tạo DRAFT/MANUAL; KEEP
  không tạo revision; archive/dismiss có nhãn rõ. Link kết quả mở exact revision
  để duyệt riêng; publish/bind vẫn qua các guards hiện hữu.
- Cùng body giữ idempotency key khi mất phản hồi. Conflict giữ reason/selection,
  chặn apply tiếp, cho xem current head hoặc dismiss/sinh job mới; không tự rebase.
  Viewer chỉ đọc; quyền mutation vẫn do API thực thi.
- Giao diện nói rõ retry hiện là cả operation, không phải failed requirement unit.
  Không suy diễn summary thay đổi nguồn/default affected scope chỉ từ count.

## Kiểm thử và cách chạy lại

- `make test lint`: TypeScript, backend/sample unit tests và Go vet.
- `npm run build` trong `frontend`: production build.
- PostgreSQL integration toàn bộ `./internal/...` trên schema 29, `-p 1`;
  bổ sung comparison giữ target cũ sau human edit và từ chối family ngoài candidate.
- HTTP: viewer GET comparison, invalid/missing target 400, review role guards.
- Browser script: `scripts/uv08-proposal-ui-e2e.cjs`. Script tạo fixture và giữ lại
  dữ liệu, chỉ chạy với API/worker/production UI trên database test riêng, provider
  disabled. Upload/extract/requirement approval, human edit và R1 dùng API fixture;
  scope generation, diff, create/keep/dismiss/revise/conflict đi qua browser.

```sh
make test lint
cd backend
TEST_DATABASE_URL='<isolated schema-30 PostgreSQL URL for current code>' \
  go test -count=1 -p 1 -tags=integration ./internal/...
cd ../frontend
npm run build
# Chạy API/worker và frontend trỏ cùng database/storage kiểm thử, LLM_PROVIDER=disabled.
# Frontend 3188 dùng role reviewer; frontend 3189 tùy chọn dùng role viewer.
cd ..
PLAYWRIGHT_MODULE='<installed playwright package path>' \
UV08_BASE_URL=http://127.0.0.1:3188 \
UV08_VIEWER_URL=http://127.0.0.1:3189 \
UV08_ARTIFACT_DIR=/tmp/uv08-browser-evidence \
node scripts/uv08-proposal-ui-e2e.cjs
```

Browser checks gồm: generation chưa đổi testcase; mất response sau server commit
và retry cùng key tạo đúng một draft; KEEP không tăng version; head đổi trả conflict
và giữ reason; dismiss stale; xác nhận ambiguity tạo successor draft; R1 vẫn pin
v1; cả hai entry pages có UI mới; viewport 390 px không tràn; không pageerror.
Khi truyền `UV08_VIEWER_URL`, kiểm tra thêm viewer xem pinned diff nhưng không có
mutation control, POST giả lập bị API trả 403.

Lượt kiểm thử local đạt trên database riêng `uv08_browser_20260923`, migrations
1–29, API/worker thật và Next.js production; provider disabled, không gọi AI trả
phí. Fixture set 6 và 7 đã qua core journey và viewer; set 7 kiểm tra thêm diff
đang mở ở 390 px, manifest R1 và draft không kế thừa execution. Artifacts được giữ tại
`/tmp/ai-test-uv08-ui.BVmtg0/evidence/` gồm `head-conflict.png`,
`proposal-applied.png`, `mobile.png`, `viewer-readonly.png`. Đây là bằng chứng tạm
trên máy, không phải artifact đã lưu Git/CI. Các lượt chưa đạt được giữ để audit;
không xóa dữ liệu ứng dụng chính. Backend integration dùng database riêng
`uv08_proposals_20260923`, không chạy trên database có worker browser đang claim.

## Triển khai và giới hạn

- Không thêm migration ngoài 29 của chặng backend. Cần migrate 29 và deploy API,
  worker, frontend đồng bộ; frontend mới cần cả list/review và comparison endpoint.
- Không dùng mode all như một default affected giả. Summary hiện chưa bao phủ
  hết mapping mơ hồ, chưa chỉ chọn case bị ảnh hưởng tự động. Selected mode không
  suy diễn removed từ yêu cầu không được chọn.
- Chưa giải quyết all-removed-only baseline; chưa có partial per-unit checkpoint/
  retry. Generation còn một operation unit, có thể gọi lại provider trước commit.
- Browser fixture không đổi tài liệu nguồn, không chạy sandbox hoặc dựng toàn bộ
  hành trình R1/run/export → source change → R2. Các gate này vẫn mở.
- Chưa nghiệm thu VoiceOver/NVDA/người mới; không đóng UV-05/07 human acceptance.
- Playwright hiện dùng package cài ngoài; dependency/CI lane chuẩn hóa thuộc UV-09.
- Không nâng database ứng dụng chính hoặc commit tự động trong chặng này.
