#!/usr/bin/env bash
# Lane: client_graphql — the GraphQL half of the framework contract for the Client aggregate.
#
# Family V of specs/qa/client-contract/plan.md §1. Two idioms, by design (see
# qa/user_graphql.sh's header): a typed domain refusal answers HTTP 200 with the
# notificationKey in errors[].extensions; a request the SCHEMA cannot express is cut by
# gqlparser before any resolver and carries no key at all.
#
# WHAT THIS LANE EXISTS TO PIN, beyond parity:
#
# 1. THE SECRET IS A FIELD OF EXACTLY TWO PAYLOAD TYPES — createClient's and
#    rotateClientSecret's — and of nothing else. On the Client READ type, `secret`,
#    `secretHash`, `previousSecretHash` and `tenantStatus` are not fields at all, so the
#    refusal comes from the schema, before any resolver: the strongest guarantee available
#    anywhere in this suite.
#
# 2. THE GRACE WINDOW IS NULLABLE IN THE SCHEMA AND THE TWO ABSENCES DIFFER. Omitting
#    gracePeriodSeconds means "the usual day" and sending 0 means "kill it now" — a
#    non-null Int would collapse the two and turn every unstated rotation into an
#    immediate revocation (client_secret_routes_manual.go says so in as many words).

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init client_graphql

# Every field the Client READ type declares. The three entry selections are COMPLETE, and
# the credential is in none of them — V2 asserts that from the schema's own side.
R_ENTRY='id roleID roleKey roleName roleArchivedAt'
N_ENTRY='id cidr label'
C_ENTRY='id claimID value claimName claimValueType claimArchivedAt'
NODE="id tenantID name description secretChangedAt previousSecretExpiresAt status createdAt updatedAt archivedAt tenantWorkspace tenantArchivedAt roles { $R_ENTRY } allowedCIDRs { $N_ENTRY } claims { $C_ENTRY }"

TEN_V=$(new_tenant active "$(ws gqlcli)") || exit 1
P_TENANT_READ=$(permission_id_of tenant read)
[ -n "$P_TENANT_READ" ] || { echo "client_graphql.sh: a seeded catalog id could not be resolved" >&2; exit 1; }

R_V=$(new_role "$(role_key gqlc)" "$TEN_V" "$P_TENANT_READ") || exit 1
C_V=$(new_claim "$(claim_name gqlc)" string both "$TEN_V") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# V1 — the thirteen mounted fields
# ═════════════════════════════════════════════════════════════════════════════════════════

N_V="QA GraphQL Client One"
case_ "V1.1 createClient — the FIRST reveal seat, on this surface" "the record as stored, secret selectable and matching ^acs_ + 43 runes"
gql "mutation(\$i: CreateClientInput!) { createClient(input: \$i) { id tenantID name status secret roles { id roleID } allowedCIDRs { id cidr label } claims { id claimID value } } }" \
    "$(jq -nc --arg n "$N_V" --arg t "$TEN_V" --arg r "$R_V" --arg c "$C_V" \
       '{i:{name:$n, description:"The GraphQL golden client, carrying one entry per collection.", status:"active", tenantID:$t,
            roles:[{roleID:$r}], allowedCIDRs:[{cidr:"203.0.113.0/24", label:"QA GraphQL egress"}], claims:[{claimID:$c, value:"gql"}]}}')"
assert_gql_ok '[.data.createClient.name, (.data.createClient.secret|test("^acs_[A-Za-z0-9_-]{43}$"))] | map(tostring) | join(",")' "$N_V,true"
ID_V=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createClient.id')
[ -n "$ID_V" ] && [ "$ID_V" != "null" ] || { echo "client_graphql.sh: createClient did not answer an id: $HTTP_BODY" >&2; exit 1; }

case_ "V1.1b HANDLER INVARIANCE: the mutation's effect is visible over REST immediately" "the same record through the other surface — one handler, no second implementation"
api GET "/clients/$ID_V"
assert_json_at 200 '.data.name' "$N_V"

case_ "V1.2 client(id:)" "the singular node, every declared field selectable"
gql "query(\$id: ID!) { client(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"
assert_gql_ok '.data.client.name' "$N_V"

case_ "V1.3 clients(...)" "the Relay connection over the same view the REST listing reads"
gql "query { clients(where: {name: {eq: \"$N_V\"}}) { totalCount edges { cursor node { id name } } pageInfo { hasNextPage hasPreviousPage } } }"
assert_gql_ok '.data.clients.totalCount' "1"

case_ "V1.3b the connection node is the same document the REST row is" "the same record under edges[].node"
assert_gql_ok '.data.clients.edges[0].node.name' "$N_V"

case_ "V1.4 patchClient" "the record after the change"
gql "mutation(\$id: ID!, \$i: PatchClientInput!) { patchClient(id: \$id, input: \$i) { id description } }" \
    "$(jq -nc --arg id "$ID_V" '{id:$id, i:{description:"The GraphQL golden client, relabelled through its own surface."}}')"
assert_gql_ok '.data.patchClient.description' "The GraphQL golden client, relabelled through its own surface."

gql "mutation(\$i: CreateClientInput!) { createClient(input: \$i) { id } }" \
    "$(jq -nc --arg t "$TEN_V" '{i:{name:"QA GraphQL Client Two", description:"The collection-verbs client of the V family.", status:"active", tenantID:$t}}')"
ID_VC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createClient.id')

case_ "V1.5 addClientRole" "the entry as stored, with the id the server minted"
gql "mutation(\$id: ID!, \$i: AddClientRoleInput!) { addClientRole(id: \$id, input: \$i) { clientId clientRole { id roleID } } }" \
    "$(jq -nc --arg id "$ID_VC" --arg r "$R_V" '{id:$id, i:{roleID:$r}}')"
assert_gql_ok '.data.addClientRole.clientRole.roleID' "$R_V"
CHV_R=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addClientRole.clientRole.id')

case_ "V1.6 archiveClientRole" "{success} — REST answers 204 with no body; a GraphQL field must answer SOMETHING"
gql "mutation(\$id: ID!, \$i: ArchiveClientRoleInput!) { archiveClientRole(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_VC" --arg c "$CHV_R" '{id:$id, i:{clientRoleId:$c}}')"
assert_gql_ok '.data.archiveClientRole.success' "true"

case_ "V1.7 addClientAllowedCIDR" "the entry as stored: id, cidr, label"
gql "mutation(\$id: ID!, \$i: AddClientAllowedCIDRInput!) { addClientAllowedCIDR(id: \$id, input: \$i) { clientId clientAllowedCIDR { id cidr label } } }" \
    "$(jq -nc --arg id "$ID_VC" '{id:$id, i:{cidr:"198.51.100.0/24", label:"QA GraphQL staging"}}')"
assert_gql_ok '.data.addClientAllowedCIDR.clientAllowedCIDR.cidr' "198.51.100.0/24"
CHV_N=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addClientAllowedCIDR.clientAllowedCIDR.id')

case_ "V1.8 archiveClientAllowedCIDR" "{success}"
gql "mutation(\$id: ID!, \$i: ArchiveClientAllowedCIDRInput!) { archiveClientAllowedCIDR(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_VC" --arg c "$CHV_N" '{id:$id, i:{clientAllowedCIDRId:$c}}')"
assert_gql_ok '.data.archiveClientAllowedCIDR.success' "true"

case_ "V1.9 addClientClaim" "the entry as stored, id + claimID + value"
gql "mutation(\$id: ID!, \$i: AddClientClaimInput!) { addClientClaim(id: \$id, input: \$i) { clientId clientClaim { id claimID value } } }" \
    "$(jq -nc --arg id "$ID_VC" --arg c "$C_V" '{id:$id, i:{claimID:$c, value:"alpha"}}')"
assert_gql_ok '.data.addClientClaim.clientClaim.value' "alpha"
CHV_C=$(printf '%s' "$HTTP_BODY" | jq -r '.data.addClientClaim.clientClaim.id')

case_ "V1.10 patchClientClaim" "the partial shape of the same operation, under its own field — the schema has no verb, so the NAME carries the difference"
gql "mutation(\$id: ID!, \$i: PatchClientClaimInput!) { patchClientClaim(id: \$id, input: \$i) { clientClaim { id value } } }" \
    "$(jq -nc --arg id "$ID_VC" --arg c "$CHV_C" '{id:$id, i:{clientClaimId:$c, value:"beta"}}')"
assert_gql_ok '.data.patchClientClaim.clientClaim.value' "beta"

case_ "V1.11 ...and it kept the entry's id" "the SAME child id, on this surface too"
assert_gql_ok '.data.patchClientClaim.clientClaim.id' "$CHV_C"

case_ "V1.12 archiveClientClaim" "{success}"
gql "mutation(\$id: ID!, \$i: ArchiveClientClaimInput!) { archiveClientClaim(id: \$id, input: \$i) { success } }" \
    "$(jq -nc --arg id "$ID_VC" --arg c "$CHV_C" '{id:$id, i:{clientClaimId:$c}}')"
assert_gql_ok '.data.archiveClientClaim.success' "true"

case_ "V1.13 rotateClientSecret — the SECOND reveal seat, on this surface" "the four-key payload, secret matching the credential shape — no payload type of its own: the REST Response projects the same Result here, so the secret is shown once on this surface too"
gql "mutation(\$id: ID!, \$i: RotateClientSecretInput!) { rotateClientSecret(id: \$id, input: \$i) { id secret secretChangedAt previousSecretExpiresAt } }" \
    "$(jq -nc --arg id "$ID_VC" '{id:$id, i:{}}')"
assert_gql_ok '[.data.rotateClientSecret.id, (.data.rotateClientSecret.secret|test("^acs_[A-Za-z0-9_-]{43}$"))] | map(tostring) | join(",")' "$ID_VC,true"

case_ "V1.14 archiveClient" "{success} — the bodyless REST verb's payload twin"
gql "mutation(\$id: ID!) { archiveClient(id: \$id) { success } }" "$(jq -nc --arg id "$ID_VC" '{id:$id}')"
assert_gql_ok '.data.archiveClient.success' "true"

case_ "V1.14b ...and the archive is visible over REST" "404 without the flag — the same scope, reached from the other surface"
api GET "/clients/$ID_VC"
assert_status 404

# ═════════════════════════════════════════════════════════════════════════════════════════
# V2 — the golden record on this surface, and the absences that come from the SCHEMA
# ═════════════════════════════════════════════════════════════════════════════════════════

gql "query(\$id: ID!) { client(id: \$id) { $NODE } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"

case_ "V2.1 every scalar round-trips identically to REST" "one view, one Response, two renderings"
assert_gql_ok '[.data.client.name,.data.client.status,.data.client.tenantID] | join("|")' "$N_V|active|$TEN_V"

case_ "V2.2 the joined fields are selectable, the join-less collection is bare" "roleKey and claimValueType resolve; the CIDR entry is exactly id+cidr+label"
assert_gql_ok '[(.data.client.roles[0].roleKey|length>0),(.data.client.claims[0].claimValueType),(.data.client.allowedCIDRs[0]|keys|sort|join("+"))] | map(tostring) | join(" / ")' "true / string / cidr+id+label"

case_ "V2.3 secret is NOT a field of the Client READ type" "'Cannot query field' — on this surface the reveal-once discipline is enforced by the schema itself, before any resolver"
gql "query(\$id: ID!) { client(id: \$id) { id secret } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "V2.4 nor is secretHash" "'Cannot query field'"
gql "query(\$id: ID!) { client(id: \$id) { id secretHash } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "V2.5 nor previousSecretHash" "'Cannot query field' — the retiring hash opens no side door here either"
gql "query(\$id: ID!) { client(id: \$id) { id previousSecretHash } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "V2.6 nor tenantStatus — the rules-only join field" "'Cannot query field' — hidden means the schema never grew the name"
gql "query(\$id: ID!) { client(id: \$id) { id tenantStatus } }" "$(jq -nc --arg id "$ID_V" '{id:$id}')"
assert_gql_validation "Cannot query field"

case_ "V2.7 the WRITE payload carries no traversal field" "'Cannot query field roleKey' on the write payload's entry type — the write side never speaks the traversal, on either surface"
gql "mutation(\$id: ID!, \$i: AddClientRoleInput!) { addClientRole(id: \$id, input: \$i) { clientRole { roleKey } } }" \
    "$(jq -nc --arg id "$ID_V" --arg r "$R_V" '{id:$id, i:{roleID:$r}}')"
assert_gql_validation "Cannot query field"

# ═════════════════════════════════════════════════════════════════════════════════════════
# V3 — a typed refusal rides errors[].extensions with HTTP 200
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "V3.1 a duplicate name" "HTTP 200 carrying ClientNameAlreadyExistsNotification — never the REST envelope"
gql "mutation(\$i: CreateClientInput!) { createClient(input: \$i) { id } }" \
    "$(jq -nc --arg n "$N_V" --arg t "$TEN_V" '{i:{name:$n, description:"A collider reaching for a label another active client of this tenant already holds.", status:"active", tenantID:$t}}')"
assert_gql ClientNameAlreadyExistsNotification

case_ "V3.2 a non-canonical CIDR" "200 + CIDRHasHostBitsSetNotification — the VO refusal travels to this surface unchanged"
gql "mutation(\$id: ID!, \$i: AddClientAllowedCIDRInput!) { addClientAllowedCIDR(id: \$id, input: \$i) { clientId } }" \
    "$(jq -nc --arg id "$ID_V" '{id:$id, i:{cidr:"203.0.113.5/24", label:"QA host bits probe"}}')"
assert_gql CIDRHasHostBitsSetNotification

case_ "V3.3 an absent id on the singular read" "200 with a null node AND RecordNotFoundNotification — both halves, because a null node with no error would be indistinguishable from an empty row"
gql "query { client(id: \"01990000-dead-7000-8000-000000000000\") { id } }"
assert_gql RecordNotFoundNotification

case_ "V3.3b ...and the node itself is null" "the other half of the same answer"
assert_json '.data.client' "null"

case_ "V3.4 a by-id address that is not a uuid, on a READ" "200 + UnknownIDAddressNotification — the v0.70.0 split by VERB, not by surface"
gql "query { client(id: \"lixo\") { id } }"
assert_gql UnknownIDAddressNotification

case_ "V3.5 the same address on a WRITE" "200 + MalformedIDNotification — the other half of the split"
gql "mutation(\$i: PatchClientInput!) { patchClient(id: \"lixo\", input: \$i) { id } }" '{"i":{"description":"A write aimed at an address that is not an id at all."}}'
assert_gql MalformedIDNotification

case_ "V3.6 ...and on the ROTATION" "200 + MalformedIDNotification — the hand-written field obeys the same rule"
gql "mutation(\$i: RotateClientSecretInput!) { rotateClientSecret(id: \"lixo\", input: \$i) { id } }" '{"i":{}}'
assert_gql MalformedIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# V4 — the surface's OWN idiom: what the schema cannot express is cut before any resolver
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "V4.1 an undeclared filter argument" "a validation message with NO notificationKey — gqlparser, not the read engine"
gql "query { clients(bogus: { eq: \"x\" }) { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "V4.2 a filter over a CHILD JOIN field" "not a field of ClientWhereInput — the 1:N boundary enforced by the schema here"
gql "query { clients(where: {roles: {eq: \"billing-manager\"}}) { totalCount } }"
assert_gql_validation "roles"

case_ "V4.3 a filter over secretHash" "not a field of ClientWhereInput — the oracle is unreachable for the strongest reason available: the schema has no name for it, so the request cannot even be composed"
gql "query { clients(where: {secretHash: {eq: \"9f86\"}}) { totalCount } }"
assert_gql_validation "secretHash"

case_ "V4.4 an operator outside a leaf's allowlist" "status declares eq and in, so contains is a field its operator input does not define"
gql "query { clients(where: {status: {contains: \"act\"}}) { totalCount } }"
assert_gql_validation "contains"

case_ "V4.5 ?search= has no GraphQL twin either" "Unknown argument — the DTO declares none, so the schema grew none"
gql "query { clients(search: \"billing\") { totalCount } }"
assert_gql_validation "Unknown argument"

case_ "V4.6 selection is NATURAL here — there is no fields argument to gate" "Unknown argument: a caller shapes the response by selecting"
gql "query { clients(fields: \"name\") { totalCount } }"
assert_gql_validation "Unknown argument"

# ═════════════════════════════════════════════════════════════════════════════════════════
# V5 — the grace window's nullability IS the contract
# ═════════════════════════════════════════════════════════════════════════════════════════

gql "mutation(\$i: CreateClientInput!) { createClient(input: \$i) { id } }" \
    "$(jq -nc --arg t "$TEN_V" '{i:{name:"QA GraphQL Client Grace", description:"The client whose rotations prove zero and absent are different requests.", status:"active", tenantID:$t}}')"
ID_VG=$(printf '%s' "$HTTP_BODY" | jq -r '.data.createClient.id')

case_ "V5.1 the argument OMITTED means the usual day" "previousSecretExpiresAt non-null — the default window was applied"
gql "mutation(\$id: ID!, \$i: RotateClientSecretInput!) { rotateClientSecret(id: \$id, input: \$i) { previousSecretExpiresAt } }" \
    "$(jq -nc --arg id "$ID_VG" '{id:$id, i:{}}')"
assert_gql_ok '.data.rotateClientSecret.previousSecretExpiresAt != null' "true"

case_ "V5.2 the argument as ZERO means kill it now" "previousSecretExpiresAt null — nothing is retiring, because the retiring slot was cleared rather than stamped. A non-null Int in the schema would have made this request inexpressible"
gql "mutation(\$id: ID!, \$i: RotateClientSecretInput!) { rotateClientSecret(id: \$id, input: \$i) { previousSecretExpiresAt } }" \
    "$(jq -nc --arg id "$ID_VG" '{id:$id, i:{gracePeriodSeconds:0}}')"
assert_gql_ok '.data.rotateClientSecret.previousSecretExpiresAt' "null"

case_ "V5.3 an out-of-range window is the typed refusal" "200 + InvalidGracePeriodNotification — the bound travels to this surface unchanged"
gql "mutation(\$id: ID!, \$i: RotateClientSecretInput!) { rotateClientSecret(id: \$id, input: \$i) { id } }" \
    "$(jq -nc --arg id "$ID_VG" '{id:$id, i:{gracePeriodSeconds:604801}}')"
assert_gql InvalidGracePeriodNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# V6 — the field inventory, asserted from the SCHEMA rather than by calling each one
# ═════════════════════════════════════════════════════════════════════════════════════════

gql 'query { q: __type(name: "Query") { fields { name } } m: __type(name: "Mutation") { fields { name } } }'
GQL_TYPES="$HTTP_BODY"
gql_client_fields() { printf '%s' "$GQL_TYPES" | jq -r "[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test(\"$1\"))) | sort | join(\",\")"; }

case_ "V6.1 the five root fields are registered" "clients, client, createClient, patchClient, archiveClient"
GOT=$(gql_client_fields "^(clients|client|createClient|patchClient|archiveClient)$")
if [ "$GOT" = "archiveClient,client,clients,createClient,patchClient" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "root fields = $GOT"; fi

case_ "V6.2 the seven COLLECTION fields are registered" "add/archive per collection, plus patchClientClaim — the change verb only the claims collection mounts"
GOT=$(gql_client_fields "^(add|archive|patch)Client(Role|AllowedCIDR|Claim)$")
WANT="addClientAllowedCIDR,addClientClaim,addClientRole,archiveClientAllowedCIDR,archiveClientClaim,archiveClientRole,patchClientClaim"
if [ "$GOT" = "$WANT" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "collection fields = $GOT"; fi

case_ "V6.3 the ROTATION field is registered" "rotateClientSecret — mounted from its own file for the reason the REST route is: the generator owns the other mount"
GOT=$(gql_client_fields "^rotateClientSecret$")
if [ "$GOT" = "rotateClientSecret" ]; then pass_; else HTTP_BODY="$GQL_TYPES"; fail_ "rotation field = '$GOT'"; fi

case_ "V6.4 thirteen Client operations in total" "matching REST's thirteen exactly — full parity"
GOT=$(printf '%s' "$GQL_TYPES" | jq -r '[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test("[Cc]lient"))) | unique | length')
if [ "$GOT" = "13" ]; then pass_; else HTTP_BODY="$(printf '%s' "$GQL_TYPES" | jq -c '[(.data.q.fields[].name),(.data.m.fields[].name)] | map(select(test("[Cc]lient"))) | unique')"; fail_ "client fields = $GOT"; fi

qa_finish
