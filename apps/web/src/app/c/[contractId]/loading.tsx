/**
 * The loading skeleton for a contract detail page, shown while the server fetch
 * is in flight (the route is force-dynamic). The wrapper is a polite `status`
 * live region with `aria-busy`, so assistive tech hears that the segment is
 * loading; the shapes themselves are decorative and hidden. The pulse honors
 * `prefers-reduced-motion`.
 */
export default function Loading() {
  return (
    <div role="status" aria-live="polite" aria-busy="true" className="flex flex-col gap-8">
      <span className="sr-only">Loading contract…</span>
      <div aria-hidden="true" className="flex flex-col gap-8">
        <div className="h-28 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        <div className="h-6 w-40 animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        <div className="flex flex-col gap-2">
          <div className="h-16 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
          <div className="h-16 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
          <div className="h-16 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        </div>
      </div>
    </div>
  );
}
