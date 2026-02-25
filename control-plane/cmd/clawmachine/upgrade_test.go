package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

func TestNewUpgradeCmd_HasExpectedFlags(t *testing.T) {
	cmd := newUpgradeCmd()

	for _, name := range []string{"namespace", "name", "yes"} {
		if got := cmd.Flags().Lookup(name); got == nil {
			t.Fatalf("expected flag %q to be present", name)
		}
	}
}

func TestNormalizeImageTag(t *testing.T) {
	tests := []struct {
		name         string
		cliVersion   string
		appVersion   string
		wantTag      string
		wantFallback bool
	}{
		{name: "release version", cliVersion: "0.2.1", appVersion: "0.1.0", wantTag: "0.2.1", wantFallback: false},
		{name: "release version with v prefix", cliVersion: "v0.2.1", appVersion: "0.1.0", wantTag: "0.2.1", wantFallback: false},
		{name: "dev fallback", cliVersion: "dev", appVersion: "0.1.0", wantTag: "0.1.0", wantFallback: true},
		{name: "empty fallback", cliVersion: "", appVersion: "0.1.0", wantTag: "0.1.0", wantFallback: true},
		{name: "prerelease fallback", cliVersion: "0.2.1-rc.1", appVersion: "0.1.0", wantTag: "0.1.0", wantFallback: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotFallback := normalizeImageTag(tt.cliVersion, tt.appVersion)
			if gotTag != tt.wantTag {
				t.Fatalf("normalizeImageTag() tag = %q, want %q", gotTag, tt.wantTag)
			}
			if gotFallback != tt.wantFallback {
				t.Fatalf("normalizeImageTag() fallback = %t, want %t", gotFallback, tt.wantFallback)
			}
		})
	}
}

func TestResolveBundledImage(t *testing.T) {
	chrt := &chartv2.Chart{
		Metadata: &chartv2.Metadata{AppVersion: "0.3.0"},
		Values: map[string]any{
			"image": map[string]any{
				"repository": "ghcr.io/zackerydev/theclawmachine",
			},
		},
	}

	repo, tag, fallback, err := resolveBundledImage(chrt, "dev")
	if err != nil {
		t.Fatalf("resolveBundledImage() error = %v", err)
	}
	if repo != "ghcr.io/zackerydev/theclawmachine" {
		t.Fatalf("repo = %q, want %q", repo, "ghcr.io/zackerydev/theclawmachine")
	}
	if tag != "0.3.0" {
		t.Fatalf("tag = %q, want %q", tag, "0.3.0")
	}
	if !fallback {
		t.Fatal("expected fallback=true for dev version")
	}
}

func TestResolveBundledImage_MissingRepository(t *testing.T) {
	chrt := &chartv2.Chart{
		Metadata: &chartv2.Metadata{AppVersion: "0.3.0"},
		Values: map[string]any{
			"image": map[string]any{},
		},
	}

	_, _, _, err := resolveBundledImage(chrt, "0.2.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpgradeOverrides(t *testing.T) {
	got := upgradeOverrides("ghcr.io/zackerydev/theclawmachine", "0.2.0")
	want := map[string]any{
		"image": map[string]any{
			"repository": "ghcr.io/zackerydev/theclawmachine",
			"tag":        "0.2.0",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("upgradeOverrides() = %#v, want %#v", got, want)
	}
}

func TestRunUpgrade_MissingRelease(t *testing.T) {
	restore := stubUpgradeDeps()
	defer restore()

	cmd := newUpgradeCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set flag yes: %v", err)
	}

	err := runUpgrade(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestRunUpgrade_UsesFallbackTagAndOverrides(t *testing.T) {
	restore := stubUpgradeDeps()
	defer restore()

	origVersion := version
	version = "dev"
	defer func() { version = origVersion }()

	listReleasesForUpgrade = func(_ *action.Configuration, _ string) ([]release.Releaser, error) {
		return []release.Releaser{&releasev1.Release{Name: "clawmachine"}}, nil
	}

	var gotVals map[string]any
	runHelmUpgradeForUpgrade = func(_ *action.Configuration, _, _ string, _ chart.Charter, vals map[string]any) error {
		gotVals = vals
		return nil
	}

	cmd := newUpgradeCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set flag yes: %v", err)
	}
	if err := runUpgrade(cmd); err != nil {
		t.Fatalf("runUpgrade() error = %v", err)
	}

	image, _ := gotVals["image"].(map[string]any)
	if image["repository"] != "ghcr.io/zackerydev/theclawmachine" {
		t.Fatalf("repository override = %v", image["repository"])
	}
	if image["tag"] != "0.9.9" {
		t.Fatalf("tag override = %v, want %q", image["tag"], "0.9.9")
	}
}

func TestRunUpgrade_UpgradeError(t *testing.T) {
	restore := stubUpgradeDeps()
	defer restore()

	listReleasesForUpgrade = func(_ *action.Configuration, _ string) ([]release.Releaser, error) {
		return []release.Releaser{&releasev1.Release{Name: "clawmachine"}}, nil
	}
	runHelmUpgradeForUpgrade = func(_ *action.Configuration, _, _ string, _ chart.Charter, _ map[string]any) error {
		return errors.New("helm upgrade failed")
	}

	cmd := newUpgradeCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set flag yes: %v", err)
	}

	err := runUpgrade(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upgrading release") {
		t.Fatalf("expected wrapped upgrade error, got: %v", err)
	}
}

func stubUpgradeDeps() func() {
	origLoad := loadControlPlaneChartForUpgrade
	origInit := initActionConfigForUpgrade
	origList := listReleasesForUpgrade
	origRun := runHelmUpgradeForUpgrade

	loadControlPlaneChartForUpgrade = func() (chart.Charter, error) {
		return &chartv2.Chart{
			Metadata: &chartv2.Metadata{AppVersion: "0.9.9"},
			Values: map[string]any{
				"image": map[string]any{
					"repository": "ghcr.io/zackerydev/theclawmachine",
				},
			},
		}, nil
	}
	initActionConfigForUpgrade = func(_ *cli.EnvSettings, _ string) (*action.Configuration, error) {
		return &action.Configuration{}, nil
	}
	listReleasesForUpgrade = func(_ *action.Configuration, _ string) ([]release.Releaser, error) {
		return nil, nil
	}
	runHelmUpgradeForUpgrade = func(_ *action.Configuration, _, _ string, _ chart.Charter, _ map[string]any) error {
		return nil
	}

	return func() {
		loadControlPlaneChartForUpgrade = origLoad
		initActionConfigForUpgrade = origInit
		listReleasesForUpgrade = origList
		runHelmUpgradeForUpgrade = origRun
	}
}
