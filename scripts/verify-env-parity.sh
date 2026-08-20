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

# process.env.NAME, process.env['NAME'], os.Getenv("NAME"), os.LookupEnv("NAME")
referenced="$(
  grep -rhoE \
    "process\.env\.[A-Z][A-Z0-9_]*|process\.env\[['\"][A-Z][A-Z0-9_]*['\"]\]|os\.(Getenv|LookupEnv)\(\"[A-Z][A-Z0-9_]*\"\)" \
    "${search_paths[@]}" \
    --include='*.ts' --include='*.tsx' --include='*.go' \
    --exclude-dir=node_modules --exclude-dir=.next --exclude-dir=dist \
    2>/dev/null \
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
