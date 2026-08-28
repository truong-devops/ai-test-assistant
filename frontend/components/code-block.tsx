export function CodeBlock({
  title,
  code,
  label = "GO",
  compact = false,
}: {
  title: string;
  code: string;
  label?: string;
  compact?: boolean;
}) {
  const lines = code.split("\n");
  return (
    <section className={compact ? "code-block compact" : "code-block"}>
      <header>
        <span>{title}</span>
        <small>{label}</small>
      </header>
      <pre aria-label={title}>
        <code>
          {lines.map((line, index) => (
            <span className="code-line" key={`${index}:${line}`}>
              <i aria-hidden="true">{index + 1}</i>
              <b>{line || " "}</b>
            </span>
          ))}
        </code>
      </pre>
    </section>
  );
}
