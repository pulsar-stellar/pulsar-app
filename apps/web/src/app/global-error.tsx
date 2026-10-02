'use client';

import { ErrorState } from '@/components/error-state';

import './globals.css';

/**
 * The root error boundary, used when the error escapes the root layout itself.
 *
 * It replaces the whole document, so it renders its own `<html>`/`<body>` and
 * pulls in the global stylesheet. Like the segment boundary, it shows a generic
 * message and a retry, never the thrown cause.
 */
export default function GlobalError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="en">
      <head>
        <title>Something went wrong | Pulsar Explorer</title>
      </head>
      <body className="min-h-dvh antialiased">
        <main className="mx-auto max-w-5xl px-4 py-10">
          <ErrorState onRetry={reset} />
        </main>
      </body>
    </html>
  );
}
