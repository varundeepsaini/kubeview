// Build-time override for non-local deployments; falls back to the dev
// backend. NEXT_PUBLIC_ vars are inlined by Next.js at build time.
export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:5501/api";

// currentContext is the ambient kubeconfig context appended to every request as
// ?context=. Empty means "let the backend use its default (current) context".
// ClusterProvider keeps this in sync with the user's selection so callers don't
// have to thread the context through every api.getX signature.
let currentContext = "";

export function setApiContext(name: string): void {
  currentContext = name;
}

// currentAt is the ambient time-travel moment (unix ms), or null for live.
// When set, resource getters resolve from the flight recorder's
// /history/state snapshot instead of the live endpoints, so every page
// renders the cluster as of that moment without changing its own code.
// TimeTravelProvider keeps this in sync with the timeline scrubber.
let currentAt: number | null = null;

export function setApiAt(at: number | null): void {
  currentAt = at;
}

export function getApiAt(): number | null {
  return currentAt;
}

// withContext appends the active context to a path, respecting any query string
// the path already carries. Kept as string manipulation (not new URL) so a
// relative NEXT_PUBLIC_API_BASE still works.
function withContext(path: string): string {
  if (!currentContext) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}context=${encodeURIComponent(currentContext)}`;
}

async function fetchApi<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${withContext(path)}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

export interface ContextInfo {
  name: string;
  cluster: string;
  current: boolean;
}

export interface ClusterInfo {
  version: string;
  platform: string;
  nodeCount: number;
  context: string;
  clusterName: string;
}

export interface Namespace {
  name: string;
  status: string;
  labels: Record<string, string>;
  createdAt: string;
  age: string;
}

export interface Container {
  name: string;
  image: string;
  kind: "container" | "init" | "sidecar" | "ephemeral";
  ports: string[];
  ready: boolean;
  state: string;
  restartCount: number;
}

export interface PodCondition {
  type: string;
  status: string;
  reason?: string;
  lastTransition?: string;
}

export interface Pod {
  name: string;
  namespace: string;
  status: string;
  ready: string;
  restarts: number;
  node: string;
  ip: string;
  labels: Record<string, string>;
  createdAt: string;
  age: string;
  containers: Container[];
  conditions: PodCondition[];
  volumes: { name: string; type: string }[];
  defaultContainer: string;
}

export interface DeploymentCondition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransition?: string;
}

export interface Deployment {
  name: string;
  namespace: string;
  replicas: number;
  readyReplicas: number;
  desiredReplicas: number;
  updatedReplicas: number;
  availableReplicas: number;
  strategy: string;
  labels: Record<string, string>;
  selector: Record<string, string>;
  createdAt: string;
  age: string;
  conditions: DeploymentCondition[];
  images: string[];
}

export interface Service {
  name: string;
  namespace: string;
  type: string;
  clusterIp: string;
  externalIp: string;
  ports: string[];
  selector: Record<string, string>;
  labels: Record<string, string>;
  createdAt: string;
  age: string;
}

export interface NodeInfo {
  name: string;
  status: string;
  roles: string[];
  version: string;
  os: string;
  arch: string;
  containerRuntime: string;
  cpu: string;
  memory: string;
  pods: string;
  labels: Record<string, string>;
  conditions: { type: string; status: string; reason?: string; message?: string }[];
  createdAt: string;
  age: string;
  addresses: { type: string; address: string }[];
}

export interface KubeEvent {
  name: string;
  type: string;
  reason: string;
  message: string;
  object: string;
  namespace: string;
  firstSeen: string;
  lastSeen: string;
  count: number;
  source: string;
}

export interface ConfigMap { name: string; namespace: string; keys: string[]; labels: Record<string,string>; age: string }
export interface Secret { name: string; namespace: string; type: string; dataLengths: Record<string,number>; age: string }
export interface IngressRule { host: string; path: string; service: string; port: string }
export interface Ingress { name: string; namespace: string; class: string; rules: IngressRule[]; addresses: string[]; age: string }
export interface StatefulSet { name: string; namespace: string; serviceName: string; replicas: number; desiredReplicas: number; readyReplicas: number; currentReplicas: number; updatedReplicas: number; strategy: string; age: string }
export interface DaemonSet { name: string; namespace: string; desired: number; current: number; ready: number; updated: number; available: number; age: string }

// --- flight recorder (time travel) ---

export interface HistoryRange {
  enabled: boolean;
  // start/end are RFC3339; start is absent until anything has been recorded.
  start?: string;
  end?: string;
  retentionHours?: number;
}

export interface HistoryResources {
  pods: Pod[];
  deployments: Deployment[];
  services: Service[];
  nodes: NodeInfo[];
  namespaces: Namespace[];
  events: KubeEvent[];
  configmaps: ConfigMap[];
  secrets: Secret[];
  ingresses: Ingress[];
  statefulsets: StatefulSet[];
  daemonsets: DaemonSet[];
}

export interface HistoryState {
  at: string;
  resources: HistoryResources;
}

export interface HistoryChange {
  resource: string;
  key: string;
  type: "added" | "removed" | "modified";
  summary: string[];
  before?: unknown;
  after?: unknown;
}

export interface HistoryDiff {
  from: string;
  to: string;
  changes: HistoryChange[];
  events: KubeEvent[];
}

// One state snapshot serves every page rendered for a given (context, at)
// pair: the pages all remount together when the scrubber commits, so their
// concurrent getX calls share a single in-flight /history/state request.
let historyStateCache: { key: string; promise: Promise<HistoryState> } | null =
  null;

function fetchHistoryState(at: number): Promise<HistoryState> {
  const key = `${currentContext}|${at}`;
  if (historyStateCache?.key === key) return historyStateCache.promise;
  const promise = fetchApi<HistoryState>(`/history/state?at=${at}`);
  // A failed snapshot must not be cached, or retry buttons would replay the
  // same rejection forever.
  promise.catch(() => {
    if (historyStateCache?.promise === promise) historyStateCache = null;
  });
  historyStateCache = { key, promise };
  return promise;
}

// historySlice resolves one resource kind from the snapshot, applying the
// same namespace filter the live endpoints implement server-side.
async function historySlice<K extends keyof HistoryResources>(
  kind: K,
  at: number,
  ns?: string
): Promise<HistoryResources[K]> {
  const state = await fetchHistoryState(at);
  const items = state.resources[kind] ?? [];
  if (!ns) return items;
  return items.filter(
    (item) => (item as { namespace?: string }).namespace === ns
  ) as HistoryResources[K];
}

export const api = {
  getContexts: () => fetchApi<ContextInfo[]>("/contexts"),
  getCluster: () => fetchApi<ClusterInfo>("/cluster"),
  getNamespaces: () =>
    currentAt !== null
      ? historySlice("namespaces", currentAt)
      : fetchApi<Namespace[]>("/namespaces"),
  getPods: (ns?: string) =>
    currentAt !== null
      ? historySlice("pods", currentAt, ns)
      : fetchApi<Pod[]>(ns ? `/pods?namespace=${ns}` : "/pods"),
  getPod: async (ns: string, name: string): Promise<Pod> => {
    if (currentAt === null) return fetchApi<Pod>(`/pods/${ns}/${name}`);
    const pods = await historySlice("pods", currentAt, ns);
    const pod = pods.find((item) => item.name === name);
    if (!pod) throw new Error("Pod not found at this moment in history");
    return pod;
  },
  getPodLogs: (ns: string, name: string, container?: string) =>
    fetchApi<{ logs: string }>(
      `/pods/${ns}/${name}/logs${container ? `?container=${container}` : ""}`
    ),
  getDeployments: (ns?: string) =>
    currentAt !== null
      ? historySlice("deployments", currentAt, ns)
      : fetchApi<Deployment[]>(ns ? `/deployments?namespace=${ns}` : "/deployments"),
  getServices: (ns?: string) =>
    currentAt !== null
      ? historySlice("services", currentAt, ns)
      : fetchApi<Service[]>(ns ? `/services?namespace=${ns}` : "/services"),
  getNodes: () =>
    currentAt !== null
      ? historySlice("nodes", currentAt)
      : fetchApi<NodeInfo[]>("/nodes"),
  getEvents: (ns?: string) =>
    currentAt !== null
      ? historySlice("events", currentAt, ns)
      : fetchApi<KubeEvent[]>(ns ? `/events?namespace=${ns}` : "/events"),
  getConfigMaps: (ns?: string) =>
    currentAt !== null
      ? historySlice("configmaps", currentAt, ns)
      : fetchApi<ConfigMap[]>(ns ? `/configmaps?namespace=${encodeURIComponent(ns)}` : "/configmaps"),
  getSecrets: (ns?: string) =>
    currentAt !== null
      ? historySlice("secrets", currentAt, ns)
      : fetchApi<Secret[]>(ns ? `/secrets?namespace=${encodeURIComponent(ns)}` : "/secrets"),
  getIngresses: (ns?: string) =>
    currentAt !== null
      ? historySlice("ingresses", currentAt, ns)
      : fetchApi<Ingress[]>(ns ? `/ingresses?namespace=${encodeURIComponent(ns)}` : "/ingresses"),
  getStatefulSets: (ns?: string) =>
    currentAt !== null
      ? historySlice("statefulsets", currentAt, ns)
      : fetchApi<StatefulSet[]>(ns ? `/statefulsets?namespace=${encodeURIComponent(ns)}` : "/statefulsets"),
  getDaemonSets: (ns?: string) =>
    currentAt !== null
      ? historySlice("daemonsets", currentAt, ns)
      : fetchApi<DaemonSet[]>(ns ? `/daemonsets?namespace=${encodeURIComponent(ns)}` : "/daemonsets"),
  // History endpoints are always live: the range powers the scrubber and the
  // diff view compares two explicit moments.
  getHistoryRange: () => fetchApi<HistoryRange>("/history/range"),
  getHistoryDiff: (from: number, to: number) =>
    fetchApi<HistoryDiff>(`/history/diff?from=${from}&to=${to}`),
};

// eventSourceUrl carries the active context like fetchApi does: watch and
// log streams must hit the same cluster the lists were fetched from, or a
// context switch would mix live events from one cluster into another's
// tables. The context in the URL also makes the shared watch connection
// reopen on switch (its URL comparison sees a change).
export function eventSourceUrl(path: string): string {
  return `${API_BASE}${withContext(path)}`;
}

export function podLogStreamUrl(namespace: string, name: string, container?: string): string {
  const params = new URLSearchParams({ follow: "true", tailLines: "100" });
  if (container) params.set("container", container);
  return eventSourceUrl(`/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/logs?${params}`);
}
