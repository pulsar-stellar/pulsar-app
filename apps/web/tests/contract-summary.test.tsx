import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { ContractSummary } from '@/components/contract-summary';
import { type ContractNode } from '@/lib/graphql/schemas';

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

function makeContract(overrides: Partial<ContractNode> = {}): ContractNode {
  return {
    id: CONTRACT_ID,
    addedAt: '2026-09-29T14:03:21Z',
    firstIndexedLedger: 1000,
    lastIndexedLedger: 2000,
    status: 'active',
    ...overrides,
  };
}

describe('ContractSummary', () => {
  it('renders the full contract id and status', () => {
    render(<ContractSummary contract={makeContract()} />);
    expect(screen.getByText(CONTRACT_ID)).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
  });

  it('formats the added timestamp and ledger bounds', () => {
    render(<ContractSummary contract={makeContract()} />);
    expect(screen.getByText('Sep 29, 2026, 14:03:21 UTC')).toBeInTheDocument();
    expect(screen.getByText('1,000')).toBeInTheDocument();
    expect(screen.getByText('2,000')).toBeInTheDocument();
  });

  it('shows a placeholder when nothing has been indexed yet', () => {
    render(
      <ContractSummary contract={makeContract({ firstIndexedLedger: null })} />,
    );
    expect(screen.getByText('Not yet indexed')).toBeInTheDocument();
  });
});
