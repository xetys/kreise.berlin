'use client';

import {createContext, useCallback, useContext, useEffect, useState} from 'react';

export type ThemePref = 'light' | 'dark' | 'system';

interface ThemeCtx {
  pref: ThemePref;
  resolved: 'light' | 'dark';
  setPref: (p: ThemePref) => void;
}

const Ctx = createContext<ThemeCtx | null>(null);

const STORAGE_KEY = 'theme';

function readPref(): ThemePref {
  if (typeof window === 'undefined') return 'system';
  const raw = window.localStorage.getItem(STORAGE_KEY);
  return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'system';
}

function systemPrefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function resolve(p: ThemePref): 'light' | 'dark' {
  return p === 'system' ? (systemPrefersDark() ? 'dark' : 'light') : p;
}

function applyToDocument(p: ThemePref) {
  if (typeof document === 'undefined') return;
  const r = resolve(p);
  document.documentElement.classList.toggle('dark', r === 'dark');
  document.documentElement.dataset.themePref = p;
}

/**
 * Provides the active theme + setter. The pre-paint script in app/layout.tsx
 * has already toggled the `dark` class on <html> before we hydrate; this
 * provider just reconciles state and reacts to runtime changes (user clicks
 * the switcher, or system theme flips while pref === 'system').
 */
export function ThemeProvider({children}: {children: React.ReactNode}) {
  const [pref, setPrefState] = useState<ThemePref>('system');
  const [resolved, setResolved] = useState<'light' | 'dark'>('light');

  useEffect(() => {
    const initial = readPref();
    setPrefState(initial);
    setResolved(resolve(initial));
  }, []);

  useEffect(() => {
    if (pref !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => {
      const r = mq.matches ? 'dark' : 'light';
      setResolved(r);
      document.documentElement.classList.toggle('dark', r === 'dark');
    };
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }, [pref]);

  const setPref = useCallback((p: ThemePref) => {
    setPrefState(p);
    setResolved(resolve(p));
    try {
      window.localStorage.setItem(STORAGE_KEY, p);
    } catch {}
    applyToDocument(p);
  }, []);

  return <Ctx.Provider value={{pref, resolved, setPref}}>{children}</Ctx.Provider>;
}

export function useTheme(): ThemeCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error('useTheme must be used inside <ThemeProvider>');
  return v;
}
