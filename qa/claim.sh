#!/usr/bin/env bash
# Lane: claim — the REST half of the framework contract for the Claim aggregate.
#
# Families W1-W13 of specs/qa/claim-contract/plan.md §1. The X family (GraphQL) lives in
# qa/claim_graphql.sh; the business rules live in qa/domain.sh (CL*); the gate lives in
# qa/security.sh (S9.x); the audit trail lives in qa/audit.sh (A76+).
#
# THE THREE IDEAS THIS LANE IS BUILT AROUND.
#
# 1. THE THREE IMMUTABLE FIELDS HAVE NO DOOR. patchExcludes removes `name` and `valueType`
#    from the PATCH body, and `assignedFrom: identity-claim` keeps `tenantID` out of every
#    update body — so R2/R3/R4 cannot fire through any mounted surface. W13 asserts what the
#    wire ACTUALLY promises (200 and unchanged) and prints the three notifications as
#    SKIPPED with that reason. Claiming them would be claiming coverage this suite does not
#    have.
#
# 2. THE JOIN HAS THREE FIELDS, AND THEY ARE NOT INTERCHANGEABLE. tenantWorkspace is
#    filterable, sortable and projectable; tenantStatus is filterable and projectable but
#    NOT sortable; tenantArchivedAt is served and projectable and addressable in NO
#    criteria at all. W7.9 and W8.11 are the pair that pins the third one — the field no
#    earlier spec sentence names (plan §0c-d).
#
# 3. THERE IS NO UNARCHIVE AND NO DELETE, AND BOTH ABSENCES ANSWER DIFFERENTLY. An
#    unmounted PATH is a 404; a mounted path under an unmounted METHOD is a 405. W4 asserts
#    the split rather than accepting either.
#
# The lane runs as the bootstrap admin (*:*), so nothing here is blocked by row scope.
# §1b and §3 are where the scoped principals do the work.

cd "$(dirname "$0")/.." || exit 1
. qa/lib/common.sh
qa_init claim

# ═════════════════════════════════════════════════════════════════════════════════════════
# Fixtures. Two tenants of this lane's own, so the read-vocabulary counts below are exact
# and no other lane's rows can drift into them.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_LANE=$(new_tenant active "$(ws clm)")  || exit 1
TEN_OTHER=$(new_tenant active "$(ws clmb)") || exit 1
WS_LANE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')
api GET "/tenants/$TEN_LANE"
WS_LANE=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')

# ═════════════════════════════════════════════════════════════════════════════════════════
# W1 — happy path, one per served verb (5 routes)
# ═════════════════════════════════════════════════════════════════════════════════════════

NAME_W1=$(claim_name w1)
case_ "W1.1 POST /claims" "201 — the record AS STORED, not an echo of the request"
api POST /claims "$(jq -nc --arg n "$NAME_W1" --arg t "$TEN_LANE" \
  '{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, defaultValue:"sao-paulo",
    description:"The region a principal of this tenant is billed against, as the ERP knows it."}')"
assert_json_at 201 '.data.name' "$NAME_W1"
ID_W1=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "W1.2 PATCH /claims/{id}" "200 — the three editable fields are appliesTo, defaultValue and description"
api PATCH "/claims/$ID_W1" '{"description":"The region a principal of this tenant is billed against, corrected by the operator."}'
assert_json_at 200 '.data.description' "The region a principal of this tenant is billed against, corrected by the operator."

case_ "W1.3 GET /claims" "200 — data + pagination"
api GET "/claims?tenantID.eq=$TEN_LANE"
assert_json_at 200 '(.data | type) + "/" + ((.pagination | type))' "array/object"

case_ "W1.4 GET /claims/{id}" "200 — the full document"
api GET "/claims/$ID_W1"
assert_json_at 200 '.data.id' "$ID_W1"

case_ "W1.5 PATCH /claims/{id}/archive" "204 and NO BODY — a bodyless result, never 200 with an envelope"
NAME_W1B=$(claim_name w1b)
ID_W1B=$(new_claim "$NAME_W1B" string both "$TEN_LANE") || exit 1
api PATCH "/claims/$ID_W1B/archive"
assert_empty_body 204

# ═════════════════════════════════════════════════════════════════════════════════════════
# W2 — the golden record: EVERY declared field, written then read back one by one
#
# Thirteen wire names. No composite value object exists on this entity, so each is a field
# of its own. This is the family that catches a column silently dropped from a projection,
# and the third joined field is why it earns its place here (plan §0c-d).
# ═════════════════════════════════════════════════════════════════════════════════════════

NAME_GOLD=$(claim_name gold)
DESC_GOLD="Internal cost center this account is billed against, as the ERP knows it."
ID_GOLD=$(new_claim "$NAME_GOLD" number user "$TEN_LANE" "1000") || exit 1
api PATCH "/claims/$ID_GOLD" "$(jq -nc --arg d "$DESC_GOLD" '{description:$d}')"

api GET "/claims/$ID_GOLD"
case_ "W2.1 the golden record's seven stored columns come back exactly as written" "id, tenantID, name, valueType, appliesTo, defaultValue, description"
GOT=$(printf '%s' "$HTTP_BODY" | jq -r '[.data.id, .data.tenantID, .data.name, .data.valueType, .data.appliesTo, .data.defaultValue, .data.description] | join("|")')
WANT="$ID_GOLD|$TEN_LANE|$NAME_GOLD|number|user|1000|$DESC_GOLD"
if [ "$GOT" = "$WANT" ]; then pass_; else fail_ "$GOT"; fi

case_ "W2.2 the three managed stamps are served" "createdAt and updatedAt present, archivedAt null on a live row"
assert_json '[(.data.createdAt|type), (.data.updatedAt|type), (.data.archivedAt|type)] | join(",")' "string,string,null"

case_ "W2.3 the read join fills all THREE fields from the counterpart" "tenantWorkspace, tenantStatus, and tenantArchivedAt — the third is the one no earlier spec sentence names"
assert_json '[(.data.tenantWorkspace), (.data.tenantStatus), (.data.tenantArchivedAt|type)] | join(",")' "$WS_LANE,active,null"

case_ "W2.4 the same names come back from the LISTING, not only from by-id" "one row, every non-null key present. Addressed by name, because `id` is NOT a filter this entity declares — see W8.1b"
api GET "/claims?name.eq=$NAME_GOLD"
assert_json_at 200 '[.data[0] | has("id"), has("tenantID"), has("name"), has("valueType"), has("appliesTo"), has("defaultValue"), has("description"), has("createdAt"), has("updatedAt"), has("tenantWorkspace"), has("tenantStatus")] | all' "true"

case_ "W2.4b the two nullable columns are ABSENT rather than null on a live row" "archivedAt and tenantArchivedAt omitted — every listing field is omitempty, so absence is how this DTO spells null (W7.9 shows the same field PRESENT once the value exists)"
assert_json '[.data[0] | has("archivedAt"), has("tenantArchivedAt")] | any' "false"

# ═════════════════════════════════════════════════════════════════════════════════════════
# W3 — the write/read asymmetry
#
# The join is a READ contract. A write response that carried it would be echoing a value
# this aggregate never owns, and the managed stamps are equally absent from both write DTOs.
# ═════════════════════════════════════════════════════════════════════════════════════════

NAME_W3=$(claim_name w3)
api POST /claims "$(jq -nc --arg n "$NAME_W3" --arg t "$TEN_LANE" \
  '{name:$n, valueType:"bool", appliesTo:"client", tenantID:$t,
    description:"Whether this machine client may reach the settlement endpoints at all."}')"
ID_W3=$(printf '%s' "$HTTP_BODY" | jq -r '.data.id')

case_ "W3.1 the INSERT response carries NOTHING traversed" "no tenantWorkspace, no tenantStatus, no tenantArchivedAt — the join is a read contract"
assert_absent '.data | has("tenantWorkspace") or has("tenantStatus") or has("tenantArchivedAt")'

case_ "W3.2 the INSERT response carries no managed stamp either" "createdAt, updatedAt and archivedAt are absent from InsertClaimResponse"
assert_absent '.data | has("createdAt") or has("updatedAt") or has("archivedAt")'

api PATCH "/claims/$ID_W3" '{"description":"Whether this machine client may reach the settlement endpoints, restated."}'
case_ "W3.3 the PATCH response makes the same two absences" "the write side never speaks the join or the stamps"
assert_absent '.data | has("tenantWorkspace") or has("tenantStatus") or has("tenantArchivedAt") or has("createdAt")'

case_ "W3.4 ...and a READ of the same row carries all of them" "the asymmetry is the contract, not a gap"
api GET "/claims/$ID_W3"
assert_json '[.data | has("tenantWorkspace"), has("tenantStatus"), has("createdAt"), has("archivedAt")] | all' "true"

# ═════════════════════════════════════════════════════════════════════════════════════════
# W4 — absent verbs, split three ways
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "W4.1 PATCH /claims/{id}/unarchive" "404 — the PATH is not mounted at all; a retired definition comes back as a NEW row (spec.md §5)"
api PATCH "/claims/$ID_W1B/unarchive"
assert_status 404

case_ "W4.2 DELETE /claims/{id}" "405 — the path IS registered, under GET and PATCH only. Verb truth: nothing soft rides behind DELETE"
api DELETE "/claims/$ID_W1"
assert_status 405

case_ "W4.3 PUT /claims/{id}" "405 — update.shape is patch; the full-replacement verb is not mounted"
api PUT "/claims/$ID_W1" '{"description":"x"}'
assert_status 405

case_ "W4.4 the mode-missing-but-mounted 403 is N/A on this entity" "named, not asserted"
skip_ "Modes() is exactly display/insert/update/archive — the four verbs mounted — so no route can reach a mode the aggregate refuses. The 403 shape needs a mounted route whose mode is absent, and this service mounts none for Claim"

# ═════════════════════════════════════════════════════════════════════════════════════════
# W5 / W6 — not found, and the by-id address that is not a uuid (pin >= v0.70.0)
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "W5.1 a well-formed uuid nobody owns" "404"
api GET "/claims/01990000-dead-7000-8000-000000000000"
assert_status 404

case_ "W6.1 a READ addressed by a non-uuid" "404 UnknownIDAddressNotification — the address is unknown, not malformed, on the read side"
api GET "/claims/not-a-uuid"
assert_rest 404 UnknownIDAddressNotification

case_ "W6.2 a WRITE addressed by a non-uuid" "400 MalformedIDNotification — the split by VERB is the half an older suite misses"
api PATCH "/claims/not-a-uuid" '{"description":"x"}'
assert_rest 400 MalformedIDNotification

case_ "W6.3 the ARCHIVE addressed by a non-uuid" "400 MalformedIDNotification — a write is a write, sub-path or not"
api PATCH "/claims/not-a-uuid/archive"
assert_rest 400 MalformedIDNotification

# ═════════════════════════════════════════════════════════════════════════════════════════
# W7 — the read vocabulary the DTO declares
#
# A dedicated tenant so every count below is exact: four definitions, deliberately spread
# across both enum sets and both default states.
# ═════════════════════════════════════════════════════════════════════════════════════════

TEN_READ=$(new_tenant active "$(ws clmr)") || exit 1
api GET "/tenants/$TEN_READ"; WS_READ=$(printf '%s' "$HTTP_BODY" | jq -r '.data.workspace')

N_R1="x_alpha_$(printf '%s' "$(qa_slug_runid)" | tr '-' '_')"
N_R2="x_bravo_$(printf '%s' "$(qa_slug_runid)" | tr '-' '_')"
N_R3="x_charlie_$(printf '%s' "$(qa_slug_runid)" | tr '-' '_')"
N_R4="x_delta_$(printf '%s' "$(qa_slug_runid)" | tr '-' '_')"
ID_R1=$(new_claim "$N_R1" string  user   "$TEN_READ" "north")   || exit 1
ID_R2=$(new_claim "$N_R2" number  client "$TEN_READ" "42")      || exit 1
ID_R3=$(new_claim "$N_R3" bool    both   "$TEN_READ" "true")    || exit 1
ID_R4=$(new_claim "$N_R4" string  both   "$TEN_READ")           || exit 1
Q="tenantID.eq=$TEN_READ"

case_ "W7.1a Name eq" "1 row — the exact string, prefix included"
api GET "/claims?$Q&name.eq=$N_R1"; assert_json_at 200 '.data | length' "1"

case_ "W7.1b Name ne" "3 rows — everything but one"
api GET "/claims?$Q&name.ne=$N_R1"; assert_json_at 200 '.data | length' "3"

case_ "W7.1c Name in" "2 rows"
api GET "/claims?$Q&name.in=$N_R1,$N_R2"; assert_json_at 200 '.data | length' "2"

case_ "W7.1d Name startswith / istartswith" "1 row each — the prefix is part of the value, so the search term carries it"
api GET "/claims?$Q&name.startswith=x_bravo"; assert_json_at 200 '.data | length' "1"
case_ "W7.1e Name istartswith, asked in the WRONG case" "1 row — the i- operator is what makes casing irrelevant to the SEARCH, though never to the stored value"
api GET "/claims?$Q&name.istartswith=X_BRAVO"; assert_json_at 200 '.data | length' "1"

case_ "W7.1f Name contains / icontains" "1 row each"
api GET "/claims?$Q&name.contains=charlie"; assert_json_at 200 '.data | length' "1"
case_ "W7.1g Name icontains" "1 row"
api GET "/claims?$Q&name.icontains=CHARLIE"; assert_json_at 200 '.data | length' "1"

case_ "W7.1h ValueType eq" "1 row — the closed set is filterable"
api GET "/claims?$Q&valueType.eq=number"; assert_json_at 200 '.data | length' "1"
case_ "W7.1i ValueType in" "3 rows"
api GET "/claims?$Q&valueType.in=string,bool"; assert_json_at 200 '.data | length' "3"

case_ "W7.1j AppliesTo eq" "2 rows — the two 'both' definitions"
api GET "/claims?$Q&appliesTo.eq=both"; assert_json_at 200 '.data | length' "2"
case_ "W7.1k AppliesTo in" "2 rows"
api GET "/claims?$Q&appliesTo.in=user,client"; assert_json_at 200 '.data | length' "2"

case_ "W7.1l DefaultValue eq" "1 row"
api GET "/claims?$Q&defaultValue.eq=north"; assert_json_at 200 '.data | length' "1"
case_ "W7.1m DefaultValue in" "2 rows"
api GET "/claims?$Q&defaultValue.in=north,42"; assert_json_at 200 '.data | length' "2"
case_ "W7.1n DefaultValue contains / icontains" "1 row each"
api GET "/claims?$Q&defaultValue.contains=ort"; assert_json_at 200 '.data | length' "1"
case_ "W7.1o DefaultValue icontains" "1 row"
api GET "/claims?$Q&defaultValue.icontains=NORT"; assert_json_at 200 '.data | length' "1"

case_ "W7.1p Description contains / icontains" "4 rows — every fixture shares the generated sentence"
api GET "/claims?$Q&description.icontains=FIXTURE"; assert_json_at 200 '.data | length' "4"

case_ "W7.1q TenantID in" "4 rows"
api GET "/claims?tenantID.in=$TEN_READ"; assert_json_at 200 '.data | length' "4"

case_ "W7.1r CreatedAt gte / UpdatedAt lte" "4 rows each — the two timestamp leaves"
api GET "/claims?$Q&createdAt.gte=2020-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "4"
case_ "W7.1s UpdatedAt lte" "4 rows"
api GET "/claims?$Q&updatedAt.lte=2999-01-01T00:00:00Z"; assert_json_at 200 '.data | length' "4"

case_ "W7.2a orderBy=name ascending" "alpha first"
api GET "/claims?$Q&orderBy=name"; assert_json_at 200 '.data[0].name' "$N_R1"
case_ "W7.2b orderBy=-name descending" "delta first"
api GET "/claims?$Q&orderBy=-name"; assert_json_at 200 '.data[0].name' "$N_R4"
case_ "W7.2c orderBy=valueType" "200 — the second declared sort"
api GET "/claims?$Q&orderBy=valueType"; assert_status 200
case_ "W7.2d orderBy=appliesTo" "200"
api GET "/claims?$Q&orderBy=appliesTo"; assert_status 200
case_ "W7.2e orderBy=tenantID" "200"
api GET "/claims?$Q&orderBy=tenantID"; assert_status 200
case_ "W7.2f orderBy=tenantWorkspace — a JOINED column in the sort vocabulary" "200"
api GET "/claims?$Q&orderBy=tenantWorkspace"; assert_status 200
case_ "W7.2g orderBy=createdAt and orderBy=updatedAt" "200 each"
api GET "/claims?$Q&orderBy=createdAt"; assert_status 200
case_ "W7.2h orderBy=updatedAt" "200"
api GET "/claims?$Q&orderBy=updatedAt"; assert_status 200

case_ "W7.3 ?fields= returns exactly the names asked for" "id and name only — every other key absent, not null"
api GET "/claims?$Q&fields=name&orderBy=name&first=1"
assert_json '[.data[0] | keys] | flatten | join(",")' "name"

case_ "W7.4 ?onlyTotal=true" "the count alone, and no data array"
api GET "/claims?$Q&onlyTotal=true"
assert_json_at 200 '.pagination.totalCount' "4"

case_ "W7.5 ?last= alone serves the TAIL window" "delta — the last row by name, not the first"
api GET "/claims?$Q&orderBy=name&last=1"
assert_json_at 200 '.data[0].name' "$N_R4"

case_ "W7.6 ?includeArchived=true is the ONLY way to see a retired definition" "with no unarchive mounted, the listing is the whole recovery surface (spec.md §9)"
api PATCH "/claims/$ID_R4/archive"
api GET "/claims?$Q"
HID=$(printf '%s' "$HTTP_BODY" | jq -r '.data | length')
api GET "/claims?$Q&includeArchived=true"
SHOWN=$(printf '%s' "$HTTP_BODY" | jq -r '.data | length')
if [ "$HID" = "3" ] && [ "$SHOWN" = "4" ]; then pass_; else fail_ "hidden=$HID revealed=$SHOWN"; fi

case_ "W7.7a the pagination envelope tells the truth on page 1" "totalCount 3, hasNextPage true, hasPreviousPage false — and THE BICONDITIONAL: an edge cursor is emitted only where its neighbouring page exists, so the head of a forward walk carries an endCursor and NO startCursor (auto-query-handlers at the pin, in as many words)"
api GET "/claims?$Q&orderBy=name&first=2"
assert_json_at 200 '[(.pagination.totalCount|tostring), (.pagination.hasNextPage|tostring), (.pagination.hasPreviousPage|tostring), ((.pagination.endCursor != null)|tostring), ((.pagination.startCursor == null)|tostring)] | join(",")' "3,true,false,true,true"
CUR_END=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.endCursor')
P1=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].id] | join(",")')

case_ "W7.7b page 2 is DISJOINT from page 1, and the cursor is a window EDGE" "one row, none of page 1's ids, hasPreviousPage true"
api GET "/claims?$Q&orderBy=name&first=2&after=$CUR_END"
P2=$(printf '%s' "$HTTP_BODY" | jq -r '[.data[].id] | join(",")')
OVERLAP=$(printf '%s\n%s\n' "${P1//,/$'\n'}" "${P2//,/$'\n'}" | sort | uniq -d | tr -d '[:space:]')
PREV=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.hasPreviousPage')
if [ -z "$OVERLAP" ] && [ "$PREV" = "true" ] && [ -n "$P2" ]; then pass_; else fail_ "page1=[$P1] page2=[$P2] overlap=[$OVERLAP] hasPreviousPage=$PREV"; fi

case_ "W7.7c walking BACKWARD with last+before returns page 1" "the other direction of the same window"
api GET "/claims?$Q&orderBy=name&first=2&after=$CUR_END"
CUR_START=$(printf '%s' "$HTTP_BODY" | jq -r '.pagination.startCursor')
api GET "/claims?$Q&orderBy=name&last=2&before=$CUR_START"
assert_json_at 200 '[.data[].id] | join(",")' "$P1"

case_ "W7.8 the JOIN reaches the counterpart's value in ONE call" "filtering by tenantWorkspace answers 'the claims of this workspace' without a second request"
api GET "/claims?tenantWorkspace.eq=$WS_READ"
assert_json_at 200 '.data | length' "3"

# The stamp is null while the owner is live, and every listing field is omitempty — so
# projectability has to be proven where the value EXISTS, against a definition whose tenant
# was retired after it was created. Asserting the key on a live owner would prove nothing
# either way.
TEN_ARC=$(new_tenant active "$(ws clmarc)") || exit 1
ID_ARCJ=$(new_claim "$(claim_name w79)" string both "$TEN_ARC") || exit 1
api PATCH "/tenants/$TEN_ARC/archive"

case_ "W7.9 ?fields=tenantArchivedAt — served and PROJECTABLE" "200 carrying the owning tenant's archive stamp (plan §0c-d); W8.11 proves the same field is in no filter and no sort"
api GET "/claims?tenantID.eq=$TEN_ARC&fields=tenantArchivedAt&first=1"
assert_json_at 200 '[.data[0] | has("tenantArchivedAt"), (.tenantArchivedAt | type == "string")] | all' "true"

case_ "W7.9b ...and the projection returns THAT KEY ALONE" "no id, no name — ?fields= shapes the row, and a joined column shapes like any other"
assert_json '[.data[0] | keys] | flatten | join(",")' "tenantArchivedAt"

case_ "W7.9c an archived OWNER does not hide its definitions" "the row is still served — the join is INNER on the foreign key, not on the counterpart's archive state"
api GET "/claims?tenantID.eq=$TEN_ARC"
GOT=$(printf '%s' "$HTTP_BODY" | jq -r --arg id "$ID_ARCJ" '[.data[]? | select(.id==$id)] | length')
if [ "$HTTP_STATUS" = "200" ] && [ "$GOT" = "1" ]; then pass_; else fail_ "HTTP $HTTP_STATUS, rows matching = $GOT"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# W8 — rejected reads: the WHOLE typed-400 guard family
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "W8.1 an unknown filter field" "400 SchemaViolationNotification — rejected, never silently ignored"
api GET "/claims?bogus.eq=x"; assert_rest 400 SchemaViolationNotification

case_ "W8.1b ?id.eq= — a filter this entity deliberately does not declare" "400 SchemaViolationNotification. Client and User each declare an ID filter on their own listing DTO; Claim declares none, and its spec §9 filter table says so. A per-entity decision, not a framework universal — which is exactly why it needs an assertion rather than an assumption"
api GET "/claims?id.eq=$ID_GOLD"; assert_rest 400 SchemaViolationNotification

case_ "W8.2 an operator outside a field's allowlist" "400 — description declares contains/icontains only, so eq is not its operator"
api GET "/claims?description.eq=x"; assert_rest 400 SchemaViolationNotification

case_ "W8.3 ?search= on a DTO that never declared it" "400 SchemaViolationNotification — the opt-in gate, and this relational posture could not serve it anyway (spec.md §9)"
api GET "/claims?search=cost"; assert_rest 400 SchemaViolationNotification

case_ "W8.4 ?onlyTotal=false — a DECLARED control sent with a value that changes nothing" "200 — the gate is about DECLARATION, not about truthiness; an undeclared control rejects on PRESENCE alone (W8.3)"
api GET "/claims?$Q&onlyTotal=false"; assert_status 200

case_ "W8.5 an unresolvable ?fields= path" "400 naming fields[bogus]"
api GET "/claims?fields=bogus"; assert_rest_field 400 SchemaViolationNotification "fields[bogus]"

case_ "W8.6 a filter VALUE outside an identity column's kind" "400 InvalidFilterValueNotification — below pin v0.70.0 this was a 500 on a relational backing"
api GET "/claims?tenantID.eq=lixo"; assert_rest 400 InvalidFilterValueNotification

case_ "W8.7 the same, on a timestamp leaf" "400 InvalidFilterValueNotification"
api GET "/claims?createdAt.gte=abc"; assert_rest 400 InvalidFilterValueNotification

case_ "W8.8 ?first= above the view's ceiling" "400 — the page ceiling is a promise, not a suggestion"
api GET "/claims?first=100000"; assert_status 400

case_ "W8.9a mixed directions: first + last" "400"
api GET "/claims?first=2&last=2"; assert_status 400
case_ "W8.9b mixed directions: first + before" "400 — forward is first+after, backward is last+before"
api GET "/claims?first=2&before=$CUR_END"; assert_status 400
case_ "W8.9c mixed directions: after + before" "400"
api GET "/claims?after=$CUR_END&before=$CUR_END"; assert_status 400

case_ "W8.10a ?onlyTotal=true beside a page-shaping control" "400 — the only-total conflict matrix"
api GET "/claims?onlyTotal=true&first=10"; assert_status 400
case_ "W8.10b ?onlyTotal=true beside a FILTER" "200 — counting a filtered subset is the point of the control"
api GET "/claims?$Q&valueType.eq=string&onlyTotal=true"; assert_status 200
case_ "W8.10c ?onlyTotal=true beside ?includeArchived" "200 — the archive gate is not a page shape"
api GET "/claims?$Q&includeArchived=true&onlyTotal=true"; assert_json_at 200 '.pagination.totalCount' "4"

case_ "W8.11a a FILTER on tenantArchivedAt" "400 — served and projectable (W7.9), addressable in NO criteria"
api GET "/claims?tenantArchivedAt.gte=2020-01-01T00:00:00Z"; assert_rest 400 SchemaViolationNotification
case_ "W8.11b ?orderBy=tenantArchivedAt" "400 — the same field, the other half of the vocabulary"
api GET "/claims?orderBy=tenantArchivedAt"; assert_status 400

case_ "W8.12a ?orderBy=defaultValue — filterable, NOT sortable" "400"
api GET "/claims?orderBy=defaultValue"; assert_status 400
case_ "W8.12b ?orderBy=description" "400"
api GET "/claims?orderBy=description"; assert_status 400
case_ "W8.12c ?orderBy=tenantStatus — a joined field that filters but does not sort" "400"
api GET "/claims?orderBy=tenantStatus"; assert_status 400

case_ "W8.13 a malformed cursor" "400"
api GET "/claims?first=2&after=not-a-cursor"; assert_status 400

case_ "W8.14a cursor <-> orderBy mismatch" "400 — a cursor is only meaningful under the ordering it was cut from"
api GET "/claims?$Q&orderBy=valueType&first=2&after=$CUR_END"; assert_status 400
case_ "W8.14b cursor <-> includeArchived mismatch" "400 — the archive gate changes the window the cursor indexes"
api GET "/claims?$Q&orderBy=name&includeArchived=true&first=2&after=$CUR_END"; assert_status 400

# ═════════════════════════════════════════════════════════════════════════════════════════
# W9 — validation 422, one per shape. The four ClaimName negative families are CL1, in the
# domain lane; here one representative proves the key reaches the envelope.
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "W9.1 a name the value object refuses" "422 InvalidClaimNameNotification"
api POST /claims "$(jq -nc --arg t "$TEN_LANE" '{name:"cost_center", valueType:"string", appliesTo:"both", tenantID:$t, description:"A definition whose name never carried the reserved prefix."}')"
assert_rest 422 InvalidClaimNameNotification

case_ "W9.2 a value type outside the closed set" "422 UnknownClaimValueTypeNotification"
api POST /claims "$(jq -nc --arg n "$(claim_name w9b)" --arg t "$TEN_LANE" '{name:$n, valueType:"json", appliesTo:"both", tenantID:$t, description:"A definition asking for a type the catalog does not accept."}')"
assert_rest 422 UnknownClaimValueTypeNotification

case_ "W9.3 an identity kind outside the closed set" "422 UnknownClaimAppliesToNotification"
api POST /claims "$(jq -nc --arg n "$(claim_name w9c)" --arg t "$TEN_LANE" '{name:$n, valueType:"string", appliesTo:"service", tenantID:$t, description:"A definition naming an identity kind this service does not mint."}')"
assert_rest 422 UnknownClaimAppliesToNotification

case_ "W9.4 a default value one rune past the claim-size budget" "422 DefaultValueTooLongNotification, the message rendering 256 through its {max} tvar"
LONG=$(printf 'a%.0s' $(seq 1 257))
api POST /claims "$(jq -nc --arg n "$(claim_name w9d)" --arg t "$TEN_LANE" --arg d "$LONG" '{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, defaultValue:$d, description:"A definition whose default overruns the header budget."}')"
assert_rest 422 DefaultValueTooLongNotification
case_ "W9.4b ...and the rendered message carries the bound" "256 in the text, from the tvar and not from a hardcoded sentence"
assert_json '[.errors[].messages[] | select(.notificationKey=="DefaultValueTooLongNotification") | .message] | join(" ") | test("256") | tostring' "true"

case_ "W9.5 a description the shared value object refuses" "422 — Description is reused, so its own notification answers"
api POST /claims "$(jq -nc --arg n "$(claim_name w9e)" --arg t "$TEN_LANE" '{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, description:"x"}')"
assert_status 422

case_ "W9.6 an omitted name" "422 RequiredFieldNotification — from the value object, ONCE. No 'required' rule is declared beside it, deliberately (spec.md §7)"
api POST /claims "$(jq -nc --arg t "$TEN_LANE" '{valueType:"string", appliesTo:"both", tenantID:$t, description:"A definition that never named itself at all."}')"
assert_rest 422 RequiredFieldNotification
case_ "W9.6b ...and the caller reads that complaint exactly once" "one message on 'name', not two — a declared 'required' beside a VO is what makes it two"
assert_json '[.errors[].messages[] | select(.field=="name")] | length' "1"

# ═════════════════════════════════════════════════════════════════════════════════════════
# W10 — the dual 409, and the wrong-state flavor that CANNOT exist here
# ═════════════════════════════════════════════════════════════════════════════════════════

case_ "W10.1 the same name twice in one tenant" "409 ClaimNameAlreadyExistsNotification — the DUPLICATE flavor"
api POST /claims "$(jq -nc --arg n "$NAME_W1" --arg t "$TEN_LANE" '{name:$n, valueType:"string", appliesTo:"both", tenantID:$t, description:"A second definition trying to take a name this tenant already uses."}')"
assert_rest 409 ClaimNameAlreadyExistsNotification

case_ "W10.2 the wrong-state 409 is N/A on this entity" "named, not asserted"
skip_ "Claim has no state machine, no transition rule and no wire field carrying a revision a caller could send stale. Every wrong-state attempt is intercepted a layer earlier by the LOAD scope, where it lands as 404 — which W10.3 asserts instead. The same derivation the tenant, permission, role and group rounds each recorded"

case_ "W10.3 a WRITE against an archived row" "404 — the LOAD scope answers before any rule does, which is why there is no 409 to assert"
api PATCH "/claims/$ID_W1B" '{"description":"A description written to a definition that was retired an hour ago."}'
assert_status 404

# ═════════════════════════════════════════════════════════════════════════════════════════
# W11 — the archive round-trip
# ═════════════════════════════════════════════════════════════════════════════════════════

NAME_W11=$(claim_name w11)
ID_W11=$(new_claim "$NAME_W11" string both "$TEN_LANE") || exit 1

case_ "W11.1 archive hides the row from by-id" "404 — the default read gate excludes it"
api PATCH "/claims/$ID_W11/archive"
api GET "/claims/$ID_W11"
assert_status 404

case_ "W11.2 ?includeArchived=true reveals it, with the stamp set" "200 and archivedAt is a timestamp, not null"
api GET "/claims/$ID_W11?includeArchived=true"
assert_json_at 200 '.data.archivedAt | type' "string"

case_ "W11.3 no child carries an archive column" "named, not asserted"
skip_ "Claim has no collection and no sibling (spec.md §3, §4), so the stamp-scoped unarchive family — a child archived on its own before the root, staying archived after it returns — has nothing to run against on this aggregate"

# ═════════════════════════════════════════════════════════════════════════════════════════
# W12 — the /openapi.json cross-check. An ENUMERATION of what exists, never an expectation.
# ═════════════════════════════════════════════════════════════════════════════════════════

api GET /openapi.json "" -
OAS="$HTTP_BODY"

case_ "W12.1 the framework enumerates exactly five claim routes" "POST /claims, PATCH /claims/{id}, PATCH /claims/{id}/archive, GET /claims, GET /claims/{id}"
GOT=$(printf '%s' "$OAS" | jq -r '[.paths | to_entries[] | select(.key | startswith("/claims")) | .value | keys[]] | length')
if [ "$GOT" = "5" ]; then pass_; else HTTP_BODY=$(printf '%s' "$OAS" | jq -c '[.paths | to_entries[] | select(.key|startswith("/claims")) | {(.key): (.value|keys)}]'); fail_ "$GOT operations"; fi

case_ "W12.2 each of the five carries its permission literal" "claim:insert, claim:update, claim:archive, claim:read x2 — a source-vs-openapi disagreement is a FINDING, never a guess resolved in silence"
GOT=$(printf '%s' "$OAS" | jq -r '[.paths | to_entries[] | select(.key|startswith("/claims")) | .value | to_entries[] | .value.description // ""] | map(capture("claim:(?<v>[a-z]+)").v) | sort | join(",")' 2>/dev/null)
if [ "$GOT" = "archive,insert,read,read,update" ]; then pass_; else HTTP_BODY=$(printf '%s' "$OAS" | jq -c '[.paths|to_entries[]|select(.key|startswith("/claims"))|.value|to_entries[]|.value.description]'); fail_ "[$GOT]"; fi

# ═════════════════════════════════════════════════════════════════════════════════════════
# W13 — the immutable fields have NO DOOR
#
# patchExcludes removes `name` and `valueType`; assignedFrom keeps `tenantID` out of every
# update body. What the wire promises is a no-op, and that is what is asserted here. The
# GraphQL halves — which answer DIFFERENTLY, and by design — are X13.
# ═════════════════════════════════════════════════════════════════════════════════════════

NAME_W13=$(claim_name w13)
ID_W13=$(new_claim "$NAME_W13" number user "$TEN_LANE" "7") || exit 1

case_ "W13.1 a PATCH naming all three immutable fields" "200 — an unknown JSON key is ignored, so the attempt is a no-op rather than an error"
api PATCH "/claims/$ID_W13" "$(jq -nc --arg t "$TEN_OTHER" '{name:"x_renamed_by_a_caller", valueType:"string", tenantID:$t, description:"An update that also tried to rewrite what every issued token carries."}')"
assert_status 200

case_ "W13.1b ...and the three values are UNCHANGED on the read-back" "the name, the type and the owner are exactly what the insert stored"
api GET "/claims/$ID_W13"
assert_json '[.data.name, .data.valueType, .data.tenantID] | join("|")' "$NAME_W13|number|$TEN_LANE"

case_ "W13.1c ...and the ONE editable field in that same body did change" "the request was honoured, not rejected wholesale — which is what makes the silence dangerous enough to pin"
assert_json '.data.description' "An update that also tried to rewrite what every issued token carries."

case_ "W13.2 the three immutability rules are UNREACHABLE through every mounted surface" "named, not asserted"
skip_ "ClaimNameIsImmutableNotification, ClaimTenantIsImmutableNotification and ClaimValueTypeIsImmutableNotification cannot be provoked through any route this service mounts: patchExcludes [Name, ValueType] removes two fields from the PATCH body and assignedFrom: identity-claim keeps TenantID out of every update body, so the door is closed one layer before the rule. They are backstops behind a closed door — the same shape P3e, RL7- and U15.2b record for their own unreachable rules. W13.1b pins what the wire DOES promise"

qa_finish
