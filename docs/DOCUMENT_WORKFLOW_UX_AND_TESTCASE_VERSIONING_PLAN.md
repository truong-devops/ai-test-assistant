# Kế hoạch cải thiện trải nghiệm tài liệu → testcase và quản lý phiên bản testcase

- Ngày lập: 17/09/2026.
- Baseline khảo sát: commit `b6c88c3`.
- Trạng thái: **UV-00 đến UV-04 đã nghiệm thu; UV-05 còn nghiệm thu usability với người mới. UV-06 đã hoàn thành kiểm thử kỹ thuật. UV-07 đã qua kiểm thử kỹ thuật, còn nghiệm thu người dùng/screen reader. UV-08 đã hoàn thành kỹ thuật (schema 30): affected/selected/all, diff/apply, retry từng requirement, all-removed và E2E đổi nguồn → R2. UV-09 đang triển khai, các gate nghiệm thu người dùng/phát hành vẫn mở**.
- Phạm vi: frontend Next.js, backend Go, worker, PostgreSQL, nguồn tài liệu,
  requirements, testcase, baseline, automation, kết quả chạy và xuất báo cáo.
- Hai mục tiêu: người mới tự hoàn thành luồng sinh testcase; QA quản lý được
  phiên bản mà không mất lịch sử, nguồn bằng chứng hoặc bản đang sử dụng.

## 1. Cách sử dụng kế hoạch

Đây là kế hoạch tiếp nối
[Document-Driven Testing Refactor Plan](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md),
không thay thế các nguyên tắc nguồn sự thật trong
[ADR 0003](adr/0003-document-authority-versioning-and-mvp-boundary.md).

Các phase dùng tiền tố **UV-00 → UV-09** để không trùng số Phase 0–11 đã triển
khai hoặc roadmap lịch sử trong [Graduation Project Plan](GRADUATION_PROJECT_PLAN.md).
Onboarding trong file hướng dẫn demo không đồng nghĩa với onboarding đã có trên UI.

Đọc nhanh: mục 4 mô tả trải nghiệm mới; mục 5 chốt quy tắc phiên bản; mục 7 là
danh sách việc thực hiện theo phase; mục 8 là ma trận kiểm thử; mục 12 dùng để
theo dõi tiến độ. `P0` là phần phải sửa để bảo đảm dữ liệu/phát hành; `P1` là phần
chính của đợt cải thiện trải nghiệm, vẫn phải hoàn thành trước khi kết thúc kế hoạch.

Quy ước thực hiện:

- `[ ]`: chưa hoàn thành; áp dụng cho toàn bộ hạng mục triển khai bên dưới.
- `[x]`: chỉ đánh dấu sau khi có code, kiểm tra phù hợp và bằng chứng nghiệm thu.
- Mỗi phase phải ghi commit, kiểm tra đã chạy, giới hạn còn lại vào mục 12.
- Nội dung chưa đánh dấu vẫn là đề xuất; chỉ coi endpoint, bảng dữ liệu hoặc trạng
  thái là đã tồn tại khi checklist và bằng chứng phase tương ứng đã được cập nhật.
- Các phát hiện ban đầu dựa trên đọc code. Từ UV-00 trở đi, fixture và integration
  test PostgreSQL được dùng để chuyển từng phát hiện thành regression test.

## 2. Hiện trạng và nguyên nhân khó sử dụng

### 2.1. Trải nghiệm người dùng

| ID | Hiện trạng | Hậu quả | Bằng chứng hiện tại |
| --- | --- | --- | --- |
| UX-01 | Trang bộ tài liệu có ba link kỹ thuật, chưa có bước hiện tại/tiếp theo | Người dùng tự đoán thứ tự upload, duyệt, index, extract, generate | [Document set page](../frontend/app/documents/[id]/page.tsx) |
| UX-02 | Tạo bộ tài liệu chỉ refresh danh sách; upload chỉ refresh một lần sau khi enqueue parse | Phải mở lại bộ tài liệu hoặc tự refresh để biết đã xử lý xong chưa | [Create set](../frontend/components/create-document-set.tsx), [Upload](../frontend/components/upload-document.tsx) |
| UX-03 | Duyệt tài liệu nằm trong trang chi tiết, chỉ tên tài liệu là link | Không rõ phải bấm đâu; badge `Draft` không phải nút duyệt | [Document set page](../frontend/app/documents/[id]/page.tsx), [Document review](../frontend/components/document-review.tsx) |
| UX-04 | Nút extract/generate chưa nhận đủ thông tin điều kiện đầu vào | Người dùng bấm rồi mới nhận lỗi thiếu index hoặc requirement đã duyệt | [Requirements page](../frontend/app/documents/[id]/requirements/page.tsx), [Testcase page](../frontend/app/documents/[id]/test-cases/page.tsx) |
| UX-05 | Requirement review phải mở từng chi tiết; nhập lại tên reviewer | Nhiều thao tác lặp, khó duyệt một bộ tài liệu lớn | [Requirement inventory](../frontend/components/requirement-inventory.tsx), [Requirement review](../frontend/components/requirement-review.tsx) |
| UX-06 | Extraction có job/polling, nhưng index và testcase generation còn chạy trong request HTTP | Trải nghiệm chờ không đồng nhất; generation nhiều requirement có nguy cơ timeout | [Workflow handlers](../backend/internal/httpapi/document_workflow.go), [Testcase service](../backend/internal/testcase/service.go) |
| UX-07 | Lỗi job `FAILED` khởi tạo từ server chưa được đưa vào state lỗi; lỗi mạng lúc start chưa được catch | Tải lại trang có thể chỉ thấy Retry, mất lý do thất bại hoặc không có hướng dẫn | [Extraction action](../frontend/components/requirement-extraction-action.tsx) |
| UX-08 | Giao diện hiển thị Phase, semantic chunk, hash và các trạng thái nội bộ ở luồng chính | Người dùng nghiệp vụ phải hiểu kiến trúc hệ thống mới thao tác được | [Index page](../frontend/app/documents/[id]/index/page.tsx), [Testcase detail](../frontend/app/documents/[id]/test-cases/[testCaseId]/page.tsx) |

### 2.2. Dữ liệu và phiên bản

| ID | Phát hiện từ code | Rủi ro cần xử lý | Bằng chứng |
| --- | --- | --- | --- |
| DATA-01 | Upload bản mới không invalidation index; request extraction chỉ kiểm tra `READY` | `Ready` có thể thuộc bộ tài liệu cũ | [Document repository](../backend/internal/document/repository.go), [Requirement service](../backend/internal/requirement/service.go) |
| DATA-02 | Index giữ chunk lịch sử; `AllChunks` gọi truy vấn không lọc membership của generation | Chunk v1 và v2 có thể cùng trở thành subject extraction, trong khi retrieval dùng `LATEST` | [Index service](../backend/internal/document/index_service.go), [Index repository](../backend/internal/document/index_repository.go) |
| DATA-03 | Fingerprint index không có approval state, nhưng warning count được lưu khi build | Sau duyệt nguồn, cảnh báo draft có thể còn cũ dù bấm re-index | [Index repository](../backend/internal/document/index_repository.go), [Index service](../backend/internal/document/index_service.go) |
| VER-01 | Đã có `version_number`, `supersedes_test_case_id`, lưu mới khi edit; UI chỉ hiện một số version | Có nền versioning nhưng chưa có history, diff, restore, chọn bản dùng | [Testcase model](../backend/internal/testcase/model.go), [Testcase repository](../backend/internal/testcase/repository.go) |
| VER-02 | `logicalCaseKey` chỉ hash `test_type + requirement_keys` | Hai kịch bản NEGATIVE của cùng requirement bị coi thành hai version của cùng testcase; danh sách chỉ giữ bản cuối | [logicalCaseKey/Save](../backend/internal/testcase/repository.go) |
| VER-03 | `generationKey` dựa vào requirement IDs, type, title, expected; không gồm đầy đủ steps/data/preconditions | Thay đổi dữ liệu/bước thực hiện có thể bị coi là không đổi | [proposalIdentity](../backend/internal/testcase/generator.go), [generationKey](../backend/internal/testcase/repository.go) |
| VER-04 | Danh sách và baseline query loại tất cả bản có bản kế tiếp, dù bản kế tiếp mới là draft | Tạo v2 draft có thể làm v1 approved biến mất khỏi tập được chọn cho lần chạy mới | [Testcase repository](../backend/internal/testcase/repository.go), [Scope repository](../backend/internal/scope/repository.go) |
| VER-05 | Edit và approve ghép chung; FE refresh URL cũ sau khi BE trả ID version mới | Người dùng vừa lưu nhưng vẫn đang xem version trước; khó biết đã tạo hay đã duyệt bản nào | [Testcase review](../frontend/components/test-case-review.tsx), [Review backend](../backend/internal/testcase/repository.go) |
| VER-06 | Export cho run vẫn lọc testcase theo bản mới nhất; join trực tiếp nhiều review rows | Xuất lại một run cũ có thể thiếu testcase cũ hoặc lặp dòng; file export đã lưu vẫn là snapshot riêng | [Report repository](../backend/internal/report/repository.go) |
| VER-07 | Coverage hiện tính testcase không bị rejected, kể cả draft | Nhãn coverage dễ bị hiểu thành tất cả testcase đã duyệt/sẵn sàng chạy | [Coverage](../backend/internal/testcase/repository.go) |

**Kết luận thiết kế:** cần sửa cả luồng thao tác và quy tắc chọn phiên bản. Chỉ thêm
nút Next hoặc dropdown version sẽ không giải quyết được các lỗi dữ liệu trên.

## 3. Mục tiêu, ranh giới và tiêu chí trải nghiệm

### 3.1. Mục tiêu nghiệm thu

- Người mới đi từ tài liệu đến testcase và Excel bằng UI, không cần đọc log/SQL
  trong luồng thành công thông thường.
- Bốn bước nghiệp vụ luôn hiện rõ: **Tài liệu → Yêu cầu → Testcase → Sử dụng/Xuất**.
- Mỗi bước có tiến độ, kết quả, một hành động chính và lý do nếu chưa thể tiếp tục.
- Upload/parse/index/extract/generate tự cập nhật; reload hoặc rời trang không làm
  mất job đang chạy. Người dùng không phải bấm Re-index trong luồng thông thường.
- Nguồn đã được người dùng chọn là phạm vi rõ ràng; không tự đưa file vừa upload
  vào job cũ hoặc tự dùng lại nguồn cũ mà không hiển thị.
- Duyệt nhiều requirement/testcase trong một màn hình, có evidence và kết quả
  riêng từng mục. Không chọn tất cả mặc định.
- Có thể mở history, so sánh hai revision, sửa thành draft mới, phục hồi nội dung
  cũ thành draft mới và phân biệt bản mới nhất với bản đã duyệt/bản đang sử dụng.
- PR/MR, automation, test run và báo cáo biết chính xác testcase revision nào đã dùng.

### 3.2. Nguyên tắc không thay đổi

- Tài liệu/requirement đã duyệt là nguồn nghiệp vụ; code chỉ phục vụ automation và chạy test.
- AI tạo draft. Tự động xử lý kỹ thuật không tự cấp approval.
- Conflict/TBD phải có quyết định xử lý; không sửa expected result chỉ để sandbox pass.
- Chỉnh sửa nội dung đã lưu tạo revision mới; lịch sử, bằng chứng, approval và run
  trước đó không bị ghi đè.
- Upload tài liệu v2 hoặc tạo testcase v2 draft không tự thay baseline đã công bố.
- “Có testcase liên kết” không đồng nghĩa “đã duyệt”, “chạy pass” hoặc “phủ mọi trường hợp”.
- Quản lý version không mở chức năng xóa riêng revision đang được tham chiếu;
  retention/purge tiếp tục theo policy hiện có, bổ sung các reference mới.

### 3.3. Ngoài phạm vi đợt này

- Không xây trình soạn thảo DOCX hoặc nhập XLSX testcase.
- Không mở rộng framework automation ngoài phạm vi repo hiện có.
- Không làm hệ thống nhánh/merge testcase như Git hay nhiều người gõ đồng thời.
- Không triển khai lại OIDC/SSO; sử dụng cơ chế identity/role hiện có, thể hiện
  đúng việc ai là service actor và ai là người review được khai báo.
- Không tự chạy sandbox hoặc đổi baseline project khi người dùng chỉ muốn xuất testcase.

## 4. Luồng sử dụng đề xuất

### 4.1. Màn hình chung cho một bộ tài liệu

Giữ URL `/documents/{setId}` làm workspace chính; các URL chi tiết hiện có tiếp tục
hoạt động và có đường quay về đúng bước. Giữ phong cách giao diện hiện tại,
ưu tiên giảm thao tác và làm rõ trạng thái trước việc đổi màu/trang trí.

```text
Đặt hàng thành công                         [Đang xem: Bộ làm việc]
Tài liệu  ✓  ──  Yêu cầu  8/12  ──  Testcase  ──  Sử dụng/Xuất

Cần làm tiếp: Duyệt 4 yêu cầu còn lại
[Xem và duyệt yêu cầu]                [Chi tiết xử lý]

Nội dung bước đang chọn
Nguồn / preview / danh sách / review nằm trong cùng ngữ cảnh
```

Thanh bước không chỉ là link trang: dữ liệu từ workflow API quyết định trạng thái
`chưa bắt đầu / đang xử lý / cần bạn xem / sẵn sàng / lỗi / cần cập nhật`.
Cho xem dữ liệu lịch sử dù bước hiện tại đang bị chặn; chỉ khóa hành động không hợp lệ.

### 4.2. Lần đầu tạo testcase

1. **Tạo bộ và tải tài liệu.** Tạo xong tự mở bộ mới. Form cơ bản chỉ cần tên;
   product/scope/description ở phần bổ sung. Có multi-upload, kết quả theo từng file.
2. **Xử lý tài liệu.** Parse tự chạy; sau khi kết thúc đợt upload, backend build
   index cho snapshot nguồn đã chọn. UI hiển thị tiến độ và tài liệu bị lỗi.
3. **Kiểm tra nguồn.** Có nút `Xem & duyệt` rõ ràng ở từng dòng, đọc preview bên cạnh
   danh sách. Chọn nguồn rồi bấm `Duyệt nguồn & trích xuất yêu cầu` khi có quyền.
   Nút nói rõ sẽ dùng AI; quyết định duyệt được lưu trước khi enqueue extraction.
   Nếu extraction thất bại, retry job không ghi thêm một approval giả.
4. **Rà soát yêu cầu.** Hiện statement, evidence, mức ưu tiên và mục chưa rõ.
   Có `Lưu bản sửa`, `Duyệt đã chọn`, `Từ chối` và `Cần làm rõ` riêng.
   Sau duyệt, bấm `Sinh testcase cho N yêu cầu đã duyệt`; hiển thị số requirement
   chưa được đưa vào phạm vi. Không đợi mọi TBD được giải quyết mới cho làm phần hợp lệ.
5. **Rà soát testcase.** Generation chạy nền. Kết quả draft có steps/data/expected
   và evidence; có preview trước bulk review. Sau duyệt, `Chốt bộ testcase` tạo
   suite baseline bất biến. Có thể cung cấp CTA kết hợp duyệt/chốt, nhưng phải hiển
   thị rõ tập revision sẽ được chốt và kiểm tra lại ở backend.
6. **Sử dụng kết quả.** Chọn `Xuất bản nháp`, `Xuất bộ đã chốt` hoặc `Gắn bộ vào
   project`. Xuất bộ testcase chưa chạy ghi rõ “Chưa chạy”; báo cáo thực thi là
   lựa chọn riêng dựa trên một run cụ thể.

Parse/index là xử lý kỹ thuật được tự nối tiếp trong đợt upload. Không tự gọi LLM
mỗi khi người dùng mở trang hoặc upload thêm một file. Extraction/generation cần
ý định được lưu từ CTA trên; retry chỉ tiếp tục đúng job/snapshot đó.

### 4.3. Khi tài liệu thay đổi

1. Dòng tài liệu có `Tải phiên bản mới`, gắn vào document ID cụ thể; không bắt người
   dùng nhập đúng Display name để nhận diện tài liệu cũ.
2. Bản v2 tạo bộ làm việc cần cập nhật. Baseline đã chốt vẫn được hiển thị là đang
   dùng nguồn v1, kèm thông báo có nguồn mới chưa duyệt.
3. Parse/index snapshot mới tự chạy. Sau duyệt nguồn, extraction xuất bản so sánh
   requirement thêm/sửa/bỏ/không đổi; reviewer xác nhận mapping không rõ.
4. Hiển thị danh sách testcase bị ảnh hưởng và lý do từ requirement/evidence links.
   CTA `Cập nhật các testcase bị ảnh hưởng`; có tùy chọn tạo lại toàn bộ với phạm vi rõ.
5. Sinh draft revision mới, giữ bản đã duyệt. Kịch bản mới tạo testcase identity mới;
   requirement bị bỏ đề xuất ngừng sử dụng case, không tự xóa.
6. Sau review, chốt baseline mới rồi chủ động chọn cho project. Run đã tạo trước đó
   và export cũ vẫn giữ nội dung cũ.

### 4.4. Hành vi khi lỗi/chờ

| Tình huống | Hiển thị và hành động |
| --- | --- |
| File hỏng/không có nội dung | Tên file, lý do dễ hiểu, `Tải bản khác`; không cản file hợp lệ nếu người dùng loại file lỗi khỏi phạm vi |
| Worker chưa nhận job | “Đang chờ xử lý”, thời điểm cập nhật, hướng dẫn vận hành trong chi tiết; không hiện tiến độ giả |
| API/provider timeout | Giữ trạng thái job, kiểm tra lại trước khi cho tạo request mới; `Thử lại bước lỗi` khi đã xác định thất bại |
| Provider output sai schema | Chỉ ra bước/requirement bị lỗi; chi tiết kỹ thuật và request ID trong mục mở rộng |
| Hết budget | Số đã dùng/còn lại và quyền cần để điều chỉnh; không tự retry vô hạn |
| Nguồn/index không cùng snapshot | “Có phiên bản tài liệu mới”; chọn xử lý nguồn mới hoặc xem job lịch sử |
| Không có yêu cầu hợp lệ | Phân biệt 0 requirement vì nội dung không có yêu cầu, vì chưa duyệt, hoặc vì job lỗi |
| Không có quyền | Lý do rõ ràng, nút bị khóa; backend vẫn kiểm tra quyền độc lập |
| Job hoàn thành một phần | Số thành công/thất bại, item cần retry; không báo hoàn tất cả bộ |

## 5. Thiết kế quản lý phiên bản testcase

### 5.1. Các khái niệm phải tách rõ

| Khái niệm | Ý nghĩa | Ví dụ |
| --- | --- | --- |
| Testcase identity/family | Một kịch bản kiểm thử ổn định qua nhiều lần sửa | `TC-ORDER-001`: đặt hàng một sản phẩm |
| Testcase revision | Nội dung cụ thể cùng nguồn evidence ở một thời điểm | `TC-ORDER-001 v2`, row ID 512 |
| Latest revision | Revision mới nhất, có thể draft hoặc rejected | v3 draft |
| Approved revision | Revision từng được duyệt; nhiều revision lịch sử có thể approved | v1, v2 approved |
| Working candidate | Revision đang được chọn để soạn/review bộ tiếp theo | v3 draft hoặc v2 approved |
| Suite baseline/release | Danh sách cố định `identity → revision ID` đã được chốt | Bộ R2 chứa TC-001 v2, TC-002 v1 |
| Project baseline | Suite release cụ thể được chọn cho project | Project A đang dùng Bộ R1 |
| Automation version | Revision code hiện thực một testcase revision | Artifact a4 cho TC-001 v2 |

Không dùng từ “bản hiện tại” một mình. UI phải nói rõ “bản mới nhất”, “bản đã duyệt
gần nhất” hoặc “bản đang dùng trong Bộ R1”. Một testcase có thể đồng thời được dùng
ở nhiều revision trong những project/release khác nhau.

### 5.2. Quy tắc nhận diện kịch bản

- Server cấp identity bền vững; không dùng title, expected result hay tổ hợp
  `type + requirement key` làm identity duy nhất.
- Hai case NEGATIVE của cùng requirement, ví dụ “thiếu địa chỉ” và “giỏ hàng rỗng”,
  phải là hai identities khác nhau nếu tài liệu thực sự mô tả cả hai.
- Lần tạo đầu: proposal khác kịch bản được cấp identity riêng. Duplicate detection
  phải xét mục tiêu, điều kiện, steps, data và assertion; không gộp chỉ vì cùng expected.
- Khi sửa hoặc regenerate một case: request chỉ rõ identity/revision gốc.
  Server xác minh identity nằm đúng bộ tài liệu và phạm vi job.
- Với sinh lại cả bộ: matching chỉ đề xuất; trường hợp mơ hồ vào `Cần đối chiếu`
  để người review chọn “case mới” hoặc “revision của case đã có”.
- Không cho AI tự quyết ghi đè lineage. Một identity không chứa hai scenario khác nhau.
- `content_hash` dùng canonical payload đầy đủ: title/type/risk/actor/preconditions,
  data, ordered steps, expected ở cả case và step, postconditions, assumptions,
  requirement revision IDs và evidence references. Không include timestamp/ID sinh ngẫu nhiên.
- Chỉ chuẩn hóa định dạng an toàn; không lowercase hoặc xóa khoảng trắng bên trong
  dữ liệu kiểm thử có phân biệt hoa/thường, chuỗi literal hay code.
- Approval, run result và automation readiness là metadata; không dùng để xác định
  nội dung revision giống nhau. Gọi lại LLM không tự động bắt buộc tạo revision mới.

### 5.3. Vòng đời và thao tác

```text
TC-001 v1 APPROVED ── được ghim trong Bộ R1 ── Run A/Excel A
        │
        └─ Sửa / cập nhật từ nguồn → v2 DRAFT
                                      │
                                      ├─ REJECTED: R1 vẫn dùng v1
                                      └─ APPROVED → chốt Bộ R2 dùng v2

Phục hồi nội dung v1 khi latest là v2 → v3 DRAFT, restored_from = v1
```

Quy tắc cụ thể:

- Revision đã lưu là nội dung bất biến, kể cả draft. `Lưu bản sửa` tạo revision kế
  tiếp khi payload thay đổi; form chưa lưu là trạng thái cục bộ, có cảnh báo rời trang.
- Duyệt/từ chối không đổi nội dung hoặc tăng số revision; ghi review event với actor,
  thời điểm, nội dung hash và comment. Không cho approve revision đã bị thay nội dung.
- Sửa và duyệt là hai command riêng. UI có thể nối hai command bằng CTA rõ ràng,
  nhưng phải review ID/hash trả về sau bước lưu, không dùng ID cũ.
- Tạo draft mới không hủy approval của revision cũ và không thay manifest release cũ.
- Rejected revision giữ nguyên; sửa tiếp tạo draft mới. Ngừng dùng một identity là
  `ARCHIVED` ở cấp identity, không biến toàn bộ lịch sử thành rejected.
- Restore sao chép nội dung và citation gốc thành draft revision mới; không giảm
  version counter, không tự approve, không tự đổi project baseline. Nguồn cũ không
  còn được chấp nhận phải được làm rõ trước lần publish tiếp theo.
- Optimistic concurrency: gửi revision ID/head token đã mở. Nếu người khác vừa sửa,
  trả `409` cùng current revision ID; cho xem diff rồi áp dụng lại thay đổi.
- Số version cấp nguyên tử theo identity; create đầu tiên và edit/regenerate/restore
  đều có idempotency key. Retry không tạo v2/v3/v4 trùng nội dung ngoài ý muốn.

### 5.4. Nguồn nghiệp vụ, requirement và automation

- Testcase revision tham chiếu requirement **revision**, không tự trỏ sang requirement
  mới nhất khi đọc. Các evidence links đã dùng không được append vào bản approved.
- Nếu statement không đổi nhưng evidence chuyển tài liệu v1 sang v2, lưu revision
  provenance mới hoặc snapshot nguồn mới có định danh riêng; không lặng lẽ sửa proof cũ.
- Chỉnh expected result phải có bằng chứng từ requirement đã duyệt. Trường hợp
  tài liệu thiếu phải sửa/làm rõ requirement trước; không chỉ giữ link cũ cho đủ điều kiện.
- Bản mới không tự kế thừa `AUTOMATED` hoặc PASS của bản cũ. Readiness phải tính
  theo artifact đúng testcase revision và nội dung hash.
- Expected hash vẫn là guard hiện có; bổ sung kiểm tra full case content/steps hash,
  vì hai revision có thể cùng expected nhưng khác data hoặc thao tác.
- Có thể tái sử dụng code automation bằng cách tạo artifact mới có nguồn gốc copy,
  pin vào revision mới và chạy kiểm tra/review tương ứng. Không rebind artifact cũ.
- Run cũ luôn đọc manifest/snapshot của run. Export đã lưu giữ nguyên bytes/hash;
  tạo export mới của run cũ vẫn phải lấy đúng nguồn, steps, expected và actual của run đó.

### 5.5. Giao diện phiên bản

- Danh sách testcase: mã ổn định, scenario, latest revision/status, bản trong baseline
  đang xem, nguồn cần cập nhật, trạng thái automation; bộ lọc theo tất cả các tiêu chí đó.
- Chi tiết có các tab: **Nội dung · Nguồn · Phiên bản · Automation · Lần chạy**.
- Timeline: version, người/AI tạo, lý do, thời gian, review status, nguồn revision,
  release đang dùng; nút mở revision cũ và so sánh hai revision.
- Diff: title/precondition/data/expected/steps/requirement references; xử lý
  thêm-xóa-đổi thứ tự step. Nhấn mạnh thay đổi expected, không dựa riêng vào màu.
- Version cũ là màn đọc; CTA `Tạo bản mới từ bản này`, `Phục hồi thành bản nháp mới`.
- Reviewer thấy side-by-side case/evidence và có `Duyệt & xem mục tiếp theo`.
- Sau lưu, router chuyển sang revision ID mới và hiện “Đã tạo vN”; có link quay
  lại vN-1. Không chỉ refresh trang revision cũ.
- Lịch sử execution của revision đang xem tách với history cả identity; không
  gắn PASS của v1 vào v2 chưa chạy.

## 6. Kiến trúc dữ liệu, job và API đề xuất

### 6.1. Tái sử dụng và mở rộng dữ liệu

Tên bảng dưới đây là đề xuất cho migration mới sau migration 22, cần chốt tên ở
UV-00. Không sửa migration lịch sử đã phát hành, không đổi ý nghĩa `test_cases.id`
đang được các FK sử dụng: **đây vẫn là ID của một revision**.

| Dữ liệu | Thiết kế đề xuất | Ràng buộc chính |
| --- | --- | --- |
| `document_sets` | Thêm `source_revision` tăng khi tập tài liệu làm việc thay đổi | Upload không còn để UI hiểu index cũ là mới nhất |
| `document_source_snapshots`, `document_source_snapshot_items` | Chốt document version IDs/checksums, phạm vi include/exclude và fingerprint | Một version được chọn/document; loại file cần có lý do hiển thị |
| `document_index_generations`, `document_index_generation_items` | Generation trỏ về source snapshot và membership chunk cụ thể | Không dùng toàn bộ chunk lịch sử làm current index |
| `document_workflow_jobs`, `document_workflow_job_items` | Parent operation, loại stage, input snapshot, status, lease, attempt, progress, output refs | At-least-once worker; unique operation/item input key; không xây lại parse queue nếu có thể nối queue cũ |
| `test_case_families` | ID bền vững, public key, suite/set, archived state, revision counter/head | Unique public key theo suite; head thuộc cùng family; lock cấp family |
| `test_cases` | Giữ revision rows hiện có; thêm family ID, parent/restored-from, full content hash, created by, reason, source/job provenance | Unique `(family_id, version_number)`; parent cùng family/set; không chu trình |
| Steps/evidence links | Là thành phần của payload bất biến | Chặn update/delete nội dung đã sealed, kể cả sửa child row để lách bảo vệ parent |
| Review/audit | Giữ bảng review, bổ sung actor provenance/hash được review và sự kiện create/restore/publish/archive | Approval không thay nội dung; lịch sử không chỉ là trạng thái cuối |
| `test_suite_releases`, `test_suite_release_items` | Manifest cố định identity → testcase revision ID, requirement/source snapshots, hash | Chỉ revision approved khi publish; mỗi family tối đa một revision/release |
| `project_document_baselines` | Bổ sung release ID thay việc tự chọn leaf mới nhất mỗi lần webhook | Chọn release mới là thay đổi có audit |
| Analysis/automation/run/export | Bổ sung family ID, revision number/content hash/release ID khi cần | Giữ FK revision và payload/hash lịch sử; không join “latest” để tái tạo run cũ |

Sau backfill, FK/composite constraint phải bảo đảm cùng set/suite/family. Bảo vệ
immutability không được làm hỏng purge toàn bộ graph có kiểm soát; kiểm tra thêm
reference của release/job mới trong purge preview và backup/restore.

### 6.2. Snapshot và điều kiện tiếp tục

- Workflow read model tính từ DB: trạng thái tài liệu, snapshot index, extraction,
  review counts, generation và release. FE không tự suy luận chỉ bằng count > 0.
- Độ mới của index dựa vào source snapshot/fingerprint. Approval eligibility là
  một trạng thái riêng; thay approval không cần embed lại nếu nội dung không đổi.
- Default extraction dùng snapshot đã chọn và các source version đã được duyệt.
  Mục bị loại khỏi phạm vi luôn hiện rõ; không fallback ngầm về bản cũ.
- Job pin snapshot lúc enqueue. Nếu có upload mới giữa chừng, job vẫn có thể hoàn
  tất với snapshot cũ và được gắn “nguồn cũ”; không trộn source mới hoặc tự publish.
- Default action cho bộ làm việc mới từ chối index không phù hợp bằng mã
  `INDEX_STALE`, và enqueue build đúng snapshot trước extraction theo workflow intent.
- Subject chunk **và** retrieval context đều phải thuộc đúng source/index snapshot.
  Chunk subject luôn có trong prompt/evidence; không để top-k retrieval bỏ mất subject.
- Khi nguồn đổi, đánh dấu cần đối chiếu requirement/testcase theo evidence links;
  không tính artifact lịch sử như coverage của nguồn mới khi chưa xác nhận mapping.

### 6.3. Hợp đồng job

- Parse tái sử dụng worker hiện tại; index và generation chuyển sang job nền.
  Extraction queue hiện có được mở rộng hoặc bridge sang read model chung.
- API enqueue trả `202 + job_id + status_url`, mục tiêu nhanh dưới 1 giây trên môi
  trường demo bình thường; thời gian LLM tách riêng, không hứa hoàn thành trong 1 giây.
- Trạng thái job thống nhất: `QUEUED/RUNNING/SUCCEEDED/PARTIAL_FAILED/FAILED/CANCELED`;
  adapter phải map `PENDING/COMPLETED` hiện có, không phá client cũ.
- `completed_units/total_units`, failed units, heartbeat, last update, attempt;
  không dùng phần trăm thời gian phỏng đoán. Chưa biết total thì hiện “đang chuẩn bị”.
- Retry từng unit dùng input hash/prompt version/model policy, giữ output đã kiểm
  tra hợp lệ; persistence nguyên tử giữa kết quả unit và checkpoint.
- Lời gọi provider không thể đảm bảo exactly-once nếu timeout sau khi provider đã
  nhận. Ghi attempt, reservation và usage; phân biệt kết quả chưa xác định, tránh
  tuyên bố retry luôn miễn phí hoặc không thể gọi AI lại.
- Cancel ở safe point, dừng tạo request mới; không công bố partial release.
  Archive/purge phối hợp dừng job và bảo toàn reservation/audit.
- Polling chung có backoff, dừng khi terminal/unmount, tải lại khi tab quay lại;
  catch lỗi mạng cả enqueue và poll, tránh double-click, hiển thị lỗi persist sau reload.

### 6.4. API contract dự kiến

Giữ `/api/test-cases/{id}` là đọc revision ID cũ. Các route mới dưới đây là dự kiến;
phải có DTO/OpenAPI hoặc tài liệu API nhất quán trước khi nối UI.

| Method và route | Mục đích | Response/guard chủ yếu |
| --- | --- | --- |
| `GET /api/document-sets/{id}/workflow` | Read model cho stepper/CTA/capabilities | source snapshot, step states, counts, blocking reasons, job refs, `next_action` |
| `POST /api/document-sets/{id}/workflow-operations` | Lưu intent build/index/extract/generate theo input đã chọn | 202; idempotency key, snapshot ID, quyền từng action |
| `GET /api/document-workflow-jobs/{jobId}` | Job và progress | item counts, last update, sanitized errors, output refs |
| `POST /api/document-workflow-jobs/{jobId}/retry` hoặc `/cancel` | Phục hồi/dừng đúng operation | CAS theo job revision, budget và quyền |
| `POST /api/documents/{documentId}/versions` | Upload bản mới của đúng tài liệu | multipart; giữ document ID, trả version ID và parse job |
| `POST /api/requirements/bulk-review` | Duyệt nhiều revision rõ ràng | per-item success/error; expected revision/hash; audit |
| `GET /api/document-sets/{id}/test-case-families` | Danh sách identity/latest/approved/pinned | pagination/filter/sort ổn định; không bỏ bản approved do có draft |
| `GET /api/test-case-families/{id}/versions` | Lịch sử revision | pagination, creator/reason/source/review/release refs |
| `POST /api/test-case-families/{id}/versions` | Tạo draft từ edit | base revision, head token, content/steps/evidence, reason; 201 hoặc 409 |
| `GET /api/test-case-families/{id}/diff?from=…&to=…` | Diff hai revision | field/step/source changes; xác minh cùng family |
| `POST /api/test-case-families/{id}/restore` | Khôi phục nội dung thành draft mới | from revision, expected head, reason; không rewrite history |
| `POST /api/test-case-families/{id}/archive` | Ngừng đưa identity vào release mới | audit; giữ run/release cũ |
| `POST /api/test-cases/{revisionId}/review` | Duyệt revision cố định | decision, hash được xem; không edit ngầm trong contract mới |
| `POST /api/test-suites/{suiteId}/releases` | Publish manifest testcase | danh sách exact revision IDs, source/requirement snapshot, reason |
| `GET /api/test-suites/{suiteId}/releases` | Lịch sử bộ đã chốt | manifest hash, actor, created_at, counts |
| `GET /api/test-suite-releases/{releaseId}` | Nội dung bộ cụ thể | không dùng query “latest” |
| `POST /api/projects/{id}/document-baseline` | Chọn release để chạy | bổ sung `release_id`, expected previous binding; audit |
| `POST /api/document-sets/{id}/exports` | Xuất working selection, release hoặc run | `mode` rõ ràng, revision IDs/release ID/run ID tương ứng |

API error envelope bổ sung `code`, `message`, `retryable`, `request_id`,
`blocked_by`, `next_action`, `current_revision_id` khi phù hợp; giữ trường `error`
để tương thích. Phân biệt 401/403, 409 stale/concurrency, 422 nguồn không hợp lệ,
429 rate limit với lỗi provider/worker.

POST quan trọng có `Idempotency-Key`; review/edit có expected revision/head token.
Proxy Next.js hiện chỉ chuyển một số header, vì vậy phải bổ sung forwarding
`Idempotency-Key`, precondition headers, `Retry-After` và lỗi có cấu trúc.
RBAC phải có rule rõ cho publish/restore/bulk review, không chỉ trông vào substring
`/review` hoặc tin reviewer name client gửi lên là danh tính xác thực.

## 7. Các phase triển khai

### 7.1. Thứ tự và mức ưu tiên

| Phase | Kết quả bàn giao | Phụ thuộc | Ưu tiên | Ước lượng ngày công |
| --- | --- | --- | --- | --- |
| UV-00 | Chốt contract, fixture và kịch bản nghiệm thu | Không | P0 | 2–3 |
| UV-01 | Snapshot nguồn/index đúng version | UV-00 | P0 | 4–6 |
| UV-02 | Identity và revision backend có migration | UV-00 | P0 | 5–7 |
| UV-03 | Release/pinning và downstream đọc đúng revision | UV-01, UV-02 | P0 | 4–6 |
| UV-04 | Job nền và workflow read model chung | UV-01; generation dùng UV-02/03 | P1 | 5–7 |
| UV-05 | Workspace bốn bước, upload và review nguồn dễ dùng | UV-04; prototype có thể làm từ UV-00 | P1 | 4–6 |
| UV-06 | Requirement review theo nhóm và xử lý nguồn thay đổi | UV-01, UV-04, UV-05 | P1 | 4–6 |
| UV-07 | UI testcase history/diff/edit/restore/release | UV-02, UV-03, UV-05 | P1 | 4–6 |
| UV-08 | Regenerate có đối chiếu và cập nhật có phạm vi | UV-03, UV-04, UV-06, UV-07 | P1 | 4–6 |
| UV-09 | Kiểm thử hành trình, migration, rollout và tài liệu | UV-01 → UV-08 | P0 trước phát hành | 4–6 |

Ước lượng tổng: **40–59 ngày công**, khoảng 8–12 tuần nếu một người làm toàn thời
gian, chưa tính chờ credential/hạ tầng hoặc vòng thay đổi yêu cầu. Đây là ước lượng
cho cả workflow bền vững và versioning đầy đủ, cần cập nhật sau UV-00.
Kiểm thử từng phase nằm ngay trong phase; UV-09 dành cho kiểm thử tích hợp toàn luồng.

Không bật writer versioning mới trước khi các consumer baseline/run/export đã hiểu
revision ở UV-03. Có thể dựng UX sớm, nhưng không gắn nhãn hoàn thành vào mock hoặc
che lỗi backend bằng trạng thái thành công trên giao diện.

### UV-00 — Chốt hành vi, contract và bằng chứng tái hiện

**Mục tiêu:** thống nhất các nghĩa “mới nhất/đã duyệt/đang sử dụng”, giảm việc sửa
schema/API lần hai khi frontend bắt đầu dùng.

Backend và dữ liệu:

- [x] Lập danh sách mọi query chọn `latest`, `supersedes`, `APPROVED` trong
  document/requirement/testcase/scope/automation/execution/report.
- [x] Chốt payload canonical, source snapshot, identity/revision/release, job states
  và per-item bulk response theo mục 5–6.
- [x] Chốt quyền editor/reviewer/admin cho thao tác mới; actor được lấy từ đâu,
  reviewer khai báo được ghi riêng ra sao khi chưa có end-user session.
- [x] Viết ADR bổ sung cho versioning và baseline, xác định policy khi nguồn mới
  còn draft, bị reject, bị bỏ hoặc chưa tìm được mapping requirement.
- [x] Tạo fixture nguồn v1/v2 nhỏ: hai NEGATIVE khác nhau cùng requirement; thay đổi
  chỉ steps/data; testcase approved có draft successor; run cũ trước khi tạo revision mới.

Frontend và sản phẩm:

- [x] Vẽ prototype bốn bước cho bộ rỗng, đang parse, lỗi, chờ review, có bản mới.
- [x] Chốt từ ngữ nghiệp vụ, CTA, vị trí history/diff và cách quay lại đúng bộ/bước.
- [x] Mặc định nhãn tiếng Việt cho workspace mới; tập trung UI copy trong một nơi
  để bổ sung ngôn ngữ sau, không thay nội dung nghiệp vụ do người dùng cung cấp.
- [ ] Chạy thử luồng cũ với ít nhất 3 người/đại diện nếu có thể; ghi số lần phải
  hỏi, thao tác thủ công, thời gian chờ máy và thời gian thao tác riêng.
  Biên bản và biểu mẫu đã chuẩn bị tại
  [USABILITY_BASELINE_PROTOCOL.md](uv00/USABILITY_BASELINE_PROTOCOL.md); chưa ghi
  kết quả vì chưa có người tham gia thực tế. Đây là khảo sát tùy điều kiện, không
  phải cổng chặn Definition of Done của UV-00.

Kiểm tra và Definition of Done:

- [x] Có test fixture tái hiện DATA-01/02 và VER-02/03/04/06 trước khi sửa.
- [x] Contract có ví dụ request/response, error và policy versioning được review.
- [x] Prototype mô tả được 3 hành trình: lần đầu, sửa testcase, đổi tài liệu.
- [x] Không dùng dữ liệu thật/credential trong fixture; tên hoặc provenance thiếu
  từ dữ liệu cũ được để unknown, không bịa người duyệt/ngày duyệt.

File dự kiến: `docs/adr/` (ADR mới), package test fixtures, tài liệu API và frontend
prototype theo route hiện có. Không cần thay provider LLM ở phase này.

### UV-01 — Sửa độ mới của index và phạm vi extraction

**Mục tiêu:** thao tác trên bộ làm việc mới không đọc nhầm hoặc trộn tài liệu cũ.

Backend/database:

- [x] Migration source snapshots, generation membership và freshness metadata.
- [x] Upload bản mới cập nhật `source_revision` nguyên tử; job/index cũ giữ reference
  lịch sử, workflow báo bộ làm việc cần cập nhật.
- [x] Chốt danh sách version cho một đợt xử lý. Parse lỗi/chưa xong không bị lặng lẽ
  bỏ qua rồi báo cả bộ sẵn sàng; cho người dùng xác nhận loại khỏi phạm vi.
- [x] Index build đọc source snapshot; commit generation chỉ khi có đủ membership
  và hash hợp lệ. Chống hai lần build cùng snapshot chạy đè nhau.
- [x] `AllChunks` cho extraction đọc membership chính xác, không query toàn lịch sử.
- [x] Retrieval và subject cùng source snapshot; luôn đưa nội dung subject/evidence
  vào prompt, top-k chỉ bổ sung ngữ cảnh được phép.
- [x] Request mới kiểm tra freshness; job cũ hoàn tất theo input đã pin, output cũ
  được đánh dấu không thuộc bộ làm việc mới.
- [x] Approval eligibility/warnings phản ánh trạng thái hiện tại mà không bắt
  embed lại tài liệu không đổi. Metadata index failure có thể đọc lại sau reload.

Frontend:

- [x] Hiện “Dữ liệu tìm kiếm cần cập nhật” cùng nguồn/version cụ thể.
- [x] Trang chẩn đoán cho xem generation lịch sử riêng, không trình bày đó là current.
- [x] Sửa hiển thị `v{document_version_id}` thành version number thực của tài liệu;
  ID database nằm trong chi tiết kỹ thuật.

Kiểm tra và Definition of Done:

- [x] Upload v2 khi v1 `READY`: default extraction không dùng v1.
- [x] Re-index v2: chỉ subject/context của snapshot v2, v1 vẫn mở được lịch sử.
- [x] Upload v3 lúc job v2 chạy: không trộn source, không tự publish output v2.
- [x] Approve nguồn sau index: warning draft không còn sai; không tốn embedding lại.
- [x] Trường hợp nhiều file, parse lỗi, empty blocks, retry build, hai build đồng
  thời và khác document set được kiểm tra.

File trọng tâm: `backend/internal/document/{repository,index_repository,index_service}.go`,
`backend/internal/requirement/{service,extractor,repository}.go`, migrations và index UI.

### UV-02 — Backend identity và revision testcase

**Mục tiêu:** kịch bản khác nhau tồn tại độc lập; lịch sử revision có API rõ ràng.

Backend/database:

- [x] Migration thêm family, canonical public key, revision provenance, content
  hash, head token và audit. Giữ mọi row ID/FK cũ.
- [x] Thay `logicalCaseKey` bằng identity cấp từ server; giữ legacy key để tra cứu.
- [x] Thay fingerprint dùng để so sánh nội dung bằng canonical payload đầy đủ.
- [x] Bổ sung list families, list versions, read revision, structured diff, create
  revision, restore và archive API.
- [x] Tách edit khỏi approval; field optional phân biệt “không gửi” và “muốn xóa
  giá trị rỗng”. Cho sửa steps/step expected và dữ liệu có cấu trúc phù hợp.
- [x] Create/edit/restore transaction gồm revision, steps, sources, hash, audit;
  tăng counter theo family lock và idempotency key.
- [x] Check base/head revision, trả `409` thay vì ghi đè hoặc lỗi unique constraint.
- [x] Bảo vệ nội dung parent/child/evidence của revision đã sealed; review metadata
  được thay có audit. Retention/purge sử dụng quy trình xóa graph được phép.
- [x] Requirement/evidence thay đổi tạo provenance mới; không merge link vào
  revision testcase approved khi dedupe hoặc generate lại.
- [x] Review kiểm tra bằng chứng cho expected ở cả case và từng step; all required
  source links phải hợp lệ theo scope, không chỉ có một link approved bất kỳ.

Frontend tích hợp tối thiểu:

- [x] Khai báo types/API client identity/revision, giữ đọc được deep link row ID cũ.
- [x] Sau create revision chuyển về URL ID mới; hiển thị thông báo vN vừa được tạo.
- [x] Legacy edit+review route có adapter trong giai đoạn chuyển đổi, dùng service
  mới và trả ID mới; đánh dấu deprecation, không duy trì hai cách viết revision.

Kiểm tra và Definition of Done:

- [x] Hai NEGATIVE cùng requirement có hai identities cùng xuất hiện.
- [x] Chỉ sửa step/data vẫn tạo revision; payload hoàn toàn không đổi không sinh
  revision thừa. Đổi citation version được nhận biết.
- [x] Hai người sửa cùng head: một thành công, người kia nhận 409 có revision hiện tại.
- [x] Restore v1 khi latest v3 tạo v4 draft, parent=v3, restored_from=v1.
- [x] Retry cùng idempotency key trả cùng kết quả; khác payload cùng key bị từ chối.
- [x] Chặn foreign-set/family references và sửa trực tiếp steps/evidence đã sealed.
- [x] Migration/backfill giữ nguyên run/artifact/export reference cũ; có báo cáo
  family legacy nghi ngờ bị trộn scenario, chưa tự động gộp/tách bằng similarity.

File trọng tâm: `backend/internal/testcase/`, testcase HTTP handlers/router,
`frontend/lib/{types,api}.ts`, migrations; cập nhật trigger cũ bằng migration mới.

### UV-03 — Baseline đã chốt và consumer đúng revision

**Mục tiêu:** tạo draft mới không làm mất testcase đang chạy; mọi báo cáo lịch sử
đọc đúng dữ liệu của lần chạy đó.

Backend/database:

- [x] Publish suite release gồm manifest revision IDs, nguồn/requirement snapshot,
  reviewer/audit và hash; publish idempotent, transaction nhất quán.
- [x] Hiển thị rõ phần requirement chưa bao phủ/ngoài phạm vi khi chốt bộ một phần;
  lưu phạm vi và quyết định review, không tự gán “complete”.
- [x] Project binding chọn release ID; webhook snapshot pin release và toàn bộ
  revision liên quan một lần, kể cả request review/update xảy ra đồng thời.
- [x] Tách latest revision, latest approved revision và revision trong release.
  Bản approved cũ không bị loại vì có successor draft/rejected.
- [x] Automation subject, source hash/expected hash và execute eligibility kiểm
  tra đúng pinned revision. Trạng thái artifact cũ không lan sang revision mới.
- [x] Coverage tách “có thiết kế”, “đã duyệt trong bộ”, “có automation”, “đã chạy”;
  mỗi chỉ số dùng denominator/source snapshot công khai.
- [x] Export `run` bắt đầu từ run items/snapshot, không từ latest testcase; source
  metadata lấy từ evidence/pinned release thay vì latest document toàn set.
- [x] Export review chọn đúng event liên quan hoặc aggregate reviewer, không nhân
  dòng theo số lần review; bản đã lưu không được tạo lại ngầm khi download.
- [x] XLSX thêm revision/release/source version ở cột hoặc sheet Metadata phù hợp;
  bảo toàn bố cục mẫu báo cáo đang dùng, không tự nhận là giống mẫu nếu chưa đối chiếu.

Frontend:

- [x] Bộ lọc `Bộ làm việc / Bộ đã chốt Rn`; mỗi case hiện version được chọn.
- [x] Project baseline selector hiển thị release và thời điểm chốt.
- [x] Export chọn rõ draft selection, approved release hoặc một run; mặc định hợp
  với ngữ cảnh đang xem, hiển thị số case trước tải xuống.
- [x] Actual/PASS/FAIL chỉ hiện khi có run của revision đó; phân biệt “Chưa chạy”.
- [x] Sửa các nhãn export/coverage hiện có cho đúng: lọc danh sách không tự đổi
  tập export; tập export phải được chọn và hiển thị rõ, không đồng thời ghi
  “xuất toàn bộ” trong khi API chỉ nhận subset đã lọc.

Kiểm tra và Definition of Done:

- [x] v1 approved trong R1, v2 draft: PR mới dùng R1/v1 cho tới khi đổi binding.
- [x] R2/v2 được chọn không thay PR/run đã snapshot R1/v1.
- [x] Export lại run v1 sau v2 vẫn có v1, đúng steps/expected/actual/evidence.
- [x] Một testcase có nhiều review event vẫn một dòng testcase trong export.
- [x] Export working/release không tự gắn actual của revision khác.
- [x] Baseline publish/update đồng thời với webhook có kết quả trọn vẹn R1 hoặc
  R2, không ghép nửa bộ; không chọn artifact khác set/revision/SHA policy.

File trọng tâm: `backend/internal/{scope,automation,execution,report,testcase}/`,
project baseline selector, testcase workspace, export controls, run workspace.

### UV-04 — Job nền, retry có tiến độ và workflow API

**Mục tiêu:** tác vụ dài sống độc lập với request/trang web, UI biết chính xác còn thiếu gì.

Backend/database:

- [x] Triển khai workflow read model theo mục 6, gồm capabilities theo role và
  machine-readable blocking reasons. Mọi mutation kiểm tra lại điều kiện.
- [x] Thêm job index và testcase generation; nối parse/extraction queue hiện có
  bằng parent operation/input snapshot, không phát sinh hai scheduler cạnh tranh.
- [x] Queue có lease, heartbeat, max attempts, timeout và recovery worker restart.
- [x] Unit checkpoint và output refs nguyên tử; retry chỉ phần failed/unfinished,
  kết quả đã commit không bị ghi thêm revision hoặc làm mất review của người dùng.
- [x] Lưu ý định `approve-and-extract`, `generate-approved` tách khỏi GET/read UI;
  queue continuation sau review thực hiện qua transaction/outbox hoặc cơ chế durable tương đương.
- [x] Budget reservation/finalize/release gắn unit attempt, job/report có tổng usage;
  UI nêu lý do dừng và quyền điều chỉnh budget.
- [x] Cancel/retry xử lý đồng thời, tránh hai attempt cùng công bố kết quả.
- [x] API proxy chuyển header idempotency/preconditions và structured error cần thiết.

Frontend:

- [x] Một hook polling dùng chung: backoff, retry tải trạng thái, terminal stop,
  visibility refresh, cleanup, chống response job cũ ghi đè job mới.
- [x] Nút pending ngay lúc gửi; catch network exception; lỗi persist được hiển thị
  ngay cả khi người dùng vào trang lần đầu sau job failed.
- [x] Progress hiển thị unit counts, partial results và nút retry/cancel đúng quyền.

Kiểm tra và Definition of Done:

- [x] Reload/đóng tab/mở lại không tạo job mới; UI tiếp tục đúng job ID.
- [x] LLM chậm hơn timeout HTTP thông thường không giữ request generation mở.
- [x] Worker crash sau gọi provider/trước commit và sau commit/trước acknowledge
  được kiểm tra; không tạo testcase trùng và usage không mất dấu.
- [x] Retry thất bại schema/budget có giới hạn; không tự duyệt partial output.
- [x] Source generation mới xuất hiện không làm job cũ thay input giữa chừng.

File trọng tâm: worker wiring `backend/cmd/worker/main.go`, requirement worker,
package workflow/job mới, testcase/index handlers, frontend polling hook/proxy.

### UV-05 — Workspace hướng dẫn từ upload đến testcase

**Mục tiêu:** người dùng nhìn trang là biết đang ở đâu và bước tiếp theo là gì.

Frontend:

- [x] Tạo document set xong tự mở workspace mới; field phụ thu gọn.
- [x] Thay ba card kỹ thuật bằng stepper bốn bước có counts/status và CTA tiếp tục.
- [x] Multi-file upload có progress/result riêng; failed file retry riêng; chọn
  đúng type từng file; giữ giới hạn file hiện có.
- [x] Document table có `Xem & duyệt`, `Tải phiên bản mới`, `Lịch sử nguồn`;
  phân biệt upload tài liệu mới với thêm version.
- [x] Preview có heading/table/source locator, mở cạnh danh sách; luôn hiện version
  đang đọc và approval trạng thái. Có nút quay lại/duyệt mục tiếp theo.
- [x] Approval cho batch chỉ dựa trên các version đã chọn và hash còn đúng;
  người dùng có thể xem evidence trước, không auto-select mọi tài liệu.
- [x] Parse/index tự cập nhật; chỉ cho vào bước sau khi đủ điều kiện hoặc mở chế độ
  xem lịch sử. Technical inspector đặt trong `Chi tiết xử lý`.
- [x] Source approval/extract kết hợp như mục 4; nguồn đã approved hiển thị action
  trích xuất phù hợp, không yêu cầu review lặp lại không cần thiết.
- [x] Archive/retention/budget chuyển vào phần quản lý, không chen giữa bước upload/review.
- [x] Lỗi có thông điệp nghiệp vụ, nút khắc phục và link chi tiết; raw provider JSON
  không chiếm vùng hướng dẫn chính.
- [x] Đồng nhất empty/loading/error, focus sau chuyển bước, keyboard navigation,
  label/status không dựa vào màu, bố cục mobile/tablet có bảng scroll được.

Backend hỗ trợ:

- [x] Document-version upload API nhận đúng document ID và kiểm tra ownership;
  list version phục vụ history, không chỉ newest version.
- [x] UI đọc capabilities thực từ server. Reviewer display name có thể ghi nhớ
  trong session cục bộ, nhưng không dùng nó để nâng quyền hoặc mạo danh user đăng nhập.

Kiểm tra và Definition of Done:

- [ ] Một người mới tạo bộ, upload và tìm thấy nơi duyệt mà không cần hướng dẫn miệng.
- [x] Không cần refresh thủ công, gõ URL version hoặc mở Semantic Index để tiếp tục.
- [x] Một file lỗi không biến các file khác thành success giả; scope bị loại hiện rõ.
- [x] Direct link trang cũ vẫn hoạt động, breadcrumb/stepper dẫn về đúng set/bước.
- [x] Màn 390 px và desktop dùng được bằng chuột/lẫn bàn phím (browser smoke;
  chưa thay thế audit accessibility đầy đủ).

Bằng chứng kỹ thuật và giới hạn: [UV05_WORKSPACE_VERIFICATION.md](UV05_WORKSPACE_VERIFICATION.md).
Checkbox người mới tự sử dụng vẫn để mở cho buổi nghiệm thu thực tế.

File trọng tâm: `frontend/app/documents/`, create/upload/document-review components,
workflow component/hook mới, source version routes, shared style/status copy.

### UV-06 — Requirement review theo nhóm và đối chiếu nguồn mới

**Mục tiêu:** duyệt yêu cầu nhanh nhưng vẫn biết yêu cầu lấy từ đâu và thiếu gì.

Backend:

- [x] Bulk-review theo exact revision IDs/hash; giới hạn batch, trả kết quả từng
  item rõ ràng. Partial success phải audit được, retry bỏ qua decision đã áp dụng.
- [x] Điều kiện approval có ít nhất đúng evidence hợp lệ theo policy; conflict/TBD
  cần resolution/answer cụ thể, không được lách bằng sửa mỗi title hoặc risk.
- [x] Xử lý conflict/open question cập nhật records liên quan và derived counts;
  không để badge “cần làm rõ” còn nguyên sau khi đã giải quyết.
- [x] Requirement revision/source links bất biến khi review/publish. Extraction
  lại cùng nội dung với nguồn mới không append citation vào proof đã duyệt.
- [x] Đối chiếu requirement cũ/mới theo stable identifier + source + review mapping;
  lưu added/changed/removed/unchanged/ambiguous và reason.
- [x] Requirement bị loại/removed không làm mất history; đánh dấu dependent case
  cần xem lại trước khi công bố cho nguồn mới.

Frontend:

- [x] Danh sách có checkbox, selection count, evidence drawer, review inline và
  `Duyệt & xem tiếp`; giữ filter/scroll khi quay lại.
- [x] Selected set phân biệt “trang hiện tại”/“toàn bộ kết quả lọc”; chưa hỗ trợ chọn
  tất cả xuyên trang thì ghi rõ, không chọn âm thầm item người dùng chưa thấy.
- [x] Bulk confirmation tóm tắt N mục và mục bị chặn; không lặp tên reviewer từng dòng.
- [x] Tab `Cần làm rõ` có source cụ thể, form giải quyết và history quyết định.
- [x] Sau review có CTA `Sinh testcase cho N yêu cầu`, nêu số unresolved/ngoài scope.

Kiểm tra và Definition of Done:

- [x] 20 draft requirement hợp lệ có thể xem evidence và duyệt theo nhóm trong một màn.
- [x] Batch có TBD/foreign-set/stale revision không approve sai; kết quả từng item rõ.
- [x] UI counts và trạng thái conflict/question khớp DB sau giải quyết/reload.
- [x] Sửa requirement tạo revision mới và chuyển trang đúng ID; testcase ảnh hưởng
  được đánh dấu nhưng case/run cũ vẫn đọc được.

Bằng chứng và giới hạn: [UV06_REQUIREMENT_REVIEW_VERIFICATION.md](UV06_REQUIREMENT_REVIEW_VERIFICATION.md).
Review mapping dùng revision cuối của chuỗi edit; AMBIGUOUS chỉ hỗ trợ đọc đối chiếu,
không tự ghép hay chuyển approval. Migration 28 cần triển khai cùng API/worker/frontend.

File trọng tâm: `backend/internal/requirement/`, workflow handlers, requirement
inventory/review/detail UI; dùng lại state/polling của UV-04/05.

### UV-07 — Giao diện quản lý version testcase hoàn chỉnh

**Mục tiêu:** QA tự xem, so sánh, sửa, duyệt và phục hồi testcase qua UI.

Frontend:

- [x] Danh sách theo identity có latest/approved/pinned version, stale source,
  readiness; filter/search/sort/pagination giữ context khi mở chi tiết.
- [x] Timeline version có creator/reason/source/model khi biết, approval và release refs.
- [x] So sánh hai revision cùng identity: field diff, ordered-step diff, citation
  diff; không dùng hash làm nội dung chính cho người dùng.
- [x] Form đầy đủ title/actor/precondition/steps/data/expected/postcondition/risk;
  CTA riêng `Lưu bản nháp mới`, `Duyệt phiên bản`, `Từ chối`.
- [x] Sau lưu mở revision mới; unsaved-change prompt khi cần; 409 mở so sánh current
  head và form đang sửa, không tự bỏ dữ liệu nhập.
- [x] `Phục hồi thành bản nháp mới` có nguồn bản phục hồi, lý do và diff preview.
- [x] Bulk approve hiển thị đúng revision IDs; archive không làm biến mất history.
- [x] Chọn revision approved để chốt release; hiển thị coverage/scope rồi publish.
- [x] Tab run/automation theo revision đang xem; xem cả identity là lựa chọn riêng.
- [x] Export version/release có thông tin version rõ; share URL revision cũ vẫn đúng.

Backend hỗ trợ:

- [x] History/diff pagination giới hạn payload lớn; stable ordering và cross-family guards.
- [x] Release preview và publish kiểm tra lại cùng manifest/hash để tránh race.
- [x] Structured field validation hướng người dùng về requirement khi expected thiếu nguồn.

Kiểm tra và Definition of Done:

- [x] QA làm được v1 approved → v2 draft → diff → approve → R2 mà không dùng DB/API tay.
- [x] Restore v1 tạo v3 draft và không đổi R2 đang dùng.
- [x] Copy expected cũ không tự mang PASS/automation approved sang revision mới.
- [ ] Người dùng phân biệt được version testcase với version automation và release bộ.
- [ ] Keyboard/screen reader đọc được diff, timeline, selection và nút review.

Bằng chứng: [UV07_TESTCASE_VERSIONING_VERIFICATION.md](UV07_TESTCASE_VERSIONING_VERIFICATION.md).
Các thao tác version/review/release/restore và conflict hai tab đã qua Chromium;
nhãn phân biệt ba loại version và kiểm tra keyboard cơ bản đã có, nhưng chưa
nghiệm thu mức độ hiểu của người mới hoặc chạy screen reader thật. Không có
migration mới: tiếp tục schema 28. Phân trang history/diff giới hạn số mục trả về,
không tuyên bố mọi payload lịch sử hoặc inventory identity đã được phân trang server.

File trọng tâm: testcase workspace/review/detail, components history/diff/editor mới,
export controls, API types; không tạo revision bằng logic riêng trên frontend.

### UV-08 — Regenerate và cập nhật testcase theo tài liệu

**Mục tiêu:** tài liệu thay đổi tạo đề xuất có kiểm soát, không tích lũy case trùng
hoặc âm thầm thay bản đang dùng.

Tiến độ chặng nền (22/09/2026): worker đã dùng exact IDs trong job thay vì list lại
baseline live; API nhận optional `requirement_ids` (1–100), kiểm tra ownership/
approval/current và idempotency theo scope. Dedupe trong generation không còn gộp
theo title/expected gần giống hoặc tự merge nguồn. Revision mới lưu generation
provenance. Chặng tiếp nối 23/09 thêm migration 29 và `review_proposals: true`:
persist/classify proposal trước khi đổi testcase, reviewer apply có CAS + receipt.
Chặng hoàn tất 24/09 thêm migration 30, affected mặc định khi cập nhật, summary
nguồn và checkpoint/retry theo requirement; retire-only chạy khi không còn yêu cầu
approved hiện hành. Client API legacy vẫn có thể sinh draft trực tiếp; các mục
backend bên dưới áp dụng cho proposal mode. Kiểm thử kỹ thuật UV-08 đã hoàn tất;
không tự đóng các gate rollout/demo thật và nghiệm thu người dùng của UV-09.
Xem [UV08_COMPLETION_VERIFICATION.md](UV08_COMPLETION_VERIFICATION.md).

Backend:

- [x] Generation job pin requirement/source snapshot; lưu input/version/prompt/output
  đủ để biết vì sao một revision được đề xuất.
- [x] Chọn scope `yêu cầu đã chọn`, `case bị ảnh hưởng`, `toàn bộ bộ làm việc`;
  default chỉ phần bị ảnh hưởng khi cập nhật tài liệu.
- [x] Kết quả phân loại `NEW_CASE`, `NEW_REVISION`, `UNCHANGED`, `RETIRE_CANDIDATE`,
  `AMBIGUOUS_MATCH`; lưu reason và exact target identity nếu đã xác nhận.
- [x] Unchanged phải so đủ content + provenance; không lấy title/type/expected
  đơn thuần. Không gộp negative/boundary case có data/steps khác nhau.
- [x] Gợi ý semantic matching không tự quyết lineage; reviewer giải quyết ambiguity
  qua API. Quan hệ nguồn chưa đủ xác nhận lineage luôn trả `AMBIGUOUS_MATCH`.
- [x] Apply proposal kiểm tra expected head, idempotency và approval đã có; user
  sửa trong lúc AI chạy thì trả conflict, không ghi đè bản human edit.
- [x] Nguồn bị removed đề xuất archive/exclude từ release mới, không delete case.
  `RETIRE_CANDIDATE`/`ARCHIVE` trong ALL/AFFECTED khi mọi nguồn liên kết removed;
  hỗ trợ baseline không còn requirement approved và không cần ngân sách AI.
- [x] Regeneration không tự approve, đổi release binding hoặc chạy sandbox.

Frontend:

- [x] Trước khi chạy: summary nguồn thay đổi, requirement/case ảnh hưởng, phạm vi
  đã chọn và usage/budget hiện có; không đưa ước lượng token giả chính xác.
  Hiển thị source revision/comparison, nguồn bị chặn, ảnh hưởng theo quan hệ lưu
  được và budget theo quyền; quan hệ mơ hồ vẫn yêu cầu reviewer xác nhận.
- [x] Sau chạy: bảng đối chiếu mới/sửa/không đổi/bỏ/không rõ, mở diff và source.
- [x] Chọn áp dụng proposal; nút publish Rn+1/gắn project chỉ sau review đúng điều kiện.
  Apply chỉ tạo draft; link dẫn sang exact revision review, không publish/bind tự động.
- [x] Job partial lỗi chỉ retry unit cần thiết, giữ kết quả human đã xử lý.

Kiểm tra và Definition of Done:

- [x] Cùng nguồn/payload không đổi không tăng revision vô ích; retry cùng output
  không nhân testcase dù provider request có thể đã tốn phí ở attempt trước.
- [x] Một thay đổi requirement chỉ đề xuất cập nhật các case thực sự có liên kết;
  quan hệ không chắc phải được báo, không giả vờ xác định ảnh hưởng đầy đủ.
- [x] Case mới cùng test type được giữ độc lập; case removed không biến mất khỏi run cũ.
- [x] Reviewer đang edit và job generate đồng thời không làm mất chỉnh sửa.
- [x] R1/run/export trước thay đổi vẫn truy xuất nguyên trạng sau publish R2.
  Browser kiểm tra nguồn upload thật/R1/export/R2/archive; PostgreSQL integration
  kiểm tra run PASSED fixture cũ và XLSX giữ nguyên, revision mới không kế thừa PASS.
  Không dùng fixture này làm bằng chứng sandbox/provider thật.

File trọng tâm: testcase generation service/schema/repository, requirement source
mapping, workflow jobs, compare proposals UI và release publishing.

### UV-09 — Kiểm thử toàn hành trình và phát hành

**Mục tiêu:** chứng minh cả tính đúng của dữ liệu lẫn khả năng tự sử dụng trước khi
bật mặc định trên môi trường demo.

Tiến độ 24/09: đã có runner Playwright/CI config, 5/5 browser local với API/worker
thật, role/token checks, verifier exact release/revision và synthetic populated
migration/restore 23→30→31. Migration 31 hiệu chỉnh timestamp legacy kèm audit,
không đổi manifest/run/export cũ. Đã đạt 3/3 U04 browser provider-error fixture.
Đã bổ sung SIGKILL/restart ba ranh giới commit, generation error/reload browser
và unknown-usage budget hold; có local evidence, không thay billing receipt thật.
**Chưa đóng UV-09**; còn CI remote, snapshot triển khai thực, demo/provider thật,
đối soát billing thực tế và human acceptance.
Xem [ma trận và bằng chứng UV-09](UV09_VERIFICATION.md).

Kiểm thử và vận hành:

- [ ] Thực hiện ma trận mục 8, bổ sung Playwright/dev dependencies và CI job cho
  browser journey. Đã có dependency/lockfile, `npm run test:e2e`, HTML/JUnit và
  GitLab job; ma trận còn các dòng chưa đủ evidence, pipeline remote chưa chạy.
- [ ] Browser tests dùng LLM fixture cho kết quả ổn định; integration dùng PostgreSQL
  và API/worker thật. Có lane smoke provider thật riêng, không dùng mock để kết
  luận Gemini production đã hoạt động.
  Đã có lane deterministic local, 3/3 browser protocol fixture cho 400/timeout/enum
  và protected/manual Gemini smoke; real smoke chưa chạy.
- [ ] Kiểm thử migration từ snapshot dữ liệu cũ, dry-run backfill, kiểm tra FK/hash,
  backup/restore có các bảng và artifact mới.
  Đã PASS synthetic schema-23 populated → 30 → 31 và empty-target restore DB/files;
  timestamp correction có audit, populated down bị chặn, empty down/up đạt;
  còn snapshot của môi trường triển khai và coordinated operational drill.
- [x] Mở workspace với viewer/editor/reviewer; API và UI cùng policy, service token
  không lộ ra browser/download/error.
- [ ] Usability test ít nhất 3 người mới với ba nhiệm vụ mục 9; ghi lỗi/điểm vướng
  và sửa blocker trước release. Nếu chưa có người thử, để checkbox này mở.
- [ ] Chạy demo thực từ upload → review → version/diff → release → webhook →
  sandbox → Excel, rồi sửa nguồn và kiểm tra run/report cũ vẫn đúng.
- [x] Mở rộng verifier persisted evidence hiện có để kiểm tra release/revision,
  không chỉ đếm record có tồn tại.
- [x] Update README, API/database/deployment/security/development, demo guide,
  review guide và kế hoạch gốc; ghi rõ hiện trạng so với mục tiêu.
- [ ] Rollout/rollback theo mục 10 và lưu bằng chứng thực tế.

Definition of Done:

- [ ] Không còn P0 trong DATA/VER list và test hồi quy tương ứng đã pass.
- [ ] Luồng thành công không cần thao tác DB/log/Re-index/refresh thủ công.
- [ ] Có video/screenshot và artifact Excel/revision manifest từ demo thật; kết quả
  fail của provider/sandbox nếu có được báo đúng, không đổi expected để làm đẹp demo.
- [ ] Người mới hoàn thành các nhiệm vụ trong mục 9 với số lần trợ giúp đạt mục tiêu.
- [ ] Không phá truy cập dữ liệu lịch sử/legacy endpoints và không mất audit.

## 8. Ma trận kiểm thử bắt buộc

Các ID dùng trong PR/checklist nghiệm thu. Không cần viết test snapshot cho từng
label/màu; ưu tiên điều kiện chuyển bước, ownership, lịch sử và thao tác người dùng.

| ID | Kịch bản | Kết quả bắt buộc | Tầng kiểm tra |
| --- | --- | --- | --- |
| U01 | Bộ mới chưa có file | Hướng dẫn upload, không cho extract/generate vô điều kiện | API + browser |
| U02 | Upload nhiều file, một DOCX hỏng | Status từng file; chỉ tiếp tục khi scope rõ, không mất file thành công | Worker integration + browser |
| U03 | Đóng tab/reload khi parse/extract/generate | Mở lại đúng job/progress, không tạo job trùng | Integration + browser |
| U04 | Provider timeout/400/invalid enum | Lỗi nghiệp vụ + chi tiết, progress persist, retry có giới hạn | Provider contract fixture + browser |
| U05 | Worker restart trước/sau commit unit | Không mất checkpoint, không nhân output, budget được đối soát | Integration |
| U06 | Double click/retry POST cùng key | Cùng operation, không thêm review/version/job | API integration |
| U07 | Viewer bấm/call trực tiếp review/publish | UI giải thích, BE trả 403; không có state change | API + browser |
| U08 | Batch requirement có draft/TBD/stale item | Thành công/lỗi riêng từng item, chỉ đúng revision được duyệt | Integration + browser |
| U09 | Nguồn thiếu ngưỡng hoặc alternate flow | Báo thiếu cần làm rõ, không bịa case/expected để báo full coverage | Unit + golden fixture |
| S01 | Upload v2 sau index v1 | Freshness đổi, generation mới được tạo theo snapshot mới | Integration |
| S02 | Chunk lịch sử v1 vẫn còn | Extraction v2 không lấy v1 làm subject/context | Integration |
| S03 | Upload v3 trong khi v2 chạy | Job v2 pin v2; workspace báo có update, không publish nhầm | Integration |
| S04 | Approve sau index, nội dung không đổi | Warning/eligibility đúng, không bắt embed lại vô ích | Integration |
| V01 | Hai NEGATIVE cùng requirement khác scenario | Hai identities độc lập, đều có trong danh sách/release | Unit + integration |
| V02 | Chỉ đổi step/test data | Revision tăng, diff có thay đổi, content hash khác | Unit + integration |
| V03 | Không đổi content/provenance | Không thêm revision; trả đúng version tái dùng | Integration |
| V04 | Approved v1 + draft/rejected v2 | R1 vẫn chọn v1; list không giấu approved baseline | Integration + browser |
| V05 | Edit thành revision mới | Điều hướng ID mới, bản cũ đọc được và không đổi | Browser + integration |
| V06 | Concurrent edit cùng head | Một commit thành công, một 409; không mất user input | Integration + browser |
| V07 | Restore version cũ | Revision mới draft, lineage/audit rõ, release cũ giữ nguyên | Integration + browser |
| V08 | Expected khác evidence | Không approve/publish; hướng dẫn làm rõ requirement | API + integration |
| V09 | Sửa trực tiếp steps/evidence đã sealed | DB/service từ chối, hash và snapshot còn đúng | Integration |
| V10 | Đổi evidence v1 sang v2 nhưng statement không đổi | Proof cũ nguyên trạng, provenance mới được version hóa | Integration |
| V11 | Case revision mới expected giống cũ | Không kế thừa artifact approval hoặc run PASS sai version | Integration |
| V12 | Sinh lại cả bộ, matching mơ hồ | Đề xuất để review, không tự nối lineage hoặc archive | Integration + browser |
| B01 | Publish R2 đồng thời với webhook | Analysis pin toàn bộ R1 hoặc R2, không trộn manifest | Integration |
| B02 | Project đổi release sau khi analysis tạo | Analysis/run cũ không đổi revision | Integration |
| E01 | Export run v1 sau khi có v2 | Đúng v1, exact expected/actual/steps/source; không thiếu dòng | Report integration |
| E02 | Nhiều review trên một revision | Một dòng testcase, reviewer metadata đúng | Report integration |
| E03 | Tải lại file export đã lưu | Bytes/hash không đổi; new export là record riêng | Integration |
| M01 | Migrate dữ liệu legacy có chuỗi khả nghi | Giữ IDs/refs, báo needs review, không fabricate history | Migration integration |
| M02 | Khác set/family/release | Từ chối reference sai ở API và DB | Integration |
| M03 | Backup/restore/purge sau schema mới | Khôi phục được toàn graph, purge vẫn chặn reference đang dùng | Integration + drill |
| A01 | Bàn phím/390 px/desktop | Truy cập được CTA, evidence, diff, review và lỗi | Browser/manual |

Lệnh có sẵn để kiểm tra nền, chạy theo phạm vi phase rồi mới chạy toàn suite ở
release gate:

```bash
make test
make lint
make frontend-typecheck
make frontend-build
make test-integration
```

`make test-integration` cần `TEST_DATABASE_URL` của PostgreSQL test riêng và migrations
đã được áp dụng; không trỏ vào database production. UV-09 đã thêm `make test-browser`
và `make test-migration-drill`; xem điều kiện opt-in/DB riêng trong bằng chứng UV-09.
Kiểm thử browser tích hợp không được
thay hết backend bằng mock rồi coi migration/job/versioning đã được chứng minh.

## 9. Đo xem trải nghiệm đã dễ hơn chưa

### 9.1. Ba nhiệm vụ cho người thử

1. Từ trang Documents trống, tải bộ fixture, duyệt nguồn/yêu cầu, tạo và duyệt
   testcase, xuất Excel của bộ đã chốt.
2. Tìm TC-001 đang dùng, tạo bản sửa, xem diff, duyệt, chốt bộ mới; mở bản cũ để
   xác nhận không mất nội dung.
3. Tải nguồn v2, xem mục ảnh hưởng, cập nhật một testcase; xuất báo cáo của run v1
   và giải thích vì sao expected trong báo cáo đó vẫn là v1.

### 9.2. Chỉ số và mục tiêu ban đầu

| Chỉ số | Mục tiêu ban đầu |
| --- | --- |
| Người mới hoàn thành nhiệm vụ 1 | Cả 3 người thử hoàn thành; không cần SQL/log trong luồng thành công |
| Phải hỏi “bấm đâu tiếp” | Không quá 1 lần/người/nhiệm vụ; ghi đúng bước vướng |
| Refresh/Re-index thủ công | 0 trong luồng thông thường |
| Review 20 requirement hợp lệ | Thực hiện được bằng một màn hình với selected batch, không mở 20 trang |
| Tìm version đang dùng và mở diff | Không quá 3 thao tác từ dòng testcase đến diff hai bản |
| Hiểu bản latest so với pinned | Người thử chỉ đúng bản đang dùng khi có draft successor |
| Cập nhật progress | Thường trong vòng 5 giây khi API/worker hoạt động bình thường |
| Job enqueue latency | Mục tiêu p95 dưới 1 giây trong fixture/demo tải bình thường; đo riêng LLM latency |
| Lỗi có hành động khắc phục | Mọi lỗi trọng tâm trong ma trận có giải thích, next action hoặc request/job ID |
| Data correctness | 0 mất lịch sử/nhầm revision/false approval trong ma trận P0 |

Đo thời gian thao tác tách thời gian chờ AI; không kết luận UX tốt chỉ vì thay
model nhanh hơn. Mẫu 3 người chỉ là smoke usability cho demo, không phải nghiên cứu
thống kê đủ để chứng minh hiệu quả luận văn.

## 10. Migration, tương thích và rollout

### 10.1. Backfill có kiểm soát

- [ ] Trước thay đổi, backup DB + file storage theo quy trình hiện có; kiểm tra
  phục hồi trên môi trường riêng.
- [ ] Dry-run báo số family/revision, successor graph, orphan refs, duplicate keys,
  chains nghi ngờ trộn scenario và các bản approved đang bị draft che khuất.
- [ ] Backfill identity theo key/lineage hiện có để giữ khả năng đọc. Chain khả nghi
  đánh dấu cần review; không tự sửa ý nghĩa lịch sử chỉ bằng embedding similarity.
- [ ] Nếu QA xác định một chain legacy chứa nhiều scenario: tạo identities mới và
  revision draft có `derived_from_legacy_revision_id`, review/chốt lại cho tương lai.
  Không rewrite ID/key/lineage trong run/export cũ để giả rằng lịch sử ban đầu đã đúng.
- [ ] Tính content hash từ dữ liệu còn lưu; nếu thiếu provenance thì ghi unknown.
  Không backfill actor/model/approval time bằng dữ liệu suy đoán.
- [ ] Với project đang bind suite, tạo release chuyển tiếp từ đúng tập mà logic
  hiện hành đang chọn, gắn `origin=MIGRATED_CURRENT_STATE`, timestamp lúc migrate.
  Không coi đây là release được user công bố trong quá khứ; không thêm case đã
  từng bị loại chỉ vì nay tìm thấy một approved version cũ.
  Forward fix 31 giữ timestamp binding sai từ migration 25 trong audit và đặt
  timestamp metadata bằng lúc hiệu chỉnh ở 31; không suy đoán thời gian chạy 25.
  Proof đã persist vẫn giữ nguyên, kể cả metadata thời gian cũ.
- [ ] Analysis/run cũ giữ snapshot/ref cũ; backfill release link chỉ khi chứng minh
  manifest khớp hoàn toàn. Nếu không khớp, trình bày “snapshot lịch sử chưa có release”.
- [ ] Migration idempotent, thống kê trước/sau, validate FK/hash rồi mới nâng constraint.

### 10.2. Chia đợt bàn giao

| Đợt | Phạm vi | Điều kiện bật |
| --- | --- | --- |
| A — Sửa nền dữ liệu | UV-00/01/02/03, regression tests | Snapshot/identity/release consumer cùng hiểu schema; chưa bắt UI mới mặc định |
| B — Luồng dễ dùng | UV-04/05/06 và phần testcase review hiện có đã tương thích | Không cần index/refresh tay; queue/review guards hoạt động |
| C — Version workspace | UV-07 và UV-08 | History/diff/restore/update scope có checks, không đổi run/release cũ |
| D — Mặc định | UV-09 | Toàn bộ release gate, migration drill và usability evidence đạt |

Feature flags đề xuất: `DOCUMENT_WORKFLOW_UI_V2`, `TESTCASE_VERSIONING_V2`.
Flags là cấu hình mới cần triển khai, không phải biến hiện có. Backend kiểm tra
tương thích consumer trước khi bật version writer; không cho FE flag tự quyết policy dữ liệu.

### 10.3. Tương thích và rollback

- [ ] Expand schema trước, backfill và đọc song song, cutover writer sau khi consumer
  tương thích; chỉ remove path cũ ở đợt khác sau khi có bằng chứng không còn dùng.
- [ ] Endpoint cũ đọc revision ID được giữ. Endpoint mutation cũ gọi cùng service
  revision mới trong thời gian chuyển đổi, tránh bypass immutability/cas/approval.
- [ ] Trước cutover, drain job cũ hoặc snapshot hóa đầu vào; không để worker phiên
  bản cũ tiếp tục append evidence dưới trigger mới rồi retry vô hạn.
- [ ] Khi UI mới lỗi: tắt UI flag và dùng màn cũ đã có adapter trên backend mới.
  Không rollback binary không hiểu family/release sau khi đã có dữ liệu mới.
- [ ] Down migration chỉ dùng test hoặc môi trường chưa có record mới; nếu có
  revision/release mới phải từ chối down gây mất dữ liệu và dùng forward fix.
- [ ] Không rollback bằng cách xóa draft/release/history vừa tạo. Khôi phục backup
  là thao tác vận hành riêng, cần cân nhắc dữ liệu phát sinh sau backup.
- [ ] Cập nhật health/readiness, logs/metrics job và runbook với migration order;
  `make prod-rebuild-app` chỉ thực hiện sau backup, schema review và compatibility gate.

## 11. Những quyết định thực hiện cần giữ nhất quán

| Vấn đề | Mặc định trong kế hoạch | Nếu muốn mở rộng sau |
| --- | --- | --- |
| Số bước hiển thị | Bốn bước nghiệp vụ; phần kỹ thuật trong chi tiết | Không thêm stepper riêng cho từng worker |
| Khi nào tự chạy AI | Sau CTA rõ ràng có intent/scope đã lưu | Có thể thêm auto-update opt-in cùng budget policy |
| Requirement chưa rõ | Cho xử lý phần approved, báo phần thiếu | Không tự bịa expected hoặc giả đầy đủ coverage |
| Khi user sửa testcase | Lưu draft revision mới, review riêng | Không collaborative branching trong đợt này |
| Nhiều người sửa | Một head/family, CAS và conflict UI | Merge content có thể là roadmap sau |
| Chọn version để chạy | Project pin suite release cụ thể | “Theo latest approved” nếu thêm sau phải có policy/audit riêng |
| Bản tài liệu mới | Tạo working snapshot mới, giữ published baseline | Không auto-replace baseline project |
| Restore | Copy nội dung cũ thành draft mới | Không đảo số version hoặc xóa successor |
| Code repair | Chỉ artifact đúng testcase revision | Không sửa expected/business payload |
| Ngừng dùng testcase | Archive identity/exclude ở release mới | Purge lịch sử theo policy hiện có, không delete tùy tiện |
| Scope thiếu mapping | Đưa ra cần đối chiếu, giữ evidence | Không khẳng định tự phát hiện hết tác động |
| Role/identity | Kiểm tra quyền server, phân biệt service/user display name | OIDC hoàn chỉnh là nhiệm vụ riêng |

## 12. Checklist tổng và mẫu ghi bằng chứng

- [x] UV-00 — Contract, prototype và fixture tái hiện.
- [x] UV-01 — Source/index snapshot và extraction đúng phạm vi.
- [x] UV-02 — Identity/revision backend, concurrency và migration.
- [x] UV-03 — Suite release, pinning, coverage/run/export đúng version.
- [x] UV-04 — Async jobs, retry, progress và workflow API.
- [ ] UV-05 — Code và kiểm thử kỹ thuật đã đạt; còn nghiệm thu người mới tự sử dụng.
- [x] UV-06 — Requirement batch review và đối chiếu nguồn; kiểm thử kỹ thuật đạt 22/09/2026.
- [ ] UV-07 — Code và kiểm thử kỹ thuật đạt; còn nghiệm thu phân biệt version với người dùng và screen reader.
- [x] UV-08 — Hoàn tất kỹ thuật: proposal-first, affected/selected/all, per-unit retry, all-removed, E2E đổi nguồn/R2 và giữ proof lịch sử; xem nghiệm thu schema 30.
- [ ] UV-09 — Đã có Playwright/CI config, 5/5 journey + 3/3 extraction/generation fault local, SIGKILL/restart + unknown-usage ledger, verifier và synthetic migration/restore schema 31; còn CI remote, billing/provider/demo thật, usability và rollout.

Mỗi lần hoàn thành phase, điền bên dưới hoặc tạo subsection theo phase:

```text
Phase:
Ngày / người thực hiện:
Commit / PR:
Backend / frontend / migration đã bàn giao:
Test IDs đã kiểm tra và kết quả:
Fixture / snapshot / screenshot / video / file Excel:
Đã xác minh trên local / staging / production:
Giới hạn còn lại và phase phụ thuộc:
Definition of Done: đạt / chưa đạt, lý do:
```

Toàn bộ đợt chỉ được coi hoàn tất khi **hành trình người dùng dễ theo dõi và mọi
revision được chọn chính xác**. Việc build pass, thêm dropdown version hoặc đánh
dấu hết checklist code chưa thay thế bằng chứng đó.

### Bằng chứng UV-00 — 17/09/2026

- Query inventory: [QUERY_AND_VERSION_SELECTION_INVENTORY.md](uv00/QUERY_AND_VERSION_SELECTION_INVENTORY.md).
- Contract request/response/error/quyền/version policy:
  [WORKFLOW_AND_VERSIONING_CONTRACT.md](uv00/WORKFLOW_AND_VERSIONING_CONTRACT.md).
- Quyết định kiến trúc: [ADR 0004](adr/0004-workflow-snapshots-testcase-revisions-and-suite-releases.md).
- Prototype bốn bước và ba hành trình:
  [WORKFLOW_UX_PROTOTYPE.md](uv00/WORKFLOW_UX_PROTOTYPE.md).
- Fixture synthetic: `backend/internal/document/testdata/uv00/` và
  `backend/internal/testcase/testdata/uv00/`.
- Characterization test: DATA-01/02 trong package `document`, VER-02/03/04 trong
  package `testcase`, VER-06 trong package `report`.
- Đã qua `go test ./...`, `go vet ./...`, `npm run typecheck` và ba integration
  test UV-00 với PostgreSQL. Không thay đổi hành vi production ở phase này.
- Khảo sát ba người chưa chạy; chỉ mới chốt protocol để không bịa bằng chứng
  usability. Việc thực thi và tổng hợp số liệu được tiếp tục ở UV-09.

### Bằng chứng UV-01 — 18/09/2026

- Migration `000023_document_source_snapshots` tạo source snapshot bất biến,
  generation membership và liên kết extraction/requirement với đúng snapshot.
- Integration tests `uv01_source_snapshot_integration_test.go` ở package
  `document` và `requirement` kiểm tra S01–S04: upload mới làm stale, chunk lịch
  sử không lọt vào subject mới, job đang chạy giữ input, và approval-only change
  không ép embed lại nội dung không đổi.
- API/proxy chuyển generation và precondition cần thiết; trang index hiển thị
  generation đang xem thay vì trộn chunk lịch sử.

### Bằng chứng UV-02 — 19/09/2026

- Migration `000024_testcase_families_revisions` backfill family/public key,
  provenance, canonical content hash, head token, idempotency command và audit;
  row ID/FK cũ được giữ nguyên, chain nghi ngờ được đưa vào migration issues.
- API đã có list family/version, read revision, diff, create revision, restore và
  archive; legacy edit route dùng cùng revision service và trả Location mới.
- `uv02_revision_integration_test.go` kiểm tra hai NEGATIVE độc lập, step/data và
  citation change, no-op, idempotency collision, concurrent head conflict,
  restore v1 thành v4, foreign scope và sealed-child immutability.
- Khi chạy lại toàn bộ integration suite trên PostgreSQL sạch, fixture `scope`
  cũ đã được bổ sung exact evidence/seal theo guardrail UV-02. `go test ./...`,
  `go vet ./...`, frontend typecheck/build và integration `./internal/...` đều đạt.

### Tiến độ UV-03 — 19/09/2026

- Migration `000025_suite_releases_and_pinned_consumers` thêm release/manifest
  bất biến, exact revision items, source snapshot, scope decision và release refs
  cho project baseline, analysis snapshot, run và export. Backfill từ schema 24
  đã được chạy thử: tạo `MIGRATED_CURRENT_STATE`, giữ đúng một testcase legacy
  và nối project baseline vào release chuyển tiếp.
- Publish API có idempotency, family/scope/evidence guard, coverage thiếu và audit
  người publish. Project selector chỉ bind published release; webhook analysis
  snapshot và test run pin một release ID nên publish R2 không trộn vào R1.
- Run export bắt đầu từ `test_run_items`; test E01 chứng minh v1 đã chạy vẫn được
  xuất sau khi có v2 draft. Review được chọn bằng lateral latest nên nhiều review
  event không nhân dòng (E02). XLSX/Markdown metadata chứa release manifest và
  danh sách testcase revision.
- UI đã có chọn revision để publish, danh sách Rn, project release selector và
  export tách rõ working set / published release / run.
- Coverage công khai bốn lớp `DESIGNED/PUBLISHED/AUTOMATED/EXECUTED`
  cùng denominator, source snapshot và Rn. Workspace chỉ hiện actual/status
  của run gắn đúng revision; test PASS v1 không lan sang successor v2.
- Integration test chạy lặp thao tác đổi binding R1/R2 đồng thời với
  webhook snapshot, và xác nhận mỗi analysis nhận trọn một manifest. Exact
  testcase ID, expected hash và approved artifact policy chặn artifact khác revision.
- UV-03 đã hoàn tất; toàn bộ unit, vet, frontend production build và
  PostgreSQL integration suite đã qua trên schema 25.

### Tiến độ UV-04 — 19/09/2026

- Migration `000026_document_workflow_jobs` thêm job/unit chung với frozen input,
  idempotency, optimistic revision, lease/heartbeat, bounded retry, cancel,
  output refs và attempt-level budget attribution. Down/up round-trip đã chạy
  trên PostgreSQL; extraction được bridge vào queue cũ thay vì có scheduler thứ hai.
- `GET .../workflow` cung cấp capabilities theo role, blockers có mã và next
  action. Ba mutation index/extract/generate trả HTTP 202/status URL; retry/cancel
  dùng expected revision và worker chỉ publish khi lease/cancel guard còn hợp lệ.
- Shared frontend hook có backoff, visibility refresh, abort/stale-response guard
  và terminal stop. Các trang index/requirements/testcases khôi phục job đang
  chạy hoặc lỗi từ server, hiển thị progress/error và action đúng quyền.
- Regression tests bao phủ idempotent replay/collision, input stale, cancel đối
  đầu commit, manual retry, lease recovery sau crash, delegated extraction,
  usage còn dấu khi kết quả provider chưa chắc chắn và structured HTTP blocker.
- UV-04 đã hoàn tất sau khi unit, vet, frontend production build và PostgreSQL
  integration suite chạy đạt trên schema 26.

### Tiến độ UV-07 — 22/09/2026

- Workspace identity/history/diff/editor/restore/release và guards backend đã bàn
  giao trong working tree (chưa commit). Schema giữ nguyên 28.
- Browser đi qua v1 approved → v2 draft → diff → approve → R2 → restore v3;
  không đổi manifest R2, không kế thừa PASS. Hai tab tạo head conflict, form còn
  nguyên và chỉ lưu v5 sau đối chiếu/rebase rõ ràng. Archive giữ đủ history.
- Unit/vet/typecheck, production build, PostgreSQL integration và Chromium đạt.
  Chi tiết fixture/script/ảnh: [UV07_TESTCASE_VERSIONING_VERIFICATION.md](UV07_TESTCASE_VERSIONING_VERIFICATION.md).
- Chưa đóng phase: còn nghiệm thu screen reader và người mới phân biệt các loại
  version; không tự đóng UV-05 usability hay E2E provider/SCM/sandbox thật.

### Tiến độ UV-08 — chặng nền, 22/09/2026

- Worker dùng exact requirement IDs đã pin; API hỗ trợ optional selected IDs và
  idempotency theo scope. Nguồn thêm trong lúc AI chạy không mở rộng input.
- Bỏ semantic auto-merge trước persist: khác data/steps/source vẫn giữ độc lập.
  Revision mới ghi generation provenance; replay không thay head do QA sửa.
- Unit/vet/typecheck và PostgreSQL integration đã chạy; chi tiết tại
  [UV08_GENERATION_FOUNDATION_VERIFICATION.md](UV08_GENERATION_FOUNDATION_VERIFICATION.md).
- Chặng nền không thêm migration; các phần proposal tiếp nối được ghi bên dưới.

### Tiến độ UV-08 — proposal backend, 23/09/2026

- Migration 29 lưu proposal input/classification/reason/candidate heads bất biến
  và receipt quyết định theo idempotency key. Enqueue opt-in pin exact heads;
  worker không rebase sang human head mới và không ghi testcase trước apply.
- Phân loại đủ năm nhóm; chỉ exact full content + known generation context là
  unchanged. Quan hệ requirement/ancestry/source comparison không tự quyết lineage.
- API list phân trang và reviewer apply/dismiss; CAS head + source/approval guard,
  draft/decision/receipt cùng transaction. Archive không xóa revision/release/run.
- Kiểm thử concurrent replay, human edit khi generate, R1/R2/PASS giữ nguyên,
  retire, rollback batch lỗi, lease/cancel và quyền reviewer. Workflow integration
  dùng service thật xác nhận pin targets và tái sử dụng checkpoint whole-batch.
- Chi tiết lệnh, bằng chứng, giới hạn và rollout:
  [UV08_PROPOSAL_VERIFICATION.md](UV08_PROPOSAL_VERIFICATION.md).
- Tại mốc backend, UI scope/diff/apply còn thiếu; chặng tiếp nối bên dưới đã nối UI.
  Default affected cases, all-removed baseline, retry failed unit và browser journey
  R1/export → source update → proposal → R2 vẫn mở.
  Không tự đóng UV-05/07 human acceptance hoặc E2E provider/SCM/sandbox thật.

### Tiến độ UV-08 — nối giao diện proposal-first, 23/09/2026

- Hai entry pages testcase dùng chung UI: explicit all/selected scope, xác nhận
  nguồn/phạm vi, usage/budget theo quyền và job polling. Không còn generation chính
  direct-draft trên hai trang này; API cũ giữ compatibility.
- Bảng proposal phân trang/lọc job, nội dung/source/provenance, diff exact bản pin.
  Reviewer chọn decision/identity/reason và xác nhận; apply giữ CAS/receipt server,
  không publish/bind/approve tự động. Viewer chỉ đọc.
- Browser kiểm tra mất response sau commit, retry cùng key, KEEP, stale-head
  conflict giữ reason, dismiss rồi REVISE có xác nhận; R1 không đổi, draft không
  kế thừa approval/PASS. Hai trang và viewport 390 px, viewer API 403 đã kiểm tra.
- `make test lint`, production build, toàn bộ PostgreSQL integration đạt.
  Bằng chứng và giới hạn: [UV08_PROPOSAL_UI_VERIFICATION.md](UV08_PROPOSAL_UI_VERIFICATION.md).
- Bước tiếp theo: scope/default affected cases + all-removed baseline, sau đó
  checkpoint/retry từng requirement và E2E đổi nguồn đầy đủ. Chưa đóng UV-08.

### Hoàn tất UV-08 — 24/09/2026

- Đã hoàn thành scope AFFECTED/SELECTED/ALL, mặc định affected khi đã có testcase,
  summary thay đổi nguồn và xác nhận phạm vi. Baseline all-removed sinh retire
  không gọi AI; archive giữ release/run/history.
- Migration 30 liên kết proposal với checkpoint từng requirement. Partial retry
  giữ unit thành công và quyết định reviewer; claim revision ngăn worker cũ ảnh
  hưởng attempt mới. Scope/targets đóng băng, không tự rebase sau human edit.
- PostgreSQL integration kiểm tra lỗi từng phần, KEEP + human edit + retry,
  affected scope không lan sang case khác, R1/run/XLSX giữ nguyên sau R2/archive.
- Browser đầy đủ PASS hai lượt (set 11/12): đổi nguồn thật → affected proposal →
  review v4 → preview/publish R2, rồi removed toàn bộ → archive; viewer 403, diff
  mobile, mất response và head conflict. Provider disabled, không suy ra PASS thật.
- `make test lint`, frontend build, toàn bộ integration, fresh migration 1–30 và
  down/up 30 trên DB trống đều đạt. Đã đóng checklist kỹ thuật UV-08.
  Xem [nghiệm thu và giới hạn](UV08_COMPLETION_VERIFICATION.md).
- UV-09, UV-05/07 human acceptance vẫn mở. Chưa migrate DB chính hoặc deploy/commit.

### Tiến độ UV-09 — 24/09/2026

- Thêm Playwright dependency/lockfile, runner API/worker thật với ba frontend role,
  CI browser/artifact lane và protected/manual provider smoke riêng.
- Browser local 5/5 đạt, gồm UV-05→08 và quyền/token. Unit/lint, frontend build,
  PostgreSQL integration và focused graph purge schema 30 đạt.
- Verifier đối chiếu exact release/revision, manifest, expected/artifact và export;
  test giữ R1 sau bind R2 và từ chối proof trộn hai release.
- Synthetic populated schema 23→30, backup/restore sang DB mới và source checksum
  đạt; không gọi đó là drill trên snapshot production.
- Đồng bộ tài liệu và mẫu human acceptance; xem [ma trận UV-09](UV09_VERIFICATION.md).
  Còn browser fault injection/process-kill, audit trên snapshot triển khai thực,
  CI remote, demo thật, nghiệm thu người mới/screen reader và rollout/rollback.
- Không đổi schema/DB chính, không gọi provider thật, không commit hoặc deploy.

### Tiến độ UV-09 — forward fix timestamp, 24/09/2026

- Thêm migration 31, không sửa migration 25: chỉ hiệu chỉnh timestamp của release
  MIGRATED_CURRENT_STATE và items, lưu bản cũ/reason/correction time vào audit bất biến.
- Synthetic drill tái hiện timestamp 2001, kiểm ID/manifest/binding/run/export không
  đổi, USER_PUBLISHED nguyên trạng, no-op, trigger từ chối write sau hiệu chỉnh,
  backup/restore audit/source; populated down bị chặn, empty down/up đạt.
- Toàn bộ PostgreSQL integration trên schema 31 đạt, gồm purge graph với audit mới.
  Thêm CI populated migration lane dùng SQL assertions chung; chưa chạy CI remote.
- Browser schema 31 đạt 5/5 sau khi sửa race trong assertion UV05: chờ dòng inventory
  hiển thị thay vì đếm ngay khi heading xuất hiện, không reload hoặc automatic retry.
- Chỉ migrate DB test riêng; database chính vẫn giữ nguyên. Gate fault-injection,
  process-kill, staging/human/production chưa được đóng.

### Tiến độ UV-09 — U04 provider fault browser, 25/09/2026

- Local Gemini protocol fixture riêng, fake key, không gửi prompt ra ngoài. Thêm
  lane CI và artifacts riêng, không dùng fixture để kết luận provider thật đã chạy.
- 3/3 đạt: HTTP 400, timeout, status enum APPROVED bị chặn. UI tự hiện lỗi;
  3 attempt rồi dừng, không lưu requirement sai; reload giữ error/progress/job.
- Retry qua UI với output hợp lệ phục hồi cùng job/intent và tạo một requirement
  DRAFT, không duplicate job hay false approval. Kiểm call count và lưu job JSON.
- U05 process-kill/usage reconciliation, generation-specific provider fault,
  pipeline remote và nghiệm thu thật vẫn mở; chưa commit/deploy/migrate DB chính.

### Tiến độ UV-09 — restart, generation faults và P0 regression, 25/09/2026

- Đã bổ sung và đạt 3 SIGKILL modes: đang gọi provider, trước commit unit,
  sau commit checkpoint. Resume cùng job, không nhân proposal; giữ quyết định
  reviewer và từ chối stale heartbeat. Test dùng subprocess thật + provider fixture.
- Unknown usage không tự release vì timeout/HTTP error/TTL. Giữ ngân sách,
  hiển thị reservation chưa đối soát qua API/UI; recorded usage chỉ finalize một
  lần. Không gọi đây là hoàn tất đối soát hóa đơn provider thật.
- 3/3 browser fault mở rộng đạt cả extraction, generation PARTIAL_FAILED/retry,
  reload giữa generation, checkpoint và budget warnings. Chạy lại 5/5 journey
  trên DB sạch schema 31 cũng đạt; report hai lane tách riêng.
- Bổ sung coverage regression VER-07 và bảng mapping DATA/VER trong
  [UV09_VERIFICATION.md](UV09_VERIFICATION.md). CI có lane restart JSON artifacts.
- Phần còn lại cần môi trường/bằng chứng thực: CI remote, provider/SCM/sandbox
  demo và billing receipts, 3 người mới + screen reader, snapshot/rollout/rollback.
  Không tự đánh dấu UV-09 hoàn thành hoặc thay human acceptance bằng browser test.

### Tiến độ UV-09 — kiểm chứng nội dung Excel, 25/09/2026

- Verifier không chỉ kiểm checksum/snapshot: đọc ZIP/XML, đối chiếu ô dữ liệu,
  history, revision/release metadata và công thức Summary với persisted snapshot.
- Thêm regression sửa expected/actual/status/metadata dù checksum được tính lại;
  kiểm routing sheet, duplicate/path/size limits, malformed XML và formula injection.
  Không giải nén ra filesystem hoặc mở external relationship.
- Scope/report integration xác nhận export R1 thật vẫn đạt sau bind R2.
  Đây là bằng chứng tính nhất quán file/snapshot, không thay demo sandbox/provider
  thật hoặc chứng minh người dùng đã mở Excel. Gate thực tế vẫn giữ trạng thái mở.
