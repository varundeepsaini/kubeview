# COVER PAGE

| | |
| --- | --- |
| **Project Title** | KubeView: A Kubernetes Cluster Dashboard with Time-Travel Observability |
| **Student Name(s) & Roll Number(s)** | Ankur Kalita, 2023EBCS782<br>Pradyut Fogla, 2023EBCS788<br>Varun Deep Saini, 2023EBCS663 |
| **Program** | BSc Computer Science (Online Mode) |
| **Institution Name** | Birla Institute of Technology & Science, Pilani |
| **Academic Year** | 2025–2026 |
| **Internal Supervisor Name** | Charan |

---

# Declaration

We hereby declare that this capstone project titled "KubeView: A Kubernetes Cluster Dashboard with Time-Travel Observability" is an original work carried out by us and has not been submitted to any other university or institution for the award of any degree.

| Name | Roll Number | Signature |
| --- | --- | --- |
| Ankur Kalita | 2023EBCS782 | |
| Pradyut Fogla | 2023EBCS788 | |
| Varun Deep Saini | 2023EBCS663 | |

Date: 28 August 2026

---

# Abstract

Kubernetes has become the default way to run containerised applications, but its main interface is still the `kubectl` command line. It's hard to learn and gives no at-a-glance picture of cluster health. Web dashboards exist, but they tend to be heavy to install, need auth infrastructure before they show anything, and only ever display the current state of the cluster. So the question an operator actually asks after an incident — what changed, and when? — goes unanswered.

KubeView is a lightweight, read-only web dashboard for Kubernetes clusters. A Go backend connects to the cluster through the standard kubeconfig using the official `client-go` library, and a Next.js/React frontend renders pods, deployments, services, StatefulSets, DaemonSets, ConfigMaps, Secrets, Ingresses, nodes, namespaces, events, and live pod logs. Updates reach the browser over Server-Sent Events, and the user can switch between clusters at runtime via kubeconfig contexts.

The part we consider the project's real contribution is the cluster flight recorder. The backend continuously writes resource state changes and events into an embedded bbolt store (a 72-hour ring buffer by default). A timeline scrubber then re-renders every page as of any past moment, and a diff view lists what changed between two points in time — pod restarts, image changes, replica scaling, condition transitions — next to the events that fired in between.

The system is covered by 227 backend Go tests, 70 frontend unit tests (Vitest/Testing Library), and 45 Playwright end-to-end tests; the E2E tests run against a real kind cluster in CI. Every commit also passes static analysis (golangci-lint, CodeQL) and vulnerability scanning (govulncheck). KubeView deploys as two containers, either through Docker Compose or with in-cluster Kubernetes manifests carrying a least-privilege, read-only RBAC role.

---

# Table of Contents

1. [Chapter 1: Introduction](#chapter-1-introduction)
2. [Chapter 2: Implementation Details](#chapter-2-implementation-details)
3. [Chapter 3: Testing, Validation & Results](#chapter-3-testing-validation--results)
4. [Chapter 4: Execution / Deployment Details](#chapter-4-execution--deployment-details)
5. [Chapter 5: Project Execution Evidence](#chapter-5-project-execution-evidence)
6. [Chapter 6: Conclusion & Future Work](#chapter-6-conclusion--future-work)
7. [References](#references)
8. [Appendix](#appendix)

# List of Figures

| Figure | Title |
| --- | --- |
| Fig. 1 | High-level system architecture |
| Fig. 2 | Data flow diagram |
| Fig. 3 | Flight recorder component interaction |
| Fig. 4 | Dashboard overview screenshot |
| Fig. 5 | Pods list screenshot |
| Fig. 6 | Pod detail with live logs screenshot |
| Fig. 7 | Timeline diff view screenshot |
| Fig. 8 | Commit history screenshot |

# List of Tables

| Table | Title |
| --- | --- |
| Table 1 | Technology stack |
| Table 2 | Backend REST API endpoints |
| Table 3 | Configuration environment variables |
| Table 4 | Test suite summary |
| Table 5 | Representative test cases |
| Table 6 | Weekly progress summary |

# List of Abbreviations

| Abbreviation | Expansion |
| --- | --- |
| API | Application Programming Interface |
| CI/CD | Continuous Integration / Continuous Delivery |
| CORS | Cross-Origin Resource Sharing |
| CRUD | Create, Read, Update, Delete |
| E2E | End-to-End (testing) |
| JSON | JavaScript Object Notation |
| K8s | Kubernetes |
| RBAC | Role-Based Access Control |
| REST | Representational State Transfer |
| SSE | Server-Sent Events |
| UI | User Interface |
| UVP | Unique Value Proposition |

---

# CHAPTER 1: INTRODUCTION

> Note: Problem identification and system design were completed as part of the Study Project. This chapter summarises that work and records what changed during implementation.

## 1.1 Overview of the Project

KubeView is a self-hosted web dashboard for watching Kubernetes clusters — a browser-friendly companion to `kubectl get`. You point it at your kubeconfig and get a live, searchable, filterable view of every major workload and configuration resource in the cluster, plus streaming pod logs. Nothing gets installed inside the cluster itself.

It also remembers. The flight recorder captures every resource state change and event into an embedded store, so you can scrub a timeline and see the cluster exactly as it was at any moment inside the retention window, or diff two moments and see what changed along with the events that came with the change.

KubeView never writes to the cluster. That one decision keeps the attack surface small and makes it safe to point at production.

## 1.2 Problem Statement & Motivation

Running a Kubernetes cluster means answering questions like: which pods are unhealthy right now, and why? What's in the logs of this crashing container? The service broke at 2 a.m. — what changed around that time?

The standard tooling answers these badly, for three separate reasons.

First, `kubectl` is powerful but opaque. Working out "what is unhealthy" takes several chained commands and a mental join of their output. Newcomers find it hostile, and even experienced operators find it slow for exploration.

Second, existing dashboards only show the present. The official Kubernetes Dashboard, Lens, and similar tools display current state. Once a pod is replaced, or an event ages out (Kubernetes keeps events for roughly an hour), the evidence of what happened is simply gone. After that, post-incident analysis depends on an external observability stack — Prometheus with Grafana and Loki, or a commercial APM — which is a lot of setup for a small team or a homelab.

Third, most dashboards have to be deployed into the cluster, with elevated RBAC, an auth proxy, and ingress configuration, before they show a single pod.

We wanted a tool that runs locally with zero in-cluster installation, is read-only by construction, and answers the "what changed?" question out of the box. No lightweight dashboard we found does that last part.

## 1.3 Objectives of the Capstone

1. Build a Go REST API that serves all major Kubernetes resource kinds (pods, deployments, services, nodes, namespaces, events, ConfigMaps, Secrets, Ingresses, StatefulSets, DaemonSets) through the official `client-go` library.
2. Build a React/Next.js frontend with search, namespace filtering, status badges, and detail views including live pod log tailing.
3. Support multi-cluster switching at runtime from the kubeconfig contexts.
4. Push live updates to the browser via Server-Sent Events instead of relying on polling alone.
5. Implement the cluster flight recorder: continuous recording of resource state and events into an embedded store with configurable retention, a "state as of T" API, and a field-level diff API between two moments.
6. Back the code with unit, integration, and E2E test suites, strict linting, static analysis, vulnerability scanning, and CI on every commit.
7. Ship the deployment artefacts: Dockerfiles, Docker Compose, and Kubernetes manifests with least-privilege RBAC.

## 1.4 Scope of Implementation

**In scope:**

- Read-only visibility of the eleven resource kinds listed above, across all namespaces or filtered to one.
- Pod detail view: containers (including init, sidecar, and ephemeral containers), conditions, volumes, and log tailing with container selection.
- Live updates over SSE, with a 5-second polling fallback.
- Multi-context switching across the clusters defined in the kubeconfig.
- History recording, time-travel rendering of all list pages, and two-point diffing with event correlation.
- Local execution, Docker Compose deployment, and in-cluster deployment via manifests.

**Deliberately out of scope:**

- Any write operation (create/edit/delete/scale/exec). Excluded for safety.
- Authentication and multi-user access control. The tool targets single-operator local use; in-cluster exposure is guarded by a NetworkPolicy and documented warnings.
- Custom Resource Definitions (CRDs) and CPU/memory metrics.

## 1.5 Organization of the Report

Chapter 2 describes the architecture, technology stack, modules, and key algorithms, and Chapter 3 covers the test strategy, test cases, and results. Chapter 4 explains how to run and deploy the system. Chapter 5 collects project-execution evidence (version control, weekly progress, supervisor interaction), and Chapter 6 closes with achievements, limitations, and future work. The appendices hold the user manual, installation guide, and source-code links.

---

# CHAPTER 2: IMPLEMENTATION DETAILS

## 2.1 System Architecture & Design

### High-level architecture

KubeView is two services:

```
┌─────────┐     HTTP/JSON + SSE      ┌──────────────────┐     client-go      ┌────────────────┐
│ Browser │ ───────────────────────▶ │  Frontend         │                   │                │
│         │                          │  Next.js (:5500)  │                   │  Kubernetes    │
│         │ ◀── HTML/JS/CSS ──────── │                   │                   │  API Server(s) │
│         │                          └──────────────────┘                   │                │
│         │     REST + SSE           ┌──────────────────┐  list/get/watch   │                │
│         │ ───────────────────────▶ │  Backend          │ ────────────────▶ │                │
│         │ ◀── JSON / event stream  │  Go (:5501)       │ ◀──────────────── └────────────────┘
└─────────┘                          │        │          │
                                     │        ▼          │
                                     │  ┌────────────┐   │
                                     │  │ history.db │   │  (embedded bbolt store,
                                     │  │  (bbolt)   │   │   72 h ring buffer)
                                     │  └────────────┘   │
                                     └──────────────────┘
```
*Fig. 1. High-level system architecture. `<Replace with rendered diagram in final PDF>`*

The **backend** (`kubeview-backend/`) is a single static Go binary exposing a REST + SSE API on port 5501. It loads the kubeconfig (or the in-cluster service-account token), keeps one Kubernetes clientset per context, reshapes raw Kubernetes objects into a compact JSON form for the frontend, and runs the flight recorder.

The **frontend** (`kubeview-frontend/`) is a Next.js App Router application on port 5500. Each resource page fetches from the backend, subscribes to the SSE watch stream so it knows when to re-fetch, and falls back to polling every 5 seconds.

### Data flow diagram

```
                       (1) GET /api/pods?namespace=ns&context=ctx
  User ──▶ Page ──▶ API client ─────────────────────────────────▶ Router/CORS
                                                                      │
                                                (2) clientset.CoreV1().Pods(ns).List()
                                                                      ▼
                                                              Kubernetes API
                                                                      │
                                    (3) []v1.Pod                      ▼
  User ◀── render ◀── React state ◀── (4) JSON ◀── transformer ◀── handler

  Flight recorder (continuous, independent of requests):
  Kubernetes API ──watch──▶ shared informers ──deltas──▶ store writer ──▶ bbolt (history.db)
  Browser ◀── timeline UI ◀── /api/history/{range,state,diff} ◀── store reader
```
*Fig. 2. Data flow. `<Replace with rendered diagram in final PDF>`*

### Component interaction

A normal request travels browser → Next.js page → typed API client (`src/lib/api.ts`) → Go router (`handlers.go`) → per-context client manager (`clients.go`) → `client-go` wrappers (`kube.go`, `resources.go`, `workloads.go`) → transformers (`transformers.go`) → JSON back to the page.

Live updates take a second path: `GET /api/watch` holds an SSE connection open, backend informers push resource-change notifications into it, and the frontend hook (`src/lib/hooks.ts`) re-fetches whichever resource list was affected.

The history path (Fig. 3) runs continuously and independently of requests. Per-context shared informers feed every add, update, and delete into a single store writer, which persists delta versions into bbolt buckets keyed by resource kind and object UID. The `/api/history/state` reader rebuilds cluster state at time T by scanning versions at or before T; `/api/history/diff` rebuilds both endpoints and computes field-level change summaries (`history_diff.go`). A retention pruner deletes versions older than the configured window.

In the UI, a `TimeTravelProvider` React context holds the selected timestamp. While one is set, every data hook switches from the live endpoints to `/api/history/state?at=T` — which means all the existing pages render historical data without any modification. A `TimelineBar` scrubber controls the timestamp.

## 2.2 Technology Stack

*Table 1. Technology stack*

| Layer | Technology | Version | Role |
| --- | --- | --- | --- |
| Backend language | Go | 1.26 | API server, flight recorder |
| HTTP layer | Go standard library `net/http` (`http.ServeMux`) | stdlib | Routing (Go 1.22+ pattern syntax), no web framework |
| Kubernetes client | `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` | v0.36.1 | Cluster access, shared informers |
| Embedded store | `go.etcd.io/bbolt` | v1.5.0 | Flight-recorder persistence (single-file B+tree) |
| Frontend framework | Next.js (App Router) | 16.1.6 | Pages, routing, build |
| UI library | React | 19.2.3 | Components |
| Language | TypeScript | 5.x | Type-safe frontend |
| Styling | Tailwind CSS | v4 | Utility-first styling |
| Frontend unit tests | Vitest + Testing Library + jsdom | 4.x | Component/hook/API-client tests |
| E2E tests | Playwright | latest | Browser tests against a real kind cluster |
| Backend tests | Go `testing` + `client-go` fakes | stdlib | Unit + integration tests |
| Lint / static analysis | golangci-lint (max strictness), ESLint 9, CodeQL, govulncheck | — | Code quality and security |
| CI | GitHub Actions | — | Test, lint, vuln-scan, E2E on every push/PR |
| Packaging | Docker, Docker Compose, Kubernetes manifests (kustomize) | — | Deployment |

## 2.3 System Modules

### Backend modules (`kubeview-backend/`)

| Module | Responsibility |
| --- | --- |
| `main.go` | Server bootstrap, read/write timeouts, graceful shutdown on SIGINT/SIGTERM |
| `kube.go` | Kubeconfig loading (`KUBECONFIG`, `~/.kube/config`, or in-cluster), clientset construction, thin list/get wrappers |
| `clients.go` | Multi-context client manager: enumerates kubeconfig contexts, lazily builds and caches one clientset per context, resolves the `?context=` query parameter |
| `handlers.go` | Router, CORS middleware, error helpers, all live-resource handlers, SSE `/api/watch` endpoint |
| `resources.go` | ConfigMap, Secret, Ingress list wrappers. Secret values are redacted; only keys and sizes are returned |
| `workloads.go` | StatefulSet, DaemonSet list wrappers |
| `transformers.go` | Converts raw Kubernetes objects into compact JSON response structs (status derivation, age formatting, container summaries) |
| `history_config.go` | History env parsing (`HISTORY_RETENTION_HOURS`, `HISTORY_DIR`) and recorder bootstrap |
| `history_recorder.go` | Per-context shared informers feeding a single store writer goroutine |
| `history_store.go` | bbolt store: delta-version writes, state-at-time scans, retention pruning |
| `history_diff.go` | Field-level change summaries between two recorded states |
| `history_handlers.go` | `/api/history/range`, `/api/history/state`, `/api/history/diff` |

### Frontend modules (`kubeview-frontend/src/`)

| Module | Responsibility |
| --- | --- |
| `app/*/page.tsx` | One route per resource kind: dashboard, namespaces, pods, deployments, services, nodes, events, configmaps, secrets, ingresses, statefulsets, daemonsets, timeline |
| `app/pods/[namespace]/[name]/page.tsx` | Pod detail: containers, conditions, volumes, live log viewer with container picker and tail-length control |
| `app/timeline/page.tsx` | Two-moment diff view: changed objects with field-level summaries plus interleaved events |
| `components/Sidebar.tsx` | Navigation |
| `components/ResourceList.tsx` | Shared list plumbing (search, namespace filter, loading/error states) |
| `components/ContextSwitcher.tsx`, `ClusterProvider.tsx`, `ClusterScope.tsx` | Cluster context selection, propagated to every API call |
| `components/TimeTravelProvider.tsx`, `TimelineBar.tsx` | Time-travel state and the timeline scrubber UI |
| `components/StatusBadge.tsx`, `NamespaceFilter.tsx`, `SearchInput.tsx`, `LoadingSpinner.tsx`, `ErrorMessage.tsx` | Reusable UI pieces |
| `lib/api.ts` | Typed API client for every backend endpoint |
| `lib/hooks.ts` | Data-fetching hooks: polling, SSE subscription, time-travel switching |

### Functional flow (example: user scrubs the timeline)

1. On load, `TimelineBar` calls `/api/history/range` to learn the recorded window.
2. The user drags the scrubber. `TimeTravelProvider` stores the chosen timestamp and the UI enters "past mode", with a visible badge.
3. Every mounted page's data hook notices past mode and re-fetches from `/api/history/state?at=T`.
4. The backend rebuilds each object's latest version at or before T from bbolt and returns the same JSON shape as the live endpoints, so the pages render unchanged.
5. Clearing the scrubber puts every page back on live data and SSE updates.

## 2.4 Key Algorithms / Logic

### (a) Flight-recorder delta versioning

Storing a full cluster snapshot every few seconds would eat disk fast. The recorder instead stores a new version of an object only when it actually changes, keyed so that a point-in-time read is just a range scan:

```
on informer event (add / update / delete) for object O of kind K:
    doc ← transform(O)                      # same JSON shape the live API serves
    if delete: doc.deleted ← true
    key ← (K, O.uid, now_unix_ms)
    if latest stored version of (K, O.uid) is identical to doc: skip   # dedup
    write bucket[K][O.uid][now] = doc

state_at(T):
    for each kind K, each uid U:
        v ← greatest version timestamp ≤ T in bucket[K][U]
        if v exists and not v.deleted: include v in result

prune(retention):
    delete every version older than (now − retention),
    keeping at least the newest version ≤ cutoff per object
    (so state_at just inside the window is still complete)
```

### (b) Two-moment diff with event correlation

```
diff(T1, T2):
    S1 ← state_at(T1); S2 ← state_at(T2)
    for each uid in S1 ∪ S2:
        absent in S1            → ADDED
        absent/deleted in S2    → REMOVED
        else compare fields     → MODIFIED with change list, e.g.
              image: nginx:1.25 → nginx:1.27
              replicas: 2 → 5
              restarts: 0 → 4
              condition Ready: True → False
    events ← all recorded events with T1 < timestamp ≤ T2
    return {changes, events} sorted for display
```

Field comparison is semantic, not textual. `history_diff.go` compares the typed fields that matter operationally — images, replica counts, restart counts, phases, conditions, node assignment — and emits one-line summaries. That choice is what keeps the Timeline page readable.

### (c) Pod status derivation

The pod "status" that `kubectl` shows isn't a single field, so the transformer reproduces the derivation: start from `pod.Status.Phase`, override with the reason of any non-running container state (`CrashLoopBackOff`, `ImagePullBackOff`, `Completed`, and so on), account for init/sidecar containers and deletion timestamps, and compute a ready count like `2/3`. Container statuses are matched to containers by name rather than by index, which is what makes init, sidecar, and ephemeral containers come out right.

### (d) Live updates over SSE

`/api/watch` opens a Server-Sent Events stream. Backend informers publish change notifications into per-subscriber channels; a slow client's channel drops and coalesces rather than blocking the informer. On the other end, the frontend hook re-fetches only the affected resource kind, so changes reach the UI as soon as the watch event arrives rather than on the next poll. The 5-second polling stays around as a fallback.

## 2.5 Screenshots / Code Snippets

*(Screenshots live in `screenshots/`; embed at full width in the final PDF.)*

- Fig. 4. Dashboard overview: `screenshots/01-dashboard.png`
- Fig. 5. Pods list with search and namespace filter: `screenshots/03-pods.png`
- Fig. 6. Pod detail with live logs: `screenshots/04-pod-detail.png`, `screenshots/05-pod-logs.png`
- Fig. 7. Timeline diff view (image change, replica scaling, pod added/removed, condition transitions): `screenshots/08-timeline.png`

A representative code section — the Go 1.22+ router with method-scoped patterns (`handlers.go`):

```go
mux.HandleFunc("GET /api/health", handleHealth)
mux.HandleFunc("GET /api/contexts", handleContexts(manager))
mux.HandleFunc("GET /api/pods", wrap(handlePods))
mux.HandleFunc("GET /api/pods/{namespace}/{name}", wrap(handlePod))
mux.HandleFunc("GET /api/pods/{namespace}/{name}/logs", wrap(handlePodLogs))
mux.HandleFunc("GET /api/watch", wrap(handleWatch))
mux.HandleFunc("GET /api/history/range", wrap(history.handleRange))
mux.HandleFunc("GET /api/history/state", wrap(history.handleState))
mux.HandleFunc("GET /api/history/diff", wrap(history.handleDiff))
```

*Table 2. Backend REST API endpoints*

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/health` | Liveness probe |
| GET | `/api/contexts` | Kubeconfig contexts available for switching |
| GET | `/api/cluster` | Cluster version, platform, node count, current context |
| GET | `/api/namespaces` | List namespaces |
| GET | `/api/pods[?namespace=ns]` | List pods |
| GET | `/api/pods/{ns}/{name}` | Pod detail |
| GET | `/api/pods/{ns}/{name}/logs?container=c&tailLines=n` | Pod logs |
| GET | `/api/deployments` / `services` / `nodes` / `events` | Core resource lists |
| GET | `/api/configmaps` / `secrets` / `ingresses` / `statefulsets` / `daemonsets` | Extended resource lists |
| GET | `/api/watch` | SSE stream of resource-change notifications |
| GET | `/api/history/range` | Flight-recorder bounds (enabled, start, end, retention) |
| GET | `/api/history/state?at=T` | Cluster state as of a past moment |
| GET | `/api/history/diff?from=T1&to=T2` | Changes between two moments plus interleaved events |

All list endpoints accept `?namespace=` and `?context=` query parameters. Secret values are never returned — only key names and sizes.

---

# CHAPTER 3: TESTING, VALIDATION & RESULTS

## 3.1 Test Plan

### Testing strategy

There are four layers, all automated in CI on every push and pull request.

Backend unit tests in Go cover the handlers, transformers, kubeconfig loading, the multi-context manager, and the entire history subsystem, all running against `client-go` fake clientsets and temporary bbolt stores. No real cluster needed, and the suite runs under the race detector. Backend integration tests then exercise the composed router end-to-end over `httptest`.

On the frontend, Vitest and Testing Library cover the API client, the polling/SSE hooks, Sidebar, ResourceList, TimelineBar, the timeline page, and past-mode pod detail, rendered in jsdom against mocked fetch/EventSource.

The end-to-end layer is the expensive one, and deliberately so. CI boots a real kind cluster, applies `kubeview-e2e/fixtures.yaml` (nginx deployments, multi-container pods, a crashing pod, ConfigMaps, Secrets, StatefulSets, DaemonSets, Ingresses), starts the real backend and frontend, and drives a real browser through every page — including the live-update and time-travel scenarios.

On top of the tests: golangci-lint at maximum strictness (driven from 1720 findings to 0), ESLint 9, CodeQL static analysis, and govulncheck. All reported CVEs were patched by dependency and toolchain bumps.

### Tools used

Go `testing` with `client-go` fakes, `httptest`, Vitest, @testing-library/react, jsdom, Playwright, kind, GitHub Actions, golangci-lint, ESLint, CodeQL, govulncheck.

## 3.2 Test Cases

*Table 4. Test suite summary*

| Suite | Tool | Count | Scope |
| --- | --- | --- | --- |
| Backend unit + integration | Go `testing` | 227 test functions | Handlers, transformers, kube client, contexts, SSE, full history subsystem |
| Frontend unit | Vitest + Testing Library | 70 tests | API client, hooks, components, timeline, past mode |
| End-to-end | Playwright + kind | 45 tests across 16 spec files | Every page against a real cluster and browser |

*Table 5. Representative test cases*

| Test Case ID | Description | Input | Expected Output | Status |
| --- | --- | --- | --- | --- |
| TC-01 | Health endpoint | `GET /api/health` | `200`, `{"status":"ok"}` | Pass |
| TC-02 | List pods, all namespaces | `GET /api/pods` against fake clientset with 3 pods | `200`, 3 pod summaries with derived status and ready counts | Pass |
| TC-03 | Namespace filter | `GET /api/pods?namespace=team-a` | Only `team-a` pods returned | Pass |
| TC-04 | Pod detail not found | `GET /api/pods/default/missing` | `404` with JSON error body | Pass |
| TC-05 | Pod logs, multi-container pod | `GET .../logs?container=sidecar&tailLines=50` | Logs of the named container only, capped tail | Pass |
| TC-06 | Log tail cap | `tailLines=999999` | Tail clamped to server maximum, no error | Pass |
| TC-07 | Pod status derivation | Pod with container in `CrashLoopBackOff` | Status string `CrashLoopBackOff`, not `Running` | Pass |
| TC-08 | Container matching by name | Pod with init + app containers, statuses out of order | Each container paired with its own status | Pass |
| TC-09 | Secret redaction | `GET /api/secrets` | Key names and sizes only; no secret values in response | Pass |
| TC-10 | CORS allow-list | Request with `Origin` not in `CORS_ORIGIN` | No CORS headers on response | Pass |
| TC-11 | Context switching | `GET /api/pods?context=cluster-b` | Pods served from cluster-b's clientset; unknown context returns an error | Pass |
| TC-12 | SSE watch stream | Client subscribes to `/api/watch`, pod created in cluster | Change notification event received on the stream | Pass |
| TC-13 | History: state-at | Record v1 at t1, v2 at t3; query `state?at=t2` | v1 returned (latest version ≤ t2) | Pass |
| TC-14 | History: deleted object | Object deleted before `at` | Object absent from state | Pass |
| TC-15 | History: diff | Deployment image changed between T1 and T2 | MODIFIED entry `image: old → new` plus events in (T1, T2] | Pass |
| TC-16 | History: retention pruning | Versions older than retention window | Pruned, while state just inside the window stays complete | Pass |
| TC-17 | History: invalid params | `state?at=garbage` | `400` with JSON error | Pass |
| TC-18 | E2E: pods page | Browser opens `/pods` against kind cluster with fixtures | Fixture pods visible, searchable, filterable | Pass |
| TC-19 | E2E: crashing pod | Fixture pod in CrashLoopBackOff | Red status badge shown on pods page | Pass |
| TC-20 | E2E: log streaming | Open pod detail, select container | Live log lines render and update | Pass |
| TC-21 | E2E: live updates | Scale deployment while page open | Pod list updates without manual refresh | Pass |
| TC-22 | E2E: time travel | Delete a pod, scrub timeline to before deletion | Deleted pod visible in past mode; timeline diff lists the deletion and its events | Pass |
| TC-23 | Graceful shutdown | SIGTERM to backend | In-flight requests complete, process exits 0 | Pass |
| TC-24 | Frontend: past-mode pod detail | Time-travel timestamp set, open pod detail | Data fetched from history endpoint; past-mode badge shown; log controls disabled | Pass |

*(The repository holds the full inventory of 227 backend, 70 frontend, and 45 E2E automated cases; this table is a cross-section. Attach full CI run output in the final PDF.)*

## 3.3 Results & Analysis

### Observations

All suites pass on the submission baseline (tag `v1.0.0-submission`) in GitHub Actions CI: backend tests with `-race`, frontend unit tests, the lint jobs, CodeQL, govulncheck, and the full Playwright E2E job against a live kind cluster. See `docs/img/ci-runs.png` for the passing workflow runs.

The real-cluster CI pipeline paid for itself. The E2E layer repeatedly caught integration regressions the unit layers couldn't see — JSON field-name mismatches between backend struct tags and frontend types, for one.

Two smaller notes. golangci-lint at maximum strictness reported 1,720 findings when first enabled; we fixed all of them, and the CI gate keeps the count at zero. And govulncheck flagged CVEs in `golang.org/x/net` and the Go standard library during the project, which dependency and toolchain upgrades fixed.

### Performance / accuracy properties

No formal performance benchmark was defined or executed for this project, so the points below are design properties and informal observations from development use, not measured results.

- **API latency.** List endpoints add little on top of the Kubernetes API round-trip, which dominates response time against a local kind cluster. History `state-at` reconstruction reads each object's versions as a single range scan in bbolt, so lookup cost grows with the number of objects, not with the length of the retention window.
- **Storage.** Delta versioning with dedup writes a record only on actual change, so an idle cluster consumes almost no history storage. The retention pruner bounds the growth of `history.db`.
- **Update latency.** SSE delivers a change notification as soon as the watch event arrives, where polling alone had a worst case of 5 seconds.
- **Memory.** The backend is a single static binary whose resident footprint is set by the informer caches and bbolt; it is expected to stay modest on small clusters.
- **Accuracy.** Time-travel state reproduces exactly what the live API served at recording time, because the recorder persists the same transformed JSON documents the live endpoints emit.

---

# CHAPTER 4: EXECUTION / DEPLOYMENT DETAILS

## 4.1 Execution Environment

- **Development machines:** macOS / Linux; Go 1.26+, Node.js 24 (the version CI uses), Docker, kubectl.
- **Clusters used:** kind and Docker Desktop Kubernetes locally; kind in CI.
- **Prerequisite check:** if `kubectl get nodes` succeeds, KubeView can connect.

## 4.2 Deployment Steps

### Local (developer mode)

```bash
# Terminal 1 — backend (http://localhost:5501)
cd kubeview-backend && go run .

# Terminal 2 — frontend (http://localhost:5500)
cd kubeview-frontend && npm install && npm run dev
```

### Docker Compose

```bash
docker compose up --build
```

The backend runs on the host network with `~/.kube/config` mounted read-only; the dashboard is at `http://localhost:5500`.

### In-cluster (Kubernetes manifests)

```bash
docker build -t kubeview-backend:latest kubeview-backend/
docker build -t kubeview-frontend:latest kubeview-frontend/
kind load docker-image kubeview-backend:latest kubeview-frontend:latest

kubectl apply -k deploy/kubernetes/
kubectl -n kubeview port-forward svc/kubeview-frontend 5500:5500
```

The manifests create a dedicated namespace, a ServiceAccount, and a ClusterRole/ClusterRoleBinding granting read-only access (`get`, `list`, `watch`) with no write verbs. A NetworkPolicy restricts backend ingress to the frontend pod. When no kubeconfig is present, the backend authenticates with the pod's service-account token.

*Table 3. Configuration environment variables*

| Variable | Service | Default | Purpose |
| --- | --- | --- | --- |
| `PORT` | backend | `5501` | API listen port |
| `CORS_ORIGIN` | backend | `http://localhost:5500` | Comma-separated allowed browser origins |
| `KUBECONFIG` | backend | `~/.kube/config` | Kubeconfig path(s), colon-separated, merged like kubectl; in-cluster token used when absent |
| `HISTORY_RETENTION_HOURS` | backend | `72` | Flight-recorder retention; `0` disables recording |
| `HISTORY_DIR` | backend | user cache dir | Location of `history.db`; point at a persistent volume in containers |
| `NEXT_PUBLIC_API_BASE` | frontend | `http://localhost:5501/api` | Backend URL, baked in at build time |

## 4.3 Demo Screenshots

See `screenshots/` (Figs. 4–7): dashboard, namespaces, pods, pod detail, pod logs, deployments, services, nodes, events, and the timeline diff view (`screenshots/08-timeline.png`).

## 4.4 Demo Video Link

`<DEMO VIDEO LINK>`

---

# CHAPTER 5: PROJECT EXECUTION EVIDENCE

## 5.1 Version Control Evidence

- **GitHub repository:** https://github.com/varundeepsaini/kubeview (public)
- **Development model:** feature branches merged via pull requests (#12–#27), with CI gates (tests, lint, CodeQL, govulncheck, E2E) required on every PR.
- **Commit history screenshot:** `docs/img/commit-history.png` (Fig. 8); CI workflow runs: `docs/img/ci-runs.png`

## 5.2 Weekly Progress Summary

*Table 6. Weekly progress summary (derived from commit/PR history; adjust weeks to the official plan)*

| Week | Task Planned | Task Completed | Supervisor Remark |
| --- | --- | --- | --- |
| 1 | Repository setup, project scaffolding | Repo initialised; Next.js frontend and initial API scaffolding (`e2541e3`) | |
| 2–3 | Backend implementation in Go | Go backend with client-go, all core endpoints; full Go test suite; legacy Node.js prototype removed (`80897d0`–`88f695c`) | |
| 4–5 | Hardening and engineering quality | Error handling hardened, log tail capped; strict CI, golangci-lint (1720→0 findings), CodeQL, govulncheck, CVE patching (`67ef1ae`–`e192afa`) | |
| 6 | Documentation + events | Top-level README (#12); Events page (#13) | |
| 7–8 | Container correctness + configurability | Multi-container/init/sidecar/ephemeral container fixes (#17, #20); env-based configuration (#21) | |
| 9 | Deployment artefacts | Dockerfiles, Compose, K8s manifests with read-only RBAC + NetworkPolicy (#22, #23) | |
| 10 | E2E infrastructure | Playwright E2E suite against real kind cluster in CI (#24) | |
| 11 | Multi-cluster support | Multi-context/multi-cluster switching (#25) | |
| 12 | Live updates + resource coverage | SSE streaming of updates and pod logs (#27); Vitest unit tooling; ConfigMaps/Secrets/Ingresses/StatefulSets/DaemonSets views (#26) | |
| 13 | Flight recorder | Cluster flight recorder with time-travel timeline (#16); final report | |

## 5.3 Supervisor Interaction Summary

*(To be filled by the supervisor.)*

| Review Date | Mode | Key Feedback Received | Action Taken |
| --- | --- | --- | --- |
| | | | |
| | | | |
| | | | |

---

# CHAPTER 6: CONCLUSION & FUTURE WORK

## 6.1 Summary of Implementation

KubeView is a working Kubernetes dashboard built from a Go backend with few dependencies (standard-library HTTP, client-go, an embedded bbolt store) and a Next.js frontend. It covers eleven resource kinds, live log tailing, SSE live updates, and multi-cluster switching. The flight recorder — the piece we consider the project's main contribution — adds time-travel rendering and two-moment diffing of cluster state with no configuration at all.

## 6.2 Achievements

The one we're most pleased with is history in a small tool: time travel in a dashboard that installs nothing in the cluster and runs as two small containers. Getting the same answer any other way means standing up a full observability stack.

The test coverage held up too. 227 backend, 70 frontend, and 45 E2E automated tests, race-detector runs, golangci-lint held at zero findings, CodeQL and govulncheck gates, and every feature merged through a PR with CI. The E2E pipeline boots an actual kind cluster per run, so the tests exercise the real system rather than mocks.

Security-wise, the tool is read-only by construction, ships least-privilege RBAC, redacts secret values, allow-lists CORS origins, includes a NetworkPolicy, and documents its exposure warnings.

## 6.3 Limitations

- **No authentication.** The backend API is unauthenticated. Fine for local single-operator use, but in-cluster exposure relies on the NetworkPolicy and must not be published via Ingress without an auth proxy.
- **History scale.** The flight recorder targets small-to-medium clusters. Very large clusters would need snapshots or compaction beyond per-object delta chains.
- **Coverage gaps.** No CRD support, no CPU/memory metrics, no exec-into-container (that last one is also a deliberate safety exclusion).
- **Single-writer store.** bbolt binds history to one backend replica; scaling the backend horizontally would need an external store.
- **No formal performance evaluation.** Latency, memory, and large-cluster behaviour are argued from the design and observed informally during development, not measured under a defined load profile.

## 6.4 Future Enhancements

1. Pluggable authentication (OIDC or reverse-proxy header auth) so multi-user, in-cluster exposure becomes safe.
2. Metrics integration (metrics-server or Prometheus) for CPU/memory charts on the existing pages and on the timeline.
3. CRD discovery, so operators can watch custom resources through the same list, detail, and history machinery.
4. History export/import and an "incident bundle" download for sharing a time window with teammates.
5. Timeline annotations (deploy markers, manual notes) correlated with the recorded diffs.

---

# REFERENCES

1. Kubernetes Authors, "Kubernetes Documentation," kubernetes.io. [Online]. Available: https://kubernetes.io/docs/. [Accessed: Aug. 2026].
2. Kubernetes Authors, "client-go: Go client for Kubernetes," GitHub. [Online]. Available: https://github.com/kubernetes/client-go. [Accessed: Aug. 2026].
3. Kubernetes Authors, "API Concepts: Efficient detection of changes (watch)," kubernetes.io. [Online]. Available: https://kubernetes.io/docs/reference/using-api/api-concepts/. [Accessed: Aug. 2026].
4. The Go Authors, "net/http, Go standard library," pkg.go.dev. [Online]. Available: https://pkg.go.dev/net/http. [Accessed: Aug. 2026].
5. etcd-io, "bbolt: an embedded key/value database for Go," GitHub. [Online]. Available: https://github.com/etcd-io/bbolt. [Accessed: Aug. 2026].
6. Vercel, "Next.js Documentation," nextjs.org. [Online]. Available: https://nextjs.org/docs. [Accessed: Aug. 2026].
7. Meta Open Source, "React Documentation," react.dev. [Online]. Available: https://react.dev/. [Accessed: Aug. 2026].
8. Tailwind Labs, "Tailwind CSS Documentation," tailwindcss.com. [Online]. Available: https://tailwindcss.com/docs. [Accessed: Aug. 2026].
9. Microsoft, "Playwright Documentation," playwright.dev. [Online]. Available: https://playwright.dev/docs/intro. [Accessed: Aug. 2026].
10. Kubernetes SIGs, "kind: Kubernetes in Docker," kind.sigs.k8s.io. [Online]. Available: https://kind.sigs.k8s.io/. [Accessed: Aug. 2026].
11. golangci, "golangci-lint Documentation," golangci-lint.run. [Online]. Available: https://golangci-lint.run/. [Accessed: Aug. 2026].
12. WHATWG, "Server-Sent Events, HTML Living Standard," html.spec.whatwg.org. [Online]. Available: https://html.spec.whatwg.org/multipage/server-sent-events.html. [Accessed: Aug. 2026].

---

# APPENDIX

## A. User Manual

See the repository README (`README.md`): features, page-by-page usage, configuration reference, and troubleshooting. `<In the final PDF, paste the user-manual sections here.>`

## B. Installation Guide

See `README.md` sections "Prerequisites", "Getting started", and "Deploying" for local, Docker Compose, and in-cluster instructions. `<In the final PDF, paste the installation sections here.>`

## C. Source Code Link (GitHub)

https://github.com/varundeepsaini/kubeview

## D. Demo Video Link

`<DEMO VIDEO LINK>`

---

*Formatting note for PDF conversion: Times New Roman; 12 pt body, 14 pt headings; 1.5 line spacing; 1-inch margins on all sides; page numbers bottom-centre; export as PDF.*
