package main

import (
	"testing"
)

func TestHelmConstants(t *testing.T) {
	if esoReleaseName != "external-secrets" {
		t.Errorf("esoReleaseName = %q, want %q", esoReleaseName, "external-secrets")
	}
	if esoNamespace != "external-secrets" {
		t.Errorf("esoNamespace = %q, want %q", esoNamespace, "external-secrets")
	}
	if ciliumReleaseName != "cilium" {
		t.Errorf("ciliumReleaseName = %q, want %q", ciliumReleaseName, "cilium")
	}
	if ciliumNamespace != "kube-system" {
		t.Errorf("ciliumNamespace = %q, want %q", ciliumNamespace, "kube-system")
	}
}

func TestNewHelmSettings(t *testing.T) {
	t.Run("empty context", func(t *testing.T) {
		settings := newHelmSettings("")
		if settings.KubeContext != "" {
			t.Errorf("expected empty KubeContext, got %q", settings.KubeContext)
		}
	})

	t.Run("with context", func(t *testing.T) {
		settings := newHelmSettings("my-cluster")
		if settings.KubeContext != "my-cluster" {
			t.Errorf("KubeContext = %q, want %q", settings.KubeContext, "my-cluster")
		}
	})
}

func TestGetenv(t *testing.T) {
	t.Run("returns fallback when unset", func(t *testing.T) {
		result := getenv("CLAWMACHINE_TEST_UNSET_VAR_12345", "fallback")
		if result != "fallback" {
			t.Errorf("getenv = %q, want %q", result, "fallback")
		}
	})

	t.Run("returns value when set", func(t *testing.T) {
		t.Setenv("CLAWMACHINE_TEST_VAR", "actual")
		result := getenv("CLAWMACHINE_TEST_VAR", "fallback")
		if result != "actual" {
			t.Errorf("getenv = %q, want %q", result, "actual")
		}
	})
}
