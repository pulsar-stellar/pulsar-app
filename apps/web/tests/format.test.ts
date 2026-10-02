import { describe, expect, it } from 'vitest';

import {
  formatDurationSeconds,
  formatInteger,
  formatTimestamp,
  formatUnixSeconds,
  truncateMiddle,
} from '@/lib/format';

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

describe('formatUnixSeconds', () => {
  it('derives a UTC date from a second count', () => {
    // 1609459200 = 2021-01-01T00:00:00Z
    expect(formatUnixSeconds('1609459200')).toBe('Jan 01, 2021, 00:00:00 UTC');
  });

  it('handles the epoch', () => {
    expect(formatUnixSeconds('0')).toBe('Jan 01, 1970, 00:00:00 UTC');
  });

  it('returns null for a non-digit or unsafe value', () => {
    expect(formatUnixSeconds('not-a-number')).toBeNull();
    expect(formatUnixSeconds('')).toBeNull();
    // Past the safe-integer range for seconds, so no trustworthy date.
    expect(formatUnixSeconds('99999999999999999999')).toBeNull();
  });
});

describe('formatDurationSeconds', () => {
  it('renders a compact span', () => {
    expect(formatDurationSeconds('0')).toBe('0s');
    expect(formatDurationSeconds('45')).toBe('45s');
    expect(formatDurationSeconds('90')).toBe('1m 30s');
    expect(formatDurationSeconds('3661')).toBe('1h 1m 1s');
    expect(formatDurationSeconds('90061')).toBe('1d 1h 1m 1s');
  });

  it('returns null for a non-digit or unsafe value', () => {
    expect(formatDurationSeconds('abc')).toBeNull();
    expect(formatDurationSeconds('')).toBeNull();
    expect(formatDurationSeconds('99999999999999999999')).toBeNull();
  });
});
