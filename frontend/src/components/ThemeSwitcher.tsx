'use client';

import {useTranslations} from 'next-intl';
import {useEffect, useState} from 'react';
import {ThemePref, useTheme} from './ThemeProvider';

const ORDER: ThemePref[] = ['system', 'light', 'dark'];

/**
 * Three-state cycle: system → light → dark → system. Single button so it fits
 * the minimalist top nav next to the locale switcher. Icon reflects the
 * current preference (not the resolved theme), so users always see which mode
 * they explicitly chose.
 *
 * Hidden until mounted to avoid a hydration mismatch: the server can't know
 * the pref, so we'd render the wrong icon for a frame. The pre-paint script
 * has already applied the right dark/light class, so the page itself is
 * styled correctly — only this control needs to wait for client state.
 */
export function ThemeSwitcher() {
  const {pref, setPref} = useTheme();
  const t = useTranslations('theme');
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  if (!mounted) {
    return <span className="inline-block w-[18px] h-[18px]" aria-hidden />;
  }

  const next = ORDER[(ORDER.indexOf(pref) + 1) % ORDER.length];
  const label = t(`label.${pref}`);
  const title = t('cycle', {next: t(`label.${next}`)});

  return (
    <button
      type="button"
      onClick={() => setPref(next)}
      aria-label={label}
      title={title}
      className="inline-flex items-center justify-center w-7 h-7 rounded opacity-60 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-current"
    >
      {pref === 'system' && <SystemIcon />}
      {pref === 'light' && <SunIcon />}
      {pref === 'dark' && <MoonIcon />}
    </button>
  );
}

function SunIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

function SystemIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="2" y="4" width="20" height="14" rx="2" />
      <path d="M8 22h8M12 18v4" />
    </svg>
  );
}
