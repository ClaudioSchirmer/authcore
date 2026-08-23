# Task 6 — bootstrap

## Docs to READ (mandatory, at the pin)

- `bootstrap` — the feature unit and how the composition root assembles it.
- `features` — what a feature registers, and in which order.
- `auto-handlers` — what a write handler needs supplied, and what happens at runtime when it
  is not.

Convention: `conventions/bootstrap.md`. Layout and naming per `service-layout.html`.

## What this layer does

Register the entity's feature in the composition root beside the three that already exist,
mirroring their shape: the repository, the schema, the view, the queries, the commands, the
domain service, and the route registration for both surfaces.

**The one thing `go build` cannot catch here.** This entity's domain requires a service
(spec §7). A write handler left without one is a **runtime** failure, not a compile error —
the request answers with the framework's service-is-required notification. So: the feature
must construct the service, pass it where the mount expects it, and **every write handler
must be given it** — the root's create, patch and archive, and both collection operations.
This is on the final verify's mechanical checklist for exactly that reason.

The service implementation needs the tenants and roles repositories in addition to its own
(`task_infra.md`), so the composition root has to hand it all three.

Nothing else in the service changes: no profile edit, no posture change. The permission gate
stays inert until `auth.authorization` is configured, which is a service-wide change owned by
`/omnicore:configure` and explicitly out of scope here (spec §10).

## Acceptance check

- The feature is registered and the service booted with the service instance wired.
- Every write handler of this entity has the service supplied — none left unset.
- Both surfaces register their routes.
- `go build` with the engine and transport tags, and `go vet`, clean.
