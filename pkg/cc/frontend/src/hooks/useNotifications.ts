import { useCallback, useEffect, useRef, useState } from 'react';

const STORAGE_KEY = 'xcli:notifications';

type Permission = NotificationPermission | 'unsupported';

/**
 * Thin wrapper around the browser Notification API.
 *
 * Persists an enabled/disabled preference in localStorage.
 * Requests browser permission only when the user enables notifications.
 * Returns a `notify` function that fires a native notification when
 * both the preference and browser permission allow it.
 */
export function useNotifications() {
  const permission = useRef<Permission>('Notification' in window ? Notification.permission : 'unsupported');
  const [enabled, setEnabled] = useState(() => {
    const stored = localStorage.getItem(STORAGE_KEY);

    // No stored preference yet — default to on if the browser already granted permission
    if (stored === null) return permission.current === 'granted';

    return stored === 'true';
  });

  // When toggled on, request permission if we haven't yet
  useEffect(() => {
    if (!enabled || permission.current !== 'default') return;

    Notification.requestPermission().then(result => {
      permission.current = result;

      // If the user denied, turn the toggle back off
      if (result === 'denied') {
        setEnabled(false);
        localStorage.setItem(STORAGE_KEY, 'false');
      }
    });
  }, [enabled]);

  const toggle = useCallback(() => {
    setEnabled(prev => {
      const next = !prev;
      localStorage.setItem(STORAGE_KEY, String(next));
      return next;
    });
  }, []);

  const notify = useCallback(
    (title: string, options?: NotificationOptions) => {
      if (!enabled || permission.current !== 'granted') return;

      try {
        new Notification(title, { icon: '/icon.png', ...options });
      } catch {
        // Safari / iOS may throw in certain contexts – ignore
      }
    },
    [enabled]
  );

  return { notify, enabled, toggle };
}
