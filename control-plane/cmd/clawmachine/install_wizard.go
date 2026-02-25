package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	installTargetCreateKind = "__create_kind__"
)

func runInstallWizard(plan installPlan) (installPlan, error) {
	contexts, currentContext, err := kubeContexts()
	if err != nil {
		styledPrintf(dimStyle, "Unable to read kube contexts: %v", err)
	}

	target, err := promptInstallTarget(contexts, currentContext, plan.KubeContext)
	if err != nil {
		return plan, err
	}

	if target == installTargetCreateKind {
		clusterName := plan.KindClusterName
		if clusterName == "" {
			clusterName = defaultKindClusterName
		}
		if err := huh.NewInput().
			Title("Kind cluster name").
			Description("A new local cluster will be created before install.").
			Value(&clusterName).
			Validate(validateClusterName).
			Run(); err != nil {
			return plan, fmt.Errorf("prompt error: %w", err)
		}
		plan.CreateKindCluster = true
		plan.KindClusterName = strings.TrimSpace(clusterName)
		plan.KubeContext = kindContextName(plan.KindClusterName)
	} else {
		plan.CreateKindCluster = false
		plan.KindClusterName = ""
		plan.KubeContext = target
	}

	if err := huh.NewConfirm().
		Title("Enable Cilium CNI for network restriction?").
		Description("Required for DNS-aware egress policy enforcement.").
		Affirmative("Yes").
		Negative("No").
		Value(&plan.InstallCilium).
		Run(); err != nil {
		return plan, fmt.Errorf("prompt error: %w", err)
	}

	if err := huh.NewConfirm().
		Title("Install External Secrets Operator?").
		Description("Required to configure secret providers from the web UI.").
		Affirmative("Yes").
		Negative("No").
		Value(&plan.InstallExternalSecrets).
		Run(); err != nil {
		return plan, fmt.Errorf("prompt error: %w", err)
	}

	printInstallPlan(plan)

	confirmed, err := promptInstallConfirmation("Install with these settings?")
	if err != nil {
		return plan, err
	}
	if !confirmed {
		return plan, errors.New("install cancelled")
	}

	return plan, nil
}

func promptInstallTarget(contexts []string, currentContext, preferredContext string) (string, error) {
	if len(contexts) == 0 {
		createKind := true
		if err := huh.NewConfirm().
			Title("No kube contexts found. Create a new kind cluster?").
			Affirmative("Yes").
			Negative("No").
			Value(&createKind).
			Run(); err != nil {
			return "", fmt.Errorf("prompt error: %w", err)
		}
		if !createKind {
			return "", errors.New("install cancelled")
		}
		return installTargetCreateKind, nil
	}

	target := installTargetCreateKind
	if preferredContext != "" && containsString(contexts, preferredContext) {
		target = preferredContext
	} else if currentContext != "" && containsString(contexts, currentContext) {
		target = currentContext
	}

	options := []huh.Option[string]{
		huh.NewOption("Create new kind cluster", installTargetCreateKind),
	}
	for _, ctx := range contexts {
		label := "Use context: " + ctx
		if ctx == currentContext {
			label += " (current)"
		}
		options = append(options, huh.NewOption(label, ctx))
	}

	if err := huh.NewSelect[string]().
		Title("Select target cluster").
		Options(options...).
		Value(&target).
		Run(); err != nil {
		return "", fmt.Errorf("prompt error: %w", err)
	}

	return target, nil
}

func promptInstallConfirmation(title string) (bool, error) {
	confirmed := false
	if err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		Run(); err != nil {
		return false, fmt.Errorf("prompt error: %w", err)
	}
	return confirmed, nil
}

func printInstallPlan(plan installPlan) {
	styledPrintf(accentStyle, "Install plan:")
	if plan.CreateKindCluster {
		fmt.Printf("  Kind cluster: %s\n", plan.KindClusterName)
		fmt.Printf("  Context: %s\n", plan.KubeContext)
	} else {
		fmt.Printf("  Context: %s\n", valueOrDefault(plan.KubeContext, "(current)"))
	}
	fmt.Printf("  Namespace: %s\n", plan.Namespace)
	fmt.Printf("  Release: %s\n", plan.ReleaseName)
	fmt.Printf("  Cilium: %t\n", plan.InstallCilium)
	fmt.Printf("  External Secrets Operator: %t\n", plan.InstallExternalSecrets)
	fmt.Println()
}

func kubeContexts() ([]string, string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	raw, err := cfg.RawConfig()
	if err != nil {
		return nil, "", fmt.Errorf("reading kubeconfig: %w", err)
	}

	contexts := make([]string, 0, len(raw.Contexts))
	for name := range raw.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)

	return contexts, raw.CurrentContext, nil
}

func containsString(values []string, value string) bool {
	return slices.Contains(values, value)
}

func validateClusterName(value string) error {
	name := strings.TrimSpace(value)
	if name == "" {
		return errors.New("cluster name is required")
	}
	if strings.ContainsAny(name, " \t\n") {
		return errors.New("cluster name must not contain whitespace")
	}
	return nil
}

func isTerminalSession() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stdinInfo.Mode()&os.ModeCharDevice) != 0 &&
		(stdoutInfo.Mode()&os.ModeCharDevice) != 0
}
