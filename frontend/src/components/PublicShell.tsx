import Link from 'next/link';
import {LegalLinks} from './LegalLinks';
import {LocaleSwitcher} from './LocaleSwitcher';
import {Logo} from './Logo';
import {ThemeSwitcher} from './ThemeSwitcher';

/**
 * Generic public-page wrapper: thin top header with the site title, main slot
 * for the page body, and a footer with site name + legal links.
 *
 * Themed pages (per-event landing, ticket page) bring their own headers and
 * footers; they should still surface <LegalLinks /> directly in their themed
 * footer instead of using PublicShell.
 */
export function PublicShell({locale, children}: {locale: string; children: React.ReactNode}) {
  return (
    <div className="min-h-screen flex flex-col bg-neutral-50 dark:bg-neutral-950">
      <header className="border-b border-neutral-200 dark:border-neutral-900">
        <div className="max-w-3xl mx-auto px-5 py-4 flex items-center justify-between gap-4">
          <Link
            href={`/${locale}`}
            className="flex items-center gap-3"
            aria-label="kreise.berlin"
          >
            <Logo size={28} variant="mark" priority />
            <span className="text-[11px] uppercase tracking-[0.22em] font-medium">
              {locale === 'en' ? 'Events' : 'Veranstaltungen'}
            </span>
          </Link>
          <div className="flex items-center gap-3">
            <LocaleSwitcher current={locale === 'en' ? 'en' : 'de'} />
            <ThemeSwitcher />
          </div>
        </div>
      </header>

      <main className="flex-1 max-w-3xl mx-auto w-full px-5 py-12 sm:py-16 flex flex-col gap-10">
        {children}
      </main>

      <footer className="border-t border-neutral-200 dark:border-neutral-900 py-8">
        <div className="max-w-3xl mx-auto px-5 flex flex-col items-center gap-2 text-[10px] uppercase tracking-[0.22em] opacity-60">
          <span>kreise.berlin</span>
          <LegalLinks locale={locale} className="text-[10px] tracking-[0.18em]" />
        </div>
      </footer>
    </div>
  );
}
