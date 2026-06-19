import type { LogLevel } from '@/types/api';
import styles from './SeveritySelect.module.css';

const LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error', 'fatal'];

interface Props {
  value: LogLevel | '';
  onChange: (value: LogLevel | '') => void;
}

export function SeveritySelect({ value, onChange }: Props) {
  return (
    <label className={styles.label}>
      <span className={styles.labelText}>Level</span>
      <select
        className={styles.select}
        value={value}
        onChange={(e) => onChange(e.target.value as LogLevel | '')}
      >
        <option value="">All levels</option>
        {LEVELS.map((l) => (
          <option key={l} value={l}>
            {l.charAt(0).toUpperCase() + l.slice(1)}
          </option>
        ))}
      </select>
    </label>
  );
}
