'use client';

import Link from 'next/link';
import {usePathname} from 'next/navigation';

const LOCALES = ['de', 'en'] as const;
type Locale = (typeof LOCALES)[number];

/**
 * Tiny de/en toggle. Lives in the top nav of public pages. Computes the
 * sister-locale URL by swapping the first path segment.
 *
 * If the current path doesn't start with a known locale (shouldn't happen,
 * but defensive), it just links to `/de` and `/en` root.
 */
export function LocaleSwitcher({current}: {current: Locale}) {
  const pathname = usePathname() ?? '/';
  const segments = pathname.split('/');
  // segments[0] is the empty leading "" before the first slash.

  function pathFor(target: Locale): string {
    if (segments.length > 1 && (LOCALES as readonly string[]).includes(segments[1])) {
      const next = [...segments];
      next[1] = target;
      return next.join('/') || '/';
    }
    return `/${target}`;
  }

  return (
    <span className="flex items-center gap-2 text-[10px] uppercase tracking-[0.22em]">
      {LOCALES.map((loc, i) => {
        const active = loc === current;
        return (
          <span key={loc} className="flex items-center gap-2">
            {i > 0 && <span className="opacity-30">·</span>}
            {active ? (
              <span className="font-medium opacity-90">{loc}</span>
            ) : (
              <Link href={pathFor(loc)} className="opacity-60 hover:opacity-100">
                {loc}
              </Link>
            )}
          </span>
        );
      })}
    </span>
  );
}
