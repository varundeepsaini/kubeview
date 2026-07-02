package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	core "k8s.io/client-go/testing"
)

// Numeric constants. The revive add-constant rule flags bare magic numbers,
// so the values used in assertions are named here. The "ht" prefix avoids
// collisions with constants declared in sibling test files.
const (
	htStatusOK     = http.StatusOK
	htStatusForbid = http.StatusForbidden
	htStatusNotF   = http.StatusNotFound
	htStatusErr    = http.StatusInternalServerError

	htRedirectFloor = 300

	htZero  = 0
	htFirst = 0
	htOne   = 1
	htTwo   = 2

	htUnsetTail = -1

	htPort80      = 80
	htDefaultTail = 100
	htTail500     = 500

	htConcurrentReqs = 50
	htLogConcurrency = 30
	htBigLogRepeats  = 4096

	htClockSkewWindow       = 5 * time.Second
	htRequestTimeout        = 2 * time.Second
	htSingleResourceSlashes = 4
)

// Static sentinel errors. err113 forbids dynamic errors, so the test reactors
// and helpers wrap these instead of building errors inline.
var (
	errBoom        = errors.New("boom")
	errPlain       = errors.New("just a plain error")
	errDenied      = errors.New("denied")
	errTransient   = errors.New("transient backend error")
	errBackend     = errors.New("backend down")
	errBrokenPipe  = errors.New("broken pipe")
	errBadStatus   = errors.New("unexpected status")
	errPodCount    = errors.New("unexpected pod count")
	errLogMismatch = errors.New("log body mismatch")
)

// String constants for repeated literals (goconst / revive add-constant).
const (
	htPathHealth      = "/api/health"
	htPathCluster     = "/api/cluster"
	htPathNamespaces  = "/api/namespaces"
	htPathPods        = "/api/pods"
	htPathDeployments = "/api/deployments"
	htPathServices    = "/api/services"
	htPathNodes       = "/api/nodes"
	htPathEvents      = "/api/events"
	htPathPodDetail   = "/api/pods/default/web"
	htPathPDetail     = "/api/pods/default/p"
	htPathWebLogs     = "/api/pods/default/web/logs"
	htPathPLogs       = "/api/pods/default/p/logs"

	htNSDefault    = "default"
	htNSKubeSystem = "kube-system"

	htNameWeb = "web"
	htNameAPI = "api"
	htNameSvc = "svc"
	htNameP   = "p"
	htNameA   = "a"
	htNameB   = "b"
	htNameC   = "c"

	htImageNginx  = "nginx:1"
	htImageAPI    = "api:1"
	htContNginx   = "nginx"
	htContSidecar = "sidecar"

	htPodIP     = "10.0.0.1"
	htClusterIP = "1.2.3.4"

	htPlatformLinux = "linux/amd64"
	htTestCluster   = "test-cluster"
	htTestContext   = "test-context"
	htVersionV1     = "v1"
	htVersionV130   = "v1.30.0"
	htPlatformP     = "p"

	htStatusActive = "Active"
	htStatusReady  = "Ready"
	htTypeNormal   = "Normal"
	htReasonSched  = "Scheduled"
	htKindPod      = "Pod"

	htLabelEnv = "env"
	htLabelApp = "app"

	htResPods        = "pods"
	htResDeployments = "deployments"
	htResServices    = "services"
	htResNodes       = "nodes"
	htResEvents      = "events"
	htResNamespaces  = "namespaces"

	htVerbGet  = "get"
	htVerbList = "list"
	htSubLog   = "log"

	htEmpty       = ""
	htLogsHello   = "hello"
	htLogsOK      = "ok"
	htLogsEmpty   = `"logs":""`
	htContentJSON = "application/json"
	htACAOValue   = "http://localhost:5500"

	htOriginExample = "https://kubeview.example.com"
	htHdrACAO       = "Access-Control-Allow-Origin"
	htOriginEvil    = "https://evil.example.com"

	htMsgStatus     = "status = %d"
	htMsgStatusBody = "status = %d, body = %s"
	htMsgNewReq     = "new request: %v"
	htMsgOrigins    = "origins = %v"
	htMsgDecode     = "decode: %v"

	htPodNotFoundMsg = "Pod not found"
)

// logsBody is the response shape for the pod-logs endpoint. It carries an
// explicit json tag so musttag is satisfied when decoding.
type logsBody struct {
	Logs string `json:"logs"`
}

// httpResult captures the parts of an HTTP response the tests inspect. The
// helpers that build it close the response body internally, so no open
// *http.Response escapes (keeping bodyclose satisfied at call sites).
type httpResult struct {
	header     http.Header
	body       []byte
	statusCode int
}

// newTestServer wires the project's real router (and CORS middleware) up to a
// fake-backed Client and starts an httptest.Server. The cleanup function is
// registered with t.Cleanup.
func newTestServer(
	t *testing.T,
	sv *version.Info,
	objs ...runtime.Object,
) (*httptest.Server, *Client) {
	t.Helper()

	c, _ := newTestClient(t, sv, objs...)
	srv := httptest.NewServer(withCORS(newRouter(c), parseCORSOrigins(htEmpty)))
	t.Cleanup(srv.Close)

	return srv, c
}

// doRequest issues a request with the given method, reads and closes the body,
// and returns the captured result.
func doRequest(t *testing.T, method, url string) httpResult {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), method, url, nil,
	)
	if err != nil {
		t.Fatalf(htMsgNewReq, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}

	body, err := io.ReadAll(resp.Body)

	cerr := resp.Body.Close()

	if err != nil {
		t.Fatalf("read body %s: %v", url, err)
	}

	if cerr != nil {
		t.Fatalf("close body %s: %v", url, cerr)
	}

	return httpResult{
		statusCode: resp.StatusCode,
		header:     resp.Header,
		body:       body,
	}
}

// httpGet issues a GET and returns the captured result.
func httpGet(t *testing.T, url string) httpResult {
	t.Helper()

	return doRequest(t, http.MethodGet, url)
}

// getJSON issues a GET against the server and, for success responses, decodes
// the body into dst.
func getJSON(
	t *testing.T,
	srv *httptest.Server,
	path string,
	dst any,
) httpResult {
	t.Helper()

	res := httpGet(t, srv.URL+path)
	if dst == nil || res.statusCode >= htRedirectFloor {
		return res
	}

	derr := json.Unmarshal(res.body, dst)
	if derr != nil {
		t.Fatalf("decode %s: %v\nbody: %s", path, derr, res.body)
	}

	return res
}

// --- fixture builders (avoid exhaustruct findings on k8s literals) ---

func newPod(name, ns string) *corev1.Pod {
	pod := new(corev1.Pod)
	pod.Name = name
	pod.Namespace = ns

	return pod
}

func newContainer(name, image string) corev1.Container {
	c := new(corev1.Container)
	c.Name = name
	c.Image = image

	return *c
}

func newRunningPod(name, ns string) *corev1.Pod {
	pod := newPod(name, ns)
	pod.Spec.Containers = []corev1.Container{newContainer(htNameC, htEmpty)}
	pod.Status.Phase = corev1.PodRunning

	return pod
}

func newNode(name string) *corev1.Node {
	node := new(corev1.Node)
	node.Name = name

	return node
}

func newVersion(git, platform string) *version.Info {
	info := new(version.Info)
	info.GitVersion = git
	info.Platform = platform

	return info
}

func newUnknown(raw []byte) *runtime.Unknown {
	u := new(runtime.Unknown)
	u.Raw = raw

	return u
}

// --- /api/health ---

func TestHandle_Health(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	var out map[string]string

	res := getJSON(t, srv, htPathHealth, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if out["status"] != "ok" {
		t.Fatalf("status field = %q", out["status"])
	}
	// timestamp is RFC3339; parse round-trips.
	_, err := time.Parse(time.RFC3339, out["timestamp"])
	if err != nil {
		t.Fatalf("timestamp not RFC3339: %q (%v)", out["timestamp"], err)
	}
}

// --- /api/cluster ---

func TestHandle_Cluster(t *testing.T) {
	t.Parallel()
	t.Run("happy path", clusterHappyPath)
	t.Run("error from kube returns 500", clusterErrorReturns500)
}

func clusterHappyPath(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(
		t,
		newVersion(htVersionV130, htPlatformLinux),
		newNode("n1"),
	)

	var info ClusterInfo

	res := getJSON(t, srv, htPathCluster, &info)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if info.Version != htVersionV130 ||
		info.Platform != htPlatformLinux ||
		info.NodeCount != htOne {
		t.Fatalf("info = %+v", info)
	}

	if info.Context != htTestContext || info.ClusterName != htTestCluster {
		t.Fatalf("context/cluster wrong: %+v", info)
	}
}

func clusterErrorReturns500(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, newVersion(htVersionV1, htPlatformP))
	cs := c.clientset
	// Reactors only work on the fake clientset, reached via Client.
	injectListNodesError(t, cs, errBoom)

	res := getJSON(t, srv, htPathCluster, nil)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	err := json.Unmarshal(res.body, &e)
	if err != nil {
		t.Fatalf(htMsgDecode, err)
	}

	if e.Status != htStatusErr {
		t.Fatalf("err response = %+v", e)
	}
	// 5xx responses must NOT leak the raw internal error to the client.
	if strings.Contains(e.Error, "boom") {
		t.Fatalf("5xx body leaked internal error detail: %q", e.Error)
	}

	if e.Error != "Internal server error" {
		t.Fatalf("err message = %q, want generic 5xx message", e.Error)
	}
}

type errResp struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

// --- /api/namespaces ---

func TestHandle_Namespaces(t *testing.T) {
	t.Parallel()

	ns := new(corev1.Namespace)
	ns.Name = htNSDefault
	ns.Labels = map[string]string{htLabelEnv: "prod"}
	ns.Status.Phase = corev1.NamespaceActive

	srv, _ := newTestServer(t, nil, ns)

	var out []Namespace

	res := getJSON(t, srv, htPathNamespaces, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne {
		t.Fatalf("len = %d", len(out))
	}

	if out[htFirst].Name != htNSDefault ||
		out[htFirst].Status != htStatusActive ||
		out[htFirst].Labels[htLabelEnv] != "prod" {
		t.Fatalf("ns = %+v", out[htFirst])
	}
}

// --- /api/pods, /api/pods/{ns}/{name}, /api/pods/{ns}/{name}/logs ---

func TestHandle_Pods_List(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(
		t, nil,
		newRunningPod(htNameA, htNSDefault),
		newRunningPod(htNameB, htNSKubeSystem),
	)

	t.Run("all namespaces", podsListAll(srv))
	t.Run("filtered by namespace", podsListFiltered(srv))
	t.Run("returns [] for empty list, never null", podsListEmpty)
}

func podsListAll(srv *httptest.Server) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		var out []Pod

		res := getJSON(t, srv, htPathPods, &out)
		if res.statusCode != htStatusOK {
			t.Fatalf(htMsgStatus, res.statusCode)
		}

		if len(out) != htTwo {
			t.Fatalf("len = %d", len(out))
		}
	}
}

func podsListFiltered(srv *httptest.Server) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		var out []Pod

		res := getJSON(t, srv, htPathPods+"?namespace="+htNSDefault, &out)
		if res.statusCode != htStatusOK {
			t.Fatalf(htMsgStatus, res.statusCode)
		}

		if len(out) != htOne || out[htFirst].Name != htNameA {
			t.Fatalf("got: %+v", out)
		}
	}
}

func podsListEmpty(t *testing.T) {
	t.Parallel()

	srv2, _ := newTestServer(t, nil)

	res := getJSON(t, srv2, htPathPods, nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if strings.TrimSpace(string(res.body)) != "[]" {
		t.Fatalf("body = %q, want []", res.body)
	}
}

func TestHandle_Pod_Detail(t *testing.T) {
	t.Parallel()
	t.Run("returns pod when present", podDetailPresent)
	t.Run("missing pod returns 404 friendly", podDetailMissing)
}

func podDetailPresent(t *testing.T) {
	t.Parallel()

	pod := newPod(htNameWeb, htNSDefault)
	pod.Spec.Containers = []corev1.Container{
		newContainer(htContNginx, htImageNginx),
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = htPodIP

	srv, _ := newTestServer(t, nil, pod)

	var p Pod

	res := getJSON(t, srv, htPathPodDetail, &p)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if p.Name != htNameWeb || p.Namespace != htNSDefault || p.IP != htPodIP {
		t.Fatalf("pod = %+v", p)
	}
}

func podDetailMissing(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := getJSON(t, srv, "/api/pods/default/missing", nil)
	if res.statusCode != htStatusNotF {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	err := json.Unmarshal(res.body, &e)
	if err != nil {
		t.Fatalf(htMsgDecode, err)
	}

	if e.Error != htPodNotFoundMsg || e.Status != htStatusNotF {
		t.Fatalf("err = %+v", e)
	}
}

func TestHandle_PodLogs(t *testing.T) {
	t.Parallel()
	t.Run("returns logs object with default tailLines", podLogsDefaultTail)
	t.Run("respects container query param", podLogsContainerParam)
	t.Run("respects tailLines query param", podLogsTailParam)
	t.Run("invalid tailLines falls back to default 100", podLogsInvalidTail)
	t.Run("missing pod -> 404", podLogsMissing)
	t.Run("response always includes logs field", podLogsAlwaysLogsField)
}

func podLogsDefaultTail(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))

	var capturedTail int64

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}

		return []byte(htLogsHello)
	})

	var out logsBody

	res := getJSON(t, srv, htPathWebLogs, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if out.Logs != htLogsHello {
		t.Fatalf("logs = %q", out.Logs)
	}

	if capturedTail != htDefaultTail {
		t.Fatalf("default tailLines = %d, want 100", capturedTail)
	}
}

func podLogsContainerParam(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))

	var captured string

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		captured = opts.Container

		return []byte(htLogsOK)
	})

	var out logsBody

	path := htPathWebLogs + "?container=" + htContSidecar

	res := getJSON(t, srv, path, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if captured != htContSidecar {
		t.Fatalf("container = %q", captured)
	}
}

func podLogsTailParam(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))

	var capturedTail int64

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}

		return []byte("")
	})

	res := getJSON(t, srv, htPathWebLogs+"?tailLines=500", nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if capturedTail != htTail500 {
		t.Fatalf("tailLines = %d", capturedTail)
	}
}

func podLogsInvalidTail(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))

	var capturedTail int64

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}

		return []byte("")
	})

	for _, raw := range []string{"abc", "-5", "0"} {
		capturedTail = htUnsetTail
		path := htPathWebLogs + "?tailLines=" + raw

		res := getJSON(t, srv, path, nil)
		if res.statusCode != htStatusOK {
			t.Fatalf("status for %q = %d", raw, res.statusCode)
		}

		if capturedTail != htDefaultTail {
			t.Fatalf("tailLines for %q = %d, want 100", raw, capturedTail)
		}
	}
}

func podLogsMissing(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	cs := c.clientset
	notFound := apierrors.NewNotFound(corev1.Resource(htResPods), "nope")
	injectLogsErrorReactor(t, cs, notFound)

	res := getJSON(t, srv, "/api/pods/default/nope/logs", nil)
	if res.statusCode != htStatusNotF {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	uerr := json.Unmarshal(res.body, &e)
	if uerr != nil {
		t.Fatalf(htMsgDecode, uerr)
	}

	if e.Error != htPodNotFoundMsg || e.Status != htStatusNotF {
		t.Fatalf("err = %+v", e)
	}
}

func podLogsAlwaysLogsField(t *testing.T) {
	t.Parallel()
	// The JS server returns `{ logs: logs || "" }`, so the frontend can
	// always read `.logs` without a null check. Mirror that.
	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte {
		return []byte("")
	})

	res := getJSON(t, srv, htPathWebLogs, nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if !strings.Contains(string(res.body), htLogsEmpty) {
		t.Fatalf("body = %q (expected logs:\"\")", res.body)
	}
}

// --- /api/deployments ---

func TestHandle_Deployments(t *testing.T) {
	t.Parallel()

	dep := new(appsv1.Deployment)
	dep.Name = htNameAPI
	dep.Namespace = htNSDefault
	sel := new(metav1.LabelSelector)
	sel.MatchLabels = map[string]string{htLabelApp: htNameAPI}
	dep.Spec.Selector = sel
	dep.Spec.Template.Spec.Containers = []corev1.Container{
		newContainer(htNameAPI, htImageAPI),
	}

	srv, _ := newTestServer(t, nil, dep)

	var out []Deployment

	res := getJSON(t, srv, htPathDeployments+"?namespace="+htNSDefault, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne ||
		out[htFirst].Name != htNameAPI ||
		len(out[htFirst].Images) != htOne ||
		out[htFirst].Images[htFirst] != htImageAPI {
		t.Fatalf("deployments = %+v", out)
	}
}

// --- /api/services ---

func TestHandle_Services(t *testing.T) {
	t.Parallel()

	port := new(corev1.ServicePort)
	port.Port = htPort80
	port.Protocol = corev1.ProtocolTCP

	svc := new(corev1.Service)
	svc.Name = htNameSvc
	svc.Namespace = htNSDefault
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.ClusterIP = htClusterIP
	svc.Spec.Ports = []corev1.ServicePort{*port}

	srv, _ := newTestServer(t, nil, svc)

	var out []Service

	res := getJSON(t, srv, htPathServices, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].ClusterIP != htClusterIP {
		t.Fatalf("services = %+v", out)
	}
}

// --- /api/nodes ---

func TestHandle_Nodes(t *testing.T) {
	t.Parallel()

	cond := new(corev1.NodeCondition)
	cond.Type = corev1.NodeReady
	cond.Status = corev1.ConditionTrue

	node := newNode("n1")
	node.Status.Conditions = []corev1.NodeCondition{*cond}

	srv, _ := newTestServer(t, nil, node)

	var out []NodeInfo

	res := getJSON(t, srv, htPathNodes, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].Status != htStatusReady {
		t.Fatalf("nodes = %+v", out)
	}
}

// --- /api/events ---

func TestHandle_Events(t *testing.T) {
	t.Parallel()

	ref := new(corev1.ObjectReference)
	ref.Kind = htKindPod
	ref.Name = "x"

	event := new(corev1.Event)
	event.Name = "e1"
	event.Namespace = htNSDefault
	event.Type = htTypeNormal
	event.Reason = htReasonSched
	event.Message = "scheduled"
	event.InvolvedObject = *ref
	event.Count = htZero

	srv, _ := newTestServer(t, nil, event)

	var out []KubeEvent

	res := getJSON(t, srv, htPathEvents, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htOne || out[htFirst].Object != "Pod/x" {
		t.Fatalf("events = %+v", out)
	}
	// count 0 should be coerced to 1 (matches JS `event.count || 1`).
	if out[htFirst].Count != htOne {
		t.Fatalf("count = %d", out[htFirst].Count)
	}
}

// --- CORS ---

func TestCORS(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)
	t.Run("GET response has CORS headers", corsGETHeaders(srv))
	t.Run("OPTIONS preflight returns 204", corsOptionsPreflight(srv))
}

func corsGETHeaders(srv *httptest.Server) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		res := httpGet(t, srv.URL+htPathHealth)

		got := res.header.Get(htHdrACAO)
		if got != htACAOValue {
			t.Fatalf("ACAO = %q", got)
		}
	}
}

func corsOptionsPreflight(srv *httptest.Server) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		res := doRequest(t, http.MethodOptions, srv.URL+htPathPods)
		if res.statusCode != http.StatusNoContent {
			t.Fatalf(htMsgStatus, res.statusCode)
		}

		assertPreflightHeaders(t, res)

		if len(res.body) != htZero {
			t.Fatalf("expected empty body, got %q", res.body)
		}
	}
}

func assertPreflightHeaders(t *testing.T, res httpResult) {
	t.Helper()

	if res.header.Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("ACAM missing")
	}

	if res.header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("ACAH missing")
	}
}

func TestParseCORSOrigins_EmptyYieldsDevDefault(t *testing.T) {
	t.Parallel()

	got := parseCORSOrigins(htEmpty)
	if len(got) != htOne || got[htZero] != htACAOValue {
		t.Fatalf(htMsgOrigins, got)
	}
}

func TestParseCORSOrigins_CommaListSplitAndTrimmed(t *testing.T) {
	t.Parallel()

	got := parseCORSOrigins(htOriginExample + " , " + htACAOValue)
	if len(got) != htTwo {
		t.Fatalf(htMsgOrigins, got)
	}

	if got[htZero] != htOriginExample || got[htOne] != htACAOValue {
		t.Fatalf(htMsgOrigins, got)
	}
}

func TestCORS_ConfiguredOrigins(t *testing.T) {
	t.Parallel(
	// With CORS_ORIGIN configured to several origins, a request whose Origin
	// header matches any of them gets that origin echoed back; anything else
	// falls back to the first configured origin so browsers block it.
	)

	origins := []string{htOriginExample, htACAOValue}
	srv := httptest.NewServer(withCORS(newRouter(nil), origins))
	t.Cleanup(srv.Close)

	cases := []struct {
		name       string
		origin     string
		wantHeader string
	}{
		{"second configured origin echoed", htACAOValue, htACAOValue},
		{"first configured origin echoed", htOriginExample, htOriginExample},
		{"unknown origin falls back to first", htOriginEvil, htOriginExample},
		{"no origin header falls back to first", htEmpty, htOriginExample},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := preflightACAO(t, srv.URL+htPathHealth, tc.origin)
			if got != tc.wantHeader {
				t.Fatalf("ACAO = %q, want %q", got, tc.wantHeader)
			}
		})
	}
}

// preflightACAO issues an OPTIONS request with the given Origin header (when
// non-empty) and returns the Access-Control-Allow-Origin response header.
func preflightACAO(t *testing.T, url, origin string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodOptions, url, nil,
	)
	if err != nil {
		t.Fatalf(htMsgNewReq, err)
	}

	if origin != htEmpty {
		req.Header.Set("Origin", origin)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}

	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			t.Errorf("close: %v", cerr)
		}
	}()

	return resp.Header.Get(htHdrACAO)
}

// --- error mapping ---

func TestErrorMapping_StatusErrorPropagatesCode(t *testing.T) {
	t.Parallel()
	// Build a Client whose underlying clientset returns an
	// apierrors.NewForbidden for namespaces — handler propagates 403.
	srv, c := newTestServer(t, nil)
	cs := c.clientset
	gr := corev1.Resource(htResNamespaces)
	forbidden := apierrors.NewForbidden(gr, "x", errDenied)
	injectListNamespacesError(t, cs, forbidden)

	res := getJSON(t, srv, htPathNamespaces, nil)
	if res.statusCode != htStatusForbid {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	uerr := json.Unmarshal(res.body, &e)
	if uerr != nil {
		t.Fatalf(htMsgDecode, uerr)
	}

	if e.Status != htStatusForbid {
		t.Fatalf("status field = %d", e.Status)
	}
}

func TestErrorMapping_NonStatusErrorBecomes500(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	cs := c.clientset
	injectListNamespacesError(t, cs, errPlain)

	res := getJSON(t, srv, htPathNamespaces, nil)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

// --- method gating ---

func TestRouter_OnlyAllowsGET(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		res := doRequest(t, method, srv.URL+htPathPods)
		// http.ServeMux pattern "GET /api/pods" rejects other methods with
		// 405 Method Not Allowed by default in Go 1.22+.
		if res.statusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s -> %d, want 405", method, res.statusCode)
		}
	}
}

// --- reactor helpers (private to handlers_test) ---

// The fake clientset is parameterised by kubernetes.Interface in the
// production code so tests reach the concrete *fake.Clientset through an
// interface. The helpers below centralise the reactor wiring.

type fakeReactor interface {
	PrependReactor(verb, resource string, reaction core.ReactionFunc)
}

// asFakeReactor asserts x to a fakeReactor and applies fn to it.
func asFakeReactor(t *testing.T, x any, fn func(fakeReactor)) {
	t.Helper()

	r, ok := x.(fakeReactor)
	if !ok {
		t.Fatalf("clientset is not a fake reactor, got %T", x)
	}

	fn(r)
}

func injectListNodesError(t *testing.T, cs any, err error) {
	t.Helper()
	injectResourceError(t, cs, htVerbList, htResNodes, err)
}

func injectListNamespacesError(t *testing.T, cs any, err error) {
	t.Helper()
	injectResourceError(t, cs, htVerbList, htResNamespaces, err)
}

func injectGetPodsError(t *testing.T, cs any, retErr error) {
	t.Helper()
	injectResourceError(t, cs, htVerbGet, htResPods, retErr)
}

func injectResourceError(
	t *testing.T,
	cs any,
	verb, resource string,
	retErr error,
) {
	t.Helper()
	asFakeReactor(t, cs, func(r fakeReactor) {
		r.PrependReactor(
			verb, resource,
			func(core.Action) (bool, runtime.Object, error) {
				return true, nil, retErr
			},
		)
	})
}

// logReaction builds a reactor that only handles the pod "log" subresource and
// delegates the matched case to handle.
func logReaction(
	handle func(core.GenericAction) (bool, runtime.Object, error),
) core.ReactionFunc {
	return func(action core.Action) (bool, runtime.Object, error) {
		g, ok := action.(core.GenericAction)
		if !ok || g.GetSubresource() != htSubLog {
			return false, nil, nil
		}

		return handle(g)
	}
}

// injectLogsReactor wires a reactor that returns the bytes produced by the
// callback for any GetLogs request.
func injectLogsReactor(
	t *testing.T,
	cs any,
	fn func(*corev1.PodLogOptions) []byte,
) {
	t.Helper()
	asFakeReactor(t, cs, func(r fakeReactor) {
		r.PrependReactor(htVerbGet, htResPods, logReaction(
			func(g core.GenericAction) (bool, runtime.Object, error) {
				opts, ok := g.GetValue().(*corev1.PodLogOptions)
				if !ok {
					return false, nil, nil
				}

				return true, newUnknown(fn(opts)), nil
			},
		))
	})
}

func injectLogsErrorReactor(t *testing.T, cs any, retErr error) {
	t.Helper()
	asFakeReactor(t, cs, func(r fakeReactor) {
		r.PrependReactor(htVerbGet, htResPods, logReaction(
			func(core.GenericAction) (bool, runtime.Object, error) {
				return true, nil, retErr
			},
		))
	})
}

// --- additional handler tests --------------------------------------------

// TestRouter_UnknownRouteReturns404 verifies Go 1.22+ ServeMux behavior: a
// pattern-mismatched path returns 404 from the mux itself (no handler runs).
func TestRouter_UnknownRouteReturns404(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)
	paths := []string{"/", "/api/unknown", "/api/pods/x", "/wrong"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			unknownRouteCase(t, srv, path)
		})
	}
}

func unknownRouteCase(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	res := httpGet(t, srv.URL+path)
	if res.statusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.statusCode)
	}
}

// TestRouter_TrailingSlashRejected is documented behavior of net/http's
// ServeMux for non-rooted patterns: /api/pods registers an exact match.
// /api/pods/ would match a different (more-specific) pattern only if one
// exists. We don't register any trailing-slash patterns, so /api/pods/ 404s.
func TestRouter_TrailingSlashRejected(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := httpGet(t, srv.URL+"/api/pods/")
	if res.statusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.statusCode)
	}
}

// TestHandle_ConcurrentRequests fires N parallel GETs at the server and
// asserts every one returns 200 with the right shape.
func TestHandle_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(
		t, nil,
		newRunningPod(htNameA, htNSDefault),
		newRunningPod(htNameB, htNSDefault),
	)

	errs := make(chan error, htConcurrentReqs)

	var wg sync.WaitGroup

	for range htConcurrentReqs {
		wg.Go(func() {
			errs <- concurrentPodsGet(srv)
		})
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		if e != nil {
			t.Errorf("concurrent request failed: %v", e)
		}
	}
}

// fetch issues a GET, reads the whole body, and closes it, returning the
// status code and bytes. It is used by the concurrency tests, where t.Fatalf
// from a non-test goroutine is not allowed.
func fetch(url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, url, nil,
	)
	if err != nil {
		return htZero, nil, fmt.Errorf("new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return htZero, nil, fmt.Errorf("do request: %w", err)
	}

	body, rerr := io.ReadAll(resp.Body)

	cerr := resp.Body.Close()

	if rerr != nil {
		return htZero, nil, fmt.Errorf("read body: %w", rerr)
	}

	if cerr != nil {
		return htZero, nil, fmt.Errorf("close body: %w", cerr)
	}

	return resp.StatusCode, body, nil
}

func concurrentPodsGet(srv *httptest.Server) error {
	status, body, err := fetch(srv.URL + htPathPods)
	if err != nil {
		return err
	}

	if status != htStatusOK {
		return fmt.Errorf("%w: %d", errBadStatus, status)
	}

	var out []Pod

	derr := json.Unmarshal(body, &out)
	if derr != nil {
		return fmt.Errorf("decode: %w", derr)
	}

	if len(out) != htTwo {
		return fmt.Errorf("%w: got %d", errPodCount, len(out))
	}

	return nil
}

// TestHandle_ContextPropagation verifies that a client whose context is
// already canceled at issue time produces a quick failure, not a hang.
func TestHandle_ContextPropagation(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+htPathPods, nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	done := make(chan error, htOne)

	go func() {
		resp, derr := http.DefaultClient.Do(req)
		if derr == nil && resp != nil {
			done <- resp.Body.Close()

			return
		}

		done <- derr
	}()

	select {
	case <-done:
	case <-time.After(htRequestTimeout):
		t.Fatal("request did not abort despite a pre-canceled context")
	}
}

// TestHandle_PodLogs_EmptyBytes confirms the response shape when the K8s API
// returns zero log bytes — the handler must still emit {"logs":""}.
func TestHandle_PodLogs_EmptyBytes(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameP, htNSDefault))
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte {
		return nil
	})

	res := getJSON(t, srv, htPathPLogs, nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if !strings.Contains(string(res.body), htLogsEmpty) {
		t.Fatalf("body = %q, expected logs:\"\"", res.body)
	}
}

// TestHandle_PodLogs_LargeBody confirms the handler streams a large log body
// back without truncation.
func TestHandle_PodLogs_LargeBody(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameP, htNSDefault))
	big := strings.Repeat("hello world\n", htBigLogRepeats) // ~48KB

	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte {
		return []byte(big)
	})

	var out logsBody

	res := getJSON(t, srv, htPathPLogs, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if out.Logs != big {
		t.Fatalf(
			"log body length mismatch: got %d, want %d",
			len(out.Logs), len(big),
		)
	}
}

// TestCORS_AllRoutes confirms every route emits the CORS header.
func TestCORS_AllRoutes(t *testing.T) {
	t.Parallel()

	pod := newRunningPod(htNameP, htNSDefault)
	srv, c := newTestServer(t, nil, pod)
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte {
		return []byte(htLogsOK)
	})

	paths := []string{
		htPathHealth,
		htPathCluster,
		htPathNamespaces,
		htPathPods,
		htPathPDetail,
		htPathPLogs,
		htPathDeployments,
		htPathServices,
		htPathNodes,
		htPathEvents,
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			corsRouteCase(t, srv, p)
		})
	}
}

func corsRouteCase(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	res := httpGet(t, srv.URL+path)

	got := res.header.Get(htHdrACAO)
	if got != htACAOValue {
		t.Fatalf("%s -> ACAO = %q", path, got)
	}
}

// TestWriteJSON_EncoderError covers writeJSON's error branch by handing it a
// ResponseWriter whose Write method always fails.
func TestWriteJSON_EncoderError(t *testing.T) {
	t.Parallel()

	w := &failingResponseWriter{header: http.Header{}, writeCalls: htZero}
	writeJSON(w, http.StatusOK, map[string]string{htNameA: htNameB})

	if w.writeCalls == htZero {
		t.Fatal("expected at least one Write call")
	}
}

type failingResponseWriter struct {
	header     http.Header
	writeCalls int
}

func (f *failingResponseWriter) Header() http.Header { return f.header }

func (*failingResponseWriter) WriteHeader(int) {}

func (f *failingResponseWriter) Write(_ []byte) (int, error) {
	f.writeCalls++

	return htZero, errBrokenPipe
}

// TestHandle_PodLogs_NonNotFoundErrorReturns500 covers the non-404 error path
// of handlePodLogs (writeError fallback).
func TestHandle_PodLogs_NonNotFoundErrorReturns500(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameP, htNSDefault))
	injectLogsErrorReactor(t, c.clientset, errTransient)

	res := getJSON(t, srv, htPathPLogs, nil)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

// TestHandle_PodDetail_NonNotFoundErrorReturns500 covers the non-404 error
// path of handlePod (mirrors the PodLogs test above).
func TestHandle_PodDetail_NonNotFoundErrorReturns500(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	injectGetPodsError(t, c.clientset, errTransient)

	res := getJSON(t, srv, "/api/pods/default/x", nil)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

// TestHandle_ListEndpointsPropagateErrors covers the writeError path of every
// list handler.
func TestHandle_ListEndpointsPropagateErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path     string
		resource string
	}{
		{htPathPods, htResPods},
		{htPathDeployments, htResDeployments},
		{htPathServices, htResServices},
		{htPathNodes, htResNodes},
		{htPathEvents, htResEvents},
		{htPathPodDetail, htResPods}, // GetPod error path (non-404)
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			listEndpointErrorCase(t, tc.path, tc.resource)
		})
	}
}

func listEndpointErrorCase(t *testing.T, path, resource string) {
	t.Helper()

	srv, c := newTestServer(
		t, newVersion(htVersionV1, htPlatformP),
		newPod(htNameWeb, htNSDefault),
	)
	injectResourceError(
		t, c.clientset, actionVerbForPath(path), resource,
		errBackend,
	)

	res := getJSON(t, srv, path, nil)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

func actionVerbForPath(path string) string {
	// The single-resource path /api/pods/{ns}/{name} maps to verb "get";
	// everything else (list endpoints) maps to "list".
	if strings.Count(path, "/") >= htSingleResourceSlashes {
		return htVerbGet
	}

	return htVerbList
}

// TestRouter_AllListEndpointsReturnEmptyArray confirms that every list
// endpoint serializes as `[]` (not `null`) when the cluster has no objects.
func TestRouter_AllListEndpointsReturnEmptyArray(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, newVersion(htVersionV1, htPlatformP))
	endpoints := []string{
		htPathNamespaces,
		htPathPods,
		htPathDeployments,
		htPathServices,
		htPathNodes,
		htPathEvents,
	}

	for _, e := range endpoints {
		t.Run(e, func(t *testing.T) {
			t.Parallel()
			emptyArrayCase(t, srv, e)
		})
	}
}

func emptyArrayCase(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	res := getJSON(t, srv, path, nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if strings.TrimSpace(string(res.body)) != "[]" {
		t.Fatalf("body = %q, want []", res.body)
	}
}

// --- expanded coverage --------------------------------------------------

// TestHandle_ContentTypeIsApplicationJSON locks in that every successful
// response declares JSON content.
func TestHandle_ContentTypeIsApplicationJSON(t *testing.T) {
	t.Parallel()

	pod := newPod(htNameP, htNSDefault)
	pod.Spec.Containers = []corev1.Container{newContainer(htNameC, htEmpty)}
	srv, c := newTestServer(t, newVersion(htVersionV1, htPlatformP), pod)
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte {
		return []byte("hi")
	})

	paths := []string{
		htPathHealth, htPathCluster, htPathNamespaces, htPathPods,
		htPathPDetail, htPathPLogs,
		htPathDeployments, htPathServices, htPathNodes, htPathEvents,
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			contentTypeCase(t, srv, p)
		})
	}
}

func contentTypeCase(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	res := httpGet(t, srv.URL+path)

	got := res.header.Get("Content-Type")
	if got != htContentJSON {
		t.Fatalf("%s Content-Type = %q", path, got)
	}
}

// TestHandle_AllResponsesAreValidJSON parses every endpoint's body and
// confirms it decodes as JSON, across success and error paths.
func TestHandle_AllResponsesAreValidJSON(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, newVersion(htVersionV1, htPlatformP))
	paths := []string{
		htPathHealth,
		htPathCluster,
		htPathNamespaces,
		htPathPods,
		htPathDeployments,
		htPathServices,
		htPathNodes,
		htPathEvents,
		"/api/pods/default/nonexistent", // 404 — error body should be JSON
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			validJSONCase(t, srv, path)
		})
	}
}

func validJSONCase(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()

	res := httpGet(t, srv.URL+path)

	var decoded any

	uerr := json.Unmarshal(res.body, &decoded)
	if uerr != nil {
		t.Fatalf("%s body not JSON: %v (body=%s)", path, uerr, res.body)
	}
}

// TestHandle_HealthTimestampParsesAndIsFresh confirms the health endpoint's
// timestamp is well-formed RFC3339 and within a sane window of "now".
func TestHandle_HealthTimestampParsesAndIsFresh(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	var out map[string]string

	res := getJSON(t, srv, htPathHealth, &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	ts, err := time.Parse(time.RFC3339, out["timestamp"])
	if err != nil {
		t.Fatalf("not RFC3339: %v", err)
	}

	delta := time.Since(ts)
	if delta < -htClockSkewWindow || delta > htClockSkewWindow {
		t.Fatalf(
			"timestamp %s is %s away from now — clock skew? expected <5s",
			ts, delta,
		)
	}
}

// TestHandle_PodLogs_Concurrent makes concurrent log requests against the same
// pod with different containers and verifies each gets its own logs.
func TestHandle_PodLogs_Concurrent(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameP, htNSDefault))
	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		return []byte("logs-for-" + opts.Container)
	})

	errs := make(chan error, htLogConcurrency)

	var wg sync.WaitGroup

	for idx := range htLogConcurrency {
		wg.Go(func() {
			errs <- concurrentLogGet(srv, idx)
		})
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		if e != nil {
			t.Error(e)
		}
	}
}

func concurrentLogGet(srv *httptest.Server, idx int) error {
	container := fmt.Sprintf("c%d", idx)
	url := srv.URL + htPathPLogs + "?container=" + container

	_, body, err := fetch(url)
	if err != nil {
		return err
	}

	var out logsBody

	derr := json.Unmarshal(body, &out)
	if derr != nil {
		return fmt.Errorf("decode: %w", derr)
	}

	want := "logs-for-" + container
	if out.Logs != want {
		return fmt.Errorf(
			"%w: got %q, want %q", errLogMismatch, out.Logs, want,
		)
	}

	return nil
}

// TestHandle_NamespaceQueryParamSpecialChars verifies the query-string
// decoding does not panic or mis-route on unusual characters.
func TestHandle_NamespaceQueryParamSpecialChars(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	for _, raw := range []string{"a%20b", "a%2Bb", "a+b", "a%26b"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			specialCharCase(t, srv, raw)
		})
	}
}

func specialCharCase(t *testing.T, srv *httptest.Server, raw string) {
	t.Helper()

	res := httpGet(t, srv.URL+"/api/pods?namespace="+raw)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}
}

// TestHandle_QueryParamCaseSensitivity verifies the handler treats query param
// names case-sensitively. `Namespace` (capital N) is NOT the filter.
func TestHandle_QueryParamCaseSensitivity(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(
		t, nil,
		newPod(htNameA, htNSDefault),
		newPod(htNameB, htNSKubeSystem),
	)

	var out []Pod
	// "Namespace" with capital N should NOT filter, returning both pods.
	res := getJSON(t, srv, "/api/pods?Namespace=default", &out)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if len(out) != htTwo {
		t.Fatalf("expected 2 pods (capital-N filter ignored), got %d", len(out))
	}
}

// TestHandle_PodLogs_TailLinesAtBoundary covers handler tailLines parsing at
// realistic edge values.
func TestHandle_PodLogs_TailLinesAtBoundary(t *testing.T) {
	t.Parallel()

	cases := map[string]int64{
		"1":       htOne,
		"5000":    maxTailLines, // exactly at the cap, passed through
		"1000000": maxTailLines, // above the cap, clamped down
	}

	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			boundaryTailCase(t, raw, want)
		})
	}
}

// boundaryTailCase spins up its own server per subtest so the captured
// tailLines value is not shared, allowing the subtests to run in parallel.
func boundaryTailCase(t *testing.T, raw string, want int64) {
	t.Helper()

	srv, c := newTestServer(t, nil, newPod(htNameP, htNSDefault))

	var capturedTail int64

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}

		return []byte(htLogsOK)
	})

	res := getJSON(t, srv, htPathPLogs+"?tailLines="+raw, nil)
	if res.statusCode != htStatusOK {
		t.Fatalf(htMsgStatus, res.statusCode)
	}

	if capturedTail != want {
		t.Fatalf("tailLines = %d, want %d", capturedTail, want)
	}
}

// TestHandle_StatusCodesAreNumericNotStrings parses error bodies and confirms
// the `status` field is a JSON number, not a string.
func TestHandle_StatusCodesAreNumericNotStrings(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := getJSON(t, srv, "/api/pods/default/missing", nil)
	if res.statusCode != htStatusNotF {
		t.Fatalf("HTTP status = %d", res.statusCode)
	}

	var raw map[string]json.RawMessage

	err := json.Unmarshal(res.body, &raw)
	if err != nil {
		t.Fatalf(htMsgDecode, err)
	}

	s, ok := raw["status"]
	if !ok {
		t.Fatal("missing status field")
	}
	// A JSON number doesn't start with a quote.
	if len(s) == htZero || s[htFirst] == '"' {
		t.Fatalf("status is a string, not a number: %s", s)
	}
}
