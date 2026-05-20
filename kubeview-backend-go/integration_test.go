package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
)

// realisticCluster returns a fixture that mirrors a typical small K8s cluster:
// 2 namespaces, 2 nodes, a couple of deployments, services, and pods (one of
// which is multi-container with conditions, volumes, and partial container
// statuses to mimic a real running workload). This is the input for every
// integration test in this file.
func realisticCluster() []runtime.Object {
	created := metav1.NewTime(time.Now().Add(-3 * time.Hour))

	return []runtime.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{"env": "dev"}, CreationTimestamp: created},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-system", CreationTimestamp: created},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},

		// Node 1 — control plane, Ready
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
					"kubernetes.io/hostname":                "node-1",
				},
				CreationTimestamp: created,
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady"}},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.31.0", OSImage: "Ubuntu 24.04",
					Architecture: "arm64", ContainerRuntimeVersion: "containerd://2.0",
				},
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
					{Type: corev1.NodeHostName, Address: "node-1"},
				},
			},
		},
		// Node 2 — worker, Ready
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"kubernetes.io/hostname": "node-2",
				},
				CreationTimestamp: created,
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.31.0", OSImage: "Ubuntu 24.04",
					Architecture: "arm64", ContainerRuntimeVersion: "containerd://2.0",
				},
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.2"}},
			},
		},

		// Multi-container running pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-abc", Namespace: "default",
				Labels:            map[string]string{"app": "web"},
				CreationTimestamp: created,
			},
			Spec: corev1.PodSpec{
				NodeName: "node-2",
				Containers: []corev1.Container{
					{Name: "app", Image: "myapp:1.0", Ports: []corev1.ContainerPort{{ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}},
					{Name: "sidecar", Image: "envoy:1.30"},
				},
				InitContainers: []corev1.Container{{Name: "init-db", Image: "init:1"}},
				Volumes: []corev1.Volume{
					{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning, PodIP: "10.0.0.5",
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					{Name: "sidecar", Ready: true, RestartCount: 1, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: created},
				},
			},
		},

		// Crash-looping pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "default", CreationTimestamp: created},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "bad:1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", Ready: false, RestartCount: 7, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
				},
			},
		},

		// Deployment with 3 replicas, 2 ready
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: "default",
				Labels:            map[string]string{"app": "web"},
				CreationTimestamp: created,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "myapp:1.0"}, {Name: "sidecar", Image: "envoy:1.30"}}},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas: 3, ReadyReplicas: 2, UpdatedReplicas: 3, AvailableReplicas: 2,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable", LastTransitionTime: created},
				},
			},
		},

		// ClusterIP service
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Labels: map[string]string{"app": "web"}, CreationTimestamp: created},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.96.0.10",
				Selector: map[string]string{"app": "web"},
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}},
			},
		},

		// Events
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "evt-1", Namespace: "default"},
			Type:       "Normal", Reason: "Scheduled", Message: "Successfully assigned default/web-abc to node-2",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-abc"},
			FirstTimestamp: created, LastTimestamp: created, Count: 1,
			Source: corev1.EventSource{Component: "default-scheduler"},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "evt-2", Namespace: "default"},
			Type:       "Warning", Reason: "BackOff", Message: "Back-off restarting failed container",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "broken"},
			FirstTimestamp: created, LastTimestamp: created, Count: 4,
			Source: corev1.EventSource{Component: "kubelet"},
		},
	}
}

func int32Ptr(v int32) *int32 { return &v }

// newIntegrationServer wires the realistic fixture into the real router and
// returns an httptest.Server. All integration tests share this setup.
func newIntegrationServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := newTestServer(t,
		&version.Info{GitVersion: "v1.31.0", Platform: "linux/arm64"},
		realisticCluster()...,
	)
	return srv
}

func fetchJSON(t *testing.T, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, body
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, b)
	}
	return v
}

// --- end-to-end integration tests over the realistic fixture --------------

func TestIntegration_Cluster(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/cluster")
	if code != 200 {
		t.Fatalf("code = %d, body = %s", code, body)
	}
	info := decode[ClusterInfo](t, body)
	if info.Version != "v1.31.0" || info.Platform != "linux/arm64" {
		t.Fatalf("version/platform: %+v", info)
	}
	if info.NodeCount != 2 {
		t.Fatalf("node count = %d", info.NodeCount)
	}
}

func TestIntegration_Namespaces(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/namespaces")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]Namespace](t, body)
	names := namesOf(out, func(n Namespace) string { return n.Name })
	wantSubset(t, names, []string{"default", "kube-system"})
	for _, ns := range out {
		if ns.Status != "Active" {
			t.Fatalf("ns %q status = %q", ns.Name, ns.Status)
		}
		if ns.Labels == nil {
			t.Fatalf("ns %q labels nil", ns.Name)
		}
	}
}

func TestIntegration_Pods_AllNamespaces(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/pods")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]Pod](t, body)
	if len(out) != 2 {
		t.Fatalf("pod count = %d, want 2", len(out))
	}
	byName := indexBy(out, func(p Pod) string { return p.Name })
	web := byName["web-abc"]
	if web.Status != "Running" {
		t.Fatalf("web status = %q", web.Status)
	}
	if web.Ready != "2/2" {
		t.Fatalf("web ready = %q", web.Ready)
	}
	if web.Restarts != 1 {
		t.Fatalf("web restarts = %d", web.Restarts)
	}
	if web.Node != "node-2" {
		t.Fatalf("web node = %q", web.Node)
	}
	if web.IP != "10.0.0.5" {
		t.Fatalf("web ip = %q", web.IP)
	}
	if len(web.Containers) != 2 {
		t.Fatalf("web containers = %d, want 2 (init not counted)", len(web.Containers))
	}
	cnames := namesOf(web.Containers, func(c Container) string { return c.Name })
	wantSubset(t, cnames, []string{"app", "sidecar"})
	// Init container "init-db" should NOT be in the list
	for _, c := range web.Containers {
		if c.Name == "init-db" {
			t.Fatalf("init container leaked into containers list")
		}
	}
	// Volumes — both with correct types
	vtypes := namesOf(web.Volumes, func(v Volume) string { return v.Type })
	sort.Strings(vtypes)
	if vtypes[0] != "configMap" || vtypes[1] != "emptyDir" {
		t.Fatalf("volume types = %v", vtypes)
	}

	broken := byName["broken"]
	if broken.Status != "CrashLoopBackOff" {
		t.Fatalf("broken status = %q", broken.Status)
	}
	if broken.Restarts != 7 {
		t.Fatalf("broken restarts = %d", broken.Restarts)
	}
}

func TestIntegration_Pods_FilteredByNamespace(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/pods?namespace=kube-system")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]Pod](t, body)
	if len(out) != 0 {
		t.Fatalf("expected 0 pods in kube-system, got %d", len(out))
	}
}

func TestIntegration_PodDetail(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/pods/default/web-abc")
	if code != 200 {
		t.Fatalf("code = %d, body = %s", code, body)
	}
	p := decode[Pod](t, body)
	if p.Name != "web-abc" {
		t.Fatalf("name = %q", p.Name)
	}
	if len(p.Conditions) != 1 || p.Conditions[0].Type != "Ready" {
		t.Fatalf("conditions = %+v", p.Conditions)
	}
}

func TestIntegration_Deployments(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/deployments")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]Deployment](t, body)
	if len(out) != 1 {
		t.Fatalf("deployment count = %d", len(out))
	}
	d := out[0]
	if d.DesiredReplicas != 3 || d.Replicas != 3 || d.ReadyReplicas != 2 || d.AvailableReplicas != 2 {
		t.Fatalf("replicas wrong: %+v", d)
	}
	if d.Strategy != "RollingUpdate" {
		t.Fatalf("strategy = %q", d.Strategy)
	}
	sort.Strings(d.Images)
	if len(d.Images) != 2 || d.Images[0] != "envoy:1.30" || d.Images[1] != "myapp:1.0" {
		t.Fatalf("images = %v", d.Images)
	}
}

func TestIntegration_Services(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/services")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]Service](t, body)
	if len(out) != 1 {
		t.Fatalf("svc count = %d", len(out))
	}
	s := out[0]
	if s.Type != "ClusterIP" || s.ClusterIP != "10.96.0.10" || s.ExternalIP != "N/A" {
		t.Fatalf("svc shape wrong: %+v", s)
	}
	if len(s.Ports) != 1 || s.Ports[0] != "80:8080/TCP" {
		t.Fatalf("ports = %v", s.Ports)
	}
}

func TestIntegration_Nodes(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/nodes")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]NodeInfo](t, body)
	if len(out) != 2 {
		t.Fatalf("node count = %d", len(out))
	}
	byName := indexBy(out, func(n NodeInfo) string { return n.Name })
	n1 := byName["node-1"]
	if n1.Status != "Ready" {
		t.Fatalf("n1 status = %q", n1.Status)
	}
	if len(n1.Roles) != 1 || n1.Roles[0] != "control-plane" {
		t.Fatalf("n1 roles = %v", n1.Roles)
	}
	if n1.CPU != "4" || n1.Memory != "8Gi" || n1.Pods != "110" {
		t.Fatalf("n1 capacity wrong: %+v", n1)
	}
	n2 := byName["node-2"]
	if len(n2.Roles) != 1 || n2.Roles[0] != "<none>" {
		t.Fatalf("n2 roles = %v, want [<none>]", n2.Roles)
	}
}

func TestIntegration_Events(t *testing.T) {
	srv := newIntegrationServer(t)
	code, body := fetchJSON(t, srv, "/api/events")
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	out := decode[[]KubeEvent](t, body)
	if len(out) != 2 {
		t.Fatalf("event count = %d", len(out))
	}
	byReason := indexBy(out, func(e KubeEvent) string { return e.Reason })
	if e, ok := byReason["Scheduled"]; !ok || e.Object != "Pod/web-abc" || e.Source != "default-scheduler" {
		t.Fatalf("Scheduled event = %+v", e)
	}
	if e, ok := byReason["BackOff"]; !ok || e.Object != "Pod/broken" || e.Count != 4 || e.Type != "Warning" {
		t.Fatalf("BackOff event = %+v", e)
	}
}

// TestIntegration_AllResponseShapesMatchFrontendContract round-trips every
// endpoint through the frontend's API interface definitions. If any field
// name/type drifts, the json.Unmarshal into the typed Go struct (which has
// the same JSON tags as the TypeScript interface in api.ts) will fail or
// produce zero values that we can detect.
func TestIntegration_AllResponseShapesMatchFrontendContract(t *testing.T) {
	srv := newIntegrationServer(t)

	type endpoint struct {
		name   string
		path   string
		assert func(t *testing.T, body []byte)
	}
	endpoints := []endpoint{
		{
			name:   "cluster",
			path:   "/api/cluster",
			assert: func(t *testing.T, b []byte) { _ = decode[ClusterInfo](t, b) },
		},
		{
			name:   "namespaces",
			path:   "/api/namespaces",
			assert: func(t *testing.T, b []byte) { _ = decode[[]Namespace](t, b) },
		},
		{
			name:   "pods",
			path:   "/api/pods",
			assert: func(t *testing.T, b []byte) { _ = decode[[]Pod](t, b) },
		},
		{
			name:   "deployments",
			path:   "/api/deployments",
			assert: func(t *testing.T, b []byte) { _ = decode[[]Deployment](t, b) },
		},
		{
			name:   "services",
			path:   "/api/services",
			assert: func(t *testing.T, b []byte) { _ = decode[[]Service](t, b) },
		},
		{
			name:   "nodes",
			path:   "/api/nodes",
			assert: func(t *testing.T, b []byte) { _ = decode[[]NodeInfo](t, b) },
		},
		{
			name:   "events",
			path:   "/api/events",
			assert: func(t *testing.T, b []byte) { _ = decode[[]KubeEvent](t, b) },
		},
	}
	for _, e := range endpoints {
		t.Run(e.name, func(t *testing.T) {
			code, body := fetchJSON(t, srv, e.path)
			if code != 200 {
				t.Fatalf("code = %d, body = %s", code, body)
			}
			// Forbid `null` anywhere in the response — the frontend treats
			// every collection field as non-nullable.
			if strings.Contains(string(body), "null") {
				t.Fatalf("response contains null literal: %s", body)
			}
			e.assert(t, body)
		})
	}
}

// --- tiny generic helpers (kept private to this file) --------------------

func namesOf[T any, K comparable](xs []T, f func(T) K) []K {
	out := make([]K, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

func indexBy[T any, K comparable](xs []T, f func(T) K) map[K]T {
	out := make(map[K]T, len(xs))
	for _, x := range xs {
		out[f(x)] = x
	}
	return out
}

func wantSubset[T comparable](t *testing.T, got []T, want []T) {
	t.Helper()
	gotSet := map[T]bool{}
	for _, g := range got {
		gotSet[g] = true
	}
	for _, w := range want {
		if !gotSet[w] {
			t.Fatalf("missing expected element %v in %v", w, got)
		}
	}
}
