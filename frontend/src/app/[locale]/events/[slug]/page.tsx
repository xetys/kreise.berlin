'use client';

import {use, useEffect, useState} from 'react';
import {usePathname} from 'next/navigation';
import {api, APIError} from '@/lib/api';
import type {PublicEventDetail} from '@/lib/public-types';
import {BookingForm} from '@/components/BookingForm';
import {EventDetailLayout} from '@/components/EventDetailLayout';

interface PageProps {
  params: Promise<{locale: string; slug: string}>;
}

export default function PublicEventDetailPage({params}: PageProps) {
  const {slug} = use(params);
  const pathname = usePathname();
  const locale = pathname.split('/')[1] ?? 'de';

  const [detail, setDetail] = useState<PublicEventDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<PublicEventDetail>(`/api/events/${slug}`)
      .then(setDetail)
      .catch((e) => {
        if (e instanceof APIError && e.status === 404) {
          setError('not_found');
        } else {
          setError(String(e));
        }
      });
  }, [slug]);

  if (error === 'not_found') {
    return (
      <main className="min-h-screen flex items-center justify-center px-6">
        <div className="text-center">
          <h1 className="text-3xl font-light tracking-wide">Veranstaltung nicht gefunden</h1>
          <p className="mt-3 text-sm text-neutral-500">Diese Veranstaltung existiert nicht oder wurde zurückgezogen.</p>
        </div>
      </main>
    );
  }
  if (error) return <p className="text-sm text-red-600 px-6 py-10">{error}</p>;
  if (!detail) {
    return (
      <main className="min-h-screen flex items-center justify-center">
        <p className="text-sm text-neutral-500">Lädt…</p>
      </main>
    );
  }

  return (
    <EventDetailLayout
      detail={detail}
      locale={locale}
      bookingSlot={
        <div className="bg-white/95 backdrop-blur-sm rounded-2xl p-5 sm:p-7 shadow-sm border border-black/5 text-neutral-900">
          <BookingForm detail={detail} />
        </div>
      }
    />
  );
}
