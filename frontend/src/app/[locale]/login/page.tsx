'use client';

import {useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';

export default function LoginPage() {
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await api('/api/auth/login', {method: 'POST', body: {email, password}});
      router.push('/admin/events');
    } catch (err) {
      if (err instanceof APIError) {
        setError(err.code === 'invalid_credentials' ? 'Falsche E-Mail oder falsches Passwort.' : err.code);
      } else {
        setError('Unbekannter Fehler');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex-1 flex flex-col items-center justify-center p-8">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm flex flex-col gap-4 border border-neutral-200 dark:border-neutral-800 rounded-lg p-6"
      >
        <h1 className="text-xl font-semibold">Anmelden</h1>

        <label className="flex flex-col gap-1 text-sm">
          E-Mail
          <input
            type="email"
            required
            autoFocus
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="border border-neutral-300 dark:border-neutral-700 rounded px-3 py-2 bg-transparent"
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          Passwort
          <input
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="border border-neutral-300 dark:border-neutral-700 rounded px-3 py-2 bg-transparent"
          />
        </label>

        {error && <p className="text-sm text-red-600">{error}</p>}

        <button
          type="submit"
          disabled={submitting}
          className="bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 rounded px-4 py-2 disabled:opacity-50"
        >
          {submitting ? 'Anmelden…' : 'Anmelden'}
        </button>

        <Link
          href={`/${localePrefix}/forgot-password`}
          className="text-xs text-center text-neutral-600 dark:text-neutral-400 hover:underline"
        >
          Passwort vergessen?
        </Link>
      </form>
    </main>
  );
}
