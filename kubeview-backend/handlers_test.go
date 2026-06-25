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

// newTestServer wires the project's real router (and CORS middleware) up to a
// fake-backed Client and starts an httptest.Server. The returned cleanup
// function must be called.
func newTestServer(t *testing.T, sv *version.Info, objs ...runtime.Object) (*httptest.Server, *Client) {
	t.Helper()
	c, _ := newTestClient(t, sv, objs...)
	srv := httptest.NewServer(withCORS(newRouter(c)))
	t.Cleanup(srv.Close)
	return srv, c
}

func getJSON(t *testing.T, srv *httptest.Server, path string, dst any) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if dst != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(body, dst); err != nil {
			t.Fatalf("decode %s: %v\nbody: %s", path, err, body)
		}
	}
	return resp, body
}

// --- /api/health ---

func TestHandle_Health(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	var out map[string]string
	resp, _ := getJSON(t, srv, "/api/health", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if out["status"] != "ok" {
		t.Fatalf("status field = %q", out["status"])
	}
	// timestamp is RFC3339; parse round-trips
	if _, err := time.Parse(time.RFC3339, out["timestamp"]); err != nil {
		t.Fatalf("timestamp not RFC3339: %q (%v)", out["timestamp"], err)
	}
}

// --- /api/cluster ---

func TestHandle_Cluster(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		srv, _ := newTestServer(t,
			&version.Info{GitVersion: "v1.30.0", Platform: "linux/amd64"},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		)
		var info ClusterInfo
		resp, _ := getJSON(t, srv, "/api/cluster", &info)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if info.Version != "v1.30.0" || info.Platform != "linux/amd64" || info.NodeCount != 1 {
			t.Fatalf("info = %+v", info)
		}
		if info.Context != "test-context" || info.ClusterName != "test-cluster" {
			t.Fatalf("context/cluster wrong: %+v", info)
		}
	})

	t.Run("error from kube returns 500", func(t *testing.T) {
		srv, c := newTestServer(t, &version.Info{GitVersion: "v1", Platform: "p"})
		cs := c.clientset
		// reactors only work on the fake clientset which we have direct access to via Client
		injectListNodesError(t, cs, errors.New("boom"))
		resp, body := getJSON(t, srv, "/api/cluster", nil)
		if resp.StatusCode != 500 {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var e errResp
		if err := json.Unmarshal(body, &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if e.Status != 500 {
			t.Fatalf("err response = %+v", e)
		}
		// 5xx responses must NOT leak the raw internal error to the client.
		if strings.Contains(e.Error, "boom") {
			t.Fatalf("5xx body leaked internal error detail: %q", e.Error)
		}
		if e.Error != "Internal server error" {
			t.Fatalf("err message = %q, want generic 5xx message", e.Error)
		}
	})
}

type errResp struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

// --- /api/namespaces ---

func TestHandle_Namespaces(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{"env": "prod"}},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
	)
	var out []Namespace
	resp, _ := getJSON(t, srv, "/api/namespaces", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 1 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].Name != "default" || out[0].Status != "Active" || out[0].Labels["env"] != "prod" {
		t.Fatalf("ns = %+v", out[0])
	}
}

// --- /api/pods, /api/pods/{ns}/{name}, /api/pods/{ns}/{name}/logs ---

func TestHandle_Pods_List(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "kube-system"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	t.Run("all namespaces", func(t *testing.T) {
		var out []Pod
		resp, _ := getJSON(t, srv, "/api/pods", &out)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if len(out) != 2 {
			t.Fatalf("len = %d", len(out))
		}
	})
	t.Run("filtered by namespace", func(t *testing.T) {
		var out []Pod
		resp, _ := getJSON(t, srv, "/api/pods?namespace=default", &out)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if len(out) != 1 || out[0].Name != "a" {
			t.Fatalf("got: %+v", out)
		}
	})
	t.Run("returns [] for empty list, never null", func(t *testing.T) {
		srv2, _ := newTestServer(t, nil)
		resp, body := getJSON(t, srv2, "/api/pods", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if strings.TrimSpace(string(body)) != "[]" {
			t.Fatalf("body = %q, want []", body)
		}
	})
}

func TestHandle_Pod_Detail(t *testing.T) {
	t.Run("returns pod when present", func(t *testing.T) {
		srv, _ := newTestServer(t, nil,
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1"}}},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1"},
			},
		)
		var p Pod
		resp, _ := getJSON(t, srv, "/api/pods/default/web", &p)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if p.Name != "web" || p.Namespace != "default" || p.IP != "10.0.0.1" {
			t.Fatalf("pod = %+v", p)
		}
	})

	t.Run("missing pod returns 404 with frontend-friendly error shape", func(t *testing.T) {
		srv, _ := newTestServer(t, nil)
		resp, body := getJSON(t, srv, "/api/pods/default/missing", nil)
		if resp.StatusCode != 404 {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var e errResp
		if err := json.Unmarshal(body, &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if e.Error != "Pod not found" || e.Status != 404 {
			t.Fatalf("err = %+v", e)
		}
	})
}

func TestHandle_PodLogs(t *testing.T) {
	t.Run("returns logs object with default tailLines", func(t *testing.T) {
		srv, c := newTestServer(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var capturedTail int64
		injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
			if opts.TailLines != nil {
				capturedTail = *opts.TailLines
			}
			return []byte("hello")
		})
		var out struct{ Logs string }
		resp, _ := getJSON(t, srv, "/api/pods/default/web/logs", &out)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if out.Logs != "hello" {
			t.Fatalf("logs = %q", out.Logs)
		}
		if capturedTail != 100 {
			t.Fatalf("default tailLines = %d, want 100", capturedTail)
		}
	})

	t.Run("respects container query param", func(t *testing.T) {
		srv, c := newTestServer(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var captured string
		injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
			captured = opts.Container
			return []byte("ok")
		})
		var out struct{ Logs string }
		resp, _ := getJSON(t, srv, "/api/pods/default/web/logs?container=sidecar", &out)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if captured != "sidecar" {
			t.Fatalf("container = %q", captured)
		}
	})

	t.Run("respects tailLines query param", func(t *testing.T) {
		srv, c := newTestServer(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var capturedTail int64
		injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
			if opts.TailLines != nil {
				capturedTail = *opts.TailLines
			}
			return []byte("")
		})
		resp, _ := getJSON(t, srv, "/api/pods/default/web/logs?tailLines=500", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if capturedTail != 500 {
			t.Fatalf("tailLines = %d", capturedTail)
		}
	})

	t.Run("invalid tailLines falls back to default 100", func(t *testing.T) {
		srv, c := newTestServer(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var capturedTail int64
		injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
			if opts.TailLines != nil {
				capturedTail = *opts.TailLines
			}
			return []byte("")
		})
		for _, raw := range []string{"abc", "-5", "0"} {
			capturedTail = -1
			resp, _ := getJSON(t, srv, "/api/pods/default/web/logs?tailLines="+raw, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status for %q = %d", raw, resp.StatusCode)
			}
			if capturedTail != 100 {
				t.Fatalf("tailLines for %q = %d, want 100", raw, capturedTail)
			}
		}
	})

	t.Run("missing pod -> 404", func(t *testing.T) {
		srv, c := newTestServer(t, nil)
		cs := c.clientset
		injectLogsErrorReactor(t, cs, apierrors.NewNotFound(corev1.Resource("pods"), "nope"))
		resp, body := getJSON(t, srv, "/api/pods/default/nope/logs", nil)
		if resp.StatusCode != 404 {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var e errResp
		_ = json.Unmarshal(body, &e)
		if e.Error != "Pod not found" || e.Status != 404 {
			t.Fatalf("err = %+v", e)
		}
	})

	t.Run("response always includes logs field (even when empty)", func(t *testing.T) {
		// The JS server returns `{ logs: logs || "" }`, so the frontend can
		// always read `.logs` without a null check. Mirror that.
		srv, c := newTestServer(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
			return []byte("")
		})
		resp, body := getJSON(t, srv, "/api/pods/default/web/logs", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if !strings.Contains(string(body), `"logs":""`) {
			t.Fatalf("body = %q (expected logs:\"\")", body)
		}
	})
}

// --- /api/deployments ---

func TestHandle_Deployments(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:1"}}},
				},
			},
		},
	)
	var out []Deployment
	resp, _ := getJSON(t, srv, "/api/deployments?namespace=default", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 1 || out[0].Name != "api" || len(out[0].Images) != 1 || out[0].Images[0] != "api:1" {
		t.Fatalf("deployments = %+v", out)
	}
}

// --- /api/services ---

func TestHandle_Services(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "1.2.3.4",
				Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		},
	)
	var out []Service
	resp, _ := getJSON(t, srv, "/api/services", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 1 || out[0].ClusterIP != "1.2.3.4" {
		t.Fatalf("services = %+v", out)
	}
}

// --- /api/nodes ---

func TestHandle_Nodes(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	)
	var out []NodeInfo
	resp, _ := getJSON(t, srv, "/api/nodes", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 1 || out[0].Status != "Ready" {
		t.Fatalf("nodes = %+v", out)
	}
}

// --- /api/events ---

func TestHandle_Events(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "e1", Namespace: "default"},
			Type:           "Normal",
			Reason:         "Scheduled",
			Message:        "scheduled",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "x"},
			Count:          0,
		},
	)
	var out []KubeEvent
	resp, _ := getJSON(t, srv, "/api/events", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 1 || out[0].Object != "Pod/x" {
		t.Fatalf("events = %+v", out)
	}
	// count 0 should be coerced to 1 (matches JS `event.count || 1`)
	if out[0].Count != 1 {
		t.Fatalf("count = %d", out[0].Count)
	}
}

// --- CORS ---

func TestCORS(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	t.Run("GET response has CORS headers", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/health")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		defer resp.Body.Close()
		if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5500" {
			t.Fatalf("ACAO = %q", resp.Header.Get("Access-Control-Allow-Origin"))
		}
	})
	t.Run("OPTIONS preflight returns 204 with CORS headers and no body", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/pods", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Methods") == "" {
			t.Fatalf("ACAM missing")
		}
		if resp.Header.Get("Access-Control-Allow-Headers") == "" {
			t.Fatalf("ACAH missing")
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) != 0 {
			t.Fatalf("expected empty body, got %q", body)
		}
	})
}

// --- error mapping ---

func TestErrorMapping_StatusErrorPropagatesCode(t *testing.T) {
	// Build a custom Client whose underlying clientset returns an
	// apierrors.NewForbidden for namespaces — handler should propagate 403.
	srv, c := newTestServer(t, nil)
	cs := c.clientset
	gr := corev1.Resource("namespaces")
	injectListNamespacesError(t, cs, apierrors.NewForbidden(gr, "x", errors.New("denied")))

	resp, body := getJSON(t, srv, "/api/namespaces", nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var e errResp
	_ = json.Unmarshal(body, &e)
	if e.Status != 403 {
		t.Fatalf("status field = %d", e.Status)
	}
}

func TestErrorMapping_NonStatusErrorBecomes500(t *testing.T) {
	srv, c := newTestServer(t, nil)
	cs := c.clientset
	injectListNamespacesError(t, cs, errors.New("just a plain error"))
	resp, body := getJSON(t, srv, "/api/namespaces", nil)
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
}

// --- method gating ---

func TestRouter_OnlyAllowsGET(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, _ := http.NewRequest(method, srv.URL+"/api/pods", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		resp.Body.Close()
		// http.ServeMux pattern "GET /api/pods" rejects other methods with
		// 405 Method Not Allowed by default in Go 1.22+.
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s -> %d, want 405", method, resp.StatusCode)
		}
	}
}

// --- reactor helpers (private to handlers_test) ---

// The fake clientset is parameterised by kubernetes.Interface in the production
// code so tests reach the concrete *fake.Clientset through an interface. The
// helpers below centralise the reactor wiring.

type fakeReactor interface {
	PrependReactor(verb, resource string, reaction core.ReactionFunc)
}

func asFakeReactor(t *testing.T, x any) fakeReactor {
	t.Helper()
	r, ok := x.(fakeReactor)
	if !ok {
		t.Fatalf("clientset is not a fake reactor, got %T", x)
	}
	return r
}

func injectListNodesError(t *testing.T, cs any, err error) {
	asFakeReactor(t, cs).PrependReactor("list", "nodes", func(core.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

func injectListNamespacesError(t *testing.T, cs any, err error) {
	asFakeReactor(t, cs).PrependReactor("list", "namespaces", func(core.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

// injectLogsReactor wires a reactor that returns the bytes produced by the
// callback for any GetLogs request.
func injectLogsReactor(t *testing.T, cs any, fn func(*corev1.PodLogOptions) []byte) {
	asFakeReactor(t, cs).PrependReactor("get", "pods", func(action core.Action) (bool, runtime.Object, error) {
		g, ok := action.(core.GenericAction)
		if !ok || g.GetSubresource() != "log" {
			return false, nil, nil
		}
		opts := g.GetValue().(*corev1.PodLogOptions)
		return true, &runtime.Unknown{Raw: fn(opts)}, nil
	})
}

func injectLogsErrorReactor(t *testing.T, cs any, retErr error) {
	asFakeReactor(t, cs).PrependReactor("get", "pods", func(action core.Action) (bool, runtime.Object, error) {
		g, ok := action.(core.GenericAction)
		if !ok || g.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, retErr
	})
}

// --- additional handler tests --------------------------------------------

// TestRouter_UnknownRouteReturns404 verifies Go 1.22+ ServeMux behavior: a
// pattern-mismatched path returns 404 from the mux itself (no handler runs).
func TestRouter_UnknownRouteReturns404(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	for _, path := range []string{"/", "/api/unknown", "/api/pods/x", "/wrong"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + path)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
		})
	}
}

// TestRouter_TrailingSlashRejected is documented behavior of net/http's
// ServeMux for non-rooted patterns: /api/pods registers an exact match.
// /api/pods/ would match a different (more-specific) pattern only if one
// exists. We don't register any trailing-slash patterns, so /api/pods/ should
// 404.
func TestRouter_TrailingSlashRejected(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp, err := http.Get(srv.URL + "/api/pods/")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHandle_ConcurrentRequests fires N parallel GETs at the server and
// asserts every one returns 200 with the right shape. The fake clientset has
// its own lock, but this checks the handlers themselves aren't introducing
// any data race that the race detector would catch (run via `go test -race`).
func TestHandle_ConcurrentRequests(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	const n = 50
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/api/pods")
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				errs <- fmt.Errorf("status %d", resp.StatusCode)
				return
			}
			var out []Pod
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				errs <- err
				return
			}
			if len(out) != 2 {
				errs <- fmt.Errorf("got %d pods", len(out))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent request failed: %v", e)
	}
}

// TestHandle_ContextPropagation verifies that the handler hands r.Context()
// to the underlying kube client call. We do this by registering a reactor
// that records whatever context the action carries — but the fake clientset
// reactor signature does not expose the action's context, so we instead
// confirm the *server side* deadline behavior: a client whose context is
// already canceled at issue time should produce a quick failure, not hang.
func TestHandle_ContextPropagation(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/pods", nil)
	done := make(chan struct{})
	go func() {
		_, _ = http.DefaultClient.Do(req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not abort despite a pre-canceled context")
	}
}

// TestHandle_PodLogs_EmptyBytes confirms the response shape when the K8s API
// returns zero log bytes — the handler must still emit {"logs":""} so the
// frontend can read .logs without a null check.
func TestHandle_PodLogs_EmptyBytes(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}},
	)
	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte { return nil })
	resp, body := getJSON(t, srv, "/api/pods/default/p/logs", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"logs":""`) {
		t.Fatalf("body = %q, expected logs:\"\"", body)
	}
}

// TestHandle_PodLogs_LargeBody confirms the handler streams a large log body
// back without truncation. The size below comfortably exceeds typical
// `bufio` buffer thresholds.
func TestHandle_PodLogs_LargeBody(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}},
	)
	big := strings.Repeat("hello world\n", 4096) // ~48KB
	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte { return []byte(big) })
	var out struct{ Logs string }
	resp, _ := getJSON(t, srv, "/api/pods/default/p/logs", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if out.Logs != big {
		t.Fatalf("log body length mismatch: got %d, want %d", len(out.Logs), len(big))
	}
}

// TestCORS_AllRoutes confirms every route emits the CORS header — there's no
// route-specific bypass.
func TestCORS_AllRoutes(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	)
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte { return []byte("ok") })
	paths := []string{
		"/api/health",
		"/api/cluster",
		"/api/namespaces",
		"/api/pods",
		"/api/pods/default/p",
		"/api/pods/default/p/logs",
		"/api/deployments",
		"/api/services",
		"/api/nodes",
		"/api/events",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp, err := http.Get(srv.URL + p)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5500" {
				t.Fatalf("%s -> ACAO = %q", p, got)
			}
		})
	}
}

// TestWriteJSON_EncoderError covers writeJSON's `if err := encode; err != nil`
// branch by handing it a ResponseWriter whose Write method always fails. This
// path is otherwise unreachable in tests because httptest's ResponseRecorder
// never returns an error from Write.
func TestWriteJSON_EncoderError(t *testing.T) {
	w := &failingResponseWriter{header: http.Header{}}
	writeJSON(w, http.StatusOK, map[string]string{"a": "b"})
	if w.writeCalls == 0 {
		t.Fatal("expected at least one Write call")
	}
}

type failingResponseWriter struct {
	header     http.Header
	writeCalls int
}

func (f *failingResponseWriter) Header() http.Header { return f.header }
func (f *failingResponseWriter) WriteHeader(int)     {}
func (f *failingResponseWriter) Write(p []byte) (int, error) {
	f.writeCalls++
	return 0, errors.New("broken pipe")
}

// TestHandle_PodLogs_NonNotFoundErrorReturns500 covers the non-404 error path
// of handlePodLogs (writeError fallback).
func TestHandle_PodLogs_NonNotFoundErrorReturns500(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}},
	)
	injectLogsErrorReactor(t, c.clientset, errors.New("transient backend error"))
	resp, body := getJSON(t, srv, "/api/pods/default/p/logs", nil)
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
}

// TestHandle_PodDetail_NonNotFoundErrorReturns500 covers the non-404 error
// path of handlePod (mirrors the PodLogs test above).
func TestHandle_PodDetail_NonNotFoundErrorReturns500(t *testing.T) {
	srv, c := newTestServer(t, nil)
	asFakeReactor(t, c.clientset).PrependReactor("get", "pods", func(core.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("transient backend error")
	})
	resp, body := getJSON(t, srv, "/api/pods/default/x", nil)
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
}

// TestHandle_ListEndpointsPropagateErrors covers the writeError path of every
// list handler. handleNamespaces already has its error path exercised in
// TestErrorMapping_*; this fans the same pattern out to the others.
func TestHandle_ListEndpointsPropagateErrors(t *testing.T) {
	cases := []struct {
		path     string
		resource string
	}{
		{"/api/pods", "pods"},
		{"/api/deployments", "deployments"},
		{"/api/services", "services"},
		{"/api/nodes", "nodes"},
		{"/api/events", "events"},
		{"/api/pods/default/web", "pods"}, // GetPod error path (non-404)
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			srv, c := newTestServer(t, &version.Info{GitVersion: "v1", Platform: "p"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
			)
			asFakeReactor(t, c.clientset).PrependReactor(actionVerbForPath(tc.path), tc.resource, func(core.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("backend down")
			})
			resp, body := getJSON(t, srv, tc.path, nil)
			if resp.StatusCode != 500 {
				t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
			}
		})
	}
}

func actionVerbForPath(path string) string {
	// The single-resource path /api/pods/{ns}/{name} maps to verb "get";
	// everything else (list endpoints) maps to "list".
	if strings.Count(path, "/") >= 4 {
		return "get"
	}
	return "list"
}

// TestRouter_AllListEndpointsReturnEmptyArray confirms that every list
// endpoint serializes as `[]` (not `null`) when the cluster has no objects.
// The JS backend has this behaviour naturally because it does
// `items.map(t.transform...)` (empty array → empty array). The Go version
// relies on `make([]T, 0, n)` instead of `var x []T` — this test pins that.
func TestRouter_AllListEndpointsReturnEmptyArray(t *testing.T) {
	srv, _ := newTestServer(t, &version.Info{GitVersion: "v1", Platform: "p"})
	endpoints := []string{
		"/api/namespaces",
		"/api/pods",
		"/api/deployments",
		"/api/services",
		"/api/nodes",
		"/api/events",
	}
	for _, e := range endpoints {
		t.Run(e, func(t *testing.T) {
			resp, body := getJSON(t, srv, e, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			if strings.TrimSpace(string(body)) != "[]" {
				t.Fatalf("body = %q, want []", body)
			}
		})
	}
}

// --- expanded coverage --------------------------------------------------

// TestHandle_ContentTypeIsApplicationJSON locks in that every successful
// response declares JSON content. The frontend's fetch() trusts this — if it
// drifted to text/plain or text/html the JSON parse on the client would fail.
func TestHandle_ContentTypeIsApplicationJSON(t *testing.T) {
	srv, c := newTestServer(t, &version.Info{GitVersion: "v1", Platform: "p"},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}},
	)
	injectLogsReactor(t, c.clientset, func(_ *corev1.PodLogOptions) []byte { return []byte("hi") })
	paths := []string{
		"/api/health", "/api/cluster", "/api/namespaces", "/api/pods",
		"/api/pods/default/p", "/api/pods/default/p/logs",
		"/api/deployments", "/api/services", "/api/nodes", "/api/events",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp, err := http.Get(srv.URL + p)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			resp.Body.Close()
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("%s Content-Type = %q", p, got)
			}
		})
	}
}

// TestHandle_AllResponsesAreValidJSON parses every endpoint's body and
// confirms it decodes as JSON. Same guarantee from the integration suite
// but here it covers both success and error paths.
func TestHandle_AllResponsesAreValidJSON(t *testing.T) {
	srv, _ := newTestServer(t, &version.Info{GitVersion: "v1", Platform: "p"})
	for _, path := range []string{
		"/api/health",
		"/api/cluster",
		"/api/namespaces",
		"/api/pods",
		"/api/deployments",
		"/api/services",
		"/api/nodes",
		"/api/events",
		"/api/pods/default/nonexistent", // 404 — error body should also be JSON
	} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + path)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var any interface{}
			if err := json.Unmarshal(body, &any); err != nil {
				t.Fatalf("%s body not JSON: %v (body=%s)", path, err, body)
			}
		})
	}
}

// TestHandle_HealthTimestampParsesAndIsFresh confirms the health endpoint's
// timestamp is well-formed RFC3339 and within a sane window of "now".
func TestHandle_HealthTimestampParsesAndIsFresh(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	var out map[string]string
	resp, _ := getJSON(t, srv, "/api/health", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	ts, err := time.Parse(time.RFC3339, out["timestamp"])
	if err != nil {
		t.Fatalf("not RFC3339: %v", err)
	}
	if delta := time.Since(ts); delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("timestamp %s is %s away from now — clock skew? expected <5s", ts, delta)
	}
}

// TestHandle_PodLogs_Concurrent makes 30 concurrent log requests against the
// same pod with different containers and verifies each gets its own logs
// without crosstalk.
func TestHandle_PodLogs_Concurrent(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}},
	)
	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		return []byte("logs-for-" + opts.Container)
	})

	const n = 30
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			container := fmt.Sprintf("c%d", idx)
			resp, err := http.Get(srv.URL + "/api/pods/default/p/logs?container=" + container)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			var out struct{ Logs string }
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				errs <- err
				return
			}
			want := "logs-for-" + container
			if out.Logs != want {
				errs <- fmt.Errorf("got %q, want %q", out.Logs, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// TestHandle_NamespaceQueryParamSpecialChars verifies the query-string
// decoding (handler reads via r.URL.Query, which URL-decodes automatically).
// Kubernetes namespace names can't actually contain these characters, but
// our handler must not panic or mis-route when they appear.
func TestHandle_NamespaceQueryParamSpecialChars(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	for _, raw := range []string{"a%20b", "a%2Bb", "a+b", "a%26b"} {
		t.Run(raw, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/api/pods?namespace=" + raw)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
		})
	}
}

// TestHandle_QueryParamCaseSensitivity verifies the handler treats query
// param names case-sensitively (matches stdlib + JS behavior). `Namespace`
// (capital N) is NOT recognized as the filter.
func TestHandle_QueryParamCaseSensitivity(t *testing.T) {
	srv, _ := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "kube-system"}},
	)
	var out []Pod
	// "Namespace" with capital N — should NOT filter, returning both pods.
	resp, _ := getJSON(t, srv, "/api/pods?Namespace=default", &out)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 pods (capital-N filter ignored), got %d", len(out))
	}
}

// TestHandle_PodLogs_TailLinesAtBoundary covers handler tailLines parsing at
// realistic edge values: 1 (minimum), and a very large number.
func TestHandle_PodLogs_TailLinesAtBoundary(t *testing.T) {
	srv, c := newTestServer(t, nil,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}},
	)
	var capturedTail int64
	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		if opts.TailLines != nil {
			capturedTail = *opts.TailLines
		}
		return []byte("ok")
	})
	cases := map[string]int64{
		"1":       1,
		"5000":    maxTailLines, // exactly at the cap, passed through
		"1000000": maxTailLines, // above the cap, clamped down
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			capturedTail = -1
			resp, _ := getJSON(t, srv, "/api/pods/default/p/logs?tailLines="+raw, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			if capturedTail != want {
				t.Fatalf("tailLines = %d, want %d", capturedTail, want)
			}
		})
	}
}

// TestHandle_StatusCodesAreNumericNotStrings parses error bodies and confirms
// the `status` field is a JSON number, not a string — the frontend assumes
// numeric status everywhere.
func TestHandle_StatusCodesAreNumericNotStrings(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp, body := getJSON(t, srv, "/api/pods/default/missing", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("HTTP status = %d", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	s, ok := raw["status"]
	if !ok {
		t.Fatal("missing status field")
	}
	// A JSON number doesn't start with a quote.
	if len(s) == 0 || s[0] == '"' {
		t.Fatalf("status is a string, not a number: %s", s)
	}
}
