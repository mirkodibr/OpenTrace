import { useEffect, useRef, useCallback } from 'react';
import { usePageVisibility } from './usePageVisibility';

interface UseAutoRefreshOptions {
  // Interval in milliseconds between refreshes.
  intervalMs: number;
  // Callback to execute on each tick.
  onRefresh: () => void;
  // When false the timer is suspended (e.g. user paused it).
  enabled: boolean;
}

// Fires onRefresh every intervalMs while enabled AND the page is visible.
// The interval is reset any time enabled, intervalMs, or visibility changes so
// the first tick after re-enabling is always a full interval away — avoids an
// immediate double-fetch when the user switches back to the tab.
export function useAutoRefresh({ intervalMs, onRefresh, enabled }: UseAutoRefreshOptions) {
  const visible = usePageVisibility();
  const callbackRef = useRef(onRefresh);

  // Keep the callback reference fresh without restarting the interval.
  useEffect(() => {
    callbackRef.current = onRefresh;
  });

  useEffect(() => {
    if (!enabled || !visible) return;

    const id = setInterval(() => callbackRef.current(), intervalMs);
    return () => clearInterval(id);
  }, [enabled, visible, intervalMs]);
}

// Hook that wraps useAutoRefresh + manages the enabled/paused toggle state.
export function usePolling(intervalMs: number, onRefresh: () => void) {
  const enabled = useRef(true);

  const pause  = useCallback(() => { enabled.current = false; }, []);
  const resume = useCallback(() => { enabled.current = true;  }, []);

  useAutoRefresh({ intervalMs, onRefresh, enabled: enabled.current });

  return { pause, resume };
}
