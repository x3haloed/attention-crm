# Open-Core Feature Policy (Decision Tree)

This repo ships an **open-source Community Edition (CE)** plus a **commercial extension (Pro/Enterprise)**.
The goal is to stay *meaning-first* and trustworthy: CE must be genuinely usable for real work, while paid
features fund development and target management/scale/compliance needs.

## Non-negotiables

- CE must fully support the MVP workflows in `mvp-def.md` (capture contact, log interaction, search, follow-ups, basic workspace use).
- CE must not be intentionally crippled (no time bombs, nagware, forced cloud dependency for core flows).
- Security basics are not paywalled (secure auth options, rate limiting primitives, safe defaults).
- “Paid” features must be additive: removing them should not break CE data or prevent CE from running.

## Definitions

- **CE (open-source):** individual-contributor value; the minimum set needed to adopt, succeed, and trust the product.
- **Pro/Enterprise (paid):** management value, operational scale, compliance, advanced automation/analytics, and premium integrations.

## Decision tree

When adding a feature, answer these in order. The first “YES” decides the default placement.

1) Does the feature directly enable the core meaning-first job?
   - “I talked to someone. I need to remember and follow up.”
   - Includes: contacts, interaction timeline, due follow-ups, search, basic workspace sharing.
   - YES -> **CE**

2) Is the feature required for baseline security/safety of the core app?
   - Examples: passkeys/WebAuthn support, local accounts, secure session handling, rate limiting hooks,
     auditability needed to detect abuse (basic), backups/export, encryption-at-rest support *when available*.
   - YES -> **CE**

3) Is the feature primarily about management oversight, compliance, or enterprise procurement?
   - Examples: SSO (SAML/OIDC for corporate IdPs), SCIM, advanced audit logs/retention, RBAC/roles,
     policy controls, data residency, legal hold, DLP, admin analytics.
   - YES -> **Pro/Enterprise**

4) Is the feature primarily about scale, automation, or “power-user” leverage beyond the MVP?
   - Examples: multi-step automations, advanced reminders, AI-assisted follow-ups, advanced reporting,
     multi-pipeline deal tooling, bulk ops at high volume.
   - YES -> **Pro**

5) Is the feature an integration?
   - If it’s a **basic on-ramp** that reduces switching cost (import/export CSV, basic email capture, calendar sync),
     default **CE**.
   - If it’s a **premium connector** that provides ongoing business value or requires heavy maintenance
     (accounting/marketing suites, high-touch CRMs, data warehouses), default **Pro**.

6) Is the feature “hosted-ops” (only meaningful for the SaaS operator)?
   - Examples: billing UI, metering, tenant ops dashboards, internal support tooling.
   - YES -> **Commercial (not part of CE)** (can still be source-available, but not required for self-host).

If you hit “NO” on all the above, default to **CE**, then explicitly justify why it should be paid.

## Quick classification examples

- Contact CRUD, interaction timeline, fuzzy search -> CE
- Due-date follow-ups + “Needs Attention” list -> CE
- Multi-user workspace with invites (no roles) -> CE
- Passkeys/WebAuthn + email magic-link fallback + local accounts -> CE
- Google/GitHub social login -> CE (unless it materially increases support burden; then Pro)
- SAML SSO, SCIM, RBAC, advanced audit log UI/export -> Enterprise
- Advanced analytics dashboards, AI follow-up suggestions, multi-step automations -> Pro

## PR checklist (required)

For every new feature PR, include a short “Packaging Decision” section answering:

- Which question number in the decision tree decided placement?
- If paid: what is the CE alternative (if any), and does CE remain fully usable without it?
- Does the change introduce any CE lock-in (data, migrations, UI flows)? If yes, how is it avoided?
- Does the feature affect auth/security? If yes, confirm security basics remain CE.

