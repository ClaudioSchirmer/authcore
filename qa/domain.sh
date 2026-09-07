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

qa_finish
