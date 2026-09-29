import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { SiteHeader } from '@/components/site-header';

// next/link expects the App Router context, which a unit render does not mount.
// Swap it for a plain anchor so the header can be rendered in isolation.
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

describe('SiteHeader', () => {
  it('shows the product mark', () => {
    render(<SiteHeader />);
    expect(screen.getByText('Pulsar')).toBeInTheDocument();
    expect(screen.getByText('Explorer')).toBeInTheDocument();
  });

  it('links the mark back to the home route', () => {
    render(<SiteHeader />);
    const home = screen.getByRole('link', { name: /pulsar explorer, home/i });
    expect(home).toHaveAttribute('href', '/');
  });
});
