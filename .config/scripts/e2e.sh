#!/usr/bin/env bash
set -euo pipefail
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)"

teardown() {
  bash "$SCRIPTS_DIR/e2e-teardown.sh" || true
}
trap teardown EXIT

bash "$SCRIPTS_DIR/e2e-setup.sh"
bash "$SCRIPTS_DIR/e2e-run.sh"
