package handler

import (
	"context"
	"io"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// KubernetesManager defines the Kubernetes operations used by the handler.
type KubernetesManager interface {
	HasCRD(name string) bool
	GetPodLogs(ctx context.Context, namespace, releaseName string, tailLines int64) (string, error)
	GetReleasePodHealthy(ctx context.Context, namespace, releaseName string) (bool, error)
	RestartBot(ctx context.Context, namespace, releaseName string) error
	ExecInReleasePod(ctx context.Context, namespace, releaseName, container string, command []string) (string, string, error)
	ReadSecretData(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

// HelmManager defines the interface for Helm operations.
type HelmManager interface {
	Install(ctx context.Context, opts service.InstallOptions) (*service.ReleaseInfo, error)
	Upgrade(ctx context.Context, name, namespace string, botType service.BotType, values map[string]any) (*service.ReleaseInfo, error)
	Uninstall(name, namespace string) error
	List(namespace string) ([]service.ReleaseInfo, error)
	Status(name, namespace string) (*service.ReleaseInfo, error)
	GetValues(name, namespace string) (map[string]any, error)
	GetValuesAll(name, namespace string) (map[string]any, error)
}

// TemplateRenderer defines the interface for template rendering.
type TemplateRenderer interface {
	Render(w io.Writer, name string, data any, isHTMX bool) error
}

// ConnectServicer defines the interface for 1Password Connect Server operations.
type ConnectServicer interface {
	GetStatus(ctx context.Context) (*service.ConnectStatus, error)
	Install(ctx context.Context, credentialsJSON string, token string) error
	Uninstall(ctx context.Context) error
}

// SecretsManager defines the interface for secrets operations.
type SecretsManager interface {
	GetSecretStoreStatus(ctx context.Context, namespace string) (*service.SecretStoreStatus, error)
	ListExternalSecrets(ctx context.Context, namespace string) ([]service.ExternalSecretInfo, error)
	CreateSecretStore(ctx context.Context, namespace string, opts service.CreateSecretStoreOptions) error
	DeleteSecretStore(ctx context.Context, namespace string) error
	CreateExternalSecret(ctx context.Context, namespace string, opts service.CreateExternalSecretOptions) error
	DeleteExternalSecret(ctx context.Context, namespace, name string) error
}
