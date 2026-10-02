import Link from 'next/link';

import { EmptyState } from '@/components/empty-state';
import { formatInteger, formatTimestamp, truncateMiddle } from '@/lib/format';
import { type EventNode } from '@/lib/graphql/schemas';

/**
 * The list of a contract's events, one row per event, each linking to its
 * detail page.
 *
 * Presentational and read-only: it renders whatever page of events it is given
 * and shows an empty state for none. Filtering, ordering, and pagination live in
 * the interactive panel that wraps this list; keeping the list dumb lets both
 * the server-rendered first page and that panel share it unchanged. The empty
 * state is neutral ("no events to show") because the same list renders both an
 * unfiltered contract and a filtered query that matched nothing.
 */

/** An event with no decoded name degrades to this label rather than a blank row (ADR-026). */
const UNNAMED_LABEL = 'Unnamed event';

interface EventRowProps {
  event: EventNode;
}

function EventRow({ event }: EventRowProps) {
  const named = event.name !== '';
  return (
    <li>
      <Link
        href={`/e/${event.id}`}
        className="flex flex-col gap-1 rounded-md border border-border px-4 py-3 transition-colors hover:bg-accent sm:flex-row sm:items-center sm:justify-between"
      >
        <div className="flex items-center gap-2">
          <span className={named ? 'font-medium' : 'font-medium italic text-muted-foreground'}>
            {named ? event.name : UNNAMED_LABEL}
          </span>
          {!event.inSuccessfulContractCall && (
            <span className="rounded-full border border-amber-500 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-400">
              Reverted
            </span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>Ledger {formatInteger(event.ledger)}</span>
          <span>{formatTimestamp(event.emittedAt)}</span>
          <span className="font-mono" title={event.txHash}>
            <span className="sr-only">Transaction </span>
            <span>{truncateMiddle(event.txHash)}</span>
          </span>
        </div>
      </Link>
    </li>
  );
}

interface EventListProps {
  events: readonly EventNode[];
}

export function EventList({ events }: EventListProps) {
  if (events.length === 0) {
    return <EmptyState>No events to show.</EmptyState>;
  }

  return (
    <ul className="flex flex-col gap-2">
      {events.map((event) => (
        <EventRow key={event.id} event={event} />
      ))}
    </ul>
  );
}
