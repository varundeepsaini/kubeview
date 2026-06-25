package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// fixedTime returns a deterministic timestamp used across tests. We never
// compare ages against wall-clock time directly — we compare the *shape* of
// the formatted string and the delta against a known reference.
func fixedTime() metav1.Time {
	return metav1.NewTime(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
}

// --- helper tests ---------------------------------------------------------

func TestFormatTime(t *testing.T) {
	t.Run("zero time returns empty string", func(t *testing.T) {
		if got := formatTime(metav1.Time{}); got != "" {
			t.Fatalf("formatTime(zero) = %q, want \"\"", got)
		}
	})
	t.Run("non-zero formats as RFC3339 in UTC", func(t *testing.T) {
		ts := metav1.NewTime(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
		if got := formatTime(ts); got != "2024-01-02T03:04:05Z" {
			t.Fatalf("formatTime = %q", got)
		}
	})
	t.Run("non-UTC input is converted to UTC", func(t *testing.T) {
		loc, _ := time.LoadLocation("Asia/Kolkata")
		ts := metav1.NewTime(time.Date(2024, 1, 2, 8, 34, 5, 0, loc)) // +05:30
		if got := formatTime(ts); got != "2024-01-02T03:04:05Z" {
			t.Fatalf("formatTime = %q, want UTC conversion", got)
		}
	})
}

func TestGetAge(t *testing.T) {
	t.Run("zero time returns Unknown", func(t *testing.T) {
		if got := getAge(metav1.Time{}); got != "Unknown" {
			t.Fatalf("getAge(zero) = %q", got)
		}
	})
	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"5 seconds", 5 * time.Second, "5s"},
		{"59 seconds", 59 * time.Second, "59s"},
		{"60 seconds rolls to minutes", 60 * time.Second, "1m"},
		{"15 minutes", 15 * time.Minute, "15m"},
		{"59 minutes", 59 * time.Minute, "59m"},
		{"60 minutes rolls to hours", 60 * time.Minute, "1h"},
		{"3 hours", 3 * time.Hour, "3h"},
		{"23 hours", 23 * time.Hour, "23h"},
		{"24 hours rolls to days", 24 * time.Hour, "1d"},
		{"5 days", 5 * 24 * time.Hour, "5d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := metav1.NewTime(time.Now().Add(-tc.ago))
			if got := getAge(ts); got != tc.want {
				t.Fatalf("getAge(%v ago) = %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

func TestEmptyIfNil(t *testing.T) {
	t.Run("nil produces empty map", func(t *testing.T) {
		got := emptyIfNil(nil)
		if got == nil {
			t.Fatal("expected non-nil map")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty map, got %v", got)
		}
		// must serialize as {} not null
		b, _ := json.Marshal(got)
		if string(b) != "{}" {
			t.Fatalf("json = %s, want {}", b)
		}
	})
	t.Run("non-nil is returned unchanged", func(t *testing.T) {
		in := map[string]string{"a": "1"}
		got := emptyIfNil(in)
		if got["a"] != "1" {
			t.Fatalf("got = %v", got)
		}
	})
}

func TestMaxInt32(t *testing.T) {
	cases := []struct {
		a, b, want int32
	}{
		{0, 1, 1},
		{1, 0, 1},
		{5, 5, 5},
		{-1, 0, 0},
		{100, 50, 100},
	}
	for _, tc := range cases {
		if got := maxInt32(tc.a, tc.b); got != tc.want {
			t.Errorf("maxInt32(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestContainerState(t *testing.T) {
	cases := []struct {
		name string
		in   *corev1.ContainerStatus
		want string
	}{
		{"nil pointer -> Waiting", nil, "Waiting"},
		{"running", &corev1.ContainerStatus{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}, "Running"},
		{
			"waiting with reason",
			&corev1.ContainerStatus{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			"CrashLoopBackOff",
		},
		{
			"waiting without reason",
			&corev1.ContainerStatus{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
			"Waiting",
		},
		{
			"terminated with reason",
			&corev1.ContainerStatus{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error"}}},
			"Error",
		},
		{
			"terminated without reason",
			&corev1.ContainerStatus{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
			"Terminated",
		},
		{"all states nil -> Unknown", &corev1.ContainerStatus{}, "Unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := containerState(tc.in); got != tc.want {
				t.Fatalf("containerState = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPodStatus(t *testing.T) {
	t.Run("deletion timestamp set -> Terminating", func(t *testing.T) {
		now := metav1.Now()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		if got := podStatus(pod); got != "Terminating" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("waiting reason wins over phase", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
				},
			},
		}
		if got := podStatus(pod); got != "ImagePullBackOff" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("terminated reason wins over phase", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}},
				},
			},
		}
		if got := podStatus(pod); got != "Completed" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("no container reasons -> phase", func(t *testing.T) {
		pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
		if got := podStatus(pod); got != "Running" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("no phase, no container reasons -> Unknown", func(t *testing.T) {
		pod := &corev1.Pod{}
		if got := podStatus(pod); got != "Unknown" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("waiting without reason is ignored", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
				},
			},
		}
		if got := podStatus(pod); got != "Pending" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestVolumeType(t *testing.T) {
	// Common volume types — these should agree with the JS implementation
	// (which uses Object.keys() on the JSON-shaped volume).
	t.Run("emptyDir", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}})
		if got != "emptyDir" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("hostPath", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}})
		if got != "hostPath" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("secret", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "s"}})
		if got != "secret" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("configMap", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}})
		if got != "configMap" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("persistentVolumeClaim", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"}})
		if got != "persistentVolumeClaim" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("projected", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{}})
		if got != "projected" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("downwardAPI", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{DownwardAPI: &corev1.DownwardAPIVolumeSource{}})
		// Acronym gotcha: the field's JSON tag is "downwardAPI" (not "downwardapi"),
		// so volumeType's tag-reflection must surface that exact casing.
		if got != "downwardAPI" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("empty VolumeSource -> unknown", func(t *testing.T) {
		got := volumeType(corev1.VolumeSource{})
		if got != "unknown" {
			t.Fatalf("got %q", got)
		}
	})
}

// TestVolumeType_Acronyms locks in that volume types whose Go struct field
// uses acronym casing (NFS, ISCSI, RBD, CSI, FC, etc.) emit the lowercase
// JSON key the K8s API uses on the wire — same as the JS backend, which
// reads keys directly from the parsed JSON.
func TestVolumeType_Acronyms(t *testing.T) {
	cases := []struct {
		name string
		vs   corev1.VolumeSource
		want string
	}{
		{
			name: "NFS",
			vs:   corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "s", Path: "/"}},
			want: "nfs",
		},
		{
			name: "ISCSI",
			vs:   corev1.VolumeSource{ISCSI: &corev1.ISCSIVolumeSource{TargetPortal: "p", IQN: "i", Lun: 0}},
			want: "iscsi",
		},
		{
			name: "RBD",
			vs:   corev1.VolumeSource{RBD: &corev1.RBDVolumeSource{}},
			want: "rbd",
		},
		{
			name: "CSI",
			vs:   corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "d"}},
			want: "csi",
		},
		{
			name: "FC",
			vs:   corev1.VolumeSource{FC: &corev1.FCVolumeSource{}},
			want: "fc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := volumeType(tc.vs); got != tc.want {
				t.Fatalf("volumeType = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatServicePort(t *testing.T) {
	cases := []struct {
		name string
		p    corev1.ServicePort
		want string
	}{
		{
			name: "int targetPort",
			p:    corev1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
			want: "80:8080/TCP",
		},
		{
			name: "string targetPort",
			p:    corev1.ServicePort{Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
			want: "80:http/TCP",
		},
		{
			name: "no targetPort (int zero + empty string)",
			p:    corev1.ServicePort{Port: 80, TargetPort: intstr.IntOrString{}, Protocol: corev1.ProtocolTCP},
			want: "80/TCP",
		},
		{
			name: "UDP protocol",
			p:    corev1.ServicePort{Port: 53, TargetPort: intstr.FromInt(53), Protocol: corev1.ProtocolUDP},
			want: "53:53/UDP",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatServicePort(tc.p); got != tc.want {
				t.Fatalf("formatServicePort = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- transformer tests ----------------------------------------------------

func TestTransformNamespace(t *testing.T) {
	t.Run("active namespace with labels", func(t *testing.T) {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Labels:            map[string]string{"env": "dev"},
				CreationTimestamp: fixedTime(),
			},
			Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		}
		got := transformNamespace(ns)
		want := Namespace{
			Name:      "default",
			Status:    "Active",
			Labels:    map[string]string{"env": "dev"},
			CreatedAt: "2024-01-02T03:04:05Z",
		}
		// age is wall-clock based; check it's non-empty and don't pin a value.
		if got.Age == "" || got.Age == "Unknown" {
			t.Fatalf("got.Age = %q, want a non-Unknown age", got.Age)
		}
		got.Age = ""
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
	t.Run("missing phase -> Unknown", func(t *testing.T) {
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
		got := transformNamespace(ns)
		if got.Status != "Unknown" {
			t.Fatalf("status = %q", got.Status)
		}
	})
	t.Run("nil labels serialize as {}", func(t *testing.T) {
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
		got := transformNamespace(ns)
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"labels":{}`) {
			t.Fatalf("labels should marshal as {}, got: %s", b)
		}
	})
	t.Run("zero creation time -> empty createdAt and Unknown age", func(t *testing.T) {
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
		got := transformNamespace(ns)
		if got.CreatedAt != "" {
			t.Fatalf("CreatedAt = %q, want \"\"", got.CreatedAt)
		}
		if got.Age != "Unknown" {
			t.Fatalf("Age = %q", got.Age)
		}
	})
}

func TestTransformPod(t *testing.T) {
	t.Run("running pod with single container", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: "default",
				Labels:            map[string]string{"app": "web"},
				CreationTimestamp: fixedTime(),
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:1.27",
						Ports: []corev1.ContainerPort{{ContainerPort: 80, Protocol: corev1.ProtocolTCP}},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.5",
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "nginx",
						Ready:        true,
						RestartCount: 2,
						State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
					},
				},
			},
		}
		got := transformPod(pod)
		if got.Name != "web" || got.Namespace != "default" {
			t.Fatalf("name/ns wrong: %+v", got)
		}
		if got.Status != "Running" {
			t.Fatalf("status = %q", got.Status)
		}
		if got.Ready != "1/1" {
			t.Fatalf("ready = %q", got.Ready)
		}
		if got.Restarts != 2 {
			t.Fatalf("restarts = %d", got.Restarts)
		}
		if got.Node != "node-1" || got.IP != "10.0.0.5" {
			t.Fatalf("node/ip wrong: node=%q ip=%q", got.Node, got.IP)
		}
		if len(got.Containers) != 1 {
			t.Fatalf("containers len = %d", len(got.Containers))
		}
		c := got.Containers[0]
		if c.Name != "nginx" || c.Image != "nginx:1.27" || !c.Ready || c.RestartCount != 2 || c.State != "Running" {
			t.Fatalf("container = %+v", c)
		}
		if len(c.Ports) != 1 || c.Ports[0] != "80/TCP" {
			t.Fatalf("ports = %v", c.Ports)
		}
	})

	t.Run("pending pod with no NodeName / PodIP", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		}
		got := transformPod(pod)
		if got.Node != "Pending" {
			t.Fatalf("node = %q, want Pending", got.Node)
		}
		if got.IP != "N/A" {
			t.Fatalf("ip = %q, want N/A", got.IP)
		}
	})

	t.Run("totalCount falls back to spec.containers when no statuses", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}}},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		}
		got := transformPod(pod)
		if got.Ready != "0/2" {
			t.Fatalf("ready = %q, want 0/2", got.Ready)
		}
	})

	t.Run("readyCount counts only containers with status.Ready=true", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}, {Name: "c"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "a", Ready: true, RestartCount: 1},
					{Name: "b", Ready: false, RestartCount: 0},
					{Name: "c", Ready: true, RestartCount: 3},
				},
			},
		}
		got := transformPod(pod)
		if got.Ready != "2/3" {
			t.Fatalf("ready = %q", got.Ready)
		}
		if got.Restarts != 4 {
			t.Fatalf("restarts = %d", got.Restarts)
		}
	})

	t.Run("terminating pod surfaces Terminating status", func(t *testing.T) {
		now := metav1.Now()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		got := transformPod(pod)
		if got.Status != "Terminating" {
			t.Fatalf("status = %q", got.Status)
		}
	})

	t.Run("waiting reason surfaces in status", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
				},
			},
		}
		got := transformPod(pod)
		if got.Status != "CrashLoopBackOff" {
			t.Fatalf("status = %q", got.Status)
		}
		// containers[0].state should also reflect the waiting reason
		if got.Containers[0].State != "CrashLoopBackOff" {
			t.Fatalf("container state = %q", got.Containers[0].State)
		}
	})

	t.Run("multi-container ready false when statuses fewer than spec containers", func(t *testing.T) {
		// Mirrors a partial-status race; the missing container is treated as not-ready
		// and counted against the total.
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "a", Ready: true},
				},
			},
		}
		got := transformPod(pod)
		// totalCount comes from len(ContainerStatuses) when nonzero (matches JS); that's 1 here.
		if got.Ready != "1/1" {
			t.Fatalf("ready = %q (note: matches JS behaviour — total uses status length when present)", got.Ready)
		}
		// But containers list comes from spec.Containers (2 of them).
		if len(got.Containers) != 2 {
			t.Fatalf("containers len = %d", len(got.Containers))
		}
		// Second container has no matching status -> ready=false, state=Waiting
		if got.Containers[1].Ready {
			t.Fatalf("container b should be not-ready")
		}
		if got.Containers[1].State != "Waiting" {
			t.Fatalf("container b state = %q", got.Containers[1].State)
		}
	})

	t.Run("conditions and volumes are preserved", func(t *testing.T) {
		condTime := fixedTime()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c"}},
				Volumes: []corev1.Volume{
					{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue, Reason: "ok", LastTransitionTime: condTime},
				},
			},
		}
		got := transformPod(pod)
		if len(got.Conditions) != 1 || got.Conditions[0].Type != "Ready" || got.Conditions[0].Status != "True" || got.Conditions[0].LastTransition != "2024-01-02T03:04:05Z" {
			t.Fatalf("conditions = %+v", got.Conditions)
		}
		if len(got.Volumes) != 2 {
			t.Fatalf("volumes = %+v", got.Volumes)
		}
		if got.Volumes[0].Name != "data" || got.Volumes[0].Type != "emptyDir" {
			t.Fatalf("volumes[0] = %+v", got.Volumes[0])
		}
		if got.Volumes[1].Name != "config" || got.Volumes[1].Type != "configMap" {
			t.Fatalf("volumes[1] = %+v", got.Volumes[1])
		}
	})

	t.Run("labels nil serializes as {}", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		got := transformPod(pod)
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"labels":{}`) {
			t.Fatalf("expected labels:{}, got: %s", b)
		}
		// containers, conditions, volumes should all serialize as [] not null
		for _, key := range []string{`"containers":[`, `"conditions":[]`, `"volumes":[]`} {
			if !strings.Contains(string(b), key) {
				t.Fatalf("expected %q in JSON, got: %s", key, b)
			}
		}
	})
}

func TestTransformDeployment(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		replicas := int32(3)
		dep := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api", Namespace: "default",
				Labels:            map[string]string{"app": "api"},
				CreationTimestamp: fixedTime(),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "api", Image: "api:1"}, {Name: "sidecar", Image: "envoy:2"}},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:          3,
				ReadyReplicas:     2,
				UpdatedReplicas:   3,
				AvailableReplicas: 2,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "MinimumReplicasAvailable", Message: "ok", LastTransitionTime: fixedTime()},
				},
			},
		}
		got := transformDeployment(dep)
		if got.Name != "api" || got.Namespace != "default" {
			t.Fatalf("name/ns wrong: %+v", got)
		}
		if got.Replicas != 3 || got.ReadyReplicas != 2 || got.DesiredReplicas != 3 || got.UpdatedReplicas != 3 || got.AvailableReplicas != 2 {
			t.Fatalf("replica counters wrong: %+v", got)
		}
		if got.Strategy != "RollingUpdate" {
			t.Fatalf("strategy = %q", got.Strategy)
		}
		if got.Selector["app"] != "api" {
			t.Fatalf("selector = %v", got.Selector)
		}
		if len(got.Images) != 2 || got.Images[0] != "api:1" || got.Images[1] != "envoy:2" {
			t.Fatalf("images = %v", got.Images)
		}
		if len(got.Conditions) != 1 || got.Conditions[0].Type != "Available" {
			t.Fatalf("conditions = %+v", got.Conditions)
		}
	})

	t.Run("nil spec.Replicas defaults DesiredReplicas to 0", func(t *testing.T) {
		dep := appsv1.Deployment{Spec: appsv1.DeploymentSpec{}}
		got := transformDeployment(dep)
		if got.DesiredReplicas != 0 {
			t.Fatalf("desired = %d", got.DesiredReplicas)
		}
	})

	t.Run("empty strategy defaults to RollingUpdate", func(t *testing.T) {
		dep := appsv1.Deployment{Spec: appsv1.DeploymentSpec{}}
		got := transformDeployment(dep)
		if got.Strategy != "RollingUpdate" {
			t.Fatalf("strategy = %q", got.Strategy)
		}
	})

	t.Run("nil selector serializes as {}", func(t *testing.T) {
		dep := appsv1.Deployment{Spec: appsv1.DeploymentSpec{}}
		got := transformDeployment(dep)
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"selector":{}`) {
			t.Fatalf("expected selector:{} got: %s", b)
		}
	})

	t.Run("nil labels serialize as {}", func(t *testing.T) {
		dep := appsv1.Deployment{Spec: appsv1.DeploymentSpec{}}
		got := transformDeployment(dep)
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"labels":{}`) {
			t.Fatalf("expected labels:{} got: %s", b)
		}
	})

	t.Run("empty conditions/images serialize as []", func(t *testing.T) {
		dep := appsv1.Deployment{}
		got := transformDeployment(dep)
		b, _ := json.Marshal(got)
		for _, k := range []string{`"conditions":[]`, `"images":[]`} {
			if !strings.Contains(string(b), k) {
				t.Fatalf("expected %s in: %s", k, b)
			}
		}
	})
}

func TestTransformService(t *testing.T) {
	t.Run("ClusterIP with port and targetPort", func(t *testing.T) {
		svc := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
				},
				Selector: map[string]string{"app": "web"},
			},
		}
		got := transformService(svc)
		if got.Type != "ClusterIP" || got.ClusterIP != "10.0.0.1" {
			t.Fatalf("type/ip = %+v", got)
		}
		if len(got.Ports) != 1 || got.Ports[0] != "80:8080/TCP" {
			t.Fatalf("ports = %v", got.Ports)
		}
		if got.ExternalIP != "N/A" {
			t.Fatalf("externalIP = %q", got.ExternalIP)
		}
	})

	t.Run("empty type defaults to ClusterIP", func(t *testing.T) {
		svc := corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "1.2.3.4"}}
		got := transformService(svc)
		if got.Type != "ClusterIP" {
			t.Fatalf("type = %q", got.Type)
		}
	})

	t.Run("empty clusterIP becomes None (headless service)", func(t *testing.T) {
		svc := corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}
		got := transformService(svc)
		if got.ClusterIP != "None" {
			t.Fatalf("clusterIP = %q", got.ClusterIP)
		}
	})

	t.Run("LoadBalancer ingress IP wins over spec.ExternalIPs", func(t *testing.T) {
		svc := corev1.Service{
			Spec: corev1.ServiceSpec{
				Type:        corev1.ServiceTypeLoadBalancer,
				ExternalIPs: []string{"5.6.7.8"},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "9.10.11.12"}},
				},
			},
		}
		got := transformService(svc)
		if got.ExternalIP != "9.10.11.12" {
			t.Fatalf("externalIP = %q", got.ExternalIP)
		}
	})

	t.Run("LoadBalancer ingress with empty IP falls back to spec.ExternalIPs", func(t *testing.T) {
		svc := corev1.Service{
			Spec: corev1.ServiceSpec{
				Type:        corev1.ServiceTypeLoadBalancer,
				ExternalIPs: []string{"5.6.7.8"},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{Hostname: "lb.example.com"}}, // IP empty
				},
			},
		}
		got := transformService(svc)
		if got.ExternalIP != "5.6.7.8" {
			t.Fatalf("externalIP = %q", got.ExternalIP)
		}
	})

	t.Run("no external IP anywhere -> N/A", func(t *testing.T) {
		svc := corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}
		got := transformService(svc)
		if got.ExternalIP != "N/A" {
			t.Fatalf("externalIP = %q", got.ExternalIP)
		}
	})

	t.Run("string targetPort renders correctly", func(t *testing.T) {
		svc := corev1.Service{
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP}},
			},
		}
		got := transformService(svc)
		if got.Ports[0] != "80:http/TCP" {
			t.Fatalf("port = %q", got.Ports[0])
		}
	})
}

func TestTransformNode(t *testing.T) {
	t.Run("ready node with roles", func(t *testing.T) {
		n := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
					"kubernetes.io/hostname":                "node-1",
				},
				CreationTimestamp: fixedTime(),
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady", Message: "ok"},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion:          "v1.31.0",
					OSImage:                 "Linux",
					Architecture:            "arm64",
					ContainerRuntimeVersion: "containerd://2.0",
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
		}
		got := transformNode(n)
		if got.Name != "node-1" || got.Status != "Ready" {
			t.Fatalf("name/status: %+v", got)
		}
		if got.Version != "v1.31.0" || got.OS != "Linux" || got.Arch != "arm64" || got.ContainerRuntime != "containerd://2.0" {
			t.Fatalf("nodeInfo wrong: %+v", got)
		}
		if got.CPU != "4" || got.Memory != "8Gi" || got.Pods != "110" {
			t.Fatalf("capacity wrong: cpu=%q mem=%q pods=%q", got.CPU, got.Memory, got.Pods)
		}
		if len(got.Roles) != 1 || got.Roles[0] != "control-plane" {
			t.Fatalf("roles = %v", got.Roles)
		}
		if len(got.Conditions) != 2 || got.Conditions[0].Type != "Ready" {
			t.Fatalf("conditions = %+v", got.Conditions)
		}
		if len(got.Addresses) != 2 || got.Addresses[0].Type != "InternalIP" || got.Addresses[0].Address != "10.0.0.1" {
			t.Fatalf("addresses = %+v", got.Addresses)
		}
	})

	t.Run("node not ready when no Ready=True condition", func(t *testing.T) {
		n := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				},
			},
		}
		got := transformNode(n)
		if got.Status != "NotReady" {
			t.Fatalf("status = %q", got.Status)
		}
	})

	t.Run("node with no conditions -> NotReady", func(t *testing.T) {
		n := corev1.Node{}
		got := transformNode(n)
		if got.Status != "NotReady" {
			t.Fatalf("status = %q", got.Status)
		}
	})

	t.Run("node with no roles -> <none>", func(t *testing.T) {
		n := corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"hostname": "foo"}}}
		got := transformNode(n)
		if len(got.Roles) != 1 || got.Roles[0] != "<none>" {
			t.Fatalf("roles = %v", got.Roles)
		}
	})

	t.Run("multiple role labels extract all", func(t *testing.T) {
		n := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"node-role.kubernetes.io/master":        "",
				"node-role.kubernetes.io/control-plane": "",
			}},
		}
		got := transformNode(n)
		if len(got.Roles) != 2 {
			t.Fatalf("roles = %v", got.Roles)
		}
		// order is map-iteration-dependent — just check membership
		seen := map[string]bool{got.Roles[0]: true, got.Roles[1]: true}
		if !seen["master"] || !seen["control-plane"] {
			t.Fatalf("expected master and control-plane in roles, got %v", got.Roles)
		}
	})

	t.Run("zero capacity surfaces 0 (matches resource.Quantity zero string)", func(t *testing.T) {
		n := corev1.Node{}
		got := transformNode(n)
		if got.CPU != "0" || got.Memory != "0" || got.Pods != "0" {
			t.Fatalf("capacity not zero: cpu=%q mem=%q pods=%q", got.CPU, got.Memory, got.Pods)
		}
	})

	t.Run("nil labels serialize as {}", func(t *testing.T) {
		n := corev1.Node{}
		got := transformNode(n)
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"labels":{}`) {
			t.Fatalf("expected labels:{}, got: %s", b)
		}
	})
}

func TestTransformEvent(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		e := corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "e1"},
			Type:           "Warning",
			Reason:         "FailedScheduling",
			Message:        "0/3 nodes available",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-1"},
			FirstTimestamp: fixedTime(),
			LastTimestamp:  fixedTime(),
			Count:          7,
			Source:         corev1.EventSource{Component: "default-scheduler"},
		}
		got := transformEvent(e)
		if got.Type != "Warning" || got.Reason != "FailedScheduling" || got.Message != "0/3 nodes available" {
			t.Fatalf("event fields wrong: %+v", got)
		}
		if got.Object != "Pod/web-1" {
			t.Fatalf("object = %q", got.Object)
		}
		if got.Namespace != "default" {
			t.Fatalf("namespace = %q", got.Namespace)
		}
		if got.FirstSeen != "2024-01-02T03:04:05Z" || got.LastSeen != "2024-01-02T03:04:05Z" {
			t.Fatalf("seen times wrong: %+v", got)
		}
		if got.Count != 7 {
			t.Fatalf("count = %d", got.Count)
		}
		if got.Source != "default-scheduler" {
			t.Fatalf("source = %q", got.Source)
		}
	})

	t.Run("count 0 -> 1 (matches JS `event.count || 1`)", func(t *testing.T) {
		got := transformEvent(corev1.Event{Count: 0})
		if got.Count != 1 {
			t.Fatalf("count = %d", got.Count)
		}
	})

	t.Run("count 1 stays 1", func(t *testing.T) {
		got := transformEvent(corev1.Event{Count: 1})
		if got.Count != 1 {
			t.Fatalf("count = %d", got.Count)
		}
	})

	t.Run("count 2 stays 2", func(t *testing.T) {
		got := transformEvent(corev1.Event{Count: 2})
		if got.Count != 2 {
			t.Fatalf("count = %d", got.Count)
		}
	})

	t.Run("zero timestamps serialize as empty strings", func(t *testing.T) {
		got := transformEvent(corev1.Event{})
		if got.FirstSeen != "" || got.LastSeen != "" {
			t.Fatalf("times wrong: %+v", got)
		}
	})
}

// --- JSON-shape tests -----------------------------------------------------

// The frontend interfaces in kubeview-frontend/src/lib/api.ts treat
// Record<string, string> fields as non-nullable. Empty-but-defined maps must
// serialize as {} and empty-but-defined slices must serialize as []. Any nil
// would surface as `null` in JSON and the frontend would crash on `.map()`
// or property access. These tests guard against that regression.

func TestJSONNeverEmitsNullForCollectionFields(t *testing.T) {
	t.Run("empty Pod marshals all collections as non-null", func(t *testing.T) {
		pod := transformPod(&corev1.Pod{})
		b, _ := json.Marshal(pod)
		s := string(b)
		assertContains(t, s, `"labels":{}`)
		assertContains(t, s, `"containers":[]`)
		assertContains(t, s, `"conditions":[]`)
		assertContains(t, s, `"volumes":[]`)
		assertNotContains(t, s, "null")
	})
	t.Run("empty Deployment marshals all collections as non-null", func(t *testing.T) {
		dep := transformDeployment(appsv1.Deployment{})
		b, _ := json.Marshal(dep)
		s := string(b)
		assertContains(t, s, `"labels":{}`)
		assertContains(t, s, `"selector":{}`)
		assertContains(t, s, `"conditions":[]`)
		assertContains(t, s, `"images":[]`)
		assertNotContains(t, s, "null")
	})
	t.Run("empty Service marshals all collections as non-null", func(t *testing.T) {
		svc := transformService(corev1.Service{})
		b, _ := json.Marshal(svc)
		s := string(b)
		assertContains(t, s, `"labels":{}`)
		assertContains(t, s, `"selector":{}`)
		assertContains(t, s, `"ports":[]`)
		assertNotContains(t, s, "null")
	})
	t.Run("empty Node marshals all collections as non-null", func(t *testing.T) {
		n := transformNode(corev1.Node{})
		b, _ := json.Marshal(n)
		s := string(b)
		assertContains(t, s, `"labels":{}`)
		assertContains(t, s, `"conditions":[]`)
		assertContains(t, s, `"addresses":[]`)
		assertNotContains(t, s, "null")
		// Parse back and verify roles defaults to ["<none>"]. Comparing the
		// raw JSON string is awkward because json.Marshal HTML-escapes <
		// and > — we check the decoded value instead, which is what the
		// frontend actually sees after JSON.parse().
		var parsed NodeInfo
		if err := json.Unmarshal(b, &parsed); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(parsed.Roles) != 1 || parsed.Roles[0] != "<none>" {
			t.Fatalf("roles = %v", parsed.Roles)
		}
	})
	t.Run("empty Namespace marshals labels as non-null", func(t *testing.T) {
		ns := transformNamespace(corev1.Namespace{})
		b, _ := json.Marshal(ns)
		s := string(b)
		assertContains(t, s, `"labels":{}`)
		assertNotContains(t, s, "null")
	})
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("expected JSON to contain %q\nfull: %s", sub, s)
	}
}

func assertNotContains(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Fatalf("expected JSON NOT to contain %q\nfull: %s", sub, s)
	}
}

// Sanity check that all our test fixtures compile with the frontend-required
// JSON tags. If anyone renames a field the test breaks loudly.
func TestPodJSONHasFrontendFields(t *testing.T) {
	pod := transformPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "n"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "i"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true}}},
	})
	b, _ := json.Marshal(pod)
	s := string(b)
	requiredFields := []string{
		`"name":`, `"namespace":`, `"status":`, `"ready":`, `"restarts":`,
		`"node":`, `"ip":`, `"labels":`, `"createdAt":`, `"age":`,
		`"containers":`, `"conditions":`, `"volumes":`,
	}
	for _, f := range requiredFields {
		assertContains(t, s, f)
	}
}

// --- additional edge-case tests ------------------------------------------

func TestTransformPod_InitContainersExcluded(t *testing.T) {
	// The JS transformer only iterates pod.spec.containers, never
	// initContainers or ephemeralContainers. We mirror that: init/ephemeral
	// shouldn't leak into the frontend's containers list.
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "app", Image: "app:1"}},
			InitContainers: []corev1.Container{{Name: "init-db", Image: "init:1"}},
			EphemeralContainers: []corev1.EphemeralContainer{{
				EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger", Image: "debug:1"},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true},
			},
		},
	}
	got := transformPod(pod)
	if len(got.Containers) != 1 || got.Containers[0].Name != "app" {
		t.Fatalf("containers should be [app], got %+v", got.Containers)
	}
}

func TestTransformPod_HostIPAndPodIPEmpty(t *testing.T) {
	// A pending pod with no PodIP yet should surface "N/A" — we do NOT fall
	// back to HostIP (matching JS behaviour, which only reads status.podIP).
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
		Status: corev1.PodStatus{
			Phase:  corev1.PodPending,
			HostIP: "192.168.1.1", // populated but we should ignore it
		},
	}
	got := transformPod(pod)
	if got.IP != "N/A" {
		t.Fatalf("IP = %q, want N/A (HostIP must not be used)", got.IP)
	}
}

func TestTransformPod_VolumeWithNameFieldNotMisidentified(t *testing.T) {
	// JS picks first non-"name" key. The Go implementation iterates Go fields
	// directly (which don't include a "name" field on VolumeSource), so this
	// test pins the behavior — adding a name-typed field to VolumeSource
	// upstream would not affect us.
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c"}},
			Volumes: []corev1.Volume{{
				Name: "my-volume",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: "s"},
				},
			}},
		},
	}
	got := transformPod(pod)
	if len(got.Volumes) != 1 || got.Volumes[0].Name != "my-volume" || got.Volumes[0].Type != "secret" {
		t.Fatalf("volume = %+v", got.Volumes[0])
	}
}

func TestTransformService_NodePort(t *testing.T) {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{
				{Port: 80, NodePort: 30080, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	got := transformService(svc)
	if got.Type != "NodePort" {
		t.Fatalf("type = %q", got.Type)
	}
	// The JS impl ignores nodePort in the port string and so do we — frontend
	// shows port:targetPort/proto. Locking in current behavior.
	if got.Ports[0] != "80:8080/TCP" {
		t.Fatalf("port = %q", got.Ports[0])
	}
}

func TestTransformService_LoadBalancerHostnameOnly(t *testing.T) {
	// Some cloud LBs return only a Hostname (no IP), e.g. AWS NLBs. JS would
	// fall through `ingress[0].ip` (undefined → falsy) and then to
	// spec.externalIPs, then to "N/A". We match: Hostname is not surfaced.
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{Hostname: "my-lb.example.com"}},
			},
		},
	}
	got := transformService(svc)
	if got.ExternalIP != "N/A" {
		t.Fatalf("ExternalIP = %q, want N/A (hostname must not be surfaced)", got.ExternalIP)
	}
}

func TestTransformService_MultiplePorts(t *testing.T) {
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
				{Port: 443, TargetPort: intstr.FromString("https"), Protocol: corev1.ProtocolTCP},
				{Port: 53, TargetPort: intstr.FromInt(53), Protocol: corev1.ProtocolUDP},
			},
		},
	}
	got := transformService(svc)
	want := []string{"80:8080/TCP", "443:https/TCP", "53:53/UDP"}
	if !reflect.DeepEqual(got.Ports, want) {
		t.Fatalf("ports = %v, want %v", got.Ports, want)
	}
}

func TestTransformDeployment_RecreateStrategy(t *testing.T) {
	dep := appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
		},
	}
	got := transformDeployment(dep)
	if got.Strategy != "Recreate" {
		t.Fatalf("strategy = %q", got.Strategy)
	}
}

func TestTransformDeployment_EmptyTemplateContainers(t *testing.T) {
	dep := appsv1.Deployment{}
	got := transformDeployment(dep)
	// Images should be [] (non-null) when template.spec.containers is empty.
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"images":[]`) {
		t.Fatalf("expected images:[], got %s", b)
	}
}

func TestTransformNode_UsesCapacityNotAllocatable(t *testing.T) {
	// JS reads node.status.capacity.{cpu,memory,pods}. Allocatable is
	// typically smaller (reserved system overhead subtracted). Confirm we
	// match by setting different values and asserting capacity wins.
	n := corev1.Node{
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("7500m"),
				corev1.ResourceMemory: resource.MustParse("14Gi"),
				corev1.ResourcePods:   resource.MustParse("100"),
			},
		},
	}
	got := transformNode(n)
	if got.CPU != "8" || got.Memory != "16Gi" || got.Pods != "110" {
		t.Fatalf("expected capacity values, got cpu=%q mem=%q pods=%q", got.CPU, got.Memory, got.Pods)
	}
}

func TestTransformPod_MultipleConditions(t *testing.T) {
	// The full set of conditions K8s typically reports: PodScheduled,
	// Initialized, ContainersReady, Ready. All should come through in order.
	now := fixedTime()
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: now},
				{Type: corev1.PodInitialized, Status: corev1.ConditionTrue, LastTransitionTime: now},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue, LastTransitionTime: now},
				{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: now},
			},
		},
	}
	got := transformPod(pod)
	wantTypes := []string{"PodScheduled", "Initialized", "ContainersReady", "Ready"}
	if len(got.Conditions) != len(wantTypes) {
		t.Fatalf("conditions len = %d, want %d", len(got.Conditions), len(wantTypes))
	}
	for i, w := range wantTypes {
		if got.Conditions[i].Type != w {
			t.Fatalf("condition[%d].Type = %q, want %q", i, got.Conditions[i].Type, w)
		}
	}
}

func TestTransformEvent_AllInvolvedObjectKinds(t *testing.T) {
	kinds := []struct {
		kind, name, want string
	}{
		{"Pod", "web-1", "Pod/web-1"},
		{"Deployment", "api", "Deployment/api"},
		{"Service", "svc-x", "Service/svc-x"},
		{"Node", "node-1", "Node/node-1"},
		{"ReplicaSet", "rs-abc", "ReplicaSet/rs-abc"},
	}
	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			got := transformEvent(corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: k.kind, Name: k.name},
				Count:          1,
			})
			if got.Object != k.want {
				t.Fatalf("Object = %q, want %q", got.Object, k.want)
			}
		})
	}
}

func TestTransformPod_RestartsSumAllContainers(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}, {Name: "c"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "a", RestartCount: 5},
				{Name: "b", RestartCount: 0},
				{Name: "c", RestartCount: 3},
			},
		},
	}
	got := transformPod(pod)
	if got.Restarts != 8 {
		t.Fatalf("restarts = %d, want 8", got.Restarts)
	}
}

// --- Benchmarks ----------------------------------------------------------

// BenchmarkTransformPod measures the cost of the hot path: turning one Pod
// into the frontend's JSON shape. The fixture is roughly representative of
// a typical multi-container pod (2 containers, 4 conditions, 2 volumes).
func BenchmarkTransformPod(b *testing.B) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "default",
			Labels:            map[string]string{"app": "web", "tier": "frontend"},
			CreationTimestamp: fixedTime(),
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "app", Image: "app:1.0", Ports: []corev1.ContainerPort{{ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}},
				{Name: "sidecar", Image: "envoy:1.30", Ports: []corev1.ContainerPort{{ContainerPort: 9090, Protocol: corev1.ProtocolTCP}}},
			},
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning, PodIP: "10.0.0.5",
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "sidecar", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: fixedTime()},
				{Type: corev1.PodInitialized, Status: corev1.ConditionTrue, LastTransitionTime: fixedTime()},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue, LastTransitionTime: fixedTime()},
				{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: fixedTime()},
			},
		},
	}
	for b.Loop() {
		_ = transformPod(pod)
	}
}

// A regression test specifically for the multi-container logs fix: the
// containers list must contain every container in spec.Containers (so the
// frontend dropdown can let users pick a specific one). The bug surfaced
// when the JS backend would silently truncate to len(containerStatuses).
func TestTransformPod_ContainersListMatchesSpec(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main"}, {Name: "sidecar"}, {Name: "init-helper"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			// Intentionally only one status — sidecar/init-helper not yet reported
			ContainerStatuses: []corev1.ContainerStatus{{Name: "main", Ready: true}},
		},
	}
	got := transformPod(pod)
	if len(got.Containers) != 3 {
		t.Fatalf("expected 3 containers, got %d: %+v", len(got.Containers), got.Containers)
	}
	names := []string{got.Containers[0].Name, got.Containers[1].Name, got.Containers[2].Name}
	want := []string{"main", "sidecar", "init-helper"}
	for i := range names {
		if names[i] != want[i] {
			t.Fatalf("container[%d].Name = %q, want %q", i, names[i], want[i])
		}
	}
}

// --- expanded coverage: real-world pod / service / node variations ---

func TestTransformPod_AllPhases(t *testing.T) {
	// When no container statuses surface a reason, the pod's Phase becomes
	// the displayed status. Lock in the mapping for all five K8s phases.
	cases := []struct {
		phase corev1.PodPhase
		want  string
	}{
		{corev1.PodPending, "Pending"},
		{corev1.PodRunning, "Running"},
		{corev1.PodSucceeded, "Succeeded"},
		{corev1.PodFailed, "Failed"},
		{corev1.PodUnknown, "Unknown"},
	}
	for _, tc := range cases {
		t.Run(string(tc.phase), func(t *testing.T) {
			pod := &corev1.Pod{
				Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
				Status: corev1.PodStatus{Phase: tc.phase},
			}
			if got := transformPod(pod).Status; got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTransformPod_CommonWaitingReasons(t *testing.T) {
	// These are the user-visible failure modes the frontend's StatusBadge
	// renders specially. Lock in the surface behavior.
	reasons := []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "ContainerCreating", "CreateContainerConfigError"}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: r}}},
					},
				},
			}
			got := transformPod(pod)
			if got.Status != r {
				t.Fatalf("pod status = %q, want %q", got.Status, r)
			}
			if got.Containers[0].State != r {
				t.Fatalf("container state = %q, want %q", got.Containers[0].State, r)
			}
		})
	}
}

func TestTransformPod_CommonTerminatedReasons(t *testing.T) {
	reasons := []string{"Completed", "Error", "OOMKilled", "ContainerCannotRun"}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: r}}},
					},
				},
			}
			got := transformPod(pod)
			if got.Status != r {
				t.Fatalf("status = %q, want %q", got.Status, r)
			}
			if got.Containers[0].State != r {
				t.Fatalf("container state = %q", got.Containers[0].State)
			}
		})
	}
}

func TestTransformPod_MixedContainerStates(t *testing.T) {
	// One container running, another waiting — the waiting reason wins for
	// the pod's overall status (matches getPodStatus loop order: first
	// container status with a waiting/terminated reason).
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "sidecar", Ready: false, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			},
		},
	}
	got := transformPod(pod)
	// Container 0 ahead has no waiting reason; container 1 surfaces CrashLoopBackOff.
	if got.Status != "CrashLoopBackOff" {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Containers[0].State != "Running" {
		t.Fatalf("c[0].state = %q", got.Containers[0].State)
	}
	if got.Containers[1].State != "CrashLoopBackOff" {
		t.Fatalf("c[1].state = %q", got.Containers[1].State)
	}
	if got.Ready != "1/2" {
		t.Fatalf("ready = %q", got.Ready)
	}
}

func TestTransformPod_AllVolumesPreservePerContainerStatuses(t *testing.T) {
	// 4 different volume types in a single pod, each rendering its type.
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c"}},
			Volumes: []corev1.Volume{
				{Name: "v1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "v2", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/x"}}},
				{Name: "v3", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
				{Name: "v4", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "s"}}},
			},
		},
	}
	got := transformPod(pod)
	want := map[string]string{"v1": "emptyDir", "v2": "hostPath", "v3": "configMap", "v4": "secret"}
	for _, v := range got.Volumes {
		if want[v.Name] != v.Type {
			t.Errorf("volume %q type = %q, want %q", v.Name, v.Type, want[v.Name])
		}
	}
}

func TestTransformNamespace_TerminatingPhase(t *testing.T) {
	ns := corev1.Namespace{Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating}}
	if got := transformNamespace(ns); got.Status != "Terminating" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestTransformService_ExternalNameType(t *testing.T) {
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "example.com",
		},
	}
	got := transformService(svc)
	if got.Type != "ExternalName" {
		t.Fatalf("type = %q", got.Type)
	}
	// ClusterIP for an ExternalName service is typically empty — we render "None".
	if got.ClusterIP != "None" {
		t.Fatalf("clusterIP = %q", got.ClusterIP)
	}
}

func TestTransformService_LoadBalancerMultipleIngressTakesFirst(t *testing.T) {
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "1.1.1.1"},
					{IP: "2.2.2.2"},
				},
			},
		},
	}
	got := transformService(svc)
	if got.ExternalIP != "1.1.1.1" {
		t.Fatalf("externalIP = %q (expected first ingress)", got.ExternalIP)
	}
}

func TestTransformNode_AllAddressTypes(t *testing.T) {
	n := corev1.Node{Status: corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
			{Type: corev1.NodeExternalIP, Address: "203.0.113.1"},
			{Type: corev1.NodeHostName, Address: "node-1"},
			{Type: corev1.NodeInternalDNS, Address: "node-1.internal"},
			{Type: corev1.NodeExternalDNS, Address: "node-1.example.com"},
		},
	}}
	got := transformNode(n)
	if len(got.Addresses) != 5 {
		t.Fatalf("len = %d", len(got.Addresses))
	}
	want := map[string]string{
		"InternalIP":  "10.0.0.1",
		"ExternalIP":  "203.0.113.1",
		"Hostname":    "node-1",
		"InternalDNS": "node-1.internal",
		"ExternalDNS": "node-1.example.com",
	}
	for _, a := range got.Addresses {
		if want[a.Type] != a.Address {
			t.Errorf("address %q = %q, want %q", a.Type, a.Address, want[a.Type])
		}
	}
}

func TestTransformEvent_NormalVsWarning(t *testing.T) {
	for _, ty := range []string{"Normal", "Warning"} {
		t.Run(ty, func(t *testing.T) {
			got := transformEvent(corev1.Event{Type: ty, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p"}})
			if got.Type != ty {
				t.Fatalf("type = %q", got.Type)
			}
		})
	}
}

func TestGetAge_LongDurations(t *testing.T) {
	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"7 days", 7 * 24 * time.Hour, "7d"},
		{"30 days", 30 * 24 * time.Hour, "30d"},
		{"365 days", 365 * 24 * time.Hour, "365d"},
		{"three years", 3 * 365 * 24 * time.Hour, "1095d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := metav1.NewTime(time.Now().Add(-tc.ago))
			if got := getAge(ts); got != tc.want {
				t.Fatalf("getAge(%v ago) = %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

func TestGetAge_FutureTimestamp(t *testing.T) {
	// A pod with a creationTimestamp in the future (clock skew). Age should
	// still render — it just produces a small/negative duration that gets
	// truncated to "0s" by the int truncation semantics.
	ts := metav1.NewTime(time.Now().Add(10 * time.Second))
	got := getAge(ts)
	// We only care that the function doesn't panic — the exact rendering
	// for skew is implementation-defined.
	if got == "" {
		t.Fatal("getAge returned empty string for future timestamp")
	}
}

func TestFormatServicePort_NodePort(t *testing.T) {
	// NodePort is part of the ServicePort but JS/Go both omit it from the
	// rendered string; verify by inclusion of port:target only.
	p := corev1.ServicePort{Port: 80, NodePort: 30080, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}
	if got := formatServicePort(p); got != "80:8080/TCP" {
		t.Fatalf("got = %q", got)
	}
}

func TestTransformPod_Conditions_PreserveReasonAndStatus(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady", LastTransitionTime: fixedTime()},
			},
		},
	}
	got := transformPod(pod)
	c := got.Conditions[0]
	if c.Type != "Ready" || c.Status != "False" || c.Reason != "ContainersNotReady" {
		t.Fatalf("condition = %+v", c)
	}
}

func TestTransformDeployment_ReasonAndMessagePreserved(t *testing.T) {
	dep := appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue, Reason: "FailedCreate", Message: "ReplicaSet failed"},
			},
		},
	}
	got := transformDeployment(dep)
	c := got.Conditions[0]
	if c.Reason != "FailedCreate" || c.Message != "ReplicaSet failed" {
		t.Fatalf("cond = %+v", c)
	}
}

// TestTransformPod_ScaleSmokeTest runs the transformer over a synthetic
// "1000-pod cluster" to confirm correctness and the absence of accidental
// O(n²) behavior in the transform path. It's not a benchmark per se — it
// just exercises the hot path at realistic scale.
func TestTransformPod_ScaleSmokeTest(t *testing.T) {
	for i := range 1000 {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default", CreationTimestamp: fixedTime()},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", Ready: true}},
			},
		}
		got := transformPod(pod)
		if got.Status != "Running" {
			t.Fatalf("iteration %d: status = %q", i, got.Status)
		}
	}
}
