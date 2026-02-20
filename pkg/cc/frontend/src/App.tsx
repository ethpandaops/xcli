import { useState, useEffect, useCallback } from 'react';
import Dashboard from '@/components/Dashboard';
import ConfigPage from '@/components/ConfigPage';
import RedisPage from '@/components/RedisPage';
import type { StackInfo } from '@/types';

type Page = 'dashboard' | 'config' | 'redis';
type ConfigTab = 'lab' | 'services' | 'overrides';

export default function App() {
  const [page, setPage] = useState<Page>('dashboard');
  const [configTab, setConfigTab] = useState<ConfigTab>('lab');
  const [activeStack, setActiveStack] = useState('lab');
  const [availableStacks, setAvailableStacks] = useState<string[]>(['lab']);

  useEffect(() => {
    fetch('/api/stacks')
      .then(res => {
        if (!res.ok) throw new Error('Failed to fetch stacks');

        return res.json() as Promise<StackInfo[]>;
      })
      .then(stacks => {
        if (stacks.length > 0) {
          setAvailableStacks(stacks.map(s => s.name));
          setActiveStack(stacks[0].name);
        }
      })
      .catch(() => {
        // Fallback to default — backend may not support the endpoint yet
      });
  }, []);

  const navigateConfig = useCallback((tab: ConfigTab = 'lab') => {
    setConfigTab(tab);
    setPage('config');
  }, []);

  if (page === 'config') {
    return <ConfigPage onBack={() => setPage('dashboard')} stack={activeStack} initialTab={configTab} />;
  }

  if (page === 'redis') {
    return (
      <RedisPage onBack={() => setPage('dashboard')} onNavigateConfig={() => navigateConfig()} stack={activeStack} />
    );
  }

  return (
    <Dashboard
      onNavigateConfig={() => navigateConfig()}
      onNavigateOverrides={() => navigateConfig('overrides')}
      onNavigateRedis={() => setPage('redis')}
      stack={activeStack}
      availableStacks={availableStacks}
      onSwitchStack={setActiveStack}
    />
  );
}
