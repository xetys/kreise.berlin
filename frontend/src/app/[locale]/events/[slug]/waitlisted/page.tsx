'use client';

import {use} from 'react';
import {useSearchParams, usePathname} from 'next/navigation';
import {useTranslations} from 'next-intl';
import Link from 'next/link';
import {LegalLinks} from '@/components/LegalLinks';

interface PageProps {
  params: Promise<{locale: string; slug: string}>;
}

export default function WaitlistedPage({params}: PageProps) {
  const {slug} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const search = useSearchParams();
  const position = Number(search?.get('pos') ?? 0);
  const total = Number(search?.get('total') ?? 0);
  const t = useTranslations('Waitlisted');

  return (
    <main className="flex-1 max-w-2xl mx-auto w-full px-6 py-16 flex flex-col gap-6 text-center">
      <div className="rounded p-3 border-2 border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-600 text-sm">
        {t('banner')}
      </div>
      <h1 className="text-3xl font-light tracking-wide">{t('headline')}</h1>

      {position > 0 && total > 0 && (
        <p className="text-base">
          <span className="font-medium">{t('position', {pos: position, total})}</span>
        </p>
      )}

      <div className="text-sm text-neutral-700 dark:text-neutral-300 max-w-md mx-auto flex flex-col gap-3">
        <p>{t('explainPaid')}</p>
        <p>{t('explainModes')}</p>
        <p className="opacity-70">{t('removeHint')}</p>
      </div>

      <div className="flex justify-center gap-4 pt-4">
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
