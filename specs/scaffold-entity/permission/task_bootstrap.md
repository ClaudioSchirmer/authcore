# task: bootstrap — Permission

## Read first (mandatory, at execution time)

- `bootstrap` — the feature interfaces, what a readable feature must publish, and how the
  wiring reaches them.
- `features` — the feature contract itself.
- Convention: `conventions/bootstrap.md`. Layout and naming: `service-layout`.

## Model decisions that touch this layer

- A feature for this aggregate, alongside the tenant one already registered, publishing:
  the repository over the aggregate, **the domain service instance** (the aggregate declares
  it required, so a nil here is a runtime failure the compiler cannot catch), the view, and
  the mount of both surfaces.
- The GraphQL surface is currently inert in this service because no feature has implemented
  the GraphQL feature interface — the tenant one registers GraphQL fields through the same
  registry, so mirror exactly what it does rather than inventing a second pattern.
- Register the new feature in the existing wiring file beside the tenant feature. That file
  already exists; this task edits it, it does not recreate it.

## Acceptance

- The service is constructed and passed to every write handler — grep the aggregate's
  service-required declaration and confirm each write path sets it.
- The view is published by the feature, so the read handlers can resolve it by name.
- The service boots against the local bench: probes answer, the new routes appear in the
  generated spec, and the GraphQL fields appear in the schema.
