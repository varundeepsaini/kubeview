# kubeview-backend-go

Go reimplementation of the KubeView backend. Drop-in replacement for `kubeview-backend/`: same REST API surface, same port (`5501`), so the existing `kubeview-frontend/` works against it without changes.

## Prerequisites

- Go 1.22 or later (uses the [Go 1.22 ServeMux pattern syntax](https://pkg.go.dev/net/http#hdr-Patterns))
- A reachable Kubernetes cluster — `kubectl get nodes` should succeed

## Run

```bash
cd kubeview-backend-go
go run .
```

The API listens on `http://localhost:5501`.

## Build

```bash
go build -o kubeview-api .
./kubeview-api
```

A single static binary, no runtime dependencies.

## Configuration

| Env var | Default | Purpose |
| --- | --- | --- |
| `PORT` | `5501` | TCP port the API listens on |
| `KUBECONFIG` | `~/.kube/config` | Path to the kubeconfig to load |

Both follow the same conventions as `kubectl`.

## File layout

| File | Responsibility |
| --- | --- |
| `main.go` | Server bootstrap, timeouts, graceful shutdown |
| `kube.go` | Kubernetes clientset + thin wrappers around list/get calls |
| `transformers.go` | Response structs + functions converting K8s objects to the frontend JSON shape |
| `handlers.go` | HTTP handlers, router, CORS middleware, error helpers |

## API

Identical to the Node.js backend — see the top-level `README.md` for the endpoint table.

## Differences from the Node.js backend

- Built as a single static binary instead of a `node_modules` tree.
- HTTP server has explicit read/write/idle timeouts and graceful shutdown on `SIGINT`/`SIGTERM`.
- Kubernetes API errors with a `Status` payload (e.g. 404) propagate their HTTP status to the response instead of always returning 500.
