import { useState, useEffect, useMemo, useCallback } from "react";
import { useAPI } from "../hooks/useAPI";
import type { CBTOverridesState } from "../types";

interface CBTOverridesEditorProps {
  onToast: (message: string, type: "success" | "error") => void;
}

export default function CBTOverridesEditor({
  onToast,
}: CBTOverridesEditorProps) {
  const { fetchJSON, putJSON, postAction } = useAPI();
  const [state, setState] = useState<CBTOverridesState | null>(null);
  const [saving, setSaving] = useState(false);
  const [showRestartPrompt, setShowRestartPrompt] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [externalFilter, setExternalFilter] = useState("");
  const [transformFilter, setTransformFilter] = useState("");
  const [selectedModel, setSelectedModel] = useState<string | null>(null);
  const [showEnabledOnly, setShowEnabledOnly] = useState(false);

  useEffect(() => {
    fetchJSON<CBTOverridesState>("/api/config/overrides")
      .then(setState)
      .catch((err) => onToast(err.message, "error"));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fetch once on mount only
  }, [fetchJSON]);

  const handleSave = async () => {
    if (!state) return;

    setSaving(true);

    try {
      const resp = await putJSON<{
        status: string;
        regenerateError?: string;
      }>("/api/config/overrides", state);

      if (resp.regenerateError) {
        onToast(
          `Saved but regen failed: ${resp.regenerateError}`,
          "error",
        );
      } else {
        onToast("Overrides saved and configs regenerated", "success");
        setShowRestartPrompt(true);
      }
    } catch (err) {
      onToast(
        err instanceof Error ? err.message : "Save failed",
        "error",
      );
    } finally {
      setSaving(false);
    }
  };

  const restartCbtApis = async () => {
    setRestarting(true);

    try {
      const services = await fetchJSON<{ name: string; status: string }[]>("/api/services");
      const cbtServices = services.filter(
        (s) => s.name.startsWith("cbt-") && !s.name.startsWith("cbt-api-") && s.status === "running",
      );

      for (const svc of cbtServices) {
        await postAction(svc.name, "restart");
      }

      if (cbtServices.length > 0) {
        onToast(`Restarted ${cbtServices.length} xatu-cbt service${cbtServices.length > 1 ? "s" : ""}`, "success");
      } else {
        onToast("No running xatu-cbt services found", "error");
      }
    } catch (err) {
      onToast(
        err instanceof Error ? err.message : "Restart failed",
        "error",
      );
    } finally {
      setRestarting(false);
      setShowRestartPrompt(false);
    }
  };

  const toggleModel = (
    section: "externalModels" | "transformationModels",
    index: number,
  ) => {
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

  const setAllModels = (
    section: "externalModels" | "transformationModels",
    enabled: boolean,
  ) => {
    if (!state) return;

    const models = state[section].map((m) => ({ ...m, enabled }));
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

    return deps.map((d) => ({ name: d, enabled: enabledSet.has(d) }));
  }, [state, selectedModel]);

  const enableMissingDeps = useCallback(() => {
    if (!state) return;

    const deps = state.dependencies ?? {};
    const extMap = new Map(state.externalModels.map((m, i) => [m.name, i]));
    const transMap = new Map(state.transformationModels.map((m, i) => [m.name, i]));

    const extModels = state.externalModels.map((m) => ({ ...m }));
    const transModels = state.transformationModels.map((m) => ({ ...m }));

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
      onToast(`Enabled ${totalEnabled} missing dependencies`, "success");
    } else {
      onToast("No missing dependencies", "success");
    }
  }, [state, onToast]);

  const filteredExternalModels = useMemo(() => {
    if (!state) return null;
    if (!externalFilter && !showEnabledOnly) return null;

    const lower = externalFilter.toLowerCase();

    return state.externalModels
      .map((m, i) => ({ ...m, originalIndex: i }))
      .filter((m) => {
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
      .filter((m) => {
        if (showEnabledOnly && !m.enabled) return false;
        if (lower && !m.name.toLowerCase().includes(lower)) return false;

        return true;
      });
  }, [state, transformFilter, showEnabledOnly]);

  if (!state) {
    return <div className="text-sm/5 text-gray-400">Loading overrides...</div>;
  }

  return (
    <div className="mx-auto max-w-5xl">
      <div className="flex flex-col gap-6">
        {/* Env vars */}
        <div className="rounded-xs border border-border bg-surface p-4">
          <h3 className="mb-3 text-sm/5 font-semibold text-gray-300">
            Environment Variables
          </h3>
          <div className="flex flex-col gap-3">
            <EnvVarField
              label="EXTERNAL_MODEL_MIN_TIMESTAMP"
              value={state.envMinTimestamp}
              enabled={state.envTimestampEnabled}
              onValueChange={(v) =>
                setState({ ...state, envMinTimestamp: v })
              }
              onEnabledChange={(v) =>
                setState({ ...state, envTimestampEnabled: v })
              }
            />
            <EnvVarField
              label="EXTERNAL_MODEL_MIN_BLOCK"
              value={state.envMinBlock}
              enabled={state.envBlockEnabled}
              onValueChange={(v) =>
                setState({ ...state, envMinBlock: v })
              }
              onEnabledChange={(v) =>
                setState({ ...state, envBlockEnabled: v })
              }
            />
          </div>
        </div>

        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowEnabledOnly((v) => !v)}
            className={`rounded-xs px-3 py-1 text-xs/4 font-medium transition-colors ${
              showEnabledOnly
                ? "bg-indigo-600 text-white"
                : "bg-surface-lighter text-gray-400 hover:text-white"
            }`}
          >
            {showEnabledOnly ? "Showing enabled only" : "Show enabled only"}
          </button>
        </div>

        {/* Enable missing deps banner */}
        {missingDepCount > 0 && (
          <div className="flex items-center justify-between rounded-xs border border-amber-500/30 bg-amber-500/10 px-4 py-3">
            <span className="text-sm/5 text-amber-300">
              {missingDepCount} missing {missingDepCount === 1 ? "dependency" : "dependencies"} detected
            </span>
            <button
              onClick={enableMissingDeps}
              className="rounded-xs bg-amber-600/40 px-3 py-1 text-xs/4 font-medium text-amber-200 transition-colors hover:bg-amber-600/60"
            >
              Enable All Deps
            </button>
          </div>
        )}

        {/* Models */}
        <div className="grid grid-cols-2 gap-4">
          {/* External Models */}
          <div className="rounded-xs border border-border bg-surface p-4">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-sm/5 font-semibold text-gray-300">
                External Models
                <span className="ml-2 text-xs/4 font-normal text-gray-500">
                  ({state.externalModels.filter((m) => m.enabled).length}/
                  {state.externalModels.length})
                </span>
              </h3>
              <div className="flex gap-2">
                <button
                  onClick={() => setAllModels("externalModels", true)}
                  className="rounded-xs bg-emerald-600/30 px-2 py-0.5 text-xs/4 text-emerald-400 transition-colors hover:bg-emerald-600/50"
                >
                  All
                </button>
                <button
                  onClick={() => setAllModels("externalModels", false)}
                  className="rounded-xs bg-red-600/30 px-2 py-0.5 text-xs/4 text-red-400 transition-colors hover:bg-red-600/50"
                >
                  None
                </button>
              </div>
            </div>
            <input
              type="text"
              value={externalFilter}
              onChange={(e) => setExternalFilter(e.target.value)}
              placeholder="Filter models..."
              className="mb-2 w-full rounded-xs border border-border bg-surface-light px-3 py-1.5 text-sm/5 text-white placeholder:text-gray-600"
            />
            <div className="flex max-h-96 flex-col gap-1 overflow-y-auto">
              {(filteredExternalModels ?? state.externalModels.map((m, i) => ({ ...m, originalIndex: i }))).map(
                (model) => (
                  <ModelRow
                    key={model.name}
                    model={model}
                    needed={neededModels.has(model.name)}
                    isSelected={selectedModel === model.name}
                    onToggle={() =>
                      toggleModel("externalModels", model.originalIndex)
                    }
                    onSelect={() =>
                      setSelectedModel(
                        selectedModel === model.name ? null : model.name,
                      )
                    }
                  />
                ),
              )}
            </div>
          </div>

          {/* Transformation Models */}
          <div className="rounded-xs border border-border bg-surface p-4">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-sm/5 font-semibold text-gray-300">
                Transformation Models
                <span className="ml-2 text-xs/4 font-normal text-gray-500">
                  (
                  {
                    state.transformationModels.filter((m) => m.enabled)
                      .length
                  }
                  /{state.transformationModels.length})
                </span>
              </h3>
              <div className="flex gap-2">
                <button
                  onClick={() =>
                    setAllModels("transformationModels", true)
                  }
                  className="rounded-xs bg-emerald-600/30 px-2 py-0.5 text-xs/4 text-emerald-400 transition-colors hover:bg-emerald-600/50"
                >
                  All
                </button>
                <button
                  onClick={() =>
                    setAllModels("transformationModels", false)
                  }
                  className="rounded-xs bg-red-600/30 px-2 py-0.5 text-xs/4 text-red-400 transition-colors hover:bg-red-600/50"
                >
                  None
                </button>
              </div>
            </div>
            <input
              type="text"
              value={transformFilter}
              onChange={(e) => setTransformFilter(e.target.value)}
              placeholder="Filter models..."
              className="mb-2 w-full rounded-xs border border-border bg-surface-light px-3 py-1.5 text-sm/5 text-white placeholder:text-gray-600"
            />
            <div className="flex max-h-96 flex-col gap-1 overflow-y-auto">
              {(filteredTransformModels ?? state.transformationModels.map((m, i) => ({ ...m, originalIndex: i }))).map(
                (model) => (
                  <ModelRow
                    key={model.name}
                    model={model}
                    needed={neededModels.has(model.name)}
                    isSelected={selectedModel === model.name}
                    hasDeps={!!state.dependencies?.[model.name]?.length}
                    onToggle={() =>
                      toggleModel(
                        "transformationModels",
                        model.originalIndex,
                      )
                    }
                    onSelect={() =>
                      setSelectedModel(
                        selectedModel === model.name ? null : model.name,
                      )
                    }
                  />
                ),
              )}
            </div>
          </div>
        </div>

        {/* Selected model deps panel */}
        {selectedModel && selectedDeps && (
          <div className="rounded-xs border border-border bg-surface p-4">
            <h3 className="mb-2 text-sm/5 font-semibold text-gray-300">
              Dependencies of{" "}
              <span className="font-mono text-indigo-400">{selectedModel}</span>
            </h3>
            <div className="flex flex-wrap gap-2">
              {selectedDeps.map((dep) => (
                <span
                  key={dep.name}
                  className={`rounded-xs px-2 py-1 font-mono text-xs/4 ${
                    dep.enabled
                      ? "bg-emerald-600/20 text-emerald-400"
                      : "bg-red-600/20 text-red-400"
                  }`}
                >
                  {dep.enabled ? "\u2713" : "\u2717"} {dep.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Restart prompt */}
        {showRestartPrompt && (
          <div className="flex items-center justify-between rounded-xs border border-indigo-500/30 bg-indigo-500/10 px-4 py-3">
            {restarting ? (
              <div className="flex items-center gap-2">
                <div className="size-3.5 animate-spin rounded-full border-2 border-indigo-400 border-t-transparent" />
                <span className="text-sm/5 text-indigo-300">Restarting xatu-cbt services...</span>
              </div>
            ) : (
              <>
                <span className="text-sm/5 text-indigo-300">
                  Restart xatu-cbt services with these changes?
                </span>
                <div className="flex gap-2">
                  <button
                    onClick={restartCbtApis}
                    className="rounded-xs bg-indigo-600 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-indigo-500"
                  >
                    Restart
                  </button>
                  <button
                    onClick={() => setShowRestartPrompt(false)}
                    className="rounded-xs bg-surface-lighter px-3 py-1 text-xs/4 text-gray-300 transition-colors hover:bg-gray-600"
                  >
                    Skip
                  </button>
                </div>
              </>
            )}
          </div>
        )}

        {/* Save */}
        <button
          onClick={handleSave}
          disabled={saving}
          className="rounded-xs bg-indigo-600 px-4 py-2 text-sm/5 font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50"
        >
          {saving ? "Saving..." : "Save Overrides"}
        </button>
      </div>
    </div>
  );
}

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
      className={`flex items-center gap-2 rounded-xs px-2 py-1 transition-colors hover:bg-white/5 ${
        isSelected ? "bg-indigo-500/10" : ""
      }`}
    >
      <button
        onClick={onToggle}
        className="flex min-w-0 flex-1 items-center gap-2 text-left"
      >
        <span
          className={`size-3 shrink-0 rounded-full border ${
            model.enabled
              ? "border-emerald-500 bg-emerald-500"
              : needed
                ? "border-amber-500 bg-transparent"
                : "border-gray-600 bg-transparent"
          }`}
        />
        <span
          className={`truncate text-xs/4 ${
            model.enabled
              ? "text-gray-200"
              : needed
                ? "text-amber-400"
                : "text-gray-500"
          }`}
        >
          {model.name}
        </span>
        {needed && !model.enabled && (
          <span className="shrink-0 text-xs/4 text-amber-500" title="Needed by an enabled model">!</span>
        )}
      </button>
      {hasDeps && (
        <button
          onClick={onSelect}
          className="shrink-0 rounded-xs p-0.5 text-gray-600 transition-colors hover:bg-white/10 hover:text-gray-400"
          title="View dependencies"
        >
          <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
          </svg>
        </button>
      )}
    </div>
  );
}

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
      <input
        type="checkbox"
        checked={enabled}
        onChange={(e) => onEnabledChange(e.target.checked)}
        className="rounded-xs"
      />
      <label className="w-72 shrink-0 text-xs/4 font-mono text-gray-400">
        {label}
      </label>
      <input
        type="text"
        value={value}
        onChange={(e) => onValueChange(e.target.value)}
        disabled={!enabled}
        className="flex-1 rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white disabled:opacity-40"
        placeholder="0"
      />
    </div>
  );
}
