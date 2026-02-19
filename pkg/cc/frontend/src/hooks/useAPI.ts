import { useCallback } from 'react';

export function useAPI() {
  const fetchJSON = useCallback(async <T>(url: string): Promise<T> => {
    const res = await fetch(url);

    if (!res.ok) {
      const body = await res.text();
      throw new Error(`${res.status}: ${body}`);
    }

    return res.json() as Promise<T>;
  }, []);

  const postAction = useCallback(async (service: string, action: string): Promise<void> => {
    const res = await fetch(`/api/services/${service}/${action}`, {
      method: 'POST',
    });

    if (!res.ok) {
      const body = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error((body as { error?: string }).error ?? 'Action failed');
    }
  }, []);

  const putJSON = useCallback(async <T>(url: string, body: unknown): Promise<T> => {
    const res = await fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      const errBody = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error((errBody as { error?: string }).error ?? 'Request failed');
    }

    return res.json() as Promise<T>;
  }, []);

  const postJSON = useCallback(async <T>(url: string): Promise<T> => {
    const res = await fetch(url, { method: 'POST' });

    if (!res.ok) {
      const errBody = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error((errBody as { error?: string }).error ?? 'Request failed');
    }

    return res.json() as Promise<T>;
  }, []);

  const deleteAction = useCallback(async (url: string): Promise<void> => {
    const res = await fetch(url, { method: 'DELETE' });

    if (!res.ok) {
      const errBody = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error((errBody as { error?: string }).error ?? 'Delete failed');
    }
  }, []);

  return { fetchJSON, postAction, putJSON, postJSON, deleteAction };
}
