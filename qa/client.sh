#!/usr/bin/env bash
# Lane: client — the REST half of the framework contract for the Client aggregate.
#
# Families Q1-Q11 of specs/qa/client-contract/plan.md §1. The V family (GraphQL) lives in
# qa/client_graphql.sh; the business rules live in qa/domain.sh (C-*); the gate lives in
# qa/security.sh (S8.x); the audit trail lives in qa/audit.sh (A68+).
#
# THE TWO IDEAS THIS LANE IS BUILT AROUND.
#
# 1. THE SECRET IS REVEALED EXACTLY ONCE PER MINT, AND ITS HASH REACHES NO SURFACE. The
#    create and the rotation are the only two responses in this service that ever carry a
#    credential; the by-id read, the listing, the PATCH response and the ?fields= vocabulary
#    must all be innocent of it — and of BOTH hash columns, whose absence is a hand-made
#    decision (spec.md §2 property 5) that nothing automatic enforces. Q3 asserts the
#    absences; Q8.8 asserts the oracle is unreachable through filter, ordering and
#    projection. Those cases fail the day somebody adds a filter: tag, which is why they
#    exist.
#
# 2. THREE JOINS, AND ONE FIELD IS RULES-ONLY. The root join into Tenant serves
#    tenantWorkspace (filterable, sortable, projectable) and tenantArchivedAt (served, NOT
#    addressable — absent from the criteria BY CHOICE), and keeps tenantStatus entirely OFF
#    the wire (hidden: the rules read it, nobody receives it). The two child joins (Role,
#    Claim) are served and projectable and addressable in NO criteria. ClientAllowedCIDR
#    declares no join at all: its entries are exactly {id, cidr, label}.
#
# The lane runs as the bootstrap admin (*:*), so nothing here is blocked by row scope or by
# the escalation rules. §1b and §3 are where the scoped and MACHINE principals do the work.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init client

# ═════════════════════════════════════════════════════════════════════════════════════════
# Fixtures. Catalog ids resolve by their PAIR, roles by their handle — no UUID literal is
# written twice in this suite.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_LANE=$(new_tenant active "$(ws cli)")  || exit 1
TEN_OTHER=$(new_tenant active "$(ws clib)") || exit 1

P_TENANT_READ=$(permission_id_of tenant read)
[ -n "$P_TENANT_READ" ] || { echo "client.sh: a seeded catalog id could not be resolved — is migration 0012 applied?" >&2; exit 1; }

# Roles the collection entries point at. The admin passes the escalation rules by
# construction (*:*), so these need only exist and be active in the right tenant.
R_LANE=$(new_role "$(role_key ca)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
R_LANE2=$(new_role "$(role_key cb)" "$TEN_LANE" "$P_TENANT_READ") || exit 1

# Claim definitions for the claims collection: appliesTo must admit a MACHINE, so `client`
# and `both` are the two usable kinds here (the user-only negative lives in qa/domain.sh).
C_STR=$(new_claim "$(claim_name cs)" string both   "$TEN_LANE") || exit 1
C_NUM=$(new_claim "$(claim_name cn)" number client "$TEN_LANE") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q1 — the thirteen mounted routes. Modes() declares display/insert/update/archive: FOUR
#      modes, THIRTEEN routes, because three collections carry seven verbs between them and
#      the hand-written rotation carries one more that no mode expresses at all.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "Q1.1 POST /clients" "201 — the record as stored, carrying all three collections AND the secret (the first of the two reveal seats)"
N_MAIN=$(client_label main)
api POST /clients "$(jq -nc --arg n "$N_MAIN" --arg t "$TEN_LANE" --arg r "$R_LANE" --arg c "$C_STR" \
  '{name:$n, description:"The main integration of the Q family, holding one of each entry.", status:"active", tenantID:$t,
    roles:[{roleID:$r}], allowedCIDRs:[{cidr:"203.0.113.0/24", label:"QA egress range"}], claims:[{claimID:$c, value:"sa-east-1"}]}')"
assert_json_at 201 '[(.data.roles|length),(.data.allowedCIDRs|length),(.data.claims|length),(.data.secret|test("^acs_[A-Za-z0-9_-]{43}$"))] | join(",")' "1,1,1,true"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
[ -n "$ID_MAIN" ] && [ "$ID_MAIN" != "null" ] || { echo "client.sh: the main client could not be created: $HTTP_BODY" >&2; exit 1; }

case_ "Q1.2 GET /clients/{id}" "200 — the full document"
api GET "/clients/$ID_MAIN"
assert_json_at 200 '.data.name' "$N_MAIN"

case_ "Q1.3 GET /clients" "200 — data plus the pagination envelope"
api GET "/clients?name.eq=$(printf '%s' "$N_MAIN" | jq -sRr @uri)"
assert_json_at 200 '[(.data | length), (.pagination.totalCount)] | join(",")' "1,1"

case_ "Q1.4 PATCH /clients/{id}" "200 — the root as stored"
api PATCH "/clients/$ID_MAIN" '{"description":"The main integration, relabelled to prove the patch verb."}'
assert_json_at 200 '.data.description' "The main integration, relabelled to prove the patch verb."

case_ "Q1.5 POST /clients/{id}/secret" "200 — the rotation: the SECOND reveal seat, a POST because it CREATES a credential, answering a body because this response is the only place the plaintext can ever be read"
rotate_secret "$ID_MAIN" '{}'
assert_json_at 200 '[(.data.secret|test("^acs_[A-Za-z0-9_-]{43}$")),(.data.id)] | join(",")' "true,$ID_MAIN"

new_client "$(client_label coll)" "$TEN_LANE" || exit 1
ID_COLL="$CLIENT_ID"

case_ "Q1.6 POST /clients/{id}/roles" "201 — the entry as stored, with the id the server minted"
api POST "/clients/$ID_COLL/roles" "$(jq -nc --arg r "$R_LANE" '{roleID:$r}')"
assert_json_at 201 '[(.data.clientId != null),(.data.clientRole.roleID)] | join(",")' "true,$R_LANE"
CH_R=$(printf '%s' "$HTTP_BODY" | jq -r '.data.clientRole.id')

case_ "Q1.7 PATCH /clients/{id}/roles/{rid}/archive" "204, NO BODY"
api PATCH "/clients/$ID_COLL/roles/$CH_R/archive"
assert_empty_body 204

case_ "Q1.8 POST /clients/{id}/allowedCIDRs" "201 — the entry as stored: id, cidr, label"
api POST "/clients/$ID_COLL/allowedCIDRs" '{"cidr":"198.51.100.0/24","label":"QA staging egress"}'
assert_json_at 201 '.data.clientAllowedCIDR.cidr' "198.51.100.0/24"
CH_N=$(printf '%s' "$HTTP_BODY" | jq -r '.data.clientAllowedCIDR.id')

case_ "Q1.9 PATCH /clients/{id}/allowedCIDRs/{cid}/archive" "204, NO BODY"
api PATCH "/clients/$ID_COLL/allowedCIDRs/$CH_N/archive"
assert_empty_body 204

case_ "Q1.10 POST /clients/{id}/claims" "201 — the entry as stored, id + claimID + value"
api POST "/clients/$ID_COLL/claims" "$(jq -nc --arg c "$C_STR" '{claimID:$c, value:"alpha"}')"
assert_json_at 201 '.data.clientClaim.value' "alpha"
CH_C=$(printf '%s' "$HTTP_BODY" | jq -r '.data.clientClaim.id')

case_ "Q1.11 PATCH /clients/{id}/claims/{cid}" "200 — the entry changed, carrying ONLY value (change.shape: patch, patchExcludes ClaimID)"
api PATCH "/clients/$ID_COLL/claims/$CH_C" '{"value":"beta"}'
assert_json_at 200 '.data.clientClaim.value' "beta"

case_ "Q1.12 the PATCH kept the entry's id" "the SAME child id, never a new one — 'correct this cost center' is one entry changing, which is why this collection mounts change and its two neighbours refuse it"
assert_json '.data.clientClaim.id' "$CH_C"

case_ "Q1.13 ...and the read-back agrees" "one claims entry, that id, that value — not a removal plus a creation"
api GET "/clients/$ID_COLL"
assert_json_at 200 '[(.data.claims|length),(.data.claims[0].id),(.data.claims[0].value)] | join(",")' "1,$CH_C,beta"

case_ "Q1.14 PATCH /clients/{id}/claims/{cid}/archive" "204, NO BODY"
api PATCH "/clients/$ID_COLL/claims/$CH_C/archive"
assert_empty_body 204

case_ "Q1.15 PATCH /clients/{id}/archive" "204, NO BODY"
new_client "$(client_label arc1)" "$TEN_LANE" || exit 1
api PATCH "/clients/$CLIENT_ID/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q2 — the write/read asymmetry, asserted as a contract rather than assumed away.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "Q2.1 a collection WRITE response carries the stored columns and nothing traversed" "roles {id,roleID} · allowedCIDRs {cidr,id,label} · claims {claimID,id,value} — no roleKey, no claimName, no stamp"
api POST /clients "$(jq -nc --arg n "$(client_label asym)" --arg t "$TEN_LANE" --arg r "$R_LANE" --arg c "$C_STR" \
  '{name:$n, description:"The asymmetry probe, holding one entry per collection.", status:"active", tenantID:$t,
    roles:[{roleID:$r}], allowedCIDRs:[{cidr:"203.0.113.64/26", label:"QA asymmetry range"}], claims:[{claimID:$c, value:"asym"}]}')"
ID_ASYM=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
assert_json '[([.data.roles[]       |(keys|sort|join(","))]|unique|join("")),
              ([.data.allowedCIDRs[]|(keys|sort|join(","))]|unique|join("")),
              ([.data.claims[]      |(keys|sort|join(","))]|unique|join(""))] | join(" / ")' \
  "id,roleID / cidr,id,label / claimID,id,value"

case_ "Q2.2 a READ response carries the traversal — where one is declared" "role entries gain key/name, claim entries gain name/type; CIDR entries gain NOTHING, because that collection references no other aggregate"
api GET "/clients/$ID_ASYM"
assert_json_at 200 '[(.data.roles[0] |has("roleKey")), (.data.roles[0] |has("roleName")),
                     (.data.claims[0]|has("claimName")),(.data.claims[0]|has("claimValueType")),
                     ([.data.allowedCIDRs[0]|keys|sort|join(",")])[0]] | join(" / ")' \
  "true / true / true / true / cidr,id,label"

case_ "Q2.2b the entry Row DTOs use omitempty, so a LIVE counterpart reports no stamp" "roleArchivedAt/claimArchivedAt ABSENT while the counterparts live — Q10 asserts the other half"
assert_json '[(.data.roles[0]|has("roleArchivedAt")),(.data.claims[0]|has("claimArchivedAt"))] | join(",")' "false,false"

case_ "Q2.3 the INSERT response declares no framework stamps and no root-join fields" "createdAt/updatedAt/archivedAt and tenantWorkspace/tenantArchivedAt all absent — the Response declares none of the five"
api POST /clients "$(client_body "$(client_label stamp)" "$TEN_LANE")"
assert_json_at 201 '[(.data|has("createdAt")),(.data|has("updatedAt")),(.data|has("archivedAt")),(.data|has("tenantWorkspace")),(.data|has("tenantArchivedAt"))] | join(",")' \
  "false,false,false,false,false"
ID_STAMP=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "Q2.4 the PATCH response says the same" "the same five absent — a suite that assumed symmetry with the read would assert a field the contract never promised"
api PATCH "/clients/$ID_STAMP" '{"description":"A different description, written to watch what the response does not carry."}'
assert_json_at 200 '[(.data|has("createdAt")),(.data|has("updatedAt")),(.data|has("archivedAt")),(.data|has("tenantWorkspace")),(.data|has("tenantArchivedAt"))] | join(",")' \
  "false,false,false,false,false"

case_ "Q2.5 the ROTATE response is exactly four keys" "id, secret, secretChangedAt, previousSecretExpiresAt — deliberately NOT the client's row: a credential response must not double as a profile read"
rotate_secret "$ID_STAMP" '{}'
assert_json_at 200 '.data | keys | sort | join(",")' "id,previousSecretExpiresAt,secret,secretChangedAt"

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q3 — the golden record. Every declared field, written then read back — and the absences
#      that ARE the entity: the secret shown once, the hashes shown never, the rules-only
#      join field on no wire at all.
# ═════════════════════════════════════════════════════════════════════════════════════════

N_GOLD=$(client_label gold)
api POST /clients "$(jq -nc --arg n "$N_GOLD" --arg t "$TEN_LANE" --arg r "$R_LANE" --arg c "$C_NUM" \
  '{name:$n, description:"The golden record: one of every field this model declares, read back verbatim.", status:"active", tenantID:$t,
    roles:[{roleID:$r}], allowedCIDRs:[{cidr:"2001:db8::/32", label:"QA IPv6 egress"}], claims:[{claimID:$c, value:"1000"}]}')"
ID_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
SECRET_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.secret')
[ -n "$ID_GOLD" ] && [ "$ID_GOLD" != "null" ] || { echo "client.sh: the golden record could not be created: $HTTP_BODY" >&2; exit 1; }

case_ "Q3.1 the CREATE is a reveal seat, and the value is a real credential shape" "secret matches ^acs_ + 43 base64url runes — 256 bits behind a scanner-matchable prefix, spec.md §B-Q2"
if printf '%s' "$SECRET_GOLD" | grep -Eq '^acs_[A-Za-z0-9_-]{43}$'; then pass_; else HTTP_BODY="secret='$SECRET_GOLD'"; fail_ "the minted secret does not match the declared shape"; fi

case_ "Q3.2 ...and the hashes were NOT in that response" "secretHash and previousSecretHash absent from the create body — no Response DTO declares either, on any operation"
api POST /clients "$(client_body "$(client_label gold2)" "$TEN_LANE")"
assert_json_at 201 '[.data | has("secretHash"), has("previousSecretHash")] | join(",")' "false,false"

api GET "/clients/$ID_GOLD"

case_ "Q3.3 every scalar of the model round-trips" "name, description, status, tenantID — written or derived, read back verbatim"
assert_json_at 200 '[.data.name,.data.status,.data.tenantID] | join("|")' "$N_GOLD|active|$TEN_LANE"

case_ "Q3.4 secretChangedAt is present and RFC3339" "always set — the credential is minted at birth (rules.manual credential-minting)"
assert_rfc3339 '.data.secretChangedAt'

case_ "Q3.5 previousSecretExpiresAt is present-and-null on a fresh client" "nothing is retiring on a first issue — the by-id Response declares the key unconditionally, so it is there carrying null"
assert_json '[(.data|has("previousSecretExpiresAt")),(.data.previousSecretExpiresAt == null)] | map(tostring) | join(",")' "true,true"

case_ "Q3.6 THE SECRET IS ABSENT FROM THE BY-ID READ" "absent, not present-and-null — the acceptance check spec.md §2 property 6 names: GET /clients/{id} immediately after the create does not contain it"
assert_json '[.data | has("secret"), has("secretHash"), has("previousSecretHash")] | join(",")' "false,false,false"

case_ "Q3.7 tenantStatus IS ON NO WIRE — the rules-only join field" "hidden: true means the rules read it and nobody receives it. Needing a value and publishing it are different questions, and this absence is the leak case nobody writes"
assert_json '.data | has("tenantStatus")' "false"

case_ "Q3.8 the ROOT join rides the read" "tenantWorkspace filled, tenantArchivedAt present (null while the tenant lives)"
assert_json '[(.data.tenantWorkspace|length > 0),(.data|has("tenantArchivedAt")),(.data.tenantArchivedAt == null)] | map(tostring) | join(",")' "true,true,true"

case_ "Q3.9 the two CHILD joins ride the read" "role entries carry roleKey/roleName, claim entries carry claimName/claimValueType — and the joined VALUE is the counterpart's, not a blank"
assert_json '[(.data.roles[0]|has("roleKey")),(.data.roles[0]|has("roleName")),(.data.claims[0].claimValueType)] | join(",")' "true,true,number"

case_ "Q3.10 the CIDR entry is exactly its two stored columns plus id" "cidr + label + id, stored as sent (canonical in, canonical out) — this collection declares NO join, so nothing else may appear"
assert_json '[(.data.allowedCIDRs[0]|keys|sort|join("+")),(.data.allowedCIDRs[0].cidr)] | join(" / ")' "cidr+id+label / 2001:db8::/32"

case_ "Q3.11 archivedAt is present-and-null on a live row" "the by-id Response declares the key unconditionally — which is what Q3.15 contrasts the listing against"
assert_json '[(.data|has("archivedAt")),(.data.archivedAt == null)] | map(tostring) | join(",")' "true,true"

case_ "Q3.12 the LISTING row is innocent of the credential too" "no secret, no secretHash, no previousSecretHash, no tenantStatus — four absences on the other read"
api GET "/clients?id.eq=$ID_GOLD"
assert_json_at 200 '[.data[0] | has("secret"), has("secretHash"), has("previousSecretHash"), has("tenantStatus")] | join(",")' "false,false,false,false"

case_ "Q3.13 ...and the PATCH response as well" "the same absences on the fourth body"
api PATCH "/clients/$ID_GOLD" '{"description":"The golden record, patched to watch the response stay innocent of the credential."}'
assert_json_at 200 '[.data | has("secret"), has("secretHash"), has("previousSecretHash")] | join(",")' "false,false,false"

case_ "Q3.14 an empty allowedCIDRs collection is SERVED, not omitted" "the visible spelling of 'unrestricted' — standing in for the ipRestricted computed field the generator refused (tasks.md deviation 4)"
api GET "/clients/$ID_STAMP"
assert_json_at 200 '[(.data|has("allowedCIDRs")),((.data.allowedCIDRs // [])|length)] | map(tostring) | join(",")' "true,0"

case_ "Q3.15 THE TWO READS DIFFER, and deliberately" "the LISTING omits a null archivedAt (every field of FindClientsResponse is omitempty) while the BY-ID read declares it and carries null"
BYID_HAS=$(api GET "/clients/$ID_GOLD"; printf "%s" "$HTTP_BODY" | jq -r ".data | has(\"archivedAt\")")
api GET "/clients?id.eq=$ID_GOLD"
LIST_HAS=$(printf "%s" "$HTTP_BODY" | jq -r ".data[0] | has(\"archivedAt\")")
if [ "$BYID_HAS" = "true" ] && [ "$LIST_HAS" = "false" ]; then pass_; else HTTP_BODY="by-id has=$BYID_HAS listing has=$LIST_HAS"; fail_ "the two reads do not differ as declared"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q4 — validation, 422, asserting the KEY. One representative per value object and shape.
# ═════════════════════════════════════════════════════════════════════════════════════════

bad_client() { # bad_client JQ_PATCH — a valid body with one field replaced
  client_body "$(client_label bad)" "$TEN_LANE" | jq -c "$1"
}

case_ "Q4.1 an empty name" "422 RequiredFieldNotification"
api POST /clients "$(bad_client '.name = ""')"
assert_rest 422 RequiredFieldNotification

case_ "Q4.2 a name that is all punctuation" "422 InvalidDisplayNameNotification — DisplayName is the VO for THINGS, and it still wants a letter"
api POST /clients "$(bad_client '.name = "!!!"')"
assert_rest 422 InvalidDisplayNameNotification

case_ "Q4.3 a description under the floor" "422 InvalidDescriptionNotification — a client nobody can explain is a client nobody dares revoke (spec.md Q7c)"
api POST /clients "$(bad_client '.description = "too short"')"
assert_rest 422 InvalidDescriptionNotification

case_ "Q4.4 a status outside the closed set" "422 UnknownClientStatusNotification"
api POST /clients "$(bad_client '.status = "pending"')"
assert_rest 422 UnknownClientStatusNotification

case_ "Q4.5 a malformed tenantID" "422 InvalidIDUUIDNotification"
api POST /clients "$(bad_client '.tenantID = "lixo"')"
assert_rest 422 InvalidIDUUIDNotification

case_ "Q4.6 tenant-valid IS A GUARD — a malformed tenantID ENDS the pass" "InvalidIDUUIDNotification ALONE, even though the body carries a second problem — the tenant is the premise of everything below it"
api POST /clients "$(bad_client '.tenantID = "lixo" | .status = "pending"')"
assert_json_at 422 '[.errors[]?.messages[]?.notificationKey] | unique | join(",")' "InvalidIDUUIDNotification"

case_ "Q4.7 an empty claim value" "422 RequiredFieldNotification on claims[0].value — the framework's required gate catches the empty before any VO, exactly as it does for an empty name (Q4.1)"
api POST "/clients/$ID_COLL/claims" "$(jq -nc --arg c "$C_STR" '{claimID:$c, value:""}')"
assert_rest 422 RequiredFieldNotification

case_ "Q4.7b a claim value over the 256-rune budget" "422 InvalidClaimValueNotification — the shared ClaimValue VO's own refusal, generated once from User's spec and reused: every value here rides on every request to every service, so the budget is a header size before it is a length"
api POST "/clients/$ID_COLL/claims" "$(jq -nc --arg c "$C_STR" --arg v "$(printf 'x%.0s' $(seq 1 260))" '{claimID:$c, value:$v}')"
assert_rest 422 InvalidClaimValueNotification

case_ "Q4.8 an empty CIDR label" "422 RequiredFieldNotification — an unlabelled range is one nobody dares remove"
api POST "/clients/$ID_COLL/allowedCIDRs" '{"cidr":"203.0.113.128/26","label":""}'
assert_rest 422 RequiredFieldNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q5 — the dual 409, and the asymmetry the user round predicted this lane would pin:
#      Client's TRANSITION rule declares SemanticValidation (422, like Tenant), while its
#      StateConflict 409 comes from the rotation (qa/domain.sh C13f).
# ═════════════════════════════════════════════════════════════════════════════════════════

N_DUP=$(client_label dup)
api POST /clients "$(client_body "$N_DUP" "$TEN_LANE")"
ID_DUP=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "Q5.1 the same name twice in one tenant" "409 ClientNameAlreadyExistsNotification — the DUPLICATE flavor, semantic 'Conflict'"
api POST /clients "$(client_body "$N_DUP" "$TEN_LANE")"
assert_rest 409 ClientNameAlreadyExistsNotification

case_ "Q5.1b ...and the envelope says which flavor" "semantic 'Conflict'"
assert_json '[.errors[]?.messages[]?.semantic] | unique | join(",")' "Conflict"

case_ "Q5.2 the same name in ANOTHER tenant" "201 — uniqueness is PER TENANT: two tenants naming their integration 'Billing' are not in conflict"
api POST /clients "$(client_body "$N_DUP" "$TEN_OTHER")"
assert_status 201

case_ "Q5.3 the same role twice, as two calls" "409 ClientAlreadyGrantsRoleNotification"
grant_role_to_client "$ID_DUP" "$R_LANE" >/dev/null
api POST "/clients/$ID_DUP/roles" "$(jq -nc --arg r "$R_LANE" '{roleID:$r}')"
assert_rest 409 ClientAlreadyGrantsRoleNotification

case_ "Q5.4 the same range twice, as two calls" "409 ClientAlreadyAllowsCIDRNotification — and the CANONICAL form is what makes the index able to see it"
allow_cidr "$ID_DUP" "192.0.2.0/24" "QA duplicate probe" >/dev/null
api POST "/clients/$ID_DUP/allowedCIDRs" '{"cidr":"192.0.2.0/24","label":"QA duplicate probe again"}'
assert_rest 409 ClientAlreadyAllowsCIDRNotification

case_ "Q5.5 the same claim twice, as two calls" "409 ClientAlreadyHoldsClaimNotification — one principal, at most one value per definition: the anti-collision argument of the whole two-level chain"
set_client_claim "$ID_DUP" "$C_STR" "one" >/dev/null
api POST "/clients/$ID_DUP/claims" "$(jq -nc --arg c "$C_STR" '{claimID:$c, value:"two"}')"
assert_rest 409 ClientAlreadyHoldsClaimNotification

case_ "Q5.6 the same role id TWICE IN ONE BODY" "409 — the insertOrUpdate path, not the per-call one: the same rule reached through the other door"
api POST /clients "$(jq -nc --arg n "$(client_label dup2)" --arg t "$TEN_LANE" --arg r "$R_LANE" \
  '{name:$n, description:"A client whose body carries the same grant twice, to reach the duplicate rule through the insert.", status:"active", tenantID:$t,
    roles:[{roleID:$r},{roleID:$r}]}')"
assert_status 409

# ── the status machine: BOTH edges pass, and the out-of-map value is a 422, NOT a 409 ─────
new_client "$(client_label st)" "$TEN_LANE" || exit 1
ID_ST="$CLIENT_ID"

case_ "Q5.7 active → suspended" "200 — an allowed edge"
api PATCH "/clients/$ID_ST" '{"status":"suspended"}'
assert_json_at 200 '.data.status' "suspended"

case_ "Q5.8 suspended → active" "200 — the other allowed edge"
api PATCH "/clients/$ID_ST" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "Q5.9 a no-op transition" "200 — staying is always allowed"
api PATCH "/clients/$ID_ST" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "Q5.10 a value outside the closed set is a 422 CARRYING BOTH KEYS" "UnknownClientStatus + InvalidClientStatusTransition at 422 — Client's transition notification declares SemanticValidation, so unlike User's (409 StateConflict) nothing outranks the validation here. Two status machines, two different status codes, both deliberate (client_test.go:275)"
api PATCH "/clients/$ID_ST" '{"status":"deleted"}'
assert_json_at 422 '[.errors[]?.messages[]?.notificationKey] | unique | sort | join(",")' \
  "InvalidClientStatusTransitionNotification,UnknownClientStatusNotification"

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q6 — archive: one-way by design, and the address it releases.
# ═════════════════════════════════════════════════════════════════════════════════════════

N_ARC=$(client_label arc)
api POST /clients "$(jq -nc --arg n "$N_ARC" --arg t "$TEN_LANE" --arg r "$R_LANE" --arg c "$C_STR" \
  '{name:$n, description:"The archive probe, holding one live entry per collection when the root goes.", status:"active", tenantID:$t,
    roles:[{roleID:$r}], allowedCIDRs:[{cidr:"203.0.113.192/27", label:"QA archived range"}], claims:[{claimID:$c, value:"keep"}]}')"
ID_ARC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
SECRET_ARC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.secret')

# One entry taken out on its own BEFORE the root's archive, so Q6.8 has a stamp to compare.
CH_ARC_N2=$(allow_cidr "$ID_ARC" "203.0.113.224/27" "QA early-out range") || exit 1
api PATCH "/clients/$ID_ARC/allowedCIDRs/$CH_ARC_N2/archive"
STAMP_EARLY=$(sql "SELECT archived_at FROM client_allowed_cidrs WHERE id = '$CH_ARC_N2';" | tr -d '[:space:]')

case_ "Q6.1 PATCH /clients/{id}/archive" "204, no body — archived while ACTIVE, so Q6.3 can watch the forced status"
api PATCH "/clients/$ID_ARC/archive"
assert_empty_body 204

case_ "Q6.2 the archived row leaves the listing" "0 rows — kept but hidden"
api GET "/clients?id.eq=$ID_ARC"
assert_json_at 200 '.data | length' "0"

case_ "Q6.3 ?includeArchived=true reveals it, stamped AND FORCED SUSPENDED" "archivedAt non-null and status 'suspended' on a client archived while active — archive-forces-suspended reached the ROW (spec.md C6, mirroring Tenant 13 and User U15)"
api GET "/clients?id.eq=$ID_ARC&includeArchived=true"
assert_json_at 200 '[(.data|length),(.data[0].archivedAt != null),(.data[0].status)] | map(tostring) | join(",")' "1,true,suspended"

case_ "Q6.4 the by-id read without the flag" "404 — the default scope refuses an archived row"
api GET "/clients/$ID_ARC"
assert_status 404

case_ "Q6.5 the by-id read WITH the flag" "200 — the same row, revealed"
api GET "/clients/$ID_ARC?includeArchived=true"
assert_json_at 200 '.data.id' "$ID_ARC"

case_ "Q6.6 no unarchive route exists" "404 RouteNotFoundNotification — a credential that can come back from the dead is a credential whose revocation nobody can trust (spec.md §5). Tenant remains the ONLY aggregate that mounts one"
api PATCH "/clients/$ID_ARC/unarchive"
assert_rest 404 RouteNotFoundNotification

case_ "Q6.7 THE NAME COMES BACK" "201 with a NEW id — the unique index is WHERE archived_at IS NULL, so archiving RELEASES the label; the retired credential does not"
api POST /clients "$(client_body "$N_ARC" "$TEN_LANE")"
ID_REBORN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
if [ "$HTTP_STATUS" = "201" ] && [ -n "$ID_REBORN" ] && [ "$ID_REBORN" != "$ID_ARC" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, id '$ID_REBORN' vs '$ID_ARC'"; fi

case_ "Q6.8 the root archive CASCADED onto its three collections" "0/0/0 active entries left"
GOT=$(sql "SELECT (SELECT count(*) FROM client_roles         WHERE client_id='$ID_ARC' AND archived_at IS NULL)
        || '/' || (SELECT count(*) FROM client_allowed_cidrs WHERE client_id='$ID_ARC' AND archived_at IS NULL)
        || '/' || (SELECT count(*) FROM client_claims        WHERE client_id='$ID_ARC' AND archived_at IS NULL);" | tr -d '[:space:]')
if [ "$GOT" = "0/0/0" ]; then pass_; else HTTP_BODY="active roles/cidrs/claims = '$GOT'"; fail_ "the cascade missed a collection"; fi

case_ "Q6.9 the cascade did NOT restamp an entry that was already out" "the early entry keeps its ORIGINAL stamp — the archive scopes its write, it does not sweep the collection"
GOT=$(sql "SELECT archived_at FROM client_allowed_cidrs WHERE id = '$CH_ARC_N2';" | tr -d '[:space:]')
if [ -n "$STAMP_EARLY" ] && [ "$GOT" = "$STAMP_EARLY" ]; then pass_; else HTTP_BODY="stamp '$GOT' vs the original '$STAMP_EARLY'"; fail_ "the early entry was restamped"; fi

case_ "Q6.10 a collection write onto an archived client" "404 — LoadForWrite runs ScopeActive"
api POST "/clients/$ID_ARC/roles" "$(jq -nc --arg r "$R_LANE2" '{roleID:$r}')"
assert_status 404

case_ "Q6.11 a PATCH onto an archived client" "404, same scope"
api PATCH "/clients/$ID_ARC" '{"description":"A write onto a row the active scope no longer serves, which must not land."}'
assert_status 404

case_ "Q6.12 THE ROTATION onto an archived client" "404 — an archived credential is not rotatable, which is half of what a trustworthy revocation means (the other half, that its secret no longer MINTS, is qa/domain.sh C-MINT4)"
rotate_secret "$ID_ARC" '{}'
assert_status 404

case_ "Q6.13 a second archive of the same row" "404 — it is no longer in the active set to archive"
api PATCH "/clients/$ID_ARC/archive"
assert_status 404

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q7 — the read vocabulary, one representative per declared operator family.
# ═════════════════════════════════════════════════════════════════════════════════════════

# A dedicated tenant, so the counts below are not hostage to what other lanes created.
TEN_READ=$(new_tenant active "$(ws clir)") || exit 1
api POST /clients "$(jq -nc --arg t "$TEN_READ" '{name:"Alpha Integration", description:"Posts invoices from the billing system into the ledger, for the read family.", status:"active", tenantID:$t}')"
ID_R1=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
api POST /clients "$(jq -nc --arg t "$TEN_READ" '{name:"Beta Integration", description:"Mirrors inventory movements into the warehouse system, for the read family.", status:"active", tenantID:$t}')"
ID_R2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
api POST /clients "$(jq -nc --arg t "$TEN_READ" '{name:"Gamma Sync", description:"Synchronises the product catalog nightly, for the read family.", status:"suspended", tenantID:$t}')"
ID_R3=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
[ -n "$ID_R1" ] && [ -n "$ID_R2" ] && [ -n "$ID_R3" ] || { echo "client.sh: the read fixtures could not be created" >&2; exit 1; }
Q="tenantID.eq=$TEN_READ"

case_ "Q7.1 tenantID.eq" "3 — the lane's own partition"
api GET "/clients?$Q"; assert_json_at 200 '.data | length' "3"

case_ "Q7.2 name.eq" "1"
api GET "/clients?$Q&name.eq=Alpha%20Integration"; assert_json_at 200 '.data | length' "1"

case_ "Q7.3 name.icontains" "2 — the case-insensitive fragment an operator actually types"
api GET "/clients?$Q&name.icontains=integration"; assert_json_at 200 '.data | length' "2"

case_ "Q7.4 name.startswith" "1"
api GET "/clients?$Q&name.startswith=Gam"; assert_json_at 200 '.data | length' "1"

case_ "Q7.5 name.ne" "2 — the operator name has and description deliberately does not"
api GET "/clients?$Q&name.ne=Alpha%20Integration"; assert_json_at 200 '.data | length' "2"

case_ "Q7.6 description.icontains" "1 — the only family that leaf declares (contains/icontains)"
api GET "/clients?$Q&description.icontains=warehouse"; assert_json_at 200 '.data | length' "1"

case_ "Q7.7 status.eq" "1 — the suspended one"
api GET "/clients?$Q&status.eq=suspended"; assert_json_at 200 '.data | length' "1"

case_ "Q7.8 status.in" "3"
api GET "/clients?$Q&status.in=active,suspended"; assert_json_at 200 '.data | length' "3"

case_ "Q7.9 secretChangedAt.gte" "3 — every credential in this partition was minted after 2020, and this is the filter that answers 'which credentials predate the incident'"
api GET "/clients?$Q&secretChangedAt.gte=2020-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "3"

case_ "Q7.10 createdAt.gte" "3"
api GET "/clients?$Q&createdAt.gte=2020-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "3"

case_ "Q7.11 id.eq" "1"
api GET "/clients?id.eq=$ID_R1"; assert_json_at 200 '.data | length' "1"

case_ "Q7.12 THE ROOT JOIN IS FILTERABLE — tenantWorkspace.eq" "3 — the reach that distinguishes this backing"
WS_READ=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].tenantWorkspace' 2>/dev/null)
api GET "/clients?tenantWorkspace.eq=$WS_READ"
assert_json_at 200 '.data | length' "3"

case_ "Q7.13 tenantWorkspace.istartswith" "at least 3 — the join leg admits the whole text family"
api GET "/clients?tenantWorkspace.istartswith=$(printf '%s' "$WS_READ" | cut -c1-8)"
assert_json_at 200 '[.data | length] | .[0] >= 3' "true"

case_ "Q7.14 ?orderBy=name" "Alpha, Beta, Gamma"
api GET "/clients?$Q&orderBy=name"
assert_json_at 200 '[.data[].name] | join(",")' "Alpha Integration,Beta Integration,Gamma Sync"

case_ "Q7.15 ?orderBy=-name" "the same, reversed"
api GET "/clients?$Q&orderBy=-name"
assert_json_at 200 '[.data[].name] | join(",")' "Gamma Sync,Beta Integration,Alpha Integration"

case_ "Q7.16 ?orderBy=tenantWorkspace" "200 — the root join is sortable too"
api GET "/clients?$Q&orderBy=tenantWorkspace"; assert_status 200

case_ "Q7.17 ?fields=previousSecretExpiresAt" "200 — the field IS projectable (tasks.md deviation 5, spec.md §9 corrected 2026-09-08): an operator asking 'until when does the old secret work' selects it; Q8.7 asserts it stays OUT of filter and sort"
api GET "/clients?$Q&fields=previousSecretExpiresAt&first=1"; assert_status 200

case_ "Q7.18 ?fields= a CHILD JOIN path" "200 — a child join's fields ARE addressable in a projection, as <segment>.<field>"
api GET "/clients?tenantID.eq=$TEN_LANE&fields=roles.roleKey,claims.claimName&first=1"
assert_status 200

case_ "Q7.19 ?onlyTotal=true" "a count and no data array at all"
api GET "/clients?$Q&onlyTotal=true"
assert_json_at 200 '[(.pagination.totalCount),(has("data") and (.data != null))] | join(",")' "3,false"

case_ "Q7.20 ?onlyTotal=true beside a filter" "counting a filtered subset is the canonical use"
api GET "/clients?$Q&status.eq=active&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "2"

case_ "Q7.21 the pagination envelope, page 1" "first=2 → 2 rows, hasNextPage true, hasPreviousPage false, an endCursor, and NO startCursor on page 1"
api GET "/clients?$Q&orderBy=name&first=2"
assert_json_at 200 '[(.data|length),(.pagination.totalCount),(.pagination.hasNextPage),(.pagination.hasPreviousPage),(.pagination.endCursor|length>0),(.pagination|has("startCursor"))] | map(tostring) | join(",")' \
  "2,3,true,false,true,false"
CUR_END=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')

case_ "Q7.22 the walk — page 2 by echoing endCursor into ?after=" "the remaining row, hasNextPage false, hasPreviousPage true"
api GET "/clients?$Q&orderBy=name&first=2&after=$CUR_END"
assert_json_at 200 '[(.data|length),(.data[0].name),(.pagination.hasNextPage),(.pagination.hasPreviousPage)] | join(",")' \
  "1,Gamma Sync,false,true"

case_ "Q7.23 the two pages are DISJOINT" "Gamma Sync is not on page 1"
api GET "/clients?$Q&orderBy=name&first=2"
assert_json_at 200 '[.data[].name] | index("Gamma Sync") | . == null' "true"

case_ "Q7.24 ?last= alone serves the TAIL window" "the last row in the ordering, not the first"
api GET "/clients?$Q&orderBy=name&last=1"
assert_json_at 200 '[(.data|length),(.data[0].name)] | join(",")' "1,Gamma Sync"

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q8 — rejected reads: the whole typed-400 family, and the CREDENTIAL oracle.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "Q8.1 an unknown filter key" "400 SchemaViolationNotification — never a silently ignored parameter"
api GET "/clients?bogus.eq=x"; assert_rest 400 SchemaViolationNotification

case_ "Q8.2 an operator outside a leaf's allowlist" "400 — status declares eq,in and nothing else"
api GET "/clients?status.contains=act"; assert_rest 400 SchemaViolationNotification

case_ "Q8.3 ?search= on a DTO that never declared it" "400 SchemaViolationNotification — the opt-in gate: no text index serves this view, so declaring the control would advertise a capability the server does not have (spec.md §9)"
api GET "/clients?search=billing"; assert_rest 400 SchemaViolationNotification

case_ "Q8.4 an unresolvable ?fields= path" "400 naming fields[bogus]"
api GET "/clients?fields=bogus"; assert_rest_field 400 SchemaViolationNotification "fields[bogus]"

case_ "Q8.5 a filter VALUE outside the leaf's kind — a timestamp" "400 InvalidFilterValueNotification (pin >= v0.70.0)"
api GET "/clients?secretChangedAt.gte=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "Q8.6 a filter VALUE outside the leaf's kind — an identity column" "400 InvalidFilterValueNotification"
api GET "/clients?tenantID.eq=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "Q8.7 previousSecretExpiresAt is OUT of filter and sort — the two-thirds that stayed" "400 on the filter AND 400 on the orderBy, while Q7.17 showed the same field projects — §0c item c pinned as its three-way split"
api GET "/clients?previousSecretExpiresAt.gte=2020-01-01T00:00:00Z"; S_F="$HTTP_STATUS"
api GET "/clients?orderBy=previousSecretExpiresAt"; S_O="$HTTP_STATUS"
if [ "$S_F" = "400" ] && [ "$S_O" = "400" ]; then pass_; else HTTP_BODY="filter=$S_F orderBy=$S_O"; fail_ "a direction that was closed answered something else"; fi

# ── THE CREDENTIAL ORACLE. spec.md §2 property 5: a hand-made decision nothing automatic
# ── enforces. These fail the day somebody adds a filter: tag, which is why they exist.
case_ "Q8.8a ?secretHash.eq=<a real SHA-256>" "400 SchemaViolationNotification — nothing declares a filter: tag on that column, on any surface, ever"
api GET "/clients?secretHash.eq=9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
assert_rest 400 SchemaViolationNotification

case_ "Q8.8b ?previousSecretHash.startswith=9f — the walk, one character at a time" "400 — the RETIRING hash is as unreachable as the current one; a grace window must not open a side door"
api GET "/clients?previousSecretHash.startswith=9f"; assert_rest 400 SchemaViolationNotification

case_ "Q8.8c ?orderBy=secretHash" "400 — an ordering over a hash is an oracle by another route"
api GET "/clients?orderBy=secretHash"; assert_rest 400 SchemaViolationNotification

case_ "Q8.8d ?fields=secretHash" "400 SchemaViolationNotification — a stored column no Response declares is NOT selectable. Never a silent 200 {}, which would leave open whether it was read"
api GET "/clients?fields=secretHash"; assert_rest 400 SchemaViolationNotification

case_ "Q8.8e ?fields=secret — the plaintext's wire name" "400 — it has no column at all, and asking by the name the two reveal seats use must not resolve on a read"
api GET "/clients?fields=secret"; assert_rest 400 SchemaViolationNotification

case_ "Q8.9 an undeclared reserved control on the BY-ID read" "400 — FindClientByIDRequest declares includeArchived ALONE, and PRESENCE is what trips the gate"
api GET "/clients/$ID_R1?fields=name"; assert_rest 400 SchemaViolationNotification

case_ "Q8.10 ...even when the undeclared control is INACTIVE" "400 — ?onlyTotal=false on the by-id read still rejects: presence, not value"
api GET "/clients/$ID_R1?onlyTotal=false"; assert_rest 400 SchemaViolationNotification

case_ "Q8.11 ?first= above the view's ceiling" "400 — the page ceiling is 100"
api GET "/clients?first=101"; assert_status 400

case_ "Q8.12 mixed directions — first + last" "400"
api GET "/clients?first=2&last=2"; assert_status 400

case_ "Q8.13 mixed directions — first + before" "400; backward is last+before"
api GET "/clients?first=2&before=abc"; assert_status 400

case_ "Q8.14 ?onlyTotal=true beside a page-shaping control" "400 — the only-total conflict matrix"
api GET "/clients?onlyTotal=true&first=10"; assert_status 400

case_ "Q8.15 a malformed cursor" "400 SchemaViolationNotification"
api GET "/clients?first=2&after=not-a-cursor"; assert_rest 400 SchemaViolationNotification

case_ "Q8.16 a cursor against a DIFFERENT orderBy" "400 — a cursor is only meaningful inside the ordering that minted it"
api GET "/clients?$Q&orderBy=name&first=2" >/dev/null
CUR_MM=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
api GET "/clients?$Q&orderBy=status&first=2&after=$CUR_MM"; assert_status 400

# ── the 1:N boundary, and the rules-only root field, from the side that REFUSES ──────────
case_ "Q8.17 a filter over a CHILD JOIN field — roles" "400 — served does not mean addressable. 'Which clients hold billing-manager?' is not answerable from this listing"
api GET "/clients?roles.roleKey.eq=billing-manager"; assert_rest 400 SchemaViolationNotification

case_ "Q8.18 ...claims" "400, same boundary"
api GET "/clients?claims.claimName.eq=x_region"; assert_rest 400 SchemaViolationNotification

case_ "Q8.19 ...the join-less collection" "400 — allowedCIDRs declares no join and is not addressable either: 'which clients allow this range' is a question for the write side's own table, not this listing"
api GET "/clients?allowedCIDRs.cidr.eq=203.0.113.0%2F24"; assert_rest 400 SchemaViolationNotification

case_ "Q8.20 an ORDER over a child-join field" "400 — the boundary holds for sorting too"
api GET "/clients?orderBy=roles.roleKey"; assert_rest 400 SchemaViolationNotification

case_ "Q8.21 tenantStatus — the rules-only join field — is OUT of the criteria vocabulary" "400 on the filter and 400 on the projection: hidden means out of the VOCABULARY, not merely out of the row. The commercial state of the owner is read by the rules and published to nobody"
api GET "/clients?tenantStatus.eq=active"; S_F="$HTTP_STATUS"
api GET "/clients?fields=tenantStatus"; S_P="$HTTP_STATUS"
if [ "$S_F" = "400" ] && [ "$S_P" = "400" ]; then pass_; else HTTP_BODY="filter=$S_F fields=$S_P"; fail_ "the hidden join field answered something else"; fi

case_ "Q8.22 tenantArchivedAt is served and NOT addressable — absent by choice" "400 — the spec's own words: the vocabulary has no isnull, so a declaration would buy only 'archived between dates' while TenantStatus answers the real question. Absent by choice, not by limitation"
api GET "/clients?tenantArchivedAt.gte=2020-01-01T00:00:00Z"; assert_rest 400 SchemaViolationNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q9 — absent verbs, wrong addresses, malformed ids.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "Q9.1 DELETE /clients/{id}" "405 MethodNotAllowedNotification — soft-delete only, and the PATH is registered (GET and PATCH live there), so the refusal is about the METHOD. The 404 shape belongs to a path nothing registers at all, which Q6.6 asserts on unarchive"
api DELETE "/clients/$ID_R1"; assert_rest 405 MethodNotAllowedNotification

case_ "Q9.2 a mounted path under another method" "405 — POST /clients/{id} is not registered, but the path is"
api POST "/clients/$ID_R1" '{}'; assert_status 405

case_ "Q9.3 a valid-but-absent uuid, on the by-id read" "404"
api GET "/clients/01990000-dead-7000-8000-000000000000"; assert_status 404

case_ "Q9.4 ...on a PATCH" "404"
api PATCH "/clients/01990000-dead-7000-8000-000000000000" '{"description":"A write aimed at a row that does not exist, which must answer not-found."}'; assert_status 404

case_ "Q9.5 ...on THE ROTATION" "404 — the hand-written route loads through the same scope as the generated ones"
rotate_secret "01990000-dead-7000-8000-000000000000" '{}'; assert_status 404

case_ "Q9.6 ...on a collection add" "404 — the owner is not there"
api POST "/clients/01990000-dead-7000-8000-000000000000/roles" "$(jq -nc --arg r "$R_LANE" '{roleID:$r}')"
assert_status 404

case_ "Q9.7 a by-id address that is not a uuid, on a READ" "404 UnknownIDAddressNotification (pin >= v0.70.0)"
api GET "/clients/lixo"; assert_rest 404 UnknownIDAddressNotification

case_ "Q9.8 the same address on a WRITE" "400 MalformedIDNotification — the split is by VERB, not by surface"
api PATCH "/clients/lixo" '{"description":"A write aimed at an address that is not an id at all."}'; assert_rest 400 MalformedIDNotification

case_ "Q9.9 ...on the archive verb" "400 MalformedIDNotification"
api PATCH "/clients/lixo/archive"; assert_rest 400 MalformedIDNotification

case_ "Q9.10 ...on the ROTATION — the hand-written route obeys the same split" "400 MalformedIDNotification"
rotate_secret "lixo" '{}'; assert_rest 400 MalformedIDNotification

case_ "Q9.11 a malformed CHILD address" "404 RecordNotFoundNotification on the collection — the v0.70.0 split governs the ROOT address; a child segment is matched against the owner's collection, so an unmatched one is the collection's own not-found"
api PATCH "/clients/$ID_COLL/claims/lixo" '{"value":"x"}'; assert_rest 404 RecordNotFoundNotification

case_ "Q9.12 a child id that is a valid uuid but belongs to ANOTHER client" "404 — an entry is addressed inside its owner, never globally"
new_client "$(client_label x1)" "$TEN_LANE" || exit 1
ID_X1="$CLIENT_ID"
new_client "$(client_label x2)" "$TEN_LANE" || exit 1
ID_X2="$CLIENT_ID"
CH_X1=$(grant_role_to_client "$ID_X1" "$R_LANE") || exit 1
api PATCH "/clients/$ID_X2/roles/$CH_X1/archive"; assert_status 404

case_ "Q9.13 an absent child id under the right owner" "404 RecordNotFoundNotification — the caller named an entry, so a missing one is an answer rather than a no-op"
api PATCH "/clients/$ID_X1/roles/01990000-dead-7000-8000-000000000000/archive"
assert_rest 404 RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q10 — the joins under an archived counterpart: the entry OUTLIVES what it points at.
# ═════════════════════════════════════════════════════════════════════════════════════════

# Counterparts archived AFTER the entries were made. The INNER join matches on the id and
# not on the stamp, so the entry survives — the served stamp is the entry's own account of
# outliving its target, and ?includeArchived governs ROOTS, never what a traversal reaches.
new_client "$(client_label out)" "$TEN_LANE" || exit 1
ID_OUT="$CLIENT_ID"
R_DOOMED=$(new_role "$(role_key cdoom)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
C_DOOMED=$(new_claim "$(claim_name cdoom)" string both "$TEN_LANE") || exit 1
grant_role_to_client "$ID_OUT" "$R_DOOMED" >/dev/null
set_client_claim "$ID_OUT" "$C_DOOMED" "still-here" >/dev/null
api PATCH "/roles/$R_DOOMED/archive"
api PATCH "/claims/$C_DOOMED/archive"

api GET "/clients/$ID_OUT"

case_ "Q10.1 a grant OUTLIVES the role it points at" "the entry is still there, roleArchivedAt now non-null, roleKey/roleName still resolved — which is exactly what makes a dangling grant VISIBLE instead of silently absent"
assert_json_at 200 '[(.data.roles|length),(.data.roles[0].roleArchivedAt != null),(.data.roles[0].roleKey|length > 0)] | join(",")' "1,true,true"

case_ "Q10.2 ...and a claim value the definition it points at" "same shape, and claimValueType still reads the archived definition — a stored 'true' stays readable"
assert_json '[(.data.claims|length),(.data.claims[0].claimArchivedAt != null),(.data.claims[0].claimValueType)] | join(",")' "1,true,string"

case_ "Q10.3 the entry's OWN stamp is still null" "the counterpart was archived, the grant was not — two different rows, two different stamps"
assert_json '.data.roles[0] | has("archivedAt")' "false"

# ═════════════════════════════════════════════════════════════════════════════════════════
# Q11 — route inventory. An ENUMERATION of what exists, never an expectation about answers.
# ═════════════════════════════════════════════════════════════════════════════════════════

api GET /openapi.json "" -
OPENAPI="$HTTP_BODY"

case_ "Q11.1 openapi.json enumerates thirteen /clients operations" "the five root/read verbs, the seven collection verbs and the hand-written rotation"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key | startswith("/clients")) | .value | to_entries[] | select(.key | test("^(get|post|patch|put|delete)$"))] | length')
if [ "$GOT" = "13" ]; then pass_; else HTTP_BODY="$(printf '%s' "$OPENAPI" | jq -c '[.paths | keys[] | select(startswith("/clients"))]')"; fail_ "operations = $GOT"; fi

case_ "Q11.2 every /clients path the source declares is registered" "the eleven distinct path templates of client_routes.go and client_secret_routes_manual.go — allowedCIDRs in camelCase, the spelling §0c item b corrected"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | keys[] | select(startswith("/clients"))] | sort | join(" ")')
WANT="/clients/ /clients/{id} /clients/{id}/allowedCIDRs /clients/{id}/allowedCIDRs/{clientAllowedCIDRId}/archive /clients/{id}/archive /clients/{id}/claims /clients/{id}/claims/{clientClaimId} /clients/{id}/claims/{clientClaimId}/archive /clients/{id}/roles /clients/{id}/roles/{clientRoleId}/archive /clients/{id}/secret"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="$GOT"; fail_ "paths differ"; fi

case_ "Q11.3 no /clients/{id}/unarchive is registered" "the absence Q6.6 asserts from the other side"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | keys[] | select(test("/clients.*unarchive"))] | length')
if [ "$GOT" = "0" ]; then pass_; else HTTP_BODY="$OPENAPI"; fail_ "unarchive paths = $GOT"; fi

case_ "Q11.4 the eight permission literals ride the operations" "client:read/insert/update/archive/grant/manage-network/set-claim/rotate-secret — the widest verb vocabulary in the service, three collections with three distinct literals"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key|startswith("/clients")) | .value | to_entries[] | .value | (.["x-required-permission"] // (.description // "")) ] | join(" ")' \
  | grep -oE 'client:[a-z]+(-[a-z]+)*' | sort -u | tr '\n' ' ' | sed 's/ $//')
WANT="client:archive client:grant client:insert client:manage-network client:read client:rotate-secret client:set-claim client:update"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="found: $GOT"; fail_ "literals differ"; fi

qa_finish
