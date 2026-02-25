#!/usr/bin/env bash
# Run E2E tests locally — no cluster needed.
# Starts ClawMachine in API-server mode (--web), runs API + CLI smoke tests.
set -euo pipefail
cd "$(dirname "$0")/../../control-plane"

PORT=8080
BIN=/tmp/clawmachine-e2e

echo "🔨 Building ClawMachine..."
go build -o "$BIN" ./cmd/clawmachine/

echo "🚀 Starting ClawMachine API server on :$PORT (--web mode)..."
"$BIN" --web --dev &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null; rm -f $BIN" EXIT

# Wait for server to be ready
echo "⏳ Waiting for server..."
for i in $(seq 1 30); do
  if curl -sf "http://localhost:$PORT/health" >/dev/null 2>&1; then
    echo "✅ Server ready"
    break
  fi
  sleep 1
done

# Install the binary into PATH for CLI smoke tests
export PATH="$(dirname $BIN):$PATH"
cp "$BIN" "$(dirname $BIN)/clawmachine"

echo "🧪 Running E2E tests (API + CLI smoke)..."
CLAWMACHINE_URL="http://localhost:$PORT" \
  go test -tags e2e -v -count=1 -timeout 5m \
    -run 'TestAPI|TestHealth|TestCLI|TestInstall|TestProvider|TestConnect' \
    ./e2e/

echo "✅ E2E tests passed!"
