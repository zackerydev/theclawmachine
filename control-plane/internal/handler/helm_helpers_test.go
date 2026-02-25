package handler

import (
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/onboarding"
)

func TestFlattenValues(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		input  map[string]any
		want   map[string]any
	}{
		{
			name:   "flat",
			prefix: "",
			input:  map[string]any{"key": "val"},
			want:   map[string]any{"key": "val"},
		},
		{
			name:   "nested",
			prefix: "",
			input:  map[string]any{"a": map[string]any{"b": "c"}},
			want:   map[string]any{"a.b": "c"},
		},
		{
			name:   "deep nested",
			prefix: "",
			input:  map[string]any{"a": map[string]any{"b": map[string]any{"c": "d"}}},
			want:   map[string]any{"a.b.c": "d"},
		},
		{
			name:   "with prefix",
			prefix: "root",
			input:  map[string]any{"key": "val"},
			want:   map[string]any{"root.key": "val"},
		},
		{
			name:   "mixed types",
			prefix: "",
			input:  map[string]any{"str": "hello", "num": 42, "bool": true},
			want:   map[string]any{"str": "hello", "num": 42, "bool": true},
		},
		{
			name:   "empty map",
			prefix: "",
			input:  map[string]any{},
			want:   map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenValues(tt.prefix, tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("flattenValues() returned %d keys, want %d: %v", len(got), len(tt.want), got)
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("flattenValues()[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestDefaultNamespace(t *testing.T) {
	// When POD_NAMESPACE is not set, should return "claw-machine"
	ns := defaultNamespace()
	if ns != "claw-machine" {
		t.Errorf("defaultNamespace() = %q, want claw-machine", ns)
	}
}

func TestNormalizeEnvSecretEntry(t *testing.T) {
	t.Run("canonical shape", func(t *testing.T) {
		entry, ok := normalizeEnvSecretEntry(map[string]any{
			"envVar": "OPENAI_API_KEY",
			"secretRef": map[string]any{
				"name": "my-secret",
				"key":  "token",
			},
		})
		if !ok {
			t.Fatal("expected normalizeEnvSecretEntry to succeed")
		}
		secretRef := entry["secretRef"].(map[string]any)
		if secretRef["name"] != "my-secret" || secretRef["key"] != "token" {
			t.Fatalf("unexpected secretRef: %#v", secretRef)
		}
	})

	t.Run("legacy shape", func(t *testing.T) {
		entry, ok := normalizeEnvSecretEntry(map[string]any{
			"envVar":     "OPENAI_API_KEY",
			"secretName": "my-secret",
			"secretKey":  "value",
		})
		if !ok {
			t.Fatal("expected legacy env secret to normalize")
		}
		secretRef := entry["secretRef"].(map[string]any)
		if secretRef["name"] != "my-secret" || secretRef["key"] != "value" {
			t.Fatalf("unexpected normalized secretRef: %#v", secretRef)
		}
	})
}

func TestMergeEnvSecrets(t *testing.T) {
	existing := []any{
		map[string]any{
			"envVar":     "OPENAI_API_KEY",
			"secretName": "old-openai",
			"secretKey":  "value",
		},
		map[string]any{
			"envVar": "ANTHROPIC_API_KEY",
			"secretRef": map[string]any{
				"name": "anthropic-secret",
				"key":  "value",
			},
		},
	}
	additions := []onboarding.EnvSecretBinding{
		{
			EnvVar: "OPENAI_API_KEY",
			SecretRef: onboarding.SecretRef{
				Name: "new-openai",
				Key:  "value",
			},
		},
	}

	got := mergeEnvSecrets(existing, additions)
	if len(got) != 2 {
		t.Fatalf("mergeEnvSecrets len=%d, want 2", len(got))
	}
	first := got[0].(map[string]any)
	secretRef := first["secretRef"].(map[string]any)
	if secretRef["name"] != "new-openai" {
		t.Fatalf("OPENAI secret not overwritten, got %v", secretRef["name"])
	}
}
