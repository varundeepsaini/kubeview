package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// errUnknownContext is returned by ClientFor when the requested context name is
// not present in the loaded kubeconfig. Handlers map it to HTTP 400 so a bad
// ?context= value is a client error, not a server error.
var errUnknownContext = errors.New("unknown context")

// ContextInfo describes one kubeconfig context for the frontend dropdown. JSON
// tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts.
type ContextInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	Current bool   `json:"current"`
}

// clientBuilder builds a *Client for a named kubeconfig context. It is a struct
// field (not a hardcoded call) so tests can inject a fake-backed builder
// without a real kubeconfig or cluster.
type clientBuilder func(name string) (*Client, error)

// ClientManager enumerates the kubeconfig contexts and hands out a *Client per
// context, building each lazily on first use and caching it thereafter. It is
// safe for concurrent use by multiple HTTP handlers.
type ClientManager struct {
	clients        map[string]*Client
	build          clientBuilder
	defaultContext string
	contexts       []ContextInfo
	mu             sync.Mutex
}

// NewClientManager loads the kubeconfig (honoring KUBECONFIG, colon-separated
// path lists, and the ~/.kube/config default) and prepares a lazy per-context
// client cache. When no kubeconfig is available it falls back to the in-cluster
// service account, exposing a single implicit "in-cluster" context. The default
// (current) context's client is built eagerly so a broken default fails fast at
// startup, matching the previous single-client behavior.
func NewClientManager() (*ClientManager, error) {
	paths, useInCluster := resolveKubeconfigPaths()
	if useInCluster {
		return newInClusterManager()
	}

	loadingRules := loadingRulesFor(paths)
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		new(clientcmd.ConfigOverrides),
	)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	manager := new(ClientManager)
	manager.clients = make(map[string]*Client)
	manager.contexts = contextsFrom(rawConfig)
	manager.defaultContext = rawConfig.CurrentContext
	manager.build = func(name string) (*Client, error) {
		return buildClientForContext(loadingRules, rawConfig, name)
	}

	// Eagerly build the default context so a malformed current context is
	// caught at startup rather than on the first request.
	_, err = manager.ClientFor(emptyString)
	if err != nil {
		return nil, err
	}

	return manager, nil
}

// newInClusterManager wraps a single in-cluster client as a manager with one
// implicit context. There is nothing to switch to, so ?context= is effectively
// ignored (any other name is rejected as unknown).
func newInClusterManager() (*ClientManager, error) {
	config, kubeCtx, err := loadInClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	client := &Client{
		clientset: clientset,
		discovery: clientset.Discovery(),
		context:   kubeCtx.contextName,
		cluster:   kubeCtx.clusterName,
	}

	manager := new(ClientManager)
	manager.clients = map[string]*Client{inClusterName: client}
	manager.contexts = []ContextInfo{
		{Name: inClusterName, Cluster: inClusterName, Current: true},
	}
	manager.defaultContext = inClusterName
	// build stays nil: only one context, already cached, so it is never called.

	return manager, nil
}

// Contexts returns the enumerated kubeconfig contexts, sorted by name.
func (m *ClientManager) Contexts() []ContextInfo {
	return m.contexts
}

// ClientFor returns the client for the named context, building and caching it
// on first use. An empty name resolves to the default (current) context. A name
// not present in the kubeconfig yields errUnknownContext.
func (m *ClientManager) ClientFor(name string) (*Client, error) {
	if name == emptyString {
		name = m.defaultContext
	}

	if !m.hasContext(name) {
		return nil, fmt.Errorf("%w: %q", errUnknownContext, name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if client, ok := m.clients[name]; ok {
		return client, nil
	}

	client, err := m.build(name)
	if err != nil {
		return nil, err
	}

	m.clients[name] = client

	return client, nil
}

// hasContext reports whether name is one of the enumerated contexts.
func (m *ClientManager) hasContext(name string) bool {
	return slices.ContainsFunc(m.contexts, func(c ContextInfo) bool {
		return c.Name == name
	})
}

// contextsFrom flattens the kubeconfig's context map into a sorted slice,
// marking the current context. Sorting gives the UI (and tests) a stable order,
// since Go map iteration is randomized.
func contextsFrom(raw clientcmdapi.Config) []ContextInfo {
	out := make([]ContextInfo, zeroCount, len(raw.Contexts))
	for name, ctx := range raw.Contexts {
		out = append(out, ContextInfo{
			Name:    name,
			Cluster: ctx.Cluster,
			Current: name == raw.CurrentContext,
		})
	}

	slices.SortFunc(out, func(a, b ContextInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	return out
}

// buildClientForContext constructs a *Client pinned to a specific kubeconfig
// context via a CurrentContext override. Building does not contact the cluster
// (client-go connects lazily on the first API call), so an unreachable context
// caches fine and only errors when a request actually hits it.
func buildClientForContext(
	loadingRules *clientcmd.ClientConfigLoadingRules,
	raw clientcmdapi.Config,
	name string,
) (*Client, error) {
	overrides := new(clientcmd.ConfigOverrides)
	overrides.CurrentContext = name
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("rest config for context %q: %w", name, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("clientset for context %q: %w", name, err)
	}

	return &Client{
		clientset: clientset,
		discovery: clientset.Discovery(),
		context:   name,
		cluster:   clusterNameFor(raw, name),
	}, nil
}
