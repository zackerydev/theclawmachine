package service

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BotSecretsService manages Kubernetes secrets for bot releases out-of-band
// from Helm. Secrets are created directly via the K8s API so they never appear
// in Helm release history (etcd).
type BotSecretsService struct {
	clientset kubernetes.Interface
}

func NewBotSecretsService(k8s *KubernetesService) *BotSecretsService {
	return &BotSecretsService{clientset: k8s.Clientset()}
}

// BotSecretName returns the conventional secret name for a release.
func BotSecretName(releaseName string) string {
	return releaseName + "-bot-secrets"
}

// CreateOrUpdate creates or updates the bot secret for a release.
// Only non-empty values are stored. Returns the secret name.
func (s *BotSecretsService) CreateOrUpdate(ctx context.Context, releaseName, namespace string, data map[string]string) (string, error) {
	name := BotSecretName(releaseName)

	// Filter out empty values
	filtered := make(map[string][]byte)
	for k, v := range data {
		if v != "" {
			filtered[k] = []byte(v)
		}
	}

	if len(filtered) == 0 {
		slog.Info("no secrets to create", "release", releaseName)
		return name, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "clawmachine",
				"app.kubernetes.io/instance":   releaseName,
				"clawmachine.dev/secret-type":  "bot-secrets",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: filtered,
	}

	existing, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("checking existing secret: %w", err)
		}
		// Create
		_, err = s.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("creating bot secret %q: %w", name, err)
		}
		slog.Info("created bot secret", "name", name, "namespace", namespace, "keys", len(filtered))
		return name, nil
	}

	// Update — merge new keys into existing
	existing.Data = filtered
	existing.Labels = secret.Labels
	_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("updating bot secret %q: %w", name, err)
	}
	slog.Info("updated bot secret", "name", name, "namespace", namespace, "keys", len(filtered))
	return name, nil
}

// Delete removes the bot secret for a release.
func (s *BotSecretsService) Delete(ctx context.Context, releaseName, namespace string) error {
	name := BotSecretName(releaseName)
	err := s.clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // already gone
		}
		return fmt.Errorf("deleting bot secret %q: %w", name, err)
	}
	slog.Info("deleted bot secret", "name", name, "namespace", namespace)
	return nil
}

// Get retrieves the bot secret key names (not values) for display.
func (s *BotSecretsService) Get(ctx context.Context, releaseName, namespace string) ([]string, error) {
	name := BotSecretName(releaseName)
	secret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting bot secret %q: %w", name, err)
	}
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	return keys, nil
}

// ExtractSecrets pulls secret values out of the Helm values map and returns
// them separately. The values map is modified in-place to remove secret data
// and add the secretName reference instead.
func ExtractSecrets(releaseName string, values map[string]any) map[string]string {
	secrets := make(map[string]string)

	// Extract from values.secrets (used by all chart types)
	if secretsMap, ok := values["secrets"].(map[string]any); ok {
		for k, v := range secretsMap {
			if s, ok := v.(string); ok && s != "" {
				secrets[secretKeyToEnv(k)] = s
			}
		}
		delete(values, "secrets")
	}

	// Extract database.url (IronClaw) — it's a connection string with credentials
	if db, ok := values["database"].(map[string]any); ok {
		if url, ok := db["url"].(string); ok && url != "" {
			secrets["DATABASE_URL"] = url
			delete(db, "url")
		}
	}

	// Set the external secret name reference
	values["externalSecretName"] = BotSecretName(releaseName)

	return secrets
}

// secretKeyToEnv maps camelCase secret keys to ENV_VAR names.
func secretKeyToEnv(key string) string {
	envMap := map[string]string{
		"gatewayAuthToken":          "GATEWAY_AUTH_TOKEN",
		"gatewayToken":              "OPENCLAW_GATEWAY_TOKEN",
		"nearaiSessionToken":        "NEARAI_SESSION_TOKEN",
		"anthropicApiKey":           "ANTHROPIC_API_KEY",
		"openaiApiKey":              "OPENAI_API_KEY",
		"geminiApiKey":              "GEMINI_API_KEY",
		"customApiKey":              "CUSTOM_API_KEY",
		"cloudflareAiGatewayApiKey": "CLOUDFLARE_AI_GATEWAY_API_KEY",
		"discordBotToken":           "DISCORD_BOT_TOKEN",
		"telegramBotToken":          "TELEGRAM_BOT_TOKEN",
		"slackBotToken":             "SLACK_BOT_TOKEN",
		"slackAppToken":             "SLACK_APP_TOKEN",
	}
	if env, ok := envMap[key]; ok {
		return env
	}
	return key
}
