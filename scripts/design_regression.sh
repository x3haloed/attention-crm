#!/usr/bin/env bash
set -euo pipefail

if ! command -v npx >/dev/null 2>&1; then
  echo "Error: npx is required but not found on PATH." >&2
  echo "Install Node.js/npm (which provides npx) and retry." >&2
  exit 1
fi

GO_BIN="${GO_BIN:-go}"
if ! command -v "${GO_BIN}" >/dev/null 2>&1; then
  if [[ -x "/usr/local/go/bin/go" ]]; then
    GO_BIN="/usr/local/go/bin/go"
  else
    echo "Error: go is required but not found on PATH (and /usr/local/go/bin/go not found)." >&2
    exit 1
  fi
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${ATTENTION_REGRESSION_PORT:-8099}"
BASE="http://127.0.0.1:${PORT}"
STATIC_PORT="${ATTENTION_REGRESSION_STATIC_PORT:-8100}"
STATIC_BASE="http://127.0.0.1:${STATIC_PORT}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${ROOT}/output/playwright/design-regression/${TS}"
DATA_DIR="${ROOT}/tmp/design-regression/${TS}/data"

mkdir -p "${OUT_DIR}" "${DATA_DIR}"

echo "[design-regression] building css"
(cd "${ROOT}" && npm run build:css >/dev/null)

echo "[design-regression] starting server on ${BASE}"
ATTENTION_LISTEN_ADDR="127.0.0.1:${PORT}" \
ATTENTION_DATA_DIR="${DATA_DIR}" \
ATTENTION_DEV_NOAUTH=1 \
"${GO_BIN}" run ./cmd/attention >/dev/null 2>&1 &
SERVER_PID="$!"
python3 -m http.server --bind 127.0.0.1 "${STATIC_PORT}" -d "${ROOT}" >/dev/null 2>&1 &
STATIC_PID="$!"

cleanup() {
  if kill -0 "${STATIC_PID}" >/dev/null 2>&1; then
    kill "${STATIC_PID}" >/dev/null 2>&1 || true
  fi
  if kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "[design-regression] waiting for server"
for _ in $(seq 1 80); do
  if curl -fsS "${BASE}/t/acme/app" >/dev/null 2>&1; then
    break
  fi
  sleep 0.15
done

PWCLI=(npx --yes --package @playwright/cli playwright-cli --session design-regression --config "${ROOT}/scripts/playwright-cli.json")

js_quote() {
  python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$1"
}

shot() {
  local url="$1"
  local path="$2"
  local qpath
  qpath="$(js_quote "${path}")"
  "${PWCLI[@]}" open "${url}" >/dev/null
  "${PWCLI[@]}" run-code "await page.waitForLoadState('networkidle').then(() => page.waitForTimeout(150)).then(() => page.screenshot({path: ${qpath}, fullPage: true}))" >/dev/null
  echo "  - ${path}"
}

echo "[design-regression] capturing reference designs"
shot "${STATIC_BASE}/docs/design/home.html" "${OUT_DIR}/design-home.png"
shot "${STATIC_BASE}/docs/design/home-omni-open.html" "${OUT_DIR}/design-home-omni-open.png"
shot "${STATIC_BASE}/docs/design/contact-edit.html" "${OUT_DIR}/design-contact-edit.png"

echo "[design-regression] capturing app pages"
shot "${BASE}/t/acme/app" "${OUT_DIR}/app-home.png"

echo "  - ${OUT_DIR}/app-home-omni-open.png"
"${PWCLI[@]}" open "${BASE}/t/acme/app" >/dev/null
Q_OMNI="$(js_quote "${OUT_DIR}/app-home-omni-open.png")"
"${PWCLI[@]}" run-code "await page.waitForLoadState('networkidle').then(() => page.fill('#omni-input','Bob Smith')).then(() => page.dispatchEvent('#omni-input','input')).then(() => page.waitForFunction(() => { const el = document.getElementById('search-suggestions'); return el && !el.classList.contains('hidden'); }, null, {timeout: 5000})).then(() => page.waitForTimeout(150)).then(() => page.screenshot({path: ${Q_OMNI}, fullPage: true}))" >/dev/null

shot "${BASE}/t/acme/contacts/1" "${OUT_DIR}/app-contact-edit.png"
shot "${BASE}/t/acme/members" "${OUT_DIR}/app-members.png"

cat > "${OUT_DIR}/manifest.txt" <<EOF
design-home.png ${STATIC_BASE}/docs/design/home.html
design-home-omni-open.png ${STATIC_BASE}/docs/design/home-omni-open.html
design-contact-edit.png ${STATIC_BASE}/docs/design/contact-edit.html
app-home.png ${BASE}/t/acme/app
app-home-omni-open.png ${BASE}/t/acme/app (typed "Bob Smith")
app-contact-edit.png ${BASE}/t/acme/contacts/1
app-members.png ${BASE}/t/acme/members
EOF

echo "[design-regression] done"
echo "[design-regression] output: ${OUT_DIR}"
