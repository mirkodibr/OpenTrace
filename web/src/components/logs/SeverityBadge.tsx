import type { LogLevel } from '@/types/api';
import styles from './SeverityBadge.module.css';

interface Props {
  level: LogLevel;
}

export function SeverityBadge({ level }: Props) {
  return (
    <span className={`${styles.badge} ${styles[level]}`}>
      {level.toUpperCase()}
    </span>
  );
}
