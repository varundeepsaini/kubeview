// Package main implements the KubeView backend HTTP API: it exposes read-only
// Kubernetes cluster data (pods, deployments, services, nodes, events, logs)
// as JSON for the KubeView frontend.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// frontendOrigin is the dev-server default, used when CORS_ORIGIN is unset.
const frontendOrigin = "http://localhost:5500"

// Query/path parameter names and HTTP-related numeric bounds.
const (
	paramNamespace = "namespace"
	paramName      = "name"
	paramContainer = "container"
	paramContext   = "context"

	defaultTailLines = 100
	logTailBase      = 10
	logTailBitSize   = 64
	minTailLines     = 1
	emptyCount       = 0

	minClientErrorCode = 400
	maxServerErrorCode = 599
)

// maxTailLines caps the number of log lines a single request may pull. Without
// a bound a client can ask for an arbitrarily large tail, forcing the server to
// buffer the entire stream in memory (io.ReadAll in GetPodLogs) — a cheap
// memory-exhaustion DoS. 5000 lines is well beyond any practical UI need.
const maxTailLines = 5000

// Response shapes for pod and deployment sub-resources. JSON tags must match
// what the frontend expects in kubeview-frontend/src/lib/api.ts.

type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
	// Kind distinguishes regular containers from init containers, native
	// sidecars (init containers with restartPolicy Always), and ephemeral
	// debug containers: "container" | "init" | "sidecar" | "ephemeral".
	Kind         string   `json:"kind"`
	Ports        []string `json:"ports"`
	RestartCount int32    `json:"restartCount"`
	Ready        bool     `json:"ready"`
}

type PodCondition struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	LastTransition string `json:"lastTransition,omitempty"`
}

type Volume struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type DeploymentCondition struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	LastTransition string `json:"lastTransition,omitempty"`
}

// parseCORSOrigins splits the CORS_ORIGIN environment value (a
// comma-separated origin list) into individual origins, falling back to the
// dev frontend when the value is empty.
func parseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, zeroCount, len(parts))

	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != emptyString {
			origins = append(origins, origin)
		}
	}

	if len(origins) == zeroCount {
		return []string{frontendOrigin}
	}

	return origins
}

// withCORS allows the API to be called cross-origin. Narrow on purpose: a
// request Origin on the allowed list (CORS_ORIGIN env, dev frontend by
// default) is echoed back; any other Origin — including none — gets no
// Access-Control-Allow-Origin header at all, so browsers block it. Exact
// matching means a configured "*" never equals a real Origin and therefore
// fails closed instead of becoming a wildcard. Vary: Origin keeps shared
// caches from serving one origin's ACAO to another.
func withCORS(next http.Handler, allowed []string) http.Handler {
	handler := func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Add("Vary", "Origin")

		requestOrigin := req.Header.Get("Origin")
		if requestOrigin != emptyString &&
			slices.Contains(allowed, requestOrigin) {
			writer.Header().Set(
				"Access-Control-Allow-Origin", requestOrigin,
			)
		}

		writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if req.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(writer, req)
	}

	return http.HandlerFunc(handler)
}

func newRouter(manager *ClientManager) *http.ServeMux {
	const (
		podDetailRoute = "GET /api/pods/{namespace}/{name}"
		podLogsRoute   = "GET /api/pods/{namespace}/{name}/logs"
	)

	// wrap binds a resource handler to the manager so per-request client
	// resolution (from ?context=) happens in one place.
	wrap := func(handler contextHandler) http.HandlerFunc {
		return withClient(manager, handler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/contexts", handleContexts(manager))
	mux.HandleFunc("GET /api/cluster", wrap(handleCluster))
	mux.HandleFunc("GET /api/namespaces", wrap(handleNamespaces))
	mux.HandleFunc("GET /api/pods", wrap(handlePods))
	mux.HandleFunc(podDetailRoute, wrap(handlePod))
	mux.HandleFunc(podLogsRoute, wrap(handlePodLogs))
	mux.HandleFunc("GET /api/deployments", wrap(handleDeployments))
	mux.HandleFunc("GET /api/services", wrap(handleServices))
	mux.HandleFunc("GET /api/nodes", wrap(handleNodes))
	mux.HandleFunc("GET /api/events", wrap(handleEvents))

	return mux
}

// contextHandler is a resource handler that has already had its per-request
// *Client resolved from the ?context= query parameter by withClient.
type contextHandler func(*Client, http.ResponseWriter, *http.Request)

// withClient resolves the client for the request's ?context= value (falling
// back to the default context when absent) and invokes handler with it. An
// unknown context name is a 400; any other resolution failure surfaces through
// writeError. Keeping this in one place spares every resource handler from
// repeating the resolve-and-check dance.
func withClient(
	manager *ClientManager,
	handler contextHandler,
) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		client, err := manager.ClientFor(req.URL.Query().Get(paramContext))
		if err != nil {
			if errors.Is(err, errUnknownContext) {
				writeJSONError(
					writer, http.StatusBadRequest, "Unknown context",
				)

				return
			}

			writeError(writer, err)

			return
		}

		handler(client, writer, req)
	}
}

// handleContexts lists the kubeconfig contexts available for switching. It
// reads from the loaded kubeconfig, not a live cluster, so it succeeds even
// when the active context is unreachable — letting the UI offer a way back.
func handleContexts(manager *ClientManager) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, manager.Contexts())
	}
}

func handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleCluster(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	info, err := client.GetClusterInfo(req.Context())
	if err != nil {
		writeError(writer, err)

		return
	}

	writeJSON(writer, http.StatusOK, info)
}

func handleNamespaces(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	items, err := client.ListNamespaces(req.Context())
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]Namespace, emptyCount, len(items))
	for _, ns := range items {
		out = append(out, transformNamespace(ns))
	}

	writeJSON(writer, http.StatusOK, out)
}

func handlePods(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	namespace := req.URL.Query().Get(paramNamespace)

	items, err := client.ListPods(req.Context(), namespace)
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]Pod, emptyCount, len(items))
	for i := range items {
		out = append(out, transformPod(&items[i]))
	}

	writeJSON(writer, http.StatusOK, out)
}

func handlePod(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	namespace := req.PathValue(paramNamespace)
	name := req.PathValue(paramName)

	pod, err := client.GetPod(req.Context(), namespace, name)
	if err != nil {
		if isNotFound(err) {
			writeJSONError(writer, http.StatusNotFound, "Pod not found")

			return
		}

		writeError(writer, err)

		return
	}

	writeJSON(writer, http.StatusOK, transformPod(pod))
}

func handlePodLogs(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	query := req.URL.Query()
	tailLines := parseTailLines(query.Get("tailLines"))

	logs, err := client.GetPodLogs(
		req.Context(),
		req.PathValue(paramNamespace),
		req.PathValue(paramName),
		query.Get(paramContainer),
		tailLines,
	)
	if err != nil {
		if isNotFound(err) {
			writeJSONError(writer, http.StatusNotFound, "Pod not found")

			return
		}

		writeError(writer, err)

		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"logs": logs})
}

// parseTailLines resolves the requested log tail length, falling back to a
// default and clamping to maxTailLines. Invalid or non-positive input yields
// the default.
func parseTailLines(raw string) int64 {
	if raw == "" {
		return defaultTailLines
	}

	n, err := strconv.ParseInt(raw, logTailBase, logTailBitSize)
	if err != nil || n < minTailLines {
		return defaultTailLines
	}

	return min(n, maxTailLines)
}

func handleDeployments(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	namespace := req.URL.Query().Get(paramNamespace)

	items, err := client.ListDeployments(req.Context(), namespace)
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]Deployment, emptyCount, len(items))
	for _, dep := range items {
		out = append(out, transformDeployment(dep))
	}

	writeJSON(writer, http.StatusOK, out)
}

func handleServices(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	namespace := req.URL.Query().Get(paramNamespace)

	items, err := client.ListServices(req.Context(), namespace)
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]Service, emptyCount, len(items))
	for _, svc := range items {
		out = append(out, transformService(svc))
	}

	writeJSON(writer, http.StatusOK, out)
}

func handleNodes(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	items, err := client.ListNodes(req.Context())
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]NodeInfo, emptyCount, len(items))
	for _, node := range items {
		out = append(out, transformNode(node))
	}

	writeJSON(writer, http.StatusOK, out)
}

func handleEvents(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	namespace := req.URL.Query().Get(paramNamespace)

	items, err := client.ListEvents(req.Context(), namespace)
	if err != nil {
		writeError(writer, err)

		return
	}

	out := make([]KubeEvent, emptyCount, len(items))
	for _, event := range items {
		out = append(out, transformEvent(event))
	}

	writeJSON(writer, http.StatusOK, out)
}

// --- response helpers ---

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	err := json.NewEncoder(writer).Encode(body)
	if err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeJSONError(writer http.ResponseWriter, status int, msg string) {
	writeJSON(writer, status, map[string]any{"error": msg, "status": status})
}

// writeError maps Kubernetes API errors to appropriate HTTP statuses and
// logs unexpected ones. Errors carrying a *apierrors.StatusError surface
// their original status code (e.g. 403, 404); everything else falls back
// to 500.
func writeError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		code := int(statusErr.ErrStatus.Code)
		if code >= minClientErrorCode && code <= maxServerErrorCode {
			status = code
		}
	}

	log.Printf("API Error: %v", err)
	// Full detail stays in the server log above; the client gets only the
	// generic status text. Raw Kubernetes errors can carry internal cluster
	// detail (RBAC subjects, service-account names, API-server URLs) — even on
	// 4xx like 403 Forbidden — that the UI doesn't need and shouldn't expose.
	msg := "Internal server error"

	if status < http.StatusInternalServerError {
		if text := http.StatusText(status); text != "" {
			msg = text
		}
	}

	writeJSONError(writer, status, msg)
}

func isNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
