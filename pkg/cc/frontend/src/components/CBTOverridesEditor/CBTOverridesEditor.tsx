import { useState, useEffect, useMemo, useCallback } from 'react';
import { useAPI } from '@/hooks/useAPI';
import type { CBTOverridesState } from '@/types';
import Spinner from '@/components/Spinner';

interface CBTOverridesEditorProps {
  onToast: (message: string, type: 'success' | 'error') => void;
  stack: string;
}

export default function CBTOverridesEditor({ onToast, stack }: CBTOverridesEditorProps) {
  const { fetchJSON, putJSON, postAction } = useAPI(stack);
  const [state, setState] = useState<CBTOverridesState | null>(null);
  const [saving, setSaving] = useState(false);
  const [showRestartPrompt, setShowRestartPrompt] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [isHybrid, setIsHybrid] = useState(false);
  const [externalFilter, setExternalFilter] = useState('');
  const [transformFilter, setTransformFilter] = useState('');
  const [selectedModel, setSelectedModel] = useState<string | null>(null);
  const [showEnabledOnly, setShowEnabledOnly] = useState(false);
  const [showEnvVars, setShowEnvVars] = useState(false);

  useEffect(() => {
    fetchJSON<CBTOverridesState>('/config/overrides')
      .then(setState)
      .catch(err => onToast(err.message, 'error'));

    fetchJSON<{ mode: string }>('/config')
      .then(cfg => setIsHybrid(cfg.mode === 'hybrid'))
      .catch(() => {}); // non-critical
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fetch once on mount only
  }, [fetchJSON]);

  const handleSave = async () => {
    if (!state) return;

    setSaving(true);

    try {
      const resp = await putJSON<{
        status: string;
        regenerateError?: string;
      }>('/config/overrides', state);

      if (resp.regenerateError) {
        onToast(`Saved but regen failed: ${resp.regenerateError}`, 'error');
      } else {
        onToast('Overrides saved and configs regenerated', 'success');

        // Only prompt restart if any relevant service is actually running
        try {
          const status = await fetchJSON<{ services: { name: string; status: string }[] }>('/status');
          const relevantRunning = status.services.some(
            s =>
              (s.name.startsWith('cbt-') || s.name.startsWith('cbt-api-') || (isHybrid && s.name === 'lab-backend')) &&
              s.status === 'running'
          );

          if (relevantRunning) {
            setShowRestartPrompt(true);
          }
        } catch {
          // If we can't check, show the prompt anyway
          setShowRestartPrompt(true);
        }
      }
    } catch (err) {
      onToast(err instanceof Error ? err.message : 'Save failed', 'error');
    } finally {
      setSaving(false);
    }
  };

  const restartServices = async () => {
    setRestarting(true);

    try {
      const services = await fetchJSON<{ name: string; status: string }[]>('/services');
      const cbtServices = services.filter(
        s => s.name.startsWith('cbt-') && !s.name.startsWith('cbt-api-') && s.status === 'running'
      );

      for (const svc of cbtServices) {
        await postAction(svc.name, 'restart');
      }

      // In hybrid mode, also restart lab-backend so it picks up updated local_overrides
      let restartedBackend = false;

      if (isHybrid) {
        const backend = services.find(s => s.name === 'lab-backend' && s.status === 'running');

        if (backend) {
          await postAction(backend.name, 'restart');
          restartedBackend = true;
        }
      }

      const parts: string[] = [];

      if (cbtServices.length > 0) {
        parts.push(`${cbtServices.length} xatu-cbt`);
      }

      if (restartedBackend) {
        parts.push('lab-backend');
      }

      if (parts.length > 0) {
        onToast(`Restarted ${parts.join(' + ')}`, 'success');
      } else {
        onToast('No running services found to restart', 'error');
      }
    } catch (err) {
      onToast(err instanceof Error ? err.message : 'Restart failed', 'error');
    } finally {
      setRestarting(false);
      setShowRestartPrompt(false);
    }
  };

  const toggleModel = (section: 'externalModels' | 'transformationModels', index: number) => {
    if (!state) return;

    const models = [...state[section]];
    const toggled = { ...models[index], enabled: !models[index].enabled };
    models[index] = toggled;
    setState({ ...state, [section]: models });

    // Clear deps panel if the selected model was just disabled.
    if (!toggled.enabled && selectedModel === toggled.name) {
      setSelectedModel(null);
    }
  };

  const setAllModels = (section: 'externalModels' | 'transformationModels', enabled: boolean) => {
    if (!state) return;

    const models = state[section].map(m => ({ ...m, enabled }));
    setState({ ...state, [section]: models });
  };

  // Build a set of model names that are needed by enabled models but currently disabled.
  const neededModels = useMemo(() => {
    if (!state) return new Set<string>();

    const needed = new Set<string>();
    const enabledSet = new Set<string>();

    for (const m of state.externalModels) {
      if (m.enabled) enabledSet.add(m.name);
    }

    for (const m of state.transformationModels) {
      if (m.enabled) enabledSet.add(m.name);
    }

    // For each enabled transformation model, check its deps.
    for (const m of state.transformationModels) {
      if (!m.enabled) continue;

      const deps = state.dependencies?.[m.name];
      if (!deps) continue;

      for (const dep of deps) {
        if (!enabledSet.has(dep)) {
          needed.add(dep);
        }
      }
    }

    return needed;
  }, [state]);

  const missingDepCount = neededModels.size;

  // Get deps for the currently selected model.
  const selectedDeps = useMemo(() => {
    if (!state || !selectedModel) return null;

    const deps = state.dependencies?.[selectedModel];
    if (!deps || deps.length === 0) return null;

    const enabledSet = new Set<string>();

    for (const m of state.externalModels) {
      if (m.enabled) enabledSet.add(m.name);
    }

    for (const m of state.transformationModels) {
      if (m.enabled) enabledSet.add(m.name);
    }

    return deps.map(d => ({ name: d, enabled: enabledSet.has(d) }));
  }, [state, selectedModel]);

  const enableMissingDeps = useCallback(() => {
    if (!state) return;

    const deps = state.dependencies ?? {};
    const extMap = new Map(state.externalModels.map((m, i) => [m.name, i]));
    const transMap = new Map(state.transformationModels.map((m, i) => [m.name, i]));

    const extModels = state.externalModels.map(m => ({ ...m }));
    const transModels = state.transformationModels.map(m => ({ ...m }));

    // Iteratively enable deps until stable (handles transitive deps).
    let changed = true;
    let totalEnabled = 0;

    while (changed) {
      changed = false;

      const enabledSet = new Set<string>();

      for (const m of extModels) {
        if (m.enabled) enabledSet.add(m.name);
      }

      for (const m of transModels) {
        if (m.enabled) enabledSet.add(m.name);
      }

      for (const m of transModels) {
        if (!m.enabled) continue;

        const modelDeps = deps[m.name];
        if (!modelDeps) continue;

        for (const dep of modelDeps) {
          if (enabledSet.has(dep)) continue;

          // Enable in external models.
          const extIdx = extMap.get(dep);
          if (extIdx !== undefined && !extModels[extIdx].enabled) {
            extModels[extIdx].enabled = true;
            changed = true;
            totalEnabled++;
          }

          // Enable in transformation models.
          const transIdx = transMap.get(dep);
          if (transIdx !== undefined && !transModels[transIdx].enabled) {
            transModels[transIdx].enabled = true;
            changed = true;
            totalEnabled++;
          }
        }
      }
    }

    setState({ ...state, externalModels: extModels, transformationModels: transModels });

    if (totalEnabled > 0) {
      onToast(`Enabled ${totalEnabled} missing dependencies`, 'success');
    } else {
      onToast('No missing dependencies', 'success');
    }
  }, [state, onToast]);

  const filteredExternalModels = useMemo(() => {
    if (!state) return null;
    if (!externalFilter && !showEnabledOnly) return null;

    const lower = externalFilter.toLowerCase();

    return state.externalModels
      .map((m, i) => ({ ...m, originalIndex: i }))
      .filter(m => {
        if (showEnabledOnly && !m.enabled) return false;
        if (lower && !m.name.toLowerCase().includes(lower)) return false;

        return true;
      });
  }, [state, externalFilter, showEnabledOnly]);

  const filteredTransformModels = useMemo(() => {
    if (!state) return null;
    if (!transformFilter && !showEnabledOnly) return null;

    const lower = transformFilter.toLowerCase();

    return state.transformationModels
      .map((m, i) => ({ ...m, originalIndex: i }))
      .filter(m => {
        if (showEnabledOnly && !m.enabled) return false;
        if (lower && !m.name.toLowerCase().includes(lower)) return false;

        return true;
      });
  }, [state, transformFilter, showEnabledOnly]);

  if (!state) {
    return <Spinner text="Loading overrides" centered />;
  }

  const extEnabled = state.externalModels.filter(m => m.enabled).length;
  const transEnabled = state.transformationModels.filter(m => m.enabled).length;

  return (
    <div className="mx-auto max-w-6xl">
      <div className="flex flex-col gap-5">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowEnabledOnly(v => !v)}
              className={`flex items-center gap-1.5 rounded-xs px-3 py-1.5 text-xs/4 font-medium transition-colors ${
                showEnabledOnly
                  ? 'bg-accent/15 text-accent-light ring-1 ring-accent/25'
                  : 'text-text-muted hover:bg-hover/5 hover:text-text-secondary'
              }`}
            >
              <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M12 3c2.755 0 5.455.232 8.083.678.533.09.917.556.917 1.096v1.044a2.25 2.25 0 0 1-.659 1.591l-5.432 5.432a2.25 2.25 0 0 0-.659 1.591v2.927a2.25 2.25 0 0 1-1.244 2.013L9.75 21v-6.568a2.25 2.25 0 0 0-.659-1.591L3.659 7.409A2.25 2.25 0 0 1 3 5.818V4.774c0-.54.384-1.006.917-1.096A48.32 48.32 0 0 1 12 3Z"
                />
              </svg>
              {showEnabledOnly ? 'Enabled only' : 'Show all'}
            </button>
            <button
              onClick={() => setShowEnvVars(v => !v)}
              className={`flex items-center gap-1.5 rounded-xs px-3 py-1.5 text-xs/4 font-medium transition-colors ${
                showEnvVars
                  ? 'bg-accent/15 text-accent-light ring-1 ring-accent/25'
                  : 'text-text-muted hover:bg-hover/5 hover:text-text-secondary'
              }`}
            >
              <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M4.745 3A23.933 23.933 0 0 0 3 12c0 3.183.62 6.22 1.745 9M19.5 3c.967 2.78 1.5 5.817 1.5 9s-.533 6.22-1.5 9M8.25 8.885l1.444-.89a.75.75 0 0 1 1.105.402l2.402 7.206a.75.75 0 0 0 1.104.401l1.445-.889"
                />
              </svg>
              Environment
            </button>
          </div>

          {missingDepCount > 0 && (
            <button
              onClick={enableMissingDeps}
              className="flex items-center gap-1.5 rounded-xs bg-warning/10 px-3 py-1.5 text-xs/4 font-medium text-warning ring-1 ring-warning/20 transition-colors hover:bg-warning/20"
            >
              <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
                />
              </svg>
              {missingDepCount} missing dep{missingDepCount > 1 ? 's' : ''} — fix
            </button>
          )}
        </div>

        {showEnvVars && (
          <div className="rounded-sm border border-border bg-surface-light p-4">
            <div className="mb-3 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">
              Environment Variables
            </div>
            <div className="flex flex-col gap-3">
              <EnvVarField
                label="EXTERNAL_MODEL_MIN_TIMESTAMP"
                value={state.envMinTimestamp}
                enabled={state.envTimestampEnabled}
                onValueChange={v => setState({ ...state, envMinTimestamp: v })}
                onEnabledChange={v => setState({ ...state, envTimestampEnabled: v })}
              />
              <EnvVarField
                label="EXTERNAL_MODEL_MIN_BLOCK"
                value={state.envMinBlock}
                enabled={state.envBlockEnabled}
                onValueChange={v => setState({ ...state, envMinBlock: v })}
                onEnabledChange={v => setState({ ...state, envBlockEnabled: v })}
              />
            </div>
          </div>
        )}

        {/* Models grid */}
        <div className="grid grid-cols-2 gap-0 overflow-hidden rounded-sm border border-border">
          {/* External Models */}
          <ModelColumn
            title="External Models"
            enabledCount={extEnabled}
            totalCount={state.externalModels.length}
            filter={externalFilter}
            onFilterChange={setExternalFilter}
            onEnableAll={() => setAllModels('externalModels', true)}
            onDisableAll={() => setAllModels('externalModels', false)}
          >
            <div className="flex flex-col">
              {(filteredExternalModels ?? state.externalModels.map((m, i) => ({ ...m, originalIndex: i }))).map(
                model => (
                  <ModelRow
                    key={model.name}
                    model={model}
                    needed={neededModels.has(model.name)}
                    isSelected={selectedModel === model.name}
                    onToggle={() => toggleModel('externalModels', model.originalIndex)}
                    onSelect={() => setSelectedModel(selectedModel === model.name ? null : model.name)}
                  />
                )
              )}
            </div>
          </ModelColumn>

          {/* Transformation Models */}
          <ModelColumn
            title="Transformation Models"
            enabledCount={transEnabled}
            totalCount={state.transformationModels.length}
            filter={transformFilter}
            onFilterChange={setTransformFilter}
            onEnableAll={() => setAllModels('transformationModels', true)}
            onDisableAll={() => setAllModels('transformationModels', false)}
            borderLeft
          >
            <div className="flex flex-col">
              {(filteredTransformModels ?? state.transformationModels.map((m, i) => ({ ...m, originalIndex: i }))).map(
                model => (
                  <ModelRow
                    key={model.name}
                    model={model}
                    needed={neededModels.has(model.name)}
                    isSelected={selectedModel === model.name}
                    hasDeps={!!state.dependencies?.[model.name]?.length}
                    onToggle={() => toggleModel('transformationModels', model.originalIndex)}
                    onSelect={() => setSelectedModel(selectedModel === model.name ? null : model.name)}
                  />
                )
              )}
            </div>
          </ModelColumn>
        </div>

        {/* Selected model deps panel */}
        {selectedModel && selectedDeps && (
          <div className="rounded-sm border border-border bg-surface-light p-4">
            <div className="mb-3 flex items-center gap-2">
              <svg
                className="size-4 text-text-muted"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1.5}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z"
                />
              </svg>
              <span className="text-xs/4 font-medium text-text-tertiary">Dependencies of</span>
              <code className="rounded-xs bg-accent/10 px-1.5 py-0.5 text-xs/4 text-accent-light">{selectedModel}</code>
            </div>
            <div className="flex flex-wrap gap-1.5">
              {selectedDeps.map(dep => (
                <span
                  key={dep.name}
                  className={`flex items-center gap-1 rounded-xs px-2 py-1 font-mono text-xs/4 ${
                    dep.enabled ? 'bg-success/10 text-success' : 'bg-error/10 text-error'
                  }`}
                >
                  {dep.enabled ? (
                    <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={3}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                    </svg>
                  ) : (
                    <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={3}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                    </svg>
                  )}
                  {dep.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Save bar */}
        <div className="flex items-center justify-between rounded-sm border border-border bg-surface-light px-4 py-3">
          <span className="text-xs/4 text-text-disabled">
            {extEnabled + transEnabled} model{extEnabled + transEnabled !== 1 ? 's' : ''} enabled
            {missingDepCount > 0 && (
              <span className="ml-1 text-warning">
                ({missingDepCount} missing dep{missingDepCount > 1 ? 's' : ''})
              </span>
            )}
          </span>
          <button
            onClick={handleSave}
            disabled={saving}
            className="rounded-xs bg-accent px-4 py-1.5 text-sm/5 font-medium text-on-accent transition-colors hover:bg-accent-light disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save Overrides'}
          </button>
        </div>
      </div>

      {/* Restart modal */}
      {showRestartPrompt && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60">
          <div className="w-full max-w-sm rounded-sm border border-border bg-surface-light p-6 shadow-xl">
            {restarting ? (
              <div className="flex flex-col items-center gap-3 py-4">
                <div className="size-6 animate-spin rounded-full border-2 border-accent-light border-t-transparent" />
                <span className="text-sm/5 text-accent-light">
                  Restarting {isHybrid ? 'xatu-cbt + lab-backend' : 'xatu-cbt'} services...
                </span>
              </div>
            ) : (
              <>
                <div className="mb-2 flex items-center gap-2">
                  <svg
                    className="size-5 text-accent-light"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={1.5}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182M2.985 19.644l3.181-3.183"
                    />
                  </svg>
                  <h3 className="text-sm/5 font-semibold text-text-primary">Restart Services</h3>
                </div>
                <p className="mt-2 text-sm/5 text-text-tertiary">
                  Restart {isHybrid ? 'xatu-cbt and lab-backend' : 'xatu-cbt services'} to apply the new overrides?
                </p>
                <div className="mt-5 flex justify-end gap-2">
                  <button
                    onClick={() => setShowRestartPrompt(false)}
                    className="rounded-xs px-3 py-1.5 text-xs/4 font-medium text-text-muted transition-colors hover:text-text-secondary"
                  >
                    Skip
                  </button>
                  <button
                    onClick={restartServices}
                    className="rounded-xs bg-accent px-3 py-1.5 text-xs/4 font-medium text-on-accent transition-colors hover:bg-accent-light"
                  >
                    Restart
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/* ── Model column ──────────────────────────────────────────────── */

function ModelColumn({
  title,
  enabledCount,
  totalCount,
  filter,
  onFilterChange,
  onEnableAll,
  onDisableAll,
  borderLeft,
  children,
}: {
  title: string;
  enabledCount: number;
  totalCount: number;
  filter: string;
  onFilterChange: (v: string) => void;
  onEnableAll: () => void;
  onDisableAll: () => void;
  borderLeft?: boolean;
  children: React.ReactNode;
}) {
  const pct = totalCount > 0 ? (enabledCount / totalCount) * 100 : 0;

  return (
    <div className={`flex flex-col bg-surface ${borderLeft ? 'border-l border-border' : ''}`}>
      {/* Header */}
      <div className="border-b border-border px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-baseline gap-2">
            <h3 className="text-sm/5 font-semibold text-text-secondary">{title}</h3>
            <span className="font-mono text-xs/4 text-text-disabled">
              {enabledCount}/{totalCount}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={onEnableAll}
              className="rounded-xs px-2 py-0.5 text-xs/4 font-medium text-text-muted transition-colors hover:bg-hover/5 hover:text-success"
            >
              All
            </button>
            <span className="text-border">{'/'}</span>
            <button
              onClick={onDisableAll}
              className="rounded-xs px-2 py-0.5 text-xs/4 font-medium text-text-muted transition-colors hover:bg-hover/5 hover:text-error"
            >
              None
            </button>
          </div>
        </div>
        {/* Progress bar */}
        <div className="mt-2 h-0.5 w-full rounded-full bg-surface-lighter">
          <div className="h-full rounded-full bg-accent/60 transition-all duration-300" style={{ width: `${pct}%` }} />
        </div>
      </div>

      {/* Search */}
      <div className="border-b border-border/50 px-4 py-2">
        <div className="flex items-center gap-2 rounded-xs bg-surface-light px-2.5 py-1.5">
          <svg
            className="size-3.5 shrink-0 text-text-disabled"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z"
            />
          </svg>
          <input
            type="text"
            value={filter}
            onChange={e => onFilterChange(e.target.value)}
            placeholder="Filter..."
            className="w-full bg-transparent text-sm/5 text-text-primary placeholder:text-text-disabled focus:outline-hidden"
          />
          {filter && (
            <button onClick={() => onFilterChange('')} className="shrink-0 text-text-disabled hover:text-text-tertiary">
              <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {/* Model list */}
      <div className="max-h-112 overflow-y-auto">{children}</div>
    </div>
  );
}

/* ── Model row ─────────────────────────────────────────────────── */

function ModelRow({
  model,
  needed,
  isSelected,
  hasDeps,
  onToggle,
  onSelect,
}: {
  model: { name: string; enabled: boolean };
  needed: boolean;
  isSelected: boolean;
  hasDeps?: boolean;
  onToggle: () => void;
  onSelect: () => void;
}) {
  return (
    <div
      className={`group flex items-center gap-3 border-b border-border/30 px-4 py-2 transition-colors last:border-b-0 ${
        isSelected ? 'bg-accent/5' : 'hover:bg-hover/2'
      }`}
    >
      {/* Toggle */}
      <button onClick={onToggle} className="relative shrink-0" aria-label={`Toggle ${model.name}`}>
        <div
          className={`h-4 w-7 rounded-full transition-colors ${
            model.enabled ? 'bg-success/80' : needed ? 'bg-warning/30' : 'bg-surface-lighter'
          }`}
        />
        <div
          className={`absolute top-0.5 left-0.5 size-3 rounded-full transition-all ${
            model.enabled ? 'translate-x-3 bg-text-primary' : needed ? 'bg-warning' : 'bg-text-muted'
          }`}
        />
      </button>

      {/* Name */}
      <button onClick={onToggle} className="min-w-0 flex-1 text-left">
        <span
          className={`truncate font-mono text-xs/4 ${
            model.enabled ? 'text-text-secondary' : needed ? 'text-warning' : 'text-text-muted'
          }`}
        >
          {model.name}
        </span>
      </button>

      {/* Needed indicator */}
      {needed && !model.enabled && (
        <span className="shrink-0 text-xs/4 font-medium text-warning" title="Required by an enabled model">
          !
        </span>
      )}

      {/* Deps button */}
      {hasDeps && (
        <button
          onClick={onSelect}
          className={`shrink-0 rounded-xs p-0.5 transition-colors ${
            isSelected ? 'text-accent-light' : 'text-border hover:text-text-tertiary'
          }`}
          title="View dependencies"
        >
          <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z"
            />
          </svg>
        </button>
      )}
    </div>
  );
}

/* ── Env var field ──────────────────────────────────────────────── */

function EnvVarField({
  label,
  value,
  enabled,
  onValueChange,
  onEnabledChange,
}: {
  label: string;
  value: string;
  enabled: boolean;
  onValueChange: (v: string) => void;
  onEnabledChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center gap-3">
      <button onClick={() => onEnabledChange(!enabled)} className="relative shrink-0" aria-label={`Toggle ${label}`}>
        <div className={`h-4 w-7 rounded-full transition-colors ${enabled ? 'bg-accent/80' : 'bg-surface-lighter'}`} />
        <div
          className={`absolute top-0.5 left-0.5 size-3 rounded-full transition-all ${
            enabled ? 'translate-x-3 bg-text-primary' : 'bg-text-muted'
          }`}
        />
      </button>
      <code className={`w-72 shrink-0 text-xs/4 ${enabled ? 'text-text-secondary' : 'text-text-disabled'}`}>
        {label}
      </code>
      <input
        type="text"
        value={value}
        onChange={e => onValueChange(e.target.value)}
        disabled={!enabled}
        className="flex-1 rounded-xs border border-border bg-surface px-3 py-1.5 font-mono text-sm/5 text-text-primary placeholder:text-text-disabled disabled:opacity-30"
        placeholder="0"
      />
    </div>
  );
}
