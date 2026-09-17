# UV-00 workflow and testcase versioning contract

- Contract version: `uv00-draft-1`
- Date: 17/09/2026
- Status: accepted as the implementation input for UV-01 through UV-08
- Authority: ADR 0003 and ADR 0004

This document defines meanings and example payloads. Routes and tables are not
implemented merely because they appear here. Existing endpoints remain the
compatibility surface until their replacement phase is completed.

## 1. Terms with one meaning

| Term | Contract meaning |
| --- | --- |
| Working source | Latest selected document versions being prepared; may contain drafts |
| Source snapshot | Immutable list of selected document version IDs/checksums for one operation |
| Current index | Successful index generation whose source snapshot equals the selected working/approved snapshot required by the action |
| Testcase family | Stable identity of one business scenario |
| Testcase revision | Immutable content and provenance for one family version |
| Latest revision | Greatest revision number, regardless of review status |
| Latest approved revision | Greatest approved revision; it remains available when a newer draft/rejected revision exists |
| Working candidate | Revision proposed for the next release; explicit, never inferred from “leaf” alone |
| Suite release | Immutable family → approved revision manifest |
| Pinned revision | Exact revision contained in a release, analysis snapshot, artifact, run, or export |
| Review actor | Authenticated service/session actor plus declared reviewer display name where end-user authentication is unavailable |

API field names use English for compatibility. The default new workspace labels
are Vietnamese and are centralized in `frontend/lib/document-workflow-copy.ts`.

## 2. Canonical testcase revision payload

The server canonicalizes and hashes this semantic structure. The example IDs are
synthetic fixture values.

```json
{
  "family_id": 7001,
  "base_revision_id": 7101,
  "title": "Từ chối đặt hàng khi giỏ hàng rỗng",
  "test_type": "NEGATIVE",
  "risk": "HIGH",
  "actor": "Người mua",
  "precondition": "Người mua đã đăng nhập",
  "test_data": "cart_items=[]",
  "steps": [
    {
      "ordinal": 1,
      "action": "Mở trang xác nhận đơn hàng",
      "expected_result": "Trang xác nhận được hiển thị"
    },
    {
      "ordinal": 2,
      "action": "Nhấn Đặt hàng",
      "expected_result": "Hiển thị Giỏ hàng trống"
    }
  ],
  "expected_result": "Hệ thống không tạo đơn",
  "postcondition": "Không phát sinh đơn hàng",
  "assumptions": [],
  "requirement_revision_ids": [4201],
  "evidence_refs": [
    {
      "requirement_revision_id": 4201,
      "document_version_id": 2101,
      "document_block_id": 3107,
      "source_locator": "lines:18-19",
      "excerpt_hash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    }
  ],
  "source_snapshot_id": 1101,
  "change_reason": "Bổ sung kiểm tra message theo UC-B08 EX-01"
}
```

Hash rules:

- Include all semantic fields above except `family_id`, `base_revision_id`,
  database IDs that are not source references, timestamps, review/execution state,
  `change_reason`, and UI-only order outside ordered steps.
- Include exact requirement revision and evidence references so an authority
  change is visible even when the sentence remains the same.
- Trim outer whitespace and normalize line endings. Do not lowercase or collapse
  inner whitespace in `test_data`, action, expected result, or other literal text.
- Sort set-like ID/reference lists using a documented stable key. Preserve steps
  by `ordinal`.
- Canonical JSON uses deterministic key ordering and UTF-8; hash with SHA-256.
- A replay with the same canonical content and provenance reuses the revision;
  different content under the same idempotency key returns `409`.

## 3. Source snapshot and index contract

```json
{
  "id": 1102,
  "document_set_id": 1,
  "source_revision": 4,
  "status": "APPROVED",
  "fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "items": [
    {
      "document_id": 10,
      "document_version_id": 21,
      "version_number": 2,
      "sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "approval_status": "APPROVED",
      "included": true,
      "exclusion_reason": ""
    }
  ],
  "created_at": "2026-09-17T08:00:00Z"
}
```

Rules:

- At most one selected version per logical document in one snapshot.
- Every item belongs to the same document set; hashes and approval state are
  copied for provenance and verified when the snapshot is created.
- A working snapshot may contain drafts; extraction/generation default to an
  approved snapshot. Excluded/failed documents remain visible with a reason.
- An index generation references one snapshot and stores explicit chunk membership.
- Extraction subject IDs and retrieved context IDs must be members of that
  generation. Historical chunks may be read only through a historical generation.
- Uploading a source changes `source_revision` and causes workflow freshness to
  become `UPDATE_AVAILABLE`; it does not mutate an old snapshot/generation.

## 4. Testcase family, revision, and release responses

Family summary:

```json
{
  "id": 7001,
  "public_key": "TC-ORDER-001",
  "document_set_id": 1,
  "test_suite_id": 51,
  "archived": false,
  "latest_revision": {"id": 7103, "version_number": 3, "status": "DRAFT"},
  "latest_approved_revision": {"id": 7102, "version_number": 2, "status": "APPROVED"},
  "working_candidate_revision_id": 7103,
  "pinned_revisions": [
    {"release_id": 9101, "release_name": "R1", "revision_id": 7102}
  ],
  "source_status": "UPDATE_AVAILABLE",
  "automation_status": "NEEDS_VALIDATION"
}
```

Create revision request:

```http
POST /api/test-case-families/7001/versions
Idempotency-Key: 98dcb099-53b1-4d7a-95b1-542b79846cf3
Content-Type: application/json
```

```json
{
  "base_revision_id": 7102,
  "expected_head_revision_id": 7102,
  "content": {
    "title": "Từ chối đặt hàng khi giỏ hàng rỗng",
    "test_type": "NEGATIVE",
    "risk": "HIGH",
    "actor": "Người mua",
    "precondition": "Người mua đã đăng nhập",
    "test_data": "cart_items=[]",
    "steps": [
      {"action": "Nhấn Đặt hàng", "expected_result": "Hiển thị Giỏ hàng trống"}
    ],
    "expected_result": "Hệ thống không tạo đơn",
    "postcondition": "Không phát sinh đơn hàng",
    "assumptions": [],
    "requirement_revision_ids": [4201],
    "evidence_refs": [
      {"requirement_revision_id": 4201, "document_version_id": 2101, "document_block_id": 3107}
    ]
  },
  "reason": "Làm rõ bước kiểm tra message"
}
```

Success is `201` with the new revision ID, version, content hash, and location.
If the canonical payload equals an existing idempotent result, return that same
result. If the head changed, return `409 REVISION_HEAD_CHANGED`.

Review request separates decision from content:

```json
{
  "expected_content_hash": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
  "decision": "APPROVED",
  "reviewer_name": "Trần Văn Trường",
  "comment": "Đúng UC-B08 EX-01"
}
```

The audit also stores the authenticated actor from the trusted request context.
`reviewer_name` is a declared display field until end-user authentication exists.

Suite release request:

```json
{
  "expected_source_snapshot_id": 1102,
  "expected_manifest_hash": "",
  "name": "Checkout R2",
  "reason": "Đã duyệt thay đổi tài liệu v2",
  "items": [
    {"family_id": 7001, "test_case_revision_id": 7103},
    {"family_id": 7002, "test_case_revision_id": 7201}
  ]
}
```

The server validates that every revision is approved, belongs to the suite/set,
is unique by family, and is compatible with the selected source/requirement scope.
The manifest hash is computed by the server. Publishing does not change a project
baseline.

## 5. Workflow read model

```json
{
  "document_set_id": 1,
  "workflow_revision": 27,
  "source_revision": 4,
  "working_source_snapshot_id": 1103,
  "approved_source_snapshot_id": 1102,
  "steps": [
    {
      "key": "DOCUMENTS",
      "state": "READY",
      "summary": "2 tài liệu đã duyệt",
      "blocking_reasons": [],
      "capabilities": ["UPLOAD_DOCUMENT", "UPLOAD_DOCUMENT_VERSION", "VIEW_HISTORY"]
    },
    {
      "key": "REQUIREMENTS",
      "state": "NEEDS_REVIEW",
      "summary": "8 đã duyệt · 4 cần xem",
      "blocking_reasons": [],
      "job": {"id": 8801, "type": "REQUIREMENT_EXTRACTION", "status": "SUCCEEDED"},
      "capabilities": ["VIEW", "BULK_REVIEW"]
    },
    {
      "key": "TEST_CASES",
      "state": "BLOCKED",
      "summary": "Chưa sinh testcase cho bản yêu cầu hiện tại",
      "blocking_reasons": [
        {"code": "REQUIREMENT_REVIEW_REQUIRED", "message": "Còn 4 yêu cầu cần xem"}
      ],
      "capabilities": ["VIEW_HISTORY"]
    },
    {
      "key": "USE_EXPORT",
      "state": "NOT_STARTED",
      "summary": "Chưa có bộ testcase được chốt",
      "blocking_reasons": [{"code": "NO_SUITE_RELEASE", "message": "Chưa chốt bộ testcase"}],
      "capabilities": []
    }
  ],
  "next_action": {
    "code": "REVIEW_REQUIREMENTS",
    "label": "Xem và duyệt 4 yêu cầu",
    "href": "/documents/1?step=requirements"
  },
  "updated_at": "2026-09-17T08:05:00Z"
}
```

Step states are `NOT_STARTED`, `WAITING`, `RUNNING`, `NEEDS_REVIEW`, `READY`,
`UPDATE_AVAILABLE`, `PARTIAL_FAILED`, `FAILED`, or `BLOCKED`. The server supplies
capabilities and blocking codes; the UI does not infer authority from counts.

## 6. Operation and job contract

Enqueue request:

```json
{
  "operation": "GENERATE_TEST_CASES",
  "input": {
    "source_snapshot_id": 1102,
    "requirement_revision_ids": [4201, 4202]
  },
  "requested_by_display": "QA demo"
}
```

Response:

```json
{
  "operation_id": 8701,
  "job": {
    "id": 8802,
    "type": "TESTCASE_GENERATION",
    "status": "QUEUED",
    "completed_units": 0,
    "total_units": 2,
    "failed_units": 0,
    "attempt": 0,
    "status_url": "/api/document-workflow-jobs/8802"
  }
}
```

Terminal job statuses are `SUCCEEDED`, `PARTIAL_FAILED`, `FAILED`, and `CANCELED`.
Non-terminal statuses are `QUEUED` and `RUNNING`. A compatibility adapter may map
the existing `PENDING/COMPLETED` values at its boundary.

Rules:

- Enqueue requires `Idempotency-Key` and returns `202` quickly.
- Every operation pins its source/index/requirement/revision inputs.
- Progress uses completed/total units. It does not estimate elapsed percentage.
- A retry references the failed operation and reuses successful committed units.
- A provider timeout after request acceptance is recorded as an uncertain attempt;
  the system does not claim exactly-once provider billing.
- Cancel stops creating new provider calls at safe points and never publishes a
  partial suite release.

## 7. Bulk review response

Request:

```json
{
  "items": [
    {"revision_id": 4201, "expected_content_hash": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
    {"revision_id": 4202, "expected_content_hash": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}
  ],
  "decision": "APPROVED",
  "reviewer_name": "PO demo",
  "comment": "Đối chiếu tài liệu UC-B08"
}
```

Response uses HTTP `200` when all succeed and `207` when valid independent items
have mixed results. A request-level authentication/schema failure uses normal
4xx and applies nothing.

```json
{
  "summary": {"requested": 2, "succeeded": 1, "failed": 1},
  "items": [
    {"revision_id": 4201, "status": "SUCCEEDED", "review_id": 9901},
    {
      "revision_id": 4202,
      "status": "FAILED",
      "error": {
        "code": "REVISION_CHANGED",
        "message": "Yêu cầu đã có nội dung mới; hãy xem lại trước khi duyệt",
        "retryable": false,
        "current_revision_id": 4203
      }
    }
  ]
}
```

The batch service processes each item transactionally and persists an audit for
each applied decision. Retrying the same idempotency key does not add reviews.

## 8. Error envelope

```json
{
  "error": "document index is not current",
  "code": "INDEX_STALE",
  "message": "Có phiên bản tài liệu mới. Hãy cập nhật dữ liệu tìm kiếm trước khi trích xuất yêu cầu.",
  "retryable": true,
  "request_id": "req_01J7...",
  "blocked_by": [{"type": "DOCUMENT_VERSION", "id": 21, "status": "PARSED"}],
  "next_action": {
    "code": "BUILD_CURRENT_INDEX",
    "label": "Cập nhật dữ liệu tài liệu",
    "href": "/documents/1?step=documents"
  }
}
```

Compatibility retains the current `error` string. New clients use `code` for
behavior and `message` for display. `request_id` and technical details belong in
the expandable troubleshooting section. Raw provider payloads are not the main
user message.

Status policy:

- `400`: malformed request;
- `401/403`: authentication/authorization;
- `404`: resource not found within the caller's visible scope;
- `409`: stale head, already-running incompatible operation, or state conflict;
- `422`: well-formed request blocked by source/evidence/business rules;
- `429`: rate/token/cost limit with `Retry-After` where appropriate;
- `502/503/504`: provider, worker, or dependency availability/timeout.

## 9. Navigation and compatibility

- `/documents/{setId}` becomes the four-step workspace.
- Existing detail URLs remain shareable and include a return URL/query preserving
  the selected step and filters.
- `/api/test-cases/{id}` continues to treat `id` as a revision ID.
- Compatibility edit+review returns the new revision ID and `Location`; the UI
  must navigate there instead of refreshing the old URL.
- GET/read endpoints never enqueue jobs or call an LLM.
- Existing service-token auth remains. New mutation routes receive explicit role
  policy and persist authenticated actor separately from declared display names.
