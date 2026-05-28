'use client';

import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
import {useEffect, useState} from 'react';
import {api, APIError} from '@/lib/api';
import {ThemeSwitcher} from '@/components/ThemeSwitcher';

interface MeResponse {
  id: string;
  email: string;
  role: string;
  display_name: string;
  impersonator_email?: string;
}

export default function AdminLayout({children}: {children: React.ReactNode}) {
  const router = useRouter();
  const pathname = usePathname();
  const [me, setMe] = useState<MeResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const u = await api<MeResponse>('/api/auth/me');
        if (alive) setMe(u);
      } catch (err) {
        if (err instanceof APIError && err.status === 401) {
          // Strip the locale prefix if present and route to login.
          const localePrefix = pathname.split('/')[1] ?? 'de';
          router.replace(`/${localePrefix}/login`);
          return;
        }
        if (alive) setMe(null);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [pathname, router]);

  async function handleLogout() {
    try {
      await api('/api/auth/logout', {method: 'POST'});
    } catch {
      // best-effort
    }
    const localePrefix = pathname.split('/')[1] ?? 'de';
    router.replace(`/${localePrefix}/login`);
  }

  async function handleStopImpersonating() {
    try {
      await api('/api/auth/stop-impersonating', {method: 'POST'});
      window.location.href = `/${pathname.split('/')[1] ?? 'de'}/admin/events`;
    } catch (err) {
      alert(err instanceof APIError ? err.developerMessage : String(err));
    }
  }

  if (loading) {
    return (
      <main className="flex-1 flex items-center justify-center">
        <p className="text-sm text-neutral-500">Lädt…</p>
      </main>
    );
  }
  if (!me) {
    return null; // redirect already issued
  }

  const localePrefix = pathname.split('/')[1] ?? 'de';

  return (
    <div className="flex-1 flex flex-col">
      {me.impersonator_email && (
        <div className="bg-amber-100 dark:bg-amber-950/40 border-b-2 border-amber-400 dark:border-amber-700 text-amber-900 dark:text-amber-100 px-6 py-2 text-sm">
          <div className="max-w-5xl mx-auto flex items-center justify-between gap-4">
            <span>
              Du arbeitest gerade als <span className="font-medium">{me.email}</span>{' '}
              <span className="opacity-70">(angemeldet als {me.impersonator_email})</span>
            </span>
            <button
              onClick={handleStopImpersonating}
              className="text-xs rounded border border-amber-400 dark:border-amber-700 px-2 py-1 hover:bg-amber-200 dark:hover:bg-amber-900/40"
            >
              ← Zurück zu mir
            </button>
          </div>
        </div>
      )}
      <header className="border-b border-neutral-200 dark:border-neutral-800">
        <div className="max-w-5xl mx-auto px-6 py-3 flex items-center gap-6">
          <Link href={`/${localePrefix}/admin/events`} className="font-semibold">
            kreise.berlin admin
          </Link>
          <nav className="flex-1 flex gap-4 text-sm">
            <Link href={`/${localePrefix}/admin/events`} className="hover:underline">
              Events
            </Link>
            {me.role === 'global_admin' && (
              <Link href={`/${localePrefix}/admin/users`} className="hover:underline">
                Admins
              </Link>
            )}
            <Link href={`/${localePrefix}/admin/account`} className="hover:underline">
              Mein Account
            </Link>
          </nav>
          <span className="text-sm text-neutral-500">{me.email}</span>
          <ThemeSwitcher />
          <button
            onClick={handleLogout}
            className="text-sm text-neutral-700 dark:text-neutral-300 hover:underline"
          >
            Abmelden
          </button>
        </div>
      </header>
      <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-6">{children}</main>
    </div>
  );
}
