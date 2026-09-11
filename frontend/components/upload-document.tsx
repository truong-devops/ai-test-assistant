"use client";

import { DragEvent, FormEvent, useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";

const documentTypes = [
  ["REQUIREMENTS", "Software requirements"],
  ["USER_STORY", "User stories / acceptance criteria"],
  ["SYSTEM_DESIGN", "System design"],
  ["DATABASE_DESIGN", "Database design"],
  ["API_CONTRACT", "API contract"],
  ["BUG_HISTORY", "Historical defects"],
  ["TEST_REFERENCE", "Test reference"],
  ["OTHER", "Other product document"],
] as const;

export function UploadDocument({ setId, maxBytes = 16 * 1024 * 1024 }: { setId: number; maxBytes?: number }) {
  const router = useRouter();
  const formRef = useRef<HTMLFormElement>(null);
  const [name, setName] = useState("");
  const [type, setType] = useState("REQUIREMENTS");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [pending, startTransition] = useTransition();

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setSuccess("");
    if (!file) {
      setError("Choose a DOCX or Markdown file.");
      return;
    }
    if (!/\.(docx|md|markdown)$/i.test(file.name)) {
      setError("Only .docx, .md, and .markdown files are supported in Phase 2.");
      return;
    }
    if (file.size > maxBytes) {
      setError(`The file exceeds the ${Math.floor(maxBytes / 1024 / 1024)} MB limit.`);
      return;
    }
    startTransition(async () => {
      const form = new FormData();
      form.set("file", file);
      form.set("document_type", type);
      if (name.trim()) form.set("document_name", name.trim());
      try {
        const response = await fetch(`/api/backend/api/document-sets/${setId}/documents`, {
          method: "POST",
          headers: { Accept: "application/json" },
          body: form,
        });
        const body = (await response.json().catch(() => ({}))) as { error?: string; version?: { version_number?: number } };
        if (!response.ok) throw new Error(body.error ?? "Could not upload the document.");
        setSuccess(`Version ${body.version?.version_number ?? "new"} was queued for parsing.`);
        setName("");
        setFile(null);
        formRef.current?.reset();
        router.refresh();
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Could not upload the document.");
      }
    });
  };

  const dropFile = (event: DragEvent<HTMLLabelElement>) => {
    event.preventDefault();
    setDragActive(false);
    setFile(event.dataTransfer.files?.[0] ?? null);
  };

  return (
    <section className="panel connect-project">
      <div className="panel-header">
        <div><h2>Upload source document</h2><p>The original file is retained as an immutable version and parsed asynchronously.</p></div>
      </div>
      <form ref={formRef} className="panel-body" onSubmit={submit}>
        <div className="connect-grid">
          <label>
            <span>Document type</span>
            <select value={type} onChange={(event) => setType(event.target.value)} disabled={pending}>
              {documentTypes.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
          <label>
            <span>Display name <em>optional</em></span>
            <input value={name} onChange={(event) => setName(event.target.value)} maxLength={240} disabled={pending} placeholder="Derived from filename" />
          </label>
          <label
            className={`repository-url-field document-dropzone${dragActive ? " active" : ""}`}
            onDragEnter={() => setDragActive(true)}
            onDragLeave={() => setDragActive(false)}
            onDragOver={(event) => event.preventDefault()}
            onDrop={dropFile}
          >
            <span>DOCX or Markdown file</span>
            <input type="file" accept=".docx,.md,.markdown,text/markdown,application/vnd.openxmlformats-officedocument.wordprocessingml.document" disabled={pending} onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
            <small>{file ? `${file.name} · ${file.size.toLocaleString("en-US")} bytes` : "Choose a file or drop it here"}</small>
          </label>
        </div>
        <p className="field-hint">Every upload starts as DRAFT. Source approval and test-case generation are introduced in later phases.</p>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        {success ? <p className="form-success" role="status">{success}</p> : null}
        <div className="connect-actions">
          <button className="button" type="submit" disabled={pending || !file}>{pending ? "Uploading…" : "Upload document"}</button>
        </div>
      </form>
    </section>
  );
}
