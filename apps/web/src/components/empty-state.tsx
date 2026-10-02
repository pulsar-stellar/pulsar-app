import { type ReactNode } from 'react';

import { cn } from '@/lib/utils';

/**
 * A consistent empty state: a dashed, centered panel with an optional title and
 * a muted message. Shared so "no events", "no topics", and similar reads the
 * same across the explorer.
 */
interface EmptyStateProps {
  /** An optional bolder lead line above the message. */
  title?: string;
  /** The message body. */
  children: ReactNode;
  className?: string;
}

export function EmptyState({ title, children, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        'rounded-md border border-dashed border-border px-4 py-8 text-center',
        className,
      )}
    >
      {title !== undefined && <p className="text-sm font-medium">{title}</p>}
      <p className={cn('text-sm text-muted-foreground', title !== undefined && 'mt-1')}>
        {children}
      </p>
    </div>
  );
}
