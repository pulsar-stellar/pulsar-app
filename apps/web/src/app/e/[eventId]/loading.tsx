/**
 * The loading skeleton for an event detail page, shown while the server fetch is
 * in flight (the route is force-dynamic). The wrapper is a polite `status` live
 * region with `aria-busy`; the shapes are decorative and hidden, and the pulse
 * honors `prefers-reduced-motion`.
 */
export default function Loading() {
  return (
    <div role="status" aria-live="polite" aria-busy="true" className="flex flex-col gap-6">
      <span className="sr-only">Loading event…</span>
      <div aria-hidden="true" className="flex flex-col gap-6">
        <div className="h-8 w-48 animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="h-10 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
          <div className="h-10 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
          <div className="h-10 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
          <div className="h-10 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        </div>
        <div className="h-32 w-full animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
      </div>
    </div>
  );
}
