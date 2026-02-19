import { useEffect, useRef } from 'react';

type StackState = 'running' | 'starting' | 'stopping' | 'error' | 'stopped' | null;

const COLORS: Record<string, string> = {
  running: '#34d399', // emerald-400
  starting: '#fbbf24', // amber-400
  stopping: '#fbbf24', // amber-400
  error: '#f87171', // red-400
  stopped: '#6b7280', // gray-500
};

function drawFavicon(color: string): string {
  const size = 64;
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d');
  if (!ctx) return '';

  // Filled circle
  ctx.beginPath();
  ctx.arc(size / 2, size / 2, size / 2 - 2, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();

  return canvas.toDataURL('image/png');
}

export function useFavicon(stackStatus: string | null, hasError: boolean) {
  const linkRef = useRef<HTMLLinkElement | null>(null);

  useEffect(() => {
    let state: StackState = stackStatus as StackState;
    if (hasError) state = 'error';
    if (!state) state = 'stopped';

    const color = COLORS[state] ?? COLORS.stopped;
    const href = drawFavicon(color);
    if (!href) return;

    if (!linkRef.current) {
      // Remove any existing favicon links
      const existing = document.querySelectorAll("link[rel='icon']");
      existing.forEach(el => el.remove());

      const link = document.createElement('link');
      link.rel = 'icon';
      link.type = 'image/png';
      document.head.appendChild(link);
      linkRef.current = link;
    }

    linkRef.current.href = href;
  }, [stackStatus, hasError]);
}
