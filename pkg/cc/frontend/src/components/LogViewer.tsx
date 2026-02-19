import { useState, useEffect, useRef, useCallback } from 'react';
import type { LogLine } from '@/types';

interface LogViewerProps {
  logs: LogLine[];
  selectedService: string | null;
}

const levelColors: Record<string, string> = {
  ERROR: 'text-red-400',
  WARN: 'text-amber-400',
  WARNING: 'text-amber-400',
  INFO: 'text-sky-400',
  DEBUG: 'text-gray-500',
};

const levels = ['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG'] as const;

export default function LogViewer({ logs, selectedService }: LogViewerProps) {
  const [levelFilter, setLevelFilter] = useState<string>('ALL');
  const [textFilter, setTextFilter] = useState('');
  const [follow, setFollow] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);

  const filteredLogs = logs.filter(line => {
    if (selectedService && line.Service !== selectedService) return false;
    if (levelFilter !== 'ALL' && line.Level !== levelFilter) return false;
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
    <div className="flex h-full flex-col overflow-hidden rounded-sm border border-border bg-surface">
      {/* Toolbar */}
      <div className="flex items-center gap-3 border-b border-border px-3 py-2">
        {/* Source badge */}
        <div className="flex items-center gap-2">
          <svg className="size-3.5 text-gray-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z"
            />
          </svg>
          <span className="text-xs/4 font-medium text-gray-300">{selectedService ?? 'All services'}</span>
        </div>

        <div className="mx-1 h-4 w-px bg-border" />

        {/* Level filter */}
        <div className="flex items-center gap-0.5">
          {levels.map(level => (
            <button
              key={level}
              onClick={() => setLevelFilter(level)}
              className={`rounded-xs px-1.5 py-0.5 text-xs/4 font-medium transition-colors ${
                levelFilter === level
                  ? level === 'ERROR'
                    ? 'bg-red-500/15 text-red-400'
                    : level === 'WARN'
                      ? 'bg-amber-500/15 text-amber-400'
                      : 'bg-indigo-500/15 text-indigo-400'
                  : 'text-gray-600 hover:text-gray-400'
              }`}
            >
              {level}
            </button>
          ))}
        </div>

        {/* Spacer + right controls */}
        <div className="ml-auto flex items-center gap-2">
          {/* Search */}
          <div className="flex items-center gap-1.5 rounded-xs bg-surface-light px-2 py-1">
            <svg className="size-3 text-gray-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z"
              />
            </svg>
            <input
              type="text"
              placeholder="Filter..."
              value={textFilter}
              onChange={e => setTextFilter(e.target.value)}
              className="w-28 bg-transparent text-xs/4 text-gray-300 placeholder:text-gray-600 focus:outline-hidden"
            />
            {textFilter && (
              <button onClick={() => setTextFilter('')} className="text-gray-600 hover:text-gray-400">
                <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>

          {/* Follow toggle */}
          <button
            onClick={() => setFollow(!follow)}
            className={`flex items-center gap-1 rounded-xs px-2 py-1 text-xs/4 font-medium transition-colors ${
              follow ? 'bg-emerald-500/15 text-emerald-400' : 'text-gray-600 hover:text-gray-400'
            }`}
          >
            <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 13.5 12 21m0 0-7.5-7.5M12 21V3" />
            </svg>
            Follow
          </button>
        </div>
      </div>

      {/* Log content */}
      <div ref={containerRef} onScroll={onScroll} className="flex-1 overflow-auto p-2 font-mono text-xs/5">
        {filteredLogs.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-3">
            <svg className="size-8 text-gray-800" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1}>
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z"
              />
            </svg>
            <span className="text-xs/4 text-gray-700">
              {selectedService ? 'No logs matching filters' : 'Waiting for logs...'}
            </span>
          </div>
        ) : (
          filteredLogs.slice(-5000).map((line, i) => (
            <div key={i} className={`break-all whitespace-pre-wrap ${levelColors[line.Level] ?? 'text-gray-400'}`}>
              {line.Message}
            </div>
          ))
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between border-t border-border px-3 py-1 text-xs/4 text-gray-700">
        <span>
          {filteredLogs.length} lines
          {filteredLogs.length !== logs.length && <span className="text-gray-800"> / {logs.length} total</span>}
        </span>
        {follow && (
          <span className="flex items-center gap-1 text-emerald-600">
            <span className="size-1 animate-pulse rounded-full bg-emerald-500" />
            auto-scrolling
          </span>
        )}
      </div>
    </div>
  );
}
