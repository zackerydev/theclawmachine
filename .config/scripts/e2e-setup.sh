#!/usr/bin/env bash
# Set up the E2E kind cluster with bot images and infrastructure.
# ClawMachine is installed in-cluster for setup parity with install flow.
set -euo pipefail
cd "$(dirname "$0")/../../control-plane"

CLUSTER_NAME="claw-machine"
CLAWMACHINE_TAG="$(awk -F'"' '/^appVersion:/ {print $2}' ./charts/clawmachine/Chart.yaml)"
if [[ -z "$CLAWMACHINE_TAG" ]]; then
  echo "❌ Unable to resolve appVersion from charts/clawmachine/Chart.yaml"
  exit 1
fi

chart_image_tag() {
  local chart_values_path="$1"
  awk '
    /^image:/ { in_image=1; next }
    in_image && /^  tag:/ { gsub(/"/, "", $2); print $2; exit }
    in_image && /^[^ ]/ { in_image=0 }
  ' "$chart_values_path"
}

PICOCLAW_TAG="$(chart_image_tag ./charts/picoclaw/values.yaml)"
OPENCLAW_TAG="$(chart_image_tag ./charts/openclaw/values.yaml)"
IRONCLAW_TAG="$(chart_image_tag ./charts/ironclaw/values.yaml)"
if [[ -z "$PICOCLAW_TAG" ]]; then
  echo "❌ Unable to resolve image.tag from charts/picoclaw/values.yaml"
  exit 1
fi
if [[ -z "$OPENCLAW_TAG" ]]; then
  echo "❌ Unable to resolve image.tag from charts/openclaw/values.yaml"
  exit 1
fi
if [[ -z "$IRONCLAW_TAG" ]]; then
  echo "❌ Unable to resolve image.tag from charts/ironclaw/values.yaml"
  exit 1
fi

# Track background PIDs and their names for error reporting
declare -a PIDS=()
declare -a PID_LABELS=()
declare -a PID_LOGS=()

bg() {
  local name="$1"; shift
  local log_file
  log_file="$(mktemp "${TMPDIR:-/tmp}/clawmachine-e2e.XXXXXX")"
  "$@" >"$log_file" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  PID_LABELS+=("$name")
  PID_LOGS+=("$log_file")
}

wait_all() {
  local failed=0
  local i
  for i in "${!PIDS[@]}"; do
    local pid="${PIDS[$i]}"
    local log_file="${PID_LOGS[$i]}"
    if ! wait "$pid"; then
      echo "❌ Failed: ${PID_LABELS[$i]}"
      if [[ -s "$log_file" ]]; then
        echo "----- ${PID_LABELS[$i]} output -----"
        cat "$log_file"
        echo "----- end output -----"
      fi
      failed=1
    fi
    rm -f "$log_file"
  done
  PIDS=()
  PID_LABELS=()
  PID_LOGS=()
  [[ $failed -eq 0 ]] || exit 1
}

echo "🔨 Step 1: Build bot images in parallel"

# Build images used by installed workloads.
bg "build clawmachine" docker buildx build --load -t "ghcr.io/zackerydev/theclawmachine:${CLAWMACHINE_TAG}" .
bg "build picoclaw"  docker buildx build --load -t "ghcr.io/zackerydev/picoclaw:${PICOCLAW_TAG}" ../docker/picoclaw
bg "build openclaw"  docker buildx build --load -t "ghcr.io/zackerydev/openclaw:${OPENCLAW_TAG}" ../docker/openclaw
bg "pull pgvector"   docker pull pgvector/pgvector:pg17

# toolbox depends on ironclaw — chain them
bg "build ironclaw+toolbox" bash -c "
  docker buildx build --load -t ghcr.io/zackerydev/ironclaw:${IRONCLAW_TAG} ../docker/ironclaw
  docker buildx build --load -t ghcr.io/zackerydev/theclawmachine-toolbox:0.1.0 ../docker/toolbox
"

wait_all
echo "✅ All images built"

echo "🚀 Step 2: Create kind cluster"
kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
kind create cluster \
  --name "$CLUSTER_NAME" \
  --config ./cmd/clawmachine/assets/kind/cilium.yaml

echo "📦 Step 3: Load images into kind (parallel)"
bg "load picoclaw"  kind load docker-image "ghcr.io/zackerydev/picoclaw:${PICOCLAW_TAG}"  --name "$CLUSTER_NAME"
bg "load ironclaw"  kind load docker-image ghcr.io/zackerydev/ironclaw:${IRONCLAW_TAG}    --name "$CLUSTER_NAME"
bg "load openclaw"  kind load docker-image "ghcr.io/zackerydev/openclaw:${OPENCLAW_TAG}"   --name "$CLUSTER_NAME"
bg "load toolbox"   kind load docker-image ghcr.io/zackerydev/theclawmachine-toolbox:0.1.0   --name "$CLUSTER_NAME"
bg "load clawmachine" kind load docker-image "ghcr.io/zackerydev/theclawmachine:${CLAWMACHINE_TAG}" --name "$CLUSTER_NAME"
bg "load pgvector"  kind load docker-image pgvector/pgvector:pg17                         --name "$CLUSTER_NAME"
wait_all
echo "✅ Images loaded"

echo "🚀 Step 4: Install cluster infrastructure (Cilium + ESO + ClawMachine)"
go run ./cmd/clawmachine install \
  --namespace "claw-machine" \
  --context "kind-$CLUSTER_NAME" \
  --cilium \
  --external-secrets \
  --interactive=false

echo "⏳ Step 5: Wait for infrastructure pods (parallel)"
bg "wait cilium" bash -c "
  kubectl wait --for=condition=ready pod -l k8s-app=cilium -n kube-system --timeout=300s 2>/dev/null || \
  kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=cilium-agent -n kube-system --timeout=300s 2>/dev/null || true
  echo '✅ Cilium ready'
"
bg "wait coredns" bash -c "
  kubectl wait --for=condition=ready pod -l k8s-app=kube-dns -n kube-system --timeout=120s 2>/dev/null || true
  echo '✅ CoreDNS ready'
"
bg "wait eso" bash -c "
  kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=external-secrets -n external-secrets --timeout=120s 2>/dev/null || true
  echo '✅ External Secrets Operator ready'
"
wait_all

echo ""
echo "✅ Setup complete."
echo "   Next: run 'mise run e2e:run' to execute lifecycle tests."
