#!/usr/bin/env bash
#
# Verifies the published artifact, not the source.
#
# The unit suite runs against `src`. Nothing in it would notice a broken
# `exports` map, a CJS bundle that fails to re-export its names, a declaration
# file the consumer's resolver cannot find, or a tarball carrying the tests.
# Those are properties of what gets published, so they are checked here against
# a real build.
#
# Run with `pnpm verify:build` from packages/sdk.

set -euo pipefail

cd "$(dirname "$0")/.."

fail() { printf '  FAIL  %s\n' "$1" >&2; exit 1; }
pass() { printf '  ok    %s\n' "$1"; }

printf '\nBuilding\n'
pnpm build >/dev/null
pass "pnpm build succeeded"

printf '\nOutput layout\n'
for file in dist/index.js dist/index.cjs dist/index.d.ts; do
  [ -f "$file" ] || fail "$file is missing"
done
pass "ESM, CJS, and declaration entry points exist"

# The exports map names files by hand, so a change to tsup's naming would break
# consumers while every test still passed.
node --input-type=module -e '
import { readFileSync } from "node:fs";
const pkg = JSON.parse(readFileSync("package.json", "utf8"));
const referenced = [
  pkg.main,
  pkg.module,
  pkg.types,
  pkg.exports["."].import,
  pkg.exports["."].require,
  pkg.exports["."].types,
];
const missing = referenced.filter((path) => {
  try { readFileSync(path); return false; } catch { return true; }
});
if (missing.length > 0) {
  console.error("package.json points at files the build does not produce: " + missing.join(", "));
  process.exit(1);
}
' || fail "package.json entry points do not match build output"
pass "every package.json entry point resolves to a real file"

printf '\nRuntime\n'
node --input-type=module -e '
import { PulsarClient, decodeScVal, buildContractCall } from "./dist/index.js";
const client = new PulsarClient({ indexerUrl: "https://indexer.example" });
if (client.config.indexerUrl !== "https://indexer.example") throw new Error("ESM client misbehaved");
if (typeof decodeScVal !== "function" || typeof buildContractCall !== "function") {
  throw new Error("ESM named exports missing");
}
' || fail "ESM smoke test"
pass "ESM build imports and constructs a client"

# Named-export destructuring is the thing that breaks when a bundler emits a
# default-only CJS shim, and it breaks for every CJS consumer at once.
node -e '
const { PulsarClient, decodeScVal, buildContractCall } = require("./dist/index.cjs");
const client = new PulsarClient({ indexerUrl: "https://indexer.example" });
if (client.config.indexerUrl !== "https://indexer.example") throw new Error("CJS client misbehaved");
if (typeof decodeScVal !== "function" || typeof buildContractCall !== "function") {
  throw new Error("CJS named exports missing");
}
' || fail "CJS smoke test"
pass "CJS build requires and destructures named exports"

printf '\nPackage contents\n'
listing="$(npm pack --dry-run --json)"
rm -f ./*.tgz

node --input-type=module -e '
import { readFileSync } from "node:fs";
const listing = JSON.parse(process.argv[1]);
const files = listing[0].files.map((entry) => entry.path);

const unwanted = files.filter((path) =>
  path.startsWith("src/") ||
  path.startsWith("tests/") ||
  path.startsWith("tmp/") ||
  path.endsWith(".test.ts") ||
  path.endsWith(".test-d.ts") ||
  path.endsWith("tsconfig.json") ||
  path.endsWith("tsconfig.build.json"),
);
if (unwanted.length > 0) {
  console.error("tarball carries files it should not: " + unwanted.join(", "));
  process.exit(1);
}

for (const required of ["package.json", "LICENSE", "dist/index.js", "dist/index.cjs", "dist/index.d.ts"]) {
  if (!files.includes(required)) {
    console.error("tarball is missing " + required);
    process.exit(1);
  }
}

// Declaration maps point at sources the tarball does not ship, so shipping them
// is dead weight that also misleads a debugger.
const orphanMaps = files.filter((path) => path.endsWith(".d.ts.map"));
if (orphanMaps.length > 0) {
  console.error("tarball carries declaration maps with no sources: " + orphanMaps.join(", "));
  process.exit(1);
}
' "$listing" || fail "tarball contents"
pass "tarball carries dist and LICENSE, and nothing else"

printf '\nAll build checks passed\n\n'
