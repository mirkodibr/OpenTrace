import { useLogs, flattenLogPages } from '@/hooks/useLogs';
import { LogTable } from '@/components/logs/LogTable';
import { ApiErrorBoundary } from '@/components/errors/ApiErrorBoundary';
import styles from './LogsPage.module.css';

// Default time window: last 1 hour
function defaultWindow() {
  const end   = new Date();
  const start = new Date(end.getTime() - 60 * 60 * 1000);
  return { start, end };
}

function LogsContent() {
  const { start, end } = defaultWindow();

  const {
    data,
    isLoading,
    isError,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useLogs({ startTime: start, endTime: end, limit: 100 });

  if (isLoading) {
    return <div className={styles.state}>Loading…</div>;
  }

  if (isError) {
    throw error;
  }

  const events = flattenLogPages(data);

  return (
    <div className={styles.tableArea}>
      <LogTable
        events={events}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        onLoadMore={fetchNextPage}
      />
    </div>
  );
}

export function LogsPage() {
  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>Logs</h1>
      </div>
      <ApiErrorBoundary>
        <LogsContent />
      </ApiErrorBoundary>
    </div>
  );
}
