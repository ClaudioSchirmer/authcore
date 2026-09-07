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

qa_finish
