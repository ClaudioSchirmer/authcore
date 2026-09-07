#!/usr/bin/env bash
# THE runner of the authcore QA suite. One command for a dev, one line for CI.
#
#   ./qa/run.sh                  every lane, fail-fast (the first RED stops the run)
#   ./qa/run.sh --all            every lane, exhaustive sweep (RED does not stop the run)
#   ./qa/run.sh tenant domain    a SUBSET, on this same runner — never a rival script
#
# Plans:   specs/qa/tenant-contract/plan.md      (tenant, tenant_graphql, and the R/S3/A blocks)
#          specs/qa/permission-contract/plan.md  (permission, permission_graphql, and the P/S4/A15+ blocks)
#          specs/qa/role-contract/plan.md        (role, role_graphql, and the RL/S5/A24+ blocks)
# Verdict: qa/qa-report.md  (rewritten in full after EVERY lane, so a run killed halfway still
#          leaves what it had proven)
# Logs:    qa/.logs/<run-id>/
#
# WHAT THIS RUNNER OWNS, and why each step is here rather than in a lane:
#   · the throwaway database (plan §2) — one drop+create for the whole run, so exact-count
#     assertions have a clean baseline instead of a hope;
#   · the build tags — `postgres` from relational.dialect, and NO transport tag because the
#     yaml declares no transport: block;
#   · the boot on :8099 under the suite's own config, and the drain-respecting shutdown;
#   · the six principals the lanes borrow: the seeded bootstrap admin (*:*), a tenant:read
#     user, a permission:read user, a permission:read user in a SECOND tenant, and the two
#     ROLE principals of specs/qa/role-contract/plan.md §2 — one holding the whole role bundle
#     inside a tenant of the suite's own, one holding role:read + role:update and NOT
#     role:grant. All but the first are created through the API, and each exists because the
#     others cannot see what it sees — qa/security.sh S4 and S5 say which is which.

set -uo pipefail
cd "$(dirname "$0")/.." || exit 1

# ── the lane list. This array IS the inventory: a .sh under qa/ that no lane names is a suite
# ── nobody runs, and a lane naming a missing file breaks the run for everyone.
LANES=(tenant tenant_graphql permission permission_graphql role role_graphql domain security audit)

PLANS="specs/qa/tenant-contract/plan.md · specs/qa/permission-contract/plan.md · specs/qa/role-contract/plan.md"
REPORT="qa/qa-report.md"
PORT=8099
QA_BASE="http://localhost:$PORT"
PG_CONTAINER="authcore-dev-postgres"
PG_DB="authcore_qa"
SIGNING_KEY_FILE="devops/dev-signing-key.pem"

FAIL_FAST=1
SELECTED=()
for arg in "$@"; do
  case "$arg" in
    --all) FAIL_FAST=0 ;;
    -*) echo "run.sh: unknown flag $arg" >&2; exit 2 ;;
    *) SELECTED+=("$arg") ;;
  esac
done
if [ "${#SELECTED[@]}" -gt 0 ]; then
  for want in "${SELECTED[@]}"; do
    found=0; for l in "${LANES[@]}"; do [ "$l" = "$want" ] && found=1; done
    [ "$found" = "1" ] || { echo "run.sh: '$want' is not a lane. Lanes: ${LANES[*]}" >&2; exit 2; }
  done
  RUN_LANES=("${SELECTED[@]}")
else
  RUN_LANES=("${LANES[@]}")
fi

# ── per-run namespacing. Two runs must never share a temp file, a log, a binary or a port.
QA_RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
QA_LOG_DIR="qa/.logs/$QA_RUN_ID"
QA_RUN_DIR="$QA_LOG_DIR/tmp"
QA_RESULT_DIR="$QA_LOG_DIR/results"
QA_BIN="$QA_RUN_DIR/authcore-$QA_RUN_ID"
SERVER_LOG="$QA_LOG_DIR/server.log"
mkdir -p "$QA_RUN_DIR" "$QA_RESULT_DIR"

export QA_RUN_ID QA_LOG_DIR QA_RUN_DIR QA_RESULT_DIR QA_BASE QA_BIN
export QA_PG_CONTAINER="$PG_CONTAINER" QA_PG_DB="$PG_DB"
export QA_SIGNING_KEY_FILE="$SIGNING_KEY_FILE"

STARTED=$(date +%s)
SERVER_PID=""
ABORT_REASON="interrupted before the final verdict was written"
VERDICT_WRITTEN=0

# ═════════════════════════════════════════════════════════════════════════════════════════
# The report. Rendered LIVE — a run that dies halfway must not leave yesterday's green on disk.
# ═════════════════════════════════════════════════════════════════════════════════════════
render_report() {
  local footer="$1"
  local total_pass=0 total_fail=0 total_skip=0
  # `lane` is declared local here on purpose: this function is called from inside the lane
  # loop, and a global would rewrite the caller's loop variable — which is how the fail-fast
  # line came to name the wrong suite.
  local lane p f s t verdict
  {
    printf '# QA report — authcore · tenant-contract + permission-contract + role-contract\n\n'
    printf -- '- **run:** `%s` · %s\n' "$QA_RUN_ID" "$(date '+%Y-%m-%d %H:%M:%S %Z')"
    printf -- '- **plans:** %s\n' "$PLANS"
    printf -- '- **profile:** `APP_PROFILE=qa` · config `qa/microservice.qa.yaml` · built with `-tags '"'"'postgres'"'"'` (no transport tag — the yaml declares no `transport:` block)\n'
    printf -- '- **omnicore pin:** `%s`\n' "$(go list -m github.com/ClaudioSchirmer/omnicore 2>/dev/null | awk '{print $2}')"
    printf -- '- **hygiene:** throwaway database `%s`, dropped and recreated before this run\n' "$PG_DB"
    printf -- '- **lanes:** %d declared, %d selected\n\n' "${#LANES[@]}" "${#RUN_LANES[@]}"
    printf '| Suite | Pass | Fail | Skip | Verdict | Time |\n'
    printf '|---|---:|---:|---:|---|---:|\n'
    for lane in "${LANES[@]}"; do
      if [ -f "$QA_RESULT_DIR/$lane.result" ]; then
        read -r p f s t < "$QA_RESULT_DIR/$lane.result"
        total_pass=$((total_pass + p)); total_fail=$((total_fail + f)); total_skip=$((total_skip + s))
        if [ "$f" -gt 0 ]; then verdict='❌ RED'; else verdict='✅ GREEN'; fi
        printf '| %s | %d | %d | %d | %s | %ds |\n' "$lane" "$p" "$f" "$s" "$verdict" "$t"
      else
        # A lane that never ran prints a dash. It must NEVER vanish: a suite missing from a
        # report reads exactly like a suite that passed.
        printf '| %s | — | — | — | — did not run | — |\n' "$lane"
      fi
    done
    printf '\n'
    if [ "$total_fail" -gt 0 ]; then
      printf '## Failures\n\n'
      for lane in "${LANES[@]}"; do
        [ -s "$QA_RESULT_DIR/$lane.failures" ] || continue
        read -r _ f _ _ < "$QA_RESULT_DIR/$lane.result" 2>/dev/null || f=0
        [ "${f:-0}" -gt 0 ] || continue
        printf '## lane: %s\n\n' "$lane"
        head -c 20000 "$QA_RESULT_DIR/$lane.failures"
        printf '\nFull log: `%s/`\n\n' "$QA_LOG_DIR"
      done
    fi
    if [ "$total_skip" -gt 0 ]; then
      printf '## Skipped — coverage this run did NOT prove\n\n'
      printf 'A security or domain family that never executed is the one place where "no failures" reads most like "we are safe". These are named here and counted in their own column, never folded into the pass count.\n\n'
      for lane in "${LANES[@]}"; do
        [ -s "$QA_RESULT_DIR/$lane.failures" ] || continue
        grep -q 'SKIPPED' "$QA_RESULT_DIR/$lane.failures" || continue
        printf '### lane: %s\n\n' "$lane"
        # skip_ writes exactly three lines per entry: the header, a blank, the reason. Read
        # them as that shape — an earlier version keyed on the blank line and printed nothing,
        # which left the SKIP column counting families the report never named.
        awk '/— SKIPPED$/{name=$0; sub(/^### /,"",name); sub(/ — SKIPPED$/,"",name);
             getline; getline; reason=$0; sub(/^- \*\*reason:\*\* /,"",reason);
             printf "- **%s**\n  %s\n\n", name, reason}' "$QA_RESULT_DIR/$lane.failures"
        printf '\n'
      done
    fi
    printf -- '---\n\n%s\n' "$footer"
  } > "$REPORT"
  REPORT_PASS=$total_pass; REPORT_FAIL=$total_fail; REPORT_SKIP=$total_skip
}

stop_server() {
  [ -n "$SERVER_PID" ] || return 0
  kill -0 "$SERVER_PID" 2>/dev/null || return 0
  # SIGTERM, never kill -9 — the drain is part of the contract, and the next run binds the
  # same port.
  kill -TERM "$SERVER_PID" 2>/dev/null
  for _ in $(seq 1 70); do kill -0 "$SERVER_PID" 2>/dev/null || break; sleep 0.5; done
  wait "$SERVER_PID" 2>/dev/null
  SERVER_PID=""
}

on_exit() {
  local code=$?
  stop_server
  # The compiled binary is 60MB and `go build` reproduces it byte for byte, so it is not
  # something a run should leave behind — the LOGS are. Without this every run parked another
  # copy under qa/.logs/.
  rm -f "$QA_BIN"
  if [ "$VERDICT_WRITTEN" != "1" ]; then
    render_report "❌ **RUN ABORTED** — $ABORT_REASON · logs: \`$QA_LOG_DIR/\`"
    printf '\n❌ RUN ABORTED — %s — logs: %s/\n' "$ABORT_REASON" "$QA_LOG_DIR"
  fi
  exit $code
}
trap on_exit EXIT
trap 'ABORT_REASON="interrupted by signal"; exit 130' INT TERM

say() { printf '\n\033[1m▸ %s\033[0m\n' "$1"; }

# ═════════════════════════════════════════════════════════════════════════════════════════
# 1. the bench
# ═════════════════════════════════════════════════════════════════════════════════════════
say "bench"
ABORT_REASON="the bench did not come up"
docker compose -f devops/docker-compose.yml up -d --wait >/dev/null 2>&1 || {
  echo "run.sh: docker compose up failed" >&2; exit 1; }
echo "   postgres healthy"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 2. the throwaway database (plan §2). Drop + create IS the whole reset: no Mongo, no CDC, so
#    there is no projection to clear and no relay to drain.
# ═════════════════════════════════════════════════════════════════════════════════════════
say "database"
ABORT_REASON="the throwaway database could not be reset"
docker exec -i "$PG_CONTAINER" psql -U omnicore -d postgres -q \
  -c "DROP DATABASE IF EXISTS $PG_DB WITH (FORCE);" \
  -c "CREATE DATABASE $PG_DB OWNER omnicore;" >/dev/null 2>&1 || {
  echo "run.sh: could not recreate $PG_DB" >&2; exit 1; }
echo "   $PG_DB dropped and recreated"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3. the signing key. Same mechanism start.sh uses: the newlines must be LITERAL backslash-n,
#    because config interpolation runs on the raw file text before the yaml is parsed.
# ═════════════════════════════════════════════════════════════════════════════════════════
if [ ! -f "$SIGNING_KEY_FILE" ]; then
  mkdir -p "$(dirname "$SIGNING_KEY_FILE")"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$SIGNING_KEY_FILE" 2>/dev/null
  chmod 600 "$SIGNING_KEY_FILE"
fi
JWT_SIGNING_KEY="$(awk '{printf "%s\\n", $0}' "$SIGNING_KEY_FILE")"
JWT_SIGNING_KID="dev-$(openssl dgst -sha256 "$SIGNING_KEY_FILE" | awk '{print substr($NF,1,8)}')"
export JWT_SIGNING_KEY JWT_SIGNING_KID
export AUTH_SELF_URL="$QA_BASE" AUTH_AUDIENCE="${AUTH_AUDIENCE:-authcore}"
export QA_DATABASE_URL="postgres://omnicore:omnicore@localhost:5432/$PG_DB?sslmode=disable"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 4. build. The engine tag comes from relational.dialect: postgres. There is no transport tag
#    because the yaml declares no transport: block — the no-op adapter is the correct one.
# ═════════════════════════════════════════════════════════════════════════════════════════
say "build"
ABORT_REASON="the service did not build"
go build -tags 'postgres' -o "$QA_BIN" ./bootstrap 2>&1 | tee "$QA_LOG_DIR/build.log"
[ -x "$QA_BIN" ] || { echo "run.sh: build failed — see $QA_LOG_DIR/build.log" >&2; exit 1; }
echo "   $QA_BIN"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 5. boot. Probe the port FIRST: something already listening there means the lanes would be
#    testing a binary this run did not build.
# ═════════════════════════════════════════════════════════════════════════════════════════
say "boot"
ABORT_REASON="the service did not become ready"
if lsof -ti "tcp:$PORT" >/dev/null 2>&1; then
  echo "   :$PORT is taken — sending SIGTERM to the listener before binding it"
  kill -TERM "$(lsof -ti "tcp:$PORT")" 2>/dev/null
  for _ in $(seq 1 40); do lsof -ti "tcp:$PORT" >/dev/null 2>&1 || break; sleep 0.5; done
fi

APP_PROFILE=qa OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml "$QA_BIN" > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!

READY=0
for _ in $(seq 1 120); do
  body=$(curl -s -w '\n%{http_code}' "$QA_BASE/readyz" 2>/dev/null)
  code=$(printf '%s' "$body" | tail -n1)
  if [ "$code" = "200" ]; then READY=1; break; fi
  kill -0 "$SERVER_PID" 2>/dev/null || break
  sleep 0.5
done
if [ "$READY" != "1" ]; then
  # READ the 503 reason rather than guessing: readyz says WHY it is not ready.
  echo "run.sh: /readyz never answered 200. Last body:" >&2
  curl -s "$QA_BASE/readyz" >&2; echo >&2
  echo "run.sh: server log tail:" >&2; tail -40 "$SERVER_LOG" >&2
  exit 1
fi
echo "   ready on $QA_BASE (pid $SERVER_PID)"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 6. the two principals every lane borrows. VALID tokens come from the service's own login
#    route — this service is its own IdP, so nothing is invented.
# ═════════════════════════════════════════════════════════════════════════════════════════
say "principals"
ABORT_REASON="the suite could not obtain its tokens"

login() { # login EMAIL PASSWORD → accessToken on stdout
  curl -s -X POST "$QA_BASE/auth/user/token" \
    -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg e "$1" --arg p "$2" '{email:$e, password:$p}')" \
  | jq -r '.data.accessToken // empty'
}

# THE BOOTSTRAP PASSWORD HAS TO BE ROTATED BEFORE THE ADMIN CAN DO ANYTHING, and that is a
# deliberate property of this service rather than an obstacle: migration 0012 sets
# must_change_password=TRUE on the seeded admin (the literal password `admin` is the one value
# in that file the API itself would refuse), and a token minted for an account in that state
# carries exactly ONE permission — user:change-password. Every other verb answers 403 until the
# credential is rotated. So the suite rotates it, in its own throwaway database, and signs in
# again for the real token.
ADMIN_PASS='Qa!Bootstrap2026'
BOOTSTRAP_TOKEN=$(login "admin@authcore.local" "admin")
if [ -n "$BOOTSTRAP_TOKEN" ]; then
  ADMIN_ID="01990000-0004-7000-8000-000000000001"
  ROTATE=$(curl -s -X PATCH "$QA_BASE/users/$ADMIN_ID/password" \
    -H "Authorization: Bearer $BOOTSTRAP_TOKEN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg np "$ADMIN_PASS" '{currentPassword:"admin", password:$np, passwordConfirmation:$np}')")
  QA_TOKEN_ADMIN=$(login "admin@authcore.local" "$ADMIN_PASS")
else
  ROTATE=""
  QA_TOKEN_ADMIN=""
fi
[ -n "$QA_TOKEN_ADMIN" ] || { echo "run.sh: the bootstrap admin could not sign in — is migration 0012 applied? rotate said: $ROTATE" >&2; exit 1; }
echo "   principal A: admin@authcore.local (bootstrap password rotated; wildcard *:* through the master role)"

# Principal B: a real, limited principal. Created through the API with principal A's token, so
# the layer-1 negative of §3c is a genuine caller and not a hypothetical.
LIM_EMAIL="qa-readonly-$QA_RUN_ID@authcore.local"
LIM_PASS='Qa!Suite2026'
TENANT_READ_PERMISSION="01990000-0000-7000-8000-00000000001e"

ROLE_JSON=$(curl -s -X POST "$QA_BASE/roles" \
  -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg k "qa-reader-$QA_RUN_ID" --arg p "$TENANT_READ_PERMISSION" \
        '{key:$k, name:"QA Reader", description:"Grants read access to the tenant registry and nothing else, for the QA suite.", permissions:[{permissionID:$p}]}')")
ROLE_ID=$(printf '%s' "$ROLE_JSON" | jq -r '.data.id // empty')
[ -n "$ROLE_ID" ] || { echo "run.sh: could not create the limited role: $ROLE_JSON" >&2; exit 1; }

USER_JSON=$(curl -s -X POST "$QA_BASE/users" \
  -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg e "$LIM_EMAIL" --arg p "$LIM_PASS" --arg r "$ROLE_ID" \
        '{givenName:"Qa", familyName:"Reader", email:$e, status:"active", password:$p, passwordConfirmation:$p, roles:[{roleID:$r}]}')")
USER_ID=$(printf '%s' "$USER_JSON" | jq -r '.data.id // empty')
[ -n "$USER_ID" ] || { echo "run.sh: could not create the limited user: $USER_JSON" >&2; exit 1; }

# Same rotation for principal B, and for the same reason: a user created through the API is
# born must_change_password=TRUE — an admin-set password has to be replaced by its owner — so
# ITS first token also carries user:change-password and nothing else. Without this the limited
# principal would answer 403 to the tenant:read it genuinely holds, and §3c's complement case
# ("a gate that refuses everyone is also broken") would pass for the wrong reason.
LIM_PASS2='Qa!Suite2026b'
BOOT_LIM=$(login "$LIM_EMAIL" "$LIM_PASS")
[ -n "$BOOT_LIM" ] || { echo "run.sh: the limited principal could not sign in" >&2; exit 1; }
curl -s -X PATCH "$QA_BASE/users/$USER_ID/password" \
  -H "Authorization: Bearer $BOOT_LIM" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg cp "$LIM_PASS" --arg np "$LIM_PASS2" '{currentPassword:$cp, password:$np, passwordConfirmation:$np}')" >/dev/null
QA_TOKEN_LIMITED=$(login "$LIM_EMAIL" "$LIM_PASS2")
[ -n "$QA_TOKEN_LIMITED" ] || { echo "run.sh: the limited principal could not sign in after rotating its password" >&2; exit 1; }
echo "   principal B: $LIM_EMAIL (tenant:read only, password rotated)"

# ── principals C and D, for specs/qa/permission-contract/plan.md §3c ──────────────────────
#
# Why two more principals rather than reusing B. A and B answer the SAME on all five
# permission routes — B is refused by all five, A passes all five — so neither can see the
# failure where every route was gated on one literal. C (permission:read only) is the one
# caller for which the five answers differ. D is C in a DIFFERENT tenant, and it exists to
# assert the design decision that Permission is the one aggregate of the seven that is NOT
# tenant-scoped: it must see exactly the catalog C sees.
#
# Both are built with principal A's token, which crosses the tenant scope because it holds
# *:* — InsertRoleRequest and InsertUserRequest each carry an optional tenantID that defaults
# to the caller's claim, so setting it explicitly is what puts D somewhere else.
#
# A failure to build either is NOT fatal to the run: the lane skips its block loudly and the
# report prints it in the SKIP column, which is the honest outcome. A run that aborted here
# would take the six lanes that need nothing from these principals down with it.
PERM_READ_PERMISSION="01990000-0000-7000-8000-000000000015"

# make_principal LABEL TENANT_ID_OR_EMPTY ROLE_NAME ROLE_DESCRIPTION PERMISSION_ID...
#   → accessToken on stdout
#
# ONE provisioner for every scoped principal this suite needs. It speaks only the service's own
# documented flow — a role, a user holding it, and the password rotation that is not optional —
# so nothing is invented and no token is forged.
make_principal() {
  local label="$1" tenant="$2" role_name="$3" role_desc="$4"
  shift 4
  local email="qa-$label-$QA_RUN_ID@authcore.local" p1='Qa!Reader2026' p2='Qa!Reader2026b'
  local perms role_json role_id user_json user_id boot

  perms=$(printf '%s\n' "$@" | jq -R . | jq -sc 'map(select(length > 0) | {permissionID: .})')

  role_json=$(curl -s -X POST "$QA_BASE/roles" \
    -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg k "qa-$label-$QA_RUN_ID" --arg n "$role_name" --arg d "$role_desc" --arg t "$tenant" --argjson p "$perms" \
          '{key:$k, name:$n, description:$d, permissions:$p} + (if $t == "" then {} else {tenantID:$t} end)')")
  role_id=$(printf '%s' "$role_json" | jq -r '.data.id // empty')
  [ -n "$role_id" ] || { echo "run.sh: could not create the $label role: $role_json" >&2; return 1; }

  user_json=$(curl -s -X POST "$QA_BASE/users" \
    -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg e "$email" --arg p "$p1" --arg r "$role_id" --arg t "$tenant" \
          '{givenName:"Qa", familyName:"Principal", email:$e, status:"active",
            password:$p, passwordConfirmation:$p, roles:[{roleID:$r}]} + (if $t == "" then {} else {tenantID:$t} end)')")
  user_id=$(printf '%s' "$user_json" | jq -r '.data.id // empty')
  [ -n "$user_id" ] || { echo "run.sh: could not create the $label user: $user_json" >&2; return 1; }

  # Born must_change_password=TRUE, like every API-created account: its first token carries
  # user:change-password alone, so without this rotation the principal would answer 403 to the
  # permissions it genuinely holds and every case below would pass for the wrong reason.
  boot=$(login "$email" "$p1")
  [ -n "$boot" ] || { echo "run.sh: the $label principal could not sign in" >&2; return 1; }
  curl -s -X PATCH "$QA_BASE/users/$user_id/password" \
    -H "Authorization: Bearer $boot" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg cp "$p1" --arg np "$p2" '{currentPassword:$cp, password:$np, passwordConfirmation:$np}')" >/dev/null
  login "$email" "$p2"
}

# make_reader LABEL TENANT_ID_OR_EMPTY → accessToken on stdout. The catalog-reader flavour of
# make_principal, kept as its own name because S4.4 and S4.5 read better naming what they mean.
make_reader() {
  make_principal "$1" "$2" "QA Catalog Reader" \
    "Grants read access to the platform permission catalog and nothing else, for the QA suite." \
    "$PERM_READ_PERMISSION"
}

QA_TOKEN_PERMREAD=$(make_reader permread "" || true)
if [ -n "$QA_TOKEN_PERMREAD" ]; then
  echo "   principal C: qa-permread-$QA_RUN_ID@authcore.local (permission:read only, master tenant)"
else
  echo "   principal C: NOT BUILT — qa/security.sh S4.4 will skip and say so" >&2
fi

# Principal D needs a tenant of its own before it can live in one.
OTHER_TENANT=$(curl -s -X POST "$QA_BASE/tenants" \
  -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg w "qa-other-$QA_RUN_ID" \
        '{name:"QA Other Tenant", workspace:$w,
          description:"A second isolation partition, so the QA suite can prove the permission catalog is global.",
          status:"active"}')" | jq -r '.data.id // empty')
if [ -n "$OTHER_TENANT" ]; then
  QA_TOKEN_OTHERTENANT=$(make_reader othertenant "$OTHER_TENANT" || true)
else
  QA_TOKEN_OTHERTENANT=""
fi
if [ -n "$QA_TOKEN_OTHERTENANT" ]; then
  echo "   principal D: qa-othertenant-$QA_RUN_ID@authcore.local (permission:read only, tenant $OTHER_TENANT)"
else
  echo "   principal D: NOT BUILT — qa/security.sh S4.5 will skip and say so" >&2
fi

# ── principals E and F, for specs/qa/role-contract/plan.md §2 ─────────────────────────────
#
# Role is the first aggregate this service owns BY TENANT, so §1b's negatives and §3's layers 2
# and 3 need a caller who is authenticated, is NOT a super-admin, and lives in a tenant of the
# suite's own making. Two of them, because one cannot see what the other sees:
#
#   E holds the whole role bundle plus tenant:read, and deliberately NOT permission:archive —
#     which is exactly the permission RL2's no-escalation negative needs it to lack.
#   F holds role:read + role:update and NOT role:grant. It is the ONLY caller for which the
#     2026-08-28 verb split is visible: it may relabel a role and must be refused on both
#     collection routes. A and B answer the same on all seven either way.
#
# Both live in ONE tenant of the suite's own, created by principal A because *:* crosses the
# row scope. A failure to build either is NOT fatal: the lanes skip their blocks loudly and the
# report prints them in the SKIP column, which is the honest outcome.
ROLE_INSERT_PERMISSION="01990000-0000-7000-8000-000000000016"
ROLE_UPDATE_PERMISSION="01990000-0000-7000-8000-000000000017"
ROLE_ARCHIVE_PERMISSION="01990000-0000-7000-8000-000000000018"
ROLE_READ_PERMISSION="01990000-0000-7000-8000-000000000019"
ROLE_GRANT_PERMISSION="01990000-0000-7000-8000-00000000001a"

QA_TENANT_SCOPED=$(curl -s -X POST "$QA_BASE/tenants" \
  -H "Authorization: Bearer $QA_TOKEN_ADMIN" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg w "qa-scoped-$QA_RUN_ID" \
        '{name:"QA Scoped Tenant", workspace:$w,
          description:"The isolation partition the role principals live in, so row scoping can be proven from both sides.",
          status:"active"}')" | jq -r '.data.id // empty')

if [ -n "$QA_TENANT_SCOPED" ]; then
  QA_TOKEN_SCOPED=$(make_principal rolescoped "$QA_TENANT_SCOPED" "QA Role Operator" \
    "Grants the whole role vocabulary inside one tenant, plus tenant:read, and no catalog write verb at all." \
    "$ROLE_INSERT_PERMISSION" "$ROLE_UPDATE_PERMISSION" "$ROLE_ARCHIVE_PERMISSION" \
    "$ROLE_READ_PERMISSION" "$ROLE_GRANT_PERMISSION" "$TENANT_READ_PERMISSION" || true)
  QA_TOKEN_NOGRANT=$(make_principal rolenogrant "$QA_TENANT_SCOPED" "QA Role Labeller" \
    "Grants reading and relabelling a role, and deliberately not the verb that changes what a role can do." \
    "$ROLE_READ_PERMISSION" "$ROLE_UPDATE_PERMISSION" || true)
else
  QA_TOKEN_SCOPED=""
  QA_TOKEN_NOGRANT=""
fi

if [ -n "$QA_TOKEN_SCOPED" ]; then
  echo "   principal E: qa-rolescoped-$QA_RUN_ID@authcore.local (the role bundle + tenant:read, tenant $QA_TENANT_SCOPED)"
else
  echo "   principal E: NOT BUILT — the role rows of qa/domain.sh and qa/security.sh S5 will skip and say so" >&2
fi
if [ -n "$QA_TOKEN_NOGRANT" ]; then
  echo "   principal F: qa-rolenogrant-$QA_RUN_ID@authcore.local (role:read + role:update, NO role:grant)"
else
  echo "   principal F: NOT BUILT — qa/security.sh S5.3 will skip and say so" >&2
fi

export QA_TENANT_SCOPED QA_TOKEN_SCOPED QA_TOKEN_NOGRANT

# The lanes need these to exercise the login route itself (§3b).
QA_ADMIN_EMAIL="admin@authcore.local"
QA_ADMIN_PASSWORD="$ADMIN_PASS"
export QA_TOKEN_ADMIN QA_TOKEN_LIMITED QA_ADMIN_EMAIL QA_ADMIN_PASSWORD
export QA_TOKEN_PERMREAD QA_TOKEN_OTHERTENANT

# ═════════════════════════════════════════════════════════════════════════════════════════
# 7. the lanes
# ═════════════════════════════════════════════════════════════════════════════════════════
ABORT_REASON="a lane was interrupted"
render_report "run in progress…"

RED_LANES=0
for lane in "${RUN_LANES[@]}"; do
  say "lane $lane"
  bash "qa/$lane.sh" 2>&1 | tee "$QA_LOG_DIR/$lane.log"
  status=${PIPESTATUS[0]}
  render_report "run in progress…"
  if [ "$status" -ne 0 ]; then
    RED_LANES=$((RED_LANES + 1))
    if [ "$FAIL_FAST" = "1" ]; then
      echo
      echo "   fail-fast: stopping after '$lane'. Re-run with --all for the exhaustive sweep."
      break
    fi
  fi
done

# ═════════════════════════════════════════════════════════════════════════════════════════
# 8. the verdict
# ═════════════════════════════════════════════════════════════════════════════════════════
ELAPSED=$(( $(date +%s) - STARTED ))
render_report "placeholder"
if [ "$REPORT_FAIL" -gt 0 ]; then
  FOOTER="❌ RED — $RED_LANES of ${#RUN_LANES[@]} suites — logs: $QA_LOG_DIR/"
else
  FOOTER="✅ ALL GREEN — ${#RUN_LANES[@]}/${#RUN_LANES[@]} suites · $REPORT_PASS cases · ${ELAPSED}s"
fi
render_report "$FOOTER"
VERDICT_WRITTEN=1

stop_server

echo
printf '%s\n' "$FOOTER"
printf 'report: %s\n' "$REPORT"
[ "$REPORT_SKIP" -gt 0 ] && printf 'skipped: %d case(s) — named in the report, never folded into the pass count\n' "$REPORT_SKIP"

[ "$REPORT_FAIL" -eq 0 ]
