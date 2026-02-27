package main

import (
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/cli"
)

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

func TestRunInstall_UsesRuntimeImageOverrides(t *testing.T) {
	restore := stubInstallDeps()
	defer restore()

	origVersion := version
	version = "0.1.6"
	defer func() { version = origVersion }()

	var (
		gotValues    map[string]any
		gotRelease   string
		gotNamespace string
	)
	runHelmInstallForInstall = func(_ *action.Configuration, releaseName, namespace string, chrt chart.Charter, vals map[string]any) error {
		gotValues = vals
		gotRelease = releaseName
		gotNamespace = namespace
		if chrt == nil {
			t.Fatal("chart should not be nil")
		}
		return nil
	}

	cmd := newInstallCmd()
	if err := cmd.Flags().Set("interactive", "false"); err != nil {
		t.Fatalf("set interactive flag: %v", err)
	}

	if err := runInstall(cmd); err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}

	if gotRelease != "clawmachine" {
		t.Fatalf("releaseName = %q, want %q", gotRelease, "clawmachine")
	}
	if gotNamespace != "claw-machine" {
		t.Fatalf("namespace = %q, want %q", gotNamespace, "claw-machine")
	}
	image, ok := gotValues["image"].(map[string]any)
	if !ok {
		t.Fatalf("image values missing: %#v", gotValues)
	}
	if image["repository"] != "ghcr.io/zackerydev/theclawmachine" {
		t.Fatalf("image.repository = %v, want %q", image["repository"], "ghcr.io/zackerydev/theclawmachine")
	}
	if image["tag"] != "0.1.6" {
		t.Fatalf("image.tag = %v, want %q", image["tag"], "0.1.6")
	}
}

func TestRunInstall_UsesFallbackTagForDev(t *testing.T) {
	restore := stubInstallDeps()
	defer restore()

	origVersion := version
	version = "dev"
	defer func() { version = origVersion }()

	var gotValues map[string]any
	runHelmInstallForInstall = func(_ *action.Configuration, _, _ string, _ chart.Charter, vals map[string]any) error {
		gotValues = vals
		return nil
	}

	cmd := newInstallCmd()
	if err := cmd.Flags().Set("interactive", "false"); err != nil {
		t.Fatalf("set interactive flag: %v", err)
	}

	if err := runInstall(cmd); err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}

	image, ok := gotValues["image"].(map[string]any)
	if !ok {
		t.Fatalf("image values missing: %#v", gotValues)
	}
	if image["tag"] != "0.1.0" {
		t.Fatalf("image.tag = %v, want %q", image["tag"], "0.1.0")
	}
}

func TestRunInstall_InvalidRuntimeVersionFails(t *testing.T) {
	restore := stubInstallDeps()
	defer restore()

	origVersion := version
	version = "dirty-build"
	defer func() { version = origVersion }()

	called := false
	runHelmInstallForInstall = func(_ *action.Configuration, _, _ string, _ chart.Charter, _ map[string]any) error {
		called = true
		return nil
	}

	cmd := newInstallCmd()
	if err := cmd.Flags().Set("interactive", "false"); err != nil {
		t.Fatalf("set interactive flag: %v", err)
	}

	err := runInstall(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "runtime version") {
		t.Fatalf("expected invalid runtime version error, got: %v", err)
	}
	if called {
		t.Fatal("install should not execute helm install when runtime version is invalid")
	}
}

func stubInstallDeps() func() {
	origLoad := loadControlPlaneChartForInstall
	origInit := initActionConfigForInstall
	origRun := runHelmInstallForInstall

	loadControlPlaneChartForInstall = func() (chart.Charter, error) {
		return &chartv2.Chart{
			Metadata: &chartv2.Metadata{
				Name:       "clawmachine",
				AppVersion: "0.1.0",
			},
			Values: map[string]any{
				"image": map[string]any{
					"repository": "ghcr.io/zackerydev/theclawmachine",
				},
			},
		}, nil
	}
	initActionConfigForInstall = func(_ *cli.EnvSettings, _ string) (*action.Configuration, error) {
		return &action.Configuration{}, nil
	}
	runHelmInstallForInstall = func(_ *action.Configuration, _, _ string, _ chart.Charter, _ map[string]any) error {
		return nil
	}

	return func() {
		loadControlPlaneChartForInstall = origLoad
		initActionConfigForInstall = origInit
		runHelmInstallForInstall = origRun
	}
}
