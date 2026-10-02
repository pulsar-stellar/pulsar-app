/**
 * Client-safe event ID validation.
 *
 * Mirrors {@link file://./contract-id.ts}: the explorer validates an event id's
 * shape without reaching the SDK's `EventIdSchema`, which would drag the SDK
 * (and `@stellar/stellar-sdk`) into any client bundle that imported it, the
 * boundary ADR-046 keeps closed. The detail route uses this to reject a
 * malformed id with a not-found before any request goes out.
 *
 * It is a pre-check, not the authority: an id that reaches the data layer is
 * validated again server-side (see `assertEventId` in the GraphQL query
 * wrappers), which returns `VALIDATION_EVENT_ID`. The event id is a `BIGSERIAL`
 * carried as a decimal string (ADR-021), so it is validated as digits rather
 * than parsed into a number it could outgrow. Keep this in step with the SDK's
 * `/^\d+$/`.
 */

/** An indexer event id: one or more decimal digits. */
export const EVENT_ID_PATTERN = /^\d+$/;

/** Whether `value` is a well-formed event id, exactly as the SDK checks. */
export function isEventId(value: string): boolean {
  return EVENT_ID_PATTERN.test(value);
}

/**
 * The filename offered for an event's JSON export.
 *
 * The id normally comes from a validated route param, but the download uses the
 * id on the fetched event, which the boundary schema constrains only to a
 * non-empty string. A misbehaving indexer could return odd characters, so the
 * id is composed into the filename only when it is a plain digit string, and a
 * neutral `event.json` is used otherwise (defense in depth; a browser also
 * sanitizes a download filename, so this is belt-and-braces).
 */
export function eventJsonFilename(id: string): string {
  return isEventId(id) ? `event-${id}.json` : 'event.json';
}
