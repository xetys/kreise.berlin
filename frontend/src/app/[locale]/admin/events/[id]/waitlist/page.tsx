'use client';

import {use, useCallback, useEffect, useState} from 'react';
import {usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

interface WaitlistRow {
  id: string;
  contact_name: string;
  contact_email: string;
  locale: string;
  requested_seats: number;
  status: 'waiting' | 'promoted' | 'fulfilled' | 'expired' | 'removed';
  created_at: string;
  promoted_at?: string;
  claim_deadline?: string;
  fulfilled_booking_id?: string;
}

interface ListResponse {
  waitlist: WaitlistRow[];
  counts: {
    waiting: number;
    promoted: number;
    fulfilled: number;
    expired: number;
    removed: number;
  };
}

interface EventResp {
  pricing_mode: string;
  participant_limit: number | null;
}

function fmt(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'});
}

// Live countdown to a claim_deadline. Format: "23h 14m left" / "abgelaufen".
function countdown(iso?: string): string {
  if (!iso) return '';
  const diff = new Date(iso).getTime() - Date.now();
  if (diff <= 0) return 'abgelaufen';
  const h = Math.floor(diff / 3600000);
  const m = Math.floor((diff % 3600000) / 60000);
  return h > 0 ? `noch ${h}h ${m}m` : `noch ${m}m`;
}

export default function AdminWaitlistPage({params}: PageProps) {
  const {id} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';

  const [data, setData] = useState<ListResponse | null>(null);
  const [event, setEvent] = useState<EventResp | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  // tick re-render so claim-deadline countdowns update every 30s without
  // refetching from the server.
  const [, setTick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setTick((n) => n + 1), 30_000);
    return () => clearInterval(t);
  }, []);

  const load = useCallback(async () => {
    try {
      const r = await api<ListResponse>(`/api/admin/events/${id}/waitlist`);
      setData(r);
      setError(null);
    } catch (e) {
      setError(e instanceof APIError ? e.developerMessage : String(e));
    }
  }, [id]);

  useEffect(() => {
    api<EventResp>(`/api/admin/events/${id}`).then(setEvent).catch(() => {});
    load();
  }, [id, load]);

  async function rotateNow() {
    setBusy('rotate');
    setMsg(null);
    try {
      await api(`/api/admin/events/${id}/waitlist/promote`, {method: 'POST'});
      setMsg('Warteliste durchlaufen.');
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function promote(wid: string) {
    if (!confirm('Diesen Eintrag jetzt manuell promoten?')) return;
    setBusy(wid);
    setMsg(null);
    try {
      const r = await api<{outcome: string; booking_reference?: string}>(
        `/api/admin/waitlist/${wid}/promote`,
        {method: 'POST'}
      );
      setMsg(
        r.outcome === 'fulfilled'
          ? `Promoted: Buchung ${r.booking_reference} erstellt + Stage-2 versandt.`
          : 'Promoted: Spot-Opened-E-Mail versandt, Claim-Frist läuft.'
      );
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function remove(wid: string, notify: boolean) {
    if (!confirm(notify ? 'Eintrag entfernen und Person benachrichtigen?' : 'Eintrag entfernen ohne Benachrichtigung?'))
      return;
    setBusy(wid);
    setMsg(null);
    try {
      await api(`/api/admin/waitlist/${wid}/remove`, {
        method: 'POST',
        body: {notify_user: notify},
      });
      setMsg('Eintrag entfernt.');
      await load();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  const unlimited = event?.participant_limit === null;

  return (
    <main className="flex-1 max-w-6xl mx-auto w-full px-6 py-8 flex flex-col gap-5">
      <header className="flex items-start justify-between gap-4">
        <div>
          <Link
            href={`/${localePrefix}/admin/events/${id}`}
            className="text-xs text-neutral-500 hover:underline"
          >
            ← Veranstaltung
          </Link>
          <h1 className="text-2xl font-light tracking-wide mt-1">Warteliste</h1>
        </div>
        {!unlimited && (
          <button
            onClick={rotateNow}
            disabled={busy === 'rotate'}
            className="text-sm rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800"
            title="FIFO-Promotion auslösen — promotet alle passenden Wartenden bis die Kapazität ausgeschöpft ist."
          >
            {busy === 'rotate' ? 'Sende…' : 'Jetzt rotieren ↻'}
          </button>
        )}
      </header>

      {unlimited && (
        <div className="rounded border border-neutral-200 dark:border-neutral-800 p-4 text-sm opacity-70">
          Diese Veranstaltung hat kein Platzlimit — es gibt keine Warteliste.
        </div>
      )}

      {!unlimited && data && (
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-3 text-sm">
          <SummaryCell label="Wartet" value={String(data.counts.waiting)} />
          <SummaryCell label="Promoted" value={String(data.counts.promoted)} />
          <SummaryCell label="Erfüllt" value={String(data.counts.fulfilled)} />
          <SummaryCell label="Abgelaufen" value={String(data.counts.expired)} />
          <SummaryCell label="Entfernt" value={String(data.counts.removed)} />
        </div>
      )}

      {msg && <p className="text-sm">{msg}</p>}
      {error && <p className="text-sm text-red-600">{error}</p>}

      {!unlimited && data && data.waitlist.length === 0 && (
        <p className="text-sm text-neutral-500 italic">Noch keine Einträge auf der Warteliste.</p>
      )}

      {!unlimited && data && data.waitlist.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left border-b border-neutral-200 dark:border-neutral-800">
              <tr>
                <th className="py-2 font-medium">Kontakt</th>
                <th className="py-2 font-medium">Plätze</th>
                <th className="py-2 font-medium">Status</th>
                <th className="py-2 font-medium">Eingetragen</th>
                <th className="py-2 font-medium">Claim-Frist</th>
                <th className="py-2 font-medium text-right">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {data.waitlist.map((w) => (
                <tr
                  key={w.id}
                  className="border-b border-neutral-100 dark:border-neutral-900 align-top"
                >
                  <td className="py-2">
                    <div>{w.contact_name}</div>
                    <div className="text-xs text-neutral-500">{w.contact_email}</div>
                  </td>
                  <td className="py-2 tabular-nums">{w.requested_seats}</td>
                  <td className="py-2">
                    <StatusPill status={w.status} />
                  </td>
                  <td className="py-2 text-xs">{fmt(w.created_at)}</td>
                  <td className="py-2 text-xs">
                    {w.claim_deadline ? (
                      <div>
                        <div>{fmt(w.claim_deadline)}</div>
                        <div className="opacity-60">{countdown(w.claim_deadline)}</div>
                      </div>
                    ) : (
                      '—'
                    )}
                  </td>
                  <td className="py-2 text-right">
                    <div className="inline-flex gap-3 text-xs">
                      {(w.status === 'waiting' || w.status === 'expired') && (
                        <button
                          onClick={() => promote(w.id)}
                          disabled={busy === w.id}
                          className="hover:underline"
                        >
                          Promoten
                        </button>
                      )}
                      {w.status !== 'fulfilled' && w.status !== 'removed' && (
                        <button
                          onClick={() => remove(w.id, true)}
                          disabled={busy === w.id}
                          className="hover:underline text-red-700 dark:text-red-300"
                          title="Entfernen + Bestätigungsmail an die Person"
                        >
                          Entfernen
                        </button>
                      )}
                      {w.status === 'fulfilled' && w.fulfilled_booking_id && (
                        <Link
                          href={`/${localePrefix}/admin/events/${id}/bookings?q=${w.fulfilled_booking_id.slice(0, 8).toUpperCase()}`}
                          className="hover:underline"
                        >
                          Buchung →
                        </Link>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}

function SummaryCell({label, value}: {label: string; value: string}) {
  return (
    <div className="rounded border border-neutral-200 dark:border-neutral-800 px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider opacity-60">{label}</div>
      <div className="text-base font-medium tabular-nums">{value}</div>
    </div>
  );
}

function StatusPill({status}: {status: WaitlistRow['status']}) {
  const map: Record<WaitlistRow['status'], {label: string; cls: string}> = {
    waiting: {label: 'wartet', cls: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200'},
    promoted: {label: 'promoted', cls: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200'},
    fulfilled: {label: 'erfüllt', cls: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200'},
    expired: {label: 'abgelaufen', cls: 'bg-neutral-200 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-400'},
    removed: {label: 'entfernt', cls: 'bg-neutral-200 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-400'},
  };
  const s = map[status];
  return <span className={`text-xs px-2 py-0.5 rounded ${s.cls}`}>{s.label}</span>;
}
