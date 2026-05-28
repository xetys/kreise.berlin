'use client';

import {use, useEffect, useState} from 'react';
import {usePathname} from 'next/navigation';
import {useTranslations} from 'next-intl';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; token: string}>;
}

export default function WaitlistRemovePage({params}: PageProps) {
  const {token} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const t = useTranslations('WaitlistRemove');

  const [state, setState] = useState<'pending' | 'removed' | 'fulfilled' | 'invalid' | 'error'>('pending');
  const [errorDetail, setErrorDetail] = useState<string>('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await api<{outcome: string}>(`/api/waitlist/remove/${encodeURIComponent(token)}`, {
          method: 'POST',
        });
        if (!cancelled) setState('removed');
      } catch (err) {
        if (cancelled) return;
        if (err instanceof APIError) {
          if (err.code === 'already_fulfilled') {
            setState('fulfilled');
          } else if (err.code === 'invalid_token') {
            setState('invalid');
          } else {
            setState('error');
            setErrorDetail(err.developerMessage);
          }
        } else {
          setState('error');
          setErrorDetail(String(err));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <main className="flex-1 max-w-2xl mx-auto w-full px-6 py-16 flex flex-col gap-6 text-center">
      {state === 'pending' && (
        <h1 className="text-3xl font-light tracking-wide">{t('pendingHeadline')}</h1>
      )}

      {state === 'removed' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('removedHeadline')}</h1>
          <p className="text-sm text-neutral-700 dark:text-neutral-300 max-w-md mx-auto">
            {t('removedBody')}
          </p>
        </>
      )}

      {state === 'fulfilled' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('fulfilledHeadline')}</h1>
          <p className="text-sm text-neutral-700 dark:text-neutral-300 max-w-md mx-auto">
            {t('fulfilledBody')}
          </p>
        </>
      )}

      {state === 'invalid' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('invalidHeadline')}</h1>
          <p className="text-sm text-neutral-700 dark:text-neutral-300 max-w-md mx-auto">
            {t('invalidBody')}
          </p>
        </>
      )}

      {state === 'error' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('errorHeadline')}</h1>
          <p className="text-sm text-red-700">{errorDetail || t('errorFallback')}</p>
        </>
      )}

      <div className="flex justify-center gap-4 pt-4">
        <Link href={`/${localePrefix}`} className="text-sm hover:underline">
          {t('allEvents')}
        </Link>
      </div>
      <div className="pt-8 text-[10px] uppercase tracking-[0.18em] opacity-50">
        <LegalLinks locale={localePrefix} />
      </div>
    </main>
  );
}
