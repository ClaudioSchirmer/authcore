#!/usr/bin/env bash
# Lane: domain — §1b of specs/qa/tenant-contract/plan.md (R1-R8)
#                and §1b of specs/qa/permission-contract/plan.md (P1-P6).
#
# The only cases in this suite NOT derived from the framework's docs, and the
# only ones that can fail for a reason the framework never had an opinion about.
# A service can satisfy every promise in qa/tenant.sh and still be wrong about
# the business; these are the rows that would see it.
#
# Each rule gets BOTH halves. A rule with only a happy path is a rule nobody
# tested — the negative case is the entire point. Sources are named per rule:
# specs/scaffold-entity/tenant/spec.md, the rules.manual items in
# specs/omnicore-gen/tenant.omnicore.yaml, and the maintainer's own answers at
# the Phase 1 gate.
#
# R1, R2 and R3 are the rows the maintainer named CRITICAL.

cd "$(dirname "$0")/.." || exit 2
LANE_NAME=domain
. qa/lib.bash

sign_in "$BOOTSTRAP_EMAIL" "$BOOTSTRAP_INITIAL_PASSWORD"
[ "$RESP_CODE" = "200" ] || sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
[ "$RESP_CODE" = "200" ] || { printf 'precondition: could not sign in (%s) %s\n' "$RESP_CODE" "$RESP_BODY"; exit 2; }
USER_ID="$(j '.data.user.id')"; TOKEN="$(j '.data.accessToken')"
if [ "$(j '.data.user.mustChangePassword')" = "true" ]; then
  req PATCH "/users/${USER_ID}/password" \
    "$(jq -nc --arg c "$BOOTSTRAP_INITIAL_PASSWORD" --arg p "$QA_ADMIN_PASSWORD" \
       '{currentPassword:$c, password:$p, passwordConfirmation:$p}')"
  sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"; TOKEN="$(j '.data.accessToken')"
fi
[ -n "$TOKEN" ] || { printf 'precondition: no access token\n'; exit 2; }

SCOPE="qa-domain-${QA_RUN_TAG}"

# ════════════════════════════════════════════════════════════════════════════
section "R1 🔴 CRITICAL · unarchiving returns the tenant SUSPENDED"
note "spec.md §7 rule 13 + consequence 2 · rules.manual: archive-forces-suspended"
note "\"restoring a tenant must never silently resume a billable, functioning account\""

W="${SCOPE}-r1"
ID="$(create_tenant "$W" 'Billable Active Tenant' 'A commercially active tenant about to be archived.' 'active')"
[ -n "$ID" ] || { fail "R1 setup" "a created tenant" "no id returned" "$RESP_BODY"; }

req GET "/tenants/${ID}"
assert_jq "R1 positive · the tenant starts active" '.data.status' 'active'

req PATCH "/tenants/${ID}/archive" ''
assert_status "R1 positive · archive" 204
# The rule is a MUTATION, not a validation: it must reach the ROW, not merely the
# audit event. Reading the archived row back is the only way to see that.
req GET "/tenants/${ID}?includeArchived=true"
assert_jq "R1 positive · archiving forced suspended ON THE ROW" '.data.status' 'suspended'

req PATCH "/tenants/${ID}/unarchive" ''
assert_status "R1 negative · unarchive" 204
req GET "/tenants/${ID}"
# THE case. An "active" here means a restored tenant silently resumed billing.
assert_jq "R1 negative · the restored tenant is STILL suspended" '.data.status' 'suspended'

# ════════════════════════════════════════════════════════════════════════════
section "R2 🔴 CRITICAL · an archived workspace still blocks the handle"
note "tenant.omnicore.yaml fields[].unique.scope: all · spec.md §7 rule 5"
note "\"an archived remnant MUST keep blocking the handle — it is what URLs, logs and support carry\""

W2="${SCOPE}-r2"
req POST /tenants "$(tenant_body 'Original Handle Holder' "$W2" 'The first tenant to hold this particular handle.')"
assert_status "R2 positive · a fresh handle inserts" 201
ID2="$(j '.data.id')"

req PATCH "/tenants/${ID2}/archive" ''
assert_status "R2 archive the holder (setup)" 204
req GET "/tenants?workspace=${W2}"
assert_jq "R2 the holder is gone from ordinary reads" '.data | length' 0

# The negative case: the handle is invisible and must STILL be taken. If this
# answers 201, a new tenant has just inherited a retired tenant's URLs, logs and
# support history.
req POST /tenants "$(tenant_body 'Handle Squatter' "$W2" 'A different tenant trying to take a retired handle.')"
assert_status_key_field "R2 negative · the retired handle is still held" 409 'TenantWorkspaceAlreadyExistsNotification' 'workspace'

# ════════════════════════════════════════════════════════════════════════════
section "R3 🔴 CRITICAL · the reserved workspace list"
note "spec.md §7 rule 4 + the reserved list — five of these are routes this service serves TODAY"

req POST /tenants "$(tenant_body 'Ordinary Company' "${SCOPE}-r3ok" 'An ordinary tenant with a handle nobody reserved.')"
assert_status "R3 positive · a non-reserved handle is accepted" 201

# The five that are live routes of this very service, plus the collection segment
# and the phishing-prone words the spec calls out.
for reserved in admin api docs graphql livez readyz openapi tenants tenant www root login security support system; do
  req POST /tenants "$(tenant_body 'Reserved Handle Attempt' "$reserved" 'A tenant attempting to claim a platform-reserved handle.')"
  assert_status_key "R3 negative · workspace '${reserved}' is reserved" 422 'ReservedTenantWorkspaceNotification'
done

# ════════════════════════════════════════════════════════════════════════════
section "R4 · no tenant returns to trial"
note "spec.md §7 rule 12 · rules.list: status-transition — \"a trial is a beginning\""

# The three legal moves, each proven on its own record.
T1="$(create_tenant "${SCOPE}-r4a" 'Trial To Active' 'A tenant moving from trial into active service.' 'trial')"
req PATCH "/tenants/${T1}" '{"status":"active"}'
assert_status "R4 positive · trial → active" 200
req PATCH "/tenants/${T1}" '{"status":"suspended"}'
assert_status "R4 positive · active → suspended" 200
req PATCH "/tenants/${T1}" '{"status":"active"}'
assert_status "R4 positive · suspended → active" 200

T2="$(create_tenant "${SCOPE}-r4b" 'Trial To Suspended' 'A tenant suspended straight out of its trial period.' 'trial')"
req PATCH "/tenants/${T2}" '{"status":"suspended"}'
assert_status "R4 positive · trial → suspended" 200

# The no-op. Rule 12 admits it explicitly, and the generated guard compares
# old != new before consulting the table — so re-sending the same value must pass.
req PATCH "/tenants/${T2}" '{"status":"suspended"}'
assert_status "R4 positive · a no-op transition (same value) is allowed" 200

# The negative half: both roads back to trial.
T3="$(create_tenant "${SCOPE}-r4c" 'Active Tenant' 'An active tenant that must not be walked back into a trial.' 'active')"
req PATCH "/tenants/${T3}" '{"status":"trial"}'
assert_status_key "R4 negative · active → trial is refused" 422 'InvalidTenantStatusTransitionNotification'
req PATCH "/tenants/${T2}" '{"status":"trial"}'
assert_status_key "R4 negative · suspended → trial is refused" 422 'InvalidTenantStatusTransitionNotification'

# ════════════════════════════════════════════════════════════════════════════
section "R5 · the description must differ from the name and from the workspace"
note "rules.manual: description-differs-from-name-and-workspace · spec.md §7 rule 10"
note "the comparison is normalized: case-folded, whitespace and hyphens collapsed"

req POST /tenants "$(tenant_body 'Distinct Name Here' "${SCOPE}-r5ok" 'A description that genuinely says something else entirely.')"
assert_status "R5 positive · a genuinely distinct description" 201

NAME='Acme Comercio'
req POST /tenants "$(tenant_body "$NAME" "${SCOPE}-r5a" "$NAME")"
assert_status_key "R5 negative · description equals the name verbatim" 422 'TenantDescriptionMustDifferNotification'

# The normalization is the part worth testing: different case, doubled spaces and
# a hyphen must all fold to the same comparison form. If this passes as 201, the
# normalizer is not doing the work it was written for.
req POST /tenants "$(tenant_body "$NAME" "${SCOPE}-r5b" '  ACME   comercio  ')"
assert_status_key "R5 negative · description equals the name under case/space folding" 422 'TenantDescriptionMustDifferNotification'

W5="${SCOPE}-r5c"
req POST /tenants "$(tenant_body 'Some Other Name' "$W5" "$(printf '%s' "$W5" | tr '-' ' ')")"
assert_status_key "R5 negative · description equals the workspace under hyphen folding" 422 'TenantDescriptionMustDifferNotification'

# ════════════════════════════════════════════════════════════════════════════
section "R6 · no silent normalization — a handle is refused, never repaired"
note "spec.md §7 rule 14 — \"storing something the caller did not send is worse than a 422\""

W6="${SCOPE}-r6"
req POST /tenants "$(tenant_body 'Compliant Handle Co' "$W6" 'A tenant whose handle already complies with the DNS-label shape.')"
assert_status "R6 positive · a compliant handle is accepted" 201
# And it comes back byte-identical. A handle silently rewritten is the exact
# failure the rule exists to prevent, and only a read-back can see it.
assert_jq "R6 positive · the stored handle is byte-identical to what was sent" '.data.workspace' "$W6"

UPPER="$(printf '%s' "$W6" | tr '[:lower:]' '[:upper:]')"
req POST /tenants "$(tenant_body 'Uppercase Attempt' "$UPPER" 'A tenant sending an uppercase handle that must not be lowercased for it.')"
assert_status_key "R6 negative · an uppercase handle is refused, not lowercased" 422 'InvalidTenantWorkspaceNotification'

req POST /tenants "$(tenant_body 'Padded Attempt' "  ${SCOPE}-r6b  " 'A tenant sending a padded handle that must not be trimmed for it.')"
assert_status_key "R6 negative · a padded handle is refused, not trimmed" 422 'InvalidTenantWorkspaceNotification'

# ════════════════════════════════════════════════════════════════════════════
section "R7 · archiving is one-way — setting a tenant active does NOT unarchive it"
note "spec.md §7 rule 13 consequence 3 — \"archiving is removal, status is commercial state\""

W7="${SCOPE}-r7"
ID7="$(create_tenant "$W7" 'One Way Tenant' 'A tenant used to prove archiving and status are independent axes.' 'active')"
req PATCH "/tenants/${ID7}/archive" ''
assert_status "R7 archive (setup)" 204

# The write must reach the archived row at all — this is the belt-and-braces
# check that the case below is asserting on something real.
req PATCH "/tenants/${ID7}" '{"status":"active"}'
R7_PATCH_CODE="$RESP_CODE"
note "PATCH status:active on an archived tenant answered HTTP ${R7_PATCH_CODE}"

# THE case: whatever that answered, the tenant must still be archived.
req GET "/tenants/${ID7}"
assert_status "R7 negative · the tenant is STILL archived after a status write" 404
req GET "/tenants/${ID7}?includeArchived=true"
assert_status "R7 the row is still there behind ?includeArchived" 200

# ════════════════════════════════════════════════════════════════════════════
section "R8 · a workspace key in a PATCH body changes nothing, and is not an error"
note "DECIDED by the maintainer at the Phase 1 gate: immutability here is STRUCTURAL —"
note "Workspace is not a member of PatchTenantRequest (patchExcludes), so the key is client noise"

W8="${SCOPE}-r8"
ID8="$(create_tenant "$W8" 'Structural Immutability' 'A tenant used to prove the handle cannot move through the update DTO.')"
req PATCH "/tenants/${ID8}" '{"name":"Renamed But Same Handle","workspace":"hijacked-handle"}'
assert_status "R8 positive · a workspace key in the body is accepted and ignored" 200
assert_jq "R8 positive · the response still carries the original handle" '.data.workspace' "$W8"
# The read-back is what actually proves it: a response echoing the old value while
# the row moved would pass an assertion on the response alone.
req GET "/tenants/${ID8}"
assert_jq "R8 positive · the STORED handle is unchanged" '.data.workspace' "$W8"
assert_jq "R8 positive · the rest of the patch did apply" '.data.name' 'Renamed But Same Handle'
req GET "/tenants?workspace=hijacked-handle"
assert_jq "R8 positive · no tenant answers to the hijacked handle" '.data | length' 0

skip "R8b TenantWorkspaceIsImmutableNotification" "declared in the model and UNREACHABLE through REST and GraphQL — patchExcludes removes Workspace from the update DTO, so the belt-and-braces layer sits behind a door no surface can open. Recorded, not silently dropped."

# ════════════════════════════════════════════════════════════════════════════
# PERMISSION — §1b of specs/qa/permission-contract/plan.md.
#
# All four rules the maintainer was asked to rank came back CRITICAL, so the
# order below is by setup cost, not by importance. P6 is LAST in the file and
# the lane is LAST in the runner, because it is irreversible within a run.
# ════════════════════════════════════════════════════════════════════════════

# Segment-safe: a resource is one lowercase slug with no run of four identical
# runes, which the raw run tag does not guarantee. See _slug in qa/lib.bash.
PPFX="qa-$(_slug "$LANE_NAME")-$(_slug "${QA_RUN_TAG:-$$}")"

# ════════════════════════════════════════════════════════════════════════════
section "P1 🔴 CRITICAL · the pair is frozen after creation"
note "spec.md §7c rule 9, §B Q2 · rules.list: key-immutable"
note "\"editing it would rewrite the meaning of every existing grant, invisibly\""

R_P1="${PPFX}-p1"
ID_P1="$(create_permission "$R_P1" read)"
[ -n "$ID_P1" ] || fail "P1 setup" "a created permission" "no id returned" "$RESP_BODY"

req PATCH "/permissions/${ID_P1}" '{"description":"Amended wording, which is the whole of this operation."}'
assert_status "P1 positive · the description IS editable" 200
assert_jq "P1 positive · and the pair did not move" '.data.permission' "${R_P1}:read"

# The negative. Immutability here is STRUCTURAL — neither half is a member of
# PatchPermissionRequest — so the keys are ignored rather than refused, and a 200
# is the correct answer. What must hold is that the STORED pair did not move: an
# assertion on the response alone would pass even if the row had changed.
req PATCH "/permissions/${ID_P1}" \
  '{"resource":"hijack","action":"insert","description":"A patch aiming both halves at something else entirely."}'
assert_status "P1 negative · the pair keys are not members of the body" 200
req GET "/permissions/${ID_P1}"
assert_jq "P1 negative · the STORED pair is unchanged" '.data.permission' "${R_P1}:read"
req GET '/permissions?resource.eq=hijack'
assert_jq "P1 negative · no permission answers to the hijacked resource" '.data | length' 0

skip "P1b PermissionKeyIsImmutableNotification" "declared in the model and UNREACHABLE through REST and GraphQL — patchExcludes: [Permission] removes both halves from the update DTO, so the belt-and-braces layer sits behind a door no surface can open. Recorded, not silently dropped."

# ════════════════════════════════════════════════════════════════════════════
section "P2 🔴 CRITICAL · active-only uniqueness, and re-insert as the only way back"
note "unique.scope: active-only · spec.md §B Q3 + Q5"
note "the exact INVERSE of tenant.workspace, whose archived remnant keeps blocking"

R_P2="${PPFX}-p2"
# req POST directly: create_permission runs in a subshell under command
# substitution, so RESP_CODE would not reach the assertion.
req POST /permissions "$(permission_body "$R_P2" read)"
assert_status "P2 setup · the pair is taken" 201
ID_P2A="$(j '.data.id')"

req POST /permissions "$(permission_body "$R_P2" read)"
assert_status_key_field "P2 negative · an ACTIVE pair cannot be duplicated" 409 'PermissionAlreadyExistsNotification' 'permission'
assert_jq "P2 negative · the refused pair is echoed back" '.errors[0].messages[0].value' "${R_P2}:read"

req PATCH "/permissions/${ID_P2A}/archive" ''
assert_status "P2 positive · archive the holder" 204
req POST /permissions "$(permission_body "$R_P2" read)"
assert_status "P2 positive · the archived pair is free again" 201
ID_P2B="$(j '.data.id')"
# A NEW row with a NEW id, granted explicitly — never a restore. That difference
# is the whole reason unarchive does not exist on this aggregate.
[ -n "$ID_P2B" ] && [ "$ID_P2B" != "$ID_P2A" ] \
  && pass "P2 positive · it returns as a NEW row, not as a restored one" \
  || fail "P2 positive · it returns as a NEW row, not as a restored one" \
          "an id different from ${ID_P2A}" "${ID_P2B:-<none>}" "$RESP_BODY"
req GET "/permissions?resource.eq=${R_P2}&includeArchived=true"
assert_jq "P2 positive · the retired row stays as history" '.data | length' 2

# ════════════════════════════════════════════════════════════════════════════
section "P3 🔴 CRITICAL · the wildcard is a WHOLE half, never a piece of one"
note "spec.md §7a rule 2, §7b rule 6 · vos/permission_key.go"
note "the claim matcher honours exactly: exact · resource:* · *:*"

req POST /permissions "$(permission_body "${PPFX}-p3" '*' 'Every action on the p3 fixture resource, as one grantable row.')"
assert_status "P3 positive · the wildcard as an ENTIRE action half" 201

# Reported against the ACTION, which is the half that has to change: with a
# wildcard resource, only a wildcard action makes the row matchable.
req POST /permissions "$(permission_body '*' 'read' 'A wildcard resource paired with one concrete action.')"
assert_status_key_field "P3 negative · a wildcard resource with a concrete action" 422 'UnmatchablePermissionKeyNotification' 'action'

req POST /permissions "$(permission_body 'user:*' 'read' 'A wildcard used as a segment inside a resource path.')"
assert_status_key_field "P3 negative · the wildcard inside a path" 422 'InvalidResourceNameNotification' 'resource'
req POST /permissions "$(permission_body 'ten*' 'read' 'A wildcard mixed into a slug rather than standing alone.')"
assert_status_key_field "P3 negative · the wildcard mixed into a slug" 422 'InvalidResourceNameNotification' 'resource'
req POST /permissions "$(permission_body "${PPFX}-p3b" 're*d' 'A wildcard mixed into the action slug.')"
assert_status_key_field "P3 negative · the same rule on the action half" 422 'InvalidActionNameNotification' 'action'

# ════════════════════════════════════════════════════════════════════════════
section "P4 🔴 CRITICAL · the description explains the permission, never echoes it"
note "rules.manual: description-does-not-echo-key · spec.md §7c rule 10"
note "normalized comparison: letters and digits only, case-folded"

R_P4="${PPFX}-p4:profile"
req POST /permissions "$(permission_body "$R_P4" 'read' 'Open a profile and read the attributes it carries.')"
assert_status "P4 positive · a description that explains is accepted" 201

# Built from the rendered pair so the two normalize identically by construction —
# colons and hyphens become spaces, which is exactly what the rule collapses.
# It clears the Description VO on its own (well past 15 runes, several words,
# more than five distinct runes, a vowel, no run of four), so a pass here is the
# echo rule firing and never the length rule.
R_P4B="${PPFX}-p4b:profile"
ECHO_DESC="$(printf '%s' "${R_P4B}:read" | tr ':-' '  ')"
req POST /permissions "$(permission_body "$R_P4B" 'read' "$ECHO_DESC")"
assert_status_key_field "P4 negative · a description that merely restates the pair" 422 'PermissionDescriptionEchoesKeyNotification' 'description'

# ════════════════════════════════════════════════════════════════════════════
section "P5 · no silent normalization — the value is compared byte for byte"
note "spec.md §7a · the rendered pair is matched against a token claim verbatim"

R_P5="${PPFX}-p5"
create_permission "$R_P5" 'rotate-secret' >/dev/null
req GET "/permissions?resource.eq=${R_P5}"
assert_jq "P5 positive · the pair reads back byte-identical to what was sent" '.data[0].permission' "${R_P5}:rotate-secret"

req POST /permissions "$(permission_body 'Tenant' 'read' 'An uppercase resource that must be refused rather than folded.')"
assert_status_key_field "P5 negative · uppercase is refused, never lowercased" 422 'InvalidResourceNameNotification' 'resource'
req POST /permissions "$(permission_body ' tenant ' 'read' 'A padded resource that must be refused rather than trimmed.')"
assert_status_key_field "P5 negative · padding is refused, never trimmed" 422 'InvalidResourceNameNotification' 'resource'

# ════════════════════════════════════════════════════════════════════════════
# §1b of specs/qa/role-contract/plan.md — RL1..RL10.
#
# These sit BEFORE P6 and that is structural, not cosmetic: P6 archives the *:*
# catalog row and ends the run's ability to authenticate as an operator. Every
# row below needs an operator.
#
# Four of these families the maintainer named CRITICAL: the escalation pair, the
# tenant isolation with its Layer-3 read scope, the catalog check, and the
# suspended-tenant rule. Role is also the first aggregate in this service whose
# rules read the PRINCIPAL, so most of what follows needs a caller who is NOT a
# super-admin — provisioned through the service's own flow, never forged.
# ════════════════════════════════════════════════════════════════════════════

RSCOPE="qa-rl-${QA_RUN_TAG}"
ADMIN_TOKEN="$TOKEN"
MASTER_TENANT_ID='01990000-0001-7000-8000-000000000001'
MASTER_ROLE_ID='01990000-0002-7000-8000-000000000001'
WILDCARD_ID='01990000-0000-7000-8000-000000000000'
RL_DESC='A role the domain lane created so exactly one business rule can be judged.'

PID_TENANT_READ="$(permission_id_of tenant read)"
PID_ROLE_READ="$(permission_id_of role read)"
PID_PERM_ARCHIVE="$(permission_id_of permission archive)"

# The tenant this lane's own roles live in — kept apart from master, so a
# cross-tenant case has two real partitions to cross.
T_D="$(create_tenant "$(_slug "${RSCOPE}-own")" 'Qa Domain Roles' 'The tenant the domain lane keeps its own roles in, so a cross-tenant case has somewhere to cross from.' 'active')"

# The scoped principal: authenticated, NOT a super-admin, bound to a tenant of
# the suite's own making, holding a chosen bundle — and deliberately WITHOUT
# permission:archive, which is what RL2's negative case needs.
provision_scoped_principal "rlp" \
  role:insert role:update role:archive role:read role:grant tenant:read
SP_OK=$?
TOKEN="$ADMIN_TOKEN"

# ════════════════════════════════════════════════════════════════════════════
section "RL1 🔴 CRITICAL · the wildcard is refused for EVERYONE, with no exemption"
note "spec.md §7 R9b · rules.manual: no-wildcard-grant"
note "the negative is aimed at the *:* super-admin ON PURPOSE: if any caller were"
note "exempt it would be them, and migration 0012's own header says this refusal is"
note "why a migration is the only way a super-admin can come into existence."

RL1_ROLE="$(create_role "$(_slug "${RSCOPE}-w")" "$T_D" '[]')"
req POST "/roles/${RL1_ROLE}/permissions" "$(jq -nc --arg p "$PID_TENANT_READ" '{permissionID:$p}')"
assert_status "RL1 positive · the admin grants a concrete catalog permission" 201

req POST "/roles/${RL1_ROLE}/permissions" "$(jq -nc --arg p "$WILDCARD_ID" '{permissionID:$p}')"
assert_status_key "RL1 negative · the *:* super-admin is refused the wildcard grant" 403 'CannotGrantWildcardPermissionNotification'

# RL1b — the same refusal on the INSERT path, and the reason the order of the two
# rules is load-bearing. Identity.HasPermission PANICS on any argument containing
# '*' (application/configuration/identity.go:52), so the wildcard rule is what
# removes the input that would crash the request into a 500 on exactly the case
# the escalation rule exists to stop. A 500 here is the regression.
req POST /roles "$(role_body "$(_slug "${RSCOPE}-w2")" 'Qa Wildcard Insert' "$RL_DESC" "$T_D" \
  "$(jq -nc --arg a "$WILDCARD_ID" '[$a]')")"
assert_status_key "RL1b negative · the wildcard is refused on the INSERT path too" 403 'CannotGrantWildcardPermissionNotification'
if [ "$RESP_CODE" != "500" ]; then
  pass "RL1b and it is a ${RESP_CODE}, never a 500 — the panic input never reached HasPermission"
else
  fail "RL1b the wildcard refusal is never a 500" "403, the rule order having removed the panic input" "HTTP 500" "$RESP_BODY"
fi

# ════════════════════════════════════════════════════════════════════════════
section "RL2 🔴 CRITICAL · you may only grant what you hold"
note "spec.md §7 R9a, §B Q3 — \"só pode conceder o que você tem, a não ser que"
note "você seja um *:*\". The super-admin exemption is FREE, not special-cased:"
note "HasPermission answers true for any concrete key when the claim set holds *:*."

if [ "$SP_OK" -ne 0 ] || [ -z "$SP_TOKEN" ]; then
  skip "RL2 / RL3 / RL3b · every rule needing a scoped principal" \
       "the principal could not be provisioned — ${SP_FAILED:-unknown reason}. These rules are UNPROVEN this run."
else
  pass "RL0 a scoped, non-super-admin principal was provisioned through the service's own flow"

  # The role it will act on lives in ITS OWN tenant, so R5 cannot fire first and
  # answer with a different key than the one under test.
  RL2_ROLE="$(create_role "$(_slug "${RSCOPE}-esc")" "$SP_TENANT_ID" '[]')"

  req_astoken "$SP_TOKEN" POST "/roles/${RL2_ROLE}/permissions" \
    "$(jq -nc --arg p "$PID_TENANT_READ" '{permissionID:$p}')"
  assert_status "RL2 positive · it grants tenant:read, which its own role carries" 201

  req_astoken "$SP_TOKEN" POST "/roles/${RL2_ROLE}/permissions" \
    "$(jq -nc --arg p "$PID_PERM_ARCHIVE" '{permissionID:$p}')"
  assert_status_key "RL2 negative · and is refused permission:archive, which it does not hold" 403 'CannotGrantUnheldPermissionNotification'

  # The complement. Without it RL2 would pass just as well for a service that
  # refuses EVERY grant — the mirror failure of a gate that refuses everyone.
  req POST "/roles/${RL2_ROLE}/permissions" "$(jq -nc --arg p "$PID_PERM_ARCHIVE" '{permissionID:$p}')"
  assert_status "RL2b positive · the *:* admin grants the very same permission" 201

  # ══════════════════════════════════════════════════════════════════════════
  section "RL3 🔴 CRITICAL · tenant isolation binds WRITES"
  note "spec.md §7 R5, §10 Layer 2, §B Q4 — reads AND writes both"

  req_astoken "$SP_TOKEN" POST /roles "$(role_body "$(_slug "${RSCOPE}-own1")" 'Qa Own Tenant' \
    'A role the scoped principal creates inside its own tenant, naming no owner at all.' 'OMIT' '[]')"
  assert_status "RL3 positive · it creates in its OWN tenant, omitting tenantID entirely" 201
  assert_jq "RL3 positive · and absent really did mean \"mine\"" '.data.tenantID' "$SP_TENANT_ID"

  req_astoken "$SP_TOKEN" POST /roles "$(role_body "$(_slug "${RSCOPE}-frn")" 'Qa Foreign Tenant' \
    'A role the scoped principal must not be able to create inside somebody else.' "$MASTER_TENANT_ID" '[]')"
  assert_status_key_field "RL3 negative · naming ANOTHER tenant is refused" 403 'TenantMismatchNotification' 'tenantID'

  # The archive verb carries the same rule: refuseForeignTenant runs under
  # IfArchive too, and the WRITE side is not filtered by ToCriteria — so the row
  # loads and the RULE is what refuses.
  RL3_FOREIGN="$(create_role "$(_slug "${RSCOPE}-fa")" "$T_D" '[]')"
  req_astoken "$SP_TOKEN" PATCH "/roles/${RL3_FOREIGN}/archive" ''
  assert_status_key "RL3 negative · archiving another tenant's role is refused" 403 'TenantMismatchNotification'

  # ══════════════════════════════════════════════════════════════════════════
  section "RL3b 🔴 CRITICAL · and it binds READS — as a 404, never a 403"
  note "spec.md §10 Layer 3: \"a by-id read of another tenant's role returns 404"
  note "rather than 403 — it does not exist for this caller, which leaks nothing"
  note "about who else exists\". A 403 here would confirm the row exists."

  req_astoken "$SP_TOKEN" GET "/roles/${RL2_ROLE}"
  assert_status "RL3b positive · the principal reads a role of its own tenant" 200

  req_astoken "$SP_TOKEN" GET "/roles/${MASTER_ROLE_ID}"
  assert_status_key "RL3b negative · the master role answers NOT-FOUND, not FORBIDDEN" 404 'RecordNotFoundNotification'

  req_astoken "$SP_TOKEN" GET '/roles?first=100'
  assert_jq_true "RL3b negative · and it is absent from every page of the listing" \
    "[.data[].id] | index(\"${MASTER_ROLE_ID}\") == null" \
    'the master role is not in the scoped principal listing'
  assert_jq_true "RL3b negative · every row the principal CAN see is its own tenant's" \
    "[.data[].tenantID] | unique == [\"${SP_TENANT_ID}\"]" \
    'the isolation filter selected, it did not merely hide one row'

  # RL3c — the bypass, without which a service that filtered EVERYONE would pass
  # RL3b for the wrong reason and a platform operator could not support a customer.
  req GET '/roles?first=100'
  assert_jq_true "RL3c positive · the *:* admin DOES see the master role" \
    "[.data[].id] | index(\"${MASTER_ROLE_ID}\") != null" \
    'the super-admin crosses the row scope'
  assert_jq_true "RL3c positive · and sees rows from more than one tenant" \
    '([.data[].tenantID] | unique | length) > 1' \
    'the admin listing is not silently scoped either'
fi

# ════════════════════════════════════════════════════════════════════════════
section "RL4 🔴 CRITICAL · a granted permission must EXIST and still be ACTIVE"
note "spec.md §7 R6 — a retired permission comes back as a NEW row with a NEW id,"
note "so re-granting the old id is refused rather than silently honoured. This is"
note "the whole reason the grant stores the id and not the string."

RL4_ROLE="$(create_role "$(_slug "${RSCOPE}-cat")" "$T_D" '[]')"
req POST "/roles/${RL4_ROLE}/permissions" "$(jq -nc --arg p "$PID_ROLE_READ" '{permissionID:$p}')"
assert_status "RL4 positive · a live catalog id is granted" 201

req POST "/roles/${RL4_ROLE}/permissions" '{"permissionID":"00000000-0000-7000-8000-0000000004c4"}'
assert_status_key "RL4 negative · a uuid no catalog row carries" 422 'PermissionNotInCatalogNotification'

# The half that matters. A permission archived a moment ago is still a real row
# with a real id — and it must stop being grantable.
RL4_RES="$(_slug "${RSCOPE}-retired")"
RL4_PID="$(create_permission "$RL4_RES" read 'A catalog entry retired mid-run so a grant against it can be refused.')"
req PATCH "/permissions/${RL4_PID}/archive" ''
assert_status "RL4 setup · the catalog entry is retired" 204
req POST "/roles/${RL4_ROLE}/permissions" "$(jq -nc --arg p "$RL4_PID" '{permissionID:$p}')"
assert_status_key "RL4 negative · an ARCHIVED catalog id is refused too" 422 'PermissionNotInCatalogNotification'

# ════════════════════════════════════════════════════════════════════════════
section "RL5 🔴 CRITICAL · a suspended tenant gets no role — and a TRIAL one does"
note "spec.md §7 R4, corrected 2026-08-24. \"Unavailable\" is not \"not active\":"
note "a trial is a live customer being onboarded, and roles are the first thing"
note "they need. A rule written as Status != active would refuse every trial signup."

T_TRIAL="$(create_tenant "$(_slug "${RSCOPE}-trial")" 'Qa Trial Tenant' 'A tenant still inside its trial period, which is a live customer being onboarded.' 'trial')"
req POST /roles "$(role_body "$(_slug "${RSCOPE}-tr")" 'Qa Trial Role' "$RL_DESC" "$T_TRIAL" '[]')"
assert_status "RL5 positive · a TRIAL tenant may be given a role — the plausible-mistake control" 201

T_SUSP="$(create_tenant "$(_slug "${RSCOPE}-susp")" 'Qa Suspended Tenant' 'A tenant that stopped paying and must not be handed anything that grants access.' 'active')"
req PATCH "/tenants/${T_SUSP}" '{"status":"suspended"}'
assert_status "RL5 setup · the tenant is suspended" 200
req POST /roles "$(role_body "$(_slug "${RSCOPE}-su")" 'Qa Suspended Role' "$RL_DESC" "$T_SUSP" '[]')"
assert_status_key_field "RL5 negative · a SUSPENDED tenant is refused" 422 'RoleTenantDoesNotExistNotification' 'tenantID'

T_ARCH="$(create_tenant "$(_slug "${RSCOPE}-arch")" 'Qa Archived Tenant' 'A tenant archived before anyone tried to mint a role inside it.' 'active')"
req PATCH "/tenants/${T_ARCH}/archive" ''
assert_status "RL5 setup · the tenant is archived" 204
req POST /roles "$(role_body "$(_slug "${RSCOPE}-ar")" 'Qa Archived Role' "$RL_DESC" "$T_ARCH" '[]')"
assert_status_key_field "RL5 negative · an ARCHIVED tenant is refused" 422 'RoleTenantDoesNotExistNotification' 'tenantID'

req POST /roles "$(role_body "$(_slug "${RSCOPE}-nx")" 'Qa Absent Tenant' "$RL_DESC" '00000000-0000-7000-8000-0000000004c4' '[]')"
assert_status_key_field "RL5 negative · a tenant id no row carries is refused" 422 'RoleTenantDoesNotExistNotification' 'tenantID'

# ════════════════════════════════════════════════════════════════════════════
section "RL6 · the key is immutable, and the refusal is explicit"
note "spec.md §7 R1 · rules.list: key-immutable — it is what API callers and"
note "audit lines reference, so moving it rewrites the meaning of both."

RL6_KEY="$(_slug "${RSCOPE}-imm")"
RL6_ROLE="$(create_role "$RL6_KEY" "$T_D" '[]')"
req PATCH "/roles/${RL6_ROLE}" '{"name":"Qa Relabelled","description":"An amended wording that leaves the machine handle exactly where it was."}'
assert_status "RL6 positive · relabelling is allowed" 200
assert_jq "RL6 positive · and the key did not move" '.data.key' "$RL6_KEY"

req PATCH "/roles/${RL6_ROLE}" "$(jq -nc --arg k "${RL6_KEY}-x" '{key:$k}')"
assert_status_key_field "RL6 negative · moving the key is refused" 422 'RoleKeyIsImmutableNotification' 'key'
req GET "/roles/${RL6_ROLE}"
assert_jq "RL6 negative · and the stored key is untouched" '.data.key' "$RL6_KEY"

# ════════════════════════════════════════════════════════════════════════════
section "RL7 · the tenant is immutable STRUCTURALLY — the door does not exist"
note "PatchRoleRequest carries key, name and description alone, so"
note "RoleTenantIsImmutableNotification is UNREACHABLE through REST and GraphQL."
note "No case asserts it; this case asserts the structure that makes it moot."

req PATCH "/roles/${RL6_ROLE}" "$(jq -nc --arg t "$MASTER_TENANT_ID" '{tenantID:$t}')"
assert_status "RL7 · a body naming another tenant is accepted and ignored" 200
req GET "/roles/${RL6_ROLE}"
assert_jq "RL7 · and the owner is UNCHANGED — the field reached no command" '.data.tenantID' "$T_D"

# ════════════════════════════════════════════════════════════════════════════
section "RL8 · at most 200 permissions in one role"
note "spec.md §7 R8. Stated plainly: the negative sends 201 invented uuids, so 201"
note "PermissionNotInCatalogNotification keys ride along with the cap's own. The"
note "assertion reads the WHOLE envelope, never only the first message. Seeding 201"
note "real catalog rows was weighed at the gate and declined as not worth ~202 calls."

# EVERY wildcard is excluded, not merely the seeded one. no-wildcard-grant
# refuses a '*' in EITHER half, and the permission lane's own P3 positive case
# creates a <resource>:* row — so filtering by the seeded id alone let a lane
# fixture answer 403 and took this case RED for a reason that had nothing to do
# with the cap it exists to prove.
req GET '/permissions?first=100&orderBy=resource'
RL8_IDS="$(j '[.data[] | select(.permission | contains("*") | not) | .id]')"
RL8_N="$(printf '%s' "$RL8_IDS" | jq 'length')"
req POST /roles "$(role_body "$(_slug "${RSCOPE}-bulk")" 'Qa Bulk Role' "$RL_DESC" "$T_D" "$RL8_IDS")"
assert_status "RL8 positive · a role bundling ${RL8_N} live catalog rows is accepted" 201
assert_jq "RL8 positive · and every one of them was stored" '.data.permissions | length' "$RL8_N"

RL8_MANY="$(jq -nc '[range(0;201)] | map("0000ffff-0000-7000-8000-" + ("000000000000" + (. | tostring) | .[-12:]))')"
req POST /roles "$(role_body "$(_slug "${RSCOPE}-cap")" 'Qa Capped Role' "$RL_DESC" "$T_D" "$RL8_MANY")"
assert_status_key "RL8 negative · 201 entries trips the cap" 422 'TooManyPermissionsInRoleNotification'
assert_jq "RL8 negative · and the refusal names the ceiling it enforced" \
  '[.errors[]?.messages[]? | select(.notificationKey=="TooManyPermissionsInRoleNotification") | .value] | first' '201'

# ════════════════════════════════════════════════════════════════════════════
section "RL9 🔴 CRITICAL · the catalog rules judge the entries a write ADDS"
note "spec.md §7 \"What these three rules judge\" · GetAddedItemsOf, not"
note "GetCurrentItemsOf. A rule re-judging STORED entries would answer 422 on a"
note "request whose only change is a label — and a caller who lost a permission"
note "could no longer even REVOKE the others, since a revocation is an update."

RL9_RES="$(_slug "${RSCOPE}-later")"
RL9_PID="$(create_permission "$RL9_RES" read 'A catalog entry granted first and retired afterwards, to prove stored grants are not re-judged.')"
RL9_ROLE="$(create_role "$(_slug "${RSCOPE}-add")" "$T_D" "$(jq -nc --arg a "$RL9_PID" '[$a]')")"
req PATCH "/permissions/${RL9_PID}/archive" ''
assert_status "RL9 setup · the platform retires a permission the role already grants" 204

req PATCH "/roles/${RL9_ROLE}" '{"name":"Qa Renamed After Retirement"}'
assert_status "RL9 positive · renaming that role still answers 200" 200
assert_jq "RL9 positive · and the rename actually applied" '.data.name' 'Qa Renamed After Retirement'

# RL10 — a REVOKE asks nothing, because it adds nothing. Revocation is the tool
# for a grant that stopped being acceptable, so it has to stay reachable exactly
# when it is needed.
RL10_PID="$(permission_id_of user read)"
req POST "/roles/${RL9_ROLE}/permissions" "$(jq -nc --arg p "$RL10_PID" '{permissionID:$p}')"
assert_status "RL10 setup · a second, live grant is added" 201
RL10_CHILD="$(j '.data.rolePermission.id')"
req PATCH "/roles/${RL9_ROLE}/permissions/${RL10_CHILD}/archive" '{}'
assert_status "RL10 positive · revoking works while a STORED grant points at a retired permission" 204

TOKEN="$ADMIN_TOKEN"

# ════════════════════════════════════════════════════════════════════════════
section "P6 · archiving a catalog row REVOKES it from every later token"
note "authentication_reader.go:105 reads PermissionArchivedAt through the join;"
note ":350 drops those rows from the effective set."
note "IRREVERSIBLE WITHIN THIS RUN — it is the last case of the last lane by design."

# The wildcard row is the only permission the bootstrap admin's master role
# carries (migration 0012 §3), so it is the only row whose archive is observable
# on this principal. Its id is a tracked literal in that migration, not something
# read off an answer.
WILDCARD_ID='01990000-0000-7000-8000-000000000000'

sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
assert_jq_true "P6 positive · the admin's token carries the wildcard before the archive" \
  '[.data.user.permissions[]] | index("*:*") != null' \
  'the permissions claim contains *:*'
req GET /permissions
assert_status "P6 positive · and a gated route serves that principal" 200

req PATCH "/permissions/${WILDCARD_ID}/archive" ''
assert_status "P6 negative · archive the wildcard catalog row" 204

# A NEW token: the claim is computed at sign-in, so an already-issued one would
# prove nothing about revocation.
sign_in "$BOOTSTRAP_EMAIL" "$QA_ADMIN_PASSWORD"
assert_jq_true "P6 negative · the reissued token no longer carries the wildcard" \
  '[.data.user.permissions[]?] | index("*:*") == null' \
  'the permissions claim has lost *:*'
REVOKED_TOKEN="$(j '.data.accessToken')"
req_astoken "$REVOKED_TOKEN" GET /permissions
assert_status_key "P6 negative · and the gated route now refuses it" 403 'MissingPermissionNotification'


lane_summary
