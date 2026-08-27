import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ResourceList from "@/components/ResourceList";

vi.mock("@/lib/api", () => ({
  api: {
    getNamespaces: vi.fn().mockResolvedValue([
      { name: "default", status: "Active", labels: {}, createdAt: "", age: "1d" },
      { name: "kube-system", status: "Active", labels: {}, createdAt: "", age: "1d" },
    ]),
  },
  eventSourceUrl: (path: string) => `http://test/api${path}`,
}));

interface Widget {
  name: string;
  namespace: string;
  status: string;
}

const columns = [{ label: "Status", render: (w: Widget) => w.status }];

const widgets: Widget[] = [
  { name: "web-1", namespace: "default", status: "Running" },
  { name: "worker-1", namespace: "jobs", status: "Pending" },
];

describe("ResourceList", () => {
  it("shows a loading spinner until data arrives", () => {
    render(
      <ResourceList title="Widgets" fetcher={() => new Promise<Widget[]>(() => {})} columns={columns} />
    );
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("renders a table row per item with name, namespace and custom columns", async () => {
    render(<ResourceList title="Widgets" fetcher={async () => widgets} columns={columns} />);

    const table = await screen.findByRole("table");
    expect(within(table).getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: "Namespace" })).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: "Status" })).toBeInTheDocument();

    const row = within(table).getByText("web-1").closest("tr")!;
    expect(within(row).getByText("default")).toBeInTheDocument();
    expect(within(row).getByText("Running")).toBeInTheDocument();
    expect(screen.getByText("worker-1")).toBeInTheDocument();
    expect(screen.getByText("2 widgets")).toBeInTheDocument();
  });

  it("renders an empty table and zero count when there is no data", async () => {
    render(<ResourceList title="Widgets" fetcher={async () => []} columns={columns} />);

    expect(await screen.findByText("0 widgets")).toBeInTheDocument();
    const table = screen.getByRole("table");
    expect(within(table).queryAllByRole("row")).toHaveLength(1); // header only
  });

  it("shows the error message and recovers via the retry button", async () => {
    const fetcher = vi
      .fn<(ns?: string) => Promise<Widget[]>>()
      .mockRejectedValueOnce(new Error("connection refused"))
      .mockResolvedValue(widgets);

    render(<ResourceList title="Widgets" fetcher={fetcher} columns={columns} />);

    expect(await screen.findByText("connection refused")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("web-1")).toBeInTheDocument();
    expect(screen.queryByText("connection refused")).not.toBeInTheDocument();
  });

  it("filters rows by the search input", async () => {
    render(<ResourceList title="Widgets" fetcher={async () => widgets} columns={columns} />);
    await screen.findByText("web-1");

    fireEvent.change(screen.getByPlaceholderText("Search widgets..."), {
      target: { value: "worker" },
    });

    expect(screen.getByText("worker-1")).toBeInTheDocument();
    expect(screen.queryByText("web-1")).not.toBeInTheDocument();
  });

  it("matches search against the namespace too", async () => {
    render(<ResourceList title="Widgets" fetcher={async () => widgets} columns={columns} />);
    await screen.findByText("web-1");

    fireEvent.change(screen.getByPlaceholderText("Search widgets..."), {
      target: { value: "JOBS" },
    });

    expect(screen.getByText("worker-1")).toBeInTheDocument();
    expect(screen.queryByText("web-1")).not.toBeInTheDocument();
  });

  it("refetches with the selected namespace", async () => {
    const fetcher = vi.fn<(ns?: string) => Promise<Widget[]>>(async () => widgets);
    render(<ResourceList title="Widgets" fetcher={fetcher} columns={columns} />);
    await screen.findByText("web-1");
    expect(fetcher).toHaveBeenCalledWith(undefined);

    await screen.findByRole("option", { name: "kube-system" });
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "kube-system" } });

    await vi.waitFor(() => expect(fetcher).toHaveBeenLastCalledWith("kube-system"));
  });
});
