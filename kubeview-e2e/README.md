# kubeview-e2e

End-to-end tests that drive the **real** stack with Playwright: a real
Kubernetes cluster (kind), the real Go backend, and the production build of
the Next.js frontend. No mocks.

```
kind cluster (seeded fixtures) → Go backend :5501 → Next.js standalone :5500 → Chromium
```

## Running locally

Prerequisites: a running kind (or other local) cluster set as your current
kubectl context, plus Go and Node installed.

```bash
# 1. Seed the deterministic fixtures the specs assert against.
kubectl apply -f fixtures.yaml
kubectl -n e2e-demo wait --for=condition=Ready pod/e2e-logger pod/e2e-multi --timeout=120s

# 2. Build the frontend (its API base is inlined at build time and defaults
#    to http://localhost:5501/api).
( cd ../kubeview-frontend && npm ci && npm run build )

# 3. Install test deps and browsers.
npm ci
npx playwright install chromium

# 4. Run. Playwright starts the backend and frontend automatically
#    (see webServer in playwright.config.ts).
npm test
```

The backend is launched with `CORS_ORIGIN=http://localhost:5500` against your
current kubeconfig context. In CI (`.github/workflows/ci.yml`, `e2e` job) a
throwaway kind cluster is created first.

## Fixtures

`fixtures.yaml` seeds namespace `e2e-demo` with stable-named resources:
`e2e-logger` (emits a known log line), `e2e-multi` (two containers, for the
log container picker), deployment `e2e-web`, and service `e2e-svc`.
