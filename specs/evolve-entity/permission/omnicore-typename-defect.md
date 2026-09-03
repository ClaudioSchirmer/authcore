# omnicore v0.72.0 — `__typename` is gated as introspection but never resolved

**Found:** 2026-09-03, while running authcore's contract QA after the v0.72.0 upgrade.
**Reported by:** the `evolve-entity` run on Permission — the defect is unrelated to that
change and is recorded here only because that run is where it surfaced.
**Status:** **FIXED upstream in omnicore v0.72.1** (2026-09-03), and verified here — see
"Verification" at the end. The framework team also found a SECOND fault in the same area that
this report had not: `__typename` was being folded into the selection's wire paths, which
broke the projection parse and silently dropped both field pushdown and
`ReadCriteria.Restrict`'s `FieldAccessForbiddenNotification` (403). That half is not
exercisable from this project — nothing here declares `read.fieldRestrict` — so it rests on
the framework's own tests, not on the verification below.

## Summary

`__typename` is accepted by the GraphQL **auth gate** as an introspection field, and is
treated as a meta-field by the **connection planner** — but no resolver ever answers it and
no selection step ever fills it. Every `__typename` this service returns is `null`.

Per the GraphQL specification `__typename` is `String!` — a NON-NULL meta-field available on
every object type in every selection set, and it must be answerable whether or not schema
introspection is enabled. It is not introspection; it is a meta-field.

## Two faces, both reproduced

### 1. Root `__typename` — an error, and reachable with NO bearer

```
POST /graphql   {"query":"{ __typename }"}      (no Authorization header)
HTTP 200 {"data":{"__typename":null},
          "errors":[{"message":"no resolver for field __typename","path":["__typename"]}]}
```

The gate lets it through on purpose: `introspection.go:205` lists `__typename` in
`introspectionRootFields`, so `IsIntrospectionOnlyRequest` judges the document
introspection-only and `graphql.introspection: true` makes it public — the v0.72.0 feature
working as designed. The executor then cannot serve what the gate promised
(`execute.go:36-38`, "no resolver for field").

**Reproduced with no service and no database at all**, which is the cheapest regression test
for a fix:

```go
reg := graphql.New(nil).EnableIntrospection(true)
reg.Execute(nil, `{ __typename }`, nil, "")
// -> {"data":{"__typename":null},"errors":[{"message":"no resolver for field __typename",…}]}
```

The same registry answers `{ __schema { queryType { name } } }` with `{"name":"Query"}` —
the exact string `__typename` should have returned.

### 2. Nested `__typename` — SILENT null, and this is the severe half

```
POST /graphql   (authenticated)
{"query":"{ permissions(first: 1) { __typename edges { node { __typename permission } } } }"}

HTTP 200 {"data":{"permissions":{"__typename":null,
                                 "edges":[{"node":{"__typename":null,"permission":"*:*"}}]}}}
```

Control — the same query without `__typename` — returns `{"permission":"*:*"}` correctly, so
the query, the auth and the read path are all fine. **No error is emitted at all**:
`applySelection` (`execute.go:69`) does `out[responseKey(f)] = applySelection(v[f.Name], …)`,
and `"__typename"` is never a key of a wire-shaped response map, so it writes `null` and
moves on.

This is the half that hits ordinary traffic. Apollo Client, urql and Relay append
`__typename` to **every** selection set for cache normalization; Apollo's normalized cache
keys on `__typename` + `id`. Against this service every one of those clients silently
receives `null` where the type name belongs, on a 200, with nothing in `errors[]` to say so.

## Where it lives

| File | Line | What it does |
|---|---|---|
| `web/graphql/introspection.go` | 205 | `__typename` IS in `introspectionRootFields` — the auth-gate allowlist |
| `web/graphql/introspection.go` | 31, 34 | `introspectionResolvers()` registers `__schema` and `__type` **only** |
| `web/graphql/execute.go` | 36-38 | root dispatch: unknown field → `"no resolver for field …"`, value `null` |
| `web/graphql/execute.go` | 69 | `applySelection`: nested unknown key → `null`, silently |
| `web/graphql/criteria.go` | 195-196 | `onlyTotalSelected` already reasons about `__typename` as a meta-field |

The comment at `introspection.go:198` says `__typename` is in the allowlist *"because tooling
appends it"* — the gate anticipates exactly the traffic the executor cannot serve.

## Suggested shape of the fix

The two sites are different problems and a fix for one does not cover the other.

1. **Nested** — `applySelection` should answer `__typename` from the type it is currently
   trimming, rather than looking it up in the value map. This is the one that matters for
   real clients.
2. **Root** — the root query type's name (`"Query"`, already computed for `__schema`'s
   `queryType`) should answer a root `__typename`.
3. **Not behind `EnableIntrospection`.** Gating it there would keep it broken whenever an
   operator turns introspection off, and `__typename` is not introspection — the spec makes
   it available unconditionally. `EnableIntrospection` should keep governing `__schema` /
   `__type` only.

## Fallout in authcore, left untouched

`qa/tenant.sh` J8 sends `{ __typename }` with no bearer and expects **401**. That assertion
went stale the moment `graphql.introspection: true` was declared in
`qa/microservice.qa.yaml:65` — an introspection-only POST is public **by design** now, so the
right answer is 200 whether or not this defect is fixed. Rewriting it is a separate decision
and it should become TWO cases: an introspection-only POST is public, and a POST carrying a
DATA field is still 401 (the security promise that actually matters — `qa/permission.sh` J8
already covers that half for its own lane and passes).


## Verification — omnicore v0.72.1, 2026-09-03

**Isolated** (no service, no database — `graphql.New(nil).Execute(...)`):

| Query | v0.72.0 | v0.72.1 |
|---|---|---|
| `{ __typename }` | `null` + `"no resolver for field __typename"` | `{"__typename":"Query"}` |
| same, **introspection OFF** | `null` + the error | `{"__typename":"Query"}` |
| `{ kind: __typename }` | — | `{"kind":"Query"}` — alias honored |

Suggestion 3 of this report was taken: it is not gated by `EnableIntrospection`, so it
resolves with introspection off, which is the default.

**Against the running service**, five checks:

1. nested resolves at every level with the real schema type names — `PermissionConnection`,
   `PermissionEdge`, `Permission`, `PageInfo` — aliases included;
2. control: the identical query without `__typename` returns identical data, so the
   meta-field perturbs no read;
3. root with no bearer → `200 {"data":{"__typename":"Query"}}`, no internal error string;
4. a data field with no bearer → still `401 MissingAuthorizationNotification`;
5. `__typename` beside a data field → still `401`; the grant stays introspection-ONLY.

**Project gates on v0.72.1:** `gofmt`, `go vet`, `go build`, the unit suite and
`omnicore-gen doctor` all clean; `./qa/run.sh --all` **450 cases, 2/2 suites, ALL GREEN**.

## The authcore-side fallout — RESOLVED

`qa/tenant.sh` J8 asserted 401 for `{ __typename }`, a rule this profile opted out of the
moment `graphql.introspection: true` was declared. It was stale independently of the
framework defect, and the framework fix could not have turned a 200 into a 401. It is now
**five** cases that pin the actual boundary — the schema is readable, the data is not:

| Case | Asserts |
|---|---|
| `J8a` | an introspection-only document is public — 200, by configuration |
| `J8b` | it answers the root type name (`Query`), not an internal error |
| `J8c` | and carries nothing in `errors[]` |
| `J8d` | a document reaching DATA is still 401 |
| `J8e` | a meta field beside a data field smuggles nothing past the gate — still 401 |

`J8b`/`J8c` would have failed on v0.72.0; `J8d`/`J8e` are the security half the single old
case never covered.
