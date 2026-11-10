import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { xdr } from '@stellar/stellar-sdk';
import { describe, expect, it } from 'vitest';

import { decodeScVal } from '../src/decode.js';

// The corpus the Go decoder and pulsar-core's Rust decoder are tested against.
// Reading it in place, rather than copying it, keeps the three decoders checked
// against the same bytes and the same expected JSON (ADR-023, ADR-051).
const FIXTURES = fileURLToPath(
  new URL('../../../indexer/internal/decoder/testdata/fixtures/', import.meta.url),
);

const names = readdirSync(FIXTURES)
  .filter((file) => file.endsWith('.xdr'))
  .map((file) => file.slice(0, -'.xdr'.length))
  .sort();

describe('shared decoder fixtures', () => {
  it('finds the full corpus', () => {
    // A wrong path would otherwise pass by iterating nothing.
    expect(names.length).toBeGreaterThanOrEqual(32);
  });

  it.each(names)('%s decodes to its expected JSON', (name) => {
    const value = xdr.ScVal.fromXDR(readFileSync(`${FIXTURES}${name}.xdr`));
    const expected: unknown = JSON.parse(readFileSync(`${FIXTURES}${name}.json`, 'utf8'));

    expect(decodeScVal(value)).toEqual(expected);
  });
});
