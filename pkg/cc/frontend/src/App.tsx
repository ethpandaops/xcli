import { useState, useEffect, useCallback } from 'react';
import Dashboard from '@/components/Dashboard';
import ConfigPage from '@/components/ConfigPage';
import RedisPage from '@/components/RedisPage';
import type { StackInfo, StackCapabilities, StackStatus } from '@/types';

type Page = 'dashboard' | 'config' | 'redis';
type ConfigTab = 'lab' | 'xatu' | 'services' | 'overrides';

const defaultCapabilities: StackCapabilities = {
  hasEditableConfig: true,
  hasServiceConfigs: true,
  hasCbtOverrides: true,
  hasRedis: true,
  hasGitRepos: true,
  hasRegenerate: true,
  hasRebuild: true,
};

const STACK_STORAGE_KEY = 'xcli:active-stack';

/** Fetch the status of a single stack, returning null on error. */
function fetchStackStatus(name: string): Promise<StackStatus | null> {
  return fetch(`/api/stacks/${name}/stack/status`)
    .then(r => (r.ok ? (r.json() as Promise<StackStatus>) : null))
    .catch(() => null);
}

export default function App() {
  const [page, setPage] = useState<Page>('dashboard');
  const [configTab, setConfigTab] = useState<ConfigTab>('lab');
  const [activeStack, setActiveStack] = useState('lab');
  const [availableStacks, setAvailableStacks] = useState<string[]>(['lab']);
  const [capabilitiesMap, setCapabilitiesMap] = useState<Record<string, StackCapabilities>>({});
  const [runningStack, setRunningStack] = useState<string | null>(null);

  const capabilities = capabilitiesMap[activeStack] ?? defaultCapabilities;

  // Persist stack selection to localStorage on change.
  const handleSwitchStack = useCallback((stack: string) => {
    setActiveStack(stack);
    localStorage.setItem(STACK_STORAGE_KEY, stack);
  }, []);

  // Initial load: fetch stacks, pick initial active stack, track running stack.
  useEffect(() => {
    fetch('/api/stacks')
      .then(res => {
        if (!res.ok) throw new Error('Failed to fetch stacks');

        return res.json() as Promise<StackInfo[]>;
      })
      .then(async stacks => {
        if (stacks.length === 0) return;

        const names = stacks.map(s => s.name);
        setAvailableStacks(names);

        const caps: Record<string, StackCapabilities> = {};
        for (const s of stacks) {
          caps[s.name] = s.capabilities;
        }
        setCapabilitiesMap(caps);

        // Check each stack's status to find one that's running.
        const statuses = await Promise.all(names.map(fetchStackStatus));

        const runningIdx = statuses.findIndex(s => s?.status === 'running');
        if (runningIdx !== -1) {
          setRunningStack(names[runningIdx]);
          setActiveStack(names[runningIdx]);
          localStorage.setItem(STACK_STORAGE_KEY, names[runningIdx]);

          return;
        }

        setRunningStack(null);

        const saved = localStorage.getItem(STACK_STORAGE_KEY);
        if (saved && names.includes(saved)) {
          setActiveStack(saved);

          return;
        }

        setActiveStack(names[0]);
      })
      .catch(() => {
        // Fallback to default — backend may not support the endpoint yet
      });
  }, []);

  // Track the active stack's status via callback from Dashboard.
  // When the active stack starts running, record it as the running stack.
  // When it stops, clear the running stack.
  const handleStackStatusChange = useCallback(
    (status: string) => {
      if (status === 'running') {
        setRunningStack(activeStack);
      } else if (status === 'stopped' && runningStack === activeStack) {
        setRunningStack(null);
      }
    },
    [activeStack, runningStack]
  );

  // Compute what to pass to Dashboard: name of running stack that isn't the active one.
  const otherRunningStack = runningStack && runningStack !== activeStack ? runningStack : null;

  const navigateConfig = useCallback(
    (tab?: ConfigTab) => {
      if (tab) {
        setConfigTab(tab);
      } else {
        // Pick an appropriate default tab based on capabilities
        const caps = capabilitiesMap[activeStack] ?? defaultCapabilities;
        if (caps.hasEditableConfig && caps.hasServiceConfigs) {
          setConfigTab('lab');
        } else if (caps.hasEditableConfig) {
          setConfigTab(activeStack === 'xatu' ? 'xatu' : 'lab');
        } else {
          setConfigTab('services');
        }
      }
      setPage('config');
    },
    [activeStack, capabilitiesMap]
  );

  if (page === 'config') {
    return (
      <ConfigPage
        onBack={() => setPage('dashboard')}
        stack={activeStack}
        initialTab={configTab}
        capabilities={capabilities}
      />
    );
  }

  if (page === 'redis') {
    return (
      <RedisPage onBack={() => setPage('dashboard')} onNavigateConfig={() => navigateConfig()} stack={activeStack} />
    );
  }

  return (
    <Dashboard
      key={activeStack}
      onNavigateConfig={() => navigateConfig()}
      onNavigateOverrides={() => navigateConfig('overrides')}
      onNavigateRedis={capabilities.hasRedis ? () => setPage('redis') : undefined}
      stack={activeStack}
      availableStacks={availableStacks}
      onSwitchStack={handleSwitchStack}
      capabilities={capabilities}
      otherRunningStack={otherRunningStack}
      onStackStatusChange={handleStackStatusChange}
    />
  );
}
