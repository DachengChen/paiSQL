#!/bin/bash
# =============================================================
# Local Lint Check — mirrors CI (golangci-lint v1.64 + Go 1.24)
# =============================================================
# Usage:
#   scripts/lint-local.sh          # run lint
#   scripts/lint-local.sh --fix     # run lint with auto-fix
#   scripts/lint-local.sh --verbose # verbose output
#
# This script downloads the exact golangci-lint version used in
# CI so local results match what GitHub Actions will report.
# =============================================================
set -euo pipefail

# ── Configuration (keep in sync with .github/workflows/ci.yml) ─
CI_GOLANGCI_LINT_VERSION="1.64.8"
CI_GO_VERSION="1.24"
# ────────────────────────────────────────────────────────────────

# ── Ensure goenv shims are first in PATH ───────────────────────
if command -v goenv &>/dev/null; then
  export GOENV_ROOT="${GOENV_ROOT:-$HOME/.goenv}"
  export PATH="${GOENV_ROOT}/shims:${PATH}"
fi


PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CACHE_DIR="${PROJECT_ROOT}/.ai_generated/bin"
LINT_BIN="${CACHE_DIR}/golangci-lint-${CI_GOLANGCI_LINT_VERSION}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}ℹ${NC}  $*"; }
ok()    { echo -e "${GREEN}✔${NC}  $*"; }
warn()  { echo -e "${YELLOW}⚠${NC}  $*"; }
err()   { echo -e "${RED}✖${NC}  $*"; }

# ── Parse flags ────────────────────────────────────────────────
FIX_FLAG=""
VERBOSE_FLAG=""
for arg in "$@"; do
  case "$arg" in
    --fix)     FIX_FLAG="--fix" ;;
    --verbose) VERBOSE_FLAG="-v" ;;
    *)         err "Unknown flag: $arg"; exit 1 ;;
  esac
done

# ── Check Go version ──────────────────────────────────────────
info "Checking Go version..."
GO_VER=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | head -1 | sed 's/go//')
if [[ -z "$GO_VER" ]]; then
  err "Go not found in PATH"
  exit 1
fi

if [[ "$GO_VER" != "$CI_GO_VERSION"* ]]; then
  warn "Local Go version is ${GO_VER}, CI uses ${CI_GO_VERSION}"
  warn "Results may differ. Switch with: goenv install ${CI_GO_VERSION} && goenv local ${CI_GO_VERSION}"
else
  ok "Go version ${GO_VER} matches CI"
fi

# ── Download golangci-lint if needed ───────────────────────────
if [[ ! -x "$LINT_BIN" ]]; then
  info "Downloading golangci-lint v${CI_GOLANGCI_LINT_VERSION}..."
  mkdir -p "$CACHE_DIR"

  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
  esac

  TAR_URL="https://github.com/golangci/golangci-lint/releases/download/v${CI_GOLANGCI_LINT_VERSION}/golangci-lint-${CI_GOLANGCI_LINT_VERSION}-${OS}-${ARCH}.tar.gz"
  TMP_DIR=$(mktemp -d)
  trap "rm -rf $TMP_DIR" EXIT

  curl -sSL "$TAR_URL" | tar xz -C "$TMP_DIR"
  cp "${TMP_DIR}/golangci-lint-${CI_GOLANGCI_LINT_VERSION}-${OS}-${ARCH}/golangci-lint" "$LINT_BIN"
  chmod +x "$LINT_BIN"

  ok "Downloaded golangci-lint v${CI_GOLANGCI_LINT_VERSION}"
else
  ok "Using cached golangci-lint v${CI_GOLANGCI_LINT_VERSION}"
fi

echo ""
info "golangci-lint version: $("$LINT_BIN" --version 2>&1 | head -1)"
info "Go version: $(go version)"
echo ""

# ── Run lint ───────────────────────────────────────────────────
cd "$PROJECT_ROOT"

info "Running golangci-lint on ./... ${FIX_FLAG:+(with --fix)}"
echo "────────────────────────────────────────────"

if "$LINT_BIN" run ./... $FIX_FLAG $VERBOSE_FLAG; then
  echo "────────────────────────────────────────────"
  ok "No lint issues found! Safe to push. 🚀"
  exit 0
else
  EXIT_CODE=$?
  echo "────────────────────────────────────────────"
  err "Lint issues found. Fix them before pushing."
  echo ""
  info "Tips:"
  echo "  • Run with --fix to auto-fix some issues:"
  echo "    scripts/lint-local.sh --fix"
  echo "  • Run with --verbose for more detail:"
  echo "    scripts/lint-local.sh --verbose"
  exit $EXIT_CODE
fi
