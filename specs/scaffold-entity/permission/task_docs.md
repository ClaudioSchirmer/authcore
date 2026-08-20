# task: docs — Permission

## Why this task exists

The project README carries a forward-declaration of the whole domain, and two of its
statements stop being true the moment this entity ships. Correcting them is part of the
run, done **after** the code exists so the text describes reality rather than intent.

## What to change in the project README

- **The scope table** — the row saying the Group / Role / Permission entities are not
  started. Permission now exists; Group and Role do not.
- **The seeding claim.** The README states the catalog "is seeded from what the code
  actually enforces". `spec.md` §B Q4 settled the opposite: the migration ships the table
  empty and an operator populates it through the API. Rewrite the sentence to say that, and
  keep the reasoning it was making — that a permission exists because a route enforces it,
  not because a customer invented it — since that part is unchanged and is why the catalog
  is global.
- **Add the entity's own section**, in the shape the Tenant section already uses: the field
  table, and the three things a reader will otherwise get wrong —
  1. the permission string is **rendered, never stored**: two columns in, one string out,
     built by one method;
  2. the resource and the action are **frozen after creation**, because the pair is the
     identity of the permission in every token and every route literal;
  3. **archive is one-way.** There is no unarchive verb, on purpose: restoring the row would
     silently re-grant it to everyone who still references it. A retired permission returns
     as a new row that must be granted explicitly, which the active-only unique index is
     what permits.
- **The wildcard note** — that a catalog row may carry a wildcard on either part, that such
  a row is grantable but never enforceable at a route, and that a wildcard resource requires
  a wildcard action because the claim matcher honors no other shape.

## Acceptance

- No statement in the README contradicts the shipped code or `spec.md`.
- The permission section points at `specs/scaffold-entity/permission/spec.md` for the full
  model, the way the tenant section points at its own.
