#!/usr/bin/env bash
# Lane: group — the REST half of the framework contract for the Group aggregate.
#
# Families K1-K11 of specs/qa/group-contract/plan.md §1. The L family (GraphQL) lives in
# qa/group_graphql.sh; the business rules live in qa/domain.sh (GR1-GR13); the gate lives in
# qa/security.sh (S6.x); the audit trail lives in qa/audit.sh (A39+).
#
# THE ONE IDEA THIS WHOLE LANE IS BUILT AROUND — one column goes in, five come out, and this
# time BOTH joins are served. What separates them is where they can be ASKED ABOUT:
#
#   the ROOT join → Tenant          the CHILD join → Role
#   ─────────────────────           ──────────────────────
#   tenantWorkspace   served        roleKey          served   ← NOT hidden, unlike Role's pair
#   tenantStatus      served        roleName         served
#   tenantArchivedAt  served        roleArchivedAt   served
#   filterable + sortable           addressable in NO criteria — the 1:N boundary
#
# A suite copied from qa/role.sh goes wrong in BOTH directions here, so every case below says
# which side of that table it is on:
#   · ?fields=roles.roleKey is a 200 — its Role twin (?fields=permissions.resource) was a 400;
#   · ?roles.roleKey.eq= is still a 400 — served does not mean addressable.
# The consequence, which spec.md §9 calls "the one real cost": "which groups confer role X?"
# is not answerable from this listing.
#
# The lane runs as the bootstrap admin (*:*), so nothing here is blocked by row scope. §1b and
# §3 are where the scoped principals do the work.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init group

# ═════════════════════════════════════════════════════════════════════════════════════════
# Fixtures. Catalog ids are resolved by their PAIR and role ids by their handle — no UUID
# literal is written twice in this suite.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_LANE=$(new_tenant active "$(ws group)")  || exit 1
TEN_OTHER=$(new_tenant active "$(ws groupb)") || exit 1

P_TENANT_READ=$(permission_id_of tenant read)
P_ROLE_READ=$(permission_id_of role read)
P_GROUP_READ=$(permission_id_of group read)
for p in "$P_TENANT_READ" "$P_ROLE_READ" "$P_GROUP_READ"; do
  [ -n "$p" ] || { echo "group.sh: a seeded catalog id could not be resolved — is migration 0012 applied?" >&2; exit 1; }
done

D_OK="A group description long enough to satisfy the shared anti-junk floor this service applies."

# Three live roles in the lane's tenant, plus one it will archive under K7.2. A role is
# tenant-scoped, so every one of these has to live where the groups do.
K_ROLE_A=$(role_key ga); R_A=$(new_role "$K_ROLE_A" "$TEN_LANE" "$P_TENANT_READ") || exit 1
K_ROLE_B=$(role_key gb); R_B=$(new_role "$K_ROLE_B" "$TEN_LANE" "$P_ROLE_READ")   || exit 1
K_ROLE_C=$(role_key gc); R_C=$(new_role "$K_ROLE_C" "$TEN_LANE" "$P_GROUP_READ")  || exit 1
K_ROLE_D=$(role_key gd); R_D=$(new_role "$K_ROLE_D" "$TEN_LANE" "$P_TENANT_READ") || exit 1

# ═════════════════════════════════════════════════════════════════════════════════════════
# K1 — the seven mounted routes. Modes() declares display, insert, update, archive: FOUR
#      modes, SEVEN routes, because the collection carries a FIFTH verb of its own.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_MAIN=$(group_key main)
case_ "K1.1 POST /groups" "201 — the record as stored, carrying both entries"
api POST /groups "$(group_body "$K_MAIN" "QA Main Group" "$D_OK" "$TEN_LANE" "$R_A" "$R_B")"
assert_json_at 201 '.data.roles | length' "2"
ID_MAIN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "K1.1b the write response carries the stored column and NOT the traversal" "each entry is id + roleID, and no roleKey/roleName/roleArchivedAt"
assert_json '[.data.roles[] | (keys | sort | join(","))] | unique | join(" / ")' "id,roleID"

case_ "K1.2 GET /groups/{id}" "200 — the full document"
api GET "/groups/$ID_MAIN"
assert_json_at 200 '.data.key' "$K_MAIN"

case_ "K1.3 GET /groups" "200 — data plus the pagination envelope"
api GET "/groups?key.eq=$K_MAIN"
assert_json_at 200 '[(.data | length), (.pagination.totalCount)] | join(",")' "1,1"

case_ "K1.4 PATCH /groups/{id}" "200 — the root as stored"
api PATCH "/groups/$ID_MAIN" "$(jq -nc --arg n "QA Main Group Renamed" '{name:$n}')"
assert_json_at 200 '.data.name' "QA Main Group Renamed"

case_ "K1.6 POST /groups/{id}/roles" "201 — the entry as stored, with the id the server minted"
api POST "/groups/$ID_MAIN/roles" "$(jq -nc --arg r "$R_C" '{roleID:$r}')"
assert_json_at 201 '[.data.groupId, (.data.groupRole.roleID)] | join(",")' "$ID_MAIN,$R_C"
CHILD_C=$(printf '%s' "$HTTP_BODY" | jq -r '.data.groupRole.id')

case_ "K1.6b the ATTACH response is the entry, not the traversal" "id + roleID alone"
assert_json '.data.groupRole | keys | sort | join(",")' "id,roleID"

# ── K1.7, the model's canonical trap ──────────────────────────────────────────────────────
# The root-archive auto handler is instantiated once per surface, and wiring it to the child
# DETACH route type-checks, boots and answers 204 — while archiving the WHOLE GROUP, which
# here de-authorizes a team. So this case does not stop at the 204.
case_ "K1.7 PATCH /groups/{id}/roles/{childId}/archive" "204 with NO body"
detach_role "$ID_MAIN" "$CHILD_C"
assert_empty_body 204

case_ "K1.7a the DETACH did not archive the ROOT" "200 on the plain by-id read — the trap this case exists for"
api GET "/groups/$ID_MAIN"
assert_json_at 200 '.data.archivedAt' "null"

case_ "K1.7b the other entries survived the DETACH" "the two originals are still served"
assert_json '[.data.roles[].roleID] | sort | join(",")' "$(printf '%s\n%s\n' "$R_A" "$R_B" | sort | paste -sd, -)"

case_ "K1.7c the detached entry is gone from the collection" "the third roleID is absent"
assert_json "[.data.roles[].roleID] | index(\"$R_C\") // \"absent\"" "absent"

# K1.5 runs LAST of this family: it is terminal on whatever it touches.
K_ARCH1=$(group_key arch)
ID_ARCH1=$(new_group "$K_ARCH1" "$TEN_LANE" "$R_A") || exit 1
case_ "K1.5 PATCH /groups/{id}/archive" "204 with NO body"
api PATCH "/groups/$ID_ARCH1/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# K2 — the golden record. Every declared field, written then read back on both REST shapes.
#      The family that catches a field silently dropped from a DTO or a projection.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_GOLD=$(group_key gold)
ID_GOLD=$(new_group "$K_GOLD" "$TEN_LANE" "$R_A" "$R_B") || exit 1

api GET "/groups/$ID_GOLD"
case_ "K2.1 the by-id document carries all twelve declared keys" "no field silently dropped; the by-id DTO does not elide"
assert_json '.data | keys | sort | join(",")' \
  "archivedAt,createdAt,description,id,key,name,roles,tenantArchivedAt,tenantID,tenantStatus,tenantWorkspace,updatedAt"

case_ "K2.2 the root's own fields"          "as written"
assert_json '[.data.key, .data.name, .data.description] | join("|")' "$K_GOLD|QA Fixture Group|Fixture group created by the QA suite for run $(qa_slug_runid), bundling tenant roles."
case_ "K2.3 tenantID"                       "the owning tenant"
assert_json '.data.tenantID' "$TEN_LANE"
case_ "K2.4 createdAt is RFC3339"           "by grammar, never through jq's fromdateiso8601"
assert_rfc3339 '.data.createdAt'
case_ "K2.5 updatedAt is RFC3339"           "same"
assert_rfc3339 '.data.updatedAt'
case_ "K2.6 archivedAt is null while live"  "the stamp reports a state; K7.6 is where it fills"
assert_json '.data.archivedAt' "null"

# The ROOT join reaching the wire over columns that live in the tenants table.
case_ "K2.7 tenantWorkspace — the ROOT join" "the owning tenant's actual handle, not an echo"
api GET "/tenants/$TEN_LANE"
WS_LANE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')
api GET "/groups/$ID_GOLD"
assert_json '.data.tenantWorkspace' "$WS_LANE"
case_ "K2.8 tenantStatus — the ROOT join"    "active"
assert_json '.data.tenantStatus' "active"
case_ "K2.9 tenantArchivedAt is null while the owner lives" "K7.5 is where it fills"
assert_json '.data.tenantArchivedAt' "null"

# The CHILD join — the half Role could not have, because its pair was hidden.
case_ "K2.10 every entry carries five keys while live" "id, roleID, roleKey, roleName; roleArchivedAt elided by omitempty until stamped"
assert_json '[.data.roles[] | (keys | sort | join(","))] | unique | join(" / ")' "id,roleID,roleKey,roleName"

case_ "K2.11 roleKey is the counterpart's ACTUAL handle" "read across the foreign key, for every entry"
assert_json '[.data.roles[] | .roleKey] | sort | join(",")' "$(printf '%s\n%s\n' "$K_ROLE_A" "$K_ROLE_B" | sort | paste -sd, -)"

case_ "K2.12 roleName is the counterpart's display name" "same traversal, second column"
assert_json '[.data.roles[] | .roleName] | unique | join(",")' "QA Fixture Role"

# The listing shape is NOT the by-id shape, and the difference is omitempty.
api GET "/groups?key.eq=$K_GOLD"
case_ "K2.13 the listing row elides the two null stamps" "omitempty on a nil pointer — the by-id Response does not elide, K2.1 does"
assert_json_at 200 '.data[0] | keys | sort | join(",")' \
  "createdAt,description,id,key,name,roles,tenantID,tenantStatus,tenantWorkspace,updatedAt"
case_ "K2.14 the listing row carries the same join values" "one document, two shapes, one truth"
assert_json '[.data[0].tenantWorkspace, .data[0].tenantStatus] | join(",")' "$WS_LANE,active"
case_ "K2.15 the listing row carries the whole collection" "a relational view DOES serve the aggregate's 1:N children"
assert_json '.data[0].roles | length' "2"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K3 — "one column in, five out": the child-join doctrine, both halves.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "K3.1 ?fields=roles.roleKey resolves" "200 — the half Role could not have: its twin, permissions.resource, was a 400"
api GET "/groups?key.eq=$K_GOLD&fields=roles.roleKey"
assert_json_at 200 '[.data[0].roles[] | .roleKey] | length' "2"

case_ "K3.2 ?fields=roles.roleName resolves" "200"
api GET "/groups?key.eq=$K_GOLD&fields=roles.roleName"
assert_json_at 200 '[.data[0].roles[] | .roleName] | length' "2"

case_ "K3.3 ?fields=roles.roleArchivedAt resolves" "200 — the counterpart's stamp is selectable, not merely present"
api GET "/groups?key.eq=$K_GOLD&fields=roles.roleArchivedAt"
assert_status 200

case_ "K3.4 ?fields=roles.roleID resolves" "200 — the entry's own stored column"
api GET "/groups?key.eq=$K_GOLD&fields=roles.roleID"
assert_json_at 200 '[.data[0].roles[] | .roleID] | length' "2"

# The negative half. On Role it was "hidden fields must never appear"; here it is the WRITE
# side, and it is the assertion that keeps a read-side value from leaking into a command.
case_ "K3.5 the INSERT 201 carries no traversal field" "roleKey absent — a KEY that is missing, not a key that is null"
api POST /groups "$(group_body "$(group_key j3)" "QA Join Probe" "$D_OK" "$TEN_LANE" "$R_A")"
assert_json '[.data.roles[] | has("roleKey")] | any' "false"
ID_J3=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "K3.6 the PATCH 200 carries no traversal field" "same"
api PATCH "/groups/$ID_J3" "$(jq -nc '{name:"QA Join Probe Two"}')"
assert_json '[.data.roles[] | has("roleName")] | any' "false"

case_ "K3.7 the ATTACH 201 carries no traversal field" "GroupRoleResponse is {id, roleID} and nothing else"
api POST "/groups/$ID_J3/roles" "$(jq -nc --arg r "$R_B" '{roleID:$r}')"
assert_json '.data.groupRole | has("roleKey")' "false"

case_ "K3.8 the write side never speaks the render" "an entry naming roleKey supplies no roleID → 422, never a silent match by string"
api POST /groups "$(jq -nc --arg k "$(group_key j4)" --arg d "$D_OK" --arg t "$TEN_LANE" --arg rk "$K_ROLE_A" \
  '{key:$k, name:"QA Render Probe", description:$d, tenantID:$t, roles:[{roleKey:$rk}]}')"
assert_status 422

# ═════════════════════════════════════════════════════════════════════════════════════════
# K4 — validation. The notification KEY and the FIELD, never prose.
# ═════════════════════════════════════════════════════════════════════════════════════════

v422() { # v422 CASE EXPECT JSON_PATCH KEY FIELD
  case_ "$1" "$2"
  api POST /groups "$(group_body "$(group_key v)" "QA Validation Group" "$D_OK" "$TEN_LANE" | jq -c "$3")"
  assert_rest_field 422 "$4" "$5"
}

v422 "K4.1 key empty"                    "422 RequiredFieldNotification — the VO short-circuits on empty, so the framework's own key is what answers" '.key = ""'            RequiredFieldNotification    key
v422 "K4.2 key uppercase"                "422 — refused, never repaired"                     '.key = "Engineering"'   InvalidGroupKeyNotification  key
v422 "K4.3 key below the 2-rune floor"   "422"                                               '.key = "e"'             InvalidGroupKeyNotification  key
v422 "K4.4 key with a doubled hyphen"    "422 — a hyphen may never lead, trail or double"    '.key = "eng--team"'     InvalidGroupKeyNotification  key
v422 "K4.5 key with a leading hyphen"    "422"                                               '.key = "-eng"'          InvalidGroupKeyNotification  key
v422 "K4.6 key with a trailing hyphen"   "422"                                               '.key = "eng-"'          InvalidGroupKeyNotification  key
v422 "K4.7 key with a run of 4 identical runes" "422 — groupKeyMaxIdenticalRun = 4"          '.key = "aaaab"'         InvalidGroupKeyNotification  key
v422 "K4.8 key of 2 runes but 1 distinct" "422 — groupKeyMinDistinct = 2, which is why 'hr' passes and 'hh' does not" '.key = "hh"' InvalidGroupKeyNotification key
v422 "K4.9 key of 65 runes"              "422 — the column is 64"                            '.key = "aaabbbcccdddeeefffggghhhiiijjjkkklllmmmnnnooopppqqqrrrssstttuuuvvv"' InvalidGroupKeyNotification key
v422 "K4.10 name empty"                  "422 RequiredFieldNotification"                     '.name = ""'             RequiredFieldNotification    name
v422 "K4.11 name below the DisplayName floor" "422"                                          '.name = "e"'            InvalidDisplayNameNotification name
v422 "K4.12 description below the 15-rune floor" "422"                                       '.description = "short"' InvalidDescriptionNotification description

case_ "K4.13 an entry whose roleID is not a uuid" "422 InvalidIDUUIDNotification on the entry — reported by the framework's child validation, NOT by a probe that panicked"
api POST /groups "$(jq -nc --arg k "$(group_key v)" --arg d "$D_OK" --arg t "$TEN_LANE" \
  '{key:$k, name:"QA Child Id Probe", description:$d, tenantID:$t, roles:[{roleID:"tatu"}]}')"
assert_rest 422 InvalidIDUUIDNotification

# Positive controls — each one a rule's own boundary.
vok() { # vok CASE EXPECT JSON_PATCH
  case_ "$1" "$2"
  api POST /groups "$(group_body "$(group_key p)" "QA Positive Group" "$D_OK" "$TEN_LANE" | jq -c "$3")"
  assert_status 201
}
vok "K4.14 a plain lowercase handle"      "201" '.key = .key'
vok "K4.15 a handle with a leading digit" "201 — '3m-approvers' is a name somebody really has" ".key = \"3m-$(qa_slug_runid)-x\""
vok "K4.16 a 2-rune, 2-distinct handle"   "201 — the floor vos.GroupKey exists to accept: 'hr', 'ap', 'it' are handles a customer really types" '.key = "hr"'
vok "K4.17 an accented, multi-word description" "201" '.description = "Time de operações da área comercial, com acesso de leitura ao registro."'

# ── the guard barrier, K4.g ───────────────────────────────────────────────────────────────
# TenantID carries a valueObject rule with guard: true, so domain.ID.IsValid runs FIRST and
# StopIfInvalid ends the WHOLE pass — the automatic value-object pass and every collection
# included. Without it an unusable owner reaches a probe bound to a UUID column and the
# request dies as a 500. That is the bug Role actually shipped.
case_ "K4.g1 tenantID empty"  "422 InvalidIDUUIDNotification — uuid.Parse refuses '' and junk through the same call"
api POST /groups "$(jq -nc --arg k "$(group_key g1)" --arg d "$D_OK" '{key:$k, name:"QA Barrier", description:$d, tenantID:"", roles:[]}')"
assert_rest 422 InvalidIDUUIDNotification

case_ "K4.g2 tenantID not a uuid" "422 InvalidIDUUIDNotification — the same key, because it is the same call"
api POST /groups "$(jq -nc --arg k "$(group_key g2)" --arg d "$D_OK" '{key:$k, name:"QA Barrier", description:$d, tenantID:"tatu", roles:[]}')"
assert_rest 422 InvalidIDUUIDNotification

case_ "K4.g3 the barrier ENDS the pass" "the owner ALONE is reported — the bad description and the bad entry must NOT appear"
api POST /groups "$(jq -nc --arg k "$(group_key g3)" '{key:$k, name:"QA Barrier", description:"short", tenantID:"tatu", roles:[{roleID:"tatu"}]}')"
assert_json '[.errors[]?.messages[]?.notificationKey] | unique | join(",")' "InvalidIDUUIDNotification"

case_ "K4.g4 the field the barrier reports" "tenantID, camelCased like every other field in the same envelope — the standing guard for the v0.72.1 defect fixed at this pin"
api POST /groups "$(jq -nc --arg k "$(group_key g4)" --arg d "$D_OK" '{key:$k, name:"QA Barrier", description:$d, tenantID:"tatu", roles:[]}')"
assert_rest_field 422 InvalidIDUUIDNotification tenantID

# ═════════════════════════════════════════════════════════════════════════════════════════
# K5 — the 409 family, and what "unique within the tenant" actually means.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_DUP=$(group_key dup)
ID_DUP=$(new_group "$K_DUP" "$TEN_LANE") || exit 1

case_ "K5.1 the same key twice in ONE tenant" "409 GroupKeyAlreadyExistsNotification on field key"
api POST /groups "$(group_body "$K_DUP" "QA Duplicate" "$D_OK" "$TEN_LANE")"
assert_rest_field 409 GroupKeyAlreadyExistsNotification key

case_ "K5.1b the refusal carries the Conflict semantic" "semantic 'Conflict' on the message — the transport-agnostic classification a client branches on without parsing the status"
assert_json '[.errors[].messages[] | select(.notificationKey=="GroupKeyAlreadyExistsNotification") | .semantic] | unique | join(",")' "Conflict"

case_ "K5.2 the SAME key in a DIFFERENT tenant" "201 — the index is (tenant_id, group_key), not group_key. Without this control an over-broad global index would pass K5.1 just as well"
api POST /groups "$(group_body "$K_DUP" "QA Duplicate Elsewhere" "$D_OK" "$TEN_OTHER")"
assert_status 201

case_ "K5.3 a patch that moves only name/description" "200 — excludeSelf, so a row never collides with itself"
api PATCH "/groups/$ID_DUP" "$(jq -nc '{name:"QA Duplicate Renamed"}')"
assert_status 200

case_ "K5.4 archive a group, then take its key again" "201 with a NEW id — unique.scope is active-only, and with no unarchive verb this is the only route back"
api PATCH "/groups/$ID_DUP/archive"
api POST /groups "$(group_body "$K_DUP" "QA Duplicate Reborn" "$D_OK" "$TEN_LANE")"
ID_REBORN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
if [ "$HTTP_STATUS" = "201" ] && [ -n "$ID_REBORN" ] && [ "$ID_REBORN" != "$ID_DUP" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, id '$ID_REBORN' vs '$ID_DUP'"; fi

case_ "K5.4b the archived row is still readable" "200 under ?includeArchived=true — a retired group stays auditable"
api GET "/groups/$ID_DUP?includeArchived=true"
assert_json_at 200 '.data.id' "$ID_DUP"

case_ "K5.5 the same roleID twice inside ONE insert body" "409 GroupAlreadyGrantsRoleNotification on field roles"
api POST /groups "$(group_body "$(group_key d2)" "QA Twin Entries" "$D_OK" "$TEN_LANE" "$R_A" "$R_A")"
assert_rest_field 409 GroupAlreadyGrantsRoleNotification roles

K_ATT=$(group_key att)
ID_ATT=$(new_group "$K_ATT" "$TEN_LANE" "$R_A") || exit 1
case_ "K5.6 ATTACH a role the group already confers" "409 — IsSameBusinessIdentity over RoleID ALONE is load-bearing: three of the five entry fields are blank on a fresh entry, so comparing all of them would answer 'different' and the guard would fail OPEN"
api POST "/groups/$ID_ATT/roles" "$(jq -nc --arg r "$R_A" '{roleID:$r}')"
assert_rest 409 GroupAlreadyGrantsRoleNotification

CH_ATT=$(attach_role "$ID_ATT" "$R_B") || exit 1
case_ "K5.7 DETACH an entry, then ATTACH the same role again" "201 with a NEW child id — the child index is active-only and there is no per-entry unarchive"
detach_role "$ID_ATT" "$CH_ATT"
CH_ATT2=$(attach_role "$ID_ATT" "$R_B") || true
if [ -n "${CH_ATT2:-}" ] && [ "$CH_ATT2" != "$CH_ATT" ]; then pass_; else fail_ "child id '$CH_ATT2' vs '$CH_ATT'"; fi

case_ "K5.8 the wrong-state 409 is N/A on this aggregate" "no PUT verb, no revision on the wire; every wrong-state attempt is intercepted a layer earlier by the LOAD scope and lands as 404 (K6)"
skip_ "EntityIsNotActiveNotification / ConcurrentModificationNotification are not reachable through this aggregate's HTTP surface: there is no PUT, no wire field carries a revision, and the archived-root cases of K6 answer 404 because LoadForWrite runs ScopeActive. Asserting a 409 there would encode a promise the pin does not make"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K6 — archive, which on this entity is a ONE-WAY DOOR that de-authorizes a team.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_ARC=$(group_key arc)
ID_ARC=$(new_group "$K_ARC" "$TEN_LANE" "$R_A" "$R_B") || exit 1

case_ "K6.1 archive"                            "204 with no body"
api PATCH "/groups/$ID_ARC/archive"; assert_empty_body 204
case_ "K6.2 the plain by-id read"               "404 RecordNotFoundNotification"
api GET "/groups/$ID_ARC"; assert_rest 404 RecordNotFoundNotification
case_ "K6.3 by-id ?includeArchived=true"        "200 with archivedAt stamped"
api GET "/groups/$ID_ARC?includeArchived=true"; assert_json_at 200 '.data.archivedAt != null' "true"
case_ "K6.3b and its entries are still readable" "a retired group must stay auditable, and this is the only way to see one"
assert_json '.data.roles | length' "2"
case_ "K6.4 the plain listing"                  "the row is absent"
api GET "/groups?key.eq=$K_ARC"; assert_json_at 200 '.data | length' "0"
case_ "K6.5 the listing with ?includeArchived=true" "the row is present"
api GET "/groups?key.eq=$K_ARC&includeArchived=true"; assert_json_at 200 '.data | length' "1"

case_ "K6.6 no unarchive route exists"          "404 RouteNotFoundNotification — not a 405: nothing is registered at that path at all"
api PATCH "/groups/$ID_ARC/unarchive"; assert_rest 404 RouteNotFoundNotification

case_ "K6.7 ATTACH onto an archived group"      "404 — LoadForWrite runs ScopeActive"
api POST "/groups/$ID_ARC/roles" "$(jq -nc --arg r "$R_C" '{roleID:$r}')"; assert_status 404
case_ "K6.8 DETACH on an archived group"        "404 — same scope"
detach_role "$ID_ARC" "00000000-0000-7000-8000-000000000001"; assert_status 404
case_ "K6.9 archive an already-archived group"  "404"
api PATCH "/groups/$ID_ARC/archive"; assert_status 404
case_ "K6.10 PATCH an archived group"           "404"
api PATCH "/groups/$ID_ARC" "$(jq -nc '{name:"QA Ghost"}')"; assert_status 404

case_ "K6.11 the way back is a fresh insert"    "201 with a DIFFERENT id — 'it came back with the same id' would mean the one-way door has a hinge"
api POST /groups "$(group_body "$K_ARC" "QA Arc Reborn" "$D_OK" "$TEN_LANE")"
ID_ARC2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
if [ "$HTTP_STATUS" = "201" ] && [ -n "$ID_ARC2" ] && [ "$ID_ARC2" != "$ID_ARC" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, id '$ID_ARC2' vs '$ID_ARC'"; fi

case_ "K6.11b the root archive CASCADED onto its active entries" "both group_roles rows stamped — the pin archives an aggregate with its children, which is what makes a stamp-scoped unarchive meaningful on entities that HAVE one"
GOT=$(sql "SELECT count(*) FILTER (WHERE archived_at IS NOT NULL) || '/' || count(*) FROM group_roles WHERE group_id = '$ID_ARC';" | tr -d '[:space:]')
if [ "$GOT" = "2/2" ]; then pass_; else fail_ "stamped/total = '$GOT'"; fi

case_ "K6.11c and the stamped entries are STILL READABLE with the root" "2 — under an archived scope children load unfiltered, which is what keeps a retired group auditable (cross-ref K6.3b)"
api GET "/groups/$ID_ARC?includeArchived=true"
assert_json_at 200 '.data.roles | length' "2"

case_ "K6.12 child stamp-scoped unarchive is N/A" "the family proves a child removed BEFORE the root's archive stays archived after the root comes back — this root never comes back"
skip_ "Group mounts no unarchive: no mode, no route, no mutation. There is no restore for a child stamp to be scoped to, and GR13 is where the cost of that is asserted instead"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K7 — the three archive stamps, and what each one REPORTS. These fields exist to make a
#      state visible that a caller could not otherwise see; asserting only that they arrive
#      null would leave their whole job unproven.
# ═════════════════════════════════════════════════════════════════════════════════════════

K_ST=$(group_key stamp)
ID_ST=$(new_group "$K_ST" "$TEN_LANE" "$R_D") || exit 1

case_ "K7.1 a live counterpart"  "roleArchivedAt absent (omitempty on nil), roleKey rendered"
api GET "/groups/$ID_ST"
assert_json '[(.data.roles[0].roleKey), (.data.roles[0].roleArchivedAt // "absent")] | join(",")' "$K_ROLE_D,absent"

# The counterpart this family is about: archive the ROLE, then read the GROUP back.
api PATCH "/roles/$R_D/archive"
case_ "K7.2a the entry is STILL SERVED after its role is archived" "an inner join is NOT gated on the archived state of its target — ?includeArchived governs which ROOTS a read returns, never the rows a traversal reaches across into"
api GET "/groups/$ID_ST"
assert_json_at 200 '.data.roles | length' "1"
case_ "K7.2b roleKey still renders"  "the traversal keeps matching its archived counterpart"
assert_json '.data.roles[0].roleKey' "$K_ROLE_D"
case_ "K7.2c roleArchivedAt is now STAMPED" "the whole reason the field exists: without it an archived role arrives here indistinguishable from a live one"
assert_json '.data.roles[0].roleArchivedAt != null' "true"
case_ "K7.4 and the rules did NOT move"     "the group is still PATCHable by name — a rule reading roleArchivedAt instead of the probe would fail open here (cross-ref GR9)"
api PATCH "/groups/$ID_ST" "$(jq -nc '{name:"QA Stamp Renamed"}')"
assert_status 200

# K7.5 runs on a tenant of the lane's own, never on master: archiving a tenant suspends it,
# and suspending the tenant that owns the operator's token would end the run.
TEN_DOOM=$(new_tenant active "$(ws doom)") || exit 1
ID_DOOM=$(new_group "$(group_key doom)" "$TEN_DOOM") || exit 1
api PATCH "/tenants/$TEN_DOOM/archive"
case_ "K7.5a tenantArchivedAt is STAMPED once the owner is archived" "the ROOT join keeps matching its archived counterpart too"
api GET "/groups/$ID_DOOM?includeArchived=true"
assert_json_at 200 '.data.tenantArchivedAt != null' "true"
case_ "K7.5b tenantWorkspace and tenantStatus are STILL served" "an archived counterpart keeps supplying its columns"
assert_json '[(.data.tenantWorkspace | length > 0), (.data.tenantStatus | length > 0)] | all' "true"

K_OWN=$(group_key own)
ID_OWN=$(new_group "$K_OWN" "$TEN_LANE") || exit 1
case_ "K7.6a the group's own archivedAt while live" "null"
api GET "/groups/$ID_OWN"; assert_json_at 200 '.data.archivedAt' "null"
api PATCH "/groups/$ID_OWN/archive"
case_ "K7.6b stamped after the archive, and visible only through ?includeArchived" "terminal — there is no unarchive to return it to null"
api GET "/groups/$ID_OWN?includeArchived=true"; assert_json_at 200 '.data.archivedAt != null' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K8 — the read vocabulary. Everything the DTO declares, and only that.
# ═════════════════════════════════════════════════════════════════════════════════════════

# A dedicated tenant so the counts below are exact and untouched by every other family.
TEN_FILT=$(new_tenant active "$(ws filt)") || exit 1
api GET "/tenants/$TEN_FILT"; WS_FILT=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')
PFX="qa-flt-$(qa_slug_runid)"
for n in 1 2 3; do
  api POST /groups "$(jq -nc --arg k "$PFX-$n" --arg n "QA Filter Group $n" --arg d "$D_OK" --arg t "$TEN_FILT" \
    '{key:$k, name:$n, description:$d, tenantID:$t, roles:[]}')" >/dev/null
done

f200() { # f200 CASE EXPECT QUERY EXPECTED_COUNT
  case_ "$1" "$2"
  api GET "/groups?$3"
  assert_json_at 200 '.data | length' "$4"
}

f200 "K8.1 key.eq"          "200 — one row"                          "key.eq=$PFX-1" 1
f200 "K8.2 key.ne"          "200 — the other two of this tenant"     "tenantID.eq=$TEN_FILT&key.ne=$PFX-1" 2
f200 "K8.3 key.in"          "200 — two named rows"                   "key.in=$PFX-1,$PFX-3" 2
f200 "K8.4 key.startswith"  "200 — all three"                        "key.startswith=$PFX" 3
f200 "K8.5 key.istartswith" "200 — case-insensitive"                 "key.istartswith=$(printf '%s' "$PFX" | tr 'a-z' 'A-Z')" 3
f200 "K8.6 key.contains"    "200"                                    "key.contains=$PFX" 3
f200 "K8.7 key.icontains"   "200"                                    "key.icontains=$(printf '%s' "$PFX" | tr 'a-z' 'A-Z')" 3
f200 "K8.8 name.eq"         "200"                                    "key.startswith=$PFX&name.eq=QA%20Filter%20Group%202" 1
f200 "K8.9 name.ne — DECLARED here and NOT on Role" "200 — a suite copied from role.sh asserts a 400 for this, and would be wrong" "key.startswith=$PFX&name.ne=QA%20Filter%20Group%202" 2
f200 "K8.10 name.in"        "200"                                    "key.startswith=$PFX&name.in=QA%20Filter%20Group%201,QA%20Filter%20Group%203" 2
f200 "K8.11 name.startswith" "200"                                   "key.startswith=$PFX&name.startswith=QA%20Filter" 3
f200 "K8.12 name.istartswith" "200"                                  "key.startswith=$PFX&name.istartswith=qa%20filter" 3
f200 "K8.13 name.contains"  "200"                                    "key.startswith=$PFX&name.contains=Filter" 3
f200 "K8.14 name.icontains" "200"                                    "key.startswith=$PFX&name.icontains=filter" 3
f200 "K8.15 description.contains" "200"                              "key.startswith=$PFX&description.contains=anti-junk" 3
f200 "K8.16 description.icontains" "200"                             "key.startswith=$PFX&description.icontains=ANTI-JUNK" 3
f200 "K8.17 tenantID.eq"    "200"                                    "key.startswith=$PFX&tenantID.eq=$TEN_FILT" 3
f200 "K8.18 tenantID.in"    "200"                                    "key.startswith=$PFX&tenantID.in=$TEN_FILT,$TEN_OTHER" 3
f200 "K8.19 tenantWorkspace.eq — the ROOT join, addressable" "200 — the round's headline capability: a column that lives in another table, answering a filter" "key.startswith=$PFX&tenantWorkspace.eq=$WS_FILT" 3
f200 "K8.20 tenantWorkspace.in"         "200"                        "key.startswith=$PFX&tenantWorkspace.in=$WS_FILT" 3
f200 "K8.21 tenantWorkspace.startswith" "200"                        "key.startswith=$PFX&tenantWorkspace.startswith=qa-" 3
f200 "K8.22 tenantWorkspace.istartswith" "200"                       "key.startswith=$PFX&tenantWorkspace.istartswith=QA-" 3
f200 "K8.23 tenantWorkspace.contains"   "200"                        "key.startswith=$PFX&tenantWorkspace.contains=filt" 3
f200 "K8.24 tenantWorkspace.icontains"  "200"                        "key.startswith=$PFX&tenantWorkspace.icontains=FILT" 3
f200 "K8.25 tenantStatus.eq — the ROOT join"  "200"                  "key.startswith=$PFX&tenantStatus.eq=active" 3
f200 "K8.26 tenantStatus.in"            "200"                        "key.startswith=$PFX&tenantStatus.in=active,trial" 3
f200 "K8.27 createdAt.gte"              "200"                        "key.startswith=$PFX&createdAt.gte=2020-01-01T00:00:00Z" 3
f200 "K8.28 createdAt.lte"              "200 — none in the past"     "key.startswith=$PFX&createdAt.lte=2020-01-01T00:00:00Z" 0
f200 "K8.29 updatedAt.gte"              "200"                        "key.startswith=$PFX&updatedAt.gte=2020-01-01T00:00:00Z" 3
f200 "K8.30 updatedAt.lte"              "200"                        "key.startswith=$PFX&updatedAt.lte=2020-01-01T00:00:00Z" 0

ord() { # ord CASE EXPECT ORDERBY FIRST_KEY_JQ EXPECTED
  case_ "$1" "$2"
  api GET "/groups?key.startswith=$PFX&orderBy=$3"
  assert_json_at 200 "$4" "$5"
}
ord "K8.31 orderBy=key asc"     "200 — the first row is …-1" "key"        '.data[0].key' "$PFX-1"
ord "K8.32 orderBy=key desc"    "200 — the first row is …-3" "-key"       '.data[0].key' "$PFX-3"
ord "K8.33 orderBy=name asc"    "200"                        "name"       '.data[0].name' "QA Filter Group 1"
ord "K8.34 orderBy=name desc"   "200"                        "-name"      '.data[0].name' "QA Filter Group 3"
ord "K8.35 orderBy=tenantID asc"  "200 — three rows still"   "tenantID"   '.data | length' "3"
ord "K8.36 orderBy=tenantID desc" "200"                      "-tenantID"  '.data | length' "3"
ord "K8.37 orderBy=tenantWorkspace asc — sorting by ANOTHER TABLE's column" "200" "tenantWorkspace" '.data | length' "3"
ord "K8.38 orderBy=tenantWorkspace desc" "200"               "-tenantWorkspace" '.data | length' "3"
ord "K8.39 orderBy=tenantStatus asc"  "200"                  "tenantStatus"  '.data | length' "3"
ord "K8.40 orderBy=tenantStatus desc" "200"                  "-tenantStatus" '.data | length' "3"
ord "K8.41 orderBy=createdAt asc"  "200"                     "createdAt"  '.data[0].key' "$PFX-1"
ord "K8.42 orderBy=createdAt desc" "200"                     "-createdAt" '.data[0].key' "$PFX-3"
ord "K8.43 orderBy=updatedAt asc"  "200"                     "updatedAt"  '.data | length' "3"
ord "K8.44 orderBy=updatedAt desc" "200"                     "-updatedAt" '.data | length' "3"

case_ "K8.45 ?fields=key alone" "200 — the projection prunes after the load on this backing"
api GET "/groups?key.eq=$PFX-1&fields=key"
assert_json_at 200 '.data[0] | keys | join(",")' "key"
case_ "K8.46 ?fields=tenantWorkspace — a ROOT-join value, selectable" "200"
api GET "/groups?key.eq=$PFX-1&fields=tenantWorkspace"
assert_json_at 200 '.data[0].tenantWorkspace' "$WS_FILT"
case_ "K8.47 ?fields=archivedAt" "200"
api GET "/groups?key.eq=$PFX-1&fields=archivedAt"; assert_status 200
case_ "K8.48 ?fields=tenantArchivedAt" "200"
api GET "/groups?key.eq=$PFX-1&fields=tenantArchivedAt"; assert_status 200

case_ "K8.49 ?onlyTotal=true" "200 with pagination.totalCount and NO data, NO cursors"
api GET "/groups?key.startswith=$PFX&onlyTotal=true"
assert_json_at 200 '[(.pagination.totalCount), (.data == null or (.data | length) == 0)] | join(",")' "3,true"

case_ "K8.50 ?last=N alone serves the TAIL window" "200 — the last two rows, hasNextPage false"
api GET "/groups?key.startswith=$PFX&orderBy=key&last=2"
assert_json_at 200 '[(.data | length), (.pagination.hasNextPage)] | join(",")' "2,false"
case_ "K8.50b the tail window is the TAIL"        "the last row of the ordered set is in it"
assert_json '[.data[].key] | index("'"$PFX"'-3") != null' "true"

# ── the pagination envelope as a BICONDITIONAL ────────────────────────────────────────────
case_ "K8.51 page 1 of a two-per-page walk" "200 — two rows, hasNextPage true, hasPreviousPage false"
api GET "/groups?key.startswith=$PFX&orderBy=key&first=2"
assert_json_at 200 '[(.data | length), (.pagination.hasNextPage), (.pagination.hasPreviousPage)] | join(",")' "2,true,false"
P1_KEYS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].key] | join(",")')
END_CUR=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor // empty')

case_ "K8.52 endCursor is emitted exactly when hasNextPage" "the biconditional, not an implication"
assert_json '(.pagination.endCursor != null) == (.pagination.hasNextPage)' "true"
case_ "K8.53 startCursor is emitted exactly when hasPreviousPage" "same, the other edge"
assert_json '(.pagination.startCursor != null) == (.pagination.hasPreviousPage)' "true"

case_ "K8.54 page 2, reached by echoing endCursor into ?after=" "200 — one row, hasPreviousPage true"
api GET "/groups?key.startswith=$PFX&orderBy=key&first=2&after=$END_CUR"
assert_json_at 200 '[(.data | length), (.pagination.hasPreviousPage)] | join(",")' "1,true"
case_ "K8.55 page 2 is DISJOINT from page 1" "no row is walked twice"
P2_KEYS=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].key] | join(",")')
if [ -n "$P2_KEYS" ] && ! printf '%s' "$P1_KEYS" | grep -q "$P2_KEYS"; then pass_; else fail_ "page1=[$P1_KEYS] page2=[$P2_KEYS]"; fi
START_CUR=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.startCursor // empty')

case_ "K8.56 walking BACK with last+before returns page 1" "200 — the two rows page 1 carried"
api GET "/groups?key.startswith=$PFX&orderBy=key&last=2&before=$START_CUR"
assert_json_at 200 '[.data[].key] | join(",")' "$P1_KEYS"

case_ "K8.57 totalCount is truthful against a known set" "3, regardless of the page window"
api GET "/groups?key.startswith=$PFX&orderBy=key&first=1"
assert_json_at 200 '.pagination.totalCount' "3"

case_ "K8.58 ?includeArchived raises totalCount by exactly the archived count" "3 live, 4 with the one this case archives"
api POST /groups "$(jq -nc --arg k "$PFX-4" --arg d "$D_OK" --arg t "$TEN_FILT" '{key:$k, name:"QA Filter Group 4", description:$d, tenantID:$t, roles:[]}')"
ID_F4=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')
api PATCH "/groups/$ID_F4/archive"
api GET "/groups?key.startswith=$PFX&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "3"
case_ "K8.58b and with ?includeArchived=true"  "4"
api GET "/groups?key.startswith=$PFX&onlyTotal=true&includeArchived=true"
assert_json_at 200 '.pagination.totalCount' "4"

# ── the seeded baseline, derived from the tracked migration and never from an answer ───────
case_ "K8.59 the seeded baseline for groups" "0 — migration 0012 seeds NO group and no user_groups row; the platform operator holds *:* through user_roles"
api GET "/groups?tenantID.eq=$QA_MASTER_TENANT_ID&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "0"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K9 — rejected reads. The WHOLE typed-400 guard family, and the field named is the WIRE
#      TOKEN, not the bare field.
# ═════════════════════════════════════════════════════════════════════════════════════════

r400() { # r400 CASE EXPECT QUERY [KEY]
  case_ "$1" "$2"
  api GET "/groups?$3"
  assert_rest 400 "${4:-SchemaViolationNotification}"
}

r400 "K9.1 an unknown query key"                      "400 SchemaViolationNotification"  "bogus=1"
r400 "K9.2 an operator outside a leaf's allowlist"    "400 — key declares no gte"        "key.gte=x"
r400 "K9.3 ?description.eq — contains/icontains only" "400"                              "description.eq=x"
r400 "K9.4 ?tenantStatus.contains — the enum takes eq/in alone" "400"                    "tenantStatus.contains=act"
r400 "K9.5 ?search= — a RESERVED control the DTO never declared" "400 — the opt-in gate answers before any engine is consulted, which is also why UnsupportedCapabilityNotification is not the key here" "search=x"
r400 "K9.6 ?archivedAt.gte — served but NOT in byParams.filters" "400 — absent by choice: the read vocabulary has no isnull/notnull, so a declaration would only buy 'archived between these dates'" "archivedAt.gte=2020-01-01T00:00:00Z"
r400 "K9.7 ?tenantArchivedAt.gte — same choice, one table over" "400"                    "tenantArchivedAt.gte=2020-01-01T00:00:00Z"

# The round's headline negative: SERVED values that are still not addressable.
r400 "K9.8 ?roles.roleKey.eq — the child join's field"   "400 — served on every read, and addressable in NO criteria. Filtering a root by a 1:N child's field is a pushdown one root SELECT cannot express" "roles.roleKey.eq=x"
r400 "K9.9 ?roles.roleName.eq"                           "400 — same boundary"            "roles.roleName.eq=x"
r400 "K9.10 ?roles.roleID.eq — the entry's OWN stored column" "400 — the boundary is the 1:N shape, not whether the value was traversed. This is why 'which groups confer role X?' stays unanswerable" "roles.roleID.eq=00000000-0000-7000-8000-000000000001"
r400 "K9.11 ?roles.roleArchivedAt.gte"                   "400"                            "roles.roleArchivedAt.gte=2020-01-01T00:00:00Z"

r400 "K9.12 ?orderBy=description — filterable, orderable in NEITHER direction" "400 — ordering a listing by a 500-char free-text column is a blocking sort nobody asks for on purpose" "orderBy=description"
r400 "K9.13 ?orderBy=archivedAt"                      "400"                               "orderBy=archivedAt"
r400 "K9.14 ?orderBy=roles.roleKey — the 1:N boundary on the SORT side" "400"              "orderBy=roles.roleKey"
r400 "K9.15 ?orderBy=id — declarable, deliberately not declared" "400"                     "orderBy=id"
r400 "K9.16 ?orderBy=bogus"                           "400"                               "orderBy=bogus"
r400 "K9.17 ?fields=bogus"                            "400"                               "fields=bogus"
r400 "K9.18 ?fields=deletedAt — the pre-v0.74.0 token" "400 — the rename is real on the wire, not only in the column" "fields=deletedAt"
r400 "K9.19 ?fields=roles.archivedAt — the token spec.md §9 used to name" "400 — the entry field is RoleArchivedAt, so the token is roles.roleArchivedAt (corrected 2026-09-07, plan §0c)" "fields=roles.archivedAt"

case_ "K9.20 ?first above the view's ceiling" "400 LimitExceededNotification, value 100 — no query: block in the yaml and no per-view override, so the framework default is the ceiling"
api GET "/groups?first=101"
assert_rest 400 LimitExceededNotification
case_ "K9.20b and the ceiling it names"  "100"
assert_json '[.errors[]?.messages[]? | select(.notificationKey=="LimitExceededNotification") | .value] | first | tostring' "100"

r400 "K9.21 ?first=0"                                 "400"                               "first=0"
r400 "K9.22 ?first=abc"                               "400"                               "first=abc"
r400 "K9.23 ?first=-5"                                "400"                               "first=-5"
r400 "K9.24 ?first + ?last — mixed directions"        "400"                               "first=2&last=2"
r400 "K9.25 ?first + ?before"                         "400 — backward is last+before"     "first=2&before=xyz"
r400 "K9.26 ?last + ?after"                           "400"                               "last=2&after=xyz"
r400 "K9.27 ?after + ?before"                         "400"                               "after=x&before=y"
r400 "K9.28 ?onlyTotal=true beside ?first"            "400 — the only-total conflict matrix" "onlyTotal=true&first=10"
r400 "K9.29 ?onlyTotal=true beside ?orderBy"          "400"                               "onlyTotal=true&orderBy=key"
r400 "K9.30 ?onlyTotal=true beside ?fields"           "400"                               "onlyTotal=true&fields=key"
r400 "K9.31 ?onlyTotal=true beside ?after"            "400"                               "onlyTotal=true&after=xyz"

case_ "K9.32 ?onlyTotal=true BESIDE A FILTER" "200 — counting a filtered subset is the canonical use, and the conflict matrix must not swallow it"
api GET "/groups?key.startswith=$PFX&onlyTotal=true"; assert_status 200
case_ "K9.33 ?onlyTotal=true + ?includeArchived=true" "200 — same"
api GET "/groups?key.startswith=$PFX&onlyTotal=true&includeArchived=true"; assert_status 200
case_ "K9.34 ?onlyTotal=false beside ?first"          "200 — present-but-inactive never trips the matrix"
api GET "/groups?key.startswith=$PFX&onlyTotal=false&first=10"; assert_status 200

r400 "K9.35 ?after= is not a cursor"                  "400"                               "after=not-a-cursor"
r400 "K9.36 ?includeArchived=1 — the boolean takes true/false" "400"                      "includeArchived=1"
r400 "K9.37 ?onlyTotal= empty"                        "400"                               "onlyTotal="

case_ "K9.38 a cursor replayed under a DIFFERENT order" "400 — the structural check"
api GET "/groups?key.startswith=$PFX&orderBy=key&first=1"
CUR=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
api GET "/groups?key.startswith=$PFX&orderBy=name&first=1&after=$CUR"
assert_status 400
case_ "K9.39 the same cursor replayed with ?includeArchived=true" "400 — the context-hash check"
api GET "/groups?key.startswith=$PFX&orderBy=key&first=1&after=$CUR&includeArchived=true"
assert_status 400

# ── the by-id DTO opt-in gate: PRESENCE is what trips it ──────────────────────────────────
case_ "K9.40 by-id ?onlyTotal=false" "400 — FindGroupByIDRequest declares includeArchived and nothing else, and presence alone trips the gate"
api GET "/groups/$ID_GOLD?onlyTotal=false"; assert_rest 400 SchemaViolationNotification
case_ "K9.41 by-id ?fields=key"      "400 — same gate"
api GET "/groups/$ID_GOLD?fields=key"; assert_rest 400 SchemaViolationNotification
case_ "K9.42 by-id ?includeArchived=true" "200 — the positive control for the ONE control it does declare"
api GET "/groups/$ID_GOLD?includeArchived=true"; assert_status 200

case_ "K9.43 a filter VALUE outside the leaf's kind — a date"  "400 InvalidFilterValueNotification (pin >= v0.70.0)"
api GET "/groups?createdAt.gte=not-a-date"; assert_rest 400 InvalidFilterValueNotification
case_ "K9.44 a filter VALUE outside the leaf's kind — an identity column" "400 InvalidFilterValueNotification. Below v0.70.0 the same request was a 500 on a relational backing and an empty 200 page on Mongo, which is why an older suite has no case for it"
api GET "/groups?tenantID.eq=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "K9.45 UnsupportedCapabilityNotification is N/A on this entity" "it needs a control the DTO DOES declare and the backing cannot serve — this entity has none"
skip_ "FindGroupsRequest declares no Search field, so ?search= is refused at the wire wrapper before any engine is consulted (K9.5), and roles.* is declared nowhere, so the schema gate answers first there too (K9.8-K9.11). Asserting UnsupportedCapabilityNotification here would assert a lie, and would go RED against a correct service"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K10 — routing, absent verbs, and the by-id ADDRESS contract. A read and a write answer
#       DIFFERENTLY for the same malformed id, and both are asserted.
# ═════════════════════════════════════════════════════════════════════════════════════════

UNUSED_UUID="00000000-0000-7000-8000-0000000000ff"

case_ "K10.1 PATCH /groups/{id}/unarchive" "404 RouteNotFoundNotification — no route is registered at that path at all"
api PATCH "/groups/$ID_GOLD/unarchive"; assert_rest 404 RouteNotFoundNotification
case_ "K10.2 DELETE /groups/{id}"  "405 — the path is registered, the method is not. This is what proves no hard delete exists"
api DELETE "/groups/$ID_GOLD"; assert_rest 405 MethodNotAllowedNotification
case_ "K10.3 PUT /groups/{id}"     "405"
api PUT "/groups/$ID_GOLD" "$(jq -nc '{name:"x"}')"; assert_rest 405 MethodNotAllowedNotification
case_ "K10.4 POST /groups/{id}"    "405"
api POST "/groups/$ID_GOLD" "$(jq -nc '{name:"x"}')"; assert_rest 405 MethodNotAllowedNotification
case_ "K10.5 GET /groups/{id}/archive" "405 — same path, wrong method"
api GET "/groups/$ID_GOLD/archive"; assert_rest 405 MethodNotAllowedNotification
case_ "K10.6 DELETE on a child entry" "404 or 405 — whatever the router answers; what it must NEVER be is 204"
api DELETE "/groups/$ID_GOLD/roles/$UNUSED_UUID"; assert_status_not 204
case_ "K10.7 GET /does-not-exist"  "404 RouteNotFoundNotification"
api GET "/does-not-exist"; assert_rest 404 RouteNotFoundNotification
case_ "K10.8 GET /groups/{unused uuid}" "404 RecordNotFoundNotification"
api GET "/groups/$UNUSED_UUID"; assert_rest 404 RecordNotFoundNotification
case_ "K10.9 ATTACH onto a group id that addresses nothing" "404"
api POST "/groups/$UNUSED_UUID/roles" "$(jq -nc --arg r "$R_A" '{roleID:$r}')"; assert_status 404

case_ "K10.10 GET /groups/not-a-uuid — a READ address" "404 UnknownIDAddressNotification (pin >= v0.70.0)"
api GET "/groups/not-a-uuid"; assert_rest 404 UnknownIDAddressNotification
case_ "K10.11 PATCH /groups/not-a-uuid — a WRITE intention" "400 MalformedIDNotification — split by VERB, not by surface"
api PATCH "/groups/not-a-uuid" "$(jq -nc '{name:"QA"}')"; assert_rest 400 MalformedIDNotification
case_ "K10.12 PATCH /groups/not-a-uuid/archive" "400 MalformedIDNotification"
api PATCH "/groups/not-a-uuid/archive"; assert_rest 400 MalformedIDNotification
case_ "K10.13 POST /groups/not-a-uuid/roles" "400 MalformedIDNotification"
api POST "/groups/not-a-uuid/roles" "$(jq -nc --arg r "$R_A" '{roleID:$r}')"; assert_rest 400 MalformedIDNotification

case_ "K10.14 the CHILD address is NOT that contract" "404 RecordNotFoundNotification — GroupRoleID is a plain string path field, so the wire wrapper never inspects it and the value reaches RemoveGroupRoleByID. Two ids in one URL, answering differently"
detach_role "$ID_GOLD" "not-a-uuid"; assert_rest 404 RecordNotFoundNotification

case_ "K10.15 the 403 mode-not-allowed shape is N/A" "it needs a mode absent from Modes() while its route is still MOUNTED — this entity has none"
skip_ "Group's absent mode (unarchive) has no route at all, so it lands on the 404 arm (K10.1); its absent verb (DELETE) has a registered path, so it lands on 405 (K10.2). No …NotAllowedNotification is reachable on this entity"

# ═════════════════════════════════════════════════════════════════════════════════════════
# K11 — suite meta. The route inventory is an ENUMERATION of what EXISTS, cross-checked
#       against the source. A disagreement is a FINDING, not something the suite reconciles.
# ═════════════════════════════════════════════════════════════════════════════════════════

api GET /openapi.json "" -
OPENAPI="$HTTP_BODY"

case_ "K11.1 the document enumerates exactly SEVEN group routes" "four modes, seven routes — the collection carries a fifth verb of its own"
COUNT=$(printf '%s' "$OPENAPI" | jq '[.paths | to_entries[] | select(.key | startswith("/groups")) | .value | to_entries[]] | length')
if [ "$COUNT" = "7" ]; then pass_; else fail_ "openapi enumerates $COUNT group operations"; fi

case_ "K11.2 the five paths are the ones the source mounts" "/groups/ (the collection is mounted at path \"/\" under app.Group(\"/groups\"), so that trailing slash IS the mounted form), /groups/{id}, /groups/{id}/archive, /groups/{id}/roles, /groups/{id}/roles/{groupRoleId}/archive"
assert_json '[.paths | keys[] | select(startswith("/groups"))] | sort | join(" ")' \
  "/groups/ /groups/{id} /groups/{id}/archive /groups/{id}/roles /groups/{id}/roles/{groupRoleId}/archive"


case_ "K11.3 every group route declares a permission" "a route mounted OPEN is visible here, rather than merely absent from a count"
assert_json '[.paths | to_entries[] | select(.key | startswith("/groups")) | .value | to_entries[] | select((.value.description // "") | test("Required permission")) ] | length' "7"

case_ "K11.4 the declared literals are the FIVE of the model" "group:insert, update, archive, read — and group:grant on BOTH collection verbs, which is the fifth verb visible in the contract itself"
LITS=$(printf '%s' "$OPENAPI" | jq -r '[.paths | to_entries[] | select(.key | startswith("/groups")) | .value | tostring] | join(" ")' | grep -oE 'group:[a-z]+' | sort | uniq -c | awk '{printf "%s:%s ", $2, $1}')
if printf '%s' "$LITS" | grep -q 'group:grant:2' && printf '%s' "$LITS" | grep -q 'group:read:2'; then pass_; else fail_ "$LITS"; fi

case_ "K11.5 no unarchive operation is published" "0 — the absent mode is absent from the CONTRACT too, not merely unrouted"
N_UN=$(printf '%s' "$OPENAPI" | jq '[.paths | to_entries[] | select(.key | test("^/groups.*unarchive"))] | length')
if [ "$N_UN" = "0" ]; then pass_; else fail_ "$N_UN unarchive paths"; fi

case_ "K11.2b and the slash-less form still SERVES" "200 — the document publishes the mounted form; the router accepts both, which K1.3 already exercised"
api GET "/groups?first=1"
assert_status 200

qa_finish
