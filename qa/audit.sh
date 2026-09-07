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

qa_finish
