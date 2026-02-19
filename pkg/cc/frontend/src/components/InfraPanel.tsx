import type { InfraInfo } from "../types";

interface InfraPanelProps {
  infrastructure: InfraInfo[];
}

const typeIcons: Record<string, string> = {
  clickhouse: "CH",
  redis: "RD",
  unknown: "??",
};

export default function InfraPanel({ infrastructure }: InfraPanelProps) {
  if (infrastructure.length === 0) {
    return (
      <div className="rounded-sm border border-border bg-surface-light p-4">
        <h3 className="mb-2 text-sm/5 font-semibold text-gray-400">
          Infrastructure
        </h3>
        <p className="text-xs/4 text-gray-600">No infrastructure found</p>
      </div>
    );
  }

  return (
    <div className="rounded-sm border border-border bg-surface-light p-4">
      <h3 className="mb-3 text-sm/5 font-semibold text-gray-400">
        Infrastructure
      </h3>
      <div className="flex flex-col gap-2">
        {infrastructure.map((item) => (
          <div
            key={item.name}
            className="flex items-center justify-between rounded-xs bg-surface px-3 py-2"
          >
            <div className="flex items-center gap-2">
              <span className="rounded-xs bg-surface-lighter px-1.5 py-0.5 font-mono text-xs/4 text-gray-400">
                {typeIcons[item.type] ?? typeIcons.unknown}
              </span>
              <span className="text-sm/5 text-gray-200">{item.name}</span>
            </div>
            <span
              className={`text-xs/4 font-medium ${
                item.status === "running"
                  ? "text-emerald-400"
                  : "text-gray-500"
              }`}
            >
              {item.status}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
