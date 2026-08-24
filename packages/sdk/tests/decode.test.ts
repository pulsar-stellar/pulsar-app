import { Address, nativeToScVal, xdr } from '@stellar/stellar-sdk';
import { describe, expect, it } from 'vitest';

import { decodeScVal, decodeTopics, eventNameFromTopics } from '../src/decode.js';
import { DecodedValueSchema } from '../src/types.js';

const CONTRACT = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

const symbolKey = (text: string): xdr.ScVal => nativeToScVal(text, { type: 'symbol' });
const u32 = (n: number): xdr.ScVal => nativeToScVal(n, { type: 'u32' });

/**
 * Builds a map from raw entries. `nativeToScVal` cannot express a non-string
 * key, a duplicate key, or a deliberate ordering, all of which these tests need.
 */
const mapOf = (entries: ReadonlyArray<readonly [xdr.ScVal, xdr.ScVal]>): xdr.ScVal =>
  xdr.ScVal.scvMap(entries.map(([key, val]) => new xdr.ScMapEntry({ key, val })));

describe('scalar variants', () => {
  it.each([
    ['a symbol', nativeToScVal('deposit', { type: 'symbol' }), { type: 'symbol', value: 'deposit' }],
    ['a string', nativeToScVal('hello', { type: 'string' }), { type: 'string', value: 'hello' }],
    ['a true bool', nativeToScVal(true), { type: 'bool', value: true }],
    ['a false bool', nativeToScVal(false), { type: 'bool', value: false }],
    ['void', xdr.ScVal.scvVoid(), { type: 'void' }],
    ['a contract address', new Address(CONTRACT).toScVal(), { type: 'address', value: CONTRACT }],
  ])('decodes %s', (_label, input, expected) => {
    expect(decodeScVal(input)).toEqual(expected);
  });

  it('decodes bytes as hex, not base64', () => {
    expect(decodeScVal(nativeToScVal(Buffer.from('beef', 'hex')))).toEqual({
      type: 'bytes',
      value: 'beef',
    });
  });
});

describe('integers', () => {
  it.each([
    ['u32', nativeToScVal(7, { type: 'u32' }), { type: 'u32', value: 7 }],
    ['i32', nativeToScVal(-7, { type: 'i32' }), { type: 'i32', value: -7 }],
  ])('keeps %s as a number, since it always fits', (_label, input, expected) => {
    expect(decodeScVal(input)).toEqual(expected);
  });

  it.each([
    ['u64', nativeToScVal(2n ** 63n, { type: 'u64' }), 'u64', String(2n ** 63n)],
    ['i64', nativeToScVal(-(2n ** 62n), { type: 'i64' }), 'i64', String(-(2n ** 62n))],
    ['u128', nativeToScVal(2n ** 100n, { type: 'u128' }), 'u128', String(2n ** 100n)],
    ['i128', nativeToScVal(-(2n ** 100n), { type: 'i128' }), 'i128', String(-(2n ** 100n))],
    ['u256', nativeToScVal(2n ** 200n, { type: 'u256' }), 'u256', String(2n ** 200n)],
    ['i256', nativeToScVal(-(2n ** 200n), { type: 'i256' }), 'i256', String(-(2n ** 200n))],
    ['timepoint', nativeToScVal(1_724_500_000n, { type: 'timepoint' }), 'timepoint', '1724500000'],
    ['duration', nativeToScVal(3600n, { type: 'duration' }), 'duration', '3600'],
  ])('renders %s as a string so no precision is lost', (_label, input, type, value) => {
    expect(decodeScVal(input)).toEqual({ type, value });
  });

  it('keeps a u128 exact past what a JSON number could hold', () => {
    const huge = 2n ** 100n + 1n;
    const decoded = decodeScVal(nativeToScVal(huge, { type: 'u128' }));
    expect(decoded).toEqual({ type: 'u128', value: '1267650600228229401496703205377' });
    if (decoded.type === 'u128') {
      expect(BigInt(decoded.value)).toBe(huge);
      expect(String(Number(decoded.value))).not.toBe(decoded.value);
    }
  });
});

describe('containers', () => {
  it('decodes a vector element by element', () => {
    expect(decodeScVal(nativeToScVal([1, 2], { type: 'u32' }))).toEqual({
      type: 'vec',
      value: [
        { type: 'u32', value: 1 },
        { type: 'u32', value: 2 },
      ],
    });
  });

  it('decodes an empty vector', () => {
    expect(decodeScVal(xdr.ScVal.scvVec([]))).toEqual({ type: 'vec', value: [] });
  });

  it('decodes a map as an ordered array of key/value pairs', () => {
    const input = nativeToScVal({ amount: 5 }, { type: { amount: ['symbol', 'u32'] } });
    expect(decodeScVal(input)).toEqual({
      type: 'map',
      value: [{ key: { type: 'symbol', value: 'amount' }, value: { type: 'u32', value: 5 } }],
    });
  });

  it('decodes an empty map', () => {
    expect(decodeScVal(xdr.ScVal.scvMap([]))).toEqual({ type: 'map', value: [] });
  });

  it('recurses through nested containers', () => {
    const inner = nativeToScVal([1], { type: 'u32' });
    expect(decodeScVal(xdr.ScVal.scvVec([inner]))).toEqual({
      type: 'vec',
      value: [{ type: 'vec', value: [{ type: 'u32', value: 1 }] }],
    });
  });

  it('emits vec for a tuple, since XDR cannot tell them apart', () => {
    const tuple = nativeToScVal(['a', 1], { type: ['symbol', 'u32'] });
    expect(decodeScVal(tuple).type).toBe('vec');
  });
});

describe('variants this SDK version cannot name', () => {
  it('degrades an ScVal error to unknown, carrying its XDR', () => {
    const value = xdr.ScVal.scvError(xdr.ScError.sceContract(1));
    const decoded = decodeScVal(value);
    expect(decoded.type).toBe('unknown');
    if (decoded.type === 'unknown') {
      expect(decoded.xdr).toBe(value.toXDR('base64'));
    }
  });

  it('degrades a ledger key nonce to unknown', () => {
    const value = xdr.ScVal.scvLedgerKeyContractInstance();
    expect(decodeScVal(value).type).toBe('unknown');
  });

  it('keeps one unknown value from spoiling the container around it', () => {
    const value = xdr.ScVal.scvVec([
      nativeToScVal(1, { type: 'u32' }),
      xdr.ScVal.scvLedgerKeyContractInstance(),
    ]);
    const decoded = decodeScVal(value);
    expect(decoded).toMatchObject({ type: 'vec' });
    if (decoded.type === 'vec') {
      expect(decoded.value[0]).toEqual({ type: 'u32', value: 1 });
      expect(decoded.value[1]?.type).toBe('unknown');
    }
  });
});

describe('a value that fails while decoding', () => {
  /**
   * Corrupt XDR from a server is the case this covers: the variant is one we
   * handle, and reading its payload throws anyway. Degrading keeps one bad
   * value from discarding every other event in the response.
   *
   * Constructed by hand because a real ScVal cannot be put into this state.
   */
  const corruptBytes = {
    switch: () => ({ name: 'scvBytes' }),
    bytes: () => {
      throw new Error('corrupt payload');
    },
    toXDR: () => 'AAAABA==',
  } as unknown as xdr.ScVal;

  it('degrades to unknown instead of throwing', () => {
    const decoded = decodeScVal(corruptBytes);
    expect(decoded).toEqual({ type: 'unknown', xdr: 'AAAABA==' });
  });

  it('degrades inside a container without losing its siblings', () => {
    const value = xdr.ScVal.scvVec([nativeToScVal(1, { type: 'u32' })]);
    const mixed = {
      switch: () => ({ name: 'scvVec' }),
      vec: () => [nativeToScVal(1, { type: 'u32' }), corruptBytes],
      toXDR: () => value.toXDR('base64'),
    } as unknown as xdr.ScVal;

    const decoded = decodeScVal(mixed);
    expect(decoded).toMatchObject({ type: 'vec' });
    if (decoded.type === 'vec') {
      expect(decoded.value[0]).toEqual({ type: 'u32', value: 1 });
      expect(decoded.value[1]?.type).toBe('unknown');
    }
  });
});

describe('containers the SDK types as possibly absent', () => {
  /**
   * `vec()` and `map()` are typed as optional even after switching on the
   * variant, so the decoder guards them. These pin what the guard does rather
   * than leaving it as untested defensive code.
   */
  it('treats a vec with no contents as empty', () => {
    const stub = {
      switch: () => ({ name: 'scvVec' }),
      vec: () => undefined,
      toXDR: () => 'AAAAEA==',
    } as unknown as xdr.ScVal;
    expect(decodeScVal(stub)).toEqual({ type: 'vec', value: [] });
  });

  it('treats a map with no entries as empty', () => {
    const stub = {
      switch: () => ({ name: 'scvMap' }),
      map: () => undefined,
      toXDR: () => 'AAAAEQ==',
    } as unknown as xdr.ScVal;
    expect(decodeScVal(stub)).toEqual({ type: 'map', value: [] });
  });
});

/**
 * ADR-023 carries a map as an ordered array rather than as an object or a
 * JavaScript `Map`. These pin the three things that choice exists to preserve:
 * the key's own type, the wire ordering, and duplicate keys. `scValToNative`
 * loses all three, so none of this is theoretical.
 */
describe('map fidelity', () => {
  it('keeps each key at its own type rather than rendering it to a string', () => {
    const decoded = decodeScVal(
      mapOf([
        [symbolKey('sym'), u32(1)],
        [new Address(CONTRACT).toScVal(), u32(2)],
        [nativeToScVal(3n, { type: 'i128' }), u32(3)],
      ]),
    );

    expect(decoded).toEqual({
      type: 'map',
      value: [
        { key: { type: 'symbol', value: 'sym' }, value: { type: 'u32', value: 1 } },
        { key: { type: 'address', value: CONTRACT }, value: { type: 'u32', value: 2 } },
        { key: { type: 'i128', value: '3' }, value: { type: 'u32', value: 3 } },
      ],
    });
  });

  it('distinguishes a symbol key from a string key of the same text', () => {
    const decoded = decodeScVal(
      mapOf([
        [symbolKey('admin'), u32(1)],
        [nativeToScVal('admin', { type: 'string' }), u32(2)],
      ]),
    );

    expect(decoded.type).toBe('map');
    if (decoded.type !== 'map') return;
    expect(decoded.value.map((entry) => entry.key.type)).toEqual(['symbol', 'string']);
    expect(decoded.value).toHaveLength(2);
  });

  it('keeps both entries when a key appears twice', () => {
    const decoded = decodeScVal(
      mapOf([
        [symbolKey('a'), u32(1)],
        [symbolKey('a'), u32(2)],
      ]),
    );

    expect(decoded).toEqual({
      type: 'map',
      value: [
        { key: { type: 'symbol', value: 'a' }, value: { type: 'u32', value: 1 } },
        { key: { type: 'symbol', value: 'a' }, value: { type: 'u32', value: 2 } },
      ],
    });
  });

  it('preserves ordering across an XDR encode and decode cycle', () => {
    const input = mapOf([
      [symbolKey('c'), u32(3)],
      [symbolKey('a'), u32(1)],
      [symbolKey('b'), u32(2)],
    ]);
    const reencoded = xdr.ScVal.fromXDR(input.toXDR('base64'), 'base64');

    expect(decodeScVal(reencoded)).toEqual(decodeScVal(input));
  });

  it('survives JSON.stringify with its structure intact', () => {
    const decoded = decodeScVal(
      mapOf([
        [symbolKey('amount'), nativeToScVal(7n, { type: 'i128' })],
        [new Address(CONTRACT).toScVal(), u32(1)],
      ]),
    );

    expect(JSON.parse(JSON.stringify(decoded))).toEqual(decoded);
  });
});

/**
 * `contractInstance` is left to the fallback for v0.1 per ADR-023. Its shape is
 * not observable in any event this project has seen, and `scValToNative` hands
 * back the raw XDR struct rather than a native value, so a typed variant would
 * be a guess. The consumer gets the base64 and can decode it themselves.
 */
describe('contractInstance falls back', () => {
  const instance = xdr.ScVal.scvContractInstance(
    new xdr.ScContractInstance({
      executable: xdr.ContractExecutable.contractExecutableStellarAsset(),
      storage: null,
    }),
  );

  it('decodes to unknown rather than to a guessed shape', () => {
    expect(decodeScVal(instance)).toEqual({ type: 'unknown', xdr: instance.toXDR('base64') });
  });

  it('carries XDR a caller can decode back to the original value', () => {
    const decoded = decodeScVal(instance);
    expect(decoded.type).toBe('unknown');
    if (decoded.type !== 'unknown') return;
    expect(xdr.ScVal.fromXDR(decoded.xdr, 'base64').switch().name).toBe('scvContractInstance');
  });

  it('is carried through a topic list without disturbing its neighbours', () => {
    expect(decodeTopics([symbolKey('upgrade'), instance])).toEqual([
      { type: 'symbol', value: 'upgrade' },
      { type: 'unknown', xdr: instance.toXDR('base64') },
    ]);
  });
});

describe('every decoded value satisfies the schema', () => {
  it.each([
    ['symbol', nativeToScVal('x', { type: 'symbol' })],
    ['u32', nativeToScVal(1, { type: 'u32' })],
    ['i256', nativeToScVal(-1n, { type: 'i256' })],
    ['bytes', nativeToScVal(Buffer.from('00', 'hex'))],
    ['address', new Address(CONTRACT).toScVal()],
    ['vec', nativeToScVal([1], { type: 'u32' })],
    ['map', mapOf([[symbolKey('k'), u32(1)]])],
    ['void', xdr.ScVal.scvVoid()],
    ['unknown', xdr.ScVal.scvLedgerKeyContractInstance()],
  ])('%s parses against DecodedValueSchema', (_label, input) => {
    expect(DecodedValueSchema.safeParse(decodeScVal(input)).success).toBe(true);
  });
});

describe('topics and event names', () => {
  it('decodes topics in order', () => {
    const topics = [nativeToScVal('deposit', { type: 'symbol' }), new Address(CONTRACT).toScVal()];
    expect(decodeTopics(topics)).toEqual([
      { type: 'symbol', value: 'deposit' },
      { type: 'address', value: CONTRACT },
    ]);
  });

  it('reads the event name from the leading symbol topic', () => {
    expect(eventNameFromTopics(decodeTopics([nativeToScVal('deposit', { type: 'symbol' })]))).toBe(
      'deposit',
    );
  });

  it('returns an empty name when the leading topic is not a symbol', () => {
    expect(eventNameFromTopics(decodeTopics([nativeToScVal(1, { type: 'u32' })]))).toBe('');
  });

  it('returns an empty name when there are no topics', () => {
    expect(eventNameFromTopics([])).toBe('');
  });
});
