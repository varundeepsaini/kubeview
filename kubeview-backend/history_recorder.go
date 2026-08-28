package main

// The flight recorder tails every supported resource kind through shared
// informers — one factory per kubeconfig context, started the first time a
// context is browsed — and funnels observed changes into the history store
// through a single writer goroutine. Informers re-list and re-watch on their
// own after connection loss, and the store's delta check makes redelivered
// state idempotent, so the pipeline needs no resourceVersion bookkeeping.

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	// recordQueueSize buffers observed changes between informer callbacks and
	// the store writer. Informer-callback enqueues never block (callbacks
	// must stay fast); a full queue drops the record, and the reconcile
	// passes heal the gap by re-recording every synced informer cache.
	recordQueueSize = 4096

	// recordBatchLimit caps how many queued records one transaction absorbs.
	// bbolt commits are fsync-bound, so batching keeps up with event bursts.
	recordBatchLimit = 256

	// pruneInterval paces retention sweeps.
	pruneInterval = 10 * time.Minute

	// noResync disables periodic informer resyncs: the recorder only needs
	// real changes, and the store already dedupes redelivered state.
	noResync time.Duration = 0

	// recorderLoopCount is the number of background loops Start launches:
	// the store writer and the maintenance sweeper (pruning + reconcile).
	recorderLoopCount = 2

	// reconcileLoopCount is the one goroutine startInformers launches per
	// context to reconcile the store against the synced informer caches.
	reconcileLoopCount = 1
)

// logHistoryError is the shared format for pipeline errors, which are logged
// rather than propagated: recording is best-effort by design.
const logHistoryError = "history: %v"

// contextInformers indexes one context's informers by resource kind.
type contextInformers map[string]cache.SharedIndexInformer

// RecorderManager owns the flight-recorder pipeline: per-context informer
// factories, the shared record queue, the store writer, and retention
// pruning. A nil manager is valid and records nothing (history disabled).
type RecorderManager struct {
	store     *HistoryStore
	queue     chan historyRecord
	stop      chan struct{}
	recording map[string]bool
	// informers registers each recording context's informer set so the
	// reconcile passes can compare informer caches against stored state;
	// guarded by mu.
	informers map[string]contextInformers
	// lastStampNanos is touched only by the writer goroutine; it keeps
	// stored timestamps strictly increasing in wall-clock nanoseconds — the
	// domain version keys persist — so same-nanosecond arrivals and backward
	// clock steps never break version-key ordering.
	lastStampNanos int64
	waitGroup      sync.WaitGroup
	mu             sync.Mutex
	retention      time.Duration
}

func NewRecorderManager(
	store *HistoryStore,
	retention time.Duration,
) *RecorderManager {
	manager := new(RecorderManager)
	manager.store = store
	manager.retention = retention
	manager.queue = make(chan historyRecord, recordQueueSize)
	manager.stop = make(chan struct{})
	manager.recording = make(map[string]bool)
	manager.informers = make(map[string]contextInformers)
	manager.lastStampNanos = storedLastStamp(store)

	return manager
}

// storedLastStamp floors the stamp guard to the newest stamp already in the
// store: after a backward wall-clock step and a restart, new writes must
// still sort after every existing version key.
func storedLastStamp(store *HistoryStore) int64 {
	nanos, err := store.LastStamp()
	if err != nil {
		log.Printf(logHistoryError, err)

		return zeroCount
	}

	return nanos
}

// Start launches the writer and retention loops.
func (m *RecorderManager) Start() {
	if m == nil {
		return
	}

	m.waitGroup.Add(recorderLoopCount)

	go m.runWriter()
	go m.runPruner()
}

// Stop ends recording: the informers stop, the queue drains, and both loops
// exit before Stop returns.
func (m *RecorderManager) Stop() {
	if m == nil {
		return
	}

	close(m.stop)
	m.waitGroup.Wait()
}

// EnsureRecording starts the informer set for the client's context on first
// use; later calls are cheap no-ops. Recording begins when a context is first
// browsed (the default context is ensured at startup), so a kubeconfig full
// of unreachable clusters never opens watches nobody asked for.
func (m *RecorderManager) EnsureRecording(client *Client) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.recording[client.context] {
		return
	}

	m.recording[client.context] = true
	m.startInformers(client)
}

func (m *RecorderManager) startInformers(client *Client) {
	factory := informers.NewSharedInformerFactory(
		client.streamClientset, noResync,
	)

	kindInformers := historyInformers(factory)
	for kind, informer := range kindInformers {
		err := m.addHandler(client.context, kind, informer)
		if err != nil {
			log.Printf(logHistoryError, err)
		}
	}

	// Registered under EnsureRecording's lock; the sweeper reconciles every
	// registered context periodically.
	m.informers[client.context] = kindInformers

	factory.Start(m.stop)

	m.waitGroup.Add(reconcileLoopCount)

	go m.reconcileWhenSynced(client.context, kindInformers)

	//nolint:gosec // G706: context names come from the kubeconfig, and
	// ClientFor rejects any ?context= value not defined there.
	log.Printf("history: recording context %q", client.context)
}

// historyInformers enumerates one informer per recorded kind; the keys match
// historyKinds().
func historyInformers(
	factory informers.SharedInformerFactory,
) contextInformers {
	return contextInformers{
		resourcePods:         factory.Core().V1().Pods().Informer(),
		resourceDeployments:  factory.Apps().V1().Deployments().Informer(),
		resourceServices:     factory.Core().V1().Services().Informer(),
		resourceNodes:        factory.Core().V1().Nodes().Informer(),
		resourceNamespaces:   factory.Core().V1().Namespaces().Informer(),
		resourceEvents:       factory.Core().V1().Events().Informer(),
		resourceConfigMaps:   factory.Core().V1().ConfigMaps().Informer(),
		resourceSecrets:      factory.Core().V1().Secrets().Informer(),
		resourceIngresses:    factory.Networking().V1().Ingresses().Informer(),
		resourceStatefulSets: factory.Apps().V1().StatefulSets().Informer(),
		resourceDaemonSets:   factory.Apps().V1().DaemonSets().Informer(),
	}
}

// addHandler subscribes the recorder to one informer's change stream.
func (m *RecorderManager) addHandler(
	contextName, kind string,
	informer cache.SharedIndexInformer,
) error {
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			m.enqueue(contextName, kind, changeAdded, obj)
		},
		UpdateFunc: func(_, newObj any) {
			m.enqueue(contextName, kind, changeModified, newObj)
		},
		DeleteFunc: func(obj any) {
			m.enqueue(contextName, kind, changeDeleted, obj)
		},
	}

	_, err := informer.AddEventHandler(handler)
	if err != nil {
		return fmt.Errorf("add %s history handler: %w", kind, err)
	}

	return nil
}

// enqueue reshapes the object and queues it for the writer.
func (m *RecorderManager) enqueue(
	contextName, kind, changeType string,
	obj any,
) {
	record, ok := buildRecord(contextName, kind, changeType, obj)
	if !ok {
		return
	}

	m.queueRecord(record)
}

// queueRecord hands one record to the writer without blocking: informer
// callbacks must never stall on the store. A full queue drops the record;
// the reconcile passes re-record the dropped state from the informer caches.
func (m *RecorderManager) queueRecord(record historyRecord) {
	select {
	case m.queue <- record:
	default:
		log.Printf(
			"history: queue full, dropped %s %s %s",
			record.changeType, record.resource, record.key,
		)
	}
}

// queueRecordBlocking hands one record to the writer, waiting for queue
// space. The reconcile passes run on background goroutines — never inside
// informer callbacks — so blocking here is safe, and it makes each completed
// pass a deterministic full heal instead of re-dropping records.
func (m *RecorderManager) queueRecordBlocking(record historyRecord) {
	select {
	case m.queue <- record:
	case <-m.stop:
	}
}

// reconcileWhenSynced waits for one context's informers to finish their
// initial list, then reconciles the store against them: a deletion that
// happened while the recorder was down is only visible as an absence, and a
// record a full queue dropped is only recoverable from the caches.
func (m *RecorderManager) reconcileWhenSynced(
	contextName string,
	kindInformers contextInformers,
) {
	defer m.waitGroup.Done()

	synced := make([]cache.InformerSynced, zeroCount, len(kindInformers))
	for _, informer := range kindInformers {
		synced = append(synced, informer.HasSynced)
	}

	if !cache.WaitForCacheSync(m.stop, synced...) {
		return
	}

	m.reconcileContext(contextName, kindInformers)
}

// reconcileContext heals the store against one context's synced informer
// caches: every cached object is re-recorded — the store's delta check makes
// unchanged bodies no-ops, so adds and updates dropped by a full queue
// reappear — and every stored-live object the caches no longer contain is
// tombstoned.
func (m *RecorderManager) reconcileContext(
	contextName string,
	kindInformers contextInformers,
) {
	// The far-future moment reads each object's newest version regardless
	// of any clock skew in past stamps.
	moment := time.Unix(zeroCount, maxUnixNano)

	state, err := m.store.StateAt(contextName, moment)
	if err != nil {
		log.Printf(logHistoryError, err)

		return
	}

	for kind, informer := range kindInformers {
		m.reconcileKind(contextName, kind, state[kind], informer)
	}
}

// reconcileKind heals one kind: it re-records the informer cache's objects
// and tombstones the stored-live objects the cache no longer lists.
func (m *RecorderManager) reconcileKind(
	contextName, kind string,
	stored []historyObject,
	informer cache.SharedIndexInformer,
) {
	live := m.recordLiveObjects(contextName, kind, informer)

	for _, object := range stored {
		if !live[object.key] {
			m.queueRecordBlocking(
				tombstoneRecord(contextName, kind, object.key),
			)
		}
	}
}

// recordLiveObjects re-enqueues every object in one informer cache — healing
// adds and updates a full queue dropped; unchanged bodies are store-level
// no-ops — and returns the cache's keys in the same shape buildRecord writes:
// namespace + "/" + name, keeping the leading slash for cluster-scoped
// objects (unlike cache.MetaNamespaceKeyFunc).
func (m *RecorderManager) recordLiveObjects(
	contextName, kind string,
	informer cache.SharedIndexInformer,
) map[string]bool {
	objects := informer.GetStore().List()
	keys := make(map[string]bool, len(objects))

	for _, obj := range objects {
		record, ok := buildRecord(contextName, kind, changeModified, obj)
		if !ok {
			continue
		}

		keys[record.key] = true

		m.queueRecordBlocking(record)
	}

	return keys
}

// tombstoneRecord builds a synthetic deletion for an object the informer no
// longer sees.
func tombstoneRecord(contextName, kind, key string) historyRecord {
	var record historyRecord

	record.context = contextName
	record.resource = kind
	record.key = key
	record.changeType = changeDeleted

	return record
}

// buildRecord turns an informer callback payload into a pending store write.
func buildRecord(
	contextName, kind, changeType string,
	obj any,
) (historyRecord, bool) {
	var record historyRecord

	// Deletions arrive wrapped when the informer missed the delete event and
	// only noticed the absence on a re-list.
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = unknown.Obj
	}

	meta, isObject := obj.(metav1.Object)
	if !isObject {
		return record, false
	}

	record.context = contextName
	record.resource = kind
	record.key = meta.GetNamespace() + "/" + meta.GetName()
	record.changeType = changeType
	record.createdAt = formatTime(meta.GetCreationTimestamp())

	if changeType == changeDeleted {
		return record, true
	}

	dto, ok := transformHistoryObject(obj)
	if !ok {
		return record, false
	}

	raw, err := json.Marshal(dto)
	if err != nil {
		log.Printf("history: encode %s %s: %v", kind, record.key, err)

		return record, false
	}

	record.object = raw

	return record, true
}

// transformHistoryObject reshapes a watched object into the same response DTO
// the live endpoints serve, so history state renders identically in the UI.
func transformHistoryObject(obj any) (any, bool) {
	if dto, ok := transformCoreHistoryObject(obj); ok {
		return dto, true
	}

	return transformWorkloadHistoryObject(obj)
}

func transformCoreHistoryObject(obj any) (any, bool) {
	switch object := obj.(type) {
	case *corev1.Pod:
		return transformPod(object), true
	case *corev1.Service:
		return transformService(*object), true
	case *corev1.Node:
		return transformNode(*object), true
	case *corev1.Namespace:
		return transformNamespace(*object), true
	case *corev1.Event:
		return transformEvent(*object), true
	case *corev1.ConfigMap:
		return transformConfigMap(*object), true
	case *corev1.Secret:
		return transformSecret(*object), true
	default:
		return nil, false
	}
}

func transformWorkloadHistoryObject(obj any) (any, bool) {
	switch object := obj.(type) {
	case *appsv1.Deployment:
		return transformDeployment(*object), true
	case *appsv1.StatefulSet:
		return transformStatefulSet(*object), true
	case *appsv1.DaemonSet:
		return transformDaemonSet(*object), true
	case *networkingv1.Ingress:
		return transformIngress(*object), true
	default:
		return nil, false
	}
}

func (m *RecorderManager) runWriter() {
	defer m.waitGroup.Done()

	for {
		select {
		case record := <-m.queue:
			m.writeBatch(record)
		case <-m.stop:
			m.drainQueue()

			return
		}
	}
}

// writeBatch groups the first record with everything else already queued into
// one transaction.
func (m *RecorderManager) writeBatch(first historyRecord) {
	batch := m.collectBatch(first)

	err := m.store.RecordBatch(batch)
	if err != nil {
		log.Printf(logHistoryError, err)
	}
}

func (m *RecorderManager) collectBatch(first historyRecord) []historyRecord {
	batch := make([]historyRecord, zeroCount, recordBatchLimit)
	batch = append(batch, m.stamp(first))

	for len(batch) < recordBatchLimit {
		select {
		case record := <-m.queue:
			batch = append(batch, m.stamp(record))
		default:
			return batch
		}
	}

	return batch
}

// stamp assigns the record's stored timestamp, kept strictly increasing in
// wall-clock nanoseconds — the domain version keys persist. Comparing
// time.Time values directly would use the monotonic clock, letting a
// backward wall step write keys that sort below already-stored versions.
func (m *RecorderManager) stamp(record historyRecord) historyRecord {
	stampNano := time.Now().UnixNano()
	if stampNano <= m.lastStampNanos {
		stampNano = m.lastStampNanos + int64(time.Nanosecond)
	}

	m.lastStampNanos = stampNano
	record.ts = time.Unix(zeroCount, stampNano)

	return record
}

// drainQueue flushes whatever is still queued at shutdown.
func (m *RecorderManager) drainQueue() {
	for {
		select {
		case record := <-m.queue:
			m.writeBatch(record)
		default:
			return
		}
	}
}

func (m *RecorderManager) runPruner() {
	defer m.waitGroup.Done()

	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.sweep()
		}
	}
}

// sweep is one maintenance pass: retention pruning plus a reconcile of every
// recording context, healing records dropped by a full queue.
func (m *RecorderManager) sweep() {
	err := m.store.Prune(time.Now().Add(-m.retention))
	if err != nil {
		log.Printf(logHistoryError, err)
	}

	for contextName, kindInformers := range m.snapshotInformers() {
		if informersSynced(kindInformers) {
			m.reconcileContext(contextName, kindInformers)
		}
	}
}

// snapshotInformers copies the informer registry out from under the lock.
func (m *RecorderManager) snapshotInformers() map[string]contextInformers {
	m.mu.Lock()
	defer m.mu.Unlock()

	return maps.Clone(m.informers)
}

// informersSynced reports whether every informer of one context has synced;
// reconciling against a cache still listing would tombstone everything.
func informersSynced(kindInformers contextInformers) bool {
	for _, informer := range kindInformers {
		if !informer.HasSynced() {
			return false
		}
	}

	return true
}
