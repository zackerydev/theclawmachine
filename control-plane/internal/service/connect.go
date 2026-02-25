package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	helmkube "helm.sh/helm/v4/pkg/kube"
)

const (
	opConnectReleaseName = "connect"
	opConnectNamespace   = "1password"
)

// ConnectStatus represents the current state of the 1Password Connect Server.
type ConnectStatus struct {
	Installed bool
	Ready     bool
	Message   string
	Host      string // In-cluster service URL
}

// ConnectService manages the 1Password Connect Server Helm release.
type ConnectService struct {
	clientset    kubernetes.Interface
	helmSettings *cli.EnvSettings
}

func NewConnectService(clientset kubernetes.Interface, kubeConfigPath, kubeContext string, inCluster bool) *ConnectService {
	settings := cli.New()
	if !inCluster {
		settings.KubeConfig = kubeConfigPath
		if kubeContext != "" {
			settings.KubeContext = kubeContext
		}
	}
	return &ConnectService{clientset: clientset, helmSettings: settings}
}

func (s *ConnectService) initActionConfig(namespace string) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	if err := cfg.Init(s.helmSettings.RESTClientGetter(), namespace, "secrets"); err != nil {
		return nil, fmt.Errorf("initializing helm action config: %w", err)
	}
	return cfg, nil
}

// GetStatus checks if the 1Password Connect Server is installed and healthy.
func (s *ConnectService) GetStatus(ctx context.Context) (*ConnectStatus, error) {
	cfg, err := s.initActionConfig(opConnectNamespace)
	if err != nil {
		return &ConnectStatus{Installed: false, Message: "Cannot reach cluster"}, nil
	}

	listClient := action.NewList(cfg)
	listClient.Filter = opConnectReleaseName
	results, err := listClient.Run()
	if err != nil {
		return &ConnectStatus{Installed: false}, nil
	}

	if len(results) == 0 {
		return &ConnectStatus{Installed: false}, nil
	}

	status := &ConnectStatus{
		Installed: true,
		Host:      fmt.Sprintf("http://onepassword-connect.%s.svc:8080", opConnectNamespace),
	}

	// Check if pod is ready
	pods, err := s.clientset.CoreV1().Pods(opConnectNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=onepassword-connect",
	})
	if err == nil && len(pods.Items) > 0 {
		for _, cond := range pods.Items[0].Status.Conditions {
			if cond.Type == corev1.PodReady {
				status.Ready = cond.Status == corev1.ConditionTrue
				if !status.Ready {
					status.Message = cond.Message
				}
			}
		}
	}

	return status, nil
}

// Install deploys the 1Password Connect Server via Helm and creates the
// op-credentials secret from the provided credentials JSON.
func (s *ConnectService) Install(ctx context.Context, credentialsJSON string, token string) error {
	cfg, err := s.initActionConfig(opConnectNamespace)
	if err != nil {
		return err
	}

	// Ensure namespace exists
	_, err = s.clientset.CoreV1().Namespaces().Get(ctx, opConnectNamespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = s.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: opConnectNamespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "clawmachine",
					},
				},
			}, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("creating namespace: %w", err)
			}
		} else {
			return fmt.Errorf("checking namespace: %w", err)
		}
	}

	// Create op-credentials secret
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "op-credentials",
			Namespace: opConnectNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "clawmachine",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"1password-credentials.json": credentialsJSON,
		},
	}

	existing, err := s.clientset.CoreV1().Secrets(opConnectNamespace).Get(ctx, credSecret.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking existing credentials secret: %w", err)
		}
		_, err = s.clientset.CoreV1().Secrets(opConnectNamespace).Create(ctx, credSecret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating credentials secret: %w", err)
		}
	} else {
		existing.StringData = credSecret.StringData
		existing.Labels = credSecret.Labels
		_, err = s.clientset.CoreV1().Secrets(opConnectNamespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating credentials secret: %w", err)
		}
	}

	// Create op-token secret
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-token",
			Namespace: opConnectNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "clawmachine",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": token,
		},
	}

	existing, err = s.clientset.CoreV1().Secrets(opConnectNamespace).Get(ctx, tokenSecret.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking existing token secret: %w", err)
		}
		_, err = s.clientset.CoreV1().Secrets(opConnectNamespace).Create(ctx, tokenSecret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating token secret: %w", err)
		}
	} else {
		existing.StringData = tokenSecret.StringData
		existing.Labels = tokenSecret.Labels
		_, err = s.clientset.CoreV1().Secrets(opConnectNamespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating token secret: %w", err)
		}
	}

	// Check if already installed — upgrade if so, install if not
	listClient := action.NewList(cfg)
	listClient.Filter = opConnectReleaseName
	releases, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("checking existing releases: %w", err)
	}

	vals := map[string]any{
		"connect": map[string]any{
			"credentials_json_base64": "",
		},
	}

	alreadyInstalled := len(releases) > 0

	if alreadyInstalled {
		// Upgrade existing release and return without waiting for readiness;
		// the UI polls status while pods roll out.
		upgradeClient := action.NewUpgrade(cfg)
		upgradeClient.Namespace = opConnectNamespace
		upgradeClient.WaitStrategy = helmkube.HookOnlyStrategy

		chrt, err := loader.LoadArchive(bytes.NewReader(GetConnectChart()))
		if err != nil {
			return fmt.Errorf("loading 1password-connect chart: %w", err)
		}
		if _, err := upgradeClient.Run(opConnectReleaseName, chrt, vals); err != nil {
			return fmt.Errorf("upgrading 1password-connect: %w", err)
		}
	} else {
		// Fresh install and return without waiting for readiness;
		// the UI polls status while pods roll out.
		installClient := action.NewInstall(cfg)
		installClient.ReleaseName = opConnectReleaseName
		installClient.Namespace = opConnectNamespace
		installClient.CreateNamespace = true
		installClient.WaitStrategy = helmkube.HookOnlyStrategy

		chrt, err := loader.LoadArchive(bytes.NewReader(GetConnectChart()))
		if err != nil {
			return fmt.Errorf("loading 1password-connect chart: %w", err)
		}
		if _, err := installClient.Run(chrt, vals); err != nil {
			return fmt.Errorf("installing 1password-connect: %w", err)
		}
	}

	return nil
}

// Uninstall removes the 1Password Connect Server and its secrets.
func (s *ConnectService) Uninstall(ctx context.Context) error {
	cfg, err := s.initActionConfig(opConnectNamespace)
	if err != nil {
		return err
	}

	listClient := action.NewList(cfg)
	listClient.Filter = opConnectReleaseName
	results, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}

	if len(results) > 0 {
		uninstallClient := action.NewUninstall(cfg)
		uninstallClient.WaitStrategy = helmkube.StatusWatcherStrategy
		uninstallClient.Timeout = 2 * time.Minute
		if _, err := uninstallClient.Run(opConnectReleaseName); err != nil {
			return fmt.Errorf("uninstalling 1password-connect: %w", err)
		}
	}

	// Clean up secrets
	for _, name := range []string{"op-credentials", "onepassword-token"} {
		err := s.clientset.CoreV1().Secrets(opConnectNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting secret %s: %w", name, err)
		}
	}

	return nil
}
