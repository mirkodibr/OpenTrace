import { useLogs, flattenLogPages } from '@/hooks/useLogs';
import { useFilterStore } from '@/store/filterStore';
import { LogTable } from '@/components/logs/LogTable';
import { FilterBar } from '@/components/filters/FilterBar';
import { ApiErrorBoundary } from '@/components/errors/ApiErrorBoundary';
import type { LogLevel } from '@/types/api';
import styles from './LogsPage.module.css';

function LogsContent() {
  const { timeRange, serviceName, level, keyword } = useFilterStore();

  const {
    data,
    isLoading,
    isError,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useLogs({
    startTime:   timeRange.start,
    endTime:     timeRange.end,
    serviceName: serviceName || undefined,
    level:       (level || undefined) as LogLevel | undefined,
    keyword:     keyword || undefined,
    limit:       100,
  });

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
      <FilterBar />
      <ApiErrorBoundary>
        <LogsContent />
      </ApiErrorBoundary>
    </div>
  );
}
