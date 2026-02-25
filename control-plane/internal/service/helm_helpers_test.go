package service

import (
	"testing"
	"time"

	helmbasechart "helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	helmrelease "helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

func testRelease(name, namespace, chartName string, deployedAt time.Time) *releasev1.Release {
	var chrt *chartv2.Chart
	if chartName != "" {
		chrt = &chartv2.Chart{
			Metadata: &chartv2.Metadata{
				Name:       chartName,
				Version:    "0.1.0",
				APIVersion: "v2",
			},
		}
	}

	return &releasev1.Release{
		Name:      name,
		Namespace: namespace,
		Version:   7,
		Chart:     chrt,
		Info: &releasev1.Info{
			Status:       releasecommon.StatusDeployed,
			LastDeployed: deployedAt,
		},
	}
}

func TestLoadBotChart(t *testing.T) {
	t.Run("known bot type", func(t *testing.T) {
		chrt, err := loadBotChart(BotTypePicoClaw)
		if err != nil {
			t.Fatalf("loadBotChart(%q) error: %v", BotTypePicoClaw, err)
		}

		acc, err := helmbasechart.NewAccessor(chrt)
		if err != nil {
			t.Fatalf("chart.NewAccessor error: %v", err)
		}
		if acc.Name() == "" {
			t.Fatal("expected loaded chart to have a name")
		}
	})

	t.Run("unknown bot type", func(t *testing.T) {
		if _, err := loadBotChart(BotType("not-a-bot")); err == nil {
			t.Fatal("expected unknown bot type to return an error")
		}
	})
}

func TestReleaseToInfo(t *testing.T) {
	deployedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("detects bot type from chart when not provided", func(t *testing.T) {
		rel := testRelease("alpha", "claw-machine", "picoclaw", deployedAt)

		info, err := releaseToInfo(rel, "")
		if err != nil {
			t.Fatalf("releaseToInfo error: %v", err)
		}

		if info.Name != "alpha" {
			t.Fatalf("Name = %q, want %q", info.Name, "alpha")
		}
		if info.Namespace != "claw-machine" {
			t.Fatalf("Namespace = %q, want %q", info.Namespace, "claw-machine")
		}
		if info.Status != "deployed" {
			t.Fatalf("Status = %q, want %q", info.Status, "deployed")
		}
		if info.Version != 7 {
			t.Fatalf("Version = %d, want %d", info.Version, 7)
		}
		if info.BotType != "picoclaw" {
			t.Fatalf("BotType = %q, want %q", info.BotType, "picoclaw")
		}
		if info.Updated != deployedAt.Format(time.RFC3339) {
			t.Fatalf("Updated = %q, want %q", info.Updated, deployedAt.Format(time.RFC3339))
		}
	})

	t.Run("uses explicit bot type override", func(t *testing.T) {
		rel := testRelease("beta", "claw-machine", "openclaw", deployedAt)

		info, err := releaseToInfo(rel, "forced-type")
		if err != nil {
			t.Fatalf("releaseToInfo error: %v", err)
		}
		if info.BotType != "forced-type" {
			t.Fatalf("BotType = %q, want %q", info.BotType, "forced-type")
		}
	})

	t.Run("errors for unsupported release type", func(t *testing.T) {
		if _, err := releaseToInfo(struct{}{}, ""); err == nil {
			t.Fatal("expected unsupported release type to return error")
		}
	})
}

func TestDetectBotType(t *testing.T) {
	t.Run("unknown when chart is nil", func(t *testing.T) {
		rel := testRelease("gamma", "claw-machine", "", time.Now().UTC())
		acc, err := helmrelease.NewAccessor(rel)
		if err != nil {
			t.Fatalf("release.NewAccessor error: %v", err)
		}

		if got := detectBotType(acc); got != "unknown" {
			t.Fatalf("detectBotType() = %q, want %q", got, "unknown")
		}
	})
}
