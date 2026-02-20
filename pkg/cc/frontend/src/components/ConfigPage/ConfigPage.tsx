import { useState, useEffect, useRef } from 'react';
import { useAPI } from '@/hooks/useAPI';
import LabConfigEditor from '@/components/LabConfigEditor';
import ServiceConfigViewer from '@/components/ServiceConfigViewer';
import CBTOverridesEditor from '@/components/CBTOverridesEditor';

type Tab = 'lab' | 'services' | 'overrides';

interface ConfigPageProps {
  onBack: () => void;
  stack: string;
  initialTab?: Tab;
}

const tabs: { key: Tab; label: string; icon: React.ReactNode }[] = [
  {
    key: 'lab',
    label: 'Lab Config',
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
        />
        <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
      </svg>
    ),
  },
  {
    key: 'services',
    label: 'Service Configs',
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z"
        />
      </svg>
    ),
  },
  {
    key: 'overrides',
    label: 'CBT Overrides',
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M10.5 6h9.75M10.5 6a1.5 1.5 0 1 1-3 0m3 0a1.5 1.5 0 1 0-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 0 1-3 0m3 0a1.5 1.5 0 0 0-3 0m-9.75 0h9.75"
        />
      </svg>
    ),
  },
];

export default function ConfigPage({ onBack, stack, initialTab = 'lab' }: ConfigPageProps) {
  const { postJSON } = useAPI(stack);
  const [activeTab, setActiveTab] = useState<Tab>(initialTab);
  const [regenerating, setRegenerating] = useState(false);
  const [toast, setToast] = useState<{
    message: string;
    type: 'success' | 'error';
  } | null>(null);
  const toastTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  const showToast = (message: string, type: 'success' | 'error') => {
    if (toastTimeout.current) clearTimeout(toastTimeout.current);
    setToast({ message, type });
    toastTimeout.current = setTimeout(() => setToast(null), 4000);
  };

  useEffect(() => {
    return () => {
      if (toastTimeout.current) clearTimeout(toastTimeout.current);
    };
  }, []);

  const handleRegenerate = async () => {
    setRegenerating(true);

    try {
      await postJSON<{ status: string }>('/config/regenerate');
      showToast('Configs regenerated successfully', 'success');
    } catch (err) {
      showToast(`Regeneration failed: ${err instanceof Error ? err.message : 'Unknown error'}`, 'error');
    } finally {
      setRegenerating(false);
    }
  };

  return (
    <div className="flex h-dvh flex-col bg-bg">
      {/* Header */}
      <header className="flex items-center justify-between border-b border-border bg-surface px-6 py-3">
        <div className="flex items-center gap-3">
          <button
            onClick={onBack}
            className="group flex items-center gap-2 rounded-xs py-1 pr-2 text-text-tertiary transition-colors hover:text-text-primary"
            title="Back to Dashboard"
          >
            <svg
              className="size-4 transition-transform group-hover:-translate-x-0.5"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={2}
              stroke="currentColor"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5 3 12m0 0 7.5-7.5M3 12h18" />
            </svg>
            <span className="text-sm/5 font-medium">Dashboard</span>
          </button>
          <span className="text-border">{'/'}</span>
          <h1 className="text-sm/5 font-semibold text-text-primary">Config Management</h1>
        </div>

        <button
          onClick={handleRegenerate}
          disabled={regenerating}
          className="flex items-center gap-1.5 rounded-xs border border-border px-3 py-1.5 text-xs/4 font-medium text-text-secondary transition-colors hover:border-text-muted hover:text-text-primary disabled:opacity-50"
        >
          <svg
            className={`size-3.5 ${regenerating ? 'animate-spin' : ''}`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182M2.985 19.644l3.181-3.183"
            />
          </svg>
          {regenerating ? 'Regenerating...' : 'Regenerate'}
        </button>
      </header>

      {/* Tabs */}
      <div className="border-b border-border bg-surface">
        <nav className="flex px-6" role="tablist">
          {tabs.map(tab => {
            const isActive = activeTab === tab.key;

            return (
              <button
                key={tab.key}
                role="tab"
                aria-selected={isActive}
                onClick={() => setActiveTab(tab.key)}
                className={`relative flex items-center gap-2 px-4 py-3 text-sm/5 font-medium transition-colors ${
                  isActive ? 'text-text-primary' : 'text-text-muted hover:text-text-secondary'
                }`}
              >
                <span className={isActive ? 'text-accent-light' : 'text-text-disabled'}>{tab.icon}</span>
                {tab.label}
                {isActive && <span className="absolute inset-x-4 -bottom-px h-0.5 rounded-full bg-accent" />}
              </button>
            );
          })}
        </nav>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        {activeTab === 'lab' && <LabConfigEditor onToast={showToast} onNavigateDashboard={onBack} stack={stack} />}
        {activeTab === 'services' && <ServiceConfigViewer onToast={showToast} stack={stack} />}
        {activeTab === 'overrides' && <CBTOverridesEditor onToast={showToast} stack={stack} />}
      </div>

      {/* Toast */}
      {toast && (
        <div
          className={`animate-in slide-in-from-bottom-2 fade-in fixed right-6 bottom-6 flex items-center gap-2 rounded-sm px-4 py-2.5 text-sm/5 font-medium shadow-lg duration-200 ${
            toast.type === 'success'
              ? 'bg-success/15 text-success ring-1 ring-success/25'
              : 'bg-error/15 text-error ring-1 ring-error/25'
          }`}
        >
          {toast.type === 'success' ? (
            <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
              />
            </svg>
          ) : (
            <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z"
              />
            </svg>
          )}
          {toast.message}
        </div>
      )}
    </div>
  );
}
