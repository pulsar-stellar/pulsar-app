import type { Metadata } from 'next';

import { SiteHeader } from '@/components/site-header';

import './globals.css';

export const metadata: Metadata = {
  title: {
    default: 'Pulsar Explorer',
    template: '%s | Pulsar Explorer',
  },
  description:
    'Browse decoded events for any Soroban contract. Paste a contract ID and read every event it has emitted, decoded and searchable.',
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="min-h-dvh antialiased">
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:shadow focus:ring-2 focus:ring-ring"
        >
          Skip to content
        </a>
        <SiteHeader />
        <main id="main" className="mx-auto max-w-5xl px-4 py-10">
          {children}
        </main>
      </body>
    </html>
  );
}
