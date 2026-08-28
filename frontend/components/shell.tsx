import Link from "next/link";
import type { ReactNode } from "react";

type Section = "overview" | "projects" | "analyses" | "evaluations";

const navigation: Array<{ href: string; label: string; section: Section; icon: string }> = [
  { href: "/", label: "Overview", section: "overview", icon: "overview" },
  { href: "/projects", label: "Projects", section: "projects", icon: "projects" },
  { href: "/analyses", label: "Review queue", section: "analyses", icon: "reviews" },
  { href: "/evaluations", label: "Evaluation", section: "evaluations", icon: "evaluation" },
];

function ProductMark() {
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true">
      <path d="M16 3.5 27 8v8.2c0 6.1-4.5 10.2-11 12.3C9.5 26.4 5 22.3 5 16.2V8z" fill="currentColor" opacity=".16" />
      <path d="m10.2 16.2 3.7 3.7 8-8" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2.4" />
    </svg>
  );
}

function NavIcon({ name }: { name: string }) {
  const common = { "aria-hidden": true, fill: "none", stroke: "currentColor", strokeLinecap: "round" as const, strokeLinejoin: "round" as const, strokeWidth: 1.8 };
  if (name === "search") return <svg viewBox="0 0 24 24" {...common}><circle cx="10.5" cy="10.5" r="5.5" /><path d="m15 15 4.5 4.5" /></svg>;
  if (name === "projects") return <svg viewBox="0 0 24 24" {...common}><path d="M3.5 6.5h6l1.8 2h9.2v10h-17z" /><path d="M3.5 8.5v-3h6" /></svg>;
  if (name === "reviews") return <svg viewBox="0 0 24 24" {...common}><path d="M7 4.5h10a2 2 0 0 1 2 2v12H5v-12a2 2 0 0 1 2-2Z" /><path d="m8.5 11 2.2 2.2 4.8-5M8.5 16h7" /></svg>;
  if (name === "evaluation") return <svg viewBox="0 0 24 24" {...common}><path d="M5 19.5V12h3v7.5zm5.5 0V5h3v14.5zm5.5 0v-11h3v11z" /></svg>;
  return <svg viewBox="0 0 24 24" {...common}><rect x="4" y="4" width="6" height="6" rx="1" /><rect x="14" y="4" width="6" height="6" rx="1" /><rect x="4" y="14" width="6" height="6" rx="1" /><rect x="14" y="14" width="6" height="6" rx="1" /></svg>;
}

export function AppShell({ active, children }: { active: Section; children: ReactNode }) {
  const current = navigation.find((item) => item.section === active) ?? navigation[0];
  return (
    <div className="app-frame">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="app-sidebar">
        <div className="sidebar-header">
          <Link className="brand" href="/" aria-label="AI Test Assistant home">
            <span className="brand-mark"><ProductMark /></span>
            <span className="brand-copy">
              <strong>Test Assistant</strong>
              <small>Engineering workspace</small>
            </span>
          </Link>
        </div>
        <p className="nav-label">Workspace</p>
        <nav className="sidebar-nav" aria-label="Primary navigation">
          {navigation.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={active === item.section ? "nav-link active" : "nav-link"}
              aria-current={active === item.section ? "page" : undefined}
            >
              <NavIcon name={item.icon} />
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div className="environment-indicator"><span /> Local environment</div>
          <small>AI Test Assistant · v0.1</small>
        </div>
      </aside>
      <div className="app-workspace">
        <header className="topbar">
          <div className="workspace-path">
            <span>AI Test Assistant</span>
            <i>/</i>
            <strong>{current.label}</strong>
          </div>
          <div className="topbar-meta">
            <span className="topbar-status"><i /> Local workspace</span>
            <span className="avatar" aria-label="Local reviewer">LR</span>
          </div>
        </header>
        <main className="page-shell" id="main-content">{children}</main>
      </div>
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
