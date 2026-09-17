"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { AIBudgetStatus, DocumentSet } from "@/lib/types";

type PurgePreview = {
  eligible: boolean;
  blockers: string[];
  purge_eligible_at?: string;
  storage_object_count: number;
  storage_bytes: number;
  confirmation: string;
};

export function DocumentLifecycle({ set, budget }: { set: DocumentSet; budget: AIBudgetStatus }) {
  const router = useRouter();
  const [retention, setRetention] = useState(String(set.retention_days || 365));
  const [tokenBudget, setTokenBudget] = useState(String(set.ai_token_budget || 1_000_000));
  const [costBudgetUSD, setCostBudgetUSD] = useState(String((set.ai_cost_budget_microusd || 0) / 1_000_000));
  const [actor, setActor] = useState("");
  const [reason, setReason] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [preview, setPreview] = useState<PurgePreview>();
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();

  const save = (status: "ACTIVE" | "ARCHIVED") => {
    const days = Number(retention);
    const tokens = Number(tokenBudget);
    const cost = Math.round(Number(costBudgetUSD) * 1_000_000);
    if (!actor.trim() || !reason.trim() || !Number.isInteger(days) || days < 30 || days > 3650 ||
      !Number.isSafeInteger(tokens) || tokens < 1000 || tokens > 1_000_000_000 ||
      !Number.isSafeInteger(cost) || cost < 1) {
      setError("Actor, reason, retention (30–3650 days), token budget, and a positive USD budget are required.");
      return;
    }
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/document-sets/${set.id}/lifecycle`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status, retention_days: days, ai_token_budget: tokens,
          ai_cost_budget_microusd: cost, actor: actor.trim(), reason: reason.trim() }),
      });
      const body = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) setError(body.error ?? "Could not update lifecycle policy.");
      else { setError(""); router.refresh(); }
    });
  };

  const inspectPurge = () => startTransition(async () => {
    const response = await fetch(`/api/backend/api/document-sets/${set.id}/purge`);
    const body = (await response.json().catch(() => ({}))) as PurgePreview & { error?: string };
    if (!response.ok) setError(body.error ?? "Could not inspect purge eligibility.");
    else { setPreview(body); setConfirmation(""); setError(""); }
  });

  const purge = () => {
    if (!preview?.eligible || confirmation !== preview.confirmation || !actor.trim() || !reason.trim()) {
      setError("Purge requires an eligible archived set, actor, reason, and the exact confirmation text.");
      return;
    }
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/document-sets/${set.id}/purge`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ actor: actor.trim(), reason: reason.trim(), confirmation }),
      });
      const body = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) setError(body.error ?? "Could not purge document set.");
      else router.push("/documents");
    });
  };

  return <details className="panel side-section">
    <summary>Retention, AI budget, and deletion policy</summary>
    <div className="stack" style={{ marginTop: 12 }}>
      <label><span>Retention (days)</span><input type="number" min={30} max={3650} value={retention} onChange={(event) => setRetention(event.target.value)} /></label>
      <label><span>Total AI token budget</span><input type="number" min={1000} max={1_000_000_000} value={tokenBudget} onChange={(event) => setTokenBudget(event.target.value)} /></label>
      <label><span>Total AI cost budget (USD)</span><input type="number" min={0.000001} step="0.000001" value={costBudgetUSD} onChange={(event) => setCostBudgetUSD(event.target.value)} /></label>
      <p className="table-subtitle">Used: {budget.used_tokens.toLocaleString()} tokens; reserved: {budget.reserved_tokens.toLocaleString()}; remaining: {budget.remaining_tokens.toLocaleString()}. Cost used: ${(budget.used_cost_microusd / 1_000_000).toFixed(4)}.</p>
      <label><span>Actor</span><input value={actor} onChange={(event) => setActor(event.target.value)} /></label>
      <label><span>Audit reason</span><input value={reason} onChange={(event) => setReason(event.target.value)} /></label>
      {set.status !== "PURGING" ? <button className="button secondary" disabled={pending} onClick={() => save(set.status === "ACTIVE" ? "ARCHIVED" : "ACTIVE")}>{set.status === "ACTIVE" ? "Archive set" : "Restore set"}</button> : null}
      {set.status !== "PURGING" ? <button className="button secondary" disabled={pending} onClick={() => save(set.status === "ARCHIVED" ? "ARCHIVED" : "ACTIVE")}>{set.status === "ACTIVE" ? "Save retention and budget" : "Save policy"}</button> : null}
      {set.status !== "ACTIVE" ? <button className="button secondary" disabled={pending} onClick={inspectPurge}>Check permanent deletion</button> : null}
      {preview ? <div className="notice"><p>{preview.eligible ? `Eligible: ${preview.storage_object_count} stored object(s), ${preview.storage_bytes} bytes will be deleted.` : `Not eligible${preview.purge_eligible_at ? ` until ${new Date(preview.purge_eligible_at).toLocaleString()}` : ""}.`}</p>{preview.blockers.map((blocker) => <p key={blocker}>{blocker}</p>)}<p className="mono">{preview.confirmation}</p><label><span>Type exact confirmation</span><input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label><button className="button secondary" disabled={pending || !preview.eligible || confirmation !== preview.confirmation} onClick={purge}>Permanently purge</button></div> : null}
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      <p className="table-subtitle">Archive is recoverable. Permanent purge is admin-only, waits for retention, refuses pinned baselines, analysis snapshots and test runs, and keeps a durable audit record.</p>
    </div>
  </details>;
}
