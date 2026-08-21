# task: application — Permission

## Read first (mandatory, at execution time)

- `auto-handlers` — what the insert / partial-update / archive command handlers expect from
  a command and its result.
- `command-handler` and `custom-command-handler` — the mapper contract
  (`ToEntity` / `ApplyPartiallyTo` / `FromEntity`) and where identity may be translated.
- `auto-query-handlers` — the read's result anatomy, **and the computed-field section**: the
  derivation runs in `FromQueryResult`, before any transport sees the value.
- `custom-query-handler` — `ToCriteria` and how a filter reaches the store.
- `lifecycle-map` — what one write touches end to end, and that both update verbs share the
  audit verb while `actionName` tells them apart.
- Convention: `conventions/application.md`. Layout and naming: `service-layout`.

## Model decisions that touch this layer

From `spec.md` §5, §8, §9:

- **Commands**: insert, partial update, archive. **No unarchive command** — the mode does
  not exist.
  - the insert mapper builds the composite from the two raw wire strings by constructing the
    value object from its two parts; it never concatenates them.
  - the partial-update mapper is lenient: only fields present in the body are applied. The
    key's parts are accepted by the mapper and refused by the domain's immutability rule —
    that ordering is deliberate, so the caller reads a domain notification rather than a
    silent drop.
  - the archive command carries no body.
- **Queries**: by id and by params. Both results carry the two stored parts under their
  exposed names, the description, the managed timestamps, and the derived permission string.
  The result declares **no** wire tags — naming belongs to the web layer.
- **The computed derivation lives here, once per document — and it delegates.** Each
  query's `FromQueryResult` **reconstructs the composite value object from the result's two
  part values and calls its rendering method**; the rendered value lands on a result field
  that no column backs. It does not join two strings: the format lives in the domain, in the
  value object that also validated it, and this layer only asks. The same applies on the
  write side — the command mapper that builds the result from the entity calls the same
  method on the entity's own field.
- **The result carries the derived field as well as its two sources.** The framework
  boot-guards Result↔Response alignment: a response field with no same-named result field is
  a panic. The two sources stay on the result and never reach the response.
- **Filters** reach `ToCriteria` per the operator table in `spec.md` §9. No data-access
  restriction is applied: the catalog is global, so nothing is filtered by identity or
  tenant — record that explicitly in the query rather than leaving it unmentioned.
- **The seven translation catalogs** gain an entry per new notification key and per new
  field label. Real translations in all seven languages — this is the one place non-English
  text is allowed by the project's own rules.

## Acceptance

- The rendered permission string is produced by calling the value object's method, on both
  the read path and the write path. No concatenation exists in this layer.
- No unarchive command exists.
- Every notification struct the domain declares has a key in all seven catalogs.
- Builds and vets clean.
