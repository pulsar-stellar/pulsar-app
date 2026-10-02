'use client';

import { useEffect, useRef, useState } from 'react';

import { EventFiltersForm } from '@/components/event-filters-form';
import { EventList } from '@/components/event-list';
import { Button } from '@/components/ui/button';
import { type EventFilters } from '@/lib/events-filters';
import { EventFetchError, fetchEventsPage } from '@/lib/graphql/browser';
import { type EventNode } from '@/lib/graphql/schemas';

/**
 * The interactive event list: filter controls, the rendered rows, and cursor
 * pagination over the explorer's `/api/graphql` route.
 *
 * The server renders the first page (against the filters in the URL) and hands
 * it to this island as `initialEvents`/`initialCursor`; from there "Load more"
 * appends pages client-side without a full navigation. Filter changes go the
 * other way, through the URL: the form navigates, the server re-renders page
 * one, and the contract page remounts this panel (keyed by the query string) so
 * the accumulated pages reset to the fresh first page.
 */

const PAGE_SIZE = 25;
const GENERIC_ERROR = 'The request could not be completed';

interface EventsPanelProps {
  contractId: string;
  /** The filters already applied server-side, mirrored into the form controls. */
  filters: EventFilters;
  /** The server-rendered first page. */
  initialEvents: readonly EventNode[];
  /** The cursor to the second page, or `null` when the first page is the last. */
  initialCursor: string | null;
}

export function EventsPanel({
  contractId,
  filters,
  initialEvents,
  initialCursor,
}: EventsPanelProps) {
  const [events, setEvents] = useState<readonly EventNode[]>(initialEvents);
  const [cursor, setCursor] = useState<string | null>(initialCursor);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [announce, setAnnounce] = useState('');

  const controllerRef = useRef<AbortController | null>(null);
  useEffect(() => () => controllerRef.current?.abort(), []);

  async function loadMore(): Promise<void> {
    if (cursor === null || loading) return;
    setLoading(true);
    setError(null);
    setAnnounce('Loading more events…');

    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;

    try {
      const page = await fetchEventsPage(
        { contractId, ...filters, cursor, limit: PAGE_SIZE },
        { signal: controller.signal },
      );
      setEvents((current) => [...current, ...page.items]);
      setCursor(page.nextCursor);
      const n = page.items.length;
      setAnnounce(`${n} more ${n === 1 ? 'event' : 'events'} loaded`);
    } catch (cause) {
      setError(cause instanceof EventFetchError ? cause.message : GENERIC_ERROR);
      setAnnounce('');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <EventFiltersForm filters={filters} />

      <EventList events={events} />

      <p role="status" aria-live="polite" className="sr-only">
        {announce}
      </p>

      {error !== null && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}

      {cursor !== null && (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="outline"
            onClick={() => void loadMore()}
            disabled={loading}
            aria-busy={loading}
          >
            {loading ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}
    </div>
  );
}
