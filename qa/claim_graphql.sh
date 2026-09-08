#!/usr/bin/env bash
# Lane: claim_graphql — the GraphQL half of the framework contract for the Claim aggregate.
#
# Family X of specs/qa/claim-contract/plan.md §1. Two idioms, by design: a typed domain
# refusal answers HTTP 200 with the notificationKey in errors[].extensions; a request the
# SCHEMA cannot express is cut by gqlparser before any resolver and carries no key at all.
#
# WHAT THIS LANE EXISTS TO PIN, beyond parity:
#
# 1. THE CLOSED DOOR ANSWERS DIFFERENTLY DEPENDING ON WHERE THE FIELD IS WRITTEN. `name`
#    and `valueType` are excluded from PatchClaimInput, so writing them INTO THE DOCUMENT is
#    a validation error — while passing the same keys through a VARIABLE is dropped in
#    silence and answers 200. Only the literal is validated. A suite asserting one half
#    would read the other as a regression, which is exactly why X13 asserts both.
#
# 2. THE SORT VOCABULARY IS INTROSPECTED, NEVER GUESSED. ClaimOrderField must carry exactly
#    the seven members read.byParams.sort declares — so a capability cannot come to exist on
#    one surface alone, and `tenantArchivedAt` must have NO member at all.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init claim_graphql

# Every field the Claim READ type declares — thirteen names, no composite value object, so
# each is a field of its own.
NODE="id tenantID name valueType appliesTo defaultValue description createdAt updatedAt archivedAt tenantWorkspace tenantStatus tenantArchivedAt"

TEN_X=$(new_tenant active "$(ws gqlclm)") || exit 1
api GET "/tenants/$TEN_X"; WS_X=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')

# ═════════════════════════════════════════════════════════════════════════════════════════
# X1 — the five mounted fields
# ═════════════════════════════════════════════════════════════════════════════════════════

N_X=$(claim_name gqlx)
case_ "X1.1 createClaim" "the record as stored, on this surface"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id tenantID name valueType appliesTo defaultValue description } }" \
    "$(jq -nc --arg n "$N_X" --arg t "$TEN_X" \
       '{i:{name:$n, valueType:"number", appliesTo:"both", tenantID:$t, defaultValue:"1000",
            description:"The GraphQL golden definition, carrying every field the catalog declares."}}')"
assert_gql_ok '.data.createClaim.name' "$N_X"
ID_X=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createClaim.id')
[ -n "$ID_X" ] && [ "$ID_X" != "null" ] || { echo "claim_graphql.sh: createClaim did not answer an id: $HTTP_BODY" >&2; exit 1; }

case_ "X1.1b HANDLER INVARIANCE: the mutation's effect is visible over REST immediately" "the same record through the other surface — one handler, no second implementation"
api GET "/claims/$ID_X"
assert_json_at 200 '.data.name' "$N_X"

case_ "X1.2 claim(id:) — the singular node, every declared field selectable" "all thirteen names resolve"
gql "query(\$id: ID!) { claim(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_X" '{id:$id}')"
assert_gql_ok '.data.claim.name' "$N_X"

case_ "X1.2b ...and the three joined fields carry the counterpart's values" "tenantWorkspace, tenantStatus, tenantArchivedAt — the join is served here exactly as it is over REST"
assert_gql_ok '[.data.claim.tenantWorkspace, .data.claim.tenantStatus, (.data.claim.tenantArchivedAt|tostring)] | join(",")' "$WS_X,active,null"

case_ "X1.3 claims(...) — the Relay connection over the same view the REST listing reads" "one row"
gql "query { claims(where: {name: {eq: \"$N_X\"}}) { totalCount edges { cursor node { id name } } pageInfo { hasNextPage hasPreviousPage } } }"
assert_gql_ok '.data.claims.totalCount' "1"

case_ "X1.3b the connection node IS the document the REST row is" "the same record under edges[].node"
assert_gql_ok '.data.claims.edges[0].node.name' "$N_X"

case_ "X1.4 patchClaim" "the record after the change — the three editable fields only"
gql "mutation(\$id: ID!, \$i: PatchClaimInput!) { patchClaim(id: \$id, input: \$i) { id description appliesTo } }" \
    "$(jq -nc --arg id "$ID_X" '{id:$id, i:{description:"The GraphQL golden definition, restated through its own surface."}}')"
assert_gql_ok '.data.patchClaim.description' "The GraphQL golden definition, restated through its own surface."

N_XA=$(claim_name gqlxa)
ID_XA=$(new_claim "$N_XA" string both "$TEN_X") || exit 1
case_ "X1.5 archiveClaim" "{success} — REST answers 204 with no body; a GraphQL field must answer SOMETHING"
gql "mutation(\$id: ID!) { archiveClaim(id: \$id) { success } }" "$(jq -nc --arg id "$ID_XA" '{id:$id}')"
assert_gql_ok '.data.archiveClaim.success' "true"

case_ "X1.5b ...and there is no unarchiveClaim field to call" "Cannot query field — the absent verb is absent from the schema, not merely unmounted on REST"
gql "mutation(\$id: ID!) { unarchiveClaim(id: \$id) { success } }" "$(jq -nc --arg id "$ID_XA" '{id:$id}')"
assert_gql_validation "unarchiveClaim"

# ═════════════════════════════════════════════════════════════════════════════════════════
# X2 — the write/read asymmetry, enforced by the SCHEMA rather than by a projection
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "X2.1 the write payload has no traversal field" "Cannot query field tenantWorkspace — the write side never speaks the join, on either surface"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id tenantWorkspace } }" \
    "$(jq -nc --arg n "$(claim_name gqlx2)" --arg t "$TEN_X" '{i:{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, description:"A definition asking the write payload for a value the write side never holds."}}')"
assert_gql_validation "Cannot query field"

case_ "X2.2 the write payload has no managed stamp either" "Cannot query field createdAt on the create payload"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id createdAt } }" \
    "$(jq -nc --arg n "$(claim_name gqlx3)" --arg t "$TEN_X" '{i:{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, description:"A definition asking the write payload for a stamp only the read side carries."}}')"
assert_gql_validation "Cannot query field"

# ═════════════════════════════════════════════════════════════════════════════════════════
# X3 — __typename, the standing regression guard (pin >= v0.72.1)
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "X3.1 __typename beside a normal selection answers identically" "the same values, with __typename resolved — every mainstream client appends it to every selection set for cache normalisation, and below v0.72.1 its presence widened the projection"
gql "query(\$id: ID!) { claim(id: \$id) { __typename name valueType tenantWorkspace } }" "$(jq -nc --arg id "$ID_X" '{id:$id}')"
assert_gql_ok '[.data.claim.__typename, .data.claim.name, .data.claim.valueType, .data.claim.tenantWorkspace] | join("|")' "Claim|$N_X|number|$WS_X"

case_ "X3.2 and it does not widen the projection" "a narrowed selection stays narrow with __typename riding along — nothing unasked-for appears"
gql "query(\$id: ID!) { claim(id: \$id) { __typename name } }" "$(jq -nc --arg id "$ID_X" '{id:$id}')"
assert_gql_ok '[.data.claim | keys] | flatten | join(",")' "__typename,name"

# ═════════════════════════════════════════════════════════════════════════════════════════
# X4 — the surface's OWN idiom: what the schema cannot express is cut before any resolver
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "X4.1 an undeclared filter argument" "a validation message with NO notificationKey — gqlparser, not the read engine"
gql "query { claims(bogus: { eq: \"x\" }) { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "X4.2 an unknown field inside where" "not a field of the where input"
gql "query { claims(where: {bogus: {eq: \"x\"}}) { totalCount } }"
assert_gql_validation "bogus"

case_ "X4.3 an operator outside a leaf's allowlist" "valueType declares eq and in, so contains is a field its operator input does not define"
gql "query { claims(where: {valueType: {contains: \"num\"}}) { totalCount } }"
assert_gql_validation "contains"

case_ "X4.4 a filter over tenantArchivedAt" "not a field of the where input — served and projectable (X4.4b), addressable in NO criteria on EITHER surface"
gql "query { claims(where: {tenantArchivedAt: {gte: \"2020-01-01T00:00:00Z\"}}) { totalCount } }"
assert_gql_validation "tenantArchivedAt"

case_ "X4.4b ...and it IS selectable" "the same field, the other half of the split — the schema serves what the criteria refuses"
gql "query(\$id: ID!) { claim(id: \$id) { id tenantArchivedAt } }" "$(jq -nc --arg id "$ID_X" '{id:$id}')"
assert_gql_ok '.data.claim | has("tenantArchivedAt")' "true"

case_ "X4.5 ?search= has no GraphQL twin either" "Unknown argument — the DTO declares none, so the schema grew none"
gql "query { claims(search: \"cost\") { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "X4.6 selection is NATURAL here — there is no fields argument to gate" "Unknown argument: a caller shapes the response by selecting, so the REST opt-in gate has nothing to guard on this surface"
gql "query { claims(fields: \"name\") { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "X4.7 the page ceiling holds on this surface too" "a typed refusal, not a validation message — the bound is the read engine's, not the schema's"
gql "query { claims(first: 100000) { totalCount } }"
if printf '%s' "$HTTP_BODY" | jq -e '(.errors | length) > 0' >/dev/null 2>&1; then pass_; else fail_ "no error for first: 100000"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# X5 — a typed refusal rides errors[].extensions with HTTP 200
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "X5.1 a name that never carried the reserved prefix" "200 + InvalidClaimNameNotification — the VO refusal travels to this surface unchanged"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id } }" \
    "$(jq -nc --arg t "$TEN_X" '{i:{name:"cost_center", valueType:"string", appliesTo:"both", tenantID:$t, description:"A definition whose name never carried the reserved prefix."}}')"
assert_gql InvalidClaimNameNotification

case_ "X5.2 a value type outside the closed set" "200 + UnknownClaimValueTypeNotification"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id } }" \
    "$(jq -nc --arg n "$(claim_name gqlx5b)" --arg t "$TEN_X" '{i:{name:$n, valueType:"json", appliesTo:"both", tenantID:$t, description:"A definition asking for a type the catalog does not accept."}}')"
assert_gql UnknownClaimValueTypeNotification

case_ "X5.3 an identity kind outside the closed set" "200 + UnknownClaimAppliesToNotification"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id } }" \
    "$(jq -nc --arg n "$(claim_name gqlx5c)" --arg t "$TEN_X" '{i:{name:$n, valueType:"string", appliesTo:"service", tenantID:$t, description:"A definition naming an identity kind this service does not mint."}}')"
assert_gql UnknownClaimAppliesToNotification

case_ "X5.4 a duplicate name" "200 + ClaimNameAlreadyExistsNotification — the 409 flavor, in this surface's envelope"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id } }" \
    "$(jq -nc --arg n "$N_X" --arg t "$TEN_X" '{i:{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, description:"A collider reaching for a name another active definition of this tenant already holds."}}')"
assert_gql ClaimNameAlreadyExistsNotification

case_ "X5.5 a default that does not parse as the declared type" "200 + DefaultValueDoesNotMatchValueTypeNotification — the rule that reads another field, on this surface"
gql "mutation(\$i: CreateClaimInput!) { createClaim(input: \$i) { id } }" \
    "$(jq -nc --arg n "$(claim_name gqlx5e)" --arg t "$TEN_X" '{i:{name:$n, valueType:"number", appliesTo:"both", tenantID:$t, defaultValue:"abc", description:"A definition whose default cannot be the number it declares."}}')"
assert_gql DefaultValueDoesNotMatchValueTypeNotification

case_ "X5.6 an absent id on the singular read" "200 with a null node AND RecordNotFoundNotification — both halves, because a null node with no error would be indistinguishable from an empty row"
gql "query { claim(id: \"01990000-dead-7000-8000-000000000000\") { id } }"
assert_gql RecordNotFoundNotification
case_ "X5.6b ...and the node itself is null" "the other half of the same answer"
assert_json '.data.claim' "null"

case_ "X5.7 a by-id address that is not a uuid, on a READ" "200 + UnknownIDAddressNotification — the v0.70.0 split by VERB, not by surface"
gql "query { claim(id: \"lixo\") { id } }"
assert_gql UnknownIDAddressNotification

case_ "X5.8 the same address on a WRITE" "200 + MalformedIDNotification — the other half of the split"
gql "mutation(\$i: PatchClaimInput!) { patchClaim(id: \"lixo\", input: \$i) { id } }" '{"i":{"description":"A write aimed at an address that is not an id at all."}}'
assert_gql MalformedIDNotification

case_ "X5.9 ...and on the ARCHIVE" "200 + MalformedIDNotification — every write obeys the same rule"
gql "mutation { archiveClaim(id: \"lixo\") { success } }"
assert_gql MalformedIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# X6 — the ordering enum, INTROSPECTED rather than guessed
# ═════════════════════════════════════════════════════════════════════════════════════════

gql 'query { __type(name: "ClaimOrderField") { enumValues { name } } }'
ENUM_MEMBERS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data.__type.enumValues[]?.name] | sort | join(" ")')

case_ "X6.1 the ClaimOrderField enum exists and is not empty" "a reflected enum — sorting on this surface is a typed input, not the REST string"
if [ -n "$ENUM_MEMBERS" ]; then pass_; else fail_ "no enum members"; fi

case_ "X6.2 PARITY: the enum has exactly SEVEN members" "7 — the same vocabulary read.byParams.sort declares, so the capability cannot come to exist on one surface alone"
N_ENUM=$(printf '%s' "$ENUM_MEMBERS" | wc -w | tr -d ' ')
if [ "$N_ENUM" = "7" ]; then pass_; else fail_ "$N_ENUM members: $ENUM_MEMBERS"; fi

case_ "X6.3 and every REST sort key has a member" "name, valueType, appliesTo, tenantID, tenantWorkspace, createdAt, updatedAt all present, however the reflection spells them"
MISSING=""
for fld in NAME VALUETYPE APPLIESTO TENANTID TENANTWORKSPACE CREATEDAT UPDATEDAT; do
  printf '%s' "$ENUM_MEMBERS" | tr -d '_' | tr 'a-z' 'A-Z' | grep -qw "$fld" || MISSING="$MISSING $fld"
done
if [ -z "$MISSING" ]; then pass_; else fail_ "missing:$MISSING (enum: $ENUM_MEMBERS)"; fi

case_ "X6.4 the three filterable-but-not-sortable fields have NO member" "defaultValue, description and tenantStatus are absent — the asymmetry W8.12 asserts over REST, held here too"
EXTRA=""
for fld in DEFAULTVALUE DESCRIPTION TENANTSTATUS TENANTARCHIVEDAT; do
  printf '%s' "$ENUM_MEMBERS" | tr -d '_' | tr 'a-z' 'A-Z' | grep -qw "$fld" && EXTRA="$EXTRA $fld"
done
if [ -z "$EXTRA" ]; then pass_; else fail_ "present but must not be:$EXTRA (enum: $ENUM_MEMBERS)"; fi

GQL_NAME_MEMBER=$(printf '%s' "$ENUM_MEMBERS" | tr ' ' '\n' | awk '{u=$0; gsub(/_/,"",u); if (toupper(u)=="NAME") print $0}' | head -1)
case_ "X6.5 the enum is USABLE, not merely declared" "an ordered connection, ascending by name"
gql "query { claims(where: {tenantID: {eq: \"$TEN_X\"}}, orderBy: [{field: $GQL_NAME_MEMBER, direction: ASC}], first: 1) { edges { node { name } } } }"
assert_gql_ok '(.data.claims.edges | length)' "1"

case_ "X6.6 PARITY on the count: the same filter answers the same total on both surfaces" "identical totalCount — one criteria vocabulary, two surfaces"
gql "query { claims(where: {tenantID: {eq: \"$TEN_X\"}}) { totalCount } }"
GQL_TOTAL=$(printf '%s' "$HTTP_BODY" | jq -r '.data.claims.totalCount')
api GET "/claims?tenantID.eq=$TEN_X&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "$GQL_TOTAL"

case_ "X6.7 includeArchived reveals the retired definition here too" "the archived row of X1.5 appears only when asked for"
gql "query { claims(where: {tenantID: {eq: \"$TEN_X\"}}, includeArchived: true) { totalCount } }"
WITH_ARC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.claims.totalCount')
if [ "$WITH_ARC" -gt "$GQL_TOTAL" ] 2>/dev/null; then pass_; else fail_ "active=$GQL_TOTAL includeArchived=$WITH_ARC"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# X13 — the closed door, and the TWO different answers it gives
#
# The plan's W13 pins the REST half (an unknown JSON key is ignored). Here both GraphQL
# halves, which differ BY DESIGN: only the literal in the document is validated.
# ═════════════════════════════════════════════════════════════════════════════════════════

N_X13=$(claim_name gqlx13)
ID_X13=$(new_claim "$N_X13" number user "$TEN_X" "7") || exit 1

case_ "X13.1 the immutable fields written INTO THE DOCUMENT" "a validation error naming the field — PatchClaimInput does not declare 'name', and gqlparser cuts the request before any resolver"
gql "mutation(\$id: ID!) { patchClaim(id: \$id, input: {name: \"x_renamed_in_the_document\", description: \"An update that also tried to rewrite what every issued token carries.\"}) { id name } }" \
    "$(jq -nc --arg id "$ID_X13" '{id:$id}')"
assert_gql_validation "name"

case_ "X13.1b ...and nothing was written" "the whole request was refused, so even the legal half of that body did not land"
api GET "/claims/$ID_X13"
assert_json '[.data.name, .data.description] | join("|")' "$N_X13|Fixture claim definition created by the QA suite for run $(qa_slug_runid), so the user claims collection has a catalog row to point at."

case_ "X13.2 the SAME keys passed through a VARIABLE" "200 — variable coercion drops an unknown field in SILENCE. The two halves answer differently by design, and a suite asserting only one would read the other as a regression"
gql "mutation(\$id: ID!, \$i: PatchClaimInput!) { patchClaim(id: \$id, input: \$i) { id name valueType tenantID description } }" \
    "$(jq -nc --arg id "$ID_X13" --arg t "$TEN_X" '{id:$id, i:{name:"x_renamed_in_a_variable", valueType:"string", tenantID:$t, description:"An update that smuggled the immutable fields in through a variable."}}')"
assert_gql_ok '.data.patchClaim.name' "$N_X13"

case_ "X13.2b ...and the ONE editable field in that same variable DID change" "the request was honoured, not rejected wholesale — which is what makes the silence worth pinning"
assert_gql_ok '.data.patchClaim.description' "An update that smuggled the immutable fields in through a variable."

case_ "X13.2c ...and the value type is untouched as well" "number, as the insert stored it"
assert_gql_ok '.data.patchClaim.valueType' "number"

qa_finish
