"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { DocumentVersion, DocumentVersionDetail, DocumentWorkflow, SourceDocument } from "@/lib/types";
import { UploadDocument } from "@/components/upload-document";
import { DocumentBlockPreview } from "@/components/document-block-preview";

const parseLabels: Record<string, string> = { UPLOADED: "Chờ đọc", PARSING: "Đang đọc", PARSED: "Đã đọc", FAILED: "Không đọc được" };
const approvalLabels: Record<string, string> = { DRAFT: "Chưa duyệt", APPROVED: "Đã duyệt", REJECTED: "Đã từ chối" };

export function DocumentSourceWorkspace({ setId, documents: initialDocuments, workflow: initialWorkflow, maxBytes }: {
  setId: number; documents: SourceDocument[]; workflow: DocumentWorkflow; maxBytes: number;
}) {
  const [documents, setDocuments] = useState(initialDocuments);
  const [workflow, setWorkflow] = useState(initialWorkflow);
  const [refreshTick, setRefreshTick] = useState(0);
  const [pollError, setPollError] = useState(false);
  const refresh = () => { setRefreshTick((value) => value + 1); window.dispatchEvent(new Event("document-workspace-updated")); };
  useEffect(() => {
    const controller = new AbortController();
    let timer: number;
    const poll = async () => {
      try {
        if (document.visibilityState !== "visible") return;
        const responses = await Promise.all(["documents", "workflow"].map((part) =>
          fetch(`/api/backend/api/document-sets/${setId}/${part}`, { cache: "no-store", signal: controller.signal })));
        if (responses.some((response) => !response.ok)) throw new Error("poll failed");
        const [source, state] = await Promise.all(responses.map((response) => response.json())) as [{ documents: SourceDocument[] }, DocumentWorkflow];
        if (!controller.signal.aborted) { setDocuments(source.documents); setWorkflow(state); setPollError(false); }
      } catch { if (!controller.signal.aborted) setPollError(true); }
      finally { if (!controller.signal.aborted) timer = window.setTimeout(poll, 3000); }
    };
    void poll();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [setId, refreshTick]);
  const [selected, setSelected] = useState<DocumentVersion[]>([]);
  const [excluded, setExcluded] = useState<number[]>([]);
  const [reviewer, setReviewer] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [technical, setTechnical] = useState("");
  const [success, setSuccess] = useState("");
  const [target, setTarget] = useState<SourceDocument>();
  const [opened, setOpened] = useState<{ documentID: number; version: number }>();
  const [detail, setDetail] = useState<DocumentVersionDetail>();
  const [previewError, setPreviewError] = useState("");
  const [history, setHistory] = useState<{ document: SourceDocument; versions: DocumentVersion[] }>();
  const [historyLoading, setHistoryLoading] = useState(false);
  const historyRequest = useRef(0);
  const historyAbort = useRef<AbortController | undefined>(undefined);
  const previewHeading = useRef<HTMLHeadingElement>(null);
  const commandKey = useRef<{ body: string; key: string } | undefined>(undefined);
  useEffect(() => { try { setReviewer(sessionStorage.getItem("source-reviewer-display") ?? ""); } catch { /* optional */ } }, []);
  useEffect(() => () => historyAbort.current?.abort(), []);
  const openedVersion = documents.find((item) => item.id === opened?.documentID)?.latest_version;
  const openedStatus = openedVersion && openedVersion.version_number === opened?.version ? `${openedVersion.parse_status}:${openedVersion.approval_status}` : "historical";
  useEffect(() => {
    if (!opened) return;
    const controller = new AbortController(); setDetail(undefined); setPreviewError("");
    void fetch(`/api/backend/api/documents/${opened.documentID}/versions/${opened.version}`, { cache: "no-store", signal: controller.signal })
      .then(async (response) => { if (!response.ok) throw new Error("Không tải được phiên bản. Hãy mở lại tài liệu."); return response.json() as Promise<DocumentVersionDetail>; })
      .then((result) => { if (!controller.signal.aborted) { setDetail(result); window.setTimeout(() => previewHeading.current?.focus(), 0); } })
      .catch((caught) => { if (!controller.signal.aborted) setPreviewError(caught instanceof Error ? caught.message : "Không tải được preview."); });
    return () => controller.abort();
  }, [opened, openedStatus]);
  const toggleSelected = (version: DocumentVersion) => setSelected((old) => old.some((item) => item.id === version.id) ? old.filter((item) => item.id !== version.id) : [...old, version]);
  const toggleExcluded = (version: DocumentVersion) => { setExcluded((old) => old.includes(version.id) ? old.filter((id) => id !== version.id) : [...old, version.id]); setSelected((old) => old.filter((item) => item.id !== version.id)); };
  const showHistory = async (item: SourceDocument) => {
    const sequence = ++historyRequest.current; historyAbort.current?.abort();
    const controller = new AbortController(); historyAbort.current = controller; setHistoryLoading(true); setError("");
    try {
      const response = await fetch(`/api/backend/api/document-sets/${setId}/documents/${item.id}/versions`, { cache: "no-store", signal: controller.signal });
      if (!response.ok) throw new Error("Không tải được lịch sử. Hãy thử lại.");
      const result = await response.json() as { versions: DocumentVersion[] };
      if (sequence === historyRequest.current) setHistory({ document: item, versions: result.versions });
    } catch (caught) { if (!controller.signal.aborted) setError(caught instanceof Error ? caught.message : "Không tải được lịch sử."); }
    finally { if (sequence === historyRequest.current) setHistoryLoading(false); }
  };
  const send = async (command: "APPROVE" | "APPROVE_AND_EXTRACT" | "EXTRACT" | "INDEX") => {
    if (pending) return;
    if (command.startsWith("APPROVE") && !reviewer.trim()) { setError("Điền tên hiển thị người duyệt trước khi xác nhận."); return; }
    setPending(true); setError(""); setSuccess(""); setTechnical("");
    const body = JSON.stringify({ command, expected_source_revision: workflow.source_revision,
      selected: command.startsWith("APPROVE") ? selected.map((item) => ({ version_id: item.id, sha256: item.sha256, approval_status: item.approval_status })) : [],
      excluded_version_ids: excluded, reviewer_name: reviewer.trim() });
    if (!commandKey.current || commandKey.current.body !== body) commandKey.current = { body, key: crypto.randomUUID() };
    try {
      const response = await fetch(`/api/backend/api/document-sets/${setId}/source-review`, {
        method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": commandKey.current.key }, body,
      });
      const result = await response.json().catch(() => ({})) as { message?: string; code?: string; request_id?: string };
      if (!response.ok) {
        setTechnical(`${result.code ?? response.status} · ${result.message ?? ""} · request ${result.request_id ?? "—"}`);
        throw new Error(response.status === 409 ? "Nguồn hoặc quyết định duyệt đã thay đổi. Xem lại danh sách và chọn lại đúng phiên bản." : response.status === 403 ? "Tài khoản hiện tại chưa có quyền thực hiện thao tác này." : "Chưa thể tiếp tục. Kiểm tra nguồn đã chọn, nguồn bị loại và trạng thái đọc/duyệt.");
      }
      try { sessionStorage.setItem("source-reviewer-display", reviewer.trim()); } catch { /* optional */ }
      commandKey.current = undefined;
      setSelected([]); setSuccess(command.includes("EXTRACT") ? "Đã lưu yêu cầu. Server sẽ chuẩn bị nguồn rồi trích xuất; bạn có thể rời trang." : "Đã lưu. Trạng thái nguồn sẽ tự cập nhật.");
      refresh();
    } catch (caught) { setError(caught instanceof Error ? caught.message : "Mất kết nối. Thử lại để kiểm tra yêu cầu đã lưu."); }
    finally { setPending(false); }
  };
  const active = workflow.source_intents.some((intent) => ["WAITING_PARSE", "INDEXING", "EXTRACTING"].includes(intent.status) && intent.command !== "INDEX");
  const latestIntent = workflow.source_intents[0];
  const included = documents.flatMap((item) => item.latest_version ? [item.latest_version] : []).filter((v) => !excluded.includes(v.id));
  const canExtractScope = included.length > 0 && included.every((v) => v.parse_status === "PARSED" && (v.approval_status === "APPROVED" || selected.some((s) => s.id === v.id)));
  const approvedScope = included.length > 0 && included.every((v) => v.parse_status === "PARSED" && v.approval_status === "APPROVED");
  const nextDocument = documents.find((item) => item.id !== opened?.documentID && item.latest_version?.parse_status === "PARSED" && item.latest_version?.approval_status !== "APPROVED");
  return <div className="source-workspace">
    {workflow.capabilities.can_upload ? <UploadDocument key={target?.id ?? "new"} setId={setId} maxBytes={maxBytes} target={target} onUploaded={refresh} /> : <p className="notice">Tài khoản chỉ có quyền xem hoặc bộ tài liệu đã lưu trữ. Liên hệ người quản lý để tải nguồn.</p>}
    {pollError ? <p className="notice" role="alert">Chưa cập nhật được nguồn. Kiểm tra kết nối; hệ thống sẽ tự thử lại, quyết định đã lưu không bị mất.</p> : null}
    {target ? <button type="button" className="button secondary" onClick={() => setTarget(undefined)}>Quay lại tải tài liệu mới</button> : null}
    {latestIntent && ["WAITING_PARSE", "INDEXING", "EXTRACTING"].includes(latestIntent.status) ? <p className="notice" role="status">{latestIntent.status === "WAITING_PARSE" ? "Đang chờ đọc tài liệu" : latestIntent.status === "INDEXING" ? "Đang chuẩn bị dữ liệu tìm kiếm" : "AI đang trích xuất yêu cầu"}. Tiến độ được lưu trên server và tự cập nhật.</p> : null}
    {latestIntent?.status === "FAILED" ? <p className="notice" role="alert">Xử lý nguồn chưa hoàn tất. Kiểm tra file lỗi và phạm vi bên dưới; có thể thử lại trong Chi tiết xử lý.</p> : null}
    {latestIntent?.request.excluded_version_ids?.length ? <p className="notice">Phạm vi đã lưu loại {latestIntent.request.excluded_version_ids.length} phiên bản: {documents.filter((item) => latestIntent.request.excluded_version_ids?.includes(item.latest_version?.id ?? 0)).map((item) => `${item.name} v${item.latest_version?.version_number}`).join(", ")}.</p> : null}
    <div className={opened ? "source-review-grid" : ""}>
      <section className="panel"><div className="panel-header"><div><h2>Nguồn tài liệu</h2><p>Mở “Xem & duyệt” để đọc bằng chứng. Không có tài liệu nào được chọn sẵn.</p></div></div>
        {documents.length ? <div className="table-wrap" tabIndex={0} aria-label="Danh sách nguồn tài liệu"><table className="data-table"><thead><tr><th>Chọn duyệt</th><th>Tài liệu</th><th>Trạng thái</th><th>Thao tác</th></tr></thead><tbody>{documents.map((item) => {
          const v = item.latest_version; if (!v) return null;
          return <tr key={item.id}><td><input type="checkbox" aria-label={`Chọn duyệt ${item.name} v${v.version_number}`} checked={selected.some((s) => s.id === v.id)} disabled={pending || !workflow.capabilities.can_review || v.parse_status !== "PARSED" || v.approval_status === "APPROVED" || excluded.includes(v.id)} onChange={() => toggleSelected(v)} /></td>
            <td><strong>{item.name}</strong><span className="table-subtitle">v{v.version_number} · {v.original_filename}</span>{excluded.includes(v.id) ? <span className="form-error">Sẽ loại khỏi phạm vi</span> : null}</td>
            <td><span>{parseLabels[v.parse_status]} · {v.block_count} phần</span><span className="table-subtitle">{approvalLabels[v.approval_status]}</span>{v.parse_error ? <details><summary>Lý do không đọc được</summary><p>Kiểm tra file có nội dung và đúng định dạng; tải bản đã sửa.</p><pre>{v.parse_error}</pre></details> : null}</td>
            <td><div className="source-row-actions"><button type="button" className="button secondary" onClick={() => setOpened({ documentID: item.id, version: v.version_number })}>Xem & duyệt</button>
              {workflow.capabilities.can_upload ? <button type="button" className="button secondary" onClick={() => { setTarget(item); window.scrollTo({ top: 0, behavior: "smooth" }); }}>Tải phiên bản mới</button> : null}
              <button type="button" className="button secondary" onClick={() => void showHistory(item)}>Lịch sử nguồn</button>
              {workflow.capabilities.can_index ? <label className="checkbox-row"><input type="checkbox" checked={excluded.includes(v.id)} disabled={pending || active} onChange={() => toggleExcluded(v)} />Loại khỏi lần xử lý này</label> : null}</div></td></tr>;
        })}</tbody></table></div> : <div className="panel-body"><h3>Chưa có tài liệu</h3><p>Tải tài liệu ở phía trên để bắt đầu. Sau khi đọc xong, nút Xem & duyệt sẽ mở nội dung nguồn.</p></div>}
        {historyLoading ? <p className="panel-body" role="status">Đang tải lịch sử…</p> : null}
        {history ? <div className="panel-body"><h3>Lịch sử · {history.document.name}</h3><ul>{history.versions.map((v) => <li key={v.id}><button className="button secondary" type="button" onClick={() => setOpened({ documentID: history.document.id, version: v.version_number })}>Xem v{v.version_number} · {approvalLabels[v.approval_status]}</button> <span>{v.original_filename}</span></li>)}</ul></div> : null}
      </section>
      {opened ? <aside className="panel source-preview"><div className="panel-body"><h2 ref={previewHeading} tabIndex={-1}>{detail ? `${detail.document.name} · v${detail.version.version_number}` : "Đang mở tài liệu…"}</h2>
        {previewError ? <p className="form-error" role="alert">{previewError}</p> : null}
        {detail ? <><p>{approvalLabels[detail.version.approval_status]} · {parseLabels[detail.version.parse_status]} · {detail.version.block_count} phần</p>
          <Link href={`/documents/${setId}/items/${detail.document.id}/versions/${detail.version.version_number}`}>Mở trang phiên bản</Link>
          {detail.version.id !== openedVersion?.id ? <p className="notice">Đang xem lịch sử. Việc duyệt ở workspace áp dụng cho bản mới nhất đã chọn trong danh sách.</p> : null}
          <DocumentBlockPreview blocks={detail.blocks} />{!detail.blocks.length ? <p>Chưa có nội dung để xem. Nếu file lỗi, tải phiên bản đã sửa.</p> : null}
          {detail.version.id === openedVersion?.id && workflow.capabilities.can_review && detail.version.parse_status === "PARSED" && detail.version.approval_status !== "APPROVED" ? <button type="button" className="button" disabled={pending || excluded.includes(detail.version.id)} onClick={() => toggleSelected(detail.version)}>{selected.some((s) => s.id === detail.version.id) ? "Bỏ chọn duyệt phiên bản này" : "Chọn phiên bản này để duyệt"}</button> : null}</> : null}
        <div className="decision-actions"><button type="button" className="button secondary" onClick={() => setOpened(undefined)}>Quay lại danh sách</button>{nextDocument?.latest_version ? <button type="button" className="button secondary" onClick={() => setOpened({ documentID: nextDocument.id, version: nextDocument.latest_version!.version_number })}>Xem nguồn cần duyệt tiếp theo</button> : null}</div>
      </div></aside> : null}
    </div>
    {documents.length ? <section className="panel"><div className="panel-body"><h2>Xác nhận phạm vi nguồn</h2>
      <p>Đang chọn duyệt {selected.length} phiên bản; xử lý {included.length}/{documents.length} tài liệu. Loại rõ ràng {excluded.length} phiên bản.</p>
      {selected.length ? <ul>{selected.map((v) => <li key={v.id}>{v.original_filename} · v{v.version_number} <button type="button" onClick={() => toggleSelected(v)} aria-label={`Bỏ chọn ${v.original_filename}`}>Bỏ chọn</button></li>)}</ul> : null}
      {workflow.capabilities.can_review ? <label><span>Tên hiển thị người duyệt</span><input value={reviewer} maxLength={160} disabled={pending} onChange={(event) => setReviewer(event.target.value)} /><small>Tên này chỉ là ghi chú audit. Quyền và tài khoản thực hiện do server xác định.</small></label> : <p>Cần quyền reviewer để duyệt nguồn. Nguồn đã duyệt có thể được editor yêu cầu trích xuất.</p>}
      <p>“Duyệt & trích xuất” lưu quyết định duyệt trước khi chạy AI trên các nguồn trong phạm vi. Yêu cầu sinh ra vẫn là bản nháp.</p>
      <div className="decision-actions">
        {workflow.capabilities.can_review ? <><button className="button secondary" type="button" disabled={pending || active || selected.length === 0} onClick={() => void send("APPROVE")}>Duyệt {selected.length} phiên bản đã chọn</button>
          <button className="button" type="button" disabled={pending || active || selected.length === 0 || !canExtractScope} onClick={() => void send("APPROVE_AND_EXTRACT")}>{pending ? "Đang lưu…" : "Duyệt nguồn & trích xuất yêu cầu bằng AI"}</button></> : null}
        {workflow.capabilities.can_index && approvedScope ? <button className="button" type="button" disabled={pending || active} onClick={() => void send("EXTRACT")}>Trích xuất yêu cầu từ nguồn đã duyệt bằng AI</button> : null}
        {workflow.capabilities.can_index && excluded.length ? <button className="button secondary" type="button" disabled={pending || active || included.length === 0} onClick={() => void send("INDEX")}>Xác nhận loại nguồn & xử lý phần còn lại</button> : null}
      </div>
      {!canExtractScope ? <p className="field-hint">Để trích xuất, các nguồn trong phạm vi cần đọc xong và được duyệt hoặc được chọn duyệt trong thao tác này.</p> : null}
      {error ? <p className="form-error" role="alert">{error}</p> : null}{technical ? <details><summary>Chi tiết lỗi</summary><p>{technical}</p></details> : null}
      {success ? <p className="form-success" role="status">{success}</p> : null}
    </div></section> : null}
  </div>;
}
