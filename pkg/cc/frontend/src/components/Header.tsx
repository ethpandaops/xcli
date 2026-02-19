import type { ServiceInfo, InfraInfo } from "../types";

interface HeaderProps {
  services: ServiceInfo[];
  infrastructure: InfraInfo[];
  mode: string;
  onNavigateConfig?: () => void;
  stackStatus: string | null;
  onStackAction: () => void;
  currentPhase?: string;
}

export default function Header({
  services,
  infrastructure,
  mode,
  onNavigateConfig,
  stackStatus,
  onStackAction,
  currentPhase,
}: HeaderProps) {
  const running = services.filter((s) => s.status === "running").length;
  const healthy = services.filter((s) => s.health === "healthy").length;
  const infraRunning = infrastructure.filter(
    (i) => i.status === "running",
  ).length;

  const isBusy = !stackStatus || stackStatus === "starting" || stackStatus === "stopping";
  const isRunning = stackStatus === "running";

  let buttonLabel = "Boot Stack";
  let buttonClass =
    "rounded-md px-3 py-1.5 text-xs font-medium transition-colors ";

  if (stackStatus === "starting") {
    buttonLabel = currentPhase ? `Starting: ${currentPhase}` : "Starting...";
    buttonClass += "cursor-not-allowed bg-amber-500/20 text-amber-400";
  } else if (stackStatus === "stopping") {
    buttonLabel = currentPhase ? `Stopping: ${currentPhase}` : "Stopping...";
    buttonClass += "cursor-not-allowed bg-amber-500/20 text-amber-400";
  } else if (isRunning) {
    buttonLabel = "Stop Stack";
    buttonClass += "bg-red-500/20 text-red-400 hover:bg-red-500/30";
  } else {
    buttonLabel = "Boot Stack";
    buttonClass += "bg-emerald-500/20 text-emerald-400 hover:bg-emerald-500/30";
  }

  return (
    <header className="flex items-center justify-between border-b border-border bg-surface px-6 py-3">
      <div className="flex items-center gap-4">
        <h1 className="group text-lg/6 font-bold tracking-tight text-white">
          <span className="inline-block origin-bottom transition-transform group-hover:animate-wobble">🍆</span> xcli Command Center
        </h1>
        <span className="rounded-xs bg-indigo-500/20 px-2 py-0.5 text-xs/4 font-medium text-indigo-400">
          {mode || "unknown"}
        </span>
      </div>

      <div className="flex items-center gap-6 text-sm/5 text-gray-400">
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-emerald-500" />
          <span>
            {running}/{services.length} services
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-sky-500" />
          <span>{healthy} healthy</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-violet-500" />
          <span>
            {infraRunning}/{infrastructure.length} infra
          </span>
        </div>
        <button
          onClick={onStackAction}
          disabled={isBusy}
          className={buttonClass}
          title={isRunning ? "Stop all services" : "Boot the full stack"}
        >
          {isBusy && (
            <svg
              className="-ml-0.5 mr-1.5 inline size-3 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
              />
            </svg>
          )}
          {buttonLabel}
        </button>
        {onNavigateConfig && (
          <button
            onClick={onNavigateConfig}
            className="rounded-xs p-1.5 text-gray-400 transition-colors hover:bg-white/10 hover:text-white"
            title="Config Management"
          >
            <svg
              className="size-5"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
              />
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"
              />
            </svg>
          </button>
        )}
      </div>
    </header>
  );
}
