package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/release"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show managed bots and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo()
			return runStatus(cmd)
		},
	}
}

func runStatus(cmd *cobra.Command) error {
	kubeContext, _ := cmd.Flags().GetString("context")
	settings := newHelmSettings(kubeContext)

	actionConfig, err := initActionConfig(settings, "")
	if err != nil {
		return err
	}

	listClient := action.NewList(actionConfig)
	listClient.AllNamespaces = true
	listClient.SetStateMask()
	results, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}
	infraStatuses, infraErr := collectInfraStatuses(kubeContext)

	header := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(purple).
		Padding(0, 2).
		Render("ClawMachine Status")
	fmt.Println(header)
	fmt.Println()

	renderInfraStatusTable(infraStatuses, infraErr)
	fmt.Println()
	renderBotStatusTable(results)

	return nil
}

func renderInfraStatusTable(statuses []infraComponentStatus, infraErr error) {
	styledPrintf(accentStyle, "Infrastructure Status")
	headerFmt := accentStyle.Render(fmt.Sprintf("  %-32s %-16s %-14s", "COMPONENT", "NAMESPACE", "STATUS"))
	fmt.Println(headerFmt)
	fmt.Println(dimStyle.Render("  " + strings.Repeat("─", 66)))

	if infraErr != nil {
		fmt.Printf("  %-32s %-16s %-14s\n", "Cluster checks", "-", errorStyle.Render("unknown"))
		fmt.Println(dimStyle.Render("    unable to inspect infrastructure health: " + infraErr.Error()))
		return
	}

	for _, s := range statuses {
		namespace := s.Namespace
		if namespace == "" {
			namespace = "-"
		}
		statusText := infraStatusText(s)
		fmt.Printf("  %-32s %-16s %-14s\n", s.Name, namespace, statusText)
		if s.Detail != "" {
			fmt.Println(dimStyle.Render("    " + s.Detail))
		}
	}
}

func renderBotStatusTable(results []release.Releaser) {
	styledPrintf(accentStyle, "ClawMachine Bots (claw-machine)")

	headerFmt := accentStyle.Render(fmt.Sprintf("  %-24s %-16s %-12s %-12s", "NAME", "NAMESPACE", "BOT TYPE", "HELM STATUS"))
	fmt.Println(headerFmt)
	fmt.Println(dimStyle.Render("  " + strings.Repeat("─", 70)))

	bots := make([]botStatusRow, 0)
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
		chartName := chartNameSafe(chartAcc)
		if !isManagedBotRelease(acc.Namespace(), chartName) {
			continue
		}
		bots = append(bots, botStatusRow{
			Name:      acc.Name(),
			Namespace: acc.Namespace(),
			BotType:   chartName,
			Status:    acc.Status(),
		})
	}

	slices.SortFunc(bots, func(a, b botStatusRow) int {
		return strings.Compare(a.Name, b.Name)
	})

	if len(bots) == 0 {
		styledPrintf(dimStyle, "No ClawMachine bot releases found in claw-machine.")
		return
	}

	for _, b := range bots {
		fmt.Printf("  %-24s %-16s %-12s %-12s\n", b.Name, b.Namespace, b.BotType, renderHelmStatus(b.Status))
	}
}

func infraStatusText(s infraComponentStatus) string {
	if !s.Installed {
		if s.Optional {
			return dimStyle.Render("not installed")
		}
		return errorStyle.Render("not installed")
	}
	if s.Healthy {
		return successStyle.Render("healthy")
	}
	return accentStyle.Render("degraded")
}

func renderHelmStatus(status string) string {
	switch status {
	case "deployed":
		return successStyle.Render(status)
	case "failed":
		return errorStyle.Render(status)
	default:
		return accentStyle.Render(status)
	}
}

func chartNameSafe(chartAcc chart.Accessor) (name string) {
	name = "unknown"
	defer func() {
		if recover() != nil {
			name = "unknown"
		}
	}()
	if n := chartAcc.Name(); n != "" {
		name = n
	}
	return name
}

type botStatusRow struct {
	Name      string
	Namespace string
	BotType   string
	Status    string
}
