#!/usr/bin/env bash
#
# Every environment variable referenced in code must appear in .env.example.
#
# Scans the TypeScript, Go, and web sources for env lookups, then fails if any
# name it finds is missing from .env.example. A variable that exists only in the
# example file is fine: defaults may be declared ahead of the code that reads
# them.
set -euo pipefail

cd "$(dirname "$0")/.."

example=".env.example"
if [ ! -f "$example" ]; then
  echo "verify-env-parity: $example not found" >&2
  exit 1
fi

declared="$(grep -oE '^[A-Z][A-Z0-9_]*=' "$example" | tr -d '=' | sort -u)"

search_paths=()
for path in packages apps indexer; do
  [ -d "$path" ] && search_paths+=("$path")
done

if [ ${#search_paths[@]} -eq 0 ]; then
  echo "verify-env-parity: no source directories yet, nothing to check"
  exit 0
fi

# Three accessor shapes are recognised:
#
#   process.env.NAME and process.env['NAME']            TypeScript
#   os.Getenv("NAME") and os.LookupEnv("NAME")          Go, direct
#   required(getenv, "NAME") and friends                Go, injected
#
# The third exists because internal/config takes a Getenv function rather than
# calling os.Getenv, so it has no direct call sites for this to match. Those
# helpers are the injected design's equivalent of an os.Getenv call, and without
# them the indexer's variables are invisible to this check while it reports a
# clean pass.
pattern="process\.env\.[A-Z][A-Z0-9_]*"
pattern="$pattern|process\.env\[['\"][A-Z][A-Z0-9_]*['\"]\]"
pattern="$pattern|os\.(Getenv|LookupEnv)\(\"[A-Z][A-Z0-9_]*\"\)"
pattern="$pattern|(required|withDefault|positiveInt|optionalPositiveInt|boolean|contractList)\([[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*,[[:space:]]*\"[A-Z][A-Z0-9_]*\""

# Line comments are stripped before names are extracted, so a helper call shown
# in a comment does not count as a reference.
referenced="$(
  grep -rhE "$pattern" \
    "${search_paths[@]}" \
    --include='*.ts' --include='*.tsx' --include='*.go' \
    --exclude-dir=node_modules --exclude-dir=.next --exclude-dir=dist \
    2>/dev/null \
  | sed 's://.*::' \
  | grep -oE "$pattern" \
  | grep -oE '[A-Z][A-Z0-9_]{2,}' \
  | grep -vE '^(NODE_ENV)$' \
  | sort -u || true
)"

if [ -z "$referenced" ]; then
  echo "verify-env-parity: no environment lookups found in source"
  exit 0
fi

missing=""
while IFS= read -r name; do
  if ! printf '%s\n' "$declared" | grep -qx "$name"; then
    missing="${missing}${name}"$'\n'
  fi
done <<< "$referenced"

if [ -n "$missing" ]; then
  echo "verify-env-parity: referenced in code but missing from $example:" >&2
  printf '%s' "$missing" >&2
  exit 1
fi

count="$(printf '%s\n' "$referenced" | wc -l | tr -d ' ')"
echo "verify-env-parity: $count variable(s) referenced, all present in $example"
