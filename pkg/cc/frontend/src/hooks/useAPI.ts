import { useCallback } from 'react';

export class APIError extends Error {
  status: number;
  body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.body = body;
  }
}

export function useAPI(stack: string) {
  const prefix = `/api/stacks/${stack}`;

  const requestJSON = useCallback(
    async <T>(url: string, init?: RequestInit): Promise<T> => {
      const res = await fetch(`${prefix}${url}`, init);

      if (!res.ok) {
        const bodyText = await res.text();
        let body: unknown = bodyText;
        let message = `${res.status}: ${bodyText}`;

        if (bodyText) {
          try {
            body = JSON.parse(bodyText) as unknown;
            if (typeof body === 'object' && body !== null && 'error' in body) {
              const err = (body as { error?: unknown }).error;
              if (typeof err === 'string' && err.trim() !== '') {
                message = err;
              }
            }
          } catch {
            // ignore parse errors and keep text body
          }
        }

        throw new APIError(message, res.status, body);
      }

      if (res.status === 204) {
        return undefined as T;
      }

      return res.json() as Promise<T>;
    },
    [prefix]
  );

  const fetchJSON = useCallback(
    async <T>(url: string): Promise<T> => {
      return requestJSON<T>(url);
    },
    [requestJSON]
  );

  const postAction = useCallback(
    async (service: string, action: string): Promise<void> => {
      await requestJSON<{ status: string }>(`/services/${service}/${action}`, { method: 'POST' });
    },
    [requestJSON]
  );

  const putJSON = useCallback(
    async <T>(url: string, body: unknown): Promise<T> => {
      return requestJSON<T>(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
    },
    [requestJSON]
  );

  const postJSON = useCallback(
    async <T>(url: string, body?: unknown): Promise<T> => {
      const init: RequestInit = { method: 'POST' };
      if (body !== undefined) {
        init.headers = { 'Content-Type': 'application/json' };
        init.body = JSON.stringify(body);
      }

      return requestJSON<T>(url, init);
    },
    [requestJSON]
  );

  const deleteAction = useCallback(
    async (url: string): Promise<void> => {
      await requestJSON<unknown>(url, { method: 'DELETE' });
    },
    [requestJSON]
  );

  const deleteJSON = useCallback(
    async <T>(url: string): Promise<T> => {
      return requestJSON<T>(url, { method: 'DELETE' });
    },
    [requestJSON]
  );

  return { fetchJSON, postAction, putJSON, postJSON, deleteAction, deleteJSON, requestJSON };
}
