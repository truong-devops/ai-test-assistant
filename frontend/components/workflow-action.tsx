"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

export function WorkflowAction({ endpoint, label, pendingLabel, body }: {
  endpoint: string;
  label: string;
  pendingLabel: string;
  body?: Record<string, unknown>;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const run = () => {
    setMessage("");
    setError("");
    startTransition(async () => {
      try {
        const response = await fetch(`/api/backend${endpoint}`, {
          method: "POST",
          headers: { Accept: "application/json", "Content-Type": "application/json" },
          body: JSON.stringify(body ?? {}),
        });
        const payload = (await response.json().catch(() => ({}))) as { error?: string };
        if (!response.ok) throw new Error(payload.error ?? `Request failed (${response.status})`);
        setMessage("Completed successfully.");
        router.refresh();
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Request failed.");
      }
    });
  };
  return (
    <div className="inline-action">
      <button className="button" type="button" onClick={run} disabled={pending}>{pending ? pendingLabel : label}</button>
      {message ? <small className="form-success" role="status">{message}</small> : null}
      {error ? <small className="form-error" role="alert">{error}</small> : null}
    </div>
  );
}
