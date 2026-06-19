import { useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useLogs, flattenLogPages } from '@/hooks/useLogs';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { useFilterStore } from '@/store/filterStore';
import { logsKeys } from '@/api/queryKeys';
import { LogTable } from '@/components/logs/LogTable';
import { FilterBar } from '@/components/filters/FilterBar';
import { AutoRefreshToggle } from '@/components/filters/AutoRefreshToggle';
import { ApiErrorBoundary } from '@/components/errors/ApiErrorBoundary';
import type { LogLevel } from '@/types/api';
import styles from './LogsPage.module.css';

function LogsContent() {
  const queryClient = useQueryClient();
  const {
    timeRange, serviceName, level, keyword, refreshIntervalSec, setRefreshIntervalSec,
  } = useFilterStore();

  const [lastRefreshed, setLastRefreshed] = useState<Date | null>(null);

  const baseParams = {
    start_time:   timeRange.start.toISOString(),
    end_time:     timeRange.end.toISOString(),
    service_name: serviceName || undefined,
    level:        (level || undefined) as LogLevel | undefined,
    keyword:      keyword || undefined,
    limit:        100,
  };

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

  const doRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: logsKeys.list(baseParams) });
    setLastRefreshed(new Date());
  }, [queryClient, baseParams]);

  useAutoRefresh({
    intervalMs: refreshIntervalSec * 1000,
    onRefresh:  doRefresh,
    enabled:    refreshIntervalSec > 0,
  });

  if (isLoading) {
    return <div className={styles.state}>Loading…</div>;
  }

  if (isError) {
    throw error;
  }

  const events = flattenLogPages(data);

  return (
    <>
      <div className={styles.toolbar}>
        <AutoRefreshToggle
          intervalSeconds={refreshIntervalSec}
          onChange={setRefreshIntervalSec}
          lastRefreshed={lastRefreshed}
        />
      </div>
      <div className={styles.tableArea}>
        <LogTable
          events={events}
          hasNextPage={hasNextPage}
          isFetchingNextPage={isFetchingNextPage}
          onLoadMore={fetchNextPage}
        />
      </div>
    </>
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
