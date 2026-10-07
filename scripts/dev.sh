#!/usr/bin/env bash
#
# Run the indexer and the explorer together, the pair needed for the full local
# loop: the daemon polling testnet while the web app reads what it stores.
#
# Both run as children of this script. Ctrl-C stops both, and if either exits on
# its own the other is stopped too, so a half-dead stack never looks like a
# working one: an explorer serving pages against a dead indexer is the confusing
# failure this avoids.
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf 'dev: %s\n' "$1"; }
fail() { printf 'dev: %s\n' "$1" >&2; exit 1; }

local_env=".env.local"
[ -f "$local_env" ] || fail "$local_env not found; run ./scripts/setup.sh first"

# Export every assignment in .env.local for both children. allexport is scoped
# to the source so nothing later in this script is exported by accident.
set -o allexport
# shellcheck source=/dev/null
. "$local_env"
set +o allexport

[ -n "${PULSAR_INDEXER_ADMIN_TOKEN:-}" ] || fail "PULSAR_INDEXER_ADMIN_TOKEN is empty in $local_env; the indexer refuses to start without one (ADR-044)"

indexer_pid=""
web_pid=""

# Stop whichever child is still running. Each kill is tolerated failing, because
# the usual reason to be here is that one of them has already exited.
stop_children() {
  trap - INT TERM EXIT
  [ -n "$indexer_pid" ] && kill "$indexer_pid" 2>/dev/null || true
  [ -n "$web_pid" ] && kill "$web_pid" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap 'stop_children' INT TERM EXIT

say "starting the indexer on ${PULSAR_INDEXER_LISTEN_ADDR:-:8080}"
( cd indexer && exec go run ./cmd/pulsar-indexer ) &
indexer_pid=$!

say "starting the explorer on http://localhost:3000"
pnpm --filter ./apps/web dev &
web_pid=$!

say "both running. Ctrl-C stops them together"

# Return when the first child exits, whichever it is, then tear the other down
# through the EXIT trap and exit with the status that ended the pair.
status=0
wait -n "$indexer_pid" "$web_pid" || status=$?

if [ "$status" -eq 0 ]; then
  say "one process exited cleanly; stopping the other"
else
  printf 'dev: a process exited with status %d; stopping the other\n' "$status" >&2
fi
exit "$status"
