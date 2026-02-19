import { useState, useCallback, useEffect, useRef } from 'react';
import type {
  ServiceInfo,
  InfraInfo,
  ConfigInfo,
  LogLine,
  RepoInfo,
  StatusResponse,
  GitResponse,
  HealthStatus,
  StackStatus,
  StackProgressEvent,
} from '@/types';
import { useSSE } from '@/hooks/useSSE';
import { useAPI } from '@/hooks/useAPI';
import Header from '@/components/Header';
import ServiceCard from '@/components/ServiceCard';
import ConfigPanel from '@/components/ConfigPanel';
import GitStatus from '@/components/GitStatus';
import LogViewer from '@/components/LogViewer';
import StackProgress, { derivePhaseStates, BOOT_PHASES, STOP_PHASES } from '@/components/StackProgress';
import Spinner from '@/components/Spinner';
import { useFavicon } from '@/hooks/useFavicon';
import { useNotifications } from '@/hooks/useNotifications';

const MAX_LOGS = 10000;

function loadSidebarState(key: string): boolean {
  try {
    return localStorage.getItem(key) === 'true';
  } catch {
    return false;
  }
}

interface DashboardProps {
  onNavigateConfig?: () => void;
}

export default function Dashboard({ onNavigateConfig }: DashboardProps) {
  const { fetchJSON, postJSON } = useAPI();
  const { notify, enabled: notificationsEnabled, toggle: toggleNotifications } = useNotifications();
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [infrastructure, setInfrastructure] = useState<InfraInfo[]>([]);
  const [config, setConfig] = useState<ConfigInfo | null>(null);
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [openTabs, setOpenTabs] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const logsRef = useRef<LogLine[]>([]);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [stackStatus, setStackStatus] = useState<string | null>(null);
  const [progressPhases, setProgressPhases] = useState<StackProgressEvent[]>([]);
  const [stackError, setStackError] = useState<string | null>(null);
  const [leftCollapsed, setLeftCollapsed] = useState(() => loadSidebarState('xcli:sidebar-left-collapsed'));
  const [rightCollapsed, setRightCollapsed] = useState(() => loadSidebarState('xcli:sidebar-right-collapsed'));

  const toggleLeft = useCallback(() => {
    setLeftCollapsed(prev => {
      const next = !prev;
      try {
        localStorage.setItem('xcli:sidebar-left-collapsed', String(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  }, []);

  const toggleRight = useCallback(() => {
    setRightCollapsed(prev => {
      const next = !prev;
      try {
        localStorage.setItem('xcli:sidebar-right-collapsed', String(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  }, []);

  // Fetch all REST data — called on mount and whenever SSE (re)connects
  const loadAllData = useCallback(() => {
    fetchJSON<StatusResponse>('/api/status')
      .then(data => {
        setServices(data.services);
        setInfrastructure(data.infrastructure);
        setConfig(data.config);
      })
      .catch(console.error);

    fetchJSON<GitResponse>('/api/git')
      .then(data => {
        setRepos(data.repos);
      })
      .catch(console.error);

    fetchJSON<LogLine[]>('/api/logs')
      .then(data => {
        if (data.length > 0) {
          logsRef.current = data;
          setLogs(data);
        }
      })
      .catch(console.error);

    fetchJSON<StackStatus>('/api/stack/status')
      .then(data => {
        setStackStatus(data.status);

        if (data.error) {
          setStackError(data.error);
        }

        if (data.progress && data.progress.length > 0) {
          setProgressPhases(data.progress);
        }
      })
      .catch(console.error);
  }, [fetchJSON]);

  // Initial data load + periodic git refresh
  useEffect(() => {
    loadAllData();

    const gitInterval = setInterval(() => {
      fetchJSON<GitResponse>('/api/git')
        .then(data => {
          setRepos(data.repos);
        })
        .catch(console.error);
    }, 60000);

    return () => {
      clearInterval(gitInterval);
    };
  }, [fetchJSON, loadAllData]);

  // SSE handler
  const handleSSE = useCallback(
    (event: string, data: unknown) => {
      switch (event) {
        case 'services': {
          const incoming = data as ServiceInfo[];
          setServices(prev => {
            const healthMap = new Map(prev.map(s => [s.name, s.health]));
            return incoming.map(s => ({
              ...s,
              health: s.health && s.health !== 'unknown' ? s.health : (healthMap.get(s.name) ?? s.health),
            }));
          });
          break;
        }
        case 'infrastructure':
          setInfrastructure(data as InfraInfo[]);
          break;
        case 'health': {
          const health = data as HealthStatus;
          setServices(prev =>
            prev.map(s => {
              const h = health[s.name];

              return h ? { ...s, health: h.Status } : s;
            })
          );
          break;
        }
        case 'log': {
          const line = data as LogLine;
          logsRef.current = [...logsRef.current.slice(-(MAX_LOGS - 1)), line];
          setLogs(logsRef.current);
          break;
        }
        case 'stack_progress': {
          const progress = data as StackProgressEvent;
          setProgressPhases(prev => [...prev, progress]);
          break;
        }
        case 'stack_starting':
          setStackStatus('starting');
          setProgressPhases([]);
          setStackError(null);
          break;
        case 'stack_started':
          setStackStatus('running');
          notify('xcli: Stack Booted 🍆', { body: 'All services are up and running.' });
          break;
        case 'stack_error': {
          const errData = data as { error?: string };
          const msg = errData?.error ?? 'Unknown error';
          setStackError(msg);
          break;
        }
        case 'stack_stopping':
          setStackStatus('stopping');
          setProgressPhases([]);
          setStackError(null);
          break;
        case 'stack_stopped':
          setStackStatus('stopped');
          setProgressPhases([]);
          setStackError(null);
          notify('xcli: Stack Stopped 🍆', { body: 'All services have been stopped.' });
          break;
        case 'stack_status': {
          const status = data as StackStatus;
          setStackStatus(status.status);

          if (status.error) {
            setStackError(status.error);
          }

          break;
        }
      }
    },
    [notify]
  );

  useSSE(handleSSE, loadAllData);
  useFavicon(stackStatus, !!stackError);

  // Track which stopped-service logs we've already fetched to avoid duplicate requests
  const fetchedStoppedLogs = useRef<Set<string>>(new Set());

  // Fetch log file from disk when a stopped service tab is opened
  useEffect(() => {
    for (const tab of openTabs) {
      if (fetchedStoppedLogs.current.has(tab)) continue;

      const svc = services.find(s => s.name === tab);
      if (!svc || svc.status === 'running' || !svc.logFile) continue;

      fetchedStoppedLogs.current.add(tab);
      fetchJSON<LogLine[]>(`/api/services/${encodeURIComponent(tab)}/logs`)
        .then(data => {
          if (data.length > 0) {
            logsRef.current = [...logsRef.current.filter(l => l.Service !== tab), ...data];
            setLogs(logsRef.current);
          }
        })
        .catch(console.error);
    }
  }, [openTabs, services, fetchJSON]);

  const handleSelectService = useCallback(
    (name: string) => {
      if (activeTab === name) {
        // Clicking active tab deselects — go back to "All"
        setActiveTab(null);
      } else {
        // Open tab if not already open, and make it active
        setOpenTabs(prev => (prev.includes(name) ? prev : [...prev, name]));
        setActiveTab(name);
      }
    },
    [activeTab]
  );

  const handleCloseTab = useCallback((name: string) => {
    setOpenTabs(prev => prev.filter(t => t !== name));
    setActiveTab(prev => (prev === name ? null : prev));
  }, []);

  const handleCancelBoot = useCallback(() => {
    postJSON<{ status: string }>('/api/stack/cancel').catch(console.error);
  }, [postJSON]);

  const handleStackAction = useCallback(() => {
    if (!stackStatus || stackStatus === 'starting' || stackStatus === 'stopping') return;

    const endpoint = stackStatus === 'running' ? '/api/stack/down' : '/api/stack/up';

    postJSON<{ status: string }>(endpoint).catch(console.error);
  }, [stackStatus, postJSON]);

  return (
    <div className="flex h-dvh flex-col">
      <Header
        services={services}
        infrastructure={infrastructure}
        mode={config?.mode ?? ''}
        onNavigateConfig={onNavigateConfig}
        stackStatus={stackStatus}
        onStackAction={handleStackAction}
        currentPhase={progressPhases.length > 0 ? progressPhases[progressPhases.length - 1].message : undefined}
        notificationsEnabled={notificationsEnabled}
        onToggleNotifications={toggleNotifications}
      />

      <div className="flex flex-1 overflow-hidden">
        {/* Left sidebar - Services + Infra */}
        <div
          className={`relative shrink-0 border-r border-border bg-surface transition-[width] duration-200 ease-in-out ${leftCollapsed ? 'w-10' : 'w-72'}`}
        >
          <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${leftCollapsed ? 'invisible' : ''}`}>
            <div className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Services</div>
            {services.map(svc => (
              <ServiceCard
                key={svc.name}
                service={svc}
                selected={activeTab === svc.name}
                onSelect={() => handleSelectService(svc.name)}
              />
            ))}

            {infrastructure.length > 0 && (
              <>
                <div className="mt-2 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">
                  Infrastructure
                </div>
                {infrastructure.map(item => (
                  <div
                    key={item.name}
                    className="flex items-center justify-between rounded-sm border border-border bg-surface-light p-3"
                  >
                    <span className="text-sm/5 font-medium text-text-primary">{item.name}</span>
                    <span
                      className={`text-xs/4 font-medium ${
                        item.status === 'running' ? 'text-success' : 'text-text-muted'
                      }`}
                    >
                      {item.status}
                    </span>
                  </div>
                ))}
              </>
            )}
          </div>
          <button
            onClick={toggleLeft}
            className="absolute top-2 -right-3 z-10 flex size-6 items-center justify-center rounded-full border border-border bg-surface-light text-text-muted transition-colors hover:bg-surface-lighter hover:text-text-secondary"
            title={leftCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            <svg className="size-3" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              {leftCollapsed ? (
                <path strokeLinecap="round" strokeLinejoin="round" d="m9 5 7 7-7 7" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" d="m15 19-7-7 7-7" />
              )}
            </svg>
          </button>
        </div>

        {/* Main area - adaptive center panel */}
        <div className="flex-1 overflow-hidden p-3">
          {stackStatus === null ? (
            <Spinner centered />
          ) : stackStatus === 'starting' || stackStatus === 'stopping' ? (
            <StackProgress
              phases={derivePhaseStates(
                progressPhases,
                stackError,
                stackStatus === 'stopping' ? STOP_PHASES : BOOT_PHASES
              )}
              error={stackError}
              title={stackStatus === 'stopping' ? 'Stopping Stack' : 'Booting Stack'}
              onCancel={stackStatus === 'starting' ? handleCancelBoot : undefined}
            />
          ) : stackError ? (
            progressPhases.length > 0 ? (
              <StackProgress
                phases={derivePhaseStates(progressPhases, stackError, BOOT_PHASES)}
                error={stackError}
                title="Boot Failed"
                onRetry={() => {
                  setStackError(null);
                  setProgressPhases([]);
                  handleStackAction();
                }}
              />
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-4">
                <svg
                  className="size-12 text-error"
                  fill="none"
                  viewBox="0 0 24 24"
                  strokeWidth={1.5}
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="m9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
                  />
                </svg>
                <p className="text-sm font-medium text-error">Stack boot failed</p>
                <p className="max-w-md text-center font-mono text-xs/5 text-error/70">{stackError}</p>
                <button
                  onClick={() => {
                    setStackError(null);
                    handleStackAction();
                  }}
                  className="mt-2 rounded-md bg-success/20 px-4 py-2 text-sm font-medium text-success transition-colors hover:bg-success/30"
                >
                  Retry Boot
                </button>
              </div>
            )
          ) : services.some(s => s.status === 'running') || openTabs.length > 0 ? (
            <LogViewer
              logs={logs}
              activeTab={activeTab}
              openTabs={openTabs}
              onSelectTab={setActiveTab}
              onCloseTab={handleCloseTab}
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-4 text-text-muted">
              <svg
                className="size-16 text-border"
                fill="none"
                viewBox="0 0 24 24"
                strokeWidth={1}
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M5.25 14.25h13.5m-13.5 0a3 3 0 0 1-3-3m3 3a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-16.5-3a3 3 0 0 1 3-3h13.5a3 3 0 0 1 3 3m-19.5 0a4.5 4.5 0 0 1 .9-2.7L5.737 5.1a3.375 3.375 0 0 1 2.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 0 1 .9 2.7m0 0a3 3 0 0 1-3 3m0 3h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Z"
                />
              </svg>
              <p className="text-sm">Stack is not running</p>
              <button
                onClick={handleStackAction}
                className="rounded-md bg-success/20 px-4 py-2 text-sm font-medium text-success transition-colors hover:bg-success/30"
              >
                Boot Stack
              </button>
            </div>
          )}
        </div>

        {/* Right sidebar - Config + Git */}
        <div
          className={`relative shrink-0 border-l border-border bg-surface transition-[width] duration-200 ease-in-out ${rightCollapsed ? 'w-10' : 'w-72'}`}
        >
          <button
            onClick={toggleRight}
            className="absolute top-2 -left-3 z-10 flex size-6 items-center justify-center rounded-full border border-border bg-surface-light text-text-muted transition-colors hover:bg-surface-lighter hover:text-text-secondary"
            title={rightCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            <svg className="size-3" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              {rightCollapsed ? (
                <path strokeLinecap="round" strokeLinejoin="round" d="m15 19-7-7 7-7" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" d="m9 5 7 7-7 7" />
              )}
            </svg>
          </button>
          <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${rightCollapsed ? 'invisible' : ''}`}>
            <ConfigPanel config={config} services={services} onNavigateConfig={onNavigateConfig} />
            <GitStatus repos={repos} />
          </div>
        </div>
      </div>
    </div>
  );
}
