# task_web — Claim

Model authority: [`spec.md`](spec.md) §5, §6, §8, §9, §10. Layout, naming and granularity:
`service-layout.html`.

## Read BEFORE writing this layer

| What | Where |
|---|---|
| REST routes, the OpenAPI contract, the required-field rule for query parameters | `openapi.html` · `reference.html` |
| the GraphQL surface | `graphql.html` |
| which handler kind enforces which field contract | `auto-handlers.html` · `auto-query-handlers.html` |
| the permission gate at each registration unit | `authz-seams.html` (Layer 1) |
| the layer's process and traps | `conventions/web.md` |

## What this layer must contain

**Five operations, on both surfaces**, with the permissions `spec.md` §10 tables:

| Operation | Shape | Permission |
|---|---|---|
| create a definition | collection write, full body | `claim:insert` |
| patch a definition | by-id partial write, the three fields of §8 | `claim:update` |
| archive a definition | by-id intent, no body, `PATCH …/archive` | `claim:archive` |
| read one definition | by-id | `claim:read` |
| list definitions | by-params, the vocabulary of §9 | `claim:read` |

**No sixth route.** No unarchive, no `DELETE`, no export, no collection sub-resource. A soft
removal never rides behind `DELETE`, and there is nothing here that is soft-removed except
the root, which uses the archive intent.

**Request and response shapes.** Reads serve the six stored fields plus the two that arrive
across the join into the owner. The listing declares the reserved controls `spec.md` §9 marks
served — and **not** free-text search, which this posture answers with a typed 400 rather
than a result set: declaring it would advertise what the read model cannot do.

**The OpenAPI examples carry the prefix.** The example for the claim name is a prefixed
value. Getting this wrong is not cosmetic: the example is the first thing an integrator
copies, and an unprefixed one produces a refusal on their first call with no hint why.

## Traps

- **A scalar query-tagged field declared as a value type renders REQUIRED in the OpenAPI
  spec.** Every optional filter is a pointer or a slice; one accidental value type turns an
  optional parameter mandatory and the UI refuses the call without it.
- **`?fields=` opt-in makes the whole response shape pointer/slice with omit-empty**, nested
  types included — on both the response and the query's result. Two separate boot guards,
  and both panic rather than misbehave quietly.
- **Declaring the path id on a by-id request is a boot panic** — the by-id spec owns it.
- **Authorization is not surface-specific.** The same permission is demanded at each
  surface's registration unit; deciding it once per operation and wiring it twice is the
  point, not duplication to be refactored away.
- **The root-archive handler trap does not apply here** — there is no child route to
  mis-wire it onto. Do not import the pattern from `Role` looking for a place to put it.

## Acceptance

- Five REST routes and five GraphQL operations, each demanding the permission tabled above.
- The OpenAPI document renders: no filter parameter is mandatory, every example is a real
  value rather than the type's name, and the claim-name example is prefixed.
- The archive route answers the service's established no-body shape on REST and the
  established success shape on GraphQL.
- `grep 'path:"id"' internal/web/requests/` hits nothing new.
- `grep 'query:"'` over this entity's requests: every scalar hit is a pointer or a slice.
