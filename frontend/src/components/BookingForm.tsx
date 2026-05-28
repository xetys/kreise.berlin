'use client';

import {useEffect, useMemo, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {api, APIError} from '@/lib/api';
import type {
  PublicEventDetail,
  QuoteResponse,
  BookingResponse,
  Category,
  Duration,
} from '@/lib/public-types';
import {formatMoney} from '@/lib/public-types';

interface Participant {
  name: string;
  email: string;
  category_id?: string;
  duration_id?: string;
  donation_amount_minor?: number;
}

export function BookingForm({detail}: {detail: PublicEventDetail}) {
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const t = useTranslations('BookingForm');
  const tErr = useTranslations('BookingForm.errors');
  const {event, donation_config: donationCfg, categories, durations} = detail;
  const isDonation = event.pricing_mode === 'donation';
  const isAtDoor = event.payment_timing === 'at_door';
  const isTestMode = event.payment_test_mode;
  const registrationOnly = isDonation || isAtDoor;
  const ctaLabel = isTestMode
    ? t('submitTest')
    : registrationOnly
      ? t('submitRegister')
      : t('submitDefault');

  function translateError(code: string): string {
    try {
      return tErr(code);
    } catch {
      return `Error: ${code}`;
    }
  }

  const [contact, setContact] = useState({name: '', email: '', phone: ''});
  const [newsletterOptin, setNewsletterOptin] = useState(false);
  const [couponCode, setCouponCode] = useState('');
  const [participants, setParticipants] = useState<Participant[]>([
    isDonation
      ? {name: '', email: '', donation_amount_minor: donationCfg?.suggested_minor ?? 0}
      : {
          name: '',
          email: '',
          category_id: categories[0]?.id,
          duration_id: durations[0]?.id,
        },
  ]);

  const [quote, setQuote] = useState<QuoteResponse | null>(null);
  const [quoteError, setQuoteError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Live quote — debounced via key dependency.
  const quoteKey = useMemo(
    () =>
      JSON.stringify({
        slug: event.slug,
        coupon: couponCode.trim(),
        email: contact.email.trim().toLowerCase(),
        ps: participants.map((p) => ({
          c: p.category_id,
          d: p.duration_id,
          a: p.donation_amount_minor,
        })),
      }),
    [event.slug, couponCode, contact.email, participants]
  );

  useEffect(() => {
    let cancelled = false;
    const t = setTimeout(async () => {
      try {
        const q = await api<QuoteResponse>('/api/quote', {
          method: 'POST',
          body: {
            event_slug: event.slug,
            contact_email: contact.email,
            coupon_code: couponCode.trim() || undefined,
            participants: participants.map(participantToInput),
          },
        });
        if (!cancelled) {
          setQuote(q);
          setQuoteError(null);
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof APIError) {
          setQuote(null);
          setQuoteError(translateError(e.code));
        } else {
          setQuoteError(String(e));
        }
      }
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [quoteKey, event.slug, couponCode, contact.email, participants]);

  function updateParticipant(i: number, patch: Partial<Participant>) {
    setParticipants((arr) => arr.map((p, idx) => (idx === i ? {...p, ...patch} : p)));
  }
  function addParticipant() {
    setParticipants((arr) => [
      ...arr,
      isDonation
        ? {name: '', email: '', donation_amount_minor: donationCfg?.suggested_minor ?? 0}
        : {name: '', email: '', category_id: categories[0]?.id, duration_id: durations[0]?.id},
    ]);
  }
  function removeParticipant(i: number) {
    setParticipants((arr) => (arr.length > 1 ? arr.filter((_, idx) => idx !== i) : arr));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitError(null);
    setSubmitting(true);
    try {
      const resp = await api<BookingResponse>('/api/bookings', {
        method: 'POST',
        body: {
          event_slug: event.slug,
          contact: {name: contact.name, email: contact.email, phone: contact.phone || undefined},
          newsletter_optin: newsletterOptin,
          coupon_code: couponCode.trim() || undefined,
          locale: 'de',
          participants: participants.map(participantToInput),
        },
      });
      if (resp.outcome === 'waitlisted') {
        const params = new URLSearchParams({
          pos: String(resp.waitlist_position ?? 0),
          total: String(resp.waitlist_total ?? 0),
        });
        router.push(`/${localePrefix}/events/${event.slug}/waitlisted?${params.toString()}`);
        return;
      }
      const modeQS = isDonation ? '&mode=donation' : '';
      router.push(
        `/${localePrefix}/events/${event.slug}/booked?ref=${encodeURIComponent(resp.booking_reference ?? '')}&status=${encodeURIComponent(resp.status ?? '')}${isTestMode ? '&test=1' : ''}${modeQS}`
      );
    } catch (err) {
      if (err instanceof APIError) {
        setSubmitError(translateError(err.code));
      } else {
        setSubmitError(String(err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-6">
      {isTestMode && (
        <div className="rounded p-4 border-2 border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-600 flex flex-col gap-1">
          <p className="font-bold text-base">{t('testModeBanner')}</p>
          <p className="text-sm">{t('testModeBannerBody')}</p>
        </div>
      )}

      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-medium mb-2">{t('contactLegend')}</legend>
        <input
          required
          placeholder={t('contactName')}
          value={contact.name}
          onChange={(e) => setContact((c) => ({...c, name: e.target.value}))}
          className={inputCls}
        />
        <input
          required
          type="email"
          placeholder={t('contactEmail')}
          value={contact.email}
          onChange={(e) => setContact((c) => ({...c, email: e.target.value}))}
          className={inputCls}
        />
        <input
          placeholder={t('contactPhone')}
          value={contact.phone}
          onChange={(e) => setContact((c) => ({...c, phone: e.target.value}))}
          className={inputCls}
        />
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={newsletterOptin}
            onChange={(e) => setNewsletterOptin(e.target.checked)}
          />
          {t('newsletter')}
        </label>
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-medium mb-2">
          {t('participantsLegend', {count: participants.length})}
        </legend>
        {participants.map((p, i) => (
          <ParticipantRow
            key={i}
            index={i}
            participant={p}
            isDonation={isDonation}
            categories={categories}
            durations={durations}
            onChange={(patch) => updateParticipant(i, patch)}
            onRemove={participants.length > 1 ? () => removeParticipant(i) : undefined}
          />
        ))}
        <button type="button" onClick={addParticipant} className={btnSecondary + ' self-start'}>
          {t('addParticipant')}
        </button>
      </fieldset>

      {!isDonation && (
        <fieldset className="flex flex-col gap-2">
          <legend className="text-sm font-medium mb-1">{t('couponLegend')}</legend>
          <input
            placeholder={t('couponPlaceholder')}
            value={couponCode}
            onChange={(e) => setCouponCode(e.target.value)}
            className={inputCls}
          />
        </fieldset>
      )}

      <div className="border-t border-neutral-200 dark:border-neutral-800 pt-4 flex flex-col gap-2">
        {quoteError && <p className="text-sm text-red-600">{quoteError}</p>}
        {/* Donation events: amount is informational, not a commitment — the
            min/suggested live in PaymentTimingBanner below, and we deliberately
            don't show line items / a Gesamt total because the booker doesn't
            commit to a number on registration. */}
        {!isDonation && quote && !quoteError && (
          <div className="text-sm flex flex-col gap-1">
            {quote.phase && (
              <p className="text-xs text-neutral-500">{t('phase', {name: quote.phase.name})}</p>
            )}
            {quote.line_items.map((li, i) => (
              <div key={i} className="flex justify-between">
                <span>
                  {participants[i]?.name || t('participantLabel', {n: i + 1})} · {li.description}
                </span>
                <span className="tabular-nums">{formatMoney(li.amount_minor, quote.currency)}</span>
              </div>
            ))}
            {quote.discount_minor > 0 && (
              <div className="flex justify-between text-emerald-700">
                <span>{t('discount')} {quote.applied_coupon_code ? `(${quote.applied_coupon_code})` : ''}</span>
                <span className="tabular-nums">−{formatMoney(quote.discount_minor, quote.currency)}</span>
              </div>
            )}
            <div className="flex justify-between font-medium border-t border-neutral-100 dark:border-neutral-900 pt-1">
              <span>{t('total')}</span>
              <span className="tabular-nums">{formatMoney(quote.total_minor, quote.currency)}</span>
            </div>
          </div>
        )}
      </div>

      {submitError && <p className="text-sm text-red-600">{submitError}</p>}

      <PaymentTimingBanner
        isAtDoor={isAtDoor}
        isDonation={isDonation}
        primary={event.color_primary}
        textColor={event.color_text}
        bankIBAN={event.bank_iban}
        bankBIC={event.bank_bic}
        bankHolder={event.bank_account_holder}
        paypalHandle={event.paypal_handle}
        totalMinor={quote?.total_minor}
        currency={quote?.currency ?? event.currency}
        donationMinMinor={donationCfg?.min_minor}
        donationSuggestedMinor={donationCfg?.suggested_minor}
      />

      <button
        type="submit"
        disabled={submitting || !quote || !!quoteError}
        className="rounded px-4 py-3 text-base font-medium disabled:opacity-50"
        style={{
          backgroundColor: event.color_primary,
          color: event.color_text,
        }}
      >
        {submitting ? t('submitSending') : ctaLabel}
      </button>
    </form>
  );
}

function PaymentTimingBanner({
  isAtDoor,
  isDonation,
  primary,
  textColor,
  bankIBAN,
  bankBIC,
  bankHolder,
  paypalHandle,
  totalMinor,
  currency,
  donationMinMinor,
  donationSuggestedMinor,
}: {
  isAtDoor: boolean;
  isDonation: boolean;
  primary: string;
  textColor: string;
  bankIBAN?: string;
  bankBIC?: string;
  bankHolder?: string;
  paypalHandle?: string;
  totalMinor?: number;
  currency: string;
  donationMinMinor?: number;
  donationSuggestedMinor?: number;
}) {
  const t = useTranslations('BookingForm');
  if (isAtDoor || isDonation) {
    // Donation amount range: prefer the actual configured min/suggested; if
    // either is missing, omit the line entirely rather than printing 0,00.
    // Build the contribution range. We deliberately show min – suggested even
    // when min is 0, because admins use the min field to communicate "no fixed
    // floor" intentionally and the bookers should see that. Only collapse to
    // a single value when min === suggested (no range to show).
    let rangeLabel: string | null = null;
    if (isDonation) {
      const min = donationMinMinor;
      const sug = donationSuggestedMinor;
      if (min !== undefined && sug !== undefined) {
        rangeLabel =
          min === sug
            ? t('donationSingle', {amount: formatMoney(sug, currency)})
            : t('donationRange', {
                min: formatMoney(min, currency),
                suggested: formatMoney(sug, currency),
              });
      } else if (sug !== undefined) {
        rangeLabel = t('donationSingle', {amount: formatMoney(sug, currency)});
      }
    }
    return (
      <div
        className="rounded p-4 flex flex-col gap-1"
        style={{backgroundColor: primary, color: textColor}}
      >
        <p className="font-semibold text-base">
          {isDonation ? t('paymentDonationHeadline') : t('paymentAtDoorHeadline')}
        </p>
        {rangeLabel && <p className="text-sm font-medium">{rangeLabel}</p>}
        <p className="text-sm opacity-90">
          {isDonation ? t('paymentDonationBody') : t('paymentAtDoorBody')}
        </p>
      </div>
    );
  }
  // Beforehand
  const paypalAmount =
    totalMinor !== undefined
      ? totalMinor % 100 === 0
        ? String(Math.floor(totalMinor / 100))
        : (totalMinor / 100).toFixed(2)
      : '';
  const paypalURL =
    paypalHandle && paypalAmount
      ? `https://paypal.me/${encodeURIComponent(paypalHandle)}/${paypalAmount}${currency}`
      : null;

  const hasBank = !!bankIBAN;
  const hasPaypal = !!paypalHandle;

  return (
    <div className="rounded border border-neutral-300 dark:border-neutral-700 p-4 flex flex-col gap-3 text-sm">
      <p className="font-semibold">{t('paymentBeforehandHeadline')}</p>
      <p className="text-neutral-600 dark:text-neutral-400">{t('paymentBeforehandBody')}</p>

      {hasBank && (
        <div className="flex flex-col gap-1">
          <p className="text-xs font-medium text-neutral-500 uppercase tracking-wide">{t('paymentBeforehandBankSection')}</p>
          <ul className="text-xs text-neutral-600 dark:text-neutral-400 list-none">
            {bankHolder && (
              <li>
                <span className="text-neutral-500">{t('paymentBeforehandBankRecipient')}:</span> {bankHolder}
              </li>
            )}
            <li>
              <span className="text-neutral-500">{t('paymentBeforehandIBAN')}:</span> <span className="font-mono">{bankIBAN}</span>
            </li>
            {bankBIC && (
              <li>
                <span className="text-neutral-500">{t('paymentBeforehandBIC')}:</span> <span className="font-mono">{bankBIC}</span>
              </li>
            )}
          </ul>
        </div>
      )}

      {hasPaypal && (
        <div className="flex flex-col gap-1">
          <p className="text-xs font-medium text-neutral-500 uppercase tracking-wide">
            {hasBank ? t('paymentBeforehandPaypalOr') : t('paymentBeforehandPaypalSection')}
          </p>
          {paypalURL ? (
            <a
              href={paypalURL}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 px-3 py-2 rounded bg-[#003087] text-white text-sm font-medium hover:opacity-90 self-start"
            >
              {t('paymentBeforehandPaypalBtn')} → {paypalAmount} {currency}
            </a>
          ) : (
            <p className="text-xs text-neutral-500 italic">
              {t('paymentBeforehandPaypalHint')}
            </p>
          )}
          <p className="text-xs text-neutral-500">
            {t('paymentBeforehandPaypalTrailing')}
          </p>
        </div>
      )}
    </div>
  );
}

function ParticipantRow({
  index,
  participant,
  isDonation,
  categories,
  durations,
  onChange,
  onRemove,
}: {
  index: number;
  participant: Participant;
  isDonation: boolean;
  categories: Category[];
  durations: Duration[];
  onChange: (patch: Partial<Participant>) => void;
  onRemove?: () => void;
}) {
  const t = useTranslations('BookingForm');
  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded p-3 flex flex-col gap-2">
      <div className="flex justify-between items-center">
        <span className="text-xs text-neutral-500">{t('participantLabel', {n: index + 1})}</span>
        {onRemove && (
          <button type="button" onClick={onRemove} className="text-xs text-red-600 hover:underline">
            {t('remove')}
          </button>
        )}
      </div>
      <input
        required
        placeholder={t('participantName')}
        value={participant.name}
        onChange={(e) => onChange({name: e.target.value})}
        className={inputCls}
      />
      <input
        type="email"
        placeholder={t('participantEmail')}
        value={participant.email}
        onChange={(e) => onChange({email: e.target.value})}
        className={inputCls}
      />
      {isDonation ? null : (
        <div className="grid grid-cols-2 gap-2">
          <select
            value={participant.category_id ?? ''}
            onChange={(e) => onChange({category_id: e.target.value})}
            className={inputCls}
            required
          >
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          {durations.length > 0 && (
            <select
              value={participant.duration_id ?? ''}
              onChange={(e) => onChange({duration_id: e.target.value})}
              className={inputCls}
              required
            >
              {durations.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          )}
        </div>
      )}
    </div>
  );
}

function participantToInput(p: Participant) {
  return {
    name: p.name,
    email: p.email,
    category_id: p.category_id,
    duration_id: p.duration_id,
    donation_amount_minor: p.donation_amount_minor,
  };
}

const inputCls =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-2 bg-transparent text-sm w-full';

const btnSecondary =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-1.5 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800';
