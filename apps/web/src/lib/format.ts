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

/** Whether a decoded wide-integer string is a plain run of decimal digits. */
const DIGITS = /^\d+$/;

/**
 * Derive a UTC date from a Soroban `timepoint`: an unsigned count of seconds
 * since the Unix epoch, carried as a string to preserve precision (ADR-023).
 *
 * Returns the formatted date, or `null` when the value is not a safe,
 * in-range second count, so the renderer shows the raw value alone rather than
 * a misleading or `Invalid Date` hint. This is a convenience gloss beside the
 * raw seconds, never a replacement for them.
 */
export function formatUnixSeconds(value: string): string | null {
  if (!DIGITS.test(value)) return null;
  const seconds = Number(value);
  if (!Number.isSafeInteger(seconds)) return null;
  const date = new Date(seconds * 1000);
  if (Number.isNaN(date.getTime())) return null;
  return `${TIMESTAMP_FORMAT.format(date)} UTC`;
}

/**
 * Render a Soroban `duration` (a span in whole seconds, carried as a string) as
 * a compact `1d 2h 3m 4s` gloss, omitting zero units and showing `0s` for zero.
 *
 * Returns `null` when the value is not a safe digit string, so the renderer
 * falls back to the raw seconds. A convenience beside the raw value, not a
 * replacement.
 */
export function formatDurationSeconds(value: string): string | null {
  if (!DIGITS.test(value)) return null;
  const total = Number(value);
  if (!Number.isSafeInteger(total)) return null;
  const days = Math.floor(total / 86_400);
  const hours = Math.floor((total % 86_400) / 3_600);
  const minutes = Math.floor((total % 3_600) / 60);
  const seconds = total % 60;
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (seconds > 0) parts.push(`${seconds}s`);
  return parts.length > 0 ? parts.join(' ') : '0s';
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
