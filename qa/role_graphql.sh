#!/usr/bin/env bash
# Lane: role_graphql — the GraphQL half of the framework contract for the Role aggregate.
#
# Family J of specs/qa/role-contract/plan.md §1, plus the H1/H3/H5/H9/H10 families repeated in
# this surface's own idiom. A route gated on REST is not thereby gated on GraphQL, and "the
# other surface forgot it" is a real regression that only a per-surface case catches.
#
# TWO IDIOMS, and the difference is BY DESIGN — the suite must not flatten it:
#   · a typed domain refusal answers HTTP 200 with the same notificationKey the REST envelope
#     carries, in errors[].extensions;
#   · a request the SCHEMA cannot express is cut by gqlparser before any resolver runs and
#     surfaces as a validation message with no notificationKey at all.
#
# AND THE THING THIS SURFACE PROVES BETTER THAN REST. The connection's `where:` argument comes
# from the Request DTO; the `Role` TYPE comes from the Response. So `tenantWorkspace` is BOTH —
# an argument and a field, because the ROOT join is served and addressable — while the CHILD
# join's `resource` is NEITHER, because it is hidden and load-only. One schema, one document,
# both halves of the doctrine visible side by side.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init role_graphql

# Every field the Role type actually declares. `resource` and `action` are deliberately absent
# from the entry selection — see J3.
ENTRY='id permissionID permission permissionArchivedAt'
NODE="id tenantID key name description createdAt updatedAt archivedAt tenantWorkspace tenantStatus tenantArchivedAt permissions { $ENTRY }"

WS_G=$(ws gqlrole)
TEN_G=$(new_tenant active "$WS_G") || exit 1
P_TENANT_READ=$(permission_id_of tenant read)
P_ROLE_READ=$(permission_id_of role read)
[ -n "$P_TENANT_READ" ] && [ -n "$P_ROLE_READ" ] || { echo "role_graphql.sh: a seeded catalog id could not be resolved" >&2; exit 1; }
D_OK="A role description long enough to satisfy the shared anti-junk floor this service applies."

# ═════════════════════════════════════════════════════════════════════════════════════════
# J1 — the seven mounted fields, and handler invariance with REST
# ═════════════════════════════════════════════════════════════════════════════════════════

K_G=$(role_key gql)
case_ "J1.1 createRole" "the record as stored, under data.createRole"
gql "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id tenantID key name description permissions { id permissionID } } }" \
    "$(jq -nc --arg k "$K_G" --arg d "$D_OK" --arg t "$TEN_G" --arg p "$P_TENANT_READ" \
       '{i:{key:$k, name:"QA GraphQL Role", description:$d, tenantID:$t, permissions:[{permissionID:$p}]}}')"
assert_gql_ok '.data.createRole.key' "$K_G"
ID_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createRole.id')

case_ "J1.1b HANDLER INVARIANCE: the mutation's effect is visible over REST immediately" "the same key, read through the other surface"
api GET "/roles/$ID_G"
assert_json_at 200 '.data.key' "$K_G"

case_ "J1.2 role(id:) — the singular by-id field" "the whole document, every declared field"
gql "query(\$id: ID!) { role(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.role.key' "$K_G"

case_ "J1.2b it equals the REST by-id document, field for field" "the ROOT join, the stamps and the derivation all agree across surfaces"
assert_json '[.data.role.tenantWorkspace, .data.role.tenantStatus, .data.role.permissions[0].permission] | join("|")' "$WS_G|active|tenant:read"

case_ "J1.3 roles(...) — the Relay connection" "edges/node/cursor + pageInfo + totalCount"
gql "query { roles(where: {key: {eq: \"$K_G\"}}) { totalCount edges { cursor node { $NODE } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } } }"
assert_gql_ok '.data.roles.edges[0].node.key' "$K_G"

case_ "J1.3b totalCount is carried by the connection" "1"
assert_json '.data.roles.totalCount' "1"

case_ "J1.4 patchRole" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchRoleInput!) { patchRole(id: \$id, input: \$i) { id key name } }" \
    "$(jq -nc --arg id "$ID_G" '{id:$id, i:{name:"QA GraphQL Role, relabelled"}}')"
assert_gql_ok '.data.patchRole.name' "QA GraphQL Role, relabelled"

case_ "J1.4b and the key did not move" "$K_G — patchable fields are name and description alone"
assert_json '.data.patchRole.key' "$K_G"

case_ "J1.5 addRolePermission" "the entry as stored, with the id the server minted"
gql "mutation(\$id: ID!, \$i: AddRolePermissionInput!) { addRolePermission(id: \$id, input: \$i) { roleId rolePermission { id permissionID } } }" \
    "$(jq -nc --arg id "$ID_G" --arg p "$P_ROLE_READ" '{id:$id, i:{permissionID:$p}}')"
assert_gql_ok '.data.addRolePermission.rolePermission.permissionID' "$P_ROLE_READ"
CHILD_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addRolePermission.rolePermission.id')

case_ "J1.5b the grant is visible over REST" "2 entries on the other surface"
api GET "/roles/$ID_G"
assert_json_at 200 '.data.permissions | length' "2"

# ── J1.6 — the canonical trap, on this surface too ────────────────────────────────────────
case_ "J1.6 archiveRolePermission" "the acknowledgement payload — a GraphQL field must resolve to SOMETHING, so 204-with-no-body becomes { success }"
gql "mutation(\$id: ID!, \$i: ArchiveRolePermissionInput!) { archiveRolePermission(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_G" --arg c "$CHILD_G" '{id:$id, i:{rolePermissionId:$c}}')"
assert_gql_ok '.data.archiveRolePermission.success' "true"

case_ "J1.6b THE TRAP, from this surface: the ROOT is still active over REST" "200 — a revoke wired to the root-archive handler would answer 404 here"
api GET "/roles/$ID_G"
assert_status 200

case_ "J1.6c and it kept its other grant" "1 entry — the revoked one is gone, the aggregate is not"
assert_json_at 200 '.data.permissions | length' "1"

case_ "J1.7 archiveRole" "the fixed bodyless payload { success, id }"
gql "mutation(\$id: ID!) { archiveRole(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.archiveRole.success' "true"

case_ "J1.7b the archived role is hidden on this surface too" "data.role is null, with the canonical not-found in errors[]"
gql "query(\$id: ID!) { role(id: \$id) { id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql RecordNotFoundNotification

case_ "J1.7c includeArchived reveals it, with the stamp" "archivedAt is no longer null"
gql "query(\$id: ID!) { role(id: \$id, includeArchived: true) { id archivedAt } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '(.data.role.archivedAt != null)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# J2 — the ROOT join on this surface: served, filterable AND sortable
# ═════════════════════════════════════════════════════════════════════════════════════════

K_J2=$(role_key j2)
gql "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id } }" \
    "$(jq -nc --arg k "$K_J2" --arg d "$D_OK" --arg t "$TEN_G" --arg p "$P_TENANT_READ" \
       '{i:{key:$k, name:"QA Join Role", description:$d, tenantID:$t, permissions:[{permissionID:$p}]}}')" >/dev/null
ID_J2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createRole.id')

case_ "J2.1 where: { tenantWorkspace: { eq } } — the ROOT join as an ARGUMENT" "the roles of a workspace, over a column that lives in another table"
gql "query { roles(where: {tenantWorkspace: {eq: \"$WS_G\"}}) { totalCount edges { node { key tenantWorkspace } } } }"
assert_gql_ok '.data.roles.edges[0].node.tenantWorkspace' "$WS_G"

case_ "J2.2 and the same filter on REST answers the same total" "identical totalCount — one criteria vocabulary, two surfaces"
GQL_TOTAL=$(printf '%s' "$HTTP_BODY" | jq -r '.data.roles.totalCount')
api GET "/roles?tenantWorkspace.eq=$WS_G&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "$GQL_TOTAL"

case_ "J2.3 where: { tenantStatus: { eq } } — the enum over the join" "the same rows, narrowed by the owner's commercial state"
gql "query { roles(where: {tenantWorkspace: {eq: \"$WS_G\"}, tenantStatus: {eq: \"active\"}}) { totalCount } }"
assert_gql_ok '.data.roles.totalCount' "$GQL_TOTAL"

# ── the ordering enum, INTROSPECTED rather than guessed ───────────────────────────────────
#
# The deleted 2026-09-03 round shipped `orderBy: "key"` — the REST string — and was wrong: this
# surface takes a typed input over a reflected enum. The member names are a mechanical
# derivation the framework performs, so they are READ from the schema and then asserted to be
# in PARITY with the REST sort vocabulary. Reading a name is addressing; the expectation is the
# parity, and that comes from the yaml.
gql 'query { __type(name: "RoleOrderField") { enumValues { name } } }'
ENUM_MEMBERS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data.__type.enumValues[]?.name] | sort | join(" ")')

case_ "J2.4 the RoleOrderField enum exists and is not empty" "a reflected enum — sorting on this surface is a typed input, not the REST string"
if [ -n "$ENUM_MEMBERS" ]; then pass_; else fail_ "no enum members"; fi

case_ "J2.5 PARITY: the enum has exactly SEVEN members" "7 — the same vocabulary read.byParams.sort declares, so the capability cannot come to exist on one surface alone"
N_ENUM=$(printf '%s' "$ENUM_MEMBERS" | wc -w | tr -d ' ')
if [ "$N_ENUM" = "7" ]; then pass_; else fail_ "$N_ENUM members: $ENUM_MEMBERS"; fi

case_ "J2.6 and every REST sort key has a member" "key, name, tenantID, tenantWorkspace, tenantStatus, createdAt, updatedAt all present, however the reflection spells them"
MISSING=""
for fld in KEY NAME TENANTID TENANTWORKSPACE TENANTSTATUS CREATEDAT UPDATEDAT; do
  printf '%s' "$ENUM_MEMBERS" | tr -d '_' | tr 'a-z' 'A-Z' | grep -qw "$fld" || MISSING="$MISSING $fld"
done
if [ -z "$MISSING" ]; then pass_; else fail_ "missing:$MISSING (enum: $ENUM_MEMBERS)"; fi

GQL_WS_MEMBER=$(printf '%s' "$ENUM_MEMBERS" | tr ' ' '\n' | awk '{u=$0; gsub(/_/,"",u); if (toupper(u)=="TENANTWORKSPACE") print $0}' | head -1)
case_ "J2.7 orderBy over the ROOT JOIN answers the same order as its REST twin" "the same first key, both directions of one seam"
if [ -n "$GQL_WS_MEMBER" ]; then
  gql "query { roles(where: {tenantWorkspace: {eq: \"$WS_G\"}}, orderBy: [{field: $GQL_WS_MEMBER, direction: ASC}], first: 1) { edges { node { key } } } }"
  G_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data.roles.edges[0].node.key // ""')
  api GET "/roles?tenantWorkspace.eq=$WS_G&orderBy=tenantWorkspace&first=1"
  R_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].key // ""')
  if [ -n "$G_FIRST" ] && [ "$G_FIRST" = "$R_FIRST" ]; then pass_; else fail_ "graphql '$G_FIRST' vs rest '$R_FIRST'"; fi
else
  skip_ "the reflected enum carries no tenantWorkspace member — J2.5/J2.6 already report that as the failure, and repeating it here would count one defect twice"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# J3 — the CHILD join, made structural: hidden on BOTH sides of the schema
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "J3.1 selecting resource on an entry is a SCHEMA error" "'Cannot query field' from gqlparser, with no notificationKey — the entry type has no such field, so the schema must not carry what the REST body hides"
gql "query(\$id: ID!) { role(id: \$id) { permissions { resource } } }" "$(jq -nc --arg id "$ID_J2" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "J3.2 selecting action is a schema error for the same reason" "'Cannot query field'"
gql "query(\$id: ID!) { role(id: \$id) { permissions { action } } }" "$(jq -nc --arg id "$ID_J2" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "J3.3 and the derivation IS selectable" "tenant:read — the two hidden halves feed it and the caller reads one token"
gql "query(\$id: ID!) { role(id: \$id) { permissions { $ENTRY } } }" "$(jq -nc --arg id "$ID_J2" '{id:$id}')"
assert_gql_ok '.data.role.permissions[0].permission' "tenant:read"

case_ "J3.4 filtering BY a child field is not expressible here either" "an unknown argument — the 1:N boundary is the criteria's, not the surface's"
gql "query { roles(where: {permissions: {eq: \"x\"}}) { totalCount } }"
assert_gql_validation "permissions"

# ═════════════════════════════════════════════════════════════════════════════════════════
# J4 — refusals in this surface's idiom
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "J4.1 a duplicate key through createRole" "RoleKeyAlreadyExistsNotification in errors[].extensions, at HTTP 200"
gql "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id } }" \
    "$(jq -nc --arg k "$K_J2" --arg d "$D_OK" --arg t "$TEN_G" '{i:{key:$k, name:"QA Duplicate", description:$d, tenantID:$t, permissions:[]}}')"
assert_gql RoleKeyAlreadyExistsNotification

case_ "J4.1b the extensions carry the Conflict semantic" "semantic 'Conflict' — the same envelope shape the REST 409 reports"
assert_json '[.errors[].extensions.semantic] | unique | join(",")' "Conflict"

case_ "J4.2 an invalid key" "InvalidRoleKeyNotification"
gql "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id } }" \
    "$(jq -nc --arg d "$D_OK" --arg t "$TEN_G" '{i:{key:"Billing", name:"QA Invalid", description:$d, tenantID:$t, permissions:[]}}')"
assert_gql InvalidRoleKeyNotification

case_ "J4.3 the guard barrier on this surface" "InvalidIDUUIDNotification — the same rule, the same pass, a different transport"
gql "mutation(\$i: CreateRoleInput!) { createRole(input: \$i) { id } }" \
    "$(jq -nc --arg k "$(role_key gj)" --arg d "$D_OK" '{i:{key:$k, name:"QA Guard", description:$d, tenantID:"tatu", permissions:[]}}')"
assert_gql InvalidIDUUIDNotification

case_ "J4.4 a duplicate grant through addRolePermission" "RoleAlreadyGrantsPermissionNotification"
gql "mutation(\$id: ID!, \$i: AddRolePermissionInput!) { addRolePermission(id: \$id, input: \$i) { roleId } }" \
    "$(jq -nc --arg id "$ID_J2" --arg p "$P_TENANT_READ" '{id:$id, i:{permissionID:$p}}')"
assert_gql RoleAlreadyGrantsPermissionNotification

case_ "J4.5 role(id: <unused uuid>)" "RecordNotFoundNotification"
gql 'query { role(id: "00000000-0000-4000-8000-00000000dead") { id } }'
assert_gql RecordNotFoundNotification

case_ "J4.6 role(id: 'not-a-uuid') — a READ address" "UnknownIDAddressNotification — the by-id address contract is split by VERB, not by surface"
gql 'query { role(id: "not-a-uuid") { id } }'
assert_gql UnknownIDAddressNotification

case_ "J4.7 archiveRole(id: 'not-a-uuid') — a WRITE intention" "MalformedIDNotification — the other arm of the same split, on the same surface"
gql 'mutation { archiveRole(id: "not-a-uuid") { success } }'
assert_gql MalformedIDNotification

case_ "J4.8 addRolePermission onto an id that addresses nothing" "RecordNotFoundNotification — the owner is loaded before the entry is considered"
gql "mutation(\$i: AddRolePermissionInput!) { addRolePermission(id: \"00000000-0000-4000-8000-00000000dead\", input: \$i) { roleId } }" \
    "$(jq -nc --arg p "$P_TENANT_READ" '{i:{permissionID:$p}}')"
assert_gql RecordNotFoundNotification

case_ "J4.9 archiveRolePermission naming an entry the collection does not hold" "RecordNotFoundNotification — the CHILD address, which is not the by-id contract"
gql "mutation(\$i: ArchiveRolePermissionInput!) { archiveRolePermission(id: \"$ID_J2\", input: \$i) { success } }" \
    '{"i":{"rolePermissionId":"00000000-0000-4000-8000-00000000beef"}}'
assert_gql RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# J5 — what the SCHEMA cannot express, which is a different idiom on purpose
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "J5.1 an undeclared argument is cut out of the schema" "a gqlparser validation error, never the REST 400 envelope — search: is not a member of this connection's arguments"
gql 'query { roles(search: "x") { totalCount } }'
assert_gql_validation "search"

case_ "J5.2 there is NO unarchiveRole field" "'Cannot query field' on the mutation type — the GraphQL twin of the REST 404 arm"
gql 'mutation { unarchiveRole(id: "00000000-0000-4000-8000-00000000dead") { success } }'
assert_gql_validation "unarchiveRole"

case_ "J5.3 fields and onlyTotal have no argument here" "selection IS the projection, so asking for a REST control is an unknown argument"
gql 'query { roles(fields: "key") { totalCount } }'
assert_gql_validation "fields"

case_ "J5.4 selecting totalCount ALONE is the only-total mode" "a count with no edges, and no error — the surface's own idiom for what REST spells ?onlyTotal=true"
gql "query { roles(where: {tenantWorkspace: {eq: \"$WS_G\"}}) { totalCount } }"
assert_gql_ok '(.data.roles.totalCount > 0)' "true"

case_ "J5.5 the page ceiling still applies" "LimitExceededNotification — a typed refusal, because first: IS a declared argument"
gql 'query { roles(first: 101) { totalCount edges { node { id } } } }'
assert_gql LimitExceededNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# J6 — __typename, the v0.72.1 regression guard
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "J6.1 __typename beside a normal selection answers identically" "the same values, with __typename resolved — every mainstream client appends it to every selection set for cache normalisation"
gql "query(\$id: ID!) { role(id: \$id) { __typename key tenantWorkspace permissions { __typename permission } } }" "$(jq -nc --arg id "$ID_J2" '{id:$id}')"
assert_gql_ok '[.data.role.__typename, .data.role.key, .data.role.permissions[0].permission] | join("|")' "Role|$K_J2|tenant:read"

case_ "J6.2 it does not widen the projection" "the hidden halves stay hidden even when __typename rides along"
assert_json '[.data.role.permissions[0] | (has("resource") or has("action"))] | any' "false"

qa_finish
