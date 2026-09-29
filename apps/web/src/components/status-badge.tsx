import { type ContractStatus } from '@pulsar-stellar/sdk';

import { cn } from '@/lib/utils';

/**
 * A contract's indexing status, as a labelled pill.
 *
 * The status is never conveyed by color alone: the text label is always
 * present, and the colored dot is decorative (`aria-hidden`). This keeps the
 * badge legible to screen readers and to anyone who cannot distinguish the hues.
 */

interface StatusStyle {
  label: string;
  dot: string;
}

const STATUS_STYLES: Record<ContractStatus, StatusStyle> = {
  active: { label: 'Active', dot: 'bg-emerald-500' },
  paused: { label: 'Paused', dot: 'bg-amber-500' },
  error: { label: 'Error', dot: 'bg-destructive' },
};

interface StatusBadgeProps {
  status: ContractStatus;
  className?: string;
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const { label, dot } = STATUS_STYLES[status];
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border border-border ' +
          'px-2.5 py-0.5 text-xs font-medium',
        className,
      )}
    >
      <span aria-hidden="true" className={cn('size-2 rounded-full', dot)} />
      {label}
    </span>
  );
}
