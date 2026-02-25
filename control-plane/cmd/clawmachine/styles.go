package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	green  = lipgloss.Color("#b8bb26") // gruvbox green
	purple = lipgloss.Color("#8ec07c") // gruvbox aqua (replaces purple for subtitle/border)
	gold   = lipgloss.Color("#fabd2f") // gruvbox yellow
	red    = lipgloss.Color("#fb4934") // gruvbox red

	titleStyle    = lipgloss.NewStyle().Foreground(green).Bold(true)
	subtitleStyle = lipgloss.NewStyle().Foreground(purple)
	accentStyle   = lipgloss.NewStyle().Foreground(gold)
	errorStyle    = lipgloss.NewStyle().Foreground(red).Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(green)
	passStyle     = lipgloss.NewStyle().Foreground(green).SetString("✓")
	failStyle     = lipgloss.NewStyle().Foreground(red).SetString("✗")
	dimStyle      = lipgloss.NewStyle().Faint(true)
)

const logo = `
   _____ _        ___        __ __  __            _     _            
  / ____| |      / \ \      / /|  \/  |          | |   (_)           
 | |    | |     / _ \ \ /\ / / | \  / | __ _  ___| |__  _ _ __   ___ 
 | |    | |    / ___ \ V  V /  | |\/| |/ _` + "`" + ` |/ __| '_ \| | '_ \ / _ \
 | |____| |___/ /   \ \_/\_/   | |  | | (_| | (__| | | | | | | |  __/
  \_____|______/     \___/     |_|  |_|\__,_|\___|_| |_|_|_| |_|\___|
`

func printLogo() {
	fmt.Println(titleStyle.Render(logo))
	fmt.Println(subtitleStyle.Render("  Kubernetes-native bot vending machine") + "  " + dimStyle.Render("v"+version))
	fmt.Println()
}

func styledPrintf(style lipgloss.Style, format string, a ...any) {
	fmt.Println(style.Render(fmt.Sprintf(format, a...)))
}
