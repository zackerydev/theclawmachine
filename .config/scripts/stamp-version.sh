#!/usr/bin/env bash
set -euo pipefail

ver="${1:?Usage: stamp-version.sh <version>}"

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
charts_dir="${repo_root}/control-plane/charts"

# Strip leading 'v' if present
ver="${ver#v}"

echo "Stamping version ${ver} into chart files..."

# ── Chart.yaml: stamp 'version' for all charts ──
for chart in clawmachine openclaw picoclaw ironclaw busybox; do
    file="${charts_dir}/${chart}/Chart.yaml"
    awk -v ver="$ver" '
        /^version:/ { $0 = "version: " ver }
        { print }
    ' "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
done

# ── Chart.yaml: stamp 'appVersion' only for clawmachine ──
file="${charts_dir}/clawmachine/Chart.yaml"
awk -v ver="$ver" '
    /^appVersion:/ { $0 = "appVersion: \"" ver "\"" }
    { print }
' "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"

# ── values.yaml: stamp image.tag and backup.image.tag ──
#
# Uses an awk state machine to target only:
#   - top-level image.tag  (indent 0 → indent 2)
#   - backup.image.tag     (indent 0 → indent 2 → indent 4)
# while leaving other nested image blocks untouched (e.g. postgresql.image).
for chart in openclaw picoclaw ironclaw busybox; do
    file="${charts_dir}/${chart}/values.yaml"
    awk -v ver="$ver" '
        /^[a-zA-Z]/ {
            section = $0; sub(/:.*/, "", section)
            in_sub = 0
        }
        section == "image" && /^  tag:/ {
            $0 = "  tag: \"" ver "\""
        }
        section == "backup" {
            if (/^  image:/ && !in_sub) {
                in_sub = 1
            } else if (in_sub && /^    tag:/) {
                $0 = "    tag: \"" ver "\""
            } else if (in_sub && /^  [a-zA-Z]/) {
                in_sub = 0
            }
        }
        { print }
    ' "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
done

echo "✓ Version ${ver} stamped"
