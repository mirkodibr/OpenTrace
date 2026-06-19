import styles from './LogsPage.module.css';

export function LogsPage() {
  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>Logs</h1>
      </div>
      <div className={styles.body}>
        <p className={styles.placeholder}>
          Log explorer — coming in Day 16
        </p>
      </div>
    </div>
  );
}
