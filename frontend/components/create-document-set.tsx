"use client";

import { FormEvent, useState, useTransition } from "react";
import { useRouter } from "next/navigation";

export function CreateDocumentSet() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [productName, setProductName] = useState("");
  const [scope, setScope] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    startTransition(async () => {
      try {
        const response = await fetch("/api/backend/api/document-sets", {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({ name: name.trim(), product_name: productName.trim(), scope: scope.trim(), description: description.trim() }),
        });
        const body = (await response.json().catch(() => ({}))) as { error?: string; id?: number };
        if (!response.ok || !body.id) throw new Error(body.error ?? "Không thể tạo bộ tài liệu. Hãy thử lại.");
        router.push(`/documents/${body.id}?step=documents`);
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Could not create the document set.");
      }
    });
  };

  return (
    <section className="panel connect-project">
      <div className="panel-header">
        <div><h2>Tạo bộ tài liệu</h2><p>Tập hợp các tài liệu mô tả chức năng cần kiểm thử.</p></div>
      </div>
      <form className="panel-body" onSubmit={submit}>
        <div className="connect-grid">
          <label>
            <span>Tên bộ tài liệu</span>
            <input value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required disabled={pending} placeholder="Checkout release 1" />
          </label>
        </div>
        <details className="workflow-details"><summary>Thông tin bổ sung (không bắt buộc)</summary><div className="connect-grid">
          <label>
            <span>Sản phẩm (không bắt buộc)</span>
            <input value={productName} onChange={(event) => setProductName(event.target.value)} maxLength={200} disabled={pending} placeholder="E-commerce platform" />
          </label>
          <label className="repository-url-field">
            <span>Scope <em>optional</em></span>
            <input value={scope} onChange={(event) => setScope(event.target.value)} maxLength={1000} disabled={pending} placeholder="UC-B08 · Place an order" />
          </label>
          <label className="repository-url-field">
            <span>Description <em>optional</em></span>
            <textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={4000} disabled={pending} placeholder="Scope, release, owner, or review note" />
          </label>
        </div></details>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        <div className="connect-actions">
          <button className="button" type="submit" disabled={pending || !name.trim()}>{pending ? "Đang tạo…" : "Tạo và mở bộ tài liệu"}</button>
        </div>
      </form>
    </section>
  );
}
