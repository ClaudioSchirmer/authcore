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
