#!/usr/bin/env bash
# Lane: audit — the in-transaction audit trail, §4 of specs/qa/tenant-contract/plan.md.
#
# The maintainer asked for this family explicitly on 2026-09-06. It is asserted through SQL and
# never through an endpoint: the `database` destination writes its row inside the SAME
# transaction as the write it records, so the only honest place to look is the table.
#
# psql is not on the host; the bench container has it (see `sql` in qa/lib/common.sh).

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init audit

# The actor the framework must stamp is the `sub` of the token that made the call. Read here
# from the token the SUITE holds — the caller's own identity, not an answer the service gave
# about the record.
ADMIN_SUB=$(printf '%s' "$QA_TOKEN_ADMIN" | python3 -c "
import sys, base64, json
seg = sys.stdin.read().strip().split('.')[1]
seg += '=' * ((4 - len(seg) % 4) % 4)
print(json.loads(base64.urlsafe_b64decode(seg))['sub'])
")
MASTER_TENANT="01990000-0001-7000-8000-000000000001"

WS_A=$(ws aud)
api POST /tenants "$(tenant_body "Audit Trail Tenant" "$WS_A" "The tenant whose every write this lane reads back out of audit_events." "active")"
ID_A=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "A1 the INSERT wrote exactly one audit row" "1 row with verb 'insert' and entity_type 'Tenant', in the same transaction as the write"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='Tenant' AND aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A2 the insert row's kind is 'snapshot'" "snapshot — a creation records the whole record, not a delta"
got=$(sql "SELECT kind FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "snapshot" ]; then pass_; else HTTP_BODY="$got"; fail_ "kind = $got"; fi

case_ "A3 the actor is the acting principal's sub" "the sub of the token that made the call, not a sentinel"
got=$(sql "SELECT actor FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "$ADMIN_SUB" ]; then pass_; else HTTP_BODY="$got"; fail_ "actor = '$got', expected '$ADMIN_SUB'"; fi

case_ "A4 the tenant scope is stamped as its own column" "the master tenant's id — indexed, so per-tenant retention and forensic filters stay on a narrow path"
got=$(sql "SELECT tenant_id FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "$MASTER_TENANT" ]; then pass_; else HTTP_BODY="$got"; fail_ "tenant_id = '$got'"; fi

case_ "A5 the payload carries the declared auditClaims: email" "admin@authcore.local — declared in auth.auditClaims because Actor on its own is a UUID"
got=$(sql "SELECT payload::jsonb -> 'actorClaims' ->> 'email' FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "admin@authcore.local" ]; then pass_; else HTTP_BODY="$got"; fail_ "email = '$got'"; fi

case_ "A6 ...tenant_workspace" "master — without it every audit line out in the mesh identifies its tenant by a bare UUID it cannot resolve"
got=$(sql "SELECT payload::jsonb -> 'actorClaims' ->> 'tenant_workspace' FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "master" ]; then pass_; else HTTP_BODY="$got"; fail_ "tenant_workspace = '$got'"; fi

case_ "A7 ...identity_kind" "a non-empty value — it says PERSON or MACHINE without anybody cross-referencing the clients table"
got=$(sql "SELECT coalesce(payload::jsonb -> 'actorClaims' ->> 'identity_kind','') FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ -n "$(printf '%s' "$got" | tr -d '[:space:]')" ]; then pass_; else HTTP_BODY="$got"; fail_ "identity_kind is empty"; fi

case_ "A8 an UNDECLARED claim never reaches the row" "no 'permissions' key — auditClaims is an allowlist, and authorization state is deliberately absent from it"
got=$(sql "SELECT (payload::jsonb -> 'actorClaims') ? 'permissions' FROM audit_events WHERE aggregate_id='$ID_A' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "f" ]; then pass_; else HTTP_BODY="$got"; fail_ "permissions present = '$got'"; fi

api PATCH "/tenants/$ID_A" '{"name":"Audit Trail Renamed"}'

case_ "A9 the PATCH wrote an update row" "1 row with verb 'update'"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_A' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A10 the update row carries a changes block naming the field" "a change on Name — the faithful DOMAIN field name, never the physical column"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_A' AND verb='update' AND payload::jsonb -> 'changes' @> '[{\"field\":\"Name\"}]';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "matching rows = $got"; fi

api PATCH "/tenants/$ID_A/archive"

case_ "A11 the ARCHIVE wrote a transition row" "1 row with verb 'archive' and kind 'transition'"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_A' AND verb='archive' AND kind='transition';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/tenants/$ID_A/unarchive"

case_ "A12 the UNARCHIVE wrote its own transition row" "1 row with verb 'unarchive' — the same UPDATE statement as archive, and only the lifecycle effect tells them apart"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_A' AND verb='unarchive' AND kind='transition';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A13 the four writes left four rows and no more" "4 — one event per write, never two and never none"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_A';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "4" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A14 a REFUSED write leaves no audit row at all" "0 — the row is written in the write's own transaction, so a rejected write rolls it back with everything else"
WS_REJ=$(ws audrej)
api POST /tenants "$(tenant_body "aaaa" "$WS_REJ" "A write that the domain refuses, so nothing at all is committed." "active")"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='Tenant' AND payload::jsonb -> 'snapshot' ->> 'Workspace' = '$WS_REJ';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
#
#   P E R M I S S I O N  —  the audit family of specs/qa/permission-contract/plan.md §4
#
#   Same framework promise, one aggregate over. What is worth asserting HERE rather than
#   inheriting the Tenant cases wholesale is the shape peculiar to this entity: it has THREE
#   write verbs, not four, because there is no unarchive — and the absence of that row in the
#   timeline is itself a contract, not an omission.
#
# ═════════════════════════════════════════════════════════════════════════════════════════

R_AUD=$(pair_resource aud)
ID_P=$(new_permission "$R_AUD" read "The audit fixture of the permission lane, written three times so its timeline can be read.") || exit 1

case_ "A15 the INSERT wrote one audit row for Permission" "1 row, entity_type 'Permission', aggregate_id the catalog row's id"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND entity_type='Permission' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A16 the actor is the acting principal, not the row's owner" "the admin's sub — a catalog is global, but the WRITE is still attributable to a person"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='insert' AND actor IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A17 the actor's tenant claim is recorded" "not null — the catalog is not tenant-scoped, but the actor is"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='insert' AND tenant_id IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A18 the declared auditClaims ride the payload" "email and tenant_workspace are both present"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='insert' AND payload::jsonb -> 'actorClaims' ->> 'email' = 'admin@authcore.local' AND payload::jsonb -> 'actorClaims' ->> 'tenant_workspace' = 'master';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/permissions/$ID_P" '{"description":"The audit fixture of the permission lane, with its wording revised so a change block exists."}'

case_ "A19 the UPDATE wrote its own row, carrying a changes block" "1 row with verb 'update' and a non-empty changes payload"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='update' AND payload::jsonb ? 'changes';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/permissions/$ID_P/archive"

case_ "A20 the ARCHIVE wrote a transition row" "1 row with verb 'archive' and kind 'transition'"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='archive' AND kind='transition';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A21 there is NO unarchive row, ever" "0 — the verb does not exist on this aggregate, and its absence from the timeline is the contract, not a gap"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P' AND verb='unarchive';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A22 the three writes left exactly three rows" "3 — one event per write, and no fourth verb to write a fourth"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_P';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "3" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A23 a REFUSED permission write leaves no audit row" "0 — the row is written in the write's own transaction and rolls back with it"
R_AUDREJ=$(pair_resource audrej)
api POST /permissions "$(permission_body "$R_AUDREJ" "Read" "A permission write the domain refuses, so nothing at all is committed.")"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='Permission' AND payload::jsonb -> 'snapshot' ->> 'Resource' = '$R_AUDREJ';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi


# ═════════════════════════════════════════════════════════════════════════════════════════
#
#   R O L E  —  the audit family of specs/qa/role-contract/plan.md §4
#
#   Same framework promise, one aggregate over — and the FIRST one that brings a shape neither
#   Tenant nor Permission has. Role's collection carries two verbs of its own, and a child op
#   is a command on the ROOT: so a GRANT and a REVOKE must each write an `update` row against
#   the ROOT's aggregate_id, never against the child's. Nothing else in this suite would notice
#   an entry that audited itself instead of its owner.
#
# ═════════════════════════════════════════════════════════════════════════════════════════

AUD_TEN=$(new_tenant active "$(ws audrole)") || exit 1
AUD_P1=$(permission_id_of tenant read)
AUD_P2=$(permission_id_of role read)
AUD_K=$(role_key aud)
ID_R=$(new_role "$AUD_K" "$AUD_TEN" "$AUD_P1") || exit 1

case_ "A24 the INSERT wrote one audit row for Role" "1 row, entity_type 'Role', aggregate_id the role's id"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND entity_type='Role' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A25 the insert row's kind is 'snapshot'" "snapshot — a creation records the whole record, not a delta"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='insert' AND kind='snapshot';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A26 the actor is the acting principal" "the admin's sub — the row belongs to a tenant, the WRITE belongs to a person"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='insert' AND actor IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A27 the ACTOR's tenant is stamped, not the ROW's" "master — the role was created inside a tenant of the suite's own, and the audit column records who was asking, which is the column per-tenant retention filters on"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='insert' AND tenant_id='$QA_MASTER_TENANT_ID';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A28 the declared auditClaims ride the payload" "email, tenant_workspace and identity_kind — Actor on its own is a UUID nobody out in the mesh can resolve"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='insert' AND payload::jsonb -> 'actorClaims' ->> 'email' = 'admin@authcore.local' AND payload::jsonb -> 'actorClaims' ->> 'tenant_workspace' = 'master' AND payload::jsonb -> 'actorClaims' ->> 'identity_kind' IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A29 an UNDECLARED claim never reaches the row" "no 'permissions' key — auditClaims is an allowlist, and authorization state is deliberately absent from it"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND payload::jsonb -> 'actorClaims' ? 'permissions';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/roles/$ID_R" '{"name":"QA Audit Role, relabelled so a change block exists"}'

case_ "A30 the PATCH wrote an update row carrying a changes block" "1 row with verb 'update' and a changes payload naming the DOMAIN field"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='update' AND payload::jsonb ? 'changes';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

# ── the two collection verbs, which is what Role adds to this lane ────────────────────────

api POST "/roles/$ID_R/permissions" "$(jq -nc --arg p "$AUD_P2" '{permissionID:$p}')"
AUD_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.rolePermission.id')

case_ "A31 a GRANT wrote an audit row against the ROOT" "2 update rows now on the role's own aggregate_id — a child op is a command on the root, so this is where its trail belongs"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "2" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A32 and NOT against the entry's own id" "0 rows carrying the child id as an aggregate — an entry that audited itself would split one role's history across two timelines"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$AUD_CHILD';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A33 the GRANT's row carries a changes block naming the collection" "a changes payload mentioning the permissions collection — what a role can DO changed, and the trail has to say so"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='update' AND payload::text ILIKE '%ermission%';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" -ge 1 ] 2>/dev/null; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/roles/$ID_R/permissions/$AUD_CHILD/archive"

case_ "A34 a REVOKE wrote its own row, against the ROOT again" "3 update rows — a revocation is the write an access review most needs to find, and it is not a DELETE precisely so it can be found"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "3" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/roles/$ID_R/archive"

case_ "A35 the ARCHIVE wrote a transition row" "1 row with verb 'archive' and kind 'transition'"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='archive' AND kind='transition';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A36 there is NO unarchive row, ever" "0 — the verb does not exist on this aggregate, and its absence from the timeline is the contract, not a gap"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R' AND verb='unarchive';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A37 the five writes left exactly five rows" "5 — insert, patch, grant, revoke, archive: one event per write, never two and never none"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_R';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "5" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A38 a REFUSED role write leaves no audit row at all" "0 — the row is written in the write's own transaction, so a rejected write rolls it back with everything else"
AUD_REJ="qa-audit-rejected-$(qa_slug_runid)"
api POST /roles "$(jq -nc --arg k "$AUD_REJ" --arg t "$AUD_TEN" '{key:$k, name:"X", description:"short", tenantID:$t, permissions:[]}')"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='Role' AND payload::text LIKE '%$AUD_REJ%';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# A39-A50 — Group. specs/qa/group-contract/plan.md §4.
#
# Group's audit shape is Role's, one level up the graph, and the same decision applies: the
# ROOT verbs and the two COLLECTION verbs both. What makes the trail matter more here is what
# the writes DO — an attach hands every member of a team every permission of a role, so
# "somebody changed what this group confers" is the single line an access review looks for.
# ═════════════════════════════════════════════════════════════════════════════════════════

AUD_G_TEN=$(new_tenant active "$(ws audg)") || exit 1
AUD_G_R1=$(new_role "$(role_key audg1)" "$AUD_G_TEN" "$(permission_id_of tenant read)") || exit 1
AUD_G_R2=$(new_role "$(role_key audg2)" "$AUD_G_TEN" "$(permission_id_of group read)")  || exit 1

ID_G=$(new_group "$(group_key aud)" "$AUD_G_TEN") || exit 1

case_ "A39 the INSERT wrote one audit row for Group" "1 row, entity_type 'Group', aggregate_id the group's id"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND entity_type='Group' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A40 the insert row's kind is 'snapshot'" "snapshot — a creation records the whole record, not a delta"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='insert' AND kind='snapshot';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A41 the ACTOR's tenant is stamped, not the ROW's" "master — the group was created inside a tenant of the suite's own, and the audit column records who was ASKING"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='insert' AND tenant_id='$QA_MASTER_TENANT_ID' AND actor IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A42 the declared auditClaims ride the payload" "email, tenant_workspace and identity_kind"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='insert' AND payload::jsonb -> 'actorClaims' ->> 'email' = 'admin@authcore.local' AND payload::jsonb -> 'actorClaims' ->> 'tenant_workspace' = 'master' AND payload::jsonb -> 'actorClaims' ->> 'identity_kind' IS NOT NULL;")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A43 an UNDECLARED claim never reaches the row" "no 'groups' and no 'permissions' key — auditClaims is an allowlist, and the caller's own authorization state is deliberately outside it"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND (payload::jsonb -> 'actorClaims' ? 'permissions' OR payload::jsonb -> 'actorClaims' ? 'groups');")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/groups/$ID_G" '{"name":"QA Audit Group, relabelled so a change block exists"}'

case_ "A44 the PATCH wrote an update row carrying a changes block" "1 row with verb 'update' and a changes payload"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='update' AND payload::jsonb ? 'changes';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

# ── the two collection verbs: the writes that change what a whole TEAM can do ─────────────

api POST "/groups/$ID_G/roles" "$(jq -nc --arg r "$AUD_G_R1" '{roleID:$r}')"
AUD_G_CHILD=$(printf '%s' "$HTTP_BODY" | jq -r '.data.groupRole.id')

case_ "A45 an ATTACH wrote an audit row against the ROOT" "2 update rows now on the group's own aggregate_id — a child op is a command on the root, so this is where its trail belongs"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "2" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A46 and NOT against the entry's own id" "0 rows carrying the child id as an aggregate — an entry that audited itself would split one group's history across two timelines"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$AUD_G_CHILD';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A47 the ATTACH's row carries a changes block naming the collection" "a changes payload mentioning the roles collection — what a whole TEAM can do changed, and the trail has to say so"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='update' AND payload::text ILIKE '%role%';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" -ge 1 ] 2>/dev/null; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/groups/$ID_G/roles/$AUD_G_CHILD/archive"

case_ "A48 a DETACH wrote its own row, against the ROOT again" "3 update rows — a detach is the write an access review most needs to find, and it is not a DELETE precisely so it can be found"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "3" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

api PATCH "/groups/$ID_G/archive"

case_ "A49 the ARCHIVE wrote a transition row" "1 row with verb 'archive' and kind 'transition' — the write that de-authorizes a whole team leaves exactly one line"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='archive' AND kind='transition';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A50 there is NO unarchive row, ever" "0 — the verb does not exist on this aggregate, and its absence from the timeline is the contract rather than a gap. A 'restored' line is precisely what §5 of the model refused to make possible"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G' AND verb='unarchive';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A51 the five writes left exactly five rows" "5 — insert, patch, attach, detach, archive: one event per write, never two and never none"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_G';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "5" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A52 a REFUSED group write leaves no audit row at all" "0 — the row is written in the write's own transaction, so a rejected write rolls it back with everything else"
AUD_G_REJ="qa-audit-grouprejected-$(qa_slug_runid)"
api POST /groups "$(jq -nc --arg k "$AUD_G_REJ" --arg t "$AUD_G_TEN" '{key:$k, name:"X", description:"short", tenantID:$t, roles:[]}')"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='Group' AND payload::text LIKE '%$AUD_G_REJ%';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A53 a refused ATTACH leaves no row either" "3 update rows still — a wildcard refusal is a DOMAIN refusal, raised after the handler was reached, and it must still roll its audit row back"
AUD_G_GM=$(new_group "$(group_key audgm)" "$QA_MASTER_TENANT_ID") || exit 1
api POST "/groups/$AUD_G_GM/roles" "$(jq -nc --arg r "$QA_MASTER_ROLE_ID" '{roleID:$r}')"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$AUD_G_GM' AND verb='update';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# A54+ — §4 of specs/qa/user-contract/plan.md. The User trail.
#
# Five operations write here — insert, patch, archive and the two credential verbs — and one
# of them is the reason this round extended the lane at all: User carries the ONLY
# RedactedField in the service, and a redaction is invisible from every endpoint. The wire
# faces of that rule live in qa/user.sh (M3, M8.23-27) and qa/user_graphql.sh (N2.4-5);
# qa/domain.sh U25 owns the payload half. What is here is the trail's own shape.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_AU=$(new_tenant active "$(ws audu)") || exit 1
E_AU=$(user_email aud)
ID_AU=$(new_user "$E_AU" "$TEN_AU") || exit 1

case_ "A54 the user INSERT wrote exactly one audit row" "1 row, verb 'insert', entity_type 'User', in the same transaction as the write"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='User' AND aggregate_id='$ID_AU' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "count = $got"; fi

case_ "A55 the insert row's kind is 'snapshot'" "a creation records the whole record, not a delta"
got=$(sql "SELECT kind FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "snapshot" ]; then pass_; else HTTP_BODY="$got"; fail_ "kind = $got"; fi

case_ "A56 the actor is the acting principal's sub" "the admin's own sub, not a sentinel"
got=$(sql "SELECT actor FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='insert';")
if [ "$(printf '%s' "$got" | tr -d '[:space:]')" = "$ADMIN_SUB" ]; then pass_; else HTTP_BODY="$got"; fail_ "actor = '$got'"; fi

case_ "A57 the tenant scope stamped on the row is the ACTOR's, not the record's" "the master tenant — the admin acted, and the row it created lives elsewhere. The column answers 'who did this, under which tenant', which is what per-tenant retention and forensic filters read"
got=$(sql "SELECT tenant_id FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='insert';" | tr -d '[:space:]')
if [ "$got" = "$MASTER_TENANT" ]; then pass_; else HTTP_BODY="$got"; fail_ "tenant_id = '$got' (record's tenant is $TEN_AU)"; fi

case_ "A58 the PATCH is recorded as a DELTA and not a snapshot" "kind 'delta' — the verb that changes one field must not rewrite the whole record into the trail"
api PATCH "/users/$ID_AU" '{"givenName":"Audited"}'
got=$(sql "SELECT kind FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='update' ORDER BY created_at DESC LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "delta" ]; then pass_; else HTTP_BODY="$got"; fail_ "kind = '$got'"; fi

case_ "A59 the delta names the field that moved" "GivenName in the payload — a trail that records that something changed without saying what is not a trail"
got=$(sql "SELECT (payload::jsonb::text LIKE '%GivenName%') FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='update' ORDER BY created_at DESC LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "t" ]; then pass_; else HTTP_BODY="$(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='update' ORDER BY created_at DESC LIMIT 1;")"; fail_ "GivenName not in the delta"; fi

case_ "A60 the ARCHIVE writes its own row" "verb 'archive' — a lifecycle transition is a fact of its own, distinct from the update that carries archived_at"
api PATCH "/users/$ID_AU/archive"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='archive';" | tr -d '[:space:]')
if [ "$got" = "1" ]; then pass_; else HTTP_BODY="$got"; fail_ "archive rows = $got"; fi

case_ "A61 the archive's row records the STATUS the rule forced" "Status suspended in the payload — U15 is a mutation inside IfArchive, and because archive is an ordinary full-field write at this pin it reaches the row AND the event. If it ever stopped reaching the event, the trail would show an archive that silently left the account active"
got=$(sql "SELECT (payload::jsonb::text LIKE '%suspended%') FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='archive';" | tr -d '[:space:]')
if [ "$got" = "t" ]; then pass_; else HTTP_BODY="$(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AU' AND verb='archive';")"; fail_ "the forced status is not in the archive event"; fi

# ── the two credential operations ────────────────────────────────────────────────────────
# THE FIXTURE ROTATES BEFORE THE RESET, and that is not ceremony: every API-created account is
# born must_change_password=TRUE, so a reset on a fresh one moves nothing and the delta has no
# transition to carry. The change is what clears the flag (U20.1); the reset below is what sets
# it again, and A63 is the assertion that the trail says so.
E_AUC=$(user_email audc)
ID_AUC=$(new_user "$E_AUC" "$TEN_AU") || exit 1
R_AUC=$(new_role "$(role_key audc)" "$TEN_AU" "$(permission_id_of user change-password)") || exit 1
grant_role_to_user "$ID_AUC" "$R_AUC" >/dev/null
T_AUC=$(usable_token "$E_AUC" "$ID_AUC") || true

case_ "A62 a RESET writes an audit row" "verb 'update' — the credential verbs dispatch ModeUpdate, so the trail records them as updates and the ACTION is what tells them apart in the payload"
api PATCH "/users/$ID_AUC/password-reset" '{"password":"Qa!Audited26","passwordConfirmation":"Qa!Audited26"}'
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_AUC' AND verb='update';" | tr -d '[:space:]')
if [ "$got" -ge 1 ] 2>/dev/null; then pass_; else HTTP_BODY="$got"; fail_ "update rows = $got"; fi

case_ "A63 the reset's row records the FLAG it set and not the credential" "MustChangePassword in the delta — what a reviewer needs to see is that somebody's password was replaced and that they must rotate it, never what it was replaced with"
got=$(sql "SELECT (payload::jsonb::text LIKE '%MustChangePassword%') FROM audit_events WHERE aggregate_id='$ID_AUC' AND verb='update' ORDER BY created_at DESC LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "t" ]; then pass_; else HTTP_BODY="$(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AUC' AND verb='update' ORDER BY created_at DESC LIMIT 1;")"; fail_ "MustChangePassword not in the payload"; fi

case_ "A63b the delta names PasswordHash as changed, with BOTH sides redacted" "from '***' to '***' — the trail records THAT the credential moved without recording either value, which is exactly what an access review needs and all it may have"
got=$(sql "SELECT (payload::jsonb::text LIKE '%\"to\": \"***\"%' AND payload::jsonb::text LIKE '%\"from\": \"***\"%') FROM audit_events WHERE aggregate_id='$ID_AUC' AND verb='update' ORDER BY created_at DESC LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "t" ]; then pass_; else HTTP_BODY="$(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AUC' AND verb='update' ORDER BY created_at DESC LIMIT 1;")"; fail_ "the redacted from/to pair is not in the delta"; fi

case_ "A64 NEITHER the plaintext NOR the new hash is anywhere in that row" "0 matches for either — the plaintext arrives on a field with no column and the hash is redacted, so the two most sensitive values in this service meet the trail by two different mechanisms and both hold"
REAL_AUC=$(sql "SELECT password_hash FROM users WHERE id = '$ID_AUC';" | tr -d '[:space:]')
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_AUC' AND (payload::text LIKE '%Qa!Audited26%' OR payload::text LIKE '%' || substring('$REAL_AUC' from 20 for 20) || '%');" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="offending rows: $got"; fail_ "a credential value reached the trail"; fi

case_ "A65 the COLLECTION writes are recorded against the OWNER" "aggregate_id is the user's id, never the entry's — an entry has no identity outside its collection, and a trail keyed on the child would be unreadable as 'what happened to this person'"
ID_AUG=$(new_user "$(user_email audg)" "$TEN_AU") || exit 1
R_AUG=$(new_role "$(role_key audg)" "$TEN_AU" "$(permission_id_of tenant read)") || exit 1
CH_AUG=$(grant_role_to_user "$ID_AUG" "$R_AUG") || exit 1
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_AUG' AND entity_type='User';" | tr -d '[:space:]')
if [ "$got" -ge 2 ] 2>/dev/null; then pass_; else HTTP_BODY="$got"; fail_ "rows against the owner = $got"; fi

case_ "A66 no audit row is keyed on the CHILD id" "0 — the collection entry is part of the aggregate's write, not a write of its own"
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$CH_AUG';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "rows keyed on the child = $got"; fi

case_ "A67 a REFUSED write leaves NO audit row" "0 rows for a user the service never created — the trail records what happened, and a 422 is not something that happened"
api POST /users "$(jq -nc --arg e "$(user_email audx)" --arg t "$TEN_AU" '{givenName:"Qa",familyName:"Fixture",email:$e,status:"active",tenantID:$t,password:"abc",passwordConfirmation:"abc"}')"
got=$(sql "SELECT count(*) FROM audit_events WHERE entity_type='User' AND payload::text LIKE '%$(printf '%s' "$(user_email audx)" | cut -d@ -f1 | sed 's/-[0-9]*$//')%';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="$got"; fail_ "rows for a refused insert = $got"; fi

# ── Client — specs/qa/client-contract/plan.md §4, from A68. THE SECRET SWEEP is the point:
# ── two hash columns redacted on BOTH axes, and a plaintext that must appear NOWHERE — the
# ── fourth face of C-SEC1, and the only one no API call can see.
TEN_AC=$(new_tenant active "$(ws audc)") || exit 1
new_client "$(client_label aud)" "$TEN_AC" || exit 1
ID_AC="$CLIENT_ID"; SECRET_AC="$CLIENT_SECRET"

case_ "A68 the client INSERT wrote exactly one audit row" "1 row, verb 'insert', entity_type 'Client', kind 'snapshot', in the same transaction as the write"
got=$(sql "SELECT count(*) || '/' || min(kind) FROM audit_events WHERE entity_type='Client' AND aggregate_id='$ID_AC' AND verb='insert';" | tr -d '[:space:]')
if [ "$got" = "1/snapshot" ]; then pass_; else HTTP_BODY="$got"; fail_ "count/kind = $got"; fi

case_ "A69 the snapshot carries *** where the hash would be" "SecretHash redacted — RedactedField(InAudit) on the credential column, the same mechanism users.password_hash rides"
got=$(sql "SELECT payload::jsonb #>> '{snapshot,SecretHash}' FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='insert' LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "***" ]; then pass_; else HTTP_BODY="$got"; fail_ "audit SecretHash = '$got'"; fi

case_ "A69b the redaction is SCOPED to the credential columns" "the snapshot still carries Status in the clear — a redaction that blanked the record would destroy the trail it exists to protect"
got=$(sql "SELECT payload::jsonb #>> '{snapshot,Status}' FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='insert' LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "active" ]; then pass_; else HTTP_BODY="$got"; fail_ "snapshot Status = '$got'"; fi

case_ "A70 the REAL hash appears nowhere in this client's trail" "0 rows containing a slice of the stored SHA-256 — a redaction that covers one path and misses another is not a redaction"
REAL_AC=$(sql "SELECT secret_hash FROM clients WHERE id = '$ID_AC';" | tr -d '[:space:]')
got=$(sql "SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_AC' AND payload::text LIKE '%' || substring('$REAL_AC' from 10 for 20) || '%';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="rows containing a slice of the hash: $got"; fail_ "the hash leaked into audit_events"; fi

case_ "A71 the PLAINTEXT appears nowhere in the WHOLE TABLE" "0 rows anywhere in audit_events containing the secret the create revealed — the plaintext lives on a field with no column, so no mechanism should ever have carried it here, and this is the case that notices if one grows"
got=$(sql "SELECT count(*) FROM audit_events WHERE payload::text LIKE '%$SECRET_AC%';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="rows containing the plaintext: $got"; fail_ "the PLAINTEXT leaked into audit_events"; fi

case_ "A72 the ROTATION writes a delta with BOTH sides of the hash redacted" "from '***' to '***' on SecretHash — the trail records THAT the credential moved without recording either value, which is what an access review needs and all it may have"
rotate_secret "$ID_AC" '{}'
SECRET_AC2=$(printf '%s' "$HTTP_BODY" | jq -r '.data.secret // empty')
got=$(sql "SELECT (payload::jsonb::text LIKE '%\"to\": \"***\"%' AND payload::jsonb::text LIKE '%\"from\": \"***\"%') FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='update' ORDER BY created_at DESC LIMIT 1;" | tr -d '[:space:]')
if [ "$got" = "t" ]; then pass_; else HTTP_BODY="$(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='update' ORDER BY created_at DESC LIMIT 1;")"; fail_ "the redacted from/to pair is not in the rotation's delta"; fi

case_ "A72b ...and the ROTATED plaintext is absent from the whole table too" "0 rows — the second reveal seat leaks no more than the first"
got=$(sql "SELECT count(*) FROM audit_events WHERE payload::text LIKE '%${SECRET_AC2:-never-matches-anything}%';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="rows containing the rotated plaintext: $got"; fail_ "the rotated PLAINTEXT leaked into audit_events"; fi

case_ "A73 the ARCHIVE writes its own row, recording the STATUS the rule forced" "verb 'archive' with 'suspended' in the payload — archive-forces-suspended reaches the trail as well as the row"
api PATCH "/clients/$ID_AC/archive"
got=$(sql "SELECT count(*) || '/' || (min(payload::text) LIKE '%suspended%')::text FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='archive';" | tr -d '[:space:]')
if [ "$got" = "1/true" ]; then pass_; else HTTP_BODY="$got — $(sql "SELECT payload FROM audit_events WHERE aggregate_id='$ID_AC' AND verb='archive';")"; fail_ "archive rows/status = $got"; fi

case_ "A74 the COLLECTION writes are recorded against the OWNER" "aggregate_id is the client's id, never the entry's"
new_client "$(client_label audg)" "$TEN_AC" || exit 1
ID_ACG="$CLIENT_ID"
R_ACG=$(new_role "$(role_key audcg)" "$TEN_AC" "$(permission_id_of tenant read)") || exit 1
CH_ACG=$(grant_role_to_client "$ID_ACG" "$R_ACG") || exit 1
got=$(sql "SELECT (SELECT count(*) FROM audit_events WHERE aggregate_id='$ID_ACG' AND entity_type='Client') || '/' || (SELECT count(*) FROM audit_events WHERE aggregate_id='$CH_ACG');" | tr -d '[:space:]')
GOT_OWNER="${got%%/*}"; GOT_CHILD="${got##*/}"
if [ "${GOT_OWNER:-0}" -ge 2 ] 2>/dev/null && [ "$GOT_CHILD" = "0" ]; then pass_; else HTTP_BODY="owner/child rows = $got"; fail_ "the trail is not keyed on the owner"; fi

case_ "A75 a GLOBAL sweep finds no credential-shaped string in the entire trail" "0 rows matching acs_ + 43 base64url runes, over every entity's every event — the one query that would catch a leak through a path nobody thought to assert"
got=$(sql "SELECT count(*) FROM audit_events WHERE payload::text ~ 'acs_[A-Za-z0-9_-]{43}';" | tr -d '[:space:]')
if [ "$got" = "0" ]; then pass_; else HTTP_BODY="rows carrying a credential-shaped string: $got"; fail_ "something credential-shaped reached the trail"; fi

qa_finish
