# Kế hoạch chuyển đổi sang Document-Driven Testing

- **Ngày chốt định hướng:** 10/09/2026
- **Cập nhật triển khai:** 21/09/2026
- **Trạng thái:** Phase 0–11 đã triển khai; Definition of Done E2E chờ chạy
  verifier trên hạ tầng demo thật
- **Phạm vi:** Backend Go, Frontend Next.js, PostgreSQL/pgvector, LLM,
  GitHub/GitLab và Docker Sandbox

## 1. Cách sử dụng tài liệu này

Đây là nguồn kế hoạch chuẩn cho quá trình chuyển AI Test Assistant từ pipeline
code-first sang:

```text
Document-driven test design + code execution
```

Đợt cải thiện tiếp theo được theo dõi riêng tại
[kế hoạch UX và testcase versioning](DOCUMENT_WORKFLOW_UX_AND_TESTCASE_VERSIONING_PLAN.md).
UV-00–UV-04 đã nghiệm thu; UV-05 đã có workspace bốn bước và kiểm thử kỹ thuật,
còn nghiệm thu usability với người mới. Xem
[bằng chứng UV-05](UV05_WORKSPACE_VERIFICATION.md). Tiến độ này không tự đóng
Definition of Done E2E trên hạ tầng demo thật của kế hoạch hiện tại.

Quy ước checklist:

- `[x]`: đã hoàn thành và có bằng chứng.
- `[ ]`: chưa hoàn thành.
- Chỉ đánh dấu phase hoàn thành khi toàn bộ Definition of Done của phase đạt.
- Không đánh dấu hoàn thành chỉ vì đã tạo interface, migration hoặc giao diện giả.

Các tài liệu mô tả Phase 0–13 cũ chỉ phản ánh baseline code-first đang chạy.
Khi có mâu thuẫn về định hướng mới, tài liệu này được ưu tiên.

## 2. Mục tiêu và nguyên tắc không được phá vỡ

### 2.1. Mục tiêu sản phẩm

Người dùng tải tài liệu nghiệp vụ lên, hệ thống trích xuất requirement có truy
vết, sinh test case đủ main/alternate/exception flow, cho con người duyệt, rồi
dùng code tại một PR/MR cụ thể để tạo và chạy automation. Kết quả thực thi được
lưu và xuất ra XLSX/Markdown.

### 2.2. Hai pipeline độc lập nhưng liên kết

```text
Pipeline A — Test design
Document set
→ parse
→ semantic chunks
→ requirement inventory
→ requirement review
→ test-case generation
→ deterministic coverage audit
→ test-case review

Pipeline B — Test execution
Approved test case + PR/MR source SHA
→ select execution scope
→ generate/reuse automation artifact
→ sandbox execution
→ classify result
→ bounded technical repair
→ human review
→ XLSX/Markdown report
```

### 2.3. Quy tắc nguồn sự thật

- Tài liệu đã phê duyệt quyết định scenario và expected result.
- Code/diff không được dùng để thay expected result.
- Code chỉ được dùng để xác định phạm vi kỹ thuật, build và hiện thực automation.
- Mọi expected result phải truy vết tới document version và source locator.
- Nguồn `DRAFT` không được giả thành `APPROVED`.
- Khi nguồn mâu thuẫn hoặc thiếu, kết quả là `CONFLICT`/`TBD`, không tự bịa.
- AI là tác nhân tạo draft; `generated_by=AI` không thay thế `business_source`.
- Sandbox pass chỉ chứng minh automation chạy theo assertion; nó không tự chứng
  minh test case đúng nghiệp vụ.
- Repair chỉ được sửa automation, fixture hoặc lỗi kỹ thuật; không được sửa
  requirement/test-case expected result đã duyệt.

## 3. Thuật ngữ miền nghiệp vụ

| Thuật ngữ | Ý nghĩa |
| --- | --- |
| Document set | Một bộ tài liệu cùng phạm vi sản phẩm/phiên bản |
| Document version | Bản bất biến của một file được upload |
| Source locator | Vị trí trích dẫn: heading, bảng/hàng, use-case step, sheet/cell hoặc trang |
| Requirement | Yêu cầu chuẩn hóa từ một hoặc nhiều đoạn nguồn |
| Requirement evidence | Liên kết requirement với document chunk gốc |
| Test case | Đặc tả nghiệp vụ gồm precondition, steps, data và expected result |
| Automation artifact | Code/script hiện thực một test case đã duyệt |
| Test run | Một lần chạy artifact trên source SHA và environment xác định |
| Product failure | Actual khác expected đã duyệt |
| Automation error | Test code/fixture/locator sai, chưa kết luận product sai |
| Infra error | Build agent, dependency hoặc môi trường chạy bị lỗi |
| Coverage audit | Kiểm tra liên kết requirement/flow ↔ test case bằng dữ liệu lưu trữ |

## 4. Phần baseline hiện có có thể tái sử dụng

- [x] Go API và modular packages.
- [x] PostgreSQL migrations/repositories.
- [x] pgvector, embedding boundary và hybrid retrieval cơ bản.
- [x] LLM abstraction cho Gemini/OpenAI và structured output.
- [x] Worker queue, lease, retry và phase handoff.
- [x] GitHub/GitLab webhook, diff và source snapshot.
- [x] Docker Sandbox giới hạn network/resource/privilege.
- [x] Generated-test versioning, validation history và review audit.
- [x] AI provenance gồm prompt/response/context/token/latency.
- [x] Next.js review console.
- [x] Đã có document upload/versioning/parsing cho DOCX và Markdown.
- [x] Đã có requirement inventory/review với evidence, conflict và TBD.
- [x] Đã có document-grounded test-case domain và versioned review.
- [x] Đã có coverage audit tái tạo từ requirement/test-case links.
- [x] Đã có XLSX/Markdown execution report exporter với immutable snapshot/hash.
- [x] Đã tách product failure khỏi automation/infra error.

## 5. Trạng thái tổng thể

| Phase | Nội dung | Backend | Frontend | Trạng thái |
| --- | --- | --- | --- | --- |
| 0 | Chốt định hướng và tài liệu | N/A | N/A | `DONE` |
| 1 | Domain model và migration nền | Đã làm | Đã làm | `DONE` |
| 2 | Upload, lưu trữ và parse tài liệu | Đã làm | Đã làm | `DONE` |
| 3 | Semantic chunking và document RAG | Đã làm | Đã làm | `DONE` |
| 4 | Requirement extraction và review | Đã làm | Đã làm | `DONE` |
| 5 | Sinh test case và coverage audit | Đã làm | Đã làm | `DONE` |
| 6 | Test-case workspace và XLSX export | Đã làm | Đã làm | `DONE` |
| 7 | Liên kết PR/MR với test scope | Đã làm | Đã làm | `DONE` |
| 8 | Sinh automation từ test case đã duyệt | Đã làm | Đã làm | `DONE` |
| 9 | Sandbox execution và phân loại kết quả | Đã làm | Đã làm | `DONE` |
| 10 | Technical repair có guardrail | Đã làm | Đã làm | `DONE` |
| 11 | Migration UI, evaluation và hardening | Đã làm phần lõi | Đã làm phần lõi | `IN_PROGRESS` |

## 6. Chi tiết từng phase

## Phase 0 — Chốt định hướng và ranh giới nghiệp vụ

### Mục tiêu

Ngăn team tiếp tục phát triển theo giả định “code là nguồn expected behavior” và
thống nhất thuật ngữ trước khi thay schema/API.

### Checklist

- [x] Chốt nguyên tắc tài liệu quyết định `WHAT/EXPECTED`.
- [x] Chốt code quyết định `HOW TO AUTOMATE` và là đối tượng thực thi.
- [x] Giữ GitHub/GitLab, webhook và Docker Sandbox trong kiến trúc mục tiêu.
- [x] Phân biệt test case, automation artifact và test run.
- [x] Ghi nhận giới hạn của coverage dựa trên bản tóm tắt AI.
- [x] Cập nhật tài liệu entry point và kiến trúc để phân biệt current/target.
- [x] Tạo kế hoạch chuyển đổi có checklist.
- [x] Chốt automation đầu tiên là Go unit test; API/UI automation được hoãn tới
  khi có contract/môi trường tương ứng.
- [x] Chốt upload MVP là DOCX + Markdown; XLSX là output ở Phase 6, chưa import.
- [x] Chốt Product Owner/BA duyệt document/requirement, QA/Test Lead duyệt test
  case, Developer/QA Automation duyệt artifact.
- [x] Chốt MVP giữ file gốc và evidence theo vòng đời document set; API purge,
  retention theo thời gian và coordinated backup/restore được thực hiện ở Phase 11.
- [x] PO đã duyệt hướng triển khai qua yêu cầu thực hiện Phase 0–2; bằng chứng
  phê duyệt học thuật riêng của mentor được quản lý ngoài runtime repository.

### Definition of Done

- [x] Không còn tài liệu entry point nào mô tả code-first là kiến trúc đích.
- [x] Có quyết định bằng văn bản cho automation type và file format MVP tại
  `docs/adr/0003-document-authority-versioning-and-mvp-boundary.md`.
- [x] Có người chịu trách nhiệm phê duyệt requirement/test case.

---

## Phase 1 — Domain model và migration nền

### Mục tiêu

Tạo mô hình dữ liệu mới song song với schema cũ để có thể chuyển dần, không làm
hỏng pipeline đang chạy.

### Backend

- [x] Viết ADR cho document authority, versioning và immutable evidence.
- [x] Thêm package `internal/document`.
- [x] Thêm package `internal/requirement`.
- [x] Thêm package `internal/testcase`.
- [x] Thêm package `internal/automation` hoặc tách rõ khỏi `generation` cũ.
- [x] Thêm package `internal/report`.
- [x] Tạo migration cho `document_sets`.
- [x] Tạo migration cho `documents` và `document_versions`.
- [x] Tạo migration cho `document_chunks` với pgvector và metadata.
- [x] Tạo migration cho `requirements` và `requirement_evidence`.
- [x] Tạo migration cho `requirement_conflicts`/`open_questions`.
- [x] Tạo migration cho `test_cases`, `test_case_steps` và
  `test_case_requirement_links`.
- [x] Tạo migration cho `automation_artifacts`.
- [x] Tạo migration cho `test_runs` và `test_run_evidence`.
- [x] Tạo migration cho document/requirement/test-case review decisions.
- [x] Dùng status CHECK constraint hoặc typed validation nhất quán.
- [x] Bổ sung index cho set/version/status, source locator, retrieval và coverage.
- [x] Viết down migration an toàn cho toàn bộ bảng mới.
- [x] Viết repository integration tests kiểm tra ownership/set isolation.

### Frontend

- [x] Định nghĩa TypeScript types cho document, requirement, test case và run.
- [x] Định nghĩa route map mới nhưng chưa cần UI hoàn chỉnh.
- [x] Thiết kế status badge cho parsing/index/review/execution.
- [x] Giữ màn hình legacy hoạt động trong thời gian chuyển đổi.

### Quyết định dữ liệu bắt buộc

- `document_versions` là bất biến sau khi index.
- Mỗi requirement phải có ít nhất một evidence link.
- Mỗi expected result phải dẫn về evidence của requirement đã duyệt.
- Automation artifact phải tham chiếu test-case version cụ thể.
- Test run phải lưu source SHA, artifact hash, environment và timestamp.

Ở Phase 1, schema cho phép tạo record `DRAFT` trước rồi gắn evidence trong cùng
workflow. Service review ở Phase 4–5 phải từ chối chuyển sang `APPROVED` nếu
thiếu evidence/link bắt buộc. “Document version bất biến” áp dụng cho file
identity/content; parse status và approval status là workflow state được phép
thay đổi có kiểm soát.

### Tests

- [x] Migration up/down round trip trên database sạch và database legacy.
- [x] Foreign-key ownership và cascade policy.
- [x] Không thể nối evidence khác document set.
- [x] Không thể nối artifact với test case chưa duyệt hoặc đổi expected hash.
- [x] Không thể sửa version/evidence bất biến.

### Definition of Done

- [x] Migration chạy được trên database sạch và database có dữ liệu legacy.
- [x] Repository tests pass với PostgreSQL thật.
- [x] Không thay đổi hành vi API legacy.

---

## Phase 2 — Upload, lưu trữ và parse tài liệu

### Mục tiêu

Cho phép người dùng tạo document set, upload tài liệu và xem nội dung được parser
đọc ra trước khi dùng LLM.

### Backend

- [x] API tạo/list/get document set.
- [x] API upload document version bằng multipart có giới hạn dung lượng.
- [x] Tính SHA-256 để deduplicate và truy vết file gốc.
- [x] Lưu filename, MIME type, size, checksum, version, approval status.
- [x] Chọn storage abstraction: local shared volume cho MVP, object storage về sau.
- [x] Không lưu file binary lớn trực tiếp vào PostgreSQL.
- [x] Parser interface trả blocks chuẩn hóa: heading, paragraph, table, list và code.
- [x] Parser DOCX giữ heading, bảng, thứ tự và source locator.
- [x] Parser Markdown giữ heading hierarchy và line range.
- [x] XLSX import không nằm trong MVP theo ADR 0003; parser được hoãn có chủ đích.
- [x] Loại bỏ macro/embedded executable; không thực thi nội dung tài liệu.
- [x] Bảo vệ zip bomb, path traversal, oversized XML và parser timeout.
- [x] Ghi parse status/error và số block trích xuất.
- [x] Tạo worker riêng cho parsing.

### Frontend

- [x] Trang danh sách document sets.
- [x] Form tạo document set với product/scope/description.
- [x] Upload drag-and-drop và file validation phía client.
- [x] Hiển thị trạng thái: `UPLOADED`, `PARSING`, `PARSED`, `FAILED`.
- [x] Trang document preview hiển thị heading/table/source locator.
- [x] Hiển thị version và approval badge rõ ràng.
- [x] Cho upload version mới mà không ghi đè version cũ.

### Tests

- [x] Fixture DOCX chứa heading, table và Unicode tiếng Việt.
- [x] Fixture Markdown có nested heading và table.
- [x] XLSX fixture không áp dụng vì import không nằm trong MVP Phase 2.
- [x] Reject extension/MIME không hỗ trợ và file quá lớn.
- [x] Parser không làm mất main/alternate/exception flow.
- [x] Parser output deterministic với cùng checksum.

### Definition of Done

- [x] Upload bộ PTYC/URD mẫu qua Docker API/worker và mở được ba route UI;
  kết quả lần lượt là 167 và 471 block.
- [x] Có thể chỉ ra nguồn tới heading/table cụ thể bằng `word/body/p[n]` và
  `word/body/table[n]` (Markdown dùng line/range).
- [x] Parser failure không làm mất file gốc và có lỗi dễ xử lý.

### Bằng chứng xác minh Phase 0–2 (11/09/2026)

- `go test -race ./...` và `go vet ./...` pass cho toàn backend.
- `go test -count=1 -p 1 -tags=integration ./internal/...` pass với PostgreSQL
  thật; Compose worker được dừng trong lúc chạy để không tranh database queue.
- Migration 1–15 chạy up/down toàn bộ trên database sạch; migration 15 chạy
  down/up trên database legacy hiện có.
- `npm run typecheck` và `npm run build` pass; Next.js build nhận đủ ba route
  Documents mới.
- Docker build API/worker/frontend pass; API nhận PTYC và URD thật, parse thành
  167/471 block, block count khớp response và không có locator rỗng.
- `make sandbox-security-check` và `make sandbox-test` pass, chứng minh thay đổi
  không phá regression guard của pipeline thực thi legacy.
- Dữ liệu/file dùng cho E2E được xóa sau test; không để sample tạm trong database
  hoặc `document_data` volume.

### Bằng chứng xác minh Phase 3–5 (11/09/2026)

- Unit tests bao phủ semantic chunking, bảng có header, YC/REQ identifier,
  prompt injection, strict LLM schemas, grounding expected result, conflict/TBD,
  exact/semantic dedupe và rule không biến mọi precondition thành state test.
- Integration workflow chạy PostgreSQL thật qua toàn chuỗi index → retrieve →
  extract → review → generate → regenerate → coverage; fixture UC-B08 giữ riêng
  main, hai alternate và hai exception flow.
- Golden retrieval `UC-B08`, `PR04.03`, `BR03`, `AC-03` đạt Recall@5 = `1.0`;
  test đồng thời kiểm tra set isolation, approved ranking và snapshot bất biến.
- Migration 1–16 chạy up/down trên database sạch; migration 16 down/up trên
  database legacy. Approval trigger từ chối requirement không evidence và test
  case không có approved requirement link.
- E2E với `dat-hang-thanh-cong.md`: 40 semantic chunks, 18 requirement có mã
  `YC-DATHANG-*` được đọc đúng cột actor/precondition/risk, retry generation tái
  sử dụng toàn bộ test case, và năm route Phase 3–5 trả HTTP 200.
- Coverage E2E không hiển thị hoàn tất dù approved denominator đã được phủ khi
  còn TBD; `baseline_complete=false` là guardrail có chủ đích.
- `go test -race ./...`, integration suite, `go vet`, frontend typecheck/
  production build và Docker API/worker/frontend build đều pass.

---

## Phase 3 — Semantic chunking và Document RAG

### Mục tiêu

Xây index tài liệu theo đơn vị nghiệp vụ và bảo đảm retrieval không bỏ qua metadata
nguồn hoặc trộn document set.

### Backend

- [x] Chunk theo requirement/FR/BR/AC/UC/YC thay vì chỉ theo token cố định.
- [x] Tách riêng main, alternate và exception flow.
- [x] Giữ parent-child link giữa use case và từng step/flow.
- [x] Lưu actor, precondition, post-condition, flow type và identifier vào metadata.
- [x] Chunk bảng theo hàng nhưng mang theo header và section cha.
- [x] Có fallback token chunk cho prose không có cấu trúc.
- [x] Embed chỉ text đã normalize, không mất raw source evidence.
- [x] Incremental re-index theo document-version checksum, embedding model và chunker version.
- [x] Hybrid retrieval: identifier exact match + full text + vector.
- [x] Filter bắt buộc theo `document_set_id` và version policy.
- [x] Ranking ưu tiên `APPROVED` hơn `DRAFT`, nhưng không che mất conflict.
- [x] Trả score breakdown và source locator.
- [x] Snapshot context bất biến cho mỗi LLM call.
- [x] Thêm adversarial prompt-injection fixtures trong nội dung tài liệu.

### Frontend

- [x] Index action và trạng thái theo document set.
- [x] Hiển thị file/chunk/skipped count và embedding model.
- [x] Chunk inspector theo document/section/type.
- [x] Retrieval debug được đặt trong review console; phải giữ console sau private proxy cho tới khi có RBAC.
- [x] Cảnh báo khi index chứa document version chưa duyệt.

### Evaluation

- [x] Golden query cho `UC-B08`, `PR04.03`, `BR03`, `AC-03`.
- [x] Đo Recall@5 theo requirement/flow; fixture đạt `1.0`.
- [x] Test isolation giữa hai document set có nội dung giống nhau.
- [x] Chứng minh hai alternate và hai exception flow của UC-B08 đều có chunk riêng.

### Definition of Done

- [x] Retrieval luôn có citation đầy đủ và đúng document set.
- [x] Golden dataset đạt Recall@5 = `1.0` cho bốn identifier fixture.
- [x] Re-index không thay đổi historical context snapshot; input không đổi là idempotent.

---

## Phase 4 — Requirement extraction, conflict/TBD và human review

### Mục tiêu

Tạo `Requirement Inventory` đầy đủ trước khi sinh bất kỳ test case nào.

### Backend

- [x] Structured schema cho requirement: ID, title, statement, type, actor,
  pre/post-condition, priority/risk, status và confidence.
- [x] Structured schema cho evidence citation; service tự gắn citation từ chunk thật thay vì tin ID do model trả.
- [x] Structured schema cho main/alternate/exception flow và từng step.
- [x] Extract theo từng semantic unit rồi aggregate toàn document set.
- [x] Không giới hạn tùy ý kiểu “tối đa 10 requirement” cho toàn tài liệu.
- [x] Deterministic dedupe theo source/identifier trước khi dùng semantic dedupe.
- [x] Phát hiện requirement trùng nhưng khác wording.
- [x] Phát hiện mâu thuẫn về trạng thái, role, threshold hoặc expected behavior.
- [x] Tạo TBD/open question cho chi tiết thiếu.
- [x] Mọi requirement bắt buộc có evidence; output không evidence bị reject.
- [x] Lưu raw LLM call và context snapshot trong provenance.
- [x] API list/detail/update-review cho requirement.
- [x] Chỉ requirement `APPROVED` mới vào baseline sinh test mặc định.
- [x] Version requirement; không ghi đè requirement đã dùng để sinh test.

### Frontend

- [x] Requirement inventory theo document set.
- [x] Filter theo document/type/status/risk/actor/flow.
- [x] Evidence drawer mở đúng nguồn và locator.
- [x] Màn hình conflict hiển thị hai nguồn cạnh nhau.
- [x] Accept/Edit/Reject requirement với reviewer và comment.
- [x] Danh sách TBD/open questions để gửi BA/PO.
- [x] Coverage counter dùng inventory trong database, không dùng số AI tự báo.

### Tests

- [x] Fixture PTYC/URD cho UC-B08 tạo main + 2 alternate + 2 exception flow.
- [x] Nguồn `DRAFT` không tự trở thành approved.
- [x] Requirement không citation bị từ chối ở service và database trigger.
- [x] Hai source mâu thuẫn tạo conflict thay vì bị merge âm thầm.
- [x] Retry extraction không tạo duplicate requirement.

### Definition of Done

- [x] Người review có thể duyệt một baseline mà không cần xem code.
- [x] Inventory giải thích được requirement nào đến từ đâu và phiên bản nào.
- [x] Chưa duyệt/conflict/TBD được thể hiện rõ trước bước sinh test.

---

## Phase 5 — Sinh test case và deterministic coverage audit

### Mục tiêu

Sinh test case từ baseline đã duyệt và chứng minh coverage bằng liên kết dữ liệu,
không bằng câu trả lời tự đánh giá của LLM.

### Backend

- [x] Test-case schema gồm ID, title, type, risk, actor, precondition, test data,
  steps, expected result, post-condition, confidence và automation status.
- [x] Mỗi test-case step hỗ trợ expected result cấp bước nếu cần.
- [x] Sinh theo từng requirement/flow, không prompt một lần cho toàn tài liệu.
- [x] Kỹ thuật sinh: happy, negative, boundary, permission, state, integration,
  regression và NFR khi đủ bằng chứng.
- [x] Không sinh expected cứng cho TBD hoặc threshold chưa được tài liệu chốt.
- [x] Gắn `ASSUMPTION` cho suy luận không được nêu trực tiếp.
- [x] Dedupe exact và semantic; giữ lý do merge/suppress.
- [x] Link nhiều requirement vào một test khi hợp lý.
- [x] Coverage engine tính requirement và flow coverage từ link trong DB.
- [x] Coverage audit so inventory với test cases, kể cả case bị reject.
- [x] Cảnh báo requirement không có positive/negative phù hợp.
- [x] Cảnh báo main/alternate/exception flow chưa được phủ.
- [x] Không hiển thị 100% nếu còn conflict/TBD ngoài mẫu số; hiển thị riêng.
- [x] API generate/regenerate/list/detail và coverage matrix.

### Frontend

- [x] Trang test-case list và detail.
- [x] Bảng requirement ↔ test case.
- [x] Badge loại test, risk, confidence, source status và automation status.
- [x] Evidence citation bên cạnh expected result.
- [x] Accept/Edit/Reject từng test case.
- [x] Bulk review giữ evidence và comment trong audit.
- [x] Coverage dashboard có mẫu số rõ ràng.
- [x] Hiển thị uncovered, conflict, TBD và duplicate riêng biệt.

### Evaluation với bộ mẫu

- [x] Không lặp lại lỗi “18/18”: baseline chưa hoàn tất khi còn flow/conflict/TBD chưa xử lý.
- [x] Sinh hoặc ghi nhận rõ case đổi địa chỉ.
- [x] Sinh hoặc ghi nhận rõ case quay lại giỏ hàng.
- [x] Sinh case thiếu thông tin nhận hàng bắt buộc.
- [x] Sinh case đơn không còn hợp lệ lúc xác nhận.
- [x] Phát hiện case exact/gần trùng tương đương TC-001/TC-006/TC-019 bằng deterministic test.
- [x] Tách `business_source`/evidence khỏi `generated_by=AI`.

### Definition of Done

- [x] Mọi expected result có citation tới requirement evidence đã duyệt.
- [x] Coverage matrix được tái tạo deterministic từ database.
- [x] Test case không đọc implementation để xác định đúng/sai nghiệp vụ.

---

## Phase 6 — Test-case workspace và XLSX/Markdown export

### Mục tiêu

Cho QA/BA sử dụng bộ test case độc lập với automation và xuất đúng template quản
lý kiểm thử.

### Backend

- [x] API export XLSX và Markdown theo document set/test suite/test run.
- [x] Workbook có metadata: scope, document versions, generated time và reviewer.
- [x] Các cột: TC ID, trace, objective, precondition, steps, data, role, expected,
  priority, environment, actual, status, evidence, notes.
- [x] Hỗ trợ nhiều execution rounds hoặc tách sheet run history rõ ràng.
- [x] Công thức thống kê P/F/NY không tính header/blank row.
- [x] Escape formula injection cho cell bắt đầu bằng `=`, `+`, `-`, `@` từ input.
- [x] Giới hạn cell length và sanitize invalid XML characters.
- [x] Export dùng immutable snapshot để có thể tái lập.
- [x] Lưu hash của artifact xuất.

### Frontend

- [x] Workspace giống test management sheet nhưng responsive.
- [x] Cho filter/sort trước khi export.
- [x] Nút tải XLSX/Markdown và hiển thị export metadata.
- [x] Preview cột Actual/Status/Evidence dù test chưa chạy.
- [x] Phân biệt `DRAFT`, `APPROVED`, `MANUAL`, `AUTOMATED`, `BLOCKED`.

### Tests

- [x] So sánh workbook xuất với template mẫu về cột bắt buộc.
- [x] Mở workbook bằng parser OOXML độc lập `openpyxl`; launcher LibreOffice cục
  bộ bị hỏng nên test LibreOffice tự skip khi executable không khả dụng.
- [x] Unicode tiếng Việt và multiline steps không bị lỗi.
- [x] Formula injection test.
- [x] Số liệu summary khớp data row range và dữ liệu database.

### Definition of Done

- [x] Có thể xuất bộ UC-B08 ra XLSX dễ đọc và truy vết được nguồn.
- [x] Export trước khi chạy hiển thị NY; sau khi chạy có Actual/P/F/Evidence.

---

## Phase 7 — Liên kết PR/MR với test scope

### Mục tiêu

Giữ tự động hóa theo code change nhưng không để code tạo ra expected behavior.

### Backend

- [x] Giữ webhook verification, dedupe, source/target SHA và changed files.
- [x] Thêm liên kết project ↔ document set/baseline version.
- [x] Cho người dùng chọn baseline áp dụng cho repository.
- [x] Cho PR/MR liên kết issue/user-story/requirement ID từ title/description/label.
- [x] Ưu tiên explicit link; Phase 7 MVP không cho AI tự thu hẹp scope, mapping
  không chắc chắn luôn dùng full approved fallback.
- [x] Dùng path/module/change symbol chỉ làm tín hiệu chọn scope.
- [x] Lưu lý do mỗi test case được chọn: explicit trace, issue link, path mapping,
  impact inference hoặc manual selection.
- [x] Cho chế độ chạy full approved suite khi scope mapping không chắc chắn.
- [x] Không thay expected result dựa trên diff.
- [x] Snapshot baseline version tại lúc tạo execution analysis.

### Frontend

- [x] Project page chọn document baseline.
- [x] Analysis page hiển thị requirement/test-case scope trước automation.
- [x] Cho reviewer thêm/bỏ test case khỏi run với audit comment.
- [x] Hiển thị confidence và lý do mapping.
- [x] Cảnh báo PR/MR không liên kết requirement.

### Tests

- [x] Webhook cũ vẫn hoạt động.
- [x] Hai project không dùng nhầm document baseline.
- [x] Explicit requirement ID chọn đúng test cases.
- [x] Low-confidence mapping không âm thầm thu hẹp test suite.

### Definition of Done

- [x] Một PR/MR tạo test run gắn source SHA và approved baseline cụ thể.
- [x] Người review biết rõ vì sao từng test case nằm trong scope.

---

## Phase 8 — Sinh automation từ test case đã duyệt

### Mục tiêu

Chuyển test case thành artifact chạy được mà không thay đổi ý nghĩa nghiệp vụ.

### Quyết định phạm vi trước khi code

- [x] Chọn Go unit test; AI chỉ đọc technical context cần cho package/interface/type.
- [x] Hoãn black-box API test tới khi có OpenAPI/base URL/test credentials.
- [x] Hoãn UI test vì chưa có locator contract ổn định.

### Backend

- [x] Automation input bắt buộc chứa immutable test-case snapshot.
- [x] Tách `business_context` và `technical_context` trong prompt/schema.
- [x] Business context chỉ lấy từ document evidence đã duyệt.
- [x] Technical context chỉ chứa thông tin cần compile/run.
- [x] Expected result nằm trong immutable section và có hash.
- [x] Structured output trả framework, target path, setup, steps/assertions và code.
- [x] Validate automation không sửa requirement/expected hash.
- [x] Validate path, package, imports, syntax, size và forbidden operations.
- [x] Một test case có thể có nhiều artifact version.
- [x] Một artifact chỉ hiện thực test cases được khai báo.
- [x] Lưu provenance riêng cho business và technical context.
- [x] Hỗ trợ `MANUAL/BLOCKED` khi thiếu thông tin automation.

### Frontend

- [x] Test-case detail hiển thị riêng Business Specification và Automation.
- [x] Diff artifact version nhưng khóa expected result.
- [x] Hiển thị technical context riêng, không gọi nó là business evidence.
- [x] Reviewer approve artifact trước lần chạy nếu policy yêu cầu.

### Tests

- [x] Prompt injection từ code không thể đổi expected result field/hash.
- [x] Artifact compile syntax đúng với fixture.
- [x] Thiếu interface/contract trả `BLOCKED`, không bịa API.
- [x] Generated code không sửa production file.

### Definition of Done

- [x] Có ít nhất một approved test case được chuyển thành artifact và truy vết hai chiều.
- [x] Có bằng chứng expected result trước/sau generation là bất biến.

---

## Phase 9 — Sandbox execution, result taxonomy và evidence

### Mục tiêu

Chạy automation tại đúng code revision và báo cáo đúng loại kết quả.

### Backend

- [x] Giữ source-SHA workspace và Docker isolation hiện có.
- [x] Chạy baseline suite trước generated artifact nếu framework hỗ trợ.
- [x] Lưu build/commit SHA, image digest, command và environment fingerprint.
- [x] Tách trạng thái: `PASSED`, `PRODUCT_FAILED`, `AUTOMATION_ERROR`,
  `INFRA_ERROR`, `TIMED_OUT`, `BLOCKED`, `NOT_RUN`.
- [x] Parser lỗi phân biệt compile error, assertion failure, panic, timeout và setup.
- [x] Assertion failure không mặc nhiên là product failure nếu artifact chưa được duyệt.
- [x] Lưu bounded/redacted stdout/stderr.
- [x] Evidence hỗ trợ log/artifact/screenshot link.
- [x] Map kết quả sang Excel `P/F/NY/BLOCKED` nhưng không làm mất taxonomy gốc.
- [x] Retry infra error riêng với repair attempt.
- [x] Không cho repair product failure bằng cách đổi expected assertion.

### Frontend

- [x] Run summary theo product/automation/infra status.
- [x] Hiển thị Expected và Actual cạnh nhau.
- [x] Hiển thị source SHA, environment, command, duration và evidence.
- [x] Filter failed/blocked/not-run.
- [x] Cho reviewer xác nhận hoặc đổi classification kèm audit reason.

### Tests

- [x] Fixture product failure có assertion mismatch.
- [x] Fixture automation compile failure.
- [x] Fixture infra failure và timeout.
- [x] Sandbox security tests vẫn pass.
- [x] Excel export nhận đúng Actual/Status/Evidence từ test run.

### Definition of Done

- [x] Báo cáo không đánh đồng mọi non-zero exit với product bug.
- [x] Một run có thể tái lập từ source SHA + artifact hash + environment fingerprint.

### Bằng chứng xác minh Phase 9 (12/09/2026)

- Migration 18 chạy up/down/up trên PostgreSQL thật; integration test xác minh
  claim bằng lease, bind approved artifact một lần, infra-only retry tạo attempt
  mới, evidence không bị ghi đè và classification override có audit.
- Test processor xác minh baseline chạy trước artifact, approved assertion
  mismatch thành `PRODUCT_FAILED`, thiếu approved artifact thành `BLOCKED` và
  code test không được ghi vào source checkout.
- Taxonomy fixtures phủ pass, compile, assertion, panic, dependency/infra,
  timeout và artifact chưa duyệt.
- `go vet ./...`, `go test -race ./...`, integration suite PostgreSQL,
  frontend typecheck/production build đều pass.
- `make sandbox-security-check` và `make sandbox-test` pass; test execution mới
  thực sự chạy clean baseline rồi approved artifact trong container không
  network, non-root, read-only và lưu image digest/fingerprint.
- XLSX giữ bốn sheet Test Cases/Run History/Summary/Metadata và đọc Actual,
  taxonomy/evidence trực tiếp từ `test_run_items`/`test_run_evidence`.

### Bổ sung vận hành — Requirement extraction bất đồng bộ (16/09/2026)

- `POST /requirements/extract` chỉ enqueue durable job và trả HTTP `202`; worker
  gọi LLM ngoài request timeout của API/reverse proxy.
- `GET /requirements/extraction` trả trạng thái, tiến độ theo semantic chunk,
  số requirement tạo/tái sử dụng, retry và lỗi cuối; frontend poll endpoint này.
- Job khóa index generation lúc enqueue, dùng lease/renewal và dừng nếu index đã
  thay đổi, tránh trộn evidence giữa hai thế hệ index.
- Migration 19 và unit/integration/API tests chứng minh idempotency, progress,
  retry hữu hạn và không còn giữ request trình duyệt đến khi Gemini hoàn tất.

---

## Phase 10 — Technical repair có guardrail

### Mục tiêu

Tự sửa lỗi triển khai automation nhưng bảo vệ tuyệt đối requirement và expected
result đã duyệt.

### Backend

- [x] Chỉ `AUTOMATION_ERROR` đủ điều kiện repair tự động mặc định.
- [x] `INFRA_ERROR` đi retry queue, không gọi LLM repair.
- [x] `PRODUCT_FAILED` chuyển review/report, không repair expected assertion.
- [x] Repair request chứa immutable expected hash và allowed-change policy.
- [x] So sánh semantic assertions trước/sau repair.
- [x] Reject unchanged artifact nhưng ghi `UNREPAIRABLE` thay vì làm fail toàn analysis.
- [x] Candidate hợp lệ luôn tạo artifact `DRAFT` và chuyển `WAITING_REVIEW`;
  provider lỗi có retry hữu hạn và trạng thái terminal riêng.
- [x] Hỗ trợ một candidate lỗi không chặn candidate độc lập khác.
- [x] Lưu before/after, reason, test-run-item/evidence link, model và prompt version.
- [x] Hard limit số lần và token/cost budget.

### Frontend

- [x] Repair history nêu rõ loại lỗi và trường được phép đổi.
- [x] Highlight assertion/expected guardrail.
- [x] Hiển thị `UNREPAIRABLE` và hướng người dùng sang xử lý thủ công.
- [x] Không hiển thị repair product failure như thành công kỹ thuật.

### Tests

- [x] Provider trả unchanged code không làm toàn analysis `FAILED`.
- [x] Repair cố đổi expected assertion bị từ chối.
- [x] Candidate pass không bị tạo version repair mới.
- [x] Max attempt luôn kết thúc.

### Definition of Done

- [x] Lỗi từng gặp ở Analysis #9 được xử lý thành candidate-level outcome.
- [x] Không có đường code nào cho repair sửa business expected result.

### Bằng chứng xác minh Phase 10 (16/09/2026)

- Migration 20 tạo queue `automation_repair_jobs`, unique active job, lease/retry,
  giới hạn ba lần và database trigger khóa expected hash, policy, source hash và
  assertion gốc.
- Repository chỉ enqueue item có taxonomy đúng bằng `AUTOMATION_ERROR` và
  approved artifact có cùng expected hash; integration test từ chối cả
  `PRODUCT_FAILED` lẫn `PASSED`.
- Processor so sánh semantic assertion chuẩn hóa trước/sau, kiểm tra Go syntax,
  target path/package/import và biến unchanged/unsafe output thành
  `UNREPAIRABLE` ở phạm vi candidate.
- UI run detail chỉ hiện nút repair cho `AUTOMATION_ERROR`, hiển thị policy,
  expected guardrail, lịch sử và liên kết tới artifact draft chờ review.
- Khi draft repair được duyệt, repository append một test-run-item attempt mới,
  đưa run về queue sandbox và giữ nguyên toàn bộ attempt/evidence cũ. Giới hạn
  repair được tính trên cả chuỗi run + test case, không reset sau mỗi rerun.

---

## Phase 11 — Migration UI, evaluation và hardening

### Mục tiêu

Chuyển trải nghiệm mặc định sang document-driven, đo chất lượng và loại bỏ legacy
chỉ sau khi dữ liệu mới ổn định.

### Backend

- [x] Giữ read path analyses legacy trong giai đoạn tương thích.
- [x] Feature flag chuyển pipeline theo project.
- [x] Backfill không gán giả citation cho dữ liệu cũ: project tồn tại được đặt
  `LEGACY`, project mới mặc định `DOCUMENT_DRIVEN`.
- [x] Retention/delete/export policy cho document và evidence: physical purge
  chỉ mở cho admin sau khi archive hết hạn, yêu cầu confirmation chính xác,
  chặn baseline/snapshot/test run còn tham chiếu và giữ audit độc lập sau khi xóa.
- [x] Service-token auth và role `viewer/editor/reviewer/admin` cho upload,
  approve, execute và export; identity người dùng cuối do reverse proxy/OIDC cấp.
- [x] Audit mọi approval và manual classification override.
- [x] Rate/token/cost limit theo document set/job: reservation nguyên tử trước
  lời gọi LLM, finalize usage thực tế, tự nhả reservation lỗi/hết hạn và hiển thị
  used/reserved/remaining theo document set.
- [x] Metrics cho parse, extraction, approval, generation và execution.
- [x] Backup/restore bao gồm file storage và database tại cùng quiesced point.
- [x] Endpoint compatibility cũ trả `Deprecation` và successor `Link` header.

### Frontend

- [x] Overview mặc định hiển thị Documents, Requirements, Test Suites và Runs.
- [x] Navigation legacy có nhãn rõ hoặc bị ẩn sau feature flag.
- [x] Empty/error/loading state dùng chung cho pipeline mới.
- [x] Accessibility và responsive review cho bảng coverage/XLSX-like workspace:
  skip link, focus-visible, live status và bảng scroll ngang trên màn hình hẹp.
- [x] Có onboarding demo PTYC/URD tại `docs/DOCUMENT_DRIVEN_DEMO.md`.

### Evaluation

- [x] Requirement extraction precision/recall trên golden-set format.
- [x] Flow coverage recall: main/alternate/exception.
- [x] Unsupported-claim/hallucination rate.
- [x] Citation correctness.
- [x] Duplicate test-case rate.
- [x] Human acceptance/edit distance/time.
- [x] Automation compile/execution rate.
- [x] Product-failure detection trên seeded defects.
- [x] So sánh `DOC_ONLY_TEST_DESIGN` với code-first recommendation cũ.
- [x] Báo riêng business quality và technical execution quality.

Fixture `document-controlled-v1` chỉ xác minh công thức và định dạng báo cáo;
số liệu luận văn phải được thay bằng annotation/trial thật, không được dùng fixture
để tuyên bố chất lượng sản phẩm.

### Definition of Done

- [ ] Demo end-to-end: upload URD → approve requirements → generate/approve test
  cases → PR webhook → sandbox run → Excel report.
- [x] Dữ liệu cũ vẫn đọc được, project cũ giữ `LEGACY` và endpoint tương thích
  có deprecation header mà không tạo citation giả.
- [x] README/API/database/deployment/security docs phản ánh trạng thái triển khai.

Phần triển khai Phase 11 đã hoàn tất. Definition of Done môi trường thật vẫn chỉ
được đánh dấu sau khi chạy `make prod-document-e2e-verify` với ID của demo dùng
credential GitHub/GitLab, LLM và sandbox; verifier kiểm tra 11 bằng chứng lưu
trong database và thoát khác 0 nếu thiếu bất kỳ mắt xích nào.

### Bằng chứng xác minh Phase 11 (17/09/2026)

- Migration 22 thêm trạng thái purge, giới hạn token/cost, reservation ledger và
  audit purge không phụ thuộc foreign key của document set.
- Integration test xác minh reservation/finalize/over-budget và purge cả file +
  graph `document → block → requirement → evidence` nhưng vẫn giữ audit.
- Backend unit suite, PostgreSQL integration suite, `go vet`, frontend production
  build, migration idempotency và API Docker image đều chạy thành công.
- Binary `/document-e2e-verify` nằm trong API image; thử với ID không tồn tại trả
  `passed: false` và exit khác 0, nên không thể đánh dấu DoD khi thiếu bằng chứng.

## 7. API document-driven hiện tại

Các endpoint đã triển khai gồm:

```text
POST /api/document-sets
GET  /api/document-sets
GET  /api/document-metrics
GET  /api/document-sets/{id}
POST /api/document-sets/{id}/lifecycle
GET  /api/document-sets/{id}/ai-budget
GET  /api/document-sets/{id}/purge
POST /api/document-sets/{id}/purge

POST /api/document-sets/{id}/documents
GET  /api/document-sets/{id}/documents
GET  /api/documents/{id}/versions/{version}
POST /api/document-versions/{id}/review
POST /api/document-sets/{id}/index
GET  /api/document-sets/{id}/index

POST /api/document-sets/{id}/requirements/extract
GET  /api/document-sets/{id}/requirements/extraction
GET  /api/document-sets/{id}/requirements
POST /api/requirements/{id}/review

POST /api/document-sets/{id}/test-cases/generate
GET  /api/document-sets/{id}/test-cases
GET  /api/document-sets/{id}/coverage
POST /api/test-cases/{id}/review

GET  /api/test-runs/{id}
POST /api/test-runs/{id}/execute
POST /api/test-run-items/{id}/classification
POST /api/test-run-items/{id}/repair
GET  /api/test-run-items/{id}/repairs

POST /api/projects/{id}/pipeline-mode

POST /api/document-sets/{id}/exports
GET  /api/document-sets/{id}/exports
GET  /api/test-exports/{id}/download
```

`POST /api/document-sets/{id}/exports` nhận `format` là `XLSX` hoặc `MARKDOWN`
và có thể nhận `test_run_id`; API trả `download_url` của snapshot bất biến.
Production API yêu cầu Bearer service token và role phù hợp; health/readiness và
webhook có cơ chế xác thực riêng nên không dùng token này.

Webhook GitHub/GitLab được giữ: với project `DOCUMENT_DRIVEN` đã chọn baseline,
webhook tạo `analysis_job`, immutable baseline snapshot, test scope và `test_run`;
project `LEGACY` tiếp tục đường tương thích mà không có citation giả.

## 8. Schema mục tiêu sơ bộ

```text
document_sets
documents
document_versions
document_blocks
document_chunks
requirement_extraction_jobs

requirements
requirement_evidence
requirement_conflicts
open_questions
requirement_reviews

test_suites
test_cases
test_case_steps
test_case_requirement_links
test_case_reviews

automation_artifacts
automation_repair_jobs
test_runs
test_run_items
test_run_evidence

llm_calls
context_snapshots
context_snapshot_items
document_set_audit_log
project_pipeline_mode_audit
```

Không tái sử dụng tên `generated_tests` cho test case nghiệp vụ vì hai khái niệm
khác nhau. Có thể migrate `generated_tests` cũ thành `automation_artifacts` nếu
giữ được provenance và ownership.

## 9. Thứ tự triển khai và dependency

```text
Phase 0
  → Phase 1
  → Phase 2
  → Phase 3
  → Phase 4
  → Phase 5
  → Phase 6
  → Phase 7
  → Phase 8
  → Phase 9
  → Phase 10
  → Phase 11
```

Không nên bắt đầu automation mới ở Phase 8 trước khi requirement/test-case
versioning và approval hoàn thành; nếu làm ngược thứ tự, hệ thống sẽ tiếp tục
dùng output AI chưa được xác nhận làm nguồn đúng.

Phase 6 có thể phát triển frontend/export song song với phần cuối Phase 5 sau khi
test-case schema ổn định. Phase 7 có thể tái sử dụng SCM baseline song song với
Phase 6, nhưng không được nối vào production flow trước khi baseline snapshot có
ownership test đầy đủ.

## 10. MVP cut line đề xuất

Nếu thời gian hạn chế, chia thành hai mốc:

### MVP A — Document to Test Case

- Phase 0–6.
- Upload DOCX/Markdown.
- Requirement review.
- Test-case generation và coverage.
- Xuất XLSX/Markdown.
- Test execution có thể là manual/NY.

### MVP B — Test Case to Automated Report

- Phase 7–10.
- PR/MR trigger.
- Automation artifact.
- Docker execution.
- Product/automation/infra classification.
- Actual result và evidence trong Excel.

Phase 11 là điều kiện để chuyển pipeline mới thành mặc định và dùng làm bản demo
đồ án chính thức.

## 11. Rủi ro chính và cách kiểm soát

| Rủi ro | Ảnh hưởng | Kiểm soát |
| --- | --- | --- |
| RAG top-k bỏ sót requirement | Coverage giả | Inventory toàn cục trước retrieval theo requirement |
| Tài liệu draft/sai | Expected không đáng tin | Approval status, conflict và human baseline review |
| AI bịa field/threshold | Test case sai | Citation bắt buộc, assumption/TBD, schema validation |
| Nhiều test trùng | Review tốn thời gian | Exact + semantic dedupe và reviewer override |
| Đọc code làm lệch expected | Hợp thức hóa bug | Tách business/technical context, immutable expected hash |
| Mọi test fail bị gọi là bug | Báo cáo sai | Result taxonomy product/automation/infra |
| Repair sửa assertion để pass | Che product defect | Repair policy và semantic assertion guard |
| XLSX chứa formula injection | Rủi ro người mở file | Cell escaping và exporter security tests |
| Viết lại toàn hệ thống | Regression lớn | Schema/API mới song song, feature flag, phase DoD |

## 12. Kịch bản demo mục tiêu

1. Tạo document set “Sàn TMĐT v1.0”.
2. Upload PTYC và URD; hệ thống hiển thị URD là `DRAFT`.
3. Parse/index và mở source locator của UC-B08.
4. Requirement inventory hiển thị main flow, hai alternate flow và hai exception flow.
5. Reviewer duyệt requirement baseline hoặc ghi conflict/TBD.
6. Sinh test case và xem coverage matrix; không được bỏ sót bốn flow nêu trên.
7. Duyệt test cases và xuất XLSX lần đầu với status `NY`.
8. Liên kết baseline với repository và mở PR/MR thay đổi chức năng đặt hàng.
9. Webhook tạo test run tại đúng source SHA.
10. Sinh/reuse automation rồi chạy trong Docker Sandbox.
11. Một case pass, một product fail và một automation error được phân loại khác nhau.
12. Repair chỉ sửa automation error; product failure được giữ nguyên.
13. Xuất XLSX có Actual Result, P/F/NY/BLOCKED và evidence.

## 13. Definition of Done toàn chương trình

- [x] Trong pipeline `DOCUMENT_DRIVEN`, tài liệu là nguồn duy nhất của business
  scenario và expected result.
- [x] Mọi test case truy vết được tới document version/source locator.
- [x] Coverage audit phát hiện requirement/flow chưa phủ.
- [x] Người dùng duyệt requirement trước test case và duyệt test case trước automation.
- [x] PR/MR tự động kích hoạt run tại đúng code revision khi project đã chọn baseline.
- [x] Generated automation chạy trong sandbox an toàn.
- [x] Product failure không bị repair thành pass bằng cách đổi expected.
- [x] Execution report xuất được XLSX/Markdown với Actual và evidence.
- [x] Có evaluation định lượng business quality và technical quality riêng.
- [x] Unit/integration/race/frontend tests pass; sandbox suite của Phase 9 tiếp tục
  là gate bắt buộc khi image/Docker runtime sẵn sàng.
- [x] Documentation current/target không còn mô tả Phase 10 là backlog.
