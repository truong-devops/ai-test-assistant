# AI Test Assistant Review Console

The frontend is a Next.js application for human review and Phase 10 experiment reporting. It presents
project/index status, analysis history, and a traceable review screen with the
MR diff, changed symbols, current project context, recommendation rationale,
generated test source, every validation result, repair history, and an
immutable Accept/Reject decision.
The Projects page also provides a responsive GitLab/GitHub connection form.
Pasting a repository URL is enough for the backend to resolve its name, numeric
provider ID, and default branch; those values remain available as manual
overrides for private or restricted repositories.
The evaluation area displays immutable dataset hashes, explicit metric
denominators, paired comparison deltas, and the interpretation guardrail that
execution success is distinct from human usefulness.

The visual system follows a restrained engineering-console layout: persistent
workspace navigation, system typography, neutral surfaces, compact tables,
semantic status colors and responsive layouts. It intentionally avoids remote
font dependencies, decorative gradients and effects that compete with review
evidence.

## Local development

Use Node.js 18.18 or newer:

```bash
npm ci --ignore-scripts
npm run typecheck
npm run dev
```

The development server listens on `http://localhost:3000` and reads the Go API
from `BACKEND_API_URL` (default: `http://localhost:8080`). Browser actions
go through the app's same-origin `/api/backend` proxy, so no CORS setup is
required.

For the complete stack, run `make dev-up` from the repository root. Build the
production bundle with `npm run build`.

The main supported layouts are desktop, compact laptop/tablet and narrow mobile.
Tables and source evidence keep horizontal scrolling inside their own container
instead of widening the application shell.
