import {EventsList} from '@/components/EventsList';
import {PublicShell} from '@/components/PublicShell';

export default async function PublicEventsPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  return (
    <PublicShell locale={locale}>
      <EventsList locale={locale} />
    </PublicShell>
  );
}
