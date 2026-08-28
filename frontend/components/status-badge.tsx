import { humanize } from "@/lib/presentation";

function statusTone(status: string): string {
  const normalized = status.toUpperCase();
  if (["PASSED", "ACCEPTED", "READY", "ACTIVE"].includes(normalized)) return "positive";
  if (["FAILED", "REJECTED", "TIMED_OUT"].includes(normalized)) return "negative";
  if (["WAITING_REVIEW", "PENDING", "REPAIRING", "VALIDATING", "GENERATING_TESTS", "RECOMMENDING_TESTS"].includes(normalized)) return "attention";
  return "neutral";
}

export function StatusBadge({ status }: { status: string }) {
  return <span className={`status-badge ${statusTone(status)}`}>{humanize(status || "unknown")}</span>;
}
