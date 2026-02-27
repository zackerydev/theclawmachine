package version

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// NormalizeRuntimeImageTag converts a runtime version into a Docker image tag.
//
// Rules:
// - "v" prefixes are stripped.
// - Stable and prerelease semver versions are accepted.
// - Empty/"dev" indicates dev mode and returns an empty tag.
// - Any other non-semver value is rejected.
func NormalizeRuntimeImageTag(runtimeVersion string) (string, error) {
	trimmed := strings.TrimSpace(runtimeVersion)
	if trimmed == "" || strings.EqualFold(trimmed, "dev") {
		return "", nil
	}

	normalized := strings.TrimPrefix(trimmed, "v")
	parsed, err := semver.NewVersion(normalized)
	if err != nil {
		return "", fmt.Errorf("runtime version %q is not valid semver; expected release or prerelease tag", runtimeVersion)
	}
	if parsed.Metadata() != "" {
		return "", fmt.Errorf("runtime version %q includes semver build metadata, which is not allowed in image tags", runtimeVersion)
	}

	return normalized, nil
}

// ResolveRuntimeOrFallbackImageTag resolves an image tag from runtime version.
// When runtime is empty/dev, fallbackTag is used.
func ResolveRuntimeOrFallbackImageTag(runtimeVersion, fallbackTag string) (tag string, usedFallback bool, err error) {
	tag, err = NormalizeRuntimeImageTag(runtimeVersion)
	if err != nil {
		return "", false, err
	}
	if tag != "" {
		return tag, false, nil
	}

	fallback := strings.TrimSpace(fallbackTag)
	if fallback == "" {
		return "", true, fmt.Errorf("fallback image tag is required for runtime version %q", runtimeVersion)
	}
	return fallback, true, nil
}
