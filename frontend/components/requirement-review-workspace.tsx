"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import type { Requirement, RequirementDetail, RequirementConflict, OpenQuestion, SourceDocument } from "@/lib/types";
import { StatusBadge } from "@/components/status-badge";

type Result = { id: number; status: string; message?: string };
type Audit = { id: number; kind: string; subject_id: number; actor: string; resolution: string; created_at: string };
type Change = { classification: string; before_ids: number[]; after_ids: number[]; reason: string; affected_test_case_ids: number[] };
type Comparison = { id: number; from_snapshot_id?: number; to_snapshot_id: number; items: Change[] };
const defaults = { status: "", type: "", risk: "", flow: "", actor: "", documentId: "", page: 0 };
const blockerText: Record<string, string> = { STALE_REVISION: "Phiên bản không còn hiện hành", SET_INACTIVE: "Bộ đã lưu trữ", MISSING_APPROVED_EVIDENCE: "Thiếu evidence từ nguồn đã duyệt", CLARIFICATION_REQUIRED: "Cần giải quyết conflict/TBD" };
const changeText: Record<string, string> = { ADDED: "Thêm mới", CHANGED: "Thay đổi", REMOVED: "Không còn trong phạm vi", UNCHANGED: "Nội dung giữ nguyên", AMBIGUOUS: "Cần đối chiếu thủ công" };
async function read<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { cache: "no-store", signal });
  if (!response.ok) throw new Error("Chưa tải được dữ liệu. Kiểm tra kết nối và thử lại.");
  return response.json();
}

export function RequirementReviewWorkspace({ setId, requirements: initialRequirements, documents, canReview = false }: {
  setId: number; requirements: Requirement[]; documents: SourceDocument[]; canReview?: boolean;
}) {
  const base = `/api/backend/api/document-sets/${setId}`;
  const [requirements, setRequirements] = useState(initialRequirements);
  const [filters, setFilters] = useState(defaults);
  const [restored, setRestored] = useState(false);
  const [tab, setTab] = useState("inventory");
  const [selected, setSelected] = useState<Requirement[]>([]);
  const [opened, setOpened] = useState<number>();
  const [detail, setDetail] = useState<RequirementDetail>();
  const [previewError, setPreviewError] = useState("");
  const [conflicts, setConflicts] = useState<RequirementConflict[]>([]);
  const [questions, setQuestions] = useState<OpenQuestion[]>([]);
  const [history, setHistory] = useState<Audit[]>([]);
  const [comparisons, setComparisons] = useState<Comparison[]>([]);
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [results, setResults] = useState<Result[]>([]);
  const [pending, setPending] = useState(false);
  const [confirmation, setConfirmation] = useState<"APPROVED" | "REJECTED">();
  const [issue, setIssue] = useState<{ kind: string; id: number }>();
  const [resolution, setResolution] = useState("");
  const [epoch, setEpoch] = useState(0);
  const command = useRef<{ body: string; key: string } | undefined>(undefined);
  const previewHeading = useRef<HTMLHeadingElement>(null);
  const busy = useRef(false);
  const listScroll = useRef(0);
  const rememberScroll = () => { try { sessionStorage.setItem(`requirements-scroll:${setId}`, String(window.scrollY)); } catch { /* optional */ } };
  const reload = () => { setEpoch((value) => value + 1); window.dispatchEvent(new Event("document-workspace-updated")); };
  useEffect(() => {
    try {
      const saved = sessionStorage.getItem(`requirements:${setId}`);
      if (saved) setFilters({ ...defaults, ...JSON.parse(saved) });
      setReviewer(sessionStorage.getItem("source-reviewer-display") ?? "");
      const scroll = sessionStorage.getItem(`requirements-scroll:${setId}`);
      if (scroll) { sessionStorage.removeItem(`requirements-scroll:${setId}`); window.requestAnimationFrame(() => window.requestAnimationFrame(() => window.scrollTo(0, Number(scroll)))); }
    } catch { /* optional */ }
    setRestored(true);
  }, [setId]);
  useEffect(() => { if (restored) { try { sessionStorage.setItem(`requirements:${setId}`, JSON.stringify(filters)); } catch { /* optional */ } } }, [restored, setId, filters]);
  useEffect(() => {
    const abort = new AbortController(); let timer: number;
    const load = async () => {
      try {
        if (document.visibilityState !== "visible" || busy.current) return;
        const [items, conflicts, questions, history, comparisons] = await Promise.all([
          read<{ requirements: Requirement[] }>(`${base}/requirements`, abort.signal),
          read<{ conflicts: RequirementConflict[] }>(`${base}/requirement-conflicts`, abort.signal),
          read<{ open_questions: OpenQuestion[] }>(`${base}/open-questions`, abort.signal),
          read<{ history: Audit[] }>(`${base}/requirement-review/clarification-history`, abort.signal),
          read<{ comparisons: Comparison[] }>(`${base}/requirement-review/source-comparisons`, abort.signal),
        ]);
        if (!abort.signal.aborted) { setRequirements(items.requirements); setConflicts(conflicts.conflicts); setQuestions(questions.open_questions); setHistory(history.history); setComparisons(comparisons.comparisons); }
      } catch (caught) { if (!abort.signal.aborted) setError((caught as Error).message); }
      finally { if (!abort.signal.aborted) timer = window.setTimeout(load, 10000); }
    };
    void load(); return () => { abort.abort(); window.clearTimeout(timer); };
  }, [base, epoch]);
  useEffect(() => {
    if (!opened) return;
    const abort = new AbortController(); setDetail(undefined); setPreviewError("");
    void read<RequirementDetail>(`/api/backend/api/requirements/${opened}`, abort.signal).then((value) => {
      if (!abort.signal.aborted) { setDetail(value); window.setTimeout(() => previewHeading.current?.focus(), 0); }
    }).catch((caught) => { if (!abort.signal.aborted) setPreviewError((caught as Error).message); });
    return () => abort.abort();
  }, [opened, epoch]);
  const filtered = useMemo(() => requirements.filter((item) =>
    (!filters.status || item.status === filters.status) && (!filters.type || item.requirement_type === filters.type) &&
    (!filters.risk || item.risk === filters.risk) && (!filters.flow || item.flow_type === filters.flow) &&
    (!filters.actor || item.actor.toLowerCase().includes(filters.actor.toLowerCase())) &&
    (!filters.documentId || item.document_ids?.includes(Number(filters.documentId))),
  ), [requirements, filters]);
  const page = Math.min(Math.max(0, filters.page), Math.max(0, Math.ceil(filtered.length / 20) - 1));
  const visible = filtered.slice(page * 20, (page + 1) * 20);
  const values = (field: keyof Requirement) => field === "status" ? ["DRAFT", "APPROVED", "REJECTED", "CONFLICT", "TBD"] : [...new Set(requirements.map((item) => String(item[field] ?? "")).filter(Boolean))].sort();
  const toggle = (item: Requirement) => setSelected((old) => old.some((entry) => entry.id === item.id) ? old.filter((entry) => entry.id !== item.id) : old.length < 100 ? [...old, item] : old);
  const issueCount = conflicts.filter((item) => item.status === "OPEN").length + questions.filter((item) => item.status === "OPEN").length;
  const approved = requirements.filter((item) => item.status === "APPROVED" && !item.review_blockers.length).length;
  const send = async (items: Requirement[], decision: "APPROVED" | "REJECTED", next = false) => {
    if (busy.current) return;
    if (!reviewer.trim()) { setError("Nhập tên hiển thị người duyệt ở phần xác nhận nhóm."); return; }
    busy.current = true; setPending(true); setError("");
    const body = JSON.stringify({ items: items.map((item) => ({ id: item.id, expected_hash: item.review_hash })), decision, reviewer_name: reviewer.trim(), comment: comment.trim() });
    if (command.current?.body !== body) command.current = { body, key: crypto.randomUUID() };
    try {
      const response = await fetch(`${base}/requirement-review/bulk-review`, { method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": command.current.key }, body });
      const payload = await response.json() as { results?: Result[]; error?: string };
      if (!response.ok || !payload.results) throw new Error(payload.error ?? "Chưa lưu được; thử lại cùng yêu cầu không ghi trùng mục đã lưu.");
      command.current = undefined; setResults(payload.results); setConfirmation(undefined);
      const applied = new Set(payload.results.filter((item) => item.status === "APPLIED").map((item) => item.id));
      setSelected((old) => old.filter((item) => !applied.has(item.id)));
      if (next && applied.has(items[0].id)) setOpened(filtered.find((item) => item.id !== items[0].id && item.status === "DRAFT")?.id);
      try { sessionStorage.setItem("source-reviewer-display", reviewer.trim()); } catch { /* optional */ }
      reload();
    } catch (caught) { setError((caught as Error).message); }
    finally { busy.current = false; setPending(false); }
  };
  const resolve = async () => {
    if (!issue || busy.current) return;
    busy.current = true; setPending(true); setError("");
    try {
      const response = await fetch(`${base}/requirement-review/clarification`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...issue, resolution }) });
      if (!response.ok) throw new Error("Chưa lưu được làm rõ. Kiểm tra quyền, nội dung trả lời và trạng thái hiện tại.");
      setIssue(undefined); setResolution(""); reload();
    } catch (caught) { setError((caught as Error).message); }
    finally { busy.current = false; setPending(false); }
  };
  const previewButton = (id: number, label = `#${id}`) => <button type="button" key={id} className="button secondary" onClick={() => { if (!opened) listScroll.current = window.scrollY; setOpened(id); }}>{label}</button>;
  return <div className="requirement-review-workspace">
    <section className="panel"><div className="panel-header"><div><h2>Requirement inventory</h2><p>Xem evidence, chọn đúng revision rồi xác nhận một lần cho nhóm.</p></div><span className="section-counter">{requirements.length}</span></div><div className="panel-body">
      <div className="decision-actions" role="group" aria-label="Nhóm yêu cầu">{[["inventory", "Danh sách"], ["issues", `Cần làm rõ · ${issueCount}`], ["comparison", "Đối chiếu nguồn"]].map(([value, label]) => <button key={value} className="button secondary" aria-pressed={tab === value} onClick={() => setTab(value)}>{label}</button>)}</div>
      <p>{approved} yêu cầu đã duyệt · {issueCount} vấn đề chưa giải quyết · {requirements.length - approved} yêu cầu ngoài phạm vi sinh testcase.</p><Link className="button" href={`/documents/${setId}?step=test-cases`}>Sinh testcase cho {approved} yêu cầu đã duyệt</Link>
      {!canReview ? <p className="notice">Chế độ xem. Cần quyền reviewer để lưu quyết định.</p> : null}
    </div></section>
    {error ? <p role="alert" className="form-error">{error} <button onClick={() => { setError(""); reload(); }}>Tải lại dữ liệu</button></p> : null}
    {results.length ? <section className="notice" aria-label="Kết quả duyệt"><p role="status">Đã lưu {results.filter((item) => item.status === "APPLIED").length}/{results.length} quyết định.</p>{results.filter((item) => item.status !== "APPLIED").map((item) => <p key={item.id}>#{item.id}: {item.message} {previewButton(item.id, "Xem lại evidence")}</p>)}</section> : null}
    <div className={opened ? "source-review-grid" : ""}><div>
    {tab === "inventory" ? <section className="panel"><div className="filter-strip">
      <select aria-label="Document filter" value={filters.documentId} onChange={(event) => setFilters({ ...filters, documentId: event.target.value, page: 0 })}><option value="">Mọi tài liệu</option>{documents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
      {([['status', 'Status filter'], ['type', 'Type filter'], ['risk', 'Risk filter'], ['flow', 'Flow filter']] as const).map(([field, label]) => <select key={field} aria-label={label} value={filters[field]} onChange={(event) => setFilters({ ...filters, [field]: event.target.value, page: 0 })}><option value="">{label}</option>{values(field === 'type' ? 'requirement_type' : field === 'flow' ? 'flow_type' : field).map((value) => <option key={value}>{value}</option>)}</select>)}
      <input aria-label="Actor filter" placeholder="Lọc actor" value={filters.actor} onChange={(event) => setFilters({ ...filters, actor: event.target.value, page: 0 })} />
    </div><div className="panel-body"><p>Hiện {visible.length}/{filtered.length} kết quả lọc. Chọn {selected.length}/100 revision; {selected.filter((item) => !visible.some((row) => row.id === item.id)).length} ngoài trang này.</p>
      <button className="button secondary" disabled={!canReview || pending} onClick={() => setSelected((old) => [...old, ...visible.filter((item) => !old.some((row) => row.id === item.id))].slice(0, 100))}>Chọn các mục trên trang này</button> <button className="button secondary" disabled={pending} onClick={() => setSelected([])}>Bỏ chọn tất cả</button><p className="field-hint">Tối đa 20 mục/trang. Không chọn âm thầm toàn bộ kết quả xuyên trang. Selection giữ nguyên hash khi danh sách cập nhật.</p></div>
      <div className="table-wrap" tabIndex={0} aria-label="Danh sách yêu cầu"><table className="data-table"><thead><tr><th>Chọn</th><th>Yêu cầu / phiên bản</th><th>Trạng thái</th><th>Bằng chứng</th></tr></thead><tbody>{visible.map((item) => <tr key={item.id}>
        <td><input type="checkbox" aria-label={`Chọn ${item.requirement_key}`} checked={selected.some((entry) => entry.id === item.id)} disabled={!canReview || pending} onChange={() => toggle(item)} /></td><td><strong>{item.title}</strong><span className="table-subtitle">{item.requirement_key} · v{item.version_number} · {item.flow_type} · {item.risk}</span><p>{item.statement}</p></td>
        <td><StatusBadge status={item.status} />{item.review_blockers.map((code) => <p key={code}>{blockerText[code] ?? code}</p>)}</td><td>{previewButton(item.id, "Xem evidence & duyệt")}<Link prefetch={false} onClick={rememberScroll} href={`/documents/${setId}/requirements/${item.id}`}>Mở chi tiết / sửa</Link></td>
      </tr>)}</tbody></table></div>{!visible.length ? <p className="panel-body">Không có yêu cầu khớp bộ lọc.</p> : null}
      <div className="panel-body decision-actions"><button className="button secondary" disabled={page === 0} onClick={() => setFilters({ ...filters, page: page - 1 })}>Trang trước</button><span>Trang {page + 1}/{Math.max(1, Math.ceil(filtered.length / 20))}</span><button className="button secondary" disabled={(page + 1) * 20 >= filtered.length} onClick={() => setFilters({ ...filters, page: page + 1 })}>Trang sau</button></div>
    </section> : null}
    {tab === "issues" ? <section className="panel"><div className="panel-body"><h2>Cần làm rõ</h2><p>Trả lời cụ thể đưa yêu cầu về nháp, không tự duyệt.</p>
      {conflicts.map((item) => <article className="clarification-item" key={`c${item.id}`}><h3>Conflict #{item.id} · {item.status}</h3><p>{item.reason}</p><div className="decision-actions">{previewButton(item.left_requirement_id, "Xem nguồn bên trái")}{previewButton(item.right_requirement_id, "Xem nguồn bên phải")}{item.status === "OPEN" && canReview ? <button className="button" onClick={() => { setIssue({ kind: "CONFLICT", id: item.id }); setResolution(""); }}>Giải quyết conflict</button> : null}</div><p>{item.resolution}</p></article>)}
      {questions.map((item) => <article className="clarification-item" key={`q${item.id}`}><h3>Câu hỏi #{item.id} · {item.status}</h3><p>{item.question}</p>{item.requirement_id ? previewButton(item.requirement_id, "Xem yêu cầu và nguồn") : null}{item.status === "OPEN" && canReview ? <button className="button" onClick={() => { setIssue({ kind: "QUESTION", id: item.id }); setResolution(""); }}>Trả lời câu hỏi</button> : null}<p>{item.answer}</p></article>)}
      {!conflicts.length && !questions.length ? <p>Không có vấn đề cần làm rõ.</p> : null}
      {issue ? <div className="connect-grid"><label className="repository-url-field"><span>Nội dung làm rõ {issue.kind} #{issue.id} (ít nhất 10 ký tự)</span><textarea value={resolution} maxLength={8000} onChange={(event) => setResolution(event.target.value)} /></label><button className="button" disabled={pending || resolution.trim().length < 10} onClick={() => void resolve()}>Lưu nội dung làm rõ</button></div> : null}
      <details><summary>Lịch sử quyết định làm rõ · {history.length}</summary>{history.map((item) => <p key={item.id}>{item.kind} #{item.subject_id} · {item.actor} · {new Date(item.created_at).toLocaleString()}<br />{item.resolution}</p>)}</details>
    </div></section> : null}
    {tab === "comparison" ? <section className="panel"><div className="panel-body"><h2>Đối chiếu nguồn cũ / mới</h2><p>Không chuyển quyết định duyệt tự động. Mục mơ hồ cần người đọc đối chiếu; testcase và run cũ giữ nguyên bằng chứng.</p>
      {!comparisons.length ? <p>Chưa có lượt trích xuất hoàn tất để đối chiếu.</p> : null}
      {comparisons.map((comparison, index) => <details key={comparison.id} open={index === 0}><summary>Nguồn #{comparison.from_snapshot_id ?? "—"} → #{comparison.to_snapshot_id} · {comparison.items.length} kết quả</summary>{comparison.items.map((item, i) => <article className="clarification-item" key={i}><h3>{changeText[item.classification]}</h3><p>{item.reason}</p><div>Cũ: {item.before_ids.map((id) => previewButton(id))} → Mới: {item.after_ids.map((id) => previewButton(id))}</div><p>Testcase cần xem lại: {item.affected_test_case_ids.length ? item.affected_test_case_ids.map((id) => <Link key={id} href={`/documents/${setId}/test-cases/${id}`}>#{id} </Link>) : "Không có"}</p></article>)}</details>)}
    </div></section> : null}</div>
    {opened ? <aside className="panel source-preview"><div className="panel-body"><h2 ref={previewHeading} tabIndex={-1}>{detail ? `${detail.requirement.requirement_key} · v${detail.requirement.version_number}` : "Đang tải evidence…"}</h2>{previewError ? <p role="alert">{previewError}</p> : null}
      {detail ? <><p>{detail.requirement.statement}</p><StatusBadge status={detail.requirement.status} /><p>{detail.requirement.source_state === "CURRENT" ? "Nguồn hiện hành" : "Đang xem lịch sử / nguồn ngoài phạm vi"}</p><div className="evidence-drawer">{detail.evidence.map((item) => <article key={item.id}><strong>{item.document_name} · v{item.version_number} · {item.approval_status}</strong><code>{item.source_locator}</code><pre className="source-text">{item.excerpt}</pre></article>)}</div>{!detail.evidence.length ? <p>Chưa có evidence hợp lệ để duyệt.</p> : null}
        {detail.requirement.review_blockers.map((code) => <p className="notice" key={code}>{blockerText[code] ?? code}</p>)}
        {canReview ? <div className="decision-actions"><button className="button secondary" disabled={pending || detail.requirement.source_state !== "CURRENT"} onClick={() => toggle(detail.requirement)}>{selected.some((item) => item.id === opened) ? "Bỏ chọn revision" : "Chọn revision để duyệt nhóm"}</button><button className="button" disabled={pending || detail.requirement.review_blockers.length > 0} onClick={() => void send([detail.requirement], "APPROVED", true)}>Duyệt & xem tiếp</button></div> : null}
        <Link prefetch={false} onClick={rememberScroll} href={`/documents/${setId}/requirements/${opened}`}>Mở chi tiết để sửa thành revision mới</Link><details><summary>Lịch sử duyệt</summary>{detail.reviews.map((review) => <p key={review.id}>{review.reviewer_name} · {review.decision} · {review.comment}</p>)}</details>
      </> : null}<button className="button secondary" onClick={() => { setOpened(undefined); window.requestAnimationFrame(() => window.scrollTo(0, listScroll.current)); }}>Đóng evidence, giữ danh sách</button>
    </div></aside> : null}</div>
    {canReview ? <section className="panel"><div className="panel-body"><h2>Duyệt nhóm đã chọn · {selected.length}</h2><div className="connect-grid"><label><span>Tên hiển thị người duyệt</span><input maxLength={160} value={reviewer} onChange={(event) => setReviewer(event.target.value)} /></label><label><span>Ghi chú chung</span><input maxLength={4000} value={comment} onChange={(event) => setComment(event.target.value)} /></label></div><p>Tài khoản audit do server xác định, tên nhập không nâng quyền.</p>
      <div className="decision-actions"><button className="button secondary" disabled={pending || !selected.length} onClick={() => setConfirmation("REJECTED")}>Từ chối nhóm đã chọn</button><button className="button" disabled={pending || !selected.length} onClick={() => setConfirmation("APPROVED")}>Duyệt nhóm đã chọn</button></div>
      {confirmation ? <section className="notice" role="dialog" aria-label="Xác nhận duyệt nhóm"><h3>Xác nhận {confirmation === "APPROVED" ? "duyệt" : "từ chối"} {selected.length} revision</h3><p>{selected.filter((item) => item.review_blockers.length).length} mục có điều kiện chặn. Server kiểm tra từng mục; thành công được giữ, mục lỗi không được duyệt thay.</p><ul>{selected.map((item) => <li key={item.id}>#{item.id} · {item.title} · v{item.version_number} <button disabled={pending} onClick={() => toggle(item)}>Bỏ chọn</button></li>)}</ul><button className="button" disabled={pending || !selected.length || !reviewer.trim()} onClick={() => void send(selected, confirmation)}>{pending ? "Đang lưu…" : "Xác nhận lưu quyết định"}</button><button className="button secondary" disabled={pending} onClick={() => setConfirmation(undefined)}>Quay lại kiểm tra</button></section> : null}
    </div></section> : null}
  </div>;
}
