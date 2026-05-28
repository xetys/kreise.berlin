'use client';

import {useEffect, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import {api, APIError} from '@/lib/api';

interface MeResponse {
  id: string;
  email: string;
  role: string;
  display_name: string;
}

export default function AdminAccountPage() {
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';

  const [me, setMe] = useState<MeResponse | null>(null);

  // Profile section state
  const [displayName, setDisplayName] = useState('');
  const [profileSubmitting, setProfileSubmitting] = useState(false);
  const [profileMsg, setProfileMsg] = useState<string | null>(null);

  // Password section state
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [pwSubmitting, setPwSubmitting] = useState(false);
  const [pwMsg, setPwMsg] = useState<string | null>(null);

  useEffect(() => {
    api<MeResponse>('/api/auth/me')
      .then((r) => {
        setMe(r);
        setDisplayName(r.display_name ?? '');
      })
      .catch(() => {
        router.replace(`/${localePrefix}/login`);
      });
  }, [router, localePrefix]);

  async function saveProfile(e: React.FormEvent) {
    e.preventDefault();
    setProfileMsg(null);
    setProfileSubmitting(true);
    try {
      const r = await api<{display_name: string}>('/api/admin/account/profile', {
        method: 'PATCH',
        body: {display_name: displayName},
      });
      setProfileMsg('Profil gespeichert.');
      if (me) setMe({...me, display_name: r.display_name});
    } catch (err) {
      setProfileMsg(err instanceof APIError ? `${err.code}: ${err.developerMessage}` : String(err));
    } finally {
      setProfileSubmitting(false);
    }
  }

  async function changePassword(e: React.FormEvent) {
    e.preventDefault();
    setPwMsg(null);
    if (next !== confirm) {
      setPwMsg('Die neuen Passwörter stimmen nicht überein.');
      return;
    }
    if (next.length < 12) {
      setPwMsg('Das neue Passwort muss mindestens 12 Zeichen lang sein.');
      return;
    }
    setPwSubmitting(true);
    try {
      await api('/api/admin/account/password', {
        method: 'POST',
        body: {current_password: current, new_password: next},
      });
      // Backend bumps password_version → all sessions (including this one)
      // become invalid. Send the user to login.
      setPwMsg('Passwort geändert. Du wirst zur Anmeldung weitergeleitet…');
      setTimeout(() => router.replace(`/${localePrefix}/login`), 1500);
    } catch (err) {
      if (err instanceof APIError && err.code === 'wrong_current_password') {
        setPwMsg('Aktuelles Passwort stimmt nicht.');
      } else if (err instanceof APIError) {
        setPwMsg(err.code + ': ' + err.developerMessage);
      } else {
        setPwMsg(String(err));
      }
    } finally {
      setPwSubmitting(false);
    }
  }

  if (!me) {
    return (
      <main className="flex-1 flex items-center justify-center">
        <p className="text-sm text-neutral-500">Lädt…</p>
      </main>
    );
  }

  return (
    <main className="flex-1 max-w-md mx-auto w-full px-6 py-10 flex flex-col gap-8">
      <header>
        <h1 className="text-2xl font-light tracking-wide">Mein Account</h1>
      </header>

      {/* Profile (display name) */}
      <section className="flex flex-col gap-4 border-b border-neutral-200 dark:border-neutral-800 pb-8">
        <header className="flex flex-col gap-1">
          <h2 className="text-sm font-medium">Profil</h2>
          <p className="text-xs opacity-70">
            E-Mail und Rolle werden vom System verwaltet — Anzeigename bestimmst du selbst.
          </p>
        </header>

        <div className="grid grid-cols-1 gap-3 text-sm">
          <ReadOnlyField label="E-Mail" value={me.email} />
          <ReadOnlyField label="Rolle" value={roleLabel(me.role)} />
        </div>

        <form onSubmit={saveProfile} className="flex flex-col gap-3">
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Anzeigename</span>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="z. B. Vorname Nachname"
              maxLength={200}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
            <span className="text-[10px] opacity-50">
              Leer lassen, um die Anzeige auf die E-Mail-Adresse zurückzusetzen.
            </span>
          </label>
          <button
            type="submit"
            disabled={profileSubmitting || displayName === (me.display_name ?? '')}
            className="rounded-md border border-neutral-300 dark:border-neutral-700 px-4 py-2 text-sm self-start disabled:opacity-50 hover:bg-neutral-100 dark:hover:bg-neutral-800"
          >
            {profileSubmitting ? 'Speichere…' : 'Profil speichern'}
          </button>
          {profileMsg && <p className="text-sm">{profileMsg}</p>}
        </form>
      </section>

      {/* Password change */}
      <section className="flex flex-col gap-4">
        <header className="flex flex-col gap-1">
          <h2 className="text-sm font-medium">Passwort ändern</h2>
          <p className="text-xs opacity-70">
            Damit werden auch alle anderen aktiven Sessions automatisch beendet.
          </p>
        </header>

        <form onSubmit={changePassword} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Aktuelles Passwort</span>
            <input
              type="password"
              required
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Neues Passwort (≥12 Zeichen)</span>
            <input
              type="password"
              required
              value={next}
              minLength={12}
              onChange={(e) => setNext(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Neues Passwort bestätigen</span>
            <input
              type="password"
              required
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <button
            type="submit"
            disabled={pwSubmitting}
            className="rounded-md bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm disabled:opacity-50 self-start"
          >
            {pwSubmitting ? 'Speichere…' : 'Passwort ändern'}
          </button>
          {pwMsg && <p className="text-sm">{pwMsg}</p>}
        </form>
      </section>
    </main>
  );
}

function ReadOnlyField({label, value}: {label: string; value: string}) {
  return (
    <div className="flex justify-between items-center gap-3 rounded-md border border-neutral-200 dark:border-neutral-800 px-3 py-2">
      <span className="text-xs uppercase tracking-wide opacity-60">{label}</span>
      <span className="text-sm">{value}</span>
    </div>
  );
}

function roleLabel(r: string): string {
  switch (r) {
    case 'global_admin':
      return 'Globaler Admin';
    case 'event_admin':
      return 'Event-Admin';
    case 'event_manager':
      return 'Event-Manager';
    default:
      return r;
  }
}
