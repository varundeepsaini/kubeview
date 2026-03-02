const k8s = require("@kubernetes/client-node");

const kc = new k8s.KubeConfig();
kc.loadFromDefault();

const coreApi = kc.makeApiClient(k8s.CoreV1Api);
const appsApi = kc.makeApiClient(k8s.AppsV1Api);

async function getClusterInfo() {
  const versionApi = kc.makeApiClient(k8s.VersionApi);
  const version = await versionApi.getCode();
  const nodes = await coreApi.listNode();
  return {
    version: version.gitVersion,
    platform: version.platform,
    nodeCount: nodes.items.length,
    context: kc.getCurrentContext(),
    clusterName: kc.getCurrentCluster()?.name || "unknown",
  };
}

async function listNamespaces() {
  const res = await coreApi.listNamespace();
  return res.items;
}

async function listPods(namespace) {
  const res = namespace
    ? await coreApi.listNamespacedPod({ namespace })
    : await coreApi.listPodForAllNamespaces();
  return res.items;
}

async function getPod(namespace, name) {
  return coreApi.readNamespacedPod({ namespace, name });
}

async function getPodLogs(namespace, name, container, tailLines = 100) {
  const params = { namespace, name, tailLines };
  if (container) params.container = container;
  return coreApi.readNamespacedPodLog(params);
}

async function listDeployments(namespace) {
  const res = namespace
    ? await appsApi.listNamespacedDeployment({ namespace })
    : await appsApi.listDeploymentForAllNamespaces();
  return res.items;
}

async function listServices(namespace) {
  const res = namespace
    ? await coreApi.listNamespacedService({ namespace })
    : await coreApi.listServiceForAllNamespaces();
  return res.items;
}

async function listNodes() {
  const res = await coreApi.listNode();
  return res.items;
}

async function listEvents(namespace) {
  const res = namespace
    ? await coreApi.listNamespacedEvent({ namespace })
    : await coreApi.listEventForAllNamespaces();
  return res.items;
}

module.exports = {
  getClusterInfo,
  listNamespaces,
  listPods,
  getPod,
  getPodLogs,
  listDeployments,
  listServices,
  listNodes,
  listEvents,
};
