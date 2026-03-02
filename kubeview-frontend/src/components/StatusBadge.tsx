interface StatusBadgeProps {
  status: string;
  size?: "sm" | "md";
}

const statusColors: Record<string, string> = {
  Running: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
  Active: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
  Ready: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
  Succeeded: "bg-blue-500/15 text-blue-400 border-blue-500/30",
  Pending: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  ContainerCreating: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  Terminating: "bg-orange-500/15 text-orange-400 border-orange-500/30",
  Failed: "bg-red-500/15 text-red-400 border-red-500/30",
  CrashLoopBackOff: "bg-red-500/15 text-red-400 border-red-500/30",
  Error: "bg-red-500/15 text-red-400 border-red-500/30",
  NotReady: "bg-red-500/15 text-red-400 border-red-500/30",
  Unknown: "bg-gray-500/15 text-gray-400 border-gray-500/30",
};

export default function StatusBadge({ status, size = "sm" }: StatusBadgeProps) {
  const colors = statusColors[status] || statusColors.Unknown;
  const sizeClass = size === "sm" ? "px-2 py-0.5 text-xs" : "px-3 py-1 text-sm";

  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border font-medium ${colors} ${sizeClass}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${
        status === "Running" || status === "Active" || status === "Ready"
          ? "bg-emerald-400"
          : status === "Pending" || status === "ContainerCreating"
          ? "bg-yellow-400 animate-pulse"
          : status === "Failed" || status === "CrashLoopBackOff" || status === "Error"
          ? "bg-red-400"
          : "bg-gray-400"
      }`} />
      {status}
    </span>
  );
}
