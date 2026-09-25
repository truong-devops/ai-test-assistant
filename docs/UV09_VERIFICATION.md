# UV-09 — Kiểm thử tích hợp và release gate

Ngày cập nhật: 25/09/2026. **Đang triển khai, chưa đóng UV-09, chưa cho phép rollout
mặc định chỉ dựa trên các kết quả local này.** Schema mới nhất là **31**; chỉ áp dụng
trên DB test trong lượt này, không migrate database chính.

## Đã triển khai và kiểm tra

- Pin `@playwright/test` 1.63.0 trong package/lockfile; frontend/test runner yêu cầu
  Node.js >=20. `npm run test:e2e` chạy UV-05/06/07/08 và role/security journey, tuần
  tự, không automatic retry để che flaky failures. HTML/JUnit/screenshot; role test
  có trace khi lỗi. Script cũ được gọi nguyên luồng với backend thật, không mock
  toàn bộ API. Wrapper lưu stdout và screenshots vào report.
- Runner tự build API/worker (hoặc nhận `UV09_BIN_DIR`), khởi động ba frontend
  reviewer/viewer/editor, dùng token ngẫu nhiên, provider disabled, storage tạm riêng.
  Chỉ nhận host local/CI và database `uv09_*`, yêu cầu opt-in; từ chối port đang dùng.
  Dừng đúng process group do runner tạo khi xong/lỗi/signal; giữ fixture/storage.
  Không tự migrate database hoặc đọc root `.env.production`.
- `.gitlab-ci.yml` thêm build binary browser và lane Chromium/pgvector/API/worker
  thật, cùng artifacts `when: always`. CI config đã parse; **chưa có pipeline remote
  chạy trong lượt này**, không gọi kết quả local là bằng chứng GitLab runner.
  Thêm lane `migration-populated`: seed schema 23 rồi up đến 31, kiểm SQL read-only
  dùng chung với local drill. Không dùng empty roundtrip để thay dữ liệu legacy.
- Lane `browser-provider-faults` dùng Gemini protocol fixture chỉ ở loopback,
  fake key, không proxy ra ngoài; tách khỏi lane 5 journey provider-disabled và
  smoke provider thật. U04 kiểm 400, timeout và output enum `APPROVED` bị từ chối.
  Mỗi ca upload/review bằng UI, lỗi hiện qua polling không refresh, dừng sau đúng
  3 attempt/call, không lưu requirement sai. Reload giữ job/error/progress; UI retry
  sau khi fixture trả hợp lệ phục hồi cùng job/intent, tạo đúng một DRAFT, không
  auto-approve. Saved job JSON/screenshot và call count là evidence của fixture,
  không phải chứng minh Gemini production. Mỗi ca còn kiểm generation lỗi →
  PARTIAL_FAILED → UI retry, reload khi provider đang xử lý, rồi resume cùng job;
  giữ checkpoint, đúng một PENDING proposal, không tự apply testcase.
  HTTP 400/timeout giữ 4 reservation chưa đối soát (3 extraction + 1 generation),
  ghi nhận 80 token từ hai call thành công. Invalid enum ghi nhận đủ 240 token từ
  6 response có usage, dù nội dung bị từ chối. UI cảnh báo unknown holds đã kiểm.
- U05 có subprocess drill SIGKILL thật tại ba ranh giới: đang gọi provider,
  đã ghi usage nhưng chưa commit unit, đã commit proposal/checkpoint nhưng chưa
  complete job. Chạy production Worker/repository/service trong test executable,
  provider deterministic và barrier chỉ có trong test, không hook production.
  Worker mới reclaim sau lease hết hạn, không nhân proposal, không ghi đè quyết
  định DISMISSED và từ chối heartbeat attempt cũ. TTL reservation được đẩy về
  quá hạn chỉ trên fixture; lease takeover dùng thời gian thực.
- Sửa budget: mọi RESERVED vẫn giữ hạn mức sau expiry/unknown provider error;
  chỉ lỗi local `ErrDisabled` tự release. Timeout/400 không được coi là miễn phí.
  `unreconciled_reservations` đi qua manager → document service → API → UI;
  workflow và budget endpoint cùng tính khoản giữ. Finalize một lần theo usage,
  lần thứ hai bị từ chối; test chứng minh hết hạn không mở lại capacity.
  Không có operator reconciliation endpoint; billing receipt thật còn là gate.
- Smoke Gemini thật là lane riêng, build tag `provider_smoke`, protected/manual
  trong CI. Chỉ chạy khi `RUN_REAL_PROVIDER_SMOKE=yes`, có file secret và model;
  URL official cố định, deadline/output cap, không log credential/provider body.
  Kiểm tra JSON trả về chứ không chỉ HTTP 200. Lượt local chỉ kiểm tra compile/SKIP;
  **chưa gọi provider thật hoặc xác nhận Gemini production hoạt động**.
- Role test kiểm tra capability + UI + POST trực tiếp của viewer/editor; giả mạo
  role/actor/Authorization từ browser không nâng quyền. Editor upload được nhưng
  không duyệt/publish. Không thấy service token trong HTML/JS/storage/cookie/error
  response; UV-08 kiểm tra cả bytes/headers tải XLSX. Đây là service-role boundary,
  không chứng minh có end-user authentication hay multi-tenant ACL.
- Verifier `document-e2e-verify` thêm kiểm tra read-only/repeatable-read:
  analysis/run cùng release/suite/set/project; exact sealed revision/hash; tính lại
  ordered manifest theo origin; snapshot/scope và run expected/artifact đúng revision;
  XLSX đầy đủ run có byte/snapshot checksum, unique rows và revision manifest đúng.
  Chỉ kiểm tra export không filter; cần ít nhất một XLSX đầy đủ run. Project đổi
  sang R2 không làm verifier đọc live head thay cho R1. Legacy run không pin release
  bị báo chưa đủ proof, không tự gán release. Các kiểm tra provider/webhook/sandbox
  cũ vẫn còn và không thể bỏ qua bằng fixture browser.
  Verifier đã đọc ZIP/XML thật: đối chiếu từng ô Test Cases/Run History/Metadata
  với snapshot, gồm expected/actual/status và exact revision/release manifest;
  kiểm công thức Summary đúng range, không thực thi công thức. Kiểm root/workbook
  relationships để không đọc nhầm sheet. Không còn chấp nhận magic header `PK`.
  Chỉ hỗ trợ format inline-string do repo xuất, không phải XLSX importer tổng quát;
  tối đa 32 ZIP parts, 16 MiB/part và 64 MiB tổng giải nén/file nén. Không extract
  file hoặc fetch external relationships. Reject duplicate ZIP entries, path lạ,
  XML hỏng/trailing root/namespace sai/attribute trùng và formula trong ô dữ liệu.
  So sánh theo quy ước XML-safe, formula escaping và giới hạn 32767 rune của renderer;
  full text vẫn nằm trong snapshot. Không chứng minh Excel đã mở/recalculate file,
  không xác nhận style hiển thị hoặc sandbox/provider đã chạy chỉ từ nội dung ô.
- Unit verifier từ chối bytes/hash sai, revision/release sai, thiếu/trùng dòng,
  expected bị sửa kể cả snapshot đã được tính lại checksum. Scope integration
  chứng minh R1 sau rebind R2 vẫn pass, foreign project hoặc trộn R1/R2 bị từ chối.
- Drill schema 23 có dữ liệu → 30 → 31, migrate lại no-op, backup/restore database sang
  target mới và tar source file. Fixture gồm chain khả nghi, approved v1 bị draft v2
  che, case độc lập, project binding, run/export cũ và proposal/unit/receipt mới.
  Kiểm tra IDs/expected, migration issue, exact legacy selection, origin, không bịa
  release cho run cũ, content/export/file hash và FK validated. Giữ nguyên cả DB và
  artifacts; không restore đè bất cứ DB có sẵn nào.
- Migration 31 hiệu chỉnh timestamp chỉ cho `MIGRATED_CURRENT_STATE`, giữ giá trị
  cũ của release/items trong `test_suite_release_timestamp_audits`. Thời gian mới
  là lúc hiệu chỉnh ở migration 31, **không suy đoán thời gian chạy migration 25**.
  ID/manifest/lineage/binding/run/export và release `USER_PUBLISHED` giữ nguyên.
  Exclusive locks + transaction bảo vệ cửa sổ hiệu chỉnh; trigger immutable bật
  lại trước commit. Audit bất biến, cascade cùng release khi purge hợp lệ.
  Down 31 chỉ được phép khi không có audit; không xóa audit hay trả lại timestamp sai.
- Purge integration chạy service thật với graph schema 31 (job/unit, applied
  proposal, result revision, receipt và timestamp audit), xác nhận cleanup không vướng FK. Đây là
  dữ liệu test sau retention, không phải cho phép purge dữ liệu triển khai.

## Kết quả local

- `make test lint`: PASS; production frontend build: PASS; `npm ci --ignore-scripts`:
  PASS; npm audit của lần cài: 0 vulnerabilities.
- PostgreSQL integration toàn bộ `./internal/...`: PASS trên
  `uv09_restore_1790264509343_4299d5` schema 31, tách khỏi worker browser;
  bao gồm graph purge + timestamp audit mới.
- Browser runner chạy lại sau budget fix: **5/5 PASS**, 1.2 phút; schema 31 database
  `uv09_final_journeys_20260925`.
  Artifacts ở `frontend/test-results/`, `frontend/playwright-report/journeys/`
  (gitignored, tách khỏi report provider faults để không ghi đè nhau), storage:
  `/var/folders/f4/b869tvmd7jb_cbxm4_tk__yc0000gn/T/ai-test-uv09-HRaMRA`.
  Lượt đầu schema 31 phát hiện test UV05 đếm dòng khi inventory đang tải; đã sửa
  thành chờ dòng visible có timeout, không reload/sleep/automatic retry. Giữ report
  fail tại `/tmp/uv09-browser-failure-GLeYGh`; cả suite chạy lại đạt 5/5.
- Migration/restore: PASS; source DB `uv09_migration_1790264630273_652653`, restored
  DB `uv09_restore_1790264630273_652653`; empty roundtrip DB
  `uv09_empty_1790264630273_652653`; dump 23/31, source archive và result JSON:
  `/var/folders/f4/b869tvmd7jb_cbxm4_tk__yc0000gn/T/ai-test-uv09-migration-Jvk5bF`.
  Tái hiện lỗi năm 2001 trước migration 31; kiểm audit/time window, no-op, graph
  không đổi, USER_PUBLISHED không đổi, write bị từ chối sau hiệu chỉnh, populated
  down bị chặn và empty down/up thành công. Sentinel USER_PUBLISHED chỉ kiểm
  metadata, không phải proof của một lần publish thật.
  Đây là synthetic populated drill, **không phải backup/restore production đã nghiệm thu**.
- SQL assertions của lane CI populated cũng PASS local trên
  `uv09_ci_timestamp_20260924`; không đồng nghĩa pipeline remote đã chạy.
- Provider fault browser mở rộng extraction + generation/reload + budget:
  **3/3 PASS**, khoảng 1 phút, DB `uv09_final_faults_20260925` schema 31.
  Artifacts: `frontend/test-results/provider-faults/` và
  `frontend/playwright-report/provider-faults/`; storage:
  `/var/folders/f4/b869tvmd7jb_cbxm4_tk__yc0000gn/T/ai-test-uv09-XZJWni`.
- `make test-worker-restart`: **3/3 SIGKILL modes + budget reconciliation test PASS**,
  DB integration phía trên; job IDs 79/80/81. JSON được in bằng `go test -v`;
  lane CI `worker-restart-drill` lưu `test-artifacts/uv09-worker-restart.json`.
  Mode in-provider: 40 token recorded + 2475 token held, 1 unknown reservation;
  before-commit: 80 token recorded, 2 provider invocations, 1 proposal;
  after-commit: 40 token recorded, 1 invocation, giữ human decision.
  Đây là local fixture ledger reconciliation, không phải hóa đơn provider thật.
- Workbook verifier hardening: unit tamper tests PASS, gồm sửa expected/actual,
  false PASS, history, release/revision metadata, Summary range/cached value,
  malformed package/XML và oversized part **kể cả khi tính lại content checksum**.
  Unicode/multiline, formula-safe escaping và cell truncation hợp lệ vẫn PASS.
  PostgreSQL scope/report/evidence integration PASS trên DB test schema 31 ở trên:
  export R1 do renderer thật tạo vẫn verify được sau khi project bind R2.
- Không commit/deploy, không đổi database chính, không gọi SCM/LLM bên ngoài.

## Lệnh chạy lại

```sh
# Node.js >=20; Docker/PostgreSQL pgvector + Go toolchain của repo.
npm --prefix frontend ci --ignore-scripts
cd frontend
npx playwright install chromium # Linux runner không dùng image: thêm --with-deps
cd ..
npm --prefix frontend run build
# Tự tạo DB test uv09_* và migrate đến 31 trước; không dùng DB ứng dụng chính.
UV09_ALLOW_TEST_WRITES=yes \
UV09_DATABASE_URL='postgres://postgres:postgres@localhost:5432/uv09_browser_test?sslmode=disable' \
make test-browser

# Lane riêng: fixture local HTTP 400 / timeout / invalid enum, không key thật.
# Dùng DB test riêng đã migrate 31; không dùng chung DB đang có worker khác.
UV09_ALLOW_TEST_WRITES=yes UV09_PROVIDER_FIXTURE=yes \
UV09_DATABASE_URL='postgres://postgres:postgres@localhost:5432/uv09_faults_test?sslmode=disable' \
make test-browser

# Synthetic populated migration + backup/restore, chỉ DEVELOPMENT compose:
UV09_ALLOW_TEST_WRITES=yes make test-migration-drill

# Chỉ DB integration riêng schema 31, không có worker khác đang lấy queue:
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/uv09_restart_test?sslmode=disable' \
make test-worker-restart

# Có thể phát sinh phí. Chỉ chạy khi có quyền dùng key/model thật:
RUN_REAL_PROVIDER_SMOKE=yes LLM_API_KEY_FILE='<private key file>' \
LLM_MODEL='<approved model>' make test-provider-smoke
```

CI browser dùng image Playwright khớp lockfile, một worker và PostgreSQL riêng.
Tham khảo [Playwright CI](https://playwright.dev/docs/ci). Lệnh browser con có thể lọc
qua `npm --prefix frontend run test:e2e -- roles` hoặc `--grep UV08`.

## Ma trận bằng chứng (không tự đóng toàn bộ chỉ vì có test cùng tên)

| ID | Bằng chứng hiện có | Phần còn thiếu ở release gate |
| --- | --- | --- |
| U01/U02/U03 | UV05 bộ mới/mixed parse; U04 reload ngay khi generation đang chạy, giữ job/checkpoint; workflow pin/dedupe | Remote runner evidence |
| U04 | 3 browser fault tests cả extraction/generation: bounded calls, persisted error, UI retry, reload, budget warning | Provider thật |
| U05/U06 | 3 SIGKILL modes trước/sau commit, ledger unknown/recorded usage, human decision; UV08 lost response | Remote runner, actual billing receipt reconciliation |
| U07 | `roles.spec.ts`, UV08 viewer direct POST 403 và read-only diff | End-user/OIDC policy trên staging |
| U08/U09 | UV06 20-item batch/TBD/browser; requirement schema/grounding/golden tests | Usability người mới, không suy ra đủ coverage từ fixture |
| S01–S04 | document/requirement UV01 PostgreSQL tests + UV08 source-version browser | Audit final trên dữ liệu staging |
| V01–V03 | testcase UV02/UV08 identity/hash/dedupe integration | Tổng hợp P0 sign-off |
| V04–V07 | UV07 browser history/edit/restore/concurrent tabs, scope integration | Human hiểu latest vs pinned |
| V08–V11 | requirement/testcase guards + immutable evidence/run proof integration | Tổng hợp P0 sign-off |
| V12 | UV08 ambiguity/default affected/apply/retire browser + integration | Không còn phần matching tự động trong phạm vi này |
| B01/B02 | scope integration pin release/rebind/concurrency; verifier R1/R2 mismatch | Webhook thật trong demo |
| E01–E03 | report integration + UV08 bytes/history + verifier checksum, parsed cells/metadata và Summary formulas | XLSX/evidence từ sandbox thật |
| M01/M02 | legacy drill và testcase/scope FK/ownership integration | Snapshot dữ liệu triển khai thực, phân loại chain cần QA review |
| M03 | schema-31 synthetic graph/audit backup/restore, source file checksum; purge integration | Coordinated production backup/restore, rollout/rollback rehearsal |
| A01 | 390px + keyboard ở UV05/07/08; role journey | NVDA/VoiceOver và ba người mới |

## Audit hồi quy P0 DATA/VER (local)

Các test bên dưới PASS với PostgreSQL schema 31 hoặc browser backend thật.
Đây là mapping hồi quy kỹ thuật, không tự thay cho QA sign-off trên dữ liệu staging.

| Lỗi gốc | Bằng chứng hồi quy |
| --- | --- |
| DATA-01/02 | `document.TestUV01UploadInvalidatesWorkingIndexAndGenerationMembershipIsIsolated`: upload invalidates index; generation membership không lẫn chunk cũ |
| DATA-03 | `document.TestUV01ApprovalRefreshesWarningsWithoutEmbedding`: đổi approval cập nhật warning không cần re-embed |
| VER-01/05 | `testcase.TestUV02RevisionLifecycle` + UV07 browser: history/diff/edit/restore, CAS, mở đúng revision mới và tách save khỏi approve |
| VER-02/03 | `TestUV02DistinctNegativeScenariosHaveDistinctCanonicalContent`, `TestUV02StepAndTestDataChangesAffectCanonicalContent`, revision lifecycle: không gộp các scenario khác nhau, hash xét steps/data |
| VER-04 | `TestUV02FamilySeparatesLatestFromLatestApproved`, `TestUV03PublishedReleasePinsExactApprovedRevisions`: draft không xóa approved/published revision |
| VER-06 | `report.TestUV03RunExportKeepsExecutedRevisionAfterDraftSuccessor`, scope/verifier R1 sau R2: run/export đọc pinned revisions |
| VER-07 | Bổ sung assertions trong `TestUV03PublishedReleasePinsExactApprovedRevisions`: draft chỉ DESIGNED; PUBLISHED/AUTOMATED/EXECUTED có denominator và release ID riêng; R2 không kế thừa execution R1 |

Legacy coverage fields vẫn mô tả thiết kế, không được hiểu là đã duyệt/đã chạy;
UI dùng các layer có nhãn riêng. Local suite không phát hiện P0 mới trong phạm vi
fixtures này; checkbox release sign-off vẫn mở đến khi QA nghiệm thu thực tế.

## Gate vẫn mở / bước tiếp theo

1. QA sign-off P0 trên dữ liệu staging; đối soát unknown usage với receipt provider
   thật bằng quy trình vận hành được phê duyệt, không release theo TTL.
2. Chạy GitLab pipeline thật; giữ artifacts lâu dài và ghi runner/image versions.
   Remote repo hiện là GitHub; chưa có URL GitLab pipeline/runner được cung cấp.
3. Có staging/repository/credential được phép dùng, chạy provider smoke rồi demo
   upload → review → release → webhook → sandbox → XLSX, đổi nguồn/publish R2 và
   chạy verifier lại với IDs của R1. Không sửa expected hoặc ép PASSED cho demo.
4. Nghiệm thu 3 người mới + screen reader theo [mẫu ghi nhận](UV09_USABILITY_ACCEPTANCE.md).
5. Backup/restore và rollout/rollback có dữ liệu thực, consumer compatibility và
   quyết định feature-flag/forward-fix. Không down schema có revision/release mới.

Điểm timestamp của migration 25 đã có forward fix 31 và regression drill local;
không sửa file migration 25 đã phát hành. Vẫn cần backup/snapshot triển khai,
đánh giá thời gian giữ exclusive locks và kiểm consumer trước production cutover.
Snapshot/export cũ có metadata thời gian cũ vẫn giữ nguyên bytes/hash; audit mới
giải thích khác biệt đó, không rewrite proof để trông như chưa từng có sai lệch.

Không đánh dấu hoàn thành phần thiếu và không dùng SKIP/provider fixture để thay
thế kết quả của môi trường thật.
