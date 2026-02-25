package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var version = "dev"

func getenv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "clawmachine",
		Short: "Kubernetes-native bot vending machine",
		Long:  titleStyle.Render(logo) + "\n" + subtitleStyle.Render("  Kubernetes-native bot vending machine"),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Name() == "help" || cmd.Flags().Changed("help") {
				printLogo()
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("context", "", "Kubernetes context to use")
	root.Flags().Bool("dev", false, "Enable dev mode (template reload, imagePullPolicy=Never)")
	root.Flags().Bool("web", false, "Run web UI (default)")

	root.AddCommand(newServeCmd())
	root.AddCommand(newInstallCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newUninstallCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newBackupCmd())
	root.AddCommand(newRestoreCmd())
	root.AddCommand(newCompletionCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the ClawMachine version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(titleStyle.Render("clawmachine") + " " + accentStyle.Render("v"+version))
		},
	}
}

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for your shell.

To load completions:

Bash:
  $ source <(clawmachine completion bash)

Zsh:
  $ source <(clawmachine completion zsh)

Fish:
  $ clawmachine completion fish | source`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	return cmd
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render("Error: "+err.Error()))
		os.Exit(1)
	}
}
