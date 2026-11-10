#!/usr/bin/env bash
#
# Succeeds only when the registry itself says an image tag does not exist.
#
#   ./scripts/indexer-image-tag-absent.sh ghcr.io/pulsar-stellar/indexer:0.1.0
#
#   exit 0  the registry answered that the tag is absent, so pushing is safe
#   exit 1  the tag exists, and version tags are never overwritten (ADR-047)
#   exit 2  anything else: a timeout, a DNS failure, a denied token, a 5xx
#
# It fails closed. skopeo exits 1 for a missing tag and for a dropped connection
# alike, so treating any failure as "absent" would let a transient error on a
# re-run push over a released version. Only the registry's not-found answers
# count as absent: MANIFEST_UNKNOWN for a missing tag, seen from ghcr.io, and
# NAME_UNKNOWN, the distribution spec's unknown-repository code. What ghcr.io
# answers an authenticated push token for a package not created yet is not
# confirmed; if it is neither, this refuses, which is the intended side to err.
set -euo pipefail

ref="${1:?usage: indexer-image-tag-absent.sh <registry>/<name>:<tag>}"

err="$(mktemp)"
trap 'rm -f "$err"' EXIT

if skopeo inspect --raw "docker://$ref" > /dev/null 2> "$err"; then
  printf 'indexer-image-tag-absent: %s already exists; version tags are never overwritten\n' "$ref" >&2
  exit 1
fi

if grep -qE 'manifest unknown|name unknown' "$err"; then
  exit 0
fi

cat "$err" >&2
printf '\nindexer-image-tag-absent: could not confirm %s is absent; refusing to push\n' "$ref" >&2
exit 2
