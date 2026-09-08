#!/usr/bin/env bash
# Lane: domain — §1b of specs/qa/tenant-contract/plan.md.
#
# These are the only cases in this suite the framework never had an opinion about. A service
# can satisfy every promise in qa/tenant.sh and qa/tenant_graphql.sh and still be wrong about
# the business; this file is where that shows up.
#
# Each row below has BOTH a positive case (an operation the rule permits, which must succeed)
# and a negative one (the input the rule must refuse). A rule with only a happy path is a rule
# nobody tested. Rows 1–4 were ranked critical by the maintainer on 2026-09-06.
#
# SOURCE is named per row: the entity spec, a rules.manual item, or "asked" — the maintainer's
# own answer, recorded verbatim in the plan.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init domain

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 1 [CRITICAL] — "An archived remnant MUST keep blocking the handle, because it is what
# URLs, logs and support conversations carry."
#   source: spec.md §A.2 rule 5 · service.facts.WorkspaceTaken activeOnly:false
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_1=$(ws r1)
case_ "R1+ a fresh handle is accepted" "201 — uniqueness refuses collisions, never novelty"
api POST /tenants "$(tenant_body "Row One Holder" "$WS_1" "The tenant that will hold this handle for good, archived or not." "active")"
assert_json_at 201 '.data.workspace' "$WS_1"
ID_1=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

api PATCH "/tenants/$ID_1/archive"

case_ "R1- the handle of an ARCHIVED tenant is still taken" "409 TenantWorkspaceAlreadyExistsNotification — reusing one would make a new tenant answer to a retired tenant's history"
api POST /tenants "$(tenant_body "Row One Collider" "$WS_1" "A tenant reaching for the handle a retired one still holds." "active")"
assert_rest 409 TenantWorkspaceAlreadyExistsNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 2 [CRITICAL] — "The handle reaches URLs, logs and external configuration; changing it
# breaks all three."
#   source: spec.md §A.2 rule 7 · rules.list.workspace-immutable · update.patchExcludes
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_2=$(ws r2)
api POST /tenants "$(tenant_body "Row Two Tenant" "$WS_2" "The tenant used to prove the handle cannot be moved by any request." "active")"
ID_2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "R2+ everything else about a tenant is editable" "200 — immutability is about the handle, not about the record"
api PATCH "/tenants/$ID_2" '{"name":"Row Two Renamed","description":"A different description, written after the tenant already existed."}'
assert_json_at 200 '.data.name' "Row Two Renamed"

case_ "R2- a request that names the handle changes nothing" "200 and the ORIGINAL handle — the field is structurally absent from the update body, so no request can reach it"
api PATCH "/tenants/$ID_2" "$(jq -nc '{name:"Row Two Renamed", workspace:"row-two-hijacked"}')"
api GET "/tenants/$ID_2"
assert_json_at 200 '.data.workspace' "$WS_2"

case_ "R2- and the handle it tried to take was never created" "0 rows — nothing was quietly written under the value the caller sent"
api GET "/tenants?workspace=row-two-hijacked&includeArchived=true"
assert_json_at 200 '(.data // []) | length' "0"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 3 [CRITICAL] — "A trial is a beginning — no tenant returns to it."
#   source: rules.list.status-transition (trial→active|suspended, active→suspended,
#           suspended→active)
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_3=$(ws r3)
api POST /tenants "$(tenant_body "Row Three Tenant" "$WS_3" "The tenant that walks the whole commercial lifecycle of this service." "trial")"
ID_3=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "R3+ trial to active" "200 — the ordinary conversion"
api PATCH "/tenants/$ID_3" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "R3+ active to suspended" "200 — suspension is not archiving; the tenant is still listed"
api PATCH "/tenants/$ID_3" '{"status":"suspended"}'
assert_json_at 200 '.data.status' "suspended"

case_ "R3+ suspended back to active" "200 — a suspension is reversible"
api PATCH "/tenants/$ID_3" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "R3- active back to trial" "422 InvalidTenantStatusTransitionNotification"
api PATCH "/tenants/$ID_3" '{"status":"trial"}'
assert_rest 422 InvalidTenantStatusTransitionNotification

WS_3B=$(ws r3b)
api POST /tenants "$(tenant_body "Row Three Suspended" "$WS_3B" "A tenant created suspended, to prove the other edge back into trial." "suspended")"
ID_3B=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "R3- suspended back to trial" "422 — the other edge into trial, and it must be closed too"
api PATCH "/tenants/$ID_3B" '{"status":"trial"}'
assert_rest 422 InvalidTenantStatusTransitionNotification

case_ "R3= a PATCH that leaves the status where it is" "200 — staying is always allowed; only a MOVE is guarded"
api PATCH "/tenants/$ID_3B" '{"status":"suspended"}'
assert_json_at 200 '.data.status' "suspended"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 4 [CRITICAL] — "Archiving forces Status to suspended… archived+active becomes an
# unrepresentable state."
#   source: spec.md §B Q10 · rules.manual.archive-forces-suspended
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_4=$(ws r4)
api POST /tenants "$(tenant_body "Row Four Tenant" "$WS_4" "An active tenant, archived to prove the status mutation reaches the row." "active")"
ID_4=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "R4+ archiving an ACTIVE tenant lands it SUSPENDED" "status 'suspended' — a mutation inside IfArchive, reaching the ROW and not merely the audit event"
api PATCH "/tenants/$ID_4/archive"
api GET "/tenants/$ID_4?includeArchived=true"
assert_json_at 200 '.data.status' "suspended"

WS_4B=$(ws r4b)
api POST /tenants "$(tenant_body "Row Four Trial" "$WS_4B" "A trial tenant, archived to prove the mutation does not depend on the old value." "trial")"
ID_4B=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "R4+ archiving a TRIAL tenant lands it SUSPENDED too" "'suspended' — the rule sets the field, it does not translate the old one"
api PATCH "/tenants/$ID_4B/archive"
api GET "/tenants/$ID_4B?includeArchived=true"
assert_json_at 200 '.data.status' "suspended"

case_ "R4- NO archived tenant anywhere reads back active or trial" "every row carrying one of those two statuses has a null archivedAt — the unrepresentable state is genuinely unreachable"
api GET "/tenants?includeArchived=true&status.in=active,trial&fields=id,archivedAt&first=100"
assert_json_at 200 '[.data[]? | .archivedAt] | map(. == null) | all' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 5 — "Unarchiving brings the tenant back suspended by consequence."
#   source: the same rule's own wording
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "R5+ unarchiving restores the row" "200 on the plain by-id read, archivedAt null"
api PATCH "/tenants/$ID_4/unarchive"
api GET "/tenants/$ID_4"
assert_json_at 200 '.data.archivedAt' "null"

case_ "R5- it does NOT come back active" "'suspended' — unarchiving restores the ROW, never the commercial state it held before"
assert_json '.data.status' "suspended"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 6 — "The description must differ from both Name and Workspace under a normalized
# comparison — case-folded, with whitespace and hyphens collapsed."
#   source: rules.manual.description-differs-from-name-and-workspace
#   asked 2026-09-06: a description that merely CONTAINS the name is LEGAL ("Passar está certo")
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "R6+ a description that CONTAINS the name" "201 — the rule targets the pasted name, not the mention (maintainer, 2026-09-06)"
api POST /tenants "$(tenant_body "Acme Comercio" "$(ws r6)" "Acme Comercio is the retail arm of the group in Brazil." "active")"
assert_status 201

case_ "R6- a description that IS the name, normalized" "422 TenantDescriptionMustDifferNotification — 'acme  comercio' folds onto 'Acme Comercio'"
api POST /tenants "$(tenant_body "Acme Comercio Dois" "$(ws r6)" "acme  comercio dois" "active")"
assert_rest 422 TenantDescriptionMustDifferNotification

WS_6=$(ws r6)
case_ "R6- a description that IS the workspace, normalized" "422 — hyphens collapse the same way spaces do, which is what catches the pasted handle"
api POST /tenants "$(tenant_body "Row Six Tenant" "$WS_6" "$(printf '%s' "$WS_6" | tr '-' ' ')" "active")"
assert_rest 422 TenantDescriptionMustDifferNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 7 — "Not a member of the reserved list (platform routes and phishing-prone words)."
#   source: internal/domain/vos/tenant_workspace.go
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "R7+ a handle outside the reserved list" "201"
api POST /tenants "$(tenant_body "Row Seven Tenant" "$(ws r7)" "An ordinary customer handle, colliding with nothing the platform keeps." "active")"
assert_status 201

for reserved in admin docs graphql readyz tenants; do
  case_ "R7- the reserved handle '$reserved'" "422 ReservedTenantWorkspaceNotification — a platform route or a phishing-adjacent word"
  api POST /tenants "$(tenant_body "Reserved Probe" "$reserved" "A tenant trying to register a handle the platform keeps for itself." "active")"
  assert_rest 422 ReservedTenantWorkspaceNotification
done

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 8 — "NOTHING IS NORMALIZED. A value that does not already comply is refused, never
# quietly repaired."
#   source: internal/domain/vos/tenant_workspace.go
# ═════════════════════════════════════════════════════════════════════════════════════════

WS_8=$(ws r8)
case_ "R8+ a handle leading with a digit" "201 — RFC 1123 relaxed RFC 1035's leading-letter rule; 3m9 and 3m-brasil are real companies"
api POST /tenants "$(tenant_body "Row Eight Tenant" "3m-${WS_8}" "A handle that leads with a digit, which the DNS label rules allow." "active")"
assert_status 201

for bad in "ACME-CORP" "-acme-lead" "acme-trail-" "ac" "aaaa"; do
  case_ "R8- the malformed handle '$bad'" "422 InvalidTenantWorkspaceNotification"
  api POST /tenants "$(tenant_body "Malformed Probe" "$bad" "A handle that does not comply with the DNS label shape at all." "active")"
  assert_rest 422 InvalidTenantWorkspaceNotification
done

case_ "R8- and 'ACME-CORP' was NOT quietly stored as 'acme-corp'" "0 rows — storing something the caller did not send is worse than a 422 on a handle that is immutable and reserved forever"
api GET "/tenants?workspace=acme-corp&includeArchived=true"
assert_json_at 200 '(.data // []) | length' "0"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 9 — the anti-junk composition rules of the two shared text types.
#   source: tenant.omnicore.yaml valueObjects + internal/domain/vos/
#   §A.5 of the spec is explicit that these raise the cost of garbage and do not prevent it;
#   the cases below assert the boundary, not a guarantee.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "R9+ a single-word company name" "201 — no word count on a display name: Nubank, Stone, Ambev, IBM are ordinary"
api POST /tenants "$(tenant_body "Nubank" "$(ws r9)" "A single word company name, which no surveyed product refuses." "active")"
assert_status 201

case_ "R9- a name below the two-rune floor" "422 InvalidDisplayNameNotification"
api POST /tenants "$(tenant_body "A" "$(ws r9)" "A perfectly ordinary description of a tenant organization." "active")"
assert_rest 422 InvalidDisplayNameNotification

case_ "R9- a one-word description" "422 InvalidDescriptionNotification — a description keeps the two-word rule a name does not"
api POST /tenants "$(tenant_body "Row Nine Tenant" "$(ws r9)" "aaabbbcccdddeee" "active")"
assert_rest 422 InvalidDescriptionNotification

case_ "R9+ a description in a non-Latin script" "201 — any letter outside the Latin script counts as a vowel, so a legitimate description is never refused as junk"
api POST /tenants "$(tenant_body "Row Nine Cyrillic" "$(ws r9)" "Роздрібні операції групи в Бразилії та інших країнах." "active")"
assert_status 201

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 10 — insert accepts ALL THREE statuses; the transition table guards the UPDATE only.
#   source: asked 2026-09-06 — "Sim, os três são legais no insert"
# ═════════════════════════════════════════════════════════════════════════════════════════

for st in trial active suspended; do
  case_ "R10+ a tenant created directly as '$st'" "201 — mandatory on insert, no server-side default, and no transition guard on the way in (maintainer, 2026-09-06)"
  api POST /tenants "$(tenant_body "Row Ten ${st}" "$(ws r10)" "A tenant created directly in the ${st} commercial state." "$st")"
  assert_json_at 201 '.data.status' "$st"
done

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW 11 — the seeded `master` tenant is archivable.
#   source: asked 2026-09-06 — "Aceitável, é operação de plataforma"
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "R11 the master tenant carries no archive guard" "recorded as a DECISION, and deliberately not exercised"
skip_ "by the maintainer's decision (2026-09-06) archiving 'master' is a legitimate platform operation, so there is no refusal to assert; and calling it would suspend the tenant that owns the wildcard role and the bootstrap administrator, invalidating the token every later case depends on"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════
#
#   P E R M I S S I O N  —  §1b of specs/qa/permission-contract/plan.md
#
#   Rows P1–P10. Same discipline as R1–R11 above: each row carries a POSITIVE case and a
#   NEGATIVE one, and the source is named. Rows P1–P4 were ranked critical by the maintainer
#   on 2026-09-07.
#
# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════

PD_OK="A description long enough to satisfy the shared anti-junk floor of this service."

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P1 [CRITICAL] — "An archived tenant:export does not block a fresh one — re-inserting is
# the ONLY route back now that /unarchive does not exist."
#   source: spec.md §B Q3 + §B Q5 · service.facts.PermissionKeyTaken activeOnly:true
#
# This is the EXACT OPPOSITE of tenant row 1 above, and the two are the sharpest pair of
# expectations in this suite: a handle an archived tenant holds still blocks; a pair an
# archived permission holds does not. Reading either into the other is the mistake this
# adjacency exists to prevent.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P1=$(pair_resource p1)
ID_P1=$(new_permission "$R_P1" export "Export rows of the P1 fixture resource, which this row archives and then re-creates.") || exit 1

case_ "P1a a pair an ACTIVE row holds is refused" "409 PermissionAlreadyExistsNotification"
api POST /permissions "$(permission_body "$R_P1" export "A second live claim on the P1 fixture pair, which the active-only pre-check refuses.")"
assert_rest 409 PermissionAlreadyExistsNotification

case_ "P1b the refusal names the pair and hands it back" "field 'permission', value '$R_P1:export'"
assert_json '[.errors[].messages[] | select(.notificationKey=="PermissionAlreadyExistsNotification") | [.field, .value] | join("|")] | first' "permission|$R_P1:export"

api PATCH "/permissions/$ID_P1/archive"

case_ "P1c once archived, the SAME pair is free again" "201 — the partial unique index is WHERE archived_at IS NULL"
api POST /permissions "$(permission_body "$R_P1" export "The re-created P1 fixture: a retired permission comes back as a new row, granted explicitly.")"
assert_status 201
ID_P1B=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "P1d and it comes back with a NEW id, which is the whole point" "a different id — old role_permissions rows point at the archived one and stay dead"
if [ -n "$ID_P1B" ] && [ "$ID_P1B" != "$ID_P1" ]; then pass_; else fail_ "reborn id '$ID_P1B' vs archived id '$ID_P1'"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P2 [CRITICAL] — "`*` is legal only as the ENTIRE part, and a `*` resource forces a `*`
# action — the claim matcher honours exactly three shapes, so anything else is a row that
# matches nothing while reading like a grant."
#   source: spec.md §7a rule 2 + §7b rule 6 · vos/permission_key.go
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "P2a a wildcard resource with a concrete action" "422 UnmatchablePermissionKeyNotification — *:read matches no route while reading like a sweeping grant"
api POST /permissions "$(permission_body "*" read "$PD_OK")"
assert_rest 422 UnmatchablePermissionKeyNotification

case_ "P2b it is reported on ACTION, the half that has to change" "field 'action' — not 'permission', and not 'resource'"
assert_json '[.errors[].messages[] | select(.notificationKey=="UnmatchablePermissionKeyNotification") | .field] | first' "action"

case_ "P2c a wildcard inside a resource PATH" "422 InvalidResourceNameNotification — a partially wildcarded value fits none of the three shapes"
api POST /permissions "$(permission_body "user:*" read "$PD_OK")"
assert_rest 422 InvalidResourceNameNotification

case_ "P2d a wildcard mixed into a slug" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body "ten*" read "$PD_OK")"
assert_rest 422 InvalidResourceNameNotification

case_ "P2e a concrete resource with a wildcard action" "201 — resource:* IS one of the three shapes"
api POST /permissions "$(permission_body "$(pair_resource p2)" "*" "Every action on the P2 fixture resource, which is a legal resource-wide grant.")"
assert_status 201

case_ "P2f the full wildcard pair" "201 — *:* is the super-admin row, one row and total power"
R_P2W=$(pair_resource p2w)
api POST /permissions "$(permission_body "$R_P2W" "*" "Every action on the second P2 fixture resource, proving the wildcard action is admitted.")"
assert_status 201

case_ "P2g the wildcard row renders with the wildcard intact" "$R_P2W:* — never normalised into something else"
assert_json '.data.permission' "$R_P2W:*"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P3 [CRITICAL] — "The pair IS the permission's identity in every issued token and every
# existing grant; editing it would rewrite what all of them mean, retroactively and
# invisibly."
#   source: spec.md §B Q2 · rules.list.key-immutable · update.patchExcludes
#
# Two layers, and they are not redundant: patchExcludes closes the door (there is no field to
# send), and the rule guards the value on every update path whatever door it came through.
# Only the first is reachable from the wire, so only the first is asserted — see permission.sh
# E9 and the plan's §1b row 3.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P3=$(pair_resource p3)
ID_P3=$(new_permission "$R_P3" read "The P3 fixture, whose pair must survive every attempt to move it through an update.") || exit 1

case_ "P3a the editable field is genuinely editable" "200 — improving the wording after operators read it is the whole of this verb"
api PATCH "/permissions/$ID_P3" '{"description":"The P3 fixture, with the wording an operator improved after reading the catalog."}'
assert_json_at 200 '.data.description' "The P3 fixture, with the wording an operator improved after reading the catalog."

case_ "P3b and the pair did not move" "$R_P3:read"
assert_json '.data.permission' "$R_P3:read"

case_ "P3c a patch that tries to move the pair changes nothing" "200, and the pair is untouched — the halves are not members of the body"
api PATCH "/permissions/$ID_P3" '{"description":"The P3 fixture, patched by a caller who also tried to rewrite the pair itself.","resource":"seized","action":"taken"}'
assert_json_at 200 '.data.permission' "$R_P3:read"

case_ "P3d nothing was written under the seized pair" "0 rows — the assignment never happened, so there is nothing to find"
api GET "/permissions?resource.eq=seized"
assert_json_at 200 '.data | length' "0"

case_ "P3e the immutability notification is UNREACHABLE from the wire" "recorded, and deliberately not asserted"
skip_ "PermissionKeyIsImmutableNotification guards a door no mounted surface can open: PatchPermissionRequest declares only 'description' (patchExcludes: [Permission]) and GraphQL mounts the same shape, so no request can provoke the key. The rule is a belt-and-braces layer behind a structural cut; P3c/P3d assert the EFFECT instead, which is the only honest assertion available"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P4 [CRITICAL] — "Three columns go in, two go out."
#   source: spec.md header + §B Q7 + §B Q8 · the Request and Response DTOs
#
# The mechanism is asserted exhaustively in qa/permission.sh (E2, E6, E7) and
# qa/permission_graphql.sh (G2). What belongs HERE is the rule as a single statement: the two
# halves are queryable and invisible, and the rendered pair is visible and unqueryable. Four
# cases, one per corner, so the doctrine has one place a reader can check it whole.
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P4=$(pair_resource p4)
new_permission "$R_P4" read "The P4 doctrine fixture, queryable by two halves that no response body carries." >/dev/null || exit 1

case_ "P4a corner 1 — a hidden half is QUERYABLE" "the fixture row, found by a column no caller can read"
api GET "/permissions?resource.eq=$R_P4&action.eq=read"
assert_json_at 200 '.data[0].permission' "$R_P4:read"

case_ "P4b corner 2 — and it is INVISIBLE in the answer it just produced" "neither key is present in the row the filter matched"
assert_json '[(.data[0] | has("resource")), (.data[0] | has("action"))] | @csv' "false,false"

case_ "P4c corner 3 — the derived value is VISIBLE" "$R_P4:read, rendered per row from the two sources"
assert_json '.data[0].permission' "$R_P4:read"

case_ "P4d corner 4 — and it is UNQUERYABLE, by construction" "400 — a computed path backs no column, so neither a filter nor an orderBy can reach it"
api GET "/permissions?permission.eq=$R_P4:read"
assert_rest 400 SchemaViolationNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P5 — "NOTHING IS NORMALIZED. A value that does not already comply is refused, never
# repaired — it matters more here than anywhere, because the rendered pair is compared
# byte-for-byte against a token claim."
#   source: vos/permission_key.go
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P5=$(pair_resource p5)

case_ "P5a an uppercase resource is refused, not lowercased" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body "$(printf '%s' "$R_P5" | tr 'a-z' 'A-Z')" read "$PD_OK")"
assert_rest 422 InvalidResourceNameNotification

case_ "P5b a padded resource is refused, not trimmed" "422 InvalidResourceNameNotification"
api POST /permissions "$(permission_body " $R_P5 " read "$PD_OK")"
assert_rest 422 InvalidResourceNameNotification

case_ "P5c and NO normalised variant was quietly stored" "0 rows — a repaired value would authorize nothing and explain nothing"
api GET "/permissions?resource.eq=$R_P5&includeArchived=true"
assert_json_at 200 '.data | length' "0"

# The resource is built from pair_resource, not from the run id alone: qa/permission.sh E3.18
# already inserts a digit-leading fixture in the same database and the same run, and a pair is
# unique among ACTIVE rows — a fixed name here would 409 on the collision instead of proving
# anything about normalisation.
R_P5D="3d-$(pair_resource p5d)"
case_ "P5d a compliant value is stored exactly as sent" "201, and the render echoes the bytes back"
api POST /permissions "$(permission_body "$R_P5D" read "Read the digit-leading P5 fixture, stored byte for byte as the caller sent it.")"
assert_json_at 201 '.data.permission' "$R_P5D:read"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P6 — "The description must explain the permission, not repeat it — under a fold that
# drops whitespace, colons and hyphens."
#   source: rules.manual.description-does-not-echo-key · permission_rules_manual.go
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P6=$(pair_resource p6)

for echoed in "$R_P6:read" "$R_P6-read" "$R_P6 read"; do
  case_ "P6a a description that merely echoes the pair ('$echoed')" "422 PermissionDescriptionEchoesKeyNotification — the fold drops colons, hyphens and whitespace, so all three are the same value to this rule"
  api POST /permissions "$(permission_body "$R_P6" read "$echoed")"
  assert_rest 422 PermissionDescriptionEchoesKeyNotification
done

case_ "P6b it is reported against the description" "field 'description' — the complaint is about the description, not about the pair"
assert_json '[.errors[].messages[] | select(.notificationKey=="PermissionDescriptionEchoesKeyNotification") | .field] | first' "description"

case_ "P6c a description that CONTAINS the pair but explains it" "201 — the rule catches the lazy paste and nothing more"
api POST /permissions "$(permission_body "$R_P6" read "Holding $R_P6:read lets an operator list the P6 fixture rows and open one by its identifier.")"
assert_status 201

# What this case is FOR is the second assertion, not the first: permission_rules_manual.go
# states that an empty description is the Description value object's complaint to make, so the
# echo rule "stays quiet about it rather than saying the same thing twice". The emptiness is
# part of that rule's CONDITION, never an early return. So: the write is refused (by the
# framework's own required check, the same key an empty composite half raises in E3.1/E3.2),
# and PermissionDescriptionEchoesKeyNotification must NOT be among the keys.
case_ "P6d an empty description is refused by the framework, not by the echo rule" "422 RequiredFieldNotification on 'description'"
api POST /permissions "$(permission_body "$(pair_resource p6b)" read "")"
assert_rest_field 422 RequiredFieldNotification description

case_ "P6e and the echo rule stays SILENT on it" "no PermissionDescriptionEchoesKeyNotification — one problem is reported once, not twice"
assert_json '[.errors[].messages[].notificationKey] | map(select(. == "PermissionDescriptionEchoesKeyNotification")) | length' "0"

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P7 — "The resource may be a colon-joined PATH; the action is exactly ONE segment —
# which is what makes the LAST colon segment always the action, so user:profile:read parses
# back one way."
#   source: spec.md §7a rules 3–4
# ═════════════════════════════════════════════════════════════════════════════════════════

R_P7=$(pair_resource p7)

case_ "P7a a hierarchical resource is accepted" "201 — the framework's own example gates users:profile:read"
api POST /permissions "$(permission_body "$R_P7:profile" read "Read the profile facet of the P7 fixture resource, exercising the colon-joined path.")"
assert_status 201

case_ "P7b and the render puts the action LAST, so it parses back one way" "$R_P7:profile:read"
assert_json '.data.permission' "$R_P7:profile:read"

case_ "P7c an action carrying a colon is refused" "422 InvalidActionNameNotification — two colons in the action would make the render ambiguous"
api POST /permissions "$(permission_body "$R_P7" "profile:read" "$PD_OK")"
assert_rest 422 InvalidActionNameNotification

case_ "P7d a three-level resource is still one permission" "201 — the vocabulary is open in depth as well as in wording"
api POST /permissions "$(permission_body "$R_P7:profile:avatar" read "Read the avatar of the profile facet of the P7 fixture, three levels deep.")"
assert_status 201

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P8 — the reused vos.Description anti-junk floor: 15–500 runes, >= 2 words, >= 5 distinct
# runes, >= 1 vowel, no run of 4.
#   source: spec.md §2 (reuse vos.Description) · vos/description.go
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "P8a a description under the 15-rune floor" "422 InvalidDescriptionNotification"
api POST /permissions "$(permission_body "$(pair_resource p8)" read "Reads rows.")"
assert_rest 422 InvalidDescriptionNotification

case_ "P8b a single word, long enough by length alone" "422 InvalidDescriptionNotification — >= 2 words is part of the floor"
api POST /permissions "$(permission_body "$(pair_resource p8)" read "Readsthecatalogrows")"
assert_rest 422 InvalidDescriptionNotification

case_ "P8c a run of four identical runes" "422 InvalidDescriptionNotification — the same anti-junk predicate the pair halves use"
api POST /permissions "$(permission_body "$(pair_resource p8)" read "Reads the caaaatalog rows of this fixture entry.")"
assert_rest 422 InvalidDescriptionNotification

case_ "P8d a real operator description" "201 — the floor admits ordinary prose without argument"
api POST /permissions "$(permission_body "$(pair_resource p8)" read "List the P8 fixture rows and open a single one by its identifier.")"
assert_status 201

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW P9 + P10 — the revocation chain, on a fixture of the suite's OWN.
#
#   P9  "Archiving a permission still granted to a role is legal; the grant stays pointing at
#        the archived row and that is HISTORY, not an error."
#        source: asked 2026-09-07 — "Está certo"
#   P10 "A retired permission must actually stop authorizing."
#        source: asked 2026-09-07 — "Sim, com fixture própria"
#
# These two are the only cases in the whole suite that prove the catalog MEANS something: that
# a row here is not a label but the thing a token is built from. They share one chain —
# permission -> role -> user -> token -> reach -> archive -> reach again — and the chain uses
# nothing seeded, which is why it can sit here instead of having to be last and irreversible.
# The seeded *:* row and the bootstrap admin are never touched.
# ═════════════════════════════════════════════════════════════════════════════════════════

REV_RESOURCE=$(pair_resource rev)
REV_EMAIL="qa-revocation-${QA_RUN_ID}@authcore.local"
REV_PASS='Qa!Revoke2026'
REV_PASS2='Qa!Revoke2026b'

# THE ROLE CARRIES TWO PERMISSIONS ON PURPOSE, and the reason is what makes the chain sharp.
# A fixture pair authorizes no route, so on its own it could prove the claim shrank but never
# that a REACH was lost. The seeded `tenant:read` does gate a route — but archiving a seeded
# row would break every later case in the run. So the role holds BOTH, and only the fixture
# pair is retired: the token must lose exactly the row that was archived and keep the one that
# was not. A revocation that took everything with it would pass a cruder test for the wrong
# reason, and P10b is the case that refuses to let it.
REV_PERM_ID=$(new_permission "$REV_RESOURCE" read "The revocation fixture pair, granted to a role and then retired to prove a token loses it.") || exit 1
TENANT_READ_ID="01990000-0000-7000-8000-00000000001e"

api POST /roles "$(jq -nc --arg k "qa-revoke-$QA_RUN_ID" --arg p1 "$REV_PERM_ID" --arg p2 "$TENANT_READ_ID" \
  '{key:$k, name:"QA Revocation", description:"Carries the revocation fixture pair and tenant:read, so a revocation can be told from a collapse.", permissions:[{permissionID:$p1},{permissionID:$p2}]}')"
REV_ROLE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

case_ "P9a a role can be granted the fixture pair" "201 — the in-catalog rule accepts an ACTIVE catalog row"
if [ -n "$REV_ROLE" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no role id"; fi

api POST /users "$(jq -nc --arg e "$REV_EMAIL" --arg p "$REV_PASS" --arg r "$REV_ROLE" \
  '{givenName:"Qa", familyName:"Revocation", email:$e, status:"active", password:$p, passwordConfirmation:$p, roles:[{roleID:$r}]}')"
REV_USER=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

case_ "P9b a user can hold that role" "201"
if [ -n "$REV_USER" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no user id"; fi

# A user created through the API is born must_change_password=TRUE, so its FIRST token carries
# user:change-password and nothing else. Rotating is what makes the reach below mean anything.
REV_BOOT=$(qa_login "$REV_EMAIL" "$REV_PASS")
api PATCH "/users/$REV_USER/password" \
  "$(jq -nc --arg cp "$REV_PASS" --arg np "$REV_PASS2" '{currentPassword:$cp, password:$np, passwordConfirmation:$np}')" "$REV_BOOT"
REV_TOKEN=$(qa_login "$REV_EMAIL" "$REV_PASS2")

case_ "P9c the principal's token carries the fixture pair BEFORE the archive" "$REV_RESOURCE:read is among its permissions claim"
if printf '%s' "$(jwt_claim "$REV_TOKEN" permissions)" | grep -q "$REV_RESOURCE:read"; then pass_; else fail_ "claim: $(jwt_claim "$REV_TOKEN" permissions)"; fi

case_ "P9d and it genuinely reaches the route tenant:read gates" "200 — the reach is real, not merely claimed"
api GET "/tenants?first=1" "" "$REV_TOKEN"
assert_status 200

case_ "P9e archiving a pair a role still grants is ACCEPTED" "204 — no rule refuses it, and the maintainer confirmed that is correct (2026-09-07)"
api PATCH "/permissions/$REV_PERM_ID/archive"
assert_empty_body 204

case_ "P9f the grant row SURVIVES, pointing at the archived permission" "1 role_permissions row — the grant is history, not an error; asserted by SQL because no endpoint exposes the join"
GOT=$(sql "SELECT count(*) FROM role_permissions WHERE role_id = '$REV_ROLE' AND permission_id = '$REV_PERM_ID';" | tr -d '[:space:]')
if [ "$GOT" = "1" ]; then pass_; else fail_ "role_permissions rows = '$GOT'"; fi

case_ "P9g and the archived row is still readable, so the grant stays explicable" "200 with ?includeArchived — an access review must be able to see what a past grant meant"
api GET "/permissions/$REV_PERM_ID?includeArchived=true"
assert_json_at 200 '.data.permission' "$REV_RESOURCE:read"

case_ "P10a a FRESHLY reissued token no longer carries the retired pair" "$REV_RESOURCE:read is gone from the permissions claim"
REV_TOKEN2=$(qa_login "$REV_EMAIL" "$REV_PASS2")
if printf '%s' "$(jwt_claim "$REV_TOKEN2" permissions)" | grep -q "$REV_RESOURCE:read"; then
  fail_ "the retired pair is still in the claim: $(jwt_claim "$REV_TOKEN2" permissions)"
else
  pass_
fi

case_ "P10b the permission that was NOT archived is still there" "tenant:read survives — a revocation that took everything with it would pass P10a for the wrong reason"
if printf '%s' "$(jwt_claim "$REV_TOKEN2" permissions)" | grep -q "tenant:read"; then pass_; else fail_ "claim: $(jwt_claim "$REV_TOKEN2" permissions)"; fi

case_ "P10c and the reach that pair gates still works" "200 — the principal lost exactly the row that was retired, and nothing else"
api GET "/tenants?first=1" "" "$REV_TOKEN2"
assert_status 200


# ═════════════════════════════════════════════════════════════════════════════════════════
# ROWS RL1-RL11 — specs/qa/role-contract/plan.md §1b
#
# The business rules of the Role aggregate: the ones the FRAMEWORK never had an opinion about.
# Eight of the eleven were called critical at the gate, and every one of them needs a caller
# who is NOT a super-admin — which is why principal E exists and why this block skips loudly
# rather than silently when it could not be built.
# ═════════════════════════════════════════════════════════════════════════════════════════

RL_D="A role description long enough to satisfy the shared anti-junk floor this service applies."
RL_TEN=$(new_tenant active "$(ws rldom)") || exit 1
RL_P_TENANT_READ=$(permission_id_of tenant read)
RL_P_PERM_ARCHIVE=$(permission_id_of permission archive)
RL_P_ROLE_READ=$(permission_id_of role read)

# ── RL1 / RL1b — no wildcard grant, and NO CALLER IS EXEMPT ───────────────────────────────
#
# The negative is aimed at the STRONGEST possible caller. If anyone were exempt it would be the
# *:* super-admin, and the seed migration's own header says this refusal is exactly why a
# migration is the only way a super-admin can come into existence.

RL_ID1=$(new_role "$(role_key rl1)" "$RL_TEN" "$RL_P_TENANT_READ") || exit 1

case_ "RL1+ the admin grants a CONCRETE catalog permission" "201 — the rule is about wildcards, not about grants"
api POST "/roles/$RL_ID1/permissions" "$(jq -nc --arg p "$RL_P_ROLE_READ" '{permissionID:$p}')"
assert_status 201

case_ "RL1- the *:* SUPER-ADMIN grants the seeded *:* row" "403 CannotGrantWildcardPermissionNotification — no caller is exempt, and this is where an exemption would hide"
api POST "/roles/$RL_ID1/permissions" "$(jq -nc --arg p "$QA_WILDCARD_PERMISSION_ID" '{permissionID:$p}')"
assert_rest 403 CannotGrantWildcardPermissionNotification

case_ "RL1b the wildcard rule runs BEFORE the escalation rule" "403 and never 500 — Identity.HasPermission PANICS on any argument containing '*', so a service that reordered the two would crash the request on exactly the case the pair exists to stop"
if [ "$HTTP_STATUS" = "403" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

case_ "RL1- and nothing was written under the refused grant" "0 role_permissions rows pointing at the wildcard from this role — a refusal that still inserted would be worse than one that did not refuse"
GOT=$(sql "SELECT count(*) FROM role_permissions WHERE role_id = '$RL_ID1' AND permission_id = '$QA_WILDCARD_PERMISSION_ID';" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else fail_ "role_permissions rows = '$GOT'"; fi

case_ "RL2b the super-admin exemption is FREE, not special-cased" "201 — HasPermission answers true for ANY concrete permission when the claim set holds *:*, so 'you may only grant what you hold, unless you are a super-admin' is one question and not two"
api POST "/roles/$RL_ID1/permissions" "$(jq -nc --arg p "$RL_P_PERM_ARCHIVE" '{permissionID:$p}')"
assert_status 201

# ── RL2 / RL3 / RL3b / RL3c — the scoped principal's whole block ──────────────────────────

if [ -z "${QA_TOKEN_SCOPED:-}" ]; then
  case_ "RL2 / RL3 / RL3b / RL3c — the scoped principal's rows" "the no-escalation rule, the write-side row scope and the read-side one"
  skip_ "principal E could not be provisioned by qa/run.sh, so no non-super-admin caller exists. Every negative in these four rows needs one: a super-admin passes the escalation rule by construction and crosses the row scope by design, so running them as the admin would prove the opposite of what they are for"
else
  RL_ID_OWN=""

  case_ "RL3+ the scoped principal creates a role OMITTING tenantID" "201 in its OWN tenant — absent means 'mine', which is what assignedFrom: identity-claim buys"
  api POST /roles "$(jq -nc --arg k "$(role_key rl3)" --arg d "$RL_D" --arg p "$RL_P_TENANT_READ" \
    '{key:$k, name:"QA Scoped Role", description:$d, permissions:[{permissionID:$p}]}')" "$QA_TOKEN_SCOPED"
  assert_status 201
  RL_ID_OWN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

  case_ "RL3+ and the owner it landed under is the caller's own claim" "the scoped tenant — not a value the body carried"
  assert_json '.data.tenantID' "$QA_TENANT_SCOPED"

  case_ "RL3- the same principal names the MASTER tenant" "403 TenantMismatchNotification — the claim decides what a caller MAY write, and refuseForeignTenant is what answers"
  api POST /roles "$(jq -nc --arg k "$(role_key rl3x)" --arg d "$RL_D" --arg t "$QA_MASTER_TENANT_ID" \
    '{key:$k, name:"QA Foreign Role", description:$d, tenantID:$t, permissions:[]}')" "$QA_TOKEN_SCOPED"
  assert_rest 403 TenantMismatchNotification

  case_ "RL3- the refusal names the owner field" "field 'tenantID'"
  assert_rest_field 403 TenantMismatchNotification tenantID

  case_ "RL3- ARCHIVING another tenant's role is refused too" "403 TenantMismatchNotification — refuseForeignTenant runs under IfArchive as well, and the write side is NOT filtered by ToCriteria, so the row loads and the RULE is what refuses"
  api PATCH "/roles/$RL_ID1/archive" "" "$QA_TOKEN_SCOPED"
  assert_rest 403 TenantMismatchNotification

  case_ "RL3- and that role is still active" "200 as the admin — the refusal refused, it did not half-apply"
  api GET "/roles/$RL_ID1"
  assert_status 200

  case_ "RL3b- a by-id read of another tenant's role answers NOT FOUND" "404 RecordNotFoundNotification — NOT 403: the row does not exist for this caller, which leaks nothing about who else exists"
  api GET "/roles/$QA_MASTER_ROLE_ID" "" "$QA_TOKEN_SCOPED"
  assert_rest 404 RecordNotFoundNotification

  case_ "RL3b- and the master role appears in NO page of its listing" "absent by ID, not by count — a leak here answers 200, which is exactly why it needs its own case"
  api GET "/roles?first=100" "" "$QA_TOKEN_SCOPED"
  assert_json_at 200 '[.data[].id] | index("'"$QA_MASTER_ROLE_ID"'") == null' "true"

  case_ "RL3b+ while its OWN role is reachable" "200 — the scope narrows what is reached, it does not refuse"
  if [ -n "$RL_ID_OWN" ]; then
    api GET "/roles/$RL_ID_OWN" "" "$QA_TOKEN_SCOPED"
    assert_status 200
  else
    fail_ "the scoped principal's own role was never created"
  fi

  case_ "RL3c the *:* holder CROSSES the row scope" "the master role IS in the admin's listing — a service that filtered everyone would pass RL3b for the wrong reason and make platform support impossible"
  api GET "/roles?first=100"
  assert_json_at 200 '[.data[].id] | index("'"$QA_MASTER_ROLE_ID"'") != null' "true"

  case_ "RL2+ the scoped principal grants a permission IT HOLDS" "201 — tenant:read is in its own bundle"
  api POST "/roles/$RL_ID_OWN/permissions" "$(jq -nc --arg p "$RL_P_ROLE_READ" '{permissionID:$p}')" "$QA_TOKEN_SCOPED"
  assert_status 201

  case_ "RL2- the same principal grants one it does NOT hold" "403 CannotGrantUnheldPermissionNotification — any principal who may touch a role could otherwise grant themselves the whole catalog"
  api POST "/roles/$RL_ID_OWN/permissions" "$(jq -nc --arg p "$RL_P_PERM_ARCHIVE" '{permissionID:$p}')" "$QA_TOKEN_SCOPED"
  assert_rest 403 CannotGrantUnheldPermissionNotification

  case_ "RL2- the refusal names the collection and the offending id" "field 'permissions', value the permission id — so a caller granting ten knows WHICH one was refused"
  assert_json '[.errors[].messages[] | select(.notificationKey=="CannotGrantUnheldPermissionNotification") | .value] | join(",")' "$RL_P_PERM_ARCHIVE"

  case_ "RL2- and the grant was not written" "0 rows — the whole write rolled back"
  GOT=$(sql "SELECT count(*) FROM role_permissions WHERE role_id = '$RL_ID_OWN' AND permission_id = '$RL_P_PERM_ARCHIVE';" | tr -d '[:space:]')
  if [ "$GOT" = "0" ]; then pass_; else fail_ "role_permissions rows = '$GOT'"; fi
fi

# ── RL4 — the granted permission must be in the catalog AND still active ──────────────────

RL_ID4=$(new_role "$(role_key rl4)" "$RL_TEN") || exit 1

case_ "RL4+ granting a LIVE catalog id" "201"
api POST "/roles/$RL_ID4/permissions" "$(jq -nc --arg p "$RL_P_TENANT_READ" '{permissionID:$p}')"
assert_status 201

case_ "RL4- granting an id no catalog row carries" "422 PermissionNotInCatalogNotification — you cannot invent a permission inside a role"
api POST "/roles/$RL_ID4/permissions" '{"permissionID":"00000000-0000-4000-8000-00000000c0de"}'
assert_rest 422 PermissionNotInCatalogNotification

RL_R4=$(pair_resource rl4)
RL_P4=$(new_permission "$RL_R4" read "A catalog entry retired before it is granted, so the in-catalog rule can be told from a mere existence check.") || exit 1
api PATCH "/permissions/$RL_P4/archive"

case_ "RL4- granting the id of a permission ARCHIVED a moment ago" "422 PermissionNotInCatalogNotification — the half that matters: a retired permission comes back as a NEW row with a NEW id, so re-granting the old id is refused rather than silently honoured, and THIS is why the grant stores the id and not the string"
api POST "/roles/$RL_ID4/permissions" "$(jq -nc --arg p "$RL_P4" '{permissionID:$p}')"
assert_rest 422 PermissionNotInCatalogNotification

# ── RL5 — the owner tenant must be usable, and a TRIAL one is ─────────────────────────────
#
# The trial case is the plausible-mistake control. A rule written as `Status != active` would
# refuse every trial signup, and only this case sees it.

RL_TRIAL=$(new_tenant trial "$(ws rltrial)") || exit 1
case_ "RL5+ a role inside a TRIAL tenant" "201 — 'unavailable' is not 'not active': a trial is a live customer being onboarded, and roles are the first thing they need"
api POST /roles "$(role_body "$(role_key rl5t)" "QA Trial Role" "$RL_D" "$RL_TRIAL")"
assert_status 201

RL_SUSP=$(new_tenant active "$(ws rlsusp)") || exit 1
api PATCH "/tenants/$RL_SUSP" '{"status":"suspended"}'
case_ "RL5- a role inside a SUSPENDED tenant" "422 RoleTenantDoesNotExistNotification — a role is the unit that grants access, so minting one inside a suspended customer hands out exactly what the commercial state says to withhold"
api POST /roles "$(role_body "$(role_key rl5s)" "QA Suspended Role" "$RL_D" "$RL_SUSP")"
assert_rest 422 RoleTenantDoesNotExistNotification

RL_ARCH=$(new_tenant active "$(ws rlarch)") || exit 1
api PATCH "/tenants/$RL_ARCH/archive"
case_ "RL5- a role inside an ARCHIVED tenant" "422 RoleTenantDoesNotExistNotification"
api POST /roles "$(role_body "$(role_key rl5a)" "QA Archived Owner Role" "$RL_D" "$RL_ARCH")"
assert_rest 422 RoleTenantDoesNotExistNotification

case_ "RL5- a role naming a tenant id no row carries" "422 RoleTenantDoesNotExistNotification — and NOT a 500: the guard barrier already proved the id parses, so the probe answers rather than panics"
api POST /roles "$(role_body "$(role_key rl5u)" "QA Ghost Owner Role" "$RL_D" "00000000-0000-4000-8000-00000000face")"
assert_rest 422 RoleTenantDoesNotExistNotification

# ── RL6 / RL7 — what is frozen, and HOW each one is frozen ────────────────────────────────

RL_K6=$(role_key rl6)
RL_ID6=$(new_role "$RL_K6" "$RL_TEN") || exit 1

case_ "RL6+ everything else about a role is editable" "200 — immutability is about the handle, not about the record"
api PATCH "/roles/$RL_ID6" '{"name":"QA Immutability Role, relabelled","description":"The wording an operator improved after reading it, which is what this verb is for."}'
assert_status 200

case_ "RL6+ and the key did not move" "$RL_K6"
assert_json '.data.key' "$RL_K6"

case_ "RL6- a request that NAMES the key is refused" "422 RoleKeyIsImmutableNotification — the key IS what every API caller and every audit line references, so editing it would rewrite the meaning of every reference retroactively and invisibly"
api PATCH "/roles/$RL_ID6" '{"key":"qa-seized-handle"}'
assert_rest 422 RoleKeyIsImmutableNotification

case_ "RL6- the refusal names the key" "field 'key'"
assert_rest_field 422 RoleKeyIsImmutableNotification key

case_ "RL6- and nothing was written under the seized handle" "0 rows — the assignment never happened, so there is nothing to find"
GOT=$(sql "SELECT count(*) FROM roles WHERE role_key = 'qa-seized-handle';" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else fail_ "roles rows = '$GOT'"; fi

case_ "RL7- a request that names the TENANT changes nothing" "200 and the ORIGINAL owner — the field is structurally absent from PatchRoleRequest, so the lenient handler ignores it and no request can reach it"
api PATCH "/roles/$RL_ID6" "$(jq -nc --arg t "$QA_MASTER_TENANT_ID" '{tenantID:$t}')"
assert_json_at 200 '.data.tenantID' "$RL_TEN"

case_ "RL7- the immutability notification is UNREACHABLE from the wire" "recorded, and deliberately not asserted"
skip_ "RoleTenantIsImmutableNotification is declared and enforced in BuildRules, and no mounted request can provoke it: PatchRoleRequest carries key, name and description alone, on REST and on GraphQL both, so the declarative rule is a belt-and-braces layer behind a structural cut. The suite asserts the EFFECT above and does not assert a notification no request can reach"

# ── RL8 — the 200-permission cap, in the cheap form approved at the gate ──────────────────

case_ "RL8+ a role carrying the whole LIVE catalog, minus every wildcard row" "201 — the cap admits an ordinary bundle without argument"
api GET "/permissions?first=100"
RL_ALL=$(printf '%s' "$HTTP_BODY" | jq -c '[.data[] | select((.permission | contains("*")) | not) | {permissionID: .id}]')
RL_N=$(printf '%s' "$RL_ALL" | jq 'length')
api POST /roles "$(jq -nc --arg k "$(role_key rl8)" --arg d "$RL_D" --arg t "$RL_TEN" --argjson p "$RL_ALL" \
  '{key:$k, name:"QA Whole Catalog Role", description:$d, tenantID:$t, permissions:$p}')"
assert_status 201

case_ "RL8+ and it really carried them all" "$RL_N entries — the fixture excludes every row whose rendered token contains a '*', not only the seeded *:*, because no-wildcard-grant refuses a wildcard in EITHER half"
assert_json '.data.permissions | length' "$RL_N"

RL_201=$(for i in $(seq 1 201); do printf '00000000-0000-4000-8000-%012d\n' "$i"; done | jq -R . | jq -sc 'map({permissionID: .})')
case_ "RL8- an insert carrying 201 distinct ids" "422 with TooManyPermissionsInRoleNotification PRESENT in the envelope — 201 PermissionNotInCatalogNotification keys ride with it, because the invented ids are in no catalog, so the assertion reads the WHOLE envelope and never only the first message"
api POST /roles "$(jq -nc --arg k "$(role_key rl8x)" --arg d "$RL_D" --arg t "$RL_TEN" --argjson p "$RL_201" \
  '{key:$k, name:"QA Over Cap Role", description:$d, tenantID:$t, permissions:$p}')"
assert_rest 422 TooManyPermissionsInRoleNotification

case_ "RL8- the cap notification hands back the limit itself" "200 — the cap is sized to this platform rather than to GCP's 3000 or Azure's 2000"
assert_json '[.errors[].messages[] | select(.notificationKey=="TooManyPermissionsInRoleNotification") | .value] | join(",")' "201"

# ── RL9 / RL10 — the three grant rules judge what a write ADDS, never what is stored ──────

RL_R9=$(pair_resource rl9)
RL_P9=$(new_permission "$RL_R9" read "A catalog entry granted first and retired afterwards, so a later write can prove it judges only what it adds.") || exit 1
RL_ID9=$(new_role "$(role_key rl9)" "$RL_TEN" "$RL_P9" "$RL_P_TENANT_READ") || exit 1
api PATCH "/permissions/$RL_P9/archive"

case_ "RL9+ a role holding a RETIRED permission is still writable" "200 on a PATCH whose only change is a label — re-judging stored entries would answer 422 on a request that grants nothing, making unrelated writes hostages of the past"
api PATCH "/roles/$RL_ID9" '{"name":"QA Added-Entries Role, relabelled"}'
assert_status 200

case_ "RL9+ and the retired grant is still there, reporting its state" "the entry survives and permissionArchivedAt is stamped — the rules stood down, the READ did not"
api GET "/roles/$RL_ID9"
assert_json_at 200 '[.data.permissions[] | select(.permissionArchivedAt != null)] | length' "1"

api GET "/roles/$RL_ID9"
RL_CHILD9=$(printf '%s' "$HTTP_BODY" | jq -r '.data.permissions[] | select(.permissionArchivedAt != null) | .id' | head -1)
case_ "RL10+ a REVOKE asks nothing, because it adds nothing" "204 — revocation is the tool for a grant that stopped being acceptable, and it stays reachable precisely when it is needed"
api PATCH "/roles/$RL_ID9/permissions/$RL_CHILD9/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# ROW RL11 — archiving a ROLE removes the power, on a fixture of the suite's OWN.
#
#   "Arquivar um papel retira o poder: a linha user_roles sobrevive apontando para o papel
#    arquivado, e isso é HISTÓRIA — mas um token reemitido não carrega mais as permissões dele."
#   source: asked 2026-09-07
#
# The Role twin of P9/P10, one level up the graph. P9/P10 archived the CATALOG row; this
# archives the ROLE that bundles it, which is a different seam entirely: the grant survives in
# role_permissions AND the membership survives in user_roles, and neither of them authorizes
# anything any more. The chain uses nothing seeded.
# ═════════════════════════════════════════════════════════════════════════════════════════

RL11_EMAIL="qa-rolerevocation-${QA_RUN_ID}@authcore.local"
RL11_PASS='Qa!RoleRevoke2026'
RL11_PASS2='Qa!RoleRevoke2026b'

# The role carries tenant:read ALONE, and that is the point: it gates a real route, so the
# assertion below is about a REACH that was lost and not merely a claim that shrank.
RL11_ROLE=$(new_role "$(role_key rl11)" "$RL_TEN" "$RL_P_TENANT_READ") || exit 1

api POST /users "$(jq -nc --arg e "$RL11_EMAIL" --arg p "$RL11_PASS" --arg r "$RL11_ROLE" --arg t "$RL_TEN" \
  '{givenName:"Qa", familyName:"RoleRevocation", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p, roles:[{roleID:$r}]}')"
RL11_USER=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

case_ "RL11a a user can hold the fixture role" "201"
if [ -n "$RL11_USER" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, no user id"; fi

RL11_BOOT=$(qa_login "$RL11_EMAIL" "$RL11_PASS")
api PATCH "/users/$RL11_USER/password" \
  "$(jq -nc --arg cp "$RL11_PASS" --arg np "$RL11_PASS2" '{currentPassword:$cp, password:$np, passwordConfirmation:$np}')" "$RL11_BOOT"
RL11_TOKEN=$(qa_login "$RL11_EMAIL" "$RL11_PASS2")

case_ "RL11b the token carries the role's permission BEFORE the archive" "tenant:read is among its permissions claim"
if printf '%s' "$(jwt_claim "$RL11_TOKEN" permissions)" | grep -q "tenant:read"; then pass_; else fail_ "claim: $(jwt_claim "$RL11_TOKEN" permissions)"; fi

case_ "RL11c and it genuinely REACHES the route that permission gates" "200 — the reach is real, not merely claimed"
api GET "/tenants?first=1" "" "$RL11_TOKEN"
assert_status 200

case_ "RL11d archiving a role a user still holds is ACCEPTED" "204 — no rule refuses it; the membership is history, not an error"
api PATCH "/roles/$RL11_ROLE/archive"
assert_empty_body 204

case_ "RL11e the user_roles row SURVIVES, pointing at the archived role" "1 row — asserted by SQL, since no endpoint exposes the membership from this side"
GOT=$(sql "SELECT count(*) FROM user_roles WHERE user_id = '$RL11_USER' AND role_id = '$RL11_ROLE';" | tr -d '[:space:]')
if [ "$GOT" = "1" ]; then pass_; else fail_ "user_roles rows = '$GOT'"; fi

case_ "RL11f and the role_permissions row survives too" "1 row — archiving the bundle does not unpick what it bundled"
GOT=$(sql "SELECT count(*) FROM role_permissions WHERE role_id = '$RL11_ROLE' AND permission_id = '$RL_P_TENANT_READ';" | tr -d '[:space:]')
if [ "$GOT" = "1" ]; then pass_; else fail_ "role_permissions rows = '$GOT'"; fi

case_ "RL11g THE POINT: a FRESHLY reissued token no longer carries the permission" "tenant:read is gone from the permissions claim — the membership is history and history authorizes nothing"
RL11_TOKEN2=$(qa_login "$RL11_EMAIL" "$RL11_PASS2")
if [ -z "$RL11_TOKEN2" ]; then
  fail_ "the principal could not sign in again"
elif printf '%s' "$(jwt_claim "$RL11_TOKEN2" permissions)" | grep -q "tenant:read"; then
  fail_ "the archived role's permission is still in the claim: $(jwt_claim "$RL11_TOKEN2" permissions)"
else
  pass_
fi

case_ "RL11h and the reach is GONE" "403 — the same call that answered 200 before the archive"
api GET "/tenants?first=1" "" "$RL11_TOKEN2"
assert_rest 403 MissingPermissionNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# GR1-GR13 — Group. specs/qa/group-contract/plan.md §1b.
#
# The rows the framework never had an opinion about. What makes this entity's set different
# from Role's, one level down:
#   · G6 asks a THIRD question — same tenant (GR3), which is a cross-tenant leak wearing the
#     clothes of ordinary group editing;
#   · G10a is TRANSITIVE (GR5) — a SET of permissions, not one key;
#   · and the power itself is transitive (GR12), so archiving a group de-authorizes a whole
#     team at once. That is the family this round exists for.
# ═════════════════════════════════════════════════════════════════════════════════════════

GR_TEN=$(new_tenant active "$(ws grten)")   || exit 1
GR_TEN_B=$(new_tenant active "$(ws grtenb)") || exit 1
GR_P_TENANT_READ=$(permission_id_of tenant read)
GR_P_GROUP_READ=$(permission_id_of group read)
GR_P_ROLE_READ=$(permission_id_of role read)
GR_P_PERM_ARCHIVE=$(permission_id_of permission archive)
GR_D="A group description long enough to satisfy the shared anti-junk floor this service applies."

GR_R1=$(new_role "$(role_key gr1)" "$GR_TEN" "$GR_P_TENANT_READ") || exit 1
GR_R2=$(new_role "$(role_key gr2)" "$GR_TEN" "$GR_P_GROUP_READ")  || exit 1
GR_RB=$(new_role "$(role_key grb)" "$GR_TEN_B" "$GR_P_TENANT_READ") || exit 1

# ── GR1 / GR2 / GR3 — the three questions G6 asks, and the ONE message it answers with ────

GR_G1=$(new_group "$(group_key gr1)" "$GR_TEN") || exit 1

case_ "GR1+ a live role of the group's own tenant can be attached" "201 — the positive half, without which every refusal below could pass for a rule that refuses everything"
api POST "/groups/$GR_G1/roles" "$(jq -nc --arg r "$GR_R1" '{roleID:$r}')"
assert_status 201

case_ "GR1- a role id no row carries" "422 RoleNotAvailableInTenantNotification on field roles"
api POST "/groups/$GR_G1/roles" '{"roleID":"00000000-0000-7000-8000-0000000000aa"}'
assert_rest_field 422 RoleNotAvailableInTenantNotification roles

GR_RDEAD=$(new_role "$(role_key grdead)" "$GR_TEN" "$GR_P_TENANT_READ") || exit 1
api PATCH "/roles/$GR_RDEAD/archive"
case_ "GR2- a role that exists but was ARCHIVED" "422, the SAME key — this is the whole reason the entry stores the id and not the handle: a retired-and-recreated role must need an explicit re-attach"
api POST "/groups/$GR_G1/roles" "$(jq -nc --arg r "$GR_RDEAD" '{roleID:$r}')"
assert_rest_field 422 RoleNotAvailableInTenantNotification roles

case_ "GR3- a LIVE, ACTIVE role that belongs to ANOTHER TENANT" "422, the SAME key again — the first thing in this entity Role did not need. Permission is a global catalog; Role is tenant-scoped, so a group in tenant A attaching tenant B's role would confer another customer's permissions on A's members"
api POST "/groups/$GR_G1/roles" "$(jq -nc --arg r "$GR_RB" '{roleID:$r}')"
assert_rest_field 422 RoleNotAvailableInTenantNotification roles

case_ "GR3-b the three questions collapse into ONE message, deliberately" "one distinct notificationKey across GR1-, GR2- and GR3- — a separate 'that role belongs to another tenant' reply would confirm to a caller in tenant A that a specific UUID is a live role in some OTHER tenant: an existence oracle over a competitor's org chart"
assert_json '[.errors[].messages[].notificationKey] | unique | join(",")' "RoleNotAvailableInTenantNotification"

# ── GR4 — the wildcard interlock, and NOBODY is exempt ────────────────────────────────────
#
# The group has to live in the MASTER tenant for this case to reach the rule it is about:
# the availability rule runs FIRST, so a group in the lane's tenant attaching the master role
# would answer 422 (another tenant) and never reach the wildcard refusal at all. Nothing is
# archived here — the master tenant and the master role are only READ.
GR_GM=$(new_group "$(group_key grm)" "$QA_MASTER_TENANT_ID") || exit 1

case_ "GR4- the *:* SUPER-ADMIN attaches the seeded master role" "403 CannotGrantWildcardRoleNotification on field roles. The strongest possible negative: if anyone were exempt it would be them"
api POST "/groups/$GR_GM/roles" "$(jq -nc --arg r "$QA_MASTER_ROLE_ID" '{roleID:$r}')"
assert_rest_field 403 CannotGrantWildcardRoleNotification roles

case_ "GR4b the wildcard rule runs BEFORE the escalation rule" "403, NEVER 500 — Identity.HasPermission PANICS on any argument containing '*', so the wildcard refusal is what removes the input that would crash the request, on exactly the case the pair exists to stop. A plain status comparison: a 500 body is not JSON"
if [ "$HTTP_STATUS" = "403" ]; then pass_; else fail_ "HTTP $HTTP_STATUS"; fi

case_ "GR4c no wildcard-bearing group exists by any other path either" "0 groups in the master tenant carry the *:* role — the platform operator holds it through user_roles, never through a group (plan §0c finding 4)"
GOT=$(sql "SELECT count(*) FROM group_roles WHERE role_id = '$QA_MASTER_ROLE_ID' AND archived_at IS NULL;" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else fail_ "group_roles rows conferring the wildcard role = '$GOT'"; fi

# ── GR5 — TRANSITIVE no-escalation: a SET, not one key ────────────────────────────────────

if [ -z "${QA_TOKEN_GROUP:-}" ]; then
  case_ "GR5 the transitive escalation rule" "principal G, which holds group:grant and NOT permission:archive"
  skip_ "principal G was not built by qa/run.sh — the whole GR5/GR6 block needs an authenticated, non-superadmin caller bound to a tenant of the suite's own, and inventing one is not available to this suite"
else
  # Both target roles are created by the ADMIN, not by G: Role's own escalation rule would
  # refuse G the creation of a role granting permission:archive (role-contract RL2), and the
  # question this row asks is whether G may ATTACH it.
  GR_R_OK=$(new_role "$(role_key grok)" "$QA_TENANT_SCOPED" "$GR_P_TENANT_READ" "$GR_P_GROUP_READ") || exit 1
  GR_R_ESC=$(new_role "$(role_key gresc)" "$QA_TENANT_SCOPED" "$GR_P_TENANT_READ" "$GR_P_GROUP_READ" "$GR_P_PERM_ARCHIVE") || exit 1

  case_ "GR5+ principal G creates a group in its OWN tenant, omitting tenantID entirely" "201 — absent means 'mine', which is what assignedFrom: identity-claim with bypassMaySet buys an ordinary caller"
  api POST /groups "$(jq -nc --arg k "$(group_key gresc)" --arg d "$GR_D" '{key:$k, name:"QA Escalation Group", description:$d, roles:[]}')" "$QA_TOKEN_GROUP"
  assert_status 201
  GR_GE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

  case_ "GR5+b G attaches a role granting ONLY permissions it holds" "201 — tenant:read and group:read are both in G's own bundle"
  api POST "/groups/$GR_GE/roles" "$(jq -nc --arg r "$GR_R_OK" '{roleID:$r}')" "$QA_TOKEN_GROUP"
  assert_status 201

  case_ "GR5- G attaches a role granting ONE permission it lacks" "403 CannotGrantRoleWithUnheldPermissionsNotification on field roles. THE SET IS THE POINT: this role grants three permissions, two of which G holds — a rule asking 'any' instead of 'every' would let it straight through"
  api POST "/groups/$GR_GE/roles" "$(jq -nc --arg r "$GR_R_ESC" '{roleID:$r}')" "$QA_TOKEN_GROUP"
  assert_rest_field 403 CannotGrantRoleWithUnheldPermissionsNotification roles

  case_ "GR5-b the value names the role that was refused" "the role id, so a caller can tell WHICH attachment in a batch was the problem"
  assert_json '[.errors[].messages[] | select(.notificationKey=="CannotGrantRoleWithUnheldPermissionsNotification") | .value] | join(",")' "$GR_R_ESC"

  case_ "GR5b THE COMPLEMENT: the admin attaches that same role" "201 — the *:* exemption is FREE rather than special-cased: HasPermission answers true for any CONCRETE permission when the claim set holds *:*. Without this the rule could pass for a service that refuses EVERY attach"
  api POST "/groups/$GR_GE/roles" "$(jq -nc --arg r "$GR_R_ESC" '{roleID:$r}')"
  assert_status 201

  case_ "GR5c THE INTERLOCK ORDER: an UNKNOWN role id answers 422, never 403" "RoleGrantsWildcard deliberately answers TRUE for an id it cannot resolve, so a service that ran the wildcard rule first would report an unknown role as an ESCALATION ATTEMPT — a 403 blaming the caller where the honest answer is 'the role is not there'. This is the only case that sees the order"
  api POST "/groups/$GR_GE/roles" '{"roleID":"00000000-0000-7000-8000-0000000000bb"}' "$QA_TOKEN_GROUP"
  assert_rest 422 RoleNotAvailableInTenantNotification

  # ── GR6 — tenant isolation, both sides ─────────────────────────────────────────────────
  case_ "GR6- G names the MASTER tenant on an insert" "403 TenantMismatchNotification on field tenantID — the claim decides what a caller MAY write, and the guard is what answers"
  api POST /groups "$(jq -nc --arg k "$(group_key grmis)" --arg d "$GR_D" --arg t "$QA_MASTER_TENANT_ID" '{key:$k, name:"QA Foreign Group", description:$d, tenantID:$t, roles:[]}')" "$QA_TOKEN_GROUP"
  assert_rest_field 403 TenantMismatchNotification tenantID

  case_ "GR6-b G ARCHIVES a group belonging to another tenant" "403 TenantMismatchNotification — refuseForeignTenant runs under IfArchive too, and the WRITE side is not filtered by ToCriteria, so the row loads and the RULE is what refuses. This is the seam that would be invisible if only reads were tested"
  api PATCH "/groups/$GR_G1/archive" "" "$QA_TOKEN_GROUP"
  assert_rest 403 TenantMismatchNotification

  case_ "GR6b- G READS another tenant's group by id" "404, NOT 403 — it does not exist for this caller, which leaks nothing about who else exists. A 403 would confirm the row is there"
  api GET "/groups/$GR_G1" "" "$QA_TOKEN_GROUP"
  assert_rest 404 RecordNotFoundNotification

  case_ "GR6b-c and G's listing never contains it" "asserted BY ID, not by count — an isolation leak answers 200, which is exactly why it needs its own case"
  api GET "/groups?first=100" "" "$QA_TOKEN_GROUP"
  assert_json_at 200 "[.data[].id] | index(\"$GR_G1\") // \"absent\"" "absent"

  case_ "GR6c THE COMPLEMENT: the admin's listing DOES reach across tenants" "the same group is present — without this, a service that filtered everyone would pass GR6b for the wrong reason and support would be impossible"
  api GET "/groups?tenantID.eq=$GR_TEN&first=100"
  assert_json_at 200 "[.data[].id] | index(\"$GR_G1\") != null" "true"
fi

# ── GR7 — the owner tenant must be AVAILABLE, and 'trial' is not 'not active' ─────────────

GR_TEN_TRIAL=$(new_tenant trial "$(ws grtrial)") || exit 1
GR_TEN_SUSP=$(new_tenant suspended "$(ws grsusp)") || exit 1
GR_TEN_ARCH=$(new_tenant active "$(ws grarch)") || exit 1
api PATCH "/tenants/$GR_TEN_ARCH/archive"

case_ "GR7+ a group inside a TRIAL tenant" "201 — THE PLAUSIBLE-MISTAKE CONTROL: a rule written as 'Status != active' would refuse every trial signup, and only this case sees it. A trial tenant is a live customer, and 'unavailable' is not 'not active'"
api POST /groups "$(jq -nc --arg k "$(group_key grtr)" --arg d "$GR_D" --arg t "$GR_TEN_TRIAL" '{key:$k, name:"QA Trial Group", description:$d, tenantID:$t, roles:[]}')"
assert_status 201

case_ "GR7- a group inside a SUSPENDED tenant" "422 GroupTenantDoesNotExistNotification on field tenantID"
api POST /groups "$(jq -nc --arg k "$(group_key grsu)" --arg d "$GR_D" --arg t "$GR_TEN_SUSP" '{key:$k, name:"QA Suspended Group", description:$d, tenantID:$t, roles:[]}')"
assert_rest_field 422 GroupTenantDoesNotExistNotification tenantID

case_ "GR7-b a group inside an ARCHIVED tenant" "422, same key"
api POST /groups "$(jq -nc --arg k "$(group_key grar)" --arg d "$GR_D" --arg t "$GR_TEN_ARCH" '{key:$k, name:"QA Archived Group", description:$d, tenantID:$t, roles:[]}')"
assert_rest 422 GroupTenantDoesNotExistNotification

case_ "GR7-c a tenant id no row carries" "422, same key — one message for all four states, which is the same non-oracle reasoning GR3 records"
api POST /groups "$(jq -nc --arg k "$(group_key grno)" --arg d "$GR_D" '{key:$k, name:"QA Ghost Tenant Group", description:$d, tenantID:"00000000-0000-7000-8000-0000000000cc", roles:[]}')"
assert_rest 422 GroupTenantDoesNotExistNotification

GR_G_LATER=$(new_group "$(group_key grlat)" "$GR_TEN_TRIAL") || exit 1
api PATCH "/tenants/$GR_TEN_TRIAL/archive"
case_ "GR7d the rule is a GATE, not a TRAP" "200 — tenant-must-be-available is scoped IfInsert alone, so a group whose tenant was suspended AFTERWARDS is still relabellable. A rule scoped insertOrUpdate would freeze every group of a suspended customer, including the renames an operator makes while sorting the suspension out"
api PATCH "/groups/$GR_G_LATER" "$(jq -nc '{name:"QA Renamed After Suspension"}')"
assert_status 200

# ── GR8 / GR8b — immutability, enforced STRUCTURALLY ──────────────────────────────────────

GR_G8=$(new_group "$(group_key gr8)" "$GR_TEN") || exit 1
api GET "/groups/$GR_G8"; GR_K8=$(printf '%s' "$HTTP_BODY" | jq -r '.data.key')

case_ "GR8+ name and description are patchable" "200, and the key is byte-identical afterwards"
api PATCH "/groups/$GR_G8" "$(jq -nc --arg d "$GR_D" '{name:"QA Relabelled Group", description:$d}')"
assert_json_at 200 '.data.key' "$GR_K8"

case_ "GR8- a patch NAMING the key" "200 and the key UNCHANGED — the door does not exist: update.patchExcludes: [Key], so PatchGroupRequest carries name and description alone and there is no value to accept"
api PATCH "/groups/$GR_G8" "$(jq -nc '{key:"some-other-key", name:"QA Relabelled Twice"}')"
assert_json_at 200 '.data.key' "$GR_K8"

case_ "GR8-b GroupKeyIsImmutableNotification is UNREACHABLE from the wire" "no case asserts it — the declarative rule is a belt-and-braces layer behind a structural cut, and asserting a notification no request can provoke would be asserting a lie"
skip_ "update.patchExcludes: [Key] removes the field from the PATCH body on REST and from PatchGroupInput on GraphQL (L1.4c), so no mounted request can provoke GroupKeyIsImmutableNotification. GR8- asserts the EFFECT instead, which is the only honest assertion available"

case_ "GR8b- a patch NAMING another tenant" "200 and tenantID UNCHANGED — a group never moves between tenants, and here too the door does not exist"
api PATCH "/groups/$GR_G8" "$(jq -nc --arg t "$GR_TEN_B" '{tenantID:$t, name:"QA Relabelled Thrice"}')"
assert_json_at 200 '.data.tenantID' "$GR_TEN"

case_ "GR8b-b GroupTenantIsImmutableNotification is UNREACHABLE from the wire" "stated, not asserted — the same structural cut, on the other frozen field"
skip_ "PatchGroupRequest carries name and description alone, so nothing can move tenantID through a PATCH. The rule stands as the backstop that answers if the field ever rejoins the body; GR8b- asserts the effect"

# ── GR9 / GR10 — the rules judge the entries a write ADDS, never the ones already stored ──

GR_R9=$(new_role "$(role_key gr9)" "$GR_TEN" "$GR_P_TENANT_READ") || exit 1
GR_G9=$(new_group "$(group_key gr9)" "$GR_TEN" "$GR_R9" "$GR_R2") || exit 1
api PATCH "/roles/$GR_R9/archive"

case_ "GR9+ a group whose STORED entry points at an archived role is still relabellable" "200 — a rule re-judging stored entries would answer 422 on a request whose only change is a label, and would make a group impossible to rename because of somebody else's archive"
api PATCH "/groups/$GR_G9" "$(jq -nc '{name:"QA Renamed Despite Archived Role"}')"
assert_status 200

case_ "GR9b and the archived counterpart is still SERVED on the entry" "roleArchivedAt stamped, roleKey still rendered — the read-side view of the same fact (cross-ref K7.2)"
api GET "/groups/$GR_G9"
assert_json_at 200 '[.data.roles[] | select(.roleArchivedAt != null)] | length' "1"

api GET "/groups/$GR_G9"
GR_CHILD9=$(printf '%s' "$HTTP_BODY" | jq -r '.data.roles[] | select(.roleArchivedAt != null) | .id' | head -1)
case_ "GR10+ a DETACH asks nothing, because it adds nothing" "204 — detaching is the tool for an attachment that stopped being acceptable, and it stays reachable precisely when it is needed"
api PATCH "/groups/$GR_G9/roles/$GR_CHILD9/archive"
assert_empty_body 204

# ── GR11 — the cap, in the HYBRID form the maintainer approved: the EDGE, not near it ─────

GR_TEN_CAP=$(new_tenant active "$(ws grcap)") || exit 1
GR_CAP_IDS=""
GR_CAP_N=0
while [ "$GR_CAP_N" -lt 51 ]; do
  GR_CAP_N=$((GR_CAP_N + 1))
  rid=$(new_role "$(role_key grc)" "$GR_TEN_CAP" "$GR_P_TENANT_READ") || break
  GR_CAP_IDS="$GR_CAP_IDS $rid"
done

case_ "GR11 setup: 51 real, live, same-tenant roles" "51 — the fixture the edge needs; invented UUIDs could never show a CLEAN refusal"
GR_CAP_HAVE=$(printf '%s' "$GR_CAP_IDS" | wc -w | tr -d ' ')
if [ "$GR_CAP_HAVE" = "51" ]; then pass_; else fail_ "$GR_CAP_HAVE roles built"; fi

if [ "$GR_CAP_HAVE" = "51" ]; then
  GR_CAP_50=$(printf '%s' "$GR_CAP_IDS" | tr ' ' '\n' | grep -v '^$' | head -50 | tr '\n' ' ')
  GR_CAP_51=$(printf '%s' "$GR_CAP_IDS" | tr ' ' '\n' | grep -v '^$' | tail -1)

  case_ "GR11+ a group carrying EXACTLY 50 roles" "201 — the edge itself, not an approximation of it"
  api POST /groups "$(group_body "$(group_key grcap)" "QA Cap Group" "$GR_D" "$GR_TEN_CAP" $GR_CAP_50)"
  assert_json_at 201 '.data.roles | length' "50"
  GR_GCAP=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

  case_ "GR11- the 51st, attached through the COLLECTION route" "422 TooManyRolesInGroupNotification on field roles — the rule counts GetCurrentItemsOf, so the collection verb trips it exactly as an oversized insert body does"
  api POST "/groups/$GR_GCAP/roles" "$(jq -nc --arg r "$GR_CAP_51" '{roleID:$r}')"
  assert_rest_field 422 TooManyRolesInGroupNotification roles

  case_ "GR11-b the refusal is CLEAN" "TooManyRolesInGroupNotification and NOTHING ELSE in the envelope — every role in the fixture is real, live, same-tenant and attachable, so the cap is the only thing left to say. A fixture of invented ids would bury this key under 51 availability refusals"
  assert_json '[.errors[].messages[].notificationKey] | unique | join(",")' "TooManyRolesInGroupNotification"

  case_ "GR11-c and it names the count it refused" "51"
  assert_json '[.errors[].messages[] | select(.notificationKey=="TooManyRolesInGroupNotification") | .value] | join(",")' "51"

  case_ "GR11-d the same cap on an oversized INSERT body" "422, same key — one rule, both write paths"
  api POST /groups "$(group_body "$(group_key grcap2)" "QA Cap Group Two" "$GR_D" "$GR_TEN_CAP" $GR_CAP_50 "$GR_CAP_51")"
  assert_rest 422 TooManyRolesInGroupNotification
else
  skip_ "the 51-role fixture could not be built, so the cap edge cannot be addressed — the failure is reported by the setup case above rather than counted twice here"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# GR12 — THE TRANSITIVE REVOCATION, and the three switches only this aggregate has.
#
#   "Arquivar o grupo tira o poder — e o poder do Group é transitivo. A linha user_groups
#    sobrevive apontando para o grupo arquivado, e isso é HISTÓRIA; mas um token reemitido
#    não carrega mais nenhuma permissão que vinha por ali."
#   source: asked 2026-09-07 — "os três interruptores"
#
# internal/infra/authentication_reader.go walks user_groups -> groups -> group_roles -> roles
# -> role_permissions -> permissions and carries three kill switches on that walk:
#   :354  GroupArchivedAt        the group is retired      -> GR12a
#   :360  GroupGrantArchivedAt   the entry was detached    -> GR12b
#   :361  RoleArchivedAt         the role was retired      -> GR12c
#
# Each switch is IRREVERSIBLE, so each gets a fixture of its own. Every chain is built from
# nothing seeded, and the member holds NO DIRECT ROLE AT ALL — which is what makes the
# assertion transitive rather than incidental: everything its token carries came through the
# group.
# ═════════════════════════════════════════════════════════════════════════════════════════

# gr12_chain LABEL → sets GR12_ROLE / GR12_GROUP / GR12_USER / GR12_TOKEN / GR12_EMAIL /
#                    GR12_PASS2 / GR12_GKEY, or leaves GR12_TOKEN empty on a fixture failure.
gr12_chain() {
  local label="$1"
  GR12_EMAIL="qa-grouprevoke-$label-${QA_RUN_ID}@authcore.local"
  local p1='Qa!GroupRevoke2026' p2='Qa!GroupRevoke2026b'
  GR12_PASS2="$p2"
  GR12_TOKEN=""

  # tenant:read ALONE, so the assertion below is about a REACH that was lost rather than a
  # claim that merely shrank.
  GR12_ROLE=$(new_role "$(role_key g12$label)" "$GR_TEN" "$GR_P_TENANT_READ") || return 1
  GR12_GKEY=$(group_key g12$label)
  GR12_GROUP=$(new_group "$GR12_GKEY" "$GR_TEN" "$GR12_ROLE") || return 1

  api POST /users "$(jq -nc --arg e "$GR12_EMAIL" --arg p "$p1" --arg g "$GR12_GROUP" --arg t "$GR_TEN" \
    '{givenName:"Qa", familyName:"GroupRevocation", email:$e, status:"active", tenantID:$t,
      password:$p, passwordConfirmation:$p, groups:[{groupID:$g}], roles:[]}')"
  GR12_USER=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
  [ -n "$GR12_USER" ] || return 1

  local boot; boot=$(qa_login "$GR12_EMAIL" "$p1")
  [ -n "$boot" ] || return 1
  api PATCH "/users/$GR12_USER/password" \
    "$(jq -nc --arg cp "$p1" --arg np "$p2" '{currentPassword:$cp, password:$np, passwordConfirmation:$np}')" "$boot"
  GR12_TOKEN=$(qa_login "$GR12_EMAIL" "$p2")
  [ -n "$GR12_TOKEN" ]
}

gr12_prove_reach() { # gr12_prove_reach PREFIX
  case_ "$1 the member holds NO direct role" "0 rows in user_roles — everything the token carries came through the group, which is what makes this transitive rather than incidental"
  local got; got=$(sql "SELECT count(*) FROM user_roles WHERE user_id = '$GR12_USER';" | tr -d '[:space:]')
  if [ "$got" = "0" ]; then pass_; else fail_ "user_roles rows = '$got'"; fi

  case_ "$1b the token names the GROUP in its groups claim" "the group's key — the membership itself, which falls out of the same read that resolves the grants"
  if printf '%s' "$(jwt_claim "$GR12_TOKEN" groups)" | grep -q "$GR12_GKEY"; then pass_; else fail_ "groups claim: $(jwt_claim "$GR12_TOKEN" groups)"; fi

  case_ "$1c and it carries the INHERITED permission" "tenant:read, reached through group -> role -> permission and through nothing else"
  if printf '%s' "$(jwt_claim "$GR12_TOKEN" permissions)" | grep -q "tenant:read"; then pass_; else fail_ "permissions claim: $(jwt_claim "$GR12_TOKEN" permissions)"; fi

  case_ "$1d and the reach is REAL" "200 on the route that permission gates — a claim nobody can spend is not a permission"
  api GET "/tenants?first=1" "" "$GR12_TOKEN"
  assert_status 200
}

# ── GR12a — the GROUP is archived ─────────────────────────────────────────────────────────
if gr12_chain a; then
  gr12_prove_reach "GR12a1"

  case_ "GR12a2 archiving a group whose members still point at it is ACCEPTED" "204 — no rule refuses it. The membership is history, not an error, and a one-way archive is what §5 of the model chose over an Unarchive that would re-authorize a whole team at once"
  api PATCH "/groups/$GR12_GROUP/archive"
  assert_empty_body 204

  case_ "GR12a3 the user_groups row SURVIVES, pointing at the archived group" "1 row — asserted by SQL, since no endpoint exposes the membership from this side"
  GOT=$(sql "SELECT count(*) FROM user_groups WHERE user_id = '$GR12_USER' AND group_id = '$GR12_GROUP';" | tr -d '[:space:]')
  if [ "$GOT" = "1" ]; then pass_; else fail_ "user_groups rows = '$GOT'"; fi

  case_ "GR12a4 and the group_roles row SURVIVES, stamped by the cascade" "1 row, archived_at filled — the pin archives an aggregate WITH its children, so the entry is not unpicked and not deleted: it is stamped, which is exactly the history an access review reads (cross-ref K6.11b)"
  GOT=$(sql "SELECT count(*) FROM group_roles WHERE group_id = '$GR12_GROUP' AND role_id = '$GR12_ROLE' AND archived_at IS NOT NULL;" | tr -d '[:space:]')
  if [ "$GOT" = "1" ]; then pass_; else fail_ "stamped group_roles rows = '$GOT'"; fi

  case_ "GR12a4b nothing was DELETED" "1 row total for the pair — the distinction between a stamp and a delete is the whole reason the detach verb is PATCH .../archive"
  GOT=$(sql "SELECT count(*) FROM group_roles WHERE group_id = '$GR12_GROUP' AND role_id = '$GR12_ROLE';" | tr -d '[:space:]')
  if [ "$GOT" = "1" ]; then pass_; else fail_ "group_roles rows = '$GOT'"; fi

  case_ "GR12a5 THE POINT: a FRESHLY reissued token no longer carries the inherited permission" "tenant:read is gone — a retired group confers nothing (authentication_reader.go:354)"
  GR12A_T2=$(qa_login "$GR12_EMAIL" "$GR12_PASS2")
  if [ -z "$GR12A_T2" ]; then fail_ "the member could not sign in again"
  elif printf '%s' "$(jwt_claim "$GR12A_T2" permissions)" | grep -q "tenant:read"; then fail_ "still claimed: $(jwt_claim "$GR12A_T2" permissions)"
  else pass_; fi

  case_ "GR12a6 and the groups claim no longer names it" "the membership stops being reported the moment the group is retired — the switch is BEFORE the group is recorded, not after"
  if printf '%s' "$(jwt_claim "$GR12A_T2" groups)" | grep -q "$GR12_GKEY"; then fail_ "groups claim: $(jwt_claim "$GR12A_T2" groups)"; else pass_; fi

  case_ "GR12a7 and the reach is GONE" "403 — the same call that answered 200 before the archive"
  api GET "/tenants?first=1" "" "$GR12A_T2"
  assert_rest 403 MissingPermissionNotification

  # ── GR13, which only exists because GR12a happened ──────────────────────────────────────
  case_ "GR13 the retired group comes back as a NEW row" "201 with a DIFFERENT id — the way back is a fresh insert, and there is no Unarchive"
  api POST /groups "$(group_body "$GR12_GKEY" "QA Group Reborn" "$GR_D" "$GR_TEN" "$GR12_ROLE")"
  GR13_ID=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')
  if [ "$HTTP_STATUS" = "201" ] && [ -n "$GR13_ID" ] && [ "$GR13_ID" != "$GR12_GROUP" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, id '$GR13_ID' vs '$GR12_GROUP'"; fi

  case_ "GR13b and the former member is NOT re-authorized by it" "tenant:read still absent — user_groups still points at the OLD id, so the team has to be re-added. This is the cost §5 accepted when it refused Unarchive, made visible"
  GR13_T=$(qa_login "$GR12_EMAIL" "$GR12_PASS2")
  if [ -z "$GR13_T" ]; then fail_ "the member could not sign in"
  elif printf '%s' "$(jwt_claim "$GR13_T" permissions)" | grep -q "tenant:read"; then fail_ "re-authorized by the new row: $(jwt_claim "$GR13_T" permissions)"
  else pass_; fi
else
  skip_ "GR12a — the group-archive revocation chain could not be provisioned (role, group, user or sign-in), so the first of the three switches is UNPROVEN this run"
  skip_ "GR13 — the one-way-door cost depends on GR12a's chain and is UNPROVEN this run"
fi

# ── GR12b — the group_role entry is DETACHED ──────────────────────────────────────────────
if gr12_chain b; then
  gr12_prove_reach "GR12b1"

  api GET "/groups/$GR12_GROUP"
  GR12B_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.roles[0].id')
  case_ "GR12b2 detaching the entry" "204"
  api PATCH "/groups/$GR12_GROUP/roles/$GR12B_CHILD/archive"
  assert_empty_body 204

  case_ "GR12b3 the group_roles row survives, STAMPED" "1 archived row — a detach is a soft removal, which is why the verb is PATCH .../archive and not DELETE: an access review needs to read what a past attachment meant"
  GOT=$(sql "SELECT count(*) FROM group_roles WHERE id = '$GR12B_CHILD' AND archived_at IS NOT NULL;" | tr -d '[:space:]')
  if [ "$GOT" = "1" ]; then pass_; else fail_ "stamped group_roles rows = '$GOT'"; fi

  case_ "GR12b4 the inherited permission is GONE from a reissued token" "tenant:read absent — the group's grant of the role was revoked (authentication_reader.go:360)"
  GR12B_T2=$(qa_login "$GR12_EMAIL" "$GR12_PASS2")
  if [ -z "$GR12B_T2" ]; then fail_ "the member could not sign in again"
  elif printf '%s' "$(jwt_claim "$GR12B_T2" permissions)" | grep -q "tenant:read"; then fail_ "still claimed: $(jwt_claim "$GR12B_T2" permissions)"
  else pass_; fi

  case_ "GR12b5 THE DISTINCTION: the groups claim STILL names the group" "the person is still a member; the bundle simply no longer confers that role. This is the case that separates 'you left the team' from 'the team stopped granting this' — two very different things an access review must be able to tell apart"
  if printf '%s' "$(jwt_claim "$GR12B_T2" groups)" | grep -q "$GR12_GKEY"; then pass_; else fail_ "groups claim: $(jwt_claim "$GR12B_T2" groups)"; fi

  case_ "GR12b6 and the reach is gone" "403"
  api GET "/tenants?first=1" "" "$GR12B_T2"
  assert_rest 403 MissingPermissionNotification
else
  skip_ "GR12b — the detach revocation chain could not be provisioned, so the second of the three switches is UNPROVEN this run"
fi

# ── GR12c — the conferred ROLE is archived ────────────────────────────────────────────────
if gr12_chain c; then
  gr12_prove_reach "GR12c1"

  case_ "GR12c2 archiving the role the group confers" "204"
  api PATCH "/roles/$GR12_ROLE/archive"
  assert_empty_body 204

  case_ "GR12c3 the group_roles row is UNTOUCHED and still ACTIVE" "1 active row — nothing cascaded: the attachment is still there, pointing at a role that no longer confers anything"
  GOT=$(sql "SELECT count(*) FROM group_roles WHERE group_id = '$GR12_GROUP' AND role_id = '$GR12_ROLE' AND archived_at IS NULL;" | tr -d '[:space:]')
  if [ "$GOT" = "1" ]; then pass_; else fail_ "active group_roles rows = '$GOT'"; fi

  case_ "GR12c4 and the group still SERVES the entry, with the counterpart stamped" "roleArchivedAt filled — the read-side view of exactly this state (cross-ref K7.2)"
  api GET "/groups/$GR12_GROUP"
  assert_json_at 200 '[.data.roles[] | select(.roleArchivedAt != null)] | length' "1"

  case_ "GR12c5 the inherited permission is GONE from a reissued token" "tenant:read absent — a retired role confers nothing (authentication_reader.go:361)"
  GR12C_T2=$(qa_login "$GR12_EMAIL" "$GR12_PASS2")
  if [ -z "$GR12C_T2" ]; then fail_ "the member could not sign in again"
  elif printf '%s' "$(jwt_claim "$GR12C_T2" permissions)" | grep -q "tenant:read"; then fail_ "still claimed: $(jwt_claim "$GR12C_T2" permissions)"
  else pass_; fi

  case_ "GR12c6 the groups claim still names the group" "membership is untouched by what the bundle stopped conferring"
  if printf '%s' "$(jwt_claim "$GR12C_T2" groups)" | grep -q "$GR12_GKEY"; then pass_; else fail_ "groups claim: $(jwt_claim "$GR12C_T2" groups)"; fi

  case_ "GR12c7 and the reach is gone" "403"
  api GET "/tenants?first=1" "" "$GR12C_T2"
  assert_rest 403 MissingPermissionNotification
else
  skip_ "GR12c — the role-archive revocation chain could not be provisioned, so the third of the three switches is UNPROVEN this run"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════
#  USER — §1b of specs/qa/user-contract/plan.md. Family U.
#
#  Ranked by the cost the maintainer named on 2026-09-07. U23, U24, U17 and U19 lead because
#  they are the rows where a regression is silent: none of them changes a status code on a
#  happy path, and three of them decide who can hold a session at all.
# ═════════════════════════════════════════════════════════════════════════════════════════
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_U=$(new_tenant active "$(ws du)") || exit 1
P_TENANT_READ_U=$(permission_id_of tenant read)
P_USER_READ_U=$(permission_id_of user read)
P_PERM_ARCHIVE_U=$(permission_id_of permission archive)
for p in "$P_TENANT_READ_U" "$P_USER_READ_U" "$P_PERM_ARCHIVE_U"; do
  [ -n "$p" ] || { echo "domain.sh: a seeded catalog id could not be resolved for the U family" >&2; exit 1; }
done

# ═════════════════════════════════════════════════════════════════════════════════════════
# U23 [CRITICAL] — "A token minted for a must_change_password account carries EXACTLY
# user:change-password and nothing else. The grant is EMBEDDED, not filtered, and `*:*` is
# dropped."
#   source: internal/application/commands/handlers/utils/authentication.go —
#           EffectivePermissions / restrictToPasswordChange / BuildClaims · asked 2026-09-07
#
# This rule appears in NO entity spec and NO route declaration. It is the sharpest invariant
# in the service and the one a regression would expose silently: losing it hands a session
# that exists only to rotate an expired credential the account's whole bundle.
# ═════════════════════════════════════════════════════════════════════════════════════════

U23_EMAIL=$(user_email u23)
U23_ROLE=$(new_role "$(role_key u23)" "$TEN_U" "$P_TENANT_READ_U" "$P_USER_READ_U") || exit 1
api POST /users "$(jq -nc --arg e "$U23_EMAIL" --arg t "$TEN_U" --arg p "$QA_USER_PASS1" --arg r "$U23_ROLE" \
  '{givenName:"Qa", familyName:"Fixture", email:$e, status:"active", tenantID:$t,
    password:$p, passwordConfirmation:$p, roles:[{roleID:$r}]}')"
U23_ID=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id // empty')

if [ -z "$U23_ID" ]; then
  skip_ "U23 — the must-change principal could not be provisioned, so the token restriction is UNPROVEN this run"
else
  U23_T1=$(qa_login "$U23_EMAIL" "$QA_USER_PASS1")

  case_ "U23.1 the account is born must_change_password" "true — an admin-set initial password has to be replaced by its owner"
  api GET "/users/$U23_ID"
  assert_json_at 200 '.data.mustChangePassword' "true"

  case_ "U23.2- the FIRST token carries exactly ONE permission" "[\"user:change-password\"] and nothing else — the bundle's tenant:read and user:read are both dropped"
  GOT=$(jwt_claim "$U23_T1" permissions | tr -d ' \n')
  if [ "$GOT" = '["user:change-password"]' ]; then pass_; else HTTP_BODY="$GOT"; fail_ "permissions = $GOT"; fi

  case_ "U23.3- ...so a permission the bundle genuinely confers is refused" "403 MissingPermissionNotification on a route the account's own role opens"
  api GET "/tenants?first=1" "" "$U23_T1"
  assert_rest 403 MissingPermissionNotification

  case_ "U23.4- ...and so is the entity's own read" "403 — the session can do one thing and nothing else"
  api GET "/users?first=1" "" "$U23_T1"
  assert_rest 403 MissingPermissionNotification

  case_ "U23.5 the one permission it DOES carry actually opens its route" "204 — the flow must not deadlock: a session that cannot even rotate the credential it exists to rotate would be a dead end"
  rotate_password "$U23_ID" "$U23_T1" "$QA_USER_PASS1" "$QA_USER_PASS2"
  assert_empty_body 204

  case_ "U23.6+ the NEXT token carries the whole bundle" "tenant:read and user:read both back — the restriction was a state, not a revocation"
  U23_T2=$(qa_login "$U23_EMAIL" "$QA_USER_PASS2")
  GOT=$(jwt_claim "$U23_T2" permissions | tr -d ' \n')
  if printf '%s' "$GOT" | grep -q 'tenant:read' && printf '%s' "$GOT" | grep -q 'user:read'; then pass_; else HTTP_BODY="$GOT"; fail_ "permissions = $GOT"; fi

  case_ "U23.7+ ...and the reach is back" "200 on the route that answered 403 four cases ago"
  api GET "/tenants?first=1" "" "$U23_T2"
  assert_status 200

  case_ "U23.8 the RESPONSE BODY and the TOKEN agree" "the body advertises exactly what the token carries — the first version of this file had them disagreeing, and a client offering actions every request then refuses is what that bug looked like"
  BODY_PERMS=$(curl -s -X POST "$QA_BASE/auth/user/token" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
    -d "$(jq -nc --arg e "$U23_EMAIL" --arg p "$QA_USER_PASS2" '{email:$e, password:$p}')" \
    | jq -c '.data.user.permissions // .data.permissions // empty' | tr -d ' \n')
  TOK_PERMS=$(jwt_claim "$U23_T2" permissions | jq -c 'sort' 2>/dev/null | tr -d ' \n')
  if [ -n "$BODY_PERMS" ] && [ "$(printf '%s' "$BODY_PERMS" | jq -c 'sort' | tr -d ' \n')" = "$TOK_PERMS" ]; then pass_
  else HTTP_BODY="body=$BODY_PERMS token=$TOK_PERMS"; fail_ "body and token disagree"; fi
fi

# ── U23's superadmin half: the cut applies to `*:*` too ──────────────────────────────────
#
# The one case that proves the grant is EMBEDDED rather than FILTERED. A wildcard bundle
# contains no literal `user:change-password`, so a filtering implementation would hand this
# account an EMPTY claim and a session that can do nothing at all — including the one thing it
# exists to do.
#
# IT CANNOT BE BUILT THROUGH THE API, and that is itself a rule this suite asserts: no role may
# be created carrying the wildcard (role_rules_manual.go no-wildcard-grant) and no user may be
# granted a wildcard-bearing role (U14.3). The ONLY such account is the one migration 0012
# seeds — born must_change_password=TRUE, holding *:* through the master role — so run.sh
# publishes its pre-rotation token rather than spending it silently.
if [ -z "${QA_BOOT_ADMIN_TOKEN:-}" ]; then
  skip_ "U23.9-12 — the seeded admin's pre-rotation token was not published, so the *:* half of the token restriction is UNPROVEN this run"
else
  case_ "U23.9- a MUST-CHANGE account holding *:* carries the single permission too" "[\"user:change-password\"] — the wildcard is DROPPED, which is the whole difference between an embedded grant and a filtered one. A filtering implementation would answer [] here, because a wildcard bundle contains no such literal"
  GOT=$(jwt_claim "$QA_BOOT_ADMIN_TOKEN" permissions | tr -d ' \n')
  if [ "$GOT" = '["user:change-password"]' ]; then pass_; else HTTP_BODY="$GOT"; fail_ "permissions = $GOT"; fi

  case_ "U23.10- ...and it is refused everywhere its wildcard would otherwise reach" "403 — a *:* session in this state is not a platform operator"
  api GET "/permissions?first=1" "" "$QA_BOOT_ADMIN_TOKEN"
  assert_rest 403 MissingPermissionNotification

  case_ "U23.11 the account itself really does hold the wildcard" "the ROTATED token of the same account reaches the route the restricted one could not — proving U23.9 measured a RESTRICTION and not an account that simply held nothing"
  api GET "/permissions?first=1" "" "$QA_TOKEN_ADMIN"
  assert_status 200

  case_ "U23.12 the must_change flag is what separated them" "false now — one account, two tokens, and the only thing that changed between them is the flag the rotation cleared"
  api GET "/users/$QA_BOOT_ADMIN_ID"
  assert_json_at 200 '.data.mustChangePassword' "false"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U24 [CRITICAL] — "Who may hold a session: the account must be active AND its owning tenant
# must not be commercially suspended. Trial passes. One generic answer for every refusal."
#   source: AccountIsUsable · spec.md §7 U15 · asked 2026-09-07 (three doors, both ways)
#
# The bridge no other aggregate has: a WRITE to this entity decides who can authenticate.
# ═════════════════════════════════════════════════════════════════════════════════════════

U24_EMAIL=$(user_email u24)
U24_ID=$(new_user "$U24_EMAIL" "$TEN_U") || exit 1

case_ "U24.1+ an active user in an active tenant signs in" "a token — the positive half every door below is measured against"
U24_T=$(qa_login "$U24_EMAIL" "$QA_USER_PASS1")
if [ -n "$U24_T" ]; then pass_; else fail_ "no token"; fi

case_ "U24.2- DOOR ONE: a SUSPENDED user is refused" "no token — status is the account's own gate, and it is orthogonal to archiving"
api PATCH "/users/$U24_ID" '{"status":"suspended"}'
U24_T2=$(qa_login "$U24_EMAIL" "$QA_USER_PASS1")
if [ -z "$U24_T2" ]; then pass_; else HTTP_BODY="$(jwt_claim "$U24_T2" permissions)"; fail_ "a suspended user signed in"; fi

case_ "U24.3+ ...and reactivating restores the session" "a token again — suspension is reversible, which is the whole reason it exists beside archive"
api PATCH "/users/$U24_ID" '{"status":"active"}'
U24_T3=$(qa_login "$U24_EMAIL" "$QA_USER_PASS1")
if [ -n "$U24_T3" ]; then pass_; else fail_ "no token after reactivation"; fi

# ── DOOR TWO: the archive, and U17's mutation on the way through ─────────────────────────
U24A_EMAIL=$(user_email u24a)
U24A_ID=$(new_user "$U24A_EMAIL" "$TEN_U") || exit 1

case_ "U17.1 ARCHIVING FORCES suspended — and it reaches the ROW" "status 'suspended' on a user archived while ACTIVE. A mutation, not a validation: it raises nothing and makes archived+active unrepresentable rather than merely refused"
api PATCH "/users/$U24A_ID/archive"
api GET "/users/$U24A_ID?includeArchived=true"
assert_json_at 200 '.data.status' "suspended"

case_ "U17.2 ...and the row itself says so, not merely the read model" "the users row carries status='suspended' AND archived_at — archive is an ordinary full-field write at this pin, which is what lets the rule live in IfArchive at all"
GOT=$(sql "SELECT status || '/' || (archived_at IS NOT NULL) FROM users WHERE id = '$U24A_ID';" | tr -d '[:space:]')
if [ "$GOT" = "suspended/true" ]; then pass_; else fail_ "status/archived = '$GOT'"; fi

case_ "U24.4- DOOR TWO: an ARCHIVED user is refused" "no token — the loader's default scope refuses the row before AccountIsUsable is even reached"
U24A_T=$(qa_login "$U24A_EMAIL" "$QA_USER_PASS1")
if [ -z "$U24A_T" ]; then pass_; else fail_ "an archived user signed in"; fi

# ── DOOR THREE: the tenant's commercial state ────────────────────────────────────────────
TEN_U3=$(new_tenant active "$(ws du3)") || exit 1
U24T_EMAIL=$(user_email u24t)
U24T_ID=$(new_user "$U24T_EMAIL" "$TEN_U3") || exit 1

case_ "U24.5+ the user signs in while its tenant is active" "a token"
U24T_T=$(qa_login "$U24T_EMAIL" "$QA_USER_PASS1")
if [ -n "$U24T_T" ]; then pass_; else fail_ "no token"; fi

case_ "U24.6- DOOR THREE: suspending the TENANT stops the session" "no token — the user row is untouched and still active; a customer who stopped paying should not be issuing credentials"
api PATCH "/tenants/$TEN_U3" '{"status":"suspended"}'
U24T_T2=$(qa_login "$U24T_EMAIL" "$QA_USER_PASS1")
if [ -z "$U24T_T2" ]; then pass_; else fail_ "a suspended tenant's user signed in"; fi

case_ "U24.7 ...and the user itself was NOT touched" "status still 'active' — the refusal came from the joined tenant column, not from a cascade nobody declared"
api GET "/users/$U24T_ID"
assert_json_at 200 '.data.status' "active"

case_ "U24.8+ reactivating the tenant restores the session" "a token again"
api PATCH "/tenants/$TEN_U3" '{"status":"active"}'
U24T_T3=$(qa_login "$U24T_EMAIL" "$QA_USER_PASS1")
if [ -n "$U24T_T3" ]; then pass_; else fail_ "no token after the tenant came back"; fi

case_ "U24.9+ a TRIAL tenant signs in" "a token — a trial is a live customer being onboarded, and reading the gate as 'status != active' would break every trial signup"
TEN_TRIAL=$(new_tenant trial "$(ws dut)") || true
if [ -z "${TEN_TRIAL:-}" ]; then
  skip_ "U24.9 — a trial tenant could not be created, so the trial-passes half is UNPROVEN this run"
else
  U24TR_EMAIL=$(user_email u24tr)
  U24TR_ID=$(new_user "$U24TR_EMAIL" "$TEN_TRIAL") || true
  U24TR_T=$(qa_login "$U24TR_EMAIL" "$QA_USER_PASS1")
  if [ -n "$U24TR_T" ]; then pass_; else fail_ "a trial tenant's user could not sign in"; fi
fi

case_ "U24.10 THE THREE REFUSALS ARE INDISTINGUISHABLE" "one key for all of them — a caller learns 'no', never WHICH half failed. Telling them apart would say whether an address exists, whether it is suspended, or whether its tenant stopped paying"
K_SUSP=$(curl -s -X POST "$QA_BASE/auth/user/token" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg e "$U24A_EMAIL" --arg p "$QA_USER_PASS1" '{email:$e,password:$p}')" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
K_NOBODY=$(curl -s -X POST "$QA_BASE/auth/user/token" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg e "$(user_email ghost)" --arg p "$QA_USER_PASS1" '{email:$e,password:$p}')" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
K_WRONGPW=$(curl -s -X POST "$QA_BASE/auth/user/token" -H 'Content-Type: application/json' -H 'Accept-Language: en-US' \
  -d "$(jq -nc --arg e "$U24_EMAIL" '{email:$e,password:"Wrong!Pass2026"}')" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
if [ -n "$K_SUSP" ] && [ "$K_SUSP" = "$K_NOBODY" ] && [ "$K_SUSP" = "$K_WRONGPW" ]; then pass_
else HTTP_BODY="archived='$K_SUSP' absent='$K_NOBODY' wrong-password='$K_WRONGPW'"; fail_ "the three refusals differ"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U19/U20/U21/U22 [CRITICAL] — the credential pair: two routes that refuse each other's rows,
# the flag they disagree about, and the two proofs of possession.
#   source: user_credential_manual.go · spec.md §10 · asked 2026-09-07 (all four families)
# ═════════════════════════════════════════════════════════════════════════════════════════

U19_A_EMAIL=$(user_email u19a); U19_A_ID=$(new_user "$U19_A_EMAIL" "$TEN_U") || exit 1
U19_B_EMAIL=$(user_email u19b); U19_B_ID=$(new_user "$U19_B_EMAIL" "$TEN_U") || exit 1
# A holds the whole user vocabulary INCLUDING reset-password, so its refusals below are row
# decisions and never a missing permission — which is the distinction these cases exist for.
U19_ROLE=$(new_role "$(role_key u19)" "$TEN_U" \
  "$(permission_id_of user read)" "$(permission_id_of user update)" \
  "$(permission_id_of user reset-password)" "$(permission_id_of user change-password)") || exit 1
grant_role_to_user "$U19_A_ID" "$U19_ROLE" >/dev/null
U19_A_T=$(usable_token "$U19_A_EMAIL" "$U19_A_ID") || true

if [ -z "${U19_A_T:-}" ]; then
  skip_ "U19-U22 — the credential principal could not be provisioned, so the two row decisions are UNPROVEN this run"
else
  case_ "U19.1+ the CHANGE on one's own row" "204 — the everyday case: the caller proves what they hold and chooses what replaces it"
  rotate_password "$U19_A_ID" "$U19_A_T" "$QA_USER_PASS2" 'Qa!Own2026xy'
  assert_empty_body 204

  case_ "U19.2- the CHANGE pointed at SOMEBODY ELSE" "403 PasswordChangeRequiresSelfNotification — the route says who may attempt the verb; this says whose row they reached"
  api PATCH "/users/$U19_B_ID/password" \
    "$(jq -nc '{currentPassword:"Qa!Own2026xy", password:"Qa!Steal2026x", passwordConfirmation:"Qa!Steal2026x"}')" "$U19_A_T"
  assert_rest 403 PasswordChangeRequiresSelfNotification

  case_ "U19.3+ the RESET on somebody else's row" "204 — the helpdesk operation, carrying no current password because not knowing it is the point"
  api PATCH "/users/$U19_B_ID/password-reset" \
    '{"password":"Qa!Helpdesk26","passwordConfirmation":"Qa!Helpdesk26"}' "$U19_A_T"
  assert_empty_body 204

  case_ "U19.4- the RESET pointed at ONE'S OWN row" "403 PasswordResetRequiresAnotherUserNotification — WITHOUT this half, a holder of user:reset-password replaces their own credential without proving the previous one, defeating the change endpoint by choosing the other URL"
  api PATCH "/users/$U19_A_ID/password-reset" \
    '{"password":"Qa!Launder26x","passwordConfirmation":"Qa!Launder26x"}' "$U19_A_T"
  assert_rest 403 PasswordResetRequiresAnotherUserNotification

  case_ "U19.5 the two operations are DISJOINT BY CONSTRUCTION" "same id is always the change, a different id is always the reset — there is no row either verb can reach that the other can"
  api PATCH "/users/$U19_A_ID/password-reset" '{"password":"Qa!Nope2026xx","passwordConfirmation":"Qa!Nope2026xx"}' "$U19_A_T"
  S1="$HTTP_STATUS"
  api PATCH "/users/$U19_B_ID/password" '{"currentPassword":"Qa!Helpdesk26","password":"Qa!Nope2026xx","passwordConfirmation":"Qa!Nope2026xx"}' "$U19_A_T"
  S2="$HTTP_STATUS"
  if [ "$S1" = "403" ] && [ "$S2" = "403" ]; then pass_; else HTTP_BODY="self-reset=$S1 other-change=$S2"; fail_ "one of the two doors was open"; fi

  # ── U20: the flag the two operations disagree about ────────────────────────────────────
  case_ "U20.1 the CHANGE clears mustChangePassword" "false — the password it leaves is the caller's own choice, so there is nothing for the next sign-in to rotate"
  api GET "/users/$U19_A_ID" "" "$U19_A_T"
  assert_json_at 200 '.data.mustChangePassword' "false"

  case_ "U20.2 the RESET sets it" "true — the password a reset leaves is somebody else's choice"
  api GET "/users/$U19_B_ID" "" "$U19_A_T"
  assert_json_at 200 '.data.mustChangePassword' "true"

  case_ "U20.3 ...and that flag is what U23 reads" "B's next token carries the single permission — the two families are one mechanism seen from two sides"
  U20_T=$(qa_login "$U19_B_EMAIL" 'Qa!Helpdesk26')
  GOT=$(jwt_claim "$U20_T" permissions | tr -d ' \n')
  if [ "$GOT" = '["user:change-password"]' ]; then pass_; else HTTP_BODY="$GOT"; fail_ "permissions = $GOT"; fi

  # ── U21/U22: the two proofs of possession ──────────────────────────────────────────────
  case_ "U21.1- the CHANGE refuses a password equal to the current one" "422 PasswordUnchangedNotification — answered from the two PLAINTEXTS, for free, because the line above already established the current one verifies"
  api PATCH "/users/$U19_A_ID/password" \
    '{"currentPassword":"Qa!Own2026xy","password":"Qa!Own2026xy","passwordConfirmation":"Qa!Own2026xy"}' "$U19_A_T"
  assert_rest 422 PasswordUnchangedNotification

  case_ "U21.2- the RESET refuses one too" "422 PasswordUnchangedNotification — the SAME key by a different route: this operation never learns the current password, so the stored hash is the only thing to ask"
  api PATCH "/users/$U19_B_ID/password-reset" \
    '{"password":"Qa!Helpdesk26","passwordConfirmation":"Qa!Helpdesk26"}' "$U19_A_T"
  assert_rest 422 PasswordUnchangedNotification

  case_ "U22.1- a WRONG current password" "422 InvalidCurrentPasswordNotification"
  api PATCH "/users/$U19_A_ID/password" \
    '{"currentPassword":"Qa!Wrong2026","password":"Qa!Fresh2026x","passwordConfirmation":"Qa!Fresh2026x"}' "$U19_A_T"
  assert_rest 422 InvalidCurrentPasswordNotification

  case_ "U22.2- an EMPTY current password" "422 InvalidCurrentPasswordNotification — the SAME answer, ~100ms cheaper: it is refused without hashing anything, and it says no more than the wrong-password case does"
  api PATCH "/users/$U19_A_ID/password" \
    '{"currentPassword":"","password":"Qa!Fresh2026x","passwordConfirmation":"Qa!Fresh2026x"}' "$U19_A_T"
  assert_rest 422 InvalidCurrentPasswordNotification

  case_ "U22.3 a refused change leaves the STORED credential untouched" "the old password still signs in — StageNewPassword writes nothing that survives a refusal, and applyNewCredential is reached only once every check has passed"
  U22_T=$(qa_login "$U19_A_EMAIL" 'Qa!Own2026xy')
  if [ -n "$U22_T" ]; then pass_; else fail_ "the original password stopped working after a refused change"; fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U5/U6/U6b/U7 — the password policy at all THREE doors.
#   source: credentialValueRules — and the comment recording that the obvious repair
#           (r.ValidateValueObject) loses to the generated IgnoreValueObject IN SILENCE, which
#           once let a five-character password through both credential operations while every
#           test of the INSERT path stayed green. That is the regression these rows exist for.
# ═════════════════════════════════════════════════════════════════════════════════════════

U5_EMAIL=$(user_email u5); U5_ID=$(new_user "$U5_EMAIL" "$TEN_U") || exit 1
U5_ROLE=$(new_role "$(role_key u5)" "$TEN_U" "$(permission_id_of user change-password)") || exit 1
grant_role_to_user "$U5_ID" "$U5_ROLE" >/dev/null
U5_T=$(usable_token "$U5_EMAIL" "$U5_ID") || true
U5_VICTIM=$(user_email u5v); U5_VICTIM_ID=$(new_user "$U5_VICTIM" "$TEN_U") || exit 1

case_ "U5.1- DOOR ONE (insert): a weak password" "422 WeakPasswordNotification"
api POST /users "$(jq -nc --arg e "$(user_email u5a)" --arg t "$TEN_U" '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:"abc",passwordConfirmation:"abc"}')"
assert_rest 422 WeakPasswordNotification

case_ "U5.2- DOOR TWO (change): the SAME weak password" "422 WeakPasswordNotification — the door where the silent failure lived"
if [ -n "${U5_T:-}" ]; then
  api PATCH "/users/$U5_ID/password" "$(jq -nc --arg c "$QA_USER_PASS2" '{currentPassword:$c, password:"abc", passwordConfirmation:"abc"}')" "$U5_T"
  assert_rest 422 WeakPasswordNotification
else skip_ "U5.2 — the change principal could not be provisioned"; fi

case_ "U5.3- DOOR THREE (reset): the same" "422 WeakPasswordNotification"
api PATCH "/users/$U5_VICTIM_ID/password-reset" '{"password":"abc","passwordConfirmation":"abc"}'
assert_rest 422 WeakPasswordNotification

case_ "U7.1- the confirmation must match, at the insert" "422 PasswordConfirmationMismatchNotification"
api POST /users "$(jq -nc --arg e "$(user_email u7a)" --arg t "$TEN_U" --arg p "$QA_USER_PASS1" '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:"Other!Pass2026"}')"
assert_rest 422 PasswordConfirmationMismatchNotification

case_ "U7.2- ...at the change" "422 — restated in credentialValueRules because a declarative rule is scoped to a verb and these operations share ModeUpdate with the ordinary patch"
if [ -n "${U5_T:-}" ]; then
  api PATCH "/users/$U5_ID/password" "$(jq -nc --arg c "$QA_USER_PASS2" '{currentPassword:$c, password:"Qa!Match2026x", passwordConfirmation:"Qa!Other2026x"}')" "$U5_T"
  assert_rest 422 PasswordConfirmationMismatchNotification
else skip_ "U7.2 — the change principal could not be provisioned"; fi

case_ "U7.3- ...at the reset" "422"
api PATCH "/users/$U5_VICTIM_ID/password-reset" '{"password":"Qa!Match2026x","passwordConfirmation":"Qa!Other2026x"}'
assert_rest 422 PasswordConfirmationMismatchNotification

# ── U6: the context rule, at all three doors ─────────────────────────────────────────────
U6_EMAIL="qa-u6-$(qa_slug_runid)@authcore.local"

case_ "U6.1- a password containing the email's LOCAL PART" "422 PasswordEchoesIdentityNotification — the single most guessable credential this service could issue"
api POST /users "$(jq -nc --arg e "$U6_EMAIL" --arg t "$TEN_U" --arg p "Qa!${U6_EMAIL%%@*}9" \
  '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:$p}')"
assert_rest 422 PasswordEchoesIdentityNotification

case_ "U6.2- a password containing a NAME word" "422 PasswordEchoesIdentityNotification — case-insensitive and over RUNES, so it holds in all seven catalogs"
api POST /users "$(jq -nc --arg e "$(user_email u6b)" --arg t "$TEN_U" \
  '{givenName:"Maria",familyName:"Souza",email:$e,status:"active",tenantID:$t,password:"Maria2026!x",passwordConfirmation:"Maria2026!x"}')"
assert_rest 422 PasswordEchoesIdentityNotification

case_ "U6.3 the refused PLAINTEXT is NOT echoed back" "no message payload carries the password — every other rule in this service passes the offending input so a caller can see what was refused; here that would put the plaintext in the 422 and in any log rendering one"
if printf '%s' "$HTTP_BODY" | grep -q 'Maria2026!x'; then fail_ "the plaintext is in the response body"; else pass_; fi

case_ "U6b.1+ THE 4-RUNE FLOOR — a short family name does not ban its letters" "201 — without the floor, a name like 'Ng' would refuse every password containing 'ng', which is most of them: a rule that reads sensible and locks a population out"
api POST /users "$(jq -nc --arg e "$(user_email u6c)" --arg t "$TEN_U" \
  '{givenName:"Li",familyName:"Ng",email:$e,status:"active",tenantID:$t,password:"Strong2026!ng",passwordConfirmation:"Strong2026!ng"}')"
assert_status 201

case_ "U6b.2- ...while a 5-rune one does" "422 — the floor is 4 runes, so 'Souza' is judged and 'Ng' is not"
api POST /users "$(jq -nc --arg e "$(user_email u6d)" --arg t "$TEN_U" \
  '{givenName:"Li",familyName:"Souza",email:$e,status:"active",tenantID:$t,password:"Strong2026!souza",passwordConfirmation:"Strong2026!souza"}')"
assert_rest 422 PasswordEchoesIdentityNotification

case_ "U6.4- the context rule holds at the CHANGE door too" "422 — shared method, so the three entry points cannot drift apart about what 'echoes the identity' means"
if [ -n "${U5_T:-}" ]; then
  api PATCH "/users/$U5_ID/password" "$(jq -nc --arg c "$QA_USER_PASS2" --arg p "Qa!${U5_EMAIL%%@*}9" '{currentPassword:$c, password:$p, passwordConfirmation:$p}')" "$U5_T"
  assert_rest 422 PasswordEchoesIdentityNotification
else skip_ "U6.4 — the change principal could not be provisioned"; fi

case_ "U6.5- ...and at the RESET door" "422"
api PATCH "/users/$U5_VICTIM_ID/password-reset" "$(jq -nc --arg p "Qa!${U5_VICTIM%%@*}9" '{password:$p, passwordConfirmation:$p}')"
assert_rest 422 PasswordEchoesIdentityNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# U3 — the owning tenant must be available, and TRIAL PASSES. IfInsert only.
#   source: spec.md §7 U3 · refuseUnavailableTenant
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "U3.1+ a user in an ACTIVE tenant" "201"
api POST /users "$(user_body "$(user_email u3a)" active "$TEN_U")"
assert_status 201

case_ "U3.2+ a user in a TRIAL tenant" "201 — 'unavailable' is not 'not active'. Reading this as Status != active would break every trial signup, and users are the first thing a customer being onboarded needs"
TEN_U3T=$(new_tenant trial "$(ws du3t)") || true
if [ -n "${TEN_U3T:-}" ]; then
  api POST /users "$(user_body "$(user_email u3b)" active "$TEN_U3T")"
  assert_status 201
else skip_ "U3.2 — a trial tenant could not be created"; fi

case_ "U3.3- a user in a SUSPENDED tenant" "422 UserTenantDoesNotExistNotification"
TEN_U3S=$(new_tenant active "$(ws du3s)") || exit 1
api PATCH "/tenants/$TEN_U3S" '{"status":"suspended"}'
api POST /users "$(user_body "$(user_email u3c)" active "$TEN_U3S")"
assert_rest 422 UserTenantDoesNotExistNotification

case_ "U3.4- a user in an ARCHIVED tenant" "422 UserTenantDoesNotExistNotification — the same key for a third cause"
TEN_U3A=$(new_tenant active "$(ws du3a)") || exit 1
api PATCH "/tenants/$TEN_U3A/archive"
api POST /users "$(user_body "$(user_email u3d)" active "$TEN_U3A")"
assert_rest 422 UserTenantDoesNotExistNotification

case_ "U3.5- a user in a tenant that never existed" "422 — a fourth cause, one answer"
api POST /users "$(user_body "$(user_email u3e)" active "01990000-dead-7000-8000-000000000000")"
assert_rest 422 UserTenantDoesNotExistNotification

case_ "U3.6 THE ASYMMETRY: a PATCH still succeeds on a user whose tenant has SINCE been suspended" "200 — IfInsert only. Suspension withholds NEW users; it does not freeze the ones already there, and re-asking on every update would make a user impossible to RENAME the day their tenant is suspended"
TEN_U3P=$(new_tenant active "$(ws du3p)") || exit 1
U3P_ID=$(new_user "$(user_email u3f)" "$TEN_U3P") || exit 1
api PATCH "/tenants/$TEN_U3P" '{"status":"suspended"}'
api PATCH "/users/$U3P_ID" '{"givenName":"Renamed"}'
assert_json_at 200 '.data.givenName' "Renamed"

case_ "U3.7 ...and so does the ARCHIVE" "204 — a suspended tenant's rows still have to be administrable"
api PATCH "/users/$U3P_ID/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# U4 — TENANT ISOLATION ON WRITES, including the archive.
#   source: spec.md §7 U4 · refuseForeignTenant, under IfInsertOrUpdate AND IfArchive
#
# qa/security.sh S7.3d-e proves the same refusals as a BOUNDARY — "is this what stands between
# a principal and another tenant's rows". These two rows prove them as the RULE the entity
# declares, which is the question §1b asks, and they are here because a reader auditing the
# business rules of User reads this file and not that one.
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_USEROP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "U4 — principal I was not built, so write-side tenant isolation is UNPROVEN in this lane (qa/security.sh S7.3 asserts the same refusals as a boundary)"
else
  U4_FOREIGN=$(new_tenant active "$(ws du4)") || true

  case_ "U4.1+ a scoped caller writes inside its OWN tenant" "201 — the rule refuses foreign rows, not the caller"
  api POST /users "$(user_body "$(user_email u4a)" active "$QA_TENANT_SCOPED")" "$QA_TOKEN_USEROP"
  assert_json_at 201 '.data.tenantID' "$QA_TENANT_SCOPED"

  case_ "U4.2- ...and naming ANOTHER tenant is refused" "403 TenantMismatchNotification — U1b refuses rather than silently overriding, so a caller who supplies the wrong tenant learns it instead of quietly writing somewhere else"
  if [ -n "${U4_FOREIGN:-}" ]; then
    api POST /users "$(user_body "$(user_email u4b)" active "$U4_FOREIGN")" "$QA_TOKEN_USEROP"
    assert_rest 403 TenantMismatchNotification
  else skip_ "U4.2 — the foreign tenant could not be created"; fi

  case_ "U4.3- THE ARCHIVE IS GUARDED TOO" "403 TenantMismatchNotification — refuseForeignTenant runs under IfArchive as well. The write side is not filtered by ToCriteria, so the foreign row LOADS and the rule is what refuses: a suite that tested only inserts would never see this gate"
  if [ -n "${U4_FOREIGN:-}" ]; then
    U4_VICTIM=$(new_user "$(user_email u4c)" "$U4_FOREIGN") || true
    if [ -n "${U4_VICTIM:-}" ]; then
      api PATCH "/users/$U4_VICTIM/archive" "" "$QA_TOKEN_USEROP"
      assert_rest 403 TenantMismatchNotification
    else skip_ "U4.3 — the foreign user could not be created"; fi
  else skip_ "U4.3 — the foreign tenant could not be created"; fi

  case_ "U4.4 the check stands down for a *:* caller" "204 — the operator supporting a customer has to be able to repair a row that is not theirs, and that bypass is the whole reason the rule asks about the CLAIM rather than about the row alone"
  if [ -n "${U4_VICTIM:-}" ]; then
    api PATCH "/users/$U4_VICTIM/archive"
    assert_empty_body 204
  else skip_ "U4.4 — the foreign user could not be created"; fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U18 — THE READ SCOPE, and the leak that is a 200.
#   source: spec.md §7 U16 · both ToCriteria implementations
#
# spec.md is explicit about the cost of getting this wrong: "a user listing is the customer's
# staff directory INCLUDING E-MAIL ADDRESSES — leaking it across tenants is strictly worse than
# leaking the org chart, which Group already refused."
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_USEROP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "U18 — principal I was not built, so the read scope is UNPROVEN in this lane"
else
  U18_FOREIGN_TEN=$(new_tenant active "$(ws du18)") || true
  U18_FOREIGN=$([ -n "${U18_FOREIGN_TEN:-}" ] && new_user "$(user_email u18)" "$U18_FOREIGN_TEN" || true)

  case_ "U18.1+ the caller sees its own tenant's users" "at least one row, and every row is its own — asserted over the VALUES, because a count passes while one foreign row rides along"
  api GET "/users?first=100" "" "$QA_TOKEN_USEROP"
  assert_json_at 200 '[.data[].tenantID] | unique | join(",")' "$QA_TENANT_SCOPED"

  case_ "U18.2- THE LEAK ITSELF: another tenant's user is simply NOT THERE" "the foreign id appears in no row of the listing. This is the one protection in the entity that is not a refusal — nothing is rejected, rows just are not there — which is exactly what makes it invisible to a suite that only asserts status codes"
  if [ -n "${U18_FOREIGN:-}" ]; then
    assert_json "[.data[].id] | index(\"$U18_FOREIGN\") | . == null" "true"
  else skip_ "U18.2 — the foreign user could not be created"; fi

  case_ "U18.3- and the by-id read answers 404, NOT 403" "a 403 confirms the id exists to a caller who may not see it; a 404 says nothing about who else is on the platform. This service answers 'you may not' and 'it is not there' identically wherever telling them apart would leak"
  if [ -n "${U18_FOREIGN:-}" ]; then
    api GET "/users/$U18_FOREIGN" "" "$QA_TOKEN_USEROP"
    assert_status 404
  else skip_ "U18.3 — the foreign user could not be created"; fi

  case_ "U18.4+ a *:* holder skips the filter" "200 on the very id the scoped caller was refused — a scope that hides rows from everyone is broken in the other direction"
  if [ -n "${U18_FOREIGN:-}" ]; then
    api GET "/users/$U18_FOREIGN"
    assert_json_at 200 '.data.id' "$U18_FOREIGN"
  else skip_ "U18.4 — the foreign user could not be created"; fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U8/U9/U10 — a group / role / claim must exist, be ACTIVE, and belong to THIS USER's tenant.
# One answer for all three causes, per collection.
#   source: spec.md §7 U8/U9 · evolve spec §4d.1 · refuseUnjoinableGroups / …Roles / …Claims
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_FOREIGN=$(new_tenant active "$(ws dufor)") || exit 1
R_MINE=$(new_role "$(role_key umine)" "$TEN_U" "$P_TENANT_READ_U") || exit 1
G_MINE=$(new_group "$(group_key umine)" "$TEN_U" "$R_MINE") || exit 1
C_MINE=$(new_claim "$(claim_name umine)" string both "$TEN_U") || exit 1
R_FOREIGN=$(new_role "$(role_key ufor)" "$TEN_FOREIGN" "$P_TENANT_READ_U") || exit 1
G_FOREIGN=$(new_group "$(group_key ufor)" "$TEN_FOREIGN" "$R_FOREIGN") || exit 1
C_FOREIGN=$(new_claim "$(claim_name ufor)" string both "$TEN_FOREIGN") || exit 1
R_DEAD=$(new_role "$(role_key udead)" "$TEN_U" "$P_TENANT_READ_U") || exit 1
G_DEAD=$(new_group "$(group_key udead)" "$TEN_U" "$R_MINE") || exit 1
C_DEAD=$(new_claim "$(claim_name udead)" string both "$TEN_U") || exit 1
api PATCH "/roles/$R_DEAD/archive"; api PATCH "/groups/$G_DEAD/archive"; api PATCH "/claims/$C_DEAD/archive"

U8_ID=$(new_user "$(user_email u8)" "$TEN_U") || exit 1

case_ "U8.1+ a group from the user's OWN tenant" "201"
api POST "/users/$U8_ID/groups" "$(jq -nc --arg g "$G_MINE" '{groupID:$g}')"
assert_status 201

case_ "U8.2- a group from ANOTHER tenant" "422 GroupNotAvailableInTenantNotification"
api POST "/users/$U8_ID/groups" "$(jq -nc --arg g "$G_FOREIGN" '{groupID:$g}')"
assert_rest 422 GroupNotAvailableInTenantNotification

case_ "U8.3- an ARCHIVED group in the right tenant" "422 — the SAME key"
api POST "/users/$U8_ID/groups" "$(jq -nc --arg g "$G_DEAD" '{groupID:$g}')"
assert_rest 422 GroupNotAvailableInTenantNotification

case_ "U8.4- a group id that never existed" "422 — the same key, third cause. A distinct 'belongs to another tenant' reply would confirm to a caller in tenant A that a specific UUID is a live group in some other tenant: an existence oracle over a competitor's org chart"
api POST "/users/$U8_ID/groups" '{"groupID":"01990000-dead-7000-8000-000000000000"}'
assert_rest 422 GroupNotAvailableInTenantNotification

case_ "U9.1+ a role from the user's own tenant" "201"
api POST "/users/$U8_ID/roles" "$(jq -nc --arg r "$R_MINE" '{roleID:$r}')"
assert_status 201

case_ "U9.2- a role from another tenant" "422 RoleNotAvailableInTenantNotification"
api POST "/users/$U8_ID/roles" "$(jq -nc --arg r "$R_FOREIGN" '{roleID:$r}')"
assert_rest 422 RoleNotAvailableInTenantNotification

case_ "U9.3- an archived role" "422, same key"
api POST "/users/$U8_ID/roles" "$(jq -nc --arg r "$R_DEAD" '{roleID:$r}')"
assert_rest 422 RoleNotAvailableInTenantNotification

case_ "U9.4- a role id that never existed" "422, same key"
api POST "/users/$U8_ID/roles" '{"roleID":"01990000-dead-7000-8000-000000000000"}'
assert_rest 422 RoleNotAvailableInTenantNotification

case_ "U10.1+ a claim definition from the user's own tenant" "201"
api POST "/users/$U8_ID/claims" "$(jq -nc --arg c "$C_MINE" '{claimID:$c, value:"ok"}')"
assert_status 201

case_ "U10.2- a definition from another tenant" "422 ClaimNotAvailableInTenantNotification — an existence oracle over a competitor's claim vocabulary is what the single key refuses to be"
api POST "/users/$U8_ID/claims" "$(jq -nc --arg c "$C_FOREIGN" '{claimID:$c, value:"x"}')"
assert_rest 422 ClaimNotAvailableInTenantNotification

case_ "U10.3- an archived definition" "422, same key"
api POST "/users/$U8_ID/claims" "$(jq -nc --arg c "$C_DEAD" '{claimID:$c, value:"x"}')"
assert_rest 422 ClaimNotAvailableInTenantNotification

case_ "U10.4- a definition id that never existed" "422, same key"
api POST "/users/$U8_ID/claims" '{"claimID":"01990000-dead-7000-8000-000000000000","value":"x"}'
assert_rest 422 ClaimNotAvailableInTenantNotification

case_ "U12b THE INTERLOCK: one bad id gets ONE answer" "ClaimNotAvailableInTenantNotification ALONE — the loop continues after the availability refusal, because nothing below can say anything true about a definition that is not there and every probe answers 'the problem is present' for an id it cannot resolve"
api POST "/users/$U8_ID/claims" '{"claimID":"01990000-dead-7000-8000-000000000000","value":"legit"}'
assert_json_at 422 '[.errors[]?.messages[]?.notificationKey] | unique | join(",")' "ClaimNotAvailableInTenantNotification"

# ═════════════════════════════════════════════════════════════════════════════════════════
# U11 — a claim whose appliesTo is `client` cannot be set on a USER. `user` and `both` pass.
#   source: evolve spec §4d.2 — "what finally makes AppliesTo mean something"
# ═════════════════════════════════════════════════════════════════════════════════════════

C_USER=$(new_claim "$(claim_name uu)" string user "$TEN_U") || exit 1
C_BOTH=$(new_claim "$(claim_name ub)" string both "$TEN_U") || exit 1
C_CLIENT=$(new_claim "$(claim_name uc)" string client "$TEN_U") || exit 1
U11_ID=$(new_user "$(user_email u11)" "$TEN_U") || exit 1

case_ "U11.1+ appliesTo: user" "201"
api POST "/users/$U11_ID/claims" "$(jq -nc --arg c "$C_USER" '{claimID:$c, value:"v"}')"
assert_status 201

case_ "U11.2+ appliesTo: both" "201"
api POST "/users/$U11_ID/claims" "$(jq -nc --arg c "$C_BOTH" '{claimID:$c, value:"v"}')"
assert_status 201

case_ "U11.3- appliesTo: client" "422 ClaimDoesNotApplyToUserNotification — the column stops merely stating something as data and starts being enforced"
api POST "/users/$U11_ID/claims" "$(jq -nc --arg c "$C_CLIENT" '{claimID:$c, value:"v"}')"
assert_rest 422 ClaimDoesNotApplyToUserNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# U12 — a claim value must parse as the definition's declared valueType. ADDED **and**
# CHANGED, because the PATCH carries a value nothing has judged yet.
#   source: evolve spec §4d.3
# ═════════════════════════════════════════════════════════════════════════════════════════

C_NUMBER=$(new_claim "$(claim_name un)" number both "$TEN_U") || exit 1
C_BOOL=$(new_claim "$(claim_name ubl)" bool both "$TEN_U") || exit 1
C_STRING=$(new_claim "$(claim_name ust)" string both "$TEN_U") || exit 1
U12_ID=$(new_user "$(user_email u12)" "$TEN_U") || exit 1

case_ "U12.1+ number ← a valid decimal" "201"
api POST "/users/$U12_ID/claims" "$(jq -nc --arg c "$C_NUMBER" '{claimID:$c, value:"1000.5"}')"
assert_status 201

# A FRESH USER PER NEGATIVE. Reusing the one its positive just wrote to would meet the
# duplicate rule first — a 409 that says nothing about the value type.
U12_BAD1=$(new_user "$(user_email u12a)" "$TEN_U") || exit 1
case_ "U12.2- number ← 'abc'" "422 ClaimValueDoesNotMatchValueTypeNotification"
api POST "/users/$U12_BAD1/claims" "$(jq -nc --arg c "$C_NUMBER" '{claimID:$c, value:"abc"}')"
assert_rest 422 ClaimValueDoesNotMatchValueTypeNotification

case_ "U12.3 the refusal echoes the VALUE, not the id" "'abc' in the payload — the caller knows which entry they sent; what they need told back is the string that did not parse"
if printf '%s' "$HTTP_BODY" | grep -q 'abc'; then pass_; else fail_ "the offending value is not in the body"; fi

case_ "U12.4+ bool ← 'true'" "201"
api POST "/users/$U12_ID/claims" "$(jq -nc --arg c "$C_BOOL" '{claimID:$c, value:"true"}')"
assert_status 201

U12_BAD2=$(new_user "$(user_email u12b)" "$TEN_U") || exit 1
case_ "U12.5- bool ← 'yes'" "422 — exactly 'true' or 'false', the same reading the catalog's own default-value check uses. Two levels of one chain must not disagree about what a bool is"
api POST "/users/$U12_BAD2/claims" "$(jq -nc --arg c "$C_BOOL" '{claimID:$c, value:"yes"}')"
assert_rest 422 ClaimValueDoesNotMatchValueTypeNotification

case_ "U12.6+ string ← any non-empty value" "201"
api POST "/users/$U12_ID/claims" "$(jq -nc --arg c "$C_STRING" '{claimID:$c, value:"anything"}')"
U12_CH=$(printf '%s' "$HTTP_BODY" | jq -r '.data.userClaim.id // empty')
assert_status 201

case_ "U12.7- THE PATCH IS JUDGED TOO" "422 — refuseUnsettableClaims walks ADDED and CHANGED, because a correction arrives with a value nothing has judged yet: exactly as unjudged as a new one. Judging only the additions would let 'correct this cost center' write anything at all"
U12N_CH=$(printf '%s' "$(api POST "/users/$U12_ID/claims" "$(jq -nc --arg c "$C_NUMBER" '{claimID:$c, value:"1"}')"; printf '%s' "$HTTP_BODY")" | jq -r '.data.userClaim.id // empty' 2>/dev/null)
U12P_ID=$(new_user "$(user_email u12p)" "$TEN_U") || exit 1
U12P_CH=$(set_claim "$U12P_ID" "$C_NUMBER" "1") || true
if [ -n "${U12P_CH:-}" ]; then
  api PATCH "/users/$U12P_ID/claims/$U12P_CH" '{"value":"abc"}'
  assert_rest 422 ClaimValueDoesNotMatchValueTypeNotification
else skip_ "U12.7 — the entry to correct could not be provisioned"; fi

case_ "U12.8+ ...and a VALID correction passes" "200 — the rule judges the value, it does not forbid the verb"
if [ -n "${U12P_CH:-}" ]; then
  api PATCH "/users/$U12P_ID/claims/$U12P_CH" '{"value":"2000"}'
  assert_json_at 200 '.data.userClaim.value' "2000"
else skip_ "U12.8 — the entry to correct could not be provisioned"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U13/U14 — NO PRIVILEGE ESCALATION, at three hops and at two, plus the two wildcard guards.
#   source: spec.md §7 U12a/U12b/U13a/U13b · refuseUnjoinableGroups / refuseUngrantableRoles
#           · asked 2026-09-07 (all six rows)
#
# PRINCIPAL I is the caller these rows need: it holds the whole user vocabulary and
# deliberately NOT permission:archive. The role that CONFERS permission:archive is created by
# the ADMIN, because Role's own escalation rule would refuse I that creation (role-contract
# RL2); the question here is whether I may ATTACH it.
# ═════════════════════════════════════════════════════════════════════════════════════════

if [ -z "${QA_TOKEN_USEROP:-}" ] || [ -z "${QA_TENANT_SCOPED:-}" ]; then
  skip_ "U13/U14 — principal I was not built, so all six escalation rows are UNPROVEN this run"
else
  # Built by the ADMIN, inside principal I's tenant: a role conferring a permission I lacks,
  # and a group conferring that role. Plus a harmless pair I does hold, for the positives.
  ESC_ROLE=$(new_role "$(role_key uesc)" "$QA_TENANT_SCOPED" "$P_PERM_ARCHIVE_U") || true
  ESC_GROUP=$(new_group "$(group_key uesc)" "$QA_TENANT_SCOPED" "${ESC_ROLE:-}") || true
  OK_ROLE=$(new_role "$(role_key uok)" "$QA_TENANT_SCOPED" "$P_TENANT_READ_U") || true
  OK_GROUP=$(new_group "$(group_key uok)" "$QA_TENANT_SCOPED" "${OK_ROLE:-}") || true
  WILD_ROLE=$(role_id_of master)
  ESC_TARGET=$(new_user "$(user_email uesc)" "$QA_TENANT_SCOPED") || true

  if [ -z "${ESC_GROUP:-}" ] || [ -z "${ESC_TARGET:-}" ] || [ -z "${OK_GROUP:-}" ]; then
    skip_ "U13/U14 — the escalation fixtures could not be provisioned, so all six rows are UNPROVEN this run"
  else
    case_ "U14.1+ TWO HOPS: I grants a role conferring only what I holds" "201 — the gate refuses escalation, not delegation"
    api POST "/users/$ESC_TARGET/roles" "$(jq -nc --arg r "$OK_ROLE" '{roleID:$r}')" "$QA_TOKEN_USEROP"
    assert_status 201

    case_ "U14.2- ...and refuses one conferring a permission I LACK" "403 CannotGrantRoleWithUnheldPermissionsNotification — a caller may grant a role only if they hold EVERY permission it grants"
    api POST "/users/$ESC_TARGET/roles" "$(jq -nc --arg r "$ESC_ROLE" '{roleID:$r}')" "$QA_TOKEN_USEROP"
    assert_rest 403 CannotGrantRoleWithUnheldPermissionsNotification

    # ── THE WILDCARD ROLE, and why it takes a whole fixture of its own ──────────────────
    #
    # Two facts have to hold at once for CannotGrantWildcardRole to be reachable, and getting
    # either wrong makes the case pass for the wrong reason or skip forever:
    #
    #   1. NO WILDCARD-BEARING ROLE CAN BE CREATED. role_rules_manual.go's no-wildcard-grant
    #      refuses the wildcard permission on any role through this API — deliberately, since
    #      Identity.HasPermission PANICS on an argument containing '*'. So the ONLY such role
    #      is the seeded `master` one.
    #   2. `master` lives in the MASTER tenant, and role-available-in-tenant runs FIRST. From
    #      any other tenant the answer is RoleNotAvailableInTenant and the wildcard probe is
    #      never reached — the interlock working exactly as designed.
    #
    # So the case needs a user in the master tenant AND a caller who is not a super-admin.
    # The admin builds both; the caller is what makes the refusal meaningful, because a *:*
    # principal would pass the escalation probe and never reach this one either.
    MASTER_TEN=$(jwt_claim "$QA_TOKEN_ADMIN" tenant_id)
    WILD_ROLE_SEEDED=$(role_id_of master)
    WILD_TARGET=$([ -n "$MASTER_TEN" ] && new_user "$(user_email uwild)" "$MASTER_TEN" || true)
    WILD_CALLER_EMAIL=$(user_email uwildc)
    WILD_CALLER_ROLE=$([ -n "$MASTER_TEN" ] && new_role "$(role_key uwildc)" "$MASTER_TEN" \
      "$(permission_id_of user read)" "$(permission_id_of user grant)" || true)
    WILD_CALLER_ID=""
    if [ -n "${WILD_CALLER_ROLE:-}" ]; then
      WILD_CALLER_ID=$(new_user "$WILD_CALLER_EMAIL" "$MASTER_TEN") || true
      [ -n "$WILD_CALLER_ID" ] && grant_role_to_user "$WILD_CALLER_ID" "$WILD_CALLER_ROLE" >/dev/null
    fi
    WILD_CALLER_T=$([ -n "${WILD_CALLER_ID:-}" ] && usable_token "$WILD_CALLER_EMAIL" "$WILD_CALLER_ID" || true)

    case_ "U14.3- a WILDCARD-bearing role" "403 CannotGrantWildcardRoleNotification — and it fires BEFORE the escalation probe, which is load-bearing: Identity.HasPermission PANICS on any argument containing '*', so this is what removes the input that would crash the request into a 500 on exactly the case the escalation rule exists to stop"
    if [ -n "${WILD_CALLER_T:-}" ] && [ -n "${WILD_TARGET:-}" ] && [ -n "${WILD_ROLE_SEEDED:-}" ]; then
      api POST "/users/$WILD_TARGET/roles" "$(jq -nc --arg r "$WILD_ROLE_SEEDED" '{roleID:$r}')" "$WILD_CALLER_T"
      assert_rest 403 CannotGrantWildcardRoleNotification
    else skip_ "U14.3 — the master-tenant fixtures for the wildcard rule could not be provisioned, so CannotGrantWildcardRole is UNPROVEN this run"; fi

    case_ "U14.3b THE ORDER IS LOAD-BEARING: availability answers FIRST" "422 RoleNotAvailableInTenantNotification, not the wildcard key — the same seeded role, pointed at a user in ANOTHER tenant. This is why U14.3 needs the master tenant at all, and it pins the interlock rather than assuming it"
    api POST "/users/$ESC_TARGET/roles" "$(jq -nc --arg r "$WILD_ROLE_SEEDED" '{roleID:$r}')" "$QA_TOKEN_USEROP"
    assert_rest 422 RoleNotAvailableInTenantNotification

    case_ "U13.1+ THREE HOPS: I joins a group whose roles confer only what I holds" "201"
    api POST "/users/$ESC_TARGET/groups" "$(jq -nc --arg g "$OK_GROUP" '{groupID:$g}')" "$QA_TOKEN_USEROP"
    assert_status 201

    case_ "U13.2- ...and is refused on one whose roles confer a permission I LACK" "403 CannotJoinGroupWithUnheldPermissionsNotification — group → roles → permissions, a SET and not one key"
    api POST "/users/$ESC_TARGET/groups" "$(jq -nc --arg g "$ESC_GROUP" '{groupID:$g}')" "$QA_TOKEN_USEROP"
    assert_rest 403 CannotJoinGroupWithUnheldPermissionsNotification

    case_ "U13.3 THE KEYS DIFFER BY DEPTH" "the group refusal and the role refusal carry different notification keys — folding them together would hide which hop a refusal came from, which is why refuseUnjoinableGroups and refuseUngrantableRoles are separate methods rather than one generic walk"
    api POST "/users/$ESC_TARGET/roles" "$(jq -nc --arg r "$ESC_ROLE" '{roleID:$r}')" "$QA_TOKEN_USEROP"
    K_ROLE_ESC=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
    api POST "/users/$ESC_TARGET/groups" "$(jq -nc --arg g "$ESC_GROUP" '{groupID:$g}')" "$QA_TOKEN_USEROP"
    K_GROUP_ESC=$(printf '%s' "$HTTP_BODY" | jq -r '[.errors[]?.messages[]?.notificationKey]|unique|join(",")')
    if [ -n "$K_ROLE_ESC" ] && [ -n "$K_GROUP_ESC" ] && [ "$K_ROLE_ESC" != "$K_GROUP_ESC" ]; then pass_
    else HTTP_BODY="role='$K_ROLE_ESC' group='$K_GROUP_ESC'"; fail_ "the two depths answer the same key"; fi

    case_ "U13.4- a group carrying a WILDCARD-bearing role" "UNREACHABLE from the wire, and the reason is a rule rather than a gap"
    # Two guards close every door into this state, and neither can be opened by any caller:
    #   · Group's own CannotGrantWildcardRole refuses attaching the seeded `master` role to any
    #     group (group-contract GR10a), so no group can come to carry a wildcard; and
    #   · no wildcard-bearing role can be CREATED either (role_rules_manual.go
    #     no-wildcard-grant), so there is no second role to build such a group out of.
    # Migration 0012 seeds the wildcard onto a ROLE and onto no group. The notification is
    # therefore defence-in-depth behind a state the service cannot reach — the same shape
    # P3e, RL7- and GR8-b record for their own unreachable guards. Asserting it would mean
    # writing a row straight into the database, which is not a contract this suite may test.
    WILD_GROUP_TRY=$(new_group "$(group_key uwildg)" "$MASTER_TEN" "$WILD_ROLE_SEEDED" 2>/dev/null) || true
    if [ -n "${WILD_GROUP_TRY:-}" ]; then
      api POST "/users/$WILD_TARGET/groups" "$(jq -nc --arg g "$WILD_GROUP_TRY" '{groupID:$g}')" "$WILD_CALLER_T"
      assert_rest 403 CannotJoinWildcardGroupNotification
    else
      skip_ "U13.4 — CannotJoinWildcardGroupNotification is UNREACHABLE from the wire: Group's own rule refuses attaching the wildcard-bearing seeded role to any group, no other wildcard-bearing role can be created, and migration 0012 seeds the wildcard onto a role and onto no group. The guard is defence-in-depth behind a state no request can produce"
    fi

    case_ "U13.5+ the ADMIN crosses all of it" "201 — a *:* superadmin passes by construction: HasPermission answers true for any concrete permission when the claim set carries the wildcard"
    ESC_TARGET2=$(new_user "$(user_email uesc2)" "$QA_TENANT_SCOPED") || true
    if [ -n "${ESC_TARGET2:-}" ]; then
      api POST "/users/$ESC_TARGET2/groups" "$(jq -nc --arg g "$ESC_GROUP" '{groupID:$g}')"
      assert_status 201
    else skip_ "U13.5 — the second escalation target could not be created"; fi

    case_ "U13.6 THE RULES JUDGE ONLY WHAT THIS WRITE ADDS" "200 — with the escalating group already attached by the admin, principal I can still RENAME the user. Re-judging stored memberships would make unrelated writes hostages of the past, and would stop the operator who has to fix exactly this situation from doing anything at all"
    if [ -n "${ESC_TARGET2:-}" ]; then
      api PATCH "/users/$ESC_TARGET2" '{"givenName":"Renamed"}' "$QA_TOKEN_USEROP"
      assert_json_at 200 '.data.givenName' "Renamed"
    else skip_ "U13.6 — the second escalation target could not be created"; fi

    case_ "U13.7+ ...and I can still REMOVE the membership it may not have added" "204 — the tool for fixing the situation must not be the thing the rule takes away"
    if [ -n "${ESC_TARGET2:-}" ]; then
      CH_ESC=$(sql "SELECT id FROM user_groups WHERE user_id='$ESC_TARGET2' AND group_id='$ESC_GROUP' AND archived_at IS NULL LIMIT 1;" | tr -d '[:space:]')
      if [ -n "$CH_ESC" ]; then
        api PATCH "/users/$ESC_TARGET2/groups/$CH_ESC/archive" "" "$QA_TOKEN_USEROP"
        assert_empty_body 204
      else skip_ "U13.7 — the membership row could not be located"; fi
    else skip_ "U13.7 — the second escalation target could not be created"; fi
  fi
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U15 — the three caps. HYBRID, decided by the maintainer 2026-09-07: the CLAIMS cap (20) is
# proven end to end because it is the cap this round adds; the two 50-caps are proven at the
# BOUNDARY only, and this file records that the passing side at exactly 50 is NOT exercised.
#   source: spec.md §7 U11a/U11b · evolve spec §4d claims-cap
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_CAP=$(new_tenant active "$(ws ducap)") || exit 1
U15_ID=$(new_user "$(user_email u15)" "$TEN_CAP") || exit 1
CAP_OK=1
CAP_IDS=""
for i in $(seq 1 20); do
  cid=$(new_claim "$(claim_name cap)" string both "$TEN_CAP") || { CAP_OK=0; break; }
  CAP_IDS="$CAP_IDS $cid"
  set_claim "$U15_ID" "$cid" "v$i" >/dev/null || { CAP_OK=0; break; }
done

case_ "U15.1+ TWENTY claim values attach, and the twentieth succeeds" "20 entries on the read-back — the cap counts the whole collection, so the passing side has to be reached for real"
if [ "$CAP_OK" = "1" ]; then
  api GET "/users/$U15_ID"
  assert_json_at 200 '.data.claims | length' "20"
else
  skip_ "U15.1 — the twenty definitions could not be provisioned, so the claims cap is UNPROVEN this run"
fi

case_ "U15.2- the TWENTY-FIRST claim value" "the User cap is UNREACHABLE from the wire, and what refuses first is worth asserting instead"
# TWO CAPS OF TWENTY, ONE BEHIND THE OTHER. User's claims-cap allows 20 values; Claim's own
# claims-per-tenant-cap-users allows 20 DEFINITIONS per tenant applying to users
# (claim_rules_manual.go refuseCatalogBudgetExceeded, bounded at 20 for the same token-size
# reason). Every value a user holds must point at a definition in the user's own tenant (U10),
# so a user can never be offered a twenty-first — the catalog runs out first.
# TooManyClaimsForUserNotification is therefore a backstop behind a boundary the caller meets
# one level earlier, and the honest assertion is the one that DOES fire.
if [ "$CAP_OK" = "1" ]; then
  api POST /claims "$(jq -nc --arg n "$(claim_name cap21)" --arg t "$TEN_CAP" \
    '{name:$n, valueType:"string", appliesTo:"both", tenantID:$t,
      description:"The twenty-first definition in one tenant, which the catalog budget is what refuses."}')"
  assert_rest 422 TooManyUserClaimsInTenantNotification
else
  skip_ "U15.2 — the twenty definitions could not be provisioned, so neither cap was reached"
fi

case_ "U15.2b and the User cap is recorded as UNREACHABLE rather than claimed" "named, not asserted"
skip_ "TooManyClaimsForUserNotification cannot be provoked through this API: a user's values must point at definitions in its own tenant, and Claim's catalog budget stops the twentieth-first definition from existing (U15.2). The guard is a backstop behind a boundary the caller meets one level earlier — the same shape P3e, RL7- and GR8-b record for their own unreachable rules. U15.1 proves the passing side at exactly 20"

# THE BOUNDARY FORM THE PLAN APPROVED DOES NOT REACH THIS RULE, and the run proved it: 51
# copies of one id is a DUPLICATE, so UserAlreadyInGroup answers before the cap ever counts.
# The two 50-caps are therefore proven with 51 DISTINCT counterparts — more coverage than §1b
# U15 approved, not less, and the only shape that reaches the rule at all.
G_CAP_IDS=""
G_CAP_OK=1
for i in $(seq 1 51); do
  gid=$(new_group "$(group_key ucap)" "$TEN_CAP") || { G_CAP_OK=0; break; }
  G_CAP_IDS="$G_CAP_IDS $gid"
done

case_ "U15.3- the GROUPS cap" "422 TooManyGroupsForUserNotification on an insert carrying 51 DISTINCT memberships — the cap counts the whole collection, so it is reached before any per-entry probe has a verdict"
if [ "$G_CAP_OK" = "1" ]; then
  ENTRIES=$(printf '%s\n' $G_CAP_IDS | jq -R . | jq -sc 'map(select(length>0) | {groupID: .})')
  api POST /users "$(jq -nc --arg e "$(user_email u15g)" --arg t "$TEN_CAP" --arg p "$QA_USER_PASS1" --argjson g "$ENTRIES" \
    '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:$p,groups:$g}')"
  assert_rest 422 TooManyGroupsForUserNotification
else skip_ "U15.3 — the fifty-one distinct groups could not be provisioned, so the groups cap is UNPROVEN this run"; fi

case_ "U15.3b+ ...and FIFTY of the same memberships pass" "201 — the passing side, which the approved hybrid form did not reach and which is what makes the cap a boundary rather than a refusal"
if [ "$G_CAP_OK" = "1" ]; then
  ENTRIES50=$(printf '%s\n' $G_CAP_IDS | jq -R . | jq -sc 'map(select(length>0) | {groupID: .}) | .[0:50]')
  api POST /users "$(jq -nc --arg e "$(user_email u15g5)" --arg t "$TEN_CAP" --arg p "$QA_USER_PASS1" --argjson g "$ENTRIES50" \
    '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:$p,groups:$g}')"
  assert_json_at 201 '.data.groups | length' "50"
else skip_ "U15.3b — the fifty-one distinct groups could not be provisioned"; fi

R_CAP_IDS=""
R_CAP_OK=1
for i in $(seq 1 51); do
  rid=$(new_role "$(role_key ucap)" "$TEN_CAP" "$P_TENANT_READ_U") || { R_CAP_OK=0; break; }
  R_CAP_IDS="$R_CAP_IDS $rid"
done

case_ "U15.4- the ROLES cap" "422 TooManyRolesForUserNotification on an insert carrying 51 DISTINCT grants"
if [ "$R_CAP_OK" = "1" ]; then
  ENTRIES=$(printf '%s\n' $R_CAP_IDS | jq -R . | jq -sc 'map(select(length>0) | {roleID: .})')
  api POST /users "$(jq -nc --arg e "$(user_email u15r)" --arg t "$TEN_CAP" --arg p "$QA_USER_PASS1" --argjson r "$ENTRIES" \
    '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:$p,roles:$r}')"
  assert_rest 422 TooManyRolesForUserNotification
else skip_ "U15.4 — the fifty-one distinct roles could not be provisioned, so the roles cap is UNPROVEN this run"; fi

case_ "U15.4b+ ...and FIFTY of the same grants pass" "201 — the passing side"
if [ "$R_CAP_OK" = "1" ]; then
  ENTRIES50=$(printf '%s\n' $R_CAP_IDS | jq -R . | jq -sc 'map(select(length>0) | {roleID: .}) | .[0:50]')
  api POST /users "$(jq -nc --arg e "$(user_email u15r5)" --arg t "$TEN_CAP" --arg p "$QA_USER_PASS1" --argjson r "$ENTRIES50" \
    '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:$p,passwordConfirmation:$p,roles:$r}')"
  assert_json_at 201 '.data.roles | length' "50"
else skip_ "U15.4b — the fifty-one distinct roles could not be provisioned"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U16 — the status machine. active↔suspended and no-ops only.
#   source: spec.md §7 U14
# ═════════════════════════════════════════════════════════════════════════════════════════

U16_ID=$(new_user "$(user_email u16)" "$TEN_U") || exit 1

case_ "U16.1+ active → suspended" "200"
api PATCH "/users/$U16_ID" '{"status":"suspended"}'
assert_json_at 200 '.data.status' "suspended"

case_ "U16.2+ suspended → active" "200"
api PATCH "/users/$U16_ID" '{"status":"active"}'
assert_json_at 200 '.data.status' "active"

case_ "U16.3+ a no-op" "200 — staying is always allowed, so a PATCH whose only change is a rename is never hostage to the state machine"
api PATCH "/users/$U16_ID" '{"givenName":"Same","status":"active"}'
assert_json_at 200 '.data.givenName' "Same"

case_ "U16.4- a value outside the closed set" "409 carrying BOTH keys — the value object reports the unknown member and the transition rule reports the illegal move, and SemanticStateConflict outranks the validation for the status. Cross-referenced to M5.10, which pins the same pair from the framework side"
api PATCH "/users/$U16_ID" '{"status":"deleted"}'
assert_json_at 409 '[.errors[]?.messages[]?.notificationKey] | unique | sort | join(",")' \
  "InvalidUserStatusTransitionNotification,UnknownUserStatusNotification"

# ═════════════════════════════════════════════════════════════════════════════════════════
# U1/U2 — the two immutability rules, and the DISJUNCTION the plan declared BEFORE the first
# request. PatchUserRequest declares only givenName, familyName and status, so a PATCH carries
# no email and no tenantID for the notifications to fire on. What is asserted is THE DTO GATE:
# a field the write DTO does not declare must not reach the aggregate — either the framework
# rejects the unknown key, or it drops it and the stored value is untouched. Both are the
# contract; a CHANGED value is not.
#   source: spec.md §7 U1/U2b · plan §1b, the row whose expected status is stated as a
#           disjunction precisely so it could not be filled in from an answer.
# ═════════════════════════════════════════════════════════════════════════════════════════

U1_EMAIL=$(user_email u1); U1_ID=$(new_user "$U1_EMAIL" "$TEN_U") || exit 1

case_ "U1.1 a PATCH carrying an email the DTO does not declare" "either a typed refusal, or the key is dropped — never a changed address. The email is the login handle: silently moving it is the one outcome that is not the contract"
api PATCH "/users/$U1_ID" "$(jq -nc --arg e "$(user_email u1x)" '{givenName:"Kept", email:$e}')"
S_U1="$HTTP_STATUS"
api GET "/users/$U1_ID"
GOT_EMAIL=$(printf '%s' "$HTTP_BODY" | jq -r '.data.email')
if [ "$GOT_EMAIL" = "$U1_EMAIL" ]; then
  pass_
  printf '      %snote: the write answered HTTP %s and the address is unchanged%s\n' "$C_DIM" "$S_U1" "$C_RESET"
else
  HTTP_BODY="write answered $S_U1; stored email is now '$GOT_EMAIL', was '$U1_EMAIL'"
  fail_ "the email MOVED"
fi

case_ "U2.1 a PATCH carrying a tenantID the DTO does not declare" "the same disjunction, and the same one forbidden outcome: a user never moves between tenants"
TEN_U2=$(new_tenant active "$(ws du2)") || exit 1
api PATCH "/users/$U1_ID" "$(jq -nc --arg t "$TEN_U2" '{givenName:"Kept", tenantID:$t}')"
S_U2="$HTTP_STATUS"
api GET "/users/$U1_ID"
GOT_TEN=$(printf '%s' "$HTTP_BODY" | jq -r '.data.tenantID')
if [ "$GOT_TEN" = "$TEN_U" ]; then
  pass_
  printf '      %snote: the write answered HTTP %s and the tenant is unchanged%s\n' "$C_DIM" "$S_U2" "$C_RESET"
else
  HTTP_BODY="write answered $S_U2; stored tenantID is now '$GOT_TEN', was '$TEN_U'"
  fail_ "the user MOVED tenant"
fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# U25 — the password hash reaches no surface, no filter, no ordering, no projection and no
# audit payload. The wire faces live in qa/user.sh (M3, M8.23-27) and qa/user_graphql.sh
# (N2.4-5); this row is the one nobody can see through the API.
#   source: user_schema.go RedactedField(InSync ***, InAudit ***) · spec.md §9 · asked
# ═════════════════════════════════════════════════════════════════════════════════════════

U25_EMAIL=$(user_email u25)
U25_ID=$(new_user "$U25_EMAIL" "$TEN_U") || exit 1

case_ "U25.1 the hash IS stored — the column is not empty" "a PHC-encoded Argon2id string, because the credential has to verify a login"
GOT=$(sql "SELECT left(password_hash, 9) FROM users WHERE id = '$U25_ID';" | tr -d '[:space:]')
if [ "$GOT" = '$argon2id' ]; then pass_; else HTTP_BODY="$GOT"; fail_ "password_hash starts with '$GOT'"; fi

case_ "U25.2 the INSERT's audit payload carries *** and not the hash" "InAudit(RedactWith(\"***\")) — the only redacted field in this service"
GOT=$(sql "SELECT payload::jsonb #>> '{snapshot,PasswordHash}' FROM audit_events WHERE entity_type='User' AND aggregate_id='$U25_ID' AND verb='insert' LIMIT 1;" | tr -d '[:space:]')
if [ "$GOT" = "***" ]; then pass_; else HTTP_BODY="$GOT"; fail_ "audit PasswordHash = '$GOT'"; fi

case_ "U25.2b the redaction is SCOPED to the one column" "the snapshot still carries Status and MustChangePassword in the clear — a redaction that blanked the record would destroy the trail it exists to protect"
GOT=$(sql "SELECT (payload::jsonb #>> '{snapshot,Status}') || '/' || (payload::jsonb #>> '{snapshot,MustChangePassword}') FROM audit_events WHERE entity_type='User' AND aggregate_id='$U25_ID' AND verb='insert' LIMIT 1;" | tr -d '[:space:]')
if [ "$GOT" = "active/true" ]; then pass_; else HTTP_BODY="$GOT"; fail_ "snapshot status/mustChange = '$GOT'"; fi

case_ "U25.3 the real hash appears NOWHERE in the audit row" "not under another key, not in a nested snapshot, not in a diff — the whole row is searched, because a redaction that covers one path and misses another is not a redaction"
REAL=$(sql "SELECT password_hash FROM users WHERE id = '$U25_ID';" | tr -d '[:space:]')
GOT=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$U25_ID' AND payload::text LIKE '%' || substring('$REAL' from 20 for 20) || '%';" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else HTTP_BODY="rows containing a 20-char slice of the real hash: $GOT"; fail_ "the hash leaked into audit_events"; fi

case_ "U25.4 a RESET's audit payload is redacted too" "*** after the credential was replaced — the operation that writes a NEW hash is the one most likely to log it"
api PATCH "/users/$U25_ID/password-reset" '{"password":"Qa!Audit2026x","passwordConfirmation":"Qa!Audit2026x"}'
REAL2=$(sql "SELECT password_hash FROM users WHERE id = '$U25_ID';" | tr -d '[:space:]')
GOT=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$U25_ID' AND payload::text LIKE '%' || substring('$REAL2' from 20 for 20) || '%';" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else HTTP_BODY="rows containing a slice of the post-reset hash: $GOT"; fail_ "the new hash leaked into audit_events"; fi

case_ "U25.5 the plaintext never reaches the audit trail either" "no row of this user's trail contains the password the caller sent — it arrives on a field with no column and goes into the hasher, and nothing copies it on the way"
GOT=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$U25_ID' AND payload::text LIKE '%Qa!Audit2026x%';" | tr -d '[:space:]')
if [ "$GOT" = "0" ]; then pass_; else HTTP_BODY="rows containing the plaintext: $GOT"; fail_ "the PLAINTEXT leaked into audit_events"; fi

qa_finish
