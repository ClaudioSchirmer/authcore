# task: bootstrap — Role

Model: `spec.md` §9, §10. Convention: `conventions/bootstrap.md`. Layout: `service-layout.html`.

## READ before writing (mandatory)

- `bootstrap.html` — the feature contract, the dependency bag, the registration order.
- `features.html` — how a feature exposes its surfaces.
- `graphql.html` — the surface stays unmounted until a feature implements its contract.

## What to build

**A feature for this aggregate**, mirroring the two that exist. It constructs the
repository once and threads that single loader into the view — the repository already owns
one, bound to the right schema, and building a second is both redundant and a boot-assert
risk.

**The cross-aggregate construction.** This aggregate's service reaches the catalog and the
tenant tables, so the feature builds or receives those repositories too. Prefer whatever the
service-to-service section prescribes over inventing a sharing mechanism here.

**The service is wired end to end.** The aggregate declares that it requires one, which
obliges the wiring to inject it: every write handler must receive it, and a missing one is a
runtime refusal the compiler cannot catch. This is the check that has to be made explicitly,
per handler, not assumed from the constructor.

**Registration** in the service's wiring alongside the two existing features. Both surfaces
are registered per §9.

**No configuration change in this run.** Q4 left the service-wide authorization switch to
the configuration skill; this feature must not turn it on as a side effect.

## Acceptance

- The feature appears in the wiring's feature list.
- One loader per aggregate, shared by repository and view.
- Every write handler receives the service.
- The service boots: the empty-shell warning is gone, the new routes appear in the generated
  specification, and the GraphQL surface mounts.
