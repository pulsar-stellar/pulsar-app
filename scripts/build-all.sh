#!/usr/bin/env bash
#
# Build every shippable artifact, in dependency order.
#
# The SDK goes first because the explorer imports it and typechecks against its
# emitted declarations, so building the web app on a cold checkout fails without
# it. The indexer builds with CGO_ENABLED=0, the static build ADR-008 settled so
# the binary carries no C toolchain dependency and the container image can be
# built from scratch. The explorer goes last.
#
# Fails on the first error rather than reporting at the end, so the output ends
# at the thing that broke.
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf 'build-all: %s\n' "$1"; }
fail() { printf 'build-all: %s\n' "$1" >&2; exit 1; }

for tool in pnpm go; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not on PATH; see the prerequisites table in docs/requirements.md"
done

say "building @pulsar-stellar/sdk"
pnpm --filter ./packages/sdk build

say "building the indexer (CGO_ENABLED=0)"
( cd indexer && CGO_ENABLED=0 go build ./... )

say "building the explorer"
pnpm --filter ./apps/web build

say "all three built"
