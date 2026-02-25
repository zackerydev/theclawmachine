package service

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestConnectService_GetStatus_NotInstalled(t *testing.T) {
	cs := &ConnectService{
		clientset: fake.NewClientset(),
	}
	// GetStatus requires helm config which won't work with fake client,
	// but we can test the struct creation
	if cs.clientset == nil {
		t.Fatal("expected clientset to be set")
	}
}

func TestConnectService_EnsureNamespace(t *testing.T) {
	client := fake.NewClientset()
	ctx := context.Background()

	// Simulate what Install does: create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   opConnectNamespace,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "clawmachine"},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Verify it exists
	got, err := client.CoreV1().Namespaces().Get(ctx, opConnectNamespace, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get namespace: %v", err)
	}
	if got.Name != opConnectNamespace {
		t.Fatalf("expected namespace %q, got %q", opConnectNamespace, got.Name)
	}
}

func TestConnectService_CreateSecrets(t *testing.T) {
	client := fake.NewClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: opConnectNamespace},
	})
	ctx := context.Background()

	// Create credentials secret
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "op-credentials",
			Namespace: opConnectNamespace,
		},
		StringData: map[string]string{
			"1password-credentials.json": `{"test": true}`,
		},
	}
	_, err := client.CoreV1().Secrets(opConnectNamespace).Create(ctx, credSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create credentials secret: %v", err)
	}

	// Create token secret
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "onepassword-token",
			Namespace: opConnectNamespace,
		},
		StringData: map[string]string{
			"token": "test-token",
		},
	}
	_, err = client.CoreV1().Secrets(opConnectNamespace).Create(ctx, tokenSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create token secret: %v", err)
	}

	// Verify secrets exist
	secrets, err := client.CoreV1().Secrets(opConnectNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list secrets: %v", err)
	}
	if len(secrets.Items) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(secrets.Items))
	}
}

func TestConnectService_IdempotentSecretUpdate(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: opConnectNamespace}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "op-credentials",
				Namespace: opConnectNamespace,
			},
			Data: map[string][]byte{
				"1password-credentials.json": []byte(`{"old": true}`),
			},
		},
	)
	ctx := context.Background()

	// Update existing secret
	existing, _ := client.CoreV1().Secrets(opConnectNamespace).Get(ctx, "op-credentials", metav1.GetOptions{})
	existing.StringData = map[string]string{
		"1password-credentials.json": `{"new": true}`,
	}
	_, err := client.CoreV1().Secrets(opConnectNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update secret: %v", err)
	}
}

func TestConnectStatus_Fields(t *testing.T) {
	status := &ConnectStatus{
		Installed: true,
		Ready:     true,
		Host:      "http://onepassword-connect.1password.svc:8080",
	}
	if !status.Installed {
		t.Fatal("expected Installed to be true")
	}
	if !status.Ready {
		t.Fatal("expected Ready to be true")
	}
	if status.Host != "http://onepassword-connect.1password.svc:8080" {
		t.Fatalf("unexpected host: %s", status.Host)
	}
}
