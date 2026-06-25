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

// maxTailLines caps the number of log lines a single request may pull. Without
// a bound a client can ask for an arbitrarily large tail, forcing the server to
// buffer the entire stream in memory (io.ReadAll in GetPodLogs) — a cheap
// memory-exhaustion DoS. 5000 lines is well beyond any practical UI need.
const maxTailLines = 5000

// withCORS allows the API to be called cross-origin. Narrow on purpose: only
// the whitelisted dev frontend, never "*".
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newRouter(c *Client) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/cluster", handleCluster(c))
	mux.HandleFunc("GET /api/namespaces", handleNamespaces(c))
	mux.HandleFunc("GET /api/pods", handlePods(c))
	mux.HandleFunc("GET /api/pods/{namespace}/{name}", handlePod(c))
	mux.HandleFunc("GET /api/pods/{namespace}/{name}/logs", handlePodLogs(c))
	mux.HandleFunc("GET /api/deployments", handleDeployments(c))
	mux.HandleFunc("GET /api/services", handleServices(c))
	mux.HandleFunc("GET /api/nodes", handleNodes(c))
	mux.HandleFunc("GET /api/events", handleEvents(c))
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleCluster(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := c.GetClusterInfo(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

func handleNamespaces(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListNamespaces(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]Namespace, 0, len(items))
		for _, ns := range items {
			out = append(out, transformNamespace(ns))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handlePods(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListPods(r.Context(), r.URL.Query().Get("namespace"))
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]Pod, 0, len(items))
		for i := range items {
			out = append(out, transformPod(&items[i]))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handlePod(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pod, err := c.GetPod(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
		if err != nil {
			if isNotFound(err) {
				writeJSONError(w, http.StatusNotFound, "Pod not found")
				return
			}
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, transformPod(pod))
	}
}

func handlePodLogs(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		tailLines := int64(100)
		if raw := q.Get("tailLines"); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
				tailLines = n
				if tailLines > maxTailLines {
					tailLines = maxTailLines
				}
			}
		}
		logs, err := c.GetPodLogs(
			r.Context(),
			r.PathValue("namespace"),
			r.PathValue("name"),
			q.Get("container"),
			tailLines,
		)
		if err != nil {
			if isNotFound(err) {
				writeJSONError(w, http.StatusNotFound, "Pod not found")
				return
			}
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
	}
}

func handleDeployments(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListDeployments(r.Context(), r.URL.Query().Get("namespace"))
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]Deployment, 0, len(items))
		for _, d := range items {
			out = append(out, transformDeployment(d))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleServices(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListServices(r.Context(), r.URL.Query().Get("namespace"))
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]Service, 0, len(items))
		for _, s := range items {
			out = append(out, transformService(s))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleNodes(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListNodes(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]NodeInfo, 0, len(items))
		for _, n := range items {
			out = append(out, transformNode(n))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleEvents(c *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := c.ListEvents(r.Context(), r.URL.Query().Get("namespace"))
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]KubeEvent, 0, len(items))
		for _, e := range items {
			out = append(out, transformEvent(e))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// --- response helpers ---

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}

// writeError maps Kubernetes API errors to appropriate HTTP statuses and
// logs unexpected ones. Errors carrying a *apierrors.StatusError surface
// their original status code (e.g. 403, 404); everything else falls back
// to 500.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		if code := int(statusErr.ErrStatus.Code); code >= 400 && code <= 599 {
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
	writeJSONError(w, status, msg)
}

func isNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
