# KubeView

KubeView is a read-only browser interface for Kubernetes. It combines live resource inspection, pod-log streaming, kubeconfig context switching, and a bounded history of cluster state in one local application.

This document is the final user manual and installation guide for submission baseline `8ee601c`.

## Contents

- [Key features](#key-features)
- [Safety and scope](#safety-and-scope)
- [Prerequisites](#prerequisites)
- [Install from source](#install-from-source)
- [Run with Docker Compose](#run-with-docker-compose)
- [Deploy inside Kubernetes](#deploy-inside-kubernetes)
- [Configuration](#configuration)
- [User manual](#user-manual)
- [Installation validation](#installation-validation)
- [Troubleshooting](#troubleshooting)
- [Removal](#removal)

## Key features

- Dashboard with live pod, deployment, namespace, and node health summaries
- Searchable resource views with namespace filtering
- Namespaces, Pods, Deployments, Services, Nodes, Events, ConfigMaps, Secrets, Ingresses, StatefulSets, and DaemonSets
- Detailed pod inspection for containers, init containers, sidecars, ephemeral containers, conditions, volumes, labels, images, and restart counts
- Live multi-container pod logs with pause and resume
- Server-Sent Event updates backed by Kubernetes watch streams
- kubeconfig context switching without restarting the application
- 72-hour historical state retention by default
- Historical-state scrubber and comparison between two moments
- Read-only Kubernetes RBAC and an in-cluster NetworkPolicy

## Safety and scope

KubeView does not create, edit, scale, restart, or delete Kubernetes objects. The supplied Kubernetes role permits only `get`, `list`, and `watch` operations.

The backend has no built-in user authentication. It can return cluster state and pod logs, which may contain sensitive information. Do not expose the backend directly to an untrusted network. Put authentication and suitable network controls in front of any shared deployment.

The Secrets page displays metadata, key names, and byte lengths only. It does not return or reveal Secret values.

## Prerequisites

All installation methods require:

- Git
- `kubectl`
- a reachable Kubernetes cluster
- a valid kubeconfig context, unless KubeView is deployed in-cluster

Source installation additionally requires:

- Go 1.26.6 or a compatible Go 1.26 release
- Node.js 24 and npm

Container installation additionally requires Docker. A local `kind` deployment also requires `kind`.

Verify cluster access before starting:

```bash
kubectl cluster-info
kubectl get nodes
kubectl config get-contexts
```

Do not continue until the intended context can reach its cluster.

## Install from source

Clone the repository and check out the frozen submission baseline:

```bash
git clone https://github.com/varundeepsaini/kubeview.git
cd kubeview
git checkout 8ee601c
```

### Start the backend

In the first terminal:

```bash
cd kubeview-backend
go mod download
go run .
```

The API starts at <http://localhost:5501>. Verify it from another terminal:

```bash
curl http://localhost:5501/api/health
```

### Start the frontend

In a second terminal, from the repository root:

```bash
cd kubeview-frontend
npm ci
npm run dev
```

Open <http://localhost:5500>.

Stop each process with `Ctrl+C`.

## Run with Docker Compose

From the repository root:

```bash
docker compose up --build
```

Docker Compose mounts the host kubeconfig read-only, stores flight-recorder data in a named volume, starts the backend on port `5501`, and serves the frontend at <http://localhost:5500>.

To use a different kubeconfig file:

```bash
KUBECONFIG_HOST=/absolute/path/to/config docker compose up --build
```

On Linux, use the current host user and group when UID 1000 cannot read the kubeconfig:

```bash
UID=$(id -u) GID=$(id -g) docker compose up --build
```

The backend uses host networking so it can reach Kubernetes API servers bound to `127.0.0.1`. On Docker Desktop for macOS or Windows, enable host networking in **Settings > Resources > Network**. If it is unavailable, run the backend directly on the host instead.

Stop the stack with `Ctrl+C`, then remove the containers:

```bash
docker compose down
```

## Deploy inside Kubernetes

Build both images from the repository root:

```bash
docker build -t kubeview-backend:latest kubeview-backend/
docker build -t kubeview-frontend:latest kubeview-frontend/
```

For a local `kind` cluster, load the images and apply the manifests:

```bash
kind load docker-image kubeview-backend:latest kubeview-frontend:latest
kubectl apply -k deploy/kubernetes/
kubectl -n kubeview rollout status deployment/kubeview-backend
kubectl -n kubeview rollout status deployment/kubeview-frontend
```

Expose the services in two separate terminals:

```bash
kubectl -n kubeview port-forward svc/kubeview-backend 5501:5501
```

```bash
kubectl -n kubeview port-forward svc/kubeview-frontend 5500:5500
```

Open <http://localhost:5500>.

The frontend API address is compiled into the browser bundle. For a deployment that does not use local port forwarding, build the frontend with a backend URL that the user's browser can reach:

```bash
docker build \
  --build-arg NEXT_PUBLIC_API_BASE=https://kubeview-api.example.com/api \
  -t kubeview-frontend:1.0.0 kubeview-frontend/
```

Set backend `CORS_ORIGIN` to the exact frontend origin. Use immutable image tags for a shared or production deployment.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `5501` | Backend listening port |
| `KUBECONFIG` | `~/.kube/config` | Kubeconfig path list using the operating-system path separator |
| `CORS_ORIGIN` | `http://localhost:5500` | Comma-separated allowed frontend origins |
| `HISTORY_RETENTION_HOURS` | `72` | Retained history duration; zero or a negative value disables recording |
| `HISTORY_DIR` | User cache directory | Directory containing the local `history.db` file |
| `NEXT_PUBLIC_API_BASE` | `http://localhost:5501/api` | Backend URL compiled into the frontend bundle |

The selected kubeconfig context is sent with requests and stored in the browser. The backend must be restarted after kubeconfig context definitions change.

## User manual

### Start a session

1. Start the backend and frontend with one of the installation methods above.
2. Open <http://localhost:5500>.
3. Check the connection indicator in the sidebar.
4. Select the required **Context** when more than one kubeconfig context is available.
5. Confirm the cluster name, Kubernetes version, and platform on the dashboard.

Changing context reloads resource requests, live streams, and history for the selected cluster.

### Use the dashboard

The dashboard summarizes running pods, healthy deployments, namespaces, and ready nodes. These values update when Kubernetes watch events reach the browser. Use the sidebar to open detailed resource pages.

### Browse resources

The sidebar provides pages for:

- Namespaces
- Pods
- Deployments
- Services
- ConfigMaps
- Secrets
- Ingresses
- StatefulSets
- DaemonSets
- Events
- Nodes
- Timeline

Namespaced resource pages include a namespace selector and text search. Search matches the resource name and namespace within the loaded data. A context switch changes the cluster for the whole application.

### Inspect a pod

1. Open **Pods**.
2. Select the required namespace or search for the pod.
3. Select the pod name.
4. Review its status, node, pod IP, containers, images, restart counts, conditions, volumes, and labels.

KubeView distinguishes regular containers, init containers, restartable init sidecars, and ephemeral containers.

### Follow pod logs

1. Open a pod detail page.
2. Select **Logs**.
3. Select a container when the pod has more than one.
4. Select **Pause** to stop display updates while the stream continues buffering.
5. Select **Resume** to append the buffered lines and continue following output.

The browser keeps a bounded log tail. A reconnect replaces the displayed tail with the backend's new authoritative tail to avoid duplicated lines.

Logs are unavailable while viewing historical state because the flight recorder stores resource state and Kubernetes events, not application log streams.

### Inspect Secrets safely

The Secrets page lists Secret metadata, type, key names, and value lengths. Secret values are not returned by the backend.

Even metadata can be sensitive. Check visible names before sharing screenshots or recordings.

### Understand live updates

Resource pages load an initial REST snapshot and then subscribe to Server-Sent Events. Added, modified, and deleted resources update in place. If the stream reconnects, the frontend performs a fresh REST list to repair any missed state.

If data appears stale:

1. confirm that the backend is reachable;
2. confirm that the selected context still exists and can reach its cluster;
3. check the namespace and search filters; and
4. reload the browser page.

### View an earlier cluster state

The timeline bar appears after the backend records a usable history range.

1. Drag the timeline slider to an earlier timestamp.
2. Release the slider to load reconstructed state.
3. Confirm the amber **Viewing past** indicator before interpreting results.
4. Select **LIVE** to return to the current cluster.

History is isolated by Kubernetes context. The default retention is 72 hours, but the available range starts when recording begins and depends on persistence of `HISTORY_DIR`.

### Compare two moments

1. Open **Timeline**.
2. Choose **From** and **To** timestamps, or select **Last 15m**, **Last hour**, or **Last 6h**.
3. Select **Compare**.
4. Review added, removed, and modified resources.
5. Review Kubernetes events from the same interval below the changes.

Modified entries can identify image, restart-count, replica, and condition changes. The comparison describes recorded Kubernetes API state; it does not replace metrics, traces, audit logs, or application logs.

## Installation validation

Check the backend API:

```bash
curl http://localhost:5501/api/health
curl http://localhost:5501/api/contexts
curl http://localhost:5501/api/namespaces
```

For the in-cluster deployment, verify the service account can read but cannot mutate pods:

```bash
kubectl auth can-i list pods \
  --as=system:serviceaccount:kubeview:kubeview-backend
kubectl auth can-i delete pods \
  --as=system:serviceaccount:kubeview:kubeview-backend
```

The first command should return `yes`; the second should return `no`.

Open the dashboard and confirm that its cluster identity and resource counts match:

```bash
kubectl get nodes
kubectl get pods --all-namespaces
kubectl get deployments --all-namespaces
```

## Troubleshooting

### The interface reports `Failed to fetch`

Confirm that the backend is running, `NEXT_PUBLIC_API_BASE` points to a URL reachable by the browser, and backend `CORS_ORIGIN` exactly matches the frontend origin.

### A context is missing or unavailable

Run:

```bash
kubectl config get-contexts
kubectl --context CONTEXT_NAME get nodes
```

The context must exist and its cluster must be reachable from the backend host. Restart the backend after changing kubeconfig context definitions.

### A resource page is empty

Clear the search field, check the namespace filter, and verify the selected context. Compare the page with:

```bash
kubectl --context CONTEXT_NAME get RESOURCE --all-namespaces
```

Replace `CONTEXT_NAME` and `RESOURCE` with the intended values.

### History controls do not appear

History is hidden when recording is disabled or no usable time range exists. Confirm that `HISTORY_RETENTION_HOURS` is greater than zero, interact with the selected context, and inspect backend logs for history-store errors.

### Pod log streaming stops

The pod may have terminated, the selected container may not have started, or the connection may have closed. Reopen the Logs tab, select the correct container, or reload the pod page.

### Docker cannot reach the cluster

The kubeconfig may point to an API server on `127.0.0.1`. Enable Docker Desktop host networking or run the backend directly on the host. Also confirm that the mounted kubeconfig is readable by the configured container UID.

## Removal

Remove the in-cluster deployment:

```bash
kubectl delete -k deploy/kubernetes/
```

Remove Docker Compose containers:

```bash
docker compose down
```

To remove the Docker history volume as well:

```bash
docker compose down --volumes
```

Delete a locally configured `HISTORY_DIR` only when its recorded data is no longer required.

## Project reference

- Repository: <https://github.com/varundeepsaini/kubeview>
- Submission baseline: `8ee601c`
- Academic year: 2025-2026
