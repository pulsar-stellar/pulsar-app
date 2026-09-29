import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { EventList } from '@/components/event-list';
import { type EventNode } from '@/lib/graphql/schemas';

// next/link expects the App Router context, which a unit render does not mount.
// Swap it for a plain anchor so the list can be rendered in isolation.
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
  } & React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

function makeEvent(overrides: Partial<EventNode> = {}): EventNode {
  return {
    id: '42',
    contractId: CONTRACT_ID,
    ledger: 1234567,
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

describe('EventList', () => {
  it('renders a row per event linking to its detail page', () => {
    render(
      <EventList
        events={[
          makeEvent({ id: '1', name: 'transfer' }),
          makeEvent({ id: '2', name: 'mint' }),
        ]}
      />,
    );
    expect(screen.getByRole('link', { name: /transfer/i })).toHaveAttribute(
      'href',
      '/e/1',
    );
    expect(screen.getByRole('link', { name: /mint/i })).toHaveAttribute(
      'href',
      '/e/2',
    );
  });

  it('shows the ledger and truncated tx hash for a row', () => {
    render(<EventList events={[makeEvent()]} />);
    expect(screen.getByText('Ledger 1,234,567')).toBeInTheDocument();
    expect(screen.getByText('abcdef…456789')).toBeInTheDocument();
  });

  it('labels an event with no decoded name', () => {
    render(<EventList events={[makeEvent({ name: '' })]} />);
    expect(screen.getByText('Unnamed event')).toBeInTheDocument();
  });

  it('flags an event from a reverted call', () => {
    render(<EventList events={[makeEvent({ inSuccessfulContractCall: false })]} />);
    expect(screen.getByText('Reverted')).toBeInTheDocument();
  });

  it('renders an empty state when there are no events', () => {
    render(<EventList events={[]} />);
    expect(screen.getByText(/no events yet/i)).toBeInTheDocument();
  });
});
