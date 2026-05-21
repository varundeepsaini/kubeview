# kubeview-backend

Go backend for KubeView. Serves the REST API on port `5501` consumed by `kubeview-frontend/`.

## Prerequisites

- Go 1.22 or later (uses the [Go 1.22 ServeMux pattern syntax](https://pkg.go.dev/net/http#hdr-Patterns))
- A reachable Kubernetes cluster — `kubectl get nodes` should succeed

## Run

```bash
cd kubeview-backend
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

All endpoints return JSON. List endpoints accept an optional `?namespace=<ns>` query parameter.

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/health` | Liveness probe |
| GET | `/api/cluster` | Cluster info (version, platform, node count, current context) |
| GET | `/api/namespaces` | List namespaces |
| GET | `/api/pods` | List pods |
| GET | `/api/pods/{namespace}/{name}` | Single pod detail |
| GET | `/api/pods/{namespace}/{name}/logs` | Pod logs (`?container=<name>`, `?tailLines=<n>`) |
| GET | `/api/deployments` | List deployments |
| GET | `/api/services` | List services |
| GET | `/api/nodes` | List nodes |
| GET | `/api/events` | List events |

CORS is whitelisted to `http://localhost:5500` (the frontend dev server).

## Tests

```bash
go test ./...           # full suite
go test -race ./...     # with race detector
go test -cover ./...    # with coverage
```
