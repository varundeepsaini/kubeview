package main

// HTTP surface of the cluster flight recorder:
//
//	GET /api/history/range          scrubber bounds for the context
//	GET /api/history/state?at=…     cluster state as of a past moment
//	GET /api/history/diff?from=&to= what changed between two moments
//
// Timestamps are accepted as RFC3339 or unix milliseconds and returned as
// RFC3339 UTC, matching the rest of the API.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	paramAt   = "at"
	paramFrom = "from"
	paramTo   = "to"

	historyTimestampBase    = 10
	historyTimestampBitSize = 64
)

// errBadTimestamp marks an at/from/to value that is neither RFC3339 nor unix
// milliseconds; handlers map it to HTTP 400.
var errBadTimestamp = errors.New("invalid timestamp")

// historyAPI wires the history endpoints to the store and recorder. A nil
// *historyAPI is valid and means history is disabled: the range endpoint
// reports enabled=false and the query endpoints answer 404.
type historyAPI struct {
	store     *HistoryStore
	recorders *RecorderManager
	retention time.Duration
}

// recording wraps a resource handler so that browsing a context starts its
// flight recorder; a nil API passes handlers through untouched.
func (h *historyAPI) recording(handler contextHandler) contextHandler {
	if h == nil {
		return handler
	}

	return func(client *Client, writer http.ResponseWriter, req *http.Request) {
		h.recorders.EnsureRecording(client)
		handler(client, writer, req)
	}
}

// historyRangeResponse mirrors what the frontend expects in
// kubeview-frontend/src/lib/api.ts. Start is empty until the context has
// recorded at least one change.
type historyRangeResponse struct {
	Start          string `json:"start,omitempty"`
	End            string `json:"end,omitempty"`
	RetentionHours int    `json:"retentionHours,omitempty"`
	Enabled        bool   `json:"enabled"`
}

func (h *historyAPI) handleRange(
	client *Client,
	writer http.ResponseWriter,
	_ *http.Request,
) {
	var resp historyRangeResponse
	if h == nil {
		writeJSON(writer, http.StatusOK, resp)

		return
	}

	since, found, err := h.store.RecordingSince(client.context)
	if err != nil {
		writeError(writer, err)

		return
	}

	now := time.Now()
	resp.Enabled = true
	resp.End = now.UTC().Format(time.RFC3339)
	resp.RetentionHours = int(h.retention.Hours())

	if found {
		start := now.Add(-h.retention)
		if since.After(start) {
			start = since
		}

		resp.Start = start.UTC().Format(time.RFC3339)
	}

	writeJSON(writer, http.StatusOK, resp)
}

func (h *historyAPI) handleState(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	if h == nil {
		writeJSONError(writer, http.StatusNotFound, "History is disabled")

		return
	}

	moment, err := parseHistoryTimestamp(req.URL.Query().Get(paramAt))
	if err != nil {
		writeJSONError(
			writer, http.StatusBadRequest, "Invalid or missing at timestamp",
		)

		return
	}

	state, err := h.store.StateAt(client.context, moment)
	if err != nil {
		writeError(writer, err)

		return
	}

	writeJSON(writer, http.StatusOK, historyStateResponse(moment, state))
}

// historyStateResponse shapes the state map for the frontend: every kind key
// present (empty array when nothing existed), with object ages recomputed as
// of the viewed moment.
func historyStateResponse(
	moment time.Time,
	state map[string][]historyObject,
) map[string]any {
	resources := make(map[string]any, len(historyKinds()))

	for _, kind := range historyKinds() {
		objects := state[kind]

		out := make([]json.RawMessage, zeroCount, len(objects))
		for _, object := range objects {
			out = append(out, rewriteAge(object, moment))
		}

		resources[kind] = out
	}

	return map[string]any{
		"at":        moment.UTC().Format(time.RFC3339),
		"resources": resources,
	}
}

// rewriteAge recomputes an object's pre-rendered age string relative to the
// viewed moment: the recorded age reflects when the version was written,
// which can be long before the moment being viewed.
func rewriteAge(object historyObject, moment time.Time) json.RawMessage {
	created, err := time.Parse(time.RFC3339, object.createdAt)
	if err != nil {
		return object.object
	}

	var fields map[string]any

	err = json.Unmarshal(object.object, &fields)
	if err != nil {
		return object.object
	}

	if _, found := fields[fieldAge]; !found {
		return object.object
	}

	fields[fieldAge] = ageBetween(created, moment)

	raw, err := json.Marshal(fields)
	if err != nil {
		return object.object
	}

	return raw
}

func (h *historyAPI) handleDiff(
	client *Client,
	writer http.ResponseWriter,
	req *http.Request,
) {
	if h == nil {
		writeJSONError(writer, http.StatusNotFound, "History is disabled")

		return
	}

	window, valid := parseDiffWindow(writer, req)
	if !valid {
		return
	}

	before, err := h.store.StateAt(client.context, window.from)
	if err != nil {
		writeError(writer, err)

		return
	}

	after, err := h.store.StateAt(client.context, window.to)
	if err != nil {
		writeError(writer, err)

		return
	}

	events, err := h.store.EventsBetween(client.context, window.from, window.to)
	if err != nil {
		writeError(writer, err)

		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"from":    window.from.UTC().Format(time.RFC3339),
		"to":      window.to.UTC().Format(time.RFC3339),
		"changes": diffStates(before, after),
		"events":  events,
	})
}

// diffWindow is a validated pair of diff endpoints, from strictly before to.
type diffWindow struct {
	from time.Time
	to   time.Time
}

// parseDiffWindow reads and validates ?from= and ?to=, reporting client
// errors itself; valid is false when the response has already been written.
func parseDiffWindow(
	writer http.ResponseWriter,
	req *http.Request,
) (diffWindow, bool) {
	var window diffWindow

	from, err := parseHistoryTimestamp(req.URL.Query().Get(paramFrom))
	if err != nil {
		writeJSONError(
			writer, http.StatusBadRequest, "Invalid or missing from timestamp",
		)

		return window, false
	}

	until, err := parseHistoryTimestamp(req.URL.Query().Get(paramTo))
	if err != nil {
		writeJSONError(
			writer, http.StatusBadRequest, "Invalid or missing to timestamp",
		)

		return window, false
	}

	if !from.Before(until) {
		writeJSONError(
			writer, http.StatusBadRequest, "from must be before to",
		)

		return window, false
	}

	window.from = from
	window.to = until

	return window, true
}

// parseHistoryTimestamp accepts RFC3339 ("2026-08-27T14:00:00Z") or unix
// milliseconds ("1787070000000").
func parseHistoryTimestamp(raw string) (time.Time, error) {
	var zero time.Time

	if raw == emptyString {
		return zero, fmt.Errorf("%w: empty", errBadTimestamp)
	}

	ts, err := time.Parse(time.RFC3339, raw)
	if err == nil {
		return ts, nil
	}

	millis, err := strconv.ParseInt(
		raw, historyTimestampBase, historyTimestampBitSize,
	)
	if err != nil {
		return zero, fmt.Errorf("%w: %q", errBadTimestamp, raw)
	}

	return time.UnixMilli(millis), nil
}
