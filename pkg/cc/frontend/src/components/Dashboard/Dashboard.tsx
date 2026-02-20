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
  AIDiagnosis,
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
import DiagnosisPanel from '@/components/DiagnosisPanel';
import { useFavicon } from '@/hooks/useFavicon';
import { useNotifications } from '@/hooks/useNotifications';
import { Group, Panel, Separator } from 'react-resizable-panels';
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels';

const MAX_LOGS = 10000;
const leftCollapsedStorageKey = 'xcli:sidebar-left-collapsed';
const rightCollapsedStorageKey = 'xcli:sidebar-right-collapsed';
const dashboardLegacyLayoutStorageKey = 'xcli:dashboard:panel-layout';
const leftPanelWidthStorageKey = 'xcli:dashboard:left-panel-width-px';
const rightPanelWidthStorageKey = 'xcli:dashboard:right-panel-width-px';
const leftPanelId = 'dashboard-left-panel';
const mainPanelId = 'dashboard-main-panel';
const rightPanelId = 'dashboard-right-panel';
const defaultLeftPanelPx = 360;
const defaultRightPanelPx = 360;
const sidebarCollapsedPx = 56;
const sidebarExpandedMinPx = 220;
const sidebarExpandedMaxPx = 920;
const mainPanelMinPx = 420;
const expandedWidthCaptureThresholdPx = sidebarCollapsedPx + 8;

function loadSidebarState(key: string): boolean {
  try {
    return localStorage.getItem(key) === 'true';
  } catch {
    return false;
  }
}

function clampPanelSize(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function loadPanelWidth(key: string, fallback: number): number {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return fallback;
    const parsed = Number.parseFloat(raw);
    if (!Number.isFinite(parsed)) return fallback;
    return clampPanelSize(parsed, sidebarExpandedMinPx, sidebarExpandedMaxPx);
  } catch {
    return fallback;
  }
}

function persistPanelWidth(key: string, widthPx: number) {
  try {
    localStorage.setItem(key, String(Math.round(widthPx)));
  } catch {
    // ignore storage failures
  }
}

interface DashboardProps {
  onNavigateConfig?: () => void;
  onNavigateRedis?: () => void;
  stack: string;
  availableStacks: string[];
  onSwitchStack: (stack: string) => void;
}

export default function Dashboard({
  onNavigateConfig,
  onNavigateRedis,
  stack,
  availableStacks,
  onSwitchStack,
}: DashboardProps) {
  const { fetchJSON, postJSON, postDiagnose } = useAPI(stack);
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
  const [leftCollapsed, setLeftCollapsed] = useState(() => loadSidebarState(leftCollapsedStorageKey));
  const [rightCollapsed, setRightCollapsed] = useState(() => loadSidebarState(rightCollapsedStorageKey));
  const leftExpandedPxRef = useRef(loadPanelWidth(leftPanelWidthStorageKey, defaultLeftPanelPx));
  const rightExpandedPxRef = useRef(loadPanelWidth(rightPanelWidthStorageKey, defaultRightPanelPx));
  const leftPanelRef = useRef<PanelImperativeHandle | null>(null);
  const rightPanelRef = useRef<PanelImperativeHandle | null>(null);
  const leftDefaultDesktopPx = leftCollapsed ? sidebarCollapsedPx : leftExpandedPxRef.current;
  const rightDefaultDesktopPx = rightCollapsed ? sidebarCollapsedPx : rightExpandedPxRef.current;
  const [diagnoseAvailable, setDiagnoseAvailable] = useState(false);
  const [diagnosing, setDiagnosing] = useState<string | null>(null);
  const [diagnosis, setDiagnosis] = useState<AIDiagnosis | null>(null);
  const [diagnosisError, setDiagnosisError] = useState<string | null>(null);

  useEffect(() => {
    try {
      localStorage.removeItem(dashboardLegacyLayoutStorageKey);
    } catch {
      // ignore storage failures
    }
  }, []);

  const toggleLeft = useCallback(() => {
    setLeftCollapsed(prev => {
      const next = !prev;
      try {
        localStorage.setItem(leftCollapsedStorageKey, String(next));
      } catch {
        /* ignore */
      }

      requestAnimationFrame(() => {
        const panel = leftPanelRef.current;
        if (!panel) return;
        panel.resize(next ? sidebarCollapsedPx : leftExpandedPxRef.current);
      });
      return next;
    });
  }, []);

  const toggleRight = useCallback(() => {
    setRightCollapsed(prev => {
      const next = !prev;
      try {
        localStorage.setItem(rightCollapsedStorageKey, String(next));
      } catch {
        /* ignore */
      }

      requestAnimationFrame(() => {
        const panel = rightPanelRef.current;
        if (!panel) return;
        panel.resize(next ? sidebarCollapsedPx : rightExpandedPxRef.current);
      });
      return next;
    });
  }, []);

  // Fetch all REST data — called on mount and whenever SSE (re)connects
  const loadAllData = useCallback(() => {
    fetchJSON<StatusResponse>('/status')
      .then(data => {
        setServices(data.services);
        setInfrastructure(data.infrastructure);
        setConfig(data.config);
      })
      .catch(console.error);

    fetchJSON<GitResponse>('/git')
      .then(data => {
        setRepos(data.repos);
      })
      .catch(console.error);

    fetchJSON<LogLine[]>('/logs')
      .then(data => {
        if (data.length > 0) {
          logsRef.current = data;
          setLogs(data);
        }
      })
      .catch(console.error);

    fetchJSON<StackStatus>('/stack/status')
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
      fetchJSON<GitResponse>('/git')
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

  useSSE(handleSSE, loadAllData, stack);
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
      fetchJSON<LogLine[]>(`/services/${encodeURIComponent(tab)}/logs`)
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
    postJSON<{ status: string }>('/stack/cancel').catch(console.error);
  }, [postJSON]);

  const handleStackAction = useCallback(() => {
    if (!stackStatus || stackStatus === 'starting' || stackStatus === 'stopping') return;

    const endpoint = stackStatus === 'running' ? '/stack/down' : '/stack/up';

    postJSON<{ status: string }>(endpoint).catch(console.error);
  }, [stackStatus, postJSON]);

  // Check if Claude CLI is available on mount
  useEffect(() => {
    fetchJSON<{ available: boolean }>('/diagnose/available')
      .then(data => setDiagnoseAvailable(data.available))
      .catch(() => setDiagnoseAvailable(false));
  }, [fetchJSON]);

  const handleDiagnose = useCallback(
    (serviceName: string) => {
      setDiagnosing(serviceName);
      setDiagnosis(null);
      setDiagnosisError(null);

      postDiagnose<AIDiagnosis>(serviceName)
        .then(data => setDiagnosis(data))
        .catch(err => setDiagnosisError(err instanceof Error ? err.message : String(err)))
        .finally(() => setDiagnosing(null));
    },
    [postDiagnose]
  );

  const handleCloseDiagnosis = useCallback(() => {
    setDiagnosing(null);
    setDiagnosis(null);
    setDiagnosisError(null);
  }, []);

  const handleLeftPanelResize = useCallback(
    (panelSize: PanelSize) => {
      if (leftCollapsed) return;
      if (panelSize.inPixels <= expandedWidthCaptureThresholdPx) return;
      const next = clampPanelSize(panelSize.inPixels, sidebarExpandedMinPx, sidebarExpandedMaxPx);
      leftExpandedPxRef.current = next;
      persistPanelWidth(leftPanelWidthStorageKey, next);
    },
    [leftCollapsed]
  );

  const handleRightPanelResize = useCallback(
    (panelSize: PanelSize) => {
      if (rightCollapsed) return;
      if (panelSize.inPixels <= expandedWidthCaptureThresholdPx) return;
      const next = clampPanelSize(panelSize.inPixels, sidebarExpandedMinPx, sidebarExpandedMaxPx);
      rightExpandedPxRef.current = next;
      persistPanelWidth(rightPanelWidthStorageKey, next);
    },
    [rightCollapsed]
  );

  const leftSidebarContent = (
    <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${leftCollapsed ? 'invisible' : ''}`}>
      <div className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Services</div>
      {services.map(svc => (
        <ServiceCard
          key={svc.name}
          service={svc}
          selected={activeTab === svc.name}
          onSelect={() => handleSelectService(svc.name)}
          stack={stack}
          showDiagnose={diagnoseAvailable}
          onDiagnose={() => handleDiagnose(svc.name)}
        />
      ))}

      {infrastructure.length > 0 && (
        <>
          <div className="mt-2 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Infrastructure</div>
          {infrastructure.map(item =>
            item.name.toLowerCase().includes('redis') && onNavigateRedis ? (
              <button
                key={item.name}
                onClick={onNavigateRedis}
                className="group flex w-full items-center justify-between rounded-sm border border-border bg-surface-light p-3 text-left transition-colors hover:border-accent/40 hover:bg-surface-lighter"
                title="Open Redis Explorer"
              >
                <span className="flex items-center gap-2">
                  <span className="text-sm/5 font-medium text-text-primary">{item.name}</span>
                  <svg
                    className="size-3 text-text-muted transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-accent-light"
                    fill="none"
                    viewBox="0 0 24 24"
                    strokeWidth={2}
                    stroke="currentColor"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" d="M7 17 17 7m0 0H9m8 0v8" />
                  </svg>
                </span>
                <span
                  className={`text-xs/4 font-medium ${item.status === 'running' ? 'text-success' : 'text-text-muted'}`}
                >
                  {item.status}
                </span>
              </button>
            ) : (
              <div
                key={item.name}
                className="flex items-center justify-between rounded-sm border border-border bg-surface-light p-3"
              >
                <span className="text-sm/5 font-medium text-text-primary">{item.name}</span>
                <span
                  className={`text-xs/4 font-medium ${item.status === 'running' ? 'text-success' : 'text-text-muted'}`}
                >
                  {item.status}
                </span>
              </div>
            )
          )}
        </>
      )}
    </div>
  );

  const rightSidebarContent = (
    <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${rightCollapsed ? 'invisible' : ''}`}>
      <ConfigPanel config={config} services={services} onNavigateConfig={onNavigateConfig} />
      <GitStatus repos={repos} />
    </div>
  );

  const mainPanelContent = (
    <>
      {stackStatus === null ? (
        <Spinner centered />
      ) : stackStatus === 'starting' || stackStatus === 'stopping' ? (
        <StackProgress
          phases={derivePhaseStates(progressPhases, stackError, stackStatus === 'stopping' ? STOP_PHASES : BOOT_PHASES)}
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
            <svg className="size-12 text-error" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
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
          showDiagnose={diagnoseAvailable}
          onDiagnose={activeTab ? () => handleDiagnose(activeTab) : undefined}
        />
      ) : (
        <div className="flex h-full flex-col items-center justify-center gap-4 text-text-muted">
          <svg className="size-16 text-border" fill="none" viewBox="0 0 24 24" strokeWidth={1} stroke="currentColor">
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
    </>
  );

  return (
    <div className="flex h-dvh flex-col">
      <Header
        services={services}
        infrastructure={infrastructure}
        onNavigateConfig={onNavigateConfig}
        stackStatus={stackStatus}
        onStackAction={handleStackAction}
        currentPhase={progressPhases.length > 0 ? progressPhases[progressPhases.length - 1].message : undefined}
        notificationsEnabled={notificationsEnabled}
        onToggleNotifications={toggleNotifications}
        activeStack={stack}
        availableStacks={availableStacks}
        onSwitchStack={onSwitchStack}
      />

      <div className="hidden flex-1 overflow-hidden xl:flex">
        <Group orientation="horizontal" resizeTargetMinimumSize={{ fine: 20, coarse: 28 }}>
          <Panel
            id={leftPanelId}
            panelRef={leftPanelRef}
            defaultSize={leftDefaultDesktopPx}
            minSize={sidebarCollapsedPx}
            maxSize={sidebarExpandedMaxPx}
            onResize={handleLeftPanelResize}
          >
            <div className="h-full overflow-y-auto border-r border-border bg-surface">{leftSidebarContent}</div>
          </Panel>

          <Separator className="cc-resize-handle cc-resize-handle--with-toggle">
            <button
              onClick={toggleLeft}
              className="cc-panel-toggle cc-panel-toggle--left"
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
          </Separator>

          <Panel id={mainPanelId} minSize={mainPanelMinPx}>
            <div className="h-full overflow-hidden p-3">{mainPanelContent}</div>
          </Panel>

          <Separator className="cc-resize-handle cc-resize-handle--with-toggle">
            <button
              onClick={toggleRight}
              className="cc-panel-toggle cc-panel-toggle--right"
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
          </Separator>

          <Panel
            id={rightPanelId}
            panelRef={rightPanelRef}
            defaultSize={rightDefaultDesktopPx}
            minSize={sidebarCollapsedPx}
            maxSize={sidebarExpandedMaxPx}
            onResize={handleRightPanelResize}
          >
            <div className="h-full overflow-y-auto border-l border-border bg-surface">{rightSidebarContent}</div>
          </Panel>
        </Group>
      </div>

      <div className="flex flex-1 overflow-hidden xl:hidden">
        <div
          className={`relative shrink-0 border-r border-border bg-surface transition-[width] duration-200 ease-in-out ${leftCollapsed ? 'w-10' : 'w-72'}`}
        >
          {leftSidebarContent}
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

        <div className="flex-1 overflow-hidden p-3">{mainPanelContent}</div>

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
          {rightSidebarContent}
        </div>
      </div>

      {/* Diagnosis modal */}
      {(diagnosing || diagnosis || diagnosisError) && (
        <DiagnosisPanel
          serviceName={diagnosing ?? ''}
          diagnosis={diagnosis}
          error={diagnosisError}
          loading={diagnosing !== null}
          onClose={handleCloseDiagnosis}
          onRetry={() => {
            if (diagnosing) handleDiagnose(diagnosing);
          }}
        />
      )}
    </div>
  );
}
