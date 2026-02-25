package main

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsManagedBotRelease(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		chartName string
		want      bool
	}{
		{name: "managed bot in default namespace", namespace: "claw-machine", chartName: "openclaw", want: true},
		{name: "managed bot case-insensitive", namespace: "claw-machine", chartName: "PICOCLAW", want: true},
		{name: "infra chart excluded", namespace: "claw-machine", chartName: "cilium", want: false},
		{name: "other namespace excluded", namespace: "default", chartName: "openclaw", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManagedBotRelease(tt.namespace, tt.chartName); got != tt.want {
				t.Fatalf("isManagedBotRelease(%q, %q) = %t, want %t", tt.namespace, tt.chartName, got, tt.want)
			}
		})
	}
}

func TestEvaluateESOStatus(t *testing.T) {
	t.Run("missing api group", func(t *testing.T) {
		got := evaluateESOStatus(esoFacts{apiGroupPresent: false})
		if got.Installed || got.Healthy {
			t.Fatalf("expected not installed and not healthy, got %+v", got)
		}
	})

	t.Run("api present and ready deployment", func(t *testing.T) {
		got := evaluateESOStatus(esoFacts{
			apiGroupPresent: true,
			deployments: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "external-secrets", Namespace: "external-secrets"},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
				},
			},
		})
		if !got.Installed || !got.Healthy {
			t.Fatalf("expected installed and healthy, got %+v", got)
		}
		if got.Namespace != "external-secrets" {
			t.Fatalf("namespace = %q, want external-secrets", got.Namespace)
		}
	})

	t.Run("api present but deployment not ready", func(t *testing.T) {
		got := evaluateESOStatus(esoFacts{
			apiGroupPresent: true,
			deployments: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "external-secrets", Namespace: "eso-system"},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 0},
				},
			},
		})
		if !got.Installed || got.Healthy {
			t.Fatalf("expected installed but unhealthy, got %+v", got)
		}
	})
}

func TestEvaluateCiliumStatus(t *testing.T) {
	got := evaluateCiliumStatus(ciliumFacts{
		apiGroupPresent: true,
		daemonSets: []appsv1.DaemonSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cilium", Namespace: "kube-system"},
				Status:     appsv1.DaemonSetStatus{NumberReady: 1},
			},
		},
	})
	if !got.Installed || !got.Healthy {
		t.Fatalf("expected installed and healthy, got %+v", got)
	}
}

func TestEvaluateOnePasswordStatus(t *testing.T) {
	t.Run("optional not installed", func(t *testing.T) {
		got := evaluateOnePasswordStatus(onePasswordFacts{})
		if !got.Optional {
			t.Fatal("expected component to remain optional")
		}
		if got.Installed || got.Healthy {
			t.Fatalf("expected not installed and not healthy, got %+v", got)
		}
	})

	t.Run("installed and healthy", func(t *testing.T) {
		got := evaluateOnePasswordStatus(onePasswordFacts{
			deployments: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "onepassword-connect", Namespace: "1password"},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "onepassword-connect", Namespace: "1password"},
				},
			},
		})
		if !got.Installed || !got.Healthy {
			t.Fatalf("expected installed and healthy, got %+v", got)
		}
	})
}
