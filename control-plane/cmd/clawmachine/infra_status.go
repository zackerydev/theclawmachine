package main

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

const (
	defaultBotNamespace = "claw-machine"
)

var managedBotCharts = map[string]struct{}{
	"openclaw": {},
	"picoclaw": {},
	"ironclaw": {},
	"busybox":  {},
}

type infraComponentStatus struct {
	Name      string
	Namespace string
	Installed bool
	Healthy   bool
	Detail    string
	Optional  bool
}

type esoFacts struct {
	apiGroupPresent bool
	deployments     []appsv1.Deployment
}

type ciliumFacts struct {
	apiGroupPresent bool
	daemonSets      []appsv1.DaemonSet
}

type onePasswordFacts struct {
	deployments []appsv1.Deployment
	services    []corev1.Service
}

func collectInfraStatuses(kubeContext string) ([]infraComponentStatus, error) {
	k8s, err := service.NewKubernetesService(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	ctx := context.Background()
	clientset := k8s.Clientset()

	esoDeployments, err := findDeployments(ctx, clientset, "app.kubernetes.io/name=external-secrets", "external-secrets")
	if err != nil {
		return nil, fmt.Errorf("checking ESO deployments: %w", err)
	}
	ciliumDaemonSets, err := findCiliumDaemonSets(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("checking Cilium daemonsets: %w", err)
	}
	opDeployments, err := findDeployments(ctx, clientset, "app=onepassword-connect", "onepassword-connect")
	if err != nil {
		return nil, fmt.Errorf("checking 1Password Connect deployments: %w", err)
	}
	opServices, err := findServices(ctx, clientset, "app=onepassword-connect", "onepassword-connect")
	if err != nil {
		return nil, fmt.Errorf("checking 1Password Connect services: %w", err)
	}

	eso := evaluateESOStatus(esoFacts{
		apiGroupPresent: k8s.HasCRD("externalsecrets.external-secrets.io"),
		deployments:     esoDeployments,
	})
	cilium := evaluateCiliumStatus(ciliumFacts{
		apiGroupPresent: k8s.HasCRD("ciliumnetworkpolicies.cilium.io"),
		daemonSets:      ciliumDaemonSets,
	})
	opConnect := evaluateOnePasswordStatus(onePasswordFacts{
		deployments: opDeployments,
		services:    opServices,
	})

	return []infraComponentStatus{eso, cilium, opConnect}, nil
}

func evaluateESOStatus(f esoFacts) infraComponentStatus {
	status := infraComponentStatus{Name: "External Secrets Operator"}

	if !f.apiGroupPresent {
		status.Detail = "Fix: clawmachine install --external-secrets"
		return status
	}

	status.Installed = true
	if len(f.deployments) == 0 {
		status.Detail = "API group found, but ESO controller deployment is not running"
		return status
	}

	best := pickMostAvailableDeployment(f.deployments)
	status.Namespace = best.Namespace
	if deploymentReady(best) {
		status.Healthy = true
		status.Detail = "controller deployment is available"
		return status
	}

	status.Detail = "controller deployment exists but is not ready"
	return status
}

func evaluateCiliumStatus(f ciliumFacts) infraComponentStatus {
	status := infraComponentStatus{Name: "Cilium CNI"}

	if !f.apiGroupPresent && len(f.daemonSets) == 0 {
		status.Detail = "Fix: clawmachine install --cilium"
		return status
	}

	status.Installed = true
	if len(f.daemonSets) == 0 {
		status.Detail = "Cilium API group found, but cilium daemonset is missing"
		return status
	}

	best := pickMostReadyDaemonSet(f.daemonSets)
	status.Namespace = best.Namespace
	if daemonSetReady(best) {
		status.Healthy = true
		status.Detail = "cilium daemonset is ready"
		return status
	}

	status.Detail = "cilium daemonset exists but is not ready"
	return status
}

func evaluateOnePasswordStatus(f onePasswordFacts) infraComponentStatus {
	status := infraComponentStatus{
		Name:     "1Password Connect",
		Optional: true,
	}

	if len(f.deployments) == 0 && len(f.services) == 0 {
		status.Detail = "Optional: configure in dashboard Settings"
		return status
	}

	status.Installed = true
	bestDeployment := pickMostAvailableDeployment(f.deployments)
	if bestDeployment.Namespace != "" {
		status.Namespace = bestDeployment.Namespace
	} else if len(f.services) > 0 {
		status.Namespace = f.services[0].Namespace
	}

	hasService := len(f.services) > 0
	if deploymentReady(bestDeployment) && hasService {
		status.Healthy = true
		status.Detail = "connect service and deployment are ready"
		return status
	}

	if !hasService {
		status.Detail = "connect deployment found, but service is missing"
		return status
	}

	status.Detail = "connect service found, but deployment is not ready"
	return status
}

func findDeployments(ctx context.Context, clientset kubernetes.Interface, selector, nameContains string) ([]appsv1.Deployment, error) {
	deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(deployments.Items) > 0 {
		return deployments.Items, nil
	}

	all, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var filtered []appsv1.Deployment
	for _, d := range all.Items {
		if strings.Contains(strings.ToLower(d.Name), strings.ToLower(nameContains)) {
			filtered = append(filtered, d)
		}
	}
	return filtered, nil
}

func findServices(ctx context.Context, clientset kubernetes.Interface, selector, nameContains string) ([]corev1.Service, error) {
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(services.Items) > 0 {
		return services.Items, nil
	}

	all, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var filtered []corev1.Service
	for _, s := range all.Items {
		if strings.Contains(strings.ToLower(s.Name), strings.ToLower(nameContains)) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func findCiliumDaemonSets(ctx context.Context, clientset kubernetes.Interface) ([]appsv1.DaemonSet, error) {
	daemonSets, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{LabelSelector: "k8s-app=cilium"})
	if err != nil {
		return nil, err
	}
	if len(daemonSets.Items) > 0 {
		return daemonSets.Items, nil
	}

	all, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var filtered []appsv1.DaemonSet
	for _, ds := range all.Items {
		if strings.Contains(strings.ToLower(ds.Name), "cilium") {
			filtered = append(filtered, ds)
		}
	}
	return filtered, nil
}

func deploymentReady(d appsv1.Deployment) bool {
	return d.Name != "" && d.Status.AvailableReplicas > 0
}

func daemonSetReady(ds appsv1.DaemonSet) bool {
	return ds.Name != "" && ds.Status.NumberReady > 0
}

func pickMostAvailableDeployment(deployments []appsv1.Deployment) appsv1.Deployment {
	if len(deployments) == 0 {
		return appsv1.Deployment{}
	}
	best := deployments[0]
	for _, d := range deployments[1:] {
		if d.Status.AvailableReplicas > best.Status.AvailableReplicas {
			best = d
		}
	}
	return best
}

func pickMostReadyDaemonSet(daemonSets []appsv1.DaemonSet) appsv1.DaemonSet {
	if len(daemonSets) == 0 {
		return appsv1.DaemonSet{}
	}
	best := daemonSets[0]
	for _, ds := range daemonSets[1:] {
		if ds.Status.NumberReady > best.Status.NumberReady {
			best = ds
		}
	}
	return best
}

func isManagedBotRelease(namespace, chartName string) bool {
	if namespace != defaultBotNamespace {
		return false
	}
	_, ok := managedBotCharts[strings.ToLower(chartName)]
	return ok
}
