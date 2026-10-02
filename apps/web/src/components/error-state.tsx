import Link from 'next/link';

import { Button } from '@/components/ui/button';

/**
 * A consistent error state: a heading, a generic message, an optional retry, and
 * a link home. Shared by the route error boundaries.
 *
 * The message is deliberately generic and never echoes the thrown error: an
 * error boundary catches unexpected failures whose messages could carry internal
 * detail, so nothing from the cause reaches the UI (the server logs the real
 * error). The region is an `alert` so a screen reader announces it.
 */
interface ErrorStateProps {
  title?: string;
  description?: string;
  /** When given, renders a "Try again" button invoking it (the boundary's reset). */
  onRetry?: () => void;
  homeHref?: string;
}

export function ErrorState({
  title = 'Something went wrong',
  description = 'The explorer could not complete that request. Please try again.',
  onRetry,
  homeHref = '/',
}: ErrorStateProps) {
  return (
    <div role="alert" className="flex flex-col items-start gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="max-w-2xl text-muted-foreground">{description}</p>
      <div className="flex items-center gap-3">
        {onRetry !== undefined && (
          <Button type="button" onClick={onRetry}>
            Try again
          </Button>
        )}
        <Link
          href={homeHref}
          className="text-sm font-medium underline underline-offset-4"
        >
          Back to search
        </Link>
      </div>
    </div>
  );
}
