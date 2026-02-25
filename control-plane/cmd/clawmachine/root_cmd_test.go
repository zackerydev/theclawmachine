package main

import "testing"

func TestNewRootCmd_RegistersExpectedCommands(t *testing.T) {
	root := newRootCmd()

	names := make(map[string]bool, len(root.Commands()))
	for _, cmd := range root.Commands() {
		names[cmd.Name()] = true
	}

	for _, expected := range []string{
		"serve",
		"install",
		"upgrade",
		"uninstall",
		"doctor",
		"status",
		"version",
		"backup",
		"restore",
		"completion",
	} {
		if !names[expected] {
			t.Fatalf("expected root command to register %q", expected)
		}
	}

	if names["tui"] {
		t.Fatal("unexpected deprecated \"tui\" command registered")
	}
}

func TestCmdExists(t *testing.T) {
	if cmdExists("clawmachine-this-command-should-never-exist-12345") {
		t.Fatal("expected cmdExists to return false for clearly invalid command")
	}

	if !cmdExists("zsh") && !cmdExists("sh") && !cmdExists("bash") {
		t.Skip("no common shell executable found in PATH")
	}
}
