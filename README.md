# attention-crm

Early scaffold for the meaning-first CRM MVP.

## Current implementation

- Control-plane SQLite registry (`.attention/data/control.sqlite`) for tenant metadata.
- SaaS/self-host tenant data in one SQLite DB per tenant (`.attention/data/tenants/<slug>.sqlite`).
- URL tenant hint using `/t/{slug}`.
- First-run setup flow at `/setup` with browser passkey enrollment.
- Passkey login flow at `/t/{slug}/login` and session landing at `/t/{slug}/app`.
- Contact capture form on `/t/{slug}/app`.
- Interaction logging form (note/call/email/meeting) with optional due date.
- "Needs Attention" list with mark-complete action.
- Recent interactions feed.
- Universal input form (search contacts and quick-create contact names).
- Contact detail page with interaction timeline.
- Invite links for teammates (no SMTP required) with passkey enrollment.

## Run

```bash
npm ci
npm run build:css
go run ./cmd/attention
```

Or:

```bash
make run
```

## Dev (hot reload)

```bash
go install github.com/air-verse/air@latest
./scripts/dev.sh
```

Optional environment variables:

- `ATTENTION_LISTEN_ADDR` (default `:8080`)
- `ATTENTION_DATA_DIR` (default `.attention/data`)
- `ATTENTION_WEBAUTHN_RPID` (default `localhost`)
- `ATTENTION_WEBAUTHN_RP_NAME` (default `Attention CRM`)
- `ATTENTION_WEBAUTHN_ORIGINS` (default `http://localhost:8080`, comma-separated)

## Notes

- Passkeys are now the primary setup/login path.
- Release build (single binary): `scripts/build_release.sh` (writes `dist/attention`).
- Self-hosting (TLS + WebAuthn config): `docs/self-host.md`.
- Design regression screenshots (compares app vs `docs/design/*`): `scripts/design_regression.sh` (writes to `output/playwright/`).
- Task board: `.isnad/state/board.md` (regenerate with `python3 tools/work-board/scripts/fold_state.py --root .`).
- Task board UI: `python3 scripts/isnad_board.py` (local web UI that writes directives).
- Control receipts: `python3 scripts/isnad_ack.py` (appends `ack_directive` evidence records for any unacked directives).
- Current implemented endpoints:
  - `GET /`
  - `GET /setup`
  - `POST /setup/passkey/start`
  - `POST /setup/passkey/finish?flow_id=...`
  - `GET /t/{slug}/login`
  - `POST /t/{slug}/login/passkey/start`
  - `POST /t/{slug}/login/passkey/finish?flow_id=...`
  - `POST /t/{slug}/logout`
  - `GET /t/{slug}/app`
  - `POST /t/{slug}/contacts`
  - `GET /t/{slug}/contacts/{id}`
  - `POST /t/{slug}/contacts/{id}/update`
  - `POST /t/{slug}/contacts/{id}/interactions`
  - `POST /t/{slug}/interactions`
  - `POST /t/{slug}/interactions/{id}/complete`
  - `POST /t/{slug}/universal`
  - `POST /t/{slug}/invites`
  - `GET /t/{slug}/invite/{token}`
  - `POST /t/{slug}/invite/{token}/passkey/start`
  - `POST /t/{slug}/invite/{token}/passkey/finish?flow_id=...`
