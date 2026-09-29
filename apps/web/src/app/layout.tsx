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
        <SiteHeader />
        <main className="mx-auto max-w-5xl px-4 py-10">{children}</main>
      </body>
    </html>
  );
}
