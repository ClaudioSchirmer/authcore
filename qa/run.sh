#!/usr/bin/env bash
# THE runner. One entry point, for a dev and for CI alike:
#
#   ./qa/run.sh              every lane, fail-fast on the first RED
#   ./qa/run.sh --all        every lane, exhaustive (no fail-fast)
#   ./qa/run.sh tenant       a subset — a convenience on this runner, never a rival script
#   ./qa/run.sh role         likewise
#
# It executes specs/qa/tenant-contract/plan.md, specs/qa/permission-contract/plan.md
# and specs/qa/role-contract/plan.md. Adding a lane means adding its
# name to SUITES below in the same change that creates the file: a qa/*.sh no
# lane names is a suite nobody runs.

set -uo pipefail

# First act: resolve the project root from THIS script's location, so
# devops/docker-compose.yml, migrations/ and the build resolve no matter which
# directory the run was started from.
cd "$(dirname "$0")/.." || exit 2

# domain runs LAST on purpose: its final case (P6) archives the wildcard catalog
# row and revokes the bootstrap admin's only grant, so nothing can authenticate
# after it. security must therefore complete first — and every entity lane before
# that, since each of them signs in as the same operator.
SUITES=(tenant permission role security domain)

FAIL_FAST=1
REQUESTED=()
for arg in "$@"; do
  case "$arg" in
    --all) FAIL_FAST=0 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) REQUESTED+=("$arg") ;;
  esac
done
[ ${#REQUESTED[@]} -gt 0 ] && SUITES=("${REQUESTED[@]}")

c_green=$'\033[32m'; c_red=$'\033[31m'; c_yellow=$'\033[33m'; c_dim=$'\033[2m'; c_bold=$'\033[1m'; c_off=$'\033[0m'
die() { printf '%s✗ precondition%s %s\n' "$c_red" "$c_off" "$1" >&2; exit 2; }

# ---------------------------------------------------------------------------
# Preconditions. Each is a loud failure, never a silent skip: a suite that
# quietly did not run reads exactly like a suite that passed.
# ---------------------------------------------------------------------------
command -v jq       >/dev/null || die "jq is required (every assertion reads JSON)."
command -v curl     >/dev/null || die "curl is required."
command -v openssl  >/dev/null || die "openssl is required (the 401 lane forges tokens to be refused)."
command -v go       >/dev/null || die "go is required (the service under test is built from source)."
command -v docker   >/dev/null || die "docker is required (the suite provisions its own throwaway database)."

export QA_RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
export QA_RUN_TAG="$$"
export LOG_DIR="qa/.logs/${QA_RUN_ID}"
export QA_PORT="${QA_PORT:-8099}"
export BASE="http://localhost:${QA_PORT}"
mkdir -p "$LOG_DIR"

REPORT=qa/qa-report.md
PLAN='specs/qa/tenant-contract/plan.md + specs/qa/permission-contract/plan.md + specs/qa/role-contract/plan.md'
COMPOSE=devops/docker-compose.yml
PGSVC=postgres
QA_DB=authcore_qa_db
BIN="bin/authcore-qa-${QA_RUN_TAG}"
SERVER_LOG="${LOG_DIR}/server.log"
BUILD_TAGS='postgres'          # relational.dialect: postgres; no transport: block → no transport tag
PIN="$(go list -m -f '{{.Version}}' github.com/ClaudioSchirmer/omnicore 2>/dev/null || echo unknown)"
START_TS=$(date +%s)

SERVER_PID=''
declare -a R_PASS R_FAIL R_SKIP R_VERDICT R_TIME
for i in "${!SUITES[@]}"; do R_PASS[$i]=0; R_FAIL[$i]=0; R_SKIP[$i]=0; R_VERDICT[$i]='—'; R_TIME[$i]='—'; done

# ---------------------------------------------------------------------------
# The report is rendered LIVE — rewritten in full after every lane — so a run
# killed halfway still leaves on disk what it had proven. And a trap stamps an
# ABORTED verdict, because yesterday's green report surviving today's crash reads
# as today's outcome, which is worse than no report at all.
# ---------------------------------------------------------------------------
ABORT_REASON=''
render_report() {
  local footer="$1" total_p=0 total_f=0 total_s=0
  {
    printf '# QA run report — authcore\n\n'
    printf -- '- **When:** %s\n' "$(date '+%Y-%m-%d %H:%M:%S %Z')"
    printf -- '- **Plan:** `%s`\n' "$PLAN"
    printf -- '- **Pin:** omnicore `%s`\n' "$PIN"
    printf -- '- **Built:** `go build -tags '"'"'%s'"'"'` — engine postgres, no transport tag\n' "$BUILD_TAGS"
    printf -- '- **Profile:** `APP_PROFILE=dev` + `OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml` on port `%s`\n' "$QA_PORT"
    printf -- '- **Data hygiene:** dedicated throwaway database `%s`, dropped and recreated this run. `authcore_db` is never written to.\n' "$QA_DB"
    printf -- '- **Run id:** `%s` — logs under `%s/`\n\n' "$QA_RUN_ID" "$LOG_DIR"
    printf '## Matrix\n\n| Suite | Pass | Fail | Skip | Verdict | Time |\n|---|---:|---:|---:|---|---:|\n'
    for i in "${!SUITES[@]}"; do
      printf '| `%s` | %s | %s | %s | %s | %s |\n' \
        "${SUITES[$i]}" "${R_PASS[$i]}" "${R_FAIL[$i]}" "${R_SKIP[$i]}" "${R_VERDICT[$i]}" "${R_TIME[$i]}"
      total_p=$((total_p + R_PASS[i])); total_f=$((total_f + R_FAIL[i])); total_s=$((total_s + R_SKIP[i]))
    done
    printf '| **total** | **%s** | **%s** | **%s** | | |\n\n' "$total_p" "$total_f" "$total_s"
    printf '> A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a passing one.\n'
    printf '> SKIP keeps its own column and is never folded into the pass count.\n\n'
    if [ "$total_f" -gt 0 ]; then
      printf '## Failures\n\n'
      for s in "${SUITES[@]}"; do
        local f="${LOG_DIR}/failures-${s}.txt"
        [ -s "$f" ] || continue
        printf '### Suite `%s`\n\n' "$s"
        head -c 20000 "$f"
        printf '\nFull log: `%s/%s.log`\n\n' "$LOG_DIR" "$s"
      done
    fi
    printf '## Verdict\n\n%s\n' "$footer"
  } > "$REPORT"
}

on_exit() {
  local code=$?
  stop_server
  if [ -n "$ABORT_REASON" ]; then
    render_report "❌ **RUN ABORTED** — ${ABORT_REASON}"
    printf '\n%s❌ RUN ABORTED — %s%s  (report: %s)\n' "$c_red" "$ABORT_REASON" "$c_off" "$REPORT"
  fi
  rm -f "$BIN"
  exit $code
}
on_signal() { ABORT_REASON='interrupted by signal'; exit 130; }
trap on_exit EXIT
trap on_signal INT TERM

# ---------------------------------------------------------------------------
# Bench, database, build, boot
# ---------------------------------------------------------------------------
ensure_bench() {
  printf '%s· bench%s ' "$c_dim" "$c_off"
  if ! docker compose -f "$COMPOSE" ps --status running --services 2>/dev/null | grep -qx "$PGSVC"; then
    printf 'starting %s…\n' "$PGSVC"
    docker compose -f "$COMPOSE" up -d "$PGSVC" >/dev/null 2>&1 || { ABORT_REASON="could not start the $PGSVC container"; exit 2; }
  else
    printf 'already up.\n'
  fi
  # Wait for HEALTHY, not merely for the container to exist: createdb against a
  # booting Postgres fails in a way that reads like a suite defect.
  local n=0
  until docker compose -f "$COMPOSE" exec -T "$PGSVC" pg_isready -U omnicore >/dev/null 2>&1; do
    n=$((n+1)); [ "$n" -gt 60 ] && { ABORT_REASON="$PGSVC never became ready"; exit 2; }
    sleep 1
  done
}

reset_database() {
  printf '%s· database%s dropping and recreating %s (nothing touches authcore_db)\n' "$c_dim" "$c_off" "$QA_DB"
  docker compose -f "$COMPOSE" exec -T "$PGSVC" dropdb --if-exists -U omnicore "$QA_DB" >/dev/null 2>&1 \
    || { ABORT_REASON="could not drop $QA_DB"; exit 2; }
  docker compose -f "$COMPOSE" exec -T "$PGSVC" createdb -U omnicore "$QA_DB" >/dev/null 2>&1 \
    || { ABORT_REASON="could not create $QA_DB"; exit 2; }
  # No Mongo and no CDC in this posture, so the reset is that one step and
  # nothing races it: migrations.autoRun rebuilds the schema and the seed.
}

free_port() {
  # Probe FIRST. Something already listening means we may be about to test a
  # binary we did not build — never assume the listener is ours.
  local pids
  pids=$(lsof -ti tcp:"$QA_PORT" -sTCP:LISTEN 2>/dev/null || true)
  [ -z "$pids" ] && return 0
  printf '%s· port%s %s was held by pid(s) %s — sending SIGTERM\n' "$c_yellow" "$c_off" "$QA_PORT" "$(echo $pids | tr '\n' ' ')"
  kill -TERM $pids 2>/dev/null
  local n=0
  while lsof -ti tcp:"$QA_PORT" -sTCP:LISTEN >/dev/null 2>&1; do
    n=$((n+1)); [ "$n" -gt 30 ] && { ABORT_REASON="port $QA_PORT is still held after SIGTERM"; exit 2; }
    sleep 1
  done
}

signing_key() {
  # Reuse the key start.sh already generates for the bench; mint one if absent.
  # It is gitignored either way — a committed signing key is a credential every
  # clone of this repository would hold.
  local key=devops/dev-signing-key.pem
  if [ ! -f "$key" ]; then
    printf '%s· keys%s generating %s\n' "$c_dim" "$c_off" "$key"
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$key" 2>/dev/null \
      || { ABORT_REASON='could not generate a signing key'; exit 2; }
    chmod 600 "$key"
  fi
  export JWT_SIGNING_KEY="$(awk '{printf "%s\\n", $0}' "$key")"
  export JWT_SIGNING_KID="dev-$(openssl dgst -sha256 "$key" | awk '{print substr($NF,1,8)}')"
  export QA_SIGNING_KEY_FILE="$key"
}

start_server() {
  printf '%s· build%s go build -tags '"'"'%s'"'"'\n' "$c_dim" "$c_off" "$BUILD_TAGS"
  go build -tags "$BUILD_TAGS" -o "$BIN" ./bootstrap 2>&1 | tee "${LOG_DIR}/build.log"
  [ -x "$BIN" ] || { ABORT_REASON='the service under test did not build'; exit 2; }

  # AUTH_SELF_URL must match the port the suite serves on: the framework refuses
  # the boot when auth.issuer.selfUrl and auth.jwt.issuer disagree, and the
  # jwksUrl is derived from the same variable.
  export AUTH_SELF_URL="http://localhost:${QA_PORT}"
  export QA_HTTP_ADDR=":${QA_PORT}"
  export APP_PROFILE=dev
  export OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml

  printf '%s· boot%s %s on %s\n' "$c_dim" "$c_off" "$BIN" "$BASE"
  "./$BIN" > "$SERVER_LOG" 2>&1 &
  SERVER_PID=$!

  local n=0 code reason
  while :; do
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      printf '%s\n' "$(tail -30 "$SERVER_LOG")"
      ABORT_REASON='the service exited during boot'; exit 2
    fi
    code=$(curl -s -o "${LOG_DIR}/readyz.json" -w '%{http_code}' "${BASE}/readyz" 2>/dev/null || echo 000)
    [ "$code" = "200" ] && break
    n=$((n+1))
    if [ "$n" -gt 60 ]; then
      # Read the 503 reason rather than reporting a bare timeout.
      reason=$(jq -r '.. | .reason? // empty' "${LOG_DIR}/readyz.json" 2>/dev/null | head -1)
      printf '%s\n' "$(tail -30 "$SERVER_LOG")"
      ABORT_REASON="/readyz never turned 200 (last ${code}${reason:+, reason: $reason})"; exit 2
    fi
    sleep 1
  done
  printf '%s· ready%s /readyz 200\n' "$c_dim" "$c_off"
}

stop_server() {
  [ -n "$SERVER_PID" ] || return 0
  kill -0 "$SERVER_PID" 2>/dev/null || { SERVER_PID=''; return 0; }
  # SIGTERM only, NEVER kill -9 — the drain is part of what is under test, and a
  # killed process leaves the port bound for the next lane.
  kill -TERM "$SERVER_PID" 2>/dev/null
  local n=0
  while kill -0 "$SERVER_PID" 2>/dev/null; do
    n=$((n+1))
    [ "$n" -gt 35 ] && { printf '%s! drain%s exceeded the 30s budget\n' "$c_yellow" "$c_off"; break; }
    sleep 1
  done
  wait "$SERVER_PID" 2>/dev/null
  SERVER_PID=''
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
printf '%s%sauthcore — QA contract suite%s  ·  pin %s  ·  run %s\n' "$c_bold" "$c_dim" "$c_off" "$PIN" "$QA_RUN_ID"
printf '%splan: %s%s\n' "$c_dim" "$PLAN" "$c_off"

for s in "${SUITES[@]}"; do
  [ -f "qa/${s}.sh" ] || { ABORT_REASON="lane '${s}' names qa/${s}.sh, which does not exist"; exit 2; }
done

ensure_bench
reset_database
free_port
signing_key
start_server

RED_TOTAL=0
for i in "${!SUITES[@]}"; do
  s="${SUITES[$i]}"
  printf '\n%s══ suite: %s%s\n' "$c_bold" "$s" "$c_off"
  t0=$(date +%s)
  # tee, so the terminal shows the run live and the log keeps every byte for the report.
  bash "qa/${s}.sh" 2>&1 | tee "${LOG_DIR}/${s}.log"
  rc=${PIPESTATUS[0]}
  t1=$(date +%s)
  R_TIME[$i]="$((t1-t0))s"

  if [ -f "${LOG_DIR}/counts-${s}.txt" ]; then
    read -r p f k < "${LOG_DIR}/counts-${s}.txt"
    R_PASS[$i]=$p; R_FAIL[$i]=$f; R_SKIP[$i]=$k
  fi
  if [ "$rc" -eq 0 ]; then R_VERDICT[$i]='✅ GREEN'; else R_VERDICT[$i]='❌ RED'; RED_TOTAL=$((RED_TOTAL+1)); fi

  render_report '_run in progress_'
  if [ "$rc" -ne 0 ] && [ "$FAIL_FAST" -eq 1 ]; then
    printf '\n%sfail-fast: stopping at the first RED suite. Use --all for the exhaustive sweep.%s\n' "$c_yellow" "$c_off"
    break
  fi
done

stop_server

TOTAL_CASES=0; TOTAL_FAIL=0
for i in "${!SUITES[@]}"; do TOTAL_CASES=$((TOTAL_CASES + R_PASS[i] + R_FAIL[i])); TOTAL_FAIL=$((TOTAL_FAIL + R_FAIL[i])); done
SECS=$(( $(date +%s) - START_TS ))
GREEN_SUITES=0; for i in "${!SUITES[@]}"; do [ "${R_VERDICT[$i]}" = '✅ GREEN' ] && GREEN_SUITES=$((GREEN_SUITES+1)); done

if [ "$RED_TOTAL" -eq 0 ] && [ "$TOTAL_FAIL" -eq 0 ]; then
  FOOTER="✅ ALL GREEN — ${GREEN_SUITES}/${#SUITES[@]} suites · ${TOTAL_CASES} cases · ${SECS}s"
  render_report "$FOOTER"; ABORT_REASON=''
  printf '\n%s%s%s\n%sreport: %s%s\n' "$c_green" "$FOOTER" "$c_off" "$c_dim" "$REPORT" "$c_off"
  exit 0
else
  FOOTER="❌ RED — ${RED_TOTAL} of ${#SUITES[@]} suites — logs: ${LOG_DIR}/"
  render_report "$FOOTER"; ABORT_REASON=''
  printf '\n%s%s%s\n%sreport: %s%s\n' "$c_red" "$FOOTER" "$c_off" "$c_dim" "$REPORT" "$c_off"
  exit 1
fi
