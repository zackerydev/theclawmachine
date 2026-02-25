package botenv

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// BuildConfigContent takes a BotConfig and a map of field key → value,
// and produces the config file content string (JSON or TOML).
// Only fields with a non-empty ConfigPath are included.
func BuildConfigContent(bot *BotConfig, values map[string]string) (string, error) {
	switch bot.ConfigFormat {
	case "json":
		return buildJSON(bot, values)
	case "toml":
		return buildTOML(bot, values)
	default:
		return "", nil // env-only bots don't generate a config file
	}
}

func buildJSON(bot *BotConfig, values map[string]string) (string, error) {
	root := make(map[string]any)

	for _, field := range bot.AllFields() {
		if field.ConfigPath == "" {
			continue
		}
		val, ok := values[field.Key]
		if !ok || val == "" {
			continue
		}
		setNestedValue(root, field.ConfigPath, coerceValue(val, field.Type))
	}

	if len(bot.ModelProviders) > 0 {
		if list := buildModelList(bot, values); len(list) > 0 {
			root["model_list"] = list
		}
		// Remove the deprecated providers key — model_list supersedes it.
		delete(root, "providers")
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config JSON: %w", err)
	}
	return string(data), nil
}

// buildModelList generates the LiteLLM model_list array from the bot's ModelProviders
// and the user's submitted form values. For each provider that has an API key value:
//   - if the selected model (AgentModelField) starts with one of the provider's ModelFilters,
//     that model value becomes the model_name
//   - otherwise DefaultModel is used (if set)
//
// The resulting "model" field is ModelPrefix + model_name.
func buildModelList(bot *BotConfig, values map[string]string) []map[string]any {
	selectedModel := values[bot.AgentModelField]

	var list []map[string]any
	for _, p := range bot.ModelProviders {
		apiKey := values[p.APIKeyField]
		if apiKey == "" {
			continue
		}

		modelName := ""
		modelFull := ""

		if selectedModelMatchesProvider(selectedModel, p) {
			modelName = selectedModel
			// If the selected model already carries this provider's prefix
			// (e.g. "anthropic/claude-sonnet-4.6"), keep it as-is to avoid
			// double-prefixing.
			if p.ModelPrefix != "" && strings.HasPrefix(selectedModel, p.ModelPrefix) {
				modelFull = selectedModel
			} else {
				modelFull = p.ModelPrefix + modelName
			}
		} else {
			modelName = p.DefaultModel
			modelFull = p.ModelPrefix + modelName
		}

		if modelName == "" {
			continue
		}

		entry := map[string]any{
			"model_name": modelName,
			"model":      modelFull,
			"api_key":    apiKey,
		}
		if p.APIBase != "" {
			entry["api_base"] = p.APIBase
		}
		list = append(list, entry)
	}
	return list
}

func selectedModelMatchesProvider(selectedModel string, p ModelProvider) bool {
	if selectedModel == "" {
		return false
	}

	if p.ModelPrefix != "" && strings.HasPrefix(selectedModel, p.ModelPrefix) {
		return true
	}

	for _, filter := range p.ModelFilters {
		if strings.HasPrefix(selectedModel, filter) {
			return true
		}
	}

	// Model picker values come from OpenRouter IDs (for example
	// "z-ai/glm-4.5-air:free"). These IDs don't start with "openrouter/" and
	// can't be fully enumerated via static filters, so treat them as valid
	// selected models when OpenRouter auth is configured.
	return p.Name == "openrouter" && strings.Contains(selectedModel, "/")
}

func buildTOML(bot *BotConfig, values map[string]string) (string, error) {
	// For TOML, build a nested map then serialize manually.
	// We keep it simple — flat key = value grouped by section.
	root := make(map[string]any)

	for _, field := range bot.AllFields() {
		if field.ConfigPath == "" {
			continue
		}
		val, ok := values[field.Key]
		if !ok || val == "" {
			continue
		}
		setNestedValue(root, field.ConfigPath, coerceValue(val, field.Type))
	}

	return tomlMarshal(root, ""), nil
}

// setNestedValue sets a value in a nested map using a dot-separated path.
func setNestedValue(m map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if i == len(parts)-1 {
			m[part] = value
			return
		}
		next, ok := m[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			m[part] = next
		}
		m = next
	}
}

// coerceValue converts a string value to the appropriate Go type.
func coerceValue(val string, fieldType string) any {
	if fieldType == "checkbox" {
		return val == "true" || val == "on"
	}
	// Try integer
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	// Try float
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return val
}

// tomlMarshal produces a simple TOML string from a nested map.
func tomlMarshal(m map[string]any, prefix string) string {
	var b strings.Builder
	var sections []string

	// Write scalar values first
	for k, v := range m {
		switch vt := v.(type) {
		case map[string]any:
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			sections = append(sections, tomlSection(key, vt))
		case string:
			fmt.Fprintf(&b, "%s = %q\n", k, vt)
		case bool:
			fmt.Fprintf(&b, "%s = %t\n", k, vt)
		case int:
			fmt.Fprintf(&b, "%s = %d\n", k, vt)
		case float64:
			fmt.Fprintf(&b, "%s = %f\n", k, vt)
		default:
			fmt.Fprintf(&b, "%s = %q\n", k, fmt.Sprint(vt))
		}
	}

	for _, s := range sections {
		b.WriteString(s)
	}

	return b.String()
}

func tomlSection(header string, m map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n[%s]\n", header)
	b.WriteString(tomlMarshal(m, header))
	return b.String()
}
