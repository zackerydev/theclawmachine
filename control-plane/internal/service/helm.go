package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ReleaseInfo contains summary information about a Helm release.
type ReleaseInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Version   int    `json:"version"`
	BotType   string `json:"botType"`
	Updated   string `json:"updated"`
}

// InstallOptions contains parameters for installing a bot release.
type InstallOptions struct {
	ReleaseName       string            `json:"releaseName"`
	BotType           BotType           `json:"botType"`
	Namespace         string            `json:"namespace,omitempty"`
	Values            map[string]any    `json:"values"`
	ConfigFields      map[string]string `json:"configFields,omitempty"`
	OnboardingVersion string            `json:"onboardingVersion,omitempty"`
}

// HelmService manages Helm releases for Claw bots.
type HelmService struct {
	settings  *cli.EnvSettings
	clientset kubernetes.Interface
	dev       bool
}

// NewHelmService creates a new HelmService configured with the given kubeconfig.
// When inCluster is true, kubeconfig settings are left empty so the Helm SDK
// uses the in-cluster service account token.
// When dev is true, installed charts use imagePullPolicy=Never.
func NewHelmService(kubeConfigPath, kubeContext string, inCluster, dev bool, clientset kubernetes.Interface) *HelmService {
	settings := cli.New()
	if !inCluster {
		settings.KubeConfig = kubeConfigPath
		if kubeContext != "" {
			settings.KubeContext = kubeContext
		}
	}
	return &HelmService{settings: settings, clientset: clientset, dev: dev}
}

func (h *HelmService) initActionConfig(namespace string) (*action.Configuration, error) {
	if namespace == "" {
		namespace = "claw-machine"
	}
	// Keep Helm's EnvSettings namespace in sync with operation scope so
	// namespaced resources are applied/watched in the intended namespace.
	h.settings.SetNamespace(namespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(h.settings.RESTClientGetter(), namespace, "secrets"); err != nil {
		return nil, fmt.Errorf("initializing helm action config: %w", err)
	}
	return actionConfig, nil
}

func loadBotChart(botType BotType) (chart.Charter, error) {
	chartBytes, err := GetEmbeddedChart(botType)
	if err != nil {
		return nil, fmt.Errorf("resolving chart: %w", err)
	}
	return loader.LoadArchive(bytes.NewReader(chartBytes))
}

// Install installs a new bot release into the cluster.
func (h *HelmService) Install(ctx context.Context, opts InstallOptions) (*ReleaseInfo, error) {
	chrt, err := loadBotChart(opts.BotType)
	if err != nil {
		return nil, err
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = "claw-machine"
	}

	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewInstall(cfg)
	client.ReleaseName = opts.ReleaseName
	client.Namespace = namespace
	// Use hook-only wait so install returns immediately after resources are applied.
	// This avoids blocking on workload readiness while still handling Helm hooks.
	client.WaitStrategy = kube.HookOnlyStrategy

	vals := opts.Values
	if vals == nil {
		vals = make(map[string]any)
	}
	if h.dev {
		img, _ := vals["image"].(map[string]any)
		if img == nil {
			img = make(map[string]any)
		}
		img["pullPolicy"] = "Never"
		vals["image"] = img
	}

	rel, err := client.RunWithContext(ctx, chrt, vals)
	if err != nil {
		return nil, fmt.Errorf("installing release %q: %w", opts.ReleaseName, err)
	}

	slog.Info("installed release", "name", opts.ReleaseName, "namespace", namespace, "botType", opts.BotType)
	return releaseToInfo(rel, string(opts.BotType))
}

// Upgrade upgrades an existing bot release.
func (h *HelmService) Upgrade(ctx context.Context, name, namespace string, botType BotType, values map[string]any) (*ReleaseInfo, error) {
	chrt, err := loadBotChart(botType)
	if err != nil {
		return nil, err
	}

	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewUpgrade(cfg)
	client.Namespace = namespace
	client.WaitStrategy = kube.StatusWatcherStrategy
	client.Timeout = 5 * time.Minute

	if values == nil {
		values = make(map[string]any)
	}

	rel, err := client.RunWithContext(ctx, name, chrt, values)
	if err != nil {
		return nil, fmt.Errorf("upgrading release %q: %w", name, err)
	}

	slog.Info("upgraded release", "name", name, "namespace", namespace, "botType", botType)
	return releaseToInfo(rel, string(botType))
}

// Uninstall removes a bot release from the cluster.
func (h *HelmService) Uninstall(name, namespace string) error {
	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return err
	}

	client := action.NewUninstall(cfg)
	// Use hook-only wait so uninstall returns quickly and the UI can show
	// transient "uninstalling" state instead of blocking.
	client.WaitStrategy = kube.HookOnlyStrategy

	_, err = client.Run(name)
	if err != nil {
		return fmt.Errorf("uninstalling release %q: %w", name, err)
	}

	slog.Info("uninstalled release", "name", name, "namespace", namespace)
	return nil
}

// List returns all bot releases in the given namespace.
func (h *HelmService) List(namespace string) ([]ReleaseInfo, error) {
	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewList(cfg)
	client.Deployed = true
	client.Pending = true
	client.Uninstalling = true

	results, err := client.Run()
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}

	releases := make([]ReleaseInfo, 0, len(results))
	for _, rel := range results {
		info, err := releaseToInfo(rel, "")
		if err != nil {
			slog.Warn("skipping release", "error", err)
			continue
		}
		releases = append(releases, *info)
	}
	return releases, nil
}

// Status returns the status of a specific release.
func (h *HelmService) Status(name, namespace string) (*ReleaseInfo, error) {
	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewStatus(cfg)
	rel, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("getting status for %q: %w", name, err)
	}

	return releaseToInfo(rel, "")
}

// GetPodLogs returns the last N lines of logs from the first pod matching the release.
func (h *HelmService) GetPodLogs(ctx context.Context, name, namespace string, lines int64) (string, error) {
	pods, err := h.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=" + name,
	})
	if err != nil {
		return "", fmt.Errorf("listing pods for %q: %w", name, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for release %q", name)
	}

	pod := pods.Items[0]
	container := selectLogContainer(pod, name)
	opts := &corev1.PodLogOptions{TailLines: &lines, Container: container}
	stream, err := h.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, opts).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("streaming logs for pod %q: %w", pod.Name, err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			slog.Warn("failed to close pod log stream", "pod", pod.Name, "namespace", namespace, "error", closeErr)
		}
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	return buf.String(), nil
}

// RestartPod deletes the first pod matching the release so K8s recreates it.
func (h *HelmService) RestartPod(ctx context.Context, name, namespace string) error {
	pods, err := h.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=" + name,
	})
	if err != nil {
		return fmt.Errorf("listing pods for %q: %w", name, err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for release %q", name)
	}

	pod := pods.Items[0]
	if err := h.clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pod %q: %w", pod.Name, err)
	}

	slog.Info("restarted pod", "pod", pod.Name, "release", name, "namespace", namespace)
	return nil
}

// GetValues returns the user-configured Helm values for a release.
func (h *HelmService) GetValues(name, namespace string) (map[string]any, error) {
	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewGetValues(cfg)
	vals, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("getting values for %q: %w", name, err)
	}
	return vals, nil
}

// GetValuesAll returns merged Helm values for a release including chart defaults.
func (h *HelmService) GetValuesAll(name, namespace string) (map[string]any, error) {
	cfg, err := h.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewGetValues(cfg)
	client.AllValues = true
	vals, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("getting all values for %q: %w", name, err)
	}
	return vals, nil
}

func releaseToInfo(rel release.Releaser, botType string) (*ReleaseInfo, error) {
	acc, err := release.NewAccessor(rel)
	if err != nil {
		return nil, fmt.Errorf("accessing release data: %w", err)
	}

	if botType == "" {
		botType = detectBotType(acc)
	}

	return &ReleaseInfo{
		Name:      acc.Name(),
		Namespace: acc.Namespace(),
		Status:    acc.Status(),
		Version:   acc.Version(),
		BotType:   botType,
		Updated:   acc.DeployedAt().Format(time.RFC3339),
	}, nil
}

func detectBotType(acc release.Accessor) (botType string) {
	botType = "unknown"

	chrt := acc.Chart()
	if chrt == nil {
		return botType
	}

	chartAcc, err := chart.NewDefaultAccessor(chrt)
	if err != nil {
		return botType
	}

	// Helm v4 chart accessors can panic when chart metadata is nil.
	defer func() {
		if recover() != nil {
			botType = "unknown"
		}
	}()
	if name := chartAcc.Name(); name != "" {
		botType = name
	}
	return botType
}
