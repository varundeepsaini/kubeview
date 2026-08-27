import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api, type ConfigMap, type DaemonSet, type Ingress, type Secret, type StatefulSet } from "@/lib/api";
import ConfigMapsPage from "@/app/configmaps/page";
import DaemonSetsPage from "@/app/daemonsets/page";
import IngressesPage from "@/app/ingresses/page";
import SecretsPage from "@/app/secrets/page";
import StatefulSetsPage from "@/app/statefulsets/page";

vi.mock("@/lib/api", () => ({
  api: {
    getNamespaces: vi.fn().mockResolvedValue([]),
    getConfigMaps: vi.fn(),
    getSecrets: vi.fn(),
    getIngresses: vi.fn(),
    getStatefulSets: vi.fn(),
    getDaemonSets: vi.fn(),
  },
  eventSourceUrl: (path: string) => `http://test/api${path}`,
}));

const mocked = vi.mocked(api);

beforeEach(() => {
  vi.clearAllMocks();
  mocked.getNamespaces.mockResolvedValue([]);
});

async function expectHeaders(labels: string[]) {
  const table = await screen.findByRole("table");
  for (const label of ["Name", "Namespace", ...labels]) {
    expect(within(table).getByRole("columnheader", { name: label })).toBeInTheDocument();
  }
  return table;
}

describe("ConfigMaps page", () => {
  it("renders columns and rows", async () => {
    const items: ConfigMap[] = [
      { name: "app-config", namespace: "default", keys: ["config.yaml", "log-level"], labels: {}, age: "3d" },
      { name: "empty-cm", namespace: "default", keys: [], labels: {}, age: "1h" },
    ];
    mocked.getConfigMaps.mockResolvedValue(items);
    render(<ConfigMapsPage />);

    await expectHeaders(["Keys", "Age"]);
    expect(screen.getByText("app-config")).toBeInTheDocument();
    expect(screen.getByText("config.yaml, log-level")).toBeInTheDocument();
    expect(screen.getByText("3d")).toBeInTheDocument();
    expect(screen.getByText("<none>")).toBeInTheDocument();
  });

  it("surfaces fetch errors", async () => {
    mocked.getConfigMaps.mockRejectedValue(new Error("configmaps unavailable"));
    render(<ConfigMapsPage />);
    expect(await screen.findByText("configmaps unavailable")).toBeInTheDocument();
  });
});

describe("Secrets page", () => {
  const secret: Secret = {
    name: "tls-cert",
    namespace: "prod",
    type: "kubernetes.io/tls",
    dataLengths: { "tls.crt": 1066, "tls.key": 227 },
    age: "12d",
  };

  it("renders type, key names and byte lengths", async () => {
    mocked.getSecrets.mockResolvedValue([secret]);
    render(<SecretsPage />);

    await expectHeaders(["Type", "Data", "Age"]);
    expect(screen.getByText("tls-cert")).toBeInTheDocument();
    expect(screen.getByText("kubernetes.io/tls")).toBeInTheDocument();
    expect(screen.getByText(/tls\.crt/)).toBeInTheDocument();
    expect(screen.getByText("(1066 bytes)")).toBeInTheDocument();
    expect(screen.getByText(/tls\.key/)).toBeInTheDocument();
    expect(screen.getByText("(227 bytes)")).toBeInTheDocument();
  });

  it("offers no way to reveal secret values", async () => {
    mocked.getSecrets.mockResolvedValue([secret]);
    render(<SecretsPage />);

    const table = await screen.findByRole("table");
    const row = within(table).getByText("tls-cert").closest("tr")!;
    expect(within(row).queryAllByRole("button")).toHaveLength(0);
    expect(screen.queryByText(/reveal|show value|decode/i)).not.toBeInTheDocument();
  });

  it("surfaces fetch errors", async () => {
    mocked.getSecrets.mockRejectedValue(new Error("secrets is forbidden"));
    render(<SecretsPage />);
    expect(await screen.findByText("secrets is forbidden")).toBeInTheDocument();
  });
});

describe("Ingresses page", () => {
  it("renders class, rules and addresses", async () => {
    const items: Ingress[] = [
      {
        name: "web-ingress",
        namespace: "default",
        class: "nginx",
        rules: [{ host: "example.com", path: "/api", service: "api-svc", port: "8080" }],
        addresses: ["10.0.0.5"],
        age: "5d",
      },
      { name: "bare-ingress", namespace: "default", class: "traefik", rules: [], addresses: [], age: "1d" },
    ];
    mocked.getIngresses.mockResolvedValue(items);
    render(<IngressesPage />);

    await expectHeaders(["Class", "Hosts / paths", "Address", "Age"]);
    expect(screen.getByText("web-ingress")).toBeInTheDocument();
    expect(screen.getByText("nginx")).toBeInTheDocument();
    expect(screen.getByText("example.com/api -> api-svc:8080")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.5")).toBeInTheDocument();
    expect(screen.getByText("<none>")).toBeInTheDocument();
    expect(screen.getByText("<pending>")).toBeInTheDocument();
  });

  it("surfaces fetch errors", async () => {
    mocked.getIngresses.mockRejectedValue(new Error("ingresses unavailable"));
    render(<IngressesPage />);
    expect(await screen.findByText("ingresses unavailable")).toBeInTheDocument();
  });
});

describe("StatefulSets page", () => {
  it("renders replica counts, service and strategy", async () => {
    const items: StatefulSet[] = [
      {
        name: "postgres",
        namespace: "db",
        serviceName: "postgres-headless",
        replicas: 3,
        desiredReplicas: 3,
        readyReplicas: 2,
        currentReplicas: 3,
        updatedReplicas: 1,
        strategy: "RollingUpdate",
        age: "30d",
      },
    ];
    mocked.getStatefulSets.mockResolvedValue(items);
    render(<StatefulSetsPage />);

    await expectHeaders(["Ready", "Current", "Updated", "Service", "Strategy", "Age"]);
    expect(screen.getByText("postgres")).toBeInTheDocument();
    expect(screen.getByText("2/3")).toBeInTheDocument();
    expect(screen.getByText("postgres-headless")).toBeInTheDocument();
    expect(screen.getByText("RollingUpdate")).toBeInTheDocument();
  });

  it("surfaces fetch errors", async () => {
    mocked.getStatefulSets.mockRejectedValue(new Error("statefulsets unavailable"));
    render(<StatefulSetsPage />);
    expect(await screen.findByText("statefulsets unavailable")).toBeInTheDocument();
  });
});

describe("DaemonSets page", () => {
  it("renders scheduling counts", async () => {
    const items: DaemonSet[] = [
      { name: "node-exporter", namespace: "monitoring", desired: 5, current: 5, ready: 4, updated: 3, available: 4, age: "60d" },
    ];
    mocked.getDaemonSets.mockResolvedValue(items);
    render(<DaemonSetsPage />);

    const table = await expectHeaders(["Desired", "Current", "Ready", "Up-to-date", "Available", "Age"]);
    const row = within(table).getByText("node-exporter").closest("tr")!;
    const cells = within(row).getAllByRole("cell").map((c) => c.textContent);
    expect(cells).toEqual(["node-exporter", "monitoring", "5", "5", "4", "3", "4", "60d"]);
  });

  it("surfaces fetch errors", async () => {
    mocked.getDaemonSets.mockRejectedValue(new Error("daemonsets unavailable"));
    render(<DaemonSetsPage />);
    expect(await screen.findByText("daemonsets unavailable")).toBeInTheDocument();
  });
});
