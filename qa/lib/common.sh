#!/usr/bin/env bash
# Shared helpers for every lane of the authcore QA suite.
#
# THIS FILE IS NOT A LANE. It is sourced, never executed, and it lives under qa/lib/ for
# exactly that reason: the suite's own reconcile rule is "`ls qa/*.sh` minus run.sh must equal
# the runner's lane list", and a helper sitting in that glob would look like a suite nobody
# runs.
#
# Plan: specs/qa/tenant-contract/plan.md
#
# Every request pins Accept-Language: en-US. Envelope assertions have to be deterministic,
# and this service translates its notifications into seven catalogs — a machine's locale must
# never be able to change a verdict.

set -uo pipefail

# ── the environment every lane inherits from run.sh ──────────────────────────────────────
: "${QA_BASE:=http://localhost:8099}"
: "${QA_RUN_DIR:?QA_RUN_DIR is required — lanes are launched by qa/run.sh, not by hand}"
: "${QA_RESULT_DIR:?QA_RESULT_DIR is required}"
QA_GQL="$QA_BASE/graphql"

# ── counters and the current case ────────────────────────────────────────────────────────
QA_LANE=""
QA_PASS=0
QA_FAIL=0
QA_SKIP=0
QA_CASE=""
QA_EXPECT=""
QA_STARTED=0

# HTTP_STATUS / HTTP_BODY are the output of every call below.
HTTP_STATUS=""
HTTP_BODY=""

C_RESET=$'\033[0m'; C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'
if [ ! -t 1 ]; then C_RESET=""; C_GREEN=""; C_RED=""; C_YELLOW=""; C_DIM=""; fi

qa_init() {
  QA_LANE="$1"
  QA_STARTED=$(date +%s)
  mkdir -p "$QA_RESULT_DIR"
  : > "$QA_RESULT_DIR/$QA_LANE.failures"
  echo "── lane: $QA_LANE ──────────────────────────────────────────────────"
}

# qa_finish writes the lane's tally where run.sh reads it and EXITS NON-ZERO on any failure.
# Without that exit code the runner's fail-fast could never trip.
qa_finish() {
  local elapsed=$(( $(date +%s) - QA_STARTED ))
  printf '%s %s %s %s\n' "$QA_PASS" "$QA_FAIL" "$QA_SKIP" "$elapsed" > "$QA_RESULT_DIR/$QA_LANE.result"
  printf '   %s: %s%d passed%s, %s%d failed%s, %s%d skipped%s (%ds)\n' \
    "$QA_LANE" "$C_GREEN" "$QA_PASS" "$C_RESET" \
    "$( [ "$QA_FAIL" -gt 0 ] && echo "$C_RED" || echo "$C_DIM" )" "$QA_FAIL" "$C_RESET" \
    "$C_YELLOW" "$QA_SKIP" "$C_RESET" "$elapsed"
  [ "$QA_FAIL" -eq 0 ]
}

# case_ opens a case. The expectation is printed BEFORE the request runs — it was decided in
# the plan, not read off the answer.
case_() {
  QA_CASE="$1"
  QA_EXPECT="$2"
  printf '  %s· %s%s\n' "$C_DIM" "$QA_CASE" "$C_RESET"
  printf '  %s  expect: %s%s\n' "$C_DIM" "$QA_EXPECT" "$C_RESET"
}

pass_() {
  QA_PASS=$((QA_PASS + 1))
  printf '  %s✔%s %s\n' "$C_GREEN" "$C_RESET" "$QA_CASE"
}

# fail_ prints the REAL response body, never a summary — a verdict nobody can diagnose from
# is a verdict that gets ignored.
fail_() {
  local got="$1"
  QA_FAIL=$((QA_FAIL + 1))
  printf '  %s✘ %s%s\n' "$C_RED" "$QA_CASE" "$C_RESET"
  printf '      expected: %s\n' "$QA_EXPECT"
  printf '      received: %s\n' "$got"
  printf '      body:     %s\n' "$(printf '%s' "$HTTP_BODY" | head -c 2000)"
  {
    printf '### %s\n\n' "$QA_CASE"
    printf -- '- **expected:** %s\n' "$QA_EXPECT"
    printf -- '- **received:** %s\n\n' "$got"
    printf '```json\n%s\n```\n\n' "$(printf '%s' "$HTTP_BODY" | head -c 4000)"
  } >> "$QA_RESULT_DIR/$QA_LANE.failures"
}

skip_() {
  QA_SKIP=$((QA_SKIP + 1))
  printf '  %s~ %s — SKIPPED: %s%s\n' "$C_YELLOW" "$QA_CASE" "$1" "$C_RESET"
  {
    printf '### %s — SKIPPED\n\n' "$QA_CASE"
    printf -- '- **reason:** %s\n\n' "$1"
  } >> "$QA_RESULT_DIR/$QA_LANE.failures"
}

# ── HTTP ─────────────────────────────────────────────────────────────────────────────────
#
# api METHOD PATH [BODY] [TOKEN]
#   TOKEN defaults to $QA_TOKEN_ADMIN. Pass the literal string "-" for a tokenless request.
api() {
  local method="$1" path="$2" body="${3:-}" token="${4:-${QA_TOKEN_ADMIN:-}}"
  local out="$QA_RUN_DIR/resp.$$"
  local args=(-s -o "$out" -w '%{http_code}' -X "$method" "$QA_BASE$path"
              -H 'Accept-Language: en-US')
  if [ "$token" != "-" ] && [ -n "$token" ]; then
    args+=(-H "Authorization: Bearer $token")
  fi
  if [ -n "$body" ]; then
    args+=(-H 'Content-Type: application/json' --data-binary "$body")
  fi
  HTTP_STATUS=$(curl "${args[@]}")
  HTTP_BODY=$(cat "$out" 2>/dev/null)
  rm -f "$out"
}

# api_raw METHOD PATH RAW_AUTH_HEADER_VALUE — for the 401 family, where the header itself is
# the thing under test (wrong scheme, empty value, lowercase scheme).
api_raw() {
  local method="$1" path="$2" authvalue="$3"
  local out="$QA_RUN_DIR/resp.$$"
  local args=(-s -o "$out" -w '%{http_code}' -X "$method" "$QA_BASE$path"
              -H 'Accept-Language: en-US')
  [ -n "$authvalue" ] && args+=(-H "Authorization: $authvalue")
  HTTP_STATUS=$(curl "${args[@]}")
  HTTP_BODY=$(cat "$out" 2>/dev/null)
  rm -f "$out"
}

# api_at BASE METHOD PATH [BODY] [TOKEN] — same, against another base URL (the security
# lane's second boot on :8098).
api_at() {
  local base="$1" method="$2" path="$3" body="${4:-}" token="${5:-}"
  local out="$QA_RUN_DIR/resp.$$"
  local args=(-s -o "$out" -w '%{http_code}' -X "$method" "$base$path"
              -H 'Accept-Language: en-US')
  if [ "$token" != "-" ] && [ -n "$token" ]; then args+=(-H "Authorization: Bearer $token"); fi
  if [ -n "$body" ]; then args+=(-H 'Content-Type: application/json' --data-binary "$body"); fi
  HTTP_STATUS=$(curl "${args[@]}")
  HTTP_BODY=$(cat "$out" 2>/dev/null)
  rm -f "$out"
}

# gql QUERY [VARIABLES_JSON] [TOKEN] — GraphQL always answers HTTP 200; the notification
# rides errors[].extensions.
gql() {
  local query="$1" vars="${2:-}" token="${3:-${QA_TOKEN_ADMIN:-}}"
  [ -z "$vars" ] && vars='{}'
  local payload
  payload=$(jq -nc --arg q "$query" --argjson v "$vars" '{query:$q, variables:$v}')
  local out="$QA_RUN_DIR/resp.$$"
  local args=(-s -o "$out" -w '%{http_code}' -X POST "$QA_GQL"
              -H 'Accept-Language: en-US' -H 'Content-Type: application/json'
              --data-binary "$payload")
  if [ "$token" != "-" ] && [ -n "$token" ]; then args+=(-H "Authorization: Bearer $token"); fi
  HTTP_STATUS=$(curl "${args[@]}")
  HTTP_BODY=$(cat "$out" 2>/dev/null)
  rm -f "$out"
}

# ── assertions ───────────────────────────────────────────────────────────────────────────
assert_status() {
  if [ "$HTTP_STATUS" = "$1" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi
}

# assert_rest EXPECTED_STATUS EXPECTED_NOTIFICATION_KEY — the pair is the contract; a case
# that checks only the status cannot see a key collapse into another one.
assert_rest() {
  local want_status="$1" want_key="$2"
  local keys
  keys=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey] | join(",")' 2>/dev/null)
  if [ "$HTTP_STATUS" = "$want_status" ] && printf '%s' ",$keys," | grep -q ",$want_key,"; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, keys [$keys]"
  fi
}

# assert_rest_field STATUS KEY FIELD — some rejections name the offending token in `field`
# (orderBy[status], onlyTotal[first], fields[bogus]); where the pin promises it, assert it.
assert_rest_field() {
  local want_status="$1" want_key="$2" want_field="$3"
  local hit
  hit=$(printf '%s' "$HTTP_BODY" | jq -r --arg k "$want_key" --arg f "$want_field" \
    '[.errors[]?.messages[]? | select(.notificationKey==$k and (.field // "")==$f)] | length' 2>/dev/null)
  if [ "$HTTP_STATUS" = "$want_status" ] && [ "${hit:-0}" -ge 1 ]; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, no $want_key on field '$want_field'"
  fi
}

# assert_json JQ_FILTER EXPECTED — a value read back off a response the suite already
# ASSERTED the shape of. The expectation is the plan's, never the service's.
assert_json() {
  local filter="$1" want="$2" got
  got=$(printf '%s' "$HTTP_BODY" | jq -r "$filter" 2>/dev/null)
  if [ "$got" = "$want" ]; then pass_; else fail_ "$filter = '$got'"; fi
}

# assert_json_at STATUS JQ EXPECTED — status and value in one verdict.
assert_json_at() {
  local want_status="$1" filter="$2" want="$3" got
  got=$(printf '%s' "$HTTP_BODY" | jq -r "$filter" 2>/dev/null)
  if [ "$HTTP_STATUS" = "$want_status" ] && [ "$got" = "$want" ]; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, $filter = '$got'"
  fi
}

assert_empty_body() {
  if [ "$HTTP_STATUS" = "$1" ] && [ -z "$HTTP_BODY" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, body '$HTTP_BODY'"; fi
}

# assert_gql KEY — the GraphQL rendering of a typed refusal: HTTP 200 with the SAME
# notificationKey the REST envelope reports, carried in errors[].extensions.
assert_gql() {
  local want_key="$1" keys
  keys=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.extensions?.notificationKey] | join(",")' 2>/dev/null)
  if [ "$HTTP_STATUS" = "200" ] && printf '%s' ",$keys," | grep -q ",$want_key,"; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, extension keys [$keys]"
  fi
}

# assert_gql_validation SUBSTRING — the surface's OWN idiom, not the REST envelope: an
# undeclared argument or an enum value outside the schema is cut by gqlparser BEFORE any
# resolver, so it surfaces as a validation message and carries no notificationKey.
assert_gql_validation() {
  local needle="$1" msgs
  msgs=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.message] | join(" | ")' 2>/dev/null)
  if [ "$HTTP_STATUS" = "200" ] && printf '%s' "$msgs" | grep -qi "$needle"; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, messages [$msgs]"
  fi
}

assert_gql_ok() {
  local filter="$1" want="$2" got errs
  errs=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.message] | join(" | ")' 2>/dev/null)
  got=$(printf '%s' "$HTTP_BODY" | jq -r "$filter" 2>/dev/null)
  if [ "$HTTP_STATUS" = "200" ] && [ -z "$errs" ] && [ "$got" = "$want" ]; then
    pass_
  else
    fail_ "HTTP $HTTP_STATUS, errors [$errs], $filter = '$got'"
  fi
}

# ── SQL, for the audit lane ──────────────────────────────────────────────────────────────
# psql is not on the host; the bench container has it. -A -t gives unaligned, header-less
# output, which is what a shell comparison wants.
sql() {
  docker exec -i "${QA_PG_CONTAINER:-authcore-dev-postgres}" \
    psql -U omnicore -d "${QA_PG_DB:-authcore_qa}" -A -t -c "$1" 2>&1
}

# ── fixtures ─────────────────────────────────────────────────────────────────────────────
# Every workspace handle is unique per run AND per case: a handle is never reused in this
# service, archived rows included (§1b row 1), so a fixed one would 409 on the second run
# even against a fresh database if the drop ever failed.
# The counter lives in a FILE, not in a shell variable, and that is not a style choice:
# `ws` is almost always called as $(ws x) — inside a subshell — so an in-memory counter
# increments a copy the parent never sees, and every call in a loop hands back the SAME
# handle. On an entity whose handles are unique forever, that turns a fixture into a 409.
ws() {
  local f="${QA_RUN_DIR:-/tmp}/.ws-seq" n
  n=$(( $(cat "$f" 2>/dev/null || echo 0) + 1 ))
  printf '%d' "$n" > "$f"
  printf 'qa-%s-%s-%d' "${1:-t}" "${QA_RUN_ID:-local}" "$n"
}

# tenant_body NAME WORKSPACE DESCRIPTION STATUS
tenant_body() {
  jq -nc --arg n "$1" --arg w "$2" --arg d "$3" --arg s "$4" \
    '{name:$n, workspace:$w, description:$d, status:$s}'
}

# new_tenant [STATUS] [WORKSPACE] → echoes the created tenant's id. Fixture creation, not a
# case: it asserts nothing and it aborts the lane loudly if the service refuses.
new_tenant() {
  local status="${1:-active}" workspace="${2:-$(ws f)}"
  local name="Fixture ${workspace}"
  local desc="Fixture tenant created by the QA suite for run ${QA_RUN_ID:-local}."
  api POST /tenants "$(tenant_body "$name" "$workspace" "$desc" "$status")"
  if [ "$HTTP_STATUS" != "201" ]; then
    printf '  %sFIXTURE FAILED%s — POST /tenants answered %s: %s\n' "$C_RED" "$C_RESET" "$HTTP_STATUS" "$HTTP_BODY" >&2
    return 1
  fi
  printf '%s' "$HTTP_BODY" | jq -r '.data.id // .id'
}

# ── JWT forging, for the 401 family and the tenant gate ──────────────────────────────────
#
# Signing a deliberately INVALID token needs no secret from anybody and asserts a REFUSAL —
# it is not the "invent a credential" the QA skill forbids. The valid tokens of §3c come from
# the service's own login route, and the suite-signed valid token of the tenant-gate lane is
# signed with a keypair the suite generated and published through its own config file.
b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

# jwt_rs256 PRIVATE_KEY_PEM_PATH HEADER_JSON PAYLOAD_JSON
jwt_rs256() {
  local key="$1" header="$2" payload="$3"
  local h p signing sig
  h=$(printf '%s' "$header" | b64url)
  p=$(printf '%s' "$payload" | b64url)
  signing="$h.$p"
  sig=$(printf '%s' "$signing" | openssl dgst -sha256 -sign "$key" -binary | b64url)
  printf '%s.%s' "$signing" "$sig"
}

# jwt_hs256 SECRET HEADER_JSON PAYLOAD_JSON — the algorithm-confusion probe.
jwt_hs256() {
  local secret="$1" header="$2" payload="$3"
  local h p signing sig
  h=$(printf '%s' "$header" | b64url)
  p=$(printf '%s' "$payload" | b64url)
  signing="$h.$p"
  sig=$(printf '%s' "$signing" | openssl dgst -sha256 -hmac "$secret" -binary | b64url)
  printf '%s.%s' "$signing" "$sig"
}

# jwt_claim TOKEN CLAIM — READ one claim out of a token the suite already holds.
#
# Not a forgery seat: it never signs and never mints. It exists because the row-scope cases
# need the CALLER's half of the comparison — principal B's own tenant — and every other way
# to learn it asks the very endpoint under test, which would make the assertion circular.
# The claim is the same value the service itself scopes by, read from the same token.
jwt_claim() {
  local payload="${1#*.}"
  payload="${payload%%.*}"
  case $(( ${#payload} % 4 )) in
    2) payload="$payload==" ;;
    3) payload="$payload=" ;;
  esac
  printf '%s' "$payload" | tr '_-' '/+' | openssl base64 -d -A 2>/dev/null \
    | jq -r --arg c "$2" '.[$c] // empty'
}

now_epoch() { date +%s; }
