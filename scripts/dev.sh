#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

AIR_BIN="${AIR_BIN:-}"
if [[ -z "${AIR_BIN}" ]]; then
  if command -v air >/dev/null 2>&1; then
    AIR_BIN="air"
  elif [[ -x "$(go env GOPATH 2>/dev/null)/bin/air" ]]; then
    AIR_BIN="$(go env GOPATH)/bin/air"
  else
    echo "air not found." >&2
    echo "Install: go install github.com/air-verse/air@latest" >&2
    echo "Then ensure GOPATH/bin is on PATH, or set AIR_BIN explicitly." >&2
    exit 1
  fi
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found (required for Tailwind watch)." >&2
  exit 1
fi

mkdir -p tmp

cleanup() {
  if [[ -n "${CSS_PID:-}" ]]; then
    kill "${CSS_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

npm run watch:css &
CSS_PID=$!

exec "$AIR_BIN" -c .air.toml

