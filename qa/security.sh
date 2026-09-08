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

# ═════════════════════════════════════════════════════════════════════════════════════════
# S6 — Group. specs/qa/group-contract/plan.md §3.
#
# §3a (the whole 401 family) and §3b direction 1 are INHERITED from the tenant round and are
# not repeated: the middleware does not know which route it is guarding. What is Group-specific
# is below — and it is where the FIFTH VERB becomes visible, because group:grant is the only
# literal in this service that one principal can lack while holding everything else.
#
# s5_denied / s5_gql_denied are generic helpers the role round introduced (principal B, the
# literal asserted by value). They are reused rather than copied.
# ═════════════════════════════════════════════════════════════════════════════════════════

S6_DEAD="00000000-0000-4000-8000-00000000dead"

# ── S6.1 the public-route half, direction 2, for the seven routes this round adds ─────────
case_ "S6.1a GET /groups tokenless" "401 MissingAuthorizationNotification — auth.publicRoutes names no /groups path, and a route that was never gated at all would pass every 401 case in §3a"
api GET "/groups" "" -
assert_rest 401 MissingAuthorizationNotification
case_ "S6.1b GET /groups/{id} tokenless" "401"
api GET "/groups/$S6_DEAD" "" -; assert_rest 401 MissingAuthorizationNotification
case_ "S6.1c POST /groups tokenless" "401 — refused before the body is ever validated"
api POST "/groups" '{"key":"qa-tokenless","name":"X","description":"Y"}' -; assert_rest 401 MissingAuthorizationNotification
case_ "S6.1d PATCH /groups/{id} tokenless" "401"
api PATCH "/groups/$S6_DEAD" '{"name":"X"}' -; assert_rest 401 MissingAuthorizationNotification
case_ "S6.1e PATCH /groups/{id}/archive tokenless" "401"
api PATCH "/groups/$S6_DEAD/archive" "" -; assert_rest 401 MissingAuthorizationNotification
case_ "S6.1f POST /groups/{id}/roles tokenless" "401 — the collection verbs are not a back door"
api POST "/groups/$S6_DEAD/roles" '{"roleID":"'"$S6_DEAD"'"}' -; assert_rest 401 MissingAuthorizationNotification
case_ "S6.1g PATCH /groups/{id}/roles/{childId}/archive tokenless" "401"
api PATCH "/groups/$S6_DEAD/roles/$S6_DEAD/archive" "" -; assert_rest 401 MissingAuthorizationNotification

s6_gql_tokenless() { # s6_gql_tokenless CASE QUERY [VARS]
  case_ "$1" "401 in the REST envelope — the bearer is checked before the document is parsed, so a GraphQL field is not a way around the gate"
  gql "$2" "${3:-}" -
  assert_rest 401 MissingAuthorizationNotification
}
s6_gql_tokenless "S6.1h groups(...) tokenless"   "query { groups(first: 1) { totalCount } }"
s6_gql_tokenless "S6.1i group(id:) tokenless"    "query { group(id: \"$S6_DEAD\") { id } }"
s6_gql_tokenless "S6.1j createGroup tokenless"   "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" '{"i":{"key":"qa-tokenless-gql","name":"X","description":"Y","roles":[]}}'
s6_gql_tokenless "S6.1k patchGroup tokenless"    "mutation(\$i: PatchGroupInput!) { patchGroup(id: \"$S6_DEAD\", input: \$i) { id } }" '{"i":{"name":"X"}}'
s6_gql_tokenless "S6.1l archiveGroup tokenless"  "mutation { archiveGroup(id: \"$S6_DEAD\") { success } }"
s6_gql_tokenless "S6.1m addGroupRole tokenless"  "mutation(\$i: AddGroupRoleInput!) { addGroupRole(id: \"$S6_DEAD\", input: \$i) { groupId } }" '{"i":{"roleID":"'"$S6_DEAD"'"}}'
s6_gql_tokenless "S6.1n archiveGroupRole tokenless" "mutation(\$i: ArchiveGroupRoleInput!) { archiveGroupRole(id: \"$S6_DEAD\", input: \$i) { success } }" '{"i":{"groupRoleId":"'"$S6_DEAD"'"}}'

# ── S6.2 Layer 1, the gate: principal B holds tenant:read and nothing else ────────────────
s6_denied() { s5_denied "$@"; }
s6_denied "S6.2a principal B on the group LISTING" GET   "/groups"                        ""                                                  "group:read"
s6_denied "S6.2b principal B on the by-id read"    GET   "/groups/$S6_DEAD"               ""                                                  "group:read"
s6_denied "S6.2c principal B on insert"            POST  "/groups"                        '{"key":"qa-denied","name":"X","description":"Y"}'  "group:insert"
s6_denied "S6.2d principal B on patch"             PATCH "/groups/$S6_DEAD"               '{"name":"X"}'                                      "group:update"
s6_denied "S6.2e principal B on archive"           PATCH "/groups/$S6_DEAD/archive"       ""                                                  "group:archive"
s6_denied "S6.2f principal B on ATTACH"            POST  "/groups/$S6_DEAD/roles"         '{"roleID":"'"$S6_DEAD"'"}'                         "group:grant"
s6_denied "S6.2g principal B on DETACH"            PATCH "/groups/$S6_DEAD/roles/$S6_DEAD/archive" ""                                         "group:grant"

# ── S6.3 the complement: a gate that refuses everyone is also broken ──────────────────────
#
# For the writes the target id addresses nothing ON PURPOSE: reaching the HANDLER — a 404,
# never a 403 — is what proves the gate opened rather than that the write happened to be valid.

S6_TEN=$(new_tenant active "$(ws s6)") || exit 1
S6_ROLE=$(new_role "$(role_key s6)" "$S6_TEN" "$(permission_id_of tenant read)") || exit 1
S6_GROUP=$(new_group "$(group_key s6)" "$S6_TEN" "$S6_ROLE") || exit 1
api GET "/groups/$S6_GROUP"; S6_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.roles[0].id')

case_ "S6.3a the admin reads the listing" "200 — the wildcard grant satisfies every check"
api GET "/groups?first=1"; assert_status 200
case_ "S6.3b the admin reads by id"       "200"
api GET "/groups/$S6_GROUP"; assert_status 200
case_ "S6.3c the admin inserts"           "201"
api POST /groups "$(group_body "$(group_key s6b)" "QA Gate Complement" "A group created only to prove the insert gate opens for a caller who holds the literal." "$S6_TEN")"
assert_status 201
case_ "S6.3d the admin patches"           "200"
api PATCH "/groups/$S6_GROUP" "$(jq -nc '{name:"QA Gate Complement Renamed"}')"; assert_status 200
case_ "S6.3e the admin ATTACHES"          "201 — group:grant is satisfied, and the domain then has its own say (GR4/GR5)"
S6_ROLE2=$(new_role "$(role_key s6b)" "$S6_TEN" "$(permission_id_of group read)") || exit 1
api POST "/groups/$S6_GROUP/roles" "$(jq -nc --arg r "$S6_ROLE2" '{roleID:$r}')"; assert_status 201
case_ "S6.3f the admin DETACHES"          "204"
api PATCH "/groups/$S6_GROUP/roles/$S6_CHILD/archive"; assert_empty_body 204
case_ "S6.3g the admin archives an id that addresses nothing" "404, NEVER 403 — reaching the handler is what proves the gate opened"
api PATCH "/groups/$S6_DEAD/archive"; assert_status 404

# ── S6.4 the FIFTH VERB, visible to exactly one principal ─────────────────────────────────
if [ -z "${QA_TOKEN_GROUPNOGRANT:-}" ]; then
  case_ "S6.4 the group:grant split" "principal H — group:read + group:update, and NOT group:grant"
  skip_ "principal H was not built by qa/run.sh. It is the ONLY caller for which the fifth verb is visible: B is refused on all seven routes and G passes all seven, so neither can see 'may relabel the group, may not change what it confers'. That split is UNPROVEN this run"
else
  S6_H_GROUP=$(new_group "$(group_key s6h)" "$QA_TENANT_SCOPED" ) || exit 1
  case_ "S6.4a principal H lists groups"  "200 — it holds group:read"
  api GET "/groups?first=1" "" "$QA_TOKEN_GROUPNOGRANT"; assert_status 200
  case_ "S6.4b principal H reads by id"   "200"
  api GET "/groups/$S6_H_GROUP" "" "$QA_TOKEN_GROUPNOGRANT"; assert_status 200
  case_ "S6.4c principal H RELABELS the group" "200 — it holds group:update"
  api PATCH "/groups/$S6_H_GROUP" "$(jq -nc '{name:"QA Relabelled By H"}')" "$QA_TOKEN_GROUPNOGRANT"; assert_status 200

  s6h_denied() { # s6h_denied CASE METHOD PATH BODY
    case_ "$1" "403 MissingPermissionNotification, value 'group:grant' — 'may rename the group' and 'may change what the group confers' are different jobs, and the second is the escalation surface"
    api "$2" "$3" "$4" "$QA_TOKEN_GROUPNOGRANT"
    local hit
    hit=$(printf '%s' "$HTTP_BODY" | jq -r \
      '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .field=="permission" and .value=="group:grant")] | length' 2>/dev/null)
    if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on 'group:grant'"; fi
  }
  s6h_denied "S6.4d principal H on ATTACH" POST  "/groups/$S6_H_GROUP/roles" "$(jq -nc --arg r "$S6_ROLE" '{roleID:$r}')"
  s6h_denied "S6.4e principal H on DETACH" PATCH "/groups/$S6_H_GROUP/roles/$S6_DEAD/archive" ""
fi

# ── S6.5 Layer 1 on GRAPHQL. A route gated on REST is not thereby gated here ──────────────
s6_gql_denied() { s5_gql_denied "$@"; }
s6_gql_denied "S6.5a principal B on the groups connection" "query { groups(first: 1) { totalCount } }" "" "group:read"
s6_gql_denied "S6.5b principal B on group(id:)"            "query { group(id: \"$S6_DEAD\") { id } }" "" "group:read"
s6_gql_denied "S6.5c principal B on createGroup"           "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" '{"i":{"key":"qa-denied-gql","name":"X","description":"Y","roles":[]}}' "group:insert"
s6_gql_denied "S6.5d principal B on patchGroup"            "mutation(\$i: PatchGroupInput!) { patchGroup(id: \"$S6_DEAD\", input: \$i) { id } }" '{"i":{"name":"X"}}' "group:update"
s6_gql_denied "S6.5e principal B on archiveGroup"          "mutation { archiveGroup(id: \"$S6_DEAD\") { success } }" "" "group:archive"
s6_gql_denied "S6.5f principal B on addGroupRole"          "mutation(\$i: AddGroupRoleInput!) { addGroupRole(id: \"$S6_DEAD\", input: \$i) { groupId } }" '{"i":{"roleID":"'"$S6_DEAD"'"}}' "group:grant"
s6_gql_denied "S6.5g principal B on archiveGroupRole"      "mutation(\$i: ArchiveGroupRoleInput!) { archiveGroupRole(id: \"$S6_DEAD\", input: \$i) { success } }" '{"i":{"groupRoleId":"'"$S6_DEAD"'"}}' "group:grant"

case_ "S6.5h the admin's GraphQL connection is served" "200 with no errors — the per-surface complement"
gql "query { groups(first: 1) { totalCount edges { node { id } } } }"
assert_gql_ok '(.data.groups.totalCount >= 0)' "true"

# ── S6.6 / S6.7 Layers 2 and 3, which need principal G ────────────────────────────────────
if [ -z "${QA_TOKEN_GROUP:-}" ]; then
  case_ "S6.6 identity-derived rules and row scoping on Group" "principal G, authenticated, not a super-admin, bound to a tenant of the suite's own"
  skip_ "principal G was not built by qa/run.sh, so Layer 2 (the tenant guard in BuildRules, the escalation and wildcard refusals) and Layer 3 (the ToCriteria row scope) are UNPROVEN this run. No token can be invented to stand in for it"
else
  case_ "S6.6a G creates a group naming the MASTER tenant" "403 TenantMismatchNotification on field tenantID — the row's tenant must equal the caller's claim"
  api POST /groups "$(jq -nc --arg k "$(group_key s6m)" --arg t "$QA_MASTER_TENANT_ID" \
    '{key:$k, name:"QA Foreign", description:"A group aimed at a tenant the caller does not belong to, to prove the write guard.", tenantID:$t, roles:[]}')" "$QA_TOKEN_GROUP"
  assert_rest_field 403 TenantMismatchNotification tenantID

  case_ "S6.6b THE SAME BODY sent by the admin" "201 — the *:* bypass, which is what lets a platform operator support a customer. Two calls that differ ONLY in who is asking"
  api POST /groups "$(jq -nc --arg k "$(group_key s6m2)" --arg t "$QA_MASTER_TENANT_ID" \
    '{key:$k, name:"QA Foreign", description:"A group aimed at a tenant the caller does not belong to, to prove the write guard.", tenantID:$t, roles:[]}')"
  assert_status 201

  case_ "S6.6c G ARCHIVES a group of another tenant" "403 TenantMismatchNotification — refuseForeignTenant runs under IfArchive too, and the write side is NOT filtered by ToCriteria, so the row LOADS and the rule is what refuses. Invisible if only reads were tested"
  api PATCH "/groups/$S6_GROUP/archive" "" "$QA_TOKEN_GROUP"
  assert_rest 403 TenantMismatchNotification

  case_ "S6.6d G attaches a role granting a permission it does not hold" "403 CannotGrantRoleWithUnheldPermissionsNotification — the TRANSITIVE second Layer-2 rule, cross-referenced to GR5"
  S6_G_GROUP=$(new_group "$(group_key s6g)" "$QA_TENANT_SCOPED") || exit 1
  S6_ESC_ROLE=$(new_role "$(role_key s6esc)" "$QA_TENANT_SCOPED" "$(permission_id_of permission archive)") || exit 1
  api POST "/groups/$S6_G_GROUP/roles" "$(jq -nc --arg r "$S6_ESC_ROLE" '{roleID:$r}')" "$QA_TOKEN_GROUP"
  assert_rest 403 CannotGrantRoleWithUnheldPermissionsNotification

  case_ "S6.6e and G is refused a WILDCARD-bearing role for everyone's reasons, not its own" "403 — G holds group:grant, so Layer 1 opened; the domain is what closes. Layer 1 asks 'may this principal touch the edge at all', G10a/G10b ask 'may this principal confer THIS role'"
  S6_GM=$(new_group "$(group_key s6gm)" "$QA_MASTER_TENANT_ID") || exit 1
  api POST "/groups/$S6_GM/roles" "$(jq -nc --arg r "$QA_MASTER_ROLE_ID" '{roleID:$r}')"
  assert_rest 403 CannotGrantWildcardRoleNotification

  # ── Layer 3, where a leak answers 200 ───────────────────────────────────────────────────
  case_ "S6.7a G's listing carries only its OWN tenant's rows" "every row's tenantID is G's own — asserted over the values, not over a count"
  api GET "/groups?first=100" "" "$QA_TOKEN_GROUP"
  assert_json_at 200 '[.data[].tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S6.7b THE LEAK ITSELF: G reads another tenant's group by id" "404 RecordNotFoundNotification — NOT 403. A 403 would confirm the row exists to a caller who may not see it, and a group listing IS the customer's org chart"
  api GET "/groups/$S6_GROUP" "" "$QA_TOKEN_GROUP"
  assert_rest 404 RecordNotFoundNotification

  case_ "S6.7c THE COMPLEMENT: the admin crosses the same scope" "200 on the very id G was refused — a scope that refuses everyone is also broken"
  api GET "/groups/$S6_GROUP"
  assert_status 200

  case_ "S6.7d and the scope holds on GRAPHQL too" "every edge carries G's own tenantID — ToCriteria is shared, but a surface that skipped it would look exactly like a passing REST case"
  gql "query { groups(first: 100) { edges { node { tenantID } } } }" "" "$QA_TOKEN_GROUP"
  assert_gql_ok '[.data.groups.edges[].node.tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S6.7e the admin's GraphQL connection is NOT narrowed" "more than one distinct tenant — the bypass reaches this surface as well"
  gql "query { groups(first: 100) { edges { node { tenantID } } } }"
  assert_gql_ok '([.data.groups.edges[].node.tenantID] | unique | length) > 1' "true"
fi

case_ "S6.8 field-level read authz on Group" "recorded as a DECISION, and deliberately not exercised"
skip_ "spec.md §9 declares no ReadCriteria.Restrict for Group: 'Every field a caller may see the row at all for, they may see entirely. Row-level isolation does the work.' There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge to assert — that edge exists only where a restricted field is in the selection. S6.7 proves the row-level boundary that does the work instead"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════
#  S7 — §3 of specs/qa/user-contract/plan.md. The User gate.
#
#  §3a (the 401 family), §3b (the public-route split) and the framework's appended surfaces
#  are INHERITED from the four approved rounds and are not repeated here.
#
#  What is new, and why this family is bigger than S5 and S6: User is gated by EIGHT distinct
#  literals over fourteen routes — the four every entity has, plus user:grant covering three
#  collections, user:set-claim covering the third of them, and the two credential verbs that
#  no other aggregate has at all. It is also the only entity whose Layer-2 rules read the
#  caller's SUBJECT rather than its tenant, which is what S7.2 exists for.
# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════

S7_DEAD="01990000-dead-7000-8000-000000000000"

# ── S7.1 Layer 1 on REST: the negative, over all fourteen routes ──────────────────────────
s7_denied() { s5_denied "$@"; }

s7_denied "S7.1a principal B on the user LISTING"     GET   "/users"                                    ""                              "user:read"
s7_denied "S7.1b principal B on the by-id read"       GET   "/users/$S7_DEAD"                           ""                              "user:read"
s7_denied "S7.1c principal B on insert"               POST  "/users"                                    '{"givenName":"X","familyName":"Y","email":"qa-denied@authcore.local","status":"active","password":"Qa!Denied2026","passwordConfirmation":"Qa!Denied2026"}' "user:insert"
s7_denied "S7.1d principal B on patch"                PATCH "/users/$S7_DEAD"                           '{"givenName":"X"}'             "user:update"
s7_denied "S7.1e principal B on archive"              PATCH "/users/$S7_DEAD/archive"                   ""                              "user:archive"
s7_denied "S7.1f principal B on JOIN A GROUP"         POST  "/users/$S7_DEAD/groups"                    '{"groupID":"'"$S7_DEAD"'"}'    "user:grant"
s7_denied "S7.1g principal B on LEAVE A GROUP"        PATCH "/users/$S7_DEAD/groups/$S7_DEAD/archive"   ""                              "user:grant"
s7_denied "S7.1h principal B on GRANT A ROLE"         POST  "/users/$S7_DEAD/roles"                     '{"roleID":"'"$S7_DEAD"'"}'     "user:grant"
s7_denied "S7.1i principal B on REVOKE A ROLE"        PATCH "/users/$S7_DEAD/roles/$S7_DEAD/archive"    ""                              "user:grant"
s7_denied "S7.1j principal B on SET A CLAIM"          POST  "/users/$S7_DEAD/claims"                    '{"claimID":"'"$S7_DEAD"'","value":"x"}' "user:set-claim"
s7_denied "S7.1k principal B on CHANGE A CLAIM"       PATCH "/users/$S7_DEAD/claims/$S7_DEAD"           '{"value":"x"}'                 "user:set-claim"
s7_denied "S7.1l principal B on WITHDRAW A CLAIM"     PATCH "/users/$S7_DEAD/claims/$S7_DEAD/archive"   ""                              "user:set-claim"
s7_denied "S7.1m principal B on RESET a password"     PATCH "/users/$S7_DEAD/password-reset"            '{"password":"Qa!Denied2026x","passwordConfirmation":"Qa!Denied2026x"}' "user:reset-password"

# The change is the ONE route with a different story, and it is worth its own case rather than
# a row in the table above: user:change-password is the permission spec.md §10 says must reach
# EVERY user, because a caller without it cannot set their own password at all. Principal B
# does not hold it, so the gate closes — and that closing is what §3e records as the
# deployment cost nobody has paid yet.
s7_denied "S7.1n principal B on CHANGE its own password" PATCH "/users/$S7_DEAD/password"               '{"currentPassword":"a","password":"Qa!Denied2026x","passwordConfirmation":"Qa!Denied2026x"}' "user:change-password"

case_ "S7.1o THE COUNT: eight distinct literals gate fourteen routes" "user:read/insert/update/archive/grant/set-claim/change-password/reset-password — a route that lost its RequirePermission would answer something other than 403 above, and a route that shared another's literal would show up here"
GATED=$(printf '%s\n' "user:read" "user:read" "user:insert" "user:update" "user:archive" "user:grant" "user:grant" "user:grant" "user:grant" "user:set-claim" "user:set-claim" "user:set-claim" "user:reset-password" "user:change-password" | sort -u | wc -l | tr -d ' ')
if [ "$GATED" = "8" ]; then pass_; else fail_ "distinct literals asserted above = $GATED"; fi

# ── S7.1p-w the complement: a gate that refuses everyone is also broken ───────────────────
#
# For the writes the target id addresses nothing ON PURPOSE: reaching the HANDLER — a 404,
# never a 403 — is what proves the gate opened rather than that the write happened to be valid.

if [ -z "${QA_TOKEN_USEROP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S7.1p-w — principal I was not built, so the Layer-1 complement is UNPROVEN this run and only the negative half above stands"
else
  S7_TARGET=$(new_user "$(user_email s7)" "$QA_TENANT_SCOPED") || true

  case_ "S7.1p principal I on the user LISTING" "200 — it holds user:read"
  api GET "/users?first=1" "" "$QA_TOKEN_USEROP"
  assert_status 200

  case_ "S7.1q principal I on insert" "201 — it holds user:insert"
  api POST /users "$(user_body "$(user_email s7i)" active "$QA_TENANT_SCOPED")" "$QA_TOKEN_USEROP"
  assert_status 201

  case_ "S7.1r principal I on patch" "200 — it holds user:update"
  if [ -n "${S7_TARGET:-}" ]; then
    api PATCH "/users/$S7_TARGET" '{"givenName":"Gated"}' "$QA_TOKEN_USEROP"
    assert_status 200
  else skip_ "S7.1r — the target could not be created"; fi

  case_ "S7.1s principal I on a collection add" "404 and NOT 403 — the gate opened and the handler was reached; the id addresses nothing on purpose"
  api POST "/users/$S7_DEAD/groups" "$(jq -nc --arg g "$S7_DEAD" '{groupID:$g}')" "$QA_TOKEN_USEROP"
  assert_status_not 403

  case_ "S7.1t principal I on a claim add" "404 and NOT 403 — user:set-claim is a literal of its own, and this is what proves I holds it separately from user:grant"
  api POST "/users/$S7_DEAD/claims" "$(jq -nc --arg c "$S7_DEAD" '{claimID:$c, value:"x"}')" "$QA_TOKEN_USEROP"
  assert_status_not 403

  case_ "S7.1u principal I on archive" "404 and NOT 403"
  api PATCH "/users/$S7_DEAD/archive" "" "$QA_TOKEN_USEROP"
  assert_status_not 403
fi

# ── S7.1x-z THE SPLIT: principal J, the only caller for which the four verbs differ ───────
#
# J holds user:read and user:update and NOT user:grant, user:set-claim or user:reset-password.
# A and B answer identically on all fourteen routes either way; J is what makes the split
# visible at all — "may fix a typo in a name" and "may hand out roles" and "may take over an
# account" are three different jobs, and spec.md §10 says so explicitly.

if [ -z "${QA_TOKEN_USERNOGRANT:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S7.1x-z — principal J was not built, so the four-way verb split is UNPROVEN this run"
else
  S7_J_TARGET=$(new_user "$(user_email s7j)" "$QA_TENANT_SCOPED") || true

  if [ -z "${S7_J_TARGET:-}" ]; then
    skip_ "S7.1x-z — the split's target user could not be created"
  else
    case_ "S7.1x principal J MAY rename" "200 — user:update is the verb it holds, and it must keep working"
    api PATCH "/users/$S7_J_TARGET" '{"givenName":"Relabelled"}' "$QA_TOKEN_USERNOGRANT"
    assert_status 200

    s7j_denied() { # s7j_denied CASE METHOD PATH BODY LITERAL
      case_ "$1" "403 MissingPermissionNotification, value '$5' — whoever may fix a typo in a name must not be able to $6"
      api "$2" "$3" "$4" "$QA_TOKEN_USERNOGRANT"
      local hit
      hit=$(printf '%s' "$HTTP_BODY" | jq -r --arg v "$5" \
        '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .field=="permission" and .value==$v)] | length' 2>/dev/null)
      if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on '$5'"; fi
    }

    s7j_denied "S7.1y1 principal J on JOIN A GROUP"   POST  "/users/$S7_J_TARGET/groups" "$(jq -nc --arg g "$S7_DEAD" '{groupID:$g}')" "user:grant" "confer permissions by indirection"
    s7j_denied "S7.1y2 principal J on GRANT A ROLE"   POST  "/users/$S7_J_TARGET/roles"  "$(jq -nc --arg r "$S7_DEAD" '{roleID:$r}')"  "user:grant" "confer permissions directly"
    s7j_denied "S7.1y3 principal J on LEAVE A GROUP"  PATCH "/users/$S7_J_TARGET/groups/$S7_DEAD/archive" "" "user:grant" "withdraw them either"
    s7j_denied "S7.1y4 principal J on REVOKE A ROLE"  PATCH "/users/$S7_J_TARGET/roles/$S7_DEAD/archive"  "" "user:grant" "withdraw them either"
    s7j_denied "S7.1y5 principal J on SET A CLAIM"    POST  "/users/$S7_J_TARGET/claims" "$(jq -nc --arg c "$S7_DEAD" '{claimID:$c, value:"x"}')" "user:set-claim" "set a value this service cannot audit the consequences of"
    s7j_denied "S7.1y6 principal J on CHANGE A CLAIM" PATCH "/users/$S7_J_TARGET/claims/$S7_DEAD" '{"value":"x"}' "user:set-claim" "correct one either"
    s7j_denied "S7.1z  principal J on RESET a password" PATCH "/users/$S7_J_TARGET/password-reset" '{"password":"Qa!Takeover26","passwordConfirmation":"Qa!Takeover26"}' "user:reset-password" "take over the account"

    case_ "S7.1z2 user:update DELIBERATELY REACHES NEITHER credential route" "the rename passed and the reset was refused, in the same principal and the same tenant — that is the escalation this model refuses everywhere else, arriving through the least-guarded door"
    api PATCH "/users/$S7_J_TARGET" '{"givenName":"Still"}' "$QA_TOKEN_USERNOGRANT"; S_REN="$HTTP_STATUS"
    api PATCH "/users/$S7_J_TARGET/password-reset" '{"password":"Qa!Takeover26","passwordConfirmation":"Qa!Takeover26"}' "$QA_TOKEN_USERNOGRANT"; S_RES="$HTTP_STATUS"
    if [ "$S_REN" = "200" ] && [ "$S_RES" = "403" ]; then pass_; else HTTP_BODY="rename=$S_REN reset=$S_RES"; fail_ "the two verbs did not separate"; fi
  fi
fi

# ── S7.1gql Layer 1 on GRAPHQL. A route gated on REST is not thereby gated here ───────────
#
# This block matters more on User than on any other entity: spec.md §9 and §10 BOTH said the
# collection and credential fields were not on GraphQL at all (corrected 2026-09-07, plan
# §0c). A gate believed not to exist is a gate nobody checks.
s7_gql_denied() { s5_gql_denied "$@"; }

s7_gql_denied "S7.1gql-a principal B on the users connection" "query { users(first: 1) { totalCount } }" "" "user:read"
s7_gql_denied "S7.1gql-b principal B on user(id:)"            "query { user(id: \"$S7_DEAD\") { id } }" "" "user:read"
s7_gql_denied "S7.1gql-c principal B on createUser"           "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id } }" '{"i":{"givenName":"X","familyName":"Y","email":"qa-denied-gql@authcore.local","status":"active","password":"Qa!Denied2026","passwordConfirmation":"Qa!Denied2026"}}' "user:insert"
s7_gql_denied "S7.1gql-d principal B on patchUser"            "mutation(\$i: PatchUserInput!) { patchUser(id: \"$S7_DEAD\", input: \$i) { id } }" '{"i":{"givenName":"X"}}' "user:update"
s7_gql_denied "S7.1gql-e principal B on archiveUser"          "mutation { archiveUser(id: \"$S7_DEAD\") { success } }" "" "user:archive"
s7_gql_denied "S7.1gql-f principal B on addUserGroup"         "mutation(\$i: AddUserGroupInput!) { addUserGroup(id: \"$S7_DEAD\", input: \$i) { userId } }" '{"i":{"groupID":"'"$S7_DEAD"'"}}' "user:grant"
s7_gql_denied "S7.1gql-g principal B on archiveUserGroup"     "mutation(\$i: ArchiveUserGroupInput!) { archiveUserGroup(id: \"$S7_DEAD\", input: \$i) { success } }" '{"i":{"userGroupId":"'"$S7_DEAD"'"}}' "user:grant"
s7_gql_denied "S7.1gql-h principal B on addUserRole"          "mutation(\$i: AddUserRoleInput!) { addUserRole(id: \"$S7_DEAD\", input: \$i) { userId } }" '{"i":{"roleID":"'"$S7_DEAD"'"}}' "user:grant"
s7_gql_denied "S7.1gql-i principal B on archiveUserRole"      "mutation(\$i: ArchiveUserRoleInput!) { archiveUserRole(id: \"$S7_DEAD\", input: \$i) { success } }" '{"i":{"userRoleId":"'"$S7_DEAD"'"}}' "user:grant"
s7_gql_denied "S7.1gql-j principal B on addUserClaim"         "mutation(\$i: AddUserClaimInput!) { addUserClaim(id: \"$S7_DEAD\", input: \$i) { userId } }" '{"i":{"claimID":"'"$S7_DEAD"'","value":"x"}}' "user:set-claim"
s7_gql_denied "S7.1gql-k principal B on patchUserClaim"       "mutation(\$i: PatchUserClaimInput!) { patchUserClaim(id: \"$S7_DEAD\", input: \$i) { userId } }" '{"i":{"userClaimId":"'"$S7_DEAD"'","value":"x"}}' "user:set-claim"
s7_gql_denied "S7.1gql-l principal B on archiveUserClaim"     "mutation(\$i: ArchiveUserClaimInput!) { archiveUserClaim(id: \"$S7_DEAD\", input: \$i) { success } }" '{"i":{"userClaimId":"'"$S7_DEAD"'"}}' "user:set-claim"
s7_gql_denied "S7.1gql-m principal B on changeUserPassword"   "mutation(\$i: ChangeUserPasswordInput!) { changeUserPassword(id: \"$S7_DEAD\", input: \$i) { success } }" '{"i":{"currentPassword":"a","password":"Qa!Denied2026x","passwordConfirmation":"Qa!Denied2026x"}}' "user:change-password"
s7_gql_denied "S7.1gql-n principal B on resetUserPassword"    "mutation(\$i: ResetUserPasswordInput!) { resetUserPassword(id: \"$S7_DEAD\", input: \$i) { success } }" '{"i":{"password":"Qa!Denied2026x","passwordConfirmation":"Qa!Denied2026x"}}' "user:reset-password"

case_ "S7.1gql-o the admin's GraphQL connection is served" "200 with no errors — the per-surface complement"
gql "query { users(first: 1) { totalCount edges { node { id } } } }"
assert_gql_ok '(.data.users.totalCount >= 0)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# S7.2 — LAYER 2: identity-derived row rules, and the only two in this service that read the
#        caller's SUBJECT rather than its tenant.
#
# Two calls that differ ONLY in who is asking. qa/domain.sh U19 proves these as business
# rules; here they are proven as a BOUNDARY, which is a different question: U19 asks whether
# the rule fires, S7.2 asks whether the rule is what stands between a principal and an account
# it must not be able to take.
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S7.2 — no scoped tenant was provisioned, so the two subject-derived row rules are UNPROVEN this run"
else
  S7_SELF_EMAIL=$(user_email s7self)
  S7_SELF_ID=$(new_user "$S7_SELF_EMAIL" "$QA_TENANT_SCOPED") || true
  S7_OTHER_EMAIL=$(user_email s7other)
  S7_OTHER_ID=$(new_user "$S7_OTHER_EMAIL" "$QA_TENANT_SCOPED") || true
  # The principal holds BOTH credential verbs, so every refusal below is a ROW decision and
  # never a missing permission — which is the entire distinction this block exists to draw.
  S7_CRED_ROLE=$(new_role "$(role_key s7cred)" "$QA_TENANT_SCOPED" \
    "$(permission_id_of user read)" "$(permission_id_of user change-password)" "$(permission_id_of user reset-password)") || true
  [ -n "${S7_SELF_ID:-}" ] && [ -n "${S7_CRED_ROLE:-}" ] && grant_role_to_user "$S7_SELF_ID" "$S7_CRED_ROLE" >/dev/null
  S7_SELF_T=$([ -n "${S7_SELF_ID:-}" ] && usable_token "$S7_SELF_EMAIL" "$S7_SELF_ID" || true)

  if [ -z "${S7_SELF_T:-}" ] || [ -z "${S7_OTHER_ID:-}" ]; then
    skip_ "S7.2 — the credential principal could not be provisioned, so the two subject-derived row rules are UNPROVEN this run"
  else
    case_ "S7.2a THE CHANGE on its own row" "204 — the caller holds user:change-password AND is the row"
    rotate_password "$S7_SELF_ID" "$S7_SELF_T" "$QA_USER_PASS2" 'Qa!Sec2026xy'
    assert_empty_body 204

    case_ "S7.2b THE SAME CALL against another row" "403 PasswordChangeRequiresSelfNotification — identical permission, identical body, different id. The gate on the route says who may attempt the verb; this says whose row they reached"
    api PATCH "/users/$S7_OTHER_ID/password" \
      '{"currentPassword":"Qa!Sec2026xy","password":"Qa!Steal2026x","passwordConfirmation":"Qa!Steal2026x"}' "$S7_SELF_T"
    assert_rest 403 PasswordChangeRequiresSelfNotification

    case_ "S7.2c THE RESET against another row" "204 — the mirror, and the caller holds user:reset-password"
    api PATCH "/users/$S7_OTHER_ID/password-reset" \
      '{"password":"Qa!Helped2026","passwordConfirmation":"Qa!Helped2026"}' "$S7_SELF_T"
    assert_empty_body 204

    case_ "S7.2d THE SAME CALL against its OWN row" "403 PasswordResetRequiresAnotherUserNotification"
    api PATCH "/users/$S7_SELF_ID/password-reset" \
      '{"password":"Qa!Launder26x","passwordConfirmation":"Qa!Launder26x"}' "$S7_SELF_T"
    assert_rest 403 PasswordResetRequiresAnotherUserNotification

    case_ "S7.2e A STOLEN TOKEN CANNOT LAUNDER ITSELF" "both doors closed on the SAME principal: it cannot replace its own credential without proving the previous one, by either URL. Without the reset's mirror rule, a holder of user:reset-password would simply choose the other endpoint"
    api PATCH "/users/$S7_SELF_ID/password-reset" '{"password":"Qa!Launder26x","passwordConfirmation":"Qa!Launder26x"}' "$S7_SELF_T"
    S_A="$HTTP_STATUS"
    api PATCH "/users/$S7_SELF_ID/password" '{"currentPassword":"","password":"Qa!Launder26x","passwordConfirmation":"Qa!Launder26x"}' "$S7_SELF_T"
    S_B="$HTTP_STATUS"
    if [ "$S_A" = "403" ] && [ "$S_B" = "422" ]; then pass_; else HTTP_BODY="self-reset=$S_A (want 403), change-without-proof=$S_B (want 422)"; fail_ "a door was open"; fi

    case_ "S7.2f THE ROW RULE IS NOT THE PERMISSION" "the same principal, holding both verbs, is refused on both — proving the refusals came from the aggregate and not from the gate. A suite that used a principal lacking the permission would have watched Layer 1 fire and called it Layer 2"
    api PATCH "/users/$S7_OTHER_ID/password" '{"currentPassword":"x","password":"Qa!Nope2026xx","passwordConfirmation":"Qa!Nope2026xx"}' "$S7_SELF_T"
    K_A=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
    if [ "$K_A" = "PasswordChangeRequiresSelfNotification" ]; then pass_; else HTTP_BODY="$K_A"; fail_ "keys = $K_A"; fi
  fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# S7.3 — LAYER 3: tenant scoping. The leak here is a 200, which is why it needs its own case.
#
# spec.md §16: "A user listing is the customer's staff directory INCLUDING E-MAIL ADDRESSES —
# leaking it across tenants is strictly worse than leaking the org chart, which Group already
# refused."
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_USEROP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S7.3 — principal I was not built, so User's row scoping is UNPROVEN this run"
else
  S7_FOREIGN_TEN=$(new_tenant active "$(ws s7for)") || true
  S7_FOREIGN_USER=$([ -n "${S7_FOREIGN_TEN:-}" ] && new_user "$(user_email s7for)" "$S7_FOREIGN_TEN" || true)

  case_ "S7.3a principal I's listing carries only its OWN tenant's rows" "every row's tenantID is I's own — asserted over the VALUES, not over a count, because a count passes while a single foreign row rides along"
  api GET "/users?first=100" "" "$QA_TOKEN_USEROP"
  assert_json_at 200 '[.data[].tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S7.3b THE LEAK ITSELF: I reads another tenant's user by id" "404 and NOT 403 — a 403 would confirm the id exists to a caller who may not see it. This service answers 'you may not' and 'it is not there' identically wherever telling them apart would leak"
  if [ -n "${S7_FOREIGN_USER:-}" ]; then
    api GET "/users/$S7_FOREIGN_USER" "" "$QA_TOKEN_USEROP"
    assert_status 404
  else skip_ "S7.3b — the foreign user could not be created"; fi

  case_ "S7.3c THE COMPLEMENT: the admin crosses the same scope" "200 on the very id I was refused — a scope that refuses everyone is also broken"
  if [ -n "${S7_FOREIGN_USER:-}" ]; then
    api GET "/users/$S7_FOREIGN_USER"
    assert_json_at 200 '.data.id' "$S7_FOREIGN_USER"
  else skip_ "S7.3c — the foreign user could not be created"; fi

  case_ "S7.3d THE CROSS-TENANT WRITE: I creates a user naming another tenant" "403 TenantMismatchNotification — the row's tenant must equal the caller's claim, and U1b refuses rather than silently overriding so the caller learns what happened"
  if [ -n "${S7_FOREIGN_TEN:-}" ]; then
    api POST /users "$(user_body "$(user_email s7x)" active "$S7_FOREIGN_TEN")" "$QA_TOKEN_USEROP"
    assert_rest 403 TenantMismatchNotification
  else skip_ "S7.3d — the foreign tenant could not be created"; fi

  case_ "S7.3e I ARCHIVES a user of another tenant" "403 TenantMismatchNotification — refuseForeignTenant runs under IfArchive too. The write side is NOT filtered by ToCriteria, so the row LOADS and the rule is what refuses: invisible if only reads were tested"
  if [ -n "${S7_FOREIGN_USER:-}" ]; then
    api PATCH "/users/$S7_FOREIGN_USER/archive" "" "$QA_TOKEN_USEROP"
    assert_rest 403 TenantMismatchNotification
  else skip_ "S7.3e — the foreign user could not be created"; fi

  case_ "S7.3f I omits the tenant entirely" "201 — absent means 'mine'. A tenant caller does not supply a tenant at all, they INHERIT one, which is why U4 is usually a no-op on insert"
  api POST /users "$(user_body "$(user_email s7own)" active "")" "$QA_TOKEN_USEROP"
  assert_json_at 201 '.data.tenantID' "$QA_TENANT_SCOPED"

  case_ "S7.3g THE BYPASS ACTS INSIDE ANOTHER TENANT, NEVER ACROSS TWO" "the admin creates a user in the foreign tenant and may then give it only THAT tenant's roles — the anchor is the USER's tenant, not the caller's"
  if [ -n "${S7_FOREIGN_TEN:-}" ] && [ -n "${QA_TENANT_SCOPED:-}" ]; then
    S7_MIX_ID=$(new_user "$(user_email s7mix)" "$S7_FOREIGN_TEN") || true
    S7_SCOPED_ROLE=$(new_role "$(role_key s7mix)" "$QA_TENANT_SCOPED" "$(permission_id_of tenant read)") || true
    if [ -n "${S7_MIX_ID:-}" ] && [ -n "${S7_SCOPED_ROLE:-}" ]; then
      api POST "/users/$S7_MIX_ID/roles" "$(jq -nc --arg r "$S7_SCOPED_ROLE" '{roleID:$r}')"
      assert_rest 422 RoleNotAvailableInTenantNotification
    else skip_ "S7.3g — the mixed-tenant fixtures could not be provisioned"; fi
  else skip_ "S7.3g — the foreign tenant could not be created"; fi

  case_ "S7.3h the scope holds on GRAPHQL too" "every node carries I's own tenantID — ToCriteria is shared, but a surface that skipped it would look exactly like a passing REST case"
  gql "query { users(first: 100) { edges { node { tenantID } } } }" "" "$QA_TOKEN_USEROP"
  assert_gql_ok '[.data.users.edges[].node.tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S7.3i the admin's GraphQL connection is NOT narrowed" "more than one distinct tenant — the bypass reaches this surface as well"
  gql "query { users(first: 100) { edges { node { tenantID } } } }"
  assert_gql_ok '([.data.users.edges[].node.tenantID] | unique | length) > 1' "true"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# S7.4 — field-level read authz: NONE, and the posture is asserted rather than skipped.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S7.4a no ReadCriteria.Restrict on User — the field is unreachable for EVERYONE" "the four password-oracle requests answer 400 for the ADMIN too. spec.md §9: the hash is off every surface for every caller by construction, which is STRONGER than restricting it — a restricted field is present for somebody"
S7_ORACLE_FAIL=""
for q in "passwordHash.eq=%24argon2id" "passwordHash.startswith=%24a" "orderBy=passwordHash" "fields=passwordHash"; do
  api GET "/users?$q"
  [ "$HTTP_STATUS" = "400" ] || S7_ORACLE_FAIL="$S7_ORACLE_FAIL $q→$HTTP_STATUS"
done
if [ -z "$S7_ORACLE_FAIL" ]; then pass_; else HTTP_BODY="$S7_ORACLE_FAIL"; fail_ "an oracle request did not answer 400 for the admin"; fi

case_ "S7.4b there is therefore no FieldAccessForbiddenNotification to assert" "recorded as a DECISION and not a gap — and unlike a Restrict, this boundary has no privileged side that could be widened by a claim"
skip_ "spec.md §9 declares no ReadCriteria.Restrict for User. The hash is kept off the wire by the Response DTOs declaring no such member, and out of the framework's own copies by RedactedField(InSync/InAudit) — two mechanisms, neither of which is a per-caller decision. S7.4a proves the boundary holds for the most privileged principal in the service; qa/domain.sh U25 proves the audit half"

# ═════════════════════════════════════════════════════════════════════════════════════════
# S7.5 — what this round leaves UNPROVEN, printed rather than only written in the plan.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S7.5a the user:change-password deployment story" "unproven, and named"
skip_ "spec.md §10 records that user:change-password must reach EVERY user or nobody can rotate their own credential once authorization is on. The suite proves the RESTRICTED token always carries it (qa/domain.sh U23, where the grant is EMBEDDED rather than filtered — which is what keeps the flow from deadlocking), but it does NOT prove any tenant's default role grants it, because no such default role exists in this service to inspect. S7.1n above shows the gate closing on a principal that lacks it"

case_ "S7.5b the externalValidator path" "unproven, and named"
skip_ "auth.externalValidator is configured in no profile, so a locally-valid token is never refused by a second opinion. There is nothing to assert and nothing is claimed"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════
#  S8 — §3 of specs/qa/client-contract/plan.md. The Client gate.
#
#  §3a (the 401 family), §3b (the public-route split) and the framework's appended surfaces
#  are INHERITED from the five approved rounds and are not repeated here.
#
#  What is new: EIGHT distinct literals over thirteen routes — the widest vocabulary in the
#  service, three collections each under a verb of its OWN (grant / manage-network /
#  set-claim: zero overlap, each a different blast radius) plus the hand-written
#  rotate-secret. And the first MACHINE captors: S8.2's row rule is driven by a token this
#  service minted for a client, through its own public exchange route.
# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════

S8_DEAD="01990000-dead-7000-8000-000000000000"

# ── S8.1 Layer 1 on REST: the negative, over all thirteen routes ──────────────────────────
s8_denied() { s5_denied "$@"; }

s8_denied "S8.1a principal B on the client LISTING"   GET   "/clients"                                          ""                                       "client:read"
s8_denied "S8.1b principal B on the by-id read"       GET   "/clients/$S8_DEAD"                                 ""                                       "client:read"
s8_denied "S8.1c principal B on insert"               POST  "/clients"                                          '{"name":"X","description":"A body principal B must never get to land anywhere.","status":"active"}' "client:insert"
s8_denied "S8.1d principal B on patch"                PATCH "/clients/$S8_DEAD"                                 '{"description":"A relabel principal B must never get to land."}' "client:update"
s8_denied "S8.1e principal B on archive"              PATCH "/clients/$S8_DEAD/archive"                         ""                                       "client:archive"
s8_denied "S8.1f principal B on GRANT A ROLE"         POST  "/clients/$S8_DEAD/roles"                           '{"roleID":"'"$S8_DEAD"'"}'              "client:grant"
s8_denied "S8.1g principal B on REVOKE A ROLE"        PATCH "/clients/$S8_DEAD/roles/$S8_DEAD/archive"          ""                                       "client:grant"
s8_denied "S8.1h principal B on ALLOW A RANGE"        POST  "/clients/$S8_DEAD/allowedCIDRs"                    '{"cidr":"203.0.113.0/24","label":"X"}'  "client:manage-network"
s8_denied "S8.1i principal B on REMOVE A RANGE"       PATCH "/clients/$S8_DEAD/allowedCIDRs/$S8_DEAD/archive"   ""                                       "client:manage-network"
s8_denied "S8.1j principal B on SET A CLAIM"          POST  "/clients/$S8_DEAD/claims"                          '{"claimID":"'"$S8_DEAD"'","value":"x"}' "client:set-claim"
s8_denied "S8.1k principal B on CHANGE A CLAIM"       PATCH "/clients/$S8_DEAD/claims/$S8_DEAD"                 '{"value":"x"}'                          "client:set-claim"
s8_denied "S8.1l principal B on WITHDRAW A CLAIM"     PATCH "/clients/$S8_DEAD/claims/$S8_DEAD/archive"         ""                                       "client:set-claim"
s8_denied "S8.1m principal B on ROTATE A SECRET"      POST  "/clients/$S8_DEAD/secret"                          '{}'                                     "client:rotate-secret"

case_ "S8.1n THE COUNT: eight distinct literals gate thirteen routes" "client:read/insert/update/archive/grant/manage-network/set-claim/rotate-secret — a route that lost its RequirePermission would answer something other than 403 above, and a collection route that borrowed a neighbour's literal would show up here"
GATED=$(printf '%s\n' "client:read" "client:read" "client:insert" "client:update" "client:archive" "client:grant" "client:grant" "client:manage-network" "client:manage-network" "client:set-claim" "client:set-claim" "client:set-claim" "client:rotate-secret" | sort -u | wc -l | tr -d ' ')
if [ "$GATED" = "8" ]; then pass_; else fail_ "distinct literals asserted above = $GATED"; fi

# ── S8.1o-u the complement: a gate that refuses everyone is also broken ───────────────────
#
# For the writes the target id addresses nothing ON PURPOSE: reaching the HANDLER — a 404,
# never a 403 — is what proves the gate opened.

if [ -z "${QA_TOKEN_CLIENTOP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S8.1o-u — principal K was not built, so the Layer-1 complement is UNPROVEN this run and only the negative half above stands"
else
  case_ "S8.1o principal K on the client LISTING" "200 — it holds client:read"
  api GET "/clients?first=1" "" "$QA_TOKEN_CLIENTOP"
  assert_status 200

  case_ "S8.1p principal K on insert" "201 — it holds client:insert"
  new_client "$(client_label s8k)" "" "$QA_TOKEN_CLIENTOP" || true
  S8_TARGET="$CLIENT_ID"
  if [ -n "$S8_TARGET" ]; then pass_; else fail_ "K could not create a client"; fi

  case_ "S8.1q principal K on patch" "200 — it holds client:update"
  if [ -n "${S8_TARGET:-}" ]; then
    api PATCH "/clients/$S8_TARGET" '{"description":"A relabel by the operator principal, proving the gate opens for the verb it holds."}' "$QA_TOKEN_CLIENTOP"
    assert_status 200
  else skip_ "S8.1q — the target could not be created"; fi

  case_ "S8.1r principal K on a role grant" "404 and NOT 403 — the gate opened and the handler was reached; the id addresses nothing on purpose"
  api POST "/clients/$S8_DEAD/roles" "$(jq -nc --arg r "$S8_DEAD" '{roleID:$r}')" "$QA_TOKEN_CLIENTOP"
  assert_status_not 403

  case_ "S8.1s principal K on a range add" "404 and NOT 403 — client:manage-network is a literal of its own, and this is what proves K holds it separately from client:grant"
  api POST "/clients/$S8_DEAD/allowedCIDRs" '{"cidr":"203.0.113.0/24","label":"QA gate probe"}' "$QA_TOKEN_CLIENTOP"
  assert_status_not 403

  case_ "S8.1t principal K on a claim add" "404 and NOT 403 — the third literal, held separately from the other two"
  api POST "/clients/$S8_DEAD/claims" "$(jq -nc --arg c "$S8_DEAD" '{claimID:$c, value:"x"}')" "$QA_TOKEN_CLIENTOP"
  assert_status_not 403

  case_ "S8.1u principal K on THE ROTATION" "200 — it holds client:rotate-secret, and the hand-written route's gate opens exactly like the generated ones'"
  if [ -n "${S8_TARGET:-}" ]; then
    rotate_secret "$S8_TARGET" '{}' "$QA_TOKEN_CLIENTOP"
    assert_status 200
  else skip_ "S8.1u — the target could not be created"; fi
fi

# ── S8.1v-y THE SPLIT: principal L, the only caller for which the eight verbs differ ──────
#
# L holds client:read and client:update and NONE of the other six. A and B answer identically
# on all thirteen routes either way; L is what makes the split visible — "may fix a typo in a
# description" and "may confer privilege" and "may open the credential to the internet" and
# "may hand out a new production credential" are four different jobs, and §10 of the spec
# argues each split on its own blast radius.

if [ -z "${QA_TOKEN_CLIENTLIM:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S8.1v-y — principal L was not built, so the eight-way verb split is UNPROVEN this run"
else
  new_client "$(client_label s8l)" "$QA_TENANT_SCOPED" || true
  S8_L_TARGET="$CLIENT_ID"

  if [ -z "${S8_L_TARGET:-}" ]; then
    skip_ "S8.1v-y — the split's target client could not be created"
  else
    case_ "S8.1v principal L MAY relabel" "200 — client:update is the verb it holds, and it must keep working"
    api PATCH "/clients/$S8_L_TARGET" '{"description":"A relabel by the limited principal, the one verb it is meant to keep."}' "$QA_TOKEN_CLIENTLIM"
    assert_status 200

    s8l_denied() { # s8l_denied CASE METHOD PATH BODY LITERAL WHY
      case_ "$1" "403 MissingPermissionNotification, value '$5' — whoever may fix a typo in a description must not be able to $6"
      api "$2" "$3" "$4" "$QA_TOKEN_CLIENTLIM"
      local hit
      hit=$(printf '%s' "$HTTP_BODY" | jq -r --arg v "$5" \
        '[.errors[]?.messages[]? | select(.notificationKey=="MissingPermissionNotification" and .field=="permission" and .value==$v)] | length' 2>/dev/null)
      if [ "$HTTP_STATUS" = "403" ] && [ "${hit:-0}" -ge 1 ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no MissingPermission on '$5'"; fi
    }

    s8l_denied "S8.1w1 principal L on GRANT A ROLE"    POST  "/clients/$S8_L_TARGET/roles" "$(jq -nc --arg r "$S8_DEAD" '{roleID:$r}')" "client:grant" "confer privilege"
    s8l_denied "S8.1w2 principal L on REVOKE A ROLE"   PATCH "/clients/$S8_L_TARGET/roles/$S8_DEAD/archive" "" "client:grant" "withdraw it either"
    s8l_denied "S8.1w3 principal L on ALLOW A RANGE"   POST  "/clients/$S8_L_TARGET/allowedCIDRs" '{"cidr":"203.0.113.0/24","label":"QA split probe"}' "client:manage-network" "touch the network boundary"
    s8l_denied "S8.1w4 principal L on REMOVE A RANGE"  PATCH "/clients/$S8_L_TARGET/allowedCIDRs/$S8_DEAD/archive" "" "client:manage-network" "open the credential to the internet by archiving the last range"
    s8l_denied "S8.1w5 principal L on SET A CLAIM"     POST  "/clients/$S8_L_TARGET/claims" "$(jq -nc --arg c "$S8_DEAD" '{claimID:$c, value:"x"}')" "client:set-claim" "set a value this service cannot audit the consequences of"
    s8l_denied "S8.1w6 principal L on CHANGE A CLAIM"  PATCH "/clients/$S8_L_TARGET/claims/$S8_DEAD" '{"value":"x"}' "client:set-claim" "correct one either"
    s8l_denied "S8.1w7 principal L on WITHDRAW A CLAIM" PATCH "/clients/$S8_L_TARGET/claims/$S8_DEAD/archive" "" "client:set-claim" "withdraw one either"
    s8l_denied "S8.1x  principal L on ROTATE A SECRET" POST  "/clients/$S8_L_TARGET/secret" '{}' "client:rotate-secret" "hand out a new production credential"
    s8l_denied "S8.1x2 principal L on INSERT"          POST  "/clients" '{"name":"X","description":"A creation the limited principal must be refused.","status":"active"}' "client:insert" "mint a machine account"
    s8l_denied "S8.1x3 principal L on ARCHIVE"         PATCH "/clients/$S8_L_TARGET/archive" "" "client:archive" "revoke one"

    case_ "S8.1y client:update DELIBERATELY DOES NOT REACH THE ROTATION" "the relabel passed and the rotation was refused, same principal, same row — an operator who may fix a typo is not thereby an operator who may hand out a new production credential (spec.md §10, the user:reset-password argument applied here)"
    api PATCH "/clients/$S8_L_TARGET" '{"description":"Still relabelling, still allowed."}' "$QA_TOKEN_CLIENTLIM"; S_REN="$HTTP_STATUS"
    rotate_secret "$S8_L_TARGET" '{}' "$QA_TOKEN_CLIENTLIM"; S_ROT="$HTTP_STATUS"
    if [ "$S_REN" = "200" ] && [ "$S_ROT" = "403" ]; then pass_; else HTTP_BODY="relabel=$S_REN rotate=$S_ROT"; fail_ "the two verbs did not separate"; fi
  fi
fi

# ── S8.1gql Layer 1 on GRAPHQL. A route gated on REST is not thereby gated here ───────────
s8_gql_denied() { s5_gql_denied "$@"; }

s8_gql_denied "S8.1gql-a principal B on the clients connection"   "query { clients(first: 1) { totalCount } }" "" "client:read"
s8_gql_denied "S8.1gql-b principal B on client(id:)"              "query { client(id: \"$S8_DEAD\") { id } }" "" "client:read"
s8_gql_denied "S8.1gql-c principal B on createClient"             "mutation(\$i: CreateClientInput!) { createClient(input: \$i) { id } }" '{"i":{"name":"X","description":"A body principal B must never get to land anywhere.","status":"active"}}' "client:insert"
s8_gql_denied "S8.1gql-d principal B on patchClient"              "mutation(\$i: PatchClientInput!) { patchClient(id: \"$S8_DEAD\", input: \$i) { id } }" '{"i":{"description":"A relabel principal B must never get to land."}}' "client:update"
s8_gql_denied "S8.1gql-e principal B on archiveClient"            "mutation { archiveClient(id: \"$S8_DEAD\") { success } }" "" "client:archive"
s8_gql_denied "S8.1gql-f principal B on addClientRole"            "mutation(\$i: AddClientRoleInput!) { addClientRole(id: \"$S8_DEAD\", input: \$i) { clientId } }" '{"i":{"roleID":"'"$S8_DEAD"'"}}' "client:grant"
s8_gql_denied "S8.1gql-g principal B on archiveClientRole"        "mutation(\$i: ArchiveClientRoleInput!) { archiveClientRole(id: \"$S8_DEAD\", input: \$i) { success } }" '{"i":{"clientRoleId":"'"$S8_DEAD"'"}}' "client:grant"
s8_gql_denied "S8.1gql-h principal B on addClientAllowedCIDR"     "mutation(\$i: AddClientAllowedCIDRInput!) { addClientAllowedCIDR(id: \"$S8_DEAD\", input: \$i) { clientId } }" '{"i":{"cidr":"203.0.113.0/24","label":"X"}}' "client:manage-network"
s8_gql_denied "S8.1gql-i principal B on archiveClientAllowedCIDR" "mutation(\$i: ArchiveClientAllowedCIDRInput!) { archiveClientAllowedCIDR(id: \"$S8_DEAD\", input: \$i) { success } }" '{"i":{"clientAllowedCIDRId":"'"$S8_DEAD"'"}}' "client:manage-network"
s8_gql_denied "S8.1gql-j principal B on addClientClaim"           "mutation(\$i: AddClientClaimInput!) { addClientClaim(id: \"$S8_DEAD\", input: \$i) { clientId } }" '{"i":{"claimID":"'"$S8_DEAD"'","value":"x"}}' "client:set-claim"
s8_gql_denied "S8.1gql-k principal B on patchClientClaim"         "mutation(\$i: PatchClientClaimInput!) { patchClientClaim(id: \"$S8_DEAD\", input: \$i) { clientId } }" '{"i":{"clientClaimId":"'"$S8_DEAD"'","value":"x"}}' "client:set-claim"
s8_gql_denied "S8.1gql-l principal B on archiveClientClaim"       "mutation(\$i: ArchiveClientClaimInput!) { archiveClientClaim(id: \"$S8_DEAD\", input: \$i) { success } }" '{"i":{"clientClaimId":"'"$S8_DEAD"'"}}' "client:set-claim"
s8_gql_denied "S8.1gql-m principal B on rotateClientSecret"       "mutation(\$i: RotateClientSecretInput!) { rotateClientSecret(id: \"$S8_DEAD\", input: \$i) { id } }" '{"i":{}}' "client:rotate-secret"

case_ "S8.1gql-n the admin's GraphQL connection is served" "200 with no errors — the per-surface complement"
gql "query { clients(first: 1) { totalCount edges { node { id } } } }"
assert_gql_ok '(.data.clients.totalCount >= 0)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# S8.2 — LAYER 2: the identity-derived row rule, driven by a MACHINE token. Two calls that
#        differ ONLY in whose row was reached. qa/domain.sh C14b proves the rule as a
#        business decision; here it is proven as the BOUNDARY between one machine and its
#        sibling's credential.
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_CLIENTOP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S8.2 — principal K was not built, so the machine row rule is UNPROVEN this run"
else
  S8_ROT_ROLE=$(new_role "$(role_key s8rot)" "$QA_TENANT_SCOPED" \
    "$(permission_id_of client read)" "$(permission_id_of client rotate-secret)") || S8_ROT_ROLE=""
  S8_MA_TOK=""; S8_MB_ID=""
  if [ -n "$S8_ROT_ROLE" ]; then
    api POST /clients "$(jq -nc --arg n "$(client_label s8ma)" --arg t "$QA_TENANT_SCOPED" --arg r "$S8_ROT_ROLE" \
      '{name:$n, description:"Machine A of the S8.2 boundary: it holds client:rotate-secret and must still be prisoner of its own row.", status:"active", tenantID:$t, roles:[{roleID:$r}]}')"
    S8_MA_ID=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
    S8_MA_SECRET=$(printf '%s' "$HTTP_BODY" | jq -r '.data.secret // empty')
    new_client "$(client_label s8mb)" "$QA_TENANT_SCOPED" && S8_MB_ID="$CLIENT_ID"
    [ -n "$S8_MA_ID" ] && S8_MA_TOK=$(mint_client_token "$S8_MA_ID" "$S8_MA_SECRET")
  fi

  if [ -z "$S8_MA_TOK" ] || [ -z "$S8_MB_ID" ]; then
    skip_ "S8.2 — the machine principals could not be provisioned, so the row rule boundary is UNPROVEN this run"
  else
    case_ "S8.2a THE ROTATION on the machine's own row" "200 — the caller holds client:rotate-secret AND is the row"
    rotate_secret "$S8_MA_ID" '{}' "$S8_MA_TOK"
    assert_status 200

    case_ "S8.2b THE SAME CALL against the sibling's row" "403 ClientMayOnlyRotateItsOwnSecretNotification — identical permission, identical body, different id: a machine that can rotate another machine's secret can lock it out and take its place. The gate on the route says who may attempt the verb; this says whose row they reached"
    rotate_secret "$S8_MB_ID" '{}' "$S8_MA_TOK"
    assert_rest 403 ClientMayOnlyRotateItsOwnSecretNotification

    case_ "S8.2c THE ROW RULE IS NOT THE PERMISSION" "the refusal names the row rule alone — a suite using a machine that lacked the permission would have watched Layer 1 fire and called it Layer 2"
    K_S8=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
    if [ "$K_S8" = "ClientMayOnlyRotateItsOwnSecretNotification" ]; then pass_; else HTTP_BODY="$K_S8"; fail_ "keys = $K_S8"; fi

    case_ "S8.2d the rule STANDS DOWN for a person" "200 — principal K's USER token rotates the sibling: the boundary narrows only when identity_kind positively says client"
    rotate_secret "$S8_MB_ID" '{}' "$QA_TOKEN_CLIENTOP"
    assert_status 200
  fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# S8.3 — LAYER 3: tenant scoping. The leak here is a 200, which is why it needs its own case
#        — a client listing is the customer's INTEGRATION INVENTORY, and each row is a
#        credential's identity half.
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_CLIENTOP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "S8.3 — principal K was not built, so Client's row scoping is UNPROVEN this run"
else
  S8_FOREIGN_TEN=$(new_tenant active "$(ws s8for)") || true
  S8_FOREIGN_CLIENT=""
  if [ -n "${S8_FOREIGN_TEN:-}" ]; then
    new_client "$(client_label s8for)" "$S8_FOREIGN_TEN" && S8_FOREIGN_CLIENT="$CLIENT_ID"
  fi

  case_ "S8.3a principal K's listing carries only its OWN tenant's clients" "every row's tenantID is K's own — asserted over the VALUES, not over a count"
  api GET "/clients?first=100" "" "$QA_TOKEN_CLIENTOP"
  assert_json_at 200 '[.data[].tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S8.3b THE LEAK ITSELF: K reads another tenant's client by id" "404 and NOT 403 — a 403 would confirm the id exists, and a client id is the sub every one of its tokens presents"
  if [ -n "$S8_FOREIGN_CLIENT" ]; then
    api GET "/clients/$S8_FOREIGN_CLIENT" "" "$QA_TOKEN_CLIENTOP"
    assert_status 404
  else skip_ "S8.3b — the foreign client could not be created"; fi

  case_ "S8.3c THE COMPLEMENT: the admin crosses the same scope" "200 on the very id K was refused — a scope that refuses everyone is also broken"
  if [ -n "$S8_FOREIGN_CLIENT" ]; then
    api GET "/clients/$S8_FOREIGN_CLIENT"
    assert_json_at 200 '.data.id' "$S8_FOREIGN_CLIENT"
  else skip_ "S8.3c — the foreign client could not be created"; fi

  case_ "S8.3d THE CROSS-TENANT WRITE: K creates a client naming another tenant" "403 TenantMismatchNotification — refused rather than silently overridden, so the caller learns what happened"
  if [ -n "${S8_FOREIGN_TEN:-}" ]; then
    api POST /clients "$(client_body "$(client_label s8x)" "$S8_FOREIGN_TEN")" "$QA_TOKEN_CLIENTOP"
    assert_rest 403 TenantMismatchNotification
  else skip_ "S8.3d — the foreign tenant could not be created"; fi

  case_ "S8.3e K ARCHIVES a client of another tenant" "refused — the row is not K's to revoke, whichever shape the refusal takes (403 from the rule or 404 from the scope, and never a 204)"
  if [ -n "$S8_FOREIGN_CLIENT" ]; then
    api PATCH "/clients/$S8_FOREIGN_CLIENT/archive" "" "$QA_TOKEN_CLIENTOP"
    if [ "$HTTP_STATUS" = "403" ] || [ "$HTTP_STATUS" = "404" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi
  else skip_ "S8.3e — the foreign client could not be created"; fi

  case_ "S8.3f THE BYPASS ACTS INSIDE ANOTHER TENANT, NEVER ACROSS TWO" "the admin creates a client in the foreign tenant and may then grant it only THAT tenant's roles — the anchor is the CLIENT's tenant, not the caller's"
  if [ -n "${S8_FOREIGN_TEN:-}" ]; then
    new_client "$(client_label s8mix)" "$S8_FOREIGN_TEN" || true
    S8_MIX_ID="$CLIENT_ID"
    S8_SCOPED_ROLE=$(new_role "$(role_key s8mix)" "$QA_TENANT_SCOPED" "$(permission_id_of tenant read)") || true
    if [ -n "${S8_MIX_ID:-}" ] && [ -n "${S8_SCOPED_ROLE:-}" ]; then
      api POST "/clients/$S8_MIX_ID/roles" "$(jq -nc --arg r "$S8_SCOPED_ROLE" '{roleID:$r}')"
      assert_rest 422 RoleNotAvailableInTenantNotification
    else skip_ "S8.3f — the mixed-tenant fixtures could not be provisioned"; fi
  else skip_ "S8.3f — the foreign tenant could not be created"; fi

  case_ "S8.3g the scope holds on GRAPHQL too" "every node carries K's own tenantID — ToCriteria is shared, but a surface that skipped it would look exactly like a passing REST case"
  gql "query { clients(first: 100) { edges { node { tenantID } } } }" "" "$QA_TOKEN_CLIENTOP"
  assert_gql_ok '[.data.clients.edges[].node.tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "S8.3h the admin's GraphQL connection is NOT narrowed" "more than one distinct tenant — the bypass reaches this surface as well"
  gql "query { clients(first: 100) { edges { node { tenantID } } } }"
  assert_gql_ok '([.data.clients.edges[].node.tenantID] | unique | length) > 1' "true"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# S8.4 — field-level read authz: NONE, and the posture is asserted rather than skipped.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S8.4a no ReadCriteria.Restrict on Client — the credential is unreachable for EVERYONE" "the oracle requests answer 400 for the ADMIN too: both hash columns and the rules-only tenantStatus are off every surface for every caller by construction, which is STRONGER than restricting them — a restricted field is present for somebody"
S8_ORACLE_FAIL=""
for q in "secretHash.eq=9f86" "previousSecretHash.startswith=9f" "orderBy=secretHash" "fields=secretHash" "fields=previousSecretHash" "fields=secret" "tenantStatus.eq=active" "fields=tenantStatus"; do
  api GET "/clients?$q"
  [ "$HTTP_STATUS" = "400" ] || S8_ORACLE_FAIL="$S8_ORACLE_FAIL $q→$HTTP_STATUS"
done
if [ -z "$S8_ORACLE_FAIL" ]; then pass_; else HTTP_BODY="$S8_ORACLE_FAIL"; fail_ "an oracle request did not answer 400 for the admin"; fi

case_ "S8.4b there is therefore no FieldAccessForbiddenNotification to assert" "recorded as a DECISION and not a gap"
skip_ "spec.md §9 declares no ReadCriteria.Restrict for Client: the hashes are off the wire because no Response DTO declares them (and RedactedField keeps them out of the framework's own copies — qa/audit.sh A69+ proves that half), and tenantStatus is hidden at the join. S8.4a proves the boundary holds for the most privileged principal in the service"

# ═════════════════════════════════════════════════════════════════════════════════════════
# S8.5 — the MINT GATE gives one generic answer, whatever failed. The route is public by
#        design, so its refusals must confirm nothing — not which half of the credential was
#        wrong, not whether the address was the problem, not whether the client is suspended.
# ═════════════════════════════════════════════════════════════════════════════════════════

S8_MINT_TEN=$(new_tenant active "$(ws s8mnt)") || true
if [ -z "${S8_MINT_TEN:-}" ]; then
  skip_ "S8.5 — the mint-gate tenant could not be provisioned"
else
  new_client "$(client_label s8mint)" "$S8_MINT_TEN" || true
  S8_MINT_ID="$CLIENT_ID"; S8_MINT_SECRET="$CLIENT_SECRET"
  if [ -z "${S8_MINT_ID:-}" ]; then
    skip_ "S8.5 — the mint-gate client could not be provisioned"
  else
    case_ "S8.5a a WRONG secret and a SUSPENDED client answer IDENTICALLY" "the same 401 and the same notification keys — an attacker probing the mint route learns nothing about WHICH check refused them"
    api POST /auth/client/token "$(jq -nc --arg i "$S8_MINT_ID" '{clientId:$i, clientSecret:"acs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}')" -
    S_WRONG="$HTTP_STATUS"; K_WRONG=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")' 2>/dev/null)
    api PATCH "/clients/$S8_MINT_ID" '{"status":"suspended"}'
    api POST /auth/client/token "$(jq -nc --arg i "$S8_MINT_ID" --arg s "$S8_MINT_SECRET" '{clientId:$i, clientSecret:$s}')" -
    S_SUSP="$HTTP_STATUS"; K_SUSP=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")' 2>/dev/null)
    if [ "$S_WRONG" = "401" ] && [ "$S_SUSP" = "401" ] && [ "$K_WRONG" = "$K_SUSP" ]; then pass_; else HTTP_BODY="wrong-secret=$S_WRONG[$K_WRONG] suspended=$S_SUSP[$K_SUSP]"; fail_ "the two refusals are distinguishable"; fi

    case_ "S8.5b an UNKNOWN client id answers the same way" "the same pair again — the mint route is not an existence oracle over client ids either"
    api POST /auth/client/token "$(jq -nc '{clientId:"01990000-dead-7000-8000-000000000000", clientSecret:"acs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}')" -
    S_GHOST="$HTTP_STATUS"; K_GHOST=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")' 2>/dev/null)
    if [ "$S_GHOST" = "401" ] && [ "$K_GHOST" = "$K_WRONG" ]; then pass_; else HTTP_BODY="ghost=$S_GHOST[$K_GHOST] vs wrong-secret=[$K_WRONG]"; fail_ "an unknown id is distinguishable from a wrong secret"; fi
  fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# S8.6 — what this round leaves UNPROVEN, printed rather than only written in the plan.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "S8.6a the trusted-proxy half of the allow-list" "unproven, and named"
skip_ "No profile configures a trusted proxy, so the mint judges the SOCKET address — which is what made C-MINT provable from localhost. Whether a spoofed X-Forwarded-For could walk through a REAL deployment behind a load balancer is /omnicore:configure territory (spec.md §F prerequisite 1), and until it is configured the allow-list constrains local semantics only. Prerequisite 2 stands with it: the list constrains where a token is OBTAINED, never where it is USED"

case_ "S8.6b the client token's own contract" "unproven here, and deferred by decision"
skip_ "The claim vocabulary (identity_kind, name, permissions, x_* values reaching the token), the deliberate absence of a lockout on this route, and the absent /refresh companion belong to the token route's own round (plan §0b, maintainer 2026-09-08: exercised, not owned). C-SEC1 proves the one bit this round cannot avoid: a minted secret signs in and its token says client"

qa_finish
