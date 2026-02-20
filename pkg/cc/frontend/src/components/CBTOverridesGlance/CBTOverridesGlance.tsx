import type { CBTOverridesState } from '@/types';
import Spinner from '@/components/Spinner';

interface CBTOverridesGlanceProps {
  overrides: CBTOverridesState | null;
}

export default function CBTOverridesGlance({ overrides }: CBTOverridesGlanceProps) {
  if (!overrides) return <Spinner />;

  const extEnabled = overrides.externalModels.filter(m => m.enabled).length;
  const extTotal = overrides.externalModels.length;
  const txEnabled = overrides.transformationModels.filter(m => m.enabled).length;
  const txTotal = overrides.transformationModels.length;

  // Detect missing dependencies: enabled transformation models whose deps are not all enabled
  const enabledExtKeys = new Set(overrides.externalModels.filter(m => m.enabled).map(m => m.overrideKey));
  const enabledTxKeys = new Set(overrides.transformationModels.filter(m => m.enabled).map(m => m.overrideKey));
  const allEnabledKeys = new Set([...enabledExtKeys, ...enabledTxKeys]);

  let missingDeps = 0;
  for (const model of overrides.transformationModels) {
    if (!model.enabled) continue;
    const deps = overrides.dependencies[model.overrideKey] ?? [];
    for (const dep of deps) {
      if (!allEnabledKeys.has(dep)) {
        missingDeps++;
        break;
      }
    }
  }

  const allExtEnabled = extEnabled === extTotal;
  const allTxEnabled = txEnabled === txTotal;

  return (
    <div className="flex flex-col gap-3 text-xs/4">
      <div className="flex flex-col gap-2">
        <ModelBar label="External" enabled={extEnabled} total={extTotal} allEnabled={allExtEnabled} />
        <ModelBar label="Transformation" enabled={txEnabled} total={txTotal} allEnabled={allTxEnabled} />
      </div>

      {missingDeps > 0 && (
        <div className="flex items-center gap-1.5 text-warning">
          <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
            />
          </svg>
          <span>
            {missingDeps} model{missingDeps > 1 ? 's' : ''} with missing deps
          </span>
        </div>
      )}

      {(overrides.envTimestampEnabled || overrides.envBlockEnabled) && (
        <div className="flex flex-col gap-1 border-t border-border/50 pt-2">
          {overrides.envTimestampEnabled && overrides.envMinTimestamp && (
            <div className="flex items-center justify-between">
              <span className="text-text-disabled">Min timestamp</span>
              <span className="font-mono text-text-secondary">{overrides.envMinTimestamp}</span>
            </div>
          )}
          {overrides.envBlockEnabled && overrides.envMinBlock && (
            <div className="flex items-center justify-between">
              <span className="text-text-disabled">Min block</span>
              <span className="font-mono text-text-secondary">{overrides.envMinBlock}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ModelBar({
  label,
  enabled,
  total,
  allEnabled,
}: {
  label: string;
  enabled: number;
  total: number;
  allEnabled: boolean;
}) {
  const pct = total > 0 ? (enabled / total) * 100 : 0;

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between">
        <span className="text-text-tertiary">{label}</span>
        <span className={allEnabled ? 'text-success' : 'text-text-secondary'}>
          {enabled}/{total}
        </span>
      </div>
      <div className="h-1 w-full overflow-hidden rounded-full bg-border/50">
        <div
          className={`h-full rounded-full transition-all duration-300 ${allEnabled ? 'bg-success' : 'bg-accent-light'}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}
