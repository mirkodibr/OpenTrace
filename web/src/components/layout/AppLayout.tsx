import { Outlet } from '@tanstack/react-router';
import { TopBar } from './TopBar';
import styles from './AppLayout.module.css';

export function AppLayout() {
  return (
    <div className={styles.shell}>
      <TopBar />
      <main className={styles.content}>
        <Outlet />
      </main>
    </div>
  );
}
