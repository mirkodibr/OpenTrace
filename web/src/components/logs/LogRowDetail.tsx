import type { LogEvent } from '@/types/api';
import styles from './LogRowDetail.module.css';

interface Props {
  event: LogEvent;
}

export function LogRowDetail({ event }: Props) {
  return (
    <div className={styles.panel}>
      <dl className={styles.grid}>
        <dt>Timestamp</dt>
        <dd>{new Date(event.timestamp).toISOString()}</dd>

        <dt>Service</dt>
        <dd>{event.service_name}</dd>

        {event.trace_id && (
          <>
            <dt>Trace ID</dt>
            <dd className={styles.mono}>{event.trace_id}</dd>
          </>
        )}
        {event.span_id && (
          <>
            <dt>Span ID</dt>
            <dd className={styles.mono}>{event.span_id}</dd>
          </>
        )}

        <dt>Message</dt>
        <dd className={styles.body}>{event.body}</dd>
      </dl>

      {event.log_attributes && Object.keys(event.log_attributes).length > 0 && (
        <section className={styles.section}>
          <h4 className={styles.sectionTitle}>Log Attributes</h4>
          <pre className={styles.json}>
            {JSON.stringify(event.log_attributes, null, 2)}
          </pre>
        </section>
      )}

      {event.resource_attributes && Object.keys(event.resource_attributes).length > 0 && (
        <section className={styles.section}>
          <h4 className={styles.sectionTitle}>Resource Attributes</h4>
          <pre className={styles.json}>
            {JSON.stringify(event.resource_attributes, null, 2)}
          </pre>
        </section>
      )}
    </div>
  );
}
