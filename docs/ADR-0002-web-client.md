# ADR 0002: Web Client as a Separate, Same-Origin Static App

- Status: Accepted
- Date: 2026-10-24
- Review date: 2027-01-24

## Context

The API needs a browser client for patients, doctors and admins. It is built from the
artboards in the client's `design/` folder: React 18, TypeScript, Vite, TanStack Router and
TanStack Query, rendered entirely in the browser. Every screen sits behind a login, so search
engines have nothing to index. The API sends no CORS headers, so a browser can only call it from
the same origin.

The client is developed in its own repository, `appointment_system_frontend`, next to this one.
It reads the API contract from this repository's DTOs and records anything it needs but the API
lacks in its `API_GAPS.md`. It does not change Go code.

## Decision

Keep the client a separate repository and serve it apart from the Go binary, on the same origin
as the API:

- Development: the Vite dev server (`http://localhost:5173`) proxies `/api` and `/health` to
  `http://localhost:8080`, so the browser only ever talks to one origin.
- Production: `npm run build` produces static files (`dist/`). A web server or reverse proxy
  serves them and forwards `/api` and `/health` to the API, under one origin. Unknown paths fall
  back to `index.html`, because routing happens in the browser.
- No server-side rendering.

Settings in this repository that point at the client use its public URL. Today there are none:
Razorpay Checkout opens over the client's page and returns to it, so nothing redirects back.

## Alternatives

Embed the built client in the Go binary with `embed`. Go can only embed files inside its own
module, so every API build would first have to copy `dist/` from the other repository. That ties
the two release cycles together and makes every client change rebuild the API. Rejected.

Serve the client from its own origin and call the API cross-origin. The API would need CORS and
the client would need to handle cross-site credentials. Rejected while one origin works.

Server-side rendering. It adds a Node runtime to production for pages that are all behind a
login. Rejected.

## Impact

Production needs a reverse proxy (or a static host with rewrites) that routes `/api` and `/health`
to the API and everything else to the client's static files, with the `index.html` fallback. The
two repositories are released separately. An API change that breaks the client's contract must be
coordinated by hand, because nothing compiles them together. The review date is a prompt to check
whether a shared contract (generated types from a complete OpenAPI spec) has become worth adding.
