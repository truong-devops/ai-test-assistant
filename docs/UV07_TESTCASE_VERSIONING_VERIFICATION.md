# UV-07 — Testcase version workspace

Ngày kiểm tra: 22/09/2026. Code và kiểm thử kỹ thuật đã đạt; chưa đóng toàn bộ
Definition of Done vì còn nghiệm thu người dùng và screen reader. Thay đổi đang
ở working tree, chưa tạo commit/PR. Không có migration 29; dùng schema 28.

## Đã triển khai

- Inventory theo identity, latest/latest-approved/pinned, cảnh báo source và
  trạng thái run đúng revision; filter/search/sort, 20 mục/trang, giữ context.
- Form title/actor/precondition/data/steps/expected/postcondition/risk/type;
  save draft tách khỏi approve/reject. Save mở đúng ID trả về. Link/reload có
  cảnh báo dữ liệu chưa lưu. Head conflict 409 giữ form, hiển thị diff head và
  form, chỉ đổi base sau xác nhận rõ ràng; không tự merge.
- Timeline có creator/reason/source/provenance khi có, status, release refs và
  nguồn restore. Diff đọc được nội dung field, thứ tự steps và citation, không
  chỉ hiển thị hash. History/diff phân trang tối đa 50 mục/lần.
- Restore cần reason và diff preview, tạo draft mới. Archive không xóa history.
  Bulk review xác nhận exact IDs/hashes; kết quả riêng từng mục, giữ mục lỗi để
  retry, không tự chọn unseen revisions. Audit dùng actor server.
- Release chọn từng approved revision, kể cả bản cũ khi latest là draft; preview
  coverage/source/manifest trước publish. Publish kiểm tra lại preview hash và
  các guards trong transaction; nguồn đổi sau preview trả conflict.
- Run/automation mặc định theo exact revision; identity-wide runs là lựa chọn
  riêng. Draft mới không sao chép PASS/artifact approved và không giữ readiness
  `AUTOMATED`. Export revision/release giữ thông tin version và immutable proof.
- Polling trạng thái không refresh toàn trang làm mất form/preview. Các liên kết
  inventory không prefetch hàng loạt gây burst request. Không nới rate limit/CSP.

## Bằng chứng kiểm thử

| Gate | Kết quả / phạm vi |
| --- | --- |
| `make test` | Đạt: TypeScript, Go unit, sample services |
| `make lint` | Đạt: Go vet |
| Frontend production build | Đạt |
| PostgreSQL integration `-count=1 -p 1 -tags=integration ./internal/...` | Đạt, schema 28 |
| `uv07_workspace_integration_test.go` | History/diff bounds/order/cross-family, review replay, source/hash preview conflict, expected thiếu nguồn, v1/v2/R2/restore, exact/family runs, inventory nhận run đúng head không kế thừa PASS, archive giữ proof |
| HTTP tests | Viewer đọc nhưng không preview/publish; reviewer preview với trusted actor; pagination/scope/path sai bị chặn; lỗi field 422 và preview 409 |
| Historical compatibility unit | Giới hạn editor không làm normalization/read dữ liệu cũ quá 16000 bytes thất bại |
| Playwright Chromium | Exact batch approval → R1 → edit v2 draft → ordered diff → approve → R2 → restore v1 thành v3 draft; R2 không đổi; no inherited run; Markdown export; 390 px không tràn; keyboard cơ bản; archive giữ history; không pageerror |
| Concurrent browser tabs | Hai tab mở v3; tab khác lưu v4; tab đầu nhận 409, giữ title chưa lưu, đối chiếu/rebase rõ ràng rồi lưu v5; R2 manifest vẫn nguyên |

Script: `scripts/uv07-testcase-versioning-e2e.cjs`. Fixture setup upload/extract/
requirement review/generate dùng API để chuẩn bị; các thao tác testcase từ v1 tới
R2/restore/conflict/archive đi qua browser, không SQL/API mutation thủ công.

Chạy local với PostgreSQL riêng `uv07_browser_20260922`, migrations 1–28, API/worker
thật, Next.js production và `LLM_PROVIDER=disabled`; không gọi provider trả phí.
Fixture set 3 chạy đạt core journey; set 4 và 5 chạy đạt cả conflict v4/v5. Storage và
ảnh/export được giữ dưới `/tmp/ai-test-uv07.FokHIG/`, ảnh ở `evidence/` gồm release
preview, revision diff, mobile và concurrent-head-conflict. Đây là artifact tạm
cục bộ, không phải evidence đã lưu trong Git hay production.

## Chạy lại

Không chạy script vào production: nó tạo và giữ fixture, publish release và archive
identity kiểm thử. Cần database riêng, Chromium và Playwright đã cài. Chạy API/worker
cùng database/storage, provider disabled; frontend production trỏ API với token
tương ứng, role reviewer và actor kiểm thử. Giữ rate limit/CSP mặc định.

```sh
make test lint
cd backend
TEST_DATABASE_URL='<test PostgreSQL URL>' go test -count=1 -p 1 -tags=integration ./internal/...
cd ../frontend
npm run build
# Chạy API/worker và production frontend ở terminal riêng, sau đó từ repo root:
cd ..
UV07_BASE_URL=http://127.0.0.1:3187 \
PLAYWRIGHT_MODULE='<path to installed playwright package>' \
UV07_ARTIFACT_DIR=/tmp/uv07-browser-evidence \
node scripts/uv07-testcase-versioning-e2e.cjs
```

Triển khai API/frontend đồng bộ trên schema 28; worker hiện hành vẫn cần cho
workflow setup. Không chạy migration down để restart ứng dụng.

## Giới hạn và nghiệm thu còn mở

- Chưa có người mới xác nhận phân biệt testcase vN, automation artifact và release
  RN. Đã có nhãn giải thích; nhãn không thay thế usability acceptance.
- Có native buttons/labels, table caption/headers và keyboard smoke; chưa chạy
  VoiceOver/NVDA hoặc kiểm tra toàn hành trình chỉ bằng keyboard. Giữ gate mở.
- Cảnh báo unsaved hiện bao phủ link và unload/reload; không cam kết mọi trường
  hợp App Router back/forward hoặc phục hồi draft sau browser crash.
- Inventory identity vẫn tải toàn bộ rồi phân trang client. History/diff giới hạn
  số mục trả về, release refs tối đa 50/revision; diff vẫn đọc hai revision đầy đủ
  bên trong. Không tuyên bố đã giải quyết mọi payload lịch sử lớn hay tải cực lớn.
- Editor giữ requirement/citations của base; không tự chế expected mới. Đổi nguồn
  và regenerate có đối chiếu thuộc UV-08, không được tự đổi release đã publish.
- Client legacy có thể publish không có preview hash; UI mới luôn gửi hash.
  Batching UI dùng từng exact-review request, không phải transaction cả nhóm.
- Browser script dùng Playwright cài ngoài; dependency/CI browser lane chuẩn hóa
  vẫn là UV-09. Chưa thay thế real-provider/SCM/sandbox E2E hay UV-05 usability.

Bước code kế tiếp: UV-08. UV-07 chỉ được đóng hoàn toàn sau các gate thủ công trên.
