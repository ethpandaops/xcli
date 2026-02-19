export const BOOT_PHASES = [
  { id: 'prerequisites', label: 'Validate Prerequisites' },
  { id: 'build_xatu_cbt', label: 'Build Xatu-CBT' },
  { id: 'infrastructure', label: 'Start Infrastructure' },
  { id: 'build_services', label: 'Build Services' },
  { id: 'network_setup', label: 'Network Setup' },
  { id: 'generate_configs', label: 'Generate Configurations' },
  { id: 'build_cbt_api', label: 'Build CBT API' },
  { id: 'start_services', label: 'Start Services' },
];

export const STOP_PHASES = [
  { id: 'stop_services', label: 'Stop Services' },
  { id: 'cleanup_orphans', label: 'Clean Up Processes' },
  { id: 'clean_logs', label: 'Clean Log Files' },
  { id: 'stop_infrastructure', label: 'Stop Infrastructure' },
];

export interface PhaseState {
  id: string;
  label: string;
  status: 'pending' | 'done' | 'active' | 'error';
  message?: string;
}

interface StackProgressProps {
  phases: PhaseState[];
  error: string | null;
  title?: string;
  onRetry?: () => void;
  onCancel?: () => void;
}

export function derivePhaseStates(
  receivedPhases: { phase: string; message: string }[],
  error: string | null,
  phaseDefs: { id: string; label: string }[] = BOOT_PHASES
): PhaseState[] {
  if (receivedPhases.length === 0) {
    return phaseDefs.map(p => ({
      ...p,
      status: 'pending' as const,
    }));
  }

  const lastReceived = receivedPhases[receivedPhases.length - 1];
  const receivedIds = new Set(receivedPhases.map(p => p.phase));
  const messageMap = new Map(receivedPhases.map(p => [p.phase, p.message]));

  // "complete" means all phases done
  if (lastReceived.phase === 'complete') {
    return phaseDefs.map(p => ({
      ...p,
      status: 'done' as const,
      message: messageMap.get(p.id),
    }));
  }

  const lastIdx = phaseDefs.findIndex(p => p.id === lastReceived.phase);

  return phaseDefs.map((p, i) => {
    const isReceived = receivedIds.has(p.id);

    if (error && p.id === lastReceived.phase) {
      return { ...p, status: 'error' as const, message: error };
    }

    if (i < lastIdx || (isReceived && i !== lastIdx)) {
      return { ...p, status: 'done' as const, message: messageMap.get(p.id) };
    }

    if (i === lastIdx) {
      return {
        ...p,
        status: 'active' as const,
        message: messageMap.get(p.id),
      };
    }

    return { ...p, status: 'pending' as const };
  });
}

export default function StackProgress({
  phases,
  error,
  title = 'Booting Stack',
  onRetry,
  onCancel,
}: StackProgressProps) {
  const doneCount = phases.filter(p => p.status === 'done').length;
  const activePhase = phases.find(p => p.status === 'active');
  const hasError = phases.some(p => p.status === 'error');
  const progress = hasError
    ? (doneCount / phases.length) * 100
    : activePhase
      ? ((doneCount + 0.5) / phases.length) * 100
      : (doneCount / phases.length) * 100;

  return (
    <div className="flex h-full items-center justify-center">
      {/* Radial backdrop glow */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div
          className={`absolute top-1/2 left-1/2 size-[600px] -translate-x-1/2 -translate-y-1/2 rounded-full opacity-[0.07] blur-[120px] ${
            hasError ? 'bg-error' : activePhase ? 'bg-success' : 'bg-accent'
          }`}
        />
      </div>

      <div className="relative w-full max-w-sm">
        {/* Header */}
        <div className="flex items-baseline justify-between">
          <h2 className="text-xs/4 font-semibold tracking-widest text-text-muted uppercase">{title}</h2>
          <span className="font-mono text-xs/4 text-text-disabled">
            {doneCount}/{phases.length}
          </span>
        </div>

        {/* Progress rail */}
        <div className="mt-3 h-px w-full bg-border">
          <div
            className={`h-full transition-all duration-700 ease-out ${hasError ? 'bg-error' : 'bg-success'}`}
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* Timeline */}
        <div className="mt-6">
          {phases.map((phase, i) => {
            const stepNum = String(i + 1).padStart(2, '0');

            return (
              <div key={phase.id} className="relative flex items-start gap-3 pb-5 last:pb-0">
                {/* Vertical connector */}
                {i < phases.length - 1 && (
                  <div
                    className={`absolute top-5 left-[11px] h-full w-px transition-colors duration-300 ${
                      phase.status === 'done' ? 'bg-success/30' : 'bg-border'
                    }`}
                  />
                )}

                {/* Step indicator */}
                <div className="relative z-10 flex size-[22px] shrink-0 items-center justify-center">
                  {phase.status === 'done' && (
                    <span className="flex size-[22px] items-center justify-center rounded-full bg-success/15">
                      <svg
                        className="size-3 text-success"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth={3}
                      >
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                      </svg>
                    </span>
                  )}
                  {phase.status === 'active' && (
                    <span className="relative flex size-[22px] items-center justify-center">
                      <span className="absolute size-[22px] animate-ping rounded-full bg-warning/20" />
                      <span className="absolute size-[22px] rounded-full bg-warning/10" />
                      <span className="relative size-2 rounded-full bg-warning shadow-[0_0_10px_var(--color-warning)]" />
                    </span>
                  )}
                  {phase.status === 'error' && (
                    <span className="flex size-[22px] items-center justify-center rounded-full bg-error/15">
                      <svg
                        className="size-3 text-error"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth={3}
                      >
                        <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </span>
                  )}
                  {phase.status === 'pending' && (
                    <span className="flex size-[22px] items-center justify-center">
                      <span className="size-1.5 rounded-full bg-border" />
                    </span>
                  )}
                </div>

                {/* Content */}
                <div className="min-w-0 flex-1 pt-0.5">
                  <div className="flex items-baseline gap-2">
                    <span
                      className={`font-mono text-xs/4 ${
                        phase.status === 'done'
                          ? 'text-success/50'
                          : phase.status === 'active'
                            ? 'text-warning/60'
                            : phase.status === 'error'
                              ? 'text-error/60'
                              : 'text-border'
                      }`}
                    >
                      {stepNum}
                    </span>
                    <span
                      className={`text-sm/5 font-medium ${
                        phase.status === 'done'
                          ? 'text-success/80'
                          : phase.status === 'active'
                            ? 'text-text-primary'
                            : phase.status === 'error'
                              ? 'text-error'
                              : 'text-text-disabled'
                      }`}
                    >
                      {phase.label}
                    </span>
                  </div>

                  {/* Active phase message */}
                  {phase.status === 'active' && phase.message && (
                    <p className="mt-1 truncate font-mono text-xs/4 text-warning/50">{phase.message}</p>
                  )}

                  {/* Error message */}
                  {phase.status === 'error' && error && (
                    <p className="mt-1 truncate font-mono text-xs/4 text-error/60">{error}</p>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {/* Actions */}
        {error && onRetry && (
          <button
            onClick={onRetry}
            className="mt-6 rounded-xs bg-success/10 px-4 py-2 text-xs/4 font-medium text-success ring-1 ring-success/20 transition-colors hover:bg-success/20"
          >
            Retry
          </button>
        )}

        {!error && onCancel && (
          <button onClick={onCancel} className="mt-5 text-xs/4 text-text-disabled transition-colors hover:text-error">
            Cancel Boot
          </button>
        )}
      </div>
    </div>
  );
}
