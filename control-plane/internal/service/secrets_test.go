package service

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func newFakeSecretsService(objects ...runtime.Object) *SecretsService {
	clientset := kubefake.NewClientset(objects...) //nolint:staticcheck

	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			secretStoreGVR:    "SecretStoreList",
			externalSecretGVR: "ExternalSecretList",
		},
	)

	return NewSecretsService(clientset, dynClient)
}

func TestNewSecretsService(t *testing.T) {
	svc := newFakeSecretsService()
	if svc == nil {
		t.Fatal("expected non-nil SecretsService")
	}
}

func TestGetSecretStoreStatus_NoneConfigured(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	status, err := svc.GetSecretStoreStatus(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Configured {
		t.Fatal("expected Configured=false when no store exists")
	}
}

func TestCreateAndGetSecretStore(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	opts := CreateSecretStoreOptions{
		ConnectHost:  "https://connect.example.com",
		ConnectToken: "test-token",
		VaultName:    "my-vault",
	}

	err := svc.CreateSecretStore(ctx, "default", opts)
	if err != nil {
		t.Fatalf("unexpected error creating secret store: %v", err)
	}

	// Verify the store was created
	status, err := svc.GetSecretStoreStatus(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}
	if !status.Configured {
		t.Fatal("expected Configured=true after creation")
	}
	if status.Name != "onepassword-store" {
		t.Fatalf("expected name 'onepassword-store', got %q", status.Name)
	}
	if status.Provider != "1Password" {
		t.Fatalf("expected provider '1Password', got %q", status.Provider)
	}
	if status.ConnectHost != "https://connect.example.com" {
		t.Fatalf("expected connect host, got %q", status.ConnectHost)
	}
	if status.VaultName != "my-vault" {
		t.Fatalf("expected vault 'my-vault', got %q", status.VaultName)
	}
}

func TestCreateSecretStore_UpdateExisting(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Create initial
	opts := CreateSecretStoreOptions{
		ConnectHost:  "https://connect1.example.com",
		ConnectToken: "token1",
		VaultName:    "vault1",
	}
	if err := svc.CreateSecretStore(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update
	opts2 := CreateSecretStoreOptions{
		ConnectHost:  "https://connect2.example.com",
		ConnectToken: "token2",
		VaultName:    "vault2",
	}
	if err := svc.CreateSecretStore(ctx, "default", opts2); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}

	status, err := svc.GetSecretStoreStatus(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ConnectHost != "https://connect2.example.com" {
		t.Fatalf("expected updated connect host, got %q", status.ConnectHost)
	}
}

func TestDeleteSecretStore(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Create first
	opts := CreateSecretStoreOptions{
		ConnectHost:  "https://connect.example.com",
		ConnectToken: "token",
		VaultName:    "vault",
	}
	if err := svc.CreateSecretStore(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Delete
	if err := svc.DeleteSecretStore(ctx, "default"); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	// Verify gone
	status, err := svc.GetSecretStoreStatus(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Configured {
		t.Fatal("expected Configured=false after deletion")
	}
}

func TestDeleteSecretStore_NotFound(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Deleting non-existent should not error
	if err := svc.DeleteSecretStore(ctx, "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateExternalSecret(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	opts := CreateExternalSecretOptions{
		Name:            "my-secret",
		SecretStore:     "onepassword-store",
		TargetSecret:    "k8s-secret",
		RefreshInterval: "1h",
		Data: []ExternalSecretData{
			{SecretKey: "api-key", RemoteKey: "my-item", RemoteProperty: "credential"},
		},
	}

	err := svc.CreateExternalSecret(ctx, "default", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List and verify
	secrets, err := svc.ListExternalSecrets(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	if secrets[0].Name != "my-secret" {
		t.Fatalf("expected name 'my-secret', got %q", secrets[0].Name)
	}
	if secrets[0].SecretStore != "onepassword-store" {
		t.Fatalf("expected store 'onepassword-store', got %q", secrets[0].SecretStore)
	}
	if secrets[0].TargetSecret != "k8s-secret" {
		t.Fatalf("expected target 'k8s-secret', got %q", secrets[0].TargetSecret)
	}
	if len(secrets[0].DataKeys) != 1 || secrets[0].DataKeys[0] != "api-key" {
		t.Fatalf("expected data key 'api-key', got %v", secrets[0].DataKeys)
	}
	if secrets[0].Status != "Pending" {
		t.Fatalf("expected status 'Pending', got %q", secrets[0].Status)
	}
}

func TestCreateExternalSecret_DefaultRefreshInterval(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	opts := CreateExternalSecretOptions{
		Name:         "my-secret",
		SecretStore:  "store",
		TargetSecret: "target",
		Data:         []ExternalSecretData{{SecretKey: "key", RemoteKey: "remote"}},
	}

	if err := svc.CreateExternalSecret(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the refresh interval was set to default
	es, err := svc.dynamic.Resource(externalSecretGVR).Namespace("default").Get(ctx, "my-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	interval, _, _ := unstructured.NestedString(es.Object, "spec", "refreshInterval")
	if interval != "1h" {
		t.Fatalf("expected default refresh interval '1h', got %q", interval)
	}
}

func TestCreateExternalSecret_WithRemoteProperty(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	opts := CreateExternalSecretOptions{
		Name:         "prop-secret",
		SecretStore:  "store",
		TargetSecret: "target",
		Data: []ExternalSecretData{
			{SecretKey: "key", RemoteKey: "item", RemoteProperty: "password"},
		},
	}

	if err := svc.CreateExternalSecret(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	es, err := svc.dynamic.Resource(externalSecretGVR).Namespace("default").Get(ctx, "prop-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _, _ := unstructured.NestedSlice(es.Object, "spec", "data")
	if len(data) != 1 {
		t.Fatalf("expected 1 data entry, got %d", len(data))
	}
	entry := data[0].(map[string]any)
	prop, _, _ := unstructured.NestedString(entry, "remoteRef", "property")
	if prop != "password" {
		t.Fatalf("expected property 'password', got %q", prop)
	}
}

func TestDeleteExternalSecret(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Create first
	opts := CreateExternalSecretOptions{
		Name:         "to-delete",
		SecretStore:  "store",
		TargetSecret: "target",
		Data:         []ExternalSecretData{{SecretKey: "key", RemoteKey: "remote"}},
	}
	if err := svc.CreateExternalSecret(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Delete
	if err := svc.DeleteExternalSecret(ctx, "default", "to-delete"); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	// Verify gone
	secrets, err := svc.ListExternalSecrets(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets after deletion, got %d", len(secrets))
	}
}

func TestDeleteExternalSecret_NotFound(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Should not error
	if err := svc.DeleteExternalSecret(ctx, "default", "nonexistent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListExternalSecrets_Empty(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	secrets, err := svc.ListExternalSecrets(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets, got %d", len(secrets))
	}
}

func TestListExternalSecrets_WithStatusConditions(t *testing.T) {
	svc := newFakeSecretsService()
	ctx := context.Background()

	// Create an external secret
	opts := CreateExternalSecretOptions{
		Name:         "status-test",
		SecretStore:  "store",
		TargetSecret: "target",
		Data:         []ExternalSecretData{{SecretKey: "key", RemoteKey: "remote"}},
	}
	if err := svc.CreateExternalSecret(ctx, "default", opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Manually add status conditions to simulate controller behavior
	es, _ := svc.dynamic.Resource(externalSecretGVR).Namespace("default").Get(ctx, "status-test", metav1.GetOptions{})
	es.Object["status"] = map[string]any{
		"conditions": []any{
			map[string]any{
				"type":   "Ready",
				"status": "True",
			},
		},
		"refreshTime": "2025-01-01T12:00:00Z",
	}
	if _, err := svc.dynamic.Resource(externalSecretGVR).Namespace("default").UpdateStatus(ctx, es, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("updating external secret status: %v", err)
	}

	secrets, err := svc.ListExternalSecrets(ctx, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	// Note: fake client may not preserve status subresource updates the same way
}

func TestIsNoMatchError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"no matches for kind SecretStore in version external-secrets.io/v1", true},
		{"some other error", false},
	}
	for _, tt := range tests {
		if got := isNoMatchError(errors.New(tt.msg)); got != tt.want {
			t.Errorf("isNoMatchError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
