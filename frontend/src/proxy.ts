import createMiddleware from 'next-intl/middleware';
import {routing} from './i18n/routing';

export default createMiddleware(routing);

// Excluded from locale routing:
//   api      — proxied to backend (no locale prefix)
//   banners  — proxied to backend banner pass-through (no locale prefix)
//   _next    — Next.js internals
//   _vercel  — Vercel internals
//   .*\..*   — any path with a dot (favicon, static assets)
export const config = {
  matcher: '/((?!api|banners|_next|_vercel|.*\\..*).*)',
};
