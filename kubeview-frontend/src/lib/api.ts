// Build-time override for non-local deployments; falls back to the dev
// backend. NEXT_PUBLIC_ vars are inlined by Next.js at build time.
const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:5501/api";

// currentContext is the ambient kubeconfig context appended to every request as
// ?context=. Empty means "let the backend use its default (current) context".
// ClusterProvider keeps this in sync with the user's selection so callers don't
// have to thread the context through every api.getX signature.
let currentContext = "";

export function setApiContext(name: string): void {
  currentContext = name;
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

export const api = {
  getContexts: () => fetchApi<ContextInfo[]>("/contexts"),
  getCluster: () => fetchApi<ClusterInfo>("/cluster"),
  getNamespaces: () => fetchApi<Namespace[]>("/namespaces"),
  getPods: (ns?: string) => fetchApi<Pod[]>(ns ? `/pods?namespace=${ns}` : "/pods"),
  getPod: (ns: string, name: string) => fetchApi<Pod>(`/pods/${ns}/${name}`),
  getPodLogs: (ns: string, name: string, container?: string) =>
    fetchApi<{ logs: string }>(
      `/pods/${ns}/${name}/logs${container ? `?container=${container}` : ""}`
    ),
  getDeployments: (ns?: string) =>
    fetchApi<Deployment[]>(ns ? `/deployments?namespace=${ns}` : "/deployments"),
  getServices: (ns?: string) =>
    fetchApi<Service[]>(ns ? `/services?namespace=${ns}` : "/services"),
  getNodes: () => fetchApi<NodeInfo[]>("/nodes"),
  getEvents: (ns?: string) =>
    fetchApi<KubeEvent[]>(ns ? `/events?namespace=${ns}` : "/events"),
};
