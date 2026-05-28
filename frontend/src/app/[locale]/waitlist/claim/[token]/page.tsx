'use client';

import {use, useEffect, useState} from 'react';
import {usePathname, useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; token: string}>;
}

interface ClaimResponse {
  outcome: 'claimed' | 'already_claimed';
  booking_reference: string;
  booking_id?: string;
  event_slug?: string;
  locale?: string;
}

export default function WaitlistClaimPage({params}: PageProps) {
  const {token} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const router = useRouter();
  const t = useTranslations('WaitlistClaim');

  const [state, setState] = useState<'pending' | 'lost' | 'invalid' | 'error' | 'claimed'>('pending');
  const [errorDetail, setErrorDetail] = useState<string>('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await api<ClaimResponse>(`/api/waitlist/claim/${encodeURIComponent(token)}`, {
          method: 'POST',
        });
        if (cancelled) return;
        const slug = resp.event_slug;
        const ref = resp.booking_reference;
        if (slug && ref) {
          setState('claimed');
          router.replace(
            `/${localePrefix}/events/${slug}/booked?ref=${encodeURIComponent(ref)}&status=booked`
          );
        } else {
          setState('error');
          setErrorDetail('Response was incomplete.');
        }
      } catch (err) {
        if (cancelled) return;
        if (err instanceof APIError) {
          if (err.code === 'claim_unavailable') {
            setState('lost');
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
  }, [token, localePrefix, router]);

  return (
    <main className="flex-1 max-w-2xl mx-auto w-full px-6 py-16 flex flex-col gap-6 text-center">
      {state === 'pending' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('pendingHeadline')}</h1>
          <p className="text-sm opacity-70">{t('pendingBody')}</p>
        </>
      )}

      {state === 'claimed' && (
        <>
          <h1 className="text-3xl font-light tracking-wide">{t('claimedHeadline')}</h1>
          <p className="text-sm opacity-70">{t('claimedBody')}</p>
        </>
      )}

      {state === 'lost' && (
        <>
          <div className="rounded p-3 border-2 border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-600 text-sm">
            {t('lostBanner')}
          </div>
          <h1 className="text-3xl font-light tracking-wide">{t('lostHeadline')}</h1>
          <p className="text-sm text-neutral-700 dark:text-neutral-300 max-w-md mx-auto">
            {t('lostBody')}
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

      <div className="flex justify-center gap-4 pt-2">
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
