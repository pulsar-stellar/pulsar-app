import { describe, expect, it } from 'vitest';

import { isContractId, normalizeContractId } from '@/lib/contract-id';

// A well-formed Soroban contract ID: 'C' plus 55 base32 (A-Z, 2-7) characters.
const VALID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('isContractId', () => {
  it('accepts a well-formed contract ID', () => {
    expect(isContractId(VALID)).toBe(true);
  });

  it('rejects the wrong length', () => {
    expect(isContractId(VALID.slice(0, 55))).toBe(false);
    expect(isContractId(`${VALID}A`)).toBe(false);
  });

  it('rejects an ID that does not start with C', () => {
    expect(isContractId(`A${VALID.slice(1)}`)).toBe(false);
  });

  it('rejects characters outside the base32 alphabet', () => {
    // '0', '1', '8', '9' and lowercase are not in RFC4648 base32.
    expect(isContractId(`C${'0'.repeat(55)}`)).toBe(false);
    expect(isContractId(VALID.toLowerCase())).toBe(false);
  });

  it('rejects empty and whitespace-only input', () => {
    expect(isContractId('')).toBe(false);
    expect(isContractId('   ')).toBe(false);
  });
});

describe('normalizeContractId', () => {
  it('trims surrounding whitespace', () => {
    expect(normalizeContractId(`  ${VALID}\n`)).toBe(VALID);
  });

  it('leaves an already-clean value unchanged', () => {
    expect(normalizeContractId(VALID)).toBe(VALID);
  });
});
