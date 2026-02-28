package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestVersionCommand_Default(t *testing.T) {
	origVersion := version
	version = "1.2.3"
	defer func() { version = origVersion }()

	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "clawmachine") {
		t.Fatalf("output missing clawmachine name: %q", got)
	}
	if !strings.Contains(got, "v1.2.3") {
		t.Fatalf("output missing version: %q", got)
	}
	if strings.Contains(got, "bot images (canonical repo:tag):") {
		t.Fatalf("unexpected --all output for default version command: %q", got)
	}
}

func TestVersionCommand_All(t *testing.T) {
	origVersion := version
	// Use a version different from chart defaults to verify runtime tag override.
	version = "9.9.9"
	defer func() { version = origVersion }()

	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version --all command: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"clawmachine",
		"v9.9.9",
		"bot images (canonical repo:tag):",
		"openclaw:",
		"picoclaw:",
		"ironclaw:",
		"busybox:",
		"vendored charts (sha256):",
		"external-secrets@2.0.0:",
		"cilium@1.17.1:",
		"connect@2.3.0:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}

	// Verify runtime tag override: all bot images should use the runtime version,
	// not the hardcoded chart default.
	for _, want := range []string{
		"ghcr.io/zackerydev/openclaw:9.9.9",
		"ghcr.io/zackerydev/picoclaw:9.9.9",
		"ghcr.io/zackerydev/ironclaw:9.9.9",
		"ghcr.io/zackerydev/theclawmachine-toolbox:9.9.9",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing runtime-tagged image %q:\n%s", want, got)
		}
	}

	shaPattern := regexp.MustCompile(`sha256:[a-f0-9]{64}`)
	matches := shaPattern.FindAllString(got, -1)
	if len(matches) < 3 {
		t.Fatalf("expected at least 3 sha256 checksums, got %d:\n%s", len(matches), got)
	}
}
