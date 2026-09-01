#!/usr/bin/env bash
#
# Negative control for verify-env-parity.sh.
#
# The parity script passes when it finds no environment lookups at all, which
# is the correct answer today and would also be the answer if its regex quietly
# stopped matching. A green from a check that scanned nothing looks exactly like
# a green from a check that scanned everything, so this asserts the script
# actually fails on input it ought to reject.
#
# Fixture cases run against a sandbox copy of the script, so nothing here writes
# into the repository being checked.
set -euo pipefail

cd "$(dirname "$0")/.."
repo_root="$PWD"
script="scripts/verify-env-parity.sh"

sandbox="$(mktemp -d)"
trap 'rm -rf "$sandbox"' EXIT

failures=0

# run_sandbox <case-name> <expected-exit> ; fixture files are staged by the
# caller into $sandbox beforehand.
run_sandbox() {
  local name="$1" expected="$2" actual=0 output=""
  output="$("$sandbox/$script" 2>&1)" || actual=$?

  if [ "$actual" -eq "$expected" ]; then
    printf 'ok   %s (exit %d)\n' "$name" "$actual"
  else
    printf 'FAIL %s: expected exit %d, got %d\n' "$name" "$expected" "$actual" >&2
    printf '     output: %s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}

reset_sandbox() {
  rm -rf "${sandbox:?}/"*
  mkdir -p "$sandbox/scripts" "$sandbox/indexer" "$sandbox/packages"
  cp "$repo_root/$script" "$sandbox/$script"
  cat > "$sandbox/.env.example" <<'EOF'
PULSAR_INDEXER_RPC_URL=https://soroban-testnet.stellar.org
NEXT_PUBLIC_PULSAR_INDEXER_URL=http://localhost:8080
EOF
}

# A Go lookup naming a variable absent from .env.example must fail. This is the
# assertion that proves the Go half of the regex is live.
reset_sandbox
cat > "$sandbox/indexer/main.go" <<'EOF'
package main

import "os"

func main() { _ = os.Getenv("PULSAR_INDEXER_NOT_IN_EXAMPLE") }
EOF
run_sandbox "go lookup missing from .env.example" 1

# The same shape with a declared variable must pass, so the failure above is
# about the missing declaration and not about Go files failing outright.
reset_sandbox
cat > "$sandbox/indexer/main.go" <<'EOF'
package main

import "os"

func main() { _ = os.Getenv("PULSAR_INDEXER_RPC_URL") }
EOF
run_sandbox "go lookup declared in .env.example" 0

# os.LookupEnv is the other Go accessor the regex claims to cover.
reset_sandbox
cat > "$sandbox/indexer/main.go" <<'EOF'
package main

import "os"

func main() { _, _ = os.LookupEnv("PULSAR_INDEXER_ALSO_MISSING") }
EOF
run_sandbox "go LookupEnv missing from .env.example" 1

# The TypeScript half of the same regex needs its own control, or half the
# scanner can break without any test noticing.
reset_sandbox
cat > "$sandbox/packages/client.ts" <<'EOF'
export const url = process.env.PULSAR_TS_NOT_IN_EXAMPLE;
EOF
run_sandbox "ts lookup missing from .env.example" 1

reset_sandbox
cat > "$sandbox/packages/client.ts" <<'EOF'
export const url = process.env.NEXT_PUBLIC_PULSAR_INDEXER_URL;
EOF
run_sandbox "ts lookup declared in .env.example" 0

# Sources present, no lookups in them. The script passes here by design, and
# pinning it means a future change to that branch is a deliberate one.
reset_sandbox
cat > "$sandbox/indexer/main.go" <<'EOF'
package main

func main() {}
EOF
run_sandbox "sources with no lookups" 0

# Finally the real tree against the real .env.example, which must pass.
real_exit=0
real_output="$("$repo_root/$script" 2>&1)" || real_exit=$?
if [ "$real_exit" -eq 0 ]; then
  printf 'ok   repository sources against .env.example (exit 0)\n'
else
  printf 'FAIL repository sources against .env.example: exit %d\n' "$real_exit" >&2
  printf '     output: %s\n' "$real_output" >&2
  failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
  printf '\nverify-env-parity.test: %d assertion(s) failed\n' "$failures" >&2
  exit 1
fi

printf '\nverify-env-parity.test: all assertions passed\n'
