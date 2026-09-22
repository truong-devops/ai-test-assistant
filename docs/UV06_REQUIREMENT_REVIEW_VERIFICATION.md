# UV-06 — Requirement batch review và đối chiếu nguồn

Ngày kiểm tra: 21–22/09/2026. Trạng thái: hoàn thành kiểm thử kỹ thuật UV-06.

## Phạm vi triển khai

- Duyệt 1–100 exact revision/hash, kết quả riêng từng mục, audit bằng actor của
  server. Receipt từng mục lưu cùng transaction; retry sau partial success không
  tạo lại quyết định. Foreign-set, stale hash, conflict/TBD và thiếu evidence bị chặn.
- Conflict/question phải có nội dung giải quyết, lưu history và cập nhật counts.
  Giải quyết chỉ đưa về DRAFT, không tự approve; sửa title/risk hoặc reject kèm edit
  không lách được điều kiện. Người duyệt chịu trách nhiệm nội dung câu trả lời.
- Migration 28 bảo vệ nội dung, evidence và steps của requirement đã review hoặc
  đã được testcase tham chiếu. Edit tạo revision mới và UI mở đúng ID trả về.
- Extraction lưu theo source snapshot/version, không append citation vào proof cũ.
  Chỉ lượt extraction hoàn tất và còn đúng generation/source revision mới thay
  inventory hiện hành. Lượt cũ hoặc dở dang không thay phạm vi đang dùng.
- So sánh stable identifier + tài liệu logic + flow, lấy revision mới nhất trong
  chuỗi review/edit. Lưu ADDED/CHANGED/REMOVED/UNCHANGED/AMBIGUOUS kèm lý do và
  testcase liên quan. Không tự ghép nhiều ứng viên hoặc chuyển approval sang nguồn mới.
- Testcase có linked requirement cũ được cảnh báo `needs_source_review`; chặn
  publish release mới khi requirement bị chặn. Release/run cũ không đổi proof.
- Workspace 20 mục/trang, chọn rõ từng trang (tối đa 100), xác nhận nhóm một lần,
  evidence drawer, duyệt-next, filter/scroll, tab làm rõ/history và đối chiếu nguồn.
  Legacy URL vẫn hoạt động. Nút sinh testcase nêu số approved/unresolved/ngoài scope.

## Kiểm tra tự động

| Gate | Kết quả / phạm vi |
| --- | --- |
| `make test` | Đạt: TypeScript, Go unit và sample services |
| `make lint` | Đạt: Go vet |
| Frontend production build | Đạt |
| PostgreSQL integration `-count=1 -p 1 -tags=integration ./internal/...` | Đạt, schema 28 |
| UV06 requirement integration | Batch 20 mục, foreign/TBD/stale, replay và key conflict, concurrent review, resolution/counts, edit bypass, SQL proof guards, nguồn v2 với đủ 5 classifications |
| UV06 testcase integration | Case bị ảnh hưởng giữ content hash, publish mới bị chặn, release manifest và run snapshot cũ vẫn nguyên |
| HTTP authorization | Reviewer-only mutations; GET không mutate; viewer đọc history/comparison; trusted actor |
| Migration | Fresh 1–28 và down/up 28 trên database riêng còn trống; không phải rollback drill production có dữ liệu |
| Playwright Chromium | Đạt: 20-item batch, evidence, không auto-select, filter/reload, giải quyết TBD/counts, edit mở ID mới và proof cũ giữ nguyên, tab đối chiếu, 390 px không tràn trang, không có pageerror |

Script browser: `scripts/uv06-requirement-review-e2e.cjs`. Browser dùng API, worker
và production frontend với PostgreSQL riêng `uv06_browser_20260921`, storage riêng,
`LLM_PROVIDER=disabled`; không gọi provider trả phí. Các fixture được giữ lại để
kiểm tra. Không chạy script vào production.

Browser phát hiện và đã sửa: prefetch từng dòng gây burst request/429; refresh
toàn route không cần thiết khi requirement workspace đã polling JSON; điều hướng
client sau edit giữ form cũ; grid min-width làm tràn màn hình hẹp. Liên kết từng
dòng không prefetch, revision mới được tải bằng URL chính thức do API trả về,
bảng hẹp cuộn nội bộ. Không nới rate limit hay CSP để làm test đạt.
Fixture set 9 và 10 chạy đạt liên tiếp trên bản production ngày 22/09; ảnh local tại
`/tmp/ai-test-uv06.CrzFGE/evidence/batch-confirmation.png` và `mobile-review.png`.

## Chạy lại và triển khai

Áp dụng migrations 1–28 lên database kiểm thử. Chạy API/worker cùng `DATABASE_URL`,
`DOCUMENT_STORAGE_PATH`, `LLM_PROVIDER=disabled`; frontend production trỏ API đó,
với token tương ứng, role reviewer và actor kiểm thử. Giữ rate limit/CSP mặc định.
Cần Chromium và Playwright đã cài.

```sh
make test
make lint
cd backend
TEST_DATABASE_URL='<test PostgreSQL URL>' go test -count=1 -p 1 -tags=integration ./internal/...
cd ../frontend
npm run build
# Chạy production frontend/API/worker ở terminal riêng, sau đó từ repo root:
cd ..
UV06_BASE_URL=http://127.0.0.1:3186 \
PLAYWRIGHT_MODULE='<path to installed playwright package>' \
UV06_ARTIFACT_DIR=/tmp/uv06-browser-evidence \
node scripts/uv06-requirement-review-e2e.cjs
```

Triển khai migration 28 trước API/worker/frontend tương ứng. Không chạy down để
restart ứng dụng: down mất receipts, clarification audit và comparison metadata.
Xem [database](database.md) và [API](api.md) cho contract và rollback costs.

## Giới hạn được giữ rõ

- Chưa thay thế kiểm thử provider thật/staging, SCM/sandbox E2E của kế hoạch chính,
  đa browser, screen reader hay buổi usability với người mới của UV-05.
- AMBIGUOUS được đưa ra để đọc evidence hai phía, không có màn cấu hình mapping
  thủ công hoặc chuyển approval hàng loạt. Regenerate theo phạm vi là UV-08.
- Comparison là báo cáo dẫn xuất có thể cập nhật khi extraction lại cùng snapshot,
  không phải baseline đã publish. Requirement/evidence, release và run proof cũ
  vẫn được bảo vệ riêng.
- Legacy single-review API vẫn chấp nhận client cũ không gửi hash; UI mới và bulk
  luôn gửi hash. Guards current/evidence/clarification vẫn áp dụng cho client cũ.
- Ảnh browser dưới `/tmp` là artifact cục bộ tạm, không phải evidence lưu trong Git.
- Bước phát triển tiếp theo: UV-07 — testcase history/diff/edit/restore/release UI.
