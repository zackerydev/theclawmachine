package main

import "testing"

func TestNewInstallCmd_DoesNotExposeOnePasswordConnectFlag(t *testing.T) {
	cmd := newInstallCmd()

	if got := cmd.Flags().Lookup("1password-connect"); got != nil {
		t.Fatalf("unexpected deprecated flag present: %s", got.Name)
	}
}

func TestNewInstallCmd_HasExpectedFlags(t *testing.T) {
	cmd := newInstallCmd()

	for _, name := range []string{
		"namespace",
		"name",
		"external-secrets",
		"cilium",
		"create-kind-cluster",
		"interactive",
		"yes",
	} {
		if got := cmd.Flags().Lookup(name); got == nil {
			t.Fatalf("expected flag %q to be present", name)
		}
	}
}
