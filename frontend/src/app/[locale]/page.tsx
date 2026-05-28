import {EventsList} from '@/components/EventsList';
import {Logo} from '@/components/Logo';
import {PublicShell} from '@/components/PublicShell';

export default async function Home({params}: {params: Promise<{locale: string}>}) {
  const {locale} = await params;
  return (
    <PublicShell locale={locale}>
      <div className="flex flex-col items-center gap-4 pt-2 pb-6">
        <Logo size={140} variant="full" priority />
        <span className="text-[11px] uppercase tracking-[0.32em] opacity-70">
          kreise.berlin
        </span>
      </div>
      <EventsList locale={locale} />
    </PublicShell>
  );
}
