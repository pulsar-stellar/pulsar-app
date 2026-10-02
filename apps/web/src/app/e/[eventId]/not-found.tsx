import Link from 'next/link';

/**
 * Shown when an event detail route resolves to nothing: an event the index does
 * not hold, or a malformed id. Styled to match the app rather than the framework
 * default, mirroring the contract not-found.
 */
export default function EventNotFound() {
  return (
    <div className="flex flex-col items-start gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Event not found</h1>
      <p className="max-w-2xl text-muted-foreground">
        This event is not in the index, or the id is not a valid event id. It may
        belong to a contract that is not being tracked.
      </p>
      <Link
        href="/"
        className="text-sm font-medium underline underline-offset-4"
      >
        Back to search
      </Link>
    </div>
  );
}
