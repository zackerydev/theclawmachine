#!/usr/bin/env bash
# Run full E2E lifecycle tests against the kind cluster.
# ClawMachine runs locally in --web mode, talking to the cluster via kubeconfig.
set -euo pipefail
cd "$(dirname "$0")/../../control-plane"

PORT=9080
BIN=/tmp/clawmachine-e2e-run

echo "🔨 Building ClawMachine..."
go build -o "$BIN" ./cmd/clawmachine/

# Switch to E2E cluster context
CLUSTER_NAME="claw-machine"
kubectl config use-context "kind-$CLUSTER_NAME"

echo "🚀 Starting ClawMachine API server on :$PORT (pointing at kind-$CLUSTER_NAME)..."
POD_NAMESPACE="claw-machine" PORT="$PORT" "$BIN" --web --dev &
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

# Resolve 1Password credentials
OP_CRED_FILE="${OP_CREDENTIALS_FILE:-}"
if [ -n "$OP_CRED_FILE" ]; then
  if [[ "$OP_CRED_FILE" != /* ]]; then
    OP_CRED_FILE="$(cd .. && pwd)/$OP_CRED_FILE"
  fi
elif [ -n "${ONEPASSWORD_CREDENTIALS_JSON:-}" ]; then
  OP_CRED_FILE="$(mktemp)"
  echo "$ONEPASSWORD_CREDENTIALS_JSON" > "$OP_CRED_FILE"
else
  OP_CRED_FILE="$HOME/1password-credentials.json"
fi

if [ ! -f "$OP_CRED_FILE" ]; then
  echo "❌ Credentials file not found: $OP_CRED_FILE"
  echo "   Set OP_CREDENTIALS_FILE in .env or create ~/1password-credentials.json"
  exit 1
fi

OP_TOKEN="${OP_CONNECT_TOKEN:-${ONEPASSWORD_CONNECT_TOKEN:-}}"
if [ -z "$OP_TOKEN" ] && [ -f "$HOME/1password-jwt.txt" ]; then
  OP_TOKEN="$(cat "$HOME/1password-jwt.txt")"
fi

if [ -z "$OP_TOKEN" ]; then
  echo "❌ No OP_CONNECT_TOKEN found"
  echo "   Set OP_CONNECT_TOKEN in .env or create ~/1password-jwt.txt"
  exit 1
fi

OP_VAULT="${OP_VAULT_NAME:-claw-machine-dev}"
OP_SECRET_ITEM="${OP_SECRET_ITEM_NAME:-anthropic-key}"

echo "  Using credentials: $OP_CRED_FILE"
echo "  Using vault: $OP_VAULT"

echo "🧪 Running lifecycle E2E tests..."
CLAWMACHINE_URL="http://localhost:$PORT" \
OP_CREDENTIALS_FILE="$OP_CRED_FILE" \
OP_CONNECT_TOKEN="$OP_TOKEN" \
OP_VAULT_NAME="$OP_VAULT" \
OP_SECRET_ITEM_NAME="$OP_SECRET_ITEM" \
  go test -tags e2e -v -count=1 -timeout 30m \
    -run 'TestLifecycle|TestIronClawLifecycle|TestOpenClawLifecycle|TestOpenClawExtraSoftwareInstallsClaude|TestPicoClawLifecycle' \
    ./e2e/

echo "✅ Lifecycle E2E tests passed!"
