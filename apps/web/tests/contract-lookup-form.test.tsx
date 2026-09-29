import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ContractLookupForm } from '@/components/contract-lookup-form';

const pushMock = vi.fn();

// The form navigates on a valid id; swap the App Router for a spy so the render
// runs in isolation and the destination can be asserted.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: pushMock }),
}));

const VALID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

function typeInto(value: string): void {
  fireEvent.change(screen.getByRole('textbox', { name: /contract id/i }), {
    target: { value },
  });
}

function submit(): void {
  fireEvent.click(screen.getByRole('button', { name: /look up/i }));
}

describe('ContractLookupForm', () => {
  beforeEach(() => {
    pushMock.mockReset();
  });

  it('routes to the contract page for a well-formed id', () => {
    render(<ContractLookupForm />);
    typeInto(VALID);
    submit();
    expect(pushMock).toHaveBeenCalledWith(`/c/${VALID}`);
  });

  it('trims surrounding whitespace before routing', () => {
    render(<ContractLookupForm />);
    typeInto(`  ${VALID}  `);
    submit();
    expect(pushMock).toHaveBeenCalledWith(`/c/${VALID}`);
  });

  it('shows an error and does not route for a malformed id', () => {
    render(<ContractLookupForm />);
    typeInto('not-a-contract');
    submit();
    expect(pushMock).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent(/contract id/i);
  });

  it('shows an error and does not route on an empty submission', () => {
    render(<ContractLookupForm />);
    submit();
    expect(pushMock).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });

  it('clears the error once the input is corrected and resubmitted', () => {
    render(<ContractLookupForm />);

    submit();
    expect(screen.getByRole('alert')).toBeInTheDocument();

    typeInto(VALID);
    submit();

    expect(pushMock).toHaveBeenCalledWith(`/c/${VALID}`);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
