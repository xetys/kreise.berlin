import {notFound} from 'next/navigation';
import {NextIntlClientProvider, hasLocale} from 'next-intl';
import {setRequestLocale, getMessages} from 'next-intl/server';
import {routing} from '@/i18n/routing';

/**
 * Per-locale layout boundary. This is what makes the language switcher feel
 * instant: when the user clicks `/de/foo` → `/en/foo`, Next.js re-mounts this
 * layout (the `[locale]` segment changed), `getMessages()` fetches the new
 * catalog, and `NextIntlClientProvider` receives a new locale + messages
 * pair. Without this boundary the provider sits in the root layout and never
 * sees the new locale until the page is hard-reloaded.
 *
 * Also sets the request locale via `setRequestLocale` so server components
 * inside this subtree can resolve translations without round-tripping
 * `getLocale()`.
 */
export default async function LocaleLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  if (!hasLocale(routing.locales, locale)) {
    notFound();
  }
  setRequestLocale(locale);

  const messages = await getMessages({locale});

  return (
    <NextIntlClientProvider locale={locale} messages={messages}>
      {children}
    </NextIntlClientProvider>
  );
}
