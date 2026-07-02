package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

// String constants shared across the integration fixture and assertions. All
// are prefixed with "it" to avoid colliding with package-level consts declared
// in other files of this package.
const (
	itNodeIP1          = "10.0.0.1"
	itNodeIP2          = "10.0.0.2"
	itPodIPWeb         = "10.0.0.5"
	itClusterIPWeb     = "10.96.0.10"
	itCrashLoopBackOff = "CrashLoopBackOff"
	itEventTypeNormal  = "Normal"
	itEventTypeWarning = "Warning"
	itStatusActive     = "Active"
	itStatusRunning    = "Running"
	itStatusReady      = "Ready"
	itServiceClusterIP = "ClusterIP"
	itStrategyRolling  = "RollingUpdate"
	itPortWeb          = "80:8080/TCP"
	itRoleNone         = "<none>"
	itRoleControlPlane = "control-plane"
	itNotApplicable    = "N/A"

	itNsDefault    = "default"
	itNsKubeSystem = "kube-system"

	itNode1 = "node-1"
	itNode2 = "node-2"

	itPodWeb    = "web-abc"
	itPodBroken = "broken"

	itAppKey   = "app"
	itAppWeb   = "web"
	itCtrApp   = "app"
	itCtrCar   = "sidecar"
	itCtrInit  = "init-db"
	itCtrBad   = "c"
	itVolData  = "data"
	itVolCfg   = "config"
	itVolTypeC = "configMap"
	itVolTypeE = "emptyDir"

	itImageApp  = "myapp:1.0"
	itImageCar  = "envoy:1.30"
	itImageInit = "init:1"
	itImageBad  = "bad:1"

	itKindPod        = "Pod"
	itReasonSched    = "Scheduled"
	itReasonBackOff  = "BackOff"
	itSrcScheduler   = "default-scheduler"
	itSrcKubelet     = "kubelet"
	itObjWebAbc      = "Pod/web-abc"
	itObjBroken      = "Pod/broken"
	itLabelHostname  = "kubernetes.io/hostname"
	itLabelCtrlPlane = "node-role.kubernetes.io/control-plane"
	itEvtName1       = "evt-1"
	itEvtName2       = "evt-2"
	itEvtMsgSched    = "Successfully assigned default/web-abc to node-2"
	itEvtMsgBackOff  = "Back-off restarting failed container"
	itReasonKubelet  = "KubeletReady"
	itReasonNewRS    = "NewReplicaSetAvailable"
	itEnvKey         = "env"
	itEnvDev         = "dev"

	itVersion = "v1.31.0"
	itPlatorm = "linux/arm64"
	itArch    = "arm64"
	itOSImage = "Ubuntu 24.04"
	itRuntime = "containerd://2.0"

	itCPU1  = "4"
	itMem1  = "8Gi"
	itCPU2  = "8"
	itMem2  = "16Gi"
	itPods  = "110"
	itEmpty = ""
	itReady = "2/2"

	itPathCluster    = "/api/cluster"
	itPathNamespaces = "/api/namespaces"
	itPathPods       = "/api/pods"
	itPathDeploys    = "/api/deployments"
	itPathServices   = "/api/services"
	itPathNodes      = "/api/nodes"
	itPathEvents     = "/api/events"
)

// Numeric constants used by the fixture and assertions.
const (
	itZero     = 0
	itOne      = 1
	itTwo      = 2
	itThree    = 3
	itFour     = 4
	itSeven    = 7
	itPort80   = 80
	itPort8080 = 8080
)

// --- fixture builders -----------------------------------------------------
//
// Every Kubernetes object is assembled by assigning onto a zero value rather
// than via a struct literal. This keeps the fixture exhaustive without
// repeating dozens of zero-valued fields, and avoids partial struct literals.

func itNamespace(
	name string,
	labels map[string]string,
	created metav1.Time,
) *corev1.Namespace {
	ns := new(corev1.Namespace)
	ns.Name = name
	ns.Labels = labels
	ns.CreationTimestamp = created
	ns.Status.Phase = corev1.NamespaceActive

	return ns
}

func itNodeCondition(created metav1.Time, reason string) corev1.NodeCondition {
	var c corev1.NodeCondition

	c.Type = corev1.NodeReady
	c.Status = corev1.ConditionTrue
	c.Reason = reason
	c.LastHeartbeatTime = created
	c.LastTransitionTime = created

	return c
}

func itNodeSystemInfo() corev1.NodeSystemInfo {
	var info corev1.NodeSystemInfo

	info.OSImage = itOSImage
	info.ContainerRuntimeVersion = itRuntime
	info.KubeletVersion = itVersion
	info.Architecture = itArch

	return info
}

func itNodeAddress(
	kind corev1.NodeAddressType,
	addr string,
) corev1.NodeAddress {
	var a corev1.NodeAddress

	a.Type = kind
	a.Address = addr

	return a
}

func itNode(
	name, reason, cpu, mem string,
	labels map[string]string,
	addrs []corev1.NodeAddress,
	created metav1.Time,
) *corev1.Node {
	n := new(corev1.Node)
	n.Name = name
	n.Labels = labels
	n.CreationTimestamp = created
	n.Status.Conditions = []corev1.NodeCondition{
		itNodeCondition(created, reason),
	}
	n.Status.NodeInfo = itNodeSystemInfo()
	n.Status.Capacity = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
		corev1.ResourcePods:   resource.MustParse(itPods),
	}
	n.Status.Addresses = addrs

	return n
}

func itContainer(name, image string) corev1.Container {
	var c corev1.Container

	c.Name = name
	c.Image = image

	return c
}

func itContainerPort(port int32) corev1.ContainerPort {
	var p corev1.ContainerPort

	p.ContainerPort = port
	p.Protocol = corev1.ProtocolTCP

	return p
}

func itAppContainer() corev1.Container {
	c := itContainer(itCtrApp, itImageApp)
	c.Ports = []corev1.ContainerPort{itContainerPort(itPort8080)}

	return c
}

func itVolume(name string, src corev1.VolumeSource) corev1.Volume {
	var v corev1.Volume

	v.Name = name
	v.VolumeSource = src

	return v
}

func itEmptyDirSource() corev1.VolumeSource {
	var src corev1.VolumeSource

	src.EmptyDir = new(corev1.EmptyDirVolumeSource)

	return src
}

func itConfigMapSource() corev1.VolumeSource {
	var src corev1.VolumeSource

	src.ConfigMap = new(corev1.ConfigMapVolumeSource)

	return src
}

func itRunningStatus(name string, restarts int32) corev1.ContainerStatus {
	var s corev1.ContainerStatus

	s.Name = name
	s.Ready = true
	s.RestartCount = restarts
	s.State.Running = new(corev1.ContainerStateRunning)

	return s
}

func itWaitingStatus(
	name, reason string,
	restarts int32,
) corev1.ContainerStatus {
	var s corev1.ContainerStatus

	waiting := new(corev1.ContainerStateWaiting)
	waiting.Reason = reason

	s.Name = name
	s.Ready = false
	s.RestartCount = restarts
	s.State.Waiting = waiting

	return s
}

func itReadyCondition(created metav1.Time) corev1.PodCondition {
	var c corev1.PodCondition

	c.Type = corev1.PodReady
	c.Status = corev1.ConditionTrue
	c.LastTransitionTime = created

	return c
}

func itWebPod(created metav1.Time) *corev1.Pod {
	emptyDir := itEmptyDirSource()
	cfgMap := itConfigMapSource()

	pod := new(corev1.Pod)
	pod.Name = itPodWeb
	pod.Namespace = itNsDefault
	pod.Labels = map[string]string{itAppKey: itAppWeb}
	pod.CreationTimestamp = created
	pod.Spec.NodeName = itNode2
	pod.Spec.Containers = []corev1.Container{
		itAppContainer(),
		itContainer(itCtrCar, itImageCar),
	}
	pod.Spec.InitContainers = []corev1.Container{
		itContainer(itCtrInit, itImageInit),
	}
	pod.Spec.Volumes = []corev1.Volume{
		itVolume(itVolData, emptyDir),
		itVolume(itVolCfg, cfgMap),
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = itPodIPWeb
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		itRunningStatus(itCtrApp, itZero),
		itRunningStatus(itCtrCar, itOne),
	}
	pod.Status.Conditions = []corev1.PodCondition{itReadyCondition(created)}

	return pod
}

func itBrokenPod(created metav1.Time) *corev1.Pod {
	pod := new(corev1.Pod)
	pod.Name = itPodBroken
	pod.Namespace = itNsDefault
	pod.CreationTimestamp = created
	pod.Spec.Containers = []corev1.Container{
		itContainer(itCtrBad, itImageBad),
	}
	pod.Status.Phase = corev1.PodPending
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		itWaitingStatus(itCtrBad, itCrashLoopBackOff, itSeven),
	}

	return pod
}

func itDepCondition(created metav1.Time) appsv1.DeploymentCondition {
	var c appsv1.DeploymentCondition

	c.Type = appsv1.DeploymentProgressing
	c.Status = corev1.ConditionTrue
	c.Reason = itReasonNewRS
	c.LastTransitionTime = created

	return c
}

func itDeployment(created metav1.Time) *appsv1.Deployment {
	replicas := int32(itThree)

	selector := new(metav1.LabelSelector)
	selector.MatchLabels = map[string]string{itAppKey: itAppWeb}

	dep := new(appsv1.Deployment)
	dep.Name = itAppWeb
	dep.Namespace = itNsDefault
	dep.Labels = map[string]string{itAppKey: itAppWeb}
	dep.CreationTimestamp = created
	dep.Spec.Replicas = &replicas
	dep.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	dep.Spec.Selector = selector
	dep.Spec.Template.Spec.Containers = []corev1.Container{
		itContainer(itCtrApp, itImageApp),
		itContainer(itCtrCar, itImageCar),
	}
	dep.Status.Replicas = itThree
	dep.Status.ReadyReplicas = itTwo
	dep.Status.UpdatedReplicas = itThree
	dep.Status.AvailableReplicas = itTwo
	dep.Status.Conditions = []appsv1.DeploymentCondition{
		itDepCondition(created),
	}

	return dep
}

func itService(created metav1.Time) *corev1.Service {
	port := corev1.ServicePort{
		Name:        itEmpty,
		Protocol:    corev1.ProtocolTCP,
		AppProtocol: nil,
		Port:        itPort80,
		TargetPort:  intstr.FromInt(itPort8080),
		NodePort:    itZero,
	}

	svc := new(corev1.Service)
	svc.Name = itAppWeb
	svc.Namespace = itNsDefault
	svc.Labels = map[string]string{itAppKey: itAppWeb}
	svc.CreationTimestamp = created
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.ClusterIP = itClusterIPWeb
	svc.Spec.Selector = map[string]string{itAppKey: itAppWeb}
	svc.Spec.Ports = []corev1.ServicePort{port}

	return svc
}

func itEvent(
	name, eventType, reason, message, obj, source string,
	count int32,
	created metav1.Time,
) *corev1.Event {
	ev := new(corev1.Event)
	ev.Name = name
	ev.Namespace = itNsDefault
	ev.Type = eventType
	ev.Reason = reason
	ev.Message = message
	ev.InvolvedObject.Kind = itKindPod
	ev.InvolvedObject.Name = obj
	ev.FirstTimestamp = created
	ev.LastTimestamp = created
	ev.Count = count
	ev.Source.Component = source

	return ev
}

// realisticCluster returns a fixture that mirrors a typical small K8s cluster:
// 2 namespaces, 2 nodes, a couple of deployments, services, and pods (one of
// which is multi-container with conditions, volumes, and partial container
// statuses to mimic a real running workload). This is the input for every
// integration test in this file.
func realisticCluster() []runtime.Object {
	created := metav1.NewTime(time.Now().Add(-itThree * time.Hour))

	node1Labels := map[string]string{
		itLabelCtrlPlane: itEmpty,
		itLabelHostname:  itNode1,
	}
	node1Addrs := []corev1.NodeAddress{
		itNodeAddress(corev1.NodeInternalIP, itNodeIP1),
		itNodeAddress(corev1.NodeHostName, itNode1),
	}
	node2Labels := map[string]string{itLabelHostname: itNode2}
	node2Addrs := []corev1.NodeAddress{
		itNodeAddress(corev1.NodeInternalIP, itNodeIP2),
	}
	defaultLabels := map[string]string{itEnvKey: itEnvDev}

	return []runtime.Object{
		itNamespace(itNsDefault, defaultLabels, created),
		itNamespace(itNsKubeSystem, nil, created),
		itNode(
			itNode1, itReasonKubelet, itCPU1, itMem1,
			node1Labels, node1Addrs, created,
		),
		itNode(
			itNode2, itEmpty, itCPU2, itMem2,
			node2Labels, node2Addrs, created,
		),
		itWebPod(created),
		itBrokenPod(created),
		itDeployment(created),
		itService(created),
		itEvent(
			itEvtName1, itEventTypeNormal, itReasonSched, itEvtMsgSched,
			itPodWeb, itSrcScheduler, itOne, created,
		),
		itEvent(
			itEvtName2, itEventTypeWarning, itReasonBackOff, itEvtMsgBackOff,
			itPodBroken, itSrcKubelet, itFour, created,
		),
	}
}

// --- HTTP / decode helpers ------------------------------------------------

// newIntegrationServer wires the realistic fixture into the real router and
// returns an httptest.Server. All integration tests share this setup.
func newIntegrationServer(t *testing.T) *httptest.Server {
	t.Helper()

	info := new(version.Info)
	info.GitVersion = itVersion
	info.Platform = itPlatorm

	srv, _ := newTestServer(t, info, realisticCluster()...)

	return srv
}

func fetchJSON(t *testing.T, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()

	ctx := t.Context()
	url := srv.URL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request %s: %v", path, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}

	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			t.Fatalf("close body %s: %v", path, cerr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", path, err)
	}

	return resp.StatusCode, body
}

// decode unmarshals b into the caller-supplied target. The target is passed by
// pointer (rather than returned) so this generic helper does not return a bare
// type parameter.
func decode[T any](t *testing.T, b []byte, target *T) {
	t.Helper()

	err := json.Unmarshal(b, target)
	if err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, b)
	}
}

// assertDecodes decodes b into a fresh value of type T and discards it; it is
// used as the endpoint contract assertion that the body unmarshals cleanly.
func assertDecodes[T any](t *testing.T, b []byte) {
	t.Helper()

	var v T

	decode(t, b, &v)
}

// fetchOK fetches path, requires HTTP 200, and decodes the body into target.
func fetchOK[T any](
	t *testing.T,
	srv *httptest.Server,
	path string,
	target *T,
) {
	t.Helper()

	code, body := fetchJSON(t, srv, path)
	if code != http.StatusOK {
		t.Fatalf("%s: code = %d, body = %s", path, code, body)
	}

	decode(t, body, target)
}

// --- end-to-end integration tests over the realistic fixture --------------

func TestIntegration_Cluster(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var info ClusterInfo

	fetchOK(t, srv, itPathCluster, &info)

	if info.Version != itVersion || info.Platform != itPlatorm {
		t.Fatalf("version/platform: %+v", info)
	}

	if info.NodeCount != itTwo {
		t.Fatalf("node count = %d", info.NodeCount)
	}
}

func TestIntegration_Namespaces(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []Namespace

	fetchOK(t, srv, itPathNamespaces, &out)

	names := namesOf(out, func(n Namespace) string { return n.Name })
	wantSubset(t, names, []string{itNsDefault, itNsKubeSystem})

	for _, ns := range out {
		if ns.Status != itStatusActive {
			t.Fatalf("ns %q status = %q", ns.Name, ns.Status)
		}

		if ns.Labels == nil {
			t.Fatalf("ns %q labels nil", ns.Name)
		}
	}
}

func TestIntegration_Pods_AllNamespaces(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []Pod

	fetchOK(t, srv, itPathPods, &out)

	if len(out) != itTwo {
		t.Fatalf("pod count = %d, want 2", len(out))
	}

	byName := indexBy(out, func(p Pod) string { return p.Name })
	assertWebPod(t, byName[itPodWeb])
	assertBrokenPod(t, byName[itPodBroken])
}

func assertWebPod(t *testing.T, web Pod) {
	t.Helper()

	if web.Status != itStatusRunning {
		t.Fatalf("web status = %q", web.Status)
	}

	if web.Ready != itReady {
		t.Fatalf("web ready = %q", web.Ready)
	}

	if web.Restarts != itOne {
		t.Fatalf("web restarts = %d", web.Restarts)
	}

	if web.Node != itNode2 {
		t.Fatalf("web node = %q", web.Node)
	}

	if web.IP != itPodIPWeb {
		t.Fatalf("web ip = %q", web.IP)
	}

	assertWebContainers(t, web.Containers)
	assertWebVolumes(t, web.Volumes)
}

func assertWebContainers(t *testing.T, containers []Container) {
	t.Helper()

	if len(containers) != itThree {
		t.Fatalf("web containers = %d, want 3", len(containers))
	}

	cnames := namesOf(containers, func(c Container) string { return c.Name })
	wantSubset(t, cnames, []string{itCtrApp, itCtrCar, itCtrInit})

	// Init container "init-db" is surfaced and tagged with its kind.
	for _, c := range containers {
		if c.Name == itCtrInit && c.Kind != "init" {
			t.Fatalf("init container kind = %q, want init", c.Kind)
		}
	}
}

func assertWebVolumes(t *testing.T, volumes []Volume) {
	t.Helper()

	vtypes := namesOf(volumes, func(v Volume) string { return v.Type })
	slices.Sort(vtypes)

	if vtypes[itZero] != itVolTypeC || vtypes[itOne] != itVolTypeE {
		t.Fatalf("volume types = %v", vtypes)
	}
}

func assertBrokenPod(t *testing.T, broken Pod) {
	t.Helper()

	if broken.Status != itCrashLoopBackOff {
		t.Fatalf("broken status = %q", broken.Status)
	}

	if broken.Restarts != itSeven {
		t.Fatalf("broken restarts = %d", broken.Restarts)
	}
}

func TestIntegration_Pods_FilteredByNamespace(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []Pod

	fetchOK(t, srv, itPathPods+"?namespace="+itNsKubeSystem, &out)

	if len(out) != itZero {
		t.Fatalf("expected 0 pods in kube-system, got %d", len(out))
	}
}

func TestIntegration_PodDetail(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var p Pod

	fetchOK(t, srv, itPathPods+"/"+itNsDefault+"/"+itPodWeb, &p)

	if p.Name != itPodWeb {
		t.Fatalf("name = %q", p.Name)
	}

	wrong := len(p.Conditions) != itOne ||
		p.Conditions[itZero].Type != itStatusReady
	if wrong {
		t.Fatalf("conditions = %+v", p.Conditions)
	}
}

func TestIntegration_Deployments(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []Deployment

	fetchOK(t, srv, itPathDeploys, &out)

	if len(out) != itOne {
		t.Fatalf("deployment count = %d", len(out))
	}

	assertWebDeployment(t, out[itZero])
}

func assertWebDeployment(t *testing.T, d Deployment) {
	t.Helper()

	okReplicas := d.DesiredReplicas == itThree && d.Replicas == itThree &&
		d.ReadyReplicas == itTwo && d.AvailableReplicas == itTwo
	if !okReplicas {
		t.Fatalf("replicas wrong: %+v", d)
	}

	if d.Strategy != itStrategyRolling {
		t.Fatalf("strategy = %q", d.Strategy)
	}

	slices.Sort(d.Images)

	okImages := len(d.Images) == itTwo &&
		d.Images[itZero] == itImageCar && d.Images[itOne] == itImageApp
	if !okImages {
		t.Fatalf("images = %v", d.Images)
	}
}

func TestIntegration_Services(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []Service

	fetchOK(t, srv, itPathServices, &out)

	if len(out) != itOne {
		t.Fatalf("svc count = %d", len(out))
	}

	assertWebService(t, out[itZero])
}

func assertWebService(t *testing.T, s Service) {
	t.Helper()

	okShape := s.Type == itServiceClusterIP &&
		s.ClusterIP == itClusterIPWeb && s.ExternalIP == itNotApplicable
	if !okShape {
		t.Fatalf("svc shape wrong: %+v", s)
	}

	if len(s.Ports) != itOne || s.Ports[itZero] != itPortWeb {
		t.Fatalf("ports = %v", s.Ports)
	}
}

func TestIntegration_Nodes(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []NodeInfo

	fetchOK(t, srv, itPathNodes, &out)

	if len(out) != itTwo {
		t.Fatalf("node count = %d", len(out))
	}

	byName := indexBy(out, func(n NodeInfo) string { return n.Name })
	assertNode1(t, byName[itNode1])
	assertNode2(t, byName[itNode2])
}

func assertNode1(t *testing.T, n1 NodeInfo) {
	t.Helper()

	if n1.Status != itStatusReady {
		t.Fatalf("n1 status = %q", n1.Status)
	}

	if len(n1.Roles) != itOne || n1.Roles[itZero] != itRoleControlPlane {
		t.Fatalf("n1 roles = %v", n1.Roles)
	}

	okCap := n1.CPU == itCPU1 && n1.Memory == itMem1 && n1.Pods == itPods
	if !okCap {
		t.Fatalf("n1 capacity wrong: %+v", n1)
	}
}

func assertNode2(t *testing.T, n2 NodeInfo) {
	t.Helper()

	if len(n2.Roles) != itOne || n2.Roles[itZero] != itRoleNone {
		t.Fatalf("n2 roles = %v, want [<none>]", n2.Roles)
	}
}

func TestIntegration_Events(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	var out []KubeEvent

	fetchOK(t, srv, itPathEvents, &out)

	if len(out) != itTwo {
		t.Fatalf("event count = %d", len(out))
	}

	byReason := indexBy(out, func(e KubeEvent) string { return e.Reason })
	assertScheduledEvent(t, byReason)
	assertBackOffEvent(t, byReason)
}

func assertScheduledEvent(t *testing.T, byReason map[string]KubeEvent) {
	t.Helper()

	e, ok := byReason[itReasonSched]

	bad := !ok || e.Object != itObjWebAbc || e.Source != itSrcScheduler
	if bad {
		t.Fatalf("Scheduled event = %+v", e)
	}
}

func assertBackOffEvent(t *testing.T, byReason map[string]KubeEvent) {
	t.Helper()

	e, ok := byReason[itReasonBackOff]

	bad := !ok || e.Object != itObjBroken ||
		e.Count != itFour || e.Type != itEventTypeWarning
	if bad {
		t.Fatalf("BackOff event = %+v", e)
	}
}

// contractEndpoint pairs an endpoint path with the decode assertion that
// proves its response matches the frontend's typed interface. The subtest
// name is derived from the path (its suffix after "/api/").
type contractEndpoint struct {
	assert func(t *testing.T, body []byte)
	path   string
}

func (e contractEndpoint) name() string {
	return strings.TrimPrefix(e.path, "/api/")
}

func contractEndpoints() []contractEndpoint {
	return []contractEndpoint{
		{path: itPathCluster, assert: assertDecodes[ClusterInfo]},
		{path: itPathNamespaces, assert: assertDecodes[[]Namespace]},
		{path: itPathPods, assert: assertDecodes[[]Pod]},
		{path: itPathDeploys, assert: assertDecodes[[]Deployment]},
		{path: itPathServices, assert: assertDecodes[[]Service]},
		{path: itPathNodes, assert: assertDecodes[[]NodeInfo]},
		{path: itPathEvents, assert: assertDecodes[[]KubeEvent]},
	}
}

// TestIntegration_AllResponseShapesMatchFrontendContract round-trips every
// endpoint through the frontend's API interface definitions. If any field
// name/type drifts, the json.Unmarshal into the typed Go struct (which has
// the same JSON tags as the TypeScript interface in api.ts) will fail or
// produce zero values that we can detect.
func TestIntegration_AllResponseShapesMatchFrontendContract(t *testing.T) {
	t.Parallel()

	srv := newIntegrationServer(t)

	for _, e := range contractEndpoints() {
		t.Run(e.name(), func(t *testing.T) {
			t.Parallel()
			runContractCase(t, srv, e)
		})
	}
}

func runContractCase(
	t *testing.T,
	srv *httptest.Server,
	e contractEndpoint,
) {
	t.Helper()

	code, body := fetchJSON(t, srv, e.path)
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, body)
	}

	// Forbid `null` anywhere in the response — the frontend treats every
	// collection field as non-nullable.
	if strings.Contains(string(body), "null") {
		t.Fatalf("response contains null literal: %s", body)
	}

	e.assert(t, body)
}

// --- tiny generic helpers (kept private to this file) --------------------

func namesOf[T any, K comparable](xs []T, f func(T) K) []K {
	out := make([]K, itZero, len(xs))
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

func wantSubset[T comparable](t *testing.T, got, want []T) {
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
