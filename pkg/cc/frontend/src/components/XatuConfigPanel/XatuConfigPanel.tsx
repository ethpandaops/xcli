import type { XatuConfigResponse } from '@/types';

interface XatuConfigPanelProps {
  config: XatuConfigResponse | null;
}

export default function XatuConfigPanel({ config }: XatuConfigPanelProps) {
  if (!config) {
    return <div className="py-2 text-xs/4 text-text-muted">No config loaded</div>;
  }

  const envEntries = Object.entries(config.envOverrides ?? {});

  return (
    <div className="flex flex-col gap-3">
      {/* Profiles */}
      <div>
        <div className="mb-1.5 text-xs/4 font-medium text-text-muted">Profiles</div>
        {config.profiles && config.profiles.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {config.profiles.map(profile => (
              <span
                key={profile}
                className="rounded-xs bg-accent/15 px-2 py-0.5 text-xs/4 font-medium text-accent-light"
              >
                {profile}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-xs/4 text-text-disabled">No profiles active</span>
        )}
      </div>

      {/* Repo path */}
      <div>
        <div className="mb-1 text-xs/4 font-medium text-text-muted">Repo Path</div>
        <div className="truncate font-mono text-xs/4 text-text-secondary" title={config.repoPath}>
          {config.repoPath}
        </div>
      </div>

      {/* Env Overrides */}
      {envEntries.length > 0 && (
        <div>
          <div className="mb-1.5 text-xs/4 font-medium text-text-muted">Env Overrides</div>
          <div className="flex flex-col gap-1">
            {envEntries.map(([key, value]) => (
              <div key={key} className="flex items-baseline gap-2 font-mono text-xs/4">
                <span className="shrink-0 text-text-tertiary">{key}</span>
                <span className="text-text-disabled">=</span>
                <span className="min-w-0 truncate text-text-secondary">{value}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
