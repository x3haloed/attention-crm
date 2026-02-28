# Architecture (CRM “Skateboard”)

This document turns `mvp-def.md` into a concrete build plan for an **open-core**, **single-binary** CRM.

## Product + packaging stance

- **Community Edition (CE, open-source):** fully usable meaning-first CRM (contacts + interactions + follow-ups + search) for real work.
- **Commercial (Pro/Enterprise):** additive features for management, scale, compliance, and premium integrations.
- **Feature placement:** follow `docs/open-core-feature-policy.md`.

## Top-level goals (MVP)

- Universal input: create contact, log interaction, or find a contact quickly.
- Multi-user from day 1: **workspace/org** exists on day 1 (no roles/RBAC in MVP).
- “Needs Attention”: due follow-ups surface and can be marked complete.
- Simple deployment: **one downloadable binary** that runs the whole app (API + UI) with SQLite (WAL).

## Trust primitives (ledger-first)

This product’s differentiator is trust: **legibility + reversibility + witness-paced commits**.

We therefore adopt an append-only mutual ledger (human + agent) as a core primitive, with rebuildable
projections and event-shaped undo/redo.

See `docs/mutual-ledger.md`.

## Deployment modes

### Self-host (primary developer/test loop)

- Download `attention` binary.
- Run `attention serve --data-dir …` (or env vars).
- Uses local SQLite file with WAL enabled.
- Optional SMTP config enables magic-link fallback + notifications.

### “Docker path” (optional)

- Provide a `Dockerfile` that runs the same released binary.
- Docker is not required to operate the product.

### Hosted SaaS (later)

- Same binary, same SQLite schema.
- Add an optional SQLite scaling layer behind a well-defined DB interface (examples: libSQL + Turso, LiteFS, dqlite).
- CE remains functional without any scaling layer.

## Technology choices (concrete, proposed)

**Backend:** Go (single static-ish binary, good WebAuthn ecosystem, simple ops)  
**DB:** SQLite with WAL; FTS5 for search  
**Frontend:** server-rendered HTML + HTMX-style interactions (minimal JS), embedded into the Go binary  
**Migrations:** SQL migrations applied at startup  
**Auth:** passkeys (WebAuthn) primary, with email magic-link fallback; local accounts supported; social login optional and additive

Rationale: this maximizes “download and run” simplicity while still supporting a modern auth posture.

## Backend language: why Go (given you don’t know Go yet)

The choice here is less about “best language” and more about matching your product constraints:

- **Single-binary distribution** (you explicitly want this).
- **SQLite-first** (including SaaS with many SQLite files).
- **Modern auth** (WebAuthn/passkeys) with good server libraries.
- **Boring ops** (few moving parts; easy to run locally and self-host).

Why Go fits:

- **True one-binary story:** cross-platform builds are first-class; embedding templates/assets is straightforward.
- **Fast, predictable server:** good latency characteristics without tuning; easy concurrency model for “many requests, many tenants”.
- **Small surface area:** language is intentionally simple; onboarding is usually faster than Rust and less “frameworky” than many JS stacks.
- **Great standard library:** `net/http`, `html/template`, crypto primitives, embedding, testing.

Downsides (real, given your context):

- You’ll be learning a new language while building product.
- Fewer “batteries included” web frameworks than e.g. Rails/Laravel; you either pick one or stay close to stdlib.

Mitigations so you’re not over-dependent on me:

- Keep the architecture **stdlib-first** (thin router, explicit handlers, plain SQL).
- Strong local dev ergonomics (`make dev`, `make test`, `make lint`), plus a small number of core patterns repeated everywhere.
- Extensive docstrings are unnecessary; instead: small packages, narrow interfaces, table-driven tests, and a “how to add a feature” doc later.

Alternatives (and why they’re less aligned with the constraints):

- **TypeScript/Node:** fastest for many founders, but “single binary” and SQLite concurrency/ops are more awkward; deployment tends to pull in Node runtime + packaging quirks.
- **Deno:** can compile to a single executable, but the ecosystem for server-side WebAuthn + mature SQLite patterns is less conventional.
- **Python:** excellent speed of development; packaging into a robust single artifact is possible but typically heavier and more fragile than Go.
- **Rust:** strong single-binary story, but learning curve is steeper than Go (especially while iterating product UX).

If you strongly prefer shipping in TypeScript, I can reframe the doc that way—but given your stated constraints, Go is the most “boring correct” fit.

## SaaS tenancy model (lock-in)

For hosted SaaS, **each customer/org gets their own SQLite database file** (one-tenant-per-DB). This is
the default assumption for all SaaS architecture decisions.

Why:

- Strong isolation: simpler threat modeling and lower blast radius.
- Operational ergonomics: per-tenant backup/restore, export, and deletes are straightforward.
- Performance: avoids noisy-neighbor query patterns inside a shared schema.

Implications:

- The app becomes a **multi-DB router**: every request must resolve a tenant and use the correct DB handle.
- There is a small **control-plane** data store that maps `tenant_slug` / hostname -> DB location + config.
  - This control-plane contains no CRM data; it’s routing + minimal metadata.
- Migrations run **per-tenant**.

## Tenant routing + DB management (clean design)

To keep tenant-per-DB clean, separate the system into:

### 1) Control plane (routing metadata)

A tiny “control” SQLite DB (or config file in single-tenant mode) that stores:

- `tenant_slug` (and optionally a primary hostname / custom domains)
- absolute DB file path (or connection string for a managed SQLite layer later)
- tenant state (active/suspended), created timestamps

No CRM objects live here.

### 2) Data plane (per-tenant SQLite DB)

Each tenant DB contains the full app schema (users, contacts, interactions, etc.).

Server request flow:

1. Resolve tenant from `Host` (subdomain/custom domain) or from the first path segment.
2. Acquire a tenant DB handle from a `TenantDBManager` (connection cache with LRU + idle close).
3. Ensure tenant DB is migrated to latest schema (at open or lazily on first request).
4. Serve request using only that tenant DB.

This makes “delete tenant”, “export tenant”, “restore tenant” and “audit tenant” mechanically simple.

### Tenant hint strategy (resolved for v1)

- Use path-based tenant hint: `/t/{slug}`.
- Session stores `active_tenant_slug` and `user_id`; authenticated requests prefer session tenant.
- `/t/{slug}` remains the canonical entry for login, invite redemption, and bootstrap flows.
- Subdomain/custom-domain routing can be added later as higher-precedence tenant resolution in the control plane.

### SQLite operational defaults (per tenant)

On every tenant DB open, apply pragmatic defaults:

- `PRAGMA journal_mode=WAL;`
- `PRAGMA synchronous=NORMAL;` (or `FULL` if you prefer max durability over throughput)
- `PRAGMA foreign_keys=ON;`
- `PRAGMA busy_timeout=5000;` (tune later)

### Migrations (per tenant)

- Each tenant DB has a `schema_migrations` table.
- Migrations are idempotent, forward-only, and run:
  - at tenant DB open (simple), or
  - in a background “migrate tenants” job for SaaS (faster deploy cutovers).

### Backups / export (per tenant)

- Self-host: backup is “copy the DB file” while the app is stopped, or use SQLite online backup API.
- SaaS: do per-tenant snapshots + optional WAL archiving; restore is just swapping a tenant DB file.

## Data model (MVP, workspace-first)

In self-host / single-tenant mode, the SQLite file contains the whole schema.

In SaaS (one tenant per DB), treat **tenant == workspace** in v1:

- Each tenant DB has exactly one row in `workspaces` (or the workspace table can be removed later).
- Future expansion (multiple workspaces per tenant) remains possible without changing the tenancy router.

## Codebase layout (proposed)

```
cmd/attention/            # main entry (serve, migrate, user admin)
internal/http/            # routing, handlers, templates, htmx endpoints
internal/auth/            # sessions, passkeys, magic links, oauth (optional)
internal/core/            # domain services (contacts, interactions, follow-ups)
internal/store/           # SQLite access, migrations, queries
internal/search/          # FTS5 index maintenance + query helpers
internal/jobs/            # reminders, cleanup, email dispatch (timer-based)
web/                      # templates + static assets (embedded)
migrations/               # .sql migrations
docs/                     # product + architecture docs
```

Commercial code does **not** live in this repo. It integrates via:

- a private module that provides additional implementations behind narrow interfaces (preferred), or
- separate service(s) for Pro-only capabilities (later), keeping CE core unchanged.

All user data is scoped to a workspace (which maps 1:1 to a tenant DB in SaaS v1).

**workspaces**
- `id`, `name`, `created_at`

**users**
- `id`, `email` (nullable if local-only), `name`, `created_at`, `last_login_at`

**memberships**
- `workspace_id`, `user_id`, `created_at`
- (no roles in MVP)

**contacts**
- `id`, `workspace_id`, `name` (required), `email`, `phone`, `company`, `notes`, `created_at`, `updated_at`
- Unique constraints: *avoid hard uniqueness* on name; prefer “duplicate suggestions” at UX layer.

**interactions**
- `id`, `workspace_id`, `contact_id`, `type`, `content`, `due_at` (nullable), `completed_at` (nullable), `created_at`

**deals** (optional)
- `id`, `workspace_id`, `contact_id`, `title`, `value`, `status`, `close_date`

**Audit / events (CE baseline)**
- Minimal `events` table (append-only) for important actions (login, invite accepted, contact created, interaction completed).
- CE keeps this lightweight; “advanced audit” is paid.

### Search indexing (SQLite FTS5)

- Maintain FTS5 virtual tables for contacts and interactions (name/email/company/content).
- Use triggers or application-level updates to keep FTS in sync.
- Query via `bm25()` ranking; add prefix matching for “universal input” feel.

## Universal input (MVP behavior)

Implementation approach (non-LLM, deterministic):

1. If input resembles an email/phone -> search by those fields.
2. Else run FTS on contacts and interactions; show best matches.
3. If input contains a recognizable date phrase -> offer “create interaction with follow-up due_at”.
4. Else if input looks like a name -> offer “create contact”.
5. Else default to “create note interaction” attached to a selected/new contact.

This should be built as a small parsing module with test vectors (so later an LLM-enhanced path can be added safely).

## Authentication (from research doc)

Default posture:

- **Passkeys/WebAuthn:** primary login and account upgrade path.
- **Magic-link email:** optional fallback for users/devices without passkeys; rate-limited and treated as lower assurance.
- **Local accounts:** always available for self-host admins; can be secured with passkeys and/or TOTP later.
- **Social login (OIDC):** optional (Google/GitHub) as additive convenience; not required for MVP.
- **Enterprise SSO (SAML/OIDC to corporate IdPs):** **Enterprise** tier when needed.

Session model:

- Secure, HTTP-only session cookie.
- CSRF protection for state-changing requests (HTMX-compatible).
- Basic login event logging in CE; advanced reporting/retention in paid tiers.

## Background jobs (MVP)

Run in-process (single binary):

- Due follow-up reminders (email optional; UI always shows “Needs Attention”).
- Magic-link token cleanup and session cleanup.

No external queue required for MVP; later, SaaS can offload via a job runner without changing CE semantics.

## Self-host bootstrap (required)

On first run (no users exist), the server exposes a **Setup** flow:

- Create workspace (name + slug) and the initial user.
- The initial user must enroll at least one **passkey** during setup.
- Email is optional at setup time; SMTP is optional.

This avoids shipping a default username/password and forces secure first-use configuration.

## Email without SMTP (creative defaults)

SMTP configuration is optional. The app should remain fully usable without email.

Defaults (CE):

- Passkeys-first login means email is not required to access the app.
- Workspace “invites” can be implemented as **copyable invite links** (and optionally QR codes) that
  the admin shares out-of-band.
- Follow-up reminders are **in-app** by default; email reminders require configuration.

If/when hosted SaaS exists, it can ship with a reliable email provider without changing CE behavior.

## Open-core boundaries (concrete examples)

**CE includes**
- Everything required to run the CRM for a small team: workspaces, invites, contacts, interactions, follow-ups, search, passkeys, magic-link fallback, local accounts.
- Import/export basics (CSV) if needed to unblock adoption.

**Pro/Enterprise later**
- Advanced analytics dashboards and reporting
- Multi-step automation / sequences
- SSO (SAML), SCIM, RBAC/roles
- Advanced audit log UI/export/retention policies
- Premium integrations (accounting/marketing suites)

## Security + privacy baseline (CE)

- Passwordless-first (passkeys) + safe fallback.
- Rate limiting primitives on auth endpoints.
- Token hashing for magic links; short expirations; single-use.
- SQLite encryption-at-rest is environment-dependent; support “bring your own disk encryption” for self-host; revisit later for paid hardened deployments.

## Testing strategy (early)

- Unit tests for: universal-input parser, store queries (SQLite), auth flows (WebAuthn ceremony boundaries), and FTS search ranking.
- Minimal end-to-end “happy path” test: create workspace, add contact, add interaction with due date, verify appears in “Needs Attention”, mark complete.

## Milestones (implementation order)

1. Schema + migrations + store layer (workspace/users/memberships/contacts/interactions).
2. Basic UI (home, contact detail, universal input) + FTS search.
3. Follow-ups (due_at + completed_at) + Needs Attention UI.
4. Auth v1: local accounts + passkeys + magic-link fallback.
5. Workspace invites (no roles) + basic audit/events.
6. Packaging: embed UI assets; produce a single release binary; optional Dockerfile.

## Open decisions

- Owner semantics: even without roles, keep a single workspace owner marker in v1 for irreversible actions.
- Social login: deferred for MVP unless activation data says it is needed.
