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

# The caller's half of the row scope, read from the token the suite already holds. Learning it
# from GET /tenants instead would ask the endpoint under test to certify its own answer.
OWN_TEN=$(jwt_claim "$QA_TOKEN_LIMITED" tenant_id)
[ -n "$OWN_TEN" ] || { echo "security.sh: principal B's token carries no tenant_id claim" >&2; exit 1; }

case_ "S3c.6 and on the by-id read of ITS OWN tenant" "200 — the row scope narrows what is reached, it does not refuse; the permission it holds still answers"
api GET "/tenants/$OWN_TEN" "" "$QA_TOKEN_LIMITED"
assert_json_at 200 '.data.id' "$OWN_TEN"

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
# 3c layer 2 — the ROW SCOPE on Tenant. Live since 2026-09-07. These two cases USED TO BE
# SKIPS asserting the opposite ("there is none to leak — the silence is a decision"); the
# contract they described changed, so they became executed cases rather than being deleted.
#
# The registry's rows ARE the partitions, so the scope hangs off the aggregate's own identity:
# authz.scopes: [{field: ID, from: tenant, applies: [read]}] forces Filter["ID"] = TenantID()
# into both reads, under an IsSuperAdmin() bypass. Before it, tenant:read — the one of the four
# tenant permissions a customer legitimately receives — answered with every other customer's
# name, workspace, description and commercial status.
#
# READ ONLY, by decision (maintainer, 2026-09-07). PATCH/archive/unarchive are NOT row-scoped:
# they are contained by the distribution of tenant:update / tenant:archive, which no principal
# inside a tenant holds. S3c.2–S3c.4 above already prove principal B is refused on all three —
# at layer 1, which is where that containment actually lives. ACCESS_MATRIX.md carries the
# decision and its date, so the absence here reads as a choice and not as a missing case.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S3c.11 the LISTING narrows to the caller's own tenant" "exactly 1 row, and it is $OWN_TEN — not the whole registry"
api GET "/tenants?first=100" "" "$QA_TOKEN_LIMITED"
assert_json '[(.data // [] | length), ([.data[]?.id] | unique | join(","))] | join("|")' "1|$OWN_TEN"

case_ "S3c.12 THE LEAK ITSELF: a by-id read of ANOTHER tenant" "404 RecordNotFoundNotification — not 403: the row does not exist for this caller, which leaks nothing about who else exists"
api GET "/tenants/$ID_SEC" "" "$QA_TOKEN_LIMITED"
assert_rest 404 RecordNotFoundNotification

case_ "S3c.12b THE COMPLEMENT: principal A (*:*) crosses the same scope" "200 on the very id principal B was refused — a scope that refuses everyone is also broken"
api GET "/tenants/$ID_SEC" ""
assert_json_at 200 '.data.workspace' "$WS_SEC"

case_ "S3c.12c and its listing still spans the registry" "more than 1 row — the bypass is on the LISTING too, not only the by-id read"
api GET "/tenants?first=100" ""
assert_json '(.data // [] | length) > 1' "true"

case_ "S3c.12d the scope holds on GRAPHQL too" "1 edge, the caller's own — ToCriteria is shared, but a surface that skipped it would look exactly like a passing REST case"
gql 'query { tenants(first: 100) { edges { node { id } } } }' '{}' "$QA_TOKEN_LIMITED"
assert_gql_ok '[(.data.tenants.edges | length), ([.data.tenants.edges[].node.id] | unique | join(","))] | join("|")' "1|$OWN_TEN"

# ═════════════════════════════════════════════════════════════════════════════════════════
#
#   P E R M I S S I O N  —  §3 of specs/qa/permission-contract/plan.md
#
#   Cases S4.x. The 401 family, the framework's appended public surfaces and the introspection
#   bypass are INHERITED from S3a/S3b above and deliberately not repeated — they are properties
#   of the middleware, not of an entity. What is per-entity is everything below: a route this
#   round added must answer 401 tokenless, and its permission gate must discriminate.
#
#   THREE PRINCIPALS, and each one exists because the other two cannot see what it sees:
#     A  *:*                      — the super-admin. Proves the gate is not shut for everyone.
#     B  tenant:read only         — proves the gate refuses a caller who holds SOMETHING else.
#     C  permission:read only     — the sharp one. A and B both answer the same on all five
#                                   permission routes, so neither can see the failure where all
#                                   five were gated on the SAME literal. C separates them.
#     D  permission:read, in a
#        SECOND tenant            — proves the catalog is GLOBAL: the design decision, not a
#                                   mechanism. If Permission is ever scoped, S4.5 goes RED first.
#
# ═════════════════════════════════════════════════════════════════════════════════════════

# ── S4.1 the public-route split, direction 2, for the routes this round added ─────────────
#
# publicRoutes names no /permissions path. One tokenless call per mounted route is what
# catches an entry widened past its intent — and it has to be per ROUTE, because the list is
# matched as exact METHOD /path with no prefix rule.

case_ "S4.1a GET /permissions tokenless" "401 MissingAuthorizationNotification"
api GET "/permissions" "" "-"
assert_rest 401 MissingAuthorizationNotification

case_ "S4.1b GET /permissions/{id} tokenless" "401"
api GET "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410" "" "-"
assert_rest 401 MissingAuthorizationNotification

case_ "S4.1c POST /permissions tokenless" "401 — refused before the body is ever validated"
api POST "/permissions" '{"resource":"tenant","action":"read","description":"A tokenless attempt to write into the platform permission catalog."}' "-"
assert_rest 401 MissingAuthorizationNotification

case_ "S4.1d PATCH /permissions/{id} tokenless" "401"
api PATCH "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410" '{"description":"A tokenless attempt to reword a catalog entry."}' "-"
assert_rest 401 MissingAuthorizationNotification

case_ "S4.1e PATCH /permissions/{id}/archive tokenless" "401"
api PATCH "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410/archive" "" "-"
assert_rest 401 MissingAuthorizationNotification

case_ "S4.1f the GraphQL connection tokenless" "401 — the bearer is checked before the document is parsed, so this is the REST envelope, not a GraphQL error"
gql 'query { permissions(first: 1) { totalCount } }' '{}' "-"
assert_status 401

# ── S4.2 layer 1: a principal holding SOMETHING ELSE ──────────────────────────────────────

case_ "S4.2a principal B (tenant:read) on the permission LISTING" "403 MissingPermissionNotification — holding a permission is not holding THIS one"
api GET "/permissions" "" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S4.2b principal B on the by-id read" "403"
api GET "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410" "" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S4.2c principal B on insert" "403 — permission:insert is a platform-only grant"
api POST "/permissions" '{"resource":"tenant","action":"read","description":"A write attempted by a principal that holds no catalog grant at all."}' "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S4.2d principal B on patch" "403"
api PATCH "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410" '{"description":"A reword attempted by a principal with no catalog grant."}' "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

case_ "S4.2e principal B on archive" "403"
api PATCH "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410/archive" "" "$QA_TOKEN_LIMITED"
assert_rest 403 MissingPermissionNotification

# ── S4.3 the complement: a gate that refuses everyone is also broken ──────────────────────

case_ "S4.3a principal A reads the catalog" "200 — the wildcard grant satisfies every check"
api GET "/permissions?first=1"
assert_status 200

case_ "S4.3b principal A writes into it" "201"
api POST /permissions "$(permission_body "$(pair_resource s4)" read "A catalog entry written by the privileged principal, proving the gate is not shut for everyone.")"
assert_status 201
S4_ID=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "S4.3c principal A patches it" "200"
api PATCH "/permissions/$S4_ID" '{"description":"The privileged-principal fixture, reworded to prove the update gate opens for it."}'
assert_status 200

case_ "S4.3d principal A archives it" "204"
api PATCH "/permissions/$S4_ID/archive"
assert_empty_body 204

# ── S4.4 principal C: the gate must discriminate PER VERB, not per caller ─────────────────
#
# This is the block A and B cannot produce between them. If someone re-mounted all five routes
# on `permission:read`, every case above would still pass: B is refused by all five either way
# and A passes all five either way. C is the only principal for which the five answers differ.

if [ -z "${QA_TOKEN_PERMREAD:-}" ]; then
  case_ "S4.4 principal C — permission:read and nothing else" "the per-verb split"
  skip_ "principal C was not built by run.sh (QA_TOKEN_PERMREAD is empty) — the per-verb half of the permission gate is UNPROVEN this run"
else
  case_ "S4.4a principal C on the LISTING it holds" "200 — permission:read reaches the collection"
  api GET "/permissions?first=1" "" "$QA_TOKEN_PERMREAD"
  assert_status 200

  case_ "S4.4b principal C on the by-id read it holds" "200 — the two reads share one literal, and both must open"
  api GET "/permissions?first=1" "" "$QA_TOKEN_PERMREAD"
  PC_ID=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].id')
  api GET "/permissions/$PC_ID" "" "$QA_TOKEN_PERMREAD"
  assert_status 200

  case_ "S4.4c principal C on INSERT" "403 MissingPermissionNotification — reading the catalog is not writing it"
  api POST "/permissions" '{"resource":"tenant","action":"read","description":"A write attempted by a principal that may only read the catalog."}' "$QA_TOKEN_PERMREAD"
  assert_rest 403 MissingPermissionNotification

  case_ "S4.4d principal C on PATCH" "403 — permission:update is its own grant"
  api PATCH "/permissions/$PC_ID" '{"description":"A reword attempted by a principal that may only read the catalog."}' "$QA_TOKEN_PERMREAD"
  assert_rest 403 MissingPermissionNotification

  case_ "S4.4e principal C on ARCHIVE" "403 — permission:archive is its own grant, and retiring a row is not rewording one"
  api PATCH "/permissions/$PC_ID/archive" "" "$QA_TOKEN_PERMREAD"
  assert_rest 403 MissingPermissionNotification

  case_ "S4.4f the same per-verb split holds on GRAPHQL" "the read resolves, the mutation is refused — a route gated on REST is not thereby gated here"
  # edges is selected on purpose: totalCount ALONE is the only-total mode on this surface, so
  # `first:` beside it would trip the only-total conflict matrix and refuse for a reason that
  # has nothing to do with the gate under test.
  gql 'query { permissions(first: 1) { totalCount edges { node { id } } } }' '{}' "$QA_TOKEN_PERMREAD"
  assert_gql_ok '(.data.permissions.totalCount > 0)' "true"

  case_ "S4.4g and the GraphQL mutation refuses it" "MissingPermissionNotification in errors[].extensions"
  gql 'mutation($i: CreatePermissionInput!) { createPermission(input: $i) { id } }' \
      '{"i":{"resource":"tenant","action":"read","description":"A mutation attempted by a principal that may only read the catalog."}}' \
      "$QA_TOKEN_PERMREAD"
  assert_gql MissingPermissionNotification
fi

# ── S4.5 principal D: the catalog is GLOBAL, and that is a decision being proven ──────────
#
# Six of this service's seven aggregates are `dataAccess: scoped` and narrow every read to the
# caller's tenant. Permission is the ONE that is not, deliberately: it is the platform's
# catalog, not a customer's data (spec §10, and the maintainer confirmed it on 2026-09-07 when
# the tenant-scoping fix of 33eb883 raised the question). Today that is asserted only by the
# ABSENCE of a filter in ToCriteria. This is the case that asserts it positively — and the case
# that turns RED first if the catalog is ever scoped.

if [ -z "${QA_TOKEN_OTHERTENANT:-}" ]; then
  case_ "S4.5 principal D — permission:read from a SECOND tenant" "the same catalog as principal C"
  skip_ "principal D was not built by run.sh (QA_TOKEN_OTHERTENANT is empty) — the global-catalog decision is UNPROVEN this run, asserted only by code inspection"
else
  case_ "S4.5a principal D is genuinely in another tenant" "a tenant_id claim different from principal C's — otherwise the comparison below proves nothing"
  TEN_C=$(jwt_claim "${QA_TOKEN_PERMREAD:-$QA_TOKEN_ADMIN}" tenant_id)
  TEN_D=$(jwt_claim "$QA_TOKEN_OTHERTENANT" tenant_id)
  if [ -n "$TEN_D" ] && [ "$TEN_D" != "$TEN_C" ]; then pass_; else fail_ "tenant_id C='$TEN_C' D='$TEN_D'"; fi

  case_ "S4.5b principal D reaches the catalog at all" "200 — permission:read is not narrowed away by a tenant it does not share"
  api GET "/permissions?onlyTotal=true" "" "$QA_TOKEN_OTHERTENANT"
  assert_status 200
  TOTAL_D=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.totalCount')

  case_ "S4.5c and it sees exactly what the master-tenant principal sees" "the SAME totalCount — the catalog is global, so a second tenant is not a second view of it"
  api GET "/permissions?onlyTotal=true" "" "${QA_TOKEN_PERMREAD:-$QA_TOKEN_ADMIN}"
  assert_json_at 200 '.pagination.totalCount' "$TOTAL_D"

  case_ "S4.5d the same first page, row for row" "identical ids in identical order — a scoped read would differ here even when the counts happened to match"
  api GET "/permissions?orderBy=resource&first=5" "" "${QA_TOKEN_PERMREAD:-$QA_TOKEN_ADMIN}"
  PAGE_C=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].id] | join(",")')
  api GET "/permissions?orderBy=resource&first=5" "" "$QA_TOKEN_OTHERTENANT"
  assert_json_at 200 '[.data[].id] | join(",")' "$PAGE_C"

  case_ "S4.5e and the same on GRAPHQL" "the same totalCount from the other surface"
  gql 'query { permissions { totalCount } }' '{}' "$QA_TOKEN_OTHERTENANT"
  assert_gql_ok '.data.permissions.totalCount' "$TOTAL_D"
fi

# ── S4.6 layer 2, and why there is nothing to assert ──────────────────────────────────────

case_ "S4.6 identity-derived row and field rules on Permission" "recorded as a DECISION, and deliberately not exercised"
skip_ "authz.dataAccess: anyone-with-permission (spec §10): the catalog is global, carries no tenant_id and has no owner. Both ToCriteria implementations return the criteria unchanged, BuildRules reads no principal field, and no Restrict is declared — so there is no per-row or per-field boundary on this entity to assert. S4.5 proves the positive form of the same decision"

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


# ═════════════════════════════════════════════════════════════════════════════════════════
# S5 — Role. specs/qa/role-contract/plan.md §3.
#
#   The 401 family, the framework's appended public surfaces, the introspection bypass and the
#   middleware tenant gate are INHERITED from S3 and not repeated: they are properties of the
#   middleware, which does not know which route it is guarding.
#
#   WHAT IS NEW HERE, and why this block is longer than S4's. Role is the FIRST aggregate this
#   service owns by tenant, so authorization layers 2 and 3 stop being N/A:
#     · layer 1 — seven routes, five literals, two surfaces, plus the role:update / role:grant
#       split that only principal F can see;
#     · layer 2 — refuseForeignTenant, which runs under IfArchive as well as IfInsertOrUpdate,
#       and the no-escalation rule beside it;
#     · layer 3 — ToCriteria's forced Filter["TenantID"], where a leak answers 200.
# ═════════════════════════════════════════════════════════════════════════════════════════

S5_DEAD="00000000-0000-4000-8000-00000000dead"
S5_PERM_TENANT_READ="01990000-0000-7000-8000-00000000001e"

# ── S5.1 the public-route split, direction 2, for the seven routes this round added ───────
#
# Direction 1 is inherited (S3b.1-S3b.3). This is the direction that catches a publicRoutes
# entry widened past its intent, and it is cheap: no token, one assertion.

case_ "S5.1a GET /roles tokenless" "401 MissingAuthorizationNotification"
api GET "/roles" "" -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1b GET /roles/{id} tokenless" "401"
api GET "/roles/$S5_DEAD" "" -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1c POST /roles tokenless" "401 — refused before the body is ever validated"
api POST "/roles" '{"key":"qa-tokenless","name":"X","description":"Y"}' -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1d PATCH /roles/{id} tokenless" "401"
api PATCH "/roles/$S5_DEAD" '{"name":"X"}' -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1e PATCH /roles/{id}/archive tokenless" "401"
api PATCH "/roles/$S5_DEAD/archive" "" -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1f POST /roles/{id}/permissions tokenless" "401 — the collection verbs are not a back door"
api POST "/roles/$S5_DEAD/permissions" '{"permissionID":"'"$S5_PERM_TENANT_READ"'"}' -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1g PATCH /roles/{id}/permissions/{childId}/archive tokenless" "401"
api PATCH "/roles/$S5_DEAD/permissions/$S5_DEAD/archive" "" -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1h the GraphQL connection tokenless" "401 — the bearer is checked before the document is parsed, so this is the REST envelope and not a GraphQL error"
gql "query { roles(first: 1) { totalCount } }" "" -
assert_rest 401 MissingAuthorizationNotification

case_ "S5.1i a GraphQL role MUTATION tokenless" "401 — same"
gql "mutation { archiveRole(id: \"$S5_DEAD\") { success } }" "" -
assert_rest 401 MissingAuthorizationNotification

# ── S5.2 layer 1: a principal holding SOMETHING ELSE ──────────────────────────────────────
#
# Principal B holds tenant:read and nothing else. Holding A permission is not holding THIS one,
# and the value the envelope hands back is what tells a caller which grant they are missing.

s5_denied() { # s5_denied CASE METHOD PATH BODY EXPECTED_LITERAL
  case_ "$1" "403 MissingPermissionNotification, value '$5'"
  api "$2" "$3" "$4" "$QA_TOKEN_LIMITED"
  local hit
  hit=$(printf '%s' "$HTTP_BODY" | jq -r --arg v "$5" \
    '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .field=="permission" and .value==$v)] | length' 2>/dev/null)
  if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on '$5'"; fi
}

s5_denied "S5.2a principal B on the role LISTING"   GET   "/roles"                              ""                                                    "role:read"
s5_denied "S5.2b principal B on the by-id read"     GET   "/roles/$S5_DEAD"                     ""                                                    "role:read"
s5_denied "S5.2c principal B on insert"             POST  "/roles"                              '{"key":"qa-denied","name":"X","description":"Y"}'    "role:insert"
s5_denied "S5.2d principal B on patch"              PATCH "/roles/$S5_DEAD"                     '{"name":"X"}'                                        "role:update"
s5_denied "S5.2e principal B on archive"            PATCH "/roles/$S5_DEAD/archive"             ""                                                    "role:archive"
s5_denied "S5.2f principal B on GRANT"              POST  "/roles/$S5_DEAD/permissions"         '{"permissionID":"'"$S5_PERM_TENANT_READ"'"}'         "role:grant"
s5_denied "S5.2g principal B on REVOKE"             PATCH "/roles/$S5_DEAD/permissions/$S5_DEAD/archive" ""                                           "role:grant"

# ── S5.3 the complement: a gate that refuses everyone is also broken ──────────────────────
#
# For the writes the target id addresses nothing ON PURPOSE: reaching the HANDLER — a 404, never
# a 403 — is what proves the gate opened rather than that the write happened to be valid.

case_ "S5.3a the admin reads the listing" "200 — the wildcard grant satisfies every check"
api GET "/roles?first=1"
assert_status 200

case_ "S5.3b the admin reads by id" "200 on the seeded master role"
api GET "/roles/$QA_MASTER_ROLE_ID"
assert_status 200

case_ "S5.3c the admin inserts" "201"
api POST /roles "$(role_body "$(role_key s5)" "QA Gate Complement" "A role created only to prove the insert gate opens for a caller who holds the literal." "")"
assert_status 201
S5_ROLE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "S5.3d the admin patches" "200"
api PATCH "/roles/$S5_ROLE" '{"name":"QA Gate Complement, relabelled"}'
assert_status 200

case_ "S5.3e the admin GRANTS" "201 — role:grant is satisfied by the wildcard like every other literal"
api POST "/roles/$S5_ROLE/permissions" '{"permissionID":"'"$S5_PERM_TENANT_READ"'"}'
assert_status 201
S5_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.rolePermission.id')

case_ "S5.3f the admin REVOKES" "204"
api PATCH "/roles/$S5_ROLE/permissions/$S5_CHILD/archive"
assert_empty_body 204

case_ "S5.3g the admin archives" "204"
api PATCH "/roles/$S5_ROLE/archive"
assert_empty_body 204

case_ "S5.3h a write the admin aims at nothing reaches the HANDLER" "404, never 403 — the gate opened and the loader is what refused"
api PATCH "/roles/$S5_DEAD/archive"
assert_rest 404 RecordNotFoundNotification

# ── S5.4 principal F: the role:update / role:grant split, decided 2026-08-28 ──────────────
#
# "May relabel the role" and "may change what the role can do" are different jobs, and the
# second is the privilege-escalation surface. F is the ONLY caller for which the two answers
# differ: A passes all seven either way and B is refused all seven either way.

if [ -z "${QA_TOKEN_NOGRANT:-}" ]; then
  case_ "S5.4 principal F — role:update WITHOUT role:grant" "the verb split of 2026-08-28"
  skip_ "principal F could not be provisioned by qa/run.sh. Without it the split is unprovable: principal A holds every literal and principal B holds none, so neither can tell role:update apart from role:grant"
else
  S5_F_ROLE=$(new_role "$(role_key s5f)" "${QA_TENANT_SCOPED:-}" "$S5_PERM_TENANT_READ") || exit 1

  case_ "S5.4a principal F reads the listing" "200 — role:read is in its bundle"
  api GET "/roles?first=1" "" "$QA_TOKEN_NOGRANT"
  assert_status 200

  case_ "S5.4b principal F reads by id" "200 — the row is in its own tenant, so neither the gate nor the scope refuses"
  api GET "/roles/$S5_F_ROLE" "" "$QA_TOKEN_NOGRANT"
  assert_status 200

  case_ "S5.4c principal F RELABELS the role" "200 — role:update is exactly what it holds"
  api PATCH "/roles/$S5_F_ROLE" '{"name":"QA Split Role, relabelled by a caller who may not grant"}' "$QA_TOKEN_NOGRANT"
  assert_status 200

  case_ "S5.4d THE SPLIT: principal F on GRANT" "403 MissingPermissionNotification, value 'role:grant' — it may rename the role and not change what the role can do"
  api POST "/roles/$S5_F_ROLE/permissions" '{"permissionID":"'"$S5_PERM_TENANT_READ"'"}' "$QA_TOKEN_NOGRANT"
  hit=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .value=="role:grant")] | length' 2>/dev/null)
  if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on 'role:grant'"; fi

  case_ "S5.4e and on REVOKE" "403, same literal — ONE verb covers both directions, because splitting grant from revoke would move the asymmetry rather than remove it"
  api GET "/roles/$S5_F_ROLE" "" "$QA_TOKEN_NOGRANT"
  S5_F_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.permissions[0].id')
  api PATCH "/roles/$S5_F_ROLE/permissions/$S5_F_CHILD/archive" "" "$QA_TOKEN_NOGRANT"
  hit=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .value=="role:grant")] | length' 2>/dev/null)
  if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on 'role:grant'"; fi

  case_ "S5.4f and the grant it was refused is still there" "1 entry — the refusal refused, it did not half-apply"
  api GET "/roles/$S5_F_ROLE"
  assert_json_at 200 '.data.permissions | length' "1"
fi

# ── S5.5 the same gate on GRAPHQL: a route gated on REST is not thereby gated here ────────

s5_gql_denied() { # s5_gql_denied CASE QUERY VARS EXPECTED_LITERAL
  case_ "$1" "MissingPermissionNotification in errors[].extensions, value '$4'"
  gql "$2" "$3" "$QA_TOKEN_LIMITED"
  local hit
  hit=$(printf '%s' "$HTTP_BODY" | jq -r --arg v "$4" \
    '[.errors[]?.extensions? | select(.notificationKey=="MissingPermissionNotification" and .value==$v)] | length' 2>/dev/null)
  if [ "$HTTP_STATUS" = "200" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, extensions $(printf '%s' "$HTTP_BODY" | jq -c '[.errors[]?.extensions]' 2>/dev/null)"; fi
}

s5_gql_denied "S5.5a principal B on the roles connection" "query { roles(first: 1) { totalCount } }" "" "role:read"
s5_gql_denied "S5.5b principal B on role(id:)"            "query { role(id: \"$S5_DEAD\") { id } }" "" "role:read"
s5_gql_denied "S5.5c principal B on createRole"           "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id } }" '{"i":{"key":"qa-denied-gql","name":"X","description":"Y","permissions":[]}}' "role:insert"
s5_gql_denied "S5.5d principal B on patchRole"            "mutation(\$i: PatchRoleInput!) { patchRole(id: \"$S5_DEAD\", input: \$i) { id } }" '{"i":{"name":"X"}}' "role:update"
s5_gql_denied "S5.5e principal B on archiveRole"          "mutation { archiveRole(id: \"$S5_DEAD\") { success } }" "" "role:archive"
s5_gql_denied "S5.5f principal B on addRolePermission"    "mutation(\$i: AddRolePermissionInput!) { addRolePermission(id: \"$S5_DEAD\", input: \$i) { roleId } }" '{"i":{"permissionID":"'"$S5_PERM_TENANT_READ"'"}}' "role:grant"
s5_gql_denied "S5.5g principal B on archiveRolePermission" "mutation(\$i: ArchiveRolePermissionInput!) { archiveRolePermission(id: \"$S5_DEAD\", input: \$i) { success } }" '{"i":{"rolePermissionId":"'"$S5_DEAD"'"}}' "role:grant"

case_ "S5.5h THE COMPLEMENT on this surface" "the admin's connection resolves — a gate that refuses everyone is also broken, per surface"
# edges is selected alongside totalCount on purpose: selecting the count ALONE is this surface's
# only-total mode, and first: beside it is the same conflict the REST matrix refuses.
gql "query { roles(first: 1) { totalCount edges { node { id } } } }"
assert_gql_ok '(.data.roles.totalCount >= 1)' "true"

# ── S5.6 layer 2 and layer 3: the seams that only exist because Role is owned by a tenant ─

if [ -z "${QA_TOKEN_SCOPED:-}" ]; then
  case_ "S5.6 layers 2 and 3 on Role" "refuseForeignTenant on the write side, and ToCriteria's forced filter on the read side"
  skip_ "principal E could not be provisioned by qa/run.sh. Both layers need a caller who is authenticated and is NOT a super-admin: a *:* holder crosses the row scope BY DESIGN, so running these as the admin would assert the bypass and call it the boundary"
else
  case_ "S5.6a LAYER 2: the scoped principal writes into ANOTHER tenant" "403 TenantMismatchNotification — the claim decides what a caller MAY write, and the write side is not narrowed by a filter the way a read is"
  api POST /roles "$(jq -nc --arg k "$(role_key s5x)" --arg t "$QA_MASTER_TENANT_ID" \
    '{key:$k, name:"QA Foreign Write", description:"A write aimed at a partition the caller has no claim to, which the row-scope rule refuses.", tenantID:$t, permissions:[]}')" "$QA_TOKEN_SCOPED"
  assert_rest 403 TenantMismatchNotification

  case_ "S5.6b THE BYPASS: the same body sent by the admin" "201 — *:* crosses the row scope, which is what lets a platform operator support a customer"
  api POST /roles "$(jq -nc --arg k "$(role_key s5y)" --arg t "$QA_MASTER_TENANT_ID" \
    '{key:$k, name:"QA Operator Write", description:"The same write, from the caller the bypass exists for, landing in the master partition.", tenantID:$t, permissions:[]}')"
  assert_status 201

  case_ "S5.6c LAYER 3: the listing narrows to the caller's own tenant" "every row carries the caller's own tenantID — a leak here answers 200, which is why it needs its own case"
  api GET "/roles?first=100" "" "$QA_TOKEN_SCOPED"
  assert_json_at 200 '[.data[].tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S5.6d THE LEAK ITSELF: a by-id read of another tenant's role" "404 RecordNotFoundNotification — NOT 403: a 403 would confirm the row exists to a caller who may not see it"
  api GET "/roles/$QA_MASTER_ROLE_ID" "" "$QA_TOKEN_SCOPED"
  assert_rest 404 RecordNotFoundNotification

  case_ "S5.6e THE COMPLEMENT: the admin crosses the same scope" "200 on the very id the scoped principal was refused — a scope that refuses everyone is also broken"
  api GET "/roles/$QA_MASTER_ROLE_ID"
  assert_status 200

  case_ "S5.6f and the scope holds on GRAPHQL too" "every edge carries the caller's own tenantID — ToCriteria is shared, but a surface that skipped it would look exactly like a passing REST case"
  gql "query { roles(first: 100) { edges { node { tenantID } } } }" "" "$QA_TOKEN_SCOPED"
  assert_gql_ok '[.data.roles.edges[].node.tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S5.6g the admin's GraphQL connection is NOT narrowed" "more than one distinct tenant — the bypass reaches this surface as well"
  gql "query { roles(first: 100) { edges { node { tenantID } } } }"
  assert_gql_ok '([.data.roles.edges[].node.tenantID] | unique | length) > 1' "true"
fi

case_ "S5.7 field-level read authz on Role" "recorded as a DECISION, and deliberately not exercised"
skip_ "spec.md §9 declares no ReadCriteria.Restrict: 'Every field a caller may see the row at all for, they may see entirely. Row-level isolation does the work here.' There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge to assert — that edge exists only where a restricted field is in the selection. Asserting one would be inventing a rule"

qa_finish
