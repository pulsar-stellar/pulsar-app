import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { EventsPanel } from '@/components/events-panel';
import { EventFetchError } from '@/lib/graphql/browser';
import { type EventNode } from '@/lib/graphql/schemas';

const fetchEventsPage = vi.hoisted(() => vi.fn());

vi.mock('@/lib/graphql/browser', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('@/lib/graphql/browser');
  return { ...actual, fetchEventsPage };
});

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/c/CONTRACT',
}));

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

function makeEvent(overrides: Partial<EventNode> = {}): EventNode {
  return {
    id: '1',
    contractId: CONTRACT_ID,
    ledger: 1000,
    txHash: 'abcdef0123456789abcdef0123456789',
    eventIndex: 0,
    name: 'transfer',
    topics: [],
    data: { type: 'void' },
    rawTopics: [],
    rawData: '',
    emittedAt: '2026-09-29T14:03:21Z',
    inSuccessfulContractCall: true,
    ...overrides,
  };
}

describe('EventsPanel', () => {
  beforeEach(() => {
    fetchEventsPage.mockReset();
  });

  it('hides the load-more button when there is no next cursor', () => {
    render(
      <EventsPanel
        contractId={CONTRACT_ID}
        filters={{}}
        initialEvents={[makeEvent()]}
        initialCursor={null}
      />,
    );
    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument();
  });

  it('fetches the next page with the cursor and filters, then appends it', async () => {
    fetchEventsPage.mockResolvedValue({
      items: [makeEvent({ id: '2', name: 'mint' })],
      nextCursor: null,
    });

    render(
      <EventsPanel
        contractId={CONTRACT_ID}
        filters={{ name: 'transfer' }}
        initialEvents={[makeEvent({ id: '1', name: 'transfer' })]}
        initialCursor="cur1"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /load more/i }));

    expect(await screen.findByText('mint')).toBeInTheDocument();
    expect(screen.getByText('transfer')).toBeInTheDocument();
    expect(fetchEventsPage).toHaveBeenCalledWith(
      { contractId: CONTRACT_ID, name: 'transfer', cursor: 'cur1', limit: 25 },
      expect.objectContaining({}),
    );
    // Cursor exhausted, so the button is gone.
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument(),
    );
  });

  it('shows an alert and keeps the button when a page fails to load', async () => {
    fetchEventsPage.mockRejectedValue(
      new EventFetchError('bad limit', 'validation', 'VALIDATION_LIMIT'),
    );

    render(
      <EventsPanel
        contractId={CONTRACT_ID}
        filters={{}}
        initialEvents={[makeEvent()]}
        initialCursor="cur1"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /load more/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent('bad limit');
    expect(screen.getByRole('button', { name: /load more/i })).toBeInTheDocument();
  });
});
