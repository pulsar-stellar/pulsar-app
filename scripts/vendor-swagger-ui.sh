#!/usr/bin/env bash
#
# Vendors a pinned swagger-ui-dist release into the indexer for GET /docs
# (ADR-059).
#
#   ./scripts/vendor-swagger-ui.sh 5.34.1
#
# It fetches the release tarball from the npm registry, refuses it unless its
# sha512 matches the integrity the registry publishes for that version, and
# copies out only what /docs needs: the bundle and its stylesheet, plus the
# licence texts that must travel with them (Apache-2.0 LICENSE and NOTICE, and
# the licences of the third-party code compiled into the bundle). It then
# writes VERSION and SHA256SUMS beside them. The indexer embeds that directory,
# and a Go test fails if any embedded file stops matching SHA256SUMS, so a hand
# edit cannot ship unnoticed.
set -euo pipefail

version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'vendor-swagger-ui: %q is not a plain <major>.<minor>.<patch> version\n' "$version" >&2
  exit 1
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$root/indexer/internal/api/docsui/vendor"
files=(swagger-ui-bundle.js swagger-ui.css LICENSE NOTICE swagger-ui-bundle.js.LICENSE.txt)

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

meta="$(curl --proto =https -fsSL "https://registry.npmjs.org/swagger-ui-dist/$version")"
tarball="$(jq -r '.dist.tarball' <<<"$meta")"
integrity="$(jq -r '.dist.integrity' <<<"$meta")"
if [[ "$tarball" != "https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-$version.tgz" || "$integrity" != sha512-* ]]; then
  echo "vendor-swagger-ui: unexpected registry metadata for $version" >&2
  exit 1
fi

curl --proto =https -fsSL "$tarball" -o "$work/pkg.tgz"
actual="sha512-$(openssl dgst -sha512 -binary "$work/pkg.tgz" | openssl base64 -A)"
if [[ "$actual" != "$integrity" ]]; then
  echo "vendor-swagger-ui: tarball integrity $actual does not match the registry's $integrity" >&2
  exit 1
fi

tar --no-same-owner --no-same-permissions -xzf "$work/pkg.tgz" -C "$work" "${files[@]/#/package/}"

# A tarball entry can be a link. cp would follow it and copy whatever it points
# at, such as a key from the home directory, into the repository, so only
# regular files are accepted.
for f in "${files[@]}"; do
  if [[ -L "$work/package/$f" || ! -f "$work/package/$f" ]]; then
    echo "vendor-swagger-ui: package/$f in the tarball is not a regular file" >&2
    exit 1
  fi
done

mkdir -p "$dest"
for f in "${files[@]}"; do
  cp "$work/package/$f" "$dest/$f"
done
printf '%s\n' "$version" >"$dest/VERSION"
(cd "$dest" && sha256sum "${files[@]}" | LC_ALL=C sort -k2 >SHA256SUMS)

echo "vendor-swagger-ui: swagger-ui-dist $version vendored into ${dest#"$root"/}"
