import { useRef, useState, useCallback } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { LogEvent } from '@/types/api';
import { SeverityBadge } from './SeverityBadge';
import { LogRowDetail } from './LogRowDetail';
import styles from './LogTable.module.css';

interface Props {
  events: LogEvent[];
  isFetchingNextPage: boolean;
  hasNextPage: boolean;
  onLoadMore: () => void;
}

// Each row has a summary line (36px) plus optionally an expanded detail panel.
// The virtualizer uses a dynamic measurement approach: each item reports its
// actual rendered height so expansion panels don't corrupt scroll position.
const ROW_ESTIMATE_PX = 36;

export function LogTable({ events, isFetchingNextPage, hasNextPage, onLoadMore }: Props) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const parentRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_ESTIMATE_PX,
    overscan: 10,
    measureElement: (el) => el.getBoundingClientRect().height,
  });

  const virtualItems = rowVirtualizer.getVirtualItems();

  const handleRowClick = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  }, []);

  // Sentinel-based infinite scroll: when the last virtual item is within the
  // last 5 rows, trigger loading the next page.
  const lastVirtual = virtualItems[virtualItems.length - 1];
  const nearEnd = lastVirtual && lastVirtual.index >= events.length - 5;
  if (nearEnd && hasNextPage && !isFetchingNextPage) {
    onLoadMore();
  }

  if (events.length === 0) {
    return (
      <div className={styles.empty}>
        No log events match the current filters.
      </div>
    );
  }

  return (
    <div className={styles.wrapper}>
      {/* Fixed header */}
      <div className={styles.header} role="rowgroup">
        <div className={styles.headerRow} role="row">
          <span className={`${styles.col} ${styles.colTs}`}    role="columnheader">Timestamp</span>
          <span className={`${styles.col} ${styles.colSvc}`}   role="columnheader">Service</span>
          <span className={`${styles.col} ${styles.colSev}`}   role="columnheader">Level</span>
          <span className={`${styles.col} ${styles.colBody}`}  role="columnheader">Message</span>
        </div>
      </div>

      {/* Virtualised rows */}
      <div ref={parentRef} className={styles.scrollArea} role="rowgroup">
        <div
          style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}
        >
          {virtualItems.map((vItem) => {
            const event = events[vItem.index];
            const expanded = expandedId === event.id;

            return (
              <div
                key={event.id}
                data-index={vItem.index}
                ref={rowVirtualizer.measureElement}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  transform: `translateY(${vItem.start}px)`,
                }}
              >
                <button
                  className={`${styles.row} ${expanded ? styles.rowExpanded : ''}`}
                  onClick={() => handleRowClick(event.id)}
                  aria-expanded={expanded}
                  role="row"
                >
                  <span className={`${styles.col} ${styles.colTs}`}>
                    {formatTimestamp(event.timestamp)}
                  </span>
                  <span className={`${styles.col} ${styles.colSvc}`}>
                    {event.service_name}
                  </span>
                  <span className={`${styles.col} ${styles.colSev}`}>
                    <SeverityBadge level={event.severity} />
                  </span>
                  <span className={`${styles.col} ${styles.colBody}`}>
                    {event.body}
                  </span>
                </button>

                {expanded && <LogRowDetail event={event} />}
              </div>
            );
          })}
        </div>
      </div>

      {isFetchingNextPage && (
        <div className={styles.loadingMore}>Loading more…</div>
      )}
      {!hasNextPage && events.length > 0 && (
        <div className={styles.endOfResults}>End of results</div>
      )}
    </div>
  );
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ` +
    `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}` +
    `.${String(d.getUTCMilliseconds()).padStart(3, '0')}`
  );
}
