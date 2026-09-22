# Hướng dẫn đọc màn hình Analysis Review

Tài liệu này giải thích các thành phần trên màn hình **Review queue / Analysis**
để người dùng review test do AI sinh và trình bày lại với giảng viên.

> **Lưu ý trạng thái – 10/09/2026:** đây là hướng dẫn cho màn hình code-first
> đang được triển khai trong repo. Kiến trúc đích sẽ thêm Document evidence,
> Requirement baseline, Test-case coverage và phân loại kết quả chạy. Xem
> [DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md](DOCUMENT_DRIVEN_TESTING_REFACTOR_PLAN.md).
> Analysis #9 bên dưới chỉ là ví dụ lịch sử, không phải luồng nghiệp vụ đích.

Với workspace testcase hiện tại, xem phần UV-07 trong
[kịch bản demo](DOCUMENT_DRIVEN_DEMO.md#quản-lý-version-testcase-uv-07) và
[bằng chứng kiểm thử](UV07_TESTCASE_VERSIONING_VERIFICATION.md). Testcase vN là
nội dung nghiệp vụ; automation artifact là mã thực thi; release RN chốt tập exact
revision. Duyệt một loại không tự duyệt hay chuyển PASS sang loại khác.

## 1. Mục đích của màn hình

Đây là nơi tổng hợp toàn bộ bằng chứng của một Pull Request/Merge Request:

- code nào đã thay đổi;
- AI đề xuất cần kiểm thử điều gì;
- AI đã sinh test nào;
- test đã chạy trong Docker Sandbox hay chưa;
- test có được AI sửa lại không;
- model, token và ngữ cảnh nào đã được sử dụng;
- quyết định cuối cùng của người review.

Màn hình này thể hiện nguyên tắc **human-in-the-loop**: AI đề xuất và sinh test,
nhưng con người kiểm tra bằng chứng rồi mới Accept hoặc Reject.

Trong kiến trúc đích, màn hình review phải tách rõ bốn lớp bằng chứng:

1. tài liệu/requirement quyết định scenario và expected result;
2. diff/code chỉ mô tả build và phạm vi kỹ thuật được kiểm thử;
3. automation artifact hiện thực test case;
4. test run cung cấp actual result và evidence.

## 2. Khối thông tin Analysis

| Thành phần | Ý nghĩa |
| --- | --- |
| `Analysis #9` | Mã analysis trong hệ thống |
| `Merge request !9` | Mã Pull/Merge Request bên GitHub hoặc GitLab |
| `Project #2` | Repository đã kết nối trong hệ thống |
| Tiêu đề | Nội dung chính của thay đổi đang được phân tích |
| `source → target` | Nhánh chứa code mới và nhánh đích |
| `Source commit` | Commit mới của nhánh source được đem đi phân tích |
| `Target commit` | Commit gốc dùng để so sánh |
| `Review candidates` | Số test candidate mới nhất cần review |
| `Final decisions` | Số candidate đã có quyết định trên tổng số candidate |
| Status badge | Bước hiện tại của pipeline |

Trong ảnh, Analysis #9 có:

- source branch: `fix/production-secret-paths`;
- target branch: `main`;
- 8 test candidates;
- 0/8 quyết định;
- trạng thái `Validating`.

`Validating` nghĩa là AI đã đề xuất và sinh code test xong, nhưng Docker Sandbox
vẫn đang kiểm tra các test. Đây chưa phải lúc Accept hoặc Reject.

## 3. Changed source

Khối **Changed source** hiển thị bằng chứng gốc lấy từ diff của PR/MR.

| Thành phần | Ý nghĩa |
| --- | --- |
| Tên file | File thật đã thay đổi trong repository |
| `+4 / −6` | Số dòng thêm và xóa |
| Merge-request diff | Nội dung thay đổi cụ thể giữa hai commit |
| Dòng bắt đầu bằng `+` | Code được thêm |
| Dòng bắt đầu bằng `-` | Code bị xóa |

Trong Analysis #9, thay đổi chính là bỏ yêu cầu Gemini response bắt buộc phải có
`id`, đồng thời vẫn sử dụng tên model đã gọi khi response không trả metadata.
File test hiện có cũng được cập nhật để kiểm tra hành vi mới này.

Khi review, cần đọc phần này trước vì mọi test candidate bên dưới phải liên quan
trực tiếp hoặc gián tiếp đến thay đổi tại đây.

## 4. Generated test review

Đây là danh sách test do AI sinh. `8 current` nghĩa là có 8 phiên bản candidate
mới nhất đang cần được kiểm tra.

Mỗi candidate gồm các phần sau:

### Candidate number và tiêu đề

Ví dụ `01`, `02` là số thứ tự để người review theo dõi. Tiêu đề mô tả mục tiêu
của test, không phải tên file hay kết quả test.

### File path và version

Ví dụ:

```text
backend/internal/llm/gemini_test.go · version 1
```

Đây là vị trí AI đề xuất đặt test. `version 1` là bản sinh đầu tiên; nếu sandbox
fail và AI sửa lại, candidate sẽ có version mới nhưng lịch sử cũ vẫn được giữ.

### Recommended scenario

Mô tả tình huống cần kiểm thử. Phần này trả lời câu hỏi:

> Cần tạo điều kiện đầu vào nào để kiểm tra thay đổi?

Ví dụ: mock Gemini API trả response hợp lệ nhưng không có `id`.

Ở màn hình hiện tại, scenario được AI suy luận chủ yếu từ diff và code context.
Sau refactor, scenario phải liên kết với requirement/use-case flow đã được duyệt;
diff chỉ giúp chọn scenario nào cần chạy cho PR/MR.

### Expected behaviour

Kết quả hệ thống được kỳ vọng trả về khi chạy scenario. Ví dụ: backend không báo
`ErrMalformedResponse`, ID được để rỗng và output vẫn được đọc thành công.

Sau refactor, expected behaviour chỉ hợp lệ khi có trích dẫn tới document
version và source locator. Không được lấy hành vi hiện tại của implementation
làm bằng chứng rằng expected result là đúng.

### Why this was suggested

Lý do AI cho rằng test này cần thiết. Kiến trúc đích phải trình bày riêng mối
liên hệ với requirement và lý do test được chọn cho diff; hai loại liên kết này
không được nhập làm một.

### Generated test

Mã nguồn test Go do AI tạo. Code này mới chỉ là candidate, chưa mặc nhiên đúng.
Nó phải qua kiểm tra cú pháp, compile và thực thi trong Docker Sandbox.

### Status hiển thị trên candidate

Trong lúc pipeline đang chạy, badge trên candidate phản ánh trạng thái chung của
analysis, ví dụ `Validating`. Kết quả thực thi riêng của candidate được xem ở
phần **Validation evidence**. Sau khi review, badge hiển thị quyết định đã lưu.

| Status | Ý nghĩa |
| --- | --- |
| `Validating` | Đang chờ hoặc đang chạy trong sandbox |
| `Accept/Reject` | Đã có quyết định của người review |

Các trạng thái `Passed`, `Failed` và `Timed out` nằm trong từng validation run,
không nên suy ra từ badge `Validating` trên đầu candidate.

## 5. Validation evidence

Phần này hiển thị kết quả thực thi thật trong Docker Sandbox:

- câu lệnh đã chạy;
- exit code;
- thời gian chạy;
- stdout và stderr;
- kết quả Passed, Failed hoặc Timed out;
- candidate version tương ứng.

`0 runs` hoặc `No sandbox validation run has been stored yet` nghĩa là candidate
chưa chạy xong, không có nghĩa là test đã pass.

`Passed` chỉ xác nhận automation chạy thành công với assertion hiện có. Reviewer
vẫn phải kiểm tra assertion có bám expected result từ tài liệu hay không. Pipeline
mới còn phải phân biệt `PRODUCT_FAILED`, `AUTOMATION_ERROR`, `INFRA_ERROR`,
`TIMED_OUT` và `NEEDS_CLARIFICATION`.

## 6. Repair history

Nếu test fail, hệ thống đưa code test cùng log lỗi cho AI để tạo bản sửa mới.
Phần **Repair history** cho biết:

- số lần sửa;
- lý do sửa;
- model và prompt version đã dùng;
- code trước và sau khi sửa.

`0 attempts` nghĩa là chưa cần sửa hoặc validation chưa chạy đến bước sửa. Vòng
sửa được giới hạn để pipeline không chạy vô hạn.

Trong kiến trúc đích, repair chỉ được sửa lỗi kỹ thuật của automation như import,
package, fixture hoặc locator. Nếu actual result khác expected result đã duyệt,
hệ thống phải giữ product failure và không được sửa assertion để làm test pass.

## 7. Human decision

Đây là bước người dùng đưa ra quyết định cuối cùng cho từng candidate.

| Thành phần | Ý nghĩa |
| --- | --- |
| Reviewer | Tên người đánh giá |
| Reviewer note | Giải thích lý do Accept hoặc Reject |
| Accept candidate | Chấp nhận candidate hiện tại |
| Reject candidate | Từ chối candidate hiện tại |

Khi màn hình ghi `Decision unavailable`, analysis chưa đạt `Waiting Review` nên
nút quyết định bị khóa. Candidate chỉ được Accept khi phiên bản hiện tại có một
lần sandbox validation **Passed**; candidate không phù hợp vẫn có thể bị Reject.
Quyết định đã lưu là bất biến và được giữ trong audit trail.

## 8. Sandbox summary

Khối bên phải tóm tắt tín hiệu thực thi:

- **Passes**: tổng số validation run đã Passed;
- **Repairs**: tổng số lần AI đã sửa các test bị lỗi.

Trong ảnh cả hai đều bằng 0 vì Analysis #9 vẫn ở bước `Validating` và chưa lưu
kết quả sandbox nào.

## 9. AI provenance – Reproducible evidence

Đây là nhật ký truy vết các lần gọi AI, giúp kết quả có thể kiểm tra và giải
thích thay vì chỉ hiển thị code cuối cùng.

| Thành phần | Ý nghĩa |
| --- | --- |
| AI calls | Tổng số lần gọi model trong analysis |
| Tokens | Tổng lượng token đã sử dụng |
| Estimated cost | Chi phí ước tính theo đơn giá cấu hình |
| Phase | `recommendation`, `generation` hoặc `repair` |
| Model | Model thực hiện lời gọi |
| Prompt version | Phiên bản prompt để tái lập thí nghiệm |
| Duration | Thời gian model xử lý |
| Status | Lời gọi Completed, Failed hoặc Invalid output |
| Prompt/config hash | Dấu vân tay để phát hiện dữ liệu bị thay đổi |
| Historical context | Snapshot ngữ cảnh RAG tại thời điểm gọi AI |

Analysis #9 đã ghi nhận 11 AI calls và 55.582 tokens. Một số test được sinh bởi
model fallback, cho thấy hệ thống có thể chuyển model khi model chính không hoàn
thành request. Giá trị `$0.0000` không có nghĩa API chắc chắn miễn phí; nó cho
biết đơn giá token chưa được cấu hình trong hệ thống.

Nút **Download evidence JSON** tải toàn bộ prompt, response, context, commit,
token và metadata để làm bằng chứng cho báo cáo hoặc thí nghiệm đồ án.

## 10. Change impact, Changed symbols và Retrieved context

Các khối nằm thấp hơn ở sidebar giải thích phạm vi mà hệ thống dùng để ra quyết
định:

- **Change impact**: symbol thay đổi trực tiếp và symbol bị ảnh hưởng gián tiếp;
- **Changed symbols**: function, method hoặc type được ánh xạ từ diff;
- **Retrieved context**: code, interface, mock và test liên quan lấy từ index của
  đúng project để đưa vào prompt AI.

Các phần này chứng minh AI không nhận toàn bộ repository một cách tùy ý mà nhận
ngữ cảnh đã được phân tích và lọc.

Đây là **technical context**, không phải nguồn sự thật nghiệp vụ. Màn hình đích
phải có thêm Document evidence và Requirement trace; chỉ các phần đó mới được
dùng để xác nhận expected behaviour.

## 11. Checklist review một candidate

Trước khi Accept, người review nên kiểm tra:

- [ ] Scenario có requirement/use-case flow và source locator rõ ràng không?
- [ ] Expected behaviour có dẫn về phiên bản tài liệu đã duyệt không?
- [ ] Scenario có lý do rõ ràng để được chọn cho diff/PR/MR này không?
- [ ] Có assumption, conflict hoặc TBD nào đang bị trình bày như sự thật không?
- [ ] Test có đặt đúng package và file path không?
- [ ] Mock response có đúng cấu trúc API thật đang sử dụng không?
- [ ] Test có assertion rõ ràng, không chỉ chạy mà không kiểm tra kết quả không?
- [ ] Test có trùng với candidate khác hoặc test sẵn có không?
- [ ] Validation evidence có trạng thái Passed không?
- [ ] Test có thay đổi production code hoặc thực hiện thao tác nguy hiểm không?
- [ ] Nếu có repair, phiên bản mới đã thật sự sửa đúng nguyên nhân fail chưa?
- [ ] Repair có giữ nguyên scenario và expected result nghiệp vụ không?
- [ ] Reviewer note đã giải thích đủ lý do Accept/Reject chưa?

Không nên Accept chỉ vì code nhìn hợp lý. Sandbox **Passed** chứng minh test chạy
được, còn người review vẫn phải xác nhận test đúng mục tiêu và có giá trị.

## 12. Cách giải thích Analysis #9 với giảng viên (ví dụ legacy)

> Analysis #9 phân tích thay đổi cho phép backend nhận Gemini response không có
> interaction ID. Hệ thống lấy diff của hai file, dùng phân tích code và RAG để
> tạo 8 test candidates. Các lời gọi recommendation và generation được lưu trong
> AI provenance, tổng cộng 11 calls và 55.582 tokens. Hiện analysis đang ở bước
> Validating nên các test đang chờ chạy trong Docker Sandbox, số Pass và Repair
> vẫn bằng 0, đồng thời nút Accept/Reject bị khóa. Khi validation hoàn tất, người
> review sẽ đọc scenario, code, log và lịch sử sửa trước khi quyết định. Như vậy
> hệ thống không tin trực tiếp kết quả AI mà kiểm chứng bằng thực thi và con người.

Đoạn trên chỉ mô tả baseline hiện tại. Khi trình bày kiến trúc mục tiêu, cần nói
thêm rằng scenario/expected result đến từ requirement đã duyệt, còn diff/code
chỉ giúp tạo automation và chọn build cần chạy.

## 13. Nhận xét review nhanh cho Analysis #9

Từ code đang hiển thị, chưa nên Accept candidate nào trước khi sandbox hoàn tất:

- Candidate 01 mock response theo dạng `output`, trong khi Gemini Interactions
  adapter hiện đọc `steps` và `model_output`.
- Candidate 02 dùng dạng `candidates`, gần với Generate Content API hơn
  Interactions API và còn thiếu trạng thái `completed`.
- Candidate 03 gọi `NewGeminiProvider` có dấu hiệu thiếu tham số
  `maxOutputTokens`, nên có khả năng không compile.
- Nhiều candidate đang kiểm tra các trường hợp khá giống nhau, cần loại bỏ test
  trùng hoặc ít giá trị sau khi xem kết quả validation.

Các điểm trên minh họa đúng vai trò của màn hình: AI tạo đề xuất nhanh nhưng có
thể nhầm API hoặc chữ ký hàm; sandbox phát hiện lỗi kỹ thuật, còn con người đánh
giá tính đúng đắn và giá trị của test.
