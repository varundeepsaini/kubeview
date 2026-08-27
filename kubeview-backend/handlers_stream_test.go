package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	core "k8s.io/client-go/testing"
)

// Constants for the SSE streaming tests. The "hs" prefix avoids collisions
// with constants declared in sibling test files.
const (
	hsPathWatchPods = "/api/watch?resources=pods"
	hsPathWatchTwo  = "/api/watch?resources=pods,deployments"
	hsPathWatchDup  = "/api/watch?resources=pods,pods"
	hsPathFollow    = htPathWebLogs + "?follow=true"

	hsEventReady    = "ready"
	hsEventResource = "resource"
	hsEventLog      = "log"
	hsEventEnd      = "end"

	hsTypeAdded    = "added"
	hsTypeModified = "modified"
	hsTypeDeleted  = "deleted"

	hsReasonEOF   = "eof"
	hsReasonError = "error"

	hsPingComment = ": ping"
	hsContentSSE  = "text/event-stream"

	hsKeyWeb = "default/web"
	hsKeyA   = "default/a"
	hsKeyB   = "default/b"

	hsNSTeamA = "team-a"

	hsMsgEvent  = "event = %q, data = %q"
	hsMsgReason = "reason = %q, want %q"
	hsMsgReady  = "ready resources = %v"

	// One upstream watch per deduped resource name.
	hsOneWatch = 1

	// Longer than bufio.Scanner's 64KB default token limit but under the
	// handler's 1MB cap, so the line must survive intact.
	hsLongLineLen = logScanInitialBuf + logScanInitialBuf/2
	// Over the handler's 1MB cap, so the scan must fail with ErrTooLong.
	hsOversizeLen = logScanMaxBuf + 2

	// Upper bounds so a broken stream fails the test instead of hanging it.
	hsStreamTimeout = 10 * time.Second
	hsHandlerWait   = 5 * time.Second
)

// sseFrame is one server-sent block: the lines between blank-line separators.
// Heartbeats carry only a comment; real events carry event and data.
type sseFrame struct {
	event   string
	data    string
	comment string
}

// readSSEFrame reads the next frame from the stream, failing the test on a
// read error (including the request-context deadline).
func readSSEFrame(t *testing.T, reader *bufio.Reader) sseFrame {
	t.Helper()

	var frame sseFrame

	seen := false

	for {
		line := readSSELine(t, reader)
		if line == htEmpty {
			if seen {
				return frame
			}

			continue
		}

		seen = true

		parseSSELine(t, &frame, line)
	}
}

// readSSELine reads one line from the stream without its trailing newline.
func readSSELine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	raw, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read sse frame: %v", err)
	}

	return strings.TrimSuffix(raw, "\n")
}

// parseSSELine folds one non-blank SSE line into the frame under assembly.
func parseSSELine(t *testing.T, frame *sseFrame, line string) {
	t.Helper()

	switch {
	case strings.HasPrefix(line, "event: "):
		frame.event = strings.TrimPrefix(line, "event: ")
	case strings.HasPrefix(line, "data: "):
		frame.data = strings.TrimPrefix(line, "data: ")
	case strings.HasPrefix(line, ":"):
		frame.comment = line
	default:
		t.Fatalf("unexpected sse line %q", line)
	}
}

// readSSEEvent returns the next named event, skipping heartbeat comments.
func readSSEEvent(t *testing.T, reader *bufio.Reader) sseFrame {
	t.Helper()

	for {
		frame := readSSEFrame(t, reader)
		if frame.event != htEmpty {
			return frame
		}
	}
}

// openSSE issues a GET for an SSE endpoint and returns a reader over the
// live response body. The request context carries a deadline so a stream
// that never produces the expected frame fails the test instead of hanging.
func openSSE(t *testing.T, srv *httptest.Server, path string) *bufio.Reader {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), hsStreamTimeout)
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+path, nil,
	)
	if err != nil {
		t.Fatalf(htMsgNewReq, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open sse %s: %v", path, err)
	}

	t.Cleanup(func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			t.Logf("close sse body: %v", cerr)
		}
	})

	if resp.StatusCode != htStatusOK {
		t.Fatalf(htMsgStatus, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != hsContentSSE {
		t.Fatalf("Content-Type = %q, want %q", ct, hsContentSSE)
	}

	return bufio.NewReader(resp.Body)
}

// fakeWatchReactor is the slice of *fake.Clientset the watch tests need,
// reached through the kubernetes.Interface the production code holds.
type fakeWatchReactor interface {
	PrependWatchReactor(resource string, reaction core.WatchReactionFunc)
}

func injectWatchReactor(
	t *testing.T,
	cs any,
	resource string,
	reaction core.WatchReactionFunc,
) {
	t.Helper()

	r, ok := cs.(fakeWatchReactor)
	if !ok {
		t.Fatalf("clientset is not a fake watch reactor, got %T", cs)
	}

	r.PrependWatchReactor(resource, reaction)
}

// injectFakeWatch wires a FakeWatcher as the upstream watch for resource and
// returns it so the test can emit events and inspect its stopped state.
func injectFakeWatch(t *testing.T, cs any, resource string) *watch.FakeWatcher {
	t.Helper()

	fw := watch.NewFake()
	injectWatchReactor(t, cs, resource, core.DefaultWatchReactor(fw, nil))

	return fw
}

// Wire shapes of the SSE payloads, mirroring what the frontend decodes.

type readyBody struct {
	Resources []string `json:"resources"`
}

type watchBody struct {
	Type     string          `json:"type"`
	Resource string          `json:"resource"`
	Key      string          `json:"key"`
	Object   json.RawMessage `json:"object"`
}

type endBody struct {
	Reason string `json:"reason"`
}

func decodeSSE(t *testing.T, frame sseFrame, dst any) {
	t.Helper()

	err := json.Unmarshal([]byte(frame.data), dst)
	if err != nil {
		t.Fatalf("decode sse data %q: %v", frame.data, err)
	}
}

// requireReady reads the next event, asserts it is "ready", and returns the
// resource list it announced.
func requireReady(t *testing.T, reader *bufio.Reader) []string {
	t.Helper()

	frame := readSSEEvent(t, reader)
	if frame.event != hsEventReady {
		t.Fatalf(hsMsgEvent, frame.event, frame.data)
	}

	var ready readyBody

	decodeSSE(t, frame, &ready)

	return ready.Resources
}

// requireResourceEvent reads the next event, asserts it is a "resource" event
// with the given type/resource/key, and returns the decoded payload.
func requireResourceEvent(
	t *testing.T,
	reader *bufio.Reader,
	wantType, wantResource, wantKey string,
) watchBody {
	t.Helper()

	frame := readSSEEvent(t, reader)
	if frame.event != hsEventResource {
		t.Fatalf(hsMsgEvent, frame.event, frame.data)
	}

	var body watchBody

	decodeSSE(t, frame, &body)

	if body.Type != wantType ||
		body.Resource != wantResource ||
		body.Key != wantKey {
		t.Fatalf(
			"watch event = %s/%s/%s, want %s/%s/%s",
			body.Type, body.Resource, body.Key,
			wantType, wantResource, wantKey,
		)
	}

	return body
}

// --- GET /api/watch ---

func TestHandle_Watch(t *testing.T) {
	t.Parallel()
	t.Run("ready event sent before any watch activity", watchReadyImmediate)
	t.Run("forwards added modified deleted", watchForwardsEvents)
	t.Run("forwards namespace filter upstream", watchForwardsNamespace)
	t.Run("multiplexes multiple resources", watchMultiplexesResources)
	t.Run("dedupes duplicate resource names", watchDedupesResources)
	t.Run("unsupported resource -> 400", watchUnsupportedResource)
	t.Run("missing resources param -> 400", watchMissingResources)
	t.Run("upstream watch failure -> 500", watchUpstreamFailure)
	t.Run("client disconnect stops upstream watches", watchDisconnectStops)
}

func watchReadyImmediate(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	injectFakeWatch(t, c.clientset, htResPods)

	// The fake watcher never emits, so "ready" arriving at all proves the
	// handler wrote and flushed it up front rather than after a first event.
	reader := openSSE(t, srv, hsPathWatchPods)

	resources := requireReady(t, reader)
	if len(resources) != htOne || resources[htFirst] != htResPods {
		t.Fatalf(hsMsgReady, resources)
	}
}

func watchForwardsEvents(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	fw := injectFakeWatch(t, c.clientset, htResPods)
	reader := openSSE(t, srv, hsPathWatchPods)
	requireReady(t, reader)

	pod := newPod(htNameWeb, htNSDefault)

	fw.Add(pod)

	body := requireResourceEvent(t, reader, hsTypeAdded, htResPods, hsKeyWeb)

	var obj struct {
		Name string `json:"name"`
	}

	err := json.Unmarshal(body.Object, &obj)
	if err != nil {
		t.Fatalf("decode object: %v", err)
	}

	if obj.Name != htNameWeb {
		t.Fatalf("object name = %q", obj.Name)
	}

	fw.Modify(pod)
	requireResourceEvent(t, reader, hsTypeModified, htResPods, hsKeyWeb)

	fw.Delete(pod)
	requireResourceEvent(t, reader, hsTypeDeleted, htResPods, hsKeyWeb)
}

// The frontend opens /watch?...&namespace=<filter> and trusts the backend to
// scope the upstream watch; dropping the param would leak cluster-wide events
// into namespace-filtered tables.
func watchForwardsNamespace(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	fw := watch.NewFake()
	namespaces := make(chan string, htOne)

	injectWatchReactor(
		t, c.clientset, htResPods,
		func(action core.Action) (bool, watch.Interface, error) {
			namespaces <- action.GetNamespace()

			return true, fw, nil
		},
	)

	reader := openSSE(t, srv, hsPathWatchPods+"&namespace="+hsNSTeamA)
	requireReady(t, reader)

	// The ready event is only written after the upstream watch is open, so
	// the reactor has already sent by the time it arrives.
	if got := <-namespaces; got != hsNSTeamA {
		t.Fatalf("upstream watch namespace = %q, want %q", got, hsNSTeamA)
	}
}

func watchMultiplexesResources(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	fwPods := injectFakeWatch(t, c.clientset, htResPods)
	fwDeps := injectFakeWatch(t, c.clientset, htResDeployments)
	reader := openSSE(t, srv, hsPathWatchTwo)

	resources := requireReady(t, reader)
	if len(resources) != htTwo ||
		resources[htFirst] != htResPods ||
		resources[htOne] != htResDeployments {
		t.Fatalf(hsMsgReady, resources)
	}

	fwPods.Add(newPod(htNameWeb, htNSDefault))
	requireResourceEvent(t, reader, hsTypeAdded, htResPods, hsKeyWeb)

	fwDeps.Add(newDeployment(htNameAPI, htNSDefault))
	requireResourceEvent(
		t, reader, hsTypeAdded, htResDeployments, htNSDefault+"/"+htNameAPI,
	)
}

func watchDedupesResources(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	fw := watch.NewFake()

	var opened atomic.Int32

	injectWatchReactor(
		t, c.clientset, htResPods,
		func(core.Action) (bool, watch.Interface, error) {
			opened.Add(htOne)

			return true, fw, nil
		},
	)

	reader := openSSE(t, srv, hsPathWatchDup)

	resources := requireReady(t, reader)
	if len(resources) != htOne || resources[htFirst] != htResPods {
		t.Fatalf(hsMsgReady, resources)
	}

	// The ready event is only written after every upstream watch is open, so
	// the counter is settled by the time it arrives.
	if got := opened.Load(); got != hsOneWatch {
		t.Fatalf("upstream watches opened = %d, want %d", got, hsOneWatch)
	}

	// Each event must be delivered exactly once: the event after a's added
	// event is b's, not a duplicate of a's.
	fw.Add(newPod(htNameA, htNSDefault))
	requireResourceEvent(t, reader, hsTypeAdded, htResPods, hsKeyA)

	fw.Add(newPod(htNameB, htNSDefault))
	requireResourceEvent(t, reader, hsTypeAdded, htResPods, hsKeyB)
}

func watchUnsupportedResource(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := httpGet(t, srv.URL+"/api/watch?resources=cronjobs")
	if res.statusCode != http.StatusBadRequest {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}

	var e errResp

	decodeErr := json.Unmarshal(res.body, &e)
	if decodeErr != nil {
		t.Fatalf(htMsgDecode, decodeErr)
	}

	if !strings.Contains(e.Error, "unsupported watch resource") {
		t.Fatalf("error = %q", e.Error)
	}
}

func watchMissingResources(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := httpGet(t, srv.URL+"/api/watch")
	if res.statusCode != http.StatusBadRequest {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

func watchUpstreamFailure(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil)
	injectWatchReactor(
		t, c.clientset, htResPods,
		func(core.Action) (bool, watch.Interface, error) {
			return true, nil, errBoom
		},
	)

	res := httpGet(t, srv.URL+hsPathWatchPods)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

func watchDisconnectStops(t *testing.T) {
	t.Parallel()

	client, clientset := newTestClient(t, nil)
	fw := injectFakeWatch(t, clientset, htResPods)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequestWithContext(
		ctx, http.MethodGet, hsPathWatchPods, nil,
	)
	rec := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		handleWatch(client)(rec, req)
		close(done)
	}()

	// FakeWatcher.Add blocks until the handler's forwarder receives, which
	// proves the watch loop is running before the context is canceled.
	fw.Add(newPod(htNameWeb, htNSDefault))
	cancel()

	select {
	case <-done:
	case <-time.After(hsHandlerWait):
		t.Fatal("handler did not return after context cancel")
	}

	if !fw.IsStopped() {
		t.Fatal("upstream watcher not stopped after disconnect")
	}
}

// --- GET /api/pods/{ns}/{name}/logs?follow=true ---

func TestHandle_PodLogStream(t *testing.T) {
	t.Parallel()
	t.Run("heartbeat commits stream before log data", logStreamHeartbeatFirst)
	t.Run("forwards lines then ends with eof", logStreamForwardsLines)
	t.Run("empty stream still ends with eof", logStreamEmptyEndsEOF)
	t.Run("line over 64KB survives intact", logStreamLongLine)
	t.Run("oversized line ends with error", logStreamOversizedLine)
	t.Run("follow sets Follow and default tail", logStreamFollowOpts)
	t.Run("open failure -> 500", logStreamOpenFailure)
	t.Run("missing pod -> 404", logStreamMissingPod)
}

// requireEnd reads the next event, asserts it is "end" with the given reason.
func requireEnd(t *testing.T, reader *bufio.Reader, wantReason string) {
	t.Helper()

	frame := readSSEEvent(t, reader)
	if frame.event != hsEventEnd {
		t.Fatalf(hsMsgEvent, frame.event, frame.data)
	}

	var end endBody

	decodeSSE(t, frame, &end)

	if end.Reason != wantReason {
		t.Fatalf(hsMsgReason, end.Reason, wantReason)
	}
}

// requireLogLine reads the next event, asserts it is a "log" event, and
// returns the decoded line.
func requireLogLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	frame := readSSEEvent(t, reader)
	if frame.event != hsEventLog {
		t.Fatalf(hsMsgEvent, frame.event, frame.data)
	}

	var line string

	decodeSSE(t, frame, &line)

	return line
}

// newLogStreamServer builds a server whose pod-log endpoint streams the given
// bytes and then hits EOF (the fake clientset serves logs from memory).
func newLogStreamServer(t *testing.T, logs string) *httptest.Server {
	t.Helper()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))
	injectLogsReactor(t, c.clientset, func(*corev1.PodLogOptions) []byte {
		return []byte(logs)
	})

	return srv
}

func logStreamHeartbeatFirst(t *testing.T) {
	t.Parallel()

	srv := newLogStreamServer(t, "one\n")
	reader := openSSE(t, srv, hsPathFollow)

	// The very first frame must be the commit heartbeat, before any log data.
	first := readSSEFrame(t, reader)
	if first.comment != hsPingComment || first.event != htEmpty {
		t.Fatalf("first frame = %+v, want ping comment", first)
	}

	second := readSSEFrame(t, reader)
	if second.event != hsEventLog {
		t.Fatalf(hsMsgEvent, second.event, second.data)
	}
}

func logStreamForwardsLines(t *testing.T) {
	t.Parallel()

	srv := newLogStreamServer(t, "one\ntwo\n")
	reader := openSSE(t, srv, hsPathFollow)

	if line := requireLogLine(t, reader); line != "one" {
		t.Fatalf("line = %q", line)
	}

	if line := requireLogLine(t, reader); line != "two" {
		t.Fatalf("line = %q", line)
	}

	requireEnd(t, reader, hsReasonEOF)
}

func logStreamEmptyEndsEOF(t *testing.T) {
	t.Parallel()

	srv := newLogStreamServer(t, htEmpty)
	reader := openSSE(t, srv, hsPathFollow)
	requireEnd(t, reader, hsReasonEOF)
}

func logStreamLongLine(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", hsLongLineLen)
	srv := newLogStreamServer(t, long+"\n")
	reader := openSSE(t, srv, hsPathFollow)

	if line := requireLogLine(t, reader); line != long {
		t.Fatalf("long line corrupted: len = %d, want %d", len(line), len(long))
	}

	requireEnd(t, reader, hsReasonEOF)
}

func logStreamOversizedLine(t *testing.T) {
	t.Parallel()

	srv := newLogStreamServer(t, strings.Repeat("x", hsOversizeLen))
	reader := openSSE(t, srv, hsPathFollow)

	// The scanner rejects the token before producing any line, so the first
	// named event is the error terminator.
	requireEnd(t, reader, hsReasonError)
}

func logStreamFollowOpts(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))

	var captured corev1.PodLogOptions

	injectLogsReactor(t, c.clientset, func(opts *corev1.PodLogOptions) []byte {
		captured = *opts

		return nil
	})

	reader := openSSE(t, srv, hsPathFollow+"&tailLines=abc")
	requireEnd(t, reader, hsReasonEOF)

	if !captured.Follow {
		t.Fatal("Follow not set on upstream log request")
	}

	if captured.TailLines == nil || *captured.TailLines != htDefaultTail {
		t.Fatalf("tailLines = %v, want %d", captured.TailLines, htDefaultTail)
	}
}

func logStreamOpenFailure(t *testing.T) {
	t.Parallel()

	srv, c := newTestServer(t, nil, newPod(htNameWeb, htNSDefault))
	injectLogsErrorReactor(t, c.clientset, errBoom)

	res := httpGet(t, srv.URL+hsPathFollow)
	if res.statusCode != htStatusErr {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}

func logStreamMissingPod(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)

	res := httpGet(t, srv.URL+"/api/pods/default/nope/logs?follow=true")
	if res.statusCode != htStatusNotF {
		t.Fatalf(htMsgStatusBody, res.statusCode, res.body)
	}
}
