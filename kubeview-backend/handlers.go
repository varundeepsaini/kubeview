// Package main implements the KubeView backend HTTP API: it exposes read-only
// Kubernetes cluster data (pods, deployments, services, nodes, events, logs)
// as JSON for the KubeView frontend.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const frontendOrigin = "http://localhost:5500"

// Query/path parameter names and HTTP-related numeric bounds.
const (
	paramNamespace = "namespace"
	paramName      = "name"
	paramContainer = "container"

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

// withCORS allows the API to be called cross-origin. Narrow on purpose: only
// the whitelisted dev frontend, never "*".
func withCORS(next http.Handler) http.Handler {
	handler := func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", frontendOrigin)
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

func newRouter(client *Client) *http.ServeMux {
	const podLogsRoute = "GET /api/pods/{namespace}/{name}/logs"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/cluster", handleCluster(client))
	mux.HandleFunc("GET /api/namespaces", handleNamespaces(client))
	mux.HandleFunc("GET /api/pods", handlePods(client))
	mux.HandleFunc("GET /api/pods/{namespace}/{name}", handlePod(client))
	mux.HandleFunc(podLogsRoute, handlePodLogs(client))
	mux.HandleFunc("GET /api/deployments", handleDeployments(client))
	mux.HandleFunc("GET /api/services", handleServices(client))
	mux.HandleFunc("GET /api/nodes", handleNodes(client))
	mux.HandleFunc("GET /api/events", handleEvents(client))

	return mux
}

func handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleCluster(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		info, err := client.GetClusterInfo(req.Context())
		if err != nil {
			writeError(writer, err)

			return
		}

		writeJSON(writer, http.StatusOK, info)
	}
}

func handleNamespaces(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handlePods(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handlePod(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handlePodLogs(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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

func handleDeployments(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handleServices(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handleNodes(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
}

func handleEvents(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
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
