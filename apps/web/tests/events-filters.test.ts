import { describe, expect, it } from 'vitest';

import { eventFiltersToQuery, readEventFilters } from '@/lib/events-filters';

describe('readEventFilters', () => {
  it('reads every filter from the query string', () => {
    const params = new URLSearchParams(
      'name=transfer&topic=abc&from=100&to=200&order=asc',
    );
    expect(readEventFilters(params)).toEqual({
      name: 'transfer',
      topicContains: 'abc',
      fromLedger: 100,
      toLedger: 200,
      order: 'asc',
    });
  });

  it('returns an empty object for no query', () => {
    expect(readEventFilters(new URLSearchParams(''))).toEqual({});
  });

  it('drops a non-integer or negative ledger bound', () => {
    const params = new URLSearchParams('from=1.5&to=-3');
    expect(readEventFilters(params)).toEqual({});
  });

  it('drops an unknown order value', () => {
    expect(readEventFilters(new URLSearchParams('order=up'))).toEqual({});
  });

  it('ignores blank text filters', () => {
    expect(readEventFilters(new URLSearchParams('name=%20&topic='))).toEqual({});
  });
});

describe('eventFiltersToQuery', () => {
  it('serializes filters in a stable key order', () => {
    const query = eventFiltersToQuery({
      order: 'asc',
      toLedger: 200,
      name: 'transfer',
      fromLedger: 100,
      topicContains: 'abc',
    });
    expect(query).toBe('name=transfer&topic=abc&from=100&to=200&order=asc');
  });

  it('omits empty fields', () => {
    expect(eventFiltersToQuery({ name: 'transfer' })).toBe('name=transfer');
    expect(eventFiltersToQuery({})).toBe('');
  });

  it('round-trips through readEventFilters', () => {
    const filters = {
      name: 'transfer',
      topicContains: 'abc',
      fromLedger: 100,
      toLedger: 200,
      order: 'asc',
    };
    expect(readEventFilters(new URLSearchParams(eventFiltersToQuery(filters)))).toEqual(
      filters,
    );
  });
});
