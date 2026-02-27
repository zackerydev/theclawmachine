#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"

goreleaser_file=".config/goreleaser.yml"
docs_file="docs/content/docs/bot-types.md"
clawmachine_chart="control-plane/charts/clawmachine/Chart.yaml"

expected_repos=(
  "ghcr.io/zackerydev/openclaw"
  "ghcr.io/zackerydev/picoclaw"
  "ghcr.io/zackerydev/ironclaw"
  "ghcr.io/zackerydev/theclawmachine-toolbox"
)

chart_for_repo() {
  case "$1" in
    ghcr.io/zackerydev/openclaw) echo "control-plane/charts/openclaw/values.yaml" ;;
    ghcr.io/zackerydev/picoclaw) echo "control-plane/charts/picoclaw/values.yaml" ;;
    ghcr.io/zackerydev/ironclaw) echo "control-plane/charts/ironclaw/values.yaml" ;;
    ghcr.io/zackerydev/theclawmachine-toolbox) echo "control-plane/charts/busybox/values.yaml" ;;
    *) return 1 ;;
  esac
}

chart_image_field() {
  local chart_values="$1"
  local field="$2"
  awk -v field="$field" '
    /^image:/ { in_image=1; next }
    in_image && $1 == field ":" { gsub(/"/, "", $2); print $2; exit }
    in_image && /^[^ ]/ { in_image=0 }
  ' "$chart_values"
}

release_tag="$(awk -F'"' '/^appVersion:/ {print $2; exit}' "$clawmachine_chart")"
if [[ -z "${release_tag}" ]]; then
  echo "❌ Unable to resolve release appVersion from ${clawmachine_chart}"
  exit 1
fi

expected_release_tag="${EXPECTED_RELEASE_TAG:-}"
if [[ -n "${expected_release_tag}" && "${release_tag}" != "${expected_release_tag}" ]]; then
  echo "❌ ${clawmachine_chart} appVersion=${release_tag}, want ${expected_release_tag} (EXPECTED_RELEASE_TAG)"
  exit 1
fi

for repo in "${expected_repos[@]}"; do
  if ! grep -Fq -- "- ${repo}" "$goreleaser_file"; then
    echo "❌ Missing Goreleaser image output for ${repo}"
    exit 1
  fi

  chart_values="$(chart_for_repo "$repo")"
  got_repo="$(chart_image_field "$chart_values" repository)"
  got_tag="$(chart_image_field "$chart_values" tag)"

  if [[ "$got_repo" != "$repo" ]]; then
    echo "❌ ${chart_values} image.repository=${got_repo}, want ${repo}"
    exit 1
  fi
  if [[ "$got_tag" != "$release_tag" ]]; then
    echo "❌ ${chart_values} image.tag=${got_tag}, want ${release_tag}"
    exit 1
  fi

  if ! grep -Fq -- "- image: \`${repo}:${release_tag}\`" "$docs_file"; then
    echo "❌ ${docs_file} missing image reference ${repo}:${release_tag}"
    exit 1
  fi
done

if ! grep -Fq "ghcr.io/zackerydev/openclaw:${release_tag}" .config/Tiltfile; then
  echo "❌ .config/Tiltfile openclaw image tag does not match ${release_tag}"
  exit 1
fi
if ! grep -Fq "ghcr.io/zackerydev/picoclaw:${release_tag}" .config/Tiltfile; then
  echo "❌ .config/Tiltfile picoclaw image tag does not match ${release_tag}"
  exit 1
fi
if ! grep -Fq "ghcr.io/zackerydev/ironclaw:${release_tag}" .config/Tiltfile; then
  echo "❌ .config/Tiltfile ironclaw image tag does not match ${release_tag}"
  exit 1
fi
if ! grep -Fq "ghcr.io/zackerydev/theclawmachine-toolbox:${release_tag}" .config/Tiltfile; then
  echo "❌ .config/Tiltfile toolbox image tag does not match ${release_tag}"
  exit 1
fi

echo "✓ Bot image contract is aligned with Goreleaser outputs"
