import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ErrorState } from '@/components/error-state';

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

describe('ErrorState', () => {
  it('renders a default generic message as an alert', () => {
    render(<ErrorState />);
    const alert = screen.getByRole('alert');
    expect(alert).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /something went wrong/i })).toBeInTheDocument();
  });

  it('does not leak an internal error message', () => {
    render(<ErrorState />);
    // The body is generic; nothing echoes a raw cause.
    expect(screen.queryByText(/stack|undefined|null pointer/i)).not.toBeInTheDocument();
  });

  it('calls onRetry when the retry button is pressed', () => {
    const onRetry = vi.fn();
    render(<ErrorState onRetry={onRetry} />);
    fireEvent.click(screen.getByRole('button', { name: /try again/i }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('omits the retry button when no handler is given', () => {
    render(<ErrorState />);
    expect(screen.queryByRole('button', { name: /try again/i })).not.toBeInTheDocument();
  });

  it('links back home', () => {
    render(<ErrorState />);
    expect(screen.getByRole('link', { name: /back to search/i })).toHaveAttribute('href', '/');
  });
});
