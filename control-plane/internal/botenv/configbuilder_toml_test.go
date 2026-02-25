package botenv

import (
	"strings"
	"testing"
)

func TestBuildConfigContent_TOML(t *testing.T) {
	bot := &BotConfig{
		Name:         "testbot",
		ConfigFormat: "toml",
		Sections: []Section{
			{
				Key:   "agent",
				Label: "Agent",
				Fields: []Field{
					{Key: "model", ConfigPath: "agent.model", Type: "text"},
					{Key: "maxTokens", ConfigPath: "agent.max_tokens", Type: "text"},
					{Key: "enabled", ConfigPath: "agent.enabled", Type: "checkbox"},
				},
			},
		},
	}

	values := map[string]string{
		"model":     "gpt-4",
		"maxTokens": "4096",
		"enabled":   "true",
	}

	content, err := BuildConfigContent(bot, values)
	if err != nil {
		t.Fatal(err)
	}

	// TOML should contain section header and values
	if !strings.Contains(content, "[agent]") {
		t.Errorf("TOML missing [agent] section header:\n%s", content)
	}
	if !strings.Contains(content, `model = "gpt-4"`) {
		t.Errorf("TOML missing model value:\n%s", content)
	}
	if !strings.Contains(content, "max_tokens = 4096") {
		t.Errorf("TOML missing max_tokens value:\n%s", content)
	}
	if !strings.Contains(content, "enabled = true") {
		t.Errorf("TOML missing enabled value:\n%s", content)
	}
}

func TestBuildConfigContent_TOML_Empty(t *testing.T) {
	bot := &BotConfig{
		Name:         "testbot",
		ConfigFormat: "toml",
		Fields: []Field{
			{Key: "foo", ConfigPath: "bar", Type: "text"},
		},
	}

	content, err := BuildConfigContent(bot, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("expected empty TOML, got %q", content)
	}
}

func TestTomlMarshal_ScalarTypes(t *testing.T) {
	m := map[string]any{
		"str":   "hello",
		"num":   42,
		"float": 3.14,
		"flag":  true,
	}

	result := tomlMarshal(m, "")
	if !strings.Contains(result, `str = "hello"`) {
		t.Errorf("missing string value:\n%s", result)
	}
	if !strings.Contains(result, "num = 42") {
		t.Errorf("missing int value:\n%s", result)
	}
	if !strings.Contains(result, "flag = true") {
		t.Errorf("missing bool value:\n%s", result)
	}
}

func TestTomlSection(t *testing.T) {
	m := map[string]any{
		"key": "value",
	}

	result := tomlSection("mysection", m)
	if !strings.Contains(result, "[mysection]") {
		t.Errorf("missing section header:\n%s", result)
	}
	if !strings.Contains(result, `key = "value"`) {
		t.Errorf("missing key value:\n%s", result)
	}
}

func TestTomlMarshal_NestedSections(t *testing.T) {
	m := map[string]any{
		"top": "value",
		"section": map[string]any{
			"inner": "deep",
		},
	}

	result := tomlMarshal(m, "")
	if !strings.Contains(result, `top = "value"`) {
		t.Errorf("missing top-level value:\n%s", result)
	}
	if !strings.Contains(result, "[section]") {
		t.Errorf("missing section header:\n%s", result)
	}
	if !strings.Contains(result, `inner = "deep"`) {
		t.Errorf("missing inner value:\n%s", result)
	}
}

func TestCoerceValue_AllTypes(t *testing.T) {
	tests := []struct {
		val       string
		fieldType string
		want      any
	}{
		{"true", "checkbox", true},
		{"false", "checkbox", false},
		{"on", "checkbox", true},
		{"off", "checkbox", false},
		{"42", "text", 42},
		{"3.14", "text", 3.14},
		{"hello", "text", "hello"},
		{"sk-ant-123", "password", "sk-ant-123"},
	}

	for _, tt := range tests {
		got := coerceValue(tt.val, tt.fieldType)
		if got != tt.want {
			t.Errorf("coerceValue(%q, %q) = %v (%T), want %v (%T)", tt.val, tt.fieldType, got, got, tt.want, tt.want)
		}
	}
}
