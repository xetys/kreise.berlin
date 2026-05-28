'use client';

import {use} from 'react';
import {useSearchParams, usePathname} from 'next/navigation';
import {useTranslations} from 'next-intl';
import Link from 'next/link';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; slug: string}>;
}

export default function BookedPage({params}: PageProps) {
  const {slug} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const search = useSearchParams();
  const ref = search?.get('ref') ?? '';
  const status = search?.get('status') ?? 'booked';
  const isTest = search?.get('test') === '1';
  const isDonation = search?.get('mode') === 'donation';
  const paid = status === 'paid';
  const t = useTranslations('Booked');

  let headline: string;
  let subline: string;
  if (isTest) {
    headline = t('testHeadline');
    subline = t('testBody');
  } else if (isDonation) {
    headline = t('donationHeadline');
    subline = t('donationBody');
  } else if (paid) {
    headline = t('paidHeadline');
    subline = t('paidBody');
  } else {
    headline = t('bookedHeadline');
    subline = t('bookedBody');
  }

  return (
    <main className="flex-1 max-w-2xl mx-auto w-full px-6 py-16 flex flex-col gap-6 text-center">
      {isTest && (
        <div className="rounded p-3 border-2 border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-600 text-sm">
          {t('testBanner')}
        </div>
      )}
      <h1 className="text-3xl font-semibold">{headline}</h1>
      <p className="text-neutral-700 dark:text-neutral-300">{subline}</p>
      <div className="rounded-lg border border-amber-300 bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:border-amber-700 dark:text-amber-100 px-4 py-3 text-sm text-left">
        <p className="font-medium mb-1">{t('spamTitle')}</p>
        <p>{t('spamBody')}</p>
      </div>
      {ref && (
        <p className="text-sm text-neutral-500">
          {t('reference')}: <span className="font-mono">{ref}</span>
        </p>
      )}
      <div className="flex justify-center gap-4">
        <Link href={`/${localePrefix}/events/${slug}`} className="text-sm hover:underline">
          {t('backToEvent')}
        </Link>
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
