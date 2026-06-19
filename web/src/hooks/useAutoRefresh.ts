import { useEffect, useRef, useCallback, useState } from 'react';
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
// Uses React state (not a ref) so that pause/resume cause a re-render which
// updates the dependency array in useAutoRefresh's effect — ensuring the
// interval is correctly started/stopped rather than silently ignoring the change.
export function usePolling(intervalMs: number, onRefresh: () => void) {
  const [paused, setPaused] = useState(false);

  const pause  = useCallback(() => setPaused(true),  []);
  const resume = useCallback(() => setPaused(false), []);

  useAutoRefresh({ intervalMs, onRefresh, enabled: !paused });

  return { pause, resume };
}
