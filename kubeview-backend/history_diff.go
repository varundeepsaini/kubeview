package main

// Diffing for the flight recorder: compares two recorded cluster states and
// renders per-object changes with human-readable summaries (image changes,
// replica scaling, restart bumps, condition transitions). The summaries work
// on the stored response DTOs generically — scalar fields compare directly,
// lists of named objects (containers, conditions) match by identity — so new
// DTO fields show up in diffs without new code here.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Diff change types. Added and modified reuse the recorder's change names;
// "removed" is distinct from the store's "deleted" tombstones because a diff
// only knows the object is gone between the two moments, not when.
const diffRemoved = "removed"

// summaryLimit caps the human-readable lines per change; the full before and
// after bodies ride along for anything the summary elides.
const summaryLimit = 12

// summaryFallback is the generic line for bodies that differ but cannot be
// summarized field by field. changeSummary must never return empty lines for
// unreadable bodies: an empty summary means "only volatile noise differs".
const summaryFallback = "object changed"

// historyChange describes one object's difference between two moments. JSON
// tags must match what the frontend expects in
// kubeview-frontend/src/lib/api.ts.
type historyChange struct {
	Resource string          `json:"resource"`
	Key      string          `json:"key"`
	Type     string          `json:"type"`
	Summary  []string        `json:"summary"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
}

// diffStates compares two recorded states and returns per-object changes
// ordered by kind, then key. Events are excluded: the diff response carries
// them separately as an activity feed.
func diffStates(before, after map[string][]historyObject) []historyChange {
	changes := []historyChange{}

	for _, kind := range historyKinds() {
		if kind == resourceEvents {
			continue
		}

		changes = append(changes, diffKind(kind, before[kind], after[kind])...)
	}

	return changes
}

func diffKind(kind string, before, after []historyObject) []historyChange {
	beforeByKey := objectsByKey(before)
	afterByKey := objectsByKey(after)
	keys := sortedKeyUnion(beforeByKey, afterByKey)
	changes := make([]historyChange, zeroCount, len(keys))

	for _, key := range keys {
		change, changed := diffObject(
			kind, key, beforeByKey[key], afterByKey[key],
		)
		if changed {
			changes = append(changes, change)
		}
	}

	return changes
}

func objectsByKey(objects []historyObject) map[string]*historyObject {
	byKey := make(map[string]*historyObject, len(objects))
	for i := range objects {
		byKey[objects[i].key] = &objects[i]
	}

	return byKey
}

// sortedKeyUnion returns the sorted union of both maps' keys.
func sortedKeyUnion[T any](before, after map[string]T) []string {
	seen := make(map[string]bool, len(before)+len(after))
	for name := range before {
		seen[name] = true
	}

	for name := range after {
		seen[name] = true
	}

	names := make([]string, zeroCount, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// diffObject classifies one object's difference; changed is false when the
// object is identical at both moments.
func diffObject(
	kind, key string,
	before, after *historyObject,
) (historyChange, bool) {
	change := historyChange{
		Resource: kind,
		Key:      key,
		Type:     emptyString,
		Summary:  []string{},
		Before:   nil,
		After:    nil,
	}

	switch {
	case before == nil && after == nil:
		return change, false
	case before == nil:
		change.Type = changeAdded
		change.After = after.object
	case after == nil:
		change.Type = diffRemoved
		change.Before = before.object
	case bytes.Equal(before.object, after.object):
		return change, false
	default:
		summary := changeSummary(before.object, after.object)
		if len(summary) == zeroCount {
			// Volatile fields are skipped, so an empty summary means only
			// age-like noise differs: not a real change.
			return change, false
		}

		change.Type = changeModified
		change.Before = before.object
		change.After = after.object
		change.Summary = summary
	}

	return change, true
}

// changeSummary renders the field-level differences between two versions of
// the same object as human-readable lines.
func changeSummary(before, after json.RawMessage) []string {
	var beforeFields, afterFields map[string]any

	if json.Unmarshal(before, &beforeFields) != nil ||
		json.Unmarshal(after, &afterFields) != nil {
		// Unreadable bodies still differ (the caller compared bytes first).
		return []string{summaryFallback}
	}

	lines := make([]string, zeroCount, summaryLimit)

	for _, field := range sortedKeyUnion(beforeFields, afterFields) {
		if isVolatileField(field) {
			continue
		}

		lines = append(
			lines, diffField(field, beforeFields[field], afterFields[field])...,
		)
		if len(lines) >= summaryLimit {
			return lines[:summaryLimit]
		}
	}

	return lines
}

// isVolatileField names fields that change with the viewing moment or move in
// lockstep with a status transition, and would only add noise to summaries.
func isVolatileField(name string) bool {
	return name == fieldAge || name == "lastTransition"
}

// diffField renders one field's difference. Lists get element-aware handling;
// scalars compare directly; everything else (maps, mixed shapes) falls back
// to a generic "changed" line.
func diffField(name string, before, after any) []string {
	if reflect.DeepEqual(before, after) {
		return nil
	}

	beforeList, beforeIsList := before.([]any)

	afterList, afterIsList := after.([]any)
	if beforeIsList && afterIsList {
		return diffListField(name, beforeList, afterList)
	}

	if isScalar(before) && isScalar(after) {
		return []string{fmt.Sprintf(
			"%s: %s → %s", name, formatScalar(before), formatScalar(after),
		)}
	}

	return []string{name + " changed"}
}

// diffListField diffs a JSON array: lists of objects keyed by "type"
// (conditions) or "name" (containers) match elements by identity, lists of
// scalars (images, ports) compare joined.
func diffListField(name string, before, after []any) []string {
	for _, idKey := range []string{"type", "name"} {
		lines, ok := namedListDiff(name, before, after, idKey)
		if ok {
			return lines
		}
	}

	lines, ok := scalarListDiff(name, before, after)
	if ok {
		return lines
	}

	return []string{name + " changed"}
}

// namedListDiff matches list elements by an identity field and reports
// per-element differences; ok is false when either list has elements without
// that identity, letting the caller try another strategy.
func namedListDiff(
	field string,
	before, after []any,
	idKey string,
) ([]string, bool) {
	beforeByID, namedBefore := elementsByID(before, idKey)
	if !namedBefore {
		return nil, false
	}

	afterByID, namedAfter := elementsByID(after, idKey)
	if !namedAfter {
		return nil, false
	}

	ids := sortedKeyUnion(beforeByID, afterByID)
	lines := make([]string, zeroCount, len(ids))

	for _, elementID := range ids {
		beforeElement, hadBefore := beforeByID[elementID]

		afterElement, hasAfter := afterByID[elementID]

		switch {
		case !hadBefore:
			lines = append(
				lines, fmt.Sprintf("%s[%s] added", field, elementID),
			)
		case !hasAfter:
			lines = append(
				lines, fmt.Sprintf("%s[%s] removed", field, elementID),
			)
		default:
			lines = append(lines, elementDiff(
				field, elementID, idKey, beforeElement, afterElement,
			)...)
		}
	}

	return lines, true
}

// elementsByID indexes object elements by their identity field; ok is false
// when any element is not an object or lacks the field.
func elementsByID(
	elements []any,
	idKey string,
) (map[string]map[string]any, bool) {
	byID := make(map[string]map[string]any, len(elements))

	for _, element := range elements {
		fields, isMap := element.(map[string]any)
		if !isMap {
			return nil, false
		}

		elementID, isString := fields[idKey].(string)
		if !isString {
			return nil, false
		}

		byID[elementID] = fields
	}

	return byID, true
}

// elementDiff reports scalar field changes within one matched list element,
// e.g. "containers[app].image: nginx:1.27 → nginx:1.28".
func elementDiff(
	field, elementID, idKey string,
	before, after map[string]any,
) []string {
	subs := sortedKeyUnion(before, after)
	lines := make([]string, zeroCount, len(subs))

	for _, sub := range subs {
		if sub == idKey || isVolatileField(sub) {
			continue
		}

		beforeValue, afterValue := before[sub], after[sub]
		if reflect.DeepEqual(beforeValue, afterValue) {
			continue
		}

		lines = append(lines, elementFieldLine(
			field, elementID, sub, beforeValue, afterValue,
		))
	}

	return lines
}

// elementFieldLine renders one changed field inside a matched list element.
func elementFieldLine(
	field, elementID, sub string,
	before, after any,
) string {
	if isScalar(before) && isScalar(after) {
		return fmt.Sprintf(
			"%s[%s].%s: %s → %s",
			field, elementID, sub, formatScalar(before), formatScalar(after),
		)
	}

	return fmt.Sprintf("%s[%s].%s changed", field, elementID, sub)
}

// scalarListDiff joins two all-scalar lists for a one-line comparison; ok is
// false when either list has non-scalar elements.
func scalarListDiff(name string, before, after []any) ([]string, bool) {
	beforeJoined, scalarBefore := joinScalars(before)
	if !scalarBefore {
		return nil, false
	}

	afterJoined, scalarAfter := joinScalars(after)
	if !scalarAfter {
		return nil, false
	}

	line := fmt.Sprintf("%s: %s → %s", name, beforeJoined, afterJoined)

	return []string{line}, true
}

func joinScalars(values []any) (string, bool) {
	if len(values) == zeroCount {
		return valueNoneBrackets, true
	}

	parts := make([]string, zeroCount, len(values))

	for _, value := range values {
		if !isScalar(value) {
			return emptyString, false
		}

		parts = append(parts, formatScalar(value))
	}

	return strings.Join(parts, ", "), true
}

// isScalar reports whether the decoded JSON value renders cleanly on one
// line: strings, numbers, booleans, and null.
func isScalar(value any) bool {
	switch value.(type) {
	case string, float64, bool, nil:
		return true
	default:
		return false
	}
}

func formatScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return valueNoneBrackets
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, historyTimestampBitSize)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
