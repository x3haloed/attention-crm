# Self-hosting guide

This app is designed to run as a single Go binary with SQLite on disk.

## Passkeys (WebAuthn) requirements

- For non-localhost deployments, WebAuthn requires HTTPS.
- `ATTENTION_WEBAUTHN_RPID` must match your site’s “relying party id” (typically your domain, e.g. `crm.example.com` or `example.com`).
- `ATTENTION_WEBAUTHN_ORIGINS` must include the exact origin(s) users will visit (e.g. `https://crm.example.com`).

## Reverse proxy (recommended)

Run the app on localhost and terminate TLS at a reverse proxy (Caddy/Nginx).
The app uses `X-Forwarded-Proto: https` to decide whether to set `Secure` cookies.

### Forwarded client IPs (rate limiting)

If you run behind a reverse proxy, configure trusted proxy CIDRs so rate limiting keys off the real client IP
instead of the proxy’s IP:

- `ATTENTION_TRUSTED_PROXY_CIDRS` is a comma-separated list of CIDRs (e.g. `127.0.0.1/32,::1/128`).
- Only requests whose `RemoteAddr` is in one of these CIDRs will honor `X-Forwarded-For` / `X-Real-IP`.

### Example: Caddy

```caddyfile
crm.example.com {
  reverse_proxy 127.0.0.1:8080
  header_up X-Forwarded-Proto {scheme}
  header_up X-Forwarded-Host {host}
}
```

### Example: Nginx

```nginx
server {
  listen 443 ssl;
  server_name crm.example.com;

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
}
```

## Running the binary

Build a release binary:

```bash
./scripts/build_release.sh
```

Run it:

```bash
ATTENTION_LISTEN_ADDR=127.0.0.1:8080 \
ATTENTION_DATA_DIR=/var/lib/attention-crm \
ATTENTION_WEBAUTHN_RPID=crm.example.com \
ATTENTION_WEBAUTHN_ORIGINS=https://crm.example.com \
./dist/attention
```

## Data and backups

- Control-plane registry DB: `<data_dir>/control.sqlite`
- Tenant DBs: `<data_dir>/tenants/<tenant_slug>.sqlite`

Back up the whole `ATTENTION_DATA_DIR` directory (while the app is stopped, or using filesystem snapshots).
