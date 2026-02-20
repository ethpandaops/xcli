import { useState } from 'react';
import Dashboard from '@/components/Dashboard';
import ConfigPage from '@/components/ConfigPage';
import RedisPage from '@/components/RedisPage';

type Page = 'dashboard' | 'config' | 'redis';

export default function App() {
  const [page, setPage] = useState<Page>('dashboard');

  if (page === 'config') {
    return <ConfigPage onBack={() => setPage('dashboard')} />;
  }

  if (page === 'redis') {
    return <RedisPage onBack={() => setPage('dashboard')} onNavigateConfig={() => setPage('config')} />;
  }

  return <Dashboard onNavigateConfig={() => setPage('config')} onNavigateRedis={() => setPage('redis')} />;
}
