import Link from 'next/link';

/**
 * The persistent top bar shown on every route.
 *
 * Presentational and server-rendered: it holds the product mark and a link home
 * and nothing interactive, so it stays a Server Component. Interactive pieces
 * (the contract lookup field) live in their own client components on the pages
 * that need them.
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
        <span className="ml-auto text-sm text-muted-foreground">
          Soroban contract events
        </span>
      </div>
    </header>
  );
}
