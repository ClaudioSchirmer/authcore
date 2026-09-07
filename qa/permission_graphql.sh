#!/usr/bin/env bash
# Lane: permission_graphql — the GraphQL half of the framework contract for the Permission
# catalog.
#
# Family E10 of specs/qa/permission-contract/plan.md §1, plus the E1/E2/E3/E4/E5/E7/E8 families
# repeated in this surface's own idiom. A route gated on REST is not thereby gated on GraphQL,
# and "the other surface forgot it" is a real regression that only a per-surface case catches.
#
# TWO IDIOMS, and the difference is BY DESIGN — the suite must not flatten it:
#   · a typed domain refusal answers HTTP 200 with the same notificationKey the REST envelope
#     carries, in errors[].extensions;
#   · a request the SCHEMA cannot express is cut by gqlparser before any resolver runs and
#     surfaces as a validation message with no notificationKey at all.
#
# AND THE THING THIS SURFACE PROVES BETTER THAN REST. The connection's `where:` argument comes
# from the Request DTO; the `Permission` TYPE comes from the Response. So `resource` exists as
# an argument and does NOT exist as a field, in one schema, in one document — which is "three
# columns go in, two values come out" made structural rather than merely observed.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init permission_graphql

# Every field the Permission type actually declares. `resource` and `action` are deliberately
# absent — see G2.
NODE='id description permission createdAt updatedAt archivedAt'

# ═════════════════════════════════════════════════════════════════════════════════════════
# G1 — the five mounted fields
# ═════════════════════════════════════════════════════════════════════════════════════════

R_G=$(pair_resource gql)
case_ "G1.1 createPermission" "the record as stored, under data.createPermission"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id description permission } }" \
    "$(jq -nc --arg r "$R_G" '{i:{resource:$r, action:"read", description:"Read the GraphQL surface fixture resource of the permission contract lane."}}')"
assert_gql_ok '.data.createPermission.permission' "$R_G:read"
ID_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createPermission.id')

case_ "G1.2 permission(id:) — the singular by-id field" "the same record"
gql "query(\$id: ID!) { permission(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.permission.permission' "$R_G:read"

case_ "G1.3 permissions(...) — the Relay connection" "edges/node/cursor + pageInfo + totalCount"
gql "query { permissions(where: {resource: {eq: \"$R_G\"}}) { totalCount edges { cursor node { $NODE } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } } }"
assert_gql_ok '.data.permissions.edges[0].node.permission' "$R_G:read"

case_ "G1.4 totalCount is carried by the connection" "1"
assert_json '.data.permissions.totalCount' "1"

case_ "G1.5 patchPermission" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchPermissionInput!) { patchPermission(id: \$id, input: \$i) { id description permission } }" \
    "$(jq -nc --arg id "$ID_G" '{id:$id, i:{description:"Read the GraphQL surface fixture, with wording revised through the mutation."}}')"
assert_gql_ok '.data.patchPermission.description' "Read the GraphQL surface fixture, with wording revised through the mutation."

case_ "G1.6 the pair survives the patch on this surface too" "$R_G:read"
assert_json '.data.patchPermission.permission' "$R_G:read"

case_ "G1.7 archivePermission" "the fixed bodyless payload { success, id }"
gql "mutation(\$id: ID!) { archivePermission(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.archivePermission.success' "true"

case_ "G1.8 the archived record is hidden here too" "data.permission is null, with the canonical not-found in errors[]"
gql "query(\$id: ID!) { permission(id: \$id) { id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql RecordNotFoundNotification

case_ "G1.9 includeArchived reveals it, with the stamp" "archivedAt is no longer null"
gql "query(\$id: ID!) { permission(id: \$id, includeArchived: true) { id archivedAt } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '(.data.permission.archivedAt != null)' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# G2 — the doctrine, made structural
#
# The two halves of "three columns go in, two values come out" are visible in the SAME schema:
# `resource` is an argument the connection accepts and a field the type does not have.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_D=$(pair_resource doc)
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg r "$R_D" '{i:{resource:$r, action:"insert", description:"The doctrine fixture: filterable by a half that no selection can name."}}')" >/dev/null
ID_D=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createPermission.id')

case_ "G2.1 filtering BY resource works — it is a Request-DTO argument" "the fixture row comes back"
gql "query { permissions(where: {resource: {eq: \"$R_D\"}}) { edges { node { permission } } } }"
assert_gql_ok '.data.permissions.edges[0].node.permission' "$R_D:insert"

case_ "G2.2 SELECTING resource is a schema error — it is not a Response field" "'Cannot query field' from gqlparser, with no notificationKey: the type has no such field"
gql "query(\$id: ID!) { permission(id: \$id) { resource } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "G2.3 selecting action is a schema error for the same reason" "'Cannot query field'"
gql "query(\$id: ID!) { permission(id: \$id) { action } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "G2.4 filtering BY action works" "the same row, reached through the other hidden half"
gql "query { permissions(where: {resource: {eq: \"$R_D\"}, action: {eq: \"insert\"}}) { totalCount } }"
assert_gql_ok '.data.permissions.totalCount' "1"

case_ "G2.5 ordering by a hidden half works" "200 with no errors — the ordering vocabulary lives on the Request DTO"
gql "query { permissions(where: {resource: {eq: \"$R_D\"}}, orderBy: [{field: ACTION, direction: DESC}]) { edges { node { permission } } } }"
assert_gql_ok '.data.permissions.edges[0].node.permission' "$R_D:insert"

case_ "G2.6 ordering by the COMPUTED field is a schema error" "the enum has no PERMISSION member — a computed path backs no column"
gql "query { permissions(orderBy: [{field: PERMISSION, direction: ASC}]) { totalCount } }"
assert_gql_validation "PERMISSION"

case_ "G2.7 the node still carries the rendered pair" "the derivation runs once per document, in the query, not per surface"
gql "query(\$id: ID!) { permission(id: \$id) { permission } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_ok '.data.permission.permission' "$R_D:insert"

case_ "G2.8 __typename beside the selection answers identically" "the same values, with __typename resolved — every mainstream client appends it to every selection set"
gql "query(\$id: ID!) { permission(id: \$id) { __typename permission } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_ok '[.data.permission.__typename, .data.permission.permission] | @csv' "\"Permission\",\"$R_D:insert\""

# ═════════════════════════════════════════════════════════════════════════════════════════
# G3 — validation and the 409, in the GraphQL envelope
# ═════════════════════════════════════════════════════════════════════════════════════════

D_OK="A description long enough to satisfy the shared anti-junk floor of this service."

case_ "G3.1 an uppercase resource" "200 with InvalidResourceNameNotification in errors[].extensions"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg d "$D_OK" '{i:{resource:"Tenant", action:"read", description:$d}}')"
assert_gql InvalidResourceNameNotification

case_ "G3.2 an action carrying a colon" "200 with InvalidActionNameNotification"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg d "$D_OK" '{i:{resource:"user", action:"profile:read", description:$d}}')"
assert_gql InvalidActionNameNotification

case_ "G3.3 a wildcard resource with a concrete action" "200 with UnmatchablePermissionKeyNotification — the pair-level rule crosses the surface intact"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg d "$D_OK" '{i:{resource:"*", action:"read", description:$d}}')"
assert_gql UnmatchablePermissionKeyNotification

case_ "G3.4 a duplicate pair" "200 with PermissionAlreadyExistsNotification"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg r "$R_D" '{i:{resource:$r, action:"insert", description:"A second attempt at the doctrine fixture pair, which the pre-check refuses."}}')"
assert_gql PermissionAlreadyExistsNotification

case_ "G3.5 the GraphQL 409 echoes the same field the REST one does" "permission — one concept, one name, on both surfaces"
assert_json '[.errors[] | select(.extensions.notificationKey=="PermissionAlreadyExistsNotification") | .extensions.field] | first' "permission"

# ═════════════════════════════════════════════════════════════════════════════════════════
# G4 — the surface's own idiom for controls REST expresses as query parameters
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G4.1 an undeclared argument is cut out of the schema" "a gqlparser validation error, never the REST envelope — search is not an argument here"
gql "query { permissions(search: \"tenant\") { totalCount } }"
assert_gql_validation "search"

case_ "G4.2 fields has no argument on this surface" "a validation error — selection IS the projection here, so ?fields= has no GraphQL twin"
gql "query { permissions(fields: \"id\") { totalCount } }"
assert_gql_validation "fields"

case_ "G4.3 onlyTotal has no argument either" "a validation error"
gql "query { permissions(onlyTotal: true) { totalCount } }"
assert_gql_validation "onlyTotal"

case_ "G4.4 selecting totalCount alone IS the only-total mode" "a count with no edges requested"
gql "query { permissions { totalCount } }"
assert_gql_ok '(.data.permissions.totalCount > 0)' "true"

case_ "G4.5 the page ceiling still applies, and a ceiling case must select edges" "LimitExceededNotification — selecting totalCount alone would be the only-total mode instead"
gql "query { permissions(first: 101) { totalCount edges { node { id } } } }"
assert_gql LimitExceededNotification

case_ "G4.6 includeArchived IS an argument here" "the archived G1 row is reachable through the connection"
gql "query { permissions(where: {resource: {eq: \"$R_G\"}}, includeArchived: true) { totalCount } }"
assert_gql_ok '.data.permissions.totalCount' "1"

case_ "G4.7 and it is genuinely applied, not ignored" "0 without it"
gql "query { permissions(where: {resource: {eq: \"$R_G\"}}) { totalCount } }"
assert_gql_ok '.data.permissions.totalCount' "0"

# ═════════════════════════════════════════════════════════════════════════════════════════
# G5 — absent verbs and addressing, in this surface's idiom
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G5.1 there is no unarchivePermission mutation" "'Cannot query field' — the GraphQL twin of the REST 404 arm; the mode does not exist, so neither does the field"
gql "mutation(\$id: ID!) { unarchivePermission(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "G5.2 there is no deletePermission mutation either" "'Cannot query field' — a purge would destroy the only record of what a past grant meant"
gql "mutation(\$id: ID!) { deletePermission(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_D" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "G5.3 an unknown but well-formed id" "RecordNotFoundNotification, at HTTP 200"
gql 'query { permission(id: "0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410") { id } }'
assert_gql RecordNotFoundNotification

case_ "G5.4 a non-uuid on a READ field" "UnknownIDAddressNotification — the same verb split REST makes, on this surface"
gql 'query { permission(id: "not-a-uuid") { id } }'
assert_gql UnknownIDAddressNotification

case_ "G5.5 a non-uuid on a WRITE field" "MalformedIDNotification — a write states an intention about a record"
gql 'mutation { archivePermission(id: "not-a-uuid") { success id } }'
assert_gql MalformedIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# G6 — handler invariance: one operation, two surfaces, one effect
# ═════════════════════════════════════════════════════════════════════════════════════════

R_INV=$(pair_resource inv)
case_ "G6.1 a row created through GraphQL is readable through REST" "the same rendered pair, byte for byte"
gql "mutation(\$i: CreatePermissionInput!) { createPermission(input: \$i) { id } }" \
    "$(jq -nc --arg r "$R_INV" '{i:{resource:$r, action:"read", description:"The handler-invariance fixture, written on one surface and read on the other."}}')"
ID_INV=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createPermission.id')
api GET "/permissions/$ID_INV"
assert_json_at 200 '.data.permission' "$R_INV:read"

case_ "G6.2 and the REST read is lean in the same way" "no resource, no action — the Response is the single wire authority for BOTH surfaces"
assert_json_at 200 '[(.data | has("resource")), (.data | has("action"))] | @csv' "false,false"

case_ "G6.3 a row created through REST is readable through GraphQL" "the same pair, from the other direction"
R_INV2=$(pair_resource inv2)
api POST /permissions "$(permission_body "$R_INV2" read "The reverse handler-invariance fixture, written on REST and read on GraphQL.")"
ID_INV2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
gql "query(\$id: ID!) { permission(id: \$id) { permission } }" "$(jq -nc --arg id "$ID_INV2" '{id:$id}')"
assert_gql_ok '.data.permission.permission' "$R_INV2:read"

qa_finish
