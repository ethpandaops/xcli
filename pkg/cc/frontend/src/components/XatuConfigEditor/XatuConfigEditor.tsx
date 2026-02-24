import { useState, useEffect, useCallback } from 'react';
import { useAPI } from '@/hooks/useAPI';
import type { XatuConfigResponse } from '@/types';
import Spinner from '@/components/Spinner';

interface XatuConfigEditorProps {
  onToast: (message: string, type: 'success' | 'error') => void;
  onNavigateDashboard?: () => void;
  stack: string;
}

export default function XatuConfigEditor({ onToast, onNavigateDashboard, stack }: XatuConfigEditorProps) {
  const { fetchJSON, putJSON, postJSON } = useAPI(stack);
  const [config, setConfig] = useState<XatuConfigResponse | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);

  // Editable state
  const [profiles, setProfiles] = useState<string[]>([]);
  const [newProfile, setNewProfile] = useState('');
  const [envOverrides, setEnvOverrides] = useState<[string, string][]>([]);

  useEffect(() => {
    fetchJSON<XatuConfigResponse>('/config')
      .then(data => {
        setConfig(data);
        setProfiles(data.profiles ?? []);
        setEnvOverrides(Object.entries(data.envOverrides ?? {}));
      })
      .catch(err => setError(err.message));
  }, [fetchJSON]);

  const addProfile = useCallback(() => {
    const trimmed = newProfile.trim();
    if (!trimmed || profiles.includes(trimmed)) return;
    setProfiles(prev => [...prev, trimmed]);
    setNewProfile('');
  }, [newProfile, profiles]);

  const removeProfile = useCallback((profile: string) => {
    setProfiles(prev => prev.filter(p => p !== profile));
  }, []);

  const addEnvOverride = useCallback(() => {
    setEnvOverrides(prev => [...prev, ['', '']]);
  }, []);

  const updateEnvKey = useCallback((index: number, key: string) => {
    setEnvOverrides(prev => prev.map((entry, i) => (i === index ? [key, entry[1]] : entry)));
  }, []);

  const updateEnvValue = useCallback((index: number, value: string) => {
    setEnvOverrides(prev => prev.map((entry, i) => (i === index ? [entry[0], value] : entry)));
  }, []);

  const removeEnvOverride = useCallback((index: number) => {
    setEnvOverrides(prev => prev.filter((_, i) => i !== index));
  }, []);

  const handleSave = async () => {
    try {
      const status = await fetchJSON<{ services: { name: string; status: string }[] }>('/status');
      const anyRunning = status.services.some(s => s.status === 'running');

      if (anyRunning) {
        setShowConfirm(true);
        return;
      }

      await doSave();
    } catch (err) {
      onToast(`Failed to check status: ${err instanceof Error ? err.message : 'Unknown error'}`, 'error');
    }
  };

  const doSave = async () => {
    setSaving(true);
    setShowConfirm(false);

    const envObj: Record<string, string> = {};
    for (const [key, value] of envOverrides) {
      const k = key.trim();
      if (k) envObj[k] = value;
    }

    try {
      await putJSON('/config', { profiles, envOverrides: envObj });
      onToast('Xatu config saved', 'success');

      // Refresh
      const updated = await fetchJSON<XatuConfigResponse>('/config');
      setConfig(updated);
      setProfiles(updated.profiles ?? []);
      setEnvOverrides(Object.entries(updated.envOverrides ?? {}));
    } catch (err) {
      onToast(`Save failed: ${err instanceof Error ? err.message : 'Unknown error'}`, 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveAndRestart = async () => {
    setSaving(true);
    setShowConfirm(false);

    const envObj: Record<string, string> = {};
    for (const [key, value] of envOverrides) {
      const k = key.trim();
      if (k) envObj[k] = value;
    }

    try {
      await putJSON('/config', { profiles, envOverrides: envObj });
      await postJSON('/stack/restart');
      onToast('Config saved, stack restarting...', 'success');
      if (onNavigateDashboard) onNavigateDashboard();
    } catch (err) {
      onToast(`Save & restart failed: ${err instanceof Error ? err.message : 'Unknown error'}`, 'error');
    } finally {
      setSaving(false);
    }
  };

  if (error) {
    return (
      <div className="flex flex-col items-center gap-3 py-12 text-error">
        <p className="text-sm font-medium">Failed to load config</p>
        <p className="font-mono text-xs/4">{error}</p>
      </div>
    );
  }

  if (!config) return <Spinner centered text="Loading Xatu config" />;

  return (
    <div className="mx-auto max-w-3xl">
      {/* Repo path (read-only) */}
      <div className="mb-6">
        <label className="mb-1.5 block text-xs/4 font-medium text-text-muted">Repo Path</label>
        <div className="truncate rounded-xs bg-surface-lighter px-3 py-2 font-mono text-sm/5 text-text-secondary">
          {config.repoPath}
        </div>
      </div>

      {/* Profiles */}
      <div className="mb-6">
        <label className="mb-1.5 block text-xs/4 font-medium text-text-muted">Profiles</label>
        <div className="flex flex-wrap gap-1.5">
          {profiles.map(profile => (
            <span
              key={profile}
              className="inline-flex items-center gap-1 rounded-xs bg-accent/15 px-2 py-0.5 text-xs/4 font-medium text-accent-light"
            >
              {profile}
              <button
                onClick={() => removeProfile(profile)}
                className="ml-0.5 text-accent-light/50 transition-colors hover:text-accent-light"
                title={`Remove ${profile}`}
              >
                <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                </svg>
              </button>
            </span>
          ))}
        </div>
        <div className="mt-2 flex gap-2">
          <input
            type="text"
            value={newProfile}
            onChange={e => setNewProfile(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') {
                e.preventDefault();
                addProfile();
              }
            }}
            placeholder="Add profile..."
            className="flex-1 rounded-xs border border-border bg-surface-lighter px-3 py-1.5 text-sm/5 text-text-primary placeholder:text-text-disabled focus:border-accent focus:outline-hidden"
          />
          <button
            onClick={addProfile}
            disabled={!newProfile.trim()}
            className="rounded-xs border border-border px-3 py-1.5 text-sm/5 font-medium text-text-secondary transition-colors hover:border-text-muted hover:text-text-primary disabled:opacity-50"
          >
            Add
          </button>
        </div>
      </div>

      {/* Env Overrides */}
      <div className="mb-8">
        <label className="mb-1.5 block text-xs/4 font-medium text-text-muted">Environment Overrides</label>
        <div className="flex flex-col gap-2">
          {envOverrides.map(([key, value], index) => (
            <div key={index} className="flex items-center gap-2">
              <input
                type="text"
                value={key}
                onChange={e => updateEnvKey(index, e.target.value)}
                placeholder="KEY"
                className="w-48 shrink-0 rounded-xs border border-border bg-surface-lighter px-3 py-1.5 font-mono text-sm/5 text-text-primary placeholder:text-text-disabled focus:border-accent focus:outline-hidden"
              />
              <span className="text-text-disabled">=</span>
              <input
                type="text"
                value={value}
                onChange={e => updateEnvValue(index, e.target.value)}
                placeholder="value"
                className="min-w-0 flex-1 rounded-xs border border-border bg-surface-lighter px-3 py-1.5 font-mono text-sm/5 text-text-primary placeholder:text-text-disabled focus:border-accent focus:outline-hidden"
              />
              <button
                onClick={() => removeEnvOverride(index)}
                className="shrink-0 p-1 text-text-disabled transition-colors hover:text-error"
                title="Remove"
              >
                <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          ))}
        </div>
        <button
          onClick={addEnvOverride}
          className="mt-2 flex items-center gap-1 text-xs/4 text-text-tertiary transition-colors hover:text-accent-light"
        >
          <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          Add override
        </button>
      </div>

      {/* Save */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={saving}
          className="rounded-xs bg-accent px-4 py-2 text-sm/5 font-medium text-text-primary transition-colors hover:bg-accent/80 disabled:opacity-50"
        >
          {saving ? 'Saving...' : 'Save'}
        </button>
      </div>

      {/* Confirm dialog */}
      {showConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60">
          <div className="mx-4 max-w-sm rounded-sm border border-border bg-surface p-6">
            <h3 className="mb-2 text-sm/5 font-semibold text-text-primary">Stack is running</h3>
            <p className="mb-4 text-xs/4 text-text-tertiary">
              The stack has running services. Would you like to save and restart, or just save?
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowConfirm(false)}
                className="rounded-xs px-3 py-1.5 text-sm/5 text-text-tertiary transition-colors hover:text-text-primary"
              >
                Cancel
              </button>
              <button
                onClick={doSave}
                className="rounded-xs border border-border px-3 py-1.5 text-sm/5 font-medium text-text-secondary transition-colors hover:text-text-primary"
              >
                Save Only
              </button>
              <button
                onClick={handleSaveAndRestart}
                className="rounded-xs bg-accent px-3 py-1.5 text-sm/5 font-medium text-text-primary transition-colors hover:bg-accent/80"
              >
                Save & Restart
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
