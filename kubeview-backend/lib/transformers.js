function transformNamespace(ns) {
  return {
    name: ns.metadata.name,
    status: ns.status?.phase || "Unknown",
    labels: ns.metadata.labels || {},
    createdAt: ns.metadata.creationTimestamp,
    age: getAge(ns.metadata.creationTimestamp),
  };
}

function transformPod(pod) {
  const containerStatuses = pod.status?.containerStatuses || [];
  const readyCount = containerStatuses.filter((c) => c.ready).length;
  const totalCount =
    containerStatuses.length || pod.spec?.containers?.length || 0;
  const restarts = containerStatuses.reduce(
    (sum, c) => sum + (c.restartCount || 0),
    0
  );

  return {
    name: pod.metadata.name,
    namespace: pod.metadata.namespace,
    status: getPodStatus(pod),
    ready: `${readyCount}/${totalCount}`,
    restarts,
    node: pod.spec?.nodeName || "Pending",
    ip: pod.status?.podIP || "N/A",
    labels: pod.metadata.labels || {},
    createdAt: pod.metadata.creationTimestamp,
    age: getAge(pod.metadata.creationTimestamp),
    containers: (pod.spec?.containers || []).map((c, i) => ({
      name: c.name,
      image: c.image,
      ports: (c.ports || []).map((p) => `${p.containerPort}/${p.protocol}`),
      ready: containerStatuses[i]?.ready || false,
      state: getContainerState(containerStatuses[i]),
      restartCount: containerStatuses[i]?.restartCount || 0,
    })),
    conditions: (pod.status?.conditions || []).map((c) => ({
      type: c.type,
      status: c.status,
      reason: c.reason,
      lastTransition: c.lastTransitionTime,
    })),
    volumes: (pod.spec?.volumes || []).map((v) => ({
      name: v.name,
      type: Object.keys(v).filter((k) => k !== "name")[0] || "unknown",
    })),
  };
}

function transformDeployment(dep) {
  return {
    name: dep.metadata.name,
    namespace: dep.metadata.namespace,
    replicas: dep.status?.replicas || 0,
    readyReplicas: dep.status?.readyReplicas || 0,
    desiredReplicas: dep.spec?.replicas || 0,
    updatedReplicas: dep.status?.updatedReplicas || 0,
    availableReplicas: dep.status?.availableReplicas || 0,
    strategy: dep.spec?.strategy?.type || "RollingUpdate",
    labels: dep.metadata.labels || {},
    selector: dep.spec?.selector?.matchLabels || {},
    createdAt: dep.metadata.creationTimestamp,
    age: getAge(dep.metadata.creationTimestamp),
    conditions: (dep.status?.conditions || []).map((c) => ({
      type: c.type,
      status: c.status,
      reason: c.reason,
      message: c.message,
      lastTransition: c.lastTransitionTime,
    })),
    images: (dep.spec?.template?.spec?.containers || []).map((c) => c.image),
  };
}

function transformService(svc) {
  return {
    name: svc.metadata.name,
    namespace: svc.metadata.namespace,
    type: svc.spec?.type || "ClusterIP",
    clusterIP: svc.spec?.clusterIP || "None",
    externalIP:
      svc.status?.loadBalancer?.ingress?.[0]?.ip ||
      svc.spec?.externalIPs?.[0] ||
      "N/A",
    ports: (svc.spec?.ports || []).map(
      (p) => `${p.port}${p.targetPort ? ":" + p.targetPort : ""}/${p.protocol}`
    ),
    selector: svc.spec?.selector || {},
    labels: svc.metadata.labels || {},
    createdAt: svc.metadata.creationTimestamp,
    age: getAge(svc.metadata.creationTimestamp),
  };
}

function transformNode(node) {
  const conditions = node.status?.conditions || [];
  const ready = conditions.find((c) => c.type === "Ready");
  const roles = Object.keys(node.metadata?.labels || {})
    .filter((l) => l.startsWith("node-role.kubernetes.io/"))
    .map((l) => l.replace("node-role.kubernetes.io/", ""));

  return {
    name: node.metadata.name,
    status: ready?.status === "True" ? "Ready" : "NotReady",
    roles: roles.length > 0 ? roles : ["<none>"],
    version: node.status?.nodeInfo?.kubeletVersion || "unknown",
    os: node.status?.nodeInfo?.osImage || "unknown",
    arch: node.status?.nodeInfo?.architecture || "unknown",
    containerRuntime: node.status?.nodeInfo?.containerRuntimeVersion || "unknown",
    cpu: node.status?.capacity?.cpu || "0",
    memory: node.status?.capacity?.memory || "0",
    pods: node.status?.capacity?.pods || "0",
    labels: node.metadata.labels || {},
    conditions: conditions.map((c) => ({
      type: c.type,
      status: c.status,
      reason: c.reason,
      message: c.message,
    })),
    createdAt: node.metadata.creationTimestamp,
    age: getAge(node.metadata.creationTimestamp),
    addresses: (node.status?.addresses || []).map((a) => ({
      type: a.type,
      address: a.address,
    })),
  };
}

function transformEvent(event) {
  return {
    type: event.type,
    reason: event.reason,
    message: event.message,
    object: `${event.involvedObject?.kind}/${event.involvedObject?.name}`,
    namespace: event.metadata?.namespace,
    firstSeen: event.firstTimestamp,
    lastSeen: event.lastTimestamp,
    count: event.count || 1,
    source: event.source?.component,
  };
}

function getPodStatus(pod) {
  if (pod.metadata?.deletionTimestamp) return "Terminating";
  const phase = pod.status?.phase || "Unknown";
  const containerStatuses = pod.status?.containerStatuses || [];
  for (const cs of containerStatuses) {
    if (cs.state?.waiting?.reason) return cs.state.waiting.reason;
    if (cs.state?.terminated?.reason) return cs.state.terminated.reason;
  }
  return phase;
}

function getContainerState(cs) {
  if (!cs) return "Waiting";
  if (cs.state?.running) return "Running";
  if (cs.state?.waiting) return cs.state.waiting.reason || "Waiting";
  if (cs.state?.terminated) return cs.state.terminated.reason || "Terminated";
  return "Unknown";
}

function getAge(timestamp) {
  if (!timestamp) return "Unknown";
  const created = new Date(timestamp);
  const now = new Date();
  const diffMs = now - created;
  const diffSecs = Math.floor(diffMs / 1000);
  if (diffSecs < 60) return `${diffSecs}s`;
  const diffMins = Math.floor(diffSecs / 60);
  if (diffMins < 60) return `${diffMins}m`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d`;
}

module.exports = {
  transformNamespace,
  transformPod,
  transformDeployment,
  transformService,
  transformNode,
  transformEvent,
};
