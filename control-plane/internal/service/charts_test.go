package service

import (
	"testing"

	"helm.sh/helm/v4/pkg/cli"
)

func TestGetEmbeddedBotChart(t *testing.T) {
	tests := []struct {
		name    string
		botType BotType
		wantErr bool
	}{
		{"picoclaw", BotTypePicoClaw, false},
		{"ironclaw", BotTypeIronClaw, false},
		{"unknown", BotType("unknown"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := getEmbeddedBotChart(tt.botType)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("expected non-empty chart data")
			}
		})
	}
}

func TestGetControlPlaneChart(t *testing.T) {
	data := GetControlPlaneChart()
	if len(data) == 0 {
		t.Fatal("expected non-empty control plane chart data")
	}
}

func TestNewChartResolver_Defaults(t *testing.T) {
	r := NewChartResolver("", cli.New())
	if r.repoURL != DefaultChartRepoURL {
		t.Fatalf("expected default repo URL %q, got %q", DefaultChartRepoURL, r.repoURL)
	}
}

func TestNewChartResolver_Custom(t *testing.T) {
	r := NewChartResolver("oci://example.com/charts", cli.New())
	if r.repoURL != "oci://example.com/charts" {
		t.Fatalf("expected custom repo URL, got %q", r.repoURL)
	}
}

func TestNewChartResolver_Embedded(t *testing.T) {
	r := NewChartResolver("embedded", cli.New())
	if !r.UseEmbeddedOnly() {
		t.Fatal("expected UseEmbeddedOnly to be true")
	}
}

func TestChartResolver_FallbackToEmbedded(t *testing.T) {
	// Use an invalid repo URL to force fallback
	r := NewChartResolver("oci://invalid.example.com/nonexistent", cli.New())

	data, err := r.ResolveChart(BotTypePicoClaw)
	if err != nil {
		t.Fatalf("expected fallback to embedded, got error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty chart data from fallback")
	}
}
