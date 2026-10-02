import { describe, expect, it } from 'vitest';

import { EVENT_ID_PATTERN, eventJsonFilename, isEventId } from '@/lib/event-id';

describe('isEventId', () => {
  it('accepts a string of digits', () => {
    expect(isEventId('0')).toBe(true);
    expect(isEventId('42')).toBe(true);
    expect(isEventId('9007199254740993')).toBe(true); // past Number.MAX_SAFE_INTEGER
  });

  it('rejects a non-digit string', () => {
    expect(isEventId('')).toBe(false);
    expect(isEventId('4x2')).toBe(false);
    expect(isEventId('-1')).toBe(false);
    expect(isEventId('1.5')).toBe(false);
    expect(isEventId(' 42 ')).toBe(false);
    // A tx hash is a common wrong paste; it is not an event id.
    expect(isEventId('abcdef0123456789')).toBe(false);
  });

  it('exposes the pattern used for validation', () => {
    expect(EVENT_ID_PATTERN.test('123')).toBe(true);
    expect(EVENT_ID_PATTERN.test('12a')).toBe(false);
  });
});

describe('eventJsonFilename', () => {
  it('composes the id into the filename for a digit id', () => {
    expect(eventJsonFilename('42')).toBe('event-42.json');
  });

  it('falls back to a neutral name for a non-digit id', () => {
    expect(eventJsonFilename('../../etc/passwd')).toBe('event.json');
    expect(eventJsonFilename('<script>')).toBe('event.json');
    expect(eventJsonFilename('')).toBe('event.json');
  });
});
