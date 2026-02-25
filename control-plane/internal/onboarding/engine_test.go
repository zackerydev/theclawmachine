package onboarding

import (
	"encoding/json"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

func newEngine(t *testing.T) *Engine {
	t.Helper()
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return NewEngine(reg)
}

func TestProfile_ReturnsGuidedAndAdvancedQuestions(t *testing.T) {
	engine := newEngine(t)

	profile, err := engine.Profile(service.BotTypePicoClaw)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if profile.Version != ProfileVersion {
		t.Fatalf("version=%q want %q", profile.Version, ProfileVersion)
	}
	if len(profile.Questions) == 0 {
		t.Fatal("expected questions in profile")
	}
	if len(profile.GuidedQuestionIDs) == 0 {
		t.Fatal("expected guided questions")
	}
	if len(profile.AdvancedQuestionIDs) == 0 {
		t.Fatal("expected advanced questions")
	}
}

func TestCompile_OpenClawSecretRefsProduceEnvSecrets(t *testing.T) {
	engine := newEngine(t)

	result, err := engine.Compile(service.BotTypeOpenClaw, map[string]string{
		"authChoice":      "apiKey",
		"anthropicApiKey": "1p:anthropic-main",
		"gatewayToken":    "1p:gateway-main",
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(result.SecretRefs) != 2 {
		t.Fatalf("SecretRefs=%d want 2", len(result.SecretRefs))
	}

	foundGateway := false
	foundAnthropic := false
	for _, env := range result.EnvSecrets {
		switch env.EnvVar {
		case "OPENCLAW_GATEWAY_TOKEN":
			foundGateway = true
		case "ANTHROPIC_API_KEY":
			foundAnthropic = true
		}
	}
	if !foundGateway || !foundAnthropic {
		t.Fatalf("expected OPENCLAW_GATEWAY_TOKEN and ANTHROPIC_API_KEY mappings, got %+v", result.EnvSecrets)
	}
	if result.ConfigFile != nil {
		t.Fatalf("expected OpenClaw compile to be values/env-only, got config file output: %#v", result.ConfigFile)
	}
}

func TestCompile_PicoClawBuildsConfigFile(t *testing.T) {
	engine := newEngine(t)

	result, err := engine.Compile(service.BotTypePicoClaw, map[string]string{
		"agentModel":      "anthropic/claude-sonnet-4.6",
		"anthropicApiKey": "sk-ant-test",
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.ConfigFile == nil || !result.ConfigFile.Enabled {
		t.Fatalf("expected config file output, got %#v", result.ConfigFile)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(result.ConfigFile.Content), &cfg); err != nil {
		t.Fatalf("invalid JSON config: %v", err)
	}
	if _, ok := cfg["model_list"]; !ok {
		t.Fatalf("model_list missing from config: %s", result.ConfigFile.Content)
	}
}

func TestDeepMergeValues(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{
			"x": "keep",
		},
	}
	patch := map[string]any{
		"a": map[string]any{
			"y": "new",
		},
		"b": "set",
	}

	DeepMergeValues(dst, patch)
	a := dst["a"].(map[string]any)
	if a["x"] != "keep" || a["y"] != "new" {
		t.Fatalf("unexpected nested merge result: %#v", a)
	}
	if dst["b"] != "set" {
		t.Fatalf("unexpected top-level merge result: %#v", dst)
	}
}
