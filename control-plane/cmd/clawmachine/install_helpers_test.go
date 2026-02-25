package main

import (
	"os"
	"strings"
	"testing"
)

func TestKindContextName(t *testing.T) {
	if got, want := kindContextName("demo"), "kind-demo"; got != want {
		t.Fatalf("kindContextName() = %q, want %q", got, want)
	}
}

func TestValidateClusterName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := validateClusterName("clawmachine"); err != nil {
			t.Fatalf("validateClusterName returned error for valid name: %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if err := validateClusterName("   "); err == nil {
			t.Fatalf("validateClusterName should fail on empty name")
		}
	})

	t.Run("whitespace", func(t *testing.T) {
		if err := validateClusterName("my cluster"); err == nil {
			t.Fatalf("validateClusterName should fail on whitespace")
		}
	})
}

func TestResolveKindClusterContext(t *testing.T) {
	cluster, ctx := resolveKindClusterContext("kind-dev")
	if cluster != "dev" || ctx != "kind-dev" {
		t.Fatalf("resolveKindClusterContext(kind-dev) = (%q, %q), want (%q, %q)", cluster, ctx, "dev", "kind-dev")
	}

	cluster, ctx = resolveKindClusterContext("orbstack")
	if cluster != "" || ctx != "orbstack" {
		t.Fatalf("resolveKindClusterContext(orbstack) = (%q, %q), want (%q, %q)", cluster, ctx, "", "orbstack")
	}
}

func TestWriteKindConfigForInstall(t *testing.T) {
	t.Run("default cni config", func(t *testing.T) {
		path, cleanup, err := writeKindConfigForInstall(false)
		if err != nil {
			t.Fatalf("writeKindConfigForInstall(false) error: %v", err)
		}
		defer cleanup()

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error: %v", path, err)
		}
		if string(got) != string(kindConfigDefaultCNI) {
			t.Fatalf("default config mismatch")
		}
	})

	t.Run("cilium config", func(t *testing.T) {
		path, cleanup, err := writeKindConfigForInstall(true)
		if err != nil {
			t.Fatalf("writeKindConfigForInstall(true) error: %v", err)
		}
		defer cleanup()

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error: %v", path, err)
		}
		if string(got) != string(kindConfigCilium) {
			t.Fatalf("cilium config mismatch")
		}
		if !strings.Contains(string(got), "disableDefaultCNI: true") {
			t.Fatalf("expected cilium kind config to disable default CNI")
		}
	})
}
