package service

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// newTestKubernetesService creates a KubernetesService with fake clients for testing.
func newTestKubernetesService(objects ...runtime.Object) *KubernetesService {
	clientset := kubefake.NewClientset(objects...) //nolint:staticcheck
	dynClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	return &KubernetesService{
		clientset:      clientset,
		dynamic:        dynClient,
		currentContext: "test-context",
		kubeConfigPath: "/test/kubeconfig",
		inCluster:      false,
	}
}

func TestKubernetesService_Accessors(t *testing.T) {
	svc := newTestKubernetesService()

	if svc.Clientset() == nil {
		t.Fatal("expected non-nil clientset")
	}
	if svc.DynamicClient() == nil {
		t.Fatal("expected non-nil dynamic client")
	}
	if svc.GetCurrentContext() != "test-context" {
		t.Fatalf("expected 'test-context', got %q", svc.GetCurrentContext())
	}
	if svc.GetKubeConfigPath() != "/test/kubeconfig" {
		t.Fatalf("expected '/test/kubeconfig', got %q", svc.GetKubeConfigPath())
	}
	if svc.InCluster() {
		t.Fatal("expected InCluster=false")
	}
}

func TestKubernetesService_InCluster(t *testing.T) {
	svc := &KubernetesService{
		clientset:      kubefake.NewClientset(), //nolint:staticcheck
		dynamic:        dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		currentContext: "in-cluster",
		inCluster:      true,
	}

	if !svc.InCluster() {
		t.Fatal("expected InCluster=true")
	}
}

func TestKubernetesService_HasCRD_InvalidName(t *testing.T) {
	svc := newTestKubernetesService()

	tests := []string{
		"",
		"ciliumnetworkpolicies", // no group separator
		"resource.",             // trailing dot yields empty group
	}

	for _, name := range tests {
		if svc.HasCRD(name) {
			t.Fatalf("HasCRD(%q) = true, want false", name)
		}
	}
}

func TestKubernetesService_GetPodLogs_NoPods(t *testing.T) {
	svc := newTestKubernetesService()

	_, err := svc.GetPodLogs(context.Background(), "default", "missing-release", 25)
	if err == nil {
		t.Fatal("expected GetPodLogs to fail when no pods exist")
	}
	if !strings.Contains(err.Error(), "no pods found for release") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKubernetesService_GetReleasePodHealthy(t *testing.T) {
	svc := newTestKubernetesService(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bot-not-ready",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-release",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bot-ready",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-release",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	healthy, err := svc.GetReleasePodHealthy(context.Background(), "default", "my-release")
	if err != nil {
		t.Fatalf("GetReleasePodHealthy error: %v", err)
	}
	if !healthy {
		t.Fatal("expected healthy=true")
	}
}

func TestKubernetesService_GetReleasePodHealthy_NoReadyPods(t *testing.T) {
	svc := newTestKubernetesService(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bot-pending",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-release",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	healthy, err := svc.GetReleasePodHealthy(context.Background(), "default", "my-release")
	if err != nil {
		t.Fatalf("GetReleasePodHealthy error: %v", err)
	}
	if healthy {
		t.Fatal("expected healthy=false")
	}
}

func TestKubernetesService_ReadSecretData(t *testing.T) {
	svc := newTestKubernetesService(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bot-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"TOKEN": []byte("abc123"),
		},
	})

	data, err := svc.ReadSecretData(context.Background(), "default", "bot-secret")
	if err != nil {
		t.Fatalf("ReadSecretData error: %v", err)
	}
	if string(data["TOKEN"]) != "abc123" {
		t.Fatalf("TOKEN = %q, want %q", string(data["TOKEN"]), "abc123")
	}
}

func TestKubernetesService_ReadSecretData_NotFound(t *testing.T) {
	svc := newTestKubernetesService()

	_, err := svc.ReadSecretData(context.Background(), "default", "missing")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !strings.Contains(err.Error(), `reading secret "missing"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKubernetesService_RestartBot(t *testing.T) {
	svc := newTestKubernetesService(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-1",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-release",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-2",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-release",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-release",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "other",
				},
			},
		},
	)

	if err := svc.RestartBot(context.Background(), "default", "my-release"); err != nil {
		t.Fatalf("RestartBot error: %v", err)
	}

	pods, err := svc.Clientset().CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listing pods: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected only non-target pods to remain, got %d pods", len(pods.Items))
	}
	if pods.Items[0].Name != "other-release" {
		t.Fatalf("remaining pod = %q, want %q", pods.Items[0].Name, "other-release")
	}
}

func TestSelectExecPod_PrefersRunningReady(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pending"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running-not-ready"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running-ready"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	}

	got := selectExecPod(pods)
	if got == nil {
		t.Fatal("expected pod, got nil")
	}
	if got.Name != "running-ready" {
		t.Fatalf("selected pod = %q, want %q", got.Name, "running-ready")
	}
}

func TestSelectExecPod_AvoidsTerminating(t *testing.T) {
	now := metav1.Now()
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "terminating", DeletionTimestamp: &now},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	got := selectExecPod(pods)
	if got == nil {
		t.Fatal("expected pod, got nil")
	}
	if got.Name != "running" {
		t.Fatalf("selected pod = %q, want %q", got.Name, "running")
	}
}

func TestSelectExecPod_Empty(t *testing.T) {
	if got := selectExecPod(nil); got != nil {
		t.Fatalf("expected nil pod, got %q", got.Name)
	}
}

func TestKubernetesService_GetBackupLastSuccess_FromCronJobStatus(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := newTestKubernetesService(
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-bot-openclaw-backup",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-bot",
				},
			},
			Status: batchv1.CronJobStatus{
				LastSuccessfulTime: &metav1.Time{Time: now},
			},
		},
	)

	got, found, err := svc.GetBackupLastSuccess(context.Background(), "default", "my-bot")
	if err != nil {
		t.Fatalf("GetBackupLastSuccess error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !got.Equal(now) {
		t.Fatalf("time = %s, want %s", got, now)
	}
}

func TestKubernetesService_GetBackupLastSuccess_FromSuccessfulJobFallback(t *testing.T) {
	older := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	latest := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	svc := newTestKubernetesService(
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-bot-openclaw-backup",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-bot",
				},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-job",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-bot",
				},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "CronJob", Name: "my-bot-openclaw-backup"},
				},
			},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &metav1.Time{Time: older},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-job",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-bot",
				},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "CronJob", Name: "my-bot-openclaw-backup"},
				},
			},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &metav1.Time{Time: latest},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-owner",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/instance": "my-bot",
				},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "CronJob", Name: "some-other-cron"},
				},
			},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &metav1.Time{Time: time.Now().UTC().Add(-10 * time.Minute)},
			},
		},
	)

	got, found, err := svc.GetBackupLastSuccess(context.Background(), "default", "my-bot")
	if err != nil {
		t.Fatalf("GetBackupLastSuccess error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !got.Equal(latest) {
		t.Fatalf("time = %s, want %s", got, latest)
	}
}

func TestKubernetesService_GetBackupLastSuccess_NoCronJob(t *testing.T) {
	svc := newTestKubernetesService()

	_, found, err := svc.GetBackupLastSuccess(context.Background(), "default", "missing")
	if err != nil {
		t.Fatalf("GetBackupLastSuccess error: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}
