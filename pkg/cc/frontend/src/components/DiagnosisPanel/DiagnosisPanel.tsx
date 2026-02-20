import { useCallback, useEffect } from 'react';
import type { AIDiagnosis } from '@/types';
import Spinner from '@/components/Spinner';

interface DiagnosisPanelProps {
  serviceName: string;
  diagnosis: AIDiagnosis | null;
  error: string | null;
  loading: boolean;
  onClose: () => void;
  onRetry: () => void;
}

export default function DiagnosisPanel({
  serviceName,
  diagnosis,
  error,
  loading,
  onClose,
  onRetry,
}: DiagnosisPanelProps) {
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose]
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60" onClick={handleBackdropClick}>
      <div className="relative flex max-h-[80vh] w-full max-w-2xl flex-col overflow-hidden rounded-sm border border-border bg-surface">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-5 py-3">
          <div className="flex items-center gap-2">
            <svg
              className="size-4 text-accent-light"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 0 0-2.455 2.456ZM16.894 20.567 16.5 21.75l-.394-1.183a2.25 2.25 0 0 0-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 0 0 1.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 0 0 1.423 1.423l1.183.394-1.183.394a2.25 2.25 0 0 0-1.423 1.423Z"
              />
            </svg>
            <h2 className="text-sm/5 font-semibold text-text-primary">Diagnosis: {serviceName}</h2>
          </div>
          <button
            onClick={onClose}
            className="rounded-xs p-1 text-text-disabled transition-colors hover:bg-hover/10 hover:text-text-secondary"
          >
            <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {loading && <Spinner text="Analyzing logs with Claude..." centered />}

          {error && (
            <div className="flex flex-col items-center gap-4 py-8">
              <svg
                className="size-10 text-error"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1.5}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="m9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
                />
              </svg>
              <p className="text-sm/5 font-medium text-error">Diagnosis failed</p>
              <p className="max-w-md text-center font-mono text-xs/5 text-error/70">{error}</p>
              <button
                onClick={onRetry}
                className="mt-1 rounded-xs bg-accent/15 px-3 py-1.5 text-xs/4 font-medium text-accent-light transition-colors hover:bg-accent/25"
              >
                Retry
              </button>
            </div>
          )}

          {diagnosis && (
            <div className="flex flex-col gap-5">
              {/* Root Cause */}
              <Section title="Root Cause">
                <p className="rounded-xs bg-error/10 px-3 py-2 text-sm/5 font-medium text-error">
                  <FormattedText text={diagnosis.rootCause} />
                </p>
              </Section>

              {/* Explanation */}
              <Section title="Explanation">
                <p className="text-sm/6 text-text-secondary">
                  <FormattedText text={diagnosis.explanation} />
                </p>
              </Section>

              {/* Affected Files */}
              {diagnosis.affectedFiles.length > 0 && (
                <Section title="Affected Files">
                  <ul className="flex flex-col gap-1">
                    {diagnosis.affectedFiles.map((file, i) => (
                      <li key={i} className="font-mono text-xs/5 text-accent-light">
                        <FormattedText text={file} />
                      </li>
                    ))}
                  </ul>
                </Section>
              )}

              {/* Suggestions */}
              {diagnosis.suggestions.length > 0 && (
                <Section title="Suggestions">
                  <ol className="flex flex-col gap-1.5 pl-5" style={{ listStyleType: 'decimal' }}>
                    {diagnosis.suggestions.map((suggestion, i) => (
                      <li key={i} className="text-sm/5 text-text-secondary">
                        <FormattedText text={suggestion} />
                      </li>
                    ))}
                  </ol>
                </Section>
              )}

              {/* Fix Commands */}
              {diagnosis.fixCommands.length > 0 && (
                <Section title="Fix Commands">
                  <div className="flex flex-col gap-2">
                    {diagnosis.fixCommands.map((cmd, i) => (
                      <CopyableCommand key={i} command={cmd} />
                    ))}
                  </div>
                </Section>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function FormattedText({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`)/g);
  return (
    <>
      {parts.map((part, i) =>
        part.startsWith('`') && part.endsWith('`') ? (
          <code key={i} className="rounded-xs bg-surface-light px-1 py-0.5 font-mono text-accent-light">
            {part.slice(1, -1)}
          </code>
        ) : (
          <span key={i}>{part}</span>
        )
      )}
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">{title}</h3>
      {children}
    </div>
  );
}

function CopyableCommand({ command }: { command: string }) {
  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(command).catch(console.error);
  }, [command]);

  return (
    <div className="group flex items-center gap-2 rounded-xs bg-surface-light px-3 py-2 font-mono text-xs/5 text-text-secondary">
      <span className="flex-1 break-all select-all">$ {command}</span>
      <button
        onClick={handleCopy}
        className="shrink-0 rounded-xs p-1 text-text-disabled opacity-0 transition-all group-hover:opacity-100 hover:bg-hover/10 hover:text-text-tertiary"
        title="Copy command"
      >
        <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M15.666 3.888A2.25 2.25 0 0 0 13.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 0 1-.75.75H9.75a.75.75 0 0 1-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 0 1-2.25 2.25H6.75A2.25 2.25 0 0 1 4.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 0 1 1.927-.184"
          />
        </svg>
      </button>
    </div>
  );
}
