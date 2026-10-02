import { act, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { HealthIndicator } from '@/components/health-indicator';

const fetchHealth = vi.hoisted(() => vi.fn());

vi.mock('@/lib/graphql/browser', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('@/lib/graphql/browser');
  return { ...actual, fetchHealth };
});

describe('HealthIndicator', () => {
  beforeEach(() => {
    fetchHealth.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('reports the indexer online with its latest ledger', async () => {
    fetchHealth.mockResolvedValue({
      ok: true,
      version: '0.1.0',
      latestLedger: 1284913,
      trackedContracts: 3,
    });

    render(<HealthIndicator />);

    expect(await screen.findByText(/indexer online/i)).toBeInTheDocument();
    expect(screen.getByText(/ledger 1,284,913/)).toBeInTheDocument();
    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('reports a degraded indexer when health is not ok', async () => {
    fetchHealth.mockResolvedValue({
      ok: false,
      version: '0.1.0',
      latestLedger: 0,
      trackedContracts: 0,
    });

    render(<HealthIndicator />);

    expect(await screen.findByText(/indexer degraded/i)).toBeInTheDocument();
  });

  it('reports the indexer unreachable when the check fails', async () => {
    fetchHealth.mockRejectedValue(new Error('offline'));

    render(<HealthIndicator />);

    expect(await screen.findByText(/indexer unreachable/i)).toBeInTheDocument();
  });

  it('checks health once on mount', async () => {
    fetchHealth.mockResolvedValue({
      ok: true,
      version: '0.1.0',
      latestLedger: 1,
      trackedContracts: 0,
    });

    render(<HealthIndicator pollMs={60000} />);

    await screen.findByText(/indexer online/i);
    expect(fetchHealth).toHaveBeenCalledTimes(1);
  });

  it('re-checks when the tab becomes visible again', async () => {
    fetchHealth.mockResolvedValue({
      ok: true,
      version: '0.1.0',
      latestLedger: 5,
      trackedContracts: 1,
    });

    render(<HealthIndicator pollMs={60000} />);
    await screen.findByText(/indexer online/i);
    expect(fetchHealth).toHaveBeenCalledTimes(1);

    // document.hidden is false in jsdom, so a visibility change triggers a check.
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await waitFor(() => expect(fetchHealth).toHaveBeenCalledTimes(2));
  });

  it('re-checks on each poll interval', async () => {
    vi.useFakeTimers();
    try {
      fetchHealth.mockResolvedValue({
        ok: true,
        version: '0.1.0',
        latestLedger: 5,
        trackedContracts: 1,
      });

      render(<HealthIndicator pollMs={5000} />);
      // Flush the mount check's microtasks under fake timers.
      await act(async () => {
        await Promise.resolve();
      });
      expect(fetchHealth).toHaveBeenCalledTimes(1);

      await act(async () => {
        vi.advanceTimersByTime(5000);
        await Promise.resolve();
      });
      expect(fetchHealth).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
