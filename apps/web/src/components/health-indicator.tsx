'use client';

import { useEffect, useState } from 'react';

import { formatInteger } from '@/lib/format';
import { fetchHealth } from '@/lib/graphql/browser';
import { type Health } from '@/lib/graphql/schemas';
import { cn } from '@/lib/utils';

/**
 * A live indicator of the indexer's health, shown in the site header.
 *
 * Fetches `health` through the browser dispatch transport on mount, then
 * re-checks on an interval. Polling pauses while the tab is hidden (nothing to
 * show, no reason to poll) and runs an immediate check when the tab becomes
 * visible again, so a returning user sees a fresh status without waiting for the
 * next tick. The timer, the visibility listener, and any in-flight request are
 * all torn down on unmount.
 *
 * A failed check is read as "unreachable" rather than surfaced as an error: the
 * indicator is ambient, so it degrades quietly. The region is an `aria-live`
 * status so a screen reader hears the status settle and change.
 */

type Status = 'loading' | 'online' | 'degraded' | 'unreachable';

/** How often to re-check, in milliseconds. */
const DEFAULT_POLL_MS = 30_000;

const STATUS_META: Record<Status, { dot: string; label: string }> = {
  loading: { dot: 'bg-muted-foreground/40', label: 'Checking indexer' },
  online: { dot: 'bg-emerald-500', label: 'Indexer online' },
  degraded: { dot: 'bg-amber-500', label: 'Indexer degraded' },
  unreachable: { dot: 'bg-destructive', label: 'Indexer unreachable' },
};

interface HealthIndicatorProps {
  /** Poll interval in milliseconds; injectable for tests. */
  pollMs?: number;
}

export function HealthIndicator({ pollMs = DEFAULT_POLL_MS }: HealthIndicatorProps) {
  const [status, setStatus] = useState<Status>('loading');
  const [health, setHealth] = useState<Health | null>(null);

  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    async function check(): Promise<void> {
      try {
        const result = await fetchHealth({ signal: controller.signal });
        if (!active) return;
        setHealth(result);
        setStatus(result.ok ? 'online' : 'degraded');
      } catch {
        if (!active) return;
        setHealth(null);
        setStatus('unreachable');
      }
    }

    void check();

    const interval = setInterval(() => {
      if (!document.hidden) void check();
    }, pollMs);

    function handleVisibility(): void {
      if (!document.hidden) void check();
    }
    document.addEventListener('visibilitychange', handleVisibility);

    return () => {
      active = false;
      controller.abort();
      clearInterval(interval);
      document.removeEventListener('visibilitychange', handleVisibility);
    };
  }, [pollMs]);

  const meta = STATUS_META[status];
  const ledger =
    status === 'online' && health !== null
      ? formatInteger(health.latestLedger)
      : null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center gap-2 text-sm text-muted-foreground"
    >
      <span
        aria-hidden="true"
        className={cn('inline-block size-2 shrink-0 rounded-full', meta.dot)}
      />
      {/* Only the status label is announced; the ledger advances every poll, so
          it is shown but marked aria-hidden to avoid repeated announcements. */}
      <span>{meta.label}</span>
      {ledger !== null && (
        <span aria-hidden="true">{`· ledger ${ledger}`}</span>
      )}
    </div>
  );
}
