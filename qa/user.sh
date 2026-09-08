#!/usr/bin/env bash
# Lane: user — the REST half of the framework contract for the User aggregate.
#
# Families M1-M11 of specs/qa/user-contract/plan.md §1. The N family (GraphQL) lives in
# qa/user_graphql.sh; the business rules live in qa/domain.sh (U1-U25); the gate lives in
# qa/security.sh (S7.x); the audit trail lives in qa/audit.sh (A54+).
#
# THE TWO IDEAS THIS LANE IS BUILT AROUND.
#
# 1. FOUR JOINS, AND ONLY ONE OF THEM IS ADDRESSABLE. The root join into Tenant is served
#    AND filterable AND projectable; the three child joins are served AND projectable and
#    addressable in NO criteria. A suite copied from qa/group.sh gets the child half right
#    and the root half wrong, so every case below says which side of that line it is on:
#
#      ROOT join → Tenant                 CHILD joins → Group · Role · Claim
#      ──────────────────────             ─────────────────────────────────────
#      tenantWorkspace   served           groupKey/groupName/groupArchivedAt   served
#      tenantStatus      served           roleKey/roleName/roleArchivedAt      served
#      tenantArchivedAt  served           claimName/claimValueType/claimArchivedAt served
#      FILTERABLE + sortable  (M7)        addressable in NO criteria  (M8.13)
#      projectable            (M7)        projectable                 (M7.10)
#
# 2. A COLUMN THAT MUST NEVER ANSWER ANYTHING. password_hash is read by the projection —
#    FindUsersByParamsResult and FindUserByIDResult both declare it — and is dropped by the
#    web layer, which declares it in no Response. M3 asserts the absence and M8.14-17 assert
#    that it is unreachable through filter, ordering and projection. Those four are the ONLY
#    thing standing between a hash and a character-at-a-time walk: spec.md §9 records that the
#    policy is the ABSENCE of a filter: tag and that nothing automatic enforces it. They fail
#    on the day somebody adds one, which is why they exist.
#
# The lane runs as the bootstrap admin (*:*), so nothing here is blocked by row scope. §1b and
# §3 are where the scoped principals do the work.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init user

# ═════════════════════════════════════════════════════════════════════════════════════════
# Fixtures. Catalog ids resolve by their PAIR, roles and groups by their handle — no UUID
# literal is written twice in this suite.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_LANE=$(new_tenant active "$(ws user)")  || exit 1
TEN_OTHER=$(new_tenant active "$(ws userb)") || exit 1

P_TENANT_READ=$(permission_id_of tenant read)
P_USER_READ=$(permission_id_of user read)
for p in "$P_TENANT_READ" "$P_USER_READ"; do
  [ -n "$p" ] || { echo "user.sh: a seeded catalog id could not be resolved — is migration 0012 applied?" >&2; exit 1; }
done

# One role and one group per tenant, so the three collections have live counterparts to point
# at. A role is tenant-scoped and a group confers roles, so all of these have to live where
# the users do.
R_LANE=$(new_role "$(role_key ua)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
R_LANE2=$(new_role "$(role_key ub)" "$TEN_LANE" "$P_USER_READ")  || exit 1
G_LANE=$(new_group "$(group_key ua)" "$TEN_LANE" "$R_LANE")      || exit 1
G_LANE2=$(new_group "$(group_key ub)" "$TEN_LANE" "$R_LANE2")    || exit 1

# Three claim definitions, one per value type, all applying to users. The `both` one is what
# M1's happy path uses; U11 and U12 in qa/domain.sh build their own.
C_STR=$(new_claim "$(claim_name s)" string both "$TEN_LANE")  || exit 1
C_NUM=$(new_claim "$(claim_name n)" number user  "$TEN_LANE") || exit 1
C_BOOL=$(new_claim "$(claim_name b)" bool  both  "$TEN_LANE") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# M1 — the fourteen mounted routes. Modes() declares display/insert/update/archive: FOUR
#      modes, FOURTEEN routes, because three collections carry seven verbs between them and
#      the credential pair carries two more that no mode expresses at all.
# ═════════════════════════════════════════════════════════════════════════════════════════

E_MAIN=$(user_email main)
case_ "M1.1 POST /users" "201 — the record as stored, carrying all three collections"
api POST /users "$(jq -nc --arg e "$E_MAIN" --arg t "$TEN_LANE" --arg p "$QA_USER_PASS1" \
  --arg g "$G_LANE" --arg r "$R_LANE" --arg c "$C_STR" \
  '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p,
    groups:[{groupID:$g}], roles:[{roleID:$r}], claims:[{claimID:$c, value:"1000"}]}')"
assert_json_at 201 '[(.data.groups|length),(.data.roles|length),(.data.claims|length)] | join(",")' "1,1,1"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "M1.2 GET /users/{id}" "200 — the full document"
api GET "/users/$ID_MAIN"
assert_json_at 200 '.data.email' "$E_MAIN"

case_ "M1.3 GET /users" "200 — data plus the pagination envelope"
api GET "/users?email.eq=$E_MAIN"
assert_json_at 200 '[(.data | length), (.pagination.totalCount)] | join(",")' "1,1"

case_ "M1.4 PATCH /users/{id}" "200 — the root as stored"
api PATCH "/users/$ID_MAIN" '{"givenName":"Renamed"}'
assert_json_at 200 '.data.givenName' "Renamed"

E_COLL=$(user_email coll)
ID_COLL=$(new_user "$E_COLL" "$TEN_LANE") || exit 1

case_ "M1.5 POST /users/{id}/groups" "201 — the entry as stored, with the id the server minted"
api POST "/users/$ID_COLL/groups" "$(jq -nc --arg g "$G_LANE" '{groupID:$g}')"
assert_json_at 201 '[(.data.userId != null),(.data.userGroup.groupID)] | join(",")' "true,$G_LANE"
CH_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.userGroup.id')

case_ "M1.6 PATCH /users/{id}/groups/{gid}/archive" "204, NO BODY"
api PATCH "/users/$ID_COLL/groups/$CH_G/archive"
assert_empty_body 204

case_ "M1.7 POST /users/{id}/roles" "201 — the entry as stored"
api POST "/users/$ID_COLL/roles" "$(jq -nc --arg r "$R_LANE" '{roleID:$r}')"
assert_json_at 201 '.data.userRole.roleID' "$R_LANE"
CH_R=$(printf '%s' "$HTTP_BODY" | jq -r '.data.userRole.id')

case_ "M1.8 PATCH /users/{id}/roles/{rid}/archive" "204, NO BODY"
api PATCH "/users/$ID_COLL/roles/$CH_R/archive"
assert_empty_body 204

case_ "M1.9 POST /users/{id}/claims" "201 — the entry as stored, id + claimID + value"
api POST "/users/$ID_COLL/claims" "$(jq -nc --arg c "$C_STR" '{claimID:$c, value:"alpha"}')"
assert_json_at 201 '.data.userClaim.value' "alpha"
CH_C=$(printf '%s' "$HTTP_BODY" | jq -r '.data.userClaim.id')

case_ "M1.10 PATCH /users/{id}/claims/{cid}" "200 — the entry changed"
api PATCH "/users/$ID_COLL/claims/$CH_C" '{"value":"beta"}'
assert_json_at 200 '.data.userClaim.value' "beta"

case_ "M1.11 the PATCH kept the entry's id" "the SAME child id, never a new one — the whole point of the verb the 2026-08-28 correction brought back"
assert_json '.data.userClaim.id' "$CH_C"

case_ "M1.12 ...and the read-back agrees" "one claims entry, that id, that value — not a removal plus a creation"
api GET "/users/$ID_COLL"
assert_json_at 200 '[(.data.claims|length),(.data.claims[0].id),(.data.claims[0].value)] | join(",")' "1,$CH_C,beta"

case_ "M1.13 PATCH /users/{id}/claims/{cid}/archive" "204, NO BODY"
api PATCH "/users/$ID_COLL/claims/$CH_C/archive"
assert_empty_body 204

# The credential pair needs a caller who IS the row, so this user signs in for itself. The
# rotation is the fixture; M1.14 asserts the SECOND change, made with a usable token.
# THE ACCOUNT NEEDS user:change-password TO CHANGE ITS OWN PASSWORD TWICE, and that is not a
# quirk of the suite: the must-change token carries the permission EMBEDDED, which is what lets
# the FIRST rotation happen, but the token after it carries the account's real bundle — empty
# here without a role. spec.md §10 calls this out: "a caller without it cannot set their own
# password at all". qa/security.sh S7.1n asserts the 403 half; this fixture is the other one.
E_CRED=$(user_email cred)
ID_CRED=$(new_user "$E_CRED" "$TEN_LANE") || exit 1
R_CRED=$(new_role "$(role_key cred)" "$TEN_LANE" "$(permission_id_of user change-password)") || exit 1
grant_role_to_user "$ID_CRED" "$R_CRED" >/dev/null
T_CRED=$(usable_token "$E_CRED" "$ID_CRED") || exit 1

case_ "M1.14 PATCH /users/{id}/password" "204, NO BODY — a credential operation that answered with the row would turn a password change into a profile read"
rotate_password "$ID_CRED" "$T_CRED" "$QA_USER_PASS2" 'Zt7#Wandering7'
assert_empty_body 204

case_ "M1.15 PATCH /users/{id}/password-reset" "204, NO BODY — the helpdesk operation, on somebody else's row"
api PATCH "/users/$ID_COLL/password-reset" '{"password":"Qa!Reset2026x","passwordConfirmation":"Qa!Reset2026x"}'
assert_empty_body 204

case_ "M1.16 PATCH /users/{id}/archive" "204, NO BODY"
E_ARCH1=$(user_email arc1); ID_ARCH1=$(new_user "$E_ARCH1" "$TEN_LANE") || exit 1
api PATCH "/users/$ID_ARCH1/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# M2 — the write/read asymmetry, asserted as a contract rather than assumed away.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "M2.1 a collection WRITE response carries the stored column and nothing traversed" "groups {id,groupID} · roles {id,roleID} · claims {id,claimID,value} — no key, no name, no stamp"
api POST /users "$(jq -nc --arg e "$(user_email asym)" --arg t "$TEN_LANE" --arg p "$QA_USER_PASS1" \
  --arg g "$G_LANE" --arg r "$R_LANE" --arg c "$C_STR" \
  '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p,
    groups:[{groupID:$g}], roles:[{roleID:$r}], claims:[{claimID:$c, value:"1000"}]}')"
ID_ASYM=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
assert_json '[([.data.groups[]|(keys|sort|join(","))]|unique|join("")),
              ([.data.roles[] |(keys|sort|join(","))]|unique|join("")),
              ([.data.claims[]|(keys|sort|join(","))]|unique|join(""))] | join(" / ")' \
  "groupID,id / id,roleID / claimID,id,value"

case_ "M2.2 a READ response carries the traversal" "every entry carries its counterpart's key/name/type beside the stored column"
api GET "/users/$ID_ASYM"
assert_json_at 200 '[(.data.groups[0]|has("groupKey")),(.data.groups[0]|has("groupName")),
                     (.data.roles[0] |has("roleKey")), (.data.roles[0] |has("roleName")),
                     (.data.claims[0]|has("claimName")),(.data.claims[0]|has("claimValueType"))] | join(",")' \
  "true,true,true,true,true,true"

case_ "M2.2b the entry Row DTOs use omitempty, so a LIVE counterpart reports no stamp" "groupArchivedAt/roleArchivedAt/claimArchivedAt all ABSENT while the counterparts live — M10.2-10.4 assert the other half, where the stamp appears because the counterpart was archived"
assert_json '[(.data.groups[0]|has("groupArchivedAt")),(.data.roles[0]|has("roleArchivedAt")),(.data.claims[0]|has("claimArchivedAt"))] | join(",")' \
  "false,false,false"

case_ "M2.3 the INSERT response declares no framework stamps and no root-join fields" "createdAt/updatedAt/archivedAt and tenantWorkspace/tenantStatus/tenantArchivedAt are all absent — the Response declares none of the six"
api POST /users "$(user_body "$(user_email stamp)" active "$TEN_LANE")"
assert_json_at 201 '[(has("data")),(.data|has("createdAt")),(.data|has("updatedAt")),(.data|has("archivedAt")),(.data|has("tenantWorkspace")),(.data|has("tenantStatus")),(.data|has("tenantArchivedAt"))] | join(",")' \
  "true,false,false,false,false,false,false"
ID_STAMP=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "M2.4 the PATCH response says the same" "the same six absent — a suite that assumed symmetry with the read would assert a field the contract never promised"
api PATCH "/users/$ID_STAMP" '{"givenName":"Stamped"}'
assert_json_at 200 '[(.data|has("createdAt")),(.data|has("updatedAt")),(.data|has("archivedAt")),(.data|has("tenantWorkspace")),(.data|has("tenantStatus")),(.data|has("tenantArchivedAt"))] | join(",")' \
  "false,false,false,false,false,false"

# ═════════════════════════════════════════════════════════════════════════════════════════
# M3 — the golden record. Every declared field, written then read back.
# ═════════════════════════════════════════════════════════════════════════════════════════

E_GOLD=$(user_email gold)
api POST /users "$(jq -nc --arg e "$E_GOLD" --arg t "$TEN_LANE" --arg p "$QA_USER_PASS1" \
  --arg g "$G_LANE" --arg r "$R_LANE" --arg c "$C_NUM" \
  '{givenName:"Maria", familyName:"Souza Lima", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p,
    groups:[{groupID:$g}], roles:[{roleID:$r}], claims:[{claimID:$c, value:"1000"}]}')"
ID_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
[ -n "$ID_GOLD" ] && [ "$ID_GOLD" != "null" ] || { echo "user.sh: the golden record could not be created: $HTTP_BODY" >&2; exit 1; }

api GET "/users/$ID_GOLD"

case_ "M3.1 every scalar of the model round-trips" "the eight the caller wrote or the domain derived, read back verbatim"
assert_json_at 200 '[.data.givenName,.data.familyName,.data.email,.data.status,.data.tenantID] | join("|")' \
  "Maria|Souza Lima|$E_GOLD|active|$TEN_LANE"

case_ "M3.2 the COMPOSITE is spoken as its EXPOSED PARTS" "givenName and familyName are on the wire; the composite's own field name is on NO surface"
assert_json '[(.data|has("givenName")),(.data|has("familyName")),(.data|has("name")),(.data|has("personName"))] | join(",")' \
  "true,true,false,false"

case_ "M3.3 fullName is served ready" "givenName + \" \" + familyName, derived per row, from FromQueryResult"
assert_json '.data.fullName' "Maria Souza Lima"

case_ "M3.4 emailVerifiedAt is NULL at birth" "deriveCredential leaves it NIL deliberately — nothing in this service verifies an address, and writing anything would be a claim it cannot support"
assert_json '.data.emailVerifiedAt' "null"

case_ "M3.5 mustChangePassword is TRUE at birth" "always — an admin-set initial password has to be replaced by its owner, and this is the flag the change endpoint clears"
assert_json '.data.mustChangePassword' "true"

case_ "M3.6 passwordChangedAt is present and RFC3339" "always set, since a password is required at creation"
assert_rfc3339 '.data.passwordChangedAt'

case_ "M3.7 archivedAt is present-and-null on a live row" "the BY-ID Response declares the key unconditionally, so it is there carrying null — which is what M3.15b contrasts the listing against"
assert_json '[(.data|has("archivedAt")),(.data.archivedAt == null)] | map(tostring) | join(",")' "true,true"

case_ "M3.8 the ROOT join rides the read" "tenantWorkspace, tenantStatus and tenantArchivedAt — Tenant's columns, filled on every load"
assert_json '[(.data.tenantStatus),(.data.tenantWorkspace|length > 0),(.data|has("tenantArchivedAt"))] | join(",")' "active,true,true"

case_ "M3.9 the three CHILD joins ride the read" "each entry carries its counterpart's key/name/type — the stamps are omitempty and appear only once the counterpart is archived (M10)"
assert_json '[(.data.groups[0]|has("groupKey")),(.data.groups[0]|has("groupName")),
              (.data.roles[0] |has("roleKey")), (.data.roles[0] |has("roleName")),
              (.data.claims[0]|has("claimName")),(.data.claims[0]|has("claimValueType"))] | join(",")' \
  "true,true,true,true,true,true"

case_ "M3.10 the joined VALUES are the counterparts', not blanks" "claimValueType is the definition's declared type, read through the InnerJoinInChild"
assert_json '.data.claims[0].claimValueType' "number"

case_ "M3.11 passwordHash is ABSENT from the by-id read" "absent, not present-and-null — has() is what tells those two apart, and a suite using == null would pass on a field that leaked as an explicit null"
assert_absent '.data | has("passwordHash")'

case_ "M3.12 ...and from the listing row" "the same, on the other read"
api GET "/users?email.eq=$E_GOLD"
assert_json_at 200 '[.data[0] | has("passwordHash"), has("password"), has("passwordConfirmation")] | join(",")' "false,false,false"

case_ "M3.13 ...and from the insert response" "the write echoes the record as stored, and the hash is not part of that record on any wire"
api POST /users "$(user_body "$(user_email nohash)" active "$TEN_LANE")"
assert_json_at 201 '[.data | has("passwordHash"), has("password"), has("passwordConfirmation")] | join(",")' "false,false,false"

case_ "M3.14 ...and from the patch response" "four bodies, one absence"
api PATCH "/users/$ID_GOLD" '{"givenName":"Maria"}'
assert_json_at 200 '[.data | has("passwordHash"), has("password"), has("passwordConfirmation")] | join(",")' "false,false,false"

case_ "M3.15 the listing row carries every field that HAS a value" "fullName, the root join and all three collections — a field silently dropped from one Response and not the other is what this catches"
api GET "/users?email.eq=$E_GOLD"
assert_json_at 200 '[.data[0] | has("fullName"), has("tenantWorkspace"), has("tenantStatus"), has("groups"), has("roles"), has("claims")] | join(",")' \
  "true,true,true,true,true,true"

case_ "M3.15b THE TWO READS DIFFER, and deliberately" "the LISTING omits a null archivedAt (every field of FindUsersResponse is omitempty) while the BY-ID read declares it and carries null. A client reading one and coding against the other is what this pins"
BYID_HAS=$(api GET "/users/$ID_GOLD"; printf "%s" "$HTTP_BODY" | jq -r ".data | has(\"archivedAt\")")
api GET "/users?email.eq=$E_GOLD"
LIST_HAS=$(printf "%s" "$HTTP_BODY" | jq -r ".data[0] | has(\"archivedAt\")")
if [ "$BYID_HAS" = "true" ] && [ "$LIST_HAS" = "false" ]; then pass_; else HTTP_BODY="by-id has=$BYID_HAS listing has=$LIST_HAS"; fail_ "the two reads do not differ as declared"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# M4 — validation, 422, asserting the KEY. One representative per value object.
# ═════════════════════════════════════════════════════════════════════════════════════════

bad_user() { # bad_user JQ_PATCH — the golden body with one field replaced
  jq -nc --arg e "$(user_email bad)" --arg t "$TEN_LANE" --arg p "$QA_USER_PASS1" \
    '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
      password:$p, passwordConfirmation:$p}' | jq -c "$1"
}

case_ "M4.1 an empty givenName" "422 RequiredFieldNotification — the composite reports per PART, under Given"
api POST /users "$(bad_user '.givenName = ""')"
assert_rest 422 RequiredFieldNotification

case_ "M4.2 a givenName that is all punctuation" "422 InvalidPersonNameNotification — hasLetter is one of the five conditions validateNameHalf applies"
api POST /users "$(bad_user '.givenName = "!!!"')"
assert_rest 422 InvalidPersonNameNotification

case_ "M4.3 a familyName with a run of five identical runes" "422 InvalidPersonNameNotification — personNameMaxIdenticalRun is 4"
api POST /users "$(bad_user '.familyName = "Saaaaantos"')"
assert_rest 422 InvalidPersonNameNotification

case_ "M4.4 a familyName that is not trimmed" "422 InvalidPersonNameNotification — isTrimmedAndSingleSpaced"
api POST /users "$(bad_user '.familyName = " Souza"')"
assert_rest 422 InvalidPersonNameNotification

case_ "M4.5 an empty email" "422 RequiredFieldNotification"
api POST /users "$(bad_user '.email = ""')"
assert_rest 422 RequiredFieldNotification

case_ "M4.6 an email with no @" "422 InvalidEmailNotification"
api POST /users "$(bad_user '.email = "nobody"')"
assert_rest 422 InvalidEmailNotification

case_ "M4.7 an UPPERCASE email" "422 InvalidEmailNotification — the pattern is lowercase-only, which is a real constraint a client has to know"
api POST /users "$(bad_user '.email = "Nobody@Acme.com"')"
assert_rest 422 InvalidEmailNotification

case_ "M4.8 an empty password" "422 RequiredFieldNotification"
api POST /users "$(bad_user '.password = "" | .passwordConfirmation = ""')"
assert_rest 422 RequiredFieldNotification

case_ "M4.9 a password under the 8-rune floor" "422 WeakPasswordNotification"
api POST /users "$(bad_user '.password = "Aa1!bcd" | .passwordConfirmation = "Aa1!bcd"')"
assert_rest 422 WeakPasswordNotification

case_ "M4.10 a password missing the digit class" "422 WeakPasswordNotification — the four classes are conjunctive"
api POST /users "$(bad_user '.password = "Abcdefgh!" | .passwordConfirmation = "Abcdefgh!"')"
assert_rest 422 WeakPasswordNotification

case_ "M4.11 a password missing the symbol class" "422 WeakPasswordNotification"
api POST /users "$(bad_user '.password = "Abcdefg1" | .passwordConfirmation = "Abcdefg1"')"
assert_rest 422 WeakPasswordNotification

case_ "M4.12 a password missing the uppercase class" "422 WeakPasswordNotification"
api POST /users "$(bad_user '.password = "abcdefg1!" | .passwordConfirmation = "abcdefg1!"')"
assert_rest 422 WeakPasswordNotification

case_ "M4.13 a password with a trailing space" "422 WeakPasswordNotification — hasLeadingOrTrailingSpace"
api POST /users "$(bad_user '.password = "Abcdefg1! " | .passwordConfirmation = "Abcdefg1! "')"
assert_rest 422 WeakPasswordNotification

case_ "M4.14 a password over the 128-rune ceiling" "422 WeakPasswordNotification — a ceiling exists because Argon2id is not free"
LONGPW="Aa1!$(printf 'x%.0s' $(seq 1 130))"
api POST /users "$(jq -nc --arg e "$(user_email bad)" --arg t "$TEN_LANE" --arg lp "$LONGPW" \
  '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
    password:$lp, passwordConfirmation:$lp}')"
assert_rest 422 WeakPasswordNotification

case_ "M4.15 a status outside the closed set" "422 UnknownUserStatusNotification"
api POST /users "$(bad_user '.status = "pending"')"
assert_rest 422 UnknownUserStatusNotification

case_ "M4.16 a malformed tenantID" "422 InvalidIDUUIDNotification"
api POST /users "$(bad_user '.tenantID = "lixo"')"
assert_rest 422 InvalidIDUUIDNotification

case_ "M4.17 U0 IS A GUARD — a malformed tenantID ENDS the pass" "InvalidIDUUIDNotification ALONE, even though the body carries a second problem the rules would otherwise also report"
api POST /users "$(bad_user '.tenantID = "lixo" | .status = "pending"')"
assert_json_at 422 '[.errors[]?.messages[]?.notificationKey] | unique | join(",")' "InvalidIDUUIDNotification"

# ═════════════════════════════════════════════════════════════════════════════════════════
# M5 — the dual 409. User is the first entity in this suite where BOTH flavors are exercised.
# ═════════════════════════════════════════════════════════════════════════════════════════

E_DUP=$(user_email dup)
ID_DUP=$(new_user "$E_DUP" "$TEN_LANE") || exit 1

case_ "M5.1 the same email twice" "409 UserEmailAlreadyExistsNotification — the DUPLICATE flavor, semantic 'Conflict'"
api POST /users "$(user_body "$E_DUP" active "$TEN_LANE")"
assert_rest 409 UserEmailAlreadyExistsNotification

case_ "M5.1b ...and the envelope says which flavor" "semantic 'Conflict', the string that lets a richer transport map it to ALREADY_EXISTS"
assert_json '[.errors[]?.messages[]?.semantic] | unique | join(",")' "Conflict"

case_ "M5.2 the same email in ANOTHER tenant" "409 — uniqueness is across the WHOLE PLATFORM, not per tenant: one address is one user, never two"
api POST /users "$(user_body "$E_DUP" active "$TEN_OTHER")"
assert_rest 409 UserEmailAlreadyExistsNotification

case_ "M5.3 the same group twice, as two calls" "409 UserAlreadyInGroupNotification — the IsSameBusinessIdentity walk"
attach_group "$ID_DUP" "$G_LANE" >/dev/null
api POST "/users/$ID_DUP/groups" "$(jq -nc --arg g "$G_LANE" '{groupID:$g}')"
assert_rest 409 UserAlreadyInGroupNotification

case_ "M5.4 the same role twice, as two calls" "409 UserAlreadyGrantsRoleNotification"
grant_role_to_user "$ID_DUP" "$R_LANE" >/dev/null
api POST "/users/$ID_DUP/roles" "$(jq -nc --arg r "$R_LANE" '{roleID:$r}')"
assert_rest 409 UserAlreadyGrantsRoleNotification

case_ "M5.5 the same claim twice, as two calls" "409 UserAlreadyHoldsClaimNotification"
set_claim "$ID_DUP" "$C_STR" "one" >/dev/null
api POST "/users/$ID_DUP/claims" "$(jq -nc --arg c "$C_STR" '{claimID:$c, value:"two"}')"
assert_rest 409 UserAlreadyHoldsClaimNotification

case_ "M5.6 the same group id TWICE IN ONE BODY" "409 — the insertOrUpdate path, not the per-call one: the same rule reached through the other door"
api POST /users "$(jq -nc --arg e "$(user_email dup2)" --arg t "$TEN_LANE" --arg p "$QA_USER_PASS1" --arg g "$G_LANE" \
  '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p, groups:[{groupID:$g},{groupID:$g}]}')"
assert_status 409

# ── the WRONG-STATE flavor ────────────────────────────────────────────────────────────────
E_ST=$(user_email st)
ID_ST=$(new_user "$E_ST" "$TEN_LANE") || exit 1

case_ "M5.7 active → suspended" "200 — an allowed edge"
api PATCH "/users/$ID_ST" '{"status":"suspended"}'
assert_json_at 200 '.data.status' "suspended"

case_ "M5.8 suspended → active" "200 — the other allowed edge"
api PATCH "/users/$ID_ST" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "M5.9 a no-op transition" "200 — staying is always allowed; a PATCH carries the value along and must not be hostage to the state machine"
api PATCH "/users/$ID_ST" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "M5.10 a transition to a value outside the map" "409 carrying BOTH keys — the value object reports the unknown member AND the transition rule reports the illegal move; the 409 wins the status because SemanticStateConflict outranks a validation"
api PATCH "/users/$ID_ST" '{"status":"deleted"}'
assert_json_at 409 '[.errors[]?.messages[]?.notificationKey] | unique | sort | join(",")' \
  "InvalidUserStatusTransitionNotification,UnknownUserStatusNotification"

# ═════════════════════════════════════════════════════════════════════════════════════════
# M6 — archive, and the address it releases.
# ═════════════════════════════════════════════════════════════════════════════════════════

E_ARC=$(user_email arc)
ID_ARC=$(new_user "$E_ARC" "$TEN_LANE") || exit 1
CH_ARC_G=$(attach_group "$ID_ARC" "$G_LANE") || exit 1
CH_ARC_R=$(grant_role_to_user "$ID_ARC" "$R_LANE") || exit 1
CH_ARC_C=$(set_claim "$ID_ARC" "$C_STR" "keep") || exit 1

# One entry taken out on its own BEFORE the root's archive, so M6.8 has a stamp to compare.
CH_ARC_G2=$(attach_group "$ID_ARC" "$G_LANE2") || exit 1
api PATCH "/users/$ID_ARC/groups/$CH_ARC_G2/archive"
STAMP_EARLY=$(sql "SELECT archived_at FROM user_groups WHERE id = '$CH_ARC_G2';" | tr -d '[:space:]')

case_ "M6.1 PATCH /users/{id}/archive" "204, no body"
api PATCH "/users/$ID_ARC/archive"
assert_empty_body 204

case_ "M6.2 the archived row leaves the listing" "0 rows — kept but hidden"
api GET "/users?email.eq=$E_ARC"
assert_json_at 200 '.data | length' "0"

case_ "M6.3 ?includeArchived=true reveals it, stamped" "1 row, archivedAt non-null"
api GET "/users?email.eq=$E_ARC&includeArchived=true"
assert_json_at 200 '[(.data|length),(.data[0].archivedAt != null)] | join(",")' "1,true"

case_ "M6.4 the by-id read without the flag" "404 — the default scope refuses an archived row"
api GET "/users/$ID_ARC"
assert_status 404

case_ "M6.5 the by-id read WITH the flag" "200 — the same row, revealed"
api GET "/users/$ID_ARC?includeArchived=true"
assert_json_at 200 '.data.id' "$ID_ARC"

case_ "M6.6 no unarchive route exists" "404 RouteNotFoundNotification — not a 405: nothing is registered at that path at all. Tenant is the ONLY aggregate in this service that mounts one"
api PATCH "/users/$ID_ARC/unarchive"
assert_rest 404 RouteNotFoundNotification

case_ "M6.7 THE ADDRESS COMES BACK" "201 with a NEW id — users_email_key is WHERE archived_at IS NULL, so archiving RELEASES the handle. This is the fact spec.md §5 built the no-unarchive decision on"
api POST /users "$(user_body "$E_ARC" active "$TEN_LANE")"
ID_REBORN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
if [ "$HTTP_STATUS" = "201" ] && [ -n "$ID_REBORN" ] && [ "$ID_REBORN" != "$ID_ARC" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, id '$ID_REBORN' vs '$ID_ARC'"; fi

case_ "M6.8 the root archive CASCADED onto its three collections" "every active entry stamped — 1/1 groups still active before, 1 role, 1 claim"
GOT=$(sql "SELECT (SELECT count(*) FROM user_groups WHERE user_id='$ID_ARC' AND archived_at IS NULL)
        || '/' || (SELECT count(*) FROM user_roles  WHERE user_id='$ID_ARC' AND archived_at IS NULL)
        || '/' || (SELECT count(*) FROM user_claims WHERE user_id='$ID_ARC' AND archived_at IS NULL);" | tr -d '[:space:]')
if [ "$GOT" = "0/0/0" ]; then pass_; else fail_ "active groups/roles/claims = '$GOT'"; fi

case_ "M6.9 the cascade did NOT restamp an entry that was already out" "the early entry keeps its ORIGINAL stamp — the archive scopes its write, it does not sweep the collection"
GOT=$(sql "SELECT archived_at FROM user_groups WHERE id = '$CH_ARC_G2';" | tr -d '[:space:]')
if [ -n "$STAMP_EARLY" ] && [ "$GOT" = "$STAMP_EARLY" ]; then pass_; else fail_ "stamp '$GOT' vs the original '$STAMP_EARLY'"; fi

case_ "M6.10 a collection write onto an archived user" "404 — LoadForWrite runs ScopeActive"
api POST "/users/$ID_ARC/groups" "$(jq -nc --arg g "$G_LANE2" '{groupID:$g}')"
assert_status 404

case_ "M6.11 a PATCH onto an archived user" "404, same scope"
api PATCH "/users/$ID_ARC" '{"givenName":"Ghost"}'
assert_status 404

case_ "M6.12 a password reset onto an archived user" "404 — the credential pair loads through the same scope"
api PATCH "/users/$ID_ARC/password-reset" '{"password":"Qa!Ghost2026","passwordConfirmation":"Qa!Ghost2026"}'
assert_status 404

case_ "M6.13 a second archive of the same row" "404 — it is no longer in the active set to archive"
api PATCH "/users/$ID_ARC/archive"
assert_status 404

# ═════════════════════════════════════════════════════════════════════════════════════════
# M7 — the read vocabulary, one representative per declared operator family.
# ═════════════════════════════════════════════════════════════════════════════════════════

# A dedicated tenant, so the counts below are not hostage to what other lanes created.
TEN_READ=$(new_tenant active "$(ws userr)") || exit 1
E_R1=$(user_email ra); ID_R1=$(new_user "$E_R1" "$TEN_READ") || exit 1
E_R2=$(user_email rb); ID_R2=$(new_user "$E_R2" "$TEN_READ") || exit 1
E_R3=$(user_email rc); ID_R3=$(new_user "$E_R3" "$TEN_READ" suspended) || exit 1
api PATCH "/users/$ID_R1" '{"givenName":"Alice","familyName":"Anderson"}'
api PATCH "/users/$ID_R2" '{"givenName":"Bruno","familyName":"Barros"}'
api PATCH "/users/$ID_R3" '{"givenName":"Carla","familyName":"Castro"}'
Q="tenantID.eq=$TEN_READ"

case_ "M7.1 tenantID.eq" "3 — the lane's own partition"
api GET "/users?$Q"; assert_json_at 200 '.data | length' "3"

case_ "M7.2 givenName.eq" "1"
api GET "/users?$Q&givenName.eq=Alice"; assert_json_at 200 '.data | length' "1"

case_ "M7.3 givenName.icontains" "1 — the case-insensitive fragment an operator actually types"
api GET "/users?$Q&givenName.icontains=lic"; assert_json_at 200 '.data | length' "1"

case_ "M7.4 familyName.startswith" "1"
api GET "/users?$Q&familyName.startswith=Barr"; assert_json_at 200 '.data | length' "1"

case_ "M7.5 familyName.ne" "2 — the operator familyName has and givenName deliberately does not"
api GET "/users?$Q&familyName.ne=Anderson"; assert_json_at 200 '.data | length' "2"

case_ "M7.6 email.in" "2"
api GET "/users?$Q&email.in=$E_R1,$E_R2"; assert_json_at 200 '.data | length' "2"

case_ "M7.7 status.eq" "1 — the suspended one"
api GET "/users?$Q&status.eq=suspended"; assert_json_at 200 '.data | length' "1"

case_ "M7.8 mustChangePassword.eq" "3 — every API-created account is born true"
api GET "/users?$Q&mustChangePassword.eq=true"; assert_json_at 200 '.data | length' "3"

case_ "M7.9 passwordChangedAt.gte" "3 — the filter that exists so 'every credential predating the incident' is answerable"
api GET "/users?$Q&passwordChangedAt.gte=2020-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "3"

case_ "M7.10 emailVerifiedAt.lte" "0 — nothing in this service ever sets it"
api GET "/users?$Q&emailVerifiedAt.lte=2030-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "0"

case_ "M7.11 createdAt.gte" "3"
api GET "/users?$Q&createdAt.gte=2020-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "3"

case_ "M7.12 id.eq" "1"
api GET "/users?id.eq=$ID_R1"; assert_json_at 200 '.data | length' "1"

case_ "M7.13 THE ROOT JOIN IS FILTERABLE — tenantWorkspace.eq" "3 — the reach that distinguishes this backing from the pre-v0.57 one"
WS_READ=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].tenantWorkspace' 2>/dev/null)
api GET "/users?tenantWorkspace.eq=$(printf '%s' "$WS_READ")"
assert_json_at 200 '.data | length' "3"

case_ "M7.14 tenantWorkspace.icontains" "3 — the join leg admits the whole text family"
api GET "/users?tenantWorkspace.icontains=$(printf '%s' "$WS_READ" | cut -c1-8)"
assert_json_at 200 '[.data | length] | .[0] >= 3' "true"

case_ "M7.15 tenantStatus.eq" "3 — the join's other filterable leg"
api GET "/users?$Q&tenantStatus.eq=active"; assert_json_at 200 '.data | length' "3"

case_ "M7.16 ?orderBy=familyName" "Anderson, Barros, Castro — the sort an operator wants, and the reason the composite is split into two columns"
api GET "/users?$Q&orderBy=familyName"
assert_json_at 200 '[.data[].familyName] | join(",")' "Anderson,Barros,Castro"

case_ "M7.17 ?orderBy=-familyName" "the same, reversed"
api GET "/users?$Q&orderBy=-familyName"
assert_json_at 200 '[.data[].familyName] | join(",")' "Castro,Barros,Anderson"

case_ "M7.18 ?orderBy=tenantWorkspace" "200 — the root join is sortable too"
api GET "/users?$Q&orderBy=tenantWorkspace"; assert_status 200

case_ "M7.19 ?fields=fullName" "200 — the computed path fetches its two sources"
api GET "/users?$Q&fields=fullName&orderBy=familyName"
assert_json_at 200 '.data[0].fullName' "Alice Anderson"

case_ "M7.20 ?fields= a CHILD JOIN path" "200 — a child join's fields ARE addressable in a projection, as <segment>.<field>"
api GET "/users?tenantID.eq=$TEN_LANE&fields=groups.groupKey,roles.roleKey,claims.claimName&first=1"
assert_status 200

case_ "M7.21 ?onlyTotal=true" "a count and no data array at all"
api GET "/users?$Q&onlyTotal=true"
assert_json_at 200 '[(.pagination.totalCount),(has("data") and (.data != null))] | join(",")' "3,false"

case_ "M7.22 ?onlyTotal=true beside a filter" "counting a filtered subset is the canonical use"
api GET "/users?$Q&status.eq=active&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "2"

case_ "M7.23 the pagination envelope, page 1" "first=2 → 2 rows, hasNextPage true, hasPreviousPage false, and an endCursor. A cursor is a WINDOW EDGE, so page 1 carries no startCursor: there is no previous edge for one to point at"
api GET "/users?$Q&orderBy=familyName&first=2"
assert_json_at 200 '[(.data|length),(.pagination.totalCount),(.pagination.hasNextPage),(.pagination.hasPreviousPage),(.pagination.endCursor|length>0),(.pagination|has("startCursor"))] | map(tostring) | join(",")' \
  "2,3,true,false,true,false"
CUR_END=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')

case_ "M7.24 the walk — page 2 by echoing endCursor into ?after=" "the remaining row, hasNextPage false, hasPreviousPage true"
api GET "/users?$Q&orderBy=familyName&first=2&after=$CUR_END"
assert_json_at 200 '[(.data|length),(.data[0].familyName),(.pagination.hasNextPage),(.pagination.hasPreviousPage)] | join(",")' \
  "1,Castro,false,true"

case_ "M7.25 the two pages are DISJOINT" "Castro is not on page 1 — a cursor that did not advance is the failure this catches"
api GET "/users?$Q&orderBy=familyName&first=2"
assert_json_at 200 '[.data[].familyName] | index("Castro") | . == null' "true"

case_ "M7.26 ?last= alone serves the TAIL window" "the last row in the ordering, not the first"
api GET "/users?$Q&orderBy=familyName&last=1"
assert_json_at 200 '[(.data|length),(.data[0].familyName)] | join(",")' "1,Castro"

# ═════════════════════════════════════════════════════════════════════════════════════════
# M8 — rejected reads. The whole typed-400 family, and the password oracle.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "M8.1 an unknown filter key" "400 SchemaViolationNotification — never a silently ignored parameter"
api GET "/users?bogus.eq=x"; assert_rest 400 SchemaViolationNotification

case_ "M8.2 an operator outside a leaf's allowlist" "400 — status declares eq,in and nothing else"
api GET "/users?status.contains=act"; assert_rest 400 SchemaViolationNotification

case_ "M8.3 givenName.ne — the operator familyName has and givenName does not" "400 — the allowlist is per LEAF, and the asymmetry is deliberate"
api GET "/users?givenName.ne=Alice"; assert_rest 400 SchemaViolationNotification

case_ "M8.4 ?search= on a DTO that never declared it" "400 SchemaViolationNotification on 'search' — the opt-in gate, reached BEFORE any engine. spec.md §9 records the decision: a relational view answers free text with a refusal, so declaring it would advertise a capability the server does not have"
api GET "/users?search=maria"; assert_rest 400 SchemaViolationNotification

case_ "M8.5 an undeclared reserved control on the BY-ID read" "400 — FindUserByIDRequest declares includeArchived alone, and PRESENCE is what trips the gate"
api GET "/users/$ID_R1?fields=email"; assert_rest 400 SchemaViolationNotification

case_ "M8.6 ...even when the undeclared control is INACTIVE" "400 — ?onlyTotal=false on the by-id read still rejects: presence, not value"
api GET "/users/$ID_R1?onlyTotal=false"; assert_rest 400 SchemaViolationNotification

case_ "M8.7 an unresolvable ?fields= path" "400 SchemaViolationNotification naming fields[bogus]"
api GET "/users?fields=bogus"; assert_rest_field 400 SchemaViolationNotification "fields[bogus]"

case_ "M8.8 a filter VALUE outside the leaf's kind — a timestamp" "400 InvalidFilterValueNotification (pin >= v0.70.0; below it this was a 500 on a relational backing)"
api GET "/users?emailVerifiedAt.gte=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "M8.9 a filter VALUE outside the leaf's kind — an identity column" "400 InvalidFilterValueNotification"
api GET "/users?tenantID.eq=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "M8.10 a filter VALUE outside the leaf's kind — a bool" "400 InvalidFilterValueNotification"
api GET "/users?mustChangePassword.eq=perhaps"; assert_rest 400 InvalidFilterValueNotification

case_ "M8.11 ?first= above the view's ceiling" "400 — the page ceiling is 100"
api GET "/users?first=101"; assert_status 400

case_ "M8.12 mixed directions — first + last" "400"
api GET "/users?first=2&last=2"; assert_status 400

case_ "M8.13 mixed directions — first + before" "400; backward is last+before"
api GET "/users?first=2&before=abc"; assert_status 400

case_ "M8.14 ?onlyTotal=true beside a page-shaping control" "400 — the only-total conflict matrix"
api GET "/users?onlyTotal=true&first=10"; assert_status 400

case_ "M8.15 a malformed cursor" "400 SchemaViolationNotification"
api GET "/users?first=2&after=not-a-cursor"; assert_rest 400 SchemaViolationNotification

case_ "M8.16 a cursor against a DIFFERENT orderBy" "400 — a cursor is only meaningful inside the ordering that minted it"
api GET "/users?$Q&orderBy=familyName&first=2" >/dev/null
CUR_MM=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
api GET "/users?$Q&orderBy=givenName&first=2&after=$CUR_MM"; assert_status 400

case_ "M8.17 ?orderBy=fullName — the computed path" "400 on orderBy[fullName] — a computed path backs no column. THE FIELD IS fullName, not `name`: spec.md §9 said `name` until 2026-09-07 and it never resolved"
api GET "/users?orderBy=fullName"; assert_rest_field 400 SchemaViolationNotification "orderBy[fullName]"

case_ "M8.18 ?orderBy=name — the name §9 used to claim" "400 — for the different and less interesting reason that no such field exists on this read model"
api GET "/users?orderBy=name"; assert_rest 400 SchemaViolationNotification

# ── the 1:N boundary, from the side that REFUSES ──────────────────────────────────────────
case_ "M8.19 a filter over a CHILD JOIN field — groups" "400 — served does not mean addressable. 'Who is in Engineering?' is not answerable from this listing, which is spec.md §9's one real cost"
api GET "/users?groups.groupKey.eq=engineering"; assert_rest 400 SchemaViolationNotification

case_ "M8.20 ...roles" "400, same boundary"
api GET "/users?roles.roleKey.eq=admin"; assert_rest 400 SchemaViolationNotification

case_ "M8.21 ...claims" "400, same boundary"
api GET "/users?claims.claimName.eq=x_cost_center"; assert_rest 400 SchemaViolationNotification

case_ "M8.22 an ORDER over a child-join field" "400 — the boundary holds for sorting too"
api GET "/users?orderBy=groups.groupKey"; assert_rest 400 SchemaViolationNotification

# ── THE PASSWORD ORACLE. The four cases spec.md §9 says nothing automatic provides. ───────
case_ "M8.23 ?passwordHash.eq=<a real PHC string>" "400 SchemaViolationNotification — nothing declares a filter: tag on that field, on any surface, ever"
api GET "/users?passwordHash.eq=%24argon2id%24v%3D19%24m%3D19456%2Ct%3D2%2Cp%3D1%24AAAA"
assert_rest 400 SchemaViolationNotification

case_ "M8.24 ?passwordHash.startswith=\$a — the walk, one character at a time" "400 — this is the request the empty filter: cell exists to refuse, and the case that fails the day somebody fills it in"
api GET "/users?passwordHash.startswith=%24a"; assert_rest 400 SchemaViolationNotification

case_ "M8.25 ?orderBy=passwordHash" "400 — an ordering over a hash is an oracle by another route"
api GET "/users?orderBy=passwordHash"; assert_rest 400 SchemaViolationNotification

case_ "M8.26 ?fields=passwordHash" "400 SchemaViolationNotification — a stored field that no Response declares is NOT selectable. Never a silent 200 {}, which would leave open whether the column was read"
api GET "/users?fields=passwordHash"; assert_rest 400 SchemaViolationNotification

case_ "M8.27 ?fields=password — the plaintext field name" "400 — it has no column at all, and asking by its wire name must not resolve either"
api GET "/users?fields=password"; assert_rest 400 SchemaViolationNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# M9 — absent verbs, wrong addresses, malformed ids.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "M9.1 DELETE /users/{id}" "405 MethodNotAllowedNotification — soft-delete only, and the PATH is registered (GET and PATCH live there), so the refusal is about the METHOD. The 404 shape belongs to a path nothing registers at all, which M6.6 asserts on unarchive"
api DELETE "/users/$ID_R1"; assert_rest 405 MethodNotAllowedNotification

case_ "M9.2 a mounted path under another method" "405 — POST /users/{id} is not registered, but the path is"
api POST "/users/$ID_R1" '{}'; assert_status 405

case_ "M9.3 a valid-but-absent uuid, on the by-id read" "404"
api GET "/users/01990000-dead-7000-8000-000000000000"; assert_status 404

case_ "M9.4 ...on a PATCH" "404"
api PATCH "/users/01990000-dead-7000-8000-000000000000" '{"givenName":"Nobody"}'; assert_status 404

case_ "M9.5 ...on an archive" "404"
api PATCH "/users/01990000-dead-7000-8000-000000000000/archive"; assert_status 404

case_ "M9.6 ...on a collection add" "404 — the owner is not there"
api POST "/users/01990000-dead-7000-8000-000000000000/groups" "$(jq -nc --arg g "$G_LANE" '{groupID:$g}')"
assert_status 404

case_ "M9.7 a by-id address that is not a uuid, on a READ" "404 UnknownIDAddressNotification (pin >= v0.70.0)"
api GET "/users/lixo"; assert_rest 404 UnknownIDAddressNotification

case_ "M9.8 the same address on a WRITE" "400 MalformedIDNotification — the split is by VERB, not by surface"
api PATCH "/users/lixo" '{"givenName":"Nobody"}'; assert_rest 400 MalformedIDNotification

case_ "M9.9 ...on the archive verb" "400 MalformedIDNotification"
api PATCH "/users/lixo/archive"; assert_rest 400 MalformedIDNotification

case_ "M9.10 a malformed CHILD address" "404 RecordNotFoundNotification on field 'claims' — the v0.70.0 MalformedID split governs the ROOT by-id address; a child segment is matched against the owner's collection, so an unmatched one is the collection's own not-found, addressed to the plural"
api PATCH "/users/$ID_R1/claims/lixo" '{"value":"x"}'; assert_rest 404 RecordNotFoundNotification

case_ "M9.11 a child id that is a valid uuid but belongs to ANOTHER user" "404 — an entry is addressed inside its owner, never globally"
E_X1=$(user_email x1); ID_X1=$(new_user "$E_X1" "$TEN_LANE") || exit 1
E_X2=$(user_email x2); ID_X2=$(new_user "$E_X2" "$TEN_LANE") || exit 1
CH_X1=$(attach_group "$ID_X1" "$G_LANE") || exit 1
api PATCH "/users/$ID_X2/groups/$CH_X1/archive"; assert_status 404

case_ "M9.12 an absent child id under the right owner" "404 RecordNotFoundNotification — the caller named an entry, so a missing one is an answer rather than a no-op"
api PATCH "/users/$ID_X1/groups/01990000-dead-7000-8000-000000000000/archive"
assert_rest 404 RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# M10 — the four read joins, and the half of each that is easy to get wrong.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "M10.1 the ROOT join reports the tenant's own archive stamp" "tenantArchivedAt is null while the tenant is live"
api GET "/users/$ID_R1"; assert_json_at 200 '.data.tenantArchivedAt' "null"

# An entry whose counterpart is archived AFTER the entry was made. The INNER join matches on
# the id and not on the stamp, so the entry survives — which is exactly what the served stamp
# is for, and the case a suite would otherwise never think to write.
E_OUT=$(user_email out); ID_OUT=$(new_user "$E_OUT" "$TEN_LANE") || exit 1
G_DOOMED=$(new_group "$(group_key doom)" "$TEN_LANE" "$R_LANE") || exit 1
R_DOOMED=$(new_role "$(role_key doom)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
C_DOOMED=$(new_claim "$(claim_name doom)" string both "$TEN_LANE") || exit 1
attach_group "$ID_OUT" "$G_DOOMED" >/dev/null
grant_role_to_user "$ID_OUT" "$R_DOOMED" >/dev/null
set_claim "$ID_OUT" "$C_DOOMED" "still-here" >/dev/null
api PATCH "/groups/$G_DOOMED/archive"
api PATCH "/roles/$R_DOOMED/archive"
api PATCH "/claims/$C_DOOMED/archive"

api GET "/users/$ID_OUT"

case_ "M10.2 an entry OUTLIVES the group it points at" "the entry is still there, groupArchivedAt now non-null, and groupKey/groupName still resolved — the join matches on the id, not on the stamp"
assert_json_at 200 '[(.data.groups|length),(.data.groups[0].groupArchivedAt != null),(.data.groups[0].groupKey|length > 0)] | join(",")' "1,true,true"

case_ "M10.3 ...the role it points at" "same shape, one hop over"
assert_json '[(.data.roles|length),(.data.roles[0].roleArchivedAt != null),(.data.roles[0].roleKey|length > 0)] | join(",")' "1,true,true"

case_ "M10.4 ...the claim definition it points at" "same shape, and claimValueType still reads the definition"
assert_json '[(.data.claims|length),(.data.claims[0].claimArchivedAt != null),(.data.claims[0].claimValueType)] | join(",")' "1,true,string"

case_ "M10.5 the entry's OWN stamp is still null" "the counterpart was archived, the membership was not — two different rows and two different stamps"
assert_json '.data.groups[0] | has("archivedAt")' "false"

# ═════════════════════════════════════════════════════════════════════════════════════════
# M11 — route inventory. An ENUMERATION of what exists, never an expectation about answers.
# ═════════════════════════════════════════════════════════════════════════════════════════

api GET /openapi.json "" -
OPENAPI="$HTTP_BODY"

case_ "M11.1 openapi.json enumerates fourteen /users operations" "the five root verbs, the seven collection verbs and the two credential verbs"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key | startswith("/users")) | .value | to_entries[] | select(.key | test("^(get|post|patch|put|delete)$"))] | length')
if [ "$GOT" = "14" ]; then pass_; else HTTP_BODY="$(printf '%s' "$OPENAPI" | jq -c '[.paths | keys[] | select(startswith("/users"))]')"; fail_ "operations = $GOT"; fi

case_ "M11.2 every /users path the source declares is registered" "the nine distinct path templates of user_routes.go and user_credential_routes_manual.go"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | keys[] | select(startswith("/users"))] | sort | join(" ")')
WANT="/users/ /users/{id} /users/{id}/archive /users/{id}/claims /users/{id}/claims/{userClaimId} /users/{id}/claims/{userClaimId}/archive /users/{id}/groups /users/{id}/groups/{userGroupId}/archive /users/{id}/password /users/{id}/password-reset /users/{id}/roles /users/{id}/roles/{userRoleId}/archive"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="$GOT"; fail_ "paths differ"; fi

case_ "M11.3 no /users/{id}/unarchive is registered" "the absence M6.6 asserts from the other side"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | keys[] | select(test("/users.*unarchive"))] | length')
if [ "$GOT" = "0" ]; then pass_; else HTTP_BODY="$OPENAPI"; fail_ "unarchive paths = $GOT"; fi

case_ "M11.4 the seven permission literals ride the operations" "user:read/insert/update/archive/grant/set-claim/change-password/reset-password — eight distinct, over fourteen operations"
GOT=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key|startswith("/users")) | .value | to_entries[] | .value | (.["x-required-permission"] // (.description // "")) ] | join(" ")' \
  | grep -oE 'user:[a-z]+(-[a-z]+)*' | sort -u | tr '\n' ' ' | sed 's/ $//')
WANT="user:archive user:change-password user:grant user:insert user:read user:reset-password user:set-claim user:update"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="found: $GOT"; fail_ "literals differ"; fi

qa_finish
