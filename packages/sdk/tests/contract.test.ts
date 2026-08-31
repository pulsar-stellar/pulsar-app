/**
 * Tests for contract.ts.
 *
 * The event fixtures are built to the shapes `pulsar-core`'s `events.rs`
 * declares, since those annotations are the wire contract: the leading topic
 * Symbol is pinned there precisely so renaming a Rust struct cannot move it.
 */

import { Account, Address, Networks, nativeToScVal, xdr } from '@stellar/stellar-sdk';
import { describe, expect, it } from 'vitest';

import {
  asAdminChangeEvent,
  asDepositEvent,
  asEmitCustomEvent,
  asInitializeEvent,
  asTransferEvent,
  asWithdrawEvent,
  buildContractCall,
  parseTopics,
  scValToNative,
} from '../src/contract.js';
import { decodeScVal } from '../src/decode.js';
import { PulsarValidationError } from '../src/errors.js';
import type { DecodedEvent, DecodedValue } from '../src/types.js';

const CONTRACT = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';
const ALICE = 'GAB62KEKMKHDOMF3256P4CS7FCWRDUPT7M2CLD4JGKFWCA2H3DW3NRRB';
const BOB = 'GDNL4YQPDMFCLTVMKPYAJ4W6LVGEJ6JTQNXSFVMAWSFYFHCE7HKIYFRA';

/** Builds a DecodedEvent with the given wire name, topics, and data. */
function event(
  name: string,
  extraTopics: readonly DecodedValue[],
  eventData: DecodedValue,
): DecodedEvent {
  return {
    id: '42',
    contractId: CONTRACT,
    ledger: 4_378_751,
    txHash: 'ab'.repeat(32),
    eventIndex: 0,
    name,
    topics: [{ type: 'symbol', value: name }, ...extraTopics],
    data: eventData,
    rawTopics: ['AAAADwAAAANmZWUA'],
    rawData: 'AAAACgAAAAAAAAAAAAAAAAAAAGQ=',
    emittedAt: '2026-08-28T11:42:22Z',
    inSuccessfulContractCall: true,
  };
}

const address = (value: string): DecodedValue => ({ type: 'address', value });
const i128 = (value: string): DecodedValue => ({ type: 'i128', value });

describe('buildContractCall', () => {
  const account = (): Account => new Account(ALICE, '100');
  const base = {
    contractId: CONTRACT,
    method: 'transfer',
    networkPassphrase: Networks.TESTNET,
  };

  it('builds an unsigned transaction invoking the contract', () => {
    const tx = buildContractCall({
      ...base,
      account: account(),
      args: [new Address(ALICE).toScVal(), nativeToScVal(100n, { type: 'i128' })],
    });

    expect(tx.source).toBe(ALICE);
    expect(tx.sequence).toBe('101');
    expect(tx.signatures).toHaveLength(0);
    expect(tx.operations).toHaveLength(1);
    expect(tx.operations[0]?.type).toBe('invokeHostFunction');
    expect(tx.networkPassphrase).toBe(Networks.TESTNET);
  });

  /**
   * The reason ADR-027 exists. `TransactionBuilder.build()` increments the
   * sequence of the account it is handed, so without an internal copy two
   * calls built from one account claim two different sequence numbers, and a
   * caller who submits only one has silently burned the other.
   */
  it('does not mutate the account it was given', () => {
    const shared = account();

    buildContractCall({ ...base, account: shared });

    expect(shared.sequenceNumber()).toBe('100');
  });

  it('gives two calls built from one account the same sequence number', () => {
    const shared = account();

    const first = buildContractCall({ ...base, account: shared });
    const second = buildContractCall({ ...base, account: shared });

    expect(first.sequence).toBe('101');
    expect(second.sequence).toBe('101');
  });

  it('builds a call with no arguments', () => {
    const tx = buildContractCall({ ...base, account: account(), method: 'initialize' });
    expect(tx.operations).toHaveLength(1);
  });

  it('applies the requested fee and timeout', () => {
    const tx = buildContractCall({
      ...base,
      account: account(),
      fee: '5000',
      timeoutSeconds: 120,
    });

    expect(tx.fee).toBe('5000');
    expect(Number(tx.timeBounds?.maxTime ?? 0)).toBeGreaterThan(0);
  });

  it('leaves the transaction unprepared, which is why preparation is documented', () => {
    const tx = buildContractCall({ ...base, account: account() });

    expect(tx.fee).toBe('100');
    expect(tx.toEnvelope().v1().tx().ext().switch()).toBe(0);
  });

  it.each([
    ['an empty method', { method: '' }],
    ['a whitespace method', { method: '   ' }],
    ['an empty network passphrase', { networkPassphrase: '' }],
  ])('rejects %s', (_label, override) => {
    expect(() => buildContractCall({ ...base, ...override, account: account() })).toThrow(
      PulsarValidationError,
    );
  });

  it('rejects a malformed contract ID, keeping the SDK error as cause', () => {
    const failure = ((): unknown => {
      try {
        buildContractCall({ ...base, account: account(), contractId: 'not-a-contract' });
      } catch (error: unknown) {
        return error;
      }
      return null;
    })();

    expect(failure).toBeInstanceOf(PulsarValidationError);
    expect((failure as PulsarValidationError).cause).toBeInstanceOf(Error);
    expect((failure as PulsarValidationError).details).toMatchObject({
      contractId: 'not-a-contract',
    });
  });

  /**
   * The SDK accepts a non-ScVal argument without complaint and produces a
   * malformed operation, so this check has to be ours or the failure surfaces
   * at submission with nothing pointing back to the argument.
   */
  it('rejects an argument that is not an ScVal', () => {
    expect(() =>
      buildContractCall({
        ...base,
        account: account(),
        args: ['raw-string' as unknown as xdr.ScVal],
      }),
    ).toThrow(PulsarValidationError);
  });

  it('names the offending argument position', () => {
    const failure = ((): unknown => {
      try {
        buildContractCall({
          ...base,
          account: account(),
          args: [new Address(ALICE).toScVal(), 42 as unknown as xdr.ScVal],
        });
      } catch (error: unknown) {
        return error;
      }
      return null;
    })();

    expect((failure as PulsarValidationError).details).toMatchObject({ argumentIndex: 1 });
  });

  it('rejects an account whose ID is not a valid address', () => {
    const broken = {
      accountId: (): string => 'nope',
      sequenceNumber: (): string => '1',
    } as unknown as Account;

    expect(() => buildContractCall({ ...base, account: broken })).toThrow(PulsarValidationError);
  });

  it('wraps a build failure rather than letting the SDK error escape', () => {
    const broken = {
      accountId: (): string => ALICE,
      sequenceNumber: (): string => 'not-a-number',
    } as unknown as Account;

    expect(() => buildContractCall({ ...base, account: broken })).toThrow(PulsarValidationError);
  });

  /**
   * Reaches the builder itself rather than the earlier guards, which is the
   * only way to exercise the wrapper around `build()`.
   */
  it('wraps a failure raised inside the builder', () => {
    expect(() =>
      buildContractCall({ ...base, account: account(), timeoutSeconds: -1 }),
    ).toThrow(PulsarValidationError);
  });
});

describe('parseTopics', () => {
  it('converts topics to plain JavaScript values', () => {
    const topics = [nativeToScVal('deposit', { type: 'symbol' }), new Address(ALICE).toScVal()];

    expect(parseTopics(topics)).toEqual(['deposit', ALICE]);
  });

  it('returns an empty array for no topics', () => {
    expect(parseTopics([])).toEqual([]);
  });

  /**
   * These pin the documented divergences from `decodeScVal` rather than
   * asserting they are absent. A consumer choosing this function accepts them,
   * and the JSDoc names them, so a change in either direction should fail here.
   */
  it('returns wide integers as bigint, where decodeScVal returns strings', () => {
    const amount = nativeToScVal(9_007_199_254_740_993n, { type: 'i128' });

    expect(parseTopics([amount])[0]).toBe(9_007_199_254_740_993n);
    expect(decodeScVal(amount)).toEqual({ type: 'i128', value: '9007199254740993' });
  });

  it('returns bytes as a Buffer, where decodeScVal returns hex', () => {
    const bytes = nativeToScVal(Buffer.from('beef', 'hex'));

    expect(Buffer.isBuffer(parseTopics([bytes])[0])).toBe(true);
    expect(decodeScVal(bytes)).toEqual({ type: 'bytes', value: 'beef' });
  });

  it('drops a duplicate map key, where decodeScVal keeps both entries', () => {
    const key = nativeToScVal('a', { type: 'symbol' });
    const map = xdr.ScVal.scvMap([
      new xdr.ScMapEntry({ key, val: nativeToScVal(1, { type: 'u32' }) }),
      new xdr.ScMapEntry({ key, val: nativeToScVal(2, { type: 'u32' }) }),
    ]);

    expect(parseTopics([map])[0]).toEqual({ a: 2 });

    const faithful = decodeScVal(map);
    expect(faithful.type).toBe('map');
    if (faithful.type !== 'map') return;
    expect(faithful.value).toHaveLength(2);
  });
});

describe('the scValToNative re-export', () => {
  it('is the SDK function itself, not a wrapper', async () => {
    const sdk = await import('@stellar/stellar-sdk');

    expect(scValToNative).toBe(sdk.scValToNative);
  });
});

describe('the ADR-016 binding bridge', () => {
  it('reads an initialize event', () => {
    expect(asInitializeEvent(event('initialize', [], address(ALICE)))).toEqual({
      name: 'Initialize',
      data: { admin: ALICE },
    });
  });

  it('reads a deposit event, taking from out of the topics', () => {
    expect(asDepositEvent(event('deposit', [address(ALICE)], i128('250')))).toEqual({
      name: 'Deposit',
      data: { from: ALICE, amount: 250n },
    });
  });

  it('reads a withdraw event', () => {
    expect(asWithdrawEvent(event('withdraw', [address(BOB)], i128('175')))).toEqual({
      name: 'Withdraw',
      data: { to: BOB, amount: 175n },
    });
  });

  it('reads a transfer event, keeping SEP-41 topic order', () => {
    expect(
      asTransferEvent(event('transfer', [address(ALICE), address(BOB)], i128('900'))),
    ).toEqual({
      name: 'Transfer',
      data: { from: ALICE, to: BOB, amount: 900n },
    });
  });

  it('reads an admin change event, with the incoming admin as the topic', () => {
    expect(
      asAdminChangeEvent(event('admin_change', [address(BOB)], address(ALICE))),
    ).toEqual({
      name: 'AdminChange',
      data: { new_admin: BOB, old_admin: ALICE },
    });
  });

  it('reads a custom event, whose wire symbol is custom rather than emit_custom', () => {
    expect(
      asEmitCustomEvent(
        event('custom', [{ type: 'symbol', value: 'settled' }], {
          type: 'bytes',
          value: 'deadbeef',
        }),
      ),
    ).toEqual({
      name: 'EmitCustom',
      data: { tag: 'settled', payload: new Uint8Array([0xde, 0xad, 0xbe, 0xef]) },
    });
  });

  /**
   * The payload is bytes rather than the binding's Buffer, which is the one
   * place this family deliberately does not compose. Pinned here and in the
   * composition check so the exception stays visible.
   */
  it('carries the custom payload as bytes, not as hex and not as a Buffer', () => {
    const mapped = asEmitCustomEvent(
      event('custom', [{ type: 'symbol', value: 'settled' }], {
        type: 'bytes',
        value: 'deadbeef',
      }),
    );

    expect(mapped?.data.payload).toBeInstanceOf(Uint8Array);
    expect(Buffer.isBuffer(mapped?.data.payload)).toBe(false);
    expect(Array.from(mapped?.data.payload ?? [])).toEqual([222, 173, 190, 239]);
  });

  it('yields empty bytes for a payload that is not usable hex', () => {
    const odd = asEmitCustomEvent(
      event('custom', [{ type: 'symbol', value: 'settled' }], { type: 'bytes', value: 'abc' }),
    );
    const notHex = asEmitCustomEvent(
      event('custom', [{ type: 'symbol', value: 'settled' }], { type: 'bytes', value: 'zz' }),
    );

    expect(odd?.data.payload).toEqual(new Uint8Array(0));
    expect(notHex?.data.payload).toEqual(new Uint8Array(0));
  });

  it('handles an empty payload', () => {
    const mapped = asEmitCustomEvent(
      event('custom', [{ type: 'symbol', value: 'settled' }], { type: 'bytes', value: '' }),
    );

    expect(mapped?.data.payload).toEqual(new Uint8Array(0));
  });

  it('keeps the amount exact past what a JavaScript number holds', () => {
    const huge = '170141183460469231731687303715884105727';
    const mapped = asDepositEvent(event('deposit', [address(ALICE)], i128(huge)));

    expect(mapped?.data.amount).toBe(BigInt(huge));
    expect(mapped?.data.amount.toString()).toBe(huge);
  });
});

describe('the bridge refusing events that are not its own', () => {
  const deposit = event('deposit', [address(ALICE)], i128('250'));

  it.each([
    ['asInitializeEvent', asInitializeEvent],
    ['asWithdrawEvent', asWithdrawEvent],
    ['asTransferEvent', asTransferEvent],
    ['asAdminChangeEvent', asAdminChangeEvent],
    ['asEmitCustomEvent', asEmitCustomEvent],
  ])('%s returns null for a deposit', (_label, helper) => {
    expect(helper(deposit)).toBeNull();
  });

  /**
   * The case difference ADR-016 warns about. A consumer matching the binding's
   * `Deposit` against a wire name never fires, so the helper must not either.
   */
  it('refuses the binding-cased name, which never appears on the wire', () => {
    expect(asDepositEvent(event('Deposit', [address(ALICE)], i128('250')))).toBeNull();
  });

  it('refuses a right-named event carrying the wrong number of topics', () => {
    expect(asTransferEvent(event('transfer', [address(ALICE)], i128('900')))).toBeNull();
    expect(
      asDepositEvent(event('deposit', [address(ALICE), address(BOB)], i128('250'))),
    ).toBeNull();
  });

  it('refuses a right-shaped event whose topic is the wrong type', () => {
    expect(
      asDepositEvent(event('deposit', [{ type: 'symbol', value: ALICE }], i128('250'))),
    ).toBeNull();
  });

  it('refuses a right-shaped event whose data is the wrong type', () => {
    expect(asDepositEvent(event('deposit', [address(ALICE)], { type: 'void' }))).toBeNull();
    expect(asInitializeEvent(event('initialize', [], i128('1')))).toBeNull();
    expect(
      asEmitCustomEvent(
        event('custom', [{ type: 'symbol', value: 'settled' }], { type: 'void' }),
      ),
    ).toBeNull();
  });

  it.each([
    ['asWithdrawEvent', asWithdrawEvent, 'withdraw', 1],
    ['asAdminChangeEvent', asAdminChangeEvent, 'admin_change', 1],
  ])('%s refuses an event whose address topic is the wrong type', (_label, helper, name, count) => {
    const wrongTopic = event(
      name,
      Array.from({ length: count }, (): DecodedValue => ({ type: 'symbol', value: 'x' })),
      name === 'admin_change' ? address(ALICE) : i128('1'),
    );

    expect(helper(wrongTopic)).toBeNull();
  });

  it('asTransferEvent refuses an event whose second address topic is wrong', () => {
    expect(
      asTransferEvent(
        event('transfer', [address(ALICE), { type: 'symbol', value: 'x' }], i128('900')),
      ),
    ).toBeNull();
  });

  it.each([
    ['asWithdrawEvent', asWithdrawEvent, 'withdraw', [address(BOB)]],
    ['asTransferEvent', asTransferEvent, 'transfer', [address(ALICE), address(BOB)]],
    ['asAdminChangeEvent', asAdminChangeEvent, 'admin_change', [address(BOB)]],
  ])('%s refuses an event whose data is the wrong type', (_label, helper, name, topics) => {
    expect(helper(event(name, topics, { type: 'void' }))).toBeNull();
  });

  it('refuses an event with no topics at all', () => {
    const empty: DecodedEvent = { ...deposit, name: '', topics: [] };

    expect(asDepositEvent(empty)).toBeNull();
    expect(asInitializeEvent(empty)).toBeNull();
  });

  it('refuses an unknown contract event rather than guessing', () => {
    expect(asDepositEvent(event('mint', [address(ALICE)], i128('1')))).toBeNull();
  });
});
