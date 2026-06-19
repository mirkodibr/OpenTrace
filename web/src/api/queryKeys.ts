import type { LogsQueryParams } from '@/types/api';

// Centralised query key factory.
// Keys are hierarchical so invalidation at a broader level cascades:
//   queryClient.invalidateQueries({ queryKey: logsKeys.all })
//   → invalidates every logs query
export const logsKeys = {
  all: ['logs'] as const,
  lists: () => [...logsKeys.all, 'list'] as const,
  list: (params: Omit<LogsQueryParams, 'cursor'>) =>
    [...logsKeys.lists(), params] as const,
};
