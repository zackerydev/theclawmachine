package main

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// knownCharts is the set of Helm chart names that ClawMachine manages.
var knownCharts = map[string]bool{
	"clawmachine":      true,
	"picoclaw":         true,
	"ironclaw":         true,
	"openclaw":         true,
	"busybox":          true,
	"external-secrets": true,
	"cilium":           true,
	"connect":          true,
}

var (
	externalSecretGVR = schema.GroupVersionResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
	}
	secretStoreGVR = schema.GroupVersionResource{
		Group: "external-secrets.io", Version: "v1", Resource: "secretstores",
	}
)

type managedRelease struct {
	name      string
	namespace string
	chartName string
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall ClawMachine and all managed resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo()
			return runUninstall(cmd)
		},
	}
	cmd.Flags().String("namespace", "claw-machine", "Target namespace")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runUninstall(cmd *cobra.Command) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	kubeContext, _ := cmd.Flags().GetString("context")
	yes, _ := cmd.Flags().GetBool("yes")

	settings := newHelmSettings(kubeContext)

	// Discover all ClawMachine-managed releases across namespaces
	var allReleases []managedRelease
	for _, ns := range []string{namespace, esoNamespace, ciliumNamespace, opConnectNamespace} {
		releases, err := listManagedReleases(settings, ns)
		if err != nil {
			continue
		}
		allReleases = append(allReleases, releases...)
	}

	// Count managed ESO CRD instances
	var esoResources []string
	dynClient, dynErr := newDynamicClient(kubeContext)
	if dynErr == nil {
		for _, gvr := range []schema.GroupVersionResource{externalSecretGVR, secretStoreGVR} {
			list, err := dynClient.Resource(gvr).Namespace(namespace).List(
				context.Background(), metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/managed-by=clawmachine",
				},
			)
			if err == nil {
				for _, item := range list.Items {
					esoResources = append(esoResources, gvr.Resource+"/"+item.GetName())
				}
			}
		}
	}

	if len(allReleases) == 0 && len(esoResources) == 0 {
		styledPrintf(dimStyle, "Nothing managed by ClawMachine found.")
		return nil
	}

	// Show what will be removed
	styledPrintf(errorStyle, "The following will be permanently removed:")
	fmt.Println()
	for _, r := range allReleases {
		fmt.Printf("  %s %s %s\n", failStyle.Render(), r.name, dimStyle.Render(fmt.Sprintf("(%s in %s)", r.chartName, r.namespace)))
	}
	for _, name := range esoResources {
		fmt.Printf("  %s %s\n", failStyle.Render(), name)
	}
	fmt.Println()

	// Confirm unless --yes
	if !yes {
		var confirm bool
		err := huh.NewConfirm().
			Title("Are you sure?").
			Description("This will remove ClawMachine and all managed resources.").
			Affirmative("Yes, uninstall everything").
			Negative("Cancel").
			Value(&confirm).
			Run()
		if err != nil {
			return fmt.Errorf("prompt error: %w", err)
		}
		if !confirm {
			styledPrintf(dimStyle, "Uninstall cancelled.")
			return nil
		}
	}

	// Delete ESO CRD instances first while the controller is still running
	if dynErr == nil && len(esoResources) > 0 {
		ctx := context.Background()
		for _, gvr := range []schema.GroupVersionResource{externalSecretGVR, secretStoreGVR} {
			list, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/managed-by=clawmachine",
			})
			if err != nil {
				continue
			}
			for _, item := range list.Items {
				styledPrintf(accentStyle, "Deleting %s/%s...", gvr.Resource, item.GetName())
				if err := dynClient.Resource(gvr).Namespace(namespace).Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil {
					styledPrintf(errorStyle, "  failed: %v", err)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	// Uninstall bot releases first (non-infrastructure)
	infraCharts := map[string]bool{
		"external-secrets": true,
		"cilium":           true,
		"connect":          true,
	}
	var failed int
	for _, r := range allReleases {
		if infraCharts[r.chartName] {
			continue
		}
		styledPrintf(accentStyle, "Uninstalling %s (%s)...", r.name, r.namespace)
		if err := uninstallHelmRelease(settings, r.namespace, r.name); err != nil {
			styledPrintf(errorStyle, "  failed: %v", err)
			failed++
		} else {
			styledPrintf(successStyle, "  done")
		}
	}

	// Uninstall infrastructure components with dedicated handlers
	for _, uninstallFn := range []func(*cli.EnvSettings) error{
		uninstall1PasswordConnect,
		uninstallESO,
		uninstallCilium,
	} {
		if err := uninstallFn(settings); err != nil {
			styledPrintf(errorStyle, "  failed: %v", err)
			failed++
		}
	}

	fmt.Println()
	if failed > 0 {
		styledPrintf(errorStyle, "Uninstall completed with %d error(s).", failed)
	} else {
		styledPrintf(successStyle, "ClawMachine has been fully uninstalled.")
	}
	return nil
}

func listManagedReleases(settings *cli.EnvSettings, namespace string) ([]managedRelease, error) {
	actionConfig, err := initActionConfig(settings, namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewList(actionConfig)
	client.All = true

	results, err := client.Run()
	if err != nil {
		return nil, err
	}

	var out []managedRelease
	for _, r := range results {
		acc, err := release.NewAccessor(r)
		if err != nil {
			continue
		}
		chrt := acc.Chart()
		if chrt == nil {
			continue
		}
		chartAcc, err := chart.NewDefaultAccessor(chrt)
		if err != nil {
			continue
		}
		if knownCharts[chartAcc.Name()] {
			out = append(out, managedRelease{
				name:      acc.Name(),
				namespace: namespace,
				chartName: chartAcc.Name(),
			})
		}
	}
	return out, nil
}

func newDynamicClient(kubeContext string) (dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}
