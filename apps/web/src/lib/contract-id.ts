/**
 * Client-safe contract ID validation.
 *
 * The explorer's lookup runs in the browser, so it cannot reach the SDK's
 * `ContractIdSchema` without dragging the SDK (and its `@stellar/stellar-sdk`
 * dependency) into the client bundle, which ADR-046 keeps out. This module
 * restates the same rule the SDK enforces, as a self-contained pattern with no
 * imports, purely to give immediate feedback before a lookup navigates.
 *
 * It is a pre-check, not the authority: every id that reaches the data layer is
 * validated again server-side against the SDK schema (see `assertContractId` in
 * the GraphQL query wrappers), which returns the same `VALIDATION_CONTRACT_ID`
 * code. Keep this pattern in step with the SDK's `/^C[A-Z2-7]{55}$/`.
 */

/** A Soroban contract ID: 'C' followed by 55 RFC4648 base32 characters. */
export const CONTRACT_ID_PATTERN = /^C[A-Z2-7]{55}$/;

/** Whether `value` is a well-formed contract ID, exactly as the SDK checks. */
export function isContractId(value: string): boolean {
  return CONTRACT_ID_PATTERN.test(value);
}

/** Trim surrounding whitespace so a pasted id validates and routes cleanly. */
export function normalizeContractId(value: string): string {
  return value.trim();
}
