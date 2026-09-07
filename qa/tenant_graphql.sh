#!/usr/bin/env bash
# Lane: tenant_graphql — the GraphQL half of the framework contract for the Tenant aggregate.
#
# Family F10 of specs/qa/tenant-contract/plan.md §1, plus the F1/F3/F4/F7/F8 families repeated
# in this surface's own idiom. A route gated on REST is not thereby gated on GraphQL, and "the
# other surface forgot it" is a real regression that only a per-surface case can catch.
#
# TWO IDIOMS, and the difference is BY DESIGN — the suite must not flatten it:
#   · a typed domain refusal answers HTTP 200 with the same notificationKey the REST envelope
#     carries, in errors[].extensions;
#   · a request the SCHEMA cannot express (an undeclared argument, an enum value that is not in
#     the enum) is cut by gqlparser before any resolver runs and surfaces as a validation
#     message with no notificationKey at all.
# Asserting the REST envelope across the surface boundary would be asserting a promise the pin
# does not make.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init tenant_graphql

NODE='id name workspace description status createdAt updatedAt archivedAt'

# ═════════════════════════════════════════════════════════════════════════════════════════
# F1 — the six mounted fields
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_G=$(ws gql)
case_ "G1.1 createTenant" "the record as stored, under data.createTenant"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id name workspace description status } }" \
    "$(jq -nc --arg w "$WS_G" '{i:{name:"GraphQL Surface Tenant", workspace:$w, description:"The tenant created through the GraphQL mutation of this suite.", status:"active"}}')"
assert_gql_ok '.data.createTenant.workspace' "$WS_G"
ID_G=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createTenant.id')

case_ "G1.2 tenant(id:) — the singular by-id field" "the same record"
gql "query(\$id: ID!) { tenant(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.tenant.workspace' "$WS_G"

case_ "G1.3 tenants(...) — the Relay connection" "edges/node/cursor + pageInfo + totalCount"
gql "query { tenants(where: {workspace: {eq: \"$WS_G\"}}) { totalCount edges { cursor node { $NODE } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } } }"
assert_gql_ok '.data.tenants.edges[0].node.workspace' "$WS_G"

case_ "G1.4 totalCount is carried by the connection" "1"
assert_json '.data.tenants.totalCount' "1"

case_ "G1.5 patchTenant" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchTenantInput!) { patchTenant(id: \$id, input: \$i) { id name status } }" \
    "$(jq -nc --arg id "$ID_G" '{id:$id, i:{name:"GraphQL Surface Renamed"}}')"
assert_gql_ok '.data.patchTenant.name' "GraphQL Surface Renamed"

case_ "G1.6 archiveTenant" "the fixed bodyless payload { success, id }"
gql "mutation(\$id: ID!) { archiveTenant(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.archiveTenant.success' "true"

case_ "G1.7 the archived record is hidden here too" "data.tenant is null, with the canonical not-found in errors[]"
gql "query(\$id: ID!) { tenant(id: \$id) { id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql RecordNotFoundNotification

case_ "G1.8 includeArchived reveals it" "the record, archived, and SUSPENDED — archive forces the status on this surface too"
gql "query(\$id: ID!) { tenant(id: \$id, includeArchived: true) { id status archivedAt } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.tenant.status' "suspended"

case_ "G1.9 unarchiveTenant" "success, and the record is reachable again"
gql "mutation(\$id: ID!) { unarchiveTenant(id: \$id) { success id } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.unarchiveTenant.success' "true"

case_ "G1.10 archivedAt is cleared on this surface" "null"
gql "query(\$id: ID!) { tenant(id: \$id) { archivedAt } }" "$(jq -nc --arg id "$ID_G" '{id:$id}')"
assert_gql_ok '.data.tenant.archivedAt' "null"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F2/F11 — handler invariance: the same operation, either surface, one answer
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G2.1 a record created on GraphQL is readable on REST" "the same workspace, byte for byte"
api GET "/tenants/$ID_G"
assert_json_at 200 '.data.workspace' "$WS_G"

WS_R=$(ws rest2gql)
api POST /tenants "$(tenant_body "Cross Surface Tenant" "$WS_R" "The tenant created on REST and read back through the GraphQL node." "trial")"
ID_R=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "G2.2 a record created on REST is readable on GraphQL" "the same status, unchanged"
gql "query(\$id: ID!) { tenant(id: \$id) { workspace status } }" "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql_ok '.data.tenant.status' "trial"

case_ "G2.3 __typename beside the selection answers identically" "the same values, with __typename resolved — every mainstream client appends it to every selection set"
gql "query(\$id: ID!) { tenant(id: \$id) { __typename workspace status } }" "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql_ok '[.data.tenant.__typename, .data.tenant.status] | @csv' '"Tenant","trial"'

# ═════════════════════════════════════════════════════════════════════════════════════════
# F3/F4 — the domain refusals, in the GraphQL rendering
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G3.1 keyboard-junk name" "InvalidDisplayNameNotification in errors[].extensions, HTTP 200"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id } }" \
    "$(jq -nc --arg w "$(ws gv)" '{i:{name:"aaaa", workspace:$w, description:"A perfectly ordinary description of a tenant.", status:"active"}}')"
assert_gql InvalidDisplayNameNotification

case_ "G3.2 reserved workspace" "ReservedTenantWorkspaceNotification"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id } }" \
    '{"i":{"name":"Valid Name","workspace":"graphql","description":"A perfectly ordinary description of a tenant.","status":"active"}}'
assert_gql ReservedTenantWorkspaceNotification

case_ "G3.3 status outside the closed set" "UnknownTenantStatusNotification"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id } }" \
    "$(jq -nc --arg w "$(ws gv)" '{i:{name:"Valid Name", workspace:$w, description:"A perfectly ordinary description of a tenant.", status:"paused"}}')"
assert_gql UnknownTenantStatusNotification

case_ "G4.1 duplicate workspace" "TenantWorkspaceAlreadyExistsNotification — the same key the REST envelope carries"
gql "mutation(\$i: CreateTenantInput!) { createTenant(input: \$i) { id } }" \
    "$(jq -nc --arg w "$WS_R" '{i:{name:"GraphQL Collider", workspace:$w, description:"A tenant reaching for a handle that is already held.", status:"active"}}')"
assert_gql TenantWorkspaceAlreadyExistsNotification

case_ "G4.2 the extensions carry the semantic too" "'Conflict' — the duplicate flavor, distinguishable from StateConflict"
assert_json '[.errors[]?.extensions?.semantic] | first' "Conflict"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F7 — controls and their refusals, in this surface's idiom
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G7.1 where + orderBy + first, the declared vocabulary" "a page, ordered by the reflected enum"
gql "query { tenants(where: {name: {startswith: \"Cross Surface\"}}, orderBy: [{field: NAME, direction: DESC}], first: 5) { totalCount edges { node { name } } } }"
assert_gql_ok '(.data.tenants.totalCount >= 1)' "true"

case_ "G7.2 first above the ceiling" "LimitExceededNotification — the same ceiling, resolved by the same cascade, on both surfaces"
gql "query { tenants(first: 101) { totalCount edges { node { id } } } }"
assert_gql LimitExceededNotification

case_ "G7.3 an undeclared control is CUT FROM THE SCHEMA" "a gqlparser validation error about an unknown argument — NOT a 400 envelope: the DTO declares no Search, so the argument does not exist"
gql "query { tenants(search: \"acme\") { totalCount } }"
assert_gql_validation "search"

case_ "G7.4 ?fields= has no GraphQL argument at all" "unknown argument — field selection IS the selection set here, so there is nothing to gate"
gql "query { tenants(fields: \"id\") { totalCount } }"
assert_gql_validation "fields"

case_ "G7.5 onlyTotal has no GraphQL argument either" "unknown argument — only-total is a selection shape on this surface"
gql "query { tenants(onlyTotal: true) { totalCount } }"
assert_gql_validation "onlyTotal"

case_ "G7.6 selecting totalCount alone IS the only-total mode" "a count with no edges requested"
gql "query { tenants { totalCount } }"
assert_gql_ok '(.data.tenants.totalCount >= 1)' "true"

case_ "G7.7 an order field outside the enum" "a validation error — STATUS is filterable, not sortable, so introspection never advertises it"
gql "query { tenants(orderBy: [{field: STATUS}]) { totalCount } }"
assert_gql_validation "STATUS"

case_ "G7.8 an operator outside a leaf's allowlist" "a validation error — the where input exposes only the declared operators per leaf"
gql "query { tenants(where: {status: {contains: \"act\"}}) { totalCount } }"
assert_gql_validation "contains"

case_ "G7.9 a malformed cursor" "SchemaViolationNotification — cursors are checked before the resolver runs"
gql "query { tenants(first: 2, after: \"not-a-cursor\") { totalCount edges { node { id } } } }"
assert_gql SchemaViolationNotification

case_ "G7.10 mixed directions" "SchemaViolationNotification — one direction at a time, the same rule as REST"
gql "query { tenants(first: 2, last: 2) { totalCount edges { node { id } } } }"
assert_gql SchemaViolationNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# F8 — addressing, split by verb, on this surface too
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G8.1 a well-formed id that matches no row" "RecordNotFoundNotification"
gql "query { tenant(id: \"019903c2-6b41-7c9e-9f2a-6d3b1e77aaaa\") { id } }"
assert_gql RecordNotFoundNotification

case_ "G8.2 a non-uuid on a READ address" "UnknownIDAddressNotification — the read names no record"
gql "query { tenant(id: \"not-a-uuid\") { id } }"
assert_gql UnknownIDAddressNotification

case_ "G8.3 a non-uuid on a WRITE address" "MalformedIDNotification — the write states an intention about one"
gql "mutation { archiveTenant(id: \"not-a-uuid\") { success } }"
assert_gql MalformedIDNotification

case_ "G8.4 a non-uuid on the patch address" "MalformedIDNotification"
gql "mutation(\$i: PatchTenantInput!) { patchTenant(id: \"not-a-uuid\", input: \$i) { id } }" '{"i":{"name":"Whatever Name"}}'
assert_gql MalformedIDNotification

case_ "G8.5 unarchive on an ACTIVE record" "RecordNotFoundNotification — the unarchive load runs OnlyArchived here too"
gql "mutation(\$id: ID!) { unarchiveTenant(id: \$id) { success } }" "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql RecordNotFoundNotification

case_ "G8.6 there is no deleteTenant field" "a validation error — the aggregate declares no delete mode, so the schema carries no such mutation"
gql "mutation(\$id: ID!) { deleteTenant(id: \$id) { success } }" "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql_validation "deleteTenant"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F9 — workspace immutability on GraphQL: the field is not in the input type at all
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "G9.1 PatchTenantInput carries no workspace field" "a validation error on the input field — the structural cut reaches the SDL, so the handle cannot even be named"
gql "mutation(\$id: ID!) { patchTenant(id: \$id, input: {name: \"Still Fine\", workspace: \"hijacked-handle\"}) { id workspace } }" \
    "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql_validation "workspace"

case_ "G9.2 and the handle is untouched" "the original workspace"
gql "query(\$id: ID!) { tenant(id: \$id) { workspace } }" "$(jq -nc --arg id "$ID_R" '{id:$id}')"
assert_gql_ok '.data.tenant.workspace' "$WS_R"

qa_finish
