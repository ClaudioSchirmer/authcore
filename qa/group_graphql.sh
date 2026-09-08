#!/usr/bin/env bash
# Lane: group_graphql — the GraphQL half of the framework contract for the Group aggregate.
#
# Family L of specs/qa/group-contract/plan.md §1, plus the K1/K3/K5/K9/K10 families repeated in
# this surface's own idiom. A route gated on REST is not thereby gated on GraphQL, and "the
# other surface forgot it" is a real regression that only a per-surface case catches.
#
# TWO IDIOMS, and the difference is BY DESIGN — the suite must not flatten it:
#   · a typed domain refusal answers HTTP 200 with the same notificationKey the REST envelope
#     carries, in errors[].extensions;
#   · a request the SCHEMA cannot express is cut by gqlparser before any resolver runs and
#     surfaces as a validation message with no notificationKey at all.
#
# AND THE INVERSION THIS LANE EXISTS TO PIN. On Role, selecting `resource` on an entry was a
# schema error, because the child join was hidden. Here the child join is SERVED, so selecting
# `roleKey` MUST WORK — and filtering by it must still be an unknown argument. Both halves are
# below (L3), because a suite copied from role_graphql.sh gets the first one exactly backwards.
#
# Group also mounts SEVEN GraphQL fields, the two collection verbs included. spec.md §9 still
# says "root verbs only"; the yaml records that the collection verbs joined when the language
# grew a seat for them, and group_routes.go registers all seven. Seven is what L1 asserts.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init group_graphql

# Every field the Group type declares. Unlike Role's, the entry selection is COMPLETE: nothing
# on this collection is hidden.
ENTRY='id roleID roleKey roleName roleArchivedAt'
NODE="id tenantID key name description createdAt updatedAt archivedAt tenantWorkspace tenantStatus tenantArchivedAt roles { $ENTRY }"

WS_G=$(ws gqlgroup)
TEN_G=$(new_tenant active "$WS_G") || exit 1
P_TENANT_READ=$(permission_id_of tenant read)
[ -n "$P_TENANT_READ" ] || { echo "group_graphql.sh: a seeded catalog id could not be resolved" >&2; exit 1; }
D_OK="A group description long enough to satisfy the shared anti-junk floor this service applies."

K_ROLE_G=$(role_key gqlg); R_G=$(new_role "$K_ROLE_G" "$TEN_G" "$P_TENANT_READ") || exit 1
K_ROLE_G2=$(role_key gqlg2); R_G2=$(new_role "$K_ROLE_G2" "$TEN_G" "$P_TENANT_READ") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# L1 — the seven mounted fields, and handler invariance with REST
# ═════════════════════════════════════════════════════════════════════════════════════════

K_G=$(group_key gql)
case_ "L1.1 createGroup" "the record as stored, under data.createGroup"
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id tenantID key name description roles { id roleID } } }" \
    "$(jq -nc --arg k "$K_G" --arg d "$D_OK" --arg t "$TEN_G" --arg r "$R_G" \
       '{i:{key:$k, name:"QA GraphQL Group", description:$d, tenantID:$t, roles:[{roleID:$r}]}}')"
assert_gql_ok '.data.createGroup.key' "$K_G"
ID_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createGroup.id')

case_ "L1.1b HANDLER INVARIANCE: the mutation's effect is visible over REST immediately" "the same key, read through the other surface"
api GET "/groups/$ID_G"
assert_json_at 200 '.data.key' "$K_G"

case_ "L1.1c the mutation payload carries no traversal field" "'Cannot query field roleKey' on the write payload's entry type — the write side never speaks the traversal, on either surface"
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id roles { roleKey } } }" \
    "$(jq -nc --arg k "$(group_key gqlx)" --arg d "$D_OK" --arg t "$TEN_G" '{i:{key:$k, name:"QA Probe", description:$d, tenantID:$t, roles:[]}}')"
assert_gql_validation "Cannot query field"

case_ "L1.2 group(id:) — the singular by-id field" "the whole document, every declared field"
gql "query(\$id: ID!) { group(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.group.key' "$K_G"

case_ "L1.2b it equals the REST by-id document, field for field" "the ROOT join, the child join and the stamps all agree across surfaces"
assert_json '[.data.group.tenantWorkspace, .data.group.tenantStatus, .data.group.roles[0].roleKey] | join("|")' "$WS_G|active|$K_ROLE_G"

case_ "L1.3 groups(...) — the Relay connection" "edges/node/cursor + pageInfo + totalCount"
gql "query { groups(where: {key: {eq: \"$K_G\"}}) { totalCount edges { cursor node { $NODE } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } } }"
assert_gql_ok '.data.groups.edges[0].node.key' "$K_G"

case_ "L1.3b totalCount is carried by the connection" "1"
assert_json '.data.groups.totalCount' "1"

case_ "L1.4 patchGroup" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchGroupInput!) { patchGroup(id: \$id, input: \$i) { id key name } }" \
    "$(jq -nc --arg id "$ID_G" '{id:$id, i:{name:"QA GraphQL Group, relabelled"}}')"
assert_gql_ok '.data.patchGroup.name' "QA GraphQL Group, relabelled"

case_ "L1.4b and the key did not move" "$K_G — patchExcludes: [Key], so the field is not a member of the input at all"
assert_json '.data.patchGroup.key' "$K_G"

case_ "L1.4c the input type DECLARES no key field" "name and description alone — patchExcludes: [Key] cuts the field out of the schema itself, so structural immutability is identical on both surfaces"
gql 'query { __type(name: "PatchGroupInput") { inputFields { name } } }'
assert_gql_ok '[.data.__type.inputFields[].name] | sort | join(",")' "description,name"

case_ "L1.4d naming key IN THE DOCUMENT is a schema error" "a gqlparser validation error — the literal is validated against the input type, which is where the structural cut becomes visible"
gql "mutation { patchGroup(id: \"$ID_G\", input: {key: \"other-key\"}) { id } }"
assert_gql_validation "key"

case_ "L1.4e naming key inside a VARIABLE is DROPPED, not refused" "200 and the key UNCHANGED — coercion discards an unknown member of an input object instead of erroring, so the EFFECT is the only honest assertion on this path. Recorded rather than smoothed: a case asserting a validation error here would pass for the wrong reason on any schema"
gql "mutation(\$id: ID!, \$i: PatchGroupInput!) { patchGroup(id: \$id, input: \$i) { id key } }" \
    "$(jq -nc --arg id "$ID_G" '{id:$id, i:{key:"other-key", name:"QA Variable Probe"}}')"
assert_gql_ok '.data.patchGroup.key' "$K_G"

case_ "L1.5 addGroupRole" "the entry as stored, with the id the server minted"
gql "mutation(\$id: ID!, \$i: AddGroupRoleInput!) { addGroupRole(id: \$id, input: \$i) { groupId groupRole { id roleID } } }" \
    "$(jq -nc --arg id "$ID_G" --arg r "$R_G2" '{id:$id, i:{roleID:$r}}')"
assert_gql_ok '.data.addGroupRole.groupRole.roleID' "$R_G2"
CHILD_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addGroupRole.groupRole.id')

case_ "L1.5b the attach is visible over REST" "2 entries on the other surface"
api GET "/groups/$ID_G"
assert_json_at 200 '.data.roles | length' "2"

# ── L1.6 — the canonical trap, on this surface too ────────────────────────────────────────
case_ "L1.6 archiveGroupRole" "the acknowledgement payload — a GraphQL field must resolve to SOMETHING, so 204-with-no-body becomes { success }"
gql "mutation(\$id: ID!, \$i: ArchiveGroupRoleInput!) { archiveGroupRole(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_G" --arg c "$CHILD_G" '{id:$id, i:{groupRoleId:$c}}')"
assert_gql_ok '.data.archiveGroupRole.success' "true"

case_ "L1.6b THE TRAP, from this surface: the ROOT is still active over REST" "200 — a detach wired to the root-archive handler would answer 404 here, having de-authorized a whole team"
api GET "/groups/$ID_G"
assert_status 200

case_ "L1.6c and it kept its other entry" "1 entry — the detached one is gone, the aggregate is not"
assert_json_at 200 '.data.roles | length' "1"

case_ "L1.7 archiveGroup" "the fixed bodyless payload { success, id }"
gql "mutation(\$id: ID!) { archiveGroup(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.archiveGroup.success' "true"

case_ "L1.7b the archived group is hidden on this surface too" "data.group is null, with the canonical not-found in errors[]"
gql "query(\$id: ID!) { group(id: \$id) { id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql RecordNotFoundNotification

case_ "L1.7c includeArchived reveals it, with the stamp" "archivedAt is no longer null"
gql "query(\$id: ID!) { group(id: \$id, includeArchived: true) { id archivedAt } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '(.data.group.archivedAt != null)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# L2 — the ROOT join on this surface: served, filterable AND sortable
# ═════════════════════════════════════════════════════════════════════════════════════════

K_L2=$(group_key l2)
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" \
    "$(jq -nc --arg k "$K_L2" --arg d "$D_OK" --arg t "$TEN_G" --arg r "$R_G" \
       '{i:{key:$k, name:"QA Join Group", description:$d, tenantID:$t, roles:[{roleID:$r}]}}')" >/dev/null
ID_L2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createGroup.id')

case_ "L2.1 where: { tenantWorkspace: { eq } } — the ROOT join as an ARGUMENT" "the groups of a workspace, over a column that lives in another table"
gql "query { groups(where: {tenantWorkspace: {eq: \"$WS_G\"}}) { totalCount edges { node { key tenantWorkspace } } } }"
assert_gql_ok '.data.groups.edges[0].node.tenantWorkspace' "$WS_G"

case_ "L2.2 and the same filter on REST answers the same total" "identical totalCount — one criteria vocabulary, two surfaces"
GQL_TOTAL=$(printf '%s' "$HTTP_BODY" | jq -r '.data.groups.totalCount')
api GET "/groups?tenantWorkspace.eq=$WS_G&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "$GQL_TOTAL"

case_ "L2.3 where: { tenantStatus: { eq } } — the enum over the join" "the same rows, narrowed by the owner's commercial state"
gql "query { groups(where: {tenantWorkspace: {eq: \"$WS_G\"}, tenantStatus: {eq: \"active\"}}) { totalCount } }"
assert_gql_ok '.data.groups.totalCount' "$GQL_TOTAL"

# ── the ordering enum, INTROSPECTED rather than guessed ───────────────────────────────────
gql 'query { __type(name: "GroupOrderField") { enumValues { name } } }'
ENUM_MEMBERS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data.__type.enumValues[]?.name] | sort | join(" ")')

case_ "L2.4 the GroupOrderField enum exists and is not empty" "a reflected enum — sorting on this surface is a typed input, not the REST string"
if [ -n "$ENUM_MEMBERS" ]; then pass_; else fail_ "no enum members"; fi

case_ "L2.5 PARITY: the enum has exactly SEVEN members" "7 — the same vocabulary read.byParams.sort declares, so the capability cannot come to exist on one surface alone"
N_ENUM=$(printf '%s' "$ENUM_MEMBERS" | wc -w | tr -d ' ')
if [ "$N_ENUM" = "7" ]; then pass_; else fail_ "$N_ENUM members: $ENUM_MEMBERS"; fi

case_ "L2.6 and every REST sort key has a member" "key, name, tenantID, tenantWorkspace, tenantStatus, createdAt, updatedAt all present, however the reflection spells them"
MISSING=""
for fld in KEY NAME TENANTID TENANTWORKSPACE TENANTSTATUS CREATEDAT UPDATEDAT; do
  printf '%s' "$ENUM_MEMBERS" | tr -d '_' | tr 'a-z' 'A-Z' | grep -qw "$fld" || MISSING="$MISSING $fld"
done
if [ -z "$MISSING" ]; then pass_; else fail_ "missing:$MISSING (enum: $ENUM_MEMBERS)"; fi

case_ "L2.7 description is orderable on NEITHER surface" "no DESCRIPTION member — the asymmetry K9.12 asserts over REST, held here too"
if printf '%s' "$ENUM_MEMBERS" | tr -d '_' | tr 'a-z' 'A-Z' | grep -qw DESCRIPTION; then fail_ "enum: $ENUM_MEMBERS"; else pass_; fi

GQL_WS_MEMBER=$(printf '%s' "$ENUM_MEMBERS" | tr ' ' '\n' | awk '{u=$0; gsub(/_/,"",u); if (toupper(u)=="TENANTWORKSPACE") print $0}' | head -1)
case_ "L2.8 orderBy over the ROOT JOIN answers the same order as its REST twin" "the same first key, both sides of one seam"
if [ -n "$GQL_WS_MEMBER" ]; then
  gql "query { groups(where: {tenantWorkspace: {eq: \"$WS_G\"}}, orderBy: [{field: $GQL_WS_MEMBER, direction: ASC}], first: 1) { edges { node { key } } } }"
  G_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data.groups.edges[0].node.key // ""')
  api GET "/groups?tenantWorkspace.eq=$WS_G&orderBy=tenantWorkspace&first=1"
  R_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].key // ""')
  if [ -n "$G_FIRST" ] && [ "$G_FIRST" = "$R_FIRST" ]; then pass_; else fail_ "graphql '$G_FIRST' vs rest '$R_FIRST'"; fi
else
  skip_ "the reflected enum carries no tenantWorkspace member — L2.5/L2.6 already report that as the failure, and repeating it here would count one defect twice"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# L3 — THE INVERSION. On Role these three cases asserted 'Cannot query field'. Here the child
#      join is SERVED, so the schema MUST carry what the REST body carries — and the criteria
#      must still refuse it. A suite copied from role_graphql.sh gets exactly this backwards.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "L3.1 selecting roleKey on an entry RESOLVES" "the counterpart's handle — the exact inverse of role_graphql.sh J3.1, and the case that catches a schema which dropped a served field"
gql "query(\$id: ID!) { group(id: \$id) { roles { roleKey } } }" "$(jq -nc --arg id "$ID_L2" '{id:$id}')"
assert_gql_ok '.data.group.roles[0].roleKey' "$K_ROLE_G"

case_ "L3.2 selecting roleName RESOLVES" "the counterpart's display name"
gql "query(\$id: ID!) { group(id: \$id) { roles { roleName } } }" "$(jq -nc --arg id "$ID_L2" '{id:$id}')"
assert_gql_ok '.data.group.roles[0].roleName' "QA Fixture Role"

case_ "L3.3 selecting roleArchivedAt RESOLVES" "null while the counterpart lives, and it is IN the schema — K7.2 is where it fills"
gql "query(\$id: ID!) { group(id: \$id) { roles { roleArchivedAt } } }" "$(jq -nc --arg id "$ID_L2" '{id:$id}')"
assert_gql_ok '(.data.group.roles[0] | has("roleArchivedAt"))' "true"

case_ "L3.4 the whole entry, five fields, one selection" "id roleID roleKey roleName roleArchivedAt — the schema carries exactly what the REST body carries"
gql "query(\$id: ID!) { group(id: \$id) { roles { $ENTRY } } }" "$(jq -nc --arg id "$ID_L2" '{id:$id}')"
assert_gql_ok '.data.group.roles[0] | keys | sort | join(",")' "id,roleArchivedAt,roleID,roleKey,roleName"

case_ "L3.5 and filtering BY a child field is STILL not expressible" "an unknown argument — served does not mean addressable, and the 1:N boundary is the criteria's rather than the surface's"
gql "query { groups(where: {roles: {eq: \"x\"}}) { totalCount } }"
assert_gql_validation "roles"

case_ "L3.6 nor is ordering by one" "the reflected enum has no member for it — 'which groups confer role X?' is unanswerable on this surface too"
gql "query { groups(orderBy: [{field: ROLE_KEY, direction: ASC}]) { totalCount } }"
assert_gql_validation "ROLE_KEY"

# ═════════════════════════════════════════════════════════════════════════════════════════
# L4 — refusals in this surface's idiom
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "L4.1 a duplicate key through createGroup" "GroupKeyAlreadyExistsNotification in errors[].extensions, at HTTP 200"
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" \
    "$(jq -nc --arg k "$K_L2" --arg d "$D_OK" --arg t "$TEN_G" '{i:{key:$k, name:"QA Duplicate", description:$d, tenantID:$t, roles:[]}}')"
assert_gql GroupKeyAlreadyExistsNotification

case_ "L4.1b the extensions carry the Conflict semantic" "semantic 'Conflict' — the same envelope shape the REST 409 reports"
assert_json '[.errors[].extensions.semantic] | unique | join(",")' "Conflict"

case_ "L4.2 an invalid key" "InvalidGroupKeyNotification"
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" \
    "$(jq -nc --arg d "$D_OK" --arg t "$TEN_G" '{i:{key:"Engineering", name:"QA Invalid", description:$d, tenantID:$t, roles:[]}}')"
assert_gql InvalidGroupKeyNotification

case_ "L4.3 the guard barrier on this surface" "InvalidIDUUIDNotification — the same rule, the same pass, a different transport"
gql "mutation(\$i: CreateGroupInput!) { createGroup(input: \$i) { id } }" \
    "$(jq -nc --arg k "$(group_key gl)" --arg d "$D_OK" '{i:{key:$k, name:"QA Guard", description:$d, tenantID:"tatu", roles:[]}}')"
assert_gql InvalidIDUUIDNotification

case_ "L4.4 a duplicate attach through addGroupRole" "GroupAlreadyGrantsRoleNotification"
gql "mutation(\$id: ID!, \$i: AddGroupRoleInput!) { addGroupRole(id: \$id, input: \$i) { groupId } }" \
    "$(jq -nc --arg id "$ID_L2" --arg r "$R_G" '{id:$id, i:{roleID:$r}}')"
assert_gql GroupAlreadyGrantsRoleNotification

case_ "L4.5 group(id: <unused uuid>)" "RecordNotFoundNotification"
gql 'query { group(id: "00000000-0000-4000-8000-00000000dead") { id } }'
assert_gql RecordNotFoundNotification

case_ "L4.6 group(id: 'not-a-uuid') — a READ address" "UnknownIDAddressNotification — the by-id address contract is split by VERB, not by surface"
gql 'query { group(id: "not-a-uuid") { id } }'
assert_gql UnknownIDAddressNotification

case_ "L4.7 archiveGroup(id: 'not-a-uuid') — a WRITE intention" "MalformedIDNotification — the other arm of the same split, on the same surface"
gql 'mutation { archiveGroup(id: "not-a-uuid") { success } }'
assert_gql MalformedIDNotification

case_ "L4.8 addGroupRole onto an id that addresses nothing" "RecordNotFoundNotification — the owner is loaded before the entry is considered"
gql "mutation(\$i: AddGroupRoleInput!) { addGroupRole(id: \"00000000-0000-4000-8000-00000000dead\", input: \$i) { groupId } }" \
    "$(jq -nc --arg r "$R_G" '{i:{roleID:$r}}')"
assert_gql RecordNotFoundNotification

case_ "L4.9 archiveGroupRole naming an entry the collection does not hold" "RecordNotFoundNotification — the CHILD address, which is not the by-id contract"
gql "mutation(\$i: ArchiveGroupRoleInput!) { archiveGroupRole(id: \"$ID_L2\", input: \$i) { success } }" \
    '{"i":{"groupRoleId":"00000000-0000-4000-8000-00000000beef"}}'
assert_gql RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# L5 — what the SCHEMA cannot express, which is a different idiom on purpose
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "L5.1 an undeclared argument is cut out of the schema" "a gqlparser validation error, never the REST 400 envelope"
gql 'query { groups(search: "x") { totalCount } }'
assert_gql_validation "search"

case_ "L5.2 there is NO unarchiveGroup field" "'Cannot query field' on the mutation type — the GraphQL twin of the REST 404 arm"
gql 'mutation { unarchiveGroup(id: "00000000-0000-4000-8000-00000000dead") { success } }'
assert_gql_validation "unarchiveGroup"

case_ "L5.3 fields and onlyTotal have no argument here" "selection IS the projection, so asking for a REST control is an unknown argument"
gql 'query { groups(fields: "key") { totalCount } }'
assert_gql_validation "fields"

case_ "L5.4 selecting totalCount ALONE is the only-total mode" "a count with no edges, and no error — this surface's idiom for what REST spells ?onlyTotal=true"
gql "query { groups(where: {tenantWorkspace: {eq: \"$WS_G\"}}) { totalCount } }"
assert_gql_ok '(.data.groups.totalCount > 0)' "true"

case_ "L5.5 the page ceiling still applies" "LimitExceededNotification — a typed refusal, because first: IS a declared argument"
gql 'query { groups(first: 101) { totalCount edges { node { id } } } }'
assert_gql LimitExceededNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# L6 — __typename, the standing regression guard (pin >= v0.72.1)
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "L6.1 __typename beside a normal selection answers identically" "every mainstream client appends it to every selection set for cache normalization; below v0.72.1 its presence widened the projection"
gql "query(\$id: ID!) { group(id: \$id) { __typename key tenantWorkspace roles { __typename roleKey } } }" "$(jq -nc --arg id "$ID_L2" '{id:$id}')"
assert_gql_ok '[.data.group.key, .data.group.roles[0].roleKey] | join("|")' "$K_L2|$K_ROLE_G"

case_ "L6.2 and it names the declared types" "Group on the node, and the entry type on the entry"
assert_json '.data.group.__typename' "Group"

qa_finish
