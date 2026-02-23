# Hosted beta runbook

This runbook is for hosting a small Attention CRM beta on a single VM (one app process, per-tenant SQLite DBs on disk).

## 1) Pick a hostname + TLS

- Choose a hostname (e.g. `crm.example.com`).
- Terminate TLS at a reverse proxy (Caddy or Nginx).
- Ensure the app receives `X-Forwarded-Proto: https`.

## 2) Create a data directory

Decide where data lives (must be persistent across deploys), e.g.:

- `/var/lib/attention-crm` (recommended)

This directory contains:

- `control.sqlite` (control-plane registry)
- `tenants/<opaque_storage_key>/tenant.db` (tenant databases)
- `backups/` (optional backups created by CLI and/or pre-migration backups)

## 3) Configure environment variables

Minimum recommended settings:

- `ATTENTION_LISTEN_ADDR=127.0.0.1:8080`
- `ATTENTION_DATA_DIR=/var/lib/attention-crm`
- `ATTENTION_PUBLIC_ORIGIN=https://crm.example.com`
- `ATTENTION_TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128` (if proxy is on the same box)

WebAuthn (passkeys):

- Typically you can omit `ATTENTION_WEBAUTHN_RPID` / `ATTENTION_WEBAUTHN_ORIGINS` when `ATTENTION_PUBLIC_ORIGIN` is set.
- If you need explicit control, set:
  - `ATTENTION_WEBAUTHN_RPID=crm.example.com`
  - `ATTENTION_WEBAUTHN_ORIGINS=https://crm.example.com`

Migration safety:

- Default: `ATTENTION_BACKUP_BEFORE_MIGRATE=true`
- Disable (optional): `ATTENTION_BACKUP_BEFORE_MIGRATE=false`

## 4) Start the server

Run the release binary behind your reverse proxy:

```bash
ATTENTION_LISTEN_ADDR=127.0.0.1:8080 \
ATTENTION_DATA_DIR=/var/lib/attention-crm \
ATTENTION_PUBLIC_ORIGIN=https://crm.example.com \
./attention
```

Health check:

- `GET /healthz` should return `{ ok: true, ... }`.

## 5) Backups

For small betas, the simplest option is filesystem snapshots of the entire data directory.

If you prefer app-level per-tenant backups:

```bash
./attention backup --tenant acme --data-dir /var/lib/attention-crm
```

Restore:

```bash
./attention restore --tenant acme --from /var/lib/attention-crm/backups/acme-20260223T000000Z.db --data-dir /var/lib/attention-crm
```

Notes:

- Restores replace the current `tenant.db` and preserve the old `tenant.db` / WAL files as `*.bak.<timestamp>`.
- Pre-migration backups (if enabled) live under `backups/migrations/`.

## 6) Upgrade procedure

1. Take a backup/snapshot.
2. Stop the process (or `systemctl stop attention-crm` if using systemd).
3. Replace the binary.
4. Start the process.
5. Verify `/healthz`.
6. Smoke test login + omnibar quick capture.

## 7) Troubleshooting

- Passkeys failing in production almost always means a mismatched origin/RPID.
  - Confirm `ATTENTION_PUBLIC_ORIGIN` matches the browser URL exactly.
  - Confirm your reverse proxy sets `X-Forwarded-Proto: https`.
- If rate limiting is unexpectedly aggressive behind a proxy, set `ATTENTION_TRUSTED_PROXY_CIDRS` correctly.

