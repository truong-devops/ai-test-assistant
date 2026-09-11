"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import type { Requirement, SourceDocument } from "@/lib/types";
import { StatusBadge } from "@/components/status-badge";
import { humanize } from "@/lib/presentation";

export function RequirementInventory({ setId, requirements, documents }: { setId: number; requirements: Requirement[]; documents: SourceDocument[] }) {
  const [status, setStatus] = useState("");
  const [type, setType] = useState("");
  const [risk, setRisk] = useState("");
  const [flow, setFlow] = useState("");
  const [actor, setActor] = useState("");
  const [documentId, setDocumentId] = useState("");
  const filtered = useMemo(() => requirements.filter((item) =>
    (!status || item.status === status) && (!type || item.requirement_type === type) &&
    (!risk || item.risk === risk) && (!flow || item.flow_type === flow) &&
    (!actor || item.actor.toLowerCase().includes(actor.toLowerCase())) &&
    (!documentId || item.document_ids?.includes(Number(documentId))),
  ), [requirements, status, type, risk, flow, actor, documentId]);
  const values = (field: keyof Requirement) => [...new Set(requirements.map((item) => String(item[field] ?? "")).filter(Boolean))].sort();
  return (
    <section className="panel">
      <div className="panel-header"><div><h2>Requirement inventory</h2><p>Counts come from stored inventory, never from an AI self-assessment.</p></div><span className="section-counter">{filtered.length}/{requirements.length}</span></div>
      <div className="filter-strip">
        <select aria-label="Document filter" value={documentId} onChange={(event) => setDocumentId(event.target.value)}><option value="">All documents</option>{documents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <select aria-label="Status filter" value={status} onChange={(event) => setStatus(event.target.value)}><option value="">All statuses</option>{values("status").map((value) => <option key={value}>{value}</option>)}</select>
        <select aria-label="Type filter" value={type} onChange={(event) => setType(event.target.value)}><option value="">All types</option>{values("requirement_type").map((value) => <option key={value}>{value}</option>)}</select>
        <select aria-label="Risk filter" value={risk} onChange={(event) => setRisk(event.target.value)}><option value="">All risks</option>{values("risk").map((value) => <option key={value}>{value}</option>)}</select>
        <select aria-label="Flow filter" value={flow} onChange={(event) => setFlow(event.target.value)}><option value="">All flows</option>{values("flow_type").map((value) => <option key={value}>{value}</option>)}</select>
        <input aria-label="Actor filter" placeholder="Filter actor" value={actor} onChange={(event) => setActor(event.target.value)} />
      </div>
      <div className="table-wrap"><table className="data-table"><thead><tr><th>ID</th><th>Requirement</th><th>Type / flow</th><th>Actor</th><th>Risk</th><th>Status</th></tr></thead><tbody>{filtered.map((item) => <tr key={item.id}>
        <td className="mono">{item.requirement_key}<span className="table-subtitle">v{item.version_number}</span></td>
        <td><Link href={`/documents/${setId}/requirements/${item.id}`}><span className="table-title">{item.title}</span><span className="table-subtitle chunk-copy">{item.statement}</span></Link></td>
        <td>{humanize(item.requirement_type)}<span className="table-subtitle">{humanize(item.flow_type)}</span></td>
        <td>{item.actor || "—"}</td><td><StatusBadge status={item.risk} /></td><td><StatusBadge status={item.status} /></td>
      </tr>)}</tbody></table></div>
    </section>
  );
}
