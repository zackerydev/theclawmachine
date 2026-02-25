package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	secretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "secretstores",
	}
	externalSecretGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "externalsecrets",
	}
)

type SecretStoreStatus struct {
	Configured  bool
	Name        string
	Provider    string
	ConnectHost string
	VaultName   string
	Ready       bool
	Message     string
}

type ExternalSecretInfo struct {
	Name           string
	Namespace      string
	SecretStore    string
	TargetSecret   string
	Status         string
	LastSynced     string
	DataKeys       []string
	RemoteKey      string // First data entry's remoteRef.key (for display)
	RemoteProperty string // First data entry's remoteRef.property (for display)
}

type CreateSecretStoreOptions struct {
	Provider        string `json:"provider"` // "onepassword", "aws-ssm", "vault" (extensible)
	ConnectHost     string `json:"connectHost"`
	ConnectToken    string `json:"connectToken"`
	VaultName       string `json:"vaultName"`
	DeployConnect   bool   `json:"deployConnect"`   // Deploy 1Password Connect into cluster
	CredentialsJSON string `json:"credentialsJson"` // 1password-credentials.json contents
}

type CreateExternalSecretOptions struct {
	Name            string               `json:"name"`
	SecretStore     string               `json:"secretStore"`
	TargetSecret    string               `json:"targetSecret"`
	RefreshInterval string               `json:"refreshInterval"`
	Data            []ExternalSecretData `json:"data"`
}

type ExternalSecretData struct {
	SecretKey      string `json:"secretKey"`
	RemoteKey      string `json:"remoteKey"`
	RemoteProperty string `json:"remoteProperty"`
}

type SecretsService struct {
	clientset kubernetes.Interface
	dynamic   dynamic.Interface
}

func NewSecretsService(clientset kubernetes.Interface, dynamic dynamic.Interface) *SecretsService {
	return &SecretsService{clientset: clientset, dynamic: dynamic}
}

func (s *SecretsService) GetSecretStoreStatus(ctx context.Context, namespace string) (*SecretStoreStatus, error) {
	list, err := s.dynamic.Resource(secretStoreGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=clawmachine",
	})
	if err != nil {
		if errors.IsNotFound(err) || isNoMatchError(err) {
			return &SecretStoreStatus{Configured: false, Message: "External Secrets Operator is not installed"}, nil
		}
		return nil, fmt.Errorf("listing secret stores: %w", err)
	}

	if len(list.Items) == 0 {
		return &SecretStoreStatus{Configured: false}, nil
	}

	store := list.Items[0]
	status := &SecretStoreStatus{
		Configured: true,
		Name:       store.GetName(),
	}

	// Parse provider info
	provider, _, _ := unstructured.NestedMap(store.Object, "spec", "provider", "onepassword")
	if provider != nil {
		status.Provider = "1Password"
		status.ConnectHost, _, _ = unstructured.NestedString(provider, "connectHost")

		vaults, _, _ := unstructured.NestedMap(provider, "vaults")
		for name := range vaults {
			status.VaultName = name
			break
		}
	}

	// Parse status conditions
	conditions, _, _ := unstructured.NestedSlice(store.Object, "status", "conditions")
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		if condType == "Ready" {
			status.Ready = condStatus == "True"
			status.Message, _, _ = unstructured.NestedString(cond, "message")
		}
	}

	return status, nil
}

func (s *SecretsService) CreateSecretStore(ctx context.Context, namespace string, opts CreateSecretStoreOptions) error {
	if opts.Provider == "" {
		opts.Provider = "onepassword" // backwards compat
	}

	switch opts.Provider {
	case "onepassword":
		return s.createOnePasswordStore(ctx, namespace, opts)
	default:
		return fmt.Errorf("unsupported provider: %s", opts.Provider)
	}
}

func (s *SecretsService) createOnePasswordStore(ctx context.Context, namespace string, opts CreateSecretStoreOptions) error {
	// If deploying Connect into the cluster, create the credentials secret and deployment
	if opts.DeployConnect && opts.CredentialsJSON != "" {
		if err := s.deployOnePasswordConnect(ctx, namespace, opts.CredentialsJSON); err != nil {
			return fmt.Errorf("deploying 1Password Connect: %w", err)
		}
		// When deployed in-cluster, Connect is reachable at the service URL
		opts.ConnectHost = fmt.Sprintf("http://onepassword-connect.%s.svc.cluster.local:8080", namespace)
	}

	// Create or update the connect token secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-connect-token",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "clawmachine",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": opts.ConnectToken,
		},
	}

	existing, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking existing secret: %w", err)
		}
		_, err = s.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating connect token secret: %w", err)
		}
	} else {
		existing.StringData = secret.StringData
		existing.Labels = secret.Labels
		_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating connect token secret: %w", err)
		}
	}

	// Build SecretStore CRD
	store := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "external-secrets.io/v1",
			"kind":       "SecretStore",
			"metadata": map[string]any{
				"name":      "onepassword-store",
				"namespace": namespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "clawmachine",
				},
			},
			"spec": map[string]any{
				"provider": map[string]any{
					"onepassword": map[string]any{
						"connectHost": opts.ConnectHost,
						"vaults": map[string]any{
							opts.VaultName: int64(1),
						},
						"auth": map[string]any{
							"secretRef": map[string]any{
								"connectTokenSecretRef": map[string]any{
									"name": "onepassword-connect-token",
									"key":  "token",
								},
							},
						},
					},
				},
			},
		},
	}

	// Get-then-Create-or-Update
	existingStore, err := s.dynamic.Resource(secretStoreGVR).Namespace(namespace).Get(ctx, "onepassword-store", metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking existing secret store: %w", err)
		}
		_, err = s.dynamic.Resource(secretStoreGVR).Namespace(namespace).Create(ctx, store, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating secret store: %w", err)
		}
	} else {
		store.SetResourceVersion(existingStore.GetResourceVersion())
		_, err = s.dynamic.Resource(secretStoreGVR).Namespace(namespace).Update(ctx, store, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating secret store: %w", err)
		}
	}

	return nil
}

func (s *SecretsService) DeleteSecretStore(ctx context.Context, namespace string) error {
	// Delete the ESO SecretStore
	err := s.dynamic.Resource(secretStoreGVR).Namespace(namespace).Delete(ctx, "onepassword-store", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting secret store: %w", err)
	}

	// Delete the connect token secret
	err = s.clientset.CoreV1().Secrets(namespace).Delete(ctx, "onepassword-connect-token", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting connect token secret: %w", err)
	}

	// Clean up in-cluster Connect deployment (if it exists)
	err = s.clientset.AppsV1().Deployments(namespace).Delete(ctx, "onepassword-connect", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting Connect deployment: %w", err)
	}
	err = s.clientset.CoreV1().Services(namespace).Delete(ctx, "onepassword-connect", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting Connect service: %w", err)
	}
	err = s.clientset.CoreV1().Secrets(namespace).Delete(ctx, "onepassword-credentials", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting credentials secret: %w", err)
	}

	return nil
}

func (s *SecretsService) ListExternalSecrets(ctx context.Context, namespace string) ([]ExternalSecretInfo, error) {
	list, err := s.dynamic.Resource(externalSecretGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=clawmachine",
	})
	if err != nil {
		if errors.IsNotFound(err) || isNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing external secrets: %w", err)
	}

	var results []ExternalSecretInfo
	for _, item := range list.Items {
		info := ExternalSecretInfo{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		}

		info.SecretStore, _, _ = unstructured.NestedString(item.Object, "spec", "secretStoreRef", "name")
		info.TargetSecret, _, _ = unstructured.NestedString(item.Object, "spec", "target", "name")

		// Parse data keys and remote refs
		data, _, _ := unstructured.NestedSlice(item.Object, "spec", "data")
		for i, d := range data {
			entry, ok := d.(map[string]any)
			if !ok {
				continue
			}
			key, _, _ := unstructured.NestedString(entry, "secretKey")
			if key != "" {
				info.DataKeys = append(info.DataKeys, key)
			}
			// Capture first entry's remote ref for display
			if i == 0 {
				info.RemoteKey, _, _ = unstructured.NestedString(entry, "remoteRef", "key")
				info.RemoteProperty, _, _ = unstructured.NestedString(entry, "remoteRef", "property")
			}
		}

		// Parse status
		conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
		for _, c := range conditions {
			cond, ok := c.(map[string]any)
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(cond, "type")
			condStatus, _, _ := unstructured.NestedString(cond, "status")
			if condType == "Ready" {
				if condStatus == "True" {
					info.Status = "Synced"
				} else {
					info.Status = "Error"
				}
			}
		}
		if info.Status == "" {
			info.Status = "Pending"
		}

		// Parse last synced
		syncedTime, _, _ := unstructured.NestedString(item.Object, "status", "refreshTime")
		if syncedTime != "" {
			t, err := time.Parse(time.RFC3339, syncedTime)
			if err == nil {
				info.LastSynced = t.Format("Jan 02 15:04")
			} else {
				info.LastSynced = syncedTime
			}
		}

		results = append(results, info)
	}

	return results, nil
}

func (s *SecretsService) CreateExternalSecret(ctx context.Context, namespace string, opts CreateExternalSecretOptions) error {
	if opts.RefreshInterval == "" {
		opts.RefreshInterval = "1h"
	}

	dataItems := make([]any, len(opts.Data))
	for i, d := range opts.Data {
		entry := map[string]any{
			"secretKey": d.SecretKey,
			"remoteRef": map[string]any{
				"key": d.RemoteKey,
			},
		}
		if d.RemoteProperty != "" {
			entry["remoteRef"].(map[string]any)["property"] = d.RemoteProperty
		}
		dataItems[i] = entry
	}

	es := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "external-secrets.io/v1",
			"kind":       "ExternalSecret",
			"metadata": map[string]any{
				"name":      opts.Name,
				"namespace": namespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "clawmachine",
				},
			},
			"spec": map[string]any{
				"refreshInterval": opts.RefreshInterval,
				"secretStoreRef": map[string]any{
					"name": opts.SecretStore,
					"kind": "SecretStore",
				},
				"target": map[string]any{
					"name": opts.TargetSecret,
				},
				"data": dataItems,
			},
		},
	}

	_, err := s.dynamic.Resource(externalSecretGVR).Namespace(namespace).Create(ctx, es, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("creating external secret: %w", err)
		}
		// Already exists — update in place (idempotent)
		existing, getErr := s.dynamic.Resource(externalSecretGVR).Namespace(namespace).Get(ctx, opts.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("getting existing external secret: %w", getErr)
		}
		es.SetResourceVersion(existing.GetResourceVersion())
		if _, updateErr := s.dynamic.Resource(externalSecretGVR).Namespace(namespace).Update(ctx, es, metav1.UpdateOptions{}); updateErr != nil {
			return fmt.Errorf("updating external secret: %w", updateErr)
		}
	}

	return nil
}

func (s *SecretsService) DeleteExternalSecret(ctx context.Context, namespace, name string) error {
	err := s.dynamic.Resource(externalSecretGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting external secret: %w", err)
	}
	return nil
}

// deployOnePasswordConnect deploys the 1Password Connect Server (API + Sync)
// into the cluster as a Deployment + Service. The credentials JSON is stored
// as a K8s Secret and mounted into the Connect containers.
func (s *SecretsService) deployOnePasswordConnect(ctx context.Context, namespace, credentialsJSON string) error {
	// Create the 1password-credentials secret
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-credentials",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "clawmachine",
				"app.kubernetes.io/component":  "onepassword-connect",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"1password-credentials.json": credentialsJSON,
		},
	}

	existing, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, credSecret.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking credentials secret: %w", err)
		}
		_, err = s.clientset.CoreV1().Secrets(namespace).Create(ctx, credSecret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating credentials secret: %w", err)
		}
	} else {
		existing.StringData = credSecret.StringData
		existing.Labels = credSecret.Labels
		_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating credentials secret: %w", err)
		}
	}

	// Create the Connect Deployment (API + Sync containers)
	replicas := int32(1)
	labels := map[string]string{
		"app.kubernetes.io/name":       "onepassword-connect",
		"app.kubernetes.io/managed-by": "clawmachine",
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-connect",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "connect-api",
							Image: "1password/connect-api:latest",
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "credentials", MountPath: "/home/opuser/.op/1password-credentials.json", SubPath: "1password-credentials.json", ReadOnly: true},
								{Name: "data", MountPath: "/home/opuser/.op/data"},
							},
						},
						{
							Name:  "connect-sync",
							Image: "1password/connect-sync:latest",
							Ports: []corev1.ContainerPort{
								{Name: "sync", ContainerPort: 8081, Protocol: corev1.ProtocolTCP},
							},
							Env: []corev1.EnvVar{
								{Name: "OP_HTTP_PORT", Value: "8081"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "credentials", MountPath: "/home/opuser/.op/1password-credentials.json", SubPath: "1password-credentials.json", ReadOnly: true},
								{Name: "data", MountPath: "/home/opuser/.op/data"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "credentials",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "onepassword-credentials",
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	existingDeploy, err := s.clientset.AppsV1().Deployments(namespace).Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking Connect deployment: %w", err)
		}
		_, err = s.clientset.AppsV1().Deployments(namespace).Create(ctx, deploy, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating Connect deployment: %w", err)
		}
	} else {
		deploy.ResourceVersion = existingDeploy.ResourceVersion
		_, err = s.clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating Connect deployment: %w", err)
		}
	}

	// Create the Connect Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-connect",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	existingSvc, err := s.clientset.CoreV1().Services(namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("checking Connect service: %w", err)
		}
		_, err = s.clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating Connect service: %w", err)
		}
	} else {
		svc.ResourceVersion = existingSvc.ResourceVersion
		svc.Spec.ClusterIP = existingSvc.Spec.ClusterIP // preserve
		_, err = s.clientset.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating Connect service: %w", err)
		}
	}

	return nil
}

func isNoMatchError(err error) bool {
	return strings.Contains(err.Error(), "no matches for")
}
