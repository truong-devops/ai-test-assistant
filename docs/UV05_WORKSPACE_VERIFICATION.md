# UV-05 — Bàn giao workspace và bằng chứng kiểm tra

Ngày: 21/09/2026. Trạng thái: triển khai kỹ thuật; chưa nghiệm thu usability với
người mới. Không đánh dấu hoàn thành toàn bộ UV-05 trước buổi nghiệm thu đó.

## Phần đã triển khai

- `/documents/{id}` có bốn bước Tài liệu → Yêu cầu → Testcase → Sử dụng / Xuất,
  counts, trạng thái, CTA; tạo bộ xong mở workspace, field phụ thu gọn.
- Upload nhiều file với loại/progress/lỗi/retry riêng. Upload version dùng đúng
  document ID, giữ danh tính khi đổi filename; history trả đủ phiên bản.
- Preview heading/table/locator cạnh danh sách; phân biệt current/history,
  version và approval. Không mặc định chọn tất cả nguồn.
- Duyệt theo exact version/hash/status và source revision, xác nhận phạm vi
  gồm/loại; quyền lấy từ server, tên hiển thị không thay authenticated actor.
- Migration 27 lưu source intent cùng upload/approval. Worker tự parse/index;
  extraction chỉ chạy sau command rõ ràng. Reload/rời trang không mất intent.
  Retry job lỗi tiếp tục intent; nguồn thay đổi trước extraction thì supersede,
  không tự duyệt hoặc gửi version mới vào AI.
- Empty/loading/error, polling, focus và layout hẹp; inspector/lỗi kỹ thuật thu
  gọn, quản lý archive/retention/budget tách khỏi nguồn. Deep link cũ được giữ.

## Kiểm tra đã chạy

| Gate | Kết quả / phạm vi |
| --- | --- |
| `make test` | Đạt: frontend typecheck, Go unit, sample services |
| `make lint` | Đạt: Go vet |
| `cd frontend && npm run build` | Đạt production build |
| PostgreSQL integration toàn bộ `./internal/...` | Đạt, schema 27, `-count=1 -p 1 -tags=integration` |
| `TestUV05SourceUploadReviewAndDurableContinuation` | Đạt lặp 2 lần; ownership, duplicate new-name, role/hash/revision guards, idempotency/audit, retry continuation, fresh service, real parser/index/extractor, atomic rollback, explicit exclusion và superseded source |
| Playwright Chromium, production frontend + API + worker | Đạt: tạo bộ, 4 bước, Markdown tốt + DOCX lỗi, preview table, loại rõ file lỗi, approve/extract qua reload, v2 đổi filename, history v1, deep link, 390 px không tràn trang, keyboard navigation; không có pageerror |

Browser dùng database riêng `uv05_browser_20260920`, storage riêng dưới `/tmp`,
`LLM_PROVIDER=disabled` (deterministic extractor), không gọi provider trả phí.
Sau khi sửa cập nhật UI, browser đạt 3 lượt liên tiếp (fixture set 7, 8, 9);
lượt set 9 dùng production build cuối cùng của đợt triển khai này.
Ảnh local: `/tmp/ai-test-uv05.tt4zf0/evidence/desktop-source-review.png` và
`mobile-workspace.png`; đây là artifact tạm, không phải evidence được lưu lâu dài
trong Git. Script tái tạo nằm ở `scripts/uv05-workspace-e2e.cjs`.

Các lần browser ban đầu gặp timeout khi chờ UI upload/chuyển bước dù server đã
có dữ liệu. Nguồn và stepper nay polling read-only JSON trực tiếp, không phụ thuộc
refresh toàn route sau mỗi file; inventory server chỉ refresh khi workflow đổi.
Polling có abort/giới hạn request chồng nhau và tạm ngừng khi chọn bước khác.
Không coi smoke test đạt là bằng chứng chống mọi race hoặc mọi lỗi accessibility.

## Chạy lại

Áp dụng đủ migrations 1–27 lên database kiểm thử riêng. Chạy API và worker cùng
`DATABASE_URL`, `DOCUMENT_STORAGE_PATH`, `LLM_PROVIDER=disabled`. Chạy frontend
production với `BACKEND_API_URL` trỏ API đó; nếu bật auth thì cấu hình token giống
API, role `reviewer` và actor kiểm thử. Cần Chromium/Playwright cài sẵn.

```sh
make test
make lint
cd backend
TEST_DATABASE_URL='<test PostgreSQL URL>' go test -count=1 -p 1 -tags=integration ./internal/...
cd ../frontend
npm run build
# Chạy production frontend/API/worker ở terminal riêng, sau đó từ repo root:
cd ..
UV05_BASE_URL=http://127.0.0.1:3185 \
PLAYWRIGHT_MODULE='<path to installed playwright package>' \
UV05_ARTIFACT_DIR=/tmp/uv05-browser-evidence \
node scripts/uv05-workspace-e2e.cjs
```

Script tạo fixture mới và giữ lại để kiểm tra, nên không chạy vào dữ liệu production.
Frontend dev hiện bị policy CSP chặn eval trong môi trường kiểm thử này; browser
smoke dùng production build, không nới CSP chỉ để chạy test.

## Triển khai và việc còn lại

Áp dụng migration 27 trước khi khởi động API/worker mới; cần nâng cả hai cùng
frontend. Chỉ restart frontend sẽ không có durable continuation. Không chạy
migration down tùy tiện vì sẽ mất các intent đang chờ.

- Mời người chưa biết hệ thống tự tạo bộ, upload, tìm nơi xem/duyệt và mô tả
  bước tiếp theo mà không hướng dẫn miệng; ghi nhận vướng mắc trước khi đóng UV-05.
- Chưa kiểm tra provider thật/staging, rollback drill migration 27, đa browser,
  screen reader hoặc usability toàn luồng requirement → testcase → release.
- UV-06 tiếp tục bulk review requirement và đối chiếu nguồn; UV-07 hoàn thiện
  UI version testcase. Không gộp các hạng mục này vào nghiệm thu UV-05.
