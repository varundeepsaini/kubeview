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

const (
	ttN1           = 1
	ttN10          = 10
	ttN100         = 100
	ttN1000        = 1000
	ttN15          = 15
	ttN2           = 2
	ttN2024        = 2024
	ttN23          = 23
	ttN24          = 24
	ttN3           = 3
	ttN30          = 30
	ttN30080       = 30080
	ttN34          = 34
	ttN365         = 365
	ttN4           = 4
	ttN443         = 443
	ttN5           = 5
	ttN50          = 50
	ttN53          = 53
	ttN59          = 59
	ttN60          = 60
	ttN7           = 7
	ttN8           = 8
	ttN80          = 80
	ttN8080        = 8080
	ttN9090        = 9090
	ttSA           = "a"
	ttSB           = "b"
	ttSC           = "c"
	ttSConditions  = `"conditions":[]`
	ttSCountD      = "count = %d"
	ttSEmptyDir    = "emptyDir"
	ttSExternalIPQ = "externalIP = %q"
	ttSGotQ        = "got %q"
	ttSHostPath    = "hostPath"
	ttSImages      = `"images":[]`
	ttSLabels      = `"labels":{}`
	ttSMain        = "main"
	ttSN0          = "0"
	ttSN110        = "110"
	ttSNginx       = "nginx"
	ttSNode1       = "node-1"
	ttSOk          = "ok"
	ttSP           = "p"
	ttSRolesV      = "roles = %v"
	ttSS           = "s"
	ttSSecret      = "secret"
	ttSSelector    = `"selector":{}`
	ttSSidecar     = "sidecar"
	ttSStatusQ     = "status = %q"
	ttSTypeQ       = "type = %q"
	ttSWeb         = "web"
	ttSX           = "x"
)

// Shared literals used across the transformer tests. Prefixed with tt to avoid
// colliding with constants other test files in the package may define.
const (
	ttFixedRFC3339          = "2024-01-02T03:04:05Z"
	ttCrashLoopBackOff      = "CrashLoopBackOff"
	ttError                 = "Error"
	ttImagePullBackOff      = "ImagePullBackOff"
	ttCompleted             = "Completed"
	ttPending               = "Pending"
	ttPort808080TCP         = "80:8080/TCP"
	ttActive                = "Active"
	ttPodIP                 = "10.0.0.5"
	ttNA                    = "N/A"
	ttClusterIPAddr         = "10.0.0.1"
	ttClusterIP             = "ClusterIP"
	ttExternalIPSpec        = "1.2.3.4"
	ttNone                  = "None"
	ttExternalIPAlt         = "5.6.7.8"
	ttNotReady              = "NotReady"
	ttRoleNone              = "<none>"
	ttNormal                = "Normal"
	ttUnknown               = "Unknown"
	ttWaiting               = "Waiting"
	ttRunning               = "Running"
	ttTerminating           = "Terminating"
	ttApp                   = "app"
	ttReady                 = "Ready"
	ttAPI                   = "api"
	ttAPIImage              = "api:1"
	ttRollingUpdate         = "RollingUpdate"
	ttWarning               = "Warning"
	ttPod                   = "Pod"
	ttZeroNum               = 0
	ttEmptyStr              = ""
	ttLblName               = "name"
	ttLblNamespace          = "namespace"
	ttUnknownLower          = "unknown"
	ttEnv                   = "env"
	ttSvc                   = "svc"
	ttRoleControlPlaneLabel = "node-role.kubernetes.io/control-plane"
	ttHostnameLabel         = "kubernetes.io/hostname"
	ttKubeVersion           = "v1.31.0"
	ttInitDB                = "init-db"
	ttEnvoyImage            = "envoy:1.30"
	ttConfigMap             = "configMap"
	ttDefault               = "default"
	ttDev                   = "dev"
	ttData                  = "data"
	ttConfig                = "config"
	ttArm64                 = "arm64"
	ttContainerd            = "containerd://2.0"
	ttScheduler             = "default-scheduler"
)

// fixedTime returns a deterministic timestamp used across tests. We never
// compare ages against wall-clock time directly — we compare the *shape* of
// the formatted string and the delta against a known reference.
func fixedTime() metav1.Time {
	return metav1.NewTime(
		time.Date(ttN2024, ttN1, ttN2, ttN3, ttN4, ttN5, ttZeroNum, time.UTC),
	)
}

// --- helper tests ---------------------------------------------------------

func TestFormatTime(t *testing.T) {
	t.Parallel()
	t.Run("zero time returns empty string", func(t *testing.T) {
		t.Parallel()

		got := formatTime(metav1.Time{
			Time: time.Time{},
		})
		wantEq(t, "formatTime(zero)", got, ttEmptyStr)
	})
	t.Run("non-zero formats as RFC3339 in UTC", func(t *testing.T) {
		t.Parallel()

		ts := metav1.NewTime(
			time.Date(
				ttN2024,
				ttN1,
				ttN2,
				ttN3,
				ttN4,
				ttN5,
				ttZeroNum,
				time.UTC,
			),
		)
		wantEq(t, "formatTime", formatTime(ts), ttFixedRFC3339)
	})
	t.Run("non-UTC input is converted to UTC", func(t *testing.T) {
		t.Parallel()

		loc, err := time.LoadLocation("Asia/Kolkata")
		if err != nil {
			t.Fatalf("LoadLocation: %v", err)
		}

		ts := metav1.NewTime(
			time.Date(ttN2024, ttN1, ttN2, ttN8, ttN34, ttN5, // +05:30
				ttZeroNum, loc),
		)
		wantEq(t, "formatTime UTC conversion", formatTime(ts), ttFixedRFC3339)
	})
}

func TestGetAge(t *testing.T) {
	t.Parallel()
	t.Run("zero time returns Unknown", func(t *testing.T) {
		t.Parallel()

		if got := getAge(metav1.Time{
			Time: time.Time{},
		}); got != ttUnknown {
			t.Fatalf("getAge(zero) = %q", got)
		}
	})

	cases := []struct {
		name string
		want string
		ago  time.Duration
	}{
		{name: "5 seconds", want: "5s", ago: ttN5 * time.Second},
		{name: "59 seconds", want: "59s", ago: ttN59 * time.Second},
		{
			name: "60 seconds rolls to minutes",
			want: "1m",
			ago:  ttN60 * time.Second,
		},
		{name: "15 minutes", want: "15m", ago: ttN15 * time.Minute},
		{name: "59 minutes", want: "59m", ago: ttN59 * time.Minute},
		{
			name: "60 minutes rolls to hours",
			want: "1h",
			ago:  ttN60 * time.Minute,
		},
		{name: "3 hours", want: "3h", ago: ttN3 * time.Hour},
		{name: "23 hours", want: "23h", ago: ttN23 * time.Hour},
		{name: "24 hours rolls to days", want: "1d", ago: ttN24 * time.Hour},
		{name: "5 days", want: "5d", ago: ttN5 * ttN24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := metav1.NewTime(time.Now().Add(-tc.ago))
			if got := getAge(ts); got != tc.want {
				t.Fatalf("getAge(%v ago) = %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

func TestEmptyIfNil(t *testing.T) {
	t.Parallel()
	t.Run("nil produces empty map", func(t *testing.T) {
		t.Parallel()

		got := emptyIfNil(nil)
		wantEq(t, "emptyIfNil(nil) is non-nil", got == nil, false)
		wantEq(t, "emptyIfNil(nil) length", len(got), ttZeroNum)
		// must serialize as {} not null
		b := mustMarshal(t, got)
		wantEq(t, "emptyIfNil(nil) json", string(b), "{}")
	})
	t.Run("non-nil is returned unchanged", func(t *testing.T) {
		t.Parallel()

		in := map[string]string{ttSA: "1"}

		got := emptyIfNil(in)
		wantEq(t, "emptyIfNil passthrough", got[ttSA], "1")
	})
}

func TestMaxInt32(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b, want int32
	}{
		{ttZeroNum, ttN1, ttN1},
		{ttN1, ttZeroNum, ttN1},
		{ttN5, ttN5, ttN5},
		{-ttN1, ttZeroNum, ttZeroNum},
		{ttN100, ttN50, ttN100},
	}
	for _, tc := range cases {
		if got := maxInt32(tc.a, tc.b); got != tc.want {
			t.Errorf("maxInt32(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestContainerState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *corev1.ContainerStatus
		want string
	}{
		{"nil pointer -> Waiting", nil, ttWaiting},
		{
			"running",
			ttBuild236(),
			ttRunning,
		},
		{
			"waiting with reason",
			ttBuild235(),
			ttCrashLoopBackOff,
		},
		{
			"waiting without reason",
			ttBuild234(),
			ttWaiting,
		},
		{
			"terminated with reason",
			ttBuild233(),
			ttError,
		},
		{
			"terminated without reason",
			ttBuild232(),
			"Terminated",
		},
		{"all states nil -> Unknown", ttBuild231(), ttUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := containerState(tc.in); got != tc.want {
				t.Fatalf("containerState = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPodStatus(t *testing.T) {
	t.Parallel()
	t.Run("deletion timestamp set -> Terminating", func(t *testing.T) {
		t.Parallel()

		now := metav1.Now()

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &now,
				Name:              ttEmptyStr,
				GenerateName:      ttEmptyStr,
				Namespace:         ttEmptyStr,
				SelfLink:          ttEmptyStr,
				UID:               ttEmptyStr,
				ResourceVersion:   ttEmptyStr,
				Generation:        ttZeroNum,
				CreationTimestamp: metav1.Time{
					Time: time.Time{},
				},
				DeletionGracePeriodSeconds: nil,
				Labels:                     nil,
				Annotations:                nil,
				OwnerReferences:            nil,
				Finalizers:                 nil,
				ManagedFields:              nil,
			},
			Status: ttBuild230(),
			TypeMeta: metav1.TypeMeta{
				Kind:       ttEmptyStr,
				APIVersion: ttEmptyStr,
			},
			Spec: ttBuild229(),
		}
		wantEq(t, ttSGotQ, podStatus(pod), ttTerminating)
	})
	t.Run("waiting reason wins over phase", func(t *testing.T) {
		t.Parallel()

		pod := ttBuild360()
		wantEq(t, ttSGotQ, podStatus(pod), ttImagePullBackOff)
	})
	t.Run("terminated reason wins over phase", func(t *testing.T) {
		t.Parallel()

		pod := ttBuild359()
		wantEq(t, ttSGotQ, podStatus(pod), ttCompleted)
	})
	t.Run("no container reasons -> phase", func(t *testing.T) {
		t.Parallel()

		pod := ttBuild318()
		wantEq(t, ttSGotQ, podStatus(pod), ttRunning)
	})
	t.Run("no phase, no container reasons -> Unknown", func(t *testing.T) {
		t.Parallel()

		pod := ttBuild317()
		wantEq(t, ttSGotQ, podStatus(pod), ttUnknown)
	})
	t.Run("waiting without reason is ignored", func(t *testing.T) {
		t.Parallel()

		pod := ttBuild358()
		wantEq(t, ttSGotQ, podStatus(pod), ttPending)
	})
}

// Common volume types — these should agree with the JS implementation
// (which uses Object.keys() on the JSON-shaped volume).
func TestVolumeType(t *testing.T) {
	t.Parallel()

	t.Run(ttSEmptyDir, func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild213()), ttSEmptyDir)
	})
	t.Run(ttSHostPath, func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild212()), ttSHostPath)
	})
	t.Run(ttSSecret, func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild211()), ttSSecret)
	})
	t.Run(ttConfigMap, func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild210()), ttConfigMap)
	})
	t.Run("persistentVolumeClaim", func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild209()), "persistentVolumeClaim")
	})
	t.Run("projected", func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild208()), "projected")
	})
	t.Run("downwardAPI", func(t *testing.T) {
		t.Parallel()
		// Acronym gotcha: the field's JSON tag is "downwardAPI" (not
		// "downwardapi"), so volumeType's tag-reflection must surface that
		// exact casing.
		wantEq(t, ttSGotQ, volumeType(ttBuild207()), "downwardAPI")
	})
	t.Run("empty VolumeSource -> unknown", func(t *testing.T) {
		t.Parallel()
		wantEq(t, ttSGotQ, volumeType(ttBuild206()), ttUnknownLower)
	})
}

// TestVolumeType_Acronyms locks in that volume types whose Go struct field
// uses acronym casing (NFS, ISCSI, RBD, CSI, FC, etc.) emit the lowercase
// JSON key the K8s API uses on the wire — same as the JS backend, which
// reads keys directly from the parsed JSON.
func TestVolumeType_Acronyms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		vs   corev1.VolumeSource
		want string
	}{
		{
			name: "NFS",
			vs:   ttBuild205(),
			want: "nfs",
		},
		{
			name: "ISCSI",
			vs:   ttBuild204(),
			want: "iscsi",
		},
		{
			name: "RBD",
			vs:   ttBuild203(),
			want: "rbd",
		},
		{
			name: "CSI",
			vs:   ttBuild202(),
			want: "csi",
		},
		{
			name: "FC",
			vs:   ttBuild201(),
			want: "fc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := volumeType(tc.vs); got != tc.want {
				t.Fatalf("volumeType = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatServicePort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		p    corev1.ServicePort
	}{
		{
			name: "int targetPort",
			p: corev1.ServicePort{
				Port:        ttN80,
				TargetPort:  intstr.FromInt(ttN8080),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			want: ttPort808080TCP,
		},
		{
			name: "string targetPort",
			p: corev1.ServicePort{
				Port:        ttN80,
				TargetPort:  intstr.FromString("http"),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			want: "80:http/TCP",
		},
		{
			name: "no targetPort (int zero + empty string)",
			p: corev1.ServicePort{
				Port: ttN80, TargetPort: intstr.IntOrString{
					Type:   ttZeroNum,
					IntVal: ttZeroNum,
					StrVal: ttEmptyStr,
				}, Protocol: corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			want: "80/TCP",
		},
		{
			name: "UDP protocol",
			p: corev1.ServicePort{
				Port:        ttN53,
				TargetPort:  intstr.FromInt(ttN53),
				Protocol:    corev1.ProtocolUDP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			want: "53:53/UDP",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := formatServicePort(tc.p); got != tc.want {
				t.Fatalf("formatServicePort = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- transformer tests ----------------------------------------------------

func TestTransformNamespace(t *testing.T) {
	t.Parallel()
	t.Run("active namespace with labels", func(t *testing.T) {
		t.Parallel()

		ns := ttBuild315()
		got := transformNamespace(ns)
		want := Namespace{
			Name:      ttDefault,
			Status:    ttActive,
			Labels:    map[string]string{ttEnv: ttDev},
			CreatedAt: ttFixedRFC3339,
			Age:       ttEmptyStr,
		}
		// age is wall-clock based; check it's non-empty and don't pin a value.
		if got.Age == ttEmptyStr || got.Age == ttUnknown {
			t.Fatalf("got.Age = %q, want a non-Unknown age", got.Age)
		}

		got.Age = ttEmptyStr
		wantEq(t, "namespace deep-equal", reflect.DeepEqual(got, want), true)
	})
	t.Run("missing phase -> Unknown", func(t *testing.T) {
		t.Parallel()

		got := transformNamespace(ttBuild314())
		wantEq(t, ttSStatusQ, got.Status, ttUnknown)
	})
	t.Run("nil labels serialize as {}", func(t *testing.T) {
		t.Parallel()

		b := mustMarshal(t, transformNamespace(ttBuild313()))
		assertContains(t, string(b), ttSLabels)
	})
	t.Run(
		"zero creation time -> empty createdAt and Unknown age",
		func(t *testing.T) {
			t.Parallel()

			got := transformNamespace(ttBuild312())
			wantEq(t, "CreatedAt", got.CreatedAt, ttEmptyStr)
			wantEq(t, "Age", got.Age, ttUnknown)
		},
	)
}

func TestTransformPod_RunningPodWithSingleContainer(t *testing.T) {
	t.Parallel()

	pod := ttBuild357()
	got := transformPod(pod)
	wantEq(t, ttLblName, got.Name, ttSWeb)
	wantEq(t, ttLblNamespace, got.Namespace, ttDefault)
	wantEq(t, "status", got.Status, ttRunning)
	wantEq(t, "ready", got.Ready, "1/1")
	wantEq(t, "restarts", got.Restarts, ttN2)
	wantEq(t, "node", got.Node, ttSNode1)
	wantEq(t, "ip", got.IP, ttPodIP)
	wantEq(t, "containers len", len(got.Containers), ttN1)

	c := got.Containers[ttZeroNum]
	wantEq(t, "container name", c.Name, ttSNginx)
	wantEq(t, "container image", c.Image, "nginx:1.27")
	wantEq(t, "container ready", c.Ready, true)
	wantEq(t, "container restartCount", c.RestartCount, ttN2)
	wantEq(t, "container state", c.State, ttRunning)
	wantEq(t, "ports len", len(c.Ports), ttN1)
	wantEq(t, "ports[0]", c.Ports[ttZeroNum], "80/TCP")
}

func TestTransformPod_PendingPodWithNoNodeNamePodIP(t *testing.T) {
	t.Parallel()

	pod := ttBuild356()

	got := transformPod(pod)
	if got.Node != ttPending {
		t.Fatalf("node = %q, want Pending", got.Node)
	}

	if got.IP != ttNA {
		t.Fatalf("ip = %q, want N/A", got.IP)
	}
}

func TestTransformPod_TotalCountFallsBackToSpecContainersWhenNoStatuses(
	t *testing.T,
) {
	t.Parallel()

	pod := ttBuild355()

	got := transformPod(pod)
	if got.Ready != "0/2" {
		t.Fatalf("ready = %q, want 0/2", got.Ready)
	}
}

func TestTransformPod_ReadyCountCountsOnlyContainersWithStatusReadyTrue(
	t *testing.T,
) {
	t.Parallel()

	pod := ttBuild354()

	got := transformPod(pod)
	if got.Ready != "2/3" {
		t.Fatalf("ready = %q", got.Ready)
	}

	if got.Restarts != ttN4 {
		t.Fatalf("restarts = %d", got.Restarts)
	}
}

func TestTransformPod_TerminatingPodSurfacesTerminatingStatus(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: &now,
			Name:              ttEmptyStr,
			GenerateName:      ttEmptyStr,
			Namespace:         ttEmptyStr,
			SelfLink:          ttEmptyStr,
			UID:               ttEmptyStr,
			ResourceVersion:   ttEmptyStr,
			Generation:        ttZeroNum,
			CreationTimestamp: metav1.Time{
				Time: time.Time{},
			},
			DeletionGracePeriodSeconds: nil,
			Labels:                     nil,
			Annotations:                nil,
			OwnerReferences:            nil,
			Finalizers:                 nil,
			ManagedFields:              nil,
		},
		Status: ttBuild179(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: ttBuild178(),
	}

	got := transformPod(pod)
	if got.Status != ttTerminating {
		t.Fatalf(ttSStatusQ, got.Status)
	}
}

func TestTransformPod_WaitingReasonSurfacesInStatus(t *testing.T) {
	t.Parallel()

	pod := ttBuild353()

	got := transformPod(pod)
	if got.Status != ttCrashLoopBackOff {
		t.Fatalf(ttSStatusQ, got.Status)
	}
	// containers[0].state should also reflect the waiting reason
	if got.Containers[ttZeroNum].State != ttCrashLoopBackOff {
		t.Fatalf("container state = %q", got.Containers[ttZeroNum].State)
	}
}

func TestTransformPod_PartialStatusReadyCount(
	t *testing.T,
) {
	t.Parallel(
	// Mirrors a partial-status race; the missing container is treated as
	// not-ready
	// and counted against the total.
	)

	pod := ttBuild352()
	got := transformPod(pod)
	// totalCount comes from len(ContainerStatuses) when nonzero (matches JS);
	// that's 1 here.
	if got.Ready != "1/1" {
		t.Fatalf(
			"ready = %q (matches JS: total uses status length when present)",
			got.Ready,
		)
	}
	// But containers list comes from spec.Containers (2 of them).
	if len(got.Containers) != ttN2 {
		t.Fatalf("containers len = %d", len(got.Containers))
	}
	// Second container has no matching status -> ready=false, state=Waiting
	if got.Containers[ttN1].Ready {
		t.Fatal("container b should be not-ready")
	}

	if got.Containers[ttN1].State != ttWaiting {
		t.Fatalf("container b state = %q", got.Containers[ttN1].State)
	}
}

func TestTransformPod_ContainerStatusesMatchedByNameNotIndex(t *testing.T) {
	t.Parallel(
	// The API server does not guarantee status.containerStatuses is ordered
	// like spec.containers; statuses must be matched by container name.
	)

	statusA := ttBuild172() // name "a", ready, zero restarts
	statusA.State = corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{
			StartedAt: metav1.Time{
				Time: time.Time{},
			},
		},
		Waiting:    nil,
		Terminated: nil,
	}

	statusB := ttBuild172()
	statusB.Name = ttSB
	statusB.Ready = false
	statusB.RestartCount = ttN2
	statusB.State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{
			Reason:  ttEmptyStr,
			Message: ttEmptyStr,
		},
		Running:    nil,
		Terminated: nil,
	}

	pod := ttBuild352() // spec containers ordered [a, b]
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{statusB, statusA}

	got := transformPod(pod)
	wantEq(t, "containers len", len(got.Containers), ttN2)

	first := got.Containers[ttZeroNum]
	wantEq(t, "containers[0] name", first.Name, ttSA)
	wantEq(t, "containers[0] ready", first.Ready, true)
	wantEq(t, "containers[0] restartCount", first.RestartCount, ttZeroNum)
	wantEq(t, "containers[0] state", first.State, ttRunning)

	second := got.Containers[ttN1]
	wantEq(t, "containers[1] name", second.Name, ttSB)
	wantEq(t, "containers[1] ready", second.Ready, false)
	wantEq(t, "containers[1] restartCount", second.RestartCount, ttN2)
	wantEq(t, "containers[1] state", second.State, ttWaiting)
}

func TestTransformPod_ConditionsAndVolumesArePreserved(t *testing.T) {
	t.Parallel()

	condTime := fixedTime()
	pod := &corev1.Pod{
		ObjectMeta: ttBuild170(),
		Spec:       ttBuild301(),
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					Reason:             ttSOk,
					LastTransitionTime: condTime,
					ObservedGeneration: ttZeroNum,
					LastProbeTime: metav1.Time{
						Time: time.Time{},
					},
					Message: ttEmptyStr,
				},
			},
			ObservedGeneration:                   ttZeroNum,
			Message:                              ttEmptyStr,
			Reason:                               ttEmptyStr,
			NominatedNodeName:                    ttEmptyStr,
			HostIP:                               ttEmptyStr,
			HostIPs:                              nil,
			PodIP:                                ttEmptyStr,
			PodIPs:                               nil,
			StartTime:                            nil,
			InitContainerStatuses:                nil,
			ContainerStatuses:                    nil,
			QOSClass:                             ttEmptyStr,
			EphemeralContainerStatuses:           nil,
			Resize:                               ttEmptyStr,
			ResourceClaimStatuses:                nil,
			ExtendedResourceClaimStatus:          nil,
			AllocatedResources:                   nil,
			Resources:                            nil,
			NodeAllocatableResourceClaimStatuses: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}

	got := transformPod(pod)
	if len(got.Conditions) != ttN1 ||
		got.Conditions[ttZeroNum].Type != ttReady ||
		got.Conditions[ttZeroNum].Status != "True" ||
		got.Conditions[ttZeroNum].LastTransition != ttFixedRFC3339 {
		t.Fatalf("conditions = %+v", got.Conditions)
	}

	if len(got.Volumes) != ttN2 {
		t.Fatalf("volumes = %+v", got.Volumes)
	}

	if got.Volumes[ttZeroNum].Name != ttData ||
		got.Volumes[ttZeroNum].Type != ttSEmptyDir {
		t.Fatalf("volumes[0] = %+v", got.Volumes[ttZeroNum])
	}

	if got.Volumes[ttN1].Name != ttConfig ||
		got.Volumes[ttN1].Type != ttConfigMap {
		t.Fatalf("volumes[1] = %+v", got.Volumes[ttN1])
	}
}

func TestTransformPod_LabelsNilSerializesAs(t *testing.T) {
	t.Parallel()

	pod := ttBuild351()
	got := transformPod(pod)

	b := mustMarshal(t, got)
	if !strings.Contains(string(b), ttSLabels) {
		t.Fatalf("expected labels:{}, got: %s", b)
	}
	// containers, conditions, volumes should all serialize as [] not null
	keys := []string{`"containers":[`, ttSConditions, `"volumes":[]`}
	for _, key := range keys {
		if !strings.Contains(string(b), key) {
			t.Fatalf("expected %q in JSON, got: %s", key, b)
		}
	}
}

func TestTransformDeployment_HappyPath(t *testing.T) {
	t.Parallel()

	replicas := int32(ttN3)
	dep := appsv1.Deployment{
		ObjectMeta: ttBuild163(),
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type:          appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: nil,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels:      map[string]string{ttApp: ttAPI},
				MatchExpressions: nil,
			},
			Template: corev1.PodTemplateSpec{
				Spec:       ttBuild299(),
				ObjectMeta: ttBuild160(),
			},
			MinReadySeconds:         ttZeroNum,
			RevisionHistoryLimit:    nil,
			Paused:                  false,
			ProgressDeadlineSeconds: nil,
		},
		Status: ttBuild159(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
	got := transformDeployment(dep)
	wantEq(t, ttLblName, got.Name, ttAPI)
	wantEq(t, ttLblNamespace, got.Namespace, ttDefault)
	wantEq(t, "replicas", got.Replicas, ttN3)
	wantEq(t, "readyReplicas", got.ReadyReplicas, ttN2)
	wantEq(t, "desiredReplicas", got.DesiredReplicas, ttN3)
	wantEq(t, "updatedReplicas", got.UpdatedReplicas, ttN3)
	wantEq(t, "availableReplicas", got.AvailableReplicas, ttN2)
	wantEq(t, "strategy", got.Strategy, ttRollingUpdate)
	wantEq(t, "selector[app]", got.Selector[ttApp], ttAPI)
	wantEq(t, "images len", len(got.Images), ttN2)
	wantEq(t, "images[0]", got.Images[ttZeroNum], ttAPIImage)
	wantEq(t, "images[1]", got.Images[ttN1], "envoy:2")
	wantEq(t, "conditions len", len(got.Conditions), ttN1)
	wantEq(t, "conditions[0].Type", got.Conditions[ttZeroNum].Type, "Available")
}

func TestTransformDeployment_NilSpecReplicasDefaultsDesiredReplicasTo0(
	t *testing.T,
) {
	t.Parallel()

	dep := ttBuild350()

	got := transformDeployment(dep)
	if got.DesiredReplicas != ttZeroNum {
		t.Fatalf("desired = %d", got.DesiredReplicas)
	}
}

func TestTransformDeployment_EmptyStrategyDefaultsToRollingUpdate(
	t *testing.T,
) {
	t.Parallel()

	dep := ttBuild349()

	got := transformDeployment(dep)
	if got.Strategy != ttRollingUpdate {
		t.Fatalf("strategy = %q", got.Strategy)
	}
}

func TestTransformDeployment_NilSelectorSerializesAs(t *testing.T) {
	t.Parallel()

	dep := ttBuild348()
	got := transformDeployment(dep)

	b := mustMarshal(t, got)
	if !strings.Contains(string(b), ttSSelector) {
		t.Fatalf("expected selector:{} got: %s", b)
	}
}

func TestTransformDeployment_NilLabelsSerializeAs(t *testing.T) {
	t.Parallel()

	dep := ttBuild347()
	got := transformDeployment(dep)

	b := mustMarshal(t, got)
	if !strings.Contains(string(b), ttSLabels) {
		t.Fatalf("expected labels:{} got: %s", b)
	}
}

func TestTransformDeployment_EmptyConditionsImagesSerializeAs(t *testing.T) {
	t.Parallel()

	dep := ttBuild346()
	got := transformDeployment(dep)

	b := mustMarshal(t, got)
	for _, k := range []string{ttSConditions, ttSImages} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("expected %s in: %s", k, b)
		}
	}
}

func TestTransformService_ClusterIPWithPortAndTargetPort(t *testing.T) {
	t.Parallel()

	svc := ttBuild293()

	got := transformService(svc)
	if got.Type != ttClusterIP || got.ClusterIP != ttClusterIPAddr {
		t.Fatalf("type/ip = %+v", got)
	}

	if len(got.Ports) != ttN1 || got.Ports[ttZeroNum] != ttPort808080TCP {
		t.Fatalf("ports = %v", got.Ports)
	}

	if got.ExternalIP != ttNA {
		t.Fatalf(ttSExternalIPQ, got.ExternalIP)
	}
}

func TestTransformService_EmptyTypeDefaultsToClusterIP(t *testing.T) {
	t.Parallel()

	svc := ttBuild292()

	got := transformService(svc)
	if got.Type != ttClusterIP {
		t.Fatalf(ttSTypeQ, got.Type)
	}
}

func TestTransformService_EmptyClusterIPBecomesNoneHeadlessService(
	t *testing.T,
) {
	t.Parallel()

	svc := ttBuild291()

	got := transformService(svc)
	if got.ClusterIP != ttNone {
		t.Fatalf("clusterIP = %q", got.ClusterIP)
	}
}

func TestTransformService_LoadBalancerIngressIPWinsOverSpecExternalIPs(
	t *testing.T,
) {
	t.Parallel()

	svc := ttBuild290()

	got := transformService(svc)
	if got.ExternalIP != "9.10.11.12" {
		t.Fatalf(ttSExternalIPQ, got.ExternalIP)
	}
}

func TestTransformService_LBEmptyIngressIPFallsBack(
	t *testing.T,
) {
	t.Parallel()

	svc := ttBuild289()

	got := transformService(svc)
	if got.ExternalIP != ttExternalIPAlt {
		t.Fatalf(ttSExternalIPQ, got.ExternalIP)
	}
}

func TestTransformService_NoExternalIPAnywhereNA(t *testing.T) {
	t.Parallel()

	svc := ttBuild288()

	got := transformService(svc)
	if got.ExternalIP != ttNA {
		t.Fatalf(ttSExternalIPQ, got.ExternalIP)
	}
}

func TestTransformService_StringTargetPortRendersCorrectly(t *testing.T) {
	t.Parallel()

	svc := ttBuild287()

	got := transformService(svc)
	if got.Ports[ttZeroNum] != "80:http/TCP" {
		t.Fatalf("port = %q", got.Ports[ttZeroNum])
	}
}

func TestTransformNode_ReadyNodeWithRoles(t *testing.T) {
	t.Parallel()

	got := transformNode(ttBuild345())
	wantEq(t, ttLblName, got.Name, ttSNode1)
	wantEq(t, "status", got.Status, ttReady)
	wantEq(t, "version", got.Version, ttKubeVersion)
	wantEq(t, "os", got.OS, "Linux")
	wantEq(t, "arch", got.Arch, ttArm64)
	wantEq(t, "containerRuntime", got.ContainerRuntime, ttContainerd)
	wantEq(t, "cpu", got.CPU, "4")
	wantEq(t, "memory", got.Memory, "8Gi")
	wantEq(t, "pods", got.Pods, ttSN110)
	wantEq(t, "roles len", len(got.Roles), ttN1)
	wantEq(t, "roles[0]", got.Roles[ttZeroNum], "control-plane")
	wantEq(t, "conditions len", len(got.Conditions), ttN2)
	wantEq(t, "conditions[0].Type", got.Conditions[ttZeroNum].Type, ttReady)
	wantEq(t, "addresses len", len(got.Addresses), ttN2)
	wantEq(t, "addresses[0].Type", got.Addresses[ttZeroNum].Type, "InternalIP")
	wantEq(t, "addr[0].Addr", got.Addresses[ttZeroNum].Address, ttClusterIPAddr)
}

func TestTransformNode_NodeNotReadyWhenNoReadyTrueCondition(t *testing.T) {
	t.Parallel()

	n := ttBuild344()

	got := transformNode(n)
	if got.Status != ttNotReady {
		t.Fatalf(ttSStatusQ, got.Status)
	}
}

func TestTransformNode_NodeWithNoConditionsNotReady(t *testing.T) {
	t.Parallel()

	n := ttBuild343()

	got := transformNode(n)
	if got.Status != ttNotReady {
		t.Fatalf(ttSStatusQ, got.Status)
	}
}

func TestTransformNode_NodeWithNoRolesNone(t *testing.T) {
	t.Parallel()

	n := ttBuild342()

	got := transformNode(n)
	if len(got.Roles) != ttN1 || got.Roles[ttZeroNum] != ttRoleNone {
		t.Fatalf(ttSRolesV, got.Roles)
	}
}

func TestTransformNode_MultipleRoleLabelsExtractAll(t *testing.T) {
	t.Parallel()

	n := ttBuild341()

	got := transformNode(n)
	if len(got.Roles) != ttN2 {
		t.Fatalf(ttSRolesV, got.Roles)
	}
	// order is map-iteration-dependent — just check membership
	seen := map[string]bool{got.Roles[ttZeroNum]: true, got.Roles[ttN1]: true}
	if !seen["master"] || !seen["control-plane"] {
		t.Fatalf(
			"expected master and control-plane in roles, got %v",
			got.Roles,
		)
	}
}

func TestTransformNode_ZeroCapacitySurfaces0MatchesResourceQuantityZeroString(
	t *testing.T,
) {
	t.Parallel()

	n := ttBuild340()

	got := transformNode(n)
	if got.CPU != ttSN0 || got.Memory != ttSN0 || got.Pods != ttSN0 {
		t.Fatalf(
			"capacity not zero: cpu=%q mem=%q pods=%q",
			got.CPU,
			got.Memory,
			got.Pods,
		)
	}
}

func TestTransformNode_NilLabelsSerializeAs(t *testing.T) {
	t.Parallel()

	n := ttBuild339()
	got := transformNode(n)

	b := mustMarshal(t, got)
	if !strings.Contains(string(b), ttSLabels) {
		t.Fatalf("expected labels:{}, got: %s", b)
	}
}

func TestTransformEvent_HappyPath(t *testing.T) {
	t.Parallel()

	got := transformEvent(ttBuild279())
	wantEq(t, "type", got.Type, ttWarning)
	wantEq(t, "reason", got.Reason, "FailedScheduling")
	wantEq(t, "message", got.Message, "0/3 nodes available")
	wantEq(t, "object", got.Object, "Pod/web-1")
	wantEq(t, ttLblNamespace, got.Namespace, ttDefault)
	wantEq(t, "firstSeen", got.FirstSeen, ttFixedRFC3339)
	wantEq(t, "lastSeen", got.LastSeen, ttFixedRFC3339)
	wantEq(t, ttSCountD, got.Count, ttN7)
	wantEq(t, "source", got.Source, ttScheduler)
}

func TestTransformEvent_Count01MatchesJSEventCount1(t *testing.T) {
	t.Parallel()

	got := transformEvent(ttBuild278())
	if got.Count != ttN1 {
		t.Fatalf(ttSCountD, got.Count)
	}
}

func TestTransformEvent_Count1Stays1(t *testing.T) {
	t.Parallel()

	got := transformEvent(ttBuild277())
	if got.Count != ttN1 {
		t.Fatalf(ttSCountD, got.Count)
	}
}

func TestTransformEvent_Count2Stays2(t *testing.T) {
	t.Parallel()

	got := transformEvent(ttBuild276())
	if got.Count != ttN2 {
		t.Fatalf(ttSCountD, got.Count)
	}
}

func TestTransformEvent_ZeroTimestampsSerializeAsEmptyStrings(t *testing.T) {
	t.Parallel()

	got := transformEvent(ttBuild275())
	if got.FirstSeen != ttEmptyStr || got.LastSeen != ttEmptyStr {
		t.Fatalf("times wrong: %+v", got)
	}
}

// --- JSON-shape tests -----------------------------------------------------

// The frontend interfaces in kubeview-frontend/src/lib/api.ts treat
// Record<string, string> fields as non-nullable. Empty-but-defined maps must
// serialize as {} and empty-but-defined slices must serialize as []. Any nil
// would surface as `null` in JSON and the frontend would crash on `.map()`
// or property access. These tests guard against that regression.

func TestJSONNeverEmitsNullForCollectionFields(t *testing.T) {
	t.Parallel()
	t.Run("empty Pod marshals all collections as non-null", func(t *testing.T) {
		t.Parallel()

		pod := transformPod(ttBuild274())
		b := mustMarshal(t, pod)
		s := string(b)
		assertContains(t, s, ttSLabels)
		assertContains(t, s, `"containers":[]`)
		assertContains(t, s, ttSConditions)
		assertContains(t, s, `"volumes":[]`)
		assertNoNull(t, s)
	})
	t.Run(
		"empty Deployment marshals all collections as non-null",
		func(t *testing.T) {
			t.Parallel()

			dep := transformDeployment(ttBuild338())
			b := mustMarshal(t, dep)
			s := string(b)
			assertContains(t, s, ttSLabels)
			assertContains(t, s, ttSSelector)
			assertContains(t, s, ttSConditions)
			assertContains(t, s, ttSImages)
			assertNoNull(t, s)
		},
	)
	t.Run(
		"empty Service marshals all collections as non-null",
		func(t *testing.T) {
			t.Parallel()

			svc := transformService(ttBuild272())
			b := mustMarshal(t, svc)
			s := string(b)
			assertContains(t, s, ttSLabels)
			assertContains(t, s, ttSSelector)
			assertContains(t, s, `"ports":[]`)
			assertNoNull(t, s)
		},
	)
}

// TestJSONNeverEmitsNullForNodeAndNamespace continues the no-null-collection
// guarantees for Node and Namespace; split from the collections test above to
// stay within the per-function length budget.
func TestJSONNeverEmitsNullForNodeAndNamespace(t *testing.T) {
	t.Parallel()
	t.Run(
		"empty Node marshals all collections as non-null",
		func(t *testing.T) {
			t.Parallel()

			n := transformNode(ttBuild337())
			b := mustMarshal(t, n)
			s := string(b)
			assertContains(t, s, ttSLabels)
			assertContains(t, s, ttSConditions)
			assertContains(t, s, `"addresses":[]`)
			assertNoNull(t, s)
			// Parse back and verify roles defaults to ["<none>"]. Comparing the
			// raw JSON string is awkward because json.Marshal HTML-escapes <
			// and > — we check the decoded value instead, which is what the
			// frontend actually sees after JSON.parse().
			var parsed NodeInfo

			err := json.Unmarshal(b, &parsed)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if len(parsed.Roles) != ttN1 ||
				parsed.Roles[ttZeroNum] != ttRoleNone {
				t.Fatalf(ttSRolesV, parsed.Roles)
			}
		},
	)
	t.Run("empty Namespace marshals labels as non-null", func(t *testing.T) {
		t.Parallel()

		ns := transformNamespace(ttBuild270())
		b := mustMarshal(t, ns)
		s := string(b)
		assertContains(t, s, ttSLabels)
		assertNoNull(t, s)
	})
}

// wantEq fails the test when got != want. Keeping the comparison branch inside
// this helper (rather than inline ifs) keeps the calling test functions flat
// and within the project's per-function complexity budget.
func wantEq[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()

	if got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

// mustMarshal JSON-encodes v, failing the test on error. It returns the raw
// bytes so callers can both Contains-check the string form and Unmarshal it.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	return b
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()

	if !strings.Contains(s, sub) {
		t.Fatalf("expected JSON to contain %q\nfull: %s", sub, s)
	}
}

// assertNoNull guards the core JSON-shape invariant: a transformed object must
// never serialize any field as the literal null (the frontend would crash on
// it). The forbidden substring is fixed, so it is not parameterized.
func assertNoNull(t *testing.T, s string) {
	t.Helper()

	const forbidden = "null"
	if strings.Contains(s, forbidden) {
		t.Fatalf("expected JSON NOT to contain %q\nfull: %s", forbidden, s)
	}
}

// Sanity check that all our test fixtures compile with the frontend-required
// JSON tags. If anyone renames a field the test breaks loudly.
func TestPodJSONHasFrontendFields(t *testing.T) {
	t.Parallel()

	pod := transformPod(ttBuild336())
	b := mustMarshal(t, pod)
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
	t.Parallel(
	// The JS transformer only iterates pod.spec.containers, never
	// initContainers or ephemeralContainers. We mirror that: init/ephemeral
	// shouldn't leak into the frontend's containers list.
	)

	pod := ttBuild335()

	got := transformPod(pod)
	if len(got.Containers) != ttN1 || got.Containers[ttZeroNum].Name != ttApp {
		t.Fatalf("containers should be [app], got %+v", got.Containers)
	}
}

func TestTransformPod_HostIPAndPodIPEmpty(t *testing.T) {
	t.Parallel(
	// A pending pod with no PodIP yet should surface "N/A" — we do NOT fall
	// back to HostIP (matching JS behaviour, which only reads status.podIP).
	)

	pod := ttBuild334()

	got := transformPod(pod)
	if got.IP != ttNA {
		t.Fatalf("IP = %q, want N/A (HostIP must not be used)", got.IP)
	}
}

func TestTransformPod_VolumeWithNameFieldNotMisidentified(t *testing.T) {
	t.Parallel(
	// JS picks first non-"name" key. The Go implementation iterates Go fields
	// directly (which don't include a "name" field on VolumeSource), so this
	// test pins the behavior — adding a name-typed field to VolumeSource
	// upstream would not affect us.
	)

	pod := ttBuild333()

	got := transformPod(pod)
	if len(got.Volumes) != ttN1 || got.Volumes[ttZeroNum].Name != "my-volume" ||
		got.Volumes[ttZeroNum].Type != ttSSecret {
		t.Fatalf("volume = %+v", got.Volumes[ttZeroNum])
	}
}

func TestTransformService_NodePort(t *testing.T) {
	t.Parallel()

	svc := ttBuild263()

	got := transformService(svc)
	if got.Type != "NodePort" {
		t.Fatalf(ttSTypeQ, got.Type)
	}
	// The JS impl ignores nodePort in the port string and so do we — frontend
	// shows port:targetPort/proto. Locking in current behavior.
	if got.Ports[ttZeroNum] != ttPort808080TCP {
		t.Fatalf("port = %q", got.Ports[ttZeroNum])
	}
}

func TestTransformService_LoadBalancerHostnameOnly(t *testing.T) {
	t.Parallel(
	// Some cloud LBs return only a Hostname (no IP), e.g. AWS NLBs. JS would
	// fall through `ingress[0].ip` (undefined → falsy) and then to
	// spec.externalIPs, then to "N/A". We match: Hostname is not surfaced.
	)

	svc := ttBuild262()

	got := transformService(svc)
	if got.ExternalIP != ttNA {
		t.Fatalf(
			"ExternalIP = %q, want N/A (hostname must not be surfaced)",
			got.ExternalIP,
		)
	}
}

func TestTransformService_MultiplePorts(t *testing.T) {
	t.Parallel()

	svc := ttBuild261()
	got := transformService(svc)

	want := []string{ttPort808080TCP, "443:https/TCP", "53:53/UDP"}

	if !reflect.DeepEqual(got.Ports, want) {
		t.Fatalf("ports = %v, want %v", got.Ports, want)
	}
}

func TestTransformDeployment_RecreateStrategy(t *testing.T) {
	t.Parallel()

	dep := ttBuild332()

	got := transformDeployment(dep)
	if got.Strategy != "Recreate" {
		t.Fatalf("strategy = %q", got.Strategy)
	}
}

func TestTransformDeployment_EmptyTemplateContainers(t *testing.T) {
	t.Parallel()

	dep := ttBuild331()
	got := transformDeployment(dep)
	// Images should be [] (non-null) when template.spec.containers is empty.
	b := mustMarshal(t, got)
	if !strings.Contains(string(b), ttSImages) {
		t.Fatalf("expected images:[], got %s", b)
	}
}

func TestTransformNode_UsesCapacityNotAllocatable(t *testing.T) {
	t.Parallel(
	// JS reads node.status.capacity.{cpu,memory,pods}. Allocatable is
	// typically smaller (reserved system overhead subtracted). Confirm we
	// match by setting different values and asserting capacity wins.
	)

	n := ttBuild330()

	got := transformNode(n)
	if got.CPU != "8" || got.Memory != "16Gi" || got.Pods != ttSN110 {
		t.Fatalf(
			"expected capacity values, got cpu=%q mem=%q pods=%q",
			got.CPU,
			got.Memory,
			got.Pods,
		)
	}
}

// ttCondition builds a fully-specified PodCondition of the given type, used to
// keep the multi-condition fixture flat enough for the length budget.
func ttCondition(
	condType corev1.PodConditionType,
	now metav1.Time,
) corev1.PodCondition {
	return corev1.PodCondition{
		Type:               condType,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: now,
		ObservedGeneration: ttZeroNum,
		LastProbeTime:      metav1.Time{Time: time.Time{}},
		Reason:             ttEmptyStr,
		Message:            ttEmptyStr,
	}
}

// ttBuildMultiConditionPod builds a pod carrying the four standard K8s pod
// conditions, used by TestTransformPod_MultipleConditions.
func ttBuildMultiConditionPod() *corev1.Pod {
	now := fixedTime()

	return &corev1.Pod{
		Spec: ttBuild257(),
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				ttCondition(corev1.PodScheduled, now),
				ttCondition(corev1.PodInitialized, now),
				ttCondition(corev1.ContainersReady, now),
				ttCondition(corev1.PodReady, now),
			},
			ObservedGeneration:                   ttZeroNum,
			Message:                              ttEmptyStr,
			Reason:                               ttEmptyStr,
			NominatedNodeName:                    ttEmptyStr,
			HostIP:                               ttEmptyStr,
			HostIPs:                              nil,
			PodIP:                                ttEmptyStr,
			PodIPs:                               nil,
			StartTime:                            nil,
			InitContainerStatuses:                nil,
			ContainerStatuses:                    nil,
			QOSClass:                             ttEmptyStr,
			EphemeralContainerStatuses:           nil,
			Resize:                               ttEmptyStr,
			ResourceClaimStatuses:                nil,
			ExtendedResourceClaimStatus:          nil,
			AllocatedResources:                   nil,
			Resources:                            nil,
			NodeAllocatableResourceClaimStatuses: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild58(),
	}
}

func TestTransformPod_MultipleConditions(t *testing.T) {
	t.Parallel()
	// The full set of conditions K8s typically reports: PodScheduled,
	// Initialized, ContainersReady, Ready. All should come through in order.
	got := transformPod(ttBuildMultiConditionPod())

	wantTypes := []string{
		"PodScheduled",
		"Initialized",
		"ContainersReady",
		ttReady,
	}

	if len(got.Conditions) != len(wantTypes) {
		t.Fatalf(
			"conditions len = %d, want %d",
			len(got.Conditions),
			len(wantTypes),
		)
	}

	for i, w := range wantTypes {
		if got.Conditions[i].Type != w {
			t.Fatalf(
				"condition[%d].Type = %q, want %q",
				i,
				got.Conditions[i].Type,
				w,
			)
		}
	}
}

func TestTransformEvent_AllInvolvedObjectKinds(t *testing.T) {
	t.Parallel()

	kinds := []struct {
		kind, name, want string
	}{
		{ttPod, "web-1", "Pod/web-1"},
		{"Deployment", ttAPI, "Deployment/api"},
		{"Service", "svc-x", "Service/svc-x"},
		{"Node", ttSNode1, "Node/node-1"},
		{"ReplicaSet", "rs-abc", "ReplicaSet/rs-abc"},
	}
	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()

			got := transformEvent(corev1.Event{
				InvolvedObject: corev1.ObjectReference{
					Kind: k.kind, Name: k.name,
					Namespace:       ttEmptyStr,
					UID:             ttEmptyStr,
					APIVersion:      ttEmptyStr,
					ResourceVersion: ttEmptyStr,
					FieldPath:       ttEmptyStr,
				},
				Count: ttN1,
				TypeMeta: metav1.TypeMeta{
					Kind:       ttEmptyStr,
					APIVersion: ttEmptyStr,
				},
				ObjectMeta: ttBuild57(),
				Reason:     ttEmptyStr,
				Message:    ttEmptyStr,
				Source: corev1.EventSource{
					Component: ttEmptyStr,
					Host:      ttEmptyStr,
				},
				FirstTimestamp: metav1.Time{
					Time: time.Time{},
				},
				LastTimestamp: metav1.Time{
					Time: time.Time{},
				},
				Type: ttEmptyStr,
				EventTime: metav1.MicroTime{
					Time: time.Time{},
				},
				Series:              nil,
				Action:              ttEmptyStr,
				Related:             nil,
				ReportingController: ttEmptyStr,
				ReportingInstance:   ttEmptyStr,
			})
			if got.Object != k.want {
				t.Fatalf("Object = %q, want %q", got.Object, k.want)
			}
		})
	}
}

func TestTransformPod_RestartsSumAllContainers(t *testing.T) {
	t.Parallel()

	pod := ttBuild329()

	got := transformPod(pod)
	if got.Restarts != ttN8 {
		t.Fatalf("restarts = %d, want 8", got.Restarts)
	}
}

// --- Benchmarks ----------------------------------------------------------

// BenchmarkTransformPod measures the cost of the hot path: turning one Pod
// into the frontend's JSON shape. The fixture is roughly representative of
// a typical multi-container pod (2 containers, 4 conditions, 2 volumes).
func BenchmarkTransformPod(b *testing.B) {
	pod := ttBuild328()
	for b.Loop() {
		_ = transformPod(pod)
	}
}

// A regression test specifically for the multi-container logs fix: the
// containers list must contain every container in spec.Containers (so the
// frontend dropdown can let users pick a specific one). The bug surfaced
// when the JS backend would silently truncate to len(containerStatuses).
func TestTransformPod_ContainersListMatchesSpec(t *testing.T) {
	t.Parallel()

	pod := ttBuild327()

	got := transformPod(pod)
	if len(got.Containers) != ttN3 {
		t.Fatalf(
			"expected 3 containers, got %d: %+v",
			len(got.Containers),
			got.Containers,
		)
	}

	names := []string{
		got.Containers[ttZeroNum].Name,
		got.Containers[ttN1].Name,
		got.Containers[ttN2].Name,
	}

	want := []string{ttSMain, ttSSidecar, "init-helper"}

	for i := range names {
		if names[i] != want[i] {
			t.Fatalf("container[%d].Name = %q, want %q", i, names[i], want[i])
		}
	}
}

// --- expanded coverage: real-world pod / service / node variations ---

func TestTransformPod_AllPhases(t *testing.T) {
	t.Parallel(
	// When no container statuses surface a reason, the pod's Phase becomes
	// the displayed status. Lock in the mapping for all five K8s phases.
	)

	cases := []struct {
		phase corev1.PodPhase
		want  string
	}{
		{corev1.PodPending, ttPending},
		{corev1.PodRunning, ttRunning},
		{corev1.PodSucceeded, "Succeeded"},
		{corev1.PodFailed, "Failed"},
		{corev1.PodUnknown, ttUnknown},
	}
	for _, tc := range cases {
		t.Run(string(tc.phase), func(t *testing.T) {
			t.Parallel()

			pod := &corev1.Pod{
				Spec: ttBuild250(),
				Status: corev1.PodStatus{
					Phase:                                tc.phase,
					ObservedGeneration:                   ttZeroNum,
					Conditions:                           nil,
					Message:                              ttEmptyStr,
					Reason:                               ttEmptyStr,
					NominatedNodeName:                    ttEmptyStr,
					HostIP:                               ttEmptyStr,
					HostIPs:                              nil,
					PodIP:                                ttEmptyStr,
					PodIPs:                               nil,
					StartTime:                            nil,
					InitContainerStatuses:                nil,
					ContainerStatuses:                    nil,
					QOSClass:                             ttEmptyStr,
					EphemeralContainerStatuses:           nil,
					Resize:                               ttEmptyStr,
					ResourceClaimStatuses:                nil,
					ExtendedResourceClaimStatus:          nil,
					AllocatedResources:                   nil,
					Resources:                            nil,
					NodeAllocatableResourceClaimStatuses: nil,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       ttEmptyStr,
					APIVersion: ttEmptyStr,
				},
				ObjectMeta: ttBuild36(),
			}
			if got := transformPod(pod).Status; got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

// These are the user-visible failure modes the frontend's StatusBadge
// renders specially. Lock in the surface behavior.
func TestTransformPod_CommonWaitingReasons(t *testing.T) {
	t.Parallel()

	reasons := []string{
		ttCrashLoopBackOff,
		ttImagePullBackOff,
		"ErrImagePull",
		"ContainerCreating",
		"CreateContainerConfigError",
	}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			t.Parallel()

			got := transformPod(ttBuildReasonPod2(r))
			wantEq(t, "pod status", got.Status, r)
			wantEq(t, "container state", got.Containers[ttZeroNum].State, r)
		})
	}
}

func TestTransformPod_CommonTerminatedReasons(t *testing.T) {
	t.Parallel()

	reasons := []string{ttCompleted, ttError, "OOMKilled", "ContainerCannotRun"}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			t.Parallel()

			got := transformPod(ttBuildReasonPod1(r))
			wantEq(t, "status", got.Status, r)
			wantEq(t, "container state", got.Containers[ttZeroNum].State, r)
		})
	}
}

func TestTransformPod_MixedContainerStates(t *testing.T) {
	t.Parallel(
	// One container running, another waiting — the waiting reason wins for
	// the pod's overall status (matches getPodStatus loop order: first
	// container status with a waiting/terminated reason).
	)

	pod := ttBuild326()
	got := transformPod(pod)
	// Container 0 ahead has no waiting reason; container 1 surfaces
	// CrashLoopBackOff.
	if got.Status != ttCrashLoopBackOff {
		t.Fatalf(ttSStatusQ, got.Status)
	}

	if got.Containers[ttZeroNum].State != ttRunning {
		t.Fatalf("c[0].state = %q", got.Containers[ttZeroNum].State)
	}

	if got.Containers[ttN1].State != ttCrashLoopBackOff {
		t.Fatalf("c[1].state = %q", got.Containers[ttN1].State)
	}

	if got.Ready != "1/2" {
		t.Fatalf("ready = %q", got.Ready)
	}
}

func TestTransformPod_AllVolumesPreservePerContainerStatuses(t *testing.T) {
	t.Parallel(
	// 4 different volume types in a single pod, each rendering its type.
	)

	pod := ttBuild325()
	got := transformPod(pod)

	want := map[string]string{
		"v1": ttSEmptyDir,
		"v2": ttSHostPath,
		"v3": ttConfigMap,
		"v4": ttSSecret,
	}

	for _, v := range got.Volumes {
		if want[v.Name] != v.Type {
			t.Errorf(
				"volume %q type = %q, want %q",
				v.Name,
				v.Type,
				want[v.Name],
			)
		}
	}
}

func TestTransformNamespace_TerminatingPhase(t *testing.T) {
	t.Parallel()

	ns := ttBuild244()
	if got := transformNamespace(ns); got.Status != ttTerminating {
		t.Fatalf(ttSStatusQ, got.Status)
	}
}

func TestTransformService_ExternalNameType(t *testing.T) {
	t.Parallel()

	svc := ttBuild243()

	got := transformService(svc)
	if got.Type != "ExternalName" {
		t.Fatalf(ttSTypeQ, got.Type)
	}
	// ClusterIP for an ExternalName service is typically empty — we render
	// "None".
	if got.ClusterIP != ttNone {
		t.Fatalf("clusterIP = %q", got.ClusterIP)
	}
}

func TestTransformService_LoadBalancerMultipleIngressTakesFirst(t *testing.T) {
	t.Parallel()

	svc := ttBuild242()

	got := transformService(svc)
	if got.ExternalIP != "1.1.1.1" {
		t.Fatalf("externalIP = %q (expected first ingress)", got.ExternalIP)
	}
}

func TestTransformNode_AllAddressTypes(t *testing.T) {
	t.Parallel()

	n := ttBuild324()

	got := transformNode(n)
	if len(got.Addresses) != ttN5 {
		t.Fatalf("len = %d", len(got.Addresses))
	}

	want := map[string]string{
		"InternalIP":  ttClusterIPAddr,
		"ExternalIP":  "203.0.113.1",
		"Hostname":    ttSNode1,
		"InternalDNS": "node-1.internal",
		"ExternalDNS": "node-1.example.com",
	}
	for _, a := range got.Addresses {
		if want[a.Type] != a.Address {
			t.Errorf(
				"address %q = %q, want %q",
				a.Type,
				a.Address,
				want[a.Type],
			)
		}
	}
}

func TestTransformEvent_NormalVsWarning(t *testing.T) {
	t.Parallel()

	for _, ty := range []string{ttNormal, ttWarning} {
		t.Run(ty, func(t *testing.T) {
			t.Parallel()

			got := transformEvent(
				corev1.Event{
					Type: ty, InvolvedObject: corev1.ObjectReference{
						Kind: ttPod, Name: ttSP,
						Namespace:       ttEmptyStr,
						UID:             ttEmptyStr,
						APIVersion:      ttEmptyStr,
						ResourceVersion: ttEmptyStr,
						FieldPath:       ttEmptyStr,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       ttEmptyStr,
						APIVersion: ttEmptyStr,
					},
					ObjectMeta: ttBuild11(),
					Reason:     ttEmptyStr,
					Message:    ttEmptyStr,
					Source: corev1.EventSource{
						Component: ttEmptyStr,
						Host:      ttEmptyStr,
					},
					FirstTimestamp: metav1.Time{
						Time: time.Time{},
					},
					LastTimestamp: metav1.Time{
						Time: time.Time{},
					},
					Count: ttZeroNum,
					EventTime: metav1.MicroTime{
						Time: time.Time{},
					},
					Series:              nil,
					Action:              ttEmptyStr,
					Related:             nil,
					ReportingController: ttEmptyStr,
					ReportingInstance:   ttEmptyStr,
				},
			)
			if got.Type != ty {
				t.Fatalf(ttSTypeQ, got.Type)
			}
		})
	}
}

func TestGetAge_LongDurations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		ago  time.Duration
	}{
		{name: "7 days", want: "7d", ago: ttN7 * ttN24 * time.Hour},
		{name: "30 days", want: "30d", ago: ttN30 * ttN24 * time.Hour},
		{name: "365 days", want: "365d", ago: ttN365 * ttN24 * time.Hour},
		{
			name: "three years",
			want: "1095d",
			ago:  ttN3 * ttN365 * ttN24 * time.Hour,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := metav1.NewTime(time.Now().Add(-tc.ago))
			if got := getAge(ts); got != tc.want {
				t.Fatalf("getAge(%v ago) = %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

func TestGetAge_FutureTimestamp(t *testing.T) {
	t.Parallel(
	// A pod with a creationTimestamp in the future (clock skew). Age should
	// still render — it just produces a small/negative duration that gets
	// truncated to "0s" by the int truncation semantics.
	)

	ts := metav1.NewTime(time.Now().Add(ttN10 * time.Second))
	got := getAge(ts)
	// We only care that the function doesn't panic — the exact rendering
	// for skew is implementation-defined.
	if got == ttEmptyStr {
		t.Fatal("getAge returned empty string for future timestamp")
	}
}

func TestFormatServicePort_NodePort(t *testing.T) {
	t.Parallel(
	// NodePort is part of the ServicePort but JS/Go both omit it from the
	// rendered string; verify by inclusion of port:target only.
	)

	p := corev1.ServicePort{
		Port:        ttN80,
		NodePort:    ttN30080,
		TargetPort:  intstr.FromInt(ttN8080),
		Protocol:    corev1.ProtocolTCP,
		Name:        ttEmptyStr,
		AppProtocol: nil,
	}
	if got := formatServicePort(p); got != ttPort808080TCP {
		t.Fatalf("got = %q", got)
	}
}

func TestTransformPod_Conditions_PreserveReasonAndStatus(t *testing.T) {
	t.Parallel()

	pod := ttBuild323()
	got := transformPod(pod)

	c := got.Conditions[ttZeroNum]
	if c.Type != ttReady || c.Status != "False" ||
		c.Reason != "ContainersNotReady" {
		t.Fatalf("condition = %+v", c)
	}
}

func TestTransformDeployment_ReasonAndMessagePreserved(t *testing.T) {
	t.Parallel()

	dep := ttBuild322()
	got := transformDeployment(dep)

	c := got.Conditions[ttZeroNum]
	if c.Reason != "FailedCreate" || c.Message != "ReplicaSet failed" {
		t.Fatalf("cond = %+v", c)
	}
}

// TestTransformPod_ScaleSmokeTest runs the transformer over a synthetic
// "1000-pod cluster" to confirm correctness and the absence of accidental
// O(n²) behavior in the transform path. It's not a benchmark per se — it
// just exercises the hot path at realistic scale.
func TestTransformPod_ScaleSmokeTest(t *testing.T) {
	t.Parallel()

	for i := range ttN1000 {
		pod := ttBuild321()

		got := transformPod(pod)
		if got.Status != ttRunning {
			t.Fatalf("iteration %d: status = %q", i, got.Status)
		}
	}
}

func ttBuild1() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSC, Ready: true,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild2() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild3() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSP, Namespace: ttDefault, CreationTimestamp: fixedTime(),
		GenerateName:               ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild4() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild5() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild6() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild7() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{
			{
				Type:    appsv1.DeploymentReplicaFailure,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedCreate",
				Message: "ReplicaSet failed",
				LastUpdateTime: metav1.Time{
					Time: time.Time{},
				},
				LastTransitionTime: metav1.Time{
					Time: time.Time{},
				},
			},
		},
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		CollisionCount:      nil,
	}
}

func ttBuild8() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild9() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		Conditions: []corev1.PodCondition{
			{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionFalse,
				Reason:             "ContainersNotReady",
				LastTransitionTime: fixedTime(),
				ObservedGeneration: ttZeroNum,
				LastProbeTime: metav1.Time{
					Time: time.Time{},
				},
				Message: ttEmptyStr,
			},
		},
		ObservedGeneration:                   ttZeroNum,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild10() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild11() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild12() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild13() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild14() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild15() corev1.ServiceStatus {
	return corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{
					IP:       "1.1.1.1",
					Hostname: ttEmptyStr,
					IPMode:   nil,
					Ports:    nil,
				},
				{
					IP:       "2.2.2.2",
					Hostname: ttEmptyStr,
					IPMode:   nil,
					Ports:    nil,
				},
			},
		},
		Conditions: nil,
	}
}

func ttBuild16() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeLoadBalancer,
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild17() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild18() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeExternalName,
		ExternalName:                  "example.com",
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild19() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild20() corev1.PodStatus {
	return corev1.PodStatus{
		ObservedGeneration:                   ttZeroNum,
		Phase:                                ttEmptyStr,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild21() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild22() corev1.VolumeSource {
	return corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName:  ttSS,
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild23() corev1.VolumeSource {
	return corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: ttEmptyStr,
			},
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild24() corev1.VolumeSource {
	return corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{
			Path: "/x",
			Type: nil,
		},
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild25() corev1.VolumeSource {
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium:    ttEmptyStr,
			SizeLimit: nil,
		},
		HostPath:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild26() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild27() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild28() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  ttSSidecar,
		Ready: false,
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttCrashLoopBackOff,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild29() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttApp, Ready: true, State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
			},
			Waiting:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild30() corev1.Container {
	return corev1.Container{
		Name:       ttSSidecar,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild31() corev1.Container {
	return corev1.Container{
		Name:       ttApp,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild32() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild33() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild34() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild35() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild36() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild37() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild38() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild39() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSMain, Ready: true,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild40() corev1.Container {
	return corev1.Container{
		Name:       "init-helper",
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild41() corev1.Container {
	return corev1.Container{
		Name:       ttSSidecar,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild42() corev1.Container {
	return corev1.Container{
		Name:       ttSMain,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild43() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         ttSSidecar,
		Ready:        true,
		RestartCount: ttZeroNum,
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
			},
			Waiting:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild44() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         ttApp,
		Ready:        true,
		RestartCount: ttZeroNum,
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
			},
			Waiting:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild45() corev1.VolumeSource {
	return corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: ttEmptyStr,
			},
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild46() corev1.VolumeSource {
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium:    ttEmptyStr,
			SizeLimit: nil,
		},
		HostPath:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild47() corev1.Container {
	return corev1.Container{
		Name:  ttSSidecar,
		Image: ttEnvoyImage,
		Ports: []corev1.ContainerPort{{
			ContainerPort: ttN9090, Protocol: corev1.ProtocolTCP,
			Name:     ttEmptyStr,
			HostPort: ttZeroNum,
			HostIP:   ttEmptyStr,
		}},
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild48() corev1.Container {
	return corev1.Container{
		Name:  ttApp,
		Image: "app:1.0",
		Ports: []corev1.ContainerPort{{
			ContainerPort: ttN8080, Protocol: corev1.ProtocolTCP,
			Name:     ttEmptyStr,
			HostPort: ttZeroNum,
			HostIP:   ttEmptyStr,
		}},
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild49() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSWeb, Namespace: ttDefault,
		Labels: map[string]string{
			ttApp:  ttSWeb,
			"tier": "frontend",
		},
		CreationTimestamp:          fixedTime(),
		GenerateName:               ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild50() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild51() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSC, RestartCount: ttN3,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild52() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSB, RestartCount: ttZeroNum,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild53() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSA, RestartCount: ttN5,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild54() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild55() corev1.Container {
	return corev1.Container{
		Name:       ttSB,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild56() corev1.Container {
	return corev1.Container{
		Name:       ttSA,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild57() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild58() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild59() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild60() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild61() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild62() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild63() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild64() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild65() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild66() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild67() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild68() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild69() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild70() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild71() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP,
		Ports: []corev1.ServicePort{
			{
				Port:        ttN80,
				TargetPort:  intstr.FromInt(ttN8080),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			{
				Port:        ttN443,
				TargetPort:  intstr.FromString("https"),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
			{
				Port:        ttN53,
				TargetPort:  intstr.FromInt(ttN53),
				Protocol:    corev1.ProtocolUDP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
		},
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild72() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild73() corev1.ServiceStatus {
	return corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{
				Hostname: "my-lb.example.com",
				IP:       ttEmptyStr,
				IPMode:   nil,
				Ports:    nil,
			}},
		},
		Conditions: nil,
	}
}

func ttBuild74() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeLoadBalancer,
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild75() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:      corev1.ServiceTypeNodePort,
		ClusterIP: ttClusterIPAddr,
		Ports: []corev1.ServicePort{
			{
				Port:        ttN80,
				NodePort:    ttN30080,
				TargetPort:  intstr.FromInt(ttN8080),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
			},
		},
		Selector:                      nil,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild76() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSvc,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild77() corev1.PodStatus {
	return corev1.PodStatus{
		ObservedGeneration:                   ttZeroNum,
		Phase:                                ttEmptyStr,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild78() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild79() corev1.VolumeSource {
	return corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName:  ttSS,
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild80() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild81() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild82() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodPending,
		HostIP:                               "192.168.1.1",
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
		// populated but we should ignore it
	}
}

func ttBuild83() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild84() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild85() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttApp, Ready: true,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild86() corev1.EphemeralContainer {
	return corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name: "debugger", Image: "debug:1",
			Command:    nil,
			Args:       nil,
			WorkingDir: ttEmptyStr,
			Ports:      nil,
			EnvFrom:    nil,
			Env:        nil,
			Resources: corev1.ResourceRequirements{
				Limits:   nil,
				Requests: nil,
				Claims:   nil,
			},
			ResizePolicy:             nil,
			RestartPolicy:            nil,
			RestartPolicyRules:       nil,
			VolumeMounts:             nil,
			VolumeDevices:            nil,
			LivenessProbe:            nil,
			ReadinessProbe:           nil,
			StartupProbe:             nil,
			Lifecycle:                nil,
			TerminationMessagePath:   ttEmptyStr,
			TerminationMessagePolicy: ttEmptyStr,
			ImagePullPolicy:          ttEmptyStr,
			SecurityContext:          nil,
			Stdin:                    false,
			StdinOnce:                false,
			TTY:                      false,
		},
		TargetContainerName: ttEmptyStr,
	}
}

func ttBuild87() corev1.Container {
	return corev1.Container{
		Name: ttInitDB, Image: "init:1",
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild88() corev1.Container {
	return corev1.Container{
		Name: ttApp, Image: "app:1",
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild89() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Ready: true,
		Name:  ttEmptyStr,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild90() corev1.Container {
	return corev1.Container{
		Name: ttSC, Image: "i",
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild91() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSP, Namespace: "n",
		GenerateName:    ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild92() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild93() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild94() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild95() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		Type:                          ttEmptyStr,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild96() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild97() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild98() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild99() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild100() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild101() corev1.PodStatus {
	return corev1.PodStatus{
		ObservedGeneration:                   ttZeroNum,
		Phase:                                ttEmptyStr,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild102() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild103() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild104() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild105() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild106() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild107() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild108() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: ttDefault, Name: "e1",
		GenerateName:    ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild109() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild110() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild111() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild112() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild113() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild114() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"node-role.kubernetes.io/master": ttEmptyStr,
			ttRoleControlPlaneLabel:          ttEmptyStr,
		},
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild115() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild116() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels:          map[string]string{"hostname": "foo"},
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild117() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild118() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild119() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild120() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		OSImage:                 ttEmptyStr,
		ContainerRuntimeVersion: ttEmptyStr,
		KubeletVersion:          ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Architecture:            ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild121() corev1.NodeSystemInfo {
	return corev1.NodeSystemInfo{
		KubeletVersion:          ttKubeVersion,
		OSImage:                 "Linux",
		Architecture:            ttArm64,
		ContainerRuntimeVersion: ttContainerd,
		MachineID:               ttEmptyStr,
		SystemUUID:              ttEmptyStr,
		BootID:                  ttEmptyStr,
		KernelVersion:           ttEmptyStr,
		KubeProxyVersion:        ttEmptyStr,
		OperatingSystem:         ttEmptyStr,
		Swap:                    nil,
	}
}

func ttBuild122() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSNode1,
		Labels: map[string]string{
			ttRoleControlPlaneLabel: ttEmptyStr,
			ttHostnameLabel:         ttSNode1,
		},
		CreationTimestamp:          fixedTime(),
		GenerateName:               ttEmptyStr,
		Namespace:                  ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild123() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild124() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{
				Port:        ttN80,
				TargetPort:  intstr.FromString("http"),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
		},
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		Type:                          ttEmptyStr,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild125() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild126() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeClusterIP,
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild127() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild128() corev1.ServiceStatus {
	return corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{
				Hostname: "lb.example.com",
				IP:       ttEmptyStr,
				IPMode:   nil,
				Ports:    nil,
			}}, // IP empty
		},
		Conditions: nil,
	}
}

func ttBuild129() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeLoadBalancer,
		ExternalIPs:                   []string{ttExternalIPAlt},
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild130() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild131() corev1.ServiceStatus {
	return corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{
				IP:       "9.10.11.12",
				Hostname: ttEmptyStr,
				IPMode:   nil,
				Ports:    nil,
			}},
		},
		Conditions: nil,
	}
}

func ttBuild132() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeLoadBalancer,
		ExternalIPs:                   []string{ttExternalIPAlt},
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild133() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild134() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeClusterIP,
		Ports:                         nil,
		Selector:                      nil,
		ClusterIP:                     ttEmptyStr,
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild135() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild136() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		ClusterIP:                     ttExternalIPSpec,
		Ports:                         nil,
		Selector:                      nil,
		ClusterIPs:                    nil,
		Type:                          ttEmptyStr,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild137() corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:      corev1.ServiceTypeClusterIP,
		ClusterIP: ttClusterIPAddr,
		Ports: []corev1.ServicePort{
			{
				Port:        ttN80,
				TargetPort:  intstr.FromInt(ttN8080),
				Protocol:    corev1.ProtocolTCP,
				Name:        ttEmptyStr,
				AppProtocol: nil,
				NodePort:    ttZeroNum,
			},
		},
		Selector:                      map[string]string{ttApp: ttSWeb},
		ClusterIPs:                    nil,
		ExternalIPs:                   nil,
		SessionAffinity:               ttEmptyStr,
		LoadBalancerIP:                ttEmptyStr,
		LoadBalancerSourceRanges:      nil,
		ExternalName:                  ttEmptyStr,
		ExternalTrafficPolicy:         ttEmptyStr,
		HealthCheckNodePort:           ttZeroNum,
		PublishNotReadyAddresses:      false,
		SessionAffinityConfig:         nil,
		IPFamilies:                    nil,
		IPFamilyPolicy:                nil,
		AllocateLoadBalancerNodePorts: nil,
		LoadBalancerClass:             nil,
		InternalTrafficPolicy:         nil,
		TrafficDistribution:           nil,
	}
}

func ttBuild138() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSvc, Namespace: ttDefault,
		GenerateName:    ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild139() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild140() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild141() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild142() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild143() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild144() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild145() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild146() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild147() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild148() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild149() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild150() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild151() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild152() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild153() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild154() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild155() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		ObservedGeneration:  ttZeroNum,
		Replicas:            ttZeroNum,
		UpdatedReplicas:     ttZeroNum,
		ReadyReplicas:       ttZeroNum,
		AvailableReplicas:   ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		Conditions:          nil,
		CollisionCount:      nil,
	}
}

func ttBuild156() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild157() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild158() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild159() appsv1.DeploymentStatus {
	return appsv1.DeploymentStatus{
		Replicas:          ttN3,
		ReadyReplicas:     ttN2,
		UpdatedReplicas:   ttN3,
		AvailableReplicas: ttN2,
		Conditions: []appsv1.DeploymentCondition{
			{
				Type:               appsv1.DeploymentAvailable,
				Status:             corev1.ConditionTrue,
				Reason:             "MinimumReplicasAvailable",
				Message:            ttSOk,
				LastTransitionTime: fixedTime(),
				LastUpdateTime: metav1.Time{
					Time: time.Time{},
				},
			},
		},
		ObservedGeneration:  ttZeroNum,
		UnavailableReplicas: ttZeroNum,
		TerminatingReplicas: nil,
		CollisionCount:      nil,
	}
}

func ttBuild160() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild161() corev1.Container {
	return corev1.Container{
		Name: ttSSidecar, Image: "envoy:2",
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild162() corev1.Container {
	return corev1.Container{
		Name: ttAPI, Image: ttAPIImage,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild163() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttAPI, Namespace: ttDefault,
		Labels:                     map[string]string{ttApp: ttAPI},
		CreationTimestamp:          fixedTime(),
		GenerateName:               ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild164() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodRunning,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild165() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild166() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSP,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild167() corev1.VolumeSource {
	return corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: ttEmptyStr,
			},
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild168() corev1.VolumeSource {
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium:    ttEmptyStr,
			SizeLimit: nil,
		},
		HostPath:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild169() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild170() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSP,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild171() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild172() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSA, Ready: true,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild173() corev1.Container {
	return corev1.Container{
		Name:       ttSB,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild174() corev1.Container {
	return corev1.Container{
		Name:       ttSA,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild175() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild176() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttCrashLoopBackOff,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		Name: ttSC,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild177() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild178() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild179() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodRunning,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild180() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild181() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSC, Ready: true, RestartCount: ttN3,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild182() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSB, Ready: false, RestartCount: ttZeroNum,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild183() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: ttSA, Ready: true, RestartCount: ttN1,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild184() corev1.Container {
	return corev1.Container{
		Name:       ttSC,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild185() corev1.Container {
	return corev1.Container{
		Name:       ttSB,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild186() corev1.Container {
	return corev1.Container{
		Name:       ttSA,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild187() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild188() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodPending,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild189() corev1.Container {
	return corev1.Container{
		Name:       ttSB,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild190() corev1.Container {
	return corev1.Container{
		Name:       ttSA,
		Image:      ttEmptyStr,
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild191() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodPending,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild192() corev1.Container {
	return corev1.Container{
		Name: ttSC, Image: "img",
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		Ports:      nil,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild193() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSP, Namespace: "ns",
		GenerateName:    ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild194() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         ttSNginx,
		Ready:        true,
		RestartCount: ttN2,
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
			},
			Waiting:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild195() corev1.Container {
	return corev1.Container{
		Name:  ttSNginx,
		Image: "nginx:1.27",
		Ports: []corev1.ContainerPort{{
			ContainerPort: ttN80, Protocol: corev1.ProtocolTCP,
			Name:     ttEmptyStr,
			HostPort: ttZeroNum,
			HostIP:   ttEmptyStr,
		}},
		Command:    nil,
		Args:       nil,
		WorkingDir: ttEmptyStr,
		EnvFrom:    nil,
		Env:        nil,
		Resources: corev1.ResourceRequirements{
			Limits:   nil,
			Requests: nil,
			Claims:   nil,
		},
		ResizePolicy:             nil,
		RestartPolicy:            nil,
		RestartPolicyRules:       nil,
		VolumeMounts:             nil,
		VolumeDevices:            nil,
		LivenessProbe:            nil,
		ReadinessProbe:           nil,
		StartupProbe:             nil,
		Lifecycle:                nil,
		TerminationMessagePath:   ttEmptyStr,
		TerminationMessagePolicy: ttEmptyStr,
		ImagePullPolicy:          ttEmptyStr,
		SecurityContext:          nil,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func ttBuild196() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: ttSWeb, Namespace: ttDefault,
		Labels:                     map[string]string{ttApp: ttSWeb},
		CreationTimestamp:          fixedTime(),
		GenerateName:               ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild197() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSX,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild198() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSX,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild199() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttSX,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild200() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:                       ttDefault,
		Labels:                     map[string]string{ttEnv: ttDev},
		CreationTimestamp:          fixedTime(),
		GenerateName:               ttEmptyStr,
		Namespace:                  ttEmptyStr,
		SelfLink:                   ttEmptyStr,
		UID:                        ttEmptyStr,
		ResourceVersion:            ttEmptyStr,
		Generation:                 ttZeroNum,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild201() corev1.VolumeSource {
	return corev1.VolumeSource{
		FC: &corev1.FCVolumeSource{
			TargetWWNs: nil,
			Lun:        nil,
			FSType:     ttEmptyStr,
			ReadOnly:   false,
			WWIDs:      nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild202() corev1.VolumeSource {
	return corev1.VolumeSource{
		CSI: &corev1.CSIVolumeSource{
			Driver:               "d",
			ReadOnly:             nil,
			FSType:               nil,
			VolumeAttributes:     nil,
			NodePublishSecretRef: nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild203() corev1.VolumeSource {
	return corev1.VolumeSource{
		RBD: &corev1.RBDVolumeSource{
			CephMonitors: nil,
			RBDImage:     ttEmptyStr,
			FSType:       ttEmptyStr,
			RBDPool:      ttEmptyStr,
			RadosUser:    ttEmptyStr,
			Keyring:      ttEmptyStr,
			SecretRef:    nil,
			ReadOnly:     false,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild204() corev1.VolumeSource {
	return corev1.VolumeSource{
		ISCSI: &corev1.ISCSIVolumeSource{
			TargetPortal: ttSP, IQN: "i", Lun: ttZeroNum,
			ISCSIInterface:    ttEmptyStr,
			FSType:            ttEmptyStr,
			ReadOnly:          false,
			Portals:           nil,
			DiscoveryCHAPAuth: false,
			SessionCHAPAuth:   false,
			SecretRef:         nil,
			InitiatorName:     nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild205() corev1.VolumeSource {
	return corev1.VolumeSource{
		NFS: &corev1.NFSVolumeSource{
			Server: ttSS, Path: "/",
			ReadOnly: false,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild206() corev1.VolumeSource {
	return corev1.VolumeSource{
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild207() corev1.VolumeSource {
	return corev1.VolumeSource{
		DownwardAPI: &corev1.DownwardAPIVolumeSource{
			Items:       nil,
			DefaultMode: nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild208() corev1.VolumeSource {
	return corev1.VolumeSource{
		Projected: &corev1.ProjectedVolumeSource{
			Sources:     nil,
			DefaultMode: nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild209() corev1.VolumeSource {
	return corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "pvc",
			ReadOnly:  false,
		},
		HostPath:             nil,
		EmptyDir:             nil,
		GCEPersistentDisk:    nil,
		AWSElasticBlockStore: nil,
		GitRepo:              nil,
		Secret:               nil,
		NFS:                  nil,
		ISCSI:                nil,
		Glusterfs:            nil,
		RBD:                  nil,
		FlexVolume:           nil,
		Cinder:               nil,
		CephFS:               nil,
		Flocker:              nil,
		DownwardAPI:          nil,
		FC:                   nil,
		AzureFile:            nil,
		ConfigMap:            nil,
		VsphereVolume:        nil,
		Quobyte:              nil,
		AzureDisk:            nil,
		PhotonPersistentDisk: nil,
		Projected:            nil,
		PortworxVolume:       nil,
		ScaleIO:              nil,
		StorageOS:            nil,
		CSI:                  nil,
		Ephemeral:            nil,
		Image:                nil,
	}
}

func ttBuild210() corev1.VolumeSource {
	return corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: ttEmptyStr,
			},
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild211() corev1.VolumeSource {
	return corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName:  ttSS,
			Items:       nil,
			DefaultMode: nil,
			Optional:    nil,
		},
		HostPath:              nil,
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild212() corev1.VolumeSource {
	return corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{
			Path: "/data",
			Type: nil,
		},
		EmptyDir:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild213() corev1.VolumeSource {
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium:    ttEmptyStr,
			SizeLimit: nil,
		},
		HostPath:              nil,
		GCEPersistentDisk:     nil,
		AWSElasticBlockStore:  nil,
		GitRepo:               nil,
		Secret:                nil,
		NFS:                   nil,
		ISCSI:                 nil,
		Glusterfs:             nil,
		PersistentVolumeClaim: nil,
		RBD:                   nil,
		FlexVolume:            nil,
		Cinder:                nil,
		CephFS:                nil,
		Flocker:               nil,
		DownwardAPI:           nil,
		FC:                    nil,
		AzureFile:             nil,
		ConfigMap:             nil,
		VsphereVolume:         nil,
		Quobyte:               nil,
		AzureDisk:             nil,
		PhotonPersistentDisk:  nil,
		Projected:             nil,
		PortworxVolume:        nil,
		ScaleIO:               nil,
		StorageOS:             nil,
		CSI:                   nil,
		Ephemeral:             nil,
		Image:                 nil,
	}
}

func ttBuild214() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild215() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild216() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild217() corev1.PodStatus {
	return corev1.PodStatus{
		ObservedGeneration:                   ttZeroNum,
		Phase:                                ttEmptyStr,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild218() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild219() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild220() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild221() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild222() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodRunning,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild223() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild224() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild225() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason:   ttCompleted,
				ExitCode: ttZeroNum,
				Signal:   ttZeroNum,
				Message:  ttEmptyStr,
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
				FinishedAt: metav1.Time{
					Time: time.Time{},
				},
				ContainerID: ttEmptyStr,
			},
			Waiting: nil,
			Running: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild226() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild227() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ttEmptyStr,
		GenerateName:    ttEmptyStr,
		Namespace:       ttEmptyStr,
		SelfLink:        ttEmptyStr,
		UID:             ttEmptyStr,
		ResourceVersion: ttEmptyStr,
		Generation:      ttZeroNum,
		CreationTimestamp: metav1.Time{
			Time: time.Time{},
		},
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ttBuild228() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttImagePullBackOff,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild229() corev1.PodSpec {
	return corev1.PodSpec{
		Volumes:                       nil,
		InitContainers:                nil,
		Containers:                    nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild230() corev1.PodStatus {
	return corev1.PodStatus{
		Phase:                                corev1.PodRunning,
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		ContainerStatuses:                    nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild231() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		Name: ttEmptyStr,
		State: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild232() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: ttZeroNum,
				Signal:   ttZeroNum,
				Reason:   ttEmptyStr,
				Message:  ttEmptyStr,
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
				FinishedAt: metav1.Time{
					Time: time.Time{},
				},
				ContainerID: ttEmptyStr,
			},
			Waiting: nil,
			Running: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild233() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason:   ttError,
				ExitCode: ttZeroNum,
				Signal:   ttZeroNum,
				Message:  ttEmptyStr,
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
				FinishedAt: metav1.Time{
					Time: time.Time{},
				},
				ContainerID: ttEmptyStr,
			},
			Waiting: nil,
			Running: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild234() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild235() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  ttCrashLoopBackOff,
				Message: ttEmptyStr,
			},
			Running:    nil,
			Terminated: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild236() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Time{
					Time: time.Time{},
				},
			},
			Waiting:    nil,
			Terminated: nil,
		},
		Name: ttEmptyStr,
		LastTerminationState: corev1.ContainerState{
			Waiting:    nil,
			Running:    nil,
			Terminated: nil,
		},
		Ready:                    false,
		RestartCount:             ttZeroNum,
		Image:                    ttEmptyStr,
		ImageID:                  ttEmptyStr,
		ContainerID:              ttEmptyStr,
		Started:                  nil,
		AllocatedResources:       nil,
		Resources:                nil,
		VolumeMounts:             nil,
		User:                     nil,
		AllocatedResourcesStatus: nil,
		StopSignal:               nil,
	}
}

func ttBuild237() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild1(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild238() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild2()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild239() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild5(),
			Spec:       ttBuild4(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild240() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild10()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild241() corev1.NodeStatus {
	return corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: ttClusterIPAddr},
			{Type: corev1.NodeExternalIP, Address: "203.0.113.1"},
			{Type: corev1.NodeHostName, Address: ttSNode1},
			{Type: corev1.NodeInternalDNS, Address: "node-1.internal"},
			{Type: corev1.NodeExternalDNS, Address: "node-1.example.com"},
		},
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild13(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild242() corev1.Service {
	return corev1.Service{
		Spec:   ttBuild16(),
		Status: ttBuild15(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild14(),
	}
}

func ttBuild243() corev1.Service {
	return corev1.Service{
		Spec: ttBuild18(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild17(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild244() corev1.Namespace {
	return corev1.Namespace{
		Status: corev1.NamespaceStatus{
			Phase:      corev1.NamespaceTerminating,
			Conditions: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild19(),
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
	}
}

func ttBuild245() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{ttBuild26()},
		Volumes: []corev1.Volume{
			{Name: "v1", VolumeSource: ttBuild25()},
			{Name: "v2", VolumeSource: ttBuild24()},
			{Name: "v3", VolumeSource: ttBuild23()},
			{Name: "v4", VolumeSource: ttBuild22()},
		},
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild246() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild29(),
			ttBuild28(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild247() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild31(),
			ttBuild30(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild248() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild33()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild249() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild35()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild250() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild37()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild251() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		// Intentionally only one status — sidecar/init-helper not yet
		// reported
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild39(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild252() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild42(),
			ttBuild41(),
			ttBuild40(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild253() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning, PodIP: ttPodIP,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild44(),
			ttBuild43(),
		},
		Conditions: []corev1.PodCondition{
			{
				Type:               corev1.PodScheduled,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: fixedTime(),
				ObservedGeneration: ttZeroNum,
				LastProbeTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
			{
				Type:               corev1.PodInitialized,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: fixedTime(),
				ObservedGeneration: ttZeroNum,
				LastProbeTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
			{
				Type:               corev1.ContainersReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: fixedTime(),
				ObservedGeneration: ttZeroNum,
				LastProbeTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
			{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: fixedTime(),
				ObservedGeneration: ttZeroNum,
				LastProbeTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
		},
		ObservedGeneration:                   ttZeroNum,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild254() corev1.PodSpec {
	return corev1.PodSpec{
		NodeName: ttSNode1,
		Containers: []corev1.Container{
			ttBuild48(),
			ttBuild47(),
		},
		Volumes: []corev1.Volume{
			{Name: ttData, VolumeSource: ttBuild46()},
			{Name: ttConfig, VolumeSource: ttBuild45()},
		},
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild255() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild53(),
			ttBuild52(),
			ttBuild51(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild256() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild56(),
			ttBuild55(),
			ttBuild54(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild257() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild59()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild258() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
			corev1.ResourcePods:   resource.MustParse(ttSN110),
		},
		Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("7500m"),
			corev1.ResourceMemory: resource.MustParse("14Gi"),
			corev1.ResourcePods:   resource.MustParse("100"),
		},
		Phase:      ttEmptyStr,
		Conditions: nil,
		Addresses:  nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild61(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild259() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild64(),
			Spec:       ttBuild63(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild260() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Strategy: appsv1.DeploymentStrategy{
			Type:          appsv1.RecreateDeploymentStrategyType,
			RollingUpdate: nil,
		},
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild69(),
			Spec:       ttBuild68(),
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild261() corev1.Service {
	return corev1.Service{
		Spec: ttBuild71(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild70(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild262() corev1.Service {
	return corev1.Service{
		Spec:   ttBuild74(),
		Status: ttBuild73(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild72(),
	}
}

func ttBuild263() corev1.Service {
	return corev1.Service{
		ObjectMeta: ttBuild76(),
		Spec:       ttBuild75(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild264() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{ttBuild80()},
		Volumes: []corev1.Volume{{
			Name:         "my-volume",
			VolumeSource: ttBuild79(),
		}},
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild265() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild83()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild266() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild85(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild267() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild88()},
		InitContainers:                []corev1.Container{ttBuild87()},
		EphemeralContainers:           []corev1.EphemeralContainer{ttBuild86()},
		Volumes:                       nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild268() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild89(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild269() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild90()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild270() corev1.Namespace {
	return corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild92(),
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
		Status: corev1.NamespaceStatus{
			Phase:      ttEmptyStr,
			Conditions: nil,
		},
	}
}

func ttBuild271() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild93(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild272() corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild96(),
		Spec:       ttBuild95(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild273() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild99(),
			Spec:       ttBuild98(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild274() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild103(),
		Spec:       ttBuild102(),
		Status:     ttBuild101(),
	}
}

func ttBuild275() corev1.Event {
	return corev1.Event{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild104(),
		InvolvedObject: corev1.ObjectReference{
			Kind:            ttEmptyStr,
			Namespace:       ttEmptyStr,
			Name:            ttEmptyStr,
			UID:             ttEmptyStr,
			APIVersion:      ttEmptyStr,
			ResourceVersion: ttEmptyStr,
			FieldPath:       ttEmptyStr,
		},
		Reason:  ttEmptyStr,
		Message: ttEmptyStr,
		Source: corev1.EventSource{
			Component: ttEmptyStr,
			Host:      ttEmptyStr,
		},
		FirstTimestamp: metav1.Time{
			Time: time.Time{},
		},
		LastTimestamp: metav1.Time{
			Time: time.Time{},
		},
		Count: ttZeroNum,
		Type:  ttEmptyStr,
		EventTime: metav1.MicroTime{
			Time: time.Time{},
		},
		Series:              nil,
		Action:              ttEmptyStr,
		Related:             nil,
		ReportingController: ttEmptyStr,
		ReportingInstance:   ttEmptyStr,
	}
}

func ttBuild276() corev1.Event {
	return corev1.Event{
		Count: ttN2,
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild105(),
		InvolvedObject: corev1.ObjectReference{
			Kind:            ttEmptyStr,
			Namespace:       ttEmptyStr,
			Name:            ttEmptyStr,
			UID:             ttEmptyStr,
			APIVersion:      ttEmptyStr,
			ResourceVersion: ttEmptyStr,
			FieldPath:       ttEmptyStr,
		},
		Reason:  ttEmptyStr,
		Message: ttEmptyStr,
		Source: corev1.EventSource{
			Component: ttEmptyStr,
			Host:      ttEmptyStr,
		},
		FirstTimestamp: metav1.Time{
			Time: time.Time{},
		},
		LastTimestamp: metav1.Time{
			Time: time.Time{},
		},
		Type: ttEmptyStr,
		EventTime: metav1.MicroTime{
			Time: time.Time{},
		},
		Series:              nil,
		Action:              ttEmptyStr,
		Related:             nil,
		ReportingController: ttEmptyStr,
		ReportingInstance:   ttEmptyStr,
	}
}

func ttBuild277() corev1.Event {
	return corev1.Event{
		Count: ttN1,
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild106(),
		InvolvedObject: corev1.ObjectReference{
			Kind:            ttEmptyStr,
			Namespace:       ttEmptyStr,
			Name:            ttEmptyStr,
			UID:             ttEmptyStr,
			APIVersion:      ttEmptyStr,
			ResourceVersion: ttEmptyStr,
			FieldPath:       ttEmptyStr,
		},
		Reason:  ttEmptyStr,
		Message: ttEmptyStr,
		Source: corev1.EventSource{
			Component: ttEmptyStr,
			Host:      ttEmptyStr,
		},
		FirstTimestamp: metav1.Time{
			Time: time.Time{},
		},
		LastTimestamp: metav1.Time{
			Time: time.Time{},
		},
		Type: ttEmptyStr,
		EventTime: metav1.MicroTime{
			Time: time.Time{},
		},
		Series:              nil,
		Action:              ttEmptyStr,
		Related:             nil,
		ReportingController: ttEmptyStr,
		ReportingInstance:   ttEmptyStr,
	}
}

func ttBuild278() corev1.Event {
	return corev1.Event{
		Count: ttZeroNum,
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild107(),
		InvolvedObject: corev1.ObjectReference{
			Kind:            ttEmptyStr,
			Namespace:       ttEmptyStr,
			Name:            ttEmptyStr,
			UID:             ttEmptyStr,
			APIVersion:      ttEmptyStr,
			ResourceVersion: ttEmptyStr,
			FieldPath:       ttEmptyStr,
		},
		Reason:  ttEmptyStr,
		Message: ttEmptyStr,
		Source: corev1.EventSource{
			Component: ttEmptyStr,
			Host:      ttEmptyStr,
		},
		FirstTimestamp: metav1.Time{
			Time: time.Time{},
		},
		LastTimestamp: metav1.Time{
			Time: time.Time{},
		},
		Type: ttEmptyStr,
		EventTime: metav1.MicroTime{
			Time: time.Time{},
		},
		Series:              nil,
		Action:              ttEmptyStr,
		Related:             nil,
		ReportingController: ttEmptyStr,
		ReportingInstance:   ttEmptyStr,
	}
}

func ttBuild279() corev1.Event {
	return corev1.Event{
		ObjectMeta: ttBuild108(),
		Type:       ttWarning,
		Reason:     "FailedScheduling",
		Message:    "0/3 nodes available",
		InvolvedObject: corev1.ObjectReference{
			Kind: ttPod, Name: "web-1",
			Namespace:       ttEmptyStr,
			UID:             ttEmptyStr,
			APIVersion:      ttEmptyStr,
			ResourceVersion: ttEmptyStr,
			FieldPath:       ttEmptyStr,
		},
		FirstTimestamp: fixedTime(),
		LastTimestamp:  fixedTime(),
		Count:          ttN7,
		Source: corev1.EventSource{
			Component: ttScheduler,
			Host:      ttEmptyStr,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		EventTime: metav1.MicroTime{
			Time: time.Time{},
		},
		Series:              nil,
		Action:              ttEmptyStr,
		Related:             nil,
		ReportingController: ttEmptyStr,
		ReportingInstance:   ttEmptyStr,
	}
}

func ttBuild280() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild109(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild281() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild111(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild282() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild113(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild283() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild115(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild284() corev1.NodeStatus {
	return corev1.NodeStatus{
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Conditions:  nil,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild117(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild285() corev1.NodeStatus {
	return corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type: corev1.NodeReady, Status: corev1.ConditionFalse,
				LastHeartbeatTime: metav1.Time{
					Time: time.Time{},
				},
				LastTransitionTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
		},
		Capacity:    nil,
		Allocatable: nil,
		Phase:       ttEmptyStr,
		Addresses:   nil,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		NodeInfo:         ttBuild120(),
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild286() corev1.NodeStatus {
	return corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionTrue,
				Reason:  "KubeletReady",
				Message: ttSOk,
				LastHeartbeatTime: metav1.Time{
					Time: time.Time{},
				},
				LastTransitionTime: metav1.Time{
					Time: time.Time{},
				},
			},
			{
				Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse,
				LastHeartbeatTime: metav1.Time{
					Time: time.Time{},
				},
				LastTransitionTime: metav1.Time{
					Time: time.Time{},
				},
				Reason:  ttEmptyStr,
				Message: ttEmptyStr,
			},
		},
		NodeInfo: ttBuild121(),
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
			corev1.ResourcePods:   resource.MustParse(ttSN110),
		},
		Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: ttClusterIPAddr},
			{Type: corev1.NodeHostName, Address: ttSNode1},
		},
		Allocatable: nil,
		Phase:       ttEmptyStr,
		DaemonEndpoints: corev1.NodeDaemonEndpoints{
			KubeletEndpoint: corev1.DaemonEndpoint{
				Port: ttZeroNum,
			},
		},
		Images:           nil,
		VolumesInUse:     nil,
		VolumesAttached:  nil,
		Config:           nil,
		RuntimeHandlers:  nil,
		Features:         nil,
		DeclaredFeatures: nil,
	}
}

func ttBuild287() corev1.Service {
	return corev1.Service{
		Spec: ttBuild124(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild123(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild288() corev1.Service {
	return corev1.Service{
		Spec: ttBuild126(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild125(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild289() corev1.Service {
	return corev1.Service{
		Spec:   ttBuild129(),
		Status: ttBuild128(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild127(),
	}
}

func ttBuild290() corev1.Service {
	return corev1.Service{
		Spec:   ttBuild132(),
		Status: ttBuild131(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild130(),
	}
}

func ttBuild291() corev1.Service {
	return corev1.Service{
		Spec: ttBuild134(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild133(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild292() corev1.Service {
	return corev1.Service{
		Spec: ttBuild136(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild135(),
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild293() corev1.Service {
	return corev1.Service{
		ObjectMeta: ttBuild138(),
		Spec:       ttBuild137(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: nil,
			},
			Conditions: nil,
		},
	}
}

func ttBuild294() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild141(),
			Spec:       ttBuild140(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild295() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild146(),
			Spec:       ttBuild145(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild296() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild150(),
			Spec:       ttBuild149(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild297() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild154(),
			Spec:       ttBuild153(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild298() appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: nil,
		Selector: nil,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: ttBuild158(),
			Spec:       ttBuild157(),
		},
		Strategy: appsv1.DeploymentStrategy{
			Type:          ttEmptyStr,
			RollingUpdate: nil,
		},
		MinReadySeconds:         ttZeroNum,
		RevisionHistoryLimit:    nil,
		Paused:                  false,
		ProgressDeadlineSeconds: nil,
	}
}

func ttBuild299() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild162(),
			ttBuild161(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild300() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild165()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild301() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{ttBuild169()},
		Volumes: []corev1.Volume{
			{Name: ttData, VolumeSource: ttBuild168()},
			{Name: ttConfig, VolumeSource: ttBuild167()},
		},
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild302() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild172(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild303() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild174(),
			ttBuild173(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild304() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodPending,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild176(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild305() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild177()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild306() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild183(),
			ttBuild182(),
			ttBuild181(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild307() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild186(),
			ttBuild185(),
			ttBuild184(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild308() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			ttBuild190(),
			ttBuild189(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild309() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                    []corev1.Container{ttBuild192()},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		NodeName:                      ttEmptyStr,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild310() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		PodIP: ttPodIP,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild194(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild311() corev1.PodSpec {
	return corev1.PodSpec{
		NodeName: ttSNode1,
		Containers: []corev1.Container{
			ttBuild195(),
		},
		Volumes:                       nil,
		InitContainers:                nil,
		EphemeralContainers:           nil,
		RestartPolicy:                 ttEmptyStr,
		TerminationGracePeriodSeconds: nil,
		ActiveDeadlineSeconds:         nil,
		DNSPolicy:                     ttEmptyStr,
		NodeSelector:                  nil,
		ServiceAccountName:            ttEmptyStr,
		DeprecatedServiceAccount:      ttEmptyStr,
		AutomountServiceAccountToken:  nil,
		HostNetwork:                   false,
		HostPID:                       false,
		HostIPC:                       false,
		ShareProcessNamespace:         nil,
		SecurityContext:               nil,
		ImagePullSecrets:              nil,
		Hostname:                      ttEmptyStr,
		Subdomain:                     ttEmptyStr,
		Affinity:                      nil,
		SchedulerName:                 ttEmptyStr,
		Tolerations:                   nil,
		HostAliases:                   nil,
		PriorityClassName:             ttEmptyStr,
		Priority:                      nil,
		DNSConfig:                     nil,
		ReadinessGates:                nil,
		RuntimeClassName:              nil,
		EnableServiceLinks:            nil,
		PreemptionPolicy:              nil,
		Overhead:                      nil,
		TopologySpreadConstraints:     nil,
		SetHostnameAsFQDN:             nil,
		OS:                            nil,
		HostUsers:                     nil,
		SchedulingGates:               nil,
		ResourceClaims:                nil,
		Resources:                     nil,
		HostnameOverride:              nil,
		SchedulingGroup:               nil,
	}
}

func ttBuild312() corev1.Namespace {
	return corev1.Namespace{
		ObjectMeta: ttBuild197(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
		Status: corev1.NamespaceStatus{
			Phase:      ttEmptyStr,
			Conditions: nil,
		},
	}
}

func ttBuild313() corev1.Namespace {
	return corev1.Namespace{
		ObjectMeta: ttBuild198(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
		Status: corev1.NamespaceStatus{
			Phase:      ttEmptyStr,
			Conditions: nil,
		},
	}
}

func ttBuild314() corev1.Namespace {
	return corev1.Namespace{
		ObjectMeta: ttBuild199(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
		Status: corev1.NamespaceStatus{
			Phase:      ttEmptyStr,
			Conditions: nil,
		},
	}
}

func ttBuild315() corev1.Namespace {
	return corev1.Namespace{
		ObjectMeta: ttBuild200(),
		Status: corev1.NamespaceStatus{
			Phase:      corev1.NamespaceActive,
			Conditions: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: nil,
		},
	}
}

func ttBuild316() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodPending,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild216(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild317() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild219(),
		Spec:       ttBuild218(),
		Status:     ttBuild217(),
	}
}

func ttBuild318() *corev1.Pod {
	return &corev1.Pod{
		Status: ttBuild222(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild221(),
		Spec:       ttBuild220(),
	}
}

func ttBuild319() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodSucceeded,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild225(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild320() corev1.PodStatus {
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			ttBuild228(),
		},
		ObservedGeneration:                   ttZeroNum,
		Conditions:                           nil,
		Message:                              ttEmptyStr,
		Reason:                               ttEmptyStr,
		NominatedNodeName:                    ttEmptyStr,
		HostIP:                               ttEmptyStr,
		HostIPs:                              nil,
		PodIP:                                ttEmptyStr,
		PodIPs:                               nil,
		StartTime:                            nil,
		InitContainerStatuses:                nil,
		QOSClass:                             ttEmptyStr,
		EphemeralContainerStatuses:           nil,
		Resize:                               ttEmptyStr,
		ResourceClaimStatuses:                nil,
		ExtendedResourceClaimStatus:          nil,
		AllocatedResources:                   nil,
		Resources:                            nil,
		NodeAllocatableResourceClaimStatuses: nil,
	}
}

func ttBuild321() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild3(),
		Spec:       ttBuild238(),
		Status:     ttBuild237(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild322() appsv1.Deployment {
	return appsv1.Deployment{
		Status: ttBuild7(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild6(),
		Spec:       ttBuild239(),
	}
}

func ttBuild323() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild240(),
		Status: ttBuild9(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild8(),
	}
}

func ttBuild324() corev1.Node {
	return corev1.Node{
		Status: ttBuild241(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild12(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
	}
}

func ttBuild325() *corev1.Pod {
	return &corev1.Pod{
		Spec: ttBuild245(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild21(),
		Status:     ttBuild20(),
	}
}

func ttBuild326() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild247(),
		Status: ttBuild246(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild27(),
	}
}

func ttBuild327() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild252(),
		Status: ttBuild251(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild38(),
	}
}

func ttBuild328() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild49(),
		Spec:       ttBuild254(),
		Status:     ttBuild253(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild329() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild256(),
		Status: ttBuild255(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild50(),
	}
}

func ttBuild330() corev1.Node {
	return corev1.Node{
		Status: ttBuild258(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild60(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
	}
}

func ttBuild331() appsv1.Deployment {
	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild65(),
		Spec:       ttBuild259(),
		Status:     ttBuild62(),
	}
}

func ttBuild332() appsv1.Deployment {
	return appsv1.Deployment{
		Spec: ttBuild260(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild67(),
		Status:     ttBuild66(),
	}
}

func ttBuild333() *corev1.Pod {
	return &corev1.Pod{
		Spec: ttBuild264(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild78(),
		Status:     ttBuild77(),
	}
}

func ttBuild334() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild265(),
		Status: ttBuild82(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild81(),
	}
}

func ttBuild335() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild267(),
		Status: ttBuild266(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild84(),
	}
}

func ttBuild336() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild91(),
		Spec:       ttBuild269(),
		Status:     ttBuild268(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild337() corev1.Node {
	return corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild94(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild271(),
	}
}

func ttBuild338() appsv1.Deployment {
	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild100(),
		Spec:       ttBuild273(),
		Status:     ttBuild97(),
	}
}

func ttBuild339() corev1.Node {
	return corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild110(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild280(),
	}
}

func ttBuild340() corev1.Node {
	return corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild112(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild281(),
	}
}

func ttBuild341() corev1.Node {
	return corev1.Node{
		ObjectMeta: ttBuild114(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild282(),
	}
}

func ttBuild342() corev1.Node {
	return corev1.Node{
		ObjectMeta: ttBuild116(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild283(),
	}
}

func ttBuild343() corev1.Node {
	return corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild118(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
		Status: ttBuild284(),
	}
}

func ttBuild344() corev1.Node {
	return corev1.Node{
		Status: ttBuild285(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild119(),
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
	}
}

func ttBuild345() corev1.Node {
	return corev1.Node{
		ObjectMeta: ttBuild122(),
		Status:     ttBuild286(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		Spec: corev1.NodeSpec{
			PodCIDR:            ttEmptyStr,
			PodCIDRs:           nil,
			ProviderID:         ttEmptyStr,
			Unschedulable:      false,
			Taints:             nil,
			ConfigSource:       nil,
			DoNotUseExternalID: ttEmptyStr,
		},
	}
}

func ttBuild346() appsv1.Deployment {
	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild142(),
		Spec:       ttBuild294(),
		Status:     ttBuild139(),
	}
}

func ttBuild347() appsv1.Deployment {
	return appsv1.Deployment{
		Spec: ttBuild295(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild144(),
		Status:     ttBuild143(),
	}
}

func ttBuild348() appsv1.Deployment {
	return appsv1.Deployment{
		Spec: ttBuild296(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild148(),
		Status:     ttBuild147(),
	}
}

func ttBuild349() appsv1.Deployment {
	return appsv1.Deployment{
		Spec: ttBuild297(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild152(),
		Status:     ttBuild151(),
	}
}

func ttBuild350() appsv1.Deployment {
	return appsv1.Deployment{
		Spec: ttBuild298(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild156(),
		Status:     ttBuild155(),
	}
}

func ttBuild351() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild166(),
		Spec:       ttBuild300(),
		Status:     ttBuild164(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild352() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild303(),
		Status: ttBuild302(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild171(),
	}
}

func ttBuild353() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild305(),
		Status: ttBuild304(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild175(),
	}
}

func ttBuild354() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild307(),
		Status: ttBuild306(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild180(),
	}
}

func ttBuild355() *corev1.Pod {
	return &corev1.Pod{
		Spec:   ttBuild308(),
		Status: ttBuild188(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild187(),
	}
}

func ttBuild356() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild193(),
		Spec:       ttBuild309(),
		Status:     ttBuild191(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild357() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: ttBuild196(),
		Spec:       ttBuild311(),
		Status:     ttBuild310(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
	}
}

func ttBuild358() *corev1.Pod {
	return &corev1.Pod{
		Status: ttBuild316(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild215(),
		Spec:       ttBuild214(),
	}
}

func ttBuild359() *corev1.Pod {
	return &corev1.Pod{
		Status: ttBuild319(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild224(),
		Spec:       ttBuild223(),
	}
}

func ttBuild360() *corev1.Pod {
	return &corev1.Pod{
		Status: ttBuild320(),
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild227(),
		Spec:       ttBuild226(),
	}
}

func ttBuildReasonPod1(r string) *corev1.Pod {
	return &corev1.Pod{
		Spec: ttBuild248(),
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   r,
							ExitCode: ttZeroNum,
							Signal:   ttZeroNum,
							Message:  ttEmptyStr,
							StartedAt: metav1.Time{
								Time: time.Time{},
							},
							FinishedAt: metav1.Time{
								Time: time.Time{},
							},
							ContainerID: ttEmptyStr,
						},
						Waiting: nil,
						Running: nil,
					},
					Name: ttSC,
					LastTerminationState: corev1.ContainerState{
						Waiting:    nil,
						Running:    nil,
						Terminated: nil,
					},
					Ready:                    false,
					RestartCount:             ttZeroNum,
					Image:                    ttEmptyStr,
					ImageID:                  ttEmptyStr,
					ContainerID:              ttEmptyStr,
					Started:                  nil,
					AllocatedResources:       nil,
					Resources:                nil,
					VolumeMounts:             nil,
					User:                     nil,
					AllocatedResourcesStatus: nil,
					StopSignal:               nil,
				},
			},
			ObservedGeneration:                   ttZeroNum,
			Conditions:                           nil,
			Message:                              ttEmptyStr,
			Reason:                               ttEmptyStr,
			NominatedNodeName:                    ttEmptyStr,
			HostIP:                               ttEmptyStr,
			HostIPs:                              nil,
			PodIP:                                ttEmptyStr,
			PodIPs:                               nil,
			StartTime:                            nil,
			InitContainerStatuses:                nil,
			QOSClass:                             ttEmptyStr,
			EphemeralContainerStatuses:           nil,
			Resize:                               ttEmptyStr,
			ResourceClaimStatuses:                nil,
			ExtendedResourceClaimStatus:          nil,
			AllocatedResources:                   nil,
			Resources:                            nil,
			NodeAllocatableResourceClaimStatuses: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild32(),
	}
}

func ttBuildReasonPod2(r string) *corev1.Pod {
	return &corev1.Pod{
		Spec: ttBuild249(),
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  r,
							Message: ttEmptyStr,
						},
						Running:    nil,
						Terminated: nil,
					},
					Name: ttSC,
					LastTerminationState: corev1.ContainerState{
						Waiting:    nil,
						Running:    nil,
						Terminated: nil,
					},
					Ready:                    false,
					RestartCount:             ttZeroNum,
					Image:                    ttEmptyStr,
					ImageID:                  ttEmptyStr,
					ContainerID:              ttEmptyStr,
					Started:                  nil,
					AllocatedResources:       nil,
					Resources:                nil,
					VolumeMounts:             nil,
					User:                     nil,
					AllocatedResourcesStatus: nil,
					StopSignal:               nil,
				},
			},
			ObservedGeneration:                   ttZeroNum,
			Conditions:                           nil,
			Message:                              ttEmptyStr,
			Reason:                               ttEmptyStr,
			NominatedNodeName:                    ttEmptyStr,
			HostIP:                               ttEmptyStr,
			HostIPs:                              nil,
			PodIP:                                ttEmptyStr,
			PodIPs:                               nil,
			StartTime:                            nil,
			InitContainerStatuses:                nil,
			QOSClass:                             ttEmptyStr,
			EphemeralContainerStatuses:           nil,
			Resize:                               ttEmptyStr,
			ResourceClaimStatuses:                nil,
			ExtendedResourceClaimStatus:          nil,
			AllocatedResources:                   nil,
			Resources:                            nil,
			NodeAllocatableResourceClaimStatuses: nil,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       ttEmptyStr,
			APIVersion: ttEmptyStr,
		},
		ObjectMeta: ttBuild34(),
	}
}
