import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import type { LogLine } from '@/types';

interface LogViewerProps {
  logs: LogLine[];
  activeTab: string | null;
  openTabs: string[];
  onSelectTab: (tab: string | null) => void;
  onCloseTab: (tab: string) => void;
}

interface TabState {
  levelFilter: string;
  textFilter: string;
  follow: boolean;
}

const levelColors: Record<string, string> = {
  FATAL: 'text-error',
  ERROR: 'text-error',
  WARN: 'text-warning',
  WARNING: 'text-warning',
  INFO: 'text-info',
  DEBUG: 'text-text-muted',
};

const levels = ['ALL', 'FATAL', 'ERROR', 'WARN', 'INFO', 'DEBUG'] as const;

function getDefaultTabState(): TabState {
  return { levelFilter: 'ALL', textFilter: '', follow: true };
}

export default function LogViewer({ logs, activeTab, openTabs, onSelectTab, onCloseTab }: LogViewerProps) {
  const [tabStates, setTabStates] = useState<Map<string, TabState>>(() => new Map());
  const containerRef = useRef<HTMLDivElement>(null);
  const tabBarRef = useRef<HTMLDivElement>(null);

  // Key for the current tab ("__all__" for All Services, or the service name)
  const tabKey = activeTab ?? '__all__';

  const currentState = tabStates.get(tabKey) ?? getDefaultTabState();

  const updateCurrentState = useCallback(
    (updater: (prev: TabState) => TabState) => {
      setTabStates(prev => {
        const next = new Map(prev);
        next.set(tabKey, updater(next.get(tabKey) ?? getDefaultTabState()));
        return next;
      });
    },
    [tabKey]
  );

  const setLevelFilter = useCallback(
    (level: string) => updateCurrentState(s => ({ ...s, levelFilter: level })),
    [updateCurrentState]
  );

  const setTextFilter = useCallback(
    (text: string) => updateCurrentState(s => ({ ...s, textFilter: text })),
    [updateCurrentState]
  );

  const setFollow = useCallback((follow: boolean) => updateCurrentState(s => ({ ...s, follow })), [updateCurrentState]);

  // Clean up tab states for closed tabs
  useEffect(() => {
    setTabStates(prev => {
      const validKeys = new Set(['__all__', ...openTabs]);
      let changed = false;
      const next = new Map(prev);
      for (const key of next.keys()) {
        if (!validKeys.has(key)) {
          next.delete(key);
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [openTabs]);

  const filteredLogs = useMemo(() => {
    const { levelFilter, textFilter } = currentState;
    const lowerTextFilter = textFilter.toLowerCase();
    return logs.filter(line => {
      if (activeTab && line.Service !== activeTab) return false;
      if (levelFilter !== 'ALL' && line.Level !== levelFilter) return false;
      if (lowerTextFilter && !line.Message.toLowerCase().includes(lowerTextFilter)) return false;
      return true;
    });
  }, [logs, activeTab, currentState]);

  // Auto-scroll when following
  useEffect(() => {
    if (currentState.follow && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [filteredLogs.length, currentState.follow]);

  // Detect manual scroll to disable follow
  const onScroll = useCallback(() => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 40;
    if (atBottom !== currentState.follow) {
      setFollow(atBottom);
    }
  }, [currentState.follow, setFollow]);

  const { levelFilter, textFilter, follow } = currentState;

  return (
    <div className="flex h-full flex-col overflow-hidden rounded-sm border border-border bg-surface">
      {/* Tab bar */}
      <div className="flex items-center border-b border-border">
        <div ref={tabBarRef} className="flex min-w-0 flex-1 items-center overflow-x-auto">
          {/* All Services tab — always present */}
          <button
            onClick={() => onSelectTab(null)}
            className={`shrink-0 border-r border-border px-3 py-1.5 text-xs/4 font-medium transition-colors ${
              activeTab === null
                ? 'bg-surface-light text-text-primary'
                : 'text-text-muted hover:bg-surface-light/50 hover:text-text-secondary'
            }`}
          >
            All Services
          </button>

          {/* Service tabs */}
          {openTabs.map(tab => (
            <div
              key={tab}
              className={`group flex shrink-0 items-center border-r border-border transition-colors ${
                activeTab === tab
                  ? 'bg-surface-light text-text-primary'
                  : 'text-text-muted hover:bg-surface-light/50 hover:text-text-secondary'
              }`}
            >
              <button onClick={() => onSelectTab(tab)} className="py-1.5 pr-1 pl-3 text-xs/4 font-medium">
                {tab}
              </button>
              <button
                onClick={e => {
                  e.stopPropagation();
                  onCloseTab(tab);
                }}
                className="mr-1 rounded-xs p-0.5 text-text-disabled transition-colors hover:bg-hover/10 hover:text-text-secondary"
                title="Close tab"
              >
                <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Toolbar */}
      <div className="flex items-center gap-3 border-b border-border px-3 py-2">
        {/* Level filter */}
        <div className="flex items-center gap-0.5">
          {levels.map(level => (
            <button
              key={level}
              onClick={() => setLevelFilter(level)}
              className={`rounded-xs px-1.5 py-0.5 text-xs/4 font-medium transition-colors ${
                levelFilter === level
                  ? level === 'FATAL' || level === 'ERROR'
                    ? 'bg-error/15 text-error'
                    : level === 'WARN'
                      ? 'bg-warning/15 text-warning'
                      : 'bg-accent/15 text-accent-light'
                  : 'text-text-disabled hover:text-text-tertiary'
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
            <svg
              className="size-3 text-text-disabled"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
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
              className="w-28 bg-transparent text-xs/4 text-text-secondary placeholder:text-text-disabled focus:outline-hidden"
            />
            {textFilter && (
              <button onClick={() => setTextFilter('')} className="text-text-disabled hover:text-text-tertiary">
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
              follow ? 'bg-success/15 text-success' : 'text-text-disabled hover:text-text-tertiary'
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
            <svg className="size-8 text-border" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1}>
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z"
              />
            </svg>
            <span className="text-xs/4 text-border">
              {activeTab ? 'No logs matching filters' : 'Waiting for logs...'}
            </span>
          </div>
        ) : (
          filteredLogs.slice(-5000).map((line, i) => (
            <div key={i} className={`break-all whitespace-pre-wrap ${levelColors[line.Level] ?? 'text-text-tertiary'}`}>
              {line.Message}
            </div>
          ))
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between border-t border-border px-3 py-1 text-xs/4 text-border">
        <span>
          {filteredLogs.length} lines
          {filteredLogs.length !== logs.length && <span className="text-border"> / {logs.length} total</span>}
        </span>
        {follow && (
          <span className="flex items-center gap-1 text-success/60">
            <span className="size-1 animate-pulse rounded-full bg-success" />
            auto-scrolling
          </span>
        )}
      </div>
    </div>
  );
}
