import React from 'react';
import type { InfraInfo } from '@/types';

interface InfraPanelProps {
  infrastructure: InfraInfo[];
}

export default function InfraPanel({ infrastructure }: InfraPanelProps): React.JSX.Element {
  if (infrastructure.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        <h3 className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Infrastructure</h3>
        <p className="px-3 text-xs/4 text-text-disabled">No infrastructure found</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Infrastructure</h3>
      {infrastructure.map(item => {
        const statusColor = item.status === 'running' ? 'bg-success' : 'bg-text-disabled';

        return (
          <div key={item.name} className="rounded-xs px-3 py-2.5">
            <div className="flex items-center gap-2.5">
              <span className={`size-1.5 shrink-0 rounded-full ${statusColor}`} />
              <span className="min-w-0 flex-1 truncate text-sm/5 font-medium text-text-secondary">{item.name}</span>
              <svg
                className={`size-3.5 shrink-0 ${item.status === 'running' ? 'text-success' : 'text-text-disabled'}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1.5}
              >
                <title>{item.status}</title>
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d={
                    item.status === 'running'
                      ? 'M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z'
                      : 'M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 5.25h.008v.008H12v-.008Z'
                  }
                />
              </svg>
            </div>
          </div>
        );
      })}
    </div>
  );
}
