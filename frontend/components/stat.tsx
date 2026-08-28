import type { ReactNode } from "react";

export function Stat({ label, value, detail, tone = "plain" }: {
  label: string;
  value: ReactNode;
  detail?: string;
  tone?: "plain" | "accent" | "warning";
}) {
  return (
    <section className={`stat-card ${tone}`}>
      <p>{label}</p>
      <strong>{value}</strong>
      {detail ? <small>{detail}</small> : null}
    </section>
  );
}
