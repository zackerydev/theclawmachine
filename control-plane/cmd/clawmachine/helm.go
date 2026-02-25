package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
)

const (
	esoReleaseName = "external-secrets"
	esoNamespace   = "external-secrets"

	ciliumReleaseName = "cilium"
	ciliumNamespace   = "kube-system"

	opConnectReleaseName = "connect"
	opConnectNamespace   = "1password"
)

func newHelmSettings(kubeContext string) *cli.EnvSettings {
	settings := cli.New()
	if kubeContext != "" {
		settings.KubeContext = kubeContext
	}
	return settings
}

func initActionConfig(settings *cli.EnvSettings, namespace string) (*action.Configuration, error) {
	settings.SetNamespace(namespace)
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "secrets"); err != nil {
		return nil, fmt.Errorf("initializing helm: %w", err)
	}
	return actionConfig, nil
}

// ── ESO ─────────────────────────────────────────────────────────────────────

func installESO(settings *cli.EnvSettings) error {
	styledPrintf(accentStyle, "Installing External Secrets Operator into namespace %q...", esoNamespace)

	actionConfig, err := initActionConfig(settings, esoNamespace)
	if err != nil {
		return err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = esoReleaseName
	client.Namespace = esoNamespace
	client.CreateNamespace = true
	client.WaitStrategy = kube.StatusWatcherStrategy
	client.Timeout = 5 * time.Minute

	chrt, err := loader.LoadArchive(bytes.NewReader(service.GetESOChart()))
	if err != nil {
		return fmt.Errorf("loading external-secrets chart: %w", err)
	}

	if _, err := client.Run(chrt, nil); err != nil {
		return fmt.Errorf("installing external-secrets: %w", err)
	}

	styledPrintf(successStyle, "External Secrets Operator installed successfully.")
	return nil
}

func uninstallESO(settings *cli.EnvSettings) error {
	actionConfig, err := initActionConfig(settings, esoNamespace)
	if err != nil {
		return err
	}

	listClient := action.NewList(actionConfig)
	listClient.Filter = esoReleaseName
	results, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}
	if len(results) == 0 {
		styledPrintf(dimStyle, "External Secrets Operator not found, skipping.")
		return nil
	}

	styledPrintf(accentStyle, "Uninstalling External Secrets Operator...")
	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.WaitStrategy = kube.StatusWatcherStrategy
	uninstallClient.Timeout = 2 * time.Minute
	if _, err := uninstallClient.Run(esoReleaseName); err != nil {
		return fmt.Errorf("uninstalling external-secrets: %w", err)
	}
	styledPrintf(successStyle, "External Secrets Operator uninstalled.")
	return nil
}

// ── Cilium ──────────────────────────────────────────────────────────────────

func installCilium(settings *cli.EnvSettings) error {
	styledPrintf(accentStyle, "Installing Cilium CNI into namespace %q...", ciliumNamespace)

	actionConfig, err := initActionConfig(settings, ciliumNamespace)
	if err != nil {
		return err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = ciliumReleaseName
	client.Namespace = ciliumNamespace
	client.CreateNamespace = true
	client.WaitStrategy = kube.StatusWatcherStrategy
	client.Timeout = 10 * time.Minute

	chrt, err := loader.LoadArchive(bytes.NewReader(service.GetCiliumChart()))
	if err != nil {
		return fmt.Errorf("loading cilium chart: %w", err)
	}

	vals := map[string]any{
		"operator": map[string]any{
			"replicas": 1,
		},
		"hubble": map[string]any{
			"relay": map[string]any{"enabled": true},
			"ui":    map[string]any{"enabled": true},
		},
	}
	// When running on kind with disableDefaultCNI=true, ClusterIP routing
	// is not available until Cilium is up, so use the control-plane IP.
	if host := getKindControlPlaneIP(settings.KubeContext); host != "" {
		vals["k8sServiceHost"] = host
		vals["k8sServicePort"] = "6443"
	}

	if _, err := client.Run(chrt, vals); err != nil {
		return fmt.Errorf("installing cilium: %w", err)
	}

	styledPrintf(successStyle, "Cilium CNI installed successfully.")
	return nil
}

// getKindControlPlaneIP returns the kind control-plane node IP for the selected context.
// Returns empty string when the context is not a kind cluster or detection fails.
func getKindControlPlaneIP(kubeContext string) string {
	clusterName, resolvedContext := resolveKindClusterContext(kubeContext)
	if clusterName == "" {
		return ""
	}

	// Try docker container first: <cluster>-control-plane.
	out, err := exec.Command("docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
		clusterName+"-control-plane").Output()
	if err == nil {
		if ip := strings.TrimSpace(string(out)); ip != "" {
			return ip
		}
	}

	// Fallback to node InternalIP via kubectl for the selected context.
	args := []string{}
	if resolvedContext != "" {
		args = append(args, "--context", resolvedContext)
	}
	args = append(args, "get", "nodes", "-o",
		"jsonpath={.items[0].status.addresses[?(@.type=='InternalIP')].address}")
	out, err = exec.Command("kubectl", args...).Output()
	if err == nil {
		if ip := strings.TrimSpace(string(out)); ip != "" {
			return ip
		}
	}

	return ""
}

func resolveKindClusterContext(kubeContext string) (clusterName, resolvedContext string) {
	resolvedContext = strings.TrimSpace(kubeContext)
	if resolvedContext == "" {
		out, err := exec.Command("kubectl", "config", "current-context").Output()
		if err != nil {
			return "", ""
		}
		resolvedContext = strings.TrimSpace(string(out))
	}
	if !strings.HasPrefix(resolvedContext, "kind-") {
		return "", resolvedContext
	}
	return strings.TrimPrefix(resolvedContext, "kind-"), resolvedContext
}

func uninstallCilium(settings *cli.EnvSettings) error {
	actionConfig, err := initActionConfig(settings, ciliumNamespace)
	if err != nil {
		return err
	}

	listClient := action.NewList(actionConfig)
	listClient.Filter = ciliumReleaseName
	results, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}
	if len(results) == 0 {
		styledPrintf(dimStyle, "Cilium CNI not found, skipping.")
		return nil
	}

	styledPrintf(accentStyle, "Uninstalling Cilium CNI...")
	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.WaitStrategy = kube.StatusWatcherStrategy
	uninstallClient.Timeout = 2 * time.Minute
	if _, err := uninstallClient.Run(ciliumReleaseName); err != nil {
		return fmt.Errorf("uninstalling cilium: %w", err)
	}
	styledPrintf(successStyle, "Cilium CNI uninstalled.")
	return nil
}

// ── 1Password Connect ───────────────────────────────────────────────────────

func uninstall1PasswordConnect(settings *cli.EnvSettings) error {
	actionConfig, err := initActionConfig(settings, opConnectNamespace)
	if err != nil {
		return err
	}

	listClient := action.NewList(actionConfig)
	listClient.Filter = opConnectReleaseName
	results, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}
	if len(results) == 0 {
		styledPrintf(dimStyle, "1Password Connect Server not found, skipping.")
		return nil
	}

	styledPrintf(accentStyle, "Uninstalling 1Password Connect Server...")
	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.WaitStrategy = kube.StatusWatcherStrategy
	uninstallClient.Timeout = 2 * time.Minute
	if _, err := uninstallClient.Run(opConnectReleaseName); err != nil {
		return fmt.Errorf("uninstalling 1password-connect: %w", err)
	}
	styledPrintf(successStyle, "1Password Connect Server uninstalled.")
	return nil
}

// ── Generic helpers ─────────────────────────────────────────────────────────

func uninstallHelmRelease(settings *cli.EnvSettings, namespace, name string) error {
	actionConfig, err := initActionConfig(settings, namespace)
	if err != nil {
		return err
	}
	client := action.NewUninstall(actionConfig)
	client.WaitStrategy = kube.StatusWatcherStrategy
	client.Timeout = 5 * time.Minute
	_, err = client.Run(name)
	return err
}
