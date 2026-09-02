# Migration plan — omnicore v0.69.0 → v0.70.0

Status: DRAFT

Nothing in this file has been applied. The `go.mod` / `go.sum` bump IS already done and
verified (`go vet -tags postgres ./...` and `go build -tags postgres ./...` both clean);
this plan covers only the service-side fallout that a green build cannot see.

Rollback point: `specs/upgrade/rollback/` (verbatim `go.mod` + `go.sum` at v0.69.0).

---

## Context — why a green build proves nothing here

The one SOURCE-breaking change in v0.70.0 (`fwweb.BindPath` returning
`*queryschema.Violation` instead of `(string, bool)`) does not touch this service: there
is no occurrence of `BindPath`, `RespondSchemaViolation` or `ApplyFilterValues` in any
`.go` file. Every handler is a framework auto-handler.

The three remaining breaking items are BEHAVIORAL — they change what the wire answers,
not what compiles. All three land on this service, because its views are relational
(`relational.dialect: postgres`, no `mongo:` block in either profile), and the relational
backing is exactly the one whose behavior moved.

Operational classes checked and clear, by diffing the two module trees:

- No DDL required — `application/configuration` is byte-identical between the pins.
- No embedded migration added — the framework's `.sql` set is identical, so no
  `autoRun: check` boot abort is coming.
- No yaml key moved, renamed, or arrived mandatory — both profiles stay valid as written.
- No shared gRPC proto change — no `.proto` / `.pb.go` differs.
- The bootstrap seed (`migrations/postgres/0012_bootstrap_seed_manual.up.sql`) writes ids
  by raw SQL, which is the case the changelog warns about for the identity-probe refusal.
  Verified safe: all 50 seeded ids are well-formed UUIDs, no literal outside the format.

---

## Item 1 — `qa/tenant.sh:653`, case I5: the notification key moved

**Trigger:** changelog v0.70.0, *"a by-id route no longer answers 500 for a `:id` that is
not a UUID"*. No compile error — this is an assertion in the in-flight contract suite.

**How it worked at v0.69.0** — section `status-mapping.html`: the section's
`SemanticNotFound → 404` row listed exactly two keys, `RecordNotFoundNotification` and
`RouteNotFoundNotification`. Nothing in the section addressed a malformed `:id` segment
at all. The wrapper bound `c.Params("id")` and passed it through; on a Postgres backing
the string reached the driver and came back as SQLSTATE 22P02, rendered as a **500**.

**How it works at v0.70.0** — same section, `status-mapping.html`: the `404` row now
reads `RecordNotFoundNotification`, `RouteNotFoundNotification`,
`UnknownIDAddressNotification`, and the section states the rule directly — *"A malformed
`:id` segment (present, and not a UUID) is refused by the wrapper BEFORE the handler, in
context `Request` with `id` echoed on `field` and the rejected segment on `value`. What
the consumer hears follows the VERB, not the view's backing: a READ answers
`UnknownIDAddressNotification` → `SemanticNotFound` → 404, a WRITE answers
`MalformedIDNotification` → `SemanticSchema` → 400."* The section also keeps the three
404 keys distinct on purpose: *"the route did not exist, the record does not exist, or
what you sent is not an address at all."*

**Effect on the case:** `GET /tenants/not-a-uuid` is a READ, so the **status the case
asserts (404) becomes correct for the first time** — at v0.69.0 this case was asserting a
contract the service did not deliver. Only the key is now wrong:
`RecordNotFoundNotification` is reserved for a row that genuinely does not exist.

**Proposed edit — `qa/tenant.sh`, line 651-653:**

```diff
-# DECIDED at the gate (Q2): 404 is the only defensible contract for an id that cannot
-# name a record. A 500 here is a FINDING about the service, not a case to weaken.
+# The pin PROMISES this since v0.70.0: a malformed :id is refused at the wrapper, before
+# the handler, and the answer follows the VERB — a READ names no record, so 404 with its
+# own key, distinct from the 404 of a well-formed id that matched nothing (I1).
 api GET "/tenants/not-a-uuid"
-expect "I5 a syntactically impossible id" 404 "RecordNotFoundNotification"
+expect "I5 a syntactically impossible id" 404 "UnknownIDAddressNotification"
```

The other six `RecordNotFoundNotification` assertions (`F1`, `F6`, `F7`, `I1`, `J5b`,
and the GraphQL `J6` idiom) are **unaffected** — every one of them sends a well-formed
UUID that names no row, which is precisely the meaning v0.70.0 leaves untouched.

---

## Item 2 — `specs/qa/tenant/plan.md`, the Q2 decision record is now false

**Trigger:** the same changelog item. The plan records the reasoning behind case I5, and
its stated premise stopped being true at the bump.

The record currently reads (lines 258-262):

> `I5` `GET /tenants/not-a-uuid` → **404**. DECIDED (Q2): the pin's docs promise nothing
> here and `domain.NewID` does not validate — the string is bound against a `uuid` column.
> 404 is the only defensible contract (a syntactically impossible id names no record); a
> 500 is a genuine FINDING about the service, reported verbatim and routed to
> `/omnicore:doctor`, never patched away in the case.

*"The pin's docs promise nothing here"* was accurate at v0.69.0 — verified: neither
`UnknownIDAddress`, `MalformedID` nor any malformed-`:id` rule appears in that pin's
`status-mapping.html`. At v0.70.0 the docs promise exactly this, and the conclusion the
gate reached by reasoning is now the documented contract.

**Proposed edit — replace that paragraph with:**

> `I5` `GET /tenants/not-a-uuid` → **404** `UnknownIDAddressNotification`. DECIDED (Q2)
> by reasoning when the pin was v0.69.0, where the docs promised nothing here and a 500
> was the observed behavior on the relational backing. **v0.70.0 made it a documented
> contract** (`status-mapping.html`): the wrapper refuses a malformed `:id` before the
> handler, and the answer follows the verb — a READ answers 404
> `UnknownIDAddressNotification`, a WRITE answers 400 `MalformedIDNotification`. The key
> is deliberately NOT `RecordNotFoundNotification`: that one means a well-formed address
> named no row (`I1`), and the two stay distinct so a consumer can tell a bad address
> from an unknown one.

---

## Item 3 — the route inventory oracle is safe, no edit needed

Recorded so it is not re-checked later. v0.70.0 adds a `400` response to `HasPathID`
routes in the OpenAPI document. Case `X1b` (`qa/tenant.sh:266`) projects only
`.paths | to_entries[] | (method + " " + path)` — method and path keys, never response
codes — so the added `400` does not move its expected string. **No change.**

---

## ⚠️ OPEN: does the suite take on the new contract surface v0.70.0 exposes?

This is a scope question, not a fix, so it is not decided here. v0.70.0 introduces two
promises the suite currently has **no case for**, and both are cheap to prove on a
service that already has the fixtures:

1. **A by-id WRITE with a malformed id → 400 `MalformedIDNotification`.** The suite's `I`
   section covers the READ arm (I5) but never the WRITE arm, e.g.
   `PATCH /tenants/not-a-uuid` or `PATCH /tenants/not-a-uuid/archive`. This is the half of
   the new rule that behaves DIFFERENTLY from the read, so it is the half a regression
   would most easily hide in.
2. **A bad filter value → 400 `InvalidFilterValueNotification`.** Section `H` proves
   fifteen flavors of typed 400 but every one of them is a violation of the query
   *grammar* (unknown key, operator outside the allowlist, bad cursor). None sends a
   well-formed key carrying a value the column cannot hold — e.g. a non-uuid on an
   identity leaf, which at v0.69.0 was a 500 on Postgres and is now a typed 400.

Adding these is beyond repairing the bump's fallout, and the tenant suite is yours in
flight on this branch — so: **add both cases now, add neither, or add only one?**

---

## Needs your attention — not auto-fixable, no edit proposed

- **The three behavioral changes affect every entity, not just Tenant.** `Claim`,
  `Client`, `Group`, `Permission`, `Role` and `User` all serve by-id routes over the same
  relational backing, so all of them moved from 500 to the contract on a malformed `:id`
  and on a bad filter value. Nothing is broken by this — it is strictly a service getting
  better — but any consumer, script or client that was written against the 500 will now
  see a 404 or a 400. Only the Tenant suite exists so far, so only its assertions are
  edited here; the same key applies when the remaining six suites are written.
- **A green build is not a green boot.** vet and build pass under `-tags postgres`, which
  proves the Go surface compiles against the new pin. It does not exercise the wrapper
  changes above, all of which are runtime. Running the service and the suite is the
  actual verification.
