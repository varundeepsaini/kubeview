package main

import appsv1 "k8s.io/api/apps/v1"

// Response shapes — JSON tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts. Any drift here breaks the dashboard.

type StatefulSet struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	ServiceName     string `json:"serviceName"`
	Strategy        string `json:"strategy"`
	Age             string `json:"age"`
	Replicas        int32  `json:"replicas"`
	DesiredReplicas int32  `json:"desiredReplicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
}

type DaemonSet struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Desired   int32  `json:"desired"`
	Current   int32  `json:"current"`
	Ready     int32  `json:"ready"`
	Updated   int32  `json:"updated"`
	Available int32  `json:"available"`
}

func transformStatefulSet(item appsv1.StatefulSet) StatefulSet {
	var desired int32
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}

	return StatefulSet{
		Name:            item.Name,
		Namespace:       item.Namespace,
		ServiceName:     item.Spec.ServiceName,
		Strategy:        string(item.Spec.UpdateStrategy.Type),
		Age:             getAge(item.CreationTimestamp),
		Replicas:        item.Status.Replicas,
		DesiredReplicas: desired,
		ReadyReplicas:   item.Status.ReadyReplicas,
		CurrentReplicas: item.Status.CurrentReplicas,
		UpdatedReplicas: item.Status.UpdatedReplicas,
	}
}

func transformDaemonSet(item appsv1.DaemonSet) DaemonSet {
	return DaemonSet{
		Name:      item.Name,
		Namespace: item.Namespace,
		Age:       getAge(item.CreationTimestamp),
		Desired:   item.Status.DesiredNumberScheduled,
		Current:   item.Status.CurrentNumberScheduled,
		Ready:     item.Status.NumberReady,
		Updated:   item.Status.UpdatedNumberScheduled,
		Available: item.Status.NumberAvailable,
	}
}
