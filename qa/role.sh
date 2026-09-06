#!/usr/bin/env bash
# Lane: role — §1 of specs/qa/role-contract/plan.md.
#
# What the FRAMEWORK promises about this entity, on both wired surfaces. The
# business rules are qa/domain.sh; the refusals are qa/security.sh.
#
# TWO SHAPES MAKE THIS ENTITY UNLIKE THE TWO BEFORE IT.
#
# 1. It carries BOTH kinds of read join at once, and half this file is built on
#    the contrast:
#      ROOT  → Tenant     TenantWorkspace / TenantStatus
#                         served · filterable · sortable · selectable
#      CHILD → Permission Resource / Action
#                         hidden · load-only · feeding the computed `permission`
#    One service, both halves, asserted side by side. Nothing earlier could.
#
# 2. It has a COLLECTION with a two-verb edit strategy — GRANT and REVOKE, no
#    per-entry update and no per-entry unarchive. The canonical trap of the model
#    is that the root-archive handler is instantiated once per surface, so wiring
#    it to the REVOKE route type-checks, boots, answers 200 and archives the whole
#    role. B6 is the case that would see it.
#
# Read backing is RELATIONAL, so read-your-writes is the promise: every read-back
# here is IMMEDIATE and a case that only passes after a retry is itself a
# failure. No poll, no drain, no CDC anywhere.

cd "$(dirname "$0")/.." || exit 2
LANE_NAME=role
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

# The lane runs as the *:* bootstrap admin, so nothing here is blocked by row
# scope: §3 and §1b are where a scoped principal does that work.
PG="qa-$(_slug "$LANE_NAME")-$(_slug "${QA_RUN_TAG:-$$}")"

# ════════════════════════════════════════════════════════════════════════════
section "G14 · the seeded baseline — FIRST, before this lane writes anything"
note "migrations/postgres/0012_bootstrap_seed_manual.up.sql seeds ONE role (master)"
note "and ONE grant (the *:* wildcard). Both numbers come from the tracked file."

MASTER_TENANT_ID='01990000-0001-7000-8000-000000000001'
MASTER_ROLE_ID='01990000-0002-7000-8000-000000000001'
WILDCARD_ID='01990000-0000-7000-8000-000000000000'

req GET '/roles?onlyTotal=true'
assert_jq "G14a a freshly migrated database holds exactly ONE role" '.pagination.totalCount' 1

req GET "/roles/${MASTER_ROLE_ID}"
assert_status "G14b the seeded master role is readable by the *:* admin" 200
assert_jq "G14c it belongs to the master tenant" '.data.tenantID' "$MASTER_TENANT_ID"
assert_jq "G14d it carries exactly one grant" '.data.permissions | length' 1
# The rendered token, read through the CHILD join and the computed derivation, on
# the one row the migration guarantees exists.
assert_jq "G14e that grant renders as the wildcard" '.data.permissions[0].permission' '*:*'
assert_jq "G14f and it addresses the seeded wildcard row by id" '.data.permissions[0].permissionID' "$WILDCARD_ID"
# The ROOT join reaching the wire on the seeded row.
assert_jq "G14g the root join carries the owner's handle" '.data.tenantWorkspace' 'master'
assert_jq "G14h and the owner's commercial status" '.data.tenantStatus' 'active'

# ── the lane's own tenants and catalog ids ─────────────────────────────────
# An id is an ADDRESS, never an expectation: every catalog id below is resolved
# from the pair migration 0012 declares, so no literal is written twice.
T_MAIN_WS="$(_slug "${PG}-main")"
T_MAIN="$(create_tenant "$T_MAIN_WS" 'Qa Role Main' 'The tenant that owns the roles this lane writes, kept apart from the master one.' 'active')"
[ -n "$T_MAIN" ] || { printf 'precondition: could not create the lane tenant: %s\n' "$RESP_BODY"; exit 2; }
T_ALT_WS="$(_slug "${PG}-alt")"
T_ALT="$(create_tenant "$T_ALT_WS" 'Qa Role Alt' 'A second tenant, so uniqueness within a tenant can be told from uniqueness across all of them.' 'active')"

PID_TENANT_READ="$(permission_id_of tenant read)"
PID_ROLE_READ="$(permission_id_of role read)"
PID_PERM_READ="$(permission_id_of permission read)"
PID_USER_READ="$(permission_id_of user read)"
for v in PID_TENANT_READ PID_ROLE_READ PID_PERM_READ PID_USER_READ; do
  eval "val=\$$v"
  [ -n "$val" ] || { printf 'precondition: %s did not resolve to a catalog id\n' "$v"; exit 2; }
done

# ════════════════════════════════════════════════════════════════════════════
section "B · happy path — one per served verb (seven, not five)"

K_B="${PG}-b1"
# req POST directly rather than create_role: the helper runs in a subshell under
# command substitution, so RESP_CODE would not reach the assertion below.
req POST /roles "$(role_body "$K_B" 'Qa Insert Role' \
  'A role created to prove the insert verb answers with the row exactly as stored.' \
  "$T_MAIN" "$(jq -nc --arg a "$PID_TENANT_READ" --arg b "$PID_ROLE_READ" '[$a,$b]')")"
assert_status "B1 insert with two grants" 201
ID_B="$(j '.data.id')"
assert_jq "B1 the owner is the tenant the body named" '.data.tenantID' "$T_MAIN"
assert_jq "B1 both grants were stored" '.data.permissions | length' 2
# The WRITE entry carries the stored column and the id the server minted, and NOT
# the derivation: `permission` is computed on the READ side, once per entry,
# below the web boundary. A value here would mean it moved somewhere it must not.
assert_absent "B1 the write entry does not carry the computed rendering" '.data.permissions[0].permission'
assert_absent "B1 nor the child join's resource" '.data.permissions[0].resource'
assert_absent "B1 nor its action" '.data.permissions[0].action'

req GET "/roles/${ID_B}"
assert_status "B2 by-id" 200
# Now the derivation runs: one token per entry, rebuilt through PermissionKey.
assert_jq "B2 the read entries render their permission" '[.data.permissions[].permission] | sort | join(",")' 'role:read,tenant:read'

req GET "/roles?key.eq=${K_B}"
assert_status "B3 list filtered to the new row" 200
assert_jq "B3 exactly one row answers that key" '.data | length' 1

req PATCH "/roles/${ID_B}" '{"name":"Qa Renamed Role","description":"An amended wording that explains what holding this role actually allows."}'
assert_status "B4 patch the two editable fields" 200
assert_jq "B4 the name changed" '.data.name' 'Qa Renamed Role'
assert_jq "B4 the key did NOT move" '.data.key' "$K_B"
assert_jq "B4 nor did the owner" '.data.tenantID' "$T_MAIN"

req POST "/roles/${ID_B}/permissions" "$(jq -nc --arg p "$PID_PERM_READ" '{permissionID:$p}')"
assert_status "B5 GRANT one more permission" 201
assert_jq "B5 the response names the owner" '.data.roleId' "$ID_B"
assert_jq "B5 and carries the entry as stored" '.data.rolePermission.permissionID' "$PID_PERM_READ"
CHILD_GRANTED="$(j '.data.rolePermission.id')"
req GET "/roles/${ID_B}"
assert_jq "B5 the grant is visible on the very next read" '.data.permissions | length' 3

# ── B6, the canonical trap of this model ───────────────────────────────────
# spec.md §3: "the root-archive auto handler is instantiated exactly ONCE per
# surface. Wiring it to the child REVOKE route type-checks, boots, answers 200 —
# and archives the entire role." Nothing else in this file would notice.
req PATCH "/roles/${ID_B}/permissions/${CHILD_GRANTED}/archive" '{}'
assert_status "B6 REVOKE one grant" 204
req GET "/roles/${ID_B}"
assert_status "B6 the ROOT IS STILL ACTIVE after a revoke — the canonical trap" 200
assert_jq "B6 the revoked entry is gone" '.data.permissions | length' 2
assert_jq_true "B6 and it is the revoked one that left, not an arbitrary entry" \
  "[.data.permissions[].permissionID] | index(\"${PID_PERM_READ}\") == null" \
  'the permission the revoke named is absent and the other two remain'

req PATCH "/roles/${ID_B}/archive" ''
assert_status "B7 archive the root" 204

# The absent verb. Role declares no unarchive mode AND mounts no route, so this
# lands on the 404 arm of the three-way split.
req PATCH "/roles/${ID_B}/unarchive" ''
assert_status_key "B8a no unarchive route exists on REST" 404 'RouteNotFoundNotification'
gql "mutation { unarchiveRole(id: \"${ID_B}\") { success } }"
assert_gql_error_matching "B8b no unarchiveRole field exists in the schema" 'unarchiveRole'

# ════════════════════════════════════════════════════════════════════════════
section "C · golden record — every declared field, and the rules-only join nowhere"

K_C="${PG}-golden"
GOLD_DESC='A golden record exercising every declared field of this aggregate exactly once.'
req POST /roles "$(role_body "$K_C" 'Qa Golden Role' "$GOLD_DESC" "$T_MAIN" \
  "$(jq -nc --arg a "$PID_TENANT_READ" '[$a]')")"
assert_status "C1 the golden record is accepted" 201
ID_C="$(j '.data.id')"

req GET "/roles/${ID_C}"
assert_status "C1 by-id" 200
assert_jq "C1 id"              '.data.id'          "$ID_C"
assert_jq "C1 tenantID"        '.data.tenantID'    "$T_MAIN"
assert_jq "C1 key"             '.data.key'         "$K_C"
assert_jq "C1 name"            '.data.name'        'Qa Golden Role'
assert_jq "C1 description"     '.data.description' "$GOLD_DESC"
# The ROOT join reaching the wire, both fields, against values this lane chose.
assert_jq "C1 tenantWorkspace — the ROOT join's first field" '.data.tenantWorkspace' "$T_MAIN_WS"
assert_jq "C1 tenantStatus — its second"                     '.data.tenantStatus'    'active'
# RFC3339 by its own grammar, NOT by jq's fromdateiso8601: that builtin accepts
# only a Z-suffixed instant with no fractional part, while this service stamps
# from Postgres (relational.clock: db) and answers e.g.
# 2026-09-03T23:31:06.322534-04:00 — a perfectly valid RFC3339 the builtin
# refuses. Asserting through it would pin a narrower format than the contract.
RFC3339='^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
assert_jq_true "C1 createdAt parses as RFC3339" "(.data.createdAt | test(\"${RFC3339}\"))" 'createdAt is a real RFC3339 instant'
assert_jq_true "C1 updatedAt parses as RFC3339" "(.data.updatedAt | test(\"${RFC3339}\"))" 'updatedAt is a real RFC3339 instant'
assert_absent  "C1 deletedAt is not projected"  '.data.deletedAt'
assert_jq "C1 the entry carries its own id"   '.data.permissions[0].id != null'  'true'
assert_jq "C1 the entry carries the stored column" '.data.permissions[0].permissionID' "$PID_TENANT_READ"
assert_jq "C1 and the derivation of the child join" '.data.permissions[0].permission' 'tenant:read'

req GET "/roles?key.eq=${K_C}"
assert_jq "C1 the LISTING row renders the same owner handle" '.data[0].tenantWorkspace' "$T_MAIN_WS"
assert_jq "C1 the LISTING row renders the same entry token"  '.data[0].permissions[0].permission' 'tenant:read'

gql "{ role(id: \"${ID_C}\") { id key name description tenantID tenantWorkspace tenantStatus permissions { id permissionID permission } } }"
assert_gql_ok "C1 the GraphQL node answers the same document"
assert_jq "C1 GraphQL renders the owner handle"  '.data.role.tenantWorkspace' "$T_MAIN_WS"
assert_jq "C1 GraphQL renders the entry token"   '.data.role.permissions[0].permission' 'tenant:read'

# ── C2, the case nobody writes ─────────────────────────────────────────────
# Resource and Action exist to FEED the derivation. They are rules-only join
# fields: in no response body, on no surface, under no selection. A leak here is
# silent and every other family in this file passes over it.
for probe in ".data.permissions[0].resource" ".data.permissions[0].action"; do
  req GET "/roles/${ID_C}"
  assert_absent "C2 by-id · ${probe} never reaches the wire" "$probe"
done
req GET "/roles?key.eq=${K_C}"
assert_absent "C2 listing · resource never reaches the wire" '.data[0].permissions[0].resource'
assert_absent "C2 listing · action never reaches the wire"   '.data[0].permissions[0].action'
req GET "/roles?key.eq=${K_C}&fields=permissions.permission"
assert_absent "C2 under ?fields=permissions.permission · resource is still absent" '.data[0].permissions[0].resource'
assert_absent "C2 under ?fields=permissions.permission · action is still absent"   '.data[0].permissions[0].action'
req GET "/roles?key.eq=${K_C}&includeArchived=true"
assert_absent "C2 under ?includeArchived=true · resource is still absent" '.data[0].permissions[0].resource'

# ════════════════════════════════════════════════════════════════════════════
section "D · validation 422 — the KEY and the FIELD"

D_DESC='A description long enough to satisfy the value object on its own terms.'
d_insert() { req POST /roles "$(role_body "$1" "${2-Qa Valid Name}" "${3-$D_DESC}" "${4-$T_MAIN}" '[]')"; }

d_insert ''            ; assert_status_key_field "D1 key: empty"                 422 'RequiredFieldNotification'  'key'
d_insert 'Billing'     ; assert_status_key_field "D2 key: uppercase"             422 'InvalidRoleKeyNotification' 'key'
d_insert 'a'           ; assert_status_key_field "D3 key: below the 2-rune floor" 422 'InvalidRoleKeyNotification' 'key'
d_insert 'bil--ling'   ; assert_status_key_field "D4 key: a doubled hyphen"      422 'InvalidRoleKeyNotification' 'key'
d_insert '-billing'    ; assert_status_key_field "D5a key: a leading hyphen"     422 'InvalidRoleKeyNotification' 'key'
d_insert 'billing-'    ; assert_status_key_field "D5b key: a trailing hyphen"    422 'InvalidRoleKeyNotification' 'key'
d_insert 'aaaab'       ; assert_status_key_field "D6 key: a run of four identical runes" 422 'InvalidRoleKeyNotification' 'key'
d_insert "$(printf 'ab%.0s' $(seq 1 33))" ; assert_status_key_field "D7 key: 66 runes, above the ceiling" 422 'InvalidRoleKeyNotification' 'key'

d_insert "${PG}-d8" ''      ; assert_status_key_field "D8 name: empty" 422 'RequiredFieldNotification' 'name'
d_insert "${PG}-d9" 'Qa Valid Name' 'short' ; assert_status_key_field "D9 description: below the floor" 422 'InvalidDescriptionNotification' 'description'

# The child's id is validated by the FRAMEWORK, after the rules — which is why
# the rules loop skips an unusable id rather than raising on it: complaining
# there would say the same thing twice.
req POST /roles "$(role_body "${PG}-d10" 'Qa Valid Name' "$D_DESC" "$T_MAIN" '["tatu"]')"
assert_status_key "D10 an entry whose permissionID is not a uuid" 422 'InvalidIDUUIDNotification'

req POST /roles "$(role_body "${PG}-d11" 'Qa Válido Nomé' 'Uma descrição acentuada que explica o que este papel realmente concede.' "$T_MAIN" '[]')"
assert_status "D11 positive control — a hyphenated key and an accented description" 201

# ── D12/D13, the guard barrier ─────────────────────────────────────────────
note "TenantID carries a valueObject rule with guard: true, so domain.ID.IsValid runs"
note "FIRST and StopIfInvalid ends the whole pass. Without it the bad owner reached"
note "the uniqueness pre-check, bound to a UUID column, and the probe panicked: a 500"
note "on a request whose problem is plain validation. That bug shipped once."

req POST /roles "$(role_body "${PG}-d12" 'Qa Valid Name' "$D_DESC" '' '[]')"
assert_status_key_field "D12a tenantID: empty" 422 'InvalidIDUUIDNotification' 'tenantID'
req POST /roles "$(role_body "${PG}-d12b" 'Qa Valid Name' "$D_DESC" 'tatu' '[]')"
assert_status_key_field "D12b tenantID: not a uuid — uuid.Parse refuses both through one call" 422 'InvalidIDUUIDNotification' 'tenantID'

# THE case: a barrier is a barrier only if nothing after it is reported.
req POST /roles "$(role_body "${PG}-d13" 'Qa Valid Name' 'short' 'tatu' '[]')"
assert_jq_true "D13 a bad owner AND a junk description report the OWNER ALONE" \
  '[.errors[]?.messages[]?.notificationKey]
     | (index("InvalidIDUUIDNotification") != null)
       and (index("InvalidDescriptionNotification") == null)' \
  'the barrier ended the pass before the value-object sweep ran'

# ════════════════════════════════════════════════════════════════════════════
section "E · 409 — and what \"unique WITHIN the tenant\" means"

K_E="${PG}-e1"
req POST /roles "$(role_body "$K_E" 'Qa Conflict Role' "$D_DESC" "$T_MAIN" '[]')"
assert_status "E1 the first insert of the key" 201
ID_E="$(j '.data.id')"
req POST /roles "$(role_body "$K_E" 'Qa Conflict Twin' "$D_DESC" "$T_MAIN" '[]')"
assert_status_key_field "E1 the same key twice in ONE tenant" 409 'RoleKeyAlreadyExistsNotification' 'key'
assert_jq "E1 the semantic is Conflict, not Validation" '.errors[0].messages[0].semantic' 'Conflict'

# The control without which E1 would pass just as well for an over-broad GLOBAL
# index — which is exactly the regression that makes two customers collide.
req POST /roles "$(role_body "$K_E" 'Qa Conflict Elsewhere' "$D_DESC" "$T_ALT" '[]')"
assert_status "E2 the SAME key in a DIFFERENT tenant is accepted" 201

req PATCH "/roles/${ID_E}" '{"name":"Qa Conflict Renamed"}'
assert_status "E3 a patch that does not move the key does not self-collide" 200

req PATCH "/roles/${ID_E}/archive" ''
assert_status "E4 archive the holder of the key" 204
req POST /roles "$(role_body "$K_E" 'Qa Conflict Reborn' "$D_DESC" "$T_MAIN" '[]')"
assert_status "E4 the archived remnant does not block the key (scope: active-only)" 201
ID_E2="$(j '.data.id')"
[ -n "$ID_E2" ] && [ "$ID_E2" != "$ID_E" ] \
  && pass "E4 and it came back as a NEW row with a NEW id" \
  || fail "E4 and it came back as a NEW row with a NEW id" "an id different from ${ID_E}" "${ID_E2}" "$RESP_BODY"

req POST /roles "$(role_body "${PG}-e5" 'Qa Duplicate Grant' "$D_DESC" "$T_MAIN" \
  "$(jq -nc --arg a "$PID_TENANT_READ" '[$a,$a]')")"
assert_status_key "E5 the same permission twice inside ONE insert body" 409 'RoleAlreadyGrantsPermissionNotification'

K_E6="${PG}-e6"
req POST /roles "$(role_body "$K_E6" 'Qa Regrant Role' "$D_DESC" "$T_MAIN" \
  "$(jq -nc --arg a "$PID_TENANT_READ" '[$a]')")"
ID_E6="$(j '.data.id')"
CHILD_E6="$(j '.data.permissions[0].id')"
req POST "/roles/${ID_E6}/permissions" "$(jq -nc --arg p "$PID_TENANT_READ" '{permissionID:$p}')"
# IsSameBusinessIdentity over PermissionID ALONE is what keeps this correct: the
# entry carries three fields and two are join fields that are BLANK on a freshly
# added entry, so a compare-all-fields identity would answer "different" and the
# duplicate guard would fail open.
assert_status_key "E6 GRANT a permission the role already holds" 409 'RoleAlreadyGrantsPermissionNotification'

req PATCH "/roles/${ID_E6}/permissions/${CHILD_E6}/archive" '{}'
assert_status "E7 revoke it" 204
req POST "/roles/${ID_E6}/permissions" "$(jq -nc --arg p "$PID_TENANT_READ" '{permissionID:$p}')"
assert_status "E7 re-granting the revoked permission is accepted (child unique is active-only)" 201
CHILD_E7="$(j '.data.rolePermission.id')"
[ -n "$CHILD_E7" ] && [ "$CHILD_E7" != "$CHILD_E6" ] \
  && pass "E7 and it is a NEW entry id — there is no per-entry unarchive" \
  || fail "E7 and it is a NEW entry id" "an id different from ${CHILD_E6}" "${CHILD_E7}" "$RESP_BODY"

# ════════════════════════════════════════════════════════════════════════════
section "F · archive round-trip — kept but hidden, and NO way back"

K_F="${PG}-f1"
req POST /roles "$(role_body "$K_F" 'Qa Archive Role' "$D_DESC" "$T_MAIN" \
  "$(jq -nc --arg a "$PID_ROLE_READ" '[$a]')")"
ID_F="$(j '.data.id')"
CHILD_F="$(j '.data.permissions[0].id')"
req PATCH "/roles/${ID_F}/archive" ''
assert_status "F0 archive" 204

req GET "/roles/${ID_F}"
assert_status_key "F1 an archived role is invisible to a plain by-id" 404 'RecordNotFoundNotification'
req GET "/roles/${ID_F}?includeArchived=true"
assert_status "F2 ?includeArchived=true reveals it" 200
# A retired role must stay AUDITABLE, and with no unarchive verb this is the only
# way to see what a past grant meant.
assert_jq "F2 and its grants are still readable" '.data.permissions | length' 1
assert_jq "F2 still rendering their token" '.data.permissions[0].permission' 'role:read'

req GET "/roles?key.eq=${K_F}"
assert_jq "F3a the listing hides it" '.data | length' 0
req GET "/roles?key.eq=${K_F}&includeArchived=true"
assert_jq "F3b ?includeArchived=true reveals it there too" '.data | length' 1

req PATCH "/roles/${ID_F}/archive" ''
assert_status_key "F4 archiving an already-archived role" 404 'RecordNotFoundNotification'
req POST "/roles/${ID_F}/permissions" "$(jq -nc --arg p "$PID_USER_READ" '{permissionID:$p}')"
assert_status_key "F5 GRANT onto an archived role — LoadForWrite does not see it" 404 'RecordNotFoundNotification'
req PATCH "/roles/${ID_F}/permissions/${CHILD_F}/archive" '{}'
assert_status_key "F6 REVOKE on an archived role — same reason" 404 'RecordNotFoundNotification'

skip "F7 the child stamp-scoped unarchive family" \
     "N/A by construction: this root NEVER comes back. No unarchive mode, no route — so there is no restore for a stamp to be scoped to."

# ════════════════════════════════════════════════════════════════════════════
section "G · read vocabulary, the ROOT join in a criteria, and the envelope"

G_PFX="${PG}-page"
for n in 1 2 3 4; do
  req POST /roles "$(role_body "${G_PFX}-${n}" "Qa Paging Row ${n}" \
    "Seeded row number ${n} for the paging, filtering and ordering cases." "$T_MAIN" '[]')"
done
req GET "/roles?key.startswith=${G_PFX}&onlyTotal=true"
assert_jq "G0 the paging fixture is countable" '.pagination.totalCount' 4

req GET "/roles?key.eq=${G_PFX}-1"
assert_jq "G1a ?key.eq="         '.data | length' 1
req GET "/roles?tenantID.eq=${T_MAIN}&key.ne=${G_PFX}-1&key.startswith=${G_PFX}"
assert_jq "G1b ?key.ne=" '.data | length' 3
req GET "/roles?key.in=${G_PFX}-1,${G_PFX}-2"
assert_jq "G1c ?key.in="         '.data | length' 2
req GET "/roles?key.startswith=${G_PFX}"
assert_jq "G1d ?key.startswith=" '.data | length' 4
req GET "/roles?key.istartswith=$(printf '%s' "$G_PFX" | tr 'a-z' 'A-Z')"
assert_jq "G1e ?key.istartswith= — case-insensitive" '.data | length' 4
req GET "/roles?key.contains=${G_PFX}"
assert_jq "G1f ?key.contains="    '.data | length' 4
req GET "/roles?key.icontains=$(printf '%s' "$G_PFX" | tr 'a-z' 'A-Z')"
assert_jq "G1g ?key.icontains="   '.data | length' 4

req GET "/roles?key.startswith=${G_PFX}&name.startswith=Qa%20Paging"
assert_jq "G2a ?name.startswith=" '.data | length' 4
req GET "/roles?key.startswith=${G_PFX}&name.contains=Paging"
assert_jq "G2b ?name.contains="   '.data | length' 4
req GET "/roles?key.startswith=${G_PFX}&name.icontains=paging"
assert_jq "G2c ?name.icontains="  '.data | length' 4
req GET "/roles?name.in=Qa%20Paging%20Row%201,Qa%20Paging%20Row%202"
assert_jq "G2d ?name.in="         '.data | length' 2

req GET "/roles?key.startswith=${G_PFX}&description.contains=paging"
assert_jq "G3a ?description.contains=" '.data | length' 4
req GET "/roles?key.startswith=${G_PFX}&description.icontains=PAGING"
assert_jq "G3b ?description.icontains=" '.data | length' 4

req GET "/roles?tenantID.eq=${T_MAIN}&key.startswith=${G_PFX}"
assert_jq "G4a ?tenantID.eq=" '.data | length' 4
req GET "/roles?tenantID.in=${T_MAIN},${T_ALT}&key.startswith=${G_PFX}"
assert_jq "G4b ?tenantID.in=" '.data | length' 4

# ── G5/G6 — the ROOT JOIN reaching a criteria. The round's headline. ───────
note "read-joins.html: a root join's fields are \"an ordinary field of the loaded"
note "entity, filterable, sortable, projectable and exportable\". These six cases"
note "answer \"the roles of <workspace>\" over a column that lives in another table."
req GET "/roles?tenantWorkspace.eq=${T_MAIN_WS}&key.startswith=${G_PFX}"
assert_jq "G5a ?tenantWorkspace.eq= — a column of ANOTHER table in a filter" '.data | length' 4
assert_jq_true "G5a and every row it returned really belongs to that tenant" \
  "[.data[].tenantID] | unique == [\"${T_MAIN}\"]" \
  'the join predicate selected the right rows, not merely some rows'
req GET "/roles?tenantWorkspace.startswith=${T_MAIN_WS}&key.startswith=${G_PFX}"
assert_jq "G5b ?tenantWorkspace.startswith=" '.data | length' 4
req GET "/roles?tenantWorkspace.istartswith=$(printf '%s' "$T_MAIN_WS" | tr 'a-z' 'A-Z')&key.startswith=${G_PFX}"
assert_jq "G5c ?tenantWorkspace.istartswith=" '.data | length' 4
req GET "/roles?tenantWorkspace.contains=${T_MAIN_WS}&key.startswith=${G_PFX}"
assert_jq "G5d ?tenantWorkspace.contains=" '.data | length' 4
req GET "/roles?tenantWorkspace.icontains=$(printf '%s' "$T_MAIN_WS" | tr 'a-z' 'A-Z')&key.startswith=${G_PFX}"
assert_jq "G5e ?tenantWorkspace.icontains=" '.data | length' 4
req GET "/roles?tenantWorkspace.in=${T_MAIN_WS},${T_ALT_WS}&key.startswith=${G_PFX}"
assert_jq "G5f ?tenantWorkspace.in=" '.data | length' 4

req GET "/roles?tenantStatus.eq=active&key.startswith=${G_PFX}"
assert_jq "G6a ?tenantStatus.eq= — the second join field" '.data | length' 4
req GET "/roles?tenantStatus.eq=suspended&key.startswith=${G_PFX}"
assert_jq "G6b ?tenantStatus.eq=suspended finds none of them" '.data | length' 0
req GET "/roles?tenantStatus.in=active,trial&key.startswith=${G_PFX}"
assert_jq "G6c ?tenantStatus.in=" '.data | length' 4

req GET "/roles?key.startswith=${G_PFX}&createdAt.gte=2000-01-01T00:00:00Z&createdAt.lte=2999-01-01T00:00:00Z"
assert_jq "G7a ?createdAt.gte= + .lte=" '.data | length' 4
req GET "/roles?key.startswith=${G_PFX}&updatedAt.gte=2000-01-01T00:00:00Z"
assert_jq "G7b ?updatedAt.gte=" '.data | length' 4

# Fourteen orderings: every one of the seven sortable fields, both directions —
# and two of them are columns of another table.
for ob in key -key name -name tenantID -tenantID tenantWorkspace -tenantWorkspace \
          tenantStatus -tenantStatus createdAt -createdAt updatedAt -updatedAt; do
  req GET "/roles?key.startswith=${G_PFX}&orderBy=${ob}"
  assert_status "G8 ?orderBy=${ob}" 200
done
req GET "/roles?key.startswith=${G_PFX}&orderBy=key"
assert_jq "G8 ascending order is honoured"  '.data[0].key' "${G_PFX}-1"
req GET "/roles?key.startswith=${G_PFX}&orderBy=-key"
assert_jq "G8 descending order is honoured" '.data[0].key' "${G_PFX}-4"

req GET "/roles?key.eq=${G_PFX}-1&fields=key"
assert_jq    "G9a ?fields=key returns it"        '.data[0].key' "${G_PFX}-1"
assert_absent "G9b ?fields=key drops name"       '.data[0].name'
assert_absent "G9c and drops the collection"     '.data[0].permissions'
req GET "/roles?key.eq=${K_C}&fields=tenantWorkspace"
assert_jq    "G9d ?fields=tenantWorkspace — the ROOT join is SELECTABLE" '.data[0].tenantWorkspace' "$T_MAIN_WS"
assert_absent "G9e and it dropped everything else" '.data[0].key'
req GET "/roles?key.eq=${K_C}&fields=permissions.permission"
assert_jq    "G9f ?fields=permissions.permission reaches inside the collection" '.data[0].permissions[0].permission' 'tenant:read'
assert_absent "G9g and drops the entry's stored column" '.data[0].permissions[0].permissionID'

req GET "/roles?key.startswith=${G_PFX}&onlyTotal=true"
assert_jq     "G10a ?onlyTotal=true reports the total" '.pagination.totalCount' 4
assert_absent "G10b ?onlyTotal=true carries no data array" '.data'

req GET "/roles?key.startswith=${G_PFX}&orderBy=key&last=2"
assert_jq "G11a ?last= alone serves the TAIL window" '.data | length' 2
assert_jq "G11b the tail window is the LAST rows"    '.data[0].key' "${G_PFX}-3"

req GET "/roles?key.startswith=${G_PFX}&orderBy=key&first=2"
assert_jq      "G12a page 1 · totalCount counts the whole filtered set" '.pagination.totalCount' 4
assert_jq_true "G12b page 1 · endCursor set exactly when hasNextPage" \
  '(.pagination.hasNextPage) == (.pagination.endCursor != null and .pagination.endCursor != "")' \
  'the endCursor/hasNextPage biconditional holds'
assert_jq_true "G12c page 1 · startCursor set exactly when hasPreviousPage" \
  '(.pagination.hasPreviousPage) == (.pagination.startCursor != null and .pagination.startCursor != "")' \
  'the startCursor/hasPreviousPage biconditional holds'
P1_IDS="$(j '[.data[].id] | sort | join(",")')"
END_CURSOR="$(j '.pagination.endCursor')"

req GET "/roles?key.startswith=${G_PFX}&orderBy=key&first=2&after=${END_CURSOR}"
assert_jq "G12d page 2 · hasPreviousPage is true once a page precedes it" '.pagination.hasPreviousPage' 'true'
P2_IDS="$(j '[.data[].id] | sort | join(",")')"
[ -n "$P1_IDS" ] && [ "$P1_IDS" != "$P2_IDS" ] \
  && pass "G12e page 2 is DISJOINT from page 1" \
  || fail "G12e page 2 is DISJOINT from page 1" "two different id sets" "both pages were [$P1_IDS]" "$RESP_BODY"
START_CURSOR="$(j '.pagination.startCursor')"
req GET "/roles?key.startswith=${G_PFX}&orderBy=key&last=2&before=${START_CURSOR}"
assert_jq_true "G12f walking back with ?before= returns page 1" \
  "([.data[].id] | sort | join(\",\")) == \"${P1_IDS}\"" \
  'the cursor is a window edge in both directions'

req GET "/roles?key.startswith=${G_PFX}&onlyTotal=true&includeArchived=true"
G_ARCH="$(j '.pagination.totalCount')"
assert_jq_true "G13 ?includeArchived=true never LOWERS the total" \
  "${G_ARCH:-0} >= 4" 'the archived set is a superset of the active one'

# ════════════════════════════════════════════════════════════════════════════
section "H · rejected reads — the whole typed-400 guard family"

req GET '/roles?bogus=1'
assert_status_key_field "H1 an unknown query key" 400 'SchemaViolationNotification' 'bogus'
req GET '/roles?key.gte=x'
assert_status_key_field "H2 an operator outside the field's allowlist" 400 'SchemaViolationNotification' 'key.gte'
# The asymmetry that earns its own case: ne IS declared on key and is NOT on name.
req GET '/roles?name.ne=x'
assert_status_key_field "H3 ?name.ne= — declared on key, not on name" 400 'SchemaViolationNotification' 'name.ne'
req GET '/roles?description.eq=x'
assert_status_key_field "H4 ?description.eq= — contains/icontains only" 400 'SchemaViolationNotification' 'description.eq'
req GET '/roles?tenantStatus.contains=act'
assert_status_key_field "H5 ?tenantStatus.contains= — an enum declares eq/in alone" 400 'SchemaViolationNotification' 'tenantStatus.contains'
req GET '/roles?search=billing'
assert_status_key_field "H6 a reserved control the DTO never declared" 400 'SchemaViolationNotification' 'search'

# ── H7, the CHILD half of the join contract ────────────────────────────────
note "the root join is filterable (G5/G6); the CHILD join is not, and neither is"
note "anything else inside the collection. Filtering a root by a field of a 1:N"
note "child is a pushdown one root SELECT cannot express — the 1:N boundary."
req GET '/roles?permissions.resource.eq=tenant'
assert_status_key "H7a filtering by the CHILD join's field" 400 'SchemaViolationNotification'
req GET '/roles?permissions.permission.eq=tenant:read'
assert_status_key "H7b filtering by the entry's COMPUTED field" 400 'SchemaViolationNotification'
req GET '/roles?permissions.permissionID.eq=00000000-0000-7000-8000-000000000999'
assert_status_key "H7c filtering by the entry's own STORED column" 400 'SchemaViolationNotification'

req GET '/roles?orderBy=description'
assert_status_key_field "H8 ?orderBy=description — filterable, orderable in neither direction" 400 'SchemaViolationNotification' 'orderBy[description]'
req GET '/roles?orderBy=permissions.permission'
assert_status_key "H9 ?orderBy= inside the collection — the 1:N boundary on the sort side" 400 'SchemaViolationNotification'
req GET '/roles?orderBy=id'
assert_status_key_field "H10 ?orderBy=id — declarable and deliberately not declared" 400 'SchemaViolationNotification' 'orderBy[id]'
req GET '/roles?orderBy=bogus'
assert_status_key_field "H11 ?orderBy= on an unknown token" 400 'SchemaViolationNotification' 'orderBy[bogus]'

# The rules-only join fields, read from the third side: not filterable (H7a), not
# orderable, and not SELECTABLE either. All three are the same `hidden`.
req GET '/roles?fields=permissions.resource'
assert_status_key "H12a ?fields=permissions.resource — hidden, feeding the derivation, never selectable" 400 'SchemaViolationNotification'
req GET '/roles?fields=permissions.action'
assert_status_key "H12b ?fields=permissions.action — same" 400 'SchemaViolationNotification'
req GET '/roles?fields=bogus'
assert_status_key_field "H13a ?fields= on an unknown path" 400 'SchemaViolationNotification' 'fields[bogus]'
req GET '/roles?fields=deletedAt'
assert_status_key "H13b ?fields=deletedAt — the archive stamp is not projected" 400 'SchemaViolationNotification'

req GET '/roles?first=101'
assert_status_key "H14 ?first= above the resolved ceiling" 400 'LimitExceededNotification'
assert_jq "H14 the ceiling reported is the framework default" '.errors[0].messages[0].value' '100'
for bad in 0 abc -5; do
  req GET "/roles?first=${bad}"
  assert_status_key_field "H15 ?first=${bad}" 400 'SchemaViolationNotification' 'first'
done

req GET '/roles?first=2&last=2'   ; assert_status "H16a ?first= with ?last="   400
req GET '/roles?first=2&before=abc'; assert_status "H16b ?first= with ?before=" 400
req GET '/roles?last=2&after=abc' ; assert_status "H16c ?last= with ?after="   400
req GET '/roles?after=abc&before=def'; assert_status "H16d ?after= with ?before=" 400

for conflict in 'first=10' 'orderBy=key' 'fields=key' 'after=abc'; do
  req GET "/roles?onlyTotal=true&${conflict}"
  assert_status_key "H17 ?onlyTotal=true beside ${conflict}" 400 'SchemaViolationNotification'
done
req GET "/roles?onlyTotal=true&key.startswith=${G_PFX}"
assert_status "H18a ?onlyTotal=true beside a FILTER is valid — counting a subset is the point" 200
req GET "/roles?onlyTotal=true&includeArchived=true"
assert_status "H18b ?onlyTotal=true beside ?includeArchived= is valid too" 200

req GET '/roles?after=not-a-cursor'
assert_status_key_field "H19 a malformed cursor" 400 'SchemaViolationNotification' 'after'

req GET "/roles?key.startswith=${G_PFX}&first=2"
NOORDER_CURSOR="$(j '.pagination.endCursor')"
req GET "/roles?key.startswith=${G_PFX}&first=2&after=${NOORDER_CURSOR}&orderBy=key"
assert_status "H20 a cursor replayed under a different orderBy — the structural check" 400
req GET "/roles?key.startswith=${G_PFX}&first=2&after=${NOORDER_CURSOR}&includeArchived=true"
assert_status "H21 the same cursor replayed under a different archive scope — the context hash" 400

req GET '/roles?includeArchived=1'
assert_status_key_field "H22a booleans take exactly true/false" 400 'SchemaViolationNotification' 'includeArchived'
req GET '/roles?onlyTotal='
assert_status_key_field "H22b an empty boolean is still a boolean" 400 'SchemaViolationNotification' 'onlyTotal'

# The by-id endpoint declares includeArchived and NOTHING else. Presence gates.
req GET "/roles/${ID_C}?onlyTotal=false"
assert_status_key "H23a by-id · an undeclared control, present and false" 400 'SchemaViolationNotification'
req GET "/roles/${ID_C}?fields=key"
assert_status_key "H23b by-id · ?fields= is not declared there" 400 'SchemaViolationNotification'
req GET "/roles/${ID_C}?includeArchived=true"
assert_status "H24 by-id · the ONE control it does declare" 200

req GET '/roles?createdAt.gte=not-a-date'
assert_status_key "H25 a filter VALUE outside the leaf's kind (pin >= v0.70.0)" 400 'InvalidFilterValueNotification'
req GET '/roles?tenantID.eq=lixo'
assert_status_key "H26 an identity column that declares eq, given junk" 400 'InvalidFilterValueNotification'

# ════════════════════════════════════════════════════════════════════════════
section "I · routing, not-found, and the by-id ADDRESS contract"

ABSENT_ID='00000000-0000-7000-8000-000000000999'

req GET "/roles/${ABSENT_ID}"
assert_status_key "I1 by-id on an unused uuid" 404 'RecordNotFoundNotification'
req DELETE "/roles/${ID_C}"
assert_status "I2 DELETE — no hard delete exists on this aggregate" 405
req POST "/roles/${ID_C}"
assert_status "I3 POST on the by-id path" 405
req GET "/roles/${ID_C}/archive"
assert_status "I4 GET on a path registered only under PATCH" 405
req PATCH "/roles/${ID_C}/unarchive" ''
assert_status_key "I5 PATCH on a path no route registers at all" 404 'RouteNotFoundNotification'
# A plain shell comparison, NOT assert_jq_true: the body of a routing refusal is
# not guaranteed to be JSON, and an assertion that pipes it through jq would fail
# for the parser rather than for the contract.
req DELETE "/roles/${ID_C}/permissions/${ABSENT_ID}"
if [ "$RESP_CODE" != "204" ]; then
  pass "I6 DELETE on the child path is never a 204 (answered ${RESP_CODE}) — a soft removal must not hide behind DELETE"
else
  fail "I6 DELETE on the child path is never a 204" "404 or 405" "HTTP 204" "$RESP_BODY"
fi
req POST "/roles/${ABSENT_ID}/permissions" "$(jq -nc --arg p "$PID_USER_READ" '{permissionID:$p}')"
assert_status_key "I7 GRANT onto a role id that addresses nothing" 404 'RecordNotFoundNotification'

# The by-id ADDRESS family, split by VERB — a read names no record, a write is a
# schema violation. Before pin v0.70.0 a relational backing answered 500 here.
req GET '/roles/not-a-uuid'
assert_status_key "I8 GET /roles/not-a-uuid — a READ names no record" 404 'UnknownIDAddressNotification'
req PATCH '/roles/not-a-uuid' '{"name":"Qa Never Applied"}'
assert_status_key "I9 PATCH /roles/not-a-uuid — a WRITE is a schema violation" 400 'MalformedIDNotification'
req PATCH '/roles/not-a-uuid/archive' ''
assert_status_key "I10 PATCH /roles/not-a-uuid/archive" 400 'MalformedIDNotification'
req POST '/roles/not-a-uuid/permissions' "$(jq -nc --arg p "$PID_USER_READ" '{permissionID:$p}')"
assert_status_key "I11 POST /roles/not-a-uuid/permissions" 400 'MalformedIDNotification'
gql '{ role(id: "not-a-uuid") { id } }'
assert_gql_key "I12 the GraphQL read follows the same split" 'UnknownIDAddressNotification'
gql 'mutation { archiveRole(id: "not-a-uuid") { success } }'
assert_gql_key "I13 and so does the GraphQL write" 'MalformedIDNotification'

# ── I14, the CHILD address, which is NOT that contract ─────────────────────
note "ArchiveRolePermissionRequest.RolePermissionID is a plain string path field,"
note "not a domain.ID — so the framework's wire wrapper never inspects it. The"
note "value reaches Role.RemoveRolePermissionByID, which answers the canonical"
note "not-found for any id the collection does not carry. Two ids in one URL,"
note "answering by two different rules, is exactly why this deserves a case."
req PATCH "/roles/${ID_C}/permissions/not-a-uuid/archive" '{}'
assert_status_key "I14 a malformed CHILD id is a not-found, not a malformed-id" 404 'RecordNotFoundNotification'

skip "I15 the mode-missing-with-route-mounted 403 arm" \
     "N/A: unarchive is absent from Modes() AND unmounted, so it lands on the 404 arm (I5). No route is mounted for any undeclared mode."

# ════════════════════════════════════════════════════════════════════════════
section "J · GraphQL — handler invariance across both surfaces"

req GET "/roles?key.startswith=${G_PFX}&orderBy=key&first=2"
REST_FIRST_KEY="$(j '.data[0].key')"
# Ordering on GraphQL is a TYPED input over the reflected <Entity>OrderField
# enum — orderBy: [{field: KEY, direction: ASC}] — not the REST string. Same
# sortable allowlist, same fold, same cursors; only the wire dialect changes
# (graphql.html, "Sorting").
gql "{ roles(first: 2, orderBy: [{field: KEY, direction: ASC}], where: { key: { startswith: \"${G_PFX}\" } }) { totalCount edges { cursor node { id key name tenantWorkspace } } pageInfo { hasNextPage endCursor } } }"
if [ "$(j '.errors // empty')" = "" ] && [ "$RESP_CODE" = "200" ]; then
  assert_jq "J1 the GraphQL connection returns the same first node as REST" '.data.roles.edges[0].node.key' "$REST_FIRST_KEY"
  assert_jq "J1 and the same total"  '.data.roles.totalCount' '4'
else
  fail "J1 the GraphQL connection mirrors the REST listing" "a connection with edges, pageInfo and totalCount" "errors present" "$RESP_BODY"
fi

req GET "/roles/${ID_C}"
REST_DOC="$(j '{key:.data.key, name:.data.name, tenantWorkspace:.data.tenantWorkspace, tenantStatus:.data.tenantStatus, permission:.data.permissions[0].permission}')"
gql "{ role(id: \"${ID_C}\") { key name tenantWorkspace tenantStatus permissions { permission } } }"
GQL_DOC="$(j '{key:.data.role.key, name:.data.role.name, tenantWorkspace:.data.role.tenantWorkspace, tenantStatus:.data.role.tenantStatus, permission:.data.role.permissions[0].permission}')"
[ -n "$REST_DOC" ] && [ "$REST_DOC" = "$GQL_DOC" ] \
  && pass "J2 the GraphQL by-id equals the REST by-id field for field" \
  || fail "J2 the GraphQL by-id equals the REST by-id field for field" "$REST_DOC" "$GQL_DOC" "$RESP_BODY"

K_J="${PG}-j3"
gql "mutation { createRole(input: { key: \"${K_J}\", name: \"Qa Graph Role\", description: \"A role created over GraphQL to prove the write reaches the same handler.\", tenantID: \"${T_MAIN}\", permissions: [{ permissionID: \"${PID_TENANT_READ}\" }] }) { id } }"
assert_gql_ok "J3 createRole"
ID_J="$(j '.data.createRole.id')"
req GET "/roles?key.eq=${K_J}"
assert_jq "J3 and it is visible over REST immediately" '.data | length' 1

gql "mutation { patchRole(id: \"${ID_J}\", input: { name: \"Qa Graph Renamed\" }) { id name } }"
assert_gql_ok "J4 patchRole"
req GET "/roles/${ID_J}"
assert_jq "J4 and the change is visible over REST" '.data.name' 'Qa Graph Renamed'

gql "mutation { addRolePermission(id: \"${ID_J}\", input: { permissionID: \"${PID_ROLE_READ}\" }) { roleId rolePermission { id permissionID } } }"
assert_gql_ok "J6 addRolePermission"
CHILD_J="$(j '.data.addRolePermission.rolePermission.id')"
req GET "/roles/${ID_J}"
assert_jq "J6 and the grant is visible over REST" '.data.permissions | length' 2

gql "mutation { archiveRolePermission(id: \"${ID_J}\", input: { rolePermissionId: \"${CHILD_J}\" }) { success } }"
assert_gql_ok "J7 archiveRolePermission"
req GET "/roles/${ID_J}"
assert_status "J7 the ROOT IS STILL ACTIVE over REST — the GraphQL half of B6" 200
assert_jq "J7 and only the revoked entry left" '.data.permissions | length' 1

gql "mutation { archiveRole(id: \"${ID_J}\") { success } }"
assert_gql_ok "J5 archiveRole"
req GET "/roles/${ID_J}"
assert_status_key "J5 and the effect is visible over REST" 404 'RecordNotFoundNotification'

gql "{ role(id: \"${ABSENT_ID}\") { id } }"
assert_gql_key "J8 a by-id on an unused uuid carries the typed key in extensions" 'RecordNotFoundNotification'

gql "mutation { createRole(input: { key: \"${K_C}\", name: \"Qa Graph Duplicate\", description: \"A duplicate key sent over GraphQL, which must answer the same conflict.\", tenantID: \"${T_MAIN}\", permissions: [] }) { id } }"
assert_gql_key "J9 a duplicate key over GraphQL answers the same conflict" 'RoleKeyAlreadyExistsNotification'
assert_jq "J9 with the same semantic" '.errors[0].extensions.semantic' 'Conflict'

gql '{ roles(search: "billing") { totalCount } }'
assert_gql_error_matching "J10 ?search= has no analogue here — an unknown ARGUMENT, not the REST envelope" 'search'

gql "{ roles(first: 4, orderBy: [{field: TENANT_WORKSPACE, direction: ASC}], where: { tenantWorkspace: { eq: \"${T_MAIN_WS}\" }, key: { startswith: \"${G_PFX}\" } }) { edges { node { key tenantWorkspace } } } }"
assert_gql_ok "J11a the ROOT JOIN is queryable and orderable on GraphQL too"
assert_jq "J11b and the filter really selected on it" '.data.roles.edges[0].node.tenantWorkspace' "$T_MAIN_WS"

# The sort allowlist is ONE declaration read from two sides: the REST ?orderBy=
# vocabulary and the reflected enum have to be the same seven fields, or a
# capability exists on one surface alone.
gql '{ __type(name: "RoleOrderField") { enumValues { name } } }'
assert_jq "J11c the reflected order enum is exactly the REST sort vocabulary" \
  '[.data.__type.enumValues[].name] | sort | join(",")' \
  'CREATED_AT,KEY,NAME,TENANT_ID,TENANT_STATUS,TENANT_WORKSPACE,UPDATED_AT'

gql "{ __typename role(id: \"${ID_C}\") { __typename key permissions { __typename permission } } }"
assert_gql_ok "J12 __typename beside every selection answers identically (pin >= v0.72.1)"
assert_jq "J12 and the document itself is unchanged" '.data.role.permissions[0].permission' 'tenant:read'

gql "{ role(id: \"${ID_C}\") { permissions { resource } } }"
assert_gql_error_matching "J13a selecting the child join's resource is an unknown FIELD" 'resource'
gql "{ role(id: \"${ID_C}\") { permissions { action } } }"
assert_gql_error_matching "J13b selecting its action likewise" 'action'

gql "mutation { unarchiveRole(id: \"${ID_C}\") { success } }"
assert_gql_error_matching "J14 unarchiveRole is not a field in the schema" 'unarchiveRole'

skip "gRPC parity" "N/A — no transport: block, so no gRPC surface is wired."
skip "tabular exports" "N/A — spec.md §9 declares none for this entity."

# ════════════════════════════════════════════════════════════════════════════
section "X · suite meta — the document and the mount must agree"

req GET /openapi.json
X_ROUTES="$(j '[ .paths | to_entries[] | select(.key | startswith("/roles"))
                 | .value | to_entries[] | .key ] | length')"
if [ "$X_ROUTES" = "7" ]; then
  pass "X1 /openapi.json enumerates exactly the SEVEN role routes"
else
  fail "X1 /openapi.json enumerates exactly the SEVEN role routes" "7 method+path pairs under /roles" "${X_ROUTES}" \
       "$(j '[ .paths | to_entries[] | select(.key | startswith("/roles")) | {p:.key, m:(.value | keys)} ]')"
fi
X_PERMS="$(j '[ .paths | to_entries[] | select(.key | startswith("/roles"))
                | .value | to_entries[] | .value.description // ""
                | scan("\\*\\*Required permission:\\*\\* `([^`]+)`") ] | flatten | sort | unique | join(",")')"
X_GATED="$(j '[ .paths | to_entries[] | select(.key | startswith("/roles"))
                | .value | to_entries[] | .value.description // ""
                | select(test("\\*\\*Required permission:\\*\\*")) ] | length')"
if [ "$X_GATED" = "7" ]; then
  pass "X2 all seven declare a required permission — none was mounted open"
else
  fail "X2 all seven declare a required permission" "7 gated operations" "${X_GATED} gated" ''
fi
case "$X_PERMS" in
  'role:archive,role:grant,role:insert,role:read,role:update')
    pass "X3 the five declared literals are exactly the taxonomy spec.md §10 names" ;;
  *) fail "X3 the five declared literals are exactly the taxonomy spec.md §10 names" \
          'role:archive,role:grant,role:insert,role:read,role:update' "${X_PERMS:-<none>}" '' ;;
esac

lane_summary
