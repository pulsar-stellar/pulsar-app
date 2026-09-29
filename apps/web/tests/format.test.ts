import { describe, expect, it } from 'vitest';

import { formatInteger, formatTimestamp, truncateMiddle } from '@/lib/format';

describe('formatTimestamp', () => {
  it('renders an ISO timestamp as a stable UTC string', () => {
    expect(formatTimestamp('2026-09-29T14:03:21Z')).toBe(
      'Sep 29, 2026, 14:03:21 UTC',
    );
  });

  it('normalizes a zoned offset to UTC', () => {
    expect(formatTimestamp('2026-09-29T16:03:21+02:00')).toBe(
      'Sep 29, 2026, 14:03:21 UTC',
    );
  });

  it('returns an unparseable value unchanged', () => {
    expect(formatTimestamp('not-a-date')).toBe('not-a-date');
  });
});

describe('formatInteger', () => {
  it('groups thousands', () => {
    expect(formatInteger(1234567)).toBe('1,234,567');
  });

  it('leaves small values alone', () => {
    expect(formatInteger(42)).toBe('42');
  });
});

describe('truncateMiddle', () => {
  const VALID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

  it('keeps the ends of a long value around an ellipsis', () => {
    expect(truncateMiddle(VALID)).toBe('CDNWTV…YYWI5L');
  });

  it('honors custom head and tail lengths', () => {
    expect(truncateMiddle(VALID, 4, 4)).toBe('CDNW…WI5L');
  });

  it('returns a value that is already short enough unchanged', () => {
    expect(truncateMiddle('CDNWTV')).toBe('CDNWTV');
  });
});
