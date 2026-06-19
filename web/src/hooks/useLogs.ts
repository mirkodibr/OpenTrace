import { useInfiniteQuery } from '@tanstack/react-query';
import { fetchLogs } from '@/api/client';
import { logsKeys } from '@/api/queryKeys';
import type { LogEvent, LogLevel } from '@/types/api';

interface UseLogsParams {
  startTime: Date;
  endTime: Date;
  serviceName?: string;
  level?: LogLevel;
  keyword?: string;
  limit?: number;
  enabled?: boolean;
}

export function useLogs({
  startTime,
  endTime,
  serviceName,
  level,
  keyword,
  limit = 100,
  enabled = true,
}: UseLogsParams) {
  const baseParams = {
    start_time:   startTime.toISOString(),
    end_time:     endTime.toISOString(),
    service_name: serviceName,
    level,
    keyword,
    limit,
  };

  return useInfiniteQuery({
    queryKey: logsKeys.list(baseParams),
    queryFn: ({ pageParam }) =>
      fetchLogs({ ...baseParams, cursor: pageParam as string | undefined }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    enabled,
    staleTime: 30_000,
  });
}

// Flattens all pages into a single sorted event array.
export function flattenLogPages(
  data: ReturnType<typeof useLogs>['data'],
): LogEvent[] {
  if (!data) return [];
  return data.pages.flatMap((page) => page.data);
}
