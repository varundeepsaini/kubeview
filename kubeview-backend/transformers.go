package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Response shapes — JSON tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts. Any drift here breaks the dashboard.

// Shared status/placeholder strings used across transformers and the JSON
// responses they produce.
const (
	statusUnknown     = "Unknown"
	statusPending     = "Pending"
	statusNotReady    = "NotReady"
	statusReady       = "Ready"
	statusWaiting     = "Waiting"
	valueNA           = "N/A"
	valueNone         = "None"
	valueNoneBrackets = "<none>"
	typeClusterIP     = "ClusterIP"
	emptyString       = ""
)

// Duration breakpoints used when rendering a human-readable age string.
const (
	secondsPerMinute = 60
	minutesPerHour   = 60
	hoursPerDay      = 24
	minEventCount    = 1
	// zeroCount is used both as a clamp floor for negative durations and to
	// detect empty collections.
	zeroCount = 0
)

// podSummary aggregates per-container counters for a pod.
type podSummary struct {
	ready    int
	total    int
	restarts int32
}

type ClusterInfo struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Context     string `json:"context"`
	ClusterName string `json:"clusterName"`
	NodeCount   int    `json:"nodeCount"`
}

type Namespace struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
	CreatedAt string            `json:"createdAt"`
	Age       string            `json:"age"`
}

type Pod struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     string            `json:"status"`
	Ready      string            `json:"ready"`
	Node       string            `json:"node"`
	IP         string            `json:"ip"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  string            `json:"createdAt"`
	Age        string            `json:"age"`
	Containers []Container       `json:"containers"`
	Conditions []PodCondition    `json:"conditions"`
	Volumes    []Volume          `json:"volumes"`
	Restarts   int32             `json:"restarts"`
}

type Deployment struct {
	Name              string                `json:"name"`
	Namespace         string                `json:"namespace"`
	Strategy          string                `json:"strategy"`
	Labels            map[string]string     `json:"labels"`
	Selector          map[string]string     `json:"selector"`
	CreatedAt         string                `json:"createdAt"`
	Age               string                `json:"age"`
	Conditions        []DeploymentCondition `json:"conditions"`
	Images            []string              `json:"images"`
	Replicas          int32                 `json:"replicas"`
	ReadyReplicas     int32                 `json:"readyReplicas"`
	DesiredReplicas   int32                 `json:"desiredReplicas"`
	UpdatedReplicas   int32                 `json:"updatedReplicas"`
	AvailableReplicas int32                 `json:"availableReplicas"`
}

type Service struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"`
	ClusterIP  string            `json:"clusterIp"`
	ExternalIP string            `json:"externalIp"`
	Selector   map[string]string `json:"selector"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  string            `json:"createdAt"`
	Age        string            `json:"age"`
	Ports      []string          `json:"ports"`
}

// --- helpers ---

func formatTime(t metav1.Time) string {
	if t.IsZero() {
		return emptyString
	}

	return t.UTC().Format(time.RFC3339)
}

func getAge(t metav1.Time) string {
	if t.IsZero() {
		return statusUnknown
	}

	d := time.Since(t.Time)
	secs := int(d.Seconds())
	// A future creationTimestamp (clock skew) yields a negative duration; clamp
	// to 0 so the UI shows "0s" instead of a nonsensical "-5s".
	secs = max(secs, zeroCount)

	if secs < secondsPerMinute {
		return fmt.Sprintf("%ds", secs)
	}

	mins := secs / secondsPerMinute
	if mins < minutesPerHour {
		return fmt.Sprintf("%dm", mins)
	}

	hours := mins / minutesPerHour
	if hours < hoursPerDay {
		return fmt.Sprintf("%dh", hours)
	}

	return fmt.Sprintf("%dd", hours/hoursPerDay)
}

func emptyIfNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}

	return m
}

// --- transformers ---

func transformNamespace(namespace corev1.Namespace) Namespace {
	status := string(namespace.Status.Phase)
	if status == emptyString {
		status = statusUnknown
	}

	return Namespace{
		Name:      namespace.Name,
		Status:    status,
		Labels:    emptyIfNil(namespace.Labels),
		CreatedAt: formatTime(namespace.CreationTimestamp),
		Age:       getAge(namespace.CreationTimestamp),
	}
}

func transformPod(pod *corev1.Pod) Pod {
	summary := podContainerSummary(pod)

	node := pod.Spec.NodeName
	if node == emptyString {
		node = statusPending
	}

	podIP := pod.Status.PodIP
	if podIP == emptyString {
		podIP = valueNA
	}

	return Pod{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Status:     podStatus(pod),
		Ready:      fmt.Sprintf("%d/%d", summary.ready, summary.total),
		Restarts:   summary.restarts,
		Node:       node,
		IP:         podIP,
		Labels:     emptyIfNil(pod.Labels),
		CreatedAt:  formatTime(pod.CreationTimestamp),
		Age:        getAge(pod.CreationTimestamp),
		Containers: podContainers(pod),
		Conditions: podConditions(pod),
		Volumes:    podVolumes(pod),
	}
}

// podContainerSummary reports the number of ready containers, the total
// container count, and the aggregate restart count for a pod.
func podContainerSummary(pod *corev1.Pod) podSummary {
	total := len(pod.Status.ContainerStatuses)
	if total == zeroCount {
		total = len(pod.Spec.Containers)
	}

	summary := podSummary{ready: zeroCount, total: total, restarts: zeroCount}

	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			summary.ready++
		}

		summary.restarts += status.RestartCount
	}

	return summary
}

func podContainers(pod *corev1.Pod) []Container {
	containers := make([]Container, zeroCount, len(pod.Spec.Containers))

	for idx, spec := range pod.Spec.Containers {
		ports := make([]string, zeroCount, len(spec.Ports))
		for _, port := range spec.Ports {
			ports = append(
				ports,
				fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol),
			)
		}

		var (
			ready        bool
			restartCount int32
			state        = statusWaiting
		)

		if idx < len(pod.Status.ContainerStatuses) {
			cs := pod.Status.ContainerStatuses[idx]
			ready = cs.Ready
			restartCount = cs.RestartCount
			state = containerState(&cs)
		}

		containers = append(containers, Container{
			Name:         spec.Name,
			Image:        spec.Image,
			Ports:        ports,
			Ready:        ready,
			State:        state,
			RestartCount: restartCount,
		})
	}

	return containers
}

func podConditions(pod *corev1.Pod) []PodCondition {
	conditions := make([]PodCondition, zeroCount, len(pod.Status.Conditions))
	for _, cond := range pod.Status.Conditions {
		conditions = append(conditions, PodCondition{
			Type:           string(cond.Type),
			Status:         string(cond.Status),
			Reason:         cond.Reason,
			LastTransition: formatTime(cond.LastTransitionTime),
		})
	}

	return conditions
}

func podVolumes(pod *corev1.Pod) []Volume {
	volumes := make([]Volume, zeroCount, len(pod.Spec.Volumes))
	for _, vol := range pod.Spec.Volumes {
		volumes = append(volumes, Volume{
			Name: vol.Name,
			Type: volumeType(vol.VolumeSource),
		})
	}

	return volumes
}

func transformDeployment(dep appsv1.Deployment) Deployment {
	var desired int32
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}

	strategy := string(dep.Spec.Strategy.Type)
	if strategy == emptyString {
		strategy = "RollingUpdate"
	}

	selector := map[string]string{}
	if dep.Spec.Selector != nil {
		selector = emptyIfNil(dep.Spec.Selector.MatchLabels)
	}

	return Deployment{
		Name:              dep.Name,
		Namespace:         dep.Namespace,
		Replicas:          dep.Status.Replicas,
		ReadyReplicas:     dep.Status.ReadyReplicas,
		DesiredReplicas:   desired,
		UpdatedReplicas:   dep.Status.UpdatedReplicas,
		AvailableReplicas: dep.Status.AvailableReplicas,
		Strategy:          strategy,
		Labels:            emptyIfNil(dep.Labels),
		Selector:          selector,
		CreatedAt:         formatTime(dep.CreationTimestamp),
		Age:               getAge(dep.CreationTimestamp),
		Conditions:        deploymentConditions(dep),
		Images:            deploymentImages(dep),
	}
}

func deploymentConditions(dep appsv1.Deployment) []DeploymentCondition {
	conditions := make(
		[]DeploymentCondition,
		zeroCount,
		len(dep.Status.Conditions),
	)
	for _, cond := range dep.Status.Conditions {
		conditions = append(conditions, DeploymentCondition{
			Type:           string(cond.Type),
			Status:         string(cond.Status),
			Reason:         cond.Reason,
			Message:        cond.Message,
			LastTransition: formatTime(cond.LastTransitionTime),
		})
	}

	return conditions
}

func deploymentImages(dep appsv1.Deployment) []string {
	containers := dep.Spec.Template.Spec.Containers

	images := make([]string, zeroCount, len(containers))
	for _, container := range containers {
		images = append(images, container.Image)
	}

	return images
}

func transformService(svc corev1.Service) Service {
	ports := make([]string, zeroCount, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, formatServicePort(port))
	}

	externalIP := serviceExternalIP(svc)

	clusterIP := svc.Spec.ClusterIP
	if clusterIP == emptyString {
		clusterIP = valueNone
	}

	svcType := string(svc.Spec.Type)
	if svcType == emptyString {
		svcType = typeClusterIP
	}

	return Service{
		Name:       svc.Name,
		Namespace:  svc.Namespace,
		Type:       svcType,
		ClusterIP:  clusterIP,
		ExternalIP: externalIP,
		Ports:      ports,
		Selector:   emptyIfNil(svc.Spec.Selector),
		Labels:     emptyIfNil(svc.Labels),
		CreatedAt:  formatTime(svc.CreationTimestamp),
		Age:        getAge(svc.CreationTimestamp),
	}
}

// serviceExternalIP picks the load-balancer ingress IP when present, otherwise
// the first declared external IP, falling back to a placeholder.
func serviceExternalIP(svc corev1.Service) string {
	ingress := svc.Status.LoadBalancer.Ingress
	if len(ingress) > zeroCount && ingress[zeroCount].IP != emptyString {
		return ingress[zeroCount].IP
	}

	if len(svc.Spec.ExternalIPs) > zeroCount {
		return svc.Spec.ExternalIPs[zeroCount]
	}

	return valueNA
}

func transformNode(node corev1.Node) NodeInfo {
	nodeInfo := node.Status.NodeInfo
	capacity := node.Status.Capacity

	return NodeInfo{
		Name:             node.Name,
		Status:           nodeStatus(node),
		Roles:            nodeRoles(node),
		Version:          nodeInfo.KubeletVersion,
		OS:               nodeInfo.OSImage,
		Arch:             nodeInfo.Architecture,
		ContainerRuntime: nodeInfo.ContainerRuntimeVersion,
		CPU:              capacity.Cpu().String(),
		Memory:           capacity.Memory().String(),
		Pods:             capacity.Pods().String(),
		Labels:           emptyIfNil(node.Labels),
		Conditions:       nodeConditions(node),
		CreatedAt:        formatTime(node.CreationTimestamp),
		Age:              getAge(node.CreationTimestamp),
		Addresses:        nodeAddresses(node),
	}
}

func nodeStatus(node corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		isReady := cond.Type == corev1.NodeReady &&
			cond.Status == corev1.ConditionTrue
		if isReady {
			return statusReady
		}
	}

	return statusNotReady
}

func nodeRoles(node corev1.Node) []string {
	const prefix = "node-role.kubernetes.io/"

	roles := []string{}

	for label := range node.Labels {
		if role, ok := strings.CutPrefix(label, prefix); ok {
			roles = append(roles, role)
		}
	}

	if len(roles) == zeroCount {
		roles = []string{valueNoneBrackets}
	}

	return roles
}

func nodeConditions(node corev1.Node) []NodeCondition {
	conditions := make([]NodeCondition, zeroCount, len(node.Status.Conditions))
	for _, cond := range node.Status.Conditions {
		conditions = append(conditions, NodeCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	return conditions
}

func nodeAddresses(node corev1.Node) []NodeAddress {
	addresses := make([]NodeAddress, zeroCount, len(node.Status.Addresses))
	for _, addr := range node.Status.Addresses {
		addresses = append(addresses, NodeAddress{
			Type:    string(addr.Type),
			Address: addr.Address,
		})
	}

	return addresses
}

func transformEvent(event corev1.Event) KubeEvent {
	involved := event.InvolvedObject
	object := fmt.Sprintf("%s/%s", involved.Kind, involved.Name)

	return KubeEvent{
		Type:      event.Type,
		Reason:    event.Reason,
		Message:   event.Message,
		Object:    object,
		Namespace: event.Namespace,
		FirstSeen: formatTime(event.FirstTimestamp),
		LastSeen:  formatTime(event.LastTimestamp),
		Count:     maxInt32(event.Count, minEventCount),
		Source:    event.Source.Component,
	}
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}

	return b
}

// podStatus surfaces a waiting or terminated container reason as the pod's
// effective status when one is present, otherwise falls back to the pod phase.
// This matches the failure modes (CrashLoopBackOff, ImagePullBackOff, ...)
// that users expect to see in a dashboard, which the phase field alone hides.
func podStatus(pod *corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	reason := containerStatusReason(pod.Status.ContainerStatuses)
	if reason != emptyString {
		return reason
	}

	if pod.Status.Phase != emptyString {
		return string(pod.Status.Phase)
	}

	return statusUnknown
}

// containerStatusReason returns the first waiting/terminated reason found among
// the container statuses, or an empty string when none carry a reason.
func containerStatusReason(statuses []corev1.ContainerStatus) string {
	for _, status := range statuses {
		if status.State.Waiting != nil &&
			status.State.Waiting.Reason != emptyString {
			return status.State.Waiting.Reason
		}

		if status.State.Terminated != nil &&
			status.State.Terminated.Reason != emptyString {
			return status.State.Terminated.Reason
		}
	}

	return emptyString
}

func containerState(status *corev1.ContainerStatus) string {
	if status == nil {
		return statusWaiting
	}

	switch {
	case status.State.Running != nil:
		return "Running"
	case status.State.Waiting != nil:
		if status.State.Waiting.Reason != emptyString {
			return status.State.Waiting.Reason
		}

		return statusWaiting
	case status.State.Terminated != nil:
		if status.State.Terminated.Reason != emptyString {
			return status.State.Terminated.Reason
		}

		return "Terminated"
	}

	return statusUnknown
}

// volumeType mirrors the JS trick of picking the first non-"name" key of a
// Volume object. corev1.VolumeSource is a struct of pointers, exactly one of
// which is non-nil for a given volume; we report the JSON tag of that field.
// Using the JSON tag (rather than lowercasing the Go field name) keeps the
// output aligned with the JS backend for acronym-prefixed types like NFS,
// iSCSI, CSI, RBD, FC.
func volumeType(vs corev1.VolumeSource) string {
	value := reflect.ValueOf(vs)
	typ := value.Type()

	for idx := range value.NumField() {
		field := value.Field(idx)
		if field.Kind() != reflect.Pointer || field.IsNil() {
			continue
		}

		tag := typ.Field(idx).Tag.Get("json")
		if comma := strings.Index(tag, ","); comma >= zeroCount {
			tag = tag[:comma]
		}

		return tag
	}

	return "unknown"
}

func formatServicePort(port corev1.ServicePort) string {
	hasTarget := port.TargetPort.IntValue() != zeroCount ||
		port.TargetPort.StrVal != emptyString
	if hasTarget {
		return fmt.Sprintf(
			"%d:%s/%s",
			port.Port,
			port.TargetPort.String(),
			port.Protocol,
		)
	}

	return fmt.Sprintf("%d/%s", port.Port, port.Protocol)
}
