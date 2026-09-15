export default function Loading() {
  return (
    <main className="error-shell" aria-busy="true" aria-live="polite">
      <section className="error-panel">
        <p className="eyebrow">Document-driven workflow</p>
        <h1>Loading workspace…</h1>
        <p>The current document, review, coverage, or execution state is being loaded.</p>
      </section>
    </main>
  );
}
