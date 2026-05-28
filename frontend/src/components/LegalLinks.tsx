import Link from 'next/link';

/**
 * Inline links to /impressum and /datenschutz, used inside footers across the
 * public site. Every public page must reach them in 1 click — that's the
 * minimum German law expects.
 */
export function LegalLinks({locale, className = ''}: {locale: string; className?: string}) {
  return (
    <span className={className}>
      <Link href={`/${locale}/impressum`} className="hover:underline">
        Impressum
      </Link>
      <span aria-hidden className="opacity-40 mx-2">
        ·
      </span>
      <Link href={`/${locale}/datenschutz`} className="hover:underline">
        Datenschutz
      </Link>
    </span>
  );
}
