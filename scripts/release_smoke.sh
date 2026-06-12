#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found (required for release smoke checks)" >&2
  exit 1
fi

"${ROOT}/scripts/build_release.sh"

if [[ ! -x "${ROOT}/dist/attention" ]]; then
  echo "release binary was not created at dist/attention" >&2
  exit 1
fi

PORT="${ATTENTION_RELEASE_SMOKE_PORT:-8098}"
BASE="http://127.0.0.1:${PORT}"
DATA_DIR="${ROOT}/tmp/release-smoke/data"

rm -rf "${DATA_DIR}"
mkdir -p "${DATA_DIR}"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

ATTENTION_LISTEN_ADDR="127.0.0.1:${PORT}" \
ATTENTION_DATA_DIR="${DATA_DIR}" \
ATTENTION_DEV_NOAUTH=1 \
"${ROOT}/dist/attention" >/dev/null 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 80); do
  if curl -fsS "${BASE}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.15
done

curl -fsS "${BASE}/healthz" >/dev/null
curl -fsS "${BASE}/" >/dev/null
"${ROOT}/dist/attention" version >/dev/null

echo "release smoke checks passed"
