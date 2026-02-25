package botenv

import (
	"encoding/json"
	"testing"
)

func TestBuildConfigContent_JSON(t *testing.T) {
	bot := &BotConfig{
		Name:         "picoclaw",
		ConfigFormat: "json",
		Sections: []Section{
			{
				Key:   "channels",
				Label: "Channels",
				Fields: []Field{
					{Key: "discordEnabled", ConfigPath: "channels.discord.enabled", Type: "checkbox"},
					{Key: "discordToken", ConfigPath: "channels.discord.token", Type: "password"},
				},
			},
			{
				Key:   "providers",
				Label: "Providers",
				Fields: []Field{
					{Key: "anthropicApiKey", ConfigPath: "providers.anthropic.api_key", Type: "password"},
				},
			},
			{
				Key:   "agent",
				Label: "Agent",
				Fields: []Field{
					{Key: "agentMaxTokens", ConfigPath: "agents.defaults.max_tokens", Type: "text"},
				},
			},
		},
	}

	values := map[string]string{
		"discordEnabled":  "true",
		"discordToken":    "my-token",
		"anthropicApiKey": "sk-ant-test",
		"agentMaxTokens":  "8192",
	}

	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nContent:\n%s", err, content)
	}

	// Check nested paths
	channels := result["channels"].(map[string]any)
	discord := channels["discord"].(map[string]any)
	if discord["enabled"] != true {
		t.Errorf("expected discord.enabled=true, got %v", discord["enabled"])
	}
	if discord["token"] != "my-token" {
		t.Errorf("expected discord.token=my-token, got %v", discord["token"])
	}

	providers := result["providers"].(map[string]any)
	anthropic := providers["anthropic"].(map[string]any)
	if anthropic["api_key"] != "sk-ant-test" {
		t.Errorf("expected providers.anthropic.api_key=sk-ant-test, got %v", anthropic["api_key"])
	}

	agents := result["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	// 8192 should be coerced to int
	if defaults["max_tokens"] != float64(8192) {
		t.Errorf("expected agents.defaults.max_tokens=8192, got %v", defaults["max_tokens"])
	}
}

func TestBuildConfigContent_EmptyValues(t *testing.T) {
	bot := &BotConfig{
		Name:         "picoclaw",
		ConfigFormat: "json",
		Sections: []Section{
			{
				Key:   "channels",
				Label: "Channels",
				Fields: []Field{
					{Key: "discordToken", ConfigPath: "channels.discord.token", Type: "password"},
				},
			},
		},
	}

	content, err := BuildConfigContent(bot, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	// Empty values should produce empty JSON object
	if content != "{}" {
		t.Errorf("expected {}, got %s", content)
	}
}

func TestBuildConfigContent_EnvFormat(t *testing.T) {
	bot := &BotConfig{
		Name:         "ironclaw",
		ConfigFormat: "env",
	}

	content, err := BuildConfigContent(bot, map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatal(err)
	}

	if content != "" {
		t.Errorf("env format should return empty string, got %s", content)
	}
}

func picoclaw() *BotConfig {
	return &BotConfig{
		Name:            "picoclaw",
		ConfigFormat:    "json",
		AgentModelField: "agentModel",
		ModelProviders: []ModelProvider{
			{
				Name:         "anthropic",
				ModelFilters: []string{"claude-"},
				ModelPrefix:  "anthropic/",
				APIBase:      "https://api.anthropic.com/v1",
				APIKeyField:  "anthropicApiKey",
			},
			{
				Name:         "openai",
				ModelFilters: []string{"gpt-", "o3-"},
				ModelPrefix:  "openai/",
				APIBase:      "https://api.openai.com/v1",
				APIKeyField:  "openaiApiKey",
				DefaultModel: "gpt-4o",
			},
			{
				Name:         "openrouter",
				ModelFilters: []string{"deepseek/", "qwen/", "meta-llama/", "llama-"},
				ModelPrefix:  "openrouter/",
				APIBase:      "https://openrouter.ai/api/v1",
				APIKeyField:  "openrouterApiKey",
				DefaultModel: "deepseek/deepseek-chat-v3-0324",
			},
		},
		Sections: []Section{
			{
				Key: "agent",
				Fields: []Field{
					{Key: "agentModel", ConfigPath: "agents.defaults.model", Type: "select"},
					{Key: "agentMaxTokens", ConfigPath: "agents.defaults.max_tokens", Type: "text"},
				},
			},
		},
	}
}

func TestBuildModelList_SelectedModelMatchesProvider(t *testing.T) {
	bot := picoclaw()
	values := map[string]string{
		"agentModel":      "claude-sonnet-4-20250514",
		"anthropicApiKey": "sk-ant-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}

	// agents.defaults.model should be set
	agents := result["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if defaults["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("agents.defaults.model = %v, want claude-sonnet-4-20250514", defaults["model"])
	}

	// model_list should have one entry for anthropic
	list := result["model_list"].([]any)
	if len(list) != 1 {
		t.Fatalf("model_list len = %d, want 1", len(list))
	}
	entry := list[0].(map[string]any)
	if entry["model_name"] != "claude-sonnet-4-20250514" {
		t.Errorf("model_name = %v, want claude-sonnet-4-20250514", entry["model_name"])
	}
	if entry["model"] != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("model = %v, want anthropic/claude-sonnet-4-20250514", entry["model"])
	}
	if entry["api_key"] != "sk-ant-test" {
		t.Errorf("api_key = %v, want sk-ant-test", entry["api_key"])
	}
	if entry["api_base"] != "https://api.anthropic.com/v1" {
		t.Errorf("api_base = %v, want https://api.anthropic.com/v1", entry["api_base"])
	}

	// deprecated providers key must not appear
	if _, ok := result["providers"]; ok {
		t.Error("providers key should not appear in output when model_list is used")
	}
}

func TestBuildModelList_MultipleProviders(t *testing.T) {
	bot := picoclaw()
	values := map[string]string{
		"agentModel":      "claude-sonnet-4-20250514",
		"anthropicApiKey": "sk-ant-test",
		"openaiApiKey":    "sk-oai-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}

	list := result["model_list"].([]any)
	if len(list) != 2 {
		t.Fatalf("model_list len = %d, want 2 (anthropic + openai fallback)", len(list))
	}

	// Second entry should use openai's DefaultModel since claude- doesn't match gpt-/o3-
	entry1 := list[0].(map[string]any)
	if entry1["model_name"] != "claude-sonnet-4-20250514" {
		t.Errorf("entry[0].model_name = %v, want claude-sonnet-4-20250514", entry1["model_name"])
	}
	entry2 := list[1].(map[string]any)
	if entry2["model_name"] != "gpt-4o" {
		t.Errorf("entry[1].model_name = %v, want gpt-4o (default)", entry2["model_name"])
	}
	if entry2["model"] != "openai/gpt-4o" {
		t.Errorf("entry[1].model = %v, want openai/gpt-4o", entry2["model"])
	}
}

func TestBuildModelList_NoAPIKey_Skipped(t *testing.T) {
	bot := picoclaw()
	// Only agentModel, no API keys — model_list should be empty (key omitted)
	values := map[string]string{
		"agentModel": "claude-sonnet-4-20250514",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}
	if _, ok := result["model_list"]; ok {
		t.Error("model_list should be absent when no API keys are provided")
	}
}

func TestBuildModelList_PrefixedModelName(t *testing.T) {
	bot := picoclaw()
	// Model submitted with full provider prefix — "anthropic/claude-sonnet-4.6"
	// must not be double-prefixed to "anthropic/anthropic/claude-sonnet-4.6".
	values := map[string]string{
		"agentModel":      "anthropic/claude-sonnet-4.6",
		"anthropicApiKey": "sk-ant-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}

	// model_list must be present
	rawList, ok := result["model_list"]
	if !ok {
		t.Fatal("model_list missing from config when prefixed model name is used")
	}
	list := rawList.([]any)
	if len(list) != 1 {
		t.Fatalf("model_list len = %d, want 1", len(list))
	}
	entry := list[0].(map[string]any)
	if entry["model_name"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model_name = %v, want anthropic/claude-sonnet-4.6", entry["model_name"])
	}
	// model must NOT be double-prefixed
	if entry["model"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model = %v, want anthropic/claude-sonnet-4.6 (no double prefix)", entry["model"])
	}
	if entry["api_key"] != "sk-ant-test" {
		t.Errorf("api_key = %v, want sk-ant-test", entry["api_key"])
	}

	// agents.defaults.model must match the model_name in model_list
	agents := result["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if defaults["model"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("agents.defaults.model = %v, want anthropic/claude-sonnet-4.6", defaults["model"])
	}
}

// TestBuildModelList_PrefixedModelName_NoDoublePrefix is a regression test for
// the bug where a model value already containing the provider prefix (e.g.
// "anthropic/claude-sonnet-4.6" from the model picker) produced no model_list
// entry because "claude-" filters didn't match "anthropic/claude-…", and would
// have double-prefixed to "anthropic/anthropic/…" if it had matched.
func TestBuildModelList_PrefixedModelName_NoDoublePrefix(t *testing.T) {
	bot := picoclaw()
	values := map[string]string{
		"agentModel":      "anthropic/claude-sonnet-4.6",
		"anthropicApiKey": "sk-ant-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}

	// model_list must be present — previously it was entirely absent
	rawList, ok := result["model_list"]
	if !ok {
		t.Fatal("model_list missing: prefixed model name must still produce a model_list entry")
	}
	list := rawList.([]any)
	if len(list) != 1 {
		t.Fatalf("model_list len = %d, want 1", len(list))
	}
	entry := list[0].(map[string]any)

	if entry["model_name"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model_name = %v, want anthropic/claude-sonnet-4.6", entry["model_name"])
	}
	// model must NOT be double-prefixed — previously would have been
	// "anthropic/anthropic/claude-sonnet-4.6" if the filter had matched
	if entry["model"] == "anthropic/anthropic/claude-sonnet-4.6" {
		t.Error("model is double-prefixed: anthropic/anthropic/claude-sonnet-4.6")
	}
	if entry["model"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model = %v, want anthropic/claude-sonnet-4.6", entry["model"])
	}
	if entry["api_key"] != "sk-ant-test" {
		t.Errorf("api_key = %v, want sk-ant-test", entry["api_key"])
	}

	// agents.defaults.model must be identical to model_name so picoclaw can
	// look up the model in the model_list
	agents := result["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if defaults["model"] != entry["model_name"] {
		t.Errorf("agents.defaults.model (%v) does not match model_list model_name (%v); picoclaw will fail to resolve the model",
			defaults["model"], entry["model_name"])
	}
}

func TestBuildModelList_OpenRouterMatchesDeepSeek(t *testing.T) {
	bot := picoclaw()
	values := map[string]string{
		"agentModel":       "deepseek/deepseek-chat-v3-0324",
		"openrouterApiKey": "sk-or-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}
	list := result["model_list"].([]any)
	entry := list[0].(map[string]any)
	if entry["model_name"] != "deepseek/deepseek-chat-v3-0324" {
		t.Errorf("model_name = %v", entry["model_name"])
	}
	if entry["model"] != "openrouter/deepseek/deepseek-chat-v3-0324" {
		t.Errorf("model = %v", entry["model"])
	}
}

func TestBuildModelList_OpenRouterModelPickerID_MatchesSelectedModel(t *testing.T) {
	bot := picoclaw()
	values := map[string]string{
		"agentModel":       "z-ai/glm-4.5-air:free",
		"openrouterApiKey": "sk-or-test",
	}
	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, content)
	}
	list := result["model_list"].([]any)
	if len(list) != 1 {
		t.Fatalf("model_list len = %d, want 1", len(list))
	}
	entry := list[0].(map[string]any)
	if entry["model_name"] != "z-ai/glm-4.5-air:free" {
		t.Errorf("model_name = %v", entry["model_name"])
	}
	if entry["model"] != "openrouter/z-ai/glm-4.5-air:free" {
		t.Errorf("model = %v", entry["model"])
	}

	agents := result["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if defaults["model"] != entry["model_name"] {
		t.Errorf("agents.defaults.model = %v, want %v", defaults["model"], entry["model_name"])
	}
}

func TestBotConfig_HasConfigFile(t *testing.T) {
	tests := []struct {
		format string
		want   bool
	}{
		{"json", true},
		{"toml", true},
		{"env", false},
		{"", false},
	}
	for _, tt := range tests {
		bc := &BotConfig{ConfigFormat: tt.format}
		if got := bc.HasConfigFile(); got != tt.want {
			t.Errorf("HasConfigFile(%q) = %v, want %v", tt.format, got, tt.want)
		}
	}
}
