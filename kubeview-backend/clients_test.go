package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	cmCtxDev           = "dev"
	cmCtxProd          = "prod"
	cmExpectedContexts = 2
)

// newTestManager builds a ClientManager over the given context names, with an
// injected build func that hands out a fresh fake-backed *Client per context
// and records how many times it was called. No real kubeconfig or cluster is
// touched. The default context is the first name.
func newTestManager(
	t *testing.T,
	names ...string,
) (*ClientManager, map[string]int) {
	t.Helper()

	contexts := make([]ContextInfo, zeroCount, len(names))
	for i, name := range names {
		contexts = append(contexts, ContextInfo{
			Name:    name,
			Cluster: name + "-cluster",
			Current: i == zeroCount,
		})
	}

	builds := make(map[string]int)

	defaultContext := emptyString
	if len(names) > zeroCount {
		defaultContext = names[zeroCount]
	}

	manager := new(ClientManager)
	manager.clients = make(map[string]*Client)
	manager.contexts = contexts
	manager.defaultContext = defaultContext
	manager.build = func(name string) (*Client, error) {
		builds[name]++
		client, _ := newTestClient(t, nil)

		return client, nil
	}

	return manager, builds
}

// The client timeout must stay under the server's write timeout: at the write
// timeout the connection is severed mid-response, while a client timeout
// before it still yields a JSON error. It must also leave room for legitimate
// slow requests (pod-log tails, large lists) that writeTimeout budgets for.
func TestClientRequestTimeoutUnderWriteTimeout(t *testing.T) {
	t.Parallel()

	if clientRequestTimeout >= writeTimeout {
		t.Fatalf(
			"clientRequestTimeout %v must be under writeTimeout %v",
			clientRequestTimeout, writeTimeout,
		)
	}
}

func TestClientManager_ClientForDefault(t *testing.T) {
	t.Parallel()

	manager, builds := newTestManager(t, cmCtxDev, cmCtxProd)

	client, err := manager.ClientFor(emptyString)
	requireNoErr(t, err)

	if client == nil {
		t.Fatal("nil client for default context")
	}

	if builds[cmCtxDev] != ktOne {
		t.Fatalf("dev builds = %d, want 1", builds[cmCtxDev])
	}
}

func TestClientManager_UnknownContext(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t, cmCtxDev)

	_, err := manager.ClientFor("nope")
	if !errors.Is(err, errUnknownContext) {
		t.Fatalf("err = %v, want errUnknownContext", err)
	}
}

func TestClientManager_CachesPerContext(t *testing.T) {
	t.Parallel()

	manager, builds := newTestManager(t, cmCtxDev, cmCtxProd)

	first, err := manager.ClientFor(cmCtxProd)
	requireNoErr(t, err)

	second, err := manager.ClientFor(cmCtxProd)
	requireNoErr(t, err)

	if first != second {
		t.Fatal("expected cached client to be reused")
	}

	if builds[cmCtxProd] != ktOne {
		t.Fatalf("prod builds = %d, want 1 (cached)", builds[cmCtxProd])
	}
}

func TestClientManager_Contexts(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t, cmCtxDev, cmCtxProd)

	got := manager.Contexts()
	requireLen(t, got, cmExpectedContexts)

	// newTestManager marks the first name (dev) current.
	for _, ctx := range got {
		wantCurrent := ctx.Name == cmCtxDev
		if ctx.Current != wantCurrent {
			t.Fatalf("context %q current = %v, want %v",
				ctx.Name, ctx.Current, wantCurrent)
		}
	}
}

// TestNewClientManager_FromKubeconfig exercises the real loading path: a
// kubeconfig on disk is enumerated, the current context resolves its cluster,
// the default client builds, and an unknown context is rejected.
func TestNewClientManager_FromKubeconfig(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	dir := t.TempDir()
	path := filepath.Join(dir, ktConfigFileName)
	mustWrite(t, path, ktKubeconfigYAML)
	t.Setenv(ktEnvKubeconfig, path)

	manager, err := NewClientManager()
	requireNoErr(t, err)

	got := manager.Contexts()
	requireLen(t, got, ktOne)

	ctx := got[zeroCount]
	if ctx.Name != ktCtxMy || ctx.Cluster != "my-cluster" || !ctx.Current {
		t.Fatalf("context = %+v", ctx)
	}

	client, err := manager.ClientFor(emptyString)
	if err != nil {
		t.Fatalf("default context client: %v", err)
	}

	requireDefaultClient(t, client)

	_, err = manager.ClientFor("missing")
	if !errors.Is(err, errUnknownContext) {
		t.Fatalf("err = %v, want errUnknownContext", err)
	}
}

// requireDefaultClient asserts the eagerly built default client carries the
// expected context identity and constructed clientsets.
func requireDefaultClient(t *testing.T, client *Client) {
	t.Helper()

	if client.context != ktCtxMy || client.cluster != "my-cluster" {
		t.Fatalf("client = %+v", client)
	}

	if client.clientset == nil || client.discovery == nil {
		t.Fatalf("client fields nil: %+v", client)
	}
}

// writeKubeconfig writes the given kubeconfig body to a temp file and returns
// its path. Callers t.Setenv it themselves so the KUBECONFIG mutation stays
// visible in the test body.
func writeKubeconfig(t *testing.T, kubeconfig string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ktConfigFileName)
	mustWrite(t, path, kubeconfig)

	return path
}

// A kubeconfig config problem (rather than a bad ?context= request) must
// surface as a config-shaped startup error, not errUnknownContext.

func TestNewClientManager_NoCurrentContext(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, writeKubeconfig(
		t,
		strings.Replace(
			ktKubeconfigYAML, "current-context: my-ctx\n", "", ktOne,
		),
	))

	_, err := NewClientManager()
	if !errors.Is(err, errNoCurrentContext) {
		t.Fatalf("err = %v, want errNoCurrentContext", err)
	}
}

func TestNewClientManager_DanglingCurrentContext(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, writeKubeconfig(
		t,
		strings.Replace(ktKubeconfigYAML, "my-ctx\n", "gone-ctx\n", ktOne),
	))

	_, err := NewClientManager()
	if !errors.Is(err, errBadCurrentContext) {
		t.Fatalf("err = %v, want errBadCurrentContext", err)
	}
}

func TestNewClientManager_MissingKubeconfigFile(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	// clientcmd silently skips missing precedence files, yielding an empty
	// config; that must fail as "no contexts", not unknown context "".
	t.Setenv(ktEnvKubeconfig, filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := NewClientManager()
	if !errors.Is(err, errNoContexts) {
		t.Fatalf("err = %v, want errNoContexts", err)
	}
}

func TestNewClientManager_BadKubeconfig(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, writeKubeconfig(t, "this is not yaml: [[["))

	_, err := NewClientManager()
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestNewClientManager_InvalidServerURLBreaksClientsetBuild covers the rare
// kubernetes.NewForConfig error path in the eager default-client build. We
// craft a kubeconfig whose server is not a parseable URL — clientcmd's
// ClientConfig() builds the *rest.Config successfully, then
// kubernetes.NewForConfig fails when it tries to derive the server URL.
func TestNewClientManager_InvalidServerURLBreaksClientsetBuild(t *testing.T) {
	// NOTE: mutates KUBECONFIG via t.Setenv; not parallel-safe.
	t.Setenv(ktEnvKubeconfig, writeKubeconfig(t, `apiVersion: v1
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
`))

	_, err := NewClientManager()
	if err == nil {
		t.Fatal("expected error from clientset build with invalid server URL")
	}
}
