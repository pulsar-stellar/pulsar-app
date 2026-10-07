#!/usr/bin/env bash
#
# Register the pulsar-core showcase contract with a locally running indexer.
#
# The daemon registers PULSAR_INDEXER_BOOTSTRAP_CONTRACTS on first run, so this
# exists for the two cases that leaves: an indexer already running whose
# bootstrap list was empty, and wanting a contract tracked without a restart.
#
# Registration is idempotent (ADR-018): registering an already-tracked contract
# returns its existing record at 200 with no status change and no progress reset,
# so running this twice is safe and the second run is not an error.
#
# The request body field is contract_id, which is what the handler decodes. A
# body of {"id": ...} parses and leaves the field empty, so it fails validation
# rather than registering anything.
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf 'seed-showcase: %s\n' "$1"; }
fail() { printf 'seed-showcase: %s\n' "$1" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || fail "curl is not on PATH"

# .env.local supplies the token and the contract id when present. It is optional
# so the script also works against an indexer configured entirely by exported
# environment, which is how a deployed one looks.
if [ -f .env.local ]; then
  set -o allexport
  # shellcheck source=/dev/null
  . .env.local
  set +o allexport
fi

contract_id="${1:-${NEXT_PUBLIC_SHOWCASE_CONTRACT_ID:-}}"
[ -n "$contract_id" ] || fail "no contract id given and NEXT_PUBLIC_SHOWCASE_CONTRACT_ID is unset; pass one as the first argument"

token="${PULSAR_INDEXER_ADMIN_TOKEN:-}"
[ -n "$token" ] || fail "PULSAR_INDEXER_ADMIN_TOKEN is unset; the write routes require it (ADR-044)"

# The listen address may be host-less, as :8080 is, so a bare value gets
# localhost prepended to make a usable URL.
listen="${PULSAR_INDEXER_LISTEN_ADDR:-:8080}"
case "$listen" in
  :*) base="http://localhost${listen}" ;;
  http://*|https://*) base="$listen" ;;
  *) base="http://${listen}" ;;
esac

# Check the indexer is up first. Without this, a connection refused surfaces as
# a bare curl error and reads like the registration was rejected.
if ! curl -fsS --max-time 5 "${base}/health" >/dev/null 2>&1; then
  fail "no indexer answering at ${base}/health; start one with ./scripts/dev.sh"
fi

body="$(mktemp)"
trap 'rm -f "$body"' EXIT

status="$(
  curl -sS --max-time 10 \
    -o "$body" -w '%{http_code}' \
    -X POST "${base}/contracts" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    --data "{\"contract_id\":\"${contract_id}\"}"
)"

case "$status" in
  200)
    say "tracking ${contract_id}"
    say "the indexer begins filling its events table within one poll interval"
    ;;
  400)
    printf 'seed-showcase: the indexer rejected the request as invalid (400)\n' >&2
    cat "$body" >&2; printf '\n' >&2
    exit 1
    ;;
  401)
    fail "the indexer rejected the token (401); check PULSAR_INDEXER_ADMIN_TOKEN matches the running daemon's"
    ;;
  429)
    fail "rate limited by the write surface (429); wait and retry"
    ;;
  *)
    printf 'seed-showcase: unexpected response (%s)\n' "$status" >&2
    cat "$body" >&2; printf '\n' >&2
    exit 1
    ;;
esac
