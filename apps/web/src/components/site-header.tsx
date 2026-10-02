import Link from 'next/link';

import { HealthIndicator } from '@/components/health-indicator';

/**
 * The persistent top bar shown on every route.
 *
 * Server-rendered, holding the product mark and a link home. The only
 * interactive piece is the health indicator, a small client island on the
 * right; the rest stays a Server Component.
 */
export function SiteHeader() {
  return (
    <header className="border-b border-border bg-background">
      <div className="mx-auto flex h-14 max-w-5xl items-center gap-3 px-4">
        <Link
          href="/"
          aria-label="Pulsar Explorer, home"
          className="flex items-center gap-2 font-semibold tracking-tight"
        >
          <span
            aria-hidden="true"
            className="inline-block size-2.5 rounded-full bg-primary"
          />
          <span>Pulsar</span>
          <span className="text-muted-foreground font-normal">Explorer</span>
        </Link>
        <div className="ml-auto">
          <HealthIndicator />
        </div>
      </div>
    </header>
  );
}
