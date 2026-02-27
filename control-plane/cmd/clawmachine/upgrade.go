package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
	versionutil "github.com/zackerydev/clawmachine/control-plane/internal/version"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
)

var (
	loadControlPlaneChartForUpgrade = func() (chart.Charter, error) {
		return loader.LoadArchive(bytes.NewReader(service.GetControlPlaneChart()))
	}
	initActionConfigForUpgrade = initActionConfig
	listReleasesForUpgrade     = func(actionConfig *action.Configuration, releaseName string) ([]release.Releaser, error) {
		client := action.NewList(actionConfig)
		client.All = true
		client.Filter = releaseName
		return client.Run()
	}
	runHelmUpgradeForUpgrade = func(actionConfig *action.Configuration, namespace, releaseName string, chrt chart.Charter, vals map[string]any) error {
		client := action.NewUpgrade(actionConfig)
		client.Namespace = namespace
		client.WaitStrategy = kube.StatusWatcherStrategy
		client.Timeout = 5 * time.Minute
		client.ReuseValues = true
		_, err := client.Run(releaseName, chrt, vals)
		return err
	}
)

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade ClawMachine control plane to the bundled image",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo()
			return runUpgrade(cmd)
		},
	}

	cmd.Flags().String("namespace", "claw-machine", "Target namespace")
	cmd.Flags().String("name", "clawmachine", "Helm release name")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runUpgrade(cmd *cobra.Command) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	releaseName, _ := cmd.Flags().GetString("name")
	kubeContext, _ := cmd.Flags().GetString("context")
	yes, _ := cmd.Flags().GetBool("yes")

	chrt, err := loadControlPlaneChartForUpgrade()
	if err != nil {
		return fmt.Errorf("loading embedded chart: %w", err)
	}

	repo, tag, fallback, err := resolveBundledImage(chrt, version)
	if err != nil {
		return err
	}

	settings := newHelmSettings(kubeContext)
	actionConfig, err := initActionConfigForUpgrade(settings, namespace)
	if err != nil {
		return err
	}

	exists, err := releaseExistsForUpgrade(actionConfig, releaseName)
	if err != nil {
		return fmt.Errorf("checking release status: %w", err)
	}
	if !exists {
		return fmt.Errorf("release %q not found in namespace %q; run \"clawmachine install\" first", releaseName, namespace)
	}

	if !yes {
		var confirm bool
		err := huh.NewConfirm().
			Title("Upgrade ClawMachine control plane?").
			Description(fmt.Sprintf("Release: %s/%s\nImage: %s:%s", namespace, releaseName, repo, tag)).
			Affirmative("Yes, upgrade").
			Negative("Cancel").
			Value(&confirm).
			Run()
		if err != nil {
			return fmt.Errorf("prompt error: %w", err)
		}
		if !confirm {
			styledPrintf(dimStyle, "Upgrade cancelled.")
			return nil
		}
	}

	if fallback {
		styledPrintf(dimStyle, "CLI version %q resolved to dev mode; using chart appVersion %q.", version, tag)
	}

	styledPrintf(accentStyle, "Upgrading ClawMachine %q in namespace %q to %s:%s...", releaseName, namespace, repo, tag)
	if err := runHelmUpgradeForUpgrade(actionConfig, namespace, releaseName, chrt, upgradeOverrides(repo, tag)); err != nil {
		return fmt.Errorf("upgrading release: %w", err)
	}

	styledPrintf(successStyle, "ClawMachine upgraded successfully!")
	return nil
}

func releaseExistsForUpgrade(actionConfig *action.Configuration, releaseName string) (bool, error) {
	results, err := listReleasesForUpgrade(actionConfig, releaseName)
	if err != nil {
		return false, err
	}

	for _, r := range results {
		acc, err := release.NewAccessor(r)
		if err != nil {
			continue
		}
		if acc.Name() == releaseName {
			return true, nil
		}
	}
	return false, nil
}

func resolveBundledImage(chrt chart.Charter, cliVersion string) (repo, tag string, usedFallback bool, err error) {
	if chrt == nil {
		return "", "", false, fmt.Errorf("embedded chart is nil")
	}

	chrtAcc, err := chart.NewDefaultAccessor(chrt)
	if err != nil {
		return "", "", false, fmt.Errorf("reading chart metadata: %w", err)
	}

	repo, err = chartImageRepository(chrtAcc.Values())
	if err != nil {
		return "", "", false, fmt.Errorf("resolving bundled image repository: %w", err)
	}

	appVersion := ""
	if withAppVersion, ok := chrt.(interface{ AppVersion() string }); ok {
		appVersion = strings.TrimSpace(withAppVersion.AppVersion())
	}
	if appVersion == "" {
		metadata := chrtAcc.MetadataAsMap()
		appVersion = strings.TrimSpace(fmt.Sprintf("%v", metadata["appVersion"]))
		if appVersion == "" {
			appVersion = strings.TrimSpace(fmt.Sprintf("%v", metadata["AppVersion"]))
		}
	}
	tag, usedFallback, err = normalizeImageTag(cliVersion, appVersion)
	if err != nil {
		return "", "", false, fmt.Errorf("resolving image tag: %w", err)
	}

	return repo, tag, usedFallback, nil
}

func chartImageRepository(values map[string]any) (string, error) {
	if values == nil {
		return "", fmt.Errorf("chart values are empty")
	}

	imageRaw, ok := values["image"]
	if !ok {
		return "", fmt.Errorf("chart values missing image block")
	}

	image, ok := imageRaw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("chart image block has unexpected type %T", imageRaw)
	}

	repoRaw, ok := image["repository"]
	if !ok {
		return "", fmt.Errorf("chart image block missing repository")
	}

	repo := strings.TrimSpace(fmt.Sprintf("%v", repoRaw))
	if repo == "" {
		return "", fmt.Errorf("chart image repository is empty")
	}
	return repo, nil
}

func normalizeImageTag(cliVersion, chartAppVersion string) (tag string, usedFallback bool, err error) {
	return versionutil.ResolveRuntimeOrFallbackImageTag(cliVersion, chartAppVersion)
}

func upgradeOverrides(repo, tag string) map[string]any {
	return map[string]any{
		"image": map[string]any{
			"repository": repo,
			"tag":        tag,
		},
	}
}
