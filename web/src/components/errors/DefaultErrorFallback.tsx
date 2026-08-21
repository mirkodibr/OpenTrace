import { ApiError } from '@/api/client';
import styles from './ApiErrorBoundary.module.css';

export function DefaultErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
  const isApiError = error instanceof ApiError;

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <h2 className={styles.title}>
          {isApiError ? `Error ${(error as ApiError).status}` : 'Unexpected Error'}
        </h2>
        <p className={styles.detail}>{error.message}</p>
        <button className={styles.button} onClick={onReset}>
          Try again
        </button>
      </div>
    </div>
  );
}
