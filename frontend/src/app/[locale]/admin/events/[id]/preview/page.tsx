'use client';

import {use, useEffect, useState} from 'react';
import {usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';
import type {PublicEventDetail} from '@/lib/public-types';
import {EventDetailLayout} from '@/components/EventDetailLayout';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

/**
 * Admin-only preview of an event's public landing page. Bypasses the
 * is_public / archived gate so admins can see exactly how an unpublished
 * event will look. Booking form is replaced with a placeholder — bookings
 * remain disabled until the event is actually published.
 */
export default function AdminPreviewPage({params}: PageProps) {
  const {id} = use(params);
  const pathname = usePathname();
  const locale = pathname.split('/')[1] ?? 'de';

  const [detail, setDetail] = useState<PublicEventDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<PublicEventDetail>(`/api/admin/events/${id}/preview`)
      .then(setDetail)
      .catch((e) => {
        if (e instanceof APIError && e.status === 404) setError('not_found');
        else if (e instanceof APIError && e.status === 401) setError('unauthorized');
        else if (e instanceof APIError && e.status === 403) setError('forbidden');
        else setError(String(e));
      });
  }, [id]);

  if (error === 'not_found') {
    return notice('Diese Veranstaltung existiert nicht.');
  }
  if (error === 'unauthorized') {
    return notice('Bitte melde dich neu an.');
  }
  if (error === 'forbidden') {
    return notice('Du hast keine Berechtigung, diese Veranstaltung anzusehen.');
  }
  if (error) return <p className="text-sm text-red-600 px-6 py-10">{error}</p>;
  if (!detail) {
    return (
      <main className="min-h-screen flex items-center justify-center">
        <p className="text-sm text-neutral-500">Lädt…</p>
      </main>
    );
  }

  const isPublished = detail.event.is_public && !detail.event.is_archived;

  return (
    <EventDetailLayout
      detail={detail}
      locale={locale}
      topBanner={
        <div className="bg-amber-100 dark:bg-amber-950/40 border-b-2 border-amber-400 dark:border-amber-700 text-amber-900 dark:text-amber-100 px-5 py-3">
          <div className="max-w-3xl mx-auto flex items-center justify-between gap-4 text-sm">
            <span>
              <span className="font-semibold">Vorschau-Modus.</span>{' '}
              {isPublished
                ? 'Diese Veranstaltung ist bereits veröffentlicht — du siehst hier denselben Inhalt wie alle anderen.'
                : 'Diese Veranstaltung ist noch nicht öffentlich. Buchungen sind erst nach der Veröffentlichung möglich.'}
            </span>
            <Link
              href={`/${locale}/admin/events/${id}`}
              className="shrink-0 rounded border border-amber-400 dark:border-amber-700 px-3 py-1 hover:bg-amber-200 dark:hover:bg-amber-900/40"
            >
              ← Zurück zur Verwaltung
            </Link>
          </div>
        </div>
      }
      bookingSlot={
        <div className="bg-white/95 backdrop-blur-sm rounded-2xl p-6 shadow-sm border border-black/5 text-neutral-700 text-center">
          <p className="font-medium">Buchungs-Formular</p>
          <p className="text-sm opacity-80 mt-1">
            {isPublished
              ? 'Auf der öffentlichen Seite erscheint hier das Formular zur Ticketbuchung.'
              : 'Sobald die Veranstaltung veröffentlicht ist, erscheint hier das Formular zur Ticketbuchung.'}
          </p>
        </div>
      }
    />
  );
}

function notice(text: string) {
  return (
    <main className="min-h-screen flex items-center justify-center px-6">
      <div className="text-center">
        <h1 className="text-2xl font-light tracking-wide">Vorschau nicht möglich</h1>
        <p className="mt-3 text-sm text-neutral-500">{text}</p>
      </div>
    </main>
  );
}
