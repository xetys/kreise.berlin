'use client';

import {use, useCallback, useEffect, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

interface BookingRow {
  id: string;
  contact_email: string;
  contact_name: string;
  status: string;
  total_amount_minor: number;
  currency: string;
  payment_method?: string;
  participant_count: number;
  created_at: string;
  paid_at?: string;
  reservation_expires_at?: string;
  reference: string;
}

interface ListResponse {
  bookings: BookingRow[];
  total: number;
  limit: number;
  offset: number;
  summary: {
    total_count: number;
    paid_count: number;
    booked_count: number;
    canceled_count: number;
    paid_revenue_minor: number;
    total_participants: number;
    paid_participants: number;
    booked_participants: number;
  };
}

const PAGE_SIZE = 50;

function formatMoney(minor: number, currency: string): string {
  const whole = Math.floor(minor / 100);
  const cents = Math.abs(minor % 100);
  return `${whole},${String(cents).padStart(2, '0')} ${currency}`;
}

function formatDate(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'});
}

const STATUSES = [
  {value: '', label: 'alle'},
  {value: 'booked', label: 'gebucht'},
  {value: 'paid', label: 'bezahlt'},
  {value: 'canceled', label: 'storniert'},
];

const PAYMENT_METHODS = [
  {value: '', label: 'alle'},
  {value: 'bank_transfer', label: 'Überweisung'},
  {value: 'paypal', label: 'PayPal'},
  {value: 'donation', label: 'Spende'},
  {value: 'at_door', label: 'Vor Ort'},
  {value: 'test', label: 'Test'},
];

export default function AdminBookingsPage({params}: PageProps) {
  const {id} = use(params);
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const search = useSearchParams();

  // URL-synced filter state. Every filter writes back via router.replace so
  // refresh + back-button restore the exact view.
  const q = search.get('q') ?? '';
  const status = search.get('status') ?? '';
  const paymentMethod = search.get('payment_method') ?? '';
  const sort = search.get('sort') ?? 'created_at_desc';
  const page = Math.max(1, Number(search.get('page') ?? '1'));
  const offset = (page - 1) * PAGE_SIZE;

  const [data, setData] = useState<ListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  // Drives whether the "Umsatz (bezahlt)" summary cell is shown. For donation
  // events the booking total is the *suggested* contribution — what people
  // actually pay at the door is off-platform — so summing those would
  // misrepresent reality. Hide the cell until we have a real per-booking
  // donation-paid number to display.
  const [pricingMode, setPricingMode] = useState<string | null>(null);

  useEffect(() => {
    api<{pricing_mode: string}>(`/api/admin/events/${id}`)
      .then((e) => setPricingMode(e.pricing_mode))
      .catch(() => {/* non-fatal — just leaves the cell hidden */});
  }, [id]);

  const load = useCallback(async () => {
    try {
      const qs = new URLSearchParams();
      if (q) qs.set('q', q);
      if (status) qs.set('status', status);
      if (paymentMethod) qs.set('payment_method', paymentMethod);
      if (sort) qs.set('sort', sort);
      qs.set('limit', String(PAGE_SIZE));
      qs.set('offset', String(offset));
      const r = await api<ListResponse>(`/api/admin/events/${id}/bookings?${qs.toString()}`);
      setData(r);
      setError(null);
      setSelected(new Set());
    } catch (e) {
      setError(e instanceof APIError ? e.developerMessage : String(e));
    }
  }, [id, q, status, paymentMethod, sort, offset]);

  useEffect(() => {
    load();
  }, [load]);

  // Keyboard ←/→ flip pages while not focused inside an input.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.target as HTMLElement)?.tagName?.toLowerCase() === 'input') return;
      if ((e.target as HTMLElement)?.tagName?.toLowerCase() === 'textarea') return;
      if (e.key === 'ArrowLeft' && page > 1) goPage(page - 1);
      else if (e.key === 'ArrowRight' && data && page * PAGE_SIZE < data.total) goPage(page + 1);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  function applyFilter(patch: Record<string, string>) {
    const next = new URLSearchParams(search.toString());
    for (const [k, v] of Object.entries(patch)) {
      if (v === '' || v === undefined) next.delete(k);
      else next.set(k, v);
    }
    // Reset page on filter change unless we're explicitly setting page.
    if (!('page' in patch)) next.delete('page');
    router.replace(`?${next.toString()}`);
  }

  function goPage(p: number) {
    applyFilter({page: String(p)});
  }

  function resetFilters() {
    router.replace(pathname);
  }

  function toggleSelect(id: string) {
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }

  function toggleSelectAll() {
    if (!data) return;
    if (selected.size === data.bookings.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(data.bookings.map((b) => b.id)));
    }
  }

  async function rowAction(bookingId: string, action: 'mark-paid' | 'refund' | 'resend-confirmation') {
    if (action === 'refund' && !confirm('Buchung wirklich stornieren?')) return;
    setBusy(bookingId);
    setMsg(null);
    try {
      await api(`/api/admin/bookings/${bookingId}/${action}`, {method: 'POST'});
      setMsg('OK');
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function editEmail(bookingId: string, current: string) {
    const next = prompt('Neue E-Mail-Adresse:', current);
    if (!next || next === current) return;
    setBusy(bookingId);
    setMsg(null);
    try {
      await api(`/api/admin/bookings/${bookingId}`, {
        method: 'PATCH',
        body: {contact_email: next.trim().toLowerCase()},
      });
      setMsg('E-Mail aktualisiert.');
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function bulkMarkPaid() {
    if (selected.size === 0) return;
    if (!confirm(`Wirklich ${selected.size} Buchung${selected.size === 1 ? '' : 'en'} als bezahlt markieren?`)) return;
    setBusy('bulk');
    setMsg(null);
    try {
      const r = await api<{succeeded: string[]; failed: {id: string; reason: string}[]}>(
        `/api/admin/bookings/bulk-mark-paid`,
        {method: 'POST', body: {event_id: id, booking_ids: Array.from(selected)}}
      );
      setMsg(`${r.succeeded.length} bestätigt, ${r.failed.length} fehlgeschlagen.`);
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  function csvHref(): string {
    const qs = new URLSearchParams();
    if (q) qs.set('q', q);
    if (status) qs.set('status', status);
    if (paymentMethod) qs.set('payment_method', paymentMethod);
    return `/api/admin/events/${id}/bookings/export.csv?${qs.toString()}`;
  }

  return (
    <main className="flex-1 max-w-6xl mx-auto w-full px-6 py-8 flex flex-col gap-5">
      <header className="flex items-center justify-between gap-4">
        <div>
          <Link
            href={`/${localePrefix}/admin/events/${id}`}
            className="text-xs text-neutral-500 hover:underline"
          >
            ← Veranstaltung
          </Link>
          <h1 className="text-2xl font-light tracking-wide mt-1">Buchungen</h1>
        </div>
        <a
          href={csvHref()}
          download
          className="text-sm rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800"
        >
          CSV exportieren ↓
        </a>
      </header>

      {/* Summary strip — reflects the current filter.
          Top row: booking counts + revenue. "Eingenommen" is the REAL
          money received (pre-paid bookings via bank/PayPal + door cash
          recorded at check-in). Pre-real-revenue we hid this for donation
          events because summing the suggested amounts was misleading;
          now that door check-ins record the actual cash, the cell shows
          for all event types. */}
      {data && (
        <div className="grid gap-3 text-sm">
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
            <SummaryCell label="Buchungen gesamt" value={String(data.summary.total_count)} />
            <SummaryCell label="Gebucht" value={String(data.summary.booked_count)} />
            <SummaryCell label="Bezahlt" value={String(data.summary.paid_count)} />
            <SummaryCell label="Storniert" value={String(data.summary.canceled_count)} />
            <SummaryCell
              label="Eingenommen"
              value={formatMoney(data.summary.paid_revenue_minor, data.bookings[0]?.currency ?? 'EUR')}
              hint={
                pricingMode === 'donation'
                  ? 'Spenden an der Tür + Vorkasse'
                  : 'Vorkasse + Vor-Ort-Kasse'
              }
            />
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            <SummaryCell
              label="Personen — erwartet"
              value={String(data.summary.paid_participants + data.summary.booked_participants)}
              hint="bezahlt + reserviert"
            />
            <SummaryCell label="Personen — bezahlt" value={String(data.summary.paid_participants)} />
            <SummaryCell label="Personen — reserviert" value={String(data.summary.booked_participants)} />
          </div>
        </div>
      )}

      {/* Filter bar */}
      <div className="grid grid-cols-1 sm:grid-cols-5 gap-2 items-end">
        <label className="flex flex-col gap-1 sm:col-span-2">
          <span className="text-xs uppercase tracking-wide opacity-70">Suche</span>
          <input
            type="search"
            placeholder="Name, E-Mail oder Referenz"
            defaultValue={q}
            onKeyDown={(e) => {
              if (e.key === 'Enter') applyFilter({q: (e.target as HTMLInputElement).value});
            }}
            onBlur={(e) => applyFilter({q: e.target.value})}
            className={inputCls}
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs uppercase tracking-wide opacity-70">Status</span>
          <select
            value={status}
            onChange={(e) => applyFilter({status: e.target.value})}
            className={inputCls}
          >
            {STATUSES.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs uppercase tracking-wide opacity-70">Zahlung</span>
          <select
            value={paymentMethod}
            onChange={(e) => applyFilter({payment_method: e.target.value})}
            className={inputCls}
          >
            {PAYMENT_METHODS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs uppercase tracking-wide opacity-70">Sortierung</span>
          <select
            value={sort}
            onChange={(e) => applyFilter({sort: e.target.value})}
            className={inputCls}
          >
            <option value="created_at_desc">neueste zuerst</option>
            <option value="created_at_asc">älteste zuerst</option>
            <option value="paid_at_desc">bezahlt: neueste zuerst</option>
            <option value="total_desc">Betrag: hoch → niedrig</option>
            <option value="total_asc">Betrag: niedrig → hoch</option>
            <option value="name_asc">Name A→Z</option>
          </select>
        </label>
      </div>

      {(q || status || paymentMethod) && (
        <button onClick={resetFilters} className="text-xs text-neutral-500 hover:underline self-start">
          Filter zurücksetzen
        </button>
      )}

      {msg && <p className="text-sm">{msg}</p>}
      {error && <p className="text-sm text-red-600">{error}</p>}

      {/* Bulk action toolbar — only when there's a selection */}
      {selected.size > 0 && (
        <div className="flex items-center justify-between bg-neutral-100 dark:bg-neutral-900 rounded px-4 py-2 text-sm">
          <span>
            {selected.size} ausgewählt
          </span>
          <button
            onClick={bulkMarkPaid}
            disabled={busy === 'bulk'}
            className="rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1 text-xs hover:bg-white dark:hover:bg-neutral-800"
          >
            {busy === 'bulk' ? 'Sende…' : 'Als bezahlt markieren'}
          </button>
        </div>
      )}

      {/* Table */}
      {data && data.bookings.length === 0 && (
        <p className="text-sm text-neutral-500 italic">
          Keine Buchungen mit diesen Filtern.{' '}
          <button onClick={resetFilters} className="underline">
            zurücksetzen
          </button>
        </p>
      )}
      {data && data.bookings.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left border-b border-neutral-200 dark:border-neutral-800">
              <tr>
                <th className="py-2 w-8">
                  <input
                    type="checkbox"
                    checked={selected.size === data.bookings.length}
                    onChange={toggleSelectAll}
                  />
                </th>
                <th className="py-2 font-medium">Ref</th>
                <th className="py-2 font-medium">Kontakt</th>
                <th className="py-2 font-medium">Status</th>
                <th className="py-2 font-medium">Pers.</th>
                <th className="py-2 font-medium">Betrag</th>
                <th className="py-2 font-medium">Zahlung</th>
                <th className="py-2 font-medium">Erstellt</th>
                <th className="py-2 font-medium text-right">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {data.bookings.map((b) => (
                <tr
                  key={b.id}
                  className="border-b border-neutral-100 dark:border-neutral-900 align-top"
                >
                  <td className="py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(b.id)}
                      onChange={() => toggleSelect(b.id)}
                    />
                  </td>
                  <td className="py-2 font-mono text-xs">{b.reference}</td>
                  <td className="py-2">
                    <div>{b.contact_name}</div>
                    <div className="text-xs text-neutral-500">{b.contact_email}</div>
                  </td>
                  <td className="py-2">
                    <StatusPill status={b.status} paymentMethod={b.payment_method} />
                  </td>
                  <td className="py-2 tabular-nums">{b.participant_count}</td>
                  <td className="py-2 tabular-nums">{formatMoney(b.total_amount_minor, b.currency)}</td>
                  <td className="py-2 text-xs">{b.payment_method ?? '—'}</td>
                  <td className="py-2 text-xs">{formatDate(b.created_at)}</td>
                  <td className="py-2 text-right">
                    <div className="inline-flex flex-wrap justify-end gap-2 text-xs">
                      {b.status === 'booked' && (
                        <button
                          onClick={() => rowAction(b.id, 'mark-paid')}
                          disabled={busy === b.id}
                          className="hover:underline"
                        >
                          Als bezahlt
                        </button>
                      )}
                      {b.status !== 'canceled' && (
                        <button
                          onClick={() => rowAction(b.id, 'resend-confirmation')}
                          disabled={busy === b.id}
                          className="hover:underline"
                          title="Bestätigungs-E-Mail erneut senden"
                        >
                          Resend
                        </button>
                      )}
                      <button
                        onClick={() => editEmail(b.id, b.contact_email)}
                        disabled={busy === b.id}
                        className="hover:underline"
                        title="Kontakt-E-Mail bearbeiten"
                      >
                        E-Mail
                      </button>
                      {b.status !== 'canceled' && (
                        <button
                          onClick={() => rowAction(b.id, 'refund')}
                          disabled={busy === b.id}
                          className="hover:underline text-red-700 dark:text-red-300"
                        >
                          Stornieren
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      {data && data.total > PAGE_SIZE && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-neutral-500">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, data.total)} von {data.total}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => goPage(page - 1)}
              disabled={page <= 1}
              className="rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1 disabled:opacity-30"
            >
              ← vorherige
            </button>
            <button
              onClick={() => goPage(page + 1)}
              disabled={page * PAGE_SIZE >= data.total}
              className="rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1 disabled:opacity-30"
            >
              nächste →
            </button>
          </div>
        </div>
      )}
    </main>
  );
}

function SummaryCell({label, value, hint}: {label: string; value: string; hint?: string}) {
  return (
    <div className="rounded border border-neutral-200 dark:border-neutral-800 px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider opacity-60">{label}</div>
      <div className="text-base font-medium tabular-nums">{value}</div>
      {hint && <div className="text-[10px] opacity-50 mt-0.5">{hint}</div>}
    </div>
  );
}

function StatusPill({status, paymentMethod}: {status: string; paymentMethod?: string}) {
  const isTest = paymentMethod === 'test';
  const map: Record<string, string> = {
    booked: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200',
    paid: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200',
    canceled: 'bg-neutral-200 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-400',
  };
  const cls = map[status] ?? 'bg-neutral-200 text-neutral-700';
  return (
    <span className="inline-flex items-center gap-1">
      <span className={`text-xs px-2 py-0.5 rounded ${cls}`}>{status}</span>
      {isTest && (
        <span className="text-xs px-1.5 py-0.5 rounded bg-amber-200 text-amber-900">TEST</span>
      )}
    </span>
  );
}

const inputCls =
  'rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm w-full';
