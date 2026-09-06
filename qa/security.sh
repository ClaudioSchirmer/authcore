#!/usr/bin/env bash
# Lane: security — §3 of specs/qa/tenant-contract/plan.md.
#
# Every other lane proves the service DOES what it should. This one proves it
# REFUSES what it should. 401 and 403 are the two answers a caller must be able
# to rely on, and they are the ones a regression turns into a 200 silently — a
# publicRoutes entry widened past what it meant to open, a RequirePermission lost
# when a route was re-mounted. None of that breaks a single happy-path case.
#
# It is its own lane because it needs a different setup: forged tokens, a
# throwaway keypair, and a principal deliberately holding no permission.
#
# WHERE THE TOKENS COME FROM, stated plainly:
#   · valid   — this service's own documented flow (auth.issuer.enabled: true).
#               POST /auth/user/token. Nothing is invented.
#   · refused — signed HERE, at run time. A token meant to FAIL needs no secret
#               from anybody, and asserting the refusal is the entire point. The
#               ones that must carry a VALID signature (wrong iss, wrong aud,
#               expired) are signed with the bench's own dev key, because
#               otherwise the refusal could be the signature rather than the
#               claim under test — which would prove nothing.

cd "$(dirname "$0")/.." || exit 2
LANE_NAME=security
. qa/lib.bash

: "${QA_SIGNING_KEY_FILE:=devops/dev-signing-key.pem}"
JWT_ISS="${AUTH_SELF_URL:-http://localhost:${QA_PORT}}"
JWT_AUD="${AUTH_AUDIENCE:-authcore}"
KID="${JWT_SIGNING_KID:-dev}"
FOREIGN_KEY="${LOG_DIR}/foreign-key.pem"

command -v openssl >/dev/null || { printf 'precondition: openssl is required to forge the refused tokens\n'; exit 2; }
[ -f "$QA_SIGNING_KEY_FILE" ] || { printf 'precondition: signing key %s not found\n' "$QA_SIGNING_KEY_FILE"; exit 2; }

# ── JWT construction ───────────────────────────────────────────────────────
b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

# mint_rs256 KEYFILE HEADER_JSON PAYLOAD_JSON
mint_rs256() {
  local key="$1" h p signing sig
  h="$(printf '%s' "$2" | b64url)"; p="$(printf '%s' "$3" | b64url)"
  signing="${h}.${p}"
  sig="$(printf '%s' "$signing" | openssl dgst -sha256 -sign "$key" -binary | b64url)"
  printf '%s.%s' "$signing" "$sig"
}

mint_hs256() {
  local secret="$1" h p signing sig
  h="$(printf '%s' "$2" | b64url)"; p="$(printf '%s' "$3" | b64url)"
  signing="${h}.${p}"
  sig="$(printf '%s' "$signing" | openssl dgst -sha256 -hmac "$secret" -binary | b64url)"
  printf '%s.%s' "$signing" "$sig"
}

rs_header() { jq -nc --arg kid "$1" '{alg:"RS256", typ:"JWT", kid:$kid}'; }

# claims ISS AUD EXP → a payload otherwise well-formed, so each case varies ONE thing.
claims() {
  jq -nc --arg iss "$1" --arg aud "$2" --argjson exp "$3" --argjson iat "$(date +%s)" \
     --arg sub '01990000-0004-7000-8000-000000000001' \
     --arg tid '01990000-0001-7000-8000-000000000001' \
    '{iss:$iss, aud:$aud, sub:$sub, exp:$exp, iat:$iat, tenant_id:$tid, permissions:["*:*"]}'
}

NOW=$(date +%s); FUTURE=$((NOW + 3600)); PAST=$((NOW - 7200))

# ── the real token, from the service's own flow ────────────────────────────
sign_in "$BOOTSTRAP_EMAIL" "$BOOTSTRAP_INITIAL_PASSWORD"
[ "$RESP_CODE" = "200" ] || sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
[ "$RESP_CODE" = "200" ] || { printf 'precondition: could not sign in (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }
USER_ID="$(j '.data.user.id')"; TOKEN="$(j '.data.accessToken')"; TENANT_ID="$(j '.data.user.tenantId')"
if [ "$(j '.data.user.mustChangePassword')" = "true" ]; then
  req PATCH "/users/${USER_ID}/password" \
    "$(jq -nc --arg c "$BOOTSTRAP_INITIAL_PASSWORD" --arg p "$QA_ADMIN_PASSWORD" \
       '{currentPassword:$c, password:$p, passwordConfirmation:$p}')"
  sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
  TOKEN="$(j '.data.accessToken')"; TENANT_ID="$(j '.data.user.tenantId')"
fi
ADMIN_TOKEN="$TOKEN"
[ -n "$ADMIN_TOKEN" ] || { printf 'precondition: no access token\n'; exit 2; }

# ════════════════════════════════════════════════════════════════════════════
section "3a · the 401 half — every case is a token meant to FAIL"
note "the KEY is asserted, not just the status: the pin splits expired from invalid"
note "precisely so a client can branch on refresh-vs-reauthenticate"

req_noauth GET /tenants
assert_status_key "S1 no Authorization header at all" 401 'MissingAuthorizationNotification'

req_rawauth "Token ${ADMIN_TOKEN}" GET /tenants
assert_status_key "S2 a scheme that is not Bearer" 401 'MissingAuthorizationNotification'

req_rawauth "Bearer" GET /tenants
assert_status_key "S3 the Bearer scheme with an empty value" 401 'MissingAuthorizationNotification'

# The scheme match is documented as case-insensitive, so this is a PASS case.
# A suite that asserted 401 here would be pinning a bug as the contract.
req_rawauth "bearer ${ADMIN_TOKEN}" GET /tenants
assert_status "S4 a lowercase 'bearer' scheme is accepted (match is case-insensitive)" 200

req_rawauth "Bearer not-a-jwt-at-all" GET /tenants
assert_status_key "S5 a bearer value that is not a JWT" 401 'InvalidTokenNotification'

# A well-formed JWT signed with a key the service has never seen. Everything else
# about it is correct, so only the signature can be what refuses it.
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$FOREIGN_KEY" 2>/dev/null
FOREIGN_TOKEN="$(mint_rs256 "$FOREIGN_KEY" "$(rs_header "$KID")" "$(claims "$JWT_ISS" "$JWT_AUD" "$FUTURE")")"
req_rawauth "Bearer ${FOREIGN_TOKEN}" GET /tenants
assert_status_key "S6 a well-formed JWT signed with a foreign key" 401 'InvalidTokenNotification'

# S7–S10 carry a VALID signature from the bench's own key. That is what makes
# them prove the claim under test: with a bad signature the refusal would be the
# signature, and the iss/aud/exp guards would never be reached.
WRONG_ISS="$(mint_rs256 "$QA_SIGNING_KEY_FILE" "$(rs_header "$KID")" "$(claims 'https://attacker.example' "$JWT_AUD" "$FUTURE")")"
req_rawauth "Bearer ${WRONG_ISS}" GET /tenants
assert_status_key "S7 a correctly-signed token with the wrong iss" 401 'InvalidTokenNotification'

WRONG_AUD="$(mint_rs256 "$QA_SIGNING_KEY_FILE" "$(rs_header "$KID")" "$(claims "$JWT_ISS" 'some-other-service' "$FUTURE")")"
req_rawauth "Bearer ${WRONG_AUD}" GET /tenants
assert_status_key "S8 a correctly-signed token whose aud is not ours" 401 'InvalidTokenNotification'

# The algorithm-confusion guard. The allowlist is [RS256]; an HS256 token asks the
# validator to treat the public key as a shared secret. This is the classic
# attack, and asserting it is the point of having an allowlist at all.
HS_HEADER='{"alg":"HS256","typ":"JWT"}'
HS_TOKEN="$(mint_hs256 'any-secret-at-all' "$HS_HEADER" "$(claims "$JWT_ISS" "$JWT_AUD" "$FUTURE")")"
req_rawauth "Bearer ${HS_TOKEN}" GET /tenants
assert_status_key "S9 an alg outside the allowlist (HS256 where only RS256 is admitted)" 401 'InvalidTokenNotification'

EXPIRED="$(mint_rs256 "$QA_SIGNING_KEY_FILE" "$(rs_header "$KID")" "$(claims "$JWT_ISS" "$JWT_AUD" "$PAST")")"
req_rawauth "Bearer ${EXPIRED}" GET /tenants
# Expired is a DISTINCT key from invalid. A case accepting either proves neither.
assert_status_key "S10 an exp in the past — expired, not merely invalid" 401 'ExpiredTokenNotification'

# ════════════════════════════════════════════════════════════════════════════
section "3b · the public-route split — BOTH directions"
note "matching is exact METHOD /path, never prefix-based"

# Direction one: every declared public route answers tokenless. A 401 here is the
# regression; a 422 on credentials still proves the route was not gated.
req_noauth GET /livez
assert_status "S11a GET /livez is public (the project opted the probes in)" 200
req_noauth GET /readyz
assert_status "S11b GET /readyz is public" 200
req_noauth POST /auth/user/token '{"email":"nobody@nowhere.test","password":"wrong"}'
if [ "$RESP_CODE" = "401" ] && [ "$(nkey)" = "MissingAuthorizationNotification" ]; then
  fail "S11c POST /auth/user/token is public" "the route reached without a bearer" \
       "the middleware refused it — the login route is gated" "$RESP_BODY"
else
  pass "S11c POST /auth/user/token is public (HTTP $RESP_CODE — the handler answered, not the middleware)"
fi
req_noauth POST /auth/user/token/refresh '{"refreshToken":"nope"}'
if [ "$RESP_CODE" = "401" ] && [ "$(nkey)" = "MissingAuthorizationNotification" ]; then
  fail "S11d POST /auth/user/token/refresh is public" "reached without a bearer" "the middleware refused it" "$RESP_BODY"
else
  pass "S11d POST /auth/user/token/refresh is public (HTTP $RESP_CODE)"
fi
req_noauth POST /auth/client/token '{"clientId":"nope","clientSecret":"nope"}'
if [ "$RESP_CODE" = "401" ] && [ "$(nkey)" = "MissingAuthorizationNotification" ]; then
  fail "S11e POST /auth/client/token is public" "reached without a bearer" "the middleware refused it" "$RESP_BODY"
else
  pass "S11e POST /auth/client/token is public (HTTP $RESP_CODE)"
fi

# Direction two — the one that catches a publicRoutes entry widened past its
# intent. It is cheap and it is the half nobody writes.
req_noauth GET /tenants
assert_status_key "S12 a route that is NOT declared answers 401 tokenless" 401 'MissingAuthorizationNotification'
req_noauth POST /tenants '{"name":"Anon","workspace":"anon-write","description":"An anonymous write attempt that must be refused.","status":"active"}'
assert_status_key "S12b nor may an anonymous caller WRITE" 401 'MissingAuthorizationNotification'

# Exactness. A sibling that "looks covered" is not covered, because the match is
# not prefix-based — this is the trap worth pinning.
req_noauth POST /livez
if [ "$RESP_CODE" = "200" ]; then
  fail "S13a POST /livez (same path, other method)" "not served tokenless — the entry is GET /livez" "HTTP 200" "$RESP_BODY"
else
  pass "S13a POST /livez is not covered by the GET entry (HTTP $RESP_CODE)"
fi
req_noauth GET /livez/extra
if [ "$RESP_CODE" = "200" ]; then
  fail "S13b GET /livez/extra (sibling sharing a prefix)" "not served tokenless" "HTTP 200" "$RESP_BODY"
else
  pass "S13b a sibling sharing a prefix is not covered (HTTP $RESP_CODE)"
fi

# The framework's own appended surfaces — public by rule, needing no entry.
req_noauth GET /openapi.json
assert_status "S14a /openapi.json is reachable tokenless" 200
req_noauth GET /docs
assert_status "S14b the OpenAPI page is reachable tokenless" 200
req_noauth GET /graphql/ui
assert_status "S14c the GraphQL playground is reachable tokenless" 200
req_noauth GET /.well-known/jwks.json
assert_status "S14d the JWKS document is reachable tokenless" 200
# rootRedirect is on, so GET / is framework-appended to the bypass list. It may
# answer 200 or a 3xx depending on how curl is asked to follow it; what it must
# NEVER answer is 401.
req_noauth GET /
case "$RESP_CODE" in
  401) fail "S14e GET / answers tokenless" "any answer but 401 — rootRedirect is on" "HTTP 401" "$RESP_BODY" ;;
  2*|3*) pass "S14e GET / answers tokenless (HTTP $RESP_CODE)" ;;
  *) fail "S14e GET / answers tokenless" "a 2xx or 3xx" "HTTP $RESP_CODE" "$RESP_BODY" ;;
esac

# The introspection bypass: an exact documented rule, and a boundary that holds
# only for documents no real client sends is not a boundary. Assert its EDGES.
section "3b · GraphQL introspection bypass and its edges"
gql_noauth '{ __schema { queryType { name } } }'
assert_status "S15a an introspection-only document passes tokenless" 200
gql_noauth '{ __typename }'
assert_status "S15b __typename alone is introspection-only" 200

gql_noauth '{ __schema { queryType { name } } tenants(first:1) { totalCount } }'
assert_status_key "S15c a data field beside __schema does NOT bypass" 401 'MissingAuthorizationNotification'
gql_noauth 'query A { __schema { queryType { name } } } query B { tenants(first:1) { totalCount } }'
assert_status_key "S15d a decoy introspection operation next to a real one does NOT bypass" 401 'MissingAuthorizationNotification'
gql_noauth 'query { ...F } fragment F on Query { tenants(first:1) { totalCount } }'
assert_status_key "S15e a root fragment spread does NOT bypass" 401 'MissingAuthorizationNotification'
gql_noauth 'mutation { archiveTenant(id: "00000000-0000-7000-8000-000000000999") { success } }'
assert_status_key "S15f a mutation does NOT bypass" 401 'MissingAuthorizationNotification'

# ════════════════════════════════════════════════════════════════════════════
section "3c · the 403 half — layer 1, which is the whole gate on this aggregate"
note "authz.dataAccess: anyone-with-permission; both ToCriteria return the criteria"
note "unchanged, with no tenant filter and no Restrict. Layers 2 and 3 are absent BY DESIGN."

# A principal deliberately holding no grant. Creating one is better than leaning
# on the bootstrap admin's must-change-password state, which the lane order would
# otherwise decide for us.
TOKEN="$ADMIN_TOKEN"
UNPRIV_EMAIL="qa-unprivileged-${QA_RUN_TAG}@authcore.local"
# The password must echo neither the name nor the e-mail: this service refuses
# that with PasswordEchoesIdentityNotification, which is a User rule this lane is
# not here to test — so the fixture simply satisfies it.
UNPRIV_PASS='Zx7#Kq2m!Wt9v'
req POST /users "$(jq -nc --arg gn 'Qa' --arg fn 'Unprivileged' --arg e "$UNPRIV_EMAIL" \
   --arg p "$UNPRIV_PASS" --arg t "$TENANT_ID" \
   '{givenName:$gn, familyName:$fn, email:$e, status:"active", password:$p, passwordConfirmation:$p, tenantID:$t, groups:[], roles:[], claims:[]}')"
if [ "$RESP_CODE" != "201" ]; then
  skip "S16–S20 the layer-1 403 family" "could not provision an unprivileged principal (HTTP $RESP_CODE) — the gate is UNPROVEN this run"
  printf '           body: %s\n' "$(printf '%s' "$RESP_BODY" | head -c 700)"
else
  pass "S16a an unprivileged principal was provisioned (no roles, therefore no grants)"
  sign_in "$UNPRIV_EMAIL" "$UNPRIV_PASS"
  UNPRIV_TOKEN="$(j '.data.accessToken')"
  UNPRIV_PERMS="$(j '[.data.user.permissions[]] | join(",")')"
  UNPRIV_MUST="$(j '.data.user.mustChangePassword')"

  # Either shape is a principal without tenant:read — a user holding no role, or
  # one whose session is restricted to changing its own password. Both are what
  # this case needs; the assertion names which one arrived.
  case "$UNPRIV_PERMS" in
    ''|'user:change-password') pass "S16b the principal holds no tenant grant (permissions=[${UNPRIV_PERMS}], mustChangePassword=${UNPRIV_MUST})" ;;
    *) fail "S16b the principal holds no tenant grant" "an empty bundle or user:change-password only" "permissions=[${UNPRIV_PERMS}]" "$RESP_BODY" ;;
  esac

  if [ -z "$UNPRIV_TOKEN" ] || [ "$UNPRIV_TOKEN" = "null" ]; then
    skip "S17–S20" "the unprivileged principal could not sign in — the gate is UNPROVEN this run"
  else
    req_astoken "$UNPRIV_TOKEN" GET /tenants
    assert_status_key_field "S17 an authenticated principal without tenant:read" 403 'MissingPermissionNotification' 'permission'
    assert_jq "S17 the refusal names the permission it wanted" '.errors[0].messages[0].value' 'tenant:read'

    # The complement. A gate that refuses EVERYONE is also broken, and only this
    # case can tell the two apart.
    req_astoken "$ADMIN_TOKEN" GET /tenants
    assert_status "S18 the same call WITH the permission is served" 200

    # Per verb: a gate is declared once per route, and a route can lose it alone.
    req_astoken "$UNPRIV_TOKEN" POST /tenants '{"name":"Refused Insert","workspace":"qa-refused-insert","description":"An insert that must never reach the handler.","status":"active"}'
    assert_status_key "S19a insert without tenant:insert" 403 'MissingPermissionNotification'
    req_astoken "$UNPRIV_TOKEN" PATCH "/tenants/00000000-0000-7000-8000-000000000999" '{"name":"Refused Patch Name"}'
    assert_status_key "S19b patch without tenant:update" 403 'MissingPermissionNotification'
    req_astoken "$UNPRIV_TOKEN" PATCH "/tenants/00000000-0000-7000-8000-000000000999/archive" ''
    assert_status_key "S19c archive without tenant:archive" 403 'MissingPermissionNotification'
    req_astoken "$UNPRIV_TOKEN" PATCH "/tenants/00000000-0000-7000-8000-000000000999/unarchive" ''
    assert_status_key "S19d unarchive without tenant:archive" 403 'MissingPermissionNotification'

    # Per SURFACE. A route gated on REST is not thereby gated on GraphQL, and
    # "the other surface forgot the gate" is a real regression only this can catch.
    gql_astoken "$UNPRIV_TOKEN" '{ tenants(first: 1) { totalCount } }'
    assert_gql_key "S20a the GraphQL read is gated too" 'MissingPermissionNotification'
    gql_astoken "$UNPRIV_TOKEN" 'mutation { createTenant(input: { name: "Refused Over Graph", workspace: "qa-refused-graph", description: "A mutation that must never reach the handler.", status: "active" }) { id } }'
    assert_gql_key "S20b the GraphQL mutation is gated too" 'MissingPermissionNotification'

    # ────────────────────────────────────────────────────────────────────────
    # §3 of specs/qa/permission-contract/plan.md — the permission routes.
    #
    # The 401 family (S1-S10), the public-route split (S11-S15) and the
    # tenant-claim gate (S21) are properties of the MIDDLEWARE and are proven
    # once, above. What is genuinely per-entity is the gate itself: a route
    # gated on REST is not thereby gated on GraphQL, and a route can lose its
    # gate alone. Hence one row per verb, per surface.
    # ────────────────────────────────────────────────────────────────────────
    PERM_ABSENT_ID='00000000-0000-7000-8000-000000000999'

    # SP1 — the "not declared public" direction, asserted on this entity's own
    # path. Every 401 case above would still pass for a route that was never
    # gated at all; only this one sees that.
    req_noauth GET /permissions
    assert_status_key "SP1 GET /permissions is not a public route" 401 'MissingAuthorizationNotification'

    req_astoken "$UNPRIV_TOKEN" GET /permissions
    assert_status_key_field "SP2 list without permission:read" 403 'MissingPermissionNotification' 'permission'
    assert_jq "SP2 the refusal names the permission it wanted" '.errors[0].messages[0].value' 'permission:read'

    req_astoken "$UNPRIV_TOKEN" GET "/permissions/${PERM_ABSENT_ID}"
    assert_status_key "SP3 by-id without permission:read" 403 'MissingPermissionNotification'
    assert_jq "SP3 the refusal names permission:read" '.errors[0].messages[0].value' 'permission:read'

    req_astoken "$UNPRIV_TOKEN" POST /permissions '{"resource":"qa-refused","action":"insert","description":"An insert that must never reach the handler."}'
    assert_status_key "SP4 insert without permission:insert" 403 'MissingPermissionNotification'
    assert_jq "SP4 the refusal names permission:insert" '.errors[0].messages[0].value' 'permission:insert'

    req_astoken "$UNPRIV_TOKEN" PATCH "/permissions/${PERM_ABSENT_ID}" '{"description":"A patch that must never reach the handler."}'
    assert_status_key "SP5 patch without permission:update" 403 'MissingPermissionNotification'
    assert_jq "SP5 the refusal names permission:update" '.errors[0].messages[0].value' 'permission:update'

    req_astoken "$UNPRIV_TOKEN" PATCH "/permissions/${PERM_ABSENT_ID}/archive" ''
    assert_status_key "SP6 archive without permission:archive" 403 'MissingPermissionNotification'
    assert_jq "SP6 the refusal names permission:archive" '.errors[0].messages[0].value' 'permission:archive'

    # The complement, per verb. A gate that refuses EVERYONE is also broken, and
    # only this can tell the two apart. The reads answer 200; the writes are
    # aimed at an id that addresses nothing, so reaching the HANDLER at all —
    # a 404, never a 403 — is what proves the gate opened.
    req_astoken "$ADMIN_TOKEN" GET /permissions
    assert_status "SP7a list WITH permission:read is served" 200
    req_astoken "$ADMIN_TOKEN" PATCH "/permissions/${PERM_ABSENT_ID}" '{"description":"A patch that reaches the handler and finds nothing."}'
    assert_status_key "SP7b patch WITH permission:update reaches the handler" 404 'RecordNotFoundNotification'
    req_astoken "$ADMIN_TOKEN" PATCH "/permissions/${PERM_ABSENT_ID}/archive" ''
    assert_status_key "SP7c archive WITH permission:archive reaches the handler" 404 'RecordNotFoundNotification'

    # Per SURFACE — five fields, because GraphQL mounts its own gate per field.
    gql_astoken "$UNPRIV_TOKEN" '{ permissions(first: 1) { totalCount } }'
    assert_gql_key "SP8a the GraphQL listing is gated too" 'MissingPermissionNotification'
    gql_astoken "$UNPRIV_TOKEN" "{ permission(id: \"${PERM_ABSENT_ID}\") { id } }"
    assert_gql_key "SP8b the GraphQL by-id is gated too" 'MissingPermissionNotification'
    gql_astoken "$UNPRIV_TOKEN" 'mutation { createPermission(input: { resource: "qa-refused-graph", action: "insert", description: "A mutation that must never reach the handler." }) { id } }'
    assert_gql_key "SP8c createPermission is gated too" 'MissingPermissionNotification'
    gql_astoken "$UNPRIV_TOKEN" "mutation { patchPermission(id: \"${PERM_ABSENT_ID}\", input: { description: \"A mutation that must never reach the handler.\" }) { id } }"
    assert_gql_key "SP8d patchPermission is gated too" 'MissingPermissionNotification'
    gql_astoken "$UNPRIV_TOKEN" "mutation { archivePermission(id: \"${PERM_ABSENT_ID}\") { success } }"
    assert_gql_key "SP8e archivePermission is gated too" 'MissingPermissionNotification'

    # The complement on GraphQL, so the surface is not proven only by refusals.
    # The selection asks for edges as well as totalCount ON PURPOSE: a GraphQL
    # selection of totalCount ALONE is how this surface spells ?onlyTotal=true,
    # which then conflicts with first: — a 400 that would say nothing about the
    # gate this case is here to prove.
    gql_astoken "$ADMIN_TOKEN" '{ permissions(first: 1) { edges { node { id } } totalCount } }'
    assert_gql_ok "SP9 the GraphQL listing WITH the permission is served"
  fi
fi

# ════════════════════════════════════════════════════════════════════════════
# §3 of specs/qa/role-contract/plan.md — SR1..SR19.
#
# Tenant and Permission could both write "N/A — structurally absent" against
# authorization layers 2 and 3. ROLE CANNOT. It is the first aggregate in this
# service owned by a tenant, so the owner-check in BuildRules and the tenant
# filter in ToCriteria both exist — and the two long-standing skips at the foot
# of this file become real cases here.
# ════════════════════════════════════════════════════════════════════════════
section "3c · Role — layer 1, the gate on SEVEN routes and two surfaces"

ROLE_ABSENT_ID='00000000-0000-7000-8000-000000000999'
MASTER_TENANT_ID='01990000-0001-7000-8000-000000000001'
MASTER_ROLE_ID='01990000-0002-7000-8000-000000000001'

TOKEN="$ADMIN_TOKEN"

# SR1 — the "not declared public" direction on this entity's own path. Every 401
# case above would still pass for a route that was never gated at all; only this
# one sees that.
req_noauth GET /roles
assert_status_key "SR1 GET /roles is not a public route" 401 'MissingAuthorizationNotification'

if [ -z "${UNPRIV_TOKEN:-}" ]; then
  skip "SR2-SR12 the layer-1 403 family on Role" \
       "no ungranted principal was available — the gate is UNPROVEN this run"
else
  req_astoken "$UNPRIV_TOKEN" GET /roles
  assert_status_key_field "SR2 list without role:read" 403 'MissingPermissionNotification' 'permission'
  assert_jq "SR2 the refusal names the permission it wanted" '.errors[0].messages[0].value' 'role:read'

  req_astoken "$UNPRIV_TOKEN" GET "/roles/${ROLE_ABSENT_ID}"
  assert_status_key "SR3 by-id without role:read" 403 'MissingPermissionNotification'
  assert_jq "SR3 names role:read" '.errors[0].messages[0].value' 'role:read'

  req_astoken "$UNPRIV_TOKEN" POST /roles '{"key":"qa-refused","name":"Qa Refused","description":"An insert that must never reach the handler at all.","permissions":[]}'
  assert_status_key "SR4 insert without role:insert" 403 'MissingPermissionNotification'
  assert_jq "SR4 names role:insert" '.errors[0].messages[0].value' 'role:insert'

  req_astoken "$UNPRIV_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}" '{"name":"Qa Refused Patch"}'
  assert_status_key "SR5 patch without role:update" 403 'MissingPermissionNotification'
  assert_jq "SR5 names role:update" '.errors[0].messages[0].value' 'role:update'

  req_astoken "$UNPRIV_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}/archive" ''
  assert_status_key "SR6 archive without role:archive" 403 'MissingPermissionNotification'
  assert_jq "SR6 names role:archive" '.errors[0].messages[0].value' 'role:archive'

  # The two child routes carry a verb of their OWN, decided 2026-08-28.
  req_astoken "$UNPRIV_TOKEN" POST "/roles/${ROLE_ABSENT_ID}/permissions" '{"permissionID":"00000000-0000-7000-8000-000000000999"}'
  assert_status_key "SR7 GRANT without role:grant" 403 'MissingPermissionNotification'
  assert_jq "SR7 names role:grant, not role:update" '.errors[0].messages[0].value' 'role:grant'

  req_astoken "$UNPRIV_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}/permissions/${ROLE_ABSENT_ID}/archive" '{}'
  assert_status_key "SR8 REVOKE without role:grant" 403 'MissingPermissionNotification'
  assert_jq "SR8 names role:grant" '.errors[0].messages[0].value' 'role:grant'

  # ── SR9, the verb SPLIT — the whole point of role:grant existing ──────────
  # "May relabel the role" and "may change what the role can do" are separately
  # grantable, and the second is the privilege-escalation surface. A principal
  # holding role:update alone must be refused on both child routes. Only a
  # principal built for it can see this.
  provision_scoped_principal "sng" role:read role:update role:archive
  if [ -z "$SP_TOKEN" ]; then
    skip "SR9 the role:update / role:grant split" "the principal could not be provisioned — ${SP_FAILED:-unknown reason}"
  else
    NOGRANT_TOKEN="$SP_TOKEN"; NOGRANT_TENANT="$SP_TENANT_ID"
    TOKEN="$ADMIN_TOKEN"
    SR9_ROLE="$(create_role "$(_slug "qa-sr9-${QA_RUN_TAG}")" "$NOGRANT_TENANT" '[]')"

    req_astoken "$NOGRANT_TOKEN" PATCH "/roles/${SR9_ROLE}" '{"name":"Qa Relabelled By Update"}'
    assert_status "SR9a role:update alone MAY relabel its own tenant's role" 200
    req_astoken "$NOGRANT_TOKEN" POST "/roles/${SR9_ROLE}/permissions" '{"permissionID":"00000000-0000-7000-8000-000000000999"}'
    assert_status_key "SR9b and is refused the GRANT route" 403 'MissingPermissionNotification'
    assert_jq "SR9b naming role:grant" '.errors[0].messages[0].value' 'role:grant'
    req_astoken "$NOGRANT_TOKEN" PATCH "/roles/${SR9_ROLE}/permissions/${ROLE_ABSENT_ID}/archive" '{}'
    assert_status_key "SR9c and the REVOKE route too" 403 'MissingPermissionNotification'
  fi

  # ── SR10, the complement per verb ────────────────────────────────────────
  # A gate that refuses EVERYONE is also broken, and only this can tell the two
  # apart. The reads answer 200; the writes are aimed at an id that addresses
  # nothing, so reaching the HANDLER — a 404, never a 403 — is what proves it.
  TOKEN="$ADMIN_TOKEN"
  req_astoken "$ADMIN_TOKEN" GET /roles
  assert_status "SR10a list WITH role:read is served" 200
  req_astoken "$ADMIN_TOKEN" GET "/roles/${ROLE_ABSENT_ID}"
  assert_status_key "SR10b by-id WITH role:read reaches the handler" 404 'RecordNotFoundNotification'
  req_astoken "$ADMIN_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}" '{"name":"Qa Reaches The Handler"}'
  assert_status_key "SR10c patch WITH role:update reaches the handler" 404 'RecordNotFoundNotification'
  req_astoken "$ADMIN_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}/archive" ''
  assert_status_key "SR10d archive WITH role:archive reaches the handler" 404 'RecordNotFoundNotification'
  req_astoken "$ADMIN_TOKEN" POST "/roles/${ROLE_ABSENT_ID}/permissions" '{"permissionID":"00000000-0000-7000-8000-000000000999"}'
  assert_status_key "SR10e GRANT WITH role:grant reaches the handler" 404 'RecordNotFoundNotification'
  req_astoken "$ADMIN_TOKEN" PATCH "/roles/${ROLE_ABSENT_ID}/permissions/${ROLE_ABSENT_ID}/archive" '{}'
  assert_status_key "SR10f REVOKE WITH role:grant reaches the handler" 404 'RecordNotFoundNotification'

  # ── SR11/SR12, per SURFACE — seven fields, because GraphQL mounts its own
  # gate per field and "the other surface forgot the gate" is a real regression.
  gql_astoken "$UNPRIV_TOKEN" '{ roles(first: 1) { edges { node { id } } totalCount } }'
  assert_gql_key "SR11a the GraphQL listing is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" "{ role(id: \"${ROLE_ABSENT_ID}\") { id } }"
  assert_gql_key "SR11b the GraphQL by-id is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" 'mutation { createRole(input: { key: "qa-refused-graph", name: "Qa Refused", description: "A mutation that must never reach the handler at all.", permissions: [] }) { id } }'
  assert_gql_key "SR11c createRole is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" "mutation { patchRole(id: \"${ROLE_ABSENT_ID}\", input: { name: \"Qa Refused\" }) { id } }"
  assert_gql_key "SR11d patchRole is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" "mutation { archiveRole(id: \"${ROLE_ABSENT_ID}\") { success } }"
  assert_gql_key "SR11e archiveRole is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" "mutation { addRolePermission(id: \"${ROLE_ABSENT_ID}\", input: { permissionID: \"${ROLE_ABSENT_ID}\" }) { roleId } }"
  assert_gql_key "SR11f addRolePermission is gated" 'MissingPermissionNotification'
  gql_astoken "$UNPRIV_TOKEN" "mutation { archiveRolePermission(id: \"${ROLE_ABSENT_ID}\", input: { rolePermissionId: \"${ROLE_ABSENT_ID}\" }) { success } }"
  assert_gql_key "SR11g archiveRolePermission is gated" 'MissingPermissionNotification'

  # The complement on GraphQL, so the surface is not proven only by refusals. The
  # selection asks for edges as well as totalCount ON PURPOSE: totalCount alone is
  # how this surface spells ?onlyTotal=true, which then conflicts with first: — a
  # 400 that would say nothing about the gate.
  gql_astoken "$ADMIN_TOKEN" '{ roles(first: 1) { edges { node { id } } totalCount } }'
  assert_gql_ok "SR12 the GraphQL listing WITH the permission is served"
fi

# ════════════════════════════════════════════════════════════════════════════
section "3c · Role — layers 2 and 3, which no earlier round could reach"
note "authz.dataAccess: tenant · tenantField: TenantID · bypass: *:*"
note "Layer 2 is refuseForeignTenant under IfInsertOrUpdate AND IfArchive."
note "Layer 3 is ToCriteria injecting Filter[TenantID] unless IsSuperAdmin()."

TOKEN="$ADMIN_TOKEN"
provision_scoped_principal "srp" role:insert role:update role:archive role:read role:grant
if [ -z "$SP_TOKEN" ]; then
  skip "SR13-SR19 authz layers 2 and 3 on Role" \
       "the scoped principal could not be provisioned — ${SP_FAILED:-unknown reason}. Both layers are UNPROVEN this run."
else
  SCOPED_TOKEN="$SP_TOKEN"; SCOPED_TENANT="$SP_TENANT_ID"
  TOKEN="$ADMIN_TOKEN"
  pass "SR13a a scoped, non-super-admin principal was provisioned through the service's own flow"

  # ── Layer 2 — an identity-derived rule, proven by two calls that differ ONLY
  # in who is asking.
  FOREIGN_BODY="$(role_body "$(_slug "qa-sr13-${QA_RUN_TAG}")" 'Qa Foreign Write' \
    'A role aimed at a tenant the caller does not belong to, sent twice by two callers.' \
    "$MASTER_TENANT_ID" '[]')"
  req_astoken "$SCOPED_TOKEN" POST /roles "$FOREIGN_BODY"
  assert_status_key_field "SR13 the scoped principal writing into ANOTHER tenant" 403 'TenantMismatchNotification' 'tenantID'
  req_astoken "$ADMIN_TOKEN" POST /roles "$FOREIGN_BODY"
  assert_status "SR14 the SAME body from the *:* admin is accepted — the bypass" 201
  SR14_ROLE="$(j '.data.id')"

  # The archive verb carries the same rule. The write side is NOT filtered by
  # ToCriteria, so the row loads and the RULE is what refuses — which is why this
  # is a 403 and not the 404 the READ side answers.
  req_astoken "$SCOPED_TOKEN" PATCH "/roles/${SR14_ROLE}/archive" ''
  assert_status_key "SR15 the scoped principal archiving another tenant's role" 403 'TenantMismatchNotification'

  # ── Layer 3 — row scoping. An isolation leak answers 200, which is exactly why
  # it needs its own cases rather than riding on a refusal.
  req_astoken "$SCOPED_TOKEN" GET '/roles?first=100'
  assert_status "SR17a the scoped principal may list" 200
  assert_jq_true "SR17b and the master role is ABSENT from every page" \
    "[.data[].id] | index(\"${MASTER_ROLE_ID}\") == null" \
    'the isolation filter removed it'
  assert_jq_true "SR17c every row it CAN see belongs to its own tenant" \
    "([.data[].tenantID] | unique) as \$t | \$t == [] or \$t == [\"${SCOPED_TENANT}\"]" \
    'the filter selected rather than merely hiding one row'

  req_astoken "$SCOPED_TOKEN" GET "/roles/${MASTER_ROLE_ID}"
  assert_status_key "SR18 a by-id read of another tenant's role is NOT-FOUND, never FORBIDDEN" 404 'RecordNotFoundNotification'
  note "a 403 here would confirm the row exists to a caller who may not see it"

  req_astoken "$ADMIN_TOKEN" GET '/roles?first=100'
  assert_jq_true "SR19a the *:* admin DOES see the master role — the bypass on the read side" \
    "[.data[].id] | index(\"${MASTER_ROLE_ID}\") != null" \
    'the super-admin crosses the row scope'
  assert_jq_true "SR19b and sees more than one tenant's rows" \
    '([.data[].tenantID] | unique | length) > 1' \
    'without this, a service filtering EVERYONE would pass SR17 for the wrong reason'
fi

TOKEN="$ADMIN_TOKEN"
skip "field-level read authz (ReadCriteria.Restrict) on Role" \
     "N/A, stated rather than skipped: spec.md §9 declares none — \"every field a caller may see the row at all for, they may see entirely; row-level isolation does the work here\". There is no column to find absent for one caller and present for another, no tabular export whose header could be pruned, and no __typename edge, which exists only where a restricted field is in the selection."

# ── what this round does NOT prove, printed rather than implied ────────────
section "3e · coverage this lane does NOT claim"
# The tenant gate is reachable with the technique S7-S10 already use: a token
# signed by the bench's own key, with a correct iss/aud/exp, differing from a
# good one in ONE thing — it carries no tenant_id claim. The JWT validation
# therefore passes and the request reaches the middleware's tenant gate, which is
# the only non-401 outcome the middleware itself produces.
#
# This needs no keypair of its own and invents no credential: like every other
# forged token here, it exists to be REFUSED.
NO_TENANT_CLAIMS="$(jq -nc --arg iss "$JWT_ISS" --arg aud "$JWT_AUD" --argjson exp "$FUTURE" \
   --argjson iat "$NOW" --arg sub '01990000-0004-7000-8000-000000000001' \
  '{iss:$iss, aud:$aud, sub:$sub, exp:$exp, iat:$iat, permissions:["*:*"]}')"
NO_TENANT_TOKEN="$(mint_rs256 "$QA_SIGNING_KEY_FILE" "$(rs_header "$KID")" "$NO_TENANT_CLAIMS")"
req_rawauth "Bearer ${NO_TENANT_TOKEN}" GET /tenants
assert_status_key "S21 a valid token carrying no tenant_id claim (tenant.required: true)" 403 'TenantMissingNotification'

# The complement, and it is what makes S21 mean something: the SAME token with
# the claim present is served. Without this pair, a service refusing every token
# would pass S21 for the wrong reason.
WITH_TENANT_TOKEN="$(mint_rs256 "$QA_SIGNING_KEY_FILE" "$(rs_header "$KID")" "$(claims "$JWT_ISS" "$JWT_AUD" "$FUTURE")")"
req_rawauth "Bearer ${WITH_TENANT_TOKEN}" GET /tenants
assert_status "S21b the same token WITH a tenant_id claim is served" 200
skip "authz layer 2 — identity-derived BuildRules" \
     "structurally absent on Tenant: BuildRules reads no principal field (internal/domain/tenant.go). Nothing to prove, and asserting one would be inventing a rule."
skip "authz layer 3 — tenant row scoping and Restrict" \
     "structurally absent: authz.dataAccess is anyone-with-permission and both ToCriteria return the criteria unchanged. Anyone holding tenant:read sees every row, by design (spec.md §B Q4)."
skip "authz layers 2 and 3 on Permission" \
     "structurally absent there too, and for a stronger reason: the catalog is GLOBAL by design (spec.md §10) — not partitioned by tenant, so there is no tenant_id to filter on and no owner to check. Both ToCriteria return the criteria unchanged (find_permissions_by_params_query.go:18, find_permission_by_id_query.go:19) and no BuildRules clause reads a principal field. Layer 1 is the whole gate, and SP2-SP9 prove it on both surfaces."

lane_summary
