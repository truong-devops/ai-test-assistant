import Link from "next/link";
import type { ReactNode } from "react";

type Section = "overview" | "projects" | "analyses" | "evaluations";

const navigation: Array<{ href: string; label: string; section: Section }> = [
  { href: "/", label: "Overview", section: "overview" },
  { href: "/projects", label: "Projects", section: "projects" },
  { href: "/analyses", label: "Review queue", section: "analyses" },
  { href: "/evaluations", label: "Evaluation", section: "evaluations" },
];

export function AppShell({ active, children }: { active: Section; children: ReactNode }) {
  return (
    <div className="app-frame">
      <header className="topbar">
        <Link className="brand" href="/" aria-label="AI Test Assistant home">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
              <path d="M5 3.5h10l4 4v13H5z" />
              <path d="M15 3.5v4h4M8 12h8M8 16h5" />
            </svg>
          </span>
          <span>
            <strong>Test Review</strong>
            <small>AI Test Assistant</small>
          </span>
        </Link>
        <nav aria-label="Primary navigation">
          {navigation.map((item) => (
            <Link key={item.href} href={item.href} className={active === item.section ? "nav-link active" : "nav-link"}>
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="environment-indicator"><span /> Local workspace</div>
      </header>
      <main className="page-shell">{children}</main>
    </div>
  );
}

export function PageHeading({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {actions ? <div className="heading-actions">{actions}</div> : null}
    </div>
  );
}

export function EmptyState({ title, message, action }: { title: string; message: string; action?: ReactNode }) {
  return (
    <section className="empty-state">
      <div className="empty-mark" aria-hidden="true">+</div>
      <div>
        <h2>{title}</h2>
        <p>{message}</p>
        {action ? <div className="empty-action">{action}</div> : null}
      </div>
    </section>
  );
}
