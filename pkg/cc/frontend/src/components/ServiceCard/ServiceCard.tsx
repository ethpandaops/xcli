import { useState, useCallback } from 'react';
import type { ServiceInfo } from '@/types';
import { useAPI } from '@/hooks/useAPI';

interface ServiceCardProps {
  service: ServiceInfo;
  selected: boolean;
  onSelect: () => void;
}

const healthIcons: Record<string, { color: string; title: string; path: string }> = {
  healthy: {
    color: 'text-success',
    title: 'Healthy',
    path: 'M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
  },
  unhealthy: {
    color: 'text-error',
    title: 'Unhealthy',
    path: 'm9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
  },
  degraded: {
    color: 'text-warning',
    title: 'Degraded',
    path: 'M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z',
  },
  unknown: {
    color: 'text-text-disabled',
    title: 'Unknown',
    path: 'M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 5.25h.008v.008H12v-.008Z',
  },
};

export default function ServiceCard({ service, selected, onSelect }: ServiceCardProps) {
  const { postAction } = useAPI();
  const [loading, setLoading] = useState<string | null>(null);

  const doAction = useCallback(
    async (action: string) => {
      setLoading(action);

      try {
        await postAction(service.name, action);
      } catch (err) {
        console.error(`${action} failed:`, err);
      } finally {
        setLoading(null);
      }
    },
    [postAction, service.name]
  );

  const isRunning = service.status === 'running';
  const isDocker = service.name === 'prometheus' || service.name === 'grafana';

  // Append /docs for cbt-api services when opening in browser
  const openUrl = service.url && service.name.startsWith('cbt-api-') ? `${service.url}/docs` : service.url;

  const actionLabels: Record<string, string> = {
    start: 'Starting',
    stop: 'Stopping',
    restart: 'Restarting',
    rebuild: 'Rebuilding',
  };

  const statusColor = isRunning ? 'bg-success' : service.status === 'crashed' ? 'bg-error' : 'bg-text-disabled';

  const icon = healthIcons[service.health] ?? healthIcons.unknown;

  return (
    <div
      onClick={onSelect}
      className={`group relative cursor-pointer rounded-xs transition-colors ${
        selected ? 'bg-accent/10 ring-1 ring-accent/30' : 'hover:bg-white/[0.03]'
      }`}
    >
      {loading && (
        <div className="absolute inset-0 z-10 flex items-center justify-center overflow-hidden rounded-xs bg-overlay/80">
          <div className="flex items-center gap-2">
            <div className="size-3.5 animate-spin rounded-full border-2 border-warning border-t-transparent" />
            <span className="text-xs/4 font-medium text-warning">{actionLabels[loading] ?? loading}...</span>
          </div>
        </div>
      )}

      <div className="px-3 py-2.5">
        {/* Top row: status + name + health */}
        <div className="flex items-center gap-2.5">
          <span className={`size-1.5 shrink-0 rounded-full ${statusColor}`} />
          <span className="min-w-0 flex-1 truncate text-sm/5 font-medium text-text-secondary">{service.name}</span>
          <svg
            className={`size-3.5 shrink-0 ${icon.color}`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.5}
          >
            <title>{icon.title}</title>
            <path strokeLinecap="round" strokeLinejoin="round" d={icon.path} />
          </svg>
        </div>

        {/* Meta row: uptime, PID, open link */}
        <div className="mt-1 flex items-center gap-2 pl-4 text-xs/4 text-text-disabled">
          {service.uptime && <span>{service.uptime}</span>}
          {service.pid > 0 && <span>PID {service.pid}</span>}
          {isRunning && service.url && service.name !== 'lab-backend' && (
            <a
              href={openUrl}
              target="_blank"
              rel="noreferrer"
              onClick={e => e.stopPropagation()}
              className="inline-flex items-center gap-0.5 text-accent-light/70 hover:text-accent-light"
            >
              open
              <svg className="size-2.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M13.5 6H5.25A2.25 2.25 0 0 0 3 8.25v10.5A2.25 2.25 0 0 0 5.25 21h10.5A2.25 2.25 0 0 0 18 18.75V10.5m-4.5-6H18m0 0v4.5m0-4.5-7.5 7.5"
                />
              </svg>
            </a>
          )}
        </div>

        {/* Actions — visible on hover */}
        <div
          className={`mt-2 flex gap-1 overflow-hidden transition-all ${
            loading ? 'invisible' : 'h-0 opacity-0 group-hover:h-6 group-hover:opacity-100'
          }`}
        >
          {!isRunning && <ActionBtn label="Start" onClick={() => doAction('start')} />}
          {isRunning && (
            <>
              <ActionBtn label="Stop" onClick={() => doAction('stop')} />
              <ActionBtn label="Restart" onClick={() => doAction('restart')} />
            </>
          )}
          {!isDocker && <ActionBtn label="Rebuild" onClick={() => doAction('rebuild')} />}
        </div>
      </div>
    </div>
  );
}

function ActionBtn({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      onClick={e => {
        e.stopPropagation();
        onClick();
      }}
      className="rounded-xs bg-white/5 px-2 py-0.5 text-xs/4 text-text-tertiary transition-colors hover:bg-white/10 hover:text-text-primary"
    >
      {label}
    </button>
  );
}
