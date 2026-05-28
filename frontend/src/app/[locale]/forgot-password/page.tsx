'use client';

import {useState} from 'react';
import {usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';
import {LegalLinks} from '@/components/LegalLinks';

export default function ForgotPasswordPage() {
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const isEN = localePrefix === 'en';

  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api('/api/forgot-password', {method: 'POST', body: {email: email.trim().toLowerCase()}});
      setDone(true);
    } catch (err) {
      // Should never happen — backend always returns 200 for this endpoint.
      // Still surface if something genuinely broke (e.g. 500).
      setError(err instanceof APIError ? err.developerMessage : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex-1 max-w-md mx-auto w-full px-6 py-16 flex flex-col gap-6">
      <header className="text-center">
        <h1 className="text-3xl font-light tracking-wide">
          {isEN ? 'Reset your password' : 'Passwort zurücksetzen'}
        </h1>
        <p className="text-sm opacity-70 mt-2">
          {isEN
            ? 'Enter the email address tied to your kreise.berlin admin account. If it matches an active account, we send a reset link.'
            : 'Gib die E-Mail-Adresse deines kreise.berlin-Admin-Accounts ein. Wir senden dir einen Reset-Link, falls die Adresse zu einem aktiven Account passt.'}
        </p>
      </header>

      {done ? (
        <div className="rounded-md border-2 border-amber-300 bg-amber-50 dark:bg-amber-950/40 dark:border-amber-700 p-4 text-sm text-center">
          {isEN
            ? "If an account exists for that address, a reset email is on its way. Check your inbox (and Spam folder) within the next minute."
            : 'Falls ein Account mit dieser Adresse existiert, ist eine E-Mail unterwegs. Schau in deinem Posteingang nach (und im Spam-Ordner), in den nächsten Minuten.'}
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">
              {isEN ? 'Email' : 'E-Mail'}
            </span>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
              autoComplete="email"
            />
          </label>
          <button
            type="submit"
            disabled={submitting}
            className="rounded-md bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm disabled:opacity-50"
          >
            {submitting
              ? isEN ? 'Sending…' : 'Sende…'
              : isEN ? 'Send reset link' : 'Reset-Link senden'}
          </button>
          {error && <p className="text-sm text-red-700 dark:text-red-300">{error}</p>}
        </form>
      )}

      <p className="text-center text-sm">
        <Link href={`/${localePrefix}/login`} className="opacity-70 hover:opacity-100 hover:underline">
          ← {isEN ? 'back to sign-in' : 'zurück zur Anmeldung'}
        </Link>
      </p>

      <div className="pt-8 text-[10px] uppercase tracking-[0.18em] opacity-50 text-center">
        <LegalLinks locale={localePrefix} />
      </div>
    </main>
  );
}
