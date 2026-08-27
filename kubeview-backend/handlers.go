// Package main implements the KubeView backend HTTP API: it exposes read-only
// Kubernetes cluster data (pods, deployments, services, nodes, events, logs)
// as JSON for the KubeView frontend.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
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

	// scanDoneBuffer lets the scan goroutine report its final error without
	// blocking, even after the write loop has stopped receiving.
	scanDoneBuffer = 1
)

// headerContentType names the header set on every JSON and SSE response.
const headerContentType = "Content-Type"

// Log lines (single-line JSON, stack traces) can exceed bufio.Scanner's
// default 64KB token limit, which would end the scan with ErrTooLong and
// silently kill the stream mid-log.
const (
	logScanInitialBuf = 64 * 1024
	logScanMaxBuf     = 1024 * 1024
)

// sseHeartbeatInterval paces keepalive comments on SSE streams. Without them
// a quiet stream over a half-open connection (NAT idle expiry, VPN drop)
// never errors: the browser sees readyState OPEN forever and the server holds
// the handler and its upstream watches indefinitely. Periodic writes keep
// idle timeouts from killing the path and surface dead peers as write errors.
const sseHeartbeatInterval = 30 * time.Second

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
		writer.Header().Set(
			"Access-Control-Allow-Headers", headerContentType,
		)

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
	mux.HandleFunc("GET /api/watch", wrap(
		func(client *Client, writer http.ResponseWriter, req *http.Request) {
			handleWatch(client)(writer, req)
		},
	))

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

	if query.Get("follow") == "true" {
		handlePodLogStream(client, writer, req, tailLines)

		return
	}

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

func handlePodLogStream(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
	tailLines int64,
) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeJSONError(
			writer, http.StatusInternalServerError, "Streaming unavailable",
		)

		return
	}

	stream, err := client.StreamPodLogs(
		req.Context(),
		req.PathValue(paramNamespace),
		req.PathValue(paramName),
		req.URL.Query().Get(paramContainer),
		tailLines,
	)
	if err != nil {
		writeError(writer, err)

		return
	}

	defer closeLogStream(stream)

	setSSEHeaders(writer)

	// Commit the response now so EventSource opens even if the container
	// stays quiet; headers are only sent on the first write or flush.
	err = writeSSEHeartbeat(writer)
	if err != nil {
		return
	}

	flusher.Flush()

	lines, scanDone := scanLogLines(req.Context(), stream)
	streamLogEvents(req.Context(), writer, flusher, lines, scanDone)
}

// scanLogLines scans the log stream in a goroutine so the caller's write
// loop can also tick heartbeats. Closing the stream unblocks the scanner and
// the canceled context releases a goroutine blocked on send.
func scanLogLines(
	ctx context.Context,
	stream io.Reader,
) (<-chan string, <-chan error) {
	lines := make(chan string)
	scanDone := make(chan error, scanDoneBuffer)

	go func() {
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, logScanInitialBuf), logScanMaxBuf)

		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}

		scanDone <- scanner.Err()
	}()

	return lines, scanDone
}

// streamLogEvents forwards scanned log lines as SSE events, ticking
// heartbeats, until the client disconnects or the scan finishes.
func streamLogEvents(
	ctx context.Context,
	writer http.ResponseWriter,
	flusher http.Flusher,
	lines <-chan string,
	scanDone <-chan error,
) {
	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		if !streamLogTick(ctx, writer, flusher, lines, scanDone, ticker.C) {
			return
		}
	}
}

// streamLogTick handles one wakeup of the log write loop; it reports whether
// the loop should continue.
func streamLogTick(
	ctx context.Context,
	writer http.ResponseWriter,
	flusher http.Flusher,
	lines <-chan string,
	scanDone <-chan error,
	heartbeat <-chan time.Time,
) bool {
	select {
	case <-ctx.Done():
		return false
	case <-heartbeat:
		err := writeSSEHeartbeat(writer)
		if err != nil {
			return false
		}

		flusher.Flush()

		return true
	case line := <-lines:
		err := writeSSE(writer, "log", line)
		if err != nil {
			return false
		}

		flusher.Flush()

		return true
	case scanErr := <-scanDone:
		writeLogEnd(writer, flusher, scanErr)

		return false
	}
}

// writeLogEnd tells the client the stream is over so it closes the
// EventSource instead of auto-reconnecting and replaying the tail forever.
// Error detail stays in the server log, matching writeError.
func writeLogEnd(
	writer http.ResponseWriter,
	flusher http.Flusher,
	scanErr error,
) {
	reason := "eof"

	if scanErr != nil {
		log.Printf("read log stream: %v", scanErr)

		reason = "error"
	}

	err := writeSSE(writer, "end", map[string]string{"reason": reason})
	if err != nil {
		return
	}

	flusher.Flush()
}

type watchMessage struct {
	Object   any    `json:"object,omitempty"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Key      string `json:"key"`
}

type resourceEvent struct {
	event    watch.Event
	resource string
}

func handleWatch(client *Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		serveWatch(client, writer, req)
	}
}

func serveWatch(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeJSONError(
			writer, http.StatusInternalServerError, "Streaming unavailable",
		)

		return
	}

	resources := parseWatchResources(req.URL.Query().Get("resources"))
	if len(resources) == emptyCount || resources[emptyCount] == emptyString {
		writeJSONError(writer, http.StatusBadRequest, "resources is required")

		return
	}

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	events := make(chan resourceEvent)

	watchers, err := openWatches(
		ctx, client, req.URL.Query().Get(paramNamespace), resources, events,
	)
	if err != nil {
		writeWatchError(writer, err)

		return
	}

	defer stopWatchers(watchers)

	setSSEHeaders(writer)

	err = writeSSE(writer, "ready", map[string]any{"resources": resources})
	if err != nil {
		return
	}

	flusher.Flush()
	streamWatchEvents(ctx, writer, flusher, events)
}

// parseWatchResources splits the ?resources= list and dedupes it so a single
// request cannot open an unbounded number of upstream watches (only the
// handful of supported kinds survive).
func parseWatchResources(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	resources := make([]string, emptyCount, len(parts))

	for _, resource := range parts {
		resource = strings.TrimSpace(resource)
		if seen[resource] {
			continue
		}

		seen[resource] = true

		resources = append(resources, resource)
	}

	return resources
}

// openWatches opens one upstream watch per resource and starts a forwarder
// goroutine for each. On failure the already-opened watches are left to die
// with ctx, matching the caller's cancel-on-return cleanup.
func openWatches(
	ctx context.Context,
	client *Client,
	namespace string,
	resources []string,
	events chan<- resourceEvent,
) ([]watch.Interface, error) {
	watchers := make([]watch.Interface, emptyCount, len(resources))

	for _, resource := range resources {
		stream, err := client.WatchResource(ctx, resource, namespace)
		if err != nil {
			return nil, err
		}

		watchers = append(watchers, stream)

		go forwardWatch(ctx, resource, stream, events)
	}

	return watchers, nil
}

func stopWatchers(watchers []watch.Interface) {
	for _, stream := range watchers {
		stream.Stop()
	}
}

// writeWatchError reports a watch-open failure: an unsupported resource name
// is a client error, anything else surfaces through writeError.
func writeWatchError(writer http.ResponseWriter, err error) {
	if errors.Is(err, errUnsupportedResource) {
		writeJSONError(writer, http.StatusBadRequest, err.Error())

		return
	}

	writeError(writer, err)
}

// streamWatchEvents forwards upstream events as SSE frames, ticking
// heartbeats, until the client disconnects or an upstream watch dies.
func streamWatchEvents(
	ctx context.Context,
	writer http.ResponseWriter,
	flusher http.Flusher,
	events <-chan resourceEvent,
) {
	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		if !watchTick(ctx, writer, flusher, events, ticker.C) {
			return
		}
	}
}

// watchTick handles one wakeup of the watch write loop; it reports whether
// the loop should continue.
func watchTick(
	ctx context.Context,
	writer http.ResponseWriter,
	flusher http.Flusher,
	events <-chan resourceEvent,
	heartbeat <-chan time.Time,
) bool {
	select {
	case <-ctx.Done():
		return false
	case <-heartbeat:
		err := writeSSEHeartbeat(writer)
		if err != nil {
			return false
		}

		flusher.Flush()

		return true
	case item := <-events:
		return writeWatchEvent(writer, flusher, item)
	}
}

// writeWatchEvent forwards one upstream event; a watch.Error terminates the
// stream so the client can reconnect with fresh state.
func writeWatchEvent(
	writer http.ResponseWriter,
	flusher http.Flusher,
	item resourceEvent,
) bool {
	message := transformWatchEvent(item.resource, item.event)

	err := writeSSE(writer, "resource", message)
	if err != nil {
		return false
	}

	flusher.Flush()

	return item.event.Type != watch.Error
}

// forwardWatch pumps events from one upstream watch into the shared channel.
// A closed upstream surfaces as a watch.Error event so the write loop can
// terminate the SSE stream.
func forwardWatch(
	ctx context.Context,
	resource string,
	stream watch.Interface,
	out chan<- resourceEvent,
) {
	for {
		if !forwardNextEvent(ctx, resource, stream, out) {
			return
		}
	}
}

// forwardNextEvent waits for one upstream event and forwards it, sending the
// watch.Error marker when the upstream has closed; it reports whether the
// forward loop should continue.
func forwardNextEvent(
	ctx context.Context,
	resource string,
	stream watch.Interface,
	out chan<- resourceEvent,
) bool {
	select {
	case <-ctx.Done():
		return false
	case event, ok := <-stream.ResultChan():
		if !ok {
			closed := resourceEvent{
				resource: resource,
				event:    watch.Event{Type: watch.Error, Object: nil},
			}
			sendWatchEvent(ctx, out, closed)

			return false
		}

		return sendWatchEvent(ctx, out, resourceEvent{
			resource: resource,
			event:    event,
		})
	}
}

// sendWatchEvent delivers one event, giving up when the request context
// ends; it reports whether the send happened.
func sendWatchEvent(
	ctx context.Context,
	out chan<- resourceEvent,
	item resourceEvent,
) bool {
	select {
	case out <- item:
		return true
	case <-ctx.Done():
		return false
	}
}

func transformWatchEvent(resource string, event watch.Event) watchMessage {
	message := watchMessage{
		Object:   nil,
		Type:     strings.ToLower(string(event.Type)),
		Resource: resource,
		Key:      emptyString,
	}

	meta, ok := event.Object.(metav1.Object)
	if ok {
		message.Key = meta.GetNamespace() + "/" + meta.GetName()
	}

	switch object := event.Object.(type) {
	case *corev1.Pod:
		message.Object = transformPod(object)
	case *appsv1.Deployment:
		message.Object = transformDeployment(*object)
	case *corev1.Service:
		message.Object = transformService(*object)
	case *corev1.Node:
		message.Object = transformNode(*object)
	case *corev1.Namespace:
		message.Object = transformNamespace(*object)
	case *corev1.Event:
		message.Object = transformEvent(*object)
	default:
	}

	return message
}

func setSSEHeaders(writer http.ResponseWriter) {
	writer.Header().Set(headerContentType, "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
}

// writeSSEHeartbeat emits an SSE comment, which EventSource clients ignore.
func writeSSEHeartbeat(writer io.Writer) error {
	_, err := io.WriteString(writer, ": ping\n\n")
	if err != nil {
		return fmt.Errorf("write sse heartbeat: %w", err)
	}

	return nil
}

func writeSSE(writer io.Writer, event string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode sse data: %w", err)
	}

	_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, raw)
	if err != nil {
		return fmt.Errorf("write sse event: %w", err)
	}

	return nil
}

// parseTailLines resolves the requested log tail length, falling back to a
// default and clamping to maxTailLines. Invalid or non-positive input yields
// the default.
func parseTailLines(raw string) int64 {
	if raw == emptyString {
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
	writer.Header().Set(headerContentType, "application/json")
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
		if text := http.StatusText(status); text != emptyString {
			msg = text
		}
	}

	writeJSONError(writer, status, msg)
}

func isNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
