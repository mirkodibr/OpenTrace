import { useEffect, useState } from 'react';

// Returns true when the browser tab is visible.
// Used to pause polling when the user switches tabs.
export function usePageVisibility(): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState === 'visible');

  useEffect(() => {
    const handler = () => setVisible(document.visibilityState === 'visible');
    document.addEventListener('visibilitychange', handler);
    return () => document.removeEventListener('visibilitychange', handler);
  }, []);

  return visible;
}
