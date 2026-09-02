#!/usr/bin/env bash
#
# Contract QA lane — Tenant. Plan: specs/qa/tenant/plan.md (APPROVED).
#
# WHAT THIS PROVES: what the wire promises for the Tenant aggregate at omnicore
# v0.69.0 — the six verbs it serves, the status code and notification KEY of each
# refusal, the archive round-trip, the whole read vocabulary, the typed-400 guard
# family, and REST/GraphQL handler invariance. It asserts BEHAVIOUR, never internals:
# refactoring behind the same contract must not turn a case red.
#
# READ-YOUR-WRITES. The Tenant read model is a RelationalView served straight from the
# tables — no projection, no CDC. Every read-back below is therefore IMMEDIATE, and a
# case that would only pass after a retry is itself a failure of the contract.
#
# SELF-CONTAINED. This lane provisions its own throwaway database, builds and boots its
# own server on its own port, and drains it with SIGTERM before returning. Run it
# through ./qa/run.sh; running it directly works and does the same thing.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# ── lane constants ───────────────────────────────────────────────────────────
# The port and the database are this LANE's, not the suite's: a second lane must pick
# its own so two lanes never corrupt each other. AUTH_SELF_URL travels with the port —
# this service issues and validates its own tokens and refuses to boot if they disagree.
LANE_NAME="tenant"
PORT="${QA_TENANT_PORT:-8099}"
QA_DB="authcore_qa_db"
COMPOSE="devops/docker-compose.yml"
BASE="http://127.0.0.1:${PORT}"

RUN="$(date +%s)"                      # run id — reaches every workspace handle
TMP="$(mktemp -d "${TMPDIR:-/tmp}/authcore-qa-${LANE_NAME}-$$-XXXXXX")"
BIN="${TMP}/authcore-qa"
LOG="${TMP}/server.log"
BODY="${TMP}/body.json"
SERVER_PID=""

PASS=0; FAIL=0; SKIP=0
STATUS=""
TOKEN=""

# When run through qa/run.sh the runner hands a log directory; the lane writes its
# machine-readable counts and failure blocks there so the report never parses prose.
# Unset when the lane is invoked directly, which still works.
QA_LOG_DIR="${QA_LOG_DIR:-}"
FAILFILE=""
[[ -n "$QA_LOG_DIR" ]] && { mkdir -p "$QA_LOG_DIR"; FAILFILE="${QA_LOG_DIR}/${LANE_NAME}.failures"; : > "$FAILFILE"; }

# Fixture credentials. The bootstrap password is a tracked literal in
# migrations/postgres/0012_bootstrap_seed_manual.up.sql — it is not invented here, and
# it is public knowledge by that file's own admission. The rotation target satisfies the
# Password value object (>=8 runes, lower + upper + digit + symbol) and echoes neither
# the admin's e-mail nor either of its names.
BOOTSTRAP_EMAIL="admin@authcore.local"
BOOTSTRAP_PASSWORD="admin"
BOOTSTRAP_USER_ID="01990000-0004-7000-8000-000000000001"
QA_PASSWORD='Qa!Contract2026'

# ── output ───────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then G=$'\033[0;32m'; R=$'\033[0;31m'; Y=$'\033[0;33m'; D=$'\033[2m'; Z=$'\033[0m'
else G=""; R=""; Y=""; D=""; Z=""; fi

section() { printf '\n%s── %s%s\n' "$D" "$1" "$Z"; }
pass()    { PASS=$((PASS+1)); printf '  %s✓ GREEN%s  %s\n' "$G" "$Z" "$1"; }
skip()    { SKIP=$((SKIP+1)); printf '  %s• SKIPPED%s %s — %s\n' "$Y" "$Z" "$1" "$2"; }
fail()    {
  FAIL=$((FAIL+1))
  printf '  %s✗ RED%s    %s\n' "$R" "$Z" "$1"
  printf '           expected: %s\n' "$2"
  printf '           actual:   %s\n' "$3"
  printf '           body:     %s\n' "$(head -c 1500 "$BODY" 2>/dev/null)"
  if [[ -n "$FAILFILE" ]]; then
    {
      printf -- '- **%s**\n' "$1"
      printf -- '  - expected: `%s`\n' "$2"
      printf -- '  - received: `%s`\n' "$3"
      printf -- '  - body:\n\n    ```json\n'
      head -c 1200 "$BODY" 2>/dev/null | sed 's/^/    /'
      printf -- '\n    ```\n'
    } >> "$FAILFILE"
  fi
}
die() { printf '\n%sqa/%s.sh: %s%s\n' "$R" "$LANE_NAME" "$1" "$Z" >&2; exit 2; }

cleanup() {
  # The server log outlives TMP when the runner gave us somewhere to keep it.
  [[ -n "$QA_LOG_DIR" && -f "$LOG" ]] && cp "$LOG" "${QA_LOG_DIR}/${LANE_NAME}-server.log" 2>/dev/null
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    # SIGTERM, never -9: the drain is part of the contract (shutdown.drainTimeoutSeconds
    # is 30s) and the next lane cannot bind this port until it completes.
    kill -TERM "$SERVER_PID" 2>/dev/null
    local waited=0
    while kill -0 "$SERVER_PID" 2>/dev/null && [[ $waited -lt 35 ]]; do sleep 1; waited=$((waited+1)); done
    kill -0 "$SERVER_PID" 2>/dev/null && printf '  %s! server %s did not drain in 35s%s\n' "$Y" "$SERVER_PID" "$Z"
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

# ── request helpers ──────────────────────────────────────────────────────────
# Accept-Language is pinned on EVERY request: envelope assertions must be deterministic,
# not hostage to the machine's locale. "en" resolves to LangENG in the framework.
api_as() { # token method path [json-body]
  local tok=$1 method=$2 path=$3 body=${4-}
  local args=(-s -o "$BODY" -w '%{http_code}' -X "$method" -H 'Accept-Language: en')
  [[ -n "$tok"  ]] && args+=(-H "Authorization: Bearer ${tok}")
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' --data-binary "$body")
  STATUS=$(curl "${args[@]}" "${BASE}${path}" 2>/dev/null)
}
api() { api_as "$TOKEN" "$@"; }
urlenc() { jq -rn --arg v "$1" '$v|@uri'; }

gql() { # graphql-document
  api POST /graphql "$(jq -n --arg q "$1" '{query:$q}')"
}

# ── assertion helpers ────────────────────────────────────────────────────────
# expect <case> <status> [notificationKey] [field]
expect() {
  local name=$1 want=$2 key=${3-} field=${4-}
  if [[ "$STATUS" != "$want" ]]; then
    fail "$name" "HTTP $want${key:+ carrying $key}" "HTTP $STATUS"; return 1
  fi
  if [[ -n "$key" ]]; then
    if ! jq -e --arg k "$key" 'any(.errors[]?.messages[]?; .notificationKey == $k)' "$BODY" >/dev/null 2>&1; then
      fail "$name" "HTTP $want carrying $key" \
           "HTTP $STATUS with keys $(jq -c '[.errors[]?.messages[]?.notificationKey]' "$BODY" 2>/dev/null)"
      return 1
    fi
  fi
  if [[ -n "$field" ]]; then
    if ! jq -e --arg f "$field" 'any(.errors[]?.messages[]?; .field == $f)' "$BODY" >/dev/null 2>&1; then
      fail "$name" "HTTP $want carrying $key on field '$field'" \
           "fields $(jq -c '[.errors[]?.messages[]?.field]' "$BODY" 2>/dev/null)"
      return 1
    fi
  fi
  pass "$name — HTTP $want${key:+ · $key}${field:+ · field=$field}"
}

# json <case> <jq-filter> <expected>
json() {
  local name=$1 filter=$2 want=$3 got
  got=$(jq -r "$filter" "$BODY" 2>/dev/null)
  if [[ "$got" == "$want" ]]; then pass "$name — $filter == $want"
  else fail "$name" "$filter == $want" "$filter == ${got:-<unreadable>}"; fi
}

# jqtrue <case> <jq-filter> <human description>
jqtrue() {
  if jq -e "$2" "$BODY" >/dev/null 2>&1; then pass "$1 — $3"
  else fail "$1" "$3" "jq filter '$2' was false"; fi
}

# ── fixtures ─────────────────────────────────────────────────────────────────
tenant_body() { # workspace name description status
  jq -n --arg w "$1" --arg n "$2" --arg d "$3" --arg s "$4" \
    '{name:$n, workspace:$w, description:$d, status:$s}'
}
# Descriptions satisfy the Description value object (>=15 runes, >=2 words, >=5 distinct
# runes, >=1 vowel, no run of 4 identical) and differ from name and workspace, which the
# manual rule description-differs-from-name-and-workspace requires.
create_tenant() { # workspace name description status -> echoes the new id
  api POST /tenants "$(tenant_body "$1" "$2" "$3" "$4")"
  [[ "$STATUS" == "201" ]] || die "fixture insert $1 answered HTTP $STATUS: $(cat "$BODY")"
  jq -r '.data.id' "$BODY"
}

# ═════════════════════════════════════════════════════════════════════════════
# PREFLIGHT
# ═════════════════════════════════════════════════════════════════════════════
section "preflight"
for tool in curl jq docker go; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required and was not found on PATH"
done
printf '  run id %s · port %s · database %s\n' "$RUN" "$PORT" "$QA_DB"

# The bench. Reads come straight from these tables, so nothing works without it.
if ! docker compose -f "$COMPOSE" ps --status running --services 2>/dev/null | grep -q '^postgres$'; then
  printf '  bringing the bench up…\n'
  docker compose -f "$COMPOSE" up -d --wait >/dev/null 2>&1 || die "docker compose up failed"
fi

# The throwaway database: dropped and recreated per run. migrations.autoRun (on under
# APP_PROFILE=dev) then rebuilds the schema AND the bootstrap seed into it. Nothing this
# lane writes ever reaches authcore_db.
printf '  provisioning %s…\n' "$QA_DB"
docker compose -f "$COMPOSE" exec -T postgres dropdb -U omnicore --if-exists "$QA_DB" >/dev/null 2>&1
docker compose -f "$COMPOSE" exec -T postgres createdb -U omnicore "$QA_DB" >/dev/null 2>&1 \
  || die "could not create $QA_DB inside the bench container"

# The dev signing key, generated the same way start.sh generates it. The literal \n form
# is required: interpolation runs on the raw yaml text before parsing.
KEY_FILE="devops/dev-signing-key.pem"
if [[ ! -f "$KEY_FILE" ]]; then
  mkdir -p "$(dirname "$KEY_FILE")"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$KEY_FILE" 2>/dev/null \
    || die "could not generate a signing key at $KEY_FILE"
  chmod 600 "$KEY_FILE"
fi
export JWT_SIGNING_KEY="$(awk '{printf "%s\\n", $0}' "$KEY_FILE")"
export JWT_SIGNING_KID="qa-$(openssl dgst -sha256 "$KEY_FILE" | awk '{print substr($NF,1,8)}')"

# Build tags come from the yaml, not from habit: relational.dialect is postgres and there
# is no transport: block, so one engine tag and no transport tag.
printf '  building (-tags postgres)…\n'
go build -tags 'postgres' -o "$BIN" ./bootstrap >"${TMP}/build.log" 2>&1 \
  || { cat "${TMP}/build.log"; die "build failed"; }

# Probe the port BEFORE booting: something already listening means the cases would be
# run against a binary this lane did not build.
if lsof -ti "tcp:${PORT}" >/dev/null 2>&1; then
  printf '  %s! port %s is occupied — sending SIGTERM to the listener%s\n' "$Y" "$PORT" "$Z"
  lsof -ti "tcp:${PORT}" | xargs -r kill -TERM 2>/dev/null
  for _ in 1 2 3 4 5 6 7 8 9 10; do lsof -ti "tcp:${PORT}" >/dev/null 2>&1 || break; sleep 1; done
  lsof -ti "tcp:${PORT}" >/dev/null 2>&1 && die "port $PORT is still occupied"
fi

printf '  booting…\n'
APP_PROFILE=dev \
OMNICORE_CONFIG_PATH="qa/microservice.qa.yaml" \
QA_HTTP_ADDR=":${PORT}" \
AUTH_SELF_URL="http://localhost:${PORT}" \
  "$BIN" >"$LOG" 2>&1 &
SERVER_PID=$!

ready=0
for _ in $(seq 1 60); do
  kill -0 "$SERVER_PID" 2>/dev/null || { cat "$LOG"; die "the server exited during boot"; }
  code=$(curl -s -o "${TMP}/readyz.json" -w '%{http_code}' "${BASE}/readyz" 2>/dev/null)
  if [[ "$code" == "200" ]]; then ready=1; break; fi
  sleep 1
done
if [[ $ready -eq 0 ]]; then
  printf '  last /readyz answer: %s\n' "$(cat "${TMP}/readyz.json" 2>/dev/null)"
  tail -40 "$LOG"
  die "the service never became ready"
fi
printf '  %sready%s (pid %s)\n' "$G" "$Z" "$SERVER_PID"

# ═════════════════════════════════════════════════════════════════════════════
# A — sign-in and the permission gate
# ═════════════════════════════════════════════════════════════════════════════
section "A · sign-in and the permission gate"

# A4/A5 first: they need no token and prove the bench really is closed.
api_as "" GET /tenants
expect "A4 tokenless listing is refused" 401 "MissingAuthorizationNotification"

api_as "not-a-real-token" GET /tenants
expect "A5 bogus bearer is refused" 401 "InvalidTokenNotification"

api_as "" POST /auth/user/token \
  "$(jq -n --arg e "$BOOTSTRAP_EMAIL" --arg p "$BOOTSTRAP_PASSWORD" '{email:$e,password:$p}')"
expect "A1 bootstrap sign-in" 200
json   "A1 must-change flag is set" '.data.user.mustChangePassword' 'true'
json   "A1 the restricted token carries one permission only" \
       '.data.user.permissions | sort | join(",")' 'user:change-password'
RESTRICTED_TOKEN=$(jq -r '.data.accessToken' "$BODY" 2>/dev/null)

api_as "$RESTRICTED_TOKEN" GET /tenants
expect "A2 the permission gate refuses the restricted token" 403 "MissingPermissionNotification" "permission"

api_as "$RESTRICTED_TOKEN" PATCH "/users/${BOOTSTRAP_USER_ID}/password" \
  "$(jq -n --arg c "$BOOTSTRAP_PASSWORD" --arg n "$QA_PASSWORD" \
        '{currentPassword:$c, password:$n, passwordConfirmation:$n}')"
expect "A3a password rotation" 204

api_as "" POST /auth/user/token \
  "$(jq -n --arg e "$BOOTSTRAP_EMAIL" --arg p "$QA_PASSWORD" '{email:$e,password:$p}')"
expect "A3b re-sign-in after the rotation" 200
json   "A3b the flag is cleared" '.data.user.mustChangePassword' 'false'
jqtrue "A3b the full token carries the wildcard" \
       '.data.user.permissions | index("*:*") != null' 'permissions contains *:*'
TOKEN=$(jq -r '.data.accessToken' "$BODY" 2>/dev/null)
[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || die "no usable access token — every later case would be meaningless"

# ═════════════════════════════════════════════════════════════════════════════
# X — the route inventory, read from the service itself
# ═════════════════════════════════════════════════════════════════════════════
section "X · route inventory (openapi.json is the oracle)"
api GET /openapi.json
expect "X1a openapi document is served" 200
json "X1b the tenant routes openapi enumerates" \
  '[.paths | to_entries[] | select(.key|startswith("/tenants")) | . as $e | ($e.value|keys[]) | (ascii_upcase + " " + $e.key)] | sort | join(" ")' \
  'GET /tenants/ GET /tenants/{id} PATCH /tenants/{id} PATCH /tenants/{id}/archive PATCH /tenants/{id}/unarchive POST /tenants/'
# The two collection verbs are advertised with a trailing slash — they are mounted on the
# Fiber group under path "/". Fiber serves both spellings, which is why every case above
# calls /tenants; a generated client would use /tenants/. Same six routes either way.

# ═════════════════════════════════════════════════════════════════════════════
# seed — the fixtures the read-vocabulary cases count on
# ═════════════════════════════════════════════════════════════════════════════
section "seed"
P_PREFIX="qa-${RUN}-p"
P1=$(create_tenant "${P_PREFIX}1" "QA Page Alpha"   "Contract suite paging fixture number one."   "trial")
P2=$(create_tenant "${P_PREFIX}2" "QA Page Bravo"   "Contract suite paging fixture number two."   "active")
P3=$(create_tenant "${P_PREFIX}3" "QA Page Charlie" "Contract suite paging fixture number three." "suspended")
P4=$(create_tenant "${P_PREFIX}4" "QA Page Delta"   "Contract suite paging fixture number four."  "active")
P5=$(create_tenant "${P_PREFIX}5" "QA Page Echo"    "Contract suite paging fixture number five."  "trial")
printf '  5 paging fixtures under %s*\n' "$P_PREFIX"

# ═════════════════════════════════════════════════════════════════════════════
# B — the happy path, one per served verb
# ═════════════════════════════════════════════════════════════════════════════
section "B · happy path per served verb"
B_WS="qa-${RUN}-b1"
api POST /tenants "$(tenant_body "$B_WS" "QA Happy Path" "The record the six verbs are proven on." "trial")"
expect "B1 insert" 201
json   "B1 the response mirrors the stored entity" '.data.workspace' "$B_WS"
B1=$(jq -r '.data.id' "$BODY")

api GET "/tenants/${B1}"
expect "B2 read back immediately (relational: no poll)" 200
json   "B2 name round-trips"   '.data.name'   'QA Happy Path'
json   "B2 status round-trips" '.data.status' 'trial'

api GET "/tenants?workspace=$(urlenc "$B_WS")"
expect "B3 listing filtered to it" 200
json   "B3 exactly one row"   '.data | length' '1'
json   "B3 and it is the one" '.data[0].id'    "$B1"

api PATCH "/tenants/${B1}" '{"name":"QA Happy Path Renamed","description":"The description this patch leaves behind."}'
expect "B4 patch" 200
json   "B4 the new name is stored"      '.data.name'      'QA Happy Path Renamed'
json   "B4 the handle is untouched"     '.data.workspace' "$B_WS"

api PATCH "/tenants/${B1}/archive"
expect "B5 archive is bodyless" 204
api PATCH "/tenants/${B1}/unarchive"
expect "B6 unarchive is bodyless" 204

# ═════════════════════════════════════════════════════════════════════════════
# C — golden record: every declared field, on every enabled surface
# ═════════════════════════════════════════════════════════════════════════════
section "C · golden-record round-trip"
G_WS="qa-${RUN}-g1"
G_NAME="QA Golden Record"
G_DESC="Every declared field of the tenant aggregate travels through this one record."
api POST /tenants "$(tenant_body "$G_WS" "$G_NAME" "$G_DESC" "active")"
expect "C1 golden insert" 201
G1=$(jq -r '.data.id' "$BODY")

api GET "/tenants/${G1}"
expect "C2 golden by-id" 200
json "C2 id"          '.data.id'          "$G1"
json "C2 name"        '.data.name'        "$G_NAME"
json "C2 workspace"   '.data.workspace'   "$G_WS"
json "C2 description" '.data.description' "$G_DESC"
json "C2 status"      '.data.status'      'active'
jqtrue "C2 createdAt" '.data.createdAt | type == "string" and test("^[0-9]{4}-")' 'createdAt is an RFC3339 instant'
jqtrue "C2 updatedAt" '.data.updatedAt | type == "string" and test("^[0-9]{4}-")' 'updatedAt is an RFC3339 instant'
jqtrue "C2 deletedAt is not projected" '.data | has("deletedAt") | not' 'no deletedAt on the wire'

api GET "/tenants?workspace=$(urlenc "$G_WS")"
expect "C3 golden listing row" 200
json "C3 name"        '.data[0].name'        "$G_NAME"
json "C3 workspace"   '.data[0].workspace'   "$G_WS"
json "C3 description" '.data[0].description' "$G_DESC"
json "C3 status"      '.data[0].status'      'active'
jqtrue "C3 deletedAt is not projected" '.data[0] | has("deletedAt") | not' 'no deletedAt on the listing row'

# ═════════════════════════════════════════════════════════════════════════════
# D — validation, 422, asserted by notification KEY
# ═════════════════════════════════════════════════════════════════════════════
section "D · validation (422)"
api POST /tenants "$(tenant_body "qa-${RUN}-d0" "aaaa" "A description that is itself perfectly fine." "active")"
expect "D1 junk display name" 422 "InvalidDisplayNameNotification"

api POST /tenants "$(tenant_body "Acme Corp" "QA Bad Handle" "A description that is itself perfectly fine." "active")"
expect "D2 malformed workspace handle" 422 "InvalidTenantWorkspaceNotification"

api POST /tenants "$(tenant_body "admin" "QA Reserved Handle" "A description that is itself perfectly fine." "active")"
expect "D3 reserved workspace handle" 422 "ReservedTenantWorkspaceNotification"

api POST /tenants "$(tenant_body "qa-${RUN}-d0" "QA Short Description" "short" "active")"
expect "D4 description below the floor" 422 "InvalidDescriptionNotification"

api POST /tenants "$(tenant_body "qa-${RUN}-d0" "QA Bad Status" "A description that is itself perfectly fine." "frozen")"
expect "D5 status outside the closed set" 422 "UnknownTenantStatusNotification"

api POST /tenants "$(tenant_body "qa-${RUN}-d0" "QA Pasted Description Case" "QA Pasted Description Case" "active")"
expect "D6 description merely repeats the name" 422 "TenantDescriptionMustDifferNotification"

D_WS="qa-${RUN}-d1"
D1=$(create_tenant "$D_WS" "QA Transition Subject" "The record the status transition rule is proven on." "active")
api PATCH "/tenants/${D1}" '{"status":"trial"}'
expect "D7 active cannot return to trial" 422 "InvalidTenantStatusTransitionNotification"

# The handle is not a member of the partial body at all (patchExcludes: [Workspace]), so
# the wire promise is that sending one changes nothing — there is no door for
# TenantWorkspaceIsImmutableNotification to answer at.
api PATCH "/tenants/${D1}" "$(jq -n '{workspace:"qa-hijacked-handle", name:"QA Transition Subject Two"}')"
expect "D8 a workspace key in a PATCH body is inert" 200
json   "D8 the handle did not move" '.data.workspace' "$D_WS"

# ═════════════════════════════════════════════════════════════════════════════
# E — 409, the duplicate flavor
# ═════════════════════════════════════════════════════════════════════════════
section "E · conflict (409)"
api POST /tenants "$(tenant_body "${P_PREFIX}1" "QA Duplicate Handle" "A second tenant reaching for a handle already held." "active")"
expect "E1 duplicate workspace" 409 "TenantWorkspaceAlreadyExistsNotification"
json   "E1 the duplicate flavor, not the wrong-state one" \
       '[.errors[].messages[] | select(.notificationKey=="TenantWorkspaceAlreadyExistsNotification") | .semantic][0]' 'Conflict'

# ═════════════════════════════════════════════════════════════════════════════
# F — the archive round-trip
# ═════════════════════════════════════════════════════════════════════════════
section "F · archive round-trip (kept-but-hidden)"
A_WS="qa-${RUN}-a1"
A1=$(create_tenant "$A_WS" "QA Archive Subject" "The record the archive round-trip is proven on." "active")

api PATCH "/tenants/${A1}/archive"
expect "F0 archive" 204

api GET "/tenants/${A1}"
expect "F1 an archived record is hidden from a plain read" 404 "RecordNotFoundNotification"

api GET "/tenants/${A1}?includeArchived=true"
expect "F2 ?includeArchived reveals it" 200
json   "F4 archive forced the status to suspended (the row, not just the audit)" '.data.status' 'suspended'

api GET "/tenants?workspace=$(urlenc "$A_WS")"
expect "F3a the listing hides it" 200
json   "F3a no rows" '.data | length' '0'
api GET "/tenants?workspace=$(urlenc "$A_WS")&includeArchived=true"
expect "F3b the listing reveals it" 200
json   "F3b one row" '.data | length' '1'

# scope: all — an archived remnant keeps holding its handle forever.
api POST /tenants "$(tenant_body "$A_WS" "QA Handle Reuse Attempt" "An attempt to reuse the handle of an archived tenant." "active")"
expect "E2 an archived tenant still blocks its workspace" 409 "TenantWorkspaceAlreadyExistsNotification"

api GET "/tenants?workspace.startswith=$(urlenc "qa-${RUN}-a")&onlyTotal=true"
expect "G12a only-total excludes archived rows" 200
json   "G12a" '.pagination.totalCount' '0'
api GET "/tenants?workspace.startswith=$(urlenc "qa-${RUN}-a")&onlyTotal=true&includeArchived=true"
expect "G12b only-total + includeArchived is a valid pair, never a conflict" 200
json   "G12b" '.pagination.totalCount' '1'

api PATCH "/tenants/${A1}/archive"
expect "F6 archiving an already-archived record finds nothing to archive" 404 "RecordNotFoundNotification"

api PATCH "/tenants/${A1}/unarchive"
expect "F5a unarchive" 204
api GET "/tenants/${A1}"
expect "F5b it is visible again" 200
json   "F5c and it comes back suspended — the rule's stated consequence" '.data.status' 'suspended'

api PATCH "/tenants/${A1}/unarchive"
expect "F7 unarchiving an active record finds no archived row" 404 "RecordNotFoundNotification"

# ═════════════════════════════════════════════════════════════════════════════
# G — the read vocabulary
# ═════════════════════════════════════════════════════════════════════════════
section "G · read vocabulary"
PQ="workspace.startswith=$(urlenc "$P_PREFIX")"

api GET "/tenants?workspace=$(urlenc "${P_PREFIX}3")"
expect "G1 eq" 200
json   "G1" '.data[0].workspace' "${P_PREFIX}3"

api GET "/tenants?${PQ}&status.in=$(urlenc 'active,trial')"
expect "G2 in" 200
json   "G2 four of the five fixtures" '.data | length' '4'

api GET "/tenants?workspace.startswith=$(urlenc "${P_PREFIX}1")"
expect "G3a startswith" 200
json   "G3a" '.data | length' '1'
api GET "/tenants?workspace.istartswith=$(urlenc "$(printf '%s' "$P_PREFIX" | tr 'a-z' 'A-Z')")"
expect "G3b istartswith is case-insensitive" 200
json   "G3b" '.data | length' '5'

api GET "/tenants?${PQ}&name.contains=$(urlenc 'Page Charlie')"
expect "G4a contains" 200
json   "G4a" '.data | length' '1'
api GET "/tenants?${PQ}&name.icontains=$(urlenc 'page charlie')"
expect "G4b icontains" 200
json   "G4b" '.data | length' '1'
api GET "/tenants?${PQ}&description.icontains=$(urlenc 'PAGING FIXTURE')"
expect "G4c icontains on description" 200
json   "G4c" '.data | length' '5'

api GET "/tenants?${PQ}&createdAt.gte=$(urlenc '2000-01-01T00:00:00Z')&createdAt.lte=$(urlenc '2999-01-01T00:00:00Z')"
expect "G5 createdAt range" 200
json   "G5" '.data | length' '5'
api GET "/tenants?${PQ}&updatedAt.gte=$(urlenc '2000-01-01T00:00:00Z')"
expect "G6 updatedAt filters even though it never sorts" 200
json   "G6" '.data | length' '5'

api GET "/tenants?${PQ}&orderBy=name"
expect "G7a orderBy ascending" 200
json   "G7a" '[.data[].name] | join("|")' 'QA Page Alpha|QA Page Bravo|QA Page Charlie|QA Page Delta|QA Page Echo'
api GET "/tenants?${PQ}&orderBy=-name"
expect "G7b orderBy descending" 200
json   "G7b" '[.data[].name] | join("|")' 'QA Page Echo|QA Page Delta|QA Page Charlie|QA Page Bravo|QA Page Alpha'
api GET "/tenants?${PQ}&orderBy=workspace"
expect "G7c orderBy workspace" 200
json   "G7c" '.data[0].workspace' "${P_PREFIX}1"
api GET "/tenants?${PQ}&orderBy=createdAt"
expect "G7d orderBy createdAt" 200
json   "G7d" '.data[0].workspace' "${P_PREFIX}1"

api GET "/tenants?${PQ}&fields=$(urlenc 'name,workspace')"
expect "G8 partial projection" 200
jqtrue "G8 the asked-for fields are there" '(.data[0].name != null) and (.data[0].workspace != null)' 'name and workspace present'
jqtrue "G8 and nothing else is"            '(.data[0].description == null) and (.data[0].status == null)' 'description and status omitted'

api GET "/tenants?${PQ}&onlyTotal=true"
expect "G9 only-total" 200
json   "G9 the count is the whole answer" '.pagination.totalCount' '5'
jqtrue "G9 no rows"      '(.data == null) or (.data | length == 0)' 'no data array'
jqtrue "G9 no page flags" '(.pagination | has("hasNextPage") | not)' 'no hasNextPage in count mode'

api GET "/tenants?${PQ}&last=2"
expect "G10 ?last alone serves the tail window" 200
json   "G10 the last two"         '[.data[].workspace] | join("|")' "${P_PREFIX}4|${P_PREFIX}5"
json   "G10 nothing beyond them"  '.pagination.hasNextPage' 'false'

section "G11 · the pagination envelope as a contract"
api GET "/tenants?${PQ}&first=2"
expect "G11a page one" 200
json   "G11a totalCount is the FULL filtered count" '.pagination.totalCount' '5'
json   "G11a hasNextPage"     '.pagination.hasNextPage' 'true'
json   "G11a hasPreviousPage" '.pagination.hasPreviousPage' 'false'
json   "G11a the rows"        '[.data[].workspace] | join("|")' "${P_PREFIX}1|${P_PREFIX}2"
P1_END=$(jq -r '.pagination.endCursor' "$BODY")
P1_START=$(jq -r '.pagination.startCursor' "$BODY")
jqtrue "G11a endCursor is present exactly when hasNextPage" \
       '(.pagination.hasNextPage == true) and (.pagination.endCursor != null and .pagination.endCursor != "")' \
       'endCursor rides with hasNextPage'

api GET "/tenants?${PQ}&first=2&after=$(urlenc "$P1_END")"
expect "G11b walking forward on the window edge" 200
json   "G11b page two is disjoint from page one" '[.data[].workspace] | join("|")' "${P_PREFIX}3|${P_PREFIX}4"
json   "G11b hasPreviousPage flipped"            '.pagination.hasPreviousPage' 'true'
json   "G11b totalCount is unchanged by paging"  '.pagination.totalCount' '5'
P2_START=$(jq -r '.pagination.startCursor' "$BODY")
P2_END=$(jq -r '.pagination.endCursor' "$BODY")

api GET "/tenants?${PQ}&last=2&before=$(urlenc "$P2_START")"
expect "G11c walking backward returns to page one" 200
json   "G11c" '[.data[].workspace] | join("|")' "${P_PREFIX}1|${P_PREFIX}2"

# The last page: an edge cursor is emitted only where its neighbouring page exists, so
# an empty endCursor here means "nothing to walk to", never a contradiction with the flag.
api GET "/tenants?${PQ}&first=2&after=$(urlenc "$P2_END")"
expect "G11d the last page" 200
json   "G11d the remaining row"      '[.data[].workspace] | join("|")' "${P_PREFIX}5"
json   "G11d nothing beyond it"      '.pagination.hasNextPage' 'false'
jqtrue "G11d and therefore no endCursor" \
       '(.pagination.endCursor == null) or (.pagination.endCursor == "")' 'endCursor is empty on the last page'

# ═════════════════════════════════════════════════════════════════════════════
# H — the typed-400 guard family
# ═════════════════════════════════════════════════════════════════════════════
section "H · rejected reads (typed 400)"
api GET "/tenants?bogus=1"
expect "H1 unknown query key" 400 "SchemaViolationNotification"

api GET "/tenants?workspace.contains=$(urlenc 'qa')"
expect "H2 operator outside the field's allowlist" 400 "SchemaViolationNotification"

# NOT UnsupportedCapabilityNotification: the DTO never declares `search`, so the opt-in
# gate answers before any read engine is reached. The relational refusal is a promise
# this endpoint does not get the chance to make.
api GET "/tenants?search=$(urlenc 'acme')"
expect "H3 a reserved control the DTO never declared" 400 "SchemaViolationNotification" "search"

api GET "/tenants?fields=bogus"
expect "H4 unresolvable projection path" 400 "SchemaViolationNotification"

api GET "/tenants?orderBy=status"
expect "H5 filterable but never declared orderable" 400 "SchemaViolationNotification"
api GET "/tenants?orderBy=updatedAt"
expect "H6 same, on the managed stamp" 400 "SchemaViolationNotification"
api GET "/tenants?orderBy=bogus"
expect "H7 unknown ordering token" 400 "SchemaViolationNotification"
api GET "/tenants?${PQ}&orderBy=-workspace"
expect "H8 a direction the declaration DOES admit" 200
json   "H8" '.data[0].workspace' "${P_PREFIX}5"

api GET "/tenants?first=101"
expect "H9 above the page ceiling" 400 "LimitExceededNotification" "first"
json   "H9 the effective maximum is reported" \
       '[.errors[].messages[] | select(.notificationKey=="LimitExceededNotification") | .value][0] | tostring' '100'
api GET "/tenants?first=0"
expect "H10 a non-positive page size" 400 "SchemaViolationNotification" "first"

api GET "/tenants?first=2&last=2"
expect "H11a first + last" 400 "SchemaViolationNotification" "last"
api GET "/tenants?first=2&before=$(urlenc "$P1_END")"
expect "H11b first + before" 400 "SchemaViolationNotification" "before"
# The violation is always reported on the BACKWARD-side key: `last` when it is present,
# `before` otherwise (web/queryschema/gate.go, ValidateControls step 2).
api GET "/tenants?last=2&after=$(urlenc "$P1_END")"
expect "H11c last + after" 400 "SchemaViolationNotification" "last"
api GET "/tenants?after=$(urlenc "$P1_END")&before=$(urlenc "$P2_START")"
expect "H11d after + before" 400 "SchemaViolationNotification" "before"

api GET "/tenants?onlyTotal=true&first=10"
expect "H12a only-total + first" 400 "SchemaViolationNotification" "onlyTotal[first]"
api GET "/tenants?onlyTotal=true&orderBy=name"
expect "H12b only-total + orderBy" 400 "SchemaViolationNotification" "onlyTotal[orderBy]"
api GET "/tenants?onlyTotal=true&fields=name"
expect "H12c only-total + fields" 400 "SchemaViolationNotification" "onlyTotal[fields]"
api GET "/tenants?onlyTotal=true&after=$(urlenc "$P1_END")"
expect "H12d only-total + after" 400 "SchemaViolationNotification" "onlyTotal[after]"

api GET "/tenants?${PQ}&onlyTotal=true&status.in=$(urlenc 'active,trial')"
expect "H13 counting a FILTERED subset is the point, not a conflict" 200
json   "H13" '.pagination.totalCount' '4'

api GET "/tenants?after=not-a-cursor"
expect "H14 malformed cursor" 400 "SchemaViolationNotification" "after"

# The cursor was issued for a listing with no ordering; replaying it under one is a
# different listing context, and the context hash is what catches it.
api GET "/tenants?${PQ}&first=2&after=$(urlenc "$P1_END")&orderBy=name"
expect "H15 cursor ↔ orderBy mismatch" 400 "SchemaViolationNotification" "after"
# NOTE — the two cursor rejections name DIFFERENT fields, and both are the contract as
# implemented at this pin:
#   • a STRUCTURAL mismatch (the ordering tuple no longer matches) is caught by the REST
#     wrapper before dispatch and names the wire key — `after` above;
#   • a CONTEXT-HASH mismatch (filter / search / includeArchived changed) is caught in the
#     reader, which raises core.InvalidCursorError with FieldName "cursor".
# The pin's auto-query-handlers doc says both land "on the cursor's wire key (after or
# before)". That is inaccurate for the second path — a FRAMEWORK doc/impl discrepancy,
# reported upstream rather than papered over here. The promise this case proves is
# unchanged: a cursor replayed under a different listing context is refused, typed.
api GET "/tenants?${PQ}&first=2&after=$(urlenc "$P1_END")&includeArchived=true"
expect "H16 cursor ↔ includeArchived mismatch" 400 "SchemaViolationNotification" "cursor"

api GET "/tenants?includeArchived=1"
expect "H17a booleans take exactly true or false" 400 "SchemaViolationNotification" "includeArchived"
api GET "/tenants?onlyTotal="
expect "H17b presence gates, not emptiness" 400 "SchemaViolationNotification" "onlyTotal"

# The by-id DTO declares includeArchived and NOTHING else. Declared-ness is the test.
api GET "/tenants/${G1}?onlyTotal=false"
expect "H18a a control this endpoint never declared" 400 "SchemaViolationNotification" "onlyTotal"
api GET "/tenants/${G1}?fields=name"
expect "H18b same, on ?fields=" 400 "SchemaViolationNotification" "fields"
api GET "/tenants/${G1}?includeArchived=true"
expect "H19 the one control it DOES declare" 200

# ═════════════════════════════════════════════════════════════════════════════
# I — routing and not-found
# ═════════════════════════════════════════════════════════════════════════════
section "I · routing and not-found"
api GET "/tenants/018f0000-0000-7000-8000-0000000fffff"
expect "I1 an unused id" 404 "RecordNotFoundNotification"

api DELETE "/tenants/${G1}"
expect "I2 there is no hard delete — the path is registered, the method is not" 405 "MethodNotAllowedNotification"

api POST "/tenants/${G1}" '{}'
expect "I3 POST on the by-id path" 405 "MethodNotAllowedNotification"

# The archive path IS registered — under PATCH. A GET on it is therefore the 405 arm of
# the split, not the 404 one: Fiber matched the path and refused the method.
api GET "/tenants/${G1}/archive"
expect "I4a a registered path reached with the wrong method" 405 "MethodNotAllowedNotification"
# The 404 arm needs a path no registered route matches at all.
api GET "/tenants/${G1}/purge"
expect "I4b a path no route matches" 404 "RouteNotFoundNotification"

# ── the by-id ADDRESS, split by VERB (pin >= v0.70.0) ────────────────────────
#
# A segment that is not a UUID is refused AT THE WRAPPER, before the handler, and what
# the consumer hears follows the VERB rather than the view's backing: a READ named no
# record (404), a WRITE stated an intention about one and violated the request shape
# (400). The key is deliberately NOT RecordNotFoundNotification — that one means a
# well-formed address matched no row (I1 above), and the two must stay distinguishable.
api GET "/tenants/not-a-uuid"
expect "I5 a malformed address on a READ" 404 "UnknownIDAddressNotification" "id"
json   "I5 the rejected segment is echoed" '[.errors[].messages[].value][0]' 'not-a-uuid'
json   "I5 an address problem is not labelled a payload problem" '[.errors[].context][0]' 'Request'

api PATCH "/tenants/not-a-uuid" '{"name":"QA Malformed Address"}'
expect "K1 a malformed address on a WRITE with a body" 400 "MalformedIDNotification" "id"
json   "K1 the rejected segment is echoed" '[.errors[].messages[].value][0]' 'not-a-uuid'

# The bodyless by-id commands — the arm the body-shaped rule left undeclared in OpenAPI
# before this pin, and therefore the arm a regression hides in most easily.
api PATCH "/tenants/not-a-uuid/archive"
expect "K2 archive, bodyless by-id" 400 "MalformedIDNotification" "id"
api PATCH "/tenants/not-a-uuid/unarchive"
expect "K3 unarchive, bodyless by-id" 400 "MalformedIDNotification" "id"

# An EMPTY segment is deliberately NOT part of this family: it means the route was
# mounted without :id, which handlers.RequirePathID diagnoses as the developer error it
# is. /tenants/ with no id is the COLLECTION route, and it answers the listing.
api GET "/tenants/"
expect "K4 the collection route is not an empty address" 200

# ═════════════════════════════════════════════════════════════════════════════
# J — GraphQL: the same handlers, the other dialect
# ═════════════════════════════════════════════════════════════════════════════
section "J · GraphQL surface"
api_as "" POST /graphql "$(jq -n '{query:"{ __typename }"}')"
expect "J8 GraphQL is not a public route" 401 "MissingAuthorizationNotification"

gql "{ tenants(where: {workspace: {startswith: \"${P_PREFIX}\"}}, first: 2) { totalCount pageInfo { hasNextPage hasPreviousPage startCursor endCursor } edges { cursor node { id name workspace status } } } }"
expect "J1a the connection" 200
json   "J1b totalCount matches its REST twin" '.data.tenants.totalCount' '5'
json   "J1c the same first page"              '[.data.tenants.edges[].node.workspace] | join("|")' "${P_PREFIX}1|${P_PREFIX}2"
json   "J1d hasNextPage"                      '.data.tenants.pageInfo.hasNextPage' 'true'

gql "{ tenant(id: \"${G1}\") { id name workspace description status createdAt updatedAt } }"
expect "J2a the singular twin" 200
json   "J2b the same document REST serves" '.data.tenant.workspace' "$G_WS"
json   "J2c description"                   '.data.tenant.description' "$G_DESC"

J_WS="qa-${RUN}-j1"
gql "mutation { createTenant(input: {name: \"QA GraphQL Write\", workspace: \"${J_WS}\", description: \"Written through GraphQL and read back through REST.\", status: \"trial\"}) { id workspace } }"
expect "J3a createTenant" 200
J1=$(jq -r '.data.createTenant.id' "$BODY")
api GET "/tenants/${J1}"
expect "J3b one write, two surfaces — REST sees it" 200
json   "J3b" '.data.workspace' "$J_WS"

gql "mutation { patchTenant(id: \"${J1}\", input: {name: \"QA GraphQL Write Renamed\"}) { id name } }"
expect "J4a patchTenant" 200
api GET "/tenants/${J1}"
json   "J4b REST sees the patch" '.data.name' 'QA GraphQL Write Renamed'

gql "mutation { archiveTenant(id: \"${J1}\") { success id } }"
expect "J5a archiveTenant" 200
json   "J5a the bodyless payload"   '.data.archiveTenant.success' 'true'
api GET "/tenants/${J1}"
expect "J5b REST sees the archive" 404 "RecordNotFoundNotification"
gql "mutation { unarchiveTenant(id: \"${J1}\") { success id } }"
expect "J5c unarchiveTenant" 200
api GET "/tenants/${J1}"
expect "J5d REST sees the restore" 200

gql "{ tenant(id: \"018f0000-0000-7000-8000-0000000fffff\") { id } }"
expect "J6a a missing record answers HTTP 200 in the GraphQL idiom" 200
json   "J6b the typed notification rides in extensions" \
       '[.errors[].extensions.notificationKey] | index("RecordNotFoundNotification") != null' 'true'

gql "mutation { createTenant(input: {name: \"QA GraphQL Duplicate\", workspace: \"${P_PREFIX}1\", description: \"A handle already held, reached through the other surface.\", status: \"active\"}) { id } }"
expect "J7a duplicate through GraphQL" 200
json   "J7b same notification key as REST" \
       '[.errors[].extensions.notificationKey] | index("TenantWorkspaceAlreadyExistsNotification") != null' 'true'
json   "J7c same semantic as REST" \
       '[.errors[] | select(.extensions.notificationKey=="TenantWorkspaceAlreadyExistsNotification") | .extensions.semantic][0]' 'Conflict'

# The DTO cuts the SCHEMA here: `search` is not an argument the endpoint advertises, so
# the refusal is a gqlparser validation error, not the REST 400 envelope. Asserting the
# REST shape cross-surface would be asserting the wrong contract.
gql "{ tenants(search: \"acme\", first: 1) { totalCount } }"
expect "J9a an undeclared control is an UNKNOWN ARGUMENT here" 200
jqtrue "J9b the query is refused, not served" '(.errors | length) > 0 and (.data.tenants == null)' 'errors[] present, no data'

gql "{ tenants(where: {workspace: {startswith: \"${P_PREFIX}\"}}, orderBy: [{field: WORKSPACE, direction: DESC}], first: 5) { edges { node { workspace } } } }"
expect "J10a typed where + orderBy" 200
json   "J10b the same order its REST twin returns" \
       '[.data.tenants.edges[].node.workspace] | join("|")' \
       "${P_PREFIX}5|${P_PREFIX}4|${P_PREFIX}3|${P_PREFIX}2|${P_PREFIX}1"

# The same rule, the other surface. At v0.69.0 this answered an UNTYPED error carrying
# only {"semantic":"Internal"} and no notificationKey at all, so a GraphQL consumer could
# not tell a bad address from a genuine server fault. The split is by VERB here too.
gql "{ tenant(id: \"not-a-uuid\") { id } }"
expect "K5a GraphQL read, malformed address" 200
json   "K5b typed, and it is the READ key" \
       '[.errors[].extensions.notificationKey][0]' 'UnknownIDAddressNotification'
json   "K5c semantic" '[.errors[].extensions.semantic][0]' 'NotFound'
json   "K5d the segment is echoed here too" '[.errors[].extensions.value][0]' 'not-a-uuid'

gql "mutation { archiveTenant(id: \"not-a-uuid\") { success } }"
expect "K6a GraphQL write, malformed address" 200
json   "K6b typed, and it is the WRITE key" \
       '[.errors[].extensions.notificationKey][0]' 'MalformedIDNotification'
json   "K6c semantic" '[.errors[].extensions.semantic][0]' 'Schema'

# ═════════════════════════════════════════════════════════════════════════════
# L — a filter VALUE the leaf cannot take (pin >= v0.70.0)
# ═════════════════════════════════════════════════════════════════════════════
section "L · filter values outside the leaf's declared kind"
# Section H proves fifteen flavors of typed 400, but every one of them violates the query
# GRAMMAR (unknown key, operator outside the allowlist, bad cursor). These two send a
# well-formed key carrying a value the COLUMN cannot hold — the other half of the rule.
#
# EXPECTED RED at v0.70.0, on purpose (plan gate Q1). The pin promises 400
# InvalidFilterValueNotification and states the probe "never reaches the driver"; the
# temporal leaf kind still falls through and answers an external 500. Verified as a
# framework defect, not a service one — every other declared kind (domain.ID, bool, and
# a plain string over an identity column) is guarded correctly. Written up in
# specs/qa/id-address-and-filter-values/finding-filter-value-temporal-leaf.md.
# These cases turn green on their own when the fix ships; nothing here needs editing.
api GET "/tenants?createdAt.gte=$(urlenc 'not-a-date')"
expect "L1 a range operator on a temporal leaf" 400 "InvalidFilterValueNotification"
api GET "/tenants?createdAt=$(urlenc 'not-a-date')"
expect "L2 equality on the same temporal leaf" 400 "InvalidFilterValueNotification"

# ═════════════════════════════════════════════════════════════════════════════
section "summary"
printf '  %sGREEN %d%s · %sRED %d%s · %sSKIPPED %d%s\n' "$G" "$PASS" "$Z" "$R" "$FAIL" "$Z" "$Y" "$SKIP" "$Z"
[[ -n "$QA_LOG_DIR" ]] && printf '%d %d %d\n' "$PASS" "$FAIL" "$SKIP" > "${QA_LOG_DIR}/${LANE_NAME}.counts"
printf '  residue: the throwaway database %s, recreated on the next run. authcore_db untouched.\n' "$QA_DB"
[[ $FAIL -eq 0 ]] || printf '  server log: %s (kept until this script exits)\n' "$LOG"
exit $(( FAIL > 0 ? 1 : 0 ))
