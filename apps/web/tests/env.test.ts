import { afterEach, describe, expect, it } from 'vitest';

import { getIndexerBaseUrl, IndexerUrlError } from '@/lib/env';

const KEY = 'NEXT_PUBLIC_PULSAR_INDEXER_URL';

describe('getIndexerBaseUrl', () => {
  const original = process.env[KEY];

  afterEach(() => {
    if (original === undefined) {
      delete process.env[KEY];
    } else {
      process.env[KEY] = original;
    }
  });

  it('returns a valid http URL with no trailing slash', () => {
    process.env[KEY] = 'http://localhost:8080';
    expect(getIndexerBaseUrl()).toBe('http://localhost:8080');
  });

  it('strips a trailing slash so path joins stay predictable', () => {
    process.env[KEY] = 'https://indexer.example.com/';
    expect(getIndexerBaseUrl()).toBe('https://indexer.example.com');
  });

  it('throws a clear error when the variable is missing', () => {
    delete process.env[KEY];
    expect(() => getIndexerBaseUrl()).toThrow(IndexerUrlError);
    expect(() => getIndexerBaseUrl()).toThrow(/NEXT_PUBLIC_PULSAR_INDEXER_URL/);
  });

  it('throws when the value is not a valid absolute URL', () => {
    process.env[KEY] = 'not-a-url';
    expect(() => getIndexerBaseUrl()).toThrow(IndexerUrlError);
  });

  it('rejects a non-http(s) scheme', () => {
    process.env[KEY] = 'ftp://indexer.example.com';
    expect(() => getIndexerBaseUrl()).toThrow(IndexerUrlError);
  });
});
