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

type ClusterInfo struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	NodeCount   int    `json:"nodeCount"`
	Context     string `json:"context"`
	ClusterName string `json:"clusterName"`
}

type Namespace struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
	CreatedAt string            `json:"createdAt"`
	Age       string            `json:"age"`
}

type Container struct {
	Name         string   `json:"name"`
	Image        string   `json:"image"`
	Ports        []string `json:"ports"`
	Ready        bool     `json:"ready"`
	State        string   `json:"state"`
	RestartCount int32    `json:"restartCount"`
}

type PodCondition struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	LastTransition string `json:"lastTransition,omitempty"`
}

type Volume struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Pod struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     string            `json:"status"`
	Ready      string            `json:"ready"`
	Restarts   int32             `json:"restarts"`
	Node       string            `json:"node"`
	IP         string            `json:"ip"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  string            `json:"createdAt"`
	Age        string            `json:"age"`
	Containers []Container       `json:"containers"`
	Conditions []PodCondition    `json:"conditions"`
	Volumes    []Volume          `json:"volumes"`
}

type DeploymentCondition struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	LastTransition string `json:"lastTransition,omitempty"`
}

type Deployment struct {
	Name              string                `json:"name"`
	Namespace         string                `json:"namespace"`
	Replicas          int32                 `json:"replicas"`
	ReadyReplicas     int32                 `json:"readyReplicas"`
	DesiredReplicas   int32                 `json:"desiredReplicas"`
	UpdatedReplicas   int32                 `json:"updatedReplicas"`
	AvailableReplicas int32                 `json:"availableReplicas"`
	Strategy          string                `json:"strategy"`
	Labels            map[string]string     `json:"labels"`
	Selector          map[string]string     `json:"selector"`
	CreatedAt         string                `json:"createdAt"`
	Age               string                `json:"age"`
	Conditions        []DeploymentCondition `json:"conditions"`
	Images            []string              `json:"images"`
}

type Service struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"`
	ClusterIP  string            `json:"clusterIP"`
	ExternalIP string            `json:"externalIP"`
	Ports      []string          `json:"ports"`
	Selector   map[string]string `json:"selector"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  string            `json:"createdAt"`
	Age        string            `json:"age"`
}

type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type NodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type NodeInfo struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Roles            []string          `json:"roles"`
	Version          string            `json:"version"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	ContainerRuntime string            `json:"containerRuntime"`
	CPU              string            `json:"cpu"`
	Memory           string            `json:"memory"`
	Pods             string            `json:"pods"`
	Labels           map[string]string `json:"labels"`
	Conditions       []NodeCondition   `json:"conditions"`
	CreatedAt        string            `json:"createdAt"`
	Age              string            `json:"age"`
	Addresses        []NodeAddress     `json:"addresses"`
}

type KubeEvent struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Object    string `json:"object"`
	Namespace string `json:"namespace"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	Count     int32  `json:"count"`
	Source    string `json:"source"`
}

// --- helpers ---

func formatTime(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func getAge(t metav1.Time) string {
	if t.IsZero() {
		return "Unknown"
	}
	d := time.Since(t.Time)
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}

func emptyIfNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// --- transformers ---

func transformNamespace(ns corev1.Namespace) Namespace {
	status := string(ns.Status.Phase)
	if status == "" {
		status = "Unknown"
	}
	return Namespace{
		Name:      ns.Name,
		Status:    status,
		Labels:    emptyIfNil(ns.Labels),
		CreatedAt: formatTime(ns.CreationTimestamp),
		Age:       getAge(ns.CreationTimestamp),
	}
}

func transformPod(pod *corev1.Pod) Pod {
	readyCount := 0
	totalCount := len(pod.Status.ContainerStatuses)
	if totalCount == 0 {
		totalCount = len(pod.Spec.Containers)
	}
	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyCount++
		}
		restarts += cs.RestartCount
	}

	containers := make([]Container, 0, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		ports := make([]string, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
		}
		var (
			ready        bool
			restartCount int32
			state        = "Waiting"
		)
		if i < len(pod.Status.ContainerStatuses) {
			cs := pod.Status.ContainerStatuses[i]
			ready = cs.Ready
			restartCount = cs.RestartCount
			state = containerState(&cs)
		}
		containers = append(containers, Container{
			Name:         c.Name,
			Image:        c.Image,
			Ports:        ports,
			Ready:        ready,
			State:        state,
			RestartCount: restartCount,
		})
	}

	conditions := make([]PodCondition, 0, len(pod.Status.Conditions))
	for _, cond := range pod.Status.Conditions {
		conditions = append(conditions, PodCondition{
			Type:           string(cond.Type),
			Status:         string(cond.Status),
			Reason:         cond.Reason,
			LastTransition: formatTime(cond.LastTransitionTime),
		})
	}

	volumes := make([]Volume, 0, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		volumes = append(volumes, Volume{
			Name: v.Name,
			Type: volumeType(v.VolumeSource),
		})
	}

	node := pod.Spec.NodeName
	if node == "" {
		node = "Pending"
	}
	ip := pod.Status.PodIP
	if ip == "" {
		ip = "N/A"
	}

	return Pod{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Status:     podStatus(pod),
		Ready:      fmt.Sprintf("%d/%d", readyCount, totalCount),
		Restarts:   restarts,
		Node:       node,
		IP:         ip,
		Labels:     emptyIfNil(pod.Labels),
		CreatedAt:  formatTime(pod.CreationTimestamp),
		Age:        getAge(pod.CreationTimestamp),
		Containers: containers,
		Conditions: conditions,
		Volumes:    volumes,
	}
}

func transformDeployment(dep appsv1.Deployment) Deployment {
	conditions := make([]DeploymentCondition, 0, len(dep.Status.Conditions))
	for _, cond := range dep.Status.Conditions {
		conditions = append(conditions, DeploymentCondition{
			Type:           string(cond.Type),
			Status:         string(cond.Status),
			Reason:         cond.Reason,
			Message:        cond.Message,
			LastTransition: formatTime(cond.LastTransitionTime),
		})
	}

	images := make([]string, 0, len(dep.Spec.Template.Spec.Containers))
	for _, c := range dep.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}

	var desired int32
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	strategy := string(dep.Spec.Strategy.Type)
	if strategy == "" {
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
		Conditions:        conditions,
		Images:            images,
	}
}

func transformService(svc corev1.Service) Service {
	ports := make([]string, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, formatServicePort(p))
	}
	externalIP := "N/A"
	if len(svc.Status.LoadBalancer.Ingress) > 0 && svc.Status.LoadBalancer.Ingress[0].IP != "" {
		externalIP = svc.Status.LoadBalancer.Ingress[0].IP
	} else if len(svc.Spec.ExternalIPs) > 0 {
		externalIP = svc.Spec.ExternalIPs[0]
	}
	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" {
		clusterIP = "None"
	}
	svcType := string(svc.Spec.Type)
	if svcType == "" {
		svcType = "ClusterIP"
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

func transformNode(n corev1.Node) NodeInfo {
	status := "NotReady"
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			status = "Ready"
			break
		}
	}

	roles := []string{}
	for label := range n.Labels {
		const prefix = "node-role.kubernetes.io/"
		if strings.HasPrefix(label, prefix) {
			roles = append(roles, strings.TrimPrefix(label, prefix))
		}
	}
	if len(roles) == 0 {
		roles = []string{"<none>"}
	}

	conditions := make([]NodeCondition, 0, len(n.Status.Conditions))
	for _, c := range n.Status.Conditions {
		conditions = append(conditions, NodeCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	addresses := make([]NodeAddress, 0, len(n.Status.Addresses))
	for _, a := range n.Status.Addresses {
		addresses = append(addresses, NodeAddress{Type: string(a.Type), Address: a.Address})
	}

	return NodeInfo{
		Name:             n.Name,
		Status:           status,
		Roles:            roles,
		Version:          n.Status.NodeInfo.KubeletVersion,
		OS:               n.Status.NodeInfo.OSImage,
		Arch:             n.Status.NodeInfo.Architecture,
		ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
		CPU:              n.Status.Capacity.Cpu().String(),
		Memory:           n.Status.Capacity.Memory().String(),
		Pods:             n.Status.Capacity.Pods().String(),
		Labels:           emptyIfNil(n.Labels),
		Conditions:       conditions,
		CreatedAt:        formatTime(n.CreationTimestamp),
		Age:              getAge(n.CreationTimestamp),
		Addresses:        addresses,
	}
}

func transformEvent(e corev1.Event) KubeEvent {
	return KubeEvent{
		Type:      e.Type,
		Reason:    e.Reason,
		Message:   e.Message,
		Object:    fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
		Namespace: e.Namespace,
		FirstSeen: formatTime(e.FirstTimestamp),
		LastSeen:  formatTime(e.LastTimestamp),
		Count:     maxInt32(e.Count, 1),
		Source:    e.Source.Component,
	}
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

// podStatus mirrors getPodStatus() from kubeview-backend/lib/transformers.js —
// surface waiting/terminated container reasons first, otherwise fall back to
// the pod phase.
func podStatus(pod *corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}
	if pod.Status.Phase != "" {
		return string(pod.Status.Phase)
	}
	return "Unknown"
}

func containerState(cs *corev1.ContainerStatus) string {
	if cs == nil {
		return "Waiting"
	}
	switch {
	case cs.State.Running != nil:
		return "Running"
	case cs.State.Waiting != nil:
		if cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		return "Waiting"
	case cs.State.Terminated != nil:
		if cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
		return "Terminated"
	}
	return "Unknown"
}

// volumeType mirrors the JS trick of picking the first non-"name" key of a
// Volume object. corev1.VolumeSource is a struct of pointers, exactly one of
// which is non-nil for a given volume; we report the field name of that one.
func volumeType(vs corev1.VolumeSource) string {
	v := reflect.ValueOf(vs)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Ptr && !f.IsNil() {
			return lowerFirst(t.Field(i).Name)
		}
	}
	return "unknown"
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func formatServicePort(p corev1.ServicePort) string {
	if p.TargetPort.IntValue() != 0 || p.TargetPort.StrVal != "" {
		return fmt.Sprintf("%d:%s/%s", p.Port, p.TargetPort.String(), p.Protocol)
	}
	return fmt.Sprintf("%d/%s", p.Port, p.Protocol)
}
