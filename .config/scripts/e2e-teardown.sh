#!/usr/bin/env bash
# Tear down E2E test state: remove any deployed test bots and verify namespace is clean.
# ClawMachine itself no longer runs in-cluster, so there's nothing to uninstall.
set -euo pipefail
cd "$(dirname "$0")/../../control-plane"

NAMESPACE="claw-machine"

echo "🗑️  Cleaning up test bots in namespace '$NAMESPACE'..."

# Delete any helm releases left by lifecycle tests
for release in lifecycle-test e2e-lifecycle e2e-nettest; do
  if helm list -n "$NAMESPACE" --short 2>/dev/null | grep -q "^$release$"; then
    echo "  Uninstalling $release..."
    helm uninstall "$release" -n "$NAMESPACE" 2>/dev/null || true
  fi
done

sleep 5

echo "🔍 Verifying cleanup..."

# Check for leftover bot pods (infra pods like ESO are fine to leave)
BOT_PODS=$(kubectl get pods -n "$NAMESPACE" \
  -l 'app.kubernetes.io/managed-by=Helm' \
  --no-headers 2>/dev/null | wc -l || echo "0")

if [ "$BOT_PODS" -gt 0 ]; then
  echo "⚠️  Some bot pods still exist in $NAMESPACE (may still be terminating):"
  kubectl get pods -n "$NAMESPACE" -l 'app.kubernetes.io/managed-by=Helm'
else
  echo "✅ No orphaned bot pods"
fi

# Check for orphaned PVCs
PVCS=$(kubectl get pvc -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l || echo "0")
if [ "$PVCS" -gt 0 ]; then
  echo "⚠️  Orphaned PVCs in $NAMESPACE:"
  kubectl get pvc -n "$NAMESPACE"
fi

echo ""
echo "🎉 Teardown complete!"
