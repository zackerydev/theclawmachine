//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var secretStoreGVR = schema.GroupVersionResource{
	Group:    "external-secrets.io",
	Version:  "v1",
	Resource: "secretstores",
}

// waitForSecretStoreReady polls the onepassword-store SecretStore until its
// Ready condition is True. This must be called before creating ExternalSecrets
// to ensure the Connect server has authenticated with 1Password and ESO can
// sync immediately on first attempt.
func waitForSecretStoreReady(t *testing.T, namespace string, timeout time.Duration) {
	t.Helper()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		t.Logf("Warning: could not get k8s config for SecretStore wait: %v", err)
		return
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Logf("Warning: could not create dynamic client for SecretStore wait: %v", err)
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ss, err := dynClient.Resource(secretStoreGVR).Namespace(namespace).Get(
			context.Background(), "onepassword-store", metav1.GetOptions{},
		)
		if err == nil {
			conditions, _, _ := unstructuredConditions(ss.Object)
			for _, c := range conditions {
				if c["type"] == "Ready" && c["status"] == "True" {
					t.Logf("SecretStore onepassword-store is Ready (reason: %s)", c["reason"])
					return
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
	t.Logf("Warning: SecretStore onepassword-store not Ready after %s, continuing anyway", timeout)
}

// unstructuredConditions extracts status.conditions from an unstructured object.
func unstructuredConditions(obj map[string]any) ([]map[string]any, bool, error) {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	rawConds, ok := status["conditions"].([]any)
	if !ok {
		return nil, false, nil
	}
	var out []map[string]any
	for _, c := range rawConds {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, true, nil
}
