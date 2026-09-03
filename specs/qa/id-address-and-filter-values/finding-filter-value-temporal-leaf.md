# Finding — the v0.70.0 filter-value guard misses the temporal leaf kind

**Component:** omnicore (framework) · **Version:** v0.70.0
**Severity:** an external HTTP 500 on a well-formed request, on every relational backing
**Found by:** authcore's contract QA suite, probing the v0.70.0 contracts, 2026-09-02
**Companion:** the by-id address fix shipped in the same release works correctly — this
is the *other* v0.70.0 promise, and it landed for every leaf kind except one.

---

## Summary

v0.70.0 promises:

> *Changed — breaking — a filter value the leaf cannot take is refused instead of passed
> through. `?age=abc` on an `int64`-declared leaf fell through as the raw string … on
> every relational backing it reached the driver and came back as a 500. It is now 400
> `InvalidFilterValueNotification`, named by the wire key … **so the probe never reaches
> the driver**.*

It does reach the driver for a **`time.Time`** leaf. Every other declared kind is guarded
correctly, so this is one missed kind rather than a feature that failed to land.

## Reproduction

authcore at omnicore v0.70.0, Postgres, relational read models, authenticated with a
token holding `*:*`. `FindTenantsRequest` declares the leaf as
`CreatedAt *time.Time \`query:"createdAt" filter:"eq,gte,lte,gt,lt"\`` — so the DTO
declares the kind, which is the wire-guarded path.

```
GET /tenants?createdAt.gte=not-a-date      → 500
GET /tenants?createdAt=not-a-date          → 500

{"success":false,"status":500,"description":"Internal Server Error",
 "errors":[{"context":"Server","messages":[
   {"notificationKey":"InternalServerErrorNotification",
    "message":"Internal server error.","semantic":"Internal"}]}]}
```

Server log, both requests:

```
ERROR: invalid input syntax for type timestamp with time zone (SQLSTATE 22P02)
```

Both the range operator (`gte`) and equality fail identically, which places the defect in
the leaf-kind classification rather than in an operator path.

## What works — one probe per declared kind

Same build, same session, same token:

| request | leaf kind the DTO declares | answer |
|---|---|---|
| `GET /users?tenantID=lixo` | `*domain.ID` | **400** `InvalidFilterValueNotification` ✅ |
| `GET /roles?tenantID=lixo` | `*domain.ID` | **400** ✅ |
| `GET /users?mustChangePassword=abc` | `*bool` | **400** ✅ |
| `GET /users?id=lixo` | `*string` over an identity column — the reader-guarded path | **400** ✅ |
| `GET /tenants?createdAt.gte=not-a-date` | `*time.Time` | **500 external** ❌ |
| `GET /tenants?createdAt=not-a-date` | `*time.Time` | **500 external** ❌ |

The `*string`-over-identity row is worth noting: the *harder* half of the feature — where
the wire cannot see the kind and the relational reader has to guard it — works. The
temporal kind is missed on the easier half, where the DTO states the type outright.

## Second, smaller discrepancy — the echoed field name

The changelog says the refusal is *"named by the wire key"*. Two of the four working rows
echo the **Go field name** instead:

| request | `field` echoed | wire key |
|---|---|---|
| `?tenantID=lixo` | `TenantID` | `tenantID` |
| `?id=lixo` | `ID` | `id` |
| `?mustChangePassword=abc` | `mustChangePassword` ✅ | `mustChangePassword` |

A consumer matching on `field` to highlight the offending input gets a name that never
appeared on the wire. Cosmetic next to the 500, and in the same feature.

## Ownership — framework

Checked as three separate questions, same method as the by-id finding filed against
v0.69.0:

- **Not the service.** `omnicore-gen doctor` is clean on all seven entities — no
  generated file is hand-edited.
- **Not the generator.** The DTO it emitted is correct and idiomatic:
  `*time.Time` with `filter:"eq,gte,lte,gt,lt"` is exactly how a temporal filter leaf is
  declared. `omnicore-gen explain` has no key concerning filter-value coercion; there is
  nothing a spec could have said differently.
- **The framework.** The kind classification, the wire guard, the relational reader's
  fallback guard and the field naming are all omnicore code.

## Suggested reading of the fix

Not prescribed — depth is the framework team's call. The shape suggested by the evidence
is that whatever table maps a declared leaf kind to a coercion+refusal has entries for
the identity, boolean and integer kinds and no entry for the temporal one, so the value
falls through to the driver exactly as every kind did before v0.70.0. The
`*string`-over-identity path proves the reader-side guard exists and works, so a temporal
entry on either side would close it.

## Test coverage now in place

authcore's suite carries two cases asserting the documented contract, deliberately RED
until this is fixed (`specs/qa/id-address-and-filter-values/plan.md`, gate decision Q1):

- `L1` — `?createdAt.gte=not-a-date` → expects 400 `InvalidFilterValueNotification`
- `L2` — `?createdAt=not-a-date` → same

They turn green on their own when the fix ships; nothing in the suite needs editing.

## Environment

- omnicore `v0.70.0`, omnicore-gen shipped with plugin `0.59.0`
- Postgres 17 (`postgres:17-alpine`), `relational.dialect: postgres`, relational read
  models (no Mongo, no CDC)
- `auth.mode: jwt`, `authorization.enabled: true`; reproduced with a `*:*` token, so no
  authorization layer is involved

---

# RESOLVED at omnicore v0.71.0 — verified 2026-09-02

**Status: CLOSED.** The 500 is gone. Verified by running the suite that filed this, plus a
fresh probe of every leaf kind, against v0.71.0 on the same bench.

The framework's own changelog names the cause exactly where this write-up guessed it — and
names it more precisely: the defect was the AXIS, not a missing entry in a table.

> *The read-side coercion switched on `reflect.Kind` alone, and its fallback returned the
> wire string verbatim while reporting success — a conversion it had not performed. A
> `*time.Time` leaf collapses to `reflect.Struct` … `reflect.Kind` is a closed enum of Go's
> primitive shapes with no member for a date, an identity or a duration. The declared TYPE
> is now consulted before the kind.*

That is why the identity and boolean kinds worked while the temporal one did not: they were
reachable through `Kind`, and a date never was.

## The two cases that were left deliberately RED are now GREEN

`qa/tenant.sh` was not edited — not one character. Both cases turned green on their own, as
the plan said they would:

```
── L · filter values outside the leaf's declared kind
  ✓ GREEN  L1 a range operator on a temporal leaf — HTTP 400 · InvalidFilterValueNotification
  ✓ GREEN  L2 equality on the same temporal leaf  — HTTP 400 · InvalidFilterValueNotification
```

Full run at v0.71.0: **445 cases, 2 lanes, 0 RED** (`qa/qa-report.md`).

## Probe of every declared leaf kind, re-run at v0.71.0

| request | leaf kind | v0.70.0 | v0.71.0 |
|---|---|---|---|
| `users ?tenantID=lixo` | `*domain.ID` | 400, field `TenantID` | **400, field `tenantID`** ✅ |
| `roles ?tenantID=lixo` | `*domain.ID` | 400, field `TenantID` | **400, field `tenantID`** ✅ |
| `users ?mustChangePassword=abc` | `*bool` | 400 ✅ | 400 ✅ |
| `users ?id=lixo` | `*string` over an identity column | 400, field `ID` | 400, field `ID` — unchanged |
| `tenants ?createdAt.gte=not-a-date` | `*time.Time` | **500 external** ❌ | **400 `InvalidFilterValueNotification`, field `createdAt.gte`** ✅ |
| `tenants ?createdAt=not-a-date` | `*time.Time` | **500 external** ❌ | **400, field `createdAt`** ✅ |

The temporal leaf now echoes the wire key including the operator suffix, and the refusal
carries `semantic: "Schema"` — a consumer typo is reported as a consumer typo.

## What remains — cosmetic, and narrower than reported

The second, smaller discrepancy this finding raised is **fixed for the identity leaves the
wire can see** (`tenantID` was `TenantID`) and **survives in exactly one place**:

```
GET /users?id=lixo  →  400 InvalidFilterValueNotification, "field": "ID"
```

That is the reader-guarded path — a leaf the DTO declares as a plain `*string` over an
identity column, where the wire cannot see the type and the relational reader mints the
refusal from the column instead. It is the one layer that knows the column and not the
query key, so the Go field name is what it has to hand. Cosmetic, one route shape, and
worth mentioning only because the rest of the same discrepancy is now closed.

No new case was added for it: this service's `?id=` leaf belongs to the user aggregate, and
asserting it from the tenant or permission lanes would be a case against a route those
lanes do not own. It belongs to a future `user` lane.

## Environment of the verification

- omnicore `v0.71.0`, omnicore-gen shipped with plugin `0.59.0`
- Postgres 17, `relational.dialect: postgres`, relational read models, no Mongo/CDC
- `auth.mode: jwt` with a `*:*` token, so no authorization layer is involved
- Reproduced through `./qa/run.sh --all` and a direct probe on a throwaway database
