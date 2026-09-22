"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { SourceDocument } from "@/lib/types";

const documentTypes = [["REQUIREMENTS", "Yêu cầu phần mềm"], ["USER_STORY", "User story"],
  ["SYSTEM_DESIGN", "Thiết kế hệ thống"], ["DATABASE_DESIGN", "Thiết kế dữ liệu"],
  ["API_CONTRACT", "Đặc tả API"], ["BUG_HISTORY", "Lịch sử lỗi"], ["TEST_REFERENCE", "Tài liệu kiểm thử"], ["OTHER", "Khác"]];
type UploadItem = { id: string; file: File; name: string; type: string; progress: number;
  status: "ready" | "uploading" | "success" | "error"; message?: string };

export function UploadDocument({ setId, maxBytes = 16 * 1024 * 1024, target, onUploaded }: {
  setId: number; maxBytes?: number; target?: SourceDocument; onUploaded?: () => void;
}) {
  const router = useRouter();
  const [items, setItems] = useState<UploadItem[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<XMLHttpRequest | undefined>(undefined);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; active.current?.abort(); }; }, []);
  const update = (id: string, patch: Partial<UploadItem>) => setItems((old) => old.map((item) => item.id === id ? { ...item, ...patch } : item));
  const choose = (files: FileList | null) => {
    if (!files || pending) return;
    if (files.length > (target ? 1 : 20)) { setError(target ? "Chọn một file cho phiên bản mới." : "Mỗi đợt chọn tối đa 20 file."); return; }
    setError("");
    setItems(Array.from(files).map((file) => ({ id: crypto.randomUUID(), file,
      name: target?.name ?? file.name.replace(/\.(docx|md|markdown)$/i, ""), type: target?.document_type ?? "REQUIREMENTS",
      progress: 0, status: "ready" })));
  };
  const upload = (item: UploadItem) => new Promise<void>((resolve) => {
    if (!/\.(docx|md|markdown)$/i.test(item.file.name) || item.file.size > maxBytes || !item.name.trim()) {
      update(item.id, { status: "error", message: `Cần file DOCX/Markdown, tên tài liệu và kích thước tối đa ${Math.floor(maxBytes / 1024 / 1024)} MB.` }); resolve(); return;
    }
    const body = new FormData(); body.set("file", item.file); body.set("document_name", item.name.trim());
    body.set("document_type", item.type); if (!target) body.set("new_document", "true");
    const xhr = new XMLHttpRequest(); active.current = xhr;
    update(item.id, { status: "uploading", progress: 0, message: undefined });
    const path = target ? `documents/${target.id}/versions` : "documents";
    xhr.open("POST", `/api/backend/api/document-sets/${setId}/${path}`);
    xhr.setRequestHeader("Accept", "application/json");
    xhr.upload.onprogress = (event) => { if (event.lengthComputable) update(item.id, { progress: Math.round(event.loaded * 100 / event.total) }); };
    xhr.onload = () => {
      let result: { error?: string; version?: { version_number: number } } = {};
      try { result = JSON.parse(xhr.responseText); } catch { /* handled below */ }
      if (xhr.status >= 200 && xhr.status < 300 && result.version) {
        update(item.id, { status: "success", progress: 100, message: `Đã nhận v${result.version.version_number} · đang chờ đọc nội dung` });
        if (onUploaded) onUploaded(); else router.refresh();
      } else update(item.id, { status: "error", message: xhr.status === 409 ? "Tài liệu hoặc nội dung đã tồn tại. Xem danh sách và dùng Tải phiên bản mới nếu cần." : "Chưa tải được file. Kiểm tra định dạng/kích thước rồi thử lại." });
      resolve();
    };
    xhr.onerror = () => { update(item.id, { status: "error", message: "Mất kết nối. Kiểm tra danh sách trước khi thử lại file này." }); resolve(); };
    xhr.onabort = () => resolve(); xhr.send(body);
  });
  const submit = async (selected: UploadItem[]) => {
    if (pending) return; setPending(true);
    try { for (const item of selected) { if (!alive.current) break; await upload(item); } }
    finally { if (alive.current) setPending(false); }
  };
  return <section className="panel" aria-label={target ? "Tải phiên bản mới" : "Tải tài liệu mới"}>
    <div className="panel-header"><div><h2>{target ? `Phiên bản mới · ${target.name}` : "Tải tài liệu mới"}</h2>
      <p>{target ? "File được gắn vào đúng tài liệu này; các phiên bản trước vẫn được giữ." : "Chọn nhiều file DOCX hoặc Markdown. Mỗi file được xử lý và báo kết quả riêng."}</p></div></div>
    <div className="panel-body">
      <label className="document-dropzone" onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); choose(event.dataTransfer.files); }}>
        <span>Chọn file hoặc kéo thả vào đây · tối đa {Math.floor(maxBytes / 1024 / 1024)} MB/file</span>
        <input type="file" multiple={!target} accept=".docx,.md,.markdown" disabled={pending} onChange={(event) => choose(event.target.files)} />
      </label>
      <div className="upload-items">{items.map((item) => <article className="upload-item" key={item.id}>
        <strong>{item.file.name}</strong>
        <label><span>Tên tài liệu</span><input aria-label={`Tên ${item.file.name}`} value={item.name} maxLength={240} disabled={Boolean(target) || pending || item.status === "success"} onChange={(event) => update(item.id, { name: event.target.value })} /></label>
        <label><span>Loại tài liệu</span><select aria-label={`Loại ${item.file.name}`} value={item.type} disabled={Boolean(target) || pending || item.status === "success"} onChange={(event) => update(item.id, { type: event.target.value })}>{documentTypes.map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
        {item.status === "uploading" ? <div role="status"><progress aria-label={`Tiến độ tải ${item.file.name}`} value={item.progress} max={100} /> {item.progress}% {item.progress === 100 ? "· đang lưu file" : ""}</div> : null}
        <p role={item.status === "error" ? "alert" : "status"} className={item.status === "error" ? "form-error" : "table-subtitle"}>{item.message ?? "Sẵn sàng tải lên"}</p>
        {item.status === "error" ? <button type="button" className="button secondary" disabled={pending} onClick={() => void submit([item])}>Thử lại file này</button> : null}
      </article>)}</div>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      <p className="field-hint">Đọc nội dung và chuẩn bị tìm kiếm tự chạy trên server. AI chỉ trích xuất khi bạn yêu cầu. Nguồn mới luôn cần được duyệt.</p>
      <button type="button" className="button" disabled={pending || !items.some((item) => item.status === "ready")} onClick={() => void submit(items.filter((item) => item.status === "ready"))}>{pending ? "Đang tải…" : "Tải tài liệu lên"}</button>
    </div>
  </section>;
}
