"use client";

import { FormEvent, useState, useTransition } from "react";
import type { SemanticChunk } from "@/lib/types";
import { StatusBadge } from "@/components/status-badge";
import { humanize } from "@/lib/presentation";

export function RetrievalDebug({ setId, generation }: { setId: number; generation?: number }) {
  const [query, setQuery] = useState("UC-B08");
  const [policy, setPolicy] = useState("LATEST");
  const [results, setResults] = useState<SemanticChunk[]>([]);
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError("");
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/document-sets/${setId}/retrieve`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ query, version_policy: policy, limit: 10, index_generation: generation }),
      });
      const payload = (await response.json().catch(() => ({}))) as { results?: SemanticChunk[]; error?: string };
      if (!response.ok) {
        setError(payload.error ?? "Retrieval failed.");
        return;
      }
      setResults(payload.results ?? []);
    });
  };
  return (
    <section className="panel">
      <div className="panel-header"><div><h2>Retrieval debug</h2><p>Reviewer/admin diagnostic view with exact, lexical, vector, and authority scores.</p></div></div>
      <form className="panel-body" onSubmit={submit}>
        <div className="connect-grid">
          <label className="repository-url-field"><span>Golden query or identifier</span><input value={query} onChange={(event) => setQuery(event.target.value)} required /></label>
          <label><span>Version policy</span><select value={policy} onChange={(event) => setPolicy(event.target.value)}><option value="LATEST">Latest</option><option value="LATEST_APPROVED">Latest approved</option><option value="ALL_INDEXED">All indexed</option></select></label>
        </div>
        <div className="connect-actions"><button className="button secondary" disabled={pending}>{pending ? "Searching…" : "Run retrieval"}</button></div>
        {error ? <p className="form-error">{error}</p> : null}
      </form>
      {results.length ? <div className="document-block-list compact-block-list">{results.map((item) => <article className="document-block" key={item.id}>
        <header><span>{item.identifier || humanize(item.chunk_type)} · <StatusBadge status={item.approval_status} /></span><code>{item.source_locator}</code></header>
        <pre>{item.content}</pre>
        <footer className="score-strip">exact {(item.exact_score ?? 0).toFixed(2)} · lexical {(item.lexical_score ?? 0).toFixed(2)} · vector {(item.semantic_score ?? 0).toFixed(2)} · authority {(item.authority_score ?? 0).toFixed(2)} · total {(item.score ?? 0).toFixed(2)}</footer>
      </article>)}</div> : null}
    </section>
  );
}
