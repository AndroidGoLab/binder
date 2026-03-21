#!/bin/bash
set -euo pipefail

# Run golangci-lint on the whole repository before each commit.
# Exclusions are configured in .golangci.yml, not here.

LINT="${GOLANGCI_LINT:-$(command -v golangci-lint 2>/dev/null || echo "${HOME}/go/bin/golangci-lint")}"
if [ ! -x "$LINT" ]; then
    echo "warning: golangci-lint not found; skipping lint check"
    echo "  install: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b ~/go/bin"
    exit 0
fi

echo "golangci-lint: checking all packages..."
export GO111MODULE=on; $LINT run ./...
