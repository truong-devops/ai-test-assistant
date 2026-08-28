"use client";

export default function GlobalError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <main className="error-shell">
      <section className="error-panel">
        <p className="eyebrow">Review console</p>
        <h1>Evidence could not be loaded.</h1>
        <p>The backend may still be starting, or it returned an unexpected response. No review decision was saved.</p>
        <button type="button" className="button secondary" onClick={() => reset()}>Try again</button>
      </section>
    </main>
  );
}
