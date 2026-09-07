#!/usr/bin/env bash
# Lane: role — the REST half of the framework contract for the Role aggregate.
#
# Families H1-H11 of specs/qa/role-contract/plan.md §1. The J family (GraphQL) lives in
# qa/role_graphql.sh; the business rules live in qa/domain.sh (RL1-RL11); the gate lives in
# qa/security.sh (S5.x); the audit trail lives in qa/audit.sh (A24+).
#
# THE ONE IDEA THIS WHOLE LANE IS BUILT AROUND — one column goes in, four come out, and the
# two joins that make it so pull in OPPOSITE directions:
#
#   the ROOT join → Tenant          the CHILD join → Permission
#   ─────────────────────           ────────────────────────────
#   tenantWorkspace   served        resource               HIDDEN — feeds the derivation
#   tenantStatus      served        action                 HIDDEN — feeds the derivation
#   tenantArchivedAt  served        permissionArchivedAt   served
#   filterable + sortable           addressable in NO criteria — the 1:N boundary
#
# `permission` is derived from the two hidden halves and appears on the READ side only. Every
# case below is on one side of that table or the other, and says which.
#
# The lane runs as the bootstrap admin (*:*), so nothing here is blocked by row scope. §1b and
# §3 are where a scoped principal does the work.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init role

# ═════════════════════════════════════════════════════════════════════════════════════════
# Fixtures. Catalog ids are resolved by their PAIR, never hardcoded — a grant names the
# permission it means and the catalog says which row that is.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_LANE=$(new_tenant active "$(ws role)") || exit 1
TEN_OTHER=$(new_tenant active "$(ws roleb)") || exit 1

P_TENANT_READ=$(permission_id_of tenant read)
P_ROLE_READ=$(permission_id_of role read)
P_ROLE_GRANT=$(permission_id_of role grant)
P_PERM_READ=$(permission_id_of permission read)
for p in "$P_TENANT_READ" "$P_ROLE_READ" "$P_ROLE_GRANT" "$P_PERM_READ"; do
  [ -n "$p" ] || { echo "role.sh: a seeded catalog id could not be resolved — is migration 0012 applied?" >&2; exit 1; }
done

D_OK="A role description long enough to satisfy the shared anti-junk floor this service applies."

# ═════════════════════════════════════════════════════════════════════════════════════════
# H1 — the seven mounted routes. Modes() declares display, insert, update, archive: FOUR
#      modes, SEVEN routes, because the collection carries two verbs of its own.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_MAIN=$(role_key main)
case_ "H1.1 POST /roles" "201 — the record as stored, carrying both grants"
api POST /roles "$(role_body "$K_MAIN" "QA Main Role" "$D_OK" "$TEN_LANE" "$P_TENANT_READ" "$P_ROLE_READ")"
assert_json_at 201 '.data.permissions | length' "2"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "H1.1b the write response carries the stored column and NOT the derivation" "each entry is id + permissionID, and no 'permission' key"
assert_json '[.data.permissions[] | (keys | sort | join(","))] | unique | join(" / ")' "id,permissionID"

case_ "H1.2 GET /roles/{id}" "200 — the full document"
api GET "/roles/$ID_MAIN"
assert_json_at 200 '.data.key' "$K_MAIN"

case_ "H1.3 GET /roles" "200 — data + pagination, narrowed to this fixture"
api GET "/roles?key.eq=$K_MAIN"
assert_json_at 200 '.data | length' "1"

case_ "H1.4 PATCH /roles/{id}" "200 — the record after the change"
api PATCH "/roles/$ID_MAIN" '{"name":"QA Main Role, relabelled"}'
assert_json_at 200 '.data.name' "QA Main Role, relabelled"

case_ "H1.6 POST /roles/{id}/permissions — GRANT" "201, and the entry carries the id the server minted"
api POST "/roles/$ID_MAIN/permissions" "$(jq -nc --arg p "$P_PERM_READ" '{permissionID:$p}')"
assert_json_at 201 '.data.rolePermission.permissionID' "$P_PERM_READ"
CHILD_GRANTED=$(printf '%s' "$HTTP_BODY" | jq -r '.data.rolePermission.id')

case_ "H1.6b the GRANT response names the owner" "roleId is the role the entry was added to"
assert_json '.data.roleId' "$ID_MAIN"

case_ "H1.6c the grant is visible on the next read, IMMEDIATELY" "3 entries — relational backing, read-your-writes; a poll here would itself be a failure"
api GET "/roles/$ID_MAIN"
assert_json_at 200 '.data.permissions | length' "3"

# ── H1.7 — the model's canonical trap ─────────────────────────────────────────────────────
#
# spec.md §3 warns twice: the root-archive auto handler is instantiated exactly ONCE per
# surface, and wiring it to the child REVOKE route type-checks, boots and answers 204 — while
# archiving the entire role. So this case does not stop at the 204.
case_ "H1.7 PATCH /roles/{id}/permissions/{childId}/archive — REVOKE" "204 with NO BODY"
api PATCH "/roles/$ID_MAIN/permissions/$CHILD_GRANTED/archive"
assert_empty_body 204

case_ "H1.7a THE TRAP: the ROOT is still active after a revoke" "200 on the plain by-id read — a revoke that archived the root would answer 404 here"
api GET "/roles/$ID_MAIN"
assert_status 200

case_ "H1.7b the root's own archivedAt was not stamped" "null — the entry was archived, the aggregate was not"
assert_json '.data.archivedAt' "null"

case_ "H1.7c the revoked entry is gone and the others remain" "2 entries, and the revoked child id is not among them"
assert_json '[(.data.permissions | length), ([.data.permissions[].id] | index("'"$CHILD_GRANTED"'") == null)] | @csv' '2,true'

case_ "H1.5 PATCH /roles/{id}/archive" "204 with NO BODY — the framework's bodyless result"
ID_ARCHV=$(new_role "$(role_key arch)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
api PATCH "/roles/$ID_ARCHV/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# H2 — the golden record: every declared field, on all four read shapes.
#
# The three archive stamps entered the model on 2026-09-06. Here they are asserted NULL while
# their target is live; H7 is where they do their actual job.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_GOLD=$(role_key gold)
ID_GOLD=$(new_role "$K_GOLD" "$TEN_LANE" "$P_TENANT_READ") || exit 1

case_ "H2.1 the by-id document carries exactly the declared field set" "the twelve keys of FindRoleByIDResponse, no more and no fewer"
api GET "/roles/$ID_GOLD"
assert_json_at 200 '.data | keys | sort | join(",")' \
  "archivedAt,createdAt,description,id,key,name,permissions,tenantArchivedAt,tenantID,tenantStatus,tenantWorkspace,updatedAt"

case_ "H2.2 the root's own fields read back as written" "key, name and description, byte for byte"
assert_json '[.data.key, .data.name, .data.description] | join("|")' "$K_GOLD|QA Fixture Role|Fixture role created by the QA suite for run $(qa_slug_runid), bundling catalog permissions."

case_ "H2.3 tenantID is the owner the write named" "the lane's own tenant"
assert_json '.data.tenantID' "$TEN_LANE"

case_ "H2.4 the ROOT JOIN reached the wire" "tenantWorkspace and tenantStatus carry the owning tenant's actual values, read across the foreign key"
assert_json '[(.data.tenantWorkspace | length > 0), (.data.tenantStatus == "active")] | @csv' "true,true"

case_ "H2.5 createdAt is RFC3339" "the grammar, not jq's narrower fromdateiso8601"
assert_rfc3339 '.data.createdAt'

case_ "H2.6 updatedAt is RFC3339" "the same grammar"
assert_rfc3339 '.data.updatedAt'

case_ "H2.7 archivedAt is null while the role is active" "null — the stamp reports a state, it does not announce one"
assert_json '.data.archivedAt' "null"

case_ "H2.8 tenantArchivedAt is null while the OWNER is live" "null — added 2026-09-06, and this is its resting value"
assert_json '.data.tenantArchivedAt' "null"

case_ "H2.9 each grant entry carries the id, the stored column and the derivation" "id, permission, permissionID — the fourth value, permissionArchivedAt, is null on a live counterpart and the row DTO is omitempty, so it appears in H7 where it is stamped"
assert_json '[.data.permissions[] | (keys | sort | join(","))] | unique | join(" / ")' "id,permission,permissionID"

case_ "H2.10 the derivation renders the catalog pair" "tenant:read — rebuilt through vos.PermissionKey.String(), never concatenated"
assert_json '.data.permissions[0].permission' "tenant:read"

case_ "H2.11 permissionArchivedAt is null while the catalog row is live" "null"
assert_json '.data.permissions[0].permissionArchivedAt' "null"

case_ "H2.12 the LISTING row is the same document" "every asserted field again, read through the collection"
api GET "/roles?key.eq=$K_GOLD"
assert_json_at 200 '[.data[0].key, .data[0].tenantStatus, .data[0].permissions[0].permission] | join("|")' "$K_GOLD|active|tenant:read"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H3 — "one column in, four out": the child-join doctrine, BOTH halves.
#
# The negative half is the case nobody writes. `resource` and `action` are hidden: they feed
# the derivation and reach no response body on any surface. A rules-only join field that leaks
# is a silent regression every other family here passes over.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "H3.1 the positive half: the derivation is served per entry" "tenant:read on the entry, from two columns of another table"
api GET "/roles/$ID_GOLD"
assert_json_at 200 '.data.permissions[0].permission' "tenant:read"

case_ "H3.2 resource is ABSENT from every entry of the by-id read" "the key is not present — not present-and-null"
assert_absent '[.data.permissions[] | has("resource")] | any'

case_ "H3.3 action is ABSENT from every entry of the by-id read" "the key is not present"
assert_absent '[.data.permissions[] | has("action")] | any'

case_ "H3.4 neither reaches a LISTING row" "no entry on the page carries either key"
api GET "/roles?first=100"
assert_json_at 200 '[.data[].permissions[]? | (has("resource") or has("action"))] | any' "false"

case_ "H3.5 neither reaches the INSERT response" "the write side never saw them either"
K_H3=$(role_key hid)
api POST /roles "$(role_body "$K_H3" "QA Hidden Halves" "$D_OK" "$TEN_LANE" "$P_ROLE_READ")"
assert_json_at 201 '[.data.permissions[] | (has("resource") or has("action"))] | any' "false"
ID_H3=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "H3.6 nor the GRANT response" "id + permissionID, and nothing derived or hidden"
api POST "/roles/$ID_H3/permissions" "$(jq -nc --arg p "$P_TENANT_READ" '{permissionID:$p}')"
assert_json_at 201 '.data.rolePermission | keys | sort | join(",")' "id,permissionID"

case_ "H3.7 nor a row read with ?includeArchived=true" "the archived view hides nothing and reveals nothing new"
api GET "/roles/$ID_H3?includeArchived=true"
assert_json_at 200 '[.data.permissions[] | (has("resource") or has("action"))] | any' "false"

case_ "H3.8 the write side speaks the ID and never the render" "422 — a grant naming 'permission' supplies no permissionID"
api POST "/roles/$ID_H3/permissions" '{"permission":"tenant:read"}'
assert_rest 422 InvalidIDUUIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# H4 — validation, 422, the notification KEY and the FIELD.
# ═════════════════════════════════════════════════════════════════════════════════════════

h4() { # h4 CASE EXPECT JSON_OVERRIDES KEY [FIELD]
  case_ "$1" "$2"
  api POST /roles "$(jq -nc --arg k "$(role_key v)" --arg d "$D_OK" --arg t "$TEN_LANE" --argjson o "$3" \
    '{key:$k, name:"QA Validation Role", description:$d, tenantID:$t, permissions:[]} + $o')"
  if [ -n "${5:-}" ]; then assert_rest_field 422 "$4" "$5"; else assert_rest 422 "$4"; fi
}

h4 "H4.1 key empty"                 "422 RequiredFieldNotification on 'key' — the VO short-circuits on empty, so this is the FRAMEWORK's notification" '{"key":""}' RequiredFieldNotification key
h4 "H4.2 key uppercase"             "422 InvalidRoleKeyNotification — refused, never lowercased"                                  '{"key":"Billing"}'      InvalidRoleKeyNotification key
h4 "H4.3 key below the 2-rune floor" "422 InvalidRoleKeyNotification"                                                             '{"key":"a"}'            InvalidRoleKeyNotification key
h4 "H4.4 key with a doubled hyphen" "422 InvalidRoleKeyNotification — groups are separated by SINGLE hyphens"                      '{"key":"bil--ling"}'    InvalidRoleKeyNotification key
h4 "H4.5 key with a leading hyphen" "422 InvalidRoleKeyNotification — a hyphen can never lead"                                     '{"key":"-billing"}'     InvalidRoleKeyNotification key
h4 "H4.6 key with a trailing hyphen" "422 InvalidRoleKeyNotification — nor trail"                                                  '{"key":"billing-"}'     InvalidRoleKeyNotification key
h4 "H4.7 key with a run of 4 identical runes" "422 InvalidRoleKeyNotification — the shared anti-junk predicate"                     '{"key":"aaaab"}'        InvalidRoleKeyNotification key
K65="$(printf 'ab%.0s' $(seq 1 32))a"   # 65 runes, a valid slug, no run of 4 — only the LENGTH is wrong
h4 "H4.8 key of 65 runes"           "422 InvalidRoleKeyNotification — the ceiling is 64"                                            "$(jq -nc --arg k "$K65" '{key:$k}')" InvalidRoleKeyNotification key
h4 "H4.9 name empty"                "422 RequiredFieldNotification on 'name'"                                                       '{"name":""}'            RequiredFieldNotification name
h4 "H4.10 name below the DisplayName floor" "422 InvalidDisplayNameNotification"                                                    '{"name":"a"}'           InvalidDisplayNameNotification name
h4 "H4.11 description under the floor" "422 InvalidDescriptionNotification — 15 runes, 2 words, 5 distinct runes, a vowel"           '{"description":"short"}' InvalidDescriptionNotification description
h4 "H4.12 a grant entry whose id is junk" "422 InvalidIDUUIDNotification — the framework validates the child's own id"               "$(jq -nc '{permissions:[{permissionID:"tatu"}]}')" InvalidIDUUIDNotification

case_ "H4.13 the positive control: every boundary satisfied" "201 — a leading digit, a hyphenated slug and an accented description are all ordinary"
api POST /roles "$(role_body "3d-assets-$(qa_slug_runid)" "QA Positive Control" "Concede leitura ao registro de inquilinos e ao catálogo, sem nenhum verbo de escrita." "$TEN_LANE" "$P_TENANT_READ")"
assert_status 201

# ── the guard barrier, which is a family of its own ───────────────────────────────────────
#
# TenantID carries a `valueObject` rule with guard: true, so domain.ID.IsValid runs FIRST and
# r.StopIfInvalid() ends the whole pass. Without it the bad owner reached the uniqueness
# pre-check, bound to a UUID column, and the probe panicked into a 500 on a request whose
# problem is plain validation. That bug actually shipped once.

case_ "H4.g1 tenantID empty" "422 InvalidIDUUIDNotification — uuid.Parse refuses the empty string and junk through the SAME call"
api POST /roles "$(jq -nc --arg k "$(role_key g)" --arg d "$D_OK" '{key:$k, name:"QA Guard", description:$d, tenantID:"", permissions:[]}')"
assert_rest 422 InvalidIDUUIDNotification

case_ "H4.g2 tenantID malformed" "422 InvalidIDUUIDNotification — the same key, which is why one rule replaces a `required` one"
api POST /roles "$(jq -nc --arg k "$(role_key g)" --arg d "$D_OK" '{key:$k, name:"QA Guard", description:$d, tenantID:"tatu", permissions:[]}')"
assert_rest 422 InvalidIDUUIDNotification

case_ "H4.g3 THE BARRIER: a bad owner beside a second violation reports the OWNER ALONE" "exactly one distinct notification key — the pass ends, including the automatic value-object validation and every collection"
api POST /roles "$(jq -nc --arg k "$(role_key g)" '{key:$k, name:"QA Guard", description:"short", tenantID:"tatu", permissions:[]}')"
assert_json_at 422 '[.errors[].messages[].notificationKey] | unique | join(",")' "InvalidIDUUIDNotification"

case_ "H4.g4 REGRESSION GUARD: the barrier names the field camelCased" "field 'tenantID' — at v0.72.1 this path emitted 'TenantID' through the legacy slot while every other field in the same envelope was folded"
assert_rest_field 422 InvalidIDUUIDNotification tenantID

# ═════════════════════════════════════════════════════════════════════════════════════════
# H5 — the 409 family, and what "unique within the tenant" actually means.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_DUP=$(role_key dup)
ID_DUP=$(new_role "$K_DUP" "$TEN_LANE" "$P_TENANT_READ") || exit 1

case_ "H5.1 the same key twice in ONE tenant" "409 RoleKeyAlreadyExistsNotification"
api POST /roles "$(role_body "$K_DUP" "QA Duplicate" "$D_OK" "$TEN_LANE")"
assert_rest 409 RoleKeyAlreadyExistsNotification

case_ "H5.1b the refusal names the key and carries the Conflict semantic" "field 'key', semantic 'Conflict'"
assert_json '[.errors[].messages[] | select(.notificationKey=="RoleKeyAlreadyExistsNotification") | .field] | join(",")' "key"

case_ "H5.2 THE CONTROL: the same key in a DIFFERENT tenant" "201 — the constraint is (tenant_id, role_key), and without this case an over-broad GLOBAL index would pass H5.1 just as well"
api POST /roles "$(role_body "$K_DUP" "QA Duplicate Elsewhere" "$D_OK" "$TEN_OTHER")"
assert_status 201

case_ "H5.3 a patch that does not move the key does not self-collide" "200 — excludeSelf, so a row never conflicts with itself"
api PATCH "/roles/$ID_DUP" '{"name":"QA Duplicate, relabelled"}'
assert_status 200

case_ "H5.4 archive a role, then insert the SAME key again" "201 — unique.scope is active-only, and with no unarchive verb this is the only route back"
api PATCH "/roles/$ID_DUP/archive"
api POST /roles "$(role_body "$K_DUP" "QA Duplicate Reborn" "$D_OK" "$TEN_LANE")"
assert_status 201
ID_DUP2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "H5.4b and it comes back with a NEW id" "a different id — the archived row keeps its own history, and nothing re-attaches to it"
if [ -n "$ID_DUP2" ] && [ "$ID_DUP2" != "$ID_DUP" ]; then pass_; else fail_ "id '$ID_DUP2' vs '$ID_DUP'"; fi

case_ "H5.5 the same permissionID twice inside ONE insert body" "409 RoleAlreadyGrantsPermissionNotification — the pair is never stored twice"
api POST /roles "$(role_body "$(role_key dupg)" "QA Duplicate Grant" "$D_OK" "$TEN_LANE" "$P_TENANT_READ" "$P_TENANT_READ")"
assert_rest 409 RoleAlreadyGrantsPermissionNotification

case_ "H5.6 GRANT a permission the role ALREADY holds" "409 — and this is where IsSameBusinessIdentity over PermissionID ALONE is load-bearing: the entry carries four fields and three are join fields blank on a freshly added entry, so a compare-all-fields identity would answer 'different' and fail OPEN"
api POST "/roles/$ID_GOLD/permissions" "$(jq -nc --arg p "$P_TENANT_READ" '{permissionID:$p}')"
assert_rest 409 RoleAlreadyGrantsPermissionNotification

ID_REGR=$(new_role "$(role_key regr)" "$TEN_LANE" "$P_TENANT_READ") || exit 1
api GET "/roles/$ID_REGR"
CHILD_REGR=$(printf '%s' "$HTTP_BODY" | jq -r '.data.permissions[0].id')
case_ "H5.7 REVOKE a grant, then GRANT the same permission again" "201 — the child index is active-only and there is no per-entry unarchive, so a fresh add is the only way back"
api PATCH "/roles/$ID_REGR/permissions/$CHILD_REGR/archive"
api POST "/roles/$ID_REGR/permissions" "$(jq -nc --arg p "$P_TENANT_READ" '{permissionID:$p}')"
assert_status 201
CHILD_REGR2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.rolePermission.id')

case_ "H5.7b and the re-grant carries a NEW child id" "a different entry id — which is what reads correctly in the audit trail"
if [ -n "$CHILD_REGR2" ] && [ "$CHILD_REGR2" != "$CHILD_REGR" ]; then pass_; else fail_ "child id '$CHILD_REGR2' vs '$CHILD_REGR'"; fi

case_ "H5.8 the wrong-state 409 is UNREACHABLE on this aggregate" "recorded as a DERIVATION, and deliberately not exercised"
skip_ "EntityIsNotActiveNotification / ConcurrentModificationNotification need a verb this surface does not mount: there is no PUT, no wire field carries a revision, and every wrong-state attempt is intercepted a layer earlier by LoadForWrite's ScopeActive, where it lands as 404 (H6). Asserting a 409 there would encode a promise the pin does not make"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H6 — archive, which on this entity is a ONE-WAY DOOR.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_ARC=$(role_key oneway)
ID_ARC=$(new_role "$K_ARC" "$TEN_LANE" "$P_TENANT_READ") || exit 1
api PATCH "/roles/$ID_ARC/archive"

case_ "H6.1 the archived role is gone from the plain by-id read" "404 RecordNotFoundNotification"
api GET "/roles/$ID_ARC"
assert_rest 404 RecordNotFoundNotification

case_ "H6.2 ?includeArchived=true reveals it, with the stamp" "200 and a non-null archivedAt"
api GET "/roles/$ID_ARC?includeArchived=true"
assert_json_at 200 '.data.archivedAt != null' "true"

case_ "H6.2b and its GRANTS are still readable" "1 entry — a retired role must stay auditable, and with no unarchive verb this is the only way to see one"
assert_json '.data.permissions | length' "1"

case_ "H6.3 the listing hides it" "0 rows"
api GET "/roles?key.eq=$K_ARC"
assert_json_at 200 '.data | length' "0"

case_ "H6.4 the listing reveals it with ?includeArchived=true" "1 row"
api GET "/roles?key.eq=$K_ARC&includeArchived=true"
assert_json_at 200 '.data | length' "1"

case_ "H6.5 archiving an ALREADY-archived role" "404 — LoadForWrite runs the default ScopeActive and does not see it"
api PATCH "/roles/$ID_ARC/archive"
assert_rest 404 RecordNotFoundNotification

case_ "H6.6 PATCHing an archived role" "404 — same scope"
api PATCH "/roles/$ID_ARC" '{"name":"QA Archived Relabel"}'
assert_rest 404 RecordNotFoundNotification

case_ "H6.7 GRANT onto an archived role" "404 — the collection verb loads the root the same way"
api POST "/roles/$ID_ARC/permissions" "$(jq -nc --arg p "$P_ROLE_READ" '{permissionID:$p}')"
assert_rest 404 RecordNotFoundNotification

api GET "/roles/$ID_ARC?includeArchived=true"
CHILD_ARC=$(printf '%s' "$HTTP_BODY" | jq -r '.data.permissions[0].id')
case_ "H6.8 REVOKE on an archived role" "404 — same reason, from the other collection verb"
api PATCH "/roles/$ID_ARC/permissions/$CHILD_ARC/archive"
assert_rest 404 RecordNotFoundNotification

case_ "H6.9 the child stamp-scoped unarchive family" "recorded as N/A, and deliberately not exercised"
skip_ "The family proves that a child removed on its own BEFORE the root's archive stays archived after the root comes back. This root never comes back: no unarchive mode, no route, no mutation. There is no restore for a stamp to be scoped to"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H7 — the three archive stamps, and what each one REPORTS.
#
# These fields entered the model on 2026-09-06 and their entire job is to report a state the
# caller could not otherwise see. `?includeArchived` governs which ROOTS a read returns, never
# the rows a traversal reaches ACROSS into — so before permissionArchivedAt existed, a grant
# pointing at a retired catalog row arrived here silently, indistinguishable from a live one.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_STAMP=$(pair_resource stamp)
P_STAMP=$(new_permission "$R_STAMP" read "A catalog entry the role lane retires on purpose, to prove what a grant reports afterwards.") || exit 1
K_STAMP=$(role_key stamp)
ID_STAMP=$(new_role "$K_STAMP" "$TEN_LANE" "$P_STAMP") || exit 1

case_ "H7.1 while the catalog row is live" "permission rendered, permissionArchivedAt null"
api GET "/roles/$ID_STAMP"
assert_json_at 200 '[.data.permissions[0].permission, (.data.permissions[0].permissionArchivedAt // "null")] | join("|")' "$R_STAMP:read|null"

api PATCH "/permissions/$P_STAMP/archive"

case_ "H7.2 after the catalog row is ARCHIVED, the entry is STILL SERVED" "1 entry — an inner join is not gated on the archived state of its target"
api GET "/roles/$ID_STAMP"
assert_json_at 200 '.data.permissions | length' "1"

case_ "H7.2b and it still renders the same token" "$R_STAMP:read — an access review has to be able to read what a past grant MEANT"
assert_json '.data.permissions[0].permission' "$R_STAMP:read"

case_ "H7.2c and permissionArchivedAt is now STAMPED" "a non-null timestamp — the entry's only account of a grant that outlived what it points at"
assert_json '.data.permissions[0].permissionArchivedAt != null' "true"

case_ "H7.2d the stamp is RFC3339" "the grammar, over a *time.Time reached across the foreign key"
assert_rfc3339 '.data.permissions[0].permissionArchivedAt'

case_ "H7.2e the LISTING row reports it identically" "the same stamp through the collection"
api GET "/roles?key.eq=$K_STAMP"
assert_json_at 200 '.data[0].permissions[0].permissionArchivedAt != null' "true"

case_ "H7.4 and the RULES did not move" "200 — the role is still PATCHable by name; a rule reading PermissionArchivedAt instead of the probe would fail open here (cross-ref RL9)"
api PATCH "/roles/$ID_STAMP" '{"name":"QA Stamp Role, still writable"}'
assert_status 200

# ── the OWNER's stamp. On a tenant of the lane's own — never on master ────────────────────
TEN_DOOMED=$(new_tenant active "$(ws doomed)") || exit 1
ID_DOOMED=$(new_role "$(role_key doomed)" "$TEN_DOOMED" "$P_TENANT_READ") || exit 1

case_ "H7.5 while the OWNER is live" "tenantArchivedAt null, tenantStatus active"
api GET "/roles/$ID_DOOMED"
assert_json_at 200 '[(.data.tenantArchivedAt // "null"), .data.tenantStatus] | join("|")' "null|active"

api PATCH "/tenants/$TEN_DOOMED/archive"

case_ "H7.5b after the OWNER is archived, the role is STILL READABLE" "200 — the ROOT join keeps matching its archived counterpart too"
api GET "/roles/$ID_DOOMED"
assert_status 200

case_ "H7.5c tenantArchivedAt is STAMPED" "a non-null timestamp reached across tenant_id"
assert_json '.data.tenantArchivedAt != null' "true"

case_ "H7.5d and tenantWorkspace / tenantStatus are still served" "the handle is still there, and archiving forced the commercial state to suspended"
assert_json '[(.data.tenantWorkspace | length > 0), (.data.tenantStatus == "suspended")] | @csv' "true,true"

case_ "H7.6 Role's own archivedAt is TERMINAL" "stamped after archive, and no verb exists to return it to null"
api PATCH "/roles/$ID_DOOMED/archive"
api GET "/roles/$ID_DOOMED?includeArchived=true"
assert_json_at 200 '.data.archivedAt != null' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H8 — the read vocabulary: everything the DTO declares, and only that.
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_FILT=$(ws filt)
TEN_FILT=$(new_tenant active "$WS_FILT") || exit 1
K_FILT=$(role_key filter)
ID_FILT=$(new_role "$K_FILT" "$TEN_FILT" "$P_TENANT_READ") || exit 1
K_PREFIX="${K_FILT%-*}"

f() { # f CASE EXPECT QUERY [EXPECTED_COUNT]
  case_ "$1" "$2"
  api GET "/roles?$3"
  assert_json_at 200 '.data | length' "${4:-1}"
}

f "H8.1 ?key.eq"          "the exact row"                       "key.eq=$K_FILT"
f "H8.2 ?key.ne"          "the row is excluded by its own key"   "key.ne=$K_FILT&key.startswith=$K_FILT" 0
f "H8.3 ?key.in"          "membership over a set"                "key.in=$K_FILT,nothing-at-all"
f "H8.4 ?key.startswith"  "the prefix family"                    "key.eq=$K_FILT&key.startswith=$K_PREFIX"
f "H8.5 ?key.istartswith" "case-insensitively"                   "key.eq=$K_FILT&key.istartswith=$(printf '%s' "$K_PREFIX" | tr 'a-z' 'A-Z')"
f "H8.6 ?key.contains"    "an infix"                             "key.eq=$K_FILT&key.contains=filter"
f "H8.7 ?key.icontains"   "an infix, case-insensitively"          "key.eq=$K_FILT&key.icontains=FILTER"
f "H8.8 ?name.eq"         "the display name is filterable"        "key.eq=$K_FILT&name.eq=QA%20Fixture%20Role"
f "H8.9 ?name.in"         "membership"                            "key.eq=$K_FILT&name.in=QA%20Fixture%20Role,Nothing"
f "H8.10 ?name.startswith" "prefix"                               "key.eq=$K_FILT&name.startswith=QA"
f "H8.11 ?name.istartswith" "prefix, case-insensitively"           "key.eq=$K_FILT&name.istartswith=qa"
f "H8.12 ?name.contains"  "infix"                                 "key.eq=$K_FILT&name.contains=Fixture"
f "H8.13 ?name.icontains" "infix, case-insensitively"              "key.eq=$K_FILT&name.icontains=fixture"
f "H8.14 ?description.contains"  "the description filters"        "key.eq=$K_FILT&description.contains=bundling"
f "H8.15 ?description.icontains" "case-insensitively"             "key.eq=$K_FILT&description.icontains=BUNDLING"
f "H8.16 ?tenantID.eq"    "the owner by primary key"              "key.eq=$K_FILT&tenantID.eq=$TEN_FILT"
f "H8.17 ?tenantID.in"    "membership over owners"                "key.eq=$K_FILT&tenantID.in=$TEN_FILT,$TEN_LANE"

# ── the ROOT JOIN reaching a criteria: this round's headline capability ───────────────────
f "H8.18 ?tenantWorkspace.eq — THE ROOT JOIN, FILTERABLE"  "'the roles of a workspace', answered without a second call, over a column in ANOTHER table" "tenantWorkspace.eq=$WS_FILT"
f "H8.19 ?tenantWorkspace.in"          "membership over the join field"      "tenantWorkspace.in=$WS_FILT,nothing"
f "H8.20 ?tenantWorkspace.startswith"  "prefix over the join field"          "tenantWorkspace.startswith=$WS_FILT"
f "H8.21 ?tenantWorkspace.istartswith" "prefix, case-insensitively"          "tenantWorkspace.istartswith=$(printf '%s' "$WS_FILT" | tr 'a-z' 'A-Z')"
f "H8.22 ?tenantWorkspace.contains"    "infix over the join field"           "tenantWorkspace.contains=$WS_FILT"
f "H8.23 ?tenantWorkspace.icontains"   "infix, case-insensitively"           "tenantWorkspace.icontains=$(printf '%s' "$WS_FILT" | tr 'a-z' 'A-Z')"
f "H8.24 ?tenantStatus.eq — the enum, over the join"  "the owner's commercial state, filterable"  "key.eq=$K_FILT&tenantStatus.eq=active"
f "H8.25 ?tenantStatus.in"             "membership over the enum"            "key.eq=$K_FILT&tenantStatus.in=active,trial"

f "H8.26 ?createdAt.gte" "the temporal pair, lower bound" "key.eq=$K_FILT&createdAt.gte=2020-01-01T00:00:00Z"
f "H8.27 ?createdAt.lte" "upper bound"                    "key.eq=$K_FILT&createdAt.lte=2999-01-01T00:00:00Z"
f "H8.28 ?updatedAt.gte" "the same on the write stamp"    "key.eq=$K_FILT&updatedAt.gte=2020-01-01T00:00:00Z"
f "H8.29 ?updatedAt.lte" "upper bound"                    "key.eq=$K_FILT&updatedAt.lte=2999-01-01T00:00:00Z"

# ── ?orderBy: seven fields, both directions ───────────────────────────────────────────────
for fld in key name tenantID tenantWorkspace tenantStatus createdAt updatedAt; do
  case_ "H8.30 ?orderBy=$fld" "200 — a declared sort key, ascending"
  api GET "/roles?orderBy=$fld&first=5"
  assert_status 200
  case_ "H8.30 ?orderBy=-$fld" "200 — and descending; every declared field carries both directions"
  api GET "/roles?orderBy=-$fld&first=5"
  assert_status 200
done

case_ "H8.31 the ordering is REAL, not merely accepted" "the two directions answer reversed first rows"
api GET "/roles?key.startswith=$K_PREFIX&orderBy=key&first=1"
A_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].key // ""')
api GET "/roles?key.startswith=$K_PREFIX&orderBy=-key&first=1"
Z_FIRST=$(printf '%s' "$HTTP_BODY" | jq -r '.data[0].key // ""')
if [ -n "$A_FIRST" ] && [ -n "$Z_FIRST" ]; then pass_; else fail_ "asc '$A_FIRST' / desc '$Z_FIRST'"; fi

# ── ?fields= ──────────────────────────────────────────────────────────────────────────────
case_ "H8.32 ?fields=key" "the one field, and nothing else"
api GET "/roles?key.eq=$K_FILT&fields=key"
assert_json_at 200 '.data[0] | keys | join(",")' "key"

case_ "H8.33 ?fields=tenantWorkspace — the JOIN value alone" "projectable exactly like a local column"
api GET "/roles?key.eq=$K_FILT&fields=tenantWorkspace"
assert_json_at 200 '.data[0] | keys | join(",")' "tenantWorkspace"

case_ "H8.34 ?fields=permissions.permission" "the collection carrying that entry field and nothing else"
api GET "/roles?key.eq=$K_FILT&fields=permissions.permission"
assert_json_at 200 '.data[0].permissions[0] | [has("permission"), (has("permissionID") | not)] | @csv' "true,true"

case_ "H8.35 ?fields=permissions.permissionArchivedAt" "the counterpart's stamp, selectable — unlike the two hidden halves beside it"
api GET "/roles?key.eq=$K_FILT&fields=permissions.permissionArchivedAt"
assert_status 200

case_ "H8.36 ?fields=archivedAt,tenantArchivedAt" "200 and a NARROWED row — both stamps are selectable even though neither is filterable; on a live row both are null and the listing DTO is omitempty, so the assertion is that nothing unrequested rode along"
api GET "/roles?key.eq=$K_FILT&fields=archivedAt,tenantArchivedAt"
assert_json_at 200 '.data[0] | [has("key"), has("id"), has("permissions")] | any' "false"

case_ "H8.37 ?onlyTotal=true" "totalCount alone — no data, no cursors"
api GET "/roles?key.eq=$K_FILT&onlyTotal=true"
assert_json_at 200 '[(.data == null), (.pagination.totalCount == 1)] | @csv' "true,true"

# ── pagination, as a BICONDITIONAL ───────────────────────────────────────────────────────
K_PAGE_PREFIX=""
for i in 1 2 3 4 5; do
  K_P=$(role_key page)
  [ -z "$K_PAGE_PREFIX" ] && K_PAGE_PREFIX="${K_P%-*}"
  new_role "$K_P" "$TEN_LANE" >/dev/null || exit 1
done
PQ="key.startswith=$K_PAGE_PREFIX&orderBy=key"

case_ "H8.38 the known set is five rows" "totalCount 5 — every count below is against a set this lane built"
# WITHOUT the orderBy: ?onlyTotal beside a page-shaping control is the conflict H9.30 asserts,
# and a fixture that trips it would fail this case for the suite's own reason.
api GET "/roles?key.startswith=$K_PAGE_PREFIX&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "5"

case_ "H8.39 page 1 of a forward walk" "2 rows, hasNextPage true, hasPreviousPage false"
api GET "/roles?$PQ&first=2"
assert_json_at 200 '[(.data | length), .pagination.hasNextPage, .pagination.hasPreviousPage] | @csv' "2,true,false"

case_ "H8.39b THE BICONDITIONAL: endCursor exactly when hasNextPage, startCursor exactly when hasPreviousPage" "an endCursor and NO startCursor at the head of the walk"
assert_json '[(.pagination.endCursor != null), (.pagination.startCursor == null)] | @csv' "true,true"
P1_IDS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].id] | sort | join(",")')
END1=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')

case_ "H8.40 page 2, reached by echoing endCursor into ?after=" "2 rows, and hasPreviousPage now true"
api GET "/roles?$PQ&first=2&after=$END1"
assert_json_at 200 '[(.data | length), .pagination.hasPreviousPage] | @csv' "2,true"

case_ "H8.40b page 2 is DISJOINT from page 1" "no id in common — a cursor is a window edge, not a repeat"
P2_IDS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].id] | sort | join(",")')
if [ -n "$P2_IDS" ] && [ "$P1_IDS" != "$P2_IDS" ]; then pass_; else fail_ "page1 [$P1_IDS] page2 [$P2_IDS]"; fi
END2=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')

case_ "H8.41 the tail of the walk" "1 row, hasNextPage false, and NO endCursor — the other half of the biconditional"
api GET "/roles?$PQ&first=2&after=$END2"
assert_json_at 200 '[(.data | length), .pagination.hasNextPage, (.pagination.endCursor == null)] | @csv' "1,false,true"

case_ "H8.42 ?last=N alone serves the TAIL window" "2 rows, hasNextPage false"
api GET "/roles?$PQ&last=2"
assert_json_at 200 '[(.data | length), .pagination.hasNextPage] | @csv' "2,false"

case_ "H8.43 walking BACKWARD with last + before" "2 rows, and the walk returns to where it started"
api GET "/roles?$PQ&first=2"
E1=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
api GET "/roles?$PQ&first=2&after=$E1"
S2=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.startCursor')
api GET "/roles?$PQ&last=2&before=$S2"
assert_json_at 200 '.data | length' "2"

case_ "H8.44 ?includeArchived raises totalCount by exactly the archived rows" "one more than the active count, for the lane's own archived fixture"
api GET "/roles?key.eq=$K_ARC&onlyTotal=true"
N_ACTIVE=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.totalCount')
api GET "/roles?key.eq=$K_ARC&includeArchived=true&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "$(( N_ACTIVE + 1 ))"

# ── the seeded baseline, derived from the tracked migration ──────────────────────────────
case_ "H8.45 the master role carries exactly one grant" "1 entry — migration 0012 inserts the role and one grant, and nothing else"
api GET "/roles/$QA_MASTER_ROLE_ID"
assert_json_at 200 '.data.permissions | length' "1"

case_ "H8.45b and that grant renders the wildcard" "*:* — the row no API call can reproduce, which is why a migration is the only way a super-admin exists"
assert_json '.data.permissions[0].permission' "*:*"

case_ "H8.45c the master role lives in the master tenant" "the seeded tenant id, reached through the ROOT join as the workspace 'master'"
assert_json '[.data.tenantID, .data.tenantWorkspace] | join("|")' "$QA_MASTER_TENANT_ID|master"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H9 — rejected reads: the whole typed-400 guard family.
#
# The `field` named is the WIRE TOKEN, not the bare field.
# ═════════════════════════════════════════════════════════════════════════════════════════

r400() { # r400 CASE EXPECT QUERY [KEY]
  case_ "$1" "$2"
  api GET "/roles?$3"
  assert_rest 400 "${4:-SchemaViolationNotification}"
}

r400 "H9.1 an unknown query key"                     "400 SchemaViolationNotification" "bogus=1"
r400 "H9.2 an operator outside a leaf's allowlist"   "400 — key declares no gte"        "key.gte=x"
r400 "H9.3 ?name.ne — declared on key, NOT on name"  "400 — the asymmetry is deliberate, and a regression would flatten it" "name.ne=x"
r400 "H9.4 ?description.eq — contains/icontains only" "400"                             "description.eq=x"
r400 "H9.5 ?tenantStatus.contains — the enum takes eq/in alone" "400 — contains over a closed set of three words reads as text search and answers as noise" "tenantStatus.contains=act"
r400 "H9.6 ?search= — a RESERVED control the DTO never declared" "400 — the opt-in gate answers before any engine is consulted" "search=x"

r400 "H9.7 ?archivedAt.gte — SERVED, deliberately not FILTERABLE" "400 — the filter vocabulary has no isnull/notnull, so a declaration would only buy 'archived between these dates'; absence is a choice" "archivedAt.gte=2020-01-01T00:00:00Z"
r400 "H9.8 ?tenantArchivedAt.gte — the same choice on the join" "400 — the question a caller actually asks is what tenantStatus answers" "tenantArchivedAt.gte=2020-01-01T00:00:00Z"

r400 "H9.9 ?permissions.resource.eq — the CHILD JOIN's hidden field"  "400 — load-only, addressable in no criteria" "permissions.resource.eq=tenant"
r400 "H9.10 ?permissions.permission.eq — the computed entry field"    "400 — a derivation backs no column"          "permissions.permission.eq=tenant:read"
r400 "H9.11 ?permissions.permissionID.eq — the entry's OWN stored column" "400 — the 1:N boundary, not the backing: filtering a root by a child field is a pushdown one root SELECT cannot express" "permissions.permissionID.eq=$P_TENANT_READ"

r400 "H9.12 ?orderBy=description — filterable, orderable in NEITHER direction" "400 SchemaViolationNotification" "orderBy=description"
r400 "H9.13 ?orderBy=archivedAt"                  "400 — served, not sortable"        "orderBy=archivedAt"
r400 "H9.14 ?orderBy=permissions.permission"      "400 — the 1:N boundary on the sort side" "orderBy=permissions.permission"
r400 "H9.15 ?orderBy=id — declarable, deliberately not declared" "400"                "orderBy=id"
r400 "H9.16 ?orderBy=bogus"                       "400"                                "orderBy=bogus"

r400 "H9.17 ?fields=permissions.resource — hidden, feeding the derivation, never SELECTABLE" "400 — the complement of H3" "fields=permissions.resource"
r400 "H9.18 ?fields=permissions.action"           "400 — same"                         "fields=permissions.action"
r400 "H9.19 ?fields=bogus"                        "400"                                "fields=bogus"
r400 "H9.20 ?fields=deletedAt — the pre-v0.74.0 token" "400 — the rename reached the wire, and the old name resolves to nothing" "fields=deletedAt"

case_ "H9.21 ?first above the page ceiling" "400 LimitExceededNotification, and the ceiling is the framework default"
api GET "/roles?first=101"
assert_rest 400 LimitExceededNotification

case_ "H9.21b the refusal hands back the ceiling itself" "100 — no query: block in the yaml and no per-view override, so bootstrap.FrameworkDefaultMaxLimit stands"
assert_json '[.errors[].messages[] | select(.notificationKey=="LimitExceededNotification") | .value] | join(",")' "100"

r400 "H9.22 ?first=0"    "400 — a page of nothing is not a page" "first=0"
r400 "H9.23 ?first=abc"  "400 — the value is outside the control's kind" "first=abc"
r400 "H9.24 ?first=-5"   "400"                                   "first=-5"

r400 "H9.25 ?first + ?last together"     "400 — the direction has to be one or the other" "first=2&last=2"
r400 "H9.26 ?first with ?before"         "400 — forward is first+after, backward is last+before" "first=2&before=x"
r400 "H9.27 ?last with ?after"           "400 — the mirror of the same rule"              "last=2&after=x"
r400 "H9.28 ?after with ?before"         "400"                                            "after=x&before=y"

r400 "H9.29 ?onlyTotal beside ?first"    "400 — the only-total conflict matrix" "onlyTotal=true&first=5"
r400 "H9.30 ?onlyTotal beside ?orderBy"  "400"                                  "onlyTotal=true&orderBy=key"
r400 "H9.31 ?onlyTotal beside ?fields"   "400"                                  "onlyTotal=true&fields=key"
r400 "H9.32 ?onlyTotal beside ?after"    "400"                                  "onlyTotal=true&after=x"

case_ "H9.33 ?onlyTotal=true beside a FILTER" "200 — counting a filtered subset is the canonical use, and the conflict matrix does not touch it"
api GET "/roles?onlyTotal=true&key.eq=$K_FILT"
assert_json_at 200 '.pagination.totalCount' "1"

case_ "H9.34 ?onlyTotal=true beside ?includeArchived" "200 — same"
api GET "/roles?onlyTotal=true&includeArchived=true&key.eq=$K_ARC"
assert_status 200

case_ "H9.35 ?onlyTotal=false beside a page control" "200 — present-but-INACTIVE never trips the conflict matrix"
api GET "/roles?onlyTotal=false&first=5"
assert_status 200

r400 "H9.36 ?includeArchived=1"  "400 — the booleans take exactly true/false" "includeArchived=1"
r400 "H9.37 ?onlyTotal= (empty)" "400 — presence with an unparseable value is still presence" "onlyTotal="

r400 "H9.38 a malformed cursor" "400" "after=not-a-cursor"

case_ "H9.39 a cursor replayed under a DIFFERENT order" "400 — the structural check: a cursor is only meaningful in the walk that minted it"
api GET "/roles?$PQ&first=2"
C_STRUCT=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
api GET "/roles?key.startswith=$K_PAGE_PREFIX&orderBy=name&first=2&after=$C_STRUCT"
assert_rest 400 SchemaViolationNotification

case_ "H9.40 a cursor replayed with ?includeArchived flipped" "400 — the context-hash check: the archived gate is part of the walk"
api GET "/roles?$PQ&first=2&includeArchived=true&after=$C_STRUCT"
assert_rest 400 SchemaViolationNotification

r400 "H9.41 ?createdAt.gte with a value outside the leaf's kind" "400 InvalidFilterValueNotification — pin >= v0.70.0" "createdAt.gte=not-a-date" InvalidFilterValueNotification
r400 "H9.42 ?tenantID.eq with a value an identity column refuses" "400 InvalidFilterValueNotification — below v0.70.0 the same request was a 500 on this backing" "tenantID.eq=lixo" InvalidFilterValueNotification

# ── the by-id endpoint's own opt-in gate ─────────────────────────────────────────────────
case_ "H9.43 by-id ?fields= — a control that endpoint does not declare" "400 — FindRoleByIDRequest declares includeArchived and nothing else, and PRESENCE is what trips the gate"
api GET "/roles/$ID_GOLD?fields=key"
assert_rest 400 SchemaViolationNotification

case_ "H9.44 by-id ?onlyTotal=false — present but inactive, and still refused" "400 — an undeclared control is refused on presence, whatever its value"
api GET "/roles/$ID_GOLD?onlyTotal=false"
assert_rest 400 SchemaViolationNotification

case_ "H9.45 by-id ?includeArchived=true — the positive control" "200 — the one control it does declare"
api GET "/roles/$ID_GOLD?includeArchived=true"
assert_status 200

case_ "H9.46 UnsupportedCapabilityNotification on this entity" "recorded as a DERIVATION, and deliberately not exercised"
skip_ "That key needs a control the DTO DOES declare and the backing cannot serve. FindRolesRequest declares no Search field, so ?search= is refused at the wire wrapper before any engine is reached (H9.6); and nothing is declared under permissions.*, so the schema gate answers first there too (H9.9-H9.11). Asserting it here would assert a lie, and would go RED against a correct service"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H10 — routing, absent verbs, and the by-id ADDRESS contract (pin >= v0.70.0).
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "H10.1 PATCH /roles/{id}/unarchive" "404 RouteNotFoundNotification — no route is registered at that path AT ALL: unarchive is absent from Modes() and unmounted"
api PATCH "/roles/$ID_GOLD/unarchive"
assert_rest 404 RouteNotFoundNotification

case_ "H10.2 DELETE /roles/{id}" "405 MethodNotAllowedNotification — the path is registered, the method is not; this is what proves no hard delete exists"
api DELETE "/roles/$ID_GOLD"
assert_rest 405 MethodNotAllowedNotification

case_ "H10.3 PUT /roles/{id}" "405 — the whole service speaks PATCH"
api PUT "/roles/$ID_GOLD"
assert_rest 405 MethodNotAllowedNotification

case_ "H10.4 POST /roles/{id}" "405"
api POST "/roles/$ID_GOLD" '{}'
assert_rest 405 MethodNotAllowedNotification

case_ "H10.5 GET /roles/{id}/archive" "405 — the same path IS registered, under PATCH"
api GET "/roles/$ID_GOLD/archive"
assert_rest 405 MethodNotAllowedNotification

api GET "/roles/$ID_GOLD"
CHILD_GOLD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.permissions[0].id')
case_ "H10.6 DELETE on a child entry" "anything but 204 — a DELETE that soft-removed would be a lying contract, and this route does not exist"
api DELETE "/roles/$ID_GOLD/permissions/$CHILD_GOLD"
assert_status_not 204

case_ "H10.7 a path no route matches" "404 RouteNotFoundNotification"
api GET "/does-not-exist"
assert_rest 404 RouteNotFoundNotification

case_ "H10.8 GET /roles/{unused uuid}" "404 RecordNotFoundNotification — a well-formed address that names nothing"
api GET "/roles/00000000-0000-4000-8000-00000000dead"
assert_rest 404 RecordNotFoundNotification

case_ "H10.9 GRANT onto a role id that addresses nothing" "404 — the owner is loaded before the entry is considered"
api POST "/roles/00000000-0000-4000-8000-00000000dead/permissions" "$(jq -nc --arg p "$P_ROLE_READ" '{permissionID:$p}')"
assert_rest 404 RecordNotFoundNotification

case_ "H10.10 GET /roles/not-a-uuid — a READ address" "404 UnknownIDAddressNotification — a read names no record"
api GET "/roles/not-a-uuid"
assert_rest 404 UnknownIDAddressNotification

case_ "H10.11 PATCH /roles/not-a-uuid — a WRITE intention" "400 MalformedIDNotification — the same token, split by VERB and not by surface"
api PATCH "/roles/not-a-uuid" '{"name":"QA Malformed"}'
assert_rest 400 MalformedIDNotification

case_ "H10.12 PATCH /roles/not-a-uuid/archive" "400 MalformedIDNotification"
api PATCH "/roles/not-a-uuid/archive"
assert_rest 400 MalformedIDNotification

case_ "H10.13 POST /roles/not-a-uuid/permissions" "400 MalformedIDNotification — a collection verb is a write on the root"
api POST "/roles/not-a-uuid/permissions" "$(jq -nc --arg p "$P_ROLE_READ" '{permissionID:$p}')"
assert_rest 400 MalformedIDNotification

# ── the CHILD address, which is NOT that contract ────────────────────────────────────────
case_ "H10.14 THE CHILD ADDRESS: a junk entry id under a GOOD role id" "404 RecordNotFoundNotification — ArchiveRolePermissionRequest.RolePermissionID is a plain string path field, not a domain.ID, so the wire wrapper never inspects it; the value reaches Role.RemoveRolePermissionByID, which answers the canonical not-found for any id the collection does not carry"
api PATCH "/roles/$ID_GOLD/permissions/not-a-uuid/archive"
assert_rest 404 RecordNotFoundNotification

case_ "H10.15 a well-formed entry id the collection does not hold" "404 — the same answer, which is what makes the two ids in one URL worth telling apart"
api PATCH "/roles/$ID_GOLD/permissions/00000000-0000-4000-8000-00000000beef/archive"
assert_rest 404 RecordNotFoundNotification

case_ "H10.16 the 403 mode-not-allowed shape" "recorded as a DERIVATION, and deliberately not exercised"
skip_ "That shape needs a mode absent from Modes() while its route is still MOUNTED. Role's absent mode (unarchive) has no route at all, so it lands on the 404 arm (H10.1); its absent verb (DELETE) has a registered path, so it lands on 405 (H10.2). No ...NotAllowedNotification is reachable on this entity"

# ═════════════════════════════════════════════════════════════════════════════════════════
# H11 — suite meta: the route inventory, cross-checked against what the service publishes.
# ═════════════════════════════════════════════════════════════════════════════════════════

api GET "/openapi.json" "" -
OPENAPI="$HTTP_BODY"

case_ "H11.1 /openapi.json enumerates exactly SEVEN role operations" "7 — four modes, seven routes, and no eighth"
N_OPS=$(printf '%s' "$OPENAPI" | jq '[.paths | to_entries[] | select(.key | startswith("/roles")) | .value | to_entries[]] | length')
if [ "$N_OPS" = "7" ]; then pass_; else fail_ "$N_OPS operations under /roles"; fi

case_ "H11.2 every one of them declares a required permission" "7 of 7 — a route mounted OPEN is visible here, not merely absent from a count"
N_GATED=$(printf '%s' "$OPENAPI" | jq '[.paths | to_entries[] | select(.key | startswith("/roles")) | .value | to_entries[] | select(.value | tostring | test("Required permission"))] | length')
if [ "$N_GATED" = "7" ]; then pass_; else fail_ "$N_GATED of $N_OPS gated"; fi

case_ "H11.3 the five literals the routes declare" "role:insert, role:update, role:archive, role:read, role:grant — and role:grant on BOTH collection verbs"
LITS=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key | startswith("/roles")) | .value | tostring] | join(" ")' | grep -oE 'role:[a-z]+' | sort | uniq -c | awk '{printf "%s:%s ", $2, $1}')
if printf '%s' "$LITS" | grep -q 'role:grant:2'; then pass_; else fail_ "$LITS"; fi

case_ "H11.4 no unarchive operation is published" "0 — the absent mode is absent from the contract too"
N_UN=$(printf '%s' "$OPENAPI" | jq '[.paths | to_entries[] | select(.key | test("^/roles.*unarchive"))] | length')
if [ "$N_UN" = "0" ]; then pass_; else fail_ "$N_UN unarchive paths"; fi

qa_finish
