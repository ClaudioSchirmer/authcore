# task_bootstrap.md — bootstrap

Model authority: [`spec.md`](spec.md) §9, §10.

## What this layer must contain

**The feature that constructs the repository, the domain service, the new hashing adapter
and the read model, and mounts the entity on both surfaces** — REST with its published
document, and GraphQL through the shared registry.

**The domain service is wired end to end.** The aggregate declares that it requires one, so
the framework refuses the write at invocation if it is missing rather than passing a nil the
rules would dereference — and every write handler must be given it. A nil there is a runtime
refusal that compilation cannot catch.

**Registration in the composition root**, in the same shape every existing feature uses.

## What to read before writing — routed sections at the pin

`bootstrap` · `features` · `graphql` · `service-layout`.
Convention: `conventions/bootstrap.md`.

## Acceptance

- The service boots.
- Every write handler for this entity is given the domain service.
- Both surfaces list the entity's operations.
