'use client';

import {useTranslations} from 'next-intl';
import type {PublicEventDetail} from '@/lib/public-types';
import {EventLocationMap} from './EventLocationMap';
import {LegalLinks} from './LegalLinks';
import {LocaleSwitcher} from './LocaleSwitcher';

function bcp(locale: string): string {
  return locale === 'en' ? 'en-GB' : 'de-DE';
}

/**
 * Themed event detail layout — shared by the public event page and the admin
 * preview page. Caller passes the booking section in via `bookingSlot` so the
 * preview can substitute a placeholder when the event isn't yet bookable.
 */
export function EventDetailLayout({
  detail,
  locale,
  bookingSlot,
  topBanner,
}: {
  detail: PublicEventDetail;
  locale: string;
  bookingSlot: React.ReactNode;
  topBanner?: React.ReactNode;
}) {
  const t = useTranslations('EventDetail');
  const {event, program} = detail;
  const themeStyle: React.CSSProperties = {
    ['--ev-primary' as string]: event.color_primary,
    ['--ev-secondary' as string]: event.color_secondary,
    ['--ev-text' as string]: event.color_text,
    backgroundColor: event.color_secondary,
    color: event.color_text,
  };

  return (
    <div className="min-h-screen flex flex-col" style={themeStyle}>
      {topBanner}
      <Hero event={event} locale={locale} />

      <main className="flex-1 px-5 py-12 sm:py-16">
        <div className="max-w-xl mx-auto flex flex-col gap-12">
          <EventMetaStrip event={event} locale={locale} />

          {event.location && (
            <section>
              <h2 className="text-center text-[11px] uppercase tracking-[0.2em] opacity-60 mb-5">
                {t('directions')}
              </h2>
              <EventLocationMap location={event.location} />
            </section>
          )}

          {event.description && (
            <section className="text-center">
              <p className="whitespace-pre-line leading-relaxed text-[15px] opacity-90">
                {event.description}
              </p>
            </section>
          )}

          {program.length > 0 && (
            <section>
              <h2 className="text-center text-[11px] uppercase tracking-[0.2em] opacity-60 mb-5">
                {t('program')}
              </h2>
              <div
                className="rounded-xl px-6 py-5"
                style={{backgroundColor: event.color_primary, color: event.color_text}}
              >
                <ul className="flex flex-col gap-2 text-sm">
                  {program.map((p) => (
                    <li key={p.id} className="flex justify-between gap-4">
                      <span className="opacity-70 tabular-nums whitespace-nowrap">
                        {formatTime(p.starts_at, locale)}
                        {p.ends_at && (
                          <>
                            {' – '}
                            {formatTime(p.ends_at, locale)}
                          </>
                        )}
                      </span>
                      <span className="text-right">
                        <strong className="font-medium">{p.title}</strong>
                        {p.description && (
                          <span className="block opacity-70 text-xs mt-0.5">{p.description}</span>
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            </section>
          )}

          <section id="book">
            <div className="text-center mb-8">
              <h2 className="text-2xl sm:text-3xl font-light tracking-wide">{t('bookHeadline')}</h2>
              <p className="mt-1 text-sm opacity-70">{paymentSubline(event, t)}</p>
            </div>

            {detail.capacity_full && (
              <div
                className="rounded-xl p-4 mb-5 border-2 border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-600 text-sm text-center"
                role="status"
              >
                <p className="font-medium mb-0.5">{t('soldOutTitle')}</p>
                <p>{t('soldOutBody')}</p>
              </div>
            )}

            {bookingSlot}
          </section>
        </div>
      </main>

      <footer
        className="text-center text-[10px] uppercase tracking-[0.22em] py-8 opacity-70 flex flex-col items-center gap-2"
        style={{borderTop: '1px solid rgba(0,0,0,0.06)'}}
      >
        <span>{event.name} · kreise.berlin</span>
        <LegalLinks locale={locale} className="tracking-[0.18em]" />
      </footer>
    </div>
  );
}

function paymentSubline(
  event: PublicEventDetail['event'],
  t: (key: string) => string
): string {
  if (event.payment_test_mode) return t('subTestMode');
  if (event.pricing_mode === 'donation') return t('subDonation');
  if (event.payment_timing === 'at_door') return t('subAtDoor');
  return t('subBeforehand');
}

function Hero({event, locale}: {event: PublicEventDetail['event']; locale: string}) {
  const t = useTranslations('EventDetail');
  return (
    <header
      className="relative w-full overflow-hidden"
      style={{backgroundColor: event.color_primary, color: event.color_text}}
    >
      {event.banner_url && (
        <img
          src={event.banner_url}
          alt=""
          className="absolute inset-0 w-full h-full object-cover opacity-50"
        />
      )}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background: `linear-gradient(180deg, transparent 0%, ${event.color_primary}AA 70%, ${event.color_primary} 100%)`,
        }}
        aria-hidden
      />

      <div className="absolute top-4 right-5 z-10" style={{color: event.color_text}}>
        <LocaleSwitcher current={locale === 'en' ? 'en' : 'de'} />
      </div>

      <div className="relative px-5 py-16 sm:py-24 flex flex-col items-center gap-3 text-center">
        <p className="text-[11px] uppercase tracking-[0.25em] opacity-80">
          {formatDateRange(event.starts_at, event.ends_at, locale)}
        </p>
        <h1
          className="text-3xl sm:text-5xl font-light tracking-wide leading-tight max-w-3xl"
          style={{textShadow: '0 1px 2px rgba(0,0,0,0.1)'}}
        >
          {event.name}
        </h1>
        {event.location && <p className="text-sm sm:text-base opacity-90 mt-1">{event.location}</p>}
        <a
          href="#book"
          className="mt-6 inline-flex items-center gap-2 rounded-full px-6 py-2.5 text-sm font-medium border-2 hover:opacity-90 transition-opacity"
          style={{borderColor: event.color_text, color: event.color_text}}
        >
          {t('viewTickets')}
        </a>
      </div>
    </header>
  );
}

function EventMetaStrip({event, locale}: {event: PublicEventDetail['event']; locale: string}) {
  const t = useTranslations('EventDetail');
  const sameDay =
    new Date(event.starts_at).toDateString() === new Date(event.ends_at).toDateString();

  const timeSuffix = t('metaStartsSuffix');
  const cells: {label: string; value: string}[] = [
    {label: t('metaDate'), value: formatLongDate(event.starts_at, locale)},
    {
      label: t('metaStarts'),
      value: formatTime(event.starts_at, locale) + (timeSuffix ? ' ' + timeSuffix : ''),
    },
  ];
  if (event.location) cells.push({label: t('metaLocation'), value: event.location});
  if (!sameDay) {
    cells.push({
      label: t('metaDuration'),
      value: t('metaDurationUntil', {
        date: new Date(event.ends_at).toLocaleDateString(bcp(locale), {
          day: '2-digit',
          month: 'short',
        }),
      }),
    });
  }
  if (event.participant_limit !== null) {
    cells.push({
      label: t('metaSeats'),
      value: t('metaSeatsValue', {limit: event.participant_limit}),
    });
  }

  return (
    <section>
      <div className="grid grid-cols-2 gap-x-8 gap-y-5 text-sm">
        {cells.map((c, i) => (
          <MetaCell
            key={c.label}
            label={c.label}
            value={c.value}
            align={i % 2 === 0 ? 'right' : 'left'}
          />
        ))}
      </div>
      <div
        className="h-px w-32 mx-auto mt-10 opacity-30"
        style={{backgroundColor: 'currentColor'}}
        aria-hidden
      />
    </section>
  );
}

function MetaCell({label, value, align}: {label: string; value: string; align: 'left' | 'right'}) {
  return (
    <div className={align === 'right' ? 'text-right' : 'text-left'}>
      <span className="block text-[10px] uppercase tracking-[0.22em] opacity-60 mb-1">{label}</span>
      <span className="text-[15px]">{value}</span>
    </div>
  );
}

function formatLongDate(iso: string, locale: string): string {
  return new Date(iso).toLocaleDateString(bcp(locale), {
    weekday: 'long',
    day: '2-digit',
    month: 'long',
    year: 'numeric',
  });
}

function formatTime(iso: string, locale: string): string {
  return new Date(iso).toLocaleTimeString(bcp(locale), {hour: '2-digit', minute: '2-digit'});
}

function formatDateRange(startIso: string, endIso: string, locale: string): string {
  const s = new Date(startIso);
  const e = new Date(endIso);
  if (s.toDateString() === e.toDateString()) return formatLongDate(startIso, locale);
  const opts: Intl.DateTimeFormatOptions = {day: '2-digit', month: 'long', year: 'numeric'};
  return `${s.toLocaleDateString(bcp(locale), opts)} – ${e.toLocaleDateString(bcp(locale), opts)}`;
}
