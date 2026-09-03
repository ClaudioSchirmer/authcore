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
  fi
fi

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

lane_summary
