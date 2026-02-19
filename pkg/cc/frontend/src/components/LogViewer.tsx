import { useState, useEffect, useRef, useCallback } from "react";
import type { LogLine } from "../types";

interface LogViewerProps {
  logs: LogLine[];
  selectedService: string | null;
}

const levelColors: Record<string, string> = {
  ERROR: "text-red-400",
  WARN: "text-amber-400",
  WARNING: "text-amber-400",
  INFO: "text-sky-400",
  DEBUG: "text-gray-500",
};

const levels = ["ALL", "ERROR", "WARN", "INFO", "DEBUG"] as const;

export default function LogViewer({ logs, selectedService }: LogViewerProps) {
  const [levelFilter, setLevelFilter] = useState<string>("ALL");
  const [textFilter, setTextFilter] = useState("");
  const [follow, setFollow] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);

  const filteredLogs = logs.filter((line) => {
    if (selectedService && line.Service !== selectedService) return false;
    if (levelFilter !== "ALL" && line.Level !== levelFilter) return false;
    if (textFilter && !line.Message.toLowerCase().includes(textFilter.toLowerCase())) return false;

    return true;
  });

  // Auto-scroll when following
  useEffect(() => {
    if (follow && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [filteredLogs.length, follow]);

  // Detect manual scroll to disable follow
  const onScroll = useCallback(() => {
    if (!containerRef.current) return;

    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 40;

    setFollow(atBottom);
  }, []);

  return (
    <div className="flex h-full flex-col rounded-sm border border-border bg-surface-light">
      {/* Toolbar */}
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <span className="text-xs/4 font-semibold text-gray-400">Logs</span>
        {selectedService && (
          <span className="rounded-xs bg-indigo-500/20 px-2 py-0.5 text-xs/4 text-indigo-400">
            {selectedService}
          </span>
        )}

        <div className="ml-auto flex items-center gap-2">
          {/* Level filter buttons */}
          {levels.map((level) => (
            <button
              key={level}
              onClick={() => setLevelFilter(level)}
              className={`rounded-xs px-1.5 py-0.5 text-xs/4 transition-colors ${
                levelFilter === level
                  ? "bg-indigo-500/30 text-indigo-300"
                  : "text-gray-500 hover:text-gray-300"
              }`}
            >
              {level}
            </button>
          ))}

          {/* Text filter */}
          <input
            type="text"
            placeholder="Filter..."
            value={textFilter}
            onChange={(e) => setTextFilter(e.target.value)}
            className="rounded-xs border border-border bg-surface px-2 py-0.5 text-xs/4 text-gray-300 placeholder:text-gray-600 focus:border-indigo-500 focus:outline-hidden"
          />

          {/* Follow toggle */}
          <button
            onClick={() => setFollow(!follow)}
            className={`rounded-xs px-1.5 py-0.5 text-xs/4 ${
              follow
                ? "bg-emerald-500/20 text-emerald-400"
                : "text-gray-500 hover:text-gray-300"
            }`}
          >
            Follow
          </button>
        </div>
      </div>

      {/* Log content */}
      <div
        ref={containerRef}
        onScroll={onScroll}
        className="flex-1 overflow-auto p-2 font-mono text-xs/5"
      >
        {filteredLogs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-gray-600">
            {selectedService
              ? "No logs matching filters"
              : "Select a service to view logs"}
          </div>
        ) : (
          filteredLogs.slice(-5000).map((line, i) => (
            <div
              key={i}
              className={`whitespace-pre-wrap break-all ${levelColors[line.Level] ?? "text-gray-400"}`}
            >
              {line.Message}
            </div>
          ))
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between border-t border-border px-3 py-1 text-xs/4 text-gray-600">
        <span>
          {filteredLogs.length} lines
          {filteredLogs.length !== logs.length && ` (${logs.length} total)`}
        </span>
        {follow && <span className="text-emerald-600">auto-scrolling</span>}
      </div>
    </div>
  );
}
