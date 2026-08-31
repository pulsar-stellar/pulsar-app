/**
 * Type-level assertions for contract.ts.
 *
 * ADR-016's whole point is that two shapes describe one event and must not be
 * confused. Most of that is a runtime concern, but the parts a runtime test
 * cannot reach live here: that the bridge narrows to one binding type, that it
 * admits null, and that the amount is a bigint rather than the string the wire
 * carries.
 */

import type { Account, Transaction, xdr } from '@stellar/stellar-sdk';
import { assertType, describe, expectTypeOf, it } from 'vitest';

import {
  asAdminChangeEvent,
  asDepositEvent,
  asEmitCustomEvent,
  asInitializeEvent,
  asTransferEvent,
  asWithdrawEvent,
  buildContractCall,
  parseTopics,
  type ContractCallOptions,
  type DepositEvent,
  type EmitCustomEvent,
  type TransferEvent,
} from '../src/contract.js';
import type { DecodedEvent } from '../src/types.js';

declare const event: DecodedEvent;
declare const account: Account;
declare const scVal: xdr.ScVal;

describe('buildContractCall', () => {
  it('returns a Transaction synchronously, not a promise', () => {
    expectTypeOf(buildContractCall).returns.toEqualTypeOf<Transaction>();
    expectTypeOf(buildContractCall).returns.not.toMatchTypeOf<Promise<unknown>>();
  });

  it('requires an Account rather than an address string', () => {
    expectTypeOf<ContractCallOptions['account']>().toEqualTypeOf<Account>();
    assertType<ContractCallOptions>({
      // @ts-expect-error an address string carries no sequence number
      account: 'GAB62KEKMKHDOMF3256P4CS7FCWRDUPT7M2CLD4JGKFWCA2H3DW3NRRB',
      contractId: 'C',
      method: 'transfer',
      networkPassphrase: 'p',
    });
  });

  it('requires arguments to be ScVal', () => {
    assertType<ContractCallOptions>({
      account,
      contractId: 'C',
      method: 'transfer',
      args: [scVal],
      networkPassphrase: 'p',
    });

    assertType<ContractCallOptions>({
      account,
      contractId: 'C',
      method: 'transfer',
      // @ts-expect-error a plain string is not an ScVal
      args: ['raw'],
      networkPassphrase: 'p',
    });
  });

  it('makes the network passphrase required, since a default would guess', () => {
    // @ts-expect-error networkPassphrase is not optional
    assertType<ContractCallOptions>({ account, contractId: 'C', method: 'transfer' });
  });
});

describe('parseTopics', () => {
  it('returns unknown values, so a caller has to narrow before using them', () => {
    expectTypeOf(parseTopics).returns.toEqualTypeOf<unknown[]>();
  });
});

describe('the binding bridge', () => {
  it('narrows to exactly one binding type, or null', () => {
    expectTypeOf(asDepositEvent(event)).toEqualTypeOf<DepositEvent | null>();
    expectTypeOf(asTransferEvent(event)).toEqualTypeOf<TransferEvent | null>();
  });

  it('admits null, so a caller cannot skip the mismatch case', () => {
    expectTypeOf(asDepositEvent(event)).toBeNullable();
    expectTypeOf(asInitializeEvent(event)).toBeNullable();
    expectTypeOf(asWithdrawEvent(event)).toBeNullable();
    expectTypeOf(asAdminChangeEvent(event)).toBeNullable();
    expectTypeOf(asEmitCustomEvent(event)).toBeNullable();
  });

  it('carries the binding name as a literal, not as a string', () => {
    expectTypeOf<DepositEvent['name']>().toEqualTypeOf<'Deposit'>();
    expectTypeOf<TransferEvent['name']>().toEqualTypeOf<'Transfer'>();
  });

  it('carries the amount as bigint, where the wire event carries a string', () => {
    expectTypeOf<DepositEvent['data']['amount']>().toEqualTypeOf<bigint>();
    expectTypeOf<Extract<DecodedEvent['data'], { type: 'i128' }>['value']>().toEqualTypeOf<string>();
  });

  it('keeps the two views from being interchanged', () => {
    const mapped = asDepositEvent(event);
    if (mapped === null) return;
    // @ts-expect-error a binding event is not a DecodedEvent
    assertType<DecodedEvent>(mapped);
    // @ts-expect-error and a DecodedEvent is not a binding event
    assertType<DepositEvent>(event);
  });

  it('carries the custom payload as bytes rather than the binding Buffer', () => {
    expectTypeOf<EmitCustomEvent['data']['payload']>().toEqualTypeOf<Uint8Array>();
  });

  it('puts a topic field inside data, which is where the binding looks for it', () => {
    expectTypeOf<TransferEvent['data']>().toHaveProperty('from');
    expectTypeOf<TransferEvent['data']>().toHaveProperty('to');
    expectTypeOf<TransferEvent['data']>().toHaveProperty('amount');
  });
});
