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
