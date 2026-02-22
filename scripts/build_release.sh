#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO_BIN="${GO_BIN:-}"
if [[ -z "${GO_BIN}" ]]; then
  if command -v go >/dev/null 2>&1; then
    GO_BIN="go"
  elif [[ -x "/usr/local/go/bin/go" ]]; then
    GO_BIN="/usr/local/go/bin/go"
  else
    echo "go not found (set GO_BIN or add go to PATH)" >&2
    exit 1
  fi
fi

if command -v npm >/dev/null 2>&1; then
  npm ci
  npm run build:css
else
  echo "npm not found (required to build Tailwind CSS)" >&2
  exit 1
fi

mkdir -p dist

VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "Building attention (${VERSION}, ${COMMIT}, ${BUILD_TIME_UTC})"
"$GO_BIN" build \
  -trimpath \
  -ldflags "-s -w" \
  -o dist/attention \
  ./cmd/attention

echo "Wrote dist/attention"
