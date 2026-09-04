# Phase 1–10: lưu ý, giới hạn và backlog sau MVP

Cập nhật lần đầu: **2026-08-28**  
Phạm vi rà soát: mã nguồn, migration, API, frontend, Docker Compose và tài liệu từ Phase 1 đến Phase 10.

Tài liệu này là nơi duy nhất để theo dõi những phần đã chủ động hoãn lại, rủi
ro còn tồn tại và tiêu chí cần đạt trước khi xem sản phẩm sẵn sàng triển khai
hoặc sử dụng làm bằng chứng luận văn. Một mục xuất hiện ở đây không đồng nghĩa
với việc hệ thống hiện tại đang có bug.

## Cách sử dụng

Trạng thái:

- `OPEN`: cần thực hiện hoặc ra quyết định.
- `VERIFY`: đã có biện pháp một phần nhưng phải kiểm chứng thêm trong môi trường thật.
- `ACCEPTED_MVP`: giới hạn đã được chấp nhận trong MVP; chỉ mở lại nếu làm Phase 12.
- `DONE`: đã đóng, phải kèm bằng chứng kiểm thử hoặc tài liệu.

Mốc xử lý:

- `P11`: phải xem xét trong Phase 11 — Deployment, CI/CD và Hardening.
- `THESIS`: phải đóng trước khi chốt số liệu/kết luận luận văn.
- `P12`: mở rộng tùy chọn sau khi MVP ổn định.

Khi xử lý một mục, không xóa dòng. Hãy đổi trạng thái, thêm ngày đóng, PR/commit,
test hoặc tài liệu chứng minh vào cột **Điều kiện đóng / ghi chú**.

Snapshot sau Phase 13 có **60 mục theo phase**: 31 `OPEN`, 16 `VERIFY`,
7 `ACCEPTED_MVP` và 6 `DONE`. Bảng ưu tiên cao bên dưới gồm 8 mục tóm tắt xuyên phase nên
không cộng thêm vào tổng số này. Hãy cập nhật snapshot khi trạng thái thay đổi.

## Các mục ưu tiên cao nhất

| ID | Trạng thái | Ưu tiên | Mốc | Vấn đề |
|---|---|---:|---|---|
| X-01 | OPEN | P0 | P11 | API và UI chưa có đăng nhập, phân quyền hoặc tenant boundary. Ngoại trừ webhook secret, các endpoint hiện chỉ phù hợp môi trường local hoặc mạng tin cậy. |
| X-02 | OPEN | P0 | P11 | Worker là control plane tin cậy và đang nhận Docker socket của host; nếu worker bị chiếm quyền thì host Docker cũng có rủi ro. |
| X-03 | OPEN | P0 | THESIS | Dataset Phase 10 hiện là controlled fixture, chưa phải dữ liệu thực nghiệm và không được dùng để kết luận luận văn. |
| X-04 | VERIFY | P1 | P11 | Đã thêm GitLab CI cho lint, unit, integration, sandbox, image build và migration round-trip; cần xác nhận một pipeline thật trên GitLab runner. |
| X-05 | VERIFY | P1 | P11 | Đã có secret-file production Compose, backup/restore drill và deployment runbook; TLS/reverse proxy và rollback trên host thật vẫn cần xác nhận. |
| X-06 | DONE | P1 | P11 | Đóng 2026-09-04: Phase 12 lưu immutable prompt/schema/response, index generation và context chunk snapshots cho recommendation, generation và repair; có evidence API/export. |
| X-07 | OPEN | P1 | THESIS | Coverage, human effort và human acceptance chưa được thu tự động từ trial thật; hiện được nhập qua dataset. |
| X-08 | OPEN | P1 | P11 | Chưa có metrics/tracing/alerting, rate limit và operational dashboard; hiện chủ yếu dựa vào structured log và timestamp DB. |

## Phase 1 — Backend foundation và database

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P1-01 | OPEN | P0 | P11 | `POST/GET` project và các API đọc khác chưa được bảo vệ bởi authentication/authorization. ID số có thể bị đoán. | Có identity, RBAC/project authorization, test truy cập chéo và audit actor. |
| P1-02 | OPEN | P2 | P11 | Project API mới có create/list/get; chưa có update, disable hoặc delete dù tài liệu ban đầu dùng từ “CRUD”. | Chốt rõ contract read/create-only hoặc thêm lifecycle endpoint an toàn cùng test cascade. |
| P1-03 | OPEN | P1 | P11 | Bảng `gitlab_connections` đã tồn tại nhưng runtime chưa sử dụng; GitLab/GitHub token và webhook secret vẫn là cấu hình toàn cục theo provider. | Hoặc wiring credential theo project với mã hóa/KMS, hoặc migration loại bỏ schema không dùng và cập nhật thiết kế. |
| P1-04 | OPEN | P1 | P11 | `/ready` chỉ kiểm tra PostgreSQL; không phản ánh GitLab, Docker daemon, sandbox image hay cấu hình provider. | Định nghĩa dependency nào là readiness bắt buộc, dependency nào chỉ báo degraded; thêm test. |
| P1-05 | VERIFY | P0 | P11 | Development Compose vẫn dùng credential local; production Compose đã tách secret file, data network và loopback port. TLS/reverse proxy trên host thật chưa được xác nhận. | Có file/stack production tách biệt, secret injection, network nội bộ, TLS và không dùng default credential. |
| P1-06 | VERIFY | P1 | P11 | Đã có migration one-shot, CI round-trip và backup→restore test trên production Compose tạm; cần restore drill định kỳ trên host triển khai. | Restore thử thành công, migration up/down được CI kiểm tra trên DB sạch và DB nâng cấp. |

## Phase 2 — GitLab integration

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P2-01 | OPEN | P1 | P11 | Mỗi provider dùng một token và webhook secret toàn cục cho mọi project. Điều này chưa phù hợp multi-user/multi-group. | Credential được scope theo project/group, mã hóa khi lưu và không xuất hiện trong log/UI. |
| P2-02 | ACCEPTED_MVP | P3 | P12 | Hỗ trợ GitLab MR `open`/`reopen`/`update` và GitHub PR `opened`/`reopened`/`synchronize`; chưa xử lý Bitbucket hoặc workflow đóng/merge. | Chỉ mở lại nếu Phase 12 mở rộng SCM/lifecycle. |
| P2-03 | VERIFY | P1 | P11 | GitLab client giới hạn response 20 MiB và tối đa 100 trang. Diff `collapsed`/`too_large` được lưu nhưng Phase 3 sẽ dừng phân tích. | Có UX/status rõ ràng và phương án fetch diff/file thay thế hoặc kết thúc job với lỗi có thể hành động. |
| P2-04 | OPEN | P2 | P11 | Dedupe hiện dựa vào `X-Gitlab-Webhook-UUID`; chưa có retention policy cho raw webhook và UUID. | Xác định thời gian lưu, cleanup, privacy policy và test replay sau retention. |
| P2-05 | OPEN | P2 | P11 | Chưa có endpoint/manual replay có kiểm soát cho job thất bại; vận hành hiện phải tạo event mới hoặc thao tác DB. | Có retry/replay được phân quyền, idempotent và ghi audit trail. |

## Phase 3 — Go change analyzer

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P3-01 | ACCEPTED_MVP | P3 | P12 | Analyzer chỉ hỗ trợ Go. | Multi-language chỉ thực hiện nếu Phase 12 được duyệt. |
| P3-02 | DONE | P2 | THESIS | Phase 13 dùng `go/packages`, `go/types`, SSA/CHA và graph có caller/callee/interface/type/test reasons; direct overlap vẫn được giữ riêng. | Đóng 2026-09-04: migration 000014, impact API, labelled corpus và PostgreSQL integration test. |
| P3-03 | VERIFY | P1 | P11 | File Go có diff collapsed/too-large làm cả analysis lỗi thay vì tạo partial result. | Chọn policy fail-fast hay partial; UI phải phân biệt “không đổi” với “không phân tích được”. |
| P3-04 | VERIFY | P2 | THESIS | Phase 13 có controlled corpus cho cross-package/interface/generic/test link và benchmark lặp lại; chưa có corpus lớn cho rename, build tag, cgo và generated source. | Mở rộng corpus thật trước Phase 19; không dùng baseline nhỏ để tuyên bố độ chính xác ngoài phạm vi. |

## Phase 4 — Knowledge index và RAG

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P4-01 | VERIFY | P0 | P11 | `project_id` đã được ép trong retrieval SQL và có isolation test. Mọi query mới vẫn có thể làm regression. | Giữ integration test bắt buộc trong CI; security review xác nhận không có đường retrieve chéo project. |
| P4-02 | OPEN | P1 | THESIS | Embedding runtime hiện chỉ có deterministic `hash-v1`; chưa có semantic embedding provider production. | Chọn/pin model, lưu model/version, đánh giá retrieval fixture và chi phí trước trial thật. |
| P4-03 | OPEN | P1 | P11 | Index lấy default branch, trong khi analysis chạy trên source/target SHA của MR. Context có thể không chứa code mới nhất của source branch. | Định nghĩa index theo commit/ref hoặc overlay MR; lưu ref/generation thực sự đã dùng. |
| P4-04 | DONE | P1 | P11 | Phase 12 snapshot nội dung/hash/rank của context và exact prompt cho từng LLM call. Evidence UI tách historical snapshot khỏi current-index context. | Đóng 2026-09-04: migration 000013, provenance integration test và evidence export. |
| P4-05 | VERIFY | P1 | P11 | Bộ lọc secret trong source là heuristic; có thể false negative/false positive và không thay thế DLP. | Security test với secret corpus, policy allow/deny path, redaction và quy trình xử lý incident. |
| P4-06 | ACCEPTED_MVP | P3 | P12 | Chỉ index Go, README và Markdown trong `docs/`; file >1 MiB, generated, vendor, node_modules và path nhạy cảm bị bỏ qua. | Giữ giới hạn nếu đủ cho thesis; chỉ mở rộng theo dữ liệu retrieval thực tế. |
| P4-07 | OPEN | P2 | THESIS | Trọng số hybrid ranking và giới hạn top-k chưa được hiệu chỉnh trên corpus đủ lớn. | Báo cáo retrieval relevance (ví dụ Recall@k/MRR hoặc rubric thủ công) và pin cấu hình cho trial. |

## Phase 5 — AI test recommendation

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P5-01 | OPEN | P0 | THESIS | `LLM_PROVIDER` mặc định `disabled`; workflow với model thật phụ thuộc API key/model riêng và chưa tạo bằng chứng luận văn chỉ từ test mock. | Chạy trial model thật với model/prompt/config được pin và lưu raw dataset hợp lệ. |
| P5-02 | DONE | P3 | P12 | Có OpenAI Responses API và Gemini Interactions API sau `llm.Provider`. | Đóng 2026-09-04: hai adapter dùng cùng request/schema, giới hạn response và provenance usage. |
| P5-03 | DONE | P1 | P11 | Usage token, latency, prompt/configuration hash và chi phí ước tính theo rate cấu hình được lưu trong `llm_calls`. | Đóng 2026-09-04: recorder không đưa API key/token/secret vào hash hoặc evidence. |
| P5-04 | OPEN | P1 | P11 | Diff, code và docs là prompt data không tin cậy; strict JSON chỉ bảo vệ output shape, không loại bỏ prompt injection/data exfiltration. | Threat model, content delimiters/policy, egress/data-sharing review và adversarial prompt tests. |
| P5-05 | OPEN | P1 | THESIS | Recommendation có trạng thái `USEFUL/PARTIALLY_USEFUL/NOT_USEFUL` nhưng chưa có workflow/API gán nhãn độc lập; review hiện quyết định trên generated test. | Có rubric và lưu nhãn recommendation bởi reviewer thật, kèm actor/timestamp. |
| P5-06 | OPEN | P2 | P11 | Mỗi changed symbol gọi LLM tuần tự; chưa có quota, rate-limit riêng, budget theo analysis hoặc circuit breaker. | Có giới hạn call/token/cost, backoff theo provider và trạng thái lỗi dễ quan sát. |

## Phase 6 — AI test generation

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P6-01 | VERIFY | P1 | P11 | Output đã được kiểm tra JSON, path, Go syntax, package và test name; type-check/compile chỉ xảy ra ở Phase 7. | Giữ ranh giới này và bảo đảm mọi candidate luôn qua sandbox trước review/final status. |
| P6-02 | ACCEPTED_MVP | P3 | P12 | Candidate phải là file `_test.go` mới nằm cạnh source; chưa thể sửa/ghép file test có sẵn hoặc tạo nhiều file cho một recommendation. | Chỉ mở rộng sau khi có conflict strategy và patch validation an toàn. |
| P6-03 | DONE | P1 | P11 | Generation lưu full prompt/schema/response, denormalized context snapshot, usage, latency, model và source/index identity. | Đóng 2026-09-04: `ai-provenance-v1` JSON export. |
| P6-04 | ACCEPTED_MVP | P3 | P12 | Không tự commit, push hoặc mở MR chứa test đã accept. | Đây là ranh giới an toàn MVP; chỉ thêm trong Phase 12 với quyền GitLab rõ ràng. |
| P6-05 | OPEN | P2 | THESIS | Chưa đo chất lượng assertion, oracle strength hoặc false-positive test ngoài compile/pass và human acceptance. | Bổ sung rubric; cân nhắc mutation testing trên tập con nếu không làm lệch scope. |

## Phase 7 — Docker test validation

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P7-01 | VERIFY | P0 | P11 | Sandbox đã non-root, no-network, read-only rootfs, drop capabilities và giới hạn CPU/RAM/PID/time; worker vẫn điều khiển host Docker qua socket. | Security review control plane; cân nhắc rootless/remote isolated runner và giới hạn quyền Docker API. |
| P7-02 | OPEN | P1 | P11 | Image mặc định dùng mutable tag `ai-test-assistant-sandbox:phase7`, chưa pin digest/SBOM/signature. | Pin immutable digest, scan CVE, tạo SBOM và quy trình rebuild image. |
| P7-03 | ACCEPTED_MVP | P2 | P12 | `GOPROXY=off`, `GOSUMDB=off`, `CGO_ENABLED=0`, network none. Repo cần external module phải commit `vendor/`; project cần DB/service/network/cgo sẽ không chạy. | Giữ cho thesis hoặc thiết kế trusted immutable dependency cache/test-service policy ở Phase 12. |
| P7-04 | OPEN | P2 | P11 | Snapshot tải từng file qua GitLab API, giới hạn mặc định 10.000 file/100 MiB; repo lớn có thể chậm hoặc vượt giới hạn. | Benchmark repo đại diện, cache/archive strategy và lỗi UI có hướng xử lý. |
| P7-05 | OPEN | P2 | THESIS | Chỉ chạy `go test -count=1 ./...`; chưa chạy coverage tự động, race detector, fuzz hoặc mutation. | Tối thiểu thêm pipeline thu coverage chuẩn hóa cho Phase 10 trial. |
| P7-06 | VERIFY | P1 | P11 | Stdout/stderr được giới hạn và redact bằng regex, nhưng không thể bảo đảm bắt mọi định dạng secret do test in ra. | Mở rộng test corpus, hạn chế secret được đưa vào sandbox và đặt retention/access control cho log. |
| P7-07 | OPEN | P2 | P11 | Chưa có custom seccomp/AppArmor profile và chưa kiểm thử trên runner production/rootless. | Hoàn tất sandbox threat model và penetration/adversarial test trên hạ tầng triển khai thật. |

## Phase 8 — AI repair loop

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P8-01 | VERIFY | P0 | P11 | Loop đã có hard maximum 3 và append version bất biến. Đây là invariant không được bỏ. | CI giữ test termination, concurrent lease và stale version. |
| P8-02 | OPEN | P1 | THESIS | Cần trial model thật chứng minh ít nhất một fail→repair→pass và một trường hợp dừng ở max attempt; fixture/test mock chưa đủ làm bằng chứng. | Lưu validation/repair trace vào dataset thực nghiệm. |
| P8-03 | OPEN | P2 | P11 | Timeout, compile failure và assertion failure đều có thể đi vào repair; chưa phân loại lỗi hạ tầng/dependency để tránh tốn lượt AI vô ích. | Error taxonomy quyết định retry infrastructure, repair code hoặc chuyển review. |
| P8-04 | DONE | P1 | P11 | Candidate fail sau max attempt vẫn được chuyển `WAITING_REVIEW` để reviewer xem và Reject. | Đóng 2026-08-28: backend từ chối Accept nếu validation mới nhất của candidate hiện tại không `PASSED`; UI khóa nút Accept, hiển thị lý do, Reject vẫn hoạt động; có unit và PostgreSQL integration test. |
| P8-05 | VERIFY | P2 | P11 | Phase 12 đã persist usage/latency/prompt/context/cost cho repair; per-analysis hard budget vẫn chưa có. | Đóng hoàn toàn khi Phase 16 thêm policy từ chối call vượt budget. |

## Phase 9 — Human review UI

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P9-01 | OPEN | P0 | P11 | Reviewer name là input tự khai, mặc định `local-reviewer`; không có login, session, RBAC hoặc danh tính kiểm chứng. | Decision lấy actor từ identity backend, không tin `reviewer_name` do client gửi. |
| P9-02 | VERIFY | P1 | P11 | Decision bất biến và chống double-click/concurrency; chưa có quy trình correction/appeal nếu reviewer thao tác nhầm. | Chốt policy append-only correction có audit hoặc xác nhận tính bất biến là yêu cầu chính thức. |
| P9-03 | DONE | P1 | P11 | UI giữ current-index context riêng và thêm AI provenance panel dùng historical index/chunk metadata cùng full evidence export. | Đóng 2026-09-04. |
| P9-04 | OPEN | P2 | P11 | UI chưa realtime/polling; trạng thái worker mới chỉ thấy sau refresh/navigation. | Thêm polling/SSE hoặc ghi rõ behavior, có retry/backoff và trạng thái stale. |
| P9-05 | VERIFY | P1 | P11 | Đã có typecheck/build, route + security-header smoke, manual headless-browser review cho overview/project/review/evaluation và responsive CSS; chưa có browser visual regression tự động trong CI. | Thêm test Accept/Reject, error states, keyboard/a11y và viewport chính vào CI để đóng hoàn toàn. |
| P9-06 | OPEN | P2 | P11 | Diff, source và log lớn có thể làm review page nặng; chưa có pagination/virtualization/download artifact. | Benchmark payload đại diện và giới hạn/render strategy. |
| P9-07 | VERIFY | P1 | P11 | Đã có API rate limit, timeout/request limits, CSP và security headers; CSRF/session, authentication và reverse-proxy policy vẫn chưa đóng. | Hoàn thiện cùng authentication, CSP, CSRF/SameSite, request limit và reverse proxy. |

## Phase 10 — Evaluation và thesis experiments

| ID | Trạng thái | Ưu tiên | Mốc | Lưu ý | Điều kiện đóng / ghi chú |
|---|---|---:|---|---|---|
| P10-01 | OPEN | P0 | THESIS | `controlled-v1.json` chỉ kiểm thử pipeline/report. Các con số của nó tuyệt đối không được trích làm kết quả nghiên cứu. | Thay bằng raw observations thu từ trial thật và giữ fixture chỉ cho regression. |
| P10-02 | OPEN | P0 | THESIS | Chưa chốt model/version, repository commit, task set, prompt config, sandbox image digest và warm-up policy cho experiment A/B/C. | Protocol được freeze trước trial; mọi run lưu đủ metadata. |
| P10-03 | OPEN | P1 | THESIS | Chưa có số replicate tối thiểu, randomization/counterbalancing, blinding reviewer, confidence interval hoặc significance/effect-size analysis. | Có kế hoạch thống kê được giảng viên chấp thuận và script phân tích tái lập. |
| P10-04 | OPEN | P1 | THESIS | Coverage before/after và duration hiện nhập thủ công; chưa có collector liên kết trực tiếp validation/review records với observation. | Collector tạo observation có source IDs, tool version và kiểm tra dữ liệu thiếu. |
| P10-05 | OPEN | P1 | THESIS | Human acceptance chưa có rubric chuẩn hóa, reviewer identity, inter-rater agreement hoặc lý do reject có cấu trúc. | Rubric, training/calibration, reviewer metadata và agreement report hoàn tất. |
| P10-06 | VERIFY | P1 | P11 | Import theo SHA-256 là idempotent và API đọc lại kiểm tra hash; DB role có quyền vẫn có thể sửa/xóa row trực tiếp. | Tách DB role, hạn chế write, backup raw dataset và ký/ghi checksum artifact ngoài DB. |
| P10-07 | OPEN | P2 | THESIS | Report hiện tổng hợp descriptive rate/mean và biểu đồ SVG; chưa có paired statistical model, missing-data policy chi tiết hoặc export notebook. | Thêm script/notebook pin dependency, paired analysis và data dictionary. |
| P10-08 | OPEN | P2 | P11 | Import chỉ có CLI và API chỉ đọc — an toàn cho MVP nhưng chưa có authorization/audit cho operator chạy import trong production. | Chạy như CI/admin job có identity, artifact retention và audit log; không mở public upload endpoint. |

## Invariant đã có — không được regression

Các cơ chế dưới đây đang hoạt động và phải tiếp tục có test khi Phase 11/12 thay đổi kiến trúc:

| Invariant | Bằng chứng hiện tại cần giữ |
|---|---|
| RAG không được trả chunk khác project | `project_id` nằm trong SQL retrieval và integration isolation test. |
| Webhook lặp không tạo analysis trùng | Unique webhook UUID và test dedupe. |
| Worker crash không khóa job vô hạn | Lease, renew, retry và stale-claim checks. |
| Generated code không chạy trong API/worker process | Chỉ Phase 7 Docker sandbox thực thi. |
| Sandbox không có network/privilege mặc định | `--network=none`, non-root, read-only, drop capabilities, resource/time limits. |
| Repair loop luôn kết thúc | `MAX_REPAIR_ATTEMPTS` hard maximum 3. |
| Repair không sửa đè version cũ | Generated version và repair attempt được append trong transaction. |
| Review không quyết định stale candidate | Backend lock + latest-version check + immutable decision. |
| Lỗi validation/repair không bị UI che giấu | Review page hiển thị toàn bộ history. |
| Evaluation không đánh đồng pass với usefulness | Syntax, compile, execution, coverage và human acceptance là metric riêng. |
| Evaluation dataset import không nhân đôi | SHA-256 unique và round-trip hash verification. |

## Ranh giới MVP được chấp nhận

Những nội dung sau không cần “sửa” trong Phase 11 trừ khi scope thay đổi:

- Chỉ hỗ trợ Go; SCM hỗ trợ GitLab và GitHub.
- Không tự commit/push/merge generated test.
- Không bắt buộc Kubernetes hoặc tách hệ thống thành nhiều microservice.
- Không bắt buộc mutation testing, release-risk scoring hoặc log root-cause analysis.
- Hai LLM provider thật (OpenAI và Gemini) là đủ cho phạm vi đồ án.
- Review decision là hành động của con người; AI không tự merge code.

Nếu thực hiện, các nội dung này thuộc Phase 12 và không được làm giảm chất lượng
thí nghiệm cốt lõi.

## Thứ tự giải quyết đề xuất

1. **Security boundary:** X-01, X-02, P1-05, P7-01, P9-01, P9-07.
2. **Reproducibility/provenance:** X-06, P4-03, P4-04, P5-03, P6-03, P8-05.
3. **CI và vận hành:** X-04, X-05, X-08, P1-04, P1-06, P7-02.
4. **Sandbox compatibility:** P7-03 đến P7-07 và P8-03/P8-04.
5. **Thí nghiệm thật:** X-03, X-07 và toàn bộ P10-01 đến P10-07.
6. **UX/audit hoàn thiện:** P5-05 và P9-02 đến P9-06.
7. **Phase 12:** chỉ mở các mục `ACCEPTED_MVP` sau khi năm nhóm trên ổn định.

## Checklist đóng Phase 11 trước khi thu số liệu cuối

- [ ] CI chạy lint, unit, PostgreSQL integration, sandbox, frontend build/E2E và migration test.
- [ ] Authentication/RBAC và reviewer identity hoạt động end-to-end.
- [ ] Secret production, TLS, backup và restore drill đã được tài liệu hóa/test.
- [ ] Sandbox/control-plane security review không còn finding P0/P1 chưa xử lý.
- [ ] Model, prompt, embedding, source commit, sandbox image và context provenance được pin/lưu.
- [ ] Trial protocol, rubric, sample size, randomization và statistical plan được freeze.
- [ ] Coverage/time/acceptance collector tạo dataset từ record thật, không nhập số giả.
- [ ] Raw dataset, report, chart và checksum có thể tái tạo trên máy sạch.
- [ ] Controlled fixture được gắn nhãn rõ và loại khỏi mọi bảng kết quả luận văn.
