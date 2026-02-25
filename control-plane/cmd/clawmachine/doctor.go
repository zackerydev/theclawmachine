package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check cluster prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo()
			return runDoctor(cmd)
		},
	}
}

type checkResult struct {
	name   string
	ok     bool
	detail string
}

func runDoctor(cmd *cobra.Command) error {
	kubeContext, _ := cmd.Flags().GetString("context")

	header := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(purple).
		Padding(0, 2).
		Render("ClawMachine Doctor")
	fmt.Println(header)
	fmt.Println()

	var checks []checkResult

	// CLI tools
	for _, tool := range []struct{ name, desc string }{
		{"kubectl", "Kubernetes CLI"},
		{"helm", "Helm package manager"},
		{"kind", "KinD (Kubernetes in Docker)"},
		{"docker", "Docker runtime"},
	} {
		ok := cmdExists(tool.name)
		detail := ""
		if !ok {
			detail = "not found in PATH"
		}
		checks = append(checks, checkResult{tool.desc + " (" + tool.name + ")", ok, detail})
	}

	// Cluster reachable
	if cmdExists("kubectl") {
		_, err := exec.Command("kubectl", "cluster-info").CombinedOutput()
		clusterOk := err == nil
		detail := ""
		if !clusterOk {
			detail = "kubectl cluster-info failed"
		}
		checks = append(checks, checkResult{"Cluster reachable", clusterOk, detail})
	} else {
		checks = append(checks, checkResult{"Cluster reachable", false, "kubectl not available"})
	}

	infraStatuses, infraErr := collectInfraStatuses(kubeContext)
	if infraErr != nil {
		checks = append(checks, checkResult{
			name:   "Infrastructure checks",
			ok:     false,
			detail: "unable to inspect cluster infrastructure: " + infraErr.Error(),
		})
	} else {
		eso := findInfraStatus(infraStatuses, "External Secrets Operator")
		checks = append(checks, checkResult{"External Secrets Operator", eso.Healthy, doctorDetail(eso)})

		cilium := findInfraStatus(infraStatuses, "Cilium CNI")
		checks = append(checks, checkResult{"Cilium CNI (NetworkPolicy + DNS)", cilium.Healthy, doctorDetail(cilium)})

		// Optional check: informational only, does not fail doctor.
		opConnect := findInfraStatus(infraStatuses, "1Password Connect")
		checks = append(checks, checkResult{"1Password Connect Server (optional)", true, doctorOptionalDetail(opConnect)})
	}

	// Render results
	allOk := true
	for _, c := range checks {
		var icon string
		if c.ok {
			icon = passStyle.Render()
		} else {
			icon = failStyle.Render()
			allOk = false
		}
		line := fmt.Sprintf("%s %s", icon, c.name)
		fmt.Println(line)
		if c.detail != "" {
			fmt.Println(dimStyle.Render("    " + c.detail))
		}
	}

	fmt.Println()
	if allOk {
		styledPrintf(successStyle, "All checks passed!")
	} else {
		styledPrintf(errorStyle, "Some checks failed. See above for fixes.")
	}

	return nil
}

func findInfraStatus(all []infraComponentStatus, name string) infraComponentStatus {
	for _, s := range all {
		if s.Name == name {
			return s
		}
	}
	return infraComponentStatus{Name: name}
}

func doctorDetail(s infraComponentStatus) string {
	detail := strings.TrimSpace(s.Detail)
	if s.Installed && s.Namespace != "" {
		if detail == "" {
			return "namespace: " + s.Namespace
		}
		return "namespace: " + s.Namespace + " (" + detail + ")"
	}
	return detail
}

func doctorOptionalDetail(s infraComponentStatus) string {
	if !s.Installed {
		return "Optional: configure in dashboard Settings"
	}
	if s.Healthy {
		return doctorDetail(s)
	}
	detail := doctorDetail(s)
	if detail == "" {
		return "optional component installed but not ready"
	}
	return "optional: " + detail
}
