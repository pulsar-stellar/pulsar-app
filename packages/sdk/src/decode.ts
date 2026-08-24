/**
 * Turns Soroban `ScVal` values into {@link DecodedValue}.
 *
 * This is the RPC path's decoder. The indexer decodes on its own side and
 * serves the same shape, so a consumer sees one representation whichever
 * source an event came from.
 *
 * Three rules govern the mapping, all from ADR-023. Anything wider than 32 bits
 * becomes a string, because a JSON number rounds silently past 2^53. Anything
 * this SDK version cannot name becomes `{ type: 'unknown', xdr }` carrying the
 * base64, so a protocol addition degrades to an opaque value rather than
 * failing the page it arrived in. And a map becomes an ordered array of
 * key/value pairs, because Soroban map keys are themselves `ScVal`s: folding
 * them into an object would erase the key's type, collapse duplicates, and
 * lose the wire ordering.
 */

import { scValToNative, type xdr } from '@stellar/stellar-sdk';

import type { DecodedValue } from './types.js';

/** ScVal variants whose native form is a bigint we render as a string. */
const WIDE_INTEGERS = {
  scvU64: 'u64',
  scvI64: 'i64',
  scvU128: 'u128',
  scvI128: 'i128',
  scvU256: 'u256',
  scvI256: 'i256',
  scvTimepoint: 'timepoint',
  scvDuration: 'duration',
} as const;

/** ScVal variants that fit in a JavaScript number without loss. */
const NARROW_INTEGERS = {
  scvU32: 'u32',
  scvI32: 'i32',
} as const;

function isWideInteger(name: string): name is keyof typeof WIDE_INTEGERS {
  return name in WIDE_INTEGERS;
}

function isNarrowInteger(name: string): name is keyof typeof NARROW_INTEGERS {
  return name in NARROW_INTEGERS;
}

/**
 * Decodes one `ScVal`.
 *
 * Never throws. An unrecognized variant, or one whose payload does not decode,
 * comes back as `unknown` with its XDR intact, which keeps one malformed value
 * from discarding every other event in the response.
 */
export function decodeScVal(value: xdr.ScVal): DecodedValue {
  const variant = value.switch().name;

  try {
    if (isNarrowInteger(variant)) {
      return { type: NARROW_INTEGERS[variant], value: Number(scValToNative(value)) };
    }

    if (isWideInteger(variant)) {
      return { type: WIDE_INTEGERS[variant], value: String(scValToNative(value)) };
    }

    switch (variant) {
      case 'scvBool':
        return { type: 'bool', value: Boolean(scValToNative(value)) };

      case 'scvVoid':
        return { type: 'void' };

      case 'scvSymbol':
        return { type: 'symbol', value: String(scValToNative(value)) };

      case 'scvString':
        return { type: 'string', value: String(scValToNative(value)) };

      case 'scvAddress':
        return { type: 'address', value: String(scValToNative(value)) };

      case 'scvBytes':
        return { type: 'bytes', value: Buffer.from(value.bytes()).toString('hex') };

      case 'scvVec':
        return { type: 'vec', value: (value.vec() ?? []).map(decodeScVal) };

      case 'scvMap':
        return {
          type: 'map',
          value: (value.map() ?? []).map((entry) => ({
            key: decodeScVal(entry.key()),
            value: decodeScVal(entry.val()),
          })),
        };

      default:
        return { type: 'unknown', xdr: value.toXDR('base64') };
    }
  } catch {
    return { type: 'unknown', xdr: value.toXDR('base64') };
  }
}

/** Decodes an event's topics, in order. */
export function decodeTopics(topics: readonly xdr.ScVal[]): DecodedValue[] {
  return topics.map(decodeScVal);
}

/**
 * Reads the event name from its leading topic.
 *
 * Soroban puts the event's Symbol first, which is what the indexer stores as
 * `name`. A leading topic that is not a symbol means the emitter did not
 * follow that convention, and the name is empty rather than a guess.
 */
export function eventNameFromTopics(topics: readonly DecodedValue[]): string {
  const [first] = topics;
  return first !== undefined && first.type === 'symbol' ? first.value : '';
}
