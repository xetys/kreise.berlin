'use client';

import {use, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {api, APIError} from '@/lib/api';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; token: string}>;
}

export default function SetupPage({params}: PageProps) {
  const {token} = use(params);
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const t = useTranslations('Setup');

  const [pw, setPw] = useState('');
  const [pwc, setPwc] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setMsg(null);
    if (pw !== pwc) {
      setMsg(t('mismatch'));
      return;
    }
    if (pw.length < 12) {
      setMsg(t('tooShort'));
      return;
    }
    setSubmitting(true);
    try {
      await api(`/api/setup/${encodeURIComponent(token)}`, {
        method: 'POST',
        body: {password: pw, password_confirm: pwc, display_name: displayName.trim()},
      });
      setDone(true);
      setTimeout(() => router.replace(`/${localePrefix}/login`), 1500);
    } catch (err) {
      if (err instanceof APIError && err.code === 'invalid_token') {
        setMsg(t('invalidToken'));
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
        <h1 className="text-3xl font-light tracking-wide">{t('headline')}</h1>
        <p className="text-sm opacity-70 mt-2">{t('intro')}</p>
      </header>

      {done ? (
        <div className="rounded-md border-2 border-green-400 bg-green-50 dark:bg-green-950/40 dark:border-green-700 p-4 text-sm text-center">
          {t('doneBanner')}
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">
              {localePrefix === 'en' ? 'Display name (optional)' : 'Anzeigename (optional)'}
            </span>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              maxLength={200}
              autoComplete="name"
              placeholder={localePrefix === 'en' ? 'e.g. Jane Doe' : 'z. B. Vor- und Nachname'}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">{t('passwordLabel')}</span>
            <input
              type="password"
              required
              minLength={12}
              value={pw}
              onChange={(e) => setPw(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">{t('passwordConfirmLabel')}</span>
            <input
              type="password"
              required
              value={pwc}
              onChange={(e) => setPwc(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <button
            type="submit"
            disabled={submitting}
            className="rounded-md bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm disabled:opacity-50"
          >
            {submitting ? t('submitting') : t('submit')}
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
