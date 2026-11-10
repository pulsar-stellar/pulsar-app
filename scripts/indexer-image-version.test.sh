#!/usr/bin/env bash
#
# Tests for indexer-image-version.sh, the gate between a pushed git tag and the
# VERSION build arg the indexer image stamps into its binary with -ldflags.
set -euo pipefail

cd "$(dirname "$0")"
failures=0
cases=0

# check <tag> <want> ; want is the printed version, or "reject".
check() {
  local tag="$1" want="$2" got status=0
  cases=$((cases + 1))
  got="$(./indexer-image-version.sh "$tag" 2>/dev/null)" || status=$?
  if [ "$want" = reject ]; then
    [ "$status" -ne 0 ] && return
    printf 'FAIL %q: accepted as %q, want rejected\n' "$tag" "$got" >&2
  else
    [ "$status" -eq 0 ] && [ "$got" = "$want" ] && return
    printf 'FAIL %q: got %q (exit %d), want %q\n' "$tag" "$got" "$status" "$want" >&2
  fi
  failures=$((failures + 1))
}

check indexer-v0.1.0 0.1.0
check indexer-v1.12.3 1.12.3
check indexer-v0.2.0-rc.1 0.2.0-rc.1
check indexer-v0.1 reject
check indexer-v01.2.3 reject
check v0.1.0 reject
check sdk-v0.2.0 reject
check 'indexer-v0.1.0 -X main.x=y' reject
check 'indexer-v0.1.0$(id)' reject
check 'indexer-v0.1.0-' reject
check indexer-v0.1.0+build.5 reject
check '' reject

if [ "$failures" -ne 0 ]; then
  printf 'indexer-image-version: %d of %d case(s) failed\n' "$failures" "$cases" >&2
  exit 1
fi
printf 'indexer-image-version: %d case(s) passed\n' "$cases"
