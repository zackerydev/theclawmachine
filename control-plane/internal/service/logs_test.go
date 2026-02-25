package service

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSelectLogContainer_ExactReleaseName(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "sidecar"},
				{Name: "dorothy-picoclaw"},
			},
		},
	}

	got := selectLogContainer(pod, "dorothy-picoclaw")
	if got != "dorothy-picoclaw" {
		t.Fatalf("container = %q, want %q", got, "dorothy-picoclaw")
	}
}

func TestSelectLogContainer_TrailingTokenMatch(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "helper"},
				{Name: "picoclaw"},
			},
		},
	}

	got := selectLogContainer(pod, "dorothy-picoclaw")
	if got != "picoclaw" {
		t.Fatalf("container = %q, want %q", got, "picoclaw")
	}
}

func TestSelectLogContainer_FallsBackToFirstContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "helper"},
				{Name: "openclaw"},
			},
		},
	}

	got := selectLogContainer(pod, "release-without-token-match")
	if got != "helper" {
		t.Fatalf("container = %q, want %q", got, "helper")
	}
}

func TestSelectLogContainer_SingleContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "openclaw"},
			},
		},
	}

	got := selectLogContainer(pod, "anything")
	if got != "openclaw" {
		t.Fatalf("container = %q, want %q", got, "openclaw")
	}
}
