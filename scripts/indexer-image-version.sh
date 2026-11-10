#!/usr/bin/env bash
#
# Prints the indexer image version for a git tag, or fails.
#
#   ./scripts/indexer-image-version.sh indexer-v0.1.0   # prints 0.1.0
#
# The publish workflow passes the result as the image tag and as the VERSION
# build arg, which the Dockerfile stamps into the binary through -ldflags, so
# anything but a plain semantic version is refused here, before the build: a
# space would make -ldflags malformed, and a registry tag cannot carry `+`.
set -euo pipefail

tag="${1:-}"
semver='^indexer-v((0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?)$'

if [[ ! "$tag" =~ $semver ]]; then
  printf 'indexer-image-version: %q is not indexer-v<major>.<minor>.<patch>[-<prerelease>]\n' "$tag" >&2
  exit 1
fi
printf '%s\n' "${BASH_REMATCH[1]}"
