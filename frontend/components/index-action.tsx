"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

export function ReindexAction({ projectId }: { projectId: number }) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [pending, startTransition] = useTransition();

  const request = () => {
    setMessage("");
    startTransition(async () => {
      try {
        const response = await fetch(`/api/backend/api/projects/${projectId}/index`, { method: "POST" });
        if (!response.ok) {
          const body = (await response.json().catch(() => ({}))) as { error?: string };
          setMessage(body.error ?? "Could not request indexing.");
          return;
        }
        setMessage("Re-index requested. The worker will refresh this project in the background.");
        router.refresh();
      } catch {
        setMessage("Could not reach the API. Check the local services and try again.");
      }
    });
  };

  return (
    <div className="inline-action">
      <button type="button" className="button secondary" onClick={request} disabled={pending}>{pending ? "Requesting…" : "Re-index project"}</button>
      {message ? <span role="status">{message}</span> : null}
    </div>
  );
}
