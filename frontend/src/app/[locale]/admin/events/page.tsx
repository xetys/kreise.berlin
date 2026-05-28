'use client';

import Link from 'next/link';
import {useEffect, useState} from 'react';
import {usePathname} from 'next/navigation';
import {api} from '@/lib/api';
import type {EventListResponse, EventDTO} from '@/lib/types';

function formatDate(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString('de-DE', {dateStyle: 'medium', timeStyle: 'short'});
}

function statusLabel(e: EventDTO): string {
  if (e.is_archived) return 'archiviert';
  if (e.is_public) return 'veröffentlicht';
  return 'Entwurf';
}

export default function EventsListPage() {
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const [events, setEvents] = useState<EventDTO[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<EventListResponse>('/api/admin/events')
      .then((r) => setEvents(r.events))
      .catch((e) => setError(String(e)));
  }, []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Events</h1>
        <Link
          href={`/${localePrefix}/admin/events/new`}
          className="bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 rounded px-4 py-2 text-sm"
        >
          Event erstellen
        </Link>
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}
      {events === null && !error && <p className="text-sm text-neutral-500">Lädt…</p>}

      {events && events.length === 0 && (
        <div className="border border-dashed border-neutral-300 dark:border-neutral-700 rounded p-6 text-center text-sm text-neutral-500">
          Noch keine Events. Über &laquo;Event erstellen&raquo; das erste anlegen.
        </div>
      )}

      {events && events.length > 0 && (
        <table className="w-full text-sm">
          <thead className="border-b border-neutral-200 dark:border-neutral-800 text-left text-neutral-500">
            <tr>
              <th className="py-2 font-normal">Name</th>
              <th className="py-2 font-normal">Slug</th>
              <th className="py-2 font-normal">Beginn</th>
              <th className="py-2 font-normal">Limit</th>
              <th className="py-2 font-normal">Status</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr
                key={e.id}
                className="border-b border-neutral-100 dark:border-neutral-900 hover:bg-neutral-50 dark:hover:bg-neutral-900/50"
              >
                <td className="py-3">
                  <Link href={`/${localePrefix}/admin/events/${e.id}`} className="hover:underline">
                    {e.name}
                  </Link>
                </td>
                <td className="py-3 text-neutral-500">{e.slug}</td>
                <td className="py-3">{formatDate(e.starts_at)}</td>
                <td className="py-3">
                  {e.participant_limit ?? <span className="text-neutral-400">unbegrenzt</span>}
                </td>
                <td className="py-3">{statusLabel(e)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
