export const BOOT_PHASES = [
  { id: "prerequisites", label: "Validate Prerequisites" },
  { id: "build_xatu_cbt", label: "Build Xatu-CBT" },
  { id: "infrastructure", label: "Start Infrastructure" },
  { id: "build_services", label: "Build Services" },
  { id: "network_setup", label: "Network Setup" },
  { id: "generate_configs", label: "Generate Configurations" },
  { id: "build_cbt_api", label: "Build CBT API" },
  { id: "start_services", label: "Start Services" },
];

export const STOP_PHASES = [
  { id: "stop_services", label: "Stop Services" },
  { id: "cleanup_orphans", label: "Clean Up Processes" },
  { id: "clean_logs", label: "Clean Log Files" },
  { id: "stop_infrastructure", label: "Stop Infrastructure" },
];

export interface PhaseState {
  id: string;
  label: string;
  status: "pending" | "done" | "active" | "error";
  message?: string;
}

interface StackProgressProps {
  phases: PhaseState[];
  error: string | null;
  title?: string;
  onRetry?: () => void;
}

export function derivePhaseStates(
  receivedPhases: { phase: string; message: string }[],
  error: string | null,
  phaseDefs: { id: string; label: string }[] = BOOT_PHASES,
): PhaseState[] {
  if (receivedPhases.length === 0) {
    return phaseDefs.map((p) => ({
      ...p,
      status: "pending" as const,
    }));
  }

  const lastReceived = receivedPhases[receivedPhases.length - 1];
  const receivedIds = new Set(receivedPhases.map((p) => p.phase));
  const messageMap = new Map(receivedPhases.map((p) => [p.phase, p.message]));

  // "complete" means all phases done
  if (lastReceived.phase === "complete") {
    return phaseDefs.map((p) => ({
      ...p,
      status: "done" as const,
      message: messageMap.get(p.id),
    }));
  }

  const lastIdx = phaseDefs.findIndex((p) => p.id === lastReceived.phase);

  return phaseDefs.map((p, i) => {
    const isReceived = receivedIds.has(p.id);

    if (error && p.id === lastReceived.phase) {
      return { ...p, status: "error" as const, message: error };
    }

    if (i < lastIdx || (isReceived && i !== lastIdx)) {
      return { ...p, status: "done" as const, message: messageMap.get(p.id) };
    }

    if (i === lastIdx) {
      return {
        ...p,
        status: "active" as const,
        message: messageMap.get(p.id),
      };
    }

    return { ...p, status: "pending" as const };
  });
}

export default function StackProgress({
  phases,
  error,
  title = "Booting Stack",
  onRetry,
}: StackProgressProps) {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="w-full max-w-md rounded-lg bg-surface p-6">
        <h2 className="mb-6 text-sm font-semibold uppercase tracking-wider text-gray-400">
          {title}
        </h2>
        <div className="relative">
          {phases.map((phase, i) => (
            <div key={phase.id} className="relative flex gap-3 pb-6 last:pb-0">
              {/* Timeline line */}
              {i < phases.length - 1 && (
                <div
                  className={`absolute left-[9px] top-5 h-full w-px ${
                    phase.status === "done"
                      ? "bg-emerald-500/50"
                      : "bg-gray-700"
                  }`}
                />
              )}

              {/* Status icon */}
              <div className="relative z-10 mt-0.5 flex size-[18px] shrink-0 items-center justify-center">
                {phase.status === "done" && (
                  <svg
                    className="size-[18px] text-emerald-500"
                    fill="none"
                    viewBox="0 0 24 24"
                    strokeWidth={2.5}
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                )}
                {phase.status === "active" && (
                  <span className="relative flex size-[18px] items-center justify-center">
                    <span className="absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-30" />
                    <span className="relative inline-flex size-2.5 rounded-full bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.5)]" />
                  </span>
                )}
                {phase.status === "error" && (
                  <svg
                    className="size-[18px] text-red-500"
                    fill="none"
                    viewBox="0 0 24 24"
                    strokeWidth={2.5}
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                )}
                {phase.status === "pending" && (
                  <span className="inline-flex size-2 rounded-full bg-gray-600" />
                )}
              </div>

              {/* Content */}
              <div className="min-w-0 flex-1">
                <p
                  className={`text-sm font-medium ${
                    phase.status === "done"
                      ? "text-emerald-400"
                      : phase.status === "active"
                        ? "text-amber-300"
                        : phase.status === "error"
                          ? "text-red-400"
                          : "text-gray-500"
                  }`}
                >
                  {phase.label}
                </p>
                {phase.status === "error" && error && (
                  <p className="mt-1 truncate font-mono text-xs text-red-400/70">
                    {error}
                  </p>
                )}
              </div>
            </div>
          ))}
        </div>

        {error && onRetry && (
          <button
            onClick={onRetry}
            className="mt-6 w-full rounded-md bg-emerald-500/20 px-4 py-2 text-sm font-medium text-emerald-400 transition-colors hover:bg-emerald-500/30"
          >
            Retry
          </button>
        )}
      </div>
    </div>
  );
}
