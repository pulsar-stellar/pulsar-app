/**
 * Helpers for calling contracts and for reading their events the way a
 * generated binding describes them.
 *
 * Three separate jobs live here. {@link buildContractCall} assembles an
 * invocation. {@link parseTopics} and the re-exported {@link scValToNative}
 * give the ergonomic, lossy view of contract values. The `as*Event` family
 * bridges ADR-016's two views of an event, wire-oriented and call-oriented.
 *
 * The bridge is opt-in and never implicit. `DecodedEvent` stays faithful to
 * what the ledger holds, and a consumer who wants the binding's shape asks for
 * it here.
 */

import {
  Account,
  Contract,
  TransactionBuilder,
  scValToNative,
  type Transaction,
  type xdr,
} from '@stellar/stellar-sdk';

import { PulsarValidationError } from './errors.js';
import type { DecodedEvent, DecodedValue } from './types.js';

/**
 * `@stellar/stellar-sdk`'s converter from `ScVal` to a plain JavaScript value,
 * re-exported so a consumer needs one import rather than two.
 *
 * This is a pass-through with no wrapping. It is the SDK's function, and it
 * carries the SDK's behaviour, including the gaps documented on
 * {@link parseTopics}. For faithful decoding, use `decodeScVal`.
 */
export { scValToNative };

/** The default transaction timeout, in seconds, when a caller names none. */
export const DEFAULT_CALL_TIMEOUT_SECONDS = 30;

/** What {@link buildContractCall} needs to assemble an invocation. */
export interface ContractCallOptions {
  /**
   * The account that will submit the call.
   *
   * Copied before use, so the caller's `Account` is never mutated. See
   * ADR-027: `TransactionBuilder.build()` increments the sequence number of
   * whatever `Account` it is handed, which would make two calls built from one
   * account silently claim two different sequence numbers.
   */
  readonly account: Account;
  /** The contract to call. */
  readonly contractId: string;
  /** The method name, as the contract declares it. */
  readonly method: string;
  /** Arguments, already converted to `ScVal` with `nativeToScVal` or `Address`. */
  readonly args?: readonly xdr.ScVal[];
  /** The network this transaction is for. */
  readonly networkPassphrase: string;
  /** Fee in stroops. Defaults to the SDK's base fee. */
  readonly fee?: string;
  /** Seconds until the transaction expires. Defaults to 30. */
  readonly timeoutSeconds?: number;
}

/**
 * Builds an unsigned, unprepared contract call transaction.
 *
 * **The returned transaction is not submittable as it stands.** It must go
 * through `server.prepareTransaction(tx)` first. An unprepared Soroban
 * transaction carries an empty footprint and only the base fee, so submitting
 * one fails. Preparation runs the simulation that attaches the resource
 * footprint and the real fee.
 *
 * The full flow:
 *
 * ```ts
 * const account = await server.getAccount(callerAddress);
 * const tx = buildContractCall({
 *   account,
 *   contractId,
 *   method: 'transfer',
 *   args: [new Address(from).toScVal(), new Address(to).toScVal(), nativeToScVal(100n, { type: 'i128' })],
 *   networkPassphrase: Networks.TESTNET,
 * });
 *
 * const prepared = await server.prepareTransaction(tx);
 * prepared.sign(keypair);
 * await server.sendTransaction(prepared);
 * ```
 *
 * This function is synchronous and touches no network, which is why it stops
 * at assembly. A build step that silently made an RPC call would also report
 * simulation failures, contract reverts and insufficient balances among them,
 * from a function whose name promises only that it assembled something.
 *
 * Arguments must already be `ScVal`. The SDK does not check this: passing a
 * plain string produces a malformed operation with no error at all, so the
 * check happens here.
 *
 * @returns An unsigned, unprepared {@link Transaction}. Preparation is
 * required before signing and submission.
 * @throws {PulsarValidationError} if the contract ID, method, arguments, or
 * account are unusable, with the underlying SDK error as `cause` where there
 * is one.
 */
export function buildContractCall(options: ContractCallOptions): Transaction {
  const operation = 'contract.buildContractCall';
  const { account, contractId, method, args = [], networkPassphrase } = options;

  if (method.trim() === '') {
    throw new PulsarValidationError('Contract method name must not be empty', {
      operation,
      details: { contractId },
    });
  }

  if (networkPassphrase.trim() === '') {
    throw new PulsarValidationError('networkPassphrase must not be empty', {
      operation,
      details: { contractId, method },
    });
  }

  args.forEach((arg, index) => {
    if (typeof arg !== 'object' || arg === null || typeof arg.toXDR !== 'function') {
      throw new PulsarValidationError(
        'Contract call arguments must be ScVal, built with nativeToScVal or Address',
        { operation, details: { contractId, method, argumentIndex: index } },
      );
    }
  });

  let contract: Contract;

  try {
    contract = new Contract(contractId);
  } catch (cause) {
    throw new PulsarValidationError('Invalid contract ID', {
      operation,
      cause,
      details: { contractId },
    });
  }

  // The copy is the point: build() advances the sequence number of the account
  // it is given, and that account belongs to the caller.
  let source: Account;

  try {
    source = new Account(account.accountId(), account.sequenceNumber());
  } catch (cause) {
    throw new PulsarValidationError('Invalid account', {
      operation,
      cause,
      details: { contractId, method },
    });
  }

  try {
    return new TransactionBuilder(source, {
      fee: options.fee ?? '100',
      networkPassphrase,
    })
      .addOperation(contract.call(method, ...args))
      .setTimeout(options.timeoutSeconds ?? DEFAULT_CALL_TIMEOUT_SECONDS)
      .build();
  } catch (cause) {
    throw new PulsarValidationError('Could not build the contract call transaction', {
      operation,
      cause,
      details: { contractId, method },
    });
  }
}

/**
 * Decodes topics into plain JavaScript values, using the SDK's
 * `scValToNative`.
 *
 * This is the ergonomic view, for inspection, logging, and ad-hoc scripting.
 * It is lossy, and the losses are the SDK's rather than this project's:
 *
 * - **A map with duplicate keys keeps only the last entry.** Verified against
 *   the SDK: a two-entry map with the same key twice returns one entry, with
 *   no error. `decodeScVal` keeps both, in wire order.
 * - **A contract instance is not converted.** `scValToNative` hands back the
 *   raw XDR struct rather than a native value. `decodeScVal` returns it as
 *   `unknown` with its base64 intact, per ADR-023.
 * - **Wide integers come back as `bigint`.** `u64`, `i64`, `u128`, `i128`,
 *   `u256`, `i256`, `timepoint`, and `duration` all do. `decodeScVal` returns
 *   strings, per ADR-021, so they survive JSON without rounding.
 * - **Bytes come back as a `Buffer`.** `decodeScVal` returns hex.
 *
 * Use this when the values are ordinary and convenience matters. Use
 * `decodeTopics` when the result is stored, compared, or sent anywhere,
 * because that is where a dropped map entry or a rounded integer does damage.
 */
export function parseTopics(topics: readonly xdr.ScVal[]): unknown[] {
  // scValToNative is declared as returning `any`. Narrowing to `unknown` here
  // is the whole reason this wrapper is typed rather than re-exported: a
  // caller has to look at what came back before using it.
  return topics.map((topic): unknown => scValToNative(topic));
}

/**
 * An event as a generated binding describes it.
 *
 * The binding is call-oriented, per ADR-016: it names the event after the Rust
 * struct, capitalized, and folds topics and data together into one `data`
 * object. `DecodedEvent` is wire-oriented and does neither. These types are
 * the target of that conversion.
 */
export interface BindingEvent<Name extends string, Data> {
  readonly name: Name;
  readonly data: Data;
}

/** `Initialize`, emitted on the wire as topic `initialize`. */
export type InitializeEvent = BindingEvent<'Initialize', { admin: string }>;

/** `Deposit`, emitted on the wire as topic `deposit`. */
export type DepositEvent = BindingEvent<'Deposit', { from: string; amount: bigint }>;

/** `Withdraw`, emitted on the wire as topic `withdraw`. */
export type WithdrawEvent = BindingEvent<'Withdraw', { to: string; amount: bigint }>;

/** `Transfer`, emitted on the wire as topic `transfer`. */
export type TransferEvent = BindingEvent<
  'Transfer',
  { from: string; to: string; amount: bigint }
>;

/** `AdminChange`, emitted on the wire as topic `admin_change`. */
export type AdminChangeEvent = BindingEvent<
  'AdminChange',
  { new_admin: string; old_admin: string }
>;

/**
 * `EmitCustom`, emitted on the wire as topic `custom`, not `emit_custom`.
 *
 * `payload` is a `Uint8Array`, and this is the one field in the family that
 * does not compose with the generated binding, which declares it `Buffer`.
 * `Buffer` extends `Uint8Array`, so the assignment fails in the direction the
 * bridge needs. That is deliberate: `Buffer` is Node's, and putting it in this
 * SDK's public types would push a polyfill onto every browser consumer for one
 * field of one event. A caller crossing to the binding writes
 * `Buffer.from(payload)`, and the composition test pins the divergence so it
 * stays visible rather than spreading.
 */
export type EmitCustomEvent = BindingEvent<'EmitCustom', { tag: string; payload: Uint8Array }>;

/**
 * Turns the hex `decodeScVal` produces back into bytes.
 *
 * Odd-length or non-hex input yields an empty array rather than a partial one,
 * since a half-decoded payload is worse than an obviously empty one.
 */
function hexToBytes(hex: string): Uint8Array {
  if (hex.length % 2 !== 0 || !/^[0-9a-f]*$/i.test(hex)) return new Uint8Array(0);

  const bytes = new Uint8Array(hex.length / 2);

  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16);
  }

  return bytes;
}

/** Reads a topic at `index`, when it is of `type`. */
function topic<T extends DecodedValue['type']>(
  event: DecodedEvent,
  index: number,
  type: T,
): Extract<DecodedValue, { type: T }> | null {
  const value = event.topics[index];
  return value !== undefined && value.type === type
    ? (value as Extract<DecodedValue, { type: T }>)
    : null;
}

/** Reads the event's data, when it is of `type`. */
function data<T extends DecodedValue['type']>(
  event: DecodedEvent,
  type: T,
): Extract<DecodedValue, { type: T }> | null {
  return event.data.type === type ? (event.data as Extract<DecodedValue, { type: T }>) : null;
}

/**
 * Checks the wire name and the topic count together.
 *
 * Both matter. The name alone would accept an event carrying the right symbol
 * with the wrong arity, which is what a contract emitting its own `transfer`
 * with a different shape looks like.
 */
function matches(event: DecodedEvent, wireName: string, topicCount: number): boolean {
  return event.name === wireName && event.topics.length === topicCount;
}

/**
 * Reads an event as `Initialize`, or null when it is not one.
 *
 * Wire topic is `initialize`, lowercase; the binding calls it `Initialize`.
 * That case difference is exactly what a hand-rolled bridge gets wrong, per
 * ADR-016, because matching the binding's name against `DecodedEvent.name`
 * silently never fires.
 */
export function asInitializeEvent(event: DecodedEvent): InitializeEvent | null {
  if (!matches(event, 'initialize', 1)) return null;

  const admin = data(event, 'address');
  return admin === null ? null : { name: 'Initialize', data: { admin: admin.value } };
}

/**
 * Reads an event as `Deposit`, or null when it is not one.
 *
 * `from` is the second wire topic, and the binding puts it inside `data`. A
 * consumer reading `from` out of the wire event's data finds nothing.
 */
export function asDepositEvent(event: DecodedEvent): DepositEvent | null {
  if (!matches(event, 'deposit', 2)) return null;

  const from = topic(event, 1, 'address');
  const amount = data(event, 'i128');

  return from === null || amount === null
    ? null
    : { name: 'Deposit', data: { from: from.value, amount: BigInt(amount.value) } };
}

/** Reads an event as `Withdraw`, or null when it is not one. */
export function asWithdrawEvent(event: DecodedEvent): WithdrawEvent | null {
  if (!matches(event, 'withdraw', 2)) return null;

  const to = topic(event, 1, 'address');
  const amount = data(event, 'i128');

  return to === null || amount === null
    ? null
    : { name: 'Withdraw', data: { to: to.value, amount: BigInt(amount.value) } };
}

/**
 * Reads an event as `Transfer`, or null when it is not one.
 *
 * Topic order is `from` then `to`, fixed by SEP-41. Reversing them produces a
 * transfer that reads as valid and points the wrong way, which no type can
 * catch, so the order is pinned by test.
 */
export function asTransferEvent(event: DecodedEvent): TransferEvent | null {
  if (!matches(event, 'transfer', 3)) return null;

  const from = topic(event, 1, 'address');
  const to = topic(event, 2, 'address');
  const amount = data(event, 'i128');

  return from === null || to === null || amount === null
    ? null
    : {
        name: 'Transfer',
        data: { from: from.value, to: to.value, amount: BigInt(amount.value) },
      };
}

/**
 * Reads an event as `AdminChange`, or null when it is not one.
 *
 * The asymmetry is the contract's: the incoming admin is a topic, so it can be
 * filtered on, and the outgoing admin is the payload.
 */
export function asAdminChangeEvent(event: DecodedEvent): AdminChangeEvent | null {
  if (!matches(event, 'admin_change', 2)) return null;

  const newAdmin = topic(event, 1, 'address');
  const oldAdmin = data(event, 'address');

  return newAdmin === null || oldAdmin === null
    ? null
    : {
        name: 'AdminChange',
        data: { new_admin: newAdmin.value, old_admin: oldAdmin.value },
      };
}

/**
 * Reads an event as `EmitCustom`, or null when it is not one.
 *
 * The wire symbol is `custom`, not `emit_custom`, because the contract pins it
 * in its annotation so renaming the Rust struct cannot move the wire contract.
 * The second topic is chosen at runtime rather than fixed at compile time, so
 * its value varies per call while its position does not.
 */
export function asEmitCustomEvent(event: DecodedEvent): EmitCustomEvent | null {
  if (!matches(event, 'custom', 2)) return null;

  const tag = topic(event, 1, 'symbol');
  const payload = data(event, 'bytes');

  return tag === null || payload === null
    ? null
    : { name: 'EmitCustom', data: { tag: tag.value, payload: hexToBytes(payload.value) } };
}
