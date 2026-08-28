import type { GeneratedTest } from "@/lib/types";

export function shortSHA(value: string): string {
  return value ? value.slice(0, 9) : "—";
}

export function formatDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function formatDuration(milliseconds: number): string {
  if (milliseconds < 1000) return `${milliseconds} ms`;
  return `${(milliseconds / 1000).toFixed(milliseconds >= 10_000 ? 0 : 1)} s`;
}

export function humanize(value: string): string {
  return value.replaceAll("_", " ").toLowerCase().replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function latestGeneratedTests(items: GeneratedTest[]): GeneratedTest[] {
  const latest = new Map<number, GeneratedTest>();
  for (const item of items) {
    const current = latest.get(item.recommendation_id);
    if (!current || item.generation_attempt > current.generation_attempt ||
      (item.generation_attempt === current.generation_attempt && item.id > current.id)) {
      latest.set(item.recommendation_id, item);
    }
  }
  return [...latest.values()].sort((left, right) => left.recommendation_id - right.recommendation_id);
}
