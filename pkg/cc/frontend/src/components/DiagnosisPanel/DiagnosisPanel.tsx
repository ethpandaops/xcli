import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { AIProviderInfo, DiagnosisReport, DiagnosisTurn } from '@/types';
import Spinner from '@/components/Spinner';
import Markdown from '@/components/Markdown';

interface DiagnosisPanelProps {
  serviceName: string;
  providers: AIProviderInfo[];
  selectedProvider: string;
  onProviderChange: (provider: string) => void;
  sessionId: string | null;
  completedTurns: DiagnosisTurn[];
  currentTurnPrompt?: string;
  thinkingText: string;
  activityText: string;
  answerText: string;
  diagnosis: DiagnosisReport | null;
  error: string | null;
  loading: boolean;
  canInterrupt: boolean;
  canInteract: boolean;
  onInterrupt: () => void;
  onSendFollowUp: (prompt: string) => void;
  onClose: () => void;
  onRetry: () => void;
}

export default function DiagnosisPanel({
  serviceName,
  providers,
  selectedProvider,
  onProviderChange,
  sessionId,
  completedTurns,
  currentTurnPrompt,
  thinkingText,
  activityText,
  answerText,
  diagnosis,
  error,
  loading,
  canInterrupt,
  canInteract,
  onInterrupt,
  onSendFollowUp,
  onClose,
  onRetry,
}: DiagnosisPanelProps) {
  const [prompt, setPrompt] = useState('');
  const streamRef = useRef<HTMLDivElement | null>(null);
  const affectedFiles = diagnosis?.affectedFiles ?? [];
  const suggestions = diagnosis?.suggestions ?? [];
  const fixCommands = diagnosis?.fixCommands ?? [];
  const rootCause = diagnosis?.rootCause ?? '';
  const explanation = diagnosis?.explanation ?? '';
  const streamStatus = loading ? 'Running' : error ? 'Failed' : diagnosis ? 'Complete' : 'Idle';

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose]
  );

  const canSend = useMemo(() => {
    return canInteract && !loading && prompt.trim() !== '' && !!sessionId;
  }, [canInteract, loading, prompt, sessionId]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  useEffect(() => {
    const el = streamRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [thinkingText, activityText, answerText]);

  const hasCurrentContent = !!(thinkingText || activityText.trim() || answerText.trim());
  const isEmpty = completedTurns.length === 0 && !hasCurrentContent;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60" onClick={handleBackdropClick}>
      <div className="relative mx-2 flex h-[95vh] w-[98vw] max-w-none flex-col overflow-hidden rounded-sm border border-border bg-surface">
        <div className="flex items-center justify-between gap-3 border-b border-border px-5 py-3">
          <div className="flex min-w-0 items-center gap-3">
            <svg
              className="size-4 shrink-0 text-accent-light"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09Z"
              />
            </svg>
            <h2 className="truncate text-sm/5 font-semibold text-text-primary">Diagnosis: {serviceName}</h2>
            <span className="rounded-xs border border-border bg-surface-light px-2 py-1 text-[11px]/4 text-text-muted uppercase">
              {streamStatus}
            </span>
            <select
              value={selectedProvider}
              onChange={e => onProviderChange(e.target.value)}
              disabled={loading}
              className="rounded-xs border border-border bg-surface-light px-2 py-1 text-xs/4 text-text-secondary"
            >
              {providers.map(provider => (
                <option key={provider.id} value={provider.id} disabled={!provider.available}>
                  {provider.label} {!provider.available ? '(Unavailable)' : ''}
                </option>
              ))}
            </select>
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

        <div
          className={`grid flex-1 grid-cols-1 gap-0 overflow-hidden ${diagnosis ? 'lg:grid-cols-[1.1fr_0.9fr]' : ''}`}
        >
          <div
            className={`flex min-h-0 flex-col px-5 py-4 ${diagnosis ? 'border-b border-border lg:border-r lg:border-b-0' : ''}`}
          >
            <div ref={streamRef} className="flex flex-col overflow-y-auto">
              <h3 className="mb-2 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Live Stream</h3>

              {loading && isEmpty && <Spinner text="Streaming diagnosis..." centered />}

              {!loading && isEmpty && (
                <div className="rounded-xs border border-border/50 bg-surface-light px-3 py-2 text-xs/5 text-text-muted">
                  Waiting for streamed output...
                </div>
              )}

              {completedTurns.map((turn, i) => (
                <TurnOutput key={i} turn={turn} />
              ))}

              {currentTurnPrompt && (
                <div className="mt-4 mb-3 border-t border-border/40 pt-4">
                  <span className="rounded-xs bg-accent/15 px-2 py-1 text-xs/4 font-medium text-accent-light">
                    Follow-up
                  </span>
                  <p className="mt-2 text-xs/5 text-text-secondary">{currentTurnPrompt}</p>
                </div>
              )}

              {loading && !isEmpty && !hasCurrentContent && <Spinner text="Streaming..." centered />}

              {thinkingText && (
                <div className="mb-3">
                  <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Thinking</h4>
                  <pre className="rounded-xs border border-border/50 bg-surface-light px-3 py-2 font-mono text-xs/5 whitespace-pre-wrap text-warning">
                    {thinkingText}
                  </pre>
                </div>
              )}

              {activityText.trim() !== '' && (
                <div className="mb-3">
                  <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Activity</h4>
                  <pre className="rounded-xs border border-border/50 bg-surface-light px-3 py-2 font-mono text-xs/5 whitespace-pre-wrap text-text-muted">
                    {activityText}
                  </pre>
                </div>
              )}

              {answerText.trim() !== '' && (
                <div className="mt-3 flex flex-col">
                  <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Answer</h4>
                  <Markdown className="rounded-xs border border-border/50 bg-surface-light px-3 py-2">
                    {answerText}
                  </Markdown>
                </div>
              )}
            </div>

            {error && (
              <div className="mt-4 rounded-xs border border-error/40 bg-error/10 px-3 py-2">
                <p className="text-xs/4 font-medium text-error">Diagnosis failed</p>
                <p className="mt-1 font-mono text-xs/5 text-error/80">{error}</p>
                <button
                  onClick={onRetry}
                  className="mt-2 rounded-xs bg-accent/15 px-3 py-1.5 text-xs/4 font-medium text-accent-light transition-colors hover:bg-accent/25"
                >
                  Retry
                </button>
              </div>
            )}
          </div>

          {diagnosis && (
            <div className="flex min-h-0 flex-col overflow-y-auto px-5 py-4">
              <div className="flex flex-col gap-5">
                <Section title="Root Cause">
                  <p className="rounded-xs bg-error/10 px-3 py-2 text-sm/5 font-medium text-error">
                    <FormattedText text={rootCause} />
                  </p>
                </Section>

                <Section title="Explanation">
                  <p className="text-sm/6 text-text-secondary">
                    <FormattedText text={explanation} />
                  </p>
                </Section>

                {affectedFiles.length > 0 && (
                  <Section title="Affected Files">
                    <ul className="flex flex-col gap-1">
                      {affectedFiles.map((file, i) => (
                        <li key={i} className="font-mono text-xs/5 text-accent-light">
                          <FormattedText text={file} />
                        </li>
                      ))}
                    </ul>
                  </Section>
                )}

                {suggestions.length > 0 && (
                  <Section title="Suggestions">
                    <ol className="flex flex-col gap-1.5 pl-5" style={{ listStyleType: 'decimal' }}>
                      {suggestions.map((suggestion, i) => (
                        <li key={i} className="text-sm/5 text-text-secondary">
                          <FormattedText text={suggestion} />
                        </li>
                      ))}
                    </ol>
                  </Section>
                )}

                {fixCommands.length > 0 && (
                  <Section title="Fix Commands">
                    <div className="flex flex-col gap-2">
                      {fixCommands.map((cmd, i) => (
                        <CopyableCommand key={i} command={cmd} />
                      ))}
                    </div>
                  </Section>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="border-t border-border bg-surface px-5 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="text"
              value={prompt}
              onChange={e => setPrompt(e.target.value)}
              placeholder={canInteract ? 'Ask a follow-up question...' : 'Provider does not support session follow-ups'}
              disabled={!canInteract || !sessionId}
              onKeyDown={e => {
                if (e.key === 'Enter' && canSend) {
                  onSendFollowUp(prompt.trim());
                  setPrompt('');
                }
              }}
              className="flex-1 rounded-xs border border-border bg-surface-light px-3 py-2 text-sm/5 text-text-secondary placeholder:text-text-disabled"
            />
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  if (!canSend) return;
                  onSendFollowUp(prompt.trim());
                  setPrompt('');
                }}
                disabled={!canSend}
                className="rounded-xs bg-accent/20 px-3 py-2 text-xs/4 font-medium text-accent-light transition-colors hover:bg-accent/30 disabled:cursor-not-allowed disabled:opacity-40"
              >
                Send
              </button>
              {canInterrupt && (
                <button
                  onClick={onInterrupt}
                  disabled={!loading || !sessionId}
                  className="rounded-xs border border-warning/30 bg-warning/20 px-3 py-2 text-xs/4 font-medium text-warning transition-colors hover:bg-warning/30 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  Interrupt
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function TurnOutput({ turn }: { turn: DiagnosisTurn }) {
  return (
    <div>
      {turn.prompt && (
        <div className="mt-4 mb-3 border-t border-border/40 pt-4">
          <span className="rounded-xs bg-accent/15 px-2 py-1 text-xs/4 font-medium text-accent-light">Follow-up</span>
          <p className="mt-2 text-xs/5 text-text-secondary">{turn.prompt}</p>
        </div>
      )}

      {turn.thinking && (
        <div className="mb-3">
          <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Thinking</h4>
          <pre className="rounded-xs border border-border/50 bg-surface-light px-3 py-2 font-mono text-xs/5 whitespace-pre-wrap text-warning">
            {turn.thinking}
          </pre>
        </div>
      )}

      {turn.activity.trim() !== '' && (
        <div className="mb-3">
          <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Activity</h4>
          <pre className="rounded-xs border border-border/50 bg-surface-light px-3 py-2 font-mono text-xs/5 whitespace-pre-wrap text-text-muted">
            {turn.activity}
          </pre>
        </div>
      )}

      {turn.answer.trim() !== '' && (
        <div className="mt-3 flex flex-col">
          <h4 className="mb-1 text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Answer</h4>
          <Markdown className="rounded-xs border border-border/50 bg-surface-light px-3 py-2">{turn.answer}</Markdown>
        </div>
      )}
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
