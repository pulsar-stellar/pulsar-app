#!/usr/bin/env bash
#
# Tests for indexer-image-tag-absent.sh, the guard that keeps the publish
# workflow from overwriting a released version tag.
#
# skopeo exits 1 for a missing tag and for a timeout alike, so the guard reads
# its message. Each case runs the real script against a stub skopeo that
# replays a message for that situation. The missing-tag, timeout, and DNS
# messages were captured from skopeo 1.13.3, the runner's version; the others
# are labelled where they are not.
set -euo pipefail

cd "$(dirname "$0")"
script="$PWD/indexer-image-tag-absent.sh"

sandbox="$(mktemp -d)"
trap 'rm -rf "$sandbox"' EXIT

failures=0
cases=0

# check <label> <want-exit> <stub-exit> <stub-stderr>
check() {
  local label="$1" want="$2" stub_exit="$3" stub_err="$4" got=0
  cases=$((cases + 1))
  printf '#!/usr/bin/env bash\nprintf %%s %q >&2\nexit %d\n' "$stub_err" "$stub_exit" > "$sandbox/skopeo"
  chmod +x "$sandbox/skopeo"
  PATH="$sandbox:$PATH" "$script" ghcr.io/pulsar-stellar/indexer:0.1.0 > /dev/null 2>&1 || got=$?
  if [ "$got" -ne "$want" ]; then
    printf 'FAIL %s: exit %d, want %d\n' "$label" "$got" "$want" >&2
    failures=$((failures + 1))
  fi
}

prefix='time="2026-11-10T14:00:53Z" level=fatal msg="Error parsing image name \"docker://ghcr.io/pulsar-stellar/indexer:0.1.0\": '

# The two answers that mean the tag is not there. Missing tag: captured from
# ghcr.io. Unknown repository: the CNCF distribution registry's NAME_UNKNOWN
# wording, not captured from ghcr.io, which may answer a new package otherwise.
check "missing tag" 0 1 "${prefix}reading manifest 0.1.0 in ghcr.io/pulsar-stellar/indexer: manifest unknown\""
check "unknown repository (registry:2 wording)" 0 1 "${prefix}reading manifest 0.1.0 in ghcr.io/pulsar-stellar/indexer: name unknown: repository name not known to registry\""
# ghcr.io, captured with skopeo 1.13.3 on 2026-11-10: an authenticated request for a
# package that does not exist yet gets the same answer as a missing tag.
check "package not created (ghcr.io, captured)" 0 1 "${prefix}reading manifest 0.1.0 in ghcr.io/pulsar-stellar/indexer: manifest unknown\""

# The tag exists: never overwrite it.
check "tag exists" 1 0 ""

# Everything else fails closed, so a re-run cannot push over a released tag.
check "token request timed out" 2 1 "${prefix}Get \\\"https://ghcr.io/token?scope=repository%3Apulsar-stellar%2Findexer%3Apull&service=ghcr.io\\\": read tcp 172.17.0.3:47514->140.82.121.33:443: read: connection timed out\""
check "dns failure" 2 1 "${prefix}pinging container registry ghcr.io: Get \\\"https://ghcr.io/v2/\\\": dial tcp: lookup ghcr.io: no such host\""
check "token denied (constructed)" 2 1 "${prefix}reading manifest 0.1.0 in ghcr.io/pulsar-stellar/indexer: denied: requested access to the resource is denied\""
check "registry 5xx (constructed)" 2 1 "${prefix}reading manifest 0.1.0 in ghcr.io/pulsar-stellar/indexer: received unexpected HTTP status: 503 Service Unavailable\""
check "silent failure" 2 1 ""
# ghcr.io, captured: the anonymous answer for the same reference. The workflow logs in
# first, but if login silently failed this must still stop the push.
check "anonymous token refused (ghcr.io, captured)" 2 1 "${prefix}Requesting bearer token: invalid status code from registry 403 (Forbidden)\""

# No reference is a usage error, not an answer.
cases=$((cases + 1))
if PATH="$sandbox:$PATH" "$script" > /dev/null 2>&1; then
  printf 'FAIL no reference: succeeded, want an error\n' >&2
  failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
  printf 'indexer-image-tag-absent: %d of %d case(s) failed\n' "$failures" "$cases" >&2
  exit 1
fi
printf 'indexer-image-tag-absent: %d case(s) passed\n' "$cases"
