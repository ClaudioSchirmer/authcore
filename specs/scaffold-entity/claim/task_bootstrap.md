# task_bootstrap — Claim

Model authority: [`spec.md`](spec.md) §9, §10. Layout and naming: `service-layout.html`.

## Read BEFORE writing this layer

| What | Where |
|---|---|
| the feature unit and how it is registered | `bootstrap.html` · `features.html` |
| the relational read-model seam, sibling of the Mongo one | `relational-view.html` · `shared/read-side.md` |
| the layer's process and traps | `conventions/bootstrap.md` |

## What this layer must contain

**One feature unit for this entity**, registered in the existing wiring file alongside the
ones already there, contributing:

- the aggregate's schema and repository;
- **the domain service — constructed and passed to the mount.** `RequiresService()` is true
  for this entity, and a nil service there is a runtime refusal that `go build` cannot
  catch. Every write handler must receive it;
- the five REST routes and the five GraphQL operations;
- the relational read model, through the relational feature seam.

Nothing else changes: no new profile key, no new infrastructure, no transport, no broker.
The posture is unchanged by this entity.

## Traps

- **A write handler mounted without the service.** Silent at compile time, a refusal at
  runtime. The final checklist greps for exactly this.
- **Registering the read model on the Mongo seam.** There is no Mongo in this service.
- **Wiring the routes on one surface and forgetting the other.** `spec.md` §9 says both.

## Acceptance

- The service boots against a live engine with the new feature registered.
- Every write handler for this entity has a non-nil service.
- The OpenAPI document lists the five routes; the GraphQL schema lists the five operations.
- No profile file changed.
