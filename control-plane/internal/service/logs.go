package service

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// selectLogContainer chooses the best container for user-facing app logs.
// Priority:
// 1) exact release name match
// 2) match trailing token from release name (e.g. "dorothy-picoclaw" -> "picoclaw")
// 3) first regular container
func selectLogContainer(pod corev1.Pod, releaseName string) string {
	if len(pod.Spec.Containers) == 0 {
		return ""
	}

	for _, c := range pod.Spec.Containers {
		if c.Name == releaseName {
			return c.Name
		}
	}

	if idx := strings.LastIndex(releaseName, "-"); idx > -1 && idx+1 < len(releaseName) {
		trailing := releaseName[idx+1:]
		for _, c := range pod.Spec.Containers {
			if c.Name == trailing {
				return c.Name
			}
		}
	}

	return pod.Spec.Containers[0].Name
}
