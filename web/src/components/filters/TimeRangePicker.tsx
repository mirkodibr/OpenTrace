import { useId } from 'react';
import type { TimeRange } from '@/store/filterStore';
import styles from './TimeRangePicker.module.css';

const PRESETS: { label: string; minutes: number }[] = [
  { label: '15m', minutes: 15   },
  { label: '1h',  minutes: 60   },
  { label: '3h',  minutes: 180  },
  { label: '6h',  minutes: 360  },
  { label: '24h', minutes: 1440 },
];

interface Props {
  value: TimeRange;
  onChange: (range: TimeRange) => void;
}

function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

export function TimeRangePicker({ value, onChange }: Props) {
  const startId = useId();
  const endId   = useId();

  const applyPreset = (minutes: number) => {
    const end   = new Date();
    const start = new Date(end.getTime() - minutes * 60 * 1000);
    onChange({ start, end });
  };

  const activePreset = PRESETS.find((p) => {
    const expected = new Date(value.end.getTime() - p.minutes * 60 * 1000);
    return Math.abs(expected.getTime() - value.start.getTime()) < 60_000;
  });

  return (
    <div className={styles.root}>
      <div className={styles.presets}>
        {PRESETS.map((p) => (
          <button
            key={p.label}
            className={`${styles.preset} ${activePreset?.label === p.label ? styles.presetActive : ''}`}
            onClick={() => applyPreset(p.minutes)}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className={styles.inputs}>
        <label className={styles.inputLabel} htmlFor={startId}>
          <span>From</span>
          <input
            id={startId}
            className={styles.input}
            type="datetime-local"
            value={toLocalInput(value.start)}
            onChange={(e) => {
              const d = new Date(e.target.value);
              if (!isNaN(d.getTime())) onChange({ ...value, start: d });
            }}
          />
        </label>
        <label className={styles.inputLabel} htmlFor={endId}>
          <span>To</span>
          <input
            id={endId}
            className={styles.input}
            type="datetime-local"
            value={toLocalInput(value.end)}
            onChange={(e) => {
              const d = new Date(e.target.value);
              if (!isNaN(d.getTime())) onChange({ ...value, end: d });
            }}
          />
        </label>
      </div>
    </div>
  );
}
