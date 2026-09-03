#!/usr/bin/env bash
# Lane: permission — §1 of specs/qa/permission-contract/plan.md.
#
# What the FRAMEWORK promises about this entity, on both wired surfaces. The
# business rules are qa/domain.sh; the refusals are qa/security.sh.
#
# The shape that makes this entity different from tenant: THREE COLUMNS GO IN,
# TWO VALUES COME OUT. `resource` and `action` are hidden — stored, filterable,
# sortable, writable, and in NO response body on ANY surface — while the read
# side answers the computed `permission`, their rendering. Half the cases below
# exist to prove both halves of that: the parts reach the store, and they never
# reach the wire.
#
# Read backing is RELATIONAL, so read-your-writes is the promise: every read-back
# here is IMMEDIATE and a case that only passes after a retry is itself a
# failure. No poll, no drain, no CDC anywhere.

cd "$(dirname "$0")/.." || exit 2
LANE_NAME=permission
. qa/lib.bash

# ── sign in ────────────────────────────────────────────────────────────────
sign_in "$BOOTSTRAP_EMAIL" "$BOOTSTRAP_INITIAL_PASSWORD"
[ "$RESP_CODE" = "200" ] || sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
[ "$RESP_CODE" = "200" ] || { printf 'precondition: could not sign in (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }
USER_ID="$(j '.data.user.id')"; TOKEN="$(j '.data.accessToken')"
if [ "$(j '.data.user.mustChangePassword')" = "true" ]; then
  req PATCH "/users/${USER_ID}/password" \
    "$(jq -nc --arg c "$BOOTSTRAP_INITIAL_PASSWORD" --arg p "$QA_ADMIN_PASSWORD" \
       '{currentPassword:$c, password:$p, passwordConfirmation:$p}')"
  [ "$RESP_CODE" = "200" ] || [ "$RESP_CODE" = "204" ] || {
    printf 'precondition: password rotation failed (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }
  sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"; TOKEN="$(j '.data.accessToken')"
fi
[ -n "$TOKEN" ] || { printf 'precondition: no access token\n'; exit 2; }

# Every listing count below is scoped by this prefix rather than assuming an
# empty table — and on THIS entity the table is never empty: migration 0012
# seeds the catalog before any case runs.
PG="qa-$(_slug "$LANE_NAME")-$(_slug "${QA_RUN_TAG:-$$}")"

# ════════════════════════════════════════════════════════════════════════════
section "G12/G13 · the seeded baseline — FIRST, before this lane writes anything"
note "migrations/postgres/0012_bootstrap_seed_manual.up.sql seeds 39 rows: the 38"
note "resource:action pairs the routes enforce, plus the *:* wildcard."

# This case MUST run before the lane inserts anything, which is why it opens the
# file rather than sitting with the rest of section G. The number comes from the
# tracked migration, never from the answer.
req GET '/permissions?onlyTotal=true'
assert_jq "G12 a freshly migrated catalog holds exactly the 39 seeded rows" '.pagination.totalCount' 39

# The seed's own header promises this list and the RequirePermission calls move
# together, and nothing enforces it at boot: a route added with a permission
# nobody seeded is invisible until a real caller is refused. This is the case
# that sees it. The literals come from the document's own
# "**Required permission:** `<p>`" suffix, which the framework renders for every
# gated route (web/openapi/spec.go:appendPermissionSuffix).
req GET /openapi.json
DECLARED="$(j '[ .paths | to_entries[] | .value | to_entries[] | .value | objects | .description? // empty ]
               | map(scan("\\*\\*Required permission:\\*\\* `([^`]+)`")) | flatten | unique | .[]')"
if [ -z "$DECLARED" ]; then
  fail "G13 the document declares at least one gated route" \
       "one or more '**Required permission:**' suffixes" "none found" "$RESP_BODY"
else
  MISSING=''
  for lit in $DECLARED; do
    RES="${lit%%:*}"; ACT="${lit##*:}"
    req GET "/permissions?resource.eq=${RES}&action.eq=${ACT}&onlyTotal=true"
    [ "$(j '.pagination.totalCount')" -ge 1 ] 2>/dev/null || MISSING="${MISSING} ${lit}"
  done
  if [ -z "$MISSING" ]; then
    pass "G13 every permission a route declares has an active catalog row ($(printf '%s\n' $DECLARED | wc -l | tr -d ' ') literals)"
  else
    fail "G13 every permission a route declares has an active catalog row" \
         "a catalog row for each declared literal" "no row for:${MISSING}" ''
  fi
fi

# ════════════════════════════════════════════════════════════════════════════
section "B · happy path — one per served verb"

R_B="${PG}-b1"
# req POST directly rather than create_permission: the helper runs in a subshell
# under command substitution, so RESP_CODE would not reach the assertion below.
req POST /permissions "$(permission_body "$R_B" read)"
assert_status "B1 insert" 201
ID_B="$(j '.data.id')"
assert_jq "B1 the response renders the pair as one string" '.data.permission' "${R_B}:read"
# The whole point of `hidden`: the two halves went to the store and neither came
# back. A leak here is silent and every other case in this file passes over it.
assert_absent "B1 the request's resource does not come back" '.data.resource'
assert_absent "B1 the request's action does not come back"   '.data.action'

req GET "/permissions/${ID_B}"
assert_status "B2 by-id" 200
assert_jq "B2 by-id renders the pair" '.data.permission' "${R_B}:read"

req GET "/permissions?resource.eq=${R_B}"
assert_status "B3 list filtered to the new row" 200
assert_jq "B3 exactly one row answers that resource" '.data | length' 1

req PATCH "/permissions/${ID_B}" '{"description":"Amended wording that explains what holding this entry allows."}'
assert_status "B4 patch the only editable field" 200
assert_jq "B4 the description changed" '.data.description' 'Amended wording that explains what holding this entry allows.'
assert_jq "B4 the pair did NOT move" '.data.permission' "${R_B}:read"

req PATCH "/permissions/${ID_B}/archive" ''
assert_status "B5 archive" 204

# The absent verb. Permission declares no unarchive mode AND mounts no route, so
# this lands on the 404 arm of the three-way split — the arm the tenant round
# could not reach, because tenant HAS the verb.
req PATCH "/permissions/${ID_B}/unarchive" ''
assert_status_key "B6a no unarchive route exists on REST" 404 'RouteNotFoundNotification'
gql "mutation { unarchivePermission(id: \"${ID_B}\") { success } }"
assert_gql_error_matching "B6b no unarchivePermission field exists in the schema" 'unarchivePermission'

# ════════════════════════════════════════════════════════════════════════════
section "C · golden record — every declared field, and the hidden parts nowhere"

R_G="${PG}-golden"
G_DESC='Open the registry of golden records and read one entry by its identifier.'
req POST /permissions "$(permission_body "$R_G" 'rotate-secret' "$G_DESC")"
assert_status "C0 golden insert" 201
GID="$(j '.data.id')"

req GET "/permissions/${GID}"
assert_jq "C1 by-id · permission renders as resource:action" '.data.permission' "${R_G}:rotate-secret"
assert_jq "C1 by-id · description" '.data.description' "$G_DESC"
assert_jq_true "C1 by-id · createdAt is RFC3339" '(.data.createdAt // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")' 'createdAt parses as RFC3339'
assert_jq_true "C1 by-id · updatedAt is RFC3339" '(.data.updatedAt // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")' 'updatedAt parses as RFC3339'
assert_absent "C1 by-id · deletedAt is not projected" '.data.deletedAt'

req GET "/permissions?resource.eq=${R_G}"
assert_jq "C2 listing row · permission" '.data[0].permission' "${R_G}:rotate-secret"
assert_jq "C2 listing row · description" '.data[0].description' "$G_DESC"
assert_absent "C2 listing row · deletedAt is not projected" '.data[0].deletedAt'

# THE case nobody writes: the hidden parts must appear in NO body, on NO surface,
# under NO projection. Each of these is a separate door the value could leak
# through.
assert_absent "C3 listing row · resource never leaves the application layer" '.data[0].resource'
assert_absent "C3 listing row · action never leaves the application layer"   '.data[0].action'
req GET "/permissions?resource.eq=${R_G}&fields=permission"
assert_absent "C3 under ?fields=permission · resource is still absent" '.data[0].resource'
assert_absent "C3 under ?fields=permission · action is still absent"   '.data[0].action'
req GET "/permissions?resource.eq=${R_G}&includeArchived=true"
assert_absent "C3 under ?includeArchived · resource is still absent" '.data[0].resource'

gql "{ permission(id: \"${GID}\") { id description permission createdAt updatedAt } }"
assert_jq "C4 GraphQL node · permission" '.data.permission.permission' "${R_G}:rotate-secret"
assert_jq "C4 GraphQL node · description" '.data.permission.description' "$G_DESC"

# ════════════════════════════════════════════════════════════════════════════
section "D · validation 422 — one per rule shape, asserting the KEY and the FIELD"

# D1/D2 assert the FRAMEWORK's required-field notification, not the VO's own:
# validateResource/validateAction short-circuit on empty (permission_key.go:74,
# :96), so a case asserting InvalidResourceNameNotification here would be
# asserting a branch the code cannot take.
req POST /permissions "$(permission_body '' read)"
assert_status_key_field "D1 an empty resource" 422 'RequiredFieldNotification' 'resource'
req POST /permissions "$(permission_body tenant '')"
assert_status_key_field "D2 an empty action" 422 'RequiredFieldNotification' 'action'

# The resource rules — one case per shape the segment rule refuses.
for pair in \
  'Tenant|D3 an uppercase resource is refused, never repaired' \
  ' tenant |D4 a padded resource is refused, never trimmed' \
  'a|D5 a resource below the 2-rune segment floor' \
  'tenant:|D6 a trailing colon leaves an empty segment' \
  'ten--ant|D7 a doubled hyphen' \
  'aaaab|D8 a run of four identical runes' \
  'user:*|D9 the wildcard inside a path, not as a whole part' \
  'ten*|D10 the wildcard mixed into a slug'
do
  VAL="${pair%%|*}"; NAME="${pair#*|}"
  req POST /permissions "$(permission_body "$VAL" read)"
  assert_status_key_field "$NAME" 422 'InvalidResourceNameNotification' 'resource'
done

req POST /permissions "$(permission_body "$(printf 'ab%.0s' $(seq 1 33))" read)"
assert_status_key_field "D11 a resource past the 64-rune part ceiling" 422 'InvalidResourceNameNotification' 'resource'

req POST /permissions "$(permission_body tenant 'read:write')"
assert_status_key_field "D12 an action carrying a colon — the action is ONE segment" 422 'InvalidActionNameNotification' 'action'
req POST /permissions "$(permission_body tenant 'Read')"
assert_status_key_field "D13 an uppercase action" 422 'InvalidActionNameNotification' 'action'

req POST /permissions "$(permission_body "${PG}-d14" read 'short')"
assert_status_key_field "D14 a description below the VO's floor" 422 'InvalidDescriptionNotification' 'description'

# Positive controls. Without these the whole section would also pass for a
# service that refuses everything.
req POST /permissions "$(permission_body "${PG}-d15:profile" read)"
assert_status "D15 a colon-joined resource PATH is accepted" 201
req POST /permissions "$(permission_body "${PG}-d16" 'rotate-secret')"
assert_status "D16 a hyphenated action slug is accepted" 201
req POST /permissions "$(permission_body "${PG}-d17" read 'Permitir a leitura do catálogo de permissões da plataforma inteira.')"
assert_status "D17 an accented description is accepted — the VO tests are Unicode" 201

# ════════════════════════════════════════════════════════════════════════════
section "E · 409 — uniqueness over the TUPLE, not over either column"

R_E="${PG}-e1"
create_permission "$R_E" read >/dev/null
assert_status "E1 setup · the pair is taken" 201
req POST /permissions "$(permission_body "$R_E" read)"
assert_status_key_field "E1 the same active pair twice" 409 'PermissionAlreadyExistsNotification' 'permission'
assert_jq "E1 semantic" '.errors[0].messages[0].semantic' 'Conflict'
# echoValue: true — without it the answer says a pair is taken and never says
# which, and the value is rendered through PermissionKey.String().
assert_jq "E1 the refused pair is echoed back, rendered" '.errors[0].messages[0].value' "${R_E}:read"

req POST /permissions "$(permission_body "$R_E" insert)"
assert_status "E2 the SAME resource with a different action is legal" 201

# excludeSelf: an update must not collide with itself.
req GET "/permissions?resource.eq=${R_E}&action.eq=read"
E3_ID="$(j '.data[0].id')"
req PATCH "/permissions/${E3_ID}" '{"description":"A patch that leaves the pair exactly where it was."}'
assert_status "E3 a patch that does not move the pair does not self-collide" 200

# The seeded wildcard row already holds *:*. This proves the seed and the
# constraint in one call.
req POST /permissions "$(permission_body '*' '*' 'An attempt to take the wildcard the seed already holds.')"
assert_status_key "E4 the seeded *:* row is already taken" 409 'PermissionAlreadyExistsNotification'

skip "E5 SemanticStateConflict" "no state-conflict notification is declared on this entity and no verb carries a revision precondition; archive misuse resolves as 404 through LoadForWrite. Asserting one would assert a promise this entity does not make."

# ════════════════════════════════════════════════════════════════════════════
section "F · archive round-trip — kept-but-hidden, and NO way back"

R_F="${PG}-f1"
ID_F="$(create_permission "$R_F" archive)"
req PATCH "/permissions/${ID_F}/archive" ''
assert_status "F0 archive" 204

req GET "/permissions/${ID_F}"
assert_status_key "F1 an archived row is invisible to by-id" 404 'RecordNotFoundNotification'
req GET "/permissions/${ID_F}?includeArchived=true"
assert_status "F2 ?includeArchived reveals it to by-id" 200
req GET "/permissions?resource.eq=${R_F}"
assert_jq "F3a the listing hides it" '.data | length' 0
req GET "/permissions?resource.eq=${R_F}&includeArchived=true"
assert_jq "F3b ?includeArchived reveals it to the listing" '.data | length' 1

req PATCH "/permissions/${ID_F}/archive" ''
assert_status_key "F4 archiving an already-archived row" 404 'RecordNotFoundNotification'

# unique.scope: active-only — the exact inverse of tenant's scope: all. With no
# unarchive verb this is the ONLY route back, which is what makes it load-bearing
# rather than a convenience.
req POST /permissions "$(permission_body "$R_F" archive)"
assert_status "F5a the archived pair can be taken again" 201
ID_F2="$(j '.data.id')"
[ -n "$ID_F2" ] && [ "$ID_F2" != "$ID_F" ] \
  && pass "F5b it comes back as a NEW row with a new id" \
  || fail "F5b it comes back as a NEW row with a new id" "an id different from ${ID_F}" "${ID_F2:-<none>}" "$RESP_BODY"
req GET "/permissions?resource.eq=${R_F}&includeArchived=true"
assert_jq "F5c the retired row is still there as history" '.data | length' 2

skip "F6 child stamp-scoped unarchive" "flat aggregate — no child table, and no unarchive verb to scope a stamp with."

# ════════════════════════════════════════════════════════════════════════════
section "G · read vocabulary, the computed field, and the pagination envelope"

# A known, countable set of the lane's own rows.
G_PFX="${PG}-page"   # not "-g": that is a prefix of "-golden" and would scope it in
for n in 1 2 3 4; do
  req POST /permissions "$(permission_body "${G_PFX}-${n}" read "Seeded row number ${n} for the paging and ordering cases.")"
done
req GET "/permissions?resource.startswith=${G_PFX}&onlyTotal=true"
assert_jq "G0 the paging fixture is countable" '.pagination.totalCount' 4

req GET "/permissions?resource.eq=${G_PFX}-1"
assert_jq "G1a ?resource.eq=" '.data | length' 1
req GET "/permissions?resource.ne=${G_PFX}-1&resource.startswith=${G_PFX}"
assert_jq "G1b ?resource.ne=" '.data | length' 3
req GET "/permissions?resource.in=${G_PFX}-1,${G_PFX}-2"
assert_jq "G1c ?resource.in=" '.data | length' 2
req GET "/permissions?resource.contains=${G_PFX}"
assert_status "G1d ?resource.contains=" 200
req GET "/permissions?resource.startswith=${G_PFX}"
assert_jq "G1e ?resource.startswith=" '.data | length' 4

req GET "/permissions?resource.startswith=${G_PFX}&action.eq=read"
assert_jq "G2a ?action.eq=" '.data | length' 4
req GET "/permissions?resource.startswith=${G_PFX}&action.ne=read"
assert_jq "G2b ?action.ne=" '.data | length' 0
req GET "/permissions?resource.startswith=${G_PFX}&action.in=read,insert"
assert_jq "G2c ?action.in=" '.data | length' 4
req GET "/permissions?resource.startswith=${G_PFX}&action.contains=ea"
assert_jq "G2d ?action.contains=" '.data | length' 4

req GET "/permissions?resource.startswith=${G_PFX}&description.contains=paging"
assert_status "G3 ?description.contains= — the only operator it declares" 200

req GET "/permissions?resource.startswith=${G_PFX}&createdAt.gte=2000-01-01T00:00:00Z&createdAt.lte=2999-01-01T00:00:00Z"
assert_jq "G4a ?createdAt.gte= + .lte=" '.data | length' 4
req GET "/permissions?resource.startswith=${G_PFX}&updatedAt.gte=2000-01-01T00:00:00Z"
assert_jq "G4b ?updatedAt.gte= — filterable, deliberately not orderable" '.data | length' 4

# Ordering by a column that carries NO value on the wire is the promise `hidden`
# makes, and these are the cases that prove it.
for ob in resource -resource action -action description -description; do
  req GET "/permissions?resource.startswith=${G_PFX}&orderBy=${ob}"
  assert_status "G5 ?orderBy=${ob} (declared in the sort vocabulary)" 200
done
req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource"
assert_jq "G5 ascending order is honoured on an invisible column" '.data[0].permission' "${G_PFX}-1:read"
req GET "/permissions?resource.startswith=${G_PFX}&orderBy=-resource"
assert_jq "G5 descending order is honoured on an invisible column" '.data[0].permission' "${G_PFX}-4:read"

# The computed-field pushdown, end to end: the framework sends Resource+Action to
# the store in place of a column that does not exist, then blanks them before the
# Response projection.
req GET "/permissions?resource.eq=${G_PFX}-1&fields=permission"
assert_jq "G6a ?fields=permission returns the rendering" '.data[0].permission' "${G_PFX}-1:read"
assert_absent "G6b ?fields=permission drops description" '.data[0].description'
assert_absent "G6c the pushed-down sources are still not on the wire" '.data[0].resource'
req GET "/permissions?resource.eq=${G_PFX}-1&fields=description"
assert_status "G7a ?fields=description" 200
assert_absent "G7b ?fields=description drops the computed field" '.data[0].permission'

req GET "/permissions?resource.startswith=${G_PFX}&onlyTotal=true"
assert_jq "G8a ?onlyTotal=true reports the total" '.pagination.totalCount' 4
assert_absent "G8b ?onlyTotal=true carries no data array" '.data'

req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource&last=2"
assert_jq "G9a ?last= alone serves the TAIL window" '.data | length' 2
assert_jq "G9b the tail window is the LAST rows, not the first" '.data[0].permission' "${G_PFX}-3:read"

# G10 — the envelope as a BICONDITIONAL, which is the correction the tenant round
# earned: EndCursor is set exactly when HasNextPage, StartCursor exactly when
# HasPreviousPage (application/queries/view_reader.go:136). So the first page of a
# forward walk carries NO startCursor, and asserting one present would be weaker
# AND wrong.
req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource&first=2"
assert_jq      "G10a page 1 · totalCount counts the whole filtered set" '.pagination.totalCount' 4
assert_jq_true "G10b page 1 · endCursor set exactly when hasNextPage" \
  '(.pagination.hasNextPage) == (.pagination.endCursor != null and .pagination.endCursor != "")' \
  'the endCursor/hasNextPage biconditional holds'
assert_jq_true "G10c page 1 · startCursor set exactly when hasPreviousPage" \
  '(.pagination.hasPreviousPage) == (.pagination.startCursor != null and .pagination.startCursor != "")' \
  'the startCursor/hasPreviousPage biconditional holds'
P1_IDS="$(j '[.data[].id] | sort | join(",")')"
END_CURSOR="$(j '.pagination.endCursor')"

req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource&first=2&after=${END_CURSOR}"
assert_jq "G10d page 2 · hasPreviousPage is true once a page precedes it" '.pagination.hasPreviousPage' 'true'
P2_IDS="$(j '[.data[].id] | sort | join(",")')"
[ -n "$P1_IDS" ] && [ "$P1_IDS" != "$P2_IDS" ] \
  && pass "G10e page 2 is DISJOINT from page 1" \
  || fail "G10e page 2 is DISJOINT from page 1" "two different id sets" "both pages were [$P1_IDS]" "$RESP_BODY"
P2_START="$(j '.pagination.startCursor')"

req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource&last=2&before=${P2_START}"
assert_jq "G10f walking back with ?before= returns page 1" '[.data[].id] | sort | join(",")' "$P1_IDS"

req PATCH "/permissions/$(printf '%s' "$P1_IDS" | cut -d, -f1)/archive" ''
req GET "/permissions?resource.startswith=${G_PFX}&onlyTotal=true"
assert_jq "G11a archiving lowers the active total by exactly one" '.pagination.totalCount' 3
req GET "/permissions?resource.startswith=${G_PFX}&includeArchived=true&onlyTotal=true"
assert_jq "G11b ?includeArchived raises it back by exactly the archived count" '.pagination.totalCount' 4

skip "G14 ?search=" "the DTO does not declare it, so the opt-in gate answers first (H5) before any read engine sees it. UnsupportedCapabilityNotification is unreachable on this entity — asserting it would assert a promise nobody made."

# ════════════════════════════════════════════════════════════════════════════
section "H · rejected reads — the whole typed-400 guard family"

req GET '/permissions?bogus=1'
assert_status_key_field "H1 an unknown query key" 400 'SchemaViolationNotification' 'bogus'
req GET '/permissions?resource.gte=x'
assert_status_key_field "H2 an operator outside the field's allowlist" 400 'SchemaViolationNotification' 'resource.gte'
# The asymmetry that earns its own case: startswith IS declared on resource and
# is NOT declared on action.
req GET '/permissions?action.startswith=re'
assert_status_key_field "H3 ?action.startswith= — declared on resource, not on action" 400 'SchemaViolationNotification' 'action.startswith'
req GET '/permissions?description.eq=x'
assert_status_key_field "H4 ?description.eq= — contains is the only operator it declares" 400 'SchemaViolationNotification' 'description.eq'
req GET '/permissions?search=tenant'
assert_status_key_field "H5 a reserved control the DTO never declared" 400 'SchemaViolationNotification' 'search'
req GET '/permissions?permission.eq=tenant:read'
assert_status_key_field "H6 filtering the COMPUTED field — it carries no query tag at all" 400 'SchemaViolationNotification' 'permission.eq'

req GET '/permissions?orderBy=permission'
assert_status_key_field "H7 ?orderBy= on the computed field — it backs no column" 400 'SchemaViolationNotification' 'orderBy[permission]'
req GET '/permissions?orderBy=createdAt'
assert_status_key_field "H8a ?orderBy=createdAt — filterable, never orderable" 400 'SchemaViolationNotification' 'orderBy[createdAt]'
req GET '/permissions?orderBy=updatedAt'
assert_status_key_field "H8b ?orderBy=updatedAt — filterable, never orderable" 400 'SchemaViolationNotification' 'orderBy[updatedAt]'
req GET '/permissions?orderBy=id'
assert_status_key_field "H9 ?orderBy=id — declarable and deliberately not declared" 400 'SchemaViolationNotification' 'orderBy[id]'
req GET '/permissions?orderBy=bogus'
assert_status_key_field "H10 ?orderBy= on an unknown token" 400 'SchemaViolationNotification' 'orderBy[bogus]'

# The sharpest pair on this entity: the hidden parts are filterable (G1) and
# orderable (G5) and NOT selectable. All three halves are the same declaration
# read from three sides.
req GET '/permissions?fields=resource'
assert_status_key_field "H11a ?fields=resource — filterable and orderable, never SELECTABLE" 400 'SchemaViolationNotification' 'fields[resource]'
req GET '/permissions?fields=action'
assert_status_key_field "H11b ?fields=action — same" 400 'SchemaViolationNotification' 'fields[action]'
req GET '/permissions?fields=bogus'
assert_status_key_field "H12 ?fields= on an unknown path" 400 'SchemaViolationNotification' 'fields[bogus]'

req GET '/permissions?first=101'
assert_status_key "H13 ?first= above the resolved ceiling" 400 'LimitExceededNotification'
assert_jq "H13 the ceiling reported is the framework default" '.errors[0].messages[0].value' '100'
for bad in 0 abc -5; do
  req GET "/permissions?first=${bad}"
  assert_status_key_field "H14 ?first=${bad}" 400 'SchemaViolationNotification' 'first'
done

req GET '/permissions?first=2&last=2'
assert_status "H15a ?first= with ?last=" 400
req GET '/permissions?first=2&before=abc'
assert_status "H15b ?first= with ?before=" 400
req GET '/permissions?last=2&after=abc'
assert_status "H15c ?last= with ?after=" 400
req GET '/permissions?after=abc&before=def'
assert_status "H15d ?after= with ?before=" 400

for conflict in 'first=10' 'orderBy=resource' 'fields=permission' 'after=abc'; do
  req GET "/permissions?onlyTotal=true&${conflict}"
  assert_status_key "H16 ?onlyTotal=true beside ${conflict}" 400 'SchemaViolationNotification'
done
req GET "/permissions?onlyTotal=true&resource.startswith=${G_PFX}"
assert_status "H17a ?onlyTotal=true with a filter — counting a subset is the point" 200
req GET '/permissions?onlyTotal=true&includeArchived=true'
assert_status "H17b ?onlyTotal=true with ?includeArchived=true" 200

req GET '/permissions?after=not-a-cursor'
assert_status_key_field "H18 a malformed cursor" 400 'SchemaViolationNotification' 'after'

req GET "/permissions?resource.startswith=${G_PFX}&first=1"
C_NOORDER="$(j '.pagination.endCursor')"
req GET "/permissions?resource.startswith=${G_PFX}&first=1&orderBy=resource&after=${C_NOORDER}"
assert_status "H19 a cursor replayed under a different ?orderBy=" 400
req GET "/permissions?resource.startswith=${G_PFX}&first=1&includeArchived=true&after=${C_NOORDER}"
assert_status "H20 a cursor replayed under a different ?includeArchived=" 400

req GET '/permissions?includeArchived=1'
assert_status_key_field "H21a booleans take exactly true/false" 400 'SchemaViolationNotification' 'includeArchived'
req GET '/permissions?onlyTotal='
assert_status_key_field "H21b an empty boolean is still a value" 400 'SchemaViolationNotification' 'onlyTotal'

# The by-id DTO declares includeArchived and NOTHING else, and presence is what
# trips the gate — so an undeclared control rejects even at its default value.
req GET "/permissions/${GID}?onlyTotal=false"
assert_status_key_field "H22a by-id gate · ?onlyTotal= is not declared there" 400 'SchemaViolationNotification' 'onlyTotal'
req GET "/permissions/${GID}?fields=permission"
assert_status_key_field "H22b by-id gate · ?fields= is not declared there" 400 'SchemaViolationNotification' 'fields'
req GET "/permissions/${GID}?includeArchived=true"
assert_status "H23 by-id · the one control it DOES declare" 200

req GET '/permissions?createdAt.gte=not-a-date'
assert_status_key "H24a a filter value outside the leaf's kind" 400 'InvalidFilterValueNotification'
# The bare shorthand is ?createdAt.eq=, and this entity declares only gte and
# lte on the temporal leaves — so the SCHEMA gate answers before any value is
# parsed. Tenant declares eq there and reaches InvalidFilterValue on the same
# request; the difference is the DTO, and asserting tenant's answer here would
# assert a promise this entity does not make.
req GET '/permissions?createdAt=not-a-date'
assert_status_key_field "H24b the bare shorthand is an undeclared OPERATOR here, not a bad value" 400 'SchemaViolationNotification' 'createdAt'

# ════════════════════════════════════════════════════════════════════════════
section "I · routing, not-found, and the by-id ADDRESS contract"

req GET '/permissions/00000000-0000-7000-8000-000000000999'
assert_status_key "I1 a well-formed id that addresses nothing" 404 'RecordNotFoundNotification'
req DELETE "/permissions/${GID}"
assert_status_key "I2 DELETE — proves no hard delete exists" 405 'MethodNotAllowedNotification'
req POST "/permissions/${GID}" '{}'
assert_status_key "I3 POST on the by-id path" 405 'MethodNotAllowedNotification'
req GET "/permissions/${GID}/archive"
assert_status_key "I4 GET on a path registered under PATCH" 405 'MethodNotAllowedNotification'
req PATCH "/permissions/${GID}/purge" ''
assert_status_key "I5 a path matching no route at all" 404 'RouteNotFoundNotification'

# The by-id address contract, split by VERB and not by surface (pin ≥ v0.70.0).
req GET '/permissions/not-a-uuid'
assert_status_key "I6 a read addressed by a non-uuid" 404 'UnknownIDAddressNotification'
req PATCH '/permissions/not-a-uuid' '{"description":"A patch aimed at an address that is not an id."}'
assert_status_key "I7 a write addressed by a non-uuid" 400 'MalformedIDNotification'
req PATCH '/permissions/not-a-uuid/archive' ''
assert_status_key "I8 archive addressed by a non-uuid" 400 'MalformedIDNotification'
gql '{ permission(id: "not-a-uuid") { id } }'
assert_gql_key "I9 the same read rule on GraphQL" 'UnknownIDAddressNotification'
gql 'mutation { archivePermission(id: "not-a-uuid") { success } }'
assert_gql_key "I10 the same write rule on GraphQL" 'MalformedIDNotification'

skip "I11 the 403 arm of the three-way split" "no route is mounted for an undeclared mode — unarchive is absent from Modes() AND unmounted, so it lands on the 404 arm (B6/I5), never the 403 one."

# ════════════════════════════════════════════════════════════════════════════
section "J · GraphQL — handler invariance, one operation answering identically"

req GET "/permissions?resource.startswith=${G_PFX}&orderBy=resource&first=2"
REST_FIRST="$(j '.data[0].permission')"
gql "{ permissions(first: 2, where: { resource: { startswith: \"${G_PFX}\" } }, orderBy: [{ field: RESOURCE, direction: ASC }]) { edges { cursor node { id description permission } } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } totalCount } }"
assert_jq "J1 the GraphQL node equals the REST listing row" '.data.permissions.edges[0].node.permission' "$REST_FIRST"

gql "{ permission(id: \"${GID}\") { id description permission } }"
assert_jq "J2 the GraphQL document equals the REST by-id document" '.data.permission.permission' "${R_G}:rotate-secret"

R_J="${PG}-j3"
gql "mutation { createPermission(input: { resource: \"${R_J}\", action: \"read\", description: \"Created over GraphQL and read back over REST.\" }) { id permission } }"
assert_gql_ok "J3a createPermission"
J3_ID="$(j '.data.createPermission.id')"
req GET "/permissions/${J3_ID}"
assert_jq "J3b one write, two surfaces — REST sees it immediately" '.data.permission' "${R_J}:read"

gql "mutation { patchPermission(id: \"${J3_ID}\", input: { description: \"Amended over GraphQL and read back over REST.\" }) { id description } }"
assert_gql_ok "J4a patchPermission"
req GET "/permissions/${J3_ID}"
assert_jq "J4b REST sees the amended description" '.data.description' 'Amended over GraphQL and read back over REST.'

gql "mutation { archivePermission(id: \"${J3_ID}\") { success id } }"
assert_jq "J5a archivePermission reports success" '.data.archivePermission.success' 'true'
req GET "/permissions/${J3_ID}"
assert_status_key "J5b REST confirms the effect" 404 'RecordNotFoundNotification'

gql '{ permission(id: "00000000-0000-7000-8000-000000000999") { id } }'
assert_gql_key "J6 not-found in the GraphQL idiom, HTTP 200 with a typed error" 'RecordNotFoundNotification'

R_J7="${PG}-j7"
create_permission "$R_J7" read >/dev/null
gql "mutation { createPermission(input: { resource: \"${R_J7}\", action: \"read\", description: \"An attempt to take a pair that is already held.\" }) { id } }"
assert_gql_key "J7a the 409 in the GraphQL idiom" 'PermissionAlreadyExistsNotification'
assert_jq "J7b semantic rides in extensions too" '.errors[0].extensions.semantic' 'Conflict'

gql '{ permissions(search: "tenant") { totalCount } }'
assert_gql_error_matching "J8 an undeclared control is an unknown argument, not a REST 400" 'search'

gql "{ permissions(where: { resource: { startswith: \"${G_PFX}\" } }, orderBy: [{ field: RESOURCE, direction: DESC }]) { edges { node { permission } } } }"
assert_jq_true "J9 where + orderBy over the HIDDEN parts agree with their REST twins" \
  "[.data.permissions.edges[].node.permission] | first | startswith(\"${G_PFX}\")" \
  'the invisible columns filter and order on GraphQL too'

# pin ≥ v0.72.1: __typename beside a selection must not change the answer. Every
# mainstream client appends it for cache normalization.
gql "{ permission(id: \"${GID}\") { __typename id description permission } }"
assert_jq "J10 __typename beside the selection answers identically" '.data.permission.permission' "${R_G}:rotate-secret"

# The GraphQL half of C3: the schema must not carry a field the REST body hides.
gql "{ permission(id: \"${GID}\") { id resource } }"
assert_gql_error_matching "J11a the node exposes no resource field" 'resource'
gql "{ permission(id: \"${GID}\") { id action } }"
assert_gql_error_matching "J11b the node exposes no action field" 'action'

skip "J12 gRPC parity" "no transport: block in the yaml — no gRPC surface is wired, so there is nothing to answer identically."
skip "J13 tabular exports" "none declared on this entity (surfaces: rest + graphql only)."

# ════════════════════════════════════════════════════════════════════════════
section "X · suite meta — the route inventory the document itself advertises"

req GET /openapi.json
assert_status "X1a the document is served" 200
assert_jq "X1b exactly FIVE permission operations are advertised — no unarchive" \
  '[.paths | to_entries[] | select(.key | test("^/permissions")) | .value | keys[]] | length' 5
assert_jq_true "X1c the listing route declares permission:read" \
  '[.paths | to_entries[] | select(.key | test("^/permissions/?$")) | .value.get.description] | join(" ") | test("permission:read")' \
  'GET /permissions names permission:read'
assert_jq_true "X1d no unarchive path is advertised" \
  '[.paths | keys[] | select(test("^/permissions.*unarchive"))] | length == 0' \
  'the document advertises no permission unarchive route'

lane_summary
