#!/usr/bin/env bash
# Lane: user_graphql — the GraphQL half of the framework contract for the User aggregate.
#
# Family N of specs/qa/user-contract/plan.md §1, plus the M families repeated in this surface's
# own idiom. A route gated on REST is not thereby gated on GraphQL, and "the other surface
# forgot it" is a real regression that only a per-surface case catches.
#
# TWO IDIOMS, and the difference is BY DESIGN — the suite must not flatten it:
#   · a typed domain refusal answers HTTP 200 with the same notificationKey the REST envelope
#     carries, in errors[].extensions;
#   · a request the SCHEMA cannot express is cut by gqlparser before any resolver runs and
#     surfaces as a validation message with no notificationKey at all.
#
# WHAT THIS LANE EXISTS TO PIN, and it is the sharpest §0c item of the round. spec.md §9 said
# "GraphQL: yes, root verbs only … the four child operations and both password operations are
# REST-only", and §10 repeated it as "Neither is on GraphQL". BOTH SENTENCES WERE WRONG:
# MountUsersGraphQL registers 12 fields and MountUserCredentialsGraphQL registers 2 more, for
# 14 against REST's 14 — full parity. The prose was corrected on 2026-09-07; N1, N5 and N6 are
# what keep it correct, because a suite written from the old text would have asserted the
# absence of nine fields that are there.
#
# The row rules travel unchanged, which is what makes the parity safe: "must be self" and
# "must not be self" live in the aggregate, fed from the identity on the AppContext, not by
# anything the transport does. N5.3 asserts exactly that.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init user_graphql

# Every field the User type declares. The three entry selections are COMPLETE — nothing on
# any of the three collections is hidden — and `passwordHash` is in none of them, which N2.4
# asserts from the schema's own side.
G_ENTRY='id groupID groupKey groupName groupArchivedAt'
R_ENTRY='id roleID roleKey roleName roleArchivedAt'
C_ENTRY='id claimID value claimName claimValueType claimArchivedAt'
NODE="id tenantID givenName familyName fullName email emailVerifiedAt passwordChangedAt mustChangePassword status createdAt updatedAt archivedAt tenantWorkspace tenantStatus tenantArchivedAt groups { $G_ENTRY } roles { $R_ENTRY } claims { $C_ENTRY }"

TEN_N=$(new_tenant active "$(ws gqluser)") || exit 1
P_TENANT_READ=$(permission_id_of tenant read)
[ -n "$P_TENANT_READ" ] || { echo "user_graphql.sh: a seeded catalog id could not be resolved" >&2; exit 1; }

R_N=$(new_role "$(role_key gqlu)" "$TEN_N" "$P_TENANT_READ") || exit 1
G_N=$(new_group "$(group_key gqlu)" "$TEN_N" "$R_N") || exit 1
C_N=$(new_claim "$(claim_name gqlu)" string both "$TEN_N") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# N1 — the fourteen mounted fields
# ═════════════════════════════════════════════════════════════════════════════════════════

E_N=$(user_email gql)
case_ "N1.1 createUser" "the record as stored, under data.createUser"
gql "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id tenantID givenName familyName email status fullName groups { id groupID } roles { id roleID } claims { id claimID value } } }" \
    "$(jq -nc --arg e "$E_N" --arg t "$TEN_N" --arg p "$QA_USER_PASS1" --arg g "$G_N" --arg r "$R_N" --arg c "$C_N" \
       '{i:{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
            password:$p, passwordConfirmation:$p,
            groups:[{groupID:$g}], roles:[{roleID:$r}], claims:[{claimID:$c, value:"1000"}]}}')"
assert_gql_ok '.data.createUser.email' "$E_N"
ID_N=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createUser.id')
[ -n "$ID_N" ] && [ "$ID_N" != "null" ] || { echo "user_graphql.sh: createUser did not answer an id: $HTTP_BODY" >&2; exit 1; }

case_ "N1.1b HANDLER INVARIANCE: the mutation's effect is visible over REST immediately" "the same address, read through the other surface — one handler, no second implementation"
api GET "/users/$ID_N"
assert_json_at 200 '.data.email' "$E_N"

case_ "N1.2 user(id:)" "the singular node, every declared field selectable"
gql "query(\$id: ID!) { user(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_N" '{id:$id}')"
assert_gql_ok '.data.user.email' "$E_N"

case_ "N1.3 users(...)" "the Relay connection over the same view the REST listing reads — edges/node, and every filter under ONE where: argument"
gql "query { users(where: {email: {eq: \"$E_N\"}}) { totalCount edges { cursor node { id email } } pageInfo { hasNextPage hasPreviousPage } } }"
assert_gql_ok '.data.users.totalCount' "1"

case_ "N1.3b the connection node is the same document the REST row is" "the same address under edges[].node — one view, one Response, two renderings"
assert_gql_ok '.data.users.edges[0].node.email' "$E_N"

case_ "N1.4 patchUser" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchUserInput!) { patchUser(id: \$id, input: \$i) { id givenName } }" \
    "$(jq -nc --arg id "$ID_N" '{id:$id, i:{givenName:"Renamed"}}')"
assert_gql_ok '.data.patchUser.givenName' "Renamed"

E_NC=$(user_email gqlc)
gql "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id } }" \
    "$(jq -nc --arg e "$E_NC" --arg t "$TEN_N" --arg p "$QA_USER_PASS1" \
       '{i:{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t, password:$p, passwordConfirmation:$p}}')"
ID_NC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createUser.id')

case_ "N1.5 addUserGroup" "the entry as stored, with the id the server minted"
gql "mutation(\$id: ID!, \$i: AddUserGroupInput!) { addUserGroup(id: \$id, input: \$i) { userId userGroup { id groupID } } }" \
    "$(jq -nc --arg id "$ID_NC" --arg g "$G_N" '{id:$id, i:{groupID:$g}}')"
assert_gql_ok '.data.addUserGroup.userGroup.groupID' "$G_N"
CHN_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addUserGroup.userGroup.id')

case_ "N1.6 archiveUserGroup" "REST answers 204 with no body; a GraphQL field must answer SOMETHING, so the payload is the acknowledgement and nothing more"
gql "mutation(\$id: ID!, \$i: ArchiveUserGroupInput!) { archiveUserGroup(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NC" --arg c "$CHN_G" '{id:$id, i:{userGroupId:$c}}')"
assert_gql_ok '.data.archiveUserGroup.success' "true"

case_ "N1.7 addUserRole" "the entry as stored"
gql "mutation(\$id: ID!, \$i: AddUserRoleInput!) { addUserRole(id: \$id, input: \$i) { userId userRole { id roleID } } }" \
    "$(jq -nc --arg id "$ID_NC" --arg r "$R_N" '{id:$id, i:{roleID:$r}}')"
assert_gql_ok '.data.addUserRole.userRole.roleID' "$R_N"
CHN_R=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addUserRole.userRole.id')

case_ "N1.8 archiveUserRole" "{success}"
gql "mutation(\$id: ID!, \$i: ArchiveUserRoleInput!) { archiveUserRole(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NC" --arg c "$CHN_R" '{id:$id, i:{userRoleId:$c}}')"
assert_gql_ok '.data.archiveUserRole.success' "true"

case_ "N1.9 addUserClaim" "the entry as stored, id + claimID + value"
gql "mutation(\$id: ID!, \$i: AddUserClaimInput!) { addUserClaim(id: \$id, input: \$i) { userId userClaim { id claimID value } } }" \
    "$(jq -nc --arg id "$ID_NC" --arg c "$C_N" '{id:$id, i:{claimID:$c, value:"alpha"}}')"
assert_gql_ok '.data.addUserClaim.userClaim.value' "alpha"
CHN_C=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addUserClaim.userClaim.id')

case_ "N1.10 patchUserClaim" "the partial shape of the same operation, under its own field — the schema has no verb to carry the difference, so the NAME does"
gql "mutation(\$id: ID!, \$i: PatchUserClaimInput!) { patchUserClaim(id: \$id, input: \$i) { userClaim { id value } } }" \
    "$(jq -nc --arg id "$ID_NC" --arg c "$CHN_C" '{id:$id, i:{userClaimId:$c, value:"beta"}}')"
assert_gql_ok '.data.patchUserClaim.userClaim.value' "beta"

case_ "N1.11 ...and it kept the entry's id" "the SAME child id, on this surface too"
assert_gql_ok '.data.patchUserClaim.userClaim.id' "$CHN_C"

case_ "N1.12 archiveUserClaim" "{success}"
gql "mutation(\$id: ID!, \$i: ArchiveUserClaimInput!) { archiveUserClaim(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NC" --arg c "$CHN_C" '{id:$id, i:{userClaimId:$c}}')"
assert_gql_ok '.data.archiveUserClaim.success' "true"

case_ "N1.13 archiveUser" "{success} — the bodyless REST verb's payload twin"
gql "mutation(\$id: ID!) { archiveUser(id: \$id) { success } }" "$(jq -nc --arg id "$ID_NC" '{id:$id}')"
assert_gql_ok '.data.archiveUser.success' "true"

case_ "N1.13b ...and the archive is visible over REST" "404 without the flag — the same scope, reached from the other surface"
api GET "/users/$ID_NC"
assert_status 404

# ═════════════════════════════════════════════════════════════════════════════════════════
# N2 — the golden record on this surface, and the absence that matters most
# ═════════════════════════════════════════════════════════════════════════════════════════

gql "query(\$id: ID!) { user(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_N" '{id:$id}')"

case_ "N2.1 every scalar round-trips identically to REST" "one view, one Response, two renderings"
assert_gql_ok '[.data.user.givenName,.data.user.familyName,.data.user.email,.data.user.status] | join("|")' "Renamed|Fixture|$E_N|active"

case_ "N2.2 fullName is served here too" "the computed field comes from FromQueryResult, so both surfaces agree by construction"
assert_gql_ok '.data.user.fullName' "Renamed Fixture"

case_ "N2.3 the three CHILD joins are selectable" "nothing on any of the three collections is hidden"
assert_gql_ok '[(.data.user.groups[0].groupKey|length>0),(.data.user.roles[0].roleKey|length>0),(.data.user.claims[0].claimValueType)] | join(",")' "true,true,string"

case_ "N2.4 passwordHash is not a field of the User TYPE" "'Cannot query field' — on this surface the refusal comes from the SCHEMA, before any resolver, which is a stronger guarantee than a body that happens to omit it"
gql "query(\$id: ID!) { user(id: \$id) { id passwordHash } }" "$(jq -nc --arg id "$ID_N" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "N2.5 nor is the plaintext password" "'Cannot query field' — it has no column and no Response member on either surface"
gql "query(\$id: ID!) { user(id: \$id) { id password } }" "$(jq -nc --arg id "$ID_N" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "N2.6 the COMPOSITE's own name is not a field either" "givenName and familyName are the only names any surface speaks"
gql "query(\$id: ID!) { user(id: \$id) { id name } }" "$(jq -nc --arg id "$ID_N" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "N2.7 the WRITE payload carries no traversal field" "'Cannot query field groupKey' on the write payload's entry type — the write side never speaks the traversal, on either surface"
gql "mutation(\$id: ID!, \$i: AddUserGroupInput!) { addUserGroup(id: \$id, input: \$i) { userGroup { groupKey } } }" \
    "$(jq -nc --arg id "$ID_N" --arg g "$G_N" '{id:$id, i:{groupID:$g}}')"
assert_gql_validation "Cannot query field"

# ═════════════════════════════════════════════════════════════════════════════════════════
# N3 — a typed refusal rides errors[].extensions with HTTP 200
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "N3.1 a duplicate email" "HTTP 200 carrying UserEmailAlreadyExistsNotification — never the REST envelope"
gql "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id } }" \
    "$(jq -nc --arg e "$E_N" --arg t "$TEN_N" --arg p "$QA_USER_PASS1" \
       '{i:{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t, password:$p, passwordConfirmation:$p}}')"
assert_gql UserEmailAlreadyExistsNotification

case_ "N3.2 a weak password" "200 + WeakPasswordNotification"
gql "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id } }" \
    "$(jq -nc --arg e "$(user_email gqlw)" --arg t "$TEN_N" \
       '{i:{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t, password:"abc", passwordConfirmation:"abc"}}')"
assert_gql WeakPasswordNotification

case_ "N3.3 a mismatched confirmation" "200 + PasswordConfirmationMismatchNotification"
gql "mutation(\$i: CreateUserInput!) { createUser(input: \$i) { id } }" \
    "$(jq -nc --arg e "$(user_email gqlm)" --arg t "$TEN_N" --arg p "$QA_USER_PASS1" \
       '{i:{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t, password:$p, passwordConfirmation:"Other!Pass2026"}}')"
assert_gql PasswordConfirmationMismatchNotification

case_ "N3.4 an absent id on the singular read" "200 with a null node AND RecordNotFoundNotification in errors[].extensions — both halves, because a null node with no error would be indistinguishable from a row that exists and is empty"
gql "query { user(id: \"01990000-dead-7000-8000-000000000000\") { id } }"
assert_gql RecordNotFoundNotification

case_ "N3.4b ...and the node itself is null" "the other half of the same answer"
assert_json '.data.user' "null"

case_ "N3.5 a by-id address that is not a uuid, on a READ" "200 + UnknownIDAddressNotification — the same key REST answers, the split by VERB and not by surface"
gql "query { user(id: \"lixo\") { id } }"
assert_gql UnknownIDAddressNotification

case_ "N3.6 the same address on a WRITE" "200 + MalformedIDNotification — the other half of the v0.70.0 split, identical across surfaces"
gql "mutation(\$i: PatchUserInput!) { patchUser(id: \"lixo\", input: \$i) { id } }" '{"i":{"givenName":"Nobody"}}'
assert_gql MalformedIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# N4 — the surface's OWN idiom: what the schema cannot express is cut before any resolver
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "N4.1 an undeclared filter argument" "a validation message with NO notificationKey — gqlparser, not the read engine"
gql "query { users(bogus: { eq: \"x\" }) { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "N4.2 a filter over a CHILD JOIN field" "not a field of UserWhereInput — served does not mean addressable, on this surface either, and here the 1:N boundary is enforced by the schema rather than by an engine"
gql "query { users(where: {groups: {eq: \"engineering\"}}) { totalCount } }"
assert_gql_validation "groups"

case_ "N4.3 a filter over passwordHash" "not a field of UserWhereInput — the oracle is unreachable here for the strongest possible reason available anywhere in this suite: the schema has no name for it, so no request can even be composed"
gql "query { users(where: {passwordHash: {eq: \"\$argon2id\"}}) { totalCount } }"
assert_gql_validation "passwordHash"

case_ "N4.4 an operator outside a leaf's allowlist" "the operator input User_status_Op declares eq and in, so `contains` is a field that type does not define — the allowlist reaches the SCHEMA here, where REST enforces it at the wire wrapper"
gql "query { users(where: {status: {contains: \"act\"}}) { totalCount } }"
assert_gql_validation "contains"

case_ "N4.4b an unknown LEAF inside where:" "UserWhereInput declares one field per filterable leaf and no others"
gql "query { users(where: {bogus: {eq: \"x\"}}) { totalCount } }"
assert_gql_validation "bogus"

case_ "N4.5 ?search= has no GraphQL twin either" "Unknown argument — the DTO declares none, so the schema grew none"
gql "query { users(search: \"maria\") { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "N4.6 selection is NATURAL here — there is no fields argument to gate" "Unknown argument: a caller shapes the response by selecting, which is why REST's ?fields= has no counterpart to reject"
gql "query { users(fields: \"email\") { totalCount } }"
assert_gql_validation "Unknown argument"

# ═════════════════════════════════════════════════════════════════════════════════════════
# N5 — the two credential fields §9 and §10 both said were not here
# ═════════════════════════════════════════════════════════════════════════════════════════

# The account needs BOTH credential permissions: the must-change token embeds only
# user:change-password and only for the FIRST rotation, and N5.5 points the reset at itself,
# which has to be refused by the ROW rule and not by the gate.
E_NP=$(user_email gqlp)
ID_NP=$(new_user "$E_NP" "$TEN_N") || exit 1
R_NP=$(new_role "$(role_key gqlp)" "$TEN_N" \
  "$(permission_id_of user change-password)" "$(permission_id_of user reset-password)") || exit 1
grant_role_to_user "$ID_NP" "$R_NP" >/dev/null
T_NP=$(usable_token "$E_NP" "$ID_NP") || exit 1

case_ "N5.1 changeUserPassword EXISTS" "{success} — spec.md §9 said 'REST-only' and §10 said 'Neither is on GraphQL'; both were corrected on 2026-09-07 and this is the case that keeps them correct"
gql "mutation(\$id: ID!, \$i: ChangeUserPasswordInput!) { changeUserPassword(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NP" --arg c "$QA_USER_PASS2" '{id:$id, i:{currentPassword:$c, password:"Qa!Gql2026x", passwordConfirmation:"Qa!Gql2026x"}}')" \
    "$T_NP"
assert_gql_ok '.data.changeUserPassword.success' "true"

case_ "N5.2 ...and it really changed the credential" "the new password signs in; the old one does not"
NEWTOK=$(qa_login "$E_NP" 'Qa!Gql2026x'); OLDTOK=$(qa_login "$E_NP" "$QA_USER_PASS2")
if [ -n "$NEWTOK" ] && [ -z "$OLDTOK" ]; then pass_; else HTTP_BODY="new='${NEWTOK:0:12}…' old='${OLDTOK:0:12}…'"; fail_ "new token empty=$([ -z "$NEWTOK" ] && echo yes || echo no), old token still valid=$([ -n "$OLDTOK" ] && echo yes || echo no)"; fi

case_ "N5.3 THE ROW RULE TRAVELS UNCHANGED" "200 + PasswordChangeRequiresSelfNotification — a caller reaching this field with somebody else's id meets the SAME refusal it meets on REST, because the decision lives in the aggregate and not in the transport"
gql "mutation(\$id: ID!, \$i: ChangeUserPasswordInput!) { changeUserPassword(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_N" --arg c "$QA_USER_PASS1" '{id:$id, i:{currentPassword:$c, password:"Qa!Other2026x", passwordConfirmation:"Qa!Other2026x"}}')" \
    "$T_NP"
assert_gql PasswordChangeRequiresSelfNotification

case_ "N5.4 resetUserPassword EXISTS" "{success} — on somebody else's row, as the admin"
E_NR=$(user_email gqlr); ID_NR=$(new_user "$E_NR" "$TEN_N") || exit 1
gql "mutation(\$id: ID!, \$i: ResetUserPasswordInput!) { resetUserPassword(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NR" '{id:$id, i:{password:"Qa!GqlReset26", passwordConfirmation:"Qa!GqlReset26"}}')"
assert_gql_ok '.data.resetUserPassword.success' "true"

case_ "N5.5 the reset's mirror rule travels too" "200 + PasswordResetRequiresAnotherUserNotification — pointing it at one's OWN row is refused here exactly as on REST, which is what stops a stolen token laundering itself by choosing the other surface"
gql "mutation(\$id: ID!, \$i: ResetUserPasswordInput!) { resetUserPassword(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_NP" '{id:$id, i:{password:"Qa!Self2026xy", passwordConfirmation:"Qa!Self2026xy"}}')" \
    "$T_NP"
assert_gql PasswordResetRequiresAnotherUserNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# N6 — the field inventory, asserted from the SCHEMA rather than by calling each one
# ═════════════════════════════════════════════════════════════════════════════════════════

# __schema { queryType { fields } } answers NULL on this server — that traversal is restricted.
# __type(name: "Query") { fields } is the shape that answers, and it asks the same question.
gql 'query { q: __type(name: "Query") { fields { name } } m: __type(name: "Mutation") { fields { name } } }'
GQL_TYPES="$HTTP_BODY"
gql_user_fields() { printf '%s' "$GQL_TYPES" | jq -r "[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test(\"$1\"))) | sort | join(\",\")"; }

case_ "N6.1 the five root fields are registered" "users, user, createUser, patchUser, archiveUser"
GOT=$(gql_user_fields "^(users|user|createUser|patchUser|archiveUser)$")
if [ "$GOT" = "archiveUser,createUser,patchUser,user,users" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "root fields = $GOT"; fi

case_ "N6.2 the seven COLLECTION fields are registered" "the ones §9 said were REST-only"
GOT=$(gql_user_fields "^(add|archive|patch)User(Group|Role|Claim)$")
WANT="addUserClaim,addUserGroup,addUserRole,archiveUserClaim,archiveUserGroup,archiveUserRole,patchUserClaim"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "collection fields = $GOT"; fi

case_ "N6.3 the two CREDENTIAL fields are registered" "changeUserPassword, resetUserPassword — the ones §10 said were not here at all"
GOT=$(gql_user_fields "Password$")
if [ "$GOT" = "changeUserPassword,resetUserPassword" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "credential fields = $GOT"; fi

case_ "N6.4 fourteen User operations in total" "matching REST's fourteen exactly — full parity, which is the corrected §9"
GOT=$(printf '%s' "$GQL_TYPES" | jq -r '[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test("[Uu]ser"))) | unique | length')
if [ "$GOT" = "14" ]; then pass_; else HTTP_BODY="$(printf '%s' "$GQL_TYPES" | jq -c '[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test("[Uu]ser"))) | unique')"; fail_ "user fields = $GOT"; fi

qa_finish
