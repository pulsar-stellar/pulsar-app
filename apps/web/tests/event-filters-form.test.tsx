import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { EventFiltersForm } from '@/components/event-filters-form';
import { type EventFilters } from '@/lib/events-filters';

const push = vi.fn();
const PATHNAME = '/c/CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
  usePathname: () => PATHNAME,
}));

/** Render the form and return a `submit()` that fires the form's submit event. */
function renderForm(filters: EventFilters) {
  const { container } = render(<EventFiltersForm filters={filters} />);
  const form = container.querySelector('form');
  if (form === null) throw new Error('form did not render');
  return { submit: () => fireEvent.submit(form) };
}

describe('EventFiltersForm', () => {
  beforeEach(() => {
    push.mockClear();
  });

  it('pushes a query string built from the entered filters', () => {
    const { submit } = renderForm({});

    fireEvent.change(screen.getByLabelText(/event name/i), {
      target: { value: 'transfer' },
    });
    fireEvent.change(screen.getByLabelText(/topic contains/i), {
      target: { value: 'abc' },
    });
    fireEvent.change(screen.getByLabelText(/from ledger/i), {
      target: { value: '100' },
    });
    submit();

    expect(push).toHaveBeenCalledWith(`${PATHNAME}?name=transfer&topic=abc&from=100`);
  });

  it('omits the default newest order and includes an explicit oldest order', () => {
    const { submit } = renderForm({ name: 'transfer' });

    // Default (newest / desc) stays out of the URL.
    submit();
    expect(push).toHaveBeenLastCalledWith(`${PATHNAME}?name=transfer`);

    // Oldest first is a non-default choice and travels in the URL.
    fireEvent.change(screen.getByLabelText(/order/i), { target: { value: 'asc' } });
    submit();
    expect(push).toHaveBeenLastCalledWith(`${PATHNAME}?name=transfer&order=asc`);
  });

  it('pushes the bare path when nothing is entered', () => {
    const { submit } = renderForm({});
    submit();
    expect(push).toHaveBeenCalledWith(PATHNAME);
  });

  it('clears back to the bare path', () => {
    renderForm({ name: 'transfer', order: 'asc' });
    fireEvent.click(screen.getByRole('button', { name: /clear/i }));
    expect(push).toHaveBeenCalledWith(PATHNAME);
  });

  it('seeds the inputs from the current filters', () => {
    renderForm({ name: 'mint', topicContains: 'xyz', fromLedger: 5, toLedger: 9, order: 'asc' });
    expect(screen.getByLabelText(/event name/i)).toHaveValue('mint');
    expect(screen.getByLabelText(/topic contains/i)).toHaveValue('xyz');
    expect(screen.getByLabelText(/from ledger/i)).toHaveValue(5);
    expect(screen.getByLabelText(/to ledger/i)).toHaveValue(9);
    expect(screen.getByLabelText(/order/i)).toHaveValue('asc');
  });
});
