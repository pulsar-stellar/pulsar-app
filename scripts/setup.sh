#!/usr/bin/env bash
#
# One command to get a working local checkout: install the workspace's
# dependencies and create the .env.local every sub-stack reads.
#
# The env file is not a plain copy. .env.example ships
# PULSAR_INDEXER_ADMIN_TOKEN empty, because a template that is safe to publish
# cannot carry a secret, and the indexer refuses to start on a token shorter
# than sixteen characters (ADR-044). A plain cp would therefore produce a
# checkout whose indexer will not boot. This generates a real random token for
# the local file and copies everything else through unchanged.
#
# An existing .env.local is never overwritten. It holds values the developer
# chose, possibly real credentials, and silently replacing them would be the
# worst thing this script could do.
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf 'setup: %s\n' "$1"; }
fail() { printf 'setup: %s\n' "$1" >&2; exit 1; }

# Required tooling. Go is checked because the indexer is a separate module that
# `pnpm install` knows nothing about, so a checkout can look complete and still
# be unable to run half the stack.
for tool in pnpm go; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not on PATH; see the prerequisites table in docs/requirements.md"
done

example=".env.example"
local_env=".env.local"
[ -f "$example" ] || fail "$example not found; run this from a full checkout"

# A long random hex token. openssl is near universal, but it is not guaranteed,
# and /dev/urandom is, so the fallback keeps this working on a minimal machine.
#
# The fallback reads a fixed 32 bytes with od rather than piping a stream into
# `head -c`. Under pipefail, head closing the pipe early sends SIGPIPE to the
# upstream process and the whole pipeline reports 141, so the obvious
# `tr -dc ... < /dev/urandom | head -c 64` spelling fails the script it is
# meant to serve.
generate_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -vtx1 -N32 /dev/urandom | tr -d ' \n'
  fi
}

if [ -f "$local_env" ]; then
  say "$local_env already exists, leaving it untouched"
else
  token="$(generate_token)"
  # The substitution is anchored to the whole line so a commented example of the
  # same variable is not rewritten, and the token is passed through a shell
  # variable rather than inlined so sed never sees it as a pattern.
  awk -v token="$token" '
    /^PULSAR_INDEXER_ADMIN_TOKEN=/ { print "PULSAR_INDEXER_ADMIN_TOKEN=" token; next }
    { print }
  ' "$example" > "$local_env"
  chmod 600 "$local_env"
  say "wrote $local_env from $example with a generated PULSAR_INDEXER_ADMIN_TOKEN"
fi

say "installing workspace dependencies"
pnpm install

say "done. Next: ./scripts/dev.sh runs the indexer and the explorer together"
