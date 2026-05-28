'use client';

import {useRouter, usePathname} from 'next/navigation';
import {api} from '@/lib/api';
import {EventForm, eventFormDefaults, type EventFormValues} from '@/components/EventForm';
import type {EventDTO} from '@/lib/types';

export default function NewEventPage() {
  const router = useRouter();
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';

  async function handleSubmit(values: EventFormValues) {
    const created = await api<EventDTO>('/api/admin/events', {method: 'POST', body: values});
    router.push(`/${localePrefix}/admin/events/${created.id}`);
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold">Event erstellen</h1>
      <EventForm mode="create" initial={eventFormDefaults()} onSubmit={handleSubmit} />
    </div>
  );
}
