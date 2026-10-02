'use client';

import { ErrorState } from '@/components/error-state';

/**
 * The route error boundary for the explorer's segments.
 *
 * Catches an unexpected failure during a server render or data load (a not-found
 * is handled separately via `notFound()`), and offers a retry that re-runs the
 * segment. The thrown error is intentionally not shown: the UI stays generic to
 * avoid leaking internals, and the server has already logged the real cause.
 */
export default function RouteError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return <ErrorState onRetry={reset} />;
}
