import { useState, useEffect } from "react";
import { useAPI } from "../hooks/useAPI";
import type { LabConfigFull } from "../types";

interface LabConfigEditorProps {
  onToast: (message: string, type: "success" | "error") => void;
  onNavigateDashboard?: () => void;
}

export default function LabConfigEditor({ onToast, onNavigateDashboard }: LabConfigEditorProps) {
  const { fetchJSON, putJSON, postJSON } = useAPI();
  const [config, setConfig] = useState<LabConfigFull | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);

  useEffect(() => {
    fetchJSON<LabConfigFull>("/api/config/lab")
      .then(setConfig)
      .catch((err) => setError(err.message));
  }, [fetchJSON]);

  const handleSave = () => setShowConfirm(true);

  const saveConfig = async (): Promise<boolean> => {
    if (!config) return false;

    try {
      const resp = await putJSON<{
        status: string;
        regenerateError?: string;
      }>("/api/config/lab", config);

      if (resp.regenerateError) {
        onToast(`Saved but regeneration failed: ${resp.regenerateError}`, "error");
        return false;
      }

      return true;
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Save failed";
      setError(msg);
      onToast(msg, "error");
      return false;
    }
  };

  const handleConfirmRestart = async () => {
    setShowConfirm(false);
    setSaving(true);

    const ok = await saveConfig();
    setSaving(false);

    if (!ok) return;

    // Fire restart first — it returns immediately with {status:"stopping"}.
    // This ensures the stack status is "stopping" before the dashboard mounts.
    try {
      await postJSON<{ status: string }>("/api/stack/restart");
    } catch {
      // If restart fails to start, fall through — dashboard will show error.
    }

    // Navigate to dashboard — it will see "stopping" status immediately.
    if (onNavigateDashboard) onNavigateDashboard();
  };

  const handleSaveOnly = async () => {
    setShowConfirm(false);
    setSaving(true);

    const ok = await saveConfig();
    setSaving(false);

    if (ok) {
      onToast("Config saved (restart required to apply)", "success");
    }
  };

  if (error && !config) {
    return (
      <div className="rounded-xs border border-red-500/30 bg-red-500/10 p-4 text-sm/5 text-red-400">
        {error}
      </div>
    );
  }

  if (!config) {
    return <div className="text-sm/5 text-gray-400">Loading config...</div>;
  }

  return (
    <div className="mx-auto max-w-3xl">
      <div className="flex flex-col gap-6">
        {/* Mode */}
        <FieldSection title="Mode">
          <select
            value={config.mode}
            onChange={(e) => {
              const newMode = e.target.value;
              const updated = { ...config, mode: newMode };

              // Sync ClickHouse Xatu mode with lab mode.
              const xatuMode = newMode === "local" ? "local" : "external";
              updated.infrastructure = {
                ...config.infrastructure,
                ClickHouse: {
                  ...config.infrastructure.ClickHouse,
                  Xatu: { ...config.infrastructure.ClickHouse.Xatu, Mode: xatuMode },
                },
              };

              setConfig(updated);
            }}
            className="rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white"
          >
            <option value="local">local</option>
            <option value="hybrid">hybrid</option>
          </select>
        </FieldSection>

        {/* Networks */}
        <FieldSection title="Networks">
          <div className="flex flex-col gap-2">
            {config.networks.map((net, i) => (
              <label
                key={net.Name}
                className="flex items-center gap-3 text-sm/5 text-gray-300"
              >
                <input
                  type="checkbox"
                  checked={net.Enabled}
                  onChange={(e) => {
                    const networks = [...config.networks];
                    networks[i] = {
                      ...networks[i],
                      Enabled: e.target.checked,
                    };
                    setConfig({ ...config, networks });
                  }}
                  className="rounded-xs"
                />
                {net.Name}
                <span className="text-xs/4 text-gray-500">
                  (offset: {net.PortOffset})
                </span>
              </label>
            ))}
          </div>
        </FieldSection>

        {/* Infrastructure - ClickHouse */}
        <FieldSection title="Infrastructure - ClickHouse">
          <div className="flex flex-col gap-4">
            <ClusterFields
              label="Xatu"
              cluster={config.infrastructure.ClickHouse.Xatu}
              onChange={(xatu) =>
                setConfig({
                  ...config,
                  infrastructure: {
                    ...config.infrastructure,
                    ClickHouse: {
                      ...config.infrastructure.ClickHouse,
                      Xatu: xatu,
                    },
                  },
                })
              }
            />
            <ClusterFields
              label="CBT"
              cluster={config.infrastructure.ClickHouse.CBT}
              onChange={(cbt) =>
                setConfig({
                  ...config,
                  infrastructure: {
                    ...config.infrastructure,
                    ClickHouse: {
                      ...config.infrastructure.ClickHouse,
                      CBT: cbt,
                    },
                  },
                })
              }
            />
          </div>
        </FieldSection>

        {/* Infrastructure - Observability */}
        <FieldSection title="Observability">
          <label className="flex items-center gap-3 text-sm/5 text-gray-300">
            <input
              type="checkbox"
              checked={config.infrastructure.Observability.Enabled}
              onChange={(e) =>
                setConfig({
                  ...config,
                  infrastructure: {
                    ...config.infrastructure,
                    Observability: {
                      ...config.infrastructure.Observability,
                      Enabled: e.target.checked,
                    },
                  },
                })
              }
              className="rounded-xs"
            />
            Enabled
          </label>
        </FieldSection>

        {/* Ports */}
        <FieldSection title="Ports">
          <div className="grid grid-cols-2 gap-3">
            <PortField
              label="Lab Backend"
              value={config.ports.LabBackend}
              onChange={(v) =>
                setConfig({
                  ...config,
                  ports: { ...config.ports, LabBackend: v },
                })
              }
            />
            <PortField
              label="Lab Frontend"
              value={config.ports.LabFrontend}
              onChange={(v) =>
                setConfig({
                  ...config,
                  ports: { ...config.ports, LabFrontend: v },
                })
              }
            />
            <PortField
              label="CBT Base"
              value={config.ports.CBTBase}
              onChange={(v) =>
                setConfig({
                  ...config,
                  ports: { ...config.ports, CBTBase: v },
                })
              }
            />
            <PortField
              label="CBT API Base"
              value={config.ports.CBTAPIBase}
              onChange={(v) =>
                setConfig({
                  ...config,
                  ports: { ...config.ports, CBTAPIBase: v },
                })
              }
            />
            <PortField
              label="CBT Frontend Base"
              value={config.ports.CBTFrontendBase}
              onChange={(v) =>
                setConfig({
                  ...config,
                  ports: { ...config.ports, CBTFrontendBase: v },
                })
              }
            />
          </div>
        </FieldSection>

        {/* Dev */}
        {config.mode === "local" && (
          <FieldSection title="Dev Settings">
            <div>
              <label className="mb-1 block text-xs/4 text-gray-500">
                Xatu Ref
              </label>
              <input
                type="text"
                value={config.dev.XatuRef ?? ""}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    dev: {
                      ...config.dev,
                      XatuRef: e.target.value || undefined,
                    },
                  })
                }
                placeholder="master"
                className="w-full rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white placeholder:text-gray-600"
              />
            </div>
          </FieldSection>
        )}

        {/* Error display */}
        {error && (
          <div className="rounded-xs border border-red-500/30 bg-red-500/10 p-3 text-sm/5 text-red-400">
            {error}
          </div>
        )}

        {/* Confirmation */}
        {showConfirm && (
          <div className="rounded-xs border border-amber-500/30 bg-amber-500/10 p-4">
            <div className="mb-3 flex items-center gap-2">
              <svg className="size-5 text-amber-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
              </svg>
              <span className="text-sm/5 font-semibold text-amber-300">
                Lab config changes require a full stack restart
              </span>
            </div>
            <p className="mb-4 text-xs/4 text-amber-200/70">
              The stack will be torn down and rebooted with the new configuration.
            </p>
            <div className="flex gap-2">
              <button
                onClick={handleConfirmRestart}
                className="rounded-xs bg-amber-600 px-3 py-1.5 text-xs/4 font-medium text-white transition-colors hover:bg-amber-500"
              >
                Save & Restart Stack
              </button>
              <button
                onClick={handleSaveOnly}
                className="rounded-xs bg-surface-lighter px-3 py-1.5 text-xs/4 text-gray-300 transition-colors hover:bg-gray-600"
              >
                Save Only
              </button>
              <button
                onClick={() => setShowConfirm(false)}
                className="rounded-xs px-3 py-1.5 text-xs/4 text-gray-500 transition-colors hover:text-gray-300"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Save */}
        <button
          onClick={handleSave}
          disabled={saving}
          className="rounded-xs bg-indigo-600 px-4 py-2 text-sm/5 font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50"
        >
          {saving ? "Saving..." : "Save & Regenerate"}
        </button>
      </div>
    </div>
  );
}

function FieldSection({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xs border border-border bg-surface p-4">
      <h3 className="mb-3 text-sm/5 font-semibold text-gray-300">{title}</h3>
      {children}
    </div>
  );
}

function PortField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs/4 text-gray-500">{label}</label>
      <input
        type="number"
        value={value}
        onChange={(e) => onChange(parseInt(e.target.value, 10) || 0)}
        className="w-full rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white"
      />
    </div>
  );
}

function ClusterFields({
  label,
  cluster,
  onChange,
}: {
  label: string;
  cluster: {
    Mode: string;
    ExternalURL?: string;
    ExternalDatabase?: string;
    ExternalUsername?: string;
    ExternalPassword?: string;
  };
  onChange: (c: typeof cluster) => void;
}) {
  return (
    <div className="rounded-xs border border-border/50 bg-surface-light p-3">
      <div className="mb-2 text-xs/4 font-semibold text-gray-400">{label}</div>
      <div className="flex flex-col gap-2">
        <div>
          <label className="mb-1 block text-xs/4 text-gray-500">Mode</label>
          <select
            value={cluster.Mode}
            onChange={(e) => onChange({ ...cluster, Mode: e.target.value })}
            className="rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white"
          >
            <option value="local">local</option>
            <option value="external">external</option>
          </select>
        </div>
        {cluster.Mode === "external" && (
          <>
            <InputField
              label="External URL"
              value={cluster.ExternalURL ?? ""}
              onChange={(v) => onChange({ ...cluster, ExternalURL: v })}
            />
            <InputField
              label="Database"
              value={cluster.ExternalDatabase ?? ""}
              onChange={(v) =>
                onChange({ ...cluster, ExternalDatabase: v })
              }
            />
            <InputField
              label="Username"
              value={cluster.ExternalUsername ?? ""}
              onChange={(v) =>
                onChange({ ...cluster, ExternalUsername: v })
              }
            />
            <InputField
              label="Password"
              value={cluster.ExternalPassword ?? ""}
              onChange={(v) =>
                onChange({ ...cluster, ExternalPassword: v })
              }
              type="password"
            />
          </>
        )}
      </div>
    </div>
  );
}

function InputField({
  label,
  value,
  onChange,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs/4 text-gray-500">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-xs border border-border bg-surface px-3 py-1.5 text-sm/5 text-white"
      />
    </div>
  );
}
