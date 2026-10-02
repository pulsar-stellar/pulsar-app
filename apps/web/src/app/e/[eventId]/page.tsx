import { type Metadata } from 'next';
import { notFound } from 'next/navigation';

import { EventDetail } from '@/components/event-detail';
import { EventJsonExport } from '@/components/event-json-export';
import { eventJsonFilename, isEventId } from '@/lib/event-id';
import { GraphQLRequestError } from '@/lib/graphql/errors';
import { getEvent } from '@/lib/graphql/queries';
import { type EventNode } from '@/lib/graphql/schemas';

/**
 * An event's detail page.
 *
 * Fetches one decoded event by id and renders its topics and data through the
 * recursive {@link EventDetail}/`DecodedValueView`, with a JSON export beside it.
 * The data is live, so the route is dynamic. A malformed id or an event the
 * index does not hold renders the segment's not-found UI (a bad id never reaches
 * the network; an absent one maps `GraphQLRequestError.isNotFound` to null).
 */

export const dynamic = 'force-dynamic';

interface EventPageProps {
  params: Promise<{ eventId: string }>;
}

/** Fetch the event, mapping a not-found signal to `null` and rethrowing the rest. */
async function loadEvent(id: string): Promise<EventNode | null> {
  try {
    return await getEvent(id);
  } catch (error) {
    if (error instanceof GraphQLRequestError && error.isNotFound) return null;
    throw error;
  }
}

export async function generateMetadata({
  params,
}: EventPageProps): Promise<Metadata> {
  const { eventId } = await params;
  return { title: isEventId(eventId) ? `Event ${eventId}` : 'Event' };
}

export default async function EventPage({ params }: EventPageProps) {
  const { eventId } = await params;
  if (!isEventId(eventId)) notFound();

  const event = await loadEvent(eventId);
  if (event === null) notFound();

  const json = JSON.stringify(event, null, 2);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex justify-end">
        <EventJsonExport json={json} filename={eventJsonFilename(event.id)} />
      </div>
      <EventDetail event={event} />
    </div>
  );
}
