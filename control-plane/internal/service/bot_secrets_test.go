package service

import (
	"context"
	"maps"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBotSecretName(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		want        string
	}{
		{"simple name", "mybot", "mybot-bot-secrets"},
		{"hyphenated name", "my-cool-bot", "my-cool-bot-bot-secrets"},
		{"empty name", "", "-bot-secrets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BotSecretName(tt.releaseName)
			if got != tt.want {
				t.Errorf("BotSecretName(%q) = %q, want %q", tt.releaseName, got, tt.want)
			}
		})
	}
}

func TestExtractSecrets(t *testing.T) {
	tests := []struct {
		name          string
		releaseName   string
		values        map[string]any
		wantSecrets   map[string]string
		wantNoSecrets bool // values should not have "secrets" key after
		wantExtSecret string
	}{
		{
			name:        "extracts secrets and maps to env vars",
			releaseName: "mybot",
			values: map[string]any{
				"secrets": map[string]any{
					"gatewayAuthToken": "token123",
					"anthropicApiKey":  "sk-ant-xxx",
				},
				"other": "value",
			},
			wantSecrets: map[string]string{
				"GATEWAY_AUTH_TOKEN": "token123",
				"ANTHROPIC_API_KEY":  "sk-ant-xxx",
			},
			wantNoSecrets: true,
			wantExtSecret: "mybot-bot-secrets",
		},
		{
			name:        "removes secrets key from values",
			releaseName: "bot1",
			values: map[string]any{
				"secrets": map[string]any{
					"discordBotToken": "discord-token",
				},
			},
			wantSecrets: map[string]string{
				"DISCORD_BOT_TOKEN": "discord-token",
			},
			wantNoSecrets: true,
			wantExtSecret: "bot1-bot-secrets",
		},
		{
			name:        "extracts database url from ironclaw values",
			releaseName: "iron1",
			values: map[string]any{
				"database": map[string]any{
					"url":  "postgres://user:pass@host/db",
					"pool": 5,
				},
			},
			wantSecrets: map[string]string{
				"DATABASE_URL": "postgres://user:pass@host/db",
			},
			wantExtSecret: "iron1-bot-secrets",
		},
		{
			name:        "returns empty map when no secrets present",
			releaseName: "nobot",
			values: map[string]any{
				"replicas": 1,
			},
			wantSecrets:   map[string]string{},
			wantExtSecret: "nobot-bot-secrets",
		},
		{
			name:          "handles empty values map",
			releaseName:   "nilbot",
			values:        map[string]any{},
			wantSecrets:   map[string]string{},
			wantExtSecret: "nilbot-bot-secrets",
		},
		{
			name:        "skips empty secret values",
			releaseName: "emptybot",
			values: map[string]any{
				"secrets": map[string]any{
					"gatewayAuthToken": "",
					"anthropicApiKey":  "real-key",
				},
			},
			wantSecrets: map[string]string{
				"ANTHROPIC_API_KEY": "real-key",
			},
			wantNoSecrets: true,
			wantExtSecret: "emptybot-bot-secrets",
		},
		{
			name:        "maps all known camelCase keys correctly",
			releaseName: "allkeys",
			values: map[string]any{
				"secrets": map[string]any{
					"gatewayToken":              "gw-tok",
					"openaiApiKey":              "oai-key",
					"geminiApiKey":              "gem-key",
					"customApiKey":              "cust-key",
					"cloudflareAiGatewayApiKey": "cf-key",
					"telegramBotToken":          "tg-tok",
					"slackBotToken":             "sl-bot",
					"slackAppToken":             "sl-app",
					"nearaiSessionToken":        "near-tok",
				},
			},
			wantSecrets: map[string]string{
				"OPENCLAW_GATEWAY_TOKEN":        "gw-tok",
				"OPENAI_API_KEY":                "oai-key",
				"GEMINI_API_KEY":                "gem-key",
				"CUSTOM_API_KEY":                "cust-key",
				"CLOUDFLARE_AI_GATEWAY_API_KEY": "cf-key",
				"TELEGRAM_BOT_TOKEN":            "tg-tok",
				"SLACK_BOT_TOKEN":               "sl-bot",
				"SLACK_APP_TOKEN":               "sl-app",
				"NEARAI_SESSION_TOKEN":          "near-tok",
			},
			wantNoSecrets: true,
			wantExtSecret: "allkeys-bot-secrets",
		},
		{
			name:        "database url removed but other db fields remain",
			releaseName: "dbbot",
			values: map[string]any{
				"database": map[string]any{
					"url":  "postgres://localhost/db",
					"pool": 10,
				},
			},
			wantSecrets: map[string]string{
				"DATABASE_URL": "postgres://localhost/db",
			},
			wantExtSecret: "dbbot-bot-secrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSecrets(tt.releaseName, tt.values)

			// Check expected secrets
			if tt.wantSecrets != nil {
				if len(got) != len(tt.wantSecrets) {
					t.Errorf("got %d secrets, want %d\ngot: %v\nwant: %v", len(got), len(tt.wantSecrets), got, tt.wantSecrets)
				}
				for k, v := range tt.wantSecrets {
					if got[k] != v {
						t.Errorf("secret[%q] = %q, want %q", k, got[k], v)
					}
				}
			}

			// Check secrets key removed from values
			if tt.wantNoSecrets {
				if _, exists := tt.values["secrets"]; exists {
					t.Error("secrets key should be removed from values after extraction")
				}
			}

			// Check externalSecretName set
			if tt.wantExtSecret != "" {
				esn, ok := tt.values["externalSecretName"].(string)
				if !ok || esn != tt.wantExtSecret {
					t.Errorf("externalSecretName = %q, want %q", esn, tt.wantExtSecret)
				}
			}

			// For database test, verify url removed but pool remains
			if tt.name == "database url removed but other db fields remain" {
				db := tt.values["database"].(map[string]any)
				if _, exists := db["url"]; exists {
					t.Error("database.url should be removed")
				}
				if db["pool"] != 10 {
					t.Error("database.pool should remain")
				}
			}
		})
	}
}

func TestBotSecretsService_CreateOrUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("creates secret and filters empty values", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService())

		name, err := svc.CreateOrUpdate(ctx, "demo", "default", map[string]string{
			"ANTHROPIC_API_KEY": "secret-value",
			"EMPTY":             "",
		})
		if err != nil {
			t.Fatalf("CreateOrUpdate error: %v", err)
		}
		if name != "demo-bot-secrets" {
			t.Fatalf("name = %q, want %q", name, "demo-bot-secrets")
		}

		got, err := svc.clientset.CoreV1().Secrets("default").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("getting created secret: %v", err)
		}
		if len(got.Data) != 1 {
			t.Fatalf("expected 1 key after filtering empties, got %d", len(got.Data))
		}
		if string(got.Data["ANTHROPIC_API_KEY"]) != "secret-value" {
			t.Fatalf("ANTHROPIC_API_KEY = %q, want %q", string(got.Data["ANTHROPIC_API_KEY"]), "secret-value")
		}
		if got.Labels["app.kubernetes.io/managed-by"] != "clawmachine" {
			t.Fatalf("managed-by label = %q, want %q", got.Labels["app.kubernetes.io/managed-by"], "clawmachine")
		}
	})

	t.Run("updates existing secret", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-bot-secrets",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"OLD_KEY": []byte("old"),
				},
			},
		))

		name, err := svc.CreateOrUpdate(ctx, "demo", "default", map[string]string{
			"NEW_KEY": "new",
		})
		if err != nil {
			t.Fatalf("CreateOrUpdate update path error: %v", err)
		}
		if name != "demo-bot-secrets" {
			t.Fatalf("name = %q, want %q", name, "demo-bot-secrets")
		}

		got, err := svc.clientset.CoreV1().Secrets("default").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("getting updated secret: %v", err)
		}
		if len(got.Data) != 1 {
			t.Fatalf("expected updated secret to contain 1 key, got %d", len(got.Data))
		}
		if _, exists := got.Data["OLD_KEY"]; exists {
			t.Fatal("OLD_KEY should be replaced on update")
		}
		if string(got.Data["NEW_KEY"]) != "new" {
			t.Fatalf("NEW_KEY = %q, want %q", string(got.Data["NEW_KEY"]), "new")
		}
	})
}

func TestBotSecretsService_CreateOrUpdate_NoSecrets(t *testing.T) {
	ctx := context.Background()
	svc := NewBotSecretsService(newTestKubernetesService())

	name, err := svc.CreateOrUpdate(ctx, "demo", "default", map[string]string{
		"EMPTY1": "",
		"EMPTY2": "",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdate error: %v", err)
	}
	if name != "demo-bot-secrets" {
		t.Fatalf("name = %q, want %q", name, "demo-bot-secrets")
	}

	_, err = svc.clientset.CoreV1().Secrets("default").Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected no secret to be created when all values are empty")
	}
}

func TestBotSecretsService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes existing secret", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-bot-secrets",
					Namespace: "default",
				},
			},
		))

		if err := svc.Delete(ctx, "demo", "default"); err != nil {
			t.Fatalf("Delete error: %v", err)
		}

		_, err := svc.clientset.CoreV1().Secrets("default").Get(ctx, "demo-bot-secrets", metav1.GetOptions{})
		if err == nil {
			t.Fatal("expected secret to be deleted")
		}
	})

	t.Run("missing secret is a no-op", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService())
		if err := svc.Delete(ctx, "demo", "default"); err != nil {
			t.Fatalf("Delete should ignore missing secret: %v", err)
		}
	})
}

func TestBotSecretsService_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil for missing secret", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService())
		keys, err := svc.Get(ctx, "demo", "default")
		if err != nil {
			t.Fatalf("Get error: %v", err)
		}
		if keys != nil {
			t.Fatalf("keys = %v, want nil", keys)
		}
	})

	t.Run("returns key names for existing secret", func(t *testing.T) {
		svc := NewBotSecretsService(newTestKubernetesService(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-bot-secrets",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"KEY_A": []byte("a"),
					"KEY_B": []byte("b"),
				},
			},
		))

		keys, err := svc.Get(ctx, "demo", "default")
		if err != nil {
			t.Fatalf("Get error: %v", err)
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(keys))
		}

		got := map[string]bool{}
		for _, k := range keys {
			got[k] = true
		}
		want := map[string]bool{"KEY_A": true, "KEY_B": true}
		if !maps.Equal(got, want) {
			t.Fatalf("keys = %v, want %v", got, want)
		}
	})
}
