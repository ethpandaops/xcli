import { useState, useRef, useEffect } from 'react';
import type { ServiceInfo } from '@/types';
import { useTheme } from '@/hooks/useTheme';

interface HeaderProps {
  services: ServiceInfo[];
  onNavigateConfig?: () => void;
  stackStatus: string | null;
  onStackAction: () => void;
  currentPhase?: string;
  notificationsEnabled: boolean;
  onToggleNotifications: () => void;
  activeStack: string;
  availableStacks: string[];
  onSwitchStack: (stack: string) => void;
  otherRunningStack?: string | null;
}

export default function Header({
  services,
  onNavigateConfig,
  stackStatus,
  onStackAction,
  currentPhase,
  notificationsEnabled,
  onToggleNotifications,
  activeStack,
  availableStacks,
  onSwitchStack,
  otherRunningStack,
}: HeaderProps) {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const { theme, cycleTheme } = useTheme();

  const running = services.filter(s => s.status === 'running').length;
  const healthy = services.filter(s => s.health === 'healthy').length;

  const isBusy = !stackStatus || stackStatus === 'starting' || stackStatus === 'stopping';
  const isRunning = stackStatus === 'running';
  const bootBlocked = !isRunning && !!otherRunningStack;
  // Close dropdown on outside click
  useEffect(() => {
    if (!dropdownOpen) return;

    const handleClick = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClick);

    return () => document.removeEventListener('mousedown', handleClick);
  }, [dropdownOpen]);

  let buttonLabel = 'Boot Stack';
  let buttonClass = 'flex items-center gap-1.5 rounded-xs px-3 py-1.5 text-xs/4 font-medium transition-colors ';

  if (stackStatus === 'starting') {
    buttonLabel = currentPhase ? `Starting: ${currentPhase}` : 'Starting...';
    buttonClass += 'cursor-not-allowed bg-warning/10 text-warning ring-1 ring-warning/20';
  } else if (stackStatus === 'stopping') {
    buttonLabel = currentPhase ? `Stopping: ${currentPhase}` : 'Stopping...';
    buttonClass += 'cursor-not-allowed bg-warning/10 text-warning ring-1 ring-warning/20';
  } else if (isRunning) {
    buttonLabel = 'Stop Stack';
    buttonClass += 'bg-error/10 text-error ring-1 ring-error/20 hover:bg-error/20';
  } else if (bootBlocked) {
    buttonLabel = `${otherRunningStack} is running`;
    buttonClass += 'cursor-not-allowed bg-surface-light text-text-disabled ring-1 ring-border';
  } else {
    buttonLabel = 'Boot Stack';
    buttonClass += 'bg-success/10 text-success ring-1 ring-success/20 hover:bg-success/20';
  }

  const themeTitle = theme === 'system' ? 'Theme: System' : theme === 'light' ? 'Theme: Light' : 'Theme: Dark';

  return (
    <header className="flex items-center justify-between border-b border-border bg-surface px-5 py-2.5">
      <div className="flex items-center gap-3">
        <h1 className="group text-sm/5 font-bold tracking-tight text-text-primary">
          <span className="inline-block origin-bottom transition-transform group-hover:animate-wobble">🍆</span> xcli
        </h1>

        <div className="h-4 w-px bg-border" />

        {/* Stack switcher */}
        <div className="relative" ref={dropdownRef}>
          <button
            onClick={() => setDropdownOpen(v => !v)}
            className={`flex items-center gap-1.5 rounded-xs border px-2.5 py-1 text-xs/4 font-medium transition-colors ${
              dropdownOpen
                ? 'border-accent/40 bg-accent/10 text-accent-light'
                : 'border-border bg-surface-light text-text-secondary hover:border-text-disabled hover:text-text-primary'
            }`}
          >
            {/* Stack icon */}
            <svg
              className="size-3.5 shrink-0 text-text-muted"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6.429 9.75 2.25 12l4.179 2.25m0-4.5 5.571 3 5.571-3m-11.142 0L2.25 7.5 12 2.25l9.75 5.25-4.179 2.25m0 0L21.75 12l-4.179 2.25m0 0 4.179 2.25L12 21.75l-9.75-5.25 4.179-2.25m11.142 0-5.571 3-5.571-3"
              />
            </svg>
            <span>{activeStack}</span>
            <svg
              className={`size-3 text-text-disabled transition-transform ${dropdownOpen ? 'rotate-180' : ''}`}
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={2}
              stroke="currentColor"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
            </svg>
          </button>

          {dropdownOpen && (
            <div className="absolute top-full left-0 z-50 mt-1 min-w-36 rounded-xs border border-border bg-surface-light py-1 shadow-lg">
              {availableStacks.map(stack => (
                <button
                  key={stack}
                  onClick={() => {
                    onSwitchStack(stack);
                    setDropdownOpen(false);
                  }}
                  className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs/4 transition-colors ${
                    stack === activeStack
                      ? 'bg-accent/10 font-medium text-accent-light'
                      : 'text-text-secondary hover:bg-hover/5'
                  }`}
                >
                  {stack === activeStack && <span className="size-1.5 rounded-full bg-accent" />}
                  {stack}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="flex items-center gap-4 text-xs/4 text-text-muted">
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-1.5">
            <span className={`size-1.5 rounded-full ${running > 0 ? 'bg-success' : 'bg-text-disabled'}`} />
            {running}/{services.length}
          </span>
          <span className="flex items-center gap-1.5">
            <span className={`size-1.5 rounded-full ${healthy > 0 ? 'bg-info' : 'bg-text-disabled'}`} />
            {healthy} healthy
          </span>
        </div>

        <div className="h-4 w-px bg-border" />

        {otherRunningStack && isRunning && (
          <span
            className="rounded-xs bg-warning/10 px-2 py-1 text-xs/4 text-warning ring-1 ring-warning/20"
            title={`${otherRunningStack} stack is also running`}
          >
            {otherRunningStack} is running
          </span>
        )}

        <button
          onClick={onStackAction}
          disabled={isBusy || bootBlocked}
          className={buttonClass}
          title={
            bootBlocked
              ? `${otherRunningStack} stack is already running — stop it first`
              : isRunning
                ? 'Stop all services'
                : 'Boot the full stack'
          }
        >
          {isBusy && (
            <svg className="size-3 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-20" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
          )}
          <span className="max-w-48 truncate">{buttonLabel}</span>
        </button>

        <button
          onClick={onToggleNotifications}
          className={`rounded-xs p-1.5 transition-colors hover:bg-hover/5 ${
            notificationsEnabled ? 'text-warning' : 'text-text-muted hover:text-text-secondary'
          }`}
          title={notificationsEnabled ? 'Disable notifications' : 'Enable notifications'}
        >
          {notificationsEnabled ? (
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M14.857 17.082a23.848 23.848 0 0 0 5.454-1.31A8.967 8.967 0 0 1 18 9.75V9A6 6 0 0 0 6 9v.75a8.967 8.967 0 0 1-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 0 1-5.714 0m5.714 0a3 3 0 1 1-5.714 0"
              />
            </svg>
          ) : (
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9.143 17.082a24.248 24.248 0 0 0 5.714 0m-5.714 0a3 3 0 1 0 5.714 0M9.143 17.082a23.848 23.848 0 0 1-5.454-1.31A8.967 8.967 0 0 0 6 9.75V9a6 6 0 0 1 12 0v.75a8.967 8.967 0 0 0 2.312 6.022 23.848 23.848 0 0 1-5.455 1.31"
              />
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 3l18 18" />
            </svg>
          )}
        </button>

        {/* Theme toggle */}
        <button
          onClick={cycleTheme}
          className="rounded-xs p-1.5 text-text-muted transition-colors hover:bg-hover/5 hover:text-text-secondary"
          title={themeTitle}
        >
          {theme === 'system' ? (
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9 17.25v1.007a3 3 0 0 1-.879 2.122L7.5 21h9l-.621-.621A3 3 0 0 1 15 18.257V17.25m6-12V15a2.25 2.25 0 0 1-2.25 2.25H5.25A2.25 2.25 0 0 1 3 15V5.25A2.25 2.25 0 0 1 5.25 3h13.5A2.25 2.25 0 0 1 21 5.25Z"
              />
            </svg>
          ) : theme === 'light' ? (
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M12 3v2.25m6.364.386-1.591 1.591M21 12h-2.25m-.386 6.364-1.591-1.591M12 18.75V21m-4.773-4.227-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0Z"
              />
            </svg>
          ) : (
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M21.752 15.002A9.72 9.72 0 0 1 18 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 0 0 3 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 0 0 9.002-5.998Z"
              />
            </svg>
          )}
        </button>

        {onNavigateConfig && (
          <button
            onClick={onNavigateConfig}
            className="rounded-xs p-1.5 text-text-muted transition-colors hover:bg-hover/5 hover:text-text-secondary"
            title="Config Management"
          >
            <svg className="size-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
              />
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
            </svg>
          </button>
        )}
      </div>
    </header>
  );
}
