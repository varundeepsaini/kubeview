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

// errDiscovery embeds the FakeDiscovery (for all the methods we don't care
// about) and overrides ServerVersion to return a configured error.
type errDiscovery struct {
	discovery.DiscoveryInterface
	err error
}

func (e errDiscovery) ServerVersion() (*version.Info, error) { return nil, e.err }

// newTestClient builds a Client wired up to a fake clientset preloaded with
// the given objects. If serverVersion is non-nil it overrides what
// Discovery().ServerVersion() reports.
func newTestClient(t *testing.T, serverVersion *version.Info, objs ...runtime.Object) (*Client, *fake.Clientset) {
	t.Helper()
	cs := fake.NewClientset(objs...)
	if serverVersion != nil {
		fd, ok := cs.Discovery().(*discoveryfake.FakeDiscovery)
		if !ok {
			t.Fatalf("expected *discoveryfake.FakeDiscovery, got %T", cs.Discovery())
		}
		fd.FakedServerVersion = serverVersion
	}
	return &Client{
		clientset: cs,
		discovery: cs.Discovery(),
		context:   "test-context",
		cluster:   "test-cluster",
	}, cs
}

// --- ListNamespaces / ListPods / ListServices / ListDeployments / ListNodes / ListEvents

func TestClient_ListNamespaces(t *testing.T) {
	t.Run("returns all namespaces", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		)
		got, err := c.ListNamespaces(context.Background())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("empty cluster returns empty slice not nil", func(t *testing.T) {
		c, _ := newTestClient(t, nil)
		got, err := c.ListNamespaces(context.Background())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("propagates underlying error", func(t *testing.T) {
		c, cs := newTestClient(t, nil)
		cs.PrependReactor("list", "namespaces", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("boom")
		})
		_, err := c.ListNamespaces(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestClient_ListPods(t *testing.T) {
	t.Run("no namespace filter -> all namespaces", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "kube-system"}},
		)
		got, err := c.ListPods(context.Background(), "")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("namespace filter -> only that namespace", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "kube-system"}},
		)
		got, err := c.ListPods(context.Background(), "default")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].Name != "a" {
			t.Fatalf("got: %+v", got)
		}
	})
}

func TestClient_GetPod(t *testing.T) {
	t.Run("returns existing pod", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		got, err := c.GetPod(context.Background(), "default", "web")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Name != "web" {
			t.Fatalf("name = %q", got.Name)
		}
	})
	t.Run("missing pod returns IsNotFound error", func(t *testing.T) {
		c, _ := newTestClient(t, nil)
		_, err := c.GetPod(context.Background(), "default", "missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if !apierrors.IsNotFound(err) {
			t.Fatalf("expected IsNotFound, got %T: %v", err, err)
		}
	})
	t.Run("returns pod from a non-default namespace", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "kube-system"}},
		)
		got, err := c.GetPod(context.Background(), "kube-system", "p")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Namespace != "kube-system" {
			t.Fatalf("ns = %q", got.Namespace)
		}
	})
}

func TestClient_GetPodLogs(t *testing.T) {
	t.Run("returns fake logs body (default reactor)", func(t *testing.T) {
		c, _ := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		got, err := c.GetPodLogs(context.Background(), "default", "web", "", 100)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != "fake logs" {
			t.Fatalf("logs = %q", got)
		}
	})
	t.Run("passes container option through", func(t *testing.T) {
		c, cs := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var capturedContainer string
		var capturedTail int64
		cs.PrependReactor("get", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			g, ok := action.(core.GenericAction)
			if !ok || g.GetSubresource() != "log" {
				return false, nil, nil
			}
			opts := g.GetValue().(*corev1.PodLogOptions)
			capturedContainer = opts.Container
			if opts.TailLines != nil {
				capturedTail = *opts.TailLines
			}
			return true, &runtime.Unknown{Raw: []byte("logs for " + opts.Container)}, nil
		})
		got, err := c.GetPodLogs(context.Background(), "default", "web", "sidecar", 42)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != "logs for sidecar" {
			t.Fatalf("got = %q", got)
		}
		if capturedContainer != "sidecar" {
			t.Fatalf("captured container = %q", capturedContainer)
		}
		if capturedTail != 42 {
			t.Fatalf("captured tail = %d", capturedTail)
		}
	})
	t.Run("empty container -> empty Container option (server picks default)", func(t *testing.T) {
		c, cs := newTestClient(t, nil,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
		)
		var captured string
		cs.PrependReactor("get", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			g, ok := action.(core.GenericAction)
			if !ok || g.GetSubresource() != "log" {
				return false, nil, nil
			}
			captured = g.GetValue().(*corev1.PodLogOptions).Container
			return true, &runtime.Unknown{Raw: []byte("ok")}, nil
		})
		_, err := c.GetPodLogs(context.Background(), "default", "web", "", 100)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if captured != "" {
			t.Fatalf("expected empty Container option, got %q", captured)
		}
	})
}

func TestClient_ListDeployments(t *testing.T) {
	c, _ := newTestClient(t, nil,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ctl", Namespace: "kube-system"}},
	)
	t.Run("no namespace filter", func(t *testing.T) {
		got, err := c.ListDeployments(context.Background(), "")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("namespace filter", func(t *testing.T) {
		got, err := c.ListDeployments(context.Background(), "default")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].Name != "api" {
			t.Fatalf("got: %+v", got)
		}
	})
}

func TestClient_ListServices(t *testing.T) {
	c, _ := newTestClient(t, nil,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc2", Namespace: "kube-system"}},
	)
	t.Run("no namespace filter", func(t *testing.T) {
		got, err := c.ListServices(context.Background(), "")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("namespace filter", func(t *testing.T) {
		got, err := c.ListServices(context.Background(), "kube-system")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].Name != "svc2" {
			t.Fatalf("got: %+v", got)
		}
	})
}

func TestClient_ListNodes(t *testing.T) {
	c, _ := newTestClient(t, nil,
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
	)
	got, err := c.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
}

// TestClient_ListMethodsPropagateErrors covers the err-return line of every
// list method besides ListNamespaces (which is covered above). They're all
// thin "list -> if err -> return Items" wrappers, so this proves the error
// path is wired up uniformly across all of them.
func TestClient_ListMethodsPropagateErrors(t *testing.T) {
	cases := []struct {
		name     string
		resource string
		call     func(*Client) error
	}{
		{"ListPods", "pods", func(c *Client) error { _, err := c.ListPods(context.Background(), ""); return err }},
		{"ListDeployments", "deployments", func(c *Client) error { _, err := c.ListDeployments(context.Background(), ""); return err }},
		{"ListServices", "services", func(c *Client) error { _, err := c.ListServices(context.Background(), ""); return err }},
		{"ListNodes", "nodes", func(c *Client) error { _, err := c.ListNodes(context.Background()); return err }},
		{"ListEvents", "events", func(c *Client) error { _, err := c.ListEvents(context.Background(), ""); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, cs := newTestClient(t, nil)
			cs.PrependReactor("list", tc.resource, func(action core.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("backend down")
			})
			if err := tc.call(c); err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
		})
	}
}

func TestClient_ListEvents(t *testing.T) {
	c, _ := newTestClient(t, nil,
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e1", Namespace: "default"}, Reason: "Scheduled"},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e2", Namespace: "kube-system"}, Reason: "Pulled"},
	)
	t.Run("no namespace filter", func(t *testing.T) {
		got, err := c.ListEvents(context.Background(), "")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
	})
	t.Run("namespace filter", func(t *testing.T) {
		got, err := c.ListEvents(context.Background(), "default")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].Name != "e1" {
			t.Fatalf("got: %+v", got)
		}
	})
}

// --- loadKubeConfig (filesystem-backed) ---

func TestLoadKubeConfig_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
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
	t.Setenv("KUBECONFIG", path)
	cfg, ctx, cluster, err := loadKubeConfig()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if cfg == nil {
		t.Fatal("nil rest config")
	}
	if ctx != "my-ctx" {
		t.Fatalf("ctx = %q", ctx)
	}
	if cluster != "my-cluster" {
		t.Fatalf("cluster = %q", cluster)
	}
}

func TestLoadKubeConfig_MissingFile(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	if _, _, _, err := loadKubeConfig(); err == nil {
		t.Fatal("expected error for missing kubeconfig")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- NewClient (filesystem + config plumbing) ---

func TestNewClient_FromKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
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
	t.Setenv("KUBECONFIG", path)
	c, err := NewClient()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.context != "ctx" || c.cluster != "c1" {
		t.Fatalf("c = %+v", c)
	}
	if c.clientset == nil || c.discovery == nil {
		t.Fatalf("client fields nil: %+v", c)
	}
}

// TestLoadKubeConfig_DefaultPath covers the branch where KUBECONFIG is unset
// and loadKubeConfig falls back to ~/.kube/config. We don't expect the load
// to succeed (the test runner's HOME might not point at a real cluster), but
// the *path-resolution* code is exercised either way.
func TestLoadKubeConfig_DefaultPath(t *testing.T) {
	// Point HOME at a temp dir with a valid kubeconfig under .kube/config.
	home := t.TempDir()
	kube := home + "/.kube"
	if err := os.MkdirAll(kube, 0o700); err != nil {
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
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", home)
	_, ctx, cluster, err := loadKubeConfig()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ctx != "c" || cluster != "cl" {
		t.Fatalf("ctx=%q cluster=%q", ctx, cluster)
	}
}

// TestLoadKubeConfig_HomeUnset covers the branch where KUBECONFIG is unset
// *and* HOME is unset — kubeconfigPath stays empty and clientcmd produces a
// build-rest-config error.
func TestLoadKubeConfig_HomeUnset(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", "")
	if _, _, _, err := loadKubeConfig(); err == nil {
		t.Fatal("expected error when neither KUBECONFIG nor HOME is set")
	}
}

func TestNewClient_BadKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	mustWrite(t, path, "this is not yaml: [[[")
	t.Setenv("KUBECONFIG", path)
	if _, err := NewClient(); err == nil {
		t.Fatal("expected error")
	}
}

// TestNewClient_InvalidServerURLBreaksClientsetBuild covers the rare
// kubernetes.NewForConfig error path. We craft a kubeconfig whose server is
// not a parseable URL — clientcmd's ClientConfig() builds the *rest.Config
// successfully, then kubernetes.NewForConfig fails when it tries to derive
// the server URL.
func TestNewClient_InvalidServerURLBreaksClientsetBuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
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
	t.Setenv("KUBECONFIG", path)
	if _, err := NewClient(); err == nil {
		t.Fatal("expected error from clientset build with invalid server URL")
	}
}

// --- GetClusterInfo ---

func TestClient_GetClusterInfo(t *testing.T) {
	t.Run("aggregates version, platform, node count, context, cluster", func(t *testing.T) {
		c, _ := newTestClient(t,
			&version.Info{GitVersion: "v1.30.0", Platform: "linux/amd64"},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3"}},
		)
		info, err := c.GetClusterInfo(context.Background())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if info.Version != "v1.30.0" {
			t.Fatalf("version = %q", info.Version)
		}
		if info.Platform != "linux/amd64" {
			t.Fatalf("platform = %q", info.Platform)
		}
		if info.NodeCount != 3 {
			t.Fatalf("node count = %d", info.NodeCount)
		}
		if info.Context != "test-context" || info.ClusterName != "test-cluster" {
			t.Fatalf("context/cluster wrong: %+v", info)
		}
	})

	t.Run("zero-node cluster reports NodeCount=0", func(t *testing.T) {
		c, _ := newTestClient(t, &version.Info{GitVersion: "v1.30.0", Platform: "linux/amd64"})
		info, err := c.GetClusterInfo(context.Background())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if info.NodeCount != 0 {
			t.Fatalf("node count = %d", info.NodeCount)
		}
	})

	t.Run("propagates server-version error", func(t *testing.T) {
		c := &Client{
			clientset: fake.NewClientset(),
			discovery: errDiscovery{err: errors.New("api unreachable")},
		}
		_, err := c.GetClusterInfo(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("propagates list-nodes error", func(t *testing.T) {
		c, cs := newTestClient(t, &version.Info{GitVersion: "v1.30.0", Platform: "linux/amd64"})
		cs.PrependReactor("list", "nodes", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("nodes down")
		})
		_, err := c.GetClusterInfo(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
