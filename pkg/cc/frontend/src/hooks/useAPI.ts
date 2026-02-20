import { useCallback } from 'react';

export function useAPI(stack: string) {
  const prefix = `/api/stacks/${stack}`;

  const fetchJSON = useCallback(
    async <T>(url: string): Promise<T> => {
      const res = await fetch(`${prefix}${url}`);

      if (!res.ok) {
        const body = await res.text();
        throw new Error(`${res.status}: ${body}`);
      }

      return res.json() as Promise<T>;
    },
    [prefix]
  );

  const postAction = useCallback(
    async (service: string, action: string): Promise<void> => {
      const res = await fetch(`${prefix}/services/${service}/${action}`, {
        method: 'POST',
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error((body as { error?: string }).error ?? 'Action failed');
      }
    },
    [prefix]
  );

  const putJSON = useCallback(
    async <T>(url: string, body: unknown): Promise<T> => {
      const res = await fetch(`${prefix}${url}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const errBody = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error((errBody as { error?: string }).error ?? 'Request failed');
      }

      return res.json() as Promise<T>;
    },
    [prefix]
  );

  const postJSON = useCallback(
    async <T>(url: string): Promise<T> => {
      const res = await fetch(`${prefix}${url}`, { method: 'POST' });

      if (!res.ok) {
        const errBody = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error((errBody as { error?: string }).error ?? 'Request failed');
      }

      return res.json() as Promise<T>;
    },
    [prefix]
  );

  const deleteAction = useCallback(
    async (url: string): Promise<void> => {
      const res = await fetch(`${prefix}${url}`, { method: 'DELETE' });

      if (!res.ok) {
        const errBody = await res.json().catch(() => ({ error: res.statusText }));
        throw new Error((errBody as { error?: string }).error ?? 'Delete failed');
      }
    },
    [prefix]
  );

  const postDiagnose = useCallback(
    async <T>(service: string): Promise<T> => {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 150000);

      try {
        const res = await fetch(`${prefix}/services/${encodeURIComponent(service)}/diagnose`, {
          method: 'POST',
          signal: controller.signal,
        });

        if (!res.ok) {
          const errBody = await res.json().catch(() => ({ error: res.statusText }));
          throw new Error((errBody as { error?: string }).error ?? 'Diagnosis failed');
        }

        return res.json() as Promise<T>;
      } finally {
        clearTimeout(timeout);
      }
    },
    [prefix]
  );

  return { fetchJSON, postAction, putJSON, postJSON, deleteAction, postDiagnose };
}
