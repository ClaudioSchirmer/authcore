# task: infra — Permission

## Read first (mandatory, at execution time)

- `table-schema` — the schema builder, the managed-column-by-presence rule, **the composite
  decomposition surface** (the constructor, its per-part declaration, the exposed-name
  alias, the once rule) and every boot panic it can raise.
- `relational-view` — what a relationally-backed view serves and what it refuses, at this
  exact pin.
- `views` — the view definition surface and its version field.
- `custom-command-handler` — the shape of the domain service the aggregate requires.
- `../../../shared/dialects/postgres.md` (plugin) — the constraint key a unique violation
  carries on this engine, and the partial-index form for active-only uniqueness.
- Convention: `conventions/infra.md`. Layout and naming: `service-layout`.

## Model decisions that touch this layer

From `spec.md` §1, §2, §9:

- **The table schema** maps: the framework id, the revision, the description, the archive
  timestamp and both audit timestamps as ordinary declarations — and **the composite
  through the decomposition surface, not through two plain field declarations**. Each part
  is declared with its own column; **no exposed-name alias is used**, because the parts'
  own names are already the right wire names for a value object this specific. Passing the
  composite to the plain field declaration is a boot panic naming the fix; so is declaring
  a part outside the decomposition.
- The archive column is declared, because the aggregate's modes include archive. The two
  must agree or the repository construction aborts the boot.
- **The repository** binds the partial unique index's constraint name to the duplicate
  notification, so the race that slips past the pre-check still surfaces as the intended
  409 rather than a raw 500. On postgres the bound key is the constraint's **name**, which
  is why the migration names it deterministically — this binding and that name are one
  decision written in two places.
- **The domain service implementation**: the active-scope "is this pair taken, excluding
  this id" probe, satisfied through the loader's hydration-free existence check rather than
  by loading a row. Its scope is active rows only — an archived remnant must not answer
  "taken", or re-inserting a retired permission becomes impossible and `spec.md` §6's only
  route back closes.
- **The view** is relationally backed by the aggregate's own loader — the same one the
  repository uses, never a second — with the read ceiling the project already applies. It
  starts at version one. Nothing about the computed field appears here: the derivation is
  an application-layer concern and the view has no column for it.

## Acceptance

- One schema per file; the composite is declared through the decomposition surface.
- The constraint binding uses the exact name the migration creates.
- The service's probe is scoped to active rows.
- The view reuses the repository's loader.
- Builds and vets clean.
