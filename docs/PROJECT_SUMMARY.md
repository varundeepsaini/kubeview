# KubeView

## Project summary

- **Programme:** BSc Computer Science, BITS Pilani Digital
- **Academic year:** 2025-2026
- **Project type:** Group capstone project

**Students:**

- Ankur Kalita (`2023EBCS782`)
- Pradyut Fogla (`2023EBCS788`)
- Varun Deep Saini (`2023EBCS663`)

- **Source code:** <https://github.com/varundeepsaini/kubeview>
- **Submission baseline:** `8ee601c`

## Executive summary

KubeView is a read-only web application for inspecting Kubernetes clusters. It combines a Go backend, a Next.js frontend, live resource updates, pod-log streaming, kubeconfig context switching, and a local history recorder. The application helps a developer or cluster operator examine current workloads and reconstruct recent cluster changes from one browser interface.

Kubernetes provides detailed operational data, but routine diagnosis often requires several `kubectl` commands. An operator may inspect resources, describe a pod, read events, follow logs, and switch contexts during a single investigation. These commands are effective, but the information is fragmented and mainly describes the present. Short failures and rollout transitions become difficult to study after the cluster state changes.

Full observability platforms provide long-term metrics, logs, traces, dashboards, and alerts. They are appropriate for production operations, but they also require collectors, storage, configuration, and maintenance. KubeView addresses a narrower need: immediate, low-overhead inspection with a bounded history of Kubernetes resource state.

The backend uses the official Kubernetes `client-go` library and owns all Kubernetes credentials. The browser never receives a kubeconfig or service-account token. Production manifests grant only `get`, `list`, and `watch` permissions for the supported resources and pod logs. KubeView therefore remains an inspection tool and does not create, modify, scale, restart, or delete workloads.

## Problem statement

The Kubernetes API exposes the information needed to investigate cluster health, but the default workflow separates that information across commands and terminal sessions. This creates three practical problems:

1. **Fragmented diagnosis:** resource health, events, configuration, container state, and logs must be correlated manually.
2. **Present-state bias:** once a resource changes or disappears, its earlier state is unavailable unless another system recorded it.
3. **Operational overhead:** a complete observability stack is often disproportionate for a development cluster, classroom environment, demonstration, or focused incident review.

The project asks a focused question: how can a user answer both "what is wrong now?" and "what changed before it?" without deploying a large monitoring platform or granting a dashboard mutation privileges?

## Unique value proposition

> **KubeView combines live Kubernetes inspection and lightweight historical replay in a local, read-only browser workflow, without requiring a separate metrics or log-monitoring stack.**

The UVP is supported by four concrete design choices:

1. **Live rather than repeatedly polled:** the browser loads an initial REST snapshot and receives later changes through Server-Sent Events backed by Kubernetes watch streams.
2. **Recent state can be reconstructed:** the flight recorder stores bounded resource versions and deletion tombstones in a local bbolt database. Retention is 72 hours by default.
3. **Multiple clusters use one interface:** kubeconfig contexts can be selected without restarting the application. Requests, live streams, and history remain isolated by context.
4. **Read-only operation is enforced:** the application exposes inspection routes only, while deployment RBAC permits `get`, `list`, and `watch` operations.

KubeView does not claim to replace Prometheus, Grafana, Loki, distributed tracing, audit logging, alerting, or a full administration console. Its value comes from keeping the diagnostic workflow focused and the deployment footprint small.

## Objectives and scope

The project objectives were to:

- provide a browser interface for common Kubernetes resources;
- support namespace filtering and resource search;
- display detailed pod, container, condition, volume, and restart information;
- stream logs for a selected pod container;
- update resource pages from Kubernetes watch events;
- switch safely between configured kubeconfig contexts;
- record a bounded history of transformed cluster state;
- reconstruct state at a selected time and compare two moments;
- deploy from source, with Docker Compose, or inside Kubernetes;
- enforce read-only cluster access; and
- validate the system through backend, frontend, security, and real-cluster tests.

The implemented resource scope includes Namespaces, Pods, Deployments, Services, Nodes, Events, ConfigMaps, Secrets, Ingresses, StatefulSets, and DaemonSets. The Secrets view exposes metadata, key names, and value lengths, but not Secret values.

Workload mutation, built-in user authentication, metrics, traces, alerts, custom resource definitions, and long-term log retention are outside the current scope.

## System design

KubeView follows a two-application architecture:

```text
Browser -> Next.js frontend -> Go backend -> Kubernetes API
                              -> local bbolt history store
```

The **frontend** is built with Next.js 16, React 19, TypeScript 5, and Tailwind CSS 4. It renders the dashboard, resource lists, pod details, context selector, historical-state control, and timeline comparison. Initial data is fetched through REST. Live pages then subscribe to a multiplexed `EventSource` connection so additions, modifications, and deletions can update the interface without reloading the page.

The **backend** is built with Go 1.26 and the standard `net/http` package. Kubernetes access uses `client-go`. The backend loads one or more kubeconfig contexts when run locally and uses the mounted service account when deployed inside Kubernetes. Raw Kubernetes objects are transformed into smaller response structures intended for the user interface.

The **history subsystem** records transformed resource versions, Kubernetes events, and deletion tombstones in bbolt. A state request reconstructs the latest recorded version of each object at a selected timestamp. A diff request compares two reconstructed states and reports added, removed, and modified resources, including changes such as container images, restart counts, replicas, and conditions.

## Key features

### Cluster dashboard

The dashboard summarizes pod health, deployment readiness, namespaces, and node readiness. It also identifies the selected context, Kubernetes version, and platform so that the user can confirm the target cluster before investigation.

### Resource inspection

Resource pages provide searchable tables and namespace filters. Pod detail includes regular containers, init containers, restartable init sidecars, ephemeral containers, images, restart counts, conditions, volumes, labels, node assignment, and pod IP.

### Live pod logs

The pod detail page streams logs from a selected container. Multi-container pods use an explicit container selector. The user can pause display updates and resume later; buffered lines are appended in order. The browser retains a bounded tail to avoid unbounded memory growth.

### Live resource updates

Kubernetes watches are forwarded to the browser with Server-Sent Events. The frontend multiplexes resource subscriptions to avoid opening a separate browser connection for every resource type. A fresh REST list repairs state after reconnection.

### Multi-context operation

The context selector lists contexts available to the backend. Selecting another context reloads resource requests, live streams, and historical data for that cluster. The backend resolves a Kubernetes client per request instead of changing global process state.

### Flight recorder and timeline

The timeline scrubber can reconstruct the cluster at an earlier recorded moment. The comparison page accepts two timestamps or presets such as the last 15 minutes, hour, or six hours. It reports resource additions, removals, field-level modifications, and Kubernetes events observed in the selected interval.

## Security approach

The application is read-only by design. The supplied Kubernetes `ClusterRole` grants `get`, `list`, and `watch` for the displayed resource types and pod logs. It contains no mutation verbs. The frontend pod does not receive a service-account token. A NetworkPolicy restricts backend ingress to the frontend pod in the supplied in-cluster deployment.

The backend does not include user authentication. This is an explicit deployment boundary rather than a hidden assumption. Cluster state and pod logs may contain sensitive information, so the backend must not be exposed to an untrusted network without an authentication proxy and appropriate network controls. The history database must also be stored on protected storage.

## Validation

Validation is layered across the application:

- **227 Go test functions** cover HTTP handlers, Kubernetes clients, object transformation, context isolation, live streams, history retention, tombstones, reconstruction, and diff logic.
- **60 frontend unit tests** use Vitest and Testing Library to verify components, resource pages, context behavior, historical mode, and timeline interaction.
- **45 Playwright tests** run the real frontend and backend against a real `kind` cluster and exercise browser workflows.
- Backend CI runs compilation, formatting checks, `go vet`, the race detector, coverage collection, staticcheck, golangci-lint, and govulncheck.
- Frontend CI runs TypeScript checking, ESLint, unit tests, and a production build.
- CodeQL scans the Go and JavaScript/TypeScript code.
- Deployment checks cover read-only RBAC, container builds, Kubernetes manifests, health probes, and real-cluster execution.

The retained CI evidence corresponds to the frozen submission baseline. The reported numbers are test counts, not code-coverage percentages.

## Outcomes

The project produced a working Kubernetes inspection tool with a deliberately limited operational boundary. It demonstrates that Kubernetes API primitives, browser event streams, and a small embedded store can support useful live and historical diagnosis without introducing a full observability platform.

The main technical outcomes are:

- one interface for live cluster resources, pod detail, events, and logs;
- automatic browser updates driven by Kubernetes watches;
- safe context isolation across configured clusters;
- bounded historical reconstruction with deletion handling;
- field-level comparison between two recorded moments; and
- deployment controls that preserve read-only access.

## Limitations and future work

Current limitations include the absence of built-in authentication, single-writer local history storage, bounded rather than permanent retention, and a lack of metrics, traces, alerts, audit logs, and long-term log storage. Large-cluster performance and accessibility require additional formal evaluation.

Future work can add an authenticated gateway, per-user authorization, shared history storage, larger-scale load testing, formal accessibility testing, metrics integration, and custom resource support. These additions should preserve the central product boundary: KubeView is an inspection and reconstruction tool, not a cluster mutation console.
