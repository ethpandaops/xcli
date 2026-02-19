import { useState } from 'react';
import Dashboard from '@/components/Dashboard';
import ConfigPage from '@/components/ConfigPage';

type Page = 'dashboard' | 'config';

export default function App() {
  const [page, setPage] = useState<Page>('dashboard');

  if (page === 'config') {
    return <ConfigPage onBack={() => setPage('dashboard')} />;
  }

  return <Dashboard onNavigateConfig={() => setPage('config')} />;
}
