package service

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/registry"
)

// BotType represents a type of Claw bot.
type BotType string

const (
	BotTypePicoClaw BotType = "picoclaw"
	BotTypeIronClaw BotType = "ironclaw"
	BotTypeOpenClaw BotType = "openclaw"
	BotTypeBusyBox  BotType = "busybox"
)

// DefaultChartRepoURL is the default Helm repository for bot charts.
// Can be overridden via CLAWMACHINE_CHART_REPO env var.
const DefaultChartRepoURL = "oci://ghcr.io/zackerydev/theclawmachine/charts"

//go:embed charts/picoclaw.tgz
var picoClawChart []byte

//go:embed charts/ironclaw.tgz
var ironClawChart []byte

//go:embed charts/openclaw.tgz
var openClawChart []byte

//go:embed charts/busybox.tgz
var busyBoxChart []byte

//go:embed charts/clawmachine.tgz
var clawMachineChart []byte

//go:embed charts/external-secrets-2.0.0.tgz
var esoChart []byte

//go:embed charts/cilium-1.17.1.tgz
var ciliumChart []byte

//go:embed charts/connect-2.3.0.tgz
var connectChart []byte

// ChartResolver resolves Helm charts, trying remote sources first
// and falling back to embedded charts.
type ChartResolver struct {
	repoURL  string
	settings *cli.EnvSettings
}

// NewChartResolver creates a ChartResolver. If repoURL is empty,
// it checks CLAWMACHINE_CHART_REPO env var, then uses DefaultChartRepoURL.
func NewChartResolver(repoURL string, settings *cli.EnvSettings) *ChartResolver {
	if repoURL == "" {
		repoURL = os.Getenv("CLAWMACHINE_CHART_REPO")
	}
	if repoURL == "" {
		repoURL = DefaultChartRepoURL
	}
	return &ChartResolver{repoURL: repoURL, settings: settings}
}

// ResolveChart returns chart bytes for the given bot type.
// It tries the remote repo first, then falls back to the embedded chart.
func (r *ChartResolver) ResolveChart(botType BotType) ([]byte, error) {
	chartName := string(botType)

	// Skip remote if configured for embedded-only (air-gapped mode)
	if r.UseEmbeddedOnly() {
		slog.Info("using embedded chart (air-gapped mode)", "chart", chartName)
		return getEmbeddedBotChart(botType)
	}

	// Try remote first
	chartBytes, err := r.fetchRemoteChart(chartName)
	if err == nil {
		slog.Info("resolved chart from remote repo", "chart", chartName, "repo", r.repoURL)
		return chartBytes, nil
	}
	slog.Warn("failed to fetch remote chart, falling back to embedded",
		"chart", chartName, "repo", r.repoURL, "error", err)

	// Fall back to embedded
	return getEmbeddedBotChart(botType)
}

// fetchRemoteChart pulls a chart from the configured repo URL.
func (r *ChartResolver) fetchRemoteChart(chartName string) ([]byte, error) {
	// Create a temp dir for the pulled chart
	tmpDir, err := os.MkdirTemp("", "clawmachine-chart-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			slog.Warn("failed to remove chart temp dir", "path", tmpDir, "error", removeErr)
		}
	}()

	chartRef := r.repoURL + "/" + chartName
	if !registry.IsOCI(r.repoURL) {
		// For HTTP repos, use repo URL + chart name pattern
		chartRef = chartName
	}

	cfg := new(action.Configuration)
	pull := action.NewPull(action.WithConfig(cfg))
	pull.Settings = r.settings
	pull.DestDir = tmpDir
	if !registry.IsOCI(r.repoURL) {
		pull.RepoURL = r.repoURL
	}

	// Set up OCI registry client if needed
	if registry.IsOCI(r.repoURL) {
		regClient, err := registry.NewClient(
			registry.ClientOptDebug(false),
			registry.ClientOptEnableCache(true),
		)
		if err != nil {
			return nil, fmt.Errorf("creating registry client: %w", err)
		}
		pull.SetRegistryClient(regClient)
	}

	output, err := pull.Run(chartRef)
	if err != nil {
		return nil, fmt.Errorf("pulling chart %q from %q: %w", chartName, r.repoURL, err)
	}
	slog.Debug("pull output", "output", output)

	// Read the pulled chart archive
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("reading temp dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			data, err := os.ReadFile(tmpDir + "/" + entry.Name())
			if err != nil {
				return nil, fmt.Errorf("reading pulled chart: %w", err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("no chart archive found in pull output")
}

// UseEmbeddedOnly returns true if the resolver is configured to skip remote.
func (r *ChartResolver) UseEmbeddedOnly() bool {
	return r.repoURL == "embedded"
}

// getEmbeddedBotChart returns the embedded chart for a bot type.
func getEmbeddedBotChart(botType BotType) ([]byte, error) {
	switch botType {
	case BotTypePicoClaw:
		return picoClawChart, nil
	case BotTypeIronClaw:
		return ironClawChart, nil
	case BotTypeOpenClaw:
		return openClawChart, nil
	case BotTypeBusyBox:
		return busyBoxChart, nil
	default:
		return nil, fmt.Errorf("unknown bot type: %s", botType)
	}
}

// GetEmbeddedChart returns the embedded chart archive for the given bot type.
// Deprecated: Use ChartResolver.ResolveChart instead for bot charts.
func GetEmbeddedChart(botType BotType) ([]byte, error) {
	return getEmbeddedBotChart(botType)
}

// GetControlPlaneChart returns the embedded control-plane chart archive.
func GetControlPlaneChart() []byte {
	return clawMachineChart
}

// GetESOChart returns the embedded External Secrets Operator chart archive.
func GetESOChart() []byte {
	return esoChart
}

// GetCiliumChart returns the embedded Cilium CNI chart archive.
func GetCiliumChart() []byte {
	return ciliumChart
}

// GetConnectChart returns the embedded 1Password Connect chart archive.
func GetConnectChart() []byte {
	return connectChart
}
