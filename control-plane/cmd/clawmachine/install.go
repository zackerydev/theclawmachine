package main

import (
	"bytes"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/kube"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

type installPlan struct {
	Namespace              string
	ReleaseName            string
	KubeContext            string
	InstallExternalSecrets bool
	InstallCilium          bool
	CreateKindCluster      bool
	KindClusterName        string
}

const defaultKindClusterName = "claw-machine"

var (
	loadControlPlaneChartForInstall = func() (chart.Charter, error) {
		return loader.LoadArchive(bytes.NewReader(service.GetControlPlaneChart()))
	}
	initActionConfigForInstall = initActionConfig
	runHelmInstallForInstall   = func(actionConfig *action.Configuration, releaseName, namespace string, chrt chart.Charter, vals map[string]any) error {
		client := action.NewInstall(actionConfig)
		client.ReleaseName = releaseName
		client.Namespace = namespace
		client.CreateNamespace = true
		client.WaitStrategy = kube.StatusWatcherStrategy
		client.Timeout = 5 * time.Minute
		_, err := client.Run(chrt, vals)
		return err
	}
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install ClawMachine to a Kubernetes cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo()
			return runInstall(cmd)
		},
	}
	cmd.Flags().String("namespace", "claw-machine", "Target namespace")
	cmd.Flags().String("name", "clawmachine", "Helm release name")
	cmd.Flags().Bool("external-secrets", false, "Also install External Secrets Operator")
	cmd.Flags().Bool("cilium", false, "Also install Cilium CNI for NetworkPolicy enforcement")
	cmd.Flags().Bool("create-kind-cluster", false, "Create a local kind cluster named claw-machine before install")
	cmd.Flags().Bool("interactive", true, "Run interactive installer prompts")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runInstall(cmd *cobra.Command) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	name, _ := cmd.Flags().GetString("name")
	kubeContext, _ := cmd.Flags().GetString("context")
	extSecrets, _ := cmd.Flags().GetBool("external-secrets")
	cilium, _ := cmd.Flags().GetBool("cilium")
	createKindClusterFlag, _ := cmd.Flags().GetBool("create-kind-cluster")
	interactive, _ := cmd.Flags().GetBool("interactive")
	yes, _ := cmd.Flags().GetBool("yes")

	plan := installPlan{
		Namespace:              namespace,
		ReleaseName:            name,
		KubeContext:            kubeContext,
		InstallExternalSecrets: extSecrets,
		InstallCilium:          cilium,
	}

	if shouldRunInstallWizard(cmd, interactive, yes) {
		var err error
		plan, err = runInstallWizard(plan)
		if err != nil {
			return err
		}
	}
	if createKindClusterFlag {
		plan.CreateKindCluster = true
		plan.KindClusterName = defaultKindClusterName
		plan.KubeContext = kindContextName(defaultKindClusterName)
	}

	if plan.CreateKindCluster {
		if err := createKindCluster(plan.KindClusterName, plan.InstallCilium); err != nil {
			return err
		}
		plan.KubeContext = kindContextName(plan.KindClusterName)
	}

	settings := newHelmSettings(plan.KubeContext)

	if plan.InstallCilium {
		if err := installCilium(settings); err != nil {
			return err
		}
	}

	if plan.InstallExternalSecrets {
		if err := installESO(settings); err != nil {
			return err
		}
	}

	chrt, err := loadControlPlaneChartForInstall()
	if err != nil {
		return fmt.Errorf("loading embedded chart: %w", err)
	}

	repo, tag, fallback, err := resolveBundledImage(chrt, version)
	if err != nil {
		return err
	}

	if fallback {
		styledPrintf(dimStyle, "CLI version %q resolved to dev mode; using chart appVersion %q.", version, tag)
	}

	actionConfig, err := initActionConfigForInstall(settings, namespace)
	if err != nil {
		return err
	}

	styledPrintf(accentStyle, "Installing ClawMachine into namespace %q using %s:%s...", plan.Namespace, repo, tag)
	if err := runHelmInstallForInstall(actionConfig, plan.ReleaseName, plan.Namespace, chrt, upgradeOverrides(repo, tag)); err != nil {
		return fmt.Errorf("installing release: %w", err)
	}

	styledPrintf(successStyle, "ClawMachine installed successfully!")
	fmt.Println()
	fmt.Println(accentStyle.Render("  Access the dashboard:"))
	fmt.Printf("  kubectl port-forward -n %s svc/clawmachine 8080:80\n", plan.Namespace)
	fmt.Println("  open http://localhost:8080")
	return nil
}

func shouldRunInstallWizard(cmd *cobra.Command, interactive, yes bool) bool {
	if !interactive || yes || !isTerminalSession() {
		return false
	}

	return !slices.ContainsFunc([]string{
		"context",
		"namespace",
		"name",
		"external-secrets",
		"cilium",
		"create-kind-cluster",
	}, cmd.Flags().Changed)
}

func valueOrDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
