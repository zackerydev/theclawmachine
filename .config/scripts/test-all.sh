#!/usr/bin/env bash
set -euo pipefail
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPTS_DIR/../../control-plane"

echo "🧪 Running unit tests..."
go test -race -count=1 ./...
echo ""
echo "🌐 Running E2E tests..."
bash "$SCRIPTS_DIR/e2e-local.sh"
