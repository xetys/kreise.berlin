'use client';

import {use, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import {api, APIError} from '@/lib/api';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; token: string}>;
}

export default function ResetPasswordPage({params}: PageProps) {
  const {token} = use(params);
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const isEN = localePrefix === 'en';

  const [pw, setPw] = useState('');
  const [pwc, setPwc] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setMsg(null);
    if (pw !== pwc) {
      setMsg(isEN ? 'The passwords don\'t match.' : 'Die Passwörter stimmen nicht überein.');
      return;
    }
    if (pw.length < 12) {
      setMsg(isEN ? 'Password must be at least 12 characters.' : 'Das Passwort muss mindestens 12 Zeichen lang sein.');
      return;
    }
    setSubmitting(true);
    try {
      await api(`/api/reset-password/${encodeURIComponent(token)}`, {
        method: 'POST',
        body: {password: pw, password_confirm: pwc},
      });
      setDone(true);
      setTimeout(() => router.replace(`/${localePrefix}/login`), 1500);
    } catch (err) {
      if (err instanceof APIError && err.code === 'invalid_token') {
        setMsg(
          isEN
            ? 'This reset link is invalid or expired. Request a new one on the forgot-password page.'
            : 'Dieser Reset-Link ist ungültig oder abgelaufen. Fordere auf der Vergessen-Seite einen neuen an.'
        );
      } else if (err instanceof APIError) {
        setMsg(err.code + ': ' + err.developerMessage);
      } else {
        setMsg(String(err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex-1 max-w-md mx-auto w-full px-6 py-16 flex flex-col gap-6">
      <header className="text-center">
        <h1 className="text-3xl font-light tracking-wide">
          {isEN ? 'Set a new password' : 'Neues Passwort setzen'}
        </h1>
        <p className="text-sm opacity-70 mt-2">
          {isEN
            ? 'Choose a fresh password. Doing this signs you out of all other devices.'
            : 'Wähle ein neues Passwort. Dabei wirst du auf allen anderen Geräten automatisch abgemeldet.'}
        </p>
      </header>

      {done ? (
        <div className="rounded-md border-2 border-green-400 bg-green-50 dark:bg-green-950/40 dark:border-green-700 p-4 text-sm text-center">
          {isEN ? 'Password updated — redirecting to sign in…' : 'Passwort gesetzt — du wirst zur Anmeldung weitergeleitet…'}
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">
              {isEN ? 'New password (≥12 characters)' : 'Neues Passwort (≥12 Zeichen)'}
            </span>
            <input
              type="password"
              required
              minLength={12}
              value={pw}
              onChange={(e) => setPw(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
              autoComplete="new-password"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">
              {isEN ? 'Confirm password' : 'Passwort bestätigen'}
            </span>
            <input
              type="password"
              required
              value={pwc}
              onChange={(e) => setPwc(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
              autoComplete="new-password"
            />
          </label>
          <button
            type="submit"
            disabled={submitting}
            className="rounded-md bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm disabled:opacity-50"
          >
            {submitting
              ? isEN ? 'Setting…' : 'Setze…'
              : isEN ? 'Set new password' : 'Neues Passwort setzen'}
          </button>
          {msg && <p className="text-sm text-red-700 dark:text-red-300">{msg}</p>}
        </form>
      )}

      <div className="pt-8 text-[10px] uppercase tracking-[0.18em] opacity-50 text-center">
        <LegalLinks locale={localePrefix} />
      </div>
    </main>
  );
}
