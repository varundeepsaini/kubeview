const express = require("express");
const cors = require("cors");
const k8s = require("./lib/k8s-client");
const t = require("./lib/transformers");

const app = express();
const PORT = 5501;

app.use(cors({ origin: "http://localhost:5500" }));
app.use(express.json());

// Health check
app.get("/api/health", (_req, res) => {
  res.json({ status: "ok", timestamp: new Date().toISOString() });
});

// Cluster info
app.get("/api/cluster", async (_req, res, next) => {
  try {
    const info = await k8s.getClusterInfo();
    res.json(info);
  } catch (err) {
    next(err);
  }
});

// Namespaces
app.get("/api/namespaces", async (_req, res, next) => {
  try {
    const items = await k8s.listNamespaces();
    res.json(items.map(t.transformNamespace));
  } catch (err) {
    next(err);
  }
});

// Pods
app.get("/api/pods", async (req, res, next) => {
  try {
    const ns = req.query.namespace;
    const items = await k8s.listPods(ns || undefined);
    res.json(items.map(t.transformPod));
  } catch (err) {
    next(err);
  }
});

// Pod detail
app.get("/api/pods/:namespace/:name", async (req, res, next) => {
  try {
    const pod = await k8s.getPod(req.params.namespace, req.params.name);
    res.json(t.transformPod(pod));
  } catch (err) {
    if (err.statusCode === 404) {
      return res.status(404).json({ error: "Pod not found" });
    }
    next(err);
  }
});

// Pod logs
app.get("/api/pods/:namespace/:name/logs", async (req, res, next) => {
  try {
    const { container, tailLines } = req.query;
    const logs = await k8s.getPodLogs(
      req.params.namespace,
      req.params.name,
      container || undefined,
      tailLines ? parseInt(tailLines) : 100
    );
    res.json({ logs: logs || "" });
  } catch (err) {
    if (err.statusCode === 404) {
      return res.status(404).json({ error: "Pod not found" });
    }
    next(err);
  }
});

// Deployments
app.get("/api/deployments", async (req, res, next) => {
  try {
    const ns = req.query.namespace;
    const items = await k8s.listDeployments(ns || undefined);
    res.json(items.map(t.transformDeployment));
  } catch (err) {
    next(err);
  }
});

// Services
app.get("/api/services", async (req, res, next) => {
  try {
    const ns = req.query.namespace;
    const items = await k8s.listServices(ns || undefined);
    res.json(items.map(t.transformService));
  } catch (err) {
    next(err);
  }
});

// Nodes
app.get("/api/nodes", async (_req, res, next) => {
  try {
    const items = await k8s.listNodes();
    res.json(items.map(t.transformNode));
  } catch (err) {
    next(err);
  }
});

// Events
app.get("/api/events", async (req, res, next) => {
  try {
    const ns = req.query.namespace;
    const items = await k8s.listEvents(ns || undefined);
    res.json(items.map(t.transformEvent));
  } catch (err) {
    next(err);
  }
});

// Error handler
app.use((err, _req, res, _next) => {
  console.error("API Error:", err.message);
  const status = err.statusCode || err.status || 500;
  res.status(status).json({
    error: err.message || "Internal server error",
    status,
  });
});

app.listen(PORT, () => {
  console.log(`KubeView API running on http://localhost:${PORT}`);
});
