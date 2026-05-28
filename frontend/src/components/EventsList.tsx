'use client';

import Link from 'next/link';
import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {api} from '@/lib/api';
import type {EventDTO} from '@/lib/types';

interface ListResp {
  events: EventDTO[];
}

function formatRange(startsISO: string, endsISO: string, locale: string): string {
  const bcp = locale === 'en' ? 'en-GB' : 'de-DE';
  const s = new Date(startsISO);
  const e = new Date(endsISO);
  const opts: Intl.DateTimeFormatOptions = {day: '2-digit', month: 'long', year: 'numeric'};
  if (s.toDateString() === e.toDateString()) {
    return s.toLocaleDateString(bcp, opts);
  }
  return `${s.toLocaleDateString(bcp, opts)} – ${e.toLocaleDateString(bcp, opts)}`;
}

export function EventsList({locale}: {locale: string}) {
  const t = useTranslations('EventsList');
  const [events, setEvents] = useState<EventDTO[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<ListResp>('/api/events')
      .then((r) => setEvents(r.events))
      .catch((e) => setError(String(e)));
  }, []);

  return (
    <>
      <section className="text-center">
        <h1 className="text-3xl sm:text-4xl font-light tracking-wide">{t('headline')}</h1>
        <p className="mt-3 text-sm opacity-70 max-w-md mx-auto">{t('intro')}</p>
      </section>

      {error && <p className="text-sm text-red-600 text-center">{t('error')}</p>}
      {events === null && !error && (
        <p className="text-sm text-neutral-500 text-center">{t('loading')}</p>
      )}
      {events && events.length === 0 && (
        <p className="text-sm text-neutral-500 text-center">{t('empty')}</p>
      )}
      {events && events.length > 0 && (
        <ul className="flex flex-col gap-6">
          {events.map((e) => (
            <li key={e.id}>
              <EventCard event={e} locale={locale} />
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function EventCard({event, locale}: {event: EventDTO; locale: string}) {
  return (
    <Link
      href={`/${locale}/events/${event.slug}`}
      className="block group relative overflow-hidden rounded-2xl shadow-sm hover:shadow-md transition-shadow border border-black/5"
      style={{backgroundColor: event.color_primary, color: event.color_text}}
    >
      <div className="aspect-[16/9] sm:aspect-[2/1] relative overflow-hidden">
        {event.banner_url ? (
          <img
            src={event.banner_url}
            alt=""
            className="absolute inset-0 w-full h-full object-cover opacity-60 group-hover:opacity-70 group-hover:scale-[1.02] transition-all duration-500"
          />
        ) : (
          <div
            className="absolute inset-0"
            style={{
              background: `linear-gradient(135deg, ${event.color_primary} 0%, ${event.color_secondary} 100%)`,
            }}
          />
        )}
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: `linear-gradient(180deg, transparent 30%, ${event.color_primary}DD 100%)`,
          }}
        />
        <div className="absolute inset-0 flex flex-col justify-end p-6">
          <p className="text-[10px] uppercase tracking-[0.22em] opacity-80 mb-1">
            {formatRange(event.starts_at, event.ends_at, locale)}
          </p>
          <h2 className="text-2xl sm:text-3xl font-light tracking-wide leading-tight">
            {event.name}
          </h2>
          {event.location && <p className="text-sm opacity-80 mt-1">{event.location}</p>}
        </div>
      </div>
      {event.description && (
        <div
          className="px-6 py-4 text-sm leading-relaxed opacity-90"
          style={{backgroundColor: event.color_secondary, color: event.color_text}}
        >
          <p className="line-clamp-2">{event.description}</p>
        </div>
      )}
    </Link>
  );
}
