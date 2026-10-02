import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { EmptyState } from '@/components/empty-state';

describe('EmptyState', () => {
  it('renders its message', () => {
    render(<EmptyState>No events to show.</EmptyState>);
    expect(screen.getByText('No events to show.')).toBeInTheDocument();
  });

  it('renders an optional title above the message', () => {
    render(<EmptyState title="Nothing here yet">Check back later.</EmptyState>);
    expect(screen.getByText('Nothing here yet')).toBeInTheDocument();
    expect(screen.getByText('Check back later.')).toBeInTheDocument();
  });
});
