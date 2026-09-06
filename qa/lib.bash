#!/usr/bin/env bash
# Shared helpers for every lane. SOURCED, never executed — and named .bash rather
# than .sh on purpose: `ls qa/*.sh` minus run.sh must equal the runner's lane list
# exactly, so a shared library must not look like a suite nobody runs.
#
# It holds three things a lane must not reimplement: the case bookkeeping (so
# PASS/FAIL/SKIP are counted one way), the request vocabulary, and the assertion
# vocabulary — where every failure prints the REAL response body, because a
# summarized failure is not diagnosable.

set -uo pipefail

: "${QA_PORT:=8099}"
: "${BASE:=http://localhost:${QA_PORT}}"
: "${QA_RUN_ID:=manual-$$}"
: "${LOG_DIR:=qa/.logs/${QA_RUN_ID}}"
: "${LANE_NAME:=lane}"
: "${TOKEN:=}"

PASS=0; FAIL=0; SKIP=0

mkdir -p "$LOG_DIR"
FAILURES_FILE="${LOG_DIR}/failures-${LANE_NAME}.txt"
: > "$FAILURES_FILE"

# Every request pins ONE locale. Envelope assertions must be deterministic, not
# hostage to whatever the machine's LANG happens to be.
ACCEPT_LANG='Accept-Language: en'

c_green=$'\033[32m'; c_red=$'\033[31m'; c_yellow=$'\033[33m'; c_dim=$'\033[2m'; c_off=$'\033[0m'

section() { printf '\n%s── %s%s\n' "$c_dim" "$1" "$c_off"; }
note()    { printf '  %s%s%s\n' "$c_dim" "$1" "$c_off"; }

pass() { PASS=$((PASS+1)); printf '  %s✓ PASS%s  %s\n' "$c_green" "$c_off" "$1"; }

# A failure prints expectation, outcome and the real body. All three, always.
fail() {
  FAIL=$((FAIL+1))
  local name="$1" expected="$2" got="$3" body="${4:-}"
  printf '  %s✗ FAIL%s  %s\n' "$c_red" "$c_off" "$name"
  printf '           expected: %s\n' "$expected"
  printf '           received: %s\n' "$got"
  [ -n "$body" ] && printf '           body: %s\n' "$(printf '%s' "$body" | head -c 900)"
  {
    printf '#### %s\n\n' "$name"
    printf -- '- **expected:** %s\n' "$expected"
    printf -- '- **received:** %s\n\n' "$got"
    printf -- '```json\n%s\n```\n\n' "$(printf '%s' "$body" | head -c 1200)"
  } >> "$FAILURES_FILE"
}

skip() { SKIP=$((SKIP+1)); printf '  %s⊘ SKIP%s  %s %s— %s%s\n' "$c_yellow" "$c_off" "$1" "$c_dim" "$2" "$c_off"; }

# ---------------------------------------------------------------------------
# HTTP. Body and status are captured TOGETHER: fetching the body a second time
# would race with the very state the case is asserting on.
#
# Headers go through an array. An unquoted ${TOKEN:+-H "…"} would be word-split
# into four arguments carrying literal quote characters — the header would be
# malformed and every authenticated case would read as a 401 the service never
# sent. That bug is silent and it makes a security lane pass for the wrong reason.
# ---------------------------------------------------------------------------
RESP_BODY=''; RESP_CODE=''

_curl() {
  local out
  out=$(curl -sS -o - -w $'\n%{http_code}' "$@" 2>&1)
  RESP_CODE="${out##*$'\n'}"
  RESP_BODY="${out%$'\n'*}"
  # curl itself failing (connection refused) leaves no numeric code; surface it
  # as 000 rather than letting an assertion compare against a stderr string.
  case "$RESP_CODE" in ''|*[!0-9]*) RESP_BODY="$out"; RESP_CODE='000';; esac
}

# req METHOD PATH [json-body]
req() {
  local method="$1" path="$2" body="${3:-}"
  local -a args=(-X "$method" "${BASE}${path}" -H "$ACCEPT_LANG")
  [ -n "$TOKEN" ] && args+=(-H "Authorization: Bearer ${TOKEN}")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' --data-binary "$body")
  _curl "${args[@]}"
}

# req_noauth — genuinely tokenless, not an empty Authorization header.
req_noauth() {
  local method="$1" path="$2" body="${3:-}"
  local -a args=(-X "$method" "${BASE}${path}" -H "$ACCEPT_LANG")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' --data-binary "$body")
  _curl "${args[@]}"
}

# req_astoken TOKEN METHOD PATH [body] — one request under a specific token,
# without disturbing the lane's ambient one.
req_astoken() {
  local t="$1"; shift
  local method="$1" path="$2" body="${3:-}"
  local -a args=(-X "$method" "${BASE}${path}" -H "$ACCEPT_LANG" -H "Authorization: Bearer ${t}")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' --data-binary "$body")
  _curl "${args[@]}"
}

# req_rawauth HEADER METHOD PATH — an Authorization header written verbatim, so
# the malformed-header cases send exactly what they mean.
req_rawauth() {
  local hdr="$1" method="$2" path="$3"
  _curl -X "$method" "${BASE}${path}" -H "$ACCEPT_LANG" -H "Authorization: ${hdr}"
}

gql() { req POST /graphql "$(jq -nc --arg q "$1" '{query:$q}')"; }
gql_noauth() { req_noauth POST /graphql "$(jq -nc --arg q "$1" '{query:$q}')"; }
gql_astoken() { local t="$1"; req_astoken "$t" POST /graphql "$(jq -nc --arg q "$2" '{query:$q}')"; }

# ---------------------------------------------------------------------------
# Reading the canonical envelope
# ---------------------------------------------------------------------------
j() { printf '%s' "$RESP_BODY" | jq -r "$1" 2>/dev/null; }

nkey()      { j '.errors[0].messages[0].notificationKey // empty'; }
nfield()    { j '.errors[0].messages[0].field // empty'; }
nsemantic() { j '.errors[0].messages[0].semantic // empty'; }
# Any key in the envelope, not only the first message — a write can report several
# violations at once, and a case asserting ONE rule must not fail because the
# fixture happened to trip another first.
has_key()   { printf '%s' "$RESP_BODY" | jq -e --arg k "$1" '[.errors[]?.messages[]?.notificationKey] | index($k) != null' >/dev/null 2>&1; }
all_keys()  { j '[.errors[]?.messages[]?.notificationKey] | join(",")'; }
# The GraphQL idiom: typed identity rides in extensions and HTTP stays 200.
gqlkey()    { j '.errors[0].extensions.notificationKey // empty'; }

# ---------------------------------------------------------------------------
# Assertions — the case NAME comes first, so a verdict line reads as the case.
# ---------------------------------------------------------------------------
assert_status() {
  local name="$1" want="$2"
  [ "$RESP_CODE" = "$want" ] && { pass "$name — HTTP $want"; return 0; }
  fail "$name" "HTTP $want" "HTTP $RESP_CODE" "$RESP_BODY"; return 1
}

# Status AND key together, because a status alone proves half the promise: the
# pin splits expired from invalid and duplicate from state-conflict precisely so
# a client can branch, and a case accepting any 401 cannot see that split die.
assert_status_key() {
  local name="$1" want="$2" key="$3"
  if [ "$RESP_CODE" = "$want" ] && has_key "$key"; then
    pass "$name — HTTP $want · $key"; return 0
  fi
  fail "$name" "HTTP $want · $key" "HTTP $RESP_CODE · keys=[$(all_keys)]" "$RESP_BODY"; return 1
}

assert_status_key_field() {
  local name="$1" want="$2" key="$3" field="$4" gf; gf="$(nfield)"
  if [ "$RESP_CODE" = "$want" ] && has_key "$key" && [ "$gf" = "$field" ]; then
    pass "$name — HTTP $want · $key · field=$field"; return 0
  fi
  fail "$name" "HTTP $want · $key · field=$field" \
       "HTTP $RESP_CODE · keys=[$(all_keys)] · field=${gf:-<none>}" "$RESP_BODY"; return 1
}

assert_gql_key() {
  local name="$1" key="$2" gk; gk="$(gqlkey)"
  if [ "$RESP_CODE" = "200" ] && [ "$gk" = "$key" ]; then
    pass "$name — GraphQL 200 · $key"; return 0
  fi
  fail "$name" "GraphQL HTTP 200 · $key" "HTTP $RESP_CODE · ${gk:-<no key>}" "$RESP_BODY"; return 1
}

# A GraphQL error that is NOT the canonical envelope — a gqlparser validation
# error, which is the idiom for an unknown argument. Asserting the REST 400
# envelope here would be asserting a promise this surface does not make.
assert_gql_error_matching() {
  local name="$1" pattern="$2" msg; msg="$(j '[.errors[]?.message] | join(" | ")')"
  if [ "$RESP_CODE" = "200" ] || [ "$RESP_CODE" = "400" ]; then
    case "$msg" in *"$pattern"*) pass "$name — GraphQL error matching '$pattern'"; return 0;; esac
  fi
  fail "$name" "a GraphQL error matching '$pattern'" "HTTP $RESP_CODE · ${msg:-<no errors>}" "$RESP_BODY"; return 1
}

assert_gql_ok() {
  local name="$1" errs; errs="$(j '.errors // empty')"
  if [ "$RESP_CODE" = "200" ] && [ -z "$errs" ]; then pass "$name — GraphQL 200, no errors"; return 0; fi
  fail "$name" "GraphQL 200 with no errors" "HTTP $RESP_CODE · errors present" "$RESP_BODY"; return 1
}

assert_jq() {
  local name="$1" expr="$2" want="$3" got; got="$(j "$expr")"
  [ "$got" = "$want" ] && { pass "$name — $expr == $want"; return 0; }
  fail "$name" "$expr == $want" "$expr == ${got:-<empty>}" "$RESP_BODY"; return 1
}

# For compound conditions the envelope cases need (disjoint pages, cursors
# present, a timestamp that parses).
assert_jq_true() {
  local name="$1" expr="$2" desc="$3" got; got="$(j "$expr")"
  [ "$got" = "true" ] && { pass "$name — $desc"; return 0; }
  fail "$name" "$desc" "predicate was ${got:-<error>}" "$RESP_BODY"; return 1
}

assert_absent() {
  local name="$1" expr="$2" got; got="$(j "$expr")"
  { [ -z "$got" ] || [ "$got" = "null" ]; } && { pass "$name — $expr absent"; return 0; }
  fail "$name" "$expr absent from the body" "$expr == $got" "$RESP_BODY"; return 1
}

# ---------------------------------------------------------------------------
# Fixtures — unique per case, so no two cases collide on the unique handle.
# ---------------------------------------------------------------------------
WS_SEQ=0
ws() { WS_SEQ=$((WS_SEQ+1)); printf 'qa-%s-%s-%s' "$LANE_NAME" "${QA_RUN_TAG:-$$}" "$WS_SEQ"; }

# A body satisfying EVERY value object, so a case testing one rule fails for that
# rule alone and never for an unrelated field it did not mean to exercise.
# The status default uses ${4-active}, NOT ${4:-active}: an explicitly empty
# status is a case in its own right (the enum's Unknown sentinel), and :- would
# quietly replace it with "active" — the fixture, not the service, answering.
tenant_body() {
  jq -nc --arg n "${1:-Acme Comercio e Servicos}" --arg w "${2:-}" \
         --arg d "${3:-Retail operations of the Acme group in Brazil.}" \
         --arg s "${4-active}" \
    '{name:$n, workspace:$w, description:$d, status:$s}'
}

# create_tenant WORKSPACE [name] [description] [status] → echoes the new id.
create_tenant() {
  req POST /tenants "$(tenant_body "${2:-Acme Comercio e Servicos}" "$1" \
      "${3:-Retail operations of the Acme group in Brazil.}" "${4-active}")"
  j '.data.id // empty'
}

# ── Permission fixtures ────────────────────────────────────────────────────
# A resource is a SEGMENT: 2–64 lowercase slug runes matching
# ^[a-z0-9]+(-[a-z0-9]+)*$ — and isPermissionSegment also refuses a run of four
# identical runes. A PID like 11115 carries exactly such a run, so the naive
# 'qa-<lane>-<pid>-<n>' would 422 on EVERY insert and the lane would go RED for a
# fixture reason rather than a service one. That is the same class of bug as the
# tenant round's D6, where the fixture answered instead of the service — so the
# tag is collapsed to a maximum run of two before it is ever sent.
_slug() { printf '%s' "$1" | tr 'A-Z' 'a-z' | tr -cd 'a-z0-9-' | sed 's/\(.\)\1\{2,\}/\1\1/g'; }

RES_SEQ=0
res() { RES_SEQ=$((RES_SEQ+1)); printf 'qa-%s-%s-%s' "$(_slug "$LANE_NAME")" "$(_slug "${QA_RUN_TAG:-$$}")" "$RES_SEQ"; }

# A body satisfying every value object, so a case testing one rule fails for that
# rule alone. The default description explains rather than echoes: it must clear
# both the Description VO (≥15 runes, ≥2 words, ≥5 distinct, a vowel) and the
# description-does-not-echo-key rule.
#
# ${3-...} and not ${3:-...}: an explicitly EMPTY description is a case of its
# own (the VO's required branch), and :- would quietly replace it with the
# default — the fixture, not the service, answering.
#
# ${1-tenant}/${2-read} and NOT ${1:-tenant}: an explicitly EMPTY resource or
# action is a case of its own (the framework's required-field branch), and :-
# would quietly substitute the default — sending a perfectly valid tenant:read
# and reporting the 409 that follows as if the empty value had been refused.
permission_body() {
  jq -nc --arg r "${1-tenant}" --arg a "${2-read}" \
         --arg d "${3-Read the registry and open a single entry by its identifier.}" \
    '{resource:$r, action:$a, description:$d}'
}

# create_permission RESOURCE ACTION [description] → echoes the new id.
#
# CALLED IN A COMMAND SUBSTITUTION, so it runs in a SUBSHELL: the id comes back
# on stdout, but RESP_CODE and RESP_BODY do NOT survive. A case that asserts on
# the status of the creation must therefore call req POST itself and read the id
# with j — otherwise it asserts against whatever the PARENT shell last sent,
# which passes or fails for reasons that have nothing to do with the case.
create_permission() {
  req POST /permissions "$(permission_body "$1" "$2" "${3-Read the registry and open a single entry by its identifier.}")"
  j '.data.id // empty'
}

# ── Role fixtures ──────────────────────────────────────────────────────────
# vos.RoleKey is the same shape as a permission SEGMENT — 2-64 runes matching
# ^[a-z0-9]+(-[a-z0-9]+)*$, at least two distinct runes, and no run of four
# identical ones — so the run tag goes through the same _slug that the permission
# fixtures use. A PID like 11115 carries a 4-run and would 422 every insert,
# taking the lane RED for a fixture reason rather than a service one.
RK_SEQ=0
rk() { RK_SEQ=$((RK_SEQ+1)); printf 'qa-%s-%s-%s' "$(_slug "$LANE_NAME")" "$(_slug "${QA_RUN_TAG:-$$}")" "$RK_SEQ"; }

# role_body KEY [name] [description] [tenantID|OMIT] [permission-ids as a JSON array]
#
# tenantID defaults to the sentinel OMIT, which leaves the key OUT of the body
# entirely — that is the ordinary shape, because the field is assignedFrom the
# identity claim and absent means "mine". Passing "" is a CASE of its own (the
# guard barrier), so the sentinel is what keeps "omitted" and "explicitly empty"
# apart. ${4-OMIT} and never ${4:-OMIT}, the same distinction the tenant round's
# D6 and the permission round's D1 both earned.
role_body() {
  jq -nc --arg k "${1-billing-manager}" \
         --arg n "${2-Billing Manager}" \
         --arg d "${3-Grants read access to the tenant registry and the permission catalog, without any write verb.}" \
         --arg t "${4-OMIT}" \
         --argjson p "${5-[]}" \
    '{key:$k, name:$n, description:$d, permissions: ($p | map({permissionID: .}))}
     + (if $t == "OMIT" then {} else {tenantID:$t} end)'
}

# permission_id_of RESOURCE ACTION → echoes the catalog id of that pair.
#
# The suite never hardcodes a catalog id twice: migration 0012 fixes them as
# literals, and a case that needs one ADDRESSES it by the pair the same migration
# declares. An id is an address, never an expectation — no assertion is derived
# from what this returns.
permission_id_of() {
  req GET "/permissions?resource.eq=$1&action.eq=$2&first=1"
  j '.data[0].id // empty'
}

# create_role KEY [tenantID|OMIT] [permission-ids as a JSON array] → echoes the id.
#
# SAME SUBSHELL TRAP as create_permission: called in a command substitution this
# runs in a subshell, so RESP_CODE and RESP_BODY do NOT survive. A case whose
# assertion IS the status must call req POST itself.
create_role() {
  req POST /roles "$(role_body "$1" "Qa Role Fixture" \
      'A role the suite created so one rule can be exercised and nothing else.' \
      "${2-OMIT}" "${3-[]}")"
  j '.data.id // empty'
}

# ---------------------------------------------------------------------------
# provision_scoped_principal LABEL PERMISSION-LITERAL...
#
# The fixture §1b and §3 of specs/qa/role-contract/plan.md are built on: a caller
# who is authenticated, is NOT a super-admin, is bound to a tenant of the suite's
# own making, and holds exactly the bundle a case needs.
#
# Four calls through the service's OWN documented flow. Nothing is invented, no
# token is forged, no credential of the maintainer's is used:
#   1. POST /tenants          — the principal needs somewhere to be
#   2. POST /roles            — the bundle, granted by the *:* admin, who bypasses
#                               the no-escalation rule by construction
#   3. POST /users            — the account holding that role
#   4. sign in, ROTATE, sign in again
#
# STEP 4 IS NOT OPTIONAL. user_rules_manual.go:125 sets MustChangePassword = true
# on every API-created user, so the FIRST token is restricted to
# `user:change-password` alone. A suite that skipped the rotation would be testing
# the restricted session and reading its 403s as the role gate — a lane that goes
# green for a reason that has nothing to do with what it claims to prove.
#
# NOT command-substituted: it SETS globals, because a subshell would lose them.
# Answers 0 on success and leaves SP_TOKEN empty on failure, so a caller can skip
# loudly instead of asserting against nothing.
# ---------------------------------------------------------------------------
SP_TOKEN=''; SP_TENANT_ID=''; SP_ROLE_ID=''; SP_USER_ID=''; SP_EMAIL=''; SP_FAILED=''
SP_PASSWORD='Zx7#Kq2m!Wt9v'

provision_scoped_principal() {
  local label="$1"; shift
  local admin="$TOKEN" tag="${QA_RUN_TAG:-$$}"
  SP_TOKEN=''; SP_TENANT_ID=''; SP_ROLE_ID=''; SP_USER_ID=''; SP_EMAIL=''; SP_FAILED=''

  _sp_abort() { SP_FAILED="$1 (HTTP $RESP_CODE) $(printf '%s' "$RESP_BODY" | head -c 400)"; TOKEN="$admin"; return 1; }

  TOKEN="$admin"

  # 1. the tenant
  req POST /tenants "$(tenant_body "Qa Scope $(_slug "$label")" "$(_slug "qa-${label}-${tag}")" \
      'A tenant the suite owns, so a scoped principal has a partition of its own to be isolated in.' 'active')"
  [ "$RESP_CODE" = "201" ] || { _sp_abort "could not create the scoped tenant"; return 1; }
  SP_TENANT_ID="$(j '.data.id')"

  # 2. the bundle — every literal resolved to its catalog id by its own pair
  local ids='[]' lit res act pid
  for lit in "$@"; do
    res="${lit%%:*}"; act="${lit##*:}"
    pid="$(permission_id_of "$res" "$act")"
    [ -n "$pid" ] || { SP_FAILED="no catalog row for ${lit}"; TOKEN="$admin"; return 1; }
    ids="$(printf '%s' "$ids" | jq -c --arg p "$pid" '. + [$p]')"
  done

  req POST /roles "$(role_body "$(_slug "qa-${label}-r-${tag}")" "Qa Scoped $(_slug "$label")" \
      'The bundle a scoped principal holds while the suite proves what it may and may not do.' \
      "$SP_TENANT_ID" "$ids")"
  [ "$RESP_CODE" = "201" ] || { _sp_abort "could not create the scoped role"; return 1; }
  SP_ROLE_ID="$(j '.data.id')"

  # 3. the account. The password must echo neither the name nor the e-mail —
  # PasswordEchoesIdentityNotification is a User rule this fixture is not here to
  # test, so it simply satisfies it.
  SP_EMAIL="qa-$(_slug "$label")-${tag}@authcore.local"
  req POST /users "$(jq -nc --arg gn 'Qa' --arg fn 'Scoped' --arg e "$SP_EMAIL" \
      --arg p "$SP_PASSWORD" --arg t "$SP_TENANT_ID" --arg r "$SP_ROLE_ID" \
      '{givenName:$gn, familyName:$fn, email:$e, status:"active", password:$p,
        passwordConfirmation:$p, tenantID:$t, groups:[], roles:[{roleID:$r}], claims:[]}')"
  [ "$RESP_CODE" = "201" ] || { _sp_abort "could not create the scoped user"; return 1; }
  SP_USER_ID="$(j '.data.id')"

  # 4. sign in, rotate, sign in again
  sign_in "$SP_EMAIL" "$SP_PASSWORD"
  [ "$RESP_CODE" = "200" ] || { _sp_abort "the scoped principal could not sign in"; return 1; }
  local first_token; first_token="$(j '.data.accessToken')"
  if [ "$(j '.data.user.mustChangePassword')" = "true" ]; then
    local rotated="${SP_PASSWORD}-2"
    req_astoken "$first_token" PATCH "/users/${SP_USER_ID}/password" \
      "$(jq -nc --arg c "$SP_PASSWORD" --arg p "$rotated" \
         '{currentPassword:$c, password:$p, passwordConfirmation:$p}')"
    case "$RESP_CODE" in 200|204) SP_PASSWORD="$rotated" ;;
      *) _sp_abort "the scoped principal's password rotation failed"; return 1 ;;
    esac
    sign_in "$SP_EMAIL" "$SP_PASSWORD"
    [ "$RESP_CODE" = "200" ] || { _sp_abort "the scoped principal could not sign in after rotating"; return 1; }
  fi
  SP_TOKEN="$(j '.data.accessToken')"
  TOKEN="$admin"
  [ -n "$SP_TOKEN" ] && [ "$SP_TOKEN" != "null" ] || { SP_FAILED="the scoped principal received no access token"; return 1; }
  return 0
}

# ---------------------------------------------------------------------------
# Sign-in. Every lane needs a token, and the ONE place it comes from is this
# service's own documented flow — nothing is invented and nothing is hardcoded
# that the repository does not already hold (migrations/postgres/0012, README).
# ---------------------------------------------------------------------------
BOOTSTRAP_EMAIL='admin@authcore.local'
BOOTSTRAP_INITIAL_PASSWORD='admin'
QA_ADMIN_PASSWORD='Qa-Suite-Passw0rd!'

sign_in() {
  req_noauth POST /auth/user/token "$(jq -nc --arg e "$1" --arg p "$2" '{email:$e,password:$p}')"
}

lane_summary() {
  printf '\n%s%s: %s%d passed%s · %s%d failed%s · %s%d skipped%s\n' \
    "$c_dim" "$LANE_NAME" "$c_green" "$PASS" "$c_off" "$c_red" "$FAIL" "$c_off" "$c_yellow" "$SKIP" "$c_off"
  printf '%d %d %d\n' "$PASS" "$FAIL" "$SKIP" > "${LOG_DIR}/counts-${LANE_NAME}.txt"
  # Non-zero on any failure — without this the runner's fail-fast can never trip.
  [ "$FAIL" -eq 0 ]
}
