#!/usr/bin/env bash
# Lane: tenant — the REST half of the framework contract for the Tenant aggregate.
#
# Families F1–F9 of specs/qa/tenant-contract/plan.md §1. The business rules live in
# qa/domain.sh, the security boundary in qa/security.sh, the GraphQL twin in
# qa/tenant_graphql.sh, the audit trail in qa/audit.sh.
#
# Every expectation below was written and approved BEFORE the first request. Nothing here was
# read off a live answer, and a disagreement between this file and the running service is a
# finding about the SERVICE — never something to fix by editing a case.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init tenant

# ═════════════════════════════════════════════════════════════════════════════════════════
# F0 — the route inventory, cross-checked against the framework's own enumeration
#
# /openapi.json enumerates the routes actually wired, probes included. It says what EXISTS —
# never what should answer what — and a disagreement with internal/web/tenant_routes.go is a
# FINDING about the service, not something this suite reconciles silently.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "F0.1 /openapi.json wires exactly six tenant operations" "POST / · GET / · GET /{id} · PATCH /{id} · PATCH /{id}/archive · PATCH /{id}/unarchive — the six verbs Modes() declares, and no seventh"
api GET /openapi.json
assert_json_at 200 '[.paths | to_entries[] | select(.key | startswith("/tenants")) | .value | keys[]] | length' "6"

case_ "F0.2 there is no DELETE operation on any tenant path" "0 — removal is archive-only: purging a tenant would orphan live tokens and release a handle that must never be reused"
assert_json '[.paths | to_entries[] | select(.key | startswith("/tenants")) | .value | keys[] | select(. == "delete")] | length' "0"

case_ "F0.3 every tenant operation advertises its required permission" "6 of 6 — the suffix is emitted only while the runtime gate is actually enforcing, so its presence is itself a posture assertion"
assert_json '[.paths | to_entries[] | select(.key | startswith("/tenants")) | .value | to_entries[] | select(.value.description // "" | test("Required permission"))] | length' "6"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F1 — happy path, one per served verb
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_MAIN=$(ws main)
BODY_MAIN=$(tenant_body "Acme Comercio e Servicos" "$WS_MAIN" "Retail operations of the Acme group in Brazil." "active")

case_ "F1.1 POST /tenants creates a tenant" "201, and the body is the record AS STORED"
api POST /tenants "$BODY_MAIN"
assert_json_at 201 '.data.workspace' "$WS_MAIN"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "F1.2 GET /tenants/{id} reads it back" "200 with the same workspace — relational backing, read-your-writes, no poll"
api GET "/tenants/$ID_MAIN"
assert_json_at 200 '.data.workspace' "$WS_MAIN"

case_ "F1.3 GET /tenants lists it" "200, envelope carries data[] and pagination"
api GET "/tenants?name=Acme%20Comercio%20e%20Servicos"
assert_json_at 200 '(.data | type)' "array"

case_ "F1.4 PATCH /tenants/{id} updates it" "200 with the new name"
api PATCH "/tenants/$ID_MAIN" '{"name":"Acme Comercio Renamed"}'
assert_json_at 200 '.data.name' "Acme Comercio Renamed"

case_ "F1.5 PATCH /tenants/{id}/archive" "204 with NO body — a bodyless verb"
api PATCH "/tenants/$ID_MAIN/archive"
assert_empty_body 204

case_ "F1.6 PATCH /tenants/{id}/unarchive" "204 with NO body"
api PATCH "/tenants/$ID_MAIN/unarchive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# F2 — golden record: every declared field written, then read back field by field
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_GOLD=$(ws gold)
G_NAME="Golden Record Tenant"
G_DESC="Every declared field of the tenant aggregate exercised in one record."
api POST /tenants "$(tenant_body "$G_NAME" "$WS_GOLD" "$G_DESC" "trial")"
ID_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "F2.1 by-id returns name" "the value as written"
api GET "/tenants/$ID_GOLD"; assert_json_at 200 '.data.name' "$G_NAME"
case_ "F2.2 by-id returns workspace" "the value as written"
assert_json '.data.workspace' "$WS_GOLD"
case_ "F2.3 by-id returns description" "the value as written"
assert_json '.data.description' "$G_DESC"
case_ "F2.4 by-id returns status" "trial, exactly as inserted — no server-side default"
assert_json '.data.status' "trial"
case_ "F2.5 by-id returns the id" "the id the insert answered with"
assert_json '.data.id' "$ID_GOLD"
case_ "F2.6 by-id returns createdAt" "a non-null managed stamp"
assert_json '(.data.createdAt != null)' "true"
case_ "F2.7 by-id returns updatedAt" "a non-null managed stamp"
assert_json '(.data.updatedAt != null)' "true"
case_ "F2.8 by-id returns archivedAt as null while active" "null — the row is not archived"
assert_json '.data.archivedAt' "null"

case_ "F2.9 the LISTING row carries the same eight fields" "every declared field present on a listing row too"
api GET "/tenants?workspace=$WS_GOLD"
assert_json_at 200 '[.data[0] | .id,.name,.workspace,.description,.status,.createdAt,.updatedAt] | map(.!=null) | all' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F3 — validation, 422, notification KEY asserted (never prose)
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "F3.1 empty name" "422 RequiredFieldNotification — the raw VO answers an empty value itself"
api POST /tenants "$(tenant_body "" "$(ws v)" "A perfectly ordinary description of a tenant." "active")"
assert_rest 422 RequiredFieldNotification

case_ "F3.2 name is keyboard junk" "422 InvalidDisplayNameNotification — a run of 4 identical runes"
api POST /tenants "$(tenant_body "aaaa" "$(ws v)" "A perfectly ordinary description of a tenant." "active")"
assert_rest 422 InvalidDisplayNameNotification

case_ "F3.3 workspace outside the alphabet" "422 InvalidTenantWorkspaceNotification — underscores are not DNS labels"
api POST /tenants "$(tenant_body "Valid Name" "acme_corp" "A perfectly ordinary description of a tenant." "active")"
assert_rest 422 InvalidTenantWorkspaceNotification

case_ "F3.4 workspace is reserved" "422 ReservedTenantWorkspaceNotification — 'admin' is well-formed and taken by the platform"
api POST /tenants "$(tenant_body "Valid Name" "admin" "A perfectly ordinary description of a tenant." "active")"
assert_rest 422 ReservedTenantWorkspaceNotification

case_ "F3.5 workspace 'id' is BOTH malformed and reserved" "one 422 carrying InvalidTenantWorkspaceNotification..."
api POST /tenants "$(tenant_body "Valid Name" "id" "A perfectly ordinary description of a tenant." "active")"
assert_rest 422 InvalidTenantWorkspaceNotification
case_ "F3.6 ...and ReservedTenantWorkspaceNotification in the same envelope" "the VO evaluates both branches — two different problems, two different fixes"
assert_rest 422 ReservedTenantWorkspaceNotification

case_ "F3.7 description under the floor" "422 InvalidDescriptionNotification — 15 runes minimum"
api POST /tenants "$(tenant_body "Valid Name" "$(ws v)" "abc" "active")"
assert_rest 422 InvalidDescriptionNotification

case_ "F3.8 status outside the closed set" "422 UnknownTenantStatusNotification — trial | active | suspended, and nothing else"
api POST /tenants "$(tenant_body "Valid Name" "$(ws v)" "A perfectly ordinary description of a tenant." "paused")"
assert_rest 422 UnknownTenantStatusNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# F4 — the 409 family (the duplicate flavor; the wrong-state flavor is N/A, see the plan)
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_DUP=$(ws dup)
api POST /tenants "$(tenant_body "Duplicate Holder" "$WS_DUP" "The tenant that holds the handle a later insert will collide with." "active")"
ID_DUP=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "F4.1 the handle of an ACTIVE tenant" "409 TenantWorkspaceAlreadyExistsNotification"
api POST /tenants "$(tenant_body "Collider One" "$WS_DUP" "A second tenant reaching for a handle that is already held." "active")"
assert_rest 409 TenantWorkspaceAlreadyExistsNotification

case_ "F4.2 the envelope's semantic is the DUPLICATE flavor" "semantic 'Conflict', not 'StateConflict'"
assert_json '[.errors[].messages[] | select(.notificationKey=="TenantWorkspaceAlreadyExistsNotification") | .semantic] | first' "Conflict"

api PATCH "/tenants/$ID_DUP/archive"
case_ "F4.3 the handle of an ARCHIVED tenant — the case nobody writes" "409, the SAME key: activeOnly is false on purpose, an archived remnant keeps blocking the handle forever"
api POST /tenants "$(tenant_body "Collider Two" "$WS_DUP" "A tenant reaching for the handle an archived remnant still holds." "active")"
assert_rest 409 TenantWorkspaceAlreadyExistsNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# F5 — archive round-trip
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_ARC=$(ws arc)
api POST /tenants "$(tenant_body "Archive Round Trip" "$WS_ARC" "The tenant that walks the whole archive and unarchive round trip." "active")"
ID_ARC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "F5.1 archive answers 204" "204, no body"
api PATCH "/tenants/$ID_ARC/archive"
assert_empty_body 204

case_ "F5.2 an archived tenant is hidden from the by-id read" "404 RecordNotFoundNotification — the default load scope is active-only"
api GET "/tenants/$ID_ARC"
assert_rest 404 RecordNotFoundNotification

case_ "F5.3 ?includeArchived=true reveals it" "200 — archiving is the only removal there is, so archived rows must stay reachable"
api GET "/tenants/$ID_ARC?includeArchived=true"
assert_json_at 200 '.data.workspace' "$WS_ARC"

case_ "F5.4 the archived row carries an archivedAt stamp" "a non-null timestamp — the caller who asked to see archived tenants has no other way to tell which is which"
assert_json '(.data.archivedAt != null)' "true"

case_ "F5.5 the archived row reads back SUSPENDED" "status 'suspended' — archive forces it (§1b row 4)"
assert_json '.data.status' "suspended"

case_ "F5.6 the listing hides it" "the row is absent from a plain listing"
api GET "/tenants?workspace=$WS_ARC"
assert_json_at 200 '(.data // []) | length' "0"

case_ "F5.7 the listing reveals it under ?includeArchived=true" "exactly one row"
api GET "/tenants?workspace=$WS_ARC&includeArchived=true"
assert_json_at 200 '.data | length' "1"

case_ "F5.8 unarchive answers 204" "204, no body"
api PATCH "/tenants/$ID_ARC/unarchive"
assert_empty_body 204

case_ "F5.9 the row is visible again" "200 on the plain by-id read"
api GET "/tenants/$ID_ARC"
assert_json_at 200 '.data.workspace' "$WS_ARC"

case_ "F5.10 archivedAt is cleared" "null — the same UPDATE statement, with an explicit null bound"
assert_json '.data.archivedAt' "null"

case_ "F5.11 the tenant comes back SUSPENDED, not active" "'suspended' — unarchiving restores the row, never the pre-archive commercial state (§1b row 5)"
assert_json '.data.status' "suspended"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F6 — read vocabulary. A deterministic five-row set, isolated by a run-scoped name prefix.
# ═════════════════════════════════════════════════════════════════════════════════════════

PFX="QAPage${QA_RUN_ID:-local}"
PAGE_IDS=()
for n in 1 2 3 4 5; do
  api POST /tenants "$(tenant_body "${PFX}0${n}" "$(ws p)" "Paging fixture number ${n} of the tenant read vocabulary suite." "active")"
  PAGE_IDS+=("$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')")
done

case_ "F6.1 filter eq (no operator suffix)" "exactly the one row whose name matches"
api GET "/tenants?name=${PFX}03"
assert_json_at 200 '.data | length' "1"

case_ "F6.2 filter in" "two rows"
api GET "/tenants?name.in=${PFX}01,${PFX}02"
assert_json_at 200 '.data | length' "2"

case_ "F6.3 filter startswith" "the whole five-row set"
api GET "/tenants?name.startswith=${PFX}"
assert_json_at 200 '.data | length' "5"

case_ "F6.4 filter contains" "the whole set — 'Page' is a substring of every name"
api GET "/tenants?name.contains=${PFX}&first=50"
assert_json_at 200 '.data | length' "5"

case_ "F6.5 filter istartswith (case-folded)" "the same five rows from a lowercased prefix"
api GET "/tenants?name.istartswith=$(printf '%s' "$PFX" | tr 'A-Z' 'a-z')"
assert_json_at 200 '.data | length' "5"

case_ "F6.6 filter icontains (case-folded)" "the same five rows"
api GET "/tenants?name.icontains=$(printf '%s' "$PFX" | tr 'A-Z' 'a-z')"
assert_json_at 200 '.data | length' "5"

case_ "F6.7 workspace startswith" "at least the five paging fixtures"
api GET "/tenants?workspace.startswith=qa-p-${QA_RUN_ID:-local}&first=50"
assert_json_at 200 '(.data | length) >= 5' "true"

case_ "F6.8 description icontains" "the five paging fixtures share one description shape"
api GET "/tenants?description.icontains=paging%20fixture%20number&first=50"
assert_json_at 200 '.data | length' "5"

case_ "F6.9 status eq" "every row of the set is active"
api GET "/tenants?name.startswith=${PFX}&status=active"
assert_json_at 200 '.data | length' "5"

case_ "F6.10 status in" "the same five under a list operator"
api GET "/tenants?name.startswith=${PFX}&status.in=active,trial"
assert_json_at 200 '.data | length' "5"

case_ "F6.11 createdAt gte a past instant" "the set is newer than 2020"
api GET "/tenants?name.startswith=${PFX}&createdAt.gte=2020-01-01T00:00:00Z"
assert_json_at 200 '.data | length' "5"

case_ "F6.12 createdAt lt a past instant" "nothing was created before 2020"
api GET "/tenants?name.startswith=${PFX}&createdAt.lt=2020-01-01T00:00:00Z"
assert_json_at 200 '(.data // []) | length' "0"

case_ "F6.13 updatedAt gte a past instant" "updatedAt FILTERS even though it does not sort"
api GET "/tenants?name.startswith=${PFX}&updatedAt.gte=2020-01-01T00:00:00Z"
assert_json_at 200 '.data | length' "5"

case_ "F6.14 orderBy name ascending" "01 first"
api GET "/tenants?name.startswith=${PFX}&orderBy=name"
assert_json_at 200 '.data[0].name' "${PFX}01"

case_ "F6.15 orderBy name descending" "05 first"
api GET "/tenants?name.startswith=${PFX}&orderBy=-name"
assert_json_at 200 '.data[0].name' "${PFX}05"

case_ "F6.16 orderBy workspace" "200 — workspace is one of the three declared sortable paths"
api GET "/tenants?name.startswith=${PFX}&orderBy=workspace"
assert_status 200

case_ "F6.17 orderBy createdAt" "200 — the third declared sortable path"
api GET "/tenants?name.startswith=${PFX}&orderBy=createdAt"
assert_status 200

case_ "F6.18 ?fields= projects exactly what was named" "id and workspace present, name absent (not null — absent)"
api GET "/tenants?name.startswith=${PFX}&fields=id,workspace&orderBy=name"
assert_json_at 200 '[.data[0] | has("id"), has("workspace"), has("name")] | @csv' 'true,true,false'

case_ "F6.19 ?onlyTotal=true short-circuits into a count" "pagination.totalCount only — no data, no cursors, no has*Page"
api GET "/tenants?name.startswith=${PFX}&onlyTotal=true"
assert_json_at 200 '[.pagination.totalCount, (has("data")), (.pagination | has("hasNextPage"))] | @csv' '5,false,false'

case_ "F6.20 ?onlyTotal=true beside a filter" "counting a filtered subset is the canonical use — 1"
api GET "/tenants?name=${PFX}02&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "1"

case_ "F6.21 pagination.totalCount is truthful on a paged read" "5, even though the window holds 2"
api GET "/tenants?name.startswith=${PFX}&orderBy=name&first=2"
assert_json_at 200 '.pagination.totalCount' "5"

case_ "F6.22 the window honours ?first=" "2 rows"
assert_json '.data | length' "2"

case_ "F6.23 hasNextPage is true at the head of the set" "true — three rows lie beyond this edge"
assert_json '.pagination.hasNextPage' "true"

case_ "F6.24 hasPreviousPage is false at the head" "false — nothing precedes the first page"
assert_json '.pagination.hasPreviousPage' "false"

case_ "F6.25 the head page carries endCursor and NO startCursor" "an edge cursor is emitted only where its neighbouring page exists — endCursor exactly when hasNextPage, startCursor exactly when hasPreviousPage; an absent edge means 'nothing to walk to on this side'"
assert_json '[(.pagination | has("startCursor")), ((.pagination.endCursor | length) > 0)] | @csv' 'false,true'

CUR_END=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
P1_NAMES=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].name] | join(",")')

case_ "F6.26 echoing endCursor into ?after= walks forward" "the next two rows: 03 and 04"
api GET "/tenants?name.startswith=${PFX}&orderBy=name&first=2&after=$(printf '%s' "$CUR_END" | jq -sRr @uri)"
assert_json_at 200 '[.data[].name] | join(",")' "${PFX}03,${PFX}04"

case_ "F6.27 page 2 is disjoint from page 1" "no name appears on both pages"
P2_NAMES=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].name] | join(",")')
if [ -n "$P2_NAMES" ] && [ "$P1_NAMES" != "$P2_NAMES" ]; then pass_; else fail_ "page1=[$P1_NAMES] page2=[$P2_NAMES]"; fi

case_ "F6.28 hasPreviousPage is true on page 2" "true — page 1 lies behind this edge"
assert_json '.pagination.hasPreviousPage' "true"

CUR_START2=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.startCursor')

case_ "F6.29 ?last= + ?before= walks BACKWARD" "back to page 1: 01 and 02"
api GET "/tenants?name.startswith=${PFX}&orderBy=name&last=2&before=$(printf '%s' "$CUR_START2" | jq -sRr @uri)"
assert_json_at 200 '[.data[].name] | join(",")' "${PFX}01,${PFX}02"

case_ "F6.30 ?last=N alone serves the TAIL window" "the last two of the set: 04 and 05, with hasNextPage false"
api GET "/tenants?name.startswith=${PFX}&orderBy=name&last=2"
assert_json_at 200 '[([.data[].name] | join(",")), (.pagination.hasNextPage | tostring)] | @csv' "\"${PFX}04,${PFX}05\",\"false\""

# ═════════════════════════════════════════════════════════════════════════════════════════
# F7 — rejected reads: the whole typed-400 guard family
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "F7.1 an unknown filter key" "400 SchemaViolationNotification — never a silently ignored parameter"
api GET "/tenants?bogus=x"
assert_rest 400 SchemaViolationNotification

case_ "F7.2 an operator outside the leaf's allowlist" "400 — Status declares eq,in and nothing else"
api GET "/tenants?status.contains=act"
assert_rest 400 SchemaViolationNotification

case_ "F7.3 bare eq on a leaf that does not declare it" "400 — Description declares contains,icontains only, and eq is the empty operator"
api GET "/tenants?description=x"
assert_rest 400 SchemaViolationNotification

case_ "F7.4 ?search= on an endpoint whose DTO never declared it" "400 SchemaViolationNotification on 'search' — the DTO opt-in gate, reached BEFORE any engine"
api GET "/tenants?search=acme"
assert_rest 400 SchemaViolationNotification

case_ "F7.5 an unresolvable ?fields= path" "400 SchemaViolationNotification on field fields[bogus]"
api GET "/tenants?fields=bogus"
assert_rest_field 400 SchemaViolationNotification "fields[bogus]"

case_ "F7.6 ?fields= on the BY-ID route, which declares only includeArchived" "400 — presence gates, per endpoint"
api GET "/tenants/$ID_GOLD?fields=id"
assert_rest 400 SchemaViolationNotification

case_ "F7.7 a filter value outside the leaf's kind" "400 InvalidFilterValueNotification — pin >= v0.70.0; below it this was a 500"
api GET "/tenants?createdAt=lixo"
assert_rest 400 InvalidFilterValueNotification

case_ "F7.8 ?first= above the view's ceiling" "400 LimitExceededNotification — the framework default is 100 (no per-view MaxLimit, no query.maxLimit in the yaml)"
api GET "/tenants?first=101"
assert_rest 400 LimitExceededNotification

case_ "F7.9 the ceiling value travels on the envelope" "value '100' — what the client needs in order to retry correctly"
assert_json '[.errors[].messages[] | select(.notificationKey=="LimitExceededNotification") | .value] | first' "100"

case_ "F7.10 mixed directions: first + last" "400 — one direction at a time"
api GET "/tenants?first=5&last=5"
assert_rest 400 SchemaViolationNotification

case_ "F7.11 mixed directions: first + before" "400 — backward is last+before"
api GET "/tenants?first=5&before=abc"
assert_rest 400 SchemaViolationNotification

case_ "F7.12 mixed directions: after + before" "400"
api GET "/tenants?after=abc&before=def"
assert_rest 400 SchemaViolationNotification

case_ "F7.13 onlyTotal beside ?fields=" "400 on field onlyTotal[fields]"
api GET "/tenants?onlyTotal=true&fields=id"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[fields]"

case_ "F7.14 onlyTotal beside ?orderBy=" "400 on field onlyTotal[orderBy]"
api GET "/tenants?onlyTotal=true&orderBy=name"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[orderBy]"

case_ "F7.15 onlyTotal beside ?first=" "400 on field onlyTotal[first]"
api GET "/tenants?onlyTotal=true&first=10"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[first]"

case_ "F7.16 onlyTotal beside ?after=" "400 on field onlyTotal[after]"
api GET "/tenants?onlyTotal=true&after=abc"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[after]"

case_ "F7.17 ?onlyTotal=1" "400 — the boolean controls take exactly true or false"
api GET "/tenants?onlyTotal=1"
assert_rest 400 SchemaViolationNotification

case_ "F7.18 ?onlyTotal= with no value" "400 — PRESENCE is the key being on the query string, not the value being non-empty"
api GET "/tenants?onlyTotal="
assert_rest 400 SchemaViolationNotification

case_ "F7.19 ?includeArchived=1" "400 — same strictness on the other boolean"
api GET "/tenants?includeArchived=1"
assert_rest 400 SchemaViolationNotification

case_ "F7.20 ?onlyTotal=false beside ?first=" "200 — present-but-inactive never trips the conflict matrix"
api GET "/tenants?onlyTotal=false&first=2"
assert_status 200

case_ "F7.21 ?onlyTotal=true beside ?includeArchived=true" "200 — counting archived rows too is legitimate"
api GET "/tenants?onlyTotal=true&includeArchived=true"
assert_json_at 200 '(.pagination.totalCount >= 1)' "true"

case_ "F7.22 a malformed cursor" "400 SchemaViolationNotification — checked before the reader runs"
api GET "/tenants?first=2&after=not-a-cursor"
assert_rest 400 SchemaViolationNotification

case_ "F7.23 cursor vs orderBy mismatch" "400 — a cursor's key tuple belongs to the ordering it was minted under"
api GET "/tenants?name.startswith=${PFX}&first=2&after=$(printf '%s' "$CUR_END" | jq -sRr @uri)&orderBy=workspace"
assert_rest 400 SchemaViolationNotification

case_ "F7.24 cursor vs includeArchived mismatch" "400 — the archived gate is part of the window the cursor addresses"
api GET "/tenants?name.startswith=${PFX}&first=2&after=$(printf '%s' "$CUR_END" | jq -sRr @uri)&includeArchived=true"
assert_rest 400 SchemaViolationNotification

case_ "F7.25 ?orderBy= on a filterable-but-not-sortable field" "400 on field orderBy[status] — an unindexed sort is a blocking sort"
api GET "/tenants?orderBy=status"
assert_rest_field 400 SchemaViolationNotification "orderBy[status]"

case_ "F7.26 ?orderBy=-updatedAt — the spec's own deliberate cut" "400 on field orderBy[-updatedAt]: updatedAt filters, and is deliberately NOT orderable"
api GET "/tenants?orderBy=-updatedAt"
assert_rest_field 400 SchemaViolationNotification "orderBy[-updatedAt]"

# ═════════════════════════════════════════════════════════════════════════════════════════
# F8 — addressing and absent verbs
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "F8.1 DELETE /tenants/{id}" "405 MethodNotAllowedNotification — no DELETE verb: purging a tenant would orphan live tokens and release a handle that must never be reused"
api DELETE "/tenants/$ID_GOLD"
assert_rest 405 MethodNotAllowedNotification

case_ "F8.2 PUT /tenants/{id}" "405 — the update shape is patch, so no full replace is mounted"
api PUT "/tenants/$ID_GOLD" '{"name":"x"}'
assert_rest 405 MethodNotAllowedNotification

case_ "F8.3 POST /tenants/{id}" "405 — the path is registered, the method is not"
api POST "/tenants/$ID_GOLD" '{}'
assert_rest 405 MethodNotAllowedNotification

case_ "F8.4 GET /tenants/{id}/archive" "405 — archive is a PATCH intent, never a read"
api GET "/tenants/$ID_GOLD/archive"
assert_rest 405 MethodNotAllowedNotification

case_ "F8.5 a path no route matches" "404 RouteNotFoundNotification — distinct from 'the record does not exist'"
api GET "/does-not-exist"
assert_rest 404 RouteNotFoundNotification

case_ "F8.6 a well-formed id that matches no row" "404 RecordNotFoundNotification"
api GET "/tenants/019903c2-6b41-7c9e-9f2a-6d3b1e77aaaa"
assert_rest 404 RecordNotFoundNotification

case_ "F8.7 a non-uuid on a READ address" "404 UnknownIDAddressNotification — a read names no record"
api GET "/tenants/not-a-uuid"
assert_rest 404 UnknownIDAddressNotification

case_ "F8.8 a non-uuid on a WRITE address (patch)" "400 MalformedIDNotification — a write states an intention about one"
api PATCH "/tenants/not-a-uuid" '{"name":"x"}'
assert_rest 400 MalformedIDNotification

case_ "F8.9 a non-uuid on a WRITE address (archive)" "400 MalformedIDNotification"
api PATCH "/tenants/not-a-uuid/archive"
assert_rest 400 MalformedIDNotification

case_ "F8.10 a non-uuid on a WRITE address (unarchive)" "400 MalformedIDNotification"
api PATCH "/tenants/not-a-uuid/unarchive"
assert_rest 400 MalformedIDNotification

case_ "F8.11 unarchive on an ACTIVE tenant" "404 RecordNotFoundNotification — the unarchive load runs OnlyArchived"
api PATCH "/tenants/$ID_GOLD/unarchive"
assert_rest 404 RecordNotFoundNotification

WS_ST=$(ws state)
api POST /tenants "$(tenant_body "State Guard Tenant" "$WS_ST" "The tenant used to prove what a write against an archived row answers." "active")"
ID_ST=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
api PATCH "/tenants/$ID_ST/archive"

case_ "F8.12 archive on an ALREADY-archived tenant" "404 RecordNotFoundNotification — the write load runs the default active scope; this is where a 409 'not active' would have landed if the framework guarded it later"
api PATCH "/tenants/$ID_ST/archive"
assert_rest 404 RecordNotFoundNotification

case_ "F8.13 PATCH on an archived tenant" "404 RecordNotFoundNotification — same scope, same answer"
api PATCH "/tenants/$ID_ST" '{"name":"Renamed While Archived"}'
assert_rest 404 RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# F9 — workspace immutability, proven by EFFECT
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "F9.1 a PATCH body carrying 'workspace' is accepted" "200 — PATCH is the lenient handler, so an unknown key is ignored rather than refused"
api PATCH "/tenants/$ID_GOLD" "$(jq -nc --arg w "hijacked-handle" '{name:"Golden Record Tenant", workspace:$w}')"
assert_status 200

case_ "F9.2 and the handle did NOT change" "the original workspace — the field is structurally absent from the update body (patchExcludes), so the immutability rule needs no request to reach it"
api GET "/tenants/$ID_GOLD"
assert_json_at 200 '.data.workspace' "$WS_GOLD"

qa_finish
