'use client';

import {use, useEffect, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';
import type {EventDTO} from '@/lib/types';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('de-DE', {dateStyle: 'medium', timeStyle: 'short'});
}

function statusLabel(e: EventDTO): {text: string; tone: 'draft' | 'public' | 'archived'} {
  if (e.is_archived) return {text: 'archiviert', tone: 'archived'};
  if (e.is_public) return {text: 'veröffentlicht', tone: 'public'};
  return {text: 'Entwurf', tone: 'draft'};
}

export default function EventDetailPage({params}: PageProps) {
  const {locale, id} = use(params);
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? locale;

  const [event, setEvent] = useState<EventDTO | null>(null);
  const [error, setError] = useState<{kind: 'forbidden' | 'not_found' | 'other'; detail?: string} | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);

  useEffect(() => {
    api<EventDTO>(`/api/admin/events/${id}`)
      .then(setEvent)
      .catch((e) => {
        if (e instanceof APIError && e.status === 403) {
          setError({kind: 'forbidden'});
        } else if (e instanceof APIError && e.status === 404) {
          setError({kind: 'not_found'});
        } else {
          setError({kind: 'other', detail: e instanceof APIError ? e.developerMessage : String(e)});
        }
      });
  }, [id]);

  async function refresh() {
    const fresh = await api<EventDTO>(`/api/admin/events/${id}`);
    setEvent(fresh);
  }

  async function lifecycle(action: 'publish' | 'unpublish' | 'archive') {
    setActionMsg(null);
    try {
      await api(`/api/admin/events/${id}/${action}`, {method: 'POST'});
      await refresh();
      const labels = {publish: 'Veröffentlicht.', unpublish: 'Zurückgezogen.', archive: 'Archiviert.'};
      setActionMsg(labels[action]);
    } catch (e) {
      setActionMsg(e instanceof APIError ? e.code : String(e));
    }
  }

  if (error) {
    if (error.kind === 'forbidden') {
      return (
        <div className="max-w-xl mx-auto py-12 flex flex-col gap-3">
          <h1 className="text-xl font-semibold">Kein Zugriff auf diese Veranstaltung</h1>
          <p className="text-sm text-neutral-600 dark:text-neutral-400">
            Dein Account ist (noch) nicht dem Team dieser Veranstaltung zugewiesen. Bitte einen Event-
            oder globalen Admin, dich in der <span className="font-medium">Team</span>-Sektion der
            Veranstaltung hinzuzufügen — als Event-Manager (Buchungen + Einlass) oder als Event-Admin
            (zusätzlich Bearbeiten + Veröffentlichen).
          </p>
          <button
            onClick={() => router.push(`/${localePrefix}/admin/events`)}
            className="text-sm self-start underline text-neutral-500"
          >
            ← zurück zur Übersicht
          </button>
        </div>
      );
    }
    if (error.kind === 'not_found') {
      return <p className="text-sm text-neutral-500">Veranstaltung nicht gefunden.</p>;
    }
    return <p className="text-sm text-red-600">{error.detail}</p>;
  }
  if (!event) return <p className="text-sm text-neutral-500">Lädt…</p>;

  const status = statusLabel(event);
  const statusClass =
    status.tone === 'public'
      ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
      : status.tone === 'archived'
        ? 'bg-neutral-200 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300'
        : 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200';

  return (
    <div className="flex flex-col gap-6">
      {/* Header: title + status pill + Edit + back */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <button
            onClick={() => router.push(`/${localePrefix}/admin/events`)}
            className="text-xs text-neutral-500 hover:underline self-start"
          >
            ← Übersicht
          </button>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold">{event.name}</h1>
            <span className={`text-xs px-2 py-0.5 rounded ${statusClass}`}>{status.text}</span>
          </div>
          <p className="text-sm text-neutral-500">/{event.slug}</p>
        </div>

        <div className="flex gap-2 text-sm">
          <Link
            href={`/${localePrefix}/admin/events/${id}/preview`}
            target="_blank"
            className={btnSecondary}
            title="Öffentliche Vorschau in neuem Tab öffnen — funktioniert auch für unveröffentlichte Veranstaltungen"
          >
            Öffentliche Vorschau ↗
          </Link>
          <Link href={`/${localePrefix}/admin/events/${id}/edit`} className={btnPrimary}>
            Bearbeiten
          </Link>
        </div>
      </div>

      {actionMsg && <p className="text-sm text-emerald-700">{actionMsg}</p>}

      {/* Lifecycle actions */}
      <div className="flex gap-2 text-sm">
        {!event.is_public && !event.is_archived && (
          <button onClick={() => lifecycle('publish')} className={btnSecondary}>
            Veröffentlichen
          </button>
        )}
        {event.is_public && (
          <button onClick={() => lifecycle('unpublish')} className={btnSecondary}>
            Zurückziehen
          </button>
        )}
        {!event.is_archived && (
          <button onClick={() => lifecycle('archive')} className={btnSecondary}>
            Archivieren
          </button>
        )}
      </div>

      {/* Summary card */}
      <section className="grid grid-cols-1 md:grid-cols-2 gap-6 border border-neutral-200 dark:border-neutral-800 rounded p-4">
        <div className="flex flex-col gap-2 text-sm">
          <Field label="Beginn" value={formatDate(event.starts_at)} />
          <Field label="Ende" value={formatDate(event.ends_at)} />
          {event.location && <Field label="Ort" value={event.location} />}
          <Field
            label="Teilnehmerlimit"
            value={event.participant_limit !== null ? String(event.participant_limit) : 'unbegrenzt'}
          />
          <Field label="Preismodell" value={event.pricing_mode === 'donation' ? 'Spende' : 'Matrix'} />
          <Field label="Währung" value={event.currency} />
        </div>
        <div className="flex flex-col gap-2">
          {event.banner_url ? (
            <img
              src={event.banner_url + '?v=' + event.id}
              alt=""
              className="max-h-48 rounded border border-neutral-200 dark:border-neutral-800 object-contain"
            />
          ) : (
            <div className="h-32 flex items-center justify-center border border-dashed border-neutral-300 dark:border-neutral-700 rounded text-xs text-neutral-500">
              Noch kein Banner
            </div>
          )}
          {event.description && (
            <p className="text-sm text-neutral-600 dark:text-neutral-400 line-clamp-4 whitespace-pre-line">
              {event.description}
            </p>
          )}
        </div>
      </section>

      <BookingsCTA eventId={id} localePrefix={localePrefix} />

      {event.participant_limit !== null && (
        <WaitlistCTA eventId={id} localePrefix={localePrefix} />
      )}

      <CheckInCTA eventId={id} localePrefix={localePrefix} />
      <Placeholder
        title="Newsletter & Mailings"
        body="Custom-E-Mails an aktuelle und ehemalige Teilnehmer. Kommt mit Phase 9."
      />
      <TeamSection eventId={id} />
    </div>
  );
}

function Field({label, value}: {label: string; value: string}) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-neutral-500">{label}</span>
      <span className="font-medium text-right">{value}</span>
    </div>
  );
}

function Placeholder({title, body}: {title: string; body: string}) {
  return (
    <section className="border border-dashed border-neutral-200 dark:border-neutral-800 rounded p-4 flex flex-col gap-1">
      <h2 className="text-sm font-medium">{title}</h2>
      <p className="text-xs text-neutral-500">{body}</p>
    </section>
  );
}

const btnPrimary =
  'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 rounded px-4 py-2 text-sm';
const btnSecondary =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800';

interface BookingsSummary {
  total: number;
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

/**
 * Thin CTA replacing the formerly-inline bookings list. The actual list with
 * filter/search/sort/pagination/CSV/bulk actions lives at
 * /admin/events/{id}/bookings — this just fetches `limit=1` so we can show the
 * counters and a "Buchungen verwalten →" link.
 */
function BookingsCTA({eventId, localePrefix}: {eventId: string; localePrefix: string}) {
  const [data, setData] = useState<BookingsSummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<BookingsSummary>(`/api/admin/events/${eventId}/bookings?limit=1`)
      .then(setData)
      .catch((e) => setError(e instanceof APIError ? e.developerMessage : String(e)));
  }, [eventId]);

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded p-4 flex items-center justify-between gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-medium">Buchungen</h2>
        {error && <p className="text-xs text-red-600">{error}</p>}
        {!error && !data && <p className="text-xs text-neutral-500">Lädt…</p>}
        {data && (
          <p className="text-xs text-neutral-500">
            {data.summary.total_count} Buchungen · {data.summary.paid_count} bezahlt ·{' '}
            {data.summary.booked_count} offen · {data.summary.canceled_count} storniert
          </p>
        )}
        {data && (data.summary.paid_participants + data.summary.booked_participants > 0) && (
          <p className="text-xs text-neutral-700 dark:text-neutral-300">
            <span className="font-medium">
              {data.summary.paid_participants + data.summary.booked_participants}
            </span>{' '}
            Personen erwartet
            <span className="opacity-60">
              {' '}
              ({data.summary.paid_participants} bezahlt, {data.summary.booked_participants}{' '}
              reserviert)
            </span>
          </p>
        )}
      </div>
      <Link
        href={`/${localePrefix}/admin/events/${eventId}/bookings`}
        className="text-sm rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 whitespace-nowrap"
      >
        Buchungen verwalten →
      </Link>
    </section>
  );
}

interface WaitlistCounts {
  counts: {
    waiting: number;
    promoted: number;
    fulfilled: number;
    expired: number;
    removed: number;
  };
}

function WaitlistCTA({eventId, localePrefix}: {eventId: string; localePrefix: string}) {
  const [data, setData] = useState<WaitlistCounts | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<WaitlistCounts>(`/api/admin/events/${eventId}/waitlist`)
      .then(setData)
      .catch((e) => setError(e instanceof APIError ? e.developerMessage : String(e)));
  }, [eventId]);

  const hasActivity =
    data &&
    (data.counts.waiting > 0 ||
      data.counts.promoted > 0 ||
      data.counts.fulfilled > 0 ||
      data.counts.expired > 0 ||
      data.counts.removed > 0);

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded p-4 flex items-center justify-between gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-medium">Warteliste</h2>
        {error && <p className="text-xs text-red-600">{error}</p>}
        {!error && !data && <p className="text-xs text-neutral-500">Lädt…</p>}
        {data && hasActivity && (
          <p className="text-xs text-neutral-500">
            {data.counts.waiting} wartet · {data.counts.promoted} promoted ·{' '}
            {data.counts.fulfilled} erfüllt · {data.counts.expired} abgelaufen ·{' '}
            {data.counts.removed} entfernt
          </p>
        )}
        {data && !hasActivity && (
          <p className="text-xs text-neutral-500">Noch keine Einträge.</p>
        )}
      </div>
      <Link
        href={`/${localePrefix}/admin/events/${eventId}/waitlist`}
        className="text-sm rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 whitespace-nowrap"
      >
        Warteliste verwalten →
      </Link>
    </section>
  );
}


interface TeamMember {
  id: string;
  email: string;
  display_name: string;
}

interface TeamResp {
  admins: TeamMember[];
  managers: TeamMember[];
}

interface AdminUserBrief {
  id: string;
  email: string;
  role: string;
  display_name: string;
  active: boolean;
  disabled: boolean;
}

/**
 * Team section: list and edit the event_admins + event_managers assigned to
 * this event. Backend already exposes /team, /admins, /managers — this is
 * the UI glue. Picker lists users from /api/admin/users (global_admin-gated
 * endpoint; if the current viewer isn't a global_admin we still load the
 * existing team but skip the add-form). global_admins are always implicitly
 * authorised for every event, so we filter them out of the picker to avoid
 * pointless rows.
 */
function TeamSection({eventId}: {eventId: string}) {
  const [team, setTeam] = useState<TeamResp | null>(null);
  const [users, setUsers] = useState<AdminUserBrief[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickRole, setPickRole] = useState<'event_admin' | 'event_manager'>('event_admin');
  const [pickUserId, setPickUserId] = useState<string>('');

  async function loadTeam() {
    try {
      const r = await api<TeamResp>(`/api/admin/events/${eventId}/team`);
      setTeam(r);
      setError(null);
    } catch (e) {
      setError(e instanceof APIError ? e.developerMessage : String(e));
    }
  }

  async function loadUsers() {
    try {
      const r = await api<{users: AdminUserBrief[]}>('/api/admin/users');
      setUsers(r.users);
    } catch {
      // Non-fatal: viewer isn't a global_admin → picker stays hidden.
    }
  }

  useEffect(() => {
    loadTeam();
    loadUsers();
  }, [eventId]);

  async function assign() {
    if (!pickUserId) return;
    setBusy('assign');
    setMsg(null);
    try {
      const path = pickRole === 'event_admin' ? 'admins' : 'managers';
      await api(`/api/admin/events/${eventId}/${path}`, {
        method: 'POST',
        body: {user_id: pickUserId},
      });
      setPickUserId('');
      setPickerOpen(false);
      setMsg('Hinzugefügt.');
      await loadTeam();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function remove(userId: string, asAdmin: boolean) {
    if (!confirm(asAdmin ? 'Event-Admin von dieser Veranstaltung entfernen?' : 'Event-Manager von dieser Veranstaltung entfernen?')) return;
    setBusy(userId);
    setMsg(null);
    try {
      const path = asAdmin ? `admins/${userId}` : `managers/${userId}`;
      await api(`/api/admin/events/${eventId}/${path}`, {method: 'DELETE'});
      setMsg('Entfernt.');
      await loadTeam();
    } catch (e) {
      setMsg(e instanceof APIError ? `${e.code}: ${e.developerMessage}` : String(e));
    } finally {
      setBusy(null);
    }
  }

  // Build the picker options: active users matching the chosen role, minus
  // anyone already on the team for THIS event and minus global_admins
  // (they're implicitly authorised everywhere).
  const assignedIds = new Set([
    ...(team?.admins.map((a) => a.id) ?? []),
    ...(team?.managers.map((m) => m.id) ?? []),
  ]);
  const pickerOptions = (users ?? []).filter(
    (u) =>
      u.active &&
      !u.disabled &&
      u.role !== 'global_admin' &&
      (pickRole === 'event_admin'
        ? u.role === 'event_admin'
        : u.role === 'event_manager' || u.role === 'event_admin') &&
      !assignedIds.has(u.id)
  );
  const canShowPicker = users !== null; // null = not authorized → hide

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded p-4 flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-sm font-medium">Team</h2>
        {canShowPicker && !pickerOpen && (
          <button
            onClick={() => setPickerOpen(true)}
            className="text-xs rounded border border-neutral-300 dark:border-neutral-700 px-2 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800"
          >
            + Person zuweisen
          </button>
        )}
      </div>

      {error && <p className="text-xs text-red-600">{error}</p>}
      {msg && <p className="text-xs text-emerald-700">{msg}</p>}

      {pickerOpen && canShowPicker && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded p-3 flex flex-col sm:flex-row gap-2 items-stretch sm:items-end">
          <label className="flex flex-col gap-1 text-xs flex-1">
            <span className="opacity-70 uppercase tracking-wide">Rolle</span>
            <select
              value={pickRole}
              onChange={(e) => setPickRole(e.target.value as 'event_admin' | 'event_manager')}
              className="rounded border border-neutral-300 dark:border-neutral-700 bg-transparent px-2 py-1.5 text-sm"
            >
              <option value="event_admin">Event-Admin</option>
              <option value="event_manager">Event-Manager</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs flex-[2]">
            <span className="opacity-70 uppercase tracking-wide">Person</span>
            <select
              value={pickUserId}
              onChange={(e) => setPickUserId(e.target.value)}
              className="rounded border border-neutral-300 dark:border-neutral-700 bg-transparent px-2 py-1.5 text-sm"
            >
              <option value="">— bitte wählen —</option>
              {pickerOptions.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email}
                  {u.display_name ? ` (${u.display_name})` : ''} · {u.role}
                </option>
              ))}
            </select>
            {pickerOptions.length === 0 && (
              <span className="text-[10px] opacity-60 mt-1">
                Keine passenden Nutzer übrig. Über{' '}
                <Link href="/de/admin/users" className="underline">Admins</Link> zuerst einladen.
              </span>
            )}
          </label>
          <div className="flex gap-2">
            <button
              onClick={assign}
              disabled={!pickUserId || busy === 'assign'}
              className="text-xs rounded bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 px-3 py-1.5 disabled:opacity-50"
            >
              Zuweisen
            </button>
            <button
              onClick={() => {setPickerOpen(false); setPickUserId('');}}
              className="text-xs rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5"
            >
              Abbrechen
            </button>
          </div>
        </div>
      )}

      <div className="grid sm:grid-cols-2 gap-4 text-sm">
        <TeamColumn
          title="Event-Admins"
          members={team?.admins ?? []}
          loading={team === null}
          onRemove={(id) => remove(id, true)}
          busy={busy}
        />
        <TeamColumn
          title="Event-Manager"
          members={team?.managers ?? []}
          loading={team === null}
          onRemove={(id) => remove(id, false)}
          busy={busy}
        />
      </div>

      <p className="text-[11px] opacity-60">
        Hinweis: Globale Admins haben automatisch Zugriff auf alle Veranstaltungen — sie müssen
        nicht zusätzlich zugewiesen werden.
      </p>
    </section>
  );
}

function TeamColumn({
  title,
  members,
  loading,
  onRemove,
  busy,
}: {
  title: string;
  members: TeamMember[];
  loading: boolean;
  onRemove: (userId: string) => void;
  busy: string | null;
}) {
  return (
    <div className="flex flex-col gap-1">
      <h3 className="text-xs uppercase tracking-wide opacity-70">{title}</h3>
      {loading && <p className="text-xs text-neutral-500">Lädt…</p>}
      {!loading && members.length === 0 && <p className="text-xs text-neutral-500">— niemand —</p>}
      {!loading && members.length > 0 && (
        <ul className="flex flex-col gap-1">
          {members.map((m) => (
            <li key={m.id} className="flex items-center justify-between gap-2 text-sm">
              <span>
                {m.email}
                {m.display_name && <span className="opacity-60"> · {m.display_name}</span>}
              </span>
              <button
                onClick={() => onRemove(m.id)}
                disabled={busy === m.id}
                className="text-xs text-red-700 dark:text-red-300 hover:underline disabled:opacity-30"
              >
                entfernen
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

interface CheckInStatus {
  expected: number;
  checked_in: number;
}

function CheckInCTA({eventId, localePrefix}: {eventId: string; localePrefix: string}) {
  const [data, setData] = useState<CheckInStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<CheckInStatus>(`/api/admin/events/${eventId}/check-ins?limit=1`)
      .then(setData)
      .catch((e) => setError(e instanceof APIError ? e.developerMessage : String(e)));
  }, [eventId]);

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded p-4 flex items-center justify-between gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-medium">Einlass</h2>
        {error && <p className="text-xs text-red-600">{error}</p>}
        {!error && !data && <p className="text-xs text-neutral-500">Lädt…</p>}
        {data && (
          <p className="text-xs text-neutral-500">
            {data.checked_in} von {data.expected} eingecheckt
          </p>
        )}
      </div>
      <Link
        href={`/${localePrefix}/admin/events/${eventId}/check-in`}
        className="text-sm rounded border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 whitespace-nowrap"
      >
        Door-Scanner öffnen →
      </Link>
    </section>
  );
}
