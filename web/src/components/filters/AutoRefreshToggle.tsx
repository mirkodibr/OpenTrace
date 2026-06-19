import styles from './AutoRefreshToggle.module.css';

const INTERVALS = [
  { label: 'Off',  value: 0    },
  { label: '10s',  value: 10   },
  { label: '30s',  value: 30   },
  { label: '1m',   value: 60   },
  { label: '5m',   value: 300  },
];

interface Props {
  intervalSeconds: number;
  onChange: (seconds: number) => void;
  lastRefreshed: Date | null;
}

export function AutoRefreshToggle({ intervalSeconds, onChange, lastRefreshed }: Props) {
  return (
    <div className={styles.root}>
      <span className={styles.label}>Auto-refresh</span>
      <div className={styles.buttons}>
        {INTERVALS.map((opt) => (
          <button
            key={opt.value}
            className={`${styles.btn} ${intervalSeconds === opt.value ? styles.btnActive : ''}`}
            onClick={() => onChange(opt.value)}
          >
            {opt.label}
          </button>
        ))}
      </div>
      {lastRefreshed && intervalSeconds > 0 && (
        <span className={styles.lastRefreshed}>
          Updated {formatAgo(lastRefreshed)}
        </span>
      )}
    </div>
  );
}

function formatAgo(d: Date): string {
  const secs = Math.floor((Date.now() - d.getTime()) / 1000);
  if (secs < 5)  return 'just now';
  if (secs < 60) return `${secs}s ago`;
  return `${Math.floor(secs / 60)}m ago`;
}
