# UV-00 four-step workspace prototype

- Fidelity: text wireframe and interaction contract
- Default language: Vietnamese
- Target route: `/documents/{documentSetId}`
- Goal: validate navigation, state, CTA, and version language before UV-05/07 UI code

The prototype preserves the existing visual direction: compact engineering
workspace, warm neutral surfaces, status text/icons, evidence-first review, and
responsive tables. It changes information architecture and actions, not branding.

## 1. Shared shell

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Đặt hàng thành công                         [Bộ đang hoạt động]      │
│ UC-B08 · Checkout release 1                                           │
├──────────────────────────────────────────────────────────────────────┤
│ ① Tài liệu ✓  ── ② Yêu cầu 8/12 ── ③ Testcase ── ④ Sử dụng / Xuất │
├──────────────────────────────────────────────────────────────────────┤
│ Cần làm tiếp: Còn 4 yêu cầu cần xem                                  │
│ [Xem và duyệt 4 yêu cầu]                         [Chi tiết xử lý ▾] │
├──────────────────────────────────────────────────────────────────────┤
│ Nội dung của bước đang chọn                                          │
└──────────────────────────────────────────────────────────────────────┘
```

Interaction rules:

- The four steps remain visible. Users may open prior/history steps at any time.
- One primary CTA comes from server `next_action`; secondary actions are nearby.
- A blocked future step opens an explanation and valid history, not a dead page.
- “Chi tiết xử lý” contains generation IDs, chunk counts, hashes, provider errors,
  request IDs, retry/cancel, and diagnostic tools.
- Query `?step=documents|requirements|test-cases|use-export` preserves the step.
  Detail links receive a return URL with selected filters/page where practical.
- The header shows working source/release context, so “current” is never implicit.

## 2. Required states

### 2.1. Empty document set

```text
① Tài liệu: Chưa bắt đầu

Tải tài liệu mô tả chức năng cần kiểm thử
Hỗ trợ DOCX và Markdown. Bạn có thể chọn nhiều file.

[Chọn tài liệu]  hoặc kéo file vào đây

Tên bộ: Đặt hàng thành công
Phạm vi bổ sung (thu gọn): product, scope, description
```

Primary CTA: `Tải tài liệu lên` after file selection. Requirement/testcase actions
are not rendered as enabled buttons. Their step explanation says what is missing.

### 2.2. Parsing/indexing

```text
① Tài liệu: Đang xử lý                            2/3 file hoàn tất

✓ URD_Checkout.docx            Đã đọc · 28 phần
◌ UserStories.md               Đang đọc
! API-contract.docx            Không đọc được: file không có nội dung
                                [Tải bản khác] [Loại khỏi đợt này]

Bạn có thể rời trang. Tiến độ được lưu và tự cập nhật.
[Dừng xử lý]                                      [Chi tiết xử lý]
```

The failed file is never silently skipped. Continuing requires successful retry
or an explicit exclusion recorded in the source snapshot.

### 2.3. Parse complete, source review required

```text
① Tài liệu: Cần bạn xem

Tài liệu                  Phiên bản      Kết quả đọc       Duyệt      Hành động
URD Checkout              v2             28 phần            Nháp       [Xem & duyệt]
User stories              v1             12 phần            Đã duyệt  [Xem]

[Tải thêm tài liệu] [Tải phiên bản mới]  [Duyệt nguồn & trích xuất yêu cầu]
```

The combined CTA shows a confirmation of selected exact version IDs and explains
that it records approval before starting an AI extraction operation. It is
available only to a reviewer. An editor sees `Gửi để duyệt` instead.

### 2.4. Requirement extraction/review

```text
② Yêu cầu: Cần bạn xem                           8 đã duyệt · 4 cần xem

[Tất cả] [Cần xem 4] [Cần làm rõ 1] [Đã duyệt 8]

□ UC-B08-EX-01  Không tạo đơn khi giỏ hàng rỗng       [Xem nguồn ▸]
  NGOẠI LỆ · HIGH · Nháp
□ UC-B08-EX-02  Không tạo đơn khi thiếu địa chỉ        [Xem nguồn ▸]
  NGOẠI LỆ · HIGH · Nháp

Đã chọn 2 mục  [Duyệt đã chọn] [Từ chối] [Cần làm rõ]

[Sinh testcase cho 8 yêu cầu đã duyệt]
1 yêu cầu TBD và 3 bản nháp chưa nằm trong phạm vi sinh.
```

The evidence drawer displays document name, business version number, locator,
excerpt, and approval. Selection never means “all results” unless the user chose
and confirmed that scope.

### 2.5. Testcase generation/review

```text
③ Testcase: Cần bạn xem                          16 nháp · 0 đã chốt

TC-ORDER-001   Giỏ hàng rỗng
NEGATIVE · HIGH · v1 Nháp
Expected: Hệ thống không tạo đơn
[Xem nội dung & nguồn] [Lịch sử phiên bản]

TC-ORDER-002   Thiếu địa chỉ nhận hàng
NEGATIVE · HIGH · v1 Nháp
Expected: Hệ thống không tạo đơn
[Xem nội dung & nguồn] [Lịch sử phiên bản]

Đã chọn 2 mục  [Duyệt đã chọn]
[Chốt bộ testcase]  Phạm vi: 16 testcase đã duyệt từ 8 yêu cầu
```

The two negative examples have separate public keys even though they share type,
requirement, and case-level expected result.

### 2.6. Failure and retry

```text
③ Testcase: Xử lý thất bại                       7/8 yêu cầu hoàn tất

Không thể sinh testcase cho UC-B08-EX-02
Nhà cung cấp trả dữ liệu không đúng định dạng confidence.

[Thử lại mục lỗi] [Mở yêu cầu]                    [Chi tiết xử lý]

7 kết quả hợp lệ đã được giữ ở bản nháp và chưa được tự duyệt.
```

Reload displays the same persisted failure. Network errors while enqueueing say
that the app is checking whether the operation was created before offering retry.

### 2.7. New source version available

```text
① Tài liệu: Có dữ liệu mới

Baseline đang dùng: Nguồn S1 · Bộ testcase R1
Bộ đang chuẩn bị:  Nguồn S2 · URD Checkout v2 vừa được tải lên

R1 vẫn dùng tài liệu v1. Chưa có lần chạy hoặc báo cáo cũ nào bị thay đổi.
[Xem thay đổi nguồn] [Chuẩn bị cập nhật yêu cầu]
```

## 3. Testcase version detail

```text
TC-ORDER-001 · Từ chối đặt hàng khi giỏ hàng rỗng
[Nội dung] [Nguồn] [Phiên bản] [Automation] [Lần chạy]

Bản đang xem: v3 Nháp
Bản đã duyệt gần nhất: v2 · trong Checkout R1

Phiên bản
v3  Nháp       AI đề xuất từ nguồn S2       [Xem] [So sánh với v2]
v2  Đã duyệt   QA Lan · Checkout R1          [Xem] [Tạo bản mới từ bản này]
v1  Đã duyệt   QA Minh                       [Xem] [Phục hồi thành bản nháp mới]
```

Edit mode:

```text
Đang sửa từ TC-ORDER-001 v2
Title / Actor / Preconditions / Test data

Steps
1. [Mở trang xác nhận]       Expected [Trang được hiển thị]  [↑][↓][Xóa]
2. [Nhấn Đặt hàng]           Expected [Giỏ hàng trống]       [↑][↓][Xóa]
[Thêm bước]

Expected result / Postcondition / Risk / Source evidence
Lý do thay đổi [________________________________________]

[Hủy] [Lưu bản nháp mới]
```

After save, navigate to the returned revision URL and announce `Đã tạo v3`.
Approval is a separate action over v3/hash. If the head changed, preserve inputs
and offer `So sánh với v3 hiện tại`.

Diff mode:

```text
So sánh TC-ORDER-001: v2 → v3

Test data       - cart_items=[]
                + cart_items=[];currency=VND

Steps           + 1. Chọn khu vực giao hàng
                ~ Các bước cũ chuyển xuống 2–3

Nguồn           - URD Checkout v1 · lines:18-19
                + URD Checkout v2 · lines:20-22

Expected        Không đổi: Hệ thống không tạo đơn
```

Add/remove/change indicators and text labels accompany color. An old revision is
read-only. Restore copies it to a new draft and never changes release membership.

## 4. Three complete journeys

### Journey A — First test suite

```text
Documents empty
→ create set and navigate to it
→ multi-upload
→ parse/index progress
→ review selected sources
→ approve-and-extract intent
→ requirement job progress
→ evidence review and bulk approve
→ generate selected approved requirements
→ testcase review
→ publish suite release R1
→ export R1 or attach R1 to project
```

Success condition: no manual browser refresh, semantic-index page visit, SQL, or
log inspection in the normal path.

### Journey B — Edit one testcase

```text
Open testcase family from R1
→ view v1 and its run history
→ edit from v1
→ save v2 draft and navigate to v2
→ compare v1/v2
→ approve v2
→ publish R2 with v2
→ explicitly bind project to R2
```

R1 and runs on v1 remain readable. Approval of v2 alone does not update project.

### Journey C — New document version

```text
Upload URD v2 to existing document
→ working source becomes S2; R1/S1 remains active
→ parse and build S2 index
→ review/approve source v2
→ compare requirements from S1/S2
→ review changed requirements
→ show affected testcase families
→ generate/apply draft revisions only for selected affected cases
→ review and publish R2
→ export old run from R1 and verify v1 content remains
```

Ambiguous requirement/testcase matches go to `Cần đối chiếu`; the system does not
silently choose lineage or retire a scenario.

## 5. Navigation and responsive behavior

- Desktop: stepper, content list, and evidence/detail drawer may use two columns.
- Narrow screens: stepper becomes a vertical/scrollable ordered list; evidence
  appears after the selected row; tables scroll within their panel.
- Focus moves to the page status after a step/action completes. Job updates use
  polite live regions; errors use alert semantics without repeatedly announcing polling.
- Every icon button has text/accessible name. Diff meaning is available without color.
- Back from detail restores step, query filters, page, and selection where safe.
- Destructive archive/purge controls stay in management, separated from primary CTA.

## 6. Copy ownership

Canonical labels/blocking messages are in
`frontend/lib/document-workflow-copy.ts`. UV-05 may reorganize the module for a
localization framework, but all new workflow components consume one source.
User-authored source, requirement, testcase, evidence, and execution output is
displayed as stored and is never automatically translated by UI copy handling.

