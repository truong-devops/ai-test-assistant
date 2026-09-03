"use client";

import { FormEvent, useState, useTransition } from "react";
import { useRouter } from "next/navigation";

type Provider = "gitlab" | "github";

function providerFromURL(value: string): Provider | null {
  try {
    const hostname = new URL(value).hostname.toLowerCase();
    if (hostname === "github.com") return "github";
    if (hostname === "gitlab.com" || hostname.includes("gitlab")) return "gitlab";
  } catch {
    // The user may still be typing a URL.
  }
  return null;
}

export function ConnectProject() {
  const router = useRouter();
  const [provider, setProvider] = useState<Provider>("gitlab");
  const [repositoryURL, setRepositoryURL] = useState("");
  const [name, setName] = useState("");
  const [projectID, setProjectID] = useState("");
  const [defaultBranch, setDefaultBranch] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [pending, startTransition] = useTransition();

  const updateRepositoryURL = (value: string) => {
    setRepositoryURL(value);
    const detected = providerFromURL(value);
    if (detected) setProvider(detected);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setSuccess("");
    startTransition(async () => {
      const parsedProjectID = projectID.trim() ? Number(projectID) : undefined;
      if (parsedProjectID !== undefined && (!Number.isSafeInteger(parsedProjectID) || parsedProjectID <= 0)) {
        setError("Provider project ID must be a positive integer.");
        return;
      }
      try {
        const response = await fetch("/api/backend/api/projects", {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({
            provider,
            repository_url: repositoryURL.trim(),
            ...(name.trim() ? { name: name.trim() } : {}),
            ...(parsedProjectID ? { provider_project_id: parsedProjectID } : {}),
            ...(defaultBranch.trim() ? { default_branch: defaultBranch.trim() } : {}),
            language: "go",
          }),
        });
        const body = (await response.json().catch(() => ({}))) as { error?: string; name?: string };
        if (!response.ok) {
          throw new Error(body.error ?? "Could not connect the repository.");
        }
        setSuccess(`${body.name || name.trim() || "Repository"} connected successfully.`);
        setRepositoryURL("");
        setName("");
        setProjectID("");
        setDefaultBranch("");
        router.refresh();
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Could not connect the repository.");
      }
    });
  };

  return (
    <section className="panel connect-project">
      <div className="panel-header">
        <div><h2>Connect source repository</h2><p>Paste a GitLab or GitHub repository URL. Public repository metadata is detected automatically.</p></div>
      </div>
      <form className="panel-body" onSubmit={submit}>
        <div className="connect-grid">
          <label>
            <span>Provider</span>
            <select value={provider} onChange={(event) => setProvider(event.target.value as Provider)} disabled={pending}>
              <option value="gitlab">GitLab</option>
              <option value="github">GitHub</option>
            </select>
          </label>
          <label className="repository-url-field">
            <span>Repository URL</span>
            <input
              type="url"
              value={repositoryURL}
              onChange={(event) => updateRepositoryURL(event.target.value)}
              placeholder={provider === "github" ? "https://github.com/owner/repository" : "https://gitlab.com/group/repository"}
              autoComplete="url"
              required
              disabled={pending}
            />
          </label>
          <label>
            <span>Project name <em>optional</em></span>
            <input value={name} onChange={(event) => setName(event.target.value)} maxLength={255} placeholder="Detected from repository" disabled={pending} />
          </label>
          <label>
            <span>Default branch <em>optional</em></span>
            <input value={defaultBranch} onChange={(event) => setDefaultBranch(event.target.value)} maxLength={255} placeholder="Detected from repository" disabled={pending} />
          </label>
          <label>
            <span>{provider === "github" ? "GitHub repository ID" : "GitLab project ID"} <em>optional</em></span>
            <input type="number" min="1" step="1" inputMode="numeric" value={projectID} onChange={(event) => setProjectID(event.target.value)} placeholder="Detected from repository" disabled={pending} />
          </label>
        </div>
        <p className="field-hint">For private repositories, configure the matching server token. You can enter name, branch, and numeric ID manually if metadata lookup is unavailable.</p>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        {success ? <p className="form-success" role="status">{success}</p> : null}
        <div className="connect-actions">
          <button className="button" type="submit" disabled={pending || !repositoryURL.trim()}>{pending ? "Connecting…" : "Connect repository"}</button>
        </div>
      </form>
    </section>
  );
}
