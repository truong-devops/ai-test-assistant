# Kế hoạch phát triển AI Test Assistant thành đồ án tốt nghiệp

**Phiên bản:** 1.0
**Ngày lập:** 2026-09-03
**Thời lượng đề xuất:** 12 tuần
**Phạm vi chính:** Go, GitLab, GitHub, PostgreSQL/pgvector, Docker sandbox, Next.js
**Trạng thái:** Kế hoạch định hướng; chưa phải cam kết mọi hạng mục đều nằm trong bản phát hành cuối

## 1. Tóm tắt định hướng

AI Test Assistant không nên được trình bày như một công cụ gọi LLM để viết unit
test. Đồ án cần được định vị thành một hệ thống kiểm thử vòng kín theo thay đổi
mã nguồn:

```text
GitHub/GitLab Pull/Merge Request
  -> phân tích thay đổi và phạm vi tác động
  -> truy xuất bằng chứng đúng project và đúng commit
  -> đề xuất kịch bản kiểm thử
  -> sinh mã kiểm thử
  -> chạy trong sandbox
  -> đo coverage và mutation score
  -> tự sửa theo phản hồi thực thi
  -> con người duyệt
  -> trả kết quả về Pull/Merge Request
```

Điểm nổi bật của đề tài là sự kết hợp của bốn thành phần:

1. Phân tích tác động của thay đổi bằng AST, type information và call graph.
2. RAG theo project, theo commit và có thể truy vết bằng chứng.
3. Sinh, thực thi, đánh giá và sửa test trong một vòng lặp có giới hạn.
4. Đánh giá thực nghiệm chất lượng test thay vì chỉ trình diễn output của AI.

## 2. Tên đề tài đề xuất

### Tên tiếng Việt

**AI Test Assistant: Sinh và tự sửa kiểm thử đơn vị theo thay đổi mã nguồn bằng
phân tích tác động, RAG và kiểm chứng thực thi**

### Tên tiếng Anh

**AI Test Assistant: Change-Aware Unit Test Generation and Repair using Impact
Analysis, Retrieval-Augmented Generation, and Execution Feedback**

## 3. Vấn đề nghiên cứu

Khi mã nguồn thay đổi, lập trình viên phải xác định phần nào cần được kiểm thử,
hiểu convention của project, viết test phù hợp, chạy test và sửa lỗi. LLM có thể
sinh test nhanh nhưng thường gặp các vấn đề:

- thiếu context riêng của project;
- sinh API, interface, mock hoặc helper không tồn tại;
- test compile được nhưng assertion yếu;
- không phát hiện lỗi dù coverage tăng;
- khó giải thích context nào đã dẫn tới kết quả;
- khó tái lập vì model, prompt và source thay đổi theo thời gian.

Đồ án giải quyết vấn đề này bằng cách kết hợp static analysis, commit-aware RAG,
LLM structured output, sandbox validation, quality feedback và human review.

## 4. Mục tiêu

### 4.1. Mục tiêu sản phẩm

- Người dùng kết nối repository GitHub hoặc GitLab từ giao diện.
- Hệ thống tự nhận Pull Request hoặc Merge Request qua webhook.
- Hệ thống xác định symbol thay đổi và symbol có khả năng bị tác động.
- Hệ thống truy xuất source, interface, mock, test và tài liệu liên quan.
- Hệ thống đề xuất và sinh Go unit test.
- Generated test được chạy trong Docker sandbox có giới hạn tài nguyên.
- Test lỗi có thể được sửa tự động trong số vòng lặp hữu hạn.
- Reviewer xem toàn bộ bằng chứng trước khi Accept hoặc Reject.
- Kết quả có thể được gửi lại GitHub Check hoặc GitLab MR discussion.

### 4.2. Mục tiêu nghiên cứu

- Đo tác động của RAG tới compile rate, execution rate và human acceptance.
- Đo tác động của repair loop tới final success rate.
- Đo khả năng phát hiện lỗi của generated test bằng mutation score.
- So sánh thời gian viết test thủ công và quy trình AI-assisted review.
- Bảo đảm mọi kết quả thí nghiệm có thể tái lập và truy vết.

### 4.3. Mục tiêu kỹ thuật

- Không trộn context giữa các project.
- Mọi AI call có source, context, prompt và model provenance bất biến.
- Mọi generated test phải qua sandbox trước khi được phép Accept.
- Repair loop luôn kết thúc.
- Pipeline chịu được worker crash nhờ lease, retry và idempotency.
- Có unit, integration, sandbox và end-to-end tests cho các luồng chính.

## 5. Câu hỏi nghiên cứu và giả thuyết

### RQ1 - Context

**Câu hỏi:** RAG theo project và đồ thị tác động có cải thiện chất lượng generated
test so với chỉ cung cấp diff không?

**Giả thuyết H1:** `DIFF_IMPACT_RAG` có compile rate, execution rate, mutation
score và human acceptance cao hơn `DIFF_ONLY`.

### RQ2 - Repair

**Câu hỏi:** Phản hồi thực thi có giúp LLM sửa generated test hiệu quả không?

**Giả thuyết H2:** `GENERATE_VALIDATE_REPAIR` có final success rate cao hơn
`GENERATE_ONLY`, với chi phí và số vòng sửa nằm trong giới hạn đã cấu hình.

### RQ3 - Human effort

**Câu hỏi:** AI-assisted workflow có giảm công sức của lập trình viên mà không
làm giảm chất lượng test không?

**Giả thuyết H3:** `AI_ASSISTED` giảm thời gian hoàn thành so với `MANUAL`, trong
khi mutation score và human acceptance không thấp hơn đáng kể.

## 6. Phạm vi

### 6.1. Bắt buộc

- Go source và Go unit test.
- GitHub.com và GitLab.com.
- Pull/Merge Request webhook.
- Change analysis, RAG, recommendation, generation, validation và repair.
- Coverage collection.
- Mutation testing trên phạm vi bị tác động.
- Human review và audit trail.
- Dữ liệu thí nghiệm thật.
- Báo cáo kết quả có thể tái tạo.

### 6.2. Nên có nếu đủ thời gian

- GitHub Check Run và GitLab MR discussion.
- Realtime status bằng Server-Sent Events hoặc polling có backoff.
- Authentication và project-level RBAC.
- OpenTelemetry metrics/tracing.
- Download generated patch sau khi Accept.

### 6.3. Ngoài phạm vi

- Hỗ trợ Java, Python hoặc JavaScript.
- Bitbucket.
- Tự động merge generated code.
- Kubernetes hoặc chia hệ thống thành nhiều microservice.
- Chatbot hỏi đáp source code tổng quát.
- Nhiều LLM provider chỉ để trình diễn.
- Mutation testing toàn bộ repository lớn ở mọi analysis.

## 7. Đánh giá baseline hiện tại

### 7.1. Phần đã có thể tái sử dụng

- Modular monolith gồm API và background worker.
- PostgreSQL migrations và pgvector.
- GitLab/GitHub SCM abstraction và webhook verification.
- Go AST parser và diff-to-symbol mapping.
- Project-isolated hybrid retrieval.
- OpenAI Responses API với strict structured output.
- Recommendation, generation, validation và bounded repair pipeline.
- Docker sandbox có network/resource/privilege limits.
- Review decision và version history.
- Evaluation CLI xuất JSON, CSV, Markdown và SVG.
- GitLab CI cho lint, unit, integration, build và sandbox checks.
- Next.js console cho project, analysis, evidence và review.

### 7.2. Khoảng trống quan trọng

| Nhóm | Khoảng trống | Mức ưu tiên |
|---|---|---:|
| RAG | Embedding hiện là deterministic hash, chưa phải semantic embedding | P0 |
| Reproducibility | Chưa lưu immutable context và prompt snapshot của từng AI call | P0 |
| Index | Default-branch index có thể khác source commit của PR/MR | P0 |
| Evaluation | Dataset hiện tại chỉ là controlled fixture | P0 |
| Quality | Chưa thu coverage và mutation score tự động | P0 |
| Analysis | Chưa có type/call graph và impact propagation | P1 |
| Security | Chưa có authentication, RBAC và verified reviewer identity | P1 |
| UX | Chưa realtime và chưa có pipeline/evidence visualization | P1 |
| Operations | Chưa có metrics, tracing, alerting và job replay UI | P1 |
| Documentation | Specification cũ chưa phản ánh GitHub và trạng thái mới | P1 |

## 8. Kiến trúc mục tiêu

```text
                         +----------------------+
GitHub/GitLab webhook -->| API + Auth + RBAC    |
                         +----------+-----------+
                                    |
                                    v
                         +----------------------+
                         | Durable job workflow |
                         +----------+-----------+
                                    |
              +---------------------+----------------------+
              |                     |                      |
              v                     v                      v
     +----------------+    +------------------+    +----------------+
     | Commit indexer |    | Impact analyzer  |    | Provenance     |
     | semantic RAG   |    | AST/types/SSA/CG |    | snapshots      |
     +--------+-------+    +---------+--------+    +--------+-------+
              +---------------------+-----------------------+
                                    |
                                    v
                       +--------------------------+
                       | Recommendation + Generate|
                       +------------+-------------+
                                    |
                                    v
                       +--------------------------+
                       | Isolated validation      |
                       | test + coverage + mutant |
                       +------------+-------------+
                                    |
                          fail -----+----- pass
                            |               |
                            v               v
                       +---------+     +----------------+
                       | Repair  |     | Human review   |
                       +----+----+     +--------+-------+
                            |                   |
                            +-------------------+
                                    |
                                    v
                       GitHub Check / GitLab Discussion
```

## 9. Các phase và workstream phát triển

Repo hiện đã hoàn thành roadmap kỹ thuật đến Phase 11. Kế hoạch đồ án tiếp tục
đánh số từ Phase 12 để lịch sử phát triển không bị đứt đoạn.

### 9.1. Roadmap theo Phase

| Phase | Tên | Thời gian | Kết quả chính |
|---|---|---:|---|
| 12 | Thesis Foundation & AI Provenance | Tuần 1-2 | Chốt đề tài và lưu được bằng chứng bất biến cho mọi AI call |
| 13 | Change Impact Analysis | Tuần 3-4 | Đồ thị symbol bị tác động bằng AST, types, SSA và call graph |
| 14 | Commit-aware Semantic RAG | Tuần 5 | Context đúng commit, semantic retrieval và ranking có giải thích |
| 15 | Verified Test Quality | Tuần 6 | Coverage, mutation score, flakiness và quality breakdown |
| 16 | Quality-guided Repair | Tuần 7 | Phân loại lỗi và sửa test dựa trên execution/mutation feedback |
| 17 | Evidence UX & SCM Feedback | Tuần 8-9 | Dashboard nổi bật và phản hồi trực tiếp lên GitHub/GitLab |
| 18 | Security & Observability | Tuần 10 | Authentication, RBAC, audit, metrics và tracing |
| 19 | Thesis Experiment & Delivery | Tuần 11-12 | Trial thật, phân tích thống kê, báo cáo và demo bảo vệ |

#### Phase 12 - Thesis Foundation & AI Provenance

**Trạng thái:** Hoàn thành ngày 2026-09-04.

**Mục tiêu:** Chuyển pipeline hiện tại thành một hệ thống có thể audit và tái
lập trước khi nâng cấp thuật toán.

Checklist:

- [x] Cập nhật `PROJECT_SPEC.md` cho GitHub/GitLab và định vị đề tài mới.
- [x] Chốt RQ1-RQ3, giả thuyết, phạm vi và metric chính.
- [x] Viết ADR cho commit-aware index và immutable context snapshot.
- [x] Thêm `llm_calls`, `context_snapshots` và `context_snapshot_items`.
- [x] Lưu source SHA, index generation, prompt/model version và config hash.
- [x] Lưu token usage, latency, provider response ID và estimated cost.
- [x] Gắn provenance vào recommendation, generation và repair.
- [x] Thêm API đọc và export evidence bundle.
- [x] Viết integration test cho immutability và project isolation.

Đầu ra:

- Updated specification và ADR.
- Database migration cùng repository/service tests.
- Evidence API và metadata tối thiểu trên UI.
- Một analysis bundle có thể dùng để audit.

Điều kiện hoàn thành Phase 12:

- Re-index project không thay đổi evidence lịch sử.
- Không có API key/secret trong database, log hoặc artifact.
- Mọi AI call đều truy ra được source, context, prompt, model và usage.

#### Phase 13 - Change Impact Analysis

**Trạng thái:** Hoàn thành ngày 2026-09-04.

**Mục tiêu:** Nâng analyzer từ changed-line overlap thành phân tích phạm vi tác
động xuyên symbol và package.

Checklist:

- [x] Load package tại source SHA bằng `go/packages`.
- [x] Type-check bằng `go/types`.
- [x] Xây SSA và call graph với thuật toán được pin.
- [x] Phát hiện callers, callees, interface implementations và type usage.
- [x] Liên kết impacted symbols với test hiện có.
- [x] Thêm impact relation, reason code và score.
- [x] Giới hạn traversal depth và số node.
- [x] Có fallback AST-only khi repository không type-check.
- [x] Tạo corpus được gán nhãn và đo precision/recall.

Đầu ra:

- Impact graph model và API.
- Analyzer fixtures cho direct/cross-package/interface/generic changes.
- Báo cáo benchmark analyzer.

Điều kiện hoàn thành Phase 13:

- Phân biệt được direct change và inferred impact.
- Mỗi edge có reason có thể giải thích.
- Analyzer failure không làm mất kết quả AST cơ bản.

#### Phase 14 - Commit-aware Semantic RAG

**Mục tiêu:** Truy xuất context đúng phiên bản và có chất lượng semantic thực
sự, thay cho việc phụ thuộc chủ yếu vào hash/lexical similarity.

Checklist:

- [ ] Tích hợp semantic embedding provider production.
- [ ] Pin embedding model, dimensions và preprocessing version.
- [ ] Index theo commit SHA hoặc tạo PR/MR overlay index.
- [ ] Kết hợp structural, lexical, semantic và impact scores.
- [ ] Lưu score breakdown, rank và retrieval configuration.
- [ ] Áp dụng context token budget và deduplication.
- [ ] Thêm repository-content security/prompt-injection tests.
- [ ] Tạo retrieval benchmark và relevance labels.
- [ ] Đo Recall@k, Precision@k, MRR hoặc nDCG.

Đầu ra:

- Semantic embedding client.
- Commit-aware index generation.
- Explainable hybrid retriever.
- Retrieval evaluation report.

Điều kiện hoàn thành Phase 14:

- Context luôn thuộc đúng project và đúng commit/index generation.
- Treatment retrieval tốt hơn baseline trên development corpus đã đóng băng.
- UI/API giải thích được vì sao mỗi chunk được chọn.

#### Phase 15 - Verified Test Quality

**Mục tiêu:** Đánh giá test bằng khả năng thực thi và phát hiện lỗi, không chỉ
dựa vào việc code compile hoặc test pass.

Checklist:

- [ ] Chạy baseline test suite trước khi chèn generated test.
- [ ] Thu coverage profile trước và sau generated test.
- [ ] Tính total coverage và changed-scope coverage delta.
- [ ] Chạy candidate lặp lại để phát hiện flakiness.
- [ ] Tích hợp scoped mutation runner trong sandbox.
- [ ] Giới hạn package, symbol, mutant count và timeout.
- [ ] Lưu killed/lived/uncovered/timed-out/non-viable mutants.
- [ ] Thiết kế quality score có breakdown minh bạch.
- [ ] Hiển thị warning khi test pass nhưng mutation score thấp.

Đầu ra:

- Coverage và mutation run models.
- Sandbox quality runner.
- Quality API và dashboard components.

Điều kiện hoàn thành Phase 15:

- Mỗi candidate đủ điều kiện có coverage before/after.
- Có demo một passing test không kill mutant và một test kill được mutant.
- Mutation workload giữ nguyên sandbox security invariants.

#### Phase 16 - Quality-guided Repair

**Mục tiêu:** Sử dụng đúng loại phản hồi để sửa generated test và không lãng phí
AI attempt cho lỗi hạ tầng.

Checklist:

- [ ] Phân loại syntax, compile, assertion, panic và timeout failures.
- [ ] Phân loại dependency và sandbox infrastructure failures.
- [ ] Tách worker retry khỏi AI repair attempt.
- [ ] Chuẩn hóa feedback đưa vào repair prompt.
- [ ] Cho phép surviving-mutant feedback bổ sung assertion/scenario.
- [ ] Lưu feedback hash, repair reason và version chain.
- [ ] Thêm per-analysis token/cost budget.
- [ ] Giữ hard limit cho số repair attempts.

Đầu ra:

- Failure classifier.
- Repair policy engine.
- Versioned repair evidence.

Điều kiện hoàn thành Phase 16:

- Có demo fail -> repair -> pass.
- Infrastructure failure không gọi LLM repair.
- Có demo dừng đúng giới hạn và chuyển sang human review.

#### Phase 17 - Evidence UX & SCM Feedback

**Mục tiêu:** Làm cho toàn bộ giá trị kỹ thuật nhìn thấy được trong một màn
hình và xuất hiện ngay trong workflow GitHub/GitLab.

Checklist:

- [ ] Pipeline timeline tự cập nhật bằng SSE hoặc polling có backoff.
- [ ] Diff viewer gắn changed/impacted symbols.
- [ ] Impact graph có filter và relation legend.
- [ ] Evidence explorer có score breakdown và historical snapshot.
- [ ] Hiển thị generated versions, validation, coverage và mutation results.
- [ ] Thêm reviewer rubric và structured rejection reasons.
- [ ] Cho phép tải accepted candidate dưới dạng patch.
- [ ] Tạo/cập nhật GitHub Check Run.
- [ ] Tạo GitLab MR note/discussion.
- [ ] Hoàn thiện connect/index/webhook onboarding wizard.
- [ ] Thêm Playwright E2E cho các luồng chính.

Đầu ra:

- Analysis evidence dashboard.
- GitHub/GitLab publisher.
- Guided onboarding và E2E suite.

Điều kiện hoàn thành Phase 17:

- Người mới hiểu được lý do, bằng chứng và chất lượng của generated test mà
  không đọc database hoặc worker log.
- PR/MR thật hiển thị trạng thái và link về analysis.
- Không cần thao tác database để chạy demo hoàn chỉnh.

#### Phase 18 - Security & Observability

**Mục tiêu:** Đóng các lỗ hổng khiến MVP chỉ phù hợp với local/trusted network
và bổ sung khả năng quan sát pipeline.

Checklist:

- [ ] Thêm authentication phù hợp môi trường triển khai.
- [ ] Thêm ADMIN, DEVELOPER, REVIEWER và VIEWER roles.
- [ ] Bảo vệ endpoint theo project membership.
- [ ] Lấy reviewer identity từ authenticated session.
- [ ] Hoàn thiện CSRF, cookie, session và SameSite policy.
- [ ] Thêm audit event cho integration, retry, publish và review.
- [ ] Thêm correlation ID xuyên toàn pipeline.
- [ ] Thu queue depth, phase latency, retry, failure, token và cost metrics.
- [ ] Thêm tracing và dashboard vận hành.
- [ ] Thêm job replay/retry có authorization và audit.

Đầu ra:

- Authentication/RBAC và audit log.
- Metrics/tracing dashboard.
- Admin recovery workflow.

Điều kiện hoàn thành Phase 18:

- Integration test xác nhận không thể truy cập chéo project.
- Reviewer name không còn do client tự khai.
- Có thể tìm nguyên nhân analysis chậm/lỗi bằng correlation ID và telemetry.

#### Phase 19 - Thesis Experiment & Delivery

**Mục tiêu:** Tạo bằng chứng thực nghiệm thật và đóng gói hệ thống để bảo vệ.

Checklist:

- [ ] Chốt repository/scenario dataset và sample-size plan.
- [ ] Freeze source commit, prompt, model, retrieval config và sandbox digest.
- [ ] Tự động thu observation từ production records.
- [ ] Chạy paired trials cho RQ1-RQ3.
- [ ] Randomize/counterbalance variant order.
- [ ] Áp dụng reviewer rubric và blinding nếu khả thi.
- [ ] Báo cáo confidence interval, effect size và missing-data policy.
- [ ] Sinh JSON, CSV, Markdown, chart và checksum artifacts.
- [ ] Viết các chương thiết kế, thực nghiệm, kết quả và giới hạn.
- [ ] Chạy deployment, backup và restore drill.
- [ ] Chuẩn bị slide, live demo và video fallback.

Đầu ra:

- Raw thesis dataset có checksum.
- Reproducible statistical report.
- Luận văn, slide, demo environment và video dự phòng.

Điều kiện hoàn thành Phase 19:

- Controlled fixture không được dùng làm bằng chứng kết luận.
- Report có thể tái tạo trên môi trường sạch.
- Demo GitHub và GitLab end-to-end hoạt động.
- Tất cả mục Definition of Done ở phần 18 được kiểm tra.

### 9.2. Backlog chi tiết theo workstream

#### WS1 - Provenance và reproducibility

Mục tiêu là tái dựng được chính xác mỗi kết quả AI.

Hạng mục:

- [ ] Thêm bảng `llm_calls`.
- [ ] Lưu phase: recommendation, generation hoặc repair.
- [ ] Lưu model name/version và provider response ID.
- [ ] Lưu prompt version, prompt hash và optional encrypted prompt artifact.
- [ ] Lưu input/output/total tokens, latency và estimated cost.
- [ ] Thêm `context_snapshots` và `context_snapshot_items`.
- [ ] Lưu chunk ID, content hash, score, rank và index generation.
- [ ] Lưu source SHA, target SHA và repository ref thực sự được sử dụng.
- [ ] Lưu sandbox image digest, command và limits.
- [ ] Cho phép export evidence bundle theo analysis.

Definition of Done:

- Cùng một analysis có thể xuất một bundle đủ metadata để audit.
- Re-index project không làm thay đổi context lịch sử của analysis cũ.
- Unit và integration test xác nhận không thể gắn chunk từ project khác.

#### WS2 - Change Impact Analysis

Mục tiêu là tìm không chỉ symbol trực tiếp bị sửa mà cả phạm vi có khả năng bị
ảnh hưởng.

Hạng mục:

- [ ] Load package bằng `go/packages` tại source SHA.
- [ ] Type-check bằng `go/types`.
- [ ] Xây SSA cho package trong phạm vi.
- [ ] Xây call graph với thuật toán được pin.
- [ ] Phát hiện callers/callees của changed function hoặc method.
- [ ] Phát hiện interface implementation liên quan.
- [ ] Liên kết symbol với test hiện có.
- [ ] Tính `impact_score` và ghi rõ reason codes.
- [ ] Giới hạn độ sâu propagation để tránh graph explosion.
- [ ] Thiết kế fallback về AST-only khi repository không type-check được.

Reason code đề xuất:

```text
DIRECT_CHANGE
CALLER_OF_CHANGED_SYMBOL
CALLEE_OF_CHANGED_SYMBOL
IMPLEMENTS_CHANGED_INTERFACE
USES_CHANGED_TYPE
EXISTING_TEST_FOR_SYMBOL
SAME_PACKAGE
```

Definition of Done:

- Có fixture cho direct call, interface, generic method và cross-package call.
- Báo cáo precision/recall trên một corpus được gán nhãn thủ công.
- UI phân biệt direct change và inferred impact.

#### WS3 - Commit-aware Semantic RAG

Mục tiêu là truy xuất context đúng phiên bản và có chất lượng semantic thực sự.

Hạng mục:

- [ ] Thêm semantic embedding provider production.
- [ ] Pin model và dimensions trong index generation.
- [ ] Index theo immutable commit SHA hoặc tạo PR overlay index.
- [ ] Kết hợp structural, lexical, semantic và graph score.
- [ ] Lưu từng thành phần score để giải thích kết quả.
- [ ] Deduplicate chunk theo symbol/content hash.
- [ ] Đặt token budget cho context builder.
- [ ] Chống prompt injection từ repository content.
- [ ] Xây retrieval benchmark có relevance labels.
- [ ] Đo Recall@k, Precision@k, MRR hoặc nDCG.

Ranking đề xuất:

```text
final_score =
    w_structural * structural_score
  + w_lexical    * lexical_score
  + w_semantic   * semantic_score
  + w_impact     * impact_score
  + w_test       * test_proximity_score
```

Không chỉnh trọng số dựa trên tập test cuối. Cần tách development set và
evaluation set.

Definition of Done:

- Context luôn thuộc đúng project và đúng commit/index generation.
- Chất lượng retrieval tốt hơn baseline hash/lexical trên benchmark đã đóng băng.
- Reviewer xem được lý do mỗi chunk được chọn.

#### WS4 - Test Quality Validation

Mục tiêu là đo test có khả năng phát hiện lỗi, không chỉ compile và pass.

Hạng mục:

- [ ] Chạy baseline test suite trước khi chèn generated test.
- [ ] Thu `go test -coverprofile` trước và sau khi chèn test.
- [ ] Parse total coverage và changed-symbol coverage.
- [ ] Chạy generated test lặp lại để phát hiện flakiness.
- [ ] Tích hợp mutation runner trong sandbox riêng.
- [ ] Chỉ mutate package hoặc symbol nằm trong impact scope.
- [ ] Lưu killed, lived, uncovered, timed-out và non-viable mutants.
- [ ] Thiết lập mutation timeout và maximum mutant count.
- [ ] Không tính equivalent/non-viable mutant như test failure thông thường.
- [ ] Tạo quality score có các thành phần minh bạch.

Quality score không được là black box. UI phải hiển thị từng metric độc lập,
sau đó mới hiển thị điểm tổng hợp.

Ví dụ:

```text
compile_valid       15 điểm
execution_valid     20 điểm
coverage_delta      15 điểm
mutation_score      30 điểm
human rubric        20 điểm
flaky               trừ 20 điểm
```

Trọng số cuối cùng phải được mô tả là quyết định thiết kế, không được trình bày
như một chuẩn khoa học phổ quát.

Definition of Done:

- Mỗi candidate có coverage before/after và mutation result nếu đủ điều kiện.
- Mutation job bị giới hạn CPU, RAM, PID, network và wall time.
- Một candidate pass nhưng mutation score thấp được cảnh báo rõ trên UI.

#### WS5 - Failure Taxonomy và Quality-guided Repair

Hạng mục:

- [ ] Phân loại `SYNTAX_ERROR`.
- [ ] Phân loại `COMPILE_ERROR`.
- [ ] Phân loại `ASSERTION_FAILURE`.
- [ ] Phân loại `PANIC`.
- [ ] Phân loại `TIMEOUT`.
- [ ] Phân loại `DEPENDENCY_UNAVAILABLE`.
- [ ] Phân loại `SANDBOX_INFRASTRUCTURE_ERROR`.
- [ ] Phân loại `SURVIVING_MUTANT`.
- [ ] Chỉ gửi lỗi có thể sửa bằng code vào AI repair.
- [ ] Infrastructure failure dùng worker retry, không tiêu tốn AI repair attempt.
- [ ] Cho phép mutation feedback bổ sung test thay vì sửa production code.
- [ ] Lưu repair reason, feedback hash và version chain.

Definition of Done:

- Có ít nhất một demo fail -> repair -> pass.
- Có một demo infrastructure failure không gọi LLM.
- Có một demo dừng đúng `MAX_REPAIR_ATTEMPTS`.
- Repair không bao giờ thay đổi production source trong snapshot.

#### WS6 - Evidence Dashboard và UX

Trang analysis mục tiêu gồm:

- [ ] Header có repository, PR/MR, commit và elapsed time.
- [ ] Pipeline timeline cập nhật tự động.
- [ ] Diff viewer với changed symbols.
- [ ] Impact graph.
- [ ] Retrieved evidence với score và lý do.
- [ ] Prompt/context provenance view.
- [ ] Recommendation cards.
- [ ] Generated test version comparison.
- [ ] Validation logs có phân loại lỗi.
- [ ] Coverage before/after.
- [ ] Mutation killed/survived list.
- [ ] Quality score breakdown.
- [ ] Accept, Reject và Download patch.
- [ ] Reviewer rubric và structured rejection reasons.

Onboarding project:

```text
Paste repository URL
  -> detect provider
  -> verify repository access
  -> select default branch
  -> show webhook configuration
  -> verify webhook delivery
  -> build initial index
  -> run sample analysis
```

Definition of Done:

- Không cần thao tác database để hoàn thành demo.
- Trạng thái worker xuất hiện mà không cần reload thủ công.
- UI hoạt động ở desktop và mobile cơ bản.
- Có Playwright E2E cho connect, index, review và error states.

#### WS7 - SCM Feedback

Hạng mục:

- [ ] Tạo GitHub Check Run khi analysis bắt đầu.
- [ ] Cập nhật Check Run khi pipeline hoàn tất.
- [ ] Gắn annotation vào file/dòng có khuyến nghị.
- [ ] Tạo GitLab MR discussion hoặc note tổng hợp.
- [ ] Link về evidence dashboard.
- [ ] Hỗ trợ re-run có kiểm soát và idempotent.
- [ ] Không tự push hoặc merge code.

Definition of Done:

- Developer thấy trạng thái và kết quả ngay trong PR/MR.
- Webhook lặp không tạo nhiều Check/discussion không cần thiết.
- Permission lỗi được hiển thị như degraded integration, không làm mất analysis.

#### WS8 - Authentication, RBAC và audit

Role tối thiểu:

```text
ADMIN      quản lý integration và user
DEVELOPER  xem project/analysis và tải patch
REVIEWER   Accept/Reject candidate
VIEWER     chỉ đọc
```

Hạng mục:

- [ ] Authentication qua OIDC hoặc một provider phù hợp môi trường triển khai.
- [ ] Project membership.
- [ ] Backend authorization cho từng endpoint.
- [ ] Reviewer identity lấy từ session, không nhận tên tự khai từ client.
- [ ] CSRF/session/SameSite policy.
- [ ] Audit log cho connect, index, replay, review và export.
- [ ] Test truy cập chéo project.

#### WS9 - Observability và operations

Hạng mục:

- [ ] Correlation ID xuyên webhook, job, AI call và sandbox run.
- [ ] Metrics: queue depth, phase latency, retry, failure, token và cost.
- [ ] Distributed tracing cho pipeline.
- [ ] Dashboard vận hành.
- [ ] Alert cho stuck job, high failure rate và sandbox unavailable.
- [ ] Admin action để retry/replay job có audit.
- [ ] Readiness phân biệt required dependency và degraded dependency.

## 10. Thay đổi dữ liệu đề xuất

Tên bảng chỉ là đề xuất và cần được review trước khi tạo migration.

```text
analysis_runs
  id
  analysis_job_id
  source_sha
  target_sha
  index_generation
  pipeline_version
  configuration_hash

llm_calls
  id
  analysis_job_id
  phase
  model_name
  prompt_version
  prompt_hash
  provider_response_id
  input_tokens
  output_tokens
  latency_ms
  estimated_cost
  created_at

context_snapshots
  id
  analysis_job_id
  phase
  changed_symbol_id
  query
  embedding_model
  retrieval_config
  created_at

context_snapshot_items
  context_snapshot_id
  knowledge_chunk_id
  content_hash
  rank
  structural_score
  lexical_score
  semantic_score
  impact_score

impact_edges
  analysis_job_id
  from_symbol
  to_symbol
  relation
  score
  reason

coverage_runs
  generated_test_id
  coverage_before
  coverage_after
  changed_scope_before
  changed_scope_after
  profile_artifact_hash

mutation_runs
  generated_test_id
  tool_version
  config_hash
  killed
  lived
  uncovered
  timed_out
  non_viable
  score

audit_events
  actor_id
  project_id
  action
  resource_type
  resource_id
  metadata
  created_at
```

## 11. API đề xuất

Không bắt buộc giữ chính xác tên endpoint dưới đây; ưu tiên contract rõ và có
authorization.

```text
GET  /api/analyses/{id}/timeline
GET  /api/analyses/{id}/impact-graph
GET  /api/analyses/{id}/evidence
GET  /api/analyses/{id}/quality
GET  /api/analyses/{id}/events
GET  /api/analyses/{id}/export
POST /api/analyses/{id}/retry
POST /api/analyses/{id}/publish
GET  /api/generated-tests/{id}/coverage
GET  /api/generated-tests/{id}/mutations
GET  /api/generated-tests/{id}/patch
```

## 12. Thiết kế thí nghiệm

### 12.1. Biến thể

| Experiment | Baseline | Treatment |
|---|---|---|
| Context impact | `DIFF_ONLY` | `DIFF_IMPACT_RAG` |
| Repair impact | `GENERATE_ONLY` | `GENERATE_VALIDATE_REPAIR` |
| Human effort | `MANUAL` | `AI_ASSISTED` |

Nếu ngân sách cho phép, context experiment có thể có nhóm trung gian
`DIFF_LEXICAL_RAG` để tách tác động của semantic/impact retrieval.

### 12.2. Dataset

Mục tiêu ban đầu:

- 5 đến 8 Go repositories có giấy phép phù hợp.
- 30 đến 50 change scenarios.
- Bao gồm validation, branching, error handling, repository/service logic và
  interface changes.
- Pin repository, commit, task definition và expected behavior.
- Loại bỏ scenario bị trùng dữ liệu hoặc nằm trong prompt examples.
- Lưu raw observation và checksum.

Số lượng mẫu cuối cùng cần được quyết định bằng power analysis và trao đổi với
giảng viên hướng dẫn, không chỉ dựa vào con số thuận tiện.

### 12.3. Metrics

Primary metrics:

- compile success rate;
- execution success rate;
- mutation score;
- human acceptance rate;
- active human time.

Secondary metrics:

- syntax validity;
- first-pass success;
- repair success;
- final success;
- coverage delta;
- changed-scope coverage delta;
- token usage;
- estimated cost;
- end-to-end latency;
- flaky-test rate.

### 12.4. Kiểm soát thí nghiệm

- [ ] Dùng cùng repository commit cho mỗi cặp.
- [ ] Pin model, prompt version, retrieval config và sandbox digest.
- [ ] Randomize hoặc counterbalance thứ tự variant.
- [ ] Quy định warm-up và cache policy.
- [ ] Không thay prompt sau khi bắt đầu final trial.
- [ ] Reviewer dùng cùng rubric.
- [ ] Blind variant đối với reviewer nếu khả thi.
- [ ] Ghi nhận missing/failed observation thay vì tự chuyển thành false.
- [ ] Báo cáo confidence interval và effect size.
- [ ] Dùng paired statistical test phù hợp loại dữ liệu và giả định phân phối.

### 12.5. Reviewer rubric

Mỗi test được chấm độc lập theo thang 1-5:

- đúng behavior cần kiểm thử;
- chất lượng assertion;
- xử lý boundary/error cases;
- phù hợp convention của project;
- khả năng bảo trì;
- không overfit implementation detail;
- không tạo flaky behavior.

Ngoài điểm số, reviewer chọn một quyết định:

```text
ACCEPT
ACCEPT_WITH_MINOR_EDIT
REJECT_INCORRECT
REJECT_REDUNDANT
REJECT_WEAK_ASSERTION
REJECT_UNMAINTAINABLE
```

Nếu có từ hai reviewer, cần báo cáo mức độ đồng thuận và cách giải quyết bất đồng.

## 13. Lịch thực hiện các Phase trong 12 tuần

### Tuần 1 - Phase 12: Freeze đề tài và baseline

- Cập nhật `PROJECT_SPEC.md`, architecture và terminology GitHub/GitLab.
- Chốt RQ1-RQ3, scope và metrics.
- Chụp baseline test/CI và danh sách technical debt.
- Thiết kế schema provenance.

**Gate:** Giảng viên duyệt tên đề tài, câu hỏi nghiên cứu và phạm vi.

### Tuần 2 - Phase 12: Provenance foundation

- Tạo migration và repository cho LLM calls/context snapshots.
- Ghi token, latency, prompt hash và index generation.
- Hiển thị evidence metadata cơ bản trên API.

**Gate:** Re-index không thay đổi historical context của analysis cũ.

### Tuần 3-4 - Phase 13: Impact analyzer

- Thêm packages/type/SSA loading.
- Xây call graph và impact edges.
- Viết fixtures và benchmark precision/recall.
- Thêm fallback AST-only.

**Gate:** Demo được direct và indirect impacted symbols.

### Tuần 5 - Phase 14: Semantic, commit-aware RAG

- Tích hợp semantic embedding.
- Index/overlay đúng source SHA.
- Thêm explainable ranking và retrieval benchmark.

**Gate:** Treatment retrieval tốt hơn baseline trên development corpus.

### Tuần 6 - Phase 15: Coverage và mutation

- Thu coverage before/after.
- Tích hợp scoped mutation runner.
- Lưu artifacts và quality metrics.

**Gate:** Demo được một test pass nhưng không kill mutant và một test kill mutant.

### Tuần 7 - Phase 16: Repair nâng cao

- Failure taxonomy.
- Infrastructure retry tách khỏi AI repair.
- Mutation-guided improvement trên phạm vi giới hạn.

**Gate:** Demo fail -> repair -> pass và bounded termination.

### Tuần 8 - Phase 17: Evidence dashboard

- Pipeline timeline.
- Impact graph và evidence explorer.
- Coverage/mutation/quality panels.
- Candidate version diff và structured review.

**Gate:** Một người mới có thể hiểu toàn bộ analysis mà không đọc database/log.

### Tuần 9 - Phase 17: SCM feedback và onboarding

- GitHub Check Run.
- GitLab discussion/note.
- Webhook verification wizard.
- Download patch.

**Gate:** PR/MR thật hiển thị link và summary của analysis.

### Tuần 10 - Phase 18: Security và observability

- Authentication/RBAC tối thiểu.
- Verified reviewer identity.
- Correlation ID, metrics và tracing.
- Job replay có audit.

**Gate:** Không có endpoint dữ liệu nào truy cập chéo project trái phép.

### Tuần 11 - Phase 19: Final experiment

- Freeze model, prompt, repository set và sandbox image.
- Chạy trials theo protocol.
- Kiểm tra dữ liệu thiếu và checksum.
- Sinh bảng, chart và statistical report.

**Gate:** Có raw dataset thật, không dùng controlled fixture để kết luận.

### Tuần 12 - Phase 19: Hoàn thiện bảo vệ

- Viết chương thiết kế, thực nghiệm, kết quả và giới hạn.
- Chuẩn bị slide và video fallback.
- Chạy deployment/backup/restore drill.
- Rehearse demo thành công và demo failure-recovery.

**Gate:** Có thể dựng hệ thống và tái tạo report trên môi trường sạch.

## 14. Kịch bản demo bảo vệ

Thời lượng mục tiêu: 8 đến 10 phút.

1. Mở một Go repository đã kết nối.
2. Tạo PR/MR thay đổi một function có caller và interface liên quan.
3. Webhook tạo analysis; UI hiển thị pipeline realtime.
4. Impact graph chỉ ra symbol trực tiếp và gián tiếp bị ảnh hưởng.
5. Evidence explorer giải thích source/test/mock nào được truy xuất.
6. AI sinh test lần đầu có một lỗi compile hoặc assertion được chuẩn bị trước.
7. Sandbox phát hiện lỗi; repair loop tạo version mới.
8. Version mới chạy pass và tăng coverage.
9. Mutation runner chứng minh test kill được mutant mục tiêu.
10. Reviewer xem provenance và Accept candidate.
11. GitHub Check hoặc GitLab discussion chuyển sang completed.
12. Mở evaluation report để trình bày kết quả thực nghiệm tổng hợp.

Luôn chuẩn bị video demo dự phòng và một analysis đã xử lý sẵn để tránh phụ
thuộc mạng hoặc LLM API trong buổi bảo vệ.

## 15. Chiến lược kiểm thử

### Unit tests

- Diff parser và changed-line mapping.
- Type/call graph relation extraction.
- Retrieval ranking và token budget.
- Failure classifier.
- Coverage/mutation parser.
- Quality score.
- Prompt/schema validation.

### Integration tests

- Project isolation trong mọi retrieval query.
- Provenance transaction.
- Job lease/retry/replay.
- Context snapshot không đổi sau re-index.
- Review authorization và latest-version invariant.
- Migration up/down/upgrade.

### Sandbox tests

- Network disabled.
- Read-only root filesystem.
- CPU/RAM/PID/time/output limits.
- Coverage artifact extraction.
- Mutation timeout và cleanup.
- Không mount Docker socket hoặc application secret vào sandbox.

### End-to-end tests

- Connect project -> index -> webhook -> review.
- GitHub và GitLab fixture flows.
- Fail -> repair -> pass.
- Retry infrastructure failure.
- Accept và Reject.
- SCM publish success/failure.
- Authentication và project authorization.

## 16. Rủi ro và phương án giảm thiểu

| Rủi ro | Tác động | Giảm thiểu |
|---|---|---|
| LLM API không ổn định | Demo hoặc trial thất bại | Retry có giới hạn, lưu response, video fallback |
| Chi phí thí nghiệm cao | Không đủ số replicate | Dry-run corpus, budget per analysis, cache immutable inputs |
| Mutation testing chậm | Pipeline timeout | Chỉ mutate impacted scope, giới hạn mutant và chạy async |
| Repo không type-check | Không tạo được call graph | Fallback AST-only và ghi confidence thấp |
| Repo cần external services | Sandbox không chạy được | Chọn dataset phù hợp hoặc định nghĩa trusted test-service policy |
| Context chứa secret | Rò rỉ dữ liệu sang LLM | Path policy, secret scan, redaction và explicit consent |
| Prompt injection trong repo | Output sai hoặc exfiltration | Delimiter, instruction hierarchy, adversarial tests, egress policy |
| Dataset thiên lệch | Kết luận yếu | Nhiều repo/scenario, randomization, công khai tiêu chí chọn mẫu |
| Scope quá lớn | Không hoàn thành | Giữ Go-only, không auto merge, ưu tiên P0 trước |

## 17. Thứ tự ưu tiên khi thiếu thời gian

### P0 - Bắt buộc để thành đồ án có bằng chứng

1. Immutable prompt/context/model provenance.
2. Commit-aware semantic RAG.
3. Coverage và scoped mutation evaluation.
4. Automated experiment collector.
5. Dataset và trial thật.

### P1 - Tạo điểm nổi bật sản phẩm

1. Change impact graph.
2. Evidence dashboard.
3. Failure taxonomy và quality-guided repair.
4. GitHub Check/GitLab discussion.
5. Authentication và verified reviewer.

### P2 - Hoàn thiện vận hành

1. OpenTelemetry dashboard.
2. Admin replay UI.
3. Performance optimization cho repository lớn.
4. Visual regression và accessibility test đầy đủ.

## 18. Definition of Done cho đồ án

Đồ án chỉ được xem là sẵn sàng bảo vệ khi:

- [ ] Một GitHub PR và một GitLab MR chạy end-to-end.
- [ ] Generated test luôn chạy trong sandbox.
- [ ] Có ít nhất một trace repair thành công và một trace dừng đúng giới hạn.
- [ ] Có coverage before/after.
- [ ] Có mutation score hoặc lý do rõ ràng khi không thể chạy mutation.
- [ ] Có immutable evidence cho mọi AI call.
- [ ] Có impact graph và lý do retrieval trên UI.
- [ ] Reviewer identity được xác thực.
- [ ] Có dataset trial thật với checksum.
- [ ] Có paired comparison cho RQ1-RQ3.
- [ ] Báo cáo confidence interval/effect size, không chỉ phần trăm mô tả.
- [ ] CI chạy unit, integration, sandbox, frontend và migration tests.
- [ ] Có threat model và nêu rõ residual risks.
- [ ] Có deployment guide, backup/restore drill và demo video dự phòng.
- [ ] Có chương giới hạn nghiên cứu và không phóng đại kết quả.

## 19. Sprint đầu tiên nên thực hiện

Sprint đầu tiên kéo dài hai tuần và chỉ tập trung vào nền tảng có giá trị lâu dài:

### Sprint Goal

Mỗi recommendation, generation và repair phải truy vết được source, context,
prompt, model và usage đã tạo ra nó.

### Backlog

- [x] Cập nhật `PROJECT_SPEC.md` để phản ánh GitHub/GitLab và đề tài mới.
- [x] Viết ADR cho commit-aware index và context snapshot.
- [x] Thiết kế migration provenance.
- [x] Thêm `llm_calls` repository/service.
- [x] Thêm `context_snapshots` repository/service.
- [x] Gắn snapshot vào recommendation pipeline.
- [x] Gắn snapshot vào generation pipeline.
- [x] Gắn snapshot vào repair pipeline.
- [x] Lưu usage và latency từ OpenAI response.
- [x] Thêm API evidence read-only.
- [x] Viết integration test cho project isolation và immutability.
- [x] Hiển thị provenance metadata tối thiểu trên analysis page.

### Sprint acceptance criteria

- Một analysis hoàn chỉnh có ba loại trace: recommendation, generation và repair.
- Trace có model, prompt hash, token usage, latency và context item hashes.
- Re-index không làm thay đổi evidence của trace cũ.
- Test xác nhận không thể tham chiếu context khác project.
- Không lưu API key hoặc secret trong database/log/artifact.

## 20. Nguyên tắc ra quyết định

Khi phải chọn giữa nhiều tính năng, ưu tiên theo thứ tự:

1. Tăng độ tin cậy của kết luận nghiên cứu.
2. Tăng khả năng truy vết và tái lập.
3. Tăng chất lượng generated test.
4. Làm rõ giá trị trong kịch bản demo.
5. Cải thiện khả năng vận hành.
6. Sau cùng mới mở rộng số ngôn ngữ, provider hoặc hạ tầng.

Mọi tính năng mới cần trả lời ít nhất một trong ba câu hỏi:

- Nó cải thiện metric nào?
- Nó đóng khoảng trống nghiên cứu nào?
- Nó làm cho bằng chứng hoặc demo dễ hiểu hơn như thế nào?

Nếu không trả lời được, tính năng đó không nên nằm trong phạm vi đồ án hiện tại.
