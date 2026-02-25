package handler

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
)

const redactedSecretValue = "__CLAWMACHINE_REDACTED__"
const secretTokenCurrent = "{{secret.current}}"

var secretTokenRegex = regexp.MustCompile(`^\{\{secret\.([a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)\}\}$`)

// redactConfigContentForUI masks secret values in JSON config content before it
// is rendered in the browser.
func redactConfigContentForUI(botCfg *botenv.BotConfig, raw string, valueToSecretName map[string]string) (string, error) {
	root, err := parseConfigJSONObject(raw)
	if err != nil {
		return "", err
	}

	for _, pattern := range secretConfigPatterns(botCfg) {
		for _, path := range expandPatternPaths(pattern, root, root, nil) {
			v, ok := getJSONPath(root, path)
			if !ok {
				continue
			}
			replacement := secretTokenCurrent
			if s, ok := v.(string); ok && s != "" {
				if secretName, found := valueToSecretName[s]; found {
					replacement = secretToken(secretName)
				}
			}
			if !setJSONPath(root, path, replacement) {
				return "", fmt.Errorf("failed to redact secret field %s", formatJSONPath(path))
			}
		}
	}
	replaceMatchingSecretValues(root, valueToSecretName)

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal redacted config JSON: %w", err)
	}
	return string(data), nil
}

// mergeEditedConfigContent validates a redacted JSON payload from the UI,
// rejects secret edits, restores preserved secret values, and returns a
// normalized JSON document ready to save to Helm values.
func mergeEditedConfigContent(botCfg *botenv.BotConfig, currentRaw, editedRaw string, secretValuesByName map[string]string) (string, error) {
	currentRoot, err := parseConfigJSONObject(currentRaw)
	if err != nil {
		return "", fmt.Errorf("current config is not valid JSON: %w", err)
	}
	editedRoot, err := parseConfigJSONObject(editedRaw)
	if err != nil {
		return "", err
	}

	if err := validateAndPreserveSecretPaths(currentRoot, editedRoot, secretConfigPatterns(botCfg), secretValuesByName); err != nil {
		return "", err
	}
	if err := resolveSecretTokensEverywhere(editedRoot, secretValuesByName); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(editedRoot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config JSON: %w", err)
	}
	return string(data), nil
}

func parseConfigJSONObject(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var root any
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return nil, fmt.Errorf("config is not valid JSON: %w", err)
	}

	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config JSON must be an object")
	}
	return obj, nil
}

func secretConfigPatterns(botCfg *botenv.BotConfig) [][]string {
	if botCfg == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var out [][]string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, strings.Split(path, "."))
	}

	for _, field := range botCfg.AllFields() {
		if !field.Secret || field.ConfigPath == "" {
			continue
		}
		add(field.ConfigPath)
	}

	// PicoClaw model providers embed API keys under model_list[].api_key.
	if len(botCfg.ModelProviders) > 0 {
		add("model_list.*.api_key")
	}

	return out
}

func validateAndPreserveSecretPaths(currentRoot, editedRoot map[string]any, patterns [][]string, secretValuesByName map[string]string) error {
	for _, pattern := range patterns {
		paths := expandPatternPaths(pattern, currentRoot, editedRoot, nil)
		for _, path := range paths {
			currentVal, currentExists := getJSONPath(currentRoot, path)
			editedVal, editedExists := getJSONPath(editedRoot, path)
			pathText := formatJSONPath(path)

			switch {
			case currentExists:
				if !editedExists {
					return fmt.Errorf("secret field %s cannot be removed in the JSON editor", pathText)
				}
				if isPreserveToken(editedVal) {
					if !setJSONPath(editedRoot, path, currentVal) {
						return fmt.Errorf("failed to restore secret field %s", pathText)
					}
					continue
				}
				if secretName, ok := parseSecretToken(editedVal); ok {
					secretVal, found := secretValuesByName[secretName]
					if !found {
						return fmt.Errorf("secret field %s references unknown secret %q; choose a synced secret from Settings", pathText, secretName)
					}
					if !setJSONPath(editedRoot, path, secretVal) {
						return fmt.Errorf("failed to resolve secret token for field %s", pathText)
					}
					continue
				}
				if reflect.DeepEqual(editedVal, currentVal) {
					continue
				}
				return fmt.Errorf("secret field %s cannot be edited in the JSON editor; manage secrets in Settings", pathText)

			default:
				if editedExists {
					return fmt.Errorf("secret field %s cannot be added in the JSON editor; manage secrets in Settings", pathText)
				}
			}
		}
	}
	return nil
}

func isPreserveToken(v any) bool {
	s, ok := v.(string)
	return ok && (s == redactedSecretValue || s == secretTokenCurrent)
}

func parseSecretToken(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	m := secretTokenRegex.FindStringSubmatch(s)
	if len(m) != 3 {
		return "", false
	}
	return m[1], true
}

func secretToken(secretName string) string {
	return "{{secret." + secretName + "}}"
}

func resolveSecretTokensEverywhere(node any, secretValuesByName map[string]string) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok {
				if s == secretTokenCurrent {
					continue
				}
				if name, tok := parseSecretToken(s); tok {
					secretVal, found := secretValuesByName[name]
					if !found {
						return fmt.Errorf("secret token %q references unknown secret; choose a synced secret from Settings", s)
					}
					v[k] = secretVal
					continue
				}
			}
			if err := resolveSecretTokensEverywhere(child, secretValuesByName); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				if s == secretTokenCurrent {
					continue
				}
				if name, tok := parseSecretToken(s); tok {
					secretVal, found := secretValuesByName[name]
					if !found {
						return fmt.Errorf("secret token %q references unknown secret; choose a synced secret from Settings", s)
					}
					v[i] = secretVal
					continue
				}
			}
			if err := resolveSecretTokensEverywhere(child, secretValuesByName); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceMatchingSecretValues(node any, valueToSecretName map[string]string) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok {
				if name, found := valueToSecretName[s]; found {
					v[k] = secretToken(name)
					continue
				}
			}
			replaceMatchingSecretValues(child, valueToSecretName)
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				if name, found := valueToSecretName[s]; found {
					v[i] = secretToken(name)
					continue
				}
			}
			replaceMatchingSecretValues(child, valueToSecretName)
		}
	}
}

func expandPatternPaths(pattern []string, current, edited any, prefix []string) [][]string {
	if len(pattern) == 0 {
		return [][]string{append([]string(nil), prefix...)}
	}

	seg := pattern[0]
	if seg == "*" {
		currentArr, _ := current.([]any)
		editedArr, _ := edited.([]any)
		n := max(len(editedArr), len(currentArr))
		if n == 0 {
			return nil
		}

		var out [][]string
		for i := range n {
			var currentChild, editedChild any
			if i < len(currentArr) {
				currentChild = currentArr[i]
			}
			if i < len(editedArr) {
				editedChild = editedArr[i]
			}
			nextPrefix := append(append([]string(nil), prefix...), strconv.Itoa(i))
			out = append(out, expandPatternPaths(pattern[1:], currentChild, editedChild, nextPrefix)...)
		}
		return out
	}

	var currentChild, editedChild any
	if m, ok := current.(map[string]any); ok {
		currentChild = m[seg]
	}
	if m, ok := edited.(map[string]any); ok {
		editedChild = m[seg]
	}

	nextPrefix := append(append([]string(nil), prefix...), seg)
	return expandPatternPaths(pattern[1:], currentChild, editedChild, nextPrefix)
}

func getJSONPath(root any, path []string) (any, bool) {
	cur := root
	for _, seg := range path {
		if idx, err := strconv.Atoi(seg); err == nil {
			arr, ok := cur.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
			continue
		}

		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := obj[seg]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func setJSONPath(root any, path []string, value any) bool {
	if len(path) == 0 {
		return false
	}

	cur := root
	for _, seg := range path[:len(path)-1] {
		if idx, err := strconv.Atoi(seg); err == nil {
			arr, ok := cur.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return false
			}
			cur = arr[idx]
			continue
		}

		obj, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		next, ok := obj[seg]
		if !ok {
			return false
		}
		cur = next
	}

	last := path[len(path)-1]
	if idx, err := strconv.Atoi(last); err == nil {
		arr, ok := cur.([]any)
		if !ok || idx < 0 || idx >= len(arr) {
			return false
		}
		arr[idx] = value
		return true
	}

	obj, ok := cur.(map[string]any)
	if !ok {
		return false
	}
	obj[last] = value
	return true
}

func formatJSONPath(path []string) string {
	if len(path) == 0 {
		return "(root)"
	}
	var b strings.Builder
	for _, seg := range path {
		if idx, err := strconv.Atoi(seg); err == nil {
			fmt.Fprintf(&b, "[%d]", idx)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}
