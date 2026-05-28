import type {Metadata} from 'next';
import {Geist, Geist_Mono} from 'next/font/google';
import {getLocale} from 'next-intl/server';
import './globals.css';

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'kreise.berlin',
  description: 'Bewusste Veranstaltungen in Berlin',
};

/**
 * Root layout — pure HTML scaffolding only. The NextIntlClientProvider lives
 * in `app/[locale]/layout.tsx` so it re-mounts on locale change (instant
 * switcher). We still call getLocale() here only to set <html lang=…> for
 * accessibility / search engines.
 */
export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const locale = await getLocale();
  return (
    <html
      lang={locale}
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">{children}</body>
    </html>
  );
}
