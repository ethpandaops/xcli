import { useState, useEffect } from 'react';
import Dashboard from '@/components/Dashboard';
import ConfigPage from '@/components/ConfigPage';
import type { StackInfo } from '@/types';

type Page = 'dashboard' | 'config';

export default function App() {
  const [page, setPage] = useState<Page>('dashboard');
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

  if (page === 'config') {
    return <ConfigPage onBack={() => setPage('dashboard')} stack={activeStack} />;
  }

  return (
    <Dashboard
      onNavigateConfig={() => setPage('config')}
      stack={activeStack}
      availableStacks={availableStacks}
      onSwitchStack={setActiveStack}
    />
  );
}
