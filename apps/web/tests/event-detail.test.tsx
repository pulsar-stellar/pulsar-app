import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { EventDetail } from '@/components/event-detail';
import { type EventNode } from '@/lib/graphql/schemas';

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
    txHash: 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789',
    eventIndex: 3,
    name: 'transfer',
    topics: [{ type: 'symbol', value: 'transfer' }],
    data: { type: 'i128', value: '1000000' },
    rawTopics: ['QUFBQXRvcGlj'],
    rawData: 'QUFBQWRhdGE=',
    emittedAt: '2026-09-29T14:03:21Z',
    inSuccessfulContractCall: true,
    ...overrides,
  };
}

describe('EventDetail', () => {
  it('shows the event name as a heading', () => {
    render(<EventDetail event={makeEvent({ name: 'transfer' })} />);
    expect(screen.getByRole('heading', { name: /transfer/i })).toBeInTheDocument();
  });

  it('labels an event with no decoded name', () => {
    render(<EventDetail event={makeEvent({ name: '' })} />);
    expect(screen.getByText('Unnamed event')).toBeInTheDocument();
  });

  it('flags a reverted event and omits the flag for a committed one', () => {
    const { rerender } = render(
      <EventDetail event={makeEvent({ inSuccessfulContractCall: false })} />,
    );
    expect(screen.getByText('Reverted')).toBeInTheDocument();

    rerender(<EventDetail event={makeEvent({ inSuccessfulContractCall: true })} />);
    expect(screen.queryByText('Reverted')).not.toBeInTheDocument();
  });

  it('shows the identity fields', () => {
    render(<EventDetail event={makeEvent()} />);
    expect(screen.getByText('42')).toBeInTheDocument(); // id
    expect(screen.getByText('1,234,567')).toBeInTheDocument(); // ledger, grouped
    expect(screen.getByText('3')).toBeInTheDocument(); // event index
    expect(
      screen.getByText('abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'),
    ).toBeInTheDocument(); // tx hash, full
    expect(screen.getByText('Sep 29, 2026, 14:03:21 UTC')).toBeInTheDocument();
  });

  it('links the contract id to its detail page', () => {
    render(<EventDetail event={makeEvent()} />);
    expect(screen.getByRole('link', { name: new RegExp(CONTRACT_ID) })).toHaveAttribute(
      'href',
      `/c/${CONTRACT_ID}`,
    );
  });

  it('renders the decoded topics and data', () => {
    render(<EventDetail event={makeEvent({ name: 'mint' })} />);
    // Topic symbol value and the i128 data value, both via DecodedValueView.
    expect(screen.getByText('transfer')).toBeInTheDocument();
    expect(screen.getByText('1000000')).toBeInTheDocument();
  });

  it('shows an empty state when there are no topics', () => {
    render(<EventDetail event={makeEvent({ topics: [] })} />);
    expect(screen.getByText(/no topics/i)).toBeInTheDocument();
  });

  it('exposes the raw XDR provenance', () => {
    render(<EventDetail event={makeEvent()} />);
    expect(screen.getByText('QUFBQXRvcGlj')).toBeInTheDocument(); // raw topic
    expect(screen.getByText('QUFBQWRhdGE=')).toBeInTheDocument(); // raw data
  });
});
