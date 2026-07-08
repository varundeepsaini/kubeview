# Multi-context / multi-cluster switching

Resolves varundeepsaini/kubeview#15.

## Goal

Let a user with several kubeconfig contexts (dev/staging/prod) switch between
them from the UI, without restarting the backend. An unreachable context must
surface as a per-context error, never take down the whole UI.

## Decisions

- **Context passing:** a `?context=<name>` query parameter on every route
  (stateless, matches the existing `?namespace=` pattern, works across browser
  tabs). No server-side "current context" session state.
- **Frontend persistence:** `localStorage` only. Survives refresh; no routing
  changes. (URL deep-linking is explicitly out of scope for this change.)
- **Eviction on kubeconfig change:** out of scope. Clients are cheap to cache
  and kubeconfig rarely changes mid-session; a backend restart picks up
  changes. Noted here as a deliberate deferral rather than a file-watcher.

## Backend

The backend currently builds one `*Client` at startup (`NewClient` →
`loadKubeConfig`) and every handler closes over it. We introduce a
`ClientManager` that owns a lazily-populated, mutex-guarded per-context cache
of `*Client`, and thread it through the router.

### ClientManager (`clients.go`, new file)

```go
type ContextInfo struct {
    Name    string `json:"name"`
    Cluster string `json:"cluster"`
    Current bool   `json:"current"`
}

type ClientManager struct {
    mu             sync.Mutex
    clients        map[string]*Client        // context name -> built client
    contexts       []ContextInfo             // enumerated once at startup
    defaultContext string                    // used when ?context= is absent
    build          func(name string) (*Client, error) // per-context client factory
}
```

Two construction paths, mirroring the existing `loadKubeConfig` logic:

- **Kubeconfig mode:** load the raw config once (honoring `KUBECONFIG`, colon
  lists, and the `~/.kube/config` default). Populate `contexts` from
  `rawConfig.Contexts` (name + cluster; `current` = matches
  `rawConfig.CurrentContext`). `defaultContext = rawConfig.CurrentContext`.
  The `build` factory constructs a client for a named context via
  `clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules,
  &clientcmd.ConfigOverrides{CurrentContext: name})`.
- **In-cluster mode:** a single implicit context named `in-cluster`.
  `contexts` is `[{in-cluster, in-cluster, current:true}]`; `?context=` is
  ignored (only one client exists).

### API of the manager

- `Contexts() []ContextInfo` — returns the enumerated list.
- `ClientFor(name string) (*Client, error)`:
  - `name == ""` → use `defaultContext`.
  - unknown name (not in `contexts`) → return `errUnknownContext` (sentinel).
  - otherwise return the cached client, building and caching it on first use.
  - concurrency-safe via `mu`.

Building a client does **not** contact the cluster (client-go connects lazily
on the first API call), so an unreachable context caches fine and only errors
when a request actually hits it — giving us per-context error isolation for
free.

### Router / handlers (`handlers.go`)

- `newRouter(*Client)` → `newRouter(*ClientManager)`.
- New route: `GET /api/contexts` → `handleContexts(mgr)` → `mgr.Contexts()`.
- A small `withClient` wrapper resolves the per-request client once, so the
  ~9 resource handlers don't each repeat the resolve/error block:

  ```go
  func withClient(mgr *ClientManager,
      fn func(*Client, http.ResponseWriter, *http.Request)) http.HandlerFunc {
      return func(w http.ResponseWriter, r *http.Request) {
          client, err := mgr.ClientFor(r.URL.Query().Get(paramContext))
          if err != nil {
              writeJSONError(w, http.StatusBadRequest, "Unknown context")
              return
          }
          fn(client, w, r)
      }
  }
  ```

  Each existing `handleX(client *Client) http.HandlerFunc` becomes
  `handleX(client *Client, w http.ResponseWriter, r *http.Request)` and is
  registered as `withClient(mgr, handleX)`. `handleHealth` needs no client and
  is unchanged.
- New constant `paramContext = "context"`.

### main.go

`run()` calls `NewClientManager()` instead of `NewClient()`, and
`newRouter(manager)`. Startup still fails fast if the kubeconfig cannot be
loaded at all.

## Frontend

### api.ts

- Module-level `currentContext` string with `setApiContext(name: string)`.
- `fetchApi` appends `context=<name>` to the query string when
  `currentContext` is non-empty. All existing `api.getX` signatures stay the
  same (the context is ambient, like a header).
- New `interface ContextInfo { name; cluster; current }` and
  `api.getContexts()`.

### ClusterProvider (`components/ClusterProvider.tsx`, new)

A client-side React context provider mounted in the layout:

- Holds `context` (string) + `setContext`, initialized from
  `localStorage["kubeview.context"]` (default `""` = backend's current).
- On change: persist to `localStorage` and call `setApiContext`.
- Exposes `{ context, setContext }` via `useCluster()`.

### Clear/refetch on switch

The layout wraps `{children}` in a element keyed by the selected context:
`<main key={context}>`. Switching context remounts the subtree, so every
`usePolling` hook re-runs from scratch — stale data is cleared and refetched
with no manual cache-busting. (Requires the layout, or a thin client wrapper
inside it, to read `useCluster()`.)

### Sidebar.tsx

- A context `<select>` fed by `api.getContexts()`, showing each context (the
  current one marked), calling `setContext` on change.
- The "Connected" footer shows the active context name.
- Hidden/degrades gracefully when only one context exists (e.g. in-cluster).

## Error handling

- Unknown context → `400` from `withClient`.
- Unreachable/again-down context → the k8s client error surfaces through the
  existing `writeError` on that context's requests only; each page renders it
  in its existing `ErrorMessage`. The `/api/contexts` list is read from
  kubeconfig (not a live cluster), so the dropdown always works and the user
  can switch back.

## Testing

- **Backend unit (`clients_test.go`, new):** `ClientFor` default resolution,
  unknown-context error, cache reuse (same pointer on repeated calls),
  `Contexts()` enumeration + `current` flag — using an injected `build`
  factory backed by a fake clientset (no real kubeconfig).
- **Backend handler tests:** a `GET /api/contexts` test; update existing
  handler tests to construct a `*ClientManager` (via a test helper that wraps
  an existing fake-clientset `*Client`) instead of passing a bare `*Client` to
  `newRouter`. A `?context=unknown` → 400 test.
- **Frontend:** manual verification against the Docker Desktop cluster
  (single context) plus a temporary second context to confirm switching,
  persistence across refresh, and per-context error isolation.

## Out of scope

- Kubeconfig file watching / cache eviction.
- URL-based context deep-linking.
- Write operations against a context (the API stays read-only).
