// Canonical Vietnamese copy for the UV-00 document workflow contract.
// UV-05 will consume this module when the four-step workspace is implemented.
// User-authored document, requirement, and testcase content must not be translated.

export const documentWorkflowCopy = {
  steps: {
    documents: "Tài liệu",
    requirements: "Yêu cầu",
    testCases: "Testcase",
    useExport: "Sử dụng / Xuất",
  },
  states: {
    notStarted: "Chưa bắt đầu",
    waiting: "Đang chờ xử lý",
    running: "Đang xử lý",
    needsReview: "Cần bạn xem",
    ready: "Sẵn sàng",
    updateAvailable: "Có dữ liệu mới",
    partialFailed: "Hoàn thành một phần",
    failed: "Xử lý thất bại",
    blocked: "Chưa thể tiếp tục",
  },
  actions: {
    createSet: "Tạo bộ tài liệu",
    uploadDocuments: "Tải tài liệu lên",
    uploadDocumentVersion: "Tải phiên bản mới",
    viewAndReviewSource: "Xem và duyệt nguồn",
    approveAndExtract: "Duyệt nguồn và trích xuất yêu cầu",
    reviewRequirements: "Xem và duyệt yêu cầu",
    generateTestCases: "Sinh testcase từ yêu cầu đã duyệt",
    reviewTestCases: "Xem và duyệt testcase",
    saveDraftRevision: "Lưu bản nháp mới",
    compareVersions: "So sánh phiên bản",
    restoreAsDraft: "Phục hồi thành bản nháp mới",
    publishSuite: "Chốt bộ testcase",
    attachToProject: "Gắn bộ vào project",
    exportWorkingSet: "Xuất bộ đang làm việc",
    exportRelease: "Xuất bộ đã chốt",
    exportRun: "Xuất kết quả lần chạy",
    retryFailedStep: "Thử lại bước lỗi",
    cancelJob: "Dừng xử lý",
    viewTechnicalDetails: "Chi tiết xử lý",
  },
  blockers: {
    noDocuments: "Hãy tải ít nhất một tài liệu để bắt đầu.",
    parsing: "Tài liệu đang được đọc. Bạn có thể rời trang và quay lại sau.",
    sourceReviewRequired: "Còn tài liệu cần được kiểm tra và duyệt.",
    indexStale: "Có phiên bản tài liệu mới cần được cập nhật vào dữ liệu tìm kiếm.",
    noApprovedRequirements: "Chưa có yêu cầu đã duyệt để sinh testcase.",
    requirementReviewRequired: "Còn yêu cầu cần xem hoặc làm rõ.",
    noApprovedTestCases: "Chưa có testcase đã duyệt để chốt thành bộ.",
    noSuiteRelease: "Chưa có bộ testcase được chốt.",
    budgetExceeded: "Ngân sách AI của bộ tài liệu không còn đủ cho thao tác này.",
    insufficientRole: "Tài khoản hiện tại không có quyền thực hiện thao tác này.",
  },
  versionLabels: {
    latest: "Bản mới nhất",
    latestApproved: "Bản đã duyệt gần nhất",
    workingCandidate: "Bản đang chuẩn bị",
    pinnedInRelease: "Bản đang dùng trong bộ",
    history: "Lịch sử phiên bản",
    automationVersions: "Phiên bản automation",
    notRun: "Chưa chạy",
  },
} as const;

export type DocumentWorkflowStepKey = keyof typeof documentWorkflowCopy.steps;

