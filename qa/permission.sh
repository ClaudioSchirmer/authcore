#!/usr/bin/env bash
#
# Contract QA lane — Permission. Plan: specs/qa/permission-catalog/plan.md (APPROVED).
#
# WHAT THIS PROVES: what the wire promises for the global permission catalog at omnicore
# v0.71.0 — the FIVE verbs it serves (there is no unarchive, by design), the status code
# and notification KEY of each refusal, the one-way archive with an active-only handle,
# the read vocabulary, the typed-400 guard family, and REST/GraphQL handler invariance.
# It asserts BEHAVIOUR, never internals.
#
# THE SHAPE THIS LANE EXISTS FOR — THREE COLUMNS GO IN, TWO COME OUT:
#   resource  written · filterable · sortable · in NO response body, on any surface
#   action    written · filterable · sortable · in NO response body, on any surface
#   permission  no column at all — computed as `resource:action` on the way out
# No other entity in this service has that shape, and section C is where it is proven.
#
# READ-YOUR-WRITES. The read model is a RelationalView served straight from the tables —
# no projection, no CDC. Every read-back below is IMMEDIATE, and a case that would only
# pass after a retry is itself a failure of the contract.
#
# SELF-CONTAINED: builds and boots its own server on its OWN port and drains it with
# SIGTERM before returning. Run it through ./qa/run.sh; running it directly does the same.
#
# ⚠️ LANES ARE SEQUENTIAL-ONLY. This lane shares the throwaway database authcore_qa_db
# with the tenant lane (plan §2, decision Q1) and drops it as its first act. qa/run.sh
# runs lanes one after another, so that is safe as invoked — but two lanes started by hand
# in two terminals WILL destroy each other's data. To split them, export QA_DATABASE_URL:
# the qa yaml already reads ${QA_DATABASE_URL:...} and no file needs to change.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# ── lane constants ───────────────────────────────────────────────────────────
LANE_NAME="permission"
PORT="${QA_PERMISSION_PORT:-8098}"     # tenant holds 8099; every lane owns its own socket
QA_DB="authcore_qa_db"
COMPOSE="devops/docker-compose.yml"
BASE="http://127.0.0.1:${PORT}"

# The scope token every fixture resource carries. The 39 rows the bootstrap seed puts in
# this table are REAL catalog entries the service's own authorization reads, so no count
# in this lane is ever taken unscoped: each one filters on ?resource.startswith=$PREFIX.
#
# Repeated digits are squeezed out because a resource segment is refused when it carries a
# run of four identical runes — an epoch like 1777123456 would make every fixture invalid
# and the failure would look like a service defect instead of a bad fixture.
RUN="$(date +%s)"
RUNTOKEN="$(printf '%s' "$RUN" | sed 's/\(.\)\1\{1,\}/\1/g')"
PREFIX="qa-${RUNTOKEN}-"
# The narrower scope every EXACT COUNT uses: the five paging fixtures, which no other
# section archives, patches or adds to. Counting over $PREFIX instead would make section G
# depend on how many rows sections B, C, E and J happened to leave live — an expectation
# that silently rots the next time one of them changes.
PAGE="${PREFIX}p"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/authcore-qa-${LANE_NAME}-$$-XXXXXX")"
BIN="${TMP}/authcore-qa"
LOG="${TMP}/server.log"
BODY="${TMP}/body.json"
SERVER_PID=""

PASS=0; FAIL=0; SKIP=0
STATUS=""
TOKEN=""

QA_LOG_DIR="${QA_LOG_DIR:-}"
FAILFILE=""
[[ -n "$QA_LOG_DIR" ]] && { mkdir -p "$QA_LOG_DIR"; FAILFILE="${QA_LOG_DIR}/${LANE_NAME}.failures"; : > "$FAILFILE"; }

# Fixture credentials — the tracked bootstrap seed, not invented here.
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
  [[ -n "$QA_LOG_DIR" && -f "$LOG" ]] && cp "$LOG" "${QA_LOG_DIR}/${LANE_NAME}-server.log" 2>/dev/null
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    # SIGTERM, never -9: the drain is part of the contract and the next lane cannot bind
    # this port until it completes.
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
# not hostage to the machine's locale.
api_as() { # token method path [json-body]
  local tok=$1 method=$2 path=$3 body=${4-}
  local args=(-s -o "$BODY" -w '%{http_code}' -X "$method" -H 'Accept-Language: en')
  [[ -n "$tok"  ]] && args+=(-H "Authorization: Bearer ${tok}")
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' --data-binary "$body")
  STATUS=$(curl "${args[@]}" "${BASE}${path}" 2>/dev/null)
}
api() { api_as "$TOKEN" "$@"; }
urlenc() { jq -rn --arg v "$1" '$v|@uri'; }
gql() { api POST /graphql "$(jq -n --arg q "$1" '{query:$q}')"; }

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

# absent <case> <json path expr> <where> — the assertion this lane is built around.
# A hidden part must reach NO response body: not as null, not as an empty string, not at
# all. `has()` is the only test that says that; comparing to null would pass on a key
# that is present and null, which is a different and weaker promise.
absent() { # case  object-filter  key  where
  local name=$1 obj=$2 key=$3 where=$4
  if jq -e --arg k "$key" "($obj) | has(\$k) | not" "$BODY" >/dev/null 2>&1; then
    pass "$name — no '$key' key $where"
  else
    fail "$name" "'$key' absent $where" "keys: $(jq -c "$obj | keys" "$BODY" 2>/dev/null)"
  fi
}

# ── fixtures ─────────────────────────────────────────────────────────────────
# Descriptions satisfy the shared Description value object (>=15 runes, >=2 words, >=5
# distinct runes, >=1 vowel, no run of 4 identical) and never normalize to the rendered
# key, which the manual rule description-does-not-echo-key refuses.
perm_body() { # resource action description
  jq -n --arg r "$1" --arg a "$2" --arg d "$3" '{resource:$r, action:$a, description:$d}'
}
create_permission() { # resource action description -> echoes the new id
  api POST /permissions "$(perm_body "$1" "$2" "$3")"
  [[ "$STATUS" == "201" ]] || die "fixture insert $1:$2 answered HTTP $STATUS: $(cat "$BODY")"
  jq -r '.data.id' "$BODY"
}

# ═════════════════════════════════════════════════════════════════════════════
# PREFLIGHT
# ═════════════════════════════════════════════════════════════════════════════
section "preflight"
for tool in curl jq docker go; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required and was not found on PATH"
done
printf '  run id %s · prefix %s · port %s · database %s\n' "$RUN" "$PREFIX" "$PORT" "$QA_DB"

if ! docker compose -f "$COMPOSE" ps --status running --services 2>/dev/null | grep -q '^postgres$'; then
  printf '  bringing the bench up…\n'
  docker compose -f "$COMPOSE" up -d --wait >/dev/null 2>&1 || die "docker compose up failed"
fi

# The throwaway database, dropped and recreated per run. migrations.autoRun then rebuilds
# the schema AND the bootstrap seed — including the 39 catalog rows this lane reads but
# never writes. Nothing here ever reaches authcore_db.
printf '  provisioning %s…\n' "$QA_DB"
docker compose -f "$COMPOSE" exec -T postgres dropdb -U omnicore --if-exists "$QA_DB" >/dev/null 2>&1
docker compose -f "$COMPOSE" exec -T postgres createdb -U omnicore "$QA_DB" >/dev/null 2>&1 \
  || die "could not create $QA_DB inside the bench container"

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

# Probe the port BEFORE booting: something already listening means the cases would run
# against a binary this lane did not build.
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

api_as "" GET /permissions
expect "A4 tokenless listing is refused" 401 "MissingAuthorizationNotification"

api_as "not-a-real-token" GET /permissions
expect "A5 bogus bearer is refused" 401 "InvalidTokenNotification"

api_as "" POST /auth/user/token \
  "$(jq -n --arg e "$BOOTSTRAP_EMAIL" --arg p "$BOOTSTRAP_PASSWORD" '{email:$e,password:$p}')"
expect "A1 bootstrap sign-in" 200
json   "A1 must-change flag is set" '.data.user.mustChangePassword' 'true'
json   "A1 the restricted token carries one permission only" \
       '.data.user.permissions | sort | join(",")' 'user:change-password'
RESTRICTED_TOKEN=$(jq -r '.data.accessToken' "$BODY" 2>/dev/null)

# The gate names THIS resource's key — the catalog row the seed put there for this route.
api_as "$RESTRICTED_TOKEN" GET /permissions
expect "A2 the permission gate refuses the restricted token" 403 "MissingPermissionNotification" "permission"
jqtrue "A2b it names permission:read" \
       '[.errors[]?.messages[]?.value] | index("permission:read") != null' 'value is permission:read'

api_as "$RESTRICTED_TOKEN" PATCH "/users/${BOOTSTRAP_USER_ID}/password" \
  "$(jq -n --arg c "$BOOTSTRAP_PASSWORD" --arg n "$QA_PASSWORD" \
        '{currentPassword:$c, password:$n, passwordConfirmation:$n}')"
expect "A3a password rotation" 204

api_as "" POST /auth/user/token \
  "$(jq -n --arg e "$BOOTSTRAP_EMAIL" --arg p "$QA_PASSWORD" '{email:$e,password:$p}')"
expect "A3b re-sign-in after the rotation" 200
json   "A3b the flag is cleared" '.data.user.mustChangePassword' 'false'
TOKEN=$(jq -r '.data.accessToken' "$BODY" 2>/dev/null)
[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || die "no usable access token — every later case would be meaningless"

# ═════════════════════════════════════════════════════════════════════════════
# X — the route inventory, read from the service itself
# ═════════════════════════════════════════════════════════════════════════════
section "X · route inventory (openapi.json is the oracle)"
api GET /openapi.json
expect "X1a openapi document is served" 200
json "X1b the FIVE permission routes openapi enumerates" \
  '[.paths | to_entries[] | select(.key|startswith("/permissions")) | . as $e | ($e.value|keys[]) | (ascii_upcase + " " + $e.key)] | sort | join(" ")' \
  'GET /permissions/ GET /permissions/{id} PATCH /permissions/{id} PATCH /permissions/{id}/archive POST /permissions/'
# Stated positively as well: the absence of unarchive is a DESIGN decision (un-archiving
# would re-enable every grant still pointing at the row, in one call), so it is asserted
# rather than merely not-mentioned.
# Scoped to THIS entity's paths on purpose: /tenants/{id}/unarchive legitimately exists,
# so an unscoped search for the word would be a case that can never pass.
jqtrue "X1c openapi advertises NO unarchive route for this entity" \
  '[.paths | keys[] | select(startswith("/permissions") and test("unarchive"))] | length == 0' \
  'no /permissions/{id}/unarchive'
# The collection verbs are advertised with a trailing slash — they are mounted on the
# Fiber group under path "/". Fiber serves both spellings; the cases below call
# /permissions and a generated client would call /permissions/. Same five routes.

# ═════════════════════════════════════════════════════════════════════════════
# seed — the fixtures the read-vocabulary cases count on
# ═════════════════════════════════════════════════════════════════════════════
section "seed"
# Five scoped rows, ordered p1..p5 by resource. EVERY count assertion in section G filters
# on this prefix: the table already holds the 39 bootstrap catalog rows, so an unscoped
# count would be neither exact nor stable.
P1=$(create_permission "${PREFIX}p1" "read"    "Contract suite paging fixture number one.")
P2=$(create_permission "${PREFIX}p2" "insert"  "Contract suite paging fixture number two.")
P3=$(create_permission "${PREFIX}p3" "update"  "Contract suite paging fixture number three.")
P4=$(create_permission "${PREFIX}p4" "archive" "Contract suite paging fixture number four.")
P5=$(create_permission "${PREFIX}p5" "read"    "Contract suite paging fixture number five.")
# A fixture of its own for the immutability case. D12 rewrites the description of whatever
# it patches, so it must never be one of the five above — section G counts them by
# description as well as by pair, and a case that quietly edits another case's fixture is
# how an arithmetic expectation drifts away from what the lane actually created.
IMM=$(create_permission "${PREFIX}imm" "read" "The fixture the immutability case may edit.")
printf '  seeded 5 paging fixtures under %s plus one for D12\n' "$PAGE"

# ═════════════════════════════════════════════════════════════════════════════
# B — happy path, one per served verb
# ═════════════════════════════════════════════════════════════════════════════
section "B · happy path per served verb"

api POST /permissions "$(perm_body "${PREFIX}b" "read" "The happy path fixture for this lane.")"
expect "B1 insert" 201
json   "B1b the rendered pair" '.data.permission' "${PREFIX}b:read"
BID=$(jq -r '.data.id' "$BODY")

api GET "/permissions/${BID}"
expect "B2 by-id, read back IMMEDIATELY (relational posture)" 200
json   "B2b same pair" '.data.permission' "${PREFIX}b:read"
json   "B2c same description" '.data.description' "The happy path fixture for this lane."

api GET "/permissions?resource=${PREFIX}b"
expect "B3 listing filtered to it" 200
json   "B3b exactly one row" '.data | length' '1'

api PATCH "/permissions/${BID}" "$(jq -n '{description:"An improved wording for the same entry."}')"
expect "B4 patch the description" 200
json   "B4b the new wording" '.data.description' "An improved wording for the same entry."
json   "B4c the pair is untouched" '.data.permission' "${PREFIX}b:read"

api PATCH "/permissions/${BID}/archive"
expect "B5 archive" 204

# There is no B6: unarchive does not exist. That is case I6, not a missing happy path.

# ═════════════════════════════════════════════════════════════════════════════
# C — the golden record, and THE HIDDEN PARTS
# ═════════════════════════════════════════════════════════════════════════════
section "C · golden record · three columns in, two out"
# The family this lane exists for. Every other family would pass unchanged if `resource`
# started leaking into response bodies tomorrow.

GOLD_R="${PREFIX}gold"
api POST /permissions "$(perm_body "$GOLD_R" "read" "The golden record exercising every declared field.")"
expect "C1a insert answers 201" 201
json   "C1b permission is the rendered pair" '.data.permission' "${GOLD_R}:read"
json   "C1c description round-trips" '.data.description' "The golden record exercising every declared field."
jqtrue "C1d id is a uuid" '.data.id | test("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")' 'id looks like a uuid'
GOLD=$(jq -r '.data.id' "$BODY")

absent "C2a" '.data' 'resource' 'in the insert response'
absent "C2b" '.data' 'action'   'in the insert response'
absent "C2c" '.data' 'key'      'in the insert response'

api PATCH "/permissions/${GOLD}" "$(jq -n '{description:"The golden record after one ordinary edit."}')"
expect "C3a patch answers 200" 200
absent "C3b" '.data' 'resource' 'in the patch response'
absent "C3c" '.data' 'action'   'in the patch response'
absent "C3d" '.data' 'key'      'in the patch response'
json   "C3e the pair survives the edit" '.data.permission' "${GOLD_R}:read"

api GET "/permissions/${GOLD}"
expect "C4a by-id answers 200" 200
json   "C4b the exact key set the document promises" \
       '.data | keys | sort | join(",")' 'createdAt,description,id,permission,updatedAt'
# Matched against the RFC3339 grammar rather than piped through jq's fromdate: that
# builtin accepts only a UTC "Z" instant with no fractional part, while the framework
# renders a full RFC3339 timestamp with microseconds and a numeric offset. Using fromdate
# here would fail a perfectly conformant value and read as a service defect.
RFC3339='^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
jqtrue "C4c createdAt is RFC3339" ".data.createdAt | test(\"${RFC3339}\")" 'createdAt matches RFC3339'
jqtrue "C4d updatedAt is RFC3339" ".data.updatedAt | test(\"${RFC3339}\")" 'updatedAt matches RFC3339'
absent "C5a" '.data' 'resource'  'in the by-id document'
absent "C5b" '.data' 'action'    'in the by-id document'
absent "C5c" '.data' 'key'       'in the by-id document'
absent "C5d" '.data' 'revision'  'in the by-id document'
absent "C5e" '.data' 'deletedAt' 'in the by-id document'

api GET "/permissions?resource=${GOLD_R}"
expect "C6a listing answers 200" 200
json   "C6b the listing row carries the same key set as the document" \
       '.data[0] | keys | sort | join(",")' 'createdAt,description,id,permission,updatedAt'
absent "C6c" '.data[0]' 'resource' 'in the listing row'
absent "C6d" '.data[0]' 'action'   'in the listing row'

gql "{ permission(id: \"${GOLD}\") { id description permission createdAt updatedAt } }"
expect "C7a GraphQL node answers 200" 200
json   "C7b same rendered pair as REST" '.data.permission.permission' "${GOLD_R}:read"
json   "C7c same description as REST" '.data.permission.description' "The golden record after one ordinary edit."
json   "C7d same id" '.data.permission.id' "$GOLD"

# The SCHEMA, not just the payload: a hidden part is not a node field at all, so asking
# for one is a validation error rather than a null. That is the stronger promise.
gql "{ permissions(first: 1) { edges { node { resource } } } }"
expect "C8a asking the node for a hidden part" 200
jqtrue "C8b the schema refuses it outright" \
       '(.errors | length) > 0 and (.data == null) and ([.errors[].message] | join(" ") | test("Cannot query field .resource."))' \
       'unknown field on type Permission'

# The whole point of the model: the pair stays QUERYABLE while leaving no value on the
# wire. Filters are declared on the Request DTO and never consult the Response.
api GET "/permissions?resource=${GOLD_R}&action=read"
expect "C9a both hidden parts used as filters" 200
json   "C9b they select exactly the record" '.data | length' '1'
json   "C9c and it is the right one, named only by the rendering" '.data[0].permission' "${GOLD_R}:read"
absent "C9d" '.data[0]' 'resource' 'even when it was the thing filtered on'

# ═════════════════════════════════════════════════════════════════════════════
# D — validation, 422, asserting the KEY
# ═════════════════════════════════════════════════════════════════════════════
section "D · validation (422)"
DESC="A perfectly ordinary sentence about it."

api POST /permissions "$(perm_body "Tenant" "read" "$DESC")"
expect "D1 an uppercase resource" 422 "InvalidResourceNameNotification" "resource"

api POST /permissions "$(perm_body "user::profile" "read" "$DESC")"
expect "D2 an empty path segment" 422 "InvalidResourceNameNotification" "resource"

api POST /permissions "$(perm_body "${PREFIX}d3" "read:all" "$DESC")"
expect "D3 a colon inside the action" 422 "InvalidActionNameNotification" "action"

api POST /permissions "$(perm_body "${PREFIX}d4" "ten*" "$DESC")"
expect "D4 a wildcard mixed into a slug" 422 "InvalidActionNameNotification" "action"

api POST /permissions "$(perm_body "user:*" "read" "$DESC")"
expect "D5 a wildcard inside a path" 422 "InvalidResourceNameNotification" "resource"

# The pair-level invariant, and the reason this concept had to be a composite: it is only
# expressible with both halves in hand. It is reported against the action, the half that
# has to change.
api POST /permissions "$(perm_body "*" "read" "$DESC")"
expect "D6 a wildcard resource with a concrete action" 422 "UnmatchablePermissionKeyNotification" "action"

api POST /permissions "$(perm_body "" "read" "$DESC")"
expect "D7 an empty resource" 422 "RequiredFieldNotification" "resource"

api POST /permissions "$(perm_body "${PREFIX}d8" "" "$DESC")"
expect "D8 an empty action" 422 "RequiredFieldNotification" "action"

api POST /permissions "$(perm_body "${PREFIX}d9" "read" "short")"
expect "D9 a description below the floor" 422 "InvalidDescriptionNotification" "description"

# The description is VALID prose on its own — 4 words, 18 runes — and still normalizes to
# the rendered key. Isolating the echo rule needs that: a description that merely repeats
# the key is usually also too short, and then both notifications fire and the case would
# pass for the wrong reason.
api POST /permissions "$(perm_body "qa-probe-echo" "read" "Qa probe echo read")"
expect "D10a valid prose that still echoes the key" 422 "PermissionDescriptionEchoesKeyNotification" "description"
json   "D10b the echo rule fires ALONE, not beside a shape complaint" \
       '[.errors[].messages[].notificationKey] | sort | join(",")' 'PermissionDescriptionEchoesKeyNotification'

# The value object checks both parts before the pair and neither short-circuits the other:
# a call carrying two malformed halves is told about BOTH in one answer, not one per
# round-trip. That promise is easy to break by refactoring and invisible without this case.
api POST /permissions "$(perm_body "Tenant" "read:all" "$DESC")"
expect "D11a two malformed halves" 422
json   "D11b both are reported in ONE answer" \
       '[.errors[].messages[].notificationKey] | sort | join(",")' \
       'InvalidActionNameNotification,InvalidResourceNameNotification'

# D12 — STRUCTURAL immutability. PatchPermissionRequest declares `description` and nothing
# else, so the pair cannot be sent at all: there is no value to accept, none to assign, and
# nothing in the OpenAPI request schema claiming a caller may edit it. The wire promise is
# therefore "these keys change nothing", not "these keys are refused".
# PermissionKeyIsImmutableNotification guards a door REST cannot open, so no case asserts
# it — stated here rather than silently skipped.
api PATCH "/permissions/${IMM}" \
  "$(jq -n '{resource:"hijacked", action:"write", description:"An attempt to rewrite the pair through patch."}')"
expect "D12a a patch carrying resource and action" 200
json   "D12b the pair is unchanged" '.data.permission' "${PREFIX}imm:read"
json   "D12c the description did change" '.data.description' "An attempt to rewrite the pair through patch."

# ═════════════════════════════════════════════════════════════════════════════
# E — 409, and the ACTIVE-ONLY scope
# ═════════════════════════════════════════════════════════════════════════════
section "E · conflict, and the handle an archived row releases"

# A seeded catalog row. This lane READS the 39 bootstrap entries and collides with two of
# them; it never mutates one.
api POST /permissions "$(perm_body "tenant" "read" "A duplicate of a seeded catalog entry.")"
expect "E1a a pair the catalog already holds" 409 "PermissionAlreadyExistsNotification"
json   "E1b the semantic" \
       '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .semantic][0]' \
       'Conflict'
# The field names the COMPOSITE, not either wire part — and it names it `key`: the value
# object the rule raises on (`AddNotification("Key", …)`) and the same name the repository
# hangs on the unique index. A consumer keying off it must know that `key` stands for the
# resource:action pair, since no request carries a field by that name and no response
# returns one — which is exactly why this case pins it instead of leaving it to drift.
json   "E1c the field the envelope names" \
       '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .field][0]' \
       'key'
# A composite is refused as a TUPLE, and the envelope echoes nothing: the generator
# silences the value on composite uniqueness because it cannot assume the owner formats
# usefully (omnicore-gen, emitUniquePrecheck) — it never emits a String() for a composite
# VO, and the one this entity has is hand-written, which the generator cannot see. So
# `key` names WHAT collided; WHICH pair collided the caller already knows — it just sent
# it. Pinned rather than dropped: the day the envelope starts echoing here by accident,
# this line is what says so.
json   "E1d the tuple itself is not echoed back" \
       '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .value][0]' \
       'null'

# Uniqueness is over the TUPLE, so a second action on the same resource is ordinary.
api POST /permissions "$(perm_body "tenant" "${RUNTOKEN}-verb" "A fresh action on a resource that exists.")"
expect "E2a the tuple is the unit, not the resource" 201
json   "E2b it is stored as its own pair" '.data.permission' "tenant:${RUNTOKEN}-verb"

# `*:*` is a VALID shape — the wildcard is accepted as an entire part, and a wildcard
# resource forces a wildcard action, which this satisfies. It answers 409 because the seed
# already holds it, not because it is malformed. That distinction is the case: a 422 here
# would mean the wildcard accept-path had broken.
api POST /permissions "$(perm_body "*" "*" "The wildcard pair, which the seed already holds.")"
expect "E3 the wildcard pair is well-formed and taken" 409 "PermissionAlreadyExistsNotification"

# THE INVERSE OF TENANT. Tenant's workspace is unique with scope: all — archiving never
# releases the handle. Here the scope is active-only, and with no unarchive verb this is
# the ONLY way a retired permission comes back: as a NEW row with a NEW id.
CYC_R="${PREFIX}cycle"
CYC1=$(create_permission "$CYC_R" "read" "A pair created only to be retired again.")
api PATCH "/permissions/${CYC1}/archive"
expect "E4a archive the pair" 204
api POST /permissions "$(perm_body "$CYC_R" "read" "The retired pair coming back as a new row.")"
expect "E4b the SAME pair inserts again — the archived row released the handle" 201
CYC2=$(jq -r '.data.id' "$BODY")
if [[ "$CYC1" != "$CYC2" && -n "$CYC2" && "$CYC2" != "null" ]]; then
  pass "E4c it came back as a NEW row with a NEW id, never as a restore"
else
  fail "E4c" "a new id, different from ${CYC1}" "id ${CYC2}"
fi

# E5 wrong-state (SemanticStateConflict) — N/A for this entity: no verb here carries a
# revision precondition, and archive/patch misuse resolves as 404 (F4/F5), which is what
# LoadForWrite actually produces.

# ═════════════════════════════════════════════════════════════════════════════
# F — archive: one-way, and what stays visible
# ═════════════════════════════════════════════════════════════════════════════
section "F · the one-way archive"

api GET "/permissions/${CYC1}"
expect "F1 an archived row is hidden from by-id" 404 "RecordNotFoundNotification" "id"

api GET "/permissions/${CYC1}?includeArchived=true"
expect "F2a ?includeArchived reveals it" 200
# A retired permission must stay auditable — with no unarchive verb, this is the only way
# anyone ever sees one again, so the rendering has to survive archiving.
json   "F2b and it still renders its pair" '.data.permission' "${CYC_R}:read"

api GET "/permissions?resource=${CYC_R}"
expect "F3a the listing hides it" 200
json   "F3b only the live row is listed" '.pagination.totalCount' '1'
api GET "/permissions?resource=${CYC_R}&includeArchived=true"
expect "F3c ?includeArchived reveals it in the listing too" 200
json   "F3d both rows, the archived one included" '.pagination.totalCount' '2'

api PATCH "/permissions/${CYC1}/archive"
expect "F4 archiving an already-archived row" 404 "RecordNotFoundNotification"

api PATCH "/permissions/${CYC1}" "$(jq -n '{description:"Trying to edit a retired catalog entry."}')"
expect "F5 an archived entry is not editable" 404 "RecordNotFoundNotification"

# ═════════════════════════════════════════════════════════════════════════════
# G — read vocabulary
# ═════════════════════════════════════════════════════════════════════════════
section "G · read vocabulary"
# Every count filters on $PREFIX. The 39 seeded catalog rows are real data this lane must
# not make assumptions about beyond G17.
SCOPE="resource.startswith=$(urlenc "$PREFIX")"     # everything this lane created
PSCOPE="resource.startswith=$(urlenc "$PAGE")"      # the five untouched paging fixtures

api GET "/permissions?resource=${PREFIX}p1"
expect "G1 eq on a hidden part" 200
json   "G1b one row" '.data | length' '1'

# `ne` is declared here and nowhere in the tenant lane — the operator this entity adds.
api GET "/permissions?${PSCOPE}&resource.ne=${PREFIX}p1"
expect "G2a ne on a hidden part" 200
json   "G2b every paging row but that one" '.pagination.totalCount' '4'

api GET "/permissions?resource.in=$(urlenc "${PREFIX}p1,${PREFIX}p2")"
expect "G3a in" 200
json   "G3b exactly the two named" '.pagination.totalCount' '2'

api GET "/permissions?${PSCOPE}&resource.contains=$(urlenc "p3")"
expect "G4a contains" 200
json   "G4b one row" '.pagination.totalCount' '1'

api GET "/permissions?${PSCOPE}"
expect "G5a startswith is the scope every exact count uses" 200
json   "G5b the five paging fixtures, and only those" '.pagination.totalCount' '5'

api GET "/permissions?${PSCOPE}&action=read"
expect "G6a eq on the other hidden part" 200
json   "G6b p1 and p5 carry it" '.pagination.totalCount' '2'
api GET "/permissions?${PSCOPE}&action.ne=read"
expect "G6c ne" 200
json   "G6d the other three" '.pagination.totalCount' '3'
api GET "/permissions?${PSCOPE}&action.in=$(urlenc "insert,update")"
expect "G6e in" 200
json   "G6f two" '.pagination.totalCount' '2'
api GET "/permissions?${PSCOPE}&action.contains=$(urlenc "rch")"
expect "G6g contains" 200
json   "G6h archive only" '.pagination.totalCount' '1'

api GET "/permissions?${PSCOPE}&description.contains=$(urlenc "paging fixture")"
expect "G7a contains on description" 200
json   "G7b all five, none of them edited by another case" '.pagination.totalCount' '5'

api GET "/permissions?${PSCOPE}&createdAt.gte=$(urlenc "2020-01-01T00:00:00Z")&createdAt.lte=$(urlenc "2999-01-01T00:00:00Z")"
expect "G8a a temporal range that contains them all" 200
json   "G8b all five" '.pagination.totalCount' '5'

api GET "/permissions?${PSCOPE}&updatedAt.gte=$(urlenc "2020-01-01T00:00:00Z")"
expect "G9 updatedAt is filterable though it is not orderable" 200

api GET "/permissions?${PSCOPE}&orderBy=resource&first=100"
expect "G10a orderBy resource asc" 200
json   "G10b ascending" '[.data[].permission] | join("|")' \
  "${PREFIX}p1:read|${PREFIX}p2:insert|${PREFIX}p3:update|${PREFIX}p4:archive|${PREFIX}p5:read"
api GET "/permissions?${PSCOPE}&orderBy=-resource&first=100"
expect "G10c orderBy resource desc" 200
json   "G10d exactly reversed" '[.data[].permission] | join("|")' \
  "${PREFIX}p5:read|${PREFIX}p4:archive|${PREFIX}p3:update|${PREFIX}p2:insert|${PREFIX}p1:read"
api GET "/permissions?${PSCOPE}&orderBy=action&first=100"
expect "G10e orderBy action — a hidden part ordering the page" 200
json   "G10f archive first, update last" \
  '[.data[0].permission, .data[-1].permission] | join("|")' \
  "${PREFIX}p4:archive|${PREFIX}p3:update"
api GET "/permissions?${PSCOPE}&orderBy=description&first=100"
expect "G10g orderBy description" 200

api GET "/permissions?resource=${GOLD_R}&fields=description"
expect "G11a fields=description" 200
json   "G11b only that key comes back" '.data[0] | keys | join(",")' 'description'

# The computed field backs no column, and `?fields=permission` still works: the framework
# pushes its two declared sources down to the store on its own. That is the promise a
# reader would not guess from the response shape alone.
api GET "/permissions?resource=${GOLD_R}&fields=permission"
expect "G12a fields=permission — a computed projection" 200
json   "G12b only the computed key" '.data[0] | keys | join(",")' 'permission'
json   "G12c and it is still derived correctly" '.data[0].permission' "${GOLD_R}:read"

api GET "/permissions?${PSCOPE}&onlyTotal=true"
expect "G13a onlyTotal" 200
json   "G13b the count is served" '.pagination.totalCount' '5'
json   "G13c and no rows are" '.data | length' '0'
absent "G13d" '.pagination' 'hasNextPage' 'in an only-total envelope'

api GET "/permissions?${PSCOPE}&orderBy=resource&last=2"
expect "G14a last alone serves the TAIL window" 200
json   "G14b the last two by that order" '[.data[].permission] | join("|")' \
  "${PREFIX}p4:archive|${PREFIX}p5:read"
json   "G14c there is nothing after the tail" '.pagination.hasNextPage' 'false'
json   "G14d but there is something before it" '.pagination.hasPreviousPage' 'true'

# The pagination envelope as a CONTRACT: cursors are window edges, and walking them has to
# come back to where it started.
api GET "/permissions?${PSCOPE}&orderBy=resource&first=3"
expect "G15a page 1" 200
json   "G15b totalCount is the whole scoped set, not the page" '.pagination.totalCount' '5'
json   "G15c there is a next page" '.pagination.hasNextPage' 'true'
json   "G15d and no previous one" '.pagination.hasPreviousPage' 'false'
jqtrue "G15e the forward edge is issued" '.pagination.endCursor | type == "string"' 'endCursor present'
PAGE1=$(jq -r '[.data[].permission] | join("|")' "$BODY")
END1=$(jq -r '.pagination.endCursor' "$BODY")

api GET "/permissions?${PSCOPE}&orderBy=resource&first=3&after=$(urlenc "$END1")"
expect "G15f page 2 through the forward edge" 200
PAGE2=$(jq -r '[.data[].permission] | join("|")' "$BODY")
json   "G15g now there IS a previous page" '.pagination.hasPreviousPage' 'true'
START2=$(jq -r '.pagination.startCursor' "$BODY")
if [[ -n "$PAGE1" && "$PAGE1" != "$PAGE2" ]]; then
  pass "G15h page 2 is disjoint from page 1"
else
  fail "G15h" "page 2 different from page 1" "both were '${PAGE1}'"
fi

api GET "/permissions?${PSCOPE}&orderBy=resource&last=3&before=$(urlenc "$START2")"
expect "G15i walking backward from page 2" 200
json   "G15j lands exactly on page 1 again" '[.data[].permission] | join("|")' "$PAGE1"

# A DELTA, not an absolute: what the control promises is that the hidden rows come back,
# and stating it as a difference keeps the case true however many live fixtures the earlier
# sections leave behind. Two rows are archived by this point — B5's and E4a's.
api GET "/permissions?${SCOPE}&onlyTotal=true"
LIVE_TOTAL=$(jq -r '.pagination.totalCount' "$BODY")
api GET "/permissions?${SCOPE}&includeArchived=true&onlyTotal=true"
expect "G16a includeArchived raises the scoped total" 200
ALL_TOTAL=$(jq -r '.pagination.totalCount' "$BODY")
if [[ "$ALL_TOTAL" == "$((LIVE_TOTAL + 2))" ]]; then
  pass "G16b by exactly the two rows this lane archived — ${LIVE_TOTAL} live, ${ALL_TOTAL} with archived"
else
  fail "G16b" "$((LIVE_TOTAL + 2)) with archived (${LIVE_TOTAL} live + 2 archived)" "$ALL_TOTAL"
fi
api GET "/permissions?resource=${CYC_R}&includeArchived=true&orderBy=resource"
expect "G16c and the archived row is genuinely among them" 200
jqtrue "G16d the retired row is listed beside the one that replaced it" \
       "[.data[].id] | index(\"${CYC1}\") != null" 'the archived id is present'

# The catalog the service's own RequirePermission calls depend on. Read-only: this lane
# never writes to a seeded row.
api GET "/permissions?resource=permission&action=read"
expect "G17a a seeded catalog entry is readable" 200
json   "G17b exactly one" '.data | length' '1'
json   "G17c rendered as a token carries it" '.data[0].permission' 'permission:read'

# ═════════════════════════════════════════════════════════════════════════════
# H — rejected reads: the whole typed-400 guard family
# ═════════════════════════════════════════════════════════════════════════════
section "H · typed 400s"

api GET "/permissions?bogus=1"
expect "H1 an unknown field" 400 "SchemaViolationNotification" "bogus"

# The composite's OWN name. It is not a filter, not a projection path, and not a response
# key — it exists only inside the domain.
api GET "/permissions?key=x"
expect "H2a the composite's own name is not a filter" 400 "SchemaViolationNotification" "key"
api GET "/permissions?${SCOPE}&fields=key"
expect "H2b nor a projection path" 400 "SchemaViolationNotification" "fields[key]"

# The computed path backs no column, so it can be SELECTED (G12) but never filtered or
# ordered. Filter its two sources instead.
api GET "/permissions?permission=$(urlenc 'tenant:read')"
expect "H3 the computed field is not filterable" 400 "SchemaViolationNotification" "permission"

api GET "/permissions?search=tenant"
expect "H4 a reserved control the DTO never declared" 400 "SchemaViolationNotification" "search"

# Operators outside each field's own allowlist. The asymmetries are deliberate and this is
# where they are pinned: no icontains anywhere (tenant HAS it), startswith on resource but
# not on action, contains-only on description, and no eq on either temporal leaf.
api GET "/permissions?resource.icontains=x"
expect "H5 icontains is declared on no field of this entity" 400 "SchemaViolationNotification" "resource.icontains"
api GET "/permissions?action.startswith=x"
expect "H6 startswith is declared on resource, not on action" 400 "SchemaViolationNotification" "action.startswith"
api GET "/permissions?description.eq=x"
expect "H7 description declares contains and nothing else" 400 "SchemaViolationNotification" "description.eq"
api GET "/permissions?createdAt.contains=x"
expect "H8a a text operator on a temporal leaf" 400 "SchemaViolationNotification" "createdAt.contains"
api GET "/permissions?createdAt=$(urlenc '2026-01-01T00:00:00Z')"
expect "H8b even eq: the temporal leaves declare gte and lte only" 400 "SchemaViolationNotification" "createdAt"

api GET "/permissions?${SCOPE}&fields=bogus"
expect "H9 an unresolvable projection path" 400 "SchemaViolationNotification" "fields[bogus]"

# A hidden part is STORED and filterable, and it is still not a Response field — so it
# cannot be selected. If this ever resolved, the hidden part would be leaking through
# ?fields= and the whole model would be broken.
api GET "/permissions?${SCOPE}&fields=resource"
expect "H10a a hidden part cannot be projected" 400 "SchemaViolationNotification" "fields[resource]"
api GET "/permissions?${SCOPE}&fields=action"
expect "H10b nor the other one" 400 "SchemaViolationNotification" "fields[action]"

api GET "/permissions?orderBy=permission"
expect "H11 the computed path is not an order token either" 400 "SchemaViolationNotification" "orderBy[permission]"
api GET "/permissions?orderBy=createdAt"
expect "H12a createdAt is filterable, never declared orderable" 400 "SchemaViolationNotification" "orderBy[createdAt]"
api GET "/permissions?orderBy=updatedAt"
expect "H12b and neither is updatedAt" 400 "SchemaViolationNotification" "orderBy[updatedAt]"
api GET "/permissions?orderBy=bogus"
expect "H13 an unknown order token" 400 "SchemaViolationNotification" "orderBy[bogus]"
api GET "/permissions?${SCOPE}&orderBy=-description"
expect "H14 positive control — desc IS declared" 200

api GET "/permissions?first=101"
expect "H15 above the page ceiling" 400 "LimitExceededNotification" "first"
api GET "/permissions?first=0"
expect "H16 below the floor" 400 "SchemaViolationNotification" "first"

# The mixed-direction matrix. The backward-side key is the one named: `last` when it is
# present, `before` otherwise — the correction the first round of this suite paid for.
api GET "/permissions?${PSCOPE}&first=2&last=2"
expect "H17a first with last" 400 "SchemaViolationNotification" "last"
api GET "/permissions?${PSCOPE}&first=2&before=$(urlenc "$START2")"
expect "H17b first with before" 400 "SchemaViolationNotification" "before"
api GET "/permissions?${PSCOPE}&last=2&after=$(urlenc "$END1")"
expect "H17c last with after" 400 "SchemaViolationNotification" "last"
api GET "/permissions?${PSCOPE}&after=$(urlenc "$END1")&before=$(urlenc "$START2")"
expect "H17d after with before" 400 "SchemaViolationNotification" "before"

# only-total conflicts: anything that SHAPES a page is a conflict.
api GET "/permissions?${SCOPE}&onlyTotal=true&first=10"
expect "H18a onlyTotal with first" 400 "SchemaViolationNotification" "onlyTotal[first]"
api GET "/permissions?${SCOPE}&onlyTotal=true&orderBy=resource"
expect "H18b onlyTotal with orderBy" 400 "SchemaViolationNotification" "onlyTotal[orderBy]"
api GET "/permissions?${SCOPE}&onlyTotal=true&fields=description"
expect "H18c onlyTotal with fields" 400 "SchemaViolationNotification" "onlyTotal[fields]"
api GET "/permissions?${PSCOPE}&onlyTotal=true&after=$(urlenc "$END1")"
expect "H18d onlyTotal with a cursor" 400 "SchemaViolationNotification" "onlyTotal[after]"

# …but a FILTER is not: counting a filtered subset is the whole point of only-total.
api GET "/permissions?onlyTotal=true&${SCOPE}"
expect "H19a a filter is no conflict" 200
api GET "/permissions?${SCOPE}&onlyTotal=true&includeArchived=true"
expect "H19b nor is includeArchived" 200

api GET "/permissions?after=not-a-cursor"
expect "H20 a malformed cursor" 400 "SchemaViolationNotification" "after"

# The two cursor checks are DIFFERENT layers and name different fields. The structural one
# runs in the REST wrapper before dispatch; the context-hash one runs inside the reader.
api GET "/permissions?${PSCOPE}&first=2"
CUR_NOORDER=$(jq -r '.pagination.endCursor' "$BODY")
api GET "/permissions?${PSCOPE}&first=2&orderBy=resource&after=$(urlenc "$CUR_NOORDER")"
expect "H21 a cursor replayed under a different orderBy" 400 "SchemaViolationNotification" "after"
api GET "/permissions?${PSCOPE}&orderBy=resource&first=3&after=$(urlenc "$END1")&includeArchived=true"
expect "H22 a cursor replayed with the archive context flipped" 400 "SchemaViolationNotification" "cursor"

api GET "/permissions?includeArchived=1"
expect "H23a a boolean takes exactly true or false" 400 "SchemaViolationNotification" "includeArchived"
api GET "/permissions?onlyTotal="
expect "H23b an empty boolean is still a presence" 400 "SchemaViolationNotification" "onlyTotal"

# The by-id DTO declares includeArchived and NOTHING else. Presence is what trips the gate.
api GET "/permissions/${GOLD}?fields=description"
expect "H24a fields is not declared on the by-id endpoint" 400 "SchemaViolationNotification" "fields"
api GET "/permissions/${GOLD}?onlyTotal=false"
expect "H24b nor onlyTotal — even set to false" 400 "SchemaViolationNotification" "onlyTotal"
api GET "/permissions/${GOLD}?includeArchived=true"
expect "H25 positive control — the one control it does declare" 200

# UnsupportedCapabilityNotification is N/A for this entity: it is raised when a read engine
# is asked for something the store cannot serve. Permission is FLAT (no 1:N leg to push
# down) and `search` is undeclared, so the DTO gate answers first with a
# SchemaViolationNotification (H4). Asserting the other key would assert a promise this
# entity does not make.

# ═════════════════════════════════════════════════════════════════════════════
# I — routing, and the verb that does not exist
# ═════════════════════════════════════════════════════════════════════════════
section "I · routing"

api GET "/permissions/2fd0f4e6-0000-4000-8000-000000000000"
expect "I1 a well-formed id naming no row" 404 "RecordNotFoundNotification" "id"

api DELETE "/permissions/${P2}"
expect "I2 no hard delete exists" 405 "MethodNotAllowedNotification"
api POST "/permissions/${P2}" '{}'
expect "I3 the path is registered, the method is not" 405 "MethodNotAllowedNotification"
api GET "/permissions/${P2}/archive"
expect "I4 archive is mounted under PATCH only" 405 "MethodNotAllowedNotification"
api GET "/permissions/${P2}/purge"
expect "I5 a path matching no registered route" 404 "RouteNotFoundNotification"

# THE design decision, made observable. Un-archiving would re-enable, in one call, every
# grant still pointing at the row — old holders would silently regain a permission nobody
# re-approved, and the audit trail would read as a restore rather than as a grant. So the
# mode is not declared, no route is mounted, and the path matches nothing.
api PATCH "/permissions/${CYC1}/unarchive"
expect "I6 unarchive does not exist — by design, not by omission" 404 "RouteNotFoundNotification"

gql "mutation { unarchivePermission(id: \"${CYC1}\") { success } }"
expect "I7a the GraphQL schema does not advertise it either" 200
jqtrue "I7b it is an unknown field on Mutation" \
  '(.errors | length) > 0 and ([.errors[].message] | join(" ") | test("Cannot query field .unarchivePermission."))' \
  'unknown mutation field'

# The 403 arm of the three-way split (a mode missing from Modes() whose route is still
# mounted) is unreachable here: every declared mode has its route and no route exists for
# an undeclared mode. The 403 that IS reachable is the permission gate — case A2.

# ═════════════════════════════════════════════════════════════════════════════
# K — the by-id address, split by VERB (pin >= v0.70.0)
# ═════════════════════════════════════════════════════════════════════════════
section "K · a by-id address that is not a uuid"
# Split by VERB, not by surface: a READ named no record (404), a WRITE violated the request
# shape (400). Before v0.70.0 both were a 500 on this backing.

api GET "/permissions/not-a-uuid"
expect "K1a a read answers not-found" 404 "UnknownIDAddressNotification" "id"
json   "K1b the segment is echoed" '[.errors[].messages[].value][0]' 'not-a-uuid'
json   "K1c an address problem is not labelled a payload problem" '[.errors[].context][0]' 'Request'

api PATCH "/permissions/not-a-uuid" "$(jq -n '{description:"A body that never gets looked at."}')"
expect "K2a a write answers malformed" 400 "MalformedIDNotification" "id"
json   "K2b same echo" '[.errors[].messages[].value][0]' 'not-a-uuid'
json   "K2c same context" '[.errors[].context][0]' 'Request'

api PATCH "/permissions/not-a-uuid/archive"
expect "K3 the bodyless by-id command follows the write rule" 400 "MalformedIDNotification" "id"

gql "{ permission(id: \"not-a-uuid\") { id } }"
expect "K5a GraphQL read" 200
json   "K5b typed, and it is the READ key" '[.errors[].extensions.notificationKey][0]' 'UnknownIDAddressNotification'
json   "K5c semantic" '[.errors[].extensions.semantic][0]' 'NotFound'

gql "mutation { archivePermission(id: \"not-a-uuid\") { success } }"
expect "K6a GraphQL write" 200
json   "K6b typed, and it is the WRITE key" '[.errors[].extensions.notificationKey][0]' 'MalformedIDNotification'
json   "K6c semantic" '[.errors[].extensions.semantic][0]' 'Schema'

# ═════════════════════════════════════════════════════════════════════════════
# L — a filter VALUE the leaf cannot take (pin >= v0.70.0, FIXED at v0.71.0)
# ═════════════════════════════════════════════════════════════════════════════
section "L · filter values outside the leaf's declared kind"
# Section H proves many flavors of typed 400, but every one of them violates the query
# GRAMMAR. These send a well-formed key carrying a value the COLUMN cannot hold.
#
# The tenant lane wrote these against v0.70.0 and they stood deliberately RED: the promise
# was made, but a *time.Time leaf collapsed to reflect.Kind=struct and the raw string
# reached the driver as an external 500. v0.71.0 consults the DECLARED TYPE before the
# kind and parses a temporal leaf as RFC3339. These are the cases proving it.
api GET "/permissions?createdAt.gte=$(urlenc 'not-a-date')"
expect "L1a a range operator on a temporal leaf" 400 "InvalidFilterValueNotification" "createdAt.gte"
json   "L1b the rejected value is echoed back" '[.errors[].messages[].value][0]' 'not-a-date'
json   "L1c and it is a Schema refusal, not a server fault" '[.errors[].messages[].semantic][0]' 'Schema'

api GET "/permissions?updatedAt.lte=$(urlenc '13/45/2026')"
expect "L2 a plausible-looking date that is not RFC3339" 400 "InvalidFilterValueNotification" "updatedAt.lte"

# The `eq` half of the temporal contract cannot be proven from this entity: neither
# temporal leaf declares eq (H8b pins that), so the grammar gate answers first. The tenant
# lane owns that case — its createdAt declares eq — and it runs in the same suite.

# ═════════════════════════════════════════════════════════════════════════════
# J — GraphQL handler invariance
# ═════════════════════════════════════════════════════════════════════════════
section "J · GraphQL"

gql "{ permissions(where: {resource: {startswith: \"${PAGE}\"}}, orderBy: [{field: RESOURCE, direction: ASC}], first: 2) { totalCount edges { cursor node { id permission description } } pageInfo { hasNextPage hasPreviousPage endCursor } } }"
expect "J1a the connection" 200
json   "J1b totalCount matches its REST twin" '.data.permissions.totalCount' '5'
json   "J1c the same first page, in the same order" \
       '[.data.permissions.edges[].node.permission] | join("|")' "${PREFIX}p1:read|${PREFIX}p2:insert"
json   "J1d and it knows there is more" '.data.permissions.pageInfo.hasNextPage' 'true'

gql "{ permission(id: \"${P3}\") { id permission description } }"
expect "J2a the singular twin" 200
json   "J2b equals the REST by-id document" '.data.permission.permission' "${PREFIX}p3:update"

gql "mutation { createPermission(input: {resource: \"${PREFIX}gq\", action: \"read\", description: \"Written over GraphQL and read over REST.\"}) { id permission } }"
expect "J3a createPermission" 200
json   "J3b the pair it rendered" '.data.createPermission.permission' "${PREFIX}gq:read"
GQID=$(jq -r '.data.createPermission.id' "$BODY")
api GET "/permissions/${GQID}"
expect "J3c one write, two surfaces — REST sees it immediately" 200
json   "J3d same record" '.data.permission' "${PREFIX}gq:read"

gql "mutation { patchPermission(id: \"${GQID}\", input: {description: \"Edited over GraphQL, verified over REST.\"}) { id description } }"
expect "J4a patchPermission" 200
api GET "/permissions/${GQID}"
json   "J4b REST confirms the edit" '.data.description' "Edited over GraphQL, verified over REST."

gql "mutation { archivePermission(id: \"${GQID}\") { success id } }"
expect "J5a archivePermission" 200
json   "J5b the payload reports success" '.data.archivePermission.success' 'true'
api GET "/permissions/${GQID}"
expect "J5c REST confirms it is hidden" 404 "RecordNotFoundNotification"

gql "{ permission(id: \"2fd0f4e6-0000-4000-8000-000000000000\") { id } }"
expect "J6a a well-formed id naming no row" 200
json   "J6b the GraphQL idiom carries the typed identity" \
       '[.errors[].extensions.notificationKey][0]' 'RecordNotFoundNotification'
json   "J6c and its semantic" '[.errors[].extensions.semantic][0]' 'NotFound'

gql "mutation { createPermission(input: {resource: \"tenant\", action: \"read\", description: \"A duplicate of a seeded entry again.\"}) { id } }"
expect "J7a a duplicate pair over GraphQL" 200
json   "J7b the same conflict key REST raises" \
       '[.errors[].extensions.notificationKey][0]' 'PermissionAlreadyExistsNotification'
json   "J7c the same semantic" '[.errors[].extensions.semantic][0]' 'Conflict'

api_as "" POST /graphql "$(jq -n '{query:"{ permissions(first: 1) { totalCount } }"}')"
expect "J8 the GraphQL route is not public" 401 "MissingAuthorizationNotification"

# The DTO cuts the SCHEMA here: `search` is not an argument the endpoint advertises, so the
# refusal is a gqlparser validation error, not the REST 400 envelope. Asserting the REST
# shape cross-surface would be asserting the wrong contract.
gql "{ permissions(search: \"tenant\", first: 1) { totalCount } }"
expect "J9a an undeclared control is an UNKNOWN ARGUMENT here" 200
jqtrue "J9b the query is refused, not served" '(.errors | length) > 0 and (.data.permissions == null)' 'errors present, no data'

gql "{ permissions(where: {resource: {startswith: \"${PAGE}\"}}, orderBy: [{field: RESOURCE, direction: DESC}], first: 3) { edges { node { permission } } } }"
expect "J10a typed where + orderBy" 200
json   "J10b the same order its REST twin returns" \
       '[.data.permissions.edges[].node.permission] | join("|")' \
       "${PREFIX}p5:read|${PREFIX}p4:archive|${PREFIX}p3:update"

# The hidden parts are filterable HERE too while absent from the node — the same split as
# REST, proven on the surface where the schema makes it visible.
gql "{ permissions(where: {action: {eq: \"insert\"}, resource: {startswith: \"${PREFIX}\"}}, first: 5) { totalCount edges { node { permission } } } }"
expect "J10c a hidden part as a GraphQL filter" 200
json   "J10d selects by a value it will never return" \
       '[.data.permissions.edges[].node.permission] | join("|")' "${PREFIX}p2:insert"

# gRPC — N/A: no transport is wired. Exports — N/A: none declared.

# ═════════════════════════════════════════════════════════════════════════════
section "summary"
printf '  %sGREEN %d%s · %sRED %d%s · %sSKIPPED %d%s\n' "$G" "$PASS" "$Z" "$R" "$FAIL" "$Z" "$Y" "$SKIP" "$Z"
[[ -n "$QA_LOG_DIR" ]] && printf '%d %d %d\n' "$PASS" "$FAIL" "$SKIP" > "${QA_LOG_DIR}/${LANE_NAME}.counts"
printf '  residue: the throwaway database %s, recreated on the next run. authcore_db untouched.\n' "$QA_DB"
printf '  the 39 seeded catalog rows were read and collided with, never written to.\n'
[[ $FAIL -eq 0 ]] || printf '  server log: %s (kept until this script exits)\n' "$LOG"
exit $(( FAIL > 0 ? 1 : 0 ))
