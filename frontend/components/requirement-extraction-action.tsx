"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import type { RequirementExtractionJob } from "@/lib/types";

type JobResponse = { job?: RequirementExtractionJob; error?: string };

export function RequirementExtractionAction({ setId, initialJob }: {
  setId: string;
  initialJob?: RequirementExtractionJob;
}) {
  const router = useRouter();
  const [job, setJob] = useState(initialJob);
  const [error, setError] = useState("");
  const active = job?.status === "PENDING" || job?.status === "RUNNING";

  const loadStatus = useCallback(async () => {
    const response = await fetch(`/api/backend/api/document-sets/${setId}/requirements/extraction`, {
      cache: "no-store", headers: { Accept: "application/json" },
    });
    const payload = (await response.json().catch(() => ({}))) as JobResponse;
    if (!response.ok) throw new Error(payload.error ?? `Status request failed (${response.status})`);
    if (payload.job) {
      setJob(payload.job);
      if (payload.job.status === "COMPLETED") {
        setError("");
        router.refresh();
      } else if (payload.job.status === "FAILED") {
        setError(payload.job.error_message || "Requirement extraction failed.");
      }
    }
  }, [router, setId]);

  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => void loadStatus().catch((caught) =>
      setError(caught instanceof Error ? caught.message : "Could not load extraction status.")), 2000);
    return () => window.clearInterval(timer);
  }, [active, loadStatus]);

  const start = async () => {
    setError("");
    const response = await fetch(`/api/backend/api/document-sets/${setId}/requirements/extract`, {
      method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ requested_by: "UI" }),
    });
    const payload = (await response.json().catch(() => ({}))) as JobResponse;
    if (!response.ok) {
      setError(payload.error ?? `Request failed (${response.status})`);
      return;
    }
    if (payload.job) setJob(payload.job);
  };

  const percent = job?.total_chunks ? Math.round(job.processed_chunks * 100 / job.total_chunks) : 0;
  return <div className="inline-action">
    <button className="button" type="button" onClick={() => void start()} disabled={active}>
      {active ? "Extracting…" : job?.status === "FAILED" ? "Retry extraction" : "Extract requirements"}
    </button>
    {active ? <small className="table-subtitle" role="status">
      {job.status} · {job.processed_chunks}/{job.total_chunks} chunks ({percent}%)
    </small> : null}
    {job?.status === "COMPLETED" ? <small className="form-success" role="status">
      Completed · {job.created_count} created · {job.reused_count} reused
    </small> : null}
    {error ? <small className="form-error" role="alert">{error}</small> : null}
  </div>;
}
