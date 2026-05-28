'use client';

import {useState} from 'react';
import {APIError} from '@/lib/api';
import type {EventDTO} from '@/lib/types';

export interface EventFormValues {
  slug: string;
  name: string;
  description: string;
  color_primary: string;
  color_secondary: string;
  color_text: string;
  location: string;
  starts_at: string; // RFC3339 (UTC)
  ends_at: string; // RFC3339 (UTC)
  participant_limit: number | null;
  pricing_mode: 'matrix' | 'donation';
  currency: string;
  default_locale: 'de' | 'en';
  payment_timing: 'beforehand' | 'at_door';
  bank_iban: string;
  bank_bic: string;
  bank_account_holder: string;
  paypal_handle: string;
  payment_test_mode: boolean;
}

const defaults: EventFormValues = {
  slug: '',
  name: '',
  description: '',
  color_primary: '#5E576A',
  color_secondary: '#F5F1EE',
  color_text: '#1A1A1A',
  location: '',
  starts_at: '',
  ends_at: '',
  participant_limit: null,
  pricing_mode: 'matrix',
  currency: 'EUR',
  default_locale: 'de',
  payment_timing: 'beforehand',
  bank_iban: '',
  bank_bic: '',
  bank_account_holder: '',
  paypal_handle: '',
  payment_test_mode: false,
};

export function eventFormDefaults(): EventFormValues {
  return {...defaults};
}

export function eventFormFromDTO(e: EventDTO): EventFormValues {
  return {
    slug: e.slug,
    name: e.name,
    description: e.description,
    color_primary: e.color_primary,
    color_secondary: e.color_secondary,
    color_text: e.color_text,
    location: e.location,
    starts_at: e.starts_at,
    ends_at: e.ends_at,
    participant_limit: e.participant_limit,
    pricing_mode: e.pricing_mode,
    currency: e.currency,
    default_locale: e.default_locale,
    payment_timing: e.payment_timing,
    bank_iban: e.bank_iban ?? '',
    bank_bic: e.bank_bic ?? '',
    bank_account_holder: e.bank_account_holder ?? '',
    paypal_handle: e.paypal_handle ?? '',
    payment_test_mode: e.payment_test_mode,
  };
}

// Convert a datetime-local input value (no tz) to an RFC3339 UTC string.
function localToISO(local: string): string {
  if (!local) return '';
  const d = new Date(local);
  return d.toISOString();
}

// Convert an RFC3339 string to a datetime-local input value.
function isoToLocal(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

interface Props {
  mode: 'create' | 'edit';
  initial: EventFormValues;
  onSubmit: (values: EventFormValues) => Promise<void>;
}

export function EventForm({mode, initial, onSubmit}: Props) {
  const [values, setValues] = useState<EventFormValues>(initial);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function update<K extends keyof EventFormValues>(key: K, val: EventFormValues[K]) {
    setValues((v) => ({...v, [key]: val}));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit(values);
    } catch (err) {
      if (err instanceof APIError) {
        setError(err.code);
      } else {
        setError(String(err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5 max-w-2xl">
      <Field label="Name">
        <input
          required
          value={values.name}
          onChange={(e) => {
            update('name', e.target.value);
            if (mode === 'create' && !values.slug) {
              update('slug', slugify(e.target.value));
            }
          }}
          className={inputClass}
        />
      </Field>

      <Field label="Slug" hint="URL-Pfad (a-z, 0-9, Bindestriche). Wird beim Erstellen aus dem Namen abgeleitet.">
        <input
          required
          disabled={mode === 'edit'}
          value={values.slug}
          onChange={(e) => update('slug', e.target.value)}
          className={inputClass + ' disabled:opacity-50'}
        />
      </Field>

      <Field label="Beschreibung">
        <textarea
          rows={4}
          value={values.description}
          onChange={(e) => update('description', e.target.value)}
          className={inputClass}
        />
      </Field>

      <Field label="Ort">
        <input
          value={values.location}
          onChange={(e) => update('location', e.target.value)}
          className={inputClass}
        />
      </Field>

      <div className="grid grid-cols-2 gap-4">
        <Field label="Beginn">
          <input
            required
            type="datetime-local"
            value={isoToLocal(values.starts_at)}
            onChange={(e) => update('starts_at', localToISO(e.target.value))}
            className={inputClass}
          />
        </Field>
        <Field label="Ende">
          <input
            required
            type="datetime-local"
            value={isoToLocal(values.ends_at)}
            onChange={(e) => update('ends_at', localToISO(e.target.value))}
            className={inputClass}
          />
        </Field>
      </div>

      <Field
        label="Teilnehmerlimit"
        hint="Leer lassen für unbegrenzt. Ein Limit aktiviert die Warteliste, sobald es erreicht ist; ohne Limit gibt es nie eine Warteliste."
      >
        <input
          type="number"
          min={1}
          value={values.participant_limit ?? ''}
          onChange={(e) => update('participant_limit', e.target.value === '' ? null : Number(e.target.value))}
          className={inputClass}
        />
      </Field>

      <Field label="Preismodell">
        <div className="flex gap-4 text-sm">
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="pricing_mode"
              checked={values.pricing_mode === 'matrix'}
              onChange={() => update('pricing_mode', 'matrix')}
            />
            Matrix (Phasen × Kategorien)
          </label>
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="pricing_mode"
              checked={values.pricing_mode === 'donation'}
              onChange={() => update('pricing_mode', 'donation')}
            />
            Spende
          </label>
        </div>
      </Field>

      <div className="grid grid-cols-2 gap-4">
        <Field label="Währung">
          <input
            required
            maxLength={3}
            value={values.currency}
            onChange={(e) => update('currency', e.target.value.toUpperCase())}
            className={inputClass}
          />
        </Field>
        <Field label="Standard-Sprache">
          <select
            value={values.default_locale}
            onChange={(e) => update('default_locale', e.target.value as 'de' | 'en')}
            className={inputClass}
          >
            <option value="de">Deutsch</option>
            <option value="en">English</option>
          </select>
        </Field>
      </div>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-medium">Zahlung</legend>
        <div className="flex flex-col gap-2 text-sm">
          <label className="flex items-start gap-2">
            <input
              type="radio"
              name="payment_timing"
              checked={values.payment_timing === 'beforehand'}
              onChange={() => update('payment_timing', 'beforehand')}
              className="mt-1"
            />
            <span>
              <strong>Im Voraus</strong> — Ticket wird gültig, sobald die Zahlung bestätigt ist (per
              Überweisung). Der QR-Code geht in einer zweiten E-Mail nach Zahlungseingang raus.
            </span>
          </label>
          <label className="flex items-start gap-2">
            <input
              type="radio"
              name="payment_timing"
              checked={values.payment_timing === 'at_door'}
              onChange={() => update('payment_timing', 'at_door')}
              className="mt-1"
            />
            <span>
              <strong>Vor Ort</strong> — Ticket ist sofort gültig, Zahlung erfolgt beim Event. QR-Code
              kommt sofort per E-Mail.
            </span>
          </label>
        </div>
      </fieldset>

      {values.payment_timing === 'beforehand' && (
        <>
          <fieldset className="flex flex-col gap-3">
            <legend className="text-sm font-medium">Bankverbindung (für Überweisungen)</legend>
            <input
              placeholder="Kontoinhaber"
              value={values.bank_account_holder}
              onChange={(e) => update('bank_account_holder', e.target.value)}
              className={inputClass}
            />
            <input
              placeholder="IBAN"
              value={values.bank_iban}
              onChange={(e) => update('bank_iban', e.target.value)}
              className={inputClass}
            />
            <input
              placeholder="BIC (optional)"
              value={values.bank_bic}
              onChange={(e) => update('bank_bic', e.target.value)}
              className={inputClass}
            />
          </fieldset>

          <fieldset className="flex flex-col gap-2">
            <legend className="text-sm font-medium">PayPal (optional)</legend>
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-neutral-500">PayPal.me-Benutzername</span>
              <input
                placeholder="z. B. daodance"
                value={values.paypal_handle}
                onChange={(e) => update('paypal_handle', e.target.value.replace(/^@|\s+/g, ''))}
                className={inputClass}
              />
              <span className="text-xs text-neutral-500">
                Erzeugt einen Bezahllink der Form{' '}
                <span className="font-mono">paypal.me/{values.paypal_handle || '<benutzername>'}/&lt;Betrag&gt;EUR</span>.
                Der Buchende zahlt direkt — du bestätigst die Buchung manuell im Dashboard, sobald das Geld da ist.
              </span>
            </label>
          </fieldset>
        </>
      )}

      <fieldset
        className={
          'flex flex-col gap-2 rounded p-3 ' +
          (values.payment_test_mode
            ? 'border border-amber-400 bg-amber-50 dark:bg-amber-950/40'
            : 'border border-neutral-300 dark:border-neutral-700')
        }
      >
        <legend className="text-sm font-medium px-1">Test-Modus</legend>
        <label className="flex items-start gap-2 text-sm">
          <input
            type="checkbox"
            checked={values.payment_test_mode}
            onChange={(e) => update('payment_test_mode', e.target.checked)}
            className="mt-1"
          />
          <span>
            <strong>Buchungen werden sofort als bezahlt markiert</strong>, ohne dass eine echte Zahlung
            ausgelöst wird. Nutze das, um die gesamte Pipeline (E-Mail, QR, Ticket-Seite) zu testen.{' '}
            <strong className="text-amber-700 dark:text-amber-400">
              Vor dem Live-Gang abschalten und Test-Buchungen löschen.
            </strong>
          </span>
        </label>
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-medium">Farbschema</legend>
        <div className="grid grid-cols-3 gap-4">
          <ColorField
            label="Primär"
            value={values.color_primary}
            onChange={(v) => update('color_primary', v)}
          />
          <ColorField
            label="Sekundär"
            value={values.color_secondary}
            onChange={(v) => update('color_secondary', v)}
          />
          <ColorField label="Text" value={values.color_text} onChange={(v) => update('color_text', v)} />
        </div>
      </fieldset>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <div>
        <button
          type="submit"
          disabled={submitting}
          className="bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 rounded px-4 py-2 text-sm disabled:opacity-50"
        >
          {submitting ? 'Speichern…' : mode === 'create' ? 'Event erstellen' : 'Speichern'}
        </button>
      </div>
    </form>
  );
}

const inputClass =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-2 bg-transparent w-full text-sm';

function Field({label, hint, children}: {label: string; hint?: string; children: React.ReactNode}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="font-medium">{label}</span>
      {children}
      {hint && <span className="text-xs text-neutral-500">{hint}</span>}
    </label>
  );
}

function ColorField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span>{label}</span>
      <div className="flex gap-2 items-center">
        <input
          type="color"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="h-10 w-12 border border-neutral-300 dark:border-neutral-700 rounded"
        />
        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={inputClass}
          pattern="^#[0-9a-fA-F]{6}$"
        />
      </div>
    </label>
  );
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}
