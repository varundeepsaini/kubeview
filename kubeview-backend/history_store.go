package main

// The cluster flight recorder persists resource history in an embedded bbolt
// file: one top-level bucket per kubeconfig context, one nested bucket per
// resource kind, plus a per-context metadata bucket. Version keys combine the
// object key ("<namespace>/<name>") with the record time, so a cursor scan
// yields versions grouped by object in time order — which is exactly the
// access pattern of "cluster state as of a moment" queries.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Change types recorded for each version. Deletions are tombstones: they mark
// the object as gone without carrying a body.
const (
	changeAdded    = "added"
	changeModified = "modified"
	changeDeleted  = "deleted"
)

// fieldAge is the render-time age field most response DTOs carry. It ticks
// with the viewing moment rather than with real changes, so the store ignores
// it when deduping versions, the diff engine skips it in summaries, and the
// state endpoint rewrites it per viewed moment.
const fieldAge = "age"

const (
	// historyFileMode keeps the database file owner-only; recorded history
	// carries the same cluster metadata the API serves.
	historyFileMode = 0o600

	// historyLockTimeout bounds how long opening the store waits for its
	// file lock. bbolt retries the lock forever by default, so a second
	// kubeview process sharing the store path would hang inside setupHistory
	// instead of starting with history disabled.
	historyLockTimeout = time.Second

	// historyKeySeparator splits the object key from the version timestamp in
	// bucket keys. Object keys are DNS-1123 names joined by a slash, which can
	// never contain a NUL byte.
	historyKeySeparator = byte(0)

	// timestampKeyBytes is the width of the big-endian nanosecond timestamp
	// suffix on version keys; fixed width keeps keys ordered by time.
	timestampKeyBytes = 8
)

// metaBucketName holds per-context metadata; the "__" prefix cannot collide
// with a resource kind name. metaKeySince records when recording began;
// metaKeyLastStamp records the newest write stamp, the floor for the
// recorder's clock guard across restarts.
const (
	metaBucketName   = "__meta"
	metaKeySince     = "since"
	metaKeyLastStamp = "lastStamp"
)

// Resource kind names, shared by the live watch endpoint, the flight
// recorder, and the history API. The names match the live endpoint paths and
// the ?resources= values of /api/watch. Events are special: the diff endpoint
// reports them as an activity feed rather than as state changes.
const (
	resourcePods         = "pods"
	resourceDeployments  = "deployments"
	resourceServices     = "services"
	resourceNodes        = "nodes"
	resourceNamespaces   = "namespaces"
	resourceEvents       = "events"
	resourceConfigMaps   = "configmaps"
	resourceSecrets      = "secrets"
	resourceIngresses    = "ingresses"
	resourceStatefulSets = "statefulsets"
	resourceDaemonSets   = "daemonsets"
)

// historyKinds returns the resource kinds the recorder captures, in the order
// they appear in history state responses.
func historyKinds() []string {
	return []string{
		resourcePods, resourceDeployments, resourceServices, resourceNodes,
		resourceNamespaces, resourceEvents, resourceConfigMaps,
		resourceSecrets, resourceIngresses, resourceStatefulSets,
		resourceDaemonSets,
	}
}

// HistoryStore is the embedded store the flight recorder writes to and the
// history API reads from. bbolt serializes writers internally, so the store
// is safe for concurrent use.
type HistoryStore struct {
	db *bolt.DB
}

// historyRecord is one pending write: a version of an object observed by the
// recorder at a point in time. object is nil for deletions.
type historyRecord struct {
	context    string
	resource   string
	key        string
	changeType string
	createdAt  string
	ts         time.Time
	object     json.RawMessage
}

// historyVersion is the persisted form of one recorded version. CreatedAt
// carries the object's creation timestamp so read paths can recompute ages
// relative to the viewed moment — several response shapes only expose a
// pre-rendered age string.
type historyVersion struct {
	Type      string          `json:"type"`
	CreatedAt string          `json:"createdAt,omitempty"`
	Object    json.RawMessage `json:"object,omitempty"`
}

// historyObject is one resource read back from the store as of a moment.
type historyObject struct {
	key       string
	createdAt string
	object    json.RawMessage
}

// OpenHistoryStore opens (creating if needed) the bbolt file at path.
func OpenHistoryStore(path string) (*HistoryStore, error) {
	options := new(bolt.Options)
	options.Timeout = historyLockTimeout

	db, err := bolt.Open(path, historyFileMode, options)
	if err != nil {
		return nil, fmt.Errorf("open history store: %w", err)
	}

	return &HistoryStore{db: db}, nil
}

func (s *HistoryStore) Close() error {
	err := s.db.Close()
	if err != nil {
		return fmt.Errorf("close history store: %w", err)
	}

	return nil
}

// separatorBytes is the width of the NUL separator inside version keys.
const separatorBytes = 1

// versionKey builds the bucket key for one version: the object key, a NUL
// separator, and the record time as fixed-width big-endian nanoseconds.
func versionKey(objectKey string, stamp time.Time) []byte {
	size := len(objectKey) + separatorBytes + timestampKeyBytes
	key := make([]byte, zeroCount, size)
	key = append(key, objectKey...)
	key = append(key, historyKeySeparator)

	return binary.BigEndian.AppendUint64(key, uint64(stamp.UnixNano()))
}

// splitVersionKey parses a bucket key back into object key and record time.
func splitVersionKey(key []byte) (string, time.Time, bool) {
	sep := len(key) - timestampKeyBytes - separatorBytes
	if sep < zeroCount || key[sep] != historyKeySeparator {
		var zero time.Time

		return emptyString, zero, false
	}

	nanos := binary.BigEndian.Uint64(key[sep+separatorBytes:])
	//nolint:gosec // G115: the value was written as int64 nanoseconds.
	ts := time.Unix(zeroCount, int64(nanos))

	return string(key[:sep]), ts, true
}

// RecordBatch persists a batch of records in one transaction. A record that
// matches its object's latest stored version is skipped, keeping storage
// delta-only and making informer re-lists (including the initial sync after
// a restart) idempotent.
func (s *HistoryStore) RecordBatch(records []historyRecord) error {
	if len(records) == zeroCount {
		return nil
	}

	err := s.db.Update(func(txn *bolt.Tx) error {
		for _, record := range records {
			err := writeRecord(txn, record)
			if err != nil {
				return err
			}
		}

		return writeLastStamps(txn, records)
	})
	if err != nil {
		return fmt.Errorf("record history batch: %w", err)
	}

	return nil
}

// writeLastStamps persists each context's newest stamp in the batch;
// storedLastStamp floors the recorder's clock guard to it on startup, so a
// restart after a backward wall-clock step cannot write below existing keys.
func writeLastStamps(txn *bolt.Tx, records []historyRecord) error {
	newest := make(map[string]time.Time, len(records))
	for _, record := range records {
		if record.ts.After(newest[record.context]) {
			newest[record.context] = record.ts
		}
	}

	for contextName, stamp := range newest {
		err := writeContextLastStamp(txn, contextName, stamp)
		if err != nil {
			return err
		}
	}

	return nil
}

// writeContextLastStamp advances one context's persisted newest stamp; it
// never moves backwards.
func writeContextLastStamp(
	tx *bolt.Tx,
	contextName string,
	stamp time.Time,
) error {
	contextBucket, err := tx.CreateBucketIfNotExists([]byte(contextName))
	if err != nil {
		return fmt.Errorf("create context bucket: %w", err)
	}

	meta, err := contextBucket.CreateBucketIfNotExists([]byte(metaBucketName))
	if err != nil {
		return fmt.Errorf("create meta bucket: %w", err)
	}

	if storedStampNanos(meta) >= stamp.UnixNano() {
		return nil
	}

	nanos := binary.BigEndian.AppendUint64(nil, uint64(stamp.UnixNano()))

	err = meta.Put([]byte(metaKeyLastStamp), nanos)
	if err != nil {
		return fmt.Errorf("write last-stamp: %w", err)
	}

	return nil
}

// storedStampNanos reads a meta bucket's persisted newest stamp, or zero.
func storedStampNanos(meta *bolt.Bucket) int64 {
	raw := meta.Get([]byte(metaKeyLastStamp))
	if len(raw) != timestampKeyBytes {
		return zeroCount
	}

	//nolint:gosec // G115: the value was written as int64 nanoseconds.
	return int64(binary.BigEndian.Uint64(raw))
}

func writeRecord(tx *bolt.Tx, record historyRecord) error {
	contextBucket, err := tx.CreateBucketIfNotExists([]byte(record.context))
	if err != nil {
		return fmt.Errorf("create context bucket: %w", err)
	}

	err = ensureSince(contextBucket, record.ts)
	if err != nil {
		return err
	}

	kindBucket, err := contextBucket.CreateBucketIfNotExists(
		[]byte(record.resource),
	)
	if err != nil {
		return fmt.Errorf("create kind bucket: %w", err)
	}

	if !versionChanged(kindBucket, record) {
		return nil
	}

	value, err := json.Marshal(historyVersion{
		Type:      record.changeType,
		CreatedAt: record.createdAt,
		Object:    record.object,
	})
	if err != nil {
		return fmt.Errorf("encode history version: %w", err)
	}

	err = kindBucket.Put(versionKey(record.key, record.ts), value)
	if err != nil {
		return fmt.Errorf("write history version: %w", err)
	}

	return nil
}

// ensureSince stamps the context's recording start time on its first write.
func ensureSince(contextBucket *bolt.Bucket, firstSeen time.Time) error {
	meta, err := contextBucket.CreateBucketIfNotExists([]byte(metaBucketName))
	if err != nil {
		return fmt.Errorf("create meta bucket: %w", err)
	}

	if meta.Get([]byte(metaKeySince)) != nil {
		return nil
	}

	nanos := binary.BigEndian.AppendUint64(
		nil, uint64(firstSeen.UnixNano()),
	)

	err = meta.Put([]byte(metaKeySince), nanos)
	if err != nil {
		return fmt.Errorf("write recording-since: %w", err)
	}

	return nil
}

// versionChanged reports whether the record differs from the object's latest
// stored version: a changed body, a deletion of a live object, or a
// resurrection after a tombstone. Duplicate bodies, repeated tombstones, and
// tombstones for never-recorded objects are no-ops.
func versionChanged(kindBucket *bolt.Bucket, record historyRecord) bool {
	value, found := latestVersion(kindBucket, record.key)
	if !found {
		return record.changeType != changeDeleted
	}

	stored, err := decodeVersion(value)
	if err != nil {
		// An unreadable latest version should never happen; recover by
		// recording fresh state over it.
		return true
	}

	if record.changeType == changeDeleted {
		return stored.Type != changeDeleted
	}

	if stored.Type == changeDeleted {
		return true
	}

	if bytes.Equal(stored.Object, record.object) {
		return false
	}

	// The bodies differ, but a difference confined to the render-time age
	// field is not a real change: it ticks with the recording moment, so an
	// informer re-list after a restart would version every object.
	return !onlyAgeDiffers(stored.Object, record.object)
}

// onlyAgeDiffers reports whether two object bodies differ solely in the
// render-time age field. Age stays in the stored body — the read path
// rewrites it per viewed moment — but it must not count as a real change.
func onlyAgeDiffers(stored, incoming json.RawMessage) bool {
	var storedFields, incomingFields map[string]any

	if json.Unmarshal(stored, &storedFields) != nil ||
		json.Unmarshal(incoming, &incomingFields) != nil {
		return false
	}

	delete(storedFields, fieldAge)
	delete(incomingFields, fieldAge)

	return reflect.DeepEqual(storedFields, incomingFields)
}

// latestVersion returns the stored value of the object's newest version.
func latestVersion(kindBucket *bolt.Bucket, objectKey string) ([]byte, bool) {
	cursor := kindBucket.Cursor()

	// Seek to just past the last possible version of this object, then step
	// back one entry: keys sort by object key, then timestamp. A real key can
	// never equal the seek key (its timestamp would have to be MaxInt64).
	seek := versionKey(objectKey, time.Unix(zeroCount, maxUnixNano))
	seeked, _ := cursor.Seek(seek)

	var key, value []byte
	if seeked == nil {
		key, value = cursor.Last()
	} else {
		key, value = cursor.Prev()
	}

	prefix := append([]byte(objectKey), historyKeySeparator)
	if key == nil || !bytes.HasPrefix(key, prefix) {
		return nil, false
	}

	return value, true
}

// maxUnixNano is the largest representable nanosecond timestamp, used as the
// upper bound when seeking an object's newest version.
const maxUnixNano = int64(^uint64(0) >> 1)

func decodeVersion(value []byte) (historyVersion, error) {
	var version historyVersion

	err := json.Unmarshal(value, &version)
	if err != nil {
		return version, fmt.Errorf("decode history version: %w", err)
	}

	return version, nil
}

// StateAt returns, for each recorded kind, the objects that existed at the
// given moment: each object's newest version at or before the timestamp,
// excluding tombstones. An unrecorded context yields empty slices.
func (s *HistoryStore) StateAt(
	contextName string,
	moment time.Time,
) (map[string][]historyObject, error) {
	state := make(map[string][]historyObject, len(historyKinds()))

	err := s.db.View(func(tx *bolt.Tx) error {
		for _, kind := range historyKinds() {
			bucket := historyKindBucket(tx, contextName, kind)
			state[kind] = kindStateAt(bucket, moment)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read history state: %w", err)
	}

	return state, nil
}

// historyKindBucket returns the bucket holding one kind of one context, or
// nil when either level does not exist yet.
func historyKindBucket(
	tx *bolt.Tx,
	contextName, kind string,
) *bolt.Bucket {
	contextBucket := tx.Bucket([]byte(contextName))
	if contextBucket == nil {
		return nil
	}

	return contextBucket.Bucket([]byte(kind))
}

// kindStateAt scans one kind bucket, tracking the newest version at or before
// the cut for each object. Versions are grouped by object key and time-ordered
// in the bucket, so a single pass suffices.
func kindStateAt(kindBucket *bolt.Bucket, moment time.Time) []historyObject {
	scan := new(stateScan)
	scan.at = moment
	scan.objects = []historyObject{}

	if kindBucket == nil {
		return scan.objects
	}

	cursor := kindBucket.Cursor()

	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		scan.observe(key, value)
	}

	scan.flush()

	return scan.objects
}

// stateScan accumulates a single ordered pass over one kind bucket: for each
// object it tracks the newest version at or before the cut, emitting live
// objects as the scan crosses object-key boundaries.
type stateScan struct {
	at      time.Time
	key     string
	objects []historyObject
	current historyObject
	have    bool
}

// observe folds one bucket entry into the scan.
func (s *stateScan) observe(key, value []byte) {
	objectKey, recordedAt, ok := splitVersionKey(key)
	if !ok {
		return
	}

	if objectKey != s.key {
		s.flush()
		s.key = objectKey
	}

	if recordedAt.After(s.at) {
		return
	}

	version, err := decodeVersion(value)
	if err != nil {
		return
	}

	s.apply(objectKey, version)
}

// apply makes the version the object's current candidate; a tombstone clears
// any earlier candidate — the object was gone as of this version.
func (s *stateScan) apply(objectKey string, version historyVersion) {
	if version.Type == changeDeleted {
		s.have = false

		return
	}

	s.current = historyObject{
		key:       objectKey,
		createdAt: version.CreatedAt,
		object:    version.Object,
	}
	s.have = true
}

// flush emits the current object's surviving version, if any.
func (s *stateScan) flush() {
	if s.have {
		s.objects = append(s.objects, s.current)
	}

	s.have = false
}

// recordedHistoryEvent pairs an event body with when it was recorded.
type recordedHistoryEvent struct {
	ts     time.Time
	object json.RawMessage
}

// EventsBetween returns event objects recorded in the window (from, to] —
// the newest recorded version of each event — ordered by record time. This
// backs the diff view's "what happened in between" feed.
func (s *HistoryStore) EventsBetween(
	contextName string,
	from, until time.Time,
) ([]json.RawMessage, error) {
	var latest map[string]recordedHistoryEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := historyKindBucket(tx, contextName, resourceEvents)
		latest = collectWindowEvents(bucket, from, until)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read history events: %w", err)
	}

	return orderedEventBodies(latest), nil
}

// collectWindowEvents gathers the newest version of each event recorded in
// the window (from, to].
func collectWindowEvents(
	kindBucket *bolt.Bucket,
	from, until time.Time,
) map[string]recordedHistoryEvent {
	latest := make(map[string]recordedHistoryEvent)
	if kindBucket == nil {
		return latest
	}

	cursor := kindBucket.Cursor()

	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		upsertWindowEvent(latest, key, value, from, until)
	}

	return latest
}

// upsertWindowEvent folds one bucket entry into the newest-per-event map when
// it falls inside the window.
func upsertWindowEvent(
	latest map[string]recordedHistoryEvent,
	key, value []byte,
	from, until time.Time,
) {
	objectKey, recordedAt, ok := splitVersionKey(key)
	if !ok || !recordedAt.After(from) || recordedAt.After(until) {
		return
	}

	version, err := decodeVersion(value)
	if err != nil || version.Type == changeDeleted {
		return
	}

	latest[objectKey] = recordedHistoryEvent{
		ts:     recordedAt,
		object: version.Object,
	}
}

// orderedEventBodies flattens the newest-per-event map into record order.
func orderedEventBodies(
	latest map[string]recordedHistoryEvent,
) []json.RawMessage {
	ordered := make([]recordedHistoryEvent, zeroCount, len(latest))
	for _, event := range latest {
		ordered = append(ordered, event)
	}

	slices.SortFunc(ordered, func(a, b recordedHistoryEvent) int {
		return a.ts.Compare(b.ts)
	})

	out := make([]json.RawMessage, zeroCount, len(ordered))
	for _, event := range ordered {
		out = append(out, event.object)
	}

	return out
}

// RecordingSince returns when recording began for the context; ok is false
// when the context has no history yet.
func (s *HistoryStore) RecordingSince(
	contextName string,
) (time.Time, bool, error) {
	var (
		since time.Time
		found bool
	)

	err := s.db.View(func(tx *bolt.Tx) error {
		contextBucket := tx.Bucket([]byte(contextName))
		if contextBucket == nil {
			return nil
		}

		meta := contextBucket.Bucket([]byte(metaBucketName))
		if meta == nil {
			return nil
		}

		raw := meta.Get([]byte(metaKeySince))
		if len(raw) != timestampKeyBytes {
			return nil
		}

		nanos := binary.BigEndian.Uint64(raw)
		//nolint:gosec // G115: the value was written as int64 nanoseconds.
		since = time.Unix(zeroCount, int64(nanos))
		found = true

		return nil
	})
	if err != nil {
		return since, false, fmt.Errorf("read recording-since: %w", err)
	}

	return since, found, nil
}

// LastStamp returns the newest write stamp any context has persisted, in
// unix nanoseconds, or zero when nothing has been recorded yet. The recorder
// floors its clock guard to it on startup.
func (s *HistoryStore) LastStamp() (int64, error) {
	var newest int64

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(_ []byte, contextBucket *bolt.Bucket) error {
			nanos := contextLastStamp(contextBucket)
			if nanos > newest {
				newest = nanos
			}

			return nil
		})
	})
	if err != nil {
		return zeroCount, fmt.Errorf("read last stamp: %w", err)
	}

	return newest, nil
}

// contextLastStamp reads one context's persisted newest stamp, or zero.
func contextLastStamp(contextBucket *bolt.Bucket) int64 {
	meta := contextBucket.Bucket([]byte(metaBucketName))
	if meta == nil {
		return zeroCount
	}

	return storedStampNanos(meta)
}

// Prune removes versions recorded before the cutoff while keeping each live
// object's newest pre-cutoff version — the baseline that state queries at the
// start of the retention window still resolve against. A pre-cutoff tombstone
// is always removable: an object with no versions is equally "not present".
func (s *HistoryStore) Prune(cutoff time.Time) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.ForEach(func(_ []byte, contextBucket *bolt.Bucket) error {
			return pruneContextBucket(contextBucket, cutoff)
		})
	})
	if err != nil {
		return fmt.Errorf("prune history: %w", err)
	}

	return nil
}

func pruneContextBucket(contextBucket *bolt.Bucket, cutoff time.Time) error {
	for _, kind := range historyKinds() {
		kindBucket := contextBucket.Bucket([]byte(kind))
		if kindBucket == nil {
			continue
		}

		err := pruneKindBucket(kindBucket, cutoff)
		if err != nil {
			return err
		}
	}

	return nil
}

// pruneKindBucket walks one kind bucket in order, marking each pre-cutoff
// version as doomed once a newer pre-cutoff version of the same object
// supersedes it, then deletes the doomed keys.
func pruneKindBucket(kindBucket *bolt.Bucket, cutoff time.Time) error {
	doomed := collectPrunableKeys(kindBucket, cutoff)
	for _, key := range doomed {
		err := kindBucket.Delete(key)
		if err != nil {
			return fmt.Errorf("delete pruned version: %w", err)
		}
	}

	return nil
}

func collectPrunableKeys(kindBucket *bolt.Bucket, cutoff time.Time) [][]byte {
	scan := new(pruneScan)
	scan.cutoff = cutoff
	scan.doomed = [][]byte{}

	cursor := kindBucket.Cursor()

	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		scan.observe(key, value)
	}

	return scan.doomed
}

// pruneScan accumulates one ordered pass over a kind bucket, collecting the
// version keys the retention cutoff makes redundant.
type pruneScan struct {
	cutoff time.Time
	key    string
	doomed [][]byte
	// baseline is the current object's newest pre-cutoff version seen so far;
	// it survives unless a newer pre-cutoff version supersedes it.
	baseline []byte
}

// observe folds one bucket entry into the scan.
func (p *pruneScan) observe(key, value []byte) {
	objectKey, recordedAt, ok := splitVersionKey(key)
	if !ok {
		return
	}

	if objectKey != p.key {
		p.key = objectKey
		p.baseline = nil
	}

	if !recordedAt.Before(p.cutoff) {
		return
	}

	p.retire(key, value)
}

// retire dooms the previously kept pre-cutoff baseline — this newer version
// supersedes it — and then either dooms a tombstone outright or keeps this
// version as the new baseline.
func (p *pruneScan) retire(key, value []byte) {
	if p.baseline != nil {
		p.doomed = append(p.doomed, p.baseline)
		p.baseline = nil
	}

	if isTombstone(value) {
		p.doomed = append(p.doomed, bytes.Clone(key))

		return
	}

	p.baseline = bytes.Clone(key)
}

func isTombstone(value []byte) bool {
	version, err := decodeVersion(value)

	return err == nil && version.Type == changeDeleted
}
