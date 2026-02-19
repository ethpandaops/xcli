import { useState, useCallback } from "react";
import type { ServiceInfo } from "../types";
import { useAPI } from "../hooks/useAPI";

interface ServiceCardProps {
  service: ServiceInfo;
  selected: boolean;
  onSelect: () => void;
}

const statusColors: Record<string, string> = {
  running: "bg-emerald-500",
  stopped: "bg-gray-500",
  crashed: "bg-red-500",
};

const healthIcons: Record<string, { color: string; title: string; path: string }> = {
  healthy: {
    color: "text-emerald-400",
    title: "Healthy",
    path: "M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
  },
  unhealthy: {
    color: "text-red-400",
    title: "Unhealthy",
    path: "m9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
  },
  degraded: {
    color: "text-amber-400",
    title: "Degraded",
    path: "M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z",
  },
  unknown: {
    color: "text-gray-500",
    title: "Unknown",
    path: "M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 5.25h.008v.008H12v-.008Z",
  },
};

export default function ServiceCard({
  service,
  selected,
  onSelect,
}: ServiceCardProps) {
  const { postAction } = useAPI();
  const [loading, setLoading] = useState<string | null>(null);

  const doAction = useCallback(
    async (action: string) => {
      setLoading(action);

      try {
        await postAction(service.name, action);
      } catch (err) {
        console.error(`${action} failed:`, err);
      } finally {
        setLoading(null);
      }
    },
    [postAction, service.name],
  );

  const isRunning = service.status === "running";

  // Append /docs for cbt-api services when opening in browser
  const openUrl = service.url && service.name.startsWith("cbt-api-")
    ? `${service.url}/docs`
    : service.url;

  const actionLabels: Record<string, string> = {
    start: "Starting",
    stop: "Stopping",
    restart: "Restarting",
    rebuild: "Rebuilding",
  };

  return (
    <div
      onClick={onSelect}
      className={`relative cursor-pointer rounded-sm border p-3 transition-colors ${
        selected
          ? "border-indigo-500/50 bg-indigo-500/10"
          : "border-border bg-surface-light hover:border-gray-600"
      }`}
    >
      {loading && (
        <div className="absolute inset-0 z-10 flex items-center justify-center overflow-hidden rounded-sm bg-gray-900/80">
          <div className="flex items-center gap-2">
            <div className="size-3.5 animate-spin rounded-full border-2 border-amber-400 border-t-transparent" />
            <span className="text-xs font-medium text-amber-400">
              {actionLabels[loading] ?? loading}...
            </span>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className={`size-2 rounded-full ${statusColors[service.status] ?? "bg-gray-500"}`}
          />
          <span className="text-sm/5 font-medium text-white">
            {service.name}
          </span>
        </div>
        {(() => {
          const icon = healthIcons[service.health] ?? healthIcons.unknown;
          return (
            <svg
              className={`size-4 ${icon.color}`}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <title>{icon.title}</title>
              <path strokeLinecap="round" strokeLinejoin="round" d={icon.path} />
            </svg>
          );
        })()}
      </div>

      <div className="mt-2 flex items-center gap-3 text-xs/4 text-gray-500">
        {service.uptime && <span>{service.uptime}</span>}
        {service.pid > 0 && <span>PID {service.pid}</span>}
        {service.url && service.name !== "lab-backend" && (
          <a
            href={openUrl}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="inline-flex items-center gap-0.5 text-indigo-400 hover:text-indigo-300"
          >
            open
            <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 6H5.25A2.25 2.25 0 0 0 3 8.25v10.5A2.25 2.25 0 0 0 5.25 21h10.5A2.25 2.25 0 0 0 18 18.75V10.5m-4.5-6H18m0 0v4.5m0-4.5-7.5 7.5" />
            </svg>
          </a>
        )}
      </div>

      <div className={`mt-2 flex gap-1 ${loading ? "invisible" : ""}`}>
        {!isRunning && (
          <ActionBtn
            label="Start"
            loading={loading === "start"}
            onClick={() => doAction("start")}
          />
        )}
        {isRunning && (
          <>
            <ActionBtn
              label="Stop"
              loading={loading === "stop"}
              onClick={() => doAction("stop")}
            />
            <ActionBtn
              label="Restart"
              loading={loading === "restart"}
              onClick={() => doAction("restart")}
            />
          </>
        )}
        <ActionBtn
          label="Rebuild"
          loading={loading === "rebuild"}
          onClick={() => doAction("rebuild")}
        />
      </div>
    </div>
  );
}

function ActionBtn({
  label,
  loading,
  onClick,
}: {
  label: string;
  loading: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      disabled={loading}
      className="rounded-xs bg-surface-lighter px-2 py-0.5 text-xs/4 text-gray-300 transition-colors hover:bg-gray-600 hover:text-white disabled:opacity-50"
    >
      {loading ? "..." : label}
    </button>
  );
}
