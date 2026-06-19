import { create } from 'zustand';
import type { LogLevel } from '@/types/api';

export interface TimeRange {
  start: Date;
  end: Date;
}

function lastHour(): TimeRange {
  const end   = new Date();
  const start = new Date(end.getTime() - 60 * 60 * 1000);
  return { start, end };
}

interface FilterState {
  timeRange:   TimeRange;
  serviceName: string;
  level:       LogLevel | '';
  keyword:     string;

  setTimeRange:   (range: TimeRange) => void;
  setServiceName: (name: string) => void;
  setLevel:       (level: LogLevel | '') => void;
  setKeyword:     (keyword: string) => void;
  resetFilters:   () => void;
}

const initialState = {
  timeRange:   lastHour(),
  serviceName: '',
  level:       '' as LogLevel | '',
  keyword:     '',
};

export const useFilterStore = create<FilterState>((set) => ({
  ...initialState,

  setTimeRange:   (range)   => set({ timeRange: range }),
  setServiceName: (name)    => set({ serviceName: name }),
  setLevel:       (level)   => set({ level }),
  setKeyword:     (keyword) => set({ keyword }),
  resetFilters:   ()        => set(initialState),
}));
