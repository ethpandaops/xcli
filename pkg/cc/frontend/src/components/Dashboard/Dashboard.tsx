import { useState, useCallback, useEffect, useRef } from 'react';
import type {
  ServiceInfo,
  ConfigInfo,
  LogLine,
  RepoInfo,
  StatusResponse,
  GitResponse,
  HealthStatus,
  StackStatus,
  StackProgressEvent,
  DiagnosisReport,
  DiagnosisTurn,
  AIProviderInfo,
  CBTOverridesState,
  StackCapabilities,
  XatuConfigResponse,
} from '@/types';
import { useSSE } from '@/hooks/useSSE';
import { useAPI } from '@/hooks/useAPI';
import Header from '@/components/Header';
import ServiceCard from '@/components/ServiceCard';
import ConfigPanel from '@/components/ConfigPanel';
import XatuConfigPanel from '@/components/XatuConfigPanel';
import GitStatus from '@/components/GitStatus';
import CBTOverridesGlance from '@/components/CBTOverridesGlance';
import SidebarSection from '@/components/SidebarSection';
import LogViewer from '@/components/LogViewer';
import StackProgress, {
  derivePhaseStates,
  BOOT_PHASES,
  STOP_PHASES,
  XATU_BOOT_PHASES,
  XATU_STOP_PHASES,
} from '@/components/StackProgress';
import Spinner from '@/components/Spinner';
import DiagnosisPanel from '@/components/DiagnosisPanel';
import { useFavicon } from '@/hooks/useFavicon';
import { useNotifications } from '@/hooks/useNotifications';
import { Group, Panel, Separator } from 'react-resizable-panels';
import type { PanelImperativeHandle, PanelSize } from 'react-resizable-panels';

const MAX_LOGS = 100_000;

function bootPhasesFor(isLab: boolean) {
  return isLab ? BOOT_PHASES : XATU_BOOT_PHASES;
}

function stopPhasesFor(isLab: boolean) {
  return isLab ? STOP_PHASES : XATU_STOP_PHASES;
}
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

function createRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }

  return `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function normalizeDiagnosis(report: DiagnosisReport): DiagnosisReport {
  return {
    rootCause: report.rootCause ?? '',
    explanation: report.explanation ?? '',
    affectedFiles: Array.isArray(report.affectedFiles) ? report.affectedFiles : [],
    suggestions: Array.isArray(report.suggestions) ? report.suggestions : [],
    fixCommands: Array.isArray(report.fixCommands) ? report.fixCommands : [],
  };
}

function appendLine(prev: string, text: string): string {
  if (text.trim() === '') return prev;
  return prev + (text.endsWith('\n') ? text : `${text}\n`);
}

interface DashboardProps {
  onNavigateConfig?: () => void;
  onNavigateOverrides?: () => void;
  stack: string;
  availableStacks: string[];
  onSwitchStack: (stack: string) => void;
  capabilities: StackCapabilities;
  otherRunningStack?: string | null;
  onStackStatusChange?: (status: string) => void;
}

export default function Dashboard({
  onNavigateConfig,
  onNavigateOverrides,
  stack,
  availableStacks,
  onSwitchStack,
  capabilities,
  otherRunningStack,
  onStackStatusChange,
}: DashboardProps) {
  const { fetchJSON, postJSON, postDiagnoseStart, postDiagnoseMessage, postDiagnoseInterrupt, deleteDiagnoseSession } =
    useAPI(stack);
  const { notify, enabled: notificationsEnabled, toggle: toggleNotifications } = useNotifications();
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [config, setConfig] = useState<ConfigInfo | null>(null);
  const [xatuConfig, setXatuConfig] = useState<XatuConfigResponse | null>(null);
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [overrides, setOverrides] = useState<CBTOverridesState | null>(null);
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
  const [providers, setProviders] = useState<AIProviderInfo[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<string>('claude');
  const [diagnoseService, setDiagnoseService] = useState<string | null>(null);
  const [diagnoseSessionId, setDiagnoseSessionId] = useState<string | null>(null);
  const [diagnoseRequestId, setDiagnoseRequestId] = useState<string | null>(null);
  const [diagnosing, setDiagnosing] = useState(false);
  const [diagnosis, setDiagnosis] = useState<DiagnosisReport | null>(null);
  const [diagnosisError, setDiagnosisError] = useState<string | null>(null);
  const [thinkingText, setThinkingText] = useState('');
  const [answerText, setAnswerText] = useState('');
  const [activityText, setActivityText] = useState('');
  const [completedTurns, setCompletedTurns] = useState<DiagnosisTurn[]>([]);
  const [currentTurnPrompt, setCurrentTurnPrompt] = useState<string | undefined>(undefined);
  const diagnoseAvailable = providers.some(provider => provider.available);
  const selectedProviderInfo = providers.find(provider => provider.id === selectedProvider);
  const diagnoseSessionRef = useRef<string | null>(null);
  const diagnoseRequestRef = useRef<string | null>(null);

  // Determine which sidebar sections to show based on stack capabilities
  const isLabStack = capabilities.hasServiceConfigs;
  const showCbtOverrides = capabilities.hasCbtOverrides;
  const showGitRepos = capabilities.hasGitRepos;

  useEffect(() => {
    diagnoseSessionRef.current = diagnoseSessionId;
  }, [diagnoseSessionId]);

  useEffect(() => {
    diagnoseRequestRef.current = diagnoseRequestId;
  }, [diagnoseRequestId]);

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
        setServices(data.services ?? []);
        setConfig(data.config);
      })
      .catch(console.error);

    if (showGitRepos) {
      fetchJSON<GitResponse>('/git')
        .then(data => {
          setRepos(data.repos);
        })
        .catch(console.error);
    }

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

    if (showCbtOverrides) {
      fetchJSON<CBTOverridesState>('/config/overrides').then(setOverrides).catch(console.error);
    }

    if (!isLabStack) {
      fetchJSON<XatuConfigResponse>('/config').then(setXatuConfig).catch(console.error);
    }
  }, [fetchJSON, showGitRepos, showCbtOverrides, isLabStack]);

  // Initial data load + periodic git refresh
  useEffect(() => {
    loadAllData();

    let gitInterval: ReturnType<typeof setInterval> | undefined;

    if (showGitRepos) {
      gitInterval = setInterval(() => {
        fetchJSON<GitResponse>('/git')
          .then(data => {
            setRepos(data.repos);
          })
          .catch(console.error);
      }, 60000);
    }

    return () => {
      if (gitInterval) clearInterval(gitInterval);
    };
  }, [fetchJSON, loadAllData, showGitRepos]);

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
        case 'diagnose_stream': {
          const evt = data as {
            sessionId?: string;
            requestId?: string;
            kind?: string;
            text?: string;
            eventType?: string;
          };
          if (!evt.sessionId) break;
          if (diagnoseSessionRef.current && evt.sessionId !== diagnoseSessionRef.current) break;
          if (!evt.requestId || evt.requestId !== diagnoseRequestRef.current) break;

          if (evt.kind === 'thinking') {
            setThinkingText(prev => prev + (evt.text ?? ''));
          } else if (evt.kind === 'meta') {
            const msg = evt.text?.trim() || evt.eventType || 'activity';
            setActivityText(prev => appendLine(prev, msg));
          } else {
            setAnswerText(prev => prev + (evt.text ?? ''));
          }
          break;
        }
        case 'diagnose_started': {
          const evt = data as {
            sessionId?: string;
            requestId?: string;
          };
          if (!evt.sessionId) break;
          if (diagnoseSessionRef.current && evt.sessionId !== diagnoseSessionRef.current) break;
          if (!evt.requestId || evt.requestId !== diagnoseRequestRef.current) break;
          setDiagnosing(true);
          break;
        }
        case 'diagnose_result': {
          const evt = data as {
            sessionId?: string;
            requestId?: string;
            rawText?: string;
            diagnosis?: DiagnosisReport;
          };
          if (!evt.sessionId) break;
          if (diagnoseSessionRef.current && evt.sessionId !== diagnoseSessionRef.current) break;
          if (!evt.requestId || evt.requestId !== diagnoseRequestRef.current) break;

          if (evt.diagnosis && evt.diagnosis.rootCause && evt.diagnosis.rootCause !== 'See explanation below') {
            setDiagnosis(normalizeDiagnosis(evt.diagnosis));
          }
          if (evt.rawText && evt.rawText.trim() !== '') {
            setAnswerText(prev => (prev.trim() !== '' ? prev : (evt.rawText ?? '')));
          }

          setDiagnosing(false);
          break;
        }
        case 'diagnose_error': {
          const evt = data as {
            sessionId?: string;
            requestId?: string;
            error?: string;
          };
          if (!evt.sessionId) break;
          if (diagnoseSessionRef.current && evt.sessionId !== diagnoseSessionRef.current) break;
          if (!evt.requestId || evt.requestId !== diagnoseRequestRef.current) break;

          setDiagnosisError(evt.error ?? 'Diagnosis failed');
          setDiagnosing(false);
          break;
        }
        case 'diagnose_interrupted': {
          const evt = data as {
            sessionId?: string;
          };
          if (!evt.sessionId || evt.sessionId !== diagnoseSessionRef.current) break;
          setDiagnosing(false);
          break;
        }
        case 'diagnose_session_closed': {
          const evt = data as {
            sessionId?: string;
          };
          if (!evt.sessionId || evt.sessionId !== diagnoseSessionRef.current) break;
          setDiagnoseSessionId(null);
          setDiagnoseRequestId(null);
          setDiagnosing(false);
          break;
        }
      }
    },
    [notify]
  );

  useSSE(handleSSE, loadAllData, stack);
  useFavicon(stackStatus, !!stackError);

  // Notify parent when the raw stack status changes so App can track which stack is running.
  useEffect(() => {
    if (stackStatus && onStackStatusChange) {
      onStackStatusChange(stackStatus);
    }
  }, [stackStatus, onStackStatusChange]);

  // When another stack is running, treat this stack as stopped regardless of what the
  // backend reports (leftover services like ClickHouse may still be up from a previous session).
  const effectiveStatus = otherRunningStack && stackStatus === 'running' ? 'stopped' : stackStatus;

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

  useEffect(() => {
    fetchJSON<AIProviderInfo[]>('/ai/providers')
      .then(data => {
        setProviders(data);
        const preferred = data.find(p => p.default && p.available) ?? data.find(p => p.available) ?? data[0];
        if (preferred) {
          setSelectedProvider(preferred.id);
        }
      })
      .catch(() => {
        setProviders([]);
      });
  }, [fetchJSON]);

  const handleDiagnose = useCallback(
    (serviceName: string) => {
      const provider = selectedProvider || 'claude';
      const requestId = createRequestId();

      setDiagnoseService(serviceName);
      setDiagnoseRequestId(requestId);
      setDiagnoseSessionId(null);
      setDiagnosing(true);
      setDiagnosis(null);
      setDiagnosisError(null);
      setThinkingText('');
      setAnswerText('');
      setActivityText('');
      setCompletedTurns([]);
      setCurrentTurnPrompt(undefined);

      postDiagnoseStart<{ sessionId: string; requestId: string }>(serviceName, {
        provider,
        requestId,
      })
        .then(data => {
          setDiagnoseSessionId(data.sessionId);
        })
        .catch(err => {
          setDiagnosisError(err instanceof Error ? err.message : String(err));
          setDiagnosing(false);
        });
    },
    [postDiagnoseStart, selectedProvider]
  );

  const handleDiagnoseFollowUp = useCallback(
    (prompt: string) => {
      if (!diagnoseService || !diagnoseSessionId) return;

      const requestId = createRequestId();
      setDiagnoseRequestId(requestId);
      setCompletedTurns(prev => [
        ...prev,
        { prompt: currentTurnPrompt, thinking: thinkingText, activity: activityText, answer: answerText },
      ]);
      setCurrentTurnPrompt(prompt);
      setDiagnosing(true);
      setDiagnosis(null);
      setDiagnosisError(null);
      setThinkingText('');
      setAnswerText('');
      setActivityText('');

      postDiagnoseMessage<{ sessionId: string }>(diagnoseService, {
        sessionId: diagnoseSessionId,
        provider: selectedProvider,
        prompt,
        requestId,
      }).catch(err => {
        setDiagnosisError(err instanceof Error ? err.message : String(err));
        setDiagnosing(false);
      });
    },
    [
      activityText,
      answerText,
      currentTurnPrompt,
      diagnoseService,
      diagnoseSessionId,
      postDiagnoseMessage,
      selectedProvider,
      thinkingText,
    ]
  );

  const handleDiagnoseInterrupt = useCallback(() => {
    if (!diagnoseService || !diagnoseSessionId) return;

    postDiagnoseInterrupt<{ status: string }>(diagnoseService, {
      sessionId: diagnoseSessionId,
      requestId: diagnoseRequestId,
    }).catch(err => setDiagnosisError(err instanceof Error ? err.message : String(err)));
  }, [diagnoseRequestId, diagnoseService, diagnoseSessionId, postDiagnoseInterrupt]);

  const handleCloseDiagnosis = useCallback(() => {
    if (diagnoseService && diagnoseSessionId) {
      deleteDiagnoseSession(diagnoseService, diagnoseSessionId).catch(() => undefined);
    }

    setDiagnoseService(null);
    setDiagnoseSessionId(null);
    setDiagnoseRequestId(null);
    setDiagnosing(false);
    setDiagnosis(null);
    setDiagnosisError(null);
    setThinkingText('');
    setAnswerText('');
    setActivityText('');
    setCompletedTurns([]);
    setCurrentTurnPrompt(undefined);
  }, [deleteDiagnoseSession, diagnoseService, diagnoseSessionId]);

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

  // When the stack is effectively stopped (another stack owns the running state),
  // override all service statuses so leftover port-conflicts don't show green dots.
  const displayServices =
    effectiveStatus === 'stopped' && otherRunningStack
      ? services.map(s => ({ ...s, status: 'stopped', health: 'unknown', pid: 0, uptime: '' }))
      : services;

  const leftSidebarContent = (
    <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${leftCollapsed ? 'invisible' : ''}`}>
      <div className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Services</div>
      {displayServices.map(svc => (
        <ServiceCard
          key={svc.name}
          service={svc}
          selected={activeTab === svc.name}
          onSelect={() => handleSelectService(svc.name)}
          stack={stack}
          showDiagnose={diagnoseAvailable}
          onDiagnose={() => handleDiagnose(svc.name)}
          disableActions={!!otherRunningStack && stackStatus !== 'running'}
        />
      ))}
    </div>
  );

  const rightSidebarContent = (
    <div className={`flex h-full flex-col gap-3 overflow-y-auto p-3 ${rightCollapsed ? 'invisible' : ''}`}>
      <SidebarSection
        title="Config"
        storageKey="xcli:sidebar:config"
        action={onNavigateConfig ? { label: 'Manage', onClick: onNavigateConfig } : undefined}
      >
        {isLabStack ? <ConfigPanel config={config} services={services} /> : <XatuConfigPanel config={xatuConfig} />}
      </SidebarSection>
      {showCbtOverrides && (
        <SidebarSection
          title="CBT Overrides"
          storageKey="xcli:sidebar:overrides"
          action={onNavigateOverrides ? { label: 'Manage', onClick: onNavigateOverrides } : undefined}
        >
          <CBTOverridesGlance overrides={overrides} />
        </SidebarSection>
      )}
      {showGitRepos && (
        <SidebarSection title="Git Status" storageKey="xcli:sidebar:git" defaultOpen={false}>
          <GitStatus repos={repos} />
        </SidebarSection>
      )}
    </div>
  );

  const mainPanelContent = (
    <>
      {effectiveStatus === null ? (
        <Spinner centered />
      ) : effectiveStatus === 'starting' || effectiveStatus === 'stopping' ? (
        <StackProgress
          phases={derivePhaseStates(
            progressPhases,
            stackError,
            effectiveStatus === 'stopping' ? stopPhasesFor(isLabStack) : bootPhasesFor(isLabStack)
          )}
          error={stackError}
          title={effectiveStatus === 'stopping' ? 'Stopping Stack' : 'Booting Stack'}
          onCancel={effectiveStatus === 'starting' ? handleCancelBoot : undefined}
        />
      ) : stackError ? (
        progressPhases.length > 0 ? (
          <StackProgress
            phases={derivePhaseStates(progressPhases, stackError, bootPhasesFor(isLabStack))}
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
      ) : (effectiveStatus === 'running' && services.some(s => s.status === 'running')) || openTabs.length > 0 ? (
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
          {otherRunningStack ? (
            <p className="text-xs text-text-disabled">{otherRunningStack} stack is already running — stop it first</p>
          ) : (
            <button
              onClick={handleStackAction}
              className="rounded-md bg-success/20 px-4 py-2 text-sm font-medium text-success transition-colors hover:bg-success/30"
            >
              Boot Stack
            </button>
          )}
        </div>
      )}
    </>
  );

  return (
    <div className="flex h-dvh flex-col">
      <Header
        services={displayServices}
        onNavigateConfig={onNavigateConfig}
        stackStatus={effectiveStatus}
        onStackAction={handleStackAction}
        currentPhase={progressPhases.length > 0 ? progressPhases[progressPhases.length - 1].message : undefined}
        notificationsEnabled={notificationsEnabled}
        onToggleNotifications={toggleNotifications}
        activeStack={stack}
        availableStacks={availableStacks}
        onSwitchStack={onSwitchStack}
        otherRunningStack={otherRunningStack}
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
      {(diagnoseService || diagnosis || diagnosisError || diagnosing) && (
        <DiagnosisPanel
          serviceName={diagnoseService ?? ''}
          providers={providers}
          selectedProvider={selectedProvider}
          onProviderChange={setSelectedProvider}
          sessionId={diagnoseSessionId}
          thinkingText={thinkingText}
          activityText={activityText}
          answerText={answerText}
          completedTurns={completedTurns}
          currentTurnPrompt={currentTurnPrompt}
          diagnosis={diagnosis}
          error={diagnosisError}
          loading={diagnosing}
          canInterrupt={!!selectedProviderInfo?.capabilities.interrupt}
          canInteract={!!selectedProviderInfo?.capabilities.sessions}
          onInterrupt={handleDiagnoseInterrupt}
          onSendFollowUp={handleDiagnoseFollowUp}
          onClose={handleCloseDiagnosis}
          onRetry={() => {
            if (diagnoseService) handleDiagnose(diagnoseService);
          }}
        />
      )}
    </div>
  );
}
