/**
 * The ADR-016 bindings composition check.
 *
 * Generates TypeScript bindings for the showcase contract at test time and
 * asserts that the binding's own event types compose with what the `as*Event`
 * helpers return. If the contract changes shape, or if the SDK changes how it
 * generates bindings, this fails here rather than in a consumer's silently
 * empty match.
 *
 * Three things follow ADR-016's settled design. Generation happens at runtime
 * into `tmp/test-fixtures/<hash>/`, gitignored, where the hash covers contract
 * ID, network, and SDK version, so different inputs get different directories
 * and identical inputs reuse one. Generated code is a build artifact, and a
 * committed fixture would go stale against the deployment it claims to
 * describe. And the assertions are type-level, because the binding's event
 * exports are interfaces and are erased at runtime.
 *
 * The type-level part is driven by running `tsc` over a probe file rather than
 * by `vitest --typecheck`, because this test has to skip when RPC is
 * unreachable and a `.test-d.ts` cannot make that decision at runtime.
 *
 * Excluded from the default run. Opt in with:
 *
 * ```sh
 * pnpm vitest run --config vitest.integration.config.ts
 * ```
 */

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';
const NETWORK = 'testnet';
const RPC_URL = 'https://soroban-testnet.stellar.org';
const SDK_VERSION = '16.2.0';

/** Where this fixture lives, keyed so different inputs never share a directory. */
const fixtureDir = join(
  process.cwd(),
  'tmp',
  'test-fixtures',
  createHash('sha256')
    .update(`${CONTRACT_ID}:${NETWORK}:${SDK_VERSION}`)
    .digest('hex')
    .slice(0, 16),
);

/** Whether RPC answers at all. An outage must skip, never fail. */
async function rpcReachable(): Promise<boolean> {
  try {
    const response = await fetch(RPC_URL, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'getHealth' }),
      signal: AbortSignal.timeout(10_000),
    });
    return response.ok;
  } catch {
    return false;
  }
}

/** Generates the bindings, reusing the directory when it is already populated. */
function generateBindings(): void {
  if (existsSync(fixtureDir) && readdirSync(fixtureDir).length > 0) return;

  mkdirSync(fixtureDir, { recursive: true });
  execFileSync(
    'npx',
    [
      '@stellar/stellar-sdk',
      'generate',
      '--contract-id',
      CONTRACT_ID,
      '--network',
      NETWORK,
      '--output-dir',
      fixtureDir,
      '--overwrite',
    ],
    { stdio: 'pipe', timeout: 180_000 },
  );
}

describe('generated bindings compose with the ADR-016 bridge', () => {
  it(
    'type-checks the bridge output against the generated binding types',
    async (context) => {
      if (!(await rpcReachable())) {
        // Skipped, not passed. A bare return would report green and look
        // identical to a run that verified something, which is the failure
        // mode this check exists to avoid.
        context.skip(`Stellar RPC unreachable at ${RPC_URL}`);
        return;
      }

      generateBindings();
      expect(readdirSync(fixtureDir).length).toBeGreaterThan(0);

      /**
       * The probe asserts assignability in both directions that matter: the
       * helper's output has to satisfy the binding's field types, and the
       * binding's name literal has to be the one the helper produces. It also
       * pins the divergence ADR-016 exists for, that the wire name is not the
       * binding name.
       */
      const probePath = join(fixtureDir, 'composition-probe.ts');
      const contractPath = join(process.cwd(), 'src', 'contract.js');
      writeFileSync(
        probePath,
        `import type {
  DepositEvent as BoundDeposit,
  TransferEvent as BoundTransfer,
  WithdrawEvent as BoundWithdraw,
  InitializeEvent as BoundInitialize,
  AdminChangeEvent as BoundAdminChange,
  EmitCustomEvent as BoundEmitCustom,
} from './src/types.js';
import type {
  DepositEvent,
  TransferEvent,
  WithdrawEvent,
  InitializeEvent,
  AdminChangeEvent,
  EmitCustomEvent,
} from '${contractPath}';

// Five of the six compose with no conversion at all.
declare const deposit: DepositEvent;
declare const transfer: TransferEvent;
declare const withdraw: WithdrawEvent;
declare const initialize: InitializeEvent;
declare const adminChange: AdminChangeEvent;

const a: BoundDeposit = deposit;
const b: BoundTransfer = transfer;
const c: BoundWithdraw = withdraw;
const d: BoundInitialize = initialize;
const e: BoundAdminChange = adminChange;
void a; void b; void c; void d; void e;

// The sixth deliberately does not. The binding declares payload as Buffer, and
// Buffer extends Uint8Array, so this assignment fails in the direction the
// bridge needs. Keeping Buffer out of the SDK's public types is worth one
// documented conversion; this assertion is what keeps the exception to one.
declare const custom: EmitCustomEvent;
// @ts-expect-error payload is Uint8Array here and Buffer in the binding
const f: BoundEmitCustom = custom;
void f;

// Crossing it explicitly is the documented one-liner.
const crossed: BoundEmitCustom = {
  ...custom,
  data: { ...custom.data, payload: Buffer.from(custom.data.payload) },
};
void crossed;

// Every other field of the custom event already lines up.
const tag: BoundEmitCustom['data']['tag'] = custom.data.tag;
const name: BoundEmitCustom['name'] = custom.name;
void tag; void name;
`,
        'utf8',
      );

      const result = execFileSync(
        'npx',
        [
          'tsc', '--noEmit', '--strict', '--skipLibCheck', '--target', 'ES2022',
          '--module', 'preserve', '--moduleResolution', 'bundler', '--types', 'node',
          probePath,
        ],
        { stdio: 'pipe', timeout: 120_000, encoding: 'utf8' },
      );

      expect(result).toBe('');
    },
    240_000,
  );
});
