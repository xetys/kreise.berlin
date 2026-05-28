'use client';

import {use, useEffect, useState} from 'react';
import {useRouter, usePathname} from 'next/navigation';
import {api, APIError} from '@/lib/api';
import {EventForm, eventFormFromDTO, type EventFormValues} from '@/components/EventForm';
import {PricingEditor} from '@/components/PricingEditor';
import type {EventDTO, ProgramEntryDTO} from '@/lib/types';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

export default function EditEventPage({params}: PageProps) {
  const {locale, id} = use(params);
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? locale;

  const [event, setEvent] = useState<EventDTO | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);

  useEffect(() => {
    api<EventDTO>(`/api/admin/events/${id}`)
      .then(setEvent)
      .catch((e) => setError(String(e)));
  }, [id]);

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!event) return <p className="text-sm text-neutral-500">Lädt…</p>;

  async function refresh() {
    const fresh = await api<EventDTO>(`/api/admin/events/${id}`);
    setEvent(fresh);
  }

  async function handleSubmit(values: EventFormValues) {
    // Slug is immutable — backend ignores it on PATCH.
    const {slug: _slug, ...patch} = values;
    void _slug;
    await api(`/api/admin/events/${id}`, {method: 'PATCH', body: patch});
    setActionMsg('Gespeichert.');
    await refresh();
  }

  async function handleBannerUpload(file: File) {
    const fd = new FormData();
    fd.append('banner', file);
    await api(`/api/admin/events/${id}/banner`, {method: 'POST', body: fd});
    await refresh();
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-1">
          <button
            onClick={() => router.push(`/${localePrefix}/admin/events/${id}`)}
            className="text-xs text-neutral-500 hover:underline self-start"
          >
            ← {event.name}
          </button>
          <h1 className="text-2xl font-semibold">Bearbeiten</h1>
        </div>
      </div>

      {actionMsg && <p className="text-sm text-emerald-700">{actionMsg}</p>}

      <BannerSection event={event} onUpload={handleBannerUpload} />

      <EventForm mode="edit" initial={eventFormFromDTO(event)} onSubmit={handleSubmit} />

      <ProgramSection eventID={id} />

      <PricingEditor eventId={id} mode={event.pricing_mode} currency={event.currency} />
    </div>
  );
}

function BannerSection({event, onUpload}: {event: EventDTO; onUpload: (f: File) => Promise<void>}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    setBusy(true);
    setErr(null);
    try {
      await onUpload(f);
    } catch (ex) {
      setErr(ex instanceof APIError ? ex.code : String(ex));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="flex flex-col gap-2 border-t border-neutral-200 dark:border-neutral-800 pt-4">
      <h2 className="text-sm font-medium">Banner</h2>
      {event.banner_url ? (
        <img
          src={event.banner_url + '?v=' + event.id}
          alt=""
          className="max-h-48 rounded border border-neutral-200 dark:border-neutral-800 object-contain"
        />
      ) : (
        <p className="text-xs text-neutral-500">Noch kein Banner.</p>
      )}
      <input type="file" accept="image/*" onChange={handleChange} disabled={busy} className="text-sm" />
      {err && <p className="text-sm text-red-600">{err}</p>}
    </section>
  );
}

function ProgramSection({eventID}: {eventID: string}) {
  const [entries, setEntries] = useState<ProgramEntryDTO[] | null>(null);
  const [title, setTitle] = useState('');
  const [startsAt, setStartsAt] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    const r = await api<{entries: ProgramEntryDTO[]}>(`/api/admin/events/${eventID}/program`);
    setEntries(r.entries);
  }

  useEffect(() => {
    load().catch((e) => setErr(String(e)));
  }, [eventID]);

  async function add() {
    if (!title || !startsAt) return;
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/admin/events/${eventID}/program`, {
        method: 'POST',
        body: {
          title,
          starts_at: new Date(startsAt).toISOString(),
          ends_at: null,
          description: '',
          ordering: entries?.length ?? 0,
        },
      });
      setTitle('');
      setStartsAt('');
      await load();
    } catch (e) {
      setErr(e instanceof APIError ? e.code : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(entryID: string) {
    await api(`/api/admin/events/${eventID}/program/${entryID}`, {method: 'DELETE'});
    await load();
  }

  return (
    <section className="flex flex-col gap-3 border-t border-neutral-200 dark:border-neutral-800 pt-4">
      <h2 className="text-sm font-medium">Programm</h2>

      {entries === null && <p className="text-xs text-neutral-500">Lädt…</p>}
      {entries && entries.length === 0 && <p className="text-xs text-neutral-500">Noch keine Programmpunkte.</p>}
      {entries && entries.length > 0 && (
        <ul className="flex flex-col gap-1 text-sm">
          {entries.map((e) => (
            <li key={e.id} className="flex items-center justify-between border-b border-neutral-100 dark:border-neutral-900 py-1">
              <span>
                {new Date(e.starts_at).toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'})} ·{' '}
                <strong>{e.title}</strong>
              </span>
              <button onClick={() => remove(e.id)} className="text-xs text-red-600 hover:underline">
                entfernen
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="flex gap-2 text-sm">
        <input
          type="datetime-local"
          value={startsAt}
          onChange={(e) => setStartsAt(e.target.value)}
          className={inputCls}
        />
        <input
          type="text"
          placeholder="Titel"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className={inputCls + ' flex-1'}
        />
        <button onClick={add} disabled={busy || !title || !startsAt} className={btnClass}>
          Hinzufügen
        </button>
      </div>
      {err && <p className="text-sm text-red-600">{err}</p>}
    </section>
  );
}

const btnClass =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800';

const inputCls = 'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-1.5 bg-transparent';
