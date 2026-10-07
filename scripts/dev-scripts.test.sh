#!/usr/bin/env bash
#
# Tests for the developer scripts in this directory.
#
# The scripts shell out to pnpm, go, and curl, none of which should run for
# real here: a test that installed dependencies or hit a network would be slow
# and would fail for reasons unrelated to the script. So each case builds a
# sandbox with a PATH holding only the externals the script under test needs,
# with stubs for the expensive ones. The scripts are copied in unmodified, so
# what runs is the real thing rather than a test-only code path.
#
# What is covered: the guard clauses, setup.sh's file handling (which is the one
# piece with real logic), and seed-showcase.sh's reading of HTTP status codes.
# What is not: dev.sh's process orchestration past its guards, and the actual
# build commands, both of which need the real toolchain and are exercised by
# running them.
set -euo pipefail

cd "$(dirname "$0")/.."
repo_root="$PWD"

sandbox="$(mktemp -d)"
trap 'rm -rf "$sandbox"' EXIT

failures=0
cases=0

# link_tools <dir> <tool>... ; symlinks real binaries into a minimal PATH dir.
link_tools() {
  local dir="$1"; shift
  mkdir -p "$dir"
  local tool path
  for tool in "$@"; do
    path="$(command -v "$tool" 2>/dev/null || true)"
    [ -n "$path" ] && ln -sf "$path" "$dir/$tool"
  done
}

# stub <dir> <name> <body> ; writes an executable stub.
stub() {
  local dir="$1" name="$2" body="$3"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\n%s\n' "$body" > "$dir/$name"
  chmod +x "$dir/$name"
}

# reset <script> ; fresh sandbox holding one script and a .env.example.
reset() {
  local script="$1"
  rm -rf "${sandbox:?}/"*
  mkdir -p "$sandbox/scripts" "$sandbox/bin"
  cp "$repo_root/scripts/$script" "$sandbox/scripts/$script"
  # The real template, so a change to its shape is caught here too.
  cp "$repo_root/.env.example" "$sandbox/.env.example"
  # bash and dirname are what the shebang and the cd to the repo root need, so
  # they are part of any usable PATH. openssl is deliberately absent: leaving it
  # out exercises setup.sh's /dev/urandom fallback.
  link_tools "$sandbox/bin" bash dirname awk chmod tr head cat mktemp rm sed grep od
}

# run <name> <script> <expected-exit> [args...] ; runs with the sandbox PATH only.
run() {
  local name="$1" script="$2" expected="$3"; shift 3
  local actual=0 output=""
  cases=$((cases + 1))
  output="$(cd "$sandbox" && PATH="$sandbox/bin" "$sandbox/scripts/$script" "$@" 2>&1)" || actual=$?
  if [ "$actual" -eq "$expected" ]; then
    printf 'ok   %s (exit %d)\n' "$name" "$actual"
  else
    printf 'FAIL %s: expected exit %d, got %d\n' "$name" "$expected" "$actual" >&2
    printf '     output: %s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}

# --- setup.sh ---

# No go on PATH. The guard exists because pnpm install says nothing about the
# indexer's separate module, so a checkout can look complete and still not run.
reset setup.sh
stub "$sandbox/bin" pnpm 'exit 0'
run "setup.sh fails when go is missing" setup.sh 1

# Happy path with no openssl, which exercises the /dev/urandom fallback.
reset setup.sh
stub "$sandbox/bin" pnpm 'exit 0'
stub "$sandbox/bin" go 'exit 0'
run "setup.sh creates .env.local" setup.sh 0
cases=$((cases + 1))
if [ -f "$sandbox/.env.local" ]; then
  token="$(grep '^PULSAR_INDEXER_ADMIN_TOKEN=' "$sandbox/.env.local" | cut -d= -f2-)"
  if [ ${#token} -ge 16 ]; then
    printf 'ok   setup.sh generates a token of at least 16 characters (%d)\n' "${#token}"
  else
    printf 'FAIL setup.sh generated a %d character token, want at least 16\n' "${#token}" >&2
    failures=$((failures + 1))
  fi
else
  printf 'FAIL setup.sh did not create .env.local\n' >&2
  failures=$((failures + 1))
fi

# An existing .env.local holds the developer's own values, so it must survive.
reset setup.sh
stub "$sandbox/bin" pnpm 'exit 0'
stub "$sandbox/bin" go 'exit 0'
printf 'PULSAR_INDEXER_ADMIN_TOKEN=do-not-clobber-me\n' > "$sandbox/.env.local"
run "setup.sh leaves an existing .env.local alone" setup.sh 0
cases=$((cases + 1))
if grep -q 'do-not-clobber-me' "$sandbox/.env.local"; then
  printf 'ok   setup.sh preserved the existing token\n'
else
  printf 'FAIL setup.sh overwrote an existing .env.local\n' >&2
  failures=$((failures + 1))
fi

# --- build-all.sh ---

reset build-all.sh
stub "$sandbox/bin" pnpm 'exit 0'
run "build-all.sh fails when go is missing" build-all.sh 1

# --- dev.sh ---

reset dev.sh
run "dev.sh fails without .env.local" dev.sh 1

reset dev.sh
printf 'PULSAR_INDEXER_ADMIN_TOKEN=\n' > "$sandbox/.env.local"
run "dev.sh fails on an empty admin token" dev.sh 1

# --- seed-showcase.sh ---

# curl_stub emits <status> for the POST and succeeds or fails the health probe.
# It reads the -o destination out of its own arguments, the way real curl does,
# so the script's handling of the body file is exercised rather than bypassed.
curl_stub() {
  cat <<'STUB'
health_ok="${STUB_HEALTH_OK:-1}"
status="${STUB_POST_STATUS:-200}"
for arg in "$@"; do
  case "$arg" in */health) [ "$health_ok" = "1" ] && exit 0 || exit 7 ;; esac
done
out=""
prev=""
for arg in "$@"; do
  [ "$prev" = "-o" ] && out="$arg"
  prev="$arg"
done
[ -n "$out" ] && printf '{"stub":true}' > "$out"
printf '%s' "$status"
STUB
}

reset seed-showcase.sh
stub "$sandbox/bin" curl "$(curl_stub)"
printf 'PULSAR_INDEXER_ADMIN_TOKEN=\n' > "$sandbox/.env.local"
run "seed-showcase.sh fails on an empty admin token" seed-showcase.sh 1 CTESTCONTRACTID

reset seed-showcase.sh
stub "$sandbox/bin" curl "$(curl_stub)"
printf 'PULSAR_INDEXER_ADMIN_TOKEN=0123456789abcdef0123456789abcdef\n' > "$sandbox/.env.local"
run "seed-showcase.sh fails with no contract id" seed-showcase.sh 1

# An indexer that is not running must say so, not report a rejected registration.
reset seed-showcase.sh
stub "$sandbox/bin" curl "$(curl_stub)"
printf 'PULSAR_INDEXER_ADMIN_TOKEN=0123456789abcdef0123456789abcdef\n' > "$sandbox/.env.local"
cases=$((cases + 1))
output="$(cd "$sandbox" && PATH="$sandbox/bin" STUB_HEALTH_OK=0 "$sandbox/scripts/seed-showcase.sh" CTEST 2>&1)" && actual=0 || actual=$?
if [ "$actual" -eq 1 ] && printf '%s' "$output" | grep -q 'no indexer answering'; then
  printf 'ok   seed-showcase.sh reports an unreachable indexer (exit 1)\n'
else
  printf 'FAIL seed-showcase.sh unreachable case: exit %d, output: %s\n' "$actual" "$output" >&2
  failures=$((failures + 1))
fi

# The status codes the write surface actually returns (ADR-018, ADR-044).
for spec in "200:0:registers" "401:1:rejects a bad token" "429:1:reports rate limiting" "400:1:reports a validation failure" "503:1:reports an unexpected status"; do
  code="${spec%%:*}"; rest="${spec#*:}"; want="${rest%%:*}"; label="${rest#*:}"
  reset seed-showcase.sh
  stub "$sandbox/bin" curl "$(curl_stub)"
  printf 'PULSAR_INDEXER_ADMIN_TOKEN=0123456789abcdef0123456789abcdef\n' > "$sandbox/.env.local"
  cases=$((cases + 1))
  actual=0
  (cd "$sandbox" && PATH="$sandbox/bin" STUB_POST_STATUS="$code" "$sandbox/scripts/seed-showcase.sh" CTEST >/dev/null 2>&1) || actual=$?
  if [ "$actual" -eq "$want" ]; then
    printf 'ok   seed-showcase.sh %s on %s (exit %d)\n' "$label" "$code" "$actual"
  else
    printf 'FAIL seed-showcase.sh on %s: expected exit %d, got %d\n' "$code" "$want" "$actual" >&2
    failures=$((failures + 1))
  fi
done

printf '\n'
if [ "$failures" -ne 0 ]; then
  printf 'dev-scripts: %d of %d case(s) failed\n' "$failures" "$cases" >&2
  exit 1
fi
printf 'dev-scripts: %d case(s) passed\n' "$cases"
