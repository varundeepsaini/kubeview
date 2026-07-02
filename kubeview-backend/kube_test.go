package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

const (
	ktNamespaceDefault = "default"
	ktNamespaceSystem  = "kube-system"
	ktContextName      = "test-context"
	ktClusterName      = "test-cluster"
	ktPodWeb           = "web"
	ktPodA             = "a"
	ktContainerSidecar = "sidecar"
	ktContainerMain    = "main"
	ktNameAPI          = "api"
	ktReasonScheduled  = "Scheduled"
	ktPlatformAMD64    = "linux/amd64"
	ktVersionGit       = "v1.30.0"
	ktSubresourceLog   = "log"
	ktVerbGet          = "get"

	ktAnnotationDefaultContainer = "kubectl.kubernetes.io/default-container"
	ktBodyOK                     = "ok"
	ktMsgContainerOpt            = "expected Container option %q, got %q"
	ktEnvKubeconfig              = "KUBECONFIG"
	ktConfigFileName             = "config"
	ktEmpty                      = ""

	ktResourcePods        = "pods"
	ktResourceDeployments = "deployments"
	ktResourceServices    = "services"
	ktResourceNodes       = "nodes"
	ktResourceEvents      = "events"

	ktNameNoNSFilter = "no namespace filter"
	ktNameNSFilter   = "namespace filter"

	ktMsgErr = "err: %v"

	ktZero               = 0
	ktOne                = 1
	ktThreeNodes         = 3
	ktExpectedNamespaces = 2
	ktSampleTailLines    = 42
	ktFileModeFile       = 0o600
	ktDirModePrivate     = 0o700
	ktDefaultTailLines   = 100
)

// Static errors injected through fake reactors. err113 forbids defining
// dynamic errors inline, so they live here as package-level sentinels.
var (
	errBoomKt      = errors.New("boom")
	errBackendDown = errors.New("backend down")
	errAPIUnreach  = errors.New("api unreachable")
	errNodesDown   = errors.New("nodes down")
)

// errDiscovery embeds the FakeDiscovery (for all the methods we don't care
// about) and overrides ServerVersion to return a configured error.
type errDiscovery struct {
	discovery.DiscoveryInterface

	err error
}

func (e errDiscovery) ServerVersion() (*version.Info, error) {
	return nil, e.err
}

// The exhaustruct linter requires every field of a struct *literal* to be
// listed. The Kubernetes API structs (PodSpec, NodeStatus, metav1.TypeMeta,
// metav1.Time, ...) have dozens of fields that the tests never exercise, so
// the builders below assign pre-declared zero-value variables to those fields
// instead of writing nested struct literals. exhaustruct only inspects
// composite literals, so a zero-value variable satisfies it without forcing us
// to enumerate every nested field by hand.
var (
	ktZeroTypeMeta      metav1.TypeMeta
	ktZeroTime          metav1.Time
	ktZeroMicroTime     metav1.MicroTime
	ktZeroPodSpec       corev1.PodSpec
	ktZeroContainer     corev1.Container
	ktZeroPodStatus     corev1.PodStatus
	ktZeroNamespaceSpec corev1.NamespaceSpec
	ktZeroNamespaceStat corev1.NamespaceStatus
	ktZeroNodeSpec      corev1.NodeSpec
	ktZeroNodeStatus    corev1.NodeStatus
	ktZeroServiceSpec   corev1.ServiceSpec
	ktZeroServiceStat   corev1.ServiceStatus
	ktZeroDeploySpec    appsv1.DeploymentSpec
	ktZeroDeployStatus  appsv1.DeploymentStatus
	ktZeroObjectRef     corev1.ObjectReference
	ktZeroEventSource   corev1.EventSource
	ktZeroRuntimeTM     runtime.TypeMeta
)

// objectMeta builds a fully-populated metav1.ObjectMeta so exhaustruct is
// satisfied while the test only cares about Name/Namespace.
func objectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:                       name,
		GenerateName:               ktEmpty,
		Namespace:                  namespace,
		SelfLink:                   ktEmpty,
		UID:                        ktEmpty,
		ResourceVersion:            ktEmpty,
		Generation:                 ktZero,
		CreationTimestamp:          ktZeroTime,
		DeletionTimestamp:          nil,
		DeletionGracePeriodSeconds: nil,
		Labels:                     nil,
		Annotations:                nil,
		OwnerReferences:            nil,
		Finalizers:                 nil,
		ManagedFields:              nil,
	}
}

func ktNewPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta:   ktZeroTypeMeta,
		ObjectMeta: objectMeta(name, namespace),
		Spec:       ktZeroPodSpec,
		Status:     ktZeroPodStatus,
	}
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta:   ktZeroTypeMeta,
		ObjectMeta: objectMeta(name, ktEmpty),
		Spec:       ktZeroNamespaceSpec,
		Status:     ktZeroNamespaceStat,
	}
}

func ktNewNode(name string) *corev1.Node {
	return &corev1.Node{
		TypeMeta:   ktZeroTypeMeta,
		ObjectMeta: objectMeta(name, ktEmpty),
		Spec:       ktZeroNodeSpec,
		Status:     ktZeroNodeStatus,
	}
}

func newService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta:   ktZeroTypeMeta,
		ObjectMeta: objectMeta(name, namespace),
		Spec:       ktZeroServiceSpec,
		Status:     ktZeroServiceStat,
	}
}

func newDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta:   ktZeroTypeMeta,
		ObjectMeta: objectMeta(name, namespace),
		Spec:       ktZeroDeploySpec,
		Status:     ktZeroDeployStatus,
	}
}

func newEvent(name, namespace, reason string) *corev1.Event {
	return &corev1.Event{
		TypeMeta:            ktZeroTypeMeta,
		ObjectMeta:          objectMeta(name, namespace),
		InvolvedObject:      ktZeroObjectRef,
		Reason:              reason,
		Message:             ktEmpty,
		Source:              ktZeroEventSource,
		FirstTimestamp:      ktZeroTime,
		LastTimestamp:       ktZeroTime,
		Count:               ktZero,
		Type:                ktEmpty,
		EventTime:           ktZeroMicroTime,
		Series:              nil,
		Action:              ktEmpty,
		Related:             nil,
		ReportingController: ktEmpty,
		ReportingInstance:   ktEmpty,
	}
}

func newVersionInfo(gitVersion, platform string) *version.Info {
	return &version.Info{
		Major:                 ktEmpty,
		Minor:                 ktEmpty,
		EmulationMajor:        ktEmpty,
		EmulationMinor:        ktEmpty,
		MinCompatibilityMajor: ktEmpty,
		MinCompatibilityMinor: ktEmpty,
		GitVersion:            gitVersion,
		GitCommit:             ktEmpty,
		GitTreeState:          ktEmpty,
		BuildDate:             ktEmpty,
		GoVersion:             ktEmpty,
		Compiler:              ktEmpty,
		Platform:              platform,
	}
}

func ktNewUnknown(raw string) *runtime.Unknown {
	return &runtime.Unknown{
		TypeMeta:        ktZeroRuntimeTM,
		Raw:             []byte(raw),
		ContentEncoding: ktEmpty,
		ContentType:     ktEmpty,
	}
}

// newTestClient builds a Client wired up to a fake clientset preloaded with
// the given objects. If serverVersion is non-nil it overrides what
// Discovery().ServerVersion() reports.
func newTestClient(
	t *testing.T,
	serverVersion *version.Info,
	objs ...runtime.Object,
) (*Client, *fake.Clientset) {
	t.Helper()

	clientset := fake.NewClientset(objs...)
	if serverVersion != nil {
		disco := clientset.Discovery()

		fakeDiscovery, ok := disco.(*discoveryfake.FakeDiscovery)
		if !ok {
			t.Fatalf(
				"expected *discoveryfake.FakeDiscovery, got %T",
				clientset.Discovery(),
			)
		}

		fakeDiscovery.FakedServerVersion = serverVersion
	}

	client := &Client{
		clientset: clientset,
		discovery: clientset.Discovery(),
		context:   ktContextName,
		cluster:   ktClusterName,
	}

	return client, clientset
}

// requireNoErr fails the test immediately when err is non-nil.
func requireNoErr(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf(ktMsgErr, err)
	}
}

// requireErr fails the test when err is nil.
func requireErr(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error")
	}
}

// requireLen fails the test when got does not have the wanted length.
func requireLen[T any](t *testing.T, got []T, want int) {
	t.Helper()

	if len(got) != want {
		t.Fatalf("len = %d", len(got))
	}
}

// loadKubeConfigErr runs loadKubeConfig and returns only the error. It exists
// so tests that solely assert the error path don't need a three-blank
// assignment (which dogsled forbids).
func loadKubeConfigErr() error {
	cfg, kc, err := loadKubeConfig()
	_ = cfg
	_ = kc

	return err
}

// --- ListNamespaces / ListPods / ListServices / ListDeployments ---
// --- ListNodes / ListEvents ---

func TestClient_ListNamespaces(t *testing.T) {
	t.Parallel()
	t.Run("returns all namespaces", testListNamespacesAll)
	t.Run("empty cluster returns empty slice not nil", testListNamespacesEmpty)
	t.Run("propagates underlying error", testListNamespacesError)
}

func testListNamespacesAll(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		newNamespace(ktNamespaceDefault),
		newNamespace(ktNamespaceSystem),
	)

	got, err := client.ListNamespaces(context.Background())
	requireNoErr(t, err)
	requireLen(t, got, ktExpectedNamespaces)
}

func testListNamespacesEmpty(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil)

	got, err := client.ListNamespaces(context.Background())
	requireNoErr(t, err)
	requireLen(t, got, ktZero)
}

func testListNamespacesError(t *testing.T) {
	t.Parallel()

	client, clientset := newTestClient(t, nil)
	clientset.PrependReactor(
		"list",
		"namespaces",
		func(_ core.Action) (bool, runtime.Object, error) {
			return true, nil, errBoomKt
		},
	)

	_, err := client.ListNamespaces(context.Background())
	requireErr(t, err)
}

func TestClient_ListPods(t *testing.T) {
	t.Parallel()
	t.Run("no namespace filter -> all namespaces", testListPodsAll)
	t.Run("namespace filter -> only that namespace", testListPodsFiltered)
}

func testListPodsAll(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		ktNewPod(ktPodA, ktNamespaceDefault),
		ktNewPod("b", ktNamespaceSystem),
	)

	got, err := client.ListPods(context.Background(), ktEmpty)
	requireNoErr(t, err)
	requireLen(t, got, ktExpectedNamespaces)
}

func testListPodsFiltered(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		ktNewPod(ktPodA, ktNamespaceDefault),
		ktNewPod("b", ktNamespaceSystem),
	)

	got, err := client.ListPods(context.Background(), ktNamespaceDefault)
	requireNoErr(t, err)

	if len(got) != ktOne || got[ktZero].Name != ktPodA {
		t.Fatalf("got: %+v", got)
	}
}

func TestClient_GetPod(t *testing.T) {
	t.Parallel()
	t.Run("returns existing pod", testGetPodExisting)
	t.Run("missing pod returns IsNotFound error", testGetPodMissing)
	t.Run("returns pod from a non-default namespace", testGetPodOtherNamespace)
}

func testGetPodExisting(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil, ktNewPod(ktPodWeb, ktNamespaceDefault))

	got, err := client.GetPod(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
	)
	requireNoErr(t, err)

	if got.Name != ktPodWeb {
		t.Fatalf("name = %q", got.Name)
	}
}

func testGetPodMissing(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil)

	_, err := client.GetPod(context.Background(), ktNamespaceDefault, "missing")
	requireErr(t, err)

	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected IsNotFound, got %T: %v", err, err)
	}
}

func testGetPodOtherNamespace(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil, ktNewPod("p", ktNamespaceSystem))

	got, err := client.GetPod(context.Background(), ktNamespaceSystem, "p")
	requireNoErr(t, err)

	if got.Namespace != ktNamespaceSystem {
		t.Fatalf("ns = %q", got.Namespace)
	}
}

func TestClient_GetPodLogs(t *testing.T) {
	t.Parallel()
	t.Run("returns fake logs body (default reactor)", testGetPodLogsDefault)
	t.Run("passes container option through", testGetPodLogsContainer)
	t.Run(
		"empty container -> defaults to first spec container",
		testGetPodLogsEmptyContainer,
	)
	t.Run(
		"explicit container -> no pod lookup",
		testGetPodLogsExplicitSkipsPodLookup,
	)
	t.Run(
		"default-container annotation wins over first spec container",
		testGetPodLogsDefaultContainerAnnotation,
	)
	t.Run(
		"annotation naming unknown container -> first spec container",
		testGetPodLogsBogusAnnotationFallsBack,
	)
	t.Run(
		"annotation naming an init container is honored",
		testGetPodLogsAnnotationNamesInitContainer,
	)
	t.Run(
		"empty container + missing pod -> NotFound from pod lookup",
		testGetPodLogsMissingPodPropagatesNotFound,
	)
}

func testGetPodLogsExplicitSkipsPodLookup(t *testing.T) {
	t.Parallel(
	// The first-spec-container fallback must not cost an extra pod GET when
	// the caller already names a container.
	)

	client, clientset := newTestClient(
		t, nil,
		ktNewPod(ktPodWeb, ktNamespaceDefault),
	)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktContainerSidecar,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	for _, action := range clientset.Actions() {
		if action.GetVerb() == ktVerbGet &&
			action.GetResource().Resource == ktResourcePods &&
			action.GetSubresource() == ktEmpty {
			t.Fatal("explicit container must not trigger a pod lookup")
		}
	}
}

func testGetPodLogsMissingPodPropagatesNotFound(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	if err == nil {
		t.Fatal("expected error for missing pod")
	}

	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func testGetPodLogsDefault(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil, ktNewPod(ktPodWeb, ktNamespaceDefault))

	got, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	if got != "fake logs" {
		t.Fatalf("logs = %q", got)
	}
}

// logReactor returns a reactor that only handles the pod "log" subresource,
// stores the observed PodLogOptions through the provided callback, and
// returns the given body. Keeping it separate keeps the test bodies simple.
func logReactor(
	t *testing.T,
	body string,
	observe func(*corev1.PodLogOptions),
) core.ReactionFunc {
	t.Helper()

	return func(action core.Action) (bool, runtime.Object, error) {
		genericAction, ok := action.(core.GenericAction)
		if !ok || genericAction.GetSubresource() != ktSubresourceLog {
			return false, nil, nil
		}

		opts, ok := genericAction.GetValue().(*corev1.PodLogOptions)
		if !ok {
			t.Fatalf("unexpected value type %T", genericAction.GetValue())
		}

		observe(opts)

		return true, ktNewUnknown(body), nil
	}
}

func testGetPodLogsContainer(t *testing.T) {
	t.Parallel()

	client, clientset := newTestClient(
		t, nil,
		ktNewPod(ktPodWeb, ktNamespaceDefault),
	)

	var (
		capturedContainer string
		capturedTail      int64
	)

	observe := func(opts *corev1.PodLogOptions) {
		capturedContainer = opts.Container

		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}
	}

	clientset.PrependReactor(
		ktVerbGet,
		ktResourcePods,
		logReactor(t, "logs for "+ktContainerSidecar, observe),
	)

	got, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktContainerSidecar,
		ktSampleTailLines,
	)
	requireNoErr(t, err)

	if got != "logs for sidecar" {
		t.Fatalf("got = %q", got)
	}

	if capturedContainer != ktContainerSidecar {
		t.Fatalf("captured container = %q", capturedContainer)
	}

	if capturedTail != ktSampleTailLines {
		t.Fatalf("captured tail = %d", capturedTail)
	}
}

func testGetPodLogsEmptyContainer(t *testing.T) {
	t.Parallel(
	// Multi-container pods reject log requests without an explicit container
	// (the API server answers 400), so an empty container must fall back to
	// the first container in the pod spec.
	)

	mainContainer := ktZeroContainer
	mainContainer.Name = ktContainerMain
	sidecarContainer := ktZeroContainer
	sidecarContainer.Name = ktContainerSidecar

	pod := ktNewPod(ktPodWeb, ktNamespaceDefault)
	pod.Spec.Containers = []corev1.Container{mainContainer, sidecarContainer}

	client, clientset := newTestClient(t, nil, pod)

	var captured string

	clientset.PrependReactor(
		ktVerbGet,
		ktResourcePods,
		logReactor(t, ktBodyOK, func(opts *corev1.PodLogOptions) {
			captured = opts.Container
		}),
	)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	if captured != ktContainerMain {
		t.Fatalf(
			ktMsgContainerOpt,
			ktContainerMain,
			captured,
		)
	}
}

func testGetPodLogsDefaultContainerAnnotation(t *testing.T) {
	t.Parallel(
	// kubectl honors kubectl.kubernetes.io/default-container (mesh injectors
	// set it so tools skip the proxy sidecar); the fallback must prefer it
	// over the first spec container.
	)

	mainContainer := ktZeroContainer
	mainContainer.Name = ktContainerMain
	sidecarContainer := ktZeroContainer
	sidecarContainer.Name = ktContainerSidecar

	pod := ktNewPod(ktPodWeb, ktNamespaceDefault)
	pod.Spec.Containers = []corev1.Container{mainContainer, sidecarContainer}
	pod.Annotations = map[string]string{
		ktAnnotationDefaultContainer: ktContainerSidecar,
	}

	client, clientset := newTestClient(t, nil, pod)

	var captured string

	clientset.PrependReactor(
		ktVerbGet,
		ktResourcePods,
		logReactor(t, ktBodyOK, func(opts *corev1.PodLogOptions) {
			captured = opts.Container
		}),
	)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	if captured != ktContainerSidecar {
		t.Fatalf(
			ktMsgContainerOpt,
			ktContainerSidecar,
			captured,
		)
	}
}

func testGetPodLogsAnnotationNamesInitContainer(t *testing.T) {
	t.Parallel(
	// kubectl resolves the default-container annotation against regular,
	// init, and ephemeral containers alike (podcmd.FindContainerByName), so
	// an annotation naming a native sidecar living in initContainers must be
	// honored too.
	)

	mainContainer := ktZeroContainer
	mainContainer.Name = ktContainerMain
	sidecarContainer := ktZeroContainer
	sidecarContainer.Name = ktContainerSidecar

	pod := ktNewPod(ktPodWeb, ktNamespaceDefault)
	pod.Spec.Containers = []corev1.Container{mainContainer}
	pod.Spec.InitContainers = []corev1.Container{sidecarContainer}
	pod.Annotations = map[string]string{
		ktAnnotationDefaultContainer: ktContainerSidecar,
	}

	client, clientset := newTestClient(t, nil, pod)

	var captured string

	clientset.PrependReactor(
		ktVerbGet,
		ktResourcePods,
		logReactor(t, ktBodyOK, func(opts *corev1.PodLogOptions) {
			captured = opts.Container
		}),
	)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	if captured != ktContainerSidecar {
		t.Fatalf(ktMsgContainerOpt, ktContainerSidecar, captured)
	}
}

func testGetPodLogsBogusAnnotationFallsBack(t *testing.T) {
	t.Parallel(
	// An annotation naming a container that does not exist in the spec must
	// be ignored in favor of the first spec container.
	)

	mainContainer := ktZeroContainer
	mainContainer.Name = ktContainerMain

	pod := ktNewPod(ktPodWeb, ktNamespaceDefault)
	pod.Spec.Containers = []corev1.Container{mainContainer}
	pod.Annotations = map[string]string{
		ktAnnotationDefaultContainer: "no-such-container",
	}

	client, clientset := newTestClient(t, nil, pod)

	var captured string

	clientset.PrependReactor(
		ktVerbGet,
		ktResourcePods,
		logReactor(t, ktBodyOK, func(opts *corev1.PodLogOptions) {
			captured = opts.Container
		}),
	)

	_, err := client.GetPodLogs(
		context.Background(),
		ktNamespaceDefault,
		ktPodWeb,
		ktEmpty,
		ktDefaultTailLines,
	)
	requireNoErr(t, err)

	if captured != ktContainerMain {
		t.Fatalf(
			ktMsgContainerOpt,
			ktContainerMain,
			captured,
		)
	}
}

func TestClient_ListDeployments(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		newDeployment(ktNameAPI, ktNamespaceDefault),
		newDeployment("ctl", ktNamespaceSystem),
	)

	t.Run(ktNameNoNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListDeployments(context.Background(), ktEmpty)
		requireNoErr(t, err)
		requireLen(t, got, ktExpectedNamespaces)
	})

	t.Run(ktNameNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListDeployments(
			context.Background(),
			ktNamespaceDefault,
		)
		requireNoErr(t, err)

		if len(got) != ktOne || got[ktZero].Name != ktNameAPI {
			t.Fatalf("got: %+v", got)
		}
	})
}

func TestClient_ListServices(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		newService("svc1", ktNamespaceDefault),
		newService("svc2", ktNamespaceSystem),
	)

	t.Run(ktNameNoNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListServices(context.Background(), ktEmpty)
		requireNoErr(t, err)
		requireLen(t, got, ktExpectedNamespaces)
	})

	t.Run(ktNameNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListServices(context.Background(), ktNamespaceSystem)
		requireNoErr(t, err)

		if len(got) != ktOne || got[ktZero].Name != "svc2" {
			t.Fatalf("got: %+v", got)
		}
	})
}

func TestClient_ListNodes(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, nil, ktNewNode("n1"), ktNewNode("n2"))

	got, err := client.ListNodes(context.Background())
	requireNoErr(t, err)
	requireLen(t, got, ktExpectedNamespaces)
}

// TestClient_ListMethodsPropagateErrors covers the err-return line of every
// list method besides ListNamespaces (which is covered above). They're all
// thin "list -> if err -> return Items" wrappers, so this proves the error
// path is wired up uniformly across all of them.
func TestClient_ListMethodsPropagateErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		call     func(*Client) error
		name     string
		resource string
	}{
		{
			name:     "ListPods",
			resource: ktResourcePods,
			call: func(c *Client) error {
				_, err := c.ListPods(context.Background(), ktEmpty)

				return err
			},
		},
		{
			name:     "ListDeployments",
			resource: ktResourceDeployments,
			call: func(c *Client) error {
				_, err := c.ListDeployments(context.Background(), ktEmpty)

				return err
			},
		},
		{
			name:     "ListServices",
			resource: ktResourceServices,
			call: func(c *Client) error {
				_, err := c.ListServices(context.Background(), ktEmpty)

				return err
			},
		},
		{
			name:     "ListNodes",
			resource: ktResourceNodes,
			call: func(c *Client) error {
				_, err := c.ListNodes(context.Background())

				return err
			},
		},
		{
			name:     "ListEvents",
			resource: ktResourceEvents,
			call: func(c *Client) error {
				_, err := c.ListEvents(context.Background(), ktEmpty)

				return err
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client, clientset := newTestClient(t, nil)
			clientset.PrependReactor(
				"list",
				testCase.resource,
				func(_ core.Action) (bool, runtime.Object, error) {
					return true, nil, errBackendDown
				},
			)

			err := testCase.call(client)
			if err == nil {
				t.Fatalf("%s: expected error", testCase.name)
			}
		})
	}
}

func TestClient_ListEvents(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t, nil,
		newEvent("e1", ktNamespaceDefault, ktReasonScheduled),
		newEvent("e2", ktNamespaceSystem, "Pulled"),
	)

	t.Run(ktNameNoNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListEvents(context.Background(), ktEmpty)
		requireNoErr(t, err)
		requireLen(t, got, ktExpectedNamespaces)
	})

	t.Run(ktNameNSFilter, func(t *testing.T) {
		t.Parallel()

		got, err := client.ListEvents(context.Background(), ktNamespaceDefault)
		requireNoErr(t, err)

		if len(got) != ktOne || got[ktZero].Name != "e1" {
			t.Fatalf("got: %+v", got)
		}
	})
}

// --- loadKubeConfig (filesystem-backed) ---

func TestLoadKubeConfig_HappyPath(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	dir := t.TempDir()
	path := filepath.Join(dir, ktConfigFileName)
	mustWrite(t, path, `apiVersion: v1
kind: Config
current-context: my-ctx
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: my-cluster
contexts:
- context:
    cluster: my-cluster
    user: me
  name: my-ctx
users:
- name: me
  user:
    token: fake
`)
	t.Setenv(ktEnvKubeconfig, path)

	cfg, kc, err := loadKubeConfig()
	requireNoErr(t, err)

	if cfg == nil {
		t.Fatal("nil rest config")
	}

	if kc.contextName != "my-ctx" {
		t.Fatalf("ctx = %q", kc.contextName)
	}

	if kc.clusterName != "my-cluster" {
		t.Fatalf("cluster = %q", kc.clusterName)
	}
}

func TestLoadKubeConfig_MissingFile(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, filepath.Join(t.TempDir(), "does-not-exist"))

	if loadKubeConfigErr() == nil {
		t.Fatal("expected error for missing kubeconfig")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()

	err := os.WriteFile(path, []byte(body), ktFileModeFile)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- NewClient (filesystem + config plumbing) ---

func TestNewClient_FromKubeconfig(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	dir := t.TempDir()
	path := filepath.Join(dir, ktConfigFileName)
	mustWrite(t, path, `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: c1
contexts:
- context:
    cluster: c1
    user: u
  name: ctx
users:
- name: u
  user:
    token: fake
`)
	t.Setenv(ktEnvKubeconfig, path)

	client, err := NewClient()
	requireNoErr(t, err)

	if client.context != "ctx" || client.cluster != "c1" {
		t.Fatalf("c = %+v", client)
	}

	if client.clientset == nil || client.discovery == nil {
		t.Fatalf("client fields nil: %+v", client)
	}
}

// TestLoadKubeConfig_DefaultPath covers the branch where KUBECONFIG is unset
// and loadKubeConfig falls back to ~/.kube/config. We don't expect the load
// to succeed (the test runner's HOME might not point at a real cluster), but
// the *path-resolution* code is exercised either way.
func TestLoadKubeConfig_DefaultPath(t *testing.T) {
	// NOTE: mutates HOME/KUBECONFIG via t.Setenv, so this test cannot run in
	// parallel with others that read those variables.
	home := t.TempDir()
	kube := home + "/.kube"

	err := os.MkdirAll(kube, ktDirModePrivate)
	if err != nil {
		t.Fatal(err)
	}

	mustWrite(t, kube+"/config", `apiVersion: v1
kind: Config
current-context: c
clusters:
- cluster: {server: https://127.0.0.1:6443, insecure-skip-tls-verify: true}
  name: cl
contexts:
- context: {cluster: cl, user: u}
  name: c
users:
- name: u
  user: {token: fake}
`)
	t.Setenv(ktEnvKubeconfig, ktEmpty)
	t.Setenv("HOME", home)

	_, kc, err := loadKubeConfig()
	requireNoErr(t, err)

	if kc.contextName != "c" || kc.clusterName != "cl" {
		t.Fatalf("ctx=%q cluster=%q", kc.contextName, kc.clusterName)
	}
}

// TestLoadKubeConfig_HomeUnset covers the branch where KUBECONFIG is unset
// *and* HOME is unset — kubeconfigPath stays empty and clientcmd produces a
// build-rest-config error.
func TestLoadKubeConfig_HomeUnset(t *testing.T) {
	// NOTE: mutates HOME/KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, ktEmpty)
	t.Setenv("HOME", ktEmpty)

	if loadKubeConfigErr() == nil {
		t.Fatal("expected error when neither KUBECONFIG nor HOME is set")
	}
}

func TestNewClient_BadKubeconfig(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	dir := t.TempDir()
	path := filepath.Join(dir, ktConfigFileName)
	mustWrite(t, path, "this is not yaml: [[[")
	t.Setenv(ktEnvKubeconfig, path)

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestNewClient_InvalidServerURLBreaksClientsetBuild covers the rare
// kubernetes.NewForConfig error path. We craft a kubeconfig whose server is
// not a parseable URL — clientcmd's ClientConfig() builds the *rest.Config
// successfully, then kubernetes.NewForConfig fails when it tries to derive
// the server URL.
func TestNewClient_InvalidServerURLBreaksClientsetBuild(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	dir := t.TempDir()
	path := filepath.Join(dir, ktConfigFileName)
	mustWrite(t, path, `apiVersion: v1
kind: Config
current-context: c
clusters:
- cluster:
    server: "://not-a-url"
  name: cl
contexts:
- context: {cluster: cl, user: u}
  name: c
users:
- name: u
  user: {token: fake}
`)
	t.Setenv(ktEnvKubeconfig, path)

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error from clientset build with invalid server URL")
	}
}

// --- GetClusterInfo ---

func TestClient_GetClusterInfo(t *testing.T) {
	t.Parallel()
	t.Run(
		"aggregates version, platform, node count, context, cluster",
		testGetClusterInfoAggregates,
	)
	t.Run("zero-node cluster reports NodeCount=0", testGetClusterInfoZeroNodes)
	t.Run("propagates server-version error", testGetClusterInfoVersionErr)
	t.Run("propagates list-nodes error", testGetClusterInfoListNodesErr)
}

func testGetClusterInfoAggregates(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(
		t,
		newVersionInfo(ktVersionGit, ktPlatformAMD64),
		ktNewNode("n1"),
		ktNewNode("n2"),
		ktNewNode("n3"),
	)

	info, err := client.GetClusterInfo(context.Background())
	requireNoErr(t, err)

	if info.Version != ktVersionGit {
		t.Fatalf("version = %q", info.Version)
	}

	if info.Platform != ktPlatformAMD64 {
		t.Fatalf("platform = %q", info.Platform)
	}

	if info.NodeCount != ktThreeNodes {
		t.Fatalf("node count = %d", info.NodeCount)
	}

	if info.Context != ktContextName || info.ClusterName != ktClusterName {
		t.Fatalf("context/cluster wrong: %+v", info)
	}
}

func testGetClusterInfoZeroNodes(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, newVersionInfo(ktVersionGit, ktPlatformAMD64))

	info, err := client.GetClusterInfo(context.Background())
	requireNoErr(t, err)

	if info.NodeCount != ktZero {
		t.Fatalf("node count = %d", info.NodeCount)
	}
}

func testGetClusterInfoVersionErr(t *testing.T) {
	t.Parallel()

	client := &Client{
		clientset: fake.NewClientset(),
		discovery: errDiscovery{
			DiscoveryInterface: nil,
			err:                errAPIUnreach,
		},
		context: ktEmpty,
		cluster: ktEmpty,
	}

	_, err := client.GetClusterInfo(context.Background())
	requireErr(t, err)
}

func testGetClusterInfoListNodesErr(t *testing.T) {
	t.Parallel()

	client, clientset := newTestClient(
		t,
		newVersionInfo(ktVersionGit, ktPlatformAMD64),
	)
	clientset.PrependReactor(
		"list",
		ktResourceNodes,
		func(_ core.Action) (bool, runtime.Object, error) {
			return true, nil, errNodesDown
		},
	)

	_, err := client.GetClusterInfo(context.Background())
	requireErr(t, err)
}
