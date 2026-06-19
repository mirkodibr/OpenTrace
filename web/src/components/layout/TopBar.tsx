import { Link } from '@tanstack/react-router';
import styles from './TopBar.module.css';

export function TopBar() {
  return (
    <header className={styles.bar}>
      <Link to="/" className={styles.logo}>
        <svg width="22" height="22" viewBox="0 0 22 22" fill="none" aria-hidden>
          <circle cx="11" cy="11" r="10" stroke="currentColor" strokeWidth="2" />
          <path d="M6 11 L10 7 L14 11 L10 15 Z" fill="currentColor" />
        </svg>
        <span>OpenTrace</span>
      </Link>
      <nav className={styles.nav}>
        <Link to="/logs" className={styles.navLink} activeProps={{ className: styles.navLinkActive }}>
          Logs
        </Link>
      </nav>
    </header>
  );
}
