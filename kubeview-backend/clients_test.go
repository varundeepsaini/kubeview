package main

import (
	"errors"
	"path/filepath"
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

	_, err = manager.ClientFor(emptyString)
	if err != nil {
		t.Fatalf("default context client: %v", err)
	}

	_, err = manager.ClientFor("missing")
	if !errors.Is(err, errUnknownContext) {
		t.Fatalf("err = %v, want errUnknownContext", err)
	}
}
