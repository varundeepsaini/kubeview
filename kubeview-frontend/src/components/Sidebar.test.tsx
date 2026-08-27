import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Sidebar from "@/components/Sidebar";
import { ClusterProvider } from "@/components/ClusterProvider";

const usePathname = vi.fn<() => string>();
vi.mock("next/navigation", () => ({
  usePathname: () => usePathname(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    getContexts: vi.fn().mockResolvedValue([
      { name: "kind-dev", cluster: "kind-dev", current: true },
      { name: "prod", cluster: "prod", current: false },
    ]),
  },
  setApiContext: vi.fn(),
}));

const expectedNav: [string, string][] = [
  ["Dashboard", "/"],
  ["Namespaces", "/namespaces"],
  ["Pods", "/pods"],
  ["Deployments", "/deployments"],
  ["Services", "/services"],
  ["ConfigMaps", "/configmaps"],
  ["Secrets", "/secrets"],
  ["Ingresses", "/ingresses"],
  ["StatefulSets", "/statefulsets"],
  ["DaemonSets", "/daemonsets"],
  ["Events", "/events"],
  ["Nodes", "/nodes"],
];

// Sidebar embeds the ContextSwitcher, which needs the ClusterProvider and
// loads contexts on mount; waiting for the switcher keeps that state update
// inside the test.
async function renderSidebar() {
  render(
    <ClusterProvider>
      <Sidebar />
    </ClusterProvider>
  );
  await screen.findByRole("combobox");
}

describe("Sidebar", () => {
  beforeEach(() => {
    usePathname.mockReturnValue("/");
  });

  it("renders every nav entry with the right href", async () => {
    await renderSidebar();
    for (const [label, href] of expectedNav) {
      expect(screen.getByRole("link", { name: label })).toHaveAttribute("href", href);
    }
    expect(screen.getAllByRole("link")).toHaveLength(expectedNav.length);
  });

  it("highlights only the link matching the current pathname", async () => {
    usePathname.mockReturnValue("/secrets");
    await renderSidebar();

    expect(screen.getByRole("link", { name: "Secrets" })).toHaveClass("text-accent");
    expect(screen.getByRole("link", { name: "Dashboard" })).not.toHaveClass("text-accent");
    expect(screen.getByRole("link", { name: "ConfigMaps" })).not.toHaveClass("text-accent");
  });

  it("highlights the dashboard link on the root path", async () => {
    await renderSidebar();
    expect(screen.getByRole("link", { name: "Dashboard" })).toHaveClass("text-accent");
    expect(screen.getByRole("link", { name: "Pods" })).not.toHaveClass("text-accent");
  });

  it("lists the kubeconfig contexts with the backend's current one selected", async () => {
    await renderSidebar();

    const switcher = screen.getByRole("combobox", { name: "Context" });
    expect(switcher).toHaveValue("kind-dev");
    const options = within(switcher).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["kind-dev (current)", "prod"]);
  });
});
