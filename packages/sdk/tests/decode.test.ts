import { Address, nativeToScVal, xdr } from '@stellar/stellar-sdk';
import { describe, expect, it } from 'vitest';

import { decodeScVal, decodeTopics, eventNameFromTopics } from '../src/decode.js';
import { DecodedValueSchema } from '../src/types.js';

const CONTRACT = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

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

  it('decodes a map with symbol keys', () => {
    const input = nativeToScVal({ amount: 5 }, { type: { amount: ['symbol', 'u32'] } });
    expect(decodeScVal(input)).toEqual({
      type: 'map',
      value: { amount: { type: 'u32', value: 5 } },
    });
  });

  it('decodes an empty map', () => {
    expect(decodeScVal(xdr.ScVal.scvMap([]))).toEqual({ type: 'map', value: {} });
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
    expect(decodeScVal(stub)).toEqual({ type: 'map', value: {} });
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
