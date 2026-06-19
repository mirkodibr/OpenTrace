import { useFilterStore } from '@/store/filterStore';
import { SeveritySelect } from './SeveritySelect';
import { TimeRangePicker } from './TimeRangePicker';
import styles from './FilterBar.module.css';

export function FilterBar() {
  const {
    timeRange, setTimeRange,
    serviceName, setServiceName,
    level, setLevel,
    keyword, setKeyword,
    resetFilters,
  } = useFilterStore();

  return (
    <div className={styles.bar}>
      <TimeRangePicker value={timeRange} onChange={setTimeRange} />

      <label className={styles.field}>
        <span className={styles.fieldLabel}>Service</span>
        <input
          className={styles.input}
          type="text"
          placeholder="e.g. payment-service"
          value={serviceName}
          onChange={(e) => setServiceName(e.target.value)}
        />
      </label>

      <SeveritySelect value={level} onChange={setLevel} />

      <label className={styles.field}>
        <span className={styles.fieldLabel}>Keyword</span>
        <input
          className={styles.input}
          type="text"
          placeholder="Search body…"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
      </label>

      <button className={styles.reset} onClick={resetFilters} title="Reset all filters">
        Reset
      </button>
    </div>
  );
}
