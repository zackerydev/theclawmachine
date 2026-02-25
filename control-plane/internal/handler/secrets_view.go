package handler

import "context"

func availableSecretsForTemplates(ctx context.Context, secretsSvc SecretsManager) []AvailableSecret {
	var secrets []AvailableSecret
	if secretsSvc == nil {
		return secrets
	}

	all, err := secretsSvc.ListExternalSecrets(ctx, defaultNamespace())
	if err != nil {
		return secrets
	}

	for _, s := range all {
		if s.Status != "Synced" {
			continue
		}
		secrets = append(secrets, AvailableSecret{Name: s.Name, TargetSecret: s.TargetSecret})
	}
	return secrets
}
