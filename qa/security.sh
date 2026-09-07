#!/usr/bin/env bash
# Lane: security — §3 of specs/qa/tenant-contract/plan.md.
#
# Every other lane proves the service DOES what it should. This one proves it REFUSES what it
# should. 401 and 403 are the two answers a caller must be able to rely on, and they are the
# ones a regression turns into a 200 silently: a publicRoutes entry widened past what it meant
# to open, a RequirePermission lost when a route was re-mounted, a tenant gate that stopped
# firing. None of those breaks a single happy-path case.
#
# WHERE THE TOKENS COME FROM — nothing here is invented:
#   · VALID tokens are minted by THE SERVICE ITSELF through POST /auth/user/token (auth.issuer
#     is enabled; this service is its own IdP). Principal A is the seeded bootstrap admin,
#     principal B a limited user the suite creates through the API. Both arrive from run.sh.
#   · DELIBERATELY INVALID tokens are forged here. Two sources, and the distinction matters:
#       - a throwaway keypair the suite generates, for the "foreign signature" probe;
#       - the bench's OWN dev signing key, used ONLY to vary one claim at a time (wrong iss,
#         missing aud, expired). Using the real key is what isolates the CLAIM check from the
#         SIGNATURE check — a foreign-signed token with a wrong iss would prove nothing about
#         iss. It is never used to mint a token that should be accepted.
#   · The tenant-gate token is signed by the suite and validated against a config file the
#     suite owns (qa/microservice.qa-key.yaml) — the suite IS the issuer there, and says so.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init security

TMP="$QA_RUN_DIR/sec"
mkdir -p "$TMP"

ISS="${AUTH_SELF_URL:-http://localhost:8099}"
AUD="${AUTH_AUDIENCE:-authcore}"
NOW=$(now_epoch)

# A throwaway keypair, for the foreign-signature probe and for the tenant-gate boot.
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$TMP/suite.pem" 2>/dev/null
openssl rsa -in "$TMP/suite.pem" -pubout -out "$TMP/suite.pub.pem" 2>/dev/null

HDR_RS=$(jq -nc --arg kid "${JWT_SIGNING_KID:-dev}" '{alg:"RS256", typ:"JWT", kid:$kid}')
HDR_RS_NOKID='{"alg":"RS256","typ":"JWT"}'
HDR_HS='{"alg":"HS256","typ":"JWT"}'

claims() { # claims ISS AUD EXP [TENANT]
  jq -nc --arg iss "$1" --arg aud "$2" --argjson exp "$3" --arg tid "${4:-}" \
    '{sub:"qa-forged-subject", iss:$iss, aud:$aud, iat:'"$NOW"', exp:$exp}
     + (if $tid == "" then {} else {tenant_id:$tid} end)'
}

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3a — the 401 half. GET /tenants is the protected route under test throughout.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S3a.1 no Authorization header at all" "401 MissingAuthorizationNotification"
api_raw GET /tenants ""
assert_rest 401 MissingAuthorizationNotification

case_ "S3a.2 a header that is not Bearer" "401 MissingAuthorizationNotification — the extraction failed, the token was never looked at"
api_raw GET /tenants "Basic YWRtaW46YWRtaW4="
assert_rest 401 MissingAuthorizationNotification

case_ "S3a.3 Bearer with an empty value" "401 MissingAuthorizationNotification"
api_raw GET /tenants "Bearer"
assert_rest 401 MissingAuthorizationNotification

case_ "S3a.4 a LOWERCASE bearer scheme is accepted" "200 — the scheme match is case-insensitive, so this is a PASS case and not a reject"
api_raw GET /tenants "bearer ${QA_TOKEN_ADMIN}"
assert_status 200

case_ "S3a.5 a value that is not a JWT at all" "401 InvalidTokenNotification — a different branch from 'no token', and a client is entitled to tell them apart"
api_raw GET /tenants "Bearer not-a-jwt"
assert_rest 401 InvalidTokenNotification

case_ "S3a.6 a well-formed JWT signed with a FOREIGN key" "401 InvalidTokenNotification"
FOREIGN=$(jwt_rs256 "$TMP/suite.pem" "$HDR_RS_NOKID" "$(claims "$ISS" "$AUD" $((NOW + 600)) "01990000-0001-7000-8000-000000000001")")
api_raw GET /tenants "Bearer $FOREIGN"
assert_rest 401 InvalidTokenNotification

case_ "S3a.7 the right key, the WRONG issuer" "401 InvalidTokenNotification — iss must match auth.jwt.issuer"
BADISS=$(jwt_rs256 "$QA_SIGNING_KEY_FILE" "$HDR_RS" "$(claims "https://evil.example" "$AUD" $((NOW + 600)) "01990000-0001-7000-8000-000000000001")")
api_raw GET /tenants "Bearer $BADISS"
assert_rest 401 InvalidTokenNotification

case_ "S3a.8 the right key, an audience that omits ours" "401 InvalidTokenNotification — aud must contain auth.jwt.audience"
BADAUD=$(jwt_rs256 "$QA_SIGNING_KEY_FILE" "$HDR_RS" "$(claims "$ISS" "someone-elses-api" $((NOW + 600)) "01990000-0001-7000-8000-000000000001")")
api_raw GET /tenants "Bearer $BADAUD"
assert_rest 401 InvalidTokenNotification

case_ "S3a.9 HS256 where the allowlist is [RS256]" "401 InvalidTokenNotification — the algorithm-confusion guard, and the classic attack it closes"
HS=$(jwt_hs256 "whatever-secret" "$HDR_HS" "$(claims "$ISS" "$AUD" $((NOW + 600)) "01990000-0001-7000-8000-000000000001")")
api_raw GET /tenants "Bearer $HS"
assert_rest 401 InvalidTokenNotification

case_ "S3a.10 an exp in the past" "401 ExpiredTokenNotification — a DISTINCT key from InvalidToken, because a client branches refresh-vs-reauthenticate on it"
EXPIRED=$(jwt_rs256 "$QA_SIGNING_KEY_FILE" "$HDR_RS" "$(claims "$ISS" "$AUD" $((NOW - 3600)) "01990000-0001-7000-8000-000000000001")")
api_raw GET /tenants "Bearer $EXPIRED"
assert_rest 401 ExpiredTokenNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3b — the public-route split, BOTH directions
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S3b.1 GET /livez tokenless" "200 — the probes are public HERE only because this project opted them in; they are not framework-public"
api_raw GET /livez ""
assert_status 200

case_ "S3b.2 GET /readyz tokenless" "200 — same opt-in"
api_raw GET /readyz ""
assert_status 200

case_ "S3b.3 POST /auth/user/token tokenless" "200 — a login cannot require the credential it is about to grant"
api POST /auth/user/token "$(jq -nc --arg e "$QA_ADMIN_EMAIL" --arg p "$QA_ADMIN_PASSWORD" '{email:$e, password:$p}')" "-"
assert_json_at 200 '(.data.accessToken | length) > 0' "true"

case_ "S3b.4 THE OTHER DIRECTION: a route that is NOT declared public" "401 MissingAuthorizationNotification — this is the case that catches a publicRoutes entry widened past its intent"
api_raw GET /tenants ""
assert_rest 401 MissingAuthorizationNotification

case_ "S3b.5 the same path under another METHOD is not public" "NOT 200 — matching is exact METHOD /path, never prefix-based"
api_raw GET /auth/user/token ""
if [ "$HTTP_STATUS" != "200" ]; then pass_; else fail_ "HTTP 200 — a public POST leaked its path to GET"; fi

case_ "S3b.6 a sibling path sharing a prefix with a public entry is not public" "NOT 200 — /auth/user/token/refresh is public, /auth/user/token/refresh/extra is not a route at all"
api_raw GET /auth/user/token/refresh/extra ""
if [ "$HTTP_STATUS" != "200" ]; then pass_; else fail_ "HTTP 200 — a prefix match leaked"; fi

case_ "S3b.7 GET /openapi.json tokenless" "200 — appended by the framework at boot, needing no publicRoutes entry"
api_raw GET /openapi.json ""
assert_status 200

case_ "S3b.8 the /docs UI tokenless" "200 — an operator reaches the docs before holding a token"
api_raw GET /docs ""
assert_status 200

case_ "S3b.9 GET / tokenless" "a redirect or a page — openapi.rootRedirect is on, so the framework appends the root too"
api_raw GET / ""
case "$HTTP_STATUS" in 200|301|302|307|308) pass_ ;; *) fail_ "HTTP $HTTP_STATUS" ;; esac

case_ "S3b.10 the JWKS document tokenless" "200 — a key set that needed a bearer to fetch is a contradiction: nothing could bootstrap trust in it"
api_raw GET /.well-known/jwks.json ""
assert_json_at 200 '(.keys | length) > 0' "true"

case_ "S3b.11 the GraphQL playground tokenless" "200 — the page opens; what it can RENDER is the introspection question below"
api_raw GET /graphql/ui ""
assert_status 200

case_ "S3b.12 an introspection-ONLY document tokenless" "200 with a schema — the same disclosure /openapi.json already makes, and no more"
gql 'query { __schema { queryType { name } } }' '{}' "-"
assert_gql_ok '(.data.__schema.queryType.name | length) > 0' "true"

case_ "S3b.13 a DATA field beside __schema, tokenless" "401 — the grant is decided per request, and this document can reach data"
gql 'query { __schema { queryType { name } } tenants { totalCount } }' '{}' "-"
if [ "$HTTP_STATUS" = "401" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

case_ "S3b.14 a DECOY introspection operation beside a real one, tokenless" "401 — every operation in the document must be introspection-only, not just the one named"
gql 'query Peek { __schema { queryType { name } } } query Real { tenants { totalCount } }' '{}' "-"
if [ "$HTTP_STATUS" = "401" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

case_ "S3b.15 a ROOT FRAGMENT SPREAD, tokenless" "401 — anything the rule cannot PROVE is introspection-only stays behind the bearer"
gql 'query { ...Root } fragment Root on Query { __schema { queryType { name } } }' '{}' "-"
if [ "$HTTP_STATUS" = "401" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

case_ "S3b.16 a MUTATION, tokenless" "401 — a mutation is never introspection"
gql 'mutation { archiveTenant(id: "019903c2-6b41-7c9e-9f2a-6d3b1e77aaaa") { success } }' '{}' "-"
if [ "$HTTP_STATUS" = "401" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3c layer 1 — RequirePermission. Principal B holds tenant:read and nothing else.
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_SEC=$(ws sec)
api POST /tenants "$(tenant_body "Security Lane Tenant" "$WS_SEC" "The tenant the permission gate cases read and try to write." "active")"
ID_SEC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "S3c.1 an authenticated principal WITHOUT tenant:insert" "403 MissingPermissionNotification"
api POST /tenants "$(tenant_body "Forbidden Insert" "$(ws sec)" "A tenant a principal without the insert permission tries to create." "active")" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S3c.2 the same principal on PATCH" "403 MissingPermissionNotification — tenant:update is a separate grant"
api PATCH "/tenants/$ID_SEC" '{"name":"Forbidden Rename"}' "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S3c.3 the same principal on archive" "403 MissingPermissionNotification"
api PATCH "/tenants/$ID_SEC/archive" "" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S3c.4 the same principal on unarchive" "403 — archive and unarchive share tenant:archive, so both must refuse"
api PATCH "/tenants/$ID_SEC/unarchive" "" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S3c.5 THE COMPLEMENT: the same principal on the LISTING it does hold" "200 — a gate that refuses everyone is also broken"
api GET "/tenants" "" "$QA_TOKEN_LIMITED"
assert_status 200

case_ "S3c.6 and on the by-id read" "200"
api GET "/tenants/$ID_SEC" "" "$QA_TOKEN_LIMITED"
assert_json_at 200 '.data.workspace' "$WS_SEC"

case_ "S3c.7 the privileged principal writes" "201 — the wildcard grant satisfies every check"
api POST /tenants "$(tenant_body "Permitted Insert" "$(ws sec)" "A tenant the privileged principal is allowed to create." "active")"
assert_status 201

case_ "S3c.8 the gate holds on GRAPHQL too" "MissingPermissionNotification in errors[].extensions — a route gated on REST is not thereby gated here"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id } }" \
    "$(jq -nc --arg w "$(ws sec)" '{i:{name:"GraphQL Forbidden", workspace:$w, description:"A tenant a principal without the permission tries to create here.", status:"active"}}')" \
    "$QA_TOKEN_LIMITED"
assert_gql MissingPermissionNotification

case_ "S3c.9 the extensions name the field 'permission'" "field 'permission' — the same notification the REST envelope reports, same shape"
assert_json '[.errors[]?.extensions? | select(.notificationKey=="MissingPermissionNotification") | .field] | first' "permission"

case_ "S3c.10 and the GraphQL READ it does hold still answers" "a connection, not a refusal"
gql 'query { tenants(first: 1) { totalCount edges { node { id } } } }' '{}' "$QA_TOKEN_LIMITED"
assert_gql_ok '(.data.tenants.totalCount >= 1)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3c layer 2 — identity-derived rules. N/A on this aggregate, BY DECISION.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S3c.11 an identity-derived row or field rule on Tenant" "there is none — and the silence is a decision, not a gap"
skip_ "authz.dataAccess is 'anyone-with-permission' (spec.md §B Q4: anyone holding the permission sees and edits every row). FindTenantsByParamsQuery.ToCriteria returns the criteria unchanged, BuildRules reads no principal field and no Restrict is declared, so there is no per-row or per-field boundary on this aggregate to assert"

case_ "S3c.12 cross-tenant isolation on Tenant" "there is none to leak — same decision"
skip_ "Tenant is the platform registry every other aggregate is scoped BY; it carries no tenant scope of its own, so a cross-tenant read or write is not a boundary this entity has. The isolation cases belong to the scoped aggregates (user, role, group, client), which are out of this round"

# ═════════════════════════════════════════════════════════════════════════════════════════
# 3c layer 3 — the tenant gate. Option (a), approved 2026-09-06: a second, short boot whose
# auth.jwt validates through a keypair the SUITE owns, so the suite can present a token that
# carries no tenant claim. No token this service mints can lack one.
# ═════════════════════════════════════════════════════════════════════════════════════════

KEY_BASE="http://localhost:8098"
KEY_LOG="$QA_LOG_DIR/server-key.log"

QA_SUITE_PUBLIC_KEY="$(awk '{printf "%s\\n", $0}' "$TMP/suite.pub.pem")"
export QA_SUITE_PUBLIC_KEY AUTH_KEY_SELF_URL="$KEY_BASE"

# Free the port before binding it: something already listening there means the next cases
# would be testing a binary this suite did not build.
if lsof -ti tcp:8098 >/dev/null 2>&1; then
  kill -TERM "$(lsof -ti tcp:8098)" 2>/dev/null
  for _ in $(seq 1 20); do lsof -ti tcp:8098 >/dev/null 2>&1 || break; sleep 0.5; done
fi

APP_PROFILE=qa OMNICORE_CONFIG_PATH=qa/microservice.qa-key.yaml \
  "$QA_BIN" > "$KEY_LOG" 2>&1 &
KEY_PID=$!

KEY_UP=0
for _ in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w '%{http_code}' "$KEY_BASE/livez" 2>/dev/null)
  if [ "$code" = "200" ]; then KEY_UP=1; break; fi
  kill -0 "$KEY_PID" 2>/dev/null || break
  sleep 0.5
done

if [ "$KEY_UP" != "1" ]; then
  case_ "S3c.13 the tenant gate refuses a token with no tenant claim" "403 TenantMissingNotification"
  skip_ "the suite-keypair boot on :8098 did not become ready — see $KEY_LOG. This is a BENCH problem, not a service verdict, and the gate stays UNPROVEN for this run"
  case_ "S3c.14 the same token WITH a tenant claim is admitted past the gate" "not a 403 TenantMissingNotification"
  skip_ "same boot failure — see $KEY_LOG"
else
  KEY_ISS="$KEY_BASE"
  NOTENANT=$(jwt_rs256 "$TMP/suite.pem" "$HDR_RS_NOKID" "$(claims "$KEY_ISS" "$AUD" $((NOW + 600)))")
  WITHTENANT=$(jwt_rs256 "$TMP/suite.pem" "$HDR_RS_NOKID" "$(claims "$KEY_ISS" "$AUD" $((NOW + 600)) "01990000-0001-7000-8000-000000000001")")

  case_ "S3c.13 a valid token carrying NO tenant claim" "403 TenantMissingNotification — the only non-401 outcome the middleware itself produces, raised before any handler runs"
  api_at "$KEY_BASE" GET /tenants "" "$NOTENANT"
  assert_rest 403 TenantMissingNotification

  case_ "S3c.14 THE COMPLEMENT: the same token WITH a tenant claim clears the gate" "anything but TenantMissingNotification — the request reaches the permission gate, which is a different refusal"
  api_at "$KEY_BASE" GET /tenants "" "$WITHTENANT"
  keys=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey] | join(",")' 2>/dev/null)
  if printf '%s' ",$keys," | grep -q ",TenantMissingNotification,"; then
    fail_ "HTTP $HTTP_STATUS, keys [$keys] — the gate refuses a token that DOES carry the claim"
  else
    pass_
  fi

  case_ "S3c.15 and past the tenant gate it is the PERMISSION gate that answers" "403 MissingPermissionNotification — the suite's token carries no permissions claim, which is the next layer down"
  assert_rest 403 MissingPermissionNotification
fi

# SIGTERM, never kill -9, and WAIT for the drain: the next thing that binds this port must not
# race a socket that is still closing.
if kill -0 "$KEY_PID" 2>/dev/null; then
  kill -TERM "$KEY_PID" 2>/dev/null
  for _ in $(seq 1 70); do kill -0 "$KEY_PID" 2>/dev/null || break; sleep 0.5; done
fi
wait "$KEY_PID" 2>/dev/null

qa_finish
