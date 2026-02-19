import { useState, useEffect } from 'react';
import { useAPI } from '@/hooks/useAPI';
import type { LabConfigFull } from '@/types';
import Spinner from '@/components/Spinner';

interface LabConfigEditorProps {
  onToast: (message: string, type: 'success' | 'error') => void;
  onNavigateDashboard?: () => void;
}

export default function LabConfigEditor({ onToast, onNavigateDashboard }: LabConfigEditorProps) {
  const { fetchJSON, putJSON, postJSON } = useAPI();
  const [config, setConfig] = useState<LabConfigFull | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);

  useEffect(() => {
    fetchJSON<LabConfigFull>('/api/config/lab')
      .then(setConfig)
      .catch(err => setError(err.message));
  }, [fetchJSON]);

  const handleSave = async () => {
    try {
      const status = await fetchJSON<{ services: { name: string; status: string }[] }>('/api/status');
      const anyRunning = status.services.some(s => s.status === 'running');

      if (anyRunning) {
        setShowConfirm(true);

        return;
      }
    } catch {
      // If we can't check, assume running and show modal to be safe.
      setShowConfirm(true);

      return;
    }

    // Nothing running — save directly, no modal needed.
    setSaving(true);
    const ok = await saveConfig();
    setSaving(false);

    if (ok) {
      onToast('Config saved', 'success');
    }
  };

  const saveConfig = async (): Promise<boolean> => {
    if (!config) return false;

    try {
      const resp = await putJSON<{
        status: string;
        regenerateError?: string;
      }>('/api/config/lab', config);

      if (resp.regenerateError) {
        onToast(`Saved but regeneration failed: ${resp.regenerateError}`, 'error');
        return false;
      }

      return true;
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Save failed';
      setError(msg);
      onToast(msg, 'error');
      return false;
    }
  };

  const handleConfirmRestart = async () => {
    setShowConfirm(false);
    setSaving(true);

    const ok = await saveConfig();
    setSaving(false);

    if (!ok) return;

    try {
      await postJSON<{ status: string }>('/api/stack/restart');
    } catch {
      // If restart fails to start, fall through — dashboard will show error.
    }

    if (onNavigateDashboard) onNavigateDashboard();
  };

  const handleSaveOnly = async () => {
    setShowConfirm(false);
    setSaving(true);

    const ok = await saveConfig();
    setSaving(false);

    if (ok) {
      onToast('Config saved (restart required to apply)', 'success');
    }
  };

  if (error && !config) {
    return <div className="rounded-xs border border-red-500/30 bg-red-500/10 p-4 text-sm/5 text-red-400">{error}</div>;
  }

  if (!config) {
    return <Spinner text="Loading config" centered />;
  }

  const isHybrid = config.mode === 'hybrid';

  return (
    <div className="mx-auto max-w-4xl">
      <div className="flex flex-col gap-8">
        {/* Mode + Networks row */}
        <div className="grid grid-cols-2 gap-6">
          {/* Mode */}
          <Section title="Mode" icon={modeIcon}>
            <div className="flex gap-2">
              {['local', 'hybrid'].map(m => (
                <button
                  key={m}
                  onClick={() => {
                    const updated = { ...config, mode: m };
                    const xatuMode = m === 'local' ? 'local' : 'external';

                    updated.infrastructure = {
                      ...config.infrastructure,
                      ClickHouse: {
                        ...config.infrastructure.ClickHouse,
                        Xatu: { ...config.infrastructure.ClickHouse.Xatu, Mode: xatuMode },
                      },
                    };
                    setConfig(updated);
                  }}
                  className={`rounded-xs px-4 py-1.5 text-sm/5 font-medium transition-colors ${
                    config.mode === m ? 'bg-indigo-600 text-white' : 'bg-surface-lighter text-gray-400 hover:text-white'
                  }`}
                >
                  {m}
                </button>
              ))}
            </div>
            <p className="mt-3 text-xs/4 text-gray-600">
              {isHybrid ? 'External Xatu ClickHouse with local CBT processing' : 'Fully local development environment'}
            </p>
          </Section>

          {/* Observability */}
          <Section title="Observability" icon={observabilityIcon}>
            <label className="flex cursor-pointer items-center gap-3">
              <div className="relative">
                <input
                  type="checkbox"
                  checked={config.infrastructure.Observability.Enabled}
                  onChange={e =>
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
                  className="peer sr-only"
                />
                <div className="h-5 w-9 rounded-full bg-surface-lighter transition-colors peer-checked:bg-indigo-600" />
                <div className="absolute top-0.5 left-0.5 size-4 rounded-full bg-gray-400 transition-all peer-checked:translate-x-4 peer-checked:bg-white" />
              </div>
              <span className="text-sm/5 text-gray-300">
                {config.infrastructure.Observability.Enabled ? 'Prometheus & Grafana enabled' : 'Disabled'}
              </span>
            </label>
          </Section>
        </div>

        {/* Networks */}
        <Section title="Networks" icon={networkIcon}>
          <div className="grid grid-cols-3 gap-3">
            {config.networks.map((net, i) => {
              const enabled = net.Enabled;

              return (
                <button
                  key={net.Name}
                  onClick={() => {
                    const networks = [...config.networks];
                    networks[i] = { ...networks[i], Enabled: !enabled };
                    setConfig({ ...config, networks });
                  }}
                  className={`group flex items-center justify-between rounded-xs border px-4 py-3 text-left transition-all ${
                    enabled ? 'border-indigo-500/40 bg-indigo-500/10' : 'border-border bg-surface hover:border-gray-600'
                  }`}
                >
                  <div>
                    <div className={`text-sm/5 font-medium ${enabled ? 'text-white' : 'text-gray-500'}`}>
                      {net.Name}
                    </div>
                    <div className="text-xs/4 text-gray-600">port offset +{net.PortOffset}</div>
                  </div>
                  <div className={`size-2 rounded-full ${enabled ? 'bg-indigo-500' : 'bg-gray-700'}`} />
                </button>
              );
            })}
          </div>
        </Section>

        {/* ClickHouse */}
        <Section title="ClickHouse" icon={databaseIcon}>
          <div className="grid grid-cols-2 gap-4">
            <ClusterCard
              label="Xatu"
              description={isHybrid ? 'External production data' : 'Local instance'}
              cluster={config.infrastructure.ClickHouse.Xatu}
              onChange={xatu =>
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
            <ClusterCard
              label="CBT"
              description="Local processing & storage"
              cluster={config.infrastructure.ClickHouse.CBT}
              onChange={cbt =>
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
        </Section>

        {/* Ports */}
        <Section title="Ports" icon={portIcon}>
          <div className="grid grid-cols-3 gap-4">
            <PortField
              label="Lab Backend"
              value={config.ports.LabBackend}
              onChange={v => setConfig({ ...config, ports: { ...config.ports, LabBackend: v } })}
            />
            <PortField
              label="Lab Frontend"
              value={config.ports.LabFrontend}
              onChange={v => setConfig({ ...config, ports: { ...config.ports, LabFrontend: v } })}
            />
            <PortField
              label="CBT Base"
              value={config.ports.CBTBase}
              onChange={v => setConfig({ ...config, ports: { ...config.ports, CBTBase: v } })}
            />
            <PortField
              label="CBT API Base"
              value={config.ports.CBTAPIBase}
              onChange={v => setConfig({ ...config, ports: { ...config.ports, CBTAPIBase: v } })}
            />
            <PortField
              label="CBT Frontend Base"
              value={config.ports.CBTFrontendBase}
              onChange={v => setConfig({ ...config, ports: { ...config.ports, CBTFrontendBase: v } })}
            />
          </div>
        </Section>

        {/* Dev Settings */}
        {config.mode === 'local' && (
          <Section title="Dev Settings" icon={devIcon}>
            <div className="max-w-xs">
              <label className="mb-1.5 block text-xs/4 font-medium text-gray-500">Xatu Ref</label>
              <input
                type="text"
                value={config.dev.XatuRef ?? ''}
                onChange={e =>
                  setConfig({
                    ...config,
                    dev: { ...config.dev, XatuRef: e.target.value || undefined },
                  })
                }
                placeholder="master"
                className="w-full rounded-xs border border-border bg-surface px-3 py-2 text-sm/5 text-white placeholder:text-gray-600 focus:border-indigo-500 focus:outline-hidden"
              />
            </div>
          </Section>
        )}

        {/* Error */}
        {error && (
          <div className="rounded-xs border border-red-500/30 bg-red-500/10 p-3 text-sm/5 text-red-400">{error}</div>
        )}

        {/* Confirm modal */}
        {showConfirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
            <div className="w-full max-w-sm rounded-sm border border-border bg-surface-light p-6 shadow-xl">
              <div className="mb-2 flex items-center gap-2">
                <svg
                  className="size-5 text-rose-400"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={1.5}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
                  />
                </svg>
                <h3 className="text-sm/5 font-semibold text-white">Restart Required</h3>
              </div>
              <p className="mb-5 text-sm/5 text-gray-400">
                Lab config changes require a full stack restart. The stack will be torn down and rebooted.
              </p>
              <div className="flex justify-end gap-2">
                <button
                  onClick={() => setShowConfirm(false)}
                  className="rounded-xs px-3 py-1.5 text-xs/4 text-gray-500 transition-colors hover:text-gray-300"
                >
                  Cancel
                </button>
                <button
                  onClick={handleSaveOnly}
                  className="rounded-xs bg-surface-lighter px-3 py-1.5 text-xs/4 font-medium text-gray-300 transition-colors hover:bg-gray-600"
                >
                  Save Only
                </button>
                <button
                  onClick={handleConfirmRestart}
                  className="rounded-xs bg-rose-600 px-3 py-1.5 text-xs/4 font-medium text-white transition-colors hover:bg-rose-500"
                >
                  Save & Restart
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Save */}
        <div className="flex justify-end">
          <button
            onClick={handleSave}
            disabled={saving}
            className="rounded-xs bg-indigo-600 px-5 py-2 text-sm/5 font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save & Regenerate'}
          </button>
        </div>
      </div>
    </div>
  );
}

/* ── Section wrapper ─────────────────────────────────────────────── */

function Section({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="rounded-sm border border-border bg-surface-light p-5">
      <div className="mb-4 flex items-center gap-2.5">
        <span className="text-gray-500">{icon}</span>
        <h3 className="text-sm/5 font-semibold text-gray-300">{title}</h3>
      </div>
      {children}
    </div>
  );
}

/* ── Port field ──────────────────────────────────────────────────── */

function PortField({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <div>
      <label className="mb-1.5 block text-xs/4 font-medium text-gray-500">{label}</label>
      <input
        type="number"
        value={value}
        onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
        className="w-full rounded-xs border border-border bg-surface px-3 py-2 text-sm/5 text-white focus:border-indigo-500 focus:outline-hidden"
      />
    </div>
  );
}

/* ── ClickHouse cluster card ─────────────────────────────────────── */

function ClusterCard({
  label,
  description,
  cluster,
  onChange,
}: {
  label: string;
  description: string;
  cluster: {
    Mode: string;
    ExternalURL?: string;
    ExternalDatabase?: string;
    ExternalUsername?: string;
    ExternalPassword?: string;
  };
  onChange: (c: typeof cluster) => void;
}) {
  const isExternal = cluster.Mode === 'external';

  return (
    <div className="flex flex-col gap-3 rounded-xs border border-border/50 bg-surface p-4">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm/5 font-medium text-white">{label}</div>
          <div className="text-xs/4 text-gray-600">{description}</div>
        </div>
        <div className="flex gap-1">
          {['local', 'external'].map(m => (
            <button
              key={m}
              onClick={() => onChange({ ...cluster, Mode: m })}
              className={`rounded-xs px-2.5 py-1 text-xs/4 font-medium transition-colors ${
                cluster.Mode === m ? 'bg-indigo-600 text-white' : 'bg-surface-lighter text-gray-500 hover:text-gray-300'
              }`}
            >
              {m}
            </button>
          ))}
        </div>
      </div>
      {isExternal && (
        <div className="flex flex-col gap-2.5 border-t border-border/50 pt-3">
          <SmallInput
            label="URL"
            value={cluster.ExternalURL ?? ''}
            onChange={v => onChange({ ...cluster, ExternalURL: v })}
          />
          <SmallInput
            label="Database"
            value={cluster.ExternalDatabase ?? ''}
            onChange={v => onChange({ ...cluster, ExternalDatabase: v })}
          />
          <div className="grid grid-cols-2 gap-2.5">
            <SmallInput
              label="Username"
              value={cluster.ExternalUsername ?? ''}
              onChange={v => onChange({ ...cluster, ExternalUsername: v })}
            />
            <SmallInput
              label="Password"
              value={cluster.ExternalPassword ?? ''}
              onChange={v => onChange({ ...cluster, ExternalPassword: v })}
              type="password"
            />
          </div>
        </div>
      )}
    </div>
  );
}

function SmallInput({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs/4 text-gray-600">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        className="w-full rounded-xs border border-border bg-surface-light px-2.5 py-1.5 text-xs/4 text-white focus:border-indigo-500 focus:outline-hidden"
      />
    </div>
  );
}

/* ── Icons ───────────────────────────────────────────────────────── */

const iconClass = 'size-4';

const modeIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
    />
    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
  </svg>
);

const networkIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M12 21a9.004 9.004 0 0 0 8.716-6.747M12 21a9.004 9.004 0 0 1-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 0 1 7.843 4.582M12 3a8.997 8.997 0 0 0-7.843 4.582m15.686 0A11.953 11.953 0 0 1 12 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0 1 21 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0 1 12 16.5a17.92 17.92 0 0 1-8.716-2.247m0 0A9 9 0 0 1 3 12c0-1.47.353-2.856.978-4.082"
    />
  </svg>
);

const databaseIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125"
    />
  </svg>
);

const portIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M5.25 14.25h13.5m-13.5 0a3 3 0 0 1-3-3m3 3a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-16.5-3a3 3 0 0 1 3-3h13.5a3 3 0 0 1 3 3m-19.5 0a4.5 4.5 0 0 1 .9-2.7L5.737 5.1a3.375 3.375 0 0 1 2.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 0 1 .9 2.7m0 0a3 3 0 0 1-3 3m0 3h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Z"
    />
  </svg>
);

const observabilityIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 0 1 3 19.875v-6.75ZM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V8.625ZM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V4.125Z"
    />
  </svg>
);

const devIcon = (
  <svg className={iconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5"
    />
  </svg>
);
