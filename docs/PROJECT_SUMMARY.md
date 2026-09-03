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

KubeView is a read-only web application for inspecting Kubernetes clusters. A Go backend and a Next.js frontend together give a developer or operator one browser interface for current workloads, live resource updates, streaming pod logs, kubeconfig context switching, and a local recording of recent cluster changes.

Kubernetes exposes plenty of operational data, but day-to-day diagnosis usually means a string of `kubectl` commands: list resources, describe a pod, read events, follow logs, maybe switch contexts, all in one investigation. Each command works, but the information arrives fragmented, and almost all of it describes the present. A short-lived failure or a rollout transition is hard to study once the cluster has moved on.

Full observability platforms solve that with long-term metrics, logs, traces, dashboards, and alerts. They're the right choice for production operations, but they bring collectors, storage, configuration, and maintenance with them. KubeView aims at a narrower need: immediate, low-overhead inspection, plus a bounded history of Kubernetes resource state.

The backend uses the official `client-go` library and holds all Kubernetes credentials itself — the browser never sees a kubeconfig or a service-account token. The production manifests grant only `get`, `list`, and `watch` on the supported resources and pod logs. KubeView stays an inspection tool: it cannot create, modify, scale, restart, or delete anything.

## Problem statement

The Kubernetes API has the information you need to investigate cluster health, but the default workflow scatters it across commands and terminal sessions. Three practical problems fall out of that:

1. **Fragmented diagnosis:** resource health, events, configuration, container state, and logs have to be correlated by hand.
2. **Present-state bias:** once a resource changes or disappears, its earlier state is gone unless something else recorded it.
3. **Operational overhead:** a full observability stack is overkill for a development cluster, a classroom, a demo, or a focused incident review.

The question the project set out to answer: how do you let a user answer both "what is wrong now?" and "what changed before it?" without deploying a large monitoring platform or handing a dashboard mutation privileges?

## Unique value proposition

> **KubeView combines live Kubernetes inspection and lightweight historical replay in a local, read-only browser workflow, without requiring a separate metrics or log-monitoring stack.**

Four design choices back this up:

1. **Live rather than repeatedly polled:** the browser loads an initial REST snapshot, then receives changes over Server-Sent Events backed by Kubernetes watch streams.
2. **Recent state can be reconstructed:** the flight recorder stores bounded resource versions and deletion tombstones in a local bbolt database, with 72-hour retention by default.
3. **One interface for multiple clusters:** kubeconfig contexts can be switched without restarting the application, and requests, live streams, and history stay isolated per context.
4. **Read-only is enforced, not promised:** the application only exposes inspection routes, and the deployment RBAC only permits `get`, `list`, and `watch`.

KubeView doesn't claim to replace Prometheus, Grafana, Loki, distributed tracing, audit logging, alerting, or a full administration console. Its value is in keeping the diagnostic workflow in one place and the deployment footprint small.

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
- enforce read-only cluster access;
- validate the system through backend, frontend, security, and real-cluster tests.

The implemented resource scope covers Namespaces, Pods, Deployments, Services, Nodes, Events, ConfigMaps, Secrets, Ingresses, StatefulSets, and DaemonSets. The Secrets view shows metadata, key names, and value lengths — never the values themselves.

Workload mutation, built-in user authentication, metrics, traces, alerts, custom resource definitions, and long-term log retention are all outside the current scope.

## System design

KubeView is two applications:

```text
Browser -> Next.js frontend -> Go backend -> Kubernetes API
                              -> local bbolt history store
```

The **frontend** is built with Next.js 16, React 19, TypeScript 5, and Tailwind CSS 4. It renders the dashboard, resource lists, pod details, the context selector, the historical-state control, and the timeline comparison. Pages fetch initial data over REST, then subscribe to a multiplexed `EventSource` connection so additions, modifications, and deletions show up without a reload.

The **backend** is Go 1.26 on the standard `net/http` package, with `client-go` for Kubernetes access. Run locally, it loads one or more kubeconfig contexts; deployed inside Kubernetes, it uses the mounted service account. Raw Kubernetes objects are trimmed down into smaller response structures shaped for the UI.

The **history subsystem** records transformed resource versions, Kubernetes events, and deletion tombstones in bbolt. A state request rebuilds the latest recorded version of each object at a chosen timestamp. A diff request compares two rebuilt states and reports added, removed, and modified resources — container images, restart counts, replicas, conditions, and so on.

## Key features

### Cluster dashboard

The dashboard summarises pod health, deployment readiness, namespaces, and node readiness. It also shows the selected context, Kubernetes version, and platform, so the user can confirm which cluster they're looking at before digging in.

### Resource inspection

Resource pages are searchable tables with namespace filters. Pod detail covers regular containers, init containers, restartable init sidecars, ephemeral containers, images, restart counts, conditions, volumes, labels, node assignment, and pod IP.

### Live pod logs

The pod detail page streams logs from a selected container, with an explicit container selector for multi-container pods. Display updates can be paused and resumed; buffered lines are appended in order on resume. The browser keeps only a bounded tail, so memory doesn't grow without limit.

### Live resource updates

Kubernetes watches are forwarded to the browser over Server-Sent Events. The frontend multiplexes its subscriptions so it doesn't open a separate connection per resource type, and a fresh REST list repairs state after any reconnect.

### Multi-context operation

The context selector lists whatever contexts the backend can see. Picking another one reloads resource requests, live streams, and historical data for that cluster. Internally the backend resolves a Kubernetes client per request rather than mutating global process state.

### Flight recorder and timeline

The timeline scrubber rebuilds the cluster at an earlier recorded moment. The comparison page takes two timestamps, or a preset (last 15 minutes, hour, or six hours), and reports resource additions, removals, field-level modifications, and the Kubernetes events observed in that interval.

## Security approach

The application is read-only by design. The supplied `ClusterRole` grants `get`, `list`, and `watch` for the displayed resource types and pod logs, and nothing else — no mutation verbs. The frontend pod receives no service-account token at all, and in the supplied in-cluster deployment a NetworkPolicy restricts backend ingress to the frontend pod.

There is no built-in user authentication. That's a stated deployment boundary, not a hidden assumption. Cluster state and pod logs can contain sensitive information, so the backend must not face an untrusted network without an authentication proxy and proper network controls in front of it, and the history database belongs on protected storage.

## Validation

Validation is layered:

- **227 Go test functions** cover HTTP handlers, Kubernetes clients, object transformation, context isolation, live streams, history retention, tombstones, reconstruction, and diff logic.
- **70 frontend unit tests** (Vitest and Testing Library) verify components, resource pages, context behaviour, historical mode, and timeline interaction.
- **45 Playwright tests** run the real frontend and backend against a real `kind` cluster and drive actual browser workflows.
- Backend CI runs compilation, formatting checks, `go vet`, the race detector, coverage collection, staticcheck, golangci-lint, and govulncheck.
- Frontend CI runs TypeScript checking, ESLint, unit tests, and a production build.
- CodeQL scans the Go and JavaScript/TypeScript code.
- Deployment checks cover read-only RBAC, container builds, Kubernetes manifests, health probes, and real-cluster execution.

The retained CI evidence corresponds to the frozen submission baseline. The numbers above are test counts, not code-coverage percentages.

## Outcomes

The project produced a working Kubernetes inspection tool with a deliberately narrow operational boundary. It shows that Kubernetes API primitives, browser event streams, and a small embedded store are enough for useful live and historical diagnosis — no full observability platform required.

The main technical outcomes:

- one interface for live cluster resources, pod detail, events, and logs;
- automatic browser updates driven by Kubernetes watches;
- safe context isolation across configured clusters;
- bounded historical reconstruction with deletion handling;
- field-level comparison between two recorded moments;
- deployment controls that keep access read-only.

## Limitations and future work

The current limitations: no built-in authentication, single-writer local history storage, bounded rather than permanent retention, and no metrics, traces, alerts, audit logs, or long-term log storage. Large-cluster performance and accessibility haven't been formally evaluated yet.

Future work could add an authenticated gateway, per-user authorization, shared history storage, larger-scale load testing, formal accessibility testing, metrics integration, and custom resource support. Whatever gets added should keep the central boundary intact: KubeView is an inspection and reconstruction tool, not a cluster mutation console.
