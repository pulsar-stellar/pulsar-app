/**
 * The event-list filters, and their round-trip with the URL query string.
 *
 * The filters live in the URL so a filtered view is shareable and survives the
 * back button (the "URL state" the event list is built around). These helpers
 * are the single place that maps between the query string and the typed filter
 * object, kept pure so both the Server Component (reading `searchParams`) and
 * the client form (reading `useSearchParams`) use the same rules.
 *
 * Business validation stays with the indexer, the single authority (ADR-046):
 * these helpers only coerce transport types and drop values that could never be
 * valid (a non-integer ledger, an unknown order), so a malformed query string
 * degrades to a looser view rather than a failed request.
 */

/** A parsed set of event filters. Every field is optional; absent means unfiltered. */
export interface EventFilters {
  name?: string;
  fromLedger?: number;
  toLedger?: number;
  topicContains?: string;
  order?: string;
}

/** The query-string keys, kept short. `topicContains` shortens to `topic`. */
const KEYS = {
  name: 'name',
  topic: 'topic',
  from: 'from',
  to: 'to',
  order: 'order',
} as const;

const ORDER_VALUES = new Set(['asc', 'desc']);

/** A non-negative integer, or `undefined` for anything a ledger bound cannot be. */
function toLedger(raw: string | null): number | undefined {
  if (raw === null || raw.trim() === '') return undefined;
  const value = Number(raw);
  return Number.isInteger(value) && value >= 0 ? value : undefined;
}

/**
 * Read filters from a URL query string.
 *
 * Accepts a {@link URLSearchParams}, so it serves both the server (built from
 * the route's `searchParams`) and the client (`useSearchParams`).
 */
export function readEventFilters(params: URLSearchParams): EventFilters {
  const filters: EventFilters = {};

  const name = params.get(KEYS.name)?.trim();
  if (name) filters.name = name;

  const topic = params.get(KEYS.topic)?.trim();
  if (topic) filters.topicContains = topic;

  const fromLedger = toLedger(params.get(KEYS.from));
  if (fromLedger !== undefined) filters.fromLedger = fromLedger;

  const toLedgerValue = toLedger(params.get(KEYS.to));
  if (toLedgerValue !== undefined) filters.toLedger = toLedgerValue;

  const order = params.get(KEYS.order);
  if (order && ORDER_VALUES.has(order)) filters.order = order;

  return filters;
}

/**
 * Serialize filters back to a query string, in a stable key order and omitting
 * empty fields, so the same filters always produce the same URL.
 */
export function eventFiltersToQuery(filters: EventFilters): string {
  const params = new URLSearchParams();
  if (filters.name) params.set(KEYS.name, filters.name);
  if (filters.topicContains) params.set(KEYS.topic, filters.topicContains);
  if (filters.fromLedger !== undefined) {
    params.set(KEYS.from, String(filters.fromLedger));
  }
  if (filters.toLedger !== undefined) {
    params.set(KEYS.to, String(filters.toLedger));
  }
  if (filters.order) params.set(KEYS.order, filters.order);
  return params.toString();
}
