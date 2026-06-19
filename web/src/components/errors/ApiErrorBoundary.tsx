import { Component, type ReactNode } from 'react';
import { ApiError } from '@/api/client';
import styles from './ApiErrorBoundary.module.css';

interface Props {
  children: ReactNode;
  fallback?: (error: Error, reset: () => void) => ReactNode;
}

interface State {
  error: Error | null;
}

export class ApiErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  reset = () => this.setState({ error: null });

  render() {
    const { error } = this.state;
    if (error) {
      if (this.props.fallback) {
        return this.props.fallback(error, this.reset);
      }
      return <DefaultErrorFallback error={error} onReset={this.reset} />;
    }
    return this.props.children;
  }
}

function DefaultErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
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
