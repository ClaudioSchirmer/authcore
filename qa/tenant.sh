#!/usr/bin/env bash
# Lane: tenant — §1 of specs/qa/tenant-contract/plan.md.
#
# What the FRAMEWORK promises about this entity, on both wired surfaces. The
# business rules are qa/domain.sh; the refusals are qa/security.sh.
#
# Read backing is RELATIONAL (read.backing: relational), so read-your-writes is
# the promise: every read-back here is IMMEDIATE and a case that only passes
# after a retry is itself a failure. No poll, no drain, no CDC anywhere.

cd "$(dirname "$0")/.." || exit 2
LANE_NAME=tenant
. qa/lib.bash

# ── sign in ────────────────────────────────────────────────────────────────
# The lane needs a token holding tenant:*. It comes from this service's own
# documented flow; the security lane proves the gate itself.
sign_in "$BOOTSTRAP_EMAIL" "$BOOTSTRAP_INITIAL_PASSWORD"
if [ "$RESP_CODE" != "200" ]; then
  # A fresh database: the seed's password is still the initial one. If that
  # failed, try the rotated one — the runner may be re-running against a
  # database a previous lane already rotated.
  sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
fi
[ "$RESP_CODE" = "200" ] || { printf 'precondition: could not sign in (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }

USER_ID="$(j '.data.user.id')"
TOKEN="$(j '.data.accessToken')"
if [ "$(j '.data.user.mustChangePassword')" = "true" ]; then
  req PATCH "/users/${USER_ID}/password" \
    "$(jq -nc --arg c "$BOOTSTRAP_INITIAL_PASSWORD" --arg p "$QA_ADMIN_PASSWORD" \
       '{currentPassword:$c, password:$p, passwordConfirmation:$p}')"
  [ "$RESP_CODE" = "200" ] || [ "$RESP_CODE" = "204" ] || {
    printf 'precondition: password rotation failed (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }
  sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
  TOKEN="$(j '.data.accessToken')"
fi
[ -n "$TOKEN" ] || { printf 'precondition: no access token\n'; exit 2; }

# Every listing count in section G is scoped by this prefix rather than assuming
# an empty table. The reset gives a clean baseline; the prefix is what keeps the
# counts exact even so, and it is the same operator (?workspace.startswith=) the
# view declares — so the scoping is itself part of the contract under test.
SCOPE="qa-tenant-${QA_RUN_TAG}"
scoped() { printf 'workspace.startswith=%s' "$SCOPE"; }

# ════════════════════════════════════════════════════════════════════════════
section "B · happy path, one case per served verb (REST)"

W1="${SCOPE}-b1"
req POST /tenants "$(tenant_body 'Acme Comercio e Servicos' "$W1")"
assert_status "B1 insert" 201
assert_jq "B1 body mirrors the stored entity" '.data.workspace' "$W1"
ID1="$(j '.data.id')"

req GET "/tenants/${ID1}"
assert_status "B2 by-id" 200
assert_jq "B2 same workspace" '.data.workspace' "$W1"

req GET "/tenants?workspace=${W1}"
assert_status "B3 list filtered to this record" 200
assert_jq "B3 exactly one row" '.data | length' 1

req PATCH "/tenants/${ID1}" '{"name":"Acme Servicos Renomeado","description":"Updated description of the Acme retail operation."}'
assert_status "B4 patch name and description" 200
assert_jq "B4 new name" '.data.name' 'Acme Servicos Renomeado'
assert_jq "B4 workspace untouched" '.data.workspace' "$W1"

req GET "/tenants/${ID1}"
assert_jq "B4 read-back is IMMEDIATE (relational posture)" '.data.name' 'Acme Servicos Renomeado'

req PATCH "/tenants/${ID1}/archive" ''
assert_status "B5 archive" 204
req PATCH "/tenants/${ID1}/unarchive" ''
assert_status "B6 unarchive" 204

# Spec §7 rule 11: on insert ANY member is accepted, suspended included — rule 12
# gates transitions, not creation, and a data migration must be able to land a
# delinquent tenant in the state it was already in.
W_SUSP="${SCOPE}-b7"
req POST /tenants "$(tenant_body 'Delinquent Holdings' "$W_SUSP" 'Migrated tenant that arrived already suspended.' 'suspended')"
assert_status "B7 insert with status suspended is legal" 201
assert_jq "B7 status stored as sent" '.data.status' 'suspended'

# ════════════════════════════════════════════════════════════════════════════
section "C · golden record — every declared field, round-tripped on both surfaces"

W_G="${SCOPE}-golden"
G_NAME='Acme Comércio e Serviços Ltda'
G_DESC='Retail operations of the Acme group in Brazil, including logistics.'
req POST /tenants "$(tenant_body "$G_NAME" "$W_G" "$G_DESC" 'trial')"
assert_status "C0 golden insert" 201
GID="$(j '.data.id')"

req GET "/tenants/${GID}"
assert_jq "C1 by-id · name (accented runes survive)" '.data.name' "$G_NAME"
assert_jq "C1 by-id · workspace"   '.data.workspace' "$W_G"
assert_jq "C1 by-id · description" '.data.description' "$G_DESC"
assert_jq "C1 by-id · status"      '.data.status' 'trial'
assert_jq_true "C1 by-id · createdAt is RFC3339" '(.data.createdAt // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")' 'createdAt parses as RFC3339'
assert_jq_true "C1 by-id · updatedAt is RFC3339" '(.data.updatedAt // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")' 'updatedAt parses as RFC3339'
# deletedAt is deliberately NOT projected: archived state is reached through
# ?includeArchived, never through a timestamp on the wire.
assert_absent "C1 by-id · deletedAt is not projected" '.data.deletedAt'

req GET "/tenants?workspace=${W_G}"
assert_jq "C2 listing row · name"        '.data[0].name' "$G_NAME"
assert_jq "C2 listing row · workspace"   '.data[0].workspace' "$W_G"
assert_jq "C2 listing row · description" '.data[0].description' "$G_DESC"
assert_jq "C2 listing row · status"      '.data[0].status' 'trial'
assert_absent "C2 listing row · deletedAt is not projected" '.data[0].deletedAt'

gql "{ tenant(id: \"${GID}\") { id name workspace description status createdAt updatedAt } }"
assert_jq "C3 GraphQL node · name"        '.data.tenant.name' "$G_NAME"
assert_jq "C3 GraphQL node · workspace"   '.data.tenant.workspace' "$W_G"
assert_jq "C3 GraphQL node · description" '.data.tenant.description' "$G_DESC"
assert_jq "C3 GraphQL node · status"      '.data.tenant.status' 'trial'

# ════════════════════════════════════════════════════════════════════════════
section "D · validation 422 — one per rule shape, asserting the KEY"

req POST /tenants "$(tenant_body 'aaaa' "${SCOPE}-d1")"
assert_status_key "D1 name with a run of 4 identical runes" 422 'InvalidDisplayNameNotification'

# Positive control for distinct >= min(3, length): a flat "3 distinct" rule would
# reject 3M and GE, which are real display names.
req POST /tenants "$(tenant_body '3M' "${SCOPE}-d2")"
assert_status "D2 a two-rune name (3M) is accepted" 201

req POST /tenants "$(tenant_body 'Acme Corp' 'Acme Corp')"
assert_status_key "D3 workspace outside the DNS-label shape" 422 'InvalidTenantWorkspaceNotification'

req POST /tenants "$(tenant_body 'Acme Corp' "${SCOPE}-d4" 'short')"
assert_status_key "D4 description below the 15-rune floor" 422 'InvalidDescriptionNotification'

req POST /tenants "$(tenant_body 'Acme Corp' "${SCOPE}-d5" 'A perfectly ordinary description here.' 'frozen')"
assert_status_key "D5 status outside the closed set" 422 'UnknownTenantStatusNotification'

# Spec §7 rule 11: no separate required rule is declared — the enum's own unknown
# member notification is what answers an empty value.
# An EXPLICIT empty string, not an absent key — TenantStatusUnknown is "" and the
# spec says the enum's own unknown-member notification is what answers it.
req POST /tenants "$(tenant_body 'Acme Corp' "${SCOPE}-d6" 'A perfectly ordinary description here.' '')"
assert_status_key "D6 an empty status answers through the enum's unknown member" 422 'UnknownTenantStatusNotification'

# The vowel test is defined over Unicode, not [aeiou]: this service ships seven
# catalogs, and an ASCII-only test would refuse a description in another script
# AS KEYBOARD JUNK. Accented Latin must pass for the same reason.
req POST /tenants "$(tenant_body 'Açaí Comércio' "${SCOPE}-d7" 'Operações de varejo do grupo Açaí, incluindo logística.')"
assert_status "D7 accented Latin description is accepted" 201

# ════════════════════════════════════════════════════════════════════════════
section "E · 409 — the duplicate flavor"

W_DUP="${SCOPE}-e1"
req POST /tenants "$(tenant_body 'Acme Original' "$W_DUP")"
assert_status "E0 first holder of the handle" 201
req POST /tenants "$(tenant_body 'Acme Impostor' "$W_DUP")"
assert_status_key_field "E1 duplicate workspace" 409 'TenantWorkspaceAlreadyExistsNotification' 'workspace'
assert_jq "E1 semantic is Conflict, not StateConflict" '.errors[0].messages[0].semantic' 'Conflict'

# excludeSelf: true — an update must not collide with its own row.
req GET "/tenants?workspace=${W_DUP}"
E_ID="$(j '.data[0].id')"
req PATCH "/tenants/${E_ID}" '{"name":"Acme Original Renamed"}'
assert_status "E2 a patch does not self-collide on the unique handle" 200

# The wrong-state flavor (SemanticStateConflict) is N/A on this entity: it
# declares no state-conflict notification, no verb carries a revision
# precondition, and archive/unarchive misuse resolves as 404 (F5/F6).
skip "E3 wrong-state 409" "N/A — no revision precondition on this surface; misuse resolves as 404 (F5/F6)"

# ════════════════════════════════════════════════════════════════════════════
section "F · archive round-trip — kept-but-hidden (DeleteOnArchive not declared)"

W_A="${SCOPE}-f"
AID="$(create_tenant "$W_A" 'Acme To Be Archived')"
req PATCH "/tenants/${AID}/archive" ''
assert_status "F0 archive" 204

req GET "/tenants/${AID}"
assert_status_key "F1 archived row is hidden from by-id" 404 'RecordNotFoundNotification'
req GET "/tenants/${AID}?includeArchived=true"
assert_status "F2 ?includeArchived reveals it" 200

req GET "/tenants?workspace=${W_A}"
assert_jq "F3 listing hides the archived row" '.data | length' 0
req GET "/tenants?workspace=${W_A}&includeArchived=true"
assert_jq "F3 ?includeArchived reveals it in the listing" '.data | length' 1

req PATCH "/tenants/${AID}/unarchive" ''
assert_status "F4 unarchive" 204
req GET "/tenants/${AID}"
assert_status "F4 visible again" 200

# LoadForWrite filters archived rows; LoadArchivedForWrite is OnlyArchived. Both
# misuses therefore name no loadable record, which is a 404 rather than a 409.
req PATCH "/tenants/${AID}/archive" ''
assert_status "F5a archive the active row (setup)" 204
req PATCH "/tenants/${AID}/archive" ''
assert_status "F5 archiving an already-archived tenant" 404
req PATCH "/tenants/${AID}/unarchive" ''
assert_status "F6a restore (setup)" 204
req PATCH "/tenants/${AID}/unarchive" ''
assert_status "F6 unarchiving an active tenant" 404

skip "F7 child stamp-scoped unarchive" "N/A — flat aggregate, no child table"

# ════════════════════════════════════════════════════════════════════════════
section "G · read vocabulary and the pagination envelope"

# Five records under one prefix, so every count below is exact without assuming
# the table holds nothing else.
PG="${SCOPE}-pg"
for n in 1 2 3 4 5; do
  create_tenant "${PG}-${n}" "Paging Fixture ${n}" "Paging fixture number ${n} for the envelope cases." >/dev/null
done

req GET "/tenants?workspace=${PG}-1"
assert_jq "G1 eq on workspace" '.data | length' 1

req GET "/tenants?workspace.startswith=${PG}&status.in=active,trial"
assert_jq "G2 status.in over the fixture set" '.data | length' 5

req GET "/tenants?workspace.startswith=${PG}"
assert_jq "G3a workspace.startswith" '.data | length' 5
req GET "/tenants?workspace.istartswith=$(printf '%s' "$PG" | tr '[:lower:]' '[:upper:]')"
assert_jq "G3b workspace.istartswith is case-insensitive" '.data | length' 5

req GET "/tenants?workspace.startswith=${PG}&name.contains=Paging%20Fixture"
assert_jq "G4a name.contains" '.data | length' 5
req GET "/tenants?workspace.startswith=${PG}&name.icontains=paging%20fixture"
assert_jq "G4b name.icontains" '.data | length' 5
req GET "/tenants?workspace.startswith=${PG}&description.icontains=ENVELOPE"
assert_jq "G4c description.icontains" '.data | length' 5

YESTERDAY=$(date -u -v-1d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d 'yesterday' '+%Y-%m-%dT%H:%M:%SZ')
TOMORROW=$(date -u -v+1d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d 'tomorrow' '+%Y-%m-%dT%H:%M:%SZ')
req GET "/tenants?workspace.startswith=${PG}&createdAt.gte=${YESTERDAY}&createdAt.lte=${TOMORROW}"
assert_jq "G5 createdAt.gte + .lte bracket the fixtures" '.data | length' 5
req GET "/tenants?workspace.startswith=${PG}&updatedAt.gte=${YESTERDAY}"
assert_jq "G6 updatedAt filters though it is not orderable" '.data | length' 5

for ob in name -name workspace createdAt; do
  req GET "/tenants?workspace.startswith=${PG}&orderBy=${ob}"
  assert_status "G7 ?orderBy=${ob} (declared in the sort vocabulary)" 200
done
req GET "/tenants?workspace.startswith=${PG}&orderBy=-name"
assert_jq_true "G7 -name actually reverses the order" \
  '[.data[].name] as $n | $n == ($n | sort | reverse)' 'the rows come back in descending name order'

req GET "/tenants?workspace=${PG}-1&fields=name,workspace"
assert_jq   "G8 ?fields= keeps the asked-for keys" '.data[0].name' 'Paging Fixture 1'
assert_absent "G8 ?fields= drops description" '.data[0].description'

req GET "/tenants?workspace.startswith=${PG}&onlyTotal=true"
assert_jq     "G9 ?onlyTotal=true reports the count" '.pagination.totalCount' 5
assert_absent "G9 ?onlyTotal=true carries no data array" '.data'
assert_absent "G9 ?onlyTotal=true carries no cursors" '.pagination.endCursor'

req GET "/tenants?workspace.startswith=${PG}&orderBy=name&last=2"
assert_jq "G10 ?last= alone serves the TAIL window" '.data | length' 2
assert_jq_true "G10 the tail is the last two by name" \
  '[.data[].name] == ["Paging Fixture 4","Paging Fixture 5"]' 'the window is the tail, not the head'

# The envelope as a CONTRACT: cursors are window edges, and walking them must
# produce disjoint pages in both directions.
req GET "/tenants?workspace.startswith=${PG}&orderBy=name&first=2"
assert_jq "G11 totalCount counts the whole filtered set, not the page" '.pagination.totalCount' 5
assert_jq "G11 hasNextPage on page 1"     '.pagination.hasNextPage' 'true'
assert_jq "G11 hasPreviousPage on page 1" '.pagination.hasPreviousPage' 'false'
# The cursor contract is a BICONDITIONAL, not "both are always there":
# EndCursor is set exactly when HasNextPage, StartCursor exactly when
# HasPreviousPage (application/queries/view_reader.go:136). So page 1 of a
# forward walk carries NO startCursor, and asserting one would pin a promise the
# framework never made.
assert_jq_true "G11 endCursor is present exactly when hasNextPage" \
  '((.pagination.endCursor // "") != "") == (.pagination.hasNextPage == true)' \
  'endCursor tracks hasNextPage'
assert_jq_true "G11 startCursor is present exactly when hasPreviousPage" \
  '((.pagination.startCursor // "") != "") == (.pagination.hasPreviousPage == true)' \
  'startCursor tracks hasPreviousPage — page 1 of a forward walk carries none'
P1_NAMES="$(j '[.data[].name] | join(",")')"
END_CURSOR="$(j '.pagination.endCursor')"

req GET "/tenants?workspace.startswith=${PG}&orderBy=name&first=2&after=${END_CURSOR}"
assert_status "G11 page 2 via ?after=<endCursor>" 200
assert_jq "G11 page 2 reports a previous page" '.pagination.hasPreviousPage' 'true'
assert_jq_true "G11 page 2 therefore carries a startCursor" \
  '((.pagination.startCursor // "") != "") == (.pagination.hasPreviousPage == true)' \
  'the biconditional holds on page 2 too'
P2_NAMES="$(j '[.data[].name] | join(",")')"
if [ -n "$P2_NAMES" ] && [ "$P1_NAMES" != "$P2_NAMES" ] && \
   ! printf '%s' "$P2_NAMES" | grep -qF "${P1_NAMES%%,*}"; then
  pass "G11 page 2 is DISJOINT from page 1 — [$P1_NAMES] vs [$P2_NAMES]"
else
  fail "G11 page 2 disjointness" "a window not overlapping [$P1_NAMES]" "[$P2_NAMES]" "$RESP_BODY"
fi
P2_START="$(j '.pagination.startCursor')"
# Backward is last + before — the only legal pairing for a backward window.
req GET "/tenants?workspace.startswith=${PG}&orderBy=name&last=2&before=${P2_START}"
assert_status "G11 walking back with ?before=<startCursor of page 2>" 200
assert_jq "G11 the backward walk lands on page 1 again" '[.data[].name] | join(",")' "$P1_NAMES"

# ?includeArchived must raise the total by EXACTLY the number of archived rows —
# the count is read before and after, so the assertion is a delta rather than a
# number copied from somewhere.
req GET "/tenants?workspace.startswith=${PG}&onlyTotal=true"
BEFORE_TOTAL="$(j '.pagination.totalCount')"
req GET "/tenants?workspace=${PG}-5"
PG5_ID="$(j '.data[0].id')"
req PATCH "/tenants/${PG5_ID}/archive" ''
assert_status "G12 archive one of the five fixtures (setup)" 204
req GET "/tenants?workspace.startswith=${PG}&onlyTotal=true"
assert_jq "G12 the archived row leaves the default count" '.pagination.totalCount' "$((BEFORE_TOTAL - 1))"
req GET "/tenants?workspace.startswith=${PG}&onlyTotal=true&includeArchived=true"
assert_jq "G12 ?includeArchived restores it exactly" '.pagination.totalCount' "$BEFORE_TOTAL"

skip "G13 ?search=" "N/A as a capability case — the DTO never declares it, so the opt-in gate answers first (H3)"

# ════════════════════════════════════════════════════════════════════════════
section "H · rejected reads — the whole typed-400 guard family"

req GET "/tenants?bogus=1"
assert_status_key "H1 an unknown filter key" 400 'SchemaViolationNotification'
req GET "/tenants?workspace.contains=x"
assert_status_key "H2 an operator outside the field's allowlist" 400 'SchemaViolationNotification'
req GET "/tenants?search=acme"
assert_status_key "H3 a RESERVED control the Request DTO never declared" 400 'SchemaViolationNotification'
req GET "/tenants?fields=bogus"
assert_status_key "H4 an unresolvable ?fields= path" 400 'SchemaViolationNotification'
req GET "/tenants?orderBy=status"
assert_status_key "H5 ?orderBy= on a filterable-but-not-orderable field" 400 'SchemaViolationNotification'
req GET "/tenants?orderBy=updatedAt"
assert_status_key "H6 ?orderBy=updatedAt — filterable, deliberately not orderable" 400 'SchemaViolationNotification'
req GET "/tenants?orderBy=bogus"
assert_status_key "H7 ?orderBy= on an unknown token" 400 'SchemaViolationNotification'
req GET "/tenants?orderBy=-workspace"
assert_status "H8 ?orderBy=-workspace — positive control, desc IS declared" 200

req GET "/tenants?first=101"
assert_status_key "H9 ?first= above the view ceiling (100)" 400 'LimitExceededNotification'
req GET "/tenants?first=0"
assert_status_key "H10 ?first=0" 400 'SchemaViolationNotification'

req GET "/tenants?first=2&last=2"
assert_status_key "H11a first + last" 400 'SchemaViolationNotification'
req GET "/tenants?first=2&before=abc"
assert_status_key "H11b first + before" 400 'SchemaViolationNotification'
req GET "/tenants?last=2&after=abc"
assert_status_key "H11c last + after" 400 'SchemaViolationNotification'
req GET "/tenants?after=abc&before=def"
assert_status_key "H11d after + before" 400 'SchemaViolationNotification'

for conflict in 'first=10' 'orderBy=name' 'fields=name' 'after=abc'; do
  req GET "/tenants?onlyTotal=true&${conflict}"
  assert_status_key "H12 ?onlyTotal=true beside &${conflict}" 400 'SchemaViolationNotification'
done
# Filters and the archive control are NOT conflicts — counting a filtered subset
# is the entire point of onlyTotal.
req GET "/tenants?onlyTotal=true&workspace.startswith=${PG}"
assert_status "H13a ?onlyTotal=true + a filter is legal" 200
req GET "/tenants?onlyTotal=true&includeArchived=true"
assert_status "H13b ?onlyTotal=true + ?includeArchived is legal" 200

req GET "/tenants?after=not-a-cursor"
assert_status_key "H14 a malformed ?after=" 400 'SchemaViolationNotification'

req GET "/tenants?workspace.startswith=${PG}&first=1"
C_NOORDER="$(j '.pagination.endCursor')"
req GET "/tenants?workspace.startswith=${PG}&first=1&orderBy=name&after=${C_NOORDER}"
assert_status "H15 a cursor replayed under a different ?orderBy=" 400
req GET "/tenants?workspace.startswith=${PG}&first=1&includeArchived=true&after=${C_NOORDER}"
assert_status "H16 a cursor replayed under a different ?includeArchived" 400

req GET "/tenants?includeArchived=1"
assert_status_key "H17a a boolean control takes exactly true/false" 400 'SchemaViolationNotification'
req GET "/tenants?onlyTotal="
assert_status_key "H17b an empty boolean control" 400 'SchemaViolationNotification'

# The by-id DTO declares includeArchived and NOTHING else: presence is the gate.
req GET "/tenants/${ID1}?onlyTotal=false"
assert_status_key "H18a by-id gate — an undeclared control, present" 400 'SchemaViolationNotification'
req GET "/tenants/${ID1}?fields=name"
assert_status_key "H18b by-id gate — ?fields= is not declared here" 400 'SchemaViolationNotification'
req GET "/tenants/${ID1}?includeArchived=true"
assert_status "H19 by-id positive control for the one control it declares" 200

skip "H20 UnsupportedCapabilityNotification" "N/A for this entity — flat aggregate, no 1:N leg to push down, and ?search= is answered by the DTO gate first (H3)"

section "L · a filter value outside the leaf's declared kind (pin ≥ v0.70.0)"
req GET "/tenants?createdAt.gte=not-a-date"
assert_status_key "L1 a range operator on a temporal leaf" 400 'InvalidFilterValueNotification'
req GET "/tenants?createdAt=not-a-date"
assert_status_key "L2 equality on the same temporal leaf" 400 'InvalidFilterValueNotification'

# ════════════════════════════════════════════════════════════════════════════
section "I · routing, not-found, and the by-id ADDRESS contract"

req GET "/tenants/00000000-0000-7000-8000-000000000999"
assert_status_key "I1 a well-formed uuid naming no row" 404 'RecordNotFoundNotification'
req DELETE "/tenants/${ID1}"
assert_status_key "I2 DELETE — proves no hard-delete verb exists" 405 'MethodNotAllowedNotification'
req POST "/tenants/${ID1}" '{}'
assert_status_key "I3 POST on a path registered under PATCH/GET" 405 'MethodNotAllowedNotification'
req GET "/tenants/${ID1}/archive"
assert_status_key "I4 GET on a path registered under PATCH" 405 'MethodNotAllowedNotification'
req GET "/tenants/${ID1}/purge"
assert_status_key "I5 a path matching no registered route" 404 'RouteNotFoundNotification'

# The address contract splits by VERB, not by surface: a read names no record
# (404), a write states an intention about one (400).
req GET "/tenants/not-a-uuid"
assert_status_key "I6 READ a non-uuid address" 404 'UnknownIDAddressNotification'
req PATCH "/tenants/not-a-uuid" '{"name":"Whatever Name Here"}'
assert_status_key "I7 WRITE a non-uuid address (patch)" 400 'MalformedIDNotification'
req PATCH "/tenants/not-a-uuid/archive" ''
assert_status_key "I8 WRITE a non-uuid address (archive)" 400 'MalformedIDNotification'
req PATCH "/tenants/not-a-uuid/unarchive" ''
assert_status_key "I9 WRITE a non-uuid address (unarchive)" 400 'MalformedIDNotification'

gql '{ tenant(id: "not-a-uuid") { id } }'
assert_gql_key "I10 GraphQL READ a non-uuid address" 'UnknownIDAddressNotification'
gql 'mutation { archiveTenant(id: "not-a-uuid") { success } }'
assert_gql_key "I11 GraphQL WRITE a non-uuid address" 'MalformedIDNotification'

skip "I12 mode-missing-with-route-mounted 403" "N/A — every declared mode has its route mounted and no route is mounted for an undeclared mode; the reachable 403 is the permission gate (qa/security.sh)"

# ════════════════════════════════════════════════════════════════════════════
section "J · GraphQL — handler invariance"

gql "{ tenants(first: 2, where: { workspace: { startswith: \"${PG}\" } }, orderBy: [{ field: NAME, direction: ASC }]) { edges { cursor node { id name workspace status } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } totalCount } }"
assert_status "J1 tenants connection" 200
assert_jq_true "J1 the connection carries edges, pageInfo and totalCount" \
  '(.data.tenants.edges | length) > 0 and (.data.tenants.pageInfo != null) and (.data.tenants.totalCount != null)' \
  'edges + pageInfo + totalCount are all present'
GQL_FIRST_WS="$(j '.data.tenants.edges[0].node.workspace')"
req GET "/tenants?workspace=${GQL_FIRST_WS}"
assert_jq "J1 the GraphQL node equals the REST listing row" '.data[0].workspace' "$GQL_FIRST_WS"

gql "{ tenant(id: \"${GID}\") { id name workspace description status } }"
assert_jq "J2 tenant(id:) equals the REST by-id document" '.data.tenant.workspace' "$W_G"

W_J3="${SCOPE}-j3"
gql "mutation { createTenant(input: { name: \"Graph Created Tenant\", workspace: \"${W_J3}\", description: \"Created over GraphQL and read back over REST.\", status: \"active\" }) { id workspace } }"
assert_gql_ok "J3 createTenant" 
req GET "/tenants?workspace=${W_J3}"
assert_jq "J3 one write, two surfaces — visible over REST immediately" '.data[0].workspace' "$W_J3"
J3_ID="$(j '.data[0].id')"

gql "mutation { patchTenant(id: \"${J3_ID}\", input: { name: \"Graph Patched Tenant\" }) { id name } }"
assert_gql_ok "J4 patchTenant"
req GET "/tenants/${J3_ID}"
assert_jq "J4 the patch is visible over REST" '.data.name' 'Graph Patched Tenant'

gql "mutation { archiveTenant(id: \"${J3_ID}\") { success id } }"
assert_jq "J5a archiveTenant payload reports success" '.data.archiveTenant.success' 'true'
req GET "/tenants/${J3_ID}"
assert_status "J5a the archive is visible over REST" 404
gql "mutation { unarchiveTenant(id: \"${J3_ID}\") { success id } }"
assert_jq "J5b unarchiveTenant payload reports success" '.data.unarchiveTenant.success' 'true'
req GET "/tenants/${J3_ID}"
assert_status "J5b the restore is visible over REST" 200

gql '{ tenant(id: "00000000-0000-7000-8000-000000000999") { id } }'
assert_gql_key "J6 a missing record in the GraphQL idiom" 'RecordNotFoundNotification'

gql "mutation { createTenant(input: { name: \"Duplicate Attempt\", workspace: \"${W_DUP}\", description: \"An attempt to take a handle that is already held.\", status: \"active\" }) { id } }"
assert_gql_key "J7 duplicate handle over GraphQL" 'TenantWorkspaceAlreadyExistsNotification'
assert_jq "J7 the semantic travels with it" '.errors[0].extensions.semantic' 'Conflict'

# The DTO opt-in gate in the GraphQL idiom: the schema never advertises `search`,
# so this is an unknown ARGUMENT — a validation error, not the REST 400 envelope.
# Asserting the REST shape here would assert a promise this surface never made.
gql '{ tenants(search: "acme") { totalCount } }'
assert_gql_error_matching "J8 an undeclared control is an unknown argument, not a REST 400" 'search'

gql "{ tenants(where: { workspace: { startswith: \"${PG}\" } }, orderBy: [{ field: NAME, direction: DESC }]) { edges { node { name } } } }"
assert_jq_true "J9 where + orderBy agree with their REST twins" \
  '[.data.tenants.edges[].node.name] as $n | $n == ($n | sort | reverse)' 'the descending order matches REST'

# pin ≥ v0.72.1: __typename beside a selection must not change the answer. Every
# mainstream client appends it for cache normalization, so a boundary that only
# holds without it is not a boundary.
gql "{ tenant(id: \"${GID}\") { __typename id name workspace } }"
assert_jq "J10 __typename beside a selection answers identically" '.data.tenant.workspace' "$W_G"
assert_jq "J10 __typename itself resolves" '.data.tenant.__typename' 'Tenant'

skip "J11 gRPC parity" "N/A — no transport: block, so no gRPC surface is wired"
skip "J12 tabular export" "N/A — no exports declared on this entity"

# ════════════════════════════════════════════════════════════════════════════
section "X · suite meta"

req_noauth GET /openapi.json
assert_status "X1 /openapi.json is reachable" 200
for route in '/tenants' '/tenants/{id}' '/tenants/{id}/archive' '/tenants/{id}/unarchive'; do
  assert_jq_true "X1 openapi enumerates ${route}" \
    "(.paths | has(\"${route}\")) or (.paths | has(\"${route}/\"))" "the document declares ${route}"
done
assert_jq_true "X1 the six tenant operations are all advertised" \
  '[.paths | to_entries[] | select(.key | test("^/tenants")) | .value | keys[]] | length >= 6' \
  'openapi.json advertises at least the six mounted tenant operations'
# The permission each route declares is part of the contract, and openapi is the
# cheapest oracle for it — a source-vs-openapi disagreement is a FINDING.
assert_jq_true "X1 the listing route advertises tenant:read" \
  '[.paths | to_entries[] | select(.key | test("^/tenants/?$")) | .value.get.description] | join(" ") | test("tenant:read")' \
  'GET /tenants names tenant:read in its description'

lane_summary
