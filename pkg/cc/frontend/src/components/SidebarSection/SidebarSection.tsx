import { useState, useRef, useEffect, useCallback } from 'react';

interface SidebarSectionProps {
  title: string;
  storageKey: string;
  defaultOpen?: boolean;
  action?: { label: string; onClick: () => void };
  children: React.ReactNode;
}

export default function SidebarSection({
  title,
  storageKey,
  defaultOpen = true,
  action,
  children,
}: SidebarSectionProps) {
  const [open, setOpen] = useState(() => {
    try {
      const stored = localStorage.getItem(storageKey);
      if (stored !== null) return stored === 'true';
    } catch {
      /* ignore */
    }
    return defaultOpen;
  });

  const contentRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState<number | undefined>(undefined);

  useEffect(() => {
    if (!contentRef.current) return;
    const el = contentRef.current;
    const ro = new ResizeObserver(() => {
      setHeight(el.scrollHeight);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const toggle = useCallback(() => {
    setOpen(prev => {
      const next = !prev;
      try {
        localStorage.setItem(storageKey, String(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  }, [storageKey]);

  return (
    <div className="rounded-xs border border-border bg-surface-light">
      <button onClick={toggle} className="flex w-full items-center justify-between px-4 py-3 text-left">
        <span className="text-sm/5 font-semibold text-text-tertiary">{title}</span>
        <div className="flex items-center gap-2">
          {action && (
            <span
              role="button"
              tabIndex={0}
              onClick={e => {
                e.stopPropagation();
                action.onClick();
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.stopPropagation();
                  action.onClick();
                }
              }}
              className="text-xs/4 text-accent-light transition-colors hover:text-accent-light/80"
            >
              {action.label}
            </span>
          )}
          <svg
            className={`size-3.5 text-text-disabled transition-transform duration-200 ${open ? 'rotate-0' : '-rotate-90'}`}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
          </svg>
        </div>
      </button>

      <div
        className="overflow-hidden transition-[max-height] duration-200 ease-in-out"
        style={{ maxHeight: open ? (height ?? 9999) : 0 }}
      >
        <div ref={contentRef} className="px-4 pb-4">
          {children}
        </div>
      </div>
    </div>
  );
}
