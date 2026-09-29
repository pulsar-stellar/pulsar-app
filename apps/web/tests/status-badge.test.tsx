import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { StatusBadge } from '@/components/status-badge';

describe('StatusBadge', () => {
  it('labels an active contract', () => {
    render(<StatusBadge status="active" />);
    expect(screen.getByText('Active')).toBeInTheDocument();
  });

  it('labels a paused contract', () => {
    render(<StatusBadge status="paused" />);
    expect(screen.getByText('Paused')).toBeInTheDocument();
  });

  it('labels an errored contract', () => {
    render(<StatusBadge status="error" />);
    expect(screen.getByText('Error')).toBeInTheDocument();
  });

  it('merges a caller-supplied class', () => {
    const { container } = render(
      <StatusBadge status="active" className="mt-4" />,
    );
    expect(container.firstElementChild).toHaveClass('mt-4');
  });
});
