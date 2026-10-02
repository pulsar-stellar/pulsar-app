'use client';

import { usePathname, useRouter } from 'next/navigation';
import { useId, useState, type FormEvent } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { eventFiltersToQuery, type EventFilters } from '@/lib/events-filters';

/**
 * The event-list filter controls.
 *
 * The URL query string is the source of truth for the filtered view (shareable,
 * survives the back button), so this form does not fetch or hold results: on
 * submit it serializes the entered filters and navigates, letting the contract
 * page re-render its first page against the new query. Seeded from the filters
 * the page parsed out of the URL, so a shared link shows its own controls.
 *
 * "Newest first" is the default order and is left out of the URL to keep a
 * shared link canonical; only the non-default "oldest first" is serialized.
 */

interface EventFiltersFormProps {
  /** The filters currently reflected in the URL, used to seed the inputs. */
  filters: EventFilters;
}

/** A non-negative integer parsed from an input, or `undefined` for anything else. */
function toLedger(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (trimmed === '') return undefined;
  const value = Number(trimmed);
  return Number.isInteger(value) && value >= 0 ? value : undefined;
}

export function EventFiltersForm({ filters }: EventFiltersFormProps) {
  const router = useRouter();
  const pathname = usePathname();
  const ids = useId();

  const [name, setName] = useState(filters.name ?? '');
  const [topic, setTopic] = useState(filters.topicContains ?? '');
  const [from, setFrom] = useState(
    filters.fromLedger !== undefined ? String(filters.fromLedger) : '',
  );
  const [to, setTo] = useState(
    filters.toLedger !== undefined ? String(filters.toLedger) : '',
  );
  const [order, setOrder] = useState(filters.order === 'asc' ? 'asc' : 'desc');

  function navigate(query: string): void {
    router.push(query ? `${pathname}?${query}` : pathname);
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const next: EventFilters = {};
    const trimmedName = name.trim();
    if (trimmedName) next.name = trimmedName;
    const trimmedTopic = topic.trim();
    if (trimmedTopic) next.topicContains = trimmedTopic;
    const fromLedger = toLedger(from);
    if (fromLedger !== undefined) next.fromLedger = fromLedger;
    const toLedgerValue = toLedger(to);
    if (toLedgerValue !== undefined) next.toLedger = toLedgerValue;
    // The default "newest first" stays out of the URL; only "oldest" travels.
    if (order === 'asc') next.order = 'asc';
    navigate(eventFiltersToQuery(next));
  }

  function handleClear(): void {
    setName('');
    setTopic('');
    setFrom('');
    setTo('');
    setOrder('desc');
    navigate('');
  }

  return (
    <form
      onSubmit={handleSubmit}
      aria-label="Event filters"
      className="flex flex-col gap-3 rounded-md border border-border p-4"
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-name`} className="text-xs font-medium text-muted-foreground">
            Event name
          </label>
          <Input
            id={`${ids}-name`}
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="transfer"
            autoComplete="off"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-topic`} className="text-xs font-medium text-muted-foreground">
            Topic contains
          </label>
          <Input
            id={`${ids}-topic`}
            value={topic}
            onChange={(event) => setTopic(event.target.value)}
            placeholder="address, symbol, ..."
            autoComplete="off"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-from`} className="text-xs font-medium text-muted-foreground">
            From ledger
          </label>
          <Input
            id={`${ids}-from`}
            type="number"
            min={0}
            step={1}
            inputMode="numeric"
            aria-describedby={`${ids}-ledger-hint`}
            value={from}
            onChange={(event) => setFrom(event.target.value)}
            placeholder="0"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-to`} className="text-xs font-medium text-muted-foreground">
            To ledger
          </label>
          <Input
            id={`${ids}-to`}
            type="number"
            min={0}
            step={1}
            inputMode="numeric"
            aria-describedby={`${ids}-ledger-hint`}
            value={to}
            onChange={(event) => setTo(event.target.value)}
            placeholder="latest"
          />
        </div>
      </div>
      <p id={`${ids}-ledger-hint`} className="text-xs text-muted-foreground">
        Ledger bounds are non-negative integers; invalid values are ignored.
      </p>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-order`} className="text-xs font-medium text-muted-foreground">
            Order
          </label>
          <select
            id={`${ids}-order`}
            value={order}
            onChange={(event) => setOrder(event.target.value)}
            className="h-10 rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            <option value="desc">Newest first</option>
            <option value="asc">Oldest first</option>
          </select>
        </div>
        <div className="flex items-center gap-2">
          <Button type="button" variant="ghost" onClick={handleClear}>
            Clear
          </Button>
          <Button type="submit">Apply filters</Button>
        </div>
      </div>
    </form>
  );
}
