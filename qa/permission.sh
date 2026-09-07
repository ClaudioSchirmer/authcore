#!/usr/bin/env bash
# Lane: permission — the REST half of the framework contract for the Permission catalog.
#
# Families E1-E9 of specs/qa/permission-contract/plan.md §1. E10 and the GraphQL repeats live
# in qa/permission_graphql.sh; the business rules live in qa/domain.sh (P1-P10); the gate lives
# in qa/security.sh (S4.x); the audit trail lives in qa/audit.sh (A15+).
#
# THE ONE IDEA THIS WHOLE LANE IS BUILT AROUND — three columns go in, two values come out:
#
#   resource     stored · filterable · sortable · in NO response body, on any surface
#   action       stored · filterable · sortable · in NO response body, on any surface
#   description  stored · filterable · sortable · on the wire
#   permission   no column at all — computed as resource:action; on the wire, and neither
#                filterable nor orderable, BY CONSTRUCTION
#
# Filters and the ordering vocabulary are declared on the Request DTO and never consult the
# Response, which is what lets the two hidden parts stay queryable while leaving no value on
# the wire. Every case below is on one side of that sentence or the other, and says which.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init permission

# ═════════════════════════════════════════════════════════════════════════════════════════
# E1 — the five mounted verbs. Modes() declares display, insert, update, archive: FOUR modes,
#      FIVE routes, and no unarchive anywhere.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_MAIN=$(pair_resource main)
case_ "E1.1 POST /permissions" "201 — the record as stored, carrying the rendered pair"
api POST /permissions "$(permission_body "$R_MAIN" read "Read the main fixture resource of this QA run, for contract case E1.1.")"
assert_json_at 201 '.data.permission' "$R_MAIN:read"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "E1.2 GET /permissions/{id}" "200 — the full document"
api GET "/permissions/$ID_MAIN"
assert_json_at 200 '.data.permission' "$R_MAIN:read"

case_ "E1.3 GET /permissions" "200 — data + pagination"
api GET "/permissions?resource.eq=$R_MAIN"
assert_json_at 200 '.data | length' "1"

case_ "E1.4 PATCH /permissions/{id}" "200 — the record after the change"
api PATCH "/permissions/$ID_MAIN" '{"description":"Read the main fixture resource, with the wording an operator improved after reading it."}'
assert_json_at 200 '.data.description' "Read the main fixture resource, with the wording an operator improved after reading it."

case_ "E1.5 PATCH /permissions/{id}/archive" "204 with NO BODY — the framework's bodyless result"
ID_ARCH=$(new_permission "$(pair_resource arch)" read "Archive round-trip fixture for the permission contract lane.") || exit 1
api PATCH "/permissions/$ID_ARCH/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# E2 — the golden record, and its NEGATIVE half
#
# The positive half is ordinary. The negative half is the family nobody writes, and on this
# entity it is the entire point: a field that is stored and queryable must still reach no
# response body. `?fields=` is not involved in any of it — these are the DEFAULT shapes.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_GOLD=$(pair_resource gold)
case_ "E2.1 the write response carries exactly id + description + permission" "the three declared keys, no stamps"
api POST /permissions "$(permission_body "$R_GOLD" rotate-secret "Rotate the secret of the golden-record fixture, exercising a hyphenated action slug.")"
assert_json_at 201 '.data | keys | sort | join(",")' "description,id,permission"
ID_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "E2.2 the by-id read carries the managed stamps too" "id, description, permission, createdAt, updatedAt, archivedAt"
api GET "/permissions/$ID_GOLD"
assert_json_at 200 '.data | keys | sort | join(",")' "archivedAt,createdAt,description,id,permission,updatedAt"

case_ "E2.3 the pair renders as resource:action on the read side" "$R_GOLD:rotate-secret"
assert_json '.data.permission' "$R_GOLD:rotate-secret"

case_ "E2.4 archivedAt is null while the row is active" "null"
assert_json '.data.archivedAt' "null"

case_ "E2.5 createdAt and updatedAt are both stamped" "two non-null timestamps"
assert_json '[(.data.createdAt != null), (.data.updatedAt != null)] | @csv' "true,true"

case_ "E2.6 the listing row carries the same document" "the same pair, read through the collection"
api GET "/permissions?resource.eq=$R_GOLD"
assert_json_at 200 '.data[0].permission' "$R_GOLD:rotate-secret"

# ── the NEGATIVE half: the two stored parts reach no body ─────────────────────────────────
case_ "E2.7 resource is ABSENT from the write response" "the key is not present — not present-and-null"
api POST /permissions "$(permission_body "$(pair_resource neg)" read "A fixture proving the two stored halves never reach a response body on this surface.")"
assert_absent '.data | has("resource")'

case_ "E2.8 action is ABSENT from the write response" "the key is not present"
assert_absent '.data | has("action")'
ID_NEG=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "E2.9 resource and action are ABSENT from the by-id read" "neither key is present"
api GET "/permissions/$ID_NEG"
assert_json_at 200 '[(.data | has("resource")), (.data | has("action"))] | @csv' "false,false"

case_ "E2.10 resource and action are ABSENT from every listing row" "no row on the page carries either key"
api GET "/permissions?first=100"
assert_json_at 200 '[.data[] | (has("resource") or has("action"))] | any' "false"

case_ "E2.11 the PATCH response is lean in the same way" "id + description + permission, and nothing else"
api PATCH "/permissions/$ID_NEG" '{"description":"The wording of the negative-half fixture, revised so the patch response can be inspected."}'
assert_json_at 200 '.data | keys | sort | join(",")' "description,id,permission"

# ── and the composite's OWN name reaches no request body ──────────────────────────────────
case_ "E2.12 sending the rendered pair instead of the two halves" "422 RequiredFieldNotification — the write side speaks the PARTS, never the composite's name"
api POST /permissions '{"permission":"qa-rejected:read","description":"A body that sends the rendered pair the read side answers with, which no request carries."}'
assert_rest 422 RequiredFieldNotification

case_ "E2.13 that rejection names BOTH halves" "resource and action are each reported missing"
assert_json '[.errors[].messages[] | select(.notificationKey=="RequiredFieldNotification") | .field] | sort | join(",")' "action,resource"

# ═════════════════════════════════════════════════════════════════════════════════════════
# E3 — validation, 422, the notification KEY and the FIELD
#
# The field matters as much as the key here: a caller who reads only "malformed" cannot tell
# WHICH half to fix, and telling them is the whole reason the two notifications are distinct.
# ═════════════════════════════════════════════════════════════════════════════════════════

D_OK="A description long enough to satisfy the shared anti-junk floor of this service."

case_ "E3.1 an empty resource" "422 RequiredFieldNotification on 'resource'"
api POST /permissions "$(permission_body "" read "$D_OK")"
assert_rest_field 422 RequiredFieldNotification resource

case_ "E3.2 an empty action" "422 RequiredFieldNotification on 'action'"
api POST /permissions "$(permission_body tenant "" "$D_OK")"
assert_rest_field 422 RequiredFieldNotification action

case_ "E3.3 an uppercase resource" "422 InvalidResourceNameNotification — refused, never lowercased"
api POST /permissions "$(permission_body Tenant read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.4 a padded resource" "422 InvalidResourceNameNotification — refused, never trimmed"
api POST /permissions "$(permission_body " tenant " read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.5 a wildcard mixed into a slug" "422 InvalidResourceNameNotification — '*' is an ENTIRE part or nothing"
api POST /permissions "$(permission_body "ten*" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.6 a wildcard as a segment inside a path" "422 InvalidResourceNameNotification — 'user:*' is not one of the three shapes the matcher honours"
api POST /permissions "$(permission_body "user:*" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.7 a resource under the 2-rune floor" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body a read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.8 a resource with a run of 4 identical runes" "422 InvalidResourceNameNotification — the shared anti-junk predicate"
api POST /permissions "$(permission_body aaaa read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.9 a leading colon (an empty segment)" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body ":tenant" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.10 a trailing colon" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body "tenant:" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.11 a doubled colon" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body "user::profile" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.12 a resource over the 64-rune ceiling" "422 InvalidResourceNameNotification — the bound the claim budget is FOR"
api POST /permissions "$(permission_body "$(printf 'ab%.0s' $(seq 1 33))" read "$D_OK")"
assert_rest_field 422 InvalidResourceNameNotification resource

case_ "E3.13 an action carrying a colon" "422 InvalidActionNameNotification — the action is EXACTLY one segment, which is what makes the render parse back one way"
api POST /permissions "$(permission_body user "profile:read" "$D_OK")"
assert_rest_field 422 InvalidActionNameNotification action

case_ "E3.14 an uppercase action" "422 InvalidActionNameNotification"
api POST /permissions "$(permission_body tenant Read "$D_OK")"
assert_rest_field 422 InvalidActionNameNotification action

case_ "E3.15 a description under the 15-rune floor" "422 InvalidDescriptionNotification on 'description'"
api POST /permissions "$(permission_body "$(pair_resource shortdesc)" read "abc")"
assert_rest_field 422 InvalidDescriptionNotification description

case_ "E3.16 BOTH halves malformed in one call" "both keys in ONE answer — IsValid evaluates both parts before short-circuiting, on purpose"
api POST /permissions "$(permission_body "Tenant" "Read" "$D_OK")"
assert_json_at 422 '[.errors[].messages[].notificationKey] | sort | unique | join(",")' "InvalidActionNameNotification,InvalidResourceNameNotification"

# ── the positive boundaries: shapes that must be ACCEPTED ─────────────────────────────────
case_ "E3.17 a hierarchical resource" "201 — the framework's own example gates users:profile:read"
api POST /permissions "$(permission_body "$(pair_resource hier):profile" read "Read the profile facet of the hierarchical fixture resource, exercising a colon-joined path.")"
assert_status 201

case_ "E3.18 a resource leading with a digit" "201 — a product line named 3d-assets is ordinary"
api POST /permissions "$(permission_body "3d-assets-$(qa_slug_runid)" read "Read the digit-leading fixture resource, which the relaxed slug rule admits on purpose.")"
assert_status 201

case_ "E3.19 a hyphenated action slug" "201 — the vocabulary is OPEN: rotate-secret is as legal as read"
api POST /permissions "$(permission_body "$(pair_resource hyph)" rotate-secret "Rotate the credential of the hyphenated-action fixture, proving the action vocabulary is open.")"
assert_status 201

# ═════════════════════════════════════════════════════════════════════════════════════════
# E4 — the 409 family, and the two things this entity echoes back
#
# Both echo assertions pin `specs/evolve-entity/permission/spec.md` §5.1 and §5.2 — the field
# used to be `key` and the value used to be null. They exist to catch that drifting back.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_DUP=$(pair_resource dup)
ID_DUP=$(new_permission "$R_DUP" read "The first holder of the duplicate-pair fixture in the permission contract lane.") || exit 1

case_ "E4.1 a pair an ACTIVE row already holds" "409 PermissionAlreadyExistsNotification"
api POST /permissions "$(permission_body "$R_DUP" read "A second attempt at a pair the catalog already holds, which the pre-check refuses.")"
assert_rest 409 PermissionAlreadyExistsNotification

case_ "E4.2 the semantic is Conflict" "Conflict"
assert_json '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .semantic] | first' "Conflict"

case_ "E4.3 the 409 names 'permission' as its field" "field == permission — evolve-entity §5.1; it must never drift back to 'key'"
assert_json '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .field] | first' "permission"

case_ "E4.4 the 409 echoes the refused pair" "value == $R_DUP:read — evolve-entity §5.2; unique.echoValue rendering through PermissionKey.String(), where it used to be null"
assert_json '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | .value] | first' "$R_DUP:read"

case_ "E4.5 the SAME resource with a DIFFERENT action" "201 — the check is over the PAIR, not over either column"
api POST /permissions "$(permission_body "$R_DUP" insert "Insert into the duplicate-pair fixture resource, proving uniqueness is over the pair.")"
assert_status 201

case_ "E4.6 a DIFFERENT resource with the SAME action" "201 — the other half of the same statement"
api POST /permissions "$(permission_body "$(pair_resource dup2)" read "Read a second fixture resource, proving the action alone never collides.")"
assert_status 201

# ═════════════════════════════════════════════════════════════════════════════════════════
# E5 — archive, which on this entity is a ONE-WAY DOOR
#
# Everything up to `?includeArchived` is the ordinary round-trip. What follows it is what
# makes this catalog different from every other aggregate in the service: there is no way
# back through the row, only forward through a new one.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_LIFE=$(pair_resource life)
ID_LIFE=$(new_permission "$R_LIFE" read "The archive-lifecycle fixture of the permission contract lane, archived and then re-inserted.") || exit 1

case_ "E5.1 archive answers 204 with no body" "204, empty"
api PATCH "/permissions/$ID_LIFE/archive"
assert_empty_body 204

case_ "E5.2 the archived row is hidden from the by-id read" "404 RecordNotFoundNotification"
api GET "/permissions/$ID_LIFE"
assert_rest 404 RecordNotFoundNotification

case_ "E5.3 ?includeArchived reveals it" "200, with archivedAt stamped"
api GET "/permissions/$ID_LIFE?includeArchived=true"
assert_json_at 200 '(.data.archivedAt != null)' "true"

case_ "E5.4 the archived row is absent from the default listing" "0 rows"
api GET "/permissions?resource.eq=$R_LIFE"
assert_json_at 200 '.data | length' "0"

case_ "E5.5 the listing reveals it with ?includeArchived=true" "1 row"
api GET "/permissions?resource.eq=$R_LIFE&includeArchived=true"
assert_json_at 200 '.data | length' "1"

case_ "E5.6 there is no unarchive ROUTE at all" "404 RouteNotFoundNotification — no unarchive mode, no route mounted; the arm Tenant could not offer"
api PATCH "/permissions/$ID_LIFE/unarchive"
assert_rest 404 RouteNotFoundNotification

case_ "E5.7 the way back is a fresh insert of the same pair" "201 — the partial unique index is scoped to active rows"
api POST /permissions "$(permission_body "$R_LIFE" read "The re-inserted lifecycle fixture: a retired permission comes back as a NEW row that must be granted explicitly.")"
assert_status 201
ID_LIFE2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "E5.8 the reborn row carries a NEW id" "a different id — 'it came back with the same id' would mean the one-way door has a hinge"
if [ -n "$ID_LIFE2" ] && [ "$ID_LIFE2" != "$ID_LIFE" ]; then pass_; else fail_ "new id '$ID_LIFE2' vs archived id '$ID_LIFE'"; fi

case_ "E5.9 the archived remnant is still readable beside it" "2 rows with ?includeArchived — the retired one stays auditable"
api GET "/permissions?resource.eq=$R_LIFE&includeArchived=true"
assert_json_at 200 '.data | length' "2"

# ═════════════════════════════════════════════════════════════════════════════════════════
# E6 — the read vocabulary: everything the DTO declares, and only that
#
# Every filter and every orderBy token below addresses a column the caller can NEVER see in a
# response. That is the assertion, not a side effect of it.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_V=$(pair_resource voc)
new_permission "$R_V" read     "Read the read-vocabulary fixture resource of the permission contract lane." >/dev/null || exit 1
new_permission "$R_V" insert   "Insert into the read-vocabulary fixture resource of the permission contract lane." >/dev/null || exit 1
new_permission "$R_V" archive  "Archive rows of the read-vocabulary fixture resource of the permission contract lane." >/dev/null || exit 1

case_ "E6.1 ?resource.eq= — filtering a column no response carries" "the three fixture rows"
api GET "/permissions?resource.eq=$R_V"
assert_json_at 200 '.data | length' "3"

case_ "E6.2 ?resource.ne=" "the fixture rows are excluded from the rest of the catalog"
api GET "/permissions?resource.ne=$R_V&first=100"
assert_json_at 200 '[.data[] | select(.permission | startswith("'"$R_V"'"))] | length' "0"

case_ "E6.3 ?resource.startswith=" "the three rows — startswith is declared on resource and only on resource"
api GET "/permissions?resource.startswith=$R_V"
assert_json_at 200 '.data | length' "3"

case_ "E6.4 ?resource.contains=" "the three rows"
api GET "/permissions?resource.contains=$R_V"
assert_json_at 200 '.data | length' "3"

case_ "E6.5 ?action.eq= combined with the resource filter" "one row"
api GET "/permissions?resource.eq=$R_V&action.eq=insert"
assert_json_at 200 '.data[0].permission' "$R_V:insert"

case_ "E6.6 ?action.in= — a multi-value filter over a hidden column" "two of the three"
api GET "/permissions?resource.eq=$R_V&action.in=read,archive"
assert_json_at 200 '.data | length' "2"

case_ "E6.7 ?action.ne=" "two of the three"
api GET "/permissions?resource.eq=$R_V&action.ne=read"
assert_json_at 200 '.data | length' "2"

case_ "E6.8 ?action.contains=" "the row whose action contains 'chiv'"
api GET "/permissions?resource.eq=$R_V&action.contains=chiv"
assert_json_at 200 '.data[0].permission' "$R_V:archive"

case_ "E6.9 ?description.contains= — the one visible column's one operator" "the insert row"
api GET "/permissions?resource.eq=$R_V&description.contains=Insert%20into"
assert_json_at 200 '.data[0].permission' "$R_V:insert"

case_ "E6.10 ?createdAt.gte= over a bounded window" "the three rows are inside it"
api GET "/permissions?resource.eq=$R_V&createdAt.gte=2020-01-01T00:00:00Z"
assert_json_at 200 '.data | length' "3"

case_ "E6.11 ?createdAt.lte= in the past" "0 rows — the filter is applied, not ignored"
api GET "/permissions?resource.eq=$R_V&createdAt.lte=2020-01-01T00:00:00Z"
assert_json_at 200 '.data | length' "0"

case_ "E6.12 ?updatedAt.gte=" "the three rows"
api GET "/permissions?resource.eq=$R_V&updatedAt.gte=2020-01-01T00:00:00Z"
assert_json_at 200 '.data | length' "3"

# ── ordering: over the hidden parts and the visible one ───────────────────────────────────
case_ "E6.13 ?orderBy=action ascending" "archive, insert, read — ordering by a column no response carries"
api GET "/permissions?resource.eq=$R_V&orderBy=action"
assert_json_at 200 '[.data[].permission] | join(",")' "$R_V:archive,$R_V:insert,$R_V:read"

case_ "E6.14 ?orderBy=-action descending" "the same three, reversed"
api GET "/permissions?resource.eq=$R_V&orderBy=-action"
assert_json_at 200 '[.data[].permission] | join(",")' "$R_V:read,$R_V:insert,$R_V:archive"

case_ "E6.15 ?orderBy=resource — the index-backed sort" "200, the fixture rows"
api GET "/permissions?resource.eq=$R_V&orderBy=resource"
assert_json_at 200 '.data | length' "3"

case_ "E6.16 ?orderBy=description — an admitted blocking sort" "200, ordered by the visible column"
api GET "/permissions?resource.eq=$R_V&orderBy=description"
assert_json_at 200 '[.data[].description] | (. == (. | sort))' "true"

case_ "E6.17 ?orderBy=-description" "the reverse of E6.16"
api GET "/permissions?resource.eq=$R_V&orderBy=-description"
assert_json_at 200 '[.data[].description] | (. == (. | sort | reverse))' "true"

# ── ?fields=, where the projection pushes hidden sources down ─────────────────────────────
case_ "E6.18 ?fields=permission" "only the rendered pair — and its two SOURCES are pushed to the store while neither surfaces"
api GET "/permissions?resource.eq=$R_V&action.eq=read&fields=permission"
assert_json_at 200 '.data[0] | keys | join(",")' "permission"

case_ "E6.19 that projection still renders the derivation" "$R_V:read"
assert_json '.data[0].permission' "$R_V:read"

case_ "E6.20 ?fields=id,description" "exactly those two keys"
api GET "/permissions?resource.eq=$R_V&action.eq=read&fields=id,description"
assert_json_at 200 '.data[0] | keys | sort | join(",")' "description,id"

case_ "E6.21 a projection never smuggles the hidden parts back" "no row carries resource or action under ?fields=permission"
api GET "/permissions?resource.eq=$R_V&fields=permission"
assert_json_at 200 '[.data[] | (has("resource") or has("action"))] | any' "false"

# ── the reserved controls ─────────────────────────────────────────────────────────────────
case_ "E6.22 ?onlyTotal=true" "a count, and no data array at all"
api GET "/permissions?resource.eq=$R_V&onlyTotal=true"
assert_json_at 200 '[(.pagination.totalCount), (has("data"))] | @csv' "3,false"

case_ "E6.23 ?onlyTotal=true beside a filter" "counting a filtered subset is the canonical use"
api GET "/permissions?resource.eq=$R_V&action.eq=read&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "1"

case_ "E6.24 ?onlyTotal=false&first=10" "200 — present-but-inactive never trips the conflict matrix"
api GET "/permissions?resource.eq=$R_V&onlyTotal=false&first=10"
assert_status 200

case_ "E6.25 ?last=N alone serves the TAIL window" "the last row of the order, and hasNextPage false"
api GET "/permissions?resource.eq=$R_V&orderBy=action&last=1"
assert_json_at 200 '[.data[0].permission, (.pagination.hasNextPage | tostring)] | join(",")' "$R_V:read,false"

# ── the pagination envelope as a contract ─────────────────────────────────────────────────
case_ "E6.26 totalCount is truthful against a known set" "3"
api GET "/permissions?resource.eq=$R_V&orderBy=action&first=2"
assert_json_at 200 '.pagination.totalCount' "3"

case_ "E6.27 the head of a forward walk has a next page and no PREVIOUS one" "hasNextPage true, hasPreviousPage false"
assert_json '[(.pagination.hasNextPage | tostring), (.pagination.hasPreviousPage | tostring)] | join(",")' "true,false"

case_ "E6.28 an edge cursor is emitted only where its neighbour exists" "endCursor present (hasNextPage), startCursor absent (no previous page)"
assert_json '[(.pagination.endCursor != null), (.pagination.startCursor == null)] | @csv' "true,true"

CUR_END=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
PAGE1=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].permission] | join(",")')

case_ "E6.29 walking the endCursor into ?after= yields page 2" "the third row, disjoint from page 1"
api GET "/permissions?resource.eq=$R_V&orderBy=action&first=2&after=$CUR_END"
assert_json_at 200 '.data | length' "1"

case_ "E6.30 page 2 is disjoint from page 1" "no row repeats"
assert_json '[.data[].permission] | map(select(. == ("'"${PAGE1%%,*}"'"))) | length' "0"

case_ "E6.31 the tail of the walk has a previous page and no next one" "hasNextPage false, hasPreviousPage true"
assert_json '[(.pagination.hasNextPage | tostring), (.pagination.hasPreviousPage | tostring)] | join(",")' "false,true"

case_ "E6.32 and there its startCursor is emitted while endCursor is not" "the mirror of E6.28"
assert_json '[(.pagination.startCursor != null), (.pagination.endCursor == null)] | @csv' "true,true"

CUR_START=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.startCursor')

case_ "E6.33 backward paging with last + before returns the previous window" "the two rows of page 1, exactly"
api GET "/permissions?resource.eq=$R_V&orderBy=action&last=2&before=$CUR_START"
assert_json_at 200 '[.data[].permission] | join(",")' "$PAGE1"

case_ "E6.34 ?includeArchived=true on the listing widens the set" "the archived lifecycle row is reachable"
api GET "/permissions?resource.eq=$R_LIFE&includeArchived=true&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "2"

# ═════════════════════════════════════════════════════════════════════════════════════════
# E7 — rejected reads: the whole typed-400 guard family
#
# Two of these exist only on this entity, and they are the pair that proves the doctrine from
# the refusing side: ?fields=resource (stored, filterable, orderable, NOT selectable) and
# ?orderBy=permission / ?permission.eq= (on the wire, and neither filterable nor orderable).
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "E7.1 an unknown filter key" "400 SchemaViolationNotification — never a silently ignored parameter"
api GET "/permissions?bogus=x"
assert_rest 400 SchemaViolationNotification

case_ "E7.2 an operator outside the leaf's allowlist" "400 — description declares only 'contains'"
api GET "/permissions?description.eq=x"
assert_rest 400 SchemaViolationNotification

case_ "E7.3 an operator declared on ANOTHER leaf" "400 — startswith is on resource, not on action"
api GET "/permissions?action.startswith=r"
assert_rest 400 SchemaViolationNotification

case_ "E7.4 filtering the COMPUTED field" "400 — permission backs no column; the caller filters the two sources instead"
api GET "/permissions?permission.eq=tenant:read"
assert_rest 400 SchemaViolationNotification

case_ "E7.5 a bare eq on a leaf that declares no eq" "400 — description takes contains and nothing else"
api GET "/permissions?description=x"
assert_rest 400 SchemaViolationNotification

case_ "E7.6 ?search= on an endpoint whose DTO never declared it" "400 SchemaViolationNotification on 'search' — the DTO opt-in gate, reached BEFORE any engine"
api GET "/permissions?search=tenant"
assert_rest 400 SchemaViolationNotification

case_ "E7.7 an unresolvable ?fields= path" "400 SchemaViolationNotification on field fields[bogus]"
api GET "/permissions?fields=bogus"
assert_rest_field 400 SchemaViolationNotification "fields[bogus]"

case_ "E7.8 ?fields=resource — stored, filterable, orderable, and NOT selectable" "400 SchemaViolationNotification — the refusal that proves the doctrine from the other side"
api GET "/permissions?fields=resource"
assert_rest 400 SchemaViolationNotification

case_ "E7.9 ?fields=action" "400 SchemaViolationNotification — the same, on the other half"
api GET "/permissions?fields=action"
assert_rest 400 SchemaViolationNotification

case_ "E7.10 ?fields= on the by-id route, which declares only includeArchived" "400 SchemaViolationNotification on field 'fields'"
api GET "/permissions/$ID_MAIN?fields=id"
assert_rest_field 400 SchemaViolationNotification "fields"

case_ "E7.11 a filter value outside the leaf's kind" "400 InvalidFilterValueNotification — pin >= v0.70.0; below it this was a 500"
api GET "/permissions?createdAt.gte=lixo"
assert_rest 400 InvalidFilterValueNotification

case_ "E7.12 ?first= above the view's ceiling" "400 LimitExceededNotification — the framework default is 100 (no per-view MaxLimit, no query.maxLimit in the yaml)"
api GET "/permissions?first=101"
assert_rest 400 LimitExceededNotification

case_ "E7.13 the ceiling is reported as the VALUE" "100"
assert_json '[.errors[].messages[] | select(.notificationKey=="LimitExceededNotification") | .value] | first' "100"

case_ "E7.14 first and last together" "400 SchemaViolationNotification — mixed directions"
api GET "/permissions?first=5&last=5"
assert_rest 400 SchemaViolationNotification

case_ "E7.15 first with before" "400 — backward is last + before"
api GET "/permissions?first=5&before=$CUR_START"
assert_rest 400 SchemaViolationNotification

case_ "E7.16 after with before" "400 — the window has one anchor, not two"
api GET "/permissions?after=$CUR_END&before=$CUR_START"
assert_rest 400 SchemaViolationNotification

case_ "E7.17 ?onlyTotal=true&fields=id" "400 on field onlyTotal[fields] — the only-total conflict matrix"
api GET "/permissions?onlyTotal=true&fields=id"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[fields]"

case_ "E7.18 ?onlyTotal=true&orderBy=resource" "400 on field onlyTotal[orderBy]"
api GET "/permissions?onlyTotal=true&orderBy=resource"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[orderBy]"

case_ "E7.19 ?onlyTotal=true&first=10" "400 on field onlyTotal[first]"
api GET "/permissions?onlyTotal=true&first=10"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[first]"

case_ "E7.20 ?onlyTotal=true&after=<cursor>" "400 on field onlyTotal[after]"
api GET "/permissions?onlyTotal=true&after=$CUR_END"
assert_rest_field 400 SchemaViolationNotification "onlyTotal[after]"

case_ "E7.21 ?onlyTotal=1" "400 — the boolean controls take exactly true or false"
api GET "/permissions?onlyTotal=1"
assert_rest 400 SchemaViolationNotification

case_ "E7.22 ?onlyTotal= (present and empty)" "400 — PRESENCE is what the gate reads"
api GET "/permissions?onlyTotal="
assert_rest 400 SchemaViolationNotification

case_ "E7.23 ?includeArchived=1" "400 — same rule, other control"
api GET "/permissions?includeArchived=1"
assert_rest 400 SchemaViolationNotification

case_ "E7.24 a malformed cursor" "400 SchemaViolationNotification — checked before the reader runs"
api GET "/permissions?after=not-a-cursor"
assert_rest 400 SchemaViolationNotification

case_ "E7.25 a cursor replayed under a DIFFERENT order" "400 — a cursor is an edge of ONE ordering"
api GET "/permissions?resource.eq=$R_V&orderBy=description&after=$CUR_END"
assert_rest 400 SchemaViolationNotification

case_ "E7.26 a cursor replayed with a different archived gate" "400 — the window would silently change size"
api GET "/permissions?resource.eq=$R_V&orderBy=action&after=$CUR_END&includeArchived=true"
assert_rest 400 SchemaViolationNotification

case_ "E7.27 ?orderBy=permission — the computed path" "400 on field orderBy[permission] — a computed path backs no column"
api GET "/permissions?orderBy=permission"
assert_rest_field 400 SchemaViolationNotification "orderBy[permission]"

case_ "E7.28 ?orderBy=createdAt — filterable, deliberately not sortable" "400 on field orderBy[createdAt]"
api GET "/permissions?orderBy=createdAt"
assert_rest_field 400 SchemaViolationNotification "orderBy[createdAt]"

case_ "E7.29 ?orderBy=id — declarable, and deliberately not declared" "400 on field orderBy[id]"
api GET "/permissions?orderBy=id"
assert_rest_field 400 SchemaViolationNotification "orderBy[id]"

# ═════════════════════════════════════════════════════════════════════════════════════════
# E8 — addressing and absent verbs: the three-way split, complete for the first time
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "E8.1 DELETE /permissions/{id}" "405 MethodNotAllowedNotification — the path is registered, the method is not"
api DELETE "/permissions/$ID_MAIN"
assert_rest 405 MethodNotAllowedNotification

case_ "E8.2 PUT /permissions/{id}" "405 — update is PATCH-only: no sibling, nothing clearable"
api PUT "/permissions/$ID_MAIN" '{"description":"x"}'
assert_rest 405 MethodNotAllowedNotification

case_ "E8.3 POST /permissions/{id}" "405"
api POST "/permissions/$ID_MAIN" '{}'
assert_rest 405 MethodNotAllowedNotification

case_ "E8.4 GET /permissions/{id}/archive" "405 — the path exists for PATCH alone"
api GET "/permissions/$ID_MAIN/archive"
assert_rest 405 MethodNotAllowedNotification

case_ "E8.5 a path no route matches" "404 RouteNotFoundNotification — distinct from 'the record does not exist'"
api GET "/permissions-does-not-exist"
assert_rest 404 RouteNotFoundNotification

case_ "E8.6 an unknown but well-formed id" "404 RecordNotFoundNotification"
api GET "/permissions/0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"
assert_rest 404 RecordNotFoundNotification

case_ "E8.7 a non-uuid on a READ address" "404 UnknownIDAddressNotification — a read names no record"
api GET "/permissions/not-a-uuid"
assert_rest 404 UnknownIDAddressNotification

case_ "E8.8 a non-uuid on a WRITE address (patch)" "400 MalformedIDNotification — a write states an intention about one"
api PATCH "/permissions/not-a-uuid" '{"description":"A patch aimed at an address that is not an identifier at all."}'
assert_rest 400 MalformedIDNotification

case_ "E8.9 a non-uuid on the archive address" "400 MalformedIDNotification — the same verb split, on the other write"
api PATCH "/permissions/not-a-uuid/archive"
assert_rest 400 MalformedIDNotification

case_ "E8.10 archiving an ALREADY-archived row" "404 — LoadForWrite runs the default ScopeActive, so it never reaches a 409"
api PATCH "/permissions/$ID_ARCH/archive"
assert_rest 404 RecordNotFoundNotification

case_ "E8.11 patching an archived row" "404 — the same load scope"
api PATCH "/permissions/$ID_ARCH" '{"description":"A patch aimed at a row that has already left the active set."}'
assert_rest 404 RecordNotFoundNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# E9 — the pair is structurally absent from PATCH, proven by EFFECT
#
# patchExcludes: [Permission] means the two halves are not members of the partial body. PATCH
# is the LENIENT handler, so an unknown key is ignored rather than refused — which is why the
# only honest assertion here is about the EFFECT, and why the immutability notification is
# unreachable through any mounted surface. The suite does not assert a key no request can
# provoke.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_IMM=$(pair_resource imm)
ID_IMM=$(new_permission "$R_IMM" read "The immutability fixture: its pair must survive a patch that tries to move it.") || exit 1

case_ "E9.1 a PATCH carrying resource and action" "200 — the keys are not members of the body and are ignored, not refused"
api PATCH "/permissions/$ID_IMM" '{"description":"A patch that also tries to move the pair, which the request shape does not carry.","resource":"hijacked","action":"stolen"}'
assert_status 200

case_ "E9.2 the pair in that response is unchanged" "$R_IMM:read"
assert_json '.data.permission' "$R_IMM:read"

case_ "E9.3 and a read-back proves nothing was written" "$R_IMM:read"
api GET "/permissions/$ID_IMM"
assert_json_at 200 '.data.permission' "$R_IMM:read"

case_ "E9.4 the description DID change in the same call" "the editable field is genuinely editable — this is not a degenerate update"
assert_json '.data.description' "A patch that also tries to move the pair, which the request shape does not carry."

case_ "E9.5 no row was created under the hijacked resource" "0 rows"
api GET "/permissions?resource.eq=hijacked"
assert_json_at 200 '.data | length' "0"

qa_finish
