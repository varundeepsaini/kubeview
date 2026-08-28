package main

// Tests for the cluster flight recorder: the bbolt store, the diff engine,
// the /api/history/* handlers, and the informer-driven recorder pipeline.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
)

const (
	hiContext   = "hi-context"
	hiKindPods  = "pods"
	hiKeyWeb    = "default/web"
	hiKeyOther  = "default/other"
	hiCreatedAt = "2026-08-27T09:00:00Z"

	hiWaitTimeout = 10 * time.Second
	hiWaitStep    = 20 * time.Millisecond

	hiMsgStatus = "status = %d, body = %s"

	// hiBaseUnix is 2026-08-27T12:00:00Z, the fixed reference moment the
	// store tests hang versions off.
	hiBaseUnix = 1787832000

	hiBodyPending = `{"phase":"Pending"}`
	hiBodyRunning = `{"phase":"Running"}`

	// Two bodies whose only difference is the render-time age field.
	hiBodyAgedOne = `{"status":"Running","age":"1h"}`
	hiBodyAgedTwo = `{"status":"Running","age":"2h"}`

	// Expected object counts in state assertions.
	hiWantNone = 0
	hiWantOne  = 1

	hiMsgStateAt = "state at: %v"
	hiMsgSince   = "recording since: %v"

	hiBodyStatusRunning = `{"status":"Running"}`
	hiKeyChanged        = "default/changed"

	hiCustomRetentionRaw = "24"
	hiCustomRetention    = 24 * time.Hour
	hiNoRetention        = time.Duration(zeroCount)
)

// hiBase is the fixed reference moment the store tests hang versions off.
func hiBase() time.Time {
	return time.Unix(hiBaseUnix, zeroCount).UTC()
}

func hiNewStore(t *testing.T) *HistoryStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), historyFileName)

	store, err := OpenHistoryStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	t.Cleanup(func() {
		closeErr := store.Close()
		if closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	})

	return store
}

// hiRecord builds one pending store write.
func hiRecord(
	key, changeType string,
	ts time.Time,
	object string,
) historyRecord {
	record := new(historyRecord)
	record.context = hiContext
	record.resource = hiKindPods
	record.key = key
	record.changeType = changeType
	record.createdAt = hiCreatedAt
	record.ts = ts

	if object != htEmpty {
		record.object = json.RawMessage(object)
	}

	return *record
}

func hiMustRecord(t *testing.T, store *HistoryStore, records ...historyRecord) {
	t.Helper()

	err := store.RecordBatch(records)
	if err != nil {
		t.Fatalf("record batch: %v", err)
	}
}

// hiStateKeys returns the object keys of one kind at one moment.
func hiStateKeys(
	t *testing.T,
	store *HistoryStore,
	moment time.Time,
) []string {
	t.Helper()

	state, err := store.StateAt(hiContext, moment)
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	keys := make([]string, zeroCount, len(state[hiKindPods]))
	for _, object := range state[hiKindPods] {
		keys = append(keys, object.key)
	}

	return keys
}

func TestHistoryStore_StateAtReplaysVersions(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyPending),
		hiRecord(hiKeyWeb, changeModified,
			base.Add(time.Minute), hiBodyRunning),
		hiRecord(hiKeyWeb, changeDeleted, base.Add(2*time.Minute), htEmpty),
	)

	cases := []struct {
		moment time.Time
		name   string
		body   string
		want   int
	}{
		{base.Add(-time.Second), "before recording", htEmpty, hiWantNone},
		{base, "after add", hiBodyPending, hiWantOne},
		{base.Add(time.Minute), "after modify", hiBodyRunning, hiWantOne},
		{base.Add(2 * time.Minute), "after delete", htEmpty, hiWantNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hiAssertStateAt(t, store, tc.moment, tc.want, tc.body)
		})
	}
}

// hiAssertStateAt checks the pod count (and body, when one pod is expected)
// as of one moment.
func hiAssertStateAt(
	t *testing.T,
	store *HistoryStore,
	moment time.Time,
	want int,
	body string,
) {
	t.Helper()

	state, err := store.StateAt(hiContext, moment)
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	pods := state[hiKindPods]
	if len(pods) != want {
		t.Fatalf("pods = %d, want %d", len(pods), want)
	}

	if want > zeroCount && string(pods[zeroCount].object) != body {
		t.Fatalf("object = %s, want %s", pods[zeroCount].object, body)
	}
}

func TestHistoryStore_UnrecordedContextIsEmpty(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)

	state, err := store.StateAt("never-seen", hiBase())
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	for _, kind := range historyKinds() {
		objects, found := state[kind]
		if !found {
			t.Fatalf("kind %s missing from state", kind)
		}

		if len(objects) != zeroCount {
			t.Fatalf("kind %s = %d objects, want 0", kind, len(objects))
		}
	}
}

func TestHistoryStore_SkipsUnchangedVersions(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()
	body := hiBodyRunning

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, body),
		// Same body again (a re-list replay): must not create a version.
		hiRecord(hiKeyWeb, changeModified, base.Add(time.Minute), body),
	)

	// If the duplicate had been stored, pruning with a cutoff between the two
	// timestamps would keep the newer one; since it was skipped, the original
	// survives as the only version.
	err := store.Prune(base.Add(30 * time.Second))
	if err != nil {
		t.Fatalf(hiMsgPrune, err)
	}

	keys := hiStateKeys(t, store, base)
	if len(keys) != hiWantOne {
		t.Fatalf("state after prune = %v, want the original version", keys)
	}
}

func TestHistoryStore_SkipsAgeOnlyChanges(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyAgedOne),
		// A restart re-list replays the object with only its age ticked:
		// must not create a version.
		hiRecord(hiKeyWeb, changeModified, base.Add(time.Hour), hiBodyAgedTwo),
	)

	state, err := store.StateAt(hiContext, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	pods := state[hiKindPods]
	if len(pods) != hiWantOne {
		t.Fatalf("pods = %d, want 1", len(pods))
	}

	got := string(pods[zeroCount].object)
	if got != hiBodyAgedOne {
		t.Fatalf("object = %s, want the original version %s",
			got, hiBodyAgedOne)
	}
}

func TestOpenHistoryStore_LockedFileTimesOut(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), historyFileName)

	store, err := OpenHistoryStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	t.Cleanup(func() {
		closeErr := store.Close()
		if closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	})

	// A second open on a held file lock must fail once the lock timeout
	// passes instead of blocking startup forever.
	second, err := OpenHistoryStore(path)
	if err == nil {
		closeErr := second.Close()
		t.Fatalf("second open succeeded (close: %v), want lock error",
			closeErr)
	}
}

func TestHistoryStore_SkipsTombstoneForUnknownObject(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeDeleted, base, htEmpty),
	)

	_, found, err := store.RecordingSince(hiContext)
	if err != nil {
		t.Fatalf(hiMsgSince, err)
	}

	// The write is skipped but recording-since is still stamped.
	if !found {
		t.Fatal("recording since not stamped")
	}

	keys := hiStateKeys(t, store, base.Add(time.Hour))
	if len(keys) != zeroCount {
		t.Fatalf("state = %v, want empty", keys)
	}
}

func TestHistoryStore_PruneKeepsBaseline(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyPending),
		hiRecord(hiKeyWeb, changeModified,
			base.Add(time.Minute), hiBodyRunning),
		hiRecord(hiKeyOther, changeAdded, base, hiBodyRunning),
		hiRecord(hiKeyOther, changeDeleted, base.Add(time.Minute), htEmpty),
	)

	// Cutoff after both objects' versions: web keeps its newest version as
	// the baseline; other was tombstoned pre-cutoff so it vanishes entirely.
	err := store.Prune(base.Add(time.Hour))
	if err != nil {
		t.Fatalf(hiMsgPrune, err)
	}

	keys := hiStateKeys(t, store, base.Add(2*time.Hour))
	if len(keys) != hiWantOne || keys[zeroCount] != hiKeyWeb {
		t.Fatalf("state after prune = %v, want [%s]", keys, hiKeyWeb)
	}

	state, err := store.StateAt(hiContext, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	got := string(state[hiKindPods][zeroCount].object)
	if got != hiBodyRunning {
		t.Fatalf("baseline = %s, want the newest pre-cutoff version", got)
	}
}

func TestHistoryStore_EventsBetween(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	event := func(key, body string, ts time.Time) historyRecord {
		record := hiRecord(key, changeModified, ts, body)
		record.resource = resourceEvents

		return record
	}

	hiMustRecord(t, store,
		event("default/boot", `{"reason":"TooEarly"}`, base.Add(-time.Hour)),
		event("default/pull", `{"reason":"Pulling","count":1}`, base),
		event("default/pull",
			`{"reason":"Pulling","count":2}`, base.Add(time.Minute)),
		event("default/late",
			`{"reason":"TooLate"}`, base.Add(time.Hour)),
	)

	events, err := store.EventsBetween(
		hiContext, base.Add(-time.Second), base.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("events between: %v", err)
	}

	if len(events) != hiWantOne {
		t.Fatalf("events = %d, want 1 (newest version of pull)", len(events))
	}

	want := `{"reason":"Pulling","count":2}`
	if string(events[zeroCount]) != want {
		t.Fatalf("event = %s, want %s", events[zeroCount], want)
	}
}

func TestHistoryStore_RecordingSince(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)

	_, found, err := store.RecordingSince(hiContext)
	if err != nil {
		t.Fatalf(hiMsgSince, err)
	}

	if found {
		t.Fatal("recording since set before any record")
	}

	base := hiBase()
	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyRunning),
	)

	since, found, err := store.RecordingSince(hiContext)
	if err != nil {
		t.Fatalf(hiMsgSince, err)
	}

	if !found || !since.Equal(base) {
		t.Fatalf("since = %v (found=%v), want %v", since, found, base)
	}
}

func TestParseHistoryTimestamp(t *testing.T) {
	t.Parallel()

	base := hiBase()
	cases := []struct {
		// want is nil when parsing must fail.
		want *time.Time
		name string
		raw  string
	}{
		{&base, "rfc3339", "2026-08-27T12:00:00Z"},
		{&base, "unix millis", "1787832000000"},
		{nil, "empty", htEmpty},
		{nil, "garbage", "yesterday"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hiAssertParsedTimestamp(t, tc.raw, tc.want)
		})
	}
}

// hiAssertParsedTimestamp checks one parse; a nil want expects an error.
func hiAssertParsedTimestamp(t *testing.T, raw string, want *time.Time) {
	t.Helper()

	got, err := parseHistoryTimestamp(raw)
	if want == nil {
		if err == nil {
			t.Fatalf("parse %q: expected error", raw)
		}

		return
	}

	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	if !got.Equal(*want) {
		t.Fatalf("parse %q = %v, want %v", raw, got, *want)
	}
}

func TestParseHistoryRetention(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"default", htEmpty, defaultRetentionHours * time.Hour},
		{"custom", hiCustomRetentionRaw, hiCustomRetention},
		{"disabled", "0", hiNoRetention},
		{"negative disables", "-3", hiNoRetention},
		{"invalid falls back", "soon", defaultRetentionHours * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseHistoryRetention(tc.raw)
			if got != tc.want {
				t.Fatalf("retention(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestChangeSummary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		before string
		after  string
		want   []string
	}{
		{
			"scalar change",
			`{"status":"Running","age":"1h"}`,
			`{"status":"CrashLoopBackOff","age":"2h"}`,
			[]string{"status: Running → CrashLoopBackOff"},
		},
		{
			"restart bump",
			`{"restarts":2}`,
			`{"restarts":5}`,
			[]string{"restarts: 2 → 5"},
		},
		{
			"image list",
			`{"images":["nginx:1.27"]}`,
			`{"images":["nginx:1.28"]}`,
			[]string{"images: nginx:1.27 → nginx:1.28"},
		},
		{
			"containers by name",
			`{"containers":[{"name":"app","image":"nginx:1.27"}]}`,
			`{"containers":[{"name":"app","image":"nginx:1.28"}]}`,
			[]string{"containers[app].image: nginx:1.27 → nginx:1.28"},
		},
		{
			"conditions by type",
			`{"conditions":[{"type":"MemoryPressure","status":"False"}]}`,
			`{"conditions":[{"type":"MemoryPressure","status":"True"}]}`,
			[]string{"conditions[MemoryPressure].status: False → True"},
		},
		{
			"element added",
			`{"containers":[]}`,
			`{"containers":[{"name":"sidecar","image":"envoy"}]}`,
			[]string{"containers[sidecar] added"},
		},
		{
			"unreadable body",
			`not-json`,
			`{}`,
			[]string{summaryFallback},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := changeSummary(
				json.RawMessage(tc.before), json.RawMessage(tc.after),
			)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("summary = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiffStates(t *testing.T) {
	t.Parallel()

	object := func(key, body string) historyObject {
		return historyObject{
			key:       key,
			createdAt: hiCreatedAt,
			object:    json.RawMessage(body),
		}
	}

	before := map[string][]historyObject{
		hiKindPods: {
			object("default/kept", hiBodyStatusRunning),
			object("default/gone", hiBodyStatusRunning),
			object(hiKeyChanged, hiBodyStatusRunning),
			// Only the age differs: must not surface as a modified row.
			object("default/aged", hiBodyAgedOne),
		},
	}
	after := map[string][]historyObject{
		hiKindPods: {
			object("default/kept", hiBodyStatusRunning),
			object(hiKeyChanged, `{"status":"Failed"}`),
			object("default/new", `{"status":"Pending"}`),
			object("default/aged", hiBodyAgedTwo),
		},
	}

	wantTypes := map[string]string{
		hiKeyChanged:   changeModified,
		"default/gone": diffRemoved,
		"default/new":  changeAdded,
	}

	changes := diffStates(before, after)
	if len(changes) != len(wantTypes) {
		t.Fatalf("changes = %d, want %d: %+v",
			len(changes), len(wantTypes), changes)
	}

	for _, change := range changes {
		want, expected := wantTypes[change.Key]
		if !expected {
			t.Fatalf("unexpected change for %s", change.Key)
		}

		if change.Type != want {
			t.Fatalf("%s type = %s, want %s", change.Key, change.Type, want)
		}
	}
}

// hiNewServer starts the real router with history wired to a fresh store.
func hiNewServer(t *testing.T) (*httptest.Server, *HistoryStore) {
	t.Helper()

	store := hiNewStore(t)

	client, _ := newTestClient(t, nil)
	manager := managerForClient(client)

	history := new(historyAPI)
	history.store = store
	history.retention = defaultRetentionHours * time.Hour

	router := newRouter(manager, history)
	srv := httptest.NewServer(withCORS(router, parseCORSOrigins(htEmpty)))
	t.Cleanup(srv.Close)

	return srv, store
}

// The store tests write under hiContext; the handlers resolve the manager's
// default context, so handler-level seeds must use the test client's context.
func hiSeedServer(t *testing.T, store *HistoryStore) time.Time {
	t.Helper()

	// The base derives from the wall clock — handleRange only reports a start
	// inside the retention window, so a fixed date would fail once real time
	// moves past it. Truncating to seconds matches the RFC3339 precision the
	// ?at= round-trip keeps.
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seed := []historyRecord{
		hiRecord(hiKeyWeb, changeAdded, base,
			`{"name":"web","namespace":"default","status":"Running",`+
				`"age":"1h","restarts":0}`),
		hiRecord(hiKeyWeb, changeModified, base.Add(time.Minute),
			`{"name":"web","namespace":"default","status":"Failed",`+
				`"age":"1h","restarts":3}`),
	}

	// hiAssertStatePod expects an object created three hours before the
	// viewed base moment, so the seed pins createdAt relative to base.
	createdAt := base.Add(-3 * time.Hour).Format(time.RFC3339)

	for index := range seed {
		seed[index].context = ktContextName
		seed[index].createdAt = createdAt
	}

	err := store.RecordBatch(seed)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}

	return base
}

type hiRangeBody struct {
	Start          string `json:"start"`
	End            string `json:"end"`
	RetentionHours int    `json:"retentionHours"`
	Enabled        bool   `json:"enabled"`
}

type hiStateBody struct {
	Resources map[string][]json.RawMessage `json:"resources"`
	At        string                       `json:"at"`
}

type hiDiffBody struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	Changes []historyChange   `json:"changes"`
	Events  []json.RawMessage `json:"events"`
}

func TestHandleHistory_DisabledEndpoints(t *testing.T) {
	t.Parallel()

	// newTestServer wires a nil *historyAPI: history disabled.
	srv, _ := newTestServer(t, nil)

	var rangeBody hiRangeBody

	res := getJSON(t, srv, "/api/history/range", &rangeBody)
	if res.statusCode != http.StatusOK || rangeBody.Enabled {
		t.Fatalf("range: status %d enabled %v, want 200 and disabled",
			res.statusCode, rangeBody.Enabled)
	}

	state := httpGet(t, srv.URL+"/api/history/state?at=2026-08-27T12:00:00Z")
	if state.statusCode != http.StatusNotFound {
		t.Fatalf(hiMsgStatus, state.statusCode, state.body)
	}

	diff := httpGet(t, srv.URL+
		"/api/history/diff?from=2026-08-27T12:00:00Z&to=2026-08-27T13:00:00Z")
	if diff.statusCode != http.StatusNotFound {
		t.Fatalf(hiMsgStatus, diff.statusCode, diff.body)
	}
}

func TestHandleHistory_Range(t *testing.T) {
	t.Parallel()

	srv, store := hiNewServer(t)
	base := hiSeedServer(t, store)

	var body hiRangeBody

	res := getJSON(t, srv, "/api/history/range", &body)
	if res.statusCode != http.StatusOK {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}

	if !body.Enabled {
		t.Fatal("range reports disabled")
	}

	if body.RetentionHours != defaultRetentionHours {
		t.Fatalf("retentionHours = %d, want %d",
			body.RetentionHours, defaultRetentionHours)
	}

	if body.Start != base.Format(time.RFC3339) {
		t.Fatalf("start = %s, want %s", body.Start, base.Format(time.RFC3339))
	}

	if body.End == htEmpty {
		t.Fatal("end missing")
	}
}

func TestHandleHistory_State(t *testing.T) {
	t.Parallel()

	srv, store := hiNewServer(t)
	base := hiSeedServer(t, store)

	var body hiStateBody

	path := "/api/history/state?at=" + base.Format(time.RFC3339)

	res := getJSON(t, srv, path, &body)
	if res.statusCode != http.StatusOK {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}

	pods := body.Resources[hiKindPods]
	if len(pods) != hiWantOne {
		t.Fatalf("pods = %d, want 1", len(pods))
	}

	hiAssertStatePod(t, pods[zeroCount])

	// Every kind key is present even when empty.
	for _, kind := range historyKinds() {
		if _, found := body.Resources[kind]; !found {
			t.Fatalf("kind %s missing from state response", kind)
		}
	}
}

// hiAssertStatePod checks the seeded pod's fields as returned by the state
// endpoint at the seed's base moment.
func hiAssertStatePod(t *testing.T, raw json.RawMessage) {
	t.Helper()

	var pod struct {
		Status string `json:"status"`
		Age    string `json:"age"`
	}

	err := json.Unmarshal(raw, &pod)
	if err != nil {
		t.Fatalf("decode pod: %v", err)
	}

	if pod.Status != "Running" {
		t.Fatalf("status = %s, want Running (state as of the add)", pod.Status)
	}

	// hiSeedServer pins createdAt three hours before the viewed base moment:
	// the recorded "1h" age must be rewritten relative to the viewed moment.
	if pod.Age != "3h" {
		t.Fatalf("age = %s, want 3h (recomputed as of ?at=)", pod.Age)
	}
}

func TestHandleHistory_StateBadTimestamp(t *testing.T) {
	t.Parallel()

	srv, _ := hiNewServer(t)

	res := httpGet(t, srv.URL+"/api/history/state?at=tomorrow")
	if res.statusCode != http.StatusBadRequest {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}
}

func TestHandleHistory_Diff(t *testing.T) {
	t.Parallel()

	srv, store := hiNewServer(t)
	base := hiSeedServer(t, store)

	path := fmt.Sprintf(
		"/api/history/diff?from=%s&to=%s",
		base.Format(time.RFC3339),
		base.Add(time.Hour).Format(time.RFC3339),
	)

	var body hiDiffBody

	res := getJSON(t, srv, path, &body)
	if res.statusCode != http.StatusOK {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}

	if len(body.Changes) != hiWantOne {
		t.Fatalf("changes = %d, want 1: %+v", len(body.Changes), body.Changes)
	}

	change := body.Changes[zeroCount]
	if change.Type != changeModified || change.Key != hiKeyWeb {
		t.Fatalf("change = %+v, want modified %s", change, hiKeyWeb)
	}

	wantSummary := fmt.Sprint([]string{
		"restarts: 0 → 3",
		"status: Running → Failed",
	})
	if fmt.Sprint(change.Summary) != wantSummary {
		t.Fatalf("summary = %v, want %v", change.Summary, wantSummary)
	}
}

func TestHandleHistory_DiffBadWindow(t *testing.T) {
	t.Parallel()

	srv, _ := hiNewServer(t)

	// from after to.
	res := httpGet(t, srv.URL+
		"/api/history/diff?from=2026-08-27T13:00:00Z&to=2026-08-27T12:00:00Z")
	if res.statusCode != http.StatusBadRequest {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}
}

// hiWaitFor polls until the condition holds or the deadline passes.
func hiWaitFor(t *testing.T, describe string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(hiWaitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(hiWaitStep)
	}

	t.Fatalf("timed out waiting for %s", describe)
}

func hiCreateOptions() metav1.CreateOptions {
	var opts metav1.CreateOptions

	return opts
}

func hiUpdateOptions() metav1.UpdateOptions {
	var opts metav1.UpdateOptions

	return opts
}

func hiDeleteOptions() metav1.DeleteOptions {
	var opts metav1.DeleteOptions

	return opts
}

func TestRecorder_RecordsFakeClusterChanges(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	recorders := NewRecorderManager(store, time.Hour)
	recorders.Start()
	t.Cleanup(recorders.Stop)

	client, clientset := newTestClient(t, nil)
	recorders.EnsureRecording(client)

	pod := newPod("recorded", htNSDefault)
	pods := clientset.CoreV1().Pods(htNSDefault)
	ctx := context.Background()

	_, err := pods.Create(ctx, pod, hiCreateOptions())
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	hiWaitFor(t, "pod add recorded", func() bool {
		_, found := hiRecordedPod(store)

		return found
	})

	// A status change must be recorded as a new version.
	pod.Status.Phase = corev1.PodFailed

	_, err = pods.Update(ctx, pod, hiUpdateOptions())
	if err != nil {
		t.Fatalf("update pod: %v", err)
	}

	hiWaitFor(t, "pod update recorded", func() bool {
		status, _ := hiRecordedPod(store)

		return status == "Failed"
	})

	beforeDelete := time.Now()

	err = pods.Delete(ctx, pod.Name, hiDeleteOptions())
	if err != nil {
		t.Fatalf("delete pod: %v", err)
	}

	hiWaitFor(t, "pod delete recorded", func() bool {
		_, found := hiRecordedPod(store)

		return !found
	})

	// Time travel: the pod still exists at the pre-delete moment.
	hiAssertPodExistedAt(t, store, beforeDelete)
}

// hiRecordedPod returns the recorded test pod's status as of now, and whether
// the pod is present in the recorded state at all.
func hiRecordedPod(store *HistoryStore) (string, bool) {
	state, err := store.StateAt(ktContextName, time.Now())
	if err != nil {
		return htEmpty, false
	}

	for _, object := range state[hiKindPods] {
		if object.key != htNSDefault+"/recorded" {
			continue
		}

		var fields struct {
			Status string `json:"status"`
		}

		decodeErr := json.Unmarshal(object.object, &fields)
		if decodeErr != nil {
			return htEmpty, true
		}

		return fields.Status, true
	}

	return htEmpty, false
}

// hiAssertPodExistedAt checks that exactly one pod exists in recorded state
// as of the given moment.
func hiAssertPodExistedAt(
	t *testing.T,
	store *HistoryStore,
	moment time.Time,
) {
	t.Helper()

	state, err := store.StateAt(ktContextName, moment)
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	if len(state[hiKindPods]) != hiWantOne {
		t.Fatalf("pods before delete = %d, want 1", len(state[hiKindPods]))
	}
}

func TestRecorder_StampsSortAfterPersistedVersions(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)

	// Seed a version stamped ahead of the wall clock, simulating records
	// written before a backward clock step and a restart.
	future := time.Now().Add(time.Hour)
	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, future, hiBodyRunning),
	)

	recorders := NewRecorderManager(store, time.Hour)

	stamped := recorders.stamp(
		hiRecord(hiKeyWeb, changeModified, future, hiBodyPending),
	)
	if !stamped.ts.After(future) {
		t.Fatalf("stamp = %v, want after persisted %v", stamped.ts, future)
	}
}

func TestRecorder_TombstonesDeletionsMissedWhileDown(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)

	// The store believes the pod is live: it was recorded before a downtime
	// during which the cluster deleted it.
	ghost := hiRecord(
		htNSDefault+"/ghost", changeAdded,
		time.Now().Add(-time.Hour), hiBodyRunning,
	)
	ghost.context = ktContextName
	hiMustRecord(t, store, ghost)

	recorders := NewRecorderManager(store, time.Hour)
	recorders.Start()
	t.Cleanup(recorders.Stop)

	// The restarted recorder syncs against a cluster without the pod; the
	// reconcile pass must tombstone it.
	client, _ := newTestClient(t, nil)
	recorders.EnsureRecording(client)

	hiWaitFor(t, "ghost tombstoned", func() bool {
		state, err := store.StateAt(ktContextName, time.Now())

		return err == nil && len(state[hiKindPods]) == zeroCount
	})
}

func TestRecorder_HealsDroppedAddsOnReconcile(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	recorders := NewRecorderManager(store, time.Hour)
	recorders.Start()
	t.Cleanup(recorders.Stop)

	// The pod is live in the cluster, but no event handler is registered on
	// the informers below, so its Add never reaches the store — the same gap
	// a full queue leaves behind when it drops the record.
	client, _ := newTestClient(t, nil, newPod("dropped", htNSDefault))

	factory := informers.NewSharedInformerFactory(
		client.streamClientset, noResync,
	)
	kindInformers := historyInformers(factory)

	factory.Start(recorders.stop)

	// Mirrors startInformers: reconcileWhenSynced waits for the caches, runs
	// the reconcile pass, and releases the wait-group slot added here.
	recorders.waitGroup.Add(reconcileLoopCount)
	recorders.reconcileWhenSynced(ktContextName, kindInformers)

	hiWaitFor(t, "dropped add healed", func() bool {
		state, err := store.StateAt(ktContextName, time.Now())

		return err == nil && len(state[hiKindPods]) == hiWantOne
	})
}

// --- coverage beyond the adversarial-review rounds ---

const (
	hiOtherContext = "hi-other-context"

	// hiConcurrentWrites sizes the concurrency smoke test: enough traffic for
	// the race detector to interleave readers with the writer.
	hiConcurrentWrites = 64

	hiMsgEvents = "events between: %v"
	hiMsgPrune  = "prune: %v"
)

// hiEventRecord builds one pending write for the events kind.
func hiEventRecord(key, body string, ts time.Time) historyRecord {
	record := hiRecord(key, changeModified, ts, body)
	record.resource = resourceEvents

	return record
}

func TestHistoryStore_ContextIsolation(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	other := hiRecord(hiKeyWeb, changeAdded, base, hiBodyRunning)
	other.context = hiOtherContext

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyPending),
		other,
	)

	// Same object key, different contexts: each context must read back only
	// its own version.
	hiAssertStateAt(t, store, base, hiWantOne, hiBodyPending)

	state, err := store.StateAt(hiOtherContext, base)
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	got := string(state[hiKindPods][zeroCount].object)
	if got != hiBodyRunning {
		t.Fatalf("other context = %s, want %s", got, hiBodyRunning)
	}

	// Recording-since must not leak across contexts either.
	_, found, err := store.RecordingSince("hi-context-never-recorded")
	if err != nil {
		t.Fatalf(hiMsgSince, err)
	}

	if found {
		t.Fatal("recording-since leaked into an unrecorded context")
	}
}

func TestHistoryStore_PrefixKeysStayDistinct(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()
	longKey := hiKeyWeb + "-1"

	// "default/web" is a byte prefix of "default/web-1"; the NUL separator in
	// version keys must keep their version chains apart.
	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base, hiBodyPending),
		hiRecord(longKey, changeAdded, base, hiBodyRunning),
		hiRecord(longKey, changeDeleted, base.Add(time.Minute), htEmpty),
	)

	// The delta check for the short key must compare against the short key's
	// latest version (an identical body: skip), never the long key's
	// tombstone (which would wrongly re-record).
	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeModified,
			base.Add(2*time.Minute), hiBodyPending),
	)

	keys := hiStateKeys(t, store, base.Add(3*time.Minute))
	if len(keys) != hiWantOne || keys[zeroCount] != hiKeyWeb {
		t.Fatalf("state = %v, want only %s", keys, hiKeyWeb)
	}

	hiAssertStateAt(t, store, base.Add(3*time.Minute), hiWantOne, hiBodyPending)
}

func TestHistoryStore_EventsBetweenWindowBounds(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	hiMustRecord(t, store,
		hiEventRecord("default/at-from", `{"reason":"AtFrom"}`, base),
		hiEventRecord("default/at-to",
			`{"reason":"AtTo"}`, base.Add(time.Minute)),
	)

	// The window is half-open (from, to]: a record stamped exactly at from is
	// excluded, one exactly at to is included.
	events, err := store.EventsBetween(hiContext, base, base.Add(time.Minute))
	if err != nil {
		t.Fatalf(hiMsgEvents, err)
	}

	if len(events) != hiWantOne {
		t.Fatalf("events = %d, want 1 (only the at-to record)", len(events))
	}

	want := `{"reason":"AtTo"}`
	if string(events[zeroCount]) != want {
		t.Fatalf("event = %s, want %s", events[zeroCount], want)
	}
}

func TestHistoryStore_PruneCutoffBoundary(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()
	cutoff := base.Add(time.Minute)

	// The post-cutoff body must differ from the at-cutoff one beyond its age
	// field, or the store's age-only delta skip would drop it.
	failedBody := `{"status":"Failed","age":"2h"}`

	hiMustRecord(t, store,
		hiRecord(hiKeyWeb, changeAdded, base.Add(-time.Minute), hiBodyPending),
		hiRecord(hiKeyWeb, changeModified, base, hiBodyRunning),
		hiRecord(hiKeyWeb, changeModified, cutoff, hiBodyAgedOne),
		hiRecord(hiKeyWeb, changeModified, cutoff.Add(time.Minute), failedBody),
	)

	err := store.Prune(cutoff)
	if err != nil {
		t.Fatalf(hiMsgPrune, err)
	}

	// The superseded pre-cutoff version is gone, the newest pre-cutoff one is
	// the baseline, and a version stamped exactly at the cutoff is not
	// "before" it — it and everything later survive untouched.
	hiAssertStateAt(t, store, base.Add(-time.Minute), hiWantNone, htEmpty)
	hiAssertStateAt(t, store, base, hiWantOne, hiBodyRunning)
	hiAssertStateAt(t, store, cutoff, hiWantOne, hiBodyAgedOne)
	hiAssertStateAt(t, store, cutoff.Add(time.Hour), hiWantOne, failedBody)
}

func TestHistoryStore_ConcurrentReadsDuringWrites(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	base := hiBase()

	var writer sync.WaitGroup

	writer.Go(func() {
		hiWriteVersionBurst(t, store, base)
	})

	// Readers overlap the writer; the race detector flags any tx misuse.
	for range hiConcurrentWrites {
		_, err := store.StateAt(hiContext, base.Add(time.Hour))
		if err != nil {
			t.Fatalf(hiMsgStateAt, err)
		}

		_, err = store.EventsBetween(hiContext, base, base.Add(time.Hour))
		if err != nil {
			t.Fatalf(hiMsgEvents, err)
		}
	}

	writer.Wait()
}

// hiWriteVersionBurst records a run of distinct versions, one batch each.
func hiWriteVersionBurst(t *testing.T, store *HistoryStore, base time.Time) {
	t.Helper()

	for index := range hiConcurrentWrites {
		record := hiRecord(
			hiKeyWeb, changeModified,
			base.Add(time.Duration(index)*time.Millisecond),
			fmt.Sprintf(`{"n":%d}`, index),
		)

		err := store.RecordBatch([]historyRecord{record})
		if err != nil {
			t.Errorf("record: %v", err)

			return
		}
	}
}

func TestRewriteAge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		createdAt string
		object    string
		wantAge   string
	}{
		// createdAt 09:00, viewed moment 12:00 (hiBase): 3h.
		{"rewrites relative to the moment", hiCreatedAt, hiBodyAgedOne, "3h"},
		// Unchanged pass-throughs report wantAge empty.
		{"no age field", hiCreatedAt, hiBodyPending, htEmpty},
		{"bad createdAt", "not-a-time", hiBodyAgedOne, htEmpty},
		{"invalid object json", hiCreatedAt, `{"age":`, htEmpty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hiAssertRewriteAge(t, tc.createdAt, tc.object, tc.wantAge)
		})
	}
}

// hiAssertRewriteAge checks one rewrite; an empty wantAge expects the object
// bytes to pass through unchanged.
func hiAssertRewriteAge(t *testing.T, createdAt, object, wantAge string) {
	t.Helper()

	source := historyObject{
		key:       hiKeyWeb,
		createdAt: createdAt,
		object:    json.RawMessage(object),
	}

	got := rewriteAge(source, hiBase())
	if wantAge == htEmpty {
		if string(got) != object {
			t.Fatalf("object rewritten to %s, want unchanged %s", got, object)
		}

		return
	}

	var fields struct {
		Age    string `json:"age"`
		Status string `json:"status"`
	}

	err := json.Unmarshal(got, &fields)
	if err != nil {
		t.Fatalf("decode rewritten object: %v", err)
	}

	if fields.Age != wantAge || fields.Status != itStatusRunning {
		t.Fatalf("age = %q status = %q, want %q with status preserved",
			fields.Age, fields.Status, wantAge)
	}
}

// hiOverCapFields is a field count safely past the summary line cap.
const hiOverCapFields = summaryLimit + summaryLimit

// hiManyFields builds an object body with distinct scalar values per field so
// every field differs between two generated bodies.
func hiManyFields(value int) string {
	fields := make([]string, zeroCount, hiOverCapFields)
	for index := range hiOverCapFields {
		fields = append(
			fields, fmt.Sprintf(`"field%02d":%d`, index, value+index),
		)
	}

	return "{" + strings.Join(fields, ",") + "}"
}

func TestChangeSummary_CapsLineCount(t *testing.T) {
	t.Parallel()

	got := changeSummary(
		json.RawMessage(hiManyFields(zeroCount)),
		json.RawMessage(hiManyFields(hiConcurrentWrites)),
	)

	if len(got) != summaryLimit {
		t.Fatalf("summary lines = %d, want capped at %d",
			len(got), summaryLimit)
	}
}

func TestRecorder_StopFlushesQueuedRecords(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	recorders := NewRecorderManager(store, time.Hour)
	recorders.Start()

	recorders.enqueue(
		hiContext, hiKindPods, changeAdded, newPod("queued", htNSDefault),
	)
	recorders.Stop()

	// Stop drains the queue before returning; the record must be durable.
	state, err := store.StateAt(hiContext, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf(hiMsgStateAt, err)
	}

	if len(state[hiKindPods]) != hiWantOne {
		t.Fatalf("pods after stop = %d, want 1", len(state[hiKindPods]))
	}
}

func TestHandleHistory_BrowsingStartsRecording(t *testing.T) {
	t.Parallel()

	store := hiNewStore(t)
	recorders := NewRecorderManager(store, defaultRetentionHours*time.Hour)
	recorders.Start()
	t.Cleanup(recorders.Stop)

	client, clientset := newTestClient(t, nil)
	manager := managerForClient(client)

	history := new(historyAPI)
	history.store = store
	history.recorders = recorders
	history.retention = defaultRetentionHours * time.Hour

	srv := httptest.NewServer(
		withCORS(newRouter(manager, history), parseCORSOrigins(htEmpty)),
	)
	t.Cleanup(srv.Close)

	// Browsing any resource endpoint lazily starts the context's recorder.
	res := httpGet(t, srv.URL+htPathPods)
	if res.statusCode != http.StatusOK {
		t.Fatalf(hiMsgStatus, res.statusCode, res.body)
	}

	// A pod created afterwards must reach the store with no further calls —
	// proof the informers are running for the browsed context.
	_, err := clientset.CoreV1().Pods(htNSDefault).Create(
		context.Background(), newPod("browsed", htNSDefault),
		hiCreateOptions(),
	)
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	hiWaitFor(t, "recording started by browsing", func() bool {
		state, stateErr := store.StateAt(ktContextName, time.Now())

		return stateErr == nil && len(state[hiKindPods]) == hiWantOne
	})
}

func TestHistoryStorePath(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "history")

	path, err := historyStorePath(dir)
	if err != nil {
		t.Fatalf("store path: %v", err)
	}

	if path != filepath.Join(dir, historyFileName) {
		t.Fatalf("path = %s, want it inside %s", path, dir)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("configured dir not created: %v", err)
	}
}

func TestHistoryStorePath_DefaultsToUserCache(t *testing.T) {
	cache := t.TempDir()
	// t.Setenv forbids t.Parallel; os.UserCacheDir honors XDG_CACHE_HOME.
	t.Setenv("XDG_CACHE_HOME", cache)

	path, err := historyStorePath(emptyString)
	if err != nil {
		t.Fatalf("store path: %v", err)
	}

	want := filepath.Join(cache, "kubeview", historyFileName)
	if path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
}
