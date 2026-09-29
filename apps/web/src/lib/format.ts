/**
 * Presentation-only formatting helpers, shared by the explorer's screens.
 *
 * These are pure and dependency-free: they take already-validated data (the
 * boundary schemas have run) and turn it into the strings the UI shows. Keeping
 * them here, rather than inline in components, is what lets them be unit-tested
 * while the Server Components that use them stay coverage-excluded.
 */

/** Fixed UTC formatter: the indexer's timestamps are absolute, so we show them so. */
const TIMESTAMP_FORMAT = new Intl.DateTimeFormat('en-US', {
  timeZone: 'UTC',
  year: 'numeric',
  month: 'short',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
});

/**
 * Render an ISO 8601 timestamp as a stable UTC string, e.g.
 * `Sep 29, 2026, 14:03:21 UTC`. An unparseable value is returned unchanged so a
 * surprising input degrades to visible raw data rather than `Invalid Date`.
 */
export function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return `${TIMESTAMP_FORMAT.format(date)} UTC`;
}

/** Group a non-negative integer with thousands separators: `1234567` to `1,234,567`. */
export function formatInteger(value: number): string {
  return value.toLocaleString('en-US');
}

/**
 * Shorten a long opaque identifier for inline display, keeping its ends:
 * `CDNWTV…CUYYWI5L`. Values already short enough are returned unchanged. Callers
 * that need the full value (copy, links) use the original, not this.
 */
export function truncateMiddle(value: string, head = 6, tail = 6): string {
  if (value.length <= head + tail + 1) return value;
  return `${value.slice(0, head)}…${value.slice(-tail)}`;
}
